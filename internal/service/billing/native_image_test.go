package billing

import (
	"context"
	"github.com/fatballfish/pic-gallery/internal/config"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/internal/domain/imagepricing"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"testing"
)

func TestNativeEstimateVersionMatchesCreatePreflightLimits(t *testing.T) {
	model := "doubao-seedream-5-0-flash-260915"
	profile, _ := nativeimage.ProfileForModel(model)
	cost, _ := nativeimage.PricingForModel(model)
	base := .3
	cost.LayerBaseCNY, cost.LayerPricingConfirmed = &base, true
	cost.LayerTiers = []nativeimage.PriceTier{{Name: "all", MaxPixels: 20000000, CNY: .15}}
	zero, layerBase := "0", "0.3"
	price := &imagepricing.Config{SchemaVersion: 1, Rules: []imagepricing.Rule{
		{Operation: "generate", Mode: "per_image", Currency: "CNY", Version: "1", Input: imagepricing.Input{PerImage: &zero}, OutputTiers: []imagepricing.Tier{{Name: "all", MaxPixels: 20000000, Price: "0.15"}}},
		{Operation: "edit", Mode: "per_image", Currency: "CNY", Version: "1", Input: imagepricing.Input{PerImage: &zero}, OutputTiers: []imagepricing.Tier{{Name: "all", MaxPixels: 20000000, Price: "0.15"}}},
		{Operation: "layer_decomposition", Mode: "per_image", Currency: "CNY", Version: "1", Input: imagepricing.Input{PerImage: &zero}, Layers: &imagepricing.Layers{Confirmed: true, BasePrice: &layerBase, Tiers: []imagepricing.Tier{{Name: "all", MaxPixels: 20000000, Price: "0.15"}}}},
	}}
	routing := modelhub.ModelRoutingSnapshot{
		RouteModels:    []modelhub.RouteModelConfig{{ID: 1, Code: "native", Visibility: "public", Enabled: true, ImagePricing: price}},
		Candidates:     []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 2, Enabled: true}},
		ProviderModels: []modelhub.ProviderCandidate{{AccountModelID: 2, AdapterType: nativeimage.AdapterSeedream, ModelCode: model, ModelExtra: map[string]any{"native_image": modelhub.NativeImageConfig{Model: model, Profile: &profile, Pricing: &cost, SalePricing: &cost}}}},
	}
	for _, platform := range []struct {
		name      string
		limits    config.GenerationLimitsConfig
		configure bool
	}{
		{"defaults", config.GenerationLimitsConfig{MaxImageCount: 5, ReferenceImageMaxCount: 4}, false},
		{"custom", config.GenerationLimitsConfig{MaxImageCount: 2, ReferenceImageMaxCount: 2}, true},
		{"zero_defaults", config.GenerationLimitsConfig{MaxImageCount: 5, ReferenceImageMaxCount: 4}, true},
	} {
		limits := platform.limits
		for _, op := range []nativeimage.Operation{nativeimage.OperationGenerate, nativeimage.OperationEdit, nativeimage.OperationLayerDecomposition} {
			t.Run(platform.name+"/"+string(op), func(t *testing.T) {
				svc := NewService(config.BillingConfig{CNYPerPoint: "0.01"})
				if platform.configure {
					configuredLimits := limits
					if platform.name == "zero_defaults" {
						configuredLimits = config.GenerationLimitsConfig{}
					}
					svc.SetGenerationLimits(configuredLimits)
				}
				svc.SetModelRoutingSource(staticRoutingSource{snapshot: routing})
				req := domainbilling.EstimateRequest{RouteModelCode: "native", TaskType: "image_edit", Operation: op, SizeMode: "auto", BaseResolution: "1K", OutputFormat: "png", RequestedOutputImageCount: 1, ReferenceImageCount: 1}
				if op == nativeimage.OperationGenerate {
					req.TaskType, req.ReferenceImageCount = "text_to_image", 0
				}
				quote, err := svc.EstimateContext(context.Background(), req)
				if err != nil {
					t.Fatal(err)
				}
				req.CapabilityVersion = quote.CapabilityVersion
				resolved, preflight, err := svc.ResolveAndEstimateRouteTask(context.Background(), req, limits)
				if err != nil {
					t.Fatalf("fresh quote rejected by create preflight: %v", err)
				}
				if quote.CapabilityVersion == "" || quote.CapabilityVersion != resolved.CapabilityVersion || quote.CapabilityVersion != preflight.CapabilityVersion || quote.EstimatedPoints != preflight.EstimatedPoints {
					t.Fatalf("quote/preflight mismatch: quote=%+v preflight=%+v", quote, preflight)
				}
				if op == nativeimage.OperationLayerDecomposition && preflight.PricingSnapshot.NativeImage.Operation != op {
					t.Fatal("layer quote lost its operation")
				}
				// A real sale change must still invalidate the previously authorized quote.
				changedPrice := *price
				changedPrice.Rules = append([]imagepricing.Rule(nil), price.Rules...)
				changedPrice.Rules[0].Version = "2"
				changedRouting := routing
				changedRouting.RouteModels = append([]modelhub.RouteModelConfig(nil), routing.RouteModels...)
				changedRouting.RouteModels[0].ImagePricing = &changedPrice
				svc.SetModelRoutingSource(staticRoutingSource{snapshot: changedRouting})
				if _, _, err := svc.ResolveAndEstimateRouteTask(context.Background(), req, limits); err == nil {
					t.Fatal("stale quote accepted after sale configuration changed")
				} else if appErr, ok := err.(*errs.Error); !ok || appErr.Code != modelhub.CodeCapabilityChanged {
					t.Fatalf("stale quote error = %v, want capability_changed", err)
				}
			})
		}
	}
}

func TestNativeEstimateLayerReservationAndRuntimeRates(t *testing.T) {
	model := "doubao-seedream-5-0-pro-260628"
	base := .3
	p, _ := nativeimage.PricingForModel(model)
	p.LayerBaseCNY = &base
	p.LayerPricingConfirmed = true
	c := modelhub.ProviderCandidate{AccountModelID: 9, AdapterType: nativeimage.AdapterSeedream, ModelCode: model, ModelExtra: map[string]any{"native_image": modelhub.NativeImageConfig{Model: model, Pricing: &p, SalePricing: &p}}}
	req := domainbilling.EstimateRequest{Operation: nativeimage.OperationLayerDecomposition, ReferenceImageCount: 1, RequestedOutputImageCount: 1}
	resolved := modelhub.ResolvedRequest{Providers: []modelhub.ProviderCandidate{c}, EffectiveMultiplier: "1", ResolvedSize: "1K"}
	zero := "0"
	basePrice := "0.3"
	price := &imagepricing.Config{SchemaVersion: 1, Rules: []imagepricing.Rule{{Operation: "layer_decomposition", Mode: "per_image", Currency: "CNY", Version: "1", Input: imagepricing.Input{PerImage: &zero}, Layers: &imagepricing.Layers{Confirmed: true, BasePrice: &basePrice, Tiers: []imagepricing.Tier{{Name: "all", MaxPixels: 20000000, Price: "0.15"}}}}}}
	got, e := estimateNative(req, resolved, "0.01", "1", price)
	if e != nil {
		t.Fatal(e)
	}
	if got.EstimatedPoints != "270.00000" || got.PricingSnapshot.NativeImage.ReservedOutputs != 17 {
		t.Fatalf("estimate %+v", got)
	}
	settled, e := domainbilling.SettleNativeImage(got.PricingSnapshot, 9, 3, []int{1000000, 1000000, 1000000})
	if e != nil || settled.ActualPoints != "60.00000" {
		t.Fatal(settled, e)
	}
	partial, err := domainbilling.SettleNativeImageArtifacts(got.PricingSnapshot, 9, []domainbilling.NativeImageArtifact{{Pixels: 1000000}, {Pixels: 1000000}})
	if err != nil || partial.PricingIncomplete || partial.ActualPoints != "30.00000" || partial.SaleCNY != "0.30000" {
		t.Fatalf("missing base partial overcharge: %+v %v", partial, err)
	}
	c.ModelExtra = nil
	resolved.Providers = []modelhub.ProviderCandidate{c}
	if _, e = estimateNative(req, resolved, "0.01", "1"); e == nil {
		t.Fatal("missing runtime pricing must not use preset")
	}
}

func TestGenericRouteUnifiedPriceWithoutLegacyRows(t *testing.T) {
	in := "0.2"
	price := &imagepricing.Config{SchemaVersion: 1, Rules: []imagepricing.Rule{{Operation: "edit", Mode: "per_image", Currency: "CNY", Version: "1", Input: imagepricing.Input{FreeImages: 1, PerImage: &in}, OutputTiers: []imagepricing.Tier{{Name: "1k", BaseResolution: "1k", Price: "0.3"}}}}}
	routing := modelhub.ModelRoutingSnapshot{RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "unified", Visibility: "public", Enabled: true, ImagePricing: price}}, Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 2, Enabled: true}}, ProviderModels: []modelhub.ProviderCandidate{{AccountModelID: 2, ModelCode: "gpt-image-1", SupportedTaskTypes: []string{"image_edit"}, SupportsImageInput: true, MaxReferenceImageCount: 3, MaxImageCount: 2, SupportedBaseResolution: []string{"1k"}, SupportedAspectRatios: []string{"1:1"}, SizeModes: []string{"ratio"}}}}
	cfg := config.BillingConfig{CNYPerPoint: "0.01"}
	svc := NewService(cfg)
	svc.SetModelRoutingSource(staticRoutingSource{snapshot: routing})
	req := domainbilling.EstimateRequest{RouteModelCode: "unified", TaskType: "image_edit", SizeMode: "ratio", AspectRatio: "1:1", BaseResolution: "1k", RequestedOutputImageCount: 2, ReferenceImageCount: 2}
	_, estimate, err := svc.ResolveAndEstimateRouteTask(context.Background(), req, config.GenerationLimitsConfig{MaxImageCount: 4, ReferenceImageMaxCount: 4})
	if err != nil {
		t.Fatal(err)
	}
	if estimate.EstimatedPoints != "80.00000" || estimate.PricingSnapshot.RouteImage == nil {
		t.Fatalf("%+v", estimate)
	}
	actual, err := svc.ActualPoints(estimate.PricingSnapshot, 1)
	if err != nil || actual != "50.00000" {
		t.Fatalf("actual=%s %v", actual, err)
	}
	cfg.CNYPerPoint = "0.02"
	svc2 := NewService(cfg)
	svc2.SetModelRoutingSource(staticRoutingSource{snapshot: routing})
	_, next, err := svc2.ResolveAndEstimateRouteTask(context.Background(), req, config.GenerationLimitsConfig{MaxImageCount: 4, ReferenceImageMaxCount: 4})
	if err != nil {
		t.Fatal(err)
	}
	if next.CapabilityVersion == estimate.CapabilityVersion {
		t.Fatal("FX change omitted from hash")
	}
	price.Rules[0].Input.FreeImages = 0
	_, changed, err := svc.ResolveAndEstimateRouteTask(context.Background(), req, config.GenerationLimitsConfig{MaxImageCount: 4, ReferenceImageMaxCount: 4})
	if err != nil {
		t.Fatal(err)
	}
	if changed.CapabilityVersion == estimate.CapabilityVersion {
		t.Fatal("sale rule change omitted from hash")
	}
	frozenActual, err := svc.ActualPoints(estimate.PricingSnapshot, 1)
	if err != nil || frozenActual != "50.00000" {
		t.Fatalf("admin edit mutated frozen quote: %s %v", frozenActual, err)
	}
}

func TestNativeTokenEstimateRequiresTrustedDimensionsAndRouteSale(t *testing.T) {
	model := "qwen-image-2.1-pro"
	cost, ok := nativeimage.PricingForModel(model)
	if !ok {
		t.Fatal("missing cost fixture")
	}
	candidate := modelhub.ProviderCandidate{AccountModelID: 7, AdapterType: nativeimage.AdapterQwen, ModelCode: model, ModelExtra: map[string]any{"native_image": modelhub.NativeImageConfig{Model: model, Pricing: &cost, SalePricing: &cost}}}
	resolved := modelhub.ResolvedRequest{Providers: []modelhub.ProviderCandidate{candidate}, EffectiveMultiplier: "1", ResolvedSize: "1024x1024", BaseResolution: "1k"}
	req := domainbilling.EstimateRequest{Operation: nativeimage.OperationEdit, ReferenceImageCount: 2, RequestedOutputImageCount: 2, TrustedInputDimensions: []imagepricing.Dimensions{{Width: 1, Height: 1}, {Width: 5, Height: 1}}}
	if _, err := estimateNative(req, resolved, "0.01", "1"); err == nil {
		t.Fatal("provider sale used without route price")
	}
	price := &imagepricing.Config{SchemaVersion: 1, Rules: []imagepricing.Rule{{Operation: "edit", Mode: "pixel_token", Currency: "points", Version: "1", Input: imagepricing.Input{FreeImages: 1}, PixelToken: &imagepricing.PixelToken{Definition: "platform_pixels_v1", PixelsPerToken: 4, InputPerMillion: "1", OutputPerMillion: "1"}}}}
	estimate, err := estimateNative(req, resolved, "0.01", "1", price)
	if err != nil {
		t.Fatal(err)
	}
	got, err := domainbilling.SettleNativeImage(estimate.PricingSnapshot, 7, 2, []int{1048576, 1048576})
	if err != nil || got.ActualPoints != estimate.EstimatedPoints {
		t.Fatalf("estimate=%s actual=%+v err=%v", estimate.EstimatedPoints, got, err)
	}
	req.TrustedInputDimensions = nil
	if _, err := estimateNative(req, resolved, "0.01", "1", price); err == nil {
		t.Fatal("token estimate accepted missing dimensions")
	}
}
