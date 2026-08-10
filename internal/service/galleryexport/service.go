package galleryexport

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/storage"
)

const (
	StateQueued    = "queued"
	StateRunning   = "running"
	StateSucceeded = "succeeded"
	StateFailed    = "failed"
	StateExpired   = "expired"

	FileStatusSucceeded = "succeeded"
	FileStatusFailed    = "failed"

	ErrorLifecycleDeadlineExceeded = "lifecycle_deadline_exceeded"
	ErrorExportTooLarge            = "EXPORT_TOO_LARGE"

	DefaultQueueTimeout     = 10 * time.Minute
	DefaultLifecycleTimeout = DefaultQueueTimeout + DefaultAsyncTimeout
)

var (
	ErrBatchEmpty           = errors.New("gallery export selection is empty")
	ErrBatchTooLarge        = errors.New("gallery export selection exceeds the batch limit")
	ErrExportNotReady       = errors.New("gallery export is not ready")
	ErrSourceLimitExceeded  = errors.New("gallery export source byte limit exceeded")
	ErrArchiveLimitExceeded = errors.New("gallery export archive byte limit exceeded")
)

type Asset struct {
	ID              string
	ProjectID       string
	StorageConfigID string
	StorageDriver   string
	ObjectKey       string
	MIMEType        string
	FileSizeBytes   int64
	DisplayName     string
}

type ManifestFile struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	Status    string `json:"status"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
	Message   string `json:"message,omitempty"`
}

type Manifest struct {
	Version int            `json:"version"`
	Files   []ManifestFile `json:"files"`
}

type Archive struct {
	Filename         string
	Manifest         Manifest
	Path             string
	Reader           io.ReadCloser
	Size             int64
	ResponseDeadline time.Time
	cancel           context.CancelFunc
}

func (a *Archive) Close() error {
	if a == nil {
		return nil
	}
	var result error
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	if a.Reader != nil {
		result = a.Reader.Close()
		a.Reader = nil
	}
	if a.Path == "" {
		return result
	}
	path := a.Path
	a.Path = ""
	return errors.Join(result, os.Remove(path))
}

type Job struct {
	ID               string     `json:"id"`
	UserID           int64      `json:"-"`
	ProjectID        string     `json:"project_id"`
	ImageIDs         []string   `json:"image_ids"`
	State            string     `json:"state"`
	EstimatedBytes   int64      `json:"estimated_bytes"`
	ArchiveSizeBytes int64      `json:"archive_size_bytes,omitempty"`
	StorageConfigID  string     `json:"-"`
	StorageDriver    string     `json:"-"`
	Bucket           string     `json:"-"`
	ObjectKey        string     `json:"-"`
	AttemptCount     int        `json:"attempt_count,omitempty"`
	LeaseOwner       string     `json:"-"`
	LeaseExpiresAt   *time.Time `json:"-"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	ErrorCode        string     `json:"error_code,omitempty"`
	ErrorMessage     string     `json:"error_message,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	DeadlineAt       *time.Time `json:"deadline_at,omitempty"`
}

func (s *Service) GetJob(ctx context.Context, userID int64, jobID string) (Job, error) {
	store, ok := s.store.(interface {
		GetJob(context.Context, int64, string, time.Time) (Job, error)
	})
	if !ok {
		return Job{}, errors.New("gallery export status is unavailable")
	}
	job, err := store.GetJob(ctx, userID, strings.TrimSpace(jobID), s.now())
	if err != nil {
		return Job{}, err
	}
	return s.decorateJob(job), nil
}

func (s *Service) DownloadJob(ctx context.Context, userID int64, jobID string) (Archive, error) {
	job, err := s.GetJob(ctx, userID, jobID)
	if err != nil {
		return Archive{}, err
	}
	if job.State != StateSucceeded || job.ExpiresAt == nil || !job.ExpiresAt.After(time.Now().UTC()) {
		return Archive{}, ErrExportNotReady
	}
	backend, err := s.router.BackendFor(ctx, job.StorageConfigID, job.StorageDriver)
	if err != nil {
		return Archive{}, fmt.Errorf("resolve gallery export storage: %w", err)
	}
	streaming, ok := backend.Backend.(storage.StreamingBackend)
	if !ok {
		return Archive{}, errors.New("gallery export storage does not support streaming downloads")
	}
	transferCtx, cancel := context.WithTimeout(ctx, s.opts.LifecycleTimeout)
	responseDeadline, _ := transferCtx.Deadline()
	reader, size, err := streaming.OpenReader(transferCtx, job.ObjectKey, s.opts.MaxArchiveBytes)
	if err != nil {
		cancel()
		return Archive{}, fmt.Errorf("open gallery export archive: %w", err)
	}
	if size < 0 {
		size = job.ArchiveSizeBytes
	}
	return Archive{Filename: "gallery-assets.zip", Reader: reader, Size: size, ResponseDeadline: responseDeadline, cancel: cancel}, nil
}

type CreateJobRequest struct {
	UserID              int64
	ProjectID           string
	ImageIDs            []string
	EstimatedBytes      int64
	LifecycleDeadlineAt time.Time
}

type Store interface {
	AuthorizeAssets(ctx context.Context, userID int64, projectID string, imageIDs []string) ([]Asset, error)
	CreateJob(ctx context.Context, req CreateJobRequest) (Job, error)
}

type Options struct {
	MaxBatchSize            int
	DirectMaxCount          int
	DirectMaxEstimatedBytes int64
	MaxFileCount            int
	MaxSourceBytes          int64
	MaxArchiveBytes         int64
	DirectTimeout           time.Duration
	LifecycleTimeout        time.Duration
	TempDir                 string
	Now                     func() time.Time
}

type Service struct {
	store  Store
	router storage.Router
	opts   Options
}

type CreateDownloadRequest struct {
	UserID    int64
	ProjectID string
	ImageIDs  []string
}

type CreateDownloadResult struct {
	Archive *Archive
	Job     *Job
}

func NewService(store Store, router storage.Router, opts Options) *Service {
	if opts.MaxBatchSize <= 0 {
		opts.MaxBatchSize = 100
	}
	if opts.DirectMaxCount <= 0 {
		opts.DirectMaxCount = 20
	}
	if opts.DirectMaxEstimatedBytes <= 0 {
		opts.DirectMaxEstimatedBytes = 64 << 20
	}
	if opts.MaxArchiveBytes <= 0 {
		opts.MaxArchiveBytes = 256 << 20
	}
	if opts.MaxSourceBytes <= 0 {
		opts.MaxSourceBytes = opts.MaxArchiveBytes
	}
	if opts.MaxFileCount <= 0 {
		opts.MaxFileCount = opts.MaxBatchSize
	}
	if opts.DirectTimeout <= 0 {
		opts.DirectTimeout = 2 * time.Minute
	}
	if opts.LifecycleTimeout <= 0 {
		opts.LifecycleTimeout = DefaultLifecycleTimeout
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Service{store: store, router: router, opts: opts}
}

func (s *Service) CreateDownload(ctx context.Context, req CreateDownloadRequest) (CreateDownloadResult, error) {
	deadline := time.Now().Add(s.opts.DirectTimeout)
	ctx, cancel := context.WithTimeout(ctx, s.opts.DirectTimeout)
	defer cancel()
	ids, err := normalizeIDs(req.ImageIDs, s.opts.MaxBatchSize)
	if err != nil {
		return CreateDownloadResult{}, err
	}
	assets, err := s.store.AuthorizeAssets(ctx, req.UserID, strings.TrimSpace(req.ProjectID), ids)
	if err != nil {
		return CreateDownloadResult{}, err
	}
	var estimatedBytes int64
	hasUnknownSize := false
	for _, asset := range assets {
		if asset.FileSizeBytes > 0 {
			estimatedBytes += asset.FileSizeBytes
		} else {
			hasUnknownSize = true
		}
	}
	if hasUnknownSize || len(assets) > s.opts.DirectMaxCount || estimatedBytes > s.opts.DirectMaxEstimatedBytes {
		job, err := s.store.CreateJob(ctx, CreateJobRequest{
			UserID: req.UserID, ProjectID: strings.TrimSpace(req.ProjectID), ImageIDs: ids, EstimatedBytes: estimatedBytes,
			LifecycleDeadlineAt: s.now().Add(s.opts.LifecycleTimeout),
		})
		if err != nil {
			return CreateDownloadResult{}, err
		}
		job = s.decorateJob(job)
		return CreateDownloadResult{Job: &job}, nil
	}
	archive, err := s.buildArchive(ctx, assets)
	if err != nil {
		return CreateDownloadResult{}, err
	}
	archive.ResponseDeadline = deadline
	return CreateDownloadResult{Archive: &archive}, nil
}

func (s *Service) decorateJob(job Job) Job {
	if job.DeadlineAt != nil {
		deadline := job.DeadlineAt.UTC()
		job.DeadlineAt = &deadline
	}
	return job
}

func (s *Service) now() time.Time {
	return s.opts.Now().UTC()
}

func normalizeIDs(values []string, max int) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
		if max > 0 && len(result) > max {
			return nil, ErrBatchTooLarge
		}
	}
	if len(result) == 0 {
		return nil, ErrBatchEmpty
	}
	return result, nil
}

func (s *Service) buildArchive(ctx context.Context, assets []Asset) (Archive, error) {
	if len(assets) > s.opts.MaxFileCount {
		return Archive{}, ErrBatchTooLarge
	}
	manifest := Manifest{Version: 1, Files: make([]ManifestFile, 0, len(assets))}
	temp, err := os.CreateTemp(s.opts.TempDir, "gallery-export-*.zip")
	if err != nil {
		return Archive{}, fmt.Errorf("create temporary ZIP archive: %w", err)
	}
	archive := Archive{Filename: "gallery-assets.zip", Path: temp.Name()}
	keep := false
	defer func() {
		_ = temp.Close()
		if !keep {
			_ = os.Remove(temp.Name())
		}
	}()
	bounded := &archiveLimitWriter{writer: temp, max: s.opts.MaxArchiveBytes}
	writer := zip.NewWriter(bounded)
	usedNames := map[string]int{"manifest.json": 1}
	var sourceBytes int64
	for _, asset := range assets {
		if err := ctx.Err(); err != nil {
			_ = writer.Close()
			return Archive{}, err
		}
		filename := uniqueArchiveFilename(asset.DisplayName, asset.MIMEType, asset.ObjectKey, usedNames)
		entry := ManifestFile{ID: asset.ID, Filename: filename}
		backend, err := s.router.BackendFor(ctx, asset.StorageConfigID, asset.StorageDriver)
		if err != nil {
			entry.Status, entry.ErrorCode, entry.Message = FileStatusFailed, "storage_unavailable", "image storage is unavailable"
			manifest.Files = append(manifest.Files, entry)
			continue
		}
		remaining := s.opts.MaxSourceBytes - sourceBytes
		if remaining <= 0 {
			_ = writer.Close()
			return Archive{}, ErrSourceLimitExceeded
		}
		content, err := getBounded(ctx, backend.Backend, asset.ObjectKey, remaining)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				_ = writer.Close()
				return Archive{}, ctxErr
			}
			if errors.Is(err, storage.ErrObjectTooLarge) {
				_ = writer.Close()
				return Archive{}, ErrSourceLimitExceeded
			}
			entry.Status, entry.ErrorCode, entry.Message = FileStatusFailed, exportReadErrorCode(err), "image file could not be read"
			manifest.Files = append(manifest.Files, entry)
			continue
		}
		file, err := writer.Create(filename)
		if err != nil {
			_ = writer.Close()
			return Archive{}, archiveWriteError("create ZIP entry", err)
		}
		if _, err := file.Write(content); err != nil {
			_ = writer.Close()
			return Archive{}, archiveWriteError("write ZIP entry", err)
		}
		sourceBytes += int64(len(content))
		entry.Status, entry.SizeBytes = FileStatusSucceeded, int64(len(content))
		manifest.Files = append(manifest.Files, entry)
	}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		_ = writer.Close()
		return Archive{}, fmt.Errorf("encode ZIP manifest: %w", err)
	}
	file, err := writer.Create("manifest.json")
	if err != nil {
		_ = writer.Close()
		return Archive{}, archiveWriteError("create ZIP manifest", err)
	}
	if _, err := file.Write(payload); err != nil {
		_ = writer.Close()
		return Archive{}, archiveWriteError("write ZIP manifest", err)
	}
	if err := writer.Close(); err != nil {
		return Archive{}, archiveWriteError("close ZIP archive", err)
	}
	if err := temp.Sync(); err != nil {
		return Archive{}, fmt.Errorf("sync ZIP archive: %w", err)
	}
	if err := temp.Close(); err != nil {
		return Archive{}, fmt.Errorf("close temporary ZIP archive: %w", err)
	}
	archive.Manifest = manifest
	archive.Size = bounded.written
	keep = true
	return archive, nil
}

type archiveLimitWriter struct {
	writer  io.Writer
	max     int64
	written int64
}

func (w *archiveLimitWriter) Write(payload []byte) (int, error) {
	remaining := w.max - w.written
	if remaining <= 0 {
		return 0, ErrArchiveLimitExceeded
	}
	if int64(len(payload)) > remaining {
		written, err := w.writer.Write(payload[:remaining])
		w.written += int64(written)
		if err != nil {
			return written, err
		}
		return written, ErrArchiveLimitExceeded
	}
	written, err := w.writer.Write(payload)
	w.written += int64(written)
	return written, err
}

func archiveWriteError(operation string, err error) error {
	if errors.Is(err, ErrArchiveLimitExceeded) {
		return ErrArchiveLimitExceeded
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func getBounded(ctx context.Context, backend storage.Backend, objectKey string, maxBytes int64) ([]byte, error) {
	if getter, ok := backend.(storage.BoundedGetter); ok {
		return getter.GetBounded(ctx, objectKey, maxBytes)
	}
	content, err := backend.Get(ctx, objectKey)
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > maxBytes {
		return nil, storage.ErrObjectTooLarge
	}
	return content, nil
}

func exportReadErrorCode(err error) string {
	switch {
	case errors.Is(err, storage.ErrNotFound):
		return "object_not_found"
	case errors.Is(err, storage.ErrObjectTooLarge):
		return "object_too_large"
	default:
		return "object_read_failed"
	}
}

var unsafeArchiveName = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]`)

func uniqueArchiveFilename(displayName, mimeType, objectKey string, used map[string]int) string {
	base := strings.TrimSpace(displayName)
	base = unsafeArchiveName.ReplaceAllString(base, " ")
	base = strings.Trim(strings.Join(strings.Fields(base), "-"), ".- ")
	if strings.EqualFold(base, "manifest.json") {
		base = "asset"
	}
	if base == "" {
		base = "image"
	}
	ext := strings.ToLower(filepath.Ext(base))
	if ext == "" {
		ext = extensionForAsset(mimeType, objectKey)
	} else {
		base = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if base == "" {
		base = "image"
	}
	candidate := base + ext
	key := strings.ToLower(candidate)
	if _, exists := used[key]; !exists {
		used[key] = 1
		return candidate
	}
	for suffix := used[key] + 1; ; suffix++ {
		candidate = fmt.Sprintf("%s-%d%s", base, suffix, ext)
		candidateKey := strings.ToLower(candidate)
		if _, exists := used[candidateKey]; exists {
			continue
		}
		used[key], used[candidateKey] = suffix, 1
		return candidate
	}
}

func extensionForAsset(mimeType, objectKey string) string {
	if extensions, err := mime.ExtensionsByType(strings.TrimSpace(mimeType)); err == nil && len(extensions) > 0 {
		return strings.ToLower(extensions[0])
	}
	ext := strings.ToLower(filepath.Ext(objectKey))
	if len(ext) > 1 && len(ext) <= 10 {
		return ext
	}
	return ".bin"
}
