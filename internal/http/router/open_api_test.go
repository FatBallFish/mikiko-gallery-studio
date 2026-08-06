package router

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	apikeyservice "github.com/fatballfish/pic-gallery/internal/service/apikey"
	assetservice "github.com/fatballfish/pic-gallery/internal/service/assets"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
)

func TestOpenImageEstimateTaskAndReferenceUploadUseAPIKeyAuth(t *testing.T) {
	handler, creds, billingSvc := newOpenAPIHandler(t)

	estimateReq := httptest.NewRequest(http.MethodGet, "/api/open/image/v1/estimate?task_type=text_to_image&abstract_model=plus&base_resolution=auto&requested_size=1536x1024&requested_output_image_count=2&reference_image_count=0", nil)
	signNativeRequest(estimateReq, creds)
	estimateRec := httptest.NewRecorder()
	handler.ServeHTTP(estimateRec, estimateReq)
	if estimateRec.Code != http.StatusOK {
		t.Fatalf("expected estimate 200, got %d body=%s", estimateRec.Code, estimateRec.Body.String())
	}
	var estimateResp struct {
		Data struct {
			EstimatedPoints string `json:"estimated_points"`
		} `json:"data"`
	}
	if err := json.NewDecoder(estimateRec.Body).Decode(&estimateResp); err != nil {
		t.Fatalf("decode estimate response: %v", err)
	}
	if estimateResp.Data.EstimatedPoints != "16.00000" {
		t.Fatalf("unexpected estimate response %#v", estimateResp)
	}

	png := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+y1X8AAAAASUVORK5CYII="
	uploadBody := bytes.NewBufferString(`{"filename":"tiny.png","mime_type":"image/png","content_base64":"` + png + `"}`)
	uploadReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/reference-assets/uploads", uploadBody)
	uploadReq.Header.Set("Content-Type", "application/json")
	signNativeRequest(uploadReq, creds)
	uploadRec := httptest.NewRecorder()
	handler.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusCreated {
		t.Fatalf("expected upload 201, got %d body=%s", uploadRec.Code, uploadRec.Body.String())
	}
	var uploadResp struct {
		Data struct {
			AssetID    string `json:"asset_id"`
			Status     string `json:"status"`
			UploadMode string `json:"upload_mode"`
			Asset      struct {
				ID string `json:"id"`
			} `json:"asset"`
		} `json:"data"`
	}
	if err := json.NewDecoder(uploadRec.Body).Decode(&uploadResp); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if uploadResp.Data.AssetID == "" || uploadResp.Data.Asset.ID != uploadResp.Data.AssetID || uploadResp.Data.UploadMode != "inline_base64" {
		t.Fatalf("unexpected upload response %#v", uploadResp)
	}

	taskBody := `{"task_type":"image_edit","prompt":"edit reference","abstract_model":"plus","base_resolution":"auto","requested_size":"1536x1024","requested_output_image_count":1,"reference_asset_ids":["` + uploadResp.Data.AssetID + `"],"response_mode":"async"}`
	firstReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/tasks", bytes.NewBufferString(taskBody))
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.Header.Set("Idempotency-Key", "open-idem")
	signNativeRequest(firstReq, creds)
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusAccepted {
		t.Fatalf("expected task create 202, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/tasks", bytes.NewBufferString(taskBody))
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.Header.Set("Idempotency-Key", "open-idem")
	signNativeRequest(secondReq, creds)
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusAccepted {
		t.Fatalf("expected idempotent task retry 202, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
	var firstResp, secondResp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(firstRec.Body).Decode(&firstResp); err != nil {
		t.Fatalf("decode first task response: %v", err)
	}
	if err := json.NewDecoder(secondRec.Body).Decode(&secondResp); err != nil {
		t.Fatalf("decode second task response: %v", err)
	}
	if firstResp.Data.ID == "" || firstResp.Data.ID != secondResp.Data.ID {
		t.Fatalf("expected same task id on idempotent retry, got first=%#v second=%#v", firstResp, secondResp)
	}

	summary, err := billingSvc.GetBalance(context.Background(), creds.UserID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if summary.AvailablePoints != "90.00000" || summary.FrozenPoints != "10.00000" {
		t.Fatalf("expected single image_edit reserve, got %#v", summary)
	}
}

func TestOpenImageEstimateRejectsRemovedReferenceGeneration(t *testing.T) {
	handler, creds, _ := newOpenAPIHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/open/image/v1/estimate?task_type=reference_generate&abstract_model=plus&base_resolution=1k&requested_output_image_count=1", nil)
	signNativeRequest(req, creds)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected removed task type to return 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOpenImageAPIRejectsMissingAccessKeyInvalidSignatureAndInvalidParams(t *testing.T) {
	handler, creds, _ := newOpenAPIHandler(t)

	missingReq := httptest.NewRequest(http.MethodGet, "/api/open/image/v1/estimate?task_type=text_to_image&abstract_model=plus&base_resolution=auto&requested_output_image_count=1", nil)
	missingRec := httptest.NewRecorder()
	handler.ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing key 401, got %d body=%s", missingRec.Code, missingRec.Body.String())
	}

	badSigReq := httptest.NewRequest(http.MethodGet, "/api/open/image/v1/estimate?task_type=text_to_image&abstract_model=plus&base_resolution=auto&requested_output_image_count=1", nil)
	signNativeRequest(badSigReq, creds)
	badSigReq.Header.Set("X-Signature", "wrong")
	badSigRec := httptest.NewRecorder()
	handler.ServeHTTP(badSigRec, badSigReq)
	if badSigRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected bad signature 401, got %d body=%s", badSigRec.Code, badSigRec.Body.String())
	}

	invalidReq := httptest.NewRequest(http.MethodGet, "/api/open/image/v1/estimate?task_type=text_to_image&abstract_model=plus&base_resolution=auto&requested_output_image_count=oops", nil)
	signNativeRequest(invalidReq, creds)
	invalidRec := httptest.NewRecorder()
	handler.ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid params 400, got %d body=%s", invalidRec.Code, invalidRec.Body.String())
	}
}

func TestOpenImageAPIRejectsMissingAuthBeforeReadingLargeBody(t *testing.T) {
	handler, _, _ := newOpenAPIHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/tasks", bytes.NewReader(bytes.Repeat([]byte("x"), 4<<20)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing auth to fail with 401 before body validation, got %d body=%s", rec.Code, rec.Body.String())
	}
}

type failOnReadBody struct {
	reads int
}

func (b *failOnReadBody) Read([]byte) (int, error) {
	b.reads++
	return 0, errors.New("request body must not be read before access key preflight")
}

func TestOpenImageAPIRejectsUnknownAccessKeyBeforeReadingBody(t *testing.T) {
	handler, _, _ := newOpenAPIHandler(t)
	body := &failOnReadBody{}
	req := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/reference-assets/uploads", body)
	timestamp := time.Now().UTC()
	bodyHash := apikeyservice.BodySHA256([]byte("unread"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Access-Key", "ak-does-not-exist")
	req.Header.Set("X-Timestamp", timestamp.Format(time.RFC3339))
	req.Header.Set("X-Body-SHA256", bodyHash)
	req.Header.Set("X-Signature", apikeyservice.SignCanonicalHMAC("unknown-secret", req.Method, req.URL.RequestURI(), timestamp, bodyHash))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unknown key 401, got %d body=%s", rec.Code, rec.Body.String())
	}
	if body.reads != 0 {
		t.Fatalf("unknown access key caused %d request-body reads", body.reads)
	}
}

func TestOpenImageTaskRejectsAPIKeyQuotaExceeded(t *testing.T) {
	dailyQuota := "1.00000"
	totalQuota := "100.00000"
	handler, creds, _ := newOpenAPIHandlerWithCreateRequest(t, apikeyservice.CreateRequest{
		Name:             "openapi-limited",
		GroupCode:        "plus",
		Secret:           "sk-openapi-secret",
		TotalQuotaPoints: &totalQuota,
		DailyQuotaPoints: &dailyQuota,
	})

	taskBody := `{"task_type":"text_to_image","prompt":"cat","abstract_model":"plus","base_resolution":"auto","requested_size":"1536x1024","requested_output_image_count":1,"response_mode":"async"}`
	req := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/tasks", bytes.NewBufferString(taskBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "quota-limit")
	signNativeRequest(req, creds)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected quota exceeded 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOpenImageTaskRejectsSyncResponseMode(t *testing.T) {
	handler, creds, _ := newOpenAPIHandler(t)

	taskBody := `{"task_type":"text_to_image","prompt":"cat","abstract_model":"plus","base_resolution":"auto","requested_size":"1536x1024","requested_output_image_count":1,"response_mode":"sync"}`
	req := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/tasks", bytes.NewBufferString(taskBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "open-sync-rejected")
	signNativeRequest(req, creds)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected sync response_mode rejection 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("only supports async")) {
		t.Fatalf("expected clear async-only error, got body=%s", rec.Body.String())
	}
}

func TestOpenAndCompatAPIRejectDisabledUserAPIKey(t *testing.T) {
	handler, creds, authSvc, _ := newOpenAPIHandlerWithAuth(t, apikeyservice.CreateRequest{
		Name:      "disabled-user",
		GroupCode: "plus",
		Secret:    "sk-disabled-user",
	})
	authSvc.DisableUserForTest(creds.UserID)

	openReq := httptest.NewRequest(http.MethodGet, "/api/open/image/v1/estimate?task_type=text_to_image&abstract_model=plus&base_resolution=auto&requested_output_image_count=1", nil)
	signNativeRequest(openReq, creds)
	openRec := httptest.NewRecorder()
	handler.ServeHTTP(openRec, openReq)
	if openRec.Code != http.StatusForbidden {
		t.Fatalf("expected disabled user native Open API call to return 403, got %d body=%s", openRec.Code, openRec.Body.String())
	}

	compatReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	compatReq.Header.Set("Authorization", "Bearer "+creds.Secret)
	compatRec := httptest.NewRecorder()
	handler.ServeHTTP(compatRec, compatReq)
	if compatRec.Code != http.StatusForbidden {
		t.Fatalf("expected disabled user compat bearer call to return 403, got %d body=%s", compatRec.Code, compatRec.Body.String())
	}
}

type openAPICredentials struct {
	UserID    int64
	AccessKey string
	Secret    string
}

func newOpenAPIHandler(t *testing.T) (http.Handler, openAPICredentials, *billingservice.Service) {
	t.Helper()
	return newOpenAPIHandlerWithCreateRequest(t, apikeyservice.CreateRequest{
		Name:      "openapi",
		GroupCode: "plus",
		Secret:    "sk-openapi-secret",
	})
}

func newOpenAPIHandlerWithCreateRequest(t *testing.T, createReq apikeyservice.CreateRequest) (http.Handler, openAPICredentials, *billingservice.Service) {
	handler, creds, _, billingSvc := newOpenAPIHandlerWithAuth(t, createReq)
	return handler, creds, billingSvc
}

func newOpenAPIHandlerWithAuth(t *testing.T, createReq apikeyservice.CreateRequest) (http.Handler, openAPICredentials, *authservice.Service, *billingservice.Service) {
	return newOpenAPIHandlerWithAuthAndConfig(t, createReq, nil)
}

func newOpenAPIHandlerWithAuthAndConfig(t *testing.T, createReq apikeyservice.CreateRequest, configure func(*config.Config)) (http.Handler, openAPICredentials, *authservice.Service, *billingservice.Service) {
	t.Helper()
	cfg := taskAPIConfig("http://127.0.0.1:1")
	cfg.Storage.LocalRoot = t.TempDir()
	if configure != nil {
		configure(&cfg)
	}
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000", "plus": "1.00000"})
	if err := authSvc.SendEmailCode("openapi@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	user, _, err := loginAuthUserWithPasswordSetup(t, authSvc, "openapi@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}
	billingSvc := billingservice.NewService(cfg.Billing)
	if _, err := billingSvc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{UserID: user.ID, ChangePoints: "100.00000", Reason: "seed balance"}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}
	keySvc := apikeyservice.NewService(nil)
	createReq.UserID = user.ID
	created, err := keySvc.CreateKey(context.Background(), createReq)
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}
	assetSvc := assetservice.NewService(cfg.Storage, cfg.GenerationLimits)
	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), assetSvc, billingSvc)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, assetSvc, taskSvc, nil, billingSvc, keySvc)
	return NewWithAPI(api), openAPICredentials{UserID: user.ID, AccessKey: created.Key.AccessKey, Secret: created.Secret}, authSvc, billingSvc
}

func signNativeRequest(req *http.Request, creds openAPICredentials) {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(body))
	}
	timestamp := time.Now().UTC()
	bodyHash := apikeyservice.BodySHA256(body)
	req.Header.Set("X-Access-Key", creds.AccessKey)
	req.Header.Set("X-Timestamp", timestamp.Format(time.RFC3339))
	req.Header.Set("X-Body-SHA256", bodyHash)
	req.Header.Set("X-Signature", apikeyservice.SignCanonicalHMAC(creds.Secret, req.Method, req.URL.RequestURI(), timestamp, bodyHash))
}

func TestOpenReferenceUploadRejectsInvalidBase64(t *testing.T) {
	handler, creds, _ := newOpenAPIHandler(t)
	body := bytes.NewBufferString(`{"filename":"tiny.png","mime_type":"image/png","content_base64":"not-base64"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/reference-assets/uploads", body)
	req.Header.Set("Content-Type", "application/json")
	signNativeRequest(req, creds)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid upload 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOpenReferenceUploadAcceptsRawStdBase64(t *testing.T) {
	handler, creds, _ := newOpenAPIHandler(t)
	raw, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+y1X8AAAAASUVORK5CYII=")
	uploadBody := bytes.NewBufferString(`{"filename":"tiny.png","mime_type":"image/png","content_base64":"` + base64.StdEncoding.EncodeToString(raw) + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/reference-assets/uploads", uploadBody)
	req.Header.Set("Content-Type", "application/json")
	signNativeRequest(req, creds)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected upload 201, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOpenReferenceUploadAcceptsFileBackedHMACSpool(t *testing.T) {
	spoolDir := t.TempDir()
	t.Setenv("TMPDIR", spoolDir)
	handler, creds, _, _ := newOpenAPIHandlerWithAuthAndConfig(t, apikeyservice.CreateRequest{
		Name:      "openapi",
		GroupCode: "plus",
		Secret:    "sk-openapi-secret",
	}, func(cfg *config.Config) {
		cfg.AttachmentPolicy = config.AttachmentPolicyConfig{
			ImageMaxMB:          2,
			ImageAllowedFormats: []string{"png"},
		}
	})
	raw, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+y1X8AAAAASUVORK5CYII=")
	raw = append(raw, bytes.Repeat([]byte{0}, 1024*1024)...)
	uploadBody := bytes.NewBufferString(`{"filename":"large.png","mime_type":"image/png","content_base64":"` + base64.StdEncoding.EncodeToString(raw) + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/reference-assets/uploads", uploadBody)
	req.Header.Set("Content-Type", "application/json")
	signNativeRequest(req, creds)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected file-backed upload 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	entries, err := os.ReadDir(spoolDir)
	if err != nil {
		t.Fatalf("read spool directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("verified request leaked HMAC spool files: %#v", entries)
	}
}

func TestOpenReferenceMultipartUploadConsumesVerifiedSpooledBody(t *testing.T) {
	handler, creds, _ := newOpenAPIHandler(t)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "tiny.png")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(tinyPNG(t)); err != nil {
		t.Fatalf("write image: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/reference-assets", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	signNativeRequest(req, creds)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected multipart upload 201, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOpenReferenceUploadOverflowUsesImageSizeErrorContract(t *testing.T) {
	handler, creds, _, _ := newOpenAPIHandlerWithAuthAndConfig(t, apikeyservice.CreateRequest{
		Name:      "openapi",
		GroupCode: "plus",
		Secret:    "sk-openapi-secret",
	}, func(cfg *config.Config) {
		cfg.AttachmentPolicy = config.AttachmentPolicyConfig{
			ImageMaxMB:          1,
			ImageAllowedFormats: []string{"png"},
		}
	})
	content := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("x"), 2*1024*1024))
	body := bytes.NewBufferString(`{"filename":"oversized.png","mime_type":"image/png","content_base64":"` + content + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/reference-assets/uploads", body)
	req.Header.Set("Content-Type", "application/json")
	signNativeRequest(req, creds)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected oversized upload 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Error struct {
			Code    string         `json:"code"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload.Error.Code != "IMAGE_REFERENCE_TOO_LARGE" || payload.Error.Details["max_size_mb"] != float64(1) || payload.Error.Details["max_size_bytes"] != float64(1024*1024) {
		t.Fatalf("unexpected oversized upload contract: %#v", payload.Error)
	}
}
