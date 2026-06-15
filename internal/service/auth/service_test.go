package auth

import (
	"context"
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

type fakeRedisRuntime struct {
	cooldowns   map[string]time.Time
	emailCodes  map[string]emailCode
	refreshes   map[string]refreshSession
	familyBlock map[string]time.Time
}

func (s *capturingEmailSender) SendVerificationCode(email, scene, code string) error {
	s.email = email
	s.scene = scene
	s.code = code
	return nil
}

func newFakeRedisRuntime() *fakeRedisRuntime {
	return &fakeRedisRuntime{
		cooldowns:   map[string]time.Time{},
		emailCodes:  map[string]emailCode{},
		refreshes:   map[string]refreshSession{},
		familyBlock: map[string]time.Time{},
	}
}

func (f *fakeRedisRuntime) EmailCooldownActive(_ context.Context, email string) (bool, error) {
	until, ok := f.cooldowns[email]
	return ok && time.Now().Before(until), nil
}

func (f *fakeRedisRuntime) StoreEmailCode(_ context.Context, email string, record emailCode, codeTTL time.Duration, cooldownTTL time.Duration) error {
	f.emailCodes[email] = record
	f.cooldowns[email] = time.Now().Add(cooldownTTL)
	_ = codeTTL
	return nil
}

func (f *fakeRedisRuntime) LoadEmailCode(_ context.Context, email string, _ []string) (emailCode, bool, error) {
	record, ok := f.emailCodes[email]
	return record, ok, nil
}

func (f *fakeRedisRuntime) DeleteEmailCodes(_ context.Context, email string, _ []string) error {
	delete(f.emailCodes, email)
	return nil
}

func (f *fakeRedisRuntime) StoreRefreshTokenState(_ context.Context, tokenHash string, session refreshSession, _ time.Duration) error {
	f.refreshes[tokenHash] = session
	return nil
}

func (f *fakeRedisRuntime) LoadRefreshTokenState(_ context.Context, tokenHash string) (refreshSession, bool, error) {
	session, ok := f.refreshes[tokenHash]
	return session, ok, nil
}

func (f *fakeRedisRuntime) MarkRefreshFamilyReplayBlocked(_ context.Context, familyID string, ttl time.Duration) error {
	f.familyBlock[familyID] = time.Now().Add(ttl)
	return nil
}

func (f *fakeRedisRuntime) IsRefreshFamilyReplayBlocked(_ context.Context, familyID string) (bool, error) {
	until, ok := f.familyBlock[familyID]
	return ok && time.Now().Before(until), nil
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
	svc := NewService(config.AuthConfig{AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: 2 * time.Hour, Issuer: "pic-gallery-local", AccessTokenSecret: "secret", RefreshCookieName: "pg_refresh", FixedEmailCode: "654321"}, map[string]string{"basic": "1.00000"})
	if err := svc.SendEmailCode("fixed@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	if _, _, err := svc.LoginWithEmailCode("fixed@example.com", "654321"); err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}
}

func TestRejectsFixedEmailCodeInProd(t *testing.T) {
	cfg := config.AuthConfig{Issuer: "pic-gallery", FixedEmailCode: "654321"}

	if err := ValidateProductionEmailCodeConfig("prod", cfg); err == nil {
		t.Fatal("expected prod auth config to reject fixed email code")
	}
}

func TestRejectsDevEmailCodesInProd(t *testing.T) {
	cfg := config.AuthConfig{Issuer: "pic-gallery", DevEmailCodes: true}

	if err := ValidateProductionEmailCodeConfig("production", cfg); err == nil {
		t.Fatal("expected production auth config to reject dev email codes")
	}
}

func TestRejectsTestIssuerFixedCodeInProd(t *testing.T) {
	cfg := config.AuthConfig{Issuer: "test"}

	if err := ValidateProductionEmailCodeConfig("prod", cfg); err == nil {
		t.Fatal("expected prod auth config to reject test issuer fixed code")
	}
}

func TestAllowsFixedEmailCodeOutsideProd(t *testing.T) {
	cfg := config.AuthConfig{Issuer: "test", FixedEmailCode: "654321"}

	if err := ValidateProductionEmailCodeConfig("local", cfg); err != nil {
		t.Fatalf("expected local auth config to allow fixed test codes, got %v", err)
	}
}

func TestAllowsDevEmailCodesOutsideProd(t *testing.T) {
	cfg := config.AuthConfig{Issuer: "pic-gallery-local", DevEmailCodes: true}

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

func TestSendEmailCodeUsesRedisCooldownAndStorage(t *testing.T) {
	sender := &capturingEmailSender{}
	runtime := newFakeRedisRuntime()
	svc := NewServiceWithStoreAndRedis(
		config.AuthConfig{AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: 2 * time.Hour, Issuer: "pic-gallery", AccessTokenSecret: "secret", RefreshCookieName: "pg_refresh"},
		map[string]string{"basic": "1.00000"},
		nil,
		runtime,
		false,
	)
	svc.emailSender = sender

	if err := svc.SendEmailCode("User@Example.COM", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	record, ok := runtime.emailCodes["user@example.com"]
	if !ok || record.Scene != "login" || record.Code == "" {
		t.Fatalf("expected email code stored in redis runtime, got %#v", record)
	}
	if _, exists := svc.codesByEmail["user@example.com"]; !exists {
		t.Fatalf("expected in-memory fallback state to stay populated for local degradation")
	}
	if err := svc.SendEmailCode("user@example.com", "login"); err == nil {
		t.Fatal("expected cooldown enforcement from redis runtime")
	}
}

func TestRefreshReplayBlockedByRedisRuntimeState(t *testing.T) {
	runtime := newFakeRedisRuntime()
	svc := NewServiceWithStoreAndRedis(
		config.AuthConfig{AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: 2 * time.Hour, Issuer: "test", AccessTokenSecret: "secret", RefreshCookieName: "pg_refresh"},
		map[string]string{"basic": "1.00000"},
		nil,
		runtime,
		false,
	)
	if err := svc.SendEmailCode("user@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	user, session, err := svc.LoginWithEmailCode("user@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}
	_, refreshed, err := svc.Refresh(session.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if refreshed.RefreshToken == "" || refreshed.RefreshToken == session.RefreshToken {
		t.Fatalf("expected rotated refresh token, got %#v", refreshed)
	}
	cached := runtime.refreshes[hashToken(session.RefreshToken)]
	if cached.Status != "rotated" || cached.FamilyID == "" {
		t.Fatalf("expected rotated refresh token state in redis runtime, got %#v", cached)
	}
	_, _, err = svc.Refresh(session.RefreshToken)
	if err == nil {
		t.Fatal("expected replay blocked from redis runtime state")
	}
	appErr, ok := err.(*errs.Error)
	if !ok || appErr.Code != errs.CodeAuthRefreshReplayBlocked {
		t.Fatalf("expected replay blocked error, got %#v", err)
	}
	if blocked, _ := runtime.IsRefreshFamilyReplayBlocked(context.Background(), cached.FamilyID); !blocked {
		t.Fatalf("expected replay-blocked family flag to be stored after reuse for user %d", user.ID)
	}
}
