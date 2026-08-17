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
	snapshot           Snapshot
	policy             MediaPolicy
	retried            []RetryRequest
	readiness          ReadinessSnapshot
	routeQuoteContexts map[int64]RouteQuoteContext
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
	return s.readiness, nil
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
func (s *fakeStore) GetVideoRouteQuoteContext(_ context.Context, routeModelID int64, _ time.Time) (RouteQuoteContext, error) {
	if contextValue, ok := s.routeQuoteContexts[routeModelID]; ok {
		return contextValue, nil
	}
	return fakeRouteQuoteContext(s.snapshot, routeModelID), nil
}
func (s *fakeStore) SaveRouteConfig(context.Context, RouteConfigWrite) (RouteConfigSummary, error) {
	return RouteConfigSummary{}, nil
}
func (s *fakeStore) DeleteVideoConfig(context.Context, ConfigKind, int64, int64) error { return nil }

func fakeRouteQuoteContext(snapshot Snapshot, routeModelID int64) RouteQuoteContext {
	result := RouteQuoteContext{CNYPerPoint: "0.01", ConversionVersion: "billing-v1"}
	for _, route := range snapshot.Routes {
		if route.RouteModelID == routeModelID {
			result.Route = route
			break
		}
	}
	for _, accountModelID := range result.Route.CandidateAccountModelIDs {
		candidate := RouteQuoteCandidate{AccountModelID: accountModelID}
		for _, capability := range snapshot.Capabilities {
			if capability.AccountModelID == accountModelID && capability.Enabled && capability.ValidationState == "verified" {
				candidate.CapabilityVersion = capability.Version
				candidate.Capability = capability.Capability
				break
			}
		}
		for _, card := range snapshot.RateCards {
			if card.AccountModelID != accountModelID || !card.Enabled {
				continue
			}
			candidate.RateCard = card
			if candidate.RateCard.RateVersion == 0 {
				candidate.RateCard.RateVersion = 1
			}
			candidate.ProviderCode = card.ProviderCode
			if candidate.ProviderCode == "" {
				switch card.PricingSchema {
				case "seedance_token_v1":
					candidate.ProviderCode = "seedance"
				case "minimax_h3_second_v1":
					candidate.ProviderCode = "minimax"
				}
			}
			break
		}
		switch candidate.ProviderCode {
		case "seedance":
			candidate.ModelCode = "doubao-seedance-2-0-260128"
		case "minimax":
			candidate.ModelCode = "MiniMax-H3"
		}
		result.Candidates = append(result.Candidates, candidate)
	}
	return result
}

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
	if len(got.Impacts) != 2 {
		t.Fatalf("expected capability and rate impacts, got %#v", got.Impacts)
	}
}

func TestSnapshotAcceptsCompleteCandidateWithoutVisibleCombinations(t *testing.T) {
	store := &fakeStore{snapshot: Snapshot{
		Capabilities: []CapabilitySummary{{AccountModelID: 11, ValidationState: "verified", Enabled: true, Capability: domainVideoCapabilityMap("720p")}},
		RateCards: []RateCardSummary{{AccountModelID: 11, ProviderCode: "seedance", PricingSchema: "seedance_token_v1", RateVersion: 2, Enabled: true, RateConfig: map[string]any{
			"resolutions": map[string]any{"720p": map[string]any{"without_input_video_million_tokens_cny": "46"}},
		}}},
		Routes: []RouteConfigSummary{{
			RouteModelID: 31, RouteName: "视频创作", CandidateCount: 1, CandidateAccountModelIDs: []int64{11}, Enabled: true,
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

func TestSnapshotDoesNotBlockMixedRouteWhenOneCandidateIsPriceable(t *testing.T) {
	store := &fakeStore{snapshot: Snapshot{
		Capabilities: []CapabilitySummary{
			{AccountModelID: 11, ValidationState: "verified", Enabled: true, Capability: domainVideoCapabilityMap("720p")},
			{AccountModelID: 12, ValidationState: "verified", Enabled: true, Capability: domainVideoCapabilityMap("720p")},
		},
		RateCards: []RateCardSummary{
			{AccountModelID: 11, ProviderCode: "seedance", PricingSchema: "seedance_token_v1", RateVersion: 2, Enabled: true, RateConfig: map[string]any{
				"resolutions": map[string]any{"720p": map[string]any{"without_input_video_million_tokens_cny": "46"}},
			}},
		},
		Routes: []RouteConfigSummary{{
			RouteModelID: 31, RouteName: "混合路由", CandidateCount: 2, CandidateAccountModelIDs: []int64{11, 12}, Enabled: true,
		}},
	}}

	got, err := NewService(store).Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Impacts) != 0 {
		t.Fatalf("an excluded unpriced candidate must not block a priceable mixed route: %#v", got.Impacts)
	}
}

func TestSnapshotIgnoresHistoricalVisibleCombinations(t *testing.T) {
	store := &fakeStore{snapshot: Snapshot{
		Capabilities: []CapabilitySummary{{AccountModelID: 11, ValidationState: "verified", Enabled: true}},
		RateCards:    []RateCardSummary{{AccountModelID: 11, Enabled: true}},
		Routes: []RouteConfigSummary{{
			RouteModelID: 31, CandidateCount: 1, CandidateAccountModelIDs: []int64{11}, Enabled: true,
			VisibleOptions: map[string]any{"combinations": []any{map[string]any{"resolution": "legacy"}}},
		}},
	}}
	got, err := NewService(store).Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Impacts) != 0 {
		t.Fatalf("historical visible combinations must not affect readiness: %#v", got.Impacts)
	}
}

func TestReadinessDoesNotRecomputeRemovedVisibleCombinations(t *testing.T) {
	store := &fakeStore{readiness: ReadinessSnapshot{EnabledVideoRoutes: 1}}
	got, err := NewService(store).Readiness(t.Context(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if got.EnabledVideoRoutes != 1 || got.VisibleCombosMissingPrice != 0 {
		t.Fatalf("readiness = %#v", got)
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
