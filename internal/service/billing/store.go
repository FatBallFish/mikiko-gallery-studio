package billing

import (
	"context"
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
	TaskID          string
	EstimatedPoints string
	Reason          string
}

type FinalizeStoreRequest struct {
	UserID          int64
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
}

type Store interface {
	GetBalance(ctx context.Context, userID int64) (BalanceState, error)
	ListLedger(ctx context.Context, userID int64, page, pageSize int) (domainbilling.LedgerPage, error)
	ReserveTask(ctx context.Context, req ReserveStoreRequest) (BalanceState, error)
	FinalizeTask(ctx context.Context, req FinalizeStoreRequest) (BalanceState, error)
	Adjust(ctx context.Context, req AdjustStoreRequest) (BalanceState, error)
}

type memoryTaskBillingState struct {
	UserID   int64
	Seen     bool
	Active   bool
	Cycle    int
	Reserved decimal.Decimal
}

type MemoryStore struct {
	mu        sync.Mutex
	scale     int32
	nextID    int64
	balances  map[int64]balanceState
	ledgers   map[int64][]domainbilling.LedgerEntry
	taskState map[string]memoryTaskBillingState
}

type balanceState struct {
	Available decimal.Decimal
	Frozen    decimal.Decimal
}

func NewMemoryStore(scale int) *MemoryStore {
	scale = 5
	return &MemoryStore{
		scale:     int32(scale),
		nextID:    1,
		balances:  map[int64]balanceState{},
		ledgers:   map[int64][]domainbilling.LedgerEntry{},
		taskState: map[string]memoryTaskBillingState{},
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
	current.Available = current.Available.Sub(estimated)
	current.Frozen = current.Frozen.Add(estimated)
	s.balances[req.UserID] = current
	s.appendLedger(req.UserID, req.TaskID, "reserve", estimated.Neg(), current, req.Reason)
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
		s.appendLedger(req.UserID, req.TaskID, "refund", reserved, current, req.Reason)
		s.balances[req.UserID] = current
		state.Active = false
		state.Reserved = decimal.Zero
		s.taskState[req.TaskID] = state
		return s.formatState(current), nil
	}
	current.Frozen = current.Frozen.Sub(actual)
	s.appendLedger(req.UserID, req.TaskID, "consume", actual.Neg(), current, req.Reason)
	diff := reserved.Sub(actual)
	if diff.GreaterThan(decimal.Zero) {
		current.Available = current.Available.Add(diff)
		current.Frozen = current.Frozen.Sub(diff)
		s.appendLedger(req.UserID, req.TaskID, "refund", diff, current, req.Reason)
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
	change, err := decimal.NewFromString(req.ChangePoints)
	if err != nil {
		return BalanceState{}, err
	}
	current := s.balances[req.UserID]
	next := current.Available.Add(change)
	if next.IsNegative() {
		return BalanceState{}, errs.New(400, errs.CodeInsufficientPoints, "insufficient points")
	}
	current.Available = next
	s.balances[req.UserID] = current
	s.appendLedger(req.UserID, "", "admin_adjust", change, current, req.Reason)
	return s.formatState(current), nil
}

func (s *MemoryStore) appendLedger(userID int64, taskID, ledgerType string, change decimal.Decimal, current balanceState, reason string) {
	entry := domainbilling.LedgerEntry{
		ID:           s.nextID,
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

func (s *MemoryStore) formatState(current balanceState) BalanceState {
	return BalanceState{
		AvailablePoints: current.Available.Round(s.scale).StringFixed(s.scale),
		FrozenPoints:    current.Frozen.Round(s.scale).StringFixed(s.scale),
	}
}
