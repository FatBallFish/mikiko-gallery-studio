package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
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

type ProviderRefundStatusRequest struct {
	UserID              int64
	OrderID             int64
	RefundTradeNo       string
	RefundAmountCNY     string
	ChannelRefundNo     string
	ChannelRefundStatus string
	Reason              string
	OperatorAdminID     int64
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
	ListPlans(ctx context.Context, req domainbilling.SubscriptionPlanListRequest) ([]domainbilling.SubscriptionPlan, error)
	CreatePlan(ctx context.Context, req domainbilling.CreateSubscriptionPlanRequest) (domainbilling.SubscriptionPlan, error)
	UpdatePlan(ctx context.Context, req domainbilling.UpdateSubscriptionPlanRequest) (domainbilling.SubscriptionPlan, error)
	TransitionPlan(ctx context.Context, req domainbilling.TransitionSubscriptionPlanRequest) (domainbilling.SubscriptionPlan, error)
	DeletePlan(ctx context.Context, planID int64) (domainbilling.SubscriptionPlan, error)
	GetActiveSubscription(ctx context.Context, userID int64) (*domainbilling.UserSubscriptionSummary, error)
	ListOrders(ctx context.Context, req domainbilling.ListOrdersRequest) (domainbilling.PaymentOrderPage, error)
	ListWebhookEvents(ctx context.Context, page, pageSize int) (domainbilling.PaymentWebhookEventPage, error)
	GetOrder(ctx context.Context, userID int64, orderID int64) (domainbilling.PaymentOrder, error)
	GetOrderByIdempotencyKey(ctx context.Context, userID int64, idempotencyKey string) (domainbilling.PaymentOrder, error)
	GetOrderForAdmin(ctx context.Context, orderID int64) (domainbilling.PaymentOrder, error)
	RecordChargebackSummary(ctx context.Context, req ChargebackSummaryStoreRequest) (domainbilling.PaymentOrder, error)
	RetryWebhookEvent(ctx context.Context, eventID int64) (domainbilling.PaymentWebhookEvent, error)
	ProcessRefundFinalizeFailures(ctx context.Context, limit int) (int, error)
	CreateOrder(ctx context.Context, req domainbilling.CreateOrderRequest) (domainbilling.PaymentOrder, error)
	CreateCustomAmountOrder(ctx context.Context, req domainbilling.CreateCustomAmountOrderRequest) (domainbilling.PaymentOrder, error)
	InitializePaymentOrder(ctx context.Context, req domainbilling.InitializePaymentOrderRequest) (domainbilling.PaymentOrder, error)
	FailPaymentOrderInitialization(ctx context.Context, req domainbilling.FailPaymentOrderInitializationRequest) (domainbilling.PaymentOrder, error)
	CancelOrder(ctx context.Context, userID int64, orderID int64) (domainbilling.PaymentOrder, error)
	MarkOrderPaid(ctx context.Context, req domainbilling.MarkOrderPaidRequest) (domainbilling.PaymentOrder, error)
	CompleteRechargeOrder(ctx context.Context, req domainbilling.CompleteRechargeOrderRequest) (domainbilling.PaymentOrder, error)
	CheckRefundPaymentOrder(ctx context.Context, req domainbilling.RefundPaymentOrderRequest) (domainbilling.PaymentOrder, error)
	FreezeRefundPaymentOrder(ctx context.Context, req domainbilling.RefundPaymentOrderRequest) (domainbilling.PaymentOrder, error)
	ReleaseRefundPaymentOrder(ctx context.Context, req domainbilling.RefundPaymentOrderRequest) (domainbilling.PaymentOrder, error)
	RecordProviderRefundStatus(ctx context.Context, req ProviderRefundStatusRequest) (domainbilling.PaymentOrder, error)
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
	UserID            int64
	APIKeyID          int64
	Seen              bool
	Active            bool
	Cycle             int
	Reserved          decimal.Decimal
	GrantAllocations  []memoryGrantAllocation
	UntrackedReserved decimal.Decimal
}

type memoryWalletGrant struct {
	ID        int64
	UserID    int64
	OrderID   int64
	GrantType string
	Status    string
	Available decimal.Decimal
	Frozen    decimal.Decimal
	Consumed  decimal.Decimal
	ExpiresAt *time.Time
}

type memoryGrantAllocation struct {
	GrantID int64
	Points  decimal.Decimal
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
	paymentTrades map[string]int64
	nextOrderID   int64
	webhooks      []domainbilling.PaymentWebhookEvent
	nextWebhookID int64
	breakdown     map[int64]memoryBreakdown
	subs          map[int64]*domainbilling.UserSubscriptionSummary
	trialGrants   map[string]SignupTrialGrantStoreRequest
	refundFreezes map[int64]memoryRefundFreeze
	refundTrades  map[int64]map[string]struct{}
	refundRetries map[int64]domainbilling.RefundPaymentOrderRequest
	walletGrants  map[int64][]*memoryWalletGrant
	nextGrantID   int64
}

type memoryBreakdown struct {
	Trial             decimal.Decimal
	Subscription      decimal.Decimal
	Gift              decimal.Decimal
	Recharge          decimal.Decimal
	TrialExpires      *time.Time
	TrialReminderDays int
}

type memoryRefundFreeze struct {
	RefundTradeNo   string
	RefundAmountCNY decimal.Decimal
	RefundPoints    decimal.Decimal
	Subscription    decimal.Decimal
	Gift            decimal.Decimal
	Recharge        decimal.Decimal
	Allocations     []memoryGrantAllocation
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
		paymentTrades: map[string]int64{},
		nextOrderID:   1,
		webhooks:      []domainbilling.PaymentWebhookEvent{},
		nextWebhookID: 1,
		breakdown:     map[int64]memoryBreakdown{},
		subs:          map[int64]*domainbilling.UserSubscriptionSummary{},
		trialGrants:   map[string]SignupTrialGrantStoreRequest{},
		refundFreezes: map[int64]memoryRefundFreeze{},
		refundTrades:  map[int64]map[string]struct{}{},
		refundRetries: map[int64]domainbilling.RefundPaymentOrderRequest{},
		walletGrants:  map[int64][]*memoryWalletGrant{},
		nextGrantID:   1,
	}
}

func (s *MemoryStore) GetBalance(_ context.Context, userID int64) (BalanceState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireMemoryGrantsLocked(userID, time.Now().UTC())
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

func (s *MemoryStore) ListPlans(_ context.Context, req domainbilling.SubscriptionPlanListRequest) ([]domainbilling.SubscriptionPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]domainbilling.SubscriptionPlan, 0, len(s.plans))
	for _, item := range s.plans {
		if planMatchesListRequest(item, req) {
			items = append(items, item)
		}
	}
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
	expiryEnabled := effectiveCreditExpiryEnabled(req.CreditExpiryEnabled)
	plan := domainbilling.SubscriptionPlan{
		ID:                  s.nextPlanID,
		PlanCode:            code,
		PlanName:            strings.TrimSpace(req.PlanName),
		PlanType:            normalizePlanType(req.PlanType),
		PurchaseEnabled:     req.PurchaseEnabled,
		Status:              normalizePlanStatus(req.Status),
		PriceCNY:            strings.TrimSpace(req.PriceCNY),
		Points:              strings.TrimSpace(req.Points),
		BonusPoints:         strings.TrimSpace(req.BonusPoints),
		CreditExpiryEnabled: expiryEnabled,
		DurationDays:        effectivePlanDurationDays(expiryEnabled, req.DurationDays),
		Currency:            "CNY",
		SortOrder:           req.SortOrder,
		Description:         strings.TrimSpace(req.Description),
		CreatedAt:           now,
		UpdatedAt:           now,
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
		if item.PlanType != "points_package" {
			item.PurchaseEnabled = false
		}
		item.PriceCNY = strings.TrimSpace(req.PriceCNY)
		item.Points = strings.TrimSpace(req.Points)
		item.BonusPoints = strings.TrimSpace(req.BonusPoints)
		expiryEnabled := effectiveCreditExpiryEnabled(req.CreditExpiryEnabled)
		item.CreditExpiryEnabled = expiryEnabled
		item.DurationDays = effectivePlanDurationDays(expiryEnabled, req.DurationDays)
		item.Currency = "CNY"
		item.SortOrder = req.SortOrder
		item.Description = strings.TrimSpace(req.Description)
		item.UpdatedAt = time.Now().UTC()
		s.plans[index] = item
		return item, nil
	}
	return domainbilling.SubscriptionPlan{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "subscription plan not found")
}

func (s *MemoryStore) DeletePlan(ctx context.Context, planID int64) (domainbilling.SubscriptionPlan, error) {
	return s.TransitionPlan(ctx, domainbilling.TransitionSubscriptionPlanRequest{
		PlanID: planID,
		Action: domainbilling.SubscriptionPlanActionArchive,
	})
}

func (s *MemoryStore) TransitionPlan(_ context.Context, req domainbilling.TransitionSubscriptionPlanRequest) (domainbilling.SubscriptionPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, item := range s.plans {
		if item.ID != req.PlanID {
			continue
		}
		status, purchaseEnabled, err := TransitionPlanState(item, req.Action)
		if err != nil {
			return domainbilling.SubscriptionPlan{}, err
		}
		item.Status = status
		item.PurchaseEnabled = purchaseEnabled
		item.UpdatedAt = time.Now().UTC()
		s.plans[index] = item
		return item, nil
	}
	return domainbilling.SubscriptionPlan{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "subscription plan not found")
}

func planMatchesListRequest(plan domainbilling.SubscriptionPlan, req domainbilling.SubscriptionPlanListRequest) bool {
	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status == "all" {
		return true
	}
	if status != "" {
		return plan.Status == status
	}
	return plan.Status != domainbilling.SubscriptionPlanStatusArchived
}

func TransitionPlanState(plan domainbilling.SubscriptionPlan, action string) (string, bool, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	switch action {
	case domainbilling.SubscriptionPlanActionEnable:
		if plan.Status == domainbilling.SubscriptionPlanStatusArchived {
			return "", false, errs.New(http.StatusConflict, errs.CodeConflict, "archived subscription plan must be restored before enabling")
		}
		return domainbilling.SubscriptionPlanStatusActive, plan.PlanType == "points_package", nil
	case domainbilling.SubscriptionPlanActionDisable:
		if plan.Status == domainbilling.SubscriptionPlanStatusArchived {
			return "", false, errs.New(http.StatusConflict, errs.CodeConflict, "archived subscription plan cannot be disabled")
		}
		return domainbilling.SubscriptionPlanStatusDisabled, false, nil
	case domainbilling.SubscriptionPlanActionArchive:
		return domainbilling.SubscriptionPlanStatusArchived, false, nil
	case domainbilling.SubscriptionPlanActionRestore:
		if plan.Status == domainbilling.SubscriptionPlanStatusActive {
			return "", false, errs.New(http.StatusConflict, errs.CodeConflict, "active subscription plan cannot be restored")
		}
		return domainbilling.SubscriptionPlanStatusDisabled, false, nil
	default:
		return "", false, errs.BadRequest("invalid subscription plan action")
	}
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
		ID:                  s.nextOrderID,
		OrderNo:             orderNo,
		UserID:              req.UserID,
		PlanID:              plan.ID,
		PlanCode:            plan.PlanCode,
		PlanName:            plan.PlanName,
		Provider:            strings.TrimSpace(req.Provider),
		PurchaseType:        defaultString(strings.TrimSpace(req.PurchaseType), "plan"),
		VisibleMethod:       strings.TrimSpace(req.VisibleMethod),
		ProviderType:        defaultString(strings.TrimSpace(req.ProviderType), strings.TrimSpace(req.Provider)),
		ProviderInstanceID:  req.ProviderInstanceID,
		PaymentDisplay:      cloneMap(req.PaymentDisplay),
		IdempotencyKey:      idempotencyKey,
		Status:              "pending",
		Currency:            "CNY",
		AmountCNY:           plan.PriceCNY,
		Points:              plan.Points,
		BonusPoints:         plan.BonusPoints,
		CreditExpiryEnabled: plan.CreditExpiryEnabled,
		CreditValidDays:     effectivePlanDurationDays(plan.CreditExpiryEnabled, plan.DurationDays),
		PaymentURL:          paymentURL,
		QRCode:              strings.TrimSpace(req.QRCode),
		ClientToken:         strings.TrimSpace(req.ClientToken),
		ExpiresAt:           now.Add(15 * time.Minute),
		CreatedAt:           now,
		UpdatedAt:           now,
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

func (s *MemoryStore) InitializePaymentOrder(_ context.Context, req domainbilling.InitializePaymentOrderRequest) (domainbilling.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[req.OrderID]
	if !ok || order.UserID != req.UserID {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
	}
	if order.Status != "pending" {
		return order, nil
	}
	order.PaymentDisplay = cloneMap(req.PaymentDisplay)
	order.PaymentURL = strings.TrimSpace(req.PaymentURL)
	order.QRCode = strings.TrimSpace(req.QRCode)
	order.ClientToken = strings.TrimSpace(req.ClientToken)
	order.TradeNo = strings.TrimSpace(req.TradeNo)
	order.FailureReason = ""
	order.UpdatedAt = time.Now().UTC()
	s.orders[order.ID] = order
	return order, nil
}

func (s *MemoryStore) FailPaymentOrderInitialization(_ context.Context, req domainbilling.FailPaymentOrderInitializationRequest) (domainbilling.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[req.OrderID]
	if !ok || order.UserID != req.UserID {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
	}
	if order.Status != "pending" || paymentOrderHasInitialization(order) {
		return order, nil
	}
	now := time.Now().UTC()
	order.Status = "failed"
	order.FailureReason = strings.TrimSpace(req.FailureReason)
	order.ClosedAt = &now
	order.UpdatedAt = now
	s.orders[order.ID] = order
	return order, nil
}

func paymentOrderHasInitialization(order domainbilling.PaymentOrder) bool {
	return strings.TrimSpace(order.PaymentURL) != "" || strings.TrimSpace(order.QRCode) != "" || strings.TrimSpace(order.ClientToken) != "" || len(order.PaymentDisplay) > 0
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
	reconciliationSource, err := NormalizePaymentReconciliationSource(req.ReconciliationSource)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, order := range s.orders {
		if order.OrderNo != req.OrderNo {
			continue
		}
		if err := ValidatePaymentCallbackBinding(order.ProviderType, order.Provider, order.ProviderInstanceID, req); err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		if err := ensurePaymentAmountMatches(order.AmountCNY, req.AmountCNY, s.scale); err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		tradeKey := memoryPaymentTradeKey(req.Provider, req.TradeNo)
		if err := s.validateMemoryPaymentTradeOwner(tradeKey, order.ID); err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		if isCashierRechargeOrder(order) {
			completed, err := s.completeRechargeOrderLocked(order, domainbilling.CompleteRechargeOrderRequest{
				UserID:   order.UserID,
				OrderID:  order.ID,
				Provider: req.Provider,
				TradeNo:  req.TradeNo,
			}, reconciliationSource)
			if err == nil {
				s.recordMemoryPaymentTradeOwner(tradeKey, order.ID)
			}
			return completed, err
		}
		if order.Status == "paid" {
			s.recordMemoryPaymentTradeOwner(tradeKey, order.ID)
			return order, nil
		}
		now := time.Now().UTC()
		order.Status = "paid"
		order.TradeNo = req.TradeNo
		order.PaidAt = &now
		order.UpdatedAt = now
		s.orders[id] = order
		s.recordMemoryPaymentTradeOwner(tradeKey, order.ID)
		current := s.balances[order.UserID]
		points, _ := decimal.NewFromString(order.Points)
		bonus, _ := decimal.NewFromString(order.BonusPoints)
		current.Available = current.Available.Add(points).Add(bonus)
		s.balances[order.UserID] = current
		breakdown := s.breakdown[order.UserID]
		breakdown.Subscription = breakdown.Subscription.Add(points)
		breakdown.Gift = breakdown.Gift.Add(bonus)
		s.breakdown[order.UserID] = breakdown
		periodEnd := now
		if order.CreditValidDays != nil {
			periodEnd = now.Add(time.Duration(*order.CreditValidDays) * 24 * time.Hour)
		}
		order.CreditedAt = &now
		if order.CreditExpiryEnabled && order.CreditValidDays != nil {
			order.CreditExpiresAt = &periodEnd
		}
		s.createMemoryWalletGrantLocked(order.UserID, order.ID, "subscription", points, order.CreditExpiresAt)
		if bonus.IsPositive() {
			s.createMemoryWalletGrantLocked(order.UserID, order.ID, "gift", bonus, order.CreditExpiresAt)
		}
		order.LedgerID = s.appendLedgerWithMetadata(order.UserID, 0, "", order.ID, "order_paid", points, current, "payment order "+order.OrderNo+" purchased credits", "subscription", order.CreditExpiresAt)
		if bonus.IsPositive() {
			s.appendLedgerWithMetadata(order.UserID, 0, "", order.ID, "order_paid", bonus, current, "payment order "+order.OrderNo+" gift credits", "gift", order.CreditExpiresAt)
		}
		s.orders[id] = order
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
		return order, nil
	}
	return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
}

func memoryPaymentTradeKey(provider, tradeNo string) string {
	return strings.ToLower(strings.TrimSpace(provider)) + "\x00" + strings.TrimSpace(tradeNo)
}

func (s *MemoryStore) validateMemoryPaymentTradeOwner(tradeKey string, orderID int64) error {
	if ownerOrderID, exists := s.paymentTrades[tradeKey]; exists && ownerOrderID != orderID {
		return errs.New(http.StatusConflict, errs.CodeConflict, "payment provider trade belongs to a different order")
	}
	return nil
}

func (s *MemoryStore) recordMemoryPaymentTradeOwner(tradeKey string, orderID int64) {
	if s.paymentTrades == nil {
		s.paymentTrades = map[string]int64{}
	}
	s.paymentTrades[tradeKey] = orderID
}

func ValidatePaymentCallbackBinding(orderProviderType, orderProvider string, orderProviderInstanceID int64, req domainbilling.MarkOrderPaidRequest) error {
	expectedProvider := strings.ToLower(strings.TrimSpace(orderProviderType))
	if expectedProvider == "" {
		expectedProvider = strings.ToLower(strings.TrimSpace(orderProvider))
	}
	callbackProvider := strings.ToLower(strings.TrimSpace(req.Provider))
	if expectedProvider == "" || callbackProvider == "" || expectedProvider != callbackProvider {
		return errs.New(http.StatusConflict, errs.CodePaymentSignatureInvalid, "payment webhook provider does not match order")
	}
	if orderProviderInstanceID > 0 && req.ProviderInstanceID != orderProviderInstanceID && callbackProvider != "mock" {
		return errs.New(http.StatusConflict, errs.CodePaymentSignatureInvalid, "payment webhook provider instance does not match order")
	}
	return nil
}

func NormalizePaymentReconciliationSource(source string) (string, error) {
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		return domainbilling.PaymentReconciliationSourceProviderWebhook, nil
	}
	switch source {
	case domainbilling.PaymentReconciliationSourceProviderWebhook,
		domainbilling.PaymentReconciliationSourceProviderQuery,
		domainbilling.PaymentReconciliationSourceMockConfirmation:
		return source, nil
	default:
		return "", errs.BadRequest("payment reconciliation source is invalid")
	}
}

func PaymentSuccessCanRecoverStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending", "canceled", "expired", "failed":
		return true
	default:
		return false
	}
}

func ValidateCompletedPaymentTrade(existingTradeNo, paidTradeNo string) error {
	if strings.TrimSpace(existingTradeNo) != strings.TrimSpace(paidTradeNo) {
		return errs.New(http.StatusConflict, errs.CodeConflict, "payment provider trade does not match completed order")
	}
	return nil
}

func ValidateInitializedPaymentTrade(existingTradeNo, paidTradeNo string) error {
	existingTradeNo = strings.TrimSpace(existingTradeNo)
	if existingTradeNo != "" && existingTradeNo != strings.TrimSpace(paidTradeNo) {
		return errs.New(http.StatusConflict, errs.CodePaymentSignatureInvalid, "payment provider trade does not match initialized order")
	}
	return nil
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
	tradeKey := memoryPaymentTradeKey(req.Provider, req.TradeNo)
	if err := s.validateMemoryPaymentTradeOwner(tradeKey, order.ID); err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	completed, err := s.completeRechargeOrderLocked(order, req, domainbilling.PaymentReconciliationSourceMockConfirmation)
	if err == nil {
		s.recordMemoryPaymentTradeOwner(tradeKey, order.ID)
	}
	return completed, err
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
	if s.memoryRefundRecordExists(order.ID, refundTradeNo) {
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
	s.expireMemoryGrantsLocked(order.UserID, time.Now().UTC())
	grants := s.memoryOrderGrantsLocked(order)
	current := s.balances[order.UserID]
	frozenRefund := s.refundFreezes[order.ID]
	var allocations []memoryGrantAllocation
	if frozenRefund.RefundPoints.IsPositive() {
		if err := ensureMemoryRefundFreezeMatches(frozenRefund, refundTradeNo, plan); err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		allocations = append([]memoryGrantAllocation(nil), frozenRefund.Allocations...)
		if !s.memoryRefundAllocationsAvailableLocked(order.UserID, allocations, true) {
			return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order recharge balance is insufficient for refund")
		}
		for _, allocation := range allocations {
			grant := s.memoryGrantByIDLocked(order.UserID, allocation.GrantID)
			grant.Frozen = decimal.Max(grant.Frozen.Sub(allocation.Points), decimal.Zero).Round(s.scale)
			if grant.Status != "expired" && (plan.FullyRefunded || (!grant.Available.IsPositive() && !grant.Frozen.IsPositive())) {
				grant.Status = "refunded"
			}
		}
		current.Frozen = decimal.Max(current.Frozen.Sub(plan.RefundPoints), decimal.Zero).Round(s.scale)
		delete(s.refundFreezes, order.ID)
	} else {
		var ok bool
		allocations, ok = allocateMemoryRefundGrants(grants, plan.RefundPoints, false)
		if !ok || memoryGrantsConsumedOrFrozen(grants) {
			return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order recharge balance is insufficient for refund")
		}
		for _, allocation := range allocations {
			grant := s.memoryGrantByIDLocked(order.UserID, allocation.GrantID)
			grant.Available = decimal.Max(grant.Available.Sub(allocation.Points), decimal.Zero).Round(s.scale)
			s.changeMemoryBreakdownLocked(order.UserID, grant.GrantType, allocation.Points.Neg())
			if grant.Status != "expired" && (plan.FullyRefunded || (!grant.Available.IsPositive() && !grant.Frozen.IsPositive())) {
				grant.Status = "refunded"
			}
		}
		current.Available = decimal.Max(current.Available.Sub(plan.RefundPoints), decimal.Zero).Round(s.scale)
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
	for _, allocation := range allocations {
		grant := s.memoryGrantByIDLocked(order.UserID, allocation.GrantID)
		if grant == nil {
			continue
		}
		s.appendLedgerWithMetadata(order.UserID, 0, "", order.ID, "payment_refund", allocation.Points.Neg(), current, strings.TrimSpace(req.Reason), grant.GrantType, grant.ExpiresAt)
	}
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
	if s.memoryRefundRecordExists(order.ID, refundTradeNo) {
		return order, nil
	}
	plan, err := s.memoryPaymentOrderRefundPlan(order, req)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	if frozen := s.refundFreezes[order.ID]; frozen.RefundPoints.IsPositive() {
		if err := ensureMemoryRefundFreezeMatches(frozen, refundTradeNo, plan); err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		return order, nil
	}
	s.expireMemoryGrantsLocked(order.UserID, time.Now().UTC())
	current := s.balances[order.UserID]
	grants := s.memoryOrderGrantsLocked(order)
	allocations, ok := allocateMemoryRefundGrants(grants, plan.RefundPoints, false)
	if !ok || memoryGrantsConsumedOrFrozen(grants) {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order recharge balance is insufficient for refund")
	}
	bucketAllocation := memoryRefundFreeze{}
	for _, allocation := range allocations {
		grant := s.memoryGrantByIDLocked(order.UserID, allocation.GrantID)
		grant.Available = grant.Available.Sub(allocation.Points).Round(s.scale)
		grant.Frozen = grant.Frozen.Add(allocation.Points).Round(s.scale)
		s.changeMemoryBreakdownLocked(order.UserID, grant.GrantType, allocation.Points.Neg())
		switch grant.GrantType {
		case "subscription":
			bucketAllocation.Subscription = bucketAllocation.Subscription.Add(allocation.Points)
		case "recharge":
			bucketAllocation.Recharge = bucketAllocation.Recharge.Add(allocation.Points)
		default:
			bucketAllocation.Gift = bucketAllocation.Gift.Add(allocation.Points)
		}
	}
	current.Available = current.Available.Sub(plan.RefundPoints).Round(s.scale)
	current.Frozen = current.Frozen.Add(plan.RefundPoints)
	s.balances[order.UserID] = current
	s.refundFreezes[order.ID] = memoryRefundFreeze{
		RefundTradeNo:   refundTradeNo,
		RefundAmountCNY: plan.RefundAmountCNY,
		RefundPoints:    plan.RefundPoints,
		Subscription:    bucketAllocation.Subscription,
		Gift:            bucketAllocation.Gift,
		Recharge:        bucketAllocation.Recharge,
		Allocations:     allocations,
	}
	return order, nil
}

func (s *MemoryStore) ReleaseRefundPaymentOrder(_ context.Context, req domainbilling.RefundPaymentOrderRequest) (domainbilling.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[req.OrderID]
	if !ok || order.UserID != req.UserID {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
	}
	freeze := s.refundFreezes[order.ID]
	if !freeze.RefundPoints.IsPositive() || freeze.RefundTradeNo != strings.TrimSpace(req.RefundTradeNo) {
		return order, nil
	}
	s.expireMemoryGrantsLocked(order.UserID, time.Now().UTC())
	current := s.balances[order.UserID]
	restored := decimal.Zero
	released := decimal.Zero
	now := time.Now().UTC()
	for _, allocation := range freeze.Allocations {
		grant := s.memoryGrantByIDLocked(order.UserID, allocation.GrantID)
		if grant == nil {
			continue
		}
		points := decimal.Min(grant.Frozen, allocation.Points)
		grant.Frozen = grant.Frozen.Sub(points).Round(s.scale)
		released = released.Add(points)
		if grant.Status == "active" && (grant.ExpiresAt == nil || grant.ExpiresAt.After(now)) {
			grant.Available = grant.Available.Add(points).Round(s.scale)
			s.changeMemoryBreakdownLocked(order.UserID, grant.GrantType, points)
			restored = restored.Add(points)
		} else {
			grant.Status = "expired"
			grant.Available = decimal.Zero
		}
	}
	current.Frozen = decimal.Max(current.Frozen.Sub(released), decimal.Zero).Round(s.scale)
	current.Available = current.Available.Add(restored).Round(s.scale)
	s.balances[order.UserID] = current
	delete(s.refundFreezes, order.ID)
	return order, nil
}

func (s *MemoryStore) RecordProviderRefundStatus(_ context.Context, req ProviderRefundStatusRequest) (domainbilling.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[req.OrderID]
	if !ok || order.UserID != req.UserID {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
	}
	order.RefundTradeNo = strings.TrimSpace(req.RefundTradeNo)
	order.ChannelRefundNo = strings.TrimSpace(req.ChannelRefundNo)
	order.ChannelRefundStatus = strings.ToLower(strings.TrimSpace(req.ChannelRefundStatus))
	order.UpdatedAt = time.Now().UTC()
	s.orders[order.ID] = order
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
	if s.memoryRefundRecordExists(order.ID, refundTradeNo) {
		return order, nil
	}
	plan, err := s.memoryPaymentOrderRefundPlan(order, req)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	s.expireMemoryGrantsLocked(order.UserID, time.Now().UTC())
	frozenRefund := s.refundFreezes[order.ID]
	if !frozenRefund.RefundPoints.IsPositive() {
		grants := s.memoryOrderGrantsLocked(order)
		_, ok := allocateMemoryRefundGrants(grants, plan.RefundPoints, false)
		if !ok || memoryGrantsConsumedOrFrozen(grants) {
			return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order recharge balance is insufficient for refund")
		}
		return order, nil
	}
	if err := ensureMemoryRefundFreezeMatches(frozenRefund, refundTradeNo, plan); err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	if !s.memoryRefundAllocationsAvailableLocked(order.UserID, frozenRefund.Allocations, true) {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order recharge balance is insufficient for refund")
	}
	return order, nil
}

func deductPaymentOrderBreakdown(order domainbilling.PaymentOrder, breakdown memoryBreakdown, amount decimal.Decimal) (memoryBreakdown, memoryRefundFreeze, bool) {
	allocation := memoryRefundFreeze{}
	remaining := amount
	if order.PurchaseType == "custom_amount" || order.PlanID == 0 {
		if breakdown.Recharge.LessThan(remaining) {
			return breakdown, allocation, false
		}
		breakdown.Recharge = breakdown.Recharge.Sub(remaining)
		allocation.Recharge = remaining
		return breakdown, allocation, true
	}
	fromGift := decimal.Min(breakdown.Gift, remaining)
	breakdown.Gift = breakdown.Gift.Sub(fromGift)
	allocation.Gift = fromGift
	remaining = remaining.Sub(fromGift)
	fromSubscription := decimal.Min(breakdown.Subscription, remaining)
	breakdown.Subscription = breakdown.Subscription.Sub(fromSubscription)
	allocation.Subscription = fromSubscription
	remaining = remaining.Sub(fromSubscription)
	return breakdown, allocation, !remaining.IsPositive()
}

func (s *MemoryStore) memoryOrderGrantsLocked(order domainbilling.PaymentOrder) []*memoryWalletGrant {
	grants := make([]*memoryWalletGrant, 0, len(s.walletGrants[order.UserID]))
	for _, grant := range s.walletGrants[order.UserID] {
		if grant.OrderID != order.ID || (grant.Status != "active" && grant.Status != "expired") {
			continue
		}
		if order.PurchaseType == "custom_amount" || order.PlanID == 0 {
			if grant.GrantType == "recharge" {
				grants = append(grants, grant)
			}
			continue
		}
		if grant.GrantType == "subscription" || grant.GrantType == "gift" || grant.GrantType == "recharge" {
			grants = append(grants, grant)
		}
	}
	sort.SliceStable(grants, func(i, j int) bool { return grants[i].ID < grants[j].ID })
	return grants
}

func allocateMemoryRefundGrants(grants []*memoryWalletGrant, amount decimal.Decimal, useFrozen bool) ([]memoryGrantAllocation, bool) {
	remaining := amount
	allocations := make([]memoryGrantAllocation, 0, len(grants))
	for _, grant := range grants {
		available := grant.Available
		if useFrozen {
			available = grant.Frozen
		}
		if !available.IsPositive() {
			continue
		}
		points := decimal.Min(available, remaining)
		allocations = append(allocations, memoryGrantAllocation{GrantID: grant.ID, Points: points})
		remaining = remaining.Sub(points)
		if !remaining.IsPositive() {
			return allocations, true
		}
	}
	return nil, false
}

func memoryGrantsConsumedOrFrozen(grants []*memoryWalletGrant) bool {
	for _, grant := range grants {
		if grant.Consumed.IsPositive() || grant.Frozen.IsPositive() {
			return true
		}
	}
	return false
}

func (s *MemoryStore) memoryRefundAllocationsAvailableLocked(userID int64, allocations []memoryGrantAllocation, useFrozen bool) bool {
	if len(allocations) == 0 {
		return false
	}
	for _, allocation := range allocations {
		grant := s.memoryGrantByIDLocked(userID, allocation.GrantID)
		if grant == nil {
			return false
		}
		available := grant.Available
		if useFrozen {
			available = grant.Frozen
		}
		if available.LessThan(allocation.Points) {
			return false
		}
	}
	return true
}

func ensureMemoryRefundFreezeMatches(freeze memoryRefundFreeze, refundTradeNo string, plan memoryPaymentOrderRefundPlan) error {
	if freeze.RefundTradeNo != strings.TrimSpace(refundTradeNo) {
		return errs.New(http.StatusConflict, errs.CodeConflict, "another payment refund is pending")
	}
	if !freeze.RefundAmountCNY.Equal(plan.RefundAmountCNY) || !freeze.RefundPoints.Equal(plan.RefundPoints) {
		return errs.New(http.StatusConflict, errs.CodePaymentAmountMismatch, "payment refund amount does not match the pending refund")
	}
	return nil
}

func (s *MemoryStore) completeRechargeOrderLocked(order domainbilling.PaymentOrder, req domainbilling.CompleteRechargeOrderRequest, reconciliationSource string) (domainbilling.PaymentOrder, error) {
	if order.Status == "completed" {
		if err := ValidateCompletedPaymentTrade(order.TradeNo, req.TradeNo); err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		return order, nil
	}
	if err := ValidateInitializedPaymentTrade(order.TradeNo, req.TradeNo); err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	if !PaymentSuccessCanRecoverStatus(order.Status) {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order cannot transition to completed")
	}
	previousStatus := order.Status
	now := time.Now().UTC()
	order.Status = "completed"
	order.Provider = strings.ToLower(strings.TrimSpace(req.Provider))
	order.TradeNo = strings.TrimSpace(req.TradeNo)
	order.PaidAt = &now
	order.CompletedAt = &now
	order.CreditedAt = &now
	if order.CreditExpiryEnabled && order.CreditValidDays != nil {
		expiresAt := now.Add(time.Duration(*order.CreditValidDays) * 24 * time.Hour)
		order.CreditExpiresAt = &expiresAt
	} else {
		order.CreditExpiresAt = nil
	}
	order.ClosedAt = nil
	order.FailureReason = ""
	order.UpdatedAt = now
	s.appendWebhookEvent(order, previousStatus, reconciliationSource, now)
	points, _ := decimal.NewFromString(order.Points)
	bonus, _ := decimal.NewFromString(order.BonusPoints)
	total := points.Add(bonus)
	current := s.balances[order.UserID]
	current.Available = current.Available.Add(total)
	s.balances[order.UserID] = current
	breakdown := s.breakdown[order.UserID]
	if order.PurchaseType == "custom_amount" || order.PlanID == 0 {
		breakdown.Recharge = breakdown.Recharge.Add(total)
		s.createMemoryWalletGrantLocked(order.UserID, order.ID, "recharge", total, nil)
	} else {
		breakdown.Subscription = breakdown.Subscription.Add(points)
		breakdown.Gift = breakdown.Gift.Add(bonus)
		s.createMemoryWalletGrantLocked(order.UserID, order.ID, "subscription", points, order.CreditExpiresAt)
		if bonus.IsPositive() {
			s.createMemoryWalletGrantLocked(order.UserID, order.ID, "gift", bonus, order.CreditExpiresAt)
		}
	}
	s.breakdown[order.UserID] = breakdown
	if order.PurchaseType == "custom_amount" || order.PlanID == 0 {
		order.LedgerID = s.appendLedgerWithMetadata(order.UserID, 0, "", order.ID, "recharge", total, current, "cashier order "+order.OrderNo, "recharge", order.CreditExpiresAt)
	} else {
		order.LedgerID = s.appendLedgerWithMetadata(order.UserID, 0, "", order.ID, "order_paid", points, current, "payment order "+order.OrderNo+" purchased credits", "subscription", order.CreditExpiresAt)
		if bonus.IsPositive() {
			s.appendLedgerWithMetadata(order.UserID, 0, "", order.ID, "order_paid", bonus, current, "payment order "+order.OrderNo+" gift credits", "gift", order.CreditExpiresAt)
		}
	}
	s.orders[order.ID] = order
	return order, nil
}

func (s *MemoryStore) createMemoryWalletGrantLocked(userID, orderID int64, grantType string, amount decimal.Decimal, expiresAt *time.Time) *memoryWalletGrant {
	if !amount.IsPositive() {
		return nil
	}
	grant := &memoryWalletGrant{
		ID: s.nextGrantID, UserID: userID, OrderID: orderID, GrantType: grantType, Status: "active",
		Available: amount.Round(s.scale), ExpiresAt: cloneMemoryTime(expiresAt),
	}
	s.nextGrantID++
	s.walletGrants[userID] = append(s.walletGrants[userID], grant)
	return grant
}

func cloneMemoryTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func isCashierRechargeOrder(order domainbilling.PaymentOrder) bool {
	return strings.TrimSpace(order.VisibleMethod) != "" || strings.TrimSpace(order.PurchaseType) == "custom_amount" || order.ProviderInstanceID > 0 || len(order.PaymentDisplay) > 0
}

func (s *MemoryStore) appendWebhookEvent(order domainbilling.PaymentOrder, previousStatus, reconciliationSource string, now time.Time) {
	payload := map[string]any{
		"order_no":              order.OrderNo,
		"previous_local_status": previousStatus,
		"reconciliation_source": reconciliationSource,
	}
	event := domainbilling.PaymentWebhookEvent{
		ID:              s.nextWebhookID,
		OrderID:         order.ID,
		OrderNo:         order.OrderNo,
		ProviderType:    order.Provider,
		Status:          "processed",
		EventType:       "payment.succeeded",
		SignatureStatus: "verified",
		ResultSummary:   "已完成本地处理",
		PayloadPreview:  string(mustMemoryJSON(payload)),
		ReceivedAt:      now,
		ProcessedAt:     &now,
	}
	s.nextWebhookID++
	s.webhooks = append([]domainbilling.PaymentWebhookEvent{event}, s.webhooks...)
}

func mustMemoryJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return encoded
}

func (s *MemoryStore) ReserveTask(_ context.Context, req ReserveStoreRequest) (BalanceState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireMemoryGrantsLocked(req.UserID, time.Now().UTC())
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
	allocations, untracked, ok := s.reserveMemoryGrantsLocked(req.UserID, estimated)
	if !ok {
		return BalanceState{}, errs.New(400, errs.CodeInsufficientPoints, "insufficient points")
	}
	current.Available = current.Available.Sub(estimated)
	current.Frozen = current.Frozen.Add(estimated)
	s.balances[req.UserID] = current
	s.appendLedger(req.UserID, req.APIKeyID, req.TaskID, "reserve", estimated.Neg(), current, req.Reason)
	s.addAPIKeyUsage(req.APIKeyID, estimated, time.Now().UTC())
	state.Active = true
	state.Reserved = estimated
	state.GrantAllocations = allocations
	state.UntrackedReserved = untracked
	s.taskState[req.TaskID] = state
	return s.formatState(req.UserID, current), nil
}

func (s *MemoryStore) FinalizeTask(_ context.Context, req FinalizeStoreRequest) (BalanceState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireMemoryGrantsLocked(req.UserID, time.Now().UTC())
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
		restored := s.settleMemoryTaskGrantsLocked(req.UserID, state, decimal.Zero)
		current.Available = current.Available.Add(restored)
		current.Frozen = current.Frozen.Sub(reserved)
		apiKeyID := firstPositive(req.APIKeyID, state.APIKeyID)
		s.appendLedger(req.UserID, apiKeyID, req.TaskID, "refund", restored, current, req.Reason)
		s.addAPIKeyUsage(apiKeyID, reserved.Neg(), time.Now().UTC())
		s.balances[req.UserID] = current
		state.Active = false
		state.Reserved = decimal.Zero
		state.GrantAllocations = nil
		state.UntrackedReserved = decimal.Zero
		s.taskState[req.TaskID] = state
		return s.formatState(req.UserID, current), nil
	}
	restored := s.settleMemoryTaskGrantsLocked(req.UserID, state, actual)
	current.Frozen = current.Frozen.Sub(reserved)
	apiKeyID := firstPositive(req.APIKeyID, state.APIKeyID)
	s.appendLedger(req.UserID, apiKeyID, req.TaskID, "consume", actual.Neg(), current, req.Reason)
	if restored.GreaterThan(decimal.Zero) {
		current.Available = current.Available.Add(restored)
		s.appendLedger(req.UserID, apiKeyID, req.TaskID, "refund", restored, current, req.Reason)
		s.addAPIKeyUsage(apiKeyID, restored.Neg(), time.Now().UTC())
	}
	s.balances[req.UserID] = current
	state.Active = false
	state.Reserved = decimal.Zero
	state.GrantAllocations = nil
	state.UntrackedReserved = decimal.Zero
	s.taskState[req.TaskID] = state
	return s.formatState(req.UserID, current), nil
}

func (s *MemoryStore) reserveMemoryGrantsLocked(userID int64, amount decimal.Decimal) ([]memoryGrantAllocation, decimal.Decimal, bool) {
	grants := append([]*memoryWalletGrant(nil), s.walletGrants[userID]...)
	now := time.Now().UTC()
	sort.SliceStable(grants, func(i, j int) bool {
		leftPriority := memoryGrantPriority(grants[i].GrantType)
		rightPriority := memoryGrantPriority(grants[j].GrantType)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		if grants[i].ExpiresAt == nil {
			return false
		}
		if grants[j].ExpiresAt == nil {
			return true
		}
		if !grants[i].ExpiresAt.Equal(*grants[j].ExpiresAt) {
			return grants[i].ExpiresAt.Before(*grants[j].ExpiresAt)
		}
		return grants[i].ID < grants[j].ID
	})
	remaining := amount
	allocations := make([]memoryGrantAllocation, 0, len(grants))
	for _, grant := range grants {
		if !remaining.IsPositive() {
			break
		}
		if grant.Status != "active" || !grant.Available.IsPositive() || (grant.ExpiresAt != nil && !grant.ExpiresAt.After(now)) {
			continue
		}
		take := decimal.Min(grant.Available, remaining).Round(s.scale)
		grant.Available = grant.Available.Sub(take).Round(s.scale)
		grant.Frozen = grant.Frozen.Add(take).Round(s.scale)
		s.changeMemoryBreakdownLocked(userID, grant.GrantType, take.Neg())
		allocations = append(allocations, memoryGrantAllocation{GrantID: grant.ID, Points: take})
		remaining = remaining.Sub(take).Round(s.scale)
	}
	untrackedAvailable := s.balances[userID].Available.Sub(amount.Sub(remaining))
	if remaining.GreaterThan(untrackedAvailable) {
		for _, allocation := range allocations {
			grant := s.memoryGrantByIDLocked(userID, allocation.GrantID)
			if grant == nil {
				continue
			}
			grant.Available = grant.Available.Add(allocation.Points).Round(s.scale)
			grant.Frozen = grant.Frozen.Sub(allocation.Points).Round(s.scale)
			s.changeMemoryBreakdownLocked(userID, grant.GrantType, allocation.Points)
		}
		return nil, decimal.Zero, false
	}
	return allocations, remaining, true
}

func (s *MemoryStore) settleMemoryTaskGrantsLocked(userID int64, state memoryTaskBillingState, actual decimal.Decimal) decimal.Decimal {
	remainingActual := actual
	restored := decimal.Zero
	now := time.Now().UTC()
	for _, allocation := range state.GrantAllocations {
		grant := s.memoryGrantByIDLocked(userID, allocation.GrantID)
		if grant == nil {
			continue
		}
		consume := decimal.Min(allocation.Points, remainingActual)
		if consume.IsNegative() {
			consume = decimal.Zero
		}
		remainingActual = remainingActual.Sub(consume)
		refund := allocation.Points.Sub(consume)
		grant.Frozen = decimal.Max(grant.Frozen.Sub(allocation.Points), decimal.Zero).Round(s.scale)
		grant.Consumed = grant.Consumed.Add(consume).Round(s.scale)
		if refund.IsPositive() && grant.Status == "active" && (grant.ExpiresAt == nil || grant.ExpiresAt.After(now)) {
			grant.Available = grant.Available.Add(refund).Round(s.scale)
			s.changeMemoryBreakdownLocked(userID, grant.GrantType, refund)
			restored = restored.Add(refund)
		}
	}
	untrackedConsume := decimal.Min(state.UntrackedReserved, remainingActual)
	if untrackedConsume.IsNegative() {
		untrackedConsume = decimal.Zero
	}
	untrackedRefund := state.UntrackedReserved.Sub(untrackedConsume)
	if untrackedRefund.IsPositive() {
		restored = restored.Add(untrackedRefund)
	}
	return restored.Round(s.scale)
}

func (s *MemoryStore) memoryGrantByIDLocked(userID, grantID int64) *memoryWalletGrant {
	for _, grant := range s.walletGrants[userID] {
		if grant.ID == grantID {
			return grant
		}
	}
	return nil
}

func memoryGrantPriority(grantType string) int {
	switch grantType {
	case "trial":
		return 1
	case "subscription":
		return 2
	case "gift":
		return 3
	case "recharge":
		return 4
	default:
		return 9
	}
}

func (s *MemoryStore) changeMemoryBreakdownLocked(userID int64, grantType string, change decimal.Decimal) {
	breakdown := s.breakdown[userID]
	switch grantType {
	case "trial":
		breakdown.Trial = decimal.Max(breakdown.Trial.Add(change), decimal.Zero)
	case "subscription":
		breakdown.Subscription = decimal.Max(breakdown.Subscription.Add(change), decimal.Zero)
	case "recharge":
		breakdown.Recharge = decimal.Max(breakdown.Recharge.Add(change), decimal.Zero)
	default:
		breakdown.Gift = decimal.Max(breakdown.Gift.Add(change), decimal.Zero)
	}
	s.breakdown[userID] = breakdown
}

func (s *MemoryStore) expireMemoryGrantsLocked(userID int64, now time.Time) {
	for _, grant := range s.walletGrants[userID] {
		if grant.Status != "active" || grant.ExpiresAt == nil || grant.ExpiresAt.After(now) {
			continue
		}
		expired := grant.Available
		grant.Available = decimal.Zero
		grant.Status = "expired"
		if !expired.IsPositive() {
			continue
		}
		current := s.balances[userID]
		current.Available = decimal.Max(current.Available.Sub(expired), decimal.Zero).Round(s.scale)
		s.balances[userID] = current
		s.changeMemoryBreakdownLocked(userID, grant.GrantType, expired.Neg())
		s.appendLedgerWithMetadata(userID, 0, "", grant.OrderID, "expire", expired.Neg(), current, "expired "+grant.GrantType+" grant", grant.GrantType, grant.ExpiresAt)
	}
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
		s.breakdown[req.UserID] = breakdown
		s.createMemoryWalletGrantLocked(req.UserID, 0, "gift", change, nil)
	} else if change.IsNegative() {
		remaining := s.deductMemoryGrantsForAdminLocked(req.UserID, change.Abs())
		breakdown = s.breakdown[req.UserID]
		if remaining.IsPositive() {
			breakdown = deductMemoryBreakdownForAdminAdjustment(breakdown, remaining)
			s.breakdown[req.UserID] = breakdown
		}
	} else {
		s.breakdown[req.UserID] = breakdown
	}
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

func (s *MemoryStore) deductMemoryGrantsForAdminLocked(userID int64, amount decimal.Decimal) decimal.Decimal {
	grants := append([]*memoryWalletGrant(nil), s.walletGrants[userID]...)
	sort.SliceStable(grants, func(i, j int) bool {
		leftPriority := memoryAdminGrantPriority(grants[i].GrantType)
		rightPriority := memoryAdminGrantPriority(grants[j].GrantType)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		return grants[i].ID < grants[j].ID
	})
	remaining := amount
	now := time.Now().UTC()
	for _, grant := range grants {
		if !remaining.IsPositive() {
			break
		}
		if grant.Status != "active" || !grant.Available.IsPositive() || (grant.ExpiresAt != nil && !grant.ExpiresAt.After(now)) {
			continue
		}
		deduct := decimal.Min(grant.Available, remaining)
		grant.Available = grant.Available.Sub(deduct).Round(s.scale)
		s.changeMemoryBreakdownLocked(userID, grant.GrantType, deduct.Neg())
		remaining = remaining.Sub(deduct).Round(s.scale)
	}
	return remaining
}

func memoryAdminGrantPriority(grantType string) int {
	switch grantType {
	case "recharge":
		return 1
	case "gift":
		return 2
	case "subscription":
		return 3
	case "trial":
		return 4
	default:
		return 9
	}
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
	breakdown.TrialExpires = &expiresAt
	breakdown.TrialReminderDays = req.ExpiryReminderDays
	s.breakdown[req.UserID] = breakdown
	s.createMemoryWalletGrantLocked(req.UserID, 0, "trial", amount, &expiresAt)
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
	return s.appendLedgerWithMetadata(userID, apiKeyID, taskID, 0, ledgerType, change, current, reason, "", nil)
}

func (s *MemoryStore) appendLedgerWithMetadata(userID, apiKeyID int64, taskID string, orderID int64, ledgerType string, change decimal.Decimal, current balanceState, reason, bucket string, expiresAt *time.Time) int64 {
	entry := domainbilling.LedgerEntry{
		ID:            s.nextID,
		UserID:        userID,
		APIKeyID:      apiKeyID,
		TaskID:        taskID,
		OrderID:       orderID,
		LedgerType:    ledgerType,
		ChangePoints:  change.Round(s.scale).StringFixed(s.scale),
		BalanceAfter:  current.Available.Round(s.scale).StringFixed(s.scale),
		FrozenAfter:   current.Frozen.Round(s.scale).StringFixed(s.scale),
		BalanceBucket: bucket,
		Reason:        reason,
		CreatedAt:     time.Now().UTC(),
	}
	if orderID > 0 {
		entry.SourceType = "payment_order"
		entry.SourceID = orderID
	}
	if bucket != "" {
		entry.BucketBalanceAfter = s.memoryBucketBalanceAfter(userID, bucket)
	}
	if expiresAt != nil {
		value := expiresAt.Format(time.RFC3339)
		entry.ExpiresAt = &value
	}
	entry = domainbilling.PopulateLedgerDisplayFields(entry)
	s.nextID++
	s.ledgers[userID] = append([]domainbilling.LedgerEntry{entry}, s.ledgers[userID]...)
	return entry.ID
}

func (s *MemoryStore) memoryBucketBalanceAfter(userID int64, bucket string) string {
	breakdown := s.breakdown[userID]
	value := breakdown.Gift
	switch bucket {
	case "trial":
		value = breakdown.Trial
	case "subscription":
		value = breakdown.Subscription
	case "recharge":
		value = breakdown.Recharge
	}
	return value.Round(s.scale).StringFixed(s.scale)
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
	now := time.Now().UTC()
	buckets := make([]domainbilling.BalanceBucket, 0, 4)
	for _, bucketType := range []string{"trial", "subscription", "gift", "recharge"} {
		available := memoryBreakdownBucket(breakdown, bucketType)
		frozen, expiresAt, mixedExpiry, trackedAvailable := s.memoryBucketGrantProjection(userID, bucketType, now)
		if available.GreaterThan(trackedAvailable) && expiresAt != nil {
			mixedExpiry = true
			expiresAt = nil
		}
		if !available.IsPositive() && !frozen.IsPositive() {
			continue
		}
		reminderDays := 2
		if bucketType == "trial" {
			reminderDays = breakdown.TrialReminderDays
		}
		buckets = append(buckets, domainbilling.BalanceBucket{
			Bucket: bucketType, Label: memoryBucketLabel(bucketType),
			AvailablePoints: available.Round(s.scale).StringFixed(s.scale),
			FrozenPoints:    frozen.Round(s.scale).StringFixed(s.scale),
			ExpiresAt:       expiresAt, ExpireWarning: memoryExpireWarning(now, expiresAt, reminderDays), MixedExpiry: mixedExpiry,
			SourceType: memoryBucketSource(bucketType), SortOrder: memoryGrantPriority(bucketType),
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
		NextExpiringGrant:  s.memoryNextExpiringGrant(userID, now),
	}
}

func memoryBreakdownBucket(breakdown memoryBreakdown, bucket string) decimal.Decimal {
	switch bucket {
	case "trial":
		return breakdown.Trial
	case "subscription":
		return breakdown.Subscription
	case "recharge":
		return breakdown.Recharge
	default:
		return breakdown.Gift
	}
}

func (s *MemoryStore) memoryBucketGrantProjection(userID int64, bucket string, now time.Time) (decimal.Decimal, *time.Time, bool, decimal.Decimal) {
	frozen := decimal.Zero
	trackedAvailable := decimal.Zero
	var expiresAt *time.Time
	mixed := false
	for _, grant := range s.walletGrants[userID] {
		if grant.GrantType != bucket || grant.Status != "active" || (grant.ExpiresAt != nil && !grant.ExpiresAt.After(now)) {
			continue
		}
		trackedAvailable = trackedAvailable.Add(grant.Available)
		frozen = frozen.Add(grant.Frozen)
		if expiresAt == nil && grant.ExpiresAt != nil && !mixed {
			expiresAt = cloneMemoryTime(grant.ExpiresAt)
			continue
		}
		if (expiresAt == nil) != (grant.ExpiresAt == nil) || (expiresAt != nil && grant.ExpiresAt != nil && !expiresAt.Equal(*grant.ExpiresAt)) {
			mixed = true
			expiresAt = nil
		}
	}
	return frozen, expiresAt, mixed, trackedAvailable
}

func (s *MemoryStore) memoryNextExpiringGrant(userID int64, now time.Time) *domainbilling.GrantExpirySummary {
	var next *domainbilling.GrantExpirySummary
	for _, grant := range s.walletGrants[userID] {
		if grant.Status != "active" || !grant.Available.IsPositive() || grant.ExpiresAt == nil || !grant.ExpiresAt.After(now) {
			continue
		}
		if next == nil || grant.ExpiresAt.Before(*next.ExpiresAt) {
			expiresAt := *grant.ExpiresAt
			next = &domainbilling.GrantExpirySummary{GrantID: grant.ID, GrantType: grant.GrantType, AvailablePoints: grant.Available.Round(s.scale).StringFixed(s.scale), ExpiresAt: &expiresAt}
			continue
		}
		if next.ExpiresAt != nil && grant.ExpiresAt.Equal(*next.ExpiresAt) {
			next.AvailablePoints = mustMemoryDecimal(next.AvailablePoints).Add(grant.Available).Round(s.scale).StringFixed(s.scale)
			next.GrantID = 0
			if next.GrantType != grant.GrantType {
				next.GrantType = "mixed"
			}
		}
	}
	return next
}

func memoryBucketLabel(bucket string) string {
	switch bucket {
	case "trial":
		return "体验额度"
	case "subscription":
		return "套餐积分"
	case "gift":
		return "赠送积分"
	default:
		return "充值积分"
	}
}

func memoryBucketSource(bucket string) string {
	if bucket == "trial" {
		return "signup"
	}
	if bucket == "gift" {
		return "gift"
	}
	return "payment_order"
}

func mustMemoryDecimal(value string) decimal.Decimal {
	parsed, _ := decimal.NewFromString(value)
	return parsed
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
		{ID: 1, PlanCode: "basic-monthly", PlanName: "Basic Monthly", PlanType: "points_package", PurchaseEnabled: true, Status: "active", PriceCNY: "19.90000", Points: "100.00000", BonusPoints: "0.00000", CreditExpiryEnabled: true, DurationDays: intPointer(30), Currency: "CNY", SortOrder: 1, CreatedAt: now, UpdatedAt: now},
		{ID: 2, PlanCode: "plus-monthly", PlanName: "Plus Monthly", PlanType: "points_package", PurchaseEnabled: true, Status: "active", PriceCNY: "49.90000", Points: "300.00000", BonusPoints: "30.00000", CreditExpiryEnabled: true, DurationDays: intPointer(30), Currency: "CNY", SortOrder: 2, CreatedAt: now, UpdatedAt: now},
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

func effectivePlanDurationDays(expiryEnabled bool, value *int) *int {
	if !expiryEnabled {
		return nil
	}
	if value != nil && *value > 0 {
		return intPointer(*value)
	}
	return intPointer(30)
}

func effectiveCreditExpiryEnabled(value *bool) bool {
	return value == nil || *value
}

func intPointer(value int) *int { return &value }

func normalizeCurrency(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return "CNY"
	}
	return value
}
