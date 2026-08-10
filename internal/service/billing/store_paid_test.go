package billing

import (
	"encoding/json"
	"testing"
	"time"

	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
)

func TestMemoryStoreRefundCannotUseCreditsFromAnotherOrder(t *testing.T) {
	store := NewMemoryStore(5)
	first := completeMemoryPlanOrder(t, store, 708, "plus-monthly", "memory-order-owned-first")
	if _, err := store.ReserveTask(t.Context(), ReserveStoreRequest{UserID: 708, TaskID: "memory-order-owned-consume", EstimatedPoints: "100.00000"}); err != nil {
		t.Fatalf("ReserveTask first order: %v", err)
	}
	if _, err := store.FinalizeTask(t.Context(), FinalizeStoreRequest{UserID: 708, TaskID: "memory-order-owned-consume", EstimatedPoints: "100.00000", ActualPoints: "100.00000"}); err != nil {
		t.Fatalf("FinalizeTask first order: %v", err)
	}
	_ = completeMemoryPlanOrder(t, store, 708, "plus-monthly", "memory-order-owned-second")
	if _, err := store.Adjust(t.Context(), AdjustStoreRequest{UserID: 708, ChangePoints: "100.00000", Reason: "unrelated gift"}); err != nil {
		t.Fatalf("Adjust unrelated gift: %v", err)
	}

	if _, err := store.RefundPaymentOrder(t.Context(), domainbilling.RefundPaymentOrderRequest{
		UserID: 708, OrderID: first.ID, RefundTradeNo: "memory-order-owned-refund",
	}); err == nil {
		t.Fatal("refund must not use credits owned by another order or an unrelated gift")
	}
	reloaded, err := store.GetOrder(t.Context(), 708, first.ID)
	if err != nil {
		t.Fatalf("GetOrder first: %v", err)
	}
	if reloaded.Status != "completed" || reloaded.RefundTradeNo != "" {
		t.Fatalf("rejected order-owned refund must not mutate order, got %#v", reloaded)
	}
}

func TestMemoryStoreLegacyMarkOrderPaidCreatesOwnedPackageGrants(t *testing.T) {
	store := NewMemoryStore(5)
	order, err := store.CreateOrder(t.Context(), domainbilling.CreateOrderRequest{
		UserID: 713, PlanCode: "plus-monthly", Provider: "mock",
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	paid, err := store.MarkOrderPaid(t.Context(), domainbilling.MarkOrderPaidRequest{
		Provider: "mock", OrderNo: order.OrderNo, TradeNo: "memory-legacy-owned", AmountCNY: order.AmountCNY,
	})
	if err != nil {
		t.Fatalf("MarkOrderPaid: %v", err)
	}
	grants := store.walletGrants[713]
	if len(grants) != 2 || grants[0].OrderID != order.ID || grants[0].GrantType != "subscription" || grants[0].Available.StringFixed(5) != "300.00000" || grants[1].OrderID != order.ID || grants[1].GrantType != "gift" || grants[1].Available.StringFixed(5) != "30.00000" {
		t.Fatalf("legacy paid path must create order-owned purchased and gift grants, got %#v", grants)
	}
	if paid.CreditExpiresAt == nil || grants[0].ExpiresAt == nil || grants[1].ExpiresAt == nil || !grants[0].ExpiresAt.Equal(*paid.CreditExpiresAt) || !grants[1].ExpiresAt.Equal(*paid.CreditExpiresAt) {
		t.Fatalf("legacy paid grants must use the order expiry snapshot, order=%#v grants=%#v", paid, grants)
	}
	if len(store.ledgers[713]) != 2 || store.ledgers[713][0].BalanceBucket != "gift" || store.ledgers[713][1].BalanceBucket != "subscription" {
		t.Fatalf("legacy paid path must write split ledgers, got %#v", store.ledgers[713])
	}
}

func TestMemoryStoreFixedPackageExpiryAndBalanceProjectionMatchGrantLifecycle(t *testing.T) {
	store := NewMemoryStore(5)
	completed := completeMemoryPlanOrder(t, store, 709, "plus-monthly", "memory-expiring-package")
	balance, err := store.GetBalance(t.Context(), 709)
	if err != nil {
		t.Fatalf("GetBalance before expiry: %v", err)
	}
	if balance.SubscriptionPoints != "300.00000" || balance.GiftPoints != "30.00000" || balance.NextExpiringGrant == nil || balance.NextExpiringGrant.AvailablePoints != "330.00000" || balance.NextExpiringGrant.GrantType != "mixed" || balance.NextExpiringGrant.ExpiresAt == nil || completed.CreditExpiresAt == nil || !balance.NextExpiringGrant.ExpiresAt.Equal(*completed.CreditExpiresAt) {
		t.Fatalf("memory balance must project package, gift, and next expiry, got %#v", balance)
	}
	foundGift := false
	for _, bucket := range balance.Buckets {
		if bucket.Bucket == "gift" && bucket.AvailablePoints == "30.00000" {
			foundGift = true
		}
	}
	if !foundGift {
		t.Fatalf("memory balance must emit the gift bucket, got %#v", balance.Buckets)
	}
	ledgers, err := store.ListLedger(t.Context(), 709, 1, 10)
	if err != nil {
		t.Fatalf("ListLedger: %v", err)
	}
	if len(ledgers.Items) != 2 || ledgers.Items[0].BalanceBucket != "gift" || ledgers.Items[0].ChangePoints != "30.00000" || ledgers.Items[1].BalanceBucket != "subscription" || ledgers.Items[1].ChangePoints != "300.00000" {
		t.Fatalf("memory package completion must write split ledgers, got %#v", ledgers.Items)
	}

	past := time.Now().UTC().Add(-time.Hour)
	for _, grant := range store.walletGrants[709] {
		grant.ExpiresAt = &past
	}
	balance, err = store.GetBalance(t.Context(), 709)
	if err != nil {
		t.Fatalf("GetBalance after expiry: %v", err)
	}
	if balance.AvailablePoints != "0.00000" || balance.SubscriptionPoints != "0.00000" || balance.GiftPoints != "0.00000" || balance.NextExpiringGrant != nil {
		t.Fatalf("expired fixed package must disappear from memory balance, got %#v", balance)
	}
}

func TestMemoryStorePermanentFixedPackageRemainsAvailable(t *testing.T) {
	store := NewMemoryStore(5)
	disabled := false
	plan, err := store.CreatePlan(t.Context(), domainbilling.CreateSubscriptionPlanRequest{
		PlanCode: "memory-permanent", PlanName: "Memory Permanent", PlanType: "points_package", PurchaseEnabled: true,
		Status: "active", PriceCNY: "10.00000", Points: "80.00000", BonusPoints: "8.00000", CreditExpiryEnabled: &disabled,
	})
	if err != nil {
		t.Fatalf("CreatePlan permanent: %v", err)
	}
	completed := completeMemoryPlanOrder(t, store, 710, plan.PlanCode, "memory-permanent-package")
	if completed.CreditExpiresAt != nil {
		t.Fatalf("permanent package must have nil expiry, got %#v", completed)
	}
	for _, grant := range store.walletGrants[710] {
		if grant.ExpiresAt != nil {
			t.Fatalf("permanent package grant must have nil expiry, got %#v", grant)
		}
	}
	balance, err := store.GetBalance(t.Context(), 710)
	if err != nil {
		t.Fatalf("GetBalance permanent: %v", err)
	}
	if balance.AvailablePoints != "88.00000" || balance.SubscriptionPoints != "80.00000" || balance.GiftPoints != "8.00000" || balance.NextExpiringGrant != nil {
		t.Fatalf("permanent package must remain available without next expiry, got %#v", balance)
	}
}

func TestMemoryStoreFrozenRefundCrossesGrantExpiry(t *testing.T) {
	t.Run("finalize", func(t *testing.T) {
		store := NewMemoryStore(5)
		order := completeMemoryPlanOrder(t, store, 711, "plus-monthly", "memory-expiry-finalize")
		request := domainbilling.RefundPaymentOrderRequest{UserID: 711, OrderID: order.ID, RefundTradeNo: "memory-expiry-finalize"}
		if _, err := store.FreezeRefundPaymentOrder(t.Context(), request); err != nil {
			t.Fatalf("FreezeRefundPaymentOrder: %v", err)
		}
		past := time.Now().UTC().Add(-time.Hour)
		for _, grant := range store.walletGrants[711] {
			grant.ExpiresAt = &past
		}
		pendingBalance, err := store.GetBalance(t.Context(), 711)
		if err != nil {
			t.Fatalf("GetBalance: %v", err)
		}
		assertMemoryBalanceHasNoFrozenProjection(t, pendingBalance)
		first, err := store.RefundPaymentOrder(t.Context(), request)
		if err != nil {
			t.Fatalf("RefundPaymentOrder after expiry: %v", err)
		}
		second, err := store.RefundPaymentOrder(t.Context(), request)
		if err != nil || second.Status != first.Status || second.RefundedPoints != first.RefundedPoints {
			t.Fatalf("expired finalize replay must be idempotent: first=%#v second=%#v err=%v", first, second, err)
		}
		for _, grant := range store.walletGrants[711] {
			if grant.Status != "expired" || !grant.Available.IsZero() || !grant.Frozen.IsZero() {
				t.Fatalf("finalize must keep expired grants empty, got %#v", grant)
			}
		}
	})

	t.Run("release", func(t *testing.T) {
		store := NewMemoryStore(5)
		order := completeMemoryPlanOrder(t, store, 712, "plus-monthly", "memory-expiry-release")
		request := domainbilling.RefundPaymentOrderRequest{UserID: 712, OrderID: order.ID, RefundTradeNo: "memory-expiry-release"}
		if _, err := store.FreezeRefundPaymentOrder(t.Context(), request); err != nil {
			t.Fatalf("FreezeRefundPaymentOrder: %v", err)
		}
		past := time.Now().UTC().Add(-time.Hour)
		for _, grant := range store.walletGrants[712] {
			grant.ExpiresAt = &past
		}
		pendingBalance, err := store.GetBalance(t.Context(), 712)
		if err != nil {
			t.Fatalf("GetBalance: %v", err)
		}
		assertMemoryBalanceHasNoFrozenProjection(t, pendingBalance)
		if _, err := store.ReleaseRefundPaymentOrder(t.Context(), request); err != nil {
			t.Fatalf("ReleaseRefundPaymentOrder after expiry: %v", err)
		}
		if _, err := store.ReleaseRefundPaymentOrder(t.Context(), request); err != nil {
			t.Fatalf("ReleaseRefundPaymentOrder replay: %v", err)
		}
		balance, err := store.GetBalance(t.Context(), 712)
		if err != nil {
			t.Fatalf("GetBalance after release: %v", err)
		}
		if balance.AvailablePoints != "0.00000" || balance.FrozenPoints != "0.00000" {
			t.Fatalf("release after expiry must not restore points, got %#v", balance)
		}
	})
}

func TestMemoryStoreExpiredRefundFreezePreservesOtherFrozenProjection(t *testing.T) {
	for _, action := range []string{"finalize", "release"} {
		t.Run(action, func(t *testing.T) {
			store := NewMemoryStore(5)
			const userID = int64(717)
			order := completeMemoryPlanOrder(t, store, userID, "plus-monthly", "memory-expired-refund-with-other-freeze-"+action)
			request := domainbilling.RefundPaymentOrderRequest{UserID: userID, OrderID: order.ID, RefundTradeNo: "memory-expired-refund-with-other-freeze-" + action}
			if _, err := store.FreezeRefundPaymentOrder(t.Context(), request); err != nil {
				t.Fatalf("FreezeRefundPaymentOrder: %v", err)
			}
			past := time.Now().UTC().Add(-time.Hour)
			for _, allocation := range store.refundFreezes[order.ID].Allocations {
				grant := store.memoryGrantByIDLocked(userID, allocation.GrantID)
				grant.ExpiresAt = &past
			}
			if _, err := store.GetBalance(t.Context(), userID); err != nil {
				t.Fatalf("GetBalance after expiry: %v", err)
			}
			if _, err := store.Adjust(t.Context(), AdjustStoreRequest{
				UserID: userID, ChangePoints: "50.00000", Reason: "unrelated active balance",
			}); err != nil {
				t.Fatalf("Adjust unrelated active balance: %v", err)
			}
			if _, err := store.ReserveTask(t.Context(), ReserveStoreRequest{
				UserID: userID, TaskID: "active-task-" + action, EstimatedPoints: "50.00000",
			}); err != nil {
				t.Fatalf("ReserveTask active task: %v", err)
			}

			var err error
			if action == "finalize" {
				_, err = store.RefundPaymentOrder(t.Context(), request)
			} else {
				_, err = store.ReleaseRefundPaymentOrder(t.Context(), request)
			}
			if err != nil {
				t.Fatalf("%s expired refund freeze: %v", action, err)
			}
			balance, err := store.GetBalance(t.Context(), userID)
			if err != nil {
				t.Fatalf("GetBalance after %s: %v", action, err)
			}
			if balance.FrozenPoints != "50.00000" {
				t.Fatalf("%s of an expired refund freeze must preserve unrelated active frozen points, got %#v", action, balance)
			}
		})
	}
}

func TestMemoryStorePartialSettlementAfterGrantExpiryUsesSemanticRefund(t *testing.T) {
	store := NewMemoryStore(5)
	const (
		userID   = int64(714)
		apiKeyID = int64(9714)
		taskID   = "memory-partial-settlement-after-expiry"
	)
	_ = completeMemoryPlanOrder(t, store, userID, "plus-monthly", "memory-partial-settlement-after-expiry")
	if _, err := store.ReserveTask(t.Context(), ReserveStoreRequest{
		UserID: userID, APIKeyID: apiKeyID, TaskID: taskID, EstimatedPoints: "100.00000", Reason: "reserve before expiry",
	}); err != nil {
		t.Fatalf("ReserveTask: %v", err)
	}
	past := time.Now().UTC().Add(-time.Hour)
	for _, grant := range store.walletGrants[userID] {
		grant.ExpiresAt = &past
	}
	if balance, err := store.GetBalance(t.Context(), userID); err != nil {
		t.Fatalf("GetBalance after expiry: %v", err)
	} else {
		assertMemoryBalanceHasNoFrozenProjection(t, balance)
	}

	settled, err := store.FinalizeTask(t.Context(), FinalizeStoreRequest{
		UserID: userID, APIKeyID: apiKeyID, TaskID: taskID, EstimatedPoints: "100.00000", ActualPoints: "40.00000", Reason: "partial result",
	})
	if err != nil {
		t.Fatalf("FinalizeTask: %v", err)
	}
	if settled.AvailablePoints != "0.00000" || settled.FrozenPoints != "0.00000" {
		t.Fatalf("expired settlement must not restore spendable points or frozen projection, got %#v", settled)
	}
	usage, err := store.APIKeyUsage(t.Context(), apiKeyID, nil)
	if err != nil {
		t.Fatalf("APIKeyUsage: %v", err)
	}
	if usage != "40.00000" {
		t.Fatalf("API key usage must settle to actual points after expiry, got %s", usage)
	}
	page, err := store.ListLedger(t.Context(), userID, 1, 20)
	if err != nil {
		t.Fatalf("ListLedger: %v", err)
	}
	entries := map[string]domainbilling.LedgerEntry{}
	for _, entry := range page.Items {
		if entry.TaskID == taskID {
			entries[entry.LedgerType] = entry
		}
	}
	if entries["consume"].ChangePoints != "-40.00000" || entries["consume"].FrozenAfter != "0.00000" {
		t.Fatalf("consume ledger must record actual settlement after expiry, got %#v", entries["consume"])
	}
	if entries["refund"].ChangePoints != "60.00000" || entries["refund"].BalanceAfter != "0.00000" || entries["refund"].FrozenAfter != "0.00000" {
		t.Fatalf("refund ledger must record the semantic reserved-actual difference without restoring expired points, got %#v", entries["refund"])
	}
}

func TestMemoryStoreExpiredTaskSettlementPreservesOtherFrozenProjection(t *testing.T) {
	store := NewMemoryStore(5)
	const userID = int64(716)
	_ = completeMemoryPlanOrder(t, store, userID, "plus-monthly", "memory-expired-task-with-other-freeze")
	if _, err := store.ReserveTask(t.Context(), ReserveStoreRequest{
		UserID: userID, TaskID: "expired-task", EstimatedPoints: "100.00000",
	}); err != nil {
		t.Fatalf("ReserveTask expired task: %v", err)
	}
	past := time.Now().UTC().Add(-time.Hour)
	for _, allocation := range store.taskState["expired-task"].GrantAllocations {
		grant := store.memoryGrantByIDLocked(userID, allocation.GrantID)
		grant.ExpiresAt = &past
	}
	if _, err := store.GetBalance(t.Context(), userID); err != nil {
		t.Fatalf("GetBalance after expiry: %v", err)
	}
	if _, err := store.Adjust(t.Context(), AdjustStoreRequest{
		UserID: userID, ChangePoints: "50.00000", Reason: "unrelated active balance",
	}); err != nil {
		t.Fatalf("Adjust unrelated active balance: %v", err)
	}
	if _, err := store.ReserveTask(t.Context(), ReserveStoreRequest{
		UserID: userID, TaskID: "active-task", EstimatedPoints: "50.00000",
	}); err != nil {
		t.Fatalf("ReserveTask active task: %v", err)
	}

	settled, err := store.FinalizeTask(t.Context(), FinalizeStoreRequest{
		UserID: userID, TaskID: "expired-task", EstimatedPoints: "100.00000", ActualPoints: "40.00000",
	})
	if err != nil {
		t.Fatalf("FinalizeTask expired task: %v", err)
	}
	if settled.FrozenPoints != "50.00000" {
		t.Fatalf("settling an expired task must preserve unrelated active frozen points, got %#v", settled)
	}
}

func TestMemoryStoreBucketExpiryAggregationIsOrderIndependent(t *testing.T) {
	now := time.Now().UTC()
	firstExpiry := now.Add(24 * time.Hour)
	secondExpiry := now.Add(48 * time.Hour)
	tests := []struct {
		name        string
		expires     []*time.Time
		wantMixed   bool
		wantExpires *time.Time
	}{
		{name: "permanent_then_expiring", expires: []*time.Time{nil, &firstExpiry}, wantMixed: true},
		{name: "expiring_then_permanent", expires: []*time.Time{&firstExpiry, nil}, wantMixed: true},
		{name: "different_expiries", expires: []*time.Time{&firstExpiry, &secondExpiry}, wantMixed: true},
		{name: "same_expiry", expires: []*time.Time{&firstExpiry, &firstExpiry}, wantExpires: &firstExpiry},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := NewMemoryStore(5)
			const userID = int64(715)
			for index, expiresAt := range test.expires {
				store.walletGrants[userID] = append(store.walletGrants[userID], &memoryWalletGrant{
					ID: int64(index + 1), UserID: userID, GrantType: "subscription", Status: "active",
					Available: mustMemoryDecimal("1.00000"), ExpiresAt: expiresAt,
				})
			}
			_, expiresAt, mixed, tracked := store.memoryBucketGrantProjection(userID, "subscription", now)
			if mixed != test.wantMixed || tracked.StringFixed(5) != "2.00000" {
				t.Fatalf("unexpected expiry aggregation: mixed=%v expires=%v tracked=%s", mixed, expiresAt, tracked)
			}
			if test.wantExpires == nil && expiresAt != nil {
				t.Fatalf("mixed expiry must not expose a whole-bucket expiry, got %s", expiresAt)
			}
			if test.wantExpires != nil && (expiresAt == nil || !expiresAt.Equal(*test.wantExpires)) {
				t.Fatalf("uniform expiry must be preserved, got %v want %v", expiresAt, test.wantExpires)
			}
		})
	}
}

func assertMemoryBalanceHasNoFrozenProjection(t *testing.T, balance BalanceState) {
	t.Helper()
	if balance.FrozenPoints != "0.00000" {
		t.Fatalf("expired grants must not remain in the total frozen projection, got %#v", balance)
	}
	for _, bucket := range balance.Buckets {
		if bucket.FrozenPoints != "0.00000" && bucket.FrozenPoints != "" {
			t.Fatalf("expired grants must not remain in bucket frozen projections, got %#v", balance.Buckets)
		}
	}
}

func completeMemoryPlanOrder(t *testing.T, store *MemoryStore, userID int64, planCode, tradeNo string) domainbilling.PaymentOrder {
	t.Helper()
	order, err := store.CreateOrder(t.Context(), domainbilling.CreateOrderRequest{
		UserID: userID, PlanCode: planCode, Provider: "mock", PurchaseType: "plan", VisibleMethod: "mock", ProviderType: "mock",
	})
	if err != nil {
		t.Fatalf("CreateOrder %s: %v", planCode, err)
	}
	completed, err := store.CompleteRechargeOrder(t.Context(), domainbilling.CompleteRechargeOrderRequest{
		UserID: userID, OrderID: order.ID, Provider: "mock", TradeNo: tradeNo,
	})
	if err != nil {
		t.Fatalf("CompleteRechargeOrder %s: %v", planCode, err)
	}
	return completed
}

func TestMemoryStoreMarkOrderPaidRecoversCashierOrdersExactlyOnce(t *testing.T) {
	for _, previousStatus := range []string{"pending", "canceled", "expired", "failed"} {
		t.Run(previousStatus, func(t *testing.T) {
			store := NewMemoryStore(5)
			order, err := store.CreateCustomAmountOrder(t.Context(), domainbilling.CreateCustomAmountOrderRequest{
				UserID:             701,
				OrderNo:            "PGO-MEMORY-RECOVER-" + previousStatus,
				AmountCNY:          "10.00000",
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
			order.Status = previousStatus
			store.orders[order.ID] = order

			req := domainbilling.MarkOrderPaidRequest{
				Provider:             "jeepay_alipay",
				ProviderInstanceID:   41,
				OrderNo:              order.OrderNo,
				TradeNo:              "JEEPAY-MEMORY-" + previousStatus,
				AmountCNY:            order.AmountCNY,
				ReconciliationSource: domainbilling.PaymentReconciliationSourceProviderWebhook,
			}
			first, err := store.MarkOrderPaid(t.Context(), req)
			if err != nil {
				t.Fatalf("MarkOrderPaid first: %v", err)
			}
			second, err := store.MarkOrderPaid(t.Context(), req)
			if err != nil {
				t.Fatalf("MarkOrderPaid second: %v", err)
			}
			if first.Status != "completed" || second.Status != "completed" || first.LedgerID == 0 || second.LedgerID != first.LedgerID {
				t.Fatalf("expected one idempotently completed order, first=%#v second=%#v", first, second)
			}

			balance, err := store.GetBalance(t.Context(), order.UserID)
			if err != nil {
				t.Fatalf("GetBalance: %v", err)
			}
			if balance.RechargePoints != "32.00000" || balance.AvailablePoints != "32.00000" {
				t.Fatalf("expected one recharge credit, got %#v", balance)
			}
			ledger, err := store.ListLedger(t.Context(), order.UserID, 1, 10)
			if err != nil {
				t.Fatalf("ListLedger: %v", err)
			}
			if len(ledger.Items) != 1 || ledger.Items[0].LedgerType != "recharge" {
				t.Fatalf("expected one recharge ledger entry, got %#v", ledger.Items)
			}
			webhooks, err := store.ListWebhookEvents(t.Context(), 1, 10)
			if err != nil {
				t.Fatalf("ListWebhookEvents: %v", err)
			}
			if len(webhooks.Items) != 1 {
				t.Fatalf("expected one reconciliation event, got %#v", webhooks.Items)
			}
			var audit map[string]any
			if err := json.Unmarshal([]byte(webhooks.Items[0].PayloadPreview), &audit); err != nil {
				t.Fatalf("decode reconciliation audit: %v", err)
			}
			if audit["previous_local_status"] != previousStatus || audit["reconciliation_source"] != domainbilling.PaymentReconciliationSourceProviderWebhook {
				t.Fatalf("expected recovery audit metadata, got %#v", audit)
			}
		})
	}
}

func TestMemoryStoreMarkOrderPaidDoesNotRecoverRefundOrDisputeStates(t *testing.T) {
	for _, terminalStatus := range []string{"partially_refunded", "refunded", "chargeback", "dispute"} {
		t.Run(terminalStatus, func(t *testing.T) {
			store := NewMemoryStore(5)
			order, err := store.CreateCustomAmountOrder(t.Context(), domainbilling.CreateCustomAmountOrderRequest{
				UserID: 702, OrderNo: "PGO-MEMORY-TERMINAL-" + terminalStatus,
				AmountCNY: "10.00000", CNYPerPoint: "0.31250", Provider: "mock",
				PurchaseType: "custom_amount", VisibleMethod: "mock", ProviderType: "mock",
			})
			if err != nil {
				t.Fatalf("CreateCustomAmountOrder: %v", err)
			}
			order.Status = terminalStatus
			store.orders[order.ID] = order

			if _, err := store.MarkOrderPaid(t.Context(), domainbilling.MarkOrderPaidRequest{
				Provider: "mock", OrderNo: order.OrderNo, TradeNo: "MOCK-TERMINAL-" + terminalStatus,
				AmountCNY: order.AmountCNY, ReconciliationSource: domainbilling.PaymentReconciliationSourceProviderWebhook,
			}); err == nil {
				t.Fatal("expected terminal payment state to reject paid recovery")
			}
			if len(store.ledgers[order.UserID]) != 0 || len(store.webhooks) != 0 {
				t.Fatalf("terminal recovery must not credit or audit success: ledgers=%#v webhooks=%#v", store.ledgers[order.UserID], store.webhooks)
			}
		})
	}
}

func TestMemoryStoreMarkOrderPaidRejectsDifferentTradeForCompletedOrder(t *testing.T) {
	store := NewMemoryStore(5)
	order, err := store.CreateCustomAmountOrder(t.Context(), domainbilling.CreateCustomAmountOrderRequest{
		UserID: 703, OrderNo: "PGO-MEMORY-IDEMPOTENCY-BINDING", AmountCNY: "10.00000",
		CNYPerPoint: "0.31250", Provider: "mock", PurchaseType: "custom_amount", VisibleMethod: "mock", ProviderType: "mock",
	})
	if err != nil {
		t.Fatalf("CreateCustomAmountOrder: %v", err)
	}
	if _, err := store.MarkOrderPaid(t.Context(), domainbilling.MarkOrderPaidRequest{
		Provider: "mock", OrderNo: order.OrderNo, TradeNo: "MOCK-FIRST", AmountCNY: order.AmountCNY,
	}); err != nil {
		t.Fatalf("MarkOrderPaid first: %v", err)
	}
	if _, err := store.MarkOrderPaid(t.Context(), domainbilling.MarkOrderPaidRequest{
		Provider: "mock", OrderNo: order.OrderNo, TradeNo: "MOCK-DIFFERENT", AmountCNY: order.AmountCNY,
	}); err == nil {
		t.Fatal("expected completed order to reject a different provider trade")
	}
	if len(store.ledgers[order.UserID]) != 1 || len(store.webhooks) != 1 {
		t.Fatalf("mismatched replay must not duplicate credit: ledgers=%d webhooks=%d", len(store.ledgers[order.UserID]), len(store.webhooks))
	}
}

func TestMemoryStoreMarkOrderPaidRejectsDifferentTradeForInitializedOrder(t *testing.T) {
	store := NewMemoryStore(5)
	order, err := store.CreateCustomAmountOrder(t.Context(), domainbilling.CreateCustomAmountOrderRequest{
		UserID: 706, OrderNo: "PGO-MEMORY-INITIAL-TRADE", AmountCNY: "10.00000", CNYPerPoint: "0.31250",
		Provider: "jeepay_alipay", PurchaseType: "custom_amount", VisibleMethod: "alipay", ProviderType: "jeepay_alipay", ProviderInstanceID: 41,
	})
	if err != nil {
		t.Fatalf("CreateCustomAmountOrder: %v", err)
	}
	order, err = store.InitializePaymentOrder(t.Context(), domainbilling.InitializePaymentOrderRequest{
		UserID: order.UserID, OrderID: order.ID, PaymentDisplay: map[string]any{"type": "redirect"}, TradeNo: "JEEPAY-BOUND-001",
	})
	if err != nil {
		t.Fatalf("InitializePaymentOrder: %v", err)
	}
	if _, err := store.MarkOrderPaid(t.Context(), domainbilling.MarkOrderPaidRequest{
		Provider: "jeepay_alipay", ProviderInstanceID: 41, OrderNo: order.OrderNo,
		TradeNo: "JEEPAY-OTHER-ORDER", AmountCNY: order.AmountCNY,
	}); err == nil {
		t.Fatal("expected initialized order to reject a different provider trade")
	}
	reloaded, err := store.GetOrder(t.Context(), order.UserID, order.ID)
	if err != nil || reloaded.Status != "pending" || reloaded.TradeNo != "JEEPAY-BOUND-001" || reloaded.LedgerID != 0 {
		t.Fatalf("mismatched initialized trade must not mutate order: order=%#v err=%v", reloaded, err)
	}
}

func TestMemoryStoreMarkOrderPaidRejectsProviderTradeOwnedByDifferentOrder(t *testing.T) {
	store := NewMemoryStore(5)
	createOrder := func(userID int64, orderNo string) domainbilling.PaymentOrder {
		t.Helper()
		order, err := store.CreateCustomAmountOrder(t.Context(), domainbilling.CreateCustomAmountOrderRequest{
			UserID: userID, OrderNo: orderNo, AmountCNY: "10.00000", CNYPerPoint: "0.31250",
			Provider: "mock", PurchaseType: "custom_amount", VisibleMethod: "mock", ProviderType: "mock",
		})
		if err != nil {
			t.Fatalf("CreateCustomAmountOrder: %v", err)
		}
		return order
	}
	firstOrder := createOrder(704, "PGO-MEMORY-TRADE-OWNER-FIRST")
	secondOrder := createOrder(705, "PGO-MEMORY-TRADE-OWNER-SECOND")
	paid := func(order domainbilling.PaymentOrder) (domainbilling.PaymentOrder, error) {
		return store.MarkOrderPaid(t.Context(), domainbilling.MarkOrderPaidRequest{
			Provider: "mock", OrderNo: order.OrderNo, TradeNo: "MOCK-SHARED-TRADE", AmountCNY: order.AmountCNY,
		})
	}
	first, err := paid(firstOrder)
	if err != nil || first.Status != "completed" {
		t.Fatalf("complete first order: order=%#v err=%v", first, err)
	}
	replayed, err := paid(firstOrder)
	if err != nil || replayed.Status != "completed" || replayed.LedgerID != first.LedgerID {
		t.Fatalf("same-order replay must remain idempotent: order=%#v err=%v", replayed, err)
	}
	if _, err := paid(secondOrder); err == nil {
		t.Fatal("expected provider trade owned by first order to reject second order")
	}
	secondReloaded, err := store.GetOrder(t.Context(), secondOrder.UserID, secondOrder.ID)
	if err != nil {
		t.Fatalf("GetOrder second: %v", err)
	}
	if secondReloaded.Status != "pending" || secondReloaded.LedgerID != 0 {
		t.Fatalf("cross-order replay must not mutate second order: %#v", secondReloaded)
	}
	if len(store.webhooks) != 1 || len(store.ledgers[firstOrder.UserID]) != 1 || len(store.ledgers[secondOrder.UserID]) != 0 {
		t.Fatalf("cross-order replay must credit once: webhooks=%d first_ledgers=%d second_ledgers=%d", len(store.webhooks), len(store.ledgers[firstOrder.UserID]), len(store.ledgers[secondOrder.UserID]))
	}
}

func TestMemoryStoreCompleteRechargeOrderRejectsProviderTradeOwnedByDifferentOrder(t *testing.T) {
	store := NewMemoryStore(5)
	createOrder := func(userID int64, orderNo string) domainbilling.PaymentOrder {
		t.Helper()
		order, err := store.CreateCustomAmountOrder(t.Context(), domainbilling.CreateCustomAmountOrderRequest{
			UserID: userID, OrderNo: orderNo, AmountCNY: "10.00000", CNYPerPoint: "0.31250",
			Provider: "mock", PurchaseType: "custom_amount", VisibleMethod: "mock", ProviderType: "mock",
		})
		if err != nil {
			t.Fatalf("CreateCustomAmountOrder: %v", err)
		}
		return order
	}
	firstOrder := createOrder(706, "PGO-MEMORY-COMPLETE-OWNER-FIRST")
	secondOrder := createOrder(707, "PGO-MEMORY-COMPLETE-OWNER-SECOND")
	complete := func(order domainbilling.PaymentOrder) (domainbilling.PaymentOrder, error) {
		return store.CompleteRechargeOrder(t.Context(), domainbilling.CompleteRechargeOrderRequest{
			UserID: order.UserID, OrderID: order.ID, Provider: "mock", TradeNo: "MOCK-COMPLETE-SHARED-TRADE",
		})
	}
	first, err := complete(firstOrder)
	if err != nil || first.Status != "completed" {
		t.Fatalf("complete first order: order=%#v err=%v", first, err)
	}
	replayed, err := complete(firstOrder)
	if err != nil || replayed.Status != "completed" || replayed.LedgerID != first.LedgerID {
		t.Fatalf("same-order replay must remain idempotent: order=%#v err=%v", replayed, err)
	}
	if _, err := complete(secondOrder); err == nil {
		t.Fatal("expected provider trade owned by first order to reject second order")
	}
	secondReloaded, err := store.GetOrder(t.Context(), secondOrder.UserID, secondOrder.ID)
	if err != nil {
		t.Fatalf("GetOrder second: %v", err)
	}
	if secondReloaded.Status != "pending" || len(store.ledgers[secondOrder.UserID]) != 0 || len(store.webhooks) != 1 {
		t.Fatalf("cross-order complete must not mutate second order: order=%#v ledgers=%#v webhooks=%#v", secondReloaded, store.ledgers[secondOrder.UserID], store.webhooks)
	}
}
