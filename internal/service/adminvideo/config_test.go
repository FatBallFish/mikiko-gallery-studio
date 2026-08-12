package adminvideo

import (
	"context"
	"testing"
	"time"
)

type fakeConfigStore struct {
	fakeStore
	capability CapabilityWrite
	cost       CostRuleWrite
	strategy   StrategyWrite
	price      PriceRuleWrite
	route      RouteConfigWrite
}

func (s *fakeConfigStore) SaveCapability(_ context.Context, input CapabilityWrite) (CapabilitySummary, error) {
	s.capability = input
	return CapabilitySummary{AccountModelID: input.AccountModelID, Version: input.CapabilityVersion, Enabled: input.Enabled}, nil
}
func (s *fakeConfigStore) SaveCostRule(_ context.Context, input CostRuleWrite) (CostRuleSummary, error) {
	s.cost = input
	return CostRuleSummary{ID: 1, AccountModelID: input.AccountModelID, RuleVersion: input.ExpectedRuleVersion + 1, Enabled: input.Enabled}, nil
}
func (s *fakeConfigStore) SaveStrategy(_ context.Context, input StrategyWrite) (PricingStrategySummary, error) {
	s.strategy = input
	return PricingStrategySummary{ID: input.ID, StrategyVersion: input.ExpectedVersion + 1, Enabled: input.Enabled}, nil
}
func (s *fakeConfigStore) SavePriceRule(_ context.Context, input PriceRuleWrite) (PriceRuleSummary, error) {
	s.price = input
	return PriceRuleSummary{ID: input.ID, StrategyID: input.StrategyID, RuleVersion: input.ExpectedVersion + 1, SalesPoints: input.MinimumTaskPoints, SafetyPoints: input.SafetyPoints, Enabled: input.Enabled}, nil
}
func (s *fakeConfigStore) SaveRouteConfig(_ context.Context, input RouteConfigWrite) (RouteConfigSummary, error) {
	s.route = input
	return RouteConfigSummary{RouteModelID: input.RouteModelID, ConfigVersion: input.ConfigVersion, Enabled: input.Enabled}, nil
}
func (s *fakeConfigStore) DeleteVideoConfig(context.Context, ConfigKind, int64, int64) error {
	return nil
}

func TestCapabilityWriteValidatesProviderNativeNAndCAS(t *testing.T) {
	store := &fakeConfigStore{fakeStore: fakeStore{snapshot: Snapshot{Capabilities: []CapabilitySummary{{AccountModelID: 7, Version: "cap-v1"}}}}}
	service := NewService(store)
	input := CapabilityWrite{AccountModelID: 7, ExpectedVersion: "wrong", CapabilityVersion: "cap-v2", Capability: map[string]any{"schema_version": 1, "provider_native_max_n": 1, "task_types": map[string]any{"text_to_video": map[string]any{"durations": map[string]any{"values": []any{5}}, "resolutions": []any{"720p"}, "aspect_ratios": []any{"16:9"}, "audio_modes": []any{"silent"}}}}, Enabled: true}
	if _, err := service.SaveCapability(t.Context(), input); err == nil {
		t.Fatal("stale capability version must fail")
	}
	input.ExpectedVersion = "cap-v1"
	input.Capability["provider_native_max_n"] = 11
	if _, err := service.SaveCapability(t.Context(), input); err == nil {
		t.Fatal("provider native n > 10 must fail")
	}
	input.Capability["provider_native_max_n"] = 1
	if _, err := service.SaveCapability(t.Context(), input); err != nil {
		t.Fatal(err)
	}
}

func TestCapabilityWriteValidatesEveryDeclaredCombination(t *testing.T) {
	store := &fakeConfigStore{fakeStore: fakeStore{snapshot: Snapshot{}}}
	service := NewService(store)
	input := CapabilityWrite{AccountModelID: 7, CapabilityVersion: "cap-v1", Capability: map[string]any{
		"schema_version": 1, "provider_native_max_n": 1,
		"task_types": map[string]any{"text_to_video": map[string]any{
			"durations": map[string]any{"values": []any{5, 0}}, "resolutions": []any{"720p"}, "aspect_ratios": []any{"16:9"}, "audio_modes": []any{"silent"},
		}},
	}}
	if _, err := service.SaveCapability(t.Context(), input); err == nil {
		t.Fatal("every declared duration/resolution/ratio/audio combination must be valid")
	}
}

func TestEnableStrategyAndRouteRejectUnsafePlansPricesAndMissingCombinations(t *testing.T) {
	store := &fakeConfigStore{fakeStore: fakeStore{snapshot: Snapshot{
		Plans: []PointProduct{{ID: 1, Code: "diluted", PriceCNY: "10", Points: "100", BonusPoints: "20", Enabled: true}},
		Capabilities: []CapabilitySummary{{AccountModelID: 7, ValidationState: "verified", Enabled: true, Capability: map[string]any{
			"schema_version": 1, "provider_native_max_n": 1,
			"task_types": map[string]any{"text_to_video": map[string]any{"durations": map[string]any{"values": []any{5}}, "resolutions": []any{"720p"}, "aspect_ratios": []any{"16:9"}, "audio_modes": []any{"silent"}}},
		}}},
		CostRules:  []CostRuleSummary{{AccountModelID: 7, Validation: "verified", Rates: map[string]any{"combinations": []any{map[string]any{"task_type": "text_to_video", "resolution": "720p", "audio_mode": "silent", "duration_seconds": 5, "cost_cny": "0.5"}}}, Enabled: true}},
		Strategies: []PricingStrategySummary{{ID: 3, StrategyVersion: 1, MinimumNetPointIncomeCNY: "0.10000", TargetMarginRate: "0.25000", ProviderCostBufferRate: "0.10000", PaymentFeeRate: "0.03000", PlatformFixedCostCNY: "0", PlatformOutputSecondCostCNY: "0", PlatformReferenceCostCNY: "0", Enabled: true}},
		Routes:     []RouteConfigSummary{{RouteModelID: 9, PricingStrategyID: 3, CandidateCount: 1, CandidateAccountModelIDs: []int64{7}}},
		PriceRules: []PriceRuleSummary{{StrategyID: 3, TaskType: "text_to_video", Resolution: "720p", AudioMode: "silent", SalesPoints: "7", SafetyPoints: "8", Enabled: true}},
	}}}
	service := NewService(store)
	strategy := StrategyWrite{ID: 3, ExpectedVersion: 1, MinimumNetPointIncomeCNY: "0.10000", PaymentFeeRate: "0.03000", TargetMarginRate: "0.25000", ProviderCostBufferRate: "0.10000", Enabled: true}
	if _, err := service.SaveStrategy(t.Context(), strategy); err == nil {
		t.Fatal("diluted point package must block strategy enable")
	}
	store.snapshot.Plans[0].PriceCNY = "20"
	if _, err := service.SaveStrategy(t.Context(), strategy); err != nil {
		t.Fatal(err)
	}
	route := RouteConfigWrite{RouteModelID: 9, ExpectedVersion: "", ConfigVersion: "route-v2", PricingStrategyID: 3, TaskTypes: []string{"text_to_video"}, VisibleCombinations: []VisibleCombination{{TaskType: "text_to_video", Resolution: "720p", AspectRatio: "16:9", AudioMode: "silent", DurationSeconds: 5}}, MaxOutputCount: 4, Enabled: true}
	if _, err := service.SaveRouteConfig(t.Context(), route); err == nil {
		t.Fatal("price below safety floor must block route enable")
	}
	store.snapshot.PriceRules[0].SalesPoints = "8"
	if _, err := service.SaveRouteConfig(t.Context(), route); err != nil {
		t.Fatal(err)
	}
}

func TestSimulateAndRecalculateUseWorstCandidateAndCreateNewRuleVersion(t *testing.T) {
	store := &fakeConfigStore{fakeStore: fakeStore{snapshot: Snapshot{
		Plans:      []PointProduct{{ID: 1, PriceCNY: "31.25", Points: "100", Enabled: true}},
		Strategies: []PricingStrategySummary{{ID: 3, StrategyVersion: 2, MinimumNetPointIncomeCNY: "0.25260", TargetMarginRate: "0.25000", ProviderCostBufferRate: "0.10000", PlatformFixedCostCNY: "0.15000", PlatformOutputSecondCostCNY: "0.02000", PlatformReferenceCostCNY: "0.03000"}},
		CostRules:  []CostRuleSummary{{AccountModelID: 7, RuleVersion: 1, Rates: map[string]any{"combinations": []any{map[string]any{"task_type": "text_to_video", "resolution": "720p", "audio_mode": "silent", "duration_seconds": 5, "cost_cny": "1.00000"}}}, Enabled: true}, {AccountModelID: 8, RuleVersion: 2, Rates: map[string]any{"combinations": []any{map[string]any{"task_type": "text_to_video", "resolution": "720p", "audio_mode": "silent", "duration_seconds": 5, "cost_cny": "2.00000"}}}, Enabled: true}},
		Routes:     []RouteConfigSummary{{RouteModelID: 9, PricingStrategyID: 3, CandidateAccountModelIDs: []int64{7, 8}}},
		PriceRules: []PriceRuleSummary{{ID: 11, StrategyID: 3, TaskType: "text_to_video", Resolution: "720p", AudioMode: "silent", RuleVersion: 4}},
	}}}
	service := NewService(store)
	result, err := service.Simulate(t.Context(), SimulationRequest{RouteModelID: 9, StrategyID: 3, TaskType: "text_to_video", Resolution: "720p", AudioMode: "silent", DurationSeconds: 5})
	if err != nil {
		t.Fatal(err)
	}
	if result.WorstCandidateCostCNY != "2.00000" || result.SafetyPoints == "0.00000" {
		t.Fatalf("simulation=%#v", result)
	}
	if _, err := service.Recalculate(t.Context(), RecalculateRequest{RouteModelID: 9, StrategyID: 3, Combinations: []SimulationRequest{{TaskType: "text_to_video", Resolution: "720p", AudioMode: "silent", DurationSeconds: 5}}, EffectiveAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if store.price.ExpectedVersion != 4 || store.price.SafetyPoints == "" || store.price.MinimumTaskPoints == "" {
		t.Fatalf("recalculated=%#v", store.price)
	}
}

func TestStrategyRejectsInvalidDecimalAndRateFields(t *testing.T) {
	store := &fakeConfigStore{fakeStore: fakeStore{snapshot: Snapshot{Plans: []PointProduct{{Code: "standard", PriceCNY: "100", Points: "100", Enabled: true}}}}}
	service := NewService(store)
	valid := StrategyWrite{
		Code: "video", Name: "Video", GrossPointValueCNY: "1", MinimumNetPointIncomeCNY: "0.5",
		MaxBonusRatio: "0.2", PaymentFeeRate: "0.03", TargetMarginRate: "0.25", ProviderCostBufferRate: "0.1",
		PlatformFixedCostCNY: "0", PlatformOutputSecondCostCNY: "0", PlatformReferenceCostCNY: "0",
		PlatformAudioFixedCostCNY: "0", PlatformAudioSecondCostCNY: "0", ExactReserveMarkup: "1", MeteredReserveMarkup: "1.1",
	}
	invalid := valid
	invalid.TargetMarginRate = "1"
	if _, err := service.SaveStrategy(t.Context(), invalid); err == nil {
		t.Fatal("target margin >= 1 must fail")
	}
	invalid = valid
	invalid.PlatformFixedCostCNY = "-0.01"
	if _, err := service.SaveStrategy(t.Context(), invalid); err == nil {
		t.Fatal("negative platform cost must fail")
	}
	invalid = valid
	invalid.ExactReserveMarkup = "0.9"
	if _, err := service.SaveStrategy(t.Context(), invalid); err == nil {
		t.Fatal("reserve markup below one must fail")
	}
}

func TestEnabledRouteRequiresVisibleCompatibleCombinationAndTargetMargin(t *testing.T) {
	capability := map[string]any{
		"schema_version": 1, "provider_native_max_n": 1,
		"task_types": map[string]any{"text_to_video": map[string]any{
			"durations": map[string]any{"values": []any{5}}, "resolutions": []any{"720p"},
			"aspect_ratios": []any{"16:9"}, "audio_modes": []any{"silent"},
		}},
	}
	store := &fakeConfigStore{fakeStore: fakeStore{snapshot: Snapshot{
		Plans:        []PointProduct{{Code: "standard", PriceCNY: "100", Points: "100", Enabled: true}},
		Capabilities: []CapabilitySummary{{AccountModelID: 7, Capability: capability, ValidationState: "verified", Enabled: true}},
		Strategies:   []PricingStrategySummary{{ID: 3, MinimumNetPointIncomeCNY: "0.5", PaymentFeeRate: "0.03", TargetMarginRate: "0.25", ProviderCostBufferRate: "0", PlatformFixedCostCNY: "0", PlatformOutputSecondCostCNY: "0", PlatformReferenceCostCNY: "0", Enabled: true}},
		CostRules:    []CostRuleSummary{{AccountModelID: 7, Validation: "verified", Rates: map[string]any{"combinations": []any{map[string]any{"task_type": "text_to_video", "resolution": "720p", "audio_mode": "silent", "duration_seconds": 5, "cost_cny": "3"}}}, Enabled: true}},
		Routes:       []RouteConfigSummary{{RouteModelID: 9, CandidateCount: 1, CandidateAccountModelIDs: []int64{7}}},
		PriceRules:   []PriceRuleSummary{{StrategyID: 3, TaskType: "text_to_video", Resolution: "720p", AudioMode: "silent", SalesPoints: "7", SafetyPoints: "8", Enabled: true}},
	}}}
	service := NewService(store)
	route := RouteConfigWrite{RouteModelID: 9, ConfigVersion: "v2", PricingStrategyID: 3, MaxOutputCount: 4, Enabled: true}
	if _, err := service.SaveRouteConfig(t.Context(), route); err == nil {
		t.Fatal("enabled route without visible combinations must fail")
	}
	route.VisibleCombinations = []VisibleCombination{{TaskType: "text_to_video", Resolution: "1080p", AudioMode: "silent", DurationSeconds: 5}}
	if _, err := service.SaveRouteConfig(t.Context(), route); err == nil {
		t.Fatal("combination unsupported by every candidate must fail")
	}
	route.VisibleCombinations[0].Resolution = "720p"
	if _, err := service.SaveRouteConfig(t.Context(), route); err == nil {
		t.Fatal("sales below target-margin safety floor must fail")
	}
	store.snapshot.PriceRules[0].SalesPoints = "8"
	if _, err := service.SaveRouteConfig(t.Context(), route); err != nil {
		t.Fatal(err)
	}
}

func TestEnabledPriceRuleUsesServerSimulationInsteadOfSubmittedSafety(t *testing.T) {
	store := &fakeConfigStore{fakeStore: fakeStore{snapshot: Snapshot{
		Strategies: []PricingStrategySummary{{ID: 3, MinimumNetPointIncomeCNY: "0.5", TargetMarginRate: "0.25", ProviderCostBufferRate: "0", PlatformFixedCostCNY: "0", PlatformOutputSecondCostCNY: "0", PlatformReferenceCostCNY: "0"}},
		CostRules:  []CostRuleSummary{{AccountModelID: 7, Rates: map[string]any{"combinations": []any{map[string]any{"task_type": "text_to_video", "resolution": "720p", "audio_mode": "silent", "duration_seconds": 5, "cost_cny": "3"}}}, Enabled: true}},
		Routes:     []RouteConfigSummary{{RouteModelID: 9, CandidateAccountModelIDs: []int64{7}}},
	}}}
	service := NewService(store)
	input := PriceRuleWrite{RouteModelID: 9, StrategyID: 3, TaskType: "text_to_video", Resolution: "720p", AudioMode: "silent", DurationSeconds: 5, EffectiveAt: time.Now(), MinimumTaskPoints: "7", SafetyPoints: "0.1", Enabled: true}
	if _, err := service.SavePriceRule(t.Context(), input); err == nil {
		t.Fatal("submitted safety below server-calculated safety must not bypass the gate")
	}
	input.MinimumTaskPoints = "8"
	if _, err := service.SavePriceRule(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	if store.price.SafetyPoints != "8.00000" || store.price.CandidateCostUpperCNY != "3.00000" {
		t.Fatalf("stored server safety=%s cost=%s", store.price.SafetyPoints, store.price.CandidateCostUpperCNY)
	}
}
