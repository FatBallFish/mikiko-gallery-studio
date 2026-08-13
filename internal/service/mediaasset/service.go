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
	PolicyResolver func(context.Context) (RuntimePolicy, error)
	Now            func() time.Time
	Observer       Observer
}

type RuntimePolicy struct {
	Policy         domainmedia.Policy
	UserQuotaBytes int64
	UploadTTL      time.Duration
}

type Observer interface {
	RecordUpload(stage, result string, bytes int64)
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

func (s *Service) recordUpload(stage, result string, bytes int64) {
	if s != nil && s.opts.Observer != nil {
		s.opts.Observer.RecordUpload(stage, result, bytes)
	}
}

type AccessResult struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
	Range     bool      `json:"range_supported"`
}

type ContentStream struct {
	Reader      io.ReadCloser
	SizeBytes   int64
	ContentType string
	Filename    string
}

const (
	AccessPurposeThumbnail = "thumbnail"
	AccessPurposePoster    = "poster"
	AccessPurposeHover     = "hover"
	AccessPurposePreview   = "preview"
	AccessPurposeWaveform  = "waveform"
	AccessPurposeDownload  = "download"
)

func ValidAccessPurpose(purpose string) bool {
	switch strings.ToLower(strings.TrimSpace(purpose)) {
	case AccessPurposeThumbnail, AccessPurposePoster, AccessPurposeHover, AccessPurposePreview, AccessPurposeWaveform, AccessPurposeDownload:
		return true
	default:
		return false
	}
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

func (s *Service) ListAssets(ctx context.Context, req AssetListRequest) (AssetPage, error) {
	if s == nil || s.store == nil {
		return AssetPage{}, errs.Internal("media asset service is unavailable")
	}
	return s.store.ListAssets(ctx, req)
}

func (s *Service) GetAsset(ctx context.Context, userID int64, assetID uuid.UUID) (Asset, error) {
	if s == nil || s.store == nil {
		return Asset{}, errs.Internal("media asset service is unavailable")
	}
	return s.store.GetAsset(ctx, userID, assetID)
}

func (s *Service) UpdateAsset(ctx context.Context, req UpdateAssetRequest) (Asset, error) {
	return s.store.UpdateAsset(ctx, req)
}

func (s *Service) DeleteAsset(ctx context.Context, req DeleteAssetRequest) (Asset, error) {
	return s.store.DeleteAsset(ctx, req)
}

func (s *Service) RetryProcessing(ctx context.Context, userID int64, assetID uuid.UUID) (Asset, error) {
	return s.store.RetryAssetProcessing(ctx, userID, assetID)
}

func (s *Service) Access(ctx context.Context, userID int64, assetID uuid.UUID, purpose string) (AccessResult, error) {
	asset, object, err := s.assetObject(ctx, userID, assetID, purpose)
	if err != nil {
		return AccessResult{}, err
	}
	ref, err := s.router.BackendFor(ctx, object.StorageConfigID, object.StorageDriver)
	if err != nil {
		return AccessResult{}, err
	}
	storagePurpose := storage.TemporaryMediaPurposePreview
	if purpose == AccessPurposeDownload {
		storagePurpose = storage.TemporaryMediaPurposeDownload
	}
	access, supported, err := storage.ProjectTemporaryMediaAccess(ctx, ref.Backend, object.ObjectKey, object.MIMEType, asset.Name, storagePurpose)
	if err != nil {
		return AccessResult{}, err
	}
	if supported {
		return AccessResult{URL: access.URL, ExpiresAt: access.ExpiresAt, Range: true}, nil
	}
	expiresAt := s.opts.Now().UTC().Add(storage.TemporaryMediaURLExpiry)
	return AccessResult{
		URL:       fmt.Sprintf("/api/agent/media/v1/assets/%s/content?purpose=%s", assetID.String(), purpose),
		ExpiresAt: expiresAt, Range: true,
	}, nil
}

func (s *Service) OpenContent(ctx context.Context, userID int64, assetID uuid.UUID, purpose string) (ContentStream, error) {
	asset, object, err := s.assetObject(ctx, userID, assetID, purpose)
	if err != nil {
		return ContentStream{}, err
	}
	ref, err := s.router.BackendFor(ctx, object.StorageConfigID, object.StorageDriver)
	if err != nil {
		return ContentStream{}, err
	}
	streaming, ok := ref.Backend.(storage.StreamingBackend)
	if !ok {
		return ContentStream{}, errs.New(503, errs.CodeArtifactStorageUnavailable, "media storage does not support streaming access")
	}
	reader, size, err := streaming.OpenReader(ctx, object.ObjectKey, domainmedia.SingleFileHardMaxBytes)
	if err != nil {
		return ContentStream{}, err
	}
	return ContentStream{Reader: reader, SizeBytes: size, ContentType: object.MIMEType, Filename: asset.Name}, nil
}

type assetStorageObject struct {
	StorageConfigID string
	StorageDriver   string
	Bucket          string
	ObjectKey       string
	MIMEType        string
}

func (s *Service) assetObject(ctx context.Context, userID int64, assetID uuid.UUID, purpose string) (Asset, assetStorageObject, error) {
	purpose = strings.ToLower(strings.TrimSpace(purpose))
	if !ValidAccessPurpose(purpose) {
		return Asset{}, assetStorageObject{}, errs.BadRequest("invalid media access purpose")
	}
	asset, err := s.store.GetAsset(ctx, userID, assetID)
	if err != nil {
		return Asset{}, assetStorageObject{}, err
	}
	object := assetStorageObject{
		StorageConfigID: asset.StorageConfigID, StorageDriver: asset.StorageDriver, Bucket: asset.Bucket,
		ObjectKey: asset.ObjectKey, MIMEType: asset.MIMEType,
	}
	if purpose == AccessPurposeDownload {
		return asset, object, nil
	}
	derivatives, err := s.store.ListReadyDerivatives(ctx, userID, assetID)
	if err != nil {
		return Asset{}, assetStorageObject{}, err
	}
	priorities := accessDerivativePriorities(asset.MediaType, purpose)
	for _, kind := range priorities {
		for _, derivative := range derivatives {
			if derivative.Kind == kind {
				return asset, assetStorageObject{
					StorageConfigID: derivative.StorageConfigID, StorageDriver: derivative.StorageDriver,
					Bucket: derivative.Bucket, ObjectKey: derivative.ObjectKey, MIMEType: derivative.MIMEType,
				}, nil
			}
		}
	}
	if purpose == AccessPurposeThumbnail && asset.MediaType == domainmedia.MediaTypeImage && asset.LegacyImageID != nil {
		return asset, object, nil
	}
	if purpose == AccessPurposePreview {
		return asset, object, nil
	}
	return Asset{}, assetStorageObject{}, errs.New(409, "DERIVATIVE_NOT_READY", "requested media derivative is not ready")
}

func accessDerivativePriorities(mediaType domainmedia.MediaType, purpose string) []domainmedia.DerivativeKind {
	switch purpose {
	case AccessPurposeThumbnail:
		if mediaType == domainmedia.MediaTypeImage {
			return []domainmedia.DerivativeKind{domainmedia.DerivativeThumbnail640, domainmedia.DerivativeThumbnail320}
		}
	case AccessPurposePoster:
		if mediaType == domainmedia.MediaTypeVideo {
			return []domainmedia.DerivativeKind{domainmedia.DerivativePoster}
		}
	case AccessPurposeHover:
		if mediaType == domainmedia.MediaTypeVideo {
			return []domainmedia.DerivativeKind{domainmedia.DerivativeHoverPreview}
		}
	case AccessPurposeWaveform:
		if mediaType == domainmedia.MediaTypeAudio {
			return []domainmedia.DerivativeKind{domainmedia.DerivativeWaveform}
		}
	}
	switch mediaType {
	case domainmedia.MediaTypeImage:
		return []domainmedia.DerivativeKind{domainmedia.DerivativePreview1280, domainmedia.DerivativeThumbnail640, domainmedia.DerivativeThumbnail320}
	case domainmedia.MediaTypeVideo, domainmedia.MediaTypeAudio:
		return []domainmedia.DerivativeKind{domainmedia.DerivativeProxy}
	default:
		return nil
	}
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
	runtimePolicy, err := s.runtimePolicy(ctx)
	if err != nil {
		return UploadSession{}, fmt.Errorf("resolve media upload policy: %w", err)
	}
	declaration := domainmedia.UploadDeclaration{Filename: req.Filename, MediaType: req.MediaType, MIMEType: req.MIMEType, SizeBytes: req.SizeBytes}
	if validationErr := runtimePolicy.Policy.ValidateDeclaration(declaration); validationErr != nil {
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
			s.recordUpload("initialize", "failed", req.SizeBytes)
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
		IdempotencyKey: req.IdempotencyKey, RequestFingerprint: fingerprint, ExpiresAt: s.opts.Now().UTC().Add(runtimePolicy.UploadTTL),
	}
	created, err := s.store.CreateUpload(ctx, CreateUploadRecord{Session: session, QuotaBytes: runtimePolicy.UserQuotaBytes})
	if err == nil {
		s.recordUpload("initialize", "success", req.SizeBytes)
		return created, nil
	}
	_ = multipart.AbortMultipart(context.WithoutCancel(ctx), backendUpload)
	if existing, found, findErr := s.store.FindUploadByIdempotency(ctx, req.UserID, req.IdempotencyKey); findErr == nil && found {
		if existing.RequestFingerprint == fingerprint {
			return existing, nil
		}
		s.recordUpload("initialize", "failed", req.SizeBytes)
		return UploadSession{}, errs.New(409, errs.CodeConflict, "idempotency key was already used with a different upload")
	}
	s.recordUpload("initialize", "failed", req.SizeBytes)
	return UploadSession{}, err
}

func (s *Service) runtimePolicy(ctx context.Context) (RuntimePolicy, error) {
	policy := RuntimePolicy{Policy: s.opts.Policy, UserQuotaBytes: s.opts.UserQuotaBytes, UploadTTL: s.opts.UploadTTL}
	if s.opts.PolicyResolver != nil {
		resolved, err := s.opts.PolicyResolver(ctx)
		if err != nil {
			return RuntimePolicy{}, err
		}
		policy = resolved
	}
	if policy.Policy.SingleFileMaxBytes <= 0 {
		policy.Policy = domainmedia.DefaultPolicy()
	}
	if policy.UploadTTL <= 0 {
		policy.UploadTTL = 24 * time.Hour
	}
	return policy, nil
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
		s.recordUpload("complete", "failed", 0)
		return Asset{}, err
	}
	if session.Status == "completed" && session.AssetID != nil {
		return s.store.CompleteUpload(ctx, CompleteUploadRecord{UserID: userID, SessionID: sessionID, AssetID: *session.AssetID, CompletedAt: s.opts.Now().UTC()})
	}
	if session.Status == "aborted" || session.ExpiresAt.Before(s.opts.Now().UTC()) {
		s.recordUpload("complete", "failed", session.DeclaredSizeBytes)
		return Asset{}, errs.New(409, errs.CodeConflict, "upload session is not completable")
	}
	session, err = s.store.MarkUploadCompleting(ctx, userID, sessionID, parts)
	if err != nil {
		s.recordUpload("complete", "failed", session.DeclaredSizeBytes)
		return Asset{}, err
	}
	completed, err := multipart.CompleteMultipart(ctx, session.MultipartUpload(), parts)
	if err != nil {
		s.recordUpload("complete", "failed", session.DeclaredSizeBytes)
		return Asset{}, err
	}
	assetID := uuid.New()
	asset, err := s.store.CompleteUpload(ctx, CompleteUploadRecord{
		UserID: userID, SessionID: sessionID, AssetID: assetID, Completed: completed, CompletedAt: s.opts.Now().UTC(),
	})
	if err != nil {
		_ = sessionStorageDelete(context.WithoutCancel(ctx), s.router, session)
		s.recordUpload("complete", "failed", session.DeclaredSizeBytes)
		return Asset{}, err
	}
	s.recordUpload("complete", "success", asset.FileSizeBytes)
	return asset, nil
}

func (s *Service) AbortUpload(ctx context.Context, userID int64, sessionID uuid.UUID) error {
	session, multipart, err := s.uploadBackend(ctx, userID, sessionID)
	if err != nil {
		s.recordUpload("abort", "failed", 0)
		return err
	}
	if session.Status == "aborted" {
		return nil
	}
	if session.Status == "completed" {
		s.recordUpload("abort", "failed", session.DeclaredSizeBytes)
		return errs.New(409, errs.CodeConflict, "completed upload cannot be aborted")
	}
	if err := multipart.AbortMultipart(ctx, session.MultipartUpload()); err != nil && !errors.Is(err, storage.ErrMultipartNotFound) {
		s.recordUpload("abort", "failed", session.DeclaredSizeBytes)
		return err
	}
	_, err = s.store.AbortUpload(ctx, userID, sessionID)
	if err == nil {
		s.recordUpload("abort", "success", session.DeclaredSizeBytes)
	} else {
		s.recordUpload("abort", "failed", session.DeclaredSizeBytes)
	}
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
