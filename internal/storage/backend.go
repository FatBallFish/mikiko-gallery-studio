package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
	errInvalidBoundedReadLimit = errors.New("storage bounded read limit is invalid")
)

type Backend interface {
	Driver() string
	Put(ctx context.Context, objectKey string, contentType string, content []byte) error
	Get(ctx context.Context, objectKey string) ([]byte, error)
	Delete(ctx context.Context, objectKey string) error
}

// BoundedGetter is an optional backend capability for callers that must not
// allocate based on untrusted object sizes. Get remains unchanged for normal
// asset reads.
type BoundedGetter interface {
	GetBounded(ctx context.Context, objectKey string, maxBytes int64) ([]byte, error)
}

type TemporaryGetURLOptions struct {
	Expiry           time.Duration
	ResponseFilename string
	ContentType      string
}

type TemporaryURLSigner interface {
	TemporaryGetURL(ctx context.Context, objectKey string, options TemporaryGetURLOptions) (string, error)
}

var (
	_ BoundedGetter      = (*LocalBackend)(nil)
	_ BoundedGetter      = (*S3Backend)(nil)
	_ TemporaryURLSigner = (*S3Backend)(nil)
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
		client:          &http.Client{Timeout: 30 * time.Second},
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

func (b *S3Backend) Get(ctx context.Context, objectKey string) ([]byte, error) {
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

func (b *S3Backend) Delete(ctx context.Context, objectKey string) error {
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

func (b *S3Backend) newSignedRequest(ctx context.Context, method string, objectKey string, contentType string, content []byte) (*http.Request, error) {
	key := b.normalizeKey(objectKey)
	if key == "" {
		return nil, fmt.Errorf("invalid s3 object key %q", objectKey)
	}
	requestURL, host, canonicalURI, err := b.requestTarget(key)
	if err != nil {
		return nil, err
	}
	payloadHash := sha256Hex(content)
	now := b.nowUTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	req, err := http.NewRequestWithContext(ctx, method, requestURL, bytes.NewReader(content))
	if err != nil {
		return nil, err
	}
	req.Host = host
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Host", host)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	req.Header.Set("X-Amz-Date", amzDate)
	canonicalHeaders := "host:" + host + "\n" + "x-amz-content-sha256:" + payloadHash + "\n" + "x-amz-date:" + amzDate + "\n"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
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
