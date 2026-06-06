package cashier

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
)

func TestEasyPayPaymentDisplayBuilderBuildsSignedPopupURL(t *testing.T) {
	builder := NewEasyPayPaymentDisplayBuilder(CallbackURLConfig{SiteBaseURL: "https://pic.example.com"})
	req := PaymentDisplayRequest{
		Method: domaincashier.VisibleMethod{Method: "alipay"},
		Instance: domaincashier.ProviderInstance{
			ID:           21,
			ProviderType: "easypay_alipay",
			Config: map[string]any{
				"gateway_url":  "https://pay.example.com/submit.php?old=1",
				"pid":          "10001",
				"key":          "merchant-secret",
				"payment_mode": "popup",
			},
		},
		OrderNo:         "PGO-EASY-001",
		AmountCNY:       "12.50000",
		Subject:         "自定义充值",
		ClientReturnURL: "https://client.example.com/#/checkout",
	}

	result, err := builder(context.Background(), req, BasePaymentDisplay(req, "easypay_alipay"))
	if err != nil {
		t.Fatalf("BuildEasyPayPaymentDisplay popup returned error: %v", err)
	}
	parsed, err := url.Parse(result.PaymentURL)
	if err != nil {
		t.Fatalf("parse easypay payment url: %v", err)
	}
	query := parsed.Query()
	if parsed.Scheme != "https" || parsed.Host != "pay.example.com" || !strings.HasSuffix(parsed.Path, "/submit.php") {
		t.Fatalf("expected submit.php payment url, got %s", result.PaymentURL)
	}
	if query.Get("pid") != "10001" || query.Get("type") != "alipay" || query.Get("out_trade_no") != "PGO-EASY-001" || query.Get("money") != "12.50000" {
		t.Fatalf("unexpected easypay params: %s", result.PaymentURL)
	}
	if query.Get("notify_url") != "https://pic.example.com/api/open/image/v1/payments/webhooks/easypay_alipay" || query.Get("return_url") != "https://client.example.com/#/checkout" {
		t.Fatalf("unexpected callback params: %s", result.PaymentURL)
	}
	if query.Get("sign") == "" || query.Get("sign_type") != "MD5" {
		t.Fatalf("expected MD5 signature in %s", result.PaymentURL)
	}
	if result.Display["type"] != "redirect" || result.Display["payment_url"] != result.PaymentURL || result.Display["sign_type"] != "MD5" {
		t.Fatalf("unexpected easypay display %#v", result.Display)
	}
}

func TestEasyPayPaymentDisplayBuilderAPIModePostsMAPI(t *testing.T) {
	var upstreamPath string
	var upstreamValues url.Values
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		upstreamValues = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"msg":"success","payurl":"https://pay.example.com/h5/session","qrcode":"https://pay.example.com/qr/session"}`))
	}))
	defer upstream.Close()

	builder := NewEasyPayPaymentDisplayBuilder(CallbackURLConfig{SiteBaseURL: "https://pic.example.com"})
	req := PaymentDisplayRequest{
		Method: domaincashier.VisibleMethod{Method: "wxpay"},
		Instance: domaincashier.ProviderInstance{
			ID:           22,
			ProviderType: "easypay_wxpay",
			Config: map[string]any{
				"gateway_url":  upstream.URL,
				"pid":          "10002",
				"key":          "merchant-secret",
				"payment_mode": "api",
				"client_ip":    "127.0.0.1",
			},
		},
		OrderNo:   "PGO-EASY-API-001",
		AmountCNY: "16.00000",
		Subject:   "自定义充值",
	}

	result, err := builder(context.Background(), req, BasePaymentDisplay(req, "easypay_wxpay"))
	if err != nil {
		t.Fatalf("BuildEasyPayPaymentDisplay api returned error: %v", err)
	}
	if upstreamPath != "/mapi.php" {
		t.Fatalf("expected mapi.php path, got %q", upstreamPath)
	}
	if upstreamValues.Get("pid") != "10002" || upstreamValues.Get("type") != "wxpay" || upstreamValues.Get("out_trade_no") != "PGO-EASY-API-001" || upstreamValues.Get("money") != "16.00000" || upstreamValues.Get("clientip") != "127.0.0.1" {
		t.Fatalf("unexpected mapi params: %#v", upstreamValues)
	}
	if upstreamValues.Get("sign") == "" || upstreamValues.Get("sign_type") != "MD5" {
		t.Fatalf("expected signed mapi params: %#v", upstreamValues)
	}
	if result.PaymentURL != "https://pay.example.com/h5/session" || result.QRCode != "https://pay.example.com/qr/session" {
		t.Fatalf("unexpected api payment result %#v", result)
	}
	if result.Display["type"] != "qr_code" || result.Display["payment_url"] != result.PaymentURL || result.Display["qr_code"] != result.QRCode || result.Display["prepay_mode"] != "api" {
		t.Fatalf("unexpected api display %#v", result.Display)
	}
}
