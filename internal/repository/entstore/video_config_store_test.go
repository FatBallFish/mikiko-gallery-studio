package entstore

import (
	"testing"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	adminvideoservice "github.com/fatballfish/pic-gallery/internal/service/adminvideo"
)

func TestDecodeVideoPricingBindings(t *testing.T) {
	bindings := decodeVideoPricingBindings(map[string]any{
		"pricing_bindings": []any{
			map[string]any{
				"task_type":           "text_to_video",
				"resolution":          "720p",
				"aspect_ratio":        "16:9",
				"audio_mode":          "silent",
				"duration_seconds":    10,
				"pricing_strategy_id": 42,
			},
			map[string]any{
				"task_type":           "",
				"resolution":          "720p",
				"audio_mode":          "silent",
				"pricing_strategy_id": 99,
			},
		},
	})

	if len(bindings) != 1 {
		t.Fatalf("expected one valid pricing binding, got %#v", bindings)
	}
	binding := bindings[0]
	if binding.TaskType != domainvideo.TaskTypeTextToVideo || binding.Resolution != domainvideo.Resolution720P || binding.AspectRatio != domainvideo.AspectRatio16x9 || binding.AudioMode != domainvideo.AudioModeSilent || binding.DurationSeconds != 10 || binding.PricingStrategyID != 42 {
		t.Fatalf("unexpected pricing binding: %#v", binding)
	}
}

func TestDecodeVideoPricingBindingsRejectsMalformedValue(t *testing.T) {
	if bindings := decodeVideoPricingBindings(map[string]any{"pricing_bindings": "invalid"}); len(bindings) != 0 {
		t.Fatalf("expected malformed pricing bindings to be ignored, got %#v", bindings)
	}
}

func TestVideoRouteMissingPriceUsesParameterBinding(t *testing.T) {
	route := adminvideoservice.RouteConfigSummary{
		PricingStrategyID: 21,
		VisibleOptions: map[string]any{
			"combinations":     []any{map[string]any{"task_type": "text_to_video", "resolution": "720p", "aspect_ratio": "16:9", "audio_mode": "silent", "duration_seconds": float64(5)}},
			"pricing_bindings": []any{map[string]any{"task_type": "text_to_video", "resolution": "720p", "aspect_ratio": "16:9", "audio_mode": "silent", "duration_seconds": float64(5), "pricing_strategy_id": float64(22)}},
		},
	}
	rules := []adminvideoservice.PriceRuleSummary{{StrategyID: 22, TaskType: "text_to_video", Resolution: "720p", AudioMode: "silent", Enabled: true}}
	if videoRouteHasMissingPrice(route, rules) {
		t.Fatal("bound strategy price must satisfy readiness")
	}
}
