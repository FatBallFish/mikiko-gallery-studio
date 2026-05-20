package billing

import (
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
)

func TestEstimateAppliesAutoQualityAndMultipliers(t *testing.T) {
	calc := NewCalculator(config.BillingConfig{
		PointsScale:               5,
		AutoQualityDefaultByGroup: map[string]string{"basic": "1k", "plus": "2k", "pro": "4k"},
		QualityPointsByModel:      map[string]map[string]string{"plus": {"1k": "5.00000", "2k": "8.00000", "4k": "16.00000"}},
		UserGroupMultipliers:      map[string]string{"plus": "1.20000"},
		TaskMultipliers:           map[string]string{"reference_generate": "1.15000"},
		ReferenceImageExtra:       config.ReferenceExtra{First: "0.10000", Additional: "0.05000"},
	})
	result, err := calc.Estimate(EstimateRequest{TaskType: "reference_generate", AbstractModel: "plus", RequestedQuality: "auto", RequestedSize: "1536x1024", RequestedOutputImageCount: 2, ReferenceImageCount: 1, UserGroupCode: "plus"})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}
	if result.ResolvedQualityBucket != "2k" {
		t.Fatalf("expected 2k, got %s", result.ResolvedQualityBucket)
	}
	if result.EstimatedPoints != "24.28800" {
		t.Fatalf("expected 24.28800, got %s", result.EstimatedPoints)
	}
	if result.PricingSnapshot.BaseUnitPoints != "8.00000" {
		t.Fatalf("expected snapshot base unit points 8.00000, got %s", result.PricingSnapshot.BaseUnitPoints)
	}
	if result.PricingSnapshot.ReferenceExtraMultiplier != "0.10000" {
		t.Fatalf("expected reference extra multiplier 0.10000, got %s", result.PricingSnapshot.ReferenceExtraMultiplier)
	}
	actual, err := calc.ActualPoints(result.PricingSnapshot, 1)
	if err != nil {
		t.Fatalf("ActualPoints: %v", err)
	}
	if actual != "12.14400" {
		t.Fatalf("expected 12.14400 actual points for single success, got %s", actual)
	}
}

func TestEstimateFailsFastOnInvalidPricingConfig(t *testing.T) {
	calc := NewCalculator(config.BillingConfig{
		PointsScale:               5,
		AutoQualityDefaultByGroup: map[string]string{"basic": "1k"},
		QualityPointsByModel:      map[string]map[string]string{"plus": {"1k": "bad"}},
	})
	if _, err := calc.Estimate(EstimateRequest{TaskType: "text_to_image", AbstractModel: "plus", RequestedQuality: "1k", RequestedSize: "1024x1024", RequestedOutputImageCount: 1, UserGroupCode: "basic"}); err == nil {
		t.Fatal("expected invalid pricing config to fail fast")
	}
}
