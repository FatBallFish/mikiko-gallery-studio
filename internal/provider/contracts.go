package provider

import "time"

import "context"

type ProviderType string

const (
	ProviderTypeOpenAI     ProviderType = "openai"
	ProviderTypeOpenRouter ProviderType = "openrouter"
)

type TaskType string

const (
	TaskTypeTextToImage       TaskType = "text_to_image"
	TaskTypeImageEdit         TaskType = "image_edit"
	TaskTypeReferenceGenerate TaskType = "reference_generate"
)

type ResponseFormat string

const (
	ResponseFormatURL     ResponseFormat = "url"
	ResponseFormatB64JSON ResponseFormat = "b64_json"
)

type ImageInput struct {
	Filename string
	MIMEType string
	Data     []byte
}

type ImageRequest struct {
	Model            string
	TaskType         TaskType
	Prompt           string
	Size             string
	Quality          string
	OutputImageCount int
	ResponseFormat   ResponseFormat
	ReferenceImages  []ImageInput
	Mask             *ImageInput
	User             string
}

type ImageResult struct {
	ID               string     `json:"id,omitempty"`
	URL              string     `json:"url,omitempty"`
	DownloadURL      string     `json:"download_url,omitempty"`
	B64JSON          string     `json:"b64_json,omitempty"`
	RevisedPrompt    string     `json:"revised_prompt,omitempty"`
	Format           string     `json:"format,omitempty"`
	MimeType         string     `json:"mime_type,omitempty"`
	FileSizeBytes    int64      `json:"file_size_bytes"`
	Width            int        `json:"width"`
	Height           int        `json:"height"`
	SHA256           string     `json:"sha256,omitempty"`
	ObjectKey        string     `json:"object_key,omitempty"`
	StorageConfigID  *int64     `json:"storage_config_id,omitempty"`
	StorageDriver    string     `json:"storage_driver,omitempty"`
	ImageGroup       string     `json:"image_group,omitempty"`
	VisibilityStatus string     `json:"visibility_status,omitempty"`
	ReviewReason     string     `json:"review_reason,omitempty"`
	PublishedAt      *time.Time `json:"published_at,omitempty"`
}

type ImageResponse struct {
	Created           int64         `json:"created,omitempty"`
	Data              []ImageResult `json:"data"`
	ProviderRequestID string        `json:"-"`
}

type ImageProvider interface {
	Generate(ctx context.Context, req ImageRequest) (ImageResponse, error)
	Edit(ctx context.Context, req ImageRequest) (ImageResponse, error)
}
