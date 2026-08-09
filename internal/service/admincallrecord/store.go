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
	CallDistribution(ctx context.Context, req domainadmincallrecord.DistributionRequest) (domainadmincallrecord.Distribution, error)
}

func (s *MemoryStore) CallDistribution(_ context.Context, req domainadmincallrecord.DistributionRequest) (domainadmincallrecord.Distribution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return AggregateCallDistribution(s.records, req), nil
}

func AggregateCallDistribution(records []domainadmincallrecord.Record, req domainadmincallrecord.DistributionRequest) domainadmincallrecord.Distribution {
	accumulator := NewDistributionAccumulator(req)
	for _, record := range records {
		accumulator.Add(record)
	}
	return accumulator.Result()
}

type DistributionAccumulator struct {
	result domainadmincallrecord.Distribution
	counts map[string]int
	req    domainadmincallrecord.DistributionRequest
}

func NewDistributionAccumulator(req domainadmincallrecord.DistributionRequest) *DistributionAccumulator {
	return &DistributionAccumulator{
		result: domainadmincallrecord.Distribution{
			Window: domainadmincallrecord.DistributionWindow{From: req.From, To: req.To},
			Groups: []domainadmincallrecord.DistributionGroup{},
		},
		counts: map[string]int{},
		req:    req,
	}
}

func (a *DistributionAccumulator) Add(record domainadmincallrecord.Record) {
	for _, attempt := range record.Attempts {
		at := record.CreatedAt
		if attempt.StartedAt != nil {
			at = *attempt.StartedAt
		}
		if at.Before(a.req.From) || !at.Before(a.req.To) {
			continue
		}
		key := strings.TrimSpace(record.RouteModelCode)
		if key == "" {
			key = "unrouted"
		}
		a.counts[key]++
		a.result.TotalCalls++
	}
	if len(record.Attempts) == 0 && record.UpstreamSucceededAt == nil && !record.CreatedAt.Before(a.req.From) && record.CreatedAt.Before(a.req.To) &&
		(record.Status == "failed" || record.Status == "rejected") {
		a.result.PreflightFailureCount++
	}
}

func (a *DistributionAccumulator) Result() domainadmincallrecord.Distribution {
	result := a.result
	result.Groups = make([]domainadmincallrecord.DistributionGroup, 0, len(a.counts))
	for key, calls := range a.counts {
		percentage := float64(0)
		if result.TotalCalls > 0 {
			percentage = float64(calls) * 100 / float64(result.TotalCalls)
		}
		result.Groups = append(result.Groups, domainadmincallrecord.DistributionGroup{
			Key: key, Calls: calls, Percentage: percentage,
		})
	}
	sort.Slice(result.Groups, func(i, j int) bool {
		if result.Groups[i].Calls == result.Groups[j].Calls {
			return result.Groups[i].Key < result.Groups[j].Key
		}
		return result.Groups[i].Calls > result.Groups[j].Calls
	})
	return result
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
