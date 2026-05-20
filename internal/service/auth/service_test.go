package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type capturingEmailSender struct {
	email string
	scene string
	code  string
}

func (s *capturingEmailSender) SendVerificationCode(email, scene, code string) error {
	s.email = email
	s.scene = scene
	s.code = code
	return nil
}

func TestLoginAndRefreshRotation(t *testing.T) {
	svc := NewService(config.AuthConfig{AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: 2 * time.Hour, Issuer: "test", AccessTokenSecret: "secret", RefreshCookieName: "pg_refresh"}, map[string]string{"basic": "1.00000"})
	if err := svc.SendEmailCode("user@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	user, session, err := svc.LoginWithEmailCode("user@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}
	if user.ID == 0 || session.AccessToken == "" || session.RefreshToken == "" {
		t.Fatalf("expected tokens and user to be created")
	}
	refreshedUser, refreshedSession, err := svc.Refresh(session.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if refreshedUser.ID != user.ID {
		t.Fatalf("expected same user after refresh")
	}
	if refreshedSession.RefreshToken == session.RefreshToken {
		t.Fatalf("expected refresh token rotation")
	}
	_, _, err = svc.Refresh(session.RefreshToken)
	if err == nil {
		t.Fatalf("expected replay detection on reused refresh token")
	}
	appErr, ok := err.(*errs.Error)
	if !ok || appErr.Code != errs.CodeAuthRefreshReplayBlocked {
		t.Fatalf("expected replay blocked error, got %#v", err)
	}
}

func TestSendEmailCodeFailsClosedWithoutDeliveryOrExplicitFixedCode(t *testing.T) {
	svc := NewService(config.AuthConfig{AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: 2 * time.Hour, Issuer: "pic-gallery-local", AccessTokenSecret: "secret", RefreshCookieName: "pg_refresh"}, map[string]string{"basic": "1.00000"})
	err := svc.SendEmailCode("user@example.com", "login")
	if err == nil {
		t.Fatalf("expected send-code to fail closed without configured delivery")
	}
	if !strings.Contains(err.Error(), "email verification SMTP delivery is not configured") {
		t.Fatalf("expected clear SMTP config error, got %v", err)
	}
}

func TestSendEmailCodeAllowsExplicitFixedCodeForTests(t *testing.T) {
	t.Setenv("PIC_GALLERY_AUTH_FIXED_CODE", "654321")
	svc := NewService(config.AuthConfig{AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: 2 * time.Hour, Issuer: "pic-gallery-local", AccessTokenSecret: "secret", RefreshCookieName: "pg_refresh"}, map[string]string{"basic": "1.00000"})
	if err := svc.SendEmailCode("fixed@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	if _, _, err := svc.LoginWithEmailCode("fixed@example.com", "654321"); err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}
}

func TestRejectsFixedEmailCodeInProd(t *testing.T) {
	t.Setenv("PIC_GALLERY_AUTH_FIXED_CODE", "654321")
	t.Setenv("PIC_GALLERY_AUTH_DEV_EMAIL_CODES", "")
	cfg := config.AuthConfig{Issuer: "pic-gallery"}

	if err := ValidateProductionEmailCodeConfig("prod", cfg); err == nil {
		t.Fatal("expected prod auth config to reject fixed email code")
	}
}

func TestRejectsDevEmailCodesInProd(t *testing.T) {
	t.Setenv("PIC_GALLERY_AUTH_FIXED_CODE", "")
	t.Setenv("PIC_GALLERY_AUTH_DEV_EMAIL_CODES", "true")
	cfg := config.AuthConfig{Issuer: "pic-gallery"}

	if err := ValidateProductionEmailCodeConfig("production", cfg); err == nil {
		t.Fatal("expected production auth config to reject dev email codes")
	}
}

func TestRejectsTestIssuerFixedCodeInProd(t *testing.T) {
	t.Setenv("PIC_GALLERY_AUTH_FIXED_CODE", "")
	t.Setenv("PIC_GALLERY_AUTH_DEV_EMAIL_CODES", "")
	cfg := config.AuthConfig{Issuer: "test"}

	if err := ValidateProductionEmailCodeConfig("prod", cfg); err == nil {
		t.Fatal("expected prod auth config to reject test issuer fixed code")
	}
}

func TestAllowsFixedEmailCodeOutsideProd(t *testing.T) {
	t.Setenv("PIC_GALLERY_AUTH_FIXED_CODE", "654321")
	t.Setenv("PIC_GALLERY_AUTH_DEV_EMAIL_CODES", "")
	cfg := config.AuthConfig{Issuer: "test"}

	if err := ValidateProductionEmailCodeConfig("local", cfg); err != nil {
		t.Fatalf("expected local auth config to allow fixed test codes, got %v", err)
	}
}

func TestAllowsDevEmailCodesOutsideProd(t *testing.T) {
	t.Setenv("PIC_GALLERY_AUTH_FIXED_CODE", "")
	t.Setenv("PIC_GALLERY_AUTH_DEV_EMAIL_CODES", "true")
	cfg := config.AuthConfig{Issuer: "pic-gallery-local"}

	if err := ValidateProductionEmailCodeConfig("test", cfg); err != nil {
		t.Fatalf("expected test auth config to allow dev email codes, got %v", err)
	}
}

func TestSendEmailCodeUsesConfiguredSenderOutsideTestIssuer(t *testing.T) {
	sender := &capturingEmailSender{}
	svc := NewServiceWithEmailSender(
		config.AuthConfig{AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: 2 * time.Hour, Issuer: "pic-gallery", AccessTokenSecret: "secret", RefreshCookieName: "pg_refresh"},
		map[string]string{"basic": "1.00000"},
		sender,
	)

	if err := svc.SendEmailCode("User@Example.COM", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	if sender.email != "user@example.com" || sender.scene != "login" {
		t.Fatalf("sender got email=%q scene=%q", sender.email, sender.scene)
	}
	if len(sender.code) != 6 {
		t.Fatalf("expected 6-digit generated code, got %q", sender.code)
	}
	if _, _, err := svc.LoginWithEmailCode("user@example.com", sender.code); err != nil {
		t.Fatalf("LoginWithEmailCode with delivered code: %v", err)
	}
}
