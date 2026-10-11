package storage

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
)

// TestRustFSLiveRoundTrip validates the project's hand-rolled SigV4 S3 client
// against a real RustFS server. It never runs in CI: it activates only when
// RUSTFS_SMOKE_ENDPOINT points at a live instance, e.g.
//
//	docker run -d --name rustfs-probe -p 127.0.0.1:39000:9000 \
//	  -e RUSTFS_ACCESS_KEY=test-ak -e RUSTFS_SECRET_KEY=test-sk-12345678 \
//	  -e RUSTFS_ADDRESS=0.0.0.0:9000 -e RUSTFS_CONSOLE_ADDRESS=0.0.0.0:9001 \
//	  rustfs/rustfs:1.0.1
//	docker run --rm --network host \
//	  -e AWS_ACCESS_KEY_ID=test-ak -e AWS_SECRET_ACCESS_KEY=test-sk-12345678 \
//	  -e AWS_DEFAULT_REGION=us-east-1 amazon/aws-cli \
//	  s3api create-bucket --bucket rustfs-smoke \
//	  --endpoint-url http://127.0.0.1:39000
//	RUSTFS_SMOKE_ENDPOINT=http://127.0.0.1:39000 \
//	RUSTFS_SMOKE_ACCESS_KEY=test-ak \
//	RUSTFS_SMOKE_SECRET_KEY=test-sk-12345678 \
//	RUSTFS_SMOKE_BUCKET=rustfs-smoke \
//	go test ./internal/storage -run TestRustFSLiveRoundTrip -v
func TestRustFSLiveRoundTrip(t *testing.T) {
	endpoint := strings.TrimSpace(getenvOr("RUSTFS_SMOKE_ENDPOINT", ""))
	if endpoint == "" {
		t.Skip("RUSTFS_SMOKE_ENDPOINT not set; live RustFS compatibility probe disabled")
	}
	bucket := getenvOr("RUSTFS_SMOKE_BUCKET", "rustfs-smoke")
	s3Backend, err := NewS3Backend(config.StorageConfig{
		Driver: "s3",
		S3: config.StorageS3Config{
			Endpoint:        endpoint,
			Region:          getenvOr("RUSTFS_SMOKE_REGION", "us-east-1"),
			Bucket:          bucket,
			AccessKeyID:     getenvOr("RUSTFS_SMOKE_ACCESS_KEY", ""),
			SecretAccessKey: getenvOr("RUSTFS_SMOKE_SECRET_KEY", ""),
			ForcePathStyle:  true,
			Prefix:          "smoke",
		},
	})
	if err != nil {
		t.Fatalf("NewS3Backend: %v", err)
	}
	var backend Backend = s3Backend
	ctx := context.Background()

	t.Run("put-get-bounded-delete", func(t *testing.T) {
		key := "generated-images/rustfs-live.png"
		content := []byte("rustfs-live-roundtrip")
		if err := backend.Put(ctx, key, "image/png", content); err != nil {
			t.Fatalf("Put: %v", err)
		}
		loaded, err := backend.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !bytes.Equal(loaded, content) {
			t.Fatalf("Get content = %q", loaded)
		}
		bounded, err := backend.(BoundedGetter).GetBounded(ctx, key, 1<<20)
		if err != nil {
			t.Fatalf("GetBounded: %v", err)
		}
		if !bytes.Equal(bounded, content) {
			t.Fatalf("GetBounded content = %q", bounded)
		}
		if err := backend.Delete(ctx, key); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := backend.Get(ctx, key); err != ErrNotFound {
			t.Fatalf("deleted object error = %v, want ErrNotFound", err)
		}
	})

	t.Run("streaming-transfers", func(t *testing.T) {
		streaming := backend.(StreamingBackend)
		key := "uploads/rustfs-live-video.bin"
		payload := bytes.Repeat([]byte("rustfs-stream-"), 64*1024)
		if err := streaming.PutReader(ctx, key, "application/octet-stream", bytes.NewReader(payload), int64(len(payload))); err != nil {
			t.Fatalf("PutReader: %v", err)
		}
		reader, size, err := streaming.OpenReader(ctx, key, int64(len(payload))+1)
		if err != nil {
			t.Fatalf("OpenReader: %v", err)
		}
		loaded, err := io.ReadAll(reader)
		closeErr := reader.Close()
		if err != nil {
			t.Fatalf("read streamed object: %v", err)
		}
		if closeErr != nil {
			t.Fatalf("close streamed reader: %v", closeErr)
		}
		if size != int64(len(payload)) || !bytes.Equal(loaded, payload) {
			t.Fatalf("streamed round-trip mismatch: size=%d want %d", size, len(payload))
		}
		if err := backend.Delete(ctx, key); err != nil {
			t.Fatalf("Delete: %v", err)
		}
	})

	t.Run("copy-and-list", func(t *testing.T) {
		copier := backend.(ObjectCopier)
		lister := backend.(ObjectLister)
		source := "generated-images/copy-source.png"
		if err := backend.Put(ctx, source, "image/png", []byte("copy-me")); err != nil {
			t.Fatalf("Put source: %v", err)
		}
		destination := "generated-images/copies/copy-dest.png"
		if err := copier.Copy(ctx, source, destination); err != nil {
			t.Fatalf("Copy: %v", err)
		}
		page, err := lister.ListObjects(ctx, "generated-images/copies/", "", 10)
		if err != nil {
			t.Fatalf("ListObjects: %v", err)
		}
		var found bool
		for _, object := range page.Objects {
			if object.ObjectKey == destination {
				found = true
			}
		}
		if !found {
			t.Fatalf("ListObjects did not return copied object: %#v", page.Objects)
		}
		if err := backend.Delete(ctx, source); err != nil {
			t.Fatalf("Delete source: %v", err)
		}
		if err := backend.Delete(ctx, destination); err != nil {
			t.Fatalf("Delete destination: %v", err)
		}
	})

	t.Run("presigned-get-url", func(t *testing.T) {
		key := "generated-images/presigned.png"
		content := []byte("presigned-content")
		if err := backend.Put(ctx, key, "image/png", content); err != nil {
			t.Fatalf("Put: %v", err)
		}
		signer := backend.(TemporaryURLSigner)
		raw, err := signer.TemporaryGetURL(ctx, key, TemporaryGetURLOptions{
			Expiry:               2 * time.Minute,
			ResponseFilename:     "rustfs-live.png",
			ContentType:          "image/png",
			ResponseCacheControl: "private, max-age=60",
		})
		if err != nil {
			t.Fatalf("TemporaryGetURL: %v", err)
		}
		parsed, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse presigned url: %v", err)
		}
		if parsed.Path != "/"+bucket+"/smoke/"+key {
			t.Fatalf("presigned path = %q", parsed.Path)
		}
		response, err := http.Get(raw)
		if err != nil {
			t.Fatalf("GET presigned url: %v", err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("presigned GET status = %d body=%s", response.StatusCode, body)
		}
		if !bytes.Equal(body, content) {
			t.Fatalf("presigned content = %q", body)
		}
		if got := response.Header.Get("Content-Disposition"); !strings.Contains(got, "rustfs-live.png") {
			t.Fatalf("Content-Disposition = %q", got)
		}
		if err := backend.Delete(ctx, key); err != nil {
			t.Fatalf("Delete: %v", err)
		}
	})
}

func getenvOr(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
