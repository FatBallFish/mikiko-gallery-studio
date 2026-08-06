package cashier

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
)

func TestJeePayQueryUsesCompleteCanonicalContract(t *testing.T) {
	const merchantKey = "merchant-secret"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/pay/query" {
			t.Fatalf("unexpected query request %s %s", r.Method, r.URL.Path)
		}
		if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
			t.Fatalf("query content type = %q, want application/json", contentType)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode query JSON: %v", err)
		}
		values := jeepayStringMapForTest(payload)
		for _, required := range []string{"mchNo", "appId", "mchOrderNo", "reqTime", "version", "signType", "sign"} {
			if strings.TrimSpace(values[required]) == "" {
				t.Fatalf("query is missing %s: %#v", required, payload)
			}
		}
		if _, ok := payload["reqTime"].(float64); !ok || len(values["reqTime"]) != 13 || values["version"] != "1.0" || values["signType"] != "MD5" {
			t.Fatalf("unexpected JeePay query contract: %#v", payload)
		}
		if got, want := values["sign"], officialJeePaySignForTest(payload, merchantKey); got != want {
			t.Fatalf("query sign = %s, want %s", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"state":2,"mchNo":"MCH10001","appId":"APP10001","mchOrderNo":"PGO-JEEPAY-QUERY","payOrderId":"P20260804001","amount":1990}}`))
	}))
	defer upstream.Close()

	result, err := JeePayOrderStatusQueryBuilder(context.Background(), QueryOrderStatusRequest{
		Order: OrderSnapshot{OrderNo: "PGO-JEEPAY-QUERY", AmountCNY: "19.90000"},
		Instance: domaincashier.ProviderInstance{ID: 8, ProviderType: "jeepay_alipay", Config: map[string]any{
			"gateway_url": upstream.URL, "mch_no": "MCH10001", "app_id": "APP10001", "key": merchantKey,
		}},
	})
	if err != nil {
		t.Fatalf("JeePayOrderStatusQueryBuilder returned error: %v", err)
	}
	if !result.Paid || result.TradeNo != "P20260804001" || result.AmountCNY != "19.90000" {
		t.Fatalf("unexpected query result %#v", result)
	}
}

func TestJeePayQueryMapsOfficialNumericStates(t *testing.T) {
	tests := []struct {
		state int
		want  string
		paid  bool
	}{
		{state: 0, want: "pending"},
		{state: 1, want: "pending"},
		{state: 2, want: "paid", paid: true},
		{state: 3, want: "failed"},
		{state: 4, want: "closed"},
		{state: 5, want: "refunded"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("state_%d", tt.state), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
					"state": tt.state, "mchNo": "MCH10001", "appId": "APP10001", "mchOrderNo": "PGO-JEEPAY-STATE",
					"payOrderId": "P-JEEPAY-STATE", "amount": 1990,
				}})
			}))
			defer upstream.Close()
			result, err := JeePayOrderStatusQueryBuilder(context.Background(), QueryOrderStatusRequest{
				Order: OrderSnapshot{OrderNo: "PGO-JEEPAY-STATE", AmountCNY: "19.90000"},
				Instance: domaincashier.ProviderInstance{ID: 8, ProviderType: "jeepay_alipay", Config: map[string]any{
					"gateway_url": upstream.URL, "mch_no": "MCH10001", "app_id": "APP10001", "key": "merchant-secret",
				}},
			})
			if err != nil || result.QueryStatus != tt.want || result.Paid != tt.paid {
				t.Fatalf("state %d result=%#v err=%v, want status=%s paid=%t", tt.state, result, err, tt.want, tt.paid)
			}
		})
	}
}

func TestEasyPayQueryPrefersTradeStatusAndSupportsNestedData(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantStatus  string
		wantPaid    bool
		wantTradeNo string
		wantAmount  string
	}{
		{
			name: "waiting trade status overrides numeric paid status", body: `{"code":1,"trade_status":"WAITING","status":1,"out_trade_no":"PGO-EASYPAY-QUERY","pid":"1001","money":"12.34000","trade_no":"gateway-123"}`,
			wantStatus: "pending", wantTradeNo: "gateway-123", wantAmount: "12.34000",
		},
		{
			name: "nested trade success", body: `{"code":1,"data":{"trade_status":"TRADE_SUCCESS","status":0,"out_trade_no":"PGO-EASYPAY-QUERY","pid":"1001","money":"9.99000","trade_no":"data-456"}}`,
			wantStatus: "paid", wantPaid: true, wantTradeNo: "data-456", wantAmount: "9.99000",
		},
		{
			name: "legacy numeric paid", body: `{"code":1,"status":1,"out_trade_no":"PGO-EASYPAY-QUERY","pid":"1001","money":"3.21000","trade_no":"legacy-789"}`,
			wantStatus: "paid", wantPaid: true, wantTradeNo: "legacy-789", wantAmount: "3.21000",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			defer upstream.Close()
			result, err := EasyPayOrderStatusQueryBuilder(context.Background(), QueryOrderStatusRequest{
				Order: OrderSnapshot{OrderNo: "PGO-EASYPAY-QUERY", AmountCNY: tt.wantAmount},
				Instance: domaincashier.ProviderInstance{ID: 9, ProviderType: "easypay_alipay", Config: map[string]any{
					"gateway_url": upstream.URL, "query_url": upstream.URL, "pid": "1001", "key": "secret",
				}},
			})
			if err != nil {
				t.Fatalf("EasyPayOrderStatusQueryBuilder returned error: %v", err)
			}
			if result.QueryStatus != tt.wantStatus || result.Paid != tt.wantPaid || result.TradeNo != tt.wantTradeNo || result.AmountCNY != tt.wantAmount {
				t.Fatalf("unexpected EasyPay query result %#v", result)
			}
		})
	}
}

func TestPaymentQueriesRejectMismatchedResponseIdentity(t *testing.T) {
	t.Run("alipay order", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"alipay_trade_query_response":{"code":"10000","trade_status":"TRADE_SUCCESS","out_trade_no":"PGO-OTHER","trade_no":"ALI-001","total_amount":"19.90000"}}`))
		}))
		defer upstream.Close()
		_, err := AlipayOrderStatusQueryBuilder(context.Background(), QueryOrderStatusRequest{
			Order: OrderSnapshot{OrderNo: "PGO-ALIPAY-BOUND", AmountCNY: "19.90000"},
			Instance: domaincashier.ProviderInstance{ProviderType: "alipay_direct", Config: map[string]any{
				"gateway_url": upstream.URL, "app_id": "app-123", "app_private_key": alipayTestPrivateKeyPEM(t),
			}},
		})
		if err == nil {
			t.Fatal("Alipay response for another merchant order must be rejected")
		}
	})

	t.Run("wxpay merchant and order", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"trade_state":"SUCCESS","out_trade_no":"PGO-OTHER","transaction_id":"WX-001","mchid":"other-mch","appid":"wx-app","amount":{"total":1990}}`))
		}))
		defer upstream.Close()
		_, err := WxPayOrderStatusQueryBuilder(context.Background(), QueryOrderStatusRequest{
			Order: OrderSnapshot{OrderNo: "PGO-WX-BOUND", AmountCNY: "19.90000"},
			Instance: domaincashier.ProviderInstance{ProviderType: "wxpay_direct", Config: map[string]any{
				"gateway_url": upstream.URL, "app_id": "wx-app", "mch_id": "mch-123",
				"merchant_private_key": wxPayTestPrivateKeyPEM(t), "merchant_certificate_serial": "SERIAL-001",
			}},
		})
		if err == nil {
			t.Fatal("WeChat Pay response for another merchant/order must be rejected")
		}
	})

	t.Run("easypay merchant and order", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"code":1,"trade_status":"TRADE_SUCCESS","out_trade_no":"PGO-OTHER","pid":"other-pid","trade_no":"EP-001","money":"19.90000"}`))
		}))
		defer upstream.Close()
		_, err := EasyPayOrderStatusQueryBuilder(context.Background(), QueryOrderStatusRequest{
			Order: OrderSnapshot{OrderNo: "PGO-EASY-BOUND", AmountCNY: "19.90000"},
			Instance: domaincashier.ProviderInstance{ProviderType: "easypay_alipay", Config: map[string]any{
				"gateway_url": upstream.URL, "query_url": upstream.URL, "pid": "1001", "key": "secret",
			}},
		})
		if err == nil {
			t.Fatal("EasyPay response for another merchant/order must be rejected")
		}
	})

	t.Run("jeepay merchant app and order", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"code":0,"data":{"state":2,"mchNo":"OTHER","appId":"APP10001","mchOrderNo":"PGO-OTHER","payOrderId":"JP-001","amount":1990}}`))
		}))
		defer upstream.Close()
		_, err := JeePayOrderStatusQueryBuilder(context.Background(), QueryOrderStatusRequest{
			Order: OrderSnapshot{OrderNo: "PGO-JEEPAY-BOUND", AmountCNY: "19.90000"},
			Instance: domaincashier.ProviderInstance{ProviderType: "jeepay_alipay", Config: map[string]any{
				"gateway_url": upstream.URL, "mch_no": "MCH10001", "app_id": "APP10001", "key": "merchant-secret",
			}},
		})
		if err == nil {
			t.Fatal("JeePay response for another merchant/app/order must be rejected")
		}
	})
}

func TestPaymentQueriesRejectTerminalResponseWithoutBoundIdentity(t *testing.T) {
	tests := []struct {
		name  string
		query func(*httptest.Server) error
		body  string
	}{
		{
			name: "alipay paid",
			body: `{"alipay_trade_query_response":{"code":"10000","trade_status":"TRADE_SUCCESS","trade_no":"ALI-001","total_amount":"19.90000"}}`,
			query: func(upstream *httptest.Server) error {
				_, err := AlipayOrderStatusQueryBuilder(context.Background(), QueryOrderStatusRequest{
					Order: OrderSnapshot{OrderNo: "PGO-ALIPAY-BOUND", AmountCNY: "19.90000"},
					Instance: domaincashier.ProviderInstance{ProviderType: "alipay_direct", Config: map[string]any{
						"gateway_url": upstream.URL, "app_id": "app-123", "app_private_key": alipayTestPrivateKeyPEM(t),
					}},
				})
				return err
			},
		},
		{
			name: "wxpay closed",
			body: `{"trade_state":"CLOSED","transaction_id":"WX-001","amount":{"total":1990}}`,
			query: func(upstream *httptest.Server) error {
				_, err := WxPayOrderStatusQueryBuilder(context.Background(), QueryOrderStatusRequest{
					Order: OrderSnapshot{OrderNo: "PGO-WX-BOUND", AmountCNY: "19.90000"},
					Instance: domaincashier.ProviderInstance{ProviderType: "wxpay_direct", Config: map[string]any{
						"gateway_url": upstream.URL, "app_id": "wx-app", "mch_id": "mch-123",
						"merchant_private_key": wxPayTestPrivateKeyPEM(t), "merchant_certificate_serial": "SERIAL-001",
					}},
				})
				return err
			},
		},
		{
			name: "easypay paid",
			body: `{"code":1,"trade_status":"TRADE_SUCCESS","trade_no":"EP-001","money":"19.90000"}`,
			query: func(upstream *httptest.Server) error {
				_, err := EasyPayOrderStatusQueryBuilder(context.Background(), QueryOrderStatusRequest{
					Order: OrderSnapshot{OrderNo: "PGO-EASY-BOUND", AmountCNY: "19.90000"},
					Instance: domaincashier.ProviderInstance{ProviderType: "easypay_alipay", Config: map[string]any{
						"gateway_url": upstream.URL, "query_url": upstream.URL, "pid": "1001", "key": "secret",
					}},
				})
				return err
			},
		},
		{
			name: "jeepay paid",
			body: `{"code":0,"data":{"state":2,"payOrderId":"JP-001","amount":1990}}`,
			query: func(upstream *httptest.Server) error {
				_, err := JeePayOrderStatusQueryBuilder(context.Background(), QueryOrderStatusRequest{
					Order: OrderSnapshot{OrderNo: "PGO-JEEPAY-BOUND", AmountCNY: "19.90000"},
					Instance: domaincashier.ProviderInstance{ProviderType: "jeepay_alipay", Config: map[string]any{
						"gateway_url": upstream.URL, "mch_no": "MCH10001", "app_id": "APP10001", "key": "merchant-secret",
					}},
				})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			defer upstream.Close()
			if err := tt.query(upstream); err == nil {
				t.Fatal("terminal provider response without bound identity must be rejected")
			}
		})
	}
}

func TestAlipayQueryRejectsProviderBusinessError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"alipay_trade_query_response":{"code":"40004","msg":"Business Failed","sub_code":"ACQ.TRADE_NOT_EXIST"},"sign":"ignored"}`))
	}))
	defer upstream.Close()

	_, err := AlipayOrderStatusQueryBuilder(context.Background(), QueryOrderStatusRequest{
		Order: OrderSnapshot{OrderNo: "PGO-ALIPAY-QUERY-ERROR", AmountCNY: "19.90000"},
		Instance: domaincashier.ProviderInstance{ID: 12, ProviderType: "alipay_direct", Config: map[string]any{
			"gateway_url": upstream.URL, "app_id": "app-123", "app_private_key": alipayTestPrivateKeyPEM(t),
		}},
	})
	if err == nil {
		t.Fatal("alipay business error must not be reported as a pending order")
	}
}

func TestPaymentQueriesRequireExplicitProviderSuccessCode(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	tests := []struct {
		name  string
		query func() error
	}{
		{name: "JeePay", query: func() error {
			_, err := JeePayOrderStatusQueryBuilder(context.Background(), QueryOrderStatusRequest{
				Order: OrderSnapshot{OrderNo: "PGO-JEEPAY-MISSING-CODE", AmountCNY: "19.90000"},
				Instance: domaincashier.ProviderInstance{ID: 8, ProviderType: "jeepay_alipay", Config: map[string]any{
					"gateway_url": upstream.URL, "mch_no": "MCH10001", "app_id": "APP10001", "key": "merchant-secret",
				}},
			})
			return err
		}},
		{name: "Alipay", query: func() error {
			_, err := AlipayOrderStatusQueryBuilder(context.Background(), QueryOrderStatusRequest{
				Order: OrderSnapshot{OrderNo: "PGO-ALIPAY-MISSING-CODE", AmountCNY: "19.90000"},
				Instance: domaincashier.ProviderInstance{ID: 12, ProviderType: "alipay_direct", Config: map[string]any{
					"gateway_url": upstream.URL, "app_id": "app-123", "app_private_key": alipayTestPrivateKeyPEM(t),
				}},
			})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.query(); err == nil {
				t.Fatal("query response without an explicit success code must be rejected")
			}
		})
	}
}

func TestQueryAdapterRegistryBuildsConfigDrivenResult(t *testing.T) {
	registry := NewQueryAdapterRegistry()

	result, err := registry.QueryOrderStatus(context.Background(), QueryOrderStatusRequest{
		Order: OrderSnapshot{OrderNo: "PGO-CONFIG-001", AmountCNY: "12.50000", Status: "pending"},
		Instance: domaincashier.ProviderInstance{ID: 5, ProviderType: "mock", Config: map[string]any{
			"query_status":     "risk_control",
			"query_trade_no":   "MOCK-TRADE-001",
			"query_amount_cny": "12.50000",
		}},
	})
	if err != nil {
		t.Fatalf("QueryOrderStatus returned error: %v", err)
	}
	if result.QueryStatus != "failed" || result.RiskCategory != "risk_control" || result.TradeNo != "MOCK-TRADE-001" {
		t.Fatalf("expected normalized config query result, got %#v", result)
	}
	if result.Raw["source"] != "provider_instance_config" || result.ProviderInstanceID != int64(5) {
		t.Fatalf("expected config raw/provider metadata, got %#v", result)
	}
}

func TestQueryAdapterRegistryDispatchesRegisteredProvider(t *testing.T) {
	registry := NewQueryAdapterRegistry()
	called := false
	registry.Register("alipay_direct", func(_ context.Context, req QueryOrderStatusRequest) (QueryOrderStatusResult, error) {
		called = true
		return BuildQueryOrderStatusResult(req.Instance, NormalizeQueryStatus("trade_success"), "ALI-TRADE-001", req.Order.AmountCNY, map[string]any{"source": "test_alipay"}), nil
	})

	result, err := registry.QueryOrderStatus(context.Background(), QueryOrderStatusRequest{
		Order: OrderSnapshot{OrderNo: "PGO-ALI-001", AmountCNY: "19.90000"},
		Instance: domaincashier.ProviderInstance{ID: 12, ProviderType: "alipay_direct", Config: map[string]any{
			"query_status": "closed",
		}},
	})
	if err != nil {
		t.Fatalf("QueryOrderStatus returned error: %v", err)
	}
	if !called || !result.Paid || result.QueryStatus != "paid" || result.TradeNo != "ALI-TRADE-001" || result.Raw["source"] != "test_alipay" {
		t.Fatalf("expected registered query adapter result, got %#v", result)
	}
}

func TestQueryAdapterRegistryWithBuildersRegistersStandardProviders(t *testing.T) {
	registry := NewQueryAdapterRegistryWithBuilders(QueryProviderBuilders{
		AlipayDirect: func(_ context.Context, req QueryOrderStatusRequest) (QueryOrderStatusResult, error) {
			return BuildQueryOrderStatusResult(req.Instance, NormalizeQueryStatus("paid"), "ALI-TRADE", req.Order.AmountCNY, map[string]any{"source": "alipay"}), nil
		},
		EasyPay: func(_ context.Context, req QueryOrderStatusRequest) (QueryOrderStatusResult, error) {
			return BuildQueryOrderStatusResult(req.Instance, NormalizeQueryStatus("paid"), "EASY-TRADE", req.Order.AmountCNY, map[string]any{"source": "easypay"}), nil
		},
		JeePay: func(_ context.Context, req QueryOrderStatusRequest) (QueryOrderStatusResult, error) {
			return BuildQueryOrderStatusResult(req.Instance, NormalizeQueryStatus("paid"), "JEEPAY-TRADE", req.Order.AmountCNY, map[string]any{"source": "jeepay"}), nil
		},
		WxPayDirect: func(_ context.Context, req QueryOrderStatusRequest) (QueryOrderStatusResult, error) {
			return BuildQueryOrderStatusResult(req.Instance, NormalizeQueryStatus("paid"), "WX-TRADE", req.Order.AmountCNY, map[string]any{"source": "wxpay"}), nil
		},
		Stripe: func(_ context.Context, req QueryOrderStatusRequest) (QueryOrderStatusResult, error) {
			return BuildQueryOrderStatusResult(req.Instance, NormalizeQueryStatus("paid"), "STRIPE-TRADE", req.Order.AmountCNY, map[string]any{"source": "stripe"}), nil
		},
	})

	for _, providerType := range []string{"alipay_direct", "easypay_alipay", "easypay_wxpay", "jeepay_alipay", "jeepay_wxpay", "wxpay_direct", "stripe"} {
		result, err := registry.QueryOrderStatus(context.Background(), QueryOrderStatusRequest{
			Order:    OrderSnapshot{OrderNo: "PGO-QUERY", AmountCNY: "12.50000"},
			Instance: domaincashier.ProviderInstance{ID: 9, ProviderType: providerType},
		})
		if err != nil {
			t.Fatalf("QueryOrderStatus(%s) returned error: %v", providerType, err)
		}
		if !result.Paid || result.TradeNo == "" {
			t.Fatalf("expected standard query provider %s to be registered, got %#v", providerType, result)
		}
	}
}

func TestQueryAdapterRegistryRejectsUnknownRealProvider(t *testing.T) {
	registry := NewQueryAdapterRegistry()

	_, err := registry.QueryOrderStatus(context.Background(), QueryOrderStatusRequest{
		Order:    OrderSnapshot{OrderNo: "PGO-CARD-001", AmountCNY: "8.00000", Status: "completed", TradeNo: "LOCAL-TRADE"},
		Instance: domaincashier.ProviderInstance{ID: 77, ProviderType: "cardpay", Config: map[string]any{"query_status": "closed"}},
	})
	if err == nil {
		t.Fatal("unknown real provider must not synthesize a query result from local config")
	}
}
