package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	domainaudit "github.com/fatballfish/pic-gallery/internal/domain/audit"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
	auditservice "github.com/fatballfish/pic-gallery/internal/service/audit"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
)

func TestAdminAliasRolloutRequiresDangerousPermissionAndExplicitCohortAttestation(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, "file:admin-alias-rollout-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	cfg := adminConfigAPIConfig()
	authSvc := authservice.NewService(cfg.Auth, map[string]string{"basic": "1.00000"})
	adminStore := adminauthservice.NewMemoryStore()
	for _, admin := range []domainadminauth.AdminUser{
		{Email: "rollout-ops@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: domainadminauth.RoleAdmin, Status: "active"},
		{Email: "rollout-root@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: domainadminauth.RoleSuperAdmin, Status: "active"},
	} {
		if _, err := adminStore.CreateAdmin(t.Context(), admin); err != nil {
			t.Fatal(err)
		}
	}
	auditSvc := auditservice.NewService(auditservice.NewMemoryStore())
	api := handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, nil, nil, adminauthservice.NewService(cfg.Auth, adminStore), auditSvc)
	rollout := entstore.NewAliasRolloutStore(client)
	api.SetAliasRolloutStore(rollout)
	handler := NewWithAPI(api)
	opsToken := loginAdminWithCredentials(t, handler, "rollout-ops@example.com", "password")
	rootToken := loginAdminWithCredentials(t, handler, "rollout-root@example.com", "password")
	statusReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/runtime-rollouts/no-copy-reference-aliases", nil)
	statusReq.Header.Set("Authorization", "Bearer "+rootToken)
	statusRec := httptest.NewRecorder()
	handler.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK || !strings.Contains(statusRec.Body.String(), `"enabled":false`) || !strings.Contains(statusRec.Body.String(), `"version":0`) {
		t.Fatalf("initial rollout status=%d body=%s", statusRec.Code, statusRec.Body.String())
	}

	request := func(token, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/runtime-rollouts/no-copy-reference-aliases", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	if rec := request(opsToken, `{"enabled":true,"expected_version":0,"all_api_nodes_cleanup_aware":true}`); rec.Code != http.StatusForbidden {
		t.Fatalf("ordinary admin activation status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := request(rootToken, `{"enabled":true,"expected_version":0}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("activation without cohort attestation status=%d body=%s", rec.Code, rec.Body.String())
	}
	activated := request(rootToken, `{"enabled":true,"expected_version":0,"all_api_nodes_cleanup_aware":true}`)
	if activated.Code != http.StatusOK || !strings.Contains(activated.Body.String(), `"enabled":true`) || !strings.Contains(activated.Body.String(), `"version":1`) {
		t.Fatalf("activation status=%d body=%s", activated.Code, activated.Body.String())
	}
	if rec := request(rootToken, `{"enabled":false,"expected_version":0}`); rec.Code != http.StatusConflict {
		t.Fatalf("stale rollback status=%d body=%s", rec.Code, rec.Body.String())
	}
	disabled := request(rootToken, `{"enabled":false,"expected_version":1}`)
	if disabled.Code != http.StatusOK || !strings.Contains(disabled.Body.String(), `"enabled":false`) || !strings.Contains(disabled.Body.String(), `"version":2`) {
		t.Fatalf("rollback status=%d body=%s", disabled.Code, disabled.Body.String())
	}
	audits, err := auditSvc.List(t.Context(), domainaudit.ListRequest{Action: "runtime_rollout.alias_creation.disable", Page: 1, PageSize: 10})
	if err != nil || len(audits.Items) != 1 || audits.Items[0].TargetID != "no_copy_reference_aliases" {
		t.Fatalf("rollout audit=%#v err=%v", audits, err)
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
