package cashier

import (
	"context"
	"testing"

	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
)

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
	})

	for _, providerType := range []string{"alipay_direct", "easypay_alipay", "easypay_wxpay", "jeepay_alipay", "jeepay_wxpay", "wxpay_direct"} {
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
