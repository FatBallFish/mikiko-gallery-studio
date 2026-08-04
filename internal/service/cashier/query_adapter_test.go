package cashier

import (
	"context"
	"encoding/json"
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
		_, _ = w.Write([]byte(`{"code":0,"data":{"state":2,"payOrderId":"P20260804001","amount":1990}}`))
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
	registry.Register("alipay_direct", func(_ context.Context, req QueryOrderStatusRequest) (QueryOrderStatusResult, error) {
		return BuildQueryOrderStatusResult(req.Instance, NormalizeQueryStatus("trade_success"), "ALI-TRADE-001", req.Order.AmountCNY, map[string]any{"source": "test_alipay"}), nil
	})

	result, err := registry.QueryOrderStatus(context.Background(), QueryOrderStatusRequest{
		Order:    OrderSnapshot{OrderNo: "PGO-ALI-001", AmountCNY: "19.90000"},
		Instance: domaincashier.ProviderInstance{ID: 12, ProviderType: "alipay_direct"},
	})
	if err != nil {
		t.Fatalf("QueryOrderStatus returned error: %v", err)
	}
	if !result.Paid || result.QueryStatus != "paid" || result.TradeNo != "ALI-TRADE-001" || result.Raw["source"] != "test_alipay" {
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

func TestQueryAdapterRegistryFallsBackToConfigForUnknownProvider(t *testing.T) {
	registry := NewQueryAdapterRegistry()

	result, err := registry.QueryOrderStatus(context.Background(), QueryOrderStatusRequest{
		Order:    OrderSnapshot{OrderNo: "PGO-CARD-001", AmountCNY: "8.00000", Status: "completed", TradeNo: "LOCAL-TRADE"},
		Instance: domaincashier.ProviderInstance{ID: 77, ProviderType: "cardpay"},
	})
	if err != nil {
		t.Fatalf("QueryOrderStatus returned error: %v", err)
	}
	if !result.Paid || result.QueryStatus != "paid" || result.TradeNo != "LOCAL-TRADE" {
		t.Fatalf("expected unknown provider to use config/local fallback, got %#v", result)
	}
}
