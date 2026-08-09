package cluster

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	domaincluster "github.com/fatballfish/pic-gallery/internal/domain/cluster"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

const clusterTestInstallationID = "019d0000-0000-7000-8000-000000000777"

func TestServiceIssuesHashOnlyRoleScopedSingleUseTokens(t *testing.T) {
	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)
	store := NewMemoryStore(domaincluster.Installation{
		InstallationID: clusterTestInstallationID, Initialized: true,
		ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 7,
	})
	service := NewService(ServiceOptions{
		Store: store, InstallationID: clusterTestInstallationID,
		DeploymentRole: domaincluster.NodeRoleControl, Now: func() time.Time { return now },
		Entropy: bytes.NewReader(bytes.Repeat([]byte{0x42}, 64)),
	})

	issued, err := service.CreateToken(t.Context(), domaincluster.CreateTokenRequest{
		Role: domaincluster.JoinRoleWorker, TTL: 10 * time.Minute, ActorID: "17",
	})
	if err != nil {
		t.Fatal(err)
	}
	if issued.Credential == "" || !strings.Contains(issued.Credential, issued.Token.TokenID) || issued.Token.Role != domaincluster.JoinRoleWorker || !issued.Token.ExpiresAt.Equal(now.Add(10*time.Minute)) {
		t.Fatalf("issued token = %#v", issued)
	}
	stored := store.tokens[issued.Token.TokenID]
	proofKey, proofErr := TokenProofKeyFromCredential(issued.Credential)
	if proofErr != nil {
		t.Fatal(proofErr)
	}
	if stored.TokenHash == "" || stored.TokenProofPublicKey != proofKey.PublicKey() || strings.Contains(issued.Credential, stored.TokenHash) || stored.TokenHash == issued.Credential {
		t.Fatalf("stored credential was not a one-way hash: %#v", stored)
	}
	page, err := service.ListTokens(t.Context(), domaincluster.ListTokensRequest{Page: 1, PageSize: 20})
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].TokenID != issued.Token.TokenID {
		t.Fatalf("list tokens = %#v, %v", page, err)
	}
	if strings.Contains(string(mustJSON(t, page)), issued.Credential) || strings.Contains(string(mustJSON(t, page)), stored.TokenHash) || strings.Contains(string(mustJSON(t, page)), stored.TokenProofPublicKey) {
		t.Fatal("token list exposed credential material")
	}
	enrollment, err := service.EnrollNode(t.Context(), issued.Credential, domaincluster.RegisterNodeRequest{
		NodeID: "node-worker-1", Role: domaincluster.NodeRoleWorker, ApplicationVersion: "v1",
		RuntimeSchemaVersion: 1, ConfigRevision: 7,
	})
	if err != nil || enrollment.Token.ConsumedAt == nil || enrollment.Token.ConsumedByNodeID != "node-worker-1" || enrollment.Node.NodeID != "node-worker-1" {
		t.Fatalf("enroll node = %#v, %v", enrollment, err)
	}
	if _, err := service.EnrollNode(t.Context(), issued.Credential, domaincluster.RegisterNodeRequest{
		NodeID: "node-worker-2", Role: domaincluster.NodeRoleWorker, ApplicationVersion: "v1",
		RuntimeSchemaVersion: 1, ConfigRevision: 7,
	}); appErrorStatus(err) != 409 {
		t.Fatalf("replayed token error = %v", err)
	}
	if len(store.auditRecords) != 3 || store.auditRecords[0].Action != "cluster.token.create" || store.auditRecords[1].Action != "cluster.token.consume" || store.auditRecords[2].Action != "cluster.node.register" {
		t.Fatalf("transactional enrollment audit records = %#v", store.auditRecords)
	}
	for _, record := range store.auditRecords {
		encoded := string(mustJSON(t, record))
		if strings.Contains(encoded, issued.Credential) || strings.Contains(encoded, stored.TokenHash) {
			t.Fatalf("audit exposed credential material: %s", encoded)
		}
	}
}

func TestServiceRejectsWrongRoleExpiredTokensAndNonControlIssuers(t *testing.T) {
	now := time.Date(2026, 7, 23, 3, 30, 0, 0, time.UTC)
	newService := func(role domaincluster.NodeRole, initialized bool) *Service {
		return NewService(ServiceOptions{
			Store:          NewMemoryStore(domaincluster.Installation{InstallationID: clusterTestInstallationID, Initialized: initialized}),
			InstallationID: clusterTestInstallationID, DeploymentRole: role,
			Now: func() time.Time { return now }, Entropy: bytes.NewReader(bytes.Repeat([]byte{0x31}, 64)),
		})
	}
	for _, testCase := range []struct {
		name        string
		role        domaincluster.NodeRole
		initialized bool
	}{
		{name: "api replica", role: domaincluster.NodeRoleAPI, initialized: true},
		{name: "uninitialized control", role: domaincluster.NodeRoleControl, initialized: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := newService(testCase.role, testCase.initialized).CreateToken(t.Context(), domaincluster.CreateTokenRequest{Role: domaincluster.JoinRoleAPI, TTL: time.Minute, ActorID: "1"})
			if appErrorStatus(err) != 403 {
				t.Fatalf("create token error = %v", err)
			}
		})
	}

	service := newService(domaincluster.NodeRoleSingle, true)
	issued, err := service.CreateToken(t.Context(), domaincluster.CreateTokenRequest{Role: domaincluster.JoinRoleAPI, TTL: time.Minute, ActorID: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.EnrollNode(t.Context(), issued.Credential, domaincluster.RegisterNodeRequest{
		NodeID: "node-1", Role: domaincluster.NodeRoleWorker, ApplicationVersion: "v1", RuntimeSchemaVersion: 1,
	}); appErrorStatus(err) != 401 {
		t.Fatalf("wrong-role token error = %v", err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := service.EnrollNode(t.Context(), issued.Credential, domaincluster.RegisterNodeRequest{
		NodeID: "node-1", Role: domaincluster.NodeRoleAPI, ApplicationVersion: "v1", RuntimeSchemaVersion: 1,
	}); appErrorStatus(err) != 401 {
		t.Fatalf("expired token error = %v", err)
	}
}

func TestServiceRevokesTokensAndRecordsTheAdminActor(t *testing.T) {
	store := NewMemoryStore(domaincluster.Installation{InstallationID: clusterTestInstallationID, Initialized: true})
	service := NewService(ServiceOptions{
		Store: store, InstallationID: clusterTestInstallationID, DeploymentRole: domaincluster.NodeRoleControl,
		Now: time.Now, Entropy: bytes.NewReader(bytes.Repeat([]byte{0x52}, 64)),
	})
	issued, err := service.CreateToken(t.Context(), domaincluster.CreateTokenRequest{Role: domaincluster.JoinRoleWeb, TTL: time.Hour, ActorID: "42"})
	if err != nil {
		t.Fatal(err)
	}
	revoked, err := service.RevokeToken(t.Context(), issued.Token.TokenID, "84")
	if err != nil || revoked.RevokedAt == nil || issued.Token.CreatedBy != "42" {
		t.Fatalf("revoked token = %#v, %v", revoked, err)
	}
	if _, err := service.EnrollNode(t.Context(), issued.Credential, domaincluster.RegisterNodeRequest{
		NodeID: "web-1", Role: domaincluster.NodeRoleWeb, ApplicationVersion: "v1", RuntimeSchemaVersion: 1,
	}); appErrorStatus(err) != 409 {
		t.Fatalf("revoked token consume error = %v", err)
	}
	if len(store.auditRecords) != 2 || store.auditRecords[1].Action != "cluster.token.revoke" || store.auditRecords[1].ActorID != "84" {
		t.Fatalf("revoke audit = %#v", store.auditRecords)
	}
}

func TestServiceRegistersUniqueNodesAndUpdatesHeartbeatMetadata(t *testing.T) {
	now := time.Date(2026, 7, 23, 4, 0, 0, 0, time.UTC)
	store := NewMemoryStore(domaincluster.Installation{
		InstallationID: clusterTestInstallationID, Initialized: true,
		ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 9,
	})
	service := NewService(ServiceOptions{
		Store: store, InstallationID: clusterTestInstallationID,
		DeploymentRole: domaincluster.NodeRoleControl, Now: func() time.Time { return now },
	})
	issue := func(role domaincluster.JoinRole) string {
		issued, issueErr := service.CreateToken(t.Context(), domaincluster.CreateTokenRequest{Role: role, TTL: time.Hour, ActorID: "1"})
		if issueErr != nil {
			t.Fatal(issueErr)
		}
		return issued.Credential
	}
	enrollment, err := service.EnrollNode(t.Context(), issue(domaincluster.JoinRoleWorker), domaincluster.RegisterNodeRequest{
		NodeID: "worker-a", Role: domaincluster.NodeRoleWorker, ApplicationVersion: "v1",
		RuntimeSchemaVersion: 1, ConfigRevision: 9,
	})
	node := enrollment.Node
	if err != nil || node.Health != domaincluster.NodeHealthJoining || node.InstallationID != clusterTestInstallationID {
		t.Fatalf("register node = %#v, %v", node, err)
	}
	if _, err := service.EnrollNode(t.Context(), issue(domaincluster.JoinRoleAPI), domaincluster.RegisterNodeRequest{
		NodeID: "worker-a", Role: domaincluster.NodeRoleAPI, ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 9,
	}); appErrorStatus(err) != 409 {
		t.Fatalf("duplicate node identity error = %v", err)
	}
	if _, err := service.EnrollNode(t.Context(), issue(domaincluster.JoinRoleWorker), domaincluster.RegisterNodeRequest{
		NodeID: "worker-a", Role: domaincluster.NodeRoleWorker, ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 10,
	}); appErrorStatus(err) != 409 {
		t.Fatalf("config revision mismatch error = %v", err)
	}
	now = now.Add(15 * time.Second)
	updated, err := service.HeartbeatNode(t.Context(), domaincluster.HeartbeatRequest{
		NodeID: "worker-a", Role: domaincluster.NodeRoleWorker, Health: domaincluster.NodeHealthHealthy,
		ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 10,
	})
	if err != nil || updated.LastHeartbeatAt == nil || !updated.LastHeartbeatAt.Equal(now) || updated.ConfigRevision != 10 {
		t.Fatalf("heartbeat node = %#v, %v", updated, err)
	}
	if len(store.auditRecords) != 5 || store.auditRecords[2].Action != "cluster.node.register" {
		t.Fatalf("node audit records = %#v", store.auditRecords)
	}
}

func TestListNodesComputesOfflineAndVersionConfigDrift(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	store := NewMemoryStore(domaincluster.Installation{
		InstallationID: clusterTestInstallationID, Initialized: true,
		ApplicationVersion: "v2", RuntimeSchemaVersion: 2, ConfigRevision: 9,
	})
	oldHeartbeat := now.Add(-time.Minute)
	store.nodes["worker-old"] = domaincluster.Node{
		NodeID: "worker-old", InstallationID: clusterTestInstallationID, Role: domaincluster.NodeRoleWorker,
		ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 8, Health: domaincluster.NodeHealthHealthy,
		LastHeartbeatAt: &oldHeartbeat, CreatedAt: oldHeartbeat, UpdatedAt: oldHeartbeat,
	}
	service := NewService(ServiceOptions{
		Store: store, InstallationID: clusterTestInstallationID, DeploymentRole: domaincluster.NodeRoleControl,
		Now: func() time.Time { return now },
	})
	page, err := service.ListNodes(t.Context(), domaincluster.ListNodesRequest{Page: 1, PageSize: 20})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("list nodes = %#v, %v", page, err)
	}
	item := page.Items[0]
	if item.EffectiveHealth != domaincluster.NodeHealthOffline || !item.ApplicationVersionDrift || !item.RuntimeSchemaDrift || !item.ConfigRevisionDrift {
		t.Fatalf("node status = %#v", item)
	}
	if item.Source != domaincluster.NodeSourceHeartbeat {
		t.Fatalf("distributed node source = %q", item.Source)
	}
}

func TestListNodesSingleTopologyReturnsOneStableLogicalNode(t *testing.T) {
	now := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	store := NewMemoryStore(domaincluster.Installation{
		InstallationID: clusterTestInstallationID, Initialized: true,
		ApplicationVersion: "v2", RuntimeSchemaVersion: 2, ConfigRevision: 9,
	})
	// Stale process rows from an older runtime must not leak into logical-single topology.
	store.nodes["api-process-123"] = domaincluster.Node{
		NodeID: "api-process-123", InstallationID: clusterTestInstallationID, Role: domaincluster.NodeRoleAPI,
		ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 8, Health: domaincluster.NodeHealthOffline,
		CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
	}
	service := NewService(ServiceOptions{
		Store: store, InstallationID: clusterTestInstallationID, DeploymentRole: domaincluster.NodeRoleSingle,
		Now: func() time.Time { return now },
	})

	first, err := service.ListNodes(t.Context(), domaincluster.ListNodesRequest{Page: 1, PageSize: 20})
	if err != nil || first.Total != 1 || len(first.Items) != 1 {
		t.Fatalf("first logical-single page = %#v, %v", first, err)
	}
	second, err := service.ListNodes(t.Context(), domaincluster.ListNodesRequest{Page: 1, PageSize: 20})
	if err != nil || second.Total != 1 || len(second.Items) != 1 {
		t.Fatalf("second logical-single page = %#v, %v", second, err)
	}
	node := first.Items[0]
	if node.NodeID == "" || node.NodeID != second.Items[0].NodeID || node.Role != domaincluster.NodeRoleSingle || node.Source != domaincluster.NodeSourceLogicalSingle {
		t.Fatalf("logical-single identity = first %#v second %#v", node, second.Items[0])
	}
	if node.EffectiveHealth != domaincluster.NodeHealthHealthy || node.LastHeartbeatAt != nil {
		t.Fatalf("logical-single health = %#v", node)
	}
}

func appErrorStatus(err error) int {
	var appError *errs.Error
	if errors.As(err, &appError) {
		return appError.StatusCode
	}
	return 0
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
