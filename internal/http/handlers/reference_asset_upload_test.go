package handlers

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	assetservice "github.com/fatballfish/pic-gallery/internal/service/assets"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

func TestReadReferenceAssetUploadUsesBoundedCurrentPolicy(t *testing.T) {
	svc := assetservice.NewService(config.StorageConfig{LocalRoot: t.TempDir()}, config.GenerationLimitsConfig{ReferenceImageMaxMB: 1})

	t.Run("accepts exact file limit", func(t *testing.T) {
		req := multipartUploadRequest(t, bytes.Repeat([]byte{1}, 1024*1024))
		rec := httptest.NewRecorder()
		filename, contentType, content, appErr := readReferenceAssetUpload(rec, req, svc)
		if appErr != nil {
			t.Fatalf("readReferenceAssetUpload: %v", appErr)
		}
		if filename != "upload.png" || contentType != "image/png" || len(content) != 1024*1024 {
			t.Fatalf("unexpected upload result filename=%q contentType=%q bytes=%d", filename, contentType, len(content))
		}
	})

	t.Run("rejects max plus one byte", func(t *testing.T) {
		req := multipartUploadRequest(t, bytes.Repeat([]byte{1}, 1024*1024+1))
		rec := httptest.NewRecorder()
		_, _, _, appErr := readReferenceAssetUpload(rec, req, svc)
		if appErr == nil || appErr.Code != errs.CodeImageReferenceTooLarge {
			t.Fatalf("expected %s, got %#v", errs.CodeImageReferenceTooLarge, appErr)
		}
		if appErr.Details["max_size_bytes"] != int64(1024*1024) || appErr.Details["actual_size_bytes"] != int64(1024*1024+1) {
			t.Fatalf("unexpected size details: %#v", appErr.Details)
		}
	})
}

func multipartUploadRequest(t *testing.T, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", `form-data; name="file"; filename="upload.png"`)
	partHeader.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest("POST", "/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}
