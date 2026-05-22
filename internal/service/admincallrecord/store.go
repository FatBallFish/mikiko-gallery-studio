package admincallrecord

import (
	"context"
	"sort"
	"strings"
	"sync"

	domainadmincallrecord "github.com/fatballfish/pic-gallery/internal/domain/admincallrecord"
)

type Store interface {
	ListCallRecords(ctx context.Context, req domainadmincallrecord.ListRequest) (domainadmincallrecord.ListPage, error)
}

type MemoryStore struct {
	mu      sync.Mutex
	records []domainadmincallrecord.Record
}

func NewMemoryStore(records ...domainadmincallrecord.Record) *MemoryStore {
	return &MemoryStore{records: append([]domainadmincallrecord.Record(nil), records...)}
}

func (s *MemoryStore) ListCallRecords(_ context.Context, req domainadmincallrecord.ListRequest) (domainadmincallrecord.ListPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	page, pageSize := normalizePage(req.Page, req.PageSize)
	items := make([]domainadmincallrecord.Record, 0, len(s.records))
	for _, record := range s.records {
		if req.Status != "" && record.Status != req.Status {
			continue
		}
		if req.Provider != "" && record.Provider != req.Provider {
			continue
		}
		if req.SourceChannel != "" && record.SourceChannel != req.SourceChannel {
			continue
		}
		if req.UserID > 0 && record.UserID != req.UserID {
			continue
		}
		if req.TaskID != "" && !strings.EqualFold(record.TaskID, req.TaskID) {
			continue
		}
		if !req.CreatedFrom.IsZero() && record.CreatedAt.Before(req.CreatedFrom) {
			continue
		}
		if !req.CreatedTo.IsZero() && record.CreatedAt.After(req.CreatedTo) {
			continue
		}
		items = append(items, record)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].TaskID > items[j].TaskID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
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
	return domainadmincallrecord.ListPage{Items: items[start:end], Page: page, PageSize: pageSize, Total: total}, nil
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
