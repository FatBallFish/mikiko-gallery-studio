package storage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
)

func TestS3RangeReadsOnlyRequestedTail(t *testing.T) {
	for _, mode := range []string{"valid", "ignored", "mismatch", "metadata"} {
		t.Run(mode, func(t *testing.T) {
			backend, err := NewS3Backend(config.StorageConfig{Driver: "s3", S3: config.StorageS3Config{Endpoint: "https://s3.example.test", Region: "us-east-1", Bucket: "bucket", AccessKeyID: "access", SecretAccessKey: "secret", ForcePathStyle: true}})
			if err != nil {
				t.Fatal(err)
			}
			gets := 0
			body := &trackingReadCloser{Reader: strings.NewReader("789")}
			backend.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Header.Get("Authorization") == "" {
					t.Fatal("unsigned storage request")
				}
				if req.Method == http.MethodHead {
					return &http.Response{StatusCode: 200, ContentLength: 10, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{"Etag": []string{"revision"}}}, nil
				}
				gets++
				if req.Header.Get("Range") != "bytes=7-9" || req.Header.Get("If-Match") != "revision" {
					t.Fatalf("range request headers %v", req.Header)
				}
				status := 206
				contentRange := "bytes 7-9/10"
				if mode == "ignored" {
					status = 200
				}
				if mode == "mismatch" {
					contentRange = "bytes 0-2/10"
				}
				return &http.Response{StatusCode: status, ContentLength: 3, Body: body, Header: http.Header{"Content-Range": []string{contentRange}}}, nil
			})}
			stream, err := backend.OpenRange(t.Context(), "video", "bytes=-3", 100, mode == "metadata")
			if mode == "ignored" || mode == "mismatch" {
				if err == nil || !body.closed {
					t.Fatalf("invalid storage range accepted/left open %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "metadata" {
				if gets != 0 || stream.Reader != nil {
					t.Fatal("HEAD fetched body")
				}
				return
			}
			defer stream.Reader.Close()
			bytes, err := io.ReadAll(stream.Reader)
			if err != nil || string(bytes) != "789" || gets != 1 {
				t.Fatalf("tail=%q err=%v GETs=%d", bytes, err, gets)
			}
		})
	}
}
func TestLocalRangeAndCancellation(t *testing.T) {
	backend := NewLocalBackend(t.TempDir())
	if err := backend.Put(t.Context(), "video", "video/mp4", []byte("0123456789")); err != nil {
		t.Fatal(err)
	}
	stream, err := backend.OpenRange(t.Context(), "video", "bytes=-3", 10, false)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Reader.Close()
	bytes, err := io.ReadAll(stream.Reader)
	if err != nil || string(bytes) != "789" {
		t.Fatalf("tail=%q %v", bytes, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = backend.OpenRange(ctx, "video", "bytes=0-1", 10, false); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error %v", err)
	}
}
