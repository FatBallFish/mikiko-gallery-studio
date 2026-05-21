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
	if key.SigningSecret != "" {
		create.SetSigningSecret(key.SigningSecret)
	}
	if key.TotalQuotaPoints != nil {
		create.SetTotalQuotaPoints(*key.TotalQuotaPoints)
	}
	if key.DailyQuotaPoints != nil {
		create.SetDailyQuotaPoints(*key.DailyQuotaPoints)
	}
	if key.RPMLimit != nil {
		create.SetRpmLimit(*key.RPMLimit)
	}
	if key.ExpiresAt != nil {
		create.SetExpiresAt(*key.ExpiresAt)
	}
	entity, err := create.Save(ctx)
	if err != nil {
		return domainapikey.APIKey{}, err
	}
	return mapAPIKeyEntity(entity), nil
}

func (s *APIKeyStore) ListByUser(ctx context.Context, userID int64) ([]domainapikey.APIKey, error) {
	entities, err := s.client.APIKey.Query().
		Where(apikey.UserIDEQ(userID), apikey.DeletedAtIsNil(), apikey.StatusNEQ(domainapikey.StatusDeleted)).
		Order(repoent.Desc(apikey.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]domainapikey.APIKey, 0, len(entities))
	for _, entity := range entities {
		keys = append(keys, mapAPIKeyEntity(entity))
	}
	return keys, nil
}

func (s *APIKeyStore) GetByID(ctx context.Context, userID int64, id int64) (domainapikey.APIKey, error) {
	entity, err := s.client.APIKey.Query().
		Where(apikey.IDEQ(int(id)), apikey.UserIDEQ(userID), apikey.DeletedAtIsNil(), apikey.StatusNEQ(domainapikey.StatusDeleted)).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainapikey.APIKey{}, repoerr.ErrNotFound
		}
		return domainapikey.APIKey{}, err
	}
	return mapAPIKeyEntity(entity), nil
}

func (s *APIKeyStore) GetByAccessKey(ctx context.Context, accessKey string) (domainapikey.APIKey, error) {
	entity, err := s.client.APIKey.Query().Where(apikey.AccessKeyEQ(accessKey), apikey.DeletedAtIsNil(), apikey.StatusNEQ(domainapikey.StatusDeleted)).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainapikey.APIKey{}, repoerr.ErrNotFound
		}
		return domainapikey.APIKey{}, err
	}
	return mapAPIKeyEntity(entity), nil
}

func (s *APIKeyStore) GetBySecretHash(ctx context.Context, secretHash string) (domainapikey.APIKey, error) {
	entity, err := s.client.APIKey.Query().Where(apikey.SecretHashEQ(secretHash), apikey.DeletedAtIsNil(), apikey.StatusNEQ(domainapikey.StatusDeleted)).Only(ctx)
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

func (s *APIKeyStore) Update(ctx context.Context, key domainapikey.APIKey) (domainapikey.APIKey, error) {
	update := s.client.APIKey.UpdateOneID(int(key.ID)).
		SetName(key.Name).
		SetStatus(key.Status).
		SetGroupCode(key.GroupCode).
		SetSecretHash(key.SecretHash)
	if key.SigningSecret == "" {
		update.ClearSigningSecret()
	} else {
		update.SetSigningSecret(key.SigningSecret)
	}
	if key.TotalQuotaPoints == nil {
		update.ClearTotalQuotaPoints()
	} else {
		update.SetTotalQuotaPoints(*key.TotalQuotaPoints)
	}
	if key.DailyQuotaPoints == nil {
		update.ClearDailyQuotaPoints()
	} else {
		update.SetDailyQuotaPoints(*key.DailyQuotaPoints)
	}
	if key.RPMLimit == nil {
		update.ClearRpmLimit()
	} else {
		update.SetRpmLimit(*key.RPMLimit)
	}
	if key.ExpiresAt == nil {
		update.ClearExpiresAt()
	} else {
		update.SetExpiresAt(*key.ExpiresAt)
	}
	if err := update.Exec(ctx); err != nil {
		return domainapikey.APIKey{}, err
	}
	return s.GetByID(ctx, key.UserID, key.ID)
}

func (s *APIKeyStore) SoftDelete(ctx context.Context, userID int64, id int64, at time.Time) error {
	affected, err := s.client.APIKey.Update().
		Where(apikey.IDEQ(int(id)), apikey.UserIDEQ(userID), apikey.DeletedAtIsNil()).
		SetStatus(domainapikey.StatusDeleted).
		SetDeletedAt(at).
		Save(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

func mapAPIKeyEntity(entity *repoent.APIKey) domainapikey.APIKey {
	key := domainapikey.APIKey{
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
	if entity.SigningSecret != nil {
		key.SigningSecret = *entity.SigningSecret
	}
	if entity.TotalQuotaPoints != nil {
		key.TotalQuotaPoints = entity.TotalQuotaPoints
	}
	if entity.DailyQuotaPoints != nil {
		key.DailyQuotaPoints = entity.DailyQuotaPoints
	}
	if entity.RpmLimit != nil {
		key.RPMLimit = entity.RpmLimit
	}
	return key
}
