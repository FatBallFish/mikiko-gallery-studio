package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	assetservice "github.com/fatballfish/pic-gallery/internal/service/assets"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	mediaassetservice "github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	projectservice "github.com/fatballfish/pic-gallery/internal/service/project"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/google/uuid"
)

func TestUnifiedReferenceUploadIDAndLegacyAliasUseCanonicalMedia(t *testing.T) {
	cfg := taskAPIConfig("http://provider.invalid")
	cfg.Storage.LocalRoot = t.TempDir()
	authSvc, owner := loginTestUser(t, "unified-reference-owner@example.com")
	other := loginExistingAuthUser(t, authSvc, "unified-reference-other@example.com")
	claims, err := authSvc.ParseAccessToken(owner.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	client, err := repoent.Open(dialect.SQLite, "file:"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
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
	backend := storage.NewLocalBackend(cfg.Storage.LocalRoot)
	assets := assetservice.NewServiceWithStoreAndBackend(cfg.Storage, cfg.GenerationLimits, entstore.NewAssetsStore(client), backend)
	taskStore := entstore.NewImageTaskStore(client)
	tasks := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, taskStore, assets, nil)
	tasks.SetProjectResolver(projectservice.NewService(entstore.NewProjectStore(client)))
	media := mediaassetservice.NewService(entstore.NewMediaStore(client), storage.NewStaticRouter(backend), mediaassetservice.Options{Policy: domainmedia.DefaultPolicy(), PartSize: 8 << 20, UploadTTL: time.Hour})
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, assets, tasks, enabledFeatureAdmin(t, "media_upload"), nil)
	api.SetMediaAssetService(media)
	api.SetProjectService(projectservice.NewService(entstore.NewProjectStore(client)))
	handler := NewWithAPI(api)
	content := tinyPNG(t)
	initialized := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodPost, "/api/agent/media/v1/uploads", fmt.Sprintf(`{"project_id":%q,"filename":"subject.png","media_type":"image","mime_type":"image/png","size_bytes":%d}`, project.ID.String(), len(content)), map[string]string{"Idempotency-Key": "unified-reference"})
	if initialized.Code != http.StatusCreated {
		t.Fatalf("upload init = %d %s", initialized.Code, initialized.Body.String())
	}
	decodeID := func(body []byte) string {
		t.Helper()
		var payload struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Data.ID == "" {
			t.Fatalf("missing id: %s", body)
		}
		return payload.Data.ID
	}
	uploadID := decodeID(initialized.Body.Bytes())
	part := authenticatedMediaRequestBytes(t, handler, owner.AccessToken, http.MethodPut, "/api/agent/media/v1/uploads/"+uploadID+"/parts/1", content, map[string]string{"Content-Type": "application/octet-stream", "X-Content-SHA256": mediaSHA256Hex(content)})
	if part.Code != http.StatusOK {
		t.Fatalf("part = %d %s", part.Code, part.Body.String())
	}
	var uploadedPart struct {
		Data struct {
			ETag string `json:"etag"`
		} `json:"data"`
	}
	if err := json.Unmarshal(part.Body.Bytes(), &uploadedPart); err != nil {
		t.Fatal(err)
	}
	completed := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodPost, "/api/agent/media/v1/uploads/"+uploadID+":complete", fmt.Sprintf(`{"parts":[{"part_number":1,"etag":%q}]}`, uploadedPart.Data.ETag), nil)
	if completed.Code != http.StatusCreated {
		t.Fatalf("complete = %d %s", completed.Code, completed.Body.String())
	}
	mediaID := uuid.MustParse(decodeID(completed.Body.Bytes()))
	// The media worker is covered by the isolated smoke; this API test sets its completed state.
	if err := client.MediaAsset.UpdateOneID(mediaID).SetStatus("ready").SetWidth(2).SetHeight(2).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	count, err := client.ReferenceAsset.Query().Count(t.Context())
	if err != nil || count != 0 {
		t.Fatalf("unified upload created aliases: %d, %v", count, err)
	}
	legacyID := uuid.New()
	canonical, err := client.MediaAsset.Get(t.Context(), mediaID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReferenceAsset.Create().SetID(legacyID).SetUserID(claims.UserID).SetMediaAssetID(mediaID).SetName("stale alias").SetObjectKey("stale.png").SetMimeType("image/png").SetSha256(mediaSHA256Hex(content)).SetStatus("ready").Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"ready", "ready_original"} {
		if err := client.MediaAsset.UpdateOneID(mediaID).SetStatus(status).Exec(t.Context()); err != nil {
			t.Fatal(err)
		}
		for _, id := range []uuid.UUID{mediaID, legacyID} {
			t.Run(status+"/"+id.String(), func(t *testing.T) {
				detail := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodGet, "/api/agent/image/v1/reference-assets/"+id.String(), "", nil)
				if detail.Code != http.StatusOK || !bytes.Contains(detail.Body.Bytes(), []byte(id.String())) {
					t.Fatalf("reference detail = %d %s", detail.Code, detail.Body.String())
				}
				ref, err := assets.GetWithContext(t.Context(), claims.UserID, id.String())
				if err != nil || ref.MediaAssetID != mediaID.String() || ref.ObjectKey != canonical.ObjectKey || ref.Status != status {
					t.Fatalf("reference = %#v, %v", ref, err)
				}
				input, err := assets.LoadInput(claims.UserID, id.String())
				if err != nil || !bytes.Equal(input.Data, content) {
					t.Fatalf("load = %#v, %v", input, err)
				}
				body := fmt.Sprintf(`{"project_id":%q,"task_type":"image_edit","prompt":%q,"abstract_model":"plus","size_mode":"auto","requested_output_image_count":1,"reference_asset_ids":[%q],"reference_bindings":[{"name":%q,"asset_id":%q}],"response_mode":"async"}`, project.ID.String(), "edit {{@"+ref.Name+"}}", id.String(), ref.Name, id.String())
				created := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodPost, "/api/agent/image/v1/tasks", body, nil)
				if created.Code != http.StatusAccepted {
					t.Fatalf("image edit = %d %s", created.Code, created.Body.String())
				}
				task, err := taskStore.GetByID(t.Context(), claims.UserID, decodeID(created.Body.Bytes()))
				if err != nil || task.ExecutionPrompt != "edit 图片1" {
					t.Fatalf("stored edit = %#v, %v", task, err)
				}
				foreign := authenticatedMediaRequest(t, handler, other.AccessToken, http.MethodGet, "/api/agent/image/v1/reference-assets/"+id.String(), "", nil)
				if foreign.Code != http.StatusNotFound {
					t.Fatalf("foreign = %d %s", foreign.Code, foreign.Body.String())
				}
			})
		}
	}
	for _, state := range []struct{ status, code string }{{"processing", "MEDIA_ASSET_NOT_READY"}, {"processing_failed", "MEDIA_ASSET_PROCESSING_FAILED"}, {"failed", "MEDIA_ASSET_PROCESSING_FAILED"}, {"deleting", "MEDIA_ASSET_DELETED"}, {"deleted", "MEDIA_ASSET_DELETED"}} {
		if err := client.MediaAsset.UpdateOneID(mediaID).SetStatus(state.status).Exec(t.Context()); err != nil {
			t.Fatal(err)
		}
		for _, id := range []uuid.UUID{mediaID, legacyID} {
			if _, err := assets.LoadInput(claims.UserID, id.String()); err == nil {
				t.Fatalf("%s load allowed %s", state.status, id)
			}
			body := fmt.Sprintf(`{"task_type":"image_edit","prompt":"edit","abstract_model":"plus","reference_asset_ids":[%q],"response_mode":"async"}`, id.String())
			rejected := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodPost, "/api/agent/image/v1/tasks", body, nil)
			if rejected.Code < 400 || !bytes.Contains(rejected.Body.Bytes(), []byte(state.code)) {
				t.Fatalf("%s edit = %d %s", state.status, rejected.Code, rejected.Body.String())
			}
		}
	}
}
