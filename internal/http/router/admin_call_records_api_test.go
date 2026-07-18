package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	"github.com/fatballfish/pic-gallery/internal/provider"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
	admincallrecordservice "github.com/fatballfish/pic-gallery/internal/service/admincallrecord"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	_ "github.com/mattn/go-sqlite3"
)

func TestAdminCallRecordsEndpointListsRealImageTasks(t *testing.T) {
	cfg := adminConfigAPIConfig()
	client, err := repoent.Open(dialect.SQLite, "file:admin-call-records-api?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	authSvc := authservice.NewServiceWithStore(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"}, entstore.NewAuthStore(client))
	adminStore := entstore.NewAdminAuthStore(client)
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "admin-call-records@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: "super_admin", Status: "active"}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	callRecords := admincallrecordservice.NewServiceWithStore(entstore.NewAdminCallRecordStore(client))
	api := handlers.NewAPIWithCallRecordService(cfg, authSvc, nil, nil, nil, nil, nil, adminAuth, nil, nil, nil, callRecords)
	handler := NewWithAPI(api)

	taskStore := entstore.NewImageTaskStore(client)
	if err := taskStore.Save(t.Context(), domainimagetask.Task{
		UserID:        77,
		APIKeyID:      13,
		SourceChannel: "openapi",
		ID:            "cccccccc-cccc-cccc-cccc-cccccccccccc",
		Status:        domainimagetask.StatusSucceeded,
		Provider:      "openai",
		AbstractModel: "plus",
		TaskType:      string(provider.TaskTypeTextToImage),
		Prompt:        "admin list",

		BaseResolution:      "2k",
		Quality:             "auto",
		OutputImageCount:    2,
		ReferenceImageCount: 1,
		EstimatedPoints:     "8.00000",
		ActualPoints:        "8.00000",
		Attempts:            []domainimagetask.Attempt{{Provider: "openai", Status: domainimagetask.StatusSucceeded}},
	}); err != nil {
		t.Fatalf("Save task: %v", err)
	}
	if err := taskStore.Save(t.Context(), domainimagetask.Task{
		UserID:        77,
		APIKeyID:      14,
		SourceChannel: "web",
		ID:            "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee",
		Status:        domainimagetask.StatusFailed,
		Provider:      "openrouter",
		AbstractModel: "plus",
		TaskType:      string(provider.TaskTypeTextToImage),
		Prompt:        "admin failed list",

		BaseResolution:      "2k",
		Quality:             "auto",
		OutputImageCount:    1,
		ReferenceImageCount: 0,
		EstimatedPoints:     "8.00000",
		ActualPoints:        "0.00000",
		ErrorCode:           "provider_error",
		ErrorMessage:        "upstream failed",
	}); err != nil {
		t.Fatalf("Save failed task: %v", err)
	}
	upstreamSucceededAt := time.Now().UTC().Add(-time.Minute)
	if err := taskStore.Save(t.Context(), domainimagetask.Task{
		UserID: 77, ID: "ffffffff-ffff-ffff-ffff-ffffffffffff", Status: domainimagetask.StatusFailed,
		Provider: "openrouter", AbstractModel: "plus", TaskType: string(provider.TaskTypeTextToImage),
		BaseResolution: "2k", Quality: "auto", OutputImageCount: 1,
		ProviderRequestID: "paid-request-api", ProviderCost: "0.34567", UpstreamSucceededAt: &upstreamSucceededAt,
		ErrorCode: "IMAGE_STORAGE_FAILED", ErrorMessage: "ARTIFACT_STORAGE_WRITE_FAILED",
		ArtifactRecovery: domainimagetask.ArtifactRecovery{
			Status: "failed", AttemptCount: 4, EncryptedPayload: "ciphertext-secret",
			LastDiagnostic: domainimagetask.ArtifactDiagnostic{Code: "ARTIFACT_STORAGE_WRITE_FAILED", Stage: "store", Attempt: 4, Retryable: true, Cause: "temporary storage failure"},
			Diagnostics:    []domainimagetask.ArtifactDiagnostic{{Code: "ARTIFACT_FETCH_TIMEOUT", Stage: "fetch", Attempt: 1, URLHost: "cdn.example.com", URLPath: "/result.png", Retryable: true}},
		},
	}); err != nil {
		t.Fatalf("Save artifact loss task: %v", err)
	}

	adminToken := loginAdminForCallRecordsTest(t, handler)
	listReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/call-records?page=1&page_size=5&status=succeeded&provider=openai&source_channel=openapi&user_id=77&task_id=cccccccc-cccc-cccc-cccc-cccccccccccc", nil)
	listReq.Header.Set("Authorization", "Bearer "+adminToken)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list call records 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	listBody := listRec.Body.String()

	var listResp struct {
		Data struct {
			Items []struct {
				TaskID                    string  `json:"task_id"`
				UserID                    int64   `json:"user_id"`
				APIKeyID                  *int64  `json:"api_key_id"`
				SourceChannel             string  `json:"source_channel"`
				TaskType                  string  `json:"task_type"`
				Status                    string  `json:"status"`
				Provider                  string  `json:"provider"`
				AbstractModel             string  `json:"abstract_model"`
				BaseResolution            string  `json:"base_resolution"`
				Quality                   string  `json:"quality"`
				RequestedOutputImageCount int     `json:"requested_output_image_count"`
				SuccessOutputImageCount   int     `json:"success_output_image_count"`
				ReferenceImageCount       int     `json:"reference_image_count"`
				EstimatedPoints           string  `json:"estimated_points"`
				ActualPoints              string  `json:"actual_points"`
				ErrorCode                 *string `json:"error_code"`
				ErrorMessage              *string `json:"error_message"`
				CreatedAt                 string  `json:"created_at"`
				UpdatedAt                 string  `json:"updated_at"`
				StartedAt                 *string `json:"started_at"`
				FinishedAt                *string `json:"finished_at"`
				AttemptCount              int     `json:"attempt_count"`
			} `json:"items"`
			Pagination struct {
				Page     int `json:"page"`
				PageSize int `json:"page_size"`
				Total    int `json:"total"`
			} `json:"pagination"`
		} `json:"data"`
	}
	if err := json.NewDecoder(strings.NewReader(listBody)).Decode(&listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if listResp.Data.Pagination.Page != 1 || listResp.Data.Pagination.PageSize != 5 || listResp.Data.Pagination.Total != 1 {
		t.Fatalf("unexpected pagination %#v", listResp.Data.Pagination)
	}
	if len(listResp.Data.Items) != 1 {
		t.Fatalf("expected one item, got %#v", listResp.Data.Items)
	}
	item := listResp.Data.Items[0]
	if item.TaskID != "cccccccc-cccc-cccc-cccc-cccccccccccc" || item.UserID != 77 || item.APIKeyID == nil || *item.APIKeyID != 13 {
		t.Fatalf("unexpected identity fields %#v", item)
	}
	if item.SourceChannel != "openapi" || item.TaskType != string(provider.TaskTypeTextToImage) || item.Status != domainimagetask.StatusSucceeded {
		t.Fatalf("unexpected classification fields %#v", item)
	}
	if item.Provider != "openai" || item.AbstractModel != "plus" || item.BaseResolution != "2k" || item.Quality != "auto" {
		t.Fatalf("unexpected model fields %#v", item)
	}
	if item.RequestedOutputImageCount != 2 || item.SuccessOutputImageCount != 0 || item.ReferenceImageCount != 1 || item.EstimatedPoints != "8.00000" || item.ActualPoints != "8.00000" || item.AttemptCount != 1 {
		t.Fatalf("unexpected usage fields %#v", item)
	}
	if item.ErrorCode != nil || item.ErrorMessage != nil || item.CreatedAt == "" || item.UpdatedAt == "" || item.StartedAt == nil || item.FinishedAt == nil {
		t.Fatalf("unexpected nullable/timestamp fields %#v", item)
	}
	for _, key := range []string{"api_key_id", "provider", "error_code", "error_message", "created_at", "updated_at", "started_at", "finished_at"} {
		if !strings.Contains(listBody, `"`+key+`":`) {
			t.Fatalf("expected response to contain stable key %q, got %s", key, listBody)
		}
	}
	errorCodeReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/call-records?page=1&page_size=5&status=failed&error_code=provider_error", nil)
	errorCodeReq.Header.Set("Authorization", "Bearer "+adminToken)
	errorCodeRec := httptest.NewRecorder()
	handler.ServeHTTP(errorCodeRec, errorCodeReq)
	if errorCodeRec.Code != http.StatusOK {
		t.Fatalf("expected error_code list 200, got %d body=%s", errorCodeRec.Code, errorCodeRec.Body.String())
	}
	var errorCodeResp struct {
		Data struct {
			Items []struct {
				TaskID    string  `json:"task_id"`
				ErrorCode *string `json:"error_code"`
			} `json:"items"`
			Pagination struct {
				Total int `json:"total"`
			} `json:"pagination"`
		} `json:"data"`
	}
	if err := json.NewDecoder(errorCodeRec.Body).Decode(&errorCodeResp); err != nil {
		t.Fatalf("decode error_code response: %v", err)
	}
	if errorCodeResp.Data.Pagination.Total != 1 || len(errorCodeResp.Data.Items) != 1 || errorCodeResp.Data.Items[0].TaskID != "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee" {
		t.Fatalf("expected error_code filter to return failed task, got %#v", errorCodeResp.Data)
	}
	lossReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/call-records?page=1&page_size=5&platform_loss=true", nil)
	lossReq.Header.Set("Authorization", "Bearer "+adminToken)
	lossRec := httptest.NewRecorder()
	handler.ServeHTTP(lossRec, lossReq)
	if lossRec.Code != http.StatusOK {
		t.Fatalf("expected platform loss list 200, got %d body=%s", lossRec.Code, lossRec.Body.String())
	}
	lossBody := lossRec.Body.String()
	for _, expected := range []string{`"task_id":"ffffffff-ffff-ffff-ffff-ffffffffffff"`, `"provider_request_id":"paid-request-api"`, `"failure_phase":"artifact_persistence"`, `"platform_loss":true`, `"attempt_count":4`, `"url_host":"cdn.example.com"`} {
		if !strings.Contains(lossBody, expected) {
			t.Fatalf("platform loss response missing %s: %s", expected, lossBody)
		}
	}
	for _, forbidden := range []string{"ciphertext-secret", "signature="} {
		if strings.Contains(lossBody, forbidden) {
			t.Fatalf("platform loss response leaked %q: %s", forbidden, lossBody)
		}
	}
	tooLargeReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/call-records?page_size=101", nil)
	tooLargeReq.Header.Set("Authorization", "Bearer "+adminToken)
	tooLargeRec := httptest.NewRecorder()
	handler.ServeHTTP(tooLargeRec, tooLargeReq)
	if tooLargeRec.Code != http.StatusBadRequest {
		t.Fatalf("expected page_size overflow 400, got %d body=%s", tooLargeRec.Code, tooLargeRec.Body.String())
	}
}

func loginAdminForCallRecordsTest(t *testing.T, handler http.Handler) string {
	t.Helper()
	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"admin-call-records@example.com","password":"password"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected admin login 200, got %d body=%s", loginRec.Code, loginRec.Body.String())
	}
	var loginResp struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(loginRec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	return loginResp.Data.AccessToken
}
