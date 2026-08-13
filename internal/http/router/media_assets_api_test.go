package router

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"

	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	galleryexportservice "github.com/fatballfish/pic-gallery/internal/service/galleryexport"
	mediaassetservice "github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/storage"
)

func TestUnifiedMediaAssetsAPIProjectsNewAndLegacyAssetsWithOwnerIsolation(t *testing.T) {
	cfg := taskAPIConfig("http://provider.invalid")
	authSvc, owner := loginTestUser(t, "unified-media-owner@example.com")
	other := loginExistingAuthUser(t, authSvc, "unified-media-other@example.com")
	ownerClaims, err := authSvc.ParseAccessToken(owner.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	ownerID := ownerClaims.UserID
	client, err := repoent.Open(dialect.SQLite, "file:unified-media-api?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	project, err := client.Project.Create().SetUserID(ownerID).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	target, err := client.Project.Create().SetUserID(ownerID).SetName("Campaign").SetNameKey("campaign").Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	assetID := uuid.New()
	videoTaskID := uuid.New()
	if _, err := client.MediaAsset.Create().SetID(assetID).SetUserID(ownerID).SetProjectID(project.ID).
		SetName("clip.mp4").SetNameKey("clip.mp4").SetMediaType("video").SetSourceType("generated").SetStatus("ready").
		SetSourceTaskKind("video").SetSourceTaskID(videoTaskID).
		SetStorageDriver("local").SetObjectKey("media/original/owner/clip.mp4").SetMimeType("video/mp4").SetFileSizeBytes(100).Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	legacyID := uuid.New()
	taskID := uuid.New()
	if _, err := client.ImageTask.Create().SetID(taskID).SetUserID(ownerID).SetProjectID(project.ID).SetTaskType("text_to_image").SetPrompt("legacy").SetAbstractModel("basic").Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ImageResult.Create().SetID(legacyID).SetTaskID(taskID).SetUserID(ownerID).SetProjectID(project.ID).
		SetObjectKey("generated-images/legacy.png").SetMimeType("image/png").SetSha256(strings.Repeat("a", 64)).Save(t.Context()); err != nil {
		t.Fatal(err)
	}

	mediaService := mediaassetservice.NewService(entstore.NewMediaStore(client), storage.NewStaticRouter(storage.NewLocalBackend(t.TempDir())), mediaassetservice.Options{Policy: domainmedia.DefaultPolicy()})
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, nil, nil)
	api.SetMediaAssetService(mediaService)
	handler := NewWithAPI(api)

	videoList := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodGet, "/api/agent/media/v1/assets?project_id="+project.ID.String()+"&media_type=video", "", nil)
	if videoList.Code != http.StatusOK || !bytes.Contains(videoList.Body.Bytes(), []byte(assetID.String())) || bytes.Contains(videoList.Body.Bytes(), []byte(legacyID.String())) ||
		!bytes.Contains(videoList.Body.Bytes(), []byte(`"source_task_kind":"video"`)) || !bytes.Contains(videoList.Body.Bytes(), []byte(videoTaskID.String())) {
		t.Fatalf("video list=%d %s", videoList.Code, videoList.Body.String())
	}
	imageList := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodGet, "/api/agent/media/v1/assets?project_id="+project.ID.String()+"&media_type=image", "", nil)
	if imageList.Code != http.StatusOK || !bytes.Contains(imageList.Body.Bytes(), []byte(legacyID.String())) ||
		!bytes.Contains(imageList.Body.Bytes(), []byte(`"source_task_kind":"image"`)) || !bytes.Contains(imageList.Body.Bytes(), []byte(taskID.String())) {
		t.Fatalf("legacy projection list=%d %s", imageList.Code, imageList.Body.String())
	}
	foreign := authenticatedMediaRequest(t, handler, other.AccessToken, http.MethodGet, "/api/agent/media/v1/assets/"+assetID.String(), "", nil)
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("foreign detail=%d %s", foreign.Code, foreign.Body.String())
	}

	patch := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodPatch, "/api/agent/media/v1/assets/"+assetID.String(), `{"name":"Final clip.mp4","group_name":"Exports","project_id":"`+target.ID.String()+`","expected_version":1}`, nil)
	if patch.Code != http.StatusOK || !bytes.Contains(patch.Body.Bytes(), []byte("Final clip.mp4")) || !bytes.Contains(patch.Body.Bytes(), []byte(target.ID.String())) {
		t.Fatalf("patch=%d %s", patch.Code, patch.Body.String())
	}
	if _, err := client.MediaAssetReference.Create().SetAssetID(assetID).SetRefType("canvas_node").SetRefID(uuid.New()).SetRefKey("video-1").SetUserID(ownerID).Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.MediaDerivative.Create().SetID(uuid.New()).SetAssetID(assetID).SetKind("proxy").SetTransformVersion(1).SetStatus("ready").
		SetStorageDriver("local").SetObjectKey("media/derivatives/owner/clip-proxy.mp4").SetMimeType("video/mp4").SetFileSizeBytes(10).Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	deleted := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodDelete, "/api/agent/media/v1/assets/"+assetID.String(), `{"expected_version":2}`, nil)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete=%d %s", deleted.Code, deleted.Body.String())
	}
	visible, err := client.MediaAsset.Query().Where(mediaasset.IDEQ(assetID), mediaasset.DeletedAtIsNil()).Exist(t.Context())
	if err != nil || visible {
		t.Fatalf("deleted asset still visible=%t err=%v", visible, err)
	}
	referencedAccess := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodGet, "/api/agent/media/v1/assets/"+assetID.String()+"/access?purpose=preview", "", nil)
	if referencedAccess.Code != http.StatusConflict || !bytes.Contains(referencedAccess.Body.Bytes(), []byte("DERIVATIVE_NOT_READY")) {
		t.Fatalf("referenced deleted video access=%d %s", referencedAccess.Code, referencedAccess.Body.String())
	}
}

func TestUnifiedMediaUploadAPICompletesLocalUpload(t *testing.T) {
	cfg := taskAPIConfig("http://provider.invalid")
	authSvc, owner := loginTestUser(t, "media-upload-api@example.com")
	ownerClaims, err := authSvc.ParseAccessToken(owner.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	ownerID := ownerClaims.UserID
	client, err := repoent.Open(dialect.SQLite, "file:media-upload-api?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	project, err := client.Project.Create().SetUserID(ownerID).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	mediaService := mediaassetservice.NewService(entstore.NewMediaStore(client), storage.NewStaticRouter(storage.NewLocalBackend(t.TempDir())), mediaassetservice.Options{
		Policy: domainmedia.DefaultPolicy(), PartSize: 8 << 20, UploadTTL: time.Hour,
	})
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, enabledFeatureAdmin(t, "media_upload"), nil)
	api.SetMediaAssetService(mediaService)
	handler := NewWithAPI(api)
	content := []byte("0123456789")
	checksum := mediaSHA256Hex(content)
	initRecorder := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodPost, "/api/agent/media/v1/uploads", `{"project_id":"`+project.ID.String()+`","filename":"voice.wav","media_type":"audio","mime_type":"audio/wav","size_bytes":10}`, map[string]string{"Idempotency-Key": "media-api-upload"})
	if initRecorder.Code != http.StatusCreated {
		t.Fatalf("init=%d %s", initRecorder.Code, initRecorder.Body.String())
	}
	var initialized struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(initRecorder.Body.Bytes(), &initialized); err != nil {
		t.Fatal(err)
	}
	part := authenticatedMediaRequestBytes(t, handler, owner.AccessToken, http.MethodPut, "/api/agent/media/v1/uploads/"+initialized.Data.ID+"/parts/1", content, map[string]string{"X-Content-SHA256": checksum, "Content-Type": "application/octet-stream"})
	if part.Code != http.StatusOK {
		t.Fatalf("part=%d %s", part.Code, part.Body.String())
	}
	var partPayload struct {
		Data struct {
			PartNumber int    `json:"part_number"`
			ETag       string `json:"etag"`
		} `json:"data"`
	}
	if err := json.Unmarshal(part.Body.Bytes(), &partPayload); err != nil {
		t.Fatal(err)
	}
	completeBody := `{"parts":[{"part_number":1,"etag":"` + partPayload.Data.ETag + `"}]}`
	completed := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodPost, "/api/agent/media/v1/uploads/"+initialized.Data.ID+":complete", completeBody, nil)
	if completed.Code != http.StatusCreated || !bytes.Contains(completed.Body.Bytes(), []byte(`"media_type":"audio"`)) {
		t.Fatalf("complete=%d %s", completed.Code, completed.Body.String())
	}
}

func TestUnifiedMediaAssetBatchReturnsPerItemResults(t *testing.T) {
	cfg := taskAPIConfig("http://provider.invalid")
	authSvc, owner := loginTestUser(t, "media-batch-api@example.com")
	claims, err := authSvc.ParseAccessToken(owner.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	client, err := repoent.Open(dialect.SQLite, "file:media-batch-api?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	project, err := client.Project.Create().SetUserID(claims.UserID).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ids := []uuid.UUID{uuid.New(), uuid.New()}
	for index, id := range ids {
		if _, err := client.MediaAsset.Create().SetID(id).SetUserID(claims.UserID).SetProjectID(project.ID).SetName("asset").SetNameKey("asset").
			SetMediaType("image").SetSourceType("generated").SetStatus("ready").SetStorageDriver("local").
			SetObjectKey("media/original/batch/" + id.String()).SetMimeType("image/png").SetFileSizeBytes(int64(index + 1)).Save(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	mediaService := mediaassetservice.NewService(entstore.NewMediaStore(client), storage.NewStaticRouter(storage.NewLocalBackend(t.TempDir())), mediaassetservice.Options{Policy: domainmedia.DefaultPolicy()})
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, nil, nil)
	api.SetMediaAssetService(mediaService)
	handler := NewWithAPI(api)
	body := `{"group_name":"Finals","items":[{"id":"` + ids[0].String() + `","expected_version":1},{"id":"` + ids[1].String() + `","expected_version":9}]}`
	result := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodPost, "/api/agent/media/v1/assets:batch-group", body, nil)
	if result.Code != http.StatusOK || !bytes.Contains(result.Body.Bytes(), []byte(`"status":"succeeded"`)) || !bytes.Contains(result.Body.Bytes(), []byte(`"status":"failed"`)) || !bytes.Contains(result.Body.Bytes(), []byte(`"code":"CONFLICT"`)) {
		t.Fatalf("batch=%d %s", result.Code, result.Body.String())
	}
}

func TestUnifiedMediaAssetAccessSelectsDedicatedDerivativeAndNeverFallsBackToOriginal(t *testing.T) {
	cfg := taskAPIConfig("http://provider.invalid")
	authSvc, owner := loginTestUser(t, "media-purpose-api@example.com")
	claims, err := authSvc.ParseAccessToken(owner.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	client, err := repoent.Open(dialect.SQLite, "file:media-purpose-api?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	project, err := client.Project.Create().SetUserID(claims.UserID).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	assetID := uuid.New()
	if _, err := client.MediaAsset.Create().SetID(assetID).SetUserID(claims.UserID).SetProjectID(project.ID).SetName("clip.mp4").SetNameKey("clip.mp4").
		SetMediaType("video").SetSourceType("generated").SetStatus("ready").SetStorageDriver("local").
		SetObjectKey("media/original/clip.mp4").SetMimeType("video/mp4").SetFileSizeBytes(100).Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.MediaDerivative.Create().SetID(uuid.New()).SetAssetID(assetID).SetKind("poster").SetTransformVersion(1).SetStatus("ready").
		SetStorageDriver("local").SetObjectKey("media/derivatives/poster.jpg").SetMimeType("image/jpeg").SetFileSizeBytes(10).Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	mediaService := mediaassetservice.NewService(entstore.NewMediaStore(client), storage.NewStaticRouter(storage.NewLocalBackend(t.TempDir())), mediaassetservice.Options{Policy: domainmedia.DefaultPolicy()})
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, nil, nil)
	api.SetMediaAssetService(mediaService)
	handler := NewWithAPI(api)

	poster := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodGet, "/api/agent/media/v1/assets/"+assetID.String()+"/access?purpose=poster", "", nil)
	if poster.Code != http.StatusOK || !bytes.Contains(poster.Body.Bytes(), []byte("purpose=poster")) {
		t.Fatalf("poster access=%d %s", poster.Code, poster.Body.String())
	}
	hover := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodGet, "/api/agent/media/v1/assets/"+assetID.String()+"/access?purpose=hover", "", nil)
	if hover.Code != http.StatusConflict || !bytes.Contains(hover.Body.Bytes(), []byte("DERIVATIVE_NOT_READY")) {
		t.Fatalf("missing hover derivative must not fall back to original: %d %s", hover.Code, hover.Body.String())
	}
}

func TestUnifiedMediaBatchDownloadCreatesAsynchronousArchiveJob(t *testing.T) {
	cfg := taskAPIConfig("http://provider.invalid")
	authSvc, owner := loginTestUser(t, "media-export-api@example.com")
	claims, err := authSvc.ParseAccessToken(owner.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	client, err := repoent.Open(dialect.SQLite, "file:media-export-api?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	project, err := client.Project.Create().SetUserID(claims.UserID).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	assetID := uuid.New()
	if _, err := client.MediaAsset.Create().SetID(assetID).SetUserID(claims.UserID).SetProjectID(project.ID).SetName("voice.wav").SetNameKey("voice.wav").
		SetMediaType("audio").SetSourceType("local_upload").SetStatus("ready").SetStorageDriver("local").SetObjectKey("media/original/voice.wav").SetMimeType("audio/wav").SetFileSizeBytes(1024).Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	router := storage.NewStaticRouter(storage.NewLocalBackend(t.TempDir()))
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, nil, nil)
	api.SetMediaAssetService(mediaassetservice.NewService(entstore.NewMediaStore(client), router, mediaassetservice.Options{Policy: domainmedia.DefaultPolicy()}))
	api.SetGalleryExportService(galleryexportservice.NewService(entstore.NewGalleryExportStore(client), router, galleryexportservice.Options{}))
	handler := NewWithAPI(api)

	created := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodPost, "/api/agent/media/v1/assets:batch-download", `{"project_id":"`+project.ID.String()+`","items":[{"id":"`+assetID.String()+`","expected_version":1}]}`, nil)
	if created.Code != http.StatusAccepted || !bytes.Contains(created.Body.Bytes(), []byte(`"status_url":"/api/agent/media/v1/export-jobs/`)) {
		t.Fatalf("media export create=%d %s", created.Code, created.Body.String())
	}
	var payload struct {
		Data struct {
			Job struct {
				ID string `json:"id"`
			} `json:"job"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &payload); err != nil || payload.Data.Job.ID == "" {
		t.Fatalf("decode media export: %v %s", err, created.Body.String())
	}
	status := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodGet, "/api/agent/media/v1/export-jobs/"+payload.Data.Job.ID, "", nil)
	if status.Code != http.StatusOK || !bytes.Contains(status.Body.Bytes(), []byte(`"state":"queued"`)) {
		t.Fatalf("media export status=%d %s", status.Code, status.Body.String())
	}
}

func authenticatedMediaRequest(t *testing.T, handler http.Handler, token, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	return authenticatedMediaRequestBytes(t, handler, token, method, path, []byte(body), headers)
}

func authenticatedMediaRequestBytes(t *testing.T, handler http.Handler, token, method, path string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	if len(body) > 0 && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func mediaSHA256Hex(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}
