package apikey

import (
	"context"
	"sync"
	"time"

	domainapikey "github.com/fatballfish/pic-gallery/internal/domain/apikey"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
)

type MemoryStore struct {
	mu          sync.Mutex
	nextID      int64
	keysByID    map[int64]domainapikey.APIKey
	accessIndex map[string]int64
	secretIndex map[string]int64
	delegate    Store
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		nextID:      1,
		keysByID:    map[int64]domainapikey.APIKey{},
		accessIndex: map[string]int64{},
		secretIndex: map[string]int64{},
	}
}

func (s *MemoryStore) Create(ctx context.Context, key domainapikey.APIKey) (domainapikey.APIKey, error) {
	if s.delegate != nil {
		return s.delegate.Create(ctx, key)
	}
	s.ensure()
	s.mu.Lock()
	defer s.mu.Unlock()
	key.ID = s.nextID
	s.nextID++
	now := time.Now().UTC()
	key.CreatedAt = now
	key.UpdatedAt = now
	s.keysByID[key.ID] = key
	s.accessIndex[key.AccessKey] = key.ID
	s.secretIndex[key.SecretHash] = key.ID
	return key, nil
}

func (s *MemoryStore) GetByAccessKey(ctx context.Context, accessKey string) (domainapikey.APIKey, error) {
	if s.delegate != nil {
		return s.delegate.GetByAccessKey(ctx, accessKey)
	}
	s.ensure()
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.accessIndex[accessKey]
	if !ok {
		return domainapikey.APIKey{}, repoerr.ErrNotFound
	}
	return s.keysByID[id], nil
}

func (s *MemoryStore) GetBySecretHash(ctx context.Context, secretHash string) (domainapikey.APIKey, error) {
	if s.delegate != nil {
		return s.delegate.GetBySecretHash(ctx, secretHash)
	}
	s.ensure()
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.secretIndex[secretHash]
	if !ok {
		return domainapikey.APIKey{}, repoerr.ErrNotFound
	}
	return s.keysByID[id], nil
}

func (s *MemoryStore) UpdateLastUsedAt(ctx context.Context, id int64, at time.Time) error {
	if s.delegate != nil {
		return s.delegate.UpdateLastUsedAt(ctx, id, at)
	}
	return s.update(ctx, id, func(key *domainapikey.APIKey) { key.LastUsedAt = &at })
}

func (s *MemoryStore) UpdateStatus(ctx context.Context, id int64, status string) error {
	if s.delegate != nil {
		return s.delegate.UpdateStatus(ctx, id, status)
	}
	return s.update(ctx, id, func(key *domainapikey.APIKey) { key.Status = status })
}

func (s *MemoryStore) UpdateExpiresAt(ctx context.Context, id int64, expiresAt *time.Time) error {
	if s.delegate != nil {
		return s.delegate.UpdateExpiresAt(ctx, id, expiresAt)
	}
	return s.update(ctx, id, func(key *domainapikey.APIKey) { key.ExpiresAt = expiresAt })
}

func (s *MemoryStore) update(_ context.Context, id int64, fn func(*domainapikey.APIKey)) error {
	s.ensure()
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.keysByID[id]
	if !ok {
		return repoerr.ErrNotFound
	}
	fn(&key)
	key.UpdatedAt = time.Now().UTC()
	s.keysByID[id] = key
	return nil
}

func (s *MemoryStore) ensure() {
	if s.keysByID != nil {
		return
	}
	s.nextID = 1
	s.keysByID = map[int64]domainapikey.APIKey{}
	s.accessIndex = map[string]int64{}
	s.secretIndex = map[string]int64{}
}
