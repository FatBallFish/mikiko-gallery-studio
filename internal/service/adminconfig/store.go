package adminconfig

import (
	"context"
	"sync"

	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
)

type Store interface {
	GetByCategory(ctx context.Context, category string) ([]domainadminconfig.Item, error)
	SaveByCategory(ctx context.Context, category string, version int64, updatedBy int64, items []domainadminconfig.Item) error
}

type MemoryStore struct {
	mu         sync.Mutex
	byCategory map[string]map[string]domainadminconfig.Item
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{byCategory: map[string]map[string]domainadminconfig.Item{}}
}

func (s *MemoryStore) GetByCategory(_ context.Context, category string) ([]domainadminconfig.Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	records := s.byCategory[category]
	items := make([]domainadminconfig.Item, 0, len(records))
	for _, item := range records {
		items = append(items, cloneItem(item))
	}
	return items, nil
}

func (s *MemoryStore) SaveByCategory(_ context.Context, category string, version int64, _ int64, items []domainadminconfig.Item) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.byCategory[category]; !ok {
		s.byCategory[category] = map[string]domainadminconfig.Item{}
	}
	for _, item := range items {
		cloned := cloneItem(item)
		cloned.ConfigCategory = category
		cloned.Scope = defaultString(cloned.Scope, "global")
		cloned.Version = version
		s.byCategory[category][itemKey(cloned.ConfigKey, cloned.Scope)] = cloned
	}
	return nil
}

func itemKey(configKey, scope string) string {
	return configKey + "::" + scope
}

func cloneItem(item domainadminconfig.Item) domainadminconfig.Item {
	cloned := item
	if item.ConfigValue != nil {
		cloned.ConfigValue = make(map[string]any, len(item.ConfigValue))
		for key, value := range item.ConfigValue {
			cloned.ConfigValue[key] = value
		}
	}
	return cloned
}
