package billing

import (
	"context"
	"net/http"
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

type SignupTrialGrantRequest struct {
	UserID         int64
	IdempotencyKey string
}

type SignupTrialGrantResult struct {
	Granted bool                         `json:"granted"`
	Balance domainbilling.BalanceSummary `json:"balance"`
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
	if strings.TrimSpace(cfg.SignupTrial.Points) == "" {
		cfg.SignupTrial.Points = "20.00000"
	}
	if cfg.SignupTrial.ValidDays == 0 {
		cfg.SignupTrial.ValidDays = 7
	}
	if cfg.SignupTrial.ExpiryReminderDays == 0 {
		cfg.SignupTrial.ExpiryReminderDays = 2
	}
	cfg.SignupTrial.GrantOncePerUser = true
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

func (s *Service) CreatePlan(ctx context.Context, req domainbilling.CreateSubscriptionPlanRequest) (domainbilling.SubscriptionPlan, error) {
	normalized, err := normalizePlanWrite(req)
	if err != nil {
		return domainbilling.SubscriptionPlan{}, err
	}
	item, err := s.store.CreatePlan(ctx, normalized)
	if err != nil {
		return domainbilling.SubscriptionPlan{}, err
	}
	return item, nil
}

func (s *Service) UpdatePlan(ctx context.Context, req domainbilling.UpdateSubscriptionPlanRequest) (domainbilling.SubscriptionPlan, error) {
	if req.PlanID <= 0 {
		return domainbilling.SubscriptionPlan{}, errs.BadRequest("plan_id is required")
	}
	normalizedCreate, err := normalizePlanWrite(domainbilling.CreateSubscriptionPlanRequest{
		PlanCode:        "existing",
		PlanName:        req.PlanName,
		PlanType:        req.PlanType,
		PurchaseEnabled: req.PurchaseEnabled,
		Status:          req.Status,
		PriceCNY:        req.PriceCNY,
		Points:          req.Points,
		BonusPoints:     req.BonusPoints,
		DurationDays:    req.DurationDays,
		Currency:        req.Currency,
		SortOrder:       req.SortOrder,
		Description:     req.Description,
	})
	if err != nil {
		return domainbilling.SubscriptionPlan{}, err
	}
	item, err := s.store.UpdatePlan(ctx, domainbilling.UpdateSubscriptionPlanRequest{
		PlanID:          req.PlanID,
		PlanName:        normalizedCreate.PlanName,
		PlanType:        normalizedCreate.PlanType,
		PurchaseEnabled: normalizedCreate.PurchaseEnabled,
		Status:          normalizedCreate.Status,
		PriceCNY:        normalizedCreate.PriceCNY,
		Points:          normalizedCreate.Points,
		BonusPoints:     normalizedCreate.BonusPoints,
		DurationDays:    normalizedCreate.DurationDays,
		Currency:        normalizedCreate.Currency,
		SortOrder:       normalizedCreate.SortOrder,
		Description:     normalizedCreate.Description,
	})
	if err != nil {
		return domainbilling.SubscriptionPlan{}, err
	}
	return item, nil
}

func normalizePlanWrite(req domainbilling.CreateSubscriptionPlanRequest) (domainbilling.CreateSubscriptionPlanRequest, error) {
	req.PlanCode = strings.ToLower(strings.TrimSpace(req.PlanCode))
	if req.PlanCode == "" {
		return req, errs.BadRequest("plan_code is required")
	}
	req.PlanName = strings.TrimSpace(req.PlanName)
	if req.PlanName == "" {
		return req, errs.BadRequest("plan_name is required")
	}
	req.PlanType = strings.ToLower(strings.TrimSpace(req.PlanType))
	if req.PlanType == "" {
		req.PlanType = "points_package"
	}
	if req.PlanType != "points_package" && req.PlanType != "subscription" {
		return req, errs.BadRequest("plan_type must be points_package or subscription")
	}
	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	if req.Status == "" {
		req.Status = "active"
	}
	if req.Status != "active" && req.Status != "disabled" && req.Status != "archived" {
		return req, errs.BadRequest("status must be active, disabled, or archived")
	}
	price, err := positivePlanDecimal(req.PriceCNY, "price_cny")
	if err != nil {
		return req, err
	}
	points, err := positivePlanDecimal(req.Points, "points")
	if err != nil {
		return req, err
	}
	bonus, err := nonNegativePlanDecimal(req.BonusPoints, "bonus_points")
	if err != nil {
		return req, err
	}
	req.PriceCNY = price
	req.Points = points
	req.BonusPoints = bonus
	if req.DurationDays <= 0 {
		req.DurationDays = 30
	}
	req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
	if req.Currency == "" {
		req.Currency = "CNY"
	}
	if req.Currency != "CNY" {
		return req, errs.BadRequest("currency must be CNY")
	}
	req.Description = strings.TrimSpace(req.Description)
	return req, nil
}

func positivePlanDecimal(raw, field string) (string, error) {
	value, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil || !value.IsPositive() {
		return "", errs.BadRequest(field + " must be positive")
	}
	return value.StringFixed(5), nil
}

func nonNegativePlanDecimal(raw, field string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		raw = "0"
	}
	value, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil || value.IsNegative() {
		return "", errs.BadRequest(field + " must be non-negative")
	}
	return value.StringFixed(5), nil
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

func (s *Service) ListWebhookEvents(ctx context.Context, page, pageSize int) (domainbilling.PaymentWebhookEventPage, error) {
	items, err := s.store.ListWebhookEvents(ctx, page, pageSize)
	if err != nil {
		return domainbilling.PaymentWebhookEventPage{}, errs.Internal("failed to load payment webhook events")
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

func (s *Service) GetOrderByIdempotencyKey(ctx context.Context, userID int64, idempotencyKey string) (domainbilling.PaymentOrder, error) {
	if strings.TrimSpace(idempotencyKey) == "" {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
	}
	item, err := s.store.GetOrderByIdempotencyKey(ctx, userID, idempotencyKey)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	return item, nil
}

func (s *Service) GetOrderForAdmin(ctx context.Context, orderID int64) (domainbilling.PaymentOrder, error) {
	if orderID <= 0 {
		return domainbilling.PaymentOrder{}, errs.BadRequest("order_id is required")
	}
	item, err := s.store.GetOrderForAdmin(ctx, orderID)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	return item, nil
}

func (s *Service) RecordChargebackSummary(ctx context.Context, req ChargebackSummaryStoreRequest) (domainbilling.PaymentOrder, error) {
	if req.OrderID <= 0 {
		return domainbilling.PaymentOrder{}, errs.BadRequest("order_id is required")
	}
	points, err := decimal.NewFromString(strings.TrimSpace(req.ChargePoints))
	if err != nil || !points.IsPositive() {
		return domainbilling.PaymentOrder{}, errs.BadRequest("charge_points must be positive")
	}
	req.ChargePoints = points.Round(5).StringFixed(5)
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" {
		return domainbilling.PaymentOrder{}, errs.BadRequest("reason is required")
	}
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	item, err := s.store.RecordChargebackSummary(ctx, req)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	return item, nil
}

func (s *Service) RetryWebhookEvent(ctx context.Context, eventID int64) (domainbilling.PaymentWebhookEvent, error) {
	if eventID <= 0 {
		return domainbilling.PaymentWebhookEvent{}, errs.BadRequest("event_id is required")
	}
	item, err := s.store.RetryWebhookEvent(ctx, eventID)
	if err != nil {
		return domainbilling.PaymentWebhookEvent{}, err
	}
	return item, nil
}

func (s *Service) ProcessRefundFinalizeFailures(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 5
	}
	return s.store.ProcessRefundFinalizeFailures(ctx, limit)
}

func (s *Service) RecordRefundFinalizeFailure(ctx context.Context, req RefundFinalizeFailureRequest) (domainbilling.PaymentWebhookEvent, error) {
	if req.UserID <= 0 || req.OrderID <= 0 {
		return domainbilling.PaymentWebhookEvent{}, errs.BadRequest("user_id and order_id are required")
	}
	if strings.TrimSpace(req.RefundTradeNo) == "" {
		return domainbilling.PaymentWebhookEvent{}, errs.BadRequest("refund_trade_no is required")
	}
	item, err := s.store.RecordRefundFinalizeFailure(ctx, req)
	if err != nil {
		return domainbilling.PaymentWebhookEvent{}, err
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

func (s *Service) CreateCustomAmountOrder(ctx context.Context, req domainbilling.CreateCustomAmountOrderRequest) (domainbilling.PaymentOrder, error) {
	item, err := s.store.CreateCustomAmountOrder(ctx, req)
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

func (s *Service) CompleteRechargeOrder(ctx context.Context, req domainbilling.CompleteRechargeOrderRequest) (domainbilling.PaymentOrder, error) {
	item, err := s.store.CompleteRechargeOrder(ctx, req)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	return item, nil
}

func (s *Service) RefundPaymentOrder(ctx context.Context, req domainbilling.RefundPaymentOrderRequest) (domainbilling.PaymentOrder, error) {
	item, err := s.store.RefundPaymentOrder(ctx, req)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	return item, nil
}

func (s *Service) CheckRefundPaymentOrder(ctx context.Context, req domainbilling.RefundPaymentOrderRequest) (domainbilling.PaymentOrder, error) {
	item, err := s.store.CheckRefundPaymentOrder(ctx, req)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	return item, nil
}

func (s *Service) FreezeRefundPaymentOrder(ctx context.Context, req domainbilling.RefundPaymentOrderRequest) (domainbilling.PaymentOrder, error) {
	item, err := s.store.FreezeRefundPaymentOrder(ctx, req)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	return item, nil
}

func (s *Service) ReleaseRefundPaymentOrder(ctx context.Context, req domainbilling.RefundPaymentOrderRequest) (domainbilling.PaymentOrder, error) {
	item, err := s.store.ReleaseRefundPaymentOrder(ctx, req)
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

func (s *Service) EnsureSignupTrialGrant(ctx context.Context, req SignupTrialGrantRequest) (SignupTrialGrantResult, error) {
	if req.UserID <= 0 {
		return SignupTrialGrantResult{}, errs.BadRequest("user id is required")
	}
	if !s.cfg.SignupTrial.Enabled {
		balance, err := s.GetBalance(ctx, req.UserID, "")
		if err != nil {
			return SignupTrialGrantResult{}, err
		}
		return SignupTrialGrantResult{Balance: balance}, nil
	}
	points, err := decimal.NewFromString(strings.TrimSpace(s.cfg.SignupTrial.Points))
	if err != nil {
		return SignupTrialGrantResult{}, errs.Internal("invalid signup trial points config")
	}
	if !points.IsPositive() {
		balance, err := s.GetBalance(ctx, req.UserID, "")
		if err != nil {
			return SignupTrialGrantResult{}, err
		}
		return SignupTrialGrantResult{Balance: balance}, nil
	}
	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if idempotencyKey == "" || s.cfg.SignupTrial.GrantOncePerUser {
		idempotencyKey = signupTrialLedgerKey(req.UserID)
	}
	result, err := s.store.EnsureSignupTrialGrant(ctx, SignupTrialGrantStoreRequest{
		UserID:             req.UserID,
		Points:             points.Round(int32(s.cfg.PointsScale)).StringFixed(int32(s.cfg.PointsScale)),
		ValidDays:          s.cfg.SignupTrial.ValidDays,
		ExpiryReminderDays: s.cfg.SignupTrial.ExpiryReminderDays,
		IdempotencyKey:     idempotencyKey,
	})
	if err != nil {
		return SignupTrialGrantResult{}, err
	}
	balance, err := s.balanceSummaryFromState(result.Balance, "")
	if err != nil {
		return SignupTrialGrantResult{}, err
	}
	return SignupTrialGrantResult{Granted: result.Granted, Balance: balance}, nil
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
		TrialPoints:         state.TrialPoints,
		SubscriptionPoints:  state.SubscriptionPoints,
		GiftPoints:          state.GiftPoints,
		RechargePoints:      state.RechargePoints,
		Buckets:             state.Buckets,
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
