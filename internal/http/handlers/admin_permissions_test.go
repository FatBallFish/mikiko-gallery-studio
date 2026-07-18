package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
)

func TestRequireAdminPermissionUsesRoleFacade(t *testing.T) {
	cfg := config.Config{Auth: config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
	}}
	store := adminauthservice.NewMemoryStore()
	if _, err := store.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "ops@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, store)
	session, err := adminAuth.Login(t.Context(), domainadminauth.LoginRequest{Email: "ops@example.com", Password: "password"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	api := NewAPIWithCompletionServices(cfg, nil, nil, nil, nil, nil, nil, adminAuth, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/users", nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)

	if _, appErr := api.requireAdminPermission(req, domainadminauth.PermissionManageUsers); appErr != nil {
		t.Fatalf("expected admin to manage users, got %v", appErr)
	}
	if _, appErr := api.requireAdminPermission(req, domainadminauth.PermissionManageAdmins); appErr == nil || appErr.StatusCode != http.StatusForbidden {
		t.Fatalf("expected admin to be forbidden from managing admins, got %#v", appErr)
	}
}

func TestAdminPermissionResolverCanBeReplacedForFutureRBAC(t *testing.T) {
	cfg := config.Config{Auth: config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
	}}
	store := adminauthservice.NewMemoryStore()
	if _, err := store.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "ops@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, store)
	session, err := adminAuth.Login(t.Context(), domainadminauth.LoginRequest{Email: "ops@example.com", Password: "password"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	api := NewAPIWithCompletionServices(cfg, nil, nil, nil, nil, nil, nil, adminAuth, nil)
	resolver := &denyManageUsersResolver{}
	api.SetAdminPermissionResolver(resolver)

	req := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/users", nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)

	if _, appErr := api.requireAdminPermission(req, domainadminauth.PermissionManageUsers); appErr == nil || appErr.StatusCode != http.StatusForbidden {
		t.Fatalf("expected injected resolver to deny manage users, got %#v", appErr)
	}
	if !resolver.sawPrincipal {
		t.Fatalf("expected injected resolver to receive full admin principal")
	}
	if got := api.adminPermissionsForPrincipal(domainadminauth.AdminPrincipal{ID: 1001, Email: "ops@example.com", Role: domainadminauth.RoleAdmin, Status: "active"}); containsString(got, string(domainadminauth.PermissionManageUsers)) {
		t.Fatalf("expected injected resolver to be used for permission listing, got %#v", got)
	}
}

func TestAdminLogoutUsesPermissionFacade(t *testing.T) {
	cfg := config.Config{Auth: config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
	}}
	store := adminauthservice.NewMemoryStore()
	if _, err := store.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "ops@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, store)
	session, err := adminAuth.Login(t.Context(), domainadminauth.LoginRequest{Email: "ops@example.com", Password: "password"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	api := NewAPIWithCompletionServices(cfg, nil, nil, nil, nil, nil, nil, adminAuth, nil)
	resolver := &denyReadOnlyResolver{}
	api.SetAdminPermissionResolver(resolver)

	req := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	rec := httptest.NewRecorder()
	api.HandleAdminLogout(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected logout to be denied by injected permission resolver, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !resolver.sawReadOnly {
		t.Fatalf("expected logout to check read-only permission through facade")
	}
}

func TestAdminLogoutRevokesBearerFamilyWhenAdminLookupFails(t *testing.T) {
	cfg := config.Config{Auth: config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
	}}
	memoryStore := adminauthservice.NewMemoryStore()
	if _, err := memoryStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "ops@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	store := &logoutLookupErrorStore{Store: memoryStore}
	adminAuth := adminauthservice.NewService(cfg.Auth, store)
	session, err := adminAuth.Login(t.Context(), domainadminauth.LoginRequest{Email: "ops@example.com", Password: "password"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	store.err = errors.New("temporary database outage")
	api := NewAPIWithCompletionServices(cfg, nil, nil, nil, nil, nil, nil, adminAuth, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	rec := httptest.NewRecorder()

	api.HandleAdminLogout(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected transient lookup failure to remain visible, got %d body=%s", rec.Code, rec.Body.String())
	}
	store.err = nil
	if _, err := adminAuth.ParseAccessToken(t.Context(), session.AccessToken); err == nil {
		t.Fatal("expected logout to revoke the bearer session family despite the lookup failure")
	}
}

func TestAdminHandlersUsePermissionFacadeContract(t *testing.T) {
	content, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("read api.go: %v", err)
	}
	source := string(content)
	matches := regexp.MustCompile(`func \(a \*API\) (HandleAdmin\w+)\(`).FindAllStringSubmatchIndex(source, -1)
	if len(matches) == 0 {
		t.Fatal("expected admin handlers in api.go")
	}

	for _, match := range matches {
		name := source[match[2]:match[3]]
		if name == "HandleAdminLogin" || name == "HandleAdminRefresh" {
			continue
		}
		body, ok := functionBody(source, match[0])
		if !ok {
			t.Fatalf("could not parse %s body", name)
		}
		if strings.Contains(body, "requireAdmin(") || strings.Contains(body, "requireAdminWithQueryToken(") {
			t.Fatalf("%s must not call raw admin authentication helpers directly; use permission facade", name)
		}
		if !strings.Contains(body, "requireAdminPermission(") && !strings.Contains(body, "requireAdminPermissionWithQueryToken(") {
			t.Fatalf("%s must check access through permission facade", name)
		}
	}
}

type denyManageUsersResolver struct {
	sawPrincipal bool
}

type logoutLookupErrorStore struct {
	adminauthservice.Store
	err error
}

func (s *logoutLookupErrorStore) GetAdminByID(ctx context.Context, id int64) (domainadminauth.AdminUser, error) {
	if s.err != nil {
		return domainadminauth.AdminUser{}, s.err
	}
	return s.Store.GetAdminByID(ctx, id)
}

func (r *denyManageUsersResolver) HasPermission(principal domainadminauth.AdminPrincipal, permission domainadminauth.Permission) bool {
	if principal.ID > 0 && principal.Email == "ops@example.com" && principal.Role == domainadminauth.RoleAdmin {
		r.sawPrincipal = true
	}
	return permission != domainadminauth.PermissionManageUsers
}

func (*denyManageUsersResolver) PermissionsForPrincipal(principal domainadminauth.AdminPrincipal) []domainadminauth.Permission {
	defaultPermissions := domainadminauth.RolePermissionResolver{}.PermissionsForPrincipal(principal)
	permissions := make([]domainadminauth.Permission, 0, len(defaultPermissions))
	for _, permission := range defaultPermissions {
		if permission != domainadminauth.PermissionManageUsers {
			permissions = append(permissions, permission)
		}
	}
	return permissions
}

type denyReadOnlyResolver struct {
	sawReadOnly bool
}

func (r *denyReadOnlyResolver) HasPermission(_ domainadminauth.AdminPrincipal, permission domainadminauth.Permission) bool {
	if permission == domainadminauth.PermissionReadOnly {
		r.sawReadOnly = true
		return false
	}
	return true
}

func (*denyReadOnlyResolver) PermissionsForPrincipal(principal domainadminauth.AdminPrincipal) []domainadminauth.Permission {
	return domainadminauth.RolePermissionResolver{}.PermissionsForPrincipal(principal)
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func functionBody(source string, funcStart int) (string, bool) {
	open := strings.IndexByte(source[funcStart:], '{')
	if open < 0 {
		return "", false
	}
	bodyStart := funcStart + open
	depth := 0
	for i := bodyStart; i < len(source); i++ {
		switch source[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return source[bodyStart : i+1], true
			}
		}
	}
	return "", false
}
