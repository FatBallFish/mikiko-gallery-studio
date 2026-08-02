package auth

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainauth "github.com/fatballfish/pic-gallery/internal/domain/auth"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	jwt "github.com/golang-jwt/jwt/v5"
)

func TestEmailCodeLoginRequiresPasswordSetupBeforeSession(t *testing.T) {
	svc := NewService(config.AuthConfig{AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: 2 * time.Hour, Issuer: "test", AccessTokenSecret: "secret", RefreshCookieName: "pg_refresh"}, map[string]string{"basic": "1.00000"})
	if err := svc.SendEmailCode("new-user@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}

	login, err := svc.LoginWithEmailCodeResult("new-user@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCodeResult: %v", err)
	}
	if !login.PasswordSetupRequired || login.PasswordSetupToken == "" {
		t.Fatalf("expected password setup grant, got %#v", login)
	}
	if login.Session.AccessToken != "" || login.Session.RefreshToken != "" || login.Session.SessionID != "" {
		t.Fatalf("passwordless login must not issue a normal session, got %#v", login.Session)
	}
	if _, err := svc.ParseAccessToken(login.PasswordSetupToken); err == nil {
		t.Fatal("password setup grant must not authorize normal user APIs")
	}

	user, session, err := svc.CompletePasswordSetup(login.PasswordSetupToken, "new-password-123")
	if err != nil {
		t.Fatalf("CompletePasswordSetup: %v", err)
	}
	if user.PasswordHash == "" || session.AccessToken == "" || session.RefreshToken == "" {
		t.Fatalf("password setup must issue the first normal session, user=%#v session=%#v", user, session)
	}
	claims, err := svc.ParseAccessToken(session.AccessToken)
	if err != nil || claims.Purpose != accessTokenPurpose {
		t.Fatalf("normal session must carry access purpose, claims=%#v err=%v", claims, err)
	}
	if _, _, err := svc.CompletePasswordSetup(login.PasswordSetupToken, "another-password-123"); err == nil {
		t.Fatal("password setup grant must be one-time")
	}
}

func TestEmailCodeLoginIssuesSessionForExistingPasswordUser(t *testing.T) {
	svc := NewService(config.AuthConfig{AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: 2 * time.Hour, Issuer: "test", AccessTokenSecret: "secret", RefreshCookieName: "pg_refresh"}, map[string]string{"basic": "1.00000"})
	if err := svc.SendEmailCode("existing@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	first, err := svc.LoginWithEmailCodeResult("existing@example.com", "123456")
	if err != nil {
		t.Fatalf("first LoginWithEmailCodeResult: %v", err)
	}
	if _, _, err := svc.CompletePasswordSetup(first.PasswordSetupToken, "existing-password-123"); err != nil {
		t.Fatalf("CompletePasswordSetup: %v", err)
	}
	if err := svc.SendEmailCode("existing@example.com", "login"); err != nil {
		t.Fatalf("second SendEmailCode: %v", err)
	}
	login, err := svc.LoginWithEmailCodeResult("existing@example.com", "123456")
	if err != nil {
		t.Fatalf("second LoginWithEmailCodeResult: %v", err)
	}
	if login.PasswordSetupRequired || login.PasswordSetupToken != "" || login.Session.AccessToken == "" || login.Session.RefreshToken == "" {
		t.Fatalf("existing password user must receive a normal session, got %#v", login)
	}
}

func TestPasswordSetupGrantValidatesPurposeExpiryIssuerAndVersion(t *testing.T) {
	cfg := config.AuthConfig{AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: 2 * time.Hour, Issuer: "test", AccessTokenSecret: "secret", RefreshCookieName: "pg_refresh"}
	svc := NewService(cfg, map[string]string{"basic": "1.00000"})
	if err := svc.SendEmailCode("grant@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	login, err := svc.LoginWithEmailCodeResult("grant@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCodeResult: %v", err)
	}

	sign := func(purpose, issuer string, version int, expiresAt time.Time) string {
		t.Helper()
		claims := Claims{
			UserID: login.User.ID, Email: login.User.Email, TokenVersion: version, Purpose: purpose,
			RegisteredClaims: jwt.RegisteredClaims{Subject: fmt.Sprintf("%d", login.User.ID), Issuer: issuer, IssuedAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)), ExpiresAt: jwt.NewNumericDate(expiresAt)},
		}
		token, signErr := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(cfg.AccessTokenSecret))
		if signErr != nil {
			t.Fatalf("sign setup fixture: %v", signErr)
		}
		return token
	}

	for name, token := range map[string]string{
		"purpose": sign(accessTokenPurpose, cfg.Issuer, login.User.TokenVersion, time.Now().Add(time.Minute)),
		"expiry":  sign(passwordSetupTokenPurpose, cfg.Issuer, login.User.TokenVersion, time.Now().Add(-time.Minute)),
		"issuer":  sign(passwordSetupTokenPurpose, "other", login.User.TokenVersion, time.Now().Add(time.Minute)),
		"version": sign(passwordSetupTokenPurpose, cfg.Issuer, login.User.TokenVersion+1, time.Now().Add(time.Minute)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := svc.CompletePasswordSetup(token, "new-password-123"); err == nil {
				t.Fatalf("expected %s setup grant to be rejected", name)
			}
		})
	}
}

func TestPasswordChangeRequiresBoundOneTimeCodeAndRevokesEverySession(t *testing.T) {
	svc := NewService(config.AuthConfig{AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: 2 * time.Hour, Issuer: "test", AccessTokenSecret: "secret", RefreshCookieName: "pg_refresh"}, map[string]string{"basic": "1.00000"})
	if err := svc.SendEmailCode("change@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	user, firstSession := completePasswordlessCodeLogin(t, svc, "change@example.com", "123456")
	_, secondSession, err := svc.LoginWithPassword(user.Email, "test-password-123")
	if err != nil {
		t.Fatalf("LoginWithPassword: %v", err)
	}
	originalVersion := user.TokenVersion
	setEmailCodeForTest(svc, user.Email, "password_change", "246810")

	updated, err := svc.ChangePassword(user.ID, "246810", "changed-password-123")
	if err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if updated.TokenVersion != originalVersion+1 {
		t.Fatalf("password change must increment token version once, before=%d after=%d", originalVersion, updated.TokenVersion)
	}
	for label, refreshToken := range map[string]string{"first": firstSession.RefreshToken, "second": secondSession.RefreshToken} {
		if _, _, err := svc.Refresh(refreshToken); err == nil {
			t.Fatalf("%s refresh session must be revoked", label)
		}
	}
	if _, err := svc.ChangePassword(user.ID, "246810", "another-password-123"); err == nil {
		t.Fatal("password change code must be one-time")
	}
	if _, _, err := svc.LoginWithPassword(user.Email, "test-password-123"); err == nil {
		t.Fatal("old password must stop working after password change")
	}
	if _, _, err := svc.LoginWithPassword(user.Email, "changed-password-123"); err != nil {
		t.Fatalf("new password login: %v", err)
	}
}

func TestPasswordChangeRejectsWrongSceneAndAnotherUsersCode(t *testing.T) {
	svc := NewService(config.AuthConfig{AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: 2 * time.Hour, Issuer: "test", AccessTokenSecret: "secret", RefreshCookieName: "pg_refresh"}, map[string]string{"basic": "1.00000"})
	if err := svc.SendEmailCode("owner@example.com", "login"); err != nil {
		t.Fatalf("owner SendEmailCode: %v", err)
	}
	owner, _ := completePasswordlessCodeLogin(t, svc, "owner@example.com", "123456")
	if err := svc.SendEmailCode("other@example.com", "login"); err != nil {
		t.Fatalf("other SendEmailCode: %v", err)
	}
	other, _ := completePasswordlessCodeLogin(t, svc, "other@example.com", "123456")

	setEmailCodeForTest(svc, owner.Email, "login", "111111")
	if _, err := svc.ChangePassword(owner.ID, "111111", "changed-password-123"); err == nil {
		t.Fatal("login-scene code must not authorize password change")
	}
	setEmailCodeForTest(svc, other.Email, "password_change", "222222")
	if _, err := svc.ChangePassword(owner.ID, "222222", "changed-password-123"); err == nil {
		t.Fatal("another user's code must not authorize password change")
	}
}

func setEmailCodeForTest(svc *Service, email, scene, code string) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	svc.codesByEmail[email] = emailCode{Code: code, Scene: scene, ExpiresAt: time.Now().Add(10 * time.Minute), LastSentAt: time.Now()}
}

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
	user, session := completePasswordlessCodeLogin(t, svc, "user@example.com", "123456")
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

func TestDefaultNicknameFromEmail(t *testing.T) {
	tests := map[string]string{
		"alice@example.com":                 "alice",
		" Alice.Name+tag@Example.com ":      "alice.name+tag",
		"@example.com":                      "user",
		"malformed":                         "user",
		strings.Repeat("长", 70) + "@x.test": strings.Repeat("长", 64),
	}
	for email, want := range tests {
		if got := defaultNicknameFromEmail(email); got != want {
			t.Errorf("defaultNicknameFromEmail(%q)=%q want %q", email, got, want)
		}
	}
}

func TestRegisterDerivesNicknameFromEmailInMemoryAndStoreModes(t *testing.T) {
	for _, tt := range []struct {
		name  string
		store Store
	}{
		{name: "memory"},
		{name: "store", store: newRecordingAuthStore()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewServiceWithStore(config.AuthConfig{AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: 2 * time.Hour, Issuer: "test", AccessTokenSecret: "secret", RefreshCookieName: "pg_refresh"}, map[string]string{"basic": "1.00000"}, tt.store)
			for _, item := range []struct {
				email string
				want  string
			}{{email: "alice@example.com", want: "alice"}, {email: "bob.name+tag@example.com", want: "bob.name+tag"}} {
				if err := svc.SendEmailCode(item.email, "login"); err != nil {
					t.Fatalf("SendEmailCode(%s): %v", item.email, err)
				}
				user, _, err := svc.LoginWithEmailCode(item.email, "123456")
				if err != nil {
					t.Fatalf("LoginWithEmailCode(%s): %v", item.email, err)
				}
				if user.Nickname != item.want || strings.HasPrefix(user.Nickname, "user-") {
					t.Fatalf("expected email-derived nickname %q, got %#v", item.want, user)
				}
			}
		})
	}
}

type recordingAuthStore struct {
	Store
	nextID int64
	users  map[string]domainauth.User
}

func newRecordingAuthStore() *recordingAuthStore {
	return &recordingAuthStore{nextID: 1, users: map[string]domainauth.User{}}
}

func (s *recordingAuthStore) GetUserByEmail(_ context.Context, email string) (domainauth.User, error) {
	user, ok := s.users[email]
	if !ok {
		return domainauth.User{}, repoerr.ErrNotFound
	}
	return user, nil
}

func (s *recordingAuthStore) CreateUser(_ context.Context, user domainauth.User) (domainauth.User, error) {
	user.ID = s.nextID
	s.nextID++
	s.users[user.Email] = user
	return user, nil
}

func (s *recordingAuthStore) SaveRefreshSession(context.Context, entstore.RefreshSessionRecord) error {
	return nil
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
	user, session := completePasswordlessCodeLogin(t, svc, "user@example.com", "123456")
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

func completePasswordlessCodeLogin(t *testing.T, svc *Service, email, code string) (domainauth.User, domainauth.Session) {
	t.Helper()
	login, err := svc.LoginWithEmailCodeResult(email, code)
	if err != nil {
		t.Fatalf("LoginWithEmailCodeResult: %v", err)
	}
	if !login.PasswordSetupRequired || login.PasswordSetupToken == "" {
		t.Fatalf("expected password setup grant, got %#v", login)
	}
	user, session, err := svc.CompletePasswordSetup(login.PasswordSetupToken, "test-password-123")
	if err != nil {
		t.Fatalf("CompletePasswordSetup: %v", err)
	}
	return user, session
}
