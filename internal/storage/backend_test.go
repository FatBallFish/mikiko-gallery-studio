package storage

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
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
