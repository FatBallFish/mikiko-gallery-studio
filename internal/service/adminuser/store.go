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
	UpdateUserLimits(ctx context.Context, req domainadminuser.LimitsRequest) (domainadminuser.UserSummary, error)
	AssignUserGroup(ctx context.Context, req domainadminuser.GroupAssignmentRequest) (domainadminuser.UserSummary, error)
	ListUserGroups(ctx context.Context, req domainadminuser.UserGroupListRequest) (domainadminuser.UserGroupListPage, error)
	GetUserGroup(ctx context.Context, groupCode string) (domainadminuser.UserGroup, error)
	CreateUserGroup(ctx context.Context, req domainadminuser.UserGroupWriteRequest) (domainadminuser.UserGroup, error)
	UpdateUserGroup(ctx context.Context, groupCode string, req domainadminuser.UserGroupWriteRequest) (domainadminuser.UserGroup, error)
	DeleteUserGroup(ctx context.Context, groupCode string) error
}

type MemoryStore struct {
	mu         sync.Mutex
	users      map[int64]domainadminuser.UserSummary
	detail     map[int64]domainadminuser.Detail
	userGroups map[string]domainadminuser.UserGroup
}

func NewMemoryStore(users ...domainadminuser.UserSummary) *MemoryStore {
	store := &MemoryStore{
		users:      map[int64]domainadminuser.UserSummary{},
		detail:     map[int64]domainadminuser.Detail{},
		userGroups: map[string]domainadminuser.UserGroup{},
	}
	for _, user := range users {
		store.users[user.ID] = user
		store.detail[user.ID] = domainadminuser.Detail{User: user}
	}
	store.userGroups["basic"] = domainadminuser.UserGroup{
		GroupCode:  "basic",
		GroupName:  "basic",
		Multiplier: "1.00000",
		Status:     "active",
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

func (s *MemoryStore) UpdateUserLimits(_ context.Context, req domainadminuser.LimitsRequest) (domainadminuser.UserSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[req.UserID]
	if !ok {
		return domainadminuser.UserSummary{}, repoerr.ErrNotFound
	}
	user.RPMLimit = req.RPMLimit
	user.ConcurrencyLimit = req.ConcurrencyLimit
	user.UpdatedAt = time.Now().UTC()
	s.users[req.UserID] = user
	detail := s.detail[req.UserID]
	detail.User = user
	s.detail[req.UserID] = detail
	return user, nil
}

func (s *MemoryStore) AssignUserGroup(_ context.Context, req domainadminuser.GroupAssignmentRequest) (domainadminuser.UserSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[req.UserID]
	if !ok {
		return domainadminuser.UserSummary{}, repoerr.ErrNotFound
	}
	if _, ok := s.userGroups[req.UserGroupCode]; !ok {
		return domainadminuser.UserSummary{}, repoerr.ErrNotFound
	}
	user.UserGroupCode = req.UserGroupCode
	user.UpdatedAt = time.Now().UTC()
	s.users[req.UserID] = user
	detail := s.detail[req.UserID]
	detail.User = user
	s.detail[req.UserID] = detail
	return user, nil
}

func (s *MemoryStore) ListUserGroups(_ context.Context, req domainadminuser.UserGroupListRequest) (domainadminuser.UserGroupListPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	page, pageSize := normalizePage(req.Page, req.PageSize)
	query := strings.ToLower(strings.TrimSpace(req.Query))
	status := strings.TrimSpace(req.Status)
	items := make([]domainadminuser.UserGroup, 0, len(s.userGroups))
	for _, group := range s.userGroups {
		if status != "" && group.Status != status {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(group.GroupCode), query) && !strings.Contains(strings.ToLower(group.GroupName), query) {
			continue
		}
		items = append(items, group)
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
	return domainadminuser.UserGroupListPage{Items: items[start:end], Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *MemoryStore) GetUserGroup(_ context.Context, groupCode string) (domainadminuser.UserGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	group, ok := s.userGroups[groupCode]
	if !ok {
		return domainadminuser.UserGroup{}, repoerr.ErrNotFound
	}
	return group, nil
}

func (s *MemoryStore) CreateUserGroup(_ context.Context, req domainadminuser.UserGroupWriteRequest) (domainadminuser.UserGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.userGroups[req.GroupCode]; ok {
		return domainadminuser.UserGroup{}, repoerr.ErrConflict
	}
	now := time.Now().UTC()
	group := domainadminuser.UserGroup{
		GroupCode:   req.GroupCode,
		GroupName:   req.GroupName,
		Multiplier:  req.Multiplier,
		Status:      req.Status,
		Description: req.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.userGroups[group.GroupCode] = group
	return group, nil
}

func (s *MemoryStore) UpdateUserGroup(_ context.Context, groupCode string, req domainadminuser.UserGroupWriteRequest) (domainadminuser.UserGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	group, ok := s.userGroups[groupCode]
	if !ok {
		return domainadminuser.UserGroup{}, repoerr.ErrNotFound
	}
	group.GroupName = req.GroupName
	group.Multiplier = req.Multiplier
	group.Status = req.Status
	group.Description = req.Description
	group.UpdatedAt = time.Now().UTC()
	s.userGroups[groupCode] = group
	return group, nil
}

func (s *MemoryStore) DeleteUserGroup(_ context.Context, groupCode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.userGroups[groupCode]; !ok {
		return repoerr.ErrNotFound
	}
	delete(s.userGroups, groupCode)
	for id, user := range s.users {
		if user.UserGroupCode == groupCode {
			user.UserGroupCode = "basic"
			s.users[id] = user
			detail := s.detail[id]
			detail.User = user
			s.detail[id] = detail
		}
	}
	return nil
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
