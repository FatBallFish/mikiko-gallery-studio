package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
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
	redeemservice "github.com/fatballfish/pic-gallery/internal/service/redeem"
	_ "github.com/mattn/go-sqlite3"
)

func TestAdminRedeemCodeManagementEndpoints(t *testing.T) {
	cfg := adminConfigAPIConfig()
	client, err := repoent.Open(dialect.SQLite, "file:admin-redeem-api?mode=memory&cache=shared&_fk=1")
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
	if err := authSvc.SendEmailCode("redeemer@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	user, userSession, err := authSvc.LoginWithEmailCode("redeemer@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}

	adminStore := entstore.NewAdminAuthStore(client)
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "admin-redeem@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: "super_admin", Status: "active"}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	billingStore := entstore.NewBillingStore(client, 5)
	billingSvc := billingservice.NewServiceWithStore(cfg.Billing, billingStore)
	adminUsers := adminuserservice.NewServiceWithStore(entstore.NewAdminUserStore(client, billingStore), billingSvc)
	auditSvc := auditservice.NewService(entstore.NewAuditStore(client))
	redeemSvc := redeemservice.NewServiceWithStore(entstore.NewRedeemAdminStore(client))
	api := handlers.NewAPIWithAdminServices(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, auditSvc, adminUsers, redeemSvc)
	handler := NewWithAPI(api)

	adminToken := loginAdminForRedeemTest(t, handler)
	validUntil := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)

	createReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/redeem-codes", bytes.NewBufferString(`{"code":"manual-copy-1","status":"available","reward_type":"points","reward_value":"6.00000","valid_until":"`+validUntil+`","max_redemptions":1}`))
	createReq.Header.Set("Authorization", "Bearer "+adminToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data struct {
			ID     int64  `json:"id"`
			Code   string `json:"code"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createResp.Data.Code != "MANUAL-COPY-1" || createResp.Data.Status != "available" {
		t.Fatalf("unexpected create response %#v", createResp)
	}

	generateReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/redeem-codes", bytes.NewBufferString(`{"status":"available","reward_type":"points","reward_value":"3.00000","valid_until":"`+validUntil+`","max_redemptions":1}`))
	generateReq.Header.Set("Authorization", "Bearer "+adminToken)
	generateReq.Header.Set("Content-Type", "application/json")
	generateRec := httptest.NewRecorder()
	handler.ServeHTTP(generateRec, generateReq)
	if generateRec.Code != http.StatusCreated {
		t.Fatalf("expected generated create 201, got %d body=%s", generateRec.Code, generateRec.Body.String())
	}
	var generateResp struct {
		Data struct {
			Code string `json:"code"`
		} `json:"data"`
	}
	if err := json.NewDecoder(generateRec.Body).Decode(&generateResp); err != nil {
		t.Fatalf("decode generated response: %v", err)
	}
	if ok := regexp.MustCompile(`^[23456789ABCDEFGHJKLMNPQRSTUVWXYZ]{12}$`).MatchString(generateResp.Data.Code); !ok {
		t.Fatalf("expected uppercase safe generated code, got %q", generateResp.Data.Code)
	}

	batchReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/redeem-codes:batch-create", bytes.NewBufferString(`{"count":2,"status":"available","reward_type":"points","reward_value":"1.50000","valid_until":"`+validUntil+`","max_redemptions":1}`))
	batchReq.Header.Set("Authorization", "Bearer "+adminToken)
	batchReq.Header.Set("Content-Type", "application/json")
	batchRec := httptest.NewRecorder()
	handler.ServeHTTP(batchRec, batchReq)
	if batchRec.Code != http.StatusCreated {
		t.Fatalf("expected batch create 201, got %d body=%s", batchRec.Code, batchRec.Body.String())
	}
	var batchResp struct {
		Data struct {
			Count   int   `json:"count"`
			BatchID int64 `json:"batch_id"`
			Items   []struct {
				Code    string `json:"code"`
				BatchID int64  `json:"batch_id"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(batchRec.Body).Decode(&batchResp); err != nil {
		t.Fatalf("decode batch response: %v", err)
	}
	if batchResp.Data.Count != 2 || len(batchResp.Data.Items) != 2 || batchResp.Data.BatchID == 0 {
		t.Fatalf("unexpected batch response %#v", batchResp)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/redeem-codes?page=1&page_size=10&status=available&code=manual", nil)
	listReq.Header.Set("Authorization", "Bearer "+adminToken)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	if !bytes.Contains(listRec.Body.Bytes(), []byte("MANUAL-COPY-1")) {
		t.Fatalf("expected list to include manual code, got %s", listRec.Body.String())
	}

	statusReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/redeem-codes/"+jsonNumber(createResp.Data.ID)+"/status", bytes.NewBufferString(`{"status":"disabled"}`))
	statusReq.Header.Set("Authorization", "Bearer "+adminToken)
	statusReq.Header.Set("Content-Type", "application/json")
	statusRec := httptest.NewRecorder()
	handler.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("expected status update 200, got %d body=%s", statusRec.Code, statusRec.Body.String())
	}

	reactivateReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/redeem-codes/"+jsonNumber(createResp.Data.ID)+"/status", bytes.NewBufferString(`{"status":"available"}`))
	reactivateReq.Header.Set("Authorization", "Bearer "+adminToken)
	reactivateReq.Header.Set("Content-Type", "application/json")
	reactivateRec := httptest.NewRecorder()
	handler.ServeHTTP(reactivateRec, reactivateReq)
	if reactivateRec.Code != http.StatusOK {
		t.Fatalf("expected status reactivation 200, got %d body=%s", reactivateRec.Code, reactivateRec.Body.String())
	}

	redeemReq := httptest.NewRequest(http.MethodPost, "/api/agent/billing/v1/redeem-codes/redeem", bytes.NewBufferString(`{"code":"MANUAL-COPY-1"}`))
	redeemReq.Header.Set("Authorization", "Bearer "+userSession.AccessToken)
	redeemReq.Header.Set("Content-Type", "application/json")
	redeemReq.Header.Set("Idempotency-Key", "redeem-admin-code-once")
	redeemRec := httptest.NewRecorder()
	handler.ServeHTTP(redeemRec, redeemReq)
	if redeemRec.Code != http.StatusOK {
		t.Fatalf("expected user redeem 200, got %d body=%s", redeemRec.Code, redeemRec.Body.String())
	}
	if user.ID <= 0 {
		t.Fatalf("expected test user id")
	}

	redemptionsReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/redeem-codes/"+jsonNumber(createResp.Data.ID)+"/redemptions", nil)
	redemptionsReq.Header.Set("Authorization", "Bearer "+adminToken)
	redemptionsRec := httptest.NewRecorder()
	handler.ServeHTTP(redemptionsRec, redemptionsReq)
	if redemptionsRec.Code != http.StatusOK {
		t.Fatalf("expected redemptions 200, got %d body=%s", redemptionsRec.Code, redemptionsRec.Body.String())
	}
	if !bytes.Contains(redemptionsRec.Body.Bytes(), []byte(`"ledger_type":"redeem"`)) || !bytes.Contains(redemptionsRec.Body.Bytes(), []byte(`"change_points":"6.00000"`)) || !bytes.Contains(redemptionsRec.Body.Bytes(), []byte(`"user_id":`)) {
		t.Fatalf("expected redemption ledger response, got %s", redemptionsRec.Body.String())
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/audit-logs", nil)
	auditReq.Header.Set("Authorization", "Bearer "+adminToken)
	auditRec := httptest.NewRecorder()
	handler.ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit list 200, got %d body=%s", auditRec.Code, auditRec.Body.String())
	}
	for _, action := range []string{"redeem_code.create", "redeem_code.batch_create", "redeem_code.status_update"} {
		if !bytes.Contains(auditRec.Body.Bytes(), []byte(action)) {
			t.Fatalf("expected audit action %s, got body=%s", action, auditRec.Body.String())
		}
	}
}

func loginAdminForRedeemTest(t *testing.T, handler http.Handler) string {
	t.Helper()
	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"admin-redeem@example.com","password":"password"}`))
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

func jsonNumber(value int64) string {
	return fmt.Sprintf("%d", value)
}
