package cashier

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
)

func TestJeePaySignMatchesOfficialVector(t *testing.T) {
	params := map[string]string{
		"mchNo":        "M1682391685",
		"appId":        "6447428682ca7458118af79f",
		"mchOrderNo":   "mho1694051705945",
		"wayCode":      "ALI_BAR",
		"amount":       "1",
		"currency":     "CNY",
		"clientIp":     "192.166.1.132",
		"subject":      "商品标题",
		"body":         "商品描述",
		"notifyUrl":    "https://www.jeequan.com",
		"reqTime":      "1694051706",
		"version":      "1.0",
		"signType":     "MD5",
		"channelExtra": `{"authCode":"284957415846666792"}`,
	}
	const key = "UNpEETkvMpqC9oDLBr9S2X7U92k462h3zhHiy7hj4xbw23PiWhMv6TCAQ2vh8PzynZXZYo9n6puxHkAHG7li6LZi8IpaQrshzydnBll64iKlb4U59ggiyCTaHJeqffiW"
	const want = "924065BA077FA461A9B06D2E76E9ED3C"
	if got := jeepaySign(params, key); got != want {
		t.Fatalf("jeepaySign() = %s, want official vector %s", got, want)
	}
}

func TestJeePayUnifiedOrderUsesNumericContractAndCanonicalSignature(t *testing.T) {
	const merchantKey = "merchant-secret"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode unified order body: %v", err)
		}
		if _, ok := body["amount"].(float64); !ok {
			t.Fatalf("amount must be a JSON number, got %T (%v)", body["amount"], body["amount"])
		}
		if _, ok := body["reqTime"].(float64); !ok {
			t.Fatalf("reqTime must be a JSON number, got %T (%v)", body["reqTime"], body["reqTime"])
		}
		gotSign, _ := body["sign"].(string)
		if wantSign := officialJeePaySignForTest(body, merchantKey); gotSign != wantSign {
			t.Fatalf("sign = %q, want %q; signType and every other non-empty field must be signed", gotSign, wantSign)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"payDataType":"payUrl","payData":"https://jeepay.example.com/pay"}}`))
	}))
	defer upstream.Close()

	req := PaymentDisplayRequest{
		Instance: domaincashier.ProviderInstance{ProviderType: "jeepay_alipay", Config: map[string]any{
			"gateway_url": upstream.URL, "mch_no": "MCH10001", "app_id": "APP10001", "key": merchantKey,
		}},
		OrderNo: "PGO-JEEPAY-CONTRACT", AmountCNY: "9.90000", Subject: "Contract test",
	}
	if _, err := NewJeePayPaymentDisplayBuilder(CallbackURLConfig{})(context.Background(), req, BasePaymentDisplay(req, "jeepay_alipay")); err != nil {
		t.Fatalf("BuildJeePayPaymentDisplay returned error: %v", err)
	}
}

func TestJeePayUnifiedOrderRejectsBusinessCodeOne(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"msg":"FAILED","data":{"payUrl":"https://jeepay.example.com/invalid"}}`))
	}))
	defer upstream.Close()
	req := PaymentDisplayRequest{
		Instance: domaincashier.ProviderInstance{ProviderType: "jeepay_alipay", Config: map[string]any{
			"gateway_url": upstream.URL, "mch_no": "MCH10001", "app_id": "APP10001", "key": "merchant-secret",
		}},
		OrderNo: "PGO-JEEPAY-CODE-ONE", AmountCNY: "9.90000", Subject: "Contract test",
	}
	if _, err := NewJeePayPaymentDisplayBuilder(CallbackURLConfig{})(context.Background(), req, BasePaymentDisplay(req, "jeepay_alipay")); err == nil {
		t.Fatal("expected JeePay business code 1 to be rejected")
	}
}

func TestJeePayUnifiedOrderRejectsFractionalFen(t *testing.T) {
	req := PaymentDisplayRequest{
		Instance: domaincashier.ProviderInstance{ProviderType: "jeepay_alipay", Config: map[string]any{
			"gateway_url": "https://jeepay.example.com", "mch_no": "MCH10001", "app_id": "APP10001", "key": "merchant-secret",
		}},
		OrderNo: "PGO-JEEPAY-FRACTIONAL-FEN", AmountCNY: "12.345", Subject: "Contract test",
	}
	if _, _, _, _, err := BuildJeePayPaymentParams(CallbackURLConfig{}, req); err == nil {
		t.Fatal("expected JeePay amount with fractional fen to be rejected")
	}
}

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
		var rawBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&rawBody); err != nil {
			t.Fatalf("decode unified order body: %v", err)
		}
		body := jeepayStringMapForTest(rawBody)
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
	if _, ok := result.Display["sign"]; ok {
		t.Fatalf("jeepay display must not expose the merchant request signature: %#v", result.Display)
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
		var rawBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&rawBody); err != nil {
			t.Fatalf("decode JSON body: %v", err)
		}
		upstreamValues = jeepayStringMapForTest(rawBody)
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

func TestJeePayALI_PCDefaultChannelExtraRequestsPayURL(t *testing.T) {
	request := PaymentDisplayRequest{
		Instance: domaincashier.ProviderInstance{ProviderType: "jeepay_alipay", Config: map[string]any{
			"gateway_url": "https://jeepay.example.com", "mch_no": "MCH10001", "app_id": "APP10001", "key": "merchant-secret", "way_code": "ALI_PC",
		}},
		OrderNo: "PGO-JEEPAY-ALI-PC", AmountCNY: "9.90000", Subject: "Basic Monthly",
	}

	_, params, sign, _, err := BuildJeePayPaymentParams(CallbackURLConfig{}, request)
	if err != nil {
		t.Fatalf("BuildJeePayPaymentParams returned error: %v", err)
	}
	if got := params["channelExtra"]; got != `{"payDataType":"payUrl"}` {
		t.Fatalf("channelExtra = %q, want payUrl default", got)
	}
	if sign == "" || sign != jeepaySign(params, "merchant-secret") {
		t.Fatalf("default channelExtra must be covered by the request signature: sign=%q params=%#v", sign, params)
	}
}

func TestJeePayWX_NATIVEDefaultChannelExtraRequestsCodeURL(t *testing.T) {
	request := PaymentDisplayRequest{
		Instance: domaincashier.ProviderInstance{ProviderType: "jeepay_wxpay", Config: map[string]any{
			"gateway_url": "https://jeepay.example.com", "mch_no": "MCH10001", "app_id": "APP10001", "key": "merchant-secret", "way_code": "WX_NATIVE",
		}},
		OrderNo: "PGO-JEEPAY-WX-NATIVE", AmountCNY: "9.90000", Subject: "Basic Monthly",
	}

	_, params, _, _, err := BuildJeePayPaymentParams(CallbackURLConfig{}, request)
	if err != nil {
		t.Fatalf("BuildJeePayPaymentParams returned error: %v", err)
	}
	if got := params["channelExtra"]; got != `{"payDataType":"codeUrl"}` {
		t.Fatalf("channelExtra = %q, want codeUrl default", got)
	}
}

func TestJeePayExplicitChannelExtraOverridesDefaultChannelExtra(t *testing.T) {
	const configured = `{"payDataType":"form","scene":"admin-configured"}`
	request := PaymentDisplayRequest{
		Instance: domaincashier.ProviderInstance{ProviderType: "jeepay_alipay", Config: map[string]any{
			"gateway_url": "https://jeepay.example.com", "mch_no": "MCH10001", "app_id": "APP10001", "key": "merchant-secret", "way_code": "ALI_PC", "channel_extra": configured,
		}},
		OrderNo: "PGO-JEEPAY-EXPLICIT", AmountCNY: "9.90000", Subject: "Basic Monthly",
	}

	_, params, _, _, err := BuildJeePayPaymentParams(CallbackURLConfig{}, request)
	if err != nil {
		t.Fatalf("BuildJeePayPaymentParams returned error: %v", err)
	}
	if got := params["channelExtra"]; got != configured {
		t.Fatalf("channelExtra = %q, want explicit configuration %q", got, configured)
	}
}

func TestJeePayHTTPFailureLogsSanitizedDiagnostic(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var rawBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&rawBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		requestBody := jeepayStringMapForTest(rawBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 50301,
			"msg":  "merchant rejected request https://cashier.example.com/pay?token=redirect-secret sign=" + requestBody["sign"] + " key=merchant-secret",
		})
	}))
	defer upstream.Close()

	request := PaymentDisplayRequest{
		Instance: domaincashier.ProviderInstance{ProviderType: "jeepay_alipay", Config: map[string]any{
			"gateway_url": upstream.URL, "mch_no": "MCH10001", "app_id": "APP10001", "key": "merchant-secret", "way_code": "ALI_PC",
		}},
		OrderNo: "PGO-JEEPAY-HTTP-FAIL", AmountCNY: "9.90000", Subject: "Basic Monthly",
	}
	if _, _, _, _, _, err := BuildJeePayAPIPayment(context.Background(), CallbackURLConfig{}, request); err == nil {
		t.Fatal("expected JeePay HTTP failure")
	}

	output := logs.String()
	for _, required := range []string{"stage=http_response", "http_status=503", "message=\"merchant rejected request"} {
		if !strings.Contains(output, required) {
			t.Fatalf("sanitized JeePay diagnostic missing %q: %s", required, output)
		}
	}
	for _, secret := range []string{"merchant-secret", "redirect-secret", "token=", "sign="} {
		if strings.Contains(output, secret) {
			t.Fatalf("sanitized JeePay diagnostic leaked %q: %s", secret, output)
		}
	}
}

func TestJeePayProviderFailureLogsCodeAndBoundedMessage(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 1008, "msg": "merchant configuration rejected"})
	}))
	defer upstream.Close()

	request := PaymentDisplayRequest{
		Instance: domaincashier.ProviderInstance{ProviderType: "jeepay_alipay", Config: map[string]any{
			"gateway_url": upstream.URL, "mch_no": "MCH10001", "app_id": "APP10001", "key": "merchant-secret", "way_code": "ALI_PC",
		}},
		OrderNo: "PGO-JEEPAY-CODE-FAIL", AmountCNY: "9.90000", Subject: "Basic Monthly",
	}
	if _, _, _, _, _, err := BuildJeePayAPIPayment(context.Background(), CallbackURLConfig{}, request); err == nil {
		t.Fatal("expected JeePay provider failure")
	}

	output := logs.String()
	for _, required := range []string{"stage=provider_response", "provider_code=1008", "message=\"merchant configuration rejected\""} {
		if !strings.Contains(output, required) {
			t.Fatalf("JeePay provider diagnostic missing %q: %s", required, output)
		}
	}
	if strings.Contains(output, "merchant-secret") {
		t.Fatalf("JeePay provider diagnostic leaked merchant key: %s", output)
	}
}

func TestJeePayUnifiedOrderRejectsOversizedResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"payOrderId":"JEEPAY-OVERSIZED","payDataType":"payUrl","payData":"https://jeepay.example.com/pay"}}`))
		_, _ = w.Write([]byte(strings.Repeat(" ", (1<<20)+1)))
	}))
	defer upstream.Close()

	req := PaymentDisplayRequest{
		Instance: domaincashier.ProviderInstance{ProviderType: "jeepay_alipay", Config: map[string]any{
			"gateway_url": upstream.URL, "mch_no": "MCH10001", "app_id": "APP10001", "key": "merchant-secret", "way_code": "ALI_PC",
		}},
		OrderNo: "PGO-JEEPAY-OVERSIZED", AmountCNY: "9.90000", Subject: "oversized response",
	}
	if _, _, _, _, _, err := BuildJeePayAPIPayment(context.Background(), CallbackURLConfig{}, req); err == nil {
		t.Fatal("JeePay response larger than the provider response limit must be rejected")
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
		formHTML    string
	}{
		{name: "browser URL", provider: "jeepay_alipay", wayCode: "ALI_PC", payData: "https://jeepay.example.com/cashier/session", payDataType: "payUrl", displayType: "redirect", paymentURL: "https://jeepay.example.com/cashier/session"},
		{name: "native QR payload", provider: "jeepay_wxpay", wayCode: "WX_NATIVE", payData: "weixin://wxpay/bizpayurl?pr=jeepay", displayType: "qr_code", qrCode: "weixin://wxpay/bizpayurl?pr=jeepay"},
		{name: "explicit QR image URL", provider: "jeepay_alipay", wayCode: "ALI_QR", payData: "https://jeepay.example.com/qr/session.png", payDataType: "codeUrl", displayType: "qr_code", qrCode: "https://jeepay.example.com/qr/session.png"},
		{name: "provider form", provider: "jeepay_alipay", wayCode: "ALI_PC", payData: `<form method="post" action="https://jeepay.example.com/cashier"><input name="token" value="opaque"></form>`, payDataType: "form", displayType: "form", formHTML: `<form method="post" action="https://jeepay.example.com/cashier"><input name="token" value="opaque"></form>`},
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
			if got, _ := result.Display["form_html"].(string); got != tt.formHTML {
				t.Fatalf("form_html = %q, want %q", got, tt.formHTML)
			}
		})
	}
}

func TestJeePayPaymentDisplayRejectsUnsupportedAppPayload(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"payOrderId":"JEEPAY-APP-001","payDataType":"wxapp","payData":"{\"appId\":\"wx-app\"}"}}`))
	}))
	defer upstream.Close()

	req := PaymentDisplayRequest{
		Instance: domaincashier.ProviderInstance{ProviderType: "jeepay_wxpay", Config: map[string]any{
			"gateway_url": upstream.URL, "mch_no": "MCH10001", "app_id": "APP10001", "key": "merchant-secret", "way_code": "WX_JSAPI",
		}},
		OrderNo: "PGO-JEEPAY-APP", AmountCNY: "9.90000", Subject: "Basic Monthly",
	}
	if _, err := BuildJeePayPaymentDisplay(context.Background(), CallbackURLConfig{}, req, BasePaymentDisplay(req, "jeepay_wxpay")); err == nil {
		t.Fatal("unsupported JeePay app payload must not be exposed as executable form HTML")
	}
}

func officialJeePaySignForTest(params map[string]any, key string) string {
	keys := make([]string, 0, len(params))
	values := make(map[string]string, len(params))
	for name, raw := range params {
		if name == "sign" || raw == nil {
			continue
		}
		var value string
		switch typed := raw.(type) {
		case string:
			value = typed
		case float64:
			value = strconv.FormatFloat(typed, 'f', -1, 64)
		default:
			value = fmt.Sprint(typed)
		}
		if strings.TrimSpace(value) == "" {
			continue
		}
		keys = append(keys, name)
		values[name] = value
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, name := range keys {
		parts = append(parts, name+"="+values[name])
	}
	sum := md5.Sum([]byte(strings.Join(parts, "&") + "&key=" + key))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func jeepayStringMapForTest(raw map[string]any) map[string]string {
	values := make(map[string]string, len(raw))
	for key, value := range raw {
		switch typed := value.(type) {
		case string:
			values[key] = typed
		case float64:
			values[key] = strconv.FormatFloat(typed, 'f', -1, 64)
		default:
			values[key] = fmt.Sprint(typed)
		}
	}
	return values
}
