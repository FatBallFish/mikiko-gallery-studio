package audit

import (
	"context"
	"sync"
	"time"

	domainaudit "github.com/fatballfish/pic-gallery/internal/domain/audit"
)

type MemoryStore struct {
	mu     sync.Mutex
	nextID int64
	logs   []domainaudit.Log
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{nextID: 1}
}

func (s *MemoryStore) Create(_ context.Context, log domainaudit.Log) (domainaudit.Log, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if log.ID == 0 {
		log.ID = s.nextID
		s.nextID++
	}
	now := time.Now().UTC()
	if log.CreatedAt.IsZero() {
		log.CreatedAt = now
	}
	log.UpdatedAt = now
	log.Metadata = cloneMetadata(log.Metadata)
	s.logs = append(s.logs, log)
	return log, nil
}

func (s *MemoryStore) List(_ context.Context) ([]domainaudit.Log, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	logs := make([]domainaudit.Log, 0, len(s.logs))
	for _, log := range s.logs {
		log.Metadata = cloneMetadata(log.Metadata)
		logs = append(logs, log)
	}
	return logs, nil
}
