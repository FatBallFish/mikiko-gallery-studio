package storage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
