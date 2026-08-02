package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainauth "github.com/fatballfish/pic-gallery/internal/domain/auth"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/internal/service/smtpdelivery"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type emailCode struct {
	Code           string
	Scene          string
	ExpiresAt      time.Time
	LastSentAt     time.Time
	FailedAttempts int
	LockedUntil    time.Time
}

type refreshSession struct {
	ID                  string
	FamilyID            string
	UserID              int64
	RefreshTokenHash    string
	Status              string
	ExpiresAt           time.Time
	ReplacedBySessionID string
}

type Claims struct {
	UserID       int64  `json:"uid"`
	Email        string `json:"email"`
	TokenVersion int    `json:"token_version"`
	GroupCode    string `json:"group_code"`
	Purpose      string `json:"purpose"`
	jwt.RegisteredClaims
}

type LoginResult struct {
	User                   domainauth.User
	Session                domainauth.Session
	Created                bool
	PasswordSetupRequired  bool
	PasswordSetupToken     string
	PasswordSetupExpiresAt time.Time
}

const (
	accessTokenPurpose        = "access"
	passwordSetupTokenPurpose = "password_setup"
	passwordSetupTokenTTL     = 10 * time.Minute
)

type EmailSender interface {
	SendVerificationCode(email, scene, code string) error
}

type SMTPConfigResolver interface {
	ResolveSMTPConfig(ctx context.Context) (config.SMTPConfig, bool, error)
}

type SMTPEmailSender struct {
	cfg config.SMTPConfig
}

func NewSMTPEmailSender(cfg config.SMTPConfig) *SMTPEmailSender {
	return &SMTPEmailSender{cfg: cfg}
}

func (s *SMTPEmailSender) SendVerificationCode(email, scene, code string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return smtpdelivery.SendVerificationCode(ctx, s.cfg, email, scene, code)
}

type Service struct {
	mu              sync.Mutex
	cfg             config.AuthConfig
	store           Store
	redisRuntime    RedisRuntime
	allowFallback   bool
	emailSender     EmailSender
	smtpResolver    SMTPConfigResolver
	userMultipliers map[string]string
	nextUserID      int64
	usersByEmail    map[string]*domainauth.User
	usersByID       map[int64]*domainauth.User
	codesByEmail    map[string]emailCode
	sessionsByHash  map[string]*refreshSession
	familySessions  map[string][]*refreshSession
}

func NewService(cfg config.AuthConfig, userMultipliers map[string]string) *Service {
	return NewServiceWithStore(cfg, userMultipliers, nil)
}

func NewServiceWithStore(cfg config.AuthConfig, userMultipliers map[string]string, store Store) *Service {
	return NewServiceWithStoreAndRedis(cfg, userMultipliers, store, nil, true)
}

func NewServiceWithStoreAndRedis(cfg config.AuthConfig, userMultipliers map[string]string, store Store, redisRuntime RedisRuntime, allowFallback bool) *Service {
	var sender EmailSender
	if smtpConfigured(cfg.SMTP) {
		sender = NewSMTPEmailSender(cfg.SMTP)
	}
	return &Service{
		cfg:             cfg,
		store:           store,
		redisRuntime:    redisRuntime,
		allowFallback:   allowFallback,
		emailSender:     sender,
		userMultipliers: userMultipliers,
		nextUserID:      1,
		usersByEmail:    map[string]*domainauth.User{},
		usersByID:       map[int64]*domainauth.User{},
		codesByEmail:    map[string]emailCode{},
		sessionsByHash:  map[string]*refreshSession{},
		familySessions:  map[string][]*refreshSession{},
	}
}

func NewServiceWithEmailSender(cfg config.AuthConfig, userMultipliers map[string]string, sender EmailSender) *Service {
	svc := NewServiceWithStoreAndRedis(cfg, userMultipliers, nil, nil, true)
	svc.emailSender = sender
	return svc
}

func (s *Service) SetSMTPConfigResolver(resolver SMTPConfigResolver) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.smtpResolver = resolver
}

func ValidateProductionEmailCodeConfig(env string, cfg config.AuthConfig) error {
	if !isProductionEnv(env) {
		return nil
	}
	if strings.TrimSpace(cfg.FixedEmailCode) != "" {
		return fmt.Errorf("auth.fixed_email_code is not allowed in %s env", env)
	}
	if cfg.DevEmailCodes {
		return fmt.Errorf("auth.dev_email_codes is not allowed in %s env", env)
	}
	if strings.EqualFold(strings.TrimSpace(cfg.Issuer), "test") {
		return fmt.Errorf("auth.issuer=test is not allowed in %s env because it enables fixed email codes", env)
	}
	return nil
}

func (s *Service) SendEmailCode(email, scene string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	scene = strings.TrimSpace(scene)
	if email == "" || scene == "" || !strings.Contains(email, "@") {
		return errs.BadRequest("email and scene are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if s.redisRuntime != nil {
		active, err := s.redisRuntime.EmailCooldownActive(context.Background(), email)
		if err != nil {
			if !s.handleRedisFallbackLocked("send email code cooldown check", err) {
				return errs.Internal("failed to verify email code cooldown")
			}
		} else if active {
			return errs.New(http.StatusTooManyRequests, errs.CodeRateLimited, "verification code resend cooldown is active")
		}
	}
	current := s.codesByEmail[email]
	if (s.redisRuntime == nil || s.allowFallback) && !current.LastSentAt.IsZero() && now.Sub(current.LastSentAt) < time.Minute {
		return errs.New(http.StatusTooManyRequests, errs.CodeRateLimited, "verification code resend cooldown is active")
	}
	code := fixedEmailCode(s.cfg)
	if code == "" {
		generated, err := randomEmailCode()
		if err != nil {
			return errs.Internal("failed to generate verification code")
		}
		code = generated
		sender, err := s.emailSenderLocked(context.Background())
		if err != nil {
			return err
		}
		if sender == nil {
			return emailDeliveryConfigError()
		}
		if err := sender.SendVerificationCode(email, scene, code); err != nil {
			if appErr, ok := err.(*errs.Error); ok {
				return appErr
			}
			return errs.Internal("failed to send email verification code")
		}
	}
	current.Code = code
	current.Scene = scene
	current.ExpiresAt = now.Add(10 * time.Minute)
	current.LastSentAt = now
	current.FailedAttempts = 0
	current.LockedUntil = time.Time{}
	if s.redisRuntime != nil {
		if err := s.redisRuntime.StoreEmailCode(context.Background(), email, current, 10*time.Minute, time.Minute); err != nil {
			if !s.handleRedisFallbackLocked("store email code", err) {
				return errs.Internal("failed to persist verification code")
			}
		}
	}
	s.codesByEmail[email] = current
	return nil
}

func (s *Service) emailSenderLocked(ctx context.Context) (EmailSender, error) {
	if s.smtpResolver != nil {
		cfg, ok, err := s.smtpResolver.ResolveSMTPConfig(ctx)
		if err != nil {
			return nil, err
		}
		if ok {
			return NewSMTPEmailSender(cfg), nil
		}
	}
	return s.emailSender, nil
}

func fixedEmailCode(cfg config.AuthConfig) string {
	if code := strings.TrimSpace(cfg.FixedEmailCode); code != "" {
		return code
	}
	if cfg.DevEmailCodes {
		return "123456"
	}
	if cfg.Issuer == "test" {
		return "123456"
	}
	return ""
}

func isProductionEnv(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "prod", "production":
		return true
	default:
		return false
	}
}

func randomEmailCode() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", value.Int64()), nil
}

func smtpConfigured(cfg config.SMTPConfig) bool {
	return smtpdelivery.Configured(cfg)
}

func emailDeliveryConfigError() *errs.Error {
	return errs.Internal(smtpdelivery.ConfigError().Error())
}

func (s *Service) LoginWithEmailCode(email, code string) (domainauth.User, domainauth.Session, error) {
	result, err := s.LoginWithEmailCodeResult(email, code)
	if err != nil {
		return domainauth.User{}, domainauth.Session{}, err
	}
	return result.User, result.Session, nil
}

func (s *Service) LoginWithEmailCodeResult(email, code string) (LoginResult, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	code = strings.TrimSpace(code)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.consumeEmailCodeLocked(email, code, "login"); err != nil {
		return LoginResult{}, err
	}

	user, err := s.getUserByEmailLocked(email)
	if err != nil && err != repoerr.ErrNotFound {
		return LoginResult{}, err
	}
	createdUser := false
	if err == repoerr.ErrNotFound {
		created, err := s.createUserLocked(email)
		if err != nil {
			return LoginResult{}, err
		}
		user = &created
		createdUser = true
	}
	if user.Status == "disabled" {
		return LoginResult{}, errs.New(403, errs.CodeUserDisabled, "user has been disabled")
	}
	if user.Status == "closed" {
		return LoginResult{}, errs.New(403, errs.CodeForbidden, "user account has been closed")
	}
	if user.PasswordHash == "" {
		setupToken, setupExpiresAt, err := s.issuePasswordSetupTokenLocked(user)
		if err != nil {
			return LoginResult{}, err
		}
		return LoginResult{
			User:                   *user,
			Created:                createdUser,
			PasswordSetupRequired:  true,
			PasswordSetupToken:     setupToken,
			PasswordSetupExpiresAt: setupExpiresAt,
		}, nil
	}
	return LoginResult{User: *user, Session: s.issueSessionLocked(user), Created: createdUser}, nil
}

func (s *Service) LoginWithPassword(email, password string) (domainauth.User, domainauth.Session, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	password = strings.TrimSpace(password)
	if email == "" || password == "" {
		return domainauth.User{}, domainauth.Session{}, errs.BadRequest("email and password are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	user, err := s.getUserByEmailLocked(email)
	if err != nil {
		if err == repoerr.ErrNotFound {
			return domainauth.User{}, domainauth.Session{}, errs.Unauthorized("invalid email or password")
		}
		return domainauth.User{}, domainauth.Session{}, err
	}
	if user.Status == "disabled" {
		return domainauth.User{}, domainauth.Session{}, errs.New(403, errs.CodeUserDisabled, "user has been disabled")
	}
	if user.Status == "closed" {
		return domainauth.User{}, domainauth.Session{}, errs.New(403, errs.CodeForbidden, "user account has been closed")
	}
	if user.PasswordHash == "" || !verifyPassword(user.PasswordHash, password) {
		return domainauth.User{}, domainauth.Session{}, errs.Unauthorized("invalid email or password")
	}
	return *user, s.issueSessionLocked(user), nil
}

func (s *Service) ChangePassword(userID int64, oldPassword, newPassword string) (domainauth.User, error) {
	oldPassword = strings.TrimSpace(oldPassword)
	if err := validateNewPassword(newPassword); err != nil {
		return domainauth.User{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.getUserByIDLocked(userID)
	if !ok {
		return domainauth.User{}, errs.New(404, errs.CodeNotFound, "user not found")
	}
	if user.PasswordHash == "" || !verifyPassword(user.PasswordHash, oldPassword) {
		return domainauth.User{}, errs.Unauthorized("old password is incorrect")
	}
	return s.updatePasswordLocked(&user, newPassword)
}

func (s *Service) RequestPasswordReset(email string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return errs.BadRequest("email is required")
	}
	if s.store != nil {
		user, err := s.store.GetUserByEmail(context.Background(), email)
		if err != nil {
			if err == repoerr.ErrNotFound {
				return nil
			}
			return err
		}
		if user.Status == "closed" {
			return nil
		}
	}
	return s.SendEmailCode(email, "password_reset")
}

func (s *Service) ConfirmPasswordReset(email, code, newPassword string) (domainauth.User, error) {
	if err := validateNewPassword(newPassword); err != nil {
		return domainauth.User{}, err
	}
	email = strings.TrimSpace(strings.ToLower(email))
	code = strings.TrimSpace(code)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.consumeEmailCodeLocked(email, code, "password_reset"); err != nil {
		return domainauth.User{}, err
	}
	user, err := s.getUserByEmailLocked(email)
	if err != nil {
		if err == repoerr.ErrNotFound {
			return domainauth.User{}, errs.New(404, errs.CodeNotFound, "user not found")
		}
		return domainauth.User{}, err
	}
	return s.updatePasswordLocked(user, newPassword)
}

func (s *Service) SetPassword(userID int64, newPassword string) (domainauth.User, error) {
	if err := validateNewPassword(newPassword); err != nil {
		return domainauth.User{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.getUserByIDLocked(userID)
	if !ok {
		return domainauth.User{}, errs.New(404, errs.CodeNotFound, "user not found")
	}
	return s.updatePasswordLocked(&user, newPassword)
}

func (s *Service) CompletePasswordSetup(setupToken, newPassword string) (domainauth.User, domainauth.Session, error) {
	if err := validateNewPassword(newPassword); err != nil {
		return domainauth.User{}, domainauth.Session{}, err
	}
	claims, err := s.parseTokenForPurpose(setupToken, passwordSetupTokenPurpose)
	if err != nil {
		return domainauth.User{}, domainauth.Session{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.getUserByIDLocked(claims.UserID)
	if !ok || !strings.EqualFold(user.Email, claims.Email) || user.TokenVersion != claims.TokenVersion || user.PasswordHash != "" {
		return domainauth.User{}, domainauth.Session{}, errs.Unauthorized("password setup token expired or invalid")
	}
	if user.Status == "disabled" {
		return domainauth.User{}, domainauth.Session{}, errs.New(403, errs.CodeUserDisabled, "user has been disabled")
	}
	if user.Status == "closed" {
		return domainauth.User{}, domainauth.Session{}, errs.New(403, errs.CodeForbidden, "user account has been closed")
	}
	updated, err := s.updatePasswordLocked(&user, newPassword)
	if err != nil {
		return domainauth.User{}, domainauth.Session{}, err
	}
	session := s.issueSessionLocked(&updated)
	return updated, session, nil
}

func (s *Service) CloseAccount(userID int64) (domainauth.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.getUserByIDLocked(userID)
	if !ok {
		return domainauth.User{}, errs.New(404, errs.CodeNotFound, "user not found")
	}
	closedAt := time.Now().UTC()
	if s.store != nil {
		updated, err := s.store.MarkUserClosed(context.Background(), userID, closedAt)
		if err != nil {
			return domainauth.User{}, err
		}
		if err := s.store.RevokeRefreshSessionsByUser(context.Background(), userID); err != nil {
			return domainauth.User{}, err
		}
		s.revokeUserSessionsLocked(userID)
		return updated, nil
	}
	user.Status = "closed"
	user.ClosedAt = &closedAt
	user.TokenVersion++
	s.usersByID[userID] = &user
	s.usersByEmail[user.Email] = &user
	s.revokeUserSessionsLocked(userID)
	return user, nil
}

func (s *Service) RevokeUserSessions(userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revokeUserSessionsLocked(userID)
	if s.store != nil {
		return s.store.RevokeRefreshSessionsByUser(context.Background(), userID)
	}
	return nil
}

func (s *Service) Refresh(refreshToken string) (domainauth.User, domainauth.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	hash := hashToken(refreshToken)
	if s.redisRuntime != nil {
		cached, ok, err := s.redisRuntime.LoadRefreshTokenState(context.Background(), hash)
		if err != nil {
			if !s.handleRedisFallbackLocked("load refresh token state", err) {
				return domainauth.User{}, domainauth.Session{}, errs.Internal("failed to load refresh session state")
			}
		} else if ok && cached.Status != "active" {
			if cached.FamilyID != "" {
				s.revokeFamilyLocked(cached.FamilyID)
			}
			return domainauth.User{}, domainauth.Session{}, errs.New(401, errs.CodeAuthRefreshReplayBlocked, "refresh token replay detected")
		}
	}
	current, ok := s.sessionsByHash[hash]
	if s.store != nil {
		record, err := s.store.GetRefreshSessionByHash(context.Background(), hash)
		if err != nil {
			if err == repoerr.ErrNotFound {
				return domainauth.User{}, domainauth.Session{}, errs.New(401, errs.CodeAuthRefreshExpired, "refresh token expired")
			}
			return domainauth.User{}, domainauth.Session{}, err
		}
		current = &refreshSession{
			ID:                  record.ID,
			FamilyID:            record.FamilyID,
			UserID:              record.UserID,
			RefreshTokenHash:    record.RefreshTokenHash,
			Status:              record.Status,
			ExpiresAt:           time.Unix(record.ExpiresAt, 0),
			ReplacedBySessionID: record.ReplacedBySessionID,
		}
		if s.redisRuntime != nil {
			blocked, err := s.redisRuntime.IsRefreshFamilyReplayBlocked(context.Background(), current.FamilyID)
			if err != nil {
				if !s.handleRedisFallbackLocked("check refresh family replay state", err) {
					return domainauth.User{}, domainauth.Session{}, errs.Internal("failed to verify refresh session family state")
				}
			} else if blocked {
				return domainauth.User{}, domainauth.Session{}, errs.New(401, errs.CodeAuthRefreshReplayBlocked, "refresh token replay detected")
			}
		}
		ok = true
	}
	if !ok {
		return domainauth.User{}, domainauth.Session{}, errs.New(401, errs.CodeAuthRefreshExpired, "refresh token expired")
	}
	if current.Status != "active" {
		s.revokeFamilyLocked(current.FamilyID)
		return domainauth.User{}, domainauth.Session{}, errs.New(401, errs.CodeAuthRefreshReplayBlocked, "refresh token replay detected")
	}
	if time.Now().After(current.ExpiresAt) {
		current.Status = "expired"
		s.persistRefreshTokenStateLocked(*current)
		if s.store != nil {
			if err := s.store.MarkRefreshSessionExpired(context.Background(), current.ID); err != nil {
				return domainauth.User{}, domainauth.Session{}, err
			}
		}
		return domainauth.User{}, domainauth.Session{}, errs.New(401, errs.CodeAuthRefreshExpired, "refresh token expired")
	}
	user, ok := s.getUserByIDLocked(current.UserID)
	if !ok {
		return domainauth.User{}, domainauth.Session{}, errs.New(401, errs.CodeAuthRefreshExpired, "refresh token expired")
	}
	if user.Status == "disabled" {
		return domainauth.User{}, domainauth.Session{}, errs.New(403, errs.CodeUserDisabled, "user has been disabled")
	}
	if user.Status == "closed" {
		return domainauth.User{}, domainauth.Session{}, errs.New(403, errs.CodeForbidden, "user account has been closed")
	}
	current.Status = "rotated"
	newSession := s.issueSessionWithFamilyLocked(&user, current.FamilyID)
	current.ReplacedBySessionID = newSession.SessionID
	s.persistRefreshTokenStateLocked(*current)
	if s.store != nil {
		if err := s.store.MarkRefreshSessionRotated(context.Background(), current.ID, newSession.SessionID); err != nil {
			return domainauth.User{}, domainauth.Session{}, err
		}
	}
	return user, newSession, nil
}

func (s *Service) ParseAccessToken(accessToken string) (*Claims, error) {
	claims, err := s.parseTokenForPurpose(accessToken, accessTokenPurpose)
	if err != nil {
		return nil, errs.New(401, errs.CodeAuthAccessExpired, "access token expired or invalid")
	}
	return claims, nil
}

func (s *Service) GetUserByID(id int64) (domainauth.User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getUserByIDLocked(id)
}

func (s *Service) Logout(refreshToken string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if refreshToken == "" {
		return nil
	}
	hash := hashToken(refreshToken)
	if current, ok := s.sessionsByHash[hash]; ok {
		current.Status = "revoked"
		s.persistRefreshTokenStateLocked(*current)
	}
	if s.store != nil {
		return s.store.RevokeRefreshSessionByHash(context.Background(), hash)
	}
	return nil
}

func (s *Service) UpdateProfile(req domainauth.UpdateProfileRequest) (domainauth.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if req.UserID <= 0 {
		return domainauth.User{}, errs.BadRequest("user_id is required")
	}
	if req.Nickname == "" {
		req.Nickname = fmt.Sprintf("user-%d", req.UserID)
	}
	if req.DefaultLocale == "" {
		req.DefaultLocale = "zh-CN"
	}
	if req.Theme == "" {
		req.Theme = "system"
	}
	if s.store != nil {
		return s.store.UpdateUserProfile(context.Background(), req)
	}
	user, ok := s.usersByID[req.UserID]
	if !ok {
		return domainauth.User{}, errs.New(404, errs.CodeNotFound, "user not found")
	}
	user.Nickname = req.Nickname
	user.Bio = req.Bio
	user.AvatarObjectKey = req.AvatarObjectKey
	user.DefaultLocale = req.DefaultLocale
	user.Theme = req.Theme
	return *user, nil
}

func (s *Service) DisableUserForTest(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if user, ok := s.usersByID[id]; ok {
		user.Status = "disabled"
		user.TokenVersion++
	}
}

func (s *Service) userMultiplierFor(groupCode string) string {
	if value, ok := s.userMultipliers[groupCode]; ok {
		return value
	}
	return "1.00000"
}

func (s *Service) issueSessionLocked(user *domainauth.User) domainauth.Session {
	return s.issueSessionWithFamilyLocked(user, uuid.NewString())
}

func (s *Service) issueSessionWithFamilyLocked(user *domainauth.User, familyID string) domainauth.Session {
	sessionID := uuid.NewString()
	refreshToken := randomToken()
	refreshHash := hashToken(refreshToken)
	accessTTL := s.cfg.AccessTokenTTL
	if accessTTL <= 0 {
		accessTTL = 10 * time.Minute
	}
	refreshTTL := s.cfg.RefreshTokenTTL
	if refreshTTL <= 0 {
		refreshTTL = 30 * time.Minute
	}
	accessExp := time.Now().Add(accessTTL)
	refreshExp := time.Now().Add(refreshTTL)
	claims := Claims{
		UserID: user.ID, Email: user.Email, TokenVersion: user.TokenVersion, GroupCode: user.GroupCode, Purpose: accessTokenPurpose,
		RegisteredClaims: jwt.RegisteredClaims{Subject: fmt.Sprintf("%d", user.ID), ExpiresAt: jwt.NewNumericDate(accessExp), IssuedAt: jwt.NewNumericDate(time.Now()), Issuer: s.cfg.Issuer},
	}
	accessToken, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.AccessTokenSecret))
	session := &refreshSession{ID: sessionID, FamilyID: familyID, UserID: user.ID, RefreshTokenHash: refreshHash, Status: "active", ExpiresAt: refreshExp}
	s.sessionsByHash[refreshHash] = session
	s.familySessions[familyID] = append(s.familySessions[familyID], session)
	s.persistRefreshTokenStateLocked(*session)
	if s.store != nil {
		_ = s.store.SaveRefreshSession(context.Background(), entstore.RefreshSessionRecord{
			ID:               session.ID,
			FamilyID:         session.FamilyID,
			UserID:           session.UserID,
			RefreshTokenHash: session.RefreshTokenHash,
			Status:           session.Status,
			ExpiresAt:        session.ExpiresAt.Unix(),
		})
	}
	return domainauth.Session{AccessToken: accessToken, AccessTokenExpiresAt: accessExp, RefreshToken: refreshToken, RefreshTokenExpiresAt: refreshExp, RefreshCookieName: s.cfg.RefreshCookieName, SessionID: sessionID, SessionFamilyID: familyID}
}

func (s *Service) issuePasswordSetupTokenLocked(user *domainauth.User) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(passwordSetupTokenTTL)
	claims := Claims{
		UserID: user.ID, Email: user.Email, TokenVersion: user.TokenVersion, Purpose: passwordSetupTokenPurpose,
		RegisteredClaims: jwt.RegisteredClaims{Subject: fmt.Sprintf("%d", user.ID), ExpiresAt: jwt.NewNumericDate(expiresAt), IssuedAt: jwt.NewNumericDate(now), Issuer: s.cfg.Issuer},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.AccessTokenSecret))
	if err != nil {
		return "", time.Time{}, errs.Internal("failed to issue password setup token")
	}
	return token, expiresAt, nil
}

func (s *Service) parseTokenForPurpose(rawToken, purpose string) (*Claims, error) {
	options := []jwt.ParserOption{jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()})}
	if issuer := strings.TrimSpace(s.cfg.Issuer); issuer != "" {
		options = append(options, jwt.WithIssuer(issuer))
	}
	token, err := jwt.ParseWithClaims(strings.TrimSpace(rawToken), &Claims{}, func(token *jwt.Token) (any, error) {
		return []byte(s.cfg.AccessTokenSecret), nil
	}, options...)
	if err != nil {
		return nil, errs.Unauthorized("token expired or invalid")
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errs.Unauthorized("token expired or invalid")
	}
	validPurpose := claims.Purpose == purpose || (purpose == accessTokenPurpose && claims.Purpose == "")
	if !validPurpose || claims.UserID <= 0 || strings.TrimSpace(claims.Email) == "" {
		return nil, errs.Unauthorized("token expired or invalid")
	}
	return claims, nil
}

func (s *Service) revokeFamilyLocked(familyID string) {
	familyTTL := s.cfg.RefreshTokenTTL
	if familyTTL <= 0 {
		familyTTL = 30 * time.Minute
	}
	if s.redisRuntime != nil {
		if err := s.redisRuntime.MarkRefreshFamilyReplayBlocked(context.Background(), familyID, familyTTL); err != nil {
			_ = s.handleRedisFallbackLocked("mark refresh family replay blocked", err)
		}
	}
	if s.store != nil {
		_ = s.store.MarkFamilyReplayBlocked(context.Background(), familyID)
	}
	for _, session := range s.familySessions[familyID] {
		if session.Status == "active" || session.Status == "rotated" {
			session.Status = "replay_blocked"
			s.persistRefreshTokenStateLocked(*session)
		}
	}
}

func (s *Service) getUserByEmailLocked(email string) (*domainauth.User, error) {
	if s.store != nil {
		user, err := s.store.GetUserByEmail(context.Background(), email)
		if err != nil {
			return nil, err
		}
		return &user, nil
	}
	user, ok := s.usersByEmail[email]
	if !ok {
		return nil, repoerr.ErrNotFound
	}
	return user, nil
}

func (s *Service) createUserLocked(email string) (domainauth.User, error) {
	now := time.Now()
	user := domainauth.User{
		ID:              s.nextUserID,
		Email:           email,
		Nickname:        defaultNicknameFromEmail(email),
		Status:          "active",
		GroupCode:       "basic",
		GroupCodes:      []string{"basic"},
		GroupMultiplier: s.userMultiplierFor("basic"),
		DefaultLocale:   "zh-CN",
		Theme:           "system",
		CreatedAt:       now,
		EmailVerifiedAt: &now,
	}
	if s.store != nil {
		return s.store.CreateUser(context.Background(), user)
	}
	s.nextUserID++
	s.usersByEmail[email] = &user
	s.usersByID[user.ID] = &user
	return user, nil
}

func defaultNicknameFromEmail(email string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	at := strings.IndexByte(normalized, '@')
	if at <= 0 {
		return "user"
	}
	localPart := strings.TrimSpace(normalized[:at])
	if localPart == "" {
		return "user"
	}
	runes := []rune(localPart)
	if len(runes) > 64 {
		runes = runes[:64]
	}
	return string(runes)
}

func (s *Service) getUserByIDLocked(id int64) (domainauth.User, bool) {
	if s.store != nil {
		user, err := s.store.GetUserByID(context.Background(), id)
		if err != nil {
			return domainauth.User{}, false
		}
		return user, true
	}
	user, ok := s.usersByID[id]
	if !ok {
		return domainauth.User{}, false
	}
	return *user, true
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomToken() string {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer)
}

func (s *Service) consumeEmailCodeLocked(email, code string, allowedScenes ...string) error {
	record, ok, err := s.loadEmailCodeLocked(email, allowedScenes...)
	if err != nil {
		return err
	}
	now := time.Now()
	if ok && !record.LockedUntil.IsZero() && now.Before(record.LockedUntil) {
		return errs.New(429, errs.CodeRateLimited, "too many invalid verification attempts")
	}
	if !ok || record.Code != code || now.After(record.ExpiresAt) || !containsScene(record.Scene, allowedScenes) {
		if ok {
			record.FailedAttempts++
			if record.FailedAttempts >= 5 {
				record.LockedUntil = now.Add(15 * time.Minute)
			}
			s.persistEmailCodeLocked(email, record)
			s.codesByEmail[email] = record
		}
		return errs.Unauthorized("invalid or expired verification code")
	}
	if err := s.deleteEmailCodeLocked(email, allowedScenes...); err != nil {
		return err
	}
	delete(s.codesByEmail, email)
	return nil
}

func containsScene(scene string, allowedScenes []string) bool {
	if len(allowedScenes) == 0 {
		return true
	}
	for _, allowed := range allowedScenes {
		if strings.EqualFold(strings.TrimSpace(scene), strings.TrimSpace(allowed)) {
			return true
		}
	}
	return false
}

func (s *Service) updatePasswordLocked(user *domainauth.User, newPassword string) (domainauth.User, error) {
	now := time.Now().UTC()
	passwordHash := hashPassword(newPassword)
	if s.store != nil {
		updated, err := s.store.UpdatePasswordHash(context.Background(), user.ID, passwordHash, now)
		if err != nil {
			return domainauth.User{}, err
		}
		if err := s.store.RevokeRefreshSessionsByUser(context.Background(), user.ID); err != nil {
			return domainauth.User{}, err
		}
		s.revokeUserSessionsLocked(user.ID)
		return updated, nil
	}
	user.PasswordHash = passwordHash
	user.PasswordUpdatedAt = &now
	user.TokenVersion++
	s.usersByID[user.ID] = user
	s.usersByEmail[user.Email] = user
	s.revokeUserSessionsLocked(user.ID)
	return *user, nil
}

func (s *Service) revokeUserSessionsLocked(userID int64) {
	for _, session := range s.sessionsByHash {
		if session.UserID == userID && session.Status != "expired" {
			session.Status = "revoked"
			s.persistRefreshTokenStateLocked(*session)
		}
	}
}

func (s *Service) loadEmailCodeLocked(email string, allowedScenes ...string) (emailCode, bool, error) {
	if s.redisRuntime != nil {
		record, ok, err := s.redisRuntime.LoadEmailCode(context.Background(), email, allowedScenes)
		if err != nil {
			if !s.handleRedisFallbackLocked("load email code", err) {
				return emailCode{}, false, errs.Internal("failed to load verification code")
			}
		} else if ok {
			return record, true, nil
		}
	}
	record, ok := s.codesByEmail[email]
	return record, ok, nil
}

func (s *Service) persistEmailCodeLocked(email string, record emailCode) {
	if s.redisRuntime == nil {
		return
	}
	if err := s.redisRuntime.StoreEmailCode(context.Background(), email, record, emailCodeTTL(record), time.Minute); err != nil {
		_ = s.handleRedisFallbackLocked("persist email code", err)
	}
}

func (s *Service) deleteEmailCodeLocked(email string, allowedScenes ...string) error {
	if s.redisRuntime == nil {
		return nil
	}
	if err := s.redisRuntime.DeleteEmailCodes(context.Background(), email, allowedScenes); err != nil {
		if !s.handleRedisFallbackLocked("delete email code", err) {
			return errs.Internal("failed to clear verification code")
		}
	}
	return nil
}

func (s *Service) persistRefreshTokenStateLocked(session refreshSession) {
	if s.redisRuntime == nil {
		return
	}
	ttl := time.Until(session.ExpiresAt)
	if ttl <= 0 {
		ttl = time.Second
	}
	if err := s.redisRuntime.StoreRefreshTokenState(context.Background(), session.RefreshTokenHash, session, ttl); err != nil {
		_ = s.handleRedisFallbackLocked("persist refresh token state", err)
	}
}

func (s *Service) handleRedisFallbackLocked(operation string, err error) bool {
	if err == nil || !s.allowFallback {
		return false
	}
	slog.Warn("auth redis operation failed; using in-memory fallback", "operation", operation, "err", err)
	return true
}

func emailCodeTTL(record emailCode) time.Duration {
	until := record.ExpiresAt
	if record.LockedUntil.After(until) {
		until = record.LockedUntil
	}
	ttl := time.Until(until)
	if ttl <= 0 {
		return time.Second
	}
	return ttl
}

func validateNewPassword(password string) error {
	password = strings.TrimSpace(password)
	if len(password) < 8 {
		return errs.BadRequest("new_password must be at least 8 characters")
	}
	return nil
}

func hashPassword(password string) string {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return "bcrypt$" + string(hashed)
}

func verifyPassword(encodedHash string, password string) bool {
	if !strings.HasPrefix(encodedHash, "bcrypt$") {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(strings.TrimPrefix(encodedHash, "bcrypt$")), []byte(password)) == nil
}
