package router

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
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

func TestRestoredLegacyAgentRoutes(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	cfg.Storage.LocalRoot = t.TempDir()
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("legacy-agent@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	_, session, err := authSvc.LoginWithEmailCode("legacy-agent@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}
	handler := NewWithAPI(handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, nil, nil))

	preferencesReq := httptest.NewRequest(http.MethodPut, "/api/agent/user/v1/preferences", bytes.NewBufferString(`{"theme":"dark","default_locale":"en-US"}`))
	preferencesReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	preferencesReq.Header.Set("Content-Type", "application/json")
	preferencesRec := httptest.NewRecorder()
	handler.ServeHTTP(preferencesRec, preferencesReq)
	if preferencesRec.Code != http.StatusOK {
		t.Fatalf("expected preferences route 200, got %d body=%s", preferencesRec.Code, preferencesRec.Body.String())
	}

	var avatarBody bytes.Buffer
	writer := multipart.NewWriter(&avatarBody)
	part, err := writer.CreateFormFile("file", "avatar.png")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	pngBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	if _, err := part.Write(pngBytes); err != nil {
		t.Fatalf("write avatar: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	avatarReq := httptest.NewRequest(http.MethodPost, "/api/agent/user/v1/avatar", &avatarBody)
	avatarReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	avatarReq.Header.Set("Content-Type", writer.FormDataContentType())
	avatarRec := httptest.NewRecorder()
	handler.ServeHTTP(avatarRec, avatarReq)
	if avatarRec.Code != http.StatusOK {
		t.Fatalf("expected avatar route 200, got %d body=%s", avatarRec.Code, avatarRec.Body.String())
	}

	changeReq := httptest.NewRequest(http.MethodPost, "/api/agent/auth/v1/password/change", bytes.NewBufferString(`{"old_password":"old","new_password":"new"}`))
	changeReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	changeReq.Header.Set("Content-Type", "application/json")
	changeRec := httptest.NewRecorder()
	handler.ServeHTTP(changeRec, changeReq)
	if changeRec.Code != http.StatusNotImplemented {
		t.Fatalf("expected password change route to return 501 envelope, got %d body=%s", changeRec.Code, changeRec.Body.String())
	}

	imageReq := httptest.NewRequest(http.MethodGet, "/api/agent/image/v1/images/missing/download", nil)
	imageReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	imageRec := httptest.NewRecorder()
	handler.ServeHTTP(imageRec, imageReq)
	if imageRec.Code != http.StatusNotFound {
		t.Fatalf("expected image download route 404 envelope, got %d body=%s", imageRec.Code, imageRec.Body.String())
	}
}

func TestRestoredAdminAndDocsRoutes(t *testing.T) {
	cfg := adminConfigAPIConfig()
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "legacy-admin@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: "super_admin", Status: "active"}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	api := handlers.NewAPIWithCompletionServices(cfg, nil, nil, nil, nil, nil, nil, adminAuth, nil)
	handler := NewWithAPI(api)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"legacy-admin@example.com","password":"password"}`))
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

	for _, tc := range []struct {
		method string
		path   string
		want   int
		auth   bool
	}{
		{method: http.MethodPost, path: "/api/ops/admin/v1/auth/logout", want: http.StatusNoContent, auth: true},
		{method: http.MethodGet, path: "/api/ops/admin/v1/audit-logs", want: http.StatusOK, auth: true},
		{method: http.MethodGet, path: "/api/ops/admin/v1/call-records", want: http.StatusOK, auth: true},
		{method: http.MethodGet, path: "/docs/openapi.yaml", want: http.StatusOK},
		{method: http.MethodGet, path: "/docs/openapi.json", want: http.StatusOK},
		{method: http.MethodGet, path: "/docs/examples", want: http.StatusOK},
		{method: http.MethodGet, path: "/docs/errors", want: http.StatusOK},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		if tc.auth {
			req.Header.Set("Authorization", "Bearer "+loginResp.Data.AccessToken)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("%s %s expected %d, got %d body=%s", tc.method, tc.path, tc.want, rec.Code, rec.Body.String())
		}
	}
}
