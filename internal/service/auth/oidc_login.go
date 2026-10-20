package auth

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	domainauth "github.com/fatballfish/pic-gallery/internal/domain/auth"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

// OIDCProviderKey is the federated-identity provider identifier stored in
// user_identities (single Mikiko ID deployment today).
const OIDCProviderKey = "mikiko"

// OIDCLoginInput carries the verified OIDC identity claims used for login.
type OIDCLoginInput struct {
	Subject       string
	Email         string
	EmailVerified bool
	DisplayName   string
	Username      string
}

// LocalAuthPolicyResolver exposes the admin-configured login-mode switches.
// Missing resolver keeps every legacy login path open.
type LocalAuthPolicyResolver interface {
	LocalAuthPolicy(ctx context.Context) (allowLocalLogin bool, allowRegistration bool, err error)
}

func (s *Service) localAuthPolicyLocked() (bool, bool) {
	if s.localAuthPolicy == nil {
		return true, true
	}
	allowLogin, allowRegistration, err := s.localAuthPolicy.LocalAuthPolicy(context.Background())
	if err != nil {
		return true, true
	}
	return allowLogin, allowRegistration
}

func localLoginDisabled() *errs.Error {
	return errs.New(403, "LOCAL_LOGIN_DISABLED", "local account login is disabled")
}

func registrationDisabled() *errs.Error {
	return errs.New(403, "REGISTRATION_DISABLED", "account registration is disabled")
}

// LoginWithOIDC maps a verified OIDC identity to a platform session:
// identity hit → login; verified-email match → link; otherwise → auto-register.
// OIDC first-login registration follows the platform SSO contract and is not
// gated by the local registration switch.
func (s *Service) LoginWithOIDC(identity OIDCLoginInput) (LoginResult, error) {
	subject := strings.TrimSpace(identity.Subject)
	if subject == "" {
		return LoginResult{}, errs.BadRequest("oidc subject is required")
	}
	email := strings.TrimSpace(strings.ToLower(identity.Email))

	s.mu.Lock()
	defer s.mu.Unlock()

	if linked, err := s.findUserIdentityLocked(OIDCProviderKey, subject); err == nil {
		user, ok := s.getUserByIDLocked(linked.UserID)
		if !ok {
			return LoginResult{}, errs.New(403, "OIDC_ACCOUNT_UNAVAILABLE", "linked account is no longer available")
		}
		if appErr := rejectInactiveUser(user); appErr != nil {
			return LoginResult{}, appErr
		}
		s.touchUserIdentityLocked(linked, email)
		session, err := s.issueSessionLocked(&user)
		if err != nil {
			return LoginResult{}, err
		}
		return LoginResult{User: user, Session: session}, nil
	} else if err != repoerr.ErrNotFound {
		return LoginResult{}, err
	}

	if email == "" || !strings.Contains(email, "@") {
		return LoginResult{}, errs.New(403, "OIDC_EMAIL_MISSING", "oidc identity has no usable email to link or register")
	}
	user, err := s.getUserByEmailLocked(email)
	if err != nil && err != repoerr.ErrNotFound {
		return LoginResult{}, err
	}
	if err == nil {
		if !identity.EmailVerified {
			return LoginResult{}, errs.New(403, "OIDC_EMAIL_NOT_VERIFIED", "oidc email is not verified; refusing to link an existing account")
		}
		if appErr := rejectInactiveUser(*user); appErr != nil {
			return LoginResult{}, appErr
		}
		if _, err := s.attachUserIdentityLocked(domainauth.UserIdentity{UserID: user.ID, Provider: OIDCProviderKey, Subject: subject, Email: email}); err != nil {
			return LoginResult{}, err
		}
		session, err := s.issueSessionLocked(user)
		if err != nil {
			return LoginResult{}, err
		}
		return LoginResult{User: *user, Session: session}, nil
	}

	created := s.oidcCreateUserLocked(email, identity)
	newUser, err := s.createUserWithIdentityLocked(created, domainauth.UserIdentity{Provider: OIDCProviderKey, Subject: subject, Email: email})
	if err != nil {
		return LoginResult{}, err
	}
	session, err := s.issueSessionLocked(&newUser)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{User: newUser, Session: session, Created: true}, nil
}

func rejectInactiveUser(user domainauth.User) *errs.Error {
	if user.Status == "disabled" {
		return errs.New(403, errs.CodeUserDisabled, "user has been disabled")
	}
	if user.Status == "closed" {
		return errs.New(403, errs.CodeForbidden, "user account has been closed")
	}
	return nil
}

func (s *Service) oidcCreateUserLocked(email string, identity OIDCLoginInput) domainauth.User {
	now := time.Now()
	user := domainauth.User{
		Email:         email,
		Nickname:      oidcNickname(identity, email),
		Status:        "active",
		GroupCode:     "basic",
		GroupCodes:    []string{"basic"},
		GroupMultiplier: s.userMultiplierFor("basic"),
		DefaultLocale: "zh-CN",
		Theme:         "system",
		CreatedAt:     now,
	}
	if identity.EmailVerified {
		user.EmailVerifiedAt = &now
	}
	return user
}

func oidcNickname(identity OIDCLoginInput, email string) string {
	for _, candidate := range []string{identity.Username, identity.DisplayName} {
		nickname := strings.TrimSpace(candidate)
		if nickname == "" {
			continue
		}
		if utf8.RuneCountInString(nickname) > 64 {
			nickname = string([]rune(nickname)[:64])
		}
		return nickname
	}
	return defaultNicknameFromEmail(email)
}

func identityKey(provider, subject string) string {
	return provider + "\x00" + subject
}

func (s *Service) findUserIdentityLocked(provider, subject string) (domainauth.UserIdentity, error) {
	if s.store != nil {
		return s.store.FindUserIdentity(context.Background(), provider, subject)
	}
	identity, ok := s.identitiesByKey[identityKey(provider, subject)]
	if !ok {
		return domainauth.UserIdentity{}, repoerr.ErrNotFound
	}
	return *identity, nil
}

// attachUserIdentityLocked links an identity to an existing user; an already
// known (provider, subject) only refreshes its email snapshot.
func (s *Service) attachUserIdentityLocked(identity domainauth.UserIdentity) (domainauth.UserIdentity, error) {
	if s.store != nil {
		return s.store.AttachUserIdentity(context.Background(), identity)
	}
	key := identityKey(identity.Provider, identity.Subject)
	if existing, ok := s.identitiesByKey[key]; ok {
		if identity.Email != "" {
			existing.Email = identity.Email
		}
		return *existing, nil
	}
	now := time.Now()
	stored := domainauth.UserIdentity{
		ID:        s.nextIdentityID,
		UserID:    identity.UserID,
		Provider:  identity.Provider,
		Subject:   identity.Subject,
		Email:     identity.Email,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.nextIdentityID++
	s.identitiesByKey[key] = &stored
	return stored, nil
}

func (s *Service) touchUserIdentityLocked(identity domainauth.UserIdentity, email string) {
	if email == "" || identity.Email == email {
		return
	}
	_, _ = s.attachUserIdentityLocked(domainauth.UserIdentity{
		UserID:   identity.UserID,
		Provider: identity.Provider,
		Subject:  identity.Subject,
		Email:    email,
	})
}

func (s *Service) createUserWithIdentityLocked(user domainauth.User, identity domainauth.UserIdentity) (domainauth.User, error) {
	if s.store != nil {
		return s.store.CreateUserWithIdentity(context.Background(), user, identity)
	}
	user.ID = s.nextUserID
	if user.EmailVerifiedAt == nil && user.CreatedAt.IsZero() {
		now := time.Now()
		user.CreatedAt = now
	}
	s.nextUserID++
	s.usersByEmail[user.Email] = &user
	s.usersByID[user.ID] = &user
	identity.UserID = user.ID
	if _, err := s.attachUserIdentityLocked(identity); err != nil {
		return domainauth.User{}, err
	}
	return user, nil
}
