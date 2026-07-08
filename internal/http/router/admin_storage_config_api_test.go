package router

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	storageconfigservice "github.com/fatballfish/pic-gallery/internal/service/storageconfig"
	"github.com/fatballfish/pic-gallery/internal/storage"
	_ "github.com/mattn/go-sqlite3"
)

func TestAdminStorageConfigEndpoints(t *testing.T) {
	cfg := adminConfigAPIConfig()
	cfg.Storage.LocalRoot = t.TempDir()
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "ops-storage@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: domainadminauth.RoleAdmin, Status: "active"}); err != nil {
		t.Fatalf("CreateAdmin ops: %v", err)
	}
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "root-storage@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: domainadminauth.RoleSuperAdmin, Status: "active"}); err != nil {
		t.Fatalf("CreateAdmin root: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)

	client, err := repoent.Open(dialect.SQLite, "file:admin-storage-config-api?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	storageSvc := storageconfigservice.NewService(entstore.NewStorageConfigStore(client), "storage-config-api-test-key", cfg.Storage, "test")
	api := handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, nil, nil, adminAuth, nil)
	api.SetStorageConfigService(storageSvc, storage.NewRegistry(storageSvc, time.Second))
	handler := NewWithAPI(api)

	opsToken := loginAdminWithCredentials(t, handler, "ops-storage@example.com", "password")
	rootToken := loginAdminWithCredentials(t, handler, "root-storage@example.com", "password")

	listReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/storage-configs", nil)
	listReq.Header.Set("Authorization", "Bearer "+opsToken)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected normal admin storage list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}

	r2Body := `{"code":"r2-prod","name":"R2 Prod","driver":"s3","provider":"r2","endpoint":"https://example.r2.cloudflarestorage.com","bucket":"pic-gallery","read_enabled":true,"write_enabled":true,"secrets":{"access_key_id":"storage-access","secret_access_key":"storage-secret"}}`
	opsCreate := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/storage-configs", bytes.NewBufferString(r2Body))
	opsCreate.Header.Set("Authorization", "Bearer "+opsToken)
	opsCreate.Header.Set("Content-Type", "application/json")
	opsCreateRec := httptest.NewRecorder()
	handler.ServeHTTP(opsCreateRec, opsCreate)
	if opsCreateRec.Code != http.StatusForbidden {
		t.Fatalf("expected normal admin storage create 403, got %d body=%s", opsCreateRec.Code, opsCreateRec.Body.String())
	}

	rootCreate := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/storage-configs", bytes.NewBufferString(r2Body))
	rootCreate.Header.Set("Authorization", "Bearer "+rootToken)
	rootCreate.Header.Set("Content-Type", "application/json")
	rootCreateRec := httptest.NewRecorder()
	handler.ServeHTTP(rootCreateRec, rootCreate)
	if rootCreateRec.Code != http.StatusCreated {
		t.Fatalf("expected super admin storage create 201, got %d body=%s", rootCreateRec.Code, rootCreateRec.Body.String())
	}
	if strings.Contains(rootCreateRec.Body.String(), "storage-secret") || strings.Contains(rootCreateRec.Body.String(), "storage-access") {
		t.Fatalf("storage create response must not contain plaintext secrets, body=%s", rootCreateRec.Body.String())
	}
	var createResp struct {
		Data struct {
			ID           string `json:"id"`
			Version      int64  `json:"version"`
			Region       string `json:"region"`
			SecretStatus struct {
				HasSecret bool `json:"has_secret"`
			} `json:"secret_status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rootCreateRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createResp.Data.ID == "" || createResp.Data.Region != "auto" || !createResp.Data.SecretStatus.HasSecret {
		t.Fatalf("unexpected storage create response %#v", createResp.Data)
	}

	setDefault := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/storage-configs/"+createResp.Data.ID+":set-default", bytes.NewBufferString(`{"version":1}`))
	setDefault.Header.Set("Authorization", "Bearer "+rootToken)
	setDefault.Header.Set("Content-Type", "application/json")
	setDefaultRec := httptest.NewRecorder()
	handler.ServeHTTP(setDefaultRec, setDefault)
	if setDefaultRec.Code != http.StatusBadRequest {
		t.Fatalf("expected set-default before probe 400, got %d body=%s", setDefaultRec.Code, setDefaultRec.Body.String())
	}

	localProbeBody := `{"name":"Local Probe","driver":"local","provider":"local","local_root":"` + cfg.Storage.LocalRoot + `"}`
	probeReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/storage-configs:probe", bytes.NewBufferString(localProbeBody))
	probeReq.Header.Set("Authorization", "Bearer "+rootToken)
	probeReq.Header.Set("Content-Type", "application/json")
	probeRec := httptest.NewRecorder()
	handler.ServeHTTP(probeRec, probeReq)
	if probeRec.Code != http.StatusOK {
		t.Fatalf("expected draft local probe 200, got %d body=%s", probeRec.Code, probeRec.Body.String())
	}
}
