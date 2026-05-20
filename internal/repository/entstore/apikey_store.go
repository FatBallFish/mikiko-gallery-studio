package entstore

import (
	"context"
	"time"

	domainapikey "github.com/fatballfish/pic-gallery/internal/domain/apikey"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/apikey"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
)

type APIKeyStore struct {
	client *repoent.Client
}

func NewAPIKeyStore(client *repoent.Client) *APIKeyStore {
	return &APIKeyStore{client: client}
}

func (s *APIKeyStore) Create(ctx context.Context, key domainapikey.APIKey) (domainapikey.APIKey, error) {
	create := s.client.APIKey.Create().
		SetUserID(key.UserID).
		SetAccessKey(key.AccessKey).
		SetSecretHash(key.SecretHash).
		SetName(key.Name).
		SetStatus(key.Status).
		SetGroupCode(key.GroupCode)
	if key.ExpiresAt != nil {
		create.SetExpiresAt(*key.ExpiresAt)
	}
	entity, err := create.Save(ctx)
	if err != nil {
		return domainapikey.APIKey{}, err
	}
	return mapAPIKeyEntity(entity), nil
}

func (s *APIKeyStore) GetByAccessKey(ctx context.Context, accessKey string) (domainapikey.APIKey, error) {
	entity, err := s.client.APIKey.Query().Where(apikey.AccessKeyEQ(accessKey), apikey.DeletedAtIsNil()).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainapikey.APIKey{}, repoerr.ErrNotFound
		}
		return domainapikey.APIKey{}, err
	}
	return mapAPIKeyEntity(entity), nil
}

func (s *APIKeyStore) GetBySecretHash(ctx context.Context, secretHash string) (domainapikey.APIKey, error) {
	entity, err := s.client.APIKey.Query().Where(apikey.SecretHashEQ(secretHash), apikey.DeletedAtIsNil()).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainapikey.APIKey{}, repoerr.ErrNotFound
		}
		return domainapikey.APIKey{}, err
	}
	return mapAPIKeyEntity(entity), nil
}

func (s *APIKeyStore) UpdateLastUsedAt(ctx context.Context, id int64, at time.Time) error {
	return s.client.APIKey.UpdateOneID(int(id)).SetLastUsedAt(at).Exec(ctx)
}

func (s *APIKeyStore) UpdateStatus(ctx context.Context, id int64, status string) error {
	return s.client.APIKey.UpdateOneID(int(id)).SetStatus(status).Exec(ctx)
}

func (s *APIKeyStore) UpdateExpiresAt(ctx context.Context, id int64, expiresAt *time.Time) error {
	update := s.client.APIKey.UpdateOneID(int(id))
	if expiresAt == nil {
		update.ClearExpiresAt()
	} else {
		update.SetExpiresAt(*expiresAt)
	}
	return update.Exec(ctx)
}

func mapAPIKeyEntity(entity *repoent.APIKey) domainapikey.APIKey {
	return domainapikey.APIKey{
		ID:         int64(entity.ID),
		UserID:     entity.UserID,
		AccessKey:  entity.AccessKey,
		SecretHash: entity.SecretHash,
		Name:       entity.Name,
		Status:     entity.Status,
		GroupCode:  entity.GroupCode,
		ExpiresAt:  entity.ExpiresAt,
		LastUsedAt: entity.LastUsedAt,
		CreatedAt:  entity.CreatedAt,
		UpdatedAt:  entity.UpdatedAt,
	}
}
