package billing

import (
	"context"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type Service struct {
	cfg     config.BillingConfig
	calc    *domainbilling.Calculator
	store   Store
	routing modelhub.ModelRoutingSource
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

func (s *Service) SetModelRoutingSource(source modelhub.ModelRoutingSource) {
	s.routing = source
}

func (s *Service) Estimate(req domainbilling.EstimateRequest) (domainbilling.EstimateResult, error) {
	if strings.TrimSpace(req.RouteModelCode) != "" {
		return s.estimateRouteModel(req)
	}
	return s.calc.Estimate(req)
}

func (s *Service) estimateRouteModel(req domainbilling.EstimateRequest) (domainbilling.EstimateResult, error) {
	if s.routing == nil {
		return domainbilling.EstimateResult{}, errs.New(409, errs.CodeConflict, "model routing is not configured")
	}
	resolver := modelhub.NewResolver(config.Config{
		Billing: s.cfg,
		GenerationLimits: config.GenerationLimitsConfig{
			MaxImageCount:          1 << 30,
			ReferenceImageMaxCount: 1 << 30,
		},
	})
	resolver.SetModelRoutingSource(s.routing)
	groupCodes := append([]string(nil), req.UserGroupCodes...)
	if len(groupCodes) == 0 && strings.TrimSpace(req.UserGroupCode) != "" {
		groupCodes = append(groupCodes, req.UserGroupCode)
	}
	resolved, err := resolver.ResolveContext(context.Background(), modelhub.ResolveRequest{
		RouteModelCode:            req.RouteModelCode,
		TaskType:                  req.TaskType,
		RequestedQuality:          req.RequestedQuality,
		RequestedSize:             req.RequestedSize,
		RequestedOutputImageCount: req.RequestedOutputImageCount,
		ReferenceImageCount:       req.ReferenceImageCount,
		UserGroupCodes:            groupCodes,
	})
	if err != nil {
		return domainbilling.EstimateResult{}, err
	}
	quality := resolved.ResolvedQualityBucket
	models, err := resolver.ListVisibleRouteModels(context.Background(), groupCodes, s.cfg.TaskMultipliers)
	if err != nil {
		return domainbilling.EstimateResult{}, err
	}
	routeCode := strings.ToLower(strings.TrimSpace(req.RouteModelCode))
	for _, model := range models {
		if !strings.EqualFold(model.Code, routeCode) {
			continue
		}
		for _, price := range model.Prices {
			if !strings.EqualFold(price.TaskType, req.TaskType) {
				continue
			}
			if !strings.EqualFold(price.Quality, quality) {
				continue
			}
			count := req.RequestedOutputImageCount
			if count <= 0 {
				count = 1
			}
			charged, err := decimal.NewFromString(price.ChargedPoints)
			if err != nil {
				return domainbilling.EstimateResult{}, errs.Internal("invalid route model price")
			}
			total := charged.Mul(decimal.NewFromInt(int64(count))).Round(5)
			snapshot := domainbilling.PricingSnapshot{
				RouteModelCode:            model.Code,
				AbstractModel:             model.Code,
				TaskType:                  req.TaskType,
				RequestedQuality:          req.RequestedQuality,
				RequestedSize:             req.RequestedSize,
				ResolvedQualityBucket:     price.Quality,
				RequestedOutputImageCount: count,
				ReferenceImageCount:       req.ReferenceImageCount,
				UserGroupCode:             strings.Join(groupCodes, ","),
				UserGroupMultiplier:       model.EffectiveMultiplier,
				BaseUnitPoints:            price.BasePoints,
				TaskMultiplier:            defaultBillingString(s.cfg.TaskMultipliers[req.TaskType], "1.00000"),
				ReferenceExtraMultiplier:  "0.00000",
				EstimatedPoints:           total.StringFixed(5),
			}
			return domainbilling.EstimateResult{
				ResolvedQualityBucket:     price.Quality,
				EstimatedPoints:           snapshot.EstimatedPoints,
				ChargedPoints:             snapshot.EstimatedPoints,
				DisplayPoints:             total.Round(2).StringFixed(2),
				UserGroupMultiplier:       model.EffectiveMultiplier,
				RequestedOutputImageCount: count,
				ReferenceImageCount:       req.ReferenceImageCount,
				PricingSnapshot:           snapshot,
			}, nil
		}
	}
	return domainbilling.EstimateResult{}, errs.New(403, errs.CodeForbidden, "route model is not visible")
}

func defaultBillingString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
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

func (s *Service) ListPlans(ctx context.Context) ([]domainbilling.SubscriptionPlan, error) {
	items, err := s.store.ListPlans(ctx)
	if err != nil {
		return nil, errs.Internal("failed to load subscription plans")
	}
	return items, nil
}

func (s *Service) GetSubscription(ctx context.Context, userID int64) (*domainbilling.UserSubscriptionSummary, error) {
	item, err := s.store.GetActiveSubscription(ctx, userID)
	if err != nil {
		return nil, errs.Internal("failed to load active subscription")
	}
	return item, nil
}

func (s *Service) ListOrders(ctx context.Context, req domainbilling.ListOrdersRequest) (domainbilling.PaymentOrderPage, error) {
	items, err := s.store.ListOrders(ctx, req)
	if err != nil {
		return domainbilling.PaymentOrderPage{}, errs.Internal("failed to load payment orders")
	}
	return items, nil
}

func (s *Service) GetOrder(ctx context.Context, userID, orderID int64) (domainbilling.PaymentOrder, error) {
	item, err := s.store.GetOrder(ctx, userID, orderID)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	return item, nil
}

func (s *Service) CreateOrder(ctx context.Context, req domainbilling.CreateOrderRequest) (domainbilling.PaymentOrder, error) {
	item, err := s.store.CreateOrder(ctx, req)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	return item, nil
}

func (s *Service) CancelOrder(ctx context.Context, userID, orderID int64) (domainbilling.PaymentOrder, error) {
	item, err := s.store.CancelOrder(ctx, userID, orderID)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	return item, nil
}

func (s *Service) MarkOrderPaid(ctx context.Context, req domainbilling.MarkOrderPaidRequest) (domainbilling.PaymentOrder, error) {
	item, err := s.store.MarkOrderPaid(ctx, req)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	return item, nil
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
		SubscriptionPoints:  state.SubscriptionPoints,
		GiftPoints:          state.GiftPoints,
		RechargePoints:      state.RechargePoints,
		UserGroupMultiplier: groupMultiplier,
		CNYPerPoint:         cnyPerPoint,
		ActiveSubscription:  state.ActiveSubscription,
		NextExpiringGrant:   state.NextExpiringGrant,
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
