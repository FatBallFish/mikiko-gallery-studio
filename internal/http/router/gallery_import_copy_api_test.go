package router

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	domainproject "github.com/fatballfish/pic-gallery/internal/domain/project"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	"github.com/fatballfish/pic-gallery/internal/provider"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	assetservice "github.com/fatballfish/pic-gallery/internal/service/assets"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	projectservice "github.com/fatballfish/pic-gallery/internal/service/project"
	"github.com/fatballfish/pic-gallery/internal/storage"
)

func TestGalleryAliasCreationDoesNotRequireRolloutConfiguration(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:gallery-alias-rollout-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := client.Project.Create().SetUserID(1).SetName("默认").SetNameKey("默认").SetIsDefault(true).SetStatus("active").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	task, err := client.ImageTask.Create().SetUserID(1).SetProjectID(project.ID).SetTaskType("text_to_image").SetPrompt("rollout").SetAbstractModel("plus").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	content := tinyPNG(t)
	hash := sha256.Sum256(content)
	results := make([]*repoent.ImageResult, 0, 2)
	for index := range 2 {
		result, createErr := client.ImageResult.Create().SetTaskID(task.ID).SetUserID(1).SetProjectID(project.ID).
			SetStorageDriver("s3").SetObjectKey(fmt.Sprintf("generated/rollout-%d.png", index)).SetMimeType("image/png").
			SetFileSizeBytes(int64(len(content))).SetWidth(1).SetHeight(1).SetSha256(hex.EncodeToString(hash[:])).Save(ctx)
		if createErr != nil {
			t.Fatal(createErr)
		}
		results = append(results, result)
	}
	backend := &galleryImportCopyBackend{objects: map[string][]byte{results[0].ObjectKey: content, results[1].ObjectKey: content}}
	storageRouter := storage.NewStaticRouter(backend)
	assetsStore := entstore.NewAssetsStore(client)
	assetSvc := assetservice.NewServiceWithStoreAndRouter(config.GenerationLimitsConfig{}, assetsStore, storageRouter)
	taskSvc := imagetaskservice.NewServiceWithProvidersStoreAssetsBillingAndRouter(taskAPIConfig("http://provider.invalid"), nil, entstore.NewImageTaskStore(client), assetSvc, nil, storageRouter)
	projectSvc := projectservice.NewService(entstore.NewProjectStore(client))
	taskSvc.SetProjectResolver(projectSvc)
	authSvc, session := loginTestUser(t, "gallery-alias-rollout@example.com")
	api := handlers.NewAPIWithRuntimeServices(taskAPIConfig("http://provider.invalid"), authSvc, assetSvc, taskSvc, nil, nil)
	api.SetProjectService(projectSvc)
	handler := NewWithAPI(api)

	importResult := func(resultID string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/agent/image/v1/reference-assets:import-from-gallery", bytes.NewBufferString(`{"gallery_image_ids":["`+resultID+`"]}`))
		req.Header.Set("Authorization", "Bearer "+session.AccessToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	created := importResult(results[0].ID.String())
	if created.Code != http.StatusCreated {
		t.Fatalf("import without rollout status=%d body=%s", created.Code, created.Body.String())
	}
	var response struct {
		Data struct {
			Items []domainassets.ReferenceAsset `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &response); err != nil || len(response.Data.Items) != 1 {
		t.Fatalf("decode activated alias: %v body=%s", err, created.Body.String())
	}
	if rec := importResult(results[1].ID.String()); rec.Code != http.StatusCreated {
		t.Fatalf("second import without rollout status=%d body=%s", rec.Code, rec.Body.String())
	}
	if count, err := client.ReferenceAsset.Query().Count(ctx); err != nil || count != 2 {
		t.Fatalf("imports without rollout aliases: count=%d err=%v", count, err)
	}
	if backend.copyCalls.Load() != 0 || backend.getCalls.Load() != 0 || backend.putCalls.Load() != 0 {
		t.Fatalf("imports performed storage IO: copy=%d get=%d put=%d", backend.copyCalls.Load(), backend.getCalls.Load(), backend.putCalls.Load())
	}
}

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

func TestGalleryImportAllowsImageFromAnotherProject(t *testing.T) {
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
	if rec.Code != http.StatusCreated {
		t.Fatalf("cross-project import status=%d body=%s, want 201", rec.Code, rec.Body.String())
	}
	if backend.copyCalls.Load() != 0 || backend.getCalls.Load() != 0 || backend.putCalls.Load() != 0 {
		t.Fatalf("cross-project import performed storage IO: copy=%d get=%d put=%d", backend.copyCalls.Load(), backend.getCalls.Load(), backend.putCalls.Load())
	}

	foreign := result
	foreign.ID = "foreign-user-image"
	foreign.ObjectKey = "generated/foreign.png"
	if err := imageStore.Save(t.Context(), domainimagetask.Task{
		ID: "foreign-user-task", UserID: 2, ProjectID: sourceProject.ID,
		Status: domainimagetask.StatusSucceeded, Results: []provider.ImageResult{foreign},
	}); err != nil {
		t.Fatalf("save foreign image: %v", err)
	}
	foreignReq := httptest.NewRequest(http.MethodPost, "/api/agent/image/v1/reference-assets:import-from-gallery", bytes.NewBufferString(`{"gallery_image_ids":["foreign-user-image"]}`))
	foreignReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	foreignReq.Header.Set("Content-Type", "application/json")
	foreignRec := httptest.NewRecorder()
	handler.ServeHTTP(foreignRec, foreignReq)
	if foreignRec.Code != http.StatusNotFound {
		t.Fatalf("foreign-user import status=%d body=%s, want 404", foreignRec.Code, foreignRec.Body.String())
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
