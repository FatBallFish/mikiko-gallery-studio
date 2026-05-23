package adminuser

import (
	"time"

	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
)

type UserSummary struct {
	ID               int64      `json:"id"`
	Email            string     `json:"email"`
	Nickname         string     `json:"nickname"`
	Status           string     `json:"status"`
	UserGroupCode    string     `json:"user_group_code"`
	TokenVersion     int        `json:"token_version"`
	RPMLimit         int        `json:"rpm_limit"`
	ConcurrencyLimit int        `json:"concurrency_limit"`
	DefaultLocale    string     `json:"default_locale"`
	Theme            string     `json:"theme"`
	ClosedAt         *time.Time `json:"closed_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
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

type CreateUserRequest struct {
	Email            string
	Nickname         string
	Status           string
	UserGroupCode    string
	RPMLimit         int
	ConcurrencyLimit int
	DefaultLocale    string
	Theme            string
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

type LimitsRequest struct {
	UserID           int64
	RPMLimit         int
	ConcurrencyLimit int
}

type GroupAssignmentRequest struct {
	UserID        int64
	UserGroupCode string
}

type MultiGroupAssignmentRequest struct {
	UserID   int64
	GroupIDs []int64
}

type UserGroup struct {
	ID          int64     `json:"id,omitempty"`
	GroupCode   string    `json:"group_code"`
	GroupName   string    `json:"group_name"`
	Multiplier  string    `json:"multiplier"`
	Status      string    `json:"status"`
	Description *string   `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type UserGroupListRequest struct {
	Page     int
	PageSize int
	Query    string
	Status   string
}

type UserGroupListPage struct {
	Items    []UserGroup `json:"items"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
	Total    int         `json:"total"`
}

type UserGroupWriteRequest struct {
	GroupCode   string
	GroupName   string
	Multiplier  string
	Status      string
	Description *string
}
