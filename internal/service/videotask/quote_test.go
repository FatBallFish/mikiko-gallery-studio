package videotask

import (
	"context"
	"errors"
	"testing"
	"time"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	adminvideo "github.com/fatballfish/pic-gallery/internal/service/adminvideo"
	videopricingservice "github.com/fatballfish/pic-gallery/internal/service/videopricing"
	videoroutingservice "github.com/fatballfish/pic-gallery/internal/service/videorouting"
)

type quoteRoutingStore struct{ group videoroutingservice.Group }

func (s *quoteRoutingStore) GetVideoGroup(context.Context, string) (videoroutingservice.Group, error) {
	return s.group, nil
}
func (s *quoteRoutingStore) ListVideoGroups(context.Context) ([]videoroutingservice.Group, error) {
	return []videoroutingservice.Group{s.group}, nil
}

type quotePricingStore struct {
	result adminvideo.QuoteSimulationResult
}

func (s *quotePricingStore) SimulateRouteQuote(_ context.Context, _ adminvideo.QuoteSimulationRequest) (adminvideo.QuoteSimulationResult, error) {
	return s.result, nil
}

func TestQuoteLocksNativeRoutePrice(t *testing.T) {
	now := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	capability := domainvideo.Capability{SchemaVersion: 1, ProviderNativeMaxN: 1, TaskTypes: map[domainvideo.TaskType]domainvideo.TaskCapability{
		domainvideo.TaskTypeTextToVideo: {
			Durations: domainvideo.DiscreteIntValues(5, 10), Resolutions: []domainvideo.Resolution{domainvideo.Resolution720P},
			AspectRatios: []domainvideo.AspectRatio{domainvideo.AspectRatio16x9}, AudioModes: []domainvideo.AudioMode{domainvideo.AudioModeSilent},
		},
	}}
	routingStore := &quoteRoutingStore{group: videoroutingservice.Group{
		RouteModelID: 9, Code: "cinema", ConfigVersion: "route-v2", MaxOutputCount: 1,
		TaskTypes: []domainvideo.TaskType{domainvideo.TaskTypeTextToVideo}, Candidates: []videoroutingservice.Candidate{{RouteCandidateID: 1, AccountModelID: 2, ModelAccountID: 3, ModelCode: "seedance-2.5", AdapterType: "seedance", CapabilityVersion: "cap-v1", Capability: capability}},
	}}
	pricingStore := &quotePricingStore{result: nativeSimulation(9, "route-v2", 2, "2.00000")}
	service := NewQuoteService(videoroutingservice.NewService(routingStore), videopricingservice.NewService(pricingStore, func() time.Time { return now }), []byte("quote-test-signing-key-at-least-32-bytes"), func() time.Time { return now })
	request := EstimateRequest{RouteModelCode: "cinema", Video: domainvideo.Request{TaskType: domainvideo.TaskTypeTextToVideo, Prompt: "test", DurationSeconds: 10, Resolution: domainvideo.Resolution720P, AspectRatio: domainvideo.AspectRatio16x9, AudioMode: domainvideo.AudioModeSilent, OutputCount: 1}}
	if _, err := service.Estimate(t.Context(), 7, request); err != nil {
		t.Fatal(err)
	}
	if estimate, err := service.Estimate(t.Context(), 7, request); err != nil || estimate.UnitPoints != "2.00000" {
		t.Fatalf("native estimate=%#v err=%v", estimate, err)
	}
}

func TestQuoteUsesHighestPriceWithoutChangingRouteExecutionPriority(t *testing.T) {
	now := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	capability := domainvideo.Capability{SchemaVersion: 1, ProviderNativeMaxN: 1, TaskTypes: map[domainvideo.TaskType]domainvideo.TaskCapability{
		domainvideo.TaskTypeTextToVideo: {
			Durations: domainvideo.DiscreteIntValues(5), Resolutions: []domainvideo.Resolution{domainvideo.Resolution720P},
			AspectRatios: []domainvideo.AspectRatio{domainvideo.AspectRatio16x9}, AudioModes: []domainvideo.AudioMode{domainvideo.AudioModeSilent},
		},
	}}
	routingStore := &quoteRoutingStore{group: videoroutingservice.Group{
		RouteModelID: 9, Code: "cinema", ConfigVersion: "route-v2", MaxOutputCount: 1,
		TaskTypes: []domainvideo.TaskType{domainvideo.TaskTypeTextToVideo}, Candidates: []videoroutingservice.Candidate{
			{RouteCandidateID: 1, AccountModelID: 101, ModelAccountID: 1001, ModelCode: "doubao-seedance-2-5", AdapterType: "seedance", CapabilityVersion: "seedance-cap-v1", Capability: capability},
			{RouteCandidateID: 2, AccountModelID: 202, ModelAccountID: 2002, ModelCode: "MiniMax-H3", AdapterType: "minimax", CapabilityVersion: "minimax-cap-v1", Capability: capability},
		},
	}}
	pricingStore := &quotePricingStore{result: adminvideo.QuoteSimulationResult{
		RouteModelID: 9, ConfigVersion: "route-v2", HighestAccountModelID: 202,
		HighestCNY: "5.00000", CNYPerPoint: "0.01000", ConversionVersion: "billing-v2",
		RoundingStepPoints: 1, UnitPoints: "500.00000", TotalPoints: "500.00000",
		Candidates: []adminvideo.QuoteSimulationCandidate{
			{AccountModelID: 101, ProviderCode: "seedance", Eligible: true, EstimatedCNY: "4.00000", RateVersion: 3},
			{AccountModelID: 202, ProviderCode: "minimax", Eligible: true, EstimatedCNY: "5.00000", RateVersion: 7},
		},
	}}
	service := NewQuoteService(videoroutingservice.NewService(routingStore), videopricingservice.NewService(pricingStore, func() time.Time { return now }), []byte("quote-test-signing-key-at-least-32-bytes"), func() time.Time { return now })
	estimate, err := service.Estimate(t.Context(), 7, EstimateRequest{RouteModelCode: "cinema", Video: domainvideo.Request{
		TaskType: domainvideo.TaskTypeTextToVideo, Prompt: "test", DurationSeconds: 5, Resolution: domainvideo.Resolution720P,
		AspectRatio: domainvideo.AspectRatio16x9, AudioMode: domainvideo.AudioModeSilent, OutputCount: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if estimate.AccountModelID != 101 {
		t.Fatalf("execution account model = %d, want route-priority candidate 101", estimate.AccountModelID)
	}
	if estimate.PricingSnapshot["highest_account_model_id"] != int64(202) {
		t.Fatalf("highest quote model snapshot = %#v", estimate.PricingSnapshot["highest_account_model_id"])
	}
	if _, exists := estimate.PricingSnapshot["sales_rule"]; exists {
		t.Fatalf("native pricing snapshot must not contain legacy sales_rule: %#v", estimate.PricingSnapshot)
	}
}

func TestQuoteSkipsRoutePriorityCandidateExcludedFromPricing(t *testing.T) {
	now := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	capability := domainvideo.Capability{SchemaVersion: 1, ProviderNativeMaxN: 1, TaskTypes: map[domainvideo.TaskType]domainvideo.TaskCapability{
		domainvideo.TaskTypeTextToVideo: {
			Durations: domainvideo.DiscreteIntValues(5), Resolutions: []domainvideo.Resolution{domainvideo.Resolution720P},
			AspectRatios: []domainvideo.AspectRatio{domainvideo.AspectRatio16x9}, AudioModes: []domainvideo.AudioMode{domainvideo.AudioModeSilent},
		},
	}}
	routingStore := &quoteRoutingStore{group: videoroutingservice.Group{
		RouteModelID: 9, Code: "cinema", ConfigVersion: "route-v2", MaxOutputCount: 1,
		TaskTypes: []domainvideo.TaskType{domainvideo.TaskTypeTextToVideo}, Candidates: []videoroutingservice.Candidate{
			{RouteCandidateID: 1, AccountModelID: 101, ModelAccountID: 1001, ModelCode: "doubao-seedance-2-5", AdapterType: "seedance", CapabilityVersion: "seedance-cap-v1", Capability: capability},
			{RouteCandidateID: 2, AccountModelID: 202, ModelAccountID: 2002, ModelCode: "MiniMax-H3", AdapterType: "minimax", CapabilityVersion: "minimax-cap-v1", Capability: capability},
		},
	}}
	pricingStore := &quotePricingStore{result: adminvideo.QuoteSimulationResult{
		RouteModelID: 9, ConfigVersion: "route-v2", HighestAccountModelID: 202,
		HighestCNY: "5.00000", CNYPerPoint: "0.01000", ConversionVersion: "billing-v2",
		RoundingStepPoints: 1, UnitPoints: "500.00000", TotalPoints: "500.00000",
		Candidates: []adminvideo.QuoteSimulationCandidate{
			{RouteCandidateID: 1, AccountModelID: 101, ProviderCode: "seedance", Eligible: false, ExclusionCode: "VIDEO_RATE_CARD_MISSING"},
			{RouteCandidateID: 2, AccountModelID: 202, ProviderCode: "minimax", Eligible: true, EstimatedCNY: "5.00000", RateVersion: 7},
		},
	}}
	service := NewQuoteService(videoroutingservice.NewService(routingStore), videopricingservice.NewService(pricingStore, func() time.Time { return now }), []byte("quote-test-signing-key-at-least-32-bytes"), func() time.Time { return now })
	estimate, err := service.Estimate(t.Context(), 7, EstimateRequest{RouteModelCode: "cinema", Video: domainvideo.Request{
		TaskType: domainvideo.TaskTypeTextToVideo, Prompt: "test", DurationSeconds: 5, Resolution: domainvideo.Resolution720P,
		AspectRatio: domainvideo.AspectRatio16x9, AudioMode: domainvideo.AudioModeSilent, OutputCount: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if estimate.AccountModelID != 202 || estimate.RouteCandidateID != 2 {
		t.Fatalf("execution candidate = %#v, want first priceable route candidate", estimate)
	}
}

func TestQuoteTokenRejectsTamperingExpiryUserAndVersionChanges(t *testing.T) {
	now := time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	capability := domainvideo.Capability{SchemaVersion: 1, ProviderNativeMaxN: 1, TaskTypes: map[domainvideo.TaskType]domainvideo.TaskCapability{
		domainvideo.TaskTypeTextToVideo: {
			Durations: domainvideo.DiscreteIntValues(5), Resolutions: []domainvideo.Resolution{domainvideo.Resolution720P},
			AspectRatios: []domainvideo.AspectRatio{domainvideo.AspectRatio16x9}, AudioModes: []domainvideo.AudioMode{domainvideo.AudioModeSilent},
		},
	}}
	routingStore := &quoteRoutingStore{group: videoroutingservice.Group{
		RouteModelID: 8, Code: "cinema", ConfigVersion: "route-v1", MaxOutputCount: 4,
		TaskTypes:  []domainvideo.TaskType{domainvideo.TaskTypeTextToVideo},
		Candidates: []videoroutingservice.Candidate{{RouteCandidateID: 10, AccountModelID: 11, ModelAccountID: 12, ModelCode: "seedance-2-5", AdapterType: "seedance", CapabilityVersion: "cap-v1", Capability: capability}},
	}}
	pricingStore := &quotePricingStore{result: nativeSimulation(8, "route-v1", 11, "10.00000")}
	service := NewQuoteService(videoroutingservice.NewService(routingStore), videopricingservice.NewService(pricingStore, clock), []byte("quote-test-signing-key-at-least-32-bytes"), clock)
	request := EstimateRequest{RouteModelCode: "cinema", Video: domainvideo.Request{
		TaskType: domainvideo.TaskTypeTextToVideo, Prompt: "lake", DurationSeconds: 5, Resolution: domainvideo.Resolution720P,
		AspectRatio: domainvideo.AspectRatio16x9, AudioMode: domainvideo.AudioModeSilent, OutputCount: 1,
	}}
	estimate, err := service.Estimate(t.Context(), 7, request)
	if err != nil {
		t.Fatal(err)
	}
	if estimate.RouteCandidateID != 10 || estimate.AccountModelID != 11 || estimate.ModelAccountID != 12 || estimate.ProviderCode != "seedance" || estimate.ModelCode != "seedance-2-5" {
		t.Fatalf("estimate candidate snapshot = %#v", estimate)
	}
	if _, err := service.Verify(t.Context(), 7, request, estimate.QuoteToken); err != nil {
		t.Fatalf("valid quote rejected: %v", err)
	}
	if _, err := service.Verify(t.Context(), 7, request, estimate.QuoteToken+"x"); err == nil {
		t.Fatal("tampered quote must be rejected")
	}
	if _, err := service.Verify(t.Context(), 8, request, estimate.QuoteToken); err == nil {
		t.Fatal("quote must be bound to the user")
	}
	changed := request
	changed.Video.Prompt = "changed"
	if _, err := service.Verify(t.Context(), 7, changed, estimate.QuoteToken); err == nil {
		t.Fatal("quote must be bound to the normalized request")
	}
	now = now.Add(121 * time.Second)
	if _, err := service.Verify(t.Context(), 7, request, estimate.QuoteToken); err == nil {
		t.Fatal("expired quote must be rejected")
	}

	now = now.Add(-121 * time.Second)
	routingStore.group.ConfigVersion = "route-v2"
	if _, err := service.Verify(t.Context(), 7, request, estimate.QuoteToken); err == nil {
		t.Fatal("stale route configuration must invalidate the quote")
	} else {
		var target interface{ Error() string }
		if !errors.As(err, &target) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	routingStore.group.ConfigVersion = "route-v1"
	pricingStore.result.Candidates[0].RateVersion++
	if _, err := service.Verify(t.Context(), 7, request, estimate.QuoteToken); err == nil {
		t.Fatal("changed rate-card version must invalidate the quote")
	}
}

func nativeSimulation(routeID int64, configVersion string, accountModelID int64, unitPoints string) adminvideo.QuoteSimulationResult {
	return adminvideo.QuoteSimulationResult{
		RouteModelID: routeID, ConfigVersion: configVersion, HighestAccountModelID: accountModelID,
		HighestCNY: "0.10000", CNYPerPoint: "0.01000", ConversionVersion: "billing-v1",
		RoundingStepPoints: 1, UnitPoints: unitPoints, TotalPoints: unitPoints,
		Candidates: []adminvideo.QuoteSimulationCandidate{{AccountModelID: accountModelID, CapabilityVersion: "cap-v1", PricingSchema: "seedance_token_v1", RateVersion: 1, Eligible: true, EstimatedCNY: "0.10000"}},
	}
}
