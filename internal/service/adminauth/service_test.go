package adminauth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

func TestServiceLoginIssuesAdminRefreshToken(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	if _, err := store.CreateAdmin(ctx, domainadminauth.AdminUser{
		Email:        "refresh-admin@example.com",
		PasswordHash: HashPassword("refresh-password"),
		Role:         domainadminauth.RoleSuperAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}

	svc := NewService(testAuthConfig(), store)
	session, err := svc.Login(ctx, domainadminauth.LoginRequest{Email: "refresh-admin@example.com", Password: "refresh-password"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if session.RefreshToken == "" {
		t.Fatalf("expected login to issue an admin refresh token, got %#v", session)
	}
}

func TestServiceRefreshPreservesTokenOnTransientAdminLookupFailure(t *testing.T) {
	ctx := context.Background()
	memoryStore := NewMemoryStore()
	admin, err := memoryStore.CreateAdmin(ctx, domainadminauth.AdminUser{
		Email:        "transient-refresh@example.com",
		PasswordHash: HashPassword("refresh-password"),
		Role:         domainadminauth.RoleSuperAdmin,
		Status:       "active",
	})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	lookupErr := errors.New("temporary database outage")
	store := &getAdminByIDErrorStore{Store: memoryStore}
	svc := NewService(testAuthConfig(), store)
	session, err := svc.Login(ctx, domainadminauth.LoginRequest{Email: admin.Email, Password: "refresh-password"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	store.err = lookupErr
	if _, err := svc.Refresh(ctx, session.RefreshToken); !errors.Is(err, lookupErr) {
		t.Fatalf("Refresh error = %v, want transient store error", err)
	}
	store.err = nil
	if _, err := svc.Refresh(ctx, session.RefreshToken); err != nil {
		t.Fatalf("Refresh after store recovery: %v", err)
	}
}

func TestServiceRefreshRestoresTokenAfterSessionIssueFailure(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	admin, err := store.CreateAdmin(ctx, domainadminauth.AdminUser{
		Email:        "session-issue-failure@example.com",
		PasswordHash: HashPassword("refresh-password"),
		Role:         domainadminauth.RoleSuperAdmin,
		Status:       "active",
	})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	svc := NewService(testAuthConfig(), store)
	session, err := svc.Login(ctx, domainadminauth.LoginRequest{Email: admin.Email, Password: "refresh-password"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	issueErr := errors.New("temporary token generation failure")
	svc.refreshToken = func(int) (string, error) { return "", issueErr }
	if _, err := svc.Refresh(ctx, session.RefreshToken); !errors.Is(err, issueErr) {
		t.Fatalf("Refresh error = %v, want token generation failure", err)
	}
	svc.refreshToken = randomToken
	if _, err := svc.Refresh(ctx, session.RefreshToken); err != nil {
		t.Fatalf("Refresh after token generator recovery: %v", err)
	}
}

func TestParseAccessTokenPreservesSessionOnTransientAdminLookupFailure(t *testing.T) {
	ctx := context.Background()
	memoryStore := NewMemoryStore()
	admin, err := memoryStore.CreateAdmin(ctx, domainadminauth.AdminUser{
		Email:        "transient-access@example.com",
		PasswordHash: HashPassword("refresh-password"),
		Role:         domainadminauth.RoleSuperAdmin,
		Status:       "active",
	})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	lookupErr := errors.New("temporary database outage")
	store := &getAdminByIDErrorStore{Store: memoryStore}
	svc := NewService(testAuthConfig(), store)
	session, err := svc.Login(ctx, domainadminauth.LoginRequest{Email: admin.Email, Password: "refresh-password"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	store.err = lookupErr
	if _, err := svc.ParseAccessToken(ctx, session.AccessToken); !errors.Is(err, lookupErr) {
		t.Fatalf("ParseAccessToken error = %v, want transient store error", err)
	}
	store.err = nil
	if _, err := svc.ParseAccessToken(ctx, session.AccessToken); err != nil {
		t.Fatalf("ParseAccessToken after store recovery: %v", err)
	}
}

func TestResetAdminPasswordRevokesExistingAccessAndRefreshSessions(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	admin, err := store.CreateAdmin(ctx, domainadminauth.AdminUser{
		Email:        "reset-session@example.com",
		PasswordHash: HashPassword("old-password"),
		Role:         domainadminauth.RoleSuperAdmin,
		Status:       "active",
	})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	svc := NewService(testAuthConfig(), store)
	session, err := svc.Login(ctx, domainadminauth.LoginRequest{Email: admin.Email, Password: "old-password"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if err := svc.ResetAdminPassword(ctx, domainadminauth.AdminPasswordResetRequest{AdminID: admin.ID, NewPassword: "new-password"}); err != nil {
		t.Fatalf("ResetAdminPassword: %v", err)
	}
	if _, err := svc.ParseAccessToken(ctx, session.AccessToken); err == nil {
		t.Fatal("expected access token issued before password reset to be rejected")
	}
	if _, err := svc.Refresh(ctx, session.RefreshToken); err == nil {
		t.Fatal("expected refresh token issued before password reset to be rejected")
	}
	if _, err := svc.Login(ctx, domainadminauth.LoginRequest{Email: admin.Email, Password: "new-password"}); err != nil {
		t.Fatalf("Login with reset password: %v", err)
	}
}

func TestPasswordResetCannotBeCrossedByConcurrentOldPasswordLogin(t *testing.T) {
	ctx := context.Background()
	memoryStore := NewMemoryStore()
	admin, err := memoryStore.CreateAdmin(ctx, domainadminauth.AdminUser{
		Email:        "reset-race@example.com",
		PasswordHash: HashPassword("old-password"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	store := newPasswordResetRaceStore(memoryStore)
	svc := NewService(testAuthConfig(), store)

	loginDone := make(chan error, 1)
	go func() {
		_, loginErr := svc.Login(ctx, domainadminauth.LoginRequest{Email: admin.Email, Password: "old-password"})
		loginDone <- loginErr
	}()
	select {
	case <-store.finalReadStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for login final credential read")
	}
	resetDone := make(chan error, 1)
	go func() {
		resetDone <- svc.ResetAdminPassword(ctx, domainadminauth.AdminPasswordResetRequest{AdminID: admin.ID, NewPassword: "new-password"})
	}()
	if err := <-loginDone; err == nil {
		t.Fatal("expected concurrent login using the old password to be rejected")
	}
	if err := <-resetDone; err != nil {
		t.Fatalf("ResetAdminPassword: %v", err)
	}
}

func TestConcurrentRefreshDoesNotRevokeSuccessfulRotation(t *testing.T) {
	ctx := context.Background()
	memoryStore := NewMemoryStore()
	admin, err := memoryStore.CreateAdmin(ctx, domainadminauth.AdminUser{
		Email:        "concurrent-refresh@example.com",
		PasswordHash: HashPassword("refresh-password"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	store := newConcurrentRefreshStore(memoryStore)
	svc := NewService(testAuthConfig(), store)
	initial, err := svc.Login(ctx, domainadminauth.LoginRequest{Email: admin.Email, Password: "refresh-password"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	store.enableBlocking()

	type refreshResult struct {
		session domainadminauth.Session
		err     error
	}
	results := make(chan refreshResult, 2)
	go func() {
		session, refreshErr := svc.Refresh(ctx, initial.RefreshToken)
		results <- refreshResult{session: session, err: refreshErr}
	}()
	select {
	case <-store.refreshStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for primary refresh to reach the store")
	}
	go func() {
		session, refreshErr := svc.Refresh(ctx, initial.RefreshToken)
		results <- refreshResult{session: session, err: refreshErr}
	}()
	duplicate := <-results
	if duplicate.err == nil {
		t.Fatalf("expected duplicate in-flight refresh to fail, got %#v", duplicate.session)
	}
	if appErr, ok := duplicate.err.(*errs.Error); !ok || appErr.StatusCode != 409 {
		t.Fatalf("duplicate in-flight refresh error = %v, want 409 conflict", duplicate.err)
	}
	store.releaseRefresh()
	primary := <-results
	if primary.err != nil || primary.session.RefreshToken == "" {
		t.Fatalf("primary concurrent refresh failed: session=%#v err=%v", primary.session, primary.err)
	}
	successful := primary.session
	if _, err := svc.ParseAccessToken(ctx, successful.AccessToken); err != nil {
		t.Fatalf("successful concurrent refresh access token was revoked: %v", err)
	}
	if _, err := svc.Refresh(ctx, successful.RefreshToken); err != nil {
		t.Fatalf("successful concurrent refresh token was revoked: %v", err)
	}
}

func TestRefreshReplayRevokesAccessTokensInSessionFamily(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	admin, err := store.CreateAdmin(ctx, domainadminauth.AdminUser{
		Email:        "replay-family@example.com",
		PasswordHash: HashPassword("refresh-password"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	svc := NewService(testAuthConfig(), store)
	first, err := svc.Login(ctx, domainadminauth.LoginRequest{Email: admin.Email, Password: "refresh-password"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	second, err := svc.Refresh(ctx, first.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if _, err := svc.Refresh(ctx, first.RefreshToken); err == nil {
		t.Fatal("expected replayed refresh token to be rejected")
	}
	for _, accessToken := range []string{first.AccessToken, second.AccessToken} {
		if _, err := svc.ParseAccessToken(ctx, accessToken); err == nil {
			t.Fatal("expected refresh replay to revoke every access token in the session family")
		}
	}
}

func TestLogoutRefreshRevokesAccessTokenFamily(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	admin, err := store.CreateAdmin(ctx, domainadminauth.AdminUser{
		Email:        "logout-family@example.com",
		PasswordHash: HashPassword("refresh-password"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	svc := NewService(testAuthConfig(), store)
	session, err := svc.Login(ctx, domainadminauth.LoginRequest{Email: admin.Email, Password: "refresh-password"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	svc.LogoutRefresh(session.RefreshToken)
	if _, err := svc.ParseAccessToken(ctx, session.AccessToken); err == nil {
		t.Fatal("expected logout to revoke the access token in the refresh family")
	}
	if _, err := svc.Refresh(ctx, session.RefreshToken); err == nil {
		t.Fatal("expected logout to revoke the refresh token")
	}
}

func TestAdminDisableAndReenableDoesNotReviveExistingSessions(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	admin, err := store.CreateAdmin(ctx, domainadminauth.AdminUser{
		Email:        "disable-session@example.com",
		PasswordHash: HashPassword("refresh-password"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	svc := NewService(testAuthConfig(), store)
	session, err := svc.Login(ctx, domainadminauth.LoginRequest{Email: admin.Email, Password: "refresh-password"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	for _, status := range []string{"disabled", "active"} {
		if _, err := svc.UpdateAdmin(ctx, domainadminauth.AdminUpdateRequest{
			AdminID:        admin.ID,
			OperatorID:     admin.ID + 100,
			Status:         status,
			StatusProvided: true,
		}); err != nil {
			t.Fatalf("UpdateAdmin status %s: %v", status, err)
		}
	}
	if _, err := svc.ParseAccessToken(ctx, session.AccessToken); err == nil {
		t.Fatal("expected access token from before disable to remain revoked after re-enable")
	}
	if _, err := svc.Refresh(ctx, session.RefreshToken); err == nil {
		t.Fatal("expected refresh token from before disable to remain revoked after re-enable")
	}
}

func TestAdminAccessTokenRejectsUnexpectedAlgorithmAndIssuer(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	admin, err := store.CreateAdmin(ctx, domainadminauth.AdminUser{
		Email:        "jwt-contract@example.com",
		PasswordHash: HashPassword("refresh-password"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	cfg := testAuthConfig()
	svc := NewService(cfg, store)
	session, err := svc.Login(ctx, domainadminauth.LoginRequest{Email: admin.Email, Password: "refresh-password"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	claims, err := svc.ParseAccessToken(ctx, session.AccessToken)
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}

	wrongAlgorithm, err := jwt.NewWithClaims(jwt.SigningMethodHS384, claims).SignedString([]byte(cfg.AccessTokenSecret))
	if err != nil {
		t.Fatalf("sign wrong-algorithm token: %v", err)
	}
	if _, err := svc.ParseAccessToken(ctx, wrongAlgorithm); err == nil {
		t.Fatal("expected HS384 access token to be rejected")
	}

	wrongIssuerClaims := *claims
	wrongIssuerClaims.Issuer = "other-issuer"
	wrongIssuer, err := jwt.NewWithClaims(jwt.SigningMethodHS256, &wrongIssuerClaims).SignedString([]byte(cfg.AccessTokenSecret))
	if err != nil {
		t.Fatalf("sign wrong-issuer token: %v", err)
	}
	if _, err := svc.ParseAccessToken(ctx, wrongIssuer); err == nil {
		t.Fatal("expected access token from another issuer to be rejected")
	}
}

func TestAdminSessionStateIsBoundedWithinTokenTTL(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	admin, err := store.CreateAdmin(ctx, domainadminauth.AdminUser{
		Email:        "bounded-sessions@example.com",
		PasswordHash: HashPassword("refresh-password"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	svc := NewService(testAuthConfig(), store)
	session, err := svc.Login(ctx, domainadminauth.LoginRequest{Email: admin.Email, Password: "refresh-password"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	for range 100 {
		session, err = svc.Refresh(ctx, session.RefreshToken)
		if err != nil {
			t.Fatalf("Refresh: %v", err)
		}
	}
	for range 24 {
		if _, err := svc.issueSession(admin); err != nil {
			t.Fatalf("issueSession: %v", err)
		}
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if got := len(svc.familyRefreshes); got > 16 {
		t.Fatalf("admin refresh family count = %d, want at most 16", got)
	}
	for familyID, sessions := range svc.familyRefreshes {
		if len(sessions) > 64 {
			t.Fatalf("refresh sessions in family %s = %d, want at most 64", familyID, len(sessions))
		}
	}
	if got := len(svc.refreshesByHash); got > 16*64 {
		t.Fatalf("refresh token hash count = %d, want at most %d", got, 16*64)
	}
	if got := len(svc.sessions); got > 16*64 {
		t.Fatalf("access session count = %d, want at most %d", got, 16*64)
	}
}

func TestServiceLoginIssuesTokenWithRoleAndStatus(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	hash := HashPasswordForTest("correct horse battery staple", "fixed-salt")
	if _, err := store.CreateAdmin(ctx, domainadminauth.AdminUser{
		Email:        "Admin@Example.COM",
		PasswordHash: hash,
		Role:         "super_admin",
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}

	svc := NewService(testAuthConfig(), store)
	session, err := svc.Login(ctx, domainadminauth.LoginRequest{Email: "admin@example.com", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if session.AccessToken == "" || session.SessionID == "" {
		t.Fatalf("expected access token and session id, got %#v", session)
	}

	claims, err := svc.ParseAccessToken(ctx, session.AccessToken)
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}
	if claims.Email != "admin@example.com" || claims.Role != "super_admin" || claims.Status != "active" {
		t.Fatalf("unexpected claims %#v", claims)
	}
	if claims.AdminID == 0 {
		t.Fatalf("expected admin id in claims")
	}
}

func TestMemoryStoreDefaultsAdminRoleToBuiltInAdmin(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	admin, err := store.CreateAdmin(ctx, domainadminauth.AdminUser{
		Email:        "default-role@example.com",
		PasswordHash: HashPasswordForTest("password", "fixed-salt"),
		Status:       "active",
	})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}

	if admin.Role != domainadminauth.RoleAdmin {
		t.Fatalf("default admin role = %q, want %q", admin.Role, domainadminauth.RoleAdmin)
	}
}

func TestServiceRejectsWrongPasswordAndDisabledAdmin(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	if _, err := store.CreateAdmin(ctx, domainadminauth.AdminUser{
		Email:        "ops@example.com",
		PasswordHash: HashPasswordForTest("good-password", "fixed-salt"),
		Role:         "ops_admin",
		Status:       "disabled",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}

	svc := NewService(testAuthConfig(), store)
	if _, err := svc.Login(ctx, domainadminauth.LoginRequest{Email: "ops@example.com", Password: "bad-password"}); err == nil {
		t.Fatalf("expected wrong password to fail")
	}
	if _, err := svc.Login(ctx, domainadminauth.LoginRequest{Email: "ops@example.com", Password: "good-password"}); err == nil {
		t.Fatalf("expected disabled admin to fail")
	}
}

func TestHashPasswordUsesBcryptAndLoginLocksAfterRepeatedFailures(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	hash := HashPassword("good-password")
	if !strings.HasPrefix(hash, "bcrypt$") {
		t.Fatalf("expected bcrypt hash prefix, got %q", hash)
	}
	if !VerifyPassword(hash, "good-password") || VerifyPassword(hash, "bad-password") {
		t.Fatalf("VerifyPassword did not validate bcrypt hash correctly")
	}
	if _, err := store.CreateAdmin(ctx, domainadminauth.AdminUser{
		Email:        "locked@example.com",
		PasswordHash: hash,
		Role:         "ops_admin",
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}

	svc := NewService(testAuthConfig(), store)
	for i := 0; i < maxFailedLoginAttempts; i++ {
		if _, err := svc.Login(ctx, domainadminauth.LoginRequest{Email: "locked@example.com", Password: "bad-password"}); err == nil {
			t.Fatalf("attempt %d should fail", i+1)
		}
	}
	if _, err := svc.Login(ctx, domainadminauth.LoginRequest{Email: "locked@example.com", Password: "good-password"}); err == nil {
		t.Fatalf("expected login to stay locked after repeated failures")
	}
}

func TestLoginRehashesLegacyPasswordAfterSuccessfulLogin(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	legacyHash := HashPasswordForTest("legacy-password", "fixed-salt")
	admin, err := store.CreateAdmin(ctx, domainadminauth.AdminUser{
		Email:        "legacy@example.com",
		PasswordHash: legacyHash,
		Role:         "ops_admin",
		Status:       "active",
	})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}

	svc := NewService(testAuthConfig(), store)
	if _, err := svc.Login(ctx, domainadminauth.LoginRequest{Email: "legacy@example.com", Password: "legacy-password"}); err != nil {
		t.Fatalf("Login: %v", err)
	}
	reloaded, err := store.GetAdminByID(ctx, admin.ID)
	if err != nil {
		t.Fatalf("GetAdminByID: %v", err)
	}
	if !strings.HasPrefix(reloaded.PasswordHash, "bcrypt$") {
		t.Fatalf("expected legacy password hash to be rehashed to bcrypt, got %q", reloaded.PasswordHash)
	}
	if !VerifyPassword(reloaded.PasswordHash, "legacy-password") {
		t.Fatalf("expected rehashed password to verify")
	}
}

func TestConcurrentLegacyLoginsDoNotFailWhenOneRehashCASLoses(t *testing.T) {
	ctx := context.Background()
	legacyHash := HashPasswordForTest("legacy-password", "fixed-salt")
	store := newBlockingRehashStore(domainadminauth.AdminUser{
		ID:           1,
		Email:        "legacy-concurrent@example.com",
		PasswordHash: legacyHash,
		Role:         "ops_admin",
		Status:       "active",
	})

	svc := NewService(testAuthConfig(), store)
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for i := 0; i < cap(errCh); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Login(ctx, domainadminauth.LoginRequest{Email: "legacy-concurrent@example.com", Password: "legacy-password"})
			errCh <- err
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent legacy Login should not fail on stale rehash CAS: %v", err)
		}
	}
	reloaded, err := store.GetAdminByID(ctx, 1)
	if err != nil {
		t.Fatalf("GetAdminByID: %v", err)
	}
	if !strings.HasPrefix(reloaded.PasswordHash, "bcrypt$") || !VerifyPassword(reloaded.PasswordHash, "legacy-password") {
		t.Fatalf("expected final password hash to be bcrypt and valid, got %q", reloaded.PasswordHash)
	}
	if store.staleCount() != 1 {
		t.Fatalf("expected exactly one stale rehash CAS loser, got %d", store.staleCount())
	}
}

type blockingRehashStore struct {
	mu       sync.Mutex
	admin    domainadminauth.AdminUser
	barrier  chan struct{}
	attempts int
	stale    int
}

type getAdminByIDErrorStore struct {
	Store
	err error
}

type passwordResetRaceStore struct {
	Store
	finalReadStarted chan struct{}
	passwordUpdated  chan struct{}
	finalReadOnce    sync.Once
	passwordOnce     sync.Once
}

func newPasswordResetRaceStore(store Store) *passwordResetRaceStore {
	return &passwordResetRaceStore{
		Store:            store,
		finalReadStarted: make(chan struct{}),
		passwordUpdated:  make(chan struct{}),
	}
}

func (s *passwordResetRaceStore) GetAdminByID(ctx context.Context, id int64) (domainadminauth.AdminUser, error) {
	s.finalReadOnce.Do(func() { close(s.finalReadStarted) })
	select {
	case <-s.passwordUpdated:
	case <-time.After(time.Second):
		return domainadminauth.AdminUser{}, errors.New("timed out waiting for password reset")
	}
	return s.Store.GetAdminByID(ctx, id)
}

func (s *passwordResetRaceStore) UpdateAdminPassword(ctx context.Context, id int64, passwordHash string) error {
	err := s.Store.UpdateAdminPassword(ctx, id, passwordHash)
	s.passwordOnce.Do(func() { close(s.passwordUpdated) })
	return err
}

type concurrentRefreshStore struct {
	Store
	mu             sync.Mutex
	blocking       bool
	refreshStarted chan struct{}
	release        chan struct{}
	startOnce      sync.Once
	releaseOnce    sync.Once
}

func newConcurrentRefreshStore(store Store) *concurrentRefreshStore {
	return &concurrentRefreshStore{Store: store, refreshStarted: make(chan struct{}), release: make(chan struct{})}
}

func (s *concurrentRefreshStore) releaseRefresh() {
	s.releaseOnce.Do(func() { close(s.release) })
}

func (s *concurrentRefreshStore) enableBlocking() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blocking = true
}

func (s *concurrentRefreshStore) GetAdminByID(ctx context.Context, id int64) (domainadminauth.AdminUser, error) {
	s.mu.Lock()
	if !s.blocking {
		s.mu.Unlock()
		return s.Store.GetAdminByID(ctx, id)
	}
	s.startOnce.Do(func() { close(s.refreshStarted) })
	release := s.release
	s.mu.Unlock()
	select {
	case <-release:
	case <-time.After(time.Second):
		return domainadminauth.AdminUser{}, errors.New("timed out waiting for concurrent refresh")
	}
	return s.Store.GetAdminByID(ctx, id)
}

func (s *getAdminByIDErrorStore) GetAdminByID(ctx context.Context, id int64) (domainadminauth.AdminUser, error) {
	if s.err != nil {
		return domainadminauth.AdminUser{}, s.err
	}
	return s.Store.GetAdminByID(ctx, id)
}

func newBlockingRehashStore(admin domainadminauth.AdminUser) *blockingRehashStore {
	return &blockingRehashStore{admin: admin, barrier: make(chan struct{})}
}

func (s *blockingRehashStore) GetAdminByEmail(_ context.Context, email string) (domainadminauth.AdminUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if normalizeEmail(email) != s.admin.Email {
		return domainadminauth.AdminUser{}, errors.New("not found")
	}
	return s.admin, nil
}

func (s *blockingRehashStore) ListAdmins(_ context.Context, _ domainadminauth.AdminListRequest) (domainadminauth.AdminListPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return domainadminauth.AdminListPage{Items: []domainadminauth.AdminUser{s.admin}, Page: 1, PageSize: 20, Total: 1}, nil
}

func (s *blockingRehashStore) GetAdminByID(_ context.Context, id int64) (domainadminauth.AdminUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id != s.admin.ID {
		return domainadminauth.AdminUser{}, errors.New("not found")
	}
	return s.admin, nil
}

func (s *blockingRehashStore) CreateAdmin(_ context.Context, admin domainadminauth.AdminUser) (domainadminauth.AdminUser, error) {
	return admin, nil
}

func (s *blockingRehashStore) UpdateAdmin(_ context.Context, id int64, role string, status string, setRole bool, setStatus bool) (domainadminauth.AdminUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id != s.admin.ID {
		return domainadminauth.AdminUser{}, errors.New("not found")
	}
	if setRole {
		s.admin.Role = role
	}
	if setStatus {
		s.admin.Status = status
	}
	return s.admin, nil
}

func (s *blockingRehashStore) UpdateAdminPassword(_ context.Context, id int64, newHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id != s.admin.ID {
		return errors.New("not found")
	}
	s.admin.PasswordHash = newHash
	return nil
}

func (s *blockingRehashStore) UpdateAdminPasswordHash(_ context.Context, id int64, oldHash string, newHash string) error {
	s.mu.Lock()
	s.attempts++
	if s.attempts == 2 {
		close(s.barrier)
	}
	s.mu.Unlock()
	select {
	case <-s.barrier:
	case <-time.After(time.Second):
		return errors.New("timed out waiting for concurrent rehash attempt")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if id != s.admin.ID || s.admin.PasswordHash != oldHash {
		s.stale++
		return errors.New("stale hash")
	}
	s.admin.PasswordHash = newHash
	return nil
}

func (s *blockingRehashStore) DeleteAdmin(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id != s.admin.ID {
		return errors.New("not found")
	}
	s.admin = domainadminauth.AdminUser{}
	return nil
}

func (s *blockingRehashStore) CountActiveSuperAdmins(_ context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.admin.Role == domainadminauth.RoleSuperAdmin && s.admin.Status == "active" {
		return 1, nil
	}
	return 0, nil
}

func (s *blockingRehashStore) staleCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stale
}

func testAuthConfig() config.AuthConfig {
	return config.AuthConfig{
		AccessTokenSecret: "admin-test-secret",
		AccessTokenTTL:    15 * time.Minute,
		Issuer:            "pic-gallery-test",
	}
}
