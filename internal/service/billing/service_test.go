package billing

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type staticRoutingSource struct {
	snapshot modelhub.ModelRoutingSnapshot
}

func (s staticRoutingSource) ModelRoutingConfig(context.Context) (modelhub.ModelRoutingSnapshot, error) {
	return s.snapshot, nil
}

func TestReserveFinalizeAndLedger(t *testing.T) {
	svc := NewService(config.BillingConfig{
		CNYPerPoint: "0.31250",
		PointsScale: 5,
	})
	if _, err := svc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{
		UserID:       101,
		ChangePoints: "100.00000",
		Reason:       "seed balance",
	}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}
	if _, err := svc.ReserveTask(context.Background(), domainbilling.ReserveRequest{
		UserID:          101,
		TaskID:          "task-1",
		EstimatedPoints: "12.00000",
		Reason:          "reserve task-1",
	}); err != nil {
		t.Fatalf("ReserveTask: %v", err)
	}
	if _, err := svc.FinalizeTask(context.Background(), domainbilling.FinalizeRequest{
		UserID:          101,
		TaskID:          "task-1",
		EstimatedPoints: "12.00000",
		ActualPoints:    "8.00000",
		Reason:          "finalize task-1",
	}); err != nil {
		t.Fatalf("FinalizeTask: %v", err)
	}
	summary, err := svc.GetBalance(context.Background(), 101, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if summary.AvailablePoints != "92.00000" || summary.FrozenPoints != "0.00000" {
		t.Fatalf("unexpected summary %#v", summary)
	}
	page, err := svc.ListLedger(context.Background(), 101, 1, 10)
	if err != nil {
		t.Fatalf("ListLedger: %v", err)
	}
	if len(page.Items) != 4 {
		t.Fatalf("expected 4 ledger entries, got %d", len(page.Items))
	}
	if page.Items[0].LedgerType != "refund" || page.Items[1].LedgerType != "consume" || page.Items[2].LedgerType != "reserve" || page.Items[3].LedgerType != "admin_adjust" {
		t.Fatalf("unexpected ledger order %#v", page.Items)
	}
	if page.Items[1].ChangePoints != "-8.00000" {
		t.Fatalf("expected consume ledger to record actual spend, got %#v", page.Items[1])
	}
}

func TestReserveTaskRejectsInsufficientBalance(t *testing.T) {
	svc := NewService(config.BillingConfig{CNYPerPoint: "0.31250", PointsScale: 5})
	if _, err := svc.ReserveTask(context.Background(), domainbilling.ReserveRequest{
		UserID:          201,
		TaskID:          "task-insufficient",
		EstimatedPoints: "1.00000",
		Reason:          "reserve",
	}); err == nil {
		t.Fatal("expected insufficient balance error")
	}
}

func TestMemoryStoreAPIKeyQuotaConcurrentReserveIsAtomic(t *testing.T) {
	svc := NewService(config.BillingConfig{CNYPerPoint: "0.31250", PointsScale: 5})
	if _, err := svc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{
		UserID:       202,
		ChangePoints: "100.00000",
		Reason:       "seed balance",
	}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}

	totalQuota := "16.00000"
	dailyQuota := "16.00000"
	dayStart := time.Now()
	const workers = 8
	start := make(chan struct{})
	errCh := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := svc.ReserveTask(context.Background(), domainbilling.ReserveRequest{
				UserID:          202,
				APIKeyID:        7001,
				TaskID:          "memory-quota-task-" + string(rune('a'+i)),
				EstimatedPoints: "8.00000",
				Reason:          "reserve with api key quota",
				APIKeyQuota: domainbilling.APIKeyQuota{
					APIKeyTotalQuotaPoints: &totalQuota,
					APIKeyDailyQuotaPoints: &dailyQuota,
					APIKeyQuotaDayStart:    &dayStart,
				},
			})
			errCh <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(errCh)

	successes := 0
	rateLimited := 0
	for err := range errCh {
		if err == nil {
			successes++
			continue
		}
		appErr, ok := err.(*errs.Error)
		if !ok || appErr.StatusCode != 429 || appErr.Code != errs.CodeRateLimited {
			t.Fatalf("expected only quota 429 errors, got %T %v", err, err)
		}
		rateLimited++
	}
	if successes != 2 || rateLimited != workers-2 {
		t.Fatalf("expected exactly 2 successes and %d quota failures, got successes=%d failures=%d", workers-2, successes, rateLimited)
	}
	summary, err := svc.GetBalance(context.Background(), 202, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if summary.AvailablePoints != "84.00000" || summary.FrozenPoints != "16.00000" {
		t.Fatalf("expected only quota-covered reserves to affect balance, got %#v", summary)
	}
	totalUsed, err := svc.APIKeyUsage(context.Background(), 7001, nil)
	if err != nil {
		t.Fatalf("APIKeyUsage total: %v", err)
	}
	dailyUsed, err := svc.APIKeyUsage(context.Background(), 7001, &dayStart)
	if err != nil {
		t.Fatalf("APIKeyUsage daily: %v", err)
	}
	if totalUsed != "16.00000" || dailyUsed != "16.00000" {
		t.Fatalf("expected usage to stop at quota, total=%s daily=%s", totalUsed, dailyUsed)
	}
}

func TestFinalizeTaskRejectsNegativeEstimate(t *testing.T) {
	svc := NewService(config.BillingConfig{CNYPerPoint: "0.31250", PointsScale: 5})
	if _, err := svc.FinalizeTask(context.Background(), domainbilling.FinalizeRequest{
		UserID:          301,
		TaskID:          "task-negative",
		EstimatedPoints: "-1.00000",
		ActualPoints:    "0.00000",
		Reason:          "finalize",
	}); err == nil {
		t.Fatal("expected negative estimate to be rejected")
	}
}

func TestFinalizeTaskRejectsWithoutReserve(t *testing.T) {
	svc := NewService(config.BillingConfig{CNYPerPoint: "0.31250", PointsScale: 5})
	if _, err := svc.FinalizeTask(context.Background(), domainbilling.FinalizeRequest{
		UserID:          302,
		TaskID:          "task-no-reserve",
		EstimatedPoints: "1.00000",
		ActualPoints:    "1.00000",
		Reason:          "finalize",
	}); err == nil {
		t.Fatal("expected finalize without reserve to be rejected")
	} else {
		appErr, ok := err.(*errs.Error)
		if !ok || appErr.Code != errs.CodeConflict {
			t.Fatalf("expected conflict error, got %T %v", err, err)
		}
	}
}

func TestFinalizeTaskRejectsDifferentUserForReservedTask(t *testing.T) {
	svc := NewService(config.BillingConfig{CNYPerPoint: "0.31250", PointsScale: 5})
	if _, err := svc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{
		UserID:       401,
		ChangePoints: "10.00000",
		Reason:       "seed owner",
	}); err != nil {
		t.Fatalf("AdminAdjust owner: %v", err)
	}
	if _, err := svc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{
		UserID:       402,
		ChangePoints: "10.00000",
		Reason:       "seed intruder",
	}); err != nil {
		t.Fatalf("AdminAdjust intruder: %v", err)
	}
	if _, err := svc.ReserveTask(context.Background(), domainbilling.ReserveRequest{
		UserID:          401,
		TaskID:          "task-owner-only",
		EstimatedPoints: "4.00000",
		Reason:          "reserve",
	}); err != nil {
		t.Fatalf("ReserveTask: %v", err)
	}

	if _, err := svc.FinalizeTask(context.Background(), domainbilling.FinalizeRequest{
		UserID:          402,
		TaskID:          "task-owner-only",
		EstimatedPoints: "4.00000",
		ActualPoints:    "4.00000",
		Reason:          "finalize wrong user",
	}); err == nil {
		t.Fatal("expected finalize to reject wrong user")
	} else {
		appErr, ok := err.(*errs.Error)
		if !ok || appErr.Code != errs.CodeConflict {
			t.Fatalf("expected conflict error, got %T %v", err, err)
		}
	}

	ownerBalance, err := svc.GetBalance(context.Background(), 401, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance owner: %v", err)
	}
	if ownerBalance.AvailablePoints != "6.00000" || ownerBalance.FrozenPoints != "4.00000" {
		t.Fatalf("expected owner reserve to remain intact, got %#v", ownerBalance)
	}

	intruderBalance, err := svc.GetBalance(context.Background(), 402, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance intruder: %v", err)
	}
	if intruderBalance.AvailablePoints != "10.00000" || intruderBalance.FrozenPoints != "0.00000" {
		t.Fatalf("expected intruder balance unchanged, got %#v", intruderBalance)
	}
}

func TestFinalizeTaskUsesReservedAmountInsteadOfCallerEstimate(t *testing.T) {
	svc := NewService(config.BillingConfig{CNYPerPoint: "0.31250", PointsScale: 5})
	if _, err := svc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{
		UserID:       501,
		ChangePoints: "10.00000",
		Reason:       "seed owner",
	}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}
	if _, err := svc.ReserveTask(context.Background(), domainbilling.ReserveRequest{
		UserID:          501,
		TaskID:          "task-reserve-authoritative",
		EstimatedPoints: "6.00000",
		Reason:          "reserve",
	}); err != nil {
		t.Fatalf("ReserveTask: %v", err)
	}
	if _, err := svc.FinalizeTask(context.Background(), domainbilling.FinalizeRequest{
		UserID:          501,
		TaskID:          "task-reserve-authoritative",
		EstimatedPoints: "4.00000",
		ActualPoints:    "4.00000",
		Reason:          "finalize mismatched estimate",
	}); err != nil {
		t.Fatalf("FinalizeTask: %v", err)
	}

	summary, err := svc.GetBalance(context.Background(), 501, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if summary.AvailablePoints != "6.00000" || summary.FrozenPoints != "0.00000" {
		t.Fatalf("expected finalize to settle full reserve, got %#v", summary)
	}

	page, err := svc.ListLedger(context.Background(), 501, 1, 10)
	if err != nil {
		t.Fatalf("ListLedger: %v", err)
	}
	if len(page.Items) != 4 || page.Items[0].LedgerType != "refund" || page.Items[0].ChangePoints != "2.00000" || page.Items[1].LedgerType != "consume" || page.Items[1].ChangePoints != "-4.00000" {
		t.Fatalf("unexpected ledger settlement %#v", page.Items)
	}
}

func TestNewServiceDefaultsPointsScaleToFiveDecimals(t *testing.T) {
	svc := NewService(config.BillingConfig{
		CNYPerPoint:               "0.31250",
		PointsScale:               2,
		QualityPointsByModel:      map[string]map[string]string{"plus": {"2k": "8"}},
		TaskMultipliers:           map[string]string{"text_to_image": "1"},
		UserGroupMultipliers:      map[string]string{"basic": "1"},
		AutoQualityDefaultByGroup: map[string]string{"plus": "2k"},
	})

	result, err := svc.Estimate(domainbilling.EstimateRequest{
		TaskType:                  "text_to_image",
		AbstractModel:             "plus",
		RequestedQuality:          "auto",
		RequestedOutputImageCount: 1,
		UserGroupCode:             "basic",
	})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}
	if result.EstimatedPoints != "8.00000" || result.UserGroupMultiplier != "1.00000" {
		t.Fatalf("expected normalized 5-decimal billing output, got %#v", result)
	}

	actual, err := svc.ActualPoints(result.PricingSnapshot, 1)
	if err != nil {
		t.Fatalf("ActualPoints: %v", err)
	}
	if actual != "8.00000" {
		t.Fatalf("expected normalized 5-decimal actual points, got %q", actual)
	}
}

func TestEstimateRouteModelAutoQualityUsesExplicitSize(t *testing.T) {
	svc := NewService(config.BillingConfig{
		CNYPerPoint:               "0.31250",
		PointsScale:               5,
		TaskMultipliers:           map[string]string{"text_to_image": "1.00000"},
		AutoQualityDefaultByGroup: map[string]string{"plus": "4k"},
	})
	svc.SetModelRoutingSource(staticRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices: []modelhub.RoutePriceConfig{
			{RouteModelID: 1, TaskType: "text_to_image", Quality: "1k", BasePoints: "2.00000", Enabled: true},
			{RouteModelID: 1, TaskType: "text_to_image", Quality: "2k", BasePoints: "4.00000", Enabled: true},
			{RouteModelID: 1, TaskType: "text_to_image", Quality: "4k", BasePoints: "8.00000", Enabled: true},
		},
		ProviderModels: []modelhub.ProviderCandidate{
			{AccountModelID: 12, ModelAccountID: 102, ModelCode: "gpt-image-1", SupportedTaskTypes: []string{"text_to_image"}, SupportedQualities: []string{"2k"}},
		},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Priority: 1, Enabled: true}},
	}})

	result, err := svc.Estimate(domainbilling.EstimateRequest{
		TaskType:                  "text_to_image",
		RouteModelCode:            "plus",
		RequestedQuality:          "auto",
		RequestedSize:             "1536x1024",
		RequestedOutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}
	if result.ResolvedQualityBucket != "2k" {
		t.Fatalf("expected route billing to resolve 2k from explicit size, got %s", result.ResolvedQualityBucket)
	}
	if result.EstimatedPoints != "4.00000" {
		t.Fatalf("expected 2k price, got %#v", result)
	}
}

func TestEstimateRouteModelRejectsWhenNoCandidateSupportsResolvedQuality(t *testing.T) {
	svc := NewService(config.BillingConfig{
		CNYPerPoint:               "0.31250",
		PointsScale:               5,
		TaskMultipliers:           map[string]string{"text_to_image": "1.00000"},
		AutoQualityDefaultByGroup: map[string]string{"plus": "2k"},
	})
	svc.SetModelRoutingSource(staticRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices:      []modelhub.RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", Quality: "2k", BasePoints: "4.00000", Enabled: true}},
		ProviderModels: []modelhub.ProviderCandidate{
			{AccountModelID: 12, ModelAccountID: 102, ModelCode: "gpt-image-1", SupportedTaskTypes: []string{"text_to_image"}, SupportedQualities: []string{"1k"}},
		},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Priority: 1, Enabled: true}},
	}})

	_, err := svc.Estimate(domainbilling.EstimateRequest{
		TaskType:                  "text_to_image",
		RouteModelCode:            "plus",
		RequestedQuality:          "auto",
		RequestedOutputImageCount: 1,
	})
	appErr, ok := err.(*errs.Error)
	if !ok || appErr.StatusCode != 409 || appErr.Code != errs.CodeConflict {
		t.Fatalf("expected estimate to reject route model without matching candidate, got %#v", err)
	}
}

func TestEstimateRouteModelRejectsInvisibleGroupBeforePricing(t *testing.T) {
	svc := NewService(config.BillingConfig{
		CNYPerPoint:     "0.31250",
		PointsScale:     5,
		TaskMultipliers: map[string]string{"text_to_image": "1.00000"},
	})
	svc.SetModelRoutingSource(staticRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "staff", Name: "Staff", Visibility: "groups", Enabled: true}},
		Groups:      []modelhub.UserGroupConfig{{ID: 10, Code: "staff", Multiplier: "0.50000", Status: "enabled"}},
		Visibility:  []modelhub.RouteVisibilityConfig{{RouteModelID: 1, GroupID: 10}},
		ProviderModels: []modelhub.ProviderCandidate{
			{AccountModelID: 12, ModelAccountID: 102, ModelCode: "gpt-image-1", SupportedTaskTypes: []string{"text_to_image"}, SupportedQualities: []string{"1k"}},
		},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Priority: 1, Enabled: true}},
	}})

	_, err := svc.Estimate(domainbilling.EstimateRequest{
		TaskType:                  "text_to_image",
		RouteModelCode:            "staff",
		RequestedQuality:          "1k",
		RequestedOutputImageCount: 1,
	})
	appErr, ok := err.(*errs.Error)
	if !ok || appErr.StatusCode != 403 {
		t.Fatalf("expected invisible group model to return 403 before pricing, got %#v", err)
	}
}

func TestGetBalanceNormalizesDecimalMetadata(t *testing.T) {
	svc := NewService(config.BillingConfig{CNYPerPoint: "0.3"})
	summary, err := svc.GetBalance(context.Background(), 601, "1.2")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if summary.UserGroupMultiplier != "1.20000" || summary.CNYPerPoint != "0.30000" {
		t.Fatalf("expected normalized balance metadata, got %#v", summary)
	}
}
