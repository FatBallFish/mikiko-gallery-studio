package mediaasset

import (
	"context"
	"time"

	"github.com/google/uuid"

	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
	"github.com/fatballfish/pic-gallery/internal/storage"
)

type UploadSession struct {
	ID                 uuid.UUID               `json:"id"`
	UserID             int64                   `json:"user_id"`
	ProjectID          uuid.UUID               `json:"project_id"`
	GroupName          string                  `json:"group_name"`
	OriginalFilename   string                  `json:"original_filename"`
	DeclaredMediaType  domainmedia.MediaType   `json:"declared_media_type"`
	DeclaredMIMEType   string                  `json:"declared_mime_type"`
	DeclaredSizeBytes  int64                   `json:"declared_size_bytes"`
	DeclaredChecksum   string                  `json:"declared_checksum,omitempty"`
	StorageConfigID    string                  `json:"-"`
	StorageDriver      string                  `json:"storage_driver"`
	Bucket             string                  `json:"-"`
	ObjectKey          string                  `json:"-"`
	BackendUploadID    string                  `json:"-"`
	PartSize           int64                   `json:"part_size"`
	PartCount          int                     `json:"part_count"`
	Status             string                  `json:"status"`
	ReservedBytes      int64                   `json:"reserved_bytes"`
	ActualBytes        int64                   `json:"actual_bytes"`
	IdempotencyKey     string                  `json:"-"`
	RequestFingerprint string                  `json:"-"`
	CompletedParts     []storage.CompletedPart `json:"completed_parts"`
	AssetID            *uuid.UUID              `json:"asset_id,omitempty"`
	ExpiresAt          time.Time               `json:"expires_at"`
	CompletedAt        *time.Time              `json:"completed_at,omitempty"`
}

func (session UploadSession) MultipartUpload() storage.MultipartUpload {
	return storage.MultipartUpload{
		UploadID: session.BackendUploadID, ObjectKey: session.ObjectKey, ContentType: session.DeclaredMIMEType,
		SizeBytes: session.DeclaredSizeBytes, PartSize: session.PartSize, PartCount: session.PartCount, Driver: session.StorageDriver,
	}
}

type Asset struct {
	ID               uuid.UUID             `json:"id"`
	UserID           int64                 `json:"user_id"`
	ProjectID        uuid.UUID             `json:"project_id"`
	LegacyImageID    *uuid.UUID            `json:"legacy_image_id,omitempty"`
	Name             string                `json:"name"`
	GroupName        string                `json:"group_name"`
	MediaType        domainmedia.MediaType `json:"media_type"`
	SourceType       string                `json:"source_type"`
	SourceTaskKind   *string               `json:"source_task_kind,omitempty"`
	SourceTaskID     *uuid.UUID            `json:"source_task_id,omitempty"`
	SourceCanvasID   *uuid.UUID            `json:"source_canvas_id,omitempty"`
	Status           string                `json:"status"`
	VisibilityStatus string                `json:"visibility_status"`
	StorageConfigID  string                `json:"-"`
	StorageDriver    string                `json:"storage_driver"`
	Bucket           string                `json:"-"`
	ObjectKey        string                `json:"-"`
	MIMEType         string                `json:"mime_type"`
	Container        string                `json:"container,omitempty"`
	Codec            string                `json:"codec,omitempty"`
	FileSizeBytes    int64                 `json:"file_size_bytes"`
	SHA256           string                `json:"sha256,omitempty"`
	Width            *int                  `json:"width,omitempty"`
	Height           *int                  `json:"height,omitempty"`
	DurationMS       *int64                `json:"duration_ms,omitempty"`
	FrameRateMilli   *int                  `json:"frame_rate_milli,omitempty"`
	AudioCodec       string                `json:"audio_codec,omitempty"`
	Channels         *int                  `json:"channels,omitempty"`
	SampleRate       *int                  `json:"sample_rate,omitempty"`
	Version          int64                 `json:"version"`
	CreatedAt        time.Time             `json:"created_at"`
	UpdatedAt        time.Time             `json:"updated_at"`
	DeletedAt        *time.Time            `json:"deleted_at,omitempty"`
}

type AssetListRequest struct {
	UserID     int64
	ProjectID  *uuid.UUID
	MediaType  domainmedia.MediaType
	SourceType string
	GroupName  string
	Status     string
	Keyword    string
	SortBy     string
	SortOrder  string
	Cursor     string
	Limit      int
}

type AssetPage struct {
	Items      []Asset `json:"items"`
	NextCursor string  `json:"next_cursor,omitempty"`
}

type UpdateAssetRequest struct {
	UserID          int64
	AssetID         uuid.UUID
	Name            *string
	GroupName       *string
	ProjectID       *uuid.UUID
	ExpectedVersion int64
}

type DeleteAssetRequest struct {
	UserID          int64
	AssetID         uuid.UUID
	ExpectedVersion int64
}

type AssetDerivative struct {
	Kind            domainmedia.DerivativeKind
	Status          string
	StorageConfigID string
	StorageDriver   string
	Bucket          string
	ObjectKey       string
	MIMEType        string
}

type CreateUploadRecord struct {
	Session    UploadSession
	QuotaBytes int64
}

type CompleteUploadRecord struct {
	UserID      int64
	SessionID   uuid.UUID
	AssetID     uuid.UUID
	Completed   storage.CompletedMultipartObject
	CompletedAt time.Time
}

type Store interface {
	FindUploadByIdempotency(context.Context, int64, string) (UploadSession, bool, error)
	CreateUpload(context.Context, CreateUploadRecord) (UploadSession, error)
	GetUpload(context.Context, int64, uuid.UUID) (UploadSession, error)
	RecordCompletedParts(context.Context, int64, uuid.UUID, []storage.CompletedPart) (UploadSession, error)
	MarkUploadCompleting(context.Context, int64, uuid.UUID, []storage.CompletedPart) (UploadSession, error)
	CompleteUpload(context.Context, CompleteUploadRecord) (Asset, error)
	AbortUpload(context.Context, int64, uuid.UUID) (UploadSession, error)
	ListAssets(context.Context, AssetListRequest) (AssetPage, error)
	GetAsset(context.Context, int64, uuid.UUID) (Asset, error)
	UpdateAsset(context.Context, UpdateAssetRequest) (Asset, error)
	DeleteAsset(context.Context, DeleteAssetRequest) (Asset, error)
	ListReadyDerivatives(context.Context, int64, uuid.UUID) ([]AssetDerivative, error)
	RetryAssetProcessing(context.Context, int64, uuid.UUID) (Asset, error)
}
