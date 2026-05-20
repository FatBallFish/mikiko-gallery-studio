package billing

import "time"

type EstimateRequest struct {
	TaskType                  string
	AbstractModel             string
	RequestedQuality          string
	RequestedSize             string
	RequestedOutputImageCount int
	ReferenceImageCount       int
	UserGroupCode             string
	UserGroupMultiplier       string
}

type EstimateResult struct {
	ResolvedQualityBucket     string          `json:"resolved_quality_bucket"`
	EstimatedPoints           string          `json:"estimated_points"`
	UserGroupMultiplier       string          `json:"user_group_multiplier"`
	RequestedOutputImageCount int             `json:"requested_output_image_count"`
	ReferenceImageCount       int             `json:"reference_image_count"`
	PricingSnapshot           PricingSnapshot `json:"-"`
}

type PricingSnapshot struct {
	AbstractModel             string `json:"abstract_model"`
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
	AvailablePoints     string `json:"available_points"`
	FrozenPoints        string `json:"frozen_points"`
	UserGroupMultiplier string `json:"user_group_multiplier"`
	CNYPerPoint         string `json:"cny_per_point"`
}

type LedgerEntry struct {
	ID           int64     `json:"id"`
	TaskID       string    `json:"task_id,omitempty"`
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

type ReserveRequest struct {
	UserID          int64
	TaskID          string
	EstimatedPoints string
	Reason          string
}

type FinalizeRequest struct {
	UserID          int64
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
}
