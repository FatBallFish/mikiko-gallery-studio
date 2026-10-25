package openapi

import (
	"os"
	"testing"
)

func TestNativeLayerPricingDiscoveryContract(t *testing.T) {
	for _, path := range []string{"components/schemas/agent.yaml", "../../web/docs/public/openapi/components/schemas/agent.yaml"} {
		schemas := nativeSchemas(t, path)
		for _, name := range []string{"CapabilityItem", "CapabilityModelGroup", "CapabilityTaskOptions"} {
			if got := schemas[name].Properties["native_image_layer_pricing_confirmed"].Type; got != "boolean" {
				t.Errorf("%s %s layer confirmation type = %v", path, name, got)
			}
		}
	}
	source, err := os.ReadFile("components/schemas/agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	mirror, err := os.ReadFile("../../web/docs/public/openapi/components/schemas/agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(source) != string(mirror) {
		t.Fatal("developer agent schema mirror differs from source")
	}
}
