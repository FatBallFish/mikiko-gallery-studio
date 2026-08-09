package entstore

import (
	"context"
	"errors"
	"time"

	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/configitem"
)

const (
	aliasRolloutCategory = "runtime_rollout"
	aliasRolloutKey      = "no_copy_reference_aliases"
)

type AliasRolloutStore struct {
	client *repoent.Client
}

func NewAliasRolloutStore(client *repoent.Client) *AliasRolloutStore {
	return &AliasRolloutStore{client: client}
}

func (s *AliasRolloutStore) AliasCreationEnabled(ctx context.Context) (bool, error) {
	status, err := s.GetAliasCreationRollout(ctx)
	return status.Enabled, err
}

func (s *AliasRolloutStore) GetAliasCreationRollout(ctx context.Context) (domainassets.AliasCreationRollout, error) {
	row, err := s.client.ConfigItem.Query().Where(
		configitem.ConfigCategoryEQ(aliasRolloutCategory),
		configitem.ConfigKeyEQ(aliasRolloutKey),
		configitem.ScopeEQ("global"),
	).Only(ctx)
	if repoent.IsNotFound(err) {
		return domainassets.AliasCreationRollout{}, nil
	}
	if err != nil {
		return domainassets.AliasCreationRollout{}, err
	}
	enabled, _ := row.ConfigValue["enabled"].(bool)
	return domainassets.AliasCreationRollout{
		Enabled: enabled, Version: row.Version, UpdatedBy: row.UpdatedBy, UpdatedAt: row.UpdatedAt,
	}, nil
}

func (s *AliasRolloutStore) UpdateAliasCreationRollout(ctx context.Context, enabled bool, expectedVersion, updatedBy int64) (domainassets.AliasCreationRollout, error) {
	now := time.Now().UTC()
	if expectedVersion == 0 {
		_, err := s.client.ConfigItem.Create().
			SetConfigCategory(aliasRolloutCategory).
			SetConfigKey(aliasRolloutKey).
			SetScope("global").
			SetConfigValue(map[string]any{"enabled": enabled}).
			SetVersion(1).
			SetUpdatedBy(updatedBy).
			SetUpdatedAt(now).
			Save(ctx)
		if repoent.IsConstraintError(err) {
			return domainassets.AliasCreationRollout{}, domainassets.ErrAliasRolloutChanged
		}
		if err != nil {
			return domainassets.AliasCreationRollout{}, err
		}
		return domainassets.AliasCreationRollout{Enabled: enabled, Version: 1, UpdatedBy: updatedBy, UpdatedAt: now}, nil
	}
	updated, err := s.client.ConfigItem.Update().Where(
		configitem.ConfigCategoryEQ(aliasRolloutCategory),
		configitem.ConfigKeyEQ(aliasRolloutKey),
		configitem.ScopeEQ("global"),
		configitem.VersionEQ(expectedVersion),
	).
		SetConfigValue(map[string]any{"enabled": enabled}).
		SetVersion(expectedVersion + 1).
		SetUpdatedBy(updatedBy).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return domainassets.AliasCreationRollout{}, err
	}
	if updated != 1 {
		return domainassets.AliasCreationRollout{}, domainassets.ErrAliasRolloutChanged
	}
	status, err := s.GetAliasCreationRollout(ctx)
	if err != nil {
		return domainassets.AliasCreationRollout{}, err
	}
	if status.Version != expectedVersion+1 || status.Enabled != enabled {
		return domainassets.AliasCreationRollout{}, errors.New("alias creation rollout update was not persisted")
	}
	return status, nil
}
