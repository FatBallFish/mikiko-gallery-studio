package router

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	domainproject "github.com/fatballfish/pic-gallery/internal/domain/project"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	"github.com/fatballfish/pic-gallery/internal/provider"
	assetservice "github.com/fatballfish/pic-gallery/internal/service/assets"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	projectservice "github.com/fatballfish/pic-gallery/internal/service/project"
	"github.com/fatballfish/pic-gallery/internal/storage"
)

func TestGalleryImportRouteCreatesAliasWithoutStorageIO(t *testing.T) {
	content := tinyPNG(t)
	hash := sha256.Sum256(content)
	backend := &galleryImportCopyBackend{objects: map[string][]byte{"generated/source.png": content}}
	router := storage.NewStaticRouter(backend)
	projectSvc := projectservice.NewService(projectservice.NewMemoryStore())
	defaultProject, err := projectSvc.EnsureDefault(t.Context(), 1)
	if err != nil {
		t.Fatalf("ensure default project: %v", err)
	}
	imageStore := imagetaskservice.NewMemoryStore()
	result := provider.ImageResult{
		ID: "copy-route-image", ProjectID: defaultProject.ID, StorageDriver: "s3", ObjectKey: "generated/source.png", MimeType: "image/png",
		FileSizeBytes: int64(len(content)), Width: 1, Height: 1, SHA256: hex.EncodeToString(hash[:]),
	}
	if err := imageStore.Save(t.Context(), domainimagetask.Task{ID: "copy-route-task", UserID: 1, ProjectID: defaultProject.ID, Status: domainimagetask.StatusSucceeded, Results: []provider.ImageResult{result}}); err != nil {
		t.Fatalf("save image result: %v", err)
	}
	cfg := taskAPIConfig("http://provider.invalid")
	authSvc, session := loginTestUser(t, "gallery-copy-route@example.com")
	taskSvc := imagetaskservice.NewServiceWithProvidersStoreAssetsBillingAndRouter(cfg, nil, imageStore, nil, nil, router)
	assetSvc := assetservice.NewServiceWithStoreAndRouter(cfg.GenerationLimits, nil, router)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, assetSvc, taskSvc, nil, nil)
	api.SetProjectService(projectSvc)
	handler := NewWithAPI(api)

	req := httptest.NewRequest(http.MethodPost, "/api/agent/image/v1/reference-assets:import-from-gallery", bytes.NewBufferString(`{"gallery_image_ids":["copy-route-image"]}`))
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("import status=%d body=%s", rec.Code, rec.Body.String())
	}
	if backend.copyCalls.Load() != 0 || backend.getCalls.Load() != 0 || backend.putCalls.Load() != 0 {
		t.Fatalf("route must create alias without storage IO: copy=%d get=%d put=%d", backend.copyCalls.Load(), backend.getCalls.Load(), backend.putCalls.Load())
	}
	var response struct {
		Data struct {
			Items []domainassets.ReferenceAsset `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil || len(response.Data.Items) != 1 {
		t.Fatalf("decode alias response: %v body=%s", err, rec.Body.String())
	}
	asset := response.Data.Items[0]
	if asset.SourceImageResultID != result.ID || asset.OwnsObject || asset.ObjectKey != result.ObjectKey {
		t.Fatalf("alias response=%#v", asset)
	}
}

func TestGalleryImportRejectsImageFromAnotherProject(t *testing.T) {
	content := tinyPNG(t)
	hash := sha256.Sum256(content)
	backend := &galleryImportCopyBackend{objects: map[string][]byte{"generated/source.png": content}}
	storageRouter := storage.NewStaticRouter(backend)
	projectSvc := projectservice.NewService(projectservice.NewMemoryStore())
	sourceProject, err := projectSvc.Create(t.Context(), 1, domainproject.CreateRequest{Name: "Source"})
	if err != nil {
		t.Fatalf("create source project: %v", err)
	}
	targetProject, err := projectSvc.Create(t.Context(), 1, domainproject.CreateRequest{Name: "Target"})
	if err != nil {
		t.Fatalf("create target project: %v", err)
	}
	imageStore := imagetaskservice.NewMemoryStore()
	result := provider.ImageResult{
		ID: "cross-project-image", ProjectID: sourceProject.ID, StorageDriver: "s3", ObjectKey: "generated/source.png", MimeType: "image/png",
		FileSizeBytes: int64(len(content)), Width: 1, Height: 1, SHA256: hex.EncodeToString(hash[:]),
	}
	if err := imageStore.Save(t.Context(), domainimagetask.Task{ID: "cross-project-task", UserID: 1, ProjectID: sourceProject.ID, Status: domainimagetask.StatusSucceeded, Results: []provider.ImageResult{result}}); err != nil {
		t.Fatalf("save image result: %v", err)
	}
	cfg := taskAPIConfig("http://provider.invalid")
	authSvc, session := loginTestUser(t, "gallery-cross-project@example.com")
	taskSvc := imagetaskservice.NewServiceWithProvidersStoreAssetsBillingAndRouter(cfg, nil, imageStore, nil, nil, storageRouter)
	taskSvc.SetProjectResolver(projectSvc)
	assetSvc := assetservice.NewServiceWithStoreAndRouter(cfg.GenerationLimits, nil, storageRouter)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, assetSvc, taskSvc, nil, nil)
	api.SetProjectService(projectSvc)
	handler := NewWithAPI(api)

	req := httptest.NewRequest(http.MethodPost, "/api/agent/image/v1/reference-assets:import-from-gallery", bytes.NewBufferString(`{"gallery_image_ids":["cross-project-image"],"project_id":"`+targetProject.ID+`"}`))
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-project import status=%d body=%s, want 404", rec.Code, rec.Body.String())
	}
	if backend.copyCalls.Load() != 0 {
		t.Fatalf("cross-project import copied %d objects, want 0", backend.copyCalls.Load())
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
