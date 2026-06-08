package secureconfig

import (
	"context"
	"strings"
	"sync"
	"time"
)

type MemoryStore struct {
	mu      sync.Mutex
	records map[string]Record
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{records: map[string]Record{}}
}

func (s *MemoryStore) Get(_ context.Context, category, key string) (Record, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[recordKey(category, key)]
	if !ok {
		return Record{}, false, nil
	}
	return cloneRecord(record), true, nil
}

func (s *MemoryStore) Save(_ context.Context, record Record) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record.Category = strings.TrimSpace(record.Category)
	record.Key = strings.TrimSpace(record.Key)
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = time.Now().UTC()
	}
	if record.Version == 0 {
		record.Version = 1
	}
	s.records[recordKey(record.Category, record.Key)] = cloneRecord(record)
	return cloneRecord(record), nil
}

func recordKey(category, key string) string {
	return strings.TrimSpace(category) + "/" + strings.TrimSpace(key)
}

func cloneRecord(record Record) Record {
	record.PublicValue = cloneMap(record.PublicValue)
	record.SecretEncrypted = cloneMap(record.SecretEncrypted)
	record.SecretFields = append([]string{}, record.SecretFields...)
	return record
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
