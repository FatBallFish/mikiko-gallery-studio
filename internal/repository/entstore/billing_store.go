package entstore

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/pointledger"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type BillingStore struct {
	client *repoent.Client
	scale  int32
}

func NewBillingStore(client *repoent.Client, scale int) *BillingStore {
	scale = 5
	return &BillingStore{client: client, scale: int32(scale)}
}

func (s *BillingStore) GetBalance(ctx context.Context, userID int64) (billingservice.BalanceState, error) {
	state, err := s.currentState(ctx, s.client, userID)
	if err != nil {
		return billingservice.BalanceState{}, err
	}
	return s.formatState(state), nil
}

func (s *BillingStore) ListLedger(ctx context.Context, userID int64, page, pageSize int) (domainbilling.LedgerPage, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	query := s.client.PointLedger.Query().Where(pointledger.UserIDEQ(userID))
	total, err := query.Count(ctx)
	if err != nil {
		return domainbilling.LedgerPage{}, err
	}
	entries, err := query.Order(repoent.Desc(pointledger.FieldID)).Offset((page - 1) * pageSize).Limit(pageSize).All(ctx)
	if err != nil {
		return domainbilling.LedgerPage{}, err
	}
	items := make([]domainbilling.LedgerEntry, 0, len(entries))
	for _, entry := range entries {
		items = append(items, mapLedgerEntry(entry))
	}
	return domainbilling.LedgerPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *BillingStore) ReserveTask(ctx context.Context, req billingservice.ReserveStoreRequest) (billingservice.BalanceState, error) {
	if strings.TrimSpace(req.TaskID) == "" {
		return billingservice.BalanceState{}, errs.BadRequest("task id is required")
	}
	amount, err := decimal.NewFromString(req.EstimatedPoints)
	if err != nil {
		return billingservice.BalanceState{}, err
	}
	if amount.IsNegative() {
		return billingservice.BalanceState{}, errs.BadRequest("estimated points must be non-negative")
	}

	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (billingservice.BalanceState, error) {
		ledgerState, err := s.taskLedgerState(ctx, tx, req.TaskID, req.UserID)
		if err != nil {
			return billingservice.BalanceState{}, err
		}
		if ledgerState.ActiveCycle >= 0 {
			state, err := s.currentState(ctx, tx.Client(), req.UserID)
			if err != nil {
				return billingservice.BalanceState{}, err
			}
			return s.formatState(state), nil
		}
		reserveKey := reserveLedgerKey(req.TaskID, ledgerState.MaxCycle+1)

		state, err := s.currentState(ctx, tx.Client(), req.UserID)
		if err != nil {
			return billingservice.BalanceState{}, err
		}
		if state.Available.LessThan(amount) {
			return billingservice.BalanceState{}, errs.New(400, errs.CodeInsufficientPoints, "insufficient points")
		}

		state.Available = state.Available.Sub(amount)
		state.Frozen = state.Frozen.Add(amount)
		if err := s.insertLedger(ctx, tx, req.UserID, req.TaskID, "reserve", amount.Neg(), state, req.Reason, 0, reserveKey); err != nil {
			return billingservice.BalanceState{}, err
		}
		return s.formatState(state), nil
	})
}

func (s *BillingStore) FinalizeTask(ctx context.Context, req billingservice.FinalizeStoreRequest) (billingservice.BalanceState, error) {
	if strings.TrimSpace(req.TaskID) == "" {
		return billingservice.BalanceState{}, errs.BadRequest("task id is required")
	}
	estimated, err := decimal.NewFromString(req.EstimatedPoints)
	if err != nil {
		return billingservice.BalanceState{}, err
	}
	if estimated.IsNegative() {
		return billingservice.BalanceState{}, errs.BadRequest("estimated points must be non-negative")
	}
	actual, err := decimal.NewFromString(req.ActualPoints)
	if err != nil {
		return billingservice.BalanceState{}, err
	}
	if actual.IsNegative() {
		actual = decimal.Zero
	}

	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (billingservice.BalanceState, error) {
		ledgerState, err := s.taskLedgerState(ctx, tx, req.TaskID, req.UserID)
		if err != nil {
			return billingservice.BalanceState{}, err
		}
		if ledgerState.ActiveCycle < 0 {
			if ledgerState.MaxCycle >= 0 {
				state, err := s.currentState(ctx, tx.Client(), req.UserID)
				if err != nil {
					return billingservice.BalanceState{}, err
				}
				return s.formatState(state), nil
			}
			return billingservice.BalanceState{}, errs.New(409, errs.CodeConflict, "image task points were not reserved")
		}
		reserveEntry, err := tx.PointLedger.Query().
			Where(
				pointledger.UserIDEQ(req.UserID),
				pointledger.IdempotencyKeyEQ(reserveLedgerKey(req.TaskID, ledgerState.ActiveCycle)),
			).
			Only(ctx)
		if err != nil {
			if repoent.IsNotFound(err) {
				return billingservice.BalanceState{}, errs.New(409, errs.CodeConflict, "image task points were not reserved")
			}
			return billingservice.BalanceState{}, err
		}
		reservedAmount, err := decimal.NewFromString(reserveEntry.ChangePoints)
		if err != nil {
			return billingservice.BalanceState{}, err
		}
		reservedAmount = reservedAmount.Abs()
		state, err := s.currentState(ctx, tx.Client(), req.UserID)
		if err != nil {
			return billingservice.BalanceState{}, err
		}
		if state.Frozen.LessThan(reservedAmount) {
			return billingservice.BalanceState{}, errs.New(409, errs.CodeConflict, "reserved image task points are inconsistent")
		}

		if actual.GreaterThan(reservedAmount) {
			actual = reservedAmount
		}
		if actual.IsZero() {
			state.Available = state.Available.Add(reservedAmount)
			state.Frozen = state.Frozen.Sub(reservedAmount)
			if err := s.insertLedger(ctx, tx, req.UserID, req.TaskID, "refund", reservedAmount, state, req.Reason, 0, refundLedgerKey(req.TaskID, ledgerState.ActiveCycle)); err != nil {
				return billingservice.BalanceState{}, err
			}
			return s.formatState(state), nil
		}

		if state.Frozen.LessThan(actual) {
			return billingservice.BalanceState{}, errs.New(409, errs.CodeConflict, "reserved image task points are inconsistent")
		}
		state.Frozen = state.Frozen.Sub(actual)
		if err := s.insertLedger(ctx, tx, req.UserID, req.TaskID, "consume", actual.Neg(), state, req.Reason, 0, consumeLedgerKey(req.TaskID, ledgerState.ActiveCycle)); err != nil {
			return billingservice.BalanceState{}, err
		}

		diff := reservedAmount.Sub(actual)
		if diff.GreaterThan(decimal.Zero) {
			state.Available = state.Available.Add(diff)
			state.Frozen = state.Frozen.Sub(diff)
			if err := s.insertLedger(ctx, tx, req.UserID, req.TaskID, "refund", diff, state, req.Reason, 0, refundLedgerKey(req.TaskID, ledgerState.ActiveCycle)); err != nil {
				return billingservice.BalanceState{}, err
			}
		}
		return s.formatState(state), nil
	})
}

func (s *BillingStore) Adjust(ctx context.Context, req billingservice.AdjustStoreRequest) (billingservice.BalanceState, error) {
	change, err := decimal.NewFromString(req.ChangePoints)
	if err != nil {
		return billingservice.BalanceState{}, err
	}

	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (billingservice.BalanceState, error) {
		state, err := s.currentState(ctx, tx.Client(), req.UserID)
		if err != nil {
			return billingservice.BalanceState{}, err
		}
		nextAvailable := state.Available.Add(change)
		if nextAvailable.IsNegative() {
			return billingservice.BalanceState{}, errs.New(400, errs.CodeInsufficientPoints, "insufficient points")
		}
		state.Available = nextAvailable
		if err := s.insertLedger(ctx, tx, req.UserID, "", "admin_adjust", change, state, req.Reason, req.OperatorAdminID, ""); err != nil {
			return billingservice.BalanceState{}, err
		}
		return s.formatState(state), nil
	})
}

type decimalState struct {
	Available decimal.Decimal
	Frozen    decimal.Decimal
}

type taskLedgerState struct {
	OwnerUserID int64
	MaxCycle    int
	ActiveCycle int
}

func (s *BillingStore) currentState(ctx context.Context, client *repoent.Client, userID int64) (decimalState, error) {
	entry, err := client.PointLedger.Query().Where(pointledger.UserIDEQ(userID)).Order(repoent.Desc(pointledger.FieldID)).First(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return decimalState{Available: decimal.Zero, Frozen: decimal.Zero}, nil
		}
		return decimalState{}, err
	}
	available, err := decimal.NewFromString(entry.BalanceAfter)
	if err != nil {
		return decimalState{}, err
	}
	frozen, err := decimal.NewFromString(entry.FrozenAfter)
	if err != nil {
		return decimalState{}, err
	}
	return decimalState{Available: available, Frozen: frozen}, nil
}

func (s *BillingStore) insertLedger(ctx context.Context, tx *repoent.Tx, userID int64, taskID, ledgerType string, change decimal.Decimal, state decimalState, reason string, operatorAdminID int64, idempotencyKey string) error {
	builder := tx.PointLedger.Create().
		SetUserID(userID).
		SetLedgerType(ledgerType).
		SetChangePoints(change.Round(s.scale).StringFixed(s.scale)).
		SetBalanceAfter(state.Available.Round(s.scale).StringFixed(s.scale)).
		SetFrozenAfter(state.Frozen.Round(s.scale).StringFixed(s.scale)).
		SetReason(reason)
	if strings.TrimSpace(taskID) != "" {
		parsedTaskID, err := uuid.Parse(taskID)
		if err != nil {
			return err
		}
		builder.SetTaskID(parsedTaskID)
	}
	if operatorAdminID > 0 {
		builder.SetOperatorAdminID(operatorAdminID)
	}
	if strings.TrimSpace(idempotencyKey) != "" {
		builder.SetIdempotencyKey(idempotencyKey)
	}
	return builder.Exec(ctx)
}

func (s *BillingStore) formatState(state decimalState) billingservice.BalanceState {
	return billingservice.BalanceState{
		AvailablePoints: state.Available.Round(s.scale).StringFixed(s.scale),
		FrozenPoints:    state.Frozen.Round(s.scale).StringFixed(s.scale),
	}
}

func mapLedgerEntry(entry *repoent.PointLedger) domainbilling.LedgerEntry {
	item := domainbilling.LedgerEntry{
		ID:           int64(entry.ID),
		LedgerType:   entry.LedgerType,
		ChangePoints: entry.ChangePoints,
		BalanceAfter: entry.BalanceAfter,
		FrozenAfter:  entry.FrozenAfter,
		Reason:       entry.Reason,
		CreatedAt:    entry.CreatedAt,
	}
	if entry.TaskID != nil {
		item.TaskID = entry.TaskID.String()
	}
	return item
}

func (s *BillingStore) taskLedgerState(ctx context.Context, tx *repoent.Tx, taskID string, userID int64) (taskLedgerState, error) {
	parsedTaskID, err := uuid.Parse(taskID)
	if err != nil {
		return taskLedgerState{}, err
	}
	entries, err := tx.PointLedger.Query().Where(pointledger.TaskIDEQ(parsedTaskID)).All(ctx)
	if err != nil {
		return taskLedgerState{}, err
	}
	state := taskLedgerState{MaxCycle: -1, ActiveCycle: -1}
	settledCycles := map[int]bool{}
	reservedCycles := map[int]bool{}
	for _, entry := range entries {
		if state.OwnerUserID == 0 {
			state.OwnerUserID = entry.UserID
		}
		if entry.UserID != state.OwnerUserID {
			return taskLedgerState{}, errs.New(409, errs.CodeConflict, "image task points belong to a different user")
		}
		cycle, ledgerKind, ok := parseLedgerCycle(nullableString(entry.IdempotencyKey))
		if !ok {
			continue
		}
		if cycle > state.MaxCycle {
			state.MaxCycle = cycle
		}
		switch ledgerKind {
		case "reserve":
			reservedCycles[cycle] = true
		case "consume", "refund":
			settledCycles[cycle] = true
		}
	}
	for cycle := state.MaxCycle; cycle >= 0; cycle-- {
		if reservedCycles[cycle] && !settledCycles[cycle] {
			state.ActiveCycle = cycle
			break
		}
	}
	if state.OwnerUserID != 0 && state.OwnerUserID != userID {
		return taskLedgerState{}, errs.New(409, errs.CodeConflict, "image task points belong to a different user")
	}
	return state, nil
}

func reserveLedgerKey(taskID string, cycle int) string {
	return fmt.Sprintf("task:%s:reserve:%d", taskID, cycle)
}

func consumeLedgerKey(taskID string, cycle int) string {
	return fmt.Sprintf("task:%s:consume:%d", taskID, cycle)
}

func refundLedgerKey(taskID string, cycle int) string {
	return fmt.Sprintf("task:%s:refund:%d", taskID, cycle)
}

func parseLedgerCycle(key string) (int, string, bool) {
	parts := strings.Split(strings.TrimSpace(key), ":")
	if len(parts) == 4 && parts[0] == "task" {
		cycle, err := strconv.Atoi(parts[3])
		if err != nil {
			return 0, "", false
		}
		return cycle, parts[2], true
	}
	if len(parts) == 3 && parts[0] == "task" {
		return 0, parts[2], true
	}
	return 0, "", false
}

func withSerializableTx[T any](ctx context.Context, client *repoent.Client, fn func(tx *repoent.Tx) (T, error)) (T, error) {
	var zero T
	for attempt := 0; attempt < 5; attempt++ {
		tx, err := client.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
		if err != nil {
			return zero, err
		}
		value, err := fn(tx)
		if err != nil {
			_ = tx.Rollback()
			if isRetryableTxErr(err) && attempt < 4 {
				time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
				continue
			}
			return zero, err
		}
		if err := tx.Commit(); err != nil {
			_ = tx.Rollback()
			if isRetryableTxErr(err) && attempt < 4 {
				time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
				continue
			}
			return zero, err
		}
		return value, nil
	}
	return zero, errs.Internal("serializable billing transaction retry exhausted")
}

func isRetryableTxErr(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "could not serialize access") ||
		strings.Contains(message, "serialization failure") ||
		strings.Contains(message, "deadlock detected") ||
		strings.Contains(message, "database is locked") ||
		strings.Contains(message, "database table is locked") ||
		(repoent.IsConstraintError(err) && strings.Contains(message, "idempotency"))
}
