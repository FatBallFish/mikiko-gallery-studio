package auth

import (
	"context"

	domainauth "github.com/fatballfish/pic-gallery/internal/domain/auth"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
)

type Store interface {
	GetUserByEmail(ctx context.Context, email string) (domainauth.User, error)
	CreateUser(ctx context.Context, user domainauth.User) (domainauth.User, error)
	GetUserByID(ctx context.Context, id int64) (domainauth.User, error)
	UpdateUser(ctx context.Context, user domainauth.User) (domainauth.User, error)
	IncrementTokenVersion(ctx context.Context, userID int64) error
	SaveRefreshSession(ctx context.Context, session entstore.RefreshSessionRecord) error
	GetRefreshSessionByHash(ctx context.Context, tokenHash string) (entstore.RefreshSessionRecord, error)
	MarkRefreshSessionRotated(ctx context.Context, sessionID string, replacedBySessionID string) error
	MarkRefreshSessionExpired(ctx context.Context, sessionID string) error
	MarkRefreshSessionRevoked(ctx context.Context, sessionID string) error
	MarkFamilyReplayBlocked(ctx context.Context, familyID string) error
}
