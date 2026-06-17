package cashier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
)

func TestJeePayPaymentDisplayBuilderDefaultModePostsUnifiedOrder(t *testing.T) {
	var upstreamPath string
	var upstreamMethod string
	var upstreamValues url.Values
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		upstreamMethod = r.Method
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		upstreamValues = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"SUCCESS","data":{"payOrderId":"JEEPAY-DEFAULT-PAY-001","payDataType":"payurl","payData":"https://jeepay.example.com/pay/session"}}`))
	}))
	defer upstream.Close()

	builder := NewJeePayPaymentDisplayBuilder(CallbackURLConfig{SiteBaseURL: "https://pic.example.com"})
	req := PaymentDisplayRequest{
		Method: domaincashier.VisibleMethod{Method: "alipay"},
		Instance: domaincashier.ProviderInstance{
			ID:           31,
			ProviderType: "jeepay_alipay",
			Config: map[string]any{
				"gateway_url": upstream.URL + "/api/pay/unifiedOrder?old=1",
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
	if upstreamPath != "/api/pay/unifiedOrder" || upstreamMethod != http.MethodPost {
		t.Fatalf("expected default JeePay mode to POST unifiedOrder, got method=%s path=%s payment_url=%s", upstreamMethod, upstreamPath, result.PaymentURL)
	}
	if upstreamValues.Get("mchNo") != "MCH10001" || upstreamValues.Get("appId") != "APP10001" || upstreamValues.Get("mchOrderNo") != "PGO-JEEPAY-001" || upstreamValues.Get("wayCode") != "ALI_PC" || upstreamValues.Get("amount") != "1250" || upstreamValues.Get("version") != "1.0" || upstreamValues.Get("reqTime") == "" {
		t.Fatalf("unexpected jeepay params: %#v", upstreamValues)
	}
	if upstreamValues.Get("notifyUrl") != "https://pic.example.com/api/open/image/v1/payments/webhooks/jeepay_alipay" || upstreamValues.Get("returnUrl") != "https://client.example.com/#/checkout" {
		t.Fatalf("unexpected callback params: %#v", upstreamValues)
	}
	if upstreamValues.Get("signType") != "MD5" || upstreamValues.Get("sign") == "" {
		t.Fatalf("expected MD5 signature in %#v", upstreamValues)
	}
	if result.PaymentURL != "https://jeepay.example.com/pay/session" {
		t.Fatalf("expected payment URL returned by JeePay unifiedOrder, got %#v", result)
	}
	if result.Display["type"] != "redirect" || result.Display["payment_url"] != result.PaymentURL || result.Display["prepay_mode"] != "api" || result.Display["sign_type"] != "MD5" || result.Display["way_code"] != "ALI_PC" || result.Display["channel_trade_no"] != "JEEPAY-DEFAULT-PAY-001" {
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
	if upstreamValues.Get("mchNo") != "MCH10001" || upstreamValues.Get("appId") != "APP10001" || upstreamValues.Get("mchOrderNo") != "PGO-JEEPAY-API-001" || upstreamValues.Get("wayCode") != "WX_JSAPI" || upstreamValues.Get("amount") != "1600" || upstreamValues.Get("clientIp") != "127.0.0.1" || upstreamValues.Get("version") != "1.0" || upstreamValues.Get("reqTime") == "" {
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

func TestJeePayPaymentDisplayBuilderAPIModeKeepsFormPayDataAsFormHTML(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/pay/unifiedOrder" {
			t.Fatalf("expected unifiedOrder POST, got %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"SUCCESS","data":{"payOrderId":"JEEPAY-FORM-PAY-001","payDataType":"form","payData":"<form action=\"https://jeepay.example.com/pay\" method=\"post\"></form>"}}`))
	}))
	defer upstream.Close()

	builder := NewJeePayPaymentDisplayBuilder(CallbackURLConfig{SiteBaseURL: "https://pic.example.com"})
	req := PaymentDisplayRequest{
		Method: domaincashier.VisibleMethod{Method: "alipay"},
		Instance: domaincashier.ProviderInstance{
			ID:           33,
			ProviderType: "jeepay_alipay",
			Config: map[string]any{
				"gateway_url":  upstream.URL,
				"mch_no":       "MCH10001",
				"app_id":       "APP10001",
				"key":          "merchant-secret",
				"payment_mode": "api",
				"way_code":     "ALI_PC",
			},
		},
		OrderNo:   "PGO-JEEPAY-FORM-001",
		AmountCNY: "18.00000",
		Subject:   "自定义充值",
	}

	result, err := builder(context.Background(), req, BasePaymentDisplay(req, "jeepay_alipay"))
	if err != nil {
		t.Fatalf("BuildJeePayPaymentDisplay form returned error: %v", err)
	}
	if result.PaymentURL != "" || result.QRCode != "" {
		t.Fatalf("expected form payData to avoid URL/QR fields, got %#v", result)
	}
	if result.Display["type"] != "form_html" || result.Display["form_html"] == "" || result.Display["payment_url"] != nil || result.Display["channel_trade_no"] != "JEEPAY-FORM-PAY-001" {
		t.Fatalf("expected form_html payment display, got %#v", result.Display)
	}
}
