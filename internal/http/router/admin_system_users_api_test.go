package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
)

func TestAdminSystemUserManagementEndpoints(t *testing.T) {
	cfg := adminConfigAPIConfig()
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	adminStore := adminauthservice.NewMemoryStore()
	root, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "root-system@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: domainadminauth.RoleSuperAdmin, Status: "active"})
	if err != nil {
		t.Fatalf("CreateAdmin root: %v", err)
	}
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "ops-system@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: domainadminauth.RoleAdmin, Status: "active"}); err != nil {
		t.Fatalf("CreateAdmin ops: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)

	api := handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, nil, nil, adminAuth, nil)
	handler := NewWithAPI(api)
	rootToken := loginSystemAdminForTest(t, handler, "root-system@example.com", "password")
	opsToken := loginSystemAdminForTest(t, handler, "ops-system@example.com", "password")

	deniedReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/admin-users", nil)
	deniedReq.Header.Set("Authorization", "Bearer "+opsToken)
	deniedRec := httptest.NewRecorder()
	handler.ServeHTTP(deniedRec, deniedReq)
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("expected normal admin to be forbidden, got %d body=%s", deniedRec.Code, deniedRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/admin-users?page=1&page_size=20&query=system", nil)
	listReq.Header.Set("Authorization", "Bearer "+rootToken)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listResp struct {
		Data struct {
			Items []struct {
				ID           int64  `json:"id"`
				Email        string `json:"email"`
				Role         string `json:"role"`
				Status       string `json:"status"`
				PasswordHash string `json:"password_hash"`
			} `json:"items"`
			Pagination struct {
				Total int `json:"total"`
			} `json:"pagination"`
		} `json:"data"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if listResp.Data.Pagination.Total != 2 || len(listResp.Data.Items) != 2 {
		t.Fatalf("unexpected list response %#v", listResp)
	}
	for _, item := range listResp.Data.Items {
		if item.PasswordHash != "" {
			t.Fatalf("admin user response must not expose password hash: %#v", item)
		}
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/admin-users", bytes.NewBufferString(`{"email":"created-system@example.com","password":"created-pass","role":"admin","status":"active"}`))
	createReq.Header.Set("Authorization", "Bearer "+rootToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data struct {
			ID     int64  `json:"id"`
			Email  string `json:"email"`
			Role   string `json:"role"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createResp.Data.ID == 0 || createResp.Data.Email != "created-system@example.com" || createResp.Data.Role != domainadminauth.RoleAdmin || createResp.Data.Status != "active" {
		t.Fatalf("unexpected create response %#v", createResp)
	}
	if token := loginSystemAdminForTest(t, handler, "created-system@example.com", "created-pass"); token == "" {
		t.Fatalf("expected created admin to login")
	}

	updateReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/admin-users/"+strconv.FormatInt(createResp.Data.ID, 10), bytes.NewBufferString(`{"status":"disabled","role":"admin"}`))
	updateReq.Header.Set("Authorization", "Bearer "+rootToken)
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected update 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}
	if statusCode := loginSystemAdminStatus(t, handler, "created-system@example.com", "created-pass"); statusCode != http.StatusForbidden {
		t.Fatalf("expected disabled admin login 403, got %d", statusCode)
	}

	resetReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/admin-users/"+strconv.FormatInt(createResp.Data.ID, 10)+"/reset-password", bytes.NewBufferString(`{"new_password":"new-created-pass"}`))
	resetReq.Header.Set("Authorization", "Bearer "+rootToken)
	resetReq.Header.Set("Content-Type", "application/json")
	resetRec := httptest.NewRecorder()
	handler.ServeHTTP(resetRec, resetReq)
	if resetRec.Code != http.StatusOK {
		t.Fatalf("expected reset password 200, got %d body=%s", resetRec.Code, resetRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/ops/admin/v1/admin-users/"+strconv.FormatInt(createResp.Data.ID, 10), nil)
	deleteReq.Header.Set("Authorization", "Bearer "+rootToken)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected delete 200, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	deleteSelfReq := httptest.NewRequest(http.MethodDelete, "/api/ops/admin/v1/admin-users/"+strconv.FormatInt(root.ID, 10), nil)
	deleteSelfReq.Header.Set("Authorization", "Bearer "+rootToken)
	deleteSelfRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteSelfRec, deleteSelfReq)
	if deleteSelfRec.Code != http.StatusBadRequest && deleteSelfRec.Code != http.StatusConflict {
		t.Fatalf("expected deleting self/last root to be rejected, got %d body=%s", deleteSelfRec.Code, deleteSelfRec.Body.String())
	}
}

func loginSystemAdminForTest(t *testing.T, handler http.Handler, email string, password string) string {
	t.Helper()
	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"`+email+`","password":"`+password+`"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected admin login 200 for %s, got %d body=%s", email, loginRec.Code, loginRec.Body.String())
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

func loginSystemAdminStatus(t *testing.T, handler http.Handler, email string, password string) int {
	t.Helper()
	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"`+email+`","password":"`+password+`"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	return loginRec.Code
}
