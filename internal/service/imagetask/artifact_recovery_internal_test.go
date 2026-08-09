package imagetask

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/fatballfish/pic-gallery/internal/storage"
)

func TestArtifactRecoveryRetriesThreeTimesWithoutSecondProviderCall(t *testing.T) {
	svc, store, backend, providerCalls, now := newArtifactRecoveryTestService(t, 3)
	billing := &trackingArtifactBilling{}
	svc.billing = billing
	task, err := svc.CreateTask(context.Background(), artifactRecoveryCreateRequest())
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	for attempt := 1; attempt <= 4; attempt++ {
		claimed, ok, err := svc.AcquireNextTask(context.Background(), "worker", time.Minute)
		if err != nil || !ok {
			t.Fatalf("AcquireNextTask attempt %d: ok=%v err=%v", attempt, ok, err)
		}
		result, err := svc.ExecuteLeasedTask(context.Background(), claimed, "worker", []string{"openrouter"})
		if err != nil {
			t.Fatalf("ExecuteLeasedTask attempt %d: %v", attempt, err)
		}
		if attempt < 4 {
			if result.Task.ArtifactRecovery.Status != artifactRecoveryPending || result.Task.ArtifactRecovery.AttemptCount != attempt {
				t.Fatalf("expected pending recovery attempt %d, got %#v", attempt, result.Task.ArtifactRecovery)
			}
			if result.Task.ArtifactRecovery.LastDiagnostic.Code != "ARTIFACT_STORAGE_WRITE_FAILED" || result.Task.ArtifactRecovery.LastDiagnostic.Stage != "store" {
				t.Fatalf("expected detailed storage diagnostic, got %#v", result.Task.ArtifactRecovery.LastDiagnostic)
			}
			*now = result.Task.ArtifactRecovery.NextRetryAt.Add(time.Millisecond)
		} else if result.Task.Status != domainimagetask.StatusSucceeded {
			t.Fatalf("expected fourth persistence attempt to succeed, got %#v", result.Task)
		}
	}

	loaded, err := store.GetByID(context.Background(), 501, task.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if loaded.Status != domainimagetask.StatusSucceeded || len(loaded.Results) != 1 {
		t.Fatalf("expected persisted success, got %#v", loaded)
	}
	if loaded.ArtifactRecovery.Status != "completed" || loaded.ArtifactRecovery.EncryptedPayload != "" || len(loaded.ArtifactRecovery.Diagnostics) != 3 {
		t.Fatalf("expected completed recovery with three retained failure diagnostics, got %#v", loaded.ArtifactRecovery)
	}
	if *providerCalls != 1 || backend.putCalls != 4 {
		t.Fatalf("expected one provider call and four puts, provider=%d put=%d", *providerCalls, backend.putCalls)
	}
	if billing.finalizeCalls != 1 || billing.lastActualPoints != "1.00000" {
		t.Fatalf("expected one successful billing finalization, got %#v", billing)
	}
}

func TestArtifactRecoverySurvivesDefaultWriterFailureAfterProviderSuccess(t *testing.T) {
	svc, store, backend, providerCalls, now := newArtifactRecoveryTestService(t, 0)
	router := &failingDefaultWriterRouter{
		failuresRemaining: 1,
		ref: storage.BackendRef{
			ConfigID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
			Version:  7,
			Driver:   backend.Driver(),
			Backend:  backend,
		},
	}
	svc.router = router
	task, err := svc.CreateTask(context.Background(), artifactRecoveryCreateRequest())
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	claimed, ok, err := svc.AcquireNextTask(context.Background(), "worker-1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("AcquireNextTask: ok=%v err=%v", ok, err)
	}
	pending, err := svc.ExecuteLeasedTask(context.Background(), claimed, "worker-1", []string{"openrouter"})
	if err != nil {
		t.Fatalf("initial writer failure should enter automatic recovery: %v", err)
	}
	if pending.Task.ArtifactRecovery.Status != artifactRecoveryPending || pending.Task.ArtifactRecovery.AttemptCount != 1 || pending.Task.ArtifactRecovery.NextRetryAt == nil {
		t.Fatalf("expected first failed persistence attempt to be queued for recovery: %#v", pending.Task.ArtifactRecovery)
	}

	checkpoint, err := store.GetByID(context.Background(), task.UserID, task.ID)
	if err != nil {
		t.Fatalf("GetByID after writer failure: %v", err)
	}
	if checkpoint.ArtifactRecovery.EncryptedPayload == "" || checkpoint.UpstreamSucceededAt == nil {
		t.Fatalf("paid provider result must be durable before writer resolution: %#v", checkpoint)
	}
	if *providerCalls != 1 {
		t.Fatalf("expected one paid provider call, got %d", *providerCalls)
	}

	*now = pending.Task.ArtifactRecovery.NextRetryAt.Add(time.Millisecond)
	reclaimed, ok, err := svc.AcquireNextTask(context.Background(), "worker-2", time.Minute)
	if err != nil || !ok {
		t.Fatalf("reclaim task after writer failure: ok=%v err=%v", ok, err)
	}
	recovered, err := svc.ExecuteLeasedTask(context.Background(), reclaimed, "worker-2", []string{"openrouter"})
	if err != nil {
		t.Fatalf("recover paid result: %v", err)
	}
	if recovered.Task.Status != domainimagetask.StatusSucceeded || *providerCalls != 1 {
		t.Fatalf("expected recovery without provider replay, task=%#v calls=%d", recovered.Task, *providerCalls)
	}
	if recovered.Task.ArtifactRecovery.StorageConfigID != router.ref.ConfigID || recovered.Task.ArtifactRecovery.StorageVersion != router.ref.Version {
		t.Fatalf("expected resolved storage config to remain pinned, got %#v", recovered.Task.ArtifactRecovery)
	}
}

type failingDefaultWriterRouter struct {
	failuresRemaining int
	ref               storage.BackendRef
}

func (r *failingDefaultWriterRouter) DefaultWriter(context.Context) (storage.BackendRef, error) {
	if r.failuresRemaining > 0 {
		r.failuresRemaining--
		return storage.BackendRef{}, errors.New("default writer unavailable")
	}
	return r.ref, nil
}

func (r *failingDefaultWriterRouter) BackendFor(_ context.Context, configID string, _ string) (storage.BackendRef, error) {
	if configID != r.ref.ConfigID {
		return storage.BackendRef{}, fmt.Errorf("unexpected storage config %q", configID)
	}
	return r.ref, nil
}

func TestOpenAIFanoutCheckpointsAllPaidResultsBeforeArtifactRecovery(t *testing.T) {
	now := time.Date(2026, 7, 15, 13, 0, 0, 0, time.UTC)
	providerCalls := 0
	providers := map[string]provider.ImageProvider{"openai": artifactTestProvider{generate: func(_ context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
		providerCalls++
		results := make([]provider.ImageResult, 0, req.OutputImageCount)
		for index := 0; index < req.OutputImageCount; index++ {
			results = append(results, provider.ImageResult{URL: fmt.Sprintf("https://cdn.example.com/result-%d-%d.png?sig=secret", providerCalls, index)})
		}
		return provider.ImageResponse{ProviderRequestID: fmt.Sprintf("paid-fanout-%d", providerCalls), Data: results}, nil
	}}}
	backend := &countingArtifactBackend{failWrites: 1, objects: map[string][]byte{}}
	store := NewMemoryStore()
	cfg := artifactRecoveryTestConfig()
	cfg.GenerationLimits.MaxImageCount = 2
	cfg.Providers.OpenRouter.Enabled = false
	cfg.Providers.OpenAI.Enabled = true
	cfg.Routing.ProviderCapabilities = map[string]config.ProviderCapabilityConfig{"openai": {
		SupportedModels: []string{"basic"}, SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"}, Quality: []string{"auto"}, SupportedAspectRatios: []string{"1:1"}, MaxImageCount: 2, Priority: 1,
	}}
	cfg.Routing.ProviderModelMap = map[string]map[string]string{"basic": {"openai": "gpt-image"}}
	svc := NewServiceWithProvidersStoreAssetsBillingAndRouter(cfg, providers, store, nil, &trackingArtifactBilling{}, storage.NewStaticRouter(backend))
	svc.now = func() time.Time { return now }
	imageBytes, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAFgwJ/lqR5DQAAAABJRU5ErkJggg==")
	svc.SetHTTPClient(&http.Client{Transport: artifactRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(imageBytes))}, nil
	})})
	created, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{UserID: 502, AbstractModel: "basic", TaskType: "text_to_image", Prompt: "recover fanout", SizeMode: "ratio", BaseResolution: "1k", Quality: "auto", AspectRatio: "1:1", OutputImageCount: 2})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	claimed, ok, err := svc.AcquireNextTask(context.Background(), "worker", time.Minute)
	if err != nil || !ok {
		t.Fatalf("AcquireNextTask: ok=%v err=%v", ok, err)
	}
	pending, err := svc.ExecuteLeasedTask(context.Background(), claimed, "worker", []string{"openai"})
	if err != nil {
		t.Fatalf("initial fanout execution: %v", err)
	}
	if pending.Task.ArtifactRecovery.Status != artifactRecoveryPending || pending.Task.ArtifactRecovery.AttemptCount != 1 || providerCalls != 1 {
		t.Fatalf("expected paid fanout checkpoint and pending recovery, task=%#v provider_calls=%d", pending.Task, providerCalls)
	}
	if pending.Task.ArtifactRecovery.StorageDriver != "local" || len(pending.Task.ArtifactRecovery.ObjectKeys) != 2 || pending.Task.ArtifactRecovery.ObjectKeys[0] == pending.Task.ArtifactRecovery.ObjectKeys[1] {
		t.Fatalf("expected exact multi-image recovery identities, got %#v", pending.Task.ArtifactRecovery)
	}
	now = pending.Task.ArtifactRecovery.NextRetryAt.Add(time.Millisecond)
	reclaimed, ok, err := svc.AcquireNextTask(context.Background(), "worker", time.Minute)
	if err != nil || !ok {
		t.Fatalf("reclaim fanout: ok=%v err=%v", ok, err)
	}
	recovered, err := svc.ExecuteLeasedTask(context.Background(), reclaimed, "worker", []string{"openai"})
	if err != nil {
		t.Fatalf("recover fanout: %v", err)
	}
	if recovered.Task.Status != domainimagetask.StatusSucceeded || len(recovered.Task.Results) != 2 || providerCalls != 1 {
		t.Fatalf("expected two recovered results without provider replay, task=%#v provider_calls=%d", recovered.Task, providerCalls)
	}
	if recovered.Task.Results[0].ID == recovered.Task.Results[1].ID || recovered.Task.Results[0].ObjectKey == recovered.Task.Results[1].ObjectKey {
		t.Fatalf("fanout results collided: %#v", recovered.Task.Results)
	}
	for index := range recovered.Task.Results {
		if recovered.Task.Results[index].ObjectKey != pending.Task.ArtifactRecovery.ObjectKeys[index] {
			t.Fatalf("result %d key=%q, pinned=%q", index, recovered.Task.Results[index].ObjectKey, pending.Task.ArtifactRecovery.ObjectKeys[index])
		}
	}
	loaded, err := store.GetByID(context.Background(), 502, created.ID)
	if err != nil || len(loaded.Results) != 2 {
		t.Fatalf("persisted fanout results: task=%#v err=%v", loaded, err)
	}
}

func TestArtifactFetchTimeoutDiagnosticSanitizesSignedURL(t *testing.T) {
	backend := &countingArtifactBackend{objects: map[string][]byte{}}
	svc := NewServiceWithProvidersStoreAssetsBillingAndRouter(artifactRecoveryTestConfig(), nil, NewMemoryStore(), nil, nil, storage.NewStaticRouter(backend))
	svc.SetHTTPClient(&http.Client{Transport: artifactRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	})})
	task := domainimagetask.Task{ID: "11111111-1111-1111-1111-111111111111", UserID: 1}
	_, err := svc.persistRemoteImageResult(context.Background(), task, 0, provider.ImageResult{URL: "https://cdn.example.com/private/result.png?signature=top-secret&expires=1"})
	var failure *artifactPersistenceFailure
	if !errors.As(err, &failure) {
		t.Fatalf("expected artifact persistence failure, got %T %v", err, err)
	}
	if failure.diagnostic.Code != "ARTIFACT_FETCH_TIMEOUT" || failure.diagnostic.URLHost != "cdn.example.com" || failure.diagnostic.URLPath != "/private/result.png" {
		t.Fatalf("unexpected timeout diagnostic %#v", failure.diagnostic)
	}
	if bytes.Contains([]byte(failure.diagnostic.Cause), []byte("top-secret")) {
		t.Fatalf("diagnostic leaked signed query: %#v", failure.diagnostic)
	}
}

func TestArtifactRecoveryFailsAfterFourPersistenceAttempts(t *testing.T) {
	svc, _, backend, providerCalls, now := newArtifactRecoveryTestService(t, 10)
	billing := &trackingArtifactBilling{}
	svc.billing = billing
	if _, err := svc.CreateTask(context.Background(), artifactRecoveryCreateRequest()); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	var final domainimagetask.Task
	for attempt := 1; attempt <= 4; attempt++ {
		claimed, ok, err := svc.AcquireNextTask(context.Background(), "worker", time.Minute)
		if err != nil || !ok {
			t.Fatalf("AcquireNextTask attempt %d: ok=%v err=%v", attempt, ok, err)
		}
		result, execErr := svc.ExecuteLeasedTask(context.Background(), claimed, "worker", []string{"openrouter"})
		final = result.Task
		if attempt < 4 {
			if execErr != nil {
				t.Fatalf("pending attempt %d returned error: %v", attempt, execErr)
			}
			*now = final.ArtifactRecovery.NextRetryAt.Add(time.Millisecond)
		} else if execErr == nil {
			t.Fatal("expected fourth persistence failure to be terminal")
		}
	}
	if final.Status != domainimagetask.StatusFailed || final.ArtifactRecovery.AttemptCount != 4 {
		t.Fatalf("expected exhausted failed task, got %#v", final)
	}
	if final.ArtifactRecovery.EncryptedPayload != "" {
		t.Fatal("terminal failure must clear encrypted recovery payload")
	}
	if len(final.ArtifactRecovery.Diagnostics) != 4 {
		t.Fatalf("expected four retained failure diagnostics, got %#v", final.ArtifactRecovery.Diagnostics)
	}
	if *providerCalls != 1 || backend.putCalls != 4 {
		t.Fatalf("expected one provider call and four puts, provider=%d put=%d", *providerCalls, backend.putCalls)
	}
	if billing.finalizeCalls != 1 || billing.lastActualPoints != "0.00000" {
		t.Fatalf("expected one refund finalization, got %#v", billing)
	}
}

func TestArtifactRecoverySurvivesServiceReconstruction(t *testing.T) {
	svc, store, backend, providerCalls, now := newArtifactRecoveryTestService(t, 1)
	if _, err := svc.CreateTask(context.Background(), artifactRecoveryCreateRequest()); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	claimed, ok, err := svc.AcquireNextTask(context.Background(), "worker-a", time.Minute)
	if err != nil || !ok {
		t.Fatalf("AcquireNextTask: ok=%v err=%v", ok, err)
	}
	pending, err := svc.ExecuteLeasedTask(context.Background(), claimed, "worker-a", []string{"openrouter"})
	if err != nil {
		t.Fatalf("initial ExecuteLeasedTask: %v", err)
	}
	*now = pending.Task.ArtifactRecovery.NextRetryAt.Add(time.Millisecond)

	restarted := NewServiceWithProvidersStoreAssetsBillingAndRouter(artifactRecoveryTestConfig(), svc.providers, store, nil, nil, storage.NewStaticRouter(backend))
	restarted.now = func() time.Time { return *now }
	restarted.SetHTTPClient(svc.httpClient)
	reclaimed, ok, err := restarted.AcquireNextTask(context.Background(), "worker-b", time.Minute)
	if err != nil || !ok {
		t.Fatalf("reclaim after restart: ok=%v err=%v", ok, err)
	}
	result, err := restarted.ExecuteLeasedTask(context.Background(), reclaimed, "worker-b", []string{"openrouter"})
	if err != nil {
		t.Fatalf("recovery after restart: %v", err)
	}
	if result.Task.Status != domainimagetask.StatusSucceeded || *providerCalls != 1 {
		t.Fatalf("expected recovery without provider replay, task=%#v calls=%d", result.Task, *providerCalls)
	}
}

func newArtifactRecoveryTestService(t *testing.T, failWrites int) (*Service, *MemoryStore, *countingArtifactBackend, *int, *time.Time) {
	t.Helper()
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	providerCalls := 0
	providers := map[string]provider.ImageProvider{"openrouter": artifactTestProvider{generate: func(context.Context, provider.ImageRequest) (provider.ImageResponse, error) {
		providerCalls++
		return provider.ImageResponse{ProviderRequestID: "paid-request", Data: []provider.ImageResult{{URL: "https://cdn.example.com/result.png?sig=secret"}}}, nil
	}}}
	backend := &countingArtifactBackend{failWrites: failWrites, objects: map[string][]byte{}}
	store := NewMemoryStore()
	svc := NewServiceWithProvidersStoreAssetsBillingAndRouter(artifactRecoveryTestConfig(), providers, store, nil, nil, storage.NewStaticRouter(backend))
	svc.now = func() time.Time { return now }
	imageBytes, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAFgwJ/lqR5DQAAAABJRU5ErkJggg==")
	svc.SetHTTPClient(&http.Client{Transport: artifactRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(imageBytes))}, nil
	})})
	return svc, store, backend, &providerCalls, &now
}

func artifactRecoveryTestConfig() config.Config {
	cfg := config.Config{}
	cfg.Security.SecureConfigEncryptionKey = "artifact-recovery-key"
	cfg.GenerationLimits.MaxImageCount = 1
	cfg.Billing.PointsScale = 5
	cfg.Billing.AutoBaseResolutionDefaultByGroup = map[string]string{"basic": "1k"}
	cfg.Billing.BaseResolutionPointsByModel = map[string]map[string]string{"basic": {"1k": "1.00000"}}
	cfg.Billing.UserGroupMultipliers = map[string]string{"basic": "1.00000"}
	cfg.Billing.TaskMultipliers = map[string]string{"text_to_image": "1.00000"}
	cfg.Providers.OpenRouter.Enabled = true
	cfg.Routing.ProviderCapabilities = map[string]config.ProviderCapabilityConfig{"openrouter": {
		SupportedModels: []string{"basic"}, SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"}, Quality: []string{"auto"},
		SupportedAspectRatios: []string{"1:1"}, MaxImageCount: 1, Priority: 1,
	}}
	cfg.Routing.ProviderModelMap = map[string]map[string]string{"basic": {"openrouter": "image-model"}}
	return cfg
}

func artifactRecoveryCreateRequest() domainimagetask.CreateRequest {
	return domainimagetask.CreateRequest{UserID: 501, AbstractModel: "basic", TaskType: "text_to_image", Prompt: "recover me", SizeMode: "ratio", BaseResolution: "1k", Quality: "auto", AspectRatio: "1:1", OutputImageCount: 1}
}

type artifactTestProvider struct {
	generate func(context.Context, provider.ImageRequest) (provider.ImageResponse, error)
}

func TestClassifyImageSizeIdentifiesObservedUpstreamRewrite(t *testing.T) {
	if got := classifyImageSize("1280x720", 1672, 941); got != "upstream_rewritten" {
		t.Fatalf("classifyImageSize() = %q, want upstream_rewritten", got)
	}
	if got := classifyImageSize("1280x720", 1280, 720); got != "match" {
		t.Fatalf("classifyImageSize() = %q, want match", got)
	}
}

func (p artifactTestProvider) Generate(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
	return p.generate(ctx, req)
}
func (p artifactTestProvider) Edit(context.Context, provider.ImageRequest) (provider.ImageResponse, error) {
	return provider.ImageResponse{}, errors.New("not implemented")
}

type artifactRoundTripFunc func(*http.Request) (*http.Response, error)

func (f artifactRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type countingArtifactBackend struct {
	mu         sync.Mutex
	failWrites int
	putCalls   int
	objects    map[string][]byte
}

type trackingArtifactBilling struct {
	reserveCalls     int
	finalizeCalls    int
	lastActualPoints string
}

func (b *trackingArtifactBilling) Estimate(domainbilling.EstimateRequest) (domainbilling.EstimateResult, error) {
	return domainbilling.EstimateResult{
		BaseResolution: "1k", EstimatedPoints: "1.00000", ChargedPoints: "1.00000", UserGroupMultiplier: "1.00000",
		PricingSnapshot: domainbilling.PricingSnapshot{BaseResolution: "1k", EstimatedPoints: "1.00000"},
	}, nil
}

func (b *trackingArtifactBilling) ActualPoints(domainbilling.PricingSnapshot, int) (string, error) {
	return "1.00000", nil
}

func (b *trackingArtifactBilling) ReserveTask(context.Context, domainbilling.ReserveRequest) (domainbilling.BalanceSummary, error) {
	b.reserveCalls++
	return domainbilling.BalanceSummary{}, nil
}

func (b *trackingArtifactBilling) FinalizeTask(_ context.Context, req domainbilling.FinalizeRequest) (domainbilling.BalanceSummary, error) {
	b.finalizeCalls++
	b.lastActualPoints = req.ActualPoints
	return domainbilling.BalanceSummary{}, nil
}

func (b *countingArtifactBackend) Driver() string { return "local" }
func (b *countingArtifactBackend) Put(_ context.Context, key string, _ string, content []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.putCalls++
	if b.putCalls <= b.failWrites {
		return errors.New("temporary storage failure")
	}
	b.objects[key] = append([]byte(nil), content...)
	return nil
}
func (b *countingArtifactBackend) Get(_ context.Context, key string) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	content, ok := b.objects[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return append([]byte(nil), content...), nil
}
func (b *countingArtifactBackend) Delete(_ context.Context, key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.objects, key)
	return nil
}
