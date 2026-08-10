package modelhub

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"reflect"
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
			{RouteModelID: 2, TaskType: "image_edit", BaseResolution: "1k", BasePoints: "12.00000", Enabled: true},
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
	if !containsString(items[1].TaskTypes, "image_edit") || items[1].MaxReferenceImageCount != 3 || items[1].MaxOutputImageCount != 4 {
		t.Fatalf("expected visible reference capabilities, got %#v", items[1])
	}
}

func TestListVisibleRouteModelsExposesAutoBaseResolutionByTaskType(t *testing.T) {
	resolver := NewResolver(config.Config{Billing: config.BillingConfig{
		AutoBaseResolutionDefaultByGroup: map[string]string{"plus": "2k"},
	}})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: ModelRoutingSnapshot{
		RouteModels: []RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices: []RoutePriceConfig{
			{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "2k", BasePoints: "2.00000", Enabled: true},
			{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "4k", BasePoints: "4.00000", Enabled: true},
			{RouteModelID: 1, TaskType: "image_edit", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true},
		},
	}})

	items, err := resolver.ListVisibleRouteModels(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("ListVisibleRouteModels: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("visible route model count = %d, want 1", len(items))
	}
	want := map[string]string{"text_to_image": "2k", "image_edit": "1k"}
	if !reflect.DeepEqual(items[0].AutoBaseResolutionByTaskType, want) {
		t.Fatalf("auto base resolution map = %#v, want %#v", items[0].AutoBaseResolutionByTaskType, want)
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

func TestResolveRouteModelFiltersCandidatesByGenerationCapabilitiesWithoutUsingBatchLimitAsTaskLimit(t *testing.T) {
	resolver := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 5, ReferenceImageMaxCount: 4}})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: ModelRoutingSnapshot{
		RouteModels: []RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices:      []RoutePriceConfig{{RouteModelID: 1, TaskType: "image_edit", BaseResolution: "1K", BasePoints: "1.00000", Enabled: true}},
		ProviderModels: []ProviderCandidate{
			{AccountModelID: 11, ModelCode: "text-only", SupportedTaskTypes: []string{"image_edit"}, SupportedBaseResolution: []string{"1k"}, Quality: []string{"auto"}, SupportedAspectRatios: []string{"16:9"}, MaxImageCount: 2},
			{AccountModelID: 12, ModelCode: "reference-model", SupportedTaskTypes: []string{"image_edit"}, SupportedBaseResolution: []string{"1k"}, Quality: []string{"auto"}, SupportedAspectRatios: []string{"16:9"}, MaxImageCount: 2, MaxReferenceImageCount: 2, SupportsImageInput: true},
			{AccountModelID: 13, ModelCode: "unhealthy-model", SupportedTaskTypes: []string{"image_edit"}, SupportedBaseResolution: []string{"1k"}, Quality: []string{"auto"}, SupportedAspectRatios: []string{"1:1"}, MaxImageCount: 2, MaxReferenceImageCount: 2, SupportsImageInput: true, HealthStatus: "disabled"},
			{AccountModelID: 14, ModelCode: "single-output", SupportedTaskTypes: []string{"image_edit"}, SupportedBaseResolution: []string{"1k"}, Quality: []string{"auto"}, SupportedAspectRatios: []string{"16:9"}, MaxImageCount: 1, MaxReferenceImageCount: 2, SupportsImageInput: true},
			{AccountModelID: 15, ModelCode: "zero-reference-limit", SupportedTaskTypes: []string{"image_edit"}, SupportedBaseResolution: []string{"1k"}, Quality: []string{"auto"}, SupportedAspectRatios: []string{"16:9"}, MaxImageCount: 2, MaxReferenceImageCount: 0, SupportsImageInput: true},
		},
		Candidates: []RouteCandidateConfig{
			{RouteModelID: 1, AccountModelID: 11, Priority: 1, Enabled: true},
			{RouteModelID: 1, AccountModelID: 12, Priority: 2, Enabled: true},
			{RouteModelID: 1, AccountModelID: 13, Priority: 3, Enabled: true},
			{RouteModelID: 1, AccountModelID: 14, Priority: 4, Enabled: true},
			{RouteModelID: 1, AccountModelID: 15, Priority: 5, Enabled: true},
		},
	}})

	resolved, err := resolver.ResolveContext(context.Background(), ResolveRequest{
		RouteModelCode:            "plus",
		TaskType:                  "image_edit",
		SizeMode:                  "ratio",
		AspectRatio:               "16:9",
		BaseResolution:            "1k",
		Quality:                   "auto",
		RequestedOutputImageCount: 2,
		ReferenceImageCount:       1,
	})
	if err != nil {
		t.Fatalf("ResolveContext() error = %v", err)
	}
	if len(resolved.Providers) != 2 || resolved.Providers[0].ModelCode != "reference-model" || resolved.Providers[1].ModelCode != "single-output" {
		t.Fatalf("expected both compatible candidates regardless of per-request batch size, got %#v", resolved.Providers)
	}
}

func TestResolveRouteModelFiltersMaskIncapableCandidates(t *testing.T) {
	resolver := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 2, ReferenceImageMaxCount: 2}})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: ModelRoutingSnapshot{
		RouteModels: []RouteModelConfig{{ID: 1, Code: "edit", Name: "Edit", Visibility: "public", Enabled: true}},
		Prices:      []RoutePriceConfig{{RouteModelID: 1, TaskType: "image_edit", BaseResolution: "1K", BasePoints: "1.00000", Enabled: true}},
		ProviderModels: []ProviderCandidate{
			{AccountModelID: 11, ModelCode: "without-mask", SupportedTaskTypes: []string{"image_edit"}, Quality: []string{"auto"}, MaxImageCount: 1, MaxReferenceImageCount: 1, SupportsImageInput: true},
			{AccountModelID: 12, ModelCode: "with-mask", SupportedTaskTypes: []string{"image_edit"}, Quality: []string{"auto"}, MaxImageCount: 1, MaxReferenceImageCount: 1, SupportsImageInput: true, SupportsMask: true},
		},
		Candidates: []RouteCandidateConfig{
			{RouteModelID: 1, AccountModelID: 11, Priority: 1, Enabled: true},
			{RouteModelID: 1, AccountModelID: 12, Priority: 2, Enabled: true},
		},
	}})

	resolved, err := resolver.ResolveContext(context.Background(), ResolveRequest{
		RouteModelCode:            "edit",
		TaskType:                  "image_edit",
		Quality:                   "auto",
		RequestedOutputImageCount: 1,
		ReferenceImageCount:       1,
		MaskPresent:               true,
	})
	if err != nil {
		t.Fatalf("ResolveContext() error = %v", err)
	}
	if len(resolved.Providers) != 1 || resolved.Providers[0].ModelCode != "with-mask" {
		t.Fatalf("expected only the mask-capable candidate, got %#v", resolved.Providers)
	}
}

func TestResolveRouteModelMatchesSupportedRatioForNonCanonicalSize(t *testing.T) {
	resolver := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 1}})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: ModelRoutingSnapshot{
		RouteModels:    []RouteModelConfig{{ID: 1, Code: "wide", Name: "Wide", Visibility: "public", Enabled: true}},
		Prices:         []RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "2K", BasePoints: "1.00000", Enabled: true}},
		ProviderModels: []ProviderCandidate{{AccountModelID: 11, ModelCode: "wide-model", SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"2k"}, Quality: []string{"high"}, SupportedAspectRatios: []string{"16:9"}, MaxImageCount: 1}},
		Candidates:     []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 11, Enabled: true}},
	}})

	resolved, err := resolver.ResolveContext(context.Background(), ResolveRequest{
		RouteModelCode:            "wide",
		TaskType:                  "text_to_image",
		SizeMode:                  "ratio",
		AspectRatio:               "16:9",
		BaseResolution:            "2k",
		Quality:                   "high",
		RequestedOutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("ResolveContext() error = %v", err)
	}
	if len(resolved.Providers) != 1 || resolved.Providers[0].ModelCode != "wide-model" {
		t.Fatalf("expected the matching 16:9 candidate, got %#v", resolved.Providers)
	}
}

func TestResolveRouteModelMatchesRatioAndRejectsInvalidAspectRatio(t *testing.T) {
	resolver := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 1}})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: ModelRoutingSnapshot{
		RouteModels:    []RouteModelConfig{{ID: 1, Code: "square", Name: "Square", Visibility: "public", Enabled: true}},
		Prices:         []RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1K", BasePoints: "1.00000", Enabled: true}},
		ProviderModels: []ProviderCandidate{{AccountModelID: 11, ModelCode: "square-model", SupportedTaskTypes: []string{"text_to_image"}, Quality: []string{"auto"}, SupportedAspectRatios: []string{"1:1"}, MaxImageCount: 1}},
		Candidates:     []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 11, Enabled: true}},
	}})

	resolved, err := resolver.ResolveContext(context.Background(), ResolveRequest{
		RouteModelCode:            "square",
		TaskType:                  "text_to_image",
		SizeMode:                  "ratio",
		AspectRatio:               "1:1",
		BaseResolution:            "1k",
		Quality:                   "auto",
		RequestedOutputImageCount: 1,
	})
	if err != nil || len(resolved.Providers) != 1 {
		t.Fatalf("ratio-form requested size should match the square candidate, resolved=%#v err=%v", resolved, err)
	}

	_, err = resolver.ResolveContext(context.Background(), ResolveRequest{
		RouteModelCode:            "square",
		TaskType:                  "text_to_image",
		SizeMode:                  "ratio",
		AspectRatio:               "not-a-ratio",
		BaseResolution:            "1k",
		Quality:                   "auto",
		RequestedOutputImageCount: 1,
	})
	appErr, ok := err.(*errs.Error)
	if !ok || appErr.Code != CodeInvalidAspectRatio {
		t.Fatalf("invalid aspect ratio must not bypass configured ratios, got %#v", err)
	}
}

func TestVisibleRouteModelPreservesExplicitNoReferenceSupport(t *testing.T) {
	resolver := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 5, ReferenceImageMaxCount: 4}})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: ModelRoutingSnapshot{
		RouteModels:    []RouteModelConfig{{ID: 1, Code: "basic", Name: "Basic", Visibility: "public", Enabled: true}},
		Prices:         []RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1K", BasePoints: "1.00000", Enabled: true}},
		ProviderModels: []ProviderCandidate{{AccountModelID: 11, ModelCode: "text-only", SupportedAspectRatios: []string{"1:1"}, MaxImageCount: 1, MaxReferenceImageCount: 0}},
		Candidates:     []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 11, Enabled: true}},
	}})

	items, err := resolver.ListVisibleRouteModels(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("ListVisibleRouteModels() error = %v", err)
	}
	if len(items) != 1 || items[0].MaxReferenceImageCount != 0 {
		t.Fatalf("explicit no-reference support must not fall back to the platform limit: %#v", items)
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
			{AccountModelID: 12, ModelAccountID: 102, ModelCode: "gpt-image-1", SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"2k"}, SupportedAspectRatios: []string{"3:2"}},
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

func TestResolveRouteModelCustomPixelSizeRequiresDeclaredCapability(t *testing.T) {
	_, err := NormalizeResolveRequest(ResolveRequest{
		RouteModelCode: "plus", TaskType: "text_to_image", SizeMode: SizeModePixel,
		RequestedSize: "1001x1001", RequestedOutputImageCount: 1,
	})
	if err == nil {
		t.Fatal("explicit dimensions must be rejected instead of rounded")
	}
}

func TestVisibleRouteModelAggregatesCustomSizeCapability(t *testing.T) {
	resolver := NewResolver(config.Config{})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: ModelRoutingSnapshot{
		RouteModels: []RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices:      []RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true}},
		ProviderModels: []ProviderCandidate{{
			AccountModelID: 12, ModelCode: "custom-size", SizeModes: []string{SizeModePixel},
			SupportedPixelSizes: []string{"1024x1024"}, SupportsCustomSize: true,
			MinWidth: 512, MaxWidth: 1024, MinHeight: 512, MaxHeight: 1024,
		}},
		Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Enabled: true}},
	}})
	items, err := resolver.ListVisibleRouteModels(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("ListVisibleRouteModels: %v", err)
	}
	if len(items) != 1 || !items[0].SupportsCustomSize {
		t.Fatalf("expected visible custom-size capability, got %#v", items)
	}
}

func TestVisibleRouteModelScopesCapabilitiesByTaskType(t *testing.T) {
	resolver := NewResolver(config.Config{Billing: config.BillingConfig{
		AutoBaseResolutionDefaultByGroup: map[string]string{"plus": "2k"},
	}})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: ModelRoutingSnapshot{
		RouteModels: []RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices: []RoutePriceConfig{
			{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true},
			{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "2k", BasePoints: "2.00000", Enabled: true},
			{RouteModelID: 1, TaskType: "image_edit", BaseResolution: "4k", BasePoints: "4.00000", Enabled: true},
		},
		ProviderModels: []ProviderCandidate{
			{
				AccountModelID: 11, ModelCode: "text-model", SupportedTaskTypes: []string{"text_to_image"},
				SizeModes: []string{SizeModeRatio}, SupportedAspectRatios: []string{"1:1"}, Quality: []string{"high"},
				OutputFormat: []string{"jpeg"}, SupportsOutputCompression: true, Moderation: []string{"auto"},
				MaxImageCount: 2,
			},
			{
				AccountModelID: 12, ModelCode: "edit-model", SupportedTaskTypes: []string{"image_edit"},
				SizeModes: []string{SizeModePixel}, SupportedPixelSizes: []string{"1536x1024"}, SupportsCustomSize: true,
				Quality: []string{"low"}, OutputFormat: []string{"webp"}, Moderation: []string{"low"},
				MinWidth: 2064, MaxWidth: 4096, MinHeight: 688, MaxHeight: 4096,
				MaxImageCount: 1, MaxReferenceImageCount: 3,
			},
		},
		Candidates: []RouteCandidateConfig{
			{RouteModelID: 1, AccountModelID: 11, Enabled: true},
			{RouteModelID: 1, AccountModelID: 12, Enabled: true},
		},
	}})

	items, err := resolver.ListVisibleRouteModels(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("ListVisibleRouteModels: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("visible route model count = %d, want 1", len(items))
	}
	want := map[string]VisibleRouteModelTaskCapability{
		"text_to_image": {
			BaseResolution: []string{"1k", "2k"}, AutoBaseResolution: "2k",
			Quality: []string{"high"}, SizeModes: []string{"ratio"}, AspectRatios: []string{"1:1"}, PixelSizes: []string{},
			OutputFormat: []string{"jpeg"}, SupportsOutputCompression: true, SupportedBackgrounds: []string{}, Moderation: []string{"auto"},
			MaxOutputImageCount: 2,
		},
		"image_edit": {
			BaseResolution: []string{"4k"}, AutoBaseResolution: "4k",
			Quality: []string{"low"}, SizeModes: []string{"pixel"}, AspectRatios: []string{}, PixelSizes: []string{},
			OutputFormat: []string{"webp"}, SupportsCustomSize: true, SupportedBackgrounds: []string{}, Moderation: []string{"low"},
			MinWidth: 2064, MaxWidth: 4096, MinHeight: 688, MaxHeight: 4096,
			MaxOutputImageCount: 1, MaxReferenceImageCount: 3,
		},
	}
	if !reflect.DeepEqual(items[0].CapabilitiesByTaskType, want) {
		t.Fatalf("task capabilities = %#v, want %#v", items[0].CapabilitiesByTaskType, want)
	}
}

func TestVisibleRouteModelLimitsUsesSafeCandidateIntersection(t *testing.T) {
	resolver := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 8}})
	routing := ModelRoutingSnapshot{
		ProviderModels: []ProviderCandidate{
			{
				AccountModelID: 11, SupportedTaskTypes: []string{"text_to_image"},
				SupportedBaseResolution: []string{"1k", "2k"}, SizeModes: []string{"auto", "ratio", "pixel"},
				SupportedAspectRatios: []string{"1:1", "16:9"}, SupportedPixelSizes: []string{"1024x1024", "2048x1024"},
				Quality: []string{"auto", "high"}, OutputFormat: []string{"png", "webp"}, Moderation: []string{"auto", "low"},
				SupportedBackgrounds: []string{"auto", "transparent"}, SupportsOutputCompression: true,
				SupportsCustomRatio: true, SupportsCustomSize: true, MinWidth: 512, MaxWidth: 2048, MinHeight: 512, MaxHeight: 2048,
				MaxImageCount: 4,
			},
			{
				AccountModelID: 12, SupportedTaskTypes: []string{"text_to_image"},
				SupportedBaseResolution: []string{"1k", "4k"}, SizeModes: []string{"ratio", "pixel"},
				SupportedAspectRatios: []string{"1:1", "4:3"}, SupportedPixelSizes: []string{"1024x1024", "1536x1024"},
				Quality: []string{"auto", "low"}, OutputFormat: []string{"png", "jpeg"}, Moderation: []string{"auto"},
				SupportedBackgrounds: []string{"auto", "opaque"}, SupportsOutputCompression: false,
				SupportsCustomRatio: true, SupportsCustomSize: false, MinWidth: 768, MaxWidth: 1536, MinHeight: 640, MaxHeight: 1440,
				MaxImageCount: 2,
			},
		},
		Candidates: []RouteCandidateConfig{
			{RouteModelID: 1, AccountModelID: 11, Enabled: true},
			{RouteModelID: 1, AccountModelID: 12, Enabled: true},
		},
	}

	got := resolver.visibleRouteModelLimits(RouteModelConfig{ID: 1}, routing, "text_to_image")
	if !reflect.DeepEqual(got.BaseResolution, []string{"1k"}) || !reflect.DeepEqual(got.SizeModes, []string{"pixel", "ratio"}) || !reflect.DeepEqual(got.AspectRatios, []string{"1:1"}) || !reflect.DeepEqual(got.PixelSizes, []string{"1024x1024"}) {
		t.Fatalf("size capability intersection = %#v", got)
	}
	if !reflect.DeepEqual(got.Quality, []string{"auto"}) || !reflect.DeepEqual(got.OutputFormat, []string{"png"}) || !reflect.DeepEqual(got.SupportedBackgrounds, []string{"auto"}) || !reflect.DeepEqual(got.Moderation, []string{"auto"}) {
		t.Fatalf("enum capability intersection = %#v", got)
	}
	if got.SupportsOutputCompression || got.SupportsCustomSize || !got.SupportsCustomRatio {
		t.Fatalf("boolean capability intersection = %#v", got)
	}
	if got.MinWidth != 768 || got.MaxWidth != 1536 || got.MinHeight != 640 || got.MaxHeight != 1440 {
		t.Fatalf("pixel bounds intersection = %#v", got)
	}

	routing.ProviderModels[0].SizeModes = []string{"auto", "ratio"}
	routing.ProviderModels[1].SizeModes = []string{"pixel"}
	routing.ProviderModels[0].Moderation = []string{"auto"}
	routing.ProviderModels[1].Quality = []string{"low"}
	routing.ProviderModels[1].OutputFormat = []string{"jpeg"}
	routing.ProviderModels[1].Moderation = []string{"low"}
	got = resolver.visibleRouteModelLimits(RouteModelConfig{ID: 1}, routing, "text_to_image")
	if len(got.SizeModes) != 0 || len(got.Quality) != 0 || len(got.OutputFormat) != 0 || len(got.Moderation) != 0 {
		t.Fatalf("disjoint candidate capabilities must remain empty, got %#v", got)
	}
}

func TestRouteListAndResolveUseSameBoundedRatioCapability(t *testing.T) {
	routing := ModelRoutingSnapshot{
		RouteModels: []RouteModelConfig{{ID: 1, Code: "tight", Name: "Tight", Visibility: "public", Enabled: true}},
		Prices:      []RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true}},
		ProviderModels: []ProviderCandidate{{
			AccountModelID: 11, ModelCode: "gpt-image-2", SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"},
			SizeModes: []string{SizeModeRatio}, SupportedAspectRatios: []string{"1:1", "16:9"}, Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"}, MaxImageCount: 1,
			MinWidth: 512, MaxWidth: 900, MinHeight: 512, MaxHeight: 900,
		}},
		Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 11, Enabled: true}},
	}
	resolver := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 4}})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: routing})
	visible, err := resolver.ListVisibleRouteModels(t.Context(), nil, nil)
	if err != nil || len(visible) != 1 {
		t.Fatalf("ListVisibleRouteModels() = %#v, %v", visible, err)
	}
	capability := visible[0].CapabilitiesByTaskType["text_to_image"]
	if !reflect.DeepEqual(capability.AspectRatios, []string{"1:1"}) {
		t.Fatalf("visible bounded ratios = %v, want only 1:1", capability.AspectRatios)
	}
	resolved, err := resolver.ResolveContext(t.Context(), ResolveRequest{
		RouteModelCode: "tight", TaskType: "text_to_image", SizeMode: SizeModeRatio, BaseResolution: "1k", AspectRatio: "1:1",
		Quality: "auto", OutputFormat: "png", Moderation: "auto", RequestedOutputImageCount: 1,
	})
	if err != nil || resolved.ResolvedSize != "896x896" {
		t.Fatalf("bounded route resolve = %#v, %v; want resolved size 896x896", resolved, err)
	}
	_, err = resolver.ResolveContext(t.Context(), ResolveRequest{
		RouteModelCode: "tight", TaskType: "text_to_image", SizeMode: SizeModeRatio, BaseResolution: "1k", AspectRatio: "16:9",
		Quality: "auto", OutputFormat: "png", Moderation: "auto", RequestedOutputImageCount: 1,
	})
	var appErr *errs.Error
	if !errors.As(err, &appErr) || appErr.StatusCode != 400 || appErr.Code != CodeInvalidAspectRatio {
		t.Fatalf("filtered ratio resolve error = %#v, want 400/%s", err, CodeInvalidAspectRatio)
	}
}

func TestVisibleRouteModelFiltersRatioUnsolvableAfterBoundsIntersection(t *testing.T) {
	routing := ModelRoutingSnapshot{
		RouteModels: []RouteModelConfig{{ID: 1, Code: "intersection", Name: "Intersection", Visibility: "public", Enabled: true}},
		Prices:      []RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true}},
		ProviderModels: []ProviderCandidate{
			{AccountModelID: 11, SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"}, SizeModes: []string{SizeModeRatio}, SupportedAspectRatios: []string{"2:1"}, Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"}, MinWidth: 1100, MaxWidth: 2000, MinHeight: 576, MaxHeight: 600, MaxImageCount: 1},
			{AccountModelID: 12, SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"}, SizeModes: []string{SizeModeRatio}, SupportedAspectRatios: []string{"2:1"}, Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"}, MinWidth: 1280, MaxWidth: 1400, MinHeight: 576, MaxHeight: 1000, MaxImageCount: 1},
		},
		Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 11, Enabled: true}, {RouteModelID: 1, AccountModelID: 12, Enabled: true}},
	}
	resolver := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 1}})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: routing})
	visible, err := resolver.ListVisibleRouteModels(t.Context(), nil, nil)
	if err != nil || len(visible) != 1 {
		t.Fatalf("ListVisibleRouteModels() = %#v, %v", visible, err)
	}
	capability := visible[0].CapabilitiesByTaskType["text_to_image"]
	if containsString(capability.SizeModes, SizeModeRatio) || len(capability.AspectRatios) != 0 || len(capability.BaseResolution) != 0 {
		t.Fatalf("aggregate unsatisfiable ratio capability was advertised: %#v", capability)
	}
}

func TestNormalizeProviderCandidateDoesNotRestoreFilteredLegacyRatioDefaults(t *testing.T) {
	got := normalizeProviderCandidate(ProviderCandidate{
		SupportedTaskTypes: []string{"text_to_image"}, MinWidth: 512, MaxWidth: 700, MinHeight: 512, MaxHeight: 700,
	})
	if containsString(got.SizeModes, SizeModeRatio) || len(got.SupportedAspectRatios) != 0 || len(got.SupportedBaseResolution) != 0 {
		t.Fatalf("filtered legacy ratio defaults were restored: %#v", got)
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

func TestVisibleCapabilityAndRouteResolutionUseSameSafeIntersection(t *testing.T) {
	baseCandidate := func(id int64) ProviderCandidate {
		return ProviderCandidate{
			AccountModelID: id, ModelCode: "gpt-image-2", SupportedTaskTypes: []string{"text_to_image"},
			SupportedBaseResolution: []string{"1k", "2k"}, Quality: []string{"auto"}, OutputFormat: []string{"png", "jpeg", "webp"},
			Moderation: []string{"auto"}, MinWidth: 512, MaxWidth: 2048, MinHeight: 512, MaxHeight: 2048,
			MaxImageCount: 1, HealthStatus: "enabled",
		}
	}
	tests := []struct {
		name          string
		configure     func(*ProviderCandidate, *ProviderCandidate)
		request       ResolveRequest
		wantCode      string
		assertVisible func(t *testing.T, capability VisibleRouteModelTaskCapability)
	}{
		{
			name: "disjoint size modes",
			configure: func(first, second *ProviderCandidate) {
				first.SizeModes, first.SupportedAspectRatios = []string{SizeModeRatio}, []string{"1:1"}
				second.SizeModes, second.SupportedPixelSizes = []string{SizeModePixel}, []string{"1024x1024"}
			},
			request:  ResolveRequest{SizeMode: SizeModeRatio, BaseResolution: "1k", AspectRatio: "1:1"},
			wantCode: CodeInvalidSizeMode,
			assertVisible: func(t *testing.T, capability VisibleRouteModelTaskCapability) {
				if len(capability.SizeModes) != 0 {
					t.Fatalf("visible size_modes = %#v, want empty intersection", capability.SizeModes)
				}
			},
		},
		{
			name: "ratio unavailable in one candidate",
			configure: func(first, second *ProviderCandidate) {
				first.SizeModes, first.SupportedAspectRatios = []string{SizeModeRatio}, []string{"1:1", "16:9"}
				second.SizeModes, second.SupportedAspectRatios = []string{SizeModeRatio}, []string{"1:1"}
			},
			request:  ResolveRequest{SizeMode: SizeModeRatio, BaseResolution: "1k", AspectRatio: "16:9"},
			wantCode: CodeInvalidAspectRatio,
			assertVisible: func(t *testing.T, capability VisibleRouteModelTaskCapability) {
				if !reflect.DeepEqual(capability.AspectRatios, []string{"1:1"}) {
					t.Fatalf("visible aspect_ratios = %#v, want [1:1]", capability.AspectRatios)
				}
			},
		},
		{
			name: "custom ratio disabled in one candidate",
			configure: func(first, second *ProviderCandidate) {
				first.SizeModes, first.SupportedAspectRatios, first.SupportsCustomRatio = []string{SizeModeRatio}, []string{"1:1"}, true
				second.SizeModes, second.SupportedAspectRatios = []string{SizeModeRatio}, []string{"1:1"}
			},
			request:  ResolveRequest{SizeMode: SizeModeRatio, BaseResolution: "1k", AspectRatio: "7:5"},
			wantCode: CodeInvalidAspectRatio,
			assertVisible: func(t *testing.T, capability VisibleRouteModelTaskCapability) {
				if capability.SupportsCustomRatio {
					t.Fatal("visible capability must disable custom ratio")
				}
			},
		},
		{
			name: "pixel preset unavailable in one candidate",
			configure: func(first, second *ProviderCandidate) {
				first.SizeModes, first.SupportedPixelSizes = []string{SizeModePixel}, []string{"1024x1024", "1280x720"}
				second.SizeModes, second.SupportedPixelSizes = []string{SizeModePixel}, []string{"1024x1024"}
			},
			request:  ResolveRequest{SizeMode: SizeModePixel, RequestedSize: "1280x720"},
			wantCode: CodeInvalidExplicitDimensions,
			assertVisible: func(t *testing.T, capability VisibleRouteModelTaskCapability) {
				if !reflect.DeepEqual(capability.PixelSizes, []string{"1024x1024"}) {
					t.Fatalf("visible pixel_sizes = %#v, want [1024x1024]", capability.PixelSizes)
				}
			},
		},
		{
			name: "custom pixels disabled in one candidate",
			configure: func(first, second *ProviderCandidate) {
				first.SizeModes, first.SupportedPixelSizes, first.SupportsCustomSize = []string{SizeModePixel}, []string{"1024x1024"}, true
				second.SizeModes, second.SupportedPixelSizes = []string{SizeModePixel}, []string{"1024x1024"}
			},
			request:  ResolveRequest{SizeMode: SizeModePixel, RequestedSize: "1280x720"},
			wantCode: CodeInvalidExplicitDimensions,
			assertVisible: func(t *testing.T, capability VisibleRouteModelTaskCapability) {
				if capability.SupportsCustomSize {
					t.Fatal("visible capability must disable custom pixels")
				}
			},
		},
		{
			name: "background unavailable in one candidate",
			configure: func(first, second *ProviderCandidate) {
				first.SizeModes, first.SupportedAspectRatios, first.SupportedBackgrounds = []string{SizeModeRatio}, []string{"1:1"}, []string{"auto", "transparent"}
				second.SizeModes, second.SupportedAspectRatios, second.SupportedBackgrounds = []string{SizeModeRatio}, []string{"1:1"}, []string{"auto"}
			},
			request:  ResolveRequest{SizeMode: SizeModeRatio, BaseResolution: "1k", AspectRatio: "1:1", Background: "transparent"},
			wantCode: errs.CodeImageCapabilityMismatch,
			assertVisible: func(t *testing.T, capability VisibleRouteModelTaskCapability) {
				if !reflect.DeepEqual(capability.SupportedBackgrounds, []string{"auto"}) {
					t.Fatalf("visible backgrounds = %#v, want [auto]", capability.SupportedBackgrounds)
				}
			},
		},
		{
			name: "output format unavailable in one candidate",
			configure: func(first, second *ProviderCandidate) {
				first.SizeModes, first.SupportedAspectRatios, first.OutputFormat = []string{SizeModeRatio}, []string{"1:1"}, []string{"png"}
				second.SizeModes, second.SupportedAspectRatios, second.OutputFormat = []string{SizeModeRatio}, []string{"1:1"}, []string{"jpeg"}
			},
			request:  ResolveRequest{SizeMode: SizeModeRatio, BaseResolution: "1k", AspectRatio: "1:1"},
			wantCode: errs.CodeImageCapabilityMismatch,
			assertVisible: func(t *testing.T, capability VisibleRouteModelTaskCapability) {
				if len(capability.OutputFormat) != 0 {
					t.Fatalf("visible output_format = %#v, want empty intersection", capability.OutputFormat)
				}
			},
		},
		{
			name: "common ratio accepted by every candidate",
			configure: func(first, second *ProviderCandidate) {
				first.SizeModes, first.SupportedAspectRatios = []string{SizeModeRatio}, []string{"1:1", "16:9"}
				second.SizeModes, second.SupportedAspectRatios = []string{SizeModeRatio}, []string{"1:1"}
			},
			request: ResolveRequest{SizeMode: SizeModeRatio, BaseResolution: "1k", AspectRatio: "1:1"},
			assertVisible: func(t *testing.T, capability VisibleRouteModelTaskCapability) {
				if !reflect.DeepEqual(capability.AspectRatios, []string{"1:1"}) {
					t.Fatalf("visible aspect_ratios = %#v, want [1:1]", capability.AspectRatios)
				}
			},
		},
		{
			name: "legacy common ratio accepted by every candidate",
			configure: func(first, second *ProviderCandidate) {
				first.SizeModes, first.SupportedAspectRatios = []string{SizeModeRatio}, []string{"1:1", "16:9"}
				second.SizeModes, second.SupportedAspectRatios = []string{SizeModeRatio}, []string{"1:1"}
			},
			request: ResolveRequest{BaseResolution: "1k", AspectRatio: "1:1"},
			assertVisible: func(t *testing.T, capability VisibleRouteModelTaskCapability) {
				if !reflect.DeepEqual(capability.AspectRatios, []string{"1:1"}) {
					t.Fatalf("visible aspect_ratios = %#v, want [1:1]", capability.AspectRatios)
				}
			},
		},
		{
			name: "legacy ratio outside intersection rejected",
			configure: func(first, second *ProviderCandidate) {
				first.SizeModes, first.SupportedAspectRatios = []string{SizeModeRatio}, []string{"1:1", "16:9"}
				second.SizeModes, second.SupportedAspectRatios = []string{SizeModeRatio}, []string{"1:1"}
			},
			request:  ResolveRequest{BaseResolution: "1k", AspectRatio: "16:9"},
			wantCode: CodeInvalidAspectRatio,
			assertVisible: func(t *testing.T, capability VisibleRouteModelTaskCapability) {
				if !reflect.DeepEqual(capability.AspectRatios, []string{"1:1"}) {
					t.Fatalf("visible aspect_ratios = %#v, want [1:1]", capability.AspectRatios)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first, second := baseCandidate(11), baseCandidate(12)
			tt.configure(&first, &second)
			routing := ModelRoutingSnapshot{
				RouteModels: []RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
				Prices: []RoutePriceConfig{
					{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true},
					{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "2k", BasePoints: "2.00000", Enabled: true},
				},
				ProviderModels: []ProviderCandidate{first, second},
				Candidates: []RouteCandidateConfig{
					{RouteModelID: 1, AccountModelID: 11, Priority: 1, Enabled: true},
					{RouteModelID: 1, AccountModelID: 12, Priority: 2, Enabled: true},
				},
			}
			resolver := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 20, ReferenceImageMaxCount: 2}})
			resolver.SetModelRoutingSource(staticRoutingSource{snapshot: routing})
			visible, err := resolver.ListVisibleRouteModels(context.Background(), nil, nil)
			if err != nil || len(visible) != 1 {
				t.Fatalf("ListVisibleRouteModels() = %#v, %v", visible, err)
			}
			capability, ok := visible[0].CapabilitiesByTaskType["text_to_image"]
			if !ok {
				t.Fatalf("visible task capability missing: %#v", visible[0])
			}
			tt.assertVisible(t, capability)

			tt.request.RouteModelCode = "plus"
			tt.request.TaskType = "text_to_image"
			tt.request.Quality = "auto"
			tt.request.OutputFormat = "png"
			tt.request.Moderation = "auto"
			tt.request.RequestedOutputImageCount = 1
			resolved, err := resolver.ResolveContext(context.Background(), tt.request)
			if tt.wantCode == "" {
				if err != nil {
					t.Fatalf("ResolveContext() error = %v", err)
				}
				if len(resolved.Providers) != 2 {
					t.Fatalf("resolved providers = %#v, want both candidates", resolved.Providers)
				}
				return
			}
			var appErr *errs.Error
			if !errors.As(err, &appErr) || appErr.StatusCode != 400 || appErr.Code != tt.wantCode {
				t.Fatalf("ResolveContext() error = %#v, want 400/%s", err, tt.wantCode)
			}
		})
	}
}

func TestVisibleCapabilityFiltersPixelSizesWithoutPriceAndRejectsCustomBucket(t *testing.T) {
	routing := ModelRoutingSnapshot{
		RouteModels: []RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices:      []RoutePriceConfig{{ID: 1, RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true}},
		ProviderModels: []ProviderCandidate{{
			AccountModelID: 11, SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k", "2k"},
			SizeModes: []string{SizeModePixel}, SupportedPixelSizes: []string{"1024x1024", "2048x1024"}, SupportsCustomSize: true,
			Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"}, MaxImageCount: 1,
			MinWidth: 512, MaxWidth: 2048, MinHeight: 512, MaxHeight: 2048, HealthStatus: "enabled",
		}},
		Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 11, Enabled: true}},
	}
	resolver := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 20, ReferenceImageMaxCount: 2}})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: routing})
	visible, err := resolver.ListVisibleRouteModels(context.Background(), nil, nil)
	if err != nil || len(visible) != 1 {
		t.Fatalf("ListVisibleRouteModels() = %#v, %v", visible, err)
	}
	capability := visible[0].CapabilitiesByTaskType["text_to_image"]
	if capability.SupportsCustomSize {
		t.Fatal("custom pixels must not be advertised when configured bounds reach an unpriced 2k bucket")
	}
	if visible[0].SupportsCustomSize {
		t.Fatal("aggregate custom pixel capability must not overstate partial task price coverage")
	}
	if !reflect.DeepEqual(capability.PixelSizes, []string{"1024x1024"}) {
		t.Fatalf("pixel_sizes = %#v, want only priced 1k preset", capability.PixelSizes)
	}
	if !reflect.DeepEqual(visible[0].PixelSizes, []string{"1024x1024"}) {
		t.Fatalf("aggregate pixel_sizes = %#v, want only priced preset", visible[0].PixelSizes)
	}
	for _, size := range []string{"2048x1024", "2048x992"} {
		_, err := resolver.ResolveContext(context.Background(), ResolveRequest{
			RouteModelCode: "plus", TaskType: "text_to_image", SizeMode: SizeModePixel, RequestedSize: size,
			Quality: "auto", OutputFormat: "png", Moderation: "auto", RequestedOutputImageCount: 1,
		})
		var appErr *errs.Error
		if !errors.As(err, &appErr) || appErr.StatusCode != 400 || appErr.Code != CodeInvalidExplicitDimensions {
			t.Fatalf("ResolveContext(%s) error = %#v, want 400/%s", size, err, CodeInvalidExplicitDimensions)
		}
	}
}

func TestVisibleCapabilityKeepsCustomPixelsWhenEveryReachableBucketIsPriced(t *testing.T) {
	routing := ModelRoutingSnapshot{
		RouteModels: []RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices: []RoutePriceConfig{
			{ID: 1, RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true},
			{ID: 2, RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "2k", BasePoints: "2.00000", Enabled: true},
		},
		ProviderModels: []ProviderCandidate{{
			AccountModelID: 11, SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k", "2k"},
			SizeModes: []string{SizeModePixel}, SupportedPixelSizes: []string{"1024x1024", "2048x1024"}, SupportsCustomSize: true,
			Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"}, MaxImageCount: 1,
			MinWidth: 512, MaxWidth: 2048, MinHeight: 512, MaxHeight: 2048, HealthStatus: "enabled",
		}},
		Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 11, Enabled: true}},
	}
	resolver := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 20, ReferenceImageMaxCount: 2}})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: routing})
	visible, err := resolver.ListVisibleRouteModels(context.Background(), nil, nil)
	if err != nil || len(visible) != 1 {
		t.Fatalf("ListVisibleRouteModels() = %#v, %v", visible, err)
	}
	capability := visible[0].CapabilitiesByTaskType["text_to_image"]
	if !capability.SupportsCustomSize {
		t.Fatal("custom pixels should remain available when every reachable price bucket is enabled")
	}
	if !visible[0].SupportsCustomSize {
		t.Fatal("aggregate custom pixels should remain available when every reachable bucket is priced")
	}
	if _, err := resolver.ResolveContext(context.Background(), ResolveRequest{
		RouteModelCode: "plus", TaskType: "text_to_image", SizeMode: SizeModePixel, RequestedSize: "2048x992",
		Quality: "auto", OutputFormat: "png", Moderation: "auto", RequestedOutputImageCount: 1,
	}); err != nil {
		t.Fatalf("fully priced custom pixel rejected: %v", err)
	}
}

func TestVisibleCapabilityDoesNotMergeCustomPixelPricesAcrossTaskTypes(t *testing.T) {
	routing := ModelRoutingSnapshot{
		RouteModels: []RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices: []RoutePriceConfig{
			{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true},
			{RouteModelID: 1, TaskType: "image_edit", BaseResolution: "2k", BasePoints: "2.00000", Enabled: true},
		},
		ProviderModels: []ProviderCandidate{{
			AccountModelID: 11, SupportedTaskTypes: []string{"text_to_image", "image_edit"}, SupportedBaseResolution: []string{"1k", "2k"},
			SizeModes: []string{SizeModePixel}, SupportedPixelSizes: []string{"1024x1024", "2048x1024"}, SupportsCustomSize: true,
			Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"}, MaxImageCount: 1,
			MinWidth: 512, MaxWidth: 2048, MinHeight: 512, MaxHeight: 2048,
		}},
		Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 11, Enabled: true}},
	}
	resolver := NewResolver(config.Config{})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: routing})
	visible, err := resolver.ListVisibleRouteModels(context.Background(), nil, nil)
	if err != nil || len(visible) != 1 {
		t.Fatalf("ListVisibleRouteModels() = %#v, %v", visible, err)
	}
	if visible[0].SupportsCustomSize {
		t.Fatal("aggregate capability must not merge disjoint task price buckets into custom pixel support")
	}
	if len(visible[0].PixelSizes) != 0 {
		t.Fatalf("aggregate capability must not merge disjoint task-priced presets, got %v", visible[0].PixelSizes)
	}
	for taskType, capability := range visible[0].CapabilitiesByTaskType {
		if capability.SupportsCustomSize {
			t.Fatalf("%s custom pixels must be disabled by partial task price coverage", taskType)
		}
	}
}

func TestPixelBucketReachabilityUsesConfiguredIntervalsAndHardRatio(t *testing.T) {
	tests := []struct {
		name                   string
		minWidth, maxWidth     int
		minHeight, maxHeight   int
		want1K, want2K, want4K bool
	}{
		{name: "one k only", minWidth: 512, maxWidth: 1024, minHeight: 512, maxHeight: 1024, want1K: true},
		{name: "one and two k boundary", minWidth: 1024, maxWidth: 2048, minHeight: 1024, maxHeight: 2048, want1K: true, want2K: true},
		{name: "four k only", minWidth: 2064, maxWidth: 4096, minHeight: 688, maxHeight: 4096, want4K: true},
		{name: "ratio makes rectangle unreachable", minWidth: 2048, maxWidth: 2048, minHeight: 512, maxHeight: 512},
		{name: "minimum area excludes one k", minWidth: 512, maxWidth: 2048, minHeight: 512, maxHeight: 512, want2K: true},
		{name: "no legal grid point", minWidth: 1025, maxWidth: 1039, minHeight: 1025, maxHeight: 1039},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			minWidth, maxWidth := effectivePixelBounds(tt.minWidth, tt.maxWidth)
			minHeight, maxHeight := effectivePixelBounds(tt.minHeight, tt.maxHeight)
			got1K := pixelBucketReachable(minWidth, maxWidth, minHeight, maxHeight, 16, 1024)
			got2K := pixelBucketReachable(minWidth, maxWidth, minHeight, maxHeight, 1040, 2048)
			got4K := pixelBucketReachable(minWidth, maxWidth, minHeight, maxHeight, 2064, 4096)
			if got1K != tt.want1K || got2K != tt.want2K || got4K != tt.want4K {
				t.Fatalf("reachable buckets = (%t,%t,%t), want (%t,%t,%t)", got1K, got2K, got4K, tt.want1K, tt.want2K, tt.want4K)
			}
			capability := VisibleRouteModelTaskCapability{SupportsCustomSize: true, MinWidth: tt.minWidth, MaxWidth: tt.maxWidth, MinHeight: tt.minHeight, MaxHeight: tt.maxHeight}
			gotAny := everyReachablePixelBucketPriced(capability, map[string]struct{}{"1k": {}, "2k": {}, "4k": {}})
			if wantAny := tt.want1K || tt.want2K || tt.want4K; gotAny != wantAny {
				t.Fatalf("all-priced custom support = %t, want reachable=%t", gotAny, wantAny)
			}
		})
	}
}

func BenchmarkEveryReachablePixelBucketPriced(b *testing.B) {
	capability := VisibleRouteModelTaskCapability{SupportsCustomSize: true, MinWidth: 16, MaxWidth: 4096, MinHeight: 16, MaxHeight: 4096}
	priced := map[string]struct{}{"1k": {}, "2k": {}, "4k": {}}
	for b.Loop() {
		if !everyReachablePixelBucketPriced(capability, priced) {
			b.Fatal("fully priced hard range rejected")
		}
	}
}

func TestVisibleCapabilityDoesNotInventDefaultsWithoutActiveCandidate(t *testing.T) {
	resolver := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 20, ReferenceImageMaxCount: 2}})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: ModelRoutingSnapshot{
		RouteModels: []RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices:      []RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true}},
		ProviderModels: []ProviderCandidate{{
			AccountModelID: 11, HealthStatus: "unhealthy", SupportedTaskTypes: []string{"text_to_image"}, MaxImageCount: 1,
		}},
		Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 11, Enabled: true}},
	}})
	visible, err := resolver.ListVisibleRouteModels(context.Background(), nil, nil)
	if err != nil || len(visible) != 1 {
		t.Fatalf("ListVisibleRouteModels() = %#v, %v", visible, err)
	}
	capability := visible[0].CapabilitiesByTaskType["text_to_image"]
	if len(capability.SizeModes) != 0 || len(capability.Quality) != 0 || len(capability.OutputFormat) != 0 || len(capability.Moderation) != 0 || capability.MaxOutputImageCount != 0 {
		t.Fatalf("inactive route must expose empty capability, got %#v", capability)
	}
}

func TestResolveRouteCapabilityVersionRejectsStaleProjection(t *testing.T) {
	routing := ModelRoutingSnapshot{
		RouteModels: []RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices: []RoutePriceConfig{
			{ID: 1, RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true},
			{ID: 2, RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "2k", BasePoints: "2.00000", Enabled: true},
		},
		ProviderModels: []ProviderCandidate{{
			AccountModelID: 11, SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k", "2k"},
			SizeModes: []string{SizeModeRatio}, SupportedAspectRatios: []string{"1:1"}, Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"}, MaxImageCount: 1,
		}},
		Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 11, Enabled: true}},
	}
	resolver := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 20, ReferenceImageMaxCount: 2}})
	resolver.SetModelRoutingSource(staticRoutingSource{snapshot: routing})
	req := ResolveRequest{RouteModelCode: "plus", TaskType: "text_to_image", SizeMode: SizeModeRatio, BaseResolution: "1k", AspectRatio: "1:1", Quality: "auto", OutputFormat: "png", Moderation: "auto", RequestedOutputImageCount: 1}
	resolved, err := resolver.ResolveContext(context.Background(), req)
	if err != nil || resolved.CapabilityVersion == "" {
		t.Fatalf("initial resolve = %#v, %v; want capability version", resolved, err)
	}
	req.ExpectedCapabilityVersion = resolved.CapabilityVersion
	if _, err := resolver.ResolveContext(context.Background(), req); err != nil {
		t.Fatalf("matching capability version rejected: %v", err)
	}
	equivalentRouting := routing
	equivalentRouting.ProviderModels = append([]ProviderCandidate(nil), routing.ProviderModels...)
	equivalentRouting.ProviderModels[0].Quality = []string{"auto", "auto"}
	equivalentRouting.ProviderModels[0].SupportedAspectRatios = []string{"1:1", "1:1"}
	equivalent := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 20, ReferenceImageMaxCount: 2}})
	equivalent.SetModelRoutingSource(staticRoutingSource{snapshot: equivalentRouting})
	equivalentResolved, err := equivalent.ResolveContext(context.Background(), ResolveRequest{RouteModelCode: "plus", TaskType: "text_to_image", SizeMode: SizeModeRatio, BaseResolution: "1k", AspectRatio: "1:1", Quality: "auto", OutputFormat: "png", Moderation: "auto", RequestedOutputImageCount: 1})
	if err != nil || equivalentResolved.CapabilityVersion != resolved.CapabilityVersion {
		t.Fatalf("non-semantic ordering/duplicate change altered version: before=%q after=%q err=%v", resolved.CapabilityVersion, equivalentResolved.CapabilityVersion, err)
	}
	unusedBillingRouting := routing
	unusedBillingRouting.Prices = append([]RoutePriceConfig(nil), routing.Prices...)
	unusedBillingRouting.Prices[0].ReferenceMultiplier = "9.00000"
	unusedBilling := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 20, ReferenceImageMaxCount: 2}})
	unusedBilling.SetModelRoutingSource(staticRoutingSource{snapshot: unusedBillingRouting})
	unusedBillingResolved, err := unusedBilling.ResolveContext(context.Background(), ResolveRequest{RouteModelCode: "plus", TaskType: "text_to_image", SizeMode: SizeModeRatio, BaseResolution: "1k", AspectRatio: "1:1", Quality: "auto", OutputFormat: "png", Moderation: "auto", RequestedOutputImageCount: 1})
	if err != nil || unusedBillingResolved.CapabilityVersion != resolved.CapabilityVersion {
		t.Fatalf("unused reference multiplier altered version: before=%q after=%q err=%v", resolved.CapabilityVersion, unusedBillingResolved.CapabilityVersion, err)
	}
	maskRouting := routing
	maskRouting.ProviderModels = append([]ProviderCandidate(nil), routing.ProviderModels...)
	maskRouting.ProviderModels[0].SupportsMask = true
	maskResolver := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 20, ReferenceImageMaxCount: 2}})
	maskResolver.SetModelRoutingSource(staticRoutingSource{snapshot: maskRouting})
	maskResolved, err := maskResolver.ResolveContext(context.Background(), ResolveRequest{RouteModelCode: "plus", TaskType: "text_to_image", SizeMode: SizeModeRatio, BaseResolution: "1k", AspectRatio: "1:1", Quality: "auto", OutputFormat: "png", Moderation: "auto", RequestedOutputImageCount: 1})
	if err != nil || maskResolved.CapabilityVersion == resolved.CapabilityVersion {
		t.Fatalf("mask support change must alter version: before=%q after=%q err=%v", resolved.CapabilityVersion, maskResolved.CapabilityVersion, err)
	}
	taskMultiplier := NewResolver(config.Config{
		Billing:          config.BillingConfig{TaskMultipliers: map[string]string{"text_to_image": "2.00000"}},
		GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 20, ReferenceImageMaxCount: 2},
	})
	taskMultiplier.SetModelRoutingSource(staticRoutingSource{snapshot: routing})
	taskMultiplierResolved, err := taskMultiplier.ResolveContext(context.Background(), ResolveRequest{RouteModelCode: "plus", TaskType: "text_to_image", SizeMode: SizeModeRatio, BaseResolution: "1k", AspectRatio: "1:1", Quality: "auto", OutputFormat: "png", Moderation: "auto", RequestedOutputImageCount: 1})
	if err != nil || taskMultiplierResolved.CapabilityVersion == resolved.CapabilityVersion {
		t.Fatalf("effective task multiplier change must alter version: before=%q after=%q err=%v", resolved.CapabilityVersion, taskMultiplierResolved.CapabilityVersion, err)
	}
	autoChanged := NewResolver(config.Config{
		Billing:          config.BillingConfig{AutoBaseResolutionDefaultByGroup: map[string]string{"plus": "2k"}},
		GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 20, ReferenceImageMaxCount: 2},
	})
	autoChanged.SetModelRoutingSource(staticRoutingSource{snapshot: routing})
	autoChangedResolved, err := autoChanged.ResolveContext(context.Background(), ResolveRequest{RouteModelCode: "plus", TaskType: "text_to_image", SizeMode: SizeModeRatio, BaseResolution: "1k", AspectRatio: "1:1", Quality: "auto", OutputFormat: "png", Moderation: "auto", RequestedOutputImageCount: 1})
	if err != nil || autoChangedResolved.CapabilityVersion == resolved.CapabilityVersion {
		t.Fatalf("effective auto-resolution change must alter version: before=%q after=%q err=%v", resolved.CapabilityVersion, autoChangedResolved.CapabilityVersion, err)
	}
	req.ExpectedCapabilityVersion = "stale-version"
	req.AspectRatio = "4:1"
	_, err = resolver.ResolveContext(context.Background(), req)
	var appErr *errs.Error
	if !errors.As(err, &appErr) || appErr.StatusCode != 409 || appErr.Code != CodeCapabilityChanged {
		t.Fatalf("stale capability error = %#v, want 409/%s", err, CodeCapabilityChanged)
	}
	req.AspectRatio = "1:1"

	routing.ProviderModels[0].SupportedAspectRatios = []string{"1:1", "16:9"}
	changed := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 20, ReferenceImageMaxCount: 2}})
	changed.SetModelRoutingSource(staticRoutingSource{snapshot: routing})
	changedResolved, err := changed.ResolveContext(context.Background(), ResolveRequest{RouteModelCode: "plus", TaskType: "text_to_image", SizeMode: SizeModeRatio, BaseResolution: "1k", AspectRatio: "1:1", Quality: "auto", OutputFormat: "png", Moderation: "auto", RequestedOutputImageCount: 1})
	if err != nil || changedResolved.CapabilityVersion == resolved.CapabilityVersion {
		t.Fatalf("capability edit must change version: before=%q after=%q err=%v", resolved.CapabilityVersion, changedResolved.CapabilityVersion, err)
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
