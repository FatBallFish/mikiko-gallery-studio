package imagetask

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	image "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNativeDefaultsFrozenInTaskSnapshot(t *testing.T) {
	p, _ := nativeimage.ProfileForModel("qwen-image-3.0")
	o, err := nativeimage.EffectiveOptions(p, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	task := buildTask(image.CreateRequest{}, modelhub.ResolvedRequest{CapabilityVersion: "quoted", NativeOptions: o}, image.StatusQueued)
	if !task.GenerationSnapshot.NativeDefaultsFrozen || task.GenerationSnapshot.CapabilityVersion != "quoted" || task.NativeOptions == nil || *task.NativeOptions.Qwen.PromptExtend || *task.NativeOptions.Watermark {
		t.Fatalf("missing frozen policy: %+v", task)
	}
	*o.Watermark = true
	if *task.NativeOptions.Watermark {
		t.Fatal("snapshot aliases resolver options")
	}
}

func TestNativeUncertainResponseNeverFallsBack(t *testing.T) {
	for _, mode := range []string{"truncated", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			svc, store, _, _, _ := newArtifactRecoveryTestService(t, 0)
			calls := 0
			svc.SetHTTPClient(&http.Client{Transport: artifactRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if mode == "timeout" {
					return nil, context.DeadlineExceeded
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"output":`))}, nil
			})})
			_, err := svc.CreateTask(context.Background(), artifactRecoveryCreateRequest())
			if err != nil {
				t.Fatal(err)
			}
			task, ok, err := svc.AcquireNextTask(context.Background(), "worker", time.Minute)
			if err != nil || !ok {
				t.Fatalf("acquire %v %v", ok, err)
			}
			task.Operation = nativeimage.OperationGenerate
			candidate := modelhub.ProviderCandidate{Provider: "qwen_image", AdapterType: "qwen_image", AuthType: "api_key", BaseURL: "https://fake.example", Credentials: map[string]string{"api_key": "fixture"}, ModelCode: "qwen-image-3.0", MaxImageCount: 6}
			result, err := svc.executeResolvedTask(context.Background(), task, "worker", modelhub.ResolvedRequest{Providers: []modelhub.ProviderCandidate{candidate, candidate}}, executionOptions{prompt: "fixture"})
			if err == nil || calls != 1 || result.Task.ErrorCode != "IMAGE_UPSTREAM_SUBMISSION_UNCERTAIN" || result.Task.UpstreamStartedAt == nil {
				t.Fatalf("unsafe retry: calls=%d task=%+v err=%v", calls, result.Task, err)
			}
			saved, err := store.GetByID(context.Background(), task.UserID, task.ID)
			if err != nil || saved.UpstreamStartedAt == nil || saved.Status != image.StatusRejected {
				t.Fatalf("uncertainty not durable: %+v %v", saved, err)
			}
		})
	}
}

func TestNativeTaskExecutionReachesHTTPWithoutOpenAIDefaults(t *testing.T) {
	svc, store, _, _, _ := newArtifactRecoveryTestService(t, 0)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		params, _ := payload["parameters"].(map[string]any)
		if _, ok := params["quality"]; ok {
			t.Error("generic quality leaked")
		}
		fmt.Fprint(w, `{"request_id":"paid","output":{"choices":[{"message":{"content":[{"image":"https://cdn.example.com/image.png"}]}}]}}`)
	}))
	defer server.Close()
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAFgwJ/lqR5DQAAAABJRU5ErkJggg==")
	svc.SetHTTPClient(&http.Client{Transport: artifactRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == strings.TrimPrefix(server.URL, "http://") {
			return http.DefaultTransport.RoundTrip(r)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(png))}, nil
	})})
	task, err := svc.CreateTask(context.Background(), artifactRecoveryCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := svc.AcquireNextTask(context.Background(), "worker", time.Minute)
	if err != nil || !ok {
		t.Fatal(err)
	}
	claimed.Operation = nativeimage.OperationGenerate
	claimed.PricingSnapshot.NativeImage = nil
	if err := store.Save(context.Background(), claimed); err != nil {
		t.Fatal(err)
	}
	result, err := svc.executeResolvedTask(context.Background(), claimed, "worker", modelhub.ResolvedRequest{Providers: []modelhub.ProviderCandidate{{Provider: "qwen_image", AdapterType: "qwen_image", AuthType: "api_key", BaseURL: server.URL, Credentials: map[string]string{"api_key": "fixture"}, ModelCode: "qwen-image-3.0", MaxImageCount: 6}}}, executionOptions{prompt: "fixture", user: "legacy-user"})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || result.Task.ID != task.ID || result.Task.Status != image.StatusSucceeded {
		t.Fatalf("native execution calls=%d task=%#v", calls, result.Task)
	}
}

func TestNativeRetryPreservesExplicitFalseAndLayerOperation(t *testing.T) {
	no := false
	task := image.Task{TaskType: "image_edit", Operation: nativeimage.OperationLayerDecomposition, OutputImageCount: 1, NativeOptions: &nativeimage.Options{Watermark: &no, Qwen: &nativeimage.QwenOptions{PromptExtend: &no}}, ReferenceAssetIDs: []string{"input"}}
	retry := imageRetryRequest(task)
	if retry.Operation != task.Operation || !reflect.DeepEqual(retry.NativeOptions, task.NativeOptions) {
		t.Fatalf("retry lost native parameters: %#v", retry)
	}
	*retry.NativeOptions.Watermark = true
	if *task.NativeOptions.Watermark {
		t.Fatal("retry aliased native option pointer")
	}
	retry.OutputImageCount = 2
	if _, err := normalizeNativeCreate(retry); err == nil {
		t.Fatal("layer operation accepted ordinary n")
	}
}

func TestNativeCheckpointEncryptsFullLayersAndPrivateMetadata(t *testing.T) {
	svc, _, _, _, _ := newArtifactRecoveryTestService(t, 0)
	layers := make([]provider.ImageResult, 17)
	for i := range layers {
		layers[i] = provider.ImageResult{ID: string(rune('a' + i)), Layer: &nativeimage.LayerMetadata{ZIndex: i, IsBase: i == 0, Name: "layer"}}
	}
	response := provider.ImageResponse{Operation: nativeimage.OperationLayerDecomposition, Data: layers, Usage: json.RawMessage(`{"images":17}`), ProviderMetadata: json.RawMessage(`{"url":"https://paid.example/secret"}`)}
	payload, err := svc.encryptArtifactResults(layers, response)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(payload, "paid.example") {
		t.Fatal("private metadata leaked from encryption")
	}
	got, err := svc.decryptArtifactResults(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, layers) {
		t.Fatalf("checkpoint lost layers: %#v", got)
	}
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "paid.example") {
		t.Fatal("public response leaked private metadata")
	}
}

type rejectPaidCheckpointStore struct{ Store }

func (s *rejectPaidCheckpointStore) SaveIfOwned(ctx context.Context, task image.Task, owner string, now time.Time) error {
	if task.ArtifactRecovery.EncryptedPayload != "" {
		return fmt.Errorf("checkpoint database unavailable")
	}
	return s.Store.SaveIfOwned(ctx, task, owner, now)
}
func (s *rejectPaidCheckpointStore) SaveTerminalState(ctx context.Context, task image.Task, owner string, now time.Time) error {
	if task.ArtifactRecovery.EncryptedPayload != "" {
		return fmt.Errorf("terminal database unavailable")
	}
	return s.Store.SaveTerminalState(ctx, task, owner, now)
}

func TestNativeDoubleCheckpointFailureNeverResubmitsAfterReclaim(t *testing.T) {
	svc, store, _, calls, now := newArtifactRecoveryTestService(t, 0)
	svc.store = &rejectPaidCheckpointStore{Store: store}
	created, err := svc.CreateTask(context.Background(), artifactRecoveryCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	task, ok, err := svc.AcquireNextTask(context.Background(), "first", time.Minute)
	if err != nil || !ok {
		t.Fatal(err)
	}
	if _, err = svc.ExecuteLeasedTask(context.Background(), task, "first", nil); err == nil {
		t.Fatal("expected double checkpoint failure")
	}
	saved, err := store.GetByID(context.Background(), created.UserID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.UpstreamStartedAt == nil || saved.ArtifactRecovery.EncryptedPayload != "" || *calls != 1 {
		t.Fatalf("marker not durable: %#v calls=%d", saved, *calls)
	}
	*now = now.Add(2 * time.Minute)
	reclaimed, ok, err := svc.AcquireNextTask(context.Background(), "second", time.Minute)
	if err != nil || !ok {
		t.Fatalf("reclaim %v %v", ok, err)
	}
	result, err := svc.ExecuteLeasedTask(context.Background(), reclaimed, "second", nil)
	if err == nil || result.Task.Status != image.StatusRejected || result.Task.ErrorCode != "IMAGE_UPSTREAM_SUBMISSION_UNCERTAIN" || *calls != 1 {
		t.Fatalf("unsafe reclaim: %#v err=%v calls=%d", result.Task, err, *calls)
	}
	if _, err = svc.RetryTask(context.Background(), created.UserID, created.ID, image.RetryRequest{}); err == nil {
		t.Fatal("uncertain paid task permitted retry")
	}
}

func TestNativeTaskSettlementUsesActualLayerRolesAndTerminalTierDiagnostic(t *testing.T) {
	svc, _, _, _, _ := newArtifactRecoveryTestService(t, 0)
	svc.billing = &trackingArtifactBilling{}
	price, _ := nativeimage.PricingForModel("doubao-seedream-5-0-pro-260628")
	base := .4
	price.LayerBaseCNY = &base
	price.LayerPricingConfirmed = true
	task := image.Task{EstimatedPoints: "280", AccountModelID: 1, PricingSnapshot: domainbilling.PricingSnapshot{EstimatedPoints: "280", NativeImage: &domainbilling.NativeImagePricingSnapshot{Operation: nativeimage.OperationLayerDecomposition, CNYPerPoint: ".01", SaleMultiplier: "1", InputImages: 1, ReservedOutputs: 17, ReservedPixels: 1000000, Candidates: []domainbilling.NativeImageCandidatePricing{{AccountModelID: 1, Cost: price, Sale: price}}}}, Results: []provider.ImageResult{{Width: 1000, Height: 1000, Layer: &nativeimage.LayerMetadata{IsBase: false}}, {Width: 1000, Height: 1000, Layer: &nativeimage.LayerMetadata{IsBase: false}}}}
	if err := svc.applyActualPoints(&task, 2); err != nil {
		t.Fatal(err)
	}
	if task.ActualPoints != "30.00000" || task.ProviderCost != "0.30000" {
		t.Fatalf("first non-base layer lost: %#v", task)
	}
	task.Results[0].Width = 5000
	if err := svc.applyActualPoints(&task, 2); err != nil {
		t.Fatal(err)
	}
	if task.ActualPoints != "280.00000" || task.ProviderCost != "" || task.GrossMargin != "" || task.BillingDiagnostic == "" {
		t.Fatalf("missing terminal unknown-cost diagnostic: %#v", task)
	}
}

func TestPaidSettlementFailureKeepsCheckpointAndReserve(t *testing.T) {
	svc, store, _, _, _ := newArtifactRecoveryTestService(t, 0)
	created, err := svc.CreateTask(context.Background(), artifactRecoveryCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	task, ok, err := svc.AcquireNextTask(context.Background(), "worker", 60000000000)
	if err != nil || !ok {
		t.Fatal(err)
	}
	task.Results = []provider.ImageResult{{ID: "paid-result"}}
	task.ArtifactRecovery.EncryptedPayload = "paid-checkpoint"
	_, err = svc.holdPaidSettlement(context.Background(), task, "worker", context.Canceled)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.GetByID(context.Background(), created.UserID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != image.StatusPartialFailed || got.ArtifactRecovery.EncryptedPayload != "" || len(got.Results) != 1 || got.ActualPoints != got.EstimatedPoints || got.GrossMargin != "" || got.BillingDiagnostic == "" {
		t.Fatalf("paid result discarded: %#v", got)
	}
}
