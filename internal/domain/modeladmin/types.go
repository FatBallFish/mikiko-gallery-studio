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

type ProviderModel struct {
	ID                     int64      `json:"id"`
	ProviderID             int64      `json:"provider_id"`
	ProviderCode           string     `json:"provider_code"`
	ModelCode              string     `json:"model_code"`
	CompatMode             string     `json:"compat_mode"`
	SupportsImageInput     bool       `json:"supports_image_input"`
	SupportsMask           bool       `json:"supports_mask"`
	SupportedQualities     []string   `json:"supported_qualities"`
	SupportedRatios        []string   `json:"supported_ratios"`
	MaxImageCount          int        `json:"max_image_count"`
	MaxReferenceImageCount int        `json:"max_reference_image_count"`
	TimeoutMS              int        `json:"timeout_ms"`
	InputCost              string     `json:"input_cost"`
	OutputCost             string     `json:"output_cost"`
	Currency               string     `json:"currency"`
	HealthStatus           string     `json:"health_status"`
	LastHealthCheckedAt    *time.Time `json:"last_health_checked_at,omitempty"`
	Enabled                bool       `json:"enabled"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

type ProviderModelWriteRequest struct {
	ProviderCode           string
	ModelCode              string
	CompatMode             string
	SupportsImageInput     bool
	SupportsMask           bool
	SupportedQualities     []string
	SupportedRatios        []string
	MaxImageCount          int
	MaxReferenceImageCount int
	TimeoutMS              int
	InputCost              string
	OutputCost             string
	Currency               string
	HealthStatus           string
	LastHealthCheckedAt    *time.Time
	Enabled                bool
}

type ProviderModelListRequest struct {
	Page         int
	PageSize     int
	ProviderCode string
	ModelCode    string
	Enabled      *bool
}

type ProviderModelListPage struct {
	Items    []ProviderModel `json:"items"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
	Total    int             `json:"total"`
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
