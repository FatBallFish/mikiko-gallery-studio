package billing

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type BalanceState struct {
	AvailablePoints string
	FrozenPoints    string
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
	}
}

func (s *MemoryStore) GetBalance(_ context.Context, userID int64) (BalanceState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.formatState(s.balances[userID]), nil
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

func (s *MemoryStore) ReserveTask(_ context.Context, req ReserveStoreRequest) (BalanceState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.taskState[req.TaskID]
	if state.Seen && state.UserID != req.UserID {
		return BalanceState{}, errs.New(409, errs.CodeConflict, "image task points belong to a different user")
	}
	if state.Active {
		return s.formatState(s.balances[req.UserID]), nil
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
	return s.formatState(current), nil
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
		return s.formatState(s.balances[req.UserID]), nil
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
		return s.formatState(current), nil
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
	return s.formatState(current), nil
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
			return s.formatState(s.balances[req.UserID]), nil
		}
	}
	current := s.balances[req.UserID]
	next := current.Available.Add(change)
	if next.IsNegative() {
		return BalanceState{}, errs.New(400, errs.CodeInsufficientPoints, "insufficient points")
	}
	current.Available = next
	s.balances[req.UserID] = current
	s.appendLedger(req.UserID, 0, "", "admin_adjust", change, current, req.Reason)
	if idempotencyKey != "" {
		stored := req
		stored.ChangePoints = change.Round(s.scale).StringFixed(s.scale)
		stored.Reason = strings.TrimSpace(req.Reason)
		stored.IdempotencyKey = idempotencyKey
		s.adjustKeys[idempotencyKey] = stored
	}
	return s.formatState(current), nil
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

func (s *MemoryStore) formatState(current balanceState) BalanceState {
	return BalanceState{
		AvailablePoints: current.Available.Round(s.scale).StringFixed(s.scale),
		FrozenPoints:    current.Frozen.Round(s.scale).StringFixed(s.scale),
	}
}
