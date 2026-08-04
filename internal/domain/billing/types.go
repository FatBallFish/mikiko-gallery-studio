package billing

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type EstimateRequest struct {
	TaskType                  string
	AbstractModel             string
	RouteModelCode            string
	SizeMode                  string
	AspectRatio               string
	BaseResolution            string
	Quality                   string
	OutputFormat              string
	OutputCompression         int
	Moderation                string
	RequestedSize             string
	RequestedOutputImageCount int
	ReferenceImageCount       int
	UserGroupCode             string
	UserGroupCodes            []string
	UserGroupMultiplier       string
}

type EstimateResult struct {
	BaseResolution            string          `json:"base_resolution"`
	EstimatedPoints           string          `json:"estimated_points"`
	ChargedPoints             string          `json:"charged_points,omitempty"`
	DisplayPoints             string          `json:"display_points,omitempty"`
	Sufficient                bool            `json:"sufficient"`
	Balance                   *BalanceSummary `json:"balance,omitempty"`
	InsufficientPoints        string          `json:"insufficient_points"`
	UserGroupMultiplier       string          `json:"user_group_multiplier"`
	RequestedOutputImageCount int             `json:"requested_output_image_count"`
	ReferenceImageCount       int             `json:"reference_image_count"`
	PricingSnapshot           PricingSnapshot `json:"-"`
}

type PricingSnapshot struct {
	AbstractModel             string `json:"abstract_model"`
	RouteModelCode            string `json:"route_model_code,omitempty"`
	TaskType                  string `json:"task_type"`
	SizeMode                  string `json:"size_mode,omitempty"`
	AspectRatio               string `json:"aspect_ratio,omitempty"`
	BaseResolution            string `json:"base_resolution"`
	Quality                   string `json:"quality,omitempty"`
	OutputFormat              string `json:"output_format,omitempty"`
	OutputCompression         int    `json:"output_compression,omitempty"`
	Moderation                string `json:"moderation,omitempty"`
	RequestedSize             string `json:"requested_size,omitempty"`
	RequestedOutputImageCount int    `json:"requested_output_image_count"`
	ReferenceImageCount       int    `json:"reference_image_count"`
	UserGroupCode             string `json:"user_group_code,omitempty"`
	UserGroupMultiplier       string `json:"user_group_multiplier"`
	BaseUnitPoints            string `json:"base_unit_points"`
	TaskMultiplier            string `json:"task_multiplier"`
	ReferenceExtraMultiplier  string `json:"reference_extra_multiplier"`
	EstimatedPoints           string `json:"estimated_points"`
}

type BalanceSummary struct {
	AvailablePoints     string                   `json:"available_points"`
	FrozenPoints        string                   `json:"frozen_points"`
	TrialPoints         string                   `json:"trial_points,omitempty"`
	SubscriptionPoints  string                   `json:"subscription_points,omitempty"`
	GiftPoints          string                   `json:"gift_points,omitempty"`
	RechargePoints      string                   `json:"recharge_points,omitempty"`
	Buckets             []BalanceBucket          `json:"buckets,omitempty"`
	UserGroupMultiplier string                   `json:"user_group_multiplier"`
	CNYPerPoint         string                   `json:"cny_per_point"`
	ActiveSubscription  *UserSubscriptionSummary `json:"active_subscription,omitempty"`
	NextExpiringGrant   *GrantExpirySummary      `json:"next_expiring_grant,omitempty"`
}

type BalanceBucket struct {
	Bucket          string     `json:"bucket"`
	Label           string     `json:"label,omitempty"`
	AvailablePoints string     `json:"available_points"`
	FrozenPoints    string     `json:"frozen_points,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	ExpireWarning   bool       `json:"expire_warning"`
	SourceType      string     `json:"source_type,omitempty"`
	SortOrder       int        `json:"sort_order,omitempty"`
}

type GrantExpirySummary struct {
	GrantID         int64      `json:"grant_id"`
	GrantType       string     `json:"grant_type"`
	AvailablePoints string     `json:"available_points"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
}

type SubscriptionPlan struct {
	ID              int64     `json:"id"`
	PlanCode        string    `json:"plan_code"`
	PlanName        string    `json:"plan_name"`
	PlanType        string    `json:"plan_type,omitempty"`
	PurchaseEnabled bool      `json:"purchase_enabled"`
	Status          string    `json:"status"`
	PriceCNY        string    `json:"price_cny"`
	Points          string    `json:"points"`
	BonusPoints     string    `json:"bonus_points"`
	DurationDays    int       `json:"duration_days"`
	Currency        string    `json:"currency"`
	SortOrder       int       `json:"sort_order,omitempty"`
	Description     string    `json:"description,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type CreateSubscriptionPlanRequest struct {
	PlanCode        string `json:"plan_code"`
	PlanName        string `json:"plan_name"`
	PlanType        string `json:"plan_type"`
	PurchaseEnabled bool   `json:"purchase_enabled"`
	Status          string `json:"status"`
	PriceCNY        string `json:"price_cny"`
	Points          string `json:"points"`
	BonusPoints     string `json:"bonus_points"`
	DurationDays    int    `json:"duration_days"`
	Currency        string `json:"currency"`
	SortOrder       int    `json:"sort_order"`
	Description     string `json:"description"`
}

type UpdateSubscriptionPlanRequest struct {
	PlanID          int64
	PlanName        string `json:"plan_name"`
	PlanType        string `json:"plan_type"`
	PurchaseEnabled bool   `json:"purchase_enabled"`
	Status          string `json:"status"`
	PriceCNY        string `json:"price_cny"`
	Points          string `json:"points"`
	BonusPoints     string `json:"bonus_points"`
	DurationDays    int    `json:"duration_days"`
	Currency        string `json:"currency"`
	SortOrder       int    `json:"sort_order"`
	Description     string `json:"description"`
}

type UserSubscriptionSummary struct {
	ID                 int64      `json:"id"`
	PlanID             int64      `json:"plan_id"`
	PlanCode           string     `json:"plan_code"`
	PlanName           string     `json:"plan_name"`
	Status             string     `json:"status"`
	StartedAt          time.Time  `json:"started_at"`
	CurrentPeriodStart time.Time  `json:"current_period_start"`
	CurrentPeriodEnd   time.Time  `json:"current_period_end"`
	ExpiredAt          *time.Time `json:"expired_at,omitempty"`
	CanceledAt         *time.Time `json:"canceled_at,omitempty"`
	GrantedPoints      string     `json:"granted_points"`
	RemainingPoints    string     `json:"remaining_points"`
}

type PaymentOrder struct {
	ID                  int64          `json:"id"`
	OrderNo             string         `json:"order_no"`
	UserID              int64          `json:"user_id,omitempty"`
	PlanID              int64          `json:"plan_id"`
	PlanCode            string         `json:"plan_code"`
	PlanName            string         `json:"plan_name"`
	Provider            string         `json:"provider"`
	PurchaseType        string         `json:"purchase_type,omitempty"`
	VisibleMethod       string         `json:"visible_method,omitempty"`
	ProviderType        string         `json:"provider_type,omitempty"`
	ProviderInstanceID  int64          `json:"provider_instance_id,omitempty"`
	PaymentDisplay      map[string]any `json:"payment_display,omitempty"`
	IdempotencyKey      string         `json:"idempotency_key,omitempty"`
	Status              string         `json:"status"`
	Currency            string         `json:"currency"`
	AmountCNY           string         `json:"amount_cny"`
	Points              string         `json:"points"`
	BonusPoints         string         `json:"bonus_points"`
	TradeNo             string         `json:"trade_no,omitempty"`
	RefundTradeNo       string         `json:"refund_trade_no,omitempty"`
	ChannelRefundNo     string         `json:"channel_refund_no,omitempty"`
	ChannelRefundStatus string         `json:"channel_refund_status,omitempty"`
	RefundedAmountCNY   string         `json:"refunded_amount_cny,omitempty"`
	RefundedPoints      string         `json:"refunded_points,omitempty"`
	ChargebackPoints    string         `json:"chargeback_points,omitempty"`
	ChargebackReason    string         `json:"chargeback_reason,omitempty"`
	ChargebackAt        *time.Time     `json:"chargeback_at,omitempty"`
	ChargebackKey       string         `json:"chargeback_idempotency_key,omitempty"`
	PaymentURL          string         `json:"payment_url,omitempty"`
	QRCode              string         `json:"qr_code,omitempty"`
	ClientToken         string         `json:"client_token,omitempty"`
	FailureReason       string         `json:"failure_reason,omitempty"`
	ExpiresAt           time.Time      `json:"expires_at"`
	PaidAt              *time.Time     `json:"paid_at,omitempty"`
	CompletedAt         *time.Time     `json:"completed_at,omitempty"`
	ClosedAt            *time.Time     `json:"closed_at,omitempty"`
	RefundedAt          *time.Time     `json:"refunded_at,omitempty"`
	LedgerID            int64          `json:"ledger_id,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

type PaymentOrderPage struct {
	Items    []PaymentOrder `json:"items"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
	Total    int            `json:"total"`
}

type PaymentWebhookEvent struct {
	ID                 int64      `json:"id"`
	OrderID            int64      `json:"order_id,omitempty"`
	OrderNo            string     `json:"order_no,omitempty"`
	ProviderType       string     `json:"provider_type"`
	ProviderInstanceID int64      `json:"provider_instance_id,omitempty"`
	Status             string     `json:"status"`
	EventType          string     `json:"event_type,omitempty"`
	FailureReason      string     `json:"failure_reason,omitempty"`
	SignatureStatus    string     `json:"signature_status,omitempty"`
	ResultSummary      string     `json:"result_summary,omitempty"`
	PayloadPreview     string     `json:"payload_preview,omitempty"`
	ReceivedAt         time.Time  `json:"received_at"`
	ProcessedAt        *time.Time `json:"processed_at,omitempty"`
}

type PaymentWebhookEventPage struct {
	Items    []PaymentWebhookEvent `json:"items"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"page_size"`
	Total    int                   `json:"total"`
}

type LedgerEntry struct {
	ID                 int64     `json:"id"`
	UserID             int64     `json:"user_id,omitempty"`
	APIKeyID           int64     `json:"api_key_id,omitempty"`
	TaskID             string    `json:"task_id,omitempty"`
	OrderID            int64     `json:"order_id,omitempty"`
	RedeemCodeID       int64     `json:"redeem_code_id,omitempty"`
	LedgerType         string    `json:"ledger_type"`
	ChangePoints       string    `json:"change_points"`
	BalanceAfter       string    `json:"balance_after"`
	FrozenAfter        string    `json:"frozen_after,omitempty"`
	BalanceBucket      string    `json:"balance_bucket,omitempty"`
	BucketType         string    `json:"bucket_type,omitempty"`
	SourceType         string    `json:"source_type,omitempty"`
	SourceID           any       `json:"source_id,omitempty"`
	BucketBalanceAfter string    `json:"bucket_balance_after,omitempty"`
	ExpiresAt          *string   `json:"expires_at,omitempty"`
	Reason             string    `json:"reason,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	Title              string    `json:"title,omitempty"`
	OccurredAt         string    `json:"occurred_at,omitempty"`
	Amount             string    `json:"amount,omitempty"`
	Type               string    `json:"type,omitempty"`
	Detail             string    `json:"detail,omitempty"`
}

func PopulateLedgerDisplayFields(entry LedgerEntry) LedgerEntry {
	entry.LedgerType = strings.TrimSpace(entry.LedgerType)
	if strings.TrimSpace(entry.BucketType) == "" {
		if strings.TrimSpace(entry.BalanceBucket) != "" {
			entry.BucketType = strings.TrimSpace(entry.BalanceBucket)
		} else {
			entry.BucketType = LedgerBucketType(entry)
		}
	}
	if strings.TrimSpace(entry.BalanceBucket) == "" {
		entry.BalanceBucket = entry.BucketType
	}
	if strings.TrimSpace(entry.SourceType) == "" {
		entry.SourceType = LedgerSourceType(entry)
	}
	if entry.SourceID == nil {
		entry.SourceID = LedgerSourceID(entry)
	}
	entry.Title = LedgerTitle(entry.LedgerType)
	entry.OccurredAt = entry.CreatedAt.Format(time.RFC3339)
	entry.Type = "credit"
	if strings.HasPrefix(strings.TrimSpace(entry.ChangePoints), "-") {
		entry.Type = "debit"
	}
	entry.Amount = formatLedgerAmount(entry.ChangePoints)
	entry.Detail = LedgerDetail(entry)
	return entry
}

func LedgerBucketType(entry LedgerEntry) string {
	switch strings.TrimSpace(entry.LedgerType) {
	case "trial_grant":
		return "trial"
	case "order_paid":
		return "subscription"
	case "recharge", "payment_refund", "redeem", "admin_adjust":
		return "recharge"
	case "reserve", "consume", "refund":
		return "usage"
	case "expire":
		if strings.TrimSpace(entry.BalanceBucket) != "" {
			return strings.TrimSpace(entry.BalanceBucket)
		}
		return "subscription"
	default:
		return "recharge"
	}
}

func LedgerSourceType(entry LedgerEntry) string {
	switch strings.TrimSpace(entry.LedgerType) {
	case "trial_grant":
		return "signup"
	case "order_paid", "recharge", "payment_refund":
		return "payment_order"
	case "redeem":
		return "redeem_code"
	case "reserve", "consume", "refund":
		return "task"
	case "admin_adjust":
		return "admin"
	case "expire":
		return "system"
	default:
		return "system"
	}
}

func LedgerSourceID(entry LedgerEntry) any {
	if entry.TaskID != "" {
		return entry.TaskID
	}
	if entry.OrderID > 0 {
		return entry.OrderID
	}
	if entry.RedeemCodeID > 0 {
		return entry.RedeemCodeID
	}
	if entry.APIKeyID > 0 {
		return entry.APIKeyID
	}
	return nil
}

func LedgerTitle(ledgerType string) string {
	switch strings.TrimSpace(ledgerType) {
	case "trial_grant":
		return "体验额度发放"
	case "order_paid":
		return "订阅额度到账"
	case "recharge":
		return "充值额度到账"
	case "payment_refund":
		return "充值退款"
	case "redeem":
		return "兑换码到账"
	case "reserve":
		return "生成预冻结"
	case "consume":
		return "生成扣费"
	case "refund":
		return "冻结退回"
	case "admin_adjust":
		return "后台积分调整"
	case "expire":
		return "额度过期"
	default:
		return "积分变动"
	}
}

func LedgerDetail(entry LedgerEntry) string {
	parts := []string{fmt.Sprintf("%s / %s", bucketLabel(entry.BucketType), sourceLabel(entry.SourceType))}
	if strings.TrimSpace(entry.Reason) != "" {
		parts = append(parts, strings.TrimSpace(entry.Reason))
	}
	if entry.TaskID != "" {
		parts = append(parts, "任务 "+entry.TaskID)
	} else if entry.OrderID > 0 {
		parts = append(parts, "订单 "+strconv.FormatInt(entry.OrderID, 10))
	} else if entry.RedeemCodeID > 0 {
		parts = append(parts, "兑换码 "+strconv.FormatInt(entry.RedeemCodeID, 10))
	}
	return strings.Join(parts, " · ")
}

func formatLedgerAmount(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "0.00000"
	}
	if strings.HasPrefix(value, "-") || strings.HasPrefix(value, "+") {
		return value
	}
	return "+" + value
}

func bucketLabel(bucket string) string {
	switch strings.TrimSpace(bucket) {
	case "trial":
		return "体验额度"
	case "subscription":
		return "订阅额度"
	case "recharge":
		return "充值额度"
	case "usage":
		return "使用中额度"
	default:
		return "积分余额"
	}
}

func sourceLabel(source string) string {
	switch strings.TrimSpace(source) {
	case "signup":
		return "注册赠送"
	case "payment_order":
		return "支付订单"
	case "redeem_code":
		return "兑换码"
	case "task":
		return "生图任务"
	case "admin":
		return "后台调整"
	default:
		return "系统"
	}
}

type LedgerPage struct {
	Items    []LedgerEntry `json:"items"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
	Total    int           `json:"total"`
}

type APIKeyQuota struct {
	APIKeyTotalQuotaPoints *string
	APIKeyDailyQuotaPoints *string
	APIKeyQuotaDayStart    *time.Time
}

type ReserveRequest struct {
	UserID          int64
	APIKeyID        int64
	TaskID          string
	EstimatedPoints string
	Reason          string
	APIKeyQuota
}

type FinalizeRequest struct {
	UserID          int64
	APIKeyID        int64
	TaskID          string
	EstimatedPoints string
	ActualPoints    string
	Reason          string
}

type AdjustRequest struct {
	UserID          int64
	ChangePoints    string
	Reason          string
	OperatorAdminID int64
	IdempotencyKey  string
}

type CreateOrderRequest struct {
	UserID             int64
	OrderNo            string
	PlanCode           string
	Provider           string
	PurchaseType       string
	VisibleMethod      string
	ProviderType       string
	ProviderInstanceID int64
	PaymentDisplay     map[string]any
	PaymentURL         string
	QRCode             string
	ClientToken        string
	IdempotencyKey     string
}

type CreateCustomAmountOrderRequest struct {
	UserID             int64
	OrderNo            string
	AmountCNY          string
	Provider           string
	CNYPerPoint        string
	PurchaseType       string
	VisibleMethod      string
	ProviderType       string
	ProviderInstanceID int64
	PaymentDisplay     map[string]any
	PaymentURL         string
	QRCode             string
	ClientToken        string
	IdempotencyKey     string
}

type InitializePaymentOrderRequest struct {
	UserID         int64
	OrderID        int64
	PaymentDisplay map[string]any
	PaymentURL     string
	QRCode         string
	ClientToken    string
	TradeNo        string
}

type FailPaymentOrderInitializationRequest struct {
	UserID        int64
	OrderID       int64
	FailureReason string
}

type ListOrdersRequest struct {
	UserID        int64
	Status        string
	OrderNo       string
	VisibleMethod string
	ProviderType  string
	PurchaseType  string
	Page          int
	PageSize      int
}

type MarkOrderPaidRequest struct {
	Provider           string
	ProviderInstanceID int64
	TradeNo            string
	OrderNo            string
	AmountCNY          string
}

type CompleteRechargeOrderRequest struct {
	UserID   int64
	OrderID  int64
	Provider string
	TradeNo  string
}

type RefundPaymentOrderRequest struct {
	UserID          int64
	OrderID         int64
	RefundTradeNo   string
	RefundAmountCNY string
	Reason          string
	OperatorAdminID int64
}
