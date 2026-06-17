package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
	adminstorageservice "github.com/fatballfish/pic-gallery/internal/service/adminstorage"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

func TestAdminStorageConfigEndpoints(t *testing.T) {
	cfg := adminConfigAPIConfig()
	cfg.Storage.Driver = "local"
	cfg.Storage.LocalRoot = "/var/lib/pic-gallery/storage"
	client, err := repoent.Open(dialect.SQLite, "file:admin-storage-api?mode=memory&cache=shared&_fk=1")
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
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "storage-admin@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: domainadminauth.RoleSuperAdmin, Status: "active"}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	api := handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, nil, nil, adminAuth, nil)
	api.SetStorageAdminService(adminstorageservice.NewServiceWithStore(cfg.Storage, entstore.NewStorageStore(client)))
	handler := NewWithAPI(api)
	token := loginAdminWithCredentials(t, handler, "storage-admin@example.com", "password")

	createReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/storage/configs", bytes.NewBufferString(`{"code":"bfss-primary","name":"BFSS Primary","driver":"s3","endpoint":"https://bfss.example.com","region":"us-east-1","bucket":"generated-assets","prefix":"prod","force_path_style":true,"access_key_id":"ak","secret_access_key":"sk","status":"active"}`))
	createReq.Header.Set("Authorization", "Bearer "+token)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected storage config create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	if bytes.Contains(createRec.Body.Bytes(), []byte("sk")) || bytes.Contains(createRec.Body.Bytes(), []byte("ak")) {
		t.Fatalf("storage config response must not expose credentials, body=%s", createRec.Body.String())
	}
	var createResp struct {
		Data struct {
			ID                 int64  `json:"id"`
			Code               string `json:"code"`
			Driver             string `json:"driver"`
			AccessKeyIDSet     bool   `json:"access_key_id_set"`
			SecretAccessKeySet bool   `json:"secret_access_key_set"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createResp.Data.ID <= 0 || createResp.Data.Code != "bfss-primary" || createResp.Data.Driver != "s3" || !createResp.Data.AccessKeyIDSet || !createResp.Data.SecretAccessKeySet {
		t.Fatalf("unexpected create response %#v", createResp.Data)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/storage/configs", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK || !bytes.Contains(listRec.Body.Bytes(), []byte("legacy-default")) || !bytes.Contains(listRec.Body.Bytes(), []byte("bfss-primary")) {
		t.Fatalf("expected storage configs list with legacy and bfss config, got %d body=%s", listRec.Code, listRec.Body.String())
	}

	statsReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/storage/stats", nil)
	statsReq.Header.Set("Authorization", "Bearer "+token)
	statsRec := httptest.NewRecorder()
	handler.ServeHTTP(statsRec, statsReq)
	if statsRec.Code != http.StatusOK || !bytes.Contains(statsRec.Body.Bytes(), []byte("items")) {
		t.Fatalf("expected storage stats 200, got %d body=%s", statsRec.Code, statsRec.Body.String())
	}
}

func TestAdminStorageMigrationDryRunAndUpdateRecords(t *testing.T) {
	cfg := adminConfigAPIConfig()
	sourceRoot := t.TempDir()
	targetRoot := t.TempDir()
	cfg.Storage.Driver = "local"
	cfg.Storage.LocalRoot = sourceRoot
	client, err := repoent.Open(dialect.SQLite, "file:admin-storage-migration?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	authSvc := authservice.NewServiceWithStore(config.AuthConfig{AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: 2 * time.Hour, Issuer: "test", AccessTokenSecret: "secret", RefreshCookieName: "pg_refresh"}, map[string]string{"basic": "1.00000"}, entstore.NewAuthStore(client))
	adminStore := entstore.NewAdminAuthStore(client)
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "migration-admin@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: domainadminauth.RoleSuperAdmin, Status: "active"}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	storageSvc := adminstorageservice.NewServiceWithStore(cfg.Storage, entstore.NewStorageStoreWithLegacyConfig(client, "test-storage-key", cfg.Storage))
	api := handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, nil, nil, adminAuth, nil)
	api.SetStorageAdminService(storageSvc)
	handler := NewWithAPI(api)
	token := loginAdminWithCredentials(t, handler, "migration-admin@example.com", "password")

	targetReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/storage/configs", bytes.NewBufferString(`{"code":"local-target","name":"Local Target","driver":"local","bucket":"`+targetRoot+`","status":"active"}`))
	targetReq.Header.Set("Authorization", "Bearer "+token)
	targetReq.Header.Set("Content-Type", "application/json")
	targetRec := httptest.NewRecorder()
	handler.ServeHTTP(targetRec, targetReq)
	if targetRec.Code != http.StatusCreated {
		t.Fatalf("create target config: %d body=%s", targetRec.Code, targetRec.Body.String())
	}
	var targetResp struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(targetRec.Body).Decode(&targetResp); err != nil {
		t.Fatalf("decode target response: %v", err)
	}

	taskID := uuid.New()
	imageID := uuid.New()
	objectKey := "generated-images/1/" + taskID.String() + "/0-" + imageID.String() + ".png"
	if err := os.MkdirAll(filepath.Dir(filepath.Join(sourceRoot, objectKey)), 0o755); err != nil {
		t.Fatalf("mkdir source object: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, objectKey), []byte("image-bytes"), 0o644); err != nil {
		t.Fatalf("write source object: %v", err)
	}
	if _, err := client.ImageTask.Create().SetID(taskID).SetUserID(1).SetTaskType("text_to_image").SetStatus("succeeded").SetPrompt("p").SetAbstractModel("plus").Save(t.Context()); err != nil {
		t.Fatalf("create image task: %v", err)
	}
	if _, err := client.ImageResult.Create().SetID(imageID).SetTaskID(taskID).SetUserID(1).SetStorageDriver("local").SetObjectKey(objectKey).SetMimeType("image/png").SetFileSizeBytes(11).SetSha256("sha").Save(t.Context()); err != nil {
		t.Fatalf("create image result: %v", err)
	}

	dryRunBody := bytes.NewBufferString(`{"target_storage_config_id":` + storageJSONNumber(targetResp.Data.ID) + `,"scope":{"object_roles":["generated_image"]},"dry_run":true,"update_records":true}`)
	dryRunReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/storage/migrations", dryRunBody)
	dryRunReq.Header.Set("Authorization", "Bearer "+token)
	dryRunReq.Header.Set("Content-Type", "application/json")
	dryRunRec := httptest.NewRecorder()
	handler.ServeHTTP(dryRunRec, dryRunReq)
	if dryRunRec.Code != http.StatusCreated || !bytes.Contains(dryRunRec.Body.Bytes(), []byte(`"total_items":1`)) {
		t.Fatalf("expected dry run migration 201 with one item, got %d body=%s", dryRunRec.Code, dryRunRec.Body.String())
	}
	row, err := client.ImageResult.Get(t.Context(), imageID)
	if err != nil {
		t.Fatalf("load image after dry run: %v", err)
	}
	if row.StorageConfigID != nil {
		t.Fatalf("dry run must not update image storage config, got %v", *row.StorageConfigID)
	}

	runBody := bytes.NewBufferString(`{"target_storage_config_id":` + storageJSONNumber(targetResp.Data.ID) + `,"scope":{"object_roles":["generated_image"]},"dry_run":false,"update_records":true}`)
	runReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/storage/migrations", runBody)
	runReq.Header.Set("Authorization", "Bearer "+token)
	runReq.Header.Set("Content-Type", "application/json")
	runRec := httptest.NewRecorder()
	handler.ServeHTTP(runRec, runReq)
	if runRec.Code != http.StatusCreated || !bytes.Contains(runRec.Body.Bytes(), []byte(`"processed_items":1`)) {
		t.Fatalf("expected migration 201 with processed item, got %d body=%s", runRec.Code, runRec.Body.String())
	}
	row, err = client.ImageResult.Get(t.Context(), imageID)
	if err != nil {
		t.Fatalf("load image after migration: %v", err)
	}
	if row.StorageConfigID == nil || *row.StorageConfigID != targetResp.Data.ID {
		t.Fatalf("expected image storage_config_id=%d after migration, got %v", targetResp.Data.ID, row.StorageConfigID)
	}
	if _, err := os.Stat(filepath.Join(targetRoot, objectKey)); err != nil {
		t.Fatalf("expected copied target object: %v", err)
	}
}

func storageJSONNumber(value int64) string {
	return strconv.FormatInt(value, 10)
}
