package billing

import (
	"strings"

	"github.com/shopspring/decimal"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type Calculator struct {
	cfg      config.BillingConfig
	resolver baseResolutionResolver
}

type baseResolutionResolver interface {
	ResolveBaseResolution(requestedBaseResolution, requestedSize, abstractModel string) (string, error)
}

func NewCalculator(cfg config.BillingConfig) *Calculator {
	return &Calculator{cfg: cfg, resolver: modelhub.NewResolver(config.Config{Billing: cfg})}
}

func NewCalculatorWithResolver(cfg config.BillingConfig, resolver baseResolutionResolver) *Calculator {
	if resolver == nil {
		resolver = modelhub.NewResolver(config.Config{Billing: cfg})
	}
	return &Calculator{cfg: cfg, resolver: resolver}
}

func (c *Calculator) Estimate(req EstimateRequest) (EstimateResult, error) {
	normalized, err := modelhub.NormalizeResolveRequest(modelhub.ResolveRequest{
		SizeMode:       req.SizeMode,
		AspectRatio:    req.AspectRatio,
		BaseResolution: req.BaseResolution,
		RequestedSize:  req.RequestedSize,
	})
	if err != nil {
		return EstimateResult{}, err
	}
	req.SizeMode = modelhub.PublicSizeMode(normalized.SizeMode)
	req.AspectRatio = normalized.AspectRatio
	req.BaseResolution = normalized.BaseResolution
	req.RequestedSize = normalized.RequestedSize
	resolved, err := c.resolver.ResolveBaseResolution(req.BaseResolution, req.RequestedSize, req.AbstractModel)
	if err != nil {
		return EstimateResult{}, err
	}
	if req.RequestedOutputImageCount <= 0 {
		req.RequestedOutputImageCount = 1
	}
	unitStr := c.cfg.BaseResolutionPointsByModel[strings.ToLower(req.AbstractModel)][resolved]
	if unitStr == "" {
		return EstimateResult{}, errs.New(400, errs.CodeImageCapabilityMismatch, "no pricing for requested model and base_resolution")
	}
	unit, err := parseDecimalRequired(unitStr, "base_resolution unit points")
	if err != nil {
		return EstimateResult{}, err
	}
	taskMul, err := parseDecimalWithFallback(c.cfg.TaskMultipliers[req.TaskType], decimal.NewFromInt(1), "task multiplier")
	if err != nil {
		return EstimateResult{}, err
	}
	groupMul, err := parseDecimalWithFallback(req.UserGroupMultiplier, decimal.Zero, "user group multiplier")
	if err != nil {
		return EstimateResult{}, err
	}
	if groupMul.IsZero() {
		groupMul, err = parseDecimalWithFallback(c.cfg.UserGroupMultipliers[strings.ToLower(req.UserGroupCode)], decimal.NewFromInt(1), "configured user group multiplier")
		if err != nil {
			return EstimateResult{}, err
		}
	}
	refExtra := decimal.Zero
	if req.ReferenceImageCount > 0 {
		first, err := parseDecimalWithFallback(c.cfg.ReferenceImageExtra.First, decimal.Zero, "first reference image extra multiplier")
		if err != nil {
			return EstimateResult{}, err
		}
		next, err := parseDecimalWithFallback(c.cfg.ReferenceImageExtra.Additional, decimal.Zero, "additional reference image extra multiplier")
		if err != nil {
			return EstimateResult{}, err
		}
		refExtra = refExtra.Add(first)
		if req.ReferenceImageCount > 1 {
			refExtra = refExtra.Add(next.Mul(decimal.NewFromInt(int64(req.ReferenceImageCount - 1))))
		}
	}
	total := unit.
		Mul(taskMul).
		Mul(decimal.NewFromInt(1).Add(refExtra)).
		Mul(groupMul).
		Mul(decimal.NewFromInt(int64(req.RequestedOutputImageCount))).
		Round(int32(c.cfg.PointsScale))
	snapshot := PricingSnapshot{
		AbstractModel:             req.AbstractModel,
		TaskType:                  req.TaskType,
		SizeMode:                  req.SizeMode,
		AspectRatio:               req.AspectRatio,
		BaseResolution:            resolved,
		Quality:                   req.Quality,
		OutputFormat:              req.OutputFormat,
		OutputCompression:         req.OutputCompression,
		Moderation:                req.Moderation,
		RequestedSize:             req.RequestedSize,
		RequestedOutputImageCount: req.RequestedOutputImageCount,
		ReferenceImageCount:       req.ReferenceImageCount,
		UserGroupCode:             req.UserGroupCode,
		UserGroupMultiplier:       groupMul.StringFixed(int32(c.cfg.PointsScale)),
		BaseUnitPoints:            unit.StringFixed(int32(c.cfg.PointsScale)),
		TaskMultiplier:            taskMul.StringFixed(int32(c.cfg.PointsScale)),
		ReferenceExtraMultiplier:  refExtra.StringFixed(int32(c.cfg.PointsScale)),
		EstimatedPoints:           total.StringFixed(int32(c.cfg.PointsScale)),
	}
	return EstimateResult{
		BaseResolution:            resolved,
		EstimatedPoints:           snapshot.EstimatedPoints,
		UserGroupMultiplier:       snapshot.UserGroupMultiplier,
		RequestedOutputImageCount: req.RequestedOutputImageCount,
		ReferenceImageCount:       req.ReferenceImageCount,
		PricingSnapshot:           snapshot,
	}, nil
}

func parseDecimalRequired(value, label string) (decimal.Decimal, error) {
	parsed, err := decimal.NewFromString(strings.TrimSpace(value))
	if err != nil {
		return decimal.Zero, errs.Internal("invalid billing config for " + label)
	}
	return parsed, nil
}

func parseDecimalWithFallback(value string, fallback decimal.Decimal, label string) (decimal.Decimal, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := decimal.NewFromString(strings.TrimSpace(value))
	if err != nil {
		return decimal.Zero, errs.Internal("invalid billing config for " + label)
	}
	return parsed, nil
}

func (c *Calculator) ActualPoints(snapshot PricingSnapshot, successOutputImageCount int) (string, error) {
	if successOutputImageCount <= 0 {
		return decimal.Zero.StringFixed(int32(c.cfg.PointsScale)), nil
	}
	unit, err := decimal.NewFromString(snapshot.BaseUnitPoints)
	if err != nil {
		return "", err
	}
	taskMultiplier, err := decimal.NewFromString(snapshot.TaskMultiplier)
	if err != nil {
		return "", err
	}
	refExtra, err := decimal.NewFromString(snapshot.ReferenceExtraMultiplier)
	if err != nil {
		return "", err
	}
	groupMultiplier, err := decimal.NewFromString(snapshot.UserGroupMultiplier)
	if err != nil {
		return "", err
	}
	actual := unit.
		Mul(taskMultiplier).
		Mul(decimal.NewFromInt(1).Add(refExtra)).
		Mul(groupMultiplier).
		Mul(decimal.NewFromInt(int64(successOutputImageCount))).
		Round(int32(c.cfg.PointsScale))
	return actual.StringFixed(int32(c.cfg.PointsScale)), nil
}
