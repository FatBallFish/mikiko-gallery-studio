package entstore

import (
	"context"
	"sort"
	"strings"
	"time"

	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/configitem"
)

type AdminConfigStore struct {
	client *repoent.Client
}

func NewAdminConfigStore(client *repoent.Client) *AdminConfigStore {
	return &AdminConfigStore{client: client}
}

func (s *AdminConfigStore) GetByCategory(ctx context.Context, category string) ([]domainadminconfig.Item, error) {
	entities, err := s.client.ConfigItem.Query().
		Where(configitem.ConfigCategoryEQ(category)).
		Order(repoent.Asc(configitem.FieldScope), repoent.Asc(configitem.FieldConfigKey)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]domainadminconfig.Item, 0, len(entities))
	for _, entity := range entities {
		items = append(items, domainadminconfig.Item{
			ConfigCategory: entity.ConfigCategory,
			ConfigKey:      entity.ConfigKey,
			ConfigValue:    cloneConfigValue(entity.ConfigValue),
			Scope:          entity.Scope,
			Version:        entity.Version,
		})
	}
	return items, nil
}

func (s *AdminConfigStore) SaveByCategory(ctx context.Context, category string, version int64, updatedBy int64, items []domainadminconfig.Item) error {
	for _, item := range items {
		scope := item.Scope
		if strings.TrimSpace(scope) == "" {
			scope = "global"
		}

		entity, err := s.client.ConfigItem.Query().
			Where(configitem.ConfigCategoryEQ(category), configitem.ConfigKeyEQ(item.ConfigKey), configitem.ScopeEQ(scope)).
			Only(ctx)
		if err != nil {
			if repoent.IsNotFound(err) {
				if _, err := s.client.ConfigItem.Create().
					SetConfigCategory(category).
					SetConfigKey(item.ConfigKey).
					SetConfigValue(cloneConfigValue(item.ConfigValue)).
					SetScope(scope).
					SetVersion(version).
					SetUpdatedBy(updatedBy).
					SetUpdatedAt(time.Now().UTC()).
					Save(ctx); err != nil {
					return err
				}
				continue
			}
			return err
		}

		if _, err := s.client.ConfigItem.UpdateOneID(entity.ID).
			SetConfigValue(cloneConfigValue(item.ConfigValue)).
			SetVersion(version).
			SetUpdatedBy(updatedBy).
			SetUpdatedAt(time.Now().UTC()).
			Save(ctx); err != nil {
			return err
		}
	}
	return nil
}

func cloneConfigValue(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	output := make(map[string]any, len(input))
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		output[key] = input[key]
	}
	return output
}
