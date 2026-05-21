package router

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
)

func TestAdminConfigTabEndpoints(t *testing.T) {
	t.Setenv("PIC_GALLERY_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("PIC_GALLERY_ADMIN_PASSWORD", "admin-password")
	t.Setenv("PIC_GALLERY_ADMIN_TOKEN_SECRET", "admin-token-secret-for-router-tests")
	t.Setenv("PIC_GALLERY_ADMIN_ID", "42")

	cfg := adminConfigAPIConfig()
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})

	store := &capturingAdminConfigStore{delegate: adminconfigservice.NewMemoryStore()}
	adminSvc := adminconfigservice.NewServiceWithStore(cfg, store)
	api := handlers.NewAPIWithServices(cfg, authSvc, nil, nil, adminSvc)
	handler := NewWithAPI(api)

	badReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/config-tabs", nil)
	badReq.Header.Set("Authorization", "Bearer arbitrary")
	badRec := httptest.NewRecorder()
	handler.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected arbitrary bearer to be rejected, got %d body=%s", badRec.Code, badRec.Body.String())
	}

	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"admin@example.com","password":"admin-password"}`))
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
	if loginResp.Data.AccessToken == "" || loginResp.Data.AccessToken == "admin-dev-token" {
		t.Fatalf("expected signed admin token, got %q", loginResp.Data.AccessToken)
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
	if store.lastUpdatedBy != 42 {
		t.Fatalf("expected config update to use authenticated admin id 42, got %d", store.lastUpdatedBy)
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

func TestAdminProdMissingAdminTokenSecretRejectsLogin(t *testing.T) {
	t.Setenv("PIC_GALLERY_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("PIC_GALLERY_ADMIN_PASSWORD", "admin-password")
	t.Setenv("PIC_GALLERY_ADMIN_TOKEN_SECRET", "")

	cfg := adminConfigAPIConfig()
	cfg.App.Env = "prod"
	cfg.Auth.AccessTokenSecret = "strong-auth-token-secret-for-users-only"
	handler := newAdminConfigHandler(t, cfg)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"admin@example.com","password":"admin-password"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusInternalServerError {
		t.Fatalf("expected prod login without admin token secret to fail closed, got %d body=%s", loginRec.Code, loginRec.Body.String())
	}
}

func TestAdminProdRejectsWeakAdminTokenSecret(t *testing.T) {
	t.Setenv("PIC_GALLERY_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("PIC_GALLERY_ADMIN_PASSWORD", "admin-password")
	t.Setenv("PIC_GALLERY_ADMIN_TOKEN_SECRET", "change-me-in-prod")

	cfg := adminConfigAPIConfig()
	cfg.App.Env = "production"
	cfg.Auth.AccessTokenSecret = "strong-auth-token-secret-for-users-only"
	handler := newAdminConfigHandler(t, cfg)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"admin@example.com","password":"admin-password"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusInternalServerError {
		t.Fatalf("expected prod login with weak admin token secret to fail closed, got %d body=%s", loginRec.Code, loginRec.Body.String())
	}
}

func TestAdminLocalFallsBackToAuthSecret(t *testing.T) {
	t.Setenv("PIC_GALLERY_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("PIC_GALLERY_ADMIN_PASSWORD", "admin-password")
	t.Setenv("PIC_GALLERY_ADMIN_TOKEN_SECRET", "")

	cfg := adminConfigAPIConfig()
	cfg.App.Env = "local"
	cfg.Auth.AccessTokenSecret = "local-compatible-auth-secret-for-admin-fallback"
	handler := newAdminConfigHandler(t, cfg)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"admin@example.com","password":"admin-password"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected local login to keep auth-secret fallback compatibility, got %d body=%s", loginRec.Code, loginRec.Body.String())
	}
}

func TestAdminProdAuthDefaultSecretForgeryRejected(t *testing.T) {
	t.Setenv("PIC_GALLERY_ADMIN_TOKEN_SECRET", "")

	cfg := adminConfigAPIConfig()
	cfg.App.Env = "prod"
	cfg.Auth.AccessTokenSecret = "change-me-in-prod"
	handler := newAdminConfigHandler(t, cfg)

	forged := signAdminTestToken(t, cfg.Auth.AccessTokenSecret, cfg.Auth.Issuer, 1)
	req := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/config-tabs", nil)
	req.Header.Set("Authorization", "Bearer "+forged)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected prod forged admin token signed with auth default secret to be rejected, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func adminConfigAPIConfig() config.Config {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	cfg.Auth.Issuer = "test"
	cfg.Auth.AccessTokenSecret = "secret"
	cfg.Docs.Title = "Pic Gallery API Docs"
	cfg.Docs.BasePath = "/docs"
	return cfg
}

func newAdminConfigHandler(t *testing.T, cfg config.Config) http.Handler {
	t.Helper()
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            cfg.Auth.Issuer,
		AccessTokenSecret: cfg.Auth.AccessTokenSecret,
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	adminSvc := adminconfigservice.NewServiceWithStore(cfg, adminconfigservice.NewMemoryStore())
	return NewWithAPI(handlers.NewAPIWithServices(cfg, authSvc, nil, nil, adminSvc))
}

func signAdminTestToken(t *testing.T, secret, issuer string, adminID int64) string {
	t.Helper()
	now := time.Now()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"admin_id": adminID,
		"email":    "admin@example.com",
		"role":     "admin",
		"sub":      strconv.FormatInt(adminID, 10),
		"iss":      issuer,
		"iat":      now.Unix(),
		"exp":      now.Add(10 * time.Minute).Unix(),
	}).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign forged admin token: %v", err)
	}
	return token
}

type capturingAdminConfigStore struct {
	delegate      adminconfigservice.Store
	lastUpdatedBy int64
}

func (s *capturingAdminConfigStore) GetByCategory(ctx context.Context, category string) ([]domainadminconfig.Item, error) {
	return s.delegate.GetByCategory(ctx, category)
}

func (s *capturingAdminConfigStore) SaveByCategory(ctx context.Context, category string, version int64, updatedBy int64, items []domainadminconfig.Item) error {
	s.lastUpdatedBy = updatedBy
	return s.delegate.SaveByCategory(ctx, category, version, updatedBy, items)
}
