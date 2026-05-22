package modeladmin

import "time"

type Provider struct {
	ID                  int64     `json:"id"`
	ProviderCode        string    `json:"provider_code"`
	ProviderType        string    `json:"provider_type"`
	AuthConfigEncrypted string    `json:"auth_config_encrypted,omitempty"`
	HealthStatus        string    `json:"health_status"`
	Enabled             bool      `json:"enabled"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type ProviderWriteRequest struct {
	ProviderCode        string
	ProviderType        string
	AuthConfigEncrypted string
	HealthStatus        string
	Enabled             bool
}

type ProviderListRequest struct {
	Page         int
	PageSize     int
	ProviderType string
	Enabled      *bool
}

type ProviderListPage struct {
	Items    []Provider `json:"items"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
	Total    int        `json:"total"`
}

type Route struct {
	ID              int64     `json:"id"`
	GroupCode       string    `json:"group_code"`
	TaskType        string    `json:"task_type"`
	ProviderModelID int64     `json:"provider_model_id"`
	ProviderCode    string    `json:"provider_code"`
	Priority        int       `json:"priority"`
	WeightPercent   int       `json:"weight_percent"`
	FallbackOrder   int       `json:"fallback_order"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type RouteWriteRequest struct {
	GroupCode       string
	TaskType        string
	ProviderModelID int64
	ProviderCode    string
	Priority        int
	WeightPercent   int
	FallbackOrder   int
	Enabled         bool
}

type RouteListRequest struct {
	Page         int
	PageSize     int
	GroupCode    string
	TaskType     string
	ProviderCode string
	Enabled      *bool
}

type RouteListPage struct {
	Items    []Route `json:"items"`
	Page     int     `json:"page"`
	PageSize int     `json:"page_size"`
	Total    int     `json:"total"`
}
