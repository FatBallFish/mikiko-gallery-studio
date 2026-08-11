package handlers

import (
	"encoding/json"
	"testing"
	"time"

	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	cashierservice "github.com/fatballfish/pic-gallery/internal/service/cashier"
)

func TestPublicCashierOrderProjectionHidesProviderTopology(t *testing.T) {
	order := domainbilling.PaymentOrder{
		ID: 11, OrderNo: "order-11", UserID: 7, PlanID: 3, PlanCode: "points", PlanName: "Points",
		Provider: "jeepay_alipay", ProviderType: "jeepay_alipay", ProviderInstanceID: 99,
		VisibleMethod: "", PurchaseType: "plan", IdempotencyKey: "internal-key", Status: "pending",
		Currency: "CNY", AmountCNY: "12.50000", Points: "100.00000", BonusPoints: "10.00000",
		TradeNo: "channel-trade", LedgerID: 123, PaymentURL: "https://pay.example.test/order-11",
		PaymentDisplay: map[string]any{
			"type": "qr_code", "payment_url": "https://pay.example.test/order-11", "qr_code": "weixin://pay",
			"prepay_mode": "api", "channel_trade_no": "channel-trade", "way_code": "ALI_PC",
		},
		ExpiresAt: time.Now().Add(time.Minute), CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}

	payload := marshalPublicJSON(t, publicCashierOrder(order))
	if payload["visible_method"] != "alipay" {
		t.Fatalf("visible_method = %#v, want alipay", payload["visible_method"])
	}
	for _, key := range []string{
		"user_id", "provider", "provider_type", "provider_instance_id", "idempotency_key", "trade_no",
		"refund_trade_no", "channel_refund_no", "ledger_id",
	} {
		if _, exists := payload[key]; exists {
			t.Fatalf("public cashier order exposed internal field %q: %#v", key, payload)
		}
	}
	display, ok := payload["payment_display"].(map[string]any)
	if !ok {
		t.Fatalf("payment_display = %#v", payload["payment_display"])
	}
	for _, key := range []string{"prepay_mode", "channel_trade_no", "way_code", "provider_type"} {
		if _, exists := display[key]; exists {
			t.Fatalf("public payment display exposed internal field %q: %#v", key, display)
		}
	}
}

func TestPublicCashierSyncProjectionHidesProviderIdentifiers(t *testing.T) {
	result := cashierservice.QueryOrderStatusResult{
		ProviderType: "easypay_alipay", ProviderInstanceID: 27, QueryStatus: "paid",
		RiskCategory: "paid", ActionHint: "refresh", Paid: true, Completed: true,
		TradeNo: "secret-channel-trade", AmountCNY: "10.00000", Message: "paid",
		Raw: map[string]any{"provider": "internal"}, SyncedAt: time.Now(),
	}

	payload := marshalPublicJSON(t, publicCashierSyncResult(result))
	for _, key := range []string{"provider_type", "provider_instance_id", "trade_no", "raw", "action_hint"} {
		if _, exists := payload[key]; exists {
			t.Fatalf("public sync result exposed internal field %q: %#v", key, payload)
		}
	}
	if payload["query_status"] != "paid" || payload["paid"] != true || payload["completed"] != true {
		t.Fatalf("public sync state lost required fields: %#v", payload)
	}
}

func marshalPublicJSON(t *testing.T, value any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}
