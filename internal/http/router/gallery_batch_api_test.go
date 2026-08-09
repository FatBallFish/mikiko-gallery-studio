package router

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainaudit "github.com/fatballfish/pic-gallery/internal/domain/audit"
	domainproject "github.com/fatballfish/pic-gallery/internal/domain/project"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imageresult"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/objectdeletionjob"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	auditservice "github.com/fatballfish/pic-gallery/internal/service/audit"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	galleryexportservice "github.com/fatballfish/pic-gallery/internal/service/galleryexport"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	projectservice "github.com/fatballfish/pic-gallery/internal/service/project"
	"github.com/fatballfish/pic-gallery/internal/storage"
)

func TestGalleryBatchRoutesReturnPartialResultsAndOneAuthorizedZIP(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, "file:gallery-batch-api-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: time.Hour, Issuer: "test", AccessTokenSecret: "secret", RefreshCookieName: "refresh",
	}, map[string]string{"basic": "1.00000"})
	session := loginExistingAuthUser(t, authSvc, "gallery-batch@example.com")
	projects := projectservice.NewService(entstore.NewProjectStore(client))
	source, err := projects.EnsureDefault(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	target, err := projects.Create(t.Context(), 1, domainproject.CreateRequest{Name: "Target", IdempotencyKey: "gallery-batch-target"})
	if err != nil {
		t.Fatal(err)
	}
	backend := storage.NewLocalBackend(t.TempDir())
	router := storage.NewStaticRouter(backend)
	taskStore := entstore.NewImageTaskStore(client)
	taskSvc := imagetaskservice.NewServiceWithProvidersStoreAssetsBillingAndRouter(config.Config{}, nil, taskStore, nil, nil, router)
	taskSvc.SetProjectResolver(projects)
	api := handlers.NewAPIWithTaskService(config.Config{}, authSvc, nil, taskSvc)
	api.SetProjectService(projects)
	api.SetGalleryExportService(galleryexportservice.NewService(entstore.NewGalleryExportStore(client), router, galleryexportservice.Options{}))
	handler := NewWithAPI(api)

	one := seedGalleryBatchAPIImage(t, client, backend, 1, source.ID, "same", "one")
	two := seedGalleryBatchAPIImage(t, client, backend, 1, source.ID, "same", "two")
	foreign := seedGalleryBatchAPIImage(t, client, backend, 2, source.ID, "foreign", "secret")

	group := galleryBatchRequest(t, handler, session.AccessToken, "/api/agent/gallery/v1/images:batch-group", map[string]any{
		"image_ids": []string{one, one, foreign, uuid.NewString()}, "project_id": source.ID, "image_group": "客户素材",
	})
	if group.Code != http.StatusOK {
		t.Fatalf("batch group status=%d body=%s", group.Code, group.Body.String())
	}
	var groupPayload struct {
		Data struct {
			Succeeded []struct {
				ID string `json:"id"`
			} `json:"succeeded"`
			Failed []struct {
				Code string `json:"code"`
			} `json:"failed"`
		} `json:"data"`
	}
	if err := json.NewDecoder(group.Body).Decode(&groupPayload); err != nil || len(groupPayload.Data.Succeeded) != 1 || len(groupPayload.Data.Failed) != 2 {
		t.Fatalf("batch group payload=%#v err=%v", groupPayload, err)
	}

	crossTenant := galleryBatchRequest(t, handler, session.AccessToken, "/api/agent/gallery/v1/images:batch-download", map[string]any{
		"image_ids": []string{one, foreign}, "project_id": source.ID,
	})
	if crossTenant.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant ZIP status=%d body=%s", crossTenant.Code, crossTenant.Body.String())
	}
	download := galleryBatchRequest(t, handler, session.AccessToken, "/api/agent/gallery/v1/images:batch-download", map[string]any{
		"image_ids": []string{one, two}, "project_id": source.ID,
	})
	if download.Code != http.StatusOK || download.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("batch ZIP status=%d content-type=%q body=%q", download.Code, download.Header().Get("Content-Type"), download.Body.String())
	}
	archive, err := zip.NewReader(bytes.NewReader(download.Body.Bytes()), int64(download.Body.Len()))
	if err != nil || len(archive.File) != 3 {
		t.Fatalf("ZIP entries=%d err=%v", len(archive.File), err)
	}

	transfer := galleryBatchRequest(t, handler, session.AccessToken, "/api/agent/gallery/v1/images:batch-transfer-project", map[string]any{
		"image_ids": []string{one}, "project_id": source.ID, "target_project_id": target.ID,
	})
	if transfer.Code != http.StatusOK || !bytes.Contains(transfer.Body.Bytes(), []byte(target.ID)) {
		t.Fatalf("batch transfer status=%d body=%s", transfer.Code, transfer.Body.String())
	}
	deleted := galleryBatchRequest(t, handler, session.AccessToken, "/api/agent/gallery/v1/images:batch-delete", map[string]any{
		"image_ids": []string{two}, "project_id": source.ID,
	})
	if deleted.Code != http.StatusOK {
		t.Fatalf("batch delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	if exists, err := client.ImageResult.Query().Where(imageresult.IDEQ(uuid.MustParse(two)), imageresult.DeletedAtIsNil()).Exist(t.Context()); err != nil || exists {
		t.Fatalf("deleted image remains live=%v err=%v", exists, err)
	}
	if count, err := client.ObjectDeletionJob.Query().Where(objectdeletionjob.ObjectKeyEQ("generated-images/" + two + ".png")).Count(t.Context()); err != nil || count != 1 {
		t.Fatalf("batch delete cleanup jobs=%d err=%v", count, err)
	}
}

func TestGalleryBatchDownloadPromotesLegacyUnknownSizeAndMapsDirectLimitToExportTooLarge(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, "file:gallery-batch-limits-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: time.Hour, Issuer: "test", AccessTokenSecret: "secret", RefreshCookieName: "refresh",
	}, map[string]string{"basic": "1.00000"})
	session := loginExistingAuthUser(t, authSvc, "gallery-export-limits@example.com")
	projects := projectservice.NewService(entstore.NewProjectStore(client))
	project, err := projects.EnsureDefault(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	backend := storage.NewLocalBackend(t.TempDir())
	storageRouter := storage.NewStaticRouter(backend)
	taskSvc := imagetaskservice.NewServiceWithProvidersStoreAssetsBillingAndRouter(config.Config{}, nil, entstore.NewImageTaskStore(client), nil, nil, storageRouter)
	taskSvc.SetProjectResolver(projects)
	api := handlers.NewAPIWithTaskService(config.Config{}, authSvc, nil, taskSvc)
	api.SetProjectService(projects)
	api.SetGalleryExportService(galleryexportservice.NewService(entstore.NewGalleryExportStore(client), storageRouter, galleryexportservice.Options{
		DirectMaxCount: 10, DirectMaxEstimatedBytes: 1 << 20, MaxSourceBytes: 4, MaxArchiveBytes: 1 << 20,
	}))
	handler := NewWithAPI(api)

	legacyID := seedGalleryBatchAPIImage(t, client, backend, 1, project.ID, "legacy", "legacy")
	if err := client.ImageResult.UpdateOneID(uuid.MustParse(legacyID)).SetFileSizeBytes(0).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	legacy := galleryBatchRequest(t, handler, session.AccessToken, "/api/agent/gallery/v1/images:batch-download", map[string]any{
		"image_ids": []string{legacyID}, "project_id": project.ID,
	})
	if legacy.Code != http.StatusAccepted {
		t.Fatalf("legacy unknown-size export status=%d body=%s", legacy.Code, legacy.Body.String())
	}

	oversizedID := seedGalleryBatchAPIImage(t, client, backend, 1, project.ID, "oversized", "oversized")
	if err := client.ImageResult.UpdateOneID(uuid.MustParse(oversizedID)).SetFileSizeBytes(1).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	oversized := galleryBatchRequest(t, handler, session.AccessToken, "/api/agent/gallery/v1/images:batch-download", map[string]any{
		"image_ids": []string{oversizedID}, "project_id": project.ID,
	})
	if oversized.Code != http.StatusRequestEntityTooLarge || !bytes.Contains(oversized.Body.Bytes(), []byte(`"code":"EXPORT_TOO_LARGE"`)) {
		t.Fatalf("direct over-limit export status=%d body=%s", oversized.Code, oversized.Body.String())
	}
}

func TestGalleryAsyncExportStatusAndDownloadAreOwnerScopedAndExpire(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, "file:gallery-async-api-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: time.Hour, Issuer: "test", AccessTokenSecret: "secret", RefreshCookieName: "refresh",
	}, map[string]string{"basic": "1.00000"})
	owner := loginExistingAuthUser(t, authSvc, "gallery-export-owner@example.com")
	other := loginExistingAuthUser(t, authSvc, "gallery-export-other@example.com")
	projects := projectservice.NewService(entstore.NewProjectStore(client))
	project, err := projects.EnsureDefault(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	backend := storage.NewLocalBackend(t.TempDir())
	storageRouter := storage.NewStaticRouter(backend)
	taskStore := entstore.NewImageTaskStore(client)
	taskSvc := imagetaskservice.NewServiceWithProvidersStoreAssetsBillingAndRouter(config.Config{}, nil, taskStore, nil, nil, storageRouter)
	taskSvc.SetProjectResolver(projects)
	exportStore := entstore.NewGalleryExportStore(client)
	api := handlers.NewAPIWithTaskService(config.Config{}, authSvc, nil, taskSvc)
	api.SetProjectService(projects)
	api.SetGalleryExportService(galleryexportservice.NewService(exportStore, storageRouter, galleryexportservice.Options{DirectMaxCount: 1}))
	handler := NewWithAPI(api)

	one := seedGalleryBatchAPIImage(t, client, backend, 1, project.ID, "one", "one")
	two := seedGalleryBatchAPIImage(t, client, backend, 1, project.ID, "two", "two")
	create := galleryBatchRequest(t, handler, owner.AccessToken, "/api/agent/gallery/v1/images:batch-download", map[string]any{
		"image_ids": []string{one, two}, "project_id": project.ID,
	})
	if create.Code != http.StatusAccepted {
		t.Fatalf("async export create status=%d body=%s", create.Code, create.Body.String())
	}
	var created struct {
		Data struct {
			Job galleryexportservice.Job `json:"job"`
		} `json:"data"`
	}
	if err := json.NewDecoder(create.Body).Decode(&created); err != nil || created.Data.Job.ID == "" || created.Data.Job.State != galleryexportservice.StateQueued {
		t.Fatalf("async export create payload=%#v err=%v", created, err)
	}
	if created.Data.Job.DeadlineAt == nil {
		t.Fatalf("queued export timing metadata=%#v", created.Data.Job)
	}
	statusPath := "/api/agent/gallery/v1/export-jobs/" + created.Data.Job.ID
	downloadPath := statusPath + "/download"
	if status := galleryExportGetRequest(handler, owner.AccessToken, statusPath); status.Code != http.StatusOK || !bytes.Contains(status.Body.Bytes(), []byte(`"state":"queued"`)) {
		t.Fatalf("owner queued status=%d body=%s", status.Code, status.Body.String())
	}
	if pending := galleryExportGetRequest(handler, owner.AccessToken, downloadPath); pending.Code != http.StatusConflict {
		t.Fatalf("pending download status=%d body=%s", pending.Code, pending.Body.String())
	}
	for _, path := range []string{statusPath, downloadPath} {
		if foreign := galleryExportGetRequest(handler, other.AccessToken, path); foreign.Code != http.StatusNotFound {
			t.Fatalf("foreign export path=%s status=%d body=%s", path, foreign.Code, foreign.Body.String())
		}
	}

	now := time.Now().UTC()
	claimed, ok, err := exportStore.AcquireNextJob(t.Context(), "api-test-worker", now, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim async export ok=%v err=%v", ok, err)
	}
	if running := galleryExportGetRequest(handler, owner.AccessToken, statusPath); running.Code != http.StatusOK || bytes.Contains(running.Body.Bytes(), []byte(`"processing_timeout_seconds"`)) || !bytes.Contains(running.Body.Bytes(), []byte(`"deadline_at"`)) {
		t.Fatalf("running timing metadata status=%d body=%s", running.Code, running.Body.String())
	}
	objectKey := "gallery-exports/1/" + claimed.ID + ".zip"
	archive := []byte("zip-content")
	if err := backend.Put(t.Context(), objectKey, "application/zip", archive); err != nil {
		t.Fatal(err)
	}
	expiresAt := now.Add(time.Hour)
	if _, err := exportStore.CompleteJob(t.Context(), galleryexportservice.CompleteJobRequest{
		JobID: claimed.ID, Owner: "api-test-worker", AttemptCount: claimed.AttemptCount,
		StorageDriver: "local", ObjectKey: objectKey, ArchiveSizeBytes: int64(len(archive)), ExpiresAt: expiresAt, CompletedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if status := galleryExportGetRequest(handler, owner.AccessToken, statusPath); status.Code != http.StatusOK || !bytes.Contains(status.Body.Bytes(), []byte(`"download_url"`)) {
		t.Fatalf("completed export status=%d body=%s", status.Code, status.Body.String())
	}
	if download := galleryExportGetRequest(handler, owner.AccessToken, downloadPath); download.Code != http.StatusOK || download.Header().Get("Content-Type") != "application/zip" || !bytes.Equal(download.Body.Bytes(), archive) {
		t.Fatalf("completed export download status=%d content-type=%q body=%q", download.Code, download.Header().Get("Content-Type"), download.Body.Bytes())
	}
	if count, err := exportStore.ExpireReady(t.Context(), expiresAt.Add(time.Second), 10); err != nil || count != 1 {
		t.Fatalf("expire async export count=%d err=%v", count, err)
	}
	if status := galleryExportGetRequest(handler, owner.AccessToken, statusPath); status.Code != http.StatusOK || !bytes.Contains(status.Body.Bytes(), []byte(`"state":"expired"`)) || bytes.Contains(status.Body.Bytes(), []byte(`"download_url"`)) {
		t.Fatalf("expired export status=%d body=%s", status.Code, status.Body.String())
	}
	if expired := galleryExportGetRequest(handler, owner.AccessToken, downloadPath); expired.Code != http.StatusConflict {
		t.Fatalf("expired download status=%d body=%s", expired.Code, expired.Body.String())
	}

	failedCreate := galleryBatchRequest(t, handler, owner.AccessToken, "/api/agent/gallery/v1/images:batch-download", map[string]any{
		"image_ids": []string{one, two}, "project_id": project.ID,
	})
	if failedCreate.Code != http.StatusAccepted {
		t.Fatalf("failed export create status=%d body=%s", failedCreate.Code, failedCreate.Body.String())
	}
	var failedJob struct {
		Data struct {
			Job galleryexportservice.Job `json:"job"`
		} `json:"data"`
	}
	if err := json.NewDecoder(failedCreate.Body).Decode(&failedJob); err != nil || failedJob.Data.Job.ID == "" {
		t.Fatalf("failed export create payload=%#v err=%v", failedJob, err)
	}
	failedClaim, ok, err := exportStore.AcquireNextJob(t.Context(), "api-test-worker", time.Now().UTC(), time.Minute)
	if err != nil || !ok || failedClaim.ID != failedJob.Data.Job.ID {
		t.Fatalf("claim failed export job=%#v ok=%v err=%v", failedClaim, ok, err)
	}
	const failureMessage = "gallery export exceeds the configured size limit"
	if err := exportStore.FailJob(t.Context(), galleryexportservice.FailJobRequest{
		Job: failedClaim, FailedAt: time.Now().UTC(), Disposition: galleryexportservice.FailureTerminal,
		Code: galleryexportservice.ErrorExportTooLarge, Message: failureMessage,
	}); err != nil {
		t.Fatal(err)
	}
	failedStatusPath := "/api/agent/gallery/v1/export-jobs/" + failedJob.Data.Job.ID
	failedStatus := galleryExportGetRequest(handler, owner.AccessToken, failedStatusPath)
	if failedStatus.Code != http.StatusOK || !bytes.Contains(failedStatus.Body.Bytes(), []byte(`"state":"failed"`)) ||
		!bytes.Contains(failedStatus.Body.Bytes(), []byte(`"error_code":"EXPORT_TOO_LARGE"`)) ||
		!bytes.Contains(failedStatus.Body.Bytes(), []byte(`"error_message":"`+failureMessage+`"`)) ||
		bytes.Contains(failedStatus.Body.Bytes(), []byte(`"download_url"`)) {
		t.Fatalf("failed export status=%d body=%s", failedStatus.Code, failedStatus.Body.String())
	}
}

func TestGalleryBatchPublishPreservesModerationOutcomesPerItem(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, "file:gallery-batch-moderation-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	moderation := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Input == "moderation-error" {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if payload.Input == "reject-me" {
			_, _ = w.Write([]byte(`{"results":[{"flagged":true,"categories":{"violence":true}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":[{"flagged":false,"categories":{}}]}`))
	}))
	defer moderation.Close()
	cfg := config.Config{}
	cfg.Providers.OpenAI.Enabled, cfg.Providers.OpenAI.BaseURL, cfg.Providers.OpenAI.APIKey = true, moderation.URL, "test"
	authSvc := authservice.NewService(config.AuthConfig{AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: time.Hour, Issuer: "test", AccessTokenSecret: "secret", RefreshCookieName: "refresh"}, map[string]string{"basic": "1.00000"})
	session := loginExistingAuthUser(t, authSvc, "gallery-batch-moderation@example.com")
	projects := projectservice.NewService(entstore.NewProjectStore(client))
	project, err := projects.EnsureDefault(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	backend := storage.NewLocalBackend(t.TempDir())
	storageRouter := storage.NewStaticRouter(backend)
	taskSvc := imagetaskservice.NewServiceWithProvidersStoreAssetsBillingAndRouter(cfg, nil, entstore.NewImageTaskStore(client), nil, nil, storageRouter)
	taskSvc.SetProjectResolver(projects)
	api := handlers.NewAPIWithTaskService(cfg, authSvc, nil, taskSvc)
	api.SetProjectService(projects)
	handler := NewWithAPI(api)
	allowed := seedGalleryBatchAPIImageWithPrompt(t, client, backend, 1, project.ID, "safe", "", "safe")
	rejected := seedGalleryBatchAPIImageWithPrompt(t, client, backend, 1, project.ID, "reject-me", "", "reject")
	fallback := seedGalleryBatchAPIImageWithPrompt(t, client, backend, 1, project.ID, "moderation-error", "", "fallback")
	rec := galleryBatchRequest(t, handler, session.AccessToken, "/api/agent/gallery/v1/images:batch-publish", map[string]any{"image_ids": []string{allowed, rejected, fallback}, "project_id": project.ID})
	if rec.Code != http.StatusOK {
		t.Fatalf("batch publish status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			Succeeded []struct {
				ID     string `json:"id"`
				Entity struct {
					VisibilityStatus string `json:"visibility_status"`
					ReviewReason     string `json:"review_reason"`
				} `json:"entity"`
			} `json:"succeeded"`
			Failed []any `json:"failed"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data.Succeeded) != 3 || len(response.Data.Failed) != 0 {
		t.Fatalf("batch moderation result=%#v", response.Data)
	}
	statuses := map[string]string{}
	for _, item := range response.Data.Succeeded {
		statuses[item.ID] = item.Entity.VisibilityStatus + ":" + item.Entity.ReviewReason
	}
	if statuses[allowed] != "pending_review:" || statuses[fallback] != "pending_review:" || statuses[rejected] != "rejected:auto_moderation_blocked:violence" {
		t.Fatalf("batch moderation statuses=%#v", statuses)
	}
}

func TestGalleryBatchCancelPublishAuditsOnlySuccessfulItemsAndRecordsMode(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, "file:gallery-batch-cancel-audit-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	authSvc := authservice.NewService(config.AuthConfig{AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: time.Hour, Issuer: "test", AccessTokenSecret: "secret", RefreshCookieName: "refresh"}, map[string]string{"basic": "1.00000"})
	session := loginExistingAuthUser(t, authSvc, "gallery-cancel-audit@example.com")
	projects := projectservice.NewService(entstore.NewProjectStore(client))
	project, err := projects.EnsureDefault(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	backend := storage.NewLocalBackend(t.TempDir())
	storageRouter := storage.NewStaticRouter(backend)
	taskSvc := imagetaskservice.NewServiceWithProvidersStoreAssetsBillingAndRouter(config.Config{}, nil, entstore.NewImageTaskStore(client), nil, nil, storageRouter)
	taskSvc.SetProjectResolver(projects)
	auditStore := auditservice.NewMemoryStore()
	auditSvc := auditservice.NewService(auditStore)
	api := handlers.NewAPIWithCompletionServices(config.Config{}, authSvc, nil, taskSvc, nil, nil, nil, nil, auditSvc)
	api.SetProjectService(projects)
	handler := NewWithAPI(api)
	imageID := seedGalleryBatchAPIImage(t, client, backend, 1, project.ID, "audit", "one")
	rec := galleryBatchRequest(t, handler, session.AccessToken, "/api/agent/gallery/v1/images:batch-publish", map[string]any{
		"image_ids": []string{imageID, uuid.NewString()}, "project_id": project.ID, "publish": false,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel status=%d body=%s", rec.Code, rec.Body.String())
	}
	itemLogs, err := auditSvc.List(t.Context(), domainaudit.ListRequest{Action: "gallery.publish_cancel", PageSize: 10})
	if err != nil || itemLogs.Total != 1 || itemLogs.Items[0].TargetID != imageID {
		t.Fatalf("item audit logs=%#v err=%v", itemLogs, err)
	}
	batchLogs, err := auditSvc.List(t.Context(), domainaudit.ListRequest{Action: "gallery.batch_publish", PageSize: 10})
	if err != nil || batchLogs.Total != 1 || batchLogs.Items[0].Metadata["publish"] != false {
		t.Fatalf("batch audit logs=%#v err=%v", batchLogs, err)
	}
}

func galleryBatchRequest(t *testing.T, handler http.Handler, token, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func galleryExportGetRequest(handler http.Handler, token, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func seedGalleryBatchAPIImage(t *testing.T, client *repoent.Client, backend storage.Backend, userID int64, projectID, group, content string) string {
	return seedGalleryBatchAPIImageWithPrompt(t, client, backend, userID, projectID, "prompt", group, content)
}

func seedGalleryBatchAPIImageWithPrompt(t *testing.T, client *repoent.Client, backend storage.Backend, userID int64, projectID, prompt, group, content string) string {
	t.Helper()
	projectUUID := uuid.MustParse(projectID)
	taskID, imageID := uuid.New(), uuid.New()
	if _, err := client.ImageTask.Create().SetID(taskID).SetUserID(userID).SetProjectID(projectUUID).SetTaskType("text_to_image").SetPrompt(prompt).SetAbstractModel("plus").Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	objectKey := "generated-images/" + imageID.String() + ".png"
	if err := backend.Put(context.Background(), objectKey, "image/png", []byte(content)); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ImageResult.Create().SetID(imageID).SetTaskID(taskID).SetUserID(userID).SetProjectID(projectUUID).
		SetObjectKey(objectKey).SetMimeType("image/png").SetFileSizeBytes(int64(len(content))).SetSha256("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa").SetImageGroup(group).Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	return imageID.String()
}
