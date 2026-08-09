package imagetask_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	domainproject "github.com/fatballfish/pic-gallery/internal/domain/project"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	compatservice "github.com/fatballfish/pic-gallery/internal/service/compat"
	"github.com/fatballfish/pic-gallery/internal/service/imagetask"
	projectservice "github.com/fatballfish/pic-gallery/internal/service/project"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type failingProjectResolver struct{ err error }

func (r failingProjectResolver) ResolveForWrite(context.Context, int64, string) (domainproject.Project, error) {
	return domainproject.Project{}, r.err
}

func TestProjectResolverInfrastructureFailuresRemainInternalWithCause(t *testing.T) {
	dbDown := errors.New("project database unavailable")
	svc := imagetask.NewServiceWithStore(taskTestConfig(), imagetask.NewMemoryStore())
	svc.SetProjectResolver(failingProjectResolver{err: dbDown})
	operations := map[string]func() error{
		"create": func() error {
			_, err := svc.CreateTask(t.Context(), domainimagetask.CreateRequest{
				UserID: 41, TaskType: string(provider.TaskTypeTextToImage), AbstractModel: "plus",
				Prompt: "resolver failure", SizeMode: "auto", OutputFormat: "png", OutputImageCount: 1,
			})
			return err
		},
		"execute": func() error {
			_, err := svc.Execute(t.Context(), domainimagetask.ExecuteRequest{
				UserID: 41, TaskType: string(provider.TaskTypeTextToImage), AbstractModel: "plus",
				Prompt: "resolver failure", SizeMode: "auto", OutputFormat: "png", OutputImageCount: 1,
			})
			return err
		},
		"task list": func() error {
			_, err := svc.ListByUserProject(t.Context(), 41, "project-1")
			return err
		},
		"gallery list": func() error {
			_, err := svc.ListGalleryByUser(t.Context(), 41, domainimagetask.GalleryListRequest{ProjectID: "project-1"})
			return err
		},
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			err := operation()
			if !errors.Is(err, dbDown) {
				t.Fatalf("error = %v, want preserved database cause", err)
			}
			if mapped := compatservice.MapError(err); mapped.StatusCode != http.StatusInternalServerError || mapped.Code != errs.CodeInternal {
				t.Fatalf("mapped error = %#v, want 500 internal", mapped)
			}
		})
	}
}

func TestCreateTaskResolvesOwnedProjectAndRejectsForeignProject(t *testing.T) {
	ctx := context.Background()
	projectSvc := projectservice.NewService(projectservice.NewMemoryStore())
	defaultProject, err := projectSvc.EnsureDefault(ctx, 501)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := projectSvc.Create(ctx, 502, domainproject.CreateRequest{Name: "Foreign"})
	if err != nil {
		t.Fatal(err)
	}
	store := imagetask.NewMemoryStore()
	svc := imagetask.NewServiceWithStore(taskTestConfig(), store)
	svc.SetProjectResolver(projectSvc)

	created, err := svc.CreateTask(ctx, domainimagetask.CreateRequest{
		UserID: 501, AbstractModel: "plus", TaskType: string(provider.TaskTypeTextToImage),
		Prompt: "project fallback", SizeMode: "auto", OutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask omitted project: %v", err)
	}
	if created.ProjectID != defaultProject.ID || created.Project == nil || created.Project.ID != defaultProject.ID {
		t.Fatalf("created task project = %q %#v, want default %#v", created.ProjectID, created.Project, defaultProject)
	}
	if _, err := svc.CreateTask(ctx, domainimagetask.CreateRequest{
		UserID: 501, ProjectID: foreign.ID, AbstractModel: "plus", TaskType: string(provider.TaskTypeTextToImage),
		Prompt: "foreign project", SizeMode: "auto", OutputImageCount: 1,
	}); err == nil {
		t.Fatal("CreateTask accepted a foreign explicit project")
	}
}

func TestCreateTaskPersistsNormalizationFailureInResolvedDefaultProject(t *testing.T) {
	ctx := context.Background()
	projectSvc := projectservice.NewService(projectservice.NewMemoryStore())
	defaultProject, err := projectSvc.EnsureDefault(ctx, 503)
	if err != nil {
		t.Fatal(err)
	}
	store := imagetask.NewMemoryStore()
	svc := imagetask.NewServiceWithStore(taskTestConfig(), store)
	svc.SetProjectResolver(projectSvc)

	const taskID = "50350350-3503-4503-8503-503503503503"
	_, err = svc.CreateTask(ctx, domainimagetask.CreateRequest{
		TaskID: taskID, UserID: 503, AbstractModel: "plus", TaskType: string(provider.TaskTypeTextToImage),
		Prompt: "invalid transparent jpeg", SizeMode: "auto", OutputFormat: "jpeg", Background: "transparent", OutputImageCount: 1,
	})
	if err == nil {
		t.Fatal("CreateTask accepted transparent JPEG")
	}
	failed, loadErr := store.GetByID(ctx, 503, taskID)
	if loadErr != nil {
		t.Fatalf("load persisted failure: %v", loadErr)
	}
	if failed.Status != domainimagetask.StatusFailed || failed.ProjectID != defaultProject.ID {
		t.Fatalf("normalization failure project = %q status=%q, want default %q and failed", failed.ProjectID, failed.Status, defaultProject.ID)
	}
}

type fakeAssetLoader struct {
	inputs map[string]provider.ImageInput
	calls  []string
}

func (f *fakeAssetLoader) LoadInput(userID int64, assetID string) (provider.ImageInput, error) {
	f.calls = append(f.calls, assetID)
	input, ok := f.inputs[assetID]
	if !ok {
		return provider.ImageInput{}, errors.New("asset not found")
	}
	return input, nil
}

type fakeProvider struct {
	generateFunc func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error)
	editFunc     func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error)
}

type userLimitedStore struct {
	imagetask.Store
	limit int
}

func (s *userLimitedStore) UserConcurrencyLimit(context.Context, int64) (int, error) {
	return s.limit, nil
}

func (f fakeProvider) Generate(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
	if f.generateFunc == nil {
		return provider.ImageResponse{}, errors.New("generate not implemented")
	}
	return f.generateFunc(ctx, req)
}

func (f fakeProvider) Edit(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
	if f.editFunc == nil {
		return provider.ImageResponse{}, errors.New("edit not implemented")
	}
	return f.editFunc(ctx, req)
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func withMockRemoteFetch(svc *imagetask.Service) *imagetask.Service {
	data, _ := base64.StdEncoding.DecodeString(tinyPNGBase64)
	svc.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"image/png"}},
				Body:       io.NopCloser(bytes.NewReader(data)),
			}, nil
		}),
	})
	return svc
}

func TestExecuteGenerateMarksMissingRequestedImagesAsPartial(t *testing.T) {
	cfg := taskTestConfig()
	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			if req.Model != "openrouter/vision" {
				t.Fatalf("unexpected provider model %q", req.Model)
			}
			return provider.ImageResponse{Created: 1770000000, Data: []provider.ImageResult{{URL: "https://cdn.example.com/result.png"}}}, nil
		}},
	}

	svc := withMockRemoteFetch(imagetask.NewServiceWithProviders(cfg, providers))
	result, err := svc.Execute(context.Background(), domainimagetask.ExecuteRequest{
		UserID:             7,
		AbstractModel:      "plus",
		TaskType:           string(provider.TaskTypeTextToImage),
		Prompt:             "Generate a poster",
		RequestedSize:      "1536x1024",
		BaseResolution:     "auto",
		OutputImageCount:   2,
		ResponseFormat:     string(provider.ResponseFormatURL),
		PreferredProviders: []string{"openrouter"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Task.Status != domainimagetask.StatusPartialFailed {
		t.Fatalf("expected partial_failed, got %s", result.Task.Status)
	}
	if result.Task.Provider != "openrouter" {
		t.Fatalf("expected openrouter provider, got %s", result.Task.Provider)
	}
	if len(result.Response.Data) != 1 || result.Response.Data[0].URL == "" {
		t.Fatalf("unexpected response %#v", result.Response)
	}

	loaded, err := svc.GetByID(context.Background(), 7, result.Task.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if loaded.ID != result.Task.ID || loaded.Status != domainimagetask.StatusPartialFailed {
		t.Fatalf("unexpected loaded task %#v", loaded)
	}
	if len(loaded.Results) != 1 || loaded.Results[0].URL == "" {
		t.Fatalf("expected persisted results, got %#v", loaded.Results)
	}

	list, err := svc.ListByUser(context.Background(), 7)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(list) != 1 || list[0].ID != result.Task.ID {
		t.Fatalf("unexpected task list %#v", list)
	}
}

func TestExecuteMapsGPTImage2CodexSourceToProviderAutoQualityAndCalculatedSize(t *testing.T) {
	cfg := taskTestConfig()
	captured := make(chan provider.ImageRequest, 1)
	providers := map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			captured <- req
			return provider.ImageResponse{Created: 1770000030, Data: []provider.ImageResult{{B64JSON: tinyPNGBase64}}}, nil
		}},
	}
	svc := imagetask.NewServiceWithProviders(cfg, providers)
	svc.SetModelRoutingSource(&staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		ProviderModels: []modelhub.ProviderCandidate{{
			AccountModelID:          301,
			ModelAccountID:          201,
			Provider:                "openai",
			AdapterType:             "",
			AuthType:                "api_key",
			BaseURL:                 "https://api.example.test/v1",
			Credentials:             map[string]string{"api_key": "test-key"},
			ModelCode:               "gpt-image-2",
			SupportedTaskTypes:      []string{"text_to_image"},
			SupportedBaseResolution: []string{"1k", "2k", "4k"},
			HealthStatus:            "enabled",
			AccountExtra:            map[string]any{"gpt_image_2_codex_source": true},
		}},
	}})

	_, err := svc.Execute(context.Background(), domainimagetask.ExecuteRequest{
		UserID:             77,
		AbstractModel:      "plus",
		TaskType:           string(provider.TaskTypeTextToImage),
		Prompt:             "Generate square 4k image",
		RequestedSize:      "auto",
		BaseResolution:     "4K",
		OutputImageCount:   1,
		ResponseFormat:     string(provider.ResponseFormatB64JSON),
		PreferredProviders: []string{"openai"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	req := <-captured
	if req.Model != "gpt-image-2" {
		t.Fatalf("expected gpt-image-2 request, got %q", req.Model)
	}
	if req.Quality != "auto" {
		t.Fatalf("expected codex source quality auto, got %q", req.Quality)
	}
	if req.Size != "2880x2880" {
		t.Fatalf("expected 4K 1:1 calculated size, got %q", req.Size)
	}
}

func TestTestModelAccountUsesDirectCandidateWithoutBilling(t *testing.T) {
	cfg := taskTestConfig()
	captured := make(chan provider.ImageRequest, 1)
	providers := map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			captured <- req
			return provider.ImageResponse{Created: 1770000040, ProviderRequestID: "req-test-image", Data: []provider.ImageResult{{B64JSON: tinyPNGBase64}}}, nil
		}},
	}
	store := imagetask.NewMemoryStore()
	backend := &modelTestTemporaryURLBackend{objects: map[string][]byte{}}
	svc := imagetask.NewServiceWithProvidersStoreAssetsBillingAndBackend(cfg, providers, store, nil, nil, backend)

	result, err := svc.TestModelAccount(context.Background(), domainimagetask.TestModelAccountRequest{
		AccountID: 201,
		ModelID:   301,
		ModelCode: "gpt-image-2",
		Prompt:    "test account",
	}, modelhub.ProviderCandidate{
		AccountModelID:          301,
		ModelAccountID:          201,
		Provider:                "openai",
		ModelCode:               "gpt-image-2",
		SupportedTaskTypes:      []string{"text_to_image"},
		SupportedBaseResolution: []string{"1k", "2k", "4k"},
		HealthStatus:            "enabled",
		AccountExtra:            map[string]any{"source_mode": "codex_responses"},
	})
	if err != nil {
		t.Fatalf("TestModelAccount: %v", err)
	}
	req := <-captured
	if req.Model != "gpt-image-2" || req.Quality != "auto" || req.Size != "1024x1024" || req.OutputImageCount != 1 {
		t.Fatalf("unexpected provider request %#v", req)
	}
	if result.Status != domainimagetask.StatusSucceeded || result.ProviderRequestID != "req-test-image" {
		t.Fatalf("unexpected test result %#v", result)
	}
	if result.ActualParams["quality"] != "auto" || result.ActualParams["size"] != "1024x1024" {
		t.Fatalf("unexpected actual params %#v", result.ActualParams)
	}
	if result.Image.ID == "" || result.Image.URL == "" || result.Width != 1 || result.Height != 1 {
		t.Fatalf("expected persisted image metadata, got %#v", result)
	}
	if result.ImageURL != backend.previewURL() || result.Image.URL != backend.previewURL() || result.Image.DownloadURL != backend.downloadURL() {
		t.Fatalf("model test result must expose direct temporary URLs, got %#v", result)
	}
	loaded, err := store.GetByID(context.Background(), 0, result.Task.ID)
	if err != nil {
		t.Fatalf("GetByID test task: %v", err)
	}
	if loaded.ActualPoints != "0.00000" || loaded.EstimatedPoints != "0.00000" || loaded.ChargedPoints != "0.00000" {
		t.Fatalf("expected model account test to avoid billing, got %#v", loaded)
	}
}

func TestTestModelAccountUsesBoundedRatioResolution(t *testing.T) {
	captured := make(chan provider.ImageRequest, 1)
	providers := map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			captured <- req
			return provider.ImageResponse{Data: []provider.ImageResult{{B64JSON: tinyPNGBase64}}}, nil
		}},
	}
	svc := imagetask.NewServiceWithProvidersAndStore(taskTestConfig(), providers, imagetask.NewMemoryStore())

	result, err := svc.TestModelAccount(context.Background(), domainimagetask.TestModelAccountRequest{
		ModelCode: "gpt-image-2", SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "1:1",
	}, modelhub.ProviderCandidate{
		AccountModelID: 301, ModelAccountID: 201, Provider: "openai", ModelCode: "gpt-image-2",
		SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"},
		SizeModes: []string{"ratio"}, SupportedAspectRatios: []string{"1:1"}, Quality: []string{"auto"},
		OutputFormat: []string{"png"}, Moderation: []string{"auto"}, MaxImageCount: 1,
		MinWidth: 512, MaxWidth: 900, MinHeight: 512, MaxHeight: 900,
	})
	if err != nil {
		t.Fatalf("TestModelAccount: %v", err)
	}
	providerRequest := <-captured
	if providerRequest.Size != "896x896" {
		t.Fatalf("provider size = %q, want 896x896", providerRequest.Size)
	}
	if result.Task.RequestedSize != "896x896" || result.Task.ResolvedWidth != 896 || result.Task.ResolvedHeight != 896 {
		t.Fatalf("bounded test task = %#v, want 896x896 snapshot", result.Task)
	}
}

type modelTestTemporaryURLBackend struct {
	objects map[string][]byte
}

func (*modelTestTemporaryURLBackend) Driver() string { return "s3" }
func (backend *modelTestTemporaryURLBackend) Put(_ context.Context, key, _ string, content []byte) error {
	backend.objects[key] = append([]byte(nil), content...)
	return nil
}
func (backend *modelTestTemporaryURLBackend) Get(_ context.Context, key string) ([]byte, error) {
	return append([]byte(nil), backend.objects[key]...), nil
}
func (backend *modelTestTemporaryURLBackend) Delete(_ context.Context, key string) error {
	delete(backend.objects, key)
	return nil
}
func (backend *modelTestTemporaryURLBackend) TemporaryGetURL(_ context.Context, _ string, options storage.TemporaryGetURLOptions) (string, error) {
	if options.ResponseFilename != "" {
		return backend.downloadURL(), nil
	}
	return backend.previewURL(), nil
}
func (*modelTestTemporaryURLBackend) previewURL() string {
	return "https://objects.example.test/model-test.png?mode=preview&X-Amz-Signature=test"
}
func (*modelTestTemporaryURLBackend) downloadURL() string {
	return "https://objects.example.test/model-test.png?mode=download&X-Amz-Signature=test"
}

func TestTestModelAccountUsesRequestedPixelSize(t *testing.T) {
	cfg := taskTestConfig()
	captured := make(chan provider.ImageRequest, 1)
	providers := map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			captured <- req
			return provider.ImageResponse{Created: 1770000041, ProviderRequestID: "req-test-pixel", Data: []provider.ImageResult{{B64JSON: tinyPNGBase64}}}, nil
		}},
	}
	svc := imagetask.NewServiceWithProvidersAndStore(cfg, providers, imagetask.NewMemoryStore())

	result, err := svc.TestModelAccount(context.Background(), domainimagetask.TestModelAccountRequest{
		AccountID:     202,
		ModelID:       302,
		ModelCode:     "gpt-image-2",
		Prompt:        "test account pixel",
		SizeMode:      "pixel",
		RequestedSize: "2048x1024",
	}, modelhub.ProviderCandidate{
		AccountModelID:          302,
		ModelAccountID:          202,
		Provider:                "openai",
		ModelCode:               "gpt-image-2",
		SupportedTaskTypes:      []string{"text_to_image"},
		SupportedBaseResolution: []string{"1k", "2k", "4k"},
		SizeModes:               []string{"ratio", "pixel"},
		SupportedPixelSizes:     []string{"2048x1024"},
		MaxReferenceImageCount:  5,
		HealthStatus:            "enabled",
		AccountExtra:            map[string]any{"source_mode": "codex_responses"},
	})
	if err != nil {
		t.Fatalf("TestModelAccount: %v", err)
	}
	req := <-captured
	if req.Size != "2048x1024" || req.Quality != "auto" {
		t.Fatalf("expected requested pixel size and auto base resolution, got %#v", req)
	}
	if result.Task.SizeMode != "pixel" || result.Task.RequestedSize != "2048x1024" || result.Task.BaseResolution != "2k" {
		t.Fatalf("expected pixel mode task metadata, got %#v", result.Task)
	}
	if result.ActualParams["size"] != "2048x1024" || result.ActualParams["quality"] != "auto" {
		t.Fatalf("unexpected actual params %#v", result.ActualParams)
	}
}

func TestTestModelAccountAutoOmitsSizeAndPassesBackground(t *testing.T) {
	captured := make(chan provider.ImageRequest, 1)
	svc := imagetask.NewServiceWithProvidersAndStore(taskTestConfig(), map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(_ context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			captured <- req
			return provider.ImageResponse{Data: []provider.ImageResult{{B64JSON: tinyPNGBase64}}}, nil
		}},
	}, imagetask.NewMemoryStore())

	_, err := svc.TestModelAccount(context.Background(), domainimagetask.TestModelAccountRequest{
		ModelID: 303, ModelCode: "gpt-image-2", Prompt: "transparent test", SizeMode: "auto",
		OutputFormat: "png", Background: "transparent",
	}, modelhub.ProviderCandidate{
		AccountModelID: 303, ModelAccountID: 203, Provider: "openai", ModelCode: "gpt-image-2",
		SupportedTaskTypes: []string{"text_to_image"}, SizeModes: []string{"auto"},
		SupportedBackgrounds: []string{"transparent"}, OutputFormat: []string{"png"}, HealthStatus: "enabled",
	})
	if err != nil {
		t.Fatalf("TestModelAccount: %v", err)
	}
	req := <-captured
	if req.Size != "" {
		t.Fatalf("auto mode provider size = %q, want omitted", req.Size)
	}
	if req.Background != "transparent" || req.OutputFormat != "png" {
		t.Fatalf("provider request lost transparent PNG parameters: %#v", req)
	}
}

func TestTestModelAccountAutoUsesCapabilityDrivenHTTPPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode provider payload: %v", err)
		}
		if _, exists := body["size"]; exists {
			t.Fatalf("auto test payload must omit size: %#v", body)
		}
		if body["background"] != "transparent" || body["output_format"] != "png" {
			t.Fatalf("test payload lost configured background/format: %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"b64_json":"`+tinyPNGBase64+`"}]}`)
	}))
	defer server.Close()

	backend := &modelTestTemporaryURLBackend{objects: map[string][]byte{}}
	svc := imagetask.NewServiceWithProvidersStoreAssetsBillingAndBackend(taskTestConfig(), nil, imagetask.NewMemoryStore(), nil, nil, backend)
	_, err := svc.TestModelAccount(context.Background(), domainimagetask.TestModelAccountRequest{
		ModelID: 304, ModelCode: "gpt-image-2", Prompt: "HTTP payload", SizeMode: "auto",
		OutputFormat: "png", Background: "transparent",
	}, modelhub.ProviderCandidate{
		AccountModelID: 304, ModelAccountID: 204, AdapterType: "openai_compatible", AuthType: "api_key",
		BaseURL: server.URL, Credentials: map[string]string{"api_key": "test-key"}, ModelCode: "gpt-image-2",
		SupportedTaskTypes: []string{"text_to_image"}, SizeModes: []string{"auto"},
		SupportedBackgrounds: []string{"transparent"}, OutputFormat: []string{"png"}, HealthStatus: "enabled",
	})
	if err != nil {
		t.Fatalf("TestModelAccount: %v", err)
	}
}

func TestTestModelAccountRejectsExplicitTransparentJPEG(t *testing.T) {
	called := false
	svc := imagetask.NewServiceWithProvidersAndStore(taskTestConfig(), map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(context.Context, provider.ImageRequest) (provider.ImageResponse, error) {
			called = true
			return provider.ImageResponse{}, nil
		}},
	}, imagetask.NewMemoryStore())
	_, err := svc.TestModelAccount(context.Background(), domainimagetask.TestModelAccountRequest{
		ModelID: 305, ModelCode: "gpt-image-2", Prompt: "invalid format", SizeMode: "auto",
		OutputFormat: "jpeg", Background: "transparent",
	}, modelhub.ProviderCandidate{
		AccountModelID: 305, ModelAccountID: 205, Provider: "openai", ModelCode: "gpt-image-2",
		SupportedTaskTypes: []string{"text_to_image"}, SizeModes: []string{"auto"},
		SupportedBackgrounds: []string{"transparent"}, OutputFormat: []string{"png", "jpeg"}, HealthStatus: "enabled",
	})
	var appErr *errs.Error
	if !errors.As(err, &appErr) || appErr.Code != modelhub.CodeTransparentFormatConflict {
		t.Fatalf("error = %#v, want %s", err, modelhub.CodeTransparentFormatConflict)
	}
	if called {
		t.Fatal("invalid transparent JPEG must be rejected before provider call")
	}
}

func TestExecuteGeneratePersistsDataURLReturnedInURLField(t *testing.T) {
	cfg := taskTestConfig()
	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			return provider.ImageResponse{Created: 1770000020, Data: []provider.ImageResult{{
				URL: "data:image/png;base64," + tinyPNGBase64,
			}}}, nil
		}},
	}
	svc := imagetask.NewServiceWithProviders(cfg, providers)
	svc.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			t.Fatalf("data URL result should not be fetched over HTTP, got %s", req.URL.String())
			return nil, nil
		}),
	})

	result, err := svc.Execute(context.Background(), domainimagetask.ExecuteRequest{
		UserID:             8,
		AbstractModel:      "plus",
		TaskType:           string(provider.TaskTypeTextToImage),
		Prompt:             "Generate an inline response",
		RequestedSize:      "1024x1024",
		BaseResolution:     "auto",
		OutputImageCount:   1,
		ResponseFormat:     string(provider.ResponseFormatURL),
		PreferredProviders: []string{"openrouter"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Task.Status != domainimagetask.StatusSucceeded {
		t.Fatalf("expected succeeded, got %s", result.Task.Status)
	}
	if len(result.Task.Results) != 1 {
		t.Fatalf("expected one persisted result, got %#v", result.Task.Results)
	}
	image := result.Task.Results[0]
	if image.StorageDriver != "local" || image.ObjectKey == "" || image.URL == "" {
		t.Fatalf("expected locally persisted image result, got %#v", image)
	}
	if image.URL == "data:image/png;base64,"+tinyPNGBase64 {
		t.Fatal("expected stored download URL, got original data URL")
	}
}

func TestExecuteFallsBackOnRetryableProviderError(t *testing.T) {
	cfg := taskTestConfig()
	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			return provider.ImageResponse{}, &provider.UpstreamError{
				Provider:   provider.ProviderTypeOpenRouter,
				HTTPStatus: 429,
				Code:       "rate_limit_error",
				Message:    "slow down",
				Action:     provider.UpstreamErrorActionRetry,
				Family:     provider.UpstreamErrorFamilyRateLimited,
			}
		}},
		"openai": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			if req.Model != "gpt-image-1" {
				t.Fatalf("unexpected provider model %q", req.Model)
			}
			return provider.ImageResponse{Created: 1770000001, Data: []provider.ImageResult{{B64JSON: tinyPNGBase64}}}, nil
		}},
	}

	svc := withMockRemoteFetch(imagetask.NewServiceWithProviders(cfg, providers))
	result, err := svc.Execute(context.Background(), domainimagetask.ExecuteRequest{
		UserID:             9,
		AbstractModel:      "plus",
		TaskType:           string(provider.TaskTypeTextToImage),
		Prompt:             "Generate a poster",
		RequestedSize:      "auto",
		BaseResolution:     "4k",
		OutputImageCount:   1,
		ResponseFormat:     string(provider.ResponseFormatB64JSON),
		PreferredProviders: []string{"openrouter", "openai"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Task.Status != domainimagetask.StatusSucceeded {
		t.Fatalf("expected succeeded, got %s", result.Task.Status)
	}
	if result.Task.Provider != "openai" {
		t.Fatalf("expected fallback provider openai, got %s", result.Task.Provider)
	}
	if len(result.Task.Attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(result.Task.Attempts))
	}
	if result.Task.Attempts[0].Provider != "openrouter" || result.Task.Attempts[1].Provider != "openai" {
		t.Fatalf("unexpected attempts %#v", result.Task.Attempts)
	}
	if result.Task.Attempts[0].ErrorCode != "rate_limit_error" || result.Task.Attempts[0].ErrorMessage != "slow down" {
		t.Fatalf("expected structured upstream error on first attempt, got %#v", result.Task.Attempts[0])
	}
	if result.Task.Attempts[0].ErrorDetail["http_status"] != 429 || result.Task.Attempts[0].ErrorDetail["family"] != string(provider.UpstreamErrorFamilyRateLimited) {
		t.Fatalf("expected upstream error detail on first attempt, got %#v", result.Task.Attempts[0].ErrorDetail)
	}
	if result.Task.Attempts[0].StartedAt == nil || result.Task.Attempts[0].FinishedAt == nil {
		t.Fatalf("expected attempt timestamps, got %#v", result.Task.Attempts[0])
	}
	if result.Task.FallbackCount != 1 {
		t.Fatalf("fallback_count = %d, want one provider-candidate fallback", result.Task.FallbackCount)
	}
}

func TestExecuteCountsMissingProviderClientAsCandidateFallback(t *testing.T) {
	cfg := taskTestConfig()
	providers := map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(context.Context, provider.ImageRequest) (provider.ImageResponse, error) {
			return provider.ImageResponse{Data: []provider.ImageResult{{B64JSON: tinyPNGBase64}}}, nil
		}},
	}
	svc := withMockRemoteFetch(imagetask.NewServiceWithProviders(cfg, providers))
	result, err := svc.Execute(context.Background(), domainimagetask.ExecuteRequest{
		UserID: 9, AbstractModel: "plus", TaskType: string(provider.TaskTypeTextToImage), Prompt: "missing first client",
		RequestedSize: "auto", BaseResolution: "1k", OutputImageCount: 1,
		ResponseFormat: string(provider.ResponseFormatB64JSON), PreferredProviders: []string{"openrouter", "openai"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Task.Provider != "openai" || result.Task.FallbackCount != 1 {
		t.Fatalf("task = %#v, want openai with fallback_count=1", result.Task)
	}
}

func TestExecuteFallsBackWhenAllOpenAIFanoutBatchesFailRetryably(t *testing.T) {
	cfg := taskTestConfig()
	openaiCapability := cfg.Routing.ProviderCapabilities["openai"]
	openaiCapability.MaxImageCount = 1
	cfg.Routing.ProviderCapabilities["openai"] = openaiCapability
	var openAICalls atomic.Int64
	providers := map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			openAICalls.Add(1)
			return provider.ImageResponse{}, &provider.UpstreamError{
				Provider: provider.ProviderTypeOpenAI, HTTPStatus: 429, Code: "rate_limit_error", Message: "slow down",
				Action: provider.UpstreamErrorActionRetry, Family: provider.UpstreamErrorFamilyRateLimited,
			}
		}},
		"openrouter": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			results := make([]provider.ImageResult, req.OutputImageCount)
			for i := range results {
				results[i] = provider.ImageResult{B64JSON: tinyPNGBase64}
			}
			return provider.ImageResponse{Created: 1770000002, Data: results}, nil
		}},
	}

	svc := withMockRemoteFetch(imagetask.NewServiceWithProviders(cfg, providers))
	result, err := svc.Execute(context.Background(), domainimagetask.ExecuteRequest{
		UserID: 10, AbstractModel: "plus", TaskType: string(provider.TaskTypeTextToImage), Prompt: "fallback fanout",
		RequestedSize: "auto", BaseResolution: "1k", OutputImageCount: 3,
		ResponseFormat: string(provider.ResponseFormatB64JSON), PreferredProviders: []string{"openai", "openrouter"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if openAICalls.Load() != 3 || result.Task.Provider != "openrouter" || result.Task.Status != domainimagetask.StatusSucceeded {
		t.Fatalf("expected retryable OpenAI fanout failure to use fallback candidate, calls=%d task=%#v", openAICalls.Load(), result.Task)
	}
	if result.Task.FallbackCount != 1 {
		t.Fatalf("fallback_count = %d, want one provider-candidate fallback despite three first-candidate attempts", result.Task.FallbackCount)
	}
}

func TestExecuteRejectsTaskImageCountAboveSafetyLimit(t *testing.T) {
	var providerCalls atomic.Int64
	providers := map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			providerCalls.Add(1)
			return provider.ImageResponse{}, nil
		}},
	}
	svc := imagetask.NewServiceWithProviders(taskTestConfig(), providers)

	_, err := svc.Execute(context.Background(), domainimagetask.ExecuteRequest{
		UserID: 10, AbstractModel: "plus", TaskType: string(provider.TaskTypeTextToImage), Prompt: "oversized",
		BaseResolution: "1k", AspectRatio: "1:1", Quality: "auto", OutputFormat: "png", Moderation: "auto",
		OutputImageCount: modelhub.MaxTaskOutputImageCount + 1,
	})
	if err == nil {
		t.Fatal("expected task safety limit error")
	}
	if providerCalls.Load() != 0 {
		t.Fatalf("provider must not be called for oversized task, calls=%d", providerCalls.Load())
	}
}

func TestExecuteLeasedTaskResumesPartialTerminalization(t *testing.T) {
	store := imagetask.NewMemoryStore()
	now := time.Now().UTC()
	leaseExpiry := now.Add(time.Minute)
	task := domainimagetask.Task{
		UserID: 95, ID: "95959595-9595-4595-8595-959595959595", Status: domainimagetask.StatusRunning,
		ProgressStage: domainimagetask.ProgressStageSettling, LeaseOwner: "worker-partial", LeaseExpiresAt: &leaseExpiry,
		AbstractModel: "plus", TaskType: string(provider.TaskTypeTextToImage), BaseResolution: "1k", OutputImageCount: 2,
		ErrorCode: errs.CodeUpstreamUnavailable, ErrorMessage: "部分上游批次未返回有效图片",
		Results: []provider.ImageResult{{ID: "partial-result", URL: "/images/partial-result.png"}},
	}
	if err := store.Save(context.Background(), task); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	svc := imagetask.NewServiceWithProvidersAndStore(taskTestConfig(), nil, store)

	result, err := svc.ExecuteLeasedTask(context.Background(), task, "worker-partial", nil)
	if err != nil {
		t.Fatalf("ExecuteLeasedTask: %v", err)
	}
	if result.Task.Status != domainimagetask.StatusPartialFailed {
		t.Fatalf("expected recovered partial_failed task, got %#v", result.Task)
	}
}

func TestExecuteUsesRuntimeModelRoutingProviderOrder(t *testing.T) {
	cfg := taskTestConfig()
	calls := []string{}
	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			calls = append(calls, "openrouter")
			return provider.ImageResponse{Created: 1770000003, Data: []provider.ImageResult{{URL: "https://cdn.example.com/runtime-route.png"}}}, nil
		}},
		"openai": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			calls = append(calls, "openai")
			return provider.ImageResponse{Created: 1770000004, Data: []provider.ImageResult{{URL: "https://cdn.example.com/wrong.png"}}}, nil
		}},
	}
	routing := &staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		Providers: []modelhub.ModelProviderConfig{
			{ID: 1, ProviderCode: "openai", ProviderType: "openai", Enabled: true},
			{ID: 2, ProviderCode: "openrouter", ProviderType: "openrouter", Enabled: true},
		},
		Routes: []modelhub.ModelRouteConfig{
			{ID: 1, GroupCode: "plus", TaskType: string(provider.TaskTypeTextToImage), ProviderCode: "openrouter", Priority: 0, FallbackOrder: 0, Enabled: true},
			{ID: 2, GroupCode: "plus", TaskType: string(provider.TaskTypeTextToImage), ProviderCode: "openai", Priority: 9, FallbackOrder: 9, Enabled: true},
		},
	}}

	svc := withMockRemoteFetch(imagetask.NewServiceWithProviders(cfg, providers))
	svc.SetModelRoutingSource(routing)
	result, err := svc.Execute(context.Background(), domainimagetask.ExecuteRequest{
		UserID:             10,
		AbstractModel:      "plus",
		TaskType:           string(provider.TaskTypeTextToImage),
		Prompt:             "Generate with DB route priority",
		RequestedSize:      "auto",
		BaseResolution:     "auto",
		OutputImageCount:   1,
		ResponseFormat:     string(provider.ResponseFormatURL),
		PreferredProviders: []string{"openai"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Task.Provider != "openrouter" {
		t.Fatalf("expected DB route to choose openrouter before preferred provider, got %s calls=%v", result.Task.Provider, calls)
	}
}

func TestExecuteUsesRouteModelPricingForSynchronousTask(t *testing.T) {
	cfg := taskTestConfig()
	routing := &staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		Version:     "route-price-v1",
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		ProviderModels: []modelhub.ProviderCandidate{{
			AccountModelID: 11, Provider: "openrouter", ModelCode: "openrouter/vision",
			SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"}, Quality: []string{"auto"}, HealthStatus: "enabled",
		}},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 11, Priority: 1, Weight: 100, Enabled: true}},
		Prices:     []modelhub.RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", ReferenceMultiplier: "1.00000", Enabled: true}},
	}}
	billingSvc := billingservice.NewService(cfg.Billing)
	billingSvc.SetModelRoutingSource(routing)
	seedBalance(t, billingSvc, 13, "100.00000")
	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(context.Context, provider.ImageRequest) (provider.ImageResponse, error) {
			return provider.ImageResponse{Created: 1770000005, Data: []provider.ImageResult{{URL: "https://cdn.example.com/route-price.png"}}}, nil
		}},
	}
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, providers, imagetask.NewMemoryStore(), nil, billingSvc))
	svc.SetModelRoutingSource(routing)

	result, err := svc.Execute(context.Background(), domainimagetask.ExecuteRequest{
		UserID: 13, UserGroupCode: "basic", UserGroupCodes: []string{"basic"}, UserGroupMultiplier: "1.00000",
		AbstractModel: "plus", RouteModelCode: "plus", TaskType: string(provider.TaskTypeTextToImage),
		Prompt: "Generate with route pricing", SizeMode: "ratio", AspectRatio: "1:1", BaseResolution: "1k", Quality: "auto", OutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Task.EstimatedPoints != "1.00000" || result.Task.ChargedPoints != "1.00000" || result.Task.PricingSnapshot.RouteModelCode != "plus" {
		t.Fatalf("expected synchronous task to keep route-model pricing, got %#v", result.Task)
	}
}

func TestExecuteUsesRuntimeModelRoutingFallbackOrder(t *testing.T) {
	cfg := taskTestConfig()
	calls := []string{}
	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			calls = append(calls, "openrouter")
			return provider.ImageResponse{Created: 1770000013, Data: []provider.ImageResult{{URL: "https://cdn.example.com/runtime-fallback.png"}}}, nil
		}},
		"openai": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			calls = append(calls, "openai")
			return provider.ImageResponse{Created: 1770000014, Data: []provider.ImageResult{{URL: "https://cdn.example.com/wrong-fallback.png"}}}, nil
		}},
	}
	routing := &staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		Providers: []modelhub.ModelProviderConfig{
			{ID: 1, ProviderCode: "openai", ProviderType: "openai", Enabled: true},
			{ID: 2, ProviderCode: "openrouter", ProviderType: "openrouter", Enabled: true},
		},
		Routes: []modelhub.ModelRouteConfig{
			{ID: 1, GroupCode: "plus", TaskType: string(provider.TaskTypeTextToImage), ProviderCode: "openai", Priority: 0, FallbackOrder: 9, Enabled: true},
			{ID: 2, GroupCode: "plus", TaskType: string(provider.TaskTypeTextToImage), ProviderCode: "openrouter", Priority: 0, FallbackOrder: 1, Enabled: true},
		},
	}}

	svc := withMockRemoteFetch(imagetask.NewServiceWithProviders(cfg, providers))
	svc.SetModelRoutingSource(routing)
	result, err := svc.Execute(context.Background(), domainimagetask.ExecuteRequest{
		UserID:             12,
		AbstractModel:      "plus",
		TaskType:           string(provider.TaskTypeTextToImage),
		Prompt:             "Generate with DB fallback order",
		RequestedSize:      "auto",
		BaseResolution:     "auto",
		OutputImageCount:   1,
		ResponseFormat:     string(provider.ResponseFormatURL),
		PreferredProviders: []string{"openai"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Task.Provider != "openrouter" {
		t.Fatalf("expected DB fallback order to choose openrouter before openai, got %s calls=%v", result.Task.Provider, calls)
	}
}

func TestCreateAndExecuteLeasedTaskRespectDisabledRuntimeProvider(t *testing.T) {
	cfg := taskTestConfig()
	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			return provider.ImageResponse{Created: 1770000005, Data: []provider.ImageResult{{URL: "https://cdn.example.com/worker-runtime.png"}}}, nil
		}},
		"openai": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			t.Fatal("disabled provider should not be called")
			return provider.ImageResponse{}, nil
		}},
	}
	routing := &staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		Providers: []modelhub.ModelProviderConfig{
			{ID: 1, ProviderCode: "openai", ProviderType: "openai", Enabled: false},
			{ID: 2, ProviderCode: "openrouter", ProviderType: "openrouter", Enabled: true},
		},
		Routes: []modelhub.ModelRouteConfig{
			{ID: 1, GroupCode: "plus", TaskType: string(provider.TaskTypeTextToImage), ProviderCode: "openai", Priority: 0, FallbackOrder: 0, Enabled: true},
			{ID: 2, GroupCode: "plus", TaskType: string(provider.TaskTypeTextToImage), ProviderCode: "openrouter", Priority: 1, FallbackOrder: 1, Enabled: true},
		},
	}}
	store := imagetask.NewMemoryStore()
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersAndStore(cfg, providers, store))
	svc.SetModelRoutingSource(routing)

	created, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:           11,
		AbstractModel:    "plus",
		TaskType:         string(provider.TaskTypeTextToImage),
		Prompt:           "Worker route",
		RequestedSize:    "auto",
		BaseResolution:   "auto",
		OutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	leased, ok, err := svc.AcquireNextTask(context.Background(), "worker-1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("AcquireNextTask ok=%v err=%v", ok, err)
	}
	result, err := svc.ExecuteLeasedTask(context.Background(), leased, "worker-1", []string{"openai"})
	if err != nil {
		t.Fatalf("ExecuteLeasedTask: %v", err)
	}
	if result.Task.ID != created.ID || result.Task.Provider != "openrouter" {
		t.Fatalf("expected worker execution through openrouter, got %#v", result.Task)
	}
}

func TestCreateRouteTaskAcceptsEnabledCustomRatio(t *testing.T) {
	cfg := taskTestConfig()
	routing := &staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		Version:     "custom-ratio-v1",
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		ProviderModels: []modelhub.ProviderCandidate{{
			AccountModelID: 401, ModelAccountID: 301, Provider: "openai", ModelCode: "gpt-image-2",
			SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"}, SizeModes: []string{"ratio"},
			SupportedAspectRatios: []string{"1:1"}, SupportsCustomRatio: true, Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"},
			MinWidth: 16, MaxWidth: 3840, MinHeight: 16, MaxHeight: 3840, MaxImageCount: 1, HealthStatus: "enabled",
		}},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 401, Enabled: true}},
		Prices:     []modelhub.RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true}},
	}}
	billingSvc := billingservice.NewService(cfg.Billing)
	billingSvc.SetModelRoutingSource(routing)
	seedBalance(t, billingSvc, 91, "10.00000")
	store := imagetask.NewMemoryStore()
	svc := imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, nil, store, nil, billingSvc)
	svc.SetModelRoutingSource(routing)

	created, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID: 91, RouteModelCode: "plus", TaskType: "text_to_image", Prompt: "custom ratio",
		SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "7:5", Quality: "auto", OutputFormat: "png", Moderation: "auto", OutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask custom ratio: %v", err)
	}
	if created.RequestedSize != "1488x1056" || created.ResolvedWidth != 1488 || created.ResolvedHeight != 1056 || created.AspectRatio != "7:5" {
		t.Fatalf("custom ratio task snapshot = %#v", created)
	}
}

func TestVisibleRouteCapabilityEstimateAndCreateAcceptSameRatioRequests(t *testing.T) {
	cfg := taskTestConfig()
	routing := &staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		Version:     "safe-intersection-v1",
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		ProviderModels: []modelhub.ProviderCandidate{
			{
				AccountModelID: 411, Provider: "openai", ModelCode: "gpt-image-2-a", SupportedTaskTypes: []string{"text_to_image"},
				SupportedBaseResolution: []string{"1k"}, SizeModes: []string{"ratio"}, SupportedAspectRatios: []string{"1:1", "16:9"},
				SupportsCustomRatio: true, Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"},
				MinWidth: 16, MaxWidth: 3840, MinHeight: 16, MaxHeight: 3840, MaxImageCount: 1, HealthStatus: "enabled",
			},
			{
				AccountModelID: 412, Provider: "openrouter", ModelCode: "gpt-image-2-b", SupportedTaskTypes: []string{"text_to_image"},
				SupportedBaseResolution: []string{"1k"}, SizeModes: []string{"ratio"}, SupportedAspectRatios: []string{"1:1"},
				Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"},
				MinWidth: 16, MaxWidth: 3840, MinHeight: 16, MaxHeight: 3840, MaxImageCount: 1, HealthStatus: "enabled",
			},
		},
		Candidates: []modelhub.RouteCandidateConfig{
			{RouteModelID: 1, AccountModelID: 411, Priority: 1, Enabled: true},
			{RouteModelID: 1, AccountModelID: 412, Priority: 2, Enabled: true},
		},
		Prices: []modelhub.RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true}},
	}}

	resolver := modelhub.NewResolver(cfg)
	resolver.SetModelRoutingSource(routing)
	visible, err := resolver.ListVisibleRouteModels(context.Background(), nil, nil)
	if err != nil || len(visible) != 1 {
		t.Fatalf("ListVisibleRouteModels() = %#v, %v", visible, err)
	}
	capability := visible[0].CapabilitiesByTaskType["text_to_image"]
	if !reflect.DeepEqual(capability.AspectRatios, []string{"1:1"}) || capability.SupportsCustomRatio {
		t.Fatalf("visible safe intersection = %#v, want only preset 1:1", capability)
	}

	billingSvc := billingservice.NewService(cfg.Billing)
	billingSvc.SetModelRoutingSource(routing)
	seedBalance(t, billingSvc, 93, "10.00000")
	svc := imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, nil, imagetask.NewMemoryStore(), nil, billingSvc)
	svc.SetModelRoutingSource(routing)

	tests := []struct {
		name     string
		ratio    string
		wantCode string
	}{
		{name: "common preset", ratio: "1:1"},
		{name: "preset absent from intersection", ratio: "16:9", wantCode: modelhub.CodeInvalidAspectRatio},
		{name: "custom disabled by intersection", ratio: "7:5", wantCode: modelhub.CodeInvalidAspectRatio},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			estimateReq := domainbilling.EstimateRequest{
				RouteModelCode: "plus", TaskType: "text_to_image", SizeMode: "ratio", BaseResolution: "1k", AspectRatio: tt.ratio,
				Quality: "auto", OutputFormat: "png", Moderation: "auto", RequestedOutputImageCount: 1,
			}
			_, estimateErr := billingSvc.Estimate(estimateReq)
			_, createErr := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
				UserID: 93, RouteModelCode: "plus", TaskType: "text_to_image", Prompt: "intersection contract",
				SizeMode: "ratio", BaseResolution: "1k", AspectRatio: tt.ratio, Quality: "auto", OutputFormat: "png", Moderation: "auto", OutputImageCount: 1,
			})
			if tt.wantCode == "" {
				if estimateErr != nil || createErr != nil {
					t.Fatalf("estimate/create errors = %v / %v, want both accepted", estimateErr, createErr)
				}
				return
			}
			for operation, operationErr := range map[string]error{"estimate": estimateErr, "create": createErr} {
				var appErr *errs.Error
				if !errors.As(operationErr, &appErr) || appErr.StatusCode != 400 || appErr.Code != tt.wantCode {
					t.Fatalf("%s error = %#v, want 400/%s", operation, operationErr, tt.wantCode)
				}
			}
		})
	}
}

func TestExecuteLeasedTaskRejectsForgedRatioSnapshotBeforeProviderCall(t *testing.T) {
	cfg := taskTestConfig()
	var providerCalls atomic.Int32
	providers := map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(context.Context, provider.ImageRequest) (provider.ImageResponse, error) {
			providerCalls.Add(1)
			return provider.ImageResponse{}, nil
		}},
	}
	routing := &staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		ProviderModels: []modelhub.ProviderCandidate{{
			AccountModelID: 402, ModelAccountID: 302, Provider: "openai", ModelCode: "gpt-image-2",
			SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"}, SizeModes: []string{"ratio"},
			SupportedAspectRatios: []string{"1:1"}, Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"},
			MinWidth: 512, MaxWidth: 900, MinHeight: 512, MaxHeight: 900, MaxImageCount: 1, HealthStatus: "enabled",
		}},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 402, Enabled: true}},
		Prices:     []modelhub.RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true}},
	}}
	billingSvc := billingservice.NewService(cfg.Billing)
	billingSvc.SetModelRoutingSource(routing)
	seedBalance(t, billingSvc, 92, "10.00000")
	store := imagetask.NewMemoryStore()
	svc := imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, providers, store, nil, billingSvc)
	svc.SetModelRoutingSource(routing)
	if _, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID: 92, RouteModelCode: "plus", TaskType: "text_to_image", Prompt: "forged snapshot",
		SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "1:1", Quality: "auto", OutputFormat: "png", Moderation: "auto", OutputImageCount: 1,
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	leased, ok, err := svc.AcquireNextTask(context.Background(), "worker-forged", time.Minute)
	if err != nil || !ok {
		t.Fatalf("AcquireNextTask ok=%v err=%v", ok, err)
	}
	leased.RequestedSize = "880x880"
	leased.ResolvedWidth = 880
	leased.ResolvedHeight = 880
	if err := store.SaveIfOwned(context.Background(), leased, "worker-forged", time.Now().UTC()); err != nil {
		t.Fatalf("persist forged snapshot: %v", err)
	}
	_, err = svc.ExecuteLeasedTask(context.Background(), leased, "worker-forged", nil)
	var appErr *errs.Error
	if !errors.As(err, &appErr) || appErr.Code != modelhub.CodeInvalidSizeMode {
		t.Fatalf("ExecuteLeasedTask error = %#v, want %s", err, modelhub.CodeInvalidSizeMode)
	}
	if providerCalls.Load() != 0 {
		t.Fatalf("provider calls = %d, want 0", providerCalls.Load())
	}
}

func TestExecuteLeasedTaskPreservesQueuedRatioSizeAfterCapabilityBoundsChange(t *testing.T) {
	captured := make(chan provider.ImageRequest, 1)
	providers := map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(_ context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			captured <- req
			return provider.ImageResponse{
				Created: 1770000043,
				Data:    []provider.ImageResult{{B64JSON: tinyPNGBase64}},
			}, nil
		}},
	}
	routing := &staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		Version:     "bounds-v1",
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		ProviderModels: []modelhub.ProviderCandidate{{
			AccountModelID: 403, ModelAccountID: 303, Provider: "openai", ModelCode: "gpt-image-2",
			SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"}, SizeModes: []string{"ratio"},
			SupportedAspectRatios: []string{"1:1"}, Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"},
			MinWidth: 512, MaxWidth: 900, MinHeight: 512, MaxHeight: 900, MaxImageCount: 1, HealthStatus: "enabled",
		}},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 403, Enabled: true}},
		Prices:     []modelhub.RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true}},
	}}
	store := imagetask.NewMemoryStore()
	svc := imagetask.NewServiceWithProvidersAndStore(taskTestConfig(), providers, store)
	svc.SetModelRoutingSource(routing)

	created, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID: 95, RouteModelCode: "plus", TaskType: "text_to_image", Prompt: "immutable queued size",
		SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "1:1", Quality: "auto", OutputFormat: "png", Moderation: "auto", OutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if created.RequestedSize != "896x896" || created.ResolvedWidth != 896 || created.ResolvedHeight != 896 {
		t.Fatalf("created snapshot = %#v, want 896x896", created)
	}

	routing.snapshot.Version = "bounds-v2"
	routing.snapshot.ProviderModels[0].MaxWidth = 800
	routing.snapshot.ProviderModels[0].MaxHeight = 800
	leased, ok, err := svc.AcquireNextTask(context.Background(), "worker-immutable-size", time.Minute)
	if err != nil || !ok {
		t.Fatalf("AcquireNextTask ok=%v err=%v", ok, err)
	}
	if _, err := svc.ExecuteLeasedTask(context.Background(), leased, "worker-immutable-size", nil); err != nil {
		t.Fatalf("ExecuteLeasedTask after capability change: %v", err)
	}
	if req := <-captured; req.Size != "896x896" {
		t.Fatalf("provider size = %q, want immutable 896x896", req.Size)
	}
}

func TestExecuteLeasedTaskRejectsForgedSizeSnapshotsBeforeProviderCall(t *testing.T) {
	tests := []struct {
		name        string
		userID      int64
		candidate   modelhub.ProviderCandidate
		create      domainimagetask.CreateRequest
		forgeSize   string
		forgeWidth  int
		forgeHeight int
	}{
		{
			name: "custom ratio", userID: 192,
			candidate: modelhub.ProviderCandidate{SizeModes: []string{"ratio"}, SupportedAspectRatios: []string{"1:1"}, SupportsCustomRatio: true, MinWidth: 512, MaxWidth: 2000, MinHeight: 512, MaxHeight: 2000},
			create:    domainimagetask.CreateRequest{SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "7:5"},
			forgeSize: "1472x1056", forgeWidth: 1472, forgeHeight: 1056,
		},
		{
			name: "auto", userID: 193,
			candidate: modelhub.ProviderCandidate{SizeModes: []string{"auto"}},
			create:    domainimagetask.CreateRequest{SizeMode: "auto"},
			forgeSize: "1024x1024", forgeWidth: 1024, forgeHeight: 1024,
		},
		{
			name: "pixel", userID: 194,
			candidate: modelhub.ProviderCandidate{SizeModes: []string{"pixel"}, SupportedPixelSizes: []string{"1024x1024"}, SupportsCustomSize: true, MinWidth: 512, MaxWidth: 1200, MinHeight: 512, MaxHeight: 1200},
			create:    domainimagetask.CreateRequest{SizeMode: "pixel", RequestedSize: "1024x1024"},
			forgeSize: "896x896", forgeWidth: 896, forgeHeight: 896,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var providerCalls atomic.Int32
			providers := map[string]provider.ImageProvider{"openai": fakeProvider{generateFunc: func(context.Context, provider.ImageRequest) (provider.ImageResponse, error) {
				providerCalls.Add(1)
				return provider.ImageResponse{}, nil
			}}}
			candidate := tt.candidate
			candidate.AccountModelID, candidate.ModelAccountID = 402, 302
			candidate.Provider, candidate.ModelCode = "openai", "gpt-image-2"
			candidate.SupportedTaskTypes = []string{"text_to_image"}
			candidate.SupportedBaseResolution = []string{"1k"}
			candidate.Quality, candidate.OutputFormat, candidate.Moderation = []string{"auto"}, []string{"png"}, []string{"auto"}
			candidate.MaxImageCount, candidate.HealthStatus = 1, "enabled"
			routing := &staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
				RouteModels:    []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
				ProviderModels: []modelhub.ProviderCandidate{candidate},
				Candidates:     []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 402, Enabled: true}},
				Prices:         []modelhub.RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true}},
			}}
			cfg := taskTestConfig()
			billingSvc := billingservice.NewService(cfg.Billing)
			billingSvc.SetModelRoutingSource(routing)
			seedBalance(t, billingSvc, tt.userID, "10.00000")
			store := imagetask.NewMemoryStore()
			svc := imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, providers, store, nil, billingSvc)
			svc.SetModelRoutingSource(routing)
			create := tt.create
			create.UserID, create.RouteModelCode, create.TaskType, create.Prompt = tt.userID, "plus", "text_to_image", "forged snapshot"
			create.Quality, create.OutputFormat, create.Moderation, create.OutputImageCount = "auto", "png", "auto", 1
			if _, err := svc.CreateTask(context.Background(), create); err != nil {
				t.Fatalf("CreateTask: %v", err)
			}
			leased, ok, err := svc.AcquireNextTask(context.Background(), "worker-forged", time.Minute)
			if err != nil || !ok {
				t.Fatalf("AcquireNextTask ok=%v err=%v", ok, err)
			}
			leased.RequestedSize, leased.ResolvedWidth, leased.ResolvedHeight = tt.forgeSize, tt.forgeWidth, tt.forgeHeight
			if err := store.SaveIfOwned(context.Background(), leased, "worker-forged", time.Now().UTC()); err != nil {
				t.Fatalf("persist forged snapshot: %v", err)
			}
			if _, err := svc.ExecuteLeasedTask(context.Background(), leased, "worker-forged", nil); err == nil {
				t.Fatal("forged size snapshot was accepted")
			}
			if providerCalls.Load() != 0 {
				t.Fatalf("provider calls = %d, want 0", providerCalls.Load())
			}
		})
	}
}

func TestExecuteLeasedTaskRejectsForgedStaticRatioSnapshotBeforeProviderCall(t *testing.T) {
	cfg := taskTestConfig()
	var providerCalls atomic.Int32
	providers := map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(context.Context, provider.ImageRequest) (provider.ImageResponse, error) {
			providerCalls.Add(1)
			return provider.ImageResponse{}, nil
		}},
	}
	store := imagetask.NewMemoryStore()
	svc := imagetask.NewServiceWithProvidersAndStore(cfg, providers, store)
	created, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID: 195, AbstractModel: "plus", TaskType: "text_to_image", Prompt: "static forged snapshot",
		SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "1:1", Quality: "auto", OutputFormat: "png", Moderation: "auto", OutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if created.RequestedSize != "1024x1024" || created.ResolvedWidth != 1024 || created.ResolvedHeight != 1024 {
		t.Fatalf("created static ratio snapshot = %#v, want 1024x1024", created)
	}
	leased, ok, err := svc.AcquireNextTask(context.Background(), "worker-static-forged", time.Minute)
	if err != nil || !ok {
		t.Fatalf("AcquireNextTask ok=%v err=%v", ok, err)
	}
	leased.RequestedSize, leased.ResolvedWidth, leased.ResolvedHeight = "1008x1008", 1008, 1008
	if err := store.SaveIfOwned(context.Background(), leased, "worker-static-forged", time.Now().UTC()); err != nil {
		t.Fatalf("persist forged snapshot: %v", err)
	}
	_, err = svc.ExecuteLeasedTask(context.Background(), leased, "worker-static-forged", []string{"openai"})
	var appErr *errs.Error
	if !errors.As(err, &appErr) || appErr.Code != modelhub.CodeInvalidSizeMode {
		t.Fatalf("ExecuteLeasedTask error = %#v, want %s", err, modelhub.CodeInvalidSizeMode)
	}
	if providerCalls.Load() != 0 {
		t.Fatalf("provider calls = %d, want 0", providerCalls.Load())
	}
}

func TestExecuteLeasedTaskCalculatesGPTImage2CodexSizeFromQualityAndAspectRatio(t *testing.T) {
	cfg := taskTestConfig()
	captured := make(chan provider.ImageRequest, 1)
	providers := map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			captured <- req
			return provider.ImageResponse{Created: 1770000031, Data: []provider.ImageResult{{B64JSON: tinyPNGBase64}}}, nil
		}},
	}
	store := imagetask.NewMemoryStore()
	svc := imagetask.NewServiceWithProvidersAndStore(cfg, providers, store)
	svc.SetModelRoutingSource(&staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		ProviderModels: []modelhub.ProviderCandidate{{
			AccountModelID:          302,
			ModelAccountID:          202,
			Provider:                "openai",
			AdapterType:             "",
			AuthType:                "api_key",
			BaseURL:                 "https://api.example.test/v1",
			Credentials:             map[string]string{"api_key": "test-key"},
			ModelCode:               "gpt-image-2",
			SupportedTaskTypes:      []string{"text_to_image"},
			SupportedBaseResolution: []string{"1k", "2k", "4k"},
			HealthStatus:            "enabled",
			AccountExtra:            map[string]any{"source_mode": "codex_responses"},
		}},
	}})

	created, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:           78,
		AbstractModel:    "plus",
		TaskType:         string(provider.TaskTypeTextToImage),
		Prompt:           "Generate a wide 4k image",
		RequestedSize:    "auto",
		BaseResolution:   "4K",
		AspectRatio:      "16:9",
		OutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	leased, ok, err := svc.AcquireNextTask(context.Background(), "worker-1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("AcquireNextTask ok=%v err=%v", ok, err)
	}
	result, err := svc.ExecuteLeasedTask(context.Background(), leased, "worker-1", []string{"openai"})
	if err != nil {
		t.Fatalf("ExecuteLeasedTask: %v", err)
	}
	if result.Task.ID != created.ID || result.Task.Status != domainimagetask.StatusSucceeded {
		t.Fatalf("unexpected execution result %#v", result.Task)
	}
	req := <-captured
	if req.Model != "gpt-image-2" || req.Quality != "auto" || req.Size != "3840x2160" || req.ResponseFormat != provider.ResponseFormatB64JSON {
		t.Fatalf("expected gpt-image-2 codex request quality auto and 4K 16:9 size, got %#v", req)
	}
}

func TestExecuteLeasedAutoTaskOmitsSizeAndPersistsDiagnostics(t *testing.T) {
	captured := make(chan provider.ImageRequest, 1)
	providers := map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(_ context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			captured <- req
			return provider.ImageResponse{
				Created: 1770000042, ProviderRequestID: "req-auto-size",
				Data: []provider.ImageResult{{B64JSON: tinyPNGBase64, Width: 1672, Height: 941}},
			}, nil
		}},
	}
	store := imagetask.NewMemoryStore()
	svc := imagetask.NewServiceWithProvidersAndStore(taskTestConfig(), providers, store)
	svc.SetModelRoutingSource(&staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices:      []modelhub.RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "5.00000", Enabled: true}},
		ProviderModels: []modelhub.ProviderCandidate{{
			AccountModelID: 306, ModelAccountID: 206, Provider: "openai", ModelCode: "gpt-image-2",
			SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"}, SizeModes: []string{"auto"},
			Quality: []string{"auto"}, OutputFormat: []string{"png"}, SupportedBackgrounds: []string{"auto"}, Moderation: []string{"auto"},
			MaxImageCount: 1, HealthStatus: "enabled",
		}},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 306, Enabled: true}},
	}})

	created, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID: 82, RouteModelCode: "plus", TaskType: "text_to_image", Prompt: "provider-selected dimensions",
		SizeMode: "auto", Quality: "auto", OutputFormat: "png", Background: "auto", Moderation: "auto", OutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if created.RequestedSize != "" || created.ResolvedWidth != 0 || created.ResolvedHeight != 0 {
		t.Fatalf("auto task must persist absent dimensions, got %#v", created)
	}
	leased, ok, err := svc.AcquireNextTask(context.Background(), "worker-auto", time.Minute)
	if err != nil || !ok {
		t.Fatalf("AcquireNextTask ok=%v err=%v", ok, err)
	}
	result, err := svc.ExecuteLeasedTask(context.Background(), leased, "worker-auto", []string{"openai"})
	if err != nil {
		t.Fatalf("ExecuteLeasedTask: %v", err)
	}
	if req := <-captured; req.Size != "" || req.Background != "auto" {
		t.Fatalf("unexpected auto provider request %#v", req)
	}
	if len(result.Task.Attempts) != 1 {
		t.Fatalf("expected one persisted attempt, got %#v", result.Task.Attempts)
	}
	attempt := result.Task.Attempts[0]
	if attempt.SourceSizeMode != "auto" || attempt.OutboundSize != "" || attempt.ReturnedWidth != 1 || attempt.ReturnedHeight != 1 || attempt.ProviderRequestID != "req-auto-size" || attempt.SizeDiagnostic != "missing_outbound_size" {
		t.Fatalf("unexpected size diagnostics %#v", attempt)
	}
	loaded, err := svc.GetByID(context.Background(), 82, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if len(loaded.Attempts) != 1 || !reflect.DeepEqual(loaded.Attempts[0], attempt) {
		t.Fatalf("diagnostic attempt was not persisted: %#v", loaded.Attempts)
	}
}

func TestExecuteLeasedTaskRequestsB64ForGPTImage2CodexMultiOutput(t *testing.T) {
	cfg := taskTestConfig()
	captured := make(chan provider.ImageRequest, 3)
	providers := map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			captured <- req
			return provider.ImageResponse{Created: 1770000034, Data: []provider.ImageResult{{B64JSON: tinyPNGBase64}, {B64JSON: tinyPNGBase64}, {B64JSON: tinyPNGBase64}}}, nil
		}},
	}
	store := imagetask.NewMemoryStore()
	svc := imagetask.NewServiceWithProvidersAndStore(cfg, providers, store)
	svc.SetModelRoutingSource(&staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		ProviderModels: []modelhub.ProviderCandidate{{
			AccountModelID:          305,
			ModelAccountID:          205,
			Provider:                "openai",
			ModelCode:               "gpt-image-2",
			SupportedTaskTypes:      []string{"text_to_image"},
			SupportedBaseResolution: []string{"1k", "2k", "4k"},
			MaxImageCount:           3,
			HealthStatus:            "enabled",
			AccountExtra:            map[string]any{"source_mode": "codex_responses"},
		}},
	}})

	created, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:           81,
		AbstractModel:    "plus",
		TaskType:         string(provider.TaskTypeTextToImage),
		Prompt:           "Generate a three image set",
		RequestedSize:    "auto",
		BaseResolution:   "1K",
		AspectRatio:      "1:1",
		OutputImageCount: 3,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	leased, ok, err := svc.AcquireNextTask(context.Background(), "worker-1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("AcquireNextTask ok=%v err=%v", ok, err)
	}
	result, err := svc.ExecuteLeasedTask(context.Background(), leased, "worker-1", []string{"openai"})
	if err != nil {
		t.Fatalf("ExecuteLeasedTask: %v", err)
	}
	if result.Task.ID != created.ID || result.Task.Status != domainimagetask.StatusSucceeded {
		t.Fatalf("unexpected execution result %#v", result.Task)
	}
	if len(result.Task.Results) != 3 {
		t.Fatalf("expected 3 persisted results, got %d", len(result.Task.Results))
	}
	req := <-captured
	if req.ResponseFormat != provider.ResponseFormatB64JSON {
		t.Fatalf("multi-output request should use b64_json, got %#v", req)
	}
	if req.OutputImageCount != 3 {
		t.Fatalf("multi-output request should request three images, got %#v", req)
	}
}

func TestExecutePreservesExplicitSizeForGPTImage2CodexSource(t *testing.T) {
	cfg := taskTestConfig()
	captured := make(chan provider.ImageRequest, 1)
	providers := map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			captured <- req
			return provider.ImageResponse{Created: 1770000032, Data: []provider.ImageResult{{B64JSON: tinyPNGBase64}}}, nil
		}},
	}
	svc := imagetask.NewServiceWithProviders(cfg, providers)
	svc.SetModelRoutingSource(&staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		ProviderModels: []modelhub.ProviderCandidate{{
			AccountModelID:          303,
			ModelAccountID:          203,
			Provider:                "openai",
			AuthType:                "api_key",
			BaseURL:                 "https://api.example.test/v1",
			Credentials:             map[string]string{"api_key": "test-key"},
			ModelCode:               "gpt-image-2",
			SupportedTaskTypes:      []string{"text_to_image"},
			SupportedBaseResolution: []string{"1k", "2k", "4k"},
			HealthStatus:            "enabled",
			AccountExtra:            map[string]any{"source_mode": "codex_responses"},
		}},
	}})

	_, err := svc.Execute(context.Background(), domainimagetask.ExecuteRequest{
		UserID:             79,
		AbstractModel:      "plus",
		TaskType:           string(provider.TaskTypeTextToImage),
		Prompt:             "Generate a wide 4k image",
		RequestedSize:      "3840x2160",
		BaseResolution:     "4K",
		OutputImageCount:   1,
		ResponseFormat:     string(provider.ResponseFormatB64JSON),
		PreferredProviders: []string{"openai"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	req := <-captured
	if req.Model != "gpt-image-2" || req.Quality != "auto" || req.Size != "3840x2160" {
		t.Fatalf("expected codex source to preserve explicit size, got %#v", req)
	}
}

func TestExecutePassesGPTImage2ImagesSourceQualityFromRequest(t *testing.T) {
	cfg := taskTestConfig()
	captured := make(chan provider.ImageRequest, 1)
	providers := map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			captured <- req
			return provider.ImageResponse{Created: 1770000033, Data: []provider.ImageResult{{B64JSON: tinyPNGBase64}}}, nil
		}},
	}
	svc := imagetask.NewServiceWithProviders(cfg, providers)
	svc.SetModelRoutingSource(&staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		ProviderModels: []modelhub.ProviderCandidate{{
			AccountModelID:          304,
			ModelAccountID:          204,
			Provider:                "openai",
			AuthType:                "api_key",
			BaseURL:                 "https://api.example.test/v1",
			Credentials:             map[string]string{"api_key": "test-key"},
			ModelCode:               "gpt-image-2",
			SupportedTaskTypes:      []string{"text_to_image"},
			SupportedBaseResolution: []string{"1k", "2k", "4k"},
			Quality:                 []string{"auto", "low", "medium", "high"},
			HealthStatus:            "enabled",
			AccountExtra:            map[string]any{"source_mode": "images"},
		}},
	}})

	_, err := svc.Execute(context.Background(), domainimagetask.ExecuteRequest{
		UserID:             80,
		AbstractModel:      "plus",
		TaskType:           string(provider.TaskTypeTextToImage),
		Prompt:             "Generate a wide 4k image",
		RequestedSize:      "3840x2160",
		BaseResolution:     "4K",
		Quality:            "high",
		OutputImageCount:   1,
		ResponseFormat:     string(provider.ResponseFormatB64JSON),
		PreferredProviders: []string{"openai"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	req := <-captured
	if req.Model != "gpt-image-2" || req.Quality != "high" || req.Size != "3840x2160" {
		t.Fatalf("expected images source to pass true quality field to upstream, got %#v", req)
	}
}

type staticModelRoutingSource struct {
	snapshot modelhub.ModelRoutingSnapshot
}

func (s *staticModelRoutingSource) ModelRoutingConfig(ctx context.Context) (modelhub.ModelRoutingSnapshot, error) {
	return s.snapshot, nil
}

type rotatingModelRoutingSource struct {
	snapshots []modelhub.ModelRoutingSnapshot
	reads     int
}

func (s *rotatingModelRoutingSource) ModelRoutingConfig(context.Context) (modelhub.ModelRoutingSnapshot, error) {
	index := s.reads
	s.reads++
	if index >= len(s.snapshots) {
		index = len(s.snapshots) - 1
	}
	return s.snapshots[index], nil
}

const tinyPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAFgwJ/lqR5DQAAAABJRU5ErkJggg=="

func TestDownloadImageResultRejectsLocalObjectKeyTraversal(t *testing.T) {
	root := t.TempDir()
	cfg := taskTestConfig()
	cfg.Storage.LocalRoot = root

	outsidePath := filepath.Join(filepath.Dir(root), "secret-image.txt")
	if err := os.WriteFile(outsidePath, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(outsidePath)
	})

	store := imagetask.NewMemoryStore()
	task := domainimagetask.Task{
		UserID:        22,
		ID:            "99999999-9999-9999-9999-999999999999",
		Status:        domainimagetask.StatusSucceeded,
		AbstractModel: "plus",
		TaskType:      string(provider.TaskTypeTextToImage),

		BaseResolution:   "2k",
		OutputImageCount: 1,
		Results: []provider.ImageResult{{
			ID:               "bad-image",
			StorageDriver:    "local",
			ObjectKey:        "../secret-image.txt",
			MimeType:         "image/png",
			VisibilityStatus: "private",
		}},
	}
	if err := store.Save(context.Background(), task); err != nil {
		t.Fatalf("Save: %v", err)
	}

	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersAndStore(cfg, nil, store))
	if _, _, err := svc.DownloadImageResult(context.Background(), 22, "bad-image"); err == nil {
		t.Fatal("expected path traversal object key to be rejected")
	}
}

func TestDeliverImageResultUsesTemporaryURLAfterOwnershipCheck(t *testing.T) {
	store := imagetask.NewMemoryStore()
	result := provider.ImageResult{
		ID: "signed-image", StorageDriver: "s3", StorageConfigID: "bfss-primary",
		ObjectKey: "generated/signed-image.png", MimeType: "image/png", VisibilityStatus: "private",
	}
	if err := store.Save(t.Context(), domainimagetask.Task{
		UserID: 22, ID: "signed-task", Status: domainimagetask.StatusSucceeded, Results: []provider.ImageResult{result},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	backend := &temporaryURLBackend{signedURL: "https://bfss.example.com/bucket/generated/signed-image.png?X-Amz-Signature=signed"}
	svc := imagetask.NewServiceWithProvidersStoreAssetsBillingAndBackend(taskTestConfig(), nil, store, nil, nil, backend)

	delivery, err := svc.DeliverImageResult(t.Context(), 22, result.ID)
	if err != nil {
		t.Fatalf("DeliverImageResult: %v", err)
	}
	if delivery.Result.ID != result.ID || delivery.TemporaryURL != backend.signedURL || len(delivery.Content) != 0 {
		t.Fatalf("unexpected temporary delivery %#v", delivery)
	}
	if backend.getCalls != 0 || backend.signCalls != 2 || backend.objectKey != result.ObjectKey {
		t.Fatalf("temporary delivery calls: get=%d sign=%d key=%q", backend.getCalls, backend.signCalls, backend.objectKey)
	}
	if backend.options.Expiry != 5*time.Minute || backend.options.ContentType != "image/png" || backend.options.ResponseFilename != "signed-image.png" {
		t.Fatalf("unexpected temporary URL options %#v", backend.options)
	}

	if _, err := svc.DeliverImageResult(t.Context(), 23, result.ID); err == nil {
		t.Fatal("expected non-owner delivery to be rejected")
	}
	if backend.signCalls != 2 {
		t.Fatalf("non-owner request reached signer; sign calls=%d", backend.signCalls)
	}
}

func TestTemporaryMediaURLProjectionForTaskResults(t *testing.T) {
	store := imagetask.NewMemoryStore()
	result := provider.ImageResult{
		ID: "projected-image", StorageDriver: "s3", StorageConfigID: "bfss-primary",
		ObjectKey: "generated/projected-image.png", MimeType: "image/png", VisibilityStatus: "private",
	}
	if err := store.Save(t.Context(), domainimagetask.Task{
		UserID: 22, ID: "projected-task", Status: domainimagetask.StatusSucceeded, Results: []provider.ImageResult{result},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	backend := &temporaryURLBackend{
		previewURL:  "https://bfss.example.com/generated/projected-image.png?mode=preview&X-Amz-Signature=preview&X-Amz-Date=20260806T120000Z&X-Amz-Expires=360",
		downloadURL: "https://bfss.example.com/generated/projected-image.png?mode=download&X-Amz-Signature=download&X-Amz-Date=20260806T120000Z&X-Amz-Expires=300",
	}
	svc := imagetask.NewServiceWithProvidersStoreAssetsBillingAndBackend(taskTestConfig(), nil, store, nil, nil, backend)

	task, err := svc.GetByID(t.Context(), 22, "projected-task")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if len(task.Results) != 1 || task.Results[0].URL != backend.previewURL || task.Results[0].DownloadURL != backend.downloadURL {
		t.Fatalf("task result must expose separate temporary URLs, got %#v", task.Results)
	}
	if task.Results[0].PreviewExpiresAt == nil || task.Results[0].DownloadExpiresAt == nil ||
		!task.Results[0].PreviewExpiresAt.Equal(time.Date(2026, time.August, 6, 12, 6, 0, 0, time.UTC)) ||
		!task.Results[0].DownloadExpiresAt.Equal(time.Date(2026, time.August, 6, 12, 5, 0, 0, time.UTC)) {
		t.Fatalf("task result must expose URL expiry metadata, got %#v", task.Results[0])
	}
	if backend.signCalls != 2 || backend.options.Expiry != 5*time.Minute || backend.options.ResponseFilename != "projected-image.png" {
		t.Fatalf("unexpected signer calls=%d options=%#v", backend.signCalls, backend.options)
	}
	if _, err := svc.GetByID(t.Context(), 23, "projected-task"); err == nil {
		t.Fatal("non-owner must be rejected before media signing")
	}
	if backend.signCalls != 2 {
		t.Fatalf("non-owner request reached signer; sign calls=%d", backend.signCalls)
	}
}

func TestMediaProjectionUsesLocalFallbackAndSurfacesSigningFailure(t *testing.T) {
	localStore := imagetask.NewMemoryStore()
	localResult := provider.ImageResult{ID: "local-projection", StorageDriver: "local", ObjectKey: "generated/local.png", MimeType: "image/png"}
	if err := localStore.Save(t.Context(), domainimagetask.Task{UserID: 22, ID: "local-projection-task", Status: domainimagetask.StatusSucceeded, Results: []provider.ImageResult{localResult}}); err != nil {
		t.Fatalf("Save local: %v", err)
	}
	localService := imagetask.NewServiceWithProvidersStoreAssetsBillingAndBackend(taskTestConfig(), nil, localStore, nil, nil, storage.NewLocalBackend(t.TempDir()))
	localTask, err := localService.GetByID(t.Context(), 22, "local-projection-task")
	if err != nil {
		t.Fatalf("GetByID local: %v", err)
	}
	wantFallback := "/api/agent/image/v1/images/local-projection"
	if localTask.Results[0].URL != wantFallback || localTask.Results[0].DownloadURL != wantFallback {
		t.Fatalf("local media must use authenticated fallback route, got %#v", localTask.Results[0])
	}

	signingStore := imagetask.NewMemoryStore()
	signedResult := provider.ImageResult{ID: "broken-signing", StorageDriver: "s3", ObjectKey: "generated/broken.png", MimeType: "image/png"}
	if err := signingStore.Save(t.Context(), domainimagetask.Task{UserID: 22, ID: "broken-signing-task", Status: domainimagetask.StatusSucceeded, Results: []provider.ImageResult{signedResult}}); err != nil {
		t.Fatalf("Save signing: %v", err)
	}
	signingService := imagetask.NewServiceWithProvidersStoreAssetsBillingAndBackend(taskTestConfig(), nil, signingStore, nil, nil, &temporaryURLBackend{signErr: errors.New("signing unavailable")})
	_, err = signingService.GetByID(t.Context(), 22, "broken-signing-task")
	appErr, ok := err.(*errs.Error)
	if !ok || appErr.Code != "STORAGE_CONFIG_UNAVAILABLE" {
		t.Fatalf("signing failure must surface stable storage error, got %T %v", err, err)
	}
}

func TestCancelPublishPendingAndApprovedAllowsReapply(t *testing.T) {
	store := imagetask.NewMemoryStore()
	const userID int64 = 71
	const imageID = "cancel-publish-image"
	if err := store.Save(t.Context(), domainimagetask.Task{
		UserID: userID, ID: "cancel-publish-task", Status: domainimagetask.StatusSucceeded,
		Prompt: "cancel publish", Results: []provider.ImageResult{{
			ID: imageID, URL: "https://example.test/cancel.png", VisibilityStatus: domainimagetask.VisibilityPrivate,
		}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	svc := imagetask.NewServiceWithStore(taskTestConfig(), store)

	pending, err := svc.RequestPublish(t.Context(), userID, imageID)
	if err != nil {
		t.Fatalf("RequestPublish pending: %v", err)
	}
	if pending.VisibilityStatus != domainimagetask.VisibilityPendingReview {
		t.Fatalf("expected pending review, got %#v", pending)
	}
	canceled, err := svc.CancelPublish(t.Context(), userID, imageID)
	if err != nil {
		t.Fatalf("CancelPublish pending: %v", err)
	}
	if canceled.VisibilityStatus != domainimagetask.VisibilityPrivate || canceled.ReviewReason != "" || canceled.PublishedAt != nil {
		t.Fatalf("pending cancellation must clear publication metadata: %#v", canceled)
	}

	reviewPage, err := svc.ListGallery(t.Context(), domainimagetask.GalleryListRequest{Page: 1, PageSize: 10, ReviewOnly: true})
	if err != nil {
		t.Fatalf("ListGallery review: %v", err)
	}
	if reviewPage.Total != 0 {
		t.Fatalf("canceled image must leave review query: %#v", reviewPage)
	}

	if _, err := svc.RequestPublish(t.Context(), userID, imageID); err != nil {
		t.Fatalf("RequestPublish again: %v", err)
	}
	publishedAt := time.Now().UTC()
	if _, err := svc.ReviewImage(t.Context(), imageID, domainimagetask.VisibilityApproved, "", &publishedAt); err != nil {
		t.Fatalf("ReviewImage approve: %v", err)
	}
	publicPage, err := svc.ListPublicGallery(t.Context(), domainimagetask.GalleryListRequest{Page: 1, PageSize: 10})
	if err != nil || publicPage.Total != 1 {
		t.Fatalf("approved image must enter public query: page=%#v err=%v", publicPage, err)
	}
	canceled, err = svc.CancelPublish(t.Context(), userID, imageID)
	if err != nil {
		t.Fatalf("CancelPublish approved: %v", err)
	}
	if canceled.VisibilityStatus != domainimagetask.VisibilityPrivate || canceled.PublishedAt != nil {
		t.Fatalf("approved cancellation must return private: %#v", canceled)
	}
	publicPage, err = svc.ListPublicGallery(t.Context(), domainimagetask.GalleryListRequest{Page: 1, PageSize: 10})
	if err != nil || publicPage.Total != 0 {
		t.Fatalf("canceled image must leave public query: page=%#v err=%v", publicPage, err)
	}
	if _, err := svc.CancelPublish(t.Context(), userID, imageID); err != nil {
		t.Fatalf("CancelPublish private idempotently: %v", err)
	}
	if _, err := svc.CancelPublish(t.Context(), userID+1, imageID); err == nil {
		t.Fatal("expected non-owner cancellation to fail")
	}
	reapplied, err := svc.RequestPublish(t.Context(), userID, imageID)
	if err != nil || reapplied.VisibilityStatus != domainimagetask.VisibilityPendingReview {
		t.Fatalf("canceled image must allow reapply: image=%#v err=%v", reapplied, err)
	}
}

func TestDeliverImageResultFallsBackToBackendBytes(t *testing.T) {
	backend := storage.NewLocalBackend(t.TempDir())
	content := []byte("local-image-content")
	if err := backend.Put(t.Context(), "generated/local-image.webp", "image/webp", content); err != nil {
		t.Fatalf("Put: %v", err)
	}
	store := imagetask.NewMemoryStore()
	result := provider.ImageResult{
		ID: "local-image", StorageDriver: "local", ObjectKey: "generated/local-image.webp",
		MimeType: "image/webp", VisibilityStatus: "private",
	}
	if err := store.Save(t.Context(), domainimagetask.Task{
		UserID: 22, ID: "local-task", Status: domainimagetask.StatusSucceeded, Results: []provider.ImageResult{result},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	svc := imagetask.NewServiceWithProvidersStoreAssetsBillingAndBackend(taskTestConfig(), nil, store, nil, nil, backend)

	delivery, err := svc.DeliverImageResult(t.Context(), 22, result.ID)
	if err != nil {
		t.Fatalf("DeliverImageResult: %v", err)
	}
	if delivery.TemporaryURL != "" || !bytes.Equal(delivery.Content, content) {
		t.Fatalf("unexpected local delivery %#v", delivery)
	}
}

type temporaryURLBackend struct {
	signedURL   string
	previewURL  string
	downloadURL string
	objectKey   string
	options     storage.TemporaryGetURLOptions
	getCalls    int
	signCalls   int
	signErr     error
}

func (backend *temporaryURLBackend) Driver() string { return "s3" }
func (backend *temporaryURLBackend) Put(context.Context, string, string, []byte) error {
	return nil
}
func (backend *temporaryURLBackend) Get(context.Context, string) ([]byte, error) {
	backend.getCalls++
	return nil, errors.New("Get must not be called for a signing backend")
}
func (backend *temporaryURLBackend) Delete(context.Context, string) error { return nil }
func (backend *temporaryURLBackend) TemporaryGetURL(_ context.Context, objectKey string, options storage.TemporaryGetURLOptions) (string, error) {
	backend.signCalls++
	backend.objectKey = objectKey
	backend.options = options
	if backend.signErr != nil {
		return "", backend.signErr
	}
	if options.ResponseFilename != "" && backend.downloadURL != "" {
		return backend.downloadURL, nil
	}
	if options.ResponseFilename == "" && backend.previewURL != "" {
		return backend.previewURL, nil
	}
	return backend.signedURL, nil
}

func taskTestConfig() config.Config {
	cfg := config.Config{}
	cfg.Billing.CNYPerPoint = "0.31250"
	cfg.Billing.PointsScale = 5
	cfg.Billing.AutoBaseResolutionDefaultByGroup = map[string]string{"plus": "2k"}
	cfg.Billing.BaseResolutionPointsByModel = map[string]map[string]string{
		"plus": {"1k": "5.00000", "2k": "8.00000", "4k": "16.00000"},
	}
	cfg.Billing.UserGroupMultipliers = map[string]string{"basic": "1.00000", "plus": "1.00000"}
	cfg.Billing.TaskMultipliers = map[string]string{"text_to_image": "1.00000", "image_edit": "1.25000"}
	cfg.Billing.ReferenceImageExtra = config.ReferenceExtra{First: "0.10000", Additional: "0.05000"}
	cfg.GenerationLimits.MaxImageCount = 5
	cfg.GenerationLimits.ReferenceImageMaxCount = 4
	cfg.Providers.OpenRouter.Enabled = true
	cfg.Providers.OpenAI.Enabled = true
	cfg.Routing.ProviderCapabilities = map[string]config.ProviderCapabilityConfig{
		"openrouter": {
			SupportedModels:         []string{"plus"},
			SupportedTaskTypes:      []string{"text_to_image", "image_edit"},
			SupportedBaseResolution: []string{"1k", "2k", "4k"},
			SupportedAspectRatios:   []string{"1:1", "4:3", "16:9"},
			MaxImageCount:           5,
			MaxReferenceImageCount:  4,
			SupportsImageInput:      true,
			SupportsMask:            false,
			Priority:                1,
		},
		"openai": {
			SupportedModels:         []string{"plus"},
			SupportedTaskTypes:      []string{"text_to_image", "image_edit"},
			SupportedBaseResolution: []string{"1k", "2k", "4k"},
			SupportedAspectRatios:   []string{"1:1", "4:3", "16:9"},
			MaxImageCount:           5,
			MaxReferenceImageCount:  4,
			SupportsImageInput:      true,
			SupportsMask:            true,
			Priority:                2,
		},
	}
	cfg.Routing.ProviderModelMap = map[string]map[string]string{
		"plus": {"openrouter": "openrouter/vision", "openai": "gpt-image-1"},
	}
	return cfg
}

type failingSaveStore struct {
	base                 *imagetask.MemoryStore
	failSave             bool
	failSaveIfOwned      bool
	failSaveIfOwnedError error
	failAcquireError     error
	ownedSnapshots       []domainimagetask.Task
	progressMu           sync.Mutex
	progressStages       []string
}

func (s *failingSaveStore) Save(ctx context.Context, task domainimagetask.Task) error {
	if s.failSave {
		s.failSave = false
		return errors.New("save failed")
	}
	if err := s.base.Save(ctx, task); err != nil {
		return err
	}
	s.recordProgressStage(task.ProgressStage)
	return nil
}

func (s *failingSaveStore) SaveIfOwned(ctx context.Context, task domainimagetask.Task, owner string, now time.Time) error {
	s.ownedSnapshots = append(s.ownedSnapshots, task)
	if s.failSaveIfOwned {
		s.failSaveIfOwned = false
		if s.failSaveIfOwnedError != nil {
			return s.failSaveIfOwnedError
		}
		return repoerr.ErrConflict
	}
	if err := s.base.SaveIfOwned(ctx, task, owner, now); err != nil {
		return err
	}
	s.recordProgressStage(task.ProgressStage)
	return nil
}

func (s *failingSaveStore) UpdateProgressIfOwned(ctx context.Context, taskID, owner, stage, message string, now time.Time) error {
	if err := s.base.UpdateProgressIfOwned(ctx, taskID, owner, stage, message, now); err != nil {
		return err
	}
	s.recordProgressStage(stage)
	return nil
}

func (s *failingSaveStore) recordProgressStage(stage string) {
	if strings.TrimSpace(stage) == "" {
		return
	}
	s.progressMu.Lock()
	defer s.progressMu.Unlock()
	if len(s.progressStages) == 0 || s.progressStages[len(s.progressStages)-1] != stage {
		s.progressStages = append(s.progressStages, stage)
	}
}

func (s *failingSaveStore) progressHistory() []string {
	s.progressMu.Lock()
	defer s.progressMu.Unlock()
	return append([]string(nil), s.progressStages...)
}

func TestProviderSuccessIsCheckpointedBeforeArtifactPersistence(t *testing.T) {
	cfg := taskTestConfig()
	cfg.Security.SecureConfigEncryptionKey = "artifact-recovery-test-key"
	providerCalls := 0
	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(context.Context, provider.ImageRequest) (provider.ImageResponse, error) {
			providerCalls++
			return provider.ImageResponse{ProviderRequestID: "req-paid-1", Data: []provider.ImageResult{{URL: "https://cdn.example.com/result.png?signature=top-secret"}}}, nil
		}},
	}
	store := &failingSaveStore{base: imagetask.NewMemoryStore()}
	failingRef := storage.BackendRef{ConfigID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Version: 3, Driver: "local", Backend: alwaysFailingArtifactBackend{}}
	router := &switchingImageRouter{defaultRef: failingRef, refs: map[string]storage.BackendRef{failingRef.ConfigID: failingRef}}
	svc := imagetask.NewServiceWithProvidersStoreAssetsBillingAndRouter(cfg, providers, store, nil, nil, router)
	svc.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		imageBytes, _ := base64.StdEncoding.DecodeString(tinyPNGBase64)
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(imageBytes))}, nil
	})})

	_, _ = svc.Execute(context.Background(), domainimagetask.ExecuteRequest{
		UserID: 77, AbstractModel: "plus", TaskType: string(provider.TaskTypeTextToImage), Prompt: "paid result",
		SizeMode: "ratio", AspectRatio: "1:1", BaseResolution: "1k", Quality: "auto", OutputImageCount: 1, ResponseFormat: string(provider.ResponseFormatURL), PreferredProviders: []string{"openrouter"},
	})
	if providerCalls != 1 {
		t.Fatalf("expected one provider call, got %d", providerCalls)
	}
	checkpointFound := false
	for _, snapshot := range store.ownedSnapshots {
		if snapshot.ProviderRequestID != "req-paid-1" || snapshot.ArtifactRecovery.EncryptedPayload == "" {
			continue
		}
		checkpointFound = true
		if snapshot.UpstreamSucceededAt == nil || len(snapshot.Attempts) != 1 || snapshot.Attempts[0].Status != domainimagetask.StatusSucceeded {
			t.Fatalf("provider success metadata missing from checkpoint: %#v", snapshot)
		}
		if strings.Contains(snapshot.ArtifactRecovery.EncryptedPayload, "top-secret") || strings.Contains(snapshot.ArtifactRecovery.EncryptedPayload, "cdn.example.com") {
			t.Fatalf("recovery payload was not encrypted: %s", snapshot.ArtifactRecovery.EncryptedPayload)
		}
		break
	}
	if !checkpointFound {
		t.Fatalf("expected provider success checkpoint before artifact failure, snapshots=%#v", store.ownedSnapshots)
	}
}

type alwaysFailingArtifactBackend struct{}

func (alwaysFailingArtifactBackend) Driver() string { return "local" }
func (alwaysFailingArtifactBackend) Put(context.Context, string, string, []byte) error {
	return errors.New("storage write unavailable")
}
func (alwaysFailingArtifactBackend) Get(context.Context, string) ([]byte, error) {
	return nil, errors.New("storage read unavailable")
}
func (alwaysFailingArtifactBackend) Delete(context.Context, string) error { return nil }

func (s *failingSaveStore) SaveTerminalState(ctx context.Context, task domainimagetask.Task, owner string, now time.Time) error {
	return s.base.SaveTerminalState(ctx, task, owner, now)
}

func (s *failingSaveStore) GetByID(ctx context.Context, userID int64, taskID string) (domainimagetask.Task, error) {
	return s.base.GetByID(ctx, userID, taskID)
}

func (s *failingSaveStore) GetImageResultByID(ctx context.Context, userID int64, imageID string) (provider.ImageResult, error) {
	return s.base.GetImageResultByID(ctx, userID, imageID)
}

func (s *failingSaveStore) GetImageResultForAdmin(ctx context.Context, imageID string) (provider.ImageResult, error) {
	return s.base.GetImageResultForAdmin(ctx, imageID)
}

func (s *failingSaveStore) ListByUser(ctx context.Context, userID int64) ([]domainimagetask.Task, error) {
	return s.base.ListByUser(ctx, userID)
}

func (s *failingSaveStore) RequestPublish(ctx context.Context, userID int64, imageID string) (domainimagetask.GalleryImage, error) {
	return s.base.RequestPublish(ctx, userID, imageID)
}

func (s *failingSaveStore) CancelPublish(ctx context.Context, userID int64, imageID string) (domainimagetask.GalleryImage, error) {
	return s.base.CancelPublish(ctx, userID, imageID)
}

func (s *failingSaveStore) SetImageGroup(ctx context.Context, userID int64, imageID, imageGroup string) (domainimagetask.GalleryImage, error) {
	return s.base.SetImageGroup(ctx, userID, imageID, imageGroup)
}

func (s *failingSaveStore) ReviewImage(ctx context.Context, imageID, nextStatus, reviewReason string, publishedAt *time.Time) (domainimagetask.GalleryImage, error) {
	return s.base.ReviewImage(ctx, imageID, nextStatus, reviewReason, publishedAt)
}

func (s *failingSaveStore) DeleteImageResult(ctx context.Context, userID int64, imageID string) (provider.ImageResult, error) {
	return s.base.DeleteImageResult(ctx, userID, imageID)
}

func (s *failingSaveStore) ListGallery(ctx context.Context, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error) {
	return s.base.ListGallery(ctx, req)
}

func (s *failingSaveStore) ListGalleryByUser(ctx context.Context, userID int64, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error) {
	return s.base.ListGalleryByUser(ctx, userID, req)
}

func (s *failingSaveStore) ListPublicGallery(ctx context.Context, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error) {
	return s.base.ListPublicGallery(ctx, req)
}

func (s *failingSaveStore) GetPublicImage(ctx context.Context, imageID string, viewerUserID int64) (domainimagetask.GalleryImage, error) {
	return s.base.GetPublicImage(ctx, imageID, viewerUserID)
}

func (s *failingSaveStore) SetPublicImageInteraction(ctx context.Context, userID int64, imageID, kind string, active bool) (domainimagetask.GalleryImage, error) {
	return s.base.SetPublicImageInteraction(ctx, userID, imageID, kind, active)
}

func (s *failingSaveStore) DeleteByID(ctx context.Context, userID int64, taskID string) error {
	return s.base.DeleteByID(ctx, userID, taskID)
}

func (s *failingSaveStore) AcquireNextQueuedTask(ctx context.Context, owner string, now time.Time, leaseTTL time.Duration) (domainimagetask.Task, error) {
	if s.failAcquireError != nil {
		err := s.failAcquireError
		s.failAcquireError = nil
		return domainimagetask.Task{}, err
	}
	task, err := s.base.AcquireNextQueuedTask(ctx, owner, now, leaseTTL)
	if err == nil {
		s.recordProgressStage(task.ProgressStage)
	}
	return task, err
}

func (s *failingSaveStore) RenewTaskLease(ctx context.Context, taskID, owner string, now time.Time, leaseTTL time.Duration) (domainimagetask.Task, error) {
	return s.base.RenewTaskLease(ctx, taskID, owner, now, leaseTTL)
}

func TestAcquireNextTaskPreservesTransientStoreContention(t *testing.T) {
	store := &failingSaveStore{
		base:             imagetask.NewMemoryStore(),
		failAcquireError: errors.New("database table is locked: image_tasks"),
	}
	svc := imagetask.NewServiceWithStore(taskTestConfig(), store)

	_, ok, err := svc.AcquireNextTask(context.Background(), "worker-a", 30*time.Second)
	if ok {
		t.Fatal("expected no acquired task on transient store contention")
	}
	if !errors.Is(err, repoerr.ErrTransientContention) {
		t.Fatalf("expected transient contention error, got %v", err)
	}
}

type raceyTerminalStore struct {
	base                    *imagetask.MemoryStore
	failSaveIfOwned         bool
	failSaveIfOwnedError    error
	blockFirstTerminalSave  bool
	terminalSaveEntered     chan struct{}
	releaseTerminalSave     chan struct{}
	terminalSaveBlockerOnce sync.Once
}

func (s *raceyTerminalStore) Save(ctx context.Context, task domainimagetask.Task) error {
	return s.base.Save(ctx, task)
}

func (s *raceyTerminalStore) SaveIfOwned(ctx context.Context, task domainimagetask.Task, owner string, now time.Time) error {
	if s.failSaveIfOwned {
		s.failSaveIfOwned = false
		if s.failSaveIfOwnedError != nil {
			return s.failSaveIfOwnedError
		}
		return repoerr.ErrConflict
	}
	return s.base.SaveIfOwned(ctx, task, owner, now)
}

func (s *raceyTerminalStore) UpdateProgressIfOwned(ctx context.Context, taskID, owner, stage, message string, now time.Time) error {
	return s.base.UpdateProgressIfOwned(ctx, taskID, owner, stage, message, now)
}

func (s *raceyTerminalStore) SaveTerminalState(ctx context.Context, task domainimagetask.Task, owner string, now time.Time) error {
	if s.blockFirstTerminalSave {
		s.terminalSaveBlockerOnce.Do(func() {
			close(s.terminalSaveEntered)
			<-s.releaseTerminalSave
		})
	}
	return s.base.SaveTerminalState(ctx, task, owner, now)
}

func (s *raceyTerminalStore) GetByID(ctx context.Context, userID int64, taskID string) (domainimagetask.Task, error) {
	return s.base.GetByID(ctx, userID, taskID)
}

func (s *raceyTerminalStore) GetImageResultByID(ctx context.Context, userID int64, imageID string) (provider.ImageResult, error) {
	return s.base.GetImageResultByID(ctx, userID, imageID)
}

func (s *raceyTerminalStore) GetImageResultForAdmin(ctx context.Context, imageID string) (provider.ImageResult, error) {
	return s.base.GetImageResultForAdmin(ctx, imageID)
}

func (s *raceyTerminalStore) ListByUser(ctx context.Context, userID int64) ([]domainimagetask.Task, error) {
	return s.base.ListByUser(ctx, userID)
}

func (s *raceyTerminalStore) RequestPublish(ctx context.Context, userID int64, imageID string) (domainimagetask.GalleryImage, error) {
	return s.base.RequestPublish(ctx, userID, imageID)
}

func (s *raceyTerminalStore) CancelPublish(ctx context.Context, userID int64, imageID string) (domainimagetask.GalleryImage, error) {
	return s.base.CancelPublish(ctx, userID, imageID)
}

func (s *raceyTerminalStore) SetImageGroup(ctx context.Context, userID int64, imageID, imageGroup string) (domainimagetask.GalleryImage, error) {
	return s.base.SetImageGroup(ctx, userID, imageID, imageGroup)
}

func (s *raceyTerminalStore) ReviewImage(ctx context.Context, imageID, nextStatus, reviewReason string, publishedAt *time.Time) (domainimagetask.GalleryImage, error) {
	return s.base.ReviewImage(ctx, imageID, nextStatus, reviewReason, publishedAt)
}

func (s *raceyTerminalStore) DeleteImageResult(ctx context.Context, userID int64, imageID string) (provider.ImageResult, error) {
	return s.base.DeleteImageResult(ctx, userID, imageID)
}

func (s *raceyTerminalStore) ListGallery(ctx context.Context, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error) {
	return s.base.ListGallery(ctx, req)
}

func (s *raceyTerminalStore) ListGalleryByUser(ctx context.Context, userID int64, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error) {
	return s.base.ListGalleryByUser(ctx, userID, req)
}

func (s *raceyTerminalStore) ListPublicGallery(ctx context.Context, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error) {
	return s.base.ListPublicGallery(ctx, req)
}

func (s *raceyTerminalStore) GetPublicImage(ctx context.Context, imageID string, viewerUserID int64) (domainimagetask.GalleryImage, error) {
	return s.base.GetPublicImage(ctx, imageID, viewerUserID)
}

func (s *raceyTerminalStore) SetPublicImageInteraction(ctx context.Context, userID int64, imageID, kind string, active bool) (domainimagetask.GalleryImage, error) {
	return s.base.SetPublicImageInteraction(ctx, userID, imageID, kind, active)
}

func (s *raceyTerminalStore) DeleteByID(ctx context.Context, userID int64, taskID string) error {
	return s.base.DeleteByID(ctx, userID, taskID)
}

func (s *raceyTerminalStore) AcquireNextQueuedTask(ctx context.Context, owner string, now time.Time, leaseTTL time.Duration) (domainimagetask.Task, error) {
	return s.base.AcquireNextQueuedTask(ctx, owner, now, leaseTTL)
}

func (s *raceyTerminalStore) RenewTaskLease(ctx context.Context, taskID, owner string, now time.Time, leaseTTL time.Duration) (domainimagetask.Task, error) {
	return s.base.RenewTaskLease(ctx, taskID, owner, now, leaseTTL)
}

func seedBalance(t *testing.T, svc *billingservice.Service, userID int64, points string) {
	t.Helper()
	if _, err := svc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{
		UserID:       userID,
		ChangePoints: points,
		Reason:       "seed balance",
	}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}
}

func TestExecuteOpenAIFormatMultiImageFansOutAndChargesActualSuccesses(t *testing.T) {
	cfg := taskTestConfig()
	openaiCapability := cfg.Routing.ProviderCapabilities["openai"]
	openaiCapability.MaxImageCount = 1
	cfg.Routing.ProviderCapabilities["openai"] = openaiCapability
	var (
		mu    sync.Mutex
		calls int
	)
	providers := map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			mu.Lock()
			calls++
			call := calls
			mu.Unlock()
			if req.OutputImageCount != 1 {
				t.Fatalf("expected OpenAI-format fanout request count 1, got %d", req.OutputImageCount)
			}
			if call == 1 {
				return provider.ImageResponse{}, errors.New("upstream single image failed")
			}
			return provider.ImageResponse{Created: 1770000000 + int64(call), Data: []provider.ImageResult{{B64JSON: tinyPNGBase64}}}, nil
		}},
	}
	billingSvc := billingservice.NewService(cfg.Billing)
	seedBalance(t, billingSvc, 77, "40.00000")
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, providers, imagetask.NewMemoryStore(), nil, billingSvc))
	result, err := svc.Execute(context.Background(), domainimagetask.ExecuteRequest{
		UserID:             77,
		AbstractModel:      "plus",
		TaskType:           string(provider.TaskTypeTextToImage),
		Prompt:             "Generate a three image set",
		RequestedSize:      "1536x1024",
		BaseResolution:     "2k",
		OutputImageCount:   3,
		ResponseFormat:     string(provider.ResponseFormatB64JSON),
		PreferredProviders: []string{"openai"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 single-image OpenAI calls, got %d", calls)
	}
	if result.Task.Status != domainimagetask.StatusPartialFailed {
		t.Fatalf("expected partial_failed, got %s", result.Task.Status)
	}
	if len(result.Task.Results) != 2 {
		t.Fatalf("expected 2 persisted images, got %d", len(result.Task.Results))
	}
	if result.Task.ActualPoints != "16.00000" {
		t.Fatalf("expected actual charge for 2 successful images, got %s", result.Task.ActualPoints)
	}
}

func TestExecuteOpenAIFormatFansOutTwelveImagesByCandidateMaxFour(t *testing.T) {
	cfg := taskTestConfig()
	capability := cfg.Routing.ProviderCapabilities["openai"]
	capability.MaxImageCount = 4
	cfg.Routing.ProviderCapabilities["openai"] = capability

	var (
		mu      sync.Mutex
		batches []int
	)
	providers := map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(_ context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			mu.Lock()
			batches = append(batches, req.OutputImageCount)
			requestID := fmt.Sprintf("fanout-request-%d", len(batches))
			mu.Unlock()
			results := make([]provider.ImageResult, req.OutputImageCount)
			for i := range results {
				results[i] = provider.ImageResult{B64JSON: tinyPNGBase64}
			}
			return provider.ImageResponse{Created: 1770000100, ProviderRequestID: requestID, Data: results}, nil
		}},
	}
	svc := imagetask.NewServiceWithProviders(cfg, providers)

	result, err := svc.Execute(context.Background(), domainimagetask.ExecuteRequest{
		UserID: 83, AbstractModel: "plus", TaskType: "text_to_image", Prompt: "twelve images",
		SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "1:1", Quality: "auto", OutputFormat: "png", Moderation: "auto",
		OutputImageCount: 12, ResponseFormat: string(provider.ResponseFormatB64JSON), PreferredProviders: []string{"openai"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	mu.Lock()
	sort.Ints(batches)
	gotBatches := append([]int(nil), batches...)
	mu.Unlock()
	if !reflect.DeepEqual(gotBatches, []int{4, 4, 4}) {
		t.Fatalf("fan-out batches = %#v, want three calls of four", gotBatches)
	}
	if len(result.Task.Results) != 12 {
		t.Fatalf("persisted result count = %d, want 12", len(result.Task.Results))
	}
	if len(result.Task.Attempts) != 3 {
		t.Fatalf("fan-out attempts = %#v, want one per upstream call", result.Task.Attempts)
	}
	if result.Task.FallbackCount != 0 {
		t.Fatalf("fallback_count = %d, want zero for single-candidate fan-out", result.Task.FallbackCount)
	}
	requestIDs := map[string]struct{}{}
	for _, attempt := range result.Task.Attempts {
		requestIDs[attempt.ProviderRequestID] = struct{}{}
		if attempt.RequestedImageCount != 4 || attempt.ReturnedImageCount != 4 || attempt.OutboundSize != "1024x1024" {
			t.Fatalf("unexpected fan-out attempt diagnostics: %#v", attempt)
		}
	}
	if len(requestIDs) != 3 {
		t.Fatalf("fan-out request IDs must be unique: %#v", result.Task.Attempts)
	}
}

func TestExecuteOpenAIFormatCapsLegacyProviderMaxImageCountPerCall(t *testing.T) {
	for _, legacyMax := range []int{11, 12} {
		t.Run(fmt.Sprintf("legacy_max_%d", legacyMax), func(t *testing.T) {
			cfg := taskTestConfig()
			capability := cfg.Routing.ProviderCapabilities["openai"]
			capability.MaxImageCount = legacyMax
			cfg.Routing.ProviderCapabilities["openai"] = capability

			var (
				mu      sync.Mutex
				batches []int
			)
			providers := map[string]provider.ImageProvider{
				"openai": fakeProvider{generateFunc: func(_ context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
					mu.Lock()
					batches = append(batches, req.OutputImageCount)
					mu.Unlock()
					results := make([]provider.ImageResult, req.OutputImageCount)
					for i := range results {
						results[i] = provider.ImageResult{B64JSON: tinyPNGBase64}
					}
					return provider.ImageResponse{Created: 1770000101, Data: results}, nil
				}},
			}
			svc := imagetask.NewServiceWithProviders(cfg, providers)

			result, err := svc.Execute(context.Background(), domainimagetask.ExecuteRequest{
				UserID: 84, AbstractModel: "plus", TaskType: "text_to_image", Prompt: "legacy provider max",
				SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "1:1", Quality: "auto", OutputFormat: "png", Moderation: "auto",
				OutputImageCount: 12, ResponseFormat: string(provider.ResponseFormatB64JSON), PreferredProviders: []string{"openai"},
			})
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			mu.Lock()
			gotBatches := append([]int(nil), batches...)
			mu.Unlock()
			sort.Ints(gotBatches)
			if !reflect.DeepEqual(gotBatches, []int{2, 10}) {
				t.Fatalf("fan-out batches = %#v, want legacy max capped to [2 10]", gotBatches)
			}
			if len(result.Task.Results) != 12 {
				t.Fatalf("persisted result count = %d, want 12", len(result.Task.Results))
			}
		})
	}
}

func TestExecuteOpenAIFormatSplitsProviderBatchesAndQueuesAtAccountConcurrency(t *testing.T) {
	cfg := taskTestConfig()
	openaiCapability := cfg.Routing.ProviderCapabilities["openai"]
	openaiCapability.MaxImageCount = 2
	cfg.Routing.ProviderCapabilities["openai"] = openaiCapability

	var (
		mu        sync.Mutex
		active    int
		maxActive int
		batches   []int
	)
	release := make(chan struct{})
	started := make(chan struct{}, 5)
	providers := map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			mu.Lock()
			active++
			if active > maxActive {
				maxActive = active
			}
			batches = append(batches, req.OutputImageCount)
			mu.Unlock()
			started <- struct{}{}
			<-release
			mu.Lock()
			active--
			mu.Unlock()
			results := make([]provider.ImageResult, req.OutputImageCount)
			for i := range results {
				results[i] = provider.ImageResult{B64JSON: tinyPNGBase64}
			}
			return provider.ImageResponse{Created: 1770001000, Data: results}, nil
		}},
	}
	svc := withMockRemoteFetch(imagetask.NewServiceWithProviders(cfg, providers))
	svc.SetModelRoutingSource(&staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{ProviderModels: []modelhub.ProviderCandidate{{
		Provider:                "openai",
		ModelCode:               "gpt-image-1",
		SupportedTaskTypes:      []string{"text_to_image"},
		SupportedBaseResolution: []string{"1k"},
		SupportedAspectRatios:   []string{"1:1"},
		Quality:                 []string{"auto"},
		OutputFormat:            []string{"png"},
		Moderation:              []string{"auto"},
		MaxImageCount:           2,
		ConcurrencyLimit:        2,
		HealthStatus:            "enabled",
	}}}})

	resultCh := make(chan domainimagetask.ExecuteResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := svc.Execute(context.Background(), domainimagetask.ExecuteRequest{
			UserID: 88, AbstractModel: "plus", TaskType: string(provider.TaskTypeTextToImage), Prompt: "five images",
			BaseResolution: "1k", AspectRatio: "1:1", Quality: "auto", OutputFormat: "png", Moderation: "auto",
			OutputImageCount: 5, ResponseFormat: string(provider.ResponseFormatB64JSON), PreferredProviders: []string{"openai"},
		})
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case err := <-errCh:
			t.Fatalf("Execute: %v", err)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for bounded fanout to start")
		}
	}
	select {
	case <-started:
		t.Fatal("third provider batch started before account concurrency was released")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)

	var result domainimagetask.ExecuteResult
	select {
	case err := <-errCh:
		t.Fatalf("Execute: %v", err)
	case result = <-resultCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for fanout result")
	}
	mu.Lock()
	sort.Ints(batches)
	gotBatches := append([]int(nil), batches...)
	gotMaxActive := maxActive
	mu.Unlock()
	if !reflect.DeepEqual(gotBatches, []int{1, 2, 2}) {
		t.Fatalf("expected batches [2 2 1], got %v", gotBatches)
	}
	if gotMaxActive != 2 {
		t.Fatalf("expected max account concurrency 2, got %d", gotMaxActive)
	}
	if len(result.Task.Results) != 5 || result.Task.Status != domainimagetask.StatusSucceeded {
		t.Fatalf("expected one succeeded task with five results, got %#v", result.Task)
	}
}

func TestExecuteFanoutSharesModelAccountConcurrencyAcrossTasks(t *testing.T) {
	cfg := taskTestConfig()
	openaiCapability := cfg.Routing.ProviderCapabilities["openai"]
	openaiCapability.MaxImageCount = 1
	cfg.Routing.ProviderCapabilities["openai"] = openaiCapability

	started := make(chan struct{}, 4)
	release := make(chan struct{})
	providers := map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			started <- struct{}{}
			<-release
			return provider.ImageResponse{Created: 1770001100, Data: []provider.ImageResult{{B64JSON: tinyPNGBase64}}}, nil
		}},
	}
	svc := withMockRemoteFetch(imagetask.NewServiceWithProviders(cfg, providers))
	svc.SetModelRoutingSource(&staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{ProviderModels: []modelhub.ProviderCandidate{{
		ModelAccountID:          901,
		Provider:                "openai",
		ModelCode:               "gpt-image-1",
		SupportedTaskTypes:      []string{"text_to_image"},
		SupportedBaseResolution: []string{"1k"},
		SupportedAspectRatios:   []string{"1:1"},
		Quality:                 []string{"auto"},
		OutputFormat:            []string{"png"},
		Moderation:              []string{"auto"},
		MaxImageCount:           1,
		ConcurrencyLimit:        1,
		HealthStatus:            "enabled",
	}}}})

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for userID := int64(91); userID <= 92; userID++ {
		userID := userID
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Execute(context.Background(), domainimagetask.ExecuteRequest{
				UserID: userID, AbstractModel: "plus", TaskType: string(provider.TaskTypeTextToImage), Prompt: "two images",
				BaseResolution: "1k", AspectRatio: "1:1", Quality: "auto", OutputFormat: "png", Moderation: "auto",
				OutputImageCount: 2, ResponseFormat: string(provider.ResponseFormatB64JSON), PreferredProviders: []string{"openai"},
			})
			if err != nil {
				errCh <- err
			}
		}()
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first account request")
	}
	select {
	case <-started:
		t.Fatal("another task bypassed the shared model-account concurrency limit")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("Execute: %v", err)
	}
}

func TestExecuteFanoutSharesUserConcurrencyAcrossTasks(t *testing.T) {
	cfg := taskTestConfig()
	openaiCapability := cfg.Routing.ProviderCapabilities["openai"]
	openaiCapability.MaxImageCount = 1
	cfg.Routing.ProviderCapabilities["openai"] = openaiCapability

	started := make(chan struct{}, 4)
	release := make(chan struct{})
	providers := map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			started <- struct{}{}
			<-release
			return provider.ImageResponse{Created: 1770001150, Data: []provider.ImageResult{{B64JSON: tinyPNGBase64}}}, nil
		}},
	}
	store := &userLimitedStore{Store: imagetask.NewMemoryStore(), limit: 1}
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersAndStore(cfg, providers, store))
	svc.SetModelRoutingSource(&staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{ProviderModels: []modelhub.ProviderCandidate{{
		ModelAccountID:          902,
		Provider:                "openai",
		ModelCode:               "gpt-image-1",
		SupportedTaskTypes:      []string{"text_to_image"},
		SupportedBaseResolution: []string{"1k"},
		SupportedAspectRatios:   []string{"1:1"},
		Quality:                 []string{"auto"},
		OutputFormat:            []string{"png"},
		Moderation:              []string{"auto"},
		MaxImageCount:           1,
		ConcurrencyLimit:        4,
		HealthStatus:            "enabled",
	}}}})

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Execute(context.Background(), domainimagetask.ExecuteRequest{
				UserID: 94, AbstractModel: "plus", TaskType: string(provider.TaskTypeTextToImage), Prompt: "two images",
				BaseResolution: "1k", AspectRatio: "1:1", Quality: "auto", OutputFormat: "png", Moderation: "auto",
				OutputImageCount: 2, ResponseFormat: string(provider.ResponseFormatB64JSON), PreferredProviders: []string{"openai"},
			})
			if err != nil {
				errCh <- err
			}
		}()
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first user request")
	}
	select {
	case <-started:
		t.Fatal("another task bypassed the shared user concurrency limit")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("Execute: %v", err)
	}
}

func TestExecuteFanoutSharesConcurrencyAcrossServiceInstances(t *testing.T) {
	cfg := taskTestConfig()
	openaiCapability := cfg.Routing.ProviderCapabilities["openai"]
	openaiCapability.MaxImageCount = 1
	cfg.Routing.ProviderCapabilities["openai"] = openaiCapability

	started := make(chan struct{}, 4)
	release := make(chan struct{})
	providers := map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			started <- struct{}{}
			<-release
			return provider.ImageResponse{Created: 1770001160, Data: []provider.ImageResult{{B64JSON: tinyPNGBase64}}}, nil
		}},
	}
	routing := &staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{ProviderModels: []modelhub.ProviderCandidate{{
		ModelAccountID: 903, Provider: "openai", ModelCode: "gpt-image-1",
		SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"}, SupportedAspectRatios: []string{"1:1"},
		Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"},
		MaxImageCount: 1, ConcurrencyLimit: 1, HealthStatus: "enabled",
	}}}}
	sharedGate := imagetask.NewLocalConcurrencyGate()
	services := make([]*imagetask.Service, 0, 2)
	for i := 0; i < 2; i++ {
		store := &userLimitedStore{Store: imagetask.NewMemoryStore(), limit: 1}
		svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersAndStore(cfg, providers, store))
		svc.SetModelRoutingSource(routing)
		svc.SetConcurrencyGate(sharedGate)
		services = append(services, svc)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for _, svc := range services {
		svc := svc
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Execute(context.Background(), domainimagetask.ExecuteRequest{
				UserID: 96, AbstractModel: "plus", TaskType: string(provider.TaskTypeTextToImage), Prompt: "cross service",
				BaseResolution: "1k", AspectRatio: "1:1", Quality: "auto", OutputFormat: "png", Moderation: "auto",
				OutputImageCount: 1, ResponseFormat: string(provider.ResponseFormatB64JSON), PreferredProviders: []string{"openai"},
			})
			if err != nil {
				errCh <- err
			}
		}()
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first cross-service request")
	}
	select {
	case <-started:
		t.Fatal("second service bypassed shared concurrency gate")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("Execute: %v", err)
	}
}

func TestExecuteNonOpenAIProviderSplitsBatchesAndMarksPartialSuccess(t *testing.T) {
	cfg := taskTestConfig()
	openrouterCapability := cfg.Routing.ProviderCapabilities["openrouter"]
	openrouterCapability.MaxImageCount = 2
	cfg.Routing.ProviderCapabilities["openrouter"] = openrouterCapability

	var (
		mu      sync.Mutex
		batches []int
	)
	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			mu.Lock()
			batches = append(batches, req.OutputImageCount)
			call := len(batches)
			mu.Unlock()
			if call == 1 {
				return provider.ImageResponse{}, errors.New("one provider batch failed")
			}
			results := make([]provider.ImageResult, req.OutputImageCount)
			for i := range results {
				results[i] = provider.ImageResult{B64JSON: tinyPNGBase64}
			}
			return provider.ImageResponse{Created: 1770001200, Data: results}, nil
		}},
	}
	svc := withMockRemoteFetch(imagetask.NewServiceWithProviders(cfg, providers))
	result, err := svc.Execute(context.Background(), domainimagetask.ExecuteRequest{
		UserID: 93, AbstractModel: "plus", TaskType: string(provider.TaskTypeTextToImage), Prompt: "five images",
		BaseResolution: "1k", AspectRatio: "1:1", Quality: "auto", OutputFormat: "png", Moderation: "auto",
		OutputImageCount: 5, ResponseFormat: string(provider.ResponseFormatB64JSON), PreferredProviders: []string{"openrouter"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	mu.Lock()
	sort.Ints(batches)
	gotBatches := append([]int(nil), batches...)
	mu.Unlock()
	if !reflect.DeepEqual(gotBatches, []int{1, 2, 2}) {
		t.Fatalf("expected provider batches [2 2 1], got %v", gotBatches)
	}
	if result.Task.Status != domainimagetask.StatusPartialFailed || len(result.Task.Results) >= 5 || len(result.Task.Results) == 0 {
		t.Fatalf("expected partial task result from mixed batch outcomes, got %#v", result.Task)
	}
	if result.Task.ErrorCode == "" || result.Task.ErrorMessage == "" {
		t.Fatalf("partial task must preserve an actionable failure reason, got %#v", result.Task)
	}
}

func TestExecuteLeasedTaskWaitsToCheckpointOpenAIFanoutBeforePersistence(t *testing.T) {
	cfg := taskTestConfig()
	openaiCapability := cfg.Routing.ProviderCapabilities["openai"]
	openaiCapability.MaxImageCount = 1
	cfg.Routing.ProviderCapabilities["openai"] = openaiCapability
	billingSvc := billingservice.NewService(cfg.Billing)
	seedBalance(t, billingSvc, 278, "40.00000")

	var (
		mu    sync.Mutex
		calls int
	)
	releaseLaterCalls := make(chan struct{})
	store := imagetask.NewMemoryStore()
	providers := map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			mu.Lock()
			calls++
			call := calls
			mu.Unlock()
			if req.OutputImageCount != 1 {
				t.Fatalf("expected single image fanout request, got %d", req.OutputImageCount)
			}
			if call > 1 {
				<-releaseLaterCalls
			}
			if call == 3 {
				return provider.ImageResponse{}, errors.New("upstream single image failed")
			}
			return provider.ImageResponse{Created: 1770000100 + int64(call), Data: []provider.ImageResult{{B64JSON: tinyPNGBase64}}}, nil
		}},
	}
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, providers, store, nil, billingSvc))

	created, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:              278,
		UserGroupCode:       "basic",
		UserGroupMultiplier: "1.00000",
		AbstractModel:       "plus",
		TaskType:            string(provider.TaskTypeTextToImage),
		Prompt:              "Persist progressive results",
		RequestedSize:       "1536x1024",
		BaseResolution:      "2k",
		OutputImageCount:    3,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	claimed, ok, err := svc.AcquireNextTask(context.Background(), "worker-a", 30*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextTask: %v", err)
	}
	if !ok {
		t.Fatal("expected task claim to succeed")
	}

	resultCh := make(chan domainimagetask.ExecuteResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, runErr := svc.ExecuteLeasedTask(context.Background(), claimed, "worker-a", []string{"openai"})
		if runErr != nil {
			errCh <- runErr
			return
		}
		resultCh <- result
	}()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case err := <-errCh:
			t.Fatalf("ExecuteLeasedTask: %v", err)
		case <-deadline:
			t.Fatal("timed out waiting for partial running snapshot")
		default:
			mu.Lock()
			startedCalls := calls
			mu.Unlock()
			if startedCalls == 3 {
				loaded, loadErr := svc.GetByID(context.Background(), 278, created.ID)
				if loadErr != nil {
					t.Fatalf("GetByID before checkpoint: %v", loadErr)
				}
				if len(loaded.Results) != 0 || loaded.ArtifactRecovery.EncryptedPayload != "" {
					t.Fatalf("fanout must not persist artifacts before all paid responses are gathered: %#v", loaded)
				}
				close(releaseLaterCalls)
				goto waitFinal
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

waitFinal:
	var result domainimagetask.ExecuteResult
	select {
	case err := <-errCh:
		t.Fatalf("ExecuteLeasedTask: %v", err)
	case result = <-resultCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for final leased task result")
	}
	if result.Task.Status != domainimagetask.StatusPartialFailed {
		t.Fatalf("expected partial_failed final status, got %s", result.Task.Status)
	}
	if len(result.Task.Results) != 2 {
		t.Fatalf("expected 2 final persisted results, got %d", len(result.Task.Results))
	}
}

func TestExecuteLeasedTaskRecordsAttemptWhenOpenAIFanoutAllFails(t *testing.T) {
	cfg := taskTestConfig()
	openaiCapability := cfg.Routing.ProviderCapabilities["openai"]
	openaiCapability.MaxImageCount = 1
	cfg.Routing.ProviderCapabilities["openai"] = openaiCapability
	billingSvc := billingservice.NewService(cfg.Billing)
	seedBalance(t, billingSvc, 279, "40.00000")

	store := imagetask.NewMemoryStore()
	providers := map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			if req.OutputImageCount != 1 {
				t.Fatalf("expected single image fanout request, got %d", req.OutputImageCount)
			}
			return provider.ImageResponse{}, provider.NewTransportError(provider.ProviderTypeOpenAI, io.ErrUnexpectedEOF)
		}},
	}
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, providers, store, nil, billingSvc))

	created, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:              279,
		UserGroupCode:       "basic",
		UserGroupMultiplier: "1.00000",
		AbstractModel:       "plus",
		TaskType:            string(provider.TaskTypeTextToImage),
		Prompt:              "All fanout requests fail",
		RequestedSize:       "1536x1024",
		BaseResolution:      "2k",
		OutputImageCount:    3,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	claimed, ok, err := svc.AcquireNextTask(context.Background(), "worker-a", 30*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextTask: %v", err)
	}
	if !ok {
		t.Fatal("expected task claim to succeed")
	}

	result, err := svc.ExecuteLeasedTask(context.Background(), claimed, "worker-a", []string{"openai"})
	if err == nil {
		t.Fatal("expected fanout execution error")
	}
	if result.Task.Status != domainimagetask.StatusFailed {
		t.Fatalf("expected failed task, got %s", result.Task.Status)
	}
	if len(result.Task.Attempts) != 3 {
		t.Fatalf("expected every failed fanout attempt to be recorded, got %#v", result.Task.Attempts)
	}
	for _, attempt := range result.Task.Attempts {
		if attempt.ErrorCode != "transport_error" {
			t.Fatalf("expected transport_error attempt, got %#v", attempt)
		}
	}

	loaded, err := svc.GetByID(context.Background(), 279, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if len(loaded.Attempts) != 3 {
		t.Fatalf("expected stored failed fanout attempt, got %#v", loaded.Attempts)
	}
}

func TestRemoteURLDimensionsUpdateProviderAttemptAfterPersistence(t *testing.T) {
	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(context.Context, provider.ImageRequest) (provider.ImageResponse, error) {
			return provider.ImageResponse{ProviderRequestID: "url-result-request", Data: []provider.ImageResult{{URL: "https://cdn.example.com/result.png"}}}, nil
		}},
	}
	svc := withMockRemoteFetch(imagetask.NewServiceWithProviders(taskTestConfig(), providers))
	result, err := svc.Execute(context.Background(), domainimagetask.ExecuteRequest{
		UserID: 280, AbstractModel: "plus", TaskType: "text_to_image", Prompt: "URL result dimensions",
		SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "1:1", OutputImageCount: 1,
		ResponseFormat: string(provider.ResponseFormatURL), PreferredProviders: []string{"openrouter"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Task.Attempts) != 1 {
		t.Fatalf("attempts = %#v", result.Task.Attempts)
	}
	attempt := result.Task.Attempts[0]
	if attempt.ProviderRequestID != "url-result-request" || attempt.ReturnedWidth != 1 || attempt.ReturnedHeight != 1 || attempt.SizeDiagnostic != "upstream_rewritten" {
		t.Fatalf("URL result dimensions were not reconciled to the attempt: %#v", attempt)
	}
}

func TestExecuteWithStorePersistsThroughStoreBackend(t *testing.T) {
	cfg := taskTestConfig()
	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			return provider.ImageResponse{Created: 1770000002, Data: []provider.ImageResult{{URL: "https://cdn.example.com/persisted.png"}}}, nil
		}},
	}
	store := imagetask.NewMemoryStore()
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersAndStore(cfg, providers, store))
	result, err := svc.Execute(context.Background(), domainimagetask.ExecuteRequest{
		UserID:             21,
		AbstractModel:      "plus",
		TaskType:           string(provider.TaskTypeTextToImage),
		Prompt:             "Generate persisted task",
		RequestedSize:      "auto",
		BaseResolution:     "auto",
		OutputImageCount:   1,
		ResponseFormat:     string(provider.ResponseFormatURL),
		PreferredProviders: []string{"openrouter"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	loaded, err := svc.GetByID(context.Background(), 21, result.Task.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if loaded.ID != result.Task.ID || len(loaded.Results) != 1 {
		t.Fatalf("unexpected persisted task %#v", loaded)
	}
}

func TestGeneratedImageStorageRouterPinsOriginalConfig(t *testing.T) {
	cfg := taskTestConfig()
	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(context.Context, provider.ImageRequest) (provider.ImageResponse, error) {
			return provider.ImageResponse{Data: []provider.ImageResult{{URL: "https://cdn.example.com/routed.png"}}}, nil
		}},
	}
	original := storage.BackendRef{ConfigID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Version: 2, Driver: "local", Backend: storage.NewLocalBackend(t.TempDir())}
	replacement := storage.BackendRef{ConfigID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", Version: 1, Driver: "local", Backend: storage.NewLocalBackend(t.TempDir())}
	router := &switchingImageRouter{defaultRef: original, refs: map[string]storage.BackendRef{original.ConfigID: original, replacement.ConfigID: replacement}}
	store := imagetask.NewMemoryStore()
	svc := imagetask.NewServiceWithProvidersStoreAssetsBillingAndRouter(cfg, providers, store, nil, nil, router)
	imageBytes, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+y1X8AAAAASUVORK5CYII=")
	svc.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(imageBytes))}, nil
	})})

	result, err := svc.Execute(context.Background(), domainimagetask.ExecuteRequest{
		UserID: 31, AbstractModel: "plus", TaskType: string(provider.TaskTypeTextToImage), Prompt: "route it",
		SizeMode: "ratio", AspectRatio: "1:1", BaseResolution: "1k", Quality: "auto", OutputImageCount: 1, ResponseFormat: string(provider.ResponseFormatURL), PreferredProviders: []string{"openrouter"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Task.Results) != 1 || result.Task.Results[0].StorageConfigID != original.ConfigID {
		t.Fatalf("expected result pinned to original config, got %#v", result.Task.Results)
	}
	router.defaultRef = replacement
	_, downloaded, err := svc.DownloadImageResult(context.Background(), 31, result.Task.Results[0].ID)
	if err != nil {
		t.Fatalf("DownloadImageResult after switch: %v", err)
	}
	if len(downloaded) == 0 {
		t.Fatal("expected historical image bytes")
	}
}

type switchingImageRouter struct {
	defaultRef storage.BackendRef
	refs       map[string]storage.BackendRef
}

func (r *switchingImageRouter) DefaultWriter(context.Context) (storage.BackendRef, error) {
	return r.defaultRef, nil
}

func (r *switchingImageRouter) BackendFor(_ context.Context, configID string, _ string) (storage.BackendRef, error) {
	return r.refs[configID], nil
}

func (r *switchingImageRouter) ReadableBackends(context.Context) ([]storage.BackendRef, error) {
	refs := make([]storage.BackendRef, 0, len(r.refs))
	for _, ref := range r.refs {
		refs = append(refs, ref)
	}
	return refs, nil
}

func TestCreateTaskQueuesResolvedTask(t *testing.T) {
	cfg := taskTestConfig()
	store := imagetask.NewMemoryStore()
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersAndStore(cfg, nil, store))

	task, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:           33,
		AbstractModel:    "plus",
		TaskType:         string(provider.TaskTypeTextToImage),
		Prompt:           "Queue me",
		RequestedSize:    "auto",
		BaseResolution:   "auto",
		OutputImageCount: 2,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.Status != domainimagetask.StatusQueued {
		t.Fatalf("expected queued task, got %s", task.Status)
	}
	if task.BaseResolution != "2k" {
		t.Fatalf("expected resolved base resolution 2k, got %s", task.BaseResolution)
	}
	if task.OutputImageCount != 2 {
		t.Fatalf("expected output count 2, got %d", task.OutputImageCount)
	}
	if task.LeaseOwner != "" || task.LeaseExpiresAt != nil {
		t.Fatalf("expected no active lease, got owner=%q lease=%v", task.LeaseOwner, task.LeaseExpiresAt)
	}

	loaded, err := svc.GetByID(context.Background(), 33, task.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if loaded.Status != domainimagetask.StatusQueued {
		t.Fatalf("expected persisted queued task, got %s", loaded.Status)
	}
}

func TestCreateTaskRejectsUnsupportedTypeWithoutPersistingHistory(t *testing.T) {
	store := imagetask.NewMemoryStore()
	svc := imagetask.NewServiceWithProvidersAndStore(taskTestConfig(), nil, store)

	_, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:           33,
		AbstractModel:    "plus",
		TaskType:         "reference_generate",
		Prompt:           "This removed task type must never enter history",
		RequestedSize:    "auto",
		BaseResolution:   "auto",
		OutputImageCount: 1,
	})
	if err == nil {
		t.Fatal("expected unsupported task type to be rejected")
	}
	tasks, listErr := store.ListByUser(context.Background(), 33)
	if listErr != nil {
		t.Fatalf("ListByUser: %v", listErr)
	}
	if len(tasks) != 0 {
		t.Fatalf("unsupported task type must not create history, got %d tasks", len(tasks))
	}
}

func TestRetryTaskCreatesQueuedCopyFromFailedTask(t *testing.T) {
	cfg := taskTestConfig()
	store := imagetask.NewMemoryStore()
	svc := imagetask.NewServiceWithProvidersAndStore(cfg, nil, store)
	projectSvc := projectservice.NewService(projectservice.NewMemoryStore())
	project, err := projectSvc.Create(context.Background(), 33, domainproject.CreateRequest{Name: "Retry project"})
	if err != nil {
		t.Fatalf("Create project: %v", err)
	}
	svc.SetProjectResolver(projectSvc)
	seed := domainimagetask.Task{
		UserID:              33,
		ProjectID:           project.ID,
		ID:                  "11111111-1111-1111-1111-111111111111",
		Status:              domainimagetask.StatusFailed,
		AbstractModel:       "plus",
		TaskType:            string(provider.TaskTypeImageEdit),
		Prompt:              "Retry this prompt",
		NegativePrompt:      "low quality",
		RequestedSize:       "1536x864",
		BaseResolution:      "auto",
		AspectRatio:         "16:9",
		OutputImageCount:    2,
		ReferenceImageCount: 2,
		ReferenceAssetIDs:   []string{"asset-a", "asset-b"},
		ReferenceStrength:   70,
		ResponseMode:        "async",
		SavePolicy:          "private",
		ErrorCode:           errs.CodeImageStorageFailed,
		ErrorMessage:        "failed to store image",
	}
	if err := store.Save(context.Background(), seed); err != nil {
		t.Fatalf("Save seed task: %v", err)
	}

	retry, err := svc.RetryTask(context.Background(), 33, seed.ID, domainimagetask.RetryRequest{
		UserGroupCode:       "basic",
		UserGroupCodes:      []string{"basic"},
		UserGroupMultiplier: "1.00000",
	})
	if err != nil {
		t.Fatalf("RetryTask: %v", err)
	}
	if retry.ID == seed.ID {
		t.Fatalf("expected retry to create a new task id")
	}
	if retry.Status != domainimagetask.StatusQueued {
		t.Fatalf("expected retry queued, got %s", retry.Status)
	}
	if retry.ProjectID != project.ID {
		t.Fatalf("retry project = %q, want original non-default project %q", retry.ProjectID, project.ID)
	}
	if retry.Prompt != seed.Prompt || retry.NegativePrompt != seed.NegativePrompt || retry.RouteModelCode != seed.RouteModelCode || retry.TaskType != seed.TaskType {
		t.Fatalf("retry did not copy core request fields: %#v", retry)
	}
	if retry.OutputImageCount != seed.OutputImageCount || retry.ReferenceImageCount != seed.ReferenceImageCount || retry.ReferenceStrength != seed.ReferenceStrength {
		t.Fatalf("retry did not copy count/reference fields: %#v", retry)
	}
	if len(retry.ReferenceAssetIDs) != 2 || retry.ReferenceAssetIDs[0] != "asset-a" || retry.ReferenceAssetIDs[1] != "asset-b" {
		t.Fatalf("retry did not copy reference assets: %#v", retry.ReferenceAssetIDs)
	}
	original, err := svc.GetByID(context.Background(), 33, seed.ID)
	if err != nil {
		t.Fatalf("Get original: %v", err)
	}
	if original.Status != domainimagetask.StatusFailed || original.ErrorCode != errs.CodeImageStorageFailed {
		t.Fatalf("expected original failed task to remain unchanged, got %#v", original)
	}
}

func TestCreateTaskReservesPointsAndRollsBackIfTaskSaveFails(t *testing.T) {
	cfg := taskTestConfig()
	billingSvc := billingservice.NewService(cfg.Billing)
	seedBalance(t, billingSvc, 77, "20.00000")

	store := &failingSaveStore{base: imagetask.NewMemoryStore(), failSave: true}
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, nil, store, nil, billingSvc))

	_, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:              77,
		UserGroupCode:       "basic",
		UserGroupMultiplier: "1.00000",
		AbstractModel:       "plus",
		TaskType:            string(provider.TaskTypeTextToImage),
		Prompt:              "Queue me",
		RequestedSize:       "auto",
		BaseResolution:      "auto",
		OutputImageCount:    1,
	})
	if err == nil {
		t.Fatal("expected CreateTask to fail when store save fails")
	}

	summary, err := billingSvc.GetBalance(context.Background(), 77, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if summary.AvailablePoints != "20.00000" || summary.FrozenPoints != "0.00000" {
		t.Fatalf("expected reserve rollback after save failure, got %#v", summary)
	}
}

func TestCreateTaskRejectsStaleCapabilityVersion(t *testing.T) {
	cfg := taskTestConfig()
	routing := &staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices:      []modelhub.RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true}},
		ProviderModels: []modelhub.ProviderCandidate{{
			AccountModelID: 11, Provider: "openai", ModelCode: "gpt-image-2", SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"},
			SizeModes: []string{"ratio"}, SupportedAspectRatios: []string{"1:1"}, Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"}, MaxImageCount: 1,
		}},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 11, Enabled: true}},
	}}
	resolver := modelhub.NewResolver(cfg)
	resolver.SetModelRoutingSource(routing)
	resolved, err := resolver.ResolveContext(context.Background(), modelhub.ResolveRequest{
		RouteModelCode: "plus", TaskType: "text_to_image", SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "1:1",
		Quality: "auto", OutputFormat: "png", Moderation: "auto", RequestedOutputImageCount: 1,
	})
	if err != nil || resolved.CapabilityVersion == "" {
		t.Fatalf("initial resolve = %#v, %v", resolved, err)
	}
	svc := imagetask.NewServiceWithProvidersAndStore(cfg, nil, imagetask.NewMemoryStore())
	svc.SetModelRoutingSource(routing)
	request := domainimagetask.CreateRequest{
		UserID: 88, RouteModelCode: "plus", AbstractModel: "plus", TaskType: "text_to_image", Prompt: "versioned",
		SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "1:1", Quality: "auto", OutputFormat: "png", Moderation: "auto", OutputImageCount: 1,
		CapabilityVersion: resolved.CapabilityVersion,
	}
	if _, err := svc.CreateTask(context.Background(), request); err != nil {
		t.Fatalf("matching capability version rejected: %v", err)
	}
	request.TaskID = "stale-capability-task"
	request.CapabilityVersion = "stale-version"
	_, err = svc.CreateTask(context.Background(), request)
	var appErr *errs.Error
	if !errors.As(err, &appErr) || appErr.StatusCode != 409 || appErr.Code != modelhub.CodeCapabilityChanged {
		t.Fatalf("stale create error = %#v, want 409/%s", err, modelhub.CodeCapabilityChanged)
	}
}

func TestCreateTaskUsesOneRoutingSnapshotWhenCapabilityVersionIsOmitted(t *testing.T) {
	cfg := taskTestConfig()
	first := modelhub.ModelRoutingSnapshot{
		Version:     "routing-a",
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices:      []modelhub.RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true}},
		ProviderModels: []modelhub.ProviderCandidate{{
			AccountModelID: 11, Provider: "openai", ModelCode: "gpt-image-2", SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"},
			SizeModes: []string{"ratio"}, SupportedAspectRatios: []string{"1:1"}, Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"}, MaxImageCount: 1,
			MinWidth: 512, MaxWidth: 900, MinHeight: 512, MaxHeight: 900,
		}},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 11, Enabled: true}},
	}
	second := first
	second.Version = "routing-b"
	second.Prices = []modelhub.RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "9.00000", Enabled: true}}
	source := &rotatingModelRoutingSource{snapshots: []modelhub.ModelRoutingSnapshot{first, second}}
	billingSvc := billingservice.NewService(cfg.Billing)
	billingSvc.SetModelRoutingSource(source)
	seedBalance(t, billingSvc, 89, "20.00000")
	svc := imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, nil, imagetask.NewMemoryStore(), nil, billingSvc)
	svc.SetModelRoutingSource(source)

	task, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID: 89, RouteModelCode: "plus", AbstractModel: "plus", TaskType: "text_to_image", Prompt: "single snapshot",
		SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "1:1", Quality: "auto", OutputFormat: "png", Moderation: "auto", OutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if source.reads != 1 {
		t.Fatalf("routing source reads = %d, want one atomic resolve-and-estimate snapshot", source.reads)
	}
	if task.RouteSnapshotVersion != "routing-a" || task.EstimatedPoints != "1.00000" || task.RequestedSize != "896x896" || task.ResolvedWidth != 896 || task.ResolvedHeight != 896 {
		t.Fatalf("task mixed routing snapshots: %#v", task)
	}
}

func TestCreateTaskAcceptsFreshEstimateFromDynamicAutoResolutionConfig(t *testing.T) {
	cfg := taskTestConfig()
	cfg.Billing.AutoBaseResolutionDefaultByGroup = map[string]string{"plus": "1k"}
	routing := &staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices: []modelhub.RoutePriceConfig{
			{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true},
			{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "2k", BasePoints: "2.00000", Enabled: true},
		},
		ProviderModels: []modelhub.ProviderCandidate{{
			AccountModelID: 11, Provider: "openai", ModelCode: "gpt-image-2", SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k", "2k"},
			SizeModes: []string{"ratio"}, SupportedAspectRatios: []string{"1:1"}, Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"}, MaxImageCount: 1,
		}},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 11, Enabled: true}},
	}}
	adminSvc := adminconfigservice.NewService(cfg)
	if _, err := adminSvc.UpdateTab(context.Background(), domainadminconfig.UpdateTabRequest{
		TabKey: "billing_pricing", Version: 1,
		Items: []domainadminconfig.Item{{
			ConfigCategory: "billing_pricing", ConfigKey: "auto_base_resolution_default_by_group",
			ConfigValue: map[string]any{"value": map[string]any{"plus": "2k"}}, Scope: "global",
		}},
	}); err != nil {
		t.Fatalf("UpdateTab: %v", err)
	}
	billingSvc := billingservice.NewService(cfg.Billing)
	billingSvc.SetAdminConfigResolver(adminSvc)
	billingSvc.SetModelRoutingSource(routing)
	seedBalance(t, billingSvc, 90, "20.00000")
	estimate, err := billingSvc.Estimate(domainbilling.EstimateRequest{
		TaskType: "text_to_image", RouteModelCode: "plus", SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "1:1",
		Quality: "auto", OutputFormat: "png", Moderation: "auto", RequestedOutputImageCount: 1,
	})
	if err != nil || estimate.CapabilityVersion == "" {
		t.Fatalf("Estimate = %#v, %v", estimate, err)
	}
	svc := imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, nil, imagetask.NewMemoryStore(), nil, billingSvc)
	svc.SetModelRoutingSource(routing)
	_, err = svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID: 90, RouteModelCode: "plus", AbstractModel: "plus", TaskType: "text_to_image", Prompt: "dynamic config",
		SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "1:1", Quality: "auto", OutputFormat: "png", Moderation: "auto", OutputImageCount: 1,
		CapabilityVersion: estimate.CapabilityVersion,
	})
	if err != nil {
		t.Fatalf("fresh dynamic-config estimate token rejected by CreateTask: %v", err)
	}
}

func TestCreateTaskAtomicPreflightPreservesMaskCapability(t *testing.T) {
	cfg := taskTestConfig()
	routing := &staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices: []modelhub.RoutePriceConfig{{
			RouteModelID: 1, TaskType: "image_edit", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true,
		}},
		ProviderModels: []modelhub.ProviderCandidate{{
			AccountModelID: 11, Provider: "openai", ModelCode: "gpt-image-2", SupportedTaskTypes: []string{"image_edit"}, SupportedBaseResolution: []string{"1k"},
			SizeModes: []string{"ratio"}, SupportedAspectRatios: []string{"1:1"}, Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"}, MaxImageCount: 1,
			MaxReferenceImageCount: 1, SupportsImageInput: true, SupportsMask: false,
		}},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 11, Enabled: true}},
	}}
	billingSvc := billingservice.NewService(cfg.Billing)
	billingSvc.SetModelRoutingSource(routing)
	seedBalance(t, billingSvc, 91, "20.00000")
	svc := imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, nil, imagetask.NewMemoryStore(), nil, billingSvc)
	svc.SetModelRoutingSource(routing)

	_, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID: 91, RouteModelCode: "plus", AbstractModel: "plus", TaskType: "image_edit", Prompt: "masked edit",
		SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "1:1", Quality: "auto", OutputFormat: "png", Moderation: "auto", OutputImageCount: 1,
		ReferenceImageCount: 1, MaskPresent: true,
	})
	var appErr *errs.Error
	if !errors.As(err, &appErr) || appErr.StatusCode != 400 || appErr.Code != errs.CodeImageCapabilityMismatch {
		t.Fatalf("masked create error = %#v, want 400/%s", err, errs.CodeImageCapabilityMismatch)
	}
}

func TestCreateTaskRejectsEstimateTokenAfterDynamicTaskMultiplierChanges(t *testing.T) {
	cfg := taskTestConfig()
	cfg.Billing.TaskMultipliers = map[string]string{"text_to_image": "1.00000"}
	routing := &staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices: []modelhub.RoutePriceConfig{{
			RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true,
		}},
		ProviderModels: []modelhub.ProviderCandidate{{
			AccountModelID: 11, Provider: "openai", ModelCode: "gpt-image-2", SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"},
			SizeModes: []string{"ratio"}, SupportedAspectRatios: []string{"1:1"}, Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"}, MaxImageCount: 1,
		}},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 11, Enabled: true}},
	}}
	adminSvc := adminconfigservice.NewService(cfg)
	billingSvc := billingservice.NewService(cfg.Billing)
	billingSvc.SetAdminConfigResolver(adminSvc)
	billingSvc.SetModelRoutingSource(routing)
	seedBalance(t, billingSvc, 92, "20.00000")
	estimate, err := billingSvc.Estimate(domainbilling.EstimateRequest{
		TaskType: "text_to_image", RouteModelCode: "plus", SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "1:1",
		Quality: "auto", OutputFormat: "png", Moderation: "auto", RequestedOutputImageCount: 1,
	})
	if err != nil || estimate.CapabilityVersion == "" {
		t.Fatalf("Estimate = %#v, %v", estimate, err)
	}
	if _, err := adminSvc.UpdateTab(context.Background(), domainadminconfig.UpdateTabRequest{
		TabKey: "billing_pricing", Version: 1,
		Items: []domainadminconfig.Item{{
			ConfigCategory: "billing_pricing", ConfigKey: "task_multipliers",
			ConfigValue: map[string]any{"value": map[string]any{"text_to_image": "2.00000"}}, Scope: "global",
		}},
	}); err != nil {
		t.Fatalf("UpdateTab: %v", err)
	}
	svc := imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, nil, imagetask.NewMemoryStore(), nil, billingSvc)
	svc.SetModelRoutingSource(routing)
	_, err = svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID: 92, RouteModelCode: "plus", AbstractModel: "plus", TaskType: "text_to_image", Prompt: "changed price",
		SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "1:1", Quality: "auto", OutputFormat: "png", Moderation: "auto", OutputImageCount: 1,
		CapabilityVersion: estimate.CapabilityVersion,
	})
	var appErr *errs.Error
	if !errors.As(err, &appErr) || appErr.StatusCode != 409 || appErr.Code != modelhub.CodeCapabilityChanged {
		t.Fatalf("task multiplier drift error = %#v, want 409/%s", err, modelhub.CodeCapabilityChanged)
	}
}

func TestCreateTaskRetryAfterSaveFailureStillReservesPoints(t *testing.T) {
	cfg := taskTestConfig()
	billingSvc := billingservice.NewService(cfg.Billing)
	seedBalance(t, billingSvc, 78, "20.00000")

	store := &failingSaveStore{base: imagetask.NewMemoryStore(), failSave: true}
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, nil, store, nil, billingSvc))
	createReq := domainimagetask.CreateRequest{
		TaskID:              "77777777-7777-7777-7777-777777777777",
		UserID:              78,
		UserGroupCode:       "basic",
		UserGroupMultiplier: "1.00000",
		AbstractModel:       "plus",
		TaskType:            string(provider.TaskTypeTextToImage),
		Prompt:              "Retry me",
		RequestedSize:       "auto",
		BaseResolution:      "auto",
		OutputImageCount:    1,
	}

	if _, err := svc.CreateTask(context.Background(), createReq); err == nil {
		t.Fatal("expected first CreateTask to fail when store save fails")
	}
	created, err := svc.CreateTask(context.Background(), createReq)
	if err != nil {
		t.Fatalf("second CreateTask: %v", err)
	}
	if created.ID != createReq.TaskID {
		t.Fatalf("expected retry to reuse task id %s, got %#v", createReq.TaskID, created)
	}

	summary, err := billingSvc.GetBalance(context.Background(), 78, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if summary.AvailablePoints != "12.00000" || summary.FrozenPoints != "8.00000" {
		t.Fatalf("expected retry to reserve points once, got %#v", summary)
	}
}

func TestCreateTaskPersistsFailedCallRecordWhenReserveRejectsInsufficientBalance(t *testing.T) {
	cfg := taskTestConfig()
	billingSvc := billingservice.NewService(cfg.Billing)
	store := imagetask.NewMemoryStore()
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, nil, store, nil, billingSvc))

	_, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		TaskID:              "99999999-9999-4999-8999-999999999999",
		UserID:              190,
		UserGroupCode:       "basic",
		UserGroupMultiplier: "1.00000",
		AbstractModel:       "plus",
		TaskType:            string(provider.TaskTypeTextToImage),
		Prompt:              "Record rejected task",
		RequestedSize:       "auto",
		BaseResolution:      "auto",
		OutputImageCount:    1,
	})
	if err == nil {
		t.Fatal("expected CreateTask to reject insufficient balance")
	}

	tasks, err := svc.ListByUser(context.Background(), 190)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one failed call record task, got %#v", tasks)
	}
	record := tasks[0]
	if record.Status != domainimagetask.StatusFailed {
		t.Fatalf("expected failed record, got %#v", record)
	}
	if record.ErrorCode != errs.CodeInsufficientPoints || record.ErrorMessage == "" {
		t.Fatalf("expected insufficient points error on record, got code=%q message=%q", record.ErrorCode, record.ErrorMessage)
	}
	if record.EstimatedPoints != "8.00000" || record.ActualPoints != "0.00000" {
		t.Fatalf("expected estimate snapshot on failed record, got estimated=%q actual=%q", record.EstimatedPoints, record.ActualPoints)
	}
}

func TestCreateTaskPersistsFailedCallRecordWhenRoutePriceMissing(t *testing.T) {
	cfg := taskTestConfig()
	store := imagetask.NewMemoryStore()
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersAndStore(cfg, nil, store))
	svc.SetModelRoutingSource(&staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		Version:     "route-snapshot-price-missing",
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		ProviderModels: []modelhub.ProviderCandidate{
			{AccountModelID: 12, ModelAccountID: 102, ModelCode: "gpt-image-1", SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"}},
		},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Priority: 1, Enabled: true}},
	}})

	_, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		TaskID:              "88888888-8888-4888-8888-888888888888",
		UserID:              191,
		UserGroupCode:       "basic",
		UserGroupMultiplier: "1.00000",
		RouteModelCode:      "plus",
		TaskType:            string(provider.TaskTypeTextToImage),
		Prompt:              "Record missing route price",
		RequestedSize:       "1024x1024",
		BaseResolution:      "1k",
		OutputImageCount:    1,
	})
	if err == nil {
		t.Fatal("expected CreateTask to reject missing route price")
	}
	var appErr *errs.Error
	if !errors.As(err, &appErr) || appErr.Code != errs.CodeRouteModelPriceMissing {
		t.Fatalf("expected route model price missing error, got %#v", err)
	}

	tasks, err := svc.ListByUser(context.Background(), 191)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one failed call record task, got %#v", tasks)
	}
	record := tasks[0]
	if record.Status != domainimagetask.StatusFailed {
		t.Fatalf("expected failed record, got %#v", record)
	}
	if record.ErrorCode != errs.CodeRouteModelPriceMissing || record.ErrorMessage == "" {
		t.Fatalf("expected route price missing error on record, got code=%q message=%q", record.ErrorCode, record.ErrorMessage)
	}
	if record.RouteModelCode != "plus" || record.RouteModelID != 1 || record.RouteSnapshotVersion != "route-snapshot-price-missing" || record.EstimatedPoints != "0.00000" || record.ActualPoints != "0.00000" {
		t.Fatalf("expected route metadata and zero point snapshot on failed record, got %#v", record)
	}
}

func TestCreateTaskPersistsFailedCallRecordWhenRouteHasNoCandidate(t *testing.T) {
	cfg := taskTestConfig()
	store := imagetask.NewMemoryStore()
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersAndStore(cfg, nil, store))
	svc.SetModelRoutingSource(&staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		Version:     "route-snapshot-no-candidate",
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices:      []modelhub.RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "8.00000", Enabled: true}},
		ProviderModels: []modelhub.ProviderCandidate{
			{AccountModelID: 12, ModelAccountID: 102, ModelCode: "gpt-image-1", SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"}},
		},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Priority: 1, Enabled: false}},
	}})

	_, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		TaskID:              "77777777-7777-4777-8777-777777777777",
		UserID:              192,
		UserGroupCode:       "basic",
		UserGroupMultiplier: "1.00000",
		RouteModelCode:      "plus",
		TaskType:            string(provider.TaskTypeTextToImage),
		Prompt:              "Record missing route candidate",
		RequestedSize:       "1024x1024",
		BaseResolution:      "1k",
		OutputImageCount:    1,
	})
	if err == nil {
		t.Fatal("expected CreateTask to reject route without candidate")
	}
	var appErr *errs.Error
	if !errors.As(err, &appErr) || appErr.Code != errs.CodeImageCapabilityMismatch {
		t.Fatalf("expected route model no candidate error, got %#v", err)
	}

	tasks, err := svc.ListByUser(context.Background(), 192)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one failed call record task, got %#v", tasks)
	}
	record := tasks[0]
	if record.Status != domainimagetask.StatusFailed {
		t.Fatalf("expected failed record, got %#v", record)
	}
	if record.ErrorCode != errs.CodeImageCapabilityMismatch || record.ErrorMessage == "" {
		t.Fatalf("expected no candidate error on record, got code=%q message=%q", record.ErrorCode, record.ErrorMessage)
	}
	if record.RouteModelCode != "plus" || record.RouteModelID != 1 || record.RouteSnapshotVersion != "route-snapshot-no-candidate" || record.BaseResolution != "1k" || record.EstimatedPoints != "0.00000" || record.ActualPoints != "0.00000" {
		t.Fatalf("expected route metadata, quality, and zero point snapshot on failed record, got %#v", record)
	}
}

func TestExecuteLeasedTaskSettlesPartialSuccessAgainstReservedEstimate(t *testing.T) {
	cfg := taskTestConfig()
	billingSvc := billingservice.NewService(cfg.Billing)
	seedBalance(t, billingSvc, 81, "100.00000")

	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			return provider.ImageResponse{Created: 1770000010, Data: []provider.ImageResult{{URL: "https://cdn.example.com/partial.png"}}}, nil
		}},
	}
	store := imagetask.NewMemoryStore()
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, providers, store, nil, billingSvc))

	created, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:              81,
		UserGroupCode:       "basic",
		UserGroupMultiplier: "1.00000",
		AbstractModel:       "plus",
		TaskType:            string(provider.TaskTypeTextToImage),
		Prompt:              "Need two images",
		RequestedSize:       "auto",
		BaseResolution:      "auto",
		OutputImageCount:    2,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if created.EstimatedPoints != "16.00000" {
		t.Fatalf("expected estimate 16.00000, got %#v", created)
	}

	claimed, ok, err := svc.AcquireNextTask(context.Background(), "worker-a", 30*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextTask: %v", err)
	}
	if !ok {
		t.Fatal("expected task claim to succeed")
	}

	result, err := svc.ExecuteLeasedTask(context.Background(), claimed, "worker-a", []string{"openrouter"})
	if err != nil {
		t.Fatalf("ExecuteLeasedTask: %v", err)
	}
	if result.Task.Status != domainimagetask.StatusPartialFailed {
		t.Fatalf("expected partial_failed task, got %s", result.Task.Status)
	}
	if result.Task.ActualPoints != "8.00000" {
		t.Fatalf("expected actual points 8.00000, got %#v", result.Task)
	}

	summary, err := billingSvc.GetBalance(context.Background(), 81, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if summary.AvailablePoints != "92.00000" || summary.FrozenPoints != "0.00000" {
		t.Fatalf("unexpected settled balance %#v", summary)
	}
}

func TestExecuteLeasedTaskRefundsReservedPointsOnFailure(t *testing.T) {
	cfg := taskTestConfig()
	billingSvc := billingservice.NewService(cfg.Billing)
	seedBalance(t, billingSvc, 82, "100.00000")

	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			return provider.ImageResponse{}, errors.New("provider failed")
		}},
	}
	store := imagetask.NewMemoryStore()
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, providers, store, nil, billingSvc))

	_, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:              82,
		UserGroupCode:       "basic",
		UserGroupMultiplier: "1.00000",
		AbstractModel:       "plus",
		TaskType:            string(provider.TaskTypeTextToImage),
		Prompt:              "Will fail",
		RequestedSize:       "auto",
		BaseResolution:      "auto",
		OutputImageCount:    1,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	claimed, ok, err := svc.AcquireNextTask(context.Background(), "worker-a", 30*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextTask: %v", err)
	}
	if !ok {
		t.Fatal("expected task claim to succeed")
	}

	if _, err := svc.ExecuteLeasedTask(context.Background(), claimed, "worker-a", []string{"openrouter"}); err == nil {
		t.Fatal("expected ExecuteLeasedTask failure")
	}

	summary, err := billingSvc.GetBalance(context.Background(), 82, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if summary.AvailablePoints != "100.00000" || summary.FrozenPoints != "0.00000" {
		t.Fatalf("expected full refund on failure, got %#v", summary)
	}
}

func TestExecuteLeasedTaskSettlesBillingWhenFirstOwnedSaveConflictsOnSuccess(t *testing.T) {
	cfg := taskTestConfig()
	billingSvc := billingservice.NewService(cfg.Billing)
	seedBalance(t, billingSvc, 83, "20.00000")

	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			return provider.ImageResponse{Created: 1770000011, Data: []provider.ImageResult{{URL: "https://cdn.example.com/conflict-success.png"}}}, nil
		}},
	}
	store := &failingSaveStore{base: imagetask.NewMemoryStore()}
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, providers, store, nil, billingSvc))

	_, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:              83,
		UserGroupCode:       "basic",
		UserGroupMultiplier: "1.00000",
		AbstractModel:       "plus",
		TaskType:            string(provider.TaskTypeTextToImage),
		Prompt:              "Will hit save conflict",
		RequestedSize:       "auto",
		BaseResolution:      "auto",
		OutputImageCount:    1,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	claimed, ok, err := svc.AcquireNextTask(context.Background(), "worker-a", 30*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextTask: %v", err)
	}
	if !ok {
		t.Fatal("expected task claim to succeed")
	}

	store.failSaveIfOwned = true
	store.failSaveIfOwnedError = repoerr.ErrConflict
	result, err := svc.ExecuteLeasedTask(context.Background(), claimed, "worker-a", []string{"openrouter"})
	if err != nil {
		t.Fatalf("ExecuteLeasedTask: %v", err)
	}
	if result.Task.Status != domainimagetask.StatusSucceeded {
		t.Fatalf("expected recovered succeeded result, got %#v", result.Task)
	}

	summary, err := billingSvc.GetBalance(context.Background(), 83, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if summary.AvailablePoints != "12.00000" || summary.FrozenPoints != "0.00000" {
		t.Fatalf("expected billing settlement despite save conflict, got %#v", summary)
	}
}

func TestExecuteLeasedTaskSettlesBillingWhenFirstOwnedSaveConflictsOnFailure(t *testing.T) {
	cfg := taskTestConfig()
	billingSvc := billingservice.NewService(cfg.Billing)
	seedBalance(t, billingSvc, 84, "20.00000")

	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			return provider.ImageResponse{}, errors.New("provider failed")
		}},
	}
	store := &failingSaveStore{base: imagetask.NewMemoryStore()}
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, providers, store, nil, billingSvc))

	_, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:              84,
		UserGroupCode:       "basic",
		UserGroupMultiplier: "1.00000",
		AbstractModel:       "plus",
		TaskType:            string(provider.TaskTypeTextToImage),
		Prompt:              "Will fail with save conflict",
		RequestedSize:       "auto",
		BaseResolution:      "auto",
		OutputImageCount:    1,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	claimed, ok, err := svc.AcquireNextTask(context.Background(), "worker-a", 30*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextTask: %v", err)
	}
	if !ok {
		t.Fatal("expected task claim to succeed")
	}

	store.failSaveIfOwned = true
	store.failSaveIfOwnedError = repoerr.ErrConflict
	if _, err := svc.ExecuteLeasedTask(context.Background(), claimed, "worker-a", []string{"openrouter"}); err == nil {
		t.Fatal("expected ExecuteLeasedTask to surface lease conflict")
	}

	summary, err := billingSvc.GetBalance(context.Background(), 84, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if summary.AvailablePoints != "20.00000" || summary.FrozenPoints != "0.00000" {
		t.Fatalf("expected refund settlement despite save conflict, got %#v", summary)
	}
}

func TestExecuteLeasedTaskDurablyPersistsSuccessAfterFirstOwnedSaveConflict(t *testing.T) {
	cfg := taskTestConfig()
	billingSvc := billingservice.NewService(cfg.Billing)
	seedBalance(t, billingSvc, 185, "20.00000")

	generateCalls := 0
	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			generateCalls++
			return provider.ImageResponse{Created: 1770000123, Data: []provider.ImageResult{{URL: "https://cdn.example.com/conflict-success.png"}}}, nil
		}},
	}
	store := &failingSaveStore{base: imagetask.NewMemoryStore()}
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, providers, store, nil, billingSvc))

	created, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:              185,
		UserGroupCode:       "basic",
		UserGroupMultiplier: "1.00000",
		AbstractModel:       "plus",
		TaskType:            string(provider.TaskTypeTextToImage),
		Prompt:              "Persist successful terminal snapshot",
		RequestedSize:       "auto",
		BaseResolution:      "auto",
		OutputImageCount:    1,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	claimed, ok, err := svc.AcquireNextTask(context.Background(), "worker-a", 30*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextTask: %v", err)
	}
	if !ok {
		t.Fatal("expected task claim to succeed")
	}

	store.failSaveIfOwned = true
	store.failSaveIfOwnedError = repoerr.ErrConflict
	result, err := svc.ExecuteLeasedTask(context.Background(), claimed, "worker-a", []string{"openrouter"})
	if err != nil {
		t.Fatalf("ExecuteLeasedTask: %v", err)
	}
	if result.Task.Status != domainimagetask.StatusSucceeded {
		t.Fatalf("expected recovered succeeded result, got %#v", result.Task)
	}

	persisted, err := svc.GetByID(context.Background(), 185, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if persisted.Status != domainimagetask.StatusSucceeded && len(persisted.Results) == 0 {
		t.Fatalf("expected persisted success marker after conflict, got %#v", persisted)
	}

	if persisted.Status == domainimagetask.StatusRunning {
		reclaimed, ok, err := svc.AcquireNextTask(context.Background(), "worker-b", 30*time.Second)
		if err != nil {
			t.Fatalf("AcquireNextTask reclaim: %v", err)
		}
		if !ok {
			t.Fatal("expected persisted running task to be reclaimable for terminalization")
		}
		if _, err := svc.ExecuteLeasedTask(context.Background(), reclaimed, "worker-b", []string{"openrouter"}); err != nil {
			t.Fatalf("ExecuteLeasedTask reclaim: %v", err)
		}
	}

	if generateCalls != 1 {
		t.Fatalf("expected provider generate to run once, got %d", generateCalls)
	}

	finalTask, err := svc.GetByID(context.Background(), 185, created.ID)
	if err != nil {
		t.Fatalf("GetByID final: %v", err)
	}
	if finalTask.Status != domainimagetask.StatusSucceeded || len(finalTask.Results) != 1 {
		t.Fatalf("expected succeeded task after recovery, got %#v", finalTask)
	}

	summary, err := billingSvc.GetBalance(context.Background(), 185, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if summary.AvailablePoints != "12.00000" || summary.FrozenPoints != "0.00000" {
		t.Fatalf("expected settled balance after recovery, got %#v", summary)
	}
}

func TestExecuteLeasedTaskDurablyPersistsFailureAfterFirstOwnedSaveConflict(t *testing.T) {
	cfg := taskTestConfig()
	billingSvc := billingservice.NewService(cfg.Billing)
	seedBalance(t, billingSvc, 186, "20.00000")

	generateCalls := 0
	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			generateCalls++
			return provider.ImageResponse{}, errors.New("provider failed permanently")
		}},
	}
	store := &failingSaveStore{base: imagetask.NewMemoryStore()}
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, providers, store, nil, billingSvc))

	created, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:              186,
		UserGroupCode:       "basic",
		UserGroupMultiplier: "1.00000",
		AbstractModel:       "plus",
		TaskType:            string(provider.TaskTypeTextToImage),
		Prompt:              "Persist failed terminal snapshot",
		RequestedSize:       "auto",
		BaseResolution:      "auto",
		OutputImageCount:    1,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	claimed, ok, err := svc.AcquireNextTask(context.Background(), "worker-a", 30*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextTask: %v", err)
	}
	if !ok {
		t.Fatal("expected task claim to succeed")
	}

	store.failSaveIfOwned = true
	store.failSaveIfOwnedError = repoerr.ErrConflict
	if _, err := svc.ExecuteLeasedTask(context.Background(), claimed, "worker-a", []string{"openrouter"}); err == nil {
		t.Fatal("expected ExecuteLeasedTask to surface lease conflict")
	}

	persisted, err := svc.GetByID(context.Background(), 186, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if persisted.Status != domainimagetask.StatusFailed && persisted.ErrorCode == "" && persisted.ErrorMessage == "" {
		t.Fatalf("expected persisted failure marker after conflict, got %#v", persisted)
	}

	if persisted.Status == domainimagetask.StatusRunning {
		reclaimed, ok, err := svc.AcquireNextTask(context.Background(), "worker-b", 30*time.Second)
		if err != nil {
			t.Fatalf("AcquireNextTask reclaim: %v", err)
		}
		if !ok {
			t.Fatal("expected persisted running task to be reclaimable for terminalization")
		}
		if _, err := svc.ExecuteLeasedTask(context.Background(), reclaimed, "worker-b", []string{"openrouter"}); err == nil {
			t.Fatal("expected reclaimed terminal failure to return an error")
		}
	}

	if generateCalls != 1 {
		t.Fatalf("expected provider generate to run once, got %d", generateCalls)
	}

	finalTask, err := svc.GetByID(context.Background(), 186, created.ID)
	if err != nil {
		t.Fatalf("GetByID final: %v", err)
	}
	if finalTask.Status != domainimagetask.StatusFailed || finalTask.ErrorMessage == "" {
		t.Fatalf("expected failed task after recovery, got %#v", finalTask)
	}

	summary, err := billingSvc.GetBalance(context.Background(), 186, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if summary.AvailablePoints != "20.00000" || summary.FrozenPoints != "0.00000" {
		t.Fatalf("expected refunded balance after recovery, got %#v", summary)
	}
}

func TestExecuteLeasedTaskDoesNotOverwriteReclaimedOwner(t *testing.T) {
	cfg := taskTestConfig()
	billingSvc := billingservice.NewService(cfg.Billing)
	seedBalance(t, billingSvc, 187, "20.00000")

	var generateMu sync.Mutex
	generateCalls := 0
	firstProviderRelease := make(chan struct{})
	secondProviderStarted := make(chan struct{})
	secondProviderRelease := make(chan struct{})
	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			generateMu.Lock()
			generateCalls++
			callNumber := generateCalls
			generateMu.Unlock()
			if callNumber == 2 {
				close(secondProviderStarted)
			}
			gate := firstProviderRelease
			if callNumber == 2 {
				gate = secondProviderRelease
			}
			select {
			case <-ctx.Done():
				return provider.ImageResponse{}, ctx.Err()
			case <-gate:
			}
			url := "https://cdn.example.com/stale-worker.png"
			revisedPrompt := "stale-worker"
			if callNumber == 2 {
				url = "https://cdn.example.com/reclaimed-worker.png"
				revisedPrompt = "reclaimed-worker"
			}
			return provider.ImageResponse{Created: 1770000456, Data: []provider.ImageResult{{URL: url, RevisedPrompt: revisedPrompt}}}, nil
		}},
	}
	store := &raceyTerminalStore{
		base:                   imagetask.NewMemoryStore(),
		blockFirstTerminalSave: true,
		terminalSaveEntered:    make(chan struct{}),
		releaseTerminalSave:    make(chan struct{}),
	}
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, providers, store, nil, billingSvc))

	created, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:              187,
		UserGroupCode:       "basic",
		UserGroupMultiplier: "1.00000",
		AbstractModel:       "plus",
		TaskType:            string(provider.TaskTypeTextToImage),
		Prompt:              "Recover reclaim race without duplicate provider call",
		RequestedSize:       "auto",
		BaseResolution:      "auto",
		OutputImageCount:    1,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	claimed, ok, err := svc.AcquireNextTask(context.Background(), "worker-a", 20*time.Millisecond)
	if err != nil {
		t.Fatalf("AcquireNextTask: %v", err)
	}
	if !ok {
		t.Fatal("expected task claim to succeed")
	}

	firstDone := make(chan struct {
		result domainimagetask.ExecuteResult
		err    error
	}, 1)
	go func() {
		result, execErr := svc.ExecuteLeasedTask(context.Background(), claimed, "worker-a", []string{"openrouter"})
		firstDone <- struct {
			result domainimagetask.ExecuteResult
			err    error
		}{result: result, err: execErr}
	}()

	time.Sleep(30 * time.Millisecond)
	close(firstProviderRelease)

	select {
	case <-store.terminalSaveEntered:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected first worker to block before persisting terminal snapshot")
	}

	reclaimed, ok, err := svc.AcquireNextTask(context.Background(), "worker-b", 30*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextTask reclaim: %v", err)
	}
	if !ok {
		t.Fatal("expected second worker to reclaim expired task")
	}

	secondDone := make(chan struct {
		result domainimagetask.ExecuteResult
		err    error
	}, 1)
	go func() {
		result, execErr := svc.ExecuteLeasedTask(context.Background(), reclaimed, "worker-b", []string{"openrouter"})
		secondDone <- struct {
			result domainimagetask.ExecuteResult
			err    error
		}{result: result, err: execErr}
	}()
	select {
	case <-secondProviderStarted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected reclaimed worker to start provider generation")
	}

	close(store.releaseTerminalSave)

	firstOutcome := <-firstDone
	var conflictErr *errs.Error
	if firstOutcome.err == nil || !errors.As(firstOutcome.err, &conflictErr) || conflictErr.Code != errs.CodeConflict {
		t.Fatalf("expected stale worker lease conflict, got result=%#v err=%v", firstOutcome.result, firstOutcome.err)
	}

	duringReclaim, err := svc.GetByID(context.Background(), 187, created.ID)
	if err != nil {
		t.Fatalf("GetByID during reclaim: %v", err)
	}
	if duringReclaim.Status != domainimagetask.StatusRunning || duringReclaim.LeaseOwner != "worker-b" || duringReclaim.ProgressStage != domainimagetask.ProgressStageProvider {
		t.Fatalf("stale worker must not overwrite reclaimed task, got %#v", duringReclaim)
	}

	close(secondProviderRelease)
	secondOutcome := <-secondDone
	if secondOutcome.err != nil {
		t.Fatalf("second ExecuteLeasedTask: %v", secondOutcome.err)
	}
	if secondOutcome.result.Task.Status != domainimagetask.StatusSucceeded {
		t.Fatalf("expected reclaimed worker to succeed, got %#v", secondOutcome.result.Task)
	}
	generateMu.Lock()
	finalGenerateCalls := generateCalls
	generateMu.Unlock()
	if finalGenerateCalls != 2 {
		t.Fatalf("expected each lease owner to make its own provider call after expiry, got %d", finalGenerateCalls)
	}

	finalTask, err := svc.GetByID(context.Background(), 187, created.ID)
	if err != nil {
		t.Fatalf("GetByID final: %v", err)
	}
	if finalTask.Status != domainimagetask.StatusSucceeded || len(finalTask.Results) != 1 || finalTask.Results[0].RevisedPrompt != "reclaimed-worker" {
		t.Fatalf("expected succeeded task after reclaim race recovery, got %#v", finalTask)
	}
}

func TestExecuteLeasedTaskRejectsStaleWorkerAfterReclaim(t *testing.T) {
	cfg := taskTestConfig()
	billingSvc := billingservice.NewService(cfg.Billing)
	seedBalance(t, billingSvc, 188, "20.00000")

	generateCalls := 0
	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			generateCalls++
			return provider.ImageResponse{Created: 1770000789, Data: []provider.ImageResult{{URL: "https://cdn.example.com/reclaim-owner.png"}}}, nil
		}},
	}
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, providers, imagetask.NewMemoryStore(), nil, billingSvc))

	created, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:              188,
		UserGroupCode:       "basic",
		UserGroupMultiplier: "1.00000",
		AbstractModel:       "plus",
		TaskType:            string(provider.TaskTypeTextToImage),
		Prompt:              "Only the reclaimed worker may call provider",
		RequestedSize:       "auto",
		BaseResolution:      "auto",
		OutputImageCount:    1,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	claimedA, ok, err := svc.AcquireNextTask(context.Background(), "worker-a", time.Nanosecond)
	if err != nil {
		t.Fatalf("AcquireNextTask worker-a: %v", err)
	}
	if !ok {
		t.Fatal("expected worker-a to claim the task")
	}
	time.Sleep(time.Millisecond)

	claimedB, ok, err := svc.AcquireNextTask(context.Background(), "worker-b", 30*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextTask worker-b: %v", err)
	}
	if !ok {
		t.Fatal("expected worker-b to reclaim the expired task")
	}

	if _, err := svc.ExecuteLeasedTask(context.Background(), claimedA, "worker-a", []string{"openrouter"}); err == nil {
		t.Fatal("expected stale worker execution to fail with lease conflict")
	}
	if generateCalls != 0 {
		t.Fatalf("expected stale worker to avoid provider call, got %d calls", generateCalls)
	}

	result, err := svc.ExecuteLeasedTask(context.Background(), claimedB, "worker-b", []string{"openrouter"})
	if err != nil {
		t.Fatalf("ExecuteLeasedTask worker-b: %v", err)
	}
	if result.Task.Status != domainimagetask.StatusSucceeded {
		t.Fatalf("expected reclaimed worker to succeed, got %#v", result.Task)
	}
	if generateCalls != 1 {
		t.Fatalf("expected exactly one provider call from reclaimed worker, got %d", generateCalls)
	}

	finalTask, err := svc.GetByID(context.Background(), 188, created.ID)
	if err != nil {
		t.Fatalf("GetByID final: %v", err)
	}
	if finalTask.Status != domainimagetask.StatusSucceeded || len(finalTask.Results) != 1 {
		t.Fatalf("expected succeeded task after reclaim, got %#v", finalTask)
	}
}

func TestAcquireNextTaskClaimsAndReclaimsExpiredLease(t *testing.T) {
	cfg := taskTestConfig()
	store := imagetask.NewMemoryStore()
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersAndStore(cfg, nil, store))

	created, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:           41,
		AbstractModel:    "plus",
		TaskType:         string(provider.TaskTypeTextToImage),
		Prompt:           "Lease me",
		RequestedSize:    "auto",
		BaseResolution:   "auto",
		OutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	claimed, ok, err := svc.AcquireNextTask(context.Background(), "worker-a", 30*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextTask first claim: %v", err)
	}
	if !ok {
		t.Fatal("expected first claim to succeed")
	}
	if claimed.ID != created.ID {
		t.Fatalf("expected claimed task %s, got %s", created.ID, claimed.ID)
	}
	if claimed.Status != domainimagetask.StatusRunning {
		t.Fatalf("expected running status after claim, got %s", claimed.Status)
	}
	if claimed.LeaseOwner != "worker-a" || claimed.LeaseExpiresAt == nil {
		t.Fatalf("expected active lease for worker-a, got owner=%q lease=%v", claimed.LeaseOwner, claimed.LeaseExpiresAt)
	}

	_, ok, err = svc.AcquireNextTask(context.Background(), "worker-b", 30*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextTask second claim: %v", err)
	}
	if ok {
		t.Fatal("expected second claim to fail while lease is active")
	}

	expiredAt := time.Now().Add(-time.Minute)
	claimed.LeaseExpiresAt = &expiredAt
	if err := store.Save(context.Background(), claimed); err != nil {
		t.Fatalf("Save expired lease: %v", err)
	}

	reclaimed, ok, err := svc.AcquireNextTask(context.Background(), "worker-b", 45*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextTask reclaim: %v", err)
	}
	if !ok {
		t.Fatal("expected reclaimed task after lease expiry")
	}
	if reclaimed.ID != created.ID {
		t.Fatalf("expected reclaimed task %s, got %s", created.ID, reclaimed.ID)
	}
	if reclaimed.LeaseOwner != "worker-b" || reclaimed.LeaseExpiresAt == nil {
		t.Fatalf("expected worker-b to own reclaimed lease, got owner=%q lease=%v", reclaimed.LeaseOwner, reclaimed.LeaseExpiresAt)
	}
}

func TestHeartbeatTaskExtendsLeaseForOwner(t *testing.T) {
	cfg := taskTestConfig()
	store := imagetask.NewMemoryStore()
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersAndStore(cfg, nil, store))

	_, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:           51,
		AbstractModel:    "plus",
		TaskType:         string(provider.TaskTypeTextToImage),
		Prompt:           "Keep alive",
		RequestedSize:    "auto",
		BaseResolution:   "auto",
		OutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	claimed, ok, err := svc.AcquireNextTask(context.Background(), "worker-a", 20*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextTask: %v", err)
	}
	if !ok || claimed.LeaseExpiresAt == nil {
		t.Fatalf("expected claimed task with lease, got ok=%v task=%#v", ok, claimed)
	}
	firstExpiry := *claimed.LeaseExpiresAt

	renewed, err := svc.HeartbeatTask(context.Background(), claimed.ID, "worker-a", 60*time.Second)
	if err != nil {
		t.Fatalf("HeartbeatTask: %v", err)
	}
	if renewed.LeaseExpiresAt == nil || !renewed.LeaseExpiresAt.After(firstExpiry) {
		t.Fatalf("expected renewed lease expiry after %v, got %v", firstExpiry, renewed.LeaseExpiresAt)
	}

	if _, err := svc.HeartbeatTask(context.Background(), claimed.ID, "worker-b", 60*time.Second); err == nil {
		t.Fatal("expected non-owner heartbeat to fail")
	}
}

func TestMemoryStoreSaveIfOwnedRejectsStaleWorkerWriteAfterReclaim(t *testing.T) {
	store := imagetask.NewMemoryStore()
	now := time.Now().UTC()
	task := domainimagetask.Task{
		UserID:        61,
		ID:            "stale-worker-task",
		Status:        domainimagetask.StatusQueued,
		AbstractModel: "plus",
		TaskType:      string(provider.TaskTypeTextToImage),
		Prompt:        "stale write",

		BaseResolution:   "2k",
		RequestedSize:    "auto",
		OutputImageCount: 1,
	}
	if err := store.Save(context.Background(), task); err != nil {
		t.Fatalf("Save: %v", err)
	}

	claimedByA, err := store.AcquireNextQueuedTask(context.Background(), "worker-a", now, 30*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextQueuedTask worker-a: %v", err)
	}
	staleSnapshot := claimedByA

	expiredAt := now.Add(-time.Minute)
	claimedByA.LeaseExpiresAt = &expiredAt
	if err := store.Save(context.Background(), claimedByA); err != nil {
		t.Fatalf("Save expired lease: %v", err)
	}
	if _, err := store.AcquireNextQueuedTask(context.Background(), "worker-b", now.Add(2*time.Second), 30*time.Second); err != nil {
		t.Fatalf("AcquireNextQueuedTask worker-b: %v", err)
	}

	staleSnapshot.Status = domainimagetask.StatusSucceeded
	staleSnapshot.LeaseOwner = ""
	staleSnapshot.LeaseExpiresAt = nil
	staleSnapshot.Results = []provider.ImageResult{{URL: "https://cdn.example.com/stale.png"}}
	if err := store.SaveIfOwned(context.Background(), staleSnapshot, "worker-a", now.Add(3*time.Second)); err == nil {
		t.Fatal("expected stale worker write to be rejected")
	}
}

func TestMemoryStoreSaveIfOwnedPreservesRenewedLeaseForRunningTask(t *testing.T) {
	store := imagetask.NewMemoryStore()
	now := time.Now().UTC()
	task := domainimagetask.Task{
		UserID:        62,
		ID:            "renewed-lease-task",
		Status:        domainimagetask.StatusQueued,
		AbstractModel: "plus",
		TaskType:      string(provider.TaskTypeTextToImage),
		Prompt:        "keep latest lease",

		BaseResolution:   "2k",
		RequestedSize:    "auto",
		OutputImageCount: 1,
	}
	if err := store.Save(context.Background(), task); err != nil {
		t.Fatalf("Save: %v", err)
	}

	claimed, err := store.AcquireNextQueuedTask(context.Background(), "worker-a", now, 20*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextQueuedTask: %v", err)
	}
	renewed, err := store.RenewTaskLease(context.Background(), claimed.ID, "worker-a", now.Add(5*time.Second), 45*time.Second)
	if err != nil {
		t.Fatalf("RenewTaskLease: %v", err)
	}

	staleRunningSnapshot := claimed
	staleRunningSnapshot.Attempts = []domainimagetask.Attempt{{Provider: "openrouter", Status: domainimagetask.StatusFailed, Error: "retry me"}}
	if err := store.SaveIfOwned(context.Background(), staleRunningSnapshot, "worker-a", now.Add(6*time.Second)); err != nil {
		t.Fatalf("SaveIfOwned: %v", err)
	}

	loaded, err := store.GetByID(context.Background(), 62, claimed.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if loaded.LeaseExpiresAt == nil || renewed.LeaseExpiresAt == nil {
		t.Fatalf("expected persisted lease expiry, got task=%#v renewed=%#v", loaded, renewed)
	}
	if !loaded.LeaseExpiresAt.Equal(*renewed.LeaseExpiresAt) {
		t.Fatalf("expected latest renewed lease %v, got %v", renewed.LeaseExpiresAt, loaded.LeaseExpiresAt)
	}
}

func TestExecuteLeasedTaskProcessesClaimedTaskWithoutCreatingNewID(t *testing.T) {
	cfg := taskTestConfig()
	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			return provider.ImageResponse{Created: 1770000003, Data: []provider.ImageResult{{URL: "https://cdn.example.com/leased.png"}}}, nil
		}},
	}
	store := imagetask.NewMemoryStore()
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersAndStore(cfg, providers, store))

	created, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:           71,
		AbstractModel:    "plus",
		TaskType:         string(provider.TaskTypeTextToImage),
		Prompt:           "Process leased task",
		RequestedSize:    "auto",
		BaseResolution:   "auto",
		OutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	claimed, ok, err := svc.AcquireNextTask(context.Background(), "worker-a", 30*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextTask: %v", err)
	}
	if !ok {
		t.Fatal("expected task claim to succeed")
	}

	result, err := svc.ExecuteLeasedTask(context.Background(), claimed, "worker-a", []string{"openrouter"})
	if err != nil {
		t.Fatalf("ExecuteLeasedTask: %v", err)
	}
	if result.Task.ID != created.ID {
		t.Fatalf("expected leased execution to keep task ID %s, got %s", created.ID, result.Task.ID)
	}
	if result.Task.Status != domainimagetask.StatusSucceeded {
		t.Fatalf("expected succeeded task, got %s", result.Task.Status)
	}
	if len(result.Task.Results) != 1 || result.Task.Results[0].URL == "" {
		t.Fatalf("expected persisted results, got %#v", result.Task.Results)
	}
}

func TestExecuteLeasedTaskPersistsRealProgressStages(t *testing.T) {
	cfg := taskTestConfig()
	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			return provider.ImageResponse{Created: 1770000003, Data: []provider.ImageResult{{URL: "https://cdn.example.com/progress.png"}}}, nil
		}},
	}
	store := &failingSaveStore{base: imagetask.NewMemoryStore()}
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersAndStore(cfg, providers, store))

	created, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:           73,
		AbstractModel:    "plus",
		TaskType:         string(provider.TaskTypeTextToImage),
		Prompt:           "Expose real progress",
		RequestedSize:    "auto",
		BaseResolution:   "auto",
		OutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if created.ProgressStage != "queued" || created.ProgressMessage == "" {
		t.Fatalf("expected queued progress on creation, got stage=%q message=%q", created.ProgressStage, created.ProgressMessage)
	}

	claimed, ok, err := svc.AcquireNextTask(context.Background(), "worker-progress", 30*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextTask: %v", err)
	}
	if !ok {
		t.Fatal("expected task claim to succeed")
	}
	result, err := svc.ExecuteLeasedTask(context.Background(), claimed, "worker-progress", []string{"openrouter"})
	if err != nil {
		t.Fatalf("ExecuteLeasedTask: %v", err)
	}
	if result.Task.ProgressStage != "completed" || result.Task.ProgressMessage == "" {
		t.Fatalf("expected completed progress, got stage=%q message=%q", result.Task.ProgressStage, result.Task.ProgressMessage)
	}

	want := []string{"queued", "provider", "persisting", "settling", "completed"}
	if got := store.progressHistory(); !reflect.DeepEqual(got, want) {
		t.Fatalf("expected real progress sequence %v, got %v", want, got)
	}
}

func TestExecuteLeasedTaskPersistsFailedProgressStage(t *testing.T) {
	cfg := taskTestConfig()
	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			return provider.ImageResponse{}, errors.New("provider failed")
		}},
	}
	store := &failingSaveStore{base: imagetask.NewMemoryStore()}
	svc := imagetask.NewServiceWithProvidersAndStore(cfg, providers, store)

	_, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:           74,
		AbstractModel:    "plus",
		TaskType:         string(provider.TaskTypeTextToImage),
		Prompt:           "Expose failed progress",
		RequestedSize:    "auto",
		BaseResolution:   "auto",
		OutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	claimed, ok, err := svc.AcquireNextTask(context.Background(), "worker-progress", 30*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextTask: %v", err)
	}
	if !ok {
		t.Fatal("expected task claim to succeed")
	}
	result, err := svc.ExecuteLeasedTask(context.Background(), claimed, "worker-progress", []string{"openrouter"})
	if err == nil {
		t.Fatal("expected provider failure")
	}
	if result.Task.ProgressStage != "failed" || result.Task.ProgressMessage == "" {
		t.Fatalf("expected failed progress, got stage=%q message=%q", result.Task.ProgressStage, result.Task.ProgressMessage)
	}

	want := []string{"queued", "provider", "settling", "failed"}
	if got := store.progressHistory(); !reflect.DeepEqual(got, want) {
		t.Fatalf("expected failed progress sequence %v, got %v", want, got)
	}
}

func TestExecuteLeasedTaskFanoutRecoversWhenLeaseExpiresBeforePersistingProgress(t *testing.T) {
	cfg := taskTestConfig()
	openaiCapability := cfg.Routing.ProviderCapabilities["openai"]
	openaiCapability.MaxImageCount = 1
	cfg.Routing.ProviderCapabilities["openai"] = openaiCapability
	billingSvc := billingservice.NewService(cfg.Billing)
	seedBalance(t, billingSvc, 75, "40.00000")

	providers := map[string]provider.ImageProvider{
		"openai": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			time.Sleep(5 * time.Millisecond)
			return provider.ImageResponse{Data: []provider.ImageResult{{B64JSON: tinyPNGBase64}}}, nil
		}},
	}
	store := imagetask.NewMemoryStore()
	svc := imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, providers, store, nil, billingSvc)

	_, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:              75,
		UserGroupCode:       "basic",
		UserGroupMultiplier: "1.00000",
		AbstractModel:       "plus",
		TaskType:            string(provider.TaskTypeTextToImage),
		Prompt:              "Recover fanout progress race",
		RequestedSize:       "auto",
		BaseResolution:      "auto",
		OutputImageCount:    2,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	claimed, ok, err := svc.AcquireNextTask(context.Background(), "worker-progress", time.Millisecond)
	if err != nil {
		t.Fatalf("AcquireNextTask: %v", err)
	}
	if !ok {
		t.Fatal("expected task claim to succeed")
	}

	result, err := svc.ExecuteLeasedTask(context.Background(), claimed, "worker-progress", []string{"openai"})
	if err != nil {
		t.Fatalf("expected terminal recovery after progress lease conflict, got %v", err)
	}
	if result.Task.Status != domainimagetask.StatusSucceeded || result.Task.ProgressStage != "completed" {
		t.Fatalf("expected recovered completed task, got %#v", result.Task)
	}
}

func TestExecuteLeasedTaskLoadsReferenceAssetsForEdit(t *testing.T) {
	cfg := taskTestConfig()
	loader := &fakeAssetLoader{
		inputs: map[string]provider.ImageInput{
			"asset-a": {Filename: "asset-a.png", MIMEType: "image/png", Data: []byte("asset-a")},
			"asset-b": {Filename: "asset-b.png", MIMEType: "image/png", Data: []byte("asset-b")},
		},
	}
	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{editFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			if len(req.ReferenceImages) != 2 {
				t.Fatalf("expected 2 reference images, got %d", len(req.ReferenceImages))
			}
			if req.ReferenceImages[0].Filename != "asset-a.png" || string(req.ReferenceImages[0].Data) != "asset-a" {
				t.Fatalf("unexpected first reference image %#v", req.ReferenceImages[0])
			}
			if req.ReferenceImages[1].Filename != "asset-b.png" || string(req.ReferenceImages[1].Data) != "asset-b" {
				t.Fatalf("unexpected second reference image %#v", req.ReferenceImages[1])
			}
			return provider.ImageResponse{Created: 1770000007, Data: []provider.ImageResult{{URL: "https://cdn.example.com/edit.png"}}}, nil
		}},
	}
	store := imagetask.NewMemoryStore()
	svc := withMockRemoteFetch(imagetask.NewServiceWithProvidersStoreAndAssets(cfg, providers, store, loader))

	created, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:              72,
		AbstractModel:       "plus",
		TaskType:            string(provider.TaskTypeImageEdit),
		Prompt:              "Edit with references",
		RequestedSize:       "auto",
		BaseResolution:      "auto",
		OutputImageCount:    1,
		ReferenceImageCount: 2,
		ReferenceAssetIDs:   []string{"asset-a", "asset-b"},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	claimed, ok, err := svc.AcquireNextTask(context.Background(), "worker-a", 30*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextTask: %v", err)
	}
	if !ok {
		t.Fatal("expected task claim to succeed")
	}

	result, err := svc.ExecuteLeasedTask(context.Background(), claimed, "worker-a", []string{"openrouter"})
	if err != nil {
		t.Fatalf("ExecuteLeasedTask: %v", err)
	}
	if result.Task.ID != created.ID {
		t.Fatalf("expected leased execution to keep task ID %s, got %s", created.ID, result.Task.ID)
	}
	if len(loader.calls) != 2 || loader.calls[0] != "asset-a" || loader.calls[1] != "asset-b" {
		t.Fatalf("expected loader to read both assets in order, got %#v", loader.calls)
	}
}
