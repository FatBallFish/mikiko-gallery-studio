package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
	adminuserservice "github.com/fatballfish/pic-gallery/internal/service/adminuser"
	auditservice "github.com/fatballfish/pic-gallery/internal/service/audit"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	_ "github.com/mattn/go-sqlite3"
)

func TestAdminUserManagementEndpoints(t *testing.T) {
	cfg := adminConfigAPIConfig()
	client, err := repoent.Open(dialect.SQLite, "file:admin-users-api?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	authStore := entstore.NewAuthStore(client)
	authSvc := authservice.NewServiceWithStore(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"}, authStore)
	if err := authSvc.SendEmailCode("managed@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	managedUser, managedSession, err := authSvc.LoginWithEmailCode("managed@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}

	adminStore := entstore.NewAdminAuthStore(client)
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "admin-users@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: "super_admin", Status: "active"}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	billingStore := entstore.NewBillingStore(client, 5)
	billingSvc := billingservice.NewServiceWithStore(cfg.Billing, billingStore)
	adminUsers := adminuserservice.NewServiceWithStore(entstore.NewAdminUserStore(client, billingStore), billingSvc)
	auditSvc := auditservice.NewService(entstore.NewAuditStore(client))
	api := handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, auditSvc, adminUsers)
	handler := NewWithAPI(api)

	adminToken := loginAdminForUserTest(t, handler)

	adjustBody := bytes.NewBufferString(`{"change_points":"12.00000","reason":"manual grant"}`)
	adjustReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/users/1/points-adjustments", adjustBody)
	adjustReq.Header.Set("Authorization", "Bearer "+adminToken)
	adjustReq.Header.Set("Content-Type", "application/json")
	adjustReq.Header.Set("Idempotency-Key", "admin-adjust-once")
	adjustRec := httptest.NewRecorder()
	handler.ServeHTTP(adjustRec, adjustReq)
	if adjustRec.Code != http.StatusOK {
		t.Fatalf("expected point adjustment 200, got %d body=%s", adjustRec.Code, adjustRec.Body.String())
	}

	replayReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/users/1/points-adjustments", bytes.NewBufferString(`{"change_points":"12.00000","reason":"manual grant"}`))
	replayReq.Header.Set("Authorization", "Bearer "+adminToken)
	replayReq.Header.Set("Content-Type", "application/json")
	replayReq.Header.Set("Idempotency-Key", "admin-adjust-once")
	replayRec := httptest.NewRecorder()
	handler.ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusOK {
		t.Fatalf("expected idempotent replay 200, got %d body=%s", replayRec.Code, replayRec.Body.String())
	}
	conflictingReplayReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/users/1/points-adjustments", bytes.NewBufferString(`{"change_points":"13.00000","reason":"manual grant"}`))
	conflictingReplayReq.Header.Set("Authorization", "Bearer "+adminToken)
	conflictingReplayReq.Header.Set("Content-Type", "application/json")
	conflictingReplayReq.Header.Set("Idempotency-Key", "admin-adjust-once")
	conflictingReplayRec := httptest.NewRecorder()
	handler.ServeHTTP(conflictingReplayRec, conflictingReplayReq)
	if conflictingReplayRec.Code != http.StatusConflict {
		t.Fatalf("expected conflicting idempotency replay 409, got %d body=%s", conflictingReplayRec.Code, conflictingReplayRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/users?page=1&page_size=10&query=managed&status=active", nil)
	listReq.Header.Set("Authorization", "Bearer "+adminToken)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list users 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listResp struct {
		Data struct {
			Items []struct {
				ID           int64  `json:"id"`
				Email        string `json:"email"`
				Status       string `json:"status"`
				TokenVersion int    `json:"token_version"`
			} `json:"items"`
			Pagination struct {
				Total int `json:"total"`
			} `json:"pagination"`
		} `json:"data"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if listResp.Data.Pagination.Total != 1 || len(listResp.Data.Items) != 1 || listResp.Data.Items[0].Email != "managed@example.com" {
		t.Fatalf("unexpected list response %#v", listResp)
	}
	invalidStatusListReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/users?status=unknown", nil)
	invalidStatusListReq.Header.Set("Authorization", "Bearer "+adminToken)
	invalidStatusListRec := httptest.NewRecorder()
	handler.ServeHTTP(invalidStatusListRec, invalidStatusListReq)
	if invalidStatusListRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid list status 400, got %d body=%s", invalidStatusListRec.Code, invalidStatusListRec.Body.String())
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/users/1", nil)
	detailReq.Header.Set("Authorization", "Bearer "+adminToken)
	detailRec := httptest.NewRecorder()
	handler.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("expected user detail 200, got %d body=%s", detailRec.Code, detailRec.Body.String())
	}
	var detailResp struct {
		Data struct {
			User struct {
				ID    int64  `json:"id"`
				Email string `json:"email"`
			} `json:"user"`
			Balance struct {
				AvailablePoints string `json:"available_points"`
			} `json:"balance"`
			RecentLedger []struct {
				LedgerType   string `json:"ledger_type"`
				ChangePoints string `json:"change_points"`
			} `json:"recent_ledger"`
		} `json:"data"`
	}
	if err := json.NewDecoder(detailRec.Body).Decode(&detailResp); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if detailResp.Data.User.ID != managedUser.ID || detailResp.Data.Balance.AvailablePoints != "12.00000" || len(detailResp.Data.RecentLedger) != 1 {
		t.Fatalf("unexpected detail response %#v", detailResp)
	}

	missingKeyReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/users/1/points-adjustments", bytes.NewBufferString(`{"change_points":"1.00000","reason":"missing key"}`))
	missingKeyReq.Header.Set("Authorization", "Bearer "+adminToken)
	missingKeyReq.Header.Set("Content-Type", "application/json")
	missingKeyRec := httptest.NewRecorder()
	handler.ServeHTTP(missingKeyRec, missingKeyReq)
	if missingKeyRec.Code != http.StatusBadRequest {
		t.Fatalf("expected missing Idempotency-Key 400, got %d body=%s", missingKeyRec.Code, missingKeyRec.Body.String())
	}
	missingUserAdjustReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/users/999/points-adjustments", bytes.NewBufferString(`{"change_points":"1.00000","reason":"missing user"}`))
	missingUserAdjustReq.Header.Set("Authorization", "Bearer "+adminToken)
	missingUserAdjustReq.Header.Set("Content-Type", "application/json")
	missingUserAdjustReq.Header.Set("Idempotency-Key", "missing-user-adjust")
	missingUserAdjustRec := httptest.NewRecorder()
	handler.ServeHTTP(missingUserAdjustRec, missingUserAdjustReq)
	if missingUserAdjustRec.Code != http.StatusNotFound {
		t.Fatalf("expected missing user adjustment 404, got %d body=%s", missingUserAdjustRec.Code, missingUserAdjustRec.Body.String())
	}
	invalidPrecisionReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/users/1/points-adjustments", bytes.NewBufferString(`{"change_points":"0.000001","reason":"too precise"}`))
	invalidPrecisionReq.Header.Set("Authorization", "Bearer "+adminToken)
	invalidPrecisionReq.Header.Set("Content-Type", "application/json")
	invalidPrecisionReq.Header.Set("Idempotency-Key", "too-precise-adjust")
	invalidPrecisionRec := httptest.NewRecorder()
	handler.ServeHTTP(invalidPrecisionRec, invalidPrecisionReq)
	if invalidPrecisionRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid precision 400, got %d body=%s", invalidPrecisionRec.Code, invalidPrecisionRec.Body.String())
	}

	statusReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/users/1/status", bytes.NewBufferString(`{"status":"disabled"}`))
	statusReq.Header.Set("Authorization", "Bearer "+adminToken)
	statusReq.Header.Set("Content-Type", "application/json")
	statusRec := httptest.NewRecorder()
	handler.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("expected status update 200, got %d body=%s", statusRec.Code, statusRec.Body.String())
	}

	profileReq := httptest.NewRequest(http.MethodGet, "/api/agent/user/v1/profile", nil)
	profileReq.Header.Set("Authorization", "Bearer "+managedSession.AccessToken)
	profileRec := httptest.NewRecorder()
	handler.ServeHTTP(profileRec, profileReq)
	if profileRec.Code != http.StatusForbidden && profileRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected existing user token to be rejected after disable, got %d body=%s", profileRec.Code, profileRec.Body.String())
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/audit-logs", nil)
	auditReq.Header.Set("Authorization", "Bearer "+adminToken)
	auditRec := httptest.NewRecorder()
	handler.ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit list 200, got %d body=%s", auditRec.Code, auditRec.Body.String())
	}
	if !bytes.Contains(auditRec.Body.Bytes(), []byte("user.points_adjust")) || !bytes.Contains(auditRec.Body.Bytes(), []byte("user.status_update")) {
		t.Fatalf("expected audit logs for admin writes, got body=%s", auditRec.Body.String())
	}
}

func loginAdminForUserTest(t *testing.T, handler http.Handler) string {
	t.Helper()
	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"admin-users@example.com","password":"password"}`))
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
