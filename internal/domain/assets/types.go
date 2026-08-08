package assets

import "time"

type ReferenceAsset struct {
	ID                  string     `json:"id"`
	APIKeyID            *int64     `json:"-"`
	UploadSource        string     `json:"-"`
	Status              string     `json:"status"`
	MimeType            string     `json:"mime_type"`
	FileSizeBytes       int64      `json:"file_size_bytes"`
	Width               int        `json:"width"`
	Height              int        `json:"height"`
	SHA256              string     `json:"sha256"`
	StorageDriver       string     `json:"storage_driver"`
	StorageConfigID     string     `json:"storage_config_id,omitempty"`
	ObjectKey           string     `json:"object_key"`
	SourceImageResultID string     `json:"source_image_result_id,omitempty"`
	OwnsObject          bool       `json:"owns_object"`
	PreviewURL          string     `json:"preview_url,omitempty"`
	DownloadURL         string     `json:"download_url,omitempty"`
	PreviewExpiresAt    *time.Time `json:"preview_expires_at,omitempty"`
	DownloadExpiresAt   *time.Time `json:"download_expires_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
}

type UploadMetadata struct {
	APIKeyID     *int64
	UploadSource string
}
