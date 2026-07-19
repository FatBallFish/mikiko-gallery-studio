package modelhub_test

import (
	"context"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
)

type testRoutingSource struct {
	snapshot modelhub.ModelRoutingSnapshot
}

func (s testRoutingSource) ModelRoutingConfig(context.Context) (modelhub.ModelRoutingSnapshot, error) {
	return s.snapshot, nil
}

func TestResolveBaseResolutionPrefersExplicitSizeAndFallsBackToAutoGroup(t *testing.T) {
	resolver := modelhub.NewResolver(config.Config{
		Billing: config.BillingConfig{
			AutoBaseResolutionDefaultByGroup: map[string]string{"basic": "1k", "plus": "2k"},
			BaseResolutionPointsByModel: map[string]map[string]string{
				"basic": {"1k": "2.00000", "2k": "4.00000"},
				"plus":  {"1k": "5.00000", "2k": "8.00000", "4k": "16.00000"},
			},
		},
	})

	baseResolution, err := resolver.ResolveBaseResolution("auto", "1536x1024", "basic")
	if err != nil {
		t.Fatalf("ResolveBaseResolution size-based: %v", err)
	}
	if baseResolution != "2k" {
		t.Fatalf("expected 2k from explicit size, got %s", baseResolution)
	}

	baseResolution, err = resolver.ResolveBaseResolution("auto", "auto", "plus")
	if err != nil {
		t.Fatalf("ResolveBaseResolution auto-group: %v", err)
	}
	if baseResolution != "2k" {
		t.Fatalf("expected 2k from auto group, got %s", baseResolution)
	}
}

func TestResolveAllowsEnabledModelAccountCandidates(t *testing.T) {
	resolver := modelhub.NewResolver(config.Config{
		Billing: config.BillingConfig{
			AutoBaseResolutionDefaultByGroup: map[string]string{"basic": "1k"},
			BaseResolutionPointsByModel: map[string]map[string]string{
				"basic": {"1k": "2.00000"},
			},
		},
		GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 5, ReferenceImageMaxCount: 4},
		Providers: config.ProvidersConfig{
			OpenRouter: config.ProviderConfig{Enabled: true},
		},
		Routing: config.RoutingConfig{
			ProviderCapabilities: map[string]config.ProviderCapabilityConfig{
				"openrouter": {
					SupportedModels:         []string{"basic"},
					SupportedTaskTypes:      []string{"text_to_image"},
					SupportedBaseResolution: []string{"1k"},
					MaxImageCount:           5,
					MaxReferenceImageCount:  4,
					Priority:                1,
				},
			},
		},
	})
	resolver.SetModelRoutingSource(testRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		ProviderModels: []modelhub.ProviderCandidate{
			{
				AccountModelID:          1,
				ModelAccountID:          1,
				Provider:                "openrouter",
				AdapterType:             "openrouter",
				AuthType:                "api_key",
				BaseURL:                 "http://127.0.0.1:1",
				Credentials:             map[string]string{"api_key": "test-key"},
				ModelCode:               "openai/gpt-image-1",
				SupportedTaskTypes:      []string{"text_to_image"},
				SupportedBaseResolution: []string{"1k"},
				HealthStatus:            "enabled",
			},
		},
	}})

	resolved, err := resolver.ResolveContext(context.Background(), modelhub.ResolveRequest{
		AbstractModel:             "basic",
		TaskType:                  "text_to_image",
		BaseResolution:            "auto",
		RequestedSize:             "1024x1024",
		RequestedOutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("Resolve with enabled model account candidate: %v", err)
	}
	if len(resolved.Providers) != 1 || resolved.Providers[0].ModelCode != "openai/gpt-image-1" {
		t.Fatalf("expected enabled account model candidate, got %#v", resolved.Providers)
	}
}

func TestResolveAllowsTaskImageCountAboveProviderBatchLimit(t *testing.T) {
	resolver := modelhub.NewResolver(config.Config{
		Billing: config.BillingConfig{
			AutoBaseResolutionDefaultByGroup: map[string]string{"basic": "1k"},
			BaseResolutionPointsByModel: map[string]map[string]string{
				"basic": {"1k": "2.00000"},
			},
		},
		GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 2, ReferenceImageMaxCount: 4},
		Providers:        config.ProvidersConfig{OpenAI: config.ProviderConfig{Enabled: true}},
		Routing: config.RoutingConfig{ProviderCapabilities: map[string]config.ProviderCapabilityConfig{
			"openai": {
				SupportedModels:         []string{"basic"},
				SupportedTaskTypes:      []string{"text_to_image"},
				SupportedBaseResolution: []string{"1k"},
				MaxImageCount:           2,
				Priority:                1,
			},
		}},
	})

	resolved, err := resolver.Resolve(modelhub.ResolveRequest{
		AbstractModel:             "basic",
		TaskType:                  "text_to_image",
		BaseResolution:            "1k",
		RequestedOutputImageCount: 5,
	})
	if err != nil {
		t.Fatalf("task total should be independent from provider batch limit: %v", err)
	}
	if len(resolved.Providers) != 1 || resolved.Providers[0].MaxImageCount != 2 {
		t.Fatalf("expected provider batch capability to remain available for scheduling, got %#v", resolved.Providers)
	}
}

func TestNormalizeResolveRequestRejectsTaskImageCountAboveSafetyLimit(t *testing.T) {
	_, err := modelhub.NormalizeResolveRequest(modelhub.ResolveRequest{
		TaskType:                  "text_to_image",
		RequestedOutputImageCount: modelhub.MaxTaskOutputImageCount + 1,
	})
	if err == nil {
		t.Fatal("expected technical task image safety limit to reject oversized request")
	}
}

func TestResolveFiltersAndOrdersProviders(t *testing.T) {
	resolver := modelhub.NewResolver(config.Config{
		Billing: config.BillingConfig{
			AutoBaseResolutionDefaultByGroup: map[string]string{"plus": "2k"},
			BaseResolutionPointsByModel: map[string]map[string]string{
				"plus": {"1k": "5.00000", "2k": "8.00000", "4k": "16.00000"},
			},
		},
		GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 5, ReferenceImageMaxCount: 4},
		Providers: config.ProvidersConfig{
			OpenAI:     config.ProviderConfig{Enabled: true},
			OpenRouter: config.ProviderConfig{Enabled: true},
		},
		Routing: config.RoutingConfig{
			DefaultProvider:   "openrouter",
			FallbackProviders: []string{"openai"},
			ProviderCapabilities: map[string]config.ProviderCapabilityConfig{
				"openrouter": {
					SupportedModels:         []string{"plus"},
					SupportedTaskTypes:      []string{"text_to_image", "image_edit", "reference_generate"},
					SupportedBaseResolution: []string{"1k", "2k", "4k"},
					SupportedAspectRatios:   []string{"1:1", "4:3", "16:9"},
					MaxImageCount:           5,
					MaxReferenceImageCount:  4,
					SupportsImageInput:      true,
					SupportsMask:            false,
					Priority:                1,
				},
				"openai": {
					SupportedModels:         []string{"plus"},
					SupportedTaskTypes:      []string{"text_to_image", "image_edit", "reference_generate"},
					SupportedBaseResolution: []string{"1k", "2k", "4k"},
					SupportedAspectRatios:   []string{"1:1", "4:3", "16:9"},
					MaxImageCount:           5,
					MaxReferenceImageCount:  4,
					SupportsImageInput:      true,
					SupportsMask:            true,
					Priority:                2,
				},
			},
		},
	})

	resolved, err := resolver.Resolve(modelhub.ResolveRequest{
		AbstractModel:             "plus",
		TaskType:                  "image_edit",
		BaseResolution:            "auto",
		RequestedSize:             "auto",
		RequestedOutputImageCount: 2,
		ReferenceImageCount:       1,
	})
	if err != nil {
		t.Fatalf("Resolve image_edit: %v", err)
	}
	if resolved.BaseResolution != "2k" {
		t.Fatalf("expected 2k, got %s", resolved.BaseResolution)
	}
	if len(resolved.Providers) != 2 {
		t.Fatalf("expected 2 eligible providers, got %d", len(resolved.Providers))
	}
	if resolved.Providers[0].Provider != "openrouter" || resolved.Providers[1].Provider != "openai" {
		t.Fatalf("unexpected provider order %#v", resolved.Providers)
	}

	resolvedMask, err := resolver.Resolve(modelhub.ResolveRequest{
		AbstractModel:             "plus",
		TaskType:                  "image_edit",
		BaseResolution:            "auto",
		RequestedSize:             "auto",
		RequestedOutputImageCount: 2,
		ReferenceImageCount:       1,
		MaskPresent:               true,
	})
	if err != nil {
		t.Fatalf("Resolve mask image_edit: %v", err)
	}
	if len(resolvedMask.Providers) != 1 || resolvedMask.Providers[0].Provider != "openai" {
		t.Fatalf("expected only openai for mask request, got %#v", resolvedMask.Providers)
	}
}
