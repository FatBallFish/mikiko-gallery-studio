package adminuser

import (
	"time"

	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
)

type UserSummary struct {
	ID            int64     `json:"id"`
	Email         string    `json:"email"`
	Nickname      string    `json:"nickname"`
	Status        string    `json:"status"`
	UserGroupCode string    `json:"user_group_code"`
	TokenVersion  int       `json:"token_version"`
	DefaultLocale string    `json:"default_locale"`
	Theme         string    `json:"theme"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ListRequest struct {
	Page     int
	PageSize int
	Query    string
	Status   string
}

type ListPage struct {
	Items    []UserSummary `json:"items"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
	Total    int           `json:"total"`
}

type Detail struct {
	User         UserSummary                  `json:"user"`
	Balance      domainbilling.BalanceSummary `json:"balance"`
	RecentLedger []domainbilling.LedgerEntry  `json:"recent_ledger"`
}

type StatusRequest struct {
	UserID        int64
	Status        string
	OperatorAdmin int64
}

type PointAdjustmentRequest struct {
	UserID         int64
	ChangePoints   string
	Reason         string
	IdempotencyKey string
	OperatorAdmin  int64
}
