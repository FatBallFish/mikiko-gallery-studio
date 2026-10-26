package openapi

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"gopkg.in/yaml.v3"
)

type nativeSchema struct {
	Ref        string                  `yaml:"$ref"`
	Type       any                     `yaml:"type"`
	Properties map[string]nativeSchema `yaml:"properties"`
	Required   []string                `yaml:"required"`
	Enum       []string                `yaml:"enum"`
	Items      *nativeSchema           `yaml:"items"`
	Minimum    *int                    `yaml:"minimum"`
	Maximum    *int                    `yaml:"maximum"`
	MinItems   int                     `yaml:"minItems"`
	MaxItems   int                     `yaml:"maxItems"`
	OneOf      []nativeSchema          `yaml:"oneOf"`
}

func nativeSchemas(t *testing.T, path string) map[string]nativeSchema {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]nativeSchema `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Components.Schemas
}

func TestNativeImageSchemasMatchSerializableTypes(t *testing.T) {
	schemas := nativeSchemas(t, "components/schemas/common.yaml")
	for name, value := range map[string]any{
		"NativeImageConfig":            modelhub.NativeImageConfig{},
		"NativeImagePricing":           nativeimage.Pricing{},
		"NativeImagePriceTier":         nativeimage.PriceTier{},
		"NativeImageOptions":           nativeimage.Options{},
		"NativeImageAnnotation":        nativeimage.Annotation{},
		"NativeImageLayerMetadata":     nativeimage.LayerMetadata{},
		"NativeImageCapabilityProfile": nativeimage.CapabilityProfile{},
	} {
		t.Run(name, func(t *testing.T) {
			schema, ok := schemas[name]
			if !ok {
				t.Fatalf("missing %s", name)
			}
			// Compare JSON tags including optional fields against the wire schema.
			typ := reflect.TypeOf(value)
			fields := map[string]bool{}
			for i := 0; i < typ.NumField(); i++ {
				field := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
				fields[field] = true
			}
			for field := range fields {
				if _, ok := schema.Properties[field]; !ok {
					t.Errorf("missing JSON field %s", field)
				}
			}
			for field := range schema.Properties {
				if !fields[field] {
					t.Errorf("invented JSON field %s", field)
				}
			}
		})
	}
	if got := schemas["NativeImageOperation"].Enum; !reflect.DeepEqual(got, []string{string(nativeimage.OperationGenerate), string(nativeimage.OperationEdit), string(nativeimage.OperationLayerDecomposition)}) {
		t.Fatalf("operation enum drift: %v", got)
	}
	if got := schemas["ImageResult"].Properties["layer"].Ref; got != "#/components/schemas/NativeImageLayerMetadata" {
		t.Fatalf("results[].layer ref = %q", got)
	}
	annotation := schemas["NativeImageAnnotation"]
	if len(annotation.OneOf) != 2 {
		t.Fatal("annotations must require exactly one point or bbox")
	}
	for name, count := range map[string]int{"point": 2, "bbox": 4} {
		field := annotation.Properties[name]
		if field.MinItems != count || field.MaxItems != count || field.Items == nil || field.Items.Minimum == nil || *field.Items.Minimum != 0 || field.Items.Maximum == nil || *field.Items.Maximum != 999 {
			t.Errorf("incorrect %s prompt coordinate contract", name)
		}
	}
}

func TestPublicMediaFeedSummaryBoundary(t *testing.T) {
	schemas := nativeSchemas(t, "openapi.yaml")
	video := schemas["PublicMediaVideoSummary"]
	for _, private := range []string{"prompt", "owner_id", "project_id", "review_reason"} {
		if _, ok := video.Properties[private]; ok {
			t.Errorf("public feed video leaks %s", private)
		}
	}
	item := schemas["PublicMediaItem"]
	if len(item.OneOf) != 2 {
		t.Fatal("mixed media feed must define image and video variants")
	}
	if item.OneOf[0].Properties["image"].Ref != "./components/schemas/common.yaml#/components/schemas/PublicGalleryListImage" {
		t.Fatal("feed images must use the redacted public list projection")
	}
	if item.OneOf[1].Properties["video"].Ref != "#/components/schemas/PublicMediaVideoSummary" {
		t.Fatal("feed videos must use the prompt-free summary")
	}
}

func TestNativeImageRequestsAndDiscovery(t *testing.T) {
	root := nativeSchemas(t, "openapi.yaml")
	for _, name := range []string{"DeveloperImagePreflightRequest", "DeveloperImageCreateRequest"} {
		for field, target := range map[string]string{"operation": "NativeImageOperation", "native_options": "NativeImageOptions"} {
			if root[name].Properties[field].Ref != "./components/schemas/common.yaml#/components/schemas/"+target {
				t.Errorf("%s.%s must reuse shared schema", name, field)
			}
		}
	}
	agent := nativeSchemas(t, "components/schemas/agent.yaml")
	for _, name := range []string{"CapabilityItem", "CapabilityTaskOptions"} {
		if agent[name].Properties["native_image_profile"].Ref != "./common.yaml#/components/schemas/NativeImageCapabilityProfile" {
			t.Errorf("%s missing native_image_profile", name)
		}
	}
	for _, spec := range openplatform.Specs() {
		if spec.Name != "get_image_task_layers" {
			continue
		}
		if spec.Scope != "images:read" {
			t.Fatalf("layer manifest scope = %s", spec.Scope)
		}
		return
	}
	t.Fatal("get_image_task_layers is not implemented in the operation catalogue")
}
