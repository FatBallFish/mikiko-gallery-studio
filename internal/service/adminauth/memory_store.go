package adminauth

import (
	"context"
	"strings"
	"sync"
	"time"

	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
)

type MemoryStore struct {
	mu      sync.Mutex
	nextID  int64
	byID    map[int64]domainadminauth.AdminUser
	byEmail map[string]int64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{nextID: 1, byID: map[int64]domainadminauth.AdminUser{}, byEmail: map[string]int64{}}
}

func (s *MemoryStore) GetAdminByEmail(_ context.Context, email string) (domainadminauth.AdminUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byEmail[normalizeEmail(email)]
	if !ok {
		return domainadminauth.AdminUser{}, repoerr.ErrNotFound
	}
	return s.byID[id], nil
}

func (s *MemoryStore) GetAdminByID(_ context.Context, id int64) (domainadminauth.AdminUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	admin, ok := s.byID[id]
	if !ok {
		return domainadminauth.AdminUser{}, repoerr.ErrNotFound
	}
	return admin, nil
}

func (s *MemoryStore) CreateAdmin(_ context.Context, admin domainadminauth.AdminUser) (domainadminauth.AdminUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	admin.Email = normalizeEmail(admin.Email)
	if admin.ID == 0 {
		admin.ID = s.nextID
		s.nextID++
	}
	if admin.Role == "" {
		admin.Role = "ops_admin"
	}
	if admin.Status == "" {
		admin.Status = "active"
	}
	now := time.Now().UTC()
	if admin.CreatedAt.IsZero() {
		admin.CreatedAt = now
	}
	admin.UpdatedAt = now
	s.byID[admin.ID] = admin
	s.byEmail[admin.Email] = admin.ID
	return admin, nil
}

func (s *MemoryStore) UpdateAdminPasswordHash(_ context.Context, id int64, oldHash string, newHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	admin, ok := s.byID[id]
	if !ok || admin.PasswordHash != oldHash {
		return repoerr.ErrNotFound
	}
	admin.PasswordHash = newHash
	admin.UpdatedAt = time.Now().UTC()
	s.byID[id] = admin
	return nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
