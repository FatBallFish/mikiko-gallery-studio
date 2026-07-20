package billing

import (
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
)

func TestEstimateAppliesAutoBaseResolutionAndMultipliers(t *testing.T) {
	calc := NewCalculator(config.BillingConfig{
		PointsScale:                      5,
		AutoBaseResolutionDefaultByGroup: map[string]string{"basic": "1k", "plus": "2k", "pro": "4k"},
		BaseResolutionPointsByModel:      map[string]map[string]string{"plus": {"1k": "5.00000", "2k": "8.00000", "4k": "16.00000"}},
		UserGroupMultipliers:             map[string]string{"plus": "1.20000"},
		TaskMultipliers:                  map[string]string{"image_edit": "1.25000"},
		ReferenceImageExtra:              config.ReferenceExtra{First: "0.10000", Additional: "0.05000"},
	})
	result, err := calc.Estimate(EstimateRequest{TaskType: "image_edit", AbstractModel: "plus", BaseResolution: "auto", RequestedSize: "1536x1024", RequestedOutputImageCount: 2, ReferenceImageCount: 1, UserGroupCode: "plus"})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}
	if result.BaseResolution != "2k" {
		t.Fatalf("expected 2k, got %s", result.BaseResolution)
	}
	if result.EstimatedPoints != "26.40000" {
		t.Fatalf("expected 26.40000, got %s", result.EstimatedPoints)
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
	if actual != "13.20000" {
		t.Fatalf("expected 13.20000 actual points for single success, got %s", actual)
	}
}

func TestEstimateFailsFastOnInvalidPricingConfig(t *testing.T) {
	calc := NewCalculator(config.BillingConfig{
		PointsScale:                      5,
		AutoBaseResolutionDefaultByGroup: map[string]string{"basic": "1k"},
		BaseResolutionPointsByModel:      map[string]map[string]string{"plus": {"1k": "bad"}},
	})
	if _, err := calc.Estimate(EstimateRequest{TaskType: "text_to_image", AbstractModel: "plus", BaseResolution: "1k", RequestedSize: "1024x1024", RequestedOutputImageCount: 1, UserGroupCode: "basic"}); err == nil {
		t.Fatal("expected invalid pricing config to fail fast")
	}
}

func TestEstimateRejectsRemovedTaskTypes(t *testing.T) {
	calc := NewCalculator(config.BillingConfig{
		AutoBaseResolutionDefaultByGroup: map[string]string{"plus": "1k"},
		BaseResolutionPointsByModel:      map[string]map[string]string{"plus": {"1k": "1.00000"}},
	})
	if _, err := calc.Estimate(EstimateRequest{TaskType: "reference_generate", AbstractModel: "plus", BaseResolution: "1k"}); err == nil {
		t.Fatal("expected removed task type to be rejected")
	}
}
