package app

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	domaincluster "github.com/fatballfish/pic-gallery/internal/domain/cluster"
	clusterservice "github.com/fatballfish/pic-gallery/internal/service/cluster"
)

func TestStartRuntimeHeartbeatStartsStableSingleComponentsAndClusterNode(t *testing.T) {
	store := clusterservice.NewMemoryStore(domaincluster.Installation{
		InstallationID: "installation-a", Initialized: true,
		ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 3,
	})
	cfg := config.Config{Runtime: config.RuntimeConfig{
		DeploymentRole: config.DeploymentRoleSingle, InstallationID: "installation-a",
		ApplicationVersion: "v1", ConfigSchemaVersion: 1, ConfigRevision: 3,
	}}
	apiHandle, err := startRuntimeHeartbeat(t.Context(), cfg, store, domaincluster.NodeRoleAPI)
	if err != nil || apiHandle == nil {
		t.Fatalf("single API heartbeat handle=%#v err=%v", apiHandle, err)
	}
	apiHandle.Stop()
	workerHandle, err := startRuntimeHeartbeat(t.Context(), cfg, store, domaincluster.NodeRoleWorker)
	if err != nil || workerHandle == nil {
		t.Fatalf("single Worker heartbeat handle=%#v err=%v", workerHandle, err)
	}
	workerHandle.Stop()
	nodes, total, err := store.ListNodes(t.Context(), "installation-a", domaincluster.ListNodesRequest{Page: 1, PageSize: 20})
	if err != nil || total != 2 || nodes[0].NodeID == nodes[1].NodeID {
		t.Fatalf("single component nodes=%#v total=%d err=%v", nodes, total, err)
	}
	apiID := clusterservice.LogicalSingleComponentNodeID("installation-a", domaincluster.NodeRoleAPI)
	apiHandle, err = startRuntimeHeartbeat(t.Context(), cfg, store, domaincluster.NodeRoleAPI)
	if err != nil || apiHandle == nil {
		t.Fatalf("restart single API heartbeat handle=%#v err=%v", apiHandle, err)
	}
	apiHandle.Stop()
	nodes, total, err = store.ListNodes(t.Context(), "installation-a", domaincluster.ListNodesRequest{Page: 1, PageSize: 20})
	if err != nil || total != 2 {
		t.Fatalf("single component restart nodes=%#v total=%d err=%v", nodes, total, err)
	}
	foundAPI := false
	for _, node := range nodes {
		foundAPI = foundAPI || node.NodeID == apiID
	}
	if !foundAPI {
		t.Fatalf("stable single API node %q missing from %#v", apiID, nodes)
	}

	cfg.Runtime.DeploymentRole = config.DeploymentRoleAPI
	cfg.Runtime.ClusterNodeID = "api-a"
	handle, err := startRuntimeHeartbeat(t.Context(), cfg, store, domaincluster.NodeRoleAPI)
	if err != nil || handle == nil {
		t.Fatalf("cluster heartbeat handle=%#v err=%v", handle, err)
	}
	handle.Stop()
	nodes, total, err = store.ListNodes(t.Context(), "installation-a", domaincluster.ListNodesRequest{Page: 1, PageSize: 20})
	if err != nil || total != 3 || nodes[0].Health != domaincluster.NodeHealthHealthy {
		t.Fatalf("cluster heartbeat nodes=%#v total=%d err=%v", nodes, total, err)
	}
}

func TestAPIAndWorkerStartHeartbeatBeforeServingOrClaimingTasks(t *testing.T) {
	for _, testCase := range []struct {
		path   string
		before string
	}{
		{path: "run.go", before: "srv.ListenAndServe()"},
		{path: "worker.go", before: "runner.Run(ctx)"},
	} {
		content, err := os.ReadFile(testCase.path)
		if err != nil {
			t.Fatal(err)
		}
		source := string(content)
		heartbeatAt := strings.Index(source, "startRuntimeHeartbeat(")
		workAt := strings.Index(source, testCase.before)
		if heartbeatAt < 0 || workAt < 0 || heartbeatAt > workAt {
			t.Fatalf("%s must start cluster heartbeat before %s", testCase.path, testCase.before)
		}
	}
}

func TestStartRuntimeHeartbeatRejectsIncompatibleSchemaAfterRecordingUnready(t *testing.T) {
	store := clusterservice.NewMemoryStore(domaincluster.Installation{
		InstallationID: "installation-a", Initialized: true,
		ApplicationVersion: "v1", RuntimeSchemaVersion: 2, ConfigRevision: 3,
	})
	cfg := config.Config{Runtime: config.RuntimeConfig{
		DeploymentRole: config.DeploymentRoleWorker, InstallationID: "installation-a", ClusterNodeID: "worker-a",
		ApplicationVersion: "v1", ConfigSchemaVersion: 1, ConfigRevision: 3,
	}}
	if _, err := startRuntimeHeartbeat(t.Context(), cfg, store, domaincluster.NodeRoleWorker); !errors.Is(err, clusterservice.ErrRuntimeSchemaMismatch) {
		t.Fatalf("schema mismatch error = %v", err)
	}
	nodes, _, _ := store.ListNodes(t.Context(), "installation-a", domaincluster.ListNodesRequest{Page: 1, PageSize: 20})
	if len(nodes) != 1 || nodes[0].Health != domaincluster.NodeHealthUnready {
		t.Fatalf("schema mismatch nodes = %#v", nodes)
	}
}
