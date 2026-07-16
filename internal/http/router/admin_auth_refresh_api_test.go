package router

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
)

func TestAdminSessionRefreshRouteRotatesCookieAndAccessToken(t *testing.T) {
	handler := newAdminRefreshTestHandler(t)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"refresh-admin@example.com","password":"refresh-password"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body=%s", loginRec.Code, loginRec.Body.String())
	}
	loginToken := responseAccessToken(t, loginRec)
	refreshCookie := responseCookie(loginRec.Result().Cookies(), "pg_admin_refresh_token")
	if refreshCookie == nil || refreshCookie.Value == "" {
		t.Fatalf("expected admin refresh cookie after login, got %#v", loginRec.Result().Cookies())
	}
	if !refreshCookie.Secure {
		t.Fatalf("expected admin refresh cookie to be secure in production, got %#v", refreshCookie)
	}

	refreshReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/session/refresh", nil)
	refreshReq.AddCookie(refreshCookie)
	refreshRec := httptest.NewRecorder()
	handler.ServeHTTP(refreshRec, refreshReq)
	if refreshRec.Code != http.StatusOK {
		t.Fatalf("refresh status = %d, body=%s", refreshRec.Code, refreshRec.Body.String())
	}
	refreshedToken := responseAccessToken(t, refreshRec)
	if refreshedToken == loginToken {
		t.Fatal("expected refresh to rotate the admin access token")
	}
	rotatedCookie := responseCookie(refreshRec.Result().Cookies(), "pg_admin_refresh_token")
	if rotatedCookie == nil || rotatedCookie.Value == "" || rotatedCookie.Value == refreshCookie.Value {
		t.Fatalf("expected rotated admin refresh cookie, got %#v", refreshRec.Result().Cookies())
	}

	replayReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/session/refresh", nil)
	replayReq.AddCookie(refreshCookie)
	replayRec := httptest.NewRecorder()
	handler.ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusUnauthorized {
		t.Fatalf("replayed refresh cookie status = %d, want 401; body=%s", replayRec.Code, replayRec.Body.String())
	}
}

func TestAdminSessionRefreshRouteRejectsMissingCookie(t *testing.T) {
	handler := newAdminRefreshTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/session/refresh", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("refresh without cookie status = %d, want 401; body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminSessionRefreshPreservesCookieOnTransientStoreFailure(t *testing.T) {
	cfg := adminRefreshTestConfig()
	memoryStore := adminauthservice.NewMemoryStore()
	if _, err := memoryStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "refresh-admin@example.com",
		PasswordHash: adminauthservice.HashPassword("refresh-password"),
		Role:         domainadminauth.RoleSuperAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	store := &adminGetByIDErrorStore{Store: memoryStore}
	adminAuth := adminauthservice.NewService(cfg.Auth, store)
	api := handlers.NewAPIWithCompletionServices(cfg, nil, nil, nil, nil, nil, nil, adminAuth, nil)
	handler := NewWithAPI(api)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"refresh-admin@example.com","password":"refresh-password"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	refreshCookie := responseCookie(loginRec.Result().Cookies(), "pg_admin_refresh_token")
	if loginRec.Code != http.StatusOK || refreshCookie == nil {
		t.Fatalf("login status = %d, cookies=%#v", loginRec.Code, loginRec.Result().Cookies())
	}

	store.err = errors.New("temporary database outage")
	refreshReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/session/refresh", nil)
	refreshReq.AddCookie(refreshCookie)
	refreshRec := httptest.NewRecorder()
	handler.ServeHTTP(refreshRec, refreshReq)
	if refreshRec.Code != http.StatusInternalServerError {
		t.Fatalf("refresh transient failure status = %d, want 500; body=%s", refreshRec.Code, refreshRec.Body.String())
	}
	if cookie := responseCookie(refreshRec.Result().Cookies(), "pg_admin_refresh_token"); cookie != nil && (cookie.Value == "" || cookie.MaxAge < 0) {
		t.Fatalf("transient refresh failure must preserve the refresh cookie, got %#v", cookie)
	}
}

func TestAdminLogoutClearsRefreshCookieWithoutValidAccessToken(t *testing.T) {
	handler := newAdminRefreshTestHandler(t)
	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"refresh-admin@example.com","password":"refresh-password"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	refreshCookie := responseCookie(loginRec.Result().Cookies(), "pg_admin_refresh_token")
	if loginRec.Code != http.StatusOK || refreshCookie == nil {
		t.Fatalf("login status = %d, cookies=%#v", loginRec.Code, loginRec.Result().Cookies())
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/logout", nil)
	logoutReq.Header.Set("Authorization", "Bearer expired-access-token")
	logoutReq.AddCookie(refreshCookie)
	logoutRec := httptest.NewRecorder()
	handler.ServeHTTP(logoutRec, logoutReq)
	if logoutRec.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want 204; body=%s", logoutRec.Code, logoutRec.Body.String())
	}
	cleared := responseCookie(logoutRec.Result().Cookies(), "pg_admin_refresh_token")
	if cleared == nil || cleared.Value != "" || cleared.MaxAge >= 0 {
		t.Fatalf("logout must clear the admin refresh cookie, got %#v", cleared)
	}

	refreshReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/session/refresh", nil)
	refreshReq.AddCookie(refreshCookie)
	refreshRec := httptest.NewRecorder()
	handler.ServeHTTP(refreshRec, refreshReq)
	if refreshRec.Code != http.StatusUnauthorized {
		t.Fatalf("refresh after logout status = %d, want 401; body=%s", refreshRec.Code, refreshRec.Body.String())
	}
}

func TestAdminLogoutRevokesBearerAndCookieSessionFamilies(t *testing.T) {
	handler, adminAuth := newAdminRefreshTestHandlerWithService(t)
	firstToken, firstCookie := loginAdminRefreshTest(t, handler)
	secondToken, secondCookie := loginAdminRefreshTest(t, handler)

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/logout", nil)
	logoutReq.Header.Set("Authorization", "Bearer "+firstToken)
	logoutReq.AddCookie(secondCookie)
	logoutRec := httptest.NewRecorder()
	handler.ServeHTTP(logoutRec, logoutReq)
	if logoutRec.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want 204; body=%s", logoutRec.Code, logoutRec.Body.String())
	}
	for _, accessToken := range []string{firstToken, secondToken} {
		if _, err := adminAuth.ParseAccessToken(t.Context(), accessToken); err == nil {
			t.Fatal("expected logout to revoke both bearer and cookie session families")
		}
	}
	if _, err := adminAuth.Refresh(t.Context(), firstCookie.Value); err == nil {
		t.Fatal("expected bearer session refresh family to be revoked")
	}
}

func newAdminRefreshTestHandler(t *testing.T) http.Handler {
	t.Helper()
	handler, _ := newAdminRefreshTestHandlerWithService(t)
	return handler
}

func newAdminRefreshTestHandlerWithService(t *testing.T) (http.Handler, *adminauthservice.Service) {
	t.Helper()
	cfg := adminRefreshTestConfig()
	store := adminauthservice.NewMemoryStore()
	if _, err := store.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "refresh-admin@example.com",
		PasswordHash: adminauthservice.HashPassword("refresh-password"),
		Role:         domainadminauth.RoleSuperAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, store)
	api := handlers.NewAPIWithCompletionServices(cfg, nil, nil, nil, nil, nil, nil, adminAuth, nil)
	return NewWithAPI(api), adminAuth
}

func loginAdminRefreshTest(t *testing.T, handler http.Handler) (string, *http.Cookie) {
	t.Helper()
	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"refresh-admin@example.com","password":"refresh-password"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body=%s", loginRec.Code, loginRec.Body.String())
	}
	cookie := responseCookie(loginRec.Result().Cookies(), "pg_admin_refresh_token")
	if cookie == nil {
		t.Fatalf("login response missing admin refresh cookie: %#v", loginRec.Result().Cookies())
	}
	return responseAccessToken(t, loginRec), cookie
}

func adminRefreshTestConfig() config.Config {
	cfg := config.Config{}
	cfg.App.Env = "production"
	cfg.Auth = config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "admin-refresh-test",
		AccessTokenSecret: "admin-refresh-secret",
	}
	return cfg
}

type adminGetByIDErrorStore struct {
	adminauthservice.Store
	err error
}

func (s *adminGetByIDErrorStore) GetAdminByID(ctx context.Context, id int64) (domainadminauth.AdminUser, error) {
	if s.err != nil {
		return domainadminauth.AdminUser{}, s.err
	}
	return s.Store.GetAdminByID(ctx, id)
}

func responseAccessToken(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var response struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Data.AccessToken == "" {
		t.Fatalf("response missing access_token: %s", rec.Body.String())
	}
	return response.Data.AccessToken
}

func responseCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}
