package cashier

import (
	"context"
	"testing"
	"time"

	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
)

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
