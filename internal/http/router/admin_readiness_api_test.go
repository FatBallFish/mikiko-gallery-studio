package router

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
	domainmodeladmin "github.com/fatballfish/pic-gallery/internal/domain/modeladmin"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	modeladminservice "github.com/fatballfish/pic-gallery/internal/service/modeladmin"
)

func TestAdminReadinessEndpointReturnsOperationalChecks(t *testing.T) {
	cfg := adminConfigAPIConfig()
	cfg.Billing.SignupTrial = config.SignupTrialConfig{Enabled: true, Points: "15.00000", ValidDays: 7, ExpiryReminderDays: 2, GrantOncePerUser: true}
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "ready@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: domainadminauth.RoleAdmin, Status: "active"}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	api := handlers.NewAPIWithModelAdminService(cfg, authSvc, nil, nil, nil, nil, nil, adminAuth, nil, nil, nil, nil, nil)
	handler := NewWithAPI(api)
	token := loginAdminWithCredentials(t, handler, "ready@example.com", "password")

	req := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/readiness", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected readiness 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data struct {
			Status        string `json:"status"`
			OverallStatus string `json:"overall_status"`
			Summary       struct {
				Pass int `json:"pass"`
				Warn int `json:"warn"`
				Fail int `json:"fail"`
			} `json:"summary"`
			Checks []struct {
				Key         string `json:"key"`
				Label       string `json:"label"`
				Status      string `json:"status"`
				Detail      string `json:"detail"`
				Summary     string `json:"summary"`
				FixRoute    string `json:"fix_route"`
				ActionRoute string `json:"action_route"`
			} `json:"checks"`
			Items []struct {
				Key    string `json:"key"`
				Status string `json:"status"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode readiness response: %v", err)
	}
	if resp.Data.Status != "fail" || resp.Data.OverallStatus != "fail" {
		t.Fatalf("expected failing readiness status for empty model config, got status=%q overall=%q", resp.Data.Status, resp.Data.OverallStatus)
	}
	if len(resp.Data.Checks) == 0 || len(resp.Data.Items) != len(resp.Data.Checks) {
		t.Fatalf("expected checks and items aliases, got checks=%d items=%d", len(resp.Data.Checks), len(resp.Data.Items))
	}
	if resp.Data.Summary.Fail == 0 {
		t.Fatalf("expected fail count in summary, got %#v", resp.Data.Summary)
	}

	checks := map[string]struct {
		Status      string
		Detail      string
		Summary     string
		FixRoute    string
		ActionRoute string
	}{}
	for _, item := range resp.Data.Checks {
		checks[item.Key] = struct {
			Status      string
			Detail      string
			Summary     string
			FixRoute    string
			ActionRoute string
		}{Status: item.Status, Detail: item.Detail, Summary: item.Summary, FixRoute: item.FixRoute, ActionRoute: item.ActionRoute}
	}
	for _, key := range []string{"model_accounts", "provider_models", "route_models", "route_candidates", "route_prices", "payments", "signup_trial", "public_gallery", "docs"} {
		if _, ok := checks[key]; !ok {
			t.Fatalf("expected readiness check %q in %#v", key, checks)
		}
	}
	if checks["model_accounts"].Status != "fail" || checks["model_accounts"].ActionRoute == "" {
		t.Fatalf("expected model_accounts fail with action route, got %#v", checks["model_accounts"])
	}
	if checks["signup_trial"].Status != "pass" {
		t.Fatalf("expected signup_trial pass, got %#v", checks["signup_trial"])
	}
	if checks["docs"].Status != "pass" {
		t.Fatalf("expected docs pass, got %#v", checks["docs"])
	}
}

func TestAdminReadinessCountsEnabledModelAccountModelsAsRealModels(t *testing.T) {
	cfg := adminConfigAPIConfig()
	cfg.Billing.SignupTrial = config.SignupTrialConfig{Enabled: true, Points: "15.00000", ValidDays: 7, ExpiryReminderDays: 2, GrantOncePerUser: true}
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "ready-models@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: domainadminauth.RoleAdmin, Status: "active"}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	modelAdminSvc := modeladminservice.NewServiceWithStore(nil)
	account, err := modelAdminSvc.CreateModelAccount(t.Context(), domainmodeladmin.ModelAccountWriteRequest{
		Name:             "OpenAI Main",
		AdapterType:      domainmodeladmin.AdapterTypeOpenAICompatible,
		AuthType:         domainmodeladmin.AuthTypeAPIKey,
		BaseURL:          "https://api.openai.com",
		Credentials:      map[string]string{"api_key": "sk-test"},
		Status:           domainmodeladmin.ModelAccountStatusEnabled,
		Priority:         1,
		Weight:           100,
		ConcurrencyLimit: 5,
		TimeoutMS:        120000,
	})
	if err != nil {
		t.Fatalf("CreateModelAccount: %v", err)
	}
	if _, err := modelAdminSvc.CreateModelAccountModel(t.Context(), domainmodeladmin.ModelAccountModelWriteRequest{
		AccountID:      account.ID,
		ModelCode:      "gpt-image-1",
		DisplayName:    "GPT Image 1",
		TaskTypes:      []string{"text_to_image"},
		BaseResolution: []string{"auto", "1K"},
		CostPerImage:   "0.04000",
		Currency:       "USD",
		Enabled:        true,
	}); err != nil {
		t.Fatalf("CreateModelAccountModel: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	api := handlers.NewAPIWithModelAdminService(cfg, authSvc, nil, nil, nil, nil, nil, adminAuth, nil, nil, nil, nil, modelAdminSvc)
	handler := NewWithAPI(api)
	token := loginAdminWithCredentials(t, handler, "ready-models@example.com", "password")

	req := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/readiness", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected readiness 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Checks []struct {
				Key    string `json:"key"`
				Status string `json:"status"`
				Detail string `json:"detail"`
			} `json:"checks"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode readiness response: %v", err)
	}
	for _, check := range resp.Data.Checks {
		if check.Key != "provider_models" {
			continue
		}
		if check.Status != "pass" || !strings.Contains(check.Detail, "1 个已启用真实模型") {
			t.Fatalf("expected real model readiness to count enabled model account models, got %#v", check)
		}
		return
	}
	t.Fatalf("expected provider_models readiness check in %#v", resp.Data.Checks)
}

func TestAdminReadinessUsesAdminSignupTrialConfig(t *testing.T) {
	cfg := adminConfigAPIConfig()
	cfg.Billing.SignupTrial = config.SignupTrialConfig{Enabled: false, Points: "0.00000", ValidDays: 0, ExpiryReminderDays: 1, GrantOncePerUser: true}
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	adminCfgSvc := adminconfigservice.NewService(cfg)
	if _, err := adminCfgSvc.UpdateTab(context.Background(), domainadminconfig.UpdateTabRequest{
		TabKey:  "trial_credits",
		Version: 1,
		Items: []domainadminconfig.Item{{
			ConfigCategory: "billing_trial",
			ConfigKey:      "signup_trial",
			ConfigValue: map[string]any{"value": map[string]any{
				"enabled":              true,
				"points":               "18.00000",
				"valid_days":           9,
				"expiry_reminder_days": 4,
				"grant_once_per_user":  true,
			}},
			Scope: "global",
		}},
	}); err != nil {
		t.Fatalf("UpdateTab trial_credits: %v", err)
	}
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "ready-trial@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: domainadminauth.RoleAdmin, Status: "active"}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	api := handlers.NewAPIWithModelAdminService(cfg, authSvc, nil, nil, adminCfgSvc, nil, nil, adminAuth, nil, nil, nil, nil, nil)
	handler := NewWithAPI(api)
	token := loginAdminWithCredentials(t, handler, "ready-trial@example.com", "password")

	req := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/readiness", bytes.NewReader(nil))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected readiness 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Checks []struct {
				Key    string `json:"key"`
				Status string `json:"status"`
				Detail string `json:"detail"`
			} `json:"checks"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode readiness response: %v", err)
	}
	for _, check := range resp.Data.Checks {
		if check.Key != "signup_trial" {
			continue
		}
		if check.Status != "pass" || !strings.Contains(check.Detail, "18.00000") || !strings.Contains(check.Detail, "9 天") {
			t.Fatalf("expected readiness to use admin signup trial config, got %#v", check)
		}
		return
	}
	t.Fatalf("expected signup_trial readiness check in %#v", resp.Data.Checks)
}

func TestAdminReadinessChecksDocsContractInsteadOfOnlyRegisteredRoutes(t *testing.T) {
	cfg := adminConfigAPIConfig()
	cfg.Billing.SignupTrial = config.SignupTrialConfig{Enabled: true, Points: "15.00000", ValidDays: 7, ExpiryReminderDays: 2, GrantOncePerUser: true}
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "ready-docs@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: domainadminauth.RoleAdmin, Status: "active"}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	api := handlers.NewAPIWithModelAdminService(cfg, authSvc, nil, nil, nil, nil, nil, adminAuth, nil, nil, nil, nil, nil)
	api.SetDocsReadinessChecker(func(context.Context) handlers.DocsReadinessResult {
		return handlers.DocsReadinessResult{Status: "fail", Detail: "OpenAPI JSON 解析失败"}
	})
	handler := NewWithAPI(api)
	token := loginAdminWithCredentials(t, handler, "ready-docs@example.com", "password")

	req := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/readiness", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected readiness 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data struct {
			Checks []struct {
				Key    string `json:"key"`
				Status string `json:"status"`
				Detail string `json:"detail"`
			} `json:"checks"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode readiness response: %v", err)
	}
	for _, check := range resp.Data.Checks {
		if check.Key != "docs" {
			continue
		}
		if check.Status != "fail" || !strings.Contains(check.Detail, "OpenAPI JSON") {
			t.Fatalf("expected docs readiness to surface checker failure, got %#v", check)
		}
		return
	}
	t.Fatalf("expected docs readiness check in %#v", resp.Data.Checks)
}
