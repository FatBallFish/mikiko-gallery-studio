package redeem

import (
	"context"
	"crypto/rand"
	"math/big"
	"regexp"
	"strings"
	"time"

	domainredeem "github.com/fatballfish/pic-gallery/internal/domain/redeem"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/shopspring/decimal"
)

const (
	defaultGeneratedCodeLength = 12
	maxGenerateAttempts        = 32
	safeCodeAlphabet           = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
)

var pointValuePattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]{1,5})?$`)
var manualCodePattern = regexp.MustCompile(`^[A-Z0-9_-]{4,64}$`)

type Service struct {
	store Store
}

func NewServiceWithStore(store Store) *Service {
	if store == nil {
		store = NewMemoryStore()
	}
	return &Service{store: store}
}

func (s *Service) ListCodes(ctx context.Context, req domainredeem.ListRequest) (domainredeem.ListPage, error) {
	req.Page, req.PageSize = normalizePage(req.Page, req.PageSize)
	rawStatus := strings.TrimSpace(req.Status)
	req.Status = normalizeStatus(rawStatus)
	if rawStatus != "" && req.Status == "" {
		return domainredeem.ListPage{}, errs.BadRequest("invalid status")
	}
	req.Code = strings.ToUpper(strings.TrimSpace(req.Code))
	if req.BatchID < 0 {
		return domainredeem.ListPage{}, errs.BadRequest("invalid batch_id")
	}
	return s.store.ListCodes(ctx, req)
}

func (s *Service) CreateCode(ctx context.Context, req domainredeem.CreateRequest) (domainredeem.Code, error) {
	normalized, err := s.normalizeCreateRequest(ctx, req)
	if err != nil {
		return domainredeem.Code{}, err
	}
	return s.store.CreateCode(ctx, normalized)
}

func (s *Service) BatchCreate(ctx context.Context, req domainredeem.BatchCreateRequest) (domainredeem.BatchCreateResult, error) {
	if req.Count <= 0 || req.Count > 100 {
		return domainredeem.BatchCreateResult{}, errs.BadRequest("count must be between 1 and 100")
	}
	if req.BatchID < 0 {
		return domainredeem.BatchCreateResult{}, errs.BadRequest("invalid batch_id")
	}
	batchID := req.BatchID
	if batchID == 0 {
		batchID = time.Now().UTC().UnixNano()
	}
	items := make([]domainredeem.Code, 0, req.Count)
	for i := 0; i < req.Count; i++ {
		item, err := s.CreateCode(ctx, domainredeem.CreateRequest{
			BatchID:        batchID,
			Status:         req.Status,
			RewardType:     req.RewardType,
			RewardValue:    req.RewardValue,
			ValidFrom:      req.ValidFrom,
			ValidUntil:     req.ValidUntil,
			MaxRedemptions: req.MaxRedemptions,
			OperatorAdmin:  req.OperatorAdmin,
		})
		if err != nil {
			return domainredeem.BatchCreateResult{}, err
		}
		items = append(items, item)
	}
	return domainredeem.BatchCreateResult{Items: items, Count: len(items), BatchID: batchID}, nil
}

func (s *Service) UpdateStatus(ctx context.Context, req domainredeem.StatusRequest) (domainredeem.Code, error) {
	if req.ID <= 0 {
		return domainredeem.Code{}, errs.BadRequest("invalid code_id")
	}
	status := normalizeStatus(req.Status)
	if status == "" {
		return domainredeem.Code{}, errs.BadRequest("invalid status")
	}
	return s.store.UpdateStatus(ctx, req.ID, status)
}

func (s *Service) ListRedemptions(ctx context.Context, codeID int64, page, pageSize int) (domainredeem.RedemptionsPage, error) {
	if codeID <= 0 {
		return domainredeem.RedemptionsPage{}, errs.BadRequest("invalid code_id")
	}
	page, pageSize = normalizePage(page, pageSize)
	return s.store.ListRedemptions(ctx, codeID, page, pageSize)
}

func (s *Service) normalizeCreateRequest(ctx context.Context, req domainredeem.CreateRequest) (domainredeem.CreateRequest, error) {
	req.Code = strings.ToUpper(strings.TrimSpace(req.Code))
	if req.Code == "" {
		code, err := s.generateUniqueCode(ctx)
		if err != nil {
			return domainredeem.CreateRequest{}, err
		}
		req.Code = code
	} else if !isSafeCode(req.Code) {
		return domainredeem.CreateRequest{}, errs.BadRequest("code must contain only uppercase safe characters")
	}
	status := normalizeStatus(req.Status)
	if status == "" {
		status = "available"
	}
	req.Status = status
	if req.BatchID < 0 {
		return domainredeem.CreateRequest{}, errs.BadRequest("invalid batch_id")
	}
	req.RewardType = strings.ToLower(strings.TrimSpace(req.RewardType))
	if req.RewardType == "" {
		req.RewardType = "points"
	}
	if req.RewardType != "points" {
		return domainredeem.CreateRequest{}, errs.BadRequest("reward_type must be points")
	}
	req.RewardValue = strings.TrimSpace(req.RewardValue)
	if req.RewardValue == "" || !pointValuePattern.MatchString(req.RewardValue) {
		return domainredeem.CreateRequest{}, errs.BadRequest("reward_value must be a positive decimal with up to 5 fractional digits")
	}
	rewardValue, err := decimal.NewFromString(req.RewardValue)
	if err != nil || !rewardValue.IsPositive() {
		return domainredeem.CreateRequest{}, errs.BadRequest("reward_value must be a positive decimal with up to 5 fractional digits")
	}
	if req.ValidFrom.IsZero() {
		req.ValidFrom = time.Now().UTC()
	}
	if req.ValidUntil.IsZero() {
		return domainredeem.CreateRequest{}, errs.BadRequest("valid_until is required")
	}
	req.ValidFrom = req.ValidFrom.UTC()
	req.ValidUntil = req.ValidUntil.UTC()
	if !req.ValidUntil.After(req.ValidFrom) {
		return domainredeem.CreateRequest{}, errs.BadRequest("valid_until must be after valid_from")
	}
	if req.MaxRedemptions <= 0 {
		req.MaxRedemptions = 1
	}
	return req, nil
}

func (s *Service) generateUniqueCode(ctx context.Context) (string, error) {
	for attempt := 0; attempt < maxGenerateAttempts; attempt++ {
		code, err := randomSafeCode(defaultGeneratedCodeLength)
		if err != nil {
			return "", err
		}
		exists, err := s.store.CodeExists(ctx, code)
		if err != nil {
			return "", err
		}
		if !exists {
			return code, nil
		}
	}
	return "", errs.Internal("failed to generate unique redeem code")
}

func randomSafeCode(length int) (string, error) {
	var b strings.Builder
	b.Grow(length)
	max := big.NewInt(int64(len(safeCodeAlphabet)))
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b.WriteByte(safeCodeAlphabet[n.Int64()])
	}
	return b.String(), nil
}

func normalizeStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "", "inactive", "available", "redeemed", "expired", "disabled":
		return status
	default:
		return ""
	}
}

func isSafeCode(code string) bool {
	return manualCodePattern.MatchString(code)
}
