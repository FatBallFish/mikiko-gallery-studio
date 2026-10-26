package billing

import (
	"fmt"
	"math"

	"github.com/fatballfish/pic-gallery/internal/domain/imagepricing"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"github.com/shopspring/decimal"
)

type NativeImageCandidatePricing struct {
	AccountModelID int64               `json:"account_model_id"`
	Cost           nativeimage.Pricing `json:"cost"`
	Sale           nativeimage.Pricing `json:"sale"`
}
type NativeImagePricingSnapshot struct {
	RouteImagePricing      *imagepricing.Config          `json:"route_image_pricing,omitempty"`
	TrustedInputDimensions []imagepricing.Dimensions     `json:"trusted_input_dimensions,omitempty"`
	BaseResolution         string                        `json:"base_resolution,omitempty"`
	Operation              nativeimage.Operation         `json:"operation"`
	CNYPerPoint            string                        `json:"cny_per_point"`
	SaleMultiplier         string                        `json:"sale_multiplier"`
	InputImages            int                           `json:"input_images"`
	ReservedOutputs        int                           `json:"reserved_outputs"`
	ReservedPixels         int                           `json:"reserved_pixels"`
	Candidates             []NativeImageCandidatePricing `json:"candidates"`
	EstimatedCostCNY       string                        `json:"estimated_cost_cny"`
	EstimatedSaleCNY       string                        `json:"estimated_sale_cny"`
}
type NativeImageArtifact struct {
	Width  int64 `json:"width,omitempty"`
	Height int64 `json:"height,omitempty"`
	Pixels int   `json:"pixels"`
	IsBase bool  `json:"is_base"`
}

type NativeImageSettlement struct {
	Terminal            bool   `json:"terminal"`
	PricingIncomplete   bool   `json:"pricing_incomplete,omitempty"`
	Diagnostic          string `json:"diagnostic,omitempty"`
	ReservationExceeded bool   `json:"reservation_exceeded,omitempty"`
	UnchargedPoints     string `json:"uncharged_points,omitempty"`
	ActualPoints        string `json:"actual_points"`
	CostCNY             string `json:"cost_cny"`
	SaleCNY             string `json:"sale_cny"`    // After group/task multipliers, before reservation cap.
	RevenueCNY          string `json:"revenue_cny"` // Actual charged points times the frozen CNY per point.
}

func nativeRate(v float64) (decimal.Decimal, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return decimal.Zero, fmt.Errorf("invalid native image rate")
	}
	return decimal.NewFromFloat(v), nil
}

// NativeImageCNY preserves the complete-set contract: a decomposition starts with its base.
func NativeImageCNY(p nativeimage.Pricing, op nativeimage.Operation, inputs int, pixels []int) (decimal.Decimal, error) {
	artifacts := make([]NativeImageArtifact, len(pixels))
	for i, pixel := range pixels {
		artifacts[i] = NativeImageArtifact{Pixels: pixel, IsBase: op == nativeimage.OperationLayerDecomposition && i == 0}
	}
	return NativeImageArtifactCNY(p, op, inputs, artifacts)
}

// NativeImageArtifactCNY charges only the base artifacts explicitly present.
func NativeImageArtifactCNY(p nativeimage.Pricing, op nativeimage.Operation, inputs int, artifacts []NativeImageArtifact) (decimal.Decimal, error) {
	if p.Currency != "CNY" || p.Version == "" || inputs < 0 || p.FreeInputImages < 0 {
		return decimal.Zero, fmt.Errorf("invalid native pricing currency/version/input")
	}
	if len(artifacts) == 0 {
		return decimal.Zero, nil
	}
	in, err := nativeRate(p.InputCNY)
	if err != nil {
		return decimal.Zero, err
	}
	n := inputs - p.FreeInputImages
	if n < 0 {
		n = 0
	}
	total := in.Mul(decimal.NewFromInt(int64(n)))
	tiers := p.OutputTiers
	if op == nativeimage.OperationLayerDecomposition {
		if !p.LayerPricingConfirmed || p.LayerBaseCNY == nil {
			return decimal.Zero, fmt.Errorf("layer pricing is unconfirmed")
		}
		tiers = p.LayerTiers
		base, e := nativeRate(*p.LayerBaseCNY)
		if e != nil {
			return decimal.Zero, e
		}
		baseCount := 0
		for _, a := range artifacts {
			if a.IsBase {
				baseCount++
			}
		}
		if baseCount > 1 {
			return decimal.Zero, fmt.Errorf("multiple base artifacts")
		}
		if baseCount == 1 {
			total = total.Add(base)
		}
	}
	previous := 0
	for _, t := range tiers {
		if t.MaxPixels <= previous {
			return decimal.Zero, fmt.Errorf("native price tiers must have increasing pixel limits")
		}
		if _, e := nativeRate(t.CNY); e != nil {
			return decimal.Zero, e
		}
		previous = t.MaxPixels
	}
	if len(tiers) == 0 {
		return decimal.Zero, fmt.Errorf("native price tiers are missing")
	}
	for _, artifact := range artifacts {
		pixel := artifact.Pixels
		if pixel <= 0 {
			return decimal.Zero, fmt.Errorf("output pixel count is required")
		}
		if op == nativeimage.OperationLayerDecomposition && artifact.IsBase {
			continue
		}
		found := false
		for _, t := range tiers {
			if pixel <= t.MaxPixels {
				rate, e := nativeRate(t.CNY)
				if e != nil {
					return decimal.Zero, e
				}
				total = total.Add(rate)
				found = true
				break
			}
		}
		if !found {
			return decimal.Zero, fmt.Errorf("output exceeds configured pricing tiers")
		}
	}
	return total, nil
}

// SettleNativeImage retains the legacy complete-set convention (base first).
// Partial decompositions must use SettleNativeImageArtifacts with explicit roles.
func SettleNativeImage(snapshot PricingSnapshot, accountModelID int64, outputs int, pixels []int) (NativeImageSettlement, error) {
	zero := NativeImageSettlement{Terminal: true, ActualPoints: "0.00000", CostCNY: "0.00000", SaleCNY: "0.00000", RevenueCNY: "0.00000"}
	s := snapshot.NativeImage
	if s == nil {
		return zero, fmt.Errorf("native image snapshot missing")
	}
	if outputs < 0 {
		return zero, fmt.Errorf("negative actual output count")
	}
	if outputs == 0 {
		return zero, nil
	}
	if len(pixels) == 0 {
		pixels = make([]int, outputs)
		for i := range pixels {
			pixels[i] = s.ReservedPixels
		}
	}
	if len(pixels) != outputs {
		return zero, fmt.Errorf("output pixel count does not match outputs")
	}
	artifacts := make([]NativeImageArtifact, outputs)
	for i, pixel := range pixels {
		artifacts[i] = NativeImageArtifact{Pixels: pixel, IsBase: s.Operation == nativeimage.OperationLayerDecomposition && i == 0}
	}
	return SettleNativeImageArtifacts(snapshot, accountModelID, artifacts)
}

// SettleNativeImageArtifacts is terminal for valid snapshots even if runtime artifact
// dimensions cannot be priced. In that case it charges the authorized reservation
// and exposes missing monetary values rather than manufacturing zero costs.
func SettleNativeImageArtifacts(snapshot PricingSnapshot, accountModelID int64, artifacts []NativeImageArtifact) (NativeImageSettlement, error) {
	zero := NativeImageSettlement{Terminal: true, ActualPoints: "0.00000", CostCNY: "0.00000", SaleCNY: "0.00000", RevenueCNY: "0.00000"}
	s := snapshot.NativeImage
	if s == nil {
		return zero, fmt.Errorf("native image snapshot missing")
	}
	outputs := len(artifacts)
	if outputs == 0 {
		return zero, nil
	}
	fx, e := decimal.NewFromString(s.CNYPerPoint)
	if e != nil || !fx.IsPositive() {
		return zero, fmt.Errorf("invalid frozen CNY per point")
	}
	mul, e := decimal.NewFromString(s.SaleMultiplier)
	if e != nil || mul.IsNegative() {
		return zero, fmt.Errorf("invalid frozen sale multiplier")
	}
	for _, c := range s.Candidates {
		if c.AccountModelID != accountModelID {
			continue
		}
		cost, costErr := NativeImageArtifactCNY(c.Cost, s.Operation, s.InputImages, artifacts)
		sale := decimal.Zero
		var saleErr error
		if s.RouteImagePricing == nil {
			sale, saleErr = NativeImageArtifactCNY(c.Sale, s.Operation, s.InputImages, artifacts)
		}
		if s.RouteImagePricing != nil {
			var quote imagepricing.Quote
			quote, saleErr = RouteNativeArtifactQuote(s, artifacts)
			sale = quote.Amount
			if quote.Currency == "points" {
				sale = sale.Mul(fx)
			}
		}
		if costErr != nil || saleErr != nil {
			reserved, err := decimal.NewFromString(snapshot.EstimatedPoints)
			if err != nil || reserved.IsNegative() {
				return zero, fmt.Errorf("invalid frozen reservation")
			}
			result := NativeImageSettlement{Terminal: true, PricingIncomplete: true, Diagnostic: fmt.Sprintf("native pricing incomplete: cost=%v sale=%v; authorized reservation charged", costErr, saleErr), ActualPoints: reserved.StringFixed(5), RevenueCNY: reserved.Mul(fx).StringFixed(5)}
			if costErr == nil {
				result.CostCNY = cost.StringFixed(5)
			}
			if saleErr == nil {
				result.SaleCNY = sale.Mul(mul).StringFixed(5)
			}
			return result, nil
		}
		sale = sale.Mul(mul)
		points := sale.Div(fx).Round(5)
		reserved, e := decimal.NewFromString(snapshot.EstimatedPoints)
		if e != nil || reserved.IsNegative() {
			return zero, fmt.Errorf("invalid frozen reservation")
		}
		result := NativeImageSettlement{Terminal: true, ActualPoints: points.StringFixed(5), CostCNY: cost.StringFixed(5), SaleCNY: sale.StringFixed(5), ReservationExceeded: outputs > s.ReservedOutputs}
		if points.GreaterThan(reserved) {
			result.ReservationExceeded = true
			result.UnchargedPoints = points.Sub(reserved).StringFixed(5)
			result.ActualPoints = reserved.StringFixed(5)
		}
		charged, _ := decimal.NewFromString(result.ActualPoints)
		result.RevenueCNY = charged.Mul(fx).StringFixed(5)
		return result, nil
	}
	return zero, fmt.Errorf("account model absent from frozen native pricing")
}

// RouteNativeArtifactQuote prices one task as a whole so input allowances apply once.
func RouteNativeArtifactQuote(s *NativeImagePricingSnapshot, artifacts []NativeImageArtifact) (imagepricing.Quote, error) {
	inputs := append([]imagepricing.Dimensions(nil), s.TrustedInputDimensions...)
	if len(inputs) != s.InputImages {
		return imagepricing.Quote{}, fmt.Errorf("frozen trusted input dimensions count mismatch")
	}
	req := imagepricing.Request{Operation: string(s.Operation), Inputs: inputs, BaseResolution: s.BaseResolution}
	baseCount := 0
	for _, a := range artifacts {
		d := imagepricing.Dimensions{Width: a.Width, Height: a.Height}
		if d.Width <= 0 || d.Height <= 0 {
			d = imagepricing.Dimensions{Width: int64(a.Pixels), Height: 1}
		}
		if s.Operation == nativeimage.OperationLayerDecomposition && !a.IsBase {
			req.Layers = append(req.Layers, d)
		} else {
			req.Outputs = append(req.Outputs, d)
			baseCount++
		}
	}
	if s.Operation == nativeimage.OperationLayerDecomposition {
		if baseCount > 1 {
			return imagepricing.Quote{}, fmt.Errorf("multiple base artifacts")
		}
		if baseCount == 0 {
			// A partial result may contain only layers. Price those real artifacts
			// with the frozen layer tiers and input allowance; no base was delivered.
			rule, err := imagepricing.RuleForOperation(s.RouteImagePricing, string(s.Operation))
			if err != nil {
				return imagepricing.Quote{}, err
			}
			if rule.Layers == nil || !rule.Layers.Confirmed || rule.Layers.BasePrice == nil {
				return imagepricing.Quote{}, fmt.Errorf("confirmed layer sale pricing required")
			}
			rule.Operation = "edit"
			rule.OutputTiers = rule.Layers.Tiers
			rule.Layers = nil
			req.Operation = "edit"
			req.Outputs = req.Layers
			req.Layers = nil
			return imagepricing.Calculate(&imagepricing.Config{SchemaVersion: 1, Rules: []imagepricing.Rule{rule}}, req)
		}
	}
	return imagepricing.Calculate(s.RouteImagePricing, req)
}

func RouteImageActualPoints(snapshot PricingSnapshot, artifacts []NativeImageArtifact) (string, error) {
	if len(artifacts) == 0 {
		return "0.00000", nil
	}
	n := snapshot.RouteImage
	if n == nil {
		return "", fmt.Errorf("route image snapshot missing")
	}
	q, err := RouteNativeArtifactQuote(n, artifacts)
	if err != nil {
		return "", err
	}
	fx, err := decimal.NewFromString(n.CNYPerPoint)
	if err != nil || !fx.IsPositive() {
		return "", fmt.Errorf("invalid frozen FX")
	}
	mul, err := decimal.NewFromString(n.SaleMultiplier)
	if err != nil || mul.IsNegative() {
		return "", fmt.Errorf("invalid frozen multiplier")
	}
	points := q.Amount.Mul(mul)
	if q.Currency == "CNY" {
		points = points.Div(fx)
	}
	reserved, err := decimal.NewFromString(snapshot.EstimatedPoints)
	if err != nil || reserved.IsNegative() {
		return "", fmt.Errorf("invalid reservation")
	}
	return decimal.Min(points.Round(5), reserved).StringFixed(5), nil
}
