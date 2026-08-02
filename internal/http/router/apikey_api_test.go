package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
)

func TestAgentAPIKeyLifecycleEndpoints(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	cfg.Auth = config.AuthConfig{AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: 2 * time.Hour, Issuer: "test", AccessTokenSecret: "secret", RefreshCookieName: "pg_refresh"}
	authSvc := authservice.NewService(cfg.Auth, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("keys@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	_, session, err := loginAuthUserWithPasswordSetup(t, authSvc, "keys@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}
	handler := NewWithAPI(handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, nil, nil))

	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/account/v1/api-keys", strings.NewReader(`{"name":"ci","rpm_limit":30}`))
	createReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data struct {
			ID        int64  `json:"id"`
			Secret    string `json:"secret"`
			AccessKey string `json:"access_key"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if createResp.Data.ID == 0 || createResp.Data.Secret == "" || createResp.Data.AccessKey == "" {
		t.Fatalf("unexpected create response %#v", createResp)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/agent/account/v1/api-keys", nil)
	listReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK || strings.Contains(listRec.Body.String(), "secret_hash") {
		t.Fatalf("unexpected list response code=%d body=%s", listRec.Code, listRec.Body.String())
	}

	resetReq := httptest.NewRequest(http.MethodPost, "/api/agent/account/v1/api-keys/"+jsonInt(createResp.Data.ID)+"/reset-secret", nil)
	resetReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	resetRec := httptest.NewRecorder()
	handler.ServeHTTP(resetRec, resetReq)
	if resetRec.Code != http.StatusOK || !strings.Contains(resetRec.Body.String(), "secret") {
		t.Fatalf("unexpected reset response code=%d body=%s", resetRec.Code, resetRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/agent/account/v1/api-keys/"+jsonInt(createResp.Data.ID), nil)
	deleteReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected delete 204, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
}

func jsonInt(value int64) string {
	b, _ := json.Marshal(value)
	return string(b)
}
