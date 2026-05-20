package modelhub_test

import (
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
)

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
