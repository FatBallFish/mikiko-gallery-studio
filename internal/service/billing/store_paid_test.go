package billing

import (
	"encoding/json"
	"testing"

	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
)

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
