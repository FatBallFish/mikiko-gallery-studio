package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
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
