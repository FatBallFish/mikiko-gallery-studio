package admincallrecord

import (
	"time"

	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
)

type ArtifactRecoverySummary struct {
	Status         string                               `json:"status"`
	AttemptCount   int                                  `json:"attempt_count"`
	LastDiagnostic domainimagetask.ArtifactDiagnostic   `json:"last_diagnostic,omitempty"`
	Diagnostics    []domainimagetask.ArtifactDiagnostic `json:"diagnostics,omitempty"`
}

type Attempt struct {
	Provider       string         `json:"provider,omitempty"`
	AdapterType    string         `json:"adapter_type,omitempty"`
	AccountModelID *int64         `json:"account_model_id,omitempty"`
	ModelAccountID *int64         `json:"model_account_id,omitempty"`
	ModelCode      string         `json:"model_code,omitempty"`
	Status         string         `json:"status,omitempty"`
	Error          string         `json:"error,omitempty"`
	ErrorCode      string         `json:"error_code,omitempty"`
	ErrorMessage   string         `json:"error_message,omitempty"`
	ErrorDetail    map[string]any `json:"error_detail,omitempty"`
	StartedAt      *time.Time     `json:"started_at,omitempty"`
	FinishedAt     *time.Time     `json:"finished_at,omitempty"`
}

type Record struct {
	TaskID                    string                   `json:"task_id"`
	UserID                    int64                    `json:"user_id"`
	APIKeyID                  *int64                   `json:"api_key_id"`
	SourceChannel             string                   `json:"source_channel"`
	TaskType                  string                   `json:"task_type"`
	Status                    string                   `json:"status"`
	Provider                  string                   `json:"provider"`
	AccountModelID            *int64                   `json:"account_model_id,omitempty"`
	ModelAccountID            *int64                   `json:"model_account_id,omitempty"`
	UpstreamModelCode         string                   `json:"upstream_model_code,omitempty"`
	AbstractModel             string                   `json:"abstract_model"`
	BaseResolution            string                   `json:"base_resolution"`
	Quality                   string                   `json:"quality"`
	RequestedOutputImageCount int                      `json:"requested_output_image_count"`
	SuccessOutputImageCount   int                      `json:"success_output_image_count"`
	ReferenceImageCount       int                      `json:"reference_image_count"`
	EstimatedPoints           string                   `json:"estimated_points"`
	ActualPoints              string                   `json:"actual_points"`
	ProviderRequestID         string                   `json:"provider_request_id,omitempty"`
	ProviderCost              string                   `json:"provider_cost"`
	GrossMargin               string                   `json:"gross_margin"`
	UpstreamSucceededAt       *time.Time               `json:"upstream_succeeded_at,omitempty"`
	FailurePhase              string                   `json:"failure_phase,omitempty"`
	PlatformLoss              bool                     `json:"platform_loss"`
	ArtifactRecovery          *ArtifactRecoverySummary `json:"artifact_recovery,omitempty"`
	ErrorCode                 *string                  `json:"error_code"`
	ErrorMessage              *string                  `json:"error_message"`
	CreatedAt                 time.Time                `json:"created_at"`
	UpdatedAt                 time.Time                `json:"updated_at"`
	StartedAt                 *time.Time               `json:"started_at"`
	FinishedAt                *time.Time               `json:"finished_at"`
	AttemptCount              int                      `json:"attempt_count"`
	ErrorDetail               map[string]any           `json:"error_detail,omitempty"`
	Attempts                  []Attempt                `json:"attempts,omitempty"`
}

type ListRequest struct {
	Page             int
	PageSize         int
	Status           string
	ErrorCode        string
	Provider         string
	SourceChannel    string
	UserID           int64
	TaskID           string
	CreatedFrom      time.Time
	CreatedTo        time.Time
	PlatformLossOnly bool
}

type ListPage struct {
	Items    []Record `json:"items"`
	Page     int      `json:"page"`
	PageSize int      `json:"page_size"`
	Total    int      `json:"total"`
}
