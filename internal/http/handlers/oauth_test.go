package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/fatballfish/pic-gallery/internal/config"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	"github.com/fatballfish/pic-gallery/internal/service/oauthserver"
)

func TestOAuthErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		err         error
		status      int
		code, retry string
	}{
		{&oauthserver.RateLimitError{RetryAfter: 1500 * time.Millisecond}, 429, "temporarily_unavailable", "2"},
		{oauthserver.ErrInvalid, 400, "invalid_client_metadata", ""},
		{oauthserver.ErrRegistrationDisabled, 403, "access_denied", ""},
		{oauthserver.ErrInvalidRequest, 400, "invalid_request", ""},
		{oauthserver.ErrUnsupportedGrant, 400, "unsupported_grant_type", ""},
		{oauthserver.ErrInvalidTarget, 400, "invalid_target", ""},
		{oauthserver.ErrInvalidScope, 400, "invalid_scope", ""},
		{oauthserver.ErrAuthorizationExpired, 400, "authorization_request_expired", ""},
		{errors.New("database offline"), 503, "temporarily_unavailable", ""},
	} {
		w := httptest.NewRecorder()
		oauthFailure(w, tc.err, "invalid_client_metadata")
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) || w.Header().Get("Retry-After") != tc.retry {
			t.Fatalf("%v: %d %s %v", tc.err, w.Code, w.Body.String(), w.Header())
		}
	}
}

func TestOAuthBrowserAuthenticationErrors(t *testing.T) {
	for _, header := range []string{"", "Bearer garbage"} {
		r := httptest.NewRequest("GET", "/api/v1/oauth/connected-apps", nil)
		r.Header.Set("Authorization", header)
		w := httptest.NewRecorder()
		oauthBrowserError(w, r)
		want := "login_required"
		if header != "" {
			want = "invalid_session"
		}
		if w.Code != 401 || !strings.Contains(w.Body.String(), want) {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
}

func TestDeveloperEndpointProjection(t *testing.T) {
	for _, origin := range []string{"", "https://studio.example/"} {
		api := &API{cfg: config.Config{Runtime: config.RuntimeConfig{PublicAPIURL: origin}}}
		r := httptest.NewRequest("GET", "https://attacker.example/api/v1/public/developer-endpoints", nil)
		r.Header.Set("X-Forwarded-Host", "attacker.example")
		w := httptest.NewRecorder()
		api.HandleDeveloperEndpoints(w, r)
		var result struct {
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		d := result.Data
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "attacker") {
			t.Fatalf("projection %s", w.Body.String())
		}
		if d["canonical_configured"] != (origin != "") {
			t.Fatalf("configured: %v", d)
		}
		if origin == "" {
			for k, v := range d {
				if k != "canonical_configured" && k != "disabled_reason" && v != "" {
					t.Fatalf("unconfigured %s=%v", k, v)
				}
			}
			if d["disabled_reason"] != "CANONICAL_URL_UNCONFIGURED" {
				t.Fatal(d)
			}
		} else {
			if d["rest_base"] != "https://studio.example/api/open/v1" || d["mcp_url"] != "https://studio.example/mcp" || d["web_url"] != "https://studio.example" || d["disabled_reason"] != "" {
				t.Fatal(d)
			}
		}
	}
}

func TestOAuthBrowserConsentHTTP(t *testing.T) {
	ctx := context.Background()
	client, e := repoent.Open(dialect.SQLite, "file:"+filepath.Join(t.TempDir(), "oauth-http.db")+"?_fk=1")
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	if e = client.Schema.Create(ctx); e != nil {
		t.Fatal(e)
	}
	users := entstore.NewAuthStore(client)
	auth := authservice.NewServiceWithStore(config.AuthConfig{Issuer: "test", AccessTokenSecret: "oauth-http-secret", AccessTokenTTL: time.Hour, RefreshTokenTTL: time.Hour}, map[string]string{"basic": "1.00000"}, users)
	login, e := auth.LoginWithOIDC(authservice.OIDCLoginInput{Subject: "consent-user", Email: "consent@example.com", EmailVerified: true})
	if e != nil {
		t.Fatal(e)
	}
	s, e := oauthserver.New(entstore.NewOAuthStore(client), oauthserver.Config{PublicAPIURL: "https://api.example", WebURL: "https://web.example", Clients: []oauthserver.Client{{ID: "app", Name: "Browser App", RedirectURIs: []string{"https://client.example/cb"}}}, User: func(ctx context.Context, id int64) (oauthserver.User, error) {
		u, e := users.GetUserByID(ctx, id)
		return oauthserver.User{ID: u.ID, Active: u.Status == "active", GroupCode: u.GroupCode}, e
	}})
	if e != nil {
		t.Fatal(e)
	}
	if _, _, err := auth.Refresh(login.Session.RefreshToken); err != nil {
		t.Fatalf("durable OIDC-only browser refresh: %v", err)
	}
	h := NewOAuth(s, auth)
	// Registration bursts are usable; only the request beyond the window limit is 429.
	registrationNow := time.Date(2026, time.October, 27, 0, 0, 0, 0, time.UTC)
	registration, err := oauthserver.New(entstore.NewOAuthStore(client), oauthserver.Config{PublicAPIURL: "https://api.example", EnableRegistration: true, RegistrationNow: func() time.Time { return registrationNow }, User: func(context.Context, int64) (oauthserver.User, error) { return oauthserver.User{Active: true}, nil }})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 11; i++ {
		rw := httptest.NewRecorder()
		NewOAuth(registration, auth).Register(rw, httptest.NewRequest("POST", "/oauth/register", strings.NewReader(`{"client_name":"DCR","redirect_uris":["http://localhost:31415/callback"]}`)))
		if i < 10 && rw.Code != 201 {
			t.Fatalf("registration %d: %d %s", i, rw.Code, rw.Body.String())
		}
		if i == 10 && (rw.Code != 429 || rw.Header().Get("Retry-After") != "60") {
			t.Fatalf("rate: %d %v %s", rw.Code, rw.Header(), rw.Body.String())
		}
	}
	// Verify exact ceiling and window recovery without depending on database speed.
	for _, step := range []struct {
		advance time.Duration
		status  int
		retry   string
	}{
		{3250 * time.Millisecond, 429, "57"},
		{56500 * time.Millisecond, 429, "1"},
		{250 * time.Millisecond, 201, ""},
	} {
		registrationNow = registrationNow.Add(step.advance)
		rw := httptest.NewRecorder()
		NewOAuth(registration, auth).Register(rw, httptest.NewRequest("POST", "/oauth/register", strings.NewReader(`{"client_name":"DCR","redirect_uris":["http://localhost:31415/callback"]}`)))
		if rw.Code != step.status || rw.Header().Get("Retry-After") != step.retry {
			t.Fatalf("window at %s: status=%d headers=%v body=%s", registrationNow, rw.Code, rw.Header(), rw.Body.String())
		}
		if step.status == 429 && !strings.Contains(rw.Body.String(), `"error":"temporarily_unavailable"`) {
			t.Fatalf("rate error: %s", rw.Body.String())
		}
	}
	verifier := strings.Repeat("v", 43)
	sum := sha256.Sum256([]byte(verifier))
	q := url.Values{"response_type": {"code"}, "client_id": {"app"}, "redirect_uri": {"https://client.example/cb"}, "resource": {s.Resource()}, "scope": {"account:read"}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}}
	req := httptest.NewRequest("GET", "https://api.example/oauth/authorize?"+q.Encode(), nil)
	req.Host = "attacker.example"
	w := httptest.NewRecorder()
	h.Authorize(w, req)
	if w.Code != 303 || !strings.HasPrefix(w.Header().Get("Location"), "https://web.example/") {
		t.Fatalf("authorize: %d %s", w.Code, w.Body.String())
	}
	target, _ := url.Parse(w.Header().Get("Location"))
	id := target.Query().Get("oauth_request")
	consentURL := "https://api.example/api/v1/oauth/consent?request_id=" + url.QueryEscape(id)
	w = httptest.NewRecorder()
	h.Consent(w, httptest.NewRequest("GET", consentURL, nil))
	if w.Code != 401 {
		t.Fatal("anonymous consent allowed")
	}
	req = httptest.NewRequest("GET", consentURL, nil)
	req.Header.Set("Authorization", "Bearer "+login.Session.AccessToken)
	w = httptest.NewRecorder()
	h.Consent(w, req)
	if w.Code != 200 {
		t.Fatalf("consent: %d %s", w.Code, w.Body.String())
	}
	var data struct {
		Data struct {
			CSRF string `json:"csrf_token"`
		} `json:"data"`
	}
	if e = json.Unmarshal(w.Body.Bytes(), &data); e != nil {
		t.Fatal(e)
	}
	body, _ := json.Marshal(map[string]any{"request_id": id, "csrf_token": data.Data.CSRF, "allow": true})
	req = httptest.NewRequest("POST", "https://api.example/api/v1/oauth/consent", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+login.Session.AccessToken)
	w = httptest.NewRecorder()
	h.Consent(w, req)
	if w.Code != 200 {
		t.Fatalf("decision: %d %s", w.Code, w.Body.String())
	}
	var decision struct {
		Data struct {
			Redirect string `json:"redirect_uri"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &decision)
	redirect, _ := url.Parse(decision.Data.Redirect)
	form := url.Values{"grant_type": {"authorization_code"}, "code": {redirect.Query().Get("code")}, "client_id": {"app"}, "redirect_uri": {"https://client.example/cb"}, "resource": {s.Resource()}, "code_verifier": {verifier}}
	req = httptest.NewRequest("POST", "https://api.example/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	h.Token(w, req)
	if w.Code != 200 {
		t.Fatalf("token: %d %s", w.Code, w.Body.String())
	}
	var token oauthserver.TokenResponse
	_ = json.Unmarshal(w.Body.Bytes(), &token)
	if _, e = s.ValidateAccess(ctx, token.AccessToken); e != nil {
		t.Fatal(e)
	}
	req = httptest.NewRequest(http.MethodGet, "https://untrusted.example/.well-known/oauth-authorization-server", nil)
	w = httptest.NewRecorder()
	h.Metadata(w, req)
	if strings.Contains(w.Body.String(), "untrusted") {
		t.Fatal("metadata trusted request host")
	}
}
