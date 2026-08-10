package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
)

var (
	ErrNotFound                = errors.New("storage object not found")
	ErrObjectTooLarge          = errors.New("storage object exceeds maximum size")
	ErrSizeMismatch            = errors.New("storage stream size does not match declared length")
	errInvalidBoundedReadLimit = errors.New("storage bounded read limit is invalid")
)

type Backend interface {
	Driver() string
	Put(ctx context.Context, objectKey string, contentType string, content []byte) error
	Get(ctx context.Context, objectKey string) ([]byte, error)
	Delete(ctx context.Context, objectKey string) error
}

type ObjectInfo struct {
	ObjectKey  string
	ModifiedAt time.Time
}

type ObjectPage struct {
	Objects    []ObjectInfo
	NextCursor string
}

// ObjectLister is an optional capability used only with platform-owned
// prefixes selected by the cleanup processor.
type ObjectLister interface {
	ListObjects(ctx context.Context, prefix, cursor string, limit int) (ObjectPage, error)
}

// BoundedGetter is an optional backend capability for callers that must not
// allocate based on untrusted object sizes. Get remains unchanged for normal
// asset reads.
type BoundedGetter interface {
	GetBounded(ctx context.Context, objectKey string, maxBytes int64) ([]byte, error)
}

// StreamingBackend transfers bounded large objects without materializing them
// as a single byte slice. Callers own the returned reader and must close it.
type StreamingBackend interface {
	PutReader(ctx context.Context, objectKey, contentType string, reader io.Reader, size int64) error
	OpenReader(ctx context.Context, objectKey string, maxBytes int64) (io.ReadCloser, int64, error)
}

// ObjectCopier is an optional backend capability for copying objects without
// routing their bytes through the application service.
type ObjectCopier interface {
	Copy(ctx context.Context, sourceKey, destinationKey string) error
}

type TemporaryGetURLOptions struct {
	Expiry               time.Duration
	SigningTimeBucket    time.Duration
	ResponseFilename     string
	ContentType          string
	ResponseCacheControl string
}

type TemporaryURLSigner interface {
	TemporaryGetURL(ctx context.Context, objectKey string, options TemporaryGetURLOptions) (string, error)
}

const (
	TemporaryMediaURLExpiry            = 5 * time.Minute
	temporaryMediaPreviewSigningBucket = time.Minute
)

type TemporaryMediaURLs struct {
	PreviewURL        string
	DownloadURL       string
	PreviewExpiresAt  time.Time
	DownloadExpiresAt time.Time
}

type TemporaryMediaAccess struct {
	URL       string
	ExpiresAt time.Time
}

const (
	TemporaryMediaPurposePreview  = "preview"
	TemporaryMediaPurposeDownload = "download"
)

func ProjectTemporaryMediaURLs(ctx context.Context, backend Backend, objectKey, contentType, responseFilename string) (TemporaryMediaURLs, bool, error) {
	preview, supported, err := ProjectTemporaryMediaAccess(ctx, backend, objectKey, contentType, responseFilename, TemporaryMediaPurposePreview)
	if err != nil || !supported {
		return TemporaryMediaURLs{}, supported, err
	}
	download, _, err := ProjectTemporaryMediaAccess(ctx, backend, objectKey, contentType, responseFilename, TemporaryMediaPurposeDownload)
	if err != nil {
		return TemporaryMediaURLs{}, true, err
	}
	return TemporaryMediaURLs{
		PreviewURL: preview.URL, DownloadURL: download.URL,
		PreviewExpiresAt: preview.ExpiresAt, DownloadExpiresAt: download.ExpiresAt,
	}, true, nil
}

func ProjectTemporaryMediaAccess(ctx context.Context, backend Backend, objectKey, contentType, responseFilename, purpose string) (TemporaryMediaAccess, bool, error) {
	signer, ok := backend.(TemporaryURLSigner)
	if !ok {
		return TemporaryMediaAccess{}, false, nil
	}
	objectKey = strings.TrimSpace(objectKey)
	if objectKey == "" {
		return TemporaryMediaAccess{}, true, errors.New("temporary media object key is required")
	}
	options := TemporaryGetURLOptions{ContentType: strings.TrimSpace(contentType)}
	switch strings.ToLower(strings.TrimSpace(purpose)) {
	case TemporaryMediaPurposePreview:
		options.Expiry = TemporaryMediaURLExpiry + temporaryMediaPreviewSigningBucket
		options.SigningTimeBucket = temporaryMediaPreviewSigningBucket
		options.ResponseCacheControl = fmt.Sprintf("private, max-age=%d", int64(TemporaryMediaURLExpiry/time.Second))
	case TemporaryMediaPurposeDownload:
		options.Expiry = TemporaryMediaURLExpiry
		options.ResponseFilename = strings.TrimSpace(responseFilename)
	default:
		return TemporaryMediaAccess{}, true, errors.New("temporary media purpose must be preview or download")
	}
	projectedURL, err := signer.TemporaryGetURL(ctx, objectKey, options)
	if err != nil {
		return TemporaryMediaAccess{}, true, fmt.Errorf("sign temporary media %s URL: %w", purpose, err)
	}
	projectedURL, err = validateTemporaryMediaURL(projectedURL)
	if err != nil {
		return TemporaryMediaAccess{}, true, fmt.Errorf("validate temporary media %s URL: %w", purpose, err)
	}
	return TemporaryMediaAccess{URL: projectedURL, ExpiresAt: temporaryMediaURLExpiry(projectedURL)}, true, nil
}

func temporaryMediaURLExpiry(value string) time.Time {
	target, err := url.Parse(value)
	if err != nil {
		return time.Time{}
	}
	signedAt, err := time.Parse("20060102T150405Z", target.Query().Get("X-Amz-Date"))
	if err != nil {
		return time.Time{}
	}
	expiresSeconds, err := strconv.ParseInt(target.Query().Get("X-Amz-Expires"), 10, 64)
	if err != nil || expiresSeconds <= 0 {
		return time.Time{}
	}
	return signedAt.Add(time.Duration(expiresSeconds) * time.Second)
}

func validateTemporaryMediaURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	target, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	if (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" || target.User != nil || target.Fragment != "" {
		return "", errors.New("temporary media URL must be an absolute HTTP(S) URL without credentials or fragment")
	}
	return target.String(), nil
}

var (
	_ BoundedGetter      = (*LocalBackend)(nil)
	_ BoundedGetter      = (*S3Backend)(nil)
	_ StreamingBackend   = (*LocalBackend)(nil)
	_ StreamingBackend   = (*S3Backend)(nil)
	_ ObjectCopier       = (*LocalBackend)(nil)
	_ ObjectCopier       = (*S3Backend)(nil)
	_ TemporaryURLSigner = (*S3Backend)(nil)
	_ ObjectLister       = (*LocalBackend)(nil)
	_ ObjectLister       = (*S3Backend)(nil)
)

func NewBackend(cfg config.StorageConfig) (Backend, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Driver)) {
	case "", "local":
		return NewLocalBackend(cfg.LocalRoot), nil
	case "s3":
		return NewS3Backend(cfg)
	default:
		return nil, fmt.Errorf("unsupported storage driver %q", cfg.Driver)
	}
}

type LocalBackend struct {
	root string
}

func NewLocalBackend(root string) *LocalBackend {
	if strings.TrimSpace(root) == "" {
		root = filepath.Join(os.TempDir(), "pic-gallery")
	}
	return &LocalBackend{root: root}
}

func (b *LocalBackend) Driver() string { return "local" }

func (b *LocalBackend) Put(_ context.Context, objectKey string, _ string, content []byte) error {
	fullPath, ok := b.resolvePath(objectKey)
	if !ok {
		return fmt.Errorf("invalid local storage key %q", objectKey)
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(fullPath, content, 0o644)
}

func (b *LocalBackend) PutReader(ctx context.Context, objectKey, _ string, reader io.Reader, size int64) error {
	if size < 0 || reader == nil {
		return ErrSizeMismatch
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	fullPath, ok := b.resolvePath(objectKey)
	if !ok {
		return fmt.Errorf("invalid local storage key %q", objectKey)
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(fullPath), ".put-*")
	if err != nil {
		return fmt.Errorf("create local storage stream target: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	source := contextReader{ctx: ctx, reader: reader}
	written, copyErr := io.CopyN(temporary, source, size)
	if copyErr != nil || written != size {
		if ctxErr := contextError(ctx); ctxErr != nil {
			return ctxErr
		}
		return ErrSizeMismatch
	}
	var extra [1]byte
	if count, readErr := source.Read(extra[:]); count != 0 || (readErr != nil && !errors.Is(readErr, io.EOF)) {
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		return ErrSizeMismatch
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync local storage stream target: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close local storage stream target: %w", err)
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := os.Chmod(temporaryPath, 0o644); err != nil {
		return fmt.Errorf("set local storage stream permissions: %w", err)
	}
	if err := os.Rename(temporaryPath, fullPath); err != nil {
		return fmt.Errorf("commit local storage stream: %w", err)
	}
	committed = true
	return nil
}

func (b *LocalBackend) Get(_ context.Context, objectKey string) ([]byte, error) {
	fullPath, ok := b.resolvePath(objectKey)
	if !ok {
		return nil, ErrNotFound
	}
	content, err := os.ReadFile(fullPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return content, nil
}

func (b *LocalBackend) GetBounded(ctx context.Context, objectKey string, maxBytes int64) ([]byte, error) {
	if err := validateBoundedReadLimit(maxBytes); err != nil {
		return nil, err
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	fullPath, ok := b.resolvePath(objectKey)
	if !ok {
		return nil, ErrNotFound
	}
	file, err := os.Open(fullPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return readBoundedAndClose(ctx, file, maxBytes)
}

func (b *LocalBackend) OpenReader(ctx context.Context, objectKey string, maxBytes int64) (io.ReadCloser, int64, error) {
	if err := validateBoundedReadLimit(maxBytes); err != nil {
		return nil, 0, err
	}
	if err := contextError(ctx); err != nil {
		return nil, 0, err
	}
	fullPath, ok := b.resolvePath(objectKey)
	if !ok {
		return nil, 0, ErrNotFound
	}
	file, err := os.Open(fullPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, 0, err
	}
	if info.Size() > maxBytes {
		_ = file.Close()
		return nil, 0, ErrObjectTooLarge
	}
	return newBoundedStreamReader(ctx, file, maxBytes), info.Size(), nil
}

func (b *LocalBackend) Copy(ctx context.Context, sourceKey, destinationKey string) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	sourcePath, ok := b.resolvePath(sourceKey)
	if !ok {
		return ErrNotFound
	}
	destinationPath, ok := b.resolvePath(destinationKey)
	if !ok {
		return fmt.Errorf("invalid local storage destination key %q", destinationKey)
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return fmt.Errorf("open local storage source %q: %w", sourceKey, err)
	}
	defer source.Close()

	destinationDir := filepath.Dir(destinationPath)
	if err := os.MkdirAll(destinationDir, 0o755); err != nil {
		return fmt.Errorf("create local storage destination directory: %w", err)
	}
	temporary, err := os.CreateTemp(destinationDir, ".copy-*")
	if err != nil {
		return fmt.Errorf("create local storage copy target: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()

	_, copyErr := io.Copy(temporary, contextReader{ctx: ctx, reader: source})
	closeErr := temporary.Close()
	if copyErr != nil {
		return fmt.Errorf("copy local storage object: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close local storage copy target: %w", closeErr)
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := os.Chmod(temporaryPath, 0o644); err != nil {
		return fmt.Errorf("set local storage copy permissions: %w", err)
	}
	if err := os.Rename(temporaryPath, destinationPath); err != nil {
		return fmt.Errorf("commit local storage copy: %w", err)
	}
	committed = true
	return nil
}

func (b *LocalBackend) Delete(_ context.Context, objectKey string) error {
	fullPath, ok := b.resolvePath(objectKey)
	if !ok {
		return ErrNotFound
	}
	if err := os.Remove(fullPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (b *LocalBackend) ListObjects(ctx context.Context, prefix, cursor string, limit int) (ObjectPage, error) {
	page, _, err := b.listObjectsIncrementally(ctx, prefix, cursor, limit)
	return page, err
}

type localObjectListStats struct {
	VisitedObjects       int
	MaterializedObjects  int
	DirectoriesRead      int
	DirectoryEntriesRead int
}

func (b *LocalBackend) listObjectsIncrementally(ctx context.Context, prefix, cursor string, limit int) (ObjectPage, localObjectListStats, error) {
	var stats localObjectListStats
	prefix, err := normalizeListPrefix(prefix)
	if err != nil {
		return ObjectPage{}, stats, err
	}
	frames, err := decodeLocalObjectCursor(cursor, prefix)
	if err != nil {
		return ObjectPage{}, stats, err
	}
	limit = normalizeObjectListLimit(limit)
	rootKey := strings.TrimSuffix(prefix, "/")
	rootPath, ok := b.resolvePath(rootKey)
	if !ok {
		return ObjectPage{}, stats, fmt.Errorf("invalid local storage prefix %q", prefix)
	}
	rootInfo, err := os.Lstat(rootPath)
	if errors.Is(err, os.ErrNotExist) {
		return ObjectPage{}, stats, nil
	}
	if err != nil {
		return ObjectPage{}, stats, err
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return ObjectPage{}, stats, nil
	}
	if len(frames) == 0 {
		frames = []localObjectCursorFrame{{Directory: rootKey}}
	}

	objects := make([]ObjectInfo, 0, limit+1)
	walker := localObjectWalker{backend: b, prefix: prefix, frames: frames, stats: &stats}
	var resumeFrames []localObjectCursorFrame
	for len(objects) < limit+1 {
		object, ok, err := walker.next(ctx)
		if err != nil {
			return ObjectPage{}, stats, err
		}
		if !ok {
			break
		}
		objects = append(objects, object)
		if len(objects) == limit {
			resumeFrames = cloneLocalObjectCursorFrames(walker.frames)
		}
	}
	pageLength := len(objects)
	if pageLength > limit {
		pageLength = limit
	}
	page := ObjectPage{Objects: append([]ObjectInfo(nil), objects[:pageLength]...)}
	if len(objects) > limit && pageLength > 0 {
		page.NextCursor, err = encodeLocalObjectCursor(prefix, resumeFrames)
		if err != nil {
			return ObjectPage{}, stats, err
		}
	}
	return page, stats, nil
}

type localObjectWalker struct {
	backend *LocalBackend
	prefix  string
	frames  []localObjectCursorFrame
	stats   *localObjectListStats
}

func (w *localObjectWalker) next(ctx context.Context) (ObjectInfo, bool, error) {
	for len(w.frames) > 0 {
		if err := contextError(ctx); err != nil {
			return ObjectInfo{}, false, err
		}
		frame := &w.frames[len(w.frames)-1]
		if len(frame.Pending) == 0 {
			directoryPath, ok := w.backend.resolvePath(frame.Directory)
			if !ok {
				return ObjectInfo{}, false, errors.New("invalid local object listing cursor")
			}
			names, nextOffset, eof, err := readLocalDirectoryChunk(directoryPath, frame.Offset)
			if errors.Is(err, os.ErrNotExist) {
				w.frames = w.frames[:len(w.frames)-1]
				continue
			}
			if err != nil {
				return ObjectInfo{}, false, err
			}
			w.stats.DirectoriesRead++
			w.stats.DirectoryEntriesRead += len(names)
			frame.Offset = nextOffset
			frame.Pending = names
			if len(names) == 0 {
				if eof {
					w.frames = w.frames[:len(w.frames)-1]
				}
				continue
			}
		}

		name := frame.Pending[0]
		frame.Pending = frame.Pending[1:]
		objectKey := path.Join(frame.Directory, name)
		if !strings.HasPrefix(objectKey, w.prefix) {
			return ObjectInfo{}, false, errors.New("invalid local object listing cursor")
		}
		objectPath, ok := w.backend.resolvePath(objectKey)
		if !ok {
			return ObjectInfo{}, false, errors.New("invalid local object listing cursor")
		}
		info, err := os.Lstat(objectPath)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return ObjectInfo{}, false, err
		}
		if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			w.frames = append(w.frames, localObjectCursorFrame{Directory: objectKey})
			continue
		}
		w.stats.VisitedObjects++
		w.stats.MaterializedObjects++
		return ObjectInfo{ObjectKey: objectKey, ModifiedAt: info.ModTime().UTC()}, true, nil
	}
	return ObjectInfo{}, false, nil
}

type localObjectCursor struct {
	Version int                      `json:"v"`
	Prefix  string                   `json:"prefix"`
	Frames  []localObjectCursorFrame `json:"frames"`
}

type localObjectCursorFrame struct {
	Directory string   `json:"directory"`
	Offset    int64    `json:"offset"`
	Pending   []string `json:"pending,omitempty"`
}

func encodeLocalObjectCursor(prefix string, frames []localObjectCursorFrame) (string, error) {
	payload, err := json.Marshal(localObjectCursor{Version: 2, Prefix: prefix, Frames: frames})
	if err != nil {
		return "", fmt.Errorf("encode local object listing cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeLocalObjectCursor(cursor, prefix string) ([]localObjectCursorFrame, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return nil, nil
	}
	if len(cursor) > 64*1024 {
		return nil, errors.New("invalid local object listing cursor")
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(cursor)
	if err != nil {
		return nil, errors.New("invalid local object listing cursor")
	}
	var decoded localObjectCursor
	if err := json.Unmarshal(payload, &decoded); err != nil || decoded.Version != 2 || decoded.Prefix != prefix || !validLocalObjectCursorFrames(decoded.Frames, prefix) {
		return nil, errors.New("invalid local object listing cursor")
	}
	return cloneLocalObjectCursorFrames(decoded.Frames), nil
}

func validLocalObjectCursorFrames(frames []localObjectCursorFrame, prefix string) bool {
	if len(frames) == 0 || len(frames) > 256 || frames[0].Directory != strings.TrimSuffix(prefix, "/") {
		return false
	}
	for index, frame := range frames {
		if frame.Offset < 0 || strings.TrimSpace(frame.Directory) != frame.Directory || path.Clean(frame.Directory) != frame.Directory || strings.HasSuffix(frame.Directory, "/") {
			return false
		}
		if index > 0 && path.Dir(frame.Directory) != frames[index-1].Directory {
			return false
		}
		if len(frame.Pending) > 128 {
			return false
		}
		for _, name := range frame.Pending {
			if strings.TrimSpace(name) != name || name == "" || name == "." || name == ".." || path.Base(name) != name {
				return false
			}
		}
	}
	return true
}

func cloneLocalObjectCursorFrames(frames []localObjectCursorFrame) []localObjectCursorFrame {
	cloned := make([]localObjectCursorFrame, len(frames))
	for index, frame := range frames {
		cloned[index] = frame
		cloned[index].Pending = append([]string(nil), frame.Pending...)
	}
	return cloned
}

func (b *LocalBackend) resolvePath(objectKey string) (string, bool) {
	cleanKey := path.Clean(strings.TrimSpace(objectKey))
	if cleanKey == "." || cleanKey == "/" || strings.HasPrefix(cleanKey, "../") || strings.HasPrefix(cleanKey, "/") {
		return "", false
	}
	rootAbs, err := filepath.Abs(b.root)
	if err != nil {
		return "", false
	}
	fullPath, err := filepath.Abs(filepath.Join(rootAbs, filepath.FromSlash(cleanKey)))
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(rootAbs, fullPath)
	if err != nil {
		return "", false
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return fullPath, true
}

type S3Backend struct {
	endpoint        *url.URL
	region          string
	bucket          string
	accessKeyID     string
	secretAccessKey string
	prefix          string
	forcePathStyle  bool
	client          *http.Client
	now             func() time.Time
}

const defaultS3RequestTimeout = 30 * time.Second

func NewS3Backend(cfg config.StorageConfig) (*S3Backend, error) {
	rawEndpoint := strings.TrimSpace(cfg.S3.Endpoint)
	if rawEndpoint == "" {
		return nil, fmt.Errorf("storage.s3.endpoint is required")
	}
	endpoint, err := url.Parse(rawEndpoint)
	if err != nil {
		return nil, fmt.Errorf("parse storage.s3.endpoint: %w", err)
	}
	if endpoint.Scheme == "" || endpoint.Host == "" {
		return nil, fmt.Errorf("storage.s3.endpoint must include scheme and host")
	}
	if strings.TrimSpace(cfg.S3.Region) == "" {
		return nil, fmt.Errorf("storage.s3.region is required")
	}
	if strings.TrimSpace(cfg.S3.Bucket) == "" {
		return nil, fmt.Errorf("storage.s3.bucket is required")
	}
	if strings.TrimSpace(cfg.S3.AccessKeyID) == "" || strings.TrimSpace(cfg.S3.SecretAccessKey) == "" {
		return nil, fmt.Errorf("storage.s3 credentials are required")
	}
	return &S3Backend{
		endpoint:        endpoint,
		region:          strings.TrimSpace(cfg.S3.Region),
		bucket:          strings.TrimSpace(cfg.S3.Bucket),
		accessKeyID:     strings.TrimSpace(cfg.S3.AccessKeyID),
		secretAccessKey: strings.TrimSpace(cfg.S3.SecretAccessKey),
		prefix:          strings.Trim(strings.TrimSpace(cfg.S3.Prefix), "/"),
		forcePathStyle:  cfg.S3.ForcePathStyle,
		client:          &http.Client{},
		now:             time.Now,
	}, nil
}

func (b *S3Backend) Driver() string { return "s3" }

func (b *S3Backend) TemporaryGetURL(ctx context.Context, objectKey string, options TemporaryGetURLOptions) (string, error) {
	if err := contextError(ctx); err != nil {
		return "", err
	}
	key := b.normalizeKey(objectKey)
	if key == "" {
		return "", fmt.Errorf("invalid s3 object key %q", objectKey)
	}
	requestURL, host, canonicalURI, err := b.requestTarget(key)
	if err != nil {
		return "", err
	}
	target, err := url.Parse(requestURL)
	if err != nil {
		return "", err
	}

	expiry := options.Expiry
	if expiry <= 0 {
		expiry = 5 * time.Minute
	}
	if expiry > 7*24*time.Hour {
		expiry = 7 * 24 * time.Hour
	}
	expiresSeconds := int64(expiry / time.Second)
	if expiresSeconds < 1 {
		expiresSeconds = 1
	}

	now := b.nowUTC()
	if bucket := options.SigningTimeBucket; bucket > 0 {
		now = now.Truncate(bucket)
	}
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	credentialScope := dateStamp + "/" + b.region + "/s3/aws4_request"
	query := url.Values{
		"X-Amz-Algorithm":     {"AWS4-HMAC-SHA256"},
		"X-Amz-Credential":    {b.accessKeyID + "/" + credentialScope},
		"X-Amz-Date":          {amzDate},
		"X-Amz-Expires":       {strconv.FormatInt(expiresSeconds, 10)},
		"X-Amz-SignedHeaders": {"host"},
	}
	if contentType := strings.TrimSpace(options.ContentType); contentType != "" {
		query.Set("response-content-type", contentType)
	}
	if cacheControl := strings.TrimSpace(options.ResponseCacheControl); cacheControl != "" {
		query.Set("response-cache-control", cacheControl)
	}
	if filename := strings.TrimSpace(options.ResponseFilename); filename != "" {
		query.Set("response-content-disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	}
	canonicalQuery := awsCanonicalQuery(query)
	canonicalRequest := strings.Join([]string{
		http.MethodGet,
		canonicalURI,
		canonicalQuery,
		"host:" + host + "\n",
		"host",
		"UNSIGNED-PAYLOAD",
	}, "\n")
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
	query.Set("X-Amz-Signature", hex.EncodeToString(hmacSHA256(b.signingKey(dateStamp), stringToSign)))
	target.RawQuery = awsCanonicalQuery(query)
	return target.String(), nil
}

func (b *S3Backend) nowUTC() time.Time {
	if b == nil || b.now == nil {
		return time.Now().UTC()
	}
	return b.now().UTC()
}

func awsCanonicalQuery(query url.Values) string {
	return strings.ReplaceAll(query.Encode(), "+", "%20")
}

func (b *S3Backend) Put(ctx context.Context, objectKey string, contentType string, content []byte) error {
	ctx, cancel := withDefaultS3RequestTimeout(ctx)
	defer cancel()
	req, err := b.newSignedRequest(ctx, http.MethodPut, objectKey, contentType, content)
	if err != nil {
		return err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	return fmt.Errorf("put s3 object %q: status=%d body=%s", objectKey, resp.StatusCode, strings.TrimSpace(string(body)))
}

func (b *S3Backend) PutReader(ctx context.Context, objectKey, contentType string, reader io.Reader, size int64) error {
	if size < 0 || reader == nil {
		return ErrSizeMismatch
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	sized := &exactSizeReader{ctx: ctx, reader: reader, remaining: size}
	req, err := b.newSignedReaderRequest(ctx, http.MethodPut, objectKey, contentType, sized, size, "UNSIGNED-PAYLOAD")
	if err != nil {
		return err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if sized.remaining != 0 {
			return fmt.Errorf("%w: %v", ErrSizeMismatch, err)
		}
		return err
	}
	defer resp.Body.Close()
	if sized.remaining != 0 {
		_ = resp.Body.Close()
		_ = b.Delete(context.WithoutCancel(ctx), objectKey)
		return ErrSizeMismatch
	}
	var extra [1]byte
	if count, readErr := (contextReader{ctx: ctx, reader: reader}).Read(extra[:]); count != 0 || (readErr != nil && !errors.Is(readErr, io.EOF)) {
		_ = resp.Body.Close()
		_ = b.Delete(context.WithoutCancel(ctx), objectKey)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		return ErrSizeMismatch
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return contextError(ctx)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	return fmt.Errorf("put s3 object %q: status=%d body=%s", objectKey, resp.StatusCode, strings.TrimSpace(string(body)))
}

func (b *S3Backend) Get(ctx context.Context, objectKey string) ([]byte, error) {
	ctx, cancel := withDefaultS3RequestTimeout(ctx)
	defer cancel()
	req, err := b.newSignedRequest(ctx, http.MethodGet, objectKey, "", nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return io.ReadAll(resp.Body)
	case http.StatusNotFound:
		return nil, ErrNotFound
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf("get s3 object %q: status=%d body=%s", objectKey, resp.StatusCode, strings.TrimSpace(string(body)))
	}
}

func (b *S3Backend) GetBounded(ctx context.Context, objectKey string, maxBytes int64) ([]byte, error) {
	if err := validateBoundedReadLimit(maxBytes); err != nil {
		return nil, err
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	ctx, cancel := withDefaultS3RequestTimeout(ctx)
	defer cancel()
	req, err := b.newSignedRequest(ctx, http.MethodGet, objectKey, "", nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	if err := contextError(ctx); err != nil {
		_ = resp.Body.Close()
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return readBoundedAndClose(ctx, resp.Body, maxBytes)
	case http.StatusNotFound:
		_ = resp.Body.Close()
		return nil, ErrNotFound
	default:
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf("get s3 object %q: status=%d body=%s", objectKey, resp.StatusCode, strings.TrimSpace(string(body)))
	}
}

func (b *S3Backend) OpenReader(ctx context.Context, objectKey string, maxBytes int64) (io.ReadCloser, int64, error) {
	if err := validateBoundedReadLimit(maxBytes); err != nil {
		return nil, 0, err
	}
	if err := contextError(ctx); err != nil {
		return nil, 0, err
	}
	req, err := b.newSignedRequest(ctx, http.MethodGet, objectKey, "", nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		if resp.ContentLength > maxBytes {
			_ = resp.Body.Close()
			return nil, 0, ErrObjectTooLarge
		}
		return newBoundedStreamReader(ctx, resp.Body, maxBytes), resp.ContentLength, nil
	case http.StatusNotFound:
		_ = resp.Body.Close()
		return nil, 0, ErrNotFound
	default:
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return nil, 0, fmt.Errorf("get s3 object %q: status=%d body=%s", objectKey, resp.StatusCode, strings.TrimSpace(string(body)))
	}
}

func (b *S3Backend) Copy(ctx context.Context, sourceKey, destinationKey string) error {
	ctx, cancel := withDefaultS3RequestTimeout(ctx)
	defer cancel()
	sourceKey = b.normalizeKey(sourceKey)
	if sourceKey == "" {
		return ErrNotFound
	}
	copySource := "/" + b.bucket + "/" + escapePath(sourceKey)
	req, err := b.newSignedRequestWithCopySource(ctx, destinationKey, copySource)
	if err != nil {
		return err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("copy s3 object: %w", err)
	}
	defer resp.Body.Close()

	const maximumCopyResponseBytes = int64(64 << 10)
	body, err := io.ReadAll(io.LimitReader(resp.Body, maximumCopyResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read s3 copy response: %w", err)
	}
	if int64(len(body)) > maximumCopyResponseBytes {
		return errors.New("s3 copy response exceeds maximum size")
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("copy s3 object %q to %q: status=%d body=%s", sourceKey, destinationKey, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := validateS3CopyResponse(body); err != nil {
		return err
	}
	return contextError(ctx)
}

func validateS3CopyResponse(body []byte) error {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return nil
	}
	var response struct {
		XMLName xml.Name
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	}
	if err := xml.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("decode s3 copy response: %w", err)
	}
	switch response.XMLName.Local {
	case "CopyObjectResult":
		return nil
	case "Error":
		if response.Code == "NoSuchKey" || response.Code == "NoSuchBucket" {
			return ErrNotFound
		}
		return fmt.Errorf("s3 copy failed: code=%s message=%s", response.Code, response.Message)
	default:
		return fmt.Errorf("unexpected s3 copy response %q", response.XMLName.Local)
	}
}

func validateBoundedReadLimit(maxBytes int64) error {
	if maxBytes < 0 || maxBytes == math.MaxInt64 {
		return errInvalidBoundedReadLimit
	}
	return nil
}

func readBounded(ctx context.Context, reader io.Reader, maxBytes int64) ([]byte, error) {
	limited := &io.LimitedReader{R: contextReader{ctx: ctx, reader: reader}, N: maxBytes + 1}
	content, err := io.ReadAll(limited)
	if err != nil {
		clear(content)
		return nil, err
	}
	if int64(len(content)) > maxBytes {
		clear(content)
		return nil, ErrObjectTooLarge
	}
	if err := contextError(ctx); err != nil {
		clear(content)
		return nil, err
	}
	return content, nil
}

func readBoundedAndClose(ctx context.Context, reader io.ReadCloser, maxBytes int64) (content []byte, resultErr error) {
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			clear(content)
			content = nil
			resultErr = errors.Join(resultErr, closeErr)
		}
	}()
	return readBounded(ctx, reader, maxBytes)
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

type exactSizeReader struct {
	ctx       context.Context
	reader    io.Reader
	remaining int64
}

func (reader *exactSizeReader) Read(buffer []byte) (int, error) {
	if err := contextError(reader.ctx); err != nil {
		return 0, err
	}
	if reader.remaining == 0 {
		return 0, io.EOF
	}
	if int64(len(buffer)) > reader.remaining {
		buffer = buffer[:reader.remaining]
	}
	count, err := reader.reader.Read(buffer)
	reader.remaining -= int64(count)
	if errors.Is(err, io.EOF) && reader.remaining != 0 {
		return count, ErrSizeMismatch
	}
	return count, err
}

type boundedStreamReader struct {
	ctx       context.Context
	reader    io.ReadCloser
	remaining int64
	closed    bool
}

func newBoundedStreamReader(ctx context.Context, reader io.ReadCloser, maxBytes int64) io.ReadCloser {
	return &boundedStreamReader{ctx: ctx, reader: reader, remaining: maxBytes}
}

func (reader *boundedStreamReader) Read(buffer []byte) (int, error) {
	if err := contextError(reader.ctx); err != nil {
		return 0, err
	}
	if reader.remaining > 0 {
		if int64(len(buffer)) > reader.remaining {
			buffer = buffer[:reader.remaining]
		}
		count, err := reader.reader.Read(buffer)
		reader.remaining -= int64(count)
		return count, err
	}
	var probe [1]byte
	count, err := reader.reader.Read(probe[:])
	if count > 0 {
		return 0, ErrObjectTooLarge
	}
	return 0, err
}

func (reader *boundedStreamReader) Close() error {
	if reader.closed {
		return nil
	}
	reader.closed = true
	return reader.reader.Close()
}

func (reader contextReader) Read(buffer []byte) (int, error) {
	if err := contextError(reader.ctx); err != nil {
		return 0, err
	}
	count, err := reader.reader.Read(buffer)
	if contextErr := contextError(reader.ctx); contextErr != nil {
		return count, contextErr
	}
	return count, err
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func withDefaultS3RequestTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, defaultS3RequestTimeout)
}

func (b *S3Backend) Delete(ctx context.Context, objectKey string) error {
	ctx, cancel := withDefaultS3RequestTimeout(ctx)
	defer cancel()
	req, err := b.newSignedRequest(ctx, http.MethodDelete, objectKey, "", nil)
	if err != nil {
		return err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	return fmt.Errorf("delete s3 object %q: status=%d body=%s", objectKey, resp.StatusCode, strings.TrimSpace(string(body)))
}

func (b *S3Backend) ListObjects(ctx context.Context, prefix, cursor string, limit int) (ObjectPage, error) {
	ctx, cancel := withDefaultS3RequestTimeout(ctx)
	defer cancel()
	prefix, err := normalizeListPrefix(prefix)
	if err != nil {
		return ObjectPage{}, err
	}
	limit = normalizeObjectListLimit(limit)
	physicalPrefix := prefix
	if b.prefix != "" {
		physicalPrefix = strings.TrimSuffix(path.Join(b.prefix, prefix), "/") + "/"
	}
	query := url.Values{
		"list-type": {"2"},
		"prefix":    {physicalPrefix},
		"max-keys":  {strconv.Itoa(limit)},
	}
	if cursor = strings.TrimSpace(cursor); cursor != "" {
		query.Set("continuation-token", cursor)
	}
	req, err := b.newSignedBucketRequest(ctx, query)
	if err != nil {
		return ObjectPage{}, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return ObjectPage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return ObjectPage{}, fmt.Errorf("list s3 objects: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		IsTruncated           bool   `xml:"IsTruncated"`
		NextContinuationToken string `xml:"NextContinuationToken"`
		Contents              []struct {
			Key          string `xml:"Key"`
			LastModified string `xml:"LastModified"`
		} `xml:"Contents"`
	}
	decoder := xml.NewDecoder(io.LimitReader(resp.Body, 2<<20))
	if err := decoder.Decode(&payload); err != nil {
		return ObjectPage{}, fmt.Errorf("decode s3 object listing: %w", err)
	}
	page := ObjectPage{Objects: make([]ObjectInfo, 0, len(payload.Contents))}
	for _, item := range payload.Contents {
		logicalKey := strings.TrimSpace(item.Key)
		if b.prefix != "" {
			configuredPrefix := b.prefix + "/"
			if !strings.HasPrefix(logicalKey, configuredPrefix) {
				continue
			}
			logicalKey = strings.TrimPrefix(logicalKey, configuredPrefix)
		}
		if !strings.HasPrefix(logicalKey, prefix) {
			continue
		}
		modifiedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(item.LastModified))
		if err != nil {
			continue
		}
		page.Objects = append(page.Objects, ObjectInfo{ObjectKey: logicalKey, ModifiedAt: modifiedAt.UTC()})
	}
	if payload.IsTruncated {
		page.NextCursor = strings.TrimSpace(payload.NextContinuationToken)
	}
	return page, nil
}

func (b *S3Backend) newSignedBucketRequest(ctx context.Context, query url.Values) (*http.Request, error) {
	clone := *b.endpoint
	host, canonicalURI, requestPath := clone.Host, "/"+b.bucket, "/"+b.bucket
	if !b.usePathStyle() {
		host = b.bucket + "." + clone.Host
		clone.Host = host
		canonicalURI, requestPath = "/", "/"
	}
	clone.Path, clone.RawPath = requestPath, canonicalURI
	clone.RawQuery, clone.Fragment = awsCanonicalQuery(query), ""
	payloadHash := sha256Hex(nil)
	now := b.nowUTC()
	amzDate, dateStamp := now.Format("20060102T150405Z"), now.Format("20060102")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, clone.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Host = host
	req.Header.Set("Host", host)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	req.Header.Set("X-Amz-Date", amzDate)
	canonicalHeaders := "host:" + host + "\n" + "x-amz-content-sha256:" + payloadHash + "\n" + "x-amz-date:" + amzDate + "\n"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := strings.Join([]string{http.MethodGet, canonicalURI, awsCanonicalQuery(query), canonicalHeaders, signedHeaders, payloadHash}, "\n")
	credentialScope := dateStamp + "/" + b.region + "/s3/aws4_request"
	stringToSign := strings.Join([]string{"AWS4-HMAC-SHA256", amzDate, credentialScope, sha256Hex([]byte(canonicalRequest))}, "\n")
	signature := hex.EncodeToString(hmacSHA256(b.signingKey(dateStamp), stringToSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+b.accessKeyID+"/"+credentialScope+", SignedHeaders="+signedHeaders+", Signature="+signature)
	return req, nil
}

func normalizeListPrefix(prefix string) (string, error) {
	prefix = strings.TrimSpace(prefix)
	clean := path.Clean(prefix)
	if prefix == "" || clean == "." || clean == "/" || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", fmt.Errorf("invalid object listing prefix %q", prefix)
	}
	return strings.TrimSuffix(clean, "/") + "/", nil
}

func normalizeObjectListLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}

func (b *S3Backend) newSignedRequest(ctx context.Context, method string, objectKey string, contentType string, content []byte) (*http.Request, error) {
	return b.newSignedRequestWithHeaders(ctx, method, objectKey, contentType, content, nil)
}

func (b *S3Backend) newSignedReaderRequest(ctx context.Context, method, objectKey, contentType string, reader io.Reader, size int64, payloadHash string) (*http.Request, error) {
	return b.newSignedRequestWithReaderAndHeaders(ctx, method, objectKey, contentType, reader, size, payloadHash, nil)
}

func (b *S3Backend) newSignedRequestWithCopySource(ctx context.Context, destinationKey, copySource string) (*http.Request, error) {
	return b.newSignedRequestWithHeaders(ctx, http.MethodPut, destinationKey, "", nil, map[string]string{
		"X-Amz-Copy-Source": copySource,
	})
}

func (b *S3Backend) newSignedRequestWithHeaders(ctx context.Context, method string, objectKey string, contentType string, content []byte, extraHeaders map[string]string) (*http.Request, error) {
	return b.newSignedRequestWithReaderAndHeaders(ctx, method, objectKey, contentType, bytes.NewReader(content), int64(len(content)), sha256Hex(content), extraHeaders)
}

func (b *S3Backend) newSignedRequestWithReaderAndHeaders(ctx context.Context, method string, objectKey string, contentType string, reader io.Reader, size int64, payloadHash string, extraHeaders map[string]string) (*http.Request, error) {
	key := b.normalizeKey(objectKey)
	if key == "" {
		return nil, fmt.Errorf("invalid s3 object key %q", objectKey)
	}
	requestURL, host, canonicalURI, err := b.requestTarget(key)
	if err != nil {
		return nil, err
	}
	now := b.nowUTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reader)
	if err != nil {
		return nil, err
	}
	req.Host = host
	req.ContentLength = size
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Host", host)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	req.Header.Set("X-Amz-Date", amzDate)
	for name, value := range extraHeaders {
		req.Header.Set(name, strings.TrimSpace(value))
	}
	canonicalHeaders := "host:" + host + "\n" + "x-amz-content-sha256:" + payloadHash + "\n"
	signedHeaders := "host;x-amz-content-sha256"
	if copySource := req.Header.Get("X-Amz-Copy-Source"); copySource != "" {
		canonicalHeaders += "x-amz-copy-source:" + copySource + "\n"
		signedHeaders += ";x-amz-copy-source"
	}
	canonicalHeaders += "x-amz-date:" + amzDate + "\n"
	signedHeaders += ";x-amz-date"
	canonicalRequest := strings.Join([]string{
		method,
		canonicalURI,
		"",
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")
	credentialScope := dateStamp + "/" + b.region + "/s3/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
	signature := hex.EncodeToString(hmacSHA256(b.signingKey(dateStamp), stringToSign))
	authorization := "AWS4-HMAC-SHA256 Credential=" + b.accessKeyID + "/" + credentialScope + ", SignedHeaders=" + signedHeaders + ", Signature=" + signature
	req.Header.Set("Authorization", authorization)
	return req, nil
}

func (b *S3Backend) requestTarget(key string) (string, string, string, error) {
	clone := *b.endpoint
	host := clone.Host
	canonicalURI := "/" + b.bucket + "/" + escapePath(key)
	requestPath := "/" + b.bucket + "/" + key
	if !b.usePathStyle() {
		host = b.bucket + "." + clone.Host
		clone.Host = host
		canonicalURI = "/" + escapePath(key)
		requestPath = "/" + key
	}
	clone.Path = requestPath
	clone.RawPath = canonicalURI
	clone.RawQuery = ""
	clone.Fragment = ""
	return clone.String(), host, canonicalURI, nil
}

func (b *S3Backend) usePathStyle() bool {
	if b.forcePathStyle {
		return true
	}
	host := b.endpoint.Hostname()
	if host == "" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return true
	}
	host = strings.ToLower(host)
	return host == "localhost" || strings.HasSuffix(host, ".localhost")
}

func (b *S3Backend) normalizeKey(objectKey string) string {
	cleanKey := path.Clean(strings.TrimSpace(objectKey))
	cleanKey = strings.TrimPrefix(cleanKey, "./")
	cleanKey = strings.TrimPrefix(cleanKey, "/")
	if cleanKey == "." || cleanKey == "" || strings.HasPrefix(cleanKey, "../") {
		return ""
	}
	if b.prefix == "" {
		return cleanKey
	}
	return path.Join(b.prefix, cleanKey)
}

func (b *S3Backend) signingKey(dateStamp string) []byte {
	dateKey := hmacSHA256([]byte("AWS4"+b.secretAccessKey), dateStamp)
	regionKey := hmacSHA256(dateKey, b.region)
	serviceKey := hmacSHA256(regionKey, "s3")
	return hmacSHA256(serviceKey, "aws4_request")
}

func hmacSHA256(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func escapePath(key string) string {
	parts := strings.Split(strings.Trim(key, "/"), "/")
	for idx, part := range parts {
		parts[idx] = strings.ReplaceAll(url.QueryEscape(part), "+", "%20")
	}
	return strings.Join(parts, "/")
}
