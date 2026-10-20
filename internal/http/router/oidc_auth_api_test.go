package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	domainsecureconfig "github.com/fatballfish/pic-gallery/internal/domain/secureconfig"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	oidcservice "github.com/fatballfish/pic-gallery/internal/service/oidc"
	"github.com/fatballfish/pic-gallery/internal/service/oidc/oidctest"
	secureconfigservice "github.com/fatballfish/pic-gallery/internal/service/secureconfig"
)

func newOIDCTestStack(t *testing.T, idp *oidctest.Server) (http.Handler, *secureconfigservice.Service, *authservice.Service) {
	t.Helper()
	cfg := taskAPIConfig(idp.URL)
	cfg.Auth.AccessTokenTTL = 10 * time.Minute
	cfg.Auth.RefreshTokenTTL = 2 * time.Hour
	cfg.Auth.Issuer = "test"
	cfg.Auth.AccessTokenSecret = "router-test-secret"
	cfg.Auth.RefreshCookieName = "pg_refresh"
	authSvc := authservice.NewService(cfg.Auth, map[string]string{"basic": "1.00000"})
	secureCfg := secureconfigservice.NewService(secureconfigservice.NewMemoryStore(), "router-test-key", cfg.Auth.SMTP, "test")
	authSvc.SetLocalAuthPolicyResolver(secureCfg)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, nil, nil)
	api.SetSecureConfigService(secureCfg)
	api.SetOIDCClient(oidcservice.NewClient(nil))
	return NewWithAPI(api), secureCfg, authSvc
}

func configureOIDC(t *testing.T, secureCfg *secureconfigservice.Service, idp *oidctest.Server, allowLocalLogin, allowRegistration bool) {
	t.Helper()
	_, err := secureCfg.UpdateOIDCConfig(t.Context(), domainsecureconfig.UpdateOIDCConfigRequest{
		Enabled:           true,
		IssuerURL:         idp.URL,
		ClientID:          idp.ClientID,
		AllowLocalLogin:   allowLocalLogin,
		AllowRegistration: allowRegistration,
		Secrets:           map[string]string{"client_secret": idp.ClientSecret},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func startOIDCAuthorize(t *testing.T, handler http.Handler, returnTo string) (*url.URL, *http.Cookie) {
	t.Helper()
	target := "/api/agent/auth/v1/oidc/authorize"
	if returnTo != "" {
		target += "?return_to=" + url.QueryEscape(returnTo)
	}
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.Host = "127.0.0.1"
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusFound {
		t.Fatalf("authorize expected 302, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	location, err := url.Parse(recorder.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	var txCookie *http.Cookie
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == oidcservice.TransactionCookieName {
			txCookie = cookie
		}
	}
	if txCookie == nil {
		t.Fatal("authorize must set the transaction cookie")
	}
	return location, txCookie
}

func completeOIDCLogin(t *testing.T, handler http.Handler, idp *oidctest.Server, authorizeURL *url.URL, txCookie *http.Cookie, state string) oidcCallbackResponse {
	t.Helper()
	query := authorizeURL.Query()
	code := idp.IssueCode(query.Get("code_challenge"), query.Get("nonce"))
	body := `{"code":` + quoteJSON(code) + `,"state":` + quoteJSON(state) + `}`
	request := httptest.NewRequest(http.MethodPost, "/api/agent/auth/v1/oidc/callback", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(txCookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("callback expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response oidcCallbackResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if cookies := recorder.Result().Cookies(); len(cookies) == 0 {
		t.Fatal("callback must set the refresh cookie")
	}
	return response
}

func quoteJSON(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

type oidcCallbackResponse struct {
	Data struct {
		AccessToken string `json:"access_token"`
		UserID      int64  `json:"user_id"`
		ReturnTo    string `json:"return_to"`
	} `json:"data"`
}

func TestOIDCEndpointsNotEnabledByDefault(t *testing.T) {
	idp := oidctest.Start(t)
	handler, _, _ := newOIDCTestStack(t, idp)
	request := httptest.NewRequest(http.MethodGet, "/api/agent/auth/v1/oidc/authorize", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("authorize must 404 while oidc is disabled, got %d", recorder.Code)
	}
	features := httptest.NewRequest(http.MethodGet, "/api/agent/features/v1", nil)
	featuresRecorder := httptest.NewRecorder()
	handler.ServeHTTP(featuresRecorder, features)
	body := featuresRecorder.Body.String()
	if !strings.Contains(body, `"password_login_enabled":true`) || !strings.Contains(body, `"registration_enabled":true`) || !strings.Contains(body, `"oidc_login":{"enabled":false`) {
		t.Fatalf("feature flags must default to open local login: %s", body)
	}
}

func TestOIDCLoginEndToEnd(t *testing.T) {
	idp := oidctest.Start(t)
	handler, secureCfg, _ := newOIDCTestStack(t, idp)
	configureOIDC(t, secureCfg, idp, true, true)

	authorizeURL, txCookie := startOIDCAuthorize(t, handler, "http://127.0.0.1/home?x=1#frag")
	state := authorizeURL.Query().Get("state")
	first := completeOIDCLogin(t, handler, idp, authorizeURL, txCookie, state)
	if first.Data.AccessToken == "" || first.Data.UserID == 0 {
		t.Fatalf("callback must issue a session: %#v", first.Data)
	}
	if first.Data.ReturnTo != "http://127.0.0.1/home?x=1#frag" {
		t.Fatalf("return_to must round-trip: %q", first.Data.ReturnTo)
	}

	// Second login reuses the linked identity (same user, no duplicate).
	authorizeURL2, txCookie2 := startOIDCAuthorize(t, handler, "")
	second := completeOIDCLogin(t, handler, idp, authorizeURL2, txCookie2, authorizeURL2.Query().Get("state"))
	if second.Data.UserID != first.Data.UserID {
		t.Fatalf("identity must be reused: first=%d second=%d", first.Data.UserID, second.Data.UserID)
	}
}

func TestOIDCCallbackRejectsStateMismatch(t *testing.T) {
	idp := oidctest.Start(t)
	handler, secureCfg, _ := newOIDCTestStack(t, idp)
	configureOIDC(t, secureCfg, idp, true, true)
	_, txCookie := startOIDCAuthorize(t, handler, "")
	request := httptest.NewRequest(http.MethodPost, "/api/agent/auth/v1/oidc/callback", strings.NewReader(`{"code":"irrelevant","state":"forged-state"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(txCookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("forged state must be rejected, got %d", recorder.Code)
	}
}

func TestOIDCCallbackWithoutCookieIsRejected(t *testing.T) {
	idp := oidctest.Start(t)
	handler, secureCfg, _ := newOIDCTestStack(t, idp)
	configureOIDC(t, secureCfg, idp, true, true)
	request := httptest.NewRequest(http.MethodPost, "/api/agent/auth/v1/oidc/callback", strings.NewReader(`{"code":"x","state":"y"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("missing transaction cookie must be rejected, got %d", recorder.Code)
	}
}

func TestLocalLoginSwitchBlocksLegacyLoginPaths(t *testing.T) {
	idp := oidctest.Start(t)
	handler, secureCfg, _ := newOIDCTestStack(t, idp)
	configureOIDC(t, secureCfg, idp, false, false)

	passwordRec := postJSON(t, handler, "/api/agent/auth/v1/login/password", `{"email":"local@example.com","password":"secret123"}`)
	if passwordRec.Code != http.StatusForbidden || !strings.Contains(passwordRec.Body.String(), "LOCAL_LOGIN_DISABLED") {
		t.Fatalf("password login must be blocked: %d %s", passwordRec.Code, passwordRec.Body.String())
	}
	codeRec := postJSON(t, handler, "/api/agent/auth/v1/email/send-code", `{"email":"local@example.com","scene":"login"}`)
	if codeRec.Code != http.StatusForbidden || !strings.Contains(codeRec.Body.String(), "LOCAL_LOGIN_DISABLED") {
		t.Fatalf("email codes must be blocked: %d %s", codeRec.Code, codeRec.Body.String())
	}
	features := httptest.NewRequest(http.MethodGet, "/api/agent/features/v1", nil)
	featuresRecorder := httptest.NewRecorder()
	handler.ServeHTTP(featuresRecorder, features)
	body := featuresRecorder.Body.String()
	if !strings.Contains(body, `"password_login_enabled":false`) || !strings.Contains(body, `"registration_enabled":false`) || !strings.Contains(body, `"oidc_login":{"enabled":true`) {
		t.Fatalf("feature flags must reflect the switches: %s", body)
	}
	// OIDC login keeps working in pure-OIDC mode.
	authorizeURL, txCookie := startOIDCAuthorize(t, handler, "")
	login := completeOIDCLogin(t, handler, idp, authorizeURL, txCookie, authorizeURL.Query().Get("state"))
	if login.Data.AccessToken == "" {
		t.Fatal("oidc login must still work with local login disabled")
	}
}
