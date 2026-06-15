package adminauth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
)

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
