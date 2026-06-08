package assets

import "time"

type ReferenceAsset struct {
	ID            string    `json:"id"`
	APIKeyID      *int64    `json:"-"`
	UploadSource  string    `json:"-"`
	Status        string    `json:"status"`
	MimeType      string    `json:"mime_type"`
	FileSizeBytes int64     `json:"file_size_bytes"`
	Width         int       `json:"width"`
	Height        int       `json:"height"`
	SHA256        string    `json:"sha256"`
	StorageDriver string    `json:"storage_driver"`
	ObjectKey     string    `json:"object_key"`
	PreviewURL    string    `json:"preview_url,omitempty"`
	DownloadURL   string    `json:"download_url,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type UploadMetadata struct {
	APIKeyID     *int64
	UploadSource string
}
