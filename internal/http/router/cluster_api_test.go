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
	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	domainaudit "github.com/fatballfish/pic-gallery/internal/domain/audit"
	domaincluster "github.com/fatballfish/pic-gallery/internal/domain/cluster"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
	auditservice "github.com/fatballfish/pic-gallery/internal/service/audit"
	clusterservice "github.com/fatballfish/pic-gallery/internal/service/cluster"
	_ "github.com/mattn/go-sqlite3"
)

func TestClusterAdminTokenLifecycleIsProtectedAndSecretSafe(t *testing.T) {
	harness := newClusterAPIHarness(t, domainadminauth.RoleSuperAdmin, domaincluster.NodeRoleControl)
	createReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cluster/tokens", bytes.NewBufferString(`{"role":"worker","ttl_seconds":600}`))
	createReq.Header.Set("Authorization", "Bearer "+harness.token)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	harness.handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create token status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		Data domaincluster.IssuedToken `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Data.Credential == "" || created.Data.Token.TokenID == "" {
		t.Fatalf("create token response = %#v", created)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/cluster/tokens?page=1&page_size=20", nil)
	listReq.Header.Set("Authorization", "Bearer "+harness.token)
	listRec := httptest.NewRecorder()
	harness.handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK || bytes.Contains(listRec.Body.Bytes(), []byte(created.Data.Credential)) || bytes.Contains(listRec.Body.Bytes(), []byte("token_hash")) {
		t.Fatalf("list token status=%d body=%s", listRec.Code, listRec.Body.String())
	}

	revokeReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cluster/tokens/"+created.Data.Token.TokenID+":revoke", nil)
	revokeReq.Header.Set("Authorization", "Bearer "+harness.token)
	revokeRec := httptest.NewRecorder()
	harness.handler.ServeHTTP(revokeRec, revokeReq)
	if revokeRec.Code != http.StatusOK || bytes.Contains(revokeRec.Body.Bytes(), []byte(created.Data.Credential)) {
		t.Fatalf("revoke token status=%d body=%s", revokeRec.Code, revokeRec.Body.String())
	}
	logs, err := harness.audit.List(t.Context(), domainaudit.ListRequest{Page: 1, PageSize: 20, TargetType: "cluster_token"})
	if err != nil || logs.Total != 2 {
		t.Fatalf("cluster audit logs = %#v, %v", logs, err)
	}
	encoded, _ := json.Marshal(logs)
	if bytes.Contains(encoded, []byte(created.Data.Credential)) || bytes.Contains(encoded, []byte("token_hash")) {
		t.Fatalf("cluster audit exposed credential material: %s", encoded)
	}
}

func TestClusterTokenAdministrationRequiresDangerousPermissionAndControlRole(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		adminRole string
		nodeRole  domaincluster.NodeRole
	}{
		{name: "ordinary admin", adminRole: domainadminauth.RoleAdmin, nodeRole: domaincluster.NodeRoleControl},
		{name: "api replica", adminRole: domainadminauth.RoleSuperAdmin, nodeRole: domaincluster.NodeRoleAPI},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			harness := newClusterAPIHarness(t, testCase.adminRole, testCase.nodeRole)
			req := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cluster/tokens", strings.NewReader(`{"role":"api","ttl_seconds":600}`))
			req.Header.Set("Authorization", "Bearer "+harness.token)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			harness.handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("create token status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestClusterPublicProtocolRoutesAreExplicitlyUnavailableUntilEncryptedHandshake(t *testing.T) {
	harness := newClusterAPIHarness(t, domainadminauth.RoleSuperAdmin, domaincluster.NodeRoleControl)
	for _, path := range []string{"/api/open/cluster/v1/challenges", "/api/open/cluster/v1/join"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"credential":"must-not-echo"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		harness.handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotImplemented || strings.Contains(rec.Body.String(), "must-not-echo") {
			t.Fatalf("protocol placeholder %s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}
}

type clusterAPIHarness struct {
	handler http.Handler
	token   string
	audit   *auditservice.Service
}

func newClusterAPIHarness(t *testing.T, adminRole string, nodeRole domaincluster.NodeRole) clusterAPIHarness {
	t.Helper()
	cfg := adminConfigAPIConfig()
	cfg.Runtime.InstallationID = "019d0000-0000-7000-8000-000000000991"
	cfg.Runtime.DeploymentRole = config.DeploymentRole(nodeRole)
	cfg.Runtime.ApplicationVersion = "v1"
	cfg.Runtime.ConfigSchemaVersion = 1
	cfg.Runtime.ConfigRevision = 5
	adminStore := adminauthservice.NewMemoryStore()
	email := strings.ReplaceAll(adminRole, "_", "-") + "-cluster@example.com"
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email: email, PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: adminRole, Status: "active",
	}); err != nil {
		t.Fatal(err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	client, err := repoent.Open(dialect.SQLite, "file:"+strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatal(err)
	}
	operationID, digest, adminID, revision := uuid.NewString(), strings.Repeat("e", 64), int64(1), 5
	if _, err := client.Installation.Create().
		SetSingletonKey("installation").SetInstallationID(cfg.Runtime.InstallationID).
		SetConfigSchemaVersion(1).SetDatabaseSchemaVersion(1).SetAppVersion("v1").
		SetSetupOperationID(operationID).SetSetupAdminID(adminID).SetSetupConfigRevision(revision).SetSetupRequestDigest(digest).
		Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	audit := auditservice.NewService(entstore.NewAuditStore(client))
	cluster := clusterservice.NewService(clusterservice.ServiceOptions{
		Store: entstore.NewClusterStore(client), InstallationID: cfg.Runtime.InstallationID,
		DeploymentRole: nodeRole, Now: func() time.Time { return time.Date(2026, 7, 23, 6, 0, 0, 0, time.UTC) },
	})
	api := handlers.NewAPIWithCompletionServices(cfg, nil, nil, nil, nil, nil, nil, adminAuth, audit)
	api.SetClusterService(cluster)
	handler := NewWithAPI(api)
	return clusterAPIHarness{handler: handler, token: loginAdminWithCredentials(t, handler, email, "password"), audit: audit}
}
