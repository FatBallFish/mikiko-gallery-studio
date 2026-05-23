package modeladmin

import "time"

const (
	AdapterTypeOpenAICompatible = "openai_compatible"
	AdapterTypeOpenRouter       = "openrouter"
	AuthTypeAPIKey              = "api_key"

	ModelAccountStatusEnabled  = "enabled"
	ModelAccountStatusDisabled = "disabled"
	ModelAccountStatusError    = "error"

	RouteModelVisibilityPublic = "public"
	RouteModelVisibilityGroups = "groups"
	RouteModelVisibilityHidden = "hidden"
)

type CredentialsStatus struct {
	HasAPIKey bool `json:"has_api_key"`
}

type ModelAccount struct {
	ID                     int64             `json:"id"`
	Name                   string            `json:"name"`
	AdapterType            string            `json:"adapter_type"`
	AuthType               string            `json:"auth_type"`
	BaseURL                string            `json:"base_url"`
	CredentialsStatus      CredentialsStatus `json:"credentials_status"`
	CredentialsEncrypted   map[string]string `json:"-"`
	CredentialsFingerprint string            `json:"credentials_fingerprint,omitempty"`
	Status                 string            `json:"status"`
	Priority               int               `json:"priority"`
	Weight                 int               `json:"weight"`
	ConcurrencyLimit       int               `json:"concurrency_limit"`
	TimeoutMS              int               `json:"timeout_ms"`
	ErrorMessage           string            `json:"error_message,omitempty"`
	LastUsedAt             *time.Time        `json:"last_used_at,omitempty"`
	Extra                  map[string]any    `json:"extra,omitempty"`
	CreatedAt              time.Time         `json:"created_at"`
	UpdatedAt              time.Time         `json:"updated_at"`
}

type ModelAccountWriteRequest struct {
	Name             string
	AdapterType      string
	AuthType         string
	BaseURL          string
	Credentials      map[string]string
	Status           string
	Priority         int
	Weight           int
	ConcurrencyLimit int
	TimeoutMS        int
	Extra            map[string]any
}

type ModelAccountListRequest struct {
	Page        int
	PageSize    int
	AdapterType string
	AuthType    string
	Status      string
	Keyword     string
}

type ModelAccountListPage struct {
	Items    []ModelAccount `json:"items"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
	Total    int            `json:"total"`
}

type ModelAccountModel struct {
	ID           int64          `json:"id"`
	AccountID    int64          `json:"account_id"`
	AccountName  string         `json:"account_name,omitempty"`
	ModelCode    string         `json:"model_code"`
	DisplayName  string         `json:"display_name"`
	TaskTypes    []string       `json:"task_types"`
	Qualities    []string       `json:"qualities"`
	CostPerImage string         `json:"cost_per_image"`
	Currency     string         `json:"currency"`
	Enabled      bool           `json:"enabled"`
	Extra        map[string]any `json:"extra,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

type ModelAccountModelWriteRequest struct {
	AccountID    int64
	ModelCode    string
	DisplayName  string
	TaskTypes    []string
	Qualities    []string
	CostPerImage string
	Currency     string
	Enabled      bool
	Extra        map[string]any
}

type ModelAccountModelListRequest struct {
	Page      int
	PageSize  int
	AccountID int64
	ModelCode string
	Enabled   *bool
}

type ModelAccountModelListPage struct {
	Items    []ModelAccountModel `json:"items"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"page_size"`
	Total    int                 `json:"total"`
}

type RouteModel struct {
	ID          int64     `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Visibility  string    `json:"visibility"`
	Enabled     bool      `json:"enabled"`
	SortOrder   int       `json:"sort_order"`
	GroupIDs    []int64   `json:"group_ids,omitempty"`
	GroupCodes  []string  `json:"group_codes,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type RouteModelWriteRequest struct {
	Code        string
	Name        string
	Description string
	Visibility  string
	Enabled     bool
	SortOrder   int
	GroupIDs    []int64
}

type RouteModelListRequest struct {
	Page       int
	PageSize   int
	Visibility string
	Enabled    *bool
	Keyword    string
}

type RouteModelListPage struct {
	Items    []RouteModel `json:"items"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
	Total    int          `json:"total"`
}

type RouteModelCandidate struct {
	ID             int64  `json:"id"`
	RouteModelID   int64  `json:"route_model_id"`
	AccountModelID int64  `json:"account_model_id"`
	AccountID      int64  `json:"account_id,omitempty"`
	AccountName    string `json:"account_name,omitempty"`
	ModelCode      string `json:"model_code,omitempty"`
	Priority       int    `json:"priority"`
	Weight         int    `json:"weight"`
	FallbackOrder  int    `json:"fallback_order"`
	Enabled        bool   `json:"enabled"`
}

type RouteModelCandidateWriteRequest struct {
	RouteModelID   int64
	AccountModelID int64
	Priority       int
	Weight         int
	FallbackOrder  int
	Enabled        bool
}

type RouteModelPrice struct {
	ID                  int64  `json:"id"`
	RouteModelID        int64  `json:"route_model_id"`
	RouteModelCode      string `json:"route_model_code,omitempty"`
	TaskType            string `json:"task_type"`
	Quality             string `json:"quality"`
	BasePoints          string `json:"base_points"`
	ReferenceMultiplier string `json:"reference_multiplier"`
	Enabled             bool   `json:"enabled"`
}

type RouteModelPriceWriteRequest struct {
	RouteModelID        int64
	TaskType            string
	Quality             string
	BasePoints          string
	ReferenceMultiplier string
	Enabled             bool
}

type RouteModelPriceListRequest struct {
	Page         int
	PageSize     int
	RouteModelID int64
	TaskType     string
	Quality      string
	Enabled      *bool
}

type RouteModelPriceListPage struct {
	Items    []RouteModelPrice `json:"items"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
	Total    int               `json:"total"`
}

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
