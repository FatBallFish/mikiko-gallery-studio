package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
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
	Code      string
	Scene     string
	ExpiresAt time.Time
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
	jwt.RegisteredClaims
}

type Service struct {
	mu              sync.Mutex
	cfg             config.AuthConfig
	store           Store
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
	return &Service{
		cfg:             cfg,
		store:           store,
		userMultipliers: userMultipliers,
		nextUserID:      1,
		usersByEmail:    map[string]*domainauth.User{},
		usersByID:       map[int64]*domainauth.User{},
		codesByEmail:    map[string]emailCode{},
		sessionsByHash:  map[string]*refreshSession{},
		familySessions:  map[string][]*refreshSession{},
	}
}

func (s *Service) SendEmailCode(email, scene string) error {
	if email == "" || scene == "" {
		return errs.BadRequest("email and scene are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.codesByEmail[email] = emailCode{Code: "123456", Scene: scene, ExpiresAt: time.Now().Add(10 * time.Minute)}
	return nil
}

func (s *Service) LoginWithEmailCode(email, code string) (domainauth.User, domainauth.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.codesByEmail[email]
	if !ok || record.Code != code || time.Now().After(record.ExpiresAt) {
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

func (s *Service) GetUserByID(id int64) (domainauth.User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getUserByIDLocked(id)
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
		UserID: user.ID, Email: user.Email, TokenVersion: user.TokenVersion, GroupCode: user.GroupCode,
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
