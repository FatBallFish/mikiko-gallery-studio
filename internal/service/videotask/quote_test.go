package videotask

import (
	"context"
	"errors"
	"testing"
	"time"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
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

type quotePricingStore struct{ rule videopricingservice.Rule }

func (s *quotePricingStore) GetVideoPriceRule(context.Context, int64, domainvideo.TaskType, domainvideo.Resolution, domainvideo.AudioMode, time.Time) (videopricingservice.Rule, error) {
	return s.rule, nil
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
		Code: "cinema", ConfigVersion: "route-v1", PricingStrategyID: 1, MaxOutputCount: 4,
		TaskTypes:  []domainvideo.TaskType{domainvideo.TaskTypeTextToVideo},
		Candidates: []videoroutingservice.Candidate{{AccountModelID: 1, CapabilityVersion: "cap-v1", Capability: capability}},
	}}
	pricingStore := &quotePricingStore{rule: videopricingservice.Rule{
		StrategyVersion: 1, RuleVersion: 1, SafetyPoints: "1.00000",
		SalesRule: domainvideo.SalesRule{OutputSecondPoints: "2.00000", ReserveMarkup: "1.00000"},
	}}
	service := NewQuoteService(videoroutingservice.NewService(routingStore), videopricingservice.NewService(pricingStore, clock), []byte("quote-test-signing-key-at-least-32-bytes"), clock)
	request := EstimateRequest{RouteModelCode: "cinema", Video: domainvideo.Request{
		TaskType: domainvideo.TaskTypeTextToVideo, Prompt: "lake", DurationSeconds: 5, Resolution: domainvideo.Resolution720P,
		AspectRatio: domainvideo.AspectRatio16x9, AudioMode: domainvideo.AudioModeSilent, OutputCount: 1,
	}}
	estimate, err := service.Estimate(t.Context(), 7, request)
	if err != nil {
		t.Fatal(err)
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
}
