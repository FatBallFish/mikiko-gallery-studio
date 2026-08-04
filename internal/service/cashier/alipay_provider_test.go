package cashier

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/url"
	"strings"
	"testing"

	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
)

func TestAlipayPaymentDisplayBuilderBuildsSignedRedirectURL(t *testing.T) {
	privateKey := alipayTestPrivateKeyPEM(t)
	builder := NewAlipayPaymentDisplayBuilder(CallbackURLConfig{
		SiteBaseURL: "https://pic.example.com",
	})

	result, err := builder(context.Background(), PaymentDisplayRequest{
		Method: domaincashier.VisibleMethod{Method: "alipay"},
		Instance: domaincashier.ProviderInstance{
			ID:           12,
			ProviderType: "alipay_direct",
			Config: map[string]any{
				"app_id":          "app-123",
				"app_private_key": privateKey,
			},
		},
		OrderNo:         "PGO-ALI-001",
		AmountCNY:       "19.90000",
		Subject:         "Pic Gallery 充值",
		ClientReturnURL: "https://client.example.com/#/checkout",
	}, BasePaymentDisplay(PaymentDisplayRequest{
		Method:    domaincashier.VisibleMethod{Method: "alipay"},
		Instance:  domaincashier.ProviderInstance{ID: 12, ProviderType: "alipay_direct"},
		OrderNo:   "PGO-ALI-001",
		AmountCNY: "19.90000",
	}, "alipay_direct"))
	if err != nil {
		t.Fatalf("BuildAlipayPaymentDisplay returned error: %v", err)
	}
	if result.PaymentURL == "" {
		t.Fatalf("expected payment url in result %#v", result)
	}
	parsed, err := url.Parse(result.PaymentURL)
	if err != nil {
		t.Fatalf("parse payment url: %v", err)
	}
	query := parsed.Query()
	if parsed.Scheme != "https" || parsed.Host != "openapi.alipaydev.com" {
		t.Fatalf("expected default alipay sandbox gateway, got %s", result.PaymentURL)
	}
	if query.Get("app_id") != "app-123" || query.Get("method") != "alipay.trade.page.pay" {
		t.Fatalf("unexpected alipay gateway params: %s", result.PaymentURL)
	}
	if query.Has("out_trade_no") || query.Has("total_amount") {
		t.Fatalf("Alipay business fields must only appear in biz_content: %s", result.PaymentURL)
	}
	if query.Get("notify_url") != "https://pic.example.com/api/open/image/v1/payments/webhooks/alipay_direct" {
		t.Fatalf("unexpected notify_url %q", query.Get("notify_url"))
	}
	if query.Get("return_url") != "https://client.example.com/#/checkout" {
		t.Fatalf("unexpected return_url %q", query.Get("return_url"))
	}
	if !strings.Contains(query.Get("biz_content"), "PGO-ALI-001") || !strings.Contains(query.Get("biz_content"), "FAST_INSTANT_TRADE_PAY") {
		t.Fatalf("expected order payload in biz_content, got %q", query.Get("biz_content"))
	}
	if query.Get("sign_type") != "RSA2" || query.Get("sign") == "" {
		t.Fatalf("expected RSA2 signature in %s", result.PaymentURL)
	}
	if result.Display["type"] != "redirect" || result.Display["payment_url"] != result.PaymentURL || result.Display["signed"] != true || result.Display["sign_type"] != "RSA2" {
		t.Fatalf("unexpected alipay display %#v", result.Display)
	}
}

func TestAlipayPaymentRejectsFractionalFen(t *testing.T) {
	req := PaymentDisplayRequest{
		Instance: domaincashier.ProviderInstance{ProviderType: "alipay_direct", Config: map[string]any{
			"app_id": "app-123", "app_private_key": alipayTestPrivateKeyPEM(t),
		}},
		OrderNo: "PGO-ALIPAY-FRACTIONAL-FEN", AmountCNY: "12.345", Subject: "fractional fen",
	}
	if _, _, err := BuildAlipayPaymentURL(CallbackURLConfig{}, req); err == nil {
		t.Fatal("alipay payment must reject amounts that cannot be represented as whole fen")
	}
}

func alipayTestPrivateKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if len(privatePEM) == 0 {
		t.Fatal("expected private key pem")
	}
	return string(privatePEM)
}
