package router

import (
	"bytes"
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

func TestAdminAliasRolloutRouteIsRemoved(t *testing.T) {
	cfg := adminConfigAPIConfig()
	authSvc := authservice.NewService(cfg.Auth, map[string]string{"basic": "1.00000"})
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email: "rollout-root@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role: domainadminauth.RoleSuperAdmin, Status: "active",
	}); err != nil {
		t.Fatal(err)
	}
	api := handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, nil, nil, adminauthservice.NewService(cfg.Auth, adminStore), nil)
	handler := NewWithAPI(api)
	rootToken := loginAdminWithCredentials(t, handler, "rollout-root@example.com", "password")
	statusReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/runtime-rollouts/no-copy-reference-aliases", nil)
	statusReq.Header.Set("Authorization", "Bearer "+rootToken)
	statusRec := httptest.NewRecorder()
	handler.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusNotFound {
		t.Fatalf("removed rollout route status=%d body=%s", statusRec.Code, statusRec.Body.String())
	}
}

func TestAdminConfigTabEndpoints(t *testing.T) {
	cfg := adminConfigAPIConfig()
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "admin@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: "super_admin", Status: "active"}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)

	api := handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, nil, nil, adminAuth, nil)
	handler := NewWithAPI(api)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"admin@example.com","password":"password"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected admin login 200, got %d body=%s", loginRec.Code, loginRec.Body.String())
	}
	var loginResp struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(loginRec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/config-tabs", nil)
	listReq.Header.Set("Authorization", "Bearer "+loginResp.Data.AccessToken)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}

	updateBody := bytes.NewBufferString(`{"version":1,"items":[{"config_category":"generation_limits","config_key":"max_image_count","config_value":{"value":3},"scope":"global"}]}`)
	updateReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/config-tabs/generation_limits", updateBody)
	updateReq.Header.Set("Authorization", "Bearer "+loginResp.Data.AccessToken)
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected update 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}

	var updateResp struct {
		Data struct {
			TabKey  string `json:"tab_key"`
			Version int64  `json:"version"`
		} `json:"data"`
	}
	if err := json.NewDecoder(updateRec.Body).Decode(&updateResp); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if updateResp.Data.TabKey != "generation_limits" || updateResp.Data.Version != 2 {
		t.Fatalf("unexpected update response %#v", updateResp)
	}

	listRec = httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list after update 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listResp struct {
		Data struct {
			Items []struct {
				TabKey  string `json:"tab_key"`
				Version int64  `json:"version"`
				Items   []struct {
					ConfigKey   string         `json:"config_key"`
					ConfigValue map[string]any `json:"config_value"`
				} `json:"items"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}

	found := false
	for _, tab := range listResp.Data.Items {
		if tab.TabKey != "generation_limits" {
			continue
		}
		if tab.Version != 2 {
			t.Fatalf("expected generation_limits version 2, got %d", tab.Version)
		}
		for _, item := range tab.Items {
			if item.ConfigKey == "max_image_count" {
				if got := item.ConfigValue["value"]; got != float64(3) {
					t.Fatalf("unexpected config value %#v", item.ConfigValue)
				}
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("expected updated generation_limits item in list response")
	}
}

func TestAdminConfigDangerousTabsRequireSuperAdmin(t *testing.T) {
	cfg := adminConfigAPIConfig()
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "ops@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: domainadminauth.RoleAdmin, Status: "active"}); err != nil {
		t.Fatalf("CreateAdmin ops: %v", err)
	}
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "root@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: domainadminauth.RoleSuperAdmin, Status: "active"}); err != nil {
		t.Fatalf("CreateAdmin root: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)

	api := handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, nil, nil, adminAuth, nil)
	handler := NewWithAPI(api)

	opsToken := loginAdminWithCredentials(t, handler, "ops@example.com", "password")
	rootToken := loginAdminWithCredentials(t, handler, "root@example.com", "password")

	normalReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/config-tabs/generation_limits", bytes.NewBufferString(`{"version":1,"items":[{"config_category":"generation_limits","config_key":"prompt_max_chars","config_value":{"value":3000},"scope":"global"}]}`))
	normalReq.Header.Set("Authorization", "Bearer "+opsToken)
	normalReq.Header.Set("Content-Type", "application/json")
	normalRec := httptest.NewRecorder()
	handler.ServeHTTP(normalRec, normalReq)
	if normalRec.Code != http.StatusOK {
		t.Fatalf("expected admin to update normal config 200, got %d body=%s", normalRec.Code, normalRec.Body.String())
	}

	dangerousReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/config-tabs/payments", bytes.NewBufferString(`{"version":1,"items":[{"config_category":"payments","config_key":"enabled","config_value":{"value":true},"scope":"global"}]}`))
	dangerousReq.Header.Set("Authorization", "Bearer "+opsToken)
	dangerousReq.Header.Set("Content-Type", "application/json")
	dangerousRec := httptest.NewRecorder()
	handler.ServeHTTP(dangerousRec, dangerousReq)
	if dangerousRec.Code != http.StatusForbidden {
		t.Fatalf("expected admin dangerous config update 403, got %d body=%s", dangerousRec.Code, dangerousRec.Body.String())
	}

	rootReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/config-tabs/payments", bytes.NewBufferString(`{"version":1,"items":[{"config_category":"payments","config_key":"enabled","config_value":{"value":true},"scope":"global"}]}`))
	rootReq.Header.Set("Authorization", "Bearer "+rootToken)
	rootReq.Header.Set("Content-Type", "application/json")
	rootRec := httptest.NewRecorder()
	handler.ServeHTTP(rootRec, rootReq)
	if rootRec.Code != http.StatusOK {
		t.Fatalf("expected super_admin dangerous config update 200, got %d body=%s", rootRec.Code, rootRec.Body.String())
	}
}

func TestAdminLoginReturnsPermissions(t *testing.T) {
	cfg := adminConfigAPIConfig()
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "ops-permissions@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: domainadminauth.RoleAdmin, Status: "active"}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	api := handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, nil, nil, adminAuth, nil)
	handler := NewWithAPI(api)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"ops-permissions@example.com","password":"password"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected admin login 200, got %d body=%s", loginRec.Code, loginRec.Body.String())
	}
	var loginResp struct {
		Data struct {
			Permissions []string `json:"permissions"`
		} `json:"data"`
	}
	if err := json.NewDecoder(loginRec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if len(loginResp.Data.Permissions) == 0 {
		t.Fatalf("expected permissions in admin login response")
	}
	if containsString(loginResp.Data.Permissions, string(domainadminauth.PermissionManageDangerousConfig)) {
		t.Fatalf("admin login response must not include dangerous config permission: %#v", loginResp.Data.Permissions)
	}
	if !containsString(loginResp.Data.Permissions, string(domainadminauth.PermissionManageCashier)) {
		t.Fatalf("admin login response should include cashier permission: %#v", loginResp.Data.Permissions)
	}
}

func loginAdminWithCredentials(t *testing.T, handler http.Handler, email, password string) string {
	t.Helper()
	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"`+email+`","password":"`+password+`"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected admin login 200, got %d body=%s", loginRec.Code, loginRec.Body.String())
	}
	var loginResp struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(loginRec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if loginResp.Data.AccessToken == "" {
		t.Fatalf("expected access token in login response")
	}
	return loginResp.Data.AccessToken
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func adminConfigAPIConfig() config.Config {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	cfg.Auth.Issuer = "test"
	cfg.Auth.AccessTokenSecret = "secret"
	return cfg
}
