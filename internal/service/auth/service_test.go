package auth

import (
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

func TestLoginAndRefreshRotation(t *testing.T) {
	svc := NewService(config.AuthConfig{AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: 2 * time.Hour, Issuer: "test", AccessTokenSecret: "secret", RefreshCookieName: "pg_refresh"}, map[string]string{"basic": "1.00000"})
	if err := svc.SendEmailCode("user@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	user, session, err := svc.LoginWithEmailCode("user@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}
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
