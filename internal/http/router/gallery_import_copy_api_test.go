package router

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	"github.com/fatballfish/pic-gallery/internal/provider"
	assetservice "github.com/fatballfish/pic-gallery/internal/service/assets"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	"github.com/fatballfish/pic-gallery/internal/storage"
)

func TestGalleryImportRouteUsesStorageCopyWithoutReadingImageBytes(t *testing.T) {
	content := tinyPNG(t)
	hash := sha256.Sum256(content)
	backend := &galleryImportCopyBackend{objects: map[string][]byte{"generated/source.png": content}}
	router := storage.NewStaticRouter(backend)
	imageStore := imagetaskservice.NewMemoryStore()
	result := provider.ImageResult{
		ID: "copy-route-image", StorageDriver: "s3", ObjectKey: "generated/source.png", MimeType: "image/png",
		FileSizeBytes: int64(len(content)), Width: 1, Height: 1, SHA256: hex.EncodeToString(hash[:]),
	}
	if err := imageStore.Save(t.Context(), domainimagetask.Task{ID: "copy-route-task", UserID: 1, Status: domainimagetask.StatusSucceeded, Results: []provider.ImageResult{result}}); err != nil {
		t.Fatalf("save image result: %v", err)
	}
	cfg := taskAPIConfig("http://provider.invalid")
	authSvc, session := loginTestUser(t, "gallery-copy-route@example.com")
	taskSvc := imagetaskservice.NewServiceWithProvidersStoreAssetsBillingAndRouter(cfg, nil, imageStore, nil, nil, router)
	assetSvc := assetservice.NewServiceWithStoreAndRouter(cfg.GenerationLimits, nil, router)
	handler := NewWithAPI(handlers.NewAPIWithRuntimeServices(cfg, authSvc, assetSvc, taskSvc, nil, nil))

	req := httptest.NewRequest(http.MethodPost, "/api/agent/image/v1/reference-assets:import-from-gallery", bytes.NewBufferString(`{"gallery_image_ids":["copy-route-image"]}`))
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("import status=%d body=%s", rec.Code, rec.Body.String())
	}
	if backend.copyCalls.Load() != 1 || backend.getCalls.Load() != 0 || backend.putCalls.Load() != 0 {
		t.Fatalf("route must copy without reading bytes: copy=%d get=%d put=%d", backend.copyCalls.Load(), backend.getCalls.Load(), backend.putCalls.Load())
	}
}

type galleryImportCopyBackend struct {
	objects   map[string][]byte
	copyCalls atomic.Int32
	getCalls  atomic.Int32
	putCalls  atomic.Int32
}

func (*galleryImportCopyBackend) Driver() string { return "s3" }
func (b *galleryImportCopyBackend) Put(_ context.Context, key, _ string, content []byte) error {
	b.putCalls.Add(1)
	b.objects[key] = append([]byte(nil), content...)
	return nil
}
func (b *galleryImportCopyBackend) Get(_ context.Context, key string) ([]byte, error) {
	b.getCalls.Add(1)
	content, ok := b.objects[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return append([]byte(nil), content...), nil
}
func (b *galleryImportCopyBackend) Delete(_ context.Context, key string) error {
	delete(b.objects, key)
	return nil
}
func (b *galleryImportCopyBackend) Copy(_ context.Context, sourceKey, destinationKey string) error {
	b.copyCalls.Add(1)
	content, ok := b.objects[sourceKey]
	if !ok {
		return storage.ErrNotFound
	}
	b.objects[destinationKey] = append([]byte(nil), content...)
	return nil
}
func (b *galleryImportCopyBackend) GetBounded(_ context.Context, key string, maxBytes int64) ([]byte, error) {
	content, ok := b.objects[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	if int64(len(content)) > maxBytes {
		return nil, storage.ErrObjectTooLarge
	}
	return append([]byte(nil), content...), nil
}
