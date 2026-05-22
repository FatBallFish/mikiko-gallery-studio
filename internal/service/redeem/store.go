package redeem

import (
	"context"
	"strings"
	"sync"
	"time"

	domainredeem "github.com/fatballfish/pic-gallery/internal/domain/redeem"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
)

type Store interface {
	ListCodes(ctx context.Context, req domainredeem.ListRequest) (domainredeem.ListPage, error)
	CreateCode(ctx context.Context, req domainredeem.CreateRequest) (domainredeem.Code, error)
	CodeExists(ctx context.Context, code string) (bool, error)
	UpdateStatus(ctx context.Context, id int64, status string) (domainredeem.Code, error)
	ListRedemptions(ctx context.Context, codeID int64, page, pageSize int) (domainredeem.RedemptionsPage, error)
}

type MemoryStore struct {
	mu     sync.Mutex
	nextID int64
	codes  map[int64]domainredeem.Code
}

func NewMemoryStore(items ...domainredeem.Code) *MemoryStore {
	store := &MemoryStore{nextID: 1, codes: map[int64]domainredeem.Code{}}
	for _, item := range items {
		if item.ID == 0 {
			item.ID = store.nextID
			store.nextID++
		}
		if item.ID >= store.nextID {
			store.nextID = item.ID + 1
		}
		store.codes[item.ID] = item
	}
	return store
}

func (s *MemoryStore) ListCodes(_ context.Context, req domainredeem.ListRequest) (domainredeem.ListPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	page, pageSize := normalizePage(req.Page, req.PageSize)
	items := make([]domainredeem.Code, 0, len(s.codes))
	codeFilter := strings.ToUpper(strings.TrimSpace(req.Code))
	status := strings.TrimSpace(req.Status)
	for _, item := range s.codes {
		if status != "" && item.Status != status {
			continue
		}
		if codeFilter != "" && !strings.Contains(item.Code, codeFilter) {
			continue
		}
		if req.BatchID > 0 && item.BatchID != req.BatchID {
			continue
		}
		items = append(items, item)
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
	return domainredeem.ListPage{Items: items[start:end], Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *MemoryStore) CreateCode(_ context.Context, req domainredeem.CreateRequest) (domainredeem.Code, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	item := domainredeem.Code{
		ID:             s.nextID,
		BatchID:        req.BatchID,
		Code:           strings.ToUpper(strings.TrimSpace(req.Code)),
		Status:         req.Status,
		RewardType:     req.RewardType,
		RewardValue:    req.RewardValue,
		ValidFrom:      req.ValidFrom,
		ValidUntil:     req.ValidUntil,
		MaxRedemptions: req.MaxRedemptions,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.nextID++
	s.codes[item.ID] = item
	return item, nil
}

func (s *MemoryStore) CodeExists(_ context.Context, code string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	code = strings.ToUpper(strings.TrimSpace(code))
	for _, item := range s.codes {
		if item.Code == code {
			return true, nil
		}
	}
	return false, nil
}

func (s *MemoryStore) UpdateStatus(_ context.Context, id int64, status string) (domainredeem.Code, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.codes[id]
	if !ok {
		return domainredeem.Code{}, repoerr.ErrNotFound
	}
	item.Status = status
	item.UpdatedAt = time.Now().UTC()
	s.codes[id] = item
	return item, nil
}

func (s *MemoryStore) ListRedemptions(_ context.Context, codeID int64, page, pageSize int) (domainredeem.RedemptionsPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.codes[codeID]; !ok {
		return domainredeem.RedemptionsPage{}, repoerr.ErrNotFound
	}
	page, pageSize = normalizePage(page, pageSize)
	return domainredeem.RedemptionsPage{Items: nil, Page: page, PageSize: pageSize, Total: 0}, nil
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
