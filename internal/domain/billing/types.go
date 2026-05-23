package billing

import "time"

type EstimateRequest struct {
	TaskType                  string
	AbstractModel             string
	RouteModelCode            string
	RequestedQuality          string
	RequestedSize             string
	RequestedOutputImageCount int
	ReferenceImageCount       int
	UserGroupCode             string
	UserGroupCodes            []string
	UserGroupMultiplier       string
}

type EstimateResult struct {
	ResolvedQualityBucket     string          `json:"resolved_quality_bucket"`
	EstimatedPoints           string          `json:"estimated_points"`
	ChargedPoints             string          `json:"charged_points,omitempty"`
	DisplayPoints             string          `json:"display_points,omitempty"`
	UserGroupMultiplier       string          `json:"user_group_multiplier"`
	RequestedOutputImageCount int             `json:"requested_output_image_count"`
	ReferenceImageCount       int             `json:"reference_image_count"`
	PricingSnapshot           PricingSnapshot `json:"-"`
}

type PricingSnapshot struct {
	AbstractModel             string `json:"abstract_model"`
	RouteModelCode            string `json:"route_model_code,omitempty"`
	TaskType                  string `json:"task_type"`
	RequestedQuality          string `json:"requested_quality"`
	RequestedSize             string `json:"requested_size,omitempty"`
	ResolvedQualityBucket     string `json:"resolved_quality_bucket"`
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
	SubscriptionPoints  string                   `json:"subscription_points,omitempty"`
	GiftPoints          string                   `json:"gift_points,omitempty"`
	RechargePoints      string                   `json:"recharge_points,omitempty"`
	UserGroupMultiplier string                   `json:"user_group_multiplier"`
	CNYPerPoint         string                   `json:"cny_per_point"`
	ActiveSubscription  *UserSubscriptionSummary `json:"active_subscription,omitempty"`
	NextExpiringGrant   *GrantExpirySummary      `json:"next_expiring_grant,omitempty"`
}

type GrantExpirySummary struct {
	GrantID         int64      `json:"grant_id"`
	GrantType       string     `json:"grant_type"`
	AvailablePoints string     `json:"available_points"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
}

type SubscriptionPlan struct {
	ID           int64     `json:"id"`
	PlanCode     string    `json:"plan_code"`
	PlanName     string    `json:"plan_name"`
	Status       string    `json:"status"`
	PriceCNY     string    `json:"price_cny"`
	Points       string    `json:"points"`
	BonusPoints  string    `json:"bonus_points"`
	DurationDays int       `json:"duration_days"`
	Currency     string    `json:"currency"`
	Description  string    `json:"description,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
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
	ID            int64      `json:"id"`
	OrderNo       string     `json:"order_no"`
	UserID        int64      `json:"user_id,omitempty"`
	PlanID        int64      `json:"plan_id"`
	PlanCode      string     `json:"plan_code"`
	PlanName      string     `json:"plan_name"`
	Provider      string     `json:"provider"`
	Status        string     `json:"status"`
	Currency      string     `json:"currency"`
	AmountCNY     string     `json:"amount_cny"`
	Points        string     `json:"points"`
	BonusPoints   string     `json:"bonus_points"`
	TradeNo       string     `json:"trade_no,omitempty"`
	PaymentURL    string     `json:"payment_url,omitempty"`
	QRCode        string     `json:"qr_code,omitempty"`
	ClientToken   string     `json:"client_token,omitempty"`
	FailureReason string     `json:"failure_reason,omitempty"`
	ExpiresAt     time.Time  `json:"expires_at"`
	PaidAt        *time.Time `json:"paid_at,omitempty"`
	ClosedAt      *time.Time `json:"closed_at,omitempty"`
	RefundedAt    *time.Time `json:"refunded_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type PaymentOrderPage struct {
	Items    []PaymentOrder `json:"items"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
	Total    int            `json:"total"`
}

type LedgerEntry struct {
	ID           int64     `json:"id"`
	UserID       int64     `json:"user_id,omitempty"`
	APIKeyID     int64     `json:"api_key_id,omitempty"`
	TaskID       string    `json:"task_id,omitempty"`
	RedeemCodeID int64     `json:"redeem_code_id,omitempty"`
	LedgerType   string    `json:"ledger_type"`
	ChangePoints string    `json:"change_points"`
	BalanceAfter string    `json:"balance_after"`
	FrozenAfter  string    `json:"frozen_after,omitempty"`
	Reason       string    `json:"reason,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
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
	UserID   int64
	PlanCode string
	Provider string
}

type ListOrdersRequest struct {
	UserID   int64
	Page     int
	PageSize int
}

type MarkOrderPaidRequest struct {
	Provider string
	TradeNo  string
	OrderNo  string
}
