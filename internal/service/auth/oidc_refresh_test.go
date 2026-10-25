package auth

import (
	"github.com/fatballfish/pic-gallery/internal/config"
	"testing"
	"time"
)

func TestRefreshOIDCOnlyAccount(t *testing.T) {
	s := NewService(config.AuthConfig{AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: time.Hour, Issuer: "test", AccessTokenSecret: "secret"}, map[string]string{"basic": "1.00000"})
	login, e := s.LoginWithOIDC(OIDCLoginInput{Subject: "oidc-only-refresh", Email: "oidc-only@example.com", EmailVerified: true})
	if e != nil {
		t.Fatal(e)
	}
	if login.User.PasswordHash != "" {
		t.Fatal("OIDC account unexpectedly has local password")
	}
	_, next, e := s.Refresh(login.Session.RefreshToken)
	if e != nil {
		t.Fatalf("OIDC refresh: %v", e)
	}
	if next.AccessToken == "" || next.RefreshToken == "" {
		t.Fatal("missing rotated session")
	}
	if _, _, e = s.Refresh(login.Session.RefreshToken); e == nil {
		t.Fatal("OIDC refresh replay accepted")
	}
	login, e = s.LoginWithOIDC(OIDCLoginInput{Subject: "oidc-only-refresh", Email: "oidc-only@example.com", EmailVerified: true})
	if e != nil {
		t.Fatal(e)
	}
	s.DisableUserForTest(login.User.ID)
	if _, _, e = s.Refresh(login.Session.RefreshToken); e == nil {
		t.Fatal("disabled OIDC refresh accepted")
	}
}
