package storage

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
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

func TestLocalMultipartResumesCompletesAndReplaysWithoutBufferingWholeObject(t *testing.T) {
	root := t.TempDir()
	backend := NewLocalBackend(root)
	multipart, ok := any(backend).(MultipartBackend)
	if !ok {
		t.Fatal("local backend must expose multipart capability")
	}
	partSize := int64(8 << 20)
	first := bytes.Repeat([]byte("a"), int(partSize))
	second := bytes.Repeat([]byte("b"), 257)
	upload, err := multipart.CreateMultipart(t.Context(), MultipartCreateRequest{
		ObjectKey: "media/original/7/session/video.mp4", ContentType: "video/mp4",
		SizeBytes: int64(len(first) + len(second)), PartSize: partSize,
	})
	if err != nil {
		t.Fatal(err)
	}
	if upload.PartCount != 2 || upload.UploadID == "" {
		t.Fatalf("unexpected multipart plan: %#v", upload)
	}

	firstPart, err := multipart.PutMultipartPart(t.Context(), upload, 1, bytes.NewReader(first), int64(len(first)), sha256HexTest(first))
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a process restart: all resumable state must live on disk.
	restarted := NewLocalBackend(root)
	restartedMultipart := any(restarted).(MultipartBackend)
	status, err := restartedMultipart.MultipartStatus(t.Context(), upload)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.CompletedParts) != 1 || status.CompletedParts[0].PartNumber != 1 {
		t.Fatalf("unexpected resumed status: %#v", status)
	}
	secondPart, err := restartedMultipart.PutMultipartPart(t.Context(), upload, 2, bytes.NewReader(second), int64(len(second)), sha256HexTest(second))
	if err != nil {
		t.Fatal(err)
	}
	completed, err := restartedMultipart.CompleteMultipart(t.Context(), upload, []CompletedPart{firstPart, secondPart})
	if err != nil {
		t.Fatal(err)
	}
	if completed.SizeBytes != int64(len(first)+len(second)) || completed.SHA256 == "" {
		t.Fatalf("unexpected completed object: %#v", completed)
	}
	// Completion is idempotent and returns the same persisted result.
	replayed, err := restartedMultipart.CompleteMultipart(t.Context(), upload, []CompletedPart{firstPart, secondPart})
	if err != nil || replayed != completed {
		t.Fatalf("replayed completion=%#v err=%v", replayed, err)
	}
	got, err := os.ReadFile(filepath.Join(root, "media/original/7/session/video.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got[:len(first)], first) || !bytes.Equal(got[len(first):], second) {
		t.Fatal("completed local multipart object content mismatch")
	}
}

func TestLocalMultipartRejectsMissingOrCorruptPartsAndAbortCleansState(t *testing.T) {
	backend := NewLocalBackend(t.TempDir())
	multipart := any(backend).(MultipartBackend)
	upload, err := multipart.CreateMultipart(t.Context(), MultipartCreateRequest{
		ObjectKey: "media/original/8/session/audio.wav", ContentType: "audio/wav", SizeBytes: 10, PartSize: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("12345678")
	part, err := multipart.PutMultipartPart(t.Context(), upload, 1, bytes.NewReader(data), 8, sha256HexTest(data))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := multipart.CompleteMultipart(t.Context(), upload, []CompletedPart{part}); err == nil {
		t.Fatal("expected missing second part to reject completion")
	}
	if _, err := multipart.PutMultipartPart(t.Context(), upload, 2, strings.NewReader("xx"), 2, strings.Repeat("0", 64)); err == nil {
		t.Fatal("expected checksum mismatch")
	}
	if err := multipart.AbortMultipart(t.Context(), upload); err != nil {
		t.Fatal(err)
	}
	if err := multipart.AbortMultipart(t.Context(), upload); err != nil {
		t.Fatalf("abort must be idempotent: %v", err)
	}
	if _, err := multipart.MultipartStatus(t.Context(), upload); err == nil {
		t.Fatal("aborted multipart state must be unavailable")
	}
}

func TestS3MultipartLifecycleAndPartSigning(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.RequestURI())
		if r.Header.Get("Authorization") == "" && r.URL.Query().Get("X-Amz-Signature") == "" {
			t.Errorf("request is not signed: %s", r.URL.String())
		}
		switch {
		case r.Method == http.MethodPost && hasBareQueryKey(r.URL, "uploads"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, `<InitiateMultipartUploadResult><UploadId>provider-upload-1</UploadId></InitiateMultipartUploadResult>`)
		case r.Method == http.MethodPost && r.URL.Query().Get("uploadId") == "provider-upload-1":
			w.Header().Set("ETag", `"object-etag"`)
			_, _ = io.WriteString(w, `<CompleteMultipartUploadResult><ETag>"object-etag"</ETag></CompleteMultipartUploadResult>`)
		case r.Method == http.MethodDelete && r.URL.Query().Get("uploadId") == "provider-upload-1":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	backend, err := NewS3Backend(config.StorageConfig{Driver: "s3", S3: config.StorageS3Config{
		Endpoint: server.URL, Region: "us-east-1", Bucket: "media", AccessKeyID: "test", SecretAccessKey: "secret", ForcePathStyle: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	backend.client = server.Client()
	backend.now = func() time.Time { return time.Date(2026, 8, 12, 3, 4, 5, 0, time.UTC) }
	multipart := any(backend).(MultipartBackend)
	upload, err := multipart.CreateMultipart(t.Context(), MultipartCreateRequest{
		ObjectKey: "media/original/9/session/clip.mp4", ContentType: "video/mp4", SizeBytes: 9 << 20, PartSize: 8 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := multipart.SignMultipartPart(t.Context(), upload, 2, base64.StdEncoding.EncodeToString(make([]byte, sha256.Size)), 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("uploadId") != "provider-upload-1" || parsed.Query().Get("partNumber") != "2" || parsed.Query().Get("X-Amz-Signature") == "" {
		t.Fatalf("invalid signed part target: %s", target.URL)
	}
	if len(target.Headers) != 0 || parsed.Query().Get("X-Amz-SignedHeaders") != "host" || !target.ExpiresAt.After(backend.now()) {
		t.Fatalf("signed part must avoid optional checksum headers and include expiry: %#v", target)
	}
	completed, err := multipart.CompleteMultipart(t.Context(), upload, []CompletedPart{{PartNumber: 1, ETag: "one"}, {PartNumber: 2, ETag: "two"}})
	if err != nil || completed.ETag != "object-etag" {
		t.Fatalf("complete=%#v err=%v", completed, err)
	}
	if err := multipart.AbortMultipart(t.Context(), upload); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 3 {
		t.Fatalf("provider request count=%d requests=%#v", len(requests), requests)
	}
}

func TestS3MultipartProxyStreamsSignedPart(t *testing.T) {
	content := []byte("streamed multipart body")
	checksum := sha256HexTest(content)
	requestSeen := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestSeen = true
		if r.Method != http.MethodPut || r.URL.Query().Get("uploadId") != "proxy-upload" || r.URL.Query().Get("partNumber") != "1" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if r.ContentLength != int64(len(content)) {
			t.Fatalf("content length = %d", r.ContentLength)
		}
		if r.Header.Get("X-Amz-Checksum-Sha256") != "" || r.Header.Get("X-Amz-Content-Sha256") != checksum {
			t.Fatalf("checksum headers = %#v", r.Header)
		}
		if authorization := r.Header.Get("Authorization"); strings.Contains(authorization, "x-amz-checksum-sha256") || !strings.Contains(authorization, "x-amz-content-sha256;x-amz-date") {
			t.Fatalf("proxy upload must sign the payload hash without the optional checksum extension: %s", authorization)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || !bytes.Equal(body, content) {
			t.Fatalf("body=%q err=%v", body, err)
		}
		w.Header().Set("ETag", `"proxy-etag"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	backend, err := NewS3Backend(config.StorageConfig{Driver: "s3", S3: config.StorageS3Config{
		Endpoint: server.URL, Region: "auto", Bucket: "media", AccessKeyID: "test", SecretAccessKey: "secret", ForcePathStyle: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	backend.client = server.Client()
	backend.now = func() time.Time { return time.Date(2026, 8, 17, 8, 0, 0, 0, time.UTC) }
	upload := MultipartUpload{UploadID: "proxy-upload", ObjectKey: "media/original/video.mp4", ContentType: "video/mp4", SizeBytes: int64(len(content)), PartSize: int64(len(content)), PartCount: 1, Driver: "s3"}

	part, err := backend.PutMultipartPart(t.Context(), upload, 1, bytes.NewReader(content), int64(len(content)), checksum)
	if err != nil {
		t.Fatal(err)
	}
	if !requestSeen || part.PartNumber != 1 || part.ETag != "proxy-etag" || part.Checksum != checksum || part.SizeBytes != int64(len(content)) {
		t.Fatalf("part = %#v", part)
	}
}

func sha256HexTest(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func hasBareQueryKey(target *url.URL, key string) bool {
	for _, part := range strings.Split(target.RawQuery, "&") {
		if part == key || strings.HasPrefix(part, key+"=") {
			return true
		}
	}
	return false
}
