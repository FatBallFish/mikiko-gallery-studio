package auth

import (
	"context"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
)

type stubLocalAuthPolicy struct {
	allowLogin       bool
	allowRegistration bool
}

func (p stubLocalAuthPolicy) LocalAuthPolicy(context.Context) (bool, bool, error) {
	return p.allowLogin, p.allowRegistration, nil
}

func newOIDCAuthService() *Service {
	return NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
		FixedEmailCode:    "123456",
	}, map[string]string{"basic": "1.00000"})
}

func seedLocalUser(t *testing.T, svc *Service, email string) LoginResult {
	t.Helper()
	if err := svc.SendEmailCode(email, "login"); err != nil {
		t.Fatalf("send code for %s: %v", email, err)
	}
	result, err := svc.LoginWithEmailCodeResult(email, "123456")
	if err != nil {
		t.Fatalf("email-code login for %s: %v", email, err)
	}
	return result
}

func TestLoginWithOIDCAutoRegistersAndReusesIdentity(t *testing.T) {
	svc := newOIDCAuthService()
	first, err := svc.LoginWithOIDC(OIDCLoginInput{Subject: "sub-1", Email: "oidc@example.com", EmailVerified: true, DisplayName: "Mikiko Tester"})
	if err != nil {
		t.Fatal(err)
	}
	if !first.Created || first.User.Email != "oidc@example.com" || first.Session.AccessToken == "" {
		t.Fatalf("first login must auto-register and issue a session: %#v", first)
	}
	if first.User.Nickname != "Mikiko Tester" {
		t.Fatalf("nickname must come from profile claims: %q", first.User.Nickname)
	}
	if first.User.EmailVerifiedAt == nil {
		t.Fatal("verified oidc email must mark the account email verified")
	}

	second, err := svc.LoginWithOIDC(OIDCLoginInput{Subject: "sub-1", Email: "oidc@example.com", EmailVerified: true})
	if err != nil {
		t.Fatal(err)
	}
	if second.Created || second.User.ID != first.User.ID {
		t.Fatalf("second login must reuse the linked account: first=%d second=%d", first.User.ID, second.User.ID)
	}
}

func TestLoginWithOIDCLinksVerifiedEmailToExistingAccount(t *testing.T) {
	svc := newOIDCAuthService()
	local := seedLocalUser(t, svc, "shared@example.com")
	linked, err := svc.LoginWithOIDC(OIDCLoginInput{Subject: "sub-shared", Email: "shared@example.com", EmailVerified: true})
	if err != nil {
		t.Fatal(err)
	}
	if linked.Created || linked.User.ID != local.User.ID {
		t.Fatalf("verified email must link the existing account: local=%d oidc=%d", local.User.ID, linked.User.ID)
	}
}

func TestLoginWithOIDCRejectsUnverifiedEmailMerge(t *testing.T) {
	svc := newOIDCAuthService()
	seedLocalUser(t, svc, "protected@example.com")
	if _, err := svc.LoginWithOIDC(OIDCLoginInput{Subject: "sub-evil", Email: "protected@example.com", EmailVerified: false}); err == nil {
		t.Fatal("unverified email must not link an existing account")
	}
}

func TestLoginWithOIDCRequiresUsableEmail(t *testing.T) {
	svc := newOIDCAuthService()
	if _, err := svc.LoginWithOIDC(OIDCLoginInput{Subject: "sub-noemail"}); err == nil {
		t.Fatal("missing email must be rejected")
	}
	if _, err := svc.LoginWithOIDC(OIDCLoginInput{Subject: "", Email: "x@example.com", EmailVerified: true}); err == nil {
		t.Fatal("missing subject must be rejected")
	}
}

func TestLocalAuthPolicyGatesLegacyLogins(t *testing.T) {
	svc := newOIDCAuthService()
	svc.SetLocalAuthPolicyResolver(stubLocalAuthPolicy{allowLogin: false, allowRegistration: true})
	if _, _, err := svc.LoginWithPassword("anyone@example.com", "secret123"); err == nil {
		t.Fatal("password login must be blocked when local login is disabled")
	}
	if err := svc.SendEmailCode("anyone@example.com", "login"); err == nil {
		t.Fatal("email codes must be blocked when local login is disabled")
	}
	if _, err := svc.LoginWithEmailCodeResult("newuser@example.com", "123456"); err == nil {
		t.Fatal("email-code login must be blocked when local login is disabled")
	}

	registrationOnly := newOIDCAuthService()
	registrationOnly.SetLocalAuthPolicyResolver(stubLocalAuthPolicy{allowLogin: true, allowRegistration: false})
	if err := registrationOnly.SendEmailCode("signup-blocked@example.com", "login"); err != nil {
		t.Fatal(err)
	}
	if _, err := registrationOnly.LoginWithEmailCodeResult("signup-blocked@example.com", "123456"); err == nil {
		t.Fatal("email-code sign-up must be blocked when registration is disabled")
	}

	// OIDC first-login registration follows the platform SSO contract and
	// stays open even with the local registration switch off.
	oidcLogin, err := registrationOnly.LoginWithOIDC(OIDCLoginInput{Subject: "sub-open", Email: "openid@example.com", EmailVerified: true})
	if err != nil || !oidcLogin.Created {
		t.Fatalf("oidc auto-registration must ignore the local switch: err=%v created=%v", err, oidcLogin.Created)
	}
}
