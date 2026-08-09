package entstore_test

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"

	domaincluster "github.com/fatballfish/pic-gallery/internal/domain/cluster"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/clustertoken"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	clusterservice "github.com/fatballfish/pic-gallery/internal/service/cluster"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

func TestClusterStorePersistsHashOnlyTokensAndAtomicallyConsumesThem(t *testing.T) {
	client := newClusterStoreClient(t)
	installationID := "019d0000-0000-7000-8000-000000000881"
	seedInitializedClusterInstallation(t, client, installationID)
	store := entstore.NewClusterStore(client)

	installation, err := store.GetInstallation(t.Context(), installationID)
	if err != nil || !installation.Initialized || installation.ApplicationVersion != "v1" || installation.RuntimeSchemaVersion != 1 || installation.ConfigRevision != 4 {
		t.Fatalf("installation = %#v, %v", installation, err)
	}
	now := time.Date(2026, 7, 23, 5, 0, 0, 0, time.UTC)
	record := domaincluster.TokenRecord{
		Token: domaincluster.Token{
			TokenID: uuid.NewString(), InstallationID: installationID, Role: domaincluster.JoinRoleWorker,
			ExpiresAt: now.Add(time.Hour), CreatedBy: "admin-7", CreatedAt: now, UpdatedAt: now,
		},
		TokenHash: strings.Repeat("a", 64), TokenProofPublicKey: base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("p", 32))),
	}
	created, err := store.CreateToken(t.Context(), record)
	if err != nil || created.TokenHash != record.TokenHash {
		t.Fatalf("create token = %#v, %v", created, err)
	}
	entity, err := client.ClusterToken.Query().Where(clustertoken.TokenIDEQ(record.TokenID)).Only(t.Context())
	if err != nil || entity.TokenHash != record.TokenHash || entity.TokenProofPublicKey != record.TokenProofPublicKey {
		t.Fatalf("stored token = %#v, %v", entity, err)
	}
	encoded, err := json.Marshal(entity)
	if err != nil || strings.Contains(string(encoded), record.TokenHash) || strings.Contains(string(encoded), record.TokenProofPublicKey) || strings.Contains(string(encoded), "token_hash") || strings.Contains(string(encoded), "token_proof_public_key") {
		t.Fatalf("stored token JSON exposed hash: %s, %v", encoded, err)
	}
	consumed, node, err := store.AcceptEnrollment(t.Context(), installationID, record.TokenID, record.TokenHash, domaincluster.Node{
		NodeID: "worker-node-1", InstallationID: installationID, Role: domaincluster.NodeRoleWorker,
		ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 4, Health: domaincluster.NodeHealthJoining,
	}, now.Add(time.Minute))
	if err != nil || consumed.ConsumedAt == nil || consumed.ConsumedByNodeID != "worker-node-1" || node.NodeID != "worker-node-1" {
		t.Fatalf("accept enrollment = %#v, %#v, %v", consumed, node, err)
	}
	entity, err = client.ClusterToken.Query().Where(clustertoken.TokenIDEQ(record.TokenID)).Only(t.Context())
	if err != nil || entity.ConsumedByNodeID == nil || *entity.ConsumedByNodeID != "worker-node-1" {
		t.Fatalf("stored consumption identity = %#v, %v", entity, err)
	}
	if _, _, err := store.AcceptEnrollment(t.Context(), installationID, record.TokenID, record.TokenHash, domaincluster.Node{
		NodeID: "worker-node-2", InstallationID: installationID, Role: domaincluster.NodeRoleWorker,
		ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 4, Health: domaincluster.NodeHealthJoining,
	}, now.Add(2*time.Minute)); !errors.Is(err, clusterservice.ErrTokenUnavailable) {
		t.Fatalf("second consumption error = %v", err)
	}
	if _, _, err := store.AcceptEnrollment(t.Context(), installationID, record.TokenID, strings.Repeat("b", 64), domaincluster.Node{
		NodeID: "worker-node-2", InstallationID: installationID, Role: domaincluster.NodeRoleWorker,
		ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 4, Health: domaincluster.NodeHealthJoining,
	}, now.Add(2*time.Minute)); !errors.Is(err, clusterservice.ErrTokenNotFound) {
		t.Fatalf("wrong hash error = %v", err)
	}
	if count, countErr := client.AuditLog.Query().Count(t.Context()); countErr != nil || count != 3 {
		t.Fatalf("transactional enrollment audit count = %d, %v", count, countErr)
	}
}

func TestClusterStoreEnrollmentConflictRollsBackTokenAndAudit(t *testing.T) {
	client := newClusterStoreClient(t)
	installationID := "019d0000-0000-7000-8000-000000000884"
	seedInitializedClusterInstallation(t, client, installationID)
	store := entstore.NewClusterStore(client)
	now := time.Now().UTC()
	if _, err := store.CreateNode(t.Context(), domaincluster.Node{
		NodeID: "shared-node", InstallationID: installationID, Role: domaincluster.NodeRoleAPI,
		ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 4,
		Health: domaincluster.NodeHealthJoining, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	record, err := store.CreateToken(t.Context(), domaincluster.TokenRecord{
		Token: domaincluster.Token{
			TokenID: uuid.NewString(), InstallationID: installationID, Role: domaincluster.JoinRoleWorker,
			ExpiresAt: now.Add(time.Hour), CreatedBy: "admin-1", CreatedAt: now, UpdatedAt: now,
		}, TokenHash: strings.Repeat("f", 64), TokenProofPublicKey: testTokenProofPublicKey(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AcceptEnrollment(t.Context(), installationID, record.TokenID, record.TokenHash, domaincluster.Node{
		NodeID: "wrong-installation-node", InstallationID: "wrong-installation", Role: domaincluster.NodeRoleWorker,
		ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 4, Health: domaincluster.NodeHealthJoining,
	}, now.Add(30*time.Second)); !errors.Is(err, clusterservice.ErrNodeConflict) {
		t.Fatalf("cross-installation enrollment error = %v", err)
	}
	if _, _, err := store.AcceptEnrollment(t.Context(), installationID, record.TokenID, record.TokenHash, domaincluster.Node{
		NodeID: "shared-node", InstallationID: installationID, Role: domaincluster.NodeRoleWorker,
		ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 4, Health: domaincluster.NodeHealthJoining,
	}, now.Add(time.Minute)); !errors.Is(err, clusterservice.ErrNodeConflict) {
		t.Fatalf("conflicting enrollment error = %v", err)
	}
	entity, err := client.ClusterToken.Query().Where(clustertoken.TokenIDEQ(record.TokenID)).Only(t.Context())
	if err != nil || entity.ConsumedAt != nil || entity.ConsumedByNodeID != nil {
		t.Fatalf("conflicting enrollment consumed token: %#v, %v", entity, err)
	}
	if count, err := client.AuditLog.Query().Count(t.Context()); err != nil || count != 1 {
		t.Fatalf("conflicting enrollment audit count = %d, %v", count, err)
	}
}

func TestClusterStoreConcurrentEnrollmentIsAtomicOnPostgres(t *testing.T) {
	adminURL := strings.TrimSpace(os.Getenv("PIC_GALLERY_TEST_POSTGRES_URL"))
	if adminURL == "" {
		t.Skip("set PIC_GALLERY_TEST_POSTGRES_URL to run PostgreSQL cluster enrollment integration")
	}
	database, err := sql.Open("postgres", adminURL)
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	defer database.Close()
	schemaName := fmt.Sprintf("cluster_enroll_%d", time.Now().UnixNano())
	if _, err := database.ExecContext(t.Context(), `CREATE SCHEMA `+schemaName); err != nil {
		t.Fatalf("create integration schema: %v", err)
	}
	t.Cleanup(func() { _, _ = database.Exec(`DROP SCHEMA IF EXISTS ` + schemaName + ` CASCADE`) })
	client, err := repoent.Open(dialect.Postgres, postgresURLWithSearchPath(t, adminURL, schemaName))
	if err != nil {
		t.Fatalf("open ent integration client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatalf("create integration schema tables: %v", err)
	}
	installationID := "019d0000-0000-7000-8000-000000000885"
	seedInitializedClusterInstallation(t, client, installationID)
	store := entstore.NewClusterStore(client)
	now := time.Now().UTC()
	record, err := store.CreateToken(t.Context(), domaincluster.TokenRecord{
		Token: domaincluster.Token{
			TokenID: uuid.NewString(), InstallationID: installationID, Role: domaincluster.JoinRoleWorker,
			ExpiresAt: now.Add(time.Hour), CreatedBy: "admin-1", CreatedAt: now, UpdatedAt: now,
		}, TokenHash: strings.Repeat("9", 64), TokenProofPublicKey: testTokenProofPublicKey(),
	})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errorsByNode := make(chan error, 2)
	var wait sync.WaitGroup
	for _, nodeID := range []string{"worker-concurrent-a", "worker-concurrent-b"} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, _, enrollmentErr := store.AcceptEnrollment(context.Background(), installationID, record.TokenID, record.TokenHash, domaincluster.Node{
				NodeID: nodeID, InstallationID: installationID, Role: domaincluster.NodeRoleWorker,
				ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 4,
				Health: domaincluster.NodeHealthJoining, CreatedAt: now, UpdatedAt: now,
			}, now.Add(time.Minute))
			errorsByNode <- enrollmentErr
		}()
	}
	close(start)
	wait.Wait()
	close(errorsByNode)
	succeeded, rejected := 0, 0
	for enrollmentErr := range errorsByNode {
		switch {
		case enrollmentErr == nil:
			succeeded++
		case errors.Is(enrollmentErr, clusterservice.ErrTokenUnavailable):
			rejected++
		default:
			t.Fatalf("concurrent enrollment error = %v", enrollmentErr)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("concurrent enrollment results: succeeded=%d rejected=%d", succeeded, rejected)
	}
	if nodes, err := client.ClusterNode.Query().Count(t.Context()); err != nil || nodes != 1 {
		t.Fatalf("cluster node count = %d, %v", nodes, err)
	}
	if audits, err := client.AuditLog.Query().Count(t.Context()); err != nil || audits != 3 {
		t.Fatalf("cluster enrollment audit count = %d, %v", audits, err)
	}
}

func TestClusterStoreListsRevokesAndKeepsInstallationBoundaries(t *testing.T) {
	client := newClusterStoreClient(t)
	installationID := "019d0000-0000-7000-8000-000000000882"
	seedInitializedClusterInstallation(t, client, installationID)
	store := entstore.NewClusterStore(client)
	now := time.Now().UTC()
	record, err := store.CreateToken(t.Context(), domaincluster.TokenRecord{
		Token: domaincluster.Token{
			TokenID: uuid.NewString(), InstallationID: installationID, Role: domaincluster.JoinRoleAPI,
			ExpiresAt: now.Add(time.Hour), CreatedBy: "admin-1", CreatedAt: now, UpdatedAt: now,
		}, TokenHash: strings.Repeat("c", 64), TokenProofPublicKey: testTokenProofPublicKey(),
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err := store.ListTokens(t.Context(), installationID, domaincluster.ListTokensRequest{Page: 1, PageSize: 20, Role: domaincluster.JoinRoleAPI, At: now})
	if err != nil || page.Total != 1 || page.Items[0].TokenID != record.TokenID {
		t.Fatalf("list tokens = %#v, %v", page, err)
	}
	revoked, err := store.RevokeToken(t.Context(), installationID, record.TokenID, "admin-2", now.Add(time.Minute))
	if err != nil || revoked.RevokedAt == nil {
		t.Fatalf("revoke token = %#v, %v", revoked, err)
	}
	if _, err := store.RevokeToken(t.Context(), installationID, record.TokenID, "admin-2", now.Add(2*time.Minute)); !errors.Is(err, clusterservice.ErrTokenUnavailable) {
		t.Fatalf("second revoke error = %v", err)
	}
	if _, err := store.RevokeToken(t.Context(), "wrong-installation", record.TokenID, "admin-2", now); !errors.Is(err, clusterservice.ErrTokenNotFound) {
		t.Fatalf("cross-installation revoke error = %v", err)
	}
	expired, err := store.CreateToken(t.Context(), domaincluster.TokenRecord{
		Token: domaincluster.Token{
			TokenID: uuid.NewString(), InstallationID: installationID, Role: domaincluster.JoinRoleWorker,
			ExpiresAt: now.Add(-time.Minute), CreatedBy: "admin-1", CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
		}, TokenHash: strings.Repeat("e", 64), TokenProofPublicKey: testTokenProofPublicKey(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RevokeToken(t.Context(), installationID, expired.TokenID, "admin-2", now); !errors.Is(err, clusterservice.ErrTokenUnavailable) {
		t.Fatalf("expired token revoke error = %v", err)
	}
}

func TestClusterStoreEnforcesNodeIdentityAndUpdatesHeartbeat(t *testing.T) {
	client := newClusterStoreClient(t)
	installationID := "019d0000-0000-7000-8000-000000000883"
	seedInitializedClusterInstallation(t, client, installationID)
	store := entstore.NewClusterStore(client)
	now := time.Now().UTC()
	node, err := store.CreateNode(t.Context(), domaincluster.Node{
		NodeID: "api-node-1", InstallationID: installationID, Role: domaincluster.NodeRoleAPI,
		ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 4,
		Health: domaincluster.NodeHealthJoining, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil || node.NodeID != "api-node-1" {
		t.Fatalf("create node = %#v, %v", node, err)
	}
	if _, err := store.CreateNode(t.Context(), domaincluster.Node{
		NodeID: "api-node-1", InstallationID: installationID, Role: domaincluster.NodeRoleWorker,
		ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 4, Health: domaincluster.NodeHealthJoining,
	}); !errors.Is(err, clusterservice.ErrNodeConflict) {
		t.Fatalf("conflicting node error = %v", err)
	}
	if _, err := store.CreateNode(t.Context(), domaincluster.Node{
		NodeID: "api-node-1", InstallationID: installationID, Role: domaincluster.NodeRoleAPI,
		ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 5, Health: domaincluster.NodeHealthJoining,
	}); !errors.Is(err, clusterservice.ErrNodeConflict) {
		t.Fatalf("config revision conflict error = %v", err)
	}
	heartbeatAt := now.Add(10 * time.Second)
	updated, err := store.HeartbeatNode(t.Context(), installationID, domaincluster.HeartbeatRequest{
		NodeID: "api-node-1", Role: domaincluster.NodeRoleAPI, Health: domaincluster.NodeHealthHealthy, ApplicationVersion: "v1",
		RuntimeSchemaVersion: 1, ConfigRevision: 5,
	}, heartbeatAt)
	if err != nil || updated.LastHeartbeatAt == nil || !updated.LastHeartbeatAt.Equal(heartbeatAt) || updated.ConfigRevision != 5 {
		t.Fatalf("heartbeat = %#v, %v", updated, err)
	}
	if _, err := store.HeartbeatNode(t.Context(), "wrong-installation", domaincluster.HeartbeatRequest{
		NodeID: "api-node-1", Role: domaincluster.NodeRoleAPI, Health: domaincluster.NodeHealthHealthy, ApplicationVersion: "v1",
		RuntimeSchemaVersion: 1, ConfigRevision: 5,
	}, heartbeatAt); !errors.Is(err, clusterservice.ErrNodeNotFound) {
		t.Fatalf("cross-installation heartbeat error = %v", err)
	}
	items, total, err := store.ListNodes(t.Context(), installationID, domaincluster.ListNodesRequest{Page: 1, PageSize: 20, Role: domaincluster.NodeRoleAPI})
	if err != nil || total != 1 || len(items) != 1 || items[0].NodeID != "api-node-1" {
		t.Fatalf("list cluster nodes = %#v total=%d err=%v", items, total, err)
	}
}

func TestClusterStorePersistsLogicalSingleComponentHeartbeat(t *testing.T) {
	client := newClusterStoreClient(t)
	installationID := "019d0000-0000-7000-8000-000000000884"
	seedInitializedClusterInstallation(t, client, installationID)
	now := time.Now().UTC()
	store := entstore.NewClusterStore(client)
	runner, err := clusterservice.NewHeartbeatRunner(clusterservice.HeartbeatOptions{
		Store: store, InstallationID: installationID,
		NodeID: clusterservice.LogicalSingleComponentNodeID(installationID, domaincluster.NodeRoleWorker),
		Role:   domaincluster.NodeRoleWorker, ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 4,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new heartbeat runner: %v", err)
	}
	if _, err := runner.Pulse(t.Context()); err != nil {
		t.Fatalf("pulse logical-single Worker: %v", err)
	}

	reloaded := entstore.NewClusterStore(client)
	items, total, err := reloaded.ListNodes(t.Context(), installationID, domaincluster.ListNodesRequest{Page: 1, PageSize: 20, Role: domaincluster.NodeRoleWorker})
	if err != nil || total != 1 || len(items) != 1 || items[0].NodeID != clusterservice.LogicalSingleComponentNodeID(installationID, domaincluster.NodeRoleWorker) || items[0].LastHeartbeatAt == nil || !items[0].LastHeartbeatAt.Equal(now) {
		t.Fatalf("persisted logical-single Worker = %#v total=%d err=%v", items, total, err)
	}
}

func newClusterStoreClient(t *testing.T) *repoent.Client {
	t.Helper()
	client, err := repoent.Open(dialect.SQLite, "file:"+strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatal(err)
	}
	return client
}

func seedInitializedClusterInstallation(t *testing.T, client *repoent.Client, installationID string) {
	t.Helper()
	operationID := uuid.NewString()
	adminID := int64(1)
	revision := 4
	digest := strings.Repeat("d", 64)
	if _, err := client.Installation.Create().
		SetSingletonKey("installation").
		SetInstallationID(installationID).
		SetConfigSchemaVersion(1).
		SetDatabaseSchemaVersion(1).
		SetAppVersion("v1").
		SetSetupOperationID(operationID).
		SetSetupAdminID(adminID).
		SetSetupConfigRevision(revision).
		SetSetupRequestDigest(digest).
		Save(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func postgresURLWithSearchPath(t *testing.T, rawURL, searchPath string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse integration database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", searchPath)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func testTokenProofPublicKey() string {
	return base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("p", 32)))
}
