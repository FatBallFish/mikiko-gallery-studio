package adminvideo

import (
	"context"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type fakeConfigStore struct {
	fakeStore
	capability   CapabilityWrite
	route        RouteConfigWrite
	modelContext ModelPricingContext
	routeContext RouteQuoteContext
	rateCard     RateCardWrite
}

func (s *fakeConfigStore) SaveCapability(_ context.Context, input CapabilityWrite) (CapabilitySummary, error) {
	s.capability = input
	return CapabilitySummary{AccountModelID: input.AccountModelID, Version: input.CapabilityVersion, Enabled: input.Enabled}, nil
}
func (s *fakeConfigStore) GetVideoModelPricingContext(context.Context, int64) (ModelPricingContext, error) {
	return s.modelContext, nil
}
func (s *fakeConfigStore) GetVideoRouteQuoteContext(context.Context, int64, time.Time) (RouteQuoteContext, error) {
	return s.routeContext, nil
}
func (s *fakeConfigStore) SaveVideoModelRateCard(_ context.Context, input RateCardWrite) (RateCardSummary, error) {
	s.rateCard = input
	return RateCardSummary{AccountModelID: input.AccountModelID, ProviderCode: input.ProviderCode, PricingSchema: input.PricingSchema, RateVersion: input.ExpectedRateVersion + 1, RateConfig: input.RateConfig, Enabled: input.Enabled}, nil
}
func (s *fakeConfigStore) SaveRouteConfig(_ context.Context, input RouteConfigWrite) (RouteConfigSummary, error) {
	s.route = input
	return RouteConfigSummary{RouteModelID: input.RouteModelID, ConfigVersion: input.ConfigVersion, Enabled: input.Enabled}, nil
}
func (s *fakeConfigStore) DeleteVideoConfig(context.Context, ConfigKind, int64, int64) error {
	return nil
}

func TestSaveVideoModelRateCardValidatesProviderSchemaAndCapability(t *testing.T) {
	capability := map[string]any{
		"schema_version": 1, "provider_native_max_n": 1,
		"task_types": map[string]any{"text_to_video": map[string]any{
			"durations": map[string]any{"values": []any{5}}, "resolutions": []any{"720p"},
			"aspect_ratios": []any{"16:9"}, "audio_modes": []any{"silent"},
		}},
	}
	store := &fakeConfigStore{modelContext: ModelPricingContext{AccountModelID: 7, ProviderCode: "seedance", ModelCode: "doubao-seedance-2-0-260128", Capability: capability}}
	service := NewService(store)
	input := RateCardWrite{
		AccountModelID: 7, ProviderCode: "minimax", PricingSchema: "seedance_token_v1", ExpectedRateVersion: 0,
		RateConfig: map[string]any{
			"resolutions": map[string]any{
				"720p": map[string]any{"without_input_video_million_tokens_cny": "46"},
			},
		},
		Enabled: true,
	}
	if _, err := service.SaveVideoModelRateCard(t.Context(), input); err != nil {
		t.Fatalf("provider and system metadata must be derived from the real model: %v", err)
	}
	if store.rateCard.ProviderCode != "seedance" || store.rateCard.Currency != "CNY" || store.rateCard.PricingSchema != "seedance_token_v1" || store.rateCard.SourceReference == "" || store.rateCard.EffectiveAt.IsZero() {
		t.Fatalf("normalized rate card = %#v", store.rateCard)
	}
}

func TestRouteQuoteSimulationUsesMappedCandidatesAndHighestCNY(t *testing.T) {
	seedanceCapability := domainVideoCapabilityMap("720p")
	minimaxCapability := domainVideoCapabilityMap("768p")
	store := &fakeConfigStore{routeContext: RouteQuoteContext{
		Route:       RouteConfigSummary{RouteModelID: 9, ConfigVersion: "route-v2", MinimumTaskPoints: "0", RoundingStepPoints: 1},
		CNYPerPoint: "0.01", ConversionVersion: "billing-v3",
		Candidates: []RouteQuoteCandidate{
			{
				RouteCandidateID: 11, AccountModelID: 101, ProviderCode: "seedance", ModelCode: "doubao-seedance-2-0-260128", Capability: seedanceCapability,
				RateCard: RateCardSummary{ProviderCode: "seedance", PricingSchema: "seedance_token_v1", RateVersion: 1, RateConfig: map[string]any{
					"resolutions": map[string]any{"720p": map[string]any{"without_input_video_million_tokens_cny": "46"}},
				}},
			},
			{
				RouteCandidateID: 12, AccountModelID: 102, ProviderCode: "minimax", ModelCode: "MiniMax-H3", Capability: minimaxCapability, ResolutionMappings: map[string]string{"720p": "768p"},
				RateCard: RateCardSummary{ProviderCode: "minimax", PricingSchema: "minimax_h3_second_v1", RateVersion: 1, RateConfig: map[string]any{
					"resolutions":      map[string]any{"768p": map[string]any{"output_second_cny": "0.50", "input_video_second_cny": "0.50"}},
					"free_image_count": 5, "extra_image_cny": "0.20", "input_audio_free": true,
				}},
			},
			{
				RouteCandidateID: 13, AccountModelID: 103, ProviderCode: "minimax", ModelCode: "MiniMax-H3",
			},
			{
				RouteCandidateID: 14, AccountModelID: 104, ProviderCode: "minimax", ModelCode: "MiniMax-H3", Capability: minimaxCapability,
				PreflightExclusionCode: "VIDEO_CANDIDATE_NOT_PRICEABLE", ResolutionMappings: map[string]string{"720p": "768p"},
				RateCard: RateCardSummary{ProviderCode: "minimax", PricingSchema: "minimax_h3_second_v1", RateVersion: 1, RateConfig: map[string]any{
					"resolutions":      map[string]any{"768p": map[string]any{"output_second_cny": "100.00", "input_video_second_cny": "100.00"}},
					"free_image_count": 5, "extra_image_cny": "0.20", "input_audio_free": true,
				}},
			},
		},
	}}
	result, err := NewService(store).SimulateRouteQuote(t.Context(), QuoteSimulationRequest{
		RouteModelID: 9, TaskType: "text_to_video", Resolution: "720p", AspectRatio: "16:9", AudioMode: "silent", DurationSeconds: 5, OutputCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 4 || result.HighestAccountModelID != 101 || result.HighestCNY != "4.96800" || result.UnitPoints != "497.00000" {
		t.Fatalf("simulation = %#v", result)
	}
	if result.Candidates[1].MappedResolution != "768p" || !result.Candidates[1].Eligible {
		t.Fatalf("mapped minimax candidate = %#v", result.Candidates[1])
	}
	if result.Candidates[2].ExclusionCode != "VIDEO_CAPABILITY_INVALID" {
		t.Fatalf("invalid capability candidate = %#v", result.Candidates[2])
	}
	if result.Candidates[3].Eligible || result.Candidates[3].ExclusionCode != "VIDEO_CANDIDATE_NOT_PRICEABLE" {
		t.Fatalf("unavailable candidate = %#v", result.Candidates[3])
	}
}

func TestRouteQuoteSimulationReturnsRoutePriceUnavailableWhenEveryCandidateIsExcluded(t *testing.T) {
	store := &fakeConfigStore{routeContext: RouteQuoteContext{
		Route:       RouteConfigSummary{RouteModelID: 9, ConfigVersion: "route-v2", RoundingStepPoints: 1},
		CNYPerPoint: "0.01", ConversionVersion: "billing-v3",
		Candidates: []RouteQuoteCandidate{{
			RouteCandidateID: 11, AccountModelID: 101, ProviderCode: "seedance", ModelCode: "doubao-seedance-2-0-260128",
		}},
	}}
	_, err := NewService(store).SimulateRouteQuote(t.Context(), QuoteSimulationRequest{
		RouteModelID: 9, TaskType: "text_to_video", Resolution: "720p", AspectRatio: "16:9", AudioMode: "silent", DurationSeconds: 5, OutputCount: 1,
	})
	typed, ok := err.(*errs.Error)
	if !ok || typed.Code != "VIDEO_ROUTE_PRICE_UNAVAILABLE" || len(typed.Details["candidates"].([]QuoteSimulationCandidate)) != 1 {
		t.Fatalf("route unavailable error = %#v", err)
	}
}

func domainVideoCapabilityMap(resolution string) map[string]any {
	return map[string]any{
		"schema_version": 1, "provider_native_max_n": 1,
		"task_types": map[string]any{"text_to_video": map[string]any{
			"durations": map[string]any{"values": []any{5}}, "resolutions": []any{resolution},
			"aspect_ratios": []any{"16:9"}, "audio_modes": []any{"silent"},
		}},
	}
}

func TestCapabilityWriteValidatesProviderNativeNAndCAS(t *testing.T) {
	store := &fakeConfigStore{fakeStore: fakeStore{snapshot: Snapshot{Capabilities: []CapabilitySummary{{AccountModelID: 7, Version: "cap-v1"}}}}}
	service := NewService(store)
	input := CapabilityWrite{AccountModelID: 7, ExpectedVersion: "wrong", CapabilityVersion: "cap-v2", Capability: map[string]any{"schema_version": 1, "provider_native_max_n": 1, "task_types": map[string]any{"text_to_video": map[string]any{"durations": map[string]any{"values": []any{5}}, "resolutions": []any{"720p"}, "aspect_ratios": []any{"16:9"}, "audio_modes": []any{"silent"}}}}, ValidationStatus: "verified", Enabled: true}
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

func TestEnabledRouteRequiresEveryCandidateToHaveEnabledRateCard(t *testing.T) {
	store := &fakeConfigStore{fakeStore: fakeStore{snapshot: Snapshot{
		Routes: []RouteConfigSummary{{RouteModelID: 9, CandidateCount: 2, CandidateAccountModelIDs: []int64{7, 8}}},
		Capabilities: []CapabilitySummary{
			{AccountModelID: 7, Capability: domainVideoCapabilityMap("720p"), ValidationState: "verified", Enabled: true},
			{AccountModelID: 8, Capability: domainVideoCapabilityMap("720p"), ValidationState: "verified", Enabled: true},
		},
		RateCards: []RateCardSummary{
			{AccountModelID: 7, PricingSchema: "seedance_token_v1", Enabled: true},
			{AccountModelID: 8, PricingSchema: "minimax_h3_second_v1", Enabled: false},
		},
	}}}
	service := NewService(store)
	route := RouteConfigWrite{RouteModelID: 9, ConfigVersion: "route-v2", MinimumTaskPoints: "0", RoundingStepPoints: 5, TaskTypes: []string{"text_to_video"}, VisibleCombinations: []VisibleCombination{{TaskType: "text_to_video", Resolution: "720p", AspectRatio: "16:9", AudioMode: "silent", DurationSeconds: 5}}, MaxOutputCount: 4, Enabled: true}
	if _, err := service.SaveRouteConfig(t.Context(), route); err == nil {
		t.Fatal("candidate without an enabled rate card must block route enable")
	}
	store.snapshot.RateCards[1].Enabled = true
	if _, err := service.SaveRouteConfig(t.Context(), route); err != nil {
		t.Fatal(err)
	}
}

func TestEnabledRouteRequiresVisibleCompatibleCombination(t *testing.T) {
	capability := map[string]any{
		"schema_version": 1, "provider_native_max_n": 1,
		"task_types": map[string]any{"text_to_video": map[string]any{
			"durations": map[string]any{"values": []any{5}}, "resolutions": []any{"720p"},
			"aspect_ratios": []any{"16:9"}, "audio_modes": []any{"silent"},
		}},
	}
	store := &fakeConfigStore{fakeStore: fakeStore{snapshot: Snapshot{
		Capabilities: []CapabilitySummary{{AccountModelID: 7, Capability: capability, ValidationState: "verified", Enabled: true}},
		Routes:       []RouteConfigSummary{{RouteModelID: 9, CandidateCount: 1, CandidateAccountModelIDs: []int64{7}}},
		RateCards:    []RateCardSummary{{AccountModelID: 7, PricingSchema: "seedance_token_v1", Enabled: true}},
	}}}
	service := NewService(store)
	route := RouteConfigWrite{RouteModelID: 9, ConfigVersion: "v2", MinimumTaskPoints: "0", RoundingStepPoints: 1, MaxOutputCount: 4, Enabled: true}
	if _, err := service.SaveRouteConfig(t.Context(), route); err == nil {
		t.Fatal("enabled route without visible combinations must fail")
	}
	route.VisibleCombinations = []VisibleCombination{{TaskType: "text_to_video", Resolution: "1080p", AudioMode: "silent", DurationSeconds: 5}}
	if _, err := service.SaveRouteConfig(t.Context(), route); err == nil {
		t.Fatal("combination unsupported by every candidate must fail")
	}
	route.VisibleCombinations[0].Resolution = "720p"
	if _, err := service.SaveRouteConfig(t.Context(), route); err != nil {
		t.Fatal(err)
	}
}

func TestEnabledRouteUsesCandidateResolutionMappingForCapabilityValidation(t *testing.T) {
	store := &fakeConfigStore{fakeStore: fakeStore{snapshot: Snapshot{
		Capabilities: []CapabilitySummary{{
			AccountModelID: 7, Capability: domainVideoCapabilityMap("768p"), ValidationState: "verified", Enabled: true,
		}},
		Routes:    []RouteConfigSummary{{RouteModelID: 9, CandidateCount: 1, CandidateAccountModelIDs: []int64{7}}},
		RateCards: []RateCardSummary{{AccountModelID: 7, PricingSchema: "minimax_h3_second_v1", Enabled: true}},
	}}}
	route := RouteConfigWrite{
		RouteModelID: 9, ConfigVersion: "v2", MinimumTaskPoints: "0", RoundingStepPoints: 1, MaxOutputCount: 4, Enabled: true,
		CandidateParameterMappings: map[string]any{"7": map[string]any{"resolutions": map[string]any{"720p": "768p"}}},
		VisibleCombinations:        []VisibleCombination{{TaskType: "text_to_video", Resolution: "720p", AspectRatio: "16:9", AudioMode: "silent", DurationSeconds: 5}},
	}
	if _, err := NewService(store).SaveRouteConfig(t.Context(), route); err != nil {
		t.Fatalf("mapped route combination must match the provider-native capability: %v", err)
	}
}

func TestRouteConfigValidatesMinimumPointsAndRoundingStep(t *testing.T) {
	service := NewService(&fakeConfigStore{})
	route := RouteConfigWrite{RouteModelID: 9, ConfigVersion: "v2", MinimumTaskPoints: "-1", RoundingStepPoints: 1, MaxOutputCount: 1}
	if _, err := service.SaveRouteConfig(t.Context(), route); err == nil {
		t.Fatal("negative minimum task points must be rejected")
	}
	route.MinimumTaskPoints = "0"
	route.RoundingStepPoints = 2
	if _, err := service.SaveRouteConfig(t.Context(), route); err == nil {
		t.Fatal("rounding step outside 1, 5, and 10 must be rejected")
	}
	for _, step := range []int{1, 5, 10} {
		route.RoundingStepPoints = step
		if _, err := service.SaveRouteConfig(t.Context(), route); err != nil {
			t.Fatalf("rounding step %d rejected: %v", step, err)
		}
	}
}

func TestSaveCapabilityAcceptsRequiredFrameInputs(t *testing.T) {
	store := &fakeConfigStore{}
	service := NewService(store)
	capability := map[string]any{
		"schema_version": 1, "provider_native_max_n": 1,
		"task_types": map[string]any{"image_to_video": map[string]any{
			"durations": map[string]any{"values": []any{5}}, "resolutions": []any{"720p"},
			"aspect_ratios": []any{"16:9"}, "audio_modes": []any{"silent"},
			"inputs": map[string]any{"first_frame": map[string]any{
				"required": true, "max_count": 1, "max_bytes": 30 << 20,
				"media_types": []any{"image"}, "formats": []any{"png", "webp"},
			}},
		}},
	}
	if _, err := service.SaveCapability(t.Context(), CapabilityWrite{AccountModelID: 7, CapabilityVersion: "cap-v1", Capability: capability, ValidationStatus: "verified", Enabled: true}); err != nil {
		t.Fatalf("valid required frame capability rejected: %v", err)
	}
}

func TestSaveCapabilityRejectsEnabledUntestedConfiguration(t *testing.T) {
	service := NewService(&fakeConfigStore{})
	capability := map[string]any{
		"schema_version": 1, "provider_native_max_n": 1,
		"task_types": map[string]any{"text_to_video": map[string]any{
			"durations": map[string]any{"values": []any{5}}, "resolutions": []any{"720p"},
			"aspect_ratios": []any{"16:9"}, "audio_modes": []any{"silent"},
		}},
	}
	if _, err := service.SaveCapability(t.Context(), CapabilityWrite{AccountModelID: 7, CapabilityVersion: "cap-v1", Capability: capability, ValidationStatus: "untested", Enabled: true}); err == nil {
		t.Fatal("enabled capability must require verified validation status")
	}
}
