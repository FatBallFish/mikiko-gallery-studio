package imagetask

import (
	"time"

	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/internal/provider"
)

const (
	StatusQueued        = "queued"
	StatusRunning       = "running"
	StatusSucceeded     = "succeeded"
	StatusPartialFailed = "partial_failed"
	StatusFailed        = "failed"
	StatusRejected      = "rejected"
	StatusDeleted       = "deleted"
)

const (
	ProgressStageQueued     = "queued"
	ProgressStageProvider   = "provider"
	ProgressStagePersisting = "persisting"
	ProgressStageSettling   = "settling"
	ProgressStageCompleted  = "completed"
	ProgressStageFailed     = "failed"
)

const (
	VisibilityPrivate       = "private"
	VisibilityPendingReview = "pending_review"
	VisibilityApproved      = "approved"
	VisibilityRejected      = "rejected"
	VisibilityUnpublished   = "unpublished"
)

type ExecuteRequest struct {
	TaskID              string
	UserID              int64
	APIKeyID            int64
	SourceChannel       string
	UserGroupCode       string
	UserGroupCodes      []string
	UserGroupMultiplier string
	AbstractModel       string
	RouteModelCode      string
	TaskType            string
	Prompt              string
	SizeMode            string
	RequestedSize       string
	BaseResolution      string
	Quality             string
	OutputFormat        string
	Background          string
	OutputCompression   int
	Moderation          string
	AspectRatio         string
	OutputImageCount    int
	ResponseFormat      string
	ReferenceImages     []provider.ImageInput
	Mask                *provider.ImageInput
	User                string
	PreferredProviders  []string
}

type CreateRequest struct {
	TaskID              string
	UserID              int64
	APIKeyID            int64
	SourceChannel       string
	AbstractModel       string
	RouteModelCode      string
	TaskType            string
	Prompt              string
	NegativePrompt      string
	SizeMode            string
	RequestedSize       string
	BaseResolution      string
	Quality             string
	OutputFormat        string
	Background          string
	OutputCompression   int
	Moderation          string
	AspectRatio         string
	OutputImageCount    int
	ReferenceImageCount int
	ReferenceAssetIDs   []string
	ReferenceStrength   int
	Seed                *int64
	UserGroupCode       string
	UserGroupCodes      []string
	UserGroupMultiplier string
	MaskPresent         bool
	ResponseMode        string
	SavePolicy          string
}

type RetryRequest struct {
	UserGroupCode       string
	UserGroupCodes      []string
	UserGroupMultiplier string
}

type TestModelAccountRequest struct {
	AccountID         int64
	ModelID           int64
	ModelCode         string
	Prompt            string
	SourceMode        string
	SizeMode          string
	RequestedSize     string
	BaseResolution    string
	Quality           string
	OutputFormat      string
	Background        string
	OutputCompression int
	Moderation        string
	AspectRatio       string
}

type TestModelAccountResult struct {
	Status            string               `json:"status"`
	ImageURL          string               `json:"image_url,omitempty"`
	Width             int                  `json:"width,omitempty"`
	Height            int                  `json:"height,omitempty"`
	ProviderRequestID string               `json:"provider_request_id,omitempty"`
	ActualParams      map[string]string    `json:"actual_params,omitempty"`
	ElapsedMS         int64                `json:"elapsed_ms"`
	Task              Task                 `json:"task,omitempty"`
	Image             provider.ImageResult `json:"image,omitempty"`
}

type Attempt struct {
	Provider            string         `json:"provider,omitempty"`
	AdapterType         string         `json:"adapter_type,omitempty"`
	AccountModelID      int64          `json:"account_model_id,omitempty"`
	ModelAccountID      int64          `json:"model_account_id,omitempty"`
	ModelCode           string         `json:"model_code,omitempty"`
	SourceSizeMode      string         `json:"source_size_mode,omitempty"`
	OutboundSize        string         `json:"outbound_size,omitempty"`
	ReturnedWidth       int            `json:"returned_width,omitempty"`
	ReturnedHeight      int            `json:"returned_height,omitempty"`
	SizeDiagnostic      string         `json:"size_diagnostic,omitempty"`
	ProviderRequestID   string         `json:"provider_request_id,omitempty"`
	RequestedImageCount int            `json:"requested_image_count,omitempty"`
	ReturnedImageCount  int            `json:"returned_image_count,omitempty"`
	Status              string         `json:"status,omitempty"`
	Error               string         `json:"error,omitempty"`
	ErrorCode           string         `json:"error_code,omitempty"`
	ErrorMessage        string         `json:"error_message,omitempty"`
	ErrorDetail         map[string]any `json:"error_detail,omitempty"`
	StartedAt           *time.Time     `json:"started_at,omitempty"`
	FinishedAt          *time.Time     `json:"finished_at,omitempty"`
}

type ArtifactDiagnostic struct {
	Code            string    `json:"code,omitempty"`
	Stage           string    `json:"stage,omitempty"`
	Attempt         int       `json:"attempt,omitempty"`
	URLHost         string    `json:"url_host,omitempty"`
	URLPath         string    `json:"url_path,omitempty"`
	HTTPStatus      int       `json:"http_status,omitempty"`
	ContentType     string    `json:"content_type,omitempty"`
	ContentLength   int64     `json:"content_length,omitempty"`
	BytesRead       int64     `json:"bytes_read,omitempty"`
	DurationMS      int64     `json:"duration_ms,omitempty"`
	StorageConfigID string    `json:"storage_config_id,omitempty"`
	StorageVersion  int64     `json:"storage_version,omitempty"`
	Retryable       bool      `json:"retryable"`
	Cause           string    `json:"cause,omitempty"`
	StartedAt       time.Time `json:"started_at,omitempty"`
	FinishedAt      time.Time `json:"finished_at,omitempty"`
}

type ArtifactRecovery struct {
	Status           string               `json:"status,omitempty"`
	EncryptedPayload string               `json:"-"`
	AttemptCount     int                  `json:"attempt_count,omitempty"`
	NextRetryAt      *time.Time           `json:"next_retry_at,omitempty"`
	LastDiagnostic   ArtifactDiagnostic   `json:"last_diagnostic,omitempty"`
	Diagnostics      []ArtifactDiagnostic `json:"diagnostics,omitempty"`
	StorageConfigID  string               `json:"storage_config_id,omitempty"`
	StorageVersion   int64                `json:"storage_version,omitempty"`
}

type Task struct {
	UserID               int64                         `json:"-"`
	APIKeyID             int64                         `json:"-"`
	SourceChannel        string                        `json:"-"`
	ID                   string                        `json:"id"`
	Status               string                        `json:"status"`
	ProgressStage        string                        `json:"progress_stage,omitempty"`
	ProgressMessage      string                        `json:"progress_message,omitempty"`
	Provider             string                        `json:"provider,omitempty"`
	ProviderModelID      int64                         `json:"provider_model_id,omitempty"`
	ProviderCost         string                        `json:"provider_cost,omitempty"`
	GrossMargin          string                        `json:"gross_margin,omitempty"`
	FallbackCount        int                           `json:"fallback_count,omitempty"`
	RouteSnapshotVersion string                        `json:"route_snapshot_version,omitempty"`
	AbstractModel        string                        `json:"abstract_model"`
	RouteModelCode       string                        `json:"route_model_code,omitempty"`
	RouteModelID         int64                         `json:"route_model_id,omitempty"`
	AccountModelID       int64                         `json:"account_model_id,omitempty"`
	ModelAccountID       int64                         `json:"model_account_id,omitempty"`
	UpstreamModelCode    string                        `json:"upstream_model_code,omitempty"`
	EffectiveMultiplier  string                        `json:"effective_multiplier,omitempty"`
	ChargedPoints        string                        `json:"charged_points,omitempty"`
	TaskType             string                        `json:"task_type"`
	Prompt               string                        `json:"prompt,omitempty"`
	NegativePrompt       string                        `json:"negative_prompt,omitempty"`
	SizeMode             string                        `json:"size_mode,omitempty"`
	AspectRatio          string                        `json:"aspect_ratio,omitempty"`
	RequestedSize        string                        `json:"requested_size,omitempty"`
	ResolvedWidth        int                           `json:"resolved_width,omitempty"`
	ResolvedHeight       int                           `json:"resolved_height,omitempty"`
	BaseResolution       string                        `json:"base_resolution"`
	Quality              string                        `json:"quality"`
	OutputFormat         string                        `json:"output_format,omitempty"`
	Background           string                        `json:"background,omitempty"`
	OutputCompression    int                           `json:"output_compression,omitempty"`
	Moderation           string                        `json:"moderation,omitempty"`
	ResponseMode         string                        `json:"response_mode,omitempty"`
	SavePolicy           string                        `json:"save_policy,omitempty"`
	OutputImageCount     int                           `json:"requested_output_image_count"`
	ReferenceImageCount  int                           `json:"reference_image_count"`
	ReferenceAssetIDs    []string                      `json:"reference_asset_ids,omitempty"`
	ReferenceStrength    int                           `json:"reference_strength,omitempty"`
	Seed                 *int64                        `json:"seed,omitempty"`
	EstimatedPoints      string                        `json:"estimated_points,omitempty"`
	ActualPoints         string                        `json:"actual_points,omitempty"`
	LeaseOwner           string                        `json:"lease_owner,omitempty"`
	LeaseExpiresAt       *time.Time                    `json:"lease_expires_at,omitempty"`
	ErrorCode            string                        `json:"error_code,omitempty"`
	Attempts             []Attempt                     `json:"attempts,omitempty"`
	ErrorMessage         string                        `json:"error_message,omitempty"`
	ProviderRequestID    string                        `json:"provider_request_id,omitempty"`
	UpstreamSucceededAt  *time.Time                    `json:"upstream_succeeded_at,omitempty"`
	ArtifactRecovery     ArtifactRecovery              `json:"artifact_recovery,omitempty"`
	Results              []provider.ImageResult        `json:"results,omitempty"`
	PricingSnapshot      domainbilling.PricingSnapshot `json:"-"`
	CreatedAt            time.Time                     `json:"created_at"`
	UpdatedAt            time.Time                     `json:"updated_at"`
}

type ExecuteResult struct {
	Task     Task
	Response provider.ImageResponse
}

type GalleryImage struct {
	ID                string                  `json:"id"`
	TaskID            string                  `json:"task_id"`
	UserID            int64                   `json:"user_id,omitempty"`
	Prompt            string                  `json:"prompt,omitempty"`
	PromptExcerpt     string                  `json:"prompt_excerpt,omitempty"`
	AbstractModel     string                  `json:"abstract_model,omitempty"`
	RouteModelCode    string                  `json:"route_model_code,omitempty"`
	TaskType          string                  `json:"task_type,omitempty"`
	TaskStatus        string                  `json:"task_status,omitempty"`
	SizeMode          string                  `json:"size_mode,omitempty"`
	RequestedSize     string                  `json:"requested_size,omitempty"`
	BaseResolution    string                  `json:"base_resolution,omitempty"`
	Quality           string                  `json:"quality,omitempty"`
	AspectRatio       string                  `json:"aspect_ratio,omitempty"`
	OutputFormat      string                  `json:"output_format,omitempty"`
	OutputCompression int                     `json:"output_compression,omitempty"`
	Moderation        string                  `json:"moderation,omitempty"`
	OutputImageCount  int                     `json:"requested_output_image_count,omitempty"`
	ActualPoints      string                  `json:"actual_points,omitempty"`
	ReferenceAssetIDs []string                `json:"reference_asset_ids,omitempty"`
	ReferenceAssets   []GalleryReferenceAsset `json:"reference_assets,omitempty"`
	URL               string                  `json:"url,omitempty"`
	DownloadURL       string                  `json:"download_url,omitempty"`
	PreviewExpiresAt  *time.Time              `json:"preview_expires_at,omitempty"`
	DownloadExpiresAt *time.Time              `json:"download_expires_at,omitempty"`
	MimeType          string                  `json:"mime_type,omitempty"`
	FileSizeBytes     int64                   `json:"file_size_bytes"`
	Width             int                     `json:"width"`
	Height            int                     `json:"height"`
	SHA256            string                  `json:"sha256,omitempty"`
	StorageConfigID   string                  `json:"storage_config_id,omitempty"`
	ObjectKey         string                  `json:"object_key,omitempty"`
	StorageDriver     string                  `json:"storage_driver,omitempty"`
	ImageGroup        string                  `json:"image_group,omitempty"`
	VisibilityStatus  string                  `json:"visibility_status"`
	ReviewReason      string                  `json:"review_reason,omitempty"`
	PublishedAt       *time.Time              `json:"published_at,omitempty"`
	AuthorName        string                  `json:"author_name,omitempty"`
	LikeCount         int                     `json:"like_count"`
	FavoriteCount     int                     `json:"favorite_count"`
	LikedByViewer     bool                    `json:"liked_by_viewer,omitempty"`
	FavoritedByViewer bool                    `json:"favorited_by_viewer,omitempty"`
	CreatedAt         time.Time               `json:"created_at"`
}

type GalleryReferenceAsset struct {
	ID               string     `json:"id"`
	Name             string     `json:"name,omitempty"`
	PreviewURL       string     `json:"preview_url,omitempty"`
	PreviewExpiresAt *time.Time `json:"preview_expires_at,omitempty"`
}

type GalleryListRequest struct {
	Page           int
	PageSize       int
	Status         string
	ReviewOnly     bool
	Sort           string
	Query          string
	UserQuery      string
	PromptQuery    string
	ModelQuery     string
	RouteModelCode string
	TaskType       string
	BaseResolution string
	RequestedSize  string
	Width          int
	Height         int
	AspectRatio    string
	CreatedFrom    time.Time
	CreatedTo      time.Time
	PublishedFrom  time.Time
	PublishedTo    time.Time
	ViewerUserID   int64
	LikedOnly      bool
	FavoritedOnly  bool
}

type GalleryPage struct {
	Items    []GalleryImage `json:"items"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
	Total    int            `json:"total"`
}
