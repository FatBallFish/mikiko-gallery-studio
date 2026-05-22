package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
	adminuserservice "github.com/fatballfish/pic-gallery/internal/service/adminuser"
	auditservice "github.com/fatballfish/pic-gallery/internal/service/audit"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	modeladminservice "github.com/fatballfish/pic-gallery/internal/service/modeladmin"
	_ "github.com/mattn/go-sqlite3"
)

func TestAdminModelManagementEndpoints(t *testing.T) {
	cfg := adminConfigAPIConfig()
	client, err := repoent.Open(dialect.SQLite, "file:admin-model-api?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	authSvc := authservice.NewServiceWithStore(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"}, entstore.NewAuthStore(client))
	adminStore := entstore.NewAdminAuthStore(client)
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "admin-model@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: "super_admin", Status: "active"}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	billingStore := entstore.NewBillingStore(client, 5)
	billingSvc := billingservice.NewServiceWithStore(cfg.Billing, billingStore)
	adminUsers := adminuserservice.NewServiceWithStore(entstore.NewAdminUserStore(client, billingStore), billingSvc)
	auditSvc := auditservice.NewService(entstore.NewAuditStore(client))
	modelAdminSvc := modeladminservice.NewServiceWithStore(entstore.NewModelAdminStore(client))
	api := handlers.NewAPIWithModelAdminService(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, auditSvc, adminUsers, nil, nil, modelAdminSvc)
	handler := NewWithAPI(api)

	adminToken := loginAdminForModelTest(t, handler)

	createProviderReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/model-providers", bytes.NewBufferString(`{"provider_code":"OpenAI","provider_type":"openai","auth_config_encrypted":"cipher","health_status":"healthy","enabled":true}`))
	createProviderReq.Header.Set("Authorization", "Bearer "+adminToken)
	createProviderReq.Header.Set("Content-Type", "application/json")
	createProviderRec := httptest.NewRecorder()
	handler.ServeHTTP(createProviderRec, createProviderReq)
	if createProviderRec.Code != http.StatusCreated {
		t.Fatalf("expected provider create 201, got %d body=%s", createProviderRec.Code, createProviderRec.Body.String())
	}
	var providerResp struct {
		Data struct {
			ID           int64  `json:"id"`
			ProviderCode string `json:"provider_code"`
			Enabled      bool   `json:"enabled"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createProviderRec.Body).Decode(&providerResp); err != nil {
		t.Fatalf("decode provider response: %v", err)
	}
	if providerResp.Data.ID <= 0 || providerResp.Data.ProviderCode != "openai" || !providerResp.Data.Enabled {
		t.Fatalf("unexpected provider response %#v", providerResp)
	}

	listProviderReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/model-providers?page=1&page_size=10&provider_type=openai&enabled=true", nil)
	listProviderReq.Header.Set("Authorization", "Bearer "+adminToken)
	listProviderRec := httptest.NewRecorder()
	handler.ServeHTTP(listProviderRec, listProviderReq)
	if listProviderRec.Code != http.StatusOK || !bytes.Contains(listProviderRec.Body.Bytes(), []byte("openai")) {
		t.Fatalf("expected provider list 200 with openai, got %d body=%s", listProviderRec.Code, listProviderRec.Body.String())
	}

	updateProviderReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/model-providers/openai", bytes.NewBufferString(`{"provider_code":"openai","provider_type":"openai","auth_config_encrypted":"cipher-2","health_status":"degraded","enabled":false}`))
	updateProviderReq.Header.Set("Authorization", "Bearer "+adminToken)
	updateProviderReq.Header.Set("Content-Type", "application/json")
	updateProviderRec := httptest.NewRecorder()
	handler.ServeHTTP(updateProviderRec, updateProviderReq)
	if updateProviderRec.Code != http.StatusOK {
		t.Fatalf("expected provider update 200, got %d body=%s", updateProviderRec.Code, updateProviderRec.Body.String())
	}

	createRouteReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/model-routes", bytes.NewBufferString(`{"group_code":"Plus","task_type":"text_to_image","provider_code":"openai","priority":1,"weight_percent":100,"fallback_order":1,"enabled":true}`))
	createRouteReq.Header.Set("Authorization", "Bearer "+adminToken)
	createRouteReq.Header.Set("Content-Type", "application/json")
	createRouteRec := httptest.NewRecorder()
	handler.ServeHTTP(createRouteRec, createRouteReq)
	if createRouteRec.Code != http.StatusCreated {
		t.Fatalf("expected route create 201, got %d body=%s", createRouteRec.Code, createRouteRec.Body.String())
	}
	var routeResp struct {
		Data struct {
			ID           int64  `json:"id"`
			GroupCode    string `json:"group_code"`
			TaskType     string `json:"task_type"`
			ProviderCode string `json:"provider_code"`
			Enabled      bool   `json:"enabled"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRouteRec.Body).Decode(&routeResp); err != nil {
		t.Fatalf("decode route response: %v", err)
	}
	if routeResp.Data.ID <= 0 || routeResp.Data.GroupCode != "plus" || routeResp.Data.ProviderCode != "openai" || !routeResp.Data.Enabled {
		t.Fatalf("unexpected route response %#v", routeResp)
	}

	listRouteReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/model-routes?page=1&page_size=10&group_code=plus&task_type=text_to_image&provider_code=openai&enabled=true", nil)
	listRouteReq.Header.Set("Authorization", "Bearer "+adminToken)
	listRouteRec := httptest.NewRecorder()
	handler.ServeHTTP(listRouteRec, listRouteReq)
	if listRouteRec.Code != http.StatusOK || !bytes.Contains(listRouteRec.Body.Bytes(), []byte("text_to_image")) {
		t.Fatalf("expected route list 200 with route, got %d body=%s", listRouteRec.Code, listRouteRec.Body.String())
	}

	updateRouteReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/model-routes/"+jsonNumber(routeResp.Data.ID), bytes.NewBufferString(`{"group_code":"plus","task_type":"text_to_image","provider_code":"openai","priority":2,"weight_percent":50,"fallback_order":2,"enabled":false}`))
	updateRouteReq.Header.Set("Authorization", "Bearer "+adminToken)
	updateRouteReq.Header.Set("Content-Type", "application/json")
	updateRouteRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRouteRec, updateRouteReq)
	if updateRouteRec.Code != http.StatusOK {
		t.Fatalf("expected route update 200, got %d body=%s", updateRouteRec.Code, updateRouteRec.Body.String())
	}

	deleteProviderWithRouteReq := httptest.NewRequest(http.MethodDelete, "/api/ops/admin/v1/model-providers/openai", nil)
	deleteProviderWithRouteReq.Header.Set("Authorization", "Bearer "+adminToken)
	deleteProviderWithRouteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteProviderWithRouteRec, deleteProviderWithRouteReq)
	if deleteProviderWithRouteRec.Code != http.StatusConflict {
		t.Fatalf("expected provider delete with route 409, got %d body=%s", deleteProviderWithRouteRec.Code, deleteProviderWithRouteRec.Body.String())
	}

	deleteRouteReq := httptest.NewRequest(http.MethodDelete, "/api/ops/admin/v1/model-routes/"+jsonNumber(routeResp.Data.ID), nil)
	deleteRouteReq.Header.Set("Authorization", "Bearer "+adminToken)
	deleteRouteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRouteRec, deleteRouteReq)
	if deleteRouteRec.Code != http.StatusNoContent {
		t.Fatalf("expected route delete 204, got %d body=%s", deleteRouteRec.Code, deleteRouteRec.Body.String())
	}

	deleteProviderReq := httptest.NewRequest(http.MethodDelete, "/api/ops/admin/v1/model-providers/openai", nil)
	deleteProviderReq.Header.Set("Authorization", "Bearer "+adminToken)
	deleteProviderRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteProviderRec, deleteProviderReq)
	if deleteProviderRec.Code != http.StatusNoContent {
		t.Fatalf("expected provider delete 204, got %d body=%s", deleteProviderRec.Code, deleteProviderRec.Body.String())
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/audit-logs", nil)
	auditReq.Header.Set("Authorization", "Bearer "+adminToken)
	auditRec := httptest.NewRecorder()
	handler.ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit list 200, got %d body=%s", auditRec.Code, auditRec.Body.String())
	}
	for _, action := range []string{"model_provider.create", "model_provider.update", "model_provider.delete", "model_route.create", "model_route.update", "model_route.delete"} {
		if !bytes.Contains(auditRec.Body.Bytes(), []byte(action)) {
			t.Fatalf("expected audit log action %q, got body=%s", action, auditRec.Body.String())
		}
	}
}

func loginAdminForModelTest(t *testing.T, handler http.Handler) string {
	t.Helper()
	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"admin-model@example.com","password":"password"}`))
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
		t.Fatalf("decode admin login response: %v", err)
	}
	if loginResp.Data.AccessToken == "" {
		t.Fatalf("expected admin access token")
	}
	return loginResp.Data.AccessToken
}
