package oauthserver_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"entgo.io/ent/dialect"
	domainauth "github.com/fatballfish/pic-gallery/internal/domain/auth"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	"github.com/fatballfish/pic-gallery/internal/service/oauthserver"
	_ "github.com/mattn/go-sqlite3"
)

func TestOAuthDurableLifecycle(t *testing.T) {
	ctx := context.Background()
	dsn := "file:" + filepath.Join(t.TempDir(), "oauth.db") + "?_fk=1&_busy_timeout=5000&_journal_mode=WAL"
	client, e := repoent.Open(dialect.SQLite, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	if e = client.Schema.Create(ctx); e != nil {
		t.Fatal(e)
	}
	users := entstore.NewAuthStore(client)
	u, e := users.CreateUser(ctx, domainauth.User{Email: "oauth@example.com", Nickname: "OAuth", Status: "active", GroupCode: "basic", GroupMultiplier: "1.00000"})
	if e != nil {
		t.Fatal(e)
	}
	cfg := oauthserver.Config{PublicAPIURL: "https://studio.example", WebURL: "https://web.example", Clients: []oauthserver.Client{{ID: "test", Name: "Test App", RedirectURIs: []string{"http://127.0.0.1:9876/callback"}}}, User: func(ctx context.Context, id int64) (oauthserver.User, error) {
		v, e := users.GetUserByID(ctx, id)
		return oauthserver.User{ID: v.ID, Active: v.Status == "active", GroupCode: v.GroupCode}, e
	}}
	s, e := oauthserver.New(entstore.NewOAuthStore(client), cfg)
	if e != nil {
		t.Fatal(e)
	}
	verifier := strings.Repeat("a", 43)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	begin := func() url.Values {
		t.Helper()
		q := url.Values{"client_id": {"test"}, "redirect_uri": {"http://127.0.0.1:9876/callback"}, "response_type": {"code"}, "code_challenge_method": {"S256"}, "code_challenge": {challenge}, "resource": {s.Resource()}, "scope": {"account:read models:read"}, "state": {"state-value"}}
		id, e := s.Begin(ctx, q)
		if e != nil {
			t.Fatal(e)
		}
		_, csrf, e := s.Consent(ctx, id, u.ID)
		if _, _, err := s.Consent(ctx, id, u.ID+1); err == nil {
			t.Fatal("cross-user authorization request accepted")
		}

		if e != nil {
			t.Fatal(e)
		}
		if _, e = s.Decide(ctx, id, u.ID, "wrong", true); e == nil {
			t.Fatal("accepted wrong CSRF")
		}
		redirect, e := s.Decide(ctx, id, u.ID, csrf, true)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = s.Decide(ctx, id, u.ID, csrf, true); e == nil {
			t.Fatal("consent replay")
		}
		uri, _ := url.Parse(redirect)
		if uri.Query().Get("state") != "state-value" {
			t.Fatal("state lost")
		}
		return url.Values{"grant_type": {"authorization_code"}, "client_id": {"test"}, "redirect_uri": {q.Get("redirect_uri")}, "code": {uri.Query().Get("code")}, "code_verifier": {verifier}, "resource": {s.Resource()}}
	}
	code := begin()
	for _, key := range []string{"code_verifier", "client_id", "redirect_uri", "resource"} {
		old := code.Get(key)
		code.Set(key, "incorrect")
		if _, e = s.Token(ctx, code); e == nil {
			t.Fatalf("accepted wrong %s", key)
		}
		code.Set(key, old)
	}
	token, e := s.Token(ctx, code)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Token(ctx, code); e == nil {
		t.Fatal("code replay accepted")
	}
	p, e := s.ValidateAccess(ctx, token.AccessToken)
	if e != nil || p.UserID != u.ID || p.ClientID != "test" || p.GrantID == "" || strings.Join(p.Scopes, " ") != "account:read models:read" {
		t.Fatalf("principal %+v, %v", p, e)
	}
	if e = s.ValidateGrant(ctx, p.UserID, p.ClientID, p.GrantID, []string{"account:read"}); e != nil {
		t.Fatal(e)
	}
	if e = s.ValidateGrant(ctx, p.UserID, p.ClientID, p.GrantID, []string{"assets:delete"}); e == nil {
		t.Fatal("delegated capability scope escalation")
	}
	// A second process/service instance uses the same persisted credential state.
	other, e := repoent.Open(dialect.SQLite, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer other.Close()
	s2, e := oauthserver.New(entstore.NewOAuthStore(other), cfg)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s2.ValidateAccess(ctx, token.AccessToken); e != nil {
		t.Fatalf("durable token: %v", e)
	}
	wrongCfg := cfg
	wrongCfg.PublicAPIURL = "https://other.example"
	wrong, _ := oauthserver.New(entstore.NewOAuthStore(other), wrongCfg)
	if _, e = wrong.ValidateAccess(ctx, token.AccessToken); e == nil {
		t.Fatal("wrong audience accepted")
	}
	refresh := url.Values{"grant_type": {"refresh_token"}, "client_id": {"test"}, "resource": {s.Resource()}, "refresh_token": {token.RefreshToken}, "scope": {"assets:delete"}}
	if _, e = s.Token(ctx, refresh); e == nil {
		t.Fatal("scope escalation")
	}
	refresh.Del("scope")
	next, e := s2.Token(ctx, refresh)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Token(ctx, refresh); e == nil {
		t.Fatal("refresh replay accepted")
	}
	if _, e = s2.ValidateAccess(ctx, next.AccessToken); e == nil {
		t.Fatal("reused refresh family still valid")
	}
	code = begin()
	token, e = s.Token(ctx, code)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Revoke(ctx, token.RefreshToken, "other"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.ValidateAccess(ctx, token.AccessToken); e != nil {
		t.Fatal("different client revoked grant")
	}
	if e = s.Revoke(ctx, token.RefreshToken, "test"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.ValidateAccess(ctx, token.AccessToken); e == nil {
		t.Fatal("revoked access accepted")
	}
	code = begin()
	token, e = s.Token(ctx, code)
	if e != nil {
		t.Fatal(e)
	}
	p, _ = s.ValidateAccess(ctx, token.AccessToken)
	if e = s.Disconnect(ctx, u.ID+1, p.GrantID); e == nil {
		t.Fatal("cross user disconnect")
	}
	if e = s.Disconnect(ctx, u.ID, p.GrantID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.ValidateAccess(ctx, token.AccessToken); e == nil {
		t.Fatal("disconnect did not revoke")
	}
	if e = s.ValidateGrant(ctx, p.UserID, p.ClientID, p.GrantID, p.Scopes); e == nil {
		t.Fatal("disconnected delegated capability accepted")
	}
	code = begin()
	token, e = s.Token(ctx, code)
	if e != nil {
		t.Fatal(e)
	}
	u.Status = "disabled"
	if _, e = users.UpdateUser(ctx, u); e != nil {
		t.Fatal(e)
	}
	if _, e = s.ValidateAccess(ctx, token.AccessToken); e == nil {
		t.Fatal("disabled user accepted")
	}
	if _, e = s.Token(ctx, url.Values{"grant_type": {"refresh_token"}, "client_id": {"test"}, "resource": {s.Resource()}, "refresh_token": {token.RefreshToken}}); e == nil {
		t.Fatal("disabled refresh accepted")
	}
	u.Status = "active"
	if _, e = users.UpdateUser(ctx, u); e != nil {
		t.Fatal(e)
	}
	code = begin()
	token, e = s.Token(ctx, code)
	if e != nil {
		t.Fatal(e)
	}
	refresh = url.Values{"grant_type": {"refresh_token"}, "client_id": {"test"}, "resource": {s.Resource()}, "refresh_token": {token.RefreshToken}}
	var wg sync.WaitGroup
	results := make(chan oauthserver.TokenResponse, 2)
	for _, server := range []*oauthserver.Service{s, s2} {
		wg.Add(1)
		go func(server *oauthserver.Service) {
			defer wg.Done()
			v, e := server.Token(ctx, refresh)
			if e == nil {
				results <- v
			}
		}(server)
	}
	wg.Wait()
	close(results)
	n := 0
	for v := range results {
		n++
		if _, e = s.ValidateAccess(ctx, v.AccessToken); e == nil {
			t.Fatal("concurrent reuse did not revoke winner")
		}
	}
	if n != 1 {
		t.Fatalf("refresh successes=%d, want one", n)
	}
}

func TestOAuthCanonicalURLAndAddressPolicy(t *testing.T) {
	for _, raw := range []string{"http://example.com", "https://user@example.com", "https://example.com?x=1", "https://example.com#fragment"} {
		_, e := oauthserver.New(nil, oauthserver.Config{PublicAPIURL: raw})
		if e == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
