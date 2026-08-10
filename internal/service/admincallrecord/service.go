package admincallrecord

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	domainadmincallrecord "github.com/fatballfish/pic-gallery/internal/domain/admincallrecord"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type Service struct {
	store Store
}

const maximumDistributionWindow = 31 * 24 * time.Hour

func (s *Service) CallDistribution(ctx context.Context, req domainadmincallrecord.DistributionRequest) (domainadmincallrecord.Distribution, error) {
	req.From = req.From.UTC()
	req.To = req.To.UTC()
	if req.From.IsZero() || req.To.IsZero() || !req.From.Before(req.To) {
		return domainadmincallrecord.Distribution{}, errs.BadRequest("from and to must define a valid time window")
	}
	if req.To.Sub(req.From) > maximumDistributionWindow {
		return domainadmincallrecord.Distribution{}, errs.BadRequest("time window must not exceed 31 days")
	}
	return s.store.CallDistribution(ctx, req)
}

func NewServiceWithStore(store Store) *Service {
	if store == nil {
		store = NewMemoryStore()
	}
	return &Service{store: store}
}

func (s *Service) ListCallRecords(ctx context.Context, req domainadmincallrecord.ListRequest) (domainadmincallrecord.ListPage, error) {
	req.Page, req.PageSize = normalizePage(req.Page, req.PageSize)
	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	req.Provider = strings.ToLower(strings.TrimSpace(req.Provider))
	req.SourceChannel = strings.ToLower(strings.TrimSpace(req.SourceChannel))
	req.TaskID = strings.TrimSpace(req.TaskID)
	if req.UserID < 0 {
		return domainadmincallrecord.ListPage{}, errs.BadRequest("invalid user_id")
	}
	if req.TaskID != "" {
		if _, err := uuid.Parse(req.TaskID); err != nil {
			return domainadmincallrecord.ListPage{}, errs.BadRequest("invalid task_id")
		}
	}
	if !req.CreatedFrom.IsZero() && !req.CreatedTo.IsZero() && req.CreatedFrom.After(req.CreatedTo) {
		return domainadmincallrecord.ListPage{}, errs.BadRequest("created_from must be before created_to")
	}
	return s.store.ListCallRecords(ctx, req)
}
