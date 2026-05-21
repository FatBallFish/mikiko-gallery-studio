package billing

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type Service struct {
	cfg   config.BillingConfig
	calc  *domainbilling.Calculator
	store Store
}

func NewService(cfg config.BillingConfig) *Service {
	cfg = normalizeBillingConfig(cfg)
	return NewServiceWithStore(cfg, NewMemoryStore(cfg.PointsScale))
}

func NewServiceWithStore(cfg config.BillingConfig, store Store) *Service {
	cfg = normalizeBillingConfig(cfg)
	if store == nil {
		store = NewMemoryStore(cfg.PointsScale)
	}
	return &Service{cfg: cfg, calc: domainbilling.NewCalculator(cfg), store: store}
}

func normalizeBillingConfig(cfg config.BillingConfig) config.BillingConfig {
	cfg.PointsScale = 5
	return cfg
}

func (s *Service) Estimate(req domainbilling.EstimateRequest) (domainbilling.EstimateResult, error) {
	return s.calc.Estimate(req)
}

func (s *Service) ActualPoints(snapshot domainbilling.PricingSnapshot, successOutputImageCount int) (string, error) {
	return s.calc.ActualPoints(snapshot, successOutputImageCount)
}

func (s *Service) GetBalance(ctx context.Context, userID int64, userGroupMultiplier string) (domainbilling.BalanceSummary, error) {
	state, err := s.store.GetBalance(ctx, userID)
	if err != nil {
		return domainbilling.BalanceSummary{}, errs.Internal("failed to load points balance")
	}
	return s.balanceSummaryFromState(state, userGroupMultiplier)
}

func (s *Service) ListLedger(ctx context.Context, userID int64, page, pageSize int) (domainbilling.LedgerPage, error) {
	pageResult, err := s.store.ListLedger(ctx, userID, page, pageSize)
	if err != nil {
		return domainbilling.LedgerPage{}, errs.Internal("failed to load points ledger")
	}
	return pageResult, nil
}

func (s *Service) ReserveTask(ctx context.Context, req domainbilling.ReserveRequest) (domainbilling.BalanceSummary, error) {
	state, err := s.store.ReserveTask(ctx, ReserveStoreRequest(req))
	if err != nil {
		return domainbilling.BalanceSummary{}, err
	}
	return s.balanceSummaryFromState(state, "")
}

func (s *Service) FinalizeTask(ctx context.Context, req domainbilling.FinalizeRequest) (domainbilling.BalanceSummary, error) {
	state, err := s.store.FinalizeTask(ctx, FinalizeStoreRequest(req))
	if err != nil {
		return domainbilling.BalanceSummary{}, err
	}
	return s.balanceSummaryFromState(state, "")
}

func (s *Service) AdminAdjust(ctx context.Context, req domainbilling.AdjustRequest) (domainbilling.BalanceSummary, error) {
	state, err := s.store.Adjust(ctx, AdjustStoreRequest(req))
	if err != nil {
		return domainbilling.BalanceSummary{}, err
	}
	return s.balanceSummaryFromState(state, "")
}

func (s *Service) RedeemCode(ctx context.Context, req RedeemCodeRequest) (domainbilling.BalanceSummary, error) {
	state, err := s.store.RedeemCode(ctx, req)
	if err != nil {
		return domainbilling.BalanceSummary{}, err
	}
	return s.balanceSummaryFromState(state, "")
}

func (s *Service) APIKeyUsage(ctx context.Context, apiKeyID int64, since *time.Time) (string, error) {
	return s.store.APIKeyUsage(ctx, apiKeyID, since)
}

func (s *Service) balanceSummaryFromState(state BalanceState, userGroupMultiplier string) (domainbilling.BalanceSummary, error) {
	groupMultiplier, err := scaledDecimalString(userGroupMultiplier, "1", s.cfg.PointsScale)
	if err != nil {
		return domainbilling.BalanceSummary{}, errs.Internal("invalid user group multiplier")
	}
	cnyPerPoint, err := scaledDecimalString(s.cfg.CNYPerPoint, "0", s.cfg.PointsScale)
	if err != nil {
		return domainbilling.BalanceSummary{}, errs.Internal("invalid cny per point config")
	}
	return domainbilling.BalanceSummary{
		AvailablePoints:     state.AvailablePoints,
		FrozenPoints:        state.FrozenPoints,
		UserGroupMultiplier: groupMultiplier,
		CNYPerPoint:         cnyPerPoint,
	}, nil
}

func scaledDecimalString(value, fallback string, scale int) (string, error) {
	trimmed := value
	if trimmed == "" {
		trimmed = fallback
	}
	parsed, err := decimal.NewFromString(trimmed)
	if err != nil {
		return "", err
	}
	return parsed.StringFixed(int32(scale)), nil
}
