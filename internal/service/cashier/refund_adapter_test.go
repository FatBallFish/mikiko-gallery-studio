package cashier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
)

func TestJeePayRefundUsesMillisecondTimeAndCanonicalSignature(t *testing.T) {
	const merchantKey = "merchant-secret"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/refund/refundOrder" {
			t.Fatalf("unexpected refund request %s %s", r.Method, r.URL.Path)
		}
		if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
			t.Fatalf("refund content type = %q, want application/json", contentType)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode refund JSON: %v", err)
		}
		values := jeepayStringMapForTest(payload)
		if _, ok := payload["reqTime"].(float64); !ok || len(values["reqTime"]) != 13 || values["version"] != "1.0" || values["signType"] != "MD5" {
			t.Fatalf("unexpected JeePay refund contract: %#v", payload)
		}
		if _, ok := payload["refundAmount"].(float64); !ok || values["refundAmount"] != "1990" {
			t.Fatalf("refundAmount must be numeric fen: %#v", payload)
		}
		if got, want := values["sign"], officialJeePaySignForTest(payload, merchantKey); got != want {
			t.Fatalf("refund sign = %s, want %s", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"state":1,"refundOrderId":"R20260804001"}}`))
	}))
	defer upstream.Close()

	result, err := JeePayRefundPaymentBuilder(context.Background(), RefundPaymentRequest{
		Order: OrderSnapshot{OrderNo: "PGO-JEEPAY-REFUND", TradeNo: "P20260804001", AmountCNY: "19.90000", Status: "completed"},
		Instance: domaincashier.ProviderInstance{ID: 8, ProviderType: "jeepay_alipay", Config: map[string]any{
			"gateway_url": upstream.URL, "mch_no": "MCH10001", "app_id": "APP10001", "key": merchantKey,
		}},
		RefundTradeNo: "MGR-REFUND-001", RefundAmountCNY: "19.90000", Reason: "user requested",
	})
	if err != nil {
		t.Fatalf("JeePayRefundPaymentBuilder returned error: %v", err)
	}
	if result.ChannelRefundNo != "R20260804001" || strings.TrimSpace(result.RefundStatus) == "" {
		t.Fatalf("unexpected refund result %#v", result)
	}
}

func TestPaymentRefundsRequireExplicitProviderSuccessCode(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	tests := []struct {
		name   string
		refund func() error
	}{
		{name: "JeePay", refund: func() error {
			_, err := JeePayRefundPaymentBuilder(context.Background(), RefundPaymentRequest{
				Order: OrderSnapshot{OrderNo: "PGO-JEEPAY-REFUND-MISSING-CODE", AmountCNY: "19.90000", Status: "completed"},
				Instance: domaincashier.ProviderInstance{ID: 8, ProviderType: "jeepay_alipay", Config: map[string]any{
					"gateway_url": upstream.URL, "mch_no": "MCH10001", "app_id": "APP10001", "key": "merchant-secret",
				}},
				RefundTradeNo: "MGR-JEEPAY-MISSING-CODE", RefundAmountCNY: "19.90000",
			})
			return err
		}},
		{name: "Alipay", refund: func() error {
			_, err := AlipayRefundPaymentBuilder(context.Background(), RefundPaymentRequest{
				Order: OrderSnapshot{OrderNo: "PGO-ALIPAY-REFUND-MISSING-CODE", AmountCNY: "19.90000", Status: "completed"},
				Instance: domaincashier.ProviderInstance{ID: 12, ProviderType: "alipay_direct", Config: map[string]any{
					"gateway_url": upstream.URL, "app_id": "app-123", "app_private_key": alipayTestPrivateKeyPEM(t),
				}},
				RefundTradeNo: "MGR-ALIPAY-MISSING-CODE", RefundAmountCNY: "19.90000",
			})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.refund(); err == nil {
				t.Fatal("refund response without an explicit success code must be rejected")
			}
		})
	}
}

func TestRefundAdapterRegistryDispatchesRegisteredProvider(t *testing.T) {
	registry := NewRefundAdapterRegistry()
	registry.Register("alipay_direct", func(_ context.Context, req RefundPaymentRequest) (RefundPaymentResult, error) {
		if req.Order.OrderNo != "PGO-REFUND-001" || req.RefundTradeNo != "refund-001" || req.RefundAmountCNY != "5.00000" {
			t.Fatalf("builder received unexpected request: %#v", req)
		}
		return RefundPaymentResult{
			ProviderType:       req.Instance.ProviderType,
			ProviderInstanceID: req.Instance.ID,
			RefundStatus:       "accepted",
			RefundTradeNo:      req.RefundTradeNo,
			ChannelRefundNo:    "ALI-REFUND-001",
			Message:            "accepted",
			Raw:                map[string]any{"source": "test_alipay_refund"},
			RefundedAt:         time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC),
		}, nil
	})

	result, shouldCall, err := registry.RefundPayment(context.Background(), RefundPaymentRequest{
		Order:           OrderSnapshot{OrderNo: "PGO-REFUND-001", AmountCNY: "19.90000", Status: "completed"},
		Instance:        domaincashier.ProviderInstance{ID: 12, ProviderType: "alipay_direct"},
		RefundTradeNo:   "refund-001",
		RefundAmountCNY: "5.00000",
		Reason:          "user requested",
	})
	if err != nil {
		t.Fatalf("RefundPayment returned error: %v", err)
	}
	if !shouldCall {
		t.Fatal("expected registered provider to require channel refund")
	}
	if result.ProviderType != "alipay_direct" || result.ChannelRefundNo != "ALI-REFUND-001" || result.Raw["source"] != "test_alipay_refund" {
		t.Fatalf("unexpected refund result: %#v", result)
	}
}

func TestRefundAdapterRegistryWithBuildersRegistersStandardProviders(t *testing.T) {
	registry := NewRefundAdapterRegistryWithBuilders(RefundProviderBuilders{
		AlipayDirect: func(_ context.Context, req RefundPaymentRequest) (RefundPaymentResult, error) {
			return refundResultForTest(req, "ALI-REFUND"), nil
		},
		EasyPay: func(_ context.Context, req RefundPaymentRequest) (RefundPaymentResult, error) {
			return refundResultForTest(req, "EASY-REFUND"), nil
		},
		JeePay: func(_ context.Context, req RefundPaymentRequest) (RefundPaymentResult, error) {
			return refundResultForTest(req, "JEEPAY-REFUND"), nil
		},
		WxPayDirect: func(_ context.Context, req RefundPaymentRequest) (RefundPaymentResult, error) {
			return refundResultForTest(req, "WX-REFUND"), nil
		},
		Stripe: func(_ context.Context, req RefundPaymentRequest) (RefundPaymentResult, error) {
			return refundResultForTest(req, "STRIPE-REFUND"), nil
		},
	})

	for _, providerType := range []string{"alipay_direct", "easypay_alipay", "easypay_wxpay", "jeepay_alipay", "jeepay_wxpay", "wxpay_direct", "stripe"} {
		result, shouldCall, err := registry.RefundPayment(context.Background(), RefundPaymentRequest{
			Order:           OrderSnapshot{OrderNo: "PGO-REFUND", Status: "completed"},
			Instance:        domaincashier.ProviderInstance{ID: 9, ProviderType: providerType},
			RefundTradeNo:   "refund-001",
			RefundAmountCNY: "5.00000",
		})
		if err != nil || !shouldCall {
			t.Fatalf("RefundPayment(%s) expected channel call, shouldCall=%v err=%v", providerType, shouldCall, err)
		}
		if result.ChannelRefundNo == "" {
			t.Fatalf("expected standard refund provider %s to be registered, got %#v", providerType, result)
		}
	}
}

func refundResultForTest(req RefundPaymentRequest, channelRefundNo string) RefundPaymentResult {
	return RefundPaymentResult{
		ProviderType:       req.Instance.ProviderType,
		ProviderInstanceID: req.Instance.ID,
		RefundStatus:       "accepted",
		RefundTradeNo:      req.RefundTradeNo,
		ChannelRefundNo:    channelRefundNo,
		RefundedAt:         time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC),
	}
}

func TestRefundAdapterRegistrySkipsLocalOnlyProviders(t *testing.T) {
	registry := NewRefundAdapterRegistry()
	for _, providerType := range []string{"mock", "manual_alipay", "manual_wxpay", "manual_bank"} {
		result, shouldCall, err := registry.RefundPayment(context.Background(), RefundPaymentRequest{
			Order:    OrderSnapshot{OrderNo: "PGO-LOCAL-ONLY", Status: "completed"},
			Instance: domaincashier.ProviderInstance{ID: 1, ProviderType: providerType},
		})
		if err != nil || shouldCall || result.ProviderType != "" {
			t.Fatalf("expected local-only provider %q to skip channel refund, got result=%#v shouldCall=%v err=%v", providerType, result, shouldCall, err)
		}
	}
}

func TestRefundAdapterRegistrySkipsNonRefundableOrderStatus(t *testing.T) {
	registry := NewRefundAdapterRegistry()
	result, shouldCall, err := registry.RefundPayment(context.Background(), RefundPaymentRequest{
		Order:    OrderSnapshot{OrderNo: "PGO-PENDING", Status: "pending"},
		Instance: domaincashier.ProviderInstance{ID: 12, ProviderType: "alipay_direct"},
	})
	if err != nil || shouldCall || result.ProviderType != "" {
		t.Fatalf("expected pending order to skip channel refund, got result=%#v shouldCall=%v err=%v", result, shouldCall, err)
	}
}

func TestRefundAdapterRegistryIgnoresUnknownProvider(t *testing.T) {
	registry := NewRefundAdapterRegistry()
	result, shouldCall, err := registry.RefundPayment(context.Background(), RefundPaymentRequest{
		Order:    OrderSnapshot{OrderNo: "PGO-CARD", Status: "completed"},
		Instance: domaincashier.ProviderInstance{ID: 77, ProviderType: "cardpay"},
	})
	if err != nil || shouldCall || result.ProviderType != "" {
		t.Fatalf("expected unknown provider to skip channel refund, got result=%#v shouldCall=%v err=%v", result, shouldCall, err)
	}
}
