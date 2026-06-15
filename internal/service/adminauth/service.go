package adminauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"sync"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

const (
	passwordHashPrefix       = "sha256"
	bcryptPasswordHashPrefix = "bcrypt"
	maxFailedLoginAttempts   = 5
	failedLoginLockDuration  = 15 * time.Minute
)

type Claims struct {
	AdminID   int64  `json:"aid"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}

type memorySession struct {
	ID        string
	AdminID   int64
	Status    string
	ExpiresAt time.Time
}

type loginFailure struct {
	Count       int
	LockedUntil time.Time
}

type Service struct {
	mu       sync.Mutex
	cfg      config.AuthConfig
	store    Store
	sessions map[string]memorySession
	failures map[string]loginFailure
}

func NewService(cfg config.AuthConfig, store Store) *Service {
	if store == nil {
		store = NewMemoryStore()
	}
	return &Service{cfg: cfg, store: store, sessions: map[string]memorySession{}, failures: map[string]loginFailure{}}
}

func (s *Service) ListAdmins(ctx context.Context, req domainadminauth.AdminListRequest) (domainadminauth.AdminListPage, error) {
	req.Page, req.PageSize = normalizeAdminPage(req.Page, req.PageSize)
	req.Query = normalizeEmail(req.Query)
	rawRole := strings.TrimSpace(req.Role)
	req.Role = normalizeAdminRole(rawRole)
	if rawRole != "" && req.Role == "" {
		return domainadminauth.AdminListPage{}, errs.BadRequest("invalid role")
	}
	rawStatus := strings.TrimSpace(req.Status)
	req.Status = normalizeAdminStatus(rawStatus)
	if rawStatus != "" && req.Status == "" {
		return domainadminauth.AdminListPage{}, errs.BadRequest("invalid status")
	}
	return s.store.ListAdmins(ctx, req)
}

func (s *Service) CreateAdmin(ctx context.Context, req domainadminauth.AdminCreateRequest) (domainadminauth.AdminUser, error) {
	email := normalizeEmail(req.Email)
	if email == "" {
		return domainadminauth.AdminUser{}, errs.BadRequest("email is required")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return domainadminauth.AdminUser{}, errs.BadRequest("email is invalid")
	}
	password := strings.TrimSpace(req.Password)
	if len(password) < 6 {
		return domainadminauth.AdminUser{}, errs.BadRequest("password must be at least 6 characters")
	}
	role := normalizeAdminRole(req.Role)
	if role == "" {
		if strings.TrimSpace(req.Role) != "" {
			return domainadminauth.AdminUser{}, errs.BadRequest("invalid role")
		}
		role = domainadminauth.RoleAdmin
	}
	status := normalizeAdminStatus(req.Status)
	if status == "" {
		if strings.TrimSpace(req.Status) != "" {
			return domainadminauth.AdminUser{}, errs.BadRequest("invalid status")
		}
		status = "active"
	}
	admin, err := s.store.CreateAdmin(ctx, domainadminauth.AdminUser{
		Email:        email,
		PasswordHash: HashPassword(password),
		Role:         role,
		Status:       status,
	})
	if errors.Is(err, repoerr.ErrConflict) {
		return domainadminauth.AdminUser{}, errs.New(409, errs.CodeConflict, "admin already exists")
	}
	return admin, err
}

func (s *Service) UpdateAdmin(ctx context.Context, req domainadminauth.AdminUpdateRequest) (domainadminauth.AdminUser, error) {
	if req.AdminID <= 0 {
		return domainadminauth.AdminUser{}, errs.BadRequest("invalid admin_id")
	}
	role := ""
	if req.RoleProvided {
		role = normalizeAdminRole(req.Role)
		if role == "" {
			return domainadminauth.AdminUser{}, errs.BadRequest("invalid role")
		}
	}
	status := ""
	if req.StatusProvided {
		status = normalizeAdminStatus(req.Status)
		if status == "" {
			return domainadminauth.AdminUser{}, errs.BadRequest("invalid status")
		}
	}
	current, err := s.store.GetAdminByID(ctx, req.AdminID)
	if err != nil {
		return domainadminauth.AdminUser{}, normalizeAdminStoreError(err, "admin not found")
	}
	if req.AdminID == req.OperatorID && req.StatusProvided && status != "active" {
		return domainadminauth.AdminUser{}, errs.BadRequest("cannot disable current admin")
	}
	if req.AdminID == req.OperatorID && req.RoleProvided && role != current.Role {
		return domainadminauth.AdminUser{}, errs.BadRequest("cannot change current admin role")
	}
	if activeSuperAdminWouldBeRemoved(current, role, status, req.RoleProvided, req.StatusProvided) {
		if count, err := s.store.CountActiveSuperAdmins(ctx); err != nil {
			return domainadminauth.AdminUser{}, err
		} else if count <= 1 {
			return domainadminauth.AdminUser{}, errs.New(409, errs.CodeConflict, "cannot remove the last active super admin")
		}
	}
	updated, err := s.store.UpdateAdmin(ctx, req.AdminID, role, status, req.RoleProvided, req.StatusProvided)
	return updated, normalizeAdminStoreError(err, "admin not found")
}

func (s *Service) ResetAdminPassword(ctx context.Context, req domainadminauth.AdminPasswordResetRequest) error {
	if req.AdminID <= 0 {
		return errs.BadRequest("invalid admin_id")
	}
	password := strings.TrimSpace(req.NewPassword)
	if len(password) < 6 {
		return errs.BadRequest("new_password must be at least 6 characters")
	}
	return normalizeAdminStoreError(s.store.UpdateAdminPassword(ctx, req.AdminID, HashPassword(password)), "admin not found")
}

func (s *Service) DeleteAdmin(ctx context.Context, req domainadminauth.AdminDeleteRequest) (domainadminauth.AdminUser, error) {
	if req.AdminID <= 0 {
		return domainadminauth.AdminUser{}, errs.BadRequest("invalid admin_id")
	}
	if req.AdminID == req.OperatorID {
		return domainadminauth.AdminUser{}, errs.BadRequest("cannot delete current admin")
	}
	current, err := s.store.GetAdminByID(ctx, req.AdminID)
	if err != nil {
		return domainadminauth.AdminUser{}, normalizeAdminStoreError(err, "admin not found")
	}
	if current.Role == domainadminauth.RoleSuperAdmin && current.Status == "active" {
		if count, err := s.store.CountActiveSuperAdmins(ctx); err != nil {
			return domainadminauth.AdminUser{}, err
		} else if count <= 1 {
			return domainadminauth.AdminUser{}, errs.New(409, errs.CodeConflict, "cannot remove the last active super admin")
		}
	}
	if err := s.store.DeleteAdmin(ctx, req.AdminID); err != nil {
		return domainadminauth.AdminUser{}, normalizeAdminStoreError(err, "admin not found")
	}
	return current, nil
}

func (s *Service) Login(ctx context.Context, req domainadminauth.LoginRequest) (domainadminauth.Session, error) {
	email := normalizeEmail(req.Email)
	if email == "" || req.Password == "" {
		return domainadminauth.Session{}, errs.BadRequest("email and password are required")
	}
	if err := s.checkLoginLock(email); err != nil {
		return domainadminauth.Session{}, err
	}
	admin, err := s.store.GetAdminByEmail(ctx, email)
	if err != nil {
		if err == repoerr.ErrNotFound {
			s.recordFailedLogin(email)
			return domainadminauth.Session{}, errs.Unauthorized("invalid admin credentials")
		}
		return domainadminauth.Session{}, err
	}
	if !VerifyPassword(admin.PasswordHash, req.Password) {
		s.recordFailedLogin(email)
		return domainadminauth.Session{}, errs.Unauthorized("invalid admin credentials")
	}
	if admin.Status != "active" {
		s.recordFailedLogin(email)
		return domainadminauth.Session{}, errs.New(403, errs.CodeForbidden, "admin account is disabled")
	}
	if PasswordNeedsRehash(admin.PasswordHash) {
		_ = s.store.UpdateAdminPasswordHash(ctx, admin.ID, admin.PasswordHash, HashPassword(req.Password))
	}
	s.clearFailedLogin(email)
	return s.issueSession(admin)
}

func (s *Service) ParseAccessToken(ctx context.Context, accessToken string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(accessToken, &Claims{}, func(token *jwt.Token) (any, error) {
		return []byte(s.cfg.AccessTokenSecret), nil
	})
	if err != nil {
		return nil, errs.New(401, errs.CodeAuthAccessExpired, "admin access token expired or invalid")
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errs.New(401, errs.CodeAuthAccessExpired, "admin access token expired or invalid")
	}
	if claims.Role == "" || claims.Status == "" || claims.SessionID == "" {
		return nil, errs.New(401, errs.CodeAuthAccessExpired, "admin access token missing required claims")
	}

	s.mu.Lock()
	session, ok := s.sessions[claims.SessionID]
	s.mu.Unlock()
	if !ok || session.Status != "active" || session.AdminID != claims.AdminID || time.Now().After(session.ExpiresAt) {
		return nil, errs.New(401, errs.CodeAuthAccessExpired, "admin session expired or invalid")
	}
	admin, err := s.store.GetAdminByID(ctx, claims.AdminID)
	if err != nil {
		return nil, errs.New(401, errs.CodeAuthAccessExpired, "admin account not found")
	}
	if admin.Status != claims.Status || admin.Role != claims.Role || admin.Status != "active" {
		return nil, errs.New(401, errs.CodeAuthAccessExpired, "admin token claims are stale")
	}
	return claims, nil
}

func (s *Service) issueSession(admin domainadminauth.AdminUser) (domainadminauth.Session, error) {
	now := time.Now().UTC()
	ttl := s.cfg.AccessTokenTTL
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	expiresAt := now.Add(ttl)
	sessionID := uuid.NewString()
	claims := Claims{
		AdminID:   admin.ID,
		Email:     admin.Email,
		Role:      admin.Role,
		Status:    admin.Status,
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   fmt.Sprintf("%d", admin.ID),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    s.cfg.Issuer,
		},
	}
	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.AccessTokenSecret))
	if err != nil {
		return domainadminauth.Session{}, err
	}
	s.mu.Lock()
	s.sessions[sessionID] = memorySession{ID: sessionID, AdminID: admin.ID, Status: "active", ExpiresAt: expiresAt}
	s.mu.Unlock()
	return domainadminauth.Session{AccessToken: accessToken, AccessTokenExpiresAt: expiresAt, SessionID: sessionID, AdminID: admin.ID, Email: admin.Email, Role: admin.Role, Status: admin.Status}, nil
}

func HashPassword(password string) string {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return bcryptPasswordHashPrefix + "$" + string(hashed)
}

func HashPasswordForTest(password, salt string) string {
	return hashPassword(password, salt)
}

func VerifyPassword(encodedHash string, password string) bool {
	if strings.HasPrefix(encodedHash, bcryptPasswordHashPrefix+"$") {
		hash := strings.TrimPrefix(encodedHash, bcryptPasswordHashPrefix+"$")
		return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
	}
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 3 || parts[0] != passwordHashPrefix || parts[1] == "" || parts[2] == "" {
		return false
	}
	expected := hashPassword(password, parts[1])
	return subtle.ConstantTimeCompare([]byte(expected), []byte(encodedHash)) == 1
}

func PasswordNeedsRehash(encodedHash string) bool {
	return !strings.HasPrefix(encodedHash, bcryptPasswordHashPrefix+"$")
}

func (s *Service) checkLoginLock(email string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	failure := s.failures[email]
	if !failure.LockedUntil.IsZero() && time.Now().Before(failure.LockedUntil) {
		return errs.New(429, errs.CodeRateLimited, "admin login temporarily locked")
	}
	return nil
}

func (s *Service) recordFailedLogin(email string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	failure := s.failures[email]
	if !failure.LockedUntil.IsZero() && time.Now().After(failure.LockedUntil) {
		failure = loginFailure{}
	}
	failure.Count++
	if failure.Count >= maxFailedLoginAttempts {
		failure.LockedUntil = time.Now().Add(failedLoginLockDuration)
	}
	s.failures[email] = failure
}

func (s *Service) clearFailedLogin(email string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.failures, email)
}

func hashPassword(password, salt string) string {
	sum := sha256.Sum256([]byte(salt + ":" + password))
	return passwordHashPrefix + "$" + salt + "$" + base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomSalt() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer)
}

func normalizeAdminPage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func normalizeAdminRole(role string) string {
	switch strings.TrimSpace(role) {
	case "", domainadminauth.RoleAdmin, domainadminauth.RoleSuperAdmin:
		return strings.TrimSpace(role)
	default:
		return ""
	}
}

func normalizeAdminStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "active", "disabled":
		return strings.ToLower(strings.TrimSpace(status))
	default:
		return ""
	}
}

func activeSuperAdminWouldBeRemoved(current domainadminauth.AdminUser, role string, status string, setRole bool, setStatus bool) bool {
	if current.Role != domainadminauth.RoleSuperAdmin || current.Status != "active" {
		return false
	}
	nextRole := current.Role
	if setRole {
		nextRole = role
	}
	nextStatus := current.Status
	if setStatus {
		nextStatus = status
	}
	return nextRole != domainadminauth.RoleSuperAdmin || nextStatus != "active"
}

func normalizeAdminStoreError(err error, notFoundMessage string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, repoerr.ErrNotFound) {
		return errs.New(404, errs.CodeNotFound, notFoundMessage)
	}
	if errors.Is(err, repoerr.ErrConflict) {
		return errs.New(409, errs.CodeConflict, "admin conflict")
	}
	return err
}
