package audit

import (
	"context"
	"sort"
	"strings"
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

func (s *MemoryStore) List(_ context.Context, req domainaudit.ListRequest) (domainaudit.ListPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	page, pageSize := normalizePage(req.Page, req.PageSize)
	logs := make([]domainaudit.Log, 0, len(s.logs))
	for _, log := range s.logs {
		if !matchAuditFilters(log, req) {
			continue
		}
		log.Metadata = cloneMetadata(log.Metadata)
		logs = append(logs, log)
	}
	sort.SliceStable(logs, func(i, j int) bool {
		if logs[i].CreatedAt.Equal(logs[j].CreatedAt) {
			return logs[i].ID > logs[j].ID
		}
		return logs[i].CreatedAt.After(logs[j].CreatedAt)
	})
	total := len(logs)
	start := (page - 1) * pageSize
	if start >= len(logs) {
		return domainaudit.ListPage{Items: []domainaudit.Log{}, Page: page, PageSize: pageSize, Total: total}, nil
	}
	end := start + pageSize
	if end > len(logs) {
		end = len(logs)
	}
	return domainaudit.ListPage{Items: logs[start:end], Page: page, PageSize: pageSize, Total: total}, nil
}

func matchAuditFilters(log domainaudit.Log, req domainaudit.ListRequest) bool {
	if req.ActorType != "" && !strings.EqualFold(log.ActorType, req.ActorType) {
		return false
	}
	if req.ActorID != "" && log.ActorID != req.ActorID {
		return false
	}
	if req.Action != "" && !strings.EqualFold(log.Action, req.Action) {
		return false
	}
	if req.TargetType != "" && !strings.EqualFold(log.TargetType, req.TargetType) {
		return false
	}
	if req.TargetID != "" && log.TargetID != req.TargetID {
		return false
	}
	if req.Result != "" && !strings.EqualFold(log.Result, req.Result) {
		return false
	}
	if !req.CreatedFrom.IsZero() && log.CreatedAt.Before(req.CreatedFrom) {
		return false
	}
	if !req.CreatedTo.IsZero() && log.CreatedAt.After(req.CreatedTo) {
		return false
	}
	return true
}
