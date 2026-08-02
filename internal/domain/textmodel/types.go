package textmodel

import "time"

const (
	PlatformOpenAICompatible = "openai_compatible"
	APIStyleChatCompletions  = "chat_completions"
	APIStyleResponses        = "responses"
)

type SecretStatus struct {
	HasSecret   bool       `json:"has_secret"`
	Fingerprint string     `json:"fingerprint,omitempty"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}

type AccountRecord struct {
	ID                int64          `json:"id"`
	Name              string         `json:"name"`
	PlatformType      string         `json:"platform_type"`
	APIStyle          string         `json:"api_style"`
	BaseURL           string         `json:"base_url"`
	SecretEncrypted   map[string]any `json:"-"`
	SecretFingerprint string         `json:"-"`
	Enabled           bool           `json:"enabled"`
	Version           int64          `json:"version"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         *time.Time     `json:"-"`
}

type Account struct {
	ID           int64        `json:"id"`
	Name         string       `json:"name"`
	PlatformType string       `json:"platform_type"`
	APIStyle     string       `json:"api_style"`
	BaseURL      string       `json:"base_url"`
	Enabled      bool         `json:"enabled"`
	SecretStatus SecretStatus `json:"secret_status"`
	Version      int64        `json:"version"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

type AccountWriteRequest struct {
	Version      int64             `json:"version"`
	Name         string            `json:"name"`
	PlatformType string            `json:"platform_type"`
	APIStyle     string            `json:"api_style"`
	BaseURL      string            `json:"base_url"`
	Enabled      bool              `json:"enabled"`
	Secrets      map[string]string `json:"secrets,omitempty"`
	ClearSecrets []string          `json:"clear_secrets,omitempty"`
}

type Model struct {
	ID                 int64      `json:"id"`
	AccountID          int64      `json:"account_id"`
	ModelCode          string     `json:"model_code"`
	DisplayName        string     `json:"display_name"`
	InputPricePerMTok  string     `json:"input_price_per_million_tokens"`
	OutputPricePerMTok string     `json:"output_price_per_million_tokens"`
	Currency           string     `json:"currency"`
	Enabled            bool       `json:"enabled"`
	IsDefault          bool       `json:"is_default"`
	Version            int64      `json:"version"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	DeletedAt          *time.Time `json:"-"`
}

type ModelWriteRequest struct {
	Version            int64  `json:"version"`
	AccountID          int64  `json:"account_id"`
	ModelCode          string `json:"model_code"`
	DisplayName        string `json:"display_name"`
	InputPricePerMTok  string `json:"input_price_per_million_tokens"`
	OutputPricePerMTok string `json:"output_price_per_million_tokens"`
	Currency           string `json:"currency"`
	Enabled            bool   `json:"enabled"`
}

type DefaultSelection struct {
	Account AccountRecord
	Model   Model
}

type OptimizationRun struct {
	ID                string         `json:"id"`
	UserID            int64          `json:"user_id"`
	AccountID         int64          `json:"account_id"`
	ModelID           int64          `json:"model_id"`
	ModelCode         string         `json:"model_code"`
	APIStyle          string         `json:"api_style"`
	ConfigVersion     int64          `json:"config_version"`
	PromptSHA256      string         `json:"prompt_sha256"`
	Status            string         `json:"status"`
	InputTokens       int            `json:"input_tokens"`
	OutputTokens      int            `json:"output_tokens"`
	EstimatedPoints   string         `json:"estimated_points"`
	ActualPoints      string         `json:"actual_points"`
	ProviderRequestID string         `json:"provider_request_id,omitempty"`
	ErrorCode         string         `json:"error_code,omitempty"`
	ErrorMessage      string         `json:"error_message,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}
