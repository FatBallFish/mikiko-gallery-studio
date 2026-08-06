package cashier

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
	stripe "github.com/stripe/stripe-go/v85"
)

func TestJeePayClosePostsSignedOfficialJSON(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/pay/close" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		params := jeepayStringMapForTest(payload)
		for key, want := range map[string]string{
			"mchNo": "MCH10001", "appId": "APP10001", "mchOrderNo": "PGO-JEEPAY-CLOSE",
			"version": "1.0", "signType": "MD5",
		} {
			if params[key] != want {
				t.Fatalf("%s=%q want %q payload=%#v", key, params[key], want, params)
			}
		}
		if params["reqTime"] == "" || params["sign"] != jeepaySign(params, "merchant-secret") {
			t.Fatalf("invalid signed payload %#v", params)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "msg": "SUCCESS", "data": map[string]any{"state": 4}})
	}))
	defer upstream.Close()

	result, err := JeePayClosePaymentBuilder(context.Background(), ClosePaymentRequest{
		Order: OrderSnapshot{OrderNo: "PGO-JEEPAY-CLOSE"},
		Instance: domaincashier.ProviderInstance{ID: 31, ProviderType: "jeepay_alipay", Config: map[string]any{
			"gateway_url": upstream.URL, "mch_no": "MCH10001", "app_id": "APP10001", "key": "merchant-secret",
		}},
	})
	if err != nil || !result.Closed || result.ProviderStatus != "4" || result.OutcomeUncertain {
		t.Fatalf("unexpected result %#v err=%v", result, err)
	}
	if raw := result.Raw; raw["sign"] != nil || raw["key"] != nil {
		t.Fatalf("diagnostics leak secrets: %#v", raw)
	}
}

func TestJeePayCloseAcceptsGatewayURLAtCloseEndpoint(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pay/close" {
			t.Fatalf("close path=%q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"state": 4}})
	}))
	defer upstream.Close()
	result, err := JeePayClosePaymentBuilder(context.Background(), ClosePaymentRequest{
		Order: OrderSnapshot{OrderNo: "PGO-JEEPAY-CLOSE-ENDPOINT"},
		Instance: domaincashier.ProviderInstance{ProviderType: "jeepay_alipay", Config: map[string]any{
			"gateway_url": upstream.URL + "/api/pay/close", "mch_no": "MCH10001", "app_id": "APP10001", "key": "merchant-secret",
		}},
	})
	if err != nil || !result.Closed {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestAlipayCloseUsesTradeCloseMethod(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Query().Get("method") != "alipay.trade.close" || r.URL.Query().Get("sign") == "" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
		var biz map[string]string
		if err := json.Unmarshal([]byte(r.URL.Query().Get("biz_content")), &biz); err != nil || biz["out_trade_no"] != "PGO-ALIPAY-CLOSE" {
			t.Fatalf("unexpected biz_content: %v %#v", err, biz)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"alipay_trade_close_response": map[string]any{
			"code": "10000", "msg": "Success", "out_trade_no": "PGO-ALIPAY-CLOSE", "trade_no": "ALI-001",
		}})
	}))
	defer upstream.Close()

	result, err := AlipayClosePaymentBuilder(context.Background(), ClosePaymentRequest{
		Order: OrderSnapshot{OrderNo: "PGO-ALIPAY-CLOSE"},
		Instance: domaincashier.ProviderInstance{ID: 41, ProviderType: "alipay_direct", Config: map[string]any{
			"gateway_url": upstream.URL, "app_id": "app-123", "app_private_key": alipayTestPrivateKeyPEM(t),
		}},
	})
	if err != nil || !result.Closed || result.ProviderStatus != "closed" {
		t.Fatalf("unexpected result %#v err=%v", result, err)
	}
}

func TestWxPayClosePostsSignedMerchantBody(t *testing.T) {
	privateKey := wxPayTestPrivateKeyPEM(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v3/pay/transactions/out-trade-no/PGO-WX-CLOSE/close" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); !strings.HasPrefix(auth, "WECHATPAY2-SHA256-RSA2048 ") || !strings.Contains(auth, `mchid="mch-123"`) {
			t.Fatalf("missing signed authorization: %q", auth)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"mchid":"mch-123"}` {
			t.Fatalf("unexpected body %s", body)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	result, err := WxPayClosePaymentBuilder(context.Background(), ClosePaymentRequest{
		Order: OrderSnapshot{OrderNo: "PGO-WX-CLOSE"},
		Instance: domaincashier.ProviderInstance{ID: 51, ProviderType: "wxpay_direct", Config: map[string]any{
			"gateway_url": upstream.URL, "app_id": "wx-app", "mch_id": "mch-123",
			"merchant_private_key": privateKey, "merchant_certificate_serial": "SERIAL-001",
		}},
	})
	if err != nil || !result.Closed || result.ProviderStatus != "closed" {
		t.Fatalf("unexpected result %#v err=%v", result, err)
	}
}

func TestWxPayCloseRequiresHTTPNoContent(t *testing.T) {
	for _, statusCode := range []int{http.StatusOK, http.StatusAccepted} {
		t.Run(http.StatusText(statusCode), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(statusCode)
			}))
			defer upstream.Close()
			result, err := WxPayClosePaymentBuilder(context.Background(), ClosePaymentRequest{
				Order: OrderSnapshot{OrderNo: "PGO-WX-STRICT"},
				Instance: domaincashier.ProviderInstance{ProviderType: "wxpay_direct", Config: map[string]any{
					"gateway_url": upstream.URL, "app_id": "wx-app", "mch_id": "mch-123",
					"merchant_private_key": wxPayTestPrivateKeyPEM(t), "merchant_certificate_serial": "SERIAL-001",
				}},
			})
			if err == nil || result.Closed || !result.OutcomeUncertain {
				t.Fatalf("HTTP %d must not confirm WeChat close: result=%#v err=%v", statusCode, result, err)
			}
		})
	}
}

type recordingStripePaymentIntentCloser struct {
	id     string
	params *stripe.PaymentIntentCancelParams
	intent *stripe.PaymentIntent
	err    error
}

func (c *recordingStripePaymentIntentCloser) Cancel(id string, params *stripe.PaymentIntentCancelParams) (*stripe.PaymentIntent, error) {
	c.id, c.params = id, params
	return c.intent, c.err
}

func TestStripeCloseCancelsOrderClientToken(t *testing.T) {
	client := &recordingStripePaymentIntentCloser{intent: &stripe.PaymentIntent{
		ID: "pi_client_123", Status: stripe.PaymentIntentStatusCanceled, Metadata: map[string]string{"order_no": "PGO-STRIPE-CLOSE"},
	}}
	builder := newStripeClosePaymentBuilder(func(secret string) StripePaymentIntentCloser {
		if secret != "sk_test_close" {
			t.Fatalf("unexpected secret selector")
		}
		return client
	})
	result, err := builder(context.Background(), ClosePaymentRequest{
		Order:    OrderSnapshot{OrderNo: "PGO-STRIPE-CLOSE", TradeNo: "pi_trade_other", ClientToken: "pi_client_123"},
		Instance: domaincashier.ProviderInstance{ID: 61, ProviderType: "stripe", Config: map[string]any{"secret_key": "sk_test_close"}},
	})
	if err != nil || !result.Closed || client.id != "pi_client_123" || client.params == nil || client.params.Context == nil {
		t.Fatalf("unexpected stripe close client=%#v result=%#v err=%v", client, result, err)
	}
}

func TestEasyPayCloseUsesOnlyExplicitCompatibleEndpoint(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/compatible-close" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		want := url.Values{"act": {"close"}, "pid": {"1001"}, "key": {"secret"}, "out_trade_no": {"PGO-EASYPAY-CLOSE"}}
		if r.PostForm.Encode() != want.Encode() {
			t.Fatalf("unexpected form %s", r.PostForm.Encode())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 1, "status": "closed", "msg": "success", "out_trade_no": "PGO-EASYPAY-CLOSE", "pid": "1001",
		})
	}))
	defer upstream.Close()

	result, err := EasyPayClosePaymentBuilder(context.Background(), ClosePaymentRequest{
		Order: OrderSnapshot{OrderNo: "PGO-EASYPAY-CLOSE"},
		Instance: domaincashier.ProviderInstance{ID: 71, ProviderType: "easypay_alipay", Config: map[string]any{
			"gateway_url": upstream.URL, "close_url": upstream.URL + "/compatible-close", "pid": "1001", "key": "secret",
		}},
	})
	if err != nil || !result.Closed {
		t.Fatalf("unexpected result %#v err=%v", result, err)
	}
}

func TestCloseProvidersRejectUnboundOrAmbiguousSuccess(t *testing.T) {
	t.Run("jeepay missing explicit state", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "msg": "SUCCESS"})
		}))
		defer upstream.Close()
		result, err := JeePayClosePaymentBuilder(context.Background(), ClosePaymentRequest{
			Order: OrderSnapshot{OrderNo: "PGO-JEEPAY-AMBIGUOUS"},
			Instance: domaincashier.ProviderInstance{ProviderType: "jeepay_alipay", Config: map[string]any{
				"gateway_url": upstream.URL, "mch_no": "MCH10001", "app_id": "APP10001", "key": "merchant-secret",
			}},
		})
		if err == nil || result.Closed || !result.OutcomeUncertain {
			t.Fatalf("ambiguous JeePay close must fail closed: result=%#v err=%v", result, err)
		}
	})

	t.Run("easypay different order", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 1, "status": "closed", "out_trade_no": "PGO-OTHER", "pid": "1001",
			})
		}))
		defer upstream.Close()
		result, err := EasyPayClosePaymentBuilder(context.Background(), ClosePaymentRequest{
			Order: OrderSnapshot{OrderNo: "PGO-EASYPAY-BOUND"},
			Instance: domaincashier.ProviderInstance{ProviderType: "easypay_alipay", Config: map[string]any{
				"gateway_url": upstream.URL, "close_url": upstream.URL, "pid": "1001", "key": "secret",
			}},
		})
		if err == nil || result.Closed || !result.OutcomeUncertain {
			t.Fatalf("EasyPay close for another order must fail closed: result=%#v err=%v", result, err)
		}
	})

	t.Run("alipay different order", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"alipay_trade_close_response": map[string]any{
				"code": "10000", "msg": "Success", "out_trade_no": "PGO-OTHER", "trade_no": "ALI-001",
			}})
		}))
		defer upstream.Close()
		result, err := AlipayClosePaymentBuilder(context.Background(), ClosePaymentRequest{
			Order: OrderSnapshot{OrderNo: "PGO-ALIPAY-BOUND"},
			Instance: domaincashier.ProviderInstance{ProviderType: "alipay_direct", Config: map[string]any{
				"gateway_url": upstream.URL, "app_id": "app-123", "app_private_key": alipayTestPrivateKeyPEM(t),
			}},
		})
		if err == nil || result.Closed || !result.OutcomeUncertain {
			t.Fatalf("Alipay close for another order must fail closed: result=%#v err=%v", result, err)
		}
	})

	t.Run("stripe different intent", func(t *testing.T) {
		client := &recordingStripePaymentIntentCloser{intent: &stripe.PaymentIntent{
			ID: "pi_other", Status: stripe.PaymentIntentStatusCanceled, Metadata: map[string]string{"order_no": "PGO-OTHER"},
		}}
		builder := newStripeClosePaymentBuilder(func(string) StripePaymentIntentCloser { return client })
		result, err := builder(context.Background(), ClosePaymentRequest{
			Order:    OrderSnapshot{OrderNo: "PGO-STRIPE-BOUND", ClientToken: "pi_expected"},
			Instance: domaincashier.ProviderInstance{ProviderType: "stripe", Config: map[string]any{"secret_key": "sk_test_close"}},
		})
		if err == nil || result.Closed || !result.OutcomeUncertain {
			t.Fatalf("Stripe close for another intent must fail closed: result=%#v err=%v", result, err)
		}
	})
}
