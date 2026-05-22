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
	SubscriptionPoints string
	GiftPoints         string
	RechargePoints     string
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

type RedeemCodeRequest struct {
	UserID         int64
	Code           string
	IdempotencyKey string
}

type Store interface {
	GetBalance(ctx context.Context, userID int64) (BalanceState, error)
	ListLedger(ctx context.Context, userID int64, page, pageSize int) (domainbilling.LedgerPage, error)
	ListPlans(ctx context.Context) ([]domainbilling.SubscriptionPlan, error)
	GetActiveSubscription(ctx context.Context, userID int64) (*domainbilling.UserSubscriptionSummary, error)
	ListOrders(ctx context.Context, req domainbilling.ListOrdersRequest) (domainbilling.PaymentOrderPage, error)
	GetOrder(ctx context.Context, userID int64, orderID int64) (domainbilling.PaymentOrder, error)
	CreateOrder(ctx context.Context, req domainbilling.CreateOrderRequest) (domainbilling.PaymentOrder, error)
	CancelOrder(ctx context.Context, userID int64, orderID int64) (domainbilling.PaymentOrder, error)
	MarkOrderPaid(ctx context.Context, req domainbilling.MarkOrderPaidRequest) (domainbilling.PaymentOrder, error)
	ReserveTask(ctx context.Context, req ReserveStoreRequest) (BalanceState, error)
	FinalizeTask(ctx context.Context, req FinalizeStoreRequest) (BalanceState, error)
	Adjust(ctx context.Context, req AdjustStoreRequest) (BalanceState, error)
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
	mu          sync.Mutex
	scale       int32
	nextID      int64
	balances    map[int64]balanceState
	ledgers     map[int64][]domainbilling.LedgerEntry
	apiKeyUsage map[int64]map[string]decimal.Decimal
	taskState   map[string]memoryTaskBillingState
	adjustKeys  map[string]AdjustStoreRequest
	plans       []domainbilling.SubscriptionPlan
	orders      map[int64]domainbilling.PaymentOrder
	nextOrderID int64
	breakdown   map[int64]memoryBreakdown
	subs        map[int64]*domainbilling.UserSubscriptionSummary
}

type memoryBreakdown struct {
	Subscription decimal.Decimal
	Gift         decimal.Decimal
	Recharge     decimal.Decimal
}

type balanceState struct {
	Available decimal.Decimal
	Frozen    decimal.Decimal
}

func NewMemoryStore(scale int) *MemoryStore {
	scale = 5
	return &MemoryStore{
		scale:       int32(scale),
		nextID:      1,
		balances:    map[int64]balanceState{},
		ledgers:     map[int64][]domainbilling.LedgerEntry{},
		apiKeyUsage: map[int64]map[string]decimal.Decimal{},
		taskState:   map[string]memoryTaskBillingState{},
		adjustKeys:  map[string]AdjustStoreRequest{},
		plans:       defaultPlans(),
		orders:      map[int64]domainbilling.PaymentOrder{},
		nextOrderID: 1,
		breakdown:   map[int64]memoryBreakdown{},
		subs:        map[int64]*domainbilling.UserSubscriptionSummary{},
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
		if order.UserID == req.UserID {
			items = append(items, order)
		}
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

func (s *MemoryStore) GetOrder(_ context.Context, userID int64, orderID int64) (domainbilling.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[orderID]
	if !ok || order.UserID != userID {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
	}
	return order, nil
}

func (s *MemoryStore) CreateOrder(_ context.Context, req domainbilling.CreateOrderRequest) (domainbilling.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	now := time.Now().UTC()
	order := domainbilling.PaymentOrder{
		ID:          s.nextOrderID,
		OrderNo:     fmt.Sprintf("PGO-%06d", s.nextOrderID),
		UserID:      req.UserID,
		PlanID:      plan.ID,
		PlanCode:    plan.PlanCode,
		PlanName:    plan.PlanName,
		Provider:    strings.TrimSpace(req.Provider),
		Status:      "pending",
		Currency:    plan.Currency,
		AmountCNY:   plan.PriceCNY,
		Points:      plan.Points,
		BonusPoints: plan.BonusPoints,
		PaymentURL:  "mock://checkout/" + fmt.Sprintf("PGO-%06d", s.nextOrderID),
		ExpiresAt:   now.Add(15 * time.Minute),
		CreatedAt:   now,
		UpdatedAt:   now,
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
	order.Status = "closed"
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
		deduct := change.Abs()
		if breakdown.Gift.GreaterThanOrEqual(deduct) {
			breakdown.Gift = breakdown.Gift.Sub(deduct)
		} else {
			breakdown.Gift = decimal.Zero
		}
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

func (s *MemoryStore) appendLedger(userID, apiKeyID int64, taskID, ledgerType string, change decimal.Decimal, current balanceState, reason string) {
	entry := domainbilling.LedgerEntry{
		ID:           s.nextID,
		APIKeyID:     apiKeyID,
		TaskID:       taskID,
		LedgerType:   ledgerType,
		ChangePoints: change.Round(s.scale).StringFixed(s.scale),
		BalanceAfter: current.Available.Round(s.scale).StringFixed(s.scale),
		FrozenAfter:  current.Frozen.Round(s.scale).StringFixed(s.scale),
		Reason:       reason,
		CreatedAt:    time.Now().UTC(),
	}
	s.nextID++
	s.ledgers[userID] = append([]domainbilling.LedgerEntry{entry}, s.ledgers[userID]...)
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
	return BalanceState{
		AvailablePoints:    current.Available.Round(s.scale).StringFixed(s.scale),
		FrozenPoints:       current.Frozen.Round(s.scale).StringFixed(s.scale),
		SubscriptionPoints: breakdown.Subscription.Round(s.scale).StringFixed(s.scale),
		GiftPoints:         breakdown.Gift.Round(s.scale).StringFixed(s.scale),
		RechargePoints:     breakdown.Recharge.Round(s.scale).StringFixed(s.scale),
		ActiveSubscription: s.subs[userID],
	}
}

func defaultPlans() []domainbilling.SubscriptionPlan {
	now := time.Now().UTC()
	return []domainbilling.SubscriptionPlan{
		{ID: 1, PlanCode: "basic-monthly", PlanName: "Basic Monthly", Status: "active", PriceCNY: "19.90000", Points: "100.00000", BonusPoints: "0.00000", DurationDays: 30, Currency: "CNY", CreatedAt: now, UpdatedAt: now},
		{ID: 2, PlanCode: "plus-monthly", PlanName: "Plus Monthly", Status: "active", PriceCNY: "49.90000", Points: "300.00000", BonusPoints: "30.00000", DurationDays: 30, Currency: "CNY", CreatedAt: now, UpdatedAt: now},
	}
}
