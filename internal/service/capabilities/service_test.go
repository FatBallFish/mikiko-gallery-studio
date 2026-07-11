package capabilities

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
)

func TestVisibleRouteModelJSONUsesSnakeCaseCapabilityFields(t *testing.T) {
	payload, err := json.Marshal(modelhub.VisibleRouteModel{
		Code:                      "plus",
		OutputFormat:              []string{"webp"},
		SupportsOutputCompression: true,
		Prices:                    []modelhub.VisibleRouteModelPrice{{TaskType: "text_to_image", BaseResolution: "1k"}},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	encoded := string(payload)
	for _, expected := range []string{`"output_format"`, `"supports_output_compression"`, `"task_type"`, `"base_resolution"`} {
		if !strings.Contains(encoded, expected) {
			t.Fatalf("expected snake-case field %s in %s", expected, encoded)
		}
	}
	if strings.Contains(encoded, `"SupportsOutputCompression"`) || strings.Contains(encoded, `"OutputFormat"`) {
		t.Fatalf("capability response must not expose Go field names: %s", encoded)
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
