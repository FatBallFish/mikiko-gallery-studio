package mediaasset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

const (
	defaultUploadPartSize = int64(16 << 20)
	minimumUploadPartSize = int64(8 << 20)
	maximumUploadPartSize = int64(32 << 20)
)

type Options struct {
	Policy         domainmedia.Policy
	UserQuotaBytes int64
	PartSize       int64
	UploadTTL      time.Duration
	Now            func() time.Time
}

type InitUploadRequest struct {
	UserID         int64
	ProjectID      uuid.UUID
	GroupName      string
	Filename       string
	MediaType      domainmedia.MediaType
	MIMEType       string
	SizeBytes      int64
	Checksum       string
	IdempotencyKey string
}

type Service struct {
	store  Store
	router storage.Router
	opts   Options
}

func NewService(store Store, router storage.Router, opts Options) *Service {
	if opts.Policy.SingleFileMaxBytes <= 0 {
		opts.Policy = domainmedia.DefaultPolicy()
	}
	if opts.PartSize < minimumUploadPartSize || opts.PartSize > maximumUploadPartSize {
		opts.PartSize = defaultUploadPartSize
	}
	if opts.UploadTTL <= 0 {
		opts.UploadTTL = 24 * time.Hour
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Service{store: store, router: router, opts: opts}
}

func (s *Service) InitUpload(ctx context.Context, req InitUploadRequest) (UploadSession, error) {
	if s == nil || s.store == nil || s.router == nil {
		return UploadSession{}, errs.Internal("media upload service is unavailable")
	}
	req.Filename = strings.TrimSpace(req.Filename)
	req.MIMEType = strings.ToLower(strings.TrimSpace(req.MIMEType))
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	if req.UserID <= 0 || req.ProjectID == uuid.Nil || req.IdempotencyKey == "" {
		return UploadSession{}, errs.BadRequest("user, project and idempotency key are required")
	}
	declaration := domainmedia.UploadDeclaration{Filename: req.Filename, MediaType: req.MediaType, MIMEType: req.MIMEType, SizeBytes: req.SizeBytes}
	if validationErr := s.opts.Policy.ValidateDeclaration(declaration); validationErr != nil {
		return UploadSession{}, errs.BadRequest(validationErr.Error())
	}
	fingerprint, err := uploadRequestFingerprint(req)
	if err != nil {
		return UploadSession{}, err
	}
	if existing, found, err := s.store.FindUploadByIdempotency(ctx, req.UserID, req.IdempotencyKey); err != nil {
		return UploadSession{}, err
	} else if found {
		if existing.RequestFingerprint != fingerprint {
			return UploadSession{}, errs.New(409, errs.CodeConflict, "idempotency key was already used with a different upload")
		}
		return existing, nil
	}

	ref, err := s.router.DefaultWriter(ctx)
	if err != nil {
		return UploadSession{}, fmt.Errorf("resolve upload storage: %w", err)
	}
	multipart, ok := ref.Backend.(storage.MultipartBackend)
	if !ok {
		return UploadSession{}, errs.New(503, errs.CodeArtifactStorageUnavailable, "configured storage does not support resumable uploads")
	}
	sessionID := uuid.New()
	extension := strings.ToLower(filepath.Ext(req.Filename))
	objectKey := fmt.Sprintf("media/original/%d/%s/original%s", req.UserID, sessionID.String(), extension)
	backendUpload, err := multipart.CreateMultipart(ctx, storage.MultipartCreateRequest{
		ObjectKey: objectKey, ContentType: req.MIMEType, SizeBytes: req.SizeBytes, PartSize: s.opts.PartSize,
	})
	if err != nil {
		return UploadSession{}, fmt.Errorf("initialize multipart upload: %w", err)
	}
	session := UploadSession{
		ID: sessionID, UserID: req.UserID, ProjectID: req.ProjectID, GroupName: strings.TrimSpace(req.GroupName),
		OriginalFilename: req.Filename, DeclaredMediaType: req.MediaType, DeclaredMIMEType: req.MIMEType,
		DeclaredSizeBytes: req.SizeBytes, DeclaredChecksum: strings.TrimSpace(req.Checksum), StorageConfigID: strings.TrimSpace(ref.ConfigID),
		StorageDriver: ref.Driver, Bucket: ref.Bucket, ObjectKey: objectKey, BackendUploadID: backendUpload.UploadID,
		PartSize: backendUpload.PartSize, PartCount: backendUpload.PartCount, Status: "initialized", ReservedBytes: req.SizeBytes,
		IdempotencyKey: req.IdempotencyKey, RequestFingerprint: fingerprint, ExpiresAt: s.opts.Now().UTC().Add(s.opts.UploadTTL),
	}
	created, err := s.store.CreateUpload(ctx, CreateUploadRecord{Session: session, QuotaBytes: s.opts.UserQuotaBytes})
	if err == nil {
		return created, nil
	}
	_ = multipart.AbortMultipart(context.WithoutCancel(ctx), backendUpload)
	if existing, found, findErr := s.store.FindUploadByIdempotency(ctx, req.UserID, req.IdempotencyKey); findErr == nil && found {
		if existing.RequestFingerprint == fingerprint {
			return existing, nil
		}
		return UploadSession{}, errs.New(409, errs.CodeConflict, "idempotency key was already used with a different upload")
	}
	return UploadSession{}, err
}

func (s *Service) SignPart(ctx context.Context, userID int64, sessionID uuid.UUID, partNumber int, checksum string) (storage.MultipartPartTarget, error) {
	session, multipart, err := s.uploadBackend(ctx, userID, sessionID)
	if err != nil {
		return storage.MultipartPartTarget{}, err
	}
	if session.Status == "completed" || session.Status == "aborted" || session.ExpiresAt.Before(s.opts.Now().UTC()) {
		return storage.MultipartPartTarget{}, errs.New(409, errs.CodeConflict, "upload session is not writable")
	}
	return multipart.SignMultipartPart(ctx, session.MultipartUpload(), partNumber, checksum, 5*time.Minute)
}

func (s *Service) UploadLocalPart(ctx context.Context, userID int64, sessionID uuid.UUID, partNumber int, reader io.Reader, size int64, checksum string) (storage.CompletedPart, error) {
	session, multipart, err := s.uploadBackend(ctx, userID, sessionID)
	if err != nil {
		return storage.CompletedPart{}, err
	}
	if session.StorageDriver != "local" || session.Status == "completed" || session.Status == "aborted" || session.ExpiresAt.Before(s.opts.Now().UTC()) {
		return storage.CompletedPart{}, errs.New(409, errs.CodeConflict, "upload session is not writable through the local endpoint")
	}
	part, err := multipart.PutMultipartPart(ctx, session.MultipartUpload(), partNumber, reader, size, checksum)
	if err != nil {
		return storage.CompletedPart{}, err
	}
	status, err := multipart.MultipartStatus(ctx, session.MultipartUpload())
	if err != nil {
		return storage.CompletedPart{}, err
	}
	if _, err := s.store.RecordCompletedParts(ctx, userID, sessionID, status.CompletedParts); err != nil {
		return storage.CompletedPart{}, err
	}
	return part, nil
}

func (s *Service) Status(ctx context.Context, userID int64, sessionID uuid.UUID) (UploadSession, error) {
	session, multipart, err := s.uploadBackend(ctx, userID, sessionID)
	if err != nil {
		return UploadSession{}, err
	}
	if session.Status == "completed" || session.Status == "aborted" {
		return session, nil
	}
	status, err := multipart.MultipartStatus(ctx, session.MultipartUpload())
	if err != nil {
		return UploadSession{}, err
	}
	return s.store.RecordCompletedParts(ctx, userID, sessionID, status.CompletedParts)
}

func (s *Service) CompleteUpload(ctx context.Context, userID int64, sessionID uuid.UUID, parts []storage.CompletedPart) (Asset, error) {
	session, multipart, err := s.uploadBackend(ctx, userID, sessionID)
	if err != nil {
		return Asset{}, err
	}
	if session.Status == "completed" && session.AssetID != nil {
		return s.store.CompleteUpload(ctx, CompleteUploadRecord{UserID: userID, SessionID: sessionID, AssetID: *session.AssetID, CompletedAt: s.opts.Now().UTC()})
	}
	if session.Status == "aborted" || session.ExpiresAt.Before(s.opts.Now().UTC()) {
		return Asset{}, errs.New(409, errs.CodeConflict, "upload session is not completable")
	}
	session, err = s.store.MarkUploadCompleting(ctx, userID, sessionID, parts)
	if err != nil {
		return Asset{}, err
	}
	completed, err := multipart.CompleteMultipart(ctx, session.MultipartUpload(), parts)
	if err != nil {
		return Asset{}, err
	}
	assetID := uuid.New()
	asset, err := s.store.CompleteUpload(ctx, CompleteUploadRecord{
		UserID: userID, SessionID: sessionID, AssetID: assetID, Completed: completed, CompletedAt: s.opts.Now().UTC(),
	})
	if err != nil {
		_ = sessionStorageDelete(context.WithoutCancel(ctx), s.router, session)
		return Asset{}, err
	}
	return asset, nil
}

func (s *Service) AbortUpload(ctx context.Context, userID int64, sessionID uuid.UUID) error {
	session, multipart, err := s.uploadBackend(ctx, userID, sessionID)
	if err != nil {
		return err
	}
	if session.Status == "aborted" {
		return nil
	}
	if session.Status == "completed" {
		return errs.New(409, errs.CodeConflict, "completed upload cannot be aborted")
	}
	if err := multipart.AbortMultipart(ctx, session.MultipartUpload()); err != nil && !errors.Is(err, storage.ErrMultipartNotFound) {
		return err
	}
	_, err = s.store.AbortUpload(ctx, userID, sessionID)
	return err
}

func (s *Service) uploadBackend(ctx context.Context, userID int64, sessionID uuid.UUID) (UploadSession, storage.MultipartBackend, error) {
	session, err := s.store.GetUpload(ctx, userID, sessionID)
	if err != nil {
		return UploadSession{}, nil, err
	}
	ref, err := s.router.BackendFor(ctx, session.StorageConfigID, session.StorageDriver)
	if err != nil {
		return UploadSession{}, nil, err
	}
	multipart, ok := ref.Backend.(storage.MultipartBackend)
	if !ok {
		return UploadSession{}, nil, errs.New(503, errs.CodeArtifactStorageUnavailable, "upload storage no longer supports resumable uploads")
	}
	return session, multipart, nil
}

func uploadRequestFingerprint(req InitUploadRequest) (string, error) {
	payload, err := json.Marshal(struct {
		ProjectID uuid.UUID             `json:"project_id"`
		GroupName string                `json:"group_name"`
		Filename  string                `json:"filename"`
		MediaType domainmedia.MediaType `json:"media_type"`
		MIMEType  string                `json:"mime_type"`
		SizeBytes int64                 `json:"size_bytes"`
		Checksum  string                `json:"checksum"`
	}{req.ProjectID, strings.TrimSpace(req.GroupName), req.Filename, req.MediaType, req.MIMEType, req.SizeBytes, strings.TrimSpace(req.Checksum)})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func sessionStorageDelete(ctx context.Context, router storage.Router, session UploadSession) error {
	ref, err := router.BackendFor(ctx, session.StorageConfigID, session.StorageDriver)
	if err != nil {
		return err
	}
	return ref.Backend.Delete(ctx, session.ObjectKey)
}
