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
