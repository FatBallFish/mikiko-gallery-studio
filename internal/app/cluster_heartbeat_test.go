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

func TestStartRuntimeHeartbeatSkipsSingleAndStartsClusterNode(t *testing.T) {
	store := clusterservice.NewMemoryStore(domaincluster.Installation{
		InstallationID: "installation-a", Initialized: true,
		ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 3,
	})
	cfg := config.Config{Runtime: config.RuntimeConfig{
		DeploymentRole: config.DeploymentRoleSingle, InstallationID: "installation-a",
		ApplicationVersion: "v1", ConfigSchemaVersion: 1, ConfigRevision: 3,
	}}
	handle, err := startRuntimeHeartbeat(t.Context(), cfg, store)
	if err != nil || handle != nil {
		t.Fatalf("single heartbeat handle=%#v err=%v", handle, err)
	}
	cfg.Runtime.ClusterNodeID = "single-process-id"
	handle, err = startRuntimeHeartbeat(t.Context(), cfg, store)
	if err != nil || handle != nil {
		t.Fatalf("single heartbeat with process ID handle=%#v err=%v", handle, err)
	}

	cfg.Runtime.DeploymentRole = config.DeploymentRoleAPI
	cfg.Runtime.ClusterNodeID = "api-a"
	handle, err = startRuntimeHeartbeat(t.Context(), cfg, store)
	if err != nil || handle == nil {
		t.Fatalf("cluster heartbeat handle=%#v err=%v", handle, err)
	}
	handle.Stop()
	nodes, total, err := store.ListNodes(t.Context(), "installation-a", domaincluster.ListNodesRequest{Page: 1, PageSize: 20})
	if err != nil || total != 1 || nodes[0].Health != domaincluster.NodeHealthHealthy {
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
	if _, err := startRuntimeHeartbeat(t.Context(), cfg, store); !errors.Is(err, clusterservice.ErrRuntimeSchemaMismatch) {
		t.Fatalf("schema mismatch error = %v", err)
	}
	nodes, _, _ := store.ListNodes(t.Context(), "installation-a", domaincluster.ListNodesRequest{Page: 1, PageSize: 20})
	if len(nodes) != 1 || nodes[0].Health != domaincluster.NodeHealthUnready {
		t.Fatalf("schema mismatch nodes = %#v", nodes)
	}
}
