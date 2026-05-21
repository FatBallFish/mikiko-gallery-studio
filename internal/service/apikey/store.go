package apikey

import (
	"context"
	"time"

	domainapikey "github.com/fatballfish/pic-gallery/internal/domain/apikey"
)

type Store interface {
	Create(ctx context.Context, key domainapikey.APIKey) (domainapikey.APIKey, error)
	ListByUser(ctx context.Context, userID int64) ([]domainapikey.APIKey, error)
	GetByID(ctx context.Context, userID, id int64) (domainapikey.APIKey, error)
	GetByAccessKey(ctx context.Context, accessKey string) (domainapikey.APIKey, error)
	GetBySecretHash(ctx context.Context, secretHash string) (domainapikey.APIKey, error)
	UpdateForUser(ctx context.Context, userID int64, key domainapikey.APIKey) (domainapikey.APIKey, error)
	UpdateLastUsedAt(ctx context.Context, id int64, at time.Time) error
	UpdateStatus(ctx context.Context, id int64, status string) error
	UpdateStatusForUser(ctx context.Context, userID, id int64, status string) (domainapikey.APIKey, error)
	UpdateExpiresAt(ctx context.Context, id int64, expiresAt *time.Time) error
	UpdateSecretForUser(ctx context.Context, userID, id int64, secretHash string, secretCiphertext string) (domainapikey.APIKey, error)
	DeleteForUser(ctx context.Context, userID, id int64, at time.Time) error
	RecordRequest(ctx context.Context, id int64, rpmLimit int, at time.Time) error
	ReserveQuota(ctx context.Context, userID, id int64, reservationID, points string, at time.Time) error
	ReleaseQuota(ctx context.Context, id int64, reservationID string) error
}
