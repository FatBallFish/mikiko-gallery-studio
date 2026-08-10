package capabilities

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
	assetservice "github.com/fatballfish/pic-gallery/internal/service/assets"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
)

type staticModelRoutingSource struct {
	snapshot modelhub.ModelRoutingSnapshot
}

func (s staticModelRoutingSource) ModelRoutingConfig(context.Context) (modelhub.ModelRoutingSnapshot, error) {
	return s.snapshot, nil
}

type flakyAttachmentPolicySource struct {
	tab domainadminconfig.Tab
	err error
}

func (s *flakyAttachmentPolicySource) GetTab(context.Context, string) (domainadminconfig.Tab, error) {
	if s.err != nil {
		return domainadminconfig.Tab{}, s.err
	}
	return s.tab, nil
}

func TestVisibleRouteModelJSONUsesSnakeCaseCapabilityFields(t *testing.T) {
	payload, err := json.Marshal(modelhub.VisibleRouteModel{
		Code:                      "plus",
		OutputFormat:              []string{"webp"},
		SupportsOutputCompression: true,
		SupportsCustomSize:        true,
		CapabilitiesByTaskType: map[string]modelhub.VisibleRouteModelTaskCapability{
			"text_to_image": {AutoBaseResolution: "2k", Quality: []string{"high"}},
		},
		Prices: []modelhub.VisibleRouteModelPrice{{TaskType: "text_to_image", BaseResolution: "1k"}},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	encoded := string(payload)
	for _, expected := range []string{`"output_format"`, `"supports_output_compression"`, `"supports_custom_size"`, `"capabilities_by_task_type"`, `"auto_base_resolution"`, `"task_type"`, `"base_resolution"`} {
		if !strings.Contains(encoded, expected) {
			t.Fatalf("expected snake-case field %s in %s", expected, encoded)
		}
	}
	if strings.Contains(encoded, `"SupportsOutputCompression"`) || strings.Contains(encoded, `"CapabilitiesByTaskType"`) || strings.Contains(encoded, `"OutputFormat"`) {
		t.Fatalf("capability response must not expose Go field names: %s", encoded)
	}
}

func TestCapabilitiesExposeDynamicAttachmentPolicy(t *testing.T) {
	cfg := config.Config{AttachmentPolicy: config.AttachmentPolicyConfig{
		ImageMaxMB: 20, ImageAllowedFormats: []string{"png", "jpeg", "webp", "gif"},
	}}
	admin := adminconfigservice.NewServiceWithStore(cfg, adminconfigservice.NewMemoryStore())
	policy := assetservice.NewAttachmentPolicyResolver(cfg.AttachmentPolicy, admin)
	svc := NewServiceWithAttachmentPolicy(cfg, policy)

	first := svc.List()
	if first.ReferenceImageMaxBytes != 20*1024*1024 || !slices.Equal(first.ReferenceImageAllowedFormats, []string{"png", "jpeg", "webp", "gif"}) {
		t.Fatalf("unexpected initial capability policy: %#v", first)
	}

	if _, err := admin.UpdateTab(context.Background(), domainadminconfig.UpdateTabRequest{
		TabKey: assetservice.AttachmentPolicyTabKey, Version: 1,
		Items: []domainadminconfig.Item{
			{ConfigCategory: assetservice.AttachmentPolicyTabKey, ConfigKey: assetservice.AttachmentImageMaxMBKey, ConfigValue: map[string]any{"value": 24}, Scope: "global"},
			{ConfigCategory: assetservice.AttachmentPolicyTabKey, ConfigKey: assetservice.AttachmentImageAllowedFormatsKey, ConfigValue: map[string]any{"value": []any{"webp"}}, Scope: "global"},
		},
	}); err != nil {
		t.Fatalf("UpdateTab: %v", err)
	}

	second := svc.List()
	if second.ReferenceImageMaxBytes != 24*1024*1024 || !slices.Equal(second.ReferenceImageAllowedFormats, []string{"webp"}) || !slices.Equal(second.ReferenceImageAllowedMIMETypes, []string{"image/webp"}) {
		t.Fatalf("capabilities kept stale attachment policy: %#v", second)
	}
}

func TestCapabilitiesKeepLastKnownGoodAttachmentPolicyWhenRefreshFails(t *testing.T) {
	cfg := config.Config{AttachmentPolicy: config.AttachmentPolicyConfig{
		ImageMaxMB: 20, ImageAllowedFormats: []string{"png", "jpeg", "webp", "gif"},
	}}
	source := &flakyAttachmentPolicySource{tab: domainadminconfig.Tab{
		TabKey:  assetservice.AttachmentPolicyTabKey,
		Version: 2,
		Items: []domainadminconfig.Item{
			{ConfigKey: assetservice.AttachmentImageMaxMBKey, ConfigValue: map[string]any{"value": 48}},
			{ConfigKey: assetservice.AttachmentImageAllowedFormatsKey, ConfigValue: map[string]any{"value": []string{"webp"}}},
		},
	}}
	policy := assetservice.NewAttachmentPolicyResolver(cfg.AttachmentPolicy, source)
	svc := NewServiceWithAttachmentPolicy(cfg, policy)

	first := svc.List()
	if first.ReferenceImageMaxMB != 48 || !slices.Equal(first.ReferenceImageAllowedFormats, []string{"webp"}) {
		t.Fatalf("unexpected initial policy: %#v", first)
	}
	source.err = errors.New("database unavailable")
	policy.Invalidate()

	second := svc.List()
	if second.ReferenceImageMaxMB != first.ReferenceImageMaxMB ||
		second.ReferenceImageMaxBytes != first.ReferenceImageMaxBytes ||
		!slices.Equal(second.ReferenceImageAllowedFormats, first.ReferenceImageAllowedFormats) ||
		!slices.Equal(second.ReferenceImageAllowedMIMETypes, first.ReferenceImageAllowedMIMETypes) {
		t.Fatalf("capabilities discarded last-known-good policy: first=%#v second=%#v", first, second)
	}
}

func TestCapabilitiesExposeConfiguredModelGroups(t *testing.T) {
	svc := NewService(config.Config{
		Billing: config.BillingConfig{
			BaseResolutionPointsByModel: map[string]map[string]string{
				"basic": {"1k": "2.00000", "2k": "4.00000", "4k": "8.00000"},
				"plus":  {"1k": "5.00000", "2k": "8.00000", "4k": "16.00000"},
			},
			AutoBaseResolutionDefaultByGroup: map[string]string{"basic": "1k", "plus": "2k"},
		},
		GenerationLimits: config.GenerationLimitsConfig{
			MaxImageCount:          5,
			ReferenceImageMaxCount: 4,
			ReferenceImageMaxMB:    12,
		},
		Providers: config.ProvidersConfig{
			OpenAI: config.ProviderConfig{Enabled: true},
		},
		Routing: config.RoutingConfig{
			DefaultProvider: "openai",
			ProviderCapabilities: map[string]config.ProviderCapabilityConfig{
				"openai": {
					SupportedModels:         []string{"basic", "plus"},
					SupportedTaskTypes:      []string{"text_to_image", "image_edit"},
					SupportedBaseResolution: []string{"1k", "2k", "4k"},
					SupportedAspectRatios:   []string{"1:1", "16:9"},
					MaxImageCount:           5,
					MaxReferenceImageCount:  4,
					SupportsImageInput:      true,
					SupportsMask:            true,
				},
			},
		},
	})

	resp := svc.List()
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 capability items, got %d", len(resp.Items))
	}
	if resp.Items[0].AbstractModel == "" || resp.Items[0].MaxOutputImageCount != 5 {
		t.Fatalf("unexpected first capability item: %#v", resp.Items[0])
	}
	if len(resp.Items[0].TaskTypes) != 2 {
		t.Fatalf("expected configured task types, got %#v", resp.Items[0].TaskTypes)
	}
	if resp.ReferenceImageMaxMB != 12 || resp.ReferenceImageMaxBytes != 12*1024*1024 {
		t.Fatalf("expected reference image upload limits, got mb=%d bytes=%d", resp.ReferenceImageMaxMB, resp.ReferenceImageMaxBytes)
	}
}

func TestCapabilitiesUseDynamicBillingConfigForAutoResolutionAndPrices(t *testing.T) {
	cfg := config.Config{Billing: config.BillingConfig{
		AutoBaseResolutionDefaultByGroup: map[string]string{"plus": "1k"},
		TaskMultipliers:                  map[string]string{"text_to_image": "1.00000"},
	}}
	admin := adminconfigservice.NewService(cfg)
	billing := billingservice.NewService(cfg.Billing)
	billing.SetAdminConfigResolver(admin)
	svc := NewService(cfg)
	svc.SetBillingConfigResolver(billing)
	svc.SetModelRoutingSource(staticModelRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices: []modelhub.RoutePriceConfig{
			{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true},
			{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "2k", BasePoints: "2.00000", Enabled: true},
		},
		ProviderModels: []modelhub.ProviderCandidate{{
			AccountModelID: 11, SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k", "2k"},
			SizeModes: []string{"ratio"}, SupportedAspectRatios: []string{"1:1"}, Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"}, MaxImageCount: 1,
		}},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 11, Enabled: true}},
	}})

	first, err := svc.ListForGroups(context.Background(), nil)
	if err != nil || len(first.ModelGroups) != 1 {
		t.Fatalf("first ListForGroups = %#v, %v", first, err)
	}
	if first.ModelGroups[0].AutoBaseResolutionByTaskType["text_to_image"] != "1k" || first.ModelGroups[0].Prices[0].ChargedPoints != "1.00000" {
		t.Fatalf("unexpected initial dynamic capability: %#v", first.ModelGroups[0])
	}
	if _, err := admin.UpdateTab(context.Background(), domainadminconfig.UpdateTabRequest{
		TabKey: "billing_pricing", Version: 1,
		Items: []domainadminconfig.Item{
			{ConfigCategory: "billing_pricing", ConfigKey: "auto_base_resolution_default_by_group", ConfigValue: map[string]any{"value": map[string]any{"plus": "2k"}}, Scope: "global"},
			{ConfigCategory: "billing_pricing", ConfigKey: "task_multipliers", ConfigValue: map[string]any{"value": map[string]any{"text_to_image": "2.00000"}}, Scope: "global"},
		},
	}); err != nil {
		t.Fatalf("UpdateTab: %v", err)
	}
	second, err := svc.ListForGroups(context.Background(), nil)
	if err != nil || len(second.ModelGroups) != 1 {
		t.Fatalf("second ListForGroups = %#v, %v", second, err)
	}
	if second.ModelGroups[0].AutoBaseResolutionByTaskType["text_to_image"] != "2k" || second.ModelGroups[0].Prices[0].ChargedPoints != "2.00000" {
		t.Fatalf("capabilities retained stale billing config: %#v", second.ModelGroups[0])
	}
}
