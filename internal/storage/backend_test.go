package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
)

func TestLocalBackendRejectsTraversal(t *testing.T) {
	backend := NewLocalBackend(t.TempDir())
	if err := backend.Put(context.Background(), "../escape.txt", "text/plain", []byte("nope")); err == nil {
		t.Fatal("expected traversal put to fail")
	}
	if _, err := backend.Get(context.Background(), "../escape.txt"); err != ErrNotFound {
		t.Fatalf("expected traversal get to behave like not found, got %v", err)
	}
}

func TestTemporaryMediaURLProjectionSignsPreviewAndDownloadSeparately(t *testing.T) {
	backend := &recordingTemporaryURLBackend{}
	urls, supported, err := ProjectTemporaryMediaURLs(t.Context(), backend, "generated/result.png", "image/png", "result.png")
	if err != nil {
		t.Fatalf("ProjectTemporaryMediaURLs: %v", err)
	}
	if !supported || urls.PreviewURL != "https://objects.example.test/generated/result.png?mode=preview&sig=secret" || urls.DownloadURL != "https://objects.example.test/generated/result.png?mode=download&sig=secret" {
		t.Fatalf("unexpected projected URLs %#v supported=%v", urls, supported)
	}
	if len(backend.options) != 2 ||
		backend.options[0].Expiry != 6*time.Minute ||
		backend.options[0].SigningTimeBucket != time.Minute ||
		backend.options[0].ResponseCacheControl != "private, max-age=300" ||
		backend.options[0].ResponseFilename != "" ||
		backend.options[1].Expiry != 5*time.Minute ||
		backend.options[1].SigningTimeBucket != 0 ||
		backend.options[1].ResponseCacheControl != "" ||
		backend.options[1].ResponseFilename != "result.png" {
		t.Fatalf("unexpected signing options %#v", backend.options)
	}
	if _, supported, err := ProjectTemporaryMediaURLs(t.Context(), NewLocalBackend(t.TempDir()), "generated/result.png", "image/png", "result.png"); err != nil || supported {
		t.Fatalf("local backend must deliberately use fallback URLs: supported=%v err=%v", supported, err)
	}
	invalid := &recordingTemporaryURLBackend{previewURL: "https://user:secret@objects.example.test/result.png?X-Amz-Signature=do-not-log"}
	if _, _, err := ProjectTemporaryMediaURLs(t.Context(), invalid, "generated/result.png", "image/png", "result.png"); err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "X-Amz-Signature") {
		t.Fatalf("invalid signed URL must fail without leaking credentials or query values: %v", err)
	}
}

type recordingTemporaryURLBackend struct {
	options    []TemporaryGetURLOptions
	previewURL string
}

func (b *recordingTemporaryURLBackend) Driver() string                                    { return "s3" }
func (b *recordingTemporaryURLBackend) Put(context.Context, string, string, []byte) error { return nil }
func (b *recordingTemporaryURLBackend) Get(context.Context, string) ([]byte, error)       { return nil, nil }
func (b *recordingTemporaryURLBackend) Delete(context.Context, string) error              { return nil }
func (b *recordingTemporaryURLBackend) TemporaryGetURL(_ context.Context, objectKey string, options TemporaryGetURLOptions) (string, error) {
	b.options = append(b.options, options)
	mode := "preview"
	if options.ResponseFilename != "" {
		mode = "download"
	}
	if mode == "preview" && b.previewURL != "" {
		return b.previewURL, nil
	}
	return "https://objects.example.test/" + objectKey + "?mode=" + mode + "&sig=secret", nil
}

func TestLocalBackendGetBoundedHonorsLimitAndContext(t *testing.T) {
	backend := NewLocalBackend(t.TempDir())
	content := []byte("probe-content")
	if err := backend.Put(t.Context(), "probe-object", "application/octet-stream", content); err != nil {
		t.Fatalf("Put: %v", err)
	}
	loaded, err := backend.GetBounded(t.Context(), "probe-object", int64(len(content)))
	if err != nil || string(loaded) != string(content) {
		t.Fatalf("bounded local read: content=%q err=%v", loaded, err)
	}
	if loaded, err := backend.GetBounded(t.Context(), "probe-object", int64(len(content)-1)); !errors.Is(err, ErrObjectTooLarge) || loaded != nil {
		t.Fatalf("oversized local read: bytes=%d err=%v", len(loaded), err)
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := backend.GetBounded(cancelled, "probe-object", int64(len(content))); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled local read error=%v", err)
	}
}

func TestLocalBackendCopyCreatesIndependentObject(t *testing.T) {
	backend := NewLocalBackend(t.TempDir())
	copier, ok := any(backend).(interface {
		Copy(context.Context, string, string) error
	})
	if !ok {
		t.Fatal("local backend does not expose server-side copy capability")
	}
	source := []byte("independent-image-content")
	if err := backend.Put(t.Context(), "source/image.png", "image/png", source); err != nil {
		t.Fatalf("Put source: %v", err)
	}
	if err := copier.Copy(t.Context(), "source/image.png", "references/copied.png"); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if err := backend.Delete(t.Context(), "source/image.png"); err != nil {
		t.Fatalf("Delete source: %v", err)
	}
	loaded, err := backend.Get(t.Context(), "references/copied.png")
	if err != nil || string(loaded) != string(source) {
		t.Fatalf("copied object after source deletion: content=%q err=%v", loaded, err)
	}
}

func TestLocalBackendCopyRejectsInvalidPathsAndCancellation(t *testing.T) {
	backend := NewLocalBackend(t.TempDir())
	copier, ok := any(backend).(interface {
		Copy(context.Context, string, string) error
	})
	if !ok {
		t.Fatal("local backend does not expose server-side copy capability")
	}
	if err := backend.Put(t.Context(), "source.png", "image/png", []byte("content")); err != nil {
		t.Fatalf("Put source: %v", err)
	}
	if err := copier.Copy(t.Context(), "source.png", "../outside.png"); err == nil {
		t.Fatal("copy accepted traversal destination")
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := copier.Copy(cancelled, "source.png", "cancelled.png"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled copy error = %v, want context.Canceled", err)
	}
	if _, err := backend.Get(t.Context(), "cancelled.png"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cancelled copy left destination behind: %v", err)
	}
}

func TestS3BackendRoundTrip(t *testing.T) {
	store := map[string][]byte{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "AWS4-HMAC-SHA256 ") {
			t.Fatalf("expected sigv4 authorization header, got %q", got)
		}
		if got := r.Header.Get("X-Amz-Date"); got == "" {
			t.Fatal("expected x-amz-date header")
		}
		if got := r.Header.Get("X-Amz-Content-Sha256"); got == "" {
			t.Fatal("expected x-amz-content-sha256 header")
		}
		if r.URL.Path != "/bucket/prefix/generated-images/result.png" {
			t.Fatalf("unexpected object path %q", r.URL.Path)
		}
		switch r.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			store[r.URL.Path] = body
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			body, ok := store[r.URL.Path]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		case http.MethodDelete:
			delete(store, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	backend, err := NewS3Backend(config.StorageConfig{
		Driver: "s3",
		S3: config.StorageS3Config{
			Endpoint:        server.URL,
			Region:          "us-east-1",
			Bucket:          "bucket",
			AccessKeyID:     "access",
			SecretAccessKey: "secret",
			ForcePathStyle:  true,
			Prefix:          "prefix",
		},
	})
	if err != nil {
		t.Fatalf("NewS3Backend: %v", err)
	}
	content := []byte("image-bytes")
	if err := backend.Put(context.Background(), "generated-images/result.png", "image/png", content); err != nil {
		t.Fatalf("Put: %v", err)
	}
	loaded, err := backend.Get(context.Background(), "generated-images/result.png")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(loaded) != string(content) {
		t.Fatalf("unexpected content %q", string(loaded))
	}
	if err := backend.Delete(context.Background(), "generated-images/result.png"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := backend.Get(context.Background(), "generated-images/result.png"); err != ErrNotFound {
		t.Fatalf("expected deleted object to be gone, got %v", err)
	}
}

func TestS3BackendTemporaryGetURLUsesBoundedSigV4QueryWithoutNetwork(t *testing.T) {
	backend, err := NewS3Backend(config.StorageConfig{
		Driver: "s3",
		S3: config.StorageS3Config{
			Endpoint: "https://s3.example.com", Region: "us-east-1", Bucket: "bucket",
			AccessKeyID: "AKIDEXAMPLE", SecretAccessKey: "secret-example", ForcePathStyle: true, Prefix: "prefix",
		},
	})
	if err != nil {
		t.Fatalf("NewS3Backend: %v", err)
	}
	backend.now = func() time.Time { return time.Date(2026, time.August, 3, 12, 34, 56, 0, time.UTC) }
	transport := &recordingRoundTripper{}
	backend.client = &http.Client{Transport: transport}

	options := TemporaryGetURLOptions{
		Expiry:           10 * 24 * time.Hour,
		ResponseFilename: "Mikiko result 01.png",
		ContentType:      "image/png",
	}
	first, err := backend.TemporaryGetURL(t.Context(), "generated images/result + one.png", options)
	if err != nil {
		t.Fatalf("TemporaryGetURL: %v", err)
	}
	second, err := backend.TemporaryGetURL(t.Context(), "generated images/result + one.png", options)
	if err != nil {
		t.Fatalf("second TemporaryGetURL: %v", err)
	}
	if first != second {
		t.Fatalf("fixed-time presign must be deterministic:\nfirst:  %s\nsecond: %s", first, second)
	}
	if transport.calls != 0 {
		t.Fatalf("TemporaryGetURL performed %d network calls", transport.calls)
	}

	parsed, err := url.Parse(first)
	if err != nil {
		t.Fatalf("parse presigned URL: %v", err)
	}
	if parsed.Path != "/bucket/prefix/generated images/result + one.png" {
		t.Fatalf("presigned path = %q", parsed.Path)
	}
	query := parsed.Query()
	for key, want := range map[string]string{
		"X-Amz-Algorithm":       "AWS4-HMAC-SHA256",
		"X-Amz-Credential":      "AKIDEXAMPLE/20260803/us-east-1/s3/aws4_request",
		"X-Amz-Date":            "20260803T123456Z",
		"X-Amz-Expires":         "604800",
		"X-Amz-SignedHeaders":   "host",
		"response-content-type": "image/png",
	} {
		if got := query.Get(key); got != want {
			t.Fatalf("%s = %q, want %q; URL=%s", key, got, want, first)
		}
	}
	if disposition := query.Get("response-content-disposition"); !strings.Contains(disposition, "attachment") || !strings.Contains(disposition, "Mikiko result 01.png") {
		t.Fatalf("unexpected response content disposition %q", disposition)
	}
	if signature := query.Get("X-Amz-Signature"); len(signature) != 64 {
		t.Fatalf("signature = %q, want 64 lowercase hexadecimal characters", signature)
	}
	if strings.Contains(first, "access_token") || strings.Contains(first, "secret-example") {
		t.Fatalf("presigned URL leaked application token or storage secret: %s", first)
	}
}

func TestS3BackendCopyUsesSignedCopyObjectRequest(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPut || r.URL.Path != "/bucket/prefix/references/copied image.png" {
			t.Fatalf("copy request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-Amz-Copy-Source"); got != "/bucket/prefix/generated/source%20image.png" {
			t.Fatalf("x-amz-copy-source = %q", got)
		}
		authorization := r.Header.Get("Authorization")
		if !strings.Contains(authorization, "SignedHeaders=host;x-amz-content-sha256;x-amz-copy-source;x-amz-date") {
			t.Fatalf("copy source missing from signed headers: %q", authorization)
		}
		if got := r.Header.Get("X-Amz-Content-Sha256"); got != sha256Hex(nil) {
			t.Fatalf("copy payload hash = %q, want empty payload hash", got)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = io.WriteString(w, `<CopyObjectResult><ETag>"copied-etag"</ETag></CopyObjectResult>`)
	}))
	defer server.Close()

	backend, err := NewS3Backend(config.StorageConfig{
		Driver: "s3",
		S3: config.StorageS3Config{
			Endpoint: server.URL, Region: "us-east-1", Bucket: "bucket",
			AccessKeyID: "access", SecretAccessKey: "secret", ForcePathStyle: true, Prefix: "prefix",
		},
	})
	if err != nil {
		t.Fatalf("NewS3Backend: %v", err)
	}
	copier, ok := any(backend).(interface {
		Copy(context.Context, string, string) error
	})
	if !ok {
		t.Fatal("S3 backend does not expose server-side copy capability")
	}
	if err := copier.Copy(t.Context(), "generated/source image.png", "references/copied image.png"); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if calls != 1 {
		t.Fatalf("copy round trips = %d, want 1", calls)
	}
}

func TestS3BackendCopyRejectsEmbeddedErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = io.WriteString(w, `<Error><Code>NoSuchKey</Code><Message>source missing</Message></Error>`)
	}))
	defer server.Close()

	backend, err := NewS3Backend(config.StorageConfig{
		Driver: "s3",
		S3: config.StorageS3Config{
			Endpoint: server.URL, Region: "us-east-1", Bucket: "bucket",
			AccessKeyID: "access", SecretAccessKey: "secret", ForcePathStyle: true,
		},
	})
	if err != nil {
		t.Fatalf("NewS3Backend: %v", err)
	}
	copier, ok := any(backend).(interface {
		Copy(context.Context, string, string) error
	})
	if !ok {
		t.Fatal("S3 backend does not expose server-side copy capability")
	}
	if err := copier.Copy(t.Context(), "missing.png", "copied.png"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("embedded NoSuchKey error = %v, want ErrNotFound", err)
	}
}

func TestS3BackendTemporaryGetURLDefaultsToFiveMinutesAndLocalDoesNotSign(t *testing.T) {
	backend, err := NewS3Backend(config.StorageConfig{
		Driver: "s3",
		S3: config.StorageS3Config{
			Endpoint: "https://bucket.example.com", Region: "us-east-1", Bucket: "bucket",
			AccessKeyID: "access", SecretAccessKey: "secret", ForcePathStyle: true,
		},
	})
	if err != nil {
		t.Fatalf("NewS3Backend: %v", err)
	}
	backend.now = func() time.Time { return time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC) }
	signedURL, err := backend.TemporaryGetURL(t.Context(), "result.png", TemporaryGetURLOptions{})
	if err != nil {
		t.Fatalf("TemporaryGetURL: %v", err)
	}
	parsed, _ := url.Parse(signedURL)
	if got := parsed.Query().Get("X-Amz-Expires"); got != "300" {
		t.Fatalf("default expiry = %q, want 300", got)
	}

	var backendContract Backend = NewLocalBackend(t.TempDir())
	if _, ok := backendContract.(TemporaryURLSigner); ok {
		t.Fatal("local storage must retain byte delivery instead of exposing a temporary URL")
	}
}

func TestProjectTemporaryMediaURLsBucketsPreviewAndFreshlySignsDownload(t *testing.T) {
	backend, err := NewS3Backend(config.StorageConfig{
		Driver: "s3",
		S3: config.StorageS3Config{
			Endpoint: "https://bucket.example.com", Region: "us-east-1", Bucket: "bucket",
			AccessKeyID: "access", SecretAccessKey: "secret", ForcePathStyle: true,
		},
	})
	if err != nil {
		t.Fatalf("NewS3Backend: %v", err)
	}

	now := time.Date(2026, time.August, 6, 12, 34, 5, 0, time.UTC)
	backend.now = func() time.Time { return now }
	first, supported, err := ProjectTemporaryMediaURLs(t.Context(), backend, "generated/result.png", "image/png", "result.png")
	if err != nil || !supported {
		t.Fatalf("first projection: supported=%v err=%v", supported, err)
	}

	now = time.Date(2026, time.August, 6, 12, 34, 59, 0, time.UTC)
	second, _, err := ProjectTemporaryMediaURLs(t.Context(), backend, "generated/result.png", "image/png", "result.png")
	if err != nil {
		t.Fatalf("second projection: %v", err)
	}
	if first.PreviewURL != second.PreviewURL {
		t.Fatalf("preview URL changed inside signing bucket:\nfirst:  %s\nsecond: %s", first.PreviewURL, second.PreviewURL)
	}
	if first.DownloadURL == second.DownloadURL {
		t.Fatal("download URL must be freshly signed instead of sharing the preview bucket")
	}
	if want := time.Date(2026, time.August, 6, 12, 40, 0, 0, time.UTC); !first.PreviewExpiresAt.Equal(want) {
		t.Fatalf("preview expiry metadata = %s, want %s", first.PreviewExpiresAt, want)
	}
	if want := time.Date(2026, time.August, 6, 12, 39, 59, 0, time.UTC); !second.DownloadExpiresAt.Equal(want) {
		t.Fatalf("download expiry metadata = %s, want %s", second.DownloadExpiresAt, want)
	}

	preview, err := url.Parse(first.PreviewURL)
	if err != nil {
		t.Fatalf("parse preview URL: %v", err)
	}
	previewQuery := preview.Query()
	if got := previewQuery.Get("X-Amz-Date"); got != "20260806T123400Z" {
		t.Fatalf("preview signing time = %q, want bucket start", got)
	}
	if got := previewQuery.Get("X-Amz-Expires"); got != "360" {
		t.Fatalf("preview expiry = %q, want 360 seconds to preserve five-minute validity at bucket end", got)
	}
	if got := previewQuery.Get("response-cache-control"); got != "private, max-age=300" {
		t.Fatalf("preview cache control = %q, want private five-minute browser cache", got)
	}
	if got := previewQuery.Get("response-content-disposition"); got != "" {
		t.Fatalf("preview unexpectedly forces download disposition %q", got)
	}

	download, err := url.Parse(second.DownloadURL)
	if err != nil {
		t.Fatalf("parse download URL: %v", err)
	}
	downloadQuery := download.Query()
	if got := downloadQuery.Get("X-Amz-Date"); got != "20260806T123459Z" {
		t.Fatalf("download signing time = %q, want current request time", got)
	}
	if disposition := downloadQuery.Get("response-content-disposition"); !strings.Contains(disposition, "attachment") || !strings.Contains(disposition, "result.png") {
		t.Fatalf("download disposition = %q, want attachment filename", disposition)
	}
	if got := downloadQuery.Get("response-cache-control"); got != "" {
		t.Fatalf("download unexpectedly reused preview cache control %q", got)
	}

	now = time.Date(2026, time.August, 6, 12, 35, 0, 0, time.UTC)
	third, _, err := ProjectTemporaryMediaURLs(t.Context(), backend, "generated/result.png", "image/png", "result.png")
	if err != nil {
		t.Fatalf("third projection: %v", err)
	}
	if second.PreviewURL == third.PreviewURL {
		t.Fatal("preview URL did not rotate at the next signing bucket")
	}
}

func TestLocalBackendListObjectsPaginatesWithinPrefix(t *testing.T) {
	root := t.TempDir()
	backend := NewLocalBackend(root)
	for _, key := range []string{
		"generated-images/7/b.png",
		"reference-assets/ignored.png",
		"generated-images/7/a.png",
		"generated-images/8/d.png",
		"generated-images/8/c.png",
		"generated-images/a/z.png",
		"generated-images/a.png",
	} {
		if err := backend.Put(t.Context(), key, "image/png", []byte(key)); err != nil {
			t.Fatal(err)
		}
	}
	wantModifiedAt := time.Date(2026, time.August, 9, 1, 2, 3, 0, time.UTC)
	modifiedPath, ok := backend.resolvePath("generated-images/7/a.png")
	if !ok {
		t.Fatal("resolve generated image path")
	}
	if err := os.Chtimes(modifiedPath, wantModifiedAt, wantModifiedAt); err != nil {
		t.Fatalf("set object mtime: %v", err)
	}

	var (
		cursor   string
		objects  []ObjectInfo
		pageRuns int
	)
	for {
		// A fresh backend proves that pagination state lives entirely in the
		// opaque cursor rather than process memory.
		page, err := NewLocalBackend(root).ListObjects(t.Context(), "generated-images/", cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		pageRuns++
		objects = append(objects, page.Objects...)
		if page.NextCursor == "" {
			break
		}
		if strings.Contains(page.NextCursor, "generated-images/") || page.NextCursor == page.Objects[len(page.Objects)-1].ObjectKey {
			t.Fatalf("cursor exposes implementation key %q", page.NextCursor)
		}
		cursor = page.NextCursor
	}
	if pageRuns != 3 {
		t.Fatalf("page runs=%d want 3", pageRuns)
	}
	wantKeys := map[string]struct{}{
		"generated-images/7/a.png": {},
		"generated-images/7/b.png": {},
		"generated-images/8/c.png": {},
		"generated-images/8/d.png": {},
		"generated-images/a.png":   {},
		"generated-images/a/z.png": {},
	}
	if len(objects) != len(wantKeys) {
		t.Fatalf("objects=%#v want keys=%#v", objects, wantKeys)
	}
	seen := make(map[string]struct{}, len(objects))
	foundKnownMtime := false
	for _, object := range objects {
		if _, ok := wantKeys[object.ObjectKey]; !ok {
			t.Fatalf("unexpected object %q", object.ObjectKey)
		}
		if _, duplicate := seen[object.ObjectKey]; duplicate {
			t.Fatalf("duplicate object %q", object.ObjectKey)
		}
		seen[object.ObjectKey] = struct{}{}
		if object.ObjectKey == "generated-images/7/a.png" {
			foundKnownMtime = object.ModifiedAt.Equal(wantModifiedAt)
		}
	}
	if !foundKnownMtime {
		t.Fatal("known object mtime was not preserved")
	}
}

func TestLocalBackendListObjectsBoundsIncrementalCandidatesPerPage(t *testing.T) {
	backend := NewLocalBackend(t.TempDir())
	for index := 99; index >= 0; index-- {
		key := fmt.Sprintf("generated-images/%03d/result.png", index)
		if err := backend.Put(t.Context(), key, "image/png", []byte(key)); err != nil {
			t.Fatal(err)
		}
	}

	first, firstStats, err := backend.listObjectsIncrementally(t.Context(), "generated-images/", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Objects) != 2 || first.NextCursor == "" {
		t.Fatalf("first page=%#v", first)
	}
	if firstStats.VisitedObjects != 3 || firstStats.MaterializedObjects != 3 || firstStats.DirectoryEntriesRead > 32 {
		t.Fatalf("first page scanned beyond limit+1: %#v", firstStats)
	}

	second, secondStats, err := backend.listObjectsIncrementally(t.Context(), "generated-images/", first.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Objects) != 2 || second.Objects[0].ObjectKey == first.Objects[0].ObjectKey || second.Objects[0].ObjectKey == first.Objects[1].ObjectKey ||
		second.Objects[1].ObjectKey == first.Objects[0].ObjectKey || second.Objects[1].ObjectKey == first.Objects[1].ObjectKey {
		t.Fatalf("second page=%#v", second)
	}
	if secondStats.VisitedObjects != 3 || secondStats.MaterializedObjects != 3 || secondStats.DirectoryEntriesRead > 32 {
		t.Fatalf("second page rescanned or materialized beyond limit+1: %#v", secondStats)
	}
}

func TestLocalBackendListObjectsBoundsFlatDirectoryTraversal(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "reference-assets")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	const objectCount = 10_000
	for index := range objectCount {
		name := fmt.Sprintf("asset-%05d.png", index)
		if err := os.WriteFile(filepath.Join(directory, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wantModifiedAt := time.Date(2026, time.August, 9, 2, 3, 4, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(directory, "asset-05000.png"), wantModifiedAt, wantModifiedAt); err != nil {
		t.Fatal(err)
	}

	backend := NewLocalBackend(root)
	seen := make(map[string]struct{}, objectCount)
	cursor := ""
	foundKnownMtime := false
	for pageNumber := 0; ; pageNumber++ {
		page, stats, err := backend.listObjectsIncrementally(t.Context(), "reference-assets/", cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		if pageNumber < 2 && (stats.DirectoryEntriesRead > 32 || stats.MaterializedObjects > 3) {
			t.Fatalf("page %d read beyond limit+1: %#v", pageNumber, stats)
		}
		for _, object := range page.Objects {
			if _, duplicate := seen[object.ObjectKey]; duplicate {
				t.Fatalf("duplicate object %q", object.ObjectKey)
			}
			seen[object.ObjectKey] = struct{}{}
			if object.ObjectKey == "reference-assets/asset-05000.png" {
				foundKnownMtime = object.ModifiedAt.Equal(wantModifiedAt)
			}
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if len(seen) != objectCount {
		t.Fatalf("listed %d objects, want %d", len(seen), objectCount)
	}
	if !foundKnownMtime {
		t.Fatal("known object mtime was not preserved")
	}
}

func TestLocalBackendListObjectsRejectsCursorOutsideOwnedPrefix(t *testing.T) {
	backend := NewLocalBackend(t.TempDir())
	forged, err := encodeLocalObjectCursor("generated-images/", []localObjectCursorFrame{{Directory: "generated-images/../reference-assets", Offset: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.ListObjects(t.Context(), "generated-images/", forged, 2); err == nil {
		t.Fatal("expected non-canonical cursor key to be rejected")
	}

	if err := backend.Put(t.Context(), "generated-images/a.png", "image/png", []byte("a")); err != nil {
		t.Fatal(err)
	}
	if err := backend.Put(t.Context(), "generated-images/b.png", "image/png", []byte("b")); err != nil {
		t.Fatal(err)
	}
	page, err := backend.ListObjects(t.Context(), "generated-images/", "", 1)
	if err != nil || page.NextCursor == "" {
		t.Fatalf("first page=%#v err=%v", page, err)
	}
	if _, err := backend.ListObjects(t.Context(), "reference-assets/", page.NextCursor, 1); err == nil {
		t.Fatal("expected cursor bound to another owned prefix to be rejected")
	}
}

func TestS3BackendListObjectsV2UsesConfiguredAndOwnedPrefix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/bucket" {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("list-type") != "2" || query.Get("prefix") != "tenant-root/generated-images/" || query.Get("continuation-token") != "token-a" || query.Get("max-keys") != "2" {
			t.Fatalf("query=%v", query)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = io.WriteString(w, `<ListBucketResult><IsTruncated>true</IsTruncated><Contents><Key>tenant-root/generated-images/7/a.png</Key><LastModified>2026-08-09T01:00:00Z</LastModified></Contents><NextContinuationToken>token-b</NextContinuationToken></ListBucketResult>`)
	}))
	defer server.Close()
	backend, err := NewS3Backend(config.StorageConfig{
		Driver: "s3",
		S3: config.StorageS3Config{
			Endpoint: server.URL, Region: "us-east-1", Bucket: "bucket", Prefix: "tenant-root",
			ForcePathStyle: true, AccessKeyID: "access", SecretAccessKey: "secret",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err := backend.ListObjects(t.Context(), "generated-images/", "token-a", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Objects) != 1 || page.Objects[0].ObjectKey != "generated-images/7/a.png" || !page.Objects[0].ModifiedAt.Equal(time.Date(2026, 8, 9, 1, 0, 0, 0, time.UTC)) || page.NextCursor != "token-b" {
		t.Fatalf("page=%#v", page)
	}
}

func TestS3BackendGetBoundedStopsAfterLimitAndClosesBody(t *testing.T) {
	body := &countingReadCloser{reader: strings.NewReader(strings.Repeat("x", 1<<20))}
	backend, err := NewS3Backend(config.StorageConfig{
		Driver: "s3",
		S3: config.StorageS3Config{
			Endpoint: "http://s3.invalid", Region: "us-east-1", Bucket: "bucket",
			AccessKeyID: "access", SecretAccessKey: "secret", ForcePathStyle: true,
		},
	})
	if err != nil {
		t.Fatalf("NewS3Backend: %v", err)
	}
	transport := &recordingRoundTripper{response: &http.Response{
		StatusCode: http.StatusOK,
		Body:       body,
		Header:     make(http.Header),
	}}
	backend.client = &http.Client{Transport: transport}

	const maximum = int64(16)
	loaded, err := backend.GetBounded(t.Context(), "probe-object", maximum)
	if !errors.Is(err, ErrObjectTooLarge) || loaded != nil {
		t.Fatalf("GetBounded oversized result: bytes=%d err=%v", len(loaded), err)
	}
	if body.bytesRead > maximum+1 {
		t.Fatalf("GetBounded consumed %d bytes, want at most %d", body.bytesRead, maximum+1)
	}
	if !body.closed {
		t.Fatal("GetBounded did not close the response body")
	}
	if transport.calls != 1 {
		t.Fatalf("round trip calls=%d want 1", transport.calls)
	}
}

func TestBoundedGetRejectsOverflowBeforeIO(t *testing.T) {
	local := NewLocalBackend(t.TempDir())
	if _, err := local.GetBounded(t.Context(), "object", math.MaxInt64); err == nil || errors.Is(err, ErrObjectTooLarge) {
		t.Fatalf("local overflow limit error=%v", err)
	}

	backend, err := NewS3Backend(config.StorageConfig{
		Driver: "s3",
		S3: config.StorageS3Config{
			Endpoint: "http://s3.invalid", Region: "us-east-1", Bucket: "bucket",
			AccessKeyID: "access", SecretAccessKey: "secret", ForcePathStyle: true,
		},
	})
	if err != nil {
		t.Fatalf("NewS3Backend: %v", err)
	}
	transport := &recordingRoundTripper{}
	backend.client = &http.Client{Transport: transport}
	if _, err := backend.GetBounded(t.Context(), "object", math.MaxInt64); err == nil || errors.Is(err, ErrObjectTooLarge) {
		t.Fatalf("S3 overflow limit error=%v", err)
	}
	if transport.calls != 0 {
		t.Fatalf("overflow limit performed %d round trips", transport.calls)
	}
}

func TestReadBoundedClearsPartialContentAfterReaderError(t *testing.T) {
	reader := &retainingErrorReader{content: []byte("sensitive-probe-content")}
	loaded, err := readBounded(t.Context(), reader, 64)
	if !errors.Is(err, errRetainingReader) || loaded != nil {
		t.Fatalf("readBounded error result: bytes=%d err=%v", len(loaded), err)
	}
	if len(reader.retained) == 0 {
		t.Fatal("test reader did not retain the destination buffer")
	}
	for index, value := range reader.retained {
		if value != 0 {
			t.Fatalf("partial content byte %d was not cleared", index)
		}
	}
}

func TestReadBoundedAndCloseClearsSuccessfulContentAfterCloseError(t *testing.T) {
	reader := &retainingCloseErrorReader{content: []byte("sensitive-probe-content")}
	loaded, err := readBoundedAndClose(t.Context(), reader, 64)
	if !errors.Is(err, errRetainingClose) || loaded != nil {
		t.Fatalf("readBoundedAndClose result: bytes=%d err=%v", len(loaded), err)
	}
	if !reader.closed {
		t.Fatal("readBoundedAndClose did not close the reader")
	}
	if len(reader.retained) == 0 {
		t.Fatal("test reader did not retain the destination buffer")
	}
	for index, value := range reader.retained {
		if value != 0 {
			t.Fatalf("close-error content byte %d was not cleared", index)
		}
	}
}

type countingReadCloser struct {
	reader    io.Reader
	bytesRead int64
	closed    bool
}

func (reader *countingReadCloser) Read(buffer []byte) (int, error) {
	count, err := reader.reader.Read(buffer)
	reader.bytesRead += int64(count)
	return count, err
}

func (reader *countingReadCloser) Close() error {
	reader.closed = true
	return nil
}

type recordingRoundTripper struct {
	response *http.Response
	err      error
	calls    int
}

var errRetainingReader = errors.New("retaining reader failed")
var errRetainingClose = errors.New("retaining reader close failed")

type retainingErrorReader struct {
	content  []byte
	retained []byte
}

func (reader *retainingErrorReader) Read(buffer []byte) (int, error) {
	count := copy(buffer, reader.content)
	reader.retained = buffer[:count]
	return count, errRetainingReader
}

type retainingCloseErrorReader struct {
	content  []byte
	retained []byte
	closed   bool
	read     bool
}

func (reader *retainingCloseErrorReader) Read(buffer []byte) (int, error) {
	if reader.read {
		return 0, io.EOF
	}
	reader.read = true
	count := copy(buffer, reader.content)
	reader.retained = buffer[:count]
	return count, io.EOF
}

func (reader *retainingCloseErrorReader) Close() error {
	reader.closed = true
	return errRetainingClose
}

func (transport *recordingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	transport.calls++
	return transport.response, transport.err
}
