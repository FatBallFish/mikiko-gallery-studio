package assets

import (
	"fmt"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/domain/prompttemplate"
)

type ReferenceAsset struct {
	ID                  string              `json:"id"`
	Name                string              `json:"name"`
	APIKeyID            *int64              `json:"-"`
	UploadSource        string              `json:"-"`
	Status              string              `json:"status"`
	MimeType            string              `json:"mime_type"`
	FileSizeBytes       int64               `json:"file_size_bytes"`
	Width               int                 `json:"width"`
	Height              int                 `json:"height"`
	SHA256              string              `json:"sha256"`
	StorageDriver       string              `json:"storage_driver"`
	StorageConfigID     string              `json:"storage_config_id,omitempty"`
	ObjectKey           string              `json:"object_key"`
	SourceImageResultID string              `json:"source_image_result_id,omitempty"`
	OwnsObject          bool                `json:"owns_object"`
	GenerationSnapshot  *GenerationSnapshot `json:"generation_snapshot,omitempty"`
	PreviewURL          string              `json:"preview_url,omitempty"`
	DownloadURL         string              `json:"download_url,omitempty"`
	PreviewExpiresAt    *time.Time          `json:"preview_expires_at,omitempty"`
	DownloadExpiresAt   *time.Time          `json:"download_expires_at,omitempty"`
	CreatedAt           time.Time           `json:"created_at"`
}

type GenerationSnapshot struct {
	TaskType          string `json:"task_type,omitempty"`
	AbstractModel     string `json:"abstract_model,omitempty"`
	RouteModelCode    string `json:"route_model_code,omitempty"`
	CapabilityVersion string `json:"capability_version,omitempty"`
	SizeMode          string `json:"size_mode,omitempty"`
	RequestedSize     string `json:"requested_size,omitempty"`
	BaseResolution    string `json:"base_resolution,omitempty"`
	AspectRatio       string `json:"aspect_ratio,omitempty"`
	Quality           string `json:"quality,omitempty"`
	Background        string `json:"background,omitempty"`
	OutputFormat      string `json:"output_format,omitempty"`
	OutputCompression int    `json:"output_compression,omitempty"`
	Moderation        string `json:"moderation,omitempty"`
	ImageCount        int    `json:"image_count,omitempty"`
}

type UploadMetadata struct {
	APIKeyID     *int64
	UploadSource string
}

func NormalizeReferenceName(raw string) (string, error) {
	return prompttemplate.NormalizeName(raw, 64)
}

func ReferenceNameCandidate(preferred string, sequence int) string {
	preferred = strings.TrimSpace(preferred)
	if preferred == "" {
		return fmt.Sprintf("图片%d", sequence)
	}
	if sequence <= 1 {
		return preferred
	}
	suffix := fmt.Sprintf(" %d", sequence)
	maxBaseRunes := 64 - len([]rune(suffix))
	baseRunes := []rune(preferred)
	if len(baseRunes) > maxBaseRunes {
		baseRunes = baseRunes[:maxBaseRunes]
	}
	return string(baseRunes) + suffix
}
