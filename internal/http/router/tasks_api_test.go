package router

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	"github.com/fatballfish/pic-gallery/internal/provider"
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
	assetservice "github.com/fatballfish/pic-gallery/internal/service/assets"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/internal/worker"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

func TestReferenceAssetUploadUsesDynamicAttachmentPolicy(t *testing.T) {
	cfg := taskAPIConfig("http://provider.invalid")
	cfg.Storage.LocalRoot = t.TempDir()
	cfg.GenerationLimits.ReferenceImageMaxMB = 20
	cfg.AttachmentPolicy = config.AttachmentPolicyConfig{ImageMaxMB: 1, ImageAllowedFormats: []string{"png"}}
	adminSvc := adminconfigservice.NewServiceWithStore(cfg, adminconfigservice.NewMemoryStore())
	authSvc, session := loginTestUser(t, "dynamic-attachment-policy@example.com")
	handler := NewWithAPI(handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, adminSvc, nil))

	assertReferenceCapabilityLimit(t, handler, session.AccessToken, 1)
	largePNG := append(tinyPNG(t), make([]byte, 1024*1024)...)
	request := referenceUploadRequest(t, session.AccessToken, largePNG)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), errs.CodeImageReferenceTooLarge) {
		t.Fatalf("expected current 1 MB policy to reject upload: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	if _, err := adminSvc.UpdateTab(context.Background(), domainadminconfig.UpdateTabRequest{
		TabKey: "attachment_policy", Version: 1,
		Items: []domainadminconfig.Item{{
			ConfigCategory: "attachment_policy", ConfigKey: "image_max_mb", ConfigValue: map[string]any{"value": 2}, Scope: "global",
		}},
	}); err != nil {
		t.Fatalf("UpdateTab: %v", err)
	}
	assertReferenceCapabilityLimit(t, handler, session.AccessToken, 2)

	request = referenceUploadRequest(t, session.AccessToken, largePNG)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected updated 2 MB policy to accept upload: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestReferenceAssetRenameAndPromptTemplateTaskContract(t *testing.T) {
	cfg := taskAPIConfig("http://provider.invalid")
	cfg.Storage.LocalRoot = t.TempDir()
	authSvc, session := loginTestUser(t, "prompt-template-api@example.com")
	assetSvc := assetservice.NewService(cfg.Storage, cfg.GenerationLimits)
	asset, err := assetSvc.Upload(1, "subject.png", "image/png", tinyPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	taskStore := imagetaskservice.NewMemoryStore()
	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, taskStore, assetSvc, nil)
	handler := NewWithAPI(handlers.NewAPIWithRuntimeServices(cfg, authSvc, assetSvc, taskSvc, nil, nil))

	renameBody := bytes.NewBufferString(`{"name":"  主体  "}`)
	renameReq := httptest.NewRequest(http.MethodPatch, "/api/agent/image/v1/reference-assets/"+asset.ID, renameBody)
	renameReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	renameReq.Header.Set("Content-Type", "application/json")
	renameRec := httptest.NewRecorder()
	handler.ServeHTTP(renameRec, renameReq)
	if renameRec.Code != http.StatusOK || !strings.Contains(renameRec.Body.String(), `"name":"主体"`) {
		t.Fatalf("rename status=%d body=%s", renameRec.Code, renameRec.Body.String())
	}

	createBody := bytes.NewBufferString(`{
		"task_type":"image_edit",
		"prompt":"让 {{@主体}} 位于 {{$地点}}",
		"abstract_model":"plus",
		"size_mode":"auto",
		"requested_output_image_count":1,
		"reference_asset_ids":["` + asset.ID + `"],
		"reference_bindings":[{"name":"主体","asset_id":"` + asset.ID + `"}],
		"prompt_variables":[{"name":"地点","value":"秘密地点"}],
		"response_mode":"async"
	}`)
	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/image/v1/tasks", createBody)
	createReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusAccepted {
		t.Fatalf("create status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	if strings.Contains(createRec.Body.String(), "秘密地点") || !strings.Contains(createRec.Body.String(), `"prompt":"让 {{@主体}} 位于 {{$地点}}"`) {
		t.Fatalf("create leaked expanded prompt: %s", createRec.Body.String())
	}
	var response struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	stored, err := taskStore.GetByID(t.Context(), 1, response.Data.ID)
	if err != nil || stored.ExecutionPrompt != "让 图片1 位于 秘密地点" {
		t.Fatalf("stored execution prompt=%q err=%v", stored.ExecutionPrompt, err)
	}
}

func assertReferenceCapabilityLimit(t *testing.T, handler http.Handler, token string, wantMB int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/agent/image/v1/capabilities", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("capabilities status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Data struct {
			ReferenceImageMaxMB int `json:"reference_image_max_mb"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode capabilities: %v", err)
	}
	if payload.Data.ReferenceImageMaxMB != wantMB {
		t.Fatalf("capabilities image max = %d, want %d body=%s", payload.Data.ReferenceImageMaxMB, wantMB, rec.Body.String())
	}
}

func referenceUploadRequest(t *testing.T, token string, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "reference.png")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write upload: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/agent/image/v1/reference-assets", &body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestReferenceAssetDownloadAcceptsQueryToken(t *testing.T) {
	cfg := taskAPIConfig("http://provider.invalid")
	cfg.Storage.LocalRoot = t.TempDir()
	authSvc, session := loginTestUser(t, "reference-query-token@example.com")
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, nil, nil)
	handler := NewWithAPI(api)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "reference.png")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	imageBytes := tinyPNG(t)
	if _, err := part.Write(imageBytes); err != nil {
		t.Fatalf("write image: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	uploadReq := httptest.NewRequest(http.MethodPost, "/api/agent/image/v1/reference-assets", &body)
	uploadReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadRec := httptest.NewRecorder()
	handler.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusCreated {
		t.Fatalf("upload reference asset: %d body=%s", uploadRec.Code, uploadRec.Body.String())
	}
	var uploadResp struct {
		Data struct {
			ID          string `json:"id"`
			PreviewURL  string `json:"preview_url"`
			DownloadURL string `json:"download_url"`
		} `json:"data"`
	}
	if err := json.NewDecoder(uploadRec.Body).Decode(&uploadResp); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if uploadResp.Data.ID == "" {
		t.Fatalf("expected reference asset id body=%s", uploadRec.Body.String())
	}
	expectedAssetURL := "/api/agent/image/v1/reference-assets/" + uploadResp.Data.ID + "/download"
	if uploadResp.Data.PreviewURL != expectedAssetURL || uploadResp.Data.DownloadURL != expectedAssetURL {
		t.Fatalf("expected upload response preview/download URLs %q, got preview=%q download=%q", expectedAssetURL, uploadResp.Data.PreviewURL, uploadResp.Data.DownloadURL)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/agent/image/v1/reference-assets/"+uploadResp.Data.ID, nil)
	getReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get reference asset: %d body=%s", getRec.Code, getRec.Body.String())
	}
	var getResp struct {
		Data struct {
			PreviewURL  string `json:"preview_url"`
			DownloadURL string `json:"download_url"`
		} `json:"data"`
	}
	if err := json.NewDecoder(getRec.Body).Decode(&getResp); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if getResp.Data.PreviewURL != expectedAssetURL || getResp.Data.DownloadURL != expectedAssetURL {
		t.Fatalf("expected get response preview/download URLs %q, got preview=%q download=%q", expectedAssetURL, getResp.Data.PreviewURL, getResp.Data.DownloadURL)
	}

	downloadReq := httptest.NewRequest(http.MethodGet, "/api/agent/image/v1/reference-assets/"+uploadResp.Data.ID+"/download?access_token="+session.AccessToken, nil)
	downloadRec := httptest.NewRecorder()
	handler.ServeHTTP(downloadRec, downloadReq)
	if downloadRec.Code != http.StatusOK {
		t.Fatalf("download reference asset with query token: %d body=%s", downloadRec.Code, downloadRec.Body.String())
	}
	if got := downloadRec.Body.Bytes(); !bytes.Equal(got, imageBytes) {
		t.Fatalf("downloaded reference bytes mismatch: got %d bytes want %d", len(got), len(imageBytes))
	}
}

func TestAgentTaskCreateAndQueryEndpoints(t *testing.T) {
	imageBytes := tinyPNG(t)
	var providerServer *httptest.Server
	providerServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"choices":[{"message":{"images":[{"image_url":{"url":"`+providerServer.URL+`/images/task.png"}}]}}]}`)
		case "/images/task.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(imageBytes)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer providerServer.Close()

	cfg := taskAPIConfig(providerServer.URL)
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("task@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	_, session, err := loginAuthUserWithPasswordSetup(t, authSvc, "task@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}

	billingSvc := billingservice.NewService(cfg.Billing)
	if _, err := billingSvc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{UserID: 1, ChangePoints: "100.00000", Reason: "seed balance"}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}
	taskStore := imagetaskservice.NewMemoryStore()
	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, taskStore, nil, billingSvc)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, taskSvc, nil, billingSvc)
	handler := NewWithAPI(api)

	createBody := bytes.NewBufferString(`{"task_type":"text_to_image","prompt":"Generate a banner","abstract_model":"plus","base_resolution":"auto","requested_size":"1536x1024","requested_output_image_count":1,"reference_image_count":0,"response_mode":"async"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/image/v1/tasks", createBody)
	createReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data struct {
			ID              string `json:"id"`
			Status          string `json:"status"`
			ProgressStage   string `json:"progress_stage"`
			ProgressMessage string `json:"progress_message"`
			Results         []struct {
				URL string `json:"url"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createResp.Data.ID == "" || createResp.Data.Status != "queued" {
		t.Fatalf("unexpected create response %#v", createResp)
	}
	if createResp.Data.ProgressStage != "queued" || createResp.Data.ProgressMessage == "" {
		t.Fatalf("expected backend progress fields on create response, got %#v", createResp.Data)
	}

	claimed, ok, err := taskSvc.AcquireNextTask(context.Background(), "router-test-worker", 30*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextTask: %v", err)
	}
	if !ok {
		t.Fatal("expected queued task to be claimed")
	}
	claimed.ProgressStage = ""
	claimed.ProgressMessage = ""
	if err := taskStore.Save(context.Background(), claimed); err != nil {
		t.Fatalf("save legacy running task: %v", err)
	}

	runningReq := httptest.NewRequest(http.MethodGet, "/api/agent/image/v1/tasks/"+createResp.Data.ID, nil)
	runningReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	runningRec := httptest.NewRecorder()
	handler.ServeHTTP(runningRec, runningReq)
	if runningRec.Code != http.StatusOK {
		t.Fatalf("expected running detail 200, got %d body=%s", runningRec.Code, runningRec.Body.String())
	}
	var runningResp struct {
		Data struct {
			ProgressStage string `json:"progress_stage"`
		} `json:"data"`
	}
	if err := json.NewDecoder(runningRec.Body).Decode(&runningResp); err != nil {
		t.Fatalf("decode running detail: %v", err)
	}
	if runningResp.Data.ProgressStage != "provider" {
		t.Fatalf("expected legacy running task to fall back to provider, got %q", runningResp.Data.ProgressStage)
	}

	_, err = taskSvc.ExecuteLeasedTask(context.Background(), claimed, "router-test-worker", []string{"openrouter"})
	if err != nil {
		t.Fatalf("ExecuteLeasedTask: %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/agent/image/v1/tasks", nil)
	listReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listResp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResp.Data) != 1 || listResp.Data[0].ID != createResp.Data.ID {
		t.Fatalf("unexpected list response %#v", listResp)
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/api/agent/image/v1/tasks/"+createResp.Data.ID, nil)
	detailReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	detailRec := httptest.NewRecorder()
	handler.ServeHTTP(detailRec, detailReq)

	if detailRec.Code != http.StatusOK {
		t.Fatalf("expected detail 200, got %d body=%s", detailRec.Code, detailRec.Body.String())
	}
	var detailResp struct {
		Data struct {
			ID              string `json:"id"`
			Status          string `json:"status"`
			ProgressStage   string `json:"progress_stage"`
			ProgressMessage string `json:"progress_message"`
			Results         []struct {
				URL string `json:"url"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.NewDecoder(detailRec.Body).Decode(&detailResp); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if detailResp.Data.ID != createResp.Data.ID || detailResp.Data.Status != "succeeded" || len(detailResp.Data.Results) != 1 || detailResp.Data.Results[0].URL == "" {
		t.Fatalf("unexpected detail response %#v", detailResp)
	}
	if detailResp.Data.ProgressStage != "completed" || detailResp.Data.ProgressMessage == "" {
		t.Fatalf("expected backend completion progress fields, got %#v", detailResp.Data)
	}

	eventsReq := httptest.NewRequest(http.MethodGet, "/api/agent/image/v1/tasks/"+createResp.Data.ID+"/events", nil)
	eventsReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	eventsRec := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
	handler.ServeHTTP(eventsRec, eventsReq)

	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected events 200, got %d body=%s", eventsRec.Code, eventsRec.Body.String())
	}
	if got := eventsRec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("expected SSE content type, got %q", got)
	}
	if body := eventsRec.Body.String(); !strings.Contains(body, "event: task") || !strings.Contains(body, `"status":"succeeded"`) || !strings.Contains(body, `"progress_stage":"completed"`) {
		t.Fatalf("unexpected SSE body %q", body)
	}

	streamReq := httptest.NewRequest(http.MethodGet, "/api/agent/image/v1/tasks/events?once=true", nil)
	streamReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	streamRec := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
	handler.ServeHTTP(streamRec, streamReq)

	if streamRec.Code != http.StatusOK {
		t.Fatalf("expected stream 200, got %d body=%s", streamRec.Code, streamRec.Body.String())
	}
	if got := streamRec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("expected stream SSE content type, got %q", got)
	}
	if body := streamRec.Body.String(); !strings.Contains(body, "event: history") || !strings.Contains(body, createResp.Data.ID) || !strings.Contains(body, `"progress_message"`) {
		t.Fatalf("unexpected stream body %q", body)
	}

	historyListReq := httptest.NewRequest(http.MethodGet, "/api/agent/image/v1/history/tasks", nil)
	historyListReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	historyListRec := httptest.NewRecorder()
	handler.ServeHTTP(historyListRec, historyListReq)

	if historyListRec.Code != http.StatusOK {
		t.Fatalf("expected history list 200, got %d body=%s", historyListRec.Code, historyListRec.Body.String())
	}

	privateGalleryReq := httptest.NewRequest(http.MethodGet, "/api/agent/gallery/v1/images?page=1&page_size=10", nil)
	privateGalleryReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	privateGalleryRec := httptest.NewRecorder()
	handler.ServeHTTP(privateGalleryRec, privateGalleryReq)
	if privateGalleryRec.Code != http.StatusOK {
		t.Fatalf("expected private gallery 200, got %d body=%s", privateGalleryRec.Code, privateGalleryRec.Body.String())
	}
	var privateGalleryResp struct {
		Data struct {
			Items []struct {
				ID          string `json:"id"`
				TaskID      string `json:"task_id"`
				TaskType    string `json:"task_type"`
				TaskStatus  string `json:"task_status"`
				DownloadURL string `json:"download_url"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(privateGalleryRec.Body).Decode(&privateGalleryResp); err != nil {
		t.Fatalf("decode private gallery response: %v", err)
	}
	if len(privateGalleryResp.Data.Items) != 1 || privateGalleryResp.Data.Items[0].TaskID != createResp.Data.ID || privateGalleryResp.Data.Items[0].TaskStatus != "succeeded" || privateGalleryResp.Data.Items[0].DownloadURL == "" {
		t.Fatalf("unexpected private gallery response %#v", privateGalleryResp)
	}

	privateGallerySlashReq := httptest.NewRequest(http.MethodGet, "/api/agent/gallery/v1/images/?page=1&page_size=10", nil)
	privateGallerySlashReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	privateGallerySlashRec := httptest.NewRecorder()
	handler.ServeHTTP(privateGallerySlashRec, privateGallerySlashReq)
	if privateGallerySlashRec.Code != http.StatusOK {
		t.Fatalf("expected private gallery with trailing slash 200, got %d body=%s", privateGallerySlashRec.Code, privateGallerySlashRec.Body.String())
	}

	historyDetailReq := httptest.NewRequest(http.MethodGet, "/api/agent/image/v1/history/tasks/"+createResp.Data.ID, nil)
	historyDetailReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	historyDetailRec := httptest.NewRecorder()
	handler.ServeHTTP(historyDetailRec, historyDetailReq)

	if historyDetailRec.Code != http.StatusOK {
		t.Fatalf("expected history detail 200, got %d body=%s", historyDetailRec.Code, historyDetailRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/agent/image/v1/history/tasks/"+createResp.Data.ID, nil)
	deleteReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected delete 204, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	historyListRec = httptest.NewRecorder()
	handler.ServeHTTP(historyListRec, historyListReq)
	if historyListRec.Code != http.StatusOK {
		t.Fatalf("expected history list after delete 200, got %d body=%s", historyListRec.Code, historyListRec.Body.String())
	}
	var historyListResp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(historyListRec.Body).Decode(&historyListResp); err != nil {
		t.Fatalf("decode history list response: %v", err)
	}
	if len(historyListResp.Data) != 0 {
		t.Fatalf("expected empty history list after delete, got %#v", historyListResp)
	}

	historyDetailRec = httptest.NewRecorder()
	handler.ServeHTTP(historyDetailRec, historyDetailReq)
	if historyDetailRec.Code != http.StatusNotFound {
		t.Fatalf("expected history detail 404 after delete, got %d body=%s", historyDetailRec.Code, historyDetailRec.Body.String())
	}
}

type flushRecorder struct {
	*httptest.ResponseRecorder
}

func (r *flushRecorder) Flush() {}

func TestAgentTaskCreateRejectsSyncResponseMode(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("sync-mode@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	_, session, err := loginAuthUserWithPasswordSetup(t, authSvc, "sync-mode@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}
	billingSvc := billingservice.NewService(cfg.Billing)
	if _, err := billingSvc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{UserID: 1, ChangePoints: "100.00000", Reason: "seed balance"}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}
	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), nil, billingSvc)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, taskSvc, nil, billingSvc)
	handler := NewWithAPI(api)

	req := httptest.NewRequest(http.MethodPost, "/api/agent/image/v1/tasks", bytes.NewBufferString(`{"task_type":"text_to_image","prompt":"Generate a banner","abstract_model":"plus","base_resolution":"auto","requested_size":"1536x1024","requested_output_image_count":1,"response_mode":"sync"}`))
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected sync response_mode rejection 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("only supports async")) {
		t.Fatalf("expected clear async-only error, got body=%s", rec.Body.String())
	}
}

func TestAgentTaskB64ResultPersistsAndDownloadsLocalImage(t *testing.T) {
	imageBytes := tinyPNG(t)
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"images": []map[string]any{{
						"image_url": map[string]string{
							"url": "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageBytes),
						},
					}},
				},
			}},
		})
	}))
	defer providerServer.Close()

	cfg := taskAPIConfig(providerServer.URL)
	cfg.Storage.LocalRoot = t.TempDir()
	authSvc, session := loginTestUser(t, "download-owner@example.com")
	billingSvc := billingservice.NewService(cfg.Billing)
	if _, err := billingSvc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{UserID: 1, ChangePoints: "100.00000", Reason: "seed balance"}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}
	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), nil, billingSvc)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, taskSvc, nil, billingSvc)
	handler := NewWithAPI(api)

	taskID := createAndProcessAgentTask(t, handler, taskSvc, session.AccessToken)
	detailReq := httptest.NewRequest(http.MethodGet, "/api/agent/image/v1/tasks/"+taskID, nil)
	detailReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	detailRec := httptest.NewRecorder()
	handler.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("expected detail 200, got %d body=%s", detailRec.Code, detailRec.Body.String())
	}
	var detailResp struct {
		Data struct {
			Results []struct {
				ID               string `json:"id"`
				URL              string `json:"url"`
				DownloadURL      string `json:"download_url"`
				MimeType         string `json:"mime_type"`
				Width            int    `json:"width"`
				Height           int    `json:"height"`
				FileSizeBytes    int64  `json:"file_size_bytes"`
				VisibilityStatus string `json:"visibility_status"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.NewDecoder(detailRec.Body).Decode(&detailResp); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if len(detailResp.Data.Results) != 1 {
		t.Fatalf("expected one result, got %#v", detailResp)
	}
	result := detailResp.Data.Results[0]
	if result.ID == "" || result.DownloadURL == "" || result.URL == "" {
		t.Fatalf("expected local result to expose id, url, and download_url, got %#v", result)
	}
	if result.MimeType != "image/png" || result.Width != 2 || result.Height != 1 || result.FileSizeBytes != int64(len(imageBytes)) || result.VisibilityStatus != "private" {
		t.Fatalf("unexpected local result metadata %#v", result)
	}

	downloadReq := httptest.NewRequest(http.MethodGet, result.DownloadURL, nil)
	downloadReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	downloadRec := httptest.NewRecorder()
	handler.ServeHTTP(downloadRec, downloadReq)
	if downloadRec.Code != http.StatusOK {
		t.Fatalf("expected download 200, got %d body=%s", downloadRec.Code, downloadRec.Body.String())
	}
	if got := downloadRec.Body.Bytes(); !bytes.Equal(got, imageBytes) {
		t.Fatalf("downloaded bytes mismatch: got %d bytes want %d", len(got), len(imageBytes))
	}
	if contentType := downloadRec.Header().Get("Content-Type"); contentType != "image/png" {
		t.Fatalf("expected image/png content type, got %q", contentType)
	}

	queryTokenReq := httptest.NewRequest(http.MethodGet, result.DownloadURL+"?access_token="+session.AccessToken, nil)
	queryTokenRec := httptest.NewRecorder()
	handler.ServeHTTP(queryTokenRec, queryTokenReq)
	if queryTokenRec.Code != http.StatusOK {
		t.Fatalf("expected query-token download 200, got %d body=%s", queryTokenRec.Code, queryTokenRec.Body.String())
	}
	if got := queryTokenRec.Body.Bytes(); !bytes.Equal(got, imageBytes) {
		t.Fatalf("query-token downloaded bytes mismatch: got %d bytes want %d", len(got), len(imageBytes))
	}

	otherSession := loginExistingAuthUser(t, authSvc, "download-other@example.com")
	otherReq := httptest.NewRequest(http.MethodGet, result.DownloadURL, nil)
	otherReq.Header.Set("Authorization", "Bearer "+otherSession.AccessToken)
	otherRec := httptest.NewRecorder()
	handler.ServeHTTP(otherRec, otherReq)
	if otherRec.Code != http.StatusNotFound {
		t.Fatalf("expected non-owner download 404, got %d body=%s", otherRec.Code, otherRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/agent/image/v1/history/tasks/"+taskID, nil)
	deleteReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected delete 204, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	deletedDownloadReq := httptest.NewRequest(http.MethodGet, result.DownloadURL, nil)
	deletedDownloadReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	deletedDownloadRec := httptest.NewRecorder()
	handler.ServeHTTP(deletedDownloadRec, deletedDownloadReq)
	if deletedDownloadRec.Code != http.StatusNotFound {
		t.Fatalf("expected deleted task image download 404, got %d body=%s", deletedDownloadRec.Code, deletedDownloadRec.Body.String())
	}
}

func TestAgentImageDownloadRedirectsToOwnedTemporaryStorageURL(t *testing.T) {
	cfg := taskAPIConfig("http://provider.invalid")
	authSvc, session := loginTestUser(t, "signed-download-owner@example.com")
	store := imagetaskservice.NewMemoryStore()
	result := provider.ImageResult{
		ID: "signed-image", StorageDriver: "s3", StorageConfigID: "bfss-primary",
		ObjectKey: "generated/signed-image.png", MimeType: "image/png", VisibilityStatus: "private",
	}
	if err := store.Save(t.Context(), domainimagetask.Task{
		UserID: 1, ID: "signed-task", Status: domainimagetask.StatusSucceeded, Results: []provider.ImageResult{result},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	backend := &temporaryRedirectBackend{location: "https://bfss.example.com/bucket/generated/signed-image.png?X-Amz-Signature=signed"}
	taskSvc := imagetaskservice.NewServiceWithProvidersStoreAssetsBillingAndBackend(cfg, nil, store, nil, nil, backend)
	handler := NewWithAPI(handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, taskSvc, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/agent/image/v1/images/"+result.ID, nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected temporary redirect 307, got %d body=%s", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); location != backend.location {
		t.Fatalf("Location = %q, want %q", location, backend.location)
	}
	if cacheControl := rec.Header().Get("Cache-Control"); cacheControl != "private, no-store" {
		t.Fatalf("Cache-Control = %q, want private, no-store", cacheControl)
	}
	if rec.Body.Len() != 0 || backend.getCalls != 0 || backend.signCalls != 2 {
		t.Fatalf("redirect response body=%q getCalls=%d signCalls=%d", rec.Body.String(), backend.getCalls, backend.signCalls)
	}
}

type temporaryRedirectBackend struct {
	location  string
	getCalls  int
	signCalls int
}

func (backend *temporaryRedirectBackend) Driver() string { return "s3" }
func (backend *temporaryRedirectBackend) Put(context.Context, string, string, []byte) error {
	return nil
}
func (backend *temporaryRedirectBackend) Get(context.Context, string) ([]byte, error) {
	backend.getCalls++
	return []byte("must not proxy signed storage bytes"), nil
}
func (backend *temporaryRedirectBackend) Delete(context.Context, string) error { return nil }
func (backend *temporaryRedirectBackend) TemporaryGetURL(context.Context, string, storage.TemporaryGetURLOptions) (string, error) {
	backend.signCalls++
	return backend.location, nil
}

func TestReferenceAssetsImportFromGalleryCreatesDownloadableReference(t *testing.T) {
	imageBytes := tinyPNG(t)
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"images": []map[string]any{{
						"image_url": map[string]string{
							"url": "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageBytes),
						},
					}},
				},
			}},
		})
	}))
	defer providerServer.Close()

	cfg := taskAPIConfig(providerServer.URL)
	cfg.Storage.LocalRoot = t.TempDir()
	authSvc, session := loginTestUser(t, "gallery-import@example.com")
	billingSvc := billingservice.NewService(cfg.Billing)
	if _, err := billingSvc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{UserID: 1, ChangePoints: "100.00000", Reason: "seed balance"}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}
	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), nil, billingSvc)
	assetSvc := assetservice.NewService(cfg.Storage, cfg.GenerationLimits)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, assetSvc, taskSvc, nil, billingSvc)
	handler := NewWithAPI(api)

	taskID := createAndProcessAgentTask(t, handler, taskSvc, session.AccessToken)
	detailReq := httptest.NewRequest(http.MethodGet, "/api/agent/image/v1/tasks/"+taskID, nil)
	detailReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	detailRec := httptest.NewRecorder()
	handler.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("expected detail 200, got %d body=%s", detailRec.Code, detailRec.Body.String())
	}
	var detailResp struct {
		Data struct {
			Results []struct {
				ID string `json:"id"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.NewDecoder(detailRec.Body).Decode(&detailResp); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if len(detailResp.Data.Results) != 1 || detailResp.Data.Results[0].ID == "" {
		t.Fatalf("expected generated image id, got %#v", detailResp)
	}

	importBody := bytes.NewBufferString(`{"gallery_image_ids":["` + detailResp.Data.Results[0].ID + `"]}`)
	importReq := httptest.NewRequest(http.MethodPost, "/api/agent/image/v1/reference-assets:import-from-gallery", importBody)
	importReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	importReq.Header.Set("Content-Type", "application/json")
	importRec := httptest.NewRecorder()
	handler.ServeHTTP(importRec, importReq)
	if importRec.Code != http.StatusCreated {
		t.Fatalf("expected import 201, got %d body=%s", importRec.Code, importRec.Body.String())
	}
	var importResp struct {
		Data struct {
			Items []struct {
				ID          string `json:"id"`
				PreviewURL  string `json:"preview_url"`
				DownloadURL string `json:"download_url"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(importRec.Body).Decode(&importResp); err != nil {
		t.Fatalf("decode import response: %v", err)
	}
	if len(importResp.Data.Items) != 1 || importResp.Data.Items[0].ID == "" || importResp.Data.Items[0].DownloadURL == "" || importResp.Data.Items[0].PreviewURL == "" {
		t.Fatalf("expected imported reference asset URLs, got %#v", importResp.Data.Items)
	}

	downloadReq := httptest.NewRequest(http.MethodGet, importResp.Data.Items[0].DownloadURL+"?access_token="+session.AccessToken, nil)
	downloadRec := httptest.NewRecorder()
	handler.ServeHTTP(downloadRec, downloadReq)
	if downloadRec.Code != http.StatusOK {
		t.Fatalf("expected imported reference download 200, got %d body=%s", downloadRec.Code, downloadRec.Body.String())
	}
	if got := downloadRec.Body.Bytes(); !bytes.Equal(got, imageBytes) {
		t.Fatalf("downloaded imported reference bytes mismatch: got %d bytes want %d", len(got), len(imageBytes))
	}
}

func TestAgentTaskRemoteResultIsMirroredAndDownloadable(t *testing.T) {
	imageBytes := tinyPNG(t)
	var providerServer *httptest.Server
	providerServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"choices":[{"message":{"images":[{"image_url":{"url":"`+providerServer.URL+`/images/task.png"}}]}}]}`)
		case "/images/task.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(imageBytes)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer providerServer.Close()

	cfg := taskAPIConfig(providerServer.URL)
	authSvc, session := loginTestUser(t, "download-remote@example.com")
	billingSvc := billingservice.NewService(cfg.Billing)
	if _, err := billingSvc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{UserID: 1, ChangePoints: "100.00000", Reason: "seed balance"}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}
	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), nil, billingSvc)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, taskSvc, nil, billingSvc)
	handler := NewWithAPI(api)

	taskID := createAndProcessAgentTask(t, handler, taskSvc, session.AccessToken)
	detailReq := httptest.NewRequest(http.MethodGet, "/api/agent/image/v1/tasks/"+taskID, nil)
	detailReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	detailRec := httptest.NewRecorder()
	handler.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("expected detail 200, got %d body=%s", detailRec.Code, detailRec.Body.String())
	}
	var detailResp struct {
		Data struct {
			Results []map[string]any `json:"results"`
		} `json:"data"`
	}
	if err := json.NewDecoder(detailRec.Body).Decode(&detailResp); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if len(detailResp.Data.Results) != 1 {
		t.Fatalf("expected one remote result, got %#v", detailResp)
	}
	result := detailResp.Data.Results[0]
	id, _ := result["id"].(string)
	url, _ := result["url"].(string)
	downloadURL, _ := result["download_url"].(string)
	if id == "" || url == "" {
		t.Fatalf("expected mirrored result to expose persisted id and url, got %#v", result)
	}
	for _, key := range []string{"mime_type", "file_size_bytes", "width", "height", "visibility_status"} {
		if _, ok := result[key]; !ok {
			t.Fatalf("expected remote result JSON to include %q, got %#v", key, result)
		}
	}
	if downloadURL == "" {
		t.Fatalf("expected mirrored remote result to expose download_url, got %#v", result)
	}

	downloadReq := httptest.NewRequest(http.MethodGet, downloadURL, nil)
	downloadReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	downloadRec := httptest.NewRecorder()
	handler.ServeHTTP(downloadRec, downloadReq)
	if downloadRec.Code != http.StatusOK {
		t.Fatalf("expected mirrored result download 200, got %d body=%s", downloadRec.Code, downloadRec.Body.String())
	}
	if got := downloadRec.Body.Bytes(); !bytes.Equal(got, imageBytes) {
		t.Fatalf("downloaded bytes mismatch: got %d bytes want %d", len(got), len(imageBytes))
	}
}

func TestAgentHistoryTaskRetryCreatesQueuedTask(t *testing.T) {
	cfg := taskAPIConfig("http://provider.invalid")
	authSvc, session := loginTestUser(t, "retry-task@example.com")
	store := imagetaskservice.NewMemoryStore()
	taskSvc := imagetaskservice.NewServiceWithProvidersAndStore(cfg, nil, store)
	failed := domainimagetask.Task{
		UserID:           1,
		ID:               "22222222-2222-2222-2222-222222222222",
		Status:           domainimagetask.StatusFailed,
		AbstractModel:    "plus",
		TaskType:         "text_to_image",
		Prompt:           "Retry from history",
		BaseResolution:   "auto",
		RequestedSize:    "auto",
		AspectRatio:      "1:1",
		OutputImageCount: 1,
		ResponseMode:     "async",
		SavePolicy:       "private",
		ErrorCode:        "IMAGE_TASK_FAILED",
		ErrorMessage:     "upstream failed",
	}
	if err := store.Save(context.Background(), failed); err != nil {
		t.Fatalf("Save failed task: %v", err)
	}
	handler := NewWithAPI(handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, taskSvc, nil, nil))

	retryReq := httptest.NewRequest(http.MethodPost, "/api/agent/image/v1/history/tasks/"+failed.ID+"/retry", nil)
	retryReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	retryRec := httptest.NewRecorder()
	handler.ServeHTTP(retryRec, retryReq)
	if retryRec.Code != http.StatusAccepted {
		t.Fatalf("expected retry 202, got %d body=%s", retryRec.Code, retryRec.Body.String())
	}
	var retryResp struct {
		Data struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Prompt string `json:"prompt"`
		} `json:"data"`
	}
	if err := json.NewDecoder(retryRec.Body).Decode(&retryResp); err != nil {
		t.Fatalf("decode retry response: %v", err)
	}
	if retryResp.Data.ID == "" || retryResp.Data.ID == failed.ID || retryResp.Data.Status != domainimagetask.StatusQueued || retryResp.Data.Prompt != failed.Prompt {
		t.Fatalf("unexpected retry response %#v", retryResp.Data)
	}
}

func taskAPIConfig(openrouterBaseURL string) config.Config {
	cfg := config.Config{}
	cfg.Billing.AutoBaseResolutionDefaultByGroup = map[string]string{"plus": "2k"}
	cfg.Billing.BaseResolutionPointsByModel = map[string]map[string]string{
		"plus": {"1k": "5.00000", "2k": "8.00000", "4k": "16.00000"},
	}
	cfg.Billing.UserGroupMultipliers = map[string]string{"basic": "1.00000", "plus": "1.00000"}
	cfg.Billing.TaskMultipliers = map[string]string{"text_to_image": "1.00000", "image_edit": "1.25000"}
	cfg.Cashier.OrderTimeoutSeconds = 1800
	cfg.Cashier.MaxPendingOrdersPerUser = 3
	cfg.GenerationLimits.MaxImageCount = 5
	cfg.GenerationLimits.ReferenceImageMaxCount = 4
	cfg.Providers.OpenRouter.Enabled = true
	cfg.Providers.OpenRouter.BaseURL = openrouterBaseURL
	cfg.Providers.OpenRouter.APIKey = "or-key"
	cfg.Providers.OpenAI.Enabled = true
	cfg.Providers.OpenAI.BaseURL = "http://127.0.0.1:1"
	cfg.Providers.OpenAI.APIKey = "oa-key"
	cfg.Routing.ProviderCapabilities = map[string]config.ProviderCapabilityConfig{
		"openrouter": {
			SupportedModels:         []string{"plus"},
			SupportedTaskTypes:      []string{"text_to_image", "image_edit"},
			SupportedBaseResolution: []string{"1k", "2k", "4k"},
			SupportedAspectRatios:   []string{"1:1", "4:3", "16:9"},
			MaxImageCount:           5,
			MaxReferenceImageCount:  4,
			SupportsImageInput:      true,
			SupportsMask:            false,
			Priority:                1,
		},
		"openai": {
			SupportedModels:         []string{"plus"},
			SupportedTaskTypes:      []string{"text_to_image", "image_edit"},
			SupportedBaseResolution: []string{"1k", "2k", "4k"},
			SupportedAspectRatios:   []string{"1:1", "4:3", "16:9"},
			MaxImageCount:           5,
			MaxReferenceImageCount:  4,
			SupportsImageInput:      true,
			SupportsMask:            true,
			Priority:                2,
		},
	}
	cfg.Routing.ProviderModelMap = map[string]map[string]string{
		"plus": {"openrouter": "openrouter/vision", "openai": "gpt-image-1"},
	}
	cfg.App.Name = "pic-gallery"
	return cfg
}

func loginTestUser(t *testing.T, email string) (*authservice.Service, domainauthSession) {
	t.Helper()
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	return authSvc, loginExistingAuthUser(t, authSvc, email)
}

func loginExistingAuthUser(t *testing.T, authSvc *authservice.Service, email string) domainauthSession {
	t.Helper()
	if err := authSvc.SendEmailCode(email, "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	_, session, err := loginAuthUserWithPasswordSetup(t, authSvc, email, "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}
	return domainauthSession{AccessToken: session.AccessToken}
}

type domainauthSession struct {
	AccessToken string
}

func createAndProcessAgentTask(t *testing.T, handler http.Handler, taskSvc *imagetaskservice.Service, accessToken string) string {
	t.Helper()
	createBody := bytes.NewBufferString(`{"task_type":"text_to_image","prompt":"Generate a downloadable banner","abstract_model":"plus","base_resolution":"auto","requested_size":"1536x1024","requested_output_image_count":1,"response_mode":"async"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/image/v1/tasks", createBody)
	createReq.Header.Set("Authorization", "Bearer "+accessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusAccepted {
		t.Fatalf("expected create 202, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	runner := worker.NewRunner(taskSvc, worker.Config{
		Owner:              "router-test-worker",
		LeaseTTL:           30 * time.Second,
		PreferredProviders: []string{"openrouter"},
	})
	processed, err := runner.ProcessOnce(createReq.Context())
	if err != nil {
		t.Fatalf("ProcessOnce: %v", err)
	}
	if !processed {
		t.Fatal("expected queued task to be processed by worker")
	}
	return createResp.Data.ID
}

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 0, color.RGBA{B: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestRedeemCodeRejectsUnknownCodeWithoutCreditingBalance(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("redeem@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	user, session, err := loginAuthUserWithPasswordSetup(t, authSvc, "redeem@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}

	billingSvc := billingservice.NewService(cfg.Billing)
	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), nil, billingSvc)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, taskSvc, nil, billingSvc)
	handler := NewWithAPI(api)

	redeemReq := httptest.NewRequest(http.MethodPost, "/api/agent/billing/v1/redeem-codes/redeem", bytes.NewBufferString(`{"code":"ANYTHING"}`))
	redeemReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	redeemReq.Header.Set("Content-Type", "application/json")
	redeemReq.Header.Set("Idempotency-Key", "redeem-unknown")
	redeemRec := httptest.NewRecorder()
	handler.ServeHTTP(redeemRec, redeemReq)
	if redeemRec.Code != http.StatusNotFound {
		t.Fatalf("expected unknown redeem code 404, got %d body=%s", redeemRec.Code, redeemRec.Body.String())
	}

	balance, err := billingSvc.GetBalance(context.Background(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance.AvailablePoints != "0.00000" || balance.FrozenPoints != "0.00000" {
		t.Fatalf("expected redeem failure to leave balance unchanged, got %#v", balance)
	}
}

func TestAgentTaskCreateQueuesTaskForWorker(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("queue@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	_, session, err := loginAuthUserWithPasswordSetup(t, authSvc, "queue@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}

	billingSvc := billingservice.NewService(cfg.Billing)
	if _, err := billingSvc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{UserID: 1, ChangePoints: "100.00000", Reason: "seed balance"}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}
	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), nil, billingSvc)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, taskSvc, nil, billingSvc)
	handler := NewWithAPI(api)

	createBody := bytes.NewBufferString(`{"task_type":"text_to_image","prompt":"Queue a banner","abstract_model":"plus","base_resolution":"auto","requested_size":"1536x1024","requested_output_image_count":1,"response_mode":"async"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/image/v1/tasks", createBody)
	createReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createResp.Data.ID == "" || createResp.Data.Status != "queued" {
		t.Fatalf("expected queued create response, got %#v", createResp)
	}
}

func TestAgentBillingBalanceAndLedgerEndpoints(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("billing@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	_, session, err := loginAuthUserWithPasswordSetup(t, authSvc, "billing@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}

	billingSvc := billingservice.NewService(cfg.Billing)
	if _, err := billingSvc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{UserID: 1, ChangePoints: "50.00000", Reason: "seed balance"}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}
	if _, err := billingSvc.ReserveTask(context.Background(), domainbilling.ReserveRequest{UserID: 1, TaskID: "44444444-4444-4444-4444-444444444444", EstimatedPoints: "8.00000", Reason: "reserve"}); err != nil {
		t.Fatalf("ReserveTask: %v", err)
	}
	if _, err := billingSvc.FinalizeTask(context.Background(), domainbilling.FinalizeRequest{UserID: 1, TaskID: "44444444-4444-4444-4444-444444444444", EstimatedPoints: "8.00000", ActualPoints: "5.00000", Reason: "finalize"}); err != nil {
		t.Fatalf("FinalizeTask: %v", err)
	}

	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), nil, billingSvc)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, taskSvc, nil, billingSvc)
	handler := NewWithAPI(api)

	balanceReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/balance", nil)
	balanceReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	balanceRec := httptest.NewRecorder()
	handler.ServeHTTP(balanceRec, balanceReq)
	if balanceRec.Code != http.StatusOK {
		t.Fatalf("expected balance 200, got %d body=%s", balanceRec.Code, balanceRec.Body.String())
	}
	var balanceResp struct {
		Data struct {
			AvailablePoints string `json:"available_points"`
			FrozenPoints    string `json:"frozen_points"`
		} `json:"data"`
		Meta struct {
			RequestID string `json:"request_id"`
		} `json:"meta"`
	}
	if err := json.NewDecoder(balanceRec.Body).Decode(&balanceResp); err != nil {
		t.Fatalf("decode balance response: %v", err)
	}
	if balanceResp.Data.AvailablePoints != "45.00000" || balanceResp.Data.FrozenPoints != "0.00000" {
		t.Fatalf("unexpected balance response %#v", balanceResp)
	}
	if balanceResp.Meta.RequestID == "" || balanceRec.Header().Get("X-Request-Id") == "" {
		t.Fatalf("expected balance response to include request id metadata, got body=%#v headers=%v", balanceResp, balanceRec.Header())
	}

	ledgerReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/ledger?page=1&page_size=10", nil)
	ledgerReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	ledgerRec := httptest.NewRecorder()
	handler.ServeHTTP(ledgerRec, ledgerReq)
	if ledgerRec.Code != http.StatusOK {
		t.Fatalf("expected ledger 200, got %d body=%s", ledgerRec.Code, ledgerRec.Body.String())
	}
	var ledgerResp struct {
		Data struct {
			Items []struct {
				ID           int64  `json:"id"`
				TaskID       string `json:"task_id"`
				LedgerType   string `json:"ledger_type"`
				ChangePoints string `json:"change_points"`
				BalanceAfter string `json:"balance_after"`
				FrozenAfter  string `json:"frozen_after"`
				BucketType   string `json:"bucket_type"`
				SourceType   string `json:"source_type"`
				Reason       string `json:"reason"`
				Title        string `json:"title"`
				OccurredAt   string `json:"occurred_at"`
				Amount       string `json:"amount"`
				Type         string `json:"type"`
				Detail       string `json:"detail"`
			} `json:"items"`
			Pagination struct {
				Page     int `json:"page"`
				PageSize int `json:"page_size"`
				Total    int `json:"total"`
			} `json:"pagination"`
		} `json:"data"`
		Meta struct {
			RequestID string `json:"request_id"`
		} `json:"meta"`
	}
	if err := json.NewDecoder(ledgerRec.Body).Decode(&ledgerResp); err != nil {
		t.Fatalf("decode ledger response: %v", err)
	}
	if ledgerResp.Data.Pagination.Page != 1 || ledgerResp.Data.Pagination.PageSize != 10 || ledgerResp.Data.Pagination.Total != 4 || len(ledgerResp.Data.Items) != 4 {
		t.Fatalf("unexpected ledger pagination %#v", ledgerResp)
	}
	if ledgerResp.Data.Items[0].LedgerType != "refund" {
		t.Fatalf("unexpected ledger order %#v", ledgerResp.Data.Items)
	}
	if ledgerResp.Data.Items[0].ID == 0 || ledgerResp.Data.Items[0].TaskID == "" || ledgerResp.Data.Items[0].Reason == "" || ledgerResp.Data.Items[0].FrozenAfter == "" || ledgerResp.Data.Items[0].BalanceAfter == "" {
		t.Fatalf("expected ledger entry contract fields to be populated, got %#v", ledgerResp.Data.Items[0])
	}
	if ledgerResp.Data.Items[0].BucketType == "" || ledgerResp.Data.Items[0].SourceType == "" || ledgerResp.Data.Items[0].Title == "" || ledgerResp.Data.Items[0].OccurredAt == "" || ledgerResp.Data.Items[0].Amount == "" || ledgerResp.Data.Items[0].Type == "" || ledgerResp.Data.Items[0].Detail == "" {
		t.Fatalf("expected ledger entry display fields to be populated, got %#v", ledgerResp.Data.Items[0])
	}
	if ledgerResp.Meta.RequestID == "" || ledgerRec.Header().Get("X-Request-Id") == "" {
		t.Fatalf("expected ledger response to include request id metadata, got body=%#v headers=%v", ledgerResp, ledgerRec.Header())
	}

	invalidLedgerReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/ledger?page=oops&page_size=-5", nil)
	invalidLedgerReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	invalidLedgerRec := httptest.NewRecorder()
	handler.ServeHTTP(invalidLedgerRec, invalidLedgerReq)
	if invalidLedgerRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid ledger pagination to return 400, got %d body=%s", invalidLedgerRec.Code, invalidLedgerRec.Body.String())
	}

	unauthorizedBalanceReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/balance", nil)
	unauthorizedBalanceRec := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedBalanceRec, unauthorizedBalanceReq)
	if unauthorizedBalanceRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized balance request to return 401, got %d body=%s", unauthorizedBalanceRec.Code, unauthorizedBalanceRec.Body.String())
	}

	methodNotAllowedReq := httptest.NewRequest(http.MethodPost, "/api/agent/billing/v1/balance", nil)
	methodNotAllowedReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	methodNotAllowedRec := httptest.NewRecorder()
	handler.ServeHTTP(methodNotAllowedRec, methodNotAllowedReq)
	if methodNotAllowedRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected non-GET balance request to return 405, got %d body=%s", methodNotAllowedRec.Code, methodNotAllowedRec.Body.String())
	}
	var methodNotAllowedResp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(methodNotAllowedRec.Body).Decode(&methodNotAllowedResp); err != nil {
		t.Fatalf("decode balance 405 response: %v", err)
	}
	if methodNotAllowedResp.Error.Code == "" {
		t.Fatalf("expected structured JSON error for 405 response, got %#v", methodNotAllowedResp)
	}

	methodNotAllowedLedgerReq := httptest.NewRequest(http.MethodPost, "/api/agent/billing/v1/ledger", nil)
	methodNotAllowedLedgerReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	methodNotAllowedLedgerRec := httptest.NewRecorder()
	handler.ServeHTTP(methodNotAllowedLedgerRec, methodNotAllowedLedgerReq)
	if methodNotAllowedLedgerRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected non-GET ledger request to return 405, got %d body=%s", methodNotAllowedLedgerRec.Code, methodNotAllowedLedgerRec.Body.String())
	}
}

func TestAgentLedgerEndpointReturnsEmptyArrayForUsersWithoutHistory(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("empty-ledger@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	_, session, err := loginAuthUserWithPasswordSetup(t, authSvc, "empty-ledger@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}

	billingSvc := billingservice.NewService(cfg.Billing)
	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), nil, billingSvc)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, taskSvc, nil, billingSvc)
	handler := NewWithAPI(api)

	ledgerReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/ledger", nil)
	ledgerReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	ledgerRec := httptest.NewRecorder()
	handler.ServeHTTP(ledgerRec, ledgerReq)
	if ledgerRec.Code != http.StatusOK {
		t.Fatalf("expected empty ledger request to return 200, got %d body=%s", ledgerRec.Code, ledgerRec.Body.String())
	}
	var ledgerResp struct {
		Data struct {
			Items []struct{} `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(ledgerRec.Body).Decode(&ledgerResp); err != nil {
		t.Fatalf("decode empty ledger response: %v", err)
	}
	if ledgerResp.Data.Items == nil {
		t.Fatalf("expected empty ledger items to marshal as [] not null, got %#v", ledgerResp)
	}
	if len(ledgerResp.Data.Items) != 0 {
		t.Fatalf("expected no ledger items, got %#v", ledgerResp.Data.Items)
	}
}

func TestAgentBillingEndpointsReuseTaskServiceBillingBackend(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("shared-billing@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	_, session, err := loginAuthUserWithPasswordSetup(t, authSvc, "shared-billing@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}

	billingSvc := billingservice.NewService(cfg.Billing)
	if _, err := billingSvc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{
		UserID:       1,
		ChangePoints: "20.00000",
		Reason:       "seed balance",
	}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}

	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), nil, billingSvc)
	if _, err := taskSvc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:              1,
		UserGroupCode:       "basic",
		UserGroupMultiplier: "1.00000",
		AbstractModel:       "plus",
		TaskType:            "text_to_image",
		Prompt:              "Create a campaign key visual",
		RequestedSize:       "1536x1024",
		BaseResolution:      "auto",
		OutputImageCount:    1,
		ReferenceImageCount: 0,
		ResponseMode:        "async",
		SavePolicy:          "private",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	expectedBalance, err := billingSvc.GetBalance(context.Background(), 1, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if expectedBalance.AvailablePoints != "12.00000" || expectedBalance.FrozenPoints != "8.00000" {
		t.Fatalf("unexpected reserved balance %#v", expectedBalance)
	}

	api := handlers.NewAPIWithTaskService(cfg, authSvc, nil, taskSvc)
	handler := NewWithAPI(api)

	balanceReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/balance", nil)
	balanceReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	balanceRec := httptest.NewRecorder()
	handler.ServeHTTP(balanceRec, balanceReq)
	if balanceRec.Code != http.StatusOK {
		t.Fatalf("expected balance 200, got %d body=%s", balanceRec.Code, balanceRec.Body.String())
	}

	var balanceResp struct {
		Data struct {
			AvailablePoints string `json:"available_points"`
			FrozenPoints    string `json:"frozen_points"`
		} `json:"data"`
	}
	if err := json.NewDecoder(balanceRec.Body).Decode(&balanceResp); err != nil {
		t.Fatalf("decode balance response: %v", err)
	}
	if balanceResp.Data.AvailablePoints != expectedBalance.AvailablePoints || balanceResp.Data.FrozenPoints != expectedBalance.FrozenPoints {
		t.Fatalf("expected API balance to reuse taskSvc billing backend, got %#v want %#v", balanceResp.Data, expectedBalance)
	}
}

func TestAgentEstimateEndpointUsesSnakeCaseAndRejectsInvalidCounts(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("estimate@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	_, session, err := loginAuthUserWithPasswordSetup(t, authSvc, "estimate@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}

	billingSvc := billingservice.NewService(cfg.Billing)
	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), nil, billingSvc)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, taskSvc, nil, billingSvc)
	handler := NewWithAPI(api)

	req := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/estimate?task_type=text_to_image&abstract_model=plus&base_resolution=auto&requested_size=1536x1024&requested_output_image_count=2&reference_image_count=1", nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected estimate 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode estimate response: %v", err)
	}
	if _, ok := resp.Data["base_resolution"]; !ok {
		t.Fatalf("expected snake_case keys, got %#v", resp.Data)
	}
	if _, ok := resp.Data["PricingSnapshot"]; ok {
		t.Fatalf("expected pricing snapshot to stay internal, got %#v", resp.Data)
	}

	invalidReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/estimate?task_type=text_to_image&abstract_model=plus&base_resolution=auto&requested_output_image_count=oops", nil)
	invalidReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	invalidRec := httptest.NewRecorder()
	handler.ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid estimate request to return 400, got %d body=%s", invalidRec.Code, invalidRec.Body.String())
	}
}

func TestAgentEstimateEndpointReturnsBalanceSufficiency(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc, session := loginTestUser(t, "estimate-sufficiency@example.com")
	billingSvc := billingservice.NewService(cfg.Billing)
	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), nil, billingSvc)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, taskSvc, nil, billingSvc)
	handler := NewWithAPI(api)

	estimateURL := "/api/agent/billing/v1/estimate?task_type=text_to_image&abstract_model=plus&base_resolution=auto&requested_size=1536x1024&requested_output_image_count=2&reference_image_count=1"
	req := httptest.NewRequest(http.MethodGet, estimateURL, nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected estimate 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var insufficientResp struct {
		Data struct {
			EstimatedPoints    string `json:"estimated_points"`
			Sufficient         bool   `json:"sufficient"`
			InsufficientPoints string `json:"insufficient_points"`
			Balance            struct {
				AvailablePoints string `json:"available_points"`
			} `json:"balance"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&insufficientResp); err != nil {
		t.Fatalf("decode insufficient estimate response: %v", err)
	}
	if insufficientResp.Data.EstimatedPoints != "16.00000" {
		t.Fatalf("expected estimate to remain deterministic, got %#v", insufficientResp.Data)
	}
	if insufficientResp.Data.Sufficient {
		t.Fatalf("expected insufficient balance, got %#v", insufficientResp.Data)
	}
	if insufficientResp.Data.Balance.AvailablePoints != "0.00000" || insufficientResp.Data.InsufficientPoints != "16.00000" {
		t.Fatalf("expected balance and missing points in estimate, got %#v", insufficientResp.Data)
	}

	if _, err := billingSvc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{UserID: 1, ChangePoints: "20.00000", Reason: "seed enough balance"}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}
	enoughReq := httptest.NewRequest(http.MethodGet, estimateURL, nil)
	enoughReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	enoughRec := httptest.NewRecorder()
	handler.ServeHTTP(enoughRec, enoughReq)
	if enoughRec.Code != http.StatusOK {
		t.Fatalf("expected enough estimate 200, got %d body=%s", enoughRec.Code, enoughRec.Body.String())
	}
	var enoughResp struct {
		Data struct {
			Sufficient         bool   `json:"sufficient"`
			InsufficientPoints string `json:"insufficient_points"`
			Balance            struct {
				AvailablePoints string `json:"available_points"`
			} `json:"balance"`
		} `json:"data"`
	}
	if err := json.NewDecoder(enoughRec.Body).Decode(&enoughResp); err != nil {
		t.Fatalf("decode enough estimate response: %v", err)
	}
	if !enoughResp.Data.Sufficient {
		t.Fatalf("expected sufficient balance, got %#v", enoughResp.Data)
	}
	if enoughResp.Data.Balance.AvailablePoints != "20.00000" || enoughResp.Data.InsufficientPoints != "0.00000" {
		t.Fatalf("expected enough balance and zero missing points, got %#v", enoughResp.Data)
	}
}

func TestAgentTaskCreateIsIdempotentWithIdempotencyKey(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("idem@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	_, session, err := loginAuthUserWithPasswordSetup(t, authSvc, "idem@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}

	billingSvc := billingservice.NewService(cfg.Billing)
	if _, err := billingSvc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{UserID: 1, ChangePoints: "100.00000", Reason: "seed balance"}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}
	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), nil, billingSvc)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, taskSvc, nil, billingSvc)
	handler := NewWithAPI(api)

	body := `{"task_type":"text_to_image","prompt":"Queue once","abstract_model":"plus","base_resolution":"auto","requested_size":"1536x1024","requested_output_image_count":1,"response_mode":"async"}`
	firstReq := httptest.NewRequest(http.MethodPost, "/api/agent/image/v1/tasks", bytes.NewBufferString(body))
	firstReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.Header.Set("Idempotency-Key", "same-request")
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusAccepted {
		t.Fatalf("expected first create 202, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/api/agent/image/v1/tasks", bytes.NewBufferString(body))
	secondReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.Header.Set("Idempotency-Key", "same-request")
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusAccepted {
		t.Fatalf("expected second create 202, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}

	var firstResp, secondResp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(firstRec.Body).Decode(&firstResp); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	if err := json.NewDecoder(secondRec.Body).Decode(&secondResp); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	if firstResp.Data.ID == "" || firstResp.Data.ID != secondResp.Data.ID {
		t.Fatalf("expected same task id for idempotent create, got first=%#v second=%#v", firstResp, secondResp)
	}

	summary, err := billingSvc.GetBalance(context.Background(), 1, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if summary.AvailablePoints != "92.00000" || summary.FrozenPoints != "8.00000" {
		t.Fatalf("expected single reserve after idempotent retry, got %#v", summary)
	}

	thirdReq := httptest.NewRequest(http.MethodPost, "/api/agent/image/v1/tasks", bytes.NewBufferString(`{"task_type":"text_to_image","prompt":"Queue changed","abstract_model":"plus","base_resolution":"auto","requested_size":"1536x1024","requested_output_image_count":1,"response_mode":"async"}`))
	thirdReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	thirdReq.Header.Set("Content-Type", "application/json")
	thirdReq.Header.Set("Idempotency-Key", "same-request")
	thirdRec := httptest.NewRecorder()
	handler.ServeHTTP(thirdRec, thirdReq)
	if thirdRec.Code != http.StatusAccepted {
		t.Fatalf("expected changed-body create 202, got %d body=%s", thirdRec.Code, thirdRec.Body.String())
	}
	var thirdResp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(thirdRec.Body).Decode(&thirdResp); err != nil {
		t.Fatalf("decode third response: %v", err)
	}
	if thirdResp.Data.ID == firstResp.Data.ID {
		t.Fatalf("expected different task id when body changes under same key, got first=%#v third=%#v", firstResp, thirdResp)
	}
}
