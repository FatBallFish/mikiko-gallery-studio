package billing

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type BalanceState struct {
	AvailablePoints    string
	FrozenPoints       string
	TrialPoints        string
	SubscriptionPoints string
	GiftPoints         string
	RechargePoints     string
	Buckets            []domainbilling.BalanceBucket
	ActiveSubscription *domainbilling.UserSubscriptionSummary
	NextExpiringGrant  *domainbilling.GrantExpirySummary
}

type ReserveStoreRequest struct {
	UserID          int64
	APIKeyID        int64
	TaskID          string
	EstimatedPoints string
	Reason          string
	domainbilling.APIKeyQuota
}

type FinalizeStoreRequest struct {
	UserID          int64
	APIKeyID        int64
	TaskID          string
	EstimatedPoints string
	ActualPoints    string
	Reason          string
}

type AdjustStoreRequest struct {
	UserID          int64
	ChangePoints    string
	Reason          string
	OperatorAdminID int64
	IdempotencyKey  string
}

type SignupTrialGrantStoreRequest struct {
	UserID             int64
	Points             string
	ValidDays          int
	ExpiryReminderDays int
	IdempotencyKey     string
}

type SignupTrialGrantStoreResult struct {
	Granted bool
	Balance BalanceState
}

type RedeemCodeRequest struct {
	UserID         int64
	Code           string
	IdempotencyKey string
}

type RefundFinalizeFailureRequest struct {
	domainbilling.RefundPaymentOrderRequest
	FailureReason string
}

type ChargebackSummaryStoreRequest struct {
	OrderID        int64
	ChargePoints   string
	Reason         string
	IdempotencyKey string
}

type Store interface {
	GetBalance(ctx context.Context, userID int64) (BalanceState, error)
	ListLedger(ctx context.Context, userID int64, page, pageSize int) (domainbilling.LedgerPage, error)
	ListPlans(ctx context.Context) ([]domainbilling.SubscriptionPlan, error)
	CreatePlan(ctx context.Context, req domainbilling.CreateSubscriptionPlanRequest) (domainbilling.SubscriptionPlan, error)
	UpdatePlan(ctx context.Context, req domainbilling.UpdateSubscriptionPlanRequest) (domainbilling.SubscriptionPlan, error)
	DeletePlan(ctx context.Context, planID int64) (domainbilling.SubscriptionPlan, error)
	GetActiveSubscription(ctx context.Context, userID int64) (*domainbilling.UserSubscriptionSummary, error)
	ListOrders(ctx context.Context, req domainbilling.ListOrdersRequest) (domainbilling.PaymentOrderPage, error)
	ListWebhookEvents(ctx context.Context, page, pageSize int) (domainbilling.PaymentWebhookEventPage, error)
	GetOrder(ctx context.Context, userID int64, orderID int64) (domainbilling.PaymentOrder, error)
	GetOrderByIdempotencyKey(ctx context.Context, userID int64, idempotencyKey string) (domainbilling.PaymentOrder, error)
	GetOrderByOrderNo(ctx context.Context, orderNo string) (domainbilling.PaymentOrder, error)
	GetOrderForAdmin(ctx context.Context, orderID int64) (domainbilling.PaymentOrder, error)
	RecordChargebackSummary(ctx context.Context, req ChargebackSummaryStoreRequest) (domainbilling.PaymentOrder, error)
	RetryWebhookEvent(ctx context.Context, eventID int64) (domainbilling.PaymentWebhookEvent, error)
	ProcessRefundFinalizeFailures(ctx context.Context, limit int) (int, error)
	CreateOrder(ctx context.Context, req domainbilling.CreateOrderRequest) (domainbilling.PaymentOrder, error)
	CreateCustomAmountOrder(ctx context.Context, req domainbilling.CreateCustomAmountOrderRequest) (domainbilling.PaymentOrder, error)
	CancelOrder(ctx context.Context, userID int64, orderID int64) (domainbilling.PaymentOrder, error)
	MarkOrderPaid(ctx context.Context, req domainbilling.MarkOrderPaidRequest) (domainbilling.PaymentOrder, error)
	CompleteRechargeOrder(ctx context.Context, req domainbilling.CompleteRechargeOrderRequest) (domainbilling.PaymentOrder, error)
	CheckRefundPaymentOrder(ctx context.Context, req domainbilling.RefundPaymentOrderRequest) (domainbilling.PaymentOrder, error)
	FreezeRefundPaymentOrder(ctx context.Context, req domainbilling.RefundPaymentOrderRequest) (domainbilling.PaymentOrder, error)
	ReleaseRefundPaymentOrder(ctx context.Context, req domainbilling.RefundPaymentOrderRequest) (domainbilling.PaymentOrder, error)
	RefundPaymentOrder(ctx context.Context, req domainbilling.RefundPaymentOrderRequest) (domainbilling.PaymentOrder, error)
	RecordRefundFinalizeFailure(ctx context.Context, req RefundFinalizeFailureRequest) (domainbilling.PaymentWebhookEvent, error)
	ReserveTask(ctx context.Context, req ReserveStoreRequest) (BalanceState, error)
	FinalizeTask(ctx context.Context, req FinalizeStoreRequest) (BalanceState, error)
	Adjust(ctx context.Context, req AdjustStoreRequest) (BalanceState, error)
	EnsureSignupTrialGrant(ctx context.Context, req SignupTrialGrantStoreRequest) (SignupTrialGrantStoreResult, error)
	RedeemCode(ctx context.Context, req RedeemCodeRequest) (BalanceState, error)
	APIKeyUsage(ctx context.Context, apiKeyID int64, since *time.Time) (string, error)
}

type memoryTaskBillingState struct {
	UserID   int64
	APIKeyID int64
	Seen     bool
	Active   bool
	Cycle    int
	Reserved decimal.Decimal
}

type MemoryStore struct {
	mu            sync.Mutex
	scale         int32
	nextID        int64
	balances      map[int64]balanceState
	ledgers       map[int64][]domainbilling.LedgerEntry
	apiKeyUsage   map[int64]map[string]decimal.Decimal
	taskState     map[string]memoryTaskBillingState
	adjustKeys    map[string]AdjustStoreRequest
	plans         []domainbilling.SubscriptionPlan
	nextPlanID    int64
	orders        map[int64]domainbilling.PaymentOrder
	nextOrderID   int64
	webhooks      []domainbilling.PaymentWebhookEvent
	nextWebhookID int64
	breakdown     map[int64]memoryBreakdown
	subs          map[int64]*domainbilling.UserSubscriptionSummary
	trialGrants   map[string]SignupTrialGrantStoreRequest
	refundFreezes map[int64]decimal.Decimal
	refundTrades  map[int64]map[string]struct{}
	refundRetries map[int64]domainbilling.RefundPaymentOrderRequest
}

type memoryBreakdown struct {
	Trial             decimal.Decimal
	Subscription      decimal.Decimal
	Gift              decimal.Decimal
	Recharge          decimal.Decimal
	TrialExpires      *time.Time
	TrialReminderDays int
}

type balanceState struct {
	Available decimal.Decimal
	Frozen    decimal.Decimal
}

func NewMemoryStore(scale int) *MemoryStore {
	scale = 5
	return &MemoryStore{
		scale:         int32(scale),
		nextID:        1,
		balances:      map[int64]balanceState{},
		ledgers:       map[int64][]domainbilling.LedgerEntry{},
		apiKeyUsage:   map[int64]map[string]decimal.Decimal{},
		taskState:     map[string]memoryTaskBillingState{},
		adjustKeys:    map[string]AdjustStoreRequest{},
		plans:         defaultPlans(),
		nextPlanID:    3,
		orders:        map[int64]domainbilling.PaymentOrder{},
		nextOrderID:   1,
		webhooks:      []domainbilling.PaymentWebhookEvent{},
		nextWebhookID: 1,
		breakdown:     map[int64]memoryBreakdown{},
		subs:          map[int64]*domainbilling.UserSubscriptionSummary{},
		trialGrants:   map[string]SignupTrialGrantStoreRequest{},
		refundFreezes: map[int64]decimal.Decimal{},
		refundTrades:  map[int64]map[string]struct{}{},
		refundRetries: map[int64]domainbilling.RefundPaymentOrderRequest{},
	}
}

func (s *MemoryStore) GetBalance(_ context.Context, userID int64) (BalanceState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.formatState(userID, s.balances[userID]), nil
}

func (s *MemoryStore) ListLedger(_ context.Context, userID int64, page, pageSize int) (domainbilling.LedgerPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	all := append([]domainbilling.LedgerEntry{}, s.ledgers[userID]...)
	total := len(all)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return domainbilling.LedgerPage{
		Items:    all[start:end],
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}, nil
}

func (s *MemoryStore) ListPlans(_ context.Context) ([]domainbilling.SubscriptionPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]domainbilling.SubscriptionPlan, 0, len(s.plans))
	items = append(items, s.plans...)
	return items, nil
}

func (s *MemoryStore) CreatePlan(_ context.Context, req domainbilling.CreateSubscriptionPlanRequest) (domainbilling.SubscriptionPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	code := strings.TrimSpace(req.PlanCode)
	for _, item := range s.plans {
		if strings.EqualFold(item.PlanCode, code) {
			return domainbilling.SubscriptionPlan{}, errs.New(http.StatusConflict, errs.CodeConflict, "subscription plan code already exists")
		}
	}
	now := time.Now().UTC()
	plan := domainbilling.SubscriptionPlan{
		ID:              s.nextPlanID,
		PlanCode:        code,
		PlanName:        strings.TrimSpace(req.PlanName),
		PlanType:        normalizePlanType(req.PlanType),
		PurchaseEnabled: req.PurchaseEnabled,
		Status:          normalizePlanStatus(req.Status),
		PriceCNY:        strings.TrimSpace(req.PriceCNY),
		Points:          strings.TrimSpace(req.Points),
		BonusPoints:     strings.TrimSpace(req.BonusPoints),
		DurationDays:    normalizeDurationDays(req.DurationDays),
		Currency:        normalizeCurrency(req.Currency),
		SortOrder:       req.SortOrder,
		Description:     strings.TrimSpace(req.Description),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	s.nextPlanID++
	s.plans = append(s.plans, plan)
	return plan, nil
}

func (s *MemoryStore) UpdatePlan(_ context.Context, req domainbilling.UpdateSubscriptionPlanRequest) (domainbilling.SubscriptionPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, item := range s.plans {
		if item.ID != req.PlanID {
			continue
		}
		item.PlanName = strings.TrimSpace(req.PlanName)
		item.PlanType = normalizePlanType(req.PlanType)
		item.PurchaseEnabled = req.PurchaseEnabled
		item.Status = normalizePlanStatus(req.Status)
		item.PriceCNY = strings.TrimSpace(req.PriceCNY)
		item.Points = strings.TrimSpace(req.Points)
		item.BonusPoints = strings.TrimSpace(req.BonusPoints)
		item.DurationDays = normalizeDurationDays(req.DurationDays)
		item.Currency = normalizeCurrency(req.Currency)
		item.SortOrder = req.SortOrder
		item.Description = strings.TrimSpace(req.Description)
		item.UpdatedAt = time.Now().UTC()
		s.plans[index] = item
		return item, nil
	}
	return domainbilling.SubscriptionPlan{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "subscription plan not found")
}

func (s *MemoryStore) DeletePlan(_ context.Context, planID int64) (domainbilling.SubscriptionPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, item := range s.plans {
		if item.ID != planID {
			continue
		}
		item.Status = "archived"
		item.PurchaseEnabled = false
		item.UpdatedAt = time.Now().UTC()
		s.plans[index] = item
		return item, nil
	}
	return domainbilling.SubscriptionPlan{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "subscription plan not found")
}

func (s *MemoryStore) GetActiveSubscription(_ context.Context, _ int64) (*domainbilling.UserSubscriptionSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sub := range s.subs {
		if sub != nil {
			cloned := *sub
			return &cloned, nil
		}
	}
	return nil, nil
}

func (s *MemoryStore) ListOrders(_ context.Context, req domainbilling.ListOrdersRequest) (domainbilling.PaymentOrderPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	items := make([]domainbilling.PaymentOrder, 0, len(s.orders))
	for _, order := range s.orders {
		if req.UserID > 0 && order.UserID != req.UserID {
			continue
		}
		if strings.TrimSpace(req.Status) != "" && order.Status != strings.TrimSpace(req.Status) {
			continue
		}
		if strings.TrimSpace(req.OrderNo) != "" && !strings.Contains(strings.ToLower(order.OrderNo), strings.ToLower(strings.TrimSpace(req.OrderNo))) {
			continue
		}
		if strings.TrimSpace(req.VisibleMethod) != "" && strings.ToLower(order.VisibleMethod) != strings.ToLower(strings.TrimSpace(req.VisibleMethod)) {
			continue
		}
		if strings.TrimSpace(req.ProviderType) != "" && strings.ToLower(order.ProviderType) != strings.ToLower(strings.TrimSpace(req.ProviderType)) {
			continue
		}
		if strings.TrimSpace(req.PurchaseType) != "" && strings.ToLower(order.PurchaseType) != strings.ToLower(strings.TrimSpace(req.PurchaseType)) {
			continue
		}
		items = append(items, order)
	}
	total := len(items)
	start := (req.Page - 1) * req.PageSize
	if start > total {
		start = total
	}
	end := start + req.PageSize
	if end > total {
		end = total
	}
	return domainbilling.PaymentOrderPage{Items: items[start:end], Page: req.Page, PageSize: req.PageSize, Total: total}, nil
}

func (s *MemoryStore) ListWebhookEvents(_ context.Context, page, pageSize int) (domainbilling.PaymentWebhookEventPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	total := len(s.webhooks)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	items := append([]domainbilling.PaymentWebhookEvent{}, s.webhooks[start:end]...)
	return domainbilling.PaymentWebhookEventPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *MemoryStore) GetOrder(_ context.Context, userID int64, orderID int64) (domainbilling.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[orderID]
	if !ok || order.UserID != userID {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
	}
	return order, nil
}

func (s *MemoryStore) GetOrderByIdempotencyKey(_ context.Context, userID int64, idempotencyKey string) (domainbilling.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
	}
	for _, order := range s.orders {
		if order.UserID == userID && order.IdempotencyKey == idempotencyKey {
			return order, nil
		}
	}
	return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
}

func (s *MemoryStore) GetOrderByOrderNo(_ context.Context, orderNo string) (domainbilling.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	orderNo = strings.TrimSpace(orderNo)
	if orderNo == "" {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
	}
	for _, order := range s.orders {
		if order.OrderNo == orderNo {
			return order, nil
		}
	}
	return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
}

func (s *MemoryStore) GetOrderForAdmin(_ context.Context, orderID int64) (domainbilling.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[orderID]
	if !ok {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
	}
	return order, nil
}

func (s *MemoryStore) RecordChargebackSummary(_ context.Context, req ChargebackSummaryStoreRequest) (domainbilling.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[req.OrderID]
	if !ok {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
	}
	now := time.Now().UTC()
	order.ChargebackPoints = strings.TrimSpace(req.ChargePoints)
	order.ChargebackReason = strings.TrimSpace(req.Reason)
	order.ChargebackKey = strings.TrimSpace(req.IdempotencyKey)
	order.ChargebackAt = &now
	order.UpdatedAt = now
	s.orders[order.ID] = order
	return order, nil
}

func (s *MemoryStore) RetryWebhookEvent(_ context.Context, eventID int64) (domainbilling.PaymentWebhookEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, event := range s.webhooks {
		if event.ID != eventID {
			continue
		}
		if event.EventType == "refund.local_finalize_failed" {
			req, ok := s.refundRetries[event.ID]
			if !ok {
				return domainbilling.PaymentWebhookEvent{}, errs.New(http.StatusConflict, errs.CodeConflict, "refund compensation payload is missing")
			}
			if _, err := s.refundPaymentOrderLocked(req); err != nil {
				return domainbilling.PaymentWebhookEvent{}, err
			}
			now := time.Now().UTC()
			event.Status = "processed"
			event.ProcessedAt = &now
			event.FailureReason = ""
			s.webhooks[index] = event
			delete(s.refundRetries, event.ID)
			return event, nil
		}
		now := time.Now().UTC()
		event.Status = "processed"
		event.ProcessedAt = &now
		event.FailureReason = ""
		if order, ok := s.orders[event.OrderID]; ok {
			event.OrderNo = order.OrderNo
			event.ProviderType = order.Provider
			if event.ProviderType == "" {
				event.ProviderType = "mock"
			}
		}
		s.webhooks[index] = event
		return event, nil
	}
	return domainbilling.PaymentWebhookEvent{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment webhook event not found")
}

func (s *MemoryStore) ProcessRefundFinalizeFailures(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 5
	}
	eventIDs := make([]int64, 0, limit)
	s.mu.Lock()
	for _, event := range s.webhooks {
		if len(eventIDs) >= limit {
			break
		}
		if event.EventType == "refund.local_finalize_failed" && event.Status == "failed" {
			eventIDs = append(eventIDs, event.ID)
		}
	}
	s.mu.Unlock()
	processed := 0
	for _, eventID := range eventIDs {
		if _, err := s.RetryWebhookEvent(ctx, eventID); err == nil {
			processed++
		}
	}
	return processed, nil
}

func (s *MemoryStore) RecordRefundFinalizeFailure(_ context.Context, req RefundFinalizeFailureRequest) (domainbilling.PaymentWebhookEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[req.OrderID]
	if !ok || order.UserID != req.UserID {
		return domainbilling.PaymentWebhookEvent{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
	}
	now := time.Now().UTC()
	event := domainbilling.PaymentWebhookEvent{
		ID:              s.nextWebhookID,
		OrderID:         order.ID,
		OrderNo:         order.OrderNo,
		ProviderType:    order.Provider,
		Status:          "failed",
		EventType:       "refund.local_finalize_failed",
		FailureReason:   strings.TrimSpace(req.FailureReason),
		SignatureStatus: "failed",
		ResultSummary:   "处理失败，等待人工或自动重试",
		PayloadPreview: fmt.Sprintf(
			`{"order_id":%d,"order_no":"%s","refund_trade_no":"%s","failure_reason":"%s"}`,
			req.OrderID,
			order.OrderNo,
			strings.TrimSpace(req.RefundTradeNo),
			strings.TrimSpace(req.FailureReason),
		),
		ReceivedAt: now,
	}
	if event.ProviderType == "" {
		event.ProviderType = "mock"
	}
	s.nextWebhookID++
	s.webhooks = append([]domainbilling.PaymentWebhookEvent{event}, s.webhooks...)
	s.refundRetries[event.ID] = req.RefundPaymentOrderRequest
	return event, nil
}

func (s *MemoryStore) CreateOrder(_ context.Context, req domainbilling.CreateOrderRequest) (domainbilling.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if idempotencyKey != "" {
		for _, order := range s.orders {
			if order.UserID == req.UserID && order.IdempotencyKey == idempotencyKey {
				return order, nil
			}
		}
	}
	var plan domainbilling.SubscriptionPlan
	found := false
	for _, item := range s.plans {
		if item.PlanCode == strings.TrimSpace(req.PlanCode) && item.Status == "active" {
			plan = item
			found = true
			break
		}
	}
	if !found {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "subscription plan not found")
	}
	if plan.PlanType != "points_package" || !plan.PurchaseEnabled {
		return domainbilling.PaymentOrder{}, errs.BadRequest("subscription plan is not purchasable")
	}
	now := time.Now().UTC()
	orderNo := strings.TrimSpace(req.OrderNo)
	if orderNo == "" {
		orderNo = fmt.Sprintf("PGO-%06d", s.nextOrderID)
	}
	paymentURL := strings.TrimSpace(req.PaymentURL)
	order := domainbilling.PaymentOrder{
		ID:                 s.nextOrderID,
		OrderNo:            orderNo,
		UserID:             req.UserID,
		PlanID:             plan.ID,
		PlanCode:           plan.PlanCode,
		PlanName:           plan.PlanName,
		Provider:           strings.TrimSpace(req.Provider),
		PurchaseType:       defaultString(strings.TrimSpace(req.PurchaseType), "plan"),
		VisibleMethod:      strings.TrimSpace(req.VisibleMethod),
		ProviderType:       defaultString(strings.TrimSpace(req.ProviderType), strings.TrimSpace(req.Provider)),
		ProviderInstanceID: req.ProviderInstanceID,
		PaymentDisplay:     cloneMap(req.PaymentDisplay),
		IdempotencyKey:     idempotencyKey,
		Status:             "pending",
		Currency:           plan.Currency,
		AmountCNY:          plan.PriceCNY,
		Points:             plan.Points,
		BonusPoints:        plan.BonusPoints,
		PaymentURL:         paymentURL,
		QRCode:             strings.TrimSpace(req.QRCode),
		ClientToken:        strings.TrimSpace(req.ClientToken),
		ExpiresAt:          now.Add(15 * time.Minute),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	s.orders[order.ID] = order
	s.nextOrderID++
	return order, nil
}

func (s *MemoryStore) CreateCustomAmountOrder(_ context.Context, req domainbilling.CreateCustomAmountOrderRequest) (domainbilling.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if idempotencyKey != "" {
		for _, order := range s.orders {
			if order.UserID == req.UserID && order.IdempotencyKey == idempotencyKey {
				return order, nil
			}
		}
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(req.AmountCNY))
	if err != nil || !amount.IsPositive() {
		return domainbilling.PaymentOrder{}, errs.BadRequest("amount_cny must be positive")
	}
	cnyPerPoint, err := decimal.NewFromString(strings.TrimSpace(req.CNYPerPoint))
	if err != nil || !cnyPerPoint.IsPositive() {
		return domainbilling.PaymentOrder{}, errs.BadRequest("cny_per_point must be positive")
	}
	points := amount.Div(cnyPerPoint).Round(s.scale)
	now := time.Now().UTC()
	orderNo := strings.TrimSpace(req.OrderNo)
	if orderNo == "" {
		orderNo = fmt.Sprintf("PGO-%06d", s.nextOrderID)
	}
	paymentURL := strings.TrimSpace(req.PaymentURL)
	order := domainbilling.PaymentOrder{
		ID:                 s.nextOrderID,
		OrderNo:            orderNo,
		UserID:             req.UserID,
		PlanCode:           "custom_amount",
		PlanName:           "自定义充值",
		Provider:           strings.TrimSpace(req.Provider),
		PurchaseType:       defaultString(strings.TrimSpace(req.PurchaseType), "custom_amount"),
		VisibleMethod:      strings.TrimSpace(req.VisibleMethod),
		ProviderType:       defaultString(strings.TrimSpace(req.ProviderType), strings.TrimSpace(req.Provider)),
		ProviderInstanceID: req.ProviderInstanceID,
		PaymentDisplay:     cloneMap(req.PaymentDisplay),
		IdempotencyKey:     idempotencyKey,
		Status:             "pending",
		Currency:           "CNY",
		AmountCNY:          amount.Round(s.scale).StringFixed(s.scale),
		Points:             points.StringFixed(s.scale),
		BonusPoints:        decimal.Zero.StringFixed(s.scale),
		PaymentURL:         paymentURL,
		QRCode:             strings.TrimSpace(req.QRCode),
		ClientToken:        strings.TrimSpace(req.ClientToken),
		ExpiresAt:          now.Add(15 * time.Minute),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	s.orders[order.ID] = order
	s.nextOrderID++
	return order, nil
}

func (s *MemoryStore) CancelOrder(_ context.Context, userID int64, orderID int64) (domainbilling.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[orderID]
	if !ok || order.UserID != userID {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
	}
	if order.Status != "pending" {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order cannot be canceled")
	}
	now := time.Now().UTC()
	order.Status = "canceled"
	order.ClosedAt = &now
	order.UpdatedAt = now
	s.orders[order.ID] = order
	return order, nil
}

func (s *MemoryStore) MarkOrderPaid(_ context.Context, req domainbilling.MarkOrderPaidRequest) (domainbilling.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, order := range s.orders {
		if order.OrderNo != req.OrderNo {
			continue
		}
		if err := ensurePaymentAmountMatches(order.AmountCNY, req.AmountCNY, s.scale); err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		if isCashierRechargeOrder(order) {
			return s.completeRechargeOrderLocked(order, domainbilling.CompleteRechargeOrderRequest{
				UserID:   order.UserID,
				OrderID:  order.ID,
				Provider: req.Provider,
				TradeNo:  req.TradeNo,
			})
		}
		if order.Status == "paid" {
			return order, nil
		}
		now := time.Now().UTC()
		order.Status = "paid"
		order.TradeNo = req.TradeNo
		order.PaidAt = &now
		order.UpdatedAt = now
		s.orders[id] = order
		current := s.balances[order.UserID]
		points, _ := decimal.NewFromString(order.Points)
		bonus, _ := decimal.NewFromString(order.BonusPoints)
		current.Available = current.Available.Add(points).Add(bonus)
		s.balances[order.UserID] = current
		breakdown := s.breakdown[order.UserID]
		breakdown.Subscription = breakdown.Subscription.Add(points)
		breakdown.Gift = breakdown.Gift.Add(bonus)
		s.breakdown[order.UserID] = breakdown
		periodEnd := now.Add(30 * 24 * time.Hour)
		s.subs[order.UserID] = &domainbilling.UserSubscriptionSummary{
			ID:                 order.ID,
			PlanID:             order.PlanID,
			PlanCode:           order.PlanCode,
			PlanName:           order.PlanName,
			Status:             "active",
			StartedAt:          now,
			CurrentPeriodStart: now,
			CurrentPeriodEnd:   periodEnd,
			GrantedPoints:      order.Points,
			RemainingPoints:    order.Points,
		}
		s.appendLedger(order.UserID, 0, "", "order_paid", points.Add(bonus), current, "payment order "+order.OrderNo)
		return order, nil
	}
	return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
}

func ensurePaymentAmountMatches(orderAmountCNY, callbackAmountCNY string, scale int32) error {
	callbackAmountCNY = strings.TrimSpace(callbackAmountCNY)
	if callbackAmountCNY == "" {
		return nil
	}
	orderAmount, orderErr := decimal.NewFromString(strings.TrimSpace(orderAmountCNY))
	callbackAmount, callbackErr := decimal.NewFromString(callbackAmountCNY)
	if orderErr != nil || callbackErr != nil || !orderAmount.Round(scale).Equal(callbackAmount.Round(scale)) {
		return errs.New(http.StatusConflict, errs.CodePaymentAmountMismatch, "payment amount does not match order")
	}
	return nil
}

type memoryPaymentOrderRefundPlan struct {
	RefundAmountCNY       decimal.Decimal
	RefundPoints          decimal.Decimal
	NextRefundedAmountCNY decimal.Decimal
	NextRefundedPoints    decimal.Decimal
	FullyRefunded         bool
}

func (s *MemoryStore) memoryPaymentOrderRefundPlan(order domainbilling.PaymentOrder, req domainbilling.RefundPaymentOrderRequest) (memoryPaymentOrderRefundPlan, error) {
	totalAmountCNY, err := decimal.NewFromString(strings.TrimSpace(order.AmountCNY))
	if err != nil || !totalAmountCNY.IsPositive() {
		return memoryPaymentOrderRefundPlan{}, errs.Internal("payment order amount is invalid")
	}
	points, pointsErr := decimal.NewFromString(strings.TrimSpace(order.Points))
	bonus, bonusErr := decimal.NewFromString(strings.TrimSpace(order.BonusPoints))
	totalPoints := points.Add(bonus).Round(s.scale)
	if pointsErr != nil || bonusErr != nil || !totalPoints.IsPositive() {
		return memoryPaymentOrderRefundPlan{}, errs.Internal("payment order points are invalid")
	}
	refundedAmountCNY := decimal.Zero
	if strings.TrimSpace(order.RefundedAmountCNY) != "" {
		if parsed, parseErr := decimal.NewFromString(strings.TrimSpace(order.RefundedAmountCNY)); parseErr == nil {
			refundedAmountCNY = parsed.Round(s.scale)
		}
	}
	refundedPoints := decimal.Zero
	if strings.TrimSpace(order.RefundedPoints) != "" {
		if parsed, parseErr := decimal.NewFromString(strings.TrimSpace(order.RefundedPoints)); parseErr == nil {
			refundedPoints = parsed.Round(s.scale)
		}
	}
	remainingAmountCNY := totalAmountCNY.Sub(refundedAmountCNY).Round(s.scale)
	remainingPoints := totalPoints.Sub(refundedPoints).Round(s.scale)
	if !remainingAmountCNY.IsPositive() || !remainingPoints.IsPositive() {
		return memoryPaymentOrderRefundPlan{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order has no refundable amount")
	}
	refundAmountCNY := remainingAmountCNY
	if strings.TrimSpace(req.RefundAmountCNY) != "" {
		parsed, parseErr := decimal.NewFromString(strings.TrimSpace(req.RefundAmountCNY))
		if parseErr != nil || !parsed.IsPositive() {
			return memoryPaymentOrderRefundPlan{}, errs.BadRequest("refund_amount_cny must be positive")
		}
		refundAmountCNY = parsed.Round(s.scale)
	}
	if refundAmountCNY.GreaterThan(remainingAmountCNY) {
		return memoryPaymentOrderRefundPlan{}, errs.New(http.StatusConflict, errs.CodeConflict, "refund amount exceeds refundable amount")
	}
	refundPoints := refundAmountCNY.Mul(totalPoints).Div(totalAmountCNY).Round(s.scale)
	if refundAmountCNY.Equal(remainingAmountCNY) {
		refundPoints = remainingPoints
	}
	if !refundPoints.IsPositive() || refundPoints.GreaterThan(remainingPoints) {
		return memoryPaymentOrderRefundPlan{}, errs.New(http.StatusConflict, errs.CodeConflict, "refund points exceed refundable balance")
	}
	nextRefundedAmountCNY := refundedAmountCNY.Add(refundAmountCNY).Round(s.scale)
	nextRefundedPoints := refundedPoints.Add(refundPoints).Round(s.scale)
	fullyRefunded := !totalAmountCNY.Sub(nextRefundedAmountCNY).Round(s.scale).IsPositive() || !totalPoints.Sub(nextRefundedPoints).Round(s.scale).IsPositive()
	return memoryPaymentOrderRefundPlan{
		RefundAmountCNY:       refundAmountCNY,
		RefundPoints:          refundPoints,
		NextRefundedAmountCNY: nextRefundedAmountCNY,
		NextRefundedPoints:    nextRefundedPoints,
		FullyRefunded:         fullyRefunded,
	}, nil
}

func (s *MemoryStore) memoryRefundRecordExists(orderID int64, refundTradeNo string) bool {
	refundTradeNo = strings.TrimSpace(refundTradeNo)
	if refundTradeNo == "" {
		return false
	}
	trades := s.refundTrades[orderID]
	_, ok := trades[refundTradeNo]
	return ok
}

func (s *MemoryStore) markMemoryRefundRecord(orderID int64, refundTradeNo string) {
	refundTradeNo = strings.TrimSpace(refundTradeNo)
	if refundTradeNo == "" {
		return
	}
	if s.refundTrades[orderID] == nil {
		s.refundTrades[orderID] = map[string]struct{}{}
	}
	s.refundTrades[orderID][refundTradeNo] = struct{}{}
}

func (s *MemoryStore) CompleteRechargeOrder(_ context.Context, req domainbilling.CompleteRechargeOrderRequest) (domainbilling.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[req.OrderID]
	if !ok || order.UserID != req.UserID {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
	}
	return s.completeRechargeOrderLocked(order, req)
}

func (s *MemoryStore) RefundPaymentOrder(_ context.Context, req domainbilling.RefundPaymentOrderRequest) (domainbilling.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refundPaymentOrderLocked(req)
}

func (s *MemoryStore) refundPaymentOrderLocked(req domainbilling.RefundPaymentOrderRequest) (domainbilling.PaymentOrder, error) {
	order, ok := s.orders[req.OrderID]
	if !ok || order.UserID != req.UserID {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
	}
	refundTradeNo := strings.TrimSpace(req.RefundTradeNo)
	if refundTradeNo == "" {
		return domainbilling.PaymentOrder{}, errs.BadRequest("refund_trade_no is required")
	}
	if s.memoryRefundRecordExists(order.ID, refundTradeNo) || order.RefundTradeNo == refundTradeNo {
		return order, nil
	}
	if order.Status == "refunded" {
		return order, nil
	}
	if order.Status != "completed" && order.Status != "partially_refunded" {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order cannot transition to refunded")
	}
	plan, err := s.memoryPaymentOrderRefundPlan(order, req)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	current := s.balances[order.UserID]
	breakdown := s.breakdown[order.UserID]
	frozenRefund := s.refundFreezes[order.ID]
	if frozenRefund.GreaterThanOrEqual(plan.RefundPoints) {
		if current.Frozen.LessThan(plan.RefundPoints) {
			return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order recharge balance is insufficient for refund")
		}
		current.Frozen = current.Frozen.Sub(plan.RefundPoints)
		delete(s.refundFreezes, order.ID)
	} else {
		if current.Available.LessThan(plan.RefundPoints) || breakdown.Recharge.LessThan(plan.RefundPoints) {
			return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order recharge balance is insufficient for refund")
		}
		current.Available = current.Available.Sub(plan.RefundPoints)
		breakdown.Recharge = breakdown.Recharge.Sub(plan.RefundPoints)
		s.breakdown[order.UserID] = breakdown
	}
	if breakdown.Recharge.IsNegative() {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order recharge balance is insufficient for refund")
	}
	s.balances[order.UserID] = current
	now := time.Now().UTC()
	order.Status = "partially_refunded"
	if plan.FullyRefunded {
		order.Status = "refunded"
		order.RefundedAt = &now
	}
	order.RefundTradeNo = refundTradeNo
	order.RefundedAmountCNY = plan.NextRefundedAmountCNY.StringFixed(s.scale)
	order.RefundedPoints = plan.NextRefundedPoints.StringFixed(s.scale)
	order.UpdatedAt = now
	s.appendLedger(order.UserID, 0, "", "payment_refund", plan.RefundPoints.Neg(), current, strings.TrimSpace(req.Reason))
	s.orders[order.ID] = order
	s.markMemoryRefundRecord(order.ID, refundTradeNo)
	return order, nil
}

func (s *MemoryStore) FreezeRefundPaymentOrder(_ context.Context, req domainbilling.RefundPaymentOrderRequest) (domainbilling.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[req.OrderID]
	if !ok || order.UserID != req.UserID {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
	}
	if order.Status == "refunded" {
		return order, nil
	}
	if order.Status != "completed" && order.Status != "partially_refunded" {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order cannot transition to refunded")
	}
	refundTradeNo := strings.TrimSpace(req.RefundTradeNo)
	if refundTradeNo == "" {
		return domainbilling.PaymentOrder{}, errs.BadRequest("refund_trade_no is required")
	}
	if s.memoryRefundRecordExists(order.ID, refundTradeNo) || order.RefundTradeNo == refundTradeNo {
		return order, nil
	}
	plan, err := s.memoryPaymentOrderRefundPlan(order, req)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	if frozen := s.refundFreezes[order.ID]; frozen.GreaterThanOrEqual(plan.RefundPoints) {
		return order, nil
	}
	current := s.balances[order.UserID]
	breakdown := s.breakdown[order.UserID]
	if current.Available.LessThan(plan.RefundPoints) || breakdown.Recharge.LessThan(plan.RefundPoints) {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order recharge balance is insufficient for refund")
	}
	current.Available = current.Available.Sub(plan.RefundPoints)
	current.Frozen = current.Frozen.Add(plan.RefundPoints)
	breakdown.Recharge = breakdown.Recharge.Sub(plan.RefundPoints)
	s.balances[order.UserID] = current
	s.breakdown[order.UserID] = breakdown
	s.refundFreezes[order.ID] = plan.RefundPoints
	return order, nil
}

func (s *MemoryStore) ReleaseRefundPaymentOrder(_ context.Context, req domainbilling.RefundPaymentOrderRequest) (domainbilling.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[req.OrderID]
	if !ok || order.UserID != req.UserID {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
	}
	frozen := s.refundFreezes[order.ID]
	if !frozen.IsPositive() {
		return order, nil
	}
	current := s.balances[order.UserID]
	breakdown := s.breakdown[order.UserID]
	if current.Frozen.LessThan(frozen) {
		frozen = current.Frozen
	}
	current.Frozen = current.Frozen.Sub(frozen)
	current.Available = current.Available.Add(frozen)
	breakdown.Recharge = breakdown.Recharge.Add(frozen)
	s.balances[order.UserID] = current
	s.breakdown[order.UserID] = breakdown
	delete(s.refundFreezes, order.ID)
	return order, nil
}

func (s *MemoryStore) CheckRefundPaymentOrder(_ context.Context, req domainbilling.RefundPaymentOrderRequest) (domainbilling.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[req.OrderID]
	if !ok || order.UserID != req.UserID {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
	}
	if order.Status == "refunded" {
		return order, nil
	}
	if order.Status != "completed" && order.Status != "partially_refunded" {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order cannot transition to refunded")
	}
	refundTradeNo := strings.TrimSpace(req.RefundTradeNo)
	if s.memoryRefundRecordExists(order.ID, refundTradeNo) || order.RefundTradeNo == refundTradeNo {
		return order, nil
	}
	plan, err := s.memoryPaymentOrderRefundPlan(order, req)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	current := s.balances[order.UserID]
	breakdown := s.breakdown[order.UserID]
	frozenRefund := s.refundFreezes[order.ID]
	if frozenRefund.IsZero() {
		if current.Available.LessThan(plan.RefundPoints) || breakdown.Recharge.LessThan(plan.RefundPoints) {
			return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order recharge balance is insufficient for refund")
		}
		return order, nil
	}
	if frozenRefund.LessThan(plan.RefundPoints) || current.Frozen.LessThan(plan.RefundPoints) {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order recharge balance is insufficient for refund")
	}
	return order, nil
}

func (s *MemoryStore) completeRechargeOrderLocked(order domainbilling.PaymentOrder, req domainbilling.CompleteRechargeOrderRequest) (domainbilling.PaymentOrder, error) {
	if order.Status == "completed" {
		return order, nil
	}
	if order.Status != "pending" {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order cannot transition to completed")
	}
	now := time.Now().UTC()
	order.Status = "completed"
	order.Provider = strings.ToLower(strings.TrimSpace(req.Provider))
	order.TradeNo = strings.TrimSpace(req.TradeNo)
	order.PaidAt = &now
	order.CompletedAt = &now
	order.UpdatedAt = now
	s.appendWebhookEvent(order, now)
	points, _ := decimal.NewFromString(order.Points)
	bonus, _ := decimal.NewFromString(order.BonusPoints)
	total := points.Add(bonus)
	current := s.balances[order.UserID]
	current.Available = current.Available.Add(total)
	s.balances[order.UserID] = current
	breakdown := s.breakdown[order.UserID]
	breakdown.Recharge = breakdown.Recharge.Add(total)
	s.breakdown[order.UserID] = breakdown
	order.LedgerID = s.appendLedger(order.UserID, 0, "", "recharge", total, current, "cashier order "+order.OrderNo)
	s.orders[order.ID] = order
	return order, nil
}

func isCashierRechargeOrder(order domainbilling.PaymentOrder) bool {
	return strings.TrimSpace(order.VisibleMethod) != "" || strings.TrimSpace(order.PurchaseType) == "custom_amount" || order.ProviderInstanceID > 0 || len(order.PaymentDisplay) > 0
}

func (s *MemoryStore) appendWebhookEvent(order domainbilling.PaymentOrder, now time.Time) {
	event := domainbilling.PaymentWebhookEvent{
		ID:              s.nextWebhookID,
		OrderID:         order.ID,
		OrderNo:         order.OrderNo,
		ProviderType:    order.Provider,
		Status:          "processed",
		EventType:       "payment.succeeded",
		SignatureStatus: "verified",
		ResultSummary:   "已完成本地处理",
		PayloadPreview:  fmt.Sprintf(`{"order_no":"%s"}`, order.OrderNo),
		ReceivedAt:      now,
		ProcessedAt:     &now,
	}
	s.nextWebhookID++
	s.webhooks = append([]domainbilling.PaymentWebhookEvent{event}, s.webhooks...)
}

func (s *MemoryStore) ReserveTask(_ context.Context, req ReserveStoreRequest) (BalanceState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.taskState[req.TaskID]
	if state.Seen && state.UserID != req.UserID {
		return BalanceState{}, errs.New(409, errs.CodeConflict, "image task points belong to a different user")
	}
	if state.Active {
		return s.formatState(req.UserID, s.balances[req.UserID]), nil
	}
	if state.Seen {
		state.Cycle++
	} else {
		state.Seen = true
		state.UserID = req.UserID
		state.APIKeyID = req.APIKeyID
	}
	estimated, err := decimal.NewFromString(req.EstimatedPoints)
	if err != nil {
		return BalanceState{}, err
	}
	if estimated.IsNegative() {
		return BalanceState{}, errs.BadRequest("estimated points must be non-negative")
	}
	current := s.balances[req.UserID]
	if current.Available.LessThan(estimated) {
		return BalanceState{}, errs.New(400, errs.CodeInsufficientPoints, "insufficient points")
	}
	if err := s.checkAPIKeyQuotaLocked(req, estimated); err != nil {
		return BalanceState{}, err
	}
	current.Available = current.Available.Sub(estimated)
	current.Frozen = current.Frozen.Add(estimated)
	s.balances[req.UserID] = current
	s.appendLedger(req.UserID, req.APIKeyID, req.TaskID, "reserve", estimated.Neg(), current, req.Reason)
	s.addAPIKeyUsage(req.APIKeyID, estimated, time.Now().UTC())
	state.Active = true
	state.Reserved = estimated
	s.taskState[req.TaskID] = state
	return s.formatState(req.UserID, current), nil
}

func (s *MemoryStore) FinalizeTask(_ context.Context, req FinalizeStoreRequest) (BalanceState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	estimated, err := decimal.NewFromString(req.EstimatedPoints)
	if err != nil {
		return BalanceState{}, err
	}
	if estimated.IsNegative() {
		return BalanceState{}, errs.BadRequest("estimated points must be non-negative")
	}
	state := s.taskState[req.TaskID]
	if state.Seen && state.UserID != req.UserID {
		return BalanceState{}, errs.New(409, errs.CodeConflict, "image task points belong to a different user")
	}
	if !state.Active {
		if !state.Seen {
			return BalanceState{}, errs.New(409, errs.CodeConflict, "image task points were not reserved")
		}
		return s.formatState(req.UserID, s.balances[req.UserID]), nil
	}
	actual, err := decimal.NewFromString(req.ActualPoints)
	if err != nil {
		return BalanceState{}, err
	}
	current := s.balances[req.UserID]
	reserved := state.Reserved
	if actual.IsNegative() {
		actual = decimal.Zero
	}
	if actual.GreaterThan(reserved) {
		actual = reserved
	}
	if actual.IsZero() {
		current.Available = current.Available.Add(reserved)
		current.Frozen = current.Frozen.Sub(reserved)
		apiKeyID := firstPositive(req.APIKeyID, state.APIKeyID)
		s.appendLedger(req.UserID, apiKeyID, req.TaskID, "refund", reserved, current, req.Reason)
		s.addAPIKeyUsage(apiKeyID, reserved.Neg(), time.Now().UTC())
		s.balances[req.UserID] = current
		state.Active = false
		state.Reserved = decimal.Zero
		s.taskState[req.TaskID] = state
		return s.formatState(req.UserID, current), nil
	}
	current.Frozen = current.Frozen.Sub(actual)
	apiKeyID := firstPositive(req.APIKeyID, state.APIKeyID)
	s.appendLedger(req.UserID, apiKeyID, req.TaskID, "consume", actual.Neg(), current, req.Reason)
	diff := reserved.Sub(actual)
	if diff.GreaterThan(decimal.Zero) {
		current.Available = current.Available.Add(diff)
		current.Frozen = current.Frozen.Sub(diff)
		s.appendLedger(req.UserID, apiKeyID, req.TaskID, "refund", diff, current, req.Reason)
		s.addAPIKeyUsage(apiKeyID, diff.Neg(), time.Now().UTC())
	}
	s.balances[req.UserID] = current
	state.Active = false
	state.Reserved = decimal.Zero
	s.taskState[req.TaskID] = state
	return s.formatState(req.UserID, current), nil
}

func (s *MemoryStore) Adjust(_ context.Context, req AdjustStoreRequest) (BalanceState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	change, err := decimal.NewFromString(req.ChangePoints)
	if err != nil {
		return BalanceState{}, err
	}
	if idempotencyKey != "" {
		if existing, ok := s.adjustKeys[idempotencyKey]; ok {
			if existing.UserID != req.UserID {
				return BalanceState{}, errs.New(409, errs.CodeConflict, "idempotency key belongs to a different user")
			}
			if existing.ChangePoints != change.Round(s.scale).StringFixed(s.scale) || strings.TrimSpace(existing.Reason) != strings.TrimSpace(req.Reason) || existing.OperatorAdminID != req.OperatorAdminID {
				return BalanceState{}, errs.New(409, errs.CodeConflict, "idempotency key was already used with a different adjustment")
			}
			return s.formatState(req.UserID, s.balances[req.UserID]), nil
		}
	}
	current := s.balances[req.UserID]
	next := current.Available.Add(change)
	if next.IsNegative() {
		return BalanceState{}, errs.New(400, errs.CodeInsufficientPoints, "insufficient points")
	}
	current.Available = next
	s.balances[req.UserID] = current
	breakdown := s.breakdown[req.UserID]
	if change.IsPositive() {
		breakdown.Gift = breakdown.Gift.Add(change)
	} else if change.IsNegative() {
		breakdown = deductMemoryBreakdownForAdminAdjustment(breakdown, change.Abs())
	}
	s.breakdown[req.UserID] = breakdown
	s.appendLedger(req.UserID, 0, "", "admin_adjust", change, current, req.Reason)
	if idempotencyKey != "" {
		stored := req
		stored.ChangePoints = change.Round(s.scale).StringFixed(s.scale)
		stored.Reason = strings.TrimSpace(req.Reason)
		stored.IdempotencyKey = idempotencyKey
		s.adjustKeys[idempotencyKey] = stored
	}
	return s.formatState(req.UserID, current), nil
}

func deductMemoryBreakdownForAdminAdjustment(breakdown memoryBreakdown, amount decimal.Decimal) memoryBreakdown {
	remaining := amount
	deduct := func(bucket decimal.Decimal) decimal.Decimal {
		if !remaining.IsPositive() || !bucket.IsPositive() {
			return bucket
		}
		if bucket.GreaterThanOrEqual(remaining) {
			bucket = bucket.Sub(remaining)
			remaining = decimal.Zero
			return bucket
		}
		remaining = remaining.Sub(bucket)
		return decimal.Zero
	}
	breakdown.Recharge = deduct(breakdown.Recharge)
	breakdown.Gift = deduct(breakdown.Gift)
	breakdown.Subscription = deduct(breakdown.Subscription)
	breakdown.Trial = deduct(breakdown.Trial)
	return breakdown
}

func (s *MemoryStore) EnsureSignupTrialGrant(_ context.Context, req SignupTrialGrantStoreRequest) (SignupTrialGrantStoreResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if req.UserID <= 0 {
		return SignupTrialGrantStoreResult{}, errs.BadRequest("user id is required")
	}
	amount, err := decimal.NewFromString(req.Points)
	if err != nil {
		return SignupTrialGrantStoreResult{}, err
	}
	if !amount.IsPositive() {
		return SignupTrialGrantStoreResult{Balance: s.formatState(req.UserID, s.balances[req.UserID])}, nil
	}
	key := strings.TrimSpace(req.IdempotencyKey)
	if key == "" {
		key = signupTrialLedgerKey(req.UserID)
	}
	if existing, ok := s.trialGrants[key]; ok {
		if existing.UserID != req.UserID {
			return SignupTrialGrantStoreResult{}, errs.New(409, errs.CodeConflict, "idempotency key belongs to a different user")
		}
		return SignupTrialGrantStoreResult{Balance: s.formatState(req.UserID, s.balances[req.UserID])}, nil
	}
	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(req.ValidDays) * 24 * time.Hour)
	current := s.balances[req.UserID]
	current.Available = current.Available.Add(amount)
	s.balances[req.UserID] = current
	breakdown := s.breakdown[req.UserID]
	breakdown.Trial = breakdown.Trial.Add(amount)
	breakdown.Gift = breakdown.Gift.Add(amount)
	breakdown.TrialExpires = &expiresAt
	breakdown.TrialReminderDays = req.ExpiryReminderDays
	s.breakdown[req.UserID] = breakdown
	s.appendLedger(req.UserID, 0, "", "trial_grant", amount, current, "signup trial grant")
	s.trialGrants[key] = req
	return SignupTrialGrantStoreResult{Granted: true, Balance: s.formatState(req.UserID, current)}, nil
}

func (s *MemoryStore) RedeemCode(_ context.Context, req RedeemCodeRequest) (BalanceState, error) {
	if req.UserID <= 0 || req.Code == "" || req.IdempotencyKey == "" {
		return BalanceState{}, errs.BadRequest("user id, code, and Idempotency-Key are required")
	}
	return BalanceState{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "redeem code not found")
}

func (s *MemoryStore) checkAPIKeyQuotaLocked(req ReserveStoreRequest, estimated decimal.Decimal) error {
	if req.APIKeyID <= 0 {
		return nil
	}
	if req.APIKeyTotalQuotaPoints != nil {
		limit, err := decimal.NewFromString(strings.TrimSpace(*req.APIKeyTotalQuotaPoints))
		if err != nil {
			return errs.Internal("invalid api key total quota")
		}
		used := s.apiKeyUsage[req.APIKeyID][""]
		if used.IsNegative() {
			used = decimal.Zero
		}
		if used.Add(estimated).GreaterThan(limit) {
			return errs.New(http.StatusTooManyRequests, errs.CodeRateLimited, "api key total quota exceeded")
		}
	}
	if req.APIKeyDailyQuotaPoints != nil {
		limit, err := decimal.NewFromString(strings.TrimSpace(*req.APIKeyDailyQuotaPoints))
		if err != nil {
			return errs.Internal("invalid api key daily quota")
		}
		dayStart := time.Now()
		if req.APIKeyQuotaDayStart != nil {
			dayStart = *req.APIKeyQuotaDayStart
		}
		used := s.apiKeyUsage[req.APIKeyID][dayKey(dayStart)]
		if used.IsNegative() {
			used = decimal.Zero
		}
		if used.Add(estimated).GreaterThan(limit) {
			return errs.New(http.StatusTooManyRequests, errs.CodeRateLimited, "api key daily quota exceeded")
		}
	}
	return nil
}

func (s *MemoryStore) APIKeyUsage(_ context.Context, apiKeyID int64, since *time.Time) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if apiKeyID <= 0 {
		return decimal.Zero.StringFixed(s.scale), nil
	}
	if since == nil {
		value := s.apiKeyUsage[apiKeyID][""]
		if value.IsNegative() {
			value = decimal.Zero
		}
		return value.Round(s.scale).StringFixed(s.scale), nil
	}
	value := s.apiKeyUsage[apiKeyID][dayKey(*since)]
	if value.IsNegative() {
		value = decimal.Zero
	}
	return value.Round(s.scale).StringFixed(s.scale), nil
}

func (s *MemoryStore) appendLedger(userID, apiKeyID int64, taskID, ledgerType string, change decimal.Decimal, current balanceState, reason string) int64 {
	entry := domainbilling.LedgerEntry{
		ID:           s.nextID,
		UserID:       userID,
		APIKeyID:     apiKeyID,
		TaskID:       taskID,
		LedgerType:   ledgerType,
		ChangePoints: change.Round(s.scale).StringFixed(s.scale),
		BalanceAfter: current.Available.Round(s.scale).StringFixed(s.scale),
		FrozenAfter:  current.Frozen.Round(s.scale).StringFixed(s.scale),
		Reason:       reason,
		CreatedAt:    time.Now().UTC(),
	}
	entry = domainbilling.PopulateLedgerDisplayFields(entry)
	s.nextID++
	s.ledgers[userID] = append([]domainbilling.LedgerEntry{entry}, s.ledgers[userID]...)
	return entry.ID
}

func (s *MemoryStore) addAPIKeyUsage(apiKeyID int64, change decimal.Decimal, at time.Time) {
	if apiKeyID <= 0 || change.IsZero() {
		return
	}
	if s.apiKeyUsage[apiKeyID] == nil {
		s.apiKeyUsage[apiKeyID] = map[string]decimal.Decimal{}
	}
	s.apiKeyUsage[apiKeyID][""] = s.apiKeyUsage[apiKeyID][""].Add(change)
	key := dayKey(at)
	s.apiKeyUsage[apiKeyID][key] = s.apiKeyUsage[apiKeyID][key].Add(change)
}

func dayKey(at time.Time) string {
	return at.Local().Format("2006-01-02")
}

func firstPositive(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func (s *MemoryStore) formatState(userID int64, current balanceState) BalanceState {
	breakdown := s.breakdown[userID]
	buckets := make([]domainbilling.BalanceBucket, 0, 3)
	if breakdown.Trial.IsPositive() {
		buckets = append(buckets, domainbilling.BalanceBucket{
			Bucket:          "trial",
			Label:           "体验额度",
			AvailablePoints: breakdown.Trial.Round(s.scale).StringFixed(s.scale),
			FrozenPoints:    decimal.Zero.Round(s.scale).StringFixed(s.scale),
			ExpiresAt:       breakdown.TrialExpires,
			ExpireWarning:   memoryExpireWarning(time.Now().UTC(), breakdown.TrialExpires, breakdown.TrialReminderDays),
			SourceType:      "signup",
			SortOrder:       1,
		})
	}
	if breakdown.Subscription.IsPositive() {
		buckets = append(buckets, domainbilling.BalanceBucket{
			Bucket:          "subscription",
			Label:           "订阅额度",
			AvailablePoints: breakdown.Subscription.Round(s.scale).StringFixed(s.scale),
			FrozenPoints:    decimal.Zero.Round(s.scale).StringFixed(s.scale),
			SortOrder:       2,
		})
	}
	if breakdown.Recharge.IsPositive() {
		buckets = append(buckets, domainbilling.BalanceBucket{
			Bucket:          "recharge",
			Label:           "充值额度",
			AvailablePoints: breakdown.Recharge.Round(s.scale).StringFixed(s.scale),
			FrozenPoints:    decimal.Zero.Round(s.scale).StringFixed(s.scale),
			SortOrder:       4,
		})
	}
	return BalanceState{
		AvailablePoints:    current.Available.Round(s.scale).StringFixed(s.scale),
		FrozenPoints:       current.Frozen.Round(s.scale).StringFixed(s.scale),
		TrialPoints:        breakdown.Trial.Round(s.scale).StringFixed(s.scale),
		SubscriptionPoints: breakdown.Subscription.Round(s.scale).StringFixed(s.scale),
		GiftPoints:         breakdown.Gift.Round(s.scale).StringFixed(s.scale),
		RechargePoints:     breakdown.Recharge.Round(s.scale).StringFixed(s.scale),
		Buckets:            buckets,
		ActiveSubscription: s.subs[userID],
	}
}

func memoryExpireWarning(now time.Time, expiresAt *time.Time, reminderDays int) bool {
	if expiresAt == nil || now.After(*expiresAt) {
		return false
	}
	if reminderDays <= 0 {
		reminderDays = 2
	}
	return expiresAt.Sub(now) <= time.Duration(reminderDays)*24*time.Hour
}

func signupTrialLedgerKey(userID int64) string {
	return fmt.Sprintf("signup_trial:%d", userID)
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func defaultPlans() []domainbilling.SubscriptionPlan {
	now := time.Now().UTC()
	return []domainbilling.SubscriptionPlan{
		{ID: 1, PlanCode: "basic-monthly", PlanName: "Basic Monthly", PlanType: "points_package", PurchaseEnabled: true, Status: "active", PriceCNY: "19.90000", Points: "100.00000", BonusPoints: "0.00000", DurationDays: 30, Currency: "CNY", SortOrder: 1, CreatedAt: now, UpdatedAt: now},
		{ID: 2, PlanCode: "plus-monthly", PlanName: "Plus Monthly", PlanType: "points_package", PurchaseEnabled: true, Status: "active", PriceCNY: "49.90000", Points: "300.00000", BonusPoints: "30.00000", DurationDays: 30, Currency: "CNY", SortOrder: 2, CreatedAt: now, UpdatedAt: now},
	}
}

func normalizePlanType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "subscription" {
		return value
	}
	return "points_package"
}

func normalizePlanStatus(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "disabled" || value == "archived" {
		return value
	}
	return "active"
}

func normalizeDurationDays(value int) int {
	if value > 0 {
		return value
	}
	return 30
}

func normalizeCurrency(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return "CNY"
	}
	return value
}
