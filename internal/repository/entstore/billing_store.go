package entstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"

	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/paymentorder"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/paymentwebhookevent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/pointledger"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/redeemcode"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/subscriptionplan"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/usersubscription"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/walletgrant"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/walletreservationallocation"
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

func (s *BillingStore) ListPlans(ctx context.Context) ([]domainbilling.SubscriptionPlan, error) {
	if err := s.ensureDefaultPlans(ctx); err != nil {
		return nil, err
	}
	plans, err := s.client.SubscriptionPlan.Query().
		Where(subscriptionplan.StatusEQ("active")).
		Order(repoent.Asc(subscriptionplan.FieldSortOrder), repoent.Asc(subscriptionplan.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]domainbilling.SubscriptionPlan, 0, len(plans))
	for _, plan := range plans {
		items = append(items, mapSubscriptionPlan(plan))
	}
	return items, nil
}

func (s *BillingStore) GetActiveSubscription(ctx context.Context, userID int64) (*domainbilling.UserSubscriptionSummary, error) {
	subscription, err := s.client.UserSubscription.Query().
		Where(
			usersubscription.UserIDEQ(userID),
			usersubscription.StatusEQ("active"),
		).
		Order(repoent.Desc(usersubscription.FieldCurrentPeriodEnd)).
		First(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return s.mapActiveSubscription(ctx, subscription), nil
}

func (s *BillingStore) ListOrders(ctx context.Context, req domainbilling.ListOrdersRequest) (domainbilling.PaymentOrderPage, error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	query := s.client.PaymentOrder.Query().Where(paymentorder.UserIDEQ(req.UserID))
	total, err := query.Count(ctx)
	if err != nil {
		return domainbilling.PaymentOrderPage{}, err
	}
	orders, err := query.Order(repoent.Desc(paymentorder.FieldID)).Offset((req.Page - 1) * req.PageSize).Limit(req.PageSize).All(ctx)
	if err != nil {
		return domainbilling.PaymentOrderPage{}, err
	}
	items := make([]domainbilling.PaymentOrder, 0, len(orders))
	for _, order := range orders {
		items = append(items, s.mapPaymentOrder(ctx, order))
	}
	return domainbilling.PaymentOrderPage{Items: items, Page: req.Page, PageSize: req.PageSize, Total: total}, nil
}

func (s *BillingStore) GetOrder(ctx context.Context, userID int64, orderID int64) (domainbilling.PaymentOrder, error) {
	order, err := s.client.PaymentOrder.Query().
		Where(paymentorder.IDEQ(int(orderID)), paymentorder.UserIDEQ(userID)).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
		}
		return domainbilling.PaymentOrder{}, err
	}
	return s.mapPaymentOrder(ctx, order), nil
}

func (s *BillingStore) CreateOrder(ctx context.Context, req domainbilling.CreateOrderRequest) (domainbilling.PaymentOrder, error) {
	if err := s.ensureDefaultPlans(ctx); err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	plan, err := s.client.SubscriptionPlan.Query().
		Where(subscriptionplan.PlanCodeEQ(strings.TrimSpace(req.PlanCode)), subscriptionplan.StatusEQ("active")).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "subscription plan not found")
		}
		return domainbilling.PaymentOrder{}, err
	}
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider != "alipay" && provider != "wxpay" {
		return domainbilling.PaymentOrder{}, errs.BadRequest("provider must be alipay or wxpay")
	}
	now := time.Now().UTC()
	orderNo := fmt.Sprintf("PGO-%d-%06d", now.Unix(), time.Now().Nanosecond()%1000000)
	order, err := s.client.PaymentOrder.Create().
		SetUserID(req.UserID).
		SetPlanID(int64(plan.ID)).
		SetOrderNo(orderNo).
		SetProvider(provider).
		SetStatus("pending").
		SetCurrency(plan.Currency).
		SetAmountCny(plan.PriceCny).
		SetPoints(plan.Points).
		SetBonusPoints(plan.BonusPoints).
		SetPaymentURL("mock://checkout/" + orderNo).
		SetExpiresAt(now.Add(15 * time.Minute)).
		SetProviderPayload(map[string]any{"provider": provider, "order_no": orderNo}).
		Save(ctx)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	return s.mapPaymentOrder(ctx, order), nil
}

func (s *BillingStore) CancelOrder(ctx context.Context, userID int64, orderID int64) (domainbilling.PaymentOrder, error) {
	order, err := s.client.PaymentOrder.Query().
		Where(paymentorder.IDEQ(int(orderID)), paymentorder.UserIDEQ(userID)).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
		}
		return domainbilling.PaymentOrder{}, err
	}
	if order.Status != "pending" {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order cannot be canceled")
	}
	now := time.Now().UTC()
	order, err = s.client.PaymentOrder.UpdateOneID(order.ID).
		SetStatus("closed").
		SetClosedAt(now).
		Save(ctx)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	return s.mapPaymentOrder(ctx, order), nil
}

func (s *BillingStore) MarkOrderPaid(ctx context.Context, req domainbilling.MarkOrderPaidRequest) (domainbilling.PaymentOrder, error) {
	orderNo := strings.TrimSpace(req.OrderNo)
	tradeNo := strings.TrimSpace(req.TradeNo)
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if orderNo == "" || tradeNo == "" || provider == "" {
		return domainbilling.PaymentOrder{}, errs.BadRequest("provider, order_no, and trade_no are required")
	}
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (domainbilling.PaymentOrder, error) {
		order, err := tx.PaymentOrder.Query().Where(paymentorder.OrderNoEQ(orderNo)).Only(ctx)
		if err != nil {
			if repoent.IsNotFound(err) {
				return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
			}
			return domainbilling.PaymentOrder{}, err
		}
		if order.Status == "paid" {
			if err := s.ensureWebhookEvent(ctx, tx, provider, tradeNo, int64(order.ID), map[string]any{"order_no": orderNo}); err != nil {
				return domainbilling.PaymentOrder{}, err
			}
			return s.mapPaymentOrder(ctx, order), nil
		}
		if order.Status != "pending" {
			return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order cannot transition to paid")
		}
		now := time.Now().UTC()
		if _, err := tx.PaymentOrder.UpdateOneID(order.ID).
			SetStatus("paid").
			SetTradeNo(tradeNo).
			SetPaidAt(now).
			Save(ctx); err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		if err := s.ensureWebhookEvent(ctx, tx, provider, tradeNo, int64(order.ID), map[string]any{"order_no": orderNo}); err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		if _, err := s.grantOrderCredits(ctx, tx, order); err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		updated, err := tx.PaymentOrder.Query().Where(paymentorder.IDEQ(order.ID)).Only(ctx)
		if err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		return s.mapPaymentOrder(ctx, updated), nil
	})
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
			_, state, err := s.currentStateWithDetails(ctx, tx.Client(), req.UserID)
			if err != nil {
				return billingservice.BalanceState{}, err
			}
			return state, nil
		}
		reserveKey := reserveLedgerKey(req.TaskID, ledgerState.MaxCycle+1)

		state, _, err := s.currentStateWithDetails(ctx, tx.Client(), req.UserID)
		if err != nil {
			return billingservice.BalanceState{}, err
		}
		if state.Available.LessThan(amount) {
			return billingservice.BalanceState{}, errs.New(400, errs.CodeInsufficientPoints, "insufficient points")
		}
		if err := s.checkAPIKeyQuota(ctx, tx.Client(), req, amount); err != nil {
			return billingservice.BalanceState{}, err
		}
		if _, err := s.reserveAcrossGrants(ctx, tx, req.UserID, req.TaskID, ledgerState.MaxCycle+1, amount); err != nil {
			return billingservice.BalanceState{}, err
		}
		state, summary, err := s.currentStateWithDetails(ctx, tx.Client(), req.UserID)
		if err != nil {
			return billingservice.BalanceState{}, err
		}
		if err := s.insertLedger(ctx, tx, req.UserID, req.APIKeyID, req.TaskID, "reserve", amount.Neg(), state, req.Reason, 0, reserveKey); err != nil {
			return billingservice.BalanceState{}, err
		}
		return summary, nil
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
				_, state, err := s.currentStateWithDetails(ctx, tx.Client(), req.UserID)
				if err != nil {
					return billingservice.BalanceState{}, err
				}
				return state, nil
			}
			return billingservice.BalanceState{}, errs.New(409, errs.CodeConflict, "image task points were not reserved")
		}
		taskUUID, err := uuid.Parse(req.TaskID)
		if err != nil {
			return billingservice.BalanceState{}, err
		}
		allocations, err := tx.WalletReservationAllocation.Query().
			Where(
				walletreservationallocation.UserIDEQ(req.UserID),
				walletreservationallocation.TaskIDEQ(taskUUID),
				walletreservationallocation.ReservationCycleEQ(ledgerState.ActiveCycle),
				walletreservationallocation.StatusEQ("reserved"),
			).
			Order(repoent.Asc(walletreservationallocation.FieldID)).
			All(ctx)
		if err != nil {
			return billingservice.BalanceState{}, err
		}
		if len(allocations) == 0 {
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
			if !reservedAmount.Abs().IsZero() {
				return billingservice.BalanceState{}, errs.New(409, errs.CodeConflict, "image task points were not reserved")
			}
			state, summary, err := s.currentStateWithDetails(ctx, tx.Client(), req.UserID)
			if err != nil {
				return billingservice.BalanceState{}, err
			}
			if err := s.insertLedger(ctx, tx, req.UserID, req.APIKeyID, req.TaskID, "refund", decimal.Zero, state, req.Reason, 0, refundLedgerKey(req.TaskID, ledgerState.ActiveCycle)); err != nil {
				return billingservice.BalanceState{}, err
			}
			return summary, nil
		}
		reservedAmount := decimal.Zero
		for _, allocation := range allocations {
			value, parseErr := decimal.NewFromString(allocation.ReservedPoints)
			if parseErr != nil {
				return billingservice.BalanceState{}, parseErr
			}
			reservedAmount = reservedAmount.Add(value)
		}
		state, _, err := s.currentStateWithDetails(ctx, tx.Client(), req.UserID)
		if err != nil {
			return billingservice.BalanceState{}, err
		}

		if actual.GreaterThan(reservedAmount) {
			actual = reservedAmount
		}
		if actual.IsZero() {
			if err := s.settleAllocations(ctx, tx, allocations, decimal.Zero); err != nil {
				return billingservice.BalanceState{}, err
			}
			state, summary, err := s.currentStateWithDetails(ctx, tx.Client(), req.UserID)
			if err != nil {
				return billingservice.BalanceState{}, err
			}
			if err := s.insertLedger(ctx, tx, req.UserID, req.APIKeyID, req.TaskID, "refund", reservedAmount, state, req.Reason, 0, refundLedgerKey(req.TaskID, ledgerState.ActiveCycle)); err != nil {
				return billingservice.BalanceState{}, err
			}
			return summary, nil
		}

		if err := s.settleAllocations(ctx, tx, allocations, actual); err != nil {
			return billingservice.BalanceState{}, err
		}
		diff := reservedAmount.Sub(actual)
		state, summary, err := s.currentStateWithDetails(ctx, tx.Client(), req.UserID)
		if err != nil {
			return billingservice.BalanceState{}, err
		}
		apiKeyID := req.APIKeyID
		if diff.GreaterThan(decimal.Zero) {
			refundState := decimalState{Available: state.Available, Frozen: state.Frozen}
			if err := s.insertLedger(ctx, tx, req.UserID, apiKeyID, req.TaskID, "consume", actual.Neg(), refundState, req.Reason, 0, consumeLedgerKey(req.TaskID, ledgerState.ActiveCycle)); err != nil {
				return billingservice.BalanceState{}, err
			}
			if err := s.insertLedger(ctx, tx, req.UserID, apiKeyID, req.TaskID, "refund", diff, refundState, req.Reason, 0, refundLedgerKey(req.TaskID, ledgerState.ActiveCycle)); err != nil {
				return billingservice.BalanceState{}, err
			}
			return summary, nil
		}
		if err := s.insertLedger(ctx, tx, req.UserID, apiKeyID, req.TaskID, "consume", actual.Neg(), state, req.Reason, 0, consumeLedgerKey(req.TaskID, ledgerState.ActiveCycle)); err != nil {
			return billingservice.BalanceState{}, err
		}
		return summary, nil
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
				_, state, err := s.currentStateWithDetails(ctx, tx.Client(), req.UserID)
				if err != nil {
					return billingservice.BalanceState{}, err
				}
				return state, nil
			} else if !repoent.IsNotFound(err) {
				return billingservice.BalanceState{}, err
			}
		}
		state, _, err := s.currentStateWithDetails(ctx, tx.Client(), req.UserID)
		if err != nil {
			return billingservice.BalanceState{}, err
		}
		if change.IsPositive() {
			if _, err := s.createWalletGrant(ctx, tx, req.UserID, "gift", "admin_adjust", nil, change, nil, map[string]any{"reason": req.Reason}); err != nil {
				return billingservice.BalanceState{}, err
			}
		} else if change.IsNegative() {
			if err := s.deductAvailableGrants(ctx, tx, req.UserID, change.Abs()); err != nil {
				return billingservice.BalanceState{}, err
			}
		}
		state, summary, err := s.currentStateWithDetails(ctx, tx.Client(), req.UserID)
		if err != nil {
			return billingservice.BalanceState{}, err
		}
		if err := s.insertLedger(ctx, tx, req.UserID, 0, "", "admin_adjust", change, state, req.Reason, req.OperatorAdminID, adminAdjustLedgerKey(req.UserID, idempotencyKey)); err != nil {
			return billingservice.BalanceState{}, err
		}
		return summary, nil
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
			_, state, err := s.currentStateWithDetails(ctx, tx.Client(), req.UserID)
			if err != nil {
				return billingservice.BalanceState{}, err
			}
			return state, nil
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

		if _, err := s.createWalletGrant(ctx, tx, req.UserID, "gift", "redeem_code", pointerInt64(int64(redeem.ID)), reward, &redeem.ValidUntil, map[string]any{"code": code}); err != nil {
			return billingservice.BalanceState{}, err
		}
		state, summary, err := s.currentStateWithDetails(ctx, tx.Client(), req.UserID)
		if err != nil {
			return billingservice.BalanceState{}, err
		}
		if err := s.insertRedeemLedger(ctx, tx, req.UserID, int64(redeem.ID), reward, state, "redeem code "+code, ledgerKey); err != nil {
			return billingservice.BalanceState{}, err
		}
		if err := tx.RedeemCode.UpdateOneID(redeem.ID).
			AddRedeemedCount(1).
			SetLastRedeemedBy(req.UserID).
			Exec(ctx); err != nil {
			return billingservice.BalanceState{}, err
		}
		return summary, nil
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
	state, _, err := s.currentStateWithDetails(ctx, client, userID)
	return state, err
}

func (s *BillingStore) currentStateWithDetails(ctx context.Context, client *repoent.Client, userID int64) (decimalState, billingservice.BalanceState, error) {
	grants, err := client.WalletGrant.Query().Where(walletgrant.UserIDEQ(userID), walletgrant.StatusEQ("active")).All(ctx)
	if err != nil {
		return decimalState{}, billingservice.BalanceState{}, err
	}
	if len(grants) == 0 {
		entry, err := client.PointLedger.Query().Where(pointledger.UserIDEQ(userID)).Order(repoent.Desc(pointledger.FieldID)).First(ctx)
		if err != nil {
			if repoent.IsNotFound(err) {
				zero := decimalState{Available: decimal.Zero, Frozen: decimal.Zero}
				return zero, s.formatDetailedState(zero, nil, nil, nil), nil
			}
			return decimalState{}, billingservice.BalanceState{}, err
		}
		available, err := decimal.NewFromString(entry.BalanceAfter)
		if err != nil {
			return decimalState{}, billingservice.BalanceState{}, err
		}
		frozen, err := decimal.NewFromString(entry.FrozenAfter)
		if err != nil {
			return decimalState{}, billingservice.BalanceState{}, err
		}
		state := decimalState{Available: available, Frozen: frozen}
		return state, s.formatDetailedState(state, nil, nil, nil), nil
	}
	state := decimalState{Available: decimal.Zero, Frozen: decimal.Zero}
	subscriptionAvailable := decimal.Zero
	giftAvailable := decimal.Zero
	rechargeAvailable := decimal.Zero
	var nextGrant *domainbilling.GrantExpirySummary
	now := time.Now().UTC()
	for _, grant := range grants {
		available, err := decimal.NewFromString(grant.AvailablePoints)
		if err != nil {
			return decimalState{}, billingservice.BalanceState{}, err
		}
		frozen, err := decimal.NewFromString(grant.FrozenPoints)
		if err != nil {
			return decimalState{}, billingservice.BalanceState{}, err
		}
		if grant.ExpiresAt != nil && now.After(*grant.ExpiresAt) {
			continue
		}
		state.Available = state.Available.Add(available)
		state.Frozen = state.Frozen.Add(frozen)
		switch grant.GrantType {
		case "subscription":
			subscriptionAvailable = subscriptionAvailable.Add(available)
		case "recharge":
			rechargeAvailable = rechargeAvailable.Add(available)
		default:
			giftAvailable = giftAvailable.Add(available)
		}
		if grant.ExpiresAt != nil && available.GreaterThan(decimal.Zero) {
			if nextGrant == nil || grant.ExpiresAt.Before(*nextGrant.ExpiresAt) {
				expiresAt := *grant.ExpiresAt
				nextGrant = &domainbilling.GrantExpirySummary{
					GrantID:         int64(grant.ID),
					GrantType:       grant.GrantType,
					AvailablePoints: available.Round(s.scale).StringFixed(s.scale),
					ExpiresAt:       &expiresAt,
				}
			}
		}
	}
	activeSubscription, err := s.GetActiveSubscription(ctx, userID)
	if err != nil {
		return decimalState{}, billingservice.BalanceState{}, err
	}
	return state, billingservice.BalanceState{
		AvailablePoints:    state.Available.Round(s.scale).StringFixed(s.scale),
		FrozenPoints:       state.Frozen.Round(s.scale).StringFixed(s.scale),
		SubscriptionPoints: subscriptionAvailable.Round(s.scale).StringFixed(s.scale),
		GiftPoints:         giftAvailable.Round(s.scale).StringFixed(s.scale),
		RechargePoints:     rechargeAvailable.Round(s.scale).StringFixed(s.scale),
		ActiveSubscription: activeSubscription,
		NextExpiringGrant:  nextGrant,
	}, nil
}

func (s *BillingStore) GetBalance(ctx context.Context, userID int64) (billingservice.BalanceState, error) {
	_, state, err := s.currentStateWithDetails(ctx, s.client, userID)
	if err != nil {
		return billingservice.BalanceState{}, err
	}
	return state, nil
}

func (s *BillingStore) formatDetailedState(current decimalState, subscription *decimal.Decimal, gift *decimal.Decimal, recharge *decimal.Decimal) billingservice.BalanceState {
	subscriptionValue := decimal.Zero
	giftValue := decimal.Zero
	rechargeValue := current.Available
	if subscription != nil {
		subscriptionValue = *subscription
	}
	if gift != nil {
		giftValue = *gift
	}
	if recharge != nil {
		rechargeValue = *recharge
	}
	return billingservice.BalanceState{
		AvailablePoints:    current.Available.Round(s.scale).StringFixed(s.scale),
		FrozenPoints:       current.Frozen.Round(s.scale).StringFixed(s.scale),
		SubscriptionPoints: subscriptionValue.Round(s.scale).StringFixed(s.scale),
		GiftPoints:         giftValue.Round(s.scale).StringFixed(s.scale),
		RechargePoints:     rechargeValue.Round(s.scale).StringFixed(s.scale),
	}
}

func (s *BillingStore) currentStateLegacy(ctx context.Context, client *repoent.Client, userID int64) (decimalState, error) {
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
		AvailablePoints:    state.Available.Round(s.scale).StringFixed(s.scale),
		FrozenPoints:       state.Frozen.Round(s.scale).StringFixed(s.scale),
		SubscriptionPoints: decimal.Zero.Round(s.scale).StringFixed(s.scale),
		GiftPoints:         decimal.Zero.Round(s.scale).StringFixed(s.scale),
		RechargePoints:     state.Available.Round(s.scale).StringFixed(s.scale),
	}
}

func (s *BillingStore) ensureDefaultPlans(ctx context.Context) error {
	count, err := s.client.SubscriptionPlan.Query().Count(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	now := time.Now().UTC()
	defaults := []domainbilling.SubscriptionPlan{
		{PlanCode: "basic-monthly", PlanName: "Basic Monthly", Status: "active", PriceCNY: "19.90000", Points: "100.00000", BonusPoints: "0.00000", DurationDays: 30, Currency: "CNY", CreatedAt: now, UpdatedAt: now},
		{PlanCode: "plus-monthly", PlanName: "Plus Monthly", Status: "active", PriceCNY: "49.90000", Points: "300.00000", BonusPoints: "30.00000", DurationDays: 30, Currency: "CNY", CreatedAt: now, UpdatedAt: now},
	}
	for index, item := range defaults {
		if _, err := s.client.SubscriptionPlan.Create().
			SetPlanCode(item.PlanCode).
			SetPlanName(item.PlanName).
			SetStatus(item.Status).
			SetPriceCny(item.PriceCNY).
			SetPoints(item.Points).
			SetBonusPoints(item.BonusPoints).
			SetDurationDays(item.DurationDays).
			SetCurrency(item.Currency).
			SetSortOrder(index + 1).
			Save(ctx); err != nil && !repoent.IsConstraintError(err) {
			return err
		}
	}
	return nil
}

func (s *BillingStore) reserveAcrossGrants(ctx context.Context, tx *repoent.Tx, userID int64, taskID string, cycle int, amount decimal.Decimal) ([]*repoent.WalletReservationAllocation, error) {
	taskUUID, err := uuid.Parse(taskID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureLegacyGrant(ctx, tx, userID); err != nil {
		return nil, err
	}
	grants, err := s.eligibleGrants(ctx, tx.Client(), userID)
	if err != nil {
		return nil, err
	}
	remaining := amount
	allocations := make([]*repoent.WalletReservationAllocation, 0)
	for _, grant := range grants {
		if !remaining.IsPositive() {
			break
		}
		available, err := decimal.NewFromString(grant.AvailablePoints)
		if err != nil {
			return nil, err
		}
		if !available.IsPositive() {
			continue
		}
		take := available
		if take.GreaterThan(remaining) {
			take = remaining
		}
		remaining = remaining.Sub(take)
		newAvailable := available.Sub(take)
		frozen, err := decimal.NewFromString(grant.FrozenPoints)
		if err != nil {
			return nil, err
		}
		if _, err := tx.WalletGrant.UpdateOneID(grant.ID).
			SetAvailablePoints(newAvailable.Round(s.scale).StringFixed(s.scale)).
			SetFrozenPoints(frozen.Add(take).Round(s.scale).StringFixed(s.scale)).
			Save(ctx); err != nil {
			return nil, err
		}
		allocation, err := tx.WalletReservationAllocation.Create().
			SetUserID(userID).
			SetWalletGrantID(int64(grant.ID)).
			SetTaskID(taskUUID).
			SetReservationCycle(cycle).
			SetStatus("reserved").
			SetReservedPoints(take.Round(s.scale).StringFixed(s.scale)).
			Save(ctx)
		if err != nil {
			return nil, err
		}
		allocations = append(allocations, allocation)
	}
	if remaining.IsPositive() {
		return nil, errs.New(http.StatusBadRequest, errs.CodeInsufficientPoints, "insufficient points")
	}
	return allocations, nil
}

func (s *BillingStore) settleAllocations(ctx context.Context, tx *repoent.Tx, allocations []*repoent.WalletReservationAllocation, actual decimal.Decimal) error {
	remainingActual := actual
	for _, allocation := range allocations {
		grant, err := tx.WalletGrant.Query().Where(walletgrant.IDEQ(int(allocation.WalletGrantID))).Only(ctx)
		if err != nil {
			return err
		}
		reserved, err := decimal.NewFromString(allocation.ReservedPoints)
		if err != nil {
			return err
		}
		consume := decimal.Zero
		refund := reserved
		if remainingActual.IsPositive() {
			consume = reserved
			if consume.GreaterThan(remainingActual) {
				consume = remainingActual
			}
			refund = reserved.Sub(consume)
			remainingActual = remainingActual.Sub(consume)
		}
		available, err := decimal.NewFromString(grant.AvailablePoints)
		if err != nil {
			return err
		}
		frozen, err := decimal.NewFromString(grant.FrozenPoints)
		if err != nil {
			return err
		}
		consumed, err := decimal.NewFromString(grant.ConsumedPoints)
		if err != nil {
			return err
		}
		nextAvailable := available.Add(refund)
		nextFrozen := frozen.Sub(reserved)
		if nextFrozen.IsNegative() {
			nextFrozen = decimal.Zero
		}
		nextConsumed := consumed.Add(consume)
		if _, err := tx.WalletGrant.UpdateOneID(grant.ID).
			SetAvailablePoints(nextAvailable.Round(s.scale).StringFixed(s.scale)).
			SetFrozenPoints(nextFrozen.Round(s.scale).StringFixed(s.scale)).
			SetConsumedPoints(nextConsumed.Round(s.scale).StringFixed(s.scale)).
			Save(ctx); err != nil {
			return err
		}
		status := "refunded"
		if refund.IsZero() {
			status = "consumed"
		} else if consume.IsPositive() {
			status = "settled"
		}
		if _, err := tx.WalletReservationAllocation.UpdateOneID(allocation.ID).
			SetConsumedPoints(consume.Round(s.scale).StringFixed(s.scale)).
			SetRefundedPoints(refund.Round(s.scale).StringFixed(s.scale)).
			SetStatus(status).
			Save(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *BillingStore) eligibleGrants(ctx context.Context, client *repoent.Client, userID int64) ([]*repoent.WalletGrant, error) {
	now := time.Now().UTC()
	grants, err := client.WalletGrant.Query().Where(walletgrant.UserIDEQ(userID), walletgrant.StatusEQ("active")).All(ctx)
	if err != nil {
		return nil, err
	}
	filtered := make([]*repoent.WalletGrant, 0, len(grants))
	for _, grant := range grants {
		available, err := decimal.NewFromString(grant.AvailablePoints)
		if err != nil {
			return nil, err
		}
		if !available.IsPositive() {
			continue
		}
		if grant.ExpiresAt != nil && now.After(*grant.ExpiresAt) {
			continue
		}
		filtered = append(filtered, grant)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if grantPriority(filtered[i].GrantType) != grantPriority(filtered[j].GrantType) {
			return grantPriority(filtered[i].GrantType) < grantPriority(filtered[j].GrantType)
		}
		if filtered[i].ExpiresAt == nil {
			return false
		}
		if filtered[j].ExpiresAt == nil {
			return true
		}
		if !filtered[i].ExpiresAt.Equal(*filtered[j].ExpiresAt) {
			return filtered[i].ExpiresAt.Before(*filtered[j].ExpiresAt)
		}
		return filtered[i].ID < filtered[j].ID
	})
	return filtered, nil
}

func (s *BillingStore) deductAvailableGrants(ctx context.Context, tx *repoent.Tx, userID int64, amount decimal.Decimal) error {
	if err := s.ensureLegacyGrant(ctx, tx, userID); err != nil {
		return err
	}
	grants, err := tx.WalletGrant.Query().Where(walletgrant.UserIDEQ(userID), walletgrant.StatusEQ("active")).All(ctx)
	if err != nil {
		return err
	}
	sort.SliceStable(grants, func(i, j int) bool {
		if adminDeductionPriority(grants[i].GrantType) != adminDeductionPriority(grants[j].GrantType) {
			return adminDeductionPriority(grants[i].GrantType) < adminDeductionPriority(grants[j].GrantType)
		}
		return grants[i].ID < grants[j].ID
	})
	remaining := amount
	for _, grant := range grants {
		if !remaining.IsPositive() {
			break
		}
		available, err := decimal.NewFromString(grant.AvailablePoints)
		if err != nil {
			return err
		}
		if !available.IsPositive() {
			continue
		}
		deduct := available
		if deduct.GreaterThan(remaining) {
			deduct = remaining
		}
		remaining = remaining.Sub(deduct)
		if _, err := tx.WalletGrant.UpdateOneID(grant.ID).
			SetAvailablePoints(available.Sub(deduct).Round(s.scale).StringFixed(s.scale)).
			Save(ctx); err != nil {
			return err
		}
	}
	if remaining.IsPositive() {
		return errs.New(400, errs.CodeInsufficientPoints, "insufficient points")
	}
	return nil
}

func (s *BillingStore) createWalletGrant(ctx context.Context, tx *repoent.Tx, userID int64, grantType, sourceType string, sourceID *int64, amount decimal.Decimal, expiresAt *time.Time, metadata map[string]any) (*repoent.WalletGrant, error) {
	builder := tx.WalletGrant.Create().
		SetUserID(userID).
		SetGrantType(grantType).
		SetSourceType(sourceType).
		SetStatus("active").
		SetTotalPoints(amount.Round(s.scale).StringFixed(s.scale)).
		SetAvailablePoints(amount.Round(s.scale).StringFixed(s.scale)).
		SetFrozenPoints(decimal.Zero.Round(s.scale).StringFixed(s.scale)).
		SetConsumedPoints(decimal.Zero.Round(s.scale).StringFixed(s.scale))
	if sourceID != nil {
		builder.SetSourceID(*sourceID)
	}
	if expiresAt != nil {
		builder.SetExpiresAt(*expiresAt)
	}
	if metadata != nil {
		builder.SetMetadata(metadata)
	}
	return builder.Save(ctx)
}

func (s *BillingStore) ensureLegacyGrant(ctx context.Context, tx *repoent.Tx, userID int64) error {
	count, err := tx.WalletGrant.Query().Where(walletgrant.UserIDEQ(userID)).Count(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	legacyState, err := s.currentStateLegacy(ctx, tx.Client(), userID)
	if err != nil {
		return err
	}
	if !legacyState.Available.IsPositive() && !legacyState.Frozen.IsPositive() {
		return nil
	}
	metadata := map[string]any{"source": "legacy_point_ledger_bootstrap"}
	grant, err := s.createWalletGrant(ctx, tx, userID, "recharge", "legacy_point_ledger", nil, legacyState.Available, nil, metadata)
	if err != nil {
		return err
	}
	if legacyState.Frozen.IsPositive() {
		if _, err := tx.WalletGrant.UpdateOneID(grant.ID).
			SetAvailablePoints(decimal.Zero.Round(s.scale).StringFixed(s.scale)).
			SetFrozenPoints(legacyState.Frozen.Round(s.scale).StringFixed(s.scale)).
			SetTotalPoints(legacyState.Available.Add(legacyState.Frozen).Round(s.scale).StringFixed(s.scale)).
			Save(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *BillingStore) grantOrderCredits(ctx context.Context, tx *repoent.Tx, order *repoent.PaymentOrder) (decimalState, error) {
	plan, err := tx.SubscriptionPlan.Query().Where(subscriptionplan.IDEQ(int(order.PlanID))).Only(ctx)
	if err != nil {
		return decimalState{}, err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(plan.DurationDays) * 24 * time.Hour)
	subscriptionGrant, err := s.createWalletGrant(ctx, tx, order.UserID, "subscription", "payment_order", pointerInt64(int64(order.ID)), mustDecimal(order.Points), &expiresAt, map[string]any{"plan_code": plan.PlanCode})
	if err != nil {
		return decimalState{}, err
	}
	if mustDecimal(order.BonusPoints).IsPositive() {
		if _, err := s.createWalletGrant(ctx, tx, order.UserID, "gift", "payment_order_bonus", pointerInt64(int64(order.ID)), mustDecimal(order.BonusPoints), &expiresAt, map[string]any{"plan_code": plan.PlanCode}); err != nil {
			return decimalState{}, err
		}
	}
	if _, err := tx.UserSubscription.Create().
		SetUserID(order.UserID).
		SetPlanID(order.PlanID).
		SetWalletGrantID(int64(subscriptionGrant.ID)).
		SetPaymentOrderID(int64(order.ID)).
		SetStatus("active").
		SetStartedAt(now).
		SetCurrentPeriodStart(now).
		SetCurrentPeriodEnd(expiresAt).
		Save(ctx); err != nil {
		return decimalState{}, err
	}
	state, _, err := s.currentStateWithDetails(ctx, tx.Client(), order.UserID)
	if err != nil {
		return decimalState{}, err
	}
	if err := s.insertLedger(ctx, tx, order.UserID, 0, "", "order_paid", mustDecimal(order.Points).Add(mustDecimal(order.BonusPoints)), state, "payment order "+order.OrderNo, 0, "order:"+order.OrderNo+":paid"); err != nil {
		return decimalState{}, err
	}
	return state, nil
}

func (s *BillingStore) ensureWebhookEvent(ctx context.Context, tx *repoent.Tx, provider, tradeNo string, orderID int64, payload map[string]any) error {
	if _, err := tx.PaymentWebhookEvent.Query().Where(paymentwebhookevent.ProviderEQ(provider), paymentwebhookevent.TradeNoEQ(tradeNo)).Only(ctx); err == nil {
		return nil
	} else if !repoent.IsNotFound(err) {
		return err
	}
	builder := tx.PaymentWebhookEvent.Create().
		SetProvider(provider).
		SetTradeNo(tradeNo).
		SetEventType("payment.succeeded").
		SetStatus("processed").
		SetProcessedAt(time.Now().UTC()).
		SetPaymentOrderID(orderID)
	if payload != nil {
		builder.SetPayload(string(mustJSON(payload)))
		builder.SetHeaders(payload)
	}
	_, err := builder.Save(ctx)
	return err
}

func (s *BillingStore) mapActiveSubscription(ctx context.Context, subscription *repoent.UserSubscription) *domainbilling.UserSubscriptionSummary {
	if subscription == nil {
		return nil
	}
	planName := ""
	planCode := ""
	if plan, err := s.client.SubscriptionPlan.Query().Where(subscriptionplan.IDEQ(int(subscription.PlanID))).Only(ctx); err == nil {
		planName = plan.PlanName
		planCode = plan.PlanCode
	}
	remaining := decimal.Zero
	granted := decimal.Zero
	if subscription.WalletGrantID != nil {
		if grant, err := s.client.WalletGrant.Query().Where(walletgrant.IDEQ(int(*subscription.WalletGrantID))).Only(ctx); err == nil {
			remaining = mustDecimal(grant.AvailablePoints).Add(mustDecimal(grant.FrozenPoints))
			granted = mustDecimal(grant.TotalPoints)
		}
	}
	return &domainbilling.UserSubscriptionSummary{
		ID:                 int64(subscription.ID),
		PlanID:             subscription.PlanID,
		PlanCode:           planCode,
		PlanName:           planName,
		Status:             subscription.Status,
		StartedAt:          subscription.StartedAt,
		CurrentPeriodStart: subscription.CurrentPeriodStart,
		CurrentPeriodEnd:   subscription.CurrentPeriodEnd,
		ExpiredAt:          subscription.ExpiredAt,
		CanceledAt:         subscription.CanceledAt,
		GrantedPoints:      granted.Round(s.scale).StringFixed(s.scale),
		RemainingPoints:    remaining.Round(s.scale).StringFixed(s.scale),
	}
}

func mapSubscriptionPlan(plan *repoent.SubscriptionPlan) domainbilling.SubscriptionPlan {
	return domainbilling.SubscriptionPlan{
		ID:           int64(plan.ID),
		PlanCode:     plan.PlanCode,
		PlanName:     plan.PlanName,
		Status:       plan.Status,
		PriceCNY:     plan.PriceCny,
		Points:       plan.Points,
		BonusPoints:  plan.BonusPoints,
		DurationDays: plan.DurationDays,
		Currency:     plan.Currency,
		Description:  plan.Description,
		CreatedAt:    plan.CreatedAt,
		UpdatedAt:    plan.UpdatedAt,
	}
}

func (s *BillingStore) mapPaymentOrder(ctx context.Context, order *repoent.PaymentOrder) domainbilling.PaymentOrder {
	item := domainbilling.PaymentOrder{
		ID:          int64(order.ID),
		OrderNo:     order.OrderNo,
		UserID:      order.UserID,
		PlanID:      order.PlanID,
		Provider:    order.Provider,
		Status:      order.Status,
		Currency:    order.Currency,
		AmountCNY:   order.AmountCny,
		Points:      order.Points,
		BonusPoints: order.BonusPoints,
		ExpiresAt:   order.ExpiresAt,
		PaidAt:      order.PaidAt,
		ClosedAt:    order.ClosedAt,
		RefundedAt:  order.RefundedAt,
		CreatedAt:   order.CreatedAt,
		UpdatedAt:   order.UpdatedAt,
	}
	if order.TradeNo != nil {
		item.TradeNo = *order.TradeNo
	}
	if order.PaymentURL != nil {
		item.PaymentURL = *order.PaymentURL
	}
	if order.QrCode != nil {
		item.QRCode = *order.QrCode
	}
	if order.ClientToken != nil {
		item.ClientToken = *order.ClientToken
	}
	if order.FailureReason != nil {
		item.FailureReason = *order.FailureReason
	}
	if plan, err := s.client.SubscriptionPlan.Query().Where(subscriptionplan.IDEQ(int(order.PlanID))).Only(ctx); err == nil {
		item.PlanCode = plan.PlanCode
		item.PlanName = plan.PlanName
	}
	return item
}

func grantPriority(grantType string) int {
	switch grantType {
	case "subscription":
		return 1
	case "gift":
		return 2
	case "recharge":
		return 3
	default:
		return 9
	}
}

func adminDeductionPriority(grantType string) int {
	switch grantType {
	case "recharge":
		return 1
	case "gift":
		return 2
	case "subscription":
		return 3
	default:
		return 9
	}
}

func mustDecimal(value string) decimal.Decimal {
	result, _ := decimal.NewFromString(value)
	return result
}

func mustJSON(value any) []byte {
	result, _ := json.Marshal(value)
	return result
}

func pointerInt64(value int64) *int64 {
	return &value
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
