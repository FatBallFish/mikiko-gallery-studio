package router

import (
	"bytes"
	"context"
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
	secureconfigservice "github.com/fatballfish/pic-gallery/internal/service/secureconfig"
)

func TestAdminSecuritySMTPConfigWriteOnlySecret(t *testing.T) {
	cfg := adminConfigAPIConfig()
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "ops-smtp@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: domainadminauth.RoleAdmin, Status: "active"}); err != nil {
		t.Fatalf("CreateAdmin ops: %v", err)
	}
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "root-smtp@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: domainadminauth.RoleSuperAdmin, Status: "active"}); err != nil {
		t.Fatalf("CreateAdmin root: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	secureSvc := secureconfigservice.NewService(secureconfigservice.NewMemoryStore(), "smtp-admin-api-test-key", config.SMTPConfig{}, "test")
	secureSvc.SetSMTPConnectivityValidator(func(context.Context, config.SMTPConfig) error {
		return nil
	})
	api := handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, nil, nil, adminAuth, nil)
	api.SetSecureConfigService(secureSvc)
	handler := NewWithAPI(api)

	opsToken := loginAdminWithCredentials(t, handler, "ops-smtp@example.com", "password")
	rootToken := loginAdminWithCredentials(t, handler, "root-smtp@example.com", "password")

	getReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/security/smtp", nil)
	getReq.Header.Set("Authorization", "Bearer "+opsToken)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected admin read smtp config 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}

	body := `{"enabled":true,"host":"smtp.example.com","port":587,"username":"mailer@example.com","from":"Pic Gallery <noreply@example.com>","starttls":true,"secrets":{"password":"smtp-password-secret"}}`
	opsPut := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/security/smtp", bytes.NewBufferString(body))
	opsPut.Header.Set("Authorization", "Bearer "+opsToken)
	opsPut.Header.Set("Content-Type", "application/json")
	opsPutRec := httptest.NewRecorder()
	handler.ServeHTTP(opsPutRec, opsPut)
	if opsPutRec.Code != http.StatusForbidden {
		t.Fatalf("expected normal admin smtp update 403, got %d body=%s", opsPutRec.Code, opsPutRec.Body.String())
	}

	rootPut := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/security/smtp", bytes.NewBufferString(body))
	rootPut.Header.Set("Authorization", "Bearer "+rootToken)
	rootPut.Header.Set("Content-Type", "application/json")
	rootPutRec := httptest.NewRecorder()
	handler.ServeHTTP(rootPutRec, rootPut)
	if rootPutRec.Code != http.StatusOK {
		t.Fatalf("expected super admin smtp update 200, got %d body=%s", rootPutRec.Code, rootPutRec.Body.String())
	}
	if bytes.Contains(rootPutRec.Body.Bytes(), []byte("smtp-password-secret")) {
		t.Fatalf("smtp update response must not contain plaintext password, body=%s", rootPutRec.Body.String())
	}
	var updateResp struct {
		Data struct {
			Version      int64 `json:"version"`
			SecretStatus struct {
				HasSecret bool     `json:"has_secret"`
				Fields    []string `json:"secret_fields"`
			} `json:"secret_status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rootPutRec.Body).Decode(&updateResp); err != nil {
		t.Fatalf("decode smtp update response: %v", err)
	}
	if updateResp.Data.Version != 1 || !updateResp.Data.SecretStatus.HasSecret || len(updateResp.Data.SecretStatus.Fields) != 1 || updateResp.Data.SecretStatus.Fields[0] != "password" {
		t.Fatalf("unexpected smtp update response %#v", updateResp.Data)
	}

	rootGet := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/security/smtp", nil)
	rootGet.Header.Set("Authorization", "Bearer "+rootToken)
	rootGetRec := httptest.NewRecorder()
	handler.ServeHTTP(rootGetRec, rootGet)
	if rootGetRec.Code != http.StatusOK {
		t.Fatalf("expected smtp get 200, got %d body=%s", rootGetRec.Code, rootGetRec.Body.String())
	}
	if bytes.Contains(rootGetRec.Body.Bytes(), []byte("smtp-password-secret")) {
		t.Fatalf("smtp get response must not contain plaintext password, body=%s", rootGetRec.Body.String())
	}
}
