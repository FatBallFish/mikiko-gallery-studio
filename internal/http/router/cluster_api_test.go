package router

import (
	"bytes"
	"context"
	"encoding/base64"
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

func TestClusterNodeListIsReadOnlyAndAvailableThroughAPIReplicas(t *testing.T) {
	harness := newClusterAPIHarness(t, domainadminauth.RoleAdmin, domaincluster.NodeRoleAPI)
	now := time.Date(2026, 7, 23, 5, 59, 50, 0, time.UTC)
	if _, err := harness.store.CreateNode(t.Context(), domaincluster.Node{
		NodeID: "worker-list-a", InstallationID: harness.installationID, Role: domaincluster.NodeRoleWorker,
		ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 5,
		Health: domaincluster.NodeHealthHealthy, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/cluster/nodes?page=1&page_size=20", nil)
	req.Header.Set("Authorization", "Bearer "+harness.token)
	rec := httptest.NewRecorder()
	harness.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list cluster nodes status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			Items []domaincluster.NodeStatus `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil || len(response.Data.Items) != 1 || response.Data.Items[0].NodeID != "worker-list-a" {
		t.Fatalf("list cluster nodes response=%#v err=%v", response, err)
	}
	post := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cluster/nodes", strings.NewReader(`{}`))
	post.Header.Set("Authorization", "Bearer "+harness.token)
	postRec := httptest.NewRecorder()
	harness.handler.ServeHTTP(postRec, post)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("cluster nodes accepted mutation status=%d body=%s", postRec.Code, postRec.Body.String())
	}
}

func TestClusterPublicProtocolEncryptsEnrollmentWithoutCredentialInHTTPBodies(t *testing.T) {
	harness := newClusterAPIHarness(t, domainadminauth.RoleSuperAdmin, domaincluster.NodeRoleControl)
	issued, err := harness.cluster.CreateToken(t.Context(), domaincluster.CreateTokenRequest{Role: domaincluster.JoinRoleWorker, TTL: time.Hour, ActorID: "1"})
	if err != nil {
		t.Fatal(err)
	}
	clientKey, err := clusterservice.GenerateEphemeralKey(bytes.NewReader(bytes.Repeat([]byte{0x41}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	challengeBody := mustClusterJSON(t, domaincluster.CreateChallengeRequest{
		Protocol: clusterservice.EnrollmentProtocolV1, TokenID: issued.Token.TokenID, NodeID: "worker-http-a",
		NodePublicKey: clientKey.PublicKey(), ApplicationVersion: "v1", RuntimeSchemaVersion: 1,
	})
	challengeReq := httptest.NewRequest(http.MethodPost, "/api/open/cluster/v1/challenges", bytes.NewReader(challengeBody))
	challengeReq.Header.Set("Content-Type", "application/json")
	challengeRec := httptest.NewRecorder()
	harness.handler.ServeHTTP(challengeRec, challengeReq)
	if challengeRec.Code != http.StatusCreated || bytes.Contains(challengeBody, []byte(issued.Credential)) || strings.Contains(challengeRec.Body.String(), issued.Credential) {
		t.Fatalf("challenge status=%d body=%s", challengeRec.Code, challengeRec.Body.String())
	}
	var challengeResponse struct {
		Data domaincluster.EnrollmentChallenge `json:"data"`
	}
	if err := json.NewDecoder(challengeRec.Body).Decode(&challengeResponse); err != nil {
		t.Fatal(err)
	}
	proofKey, err := clusterservice.TokenProofKeyFromCredential(issued.Credential)
	if err != nil {
		t.Fatal(err)
	}
	defer proofKey.Clear()
	proof, err := clusterservice.ComputeClientPossessionProof(proofKey, challengeResponse.Data, "worker-http-a")
	if err != nil {
		t.Fatal(err)
	}
	joinBody := mustClusterJSON(t, domaincluster.JoinRequest{Protocol: clusterservice.EnrollmentProtocolV1, ChallengeID: challengeResponse.Data.ChallengeID, Proof: proof})
	joinReq := httptest.NewRequest(http.MethodPost, "/api/open/cluster/v1/join", bytes.NewReader(joinBody))
	joinReq.Header.Set("Content-Type", "application/json")
	joinRec := httptest.NewRecorder()
	harness.handler.ServeHTTP(joinRec, joinReq)
	if joinRec.Code != http.StatusCreated || bytes.Contains(joinBody, []byte(issued.Credential)) || strings.Contains(joinRec.Body.String(), issued.Credential) || strings.Contains(joinRec.Body.String(), "postgres://") {
		t.Fatalf("join status=%d body=%s", joinRec.Code, joinRec.Body.String())
	}
	var joinResponse struct {
		Data domaincluster.JoinResponse `json:"data"`
	}
	if err := json.NewDecoder(joinRec.Body).Decode(&joinResponse); err != nil {
		t.Fatal(err)
	}
	payload, err := clusterservice.OpenRuntimeEnvelope(clientKey, proofKey, challengeResponse.Data, "worker-http-a", joinResponse.Data.EncryptedEnvelope)
	if err != nil || payload.Values["DATABASE_URL"] != "postgres://app:password@db:5432/app" {
		t.Fatalf("opened HTTP envelope = %#v, %v", payload, err)
	}
}

type clusterAPIHarness struct {
	handler        http.Handler
	token          string
	audit          *auditservice.Service
	cluster        *clusterservice.Service
	store          *entstore.ClusterStore
	installationID string
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
	clusterStore := entstore.NewClusterStore(client)
	cluster := clusterservice.NewService(clusterservice.ServiceOptions{
		Store: clusterStore, InstallationID: cfg.Runtime.InstallationID,
		DeploymentRole: nodeRole, Now: func() time.Time { return time.Date(2026, 7, 23, 6, 0, 0, 0, time.UTC) },
		RuntimeValues: map[string]string{
			"DATABASE_URL": "postgres://app:password@db:5432/app", "REDIS_URL": "redis://redis:6379/0", "REDIS_KEY_PREFIX": "app",
			"STORAGE_DRIVER": "s3", "STORAGE_S3_ENDPOINT": "http://minio:9000", "STORAGE_S3_REGION": "us-east-1",
			"STORAGE_S3_BUCKET": "assets", "STORAGE_S3_ACCESS_KEY_ID": "access", "STORAGE_S3_SECRET_ACCESS_KEY": "secret",
			"STORAGE_S3_FORCE_PATH_STYLE": "true", "PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY": "secure",
		},
		EnrollmentSealKey: base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32)),
		Entropy:           bytes.NewReader(bytes.Repeat([]byte{0x43}, 4096)),
	})
	api := handlers.NewAPIWithCompletionServices(cfg, nil, nil, nil, nil, nil, nil, adminAuth, audit)
	api.SetClusterService(cluster)
	handler := NewWithAPI(api)
	return clusterAPIHarness{
		handler: handler, token: loginAdminWithCredentials(t, handler, email, "password"), audit: audit, cluster: cluster,
		store: clusterStore, installationID: cfg.Runtime.InstallationID,
	}
}

func mustClusterJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
