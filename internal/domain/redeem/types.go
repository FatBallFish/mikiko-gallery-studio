package redeem

import (
	"time"

	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
)

type Code struct {
	ID             int64     `json:"id"`
	BatchID        int64     `json:"batch_id"`
	Code           string    `json:"code"`
	Status         string    `json:"status"`
	RewardType     string    `json:"reward_type"`
	RewardValue    string    `json:"reward_value"`
	ValidFrom      time.Time `json:"valid_from"`
	ValidUntil     time.Time `json:"valid_until"`
	MaxRedemptions int       `json:"max_redemptions"`
	RedeemedCount  int       `json:"redeemed_count"`
	LastRedeemedBy *int64    `json:"last_redeemed_by,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ListRequest struct {
	Page     int
	PageSize int
	Status   string
	Code     string
	BatchID  int64
}

type ListPage struct {
	Items    []Code `json:"items"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Total    int    `json:"total"`
}

type CreateRequest struct {
	Code           string    `json:"code"`
	BatchID        int64     `json:"batch_id"`
	Status         string    `json:"status"`
	RewardType     string    `json:"reward_type"`
	RewardValue    string    `json:"reward_value"`
	ValidFrom      time.Time `json:"valid_from"`
	ValidUntil     time.Time `json:"valid_until"`
	MaxRedemptions int       `json:"max_redemptions"`
	OperatorAdmin  int64     `json:"-"`
}

type BatchCreateRequest struct {
	Count          int       `json:"count"`
	BatchID        int64     `json:"batch_id"`
	Status         string    `json:"status"`
	RewardType     string    `json:"reward_type"`
	RewardValue    string    `json:"reward_value"`
	ValidFrom      time.Time `json:"valid_from"`
	ValidUntil     time.Time `json:"valid_until"`
	MaxRedemptions int       `json:"max_redemptions"`
	OperatorAdmin  int64     `json:"-"`
}

type BatchCreateResult struct {
	Items   []Code `json:"items"`
	Count   int    `json:"count"`
	BatchID int64  `json:"batch_id"`
}

type StatusRequest struct {
	ID            int64
	Status        string
	OperatorAdmin int64
}

type RedemptionsPage struct {
	Items    []domainbilling.LedgerEntry `json:"items"`
	Page     int                         `json:"page"`
	PageSize int                         `json:"page_size"`
	Total    int                         `json:"total"`
}
