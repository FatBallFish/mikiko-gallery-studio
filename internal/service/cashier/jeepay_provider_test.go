package cashier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
)

func TestJeePayUnifiedOrderHasIndependentRequestTimeout(t *testing.T) {
	previousClient := jeepayHTTPClient
	jeepayHTTPClient = &http.Client{Timeout: 25 * time.Millisecond}
	t.Cleanup(func() { jeepayHTTPClient = previousClient })

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"payUrl":"https://jeepay.example.com/late"}}`))
	}))
	defer upstream.Close()

	request := PaymentDisplayRequest{
		Instance: domaincashier.ProviderInstance{ProviderType: "jeepay_alipay", Config: map[string]any{
			"gateway_url": upstream.URL, "mch_no": "MCH10001", "app_id": "APP10001", "key": "merchant-secret",
		}},
		OrderNo: "PGO-JEEPAY-TIMEOUT", AmountCNY: "9.90000", Subject: "Timeout test",
	}
	startedAt := time.Now()
	if _, _, _, _, _, err := BuildJeePayAPIPayment(context.Background(), CallbackURLConfig{}, request); err == nil {
		t.Fatal("expected a stalled JeePay unified-order request to time out")
	}
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("JeePay timeout took too long: %s", elapsed)
	}
}

func TestJeePayPaymentDisplayBuilderDefaultsToAPIPost(t *testing.T) {
	var called bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
			t.Fatalf("expected application/json, got %q", contentType)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode unified order body: %v", err)
		}
		for key, want := range map[string]string{
			"mchNo": "MCH10001", "appId": "APP10001", "mchOrderNo": "PGO-JEEPAY-001",
			"wayCode": "ALI_PC", "amount": "1250", "version": "1.0", "signType": "MD5",
		} {
			if body[key] != want {
				t.Fatalf("unified order %s = %q, want %q; body=%#v", key, body[key], want, body)
			}
		}
		if body["reqTime"] == "" || body["sign"] == "" {
			t.Fatalf("expected reqTime and sign in %#v", body)
		}
		if body["notifyUrl"] != "https://pic.example.com/api/open/image/v1/payments/webhooks/jeepay_alipay" || body["returnUrl"] != "https://client.example.com/#/checkout" {
			t.Fatalf("unexpected callbacks in %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"SUCCESS","data":{"payOrderId":"JEEPAY-PAY-001","payUrl":"https://jeepay.example.com/pay/session"}}`))
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
	if !called {
		t.Fatal("expected JeePay unified order API to be called")
	}
	if result.PaymentURL != "https://jeepay.example.com/pay/session" || result.Display["type"] != "redirect" || result.Display["payment_url"] != result.PaymentURL || result.Display["prepay_mode"] != "api" || result.Display["channel_trade_no"] != "JEEPAY-PAY-001" {
		t.Fatalf("unexpected jeepay display %#v", result.Display)
	}
}

func TestJeePayPaymentDisplayBuilderAPIModePostsChannelExtra(t *testing.T) {
	var upstreamPath string
	var upstreamValues map[string]string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
			t.Fatalf("expected application/json, got %q", contentType)
		}
		if err := json.NewDecoder(r.Body).Decode(&upstreamValues); err != nil {
			t.Fatalf("decode JSON body: %v", err)
		}
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
	if upstreamValues["mchNo"] != "MCH10001" || upstreamValues["appId"] != "APP10001" || upstreamValues["mchOrderNo"] != "PGO-JEEPAY-API-001" || upstreamValues["wayCode"] != "WX_JSAPI" || upstreamValues["amount"] != "1600" || upstreamValues["clientIp"] != "127.0.0.1" {
		t.Fatalf("unexpected unifiedOrder params: %#v", upstreamValues)
	}
	var channelExtra map[string]string
	if err := json.Unmarshal([]byte(upstreamValues["channelExtra"]), &channelExtra); err != nil {
		t.Fatalf("expected channelExtra JSON, got %q: %v", upstreamValues["channelExtra"], err)
	}
	if channelExtra["openid"] != "wx-openid-001" || channelExtra["subAppId"] != "wx-sub-app" {
		t.Fatalf("unexpected channelExtra %#v", channelExtra)
	}
	if upstreamValues["signType"] != "MD5" || upstreamValues["sign"] == "" || upstreamValues["reqTime"] == "" || upstreamValues["version"] != "1.0" {
		t.Fatalf("expected signed unifiedOrder params: %#v", upstreamValues)
	}
	if result.PaymentURL != "https://jeepay.example.com/pay/session" || result.QRCode != "https://jeepay.example.com/qr/session" {
		t.Fatalf("unexpected api result %#v", result)
	}
	if result.Display["type"] != "qr_code" || result.Display["payment_url"] != result.PaymentURL || result.Display["qr_code"] != result.QRCode || result.Display["prepay_mode"] != "api" || result.Display["way_code"] != "WX_JSAPI" || result.Display["channel_trade_no"] != "JEEPAY-API-PAY-001" {
		t.Fatalf("unexpected api display %#v", result.Display)
	}
}

func TestJeePayPaymentDisplayBuilderClassifiesPayData(t *testing.T) {
	tests := []struct {
		name        string
		provider    string
		wayCode     string
		payData     string
		payDataType string
		displayType string
		paymentURL  string
		qrCode      string
	}{
		{name: "browser URL", provider: "jeepay_alipay", wayCode: "ALI_PC", payData: "https://jeepay.example.com/cashier/session", displayType: "redirect", paymentURL: "https://jeepay.example.com/cashier/session"},
		{name: "native QR payload", provider: "jeepay_wxpay", wayCode: "WX_NATIVE", payData: "weixin://wxpay/bizpayurl?pr=jeepay", displayType: "qr_code", qrCode: "weixin://wxpay/bizpayurl?pr=jeepay"},
		{name: "explicit QR image URL", provider: "jeepay_alipay", wayCode: "ALI_QR", payData: "https://jeepay.example.com/qr/session.png", payDataType: "codeUrl", displayType: "qr_code", qrCode: "https://jeepay.example.com/qr/session.png"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"payOrderId": "JEEPAY-PAY-DATA", "payData": tt.payData, "payDataType": tt.payDataType}})
			}))
			defer upstream.Close()

			builder := NewJeePayPaymentDisplayBuilder(CallbackURLConfig{SiteBaseURL: "https://pic.example.com"})
			request := PaymentDisplayRequest{
				Instance: domaincashier.ProviderInstance{ProviderType: tt.provider, Config: map[string]any{
					"gateway_url": upstream.URL, "mch_no": "MCH10001", "app_id": "APP10001", "key": "merchant-secret", "payment_mode": "api", "way_code": tt.wayCode,
				}},
				OrderNo: "PGO-PAY-DATA", AmountCNY: "9.90000", Subject: "Basic Monthly",
			}
			result, err := builder(context.Background(), request, BasePaymentDisplay(request, tt.provider))
			if err != nil {
				t.Fatalf("BuildJeePayPaymentDisplay returned error: %v", err)
			}
			if result.Display["type"] != tt.displayType || result.PaymentURL != tt.paymentURL || result.QRCode != tt.qrCode {
				t.Fatalf("display = %#v, paymentURL=%q qrCode=%q", result.Display, result.PaymentURL, result.QRCode)
			}
		})
	}
}
