package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainauth "github.com/fatballfish/pic-gallery/internal/domain/auth"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
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
	SessionID    string `json:"session_id"`
	jwt.RegisteredClaims
}

type Service struct {
	mu              sync.Mutex
	cfg             config.AuthConfig
	store           Store
	emailSender     EmailSender
	userMultipliers map[string]string
	nextUserID      int64
	usersByEmail    map[string]*domainauth.User
	usersByID       map[int64]*domainauth.User
	codesByEmail    map[string]emailCode
	sessionsByHash  map[string]*refreshSession
	familySessions  map[string][]*refreshSession
}

type EmailSender interface {
	SendVerificationCode(email, scene, code string) error
}

type SMTPEmailSender struct {
	cfg config.SMTPConfig
}

func NewSMTPEmailSender(cfg config.SMTPConfig) *SMTPEmailSender {
	return &SMTPEmailSender{cfg: cfg}
}

func (s *SMTPEmailSender) SendVerificationCode(email, scene, code string) error {
	if !smtpConfigured(s.cfg) {
		return emailDeliveryConfigError()
	}
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("connect smtp server: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return fmt.Errorf("create smtp client: %w", err)
	}
	defer client.Close()

	if err := client.Hello("localhost"); err != nil {
		return fmt.Errorf("smtp hello: %w", err)
	}
	if s.cfg.StartTLS {
		tlsCfg := &tls.Config{ServerName: s.cfg.Host, InsecureSkipVerify: s.cfg.InsecureSkipVerify}
		if err := client.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}
	if strings.TrimSpace(s.cfg.Username) != "" {
		auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(envelopeAddress(s.cfg.From)); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := client.Rcpt(email); err != nil {
		return fmt.Errorf("smtp rcpt to: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	message := verificationEmailMessage(s.cfg.From, email, scene, code)
	if _, err := writer.Write([]byte(message)); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write smtp data: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close smtp data: %w", err)
	}
	return client.Quit()
}

func NewService(cfg config.AuthConfig, userMultipliers map[string]string) *Service {
	return NewServiceWithStore(cfg, userMultipliers, nil)
}

func NewServiceWithStore(cfg config.AuthConfig, userMultipliers map[string]string, store Store) *Service {
	var sender EmailSender
	if smtpConfigured(cfg.SMTP) {
		sender = NewSMTPEmailSender(cfg.SMTP)
	}
	return &Service{
		cfg:             cfg,
		store:           store,
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
	svc := NewServiceWithStore(cfg, userMultipliers, nil)
	svc.emailSender = sender
	return svc
}

func ValidateProductionEmailCodeConfig(env string, cfg config.AuthConfig) error {
	if !isProductionEnv(env) {
		return nil
	}
	if strings.TrimSpace(os.Getenv("PIC_GALLERY_AUTH_FIXED_CODE")) != "" {
		return fmt.Errorf("PIC_GALLERY_AUTH_FIXED_CODE is not allowed in %s env", env)
	}
	if enabled, err := strconv.ParseBool(os.Getenv("PIC_GALLERY_AUTH_DEV_EMAIL_CODES")); err == nil && enabled {
		return fmt.Errorf("PIC_GALLERY_AUTH_DEV_EMAIL_CODES is not allowed in %s env", env)
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
	existing := s.codesByEmail[email]
	now := time.Now()
	if !existing.LastSentAt.IsZero() && now.Sub(existing.LastSentAt) < time.Minute {
		return errs.New(http.StatusTooManyRequests, errs.CodeRateLimited, "verification code was sent recently")
	}
	code := fixedEmailCode(s.cfg)
	if code == "" {
		generated, err := randomEmailCode()
		if err != nil {
			return errs.Internal("failed to generate verification code")
		}
		code = generated
		if s.emailSender == nil {
			return emailDeliveryConfigError()
		}
		if err := s.emailSender.SendVerificationCode(email, scene, code); err != nil {
			if appErr, ok := err.(*errs.Error); ok {
				return appErr
			}
			return errs.Internal("failed to send email verification code")
		}
	}
	s.codesByEmail[email] = emailCode{Code: code, Scene: scene, ExpiresAt: now.Add(10 * time.Minute), LastSentAt: now}
	return nil
}

func fixedEmailCode(cfg config.AuthConfig) string {
	if code := strings.TrimSpace(os.Getenv("PIC_GALLERY_AUTH_FIXED_CODE")); code != "" {
		return code
	}
	if enabled, err := strconv.ParseBool(os.Getenv("PIC_GALLERY_AUTH_DEV_EMAIL_CODES")); err == nil && enabled {
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
	return strings.TrimSpace(cfg.Host) != "" && cfg.Port > 0 && strings.TrimSpace(cfg.From) != ""
}

func emailDeliveryConfigError() *errs.Error {
	return errs.Internal("email verification SMTP delivery is not configured: set auth.smtp.host, auth.smtp.port, and auth.smtp.from")
}

func verificationEmailMessage(from, to, scene, code string) string {
	subject := "Pic Gallery verification code"
	body := fmt.Sprintf("Your Pic Gallery verification code is %s. It expires in 10 minutes.", code)
	if scene != "" {
		body = fmt.Sprintf("Your Pic Gallery verification code for %s is %s. It expires in 10 minutes.", scene, code)
	}
	headers := []string{
		"From: " + sanitizeHeader(from),
		"To: " + sanitizeHeader(to),
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
	}
	return strings.Join(headers, "\r\n") + "\r\n\r\n" + body + "\r\n"
}

func sanitizeHeader(value string) string {
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, "\n", "")
	return value
}

func envelopeAddress(value string) string {
	parsed, err := mail.ParseAddress(strings.TrimSpace(value))
	if err != nil {
		return strings.TrimSpace(value)
	}
	return parsed.Address
}

func (s *Service) LoginWithEmailCode(email, code string) (domainauth.User, domainauth.Session, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.codesByEmail[email]
	now := time.Now()
	if ok && !record.LockedUntil.IsZero() && now.Before(record.LockedUntil) {
		return domainauth.User{}, domainauth.Session{}, errs.New(http.StatusTooManyRequests, errs.CodeRateLimited, "too many invalid verification attempts")
	}
	if !ok || record.Code != strings.TrimSpace(code) || now.After(record.ExpiresAt) {
		if ok {
			record.FailedAttempts++
			if record.FailedAttempts >= 5 {
				record.LockedUntil = now.Add(15 * time.Minute)
			}
			s.codesByEmail[email] = record
		}
		return domainauth.User{}, domainauth.Session{}, errs.Unauthorized("invalid or expired verification code")
	}
	delete(s.codesByEmail, email)

	user, err := s.getUserByEmailLocked(email)
	if err != nil && err != repoerr.ErrNotFound {
		return domainauth.User{}, domainauth.Session{}, err
	}
	if err == repoerr.ErrNotFound {
		created, err := s.createUserLocked(email)
		if err != nil {
			return domainauth.User{}, domainauth.Session{}, err
		}
		user = &created
	}
	if user.Status == "disabled" {
		return domainauth.User{}, domainauth.Session{}, errs.New(403, errs.CodeUserDisabled, "user has been disabled")
	}
	return *user, s.issueSessionLocked(user), nil
}

func (s *Service) Refresh(refreshToken string) (domainauth.User, domainauth.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	hash := hashToken(refreshToken)
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
	current.Status = "rotated"
	newSession := s.issueSessionWithFamilyLocked(&user, current.FamilyID)
	current.ReplacedBySessionID = newSession.SessionID
	if s.store != nil {
		if err := s.store.MarkRefreshSessionRotated(context.Background(), current.ID, newSession.SessionID); err != nil {
			return domainauth.User{}, domainauth.Session{}, err
		}
	}
	return user, newSession, nil
}

func (s *Service) Logout(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(sessionID) == "" {
		return nil
	}
	if s.store != nil {
		return s.store.MarkRefreshSessionRevoked(context.Background(), sessionID)
	}
	for _, session := range s.sessionsByHash {
		if session.ID == sessionID && session.Status == "active" {
			session.Status = "revoked"
		}
	}
	return nil
}

func (s *Service) ParseAccessToken(accessToken string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(accessToken, &Claims{}, func(token *jwt.Token) (any, error) {
		return []byte(s.cfg.AccessTokenSecret), nil
	})
	if err != nil {
		return nil, errs.New(401, errs.CodeAuthAccessExpired, "access token expired or invalid")
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errs.New(401, errs.CodeAuthAccessExpired, "access token expired or invalid")
	}
	return claims, nil
}

func (s *Service) ValidateAccessToken(accessToken string) (domainauth.User, *Claims, error) {
	claims, err := s.ParseAccessToken(accessToken)
	if err != nil {
		return domainauth.User{}, nil, err
	}
	user, ok := s.GetUserByID(claims.UserID)
	if !ok {
		return domainauth.User{}, nil, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "user not found")
	}
	if user.Status == "disabled" {
		return domainauth.User{}, nil, errs.New(http.StatusForbidden, errs.CodeUserDisabled, "user has been disabled")
	}
	if user.TokenVersion != claims.TokenVersion {
		return domainauth.User{}, nil, errs.New(http.StatusUnauthorized, errs.CodeAuthAccessExpired, "access token revoked")
	}
	return user, claims, nil
}

func (s *Service) GetUserByID(id int64) (domainauth.User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getUserByIDLocked(id)
}

func (s *Service) UpdateProfile(userID int64, nickname, bio, theme, locale, avatarObjectKey *string) (domainauth.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.getUserByIDLocked(userID)
	if !ok {
		return domainauth.User{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "user not found")
	}
	if nickname != nil {
		value := strings.TrimSpace(*nickname)
		if len([]rune(value)) < 2 || len([]rune(value)) > 30 {
			return domainauth.User{}, errs.BadRequest("nickname must be 2-30 characters")
		}
		user.Nickname = value
	}
	if bio != nil {
		value := strings.TrimSpace(*bio)
		if len([]rune(value)) > 120 {
			return domainauth.User{}, errs.BadRequest("bio must be up to 120 characters")
		}
		user.Bio = value
	}
	if theme != nil {
		value := strings.TrimSpace(*theme)
		if value != "light" && value != "dark" && value != "system" {
			return domainauth.User{}, errs.BadRequest("invalid theme")
		}
		user.Theme = value
	}
	if locale != nil {
		user.DefaultLocale = defaultString(strings.TrimSpace(*locale), "zh-CN")
	}
	if avatarObjectKey != nil {
		user.AvatarObjectKey = strings.TrimSpace(*avatarObjectKey)
	}
	return s.saveUserLocked(user)
}

func (s *Service) ChangePassword(userID int64, _ string, newPassword string) error {
	if strings.TrimSpace(newPassword) == "" {
		return errs.BadRequest("new password is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.getUserByIDLocked(userID)
	if !ok {
		return errs.New(http.StatusNotFound, errs.CodeNotFound, "user not found")
	}
	user.TokenVersion++
	if _, err := s.saveUserLocked(user); err != nil {
		return err
	}
	return nil
}

func (s *Service) ResetPassword(email, code, newPassword string) error {
	user, _, err := s.LoginWithEmailCode(email, code)
	if err != nil {
		return err
	}
	return s.ChangePassword(user.ID, "", newPassword)
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
	accessExp := time.Now().Add(s.cfg.AccessTokenTTL)
	refreshExp := time.Now().Add(s.cfg.RefreshTokenTTL)
	claims := Claims{
		UserID: user.ID, Email: user.Email, TokenVersion: user.TokenVersion, GroupCode: user.GroupCode, SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{Subject: fmt.Sprintf("%d", user.ID), ExpiresAt: jwt.NewNumericDate(accessExp), IssuedAt: jwt.NewNumericDate(time.Now()), Issuer: s.cfg.Issuer},
	}
	accessToken, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.AccessTokenSecret))
	session := &refreshSession{ID: sessionID, FamilyID: familyID, UserID: user.ID, RefreshTokenHash: refreshHash, Status: "active", ExpiresAt: refreshExp}
	s.sessionsByHash[refreshHash] = session
	s.familySessions[familyID] = append(s.familySessions[familyID], session)
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

func (s *Service) revokeFamilyLocked(familyID string) {
	if s.store != nil {
		_ = s.store.MarkFamilyReplayBlocked(context.Background(), familyID)
	}
	for _, session := range s.familySessions[familyID] {
		if session.Status == "active" || session.Status == "rotated" {
			session.Status = "replay_blocked"
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
	user := domainauth.User{
		ID:              s.nextUserID,
		Email:           email,
		Nickname:        fmt.Sprintf("user-%d", s.nextUserID),
		Status:          "active",
		GroupCode:       "basic",
		GroupMultiplier: s.userMultiplierFor("basic"),
		DefaultLocale:   "zh-CN",
		Theme:           "system",
		CreatedAt:       time.Now(),
	}
	if s.store != nil {
		return s.store.CreateUser(context.Background(), user)
	}
	s.nextUserID++
	s.usersByEmail[email] = &user
	s.usersByID[user.ID] = &user
	return user, nil
}

func (s *Service) saveUserLocked(user domainauth.User) (domainauth.User, error) {
	if s.store != nil {
		return s.store.UpdateUser(context.Background(), user)
	}
	existing := user
	s.usersByID[user.ID] = &existing
	s.usersByEmail[user.Email] = &existing
	return existing, nil
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

func randomInt(max int) int {
	if max <= 0 {
		return 0
	}
	buffer := make([]byte, 4)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	value := int(buffer[0])<<24 | int(buffer[1])<<16 | int(buffer[2])<<8 | int(buffer[3])
	if value < 0 {
		value = -value
	}
	return value % max
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
