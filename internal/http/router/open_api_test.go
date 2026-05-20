package router

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
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

	estimateReq := httptest.NewRequest(http.MethodGet, "/api/open/image/v1/estimate?task_type=text_to_image&abstract_model=plus&requested_quality=auto&requested_size=1536x1024&requested_output_image_count=2&reference_image_count=0", nil)
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

	taskBody := `{"task_type":"reference_generate","prompt":"use reference","abstract_model":"plus","requested_quality":"auto","requested_size":"1536x1024","requested_output_image_count":1,"reference_asset_ids":["` + uploadResp.Data.AssetID + `"],"response_mode":"async"}`
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
	if summary.AvailablePoints != "90.80000" || summary.FrozenPoints != "9.20000" {
		t.Fatalf("expected single reference_generate reserve, got %#v", summary)
	}
}

func TestOpenImageAPIRejectsMissingAccessKeyInvalidSignatureAndInvalidParams(t *testing.T) {
	handler, creds, _ := newOpenAPIHandler(t)

	missingReq := httptest.NewRequest(http.MethodGet, "/api/open/image/v1/estimate?task_type=text_to_image&abstract_model=plus&requested_quality=auto&requested_output_image_count=1", nil)
	missingRec := httptest.NewRecorder()
	handler.ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing key 401, got %d body=%s", missingRec.Code, missingRec.Body.String())
	}

	missingTimestampReq := httptest.NewRequest(http.MethodGet, "/api/open/image/v1/estimate?task_type=text_to_image&abstract_model=plus&requested_quality=auto&requested_output_image_count=1", nil)
	missingTimestampReq.Header.Set("X-Access-Key", creds.AccessKey)
	missingTimestampReq.Header.Set("X-Signature", creds.Secret)
	missingTimestampRec := httptest.NewRecorder()
	handler.ServeHTTP(missingTimestampRec, missingTimestampReq)
	if missingTimestampRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing timestamp 401, got %d body=%s", missingTimestampRec.Code, missingTimestampRec.Body.String())
	}

	badSigReq := httptest.NewRequest(http.MethodGet, "/api/open/image/v1/estimate?task_type=text_to_image&abstract_model=plus&requested_quality=auto&requested_output_image_count=1", nil)
	signNativeRequest(badSigReq, creds)
	badSigReq.Header.Set("X-Signature", "wrong")
	badSigRec := httptest.NewRecorder()
	handler.ServeHTTP(badSigRec, badSigReq)
	if badSigRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected bad signature 401, got %d body=%s", badSigRec.Code, badSigRec.Body.String())
	}

	invalidReq := httptest.NewRequest(http.MethodGet, "/api/open/image/v1/estimate?task_type=text_to_image&abstract_model=plus&requested_quality=auto&requested_output_image_count=oops", nil)
	signNativeRequest(invalidReq, creds)
	invalidRec := httptest.NewRecorder()
	handler.ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid params 400, got %d body=%s", invalidRec.Code, invalidRec.Body.String())
	}
}

func TestOpenImageAPIRejectsCanonicalBodyAndTimestampMismatch(t *testing.T) {
	handler, creds, _ := newOpenAPIHandler(t)

	goodBody := `{"task_type":"text_to_image","prompt":"signed body","abstract_model":"plus","requested_quality":"auto","requested_size":"1536x1024","requested_output_image_count":1,"response_mode":"async"}`
	badBodyReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/tasks", bytes.NewBufferString(goodBody))
	badBodyReq.Header.Set("Content-Type", "application/json")
	signNativeRequest(badBodyReq, creds)
	badBodyReq.Body = io.NopCloser(bytes.NewBufferString(`{"task_type":"text_to_image","prompt":"tampered","abstract_model":"plus","requested_quality":"auto","requested_size":"1536x1024","requested_output_image_count":1,"response_mode":"async"}`))
	badBodyRec := httptest.NewRecorder()
	handler.ServeHTTP(badBodyRec, badBodyReq)
	if badBodyRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected body mismatch 401, got %d body=%s", badBodyRec.Code, badBodyRec.Body.String())
	}

	oldReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/tasks", bytes.NewBufferString(goodBody))
	oldReq.Header.Set("Content-Type", "application/json")
	signNativeRequestAt(oldReq, creds, time.Now().Add(-10*time.Minute))
	oldRec := httptest.NewRecorder()
	handler.ServeHTTP(oldRec, oldReq)
	if oldRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected stale timestamp 401, got %d body=%s", oldRec.Code, oldRec.Body.String())
	}
}

func TestOpenTaskCreateEnforcesAPIKeyRPMBeforeReserve(t *testing.T) {
	rpm := 1
	handler, creds, billingSvc := newOpenAPIHandlerWithKey(t, apikeyservice.CreateRequest{
		Name:      "openapi-rpm",
		GroupCode: "plus",
		Secret:    "sk-openapi-rpm-secret",
		RPMLimit:  &rpm,
	})

	firstBody := `{"task_type":"text_to_image","prompt":"first","abstract_model":"plus","requested_quality":"auto","requested_size":"1536x1024","requested_output_image_count":1,"response_mode":"async"}`
	firstReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/tasks", bytes.NewBufferString(firstBody))
	firstReq.Header.Set("Content-Type", "application/json")
	signNativeRequest(firstReq, creds)
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusAccepted {
		t.Fatalf("expected first task 202, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	secondBody := `{"task_type":"text_to_image","prompt":"second","abstract_model":"plus","requested_quality":"auto","requested_size":"1536x1024","requested_output_image_count":1,"response_mode":"async"}`
	secondReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/tasks", bytes.NewBufferString(secondBody))
	secondReq.Header.Set("Content-Type", "application/json")
	signNativeRequest(secondReq, creds)
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected rpm limit 429, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}

	summary, err := billingSvc.GetBalance(context.Background(), creds.UserID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if summary.AvailablePoints != "92.00000" || summary.FrozenPoints != "8.00000" {
		t.Fatalf("expected only first task reserve after rpm limit, got %#v", summary)
	}
}

func TestOpenTaskCreateEnforcesAPIKeyQuotaBeforeReserve(t *testing.T) {
	for _, tc := range []struct {
		name  string
		total *string
		daily *string
	}{
		{name: "total", total: ptrString("7.00000")},
		{name: "daily", daily: ptrString("7.00000")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler, creds, billingSvc := newOpenAPIHandlerWithKey(t, apikeyservice.CreateRequest{
				Name:             "openapi-quota-" + tc.name,
				GroupCode:        "plus",
				Secret:           "sk-openapi-quota-secret-" + tc.name,
				TotalQuotaPoints: tc.total,
				DailyQuotaPoints: tc.daily,
			})

			body := `{"task_type":"text_to_image","prompt":"quota","abstract_model":"plus","requested_quality":"auto","requested_size":"1536x1024","requested_output_image_count":1,"response_mode":"async"}`
			req := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/tasks", bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
			signNativeRequest(req, creds)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusTooManyRequests {
				t.Fatalf("expected quota limit 429, got %d body=%s", rec.Code, rec.Body.String())
			}

			summary, err := billingSvc.GetBalance(context.Background(), creds.UserID, "1.00000")
			if err != nil {
				t.Fatalf("GetBalance: %v", err)
			}
			if summary.AvailablePoints != "100.00000" || summary.FrozenPoints != "0.00000" {
				t.Fatalf("expected no reserve on quota failure, got %#v", summary)
			}
		})
	}
}

type openAPICredentials struct {
	UserID    int64
	AccessKey string
	Secret    string
}

func newOpenAPIHandler(t *testing.T) (http.Handler, openAPICredentials, *billingservice.Service) {
	t.Helper()
	return newOpenAPIHandlerWithKey(t, apikeyservice.CreateRequest{
		Name:      "openapi",
		GroupCode: "plus",
		Secret:    "sk-openapi-secret",
	})
}

func newOpenAPIHandlerWithKey(t *testing.T, keyReq apikeyservice.CreateRequest) (http.Handler, openAPICredentials, *billingservice.Service) {
	t.Helper()
	cfg := taskAPIConfig("http://127.0.0.1:1")
	cfg.Storage.LocalRoot = t.TempDir()
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
	user, _, err := authSvc.LoginWithEmailCode("openapi@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}
	billingSvc := billingservice.NewService(cfg.Billing)
	if _, err := billingSvc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{UserID: user.ID, ChangePoints: "100.00000", Reason: "seed balance"}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}
	keySvc := apikeyservice.NewService(nil)
	keyReq.UserID = user.ID
	created, err := keySvc.CreateKey(context.Background(), keyReq)
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}
	assetSvc := assetservice.NewService(cfg.Storage, cfg.GenerationLimits)
	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), assetSvc, billingSvc)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, assetSvc, taskSvc, nil, billingSvc, keySvc)
	return NewWithAPI(api), openAPICredentials{UserID: user.ID, AccessKey: created.Key.AccessKey, Secret: created.Secret}, billingSvc
}

func signNativeRequest(req *http.Request, creds openAPICredentials) {
	signNativeRequestAt(req, creds, time.Now())
}

func signNativeRequestAt(req *http.Request, creds openAPICredentials, at time.Time) {
	body, _ := io.ReadAll(req.Body)
	req.Body = io.NopCloser(bytes.NewReader(body))
	bodySum := sha256.Sum256(body)
	timestamp := strconv.FormatInt(at.Unix(), 10)
	canonical := req.Method + req.URL.Path + timestamp + hex.EncodeToString(bodySum[:])
	mac := hmac.New(sha256.New, []byte(creds.Secret))
	_, _ = mac.Write([]byte(canonical))
	req.Header.Set("X-Access-Key", creds.AccessKey)
	req.Header.Set("X-Timestamp", timestamp)
	req.Header.Set("X-Signature", hex.EncodeToString(mac.Sum(nil)))
}

func ptrString(value string) *string {
	return &value
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
