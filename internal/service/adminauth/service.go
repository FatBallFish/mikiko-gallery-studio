package adminauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
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
