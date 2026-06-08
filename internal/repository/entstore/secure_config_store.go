package entstore

import (
	"context"
	"strings"
	"time"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/secureconfig"
	secureconfigservice "github.com/fatballfish/pic-gallery/internal/service/secureconfig"
)

type SecureConfigStore struct {
	client *repoent.Client
}

func NewSecureConfigStore(client *repoent.Client) *SecureConfigStore {
	return &SecureConfigStore{client: client}
}

func (s *SecureConfigStore) Get(ctx context.Context, category, key string) (secureconfigservice.Record, bool, error) {
	row, err := s.client.SecureConfig.Query().
		Where(
			secureconfig.ConfigCategoryEQ(strings.TrimSpace(category)),
			secureconfig.ConfigKeyEQ(strings.TrimSpace(key)),
		).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return secureconfigservice.Record{}, false, nil
		}
		return secureconfigservice.Record{}, false, err
	}
	return secureConfigRecordFromEnt(row), true, nil
}

func (s *SecureConfigStore) Save(ctx context.Context, record secureconfigservice.Record) (secureconfigservice.Record, error) {
	category := strings.TrimSpace(record.Category)
	key := strings.TrimSpace(record.Key)
	now := time.Now().UTC()
	row, err := s.client.SecureConfig.Query().
		Where(secureconfig.ConfigCategoryEQ(category), secureconfig.ConfigKeyEQ(key)).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			created, createErr := s.client.SecureConfig.Create().
				SetConfigCategory(category).
				SetConfigKey(key).
				SetPublicValue(cloneConfigValue(record.PublicValue)).
				SetSecretEncrypted(cloneConfigValue(record.SecretEncrypted)).
				SetSecretFingerprint(record.SecretFingerprint).
				SetSecretFields(append([]string{}, record.SecretFields...)).
				SetVersion(defaultInt64(record.Version, 1)).
				SetUpdatedBy(record.UpdatedBy).
				SetUpdatedAt(now).
				Save(ctx)
			if createErr != nil {
				return secureconfigservice.Record{}, createErr
			}
			return secureConfigRecordFromEnt(created), nil
		}
		return secureconfigservice.Record{}, err
	}
	updated, err := s.client.SecureConfig.UpdateOneID(row.ID).
		SetPublicValue(cloneConfigValue(record.PublicValue)).
		SetSecretEncrypted(cloneConfigValue(record.SecretEncrypted)).
		SetSecretFingerprint(record.SecretFingerprint).
		SetSecretFields(append([]string{}, record.SecretFields...)).
		SetVersion(defaultInt64(record.Version, row.Version+1)).
		SetUpdatedBy(record.UpdatedBy).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return secureconfigservice.Record{}, err
	}
	return secureConfigRecordFromEnt(updated), nil
}

func secureConfigRecordFromEnt(row *repoent.SecureConfig) secureconfigservice.Record {
	if row == nil {
		return secureconfigservice.Record{}
	}
	return secureconfigservice.Record{
		Category:          row.ConfigCategory,
		Key:               row.ConfigKey,
		PublicValue:       cloneConfigValue(row.PublicValue),
		SecretEncrypted:   cloneConfigValue(row.SecretEncrypted),
		SecretFingerprint: row.SecretFingerprint,
		SecretFields:      append([]string{}, row.SecretFields...),
		Version:           row.Version,
		UpdatedBy:         row.UpdatedBy,
		UpdatedAt:         row.UpdatedAt,
	}
}

func defaultInt64(value, fallback int64) int64 {
	if value == 0 {
		return fallback
	}
	return value
}
