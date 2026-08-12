package adminvideo

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

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
func (s *fakeStore) SaveCostRule(context.Context, CostRuleWrite) (CostRuleSummary, error) {
	return CostRuleSummary{}, nil
}
func (s *fakeStore) SaveStrategy(context.Context, StrategyWrite) (PricingStrategySummary, error) {
	return PricingStrategySummary{}, nil
}
func (s *fakeStore) SavePriceRule(context.Context, PriceRuleWrite) (PriceRuleSummary, error) {
	return PriceRuleSummary{}, nil
}
func (s *fakeStore) SaveRouteConfig(context.Context, RouteConfigWrite) (RouteConfigSummary, error) {
	return RouteConfigSummary{}, nil
}
func (s *fakeStore) DeleteVideoConfig(context.Context, ConfigKind, int64, int64) error { return nil }

func TestSnapshotIncludesIndependentVersionsAndBlockingImpact(t *testing.T) {
	store := &fakeStore{snapshot: Snapshot{
		Capabilities: []CapabilitySummary{{AccountModelID: 11, Version: "cap-v2", Enabled: true}},
		CostRules:    []CostRuleSummary{{AccountModelID: 11, RuleVersion: 3, Enabled: true}},
		Strategies:   []PricingStrategySummary{{ID: 21, StrategyVersion: 4, Enabled: true}},
		PriceRules:   []PriceRuleSummary{{StrategyID: 21, RuleVersion: 5, SafetyPoints: "8", SalesPoints: "7", Enabled: true}},
		Routes:       []RouteConfigSummary{{RouteModelID: 31, ConfigVersion: "route-v6", PricingStrategyID: 21, CandidateCount: 1, Enabled: true}},
	}}

	got, err := NewService(store).Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got.Capabilities[0].Version != "cap-v2" || got.CostRules[0].RuleVersion != 3 || got.Strategies[0].StrategyVersion != 4 || got.Routes[0].ConfigVersion != "route-v6" {
		t.Fatalf("independent versions were lost: %#v", got)
	}
	if len(got.Impacts) != 1 || !got.Impacts[0].Blocking || got.Impacts[0].Code != "price_below_safety_floor" {
		t.Fatalf("expected price safety impact, got %#v", got.Impacts)
	}
}

func TestRetryOnlyAllowsArtifactAndDerivativeRecovery(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store)
	taskID := uuid.New()
	if err := service.Retry(t.Context(), RetryRequest{Kind: RetryArtifact, TaskID: taskID}); err != nil {
		t.Fatal(err)
	}
	if err := service.Retry(t.Context(), RetryRequest{Kind: RetryDerivative, JobID: uuid.New()}); err != nil {
		t.Fatal(err)
	}
	if err := service.Retry(t.Context(), RetryRequest{Kind: "provider", TaskID: taskID}); err == nil {
		t.Fatal("provider generation retry must be rejected")
	}
	if len(store.retried) != 2 {
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
