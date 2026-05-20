package router

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainapikey "github.com/fatballfish/pic-gallery/internal/domain/apikey"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	apikeyservice "github.com/fatballfish/pic-gallery/internal/service/apikey"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
)

func TestAPIKeysCreateForcesCurrentUserGroupAndPatchRejectsGroupCode(t *testing.T) {
	handler, _, token, _, _, _ := newDeveloperAPIKeyHandler(t)

	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/developer/v1/api-keys", bytes.NewBufferString(`{"name":"attempt-pro","group_code":"pro"}`))
	createReq.Header.Set("Authorization", "Bearer "+token)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data struct {
			APIKey domainapikey.APIKey `json:"api_key"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createResp.Data.APIKey.GroupCode != "basic" {
		t.Fatalf("expected created key to inherit current user's basic group, got %#v", createResp.Data.APIKey)
	}

	patchReq := httptest.NewRequest(http.MethodPatch, "/api/agent/developer/v1/api-keys/"+strconv.FormatInt(createResp.Data.APIKey.ID, 10), bytes.NewBufferString(`{"group_code":"pro"}`))
	patchReq.Header.Set("Authorization", "Bearer "+token)
	patchReq.Header.Set("Content-Type", "application/json")
	patchRec := httptest.NewRecorder()
	handler.ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusBadRequest {
		t.Fatalf("expected group_code patch 400, got %d body=%s", patchRec.Code, patchRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/agent/developer/v1/api-keys", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listResp struct {
		Data struct {
			Items []domainapikey.APIKey `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	for _, item := range listResp.Data.Items {
		if item.ID == createResp.Data.APIKey.ID && item.GroupCode != "basic" {
			t.Fatalf("expected rejected patch not to change group_code, got %#v", item)
		}
	}
}

func TestAPIKeysResetSecretRejectsNonPOSTMethodsWithoutRotatingSecret(t *testing.T) {
	handler, keySvc, token, keyID, accessKey, secret := newDeveloperAPIKeyHandler(t)

	for _, method := range []string{http.MethodGet, http.MethodDelete, http.MethodPut} {
		req := httptest.NewRequest(method, "/api/agent/developer/v1/api-keys/"+strconv.FormatInt(keyID, 10)+"/reset-secret", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected %s reset-secret to return 405, got %d body=%s", method, rec.Code, rec.Body.String())
		}
		if _, err := keySvc.AuthenticateNative(context.Background(), accessKey, secret); err != nil {
			t.Fatalf("expected %s reset-secret not to rotate the secret: %v", method, err)
		}
	}
}

func newDeveloperAPIKeyHandler(t *testing.T) (http.Handler, *apikeyservice.Service, string, int64, string, string) {
	t.Helper()
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("apikey-reset@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	user, session, err := authSvc.LoginWithEmailCode("apikey-reset@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}
	keySvc := apikeyservice.NewService(nil)
	created, err := keySvc.CreateKey(context.Background(), apikeyservice.CreateRequest{
		UserID: user.ID,
		Name:   "reset-secret-method-test",
		Secret: "sk-original-reset-secret",
	})
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, nil, nil, keySvc)
	return NewWithAPI(api), keySvc, session.AccessToken, created.Key.ID, created.Key.AccessKey, created.Secret
}
