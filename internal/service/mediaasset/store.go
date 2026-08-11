package mediaasset

import (
	"context"
	"time"

	"github.com/google/uuid"

	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
	"github.com/fatballfish/pic-gallery/internal/storage"
)

type UploadSession struct {
	ID                 uuid.UUID
	UserID             int64
	ProjectID          uuid.UUID
	GroupName          string
	OriginalFilename   string
	DeclaredMediaType  domainmedia.MediaType
	DeclaredMIMEType   string
	DeclaredSizeBytes  int64
	DeclaredChecksum   string
	StorageConfigID    string
	StorageDriver      string
	Bucket             string
	ObjectKey          string
	BackendUploadID    string
	PartSize           int64
	PartCount          int
	Status             string
	ReservedBytes      int64
	ActualBytes        int64
	IdempotencyKey     string
	RequestFingerprint string
	CompletedParts     []storage.CompletedPart
	AssetID            *uuid.UUID
	ExpiresAt          time.Time
	CompletedAt        *time.Time
}

func (session UploadSession) MultipartUpload() storage.MultipartUpload {
	return storage.MultipartUpload{
		UploadID: session.BackendUploadID, ObjectKey: session.ObjectKey, ContentType: session.DeclaredMIMEType,
		SizeBytes: session.DeclaredSizeBytes, PartSize: session.PartSize, PartCount: session.PartCount, Driver: session.StorageDriver,
	}
}

type Asset struct {
	ID               uuid.UUID
	UserID           int64
	ProjectID        uuid.UUID
	Name             string
	GroupName        string
	MediaType        domainmedia.MediaType
	SourceType       string
	Status           string
	VisibilityStatus string
	StorageConfigID  string
	StorageDriver    string
	Bucket           string
	ObjectKey        string
	MIMEType         string
	FileSizeBytes    int64
	SHA256           string
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
}
