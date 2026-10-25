package billing

import (
	"fmt"
	"strings"

	"encoding/json"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/internal/domain/imagepricing"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/shopspring/decimal"
)

func (s *Service) ActualNativeArtifactSettlement(snapshot domainbilling.PricingSnapshot, accountModelID int64, artifacts []domainbilling.NativeImageArtifact) (domainbilling.NativeImageSettlement, error) {
	return domainbilling.SettleNativeImageArtifacts(snapshot, accountModelID, artifacts)
}

func (s *Service) ActualNativeSettlement(snapshot domainbilling.PricingSnapshot, accountModelID int64, outputImageCount int, outputPixels []int) (domainbilling.NativeImageSettlement, error) {
	return domainbilling.SettleNativeImage(snapshot, accountModelID, outputImageCount, outputPixels)
}
func estimateNative(req domainbilling.EstimateRequest, resolved modelhub.ResolvedRequest, fxRaw, taskMultiplier string, routePrices ...*imagepricing.Config) (domainbilling.EstimateResult, error) {
	var routePrice *imagepricing.Config
	if len(routePrices) > 0 {
		routePrice = routePrices[0]
	}
	if _, err := imagepricing.RuleForOperation(routePrice, string(modelhub.NativeOperation(req.Operation, req.ReferenceImageCount))); err != nil {
		return domainbilling.EstimateResult{}, errs.New(409, errs.CodeRouteModelPriceMissing, err.Error())
	}
	// Copy the complete rule graph: later admin edits cannot mutate an authorized quote.
	raw, err := json.Marshal(routePrice)
	if err != nil {
		return domainbilling.EstimateResult{}, err
	}
	var frozen imagepricing.Config
	if err := json.Unmarshal(raw, &frozen); err != nil {
		return domainbilling.EstimateResult{}, err
	}
	fx, e := decimal.NewFromString(fxRaw)
	if e != nil || !fx.IsPositive() {
		return domainbilling.EstimateResult{}, fmt.Errorf("native pricing requires positive CNY per point")
	}
	group, e := decimal.NewFromString(resolved.EffectiveMultiplier)
	if e != nil || group.IsNegative() {
		return domainbilling.EstimateResult{}, fmt.Errorf("invalid native group multiplier")
	}
	tm, e := decimal.NewFromString(taskMultiplier)
	if e != nil || tm.IsNegative() {
		return domainbilling.EstimateResult{}, fmt.Errorf("invalid native task multiplier")
	}
	op := modelhub.NativeOperation(req.Operation, req.ReferenceImageCount)
	count := req.RequestedOutputImageCount
	if count <= 0 {
		count = 1
	}
	reserved := count
	if op == nativeimage.OperationLayerDecomposition {
		reserved = 1
		for _, c := range resolved.Providers {
			cfg, err := modelhub.NativeConfig(c)
			if err != nil {
				return domainbilling.EstimateResult{}, err
			}
			if cfg.Profile.MaxLayers < 1 || len(cfg.Profile.LayerSizeModes) == 0 {
				return domainbilling.EstimateResult{}, fmt.Errorf("layer max_layers and size modes required")
			}
			reserved = max(reserved, cfg.Profile.MaxLayers+1)
		}
	}
	n := &domainbilling.NativeImagePricingSnapshot{Operation: op, CNYPerPoint: fx.String(), SaleMultiplier: group.Mul(tm).String(), InputImages: req.ReferenceImageCount, ReservedOutputs: reserved}
	n.RouteImagePricing = &frozen
	n.BaseResolution = resolved.BaseResolution
	n.TrustedInputDimensions = append([]imagepricing.Dimensions(nil), req.TrustedInputDimensions...)
	if len(n.TrustedInputDimensions) != req.ReferenceImageCount {
		n.TrustedInputDimensions = make([]imagepricing.Dimensions, req.ReferenceImageCount)
	}
	maxCost, maxSale := decimal.Zero, decimal.Zero
	for _, c := range resolved.Providers {
		if !modelhub.IsNativeImageCandidate(c) {
			return domainbilling.EstimateResult{}, fmt.Errorf("native pricing cannot fall back to generic image provider")
		}
		cfg, e := modelhub.NativeConfig(c)
		if e != nil {
			return domainbilling.EstimateResult{}, e
		}
		if cfg.Pricing == nil || cfg.Pricing.Model != cfg.Model {
			return domainbilling.EstimateResult{}, fmt.Errorf("native cost pricing must be explicitly configured for actual model")
		}
		pixels := cfg.Profile.MaxPixels
		if w, h, e := nativeimage.ParseDimensions(resolved.ResolvedSize); e == nil {
			pixels = w * h
		} else {
			switch strings.ToUpper(resolved.ResolvedSize) {
			case "1K":
				if cfg.Profile.Adapter == nativeimage.AdapterQwen {
					pixels = 1024 * 1024
				} else {
					pixels = 1280 * 960
				}
			case "1.5K":
				pixels = 1536 * 1024
			case "2K":
				pixels = 2048 * 2048
			}
		}
		if pixels > n.ReservedPixels {
			n.ReservedPixels = pixels
		}
		sizes := make([]int, reserved)
		for i := range sizes {
			sizes[i] = pixels
		}
		cost, e := domainbilling.NativeImageCNY(*cfg.Pricing, op, req.ReferenceImageCount, sizes)
		if e != nil {
			return domainbilling.EstimateResult{}, e
		}
		artifacts := make([]domainbilling.NativeImageArtifact, reserved)
		for i := range artifacts {
			artifacts[i] = domainbilling.NativeImageArtifact{Pixels: pixels, IsBase: op == nativeimage.OperationLayerDecomposition && i == 0}
		}
		quote, e := domainbilling.RouteNativeArtifactQuote(n, artifacts)
		sale := quote.Amount
		if quote.Currency == "points" {
			sale = sale.Mul(fx)
		}
		if e != nil {
			return domainbilling.EstimateResult{}, e
		}
		if cost.GreaterThan(maxCost) {
			maxCost = cost
		}
		if sale.GreaterThan(maxSale) {
			maxSale = sale
		}
		n.Candidates = append(n.Candidates, domainbilling.NativeImageCandidatePricing{AccountModelID: c.AccountModelID, Cost: *cfg.Pricing})
	}
	if len(n.Candidates) == 0 {
		return domainbilling.EstimateResult{}, fmt.Errorf("no native pricing candidates")
	}
	sale := maxSale.Mul(group).Mul(tm)
	points := sale.Div(fx).Round(5)
	n.EstimatedCostCNY = maxCost.StringFixed(5)
	n.EstimatedSaleCNY = sale.StringFixed(5)
	snapshot := domainbilling.PricingSnapshot{NativeImage: n, AbstractModel: resolved.RouteModelCode, RouteModelCode: resolved.RouteModelCode, TaskType: req.TaskType, SizeMode: req.SizeMode, RequestedSize: resolved.ResolvedSize, BaseResolution: resolved.BaseResolution, RequestedOutputImageCount: count, ReferenceImageCount: req.ReferenceImageCount, UserGroupMultiplier: group.String(), TaskMultiplier: tm.String(), BaseUnitPoints: "0.00000", ReferenceExtraMultiplier: "0.00000", EstimatedPoints: points.StringFixed(5)}
	size := resolved.ResolvedSize
	return domainbilling.EstimateResult{CapabilityVersion: resolved.CapabilityVersion, BaseResolution: resolved.BaseResolution, ResolvedSize: &size, EstimatedPoints: snapshot.EstimatedPoints, ChargedPoints: snapshot.EstimatedPoints, DisplayPoints: points.Round(2).StringFixed(2), UserGroupMultiplier: group.String(), RequestedOutputImageCount: count, ReferenceImageCount: req.ReferenceImageCount, PricingSnapshot: snapshot}, nil
}

func estimateRouteImage(req domainbilling.EstimateRequest, resolved modelhub.ResolvedRequest, price *imagepricing.Config, fxRaw, taskMultiplier string) (domainbilling.EstimateResult, error) {
	fx, err := decimal.NewFromString(fxRaw)
	if err != nil || !fx.IsPositive() {
		return domainbilling.EstimateResult{}, fmt.Errorf("positive CNY per point required")
	}
	group, err := decimal.NewFromString(resolved.EffectiveMultiplier)
	if err != nil || group.IsNegative() {
		return domainbilling.EstimateResult{}, fmt.Errorf("invalid group multiplier")
	}
	tm, err := decimal.NewFromString(taskMultiplier)
	if err != nil || tm.IsNegative() {
		return domainbilling.EstimateResult{}, fmt.Errorf("invalid task multiplier")
	}
	raw, err := json.Marshal(price)
	if err != nil {
		return domainbilling.EstimateResult{}, err
	}
	var frozen imagepricing.Config
	if err = json.Unmarshal(raw, &frozen); err != nil {
		return domainbilling.EstimateResult{}, err
	}
	count := req.RequestedOutputImageCount
	if count <= 0 {
		count = 1
	}
	w, h, err := nativeimage.ParseDimensions(resolved.ResolvedSize)
	if err != nil {
		return domainbilling.EstimateResult{}, fmt.Errorf("resolved output dimensions required: %w", err)
	}
	n := &domainbilling.NativeImagePricingSnapshot{Operation: modelhub.NativeOperation(req.Operation, req.ReferenceImageCount), RouteImagePricing: &frozen, TrustedInputDimensions: append([]imagepricing.Dimensions(nil), req.TrustedInputDimensions...), BaseResolution: resolved.BaseResolution, InputImages: req.ReferenceImageCount, ReservedOutputs: count, ReservedPixels: w * h, CNYPerPoint: fx.String(), SaleMultiplier: group.Mul(tm).String()}
	if len(n.TrustedInputDimensions) != req.ReferenceImageCount {
		n.TrustedInputDimensions = make([]imagepricing.Dimensions, req.ReferenceImageCount)
	}
	artifacts := make([]domainbilling.NativeImageArtifact, count)
	for i := range artifacts {
		artifacts[i] = domainbilling.NativeImageArtifact{Width: int64(w), Height: int64(h), Pixels: w * h}
	}
	quote, err := domainbilling.RouteNativeArtifactQuote(n, artifacts)
	if err != nil {
		return domainbilling.EstimateResult{}, errs.New(409, errs.CodeRouteModelPriceMissing, err.Error())
	}
	points := quote.Amount.Mul(group).Mul(tm)
	if quote.Currency == "CNY" {
		points = points.Div(fx)
	}
	points = points.Round(5)
	snap := domainbilling.PricingSnapshot{RouteImage: n, RouteModelCode: resolved.RouteModelCode, AbstractModel: resolved.RouteModelCode, TaskType: req.TaskType, RequestedSize: resolved.ResolvedSize, BaseResolution: resolved.BaseResolution, RequestedOutputImageCount: count, ReferenceImageCount: req.ReferenceImageCount, EstimatedPoints: points.StringFixed(5), UserGroupMultiplier: group.String(), TaskMultiplier: tm.String()}
	size := resolved.ResolvedSize
	return domainbilling.EstimateResult{PricingSnapshot: snap, CapabilityVersion: resolved.CapabilityVersion, ResolvedSize: &size, BaseResolution: resolved.BaseResolution, EstimatedPoints: snap.EstimatedPoints, ChargedPoints: snap.EstimatedPoints, DisplayPoints: points.Round(2).StringFixed(2), UserGroupMultiplier: group.String(), RequestedOutputImageCount: count, ReferenceImageCount: req.ReferenceImageCount}, nil
}
