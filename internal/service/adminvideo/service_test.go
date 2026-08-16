package adminvideo

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSnapshotNormalizesEmptyCollectionsForJSONClients(t *testing.T) {
	got, err := NewService(&fakeStore{}).Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"capabilities", "rate_cards", "routes", "impacts"} {
		items, ok := decoded[field].([]any)
		if !ok || len(items) != 0 {
			t.Fatalf("%s must serialize as an empty array, payload=%s", field, payload)
		}
	}
}

type fakeStore struct {
	snapshot Snapshot
	policy   MediaPolicy
	retried  []RetryRequest
}

func (s *fakeStore) Snapshot(context.Context) (Snapshot, error)              { return s.snapshot, nil }
func (s *fakeStore) ListTasks(context.Context, TaskFilter) (TaskPage, error) { return TaskPage{}, nil }
func (s *fakeStore) GetTask(context.Context, uuid.UUID) (TaskDetail, error)  { return TaskDetail{}, nil }
func (s *fakeStore) Retry(_ context.Context, request RetryRequest) error {
	s.retried = append(s.retried, request)
	return nil
}
func (s *fakeStore) GetMediaPolicy(context.Context) (MediaPolicy, error) { return s.policy, nil }
func (s *fakeStore) SaveMediaPolicy(_ context.Context, policy MediaPolicy, _ int64) (MediaPolicy, error) {
	s.policy = policy
	return policy, nil
}
func (s *fakeStore) Readiness(context.Context, time.Time) (ReadinessSnapshot, error) {
	return ReadinessSnapshot{}, nil
}
func (s *fakeStore) SaveCapability(context.Context, CapabilityWrite) (CapabilitySummary, error) {
	return CapabilitySummary{}, nil
}
func (s *fakeStore) ListVideoModelRateCards(context.Context, int64) ([]RateCardSummary, error) {
	return nil, nil
}
func (s *fakeStore) SaveVideoModelRateCard(context.Context, RateCardWrite) (RateCardSummary, error) {
	return RateCardSummary{}, nil
}
func (s *fakeStore) DeleteVideoModelRateCard(context.Context, int64, int) error { return nil }
func (s *fakeStore) GetEffectiveVideoModelRateCard(context.Context, int64, time.Time) (RateCardSummary, error) {
	return RateCardSummary{}, nil
}
func (s *fakeStore) GetVideoModelPricingContext(context.Context, int64) (ModelPricingContext, error) {
	return ModelPricingContext{}, nil
}
func (s *fakeStore) GetVideoRouteQuoteContext(context.Context, int64, time.Time) (RouteQuoteContext, error) {
	return RouteQuoteContext{}, nil
}
func (s *fakeStore) SaveRouteConfig(context.Context, RouteConfigWrite) (RouteConfigSummary, error) {
	return RouteConfigSummary{}, nil
}
func (s *fakeStore) DeleteVideoConfig(context.Context, ConfigKind, int64, int64) error { return nil }

func TestSnapshotIncludesNativePricingVersionsAndBlockingImpacts(t *testing.T) {
	store := &fakeStore{snapshot: Snapshot{
		Capabilities: []CapabilitySummary{{AccountModelID: 11, Version: "cap-v2", ValidationState: "untested", Enabled: true}},
		RateCards:    []RateCardSummary{{AccountModelID: 11, RateVersion: 3, Enabled: false}},
		Routes: []RouteConfigSummary{{
			RouteModelID: 31, ConfigVersion: "route-v6", CandidateCount: 1,
			CandidateAccountModelIDs: []int64{11}, Enabled: true,
		}},
	}}

	got, err := NewService(store).Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got.Capabilities[0].Version != "cap-v2" || got.RateCards[0].RateVersion != 3 || got.Routes[0].ConfigVersion != "route-v6" {
		t.Fatalf("independent versions were lost: %#v", got)
	}
	if len(got.Impacts) != 3 {
		t.Fatalf("expected missing combination, capability, and rate impacts, got %#v", got.Impacts)
	}
}

func TestSnapshotAcceptsTypedVisibleCombinationsWithCompleteCandidate(t *testing.T) {
	store := &fakeStore{snapshot: Snapshot{
		Capabilities: []CapabilitySummary{{AccountModelID: 11, ValidationState: "verified", Enabled: true}},
		RateCards:    []RateCardSummary{{AccountModelID: 11, RateVersion: 2, Enabled: true}},
		Routes: []RouteConfigSummary{{
			RouteModelID: 31, RouteName: "视频创作", CandidateCount: 1, CandidateAccountModelIDs: []int64{11}, Enabled: true,
			VisibleOptions: map[string]any{"combinations": []VisibleCombination{{
				TaskType: "text_to_video", Resolution: "720p", AspectRatio: "16:9", AudioMode: "silent", DurationSeconds: 5,
			}}},
		}},
	}}

	got, err := NewService(store).Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Impacts) != 0 {
		t.Fatalf("complete native pricing configuration must not report impacts, got %#v", got.Impacts)
	}
}

func TestRetryOnlyAllowsArtifactDerivativeAndSettlementRecovery(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store)
	taskID := uuid.New()
	if err := service.Retry(t.Context(), RetryRequest{Kind: RetryArtifact, TaskID: taskID}); err != nil {
		t.Fatal(err)
	}
	if err := service.Retry(t.Context(), RetryRequest{Kind: RetryDerivative, JobID: uuid.New()}); err != nil {
		t.Fatal(err)
	}
	if err := service.Retry(t.Context(), RetryRequest{Kind: RetrySettlement, TaskID: taskID}); err != nil {
		t.Fatal(err)
	}
	if err := service.Retry(t.Context(), RetryRequest{Kind: "provider", TaskID: taskID}); err == nil {
		t.Fatal("provider generation retry must be rejected")
	}
	if len(store.retried) != 3 {
		t.Fatalf("unexpected retries: %#v", store.retried)
	}
}

func TestMediaPolicyUsesOptimisticVersionAndHardLimits(t *testing.T) {
	store := &fakeStore{policy: DefaultMediaPolicy()}
	service := NewService(store)
	policy := DefaultMediaPolicy()
	policy.Version = 2
	if _, err := service.UpdateMediaPolicy(t.Context(), 1, policy, 9); err == nil {
		t.Fatal("version conflict must be rejected")
	}
	policy.Version = 1
	policy.SingleFileMaxBytes = 1024*1024*1024 + 1
	if _, err := service.UpdateMediaPolicy(t.Context(), 1, policy, 9); err == nil {
		t.Fatal("platform hard limit must be rejected")
	}
	policy.SingleFileMaxBytes = 512 * 1024 * 1024
	updated, err := service.UpdateMediaPolicy(t.Context(), 1, policy, 9)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 || updated.AppliesTo != "new_objects_and_derivative_versions" {
		t.Fatalf("unexpected versioned policy: %#v", updated)
	}
}

func TestMediaPolicyBuildsRuntimeUploadAndDerivativePolicy(t *testing.T) {
	policy := DefaultMediaPolicy()
	policy.AllowedFormats["image"] = []string{"png"}
	policy.SingleFileMaxBytes = 512
	policy.VideoMaxDurationSeconds = 12
	policy.UserQuotaBytes = 4096
	policy.ImageThumbnailWidths = []int{320}
	policy.VideoPosterEnabled = false
	policy.UploadSessionTTLHours = 6

	runtime := policy.RuntimePolicy()
	if runtime.Policy.SingleFileMaxBytes != 512 || runtime.Policy.VideoMaxDurationMS != 12_000 {
		t.Fatalf("runtime limits = %#v", runtime.Policy)
	}
	if got := runtime.Policy.AllowedFormats["image"]; len(got) != 1 || got[0] != "png" {
		t.Fatalf("runtime image formats = %#v", got)
	}
	if runtime.Policy.VideoPosterEnabled || len(runtime.Policy.ImageThumbnailWidths) != 1 {
		t.Fatalf("runtime derivatives = %#v", runtime.Policy)
	}
	if runtime.UserQuotaBytes != 4096 || runtime.UploadTTL != 6*time.Hour {
		t.Fatalf("runtime upload limits = %#v", runtime)
	}
}
