package modelhub

import (
	"context"
	"encoding/json"
	"github.com/fatballfish/pic-gallery/internal/domain/imagepricing"
	"math"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
)

func confirmedLayerCandidate() ProviderCandidate {
	model := "doubao-seedream-5-0-pro-260628"
	cost, _ := nativeimage.PricingForModel(model)
	base := 0.3
	cost.LayerBaseCNY = &base
	cost.LayerPricingConfirmed = true
	sale := cost
	return ProviderCandidate{AccountModelID: 2, AdapterType: nativeimage.AdapterSeedream, ModelCode: model, ModelExtra: map[string]any{"native_image": NativeImageConfig{Pricing: &cost, SalePricing: &sale}}}
}

func TestNativeLayerPricingCapabilityRequiresCompleteConfirmedPrices(t *testing.T) {
	cases := []struct {
		name   string
		change func(*NativeImageConfig)
		want   bool
	}{
		{"confirmed", func(*NativeImageConfig) {}, true},
		{"missing cost", func(c *NativeImageConfig) { c.Pricing = nil }, false},
		{"missing sale", func(c *NativeImageConfig) { c.SalePricing = nil }, true},
		{"unconfirmed cost", func(c *NativeImageConfig) { c.Pricing.LayerPricingConfirmed = false }, false},
		{"unconfirmed sale", func(c *NativeImageConfig) { c.Pricing.LayerPricingConfirmed = false }, false},
		{"unknown base", func(c *NativeImageConfig) { c.Pricing.LayerBaseCNY = nil }, false},
		{"negative base", func(c *NativeImageConfig) { v := -0.1; c.Pricing.LayerBaseCNY = &v }, false},
		{"missing layer tiers", func(c *NativeImageConfig) { c.Pricing.LayerTiers = nil }, false},
		{"incomplete coverage", func(c *NativeImageConfig) { c.Pricing.LayerTiers = c.Pricing.LayerTiers[:1] }, false},
		{"unordered tiers", func(c *NativeImageConfig) { c.Pricing.LayerTiers[1].MaxPixels = 1 }, false},
		{"negative rate", func(c *NativeImageConfig) { c.Pricing.LayerTiers[0].CNY = -1 }, false},
		{"invalid output pricing", func(c *NativeImageConfig) { c.Pricing.OutputTiers = nil }, false},
		{"wrong model", func(c *NativeImageConfig) { c.Pricing.Model = "unknown" }, false},
		{"wrong currency", func(c *NativeImageConfig) { c.Pricing.Currency = "USD" }, false},
		{"no layer operation", func(c *NativeImageConfig) {
			p, _ := nativeimage.ProfileForModel(c.Pricing.Model)
			p.Operations = []nativeimage.Operation{nativeimage.OperationGenerate, nativeimage.OperationEdit}
			c.Profile = &p
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := confirmedLayerCandidate()
			cfg := candidate.ModelExtra["native_image"].(NativeImageConfig)
			tc.change(&cfg)
			candidate.ModelExtra["native_image"] = cfg
			snapshot := ModelRoutingSnapshot{Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 2, Enabled: true}}, ProviderModels: []ProviderCandidate{candidate}}
			if got := nativeRouteLayerPricingConfirmed(RouteModelConfig{ID: 1, ImagePricing: layerRoutePrice()}, snapshot); got != tc.want {
				t.Fatalf("confirmed = %v, want %v", got, tc.want)
			}
		})
	}
	// Non-finite values must also fail before any price can be advertised.
	candidate := confirmedLayerCandidate()
	cfg := candidate.ModelExtra["native_image"].(NativeImageConfig)
	for _, invalid := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		cfg.Pricing.LayerBaseCNY = &invalid
		if nativeLayerPricingValid(cfg.Pricing, mustLayerProfile(t)) {
			t.Fatal("non-finite base accepted")
		}
	}
}

func mustLayerProfile(t *testing.T) *nativeimage.CapabilityProfile {
	t.Helper()
	p, ok := nativeimage.ProfileForModel("doubao-seedream-5-0-pro-260628")
	if !ok {
		t.Fatal("missing seedream profile")
	}
	return &p
}

func TestNativeLayerPricingCapabilityConservativeFallbackAndProjection(t *testing.T) {
	for _, status := range []string{"healthy", "unknown", "unhealthy", "disabled"} {
		t.Run(status, func(t *testing.T) {
			good := confirmedLayerCandidate()
			fallback := good
			fallback.AccountModelID = 3
			fallback.HealthStatus = status
			fallback.ModelExtra = nil // Unknown cost/sale must never become zero.
			snapshot := ModelRoutingSnapshot{RouteModels: []RouteModelConfig{{ID: 1, ImagePricing: layerRoutePrice(), Code: "layers", Visibility: "public", Enabled: true}}, Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 2, Enabled: true}, {RouteModelID: 1, AccountModelID: 3, Enabled: true}}, ProviderModels: []ProviderCandidate{good, fallback}}
			r := NewResolver(config.Config{})
			r.SetModelRoutingSource(staticRoutingSource{snapshot: snapshot})
			items, err := r.ListVisibleRouteModels(context.Background(), nil, nil)
			if err != nil || len(items) != 1 {
				t.Fatalf("items=%+v err=%v", items, err)
			}
			want := status == "unhealthy" || status == "disabled"
			if items[0].NativeImageLayerPricingConfirmed != want {
				t.Fatalf("route confirmation = %v, want %v", items[0].NativeImageLayerPricingConfirmed, want)
			}
			for task, capability := range items[0].CapabilitiesByTaskType {
				if capability.NativeImageLayerPricingConfirmed != want {
					t.Fatalf("%s confirmation = %v, want %v", task, capability.NativeImageLayerPricingConfirmed, want)
				}
			}
			data, err := json.Marshal(items[0])
			if err != nil {
				t.Fatal(err)
			}
			var result map[string]any
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatal(err)
			}
			if result["native_image_layer_pricing_confirmed"] != want {
				t.Fatalf("JSON boolean missing or incorrect: %s", data)
			}
		})
	}
	if nativeRouteLayerPricingConfirmed(RouteModelConfig{ID: 1, ImagePricing: layerRoutePrice()}, ModelRoutingSnapshot{}) {
		t.Fatal("empty route confirmed")
	}
	for _, kind := range []string{"disabled route candidate", "mixed generic fallback", "non-layer native fallback"} {
		t.Run(kind, func(t *testing.T) {
			good := confirmedLayerCandidate()
			fallback := ProviderCandidate{AccountModelID: 3, AdapterType: "openai"}
			rc := RouteCandidateConfig{RouteModelID: 1, AccountModelID: 3, Enabled: true}
			want := false
			if kind == "disabled route candidate" {
				rc.Enabled = false
				want = true
			}
			if kind == "non-layer native fallback" {
				fallback.AdapterType = nativeimage.AdapterSeedream
				fallback.ModelCode = "doubao-seedream-5-0-flash-260915"
			}
			snapshot := ModelRoutingSnapshot{Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 2, Enabled: true}, rc}, ProviderModels: []ProviderCandidate{good, fallback}}
			if got := nativeRouteLayerPricingConfirmed(RouteModelConfig{ID: 1, ImagePricing: layerRoutePrice()}, snapshot); got != want {
				t.Fatalf("confirmed = %v, want %v", got, want)
			}
		})
	}
}

func layerRoutePrice() *imagepricing.Config {
	zero := "0"
	base := "0.3"
	return &imagepricing.Config{SchemaVersion: 1, Rules: []imagepricing.Rule{{Operation: "layer_decomposition", Mode: "per_image", Currency: "CNY", Version: "1", Input: imagepricing.Input{PerImage: &zero}, Layers: &imagepricing.Layers{Confirmed: true, BasePrice: &base, Tiers: []imagepricing.Tier{{Name: "all", MaxPixels: 20000000, Price: "0.15"}}}}}}
}

func TestNativeLayerUnavailableReasonChineseMissingFields(t *testing.T) {
	candidate := confirmedLayerCandidate()
	cfg := candidate.ModelExtra["native_image"].(NativeImageConfig)
	profile := mustLayerProfile(t)
	profile.Operations = []nativeimage.Operation{nativeimage.OperationGenerate}
	profile.LayerDecomposition = false
	profile.MaxLayers = 0
	profile.LayerSizeModes = nil
	cfg.Profile = profile
	candidate.ModelExtra["native_image"] = cfg
	snapshot := ModelRoutingSnapshot{Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 2, Enabled: true}}, ProviderModels: []ProviderCandidate{candidate}}
	reason := nativeRouteLayerUnavailableReason(RouteModelConfig{ID: 1}, snapshot)
	for _, text := range []string{"未声明拆层操作", "未开启拆层开关", "最大图层数", "拆层尺寸模式", "路由未配置拆层销售价格"} {
		if !strings.Contains(reason, text) {
			t.Fatalf("missing %q in %q", text, reason)
		}
	}
	if strings.Contains(reason, "candidate") || strings.Contains(reason, "2:") {
		t.Fatal(reason)
	}
	candidate = confirmedLayerCandidate()
	cfg = candidate.ModelExtra["native_image"].(NativeImageConfig)
	cfg.Pricing.LayerPricingConfirmed = false
	cfg.Pricing.LayerBaseCNY = nil
	cfg.Pricing.LayerTiers = nil
	candidate.ModelExtra["native_image"] = cfg
	snapshot.ProviderModels = []ProviderCandidate{candidate}
	price := layerRoutePrice()
	price.Rules[0].Layers.Confirmed = false
	price.Rules[0].Layers.BasePrice = nil
	price.Rules[0].Layers.Tiers = nil
	reason = nativeRouteLayerUnavailableReason(RouteModelConfig{ID: 1, ImagePricing: price}, snapshot)
	for _, text := range []string{"拆层成本尚未确认", "拆层成本缺少底图价格", "拆层成本缺少完整有效的图层价格档位", "路由拆层销售价格尚未确认", "路由拆层销售价格缺少底图价格", "路由拆层销售价格缺少图层价格档位"} {
		if !strings.Contains(reason, text) {
			t.Fatalf("missing %q in %q", text, reason)
		}
	}
}
