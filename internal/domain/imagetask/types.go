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
