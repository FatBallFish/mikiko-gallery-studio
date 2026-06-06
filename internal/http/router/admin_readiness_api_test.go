package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
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
