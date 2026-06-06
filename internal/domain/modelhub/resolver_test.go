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

func TestResolveQualityPrefersExplicitSizeAndFallsBackToAutoGroup(t *testing.T) {
	resolver := modelhub.NewResolver(config.Config{
		Billing: config.BillingConfig{
			AutoQualityDefaultByGroup: map[string]string{"basic": "1k", "plus": "2k"},
			QualityPointsByModel: map[string]map[string]string{
				"basic": {"1k": "2.00000", "2k": "4.00000"},
				"plus":  {"1k": "5.00000", "2k": "8.00000", "4k": "16.00000"},
			},
		},
	})

	quality, err := resolver.ResolveQuality("auto", "1536x1024", "basic")
	if err != nil {
		t.Fatalf("ResolveQuality size-based: %v", err)
	}
	if quality != "2k" {
		t.Fatalf("expected 2k from explicit size, got %s", quality)
	}

	quality, err = resolver.ResolveQuality("auto", "auto", "plus")
	if err != nil {
		t.Fatalf("ResolveQuality auto-group: %v", err)
	}
	if quality != "2k" {
		t.Fatalf("expected 2k from auto group, got %s", quality)
	}
}

func TestResolveAllowsEnabledModelAccountCandidates(t *testing.T) {
	resolver := modelhub.NewResolver(config.Config{
		Billing: config.BillingConfig{
			AutoQualityDefaultByGroup: map[string]string{"basic": "1k"},
			QualityPointsByModel: map[string]map[string]string{
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
					SupportedModels:        []string{"basic"},
					SupportedTaskTypes:     []string{"text_to_image"},
					SupportedQualities:     []string{"1k"},
					MaxImageCount:          5,
					MaxReferenceImageCount: 4,
					Priority:               1,
				},
			},
		},
	})
	resolver.SetModelRoutingSource(testRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		ProviderModels: []modelhub.ProviderCandidate{
			{
				AccountModelID:     1,
				ModelAccountID:     1,
				Provider:           "openrouter",
				AdapterType:        "openrouter",
				AuthType:           "api_key",
				BaseURL:            "http://127.0.0.1:1",
				Credentials:        map[string]string{"api_key": "test-key"},
				ModelCode:          "openai/gpt-image-1",
				SupportedTaskTypes: []string{"text_to_image"},
				SupportedQualities: []string{"1k"},
				HealthStatus:       "enabled",
			},
		},
	}})

	resolved, err := resolver.ResolveContext(context.Background(), modelhub.ResolveRequest{
		AbstractModel:             "basic",
		TaskType:                  "text_to_image",
		RequestedQuality:          "auto",
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

func TestResolveFiltersAndOrdersProviders(t *testing.T) {
	resolver := modelhub.NewResolver(config.Config{
		Billing: config.BillingConfig{
			AutoQualityDefaultByGroup: map[string]string{"plus": "2k"},
			QualityPointsByModel: map[string]map[string]string{
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
					SupportedModels:        []string{"plus"},
					SupportedTaskTypes:     []string{"text_to_image", "image_edit", "reference_generate"},
					SupportedQualities:     []string{"1k", "2k", "4k"},
					SupportedAspectRatios:  []string{"1:1", "4:3", "16:9"},
					MaxImageCount:          5,
					MaxReferenceImageCount: 4,
					SupportsImageInput:     true,
					SupportsMask:           false,
					Priority:               1,
				},
				"openai": {
					SupportedModels:        []string{"plus"},
					SupportedTaskTypes:     []string{"text_to_image", "image_edit", "reference_generate"},
					SupportedQualities:     []string{"1k", "2k", "4k"},
					SupportedAspectRatios:  []string{"1:1", "4:3", "16:9"},
					MaxImageCount:          5,
					MaxReferenceImageCount: 4,
					SupportsImageInput:     true,
					SupportsMask:           true,
					Priority:               2,
				},
			},
		},
	})

	resolved, err := resolver.Resolve(modelhub.ResolveRequest{
		AbstractModel:             "plus",
		TaskType:                  "image_edit",
		RequestedQuality:          "auto",
		RequestedSize:             "auto",
		RequestedOutputImageCount: 2,
		ReferenceImageCount:       1,
	})
	if err != nil {
		t.Fatalf("Resolve image_edit: %v", err)
	}
	if resolved.ResolvedQualityBucket != "2k" {
		t.Fatalf("expected 2k, got %s", resolved.ResolvedQualityBucket)
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
		RequestedQuality:          "auto",
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
