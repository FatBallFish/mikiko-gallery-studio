package modeladmin

import (
	"encoding/json"
	domainmodeladmin "github.com/fatballfish/pic-gallery/internal/domain/modeladmin"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/domain/imagepricing"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
)

func migrationSnapshot() modelhub.ModelRoutingSnapshot {
	model := "qwen-image-3.0-pro"
	p, _ := nativeimage.PricingForModel(model)
	return modelhub.ModelRoutingSnapshot{RouteModels: []modelhub.RouteModelConfig{{ID: 1, Enabled: true}}, Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 2, Enabled: true}}, ProviderModels: []modelhub.ProviderCandidate{{AccountModelID: 2, AdapterType: nativeimage.AdapterQwen, ModelCode: model, ModelExtra: map[string]any{"native_image": modelhub.NativeImageConfig{SalePricing: &p}}}}}
}
func TestRouteMigrationIsConservativeAndIdempotent(t *testing.T) {
	s := migrationSnapshot()
	plan, err := PlanImagePricingMigration(s)
	if err != nil || len(plan) != 1 {
		t.Fatalf("%+v %v", plan, err)
	}
	if plan[0].ImagePricing.Rules[0].Currency != "CNY" || plan[0].ImagePricing.Rules[0].OutputTiers[0].Price != "0.25" {
		t.Fatal("sale changed")
	}
	s.RouteModels[0].ImagePricing = plan[0].ImagePricing
	again, err := PlanImagePricingMigration(s)
	if err != nil || !again[0].AlreadyApplied {
		t.Fatal("not idempotent", err)
	}
	s.RouteModels[0].ImagePricing = &imagepricing.Config{SchemaVersion: 1}
	if _, err := PlanImagePricingMigration(s); err == nil {
		t.Fatal("route conflict accepted")
	}
	s = migrationSnapshot()
	s.Prices = []modelhub.RoutePriceConfig{{RouteModelID: 1, Enabled: true}}
	if _, err := PlanImagePricingMigration(s); err == nil {
		t.Fatal("legacy conflict accepted")
	}
	s = migrationSnapshot()
	s.ProviderModels[0].ModelExtra = nil
	if _, err := PlanImagePricingMigration(s); err == nil {
		t.Fatal("missing sale accepted")
	}
	s = migrationSnapshot()
	other := s.ProviderModels[0]
	other.AccountModelID = 3
	p, _ := nativeimage.PricingForModel(other.ModelCode)
	p.InputCNY = .99
	other.ModelExtra = map[string]any{"native_image": modelhub.NativeImageConfig{SalePricing: &p}}
	s.ProviderModels = append(s.ProviderModels, other)
	s.Candidates = append(s.Candidates, modelhub.RouteCandidateConfig{RouteModelID: 1, AccountModelID: 3, Enabled: true})
	if _, err := PlanImagePricingMigration(s); err == nil {
		t.Fatal("different candidate sales accepted")
	}
}
func TestRoutePricingRejectsUnsupported4K(t *testing.T) {
	s := migrationSnapshot()
	plan, err := PlanImagePricingMigration(s)
	if err != nil {
		t.Fatal(err)
	}
	c := plan[0].ImagePricing
	c.Rules[0].OutputTiers = append(c.Rules[0].OutputTiers, imagepricing.Tier{Name: "4K", MaxPixels: 16777216, Price: "1"})
	if ValidateRouteImagePricing(c, s.ProviderModels) == nil {
		t.Fatal("unsupported4K accepted")
	}
}

func TestMigrationRejectsMissingNullAndWrongTypeRates(t *testing.T) {
	for _, field := range []string{"input_cny", "output_cny", "layer_cny", "layer_base_cny"} {
		for _, value := range []any{nil, "0", true, "missing"} {
			t.Run(field+fmtValue(value), func(t *testing.T) {
				s := migrationSnapshot()
				cfg := s.ProviderModels[0].ModelExtra["native_image"]
				data, _ := json.Marshal(cfg)
				var node map[string]any
				_ = json.Unmarshal(data, &node)
				sale := node["sale_pricing"].(map[string]any)
				target := sale
				key := field
				if field == "output_cny" {
					target = sale["output_tiers"].([]any)[0].(map[string]any)
					key = "cny"
				}
				if field == "layer_cny" || field == "layer_base_cny" {
					sale["layer_tiers"] = []any{map[string]any{"name": "1K", "max_pixels": 1000000, "cny": 0.1}}
					sale["layer_base_cny"] = 0.2
					if field == "layer_cny" {
						target = sale["layer_tiers"].([]any)[0].(map[string]any)
						key = "cny"
					}
				}
				if value == "missing" {
					delete(target, key)
				} else {
					target[key] = value
				}
				s.ProviderModels[0].ModelExtra["native_image"] = node
				if _, err := PlanImagePricingMigration(s); err == nil {
					t.Fatal("incomplete numeric rate accepted")
				}
			})
		}
	}
	s := migrationSnapshot()
	data, _ := json.Marshal(s.ProviderModels[0].ModelExtra["native_image"])
	var node map[string]any
	_ = json.Unmarshal(data, &node)
	node["sale_pricing"].(map[string]any)["input_cny"] = float64(0)
	s.ProviderModels[0].ModelExtra["native_image"] = node
	if _, err := PlanImagePricingMigration(s); err != nil {
		t.Fatal("explicit zero rejected", err)
	}
}
func fmtValue(v any) string { b, _ := json.Marshal(v); return string(b) }
func TestGenericCandidatePricingCapabilities(t *testing.T) {
	zero := "0"
	config := &imagepricing.Config{SchemaVersion: 1, Rules: []imagepricing.Rule{{Operation: "generate", Mode: "per_image", Currency: "points", Version: "v", Input: imagepricing.Input{PerImage: &zero}, OutputTiers: []imagepricing.Tier{{Name: "1K", MaxPixels: 1048576, Price: "1"}}}}}
	c := modelhub.ProviderCandidate{ModelCode: "generic", SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"}}
	if err := ValidateRouteImagePricing(config, []modelhub.ProviderCandidate{c}); err != nil {
		t.Fatal(err)
	}
	config.Rules[0].Operation = "edit"
	if ValidateRouteImagePricing(config, []modelhub.ProviderCandidate{c}) == nil {
		t.Fatal("generation-only edit accepted")
	}
	c.SupportedTaskTypes = append(c.SupportedTaskTypes, "image_edit")
	if ValidateRouteImagePricing(config, []modelhub.ProviderCandidate{c}) == nil {
		t.Fatal("no image input accepted")
	}
	c.SupportsImageInput = true
	c.MaxReferenceImageCount = 1
	if err := ValidateRouteImagePricing(config, []modelhub.ProviderCandidate{c}); err != nil {
		t.Fatal(err)
	}
	config.Rules[0].OutputTiers[0].MaxPixels = 16777216
	if ValidateRouteImagePricing(config, []modelhub.ProviderCandidate{c}) == nil {
		t.Fatal("unsupported 4K accepted")
	}
}
func TestMemoryRoutePricingOmissionAndNull(t *testing.T) {
	store := NewMemoryStore()
	ctx := t.Context()
	zero := "0"
	config := &imagepricing.Config{SchemaVersion: 1, Rules: []imagepricing.Rule{{Operation: "generate", Mode: "per_image", Currency: "CNY", Version: "v", Input: imagepricing.Input{PerImage: &zero}, OutputTiers: []imagepricing.Tier{{Name: "1K", MaxPixels: 100, Price: "1"}}}}}
	req := domainmodeladmin.RouteModelWriteRequest{Code: "r", Name: "Route", ImagePricing: config}
	route, err := store.CreateRouteModel(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	req.ImagePricing = nil
	route, err = store.UpdateRouteModel(ctx, route.ID, req)
	if err != nil || route.ImagePricing == nil {
		t.Fatal("omission cleared", err)
	}
	req.ImagePricingPresent = true
	route, err = store.UpdateRouteModel(ctx, route.ID, req)
	if err != nil || route.ImagePricing != nil {
		t.Fatal("null not cleared", err)
	}
}

func TestLayerPricingUsesLayerSizeModes(t *testing.T) {
	model := "doubao-seedream-5-0-pro-260628"
	p, _ := nativeimage.ProfileForModel(model)
	zero := "0"
	config := &imagepricing.Config{SchemaVersion: 1, Rules: []imagepricing.Rule{{Operation: "layer_decomposition", Mode: "per_image", Currency: "CNY", Version: "v", Input: imagepricing.Input{PerImage: &zero}, Layers: &imagepricing.Layers{Confirmed: true, BasePrice: &zero, Tiers: []imagepricing.Tier{{Name: "layer", BaseResolution: "1K", Price: "0.1"}}}}}}
	c := modelhub.ProviderCandidate{AdapterType: nativeimage.AdapterSeedream, ModelCode: model, ModelExtra: map[string]any{"native_image": modelhub.NativeImageConfig{Profile: &p}}}
	if err := ValidateRouteImagePricing(config, []modelhub.ProviderCandidate{c}); err != nil {
		t.Fatal("layer size supported independently", err)
	}
	config.Rules[0].Layers.Tiers[0].BaseResolution = "auto"
	if ValidateRouteImagePricing(config, []modelhub.ProviderCandidate{c}) == nil {
		t.Fatal("auto pricing tier accepted")
	}
}
