package admincallrecord

import (
	"context"
	"strings"

	"github.com/google/uuid"

	domainadmincallrecord "github.com/fatballfish/pic-gallery/internal/domain/admincallrecord"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type Service struct {
	store Store
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
