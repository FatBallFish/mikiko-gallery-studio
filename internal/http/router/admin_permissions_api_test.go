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

func TestAdminPermissionResolverControlsMountedRoutes(t *testing.T) {
	cfg := adminConfigAPIConfig()
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "readonly-admin@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	api := handlers.NewAPIWithModelAdminService(cfg, authSvc, nil, nil, nil, nil, nil, adminAuth, nil, nil, nil, nil, nil)
	api.SetAdminPermissionResolver(readOnlyRouteResolver{})
	handler := NewWithAPI(api)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"readonly-admin@example.com","password":"password"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected admin login 200, got %d body=%s", loginRec.Code, loginRec.Body.String())
	}
	var loginResp struct {
		Data struct {
			AccessToken string   `json:"access_token"`
			Permissions []string `json:"permissions"`
		} `json:"data"`
	}
	if err := json.NewDecoder(loginRec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if loginResp.Data.AccessToken == "" || len(loginResp.Data.Permissions) != 1 || loginResp.Data.Permissions[0] != string(domainadminauth.PermissionReadOnly) {
		t.Fatalf("expected injected resolver permissions in login response, got %#v", loginResp.Data)
	}

	readReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/readiness", nil)
	readReq.Header.Set("Authorization", "Bearer "+loginResp.Data.AccessToken)
	readRec := httptest.NewRecorder()
	handler.ServeHTTP(readRec, readReq)
	if readRec.Code != http.StatusOK {
		t.Fatalf("expected read-only route 200, got %d body=%s", readRec.Code, readRec.Body.String())
	}

	manageReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/users?page=1&page_size=10", nil)
	manageReq.Header.Set("Authorization", "Bearer "+loginResp.Data.AccessToken)
	manageRec := httptest.NewRecorder()
	handler.ServeHTTP(manageRec, manageReq)
	if manageRec.Code != http.StatusForbidden {
		t.Fatalf("expected manage users route 403 from injected resolver, got %d body=%s", manageRec.Code, manageRec.Body.String())
	}
}

type readOnlyRouteResolver struct{}

func (readOnlyRouteResolver) HasPermission(principal domainadminauth.AdminPrincipal, permission domainadminauth.Permission) bool {
	return permission == domainadminauth.PermissionReadOnly
}

func (readOnlyRouteResolver) PermissionsForPrincipal(principal domainadminauth.AdminPrincipal) []domainadminauth.Permission {
	return []domainadminauth.Permission{domainadminauth.PermissionReadOnly}
}
