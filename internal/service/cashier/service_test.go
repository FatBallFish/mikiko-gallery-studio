package cashier

import (
	"context"
	"strings"
	"testing"

	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
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
