package entstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"

	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/pointledger"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/redeemcode"
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

func (s *BillingStore) APIKeyUsage(ctx context.Context, apiKeyID int64, since *time.Time) (string, error) {
	if apiKeyID <= 0 {
		return decimal.Zero.StringFixed(s.scale), nil
	}
	usage, err := s.apiKeyUsage(ctx, s.client, apiKeyID, since)
	if err != nil {
		return "", err
	}
	return usage.Round(s.scale).StringFixed(s.scale), nil
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
		if err := s.checkAPIKeyQuota(ctx, tx.Client(), req, amount); err != nil {
			return billingservice.BalanceState{}, err
		}

		state.Available = state.Available.Sub(amount)
		state.Frozen = state.Frozen.Add(amount)
		if err := s.insertLedger(ctx, tx, req.UserID, req.APIKeyID, req.TaskID, "reserve", amount.Neg(), state, req.Reason, 0, reserveKey); err != nil {
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
			if err := s.insertLedger(ctx, tx, req.UserID, firstPositive(req.APIKeyID, nullableInt64(reserveEntry.APIKeyID)), req.TaskID, "refund", reservedAmount, state, req.Reason, 0, refundLedgerKey(req.TaskID, ledgerState.ActiveCycle)); err != nil {
				return billingservice.BalanceState{}, err
			}
			return s.formatState(state), nil
		}

		if state.Frozen.LessThan(actual) {
			return billingservice.BalanceState{}, errs.New(409, errs.CodeConflict, "reserved image task points are inconsistent")
		}
		state.Frozen = state.Frozen.Sub(actual)
		apiKeyID := firstPositive(req.APIKeyID, nullableInt64(reserveEntry.APIKeyID))
		if err := s.insertLedger(ctx, tx, req.UserID, apiKeyID, req.TaskID, "consume", actual.Neg(), state, req.Reason, 0, consumeLedgerKey(req.TaskID, ledgerState.ActiveCycle)); err != nil {
			return billingservice.BalanceState{}, err
		}

		diff := reservedAmount.Sub(actual)
		if diff.GreaterThan(decimal.Zero) {
			state.Available = state.Available.Add(diff)
			state.Frozen = state.Frozen.Sub(diff)
			if err := s.insertLedger(ctx, tx, req.UserID, apiKeyID, req.TaskID, "refund", diff, state, req.Reason, 0, refundLedgerKey(req.TaskID, ledgerState.ActiveCycle)); err != nil {
				return billingservice.BalanceState{}, err
			}
		}
		return s.formatState(state), nil
	})
}

func (s *BillingStore) Adjust(ctx context.Context, req billingservice.AdjustStoreRequest) (billingservice.BalanceState, error) {
	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	change, err := decimal.NewFromString(req.ChangePoints)
	if err != nil {
		return billingservice.BalanceState{}, err
	}

	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (billingservice.BalanceState, error) {
		if idempotencyKey != "" {
			existing, err := tx.PointLedger.Query().
				Where(pointledger.IdempotencyKeyEQ(adminAdjustLedgerKey(req.UserID, idempotencyKey))).
				Only(ctx)
			if err == nil {
				if existing.UserID != req.UserID {
					return billingservice.BalanceState{}, errs.New(409, errs.CodeConflict, "idempotency key belongs to a different user")
				}
				if existing.ChangePoints != change.Round(s.scale).StringFixed(s.scale) || strings.TrimSpace(existing.Reason) != strings.TrimSpace(req.Reason) || nullableInt64(existing.OperatorAdminID) != req.OperatorAdminID {
					return billingservice.BalanceState{}, errs.New(409, errs.CodeConflict, "idempotency key was already used with a different adjustment")
				}
				state, err := s.currentState(ctx, tx.Client(), req.UserID)
				if err != nil {
					return billingservice.BalanceState{}, err
				}
				return s.formatState(state), nil
			} else if !repoent.IsNotFound(err) {
				return billingservice.BalanceState{}, err
			}
		}
		state, err := s.currentState(ctx, tx.Client(), req.UserID)
		if err != nil {
			return billingservice.BalanceState{}, err
		}
		nextAvailable := state.Available.Add(change)
		if nextAvailable.IsNegative() {
			return billingservice.BalanceState{}, errs.New(400, errs.CodeInsufficientPoints, "insufficient points")
		}
		state.Available = nextAvailable
		if err := s.insertLedger(ctx, tx, req.UserID, 0, "", "admin_adjust", change, state, req.Reason, req.OperatorAdminID, adminAdjustLedgerKey(req.UserID, idempotencyKey)); err != nil {
			return billingservice.BalanceState{}, err
		}
		return s.formatState(state), nil
	})
}

func (s *BillingStore) RedeemCode(ctx context.Context, req billingservice.RedeemCodeRequest) (billingservice.BalanceState, error) {
	code := strings.ToUpper(strings.TrimSpace(req.Code))
	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if req.UserID <= 0 || code == "" || idempotencyKey == "" {
		return billingservice.BalanceState{}, errs.BadRequest("user id, code, and Idempotency-Key are required")
	}

	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (billingservice.BalanceState, error) {
		redeem, err := tx.RedeemCode.Query().Where(redeemcode.CodeEQ(code)).Only(ctx)
		if err != nil {
			if repoent.IsNotFound(err) {
				return billingservice.BalanceState{}, redeemCodeNotFound()
			}
			return billingservice.BalanceState{}, err
		}
		ledgerKey := redeemLedgerKey(redeem.ID, req.UserID, idempotencyKey)
		if _, err := tx.PointLedger.Query().
			Where(
				pointledger.UserIDEQ(req.UserID),
				pointledger.RedeemCodeIDEQ(int64(redeem.ID)),
				pointledger.IdempotencyKeyEQ(ledgerKey),
			).
			Only(ctx); err == nil {
			state, err := s.currentState(ctx, tx.Client(), req.UserID)
			if err != nil {
				return billingservice.BalanceState{}, err
			}
			return s.formatState(state), nil
		} else if !repoent.IsNotFound(err) {
			return billingservice.BalanceState{}, err
		}

		now := time.Now().UTC()
		if redeem.Status != "available" || redeem.RewardType != "points" || now.Before(redeem.ValidFrom) || now.After(redeem.ValidUntil) || redeem.RedeemedCount >= redeem.MaxRedemptions {
			return billingservice.BalanceState{}, redeemCodeNotFound()
		}
		reward, err := decimal.NewFromString(redeem.RewardValue)
		if err != nil || !reward.IsPositive() {
			return billingservice.BalanceState{}, errs.Internal("redeem code reward is invalid")
		}

		state, err := s.currentState(ctx, tx.Client(), req.UserID)
		if err != nil {
			return billingservice.BalanceState{}, err
		}
		state.Available = state.Available.Add(reward)
		if err := s.insertRedeemLedger(ctx, tx, req.UserID, int64(redeem.ID), reward, state, "redeem code "+code, ledgerKey); err != nil {
			return billingservice.BalanceState{}, err
		}
		if err := tx.RedeemCode.UpdateOneID(redeem.ID).
			AddRedeemedCount(1).
			SetLastRedeemedBy(req.UserID).
			Exec(ctx); err != nil {
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

func (s *BillingStore) insertLedger(ctx context.Context, tx *repoent.Tx, userID, apiKeyID int64, taskID, ledgerType string, change decimal.Decimal, state decimalState, reason string, operatorAdminID int64, idempotencyKey string) error {
	builder := tx.PointLedger.Create().
		SetUserID(userID).
		SetLedgerType(ledgerType).
		SetChangePoints(change.Round(s.scale).StringFixed(s.scale)).
		SetBalanceAfter(state.Available.Round(s.scale).StringFixed(s.scale)).
		SetFrozenAfter(state.Frozen.Round(s.scale).StringFixed(s.scale)).
		SetReason(reason)
	if apiKeyID > 0 {
		builder.SetAPIKeyID(apiKeyID)
	}
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

func (s *BillingStore) checkAPIKeyQuota(ctx context.Context, client *repoent.Client, req billingservice.ReserveStoreRequest, amount decimal.Decimal) error {
	if req.APIKeyID <= 0 {
		return nil
	}
	if req.APIKeyTotalQuotaPoints != nil {
		limit, err := decimal.NewFromString(strings.TrimSpace(*req.APIKeyTotalQuotaPoints))
		if err != nil {
			return errs.Internal("invalid api key total quota")
		}
		used, err := s.apiKeyUsage(ctx, client, req.APIKeyID, nil)
		if err != nil {
			return err
		}
		if used.Add(amount).GreaterThan(limit) {
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
		used, err := s.apiKeyUsage(ctx, client, req.APIKeyID, &dayStart)
		if err != nil {
			return err
		}
		if used.Add(amount).GreaterThan(limit) {
			return errs.New(http.StatusTooManyRequests, errs.CodeRateLimited, "api key daily quota exceeded")
		}
	}
	return nil
}

func (s *BillingStore) apiKeyUsage(ctx context.Context, client *repoent.Client, apiKeyID int64, since *time.Time) (decimal.Decimal, error) {
	query := client.PointLedger.Query().
		Where(pointledger.APIKeyIDEQ(apiKeyID), pointledger.LedgerTypeIn("reserve", "refund"))
	if since != nil {
		query = query.Where(pointledger.CreatedAtGTE(*since))
	}
	entries, err := query.All(ctx)
	if err != nil {
		return decimal.Zero, err
	}
	usage := decimal.Zero
	for _, entry := range entries {
		change, err := decimal.NewFromString(entry.ChangePoints)
		if err != nil {
			return decimal.Zero, err
		}
		usage = usage.Sub(change)
	}
	if usage.IsNegative() {
		usage = decimal.Zero
	}
	return usage, nil
}

func (s *BillingStore) insertRedeemLedger(ctx context.Context, tx *repoent.Tx, userID int64, redeemCodeID int64, change decimal.Decimal, state decimalState, reason string, idempotencyKey string) error {
	return tx.PointLedger.Create().
		SetUserID(userID).
		SetRedeemCodeID(redeemCodeID).
		SetLedgerType("redeem").
		SetChangePoints(change.Round(s.scale).StringFixed(s.scale)).
		SetBalanceAfter(state.Available.Round(s.scale).StringFixed(s.scale)).
		SetFrozenAfter(state.Frozen.Round(s.scale).StringFixed(s.scale)).
		SetReason(reason).
		SetIdempotencyKey(idempotencyKey).
		Exec(ctx)
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
		UserID:       entry.UserID,
		LedgerType:   entry.LedgerType,
		ChangePoints: entry.ChangePoints,
		BalanceAfter: entry.BalanceAfter,
		FrozenAfter:  entry.FrozenAfter,
		Reason:       entry.Reason,
		CreatedAt:    entry.CreatedAt,
	}
	if entry.APIKeyID != nil {
		item.APIKeyID = *entry.APIKeyID
	}
	if entry.TaskID != nil {
		item.TaskID = entry.TaskID.String()
	}
	if entry.RedeemCodeID != nil {
		item.RedeemCodeID = *entry.RedeemCodeID
	}
	return item
}

func redeemLedgerKey(redeemCodeID int, userID int64, idempotencyKey string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(idempotencyKey)))
	return fmt.Sprintf("redeem:%d:%d:%x", redeemCodeID, userID, sum)
}

func adminAdjustLedgerKey(userID int64, idempotencyKey string) string {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(idempotencyKey))
	return fmt.Sprintf("admin_adjust:%d:%x", userID, sum)
}

func redeemCodeNotFound() *errs.Error {
	return errs.New(http.StatusNotFound, errs.CodeNotFound, "redeem code not found")
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

func nullableInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func firstPositive(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

const serializableTxMaxAttempts = 25

func withSerializableTx[T any](ctx context.Context, client *repoent.Client, fn func(tx *repoent.Tx) (T, error)) (T, error) {
	var zero T
	for attempt := 0; attempt < serializableTxMaxAttempts; attempt++ {
		tx, err := client.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
		if err != nil {
			retry, retryErr := waitSerializableRetry(ctx, attempt, err)
			if retryErr != nil {
				return zero, retryErr
			}
			if retry {
				continue
			}
			return zero, err
		}

		value, err := fn(tx)
		if err != nil {
			_ = tx.Rollback()
			retry, retryErr := waitSerializableRetry(ctx, attempt, err)
			if retryErr != nil {
				return zero, retryErr
			}
			if retry {
				continue
			}
			return zero, err
		}

		if err := tx.Commit(); err != nil {
			_ = tx.Rollback()
			retry, retryErr := waitSerializableRetry(ctx, attempt, err)
			if retryErr != nil {
				return zero, retryErr
			}
			if retry {
				continue
			}
			return zero, err
		}
		return value, nil
	}
	return zero, errs.Internal("serializable billing transaction retry exhausted")
}

func waitSerializableRetry(ctx context.Context, attempt int, err error) (bool, error) {
	if !isRetryableTxErr(err) {
		return false, nil
	}
	if attempt >= serializableTxMaxAttempts-1 {
		return false, errs.Internal("serializable billing transaction retry exhausted")
	}

	timer := time.NewTimer(serializableTxBackoff(attempt))
	defer timer.Stop()
	select {
	case <-timer.C:
		return true, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func serializableTxBackoff(attempt int) time.Duration {
	delay := time.Duration(attempt+1) * 10 * time.Millisecond
	if delay > 100*time.Millisecond {
		return 100 * time.Millisecond
	}
	return delay
}

func isRetryableTxErr(err error) bool {
	if err == nil {
		return false
	}

	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		switch string(pqErr.Code) {
		case "40001", "40P01":
			return true
		}
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "could not serialize access") ||
		strings.Contains(message, "serialization failure") ||
		strings.Contains(message, "deadlock detected") ||
		strings.Contains(message, "sqlite_busy") ||
		strings.Contains(message, "sqlite_locked") ||
		strings.Contains(message, "database is locked") ||
		strings.Contains(message, "database table is locked") ||
		(repoent.IsConstraintError(err) && strings.Contains(message, "idempotency"))
}
