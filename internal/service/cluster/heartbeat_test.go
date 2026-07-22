package cluster

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	domaincluster "github.com/fatballfish/pic-gallery/internal/domain/cluster"
)

func TestHeartbeatPulseRegistersControlAndReportsRuntimeDrift(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	store := NewMemoryStore(domaincluster.Installation{
		InstallationID: clusterTestInstallationID, Initialized: true,
		ApplicationVersion: "v2", RuntimeSchemaVersion: 1, ConfigRevision: 8,
	})
	runner, err := NewHeartbeatRunner(HeartbeatOptions{
		Store: store, InstallationID: clusterTestInstallationID, NodeID: "control-a", Role: domaincluster.NodeRoleControl,
		ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 7,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	node, err := runner.Pulse(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if node.Health != domaincluster.NodeHealthDegraded || node.LastHeartbeatAt == nil || !node.LastHeartbeatAt.Equal(now) || node.LastError != "application version and config revision drift" {
		t.Fatalf("drift heartbeat = %#v", node)
	}
	stored := store.nodes[node.NodeID]
	if stored.Role != domaincluster.NodeRoleControl || stored.InstallationID != clusterTestInstallationID {
		t.Fatalf("registered control node = %#v", stored)
	}
}

func TestHeartbeatPulsePersistsUnreadyBeforeRejectingSchemaMismatch(t *testing.T) {
	store := NewMemoryStore(domaincluster.Installation{
		InstallationID: clusterTestInstallationID, Initialized: true,
		ApplicationVersion: "v1", RuntimeSchemaVersion: 2, ConfigRevision: 7,
	})
	runner, err := NewHeartbeatRunner(HeartbeatOptions{
		Store: store, InstallationID: clusterTestInstallationID, NodeID: "worker-a", Role: domaincluster.NodeRoleWorker,
		ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	node, err := runner.Pulse(t.Context())
	if !errors.Is(err, ErrRuntimeSchemaMismatch) || node.Health != domaincluster.NodeHealthUnready || node.LastError != "runtime schema version mismatch" {
		t.Fatalf("schema mismatch node=%#v err=%v", node, err)
	}
	if store.nodes["worker-a"].Health != domaincluster.NodeHealthUnready {
		t.Fatalf("schema mismatch was not persisted: %#v", store.nodes["worker-a"])
	}
}

func TestHeartbeatPulseRejectsWrongInstallationWithoutCreatingNode(t *testing.T) {
	store := NewMemoryStore(domaincluster.Installation{InstallationID: "other-installation", Initialized: true})
	runner, err := NewHeartbeatRunner(HeartbeatOptions{
		Store: store, InstallationID: clusterTestInstallationID, NodeID: "api-a", Role: domaincluster.NodeRoleAPI,
		ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Pulse(t.Context()); !errors.Is(err, ErrInstallationNotFound) || len(store.nodes) != 0 {
		t.Fatalf("wrong installation error=%v nodes=%#v", err, store.nodes)
	}
}

func TestHeartbeatStartRunsUntilStopped(t *testing.T) {
	base := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	var ticks atomic.Int64
	store := NewMemoryStore(domaincluster.Installation{
		InstallationID: clusterTestInstallationID, Initialized: true,
		ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 7,
	})
	runner, err := NewHeartbeatRunner(HeartbeatOptions{
		Store: store, InstallationID: clusterTestInstallationID, NodeID: "api-a", Role: domaincluster.NodeRoleAPI,
		ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 7, Interval: 5 * time.Millisecond,
		Now: func() time.Time { return base.Add(time.Duration(ticks.Add(1)) * time.Second) },
	})
	if err != nil {
		t.Fatal(err)
	}
	handle, err := runner.Start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for ticks.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if ticks.Load() < 2 {
		t.Fatal("heartbeat loop did not run after the initial pulse")
	}
	handle.Stop()
}
