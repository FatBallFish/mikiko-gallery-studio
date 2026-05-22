package adminuser

import (
	"context"
	"regexp"
	"strings"

	domainadminuser "github.com/fatballfish/pic-gallery/internal/domain/adminuser"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

var pointValuePattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]{1,5})?$`)

type Billing interface {
	AdminAdjust(ctx context.Context, req domainbilling.AdjustRequest) (domainbilling.BalanceSummary, error)
}

type Service struct {
	store   Store
	billing Billing
}

func NewServiceWithStore(store Store, billing Billing) *Service {
	if store == nil {
		store = NewMemoryStore()
	}
	return &Service{store: store, billing: billing}
}

func (s *Service) ListUsers(ctx context.Context, req domainadminuser.ListRequest) (domainadminuser.ListPage, error) {
	req.Page, req.PageSize = normalizePage(req.Page, req.PageSize)
	req.Query = strings.TrimSpace(req.Query)
	rawStatus := strings.TrimSpace(req.Status)
	req.Status = normalizeStatus(rawStatus)
	if rawStatus != "" && req.Status == "" {
		return domainadminuser.ListPage{}, errs.BadRequest("invalid status")
	}
	return s.store.ListUsers(ctx, req)
}

func (s *Service) GetUserDetail(ctx context.Context, userID int64) (domainadminuser.Detail, error) {
	if userID <= 0 {
		return domainadminuser.Detail{}, errs.BadRequest("invalid user_id")
	}
	return s.store.GetUserDetail(ctx, userID, 10)
}

func (s *Service) UpdateUserStatus(ctx context.Context, req domainadminuser.StatusRequest) (domainadminuser.UserSummary, error) {
	if req.UserID <= 0 {
		return domainadminuser.UserSummary{}, errs.BadRequest("invalid user_id")
	}
	status := normalizeStatus(req.Status)
	if status == "" {
		return domainadminuser.UserSummary{}, errs.BadRequest("status must be active or disabled")
	}
	return s.store.UpdateUserStatus(ctx, req.UserID, status)
}

func (s *Service) AdjustPoints(ctx context.Context, req domainadminuser.PointAdjustmentRequest) (domainbilling.BalanceSummary, error) {
	if req.UserID <= 0 {
		return domainbilling.BalanceSummary{}, errs.BadRequest("invalid user_id")
	}
	if _, err := s.store.GetUserDetail(ctx, req.UserID, 1); err != nil {
		return domainbilling.BalanceSummary{}, err
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return domainbilling.BalanceSummary{}, errs.BadRequest("Idempotency-Key is required")
	}
	changePoints := strings.TrimSpace(req.ChangePoints)
	reason := strings.TrimSpace(req.Reason)
	if changePoints == "" || reason == "" {
		return domainbilling.BalanceSummary{}, errs.BadRequest("change_points and reason are required")
	}
	if !pointValuePattern.MatchString(changePoints) {
		return domainbilling.BalanceSummary{}, errs.BadRequest("change_points must be a decimal with up to 5 fractional digits")
	}
	if len(reason) > 255 {
		return domainbilling.BalanceSummary{}, errs.BadRequest("reason must be at most 255 characters")
	}
	if s.billing == nil {
		return domainbilling.BalanceSummary{}, errs.Internal("billing service is not configured")
	}
	return s.billing.AdminAdjust(ctx, domainbilling.AdjustRequest{
		UserID:          req.UserID,
		ChangePoints:    changePoints,
		Reason:          reason,
		OperatorAdminID: req.OperatorAdmin,
		IdempotencyKey:  strings.TrimSpace(req.IdempotencyKey),
	})
}

func normalizeStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "", "pending", "active", "disabled":
		return status
	default:
		return ""
	}
}
