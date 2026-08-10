package billing

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type staticRoutingSource struct {
	snapshot modelhub.ModelRoutingSnapshot
}

func (s staticRoutingSource) ModelRoutingConfig(context.Context) (modelhub.ModelRoutingSnapshot, error) {
	return s.snapshot, nil
}

type countingRoutingSource struct {
	snapshot modelhub.ModelRoutingSnapshot
	reads    int
}

type contextObservingRoutingSource struct {
	err error
}

type auditedPlanStore struct {
	Store
	request domainbilling.TransitionSubscriptionPlanRequest
	audit   domainbilling.PlanLifecycleAudit
}

func (s *auditedPlanStore) TransitionPlanAudited(_ context.Context, req domainbilling.TransitionSubscriptionPlanRequest, audit domainbilling.PlanLifecycleAudit) (domainbilling.SubscriptionPlan, error) {
	s.request = req
	s.audit = audit
	return domainbilling.SubscriptionPlan{ID: req.PlanID, Status: domainbilling.SubscriptionPlanStatusActive}, nil
}

func (s *contextObservingRoutingSource) ModelRoutingConfig(ctx context.Context) (modelhub.ModelRoutingSnapshot, error) {
	s.err = ctx.Err()
	return modelhub.ModelRoutingSnapshot{}, ctx.Err()
}

func (s *countingRoutingSource) ModelRoutingConfig(context.Context) (modelhub.ModelRoutingSnapshot, error) {
	s.reads++
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

func TestMarkOrderPaidRejectsCallbackFromDifferentProviderInstance(t *testing.T) {
	svc := NewService(config.BillingConfig{CNYPerPoint: "0.31250", PointsScale: 5})
	order, err := svc.CreateCustomAmountOrder(t.Context(), domainbilling.CreateCustomAmountOrderRequest{
		UserID:             901,
		OrderNo:            "PGO-CALLBACK-BINDING-MEMORY",
		AmountCNY:          "10.00",
		CNYPerPoint:        "0.31250",
		Provider:           "jeepay_alipay",
		PurchaseType:       "custom_amount",
		VisibleMethod:      "alipay",
		ProviderType:       "jeepay_alipay",
		ProviderInstanceID: 41,
	})
	if err != nil {
		t.Fatalf("CreateCustomAmountOrder: %v", err)
	}

	if _, err := svc.MarkOrderPaid(t.Context(), domainbilling.MarkOrderPaidRequest{
		Provider:           "jeepay_alipay",
		ProviderInstanceID: 42,
		OrderNo:            order.OrderNo,
		TradeNo:            "JEEPAY-CROSS-INSTANCE",
		AmountCNY:          order.AmountCNY,
	}); err == nil {
		t.Fatal("expected callback from a different provider instance to be rejected")
	}
	reloaded, err := svc.GetOrder(t.Context(), order.UserID, order.ID)
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if reloaded.Status != "pending" || reloaded.TradeNo != "" {
		t.Fatalf("cross-instance callback must not mutate order: %#v", reloaded)
	}
}

func TestPaymentOrdersSnapshotExpiryAndExpirePendingIdempotently(t *testing.T) {
	ctx := t.Context()
	svc := NewService(config.BillingConfig{CNYPerPoint: "0.31250", PointsScale: 5})
	before := time.Now().UTC()
	defaultOrder, err := svc.CreateOrder(ctx, domainbilling.CreateOrderRequest{
		UserID: 9021, OrderNo: "PGO-DEFAULT-EXPIRY", PlanCode: "basic-monthly", Provider: "mock",
	})
	if err != nil {
		t.Fatal(err)
	}
	defaultTTL := defaultOrder.ExpiresAt.Sub(before)
	if defaultTTL < 899*time.Second || defaultTTL > 901*time.Second {
		t.Fatalf("default order ttl=%s, want 900s", defaultTTL)
	}

	explicitExpiry := time.Now().UTC().Add(2 * time.Minute)
	explicitOrder, err := svc.CreateCustomAmountOrder(ctx, domainbilling.CreateCustomAmountOrderRequest{
		UserID: 9021, OrderNo: "PGO-EXPLICIT-EXPIRY", AmountCNY: "10.00000", CNYPerPoint: "0.31250", Provider: "mock",
		ExpiresAt: explicitExpiry,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !explicitOrder.ExpiresAt.Equal(explicitExpiry) {
		t.Fatalf("explicit expiry=%s, want %s", explicitOrder.ExpiresAt, explicitExpiry)
	}

	expired, err := svc.ExpirePendingOrders(ctx, explicitExpiry.Add(time.Second), 1)
	if err != nil || expired != 1 {
		t.Fatalf("first expiry sweep count=%d err=%v", expired, err)
	}
	expiredAgain, err := svc.ExpirePendingOrders(ctx, explicitExpiry.Add(time.Second), 10)
	if err != nil || expiredAgain != 0 {
		t.Fatalf("idempotent expiry sweep count=%d err=%v", expiredAgain, err)
	}
	reloaded, err := svc.GetOrder(ctx, explicitOrder.UserID, explicitOrder.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != "expired" || reloaded.ClosedAt == nil {
		t.Fatalf("expired order=%#v", reloaded)
	}
}

func TestPaymentOrderReadsLazilyExpirePendingOrders(t *testing.T) {
	ctx := t.Context()
	svc := NewService(config.BillingConfig{CNYPerPoint: "0.31250", PointsScale: 5})
	order, err := svc.CreateOrder(ctx, domainbilling.CreateOrderRequest{
		UserID: 9022, OrderNo: "PGO-LAZY-EXPIRY", PlanCode: "basic-monthly", Provider: "mock",
		ExpiresAt: time.Now().UTC().Add(-time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := svc.GetOrder(ctx, order.UserID, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != "expired" || loaded.ClosedAt == nil {
		t.Fatalf("GetOrder must lazily expire pending order, got %#v", loaded)
	}
	page, err := svc.ListOrders(ctx, domainbilling.ListOrdersRequest{UserID: order.UserID, Status: "expired"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Items[0].ID != order.ID {
		t.Fatalf("expired order must be visible through status filter, got %#v", page)
	}
}

func TestExpiredPlanOrderCanCompleteFromVerifiedLatePaymentExactlyOnce(t *testing.T) {
	ctx := t.Context()
	svc := NewService(config.BillingConfig{CNYPerPoint: "0.31250", PointsScale: 5})
	order, err := svc.CreateOrder(ctx, domainbilling.CreateOrderRequest{
		UserID: 9023, OrderNo: "PGO-LATE-PLAN", PlanCode: "basic-monthly", Provider: "mock",
		ExpiresAt: time.Now().UTC().Add(-time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetOrder(ctx, order.UserID, order.ID); err != nil {
		t.Fatal(err)
	}

	req := domainbilling.MarkOrderPaidRequest{
		Provider: "mock", OrderNo: order.OrderNo, TradeNo: "MOCK-LATE-PLAN", AmountCNY: order.AmountCNY,
		ReconciliationSource: domainbilling.PaymentReconciliationSourceProviderWebhook,
	}
	first, err := svc.MarkOrderPaid(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.MarkOrderPaid(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != "paid" || second.Status != "paid" || first.LedgerID == 0 || second.LedgerID != first.LedgerID {
		t.Fatalf("late payment must credit exactly once: first=%#v second=%#v", first, second)
	}
	balance, err := svc.GetBalance(ctx, order.UserID, "1.00000")
	if err != nil {
		t.Fatal(err)
	}
	if balance.AvailablePoints != "100.00000" {
		t.Fatalf("late payment must credit one order only, got %#v", balance)
	}
}

func TestPlanListDefaultsToNonArchivedAndSupportsStatusFilter(t *testing.T) {
	svc := NewService(config.BillingConfig{CNYPerPoint: "0.31250", PointsScale: 5})

	if _, err := svc.TransitionPlan(t.Context(), domainbilling.TransitionSubscriptionPlanRequest{PlanID: 1, Action: "disable"}); err != nil {
		t.Fatalf("disable plan: %v", err)
	}
	if _, err := svc.TransitionPlan(t.Context(), domainbilling.TransitionSubscriptionPlanRequest{PlanID: 2, Action: "archive"}); err != nil {
		t.Fatalf("archive plan: %v", err)
	}

	visible, err := svc.ListPlans(t.Context(), domainbilling.SubscriptionPlanListRequest{})
	if err != nil {
		t.Fatalf("ListPlans default: %v", err)
	}
	if len(visible) != 1 || visible[0].ID != 1 || visible[0].Status != "disabled" {
		t.Fatalf("expected only non-archived disabled plan, got %#v", visible)
	}

	archived, err := svc.ListPlans(t.Context(), domainbilling.SubscriptionPlanListRequest{Status: "archived"})
	if err != nil {
		t.Fatalf("ListPlans archived: %v", err)
	}
	if len(archived) != 1 || archived[0].ID != 2 || archived[0].Status != "archived" {
		t.Fatalf("expected only archived plan, got %#v", archived)
	}
}

func TestMemoryPlanPageUsesStableIDTieBreakForDescendingSort(t *testing.T) {
	svc := NewService(config.BillingConfig{CNYPerPoint: "0.31250", PointsScale: 5})
	created := make([]domainbilling.SubscriptionPlan, 0, 2)
	for _, code := range []string{"stable-alpha", "stable-beta"} {
		plan, err := svc.CreatePlan(t.Context(), domainbilling.CreateSubscriptionPlanRequest{
			PlanCode: code, PlanName: code, PlanType: "points_package", PurchaseEnabled: true,
			Status: "active", PriceCNY: "12.00000", Points: "20.00000", BonusPoints: "0.00000",
		})
		if err != nil {
			t.Fatal(err)
		}
		created = append(created, plan)
	}
	page, err := svc.ListPlansPage(t.Context(), domainbilling.SubscriptionPlanListRequest{
		Query: "stable-", SortBy: "price_cny", SortOrder: "desc", Page: 1, PageSize: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Items[0].ID != created[0].ID || page.Items[1].ID != created[1].ID {
		t.Fatalf("equal primary values must use ascending ID tie-break, got %#v", page.Items)
	}
}

func TestPlanStateTransitionsAreSafeAndIdempotent(t *testing.T) {
	svc := NewService(config.BillingConfig{CNYPerPoint: "0.31250", PointsScale: 5})

	disabled, err := svc.TransitionPlan(t.Context(), domainbilling.TransitionSubscriptionPlanRequest{PlanID: 1, Action: "disable"})
	if err != nil {
		t.Fatalf("disable plan: %v", err)
	}
	if disabled.Status != "disabled" || disabled.PurchaseEnabled {
		t.Fatalf("disabled plan must not be purchasable: %#v", disabled)
	}

	disabledAgain, err := svc.TransitionPlan(t.Context(), domainbilling.TransitionSubscriptionPlanRequest{PlanID: 1, Action: "disable"})
	if err != nil {
		t.Fatalf("disable plan again: %v", err)
	}
	if disabledAgain.Status != "disabled" || disabledAgain.PurchaseEnabled {
		t.Fatalf("repeated disable must remain disabled: %#v", disabledAgain)
	}

	enabled, err := svc.TransitionPlan(t.Context(), domainbilling.TransitionSubscriptionPlanRequest{PlanID: 1, Action: "enable"})
	if err != nil {
		t.Fatalf("enable plan: %v", err)
	}
	if enabled.Status != "active" || !enabled.PurchaseEnabled {
		t.Fatalf("enabled points package must be purchasable: %#v", enabled)
	}

	archived, err := svc.TransitionPlan(t.Context(), domainbilling.TransitionSubscriptionPlanRequest{PlanID: 1, Action: "archive"})
	if err != nil {
		t.Fatalf("archive plan: %v", err)
	}
	if archived.Status != "archived" || archived.PurchaseEnabled {
		t.Fatalf("archived plan must not be purchasable: %#v", archived)
	}

	restored, err := svc.TransitionPlan(t.Context(), domainbilling.TransitionSubscriptionPlanRequest{PlanID: 1, Action: "restore"})
	if err != nil {
		t.Fatalf("restore plan: %v", err)
	}
	if restored.Status != "disabled" || restored.PurchaseEnabled {
		t.Fatalf("restored plan must require explicit enable: %#v", restored)
	}

	durationDays := 30
	subscription, err := svc.CreatePlan(t.Context(), domainbilling.CreateSubscriptionPlanRequest{
		PlanCode: "subscription-placeholder", PlanName: "Subscription", PlanType: "subscription",
		PurchaseEnabled: false, Status: "disabled", PriceCNY: "99.00000", Points: "500.00000",
		Currency: "CNY", DurationDays: &durationDays,
	})
	if err != nil {
		t.Fatalf("CreatePlan subscription: %v", err)
	}
	subscription, err = svc.TransitionPlan(t.Context(), domainbilling.TransitionSubscriptionPlanRequest{PlanID: subscription.ID, Action: "enable"})
	if err != nil {
		t.Fatalf("enable subscription: %v", err)
	}
	if subscription.Status != "active" || subscription.PurchaseEnabled {
		t.Fatalf("non-points plan must not become purchasable: %#v", subscription)
	}
}

func TestAuditedPlanTransitionValidatesAndForwardsAuditContext(t *testing.T) {
	store := &auditedPlanStore{}
	svc := NewServiceWithStore(config.BillingConfig{}, store)
	audit := domainbilling.PlanLifecycleAudit{
		ActorType: "admin", ActorID: "9", Action: "cashier.plan.enable", TargetType: "cashier_plan", TargetID: "42", RequestID: "request-42",
	}
	result, err := svc.TransitionPlanAudited(t.Context(), domainbilling.TransitionSubscriptionPlanRequest{PlanID: 42, Action: " ENABLE "}, audit)
	if err != nil {
		t.Fatalf("TransitionPlanAudited: %v", err)
	}
	if result.ID != 42 || store.request.Action != domainbilling.SubscriptionPlanActionEnable || store.audit.RequestID != "request-42" {
		t.Fatalf("audited transition was not normalized and forwarded: result=%#v request=%#v audit=%#v", result, store.request, store.audit)
	}
	if _, err := svc.TransitionPlanAudited(t.Context(), domainbilling.TransitionSubscriptionPlanRequest{PlanID: 0, Action: "enable"}, audit); err == nil {
		t.Fatal("invalid audited transition plan_id was accepted")
	}
}

func TestPlanExpiryPolicyCreateListAndUpdateTransitions(t *testing.T) {
	svc := NewService(config.BillingConfig{CNYPerPoint: "0.31250", PointsScale: 5})
	created, err := svc.CreatePlan(t.Context(), domainbilling.CreateSubscriptionPlanRequest{
		PlanCode: "expiry-policy", PlanName: "Expiry Policy", PlanType: "points_package", PurchaseEnabled: true,
		Status: "active", PriceCNY: "10.00000", Points: "20.00000", BonusPoints: "2.00000", Currency: "CNY",
	})
	if err != nil {
		t.Fatalf("legacy CreatePlan: %v", err)
	}
	if !created.CreditExpiryEnabled || created.DurationDays == nil || *created.DurationDays != 30 {
		t.Fatalf("legacy create must default to 30-day expiry: %#v", created)
	}

	update := func(enabled *bool, days *int) domainbilling.SubscriptionPlan {
		t.Helper()
		updated, err := svc.UpdatePlan(t.Context(), domainbilling.UpdateSubscriptionPlanRequest{
			PlanID: created.ID, PlanName: created.PlanName, PlanType: created.PlanType, PurchaseEnabled: created.PurchaseEnabled,
			Status: created.Status, PriceCNY: created.PriceCNY, Points: created.Points, BonusPoints: created.BonusPoints,
			CreditExpiryEnabled: enabled, DurationDays: days, Currency: created.Currency,
		})
		if err != nil {
			t.Fatalf("UpdatePlan enabled=%v days=%v: %v", enabled, days, err)
		}
		created = updated
		return updated
	}

	if updated := update(boolPointerTest(true), intPointer(60)); !updated.CreditExpiryEnabled || updated.DurationDays == nil || *updated.DurationDays != 60 {
		t.Fatalf("expiring -> expiring lost policy: %#v", updated)
	}
	if updated := update(boolPointerTest(false), nil); updated.CreditExpiryEnabled || updated.DurationDays != nil {
		t.Fatalf("expiring -> permanent lost policy: %#v", updated)
	}
	if updated := update(boolPointerTest(true), intPointer(45)); !updated.CreditExpiryEnabled || updated.DurationDays == nil || *updated.DurationDays != 45 {
		t.Fatalf("permanent -> expiring lost policy: %#v", updated)
	}
	if updated := update(nil, nil); !updated.CreditExpiryEnabled || updated.DurationDays == nil || *updated.DurationDays != 30 {
		t.Fatalf("legacy update must default to 30-day expiry: %#v", updated)
	}

	items, err := svc.ListPlans(t.Context(), domainbilling.SubscriptionPlanListRequest{})
	if err != nil {
		t.Fatalf("ListPlans: %v", err)
	}
	found := false
	for _, item := range items {
		if item.ID == created.ID {
			found = item.CreditExpiryEnabled && item.DurationDays != nil && *item.DurationDays == 30
		}
	}
	if !found {
		t.Fatalf("list did not preserve final expiry policy: %#v", items)
	}
}

func TestPlanExpiryPolicyRequiresPositiveDurationWhenExplicitlyEnabled(t *testing.T) {
	svc := NewService(config.BillingConfig{CNYPerPoint: "0.31250", PointsScale: 5})
	enabled := true

	for name, days := range map[string]*int{
		"missing":  nil,
		"zero":     intPointer(0),
		"negative": intPointer(-1),
	} {
		t.Run("create "+name, func(t *testing.T) {
			_, err := svc.CreatePlan(t.Context(), domainbilling.CreateSubscriptionPlanRequest{
				PlanCode: "invalid-expiry-" + name, PlanName: "Invalid Expiry", PlanType: "points_package",
				Status: "active", PriceCNY: "10.00000", Points: "20.00000", BonusPoints: "0.00000",
				CreditExpiryEnabled: &enabled, DurationDays: days,
			})
			if err == nil {
				t.Fatalf("explicit expiry with %s duration must be rejected", name)
			}
		})
	}

	created, err := svc.CreatePlan(t.Context(), domainbilling.CreateSubscriptionPlanRequest{
		PlanCode: "valid-expiry-update", PlanName: "Valid Expiry", PlanType: "points_package",
		Status: "active", PriceCNY: "10.00000", Points: "20.00000", BonusPoints: "0.00000",
	})
	if err != nil {
		t.Fatalf("legacy CreatePlan: %v", err)
	}
	_, err = svc.UpdatePlan(t.Context(), domainbilling.UpdateSubscriptionPlanRequest{
		PlanID: created.ID, PlanName: created.PlanName, PlanType: created.PlanType, Status: created.Status,
		PriceCNY: created.PriceCNY, Points: created.Points, BonusPoints: created.BonusPoints,
		CreditExpiryEnabled: &enabled,
	})
	if err == nil {
		t.Fatal("explicit expiry update without duration must be rejected")
	}
}

func TestMemoryStoreFixedPackageOrderAlwaysSnapshotsCNY(t *testing.T) {
	store := NewMemoryStore(5)
	store.plans[0].Currency = "USD"

	order, err := store.CreateOrder(t.Context(), domainbilling.CreateOrderRequest{
		UserID: 903, PlanCode: store.plans[0].PlanCode, Provider: "mock",
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if order.Currency != "CNY" {
		t.Fatalf("fixed package order must snapshot CNY despite legacy plan currency, got %q", order.Currency)
	}
}

func boolPointerTest(value bool) *bool { return &value }

func TestPlanHistoricalOrderUsesSnapshotAfterArchive(t *testing.T) {
	svc := NewService(config.BillingConfig{CNYPerPoint: "0.31250", PointsScale: 5})
	order, err := svc.CreateOrder(t.Context(), domainbilling.CreateOrderRequest{
		UserID: 902, OrderNo: "PGO-PLAN-SNAPSHOT", PlanCode: "basic-monthly", Provider: "mock",
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if order.Points != "100.00000" {
		t.Fatalf("unexpected order snapshot: %#v", order)
	}
	if _, err := svc.TransitionPlan(t.Context(), domainbilling.TransitionSubscriptionPlanRequest{PlanID: order.PlanID, Action: "archive"}); err != nil {
		t.Fatalf("archive plan: %v", err)
	}
	if _, err := svc.MarkOrderPaid(t.Context(), domainbilling.MarkOrderPaidRequest{
		Provider: "mock", OrderNo: order.OrderNo, TradeNo: "SNAPSHOT-PAID", AmountCNY: order.AmountCNY,
	}); err != nil {
		t.Fatalf("MarkOrderPaid: %v", err)
	}
	balance, err := svc.GetBalance(t.Context(), order.UserID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance.AvailablePoints != "100.00000" {
		t.Fatalf("historical order must credit snapshotted points, got %#v", balance)
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

func TestEnsureSignupTrialGrantIsIdempotent(t *testing.T) {
	svc := NewService(config.BillingConfig{
		CNYPerPoint: "0.31250",
		SignupTrial: config.SignupTrialConfig{
			Enabled:            true,
			Points:             "15.00000",
			ValidDays:          7,
			ExpiryReminderDays: 2,
			GrantOncePerUser:   true,
		},
	})

	first, err := svc.EnsureSignupTrialGrant(context.Background(), SignupTrialGrantRequest{UserID: 203})
	if err != nil {
		t.Fatalf("EnsureSignupTrialGrant first: %v", err)
	}
	if !first.Granted || first.Balance.TrialPoints != "15.00000" || len(first.Balance.Buckets) != 1 || first.Balance.Buckets[0].Bucket != "trial" {
		t.Fatalf("expected signup trial grant balance, got %#v", first)
	}
	if first.Balance.Buckets[0].ExpiresAt == nil || first.Balance.Buckets[0].ExpireWarning {
		t.Fatalf("expected non-warning expiring trial bucket, got %#v", first.Balance.Buckets[0])
	}

	second, err := svc.EnsureSignupTrialGrant(context.Background(), SignupTrialGrantRequest{UserID: 203})
	if err != nil {
		t.Fatalf("EnsureSignupTrialGrant second: %v", err)
	}
	if second.Granted || second.Balance.TrialPoints != "15.00000" || second.Balance.AvailablePoints != "15.00000" {
		t.Fatalf("expected idempotent no-op second grant, got %#v", second)
	}

	disabled := NewService(config.BillingConfig{SignupTrial: config.SignupTrialConfig{Enabled: false, Points: "15.00000", ValidDays: 7}})
	result, err := disabled.EnsureSignupTrialGrant(context.Background(), SignupTrialGrantRequest{UserID: 204})
	if err != nil {
		t.Fatalf("EnsureSignupTrialGrant disabled: %v", err)
	}
	if result.Granted || result.Balance.AvailablePoints != "0.00000" {
		t.Fatalf("expected disabled signup trial to be a no-op, got %#v", result)
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
		CNYPerPoint:                      "0.31250",
		PointsScale:                      2,
		BaseResolutionPointsByModel:      map[string]map[string]string{"plus": {"2k": "8"}},
		TaskMultipliers:                  map[string]string{"text_to_image": "1"},
		UserGroupMultipliers:             map[string]string{"basic": "1"},
		AutoBaseResolutionDefaultByGroup: map[string]string{"plus": "2k"},
	})

	result, err := svc.Estimate(domainbilling.EstimateRequest{
		TaskType:                  "text_to_image",
		AbstractModel:             "plus",
		BaseResolution:            "auto",
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

func TestEstimateUsesAdminBillingPricingOverrides(t *testing.T) {
	cfg := config.BillingConfig{
		CNYPerPoint:                      "0.31250",
		PointsScale:                      5,
		BaseResolutionPointsByModel:      map[string]map[string]string{"plus": {"2k": "10.00000"}},
		TaskMultipliers:                  map[string]string{"text_to_image": "1.00000"},
		UserGroupMultipliers:             map[string]string{"basic": "1.00000"},
		AutoBaseResolutionDefaultByGroup: map[string]string{"plus": "2k"},
		ReferenceImageExtra:              config.ReferenceExtra{First: "0.00000", Additional: "0.00000"},
	}
	adminSvc := adminconfigservice.NewService(config.Config{Billing: cfg})
	if _, err := adminSvc.UpdateTab(context.Background(), domainadminconfig.UpdateTabRequest{
		TabKey:  "billing_pricing",
		Version: 1,
		Items: []domainadminconfig.Item{{
			ConfigCategory: "billing_pricing",
			ConfigKey:      "task_multipliers",
			ConfigValue:    map[string]any{"value": map[string]any{"text_to_image": "2.00000"}},
			Scope:          "global",
		}},
	}); err != nil {
		t.Fatalf("UpdateTab billing_pricing: %v", err)
	}

	svc := NewService(cfg)
	svc.SetAdminConfigResolver(adminSvc)

	result, err := svc.Estimate(domainbilling.EstimateRequest{
		TaskType:                  "text_to_image",
		AbstractModel:             "plus",
		BaseResolution:            "auto",
		RequestedOutputImageCount: 1,
		UserGroupCode:             "basic",
	})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}
	if result.EstimatedPoints != "20.00000" {
		t.Fatalf("expected DB task multiplier override to affect estimate, got %#v", result)
	}
}

func TestEstimateRouteModelAutoBaseResolutionUsesExplicitSize(t *testing.T) {
	svc := NewService(config.BillingConfig{
		CNYPerPoint:                      "0.31250",
		PointsScale:                      5,
		TaskMultipliers:                  map[string]string{"text_to_image": "1.00000"},
		AutoBaseResolutionDefaultByGroup: map[string]string{"plus": "4k"},
	})
	svc.SetModelRoutingSource(staticRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices: []modelhub.RoutePriceConfig{
			{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "2.00000", Enabled: true},
			{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "2k", BasePoints: "4.00000", Enabled: true},
			{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "4k", BasePoints: "8.00000", Enabled: true},
		},
		ProviderModels: []modelhub.ProviderCandidate{
			{AccountModelID: 12, ModelAccountID: 102, ModelCode: "gpt-image-1", SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"2k"}, SupportedAspectRatios: []string{"3:2"}},
		},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Priority: 1, Enabled: true}},
	}})

	result, err := svc.Estimate(domainbilling.EstimateRequest{
		TaskType:                  "text_to_image",
		RouteModelCode:            "plus",
		BaseResolution:            "auto",
		RequestedSize:             "1536x1024",
		RequestedOutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}
	if result.BaseResolution != "2k" {
		t.Fatalf("expected route billing to resolve 2k from explicit size, got %s", result.BaseResolution)
	}
	if result.EstimatedPoints != "4.00000" {
		t.Fatalf("expected 2k price, got %#v", result)
	}
}

func TestEstimateRouteModelPixelModeUsesPixelCapabilityWithoutQualityFilter(t *testing.T) {
	svc := NewService(config.BillingConfig{
		CNYPerPoint:     "0.31250",
		PointsScale:     5,
		TaskMultipliers: map[string]string{"text_to_image": "1.00000"},
	})
	svc.SetModelRoutingSource(staticRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices: []modelhub.RoutePriceConfig{
			{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "2.00000", Enabled: true},
			{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "2k", BasePoints: "4.00000", Enabled: true},
		},
		ProviderModels: []modelhub.ProviderCandidate{{
			AccountModelID:          12,
			ModelAccountID:          102,
			ModelCode:               "gpt-image-2",
			SupportedTaskTypes:      []string{"text_to_image"},
			SupportedBaseResolution: []string{"auto", "1k"},
			SizeModes:               []string{"ratio", "pixel"},
			SupportedPixelSizes:     []string{"1024x1024", "1824x1024"},
			SupportedAspectRatios:   []string{"1:1", "16:9"},
		}},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Priority: 1, Enabled: true}},
	}})

	result, err := svc.Estimate(domainbilling.EstimateRequest{
		TaskType:                  "text_to_image",
		RouteModelCode:            "plus",
		SizeMode:                  "pixel",
		RequestedSize:             "1824x1024",
		RequestedOutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("Estimate pixel mode: %v", err)
	}
	if result.BaseResolution != "2k" || result.EstimatedPoints != "4.00000" {
		t.Fatalf("expected pixel estimate to use 2k price, got %#v", result)
	}
}

func TestEstimateRouteModelAutoOmitsResolvedSize(t *testing.T) {
	svc := NewService(config.BillingConfig{
		CNYPerPoint: "0.31250", PointsScale: 5,
		TaskMultipliers:                  map[string]string{"text_to_image": "1.00000"},
		AutoBaseResolutionDefaultByGroup: map[string]string{"plus": "1k"},
	})
	svc.SetModelRoutingSource(staticRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices:      []modelhub.RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "2.00000", Enabled: true}},
		ProviderModels: []modelhub.ProviderCandidate{{
			AccountModelID: 12, ModelAccountID: 102, ModelCode: "gpt-image-2", SupportedTaskTypes: []string{"text_to_image"},
			SupportedBaseResolution: []string{"1k"}, SizeModes: []string{"auto", "ratio"}, SupportedAspectRatios: []string{"1:1"},
			Quality: []string{"auto"}, OutputFormat: []string{"png", "jpeg"}, SupportedBackgrounds: []string{"auto", "transparent"}, Moderation: []string{"auto"}, MaxImageCount: 1,
		}},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Priority: 1, Enabled: true}},
	}})

	result, err := svc.Estimate(domainbilling.EstimateRequest{
		TaskType: "text_to_image", RouteModelCode: "plus", SizeMode: "auto",
		Quality: "auto", OutputFormat: "png", Background: "auto", Moderation: "auto", RequestedOutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("Estimate auto: %v", err)
	}
	if result.ResolvedSize != nil || result.PricingSnapshot.RequestedSize != "" || result.PricingSnapshot.SizeMode != "auto" {
		t.Fatalf("auto estimate must omit resolved size, got %#v", result)
	}
}

func TestEstimateRouteModelAcceptsEnabledCustomRatio(t *testing.T) {
	svc := NewService(config.BillingConfig{
		CNYPerPoint: "0.31250", PointsScale: 5,
		TaskMultipliers: map[string]string{"text_to_image": "1.00000"},
	})
	svc.SetModelRoutingSource(staticRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices:      []modelhub.RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "2.00000", Enabled: true}},
		ProviderModels: []modelhub.ProviderCandidate{{
			AccountModelID: 12, ModelAccountID: 102, ModelCode: "gpt-image-2", SupportedTaskTypes: []string{"text_to_image"},
			SupportedBaseResolution: []string{"1k"}, SizeModes: []string{"ratio"}, SupportedAspectRatios: []string{"1:1"}, SupportsCustomRatio: true,
			Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"}, MaxImageCount: 1,
			MinWidth: 16, MaxWidth: 3840, MinHeight: 16, MaxHeight: 3840,
		}},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Priority: 1, Enabled: true}},
	}})

	result, err := svc.Estimate(domainbilling.EstimateRequest{
		TaskType: "text_to_image", RouteModelCode: "plus", SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "7:5",
		Quality: "auto", OutputFormat: "png", Moderation: "auto", RequestedOutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("Estimate custom ratio: %v", err)
	}
	if result.ResolvedSize == nil || *result.ResolvedSize != "1488x1056" {
		t.Fatalf("custom-ratio resolved size = %#v, want 1488x1056", result.ResolvedSize)
	}
}

func TestEstimateRouteModelReturnsBoundedRatioSize(t *testing.T) {
	svc := NewService(config.BillingConfig{TaskMultipliers: map[string]string{"text_to_image": "1.00000"}})
	svc.SetModelRoutingSource(staticRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "tight", Name: "Tight", Visibility: "public", Enabled: true}},
		Prices:      []modelhub.RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "2.00000", Enabled: true}},
		ProviderModels: []modelhub.ProviderCandidate{{
			AccountModelID: 12, ModelCode: "gpt-image-2", SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"},
			SizeModes: []string{"ratio"}, SupportedAspectRatios: []string{"1:1"}, Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"}, MaxImageCount: 1,
			MinWidth: 512, MaxWidth: 900, MinHeight: 512, MaxHeight: 900,
		}},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Enabled: true}},
	}})
	result, err := svc.Estimate(domainbilling.EstimateRequest{
		TaskType: "text_to_image", RouteModelCode: "tight", SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "1:1",
		Quality: "auto", OutputFormat: "png", Moderation: "auto", RequestedOutputImageCount: 1,
	})
	if err != nil || result.ResolvedSize == nil || *result.ResolvedSize != "896x896" || result.PricingSnapshot.RequestedSize != "896x896" {
		t.Fatalf("bounded estimate = %#v, %v; want resolved/requested size 896x896", result, err)
	}
}

func TestEstimateRouteModelRejectsTransparentJPEG(t *testing.T) {
	svc := NewService(config.BillingConfig{
		CNYPerPoint: "0.31250", PointsScale: 5,
		TaskMultipliers:                  map[string]string{"text_to_image": "1.00000"},
		AutoBaseResolutionDefaultByGroup: map[string]string{"plus": "1k"},
	})
	svc.SetModelRoutingSource(staticRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices:      []modelhub.RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "2.00000", Enabled: true}},
		ProviderModels: []modelhub.ProviderCandidate{{
			AccountModelID: 12, ModelAccountID: 102, ModelCode: "gpt-image-2", SupportedTaskTypes: []string{"text_to_image"},
			SupportedBaseResolution: []string{"1k"}, SizeModes: []string{"auto"}, Quality: []string{"auto"},
			OutputFormat: []string{"png", "jpeg"}, SupportedBackgrounds: []string{"transparent"}, Moderation: []string{"auto"}, MaxImageCount: 1,
		}},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Priority: 1, Enabled: true}},
	}})

	_, err := svc.Estimate(domainbilling.EstimateRequest{
		TaskType: "text_to_image", RouteModelCode: "plus", SizeMode: "auto",
		Quality: "auto", OutputFormat: "jpeg", Background: "transparent", Moderation: "auto", RequestedOutputImageCount: 1,
	})
	appErr, ok := err.(*errs.Error)
	if !ok || appErr.StatusCode != 400 || appErr.Code != modelhub.CodeTransparentFormatConflict {
		t.Fatalf("transparent JPEG estimate error = %#v, want 400/%s", err, modelhub.CodeTransparentFormatConflict)
	}
}

func TestEstimateRouteModelPreservesTypedSizeValidationErrors(t *testing.T) {
	svc := NewService(config.BillingConfig{
		CNYPerPoint: "0.31250", PointsScale: 5,
		TaskMultipliers: map[string]string{"text_to_image": "1.00000"},
	})
	svc.SetModelRoutingSource(staticRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices:      []modelhub.RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "2.00000", Enabled: true}},
		ProviderModels: []modelhub.ProviderCandidate{{
			AccountModelID: 12, ModelAccountID: 102, ModelCode: "gpt-image-2", SupportedTaskTypes: []string{"text_to_image"},
			SupportedBaseResolution: []string{"1k"}, SizeModes: []string{"ratio", "pixel"}, SupportedAspectRatios: []string{"1:1"},
			SupportedPixelSizes: []string{"1024x1024"}, SupportsCustomSize: true, Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"}, MaxImageCount: 1,
			MinWidth: 512, MaxWidth: 2048, MinHeight: 512, MaxHeight: 2048,
		}},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Priority: 1, Enabled: true}},
	}})

	tests := []struct {
		name  string
		req   domainbilling.EstimateRequest
		field string
		rule  string
	}{
		{name: "ratio bounds", req: domainbilling.EstimateRequest{SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "4:1"}, field: "aspect_ratio", rule: "max_ratio"},
		{name: "illegal pixels", req: domainbilling.EstimateRequest{SizeMode: "pixel", RequestedSize: "1001x777"}, field: "pixel_size", rule: "multiple_of_16"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.req.TaskType, tt.req.RouteModelCode = "text_to_image", "plus"
			tt.req.Quality, tt.req.OutputFormat, tt.req.Moderation = "auto", "png", "auto"
			tt.req.RequestedOutputImageCount = 1
			_, err := svc.Estimate(tt.req)
			appErr, ok := err.(*errs.Error)
			if !ok || appErr.StatusCode != 400 || appErr.Code != errs.CodeImageCapabilityMismatch {
				t.Fatalf("Estimate error = %#v, want 400/%s", err, errs.CodeImageCapabilityMismatch)
			}
			if appErr.Details["field"] != tt.field || appErr.Details["rule"] != tt.rule {
				t.Fatalf("Estimate details = %#v, want field=%q rule=%q", appErr.Details, tt.field, tt.rule)
			}
		})
	}

	_, err := svc.Estimate(domainbilling.EstimateRequest{
		TaskType: "text_to_image", RouteModelCode: "plus", SizeMode: "ratio", BaseResolution: "auto", AspectRatio: "1:1", RequestedSize: "1024x1024",
		Quality: "auto", OutputFormat: "png", Moderation: "auto", RequestedOutputImageCount: 1,
	})
	appErr, ok := err.(*errs.Error)
	if !ok || appErr.StatusCode != 400 || appErr.Code != modelhub.CodeInvalidSizeMode {
		t.Fatalf("mixed size mode error = %#v, want 400/%s", err, modelhub.CodeInvalidSizeMode)
	}
}

func TestEstimateRouteModelUsesOneRoutingSnapshotAndReturnsCapabilityVersion(t *testing.T) {
	source := &countingRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices:      []modelhub.RoutePriceConfig{{ID: 1, RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "2.00000", Enabled: true}},
		ProviderModels: []modelhub.ProviderCandidate{{
			AccountModelID: 12, SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"},
			SizeModes: []string{"ratio"}, SupportedAspectRatios: []string{"1:1"}, Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"}, MaxImageCount: 1,
		}},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Enabled: true}},
	}}
	svc := NewService(config.BillingConfig{TaskMultipliers: map[string]string{"text_to_image": "1.00000"}})
	svc.SetModelRoutingSource(source)
	result, err := svc.Estimate(domainbilling.EstimateRequest{
		TaskType: "text_to_image", RouteModelCode: "plus", SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "1:1",
		Quality: "auto", OutputFormat: "png", Moderation: "auto", RequestedOutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}
	if source.reads != 1 {
		t.Fatalf("routing source reads = %d, want exactly one immutable snapshot", source.reads)
	}
	if result.CapabilityVersion == "" {
		t.Fatal("estimate must return capability_version")
	}
}

func TestResolveAndEstimateRouteTaskPropagatesCanceledContext(t *testing.T) {
	source := &contextObservingRoutingSource{}
	svc := NewService(config.BillingConfig{})
	svc.SetModelRoutingSource(source)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, _ = svc.ResolveAndEstimateRouteTask(ctx, domainbilling.EstimateRequest{RouteModelCode: "plus", TaskType: "text_to_image"}, config.GenerationLimitsConfig{})
	if !errors.Is(source.err, context.Canceled) {
		t.Fatalf("routing source context error = %v, want context.Canceled", source.err)
	}
}

func TestEstimateRouteModelIgnoresInvalidPricesOnUnrelatedRoutes(t *testing.T) {
	svc := NewService(config.BillingConfig{TaskMultipliers: map[string]string{"text_to_image": "1.00000"}})
	svc.SetModelRoutingSource(staticRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		RouteModels: []modelhub.RouteModelConfig{
			{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true},
			{ID: 2, Code: "unrelated", Name: "Unrelated", Visibility: "public", Enabled: true},
		},
		Prices: []modelhub.RoutePriceConfig{
			{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "2.00000", Enabled: true},
			{RouteModelID: 2, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "not-a-decimal", Enabled: true},
		},
		ProviderModels: []modelhub.ProviderCandidate{{
			AccountModelID: 12, SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"},
			SizeModes: []string{"ratio"}, SupportedAspectRatios: []string{"1:1"}, Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"}, MaxImageCount: 1,
		}},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Enabled: true}},
	}})
	result, err := svc.Estimate(domainbilling.EstimateRequest{
		TaskType: "text_to_image", RouteModelCode: "plus", SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "1:1",
		Quality: "auto", OutputFormat: "png", Moderation: "auto", RequestedOutputImageCount: 1,
	})
	if err != nil || result.EstimatedPoints != "2.00000" {
		t.Fatalf("target route estimate = %#v, %v", result, err)
	}
}

func TestEstimateRouteModelRejectsWhenNoCandidateSupportsResolvedBaseResolution(t *testing.T) {
	svc := NewService(config.BillingConfig{
		CNYPerPoint:                      "0.31250",
		PointsScale:                      5,
		TaskMultipliers:                  map[string]string{"text_to_image": "1.00000"},
		AutoBaseResolutionDefaultByGroup: map[string]string{"plus": "2k"},
	})
	svc.SetModelRoutingSource(staticRoutingSource{snapshot: modelhub.ModelRoutingSnapshot{
		RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
		Prices:      []modelhub.RoutePriceConfig{{RouteModelID: 1, TaskType: "text_to_image", BaseResolution: "2k", BasePoints: "4.00000", Enabled: true}},
		ProviderModels: []modelhub.ProviderCandidate{
			{AccountModelID: 12, ModelAccountID: 102, ModelCode: "gpt-image-1", SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"}},
		},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Priority: 1, Enabled: true}},
	}})

	_, err := svc.Estimate(domainbilling.EstimateRequest{
		TaskType:                  "text_to_image",
		RouteModelCode:            "plus",
		BaseResolution:            "auto",
		RequestedOutputImageCount: 1,
	})
	appErr, ok := err.(*errs.Error)
	if !ok || appErr.StatusCode != 400 || appErr.Code != errs.CodeImageCapabilityMismatch {
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
			{AccountModelID: 12, ModelAccountID: 102, ModelCode: "gpt-image-1", SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"}},
		},
		Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Priority: 1, Enabled: true}},
	}})

	_, err := svc.Estimate(domainbilling.EstimateRequest{
		TaskType:                  "text_to_image",
		RouteModelCode:            "staff",
		BaseResolution:            "1k",
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
