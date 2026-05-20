package apikey

import (
	"context"
	"time"

	domainapikey "github.com/fatballfish/pic-gallery/internal/domain/apikey"
)

type Store interface {
	Create(ctx context.Context, key domainapikey.APIKey) (domainapikey.APIKey, error)
	GetByAccessKey(ctx context.Context, accessKey string) (domainapikey.APIKey, error)
	GetBySecretHash(ctx context.Context, secretHash string) (domainapikey.APIKey, error)
	UpdateLastUsedAt(ctx context.Context, id int64, at time.Time) error
	UpdateStatus(ctx context.Context, id int64, status string) error
	UpdateExpiresAt(ctx context.Context, id int64, expiresAt *time.Time) error
}
