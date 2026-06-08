package cashier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
)

func TestJeePayPaymentDisplayBuilderBuildsSignedUnifiedOrderURL(t *testing.T) {
	builder := NewJeePayPaymentDisplayBuilder(CallbackURLConfig{SiteBaseURL: "https://pic.example.com"})
	req := PaymentDisplayRequest{
		Method: domaincashier.VisibleMethod{Method: "alipay"},
		Instance: domaincashier.ProviderInstance{
			ID:           31,
			ProviderType: "jeepay_alipay",
			Config: map[string]any{
				"gateway_url": "https://jeepay.example.com/api/pay/unifiedOrder?old=1",
				"mch_no":      "MCH10001",
				"app_id":      "APP10001",
				"key":         "merchant-secret",
				"way_code":    "ALI_PC",
			},
		},
		OrderNo:         "PGO-JEEPAY-001",
		AmountCNY:       "12.50000",
		Subject:         "自定义充值",
		ClientReturnURL: "https://client.example.com/#/checkout",
	}

	result, err := builder(context.Background(), req, BasePaymentDisplay(req, "jeepay_alipay"))
	if err != nil {
		t.Fatalf("BuildJeePayPaymentDisplay returned error: %v", err)
	}
	parsed, err := url.Parse(result.PaymentURL)
	if err != nil {
		t.Fatalf("parse jeepay payment url: %v", err)
	}
	query := parsed.Query()
	if parsed.Scheme != "https" || parsed.Host != "jeepay.example.com" || !strings.HasSuffix(parsed.Path, "/api/pay/unifiedOrder") {
		t.Fatalf("expected unifiedOrder URL, got %s", result.PaymentURL)
	}
	if query.Get("mchNo") != "MCH10001" || query.Get("appId") != "APP10001" || query.Get("mchOrderNo") != "PGO-JEEPAY-001" || query.Get("wayCode") != "ALI_PC" || query.Get("amount") != "1250" {
		t.Fatalf("unexpected jeepay params: %s", result.PaymentURL)
	}
	if query.Get("notifyUrl") != "https://pic.example.com/api/open/image/v1/payments/webhooks/jeepay_alipay" || query.Get("returnUrl") != "https://client.example.com/#/checkout" {
		t.Fatalf("unexpected callback params: %s", result.PaymentURL)
	}
	if query.Get("signType") != "MD5" || query.Get("sign") == "" {
		t.Fatalf("expected MD5 signature in %s", result.PaymentURL)
	}
	if result.Display["type"] != "redirect" || result.Display["payment_url"] != result.PaymentURL || result.Display["sign_type"] != "MD5" || result.Display["way_code"] != "ALI_PC" {
		t.Fatalf("unexpected jeepay display %#v", result.Display)
	}
}

func TestJeePayPaymentDisplayBuilderAPIModePostsChannelExtra(t *testing.T) {
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
		_, _ = w.Write([]byte(`{"code":0,"msg":"SUCCESS","data":{"payOrderId":"JEEPAY-API-PAY-001","payUrl":"https://jeepay.example.com/pay/session","codeUrl":"https://jeepay.example.com/qr/session"}}`))
	}))
	defer upstream.Close()

	builder := NewJeePayPaymentDisplayBuilder(CallbackURLConfig{SiteBaseURL: "https://pic.example.com"})
	req := PaymentDisplayRequest{
		Method: domaincashier.VisibleMethod{Method: "wxpay"},
		Instance: domaincashier.ProviderInstance{
			ID:           32,
			ProviderType: "jeepay_wxpay",
			Config: map[string]any{
				"gateway_url":   upstream.URL,
				"mch_no":        "MCH10001",
				"app_id":        "APP10001",
				"key":           "merchant-secret",
				"payment_mode":  "api",
				"way_code":      "WX_JSAPI",
				"client_ip":     "127.0.0.1",
				"channel_extra": map[string]any{"openid": "wx-openid-001", "subAppId": "wx-sub-app"},
			},
		},
		OrderNo:   "PGO-JEEPAY-API-001",
		AmountCNY: "16.00000",
		Subject:   "自定义充值",
	}

	result, err := builder(context.Background(), req, BasePaymentDisplay(req, "jeepay_wxpay"))
	if err != nil {
		t.Fatalf("BuildJeePayPaymentDisplay api returned error: %v", err)
	}
	if upstreamPath != "/api/pay/unifiedOrder" {
		t.Fatalf("expected unifiedOrder path, got %q", upstreamPath)
	}
	if upstreamValues.Get("mchNo") != "MCH10001" || upstreamValues.Get("appId") != "APP10001" || upstreamValues.Get("mchOrderNo") != "PGO-JEEPAY-API-001" || upstreamValues.Get("wayCode") != "WX_JSAPI" || upstreamValues.Get("amount") != "1600" || upstreamValues.Get("clientIp") != "127.0.0.1" {
		t.Fatalf("unexpected unifiedOrder params: %#v", upstreamValues)
	}
	var channelExtra map[string]string
	if err := json.Unmarshal([]byte(upstreamValues.Get("channelExtra")), &channelExtra); err != nil {
		t.Fatalf("expected channelExtra JSON, got %q: %v", upstreamValues.Get("channelExtra"), err)
	}
	if channelExtra["openid"] != "wx-openid-001" || channelExtra["subAppId"] != "wx-sub-app" {
		t.Fatalf("unexpected channelExtra %#v", channelExtra)
	}
	if upstreamValues.Get("signType") != "MD5" || upstreamValues.Get("sign") == "" {
		t.Fatalf("expected signed unifiedOrder params: %#v", upstreamValues)
	}
	if result.PaymentURL != "https://jeepay.example.com/pay/session" || result.QRCode != "https://jeepay.example.com/qr/session" {
		t.Fatalf("unexpected api result %#v", result)
	}
	if result.Display["type"] != "qr_code" || result.Display["payment_url"] != result.PaymentURL || result.Display["qr_code"] != result.QRCode || result.Display["prepay_mode"] != "api" || result.Display["way_code"] != "WX_JSAPI" || result.Display["channel_trade_no"] != "JEEPAY-API-PAY-001" {
		t.Fatalf("unexpected api display %#v", result.Display)
	}
}
