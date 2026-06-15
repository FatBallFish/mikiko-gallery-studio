package adminauth

import (
	"context"
	"sort"
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

func (s *MemoryStore) ListAdmins(_ context.Context, req domainadminauth.AdminListRequest) (domainadminauth.AdminListPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	page, pageSize := normalizeAdminPage(req.Page, req.PageSize)
	query := normalizeEmail(req.Query)
	role := strings.TrimSpace(req.Role)
	status := strings.TrimSpace(req.Status)
	items := make([]domainadminauth.AdminUser, 0, len(s.byID))
	for _, admin := range s.byID {
		if query != "" && !strings.Contains(admin.Email, query) {
			continue
		}
		if role != "" && admin.Role != role {
			continue
		}
		if status != "" && admin.Status != status {
			continue
		}
		items = append(items, admin)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	total := len(items)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return domainadminauth.AdminListPage{Items: items[start:end], Page: page, PageSize: pageSize, Total: total}, nil
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
	if _, ok := s.byEmail[admin.Email]; ok {
		return domainadminauth.AdminUser{}, repoerr.ErrConflict
	}
	if admin.ID == 0 {
		admin.ID = s.nextID
		s.nextID++
	}
	if admin.Role == "" {
		admin.Role = domainadminauth.RoleAdmin
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

func (s *MemoryStore) UpdateAdmin(_ context.Context, id int64, role string, status string, setRole bool, setStatus bool) (domainadminauth.AdminUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	admin, ok := s.byID[id]
	if !ok {
		return domainadminauth.AdminUser{}, repoerr.ErrNotFound
	}
	if setRole {
		admin.Role = role
	}
	if setStatus {
		admin.Status = status
	}
	admin.UpdatedAt = time.Now().UTC()
	s.byID[id] = admin
	return admin, nil
}

func (s *MemoryStore) UpdateAdminPassword(_ context.Context, id int64, passwordHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	admin, ok := s.byID[id]
	if !ok {
		return repoerr.ErrNotFound
	}
	admin.PasswordHash = passwordHash
	admin.UpdatedAt = time.Now().UTC()
	s.byID[id] = admin
	return nil
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

func (s *MemoryStore) DeleteAdmin(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	admin, ok := s.byID[id]
	if !ok {
		return repoerr.ErrNotFound
	}
	delete(s.byID, id)
	delete(s.byEmail, admin.Email)
	return nil
}

func (s *MemoryStore) CountActiveSuperAdmins(_ context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, admin := range s.byID {
		if admin.Role == domainadminauth.RoleSuperAdmin && admin.Status == "active" {
			count++
		}
	}
	return count, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
