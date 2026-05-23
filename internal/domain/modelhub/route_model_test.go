package modelhub

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type staticRoutingSource struct {
	snapshot ModelRoutingSnapshot
}

func (s staticRoutingSource) ModelRoutingConfig(context.Context) (ModelRoutingSnapshot, error) {
	return s.snapshot, nil
}

func TestListVisibleRouteModelsMergesGroupsAndUsesLowestMultiplier(t *testing.T) {
	resolver := NewResolver(config.Config{})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: ModelRoutingSnapshot{
		RouteModels: []RouteModelConfig{
			{ID: 1, Code: "basic", Name: "Basic", Visibility: "public", Enabled: true, SortOrder: 1},
			{ID: 2, Code: "plus", Name: "Plus", Visibility: "groups", Enabled: true, SortOrder: 2},
		},
		Groups: []UserGroupConfig{
			{ID: 10, Code: "vip", Multiplier: "0.80000", Status: "enabled"},
			{ID: 20, Code: "staff", Multiplier: "0.60000", Status: "enabled"},
		},
		Visibility: []RouteVisibilityConfig{
			{RouteModelID: 2, GroupID: 10},
			{RouteModelID: 2, GroupID: 20},
		},
		Prices: []RoutePriceConfig{
			{RouteModelID: 1, TaskType: "text_to_image", Quality: "1k", BasePoints: "10.00000", Enabled: true},
			{RouteModelID: 2, TaskType: "text_to_image", Quality: "1k", BasePoints: "10.00000", Enabled: true},
		},
	}})

	items, err := resolver.ListVisibleRouteModels(context.Background(), []string{"vip", "staff"}, map[string]string{"text_to_image": "1.00000"})
	if err != nil {
		t.Fatalf("ListVisibleRouteModels() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 visible route models, got %d", len(items))
	}
	if items[0].Code != "basic" || items[0].EffectiveMultiplier != "1.00000" {
		t.Fatalf("unexpected public model: %#v", items[0])
	}
	if items[1].Code != "plus" || items[1].EffectiveMultiplier != "0.60000" {
		t.Fatalf("expected lowest matched group multiplier for plus, got %#v", items[1])
	}
	if got := items[1].Prices[0].ChargedPoints; got != "6.00000" {
		t.Fatalf("expected charged points at 5 decimal places, got %s", got)
	}
	if got := items[1].Prices[0].DisplayPoints; got != "6.00" {
		t.Fatalf("expected display points at 2 decimal places, got %s", got)
	}
}

func TestResolveRouteModelSkipsDisabledCandidates(t *testing.T) {
	resolver := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 4, ReferenceImageMaxCount: 2}})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: ModelRoutingSnapshot{
		RouteModels: []RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices:      []RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", Quality: "1k", BasePoints: "1.00000", Enabled: true}},
		ProviderModels: []ProviderCandidate{
			{AccountModelID: 11, ModelAccountID: 101, ModelCode: "disabled-model", SupportedTaskTypes: []string{"text_to_image"}, SupportedQualities: []string{"1k"}},
			{AccountModelID: 12, ModelAccountID: 102, ModelCode: "gpt-image-1", SupportedTaskTypes: []string{"text_to_image"}, SupportedQualities: []string{"1k"}},
		},
		Candidates: []RouteCandidateConfig{
			{RouteModelID: 1, AccountModelID: 11, Priority: 1, Enabled: false},
			{RouteModelID: 1, AccountModelID: 12, Priority: 2, Enabled: true},
		},
	}})

	resolved, err := resolver.ResolveContext(context.Background(), ResolveRequest{RouteModelCode: "plus", TaskType: "text_to_image", RequestedQuality: "1k", RequestedOutputImageCount: 1})
	if err != nil {
		t.Fatalf("ResolveContext() error = %v", err)
	}
	if len(resolved.Providers) != 1 || resolved.Providers[0].ModelCode != "gpt-image-1" {
		t.Fatalf("expected enabled fallback candidate only, got %#v", resolved.Providers)
	}
}

func TestResolveRouteModelRejectsInvisibleGroupModel(t *testing.T) {
	resolver := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 4, ReferenceImageMaxCount: 2}})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: ModelRoutingSnapshot{
		RouteModels: []RouteModelConfig{{ID: 1, Code: "staff", Name: "Staff", Visibility: "groups", Enabled: true}},
		Groups:      []UserGroupConfig{{ID: 10, Code: "staff", Multiplier: "0.50000", Status: "enabled"}},
		Visibility:  []RouteVisibilityConfig{{RouteModelID: 1, GroupID: 10}},
		Prices:      []RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", Quality: "1k", BasePoints: "1.00000", Enabled: true}},
		ProviderModels: []ProviderCandidate{
			{AccountModelID: 12, ModelAccountID: 102, ModelCode: "gpt-image-1", SupportedTaskTypes: []string{"text_to_image"}, SupportedQualities: []string{"1k"}},
		},
		Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Priority: 1, Enabled: true}},
	}})

	if _, err := resolver.ResolveContext(context.Background(), ResolveRequest{
		RouteModelCode:            "staff",
		UserGroupCodes:            []string{"basic"},
		TaskType:                  "text_to_image",
		RequestedQuality:          "1k",
		RequestedOutputImageCount: 1,
	}); err == nil {
		t.Fatal("expected invisible route model to be rejected")
	}
}

func TestResolveRouteModelRejectsGroupModelWithoutUserGroup(t *testing.T) {
	resolver := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 4, ReferenceImageMaxCount: 2}})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: ModelRoutingSnapshot{
		RouteModels: []RouteModelConfig{{ID: 1, Code: "staff", Name: "Staff", Visibility: "groups", Enabled: true}},
		Groups:      []UserGroupConfig{{ID: 10, Code: "staff", Multiplier: "0.50000", Status: "enabled"}},
		Visibility:  []RouteVisibilityConfig{{RouteModelID: 1, GroupID: 10}},
		Prices:      []RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", Quality: "1k", BasePoints: "1.00000", Enabled: true}},
		ProviderModels: []ProviderCandidate{
			{AccountModelID: 12, ModelAccountID: 102, ModelCode: "gpt-image-1", SupportedTaskTypes: []string{"text_to_image"}, SupportedQualities: []string{"1k"}},
		},
		Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Priority: 1, Enabled: true}},
	}})

	if _, err := resolver.ResolveContext(context.Background(), ResolveRequest{
		RouteModelCode:            "staff",
		TaskType:                  "text_to_image",
		RequestedQuality:          "1k",
		RequestedOutputImageCount: 1,
	}); err == nil {
		t.Fatal("expected group-only route model to be rejected without user group")
	}
}

func TestResolveRouteModelAutoQualityUsesExplicitSize(t *testing.T) {
	resolver := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 4, ReferenceImageMaxCount: 2}})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: ModelRoutingSnapshot{
		RouteModels: []RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices: []RoutePriceConfig{
			{RouteModelID: 1, TaskType: "text_to_image", Quality: "1k", BasePoints: "1.00000", Enabled: true},
			{RouteModelID: 1, TaskType: "text_to_image", Quality: "2k", BasePoints: "2.00000", Enabled: true},
			{RouteModelID: 1, TaskType: "text_to_image", Quality: "4k", BasePoints: "4.00000", Enabled: true},
		},
		ProviderModels: []ProviderCandidate{
			{AccountModelID: 12, ModelAccountID: 102, ModelCode: "gpt-image-1", SupportedTaskTypes: []string{"text_to_image"}, SupportedQualities: []string{"2k"}},
		},
		Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Priority: 1, Enabled: true}},
	}})

	resolved, err := resolver.ResolveContext(context.Background(), ResolveRequest{
		RouteModelCode:            "plus",
		TaskType:                  "text_to_image",
		RequestedQuality:          "auto",
		RequestedSize:             "1536x1024",
		RequestedOutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("ResolveContext() error = %v", err)
	}
	if resolved.ResolvedQualityBucket != "2k" {
		t.Fatalf("expected explicit size to resolve 2k, got %s", resolved.ResolvedQualityBucket)
	}
	if len(resolved.Providers) != 1 || resolved.Providers[0].ModelCode != "gpt-image-1" {
		t.Fatalf("expected 2k candidate, got %#v", resolved.Providers)
	}
}

func TestResolveRouteModelRejectsUnsupportedExplicitQuality(t *testing.T) {
	resolver := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 4, ReferenceImageMaxCount: 2}})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: ModelRoutingSnapshot{
		RouteModels: []RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices:      []RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", Quality: "2k", BasePoints: "2.00000", Enabled: true}},
		ProviderModels: []ProviderCandidate{
			{AccountModelID: 12, ModelAccountID: 102, ModelCode: "gpt-image-1", SupportedTaskTypes: []string{"text_to_image"}, SupportedQualities: []string{"2k"}},
		},
		Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Priority: 1, Enabled: true}},
	}})

	_, err := resolver.ResolveContext(context.Background(), ResolveRequest{
		RouteModelCode:            "plus",
		TaskType:                  "text_to_image",
		RequestedQuality:          "banana",
		RequestedSize:             "1536x1024",
		RequestedOutputImageCount: 1,
	})
	appErr, ok := err.(*errs.Error)
	if !ok || appErr.StatusCode != 400 || appErr.Code != errs.CodeImageCapabilityMismatch {
		t.Fatalf("expected unsupported explicit quality error, got %#v", err)
	}
}

func TestResolveRouteQualityWarnsWhenFallingBackToDefault(t *testing.T) {
	var logs bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo})))
	defer slog.SetDefault(oldLogger)

	quality, err := ResolveRouteQuality(
		RouteModelConfig{ID: 1, Code: "plus"},
		"text_to_image",
		"auto",
		"auto",
		map[string]string{"plus": "4k"},
		[]RoutePriceConfig{
			{RouteModelID: 1, TaskType: "text_to_image", Quality: "1k", Enabled: true},
			{RouteModelID: 1, TaskType: "text_to_image", Quality: "4k", Enabled: true},
		},
	)
	if err != nil {
		t.Fatalf("ResolveRouteQuality() error = %v", err)
	}
	if quality != "4k" {
		t.Fatalf("expected configured default quality 4k, got %s", quality)
	}
	output := logs.String()
	if !strings.Contains(output, "route model auto quality fell back to default bucket") || !strings.Contains(output, "fallback_source=route_model_default") {
		t.Fatalf("expected fallback warning log, got %q", output)
	}
}

func TestResolveRouteQualityWarnsWhenFallingBackToFirstConfiguredPrice(t *testing.T) {
	var logs bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo})))
	defer slog.SetDefault(oldLogger)

	quality, err := ResolveRouteQuality(
		RouteModelConfig{ID: 1, Code: "plus"},
		"text_to_image",
		"auto",
		"",
		map[string]string{"plus": "4k"},
		[]RoutePriceConfig{
			{RouteModelID: 1, TaskType: "text_to_image", Quality: "1k", Enabled: true},
			{RouteModelID: 1, TaskType: "text_to_image", Quality: "2k", Enabled: true},
		},
	)
	if err != nil {
		t.Fatalf("ResolveRouteQuality() error = %v", err)
	}
	if quality != "1k" {
		t.Fatalf("expected first configured quality 1k, got %s", quality)
	}
	output := logs.String()
	if !strings.Contains(output, "route model auto quality fell back to default bucket") || !strings.Contains(output, "fallback_source=first_configured_price") {
		t.Fatalf("expected first-price fallback warning log, got %q", output)
	}
}
