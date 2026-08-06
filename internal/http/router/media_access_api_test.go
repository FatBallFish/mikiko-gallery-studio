package router

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	"github.com/fatballfish/pic-gallery/internal/provider"
	assetservice "github.com/fatballfish/pic-gallery/internal/service/assets"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	"github.com/fatballfish/pic-gallery/internal/storage"
)

func TestStableMediaAccessProjectsOwnedImageAndReferenceURLs(t *testing.T) {
	cfg := taskAPIConfig("http://provider.invalid")
	authSvc, owner := loginTestUser(t, "media-access-owner@example.com")
	other := loginExistingAuthUser(t, authSvc, "media-access-other@example.com")
	backend := &mediaAccessBackend{}

	imageStore := imagetaskservice.NewMemoryStore()
	image := provider.ImageResult{
		ID: "media-image", StorageDriver: "s3", StorageConfigID: "bfss-primary", ObjectKey: "generated/media-image.png", MimeType: "image/png",
	}
	if err := imageStore.Save(t.Context(), domainimagetask.Task{ID: "media-task", UserID: 1, Status: domainimagetask.StatusSucceeded, Results: []provider.ImageResult{image}}); err != nil {
		t.Fatalf("save image result: %v", err)
	}
	taskSvc := imagetaskservice.NewServiceWithProvidersStoreAssetsBillingAndBackend(cfg, nil, imageStore, nil, nil, backend)
	assetSvc := assetservice.NewServiceWithStoreAndRouter(cfg.GenerationLimits, nil, storage.NewStaticRouter(backend))
	asset, err := assetSvc.UploadWithMetadataContext(t.Context(), 1, "reference.png", "image/png", tinyPNG(t), domainassets.UploadMetadata{UploadSource: "web"})
	if err != nil {
		t.Fatalf("upload reference asset: %v", err)
	}
	handler := NewWithAPI(handlers.NewAPIWithRuntimeServices(cfg, authSvc, assetSvc, taskSvc, nil, nil))

	for _, test := range []struct {
		name string
		path string
	}{
		{name: "image", path: "/api/agent/image/v1/images/" + image.ID + "/access"},
		{name: "reference", path: "/api/agent/image/v1/reference-assets/" + asset.ID + "/access"},
	} {
		t.Run(test.name, func(t *testing.T) {
			preview := requestMediaAccess(t, handler, owner.AccessToken, test.path+"?purpose=preview", http.StatusOK)
			if !strings.Contains(preview.URL, "/preview?") || preview.ExpiresAt != "2026-08-06T12:06:00Z" {
				t.Fatalf("unexpected preview access %#v", preview)
			}
			download := requestMediaAccess(t, handler, owner.AccessToken, test.path+"?purpose=download", http.StatusOK)
			if !strings.Contains(download.URL, "/download?") || download.ExpiresAt != "2026-08-06T12:05:00Z" {
				t.Fatalf("unexpected download access %#v", download)
			}
			requestMediaAccess(t, handler, other.AccessToken, test.path+"?purpose=preview", http.StatusNotFound)
		})
	}
	if got := backend.getCalls.Load(); got != 0 {
		t.Fatalf("media access proxied object bytes %d times", got)
	}
}

func TestStableMediaAccessRejectsUnknownPurpose(t *testing.T) {
	cfg := taskAPIConfig("http://provider.invalid")
	authSvc, owner := loginTestUser(t, "media-purpose-owner@example.com")
	backend := &mediaAccessBackend{}
	imageStore := imagetaskservice.NewMemoryStore()
	if err := imageStore.Save(t.Context(), domainimagetask.Task{ID: "media-purpose-task", UserID: 1, Status: domainimagetask.StatusSucceeded, Results: []provider.ImageResult{{
		ID: "media-purpose-image", StorageDriver: "s3", StorageConfigID: "bfss-primary", ObjectKey: "generated/media-purpose.png", MimeType: "image/png",
	}}}); err != nil {
		t.Fatalf("save image result: %v", err)
	}
	taskSvc := imagetaskservice.NewServiceWithProvidersStoreAssetsBillingAndBackend(cfg, nil, imageStore, nil, nil, backend)
	handler := NewWithAPI(handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, taskSvc, nil, nil))
	requestMediaAccess(t, handler, owner.AccessToken, "/api/agent/image/v1/images/media-purpose-image/access?purpose=raw", http.StatusBadRequest)
}

func TestStableMediaAccessProjectsApprovedPublicImageWithoutAuthentication(t *testing.T) {
	cfg := taskAPIConfig("http://provider.invalid")
	backend := &mediaAccessBackend{}
	imageStore := imagetaskservice.NewMemoryStore()
	if err := imageStore.Save(t.Context(), domainimagetask.Task{
		ID:     "public-media-task",
		UserID: 42,
		Status: domainimagetask.StatusSucceeded,
		Results: []provider.ImageResult{
			{
				ID: "approved-public-media", StorageDriver: "s3", StorageConfigID: "bfss-primary", ObjectKey: "generated/approved-public-media.png", MimeType: "image/png",
				VisibilityStatus: domainimagetask.VisibilityApproved,
			},
			{
				ID: "private-public-media", StorageDriver: "s3", StorageConfigID: "bfss-primary", ObjectKey: "generated/private-public-media.png", MimeType: "image/png",
				VisibilityStatus: domainimagetask.VisibilityPrivate,
			},
		},
	}); err != nil {
		t.Fatalf("save public image results: %v", err)
	}
	taskSvc := imagetaskservice.NewServiceWithProvidersStoreAssetsBillingAndBackend(cfg, nil, imageStore, nil, nil, backend)
	handler := NewWithAPI(handlers.NewAPIWithRuntimeServices(cfg, nil, nil, taskSvc, nil, nil))

	approved := requestOpenMediaAccess(t, handler, "/api/open/image/v1/gallery/images/approved-public-media/access?purpose=preview", http.StatusOK)
	if !strings.Contains(approved.URL, "/preview?") || approved.ExpiresAt != "2026-08-06T12:06:00Z" {
		t.Fatalf("unexpected public preview access %#v", approved)
	}
	requestOpenMediaAccess(t, handler, "/api/open/image/v1/gallery/images/private-public-media/access?purpose=preview", http.StatusNotFound)
	if got := backend.getCalls.Load(); got != 0 {
		t.Fatalf("public media access proxied object bytes %d times", got)
	}
}

func TestStableMediaAccessLocalFallbackRemainsAuthenticatedAndDoesNotReadDuringProjection(t *testing.T) {
	cfg := taskAPIConfig("http://provider.invalid")
	authSvc, owner := loginTestUser(t, "media-local-owner@example.com")
	backend := &localFallbackMediaBackend{objects: map[string][]byte{}}
	content := tinyPNG(t)
	imageKey := "generated/local-media.png"
	backend.objects[imageKey] = append([]byte(nil), content...)

	imageStore := imagetaskservice.NewMemoryStore()
	image := provider.ImageResult{ID: "local-media-image", StorageDriver: "local", ObjectKey: imageKey, MimeType: "image/png"}
	if err := imageStore.Save(t.Context(), domainimagetask.Task{ID: "local-media-task", UserID: 1, Status: domainimagetask.StatusSucceeded, Results: []provider.ImageResult{image}}); err != nil {
		t.Fatalf("save local image result: %v", err)
	}
	router := storage.NewStaticRouter(backend)
	taskSvc := imagetaskservice.NewServiceWithProvidersStoreAssetsBillingAndRouter(cfg, nil, imageStore, nil, nil, router)
	assetSvc := assetservice.NewServiceWithStoreAndRouter(cfg.GenerationLimits, nil, router)
	asset, err := assetSvc.UploadWithMetadataContext(t.Context(), 1, "local-reference.png", "image/png", content, domainassets.UploadMetadata{UploadSource: "web"})
	if err != nil {
		t.Fatalf("upload local reference asset: %v", err)
	}
	handler := NewWithAPI(handlers.NewAPIWithRuntimeServices(cfg, authSvc, assetSvc, taskSvc, nil, nil))

	imageAccess := requestMediaAccess(t, handler, owner.AccessToken, "/api/agent/image/v1/images/"+image.ID+"/access?purpose=download", http.StatusOK)
	referenceAccess := requestMediaAccess(t, handler, owner.AccessToken, "/api/agent/image/v1/reference-assets/"+asset.ID+"/access?purpose=download", http.StatusOK)
	if imageAccess.URL != "/api/agent/image/v1/images/"+image.ID || referenceAccess.URL != "/api/agent/image/v1/reference-assets/"+asset.ID+"/download" {
		t.Fatalf("unexpected local fallback URLs image=%q reference=%q", imageAccess.URL, referenceAccess.URL)
	}
	if backend.getCalls.Load() != 0 {
		t.Fatalf("projecting local fallback URLs must not read object bytes, gets=%d", backend.getCalls.Load())
	}

	for _, path := range []string{imageAccess.URL, referenceAccess.URL} {
		req := httptest.NewRequest(http.MethodGet, path+"?access_token="+owner.AccessToken, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), content) {
			t.Fatalf("authenticated local fallback status=%d path=%s body=%q", rec.Code, path, rec.Body.String())
		}
	}
}

type mediaAccessResponse struct {
	URL       string `json:"url"`
	ExpiresAt string `json:"expires_at"`
}

func requestMediaAccess(t *testing.T, handler http.Handler, token, path string, wantStatus int) mediaAccessResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("media access status=%d want=%d body=%s", rec.Code, wantStatus, rec.Body.String())
	}
	if wantStatus != http.StatusOK {
		return mediaAccessResponse{}
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("media access projection must not be cached, Cache-Control=%q", got)
	}
	var payload struct {
		Data mediaAccessResponse `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode media access: %v", err)
	}
	return payload.Data
}

func requestOpenMediaAccess(t *testing.T, handler http.Handler, path string, wantStatus int) mediaAccessResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("open media access status=%d want=%d body=%s", rec.Code, wantStatus, rec.Body.String())
	}
	if wantStatus != http.StatusOK {
		return mediaAccessResponse{}
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("open media access projection must not be cached, Cache-Control=%q", got)
	}
	var payload struct {
		Data mediaAccessResponse `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode open media access: %v", err)
	}
	return payload.Data
}

type mediaAccessBackend struct {
	getCalls atomic.Int32
}

type localFallbackMediaBackend struct {
	objects  map[string][]byte
	getCalls atomic.Int32
}

func (*localFallbackMediaBackend) Driver() string { return "local" }
func (backend *localFallbackMediaBackend) Put(_ context.Context, key, _ string, content []byte) error {
	backend.objects[key] = append([]byte(nil), content...)
	return nil
}
func (backend *localFallbackMediaBackend) Get(_ context.Context, key string) ([]byte, error) {
	backend.getCalls.Add(1)
	content, ok := backend.objects[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return append([]byte(nil), content...), nil
}
func (backend *localFallbackMediaBackend) Delete(_ context.Context, key string) error {
	delete(backend.objects, key)
	return nil
}

func (*mediaAccessBackend) Driver() string                                    { return "s3" }
func (*mediaAccessBackend) Put(context.Context, string, string, []byte) error { return nil }
func (backend *mediaAccessBackend) Get(context.Context, string) ([]byte, error) {
	backend.getCalls.Add(1)
	return nil, nil
}
func (*mediaAccessBackend) Delete(context.Context, string) error { return nil }
func (*mediaAccessBackend) TemporaryGetURL(_ context.Context, objectKey string, options storage.TemporaryGetURLOptions) (string, error) {
	purpose := "preview"
	expires := "360"
	if options.ResponseFilename != "" {
		purpose = "download"
		expires = "300"
	}
	return "https://bfss.example.test/" + purpose + "?object=" + objectKey + "&X-Amz-Date=20260806T120000Z&X-Amz-Expires=" + expires, nil
}
