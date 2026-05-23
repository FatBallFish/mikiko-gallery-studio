package adminuser

import (
	"context"
	"errors"
	"net/mail"
	"regexp"
	"strings"

	domainadminuser "github.com/fatballfish/pic-gallery/internal/domain/adminuser"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
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

func (s *Service) CreateUser(ctx context.Context, req domainadminuser.CreateUserRequest) (domainadminuser.UserSummary, error) {
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Nickname = strings.TrimSpace(req.Nickname)
	rawStatus := strings.TrimSpace(req.Status)
	req.Status = normalizeStatus(rawStatus)
	req.UserGroupCode = strings.ToLower(strings.TrimSpace(req.UserGroupCode))
	req.DefaultLocale = strings.TrimSpace(req.DefaultLocale)
	req.Theme = strings.TrimSpace(req.Theme)
	if req.Email == "" {
		return domainadminuser.UserSummary{}, errs.BadRequest("email is required")
	}
	if _, err := mail.ParseAddress(req.Email); err != nil {
		return domainadminuser.UserSummary{}, errs.BadRequest("email is invalid")
	}
	if rawStatus != "" && req.Status == "" {
		return domainadminuser.UserSummary{}, errs.BadRequest("invalid status")
	}
	if req.Status == "" {
		req.Status = "active"
	}
	if req.UserGroupCode == "" {
		req.UserGroupCode = "basic"
	}
	if req.DefaultLocale == "" {
		req.DefaultLocale = "zh-CN"
	}
	if req.Theme == "" {
		req.Theme = "system"
	}
	if req.RPMLimit < 0 || req.ConcurrencyLimit < 0 {
		return domainadminuser.UserSummary{}, errs.BadRequest("rpm_limit and concurrency_limit must be non-negative")
	}
	user, err := s.store.CreateUser(ctx, req)
	if err != nil {
		return domainadminuser.UserSummary{}, normalizeStoreError(err, "user already exists")
	}
	return user, nil
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

func (s *Service) UpdateUserLimits(ctx context.Context, req domainadminuser.LimitsRequest) (domainadminuser.UserSummary, error) {
	if req.UserID <= 0 {
		return domainadminuser.UserSummary{}, errs.BadRequest("invalid user_id")
	}
	if req.RPMLimit < 0 || req.ConcurrencyLimit < 0 {
		return domainadminuser.UserSummary{}, errs.BadRequest("rpm_limit and concurrency_limit must be non-negative")
	}
	return s.store.UpdateUserLimits(ctx, req)
}

func (s *Service) AssignUserGroup(ctx context.Context, req domainadminuser.GroupAssignmentRequest) (domainadminuser.UserSummary, error) {
	if req.UserID <= 0 {
		return domainadminuser.UserSummary{}, errs.BadRequest("invalid user_id")
	}
	req.UserGroupCode = strings.ToLower(strings.TrimSpace(req.UserGroupCode))
	if req.UserGroupCode == "" {
		return domainadminuser.UserSummary{}, errs.BadRequest("user_group_code is required")
	}
	return s.store.AssignUserGroup(ctx, req)
}

func (s *Service) AssignUserGroups(ctx context.Context, req domainadminuser.MultiGroupAssignmentRequest) (domainadminuser.UserSummary, error) {
	if req.UserID <= 0 {
		return domainadminuser.UserSummary{}, errs.BadRequest("invalid user_id")
	}
	if len(req.GroupIDs) == 0 {
		return domainadminuser.UserSummary{}, errs.BadRequest("group_ids is required")
	}
	return s.store.AssignUserGroups(ctx, req)
}

func (s *Service) DeleteUser(ctx context.Context, userID int64) (domainadminuser.UserSummary, error) {
	if userID <= 0 {
		return domainadminuser.UserSummary{}, errs.BadRequest("invalid user_id")
	}
	user, err := s.store.DeleteUser(ctx, userID)
	if err != nil {
		return domainadminuser.UserSummary{}, normalizeStoreError(err, "user delete conflict")
	}
	return user, nil
}

func (s *Service) ListUserGroups(ctx context.Context, req domainadminuser.UserGroupListRequest) (domainadminuser.UserGroupListPage, error) {
	req.Page, req.PageSize = normalizePage(req.Page, req.PageSize)
	req.Query = strings.TrimSpace(req.Query)
	rawStatus := strings.TrimSpace(req.Status)
	req.Status = normalizeGroupStatus(rawStatus)
	if rawStatus != "" && req.Status == "" {
		return domainadminuser.UserGroupListPage{}, errs.BadRequest("invalid status")
	}
	return s.store.ListUserGroups(ctx, req)
}

func (s *Service) GetUserGroup(ctx context.Context, groupCode string) (domainadminuser.UserGroup, error) {
	groupCode = strings.ToLower(strings.TrimSpace(groupCode))
	if groupCode == "" {
		return domainadminuser.UserGroup{}, errs.BadRequest("group_code is required")
	}
	return s.store.GetUserGroup(ctx, groupCode)
}

func (s *Service) CreateUserGroup(ctx context.Context, req domainadminuser.UserGroupWriteRequest) (domainadminuser.UserGroup, error) {
	normalized, err := normalizeUserGroupWrite(req, true)
	if err != nil {
		return domainadminuser.UserGroup{}, err
	}
	return s.store.CreateUserGroup(ctx, normalized)
}

func (s *Service) UpdateUserGroup(ctx context.Context, groupCode string, req domainadminuser.UserGroupWriteRequest) (domainadminuser.UserGroup, error) {
	groupCode = strings.ToLower(strings.TrimSpace(groupCode))
	if groupCode == "" {
		return domainadminuser.UserGroup{}, errs.BadRequest("group_code is required")
	}
	req.GroupCode = groupCode
	normalized, err := normalizeUserGroupWrite(req, false)
	if err != nil {
		return domainadminuser.UserGroup{}, err
	}
	return s.store.UpdateUserGroup(ctx, groupCode, normalized)
}

func (s *Service) DeleteUserGroup(ctx context.Context, groupCode string) error {
	groupCode = strings.ToLower(strings.TrimSpace(groupCode))
	if groupCode == "" {
		return errs.BadRequest("group_code is required")
	}
	if groupCode == "basic" {
		return errs.BadRequest("basic group cannot be deleted")
	}
	return s.store.DeleteUserGroup(ctx, groupCode)
}

func normalizeStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "", "pending", "active", "disabled", "closed":
		return status
	default:
		return ""
	}
}

func normalizeGroupStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "", "active", "enabled", "disabled":
		if status == "active" {
			return "enabled"
		}
		return status
	default:
		return ""
	}
}

func normalizeUserGroupWrite(req domainadminuser.UserGroupWriteRequest, requireCode bool) (domainadminuser.UserGroupWriteRequest, error) {
	req.GroupCode = strings.ToLower(strings.TrimSpace(req.GroupCode))
	req.GroupName = strings.TrimSpace(req.GroupName)
	req.Multiplier = strings.TrimSpace(req.Multiplier)
	req.Status = normalizeGroupStatus(req.Status)
	if requireCode && req.GroupCode == "" {
		return domainadminuser.UserGroupWriteRequest{}, errs.BadRequest("group_code is required")
	}
	if req.GroupName == "" || req.Multiplier == "" {
		return domainadminuser.UserGroupWriteRequest{}, errs.BadRequest("group_name and multiplier are required")
	}
	if !pointValuePattern.MatchString(req.Multiplier) || strings.HasPrefix(req.Multiplier, "-") {
		return domainadminuser.UserGroupWriteRequest{}, errs.BadRequest("multiplier must be a non-negative decimal with up to 5 fractional digits")
	}
	if req.Status == "" {
		req.Status = "active"
	}
	return req, nil
}

func normalizeStoreError(err error, conflictMessage string) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, repoerr.ErrNotFound):
		return errs.New(404, errs.CodeNotFound, "user group not found")
	case errors.Is(err, repoerr.ErrConflict):
		return errs.New(409, errs.CodeConflict, conflictMessage)
	default:
		return err
	}
}
