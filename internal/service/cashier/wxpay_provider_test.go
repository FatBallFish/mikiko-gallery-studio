package cashier

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
)

func TestWxPayPaymentDisplayBuilderBuildsNativeQRCode(t *testing.T) {
	privateKey := wxPayTestPrivateKeyPEM(t)
	var upstreamBody struct {
		AppID      string `json:"appid"`
		MchID      string `json:"mchid"`
		OutTradeNo string `json:"out_trade_no"`
		NotifyURL  string `json:"notify_url"`
		Amount     struct {
			Total    int64  `json:"total"`
			Currency string `json:"currency"`
		} `json:"amount"`
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v3/pay/transactions/native" {
			t.Fatalf("expected wxpay native POST, got %s %s", r.Method, r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); !strings.HasPrefix(auth, "WECHATPAY2-SHA256-RSA2048 ") || !strings.Contains(auth, `mchid="mch-123"`) {
			t.Fatalf("expected wxpay authorization header, got %s", auth)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read upstream body: %v", err)
		}
		if err := json.Unmarshal(body, &upstreamBody); err != nil {
			t.Fatalf("decode upstream body: %v body=%s", err, string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code_url":"weixin://wxpay/bizpayurl?pr=native-prepay"}`))
	}))
	defer upstream.Close()

	result := buildWxPayDisplayForTest(t, privateKey, upstream.URL, "native", nil)
	if result.QRCode != "weixin://wxpay/bizpayurl?pr=native-prepay" || result.Display["type"] != "qr_code" || result.Display["prepay_mode"] != "native" {
		t.Fatalf("expected native qr display, got result=%#v display=%#v", result, result.Display)
	}
	if upstreamBody.AppID != "wx-app-123" || upstreamBody.MchID != "mch-123" || upstreamBody.OutTradeNo != "PGO-WXPAY-001" || upstreamBody.Amount.Total != 1250 || upstreamBody.Amount.Currency != "CNY" {
		t.Fatalf("unexpected native upstream body %#v", upstreamBody)
	}
	if upstreamBody.NotifyURL != "https://pic.example.com/api/open/image/v1/payments/webhooks/wxpay_direct" {
		t.Fatalf("unexpected notify_url %q", upstreamBody.NotifyURL)
	}
}

func TestWxPayPaymentDisplayBuilderBuildsH5Redirect(t *testing.T) {
	privateKey := wxPayTestPrivateKeyPEM(t)
	var upstreamBody struct {
		SceneInfo struct {
			PayerClientIP string `json:"payer_client_ip"`
			H5Info        struct {
				Type string `json:"type"`
			} `json:"h5_info"`
		} `json:"scene_info"`
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v3/pay/transactions/h5" {
			t.Fatalf("expected wxpay h5 POST, got %s %s", r.Method, r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read upstream body: %v", err)
		}
		if err := json.Unmarshal(body, &upstreamBody); err != nil {
			t.Fatalf("decode upstream body: %v body=%s", err, string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"h5_url":"https://wx.tenpay.example/h5pay?prepay_id=h5-prepay"}`))
	}))
	defer upstream.Close()

	result := buildWxPayDisplayForTest(t, privateKey, upstream.URL, "h5", nil)
	if result.PaymentURL != "https://wx.tenpay.example/h5pay?prepay_id=h5-prepay" || result.Display["type"] != "redirect" || result.Display["payment_url"] != result.PaymentURL || result.Display["prepay_mode"] != "h5" {
		t.Fatalf("expected h5 redirect display, got result=%#v display=%#v", result, result.Display)
	}
	if upstreamBody.SceneInfo.PayerClientIP != "127.0.0.1" || upstreamBody.SceneInfo.H5Info.Type != "Wap" {
		t.Fatalf("expected h5 scene info, got %#v", upstreamBody.SceneInfo)
	}
}

func TestWxPayPaymentDisplayBuilderBuildsJSAPIToken(t *testing.T) {
	privateKey := wxPayTestPrivateKeyPEM(t)
	var upstreamBody struct {
		Payer struct {
			OpenID string `json:"openid"`
		} `json:"payer"`
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v3/pay/transactions/jsapi" {
			t.Fatalf("expected wxpay jsapi POST, got %s %s", r.Method, r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read upstream body: %v", err)
		}
		if err := json.Unmarshal(body, &upstreamBody); err != nil {
			t.Fatalf("decode upstream body: %v body=%s", err, string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"prepay_id":"wx-jsapi-prepay"}`))
	}))
	defer upstream.Close()

	result := buildWxPayDisplayForTest(t, privateKey, upstream.URL, "jsapi", map[string]any{"openid": "wx-openid-001"})
	var clientToken map[string]string
	if err := json.Unmarshal([]byte(result.ClientToken), &clientToken); err != nil {
		t.Fatalf("decode client token: %v token=%q", err, result.ClientToken)
	}
	if clientToken["appId"] != "wx-app-123" || clientToken["package"] != "prepay_id=wx-jsapi-prepay" || clientToken["signType"] != "RSA" || clientToken["paySign"] == "" {
		t.Fatalf("unexpected jsapi client token %#v", clientToken)
	}
	if result.Display["type"] != "jsapi" || result.Display["client_token"] != result.ClientToken || result.Display["prepay_mode"] != "jsapi" {
		t.Fatalf("expected jsapi display, got result=%#v display=%#v", result, result.Display)
	}
	if upstreamBody.Payer.OpenID != "wx-openid-001" {
		t.Fatalf("expected payer openid, got %#v", upstreamBody.Payer)
	}
}

func TestWxPayAmountRejectsFractionalFen(t *testing.T) {
	if _, err := WxPayAmountFenFromCNY("12.345"); err == nil {
		t.Fatal("expected WeChat Pay amount with fractional fen to be rejected")
	}
	if amount, err := WxPayAmountFenFromCNY("12.34000"); err != nil || amount != 1234 {
		t.Fatalf("expected exact-fen amount 1234, got amount=%d err=%v", amount, err)
	}
}

func buildWxPayDisplayForTest(t *testing.T, privateKey string, gatewayURL string, paymentMode string, extraConfig map[string]any) PaymentDisplayResult {
	t.Helper()
	config := map[string]any{
		"app_id":                      "wx-app-123",
		"mch_id":                      "mch-123",
		"merchant_private_key":        privateKey,
		"merchant_certificate_serial": "MERCHANTSERIAL001",
		"gateway_url":                 gatewayURL,
		"payment_mode":                paymentMode,
		"client_ip":                   "127.0.0.1",
		"h5_type":                     "Wap",
	}
	for key, value := range extraConfig {
		config[key] = value
	}
	req := PaymentDisplayRequest{
		Method: domaincashier.VisibleMethod{Method: "wxpay"},
		Instance: domaincashier.ProviderInstance{
			ID:           21,
			ProviderType: "wxpay_direct",
			Config:       config,
		},
		OrderNo:   "PGO-WXPAY-001",
		AmountCNY: "12.50000",
		Subject:   "Pic Gallery 充值",
	}
	builder := NewWxPayPaymentDisplayBuilder(CallbackURLConfig{SiteBaseURL: "https://pic.example.com"})
	result, err := builder(context.Background(), req, BasePaymentDisplay(req, "wxpay_direct"))
	if err != nil {
		t.Fatalf("BuildWxPayPaymentDisplay returned error: %v", err)
	}
	return result
}

func wxPayTestPrivateKeyPEM(t *testing.T) string {
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
