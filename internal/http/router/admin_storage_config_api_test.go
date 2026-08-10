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

func TestAdminStorageConfigPropagationUpdatesAPIAndWorkerRouters(t *testing.T) {
	harness := newAdminStorageConfigTestHarness(t)
	initialAPI, err := harness.apiRegistry.DefaultWriter(context.Background())
	if err != nil {
		t.Fatalf("prime api registry: %v", err)
	}
	initialWorker, err := harness.workerRegistry.DefaultWriter(context.Background())
	if err != nil {
		t.Fatalf("prime worker registry: %v", err)
	}
	if initialAPI.ConfigID != initialWorker.ConfigID {
		t.Fatalf("initial routers disagree: api=%#v worker=%#v", initialAPI, initialWorker)
	}

	newRoot := t.TempDir()
	draftBody, _ := json.Marshal(map[string]any{"name": "Draft Local", "driver": "local", "provider": "local", "local_root": newRoot})
	draftReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/storage-configs:probe", bytes.NewReader(draftBody))
	draftReq.Header.Set("Authorization", "Bearer "+harness.token)
	draftReq.Header.Set("Content-Type", "application/json")
	draftRec := httptest.NewRecorder()
	harness.handler.ServeHTTP(draftRec, draftReq)
	if draftRec.Code != http.StatusOK || !bytes.Contains(draftRec.Body.Bytes(), []byte(`"status":"success"`)) || bytes.Contains(draftRec.Body.Bytes(), []byte(`"Status"`)) {
		t.Fatalf("draft probe contract: status=%d body=%s", draftRec.Code, draftRec.Body.String())
	}
	createBody, _ := json.Marshal(map[string]any{
		"code": "new-local", "name": "New Local", "driver": "local", "provider": "local",
		"status": "enabled", "read_enabled": true, "write_enabled": true, "local_root": newRoot,
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/storage-configs", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", "Bearer "+harness.token)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	harness.handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create storage config: status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		Data struct {
			ID      string `json:"id"`
			Version int64  `json:"version"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	probeReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/storage-configs/"+created.Data.ID+":probe", nil)
	probeReq.Header.Set("Authorization", "Bearer "+harness.token)
	probeRec := httptest.NewRecorder()
	harness.handler.ServeHTTP(probeRec, probeReq)
	if probeRec.Code != http.StatusOK {
		t.Fatalf("probe storage config: status=%d body=%s", probeRec.Code, probeRec.Body.String())
	}
	var probed struct {
		Data struct {
			Version int64 `json:"version"`
		} `json:"data"`
	}
	if err := json.NewDecoder(probeRec.Body).Decode(&probed); err != nil {
		t.Fatalf("decode probe: %v", err)
	}

	defaultBody, _ := json.Marshal(map[string]any{"version": probed.Data.Version})
	defaultReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/storage-configs/"+created.Data.ID+":set-default", bytes.NewReader(defaultBody))
	defaultReq.Header.Set("Authorization", "Bearer "+harness.token)
	defaultReq.Header.Set("Content-Type", "application/json")
	defaultRec := httptest.NewRecorder()
	harness.handler.ServeHTTP(defaultRec, defaultReq)
	if defaultRec.Code != http.StatusOK {
		t.Fatalf("set default: status=%d body=%s", defaultRec.Code, defaultRec.Body.String())
	}

	apiWriter, err := harness.apiRegistry.DefaultWriter(context.Background())
	if err != nil {
		t.Fatalf("api writer after default switch: %v", err)
	}
	workerWriter, err := harness.workerRegistry.DefaultWriter(context.Background())
	if err != nil {
		t.Fatalf("worker writer after default switch: %v", err)
	}
	if apiWriter.ConfigID != created.Data.ID || workerWriter.ConfigID != created.Data.ID {
		t.Fatalf("storage default did not propagate: api=%#v worker=%#v expected=%s events=%#v", apiWriter, workerWriter, created.Data.ID, harness.publisher.events)
	}
}

func TestAdminStorageConfigMalformedIDReturnsNotFound(t *testing.T) {
	harness := newAdminStorageConfigTestHarness(t)
	cases := []struct {
		name   string
		method string
		suffix string
		body   string
	}{
		{name: "get", method: http.MethodGet},
		{name: "update", method: http.MethodPut, body: `{}`},
		{name: "probe", method: http.MethodPost, suffix: ":probe"},
		{name: "set default", method: http.MethodPost, suffix: ":set-default", body: `{}`},
		{name: "set status", method: http.MethodPost, suffix: ":set-status", body: `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/api/ops/admin/v1/storage-configs/{storage_config_id}"+tc.suffix, strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer "+harness.token)
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			harness.handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
			}
			var response struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if response.Error.Code != "NOT_FOUND" {
				t.Fatalf("expected NOT_FOUND, got %#v", response)
			}
		})
	}
}

func TestAdminStorageConfigUpdateRejectsNamespaceChanges(t *testing.T) {
	harness := newAdminStorageConfigTestHarness(t)
	listReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/storage-configs", nil)
	listReq.Header.Set("Authorization", "Bearer "+harness.token)
	listRec := httptest.NewRecorder()
	harness.handler.ServeHTTP(listRec, listReq)
	var listed struct {
		Data struct {
			Items []struct {
				ID        string `json:"id"`
				Code      string `json:"code"`
				Name      string `json:"name"`
				Version   int64  `json:"version"`
				LocalRoot string `json:"local_root"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&listed); err != nil || len(listed.Data.Items) == 0 {
		t.Fatalf("list configs: status=%d body=%s err=%v", listRec.Code, listRec.Body.String(), err)
	}
	current := listed.Data.Items[0]
	body, _ := json.Marshal(map[string]any{
		"version": current.Version, "code": current.Code, "name": current.Name, "driver": "local", "provider": "local",
		"status": "enabled", "read_enabled": true, "write_enabled": true, "local_root": current.LocalRoot + "-moved",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/storage-configs/"+current.ID, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+harness.token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	harness.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict || !bytes.Contains(rec.Body.Bytes(), []byte(`"code":"STORAGE_NAMESPACE_IMMUTABLE"`)) {
		t.Fatalf("namespace update: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

type adminStorageConfigTestHarness struct {
	handler        http.Handler
	token          string
	apiRegistry    *storage.Registry
	workerRegistry *storage.Registry
	publisher      *broadcastStorageInvalidation
}

func newAdminStorageConfigTestHarness(t *testing.T) adminStorageConfigTestHarness {
	t.Helper()
	cfg := adminConfigAPIConfig()
	cfg.Storage.Driver = "local"
	cfg.Storage.LocalRoot = t.TempDir()
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: 2 * time.Hour, Issuer: "test",
		AccessTokenSecret: "secret", RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "root-storage@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: domainadminauth.RoleSuperAdmin, Status: "active"}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)

	databaseName := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	client, err := repoent.Open(dialect.SQLite, "file:"+databaseName+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	storageSvc := storageconfigservice.NewService(entstore.NewStorageConfigStore(client), "storage-config-api-test-key", cfg.Storage, "test")
	if err := storageSvc.Bootstrap(context.Background(), 0); err != nil {
		t.Fatalf("bootstrap storage: %v", err)
	}
	apiRegistry := storage.NewRegistry(storageSvc, time.Hour)
	workerRegistry := storage.NewRegistry(storageSvc, time.Hour)
	publisher := &broadcastStorageInvalidation{registries: []*storage.Registry{apiRegistry, workerRegistry}}
	api := handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, nil, nil, adminAuth, nil)
	api.SetStorageConfigService(storageSvc, apiRegistry, publisher)
	handler := NewWithAPI(api)
	token := loginAdminWithCredentials(t, handler, "root-storage@example.com", "password")
	return adminStorageConfigTestHarness{handler: handler, token: token, apiRegistry: apiRegistry, workerRegistry: workerRegistry, publisher: publisher}
}

type broadcastStorageInvalidation struct {
	registries []*storage.Registry
	events     []storage.StorageInvalidation
}

func (p *broadcastStorageInvalidation) Publish(_ context.Context, event storage.StorageInvalidation) error {
	p.events = append(p.events, event)
	for _, registry := range p.registries {
		registry.Invalidate(event)
	}
	return nil
}
