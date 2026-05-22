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
	UserGroupMultiplier string
	AbstractModel       string
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
	UserGroupMultiplier string
	MaskPresent         bool
	ResponseMode        string
	SavePolicy          string
}

type Attempt struct {
	Provider string
	Status   string
	Error    string
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
}

type ExecuteResult struct {
	Task     Task
	Response provider.ImageResponse
}

type GalleryImage struct {
	ID               string     `json:"id"`
	TaskID           string     `json:"task_id"`
	UserID           int64      `json:"user_id,omitempty"`
	Prompt           string     `json:"prompt,omitempty"`
	AbstractModel    string     `json:"abstract_model,omitempty"`
	TaskType         string     `json:"task_type,omitempty"`
	URL              string     `json:"url,omitempty"`
	DownloadURL      string     `json:"download_url,omitempty"`
	MimeType         string     `json:"mime_type,omitempty"`
	FileSizeBytes    int64      `json:"file_size_bytes"`
	Width            int        `json:"width"`
	Height           int        `json:"height"`
	SHA256           string     `json:"sha256,omitempty"`
	ObjectKey        string     `json:"object_key,omitempty"`
	StorageDriver    string     `json:"storage_driver,omitempty"`
	VisibilityStatus string     `json:"visibility_status"`
	ReviewReason     string     `json:"review_reason,omitempty"`
	PublishedAt      *time.Time `json:"published_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type GalleryListRequest struct {
	Page     int
	PageSize int
	Status   string
}

type GalleryPage struct {
	Items    []GalleryImage `json:"items"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
	Total    int            `json:"total"`
}
