package adminuser

import (
	"context"
	"strings"
	"sync"
	"time"

	domainadminuser "github.com/fatballfish/pic-gallery/internal/domain/adminuser"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
)

type Store interface {
	ListUsers(ctx context.Context, req domainadminuser.ListRequest) (domainadminuser.ListPage, error)
	GetUserDetail(ctx context.Context, userID int64, recentLedgerLimit int) (domainadminuser.Detail, error)
	UpdateUserStatus(ctx context.Context, userID int64, status string) (domainadminuser.UserSummary, error)
}

type MemoryStore struct {
	mu     sync.Mutex
	users  map[int64]domainadminuser.UserSummary
	detail map[int64]domainadminuser.Detail
}

func NewMemoryStore(users ...domainadminuser.UserSummary) *MemoryStore {
	store := &MemoryStore{users: map[int64]domainadminuser.UserSummary{}, detail: map[int64]domainadminuser.Detail{}}
	for _, user := range users {
		store.users[user.ID] = user
		store.detail[user.ID] = domainadminuser.Detail{User: user}
	}
	return store
}

func (s *MemoryStore) ListUsers(_ context.Context, req domainadminuser.ListRequest) (domainadminuser.ListPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	page, pageSize := normalizePage(req.Page, req.PageSize)
	items := make([]domainadminuser.UserSummary, 0, len(s.users))
	query := strings.ToLower(strings.TrimSpace(req.Query))
	status := strings.TrimSpace(req.Status)
	for _, user := range s.users {
		if status != "" && user.Status != status {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(user.Email), query) && !strings.Contains(strings.ToLower(user.Nickname), query) {
			continue
		}
		items = append(items, user)
	}
	total := len(items)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return domainadminuser.ListPage{Items: items[start:end], Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *MemoryStore) GetUserDetail(_ context.Context, userID int64, _ int) (domainadminuser.Detail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	detail, ok := s.detail[userID]
	if !ok {
		return domainadminuser.Detail{}, repoerr.ErrNotFound
	}
	return detail, nil
}

func (s *MemoryStore) UpdateUserStatus(_ context.Context, userID int64, status string) (domainadminuser.UserSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[userID]
	if !ok {
		return domainadminuser.UserSummary{}, repoerr.ErrNotFound
	}
	if user.Status != status {
		user.Status = status
		user.TokenVersion++
		user.UpdatedAt = time.Now().UTC()
	}
	s.users[userID] = user
	detail := s.detail[userID]
	detail.User = user
	s.detail[userID] = detail
	return user, nil
}

func normalizePage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
