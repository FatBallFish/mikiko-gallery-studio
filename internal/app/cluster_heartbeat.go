package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/config"
	domaincluster "github.com/fatballfish/pic-gallery/internal/domain/cluster"
	clusterservice "github.com/fatballfish/pic-gallery/internal/service/cluster"
)

func startRuntimeHeartbeat(ctx context.Context, cfg config.Config, store clusterservice.HeartbeatStore) (*clusterservice.HeartbeatHandle, error) {
	nodeID := strings.TrimSpace(cfg.Runtime.ClusterNodeID)
	if nodeID == "" && cfg.Runtime.DeploymentRole == config.DeploymentRoleSingle {
		return nil, nil
	}
	if nodeID == "" {
		return nil, fmt.Errorf("cluster node ID is required for role %q", cfg.Runtime.DeploymentRole)
	}
	runner, err := clusterservice.NewHeartbeatRunner(clusterservice.HeartbeatOptions{
		Store: store, InstallationID: cfg.Runtime.InstallationID, NodeID: nodeID,
		Role: domaincluster.NodeRole(cfg.Runtime.DeploymentRole), ApplicationVersion: cfg.Runtime.ApplicationVersion,
		RuntimeSchemaVersion: cfg.Runtime.ConfigSchemaVersion, ConfigRevision: int64(cfg.Runtime.ConfigRevision),
	})
	if err != nil {
		return nil, fmt.Errorf("configure cluster heartbeat: %w", err)
	}
	handle, err := runner.Start(ctx)
	if err != nil {
		return nil, fmt.Errorf("start cluster heartbeat: %w", err)
	}
	return handle, nil
}
