package modelhub

import (
	"bytes"
	"context"
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
			{AccountModelID: 13, ModelCode: "wrong-ratio", SupportedTaskTypes: []string{"image_edit"}, SupportedBaseResolution: []string{"1k"}, Quality: []string{"auto"}, SupportedAspectRatios: []string{"1:1"}, MaxImageCount: 2, MaxReferenceImageCount: 2, SupportsImageInput: true},
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
			Quality: []string{"low"}, SizeModes: []string{"pixel"}, AspectRatios: []string{}, PixelSizes: []string{"1536x1024"},
			OutputFormat: []string{"webp"}, SupportsCustomSize: true, SupportedBackgrounds: []string{}, Moderation: []string{"low"},
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
