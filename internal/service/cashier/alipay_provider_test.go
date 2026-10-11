package cashier

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

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

func TestAlipayPaymentUsesWapPayForMobileDevices(t *testing.T) {
	privateKey := alipayTestPrivateKeyPEM(t)
	config := map[string]any{"app_id": "app-123", "app_private_key": privateKey}
	base := PaymentDisplayRequest{
		Instance: domaincashier.ProviderInstance{ID: 7, ProviderType: "alipay_direct", Config: config},
		OrderNo:  "PGO-ALI-DEV", AmountCNY: "9.9", Subject: "device test",
	}

	desktop := base
	desktopURL, _, err := BuildAlipayPaymentURL(CallbackURLConfig{SiteBaseURL: "https://pic.example.com"}, desktop)
	if err != nil {
		t.Fatalf("build desktop payment url: %v", err)
	}
	if desktopQuery := mustAlipayQuery(t, desktopURL); desktopQuery.Get("method") != "alipay.trade.page.pay" || !strings.Contains(desktopQuery.Get("biz_content"), "FAST_INSTANT_TRADE_PAY") {
		t.Fatalf("desktop must use page.pay, got %s", desktopURL)
	}

	mobile := base
	mobile.ClientDevice = "mobile"
	mobileURL, _, err := BuildAlipayPaymentURL(CallbackURLConfig{SiteBaseURL: "https://pic.example.com"}, mobile)
	if err != nil {
		t.Fatalf("build mobile payment url: %v", err)
	}
	mobileQuery := mustAlipayQuery(t, mobileURL)
	if mobileQuery.Get("method") != "alipay.trade.wap.pay" || !strings.Contains(mobileQuery.Get("biz_content"), "QUICK_WAP_WAY") {
		t.Fatalf("mobile must use wap.pay with QUICK_WAP_WAY, got %s", mobileURL)
	}
}

func TestAlipayPaymentNormalizesTotalAmountToTwoDecimals(t *testing.T) {
	privateKey := alipayTestPrivateKeyPEM(t)
	for raw, want := range map[string]string{"9.90000": "9.90", "9.9": "9.90", "12.50000": "12.50"} {
		req := PaymentDisplayRequest{
			Instance: domaincashier.ProviderInstance{ProviderType: "alipay_direct", Config: map[string]any{
				"app_id": "app-123", "app_private_key": privateKey,
			}},
			OrderNo: "PGO-ALI-AMOUNT", AmountCNY: raw, Subject: "amount",
		}
		paymentURL, _, err := BuildAlipayPaymentURL(CallbackURLConfig{}, req)
		if err != nil {
			t.Fatalf("build payment url for %s: %v", raw, err)
		}
		bizContent := mustAlipayQuery(t, paymentURL).Get("biz_content")
		if !strings.Contains(bizContent, fmt.Sprintf(`"total_amount":%q`, want)) {
			t.Fatalf("expected total_amount %s for raw %s, got %s", want, raw, bizContent)
		}
	}
}

func TestAlipayPaymentTimestampUsesBeijingTime(t *testing.T) {
	privateKey := alipayTestPrivateKeyPEM(t)
	paymentURL, _, err := BuildAlipayPaymentURL(CallbackURLConfig{}, PaymentDisplayRequest{
		Instance: domaincashier.ProviderInstance{ProviderType: "alipay_direct", Config: map[string]any{
			"app_id": "app-123", "app_private_key": privateKey,
		}},
		OrderNo: "PGO-ALI-TZ", AmountCNY: "1.00", Subject: "tz",
	})
	if err != nil {
		t.Fatalf("build payment url: %v", err)
	}
	timestamp := mustAlipayQuery(t, paymentURL).Get("timestamp")
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", timestamp, time.FixedZone("GMT+8", 8*60*60))
	if err != nil {
		t.Fatalf("timestamp %q is not in alipay format", timestamp)
	}
	beijingNow := time.Now().In(time.FixedZone("GMT+8", 8*60*60))
	if diff := parsed.Sub(beijingNow); diff > time.Minute || diff < -time.Minute {
		t.Fatalf("timestamp %q must be Beijing local time, now=%s", timestamp, beijingNow.Format("2006-01-02 15:04:05"))
	}
}

func mustAlipayQuery(t *testing.T, paymentURL string) url.Values {
	t.Helper()
	parsed, err := url.Parse(paymentURL)
	if err != nil {
		t.Fatalf("parse payment url: %v", err)
	}
	return parsed.Query()
}
