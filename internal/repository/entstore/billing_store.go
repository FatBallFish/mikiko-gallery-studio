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

func (s *BillingStore) ListPlans(ctx context.Context, req domainbilling.SubscriptionPlanListRequest) ([]domainbilling.SubscriptionPlan, error) {
	if err := s.ensureDefaultPlans(ctx); err != nil {
		return nil, err
	}
	query := s.client.SubscriptionPlan.Query()
	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status == "" {
		query = query.Where(subscriptionplan.StatusNEQ(domainbilling.SubscriptionPlanStatusArchived))
	} else if status != "all" {
		query = query.Where(subscriptionplan.StatusEQ(status))
	}
	plans, err := query.
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

func (s *BillingStore) CreatePlan(ctx context.Context, req domainbilling.CreateSubscriptionPlanRequest) (domainbilling.SubscriptionPlan, error) {
	metadata := subscriptionPlanMetadata(req.PlanType, req.PurchaseEnabled)
	expiryEnabled := effectivePlanCreditExpiryEnabled(req.CreditExpiryEnabled)
	plan, err := s.client.SubscriptionPlan.Create().
		SetPlanCode(strings.TrimSpace(req.PlanCode)).
		SetPlanName(strings.TrimSpace(req.PlanName)).
		SetPlanType(strings.TrimSpace(req.PlanType)).
		SetPurchaseEnabled(req.PurchaseEnabled).
		SetStatus(strings.TrimSpace(req.Status)).
		SetPriceCny(strings.TrimSpace(req.PriceCNY)).
		SetPoints(strings.TrimSpace(req.Points)).
		SetBonusPoints(strings.TrimSpace(req.BonusPoints)).
		SetCreditExpiryEnabled(expiryEnabled).
		SetNillableDurationDays(req.DurationDays).
		SetCurrency(strings.TrimSpace(req.Currency)).
		SetDescription(strings.TrimSpace(req.Description)).
		SetSortOrder(req.SortOrder).
		SetMetadata(metadata).
		Save(ctx)
	if err != nil {
		if repoent.IsConstraintError(err) {
			return domainbilling.SubscriptionPlan{}, errs.New(http.StatusConflict, errs.CodeConflict, "subscription plan code already exists")
		}
		return domainbilling.SubscriptionPlan{}, err
	}
	return mapSubscriptionPlan(plan), nil
}

func (s *BillingStore) UpdatePlan(ctx context.Context, req domainbilling.UpdateSubscriptionPlanRequest) (domainbilling.SubscriptionPlan, error) {
	current, err := s.client.SubscriptionPlan.Get(ctx, int(req.PlanID))
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainbilling.SubscriptionPlan{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "subscription plan not found")
		}
		return domainbilling.SubscriptionPlan{}, err
	}
	purchaseEnabled := current.PurchaseEnabled && strings.TrimSpace(req.PlanType) == "points_package"
	metadata := subscriptionPlanMetadata(req.PlanType, purchaseEnabled)
	expiryEnabled := effectivePlanCreditExpiryEnabled(req.CreditExpiryEnabled)
	plan, err := s.client.SubscriptionPlan.UpdateOneID(int(req.PlanID)).
		SetPlanName(strings.TrimSpace(req.PlanName)).
		SetPlanType(strings.TrimSpace(req.PlanType)).
		SetPurchaseEnabled(purchaseEnabled).
		SetPriceCny(strings.TrimSpace(req.PriceCNY)).
		SetPoints(strings.TrimSpace(req.Points)).
		SetBonusPoints(strings.TrimSpace(req.BonusPoints)).
		SetCreditExpiryEnabled(expiryEnabled).
		SetNillableDurationDays(req.DurationDays).
		SetCurrency(strings.TrimSpace(req.Currency)).
		SetDescription(strings.TrimSpace(req.Description)).
		SetSortOrder(req.SortOrder).
		SetMetadata(metadata).
		Save(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainbilling.SubscriptionPlan{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "subscription plan not found")
		}
		return domainbilling.SubscriptionPlan{}, err
	}
	return mapSubscriptionPlan(plan), nil
}

func (s *BillingStore) DeletePlan(ctx context.Context, planID int64) (domainbilling.SubscriptionPlan, error) {
	return s.TransitionPlan(ctx, domainbilling.TransitionSubscriptionPlanRequest{
		PlanID: planID,
		Action: domainbilling.SubscriptionPlanActionArchive,
	})
}

func (s *BillingStore) TransitionPlan(ctx context.Context, req domainbilling.TransitionSubscriptionPlanRequest) (domainbilling.SubscriptionPlan, error) {
	current, err := s.client.SubscriptionPlan.Get(ctx, int(req.PlanID))
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainbilling.SubscriptionPlan{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "subscription plan not found")
		}
		return domainbilling.SubscriptionPlan{}, err
	}
	planType := subscriptionPlanType(current.PlanType, current.Metadata)
	currentPlan := mapSubscriptionPlan(current)
	currentPlan.PlanType = planType
	status, purchaseEnabled, err := billingservice.TransitionPlanState(currentPlan, req.Action)
	if err != nil {
		return domainbilling.SubscriptionPlan{}, err
	}
	plan, err := s.client.SubscriptionPlan.UpdateOneID(int(req.PlanID)).
		SetStatus(status).
		SetPurchaseEnabled(purchaseEnabled).
		SetMetadata(subscriptionPlanMetadata(planType, purchaseEnabled)).
		Save(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainbilling.SubscriptionPlan{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "subscription plan not found")
		}
		return domainbilling.SubscriptionPlan{}, err
	}
	return mapSubscriptionPlan(plan), nil
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
	query := s.client.PaymentOrder.Query()
	if req.UserID > 0 {
		query = query.Where(paymentorder.UserIDEQ(req.UserID))
	}
	if strings.TrimSpace(req.Status) != "" {
		query = query.Where(paymentorder.StatusEQ(strings.TrimSpace(req.Status)))
	}
	if strings.TrimSpace(req.OrderNo) != "" {
		query = query.Where(paymentorder.OrderNoContainsFold(strings.TrimSpace(req.OrderNo)))
	}
	if strings.TrimSpace(req.VisibleMethod) != "" {
		query = query.Where(paymentorder.VisibleMethodEQ(strings.ToLower(strings.TrimSpace(req.VisibleMethod))))
	}
	if strings.TrimSpace(req.ProviderType) != "" {
		query = query.Where(paymentorder.ProviderTypeEQ(strings.ToLower(strings.TrimSpace(req.ProviderType))))
	}
	if strings.TrimSpace(req.PurchaseType) != "" {
		query = query.Where(paymentorder.PurchaseTypeEQ(strings.ToLower(strings.TrimSpace(req.PurchaseType))))
	}
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

func (s *BillingStore) ListWebhookEvents(ctx context.Context, page, pageSize int) (domainbilling.PaymentWebhookEventPage, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	query := s.client.PaymentWebhookEvent.Query()
	total, err := query.Count(ctx)
	if err != nil {
		return domainbilling.PaymentWebhookEventPage{}, err
	}
	events, err := query.Order(repoent.Desc(paymentwebhookevent.FieldID)).Offset((page - 1) * pageSize).Limit(pageSize).All(ctx)
	if err != nil {
		return domainbilling.PaymentWebhookEventPage{}, err
	}
	items := make([]domainbilling.PaymentWebhookEvent, 0, len(events))
	for _, event := range events {
		items = append(items, s.mapPaymentWebhookEvent(ctx, event))
	}
	return domainbilling.PaymentWebhookEventPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
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

func (s *BillingStore) GetOrderByIdempotencyKey(ctx context.Context, userID int64, idempotencyKey string) (domainbilling.PaymentOrder, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
	}
	order, err := s.client.PaymentOrder.Query().
		Where(paymentorder.UserIDEQ(userID), paymentorder.IdempotencyKeyEQ(idempotencyKey)).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
		}
		return domainbilling.PaymentOrder{}, err
	}
	return s.mapPaymentOrder(ctx, order), nil
}

func (s *BillingStore) GetOrderForAdmin(ctx context.Context, orderID int64) (domainbilling.PaymentOrder, error) {
	order, err := s.client.PaymentOrder.Query().
		Where(paymentorder.IDEQ(int(orderID))).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
		}
		return domainbilling.PaymentOrder{}, err
	}
	return s.mapPaymentOrder(ctx, order), nil
}

func (s *BillingStore) RecordChargebackSummary(ctx context.Context, req billingservice.ChargebackSummaryStoreRequest) (domainbilling.PaymentOrder, error) {
	order, err := s.client.PaymentOrder.Query().
		Where(paymentorder.IDEQ(int(req.OrderID))).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
		}
		return domainbilling.PaymentOrder{}, err
	}
	payload := cloneMap(order.ProviderPayload)
	if payload == nil {
		payload = map[string]any{}
	}
	now := time.Now().UTC()
	payload["chargeback_points"] = strings.TrimSpace(req.ChargePoints)
	payload["chargeback_reason"] = strings.TrimSpace(req.Reason)
	payload["chargeback_idempotency_key"] = strings.TrimSpace(req.IdempotencyKey)
	payload["chargeback_at"] = now.Format(time.RFC3339Nano)
	updated, err := s.client.PaymentOrder.UpdateOneID(order.ID).
		SetProviderPayload(payload).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	return s.mapPaymentOrder(ctx, updated), nil
}

func (s *BillingStore) RetryWebhookEvent(ctx context.Context, eventID int64) (domainbilling.PaymentWebhookEvent, error) {
	event, err := s.client.PaymentWebhookEvent.Query().
		Where(paymentwebhookevent.IDEQ(int(eventID))).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainbilling.PaymentWebhookEvent{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment webhook event not found")
		}
		return domainbilling.PaymentWebhookEvent{}, err
	}
	if event.EventType == "refund.local_finalize_failed" {
		var payload struct {
			UserID          int64  `json:"user_id"`
			OrderID         int64  `json:"order_id"`
			RefundTradeNo   string `json:"refund_trade_no"`
			Reason          string `json:"reason"`
			OperatorAdminID int64  `json:"operator_admin_id"`
		}
		if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
			return domainbilling.PaymentWebhookEvent{}, errs.New(http.StatusConflict, errs.CodeConflict, "refund compensation payload is invalid")
		}
		if _, err := s.RefundPaymentOrder(ctx, domainbilling.RefundPaymentOrderRequest{
			UserID:          payload.UserID,
			OrderID:         payload.OrderID,
			RefundTradeNo:   payload.RefundTradeNo,
			Reason:          payload.Reason,
			OperatorAdminID: payload.OperatorAdminID,
		}); err != nil {
			return domainbilling.PaymentWebhookEvent{}, err
		}
		event, err = s.client.PaymentWebhookEvent.UpdateOneID(event.ID).
			SetStatus("processed").
			SetProcessedAt(time.Now().UTC()).
			Save(ctx)
		if err != nil {
			return domainbilling.PaymentWebhookEvent{}, err
		}
		return s.mapPaymentWebhookEvent(ctx, event), nil
	}
	event, err = s.client.PaymentWebhookEvent.UpdateOneID(event.ID).
		SetStatus("processed").
		SetProcessedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return domainbilling.PaymentWebhookEvent{}, err
	}
	return s.mapPaymentWebhookEvent(ctx, event), nil
}

func (s *BillingStore) ProcessRefundFinalizeFailures(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 5
	}
	events, err := s.client.PaymentWebhookEvent.Query().
		Where(paymentwebhookevent.EventTypeEQ("refund.local_finalize_failed"), paymentwebhookevent.StatusEQ("failed")).
		Order(repoent.Asc(paymentwebhookevent.FieldID)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, event := range events {
		if _, err := s.RetryWebhookEvent(ctx, int64(event.ID)); err == nil {
			processed++
		}
	}
	return processed, nil
}

func (s *BillingStore) RecordRefundFinalizeFailure(ctx context.Context, req billingservice.RefundFinalizeFailureRequest) (domainbilling.PaymentWebhookEvent, error) {
	order, err := s.client.PaymentOrder.Query().
		Where(paymentorder.IDEQ(int(req.OrderID)), paymentorder.UserIDEQ(req.UserID)).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainbilling.PaymentWebhookEvent{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
		}
		return domainbilling.PaymentWebhookEvent{}, err
	}
	tradeNo := "refund-finalize:" + order.OrderNo + ":" + strings.TrimSpace(req.RefundTradeNo)
	payload := map[string]any{
		"user_id":           req.UserID,
		"order_id":          req.OrderID,
		"order_no":          order.OrderNo,
		"refund_trade_no":   strings.TrimSpace(req.RefundTradeNo),
		"reason":            strings.TrimSpace(req.Reason),
		"operator_admin_id": req.OperatorAdminID,
		"failure_reason":    strings.TrimSpace(req.FailureReason),
	}
	if existing, err := s.client.PaymentWebhookEvent.Query().
		Where(paymentwebhookevent.ProviderEQ(order.Provider), paymentwebhookevent.TradeNoEQ(tradeNo)).
		Only(ctx); err == nil {
		updated, err := s.client.PaymentWebhookEvent.UpdateOneID(existing.ID).
			SetStatus("failed").
			SetPayload(string(mustJSON(payload))).
			SetHeaders(payload).
			ClearProcessedAt().
			Save(ctx)
		if err != nil {
			return domainbilling.PaymentWebhookEvent{}, err
		}
		return s.mapPaymentWebhookEvent(ctx, updated), nil
	} else if !repoent.IsNotFound(err) {
		return domainbilling.PaymentWebhookEvent{}, err
	}
	event, err := s.client.PaymentWebhookEvent.Create().
		SetProvider(order.Provider).
		SetTradeNo(tradeNo).
		SetEventType("refund.local_finalize_failed").
		SetStatus("failed").
		SetPaymentOrderID(int64(order.ID)).
		SetPayload(string(mustJSON(payload))).
		SetHeaders(payload).
		Save(ctx)
	if err != nil {
		return domainbilling.PaymentWebhookEvent{}, err
	}
	return s.mapPaymentWebhookEvent(ctx, event), nil
}

func (s *BillingStore) CreateOrder(ctx context.Context, req domainbilling.CreateOrderRequest) (domainbilling.PaymentOrder, error) {
	if err := s.ensureDefaultPlans(ctx); err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if idempotencyKey != "" {
		existing, err := s.client.PaymentOrder.Query().
			Where(paymentorder.UserIDEQ(req.UserID), paymentorder.IdempotencyKeyEQ(idempotencyKey)).
			Only(ctx)
		if err == nil {
			return s.mapPaymentOrder(ctx, existing), nil
		}
		if !repoent.IsNotFound(err) {
			return domainbilling.PaymentOrder{}, err
		}
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
	mappedPlan := mapSubscriptionPlan(plan)
	if mappedPlan.PlanType != "points_package" || !mappedPlan.PurchaseEnabled {
		return domainbilling.PaymentOrder{}, errs.BadRequest("subscription plan is not purchasable")
	}
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "" {
		return domainbilling.PaymentOrder{}, errs.BadRequest("provider is required")
	}
	now := time.Now().UTC()
	orderNo := strings.TrimSpace(req.OrderNo)
	if orderNo == "" {
		orderNo = fmt.Sprintf("PGO-%d-%06d", now.Unix(), time.Now().Nanosecond()%1000000)
	}
	paymentURL := strings.TrimSpace(req.PaymentURL)
	providerPayload := paymentOrderProviderPayload(req.PurchaseType, req.VisibleMethod, req.ProviderType, req.ProviderInstanceID, req.PaymentDisplay)
	providerPayload["provider"] = provider
	providerPayload["order_no"] = orderNo
	purchaseType := defaultString(strings.ToLower(strings.TrimSpace(req.PurchaseType)), "plan")
	visibleMethod := strings.ToLower(strings.TrimSpace(req.VisibleMethod))
	providerType := defaultString(strings.ToLower(strings.TrimSpace(req.ProviderType)), provider)
	providerSnapshot := paymentOrderProviderSnapshot(provider, providerType, req.ProviderInstanceID)
	create := s.client.PaymentOrder.Create().
		SetUserID(req.UserID).
		SetPlanID(int64(plan.ID)).
		SetOrderNo(orderNo).
		SetProvider(provider).
		SetPurchaseType(purchaseType).
		SetVisibleMethod(visibleMethod).
		SetProviderType(providerType).
		SetProviderSnapshot(providerSnapshot).
		SetStatus("pending").
		SetCurrency(plan.Currency).
		SetAmountCny(plan.PriceCny).
		SetPoints(plan.Points).
		SetBonusPoints(plan.BonusPoints).
		SetExpiresAt(now.Add(15 * time.Minute)).
		SetProviderPayload(providerPayload)
	if len(req.PaymentDisplay) > 0 {
		create.SetPaymentDisplay(cloneMap(req.PaymentDisplay))
	}
	if paymentURL != "" {
		create.SetPaymentURL(paymentURL)
	}
	if req.ProviderInstanceID > 0 {
		create.SetProviderInstanceID(req.ProviderInstanceID)
	}
	if strings.TrimSpace(req.QRCode) != "" {
		create.SetQrCode(strings.TrimSpace(req.QRCode))
	}
	if strings.TrimSpace(req.ClientToken) != "" {
		create.SetClientToken(strings.TrimSpace(req.ClientToken))
	}
	if idempotencyKey != "" {
		create.SetIdempotencyKey(idempotencyKey)
	}
	order, err := create.Save(ctx)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	return s.mapPaymentOrder(ctx, order), nil
}

func (s *BillingStore) CreateCustomAmountOrder(ctx context.Context, req domainbilling.CreateCustomAmountOrderRequest) (domainbilling.PaymentOrder, error) {
	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if idempotencyKey != "" {
		existing, err := s.client.PaymentOrder.Query().
			Where(paymentorder.UserIDEQ(req.UserID), paymentorder.IdempotencyKeyEQ(idempotencyKey)).
			Only(ctx)
		if err == nil {
			return s.mapPaymentOrder(ctx, existing), nil
		}
		if !repoent.IsNotFound(err) {
			return domainbilling.PaymentOrder{}, err
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
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "" {
		return domainbilling.PaymentOrder{}, errs.BadRequest("provider is required")
	}
	now := time.Now().UTC()
	orderNo := strings.TrimSpace(req.OrderNo)
	if orderNo == "" {
		orderNo = fmt.Sprintf("PGO-%d-%06d", now.Unix(), time.Now().Nanosecond()%1000000)
	}
	paymentURL := strings.TrimSpace(req.PaymentURL)
	points := amount.Div(cnyPerPoint).Round(s.scale)
	providerPayload := paymentOrderProviderPayload(req.PurchaseType, req.VisibleMethod, req.ProviderType, req.ProviderInstanceID, req.PaymentDisplay)
	providerPayload["provider"] = provider
	providerPayload["order_no"] = orderNo
	providerPayload["purchase_type"] = "custom_amount"
	providerPayload["cny_per_point"] = cnyPerPoint.StringFixed(s.scale)
	purchaseType := defaultString(strings.ToLower(strings.TrimSpace(req.PurchaseType)), "custom_amount")
	visibleMethod := strings.ToLower(strings.TrimSpace(req.VisibleMethod))
	providerType := defaultString(strings.ToLower(strings.TrimSpace(req.ProviderType)), provider)
	providerSnapshot := paymentOrderProviderSnapshot(provider, providerType, req.ProviderInstanceID)
	create := s.client.PaymentOrder.Create().
		SetUserID(req.UserID).
		SetPlanID(0).
		SetOrderNo(orderNo).
		SetProvider(provider).
		SetPurchaseType(purchaseType).
		SetVisibleMethod(visibleMethod).
		SetProviderType(providerType).
		SetProviderSnapshot(providerSnapshot).
		SetStatus("pending").
		SetCurrency("CNY").
		SetAmountCny(amount.Round(s.scale).StringFixed(s.scale)).
		SetPoints(points.StringFixed(s.scale)).
		SetBonusPoints(decimal.Zero.StringFixed(s.scale)).
		SetExpiresAt(now.Add(15 * time.Minute)).
		SetProviderPayload(providerPayload)
	if len(req.PaymentDisplay) > 0 {
		create.SetPaymentDisplay(cloneMap(req.PaymentDisplay))
	}
	if paymentURL != "" {
		create.SetPaymentURL(paymentURL)
	}
	if req.ProviderInstanceID > 0 {
		create.SetProviderInstanceID(req.ProviderInstanceID)
	}
	if strings.TrimSpace(req.QRCode) != "" {
		create.SetQrCode(strings.TrimSpace(req.QRCode))
	}
	if strings.TrimSpace(req.ClientToken) != "" {
		create.SetClientToken(strings.TrimSpace(req.ClientToken))
	}
	if idempotencyKey != "" {
		create.SetIdempotencyKey(idempotencyKey)
	}
	order, err := create.Save(ctx)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	return s.mapPaymentOrder(ctx, order), nil
}

func (s *BillingStore) InitializePaymentOrder(ctx context.Context, req domainbilling.InitializePaymentOrderRequest) (domainbilling.PaymentOrder, error) {
	update := s.client.PaymentOrder.Update().
		Where(paymentorder.IDEQ(int(req.OrderID)), paymentorder.UserIDEQ(req.UserID), paymentorder.StatusEQ("pending")).
		SetPaymentDisplay(cloneMap(req.PaymentDisplay)).
		ClearFailureReason()
	if paymentURL := strings.TrimSpace(req.PaymentURL); paymentURL != "" {
		update.SetPaymentURL(paymentURL)
	}
	if qrCode := strings.TrimSpace(req.QRCode); qrCode != "" {
		update.SetQrCode(qrCode)
	}
	if clientToken := strings.TrimSpace(req.ClientToken); clientToken != "" {
		update.SetClientToken(clientToken)
	}
	if tradeNo := strings.TrimSpace(req.TradeNo); tradeNo != "" {
		update.SetTradeNo(tradeNo)
	}
	_, err := update.Save(ctx)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	return s.paymentOrderAfterInitializationMutation(ctx, req.UserID, req.OrderID)
}

func (s *BillingStore) FailPaymentOrderInitialization(ctx context.Context, req domainbilling.FailPaymentOrderInitializationRequest) (domainbilling.PaymentOrder, error) {
	now := time.Now().UTC()
	_, err := s.client.PaymentOrder.Update().
		Where(
			paymentorder.IDEQ(int(req.OrderID)),
			paymentorder.UserIDEQ(req.UserID),
			paymentorder.StatusEQ("pending"),
			paymentorder.PaymentDisplayIsNil(),
			paymentorder.PaymentURLIsNil(),
			paymentorder.QrCodeIsNil(),
			paymentorder.ClientTokenIsNil(),
		).
		SetStatus("failed").
		SetFailureReason(strings.TrimSpace(req.FailureReason)).
		SetClosedAt(now).
		Save(ctx)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	return s.paymentOrderAfterInitializationMutation(ctx, req.UserID, req.OrderID)
}

func (s *BillingStore) paymentOrderAfterInitializationMutation(ctx context.Context, userID, orderID int64) (domainbilling.PaymentOrder, error) {
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

func (s *BillingStore) CompleteRechargeOrder(ctx context.Context, req domainbilling.CompleteRechargeOrderRequest) (domainbilling.PaymentOrder, error) {
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "" {
		provider = "mock"
	}
	tradeNo := strings.TrimSpace(req.TradeNo)
	if tradeNo == "" {
		return domainbilling.PaymentOrder{}, errs.BadRequest("trade_no is required")
	}
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (domainbilling.PaymentOrder, error) {
		order, err := tx.PaymentOrder.Query().
			Where(paymentorder.IDEQ(int(req.OrderID)), paymentorder.UserIDEQ(req.UserID)).
			Only(ctx)
		if err != nil {
			if repoent.IsNotFound(err) {
				return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
			}
			return domainbilling.PaymentOrder{}, err
		}
		return s.completeRechargeOrderInTx(ctx, tx, order, provider, tradeNo, map[string]any{"order_no": order.OrderNo}, domainbilling.PaymentReconciliationSourceMockConfirmation)
	})
}

func (s *BillingStore) RefundPaymentOrder(ctx context.Context, req domainbilling.RefundPaymentOrderRequest) (domainbilling.PaymentOrder, error) {
	refundTradeNo := strings.TrimSpace(req.RefundTradeNo)
	if refundTradeNo == "" {
		return domainbilling.PaymentOrder{}, errs.BadRequest("refund_trade_no is required")
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "cashier order refund"
	}
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (domainbilling.PaymentOrder, error) {
		order, err := tx.PaymentOrder.Query().
			Where(paymentorder.IDEQ(int(req.OrderID)), paymentorder.UserIDEQ(req.UserID)).
			Only(ctx)
		if err != nil {
			if repoent.IsNotFound(err) {
				return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
			}
			return domainbilling.PaymentOrder{}, err
		}
		payload := cloneMap(order.ProviderPayload)
		if payload == nil {
			payload = map[string]any{}
		}
		if refundRecordExists(payload, refundTradeNo) {
			return s.mapPaymentOrder(ctx, order), nil
		}
		if order.Status == "refunded" {
			return s.mapPaymentOrder(ctx, order), nil
		}
		if order.Status != "completed" && order.Status != "partially_refunded" {
			return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order cannot transition to refunded")
		}
		plan, err := s.paymentOrderRefundPlan(order, req, payload)
		if err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		grant, err := s.refundableRechargeGrantInTx(ctx, tx, order)
		if err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		available := mustDecimal(grant.AvailablePoints)
		frozen := mustDecimal(grant.FrozenPoints)
		consumed := mustDecimal(grant.ConsumedPoints)
		refundFrozen := refundFreezeAmount(grant.Metadata, refundTradeNo, s.scale)
		if refundFrozen.IsPositive() {
			if err := ensureRefundFreezeMatches(grant.Metadata, refundTradeNo, plan.RefundAmountCNY, plan.RefundPoints, s.scale); err != nil {
				return domainbilling.PaymentOrder{}, err
			}
		}
		if consumed.IsPositive() || (refundFrozen.IsZero() && (frozen.IsPositive() || available.LessThan(plan.RefundPoints))) || (refundFrozen.IsPositive() && frozen.LessThan(plan.RefundPoints)) {
			return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order recharge balance is insufficient for refund")
		}
		nextAvailable := available.Sub(plan.RefundPoints)
		nextFrozen := decimal.Zero
		if refundFrozen.IsPositive() {
			nextAvailable = available
			nextFrozen = frozen.Sub(plan.RefundPoints)
			if nextFrozen.IsNegative() {
				nextFrozen = decimal.Zero
			}
		}
		grantStatus := "active"
		if plan.FullyRefunded {
			grantStatus = "refunded"
			nextAvailable = decimal.Zero
			nextFrozen = decimal.Zero
		}
		if _, err := tx.WalletGrant.UpdateOneID(grant.ID).
			SetAvailablePoints(nextAvailable.Round(s.scale).StringFixed(s.scale)).
			SetFrozenPoints(nextFrozen.Round(s.scale).StringFixed(s.scale)).
			SetStatus(grantStatus).
			Save(ctx); err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		state, _, err := s.currentStateWithDetails(ctx, tx.Client(), order.UserID)
		if err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		if _, err := s.insertPaymentOrderLedger(ctx, tx, order.UserID, int64(order.ID), "payment_refund", plan.RefundPoints.Neg(), state, reason, req.OperatorAdminID, "cashier:"+order.OrderNo+":refund:"+refundTradeNo); err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		payload["refund_trade_no"] = refundTradeNo
		payload["refunded_amount_cny"] = plan.NextRefundedAmountCNY.StringFixed(s.scale)
		payload["refunded_points"] = plan.NextRefundedPoints.StringFixed(s.scale)
		payload["refund_records"] = appendRefundRecord(payload, refundTradeNo, plan.RefundAmountCNY, plan.RefundPoints, reason, req.OperatorAdminID, s.scale)
		if strings.TrimSpace(req.Reason) != "" {
			payload["refund_reason"] = strings.TrimSpace(req.Reason)
		}
		now := time.Now().UTC()
		update := tx.PaymentOrder.UpdateOneID(order.ID).
			SetStatus(boolString(plan.FullyRefunded, "refunded", "partially_refunded")).
			SetProviderPayload(payload)
		if plan.FullyRefunded {
			update.SetRefundedAt(now)
		}
		if _, err := update.Save(ctx); err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		updated, err := tx.PaymentOrder.Query().Where(paymentorder.IDEQ(order.ID)).Only(ctx)
		if err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		return s.mapPaymentOrder(ctx, updated), nil
	})
}

func (s *BillingStore) FreezeRefundPaymentOrder(ctx context.Context, req domainbilling.RefundPaymentOrderRequest) (domainbilling.PaymentOrder, error) {
	refundTradeNo := strings.TrimSpace(req.RefundTradeNo)
	if refundTradeNo == "" {
		return domainbilling.PaymentOrder{}, errs.BadRequest("refund_trade_no is required")
	}
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (domainbilling.PaymentOrder, error) {
		order, err := tx.PaymentOrder.Query().
			Where(paymentorder.IDEQ(int(req.OrderID)), paymentorder.UserIDEQ(req.UserID)).
			Only(ctx)
		if err != nil {
			if repoent.IsNotFound(err) {
				return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
			}
			return domainbilling.PaymentOrder{}, err
		}
		if order.Status == "refunded" {
			return s.mapPaymentOrder(ctx, order), nil
		}
		if order.Status != "completed" && order.Status != "partially_refunded" {
			return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order cannot transition to refunded")
		}
		payload := cloneMap(order.ProviderPayload)
		if payload == nil {
			payload = map[string]any{}
		}
		if refundRecordExists(payload, refundTradeNo) {
			return s.mapPaymentOrder(ctx, order), nil
		}
		plan, err := s.paymentOrderRefundPlan(order, req, payload)
		if err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		grant, err := s.refundableRechargeGrantInTx(ctx, tx, order)
		if err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		existingFreeze := refundFreezeAmount(grant.Metadata, refundTradeNo, s.scale)
		if existingFreeze.IsPositive() {
			if err := ensureRefundFreezeMatches(grant.Metadata, refundTradeNo, plan.RefundAmountCNY, plan.RefundPoints, s.scale); err != nil {
				return domainbilling.PaymentOrder{}, err
			}
			return s.mapPaymentOrder(ctx, order), nil
		}
		available := mustDecimal(grant.AvailablePoints)
		frozen := mustDecimal(grant.FrozenPoints)
		consumed := mustDecimal(grant.ConsumedPoints)
		if consumed.IsPositive() || frozen.IsPositive() || available.LessThan(plan.RefundPoints) {
			return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order recharge balance is insufficient for refund")
		}
		metadata := cloneMap(grant.Metadata)
		if metadata == nil {
			metadata = map[string]any{}
		}
		metadata["refund_freeze_trade_no"] = refundTradeNo
		metadata["refund_freeze_amount_cny"] = plan.RefundAmountCNY.StringFixed(s.scale)
		metadata["refund_freeze_points"] = plan.RefundPoints.StringFixed(s.scale)
		metadata["refund_freeze_reason"] = strings.TrimSpace(req.Reason)
		metadata["refund_freeze_operator_admin_id"] = req.OperatorAdminID
		metadata["refund_freeze_at"] = time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.WalletGrant.UpdateOneID(grant.ID).
			SetAvailablePoints(available.Sub(plan.RefundPoints).Round(s.scale).StringFixed(s.scale)).
			SetFrozenPoints(frozen.Add(plan.RefundPoints).Round(s.scale).StringFixed(s.scale)).
			SetMetadata(metadata).
			Save(ctx); err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		return s.mapPaymentOrder(ctx, order), nil
	})
}

func (s *BillingStore) ReleaseRefundPaymentOrder(ctx context.Context, req domainbilling.RefundPaymentOrderRequest) (domainbilling.PaymentOrder, error) {
	refundTradeNo := strings.TrimSpace(req.RefundTradeNo)
	if refundTradeNo == "" {
		return domainbilling.PaymentOrder{}, errs.BadRequest("refund_trade_no is required")
	}
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (domainbilling.PaymentOrder, error) {
		order, err := tx.PaymentOrder.Query().
			Where(paymentorder.IDEQ(int(req.OrderID)), paymentorder.UserIDEQ(req.UserID)).
			Only(ctx)
		if err != nil {
			if repoent.IsNotFound(err) {
				return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
			}
			return domainbilling.PaymentOrder{}, err
		}
		if order.Status == "refunded" {
			return s.mapPaymentOrder(ctx, order), nil
		}
		grant, err := s.refundableRechargeGrantInTx(ctx, tx, order)
		if err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		frozenRefund := refundFreezeAmount(grant.Metadata, refundTradeNo, s.scale)
		if !frozenRefund.IsPositive() {
			return s.mapPaymentOrder(ctx, order), nil
		}
		available := mustDecimal(grant.AvailablePoints)
		frozen := mustDecimal(grant.FrozenPoints)
		if frozen.LessThan(frozenRefund) {
			frozenRefund = frozen
		}
		metadata := cloneMap(grant.Metadata)
		delete(metadata, "refund_freeze_trade_no")
		delete(metadata, "refund_freeze_amount_cny")
		delete(metadata, "refund_freeze_points")
		delete(metadata, "refund_freeze_reason")
		delete(metadata, "refund_freeze_operator_admin_id")
		delete(metadata, "refund_freeze_at")
		if _, err := tx.WalletGrant.UpdateOneID(grant.ID).
			SetAvailablePoints(available.Add(frozenRefund).Round(s.scale).StringFixed(s.scale)).
			SetFrozenPoints(frozen.Sub(frozenRefund).Round(s.scale).StringFixed(s.scale)).
			SetMetadata(metadata).
			Save(ctx); err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		return s.mapPaymentOrder(ctx, order), nil
	})
}

func (s *BillingStore) RecordProviderRefundStatus(ctx context.Context, req billingservice.ProviderRefundStatusRequest) (domainbilling.PaymentOrder, error) {
	refundTradeNo := strings.TrimSpace(req.RefundTradeNo)
	channelRefundNo := strings.TrimSpace(req.ChannelRefundNo)
	channelRefundStatus := strings.ToLower(strings.TrimSpace(req.ChannelRefundStatus))
	if refundTradeNo == "" || channelRefundNo == "" || channelRefundStatus == "" {
		return domainbilling.PaymentOrder{}, errs.BadRequest("provider refund status is incomplete")
	}
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (domainbilling.PaymentOrder, error) {
		order, err := tx.PaymentOrder.Query().
			Where(paymentorder.IDEQ(int(req.OrderID)), paymentorder.UserIDEQ(req.UserID)).
			Only(ctx)
		if err != nil {
			if repoent.IsNotFound(err) {
				return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
			}
			return domainbilling.PaymentOrder{}, err
		}
		payload := cloneMap(order.ProviderPayload)
		if payload == nil {
			payload = map[string]any{}
		}
		payload["refund_trade_no"] = refundTradeNo
		payload["provider_refund"] = map[string]any{
			"refund_trade_no":       refundTradeNo,
			"refund_amount_cny":     strings.TrimSpace(req.RefundAmountCNY),
			"channel_refund_no":     channelRefundNo,
			"channel_refund_status": channelRefundStatus,
			"reason":                strings.TrimSpace(req.Reason),
			"operator_admin_id":     req.OperatorAdminID,
			"updated_at":            time.Now().UTC().Format(time.RFC3339Nano),
		}
		updated, err := tx.PaymentOrder.UpdateOneID(order.ID).SetProviderPayload(payload).Save(ctx)
		if err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		return s.mapPaymentOrder(ctx, updated), nil
	})
}

func (s *BillingStore) CheckRefundPaymentOrder(ctx context.Context, req domainbilling.RefundPaymentOrderRequest) (domainbilling.PaymentOrder, error) {
	order, err := s.client.PaymentOrder.Query().
		Where(paymentorder.IDEQ(int(req.OrderID)), paymentorder.UserIDEQ(req.UserID)).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
		}
		return domainbilling.PaymentOrder{}, err
	}
	if order.Status == "refunded" {
		return s.mapPaymentOrder(ctx, order), nil
	}
	if order.Status != "completed" && order.Status != "partially_refunded" {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order cannot transition to refunded")
	}
	payload := cloneMap(order.ProviderPayload)
	if payload == nil {
		payload = map[string]any{}
	}
	if refundRecordExists(payload, strings.TrimSpace(req.RefundTradeNo)) {
		item := s.mapPaymentOrder(ctx, order)
		item.RefundTradeNo = strings.TrimSpace(req.RefundTradeNo)
		return item, nil
	}
	plan, err := s.paymentOrderRefundPlan(order, req, payload)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	grant, err := s.refundableRechargeGrant(ctx, order)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	available := mustDecimal(grant.AvailablePoints)
	frozen := mustDecimal(grant.FrozenPoints)
	consumed := mustDecimal(grant.ConsumedPoints)
	refundFrozen := refundFreezeAmount(grant.Metadata, strings.TrimSpace(req.RefundTradeNo), s.scale)
	if refundFrozen.IsPositive() {
		if err := ensureRefundFreezeMatches(grant.Metadata, strings.TrimSpace(req.RefundTradeNo), plan.RefundAmountCNY, plan.RefundPoints, s.scale); err != nil {
			return domainbilling.PaymentOrder{}, err
		}
	}
	if consumed.IsPositive() || (refundFrozen.IsZero() && (frozen.IsPositive() || available.LessThan(plan.RefundPoints))) || (refundFrozen.IsPositive() && frozen.LessThan(plan.RefundPoints)) {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order recharge balance is insufficient for refund")
	}
	return s.mapPaymentOrder(ctx, order), nil
}

func (s *BillingStore) CancelOrder(ctx context.Context, userID int64, orderID int64) (domainbilling.PaymentOrder, error) {
	now := time.Now().UTC()
	updated, err := s.client.PaymentOrder.Update().
		Where(paymentorder.IDEQ(int(orderID)), paymentorder.UserIDEQ(userID), paymentorder.StatusEQ("pending")).
		SetStatus("canceled").
		SetClosedAt(now).
		Save(ctx)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	order, err := s.client.PaymentOrder.Query().
		Where(paymentorder.IDEQ(int(orderID)), paymentorder.UserIDEQ(userID)).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
		}
		return domainbilling.PaymentOrder{}, err
	}
	if updated == 0 {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order cannot be canceled")
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
	reconciliationSource, err := billingservice.NormalizePaymentReconciliationSource(req.ReconciliationSource)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (domainbilling.PaymentOrder, error) {
		order, err := tx.PaymentOrder.Query().Where(paymentorder.OrderNoEQ(orderNo)).Only(ctx)
		if err != nil {
			if repoent.IsNotFound(err) {
				return domainbilling.PaymentOrder{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment order not found")
			}
			return domainbilling.PaymentOrder{}, err
		}
		providerInstanceID := int64(0)
		if order.ProviderInstanceID != nil {
			providerInstanceID = *order.ProviderInstanceID
		}
		if err := billingservice.ValidatePaymentCallbackBinding(order.ProviderType, order.Provider, providerInstanceID, req); err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		if err := ensurePaymentAmountMatches(order.AmountCny, req.AmountCNY, s.scale); err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		if isEntCashierRechargeOrder(order) {
			return s.completeRechargeOrderInTx(ctx, tx, order, provider, tradeNo, map[string]any{"order_no": orderNo}, reconciliationSource)
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
		if _, err := s.insertLedger(ctx, tx, req.UserID, req.APIKeyID, req.TaskID, "reserve", amount.Neg(), state, req.Reason, 0, reserveKey); err != nil {
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
			if _, err := s.insertLedger(ctx, tx, req.UserID, req.APIKeyID, req.TaskID, "refund", decimal.Zero, state, req.Reason, 0, refundLedgerKey(req.TaskID, ledgerState.ActiveCycle)); err != nil {
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
			if _, err := s.insertLedger(ctx, tx, req.UserID, req.APIKeyID, req.TaskID, "refund", reservedAmount, state, req.Reason, 0, refundLedgerKey(req.TaskID, ledgerState.ActiveCycle)); err != nil {
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
			if _, err := s.insertLedger(ctx, tx, req.UserID, apiKeyID, req.TaskID, "consume", actual.Neg(), refundState, req.Reason, 0, consumeLedgerKey(req.TaskID, ledgerState.ActiveCycle)); err != nil {
				return billingservice.BalanceState{}, err
			}
			if _, err := s.insertLedger(ctx, tx, req.UserID, apiKeyID, req.TaskID, "refund", diff, refundState, req.Reason, 0, refundLedgerKey(req.TaskID, ledgerState.ActiveCycle)); err != nil {
				return billingservice.BalanceState{}, err
			}
			return summary, nil
		}
		if _, err := s.insertLedger(ctx, tx, req.UserID, apiKeyID, req.TaskID, "consume", actual.Neg(), state, req.Reason, 0, consumeLedgerKey(req.TaskID, ledgerState.ActiveCycle)); err != nil {
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
		if _, err := s.insertLedger(ctx, tx, req.UserID, 0, "", "admin_adjust", change, state, req.Reason, req.OperatorAdminID, adminAdjustLedgerKey(req.UserID, idempotencyKey)); err != nil {
			return billingservice.BalanceState{}, err
		}
		return summary, nil
	})
}

func (s *BillingStore) EnsureSignupTrialGrant(ctx context.Context, req billingservice.SignupTrialGrantStoreRequest) (billingservice.SignupTrialGrantStoreResult, error) {
	if req.UserID <= 0 {
		return billingservice.SignupTrialGrantStoreResult{}, errs.BadRequest("user id is required")
	}
	amount, err := decimal.NewFromString(req.Points)
	if err != nil {
		return billingservice.SignupTrialGrantStoreResult{}, err
	}
	if !amount.IsPositive() {
		_, state, err := s.currentStateWithDetails(ctx, s.client, req.UserID)
		if err != nil {
			return billingservice.SignupTrialGrantStoreResult{}, err
		}
		return billingservice.SignupTrialGrantStoreResult{Balance: state}, nil
	}
	validDays := req.ValidDays
	if validDays <= 0 {
		validDays = 7
	}
	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("signup_trial:%d", req.UserID)
	}

	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (billingservice.SignupTrialGrantStoreResult, error) {
		if existing, err := tx.PointLedger.Query().
			Where(pointledger.IdempotencyKeyEQ(idempotencyKey)).
			Only(ctx); err == nil {
			if existing.UserID != req.UserID {
				return billingservice.SignupTrialGrantStoreResult{}, errs.New(409, errs.CodeConflict, "idempotency key belongs to a different user")
			}
			_, state, err := s.currentStateWithDetails(ctx, tx.Client(), req.UserID)
			if err != nil {
				return billingservice.SignupTrialGrantStoreResult{}, err
			}
			return billingservice.SignupTrialGrantStoreResult{Balance: state}, nil
		} else if !repoent.IsNotFound(err) {
			return billingservice.SignupTrialGrantStoreResult{}, err
		}

		expiresAt := time.Now().UTC().Add(time.Duration(validDays) * 24 * time.Hour)
		if _, err := s.createWalletGrant(ctx, tx, req.UserID, "trial", "signup", nil, amount, &expiresAt, map[string]any{
			"valid_days":           validDays,
			"expiry_reminder_days": req.ExpiryReminderDays,
		}); err != nil {
			return billingservice.SignupTrialGrantStoreResult{}, err
		}
		state, summary, err := s.currentStateWithDetails(ctx, tx.Client(), req.UserID)
		if err != nil {
			return billingservice.SignupTrialGrantStoreResult{}, err
		}
		if _, err := s.insertLedgerWithMetadata(ctx, tx, req.UserID, 0, "", "trial_grant", amount, state, "signup trial grant", 0, idempotencyKey, ledgerMetadata{
			BalanceBucket:      "trial",
			SourceType:         "signup",
			BucketBalanceAfter: summary.TrialPoints,
			ExpiresAt:          &expiresAt,
		}); err != nil {
			return billingservice.SignupTrialGrantStoreResult{}, err
		}
		return billingservice.SignupTrialGrantStoreResult{Granted: true, Balance: summary}, nil
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
		grantCount, err := client.WalletGrant.Query().Where(walletgrant.UserIDEQ(userID)).Count(ctx)
		if err != nil {
			return decimalState{}, billingservice.BalanceState{}, err
		}
		if grantCount > 0 {
			zero := decimalState{Available: decimal.Zero, Frozen: decimal.Zero}
			return zero, s.formatDetailedState(zero, nil, nil, nil), nil
		}
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
	trialAvailable := decimal.Zero
	subscriptionAvailable := decimal.Zero
	giftAvailable := decimal.Zero
	rechargeAvailable := decimal.Zero
	bucketsByType := map[string]*balanceBucketAccumulator{}
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
		case "trial":
			trialAvailable = trialAvailable.Add(available)
			giftAvailable = giftAvailable.Add(available)
		case "subscription":
			subscriptionAvailable = subscriptionAvailable.Add(available)
		case "recharge":
			rechargeAvailable = rechargeAvailable.Add(available)
		default:
			giftAvailable = giftAvailable.Add(available)
		}
		accumulateBalanceBucket(bucketsByType, grant.GrantType, grant.SourceType, available, frozen, grant.ExpiresAt, intFromAny(grant.Metadata["expiry_reminder_days"]))
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
		TrialPoints:        trialAvailable.Round(s.scale).StringFixed(s.scale),
		SubscriptionPoints: subscriptionAvailable.Round(s.scale).StringFixed(s.scale),
		GiftPoints:         giftAvailable.Round(s.scale).StringFixed(s.scale),
		RechargePoints:     rechargeAvailable.Round(s.scale).StringFixed(s.scale),
		Buckets:            formatBalanceBuckets(bucketsByType, s.scale, now),
		ActiveSubscription: activeSubscription,
		NextExpiringGrant:  nextGrant,
	}, nil
}

func (s *BillingStore) GetBalance(ctx context.Context, userID int64) (billingservice.BalanceState, error) {
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (billingservice.BalanceState, error) {
		if err := s.expireExpiredGrants(ctx, tx, userID, time.Now().UTC()); err != nil {
			return billingservice.BalanceState{}, err
		}
		_, state, err := s.currentStateWithDetails(ctx, tx.Client(), userID)
		if err != nil {
			return billingservice.BalanceState{}, err
		}
		return state, nil
	})
}

func (s *BillingStore) expireExpiredGrants(ctx context.Context, tx *repoent.Tx, userID int64, now time.Time) error {
	expiredGrants, err := tx.WalletGrant.Query().
		Where(
			walletgrant.UserIDEQ(userID),
			walletgrant.StatusEQ("active"),
			walletgrant.GrantTypeIn("trial", "subscription"),
			walletgrant.ExpiresAtNotNil(),
			walletgrant.ExpiresAtLTE(now),
		).
		Order(repoent.Asc(walletgrant.FieldExpiresAt), repoent.Asc(walletgrant.FieldID)).
		All(ctx)
	if err != nil {
		return err
	}
	for _, grant := range expiredGrants {
		available, err := decimal.NewFromString(grant.AvailablePoints)
		if err != nil {
			return err
		}
		if _, err := tx.WalletGrant.UpdateOneID(grant.ID).
			SetStatus("expired").
			Save(ctx); err != nil {
			return err
		}
		if !available.IsPositive() {
			continue
		}
		state, summary, err := s.currentStateWithDetails(ctx, tx.Client(), userID)
		if err != nil {
			return err
		}
		if _, err := s.insertLedgerWithMetadata(ctx, tx, userID, 0, "", "expire", available.Neg(), state, "expired "+grant.GrantType+" grant", 0, expireGrantLedgerKey(grant.ID), ledgerMetadata{
			BalanceBucket:      grant.GrantType,
			SourceType:         grant.SourceType,
			SourceID:           grant.SourceID,
			BucketBalanceAfter: balanceBucketAfter(summary, grant.GrantType),
			ExpiresAt:          grant.ExpiresAt,
		}); err != nil {
			if repoent.IsConstraintError(err) {
				continue
			}
			return err
		}
	}
	return nil
}

type balanceBucketAccumulator struct {
	Bucket       string
	Source       string
	Available    decimal.Decimal
	Frozen       decimal.Decimal
	ExpiresAt    *time.Time
	ReminderDays int
}

func accumulateBalanceBucket(buckets map[string]*balanceBucketAccumulator, bucket, source string, available, frozen decimal.Decimal, expiresAt *time.Time, reminderDays int) {
	key := strings.TrimSpace(bucket)
	if key == "" {
		key = "gift"
	}
	current := buckets[key]
	if current == nil {
		current = &balanceBucketAccumulator{Bucket: key}
		buckets[key] = current
	}
	current.Available = current.Available.Add(available)
	current.Frozen = current.Frozen.Add(frozen)
	if current.Source == "" {
		current.Source = source
	}
	if expiresAt != nil {
		expires := *expiresAt
		if current.ExpiresAt == nil || expires.Before(*current.ExpiresAt) {
			current.ExpiresAt = &expires
			current.ReminderDays = reminderDays
		}
	}
}

func formatBalanceBuckets(buckets map[string]*balanceBucketAccumulator, scale int32, now time.Time) []domainbilling.BalanceBucket {
	items := make([]domainbilling.BalanceBucket, 0, len(buckets))
	for _, bucket := range buckets {
		if !bucket.Available.IsPositive() && !bucket.Frozen.IsPositive() {
			continue
		}
		item := domainbilling.BalanceBucket{
			Bucket:          bucket.Bucket,
			Label:           balanceBucketLabel(bucket.Bucket),
			AvailablePoints: bucket.Available.Round(scale).StringFixed(scale),
			FrozenPoints:    bucket.Frozen.Round(scale).StringFixed(scale),
			ExpiresAt:       bucket.ExpiresAt,
			ExpireWarning:   balanceBucketExpireWarning(now, bucket.ExpiresAt, bucket.ReminderDays),
			SourceType:      bucket.Source,
			SortOrder:       grantPriority(bucket.Bucket),
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].SortOrder != items[j].SortOrder {
			return items[i].SortOrder < items[j].SortOrder
		}
		if items[i].ExpiresAt == nil && items[j].ExpiresAt != nil {
			return false
		}
		if items[i].ExpiresAt != nil && items[j].ExpiresAt == nil {
			return true
		}
		if items[i].ExpiresAt != nil && items[j].ExpiresAt != nil && !items[i].ExpiresAt.Equal(*items[j].ExpiresAt) {
			return items[i].ExpiresAt.Before(*items[j].ExpiresAt)
		}
		return items[i].Bucket < items[j].Bucket
	})
	return items
}

func balanceBucketExpireWarning(now time.Time, expiresAt *time.Time, reminderDays int) bool {
	if expiresAt == nil || now.After(*expiresAt) {
		return false
	}
	if reminderDays <= 0 {
		reminderDays = 2
	}
	return expiresAt.Sub(now) <= time.Duration(reminderDays)*24*time.Hour
}

func balanceBucketLabel(bucket string) string {
	switch bucket {
	case "trial":
		return "体验额度"
	case "subscription":
		return "订阅额度"
	case "recharge":
		return "充值额度"
	case "gift":
		return "赠送额度"
	default:
		return bucket
	}
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
		TrialPoints:        decimal.Zero.Round(s.scale).StringFixed(s.scale),
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

func (s *BillingStore) insertLedger(ctx context.Context, tx *repoent.Tx, userID, apiKeyID int64, taskID, ledgerType string, change decimal.Decimal, state decimalState, reason string, operatorAdminID int64, idempotencyKey string) (int64, error) {
	return s.insertLedgerWithMetadata(ctx, tx, userID, apiKeyID, taskID, ledgerType, change, state, reason, operatorAdminID, idempotencyKey, ledgerMetadata{})
}

type ledgerMetadata struct {
	BalanceBucket      string
	SourceType         string
	SourceID           *int64
	BucketBalanceAfter string
	ExpiresAt          *time.Time
}

func (s *BillingStore) insertLedgerWithMetadata(ctx context.Context, tx *repoent.Tx, userID, apiKeyID int64, taskID, ledgerType string, change decimal.Decimal, state decimalState, reason string, operatorAdminID int64, idempotencyKey string, metadata ledgerMetadata) (int64, error) {
	metadata = normalizeLedgerMetadata(ledgerType, state, s.scale, metadata)
	builder := tx.PointLedger.Create().
		SetUserID(userID).
		SetLedgerType(ledgerType).
		SetChangePoints(change.Round(s.scale).StringFixed(s.scale)).
		SetBalanceAfter(state.Available.Round(s.scale).StringFixed(s.scale)).
		SetFrozenAfter(state.Frozen.Round(s.scale).StringFixed(s.scale)).
		SetBalanceBucket(metadata.BalanceBucket).
		SetSourceType(metadata.SourceType).
		SetBucketBalanceAfter(metadata.BucketBalanceAfter).
		SetReason(reason)
	if metadata.SourceID != nil {
		builder.SetSourceID(*metadata.SourceID)
	}
	if metadata.ExpiresAt != nil {
		builder.SetExpiresAt(*metadata.ExpiresAt)
	}
	if apiKeyID > 0 {
		builder.SetAPIKeyID(apiKeyID)
	}
	if strings.TrimSpace(taskID) != "" {
		parsedTaskID, err := uuid.Parse(taskID)
		if err != nil {
			return 0, err
		}
		builder.SetTaskID(parsedTaskID)
	}
	if operatorAdminID > 0 {
		builder.SetOperatorAdminID(operatorAdminID)
	}
	if strings.TrimSpace(idempotencyKey) != "" {
		builder.SetIdempotencyKey(idempotencyKey)
	}
	ledger, err := builder.Save(ctx)
	if err != nil {
		return 0, err
	}
	return int64(ledger.ID), nil
}

func (s *BillingStore) insertPaymentOrderLedger(ctx context.Context, tx *repoent.Tx, userID, orderID int64, ledgerType string, change decimal.Decimal, state decimalState, reason string, operatorAdminID int64, idempotencyKey string) (int64, error) {
	metadata := normalizeLedgerMetadata(ledgerType, state, s.scale, ledgerMetadata{SourceID: pointerInt64(orderID)})
	builder := tx.PointLedger.Create().
		SetUserID(userID).
		SetOrderID(orderID).
		SetLedgerType(ledgerType).
		SetChangePoints(change.Round(s.scale).StringFixed(s.scale)).
		SetBalanceAfter(state.Available.Round(s.scale).StringFixed(s.scale)).
		SetFrozenAfter(state.Frozen.Round(s.scale).StringFixed(s.scale)).
		SetBalanceBucket(metadata.BalanceBucket).
		SetSourceType(metadata.SourceType).
		SetBucketBalanceAfter(metadata.BucketBalanceAfter).
		SetReason(reason)
	if metadata.SourceID != nil {
		builder.SetSourceID(*metadata.SourceID)
	}
	if operatorAdminID > 0 {
		builder.SetOperatorAdminID(operatorAdminID)
	}
	if strings.TrimSpace(idempotencyKey) != "" {
		builder.SetIdempotencyKey(idempotencyKey)
	}
	ledger, err := builder.Save(ctx)
	if err != nil {
		return 0, err
	}
	return int64(ledger.ID), nil
}

func normalizeLedgerMetadata(ledgerType string, state decimalState, scale int32, metadata ledgerMetadata) ledgerMetadata {
	entry := domainbilling.LedgerEntry{LedgerType: ledgerType}
	if strings.TrimSpace(metadata.BalanceBucket) == "" {
		metadata.BalanceBucket = domainbilling.LedgerBucketType(entry)
	}
	if strings.TrimSpace(metadata.SourceType) == "" {
		metadata.SourceType = domainbilling.LedgerSourceType(entry)
	}
	if strings.TrimSpace(metadata.BucketBalanceAfter) == "" {
		metadata.BucketBalanceAfter = state.Available.Round(scale).StringFixed(scale)
	}
	return metadata
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
		TrialPoints:        decimal.Zero.Round(s.scale).StringFixed(s.scale),
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
		{PlanCode: "basic-monthly", PlanName: "Basic Monthly", Status: "active", PriceCNY: "19.90000", Points: "100.00000", BonusPoints: "0.00000", CreditExpiryEnabled: true, DurationDays: nullablePlanDurationDays(true, 30), Currency: "CNY", PlanType: "points_package", PurchaseEnabled: true, SortOrder: 1, CreatedAt: now, UpdatedAt: now},
		{PlanCode: "plus-monthly", PlanName: "Plus Monthly", Status: "active", PriceCNY: "49.90000", Points: "300.00000", BonusPoints: "30.00000", CreditExpiryEnabled: true, DurationDays: nullablePlanDurationDays(true, 30), Currency: "CNY", PlanType: "points_package", PurchaseEnabled: true, SortOrder: 2, CreatedAt: now, UpdatedAt: now},
	}
	for index, item := range defaults {
		sortOrder := item.SortOrder
		if sortOrder <= 0 {
			sortOrder = index + 1
		}
		if _, err := s.client.SubscriptionPlan.Create().
			SetPlanCode(item.PlanCode).
			SetPlanName(item.PlanName).
			SetPlanType(item.PlanType).
			SetPurchaseEnabled(item.PurchaseEnabled).
			SetStatus(item.Status).
			SetPriceCny(item.PriceCNY).
			SetPoints(item.Points).
			SetBonusPoints(item.BonusPoints).
			SetCreditExpiryEnabled(item.CreditExpiryEnabled).
			SetNillableDurationDays(item.DurationDays).
			SetCurrency(item.Currency).
			SetSortOrder(sortOrder).
			SetMetadata(subscriptionPlanMetadata(item.PlanType, item.PurchaseEnabled)).
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
	if _, err := s.insertLedger(ctx, tx, order.UserID, 0, "", "order_paid", mustDecimal(order.Points).Add(mustDecimal(order.BonusPoints)), state, "payment order "+order.OrderNo, 0, "order:"+order.OrderNo+":paid"); err != nil {
		return decimalState{}, err
	}
	return state, nil
}

func (s *BillingStore) grantRechargeOrderCredits(ctx context.Context, tx *repoent.Tx, order *repoent.PaymentOrder) (decimalState, int64, error) {
	total := mustDecimal(order.Points).Add(mustDecimal(order.BonusPoints))
	if !total.IsPositive() {
		return decimalState{}, 0, errs.Internal("payment order points are invalid")
	}
	if _, err := s.createWalletGrant(ctx, tx, order.UserID, "recharge", "payment_order", pointerInt64(int64(order.ID)), total, nil, map[string]any{"order_no": order.OrderNo}); err != nil {
		return decimalState{}, 0, err
	}
	state, _, err := s.currentStateWithDetails(ctx, tx.Client(), order.UserID)
	if err != nil {
		return decimalState{}, 0, err
	}
	ledgerID, err := s.insertPaymentOrderLedger(ctx, tx, order.UserID, int64(order.ID), "recharge", total, state, "cashier order "+order.OrderNo, 0, "cashier:"+order.OrderNo+":recharge")
	if err != nil {
		return decimalState{}, 0, err
	}
	return state, ledgerID, nil
}

func (s *BillingStore) completeRechargeOrderInTx(ctx context.Context, tx *repoent.Tx, order *repoent.PaymentOrder, provider, tradeNo string, payload map[string]any, reconciliationSource string) (domainbilling.PaymentOrder, error) {
	payload = cloneMap(payload)
	if payload == nil {
		payload = map[string]any{}
	}
	payload["previous_local_status"] = order.Status
	payload["reconciliation_source"] = reconciliationSource
	if order.Status == "completed" {
		existingTradeNo := ""
		if order.TradeNo != nil {
			existingTradeNo = *order.TradeNo
		}
		if err := billingservice.ValidateCompletedPaymentTrade(existingTradeNo, tradeNo); err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		if err := s.ensureWebhookEvent(ctx, tx, provider, tradeNo, int64(order.ID), payload); err != nil {
			return domainbilling.PaymentOrder{}, err
		}
		return s.mapPaymentOrder(ctx, order), nil
	}
	existingTradeNo := ""
	if order.TradeNo != nil {
		existingTradeNo = *order.TradeNo
	}
	if err := billingservice.ValidateInitializedPaymentTrade(existingTradeNo, tradeNo); err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	if !billingservice.PaymentSuccessCanRecoverStatus(order.Status) {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order cannot transition to completed")
	}
	now := time.Now().UTC()
	if err := s.ensureWebhookEvent(ctx, tx, provider, tradeNo, int64(order.ID), payload); err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	_, ledgerID, err := s.grantRechargeOrderCredits(ctx, tx, order)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	if _, err := tx.PaymentOrder.UpdateOneID(order.ID).
		SetStatus("completed").
		SetProvider(provider).
		SetTradeNo(tradeNo).
		SetPaidAt(now).
		SetCompletedAt(now).
		SetLedgerID(ledgerID).
		ClearClosedAt().
		ClearFailureReason().
		Save(ctx); err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	updated, err := tx.PaymentOrder.Query().Where(paymentorder.IDEQ(order.ID)).Only(ctx)
	if err != nil {
		return domainbilling.PaymentOrder{}, err
	}
	return s.mapPaymentOrder(ctx, updated), nil
}

func isEntCashierRechargeOrder(order *repoent.PaymentOrder) bool {
	return strings.TrimSpace(order.VisibleMethod) != "" || strings.TrimSpace(order.PurchaseType) == "custom_amount" || order.ProviderInstanceID != nil || len(order.PaymentDisplay) > 0
}

func (s *BillingStore) ensureWebhookEvent(ctx context.Context, tx *repoent.Tx, provider, tradeNo string, orderID int64, payload map[string]any) error {
	if event, err := tx.PaymentWebhookEvent.Query().Where(paymentwebhookevent.ProviderEQ(provider), paymentwebhookevent.TradeNoEQ(tradeNo)).Only(ctx); err == nil {
		if event.PaymentOrderID == nil || *event.PaymentOrderID != orderID {
			return errs.New(http.StatusConflict, errs.CodeConflict, "payment provider trade belongs to a different order")
		}
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
		ID:                  int64(plan.ID),
		PlanCode:            plan.PlanCode,
		PlanName:            plan.PlanName,
		PlanType:            subscriptionPlanType(plan.PlanType, plan.Metadata),
		PurchaseEnabled:     subscriptionPlanPurchaseEnabled(plan.PurchaseEnabled, plan.Metadata),
		Status:              plan.Status,
		PriceCNY:            plan.PriceCny,
		Points:              plan.Points,
		BonusPoints:         plan.BonusPoints,
		CreditExpiryEnabled: plan.CreditExpiryEnabled,
		DurationDays:        nullablePlanDurationDays(plan.CreditExpiryEnabled, plan.DurationDays),
		Currency:            plan.Currency,
		SortOrder:           plan.SortOrder,
		Description:         plan.Description,
		CreatedAt:           plan.CreatedAt,
		UpdatedAt:           plan.UpdatedAt,
	}
}

func nullablePlanDurationDays(expiryEnabled bool, days int) *int {
	if !expiryEnabled {
		return nil
	}
	return &days
}

func effectivePlanCreditExpiryEnabled(value *bool) bool {
	return value == nil || *value
}

func subscriptionPlanType(value string, metadata map[string]any) string {
	if metadata != nil {
		if metadataValue, ok := metadata["plan_type"].(string); ok && strings.TrimSpace(metadataValue) != "" {
			if strings.EqualFold(metadataValue, "subscription") {
				return "subscription"
			}
			return "points_package"
		}
	}
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "subscription" {
		return "subscription"
	}
	return "points_package"
}

func subscriptionPlanPurchaseEnabled(value bool, metadata map[string]any) bool {
	if metadata != nil {
		switch metadataValue := metadata["purchase_enabled"].(type) {
		case bool:
			return metadataValue
		case string:
			parsed, err := strconv.ParseBool(strings.TrimSpace(metadataValue))
			if err == nil {
				return parsed
			}
		}
	}
	return value
}

func subscriptionPlanMetadata(planType string, purchaseEnabled bool) map[string]any {
	return map[string]any{
		"plan_type":        strings.ToLower(strings.TrimSpace(planType)),
		"purchase_enabled": purchaseEnabled,
	}
}

func (s *BillingStore) paymentOrderRefundTotal(order *repoent.PaymentOrder) (decimal.Decimal, error) {
	total := mustDecimal(order.Points).Add(mustDecimal(order.BonusPoints)).Round(s.scale)
	if !total.IsPositive() {
		return decimal.Decimal{}, errs.Internal("payment order points are invalid")
	}
	return total, nil
}

type paymentOrderRefundPlan struct {
	RefundAmountCNY       decimal.Decimal
	RefundPoints          decimal.Decimal
	NextRefundedAmountCNY decimal.Decimal
	NextRefundedPoints    decimal.Decimal
	FullyRefunded         bool
}

func (s *BillingStore) paymentOrderRefundPlan(order *repoent.PaymentOrder, req domainbilling.RefundPaymentOrderRequest, payload map[string]any) (paymentOrderRefundPlan, error) {
	totalAmountCNY, err := decimal.NewFromString(strings.TrimSpace(order.AmountCny))
	if err != nil || !totalAmountCNY.IsPositive() {
		return paymentOrderRefundPlan{}, errs.Internal("payment order amount is invalid")
	}
	totalPoints, err := s.paymentOrderRefundTotal(order)
	if err != nil {
		return paymentOrderRefundPlan{}, err
	}
	refundedAmountCNY := decimalFromPayload(payload, "refunded_amount_cny", s.scale)
	refundedPoints := decimalFromPayload(payload, "refunded_points", s.scale)
	remainingAmountCNY := totalAmountCNY.Sub(refundedAmountCNY).Round(s.scale)
	remainingPoints := totalPoints.Sub(refundedPoints).Round(s.scale)
	if !remainingAmountCNY.IsPositive() || !remainingPoints.IsPositive() {
		return paymentOrderRefundPlan{}, errs.New(http.StatusConflict, errs.CodeConflict, "payment order has no refundable amount")
	}
	refundAmountCNY := remainingAmountCNY
	if strings.TrimSpace(req.RefundAmountCNY) != "" {
		parsed, parseErr := decimal.NewFromString(strings.TrimSpace(req.RefundAmountCNY))
		if parseErr != nil || !parsed.IsPositive() {
			return paymentOrderRefundPlan{}, errs.BadRequest("refund_amount_cny must be positive")
		}
		refundAmountCNY = parsed.Round(s.scale)
	}
	if refundAmountCNY.GreaterThan(remainingAmountCNY) {
		return paymentOrderRefundPlan{}, errs.New(http.StatusConflict, errs.CodeConflict, "refund amount exceeds refundable amount")
	}
	refundPoints := refundAmountCNY.Mul(totalPoints).Div(totalAmountCNY).Round(s.scale)
	if refundAmountCNY.Equal(remainingAmountCNY) {
		refundPoints = remainingPoints
	}
	if !refundPoints.IsPositive() || refundPoints.GreaterThan(remainingPoints) {
		return paymentOrderRefundPlan{}, errs.New(http.StatusConflict, errs.CodeConflict, "refund points exceed refundable balance")
	}
	nextRefundedAmountCNY := refundedAmountCNY.Add(refundAmountCNY).Round(s.scale)
	nextRefundedPoints := refundedPoints.Add(refundPoints).Round(s.scale)
	fullyRefunded := !totalAmountCNY.Sub(nextRefundedAmountCNY).Round(s.scale).IsPositive() || !totalPoints.Sub(nextRefundedPoints).Round(s.scale).IsPositive()
	return paymentOrderRefundPlan{
		RefundAmountCNY:       refundAmountCNY,
		RefundPoints:          refundPoints,
		NextRefundedAmountCNY: nextRefundedAmountCNY,
		NextRefundedPoints:    nextRefundedPoints,
		FullyRefunded:         fullyRefunded,
	}, nil
}

func (s *BillingStore) refundableRechargeGrant(ctx context.Context, order *repoent.PaymentOrder) (*repoent.WalletGrant, error) {
	grant, err := s.client.WalletGrant.Query().
		Where(
			walletgrant.UserIDEQ(order.UserID),
			walletgrant.GrantTypeEQ("recharge"),
			walletgrant.SourceTypeEQ("payment_order"),
			walletgrant.SourceIDEQ(int64(order.ID)),
			walletgrant.StatusEQ("active"),
		).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return nil, errs.New(http.StatusConflict, errs.CodeConflict, "payment order recharge balance is insufficient for refund")
		}
		return nil, err
	}
	return grant, nil
}

func (s *BillingStore) refundableRechargeGrantInTx(ctx context.Context, tx *repoent.Tx, order *repoent.PaymentOrder) (*repoent.WalletGrant, error) {
	grant, err := tx.WalletGrant.Query().
		Where(
			walletgrant.UserIDEQ(order.UserID),
			walletgrant.GrantTypeEQ("recharge"),
			walletgrant.SourceTypeEQ("payment_order"),
			walletgrant.SourceIDEQ(int64(order.ID)),
			walletgrant.StatusEQ("active"),
		).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return nil, errs.New(http.StatusConflict, errs.CodeConflict, "payment order recharge balance is insufficient for refund")
		}
		return nil, err
	}
	return grant, nil
}

func refundFreezeAmount(metadata map[string]any, refundTradeNo string, scale int32) decimal.Decimal {
	if metadata == nil {
		return decimal.Zero
	}
	tradeNo := strings.TrimSpace(fmt.Sprint(metadata["refund_freeze_trade_no"]))
	if tradeNo == "" || tradeNo == "<nil>" || (strings.TrimSpace(refundTradeNo) != "" && tradeNo != strings.TrimSpace(refundTradeNo)) {
		return decimal.Zero
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(fmt.Sprint(metadata["refund_freeze_points"])))
	if err != nil || !amount.IsPositive() {
		return decimal.Zero
	}
	return amount.Round(scale)
}

func ensureRefundFreezeMatches(metadata map[string]any, refundTradeNo string, refundAmountCNY, refundPoints decimal.Decimal, scale int32) error {
	frozenPoints := refundFreezeAmount(metadata, refundTradeNo, scale)
	if !frozenPoints.IsPositive() || !frozenPoints.Equal(refundPoints.Round(scale)) {
		return errs.New(http.StatusConflict, errs.CodePaymentAmountMismatch, "payment refund amount does not match the pending refund")
	}
	rawAmount := strings.TrimSpace(fmt.Sprint(metadata["refund_freeze_amount_cny"]))
	if rawAmount == "" || rawAmount == "<nil>" {
		return nil
	}
	frozenAmount, err := decimal.NewFromString(rawAmount)
	if err != nil || !frozenAmount.Round(scale).Equal(refundAmountCNY.Round(scale)) {
		return errs.New(http.StatusConflict, errs.CodePaymentAmountMismatch, "payment refund amount does not match the pending refund")
	}
	return nil
}

func decimalFromPayload(payload map[string]any, key string, scale int32) decimal.Decimal {
	if payload == nil {
		return decimal.Zero
	}
	value := strings.TrimSpace(fmt.Sprint(payload[key]))
	if value == "" || value == "<nil>" {
		return decimal.Zero
	}
	parsed, err := decimal.NewFromString(value)
	if err != nil {
		return decimal.Zero
	}
	return parsed.Round(scale)
}

func refundRecordExists(payload map[string]any, refundTradeNo string) bool {
	refundTradeNo = strings.TrimSpace(refundTradeNo)
	if refundTradeNo == "" || payload == nil {
		return false
	}
	for _, item := range refundRecords(payload) {
		if strings.TrimSpace(fmt.Sprint(item["refund_trade_no"])) == refundTradeNo {
			return true
		}
	}
	return false
}

func refundRecords(payload map[string]any) []map[string]any {
	if payload == nil {
		return nil
	}
	raw, ok := payload["refund_records"].([]any)
	if !ok {
		if typed, typedOK := payload["refund_records"].([]map[string]any); typedOK {
			return typed
		}
		return nil
	}
	records := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if record, ok := item.(map[string]any); ok {
			records = append(records, record)
		}
	}
	return records
}

func appendRefundRecord(payload map[string]any, refundTradeNo string, refundAmountCNY decimal.Decimal, refundPoints decimal.Decimal, reason string, operatorAdminID int64, scale int32) []map[string]any {
	records := append([]map[string]any{}, refundRecords(payload)...)
	records = append(records, map[string]any{
		"refund_trade_no":   strings.TrimSpace(refundTradeNo),
		"refund_amount_cny": refundAmountCNY.Round(scale).StringFixed(scale),
		"refund_points":     refundPoints.Round(scale).StringFixed(scale),
		"reason":            strings.TrimSpace(reason),
		"operator_admin_id": operatorAdminID,
		"refunded_at":       time.Now().UTC().Format(time.RFC3339Nano),
	})
	return records
}

func boolString(ok bool, yes string, no string) string {
	if ok {
		return yes
	}
	return no
}

func (s *BillingStore) mapPaymentOrder(ctx context.Context, order *repoent.PaymentOrder) domainbilling.PaymentOrder {
	item := domainbilling.PaymentOrder{
		ID:             int64(order.ID),
		OrderNo:        order.OrderNo,
		UserID:         order.UserID,
		PlanID:         order.PlanID,
		Provider:       order.Provider,
		PurchaseType:   order.PurchaseType,
		VisibleMethod:  order.VisibleMethod,
		ProviderType:   order.ProviderType,
		PaymentDisplay: cloneMap(order.PaymentDisplay),
		Status:         order.Status,
		Currency:       order.Currency,
		AmountCNY:      order.AmountCny,
		Points:         order.Points,
		BonusPoints:    order.BonusPoints,
		ExpiresAt:      order.ExpiresAt,
		PaidAt:         order.PaidAt,
		CompletedAt:    order.CompletedAt,
		ClosedAt:       order.ClosedAt,
		RefundedAt:     order.RefundedAt,
		CreatedAt:      order.CreatedAt,
		UpdatedAt:      order.UpdatedAt,
	}
	if order.ProviderInstanceID != nil {
		item.ProviderInstanceID = *order.ProviderInstanceID
	}
	if order.LedgerID != nil {
		item.LedgerID = *order.LedgerID
	}
	if order.IdempotencyKey != nil {
		item.IdempotencyKey = *order.IdempotencyKey
	}
	applyPaymentOrderProviderPayload(&item, order.ProviderPayload)
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

func paymentOrderProviderPayload(purchaseType, visibleMethod, providerType string, providerInstanceID int64, paymentDisplay map[string]any) map[string]any {
	payload := map[string]any{}
	purchaseType = strings.ToLower(strings.TrimSpace(purchaseType))
	if purchaseType != "" {
		payload["purchase_type"] = purchaseType
	}
	visibleMethod = strings.ToLower(strings.TrimSpace(visibleMethod))
	if visibleMethod != "" {
		payload["visible_method"] = visibleMethod
	}
	providerType = strings.ToLower(strings.TrimSpace(providerType))
	if providerType != "" {
		payload["provider_type"] = providerType
	}
	if providerInstanceID > 0 {
		payload["provider_instance_id"] = providerInstanceID
	}
	if len(paymentDisplay) > 0 {
		payload["payment_display"] = paymentDisplay
	}
	return payload
}

func paymentOrderProviderSnapshot(provider, providerType string, providerInstanceID int64) map[string]any {
	snapshot := map[string]any{
		"provider":      strings.ToLower(strings.TrimSpace(provider)),
		"provider_type": strings.ToLower(strings.TrimSpace(providerType)),
	}
	if providerInstanceID > 0 {
		snapshot["provider_instance_id"] = providerInstanceID
	}
	return snapshot
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

func applyPaymentOrderProviderPayload(order *domainbilling.PaymentOrder, payload map[string]any) {
	if order == nil {
		return
	}
	if purchaseType := strings.TrimSpace(fmt.Sprint(payload["purchase_type"])); purchaseType != "" && purchaseType != "<nil>" {
		order.PurchaseType = purchaseType
	}
	if visibleMethod := strings.TrimSpace(fmt.Sprint(payload["visible_method"])); visibleMethod != "" && visibleMethod != "<nil>" {
		order.VisibleMethod = visibleMethod
	}
	if providerType := strings.TrimSpace(fmt.Sprint(payload["provider_type"])); providerType != "" && providerType != "<nil>" {
		order.ProviderType = providerType
	}
	if refundTradeNo := strings.TrimSpace(fmt.Sprint(payload["refund_trade_no"])); refundTradeNo != "" && refundTradeNo != "<nil>" {
		order.RefundTradeNo = refundTradeNo
	}
	if providerRefund, ok := payload["provider_refund"].(map[string]any); ok {
		if channelRefundNo := strings.TrimSpace(fmt.Sprint(providerRefund["channel_refund_no"])); channelRefundNo != "" && channelRefundNo != "<nil>" {
			order.ChannelRefundNo = channelRefundNo
		}
		if channelRefundStatus := strings.TrimSpace(fmt.Sprint(providerRefund["channel_refund_status"])); channelRefundStatus != "" && channelRefundStatus != "<nil>" {
			order.ChannelRefundStatus = strings.ToLower(channelRefundStatus)
		}
	}
	if refundedAmountCNY := strings.TrimSpace(fmt.Sprint(payload["refunded_amount_cny"])); refundedAmountCNY != "" && refundedAmountCNY != "<nil>" {
		order.RefundedAmountCNY = refundedAmountCNY
	}
	if refundedPoints := strings.TrimSpace(fmt.Sprint(payload["refunded_points"])); refundedPoints != "" && refundedPoints != "<nil>" {
		order.RefundedPoints = refundedPoints
	}
	if chargebackPoints := strings.TrimSpace(fmt.Sprint(payload["chargeback_points"])); chargebackPoints != "" && chargebackPoints != "<nil>" {
		order.ChargebackPoints = chargebackPoints
	}
	if chargebackReason := strings.TrimSpace(fmt.Sprint(payload["chargeback_reason"])); chargebackReason != "" && chargebackReason != "<nil>" {
		order.ChargebackReason = chargebackReason
	}
	if chargebackKey := strings.TrimSpace(fmt.Sprint(payload["chargeback_idempotency_key"])); chargebackKey != "" && chargebackKey != "<nil>" {
		order.ChargebackKey = chargebackKey
	}
	if chargebackAt := strings.TrimSpace(fmt.Sprint(payload["chargeback_at"])); chargebackAt != "" && chargebackAt != "<nil>" {
		if parsed, err := time.Parse(time.RFC3339Nano, chargebackAt); err == nil {
			order.ChargebackAt = &parsed
		}
	}
	order.ProviderInstanceID = int64FromAny(payload["provider_instance_id"])
	if display, ok := payload["payment_display"].(map[string]any); ok && len(display) > 0 {
		order.PaymentDisplay = display
	}
	if order.PurchaseType == "" && order.PlanCode == "custom_amount" {
		order.PurchaseType = "custom_amount"
	}
	if order.PurchaseType == "" {
		order.PurchaseType = "plan"
	}
	if order.ProviderType == "" {
		order.ProviderType = order.Provider
	}
}

func int64FromAny(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return parsed
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed
	default:
		return 0
	}
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(typed))
		return parsed
	default:
		return 0
	}
}

func (s *BillingStore) mapPaymentWebhookEvent(ctx context.Context, event *repoent.PaymentWebhookEvent) domainbilling.PaymentWebhookEvent {
	item := domainbilling.PaymentWebhookEvent{
		ID:              int64(event.ID),
		ProviderType:    event.Provider,
		Status:          event.Status,
		EventType:       event.EventType,
		SignatureStatus: webhookSignatureStatus(event),
		ResultSummary:   webhookResultSummary(event),
		PayloadPreview:  webhookPayloadPreview(event.Payload),
		ReceivedAt:      event.CreatedAt,
		ProcessedAt:     event.ProcessedAt,
	}
	if event.PaymentOrderID != nil {
		item.OrderID = *event.PaymentOrderID
		if order, err := s.client.PaymentOrder.Query().Where(paymentorder.IDEQ(int(*event.PaymentOrderID))).Only(ctx); err == nil {
			item.OrderNo = order.OrderNo
			if order.FailureReason != nil {
				item.FailureReason = *order.FailureReason
			}
		}
	}
	if item.FailureReason == "" && strings.TrimSpace(event.Payload) != "" {
		var payload map[string]any
		if err := json.Unmarshal([]byte(event.Payload), &payload); err == nil {
			if reason := strings.TrimSpace(fmt.Sprint(payload["failure_reason"])); reason != "" && reason != "<nil>" {
				item.FailureReason = reason
			}
		}
	}
	return item
}

func webhookSignatureStatus(event *repoent.PaymentWebhookEvent) string {
	if event.Status == "failed" {
		return "failed"
	}
	if event.Status == "verified" || event.Status == "processed" {
		return "verified"
	}
	if event.Signature != nil && strings.TrimSpace(*event.Signature) != "" {
		return "recorded"
	}
	return "not_recorded"
}

func webhookResultSummary(event *repoent.PaymentWebhookEvent) string {
	if event.Status == "processed" {
		if event.ProcessedAt != nil {
			return "已完成本地处理"
		}
		return "已处理"
	}
	if event.Status == "failed" {
		return "处理失败，等待人工或自动重试"
	}
	if event.Status == "verified" {
		return "已验签，等待本地落账"
	}
	if event.Status == "received" {
		return "已接收，等待验签"
	}
	return event.Status
}

func webhookPayloadPreview(payload string) string {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return ""
	}
	var decoded any
	if err := json.Unmarshal([]byte(payload), &decoded); err == nil {
		if normalized, marshalErr := json.Marshal(decoded); marshalErr == nil {
			payload = string(normalized)
		}
	}
	const maxLen = 600
	if len(payload) <= maxLen {
		return payload
	}
	return payload[:maxLen] + "..."
}

func grantPriority(grantType string) int {
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
		ID:                 int64(entry.ID),
		UserID:             entry.UserID,
		LedgerType:         entry.LedgerType,
		ChangePoints:       entry.ChangePoints,
		BalanceAfter:       entry.BalanceAfter,
		FrozenAfter:        entry.FrozenAfter,
		BalanceBucket:      entry.BalanceBucket,
		BucketType:         entry.BalanceBucket,
		SourceType:         entry.SourceType,
		BucketBalanceAfter: entry.BucketBalanceAfter,
		Reason:             entry.Reason,
		CreatedAt:          entry.CreatedAt,
	}
	if entry.APIKeyID != nil {
		item.APIKeyID = *entry.APIKeyID
	}
	if entry.TaskID != nil {
		item.TaskID = entry.TaskID.String()
	}
	if entry.OrderID != nil {
		item.OrderID = *entry.OrderID
	}
	if entry.RedeemCodeID != nil {
		item.RedeemCodeID = *entry.RedeemCodeID
	}
	if entry.SourceID != nil {
		item.SourceID = *entry.SourceID
	}
	if entry.ExpiresAt != nil {
		expiresAt := entry.ExpiresAt.Format(time.RFC3339)
		item.ExpiresAt = &expiresAt
	}
	return domainbilling.PopulateLedgerDisplayFields(item)
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

func expireGrantLedgerKey(grantID int) string {
	return fmt.Sprintf("expire:wallet_grant:%d", grantID)
}

func balanceBucketAfter(state billingservice.BalanceState, bucket string) string {
	switch strings.TrimSpace(bucket) {
	case "trial":
		return state.TrialPoints
	case "subscription":
		return state.SubscriptionPoints
	case "recharge":
		return state.RechargePoints
	default:
		return state.GiftPoints
	}
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
		case "23505":
			return pqErr.Constraint == "paymentwebhookevent_provider_trade_no"
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
		(repoent.IsConstraintError(err) && strings.Contains(message, "payment_webhook_events.provider") && strings.Contains(message, "payment_webhook_events.trade_no")) ||
		(repoent.IsConstraintError(err) && strings.Contains(message, "idempotency"))
}
