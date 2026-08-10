package cashier

import (
	"context"
	"strings"
	"testing"
	"time"

	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
	"github.com/shopspring/decimal"
)

func TestServiceScheduleProviderInstanceFiltersEnabledConfiguredAndAmount(t *testing.T) {
	svc := NewService()
	method := domaincashier.VisibleMethod{Method: "alipay", Enabled: true, SourceProviderType: "alipay_direct"}
	instances := []domaincashier.ProviderInstance{
		{ID: 1, ProviderType: "alipay_direct", Enabled: false, SupportedMethods: []string{"alipay"}, ConfigStatus: "configured"},
		{ID: 2, ProviderType: "wxpay_direct", Enabled: true, SupportedMethods: []string{"wxpay"}, ConfigStatus: "configured"},
		{ID: 3, ProviderType: "alipay_direct", Enabled: true, SupportedMethods: []string{"alipay"}, ConfigStatus: "missing"},
		{ID: 4, ProviderType: "alipay_direct", Enabled: true, SupportedMethods: []string{"alipay"}, ConfigStatus: "configured", Limits: map[string]any{"min_amount_cny": "20.00000"}},
		{ID: 5, ProviderType: "alipay_direct", Enabled: true, SupportedMethods: []string{"alipay"}, ConfigStatus: "configured", Limits: map[string]any{"min_amount_cny": "5.00000", "max_amount_cny": "20.00000"}},
	}

	selected, err := svc.ScheduleProviderInstance(context.Background(), method, instances, "10.00000")
	if err != nil {
		t.Fatalf("ScheduleProviderInstance returned error: %v", err)
	}
	if selected.ID != 5 {
		t.Fatalf("expected configured amount-allowed provider instance, got %#v", selected)
	}
}

func TestEligibleProviderInstancesUsesCheckoutMethodEligibility(t *testing.T) {
	method := domaincashier.VisibleMethod{Method: "alipay", Enabled: true, SourceProviderType: "alipay_direct"}
	instances := []domaincashier.ProviderInstance{
		{ID: 1, ProviderType: "alipay_direct", Enabled: true, SupportedMethods: []string{"wxpay"}, ConfigStatus: "configured"},
		{ID: 2, ProviderType: "alipay_direct", Enabled: true, SupportedMethods: []string{"alipay"}, ConfigStatus: "missing"},
		{ID: 3, ProviderType: "wxpay_direct", Enabled: true, SupportedMethods: []string{"alipay"}, ConfigStatus: "configured"},
		{ID: 4, ProviderType: "alipay_direct", Enabled: false, SupportedMethods: []string{"alipay"}, ConfigStatus: "configured"},
		{ID: 5, ProviderType: "alipay_direct", Enabled: true, SupportedMethods: []string{"alipay"}, ConfigStatus: "configured"},
	}
	eligible := EligibleProviderInstances(method, instances)
	if len(eligible) != 1 || eligible[0].ID != 5 {
		t.Fatalf("eligible provider instances = %#v", eligible)
	}
}

func TestServiceScheduleProviderInstanceHonorsDailyAmountLimit(t *testing.T) {
	svc := NewService()
	method := domaincashier.VisibleMethod{Method: "alipay", Enabled: true, SourceProviderType: "alipay_direct"}
	instances := []domaincashier.ProviderInstance{
		{ID: 10, ProviderType: "alipay_direct", Enabled: true, SupportedMethods: []string{"alipay"}, ConfigStatus: "configured", Limits: map[string]any{"daily_amount_limit_cny": "15.00000"}},
		{ID: 20, ProviderType: "alipay_direct", Enabled: true, SupportedMethods: []string{"alipay"}, ConfigStatus: "configured", Limits: map[string]any{"daily_amount_limit_cny": "30.00000"}},
	}

	selected, err := svc.ScheduleProviderInstanceWithDailyUsage(context.Background(), method, instances, "10.00000", map[int64]decimal.Decimal{
		10: decimal.RequireFromString("10.00000"),
		20: decimal.RequireFromString("10.00000"),
	})
	if err != nil {
		t.Fatalf("ScheduleProviderInstanceWithDailyUsage returned error: %v", err)
	}
	if selected.ID != 20 {
		t.Fatalf("expected provider with remaining daily quota, got %#v", selected)
	}
}

func TestServiceScheduleProviderInstanceRoundRobinPersistsCursor(t *testing.T) {
	svc := NewService()
	method := domaincashier.VisibleMethod{Method: "mock", Enabled: true, SourceProviderType: "mock", SchedulerStrategy: "round_robin"}
	instances := []domaincashier.ProviderInstance{
		{ID: 10, ProviderType: "mock", Enabled: true, SupportedMethods: []string{"mock"}},
		{ID: 20, ProviderType: "mock", Enabled: true, SupportedMethods: []string{"mock"}},
	}

	first, err := svc.ScheduleProviderInstance(context.Background(), method, instances, "10.00000")
	if err != nil {
		t.Fatalf("first ScheduleProviderInstance: %v", err)
	}
	second, err := svc.ScheduleProviderInstance(context.Background(), method, instances, "10.00000")
	if err != nil {
		t.Fatalf("second ScheduleProviderInstance: %v", err)
	}
	third, err := svc.ScheduleProviderInstance(context.Background(), method, instances, "10.00000")
	if err != nil {
		t.Fatalf("third ScheduleProviderInstance: %v", err)
	}

	if first.ID != 10 || second.ID != 20 || third.ID != 10 {
		t.Fatalf("expected round robin 10 -> 20 -> 10, got %d -> %d -> %d", first.ID, second.ID, third.ID)
	}
}

func TestServiceScheduleProviderInstanceRejectsUnavailableMethod(t *testing.T) {
	svc := NewService()
	method := domaincashier.VisibleMethod{Method: "alipay", Enabled: false}

	_, err := svc.ScheduleProviderInstance(context.Background(), method, nil, "10.00000")
	if err == nil || !strings.Contains(err.Error(), "payment method is unavailable") {
		t.Fatalf("expected unavailable method error, got %v", err)
	}
}

func TestNormalizeProviderInstanceDefaultsMethodsAndLimits(t *testing.T) {
	now := fixedCashierTime()
	item, err := NormalizeProviderInstance(domaincashier.ProviderInstance{
		ID:           7,
		ProviderType: " wxpay_direct ",
		Name:         "  微信商户 A ",
		Enabled:      true,
		Limits: map[string]any{
			"min_amount_cny": "5",
			"max_amount_cny": "500",
		},
		Config: map[string]any{"api_v3_key": "secret", "mch_id": "mch-1"},
	}, 7, now)
	if err != nil {
		t.Fatalf("NormalizeProviderInstance returned error: %v", err)
	}

	if item.ProviderType != "wxpay_direct" || item.Name != "微信商户 A" {
		t.Fatalf("expected normalized provider type/name, got %#v", item)
	}
	if len(item.SupportedMethods) != 1 || item.SupportedMethods[0] != "wxpay" {
		t.Fatalf("expected wxpay default supported method, got %#v", item.SupportedMethods)
	}
	if item.Limits["min_amount_cny"] != "5.00000" || item.Limits["max_amount_cny"] != "500.00000" {
		t.Fatalf("expected formatted limits, got %#v", item.Limits)
	}
	if item.ConfigStatus != "configured" || item.CreatedAt != now || item.UpdatedAt != now {
		t.Fatalf("expected configured timestamps, got %#v", item)
	}
}

func TestProviderInstancePayloadRedactsSecretConfig(t *testing.T) {
	payload := ProviderInstancePayload(domaincashier.ProviderInstance{
		ID:           9,
		ProviderType: "alipay_direct",
		Name:         "支付宝",
		Enabled:      true,
		Config: map[string]any{
			"app_id":          "app-1",
			"app_private_key": "private-key",
			"gateway_url":     "https://example.test",
		},
		UpdatedAt: fixedCashierTime(),
	})

	config := payload["config"].(map[string]any)
	if config["app_private_key"] != nil {
		t.Fatalf("secret config should be redacted, got %#v", config)
	}
	if config["app_id"] != "app-1" || config["gateway_url"] != "https://example.test" {
		t.Fatalf("non-secret config should remain visible, got %#v", config)
	}
	credentials := payload["credentials_status"].(map[string]any)
	if credentials["has_secret"] != true || credentials["fingerprint"] == "" {
		t.Fatalf("expected secret fingerprint status, got %#v", credentials)
	}
}

func fixedCashierTime() time.Time {
	return time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
}
