package cashier

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

func TestConfigFacadeValidatesAndPreservesJeePayProviderConfiguration(t *testing.T) {
	ctx := context.Background()
	facade := NewConfigFacade(NewAdminConfigStore(adminconfigservice.NewServiceWithStore(config.Config{}, adminconfigservice.NewMemoryStore()), false))
	request := domaincashier.ProviderInstanceWriteRequest{
		ProviderInstance: domaincashier.ProviderInstance{
			ProviderType: "jeepay_alipay", Name: "JeePay", Enabled: true,
			SupportedMethods: []string{"alipay"},
			Config:           map[string]any{"gateway_url": "https://pay.example.com", "mch_no": "M100", "app_id": "A100", "way_code": "ALI_PC"},
		},
		Secrets: map[string]any{"key": "signing-secret"},
	}
	created, err := facade.CreateProviderInstance(ctx, request, 99)
	if err != nil {
		t.Fatalf("CreateProviderInstance: %v", err)
	}
	if created.Config["mch_no"] != "M100" || created.Config["app_id"] != "A100" || created.Config["key"] != "signing-secret" {
		t.Fatalf("JeePay configuration was not merged: %#v", created.Config)
	}
	request.ProviderInstance.Name = "JeePay updated"
	request.Secrets = map[string]any{"key": ""}
	updated, err := facade.UpdateProviderInstance(ctx, created.ID, request, 99)
	if err != nil {
		t.Fatalf("UpdateProviderInstance: %v", err)
	}
	if updated.Config["key"] != "signing-secret" {
		t.Fatalf("empty edited key replaced the stored secret: %#v", updated.Config)
	}
	payload := ProviderInstancePayload(updated)
	publicConfig := payload["config"].(map[string]any)
	if publicConfig["mch_no"] != "M100" || publicConfig["app_id"] != "A100" || publicConfig["key"] != nil {
		t.Fatalf("JeePay response redaction is incorrect: %#v", publicConfig)
	}

	for _, missing := range []string{"gateway_url", "mch_no", "app_id", "way_code", "key"} {
		t.Run("missing_"+missing, func(t *testing.T) {
			invalid := request
			invalid.Config = map[string]any{"gateway_url": "https://pay.example.com", "mch_no": "M100", "app_id": "A100", "way_code": "ALI_PC"}
			invalid.Secrets = map[string]any{"key": "signing-secret"}
			if missing == "key" {
				invalid.Secrets = nil
			} else {
				delete(invalid.Config, missing)
			}
			_, err := facade.CreateProviderInstance(ctx, invalid, 99)
			var appErr *errs.Error
			if !errors.As(err, &appErr) || appErr.Code != errs.CodePaymentProviderConfigInvalid || appErr.StatusCode != 400 || !strings.Contains(appErr.Message, missing) {
				t.Fatalf("missing %s error = %#v", missing, err)
			}
			if strings.Contains(appErr.Message, "signing-secret") {
				t.Fatal("provider validation error leaked a submitted value")
			}
		})
	}
}

func TestConfigFacadeReadsAndWritesCashierRuntimeConfig(t *testing.T) {
	ctx := context.Background()
	adminSvc := adminconfigservice.NewServiceWithStore(config.Config{
		Billing: config.BillingConfig{CNYPerPoint: "0.50000"},
	}, adminconfigservice.NewMemoryStore())
	store := NewAdminConfigStoreWithDefaultCNYPerPoint(adminSvc, false, "0.50000")
	facade := NewConfigFacade(store)

	custom, err := facade.CustomAmountConfig(ctx)
	if err != nil {
		t.Fatalf("CustomAmountConfig: %v", err)
	}
	if custom.CNYPerPoint != "0.50000" || !custom.Enabled {
		t.Fatalf("expected default custom amount from admin config, got %#v", custom)
	}

	updatedCustom := domaincashier.CustomAmountConfig{Enabled: true, MinAmountCNY: "3", MaxAmountCNY: "300", CNYPerPoint: "0.75000"}
	custom, err = facade.UpdateCustomAmountConfig(ctx, updatedCustom, 99)
	if err != nil {
		t.Fatalf("UpdateCustomAmountConfig: %v", err)
	}
	if custom.MinAmountCNY != "3.00000" || custom.MaxAmountCNY != "300.00000" || custom.CNYPerPoint != "0.75000" {
		t.Fatalf("expected normalized custom amount config, got %#v", custom)
	}

	methods, err := facade.UpdateVisibleMethods(ctx, []domaincashier.VisibleMethod{
		{Method: "wxpay", Label: "微信", Enabled: true, SourceProviderType: "mock", SchedulerStrategy: "random", DisplayOrder: 20},
		{Method: "alipay", Label: "支付宝", Enabled: true, SourceProviderType: "mock", DisplayOrder: 10},
	}, 99)
	if err != nil {
		t.Fatalf("UpdateVisibleMethods: %v", err)
	}
	if len(methods) != 2 || methods[0].Method != "alipay" || methods[1].SchedulerStrategy != "random" {
		t.Fatalf("expected sorted normalized methods, got %#v", methods)
	}

	instance, err := facade.CreateProviderInstance(ctx, domaincashier.ProviderInstanceWriteRequest{
		ProviderInstance: domaincashier.ProviderInstance{
			ProviderType: "mock",
			Name:         "Mock A",
			Enabled:      true,
			Limits:       map[string]any{"min_amount_cny": "1", "max_amount_cny": "100"},
		},
	}, 99)
	if err != nil {
		t.Fatalf("CreateProviderInstance: %v", err)
	}
	if instance.ID != 2 || instance.ConfigStatus != "configured" || len(instance.SupportedMethods) != 1 || instance.SupportedMethods[0] != "mock" {
		t.Fatalf("expected normalized created instance, got %#v", instance)
	}

	loadedInstances, err := facade.ProviderInstances(ctx)
	if err != nil {
		t.Fatalf("ProviderInstances: %v", err)
	}
	if len(loadedInstances) != 2 || !providerInstanceExists(loadedInstances, instance.ID) {
		t.Fatalf("expected stored provider instance, got %#v", loadedInstances)
	}

	updated, err := facade.UpdateProviderInstance(ctx, instance.ID, domaincashier.ProviderInstanceWriteRequest{
		ProviderInstance: domaincashier.ProviderInstance{
			ProviderType: "mock",
			Name:         "Mock B",
			Enabled:      false,
			Config:       map[string]any{"token": "secret", "public": "visible"},
		},
	}, 99)
	if err != nil {
		t.Fatalf("UpdateProviderInstance: %v", err)
	}
	if updated.Name != "Mock B" || updated.Enabled {
		t.Fatalf("expected updated provider instance, got %#v", updated)
	}
	payload := ProviderInstancePayload(updated)
	configPayload := payload["config"].(map[string]any)
	if configPayload["token"] != nil || configPayload["public"] != "visible" {
		t.Fatalf("expected redacted provider config payload, got %#v", configPayload)
	}

	deleted, err := facade.DeleteProviderInstance(ctx, instance.ID, 99)
	if err != nil {
		t.Fatalf("DeleteProviderInstance: %v", err)
	}
	if deleted.ID != instance.ID || deleted.Name != "Mock B" {
		t.Fatalf("expected deleted provider instance snapshot, got %#v", deleted)
	}
	afterDelete, err := facade.ProviderInstances(ctx)
	if err != nil {
		t.Fatalf("ProviderInstances after delete: %v", err)
	}
	if providerInstanceExists(afterDelete, instance.ID) {
		t.Fatalf("expected deleted provider instance to be removed, got %#v", afterDelete)
	}
}

func TestConfigFacadeCustomAmountFallsBackToProductDefaultCNYPerPoint(t *testing.T) {
	ctx := context.Background()
	adminSvc := adminconfigservice.NewServiceWithStore(config.Config{}, adminconfigservice.NewMemoryStore())
	facade := NewConfigFacade(NewAdminConfigStore(adminSvc, false))

	custom, err := facade.CustomAmountConfig(ctx)
	if err != nil {
		t.Fatalf("CustomAmountConfig: %v", err)
	}
	if custom.CNYPerPoint != "0.31250" {
		t.Fatalf("expected product default CNY per point fallback, got %#v", custom)
	}
}

func TestConfigFacadeKeepsBuiltinMockProviderAvailableInNonProduction(t *testing.T) {
	ctx := context.Background()
	adminSvc := adminconfigservice.NewServiceWithStore(config.Config{}, adminconfigservice.NewMemoryStore())
	facade := NewConfigFacade(NewAdminConfigStore(adminSvc, false))

	alipay, err := facade.CreateProviderInstance(ctx, domaincashier.ProviderInstanceWriteRequest{
		ProviderInstance: domaincashier.ProviderInstance{
			ProviderType:     "alipay_direct",
			Name:             "Alipay Sandbox",
			Enabled:          true,
			SupportedMethods: []string{"alipay"},
			SortOrder:        80,
			SchedulerWeight:  100,
			Limits:           map[string]any{"min_amount_cny": "1.00000", "max_amount_cny": "500.00000"},
			Config:           map[string]any{"app_id": "app", "app_private_key": "secret", "alipay_public_key": "public"},
			ConfigStatus:     "configured",
		},
	}, 99)
	if err != nil {
		t.Fatalf("CreateProviderInstance: %v", err)
	}

	instances, err := facade.ProviderInstances(ctx)
	if err != nil {
		t.Fatalf("ProviderInstances: %v", err)
	}
	if !providerInstanceExists(instances, alipay.ID) {
		t.Fatalf("expected configured alipay instance, got %#v", instances)
	}
	mockCount := 0
	var mock domaincashier.ProviderInstance
	for _, instance := range instances {
		if instance.ProviderType == "mock" {
			mockCount++
			mock = instance
		}
	}
	if mockCount != 1 || mock.ID == alipay.ID || !mock.Enabled || len(mock.SupportedMethods) != 1 || mock.SupportedMethods[0] != "mock" {
		t.Fatalf("expected one non-conflicting builtin mock instance, got count=%d mock=%#v instances=%#v", mockCount, mock, instances)
	}
}

func TestConfigFacadeDoesNotAppendBuiltinMockProviderInProduction(t *testing.T) {
	ctx := context.Background()
	adminSvc := adminconfigservice.NewServiceWithStore(config.Config{}, adminconfigservice.NewMemoryStore())
	facade := NewConfigFacade(NewAdminConfigStore(adminSvc, true))

	instances, err := facade.ProviderInstances(ctx)
	if err != nil {
		t.Fatalf("ProviderInstances: %v", err)
	}
	if len(instances) != 0 {
		t.Fatalf("expected production config to avoid implicit mock provider, got %#v", instances)
	}
}

func providerInstanceExists(instances []domaincashier.ProviderInstance, id int64) bool {
	for _, instance := range instances {
		if instance.ID == id {
			return true
		}
	}
	return false
}

func TestConfigFacadeSchedulerStateRoundTrips(t *testing.T) {
	ctx := context.Background()
	adminSvc := adminconfigservice.NewServiceWithStore(config.Config{}, adminconfigservice.NewMemoryStore())
	facade := NewConfigFacade(NewAdminConfigStore(adminSvc, false))

	if err := facade.SaveSchedulerState(ctx, map[string]map[string]any{
		"alipay:mock": {"last_instance_id": int64(12), "at": time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC).Format(time.RFC3339)},
	}, 7); err != nil {
		t.Fatalf("SaveSchedulerState: %v", err)
	}

	state, err := facade.SchedulerState(ctx)
	if err != nil {
		t.Fatalf("SchedulerState: %v", err)
	}
	if state["alipay:mock"]["last_instance_id"] != float64(12) && state["alipay:mock"]["last_instance_id"] != int64(12) {
		t.Fatalf("expected persisted scheduler state, got %#v", state)
	}
}
