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
			{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "10.00000", Enabled: true},
			{RouteModelID: 2, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "10.00000", Enabled: true},
			{RouteModelID: 2, TaskType: "reference_generate", BaseResolution: "1k", BasePoints: "12.00000", Enabled: true},
		},
		ProviderModels: []ProviderCandidate{
			{AccountModelID: 101, ProviderModelID: 101, ModelCode: "text-model", SupportedAspectRatios: []string{"1:1"}, MaxImageCount: 2},
			{AccountModelID: 102, ProviderModelID: 102, ModelCode: "ref-model", SupportedAspectRatios: []string{"16:9"}, MaxImageCount: 4, MaxReferenceImageCount: 3},
		},
		Candidates: []RouteCandidateConfig{
			{RouteModelID: 2, AccountModelID: 101, Enabled: true},
			{RouteModelID: 2, AccountModelID: 102, Enabled: true},
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
	if !containsString(items[1].TaskTypes, "reference_generate") || items[1].MaxReferenceImageCount != 3 || items[1].MaxOutputImageCount != 4 {
		t.Fatalf("expected visible reference capabilities, got %#v", items[1])
	}
}

func TestListVisibleRouteModelsPreservesZeroReferenceLimit(t *testing.T) {
	resolver := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 4, ReferenceImageMaxCount: 5}})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: ModelRoutingSnapshot{
		RouteModels: []RouteModelConfig{{ID: 1, Code: "basic", Name: "Basic", Visibility: "public", Enabled: true}},
		Prices:      []RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "2.00000", Enabled: true}},
		ProviderModels: []ProviderCandidate{
			{
				AccountModelID:          101,
				ModelCode:               "text-only",
				SupportedTaskTypes:      []string{"text_to_image"},
				SupportedBaseResolution: []string{"1k"},
				MaxImageCount:           2,
				MaxReferenceImageCount:  0,
			},
		},
		Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 101, Enabled: true}},
	}})

	items, err := resolver.ListVisibleRouteModels(context.Background(), nil, map[string]string{"text_to_image": "1.00000"})
	if err != nil {
		t.Fatalf("ListVisibleRouteModels() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one visible route model, got %d", len(items))
	}
	if items[0].MaxReferenceImageCount != 0 {
		t.Fatalf("expected explicit zero reference limit to be preserved, got %#v", items[0])
	}
}

func TestResolveRouteModelSkipsDisabledCandidates(t *testing.T) {
	resolver := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 4, ReferenceImageMaxCount: 2}})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: ModelRoutingSnapshot{
		RouteModels: []RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices:      []RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true}},
		ProviderModels: []ProviderCandidate{
			{AccountModelID: 11, ModelAccountID: 101, ModelCode: "disabled-model", SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"}},
			{AccountModelID: 12, ModelAccountID: 102, ModelCode: "gpt-image-1", SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"}},
		},
		Candidates: []RouteCandidateConfig{
			{RouteModelID: 1, AccountModelID: 11, Priority: 1, Enabled: false},
			{RouteModelID: 1, AccountModelID: 12, Priority: 2, Enabled: true},
		},
	}})

	resolved, err := resolver.ResolveContext(context.Background(), ResolveRequest{RouteModelCode: "plus", TaskType: "text_to_image", BaseResolution: "1k", RequestedOutputImageCount: 1})
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
		Prices:      []RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true}},
		ProviderModels: []ProviderCandidate{
			{AccountModelID: 12, ModelAccountID: 102, ModelCode: "gpt-image-1", SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"}},
		},
		Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Priority: 1, Enabled: true}},
	}})

	if _, err := resolver.ResolveContext(context.Background(), ResolveRequest{
		RouteModelCode:            "staff",
		UserGroupCodes:            []string{"basic"},
		TaskType:                  "text_to_image",
		BaseResolution:            "1k",
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
		Prices:      []RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true}},
		ProviderModels: []ProviderCandidate{
			{AccountModelID: 12, ModelAccountID: 102, ModelCode: "gpt-image-1", SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"}},
		},
		Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Priority: 1, Enabled: true}},
	}})

	if _, err := resolver.ResolveContext(context.Background(), ResolveRequest{
		RouteModelCode:            "staff",
		TaskType:                  "text_to_image",
		BaseResolution:            "1k",
		RequestedOutputImageCount: 1,
	}); err == nil {
		t.Fatal("expected group-only route model to be rejected without user group")
	}
}

func TestResolveRouteModelAutoBaseResolutionUsesExplicitSize(t *testing.T) {
	resolver := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 4, ReferenceImageMaxCount: 2}})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: ModelRoutingSnapshot{
		RouteModels: []RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices: []RoutePriceConfig{
			{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true},
			{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "2k", BasePoints: "2.00000", Enabled: true},
			{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "4k", BasePoints: "4.00000", Enabled: true},
		},
		ProviderModels: []ProviderCandidate{
			{AccountModelID: 12, ModelAccountID: 102, ModelCode: "gpt-image-1", SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"2k"}},
		},
		Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Priority: 1, Enabled: true}},
	}})

	resolved, err := resolver.ResolveContext(context.Background(), ResolveRequest{
		RouteModelCode:            "plus",
		TaskType:                  "text_to_image",
		BaseResolution:            "auto",
		RequestedSize:             "1536x1024",
		RequestedOutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("ResolveContext() error = %v", err)
	}
	if resolved.BaseResolution != "2k" {
		t.Fatalf("expected explicit size to resolve 2k, got %s", resolved.BaseResolution)
	}
	if len(resolved.Providers) != 1 || resolved.Providers[0].ModelCode != "gpt-image-1" {
		t.Fatalf("expected 2k candidate, got %#v", resolved.Providers)
	}
}

func TestResolveRouteModelPixelModeUsesPixelCapabilityWithoutQualityFilter(t *testing.T) {
	resolver := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 4, ReferenceImageMaxCount: 2}})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: ModelRoutingSnapshot{
		RouteModels: []RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices: []RoutePriceConfig{
			{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true},
			{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "2k", BasePoints: "2.00000", Enabled: true},
		},
		ProviderModels: []ProviderCandidate{{
			AccountModelID:          12,
			ModelAccountID:          102,
			ModelCode:               "gpt-image-2",
			SupportedTaskTypes:      []string{"text_to_image"},
			SupportedBaseResolution: []string{"auto", "1k"},
			SizeModes:               []string{"ratio", "pixel"},
			SupportedPixelSizes:     []string{"1024x1024", "1824x1024"},
			SupportedAspectRatios:   []string{"1:1", "16:9"},
		}},
		Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Priority: 1, Enabled: true}},
	}})

	resolved, err := resolver.ResolveContext(context.Background(), ResolveRequest{
		RouteModelCode:            "plus",
		TaskType:                  "text_to_image",
		SizeMode:                  SizeModePixel,
		RequestedSize:             "1824x1024",
		RequestedOutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("ResolveContext() pixel mode error = %v", err)
	}
	if resolved.BaseResolution != "2k" {
		t.Fatalf("expected pixel size to map to 2k price bucket, got %s", resolved.BaseResolution)
	}
	if len(resolved.Providers) != 1 || resolved.Providers[0].ModelCode != "gpt-image-2" {
		t.Fatalf("expected pixel-capable candidate, got %#v", resolved.Providers)
	}
}

func TestResolveRouteModelRejectsUnsupportedExplicitBaseResolution(t *testing.T) {
	resolver := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 4, ReferenceImageMaxCount: 2}})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: ModelRoutingSnapshot{
		RouteModels: []RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices:      []RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "2k", BasePoints: "2.00000", Enabled: true}},
		ProviderModels: []ProviderCandidate{
			{AccountModelID: 12, ModelAccountID: 102, ModelCode: "gpt-image-1", SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"2k"}},
		},
		Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Priority: 1, Enabled: true}},
	}})

	_, err := resolver.ResolveContext(context.Background(), ResolveRequest{
		RouteModelCode:            "plus",
		TaskType:                  "text_to_image",
		BaseResolution:            "banana",
		RequestedSize:             "1536x1024",
		RequestedOutputImageCount: 1,
	})
	appErr, ok := err.(*errs.Error)
	if !ok || appErr.StatusCode != 400 || appErr.Code != errs.CodeImageCapabilityMismatch {
		t.Fatalf("expected unsupported explicit base resolution error, got %#v", err)
	}
}

func TestResolveRouteBaseResolutionWarnsWhenFallingBackToDefault(t *testing.T) {
	var logs bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo})))
	defer slog.SetDefault(oldLogger)

	baseResolution, err := ResolveRouteBaseResolution(
		RouteModelConfig{ID: 1, Code: "plus"},
		"text_to_image",
		"auto",
		"auto",
		map[string]string{"plus": "4k"},
		[]RoutePriceConfig{
			{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", Enabled: true},
			{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "4k", Enabled: true},
		},
	)
	if err != nil {
		t.Fatalf("ResolveRouteBaseResolution() error = %v", err)
	}
	if baseResolution != "4k" {
		t.Fatalf("expected configured default base resolution 4k, got %s", baseResolution)
	}
	output := logs.String()
	if !strings.Contains(output, "route model auto base resolution fell back to default bucket") || !strings.Contains(output, "fallback_source=route_model_default") {
		t.Fatalf("expected fallback warning log, got %q", output)
	}
}

func TestResolveRouteBaseResolutionWarnsWhenFallingBackToFirstConfiguredPrice(t *testing.T) {
	var logs bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo})))
	defer slog.SetDefault(oldLogger)

	baseResolution, err := ResolveRouteBaseResolution(
		RouteModelConfig{ID: 1, Code: "plus"},
		"text_to_image",
		"auto",
		"",
		map[string]string{"plus": "4k"},
		[]RoutePriceConfig{
			{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", Enabled: true},
			{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "2k", Enabled: true},
		},
	)
	if err != nil {
		t.Fatalf("ResolveRouteBaseResolution() error = %v", err)
	}
	if baseResolution != "1k" {
		t.Fatalf("expected first configured base resolution 1k, got %s", baseResolution)
	}
	output := logs.String()
	if !strings.Contains(output, "route model auto base resolution fell back to default bucket") || !strings.Contains(output, "fallback_source=first_configured_price") {
		t.Fatalf("expected first-price fallback warning log, got %q", output)
	}
}
