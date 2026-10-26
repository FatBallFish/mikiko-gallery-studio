package billing

import (
	"github.com/fatballfish/pic-gallery/internal/domain/imagepricing"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"testing"
)

func TestNativeLayerRolesAndTerminalMissingTier(t *testing.T) {
	p, _ := nativeimage.PricingForModel("doubao-seedream-5-0-pro-260628")
	base := .4
	p.LayerBaseCNY = &base
	p.LayerPricingConfirmed = true
	snapshot := PricingSnapshot{EstimatedPoints: "280", NativeImage: &NativeImagePricingSnapshot{Operation: nativeimage.OperationLayerDecomposition, CNYPerPoint: ".01", SaleMultiplier: "1", InputImages: 1, ReservedOutputs: 17, ReservedPixels: 1000000, Candidates: []NativeImageCandidatePricing{{AccountModelID: 1, Cost: p, Sale: p}}}}
	for _, tc := range []struct {
		name         string
		artifacts    []NativeImageArtifact
		points, cost string
	}{
		{"no base", []NativeImageArtifact{{Pixels: 1000000}, {Pixels: 1000000}}, "30.00000", "0.30000"},
		{"base last", []NativeImageArtifact{{Pixels: 1000000}, {Pixels: 1000000, IsBase: true}}, "55.00000", "0.55000"},
		{"base only", []NativeImageArtifact{{Pixels: 1000000, IsBase: true}}, "40.00000", "0.40000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, e := SettleNativeImageArtifacts(snapshot, 1, tc.artifacts)
			if e != nil || got.ActualPoints != tc.points || got.CostCNY != tc.cost || !got.Terminal || got.PricingIncomplete {
				t.Fatalf("%+v %v", got, e)
			}
		})
	}
	// An actual artifact outside all frozen tiers is a deterministic terminal event.
	got, e := SettleNativeImageArtifacts(snapshot, 1, []NativeImageArtifact{{Pixels: 5000000}})
	if e != nil || !got.Terminal || !got.PricingIncomplete || got.Diagnostic == "" || got.ActualPoints != "280.00000" || got.RevenueCNY != "2.80000" || got.CostCNY != "" || got.SaleCNY != "" {
		t.Fatalf("terminal %+v %v", got, e)
	}
	got, e = SettleNativeImageArtifacts(snapshot, 1, nil)
	if e != nil || got.ActualPoints != "0.00000" || !got.Terminal {
		t.Fatal(got, e)
	}
}

func TestNativeDecimalInputTierAndSettlement(t *testing.T) {
	p := nativeimage.Pricing{Model: "test", Version: "v1", Currency: "CNY", InputCNY: .02, FreeInputImages: 1, OutputTiers: []nativeimage.PriceTier{{Name: "small", MaxPixels: 2610000, CNY: .3}, {Name: "large", MaxPixels: 4624220, CNY: .6}}}
	c, e := NativeImageCNY(p, nativeimage.OperationEdit, 3, []int{2610000, 2610001})
	if e != nil || c.String() != "0.94" {
		t.Fatalf("cost %v %v", c, e)
	}
	snap := PricingSnapshot{EstimatedPoints: "60", NativeImage: &NativeImagePricingSnapshot{Operation: nativeimage.OperationEdit, CNYPerPoint: "0.02", SaleMultiplier: "1.5", InputImages: 3, ReservedOutputs: 2, ReservedPixels: 4624220, Candidates: []NativeImageCandidatePricing{{AccountModelID: 7, Cost: p, Sale: p}}}}
	got, e := SettleNativeImage(snap, 7, 1, []int{2610000})
	if e != nil || got.ActualPoints != "25.50000" || got.CostCNY != "0.34000" || got.SaleCNY != "0.51000" || got.RevenueCNY != "0.51000" {
		t.Fatalf("settlement %+v %v", got, e)
	}
	got, e = SettleNativeImage(snap, 7, 0, nil)
	if e != nil || got.ActualPoints != "0.00000" {
		t.Fatal(got, e)
	}
	snap.EstimatedPoints = "10"
	got, e = SettleNativeImage(snap, 7, 3, []int{2610000, 2610000, 2610000})
	if e != nil || got.ActualPoints != "10.00000" || !got.ReservationExceeded || got.UnchargedPoints != "60.50000" || got.CostCNY != "0.94000" || got.RevenueCNY != "0.20000" || got.SaleCNY != "1.41000" {
		t.Fatalf("overage %+v %v", got, e)
	}
	snap.NativeImage.CNYPerPoint = "0"
	if _, e = SettleNativeImage(snap, 7, 1, nil); e == nil {
		t.Fatal("invalid exchange accepted")
	}
}
func TestNativeLayersRequireExplicitConfirmation(t *testing.T) {
	p, _ := nativeimage.PricingForModel("doubao-seedream-5-0-pro-260628")
	if _, e := NativeImageCNY(p, nativeimage.OperationLayerDecomposition, 1, []int{1000000, 1000000}); e == nil {
		t.Fatal("unconfirmed base accepted")
	}
	base := .3
	p.LayerBaseCNY = &base
	p.LayerPricingConfirmed = true
	pixels := make([]int, 17)
	for i := range pixels {
		pixels[i] = 1000000
	}
	cost, e := NativeImageCNY(p, nativeimage.OperationLayerDecomposition, 1, pixels)
	if e != nil || cost.String() != "2.7" {
		t.Fatal(cost, e)
	}
	cost, e = NativeImageCNY(p, nativeimage.OperationLayerDecomposition, 1, pixels[:3])
	if e != nil || cost.String() != "0.6" {
		t.Fatal(cost, e)
	}
	flash, _ := nativeimage.PricingForModel("doubao-seedream-5-0-flash-260915")
	flash.LayerPricingConfirmed = true
	flash.LayerBaseCNY = &base
	if _, e = NativeImageCNY(flash, nativeimage.OperationLayerDecomposition, 1, pixels); e == nil {
		t.Fatal("missing Flash layer price accepted")
	}
}

func TestFrozenRouteSaleInputAllowanceAndTokenSettlement(t *testing.T) {
	cost := nativeimage.Pricing{Model: "cost", Version: "1", Currency: "CNY", OutputTiers: []nativeimage.PriceTier{{Name: "all", MaxPixels: 1000, CNY: .01}}}
	for _, mode := range []string{"per_image", "pixel_token"} {
		t.Run(mode, func(t *testing.T) {
			in := "0.2"
			r := imagepricing.Rule{Operation: "edit", Mode: mode, Currency: "points", Version: "1", Input: imagepricing.Input{FreeImages: 1, PerImage: &in}, OutputTiers: []imagepricing.Tier{{Name: "all", MaxPixels: 1000, Price: "0.3"}}}
			expected := "0.80000"
			if mode == "pixel_token" {
				r.Input.PerImage = nil
				r.OutputTiers = nil
				r.PixelToken = &imagepricing.PixelToken{Definition: "platform_pixels_v1", PixelsPerToken: 4, InputPerMillion: "100000", OutputPerMillion: "100000"}
				expected = "0.60000"
			}
			n := &NativeImagePricingSnapshot{Operation: nativeimage.OperationEdit, RouteImagePricing: &imagepricing.Config{SchemaVersion: 1, Rules: []imagepricing.Rule{r}}, TrustedInputDimensions: []imagepricing.Dimensions{{Width: 3, Height: 1}, {Width: 5, Height: 1}}, InputImages: 2, CNYPerPoint: "0.01", SaleMultiplier: "1", ReservedOutputs: 2, Candidates: []NativeImageCandidatePricing{{AccountModelID: 1, Cost: cost}}}
			snap := PricingSnapshot{NativeImage: n, EstimatedPoints: "10"}
			artifacts := []NativeImageArtifact{{Pixels: 5}, {Pixels: 5}}
			q, err := RouteNativeArtifactQuote(n, artifacts)
			if err != nil {
				t.Fatal(err)
			}
			got, err := SettleNativeImageArtifacts(snap, 1, artifacts)
			if err != nil || got.ActualPoints != expected || got.PricingIncomplete {
				t.Fatalf("quote=%+v settlement=%+v err=%v", q, got, err)
			}
			if mode == "pixel_token" {
				if q.InputTokens != 2 || q.OutputTokens != 4 {
					t.Fatal(q)
				}
				n.TrustedInputDimensions[0] = imagepricing.Dimensions{}
				if _, err := RouteNativeArtifactQuote(n, artifacts); err == nil {
					t.Fatal("missing free input dimensions accepted")
				}
			}
		})
	}
}
