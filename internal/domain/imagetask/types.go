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
	RequestedSize       string
	RequestedQuality    string
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
	RequestedSize       string
	RequestedQuality    string
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
	AccountID  int64
	ModelID    int64
	ModelCode  string
	Prompt     string
	SourceMode string
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
	Provider       string         `json:"provider,omitempty"`
	AdapterType    string         `json:"adapter_type,omitempty"`
	AccountModelID int64          `json:"account_model_id,omitempty"`
	ModelAccountID int64          `json:"model_account_id,omitempty"`
	ModelCode      string         `json:"model_code,omitempty"`
	Status         string         `json:"status,omitempty"`
	Error          string         `json:"error,omitempty"`
	ErrorCode      string         `json:"error_code,omitempty"`
	ErrorMessage   string         `json:"error_message,omitempty"`
	ErrorDetail    map[string]any `json:"error_detail,omitempty"`
	StartedAt      *time.Time     `json:"started_at,omitempty"`
	FinishedAt     *time.Time     `json:"finished_at,omitempty"`
}

type Task struct {
	UserID                int64                         `json:"-"`
	APIKeyID              int64                         `json:"-"`
	SourceChannel         string                        `json:"-"`
	ID                    string                        `json:"id"`
	Status                string                        `json:"status"`
	Provider              string                        `json:"provider,omitempty"`
	ProviderModelID       int64                         `json:"provider_model_id,omitempty"`
	ProviderCost          string                        `json:"provider_cost,omitempty"`
	GrossMargin           string                        `json:"gross_margin,omitempty"`
	FallbackCount         int                           `json:"fallback_count,omitempty"`
	RouteSnapshotVersion  string                        `json:"route_snapshot_version,omitempty"`
	AbstractModel         string                        `json:"abstract_model"`
	RouteModelCode        string                        `json:"route_model_code,omitempty"`
	RouteModelID          int64                         `json:"route_model_id,omitempty"`
	AccountModelID        int64                         `json:"account_model_id,omitempty"`
	ModelAccountID        int64                         `json:"model_account_id,omitempty"`
	UpstreamModelCode     string                        `json:"upstream_model_code,omitempty"`
	EffectiveMultiplier   string                        `json:"effective_multiplier,omitempty"`
	ChargedPoints         string                        `json:"charged_points,omitempty"`
	TaskType              string                        `json:"task_type"`
	Prompt                string                        `json:"prompt,omitempty"`
	NegativePrompt        string                        `json:"negative_prompt,omitempty"`
	AspectRatio           string                        `json:"aspect_ratio,omitempty"`
	RequestedSize         string                        `json:"requested_size,omitempty"`
	RequestedQuality      string                        `json:"requested_quality"`
	ResolvedQualityBucket string                        `json:"resolved_quality_bucket"`
	ResponseMode          string                        `json:"response_mode,omitempty"`
	SavePolicy            string                        `json:"save_policy,omitempty"`
	OutputImageCount      int                           `json:"requested_output_image_count"`
	ReferenceImageCount   int                           `json:"reference_image_count"`
	ReferenceAssetIDs     []string                      `json:"reference_asset_ids,omitempty"`
	ReferenceStrength     int                           `json:"reference_strength,omitempty"`
	Seed                  *int64                        `json:"seed,omitempty"`
	EstimatedPoints       string                        `json:"estimated_points,omitempty"`
	ActualPoints          string                        `json:"actual_points,omitempty"`
	LeaseOwner            string                        `json:"lease_owner,omitempty"`
	LeaseExpiresAt        *time.Time                    `json:"lease_expires_at,omitempty"`
	ErrorCode             string                        `json:"error_code,omitempty"`
	Attempts              []Attempt                     `json:"attempts,omitempty"`
	ErrorMessage          string                        `json:"error_message,omitempty"`
	Results               []provider.ImageResult        `json:"results,omitempty"`
	PricingSnapshot       domainbilling.PricingSnapshot `json:"-"`
	CreatedAt             time.Time                     `json:"created_at"`
	UpdatedAt             time.Time                     `json:"updated_at"`
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
	Quality           string                  `json:"quality,omitempty"`
	AspectRatio       string                  `json:"aspect_ratio,omitempty"`
	ActualPoints      string                  `json:"actual_points,omitempty"`
	ReferenceAssetIDs []string                `json:"reference_asset_ids,omitempty"`
	ReferenceAssets   []GalleryReferenceAsset `json:"reference_assets,omitempty"`
	URL               string                  `json:"url,omitempty"`
	DownloadURL       string                  `json:"download_url,omitempty"`
	MimeType          string                  `json:"mime_type,omitempty"`
	FileSizeBytes     int64                   `json:"file_size_bytes"`
	Width             int                     `json:"width"`
	Height            int                     `json:"height"`
	SHA256            string                  `json:"sha256,omitempty"`
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
	ID         string `json:"id"`
	Name       string `json:"name,omitempty"`
	PreviewURL string `json:"preview_url,omitempty"`
}

type GalleryListRequest struct {
	Page           int
	PageSize       int
	Status         string
	ReviewOnly     bool
	Sort           string
	Query          string
	RouteModelCode string
	TaskType       string
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
