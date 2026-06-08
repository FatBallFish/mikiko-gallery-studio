package cashier

import (
	"context"
	"errors"
	"strings"
	"testing"

	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
)

func TestPaymentAdapterRegistryBuildsMockDisplay(t *testing.T) {
	registry := NewPaymentAdapterRegistry()

	result, err := registry.BuildPaymentDisplay(context.Background(), PaymentDisplayRequest{
		Method:    domaincashier.VisibleMethod{Method: "mock"},
		Instance:  domaincashier.ProviderInstance{ID: 7, ProviderType: "mock"},
		OrderNo:   "PGO-MOCK-001",
		AmountCNY: "10.00000",
	})
	if err != nil {
		t.Fatalf("BuildPaymentDisplay mock returned error: %v", err)
	}
	if result.Display["type"] != "mock" || result.Display["provider_type"] != "mock" || result.Display["provider_instance_id"] != int64(7) {
		t.Fatalf("expected mock payment display base fields, got %#v", result.Display)
	}
	if result.PaymentURL != "" || result.QRCode != "" || result.ClientToken != "" {
		t.Fatalf("mock adapter should not expose legacy payment resources, got %#v", result)
	}
}

func TestPaymentAdapterRegistryDispatchesRegisteredBuilder(t *testing.T) {
	registry := NewPaymentAdapterRegistry()
	registry.Register("alipay_direct", func(_ context.Context, req PaymentDisplayRequest, display map[string]any) (PaymentDisplayResult, error) {
		if req.Instance.ID != 12 || display["type"] != "redirect" {
			t.Fatalf("builder received unexpected request/display: req=%#v display=%#v", req, display)
		}
		display["payment_url"] = "https://pay.example.test/checkout"
		display["signed"] = true
		return PaymentDisplayResult{Display: display, PaymentURL: "https://pay.example.test/checkout"}, nil
	})

	result, err := registry.BuildPaymentDisplay(context.Background(), PaymentDisplayRequest{
		Method:    domaincashier.VisibleMethod{Method: "alipay"},
		Instance:  domaincashier.ProviderInstance{ID: 12, ProviderType: "alipay_direct"},
		OrderNo:   "PGO-ALI-001",
		AmountCNY: "19.90000",
	})
	if err != nil {
		t.Fatalf("BuildPaymentDisplay alipay returned error: %v", err)
	}
	if result.PaymentURL == "" || result.Display["signed"] != true {
		t.Fatalf("expected registered builder result, got %#v", result)
	}
}

func TestPaymentAdapterRegistryWithBuildersRegistersStandardProviders(t *testing.T) {
	registry := NewPaymentAdapterRegistryWithBuilders(PaymentProviderBuilders{
		AlipayDirect: func(_ context.Context, req PaymentDisplayRequest, display map[string]any) (PaymentDisplayResult, error) {
			display["payment_url"] = "https://alipay.example.test/" + req.OrderNo
			return PaymentDisplayResult{Display: display, PaymentURL: "https://alipay.example.test/" + req.OrderNo}, nil
		},
		EasyPay: func(_ context.Context, _ PaymentDisplayRequest, display map[string]any) (PaymentDisplayResult, error) {
			display["payment_url"] = "https://easypay.example.test"
			return PaymentDisplayResult{Display: display, PaymentURL: "https://easypay.example.test"}, nil
		},
		JeePay: func(_ context.Context, _ PaymentDisplayRequest, display map[string]any) (PaymentDisplayResult, error) {
			display["payment_url"] = "https://jeepay.example.test"
			return PaymentDisplayResult{Display: display, PaymentURL: "https://jeepay.example.test"}, nil
		},
		WxPayDirect: func(_ context.Context, _ PaymentDisplayRequest, display map[string]any) (PaymentDisplayResult, error) {
			display["qr_code"] = "weixin://pay"
			return PaymentDisplayResult{Display: display, QRCode: "weixin://pay"}, nil
		},
	})

	cases := []string{"alipay_direct", "easypay_alipay", "easypay_wxpay", "jeepay_alipay", "jeepay_wxpay", "wxpay_direct"}
	for _, providerType := range cases {
		result, err := registry.BuildPaymentDisplay(context.Background(), PaymentDisplayRequest{
			Method:    domaincashier.VisibleMethod{Method: "alipay"},
			Instance:  domaincashier.ProviderInstance{ID: 12, ProviderType: providerType},
			OrderNo:   "PGO-STANDARD-001",
			AmountCNY: "19.90000",
		})
		if err != nil {
			t.Fatalf("BuildPaymentDisplay(%s) returned error: %v", providerType, err)
		}
		if result.PaymentURL == "" && result.QRCode == "" {
			t.Fatalf("expected standard provider %s to be registered, got %#v", providerType, result)
		}
	}
}

func TestPaymentAdapterRegistryRejectsUnknownProvider(t *testing.T) {
	registry := NewPaymentAdapterRegistry()

	_, err := registry.BuildPaymentDisplay(context.Background(), PaymentDisplayRequest{
		Method:   domaincashier.VisibleMethod{Method: "card"},
		Instance: domaincashier.ProviderInstance{ID: 99, ProviderType: "cardpay"},
	})
	if !errors.Is(err, ErrPaymentProviderNotImplemented) || !strings.Contains(err.Error(), "cardpay") {
		t.Fatalf("expected provider not implemented error, got %v", err)
	}
}
