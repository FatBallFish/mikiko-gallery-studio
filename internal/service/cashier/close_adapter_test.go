package cashier

import (
	"context"
	"errors"
	"testing"

	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
)

func TestCloseAdapterRegistryRoutesSupportedProviders(t *testing.T) {
	called := ""
	builder := func(provider string) ClosePaymentBuilder {
		return func(_ context.Context, req ClosePaymentRequest) (ClosePaymentResult, error) {
			called = provider
			return BuildClosePaymentResult(req.Instance, "closed", true, false, nil), nil
		}
	}
	registry := NewCloseAdapterRegistryWithBuilders(CloseProviderBuilders{
		AlipayDirect: builder("alipay"), WxPayDirect: builder("wxpay"), EasyPay: builder("easypay"),
		JeePay: builder("jeepay"), Stripe: builder("stripe"),
	})

	for _, providerType := range []string{"alipay_direct", "wxpay_direct", "easypay_alipay", "easypay_wxpay", "jeepay_alipay", "jeepay_wxpay", "stripe"} {
		called = ""
		result, err := registry.ClosePayment(context.Background(), ClosePaymentRequest{
			Order:    OrderSnapshot{OrderNo: "PGO-CLOSE-001"},
			Instance: domaincashier.ProviderInstance{ID: 9, ProviderType: providerType},
		})
		if err != nil || !result.Closed || called == "" {
			t.Fatalf("close %s: called=%q result=%#v err=%v", providerType, called, result, err)
		}
	}
}

func TestCloseAdapterRegistryReturnsTypedUnsupportedForUnknownAndMock(t *testing.T) {
	registry := NewCloseAdapterRegistry()
	for _, providerType := range []string{"cardpay", "mock", ""} {
		result, err := registry.ClosePayment(context.Background(), ClosePaymentRequest{
			Order:    OrderSnapshot{OrderNo: "PGO-CLOSE-UNSUPPORTED"},
			Instance: domaincashier.ProviderInstance{ProviderType: providerType},
		})
		if !errors.Is(err, ErrPaymentCloseUnsupported) || !result.Unsupported || result.Closed {
			t.Fatalf("provider %q should be typed unsupported, result=%#v err=%v", providerType, result, err)
		}
	}
}

func TestEasyPayCloseRequiresExplicitEndpoint(t *testing.T) {
	result, err := EasyPayClosePaymentBuilder(context.Background(), ClosePaymentRequest{
		Order: OrderSnapshot{OrderNo: "PGO-EASYPAY-CLOSE"},
		Instance: domaincashier.ProviderInstance{ProviderType: "easypay_alipay", Config: map[string]any{
			"gateway_url": "https://easypay.example", "pid": "1001", "key": "secret",
		}},
	})
	if !errors.Is(err, ErrPaymentCloseUnsupported) || !result.Unsupported || result.Closed {
		t.Fatalf("EasyPay without close_url must fail closed: result=%#v err=%v", result, err)
	}
}
