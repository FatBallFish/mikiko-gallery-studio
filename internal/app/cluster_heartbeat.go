package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/config"
	domaincluster "github.com/fatballfish/pic-gallery/internal/domain/cluster"
	clusterservice "github.com/fatballfish/pic-gallery/internal/service/cluster"
)

func runtimeClusterNodeRoles(runtime config.RuntimeConfig) []domaincluster.NodeRole {
	roles := config.RuntimeHeartbeatRoles(runtime)
	result := make([]domaincluster.NodeRole, 0, len(roles))
	for _, role := range roles {
		result = append(result, domaincluster.NodeRole(role))
	}
	return result
}

func startRuntimeHeartbeat(ctx context.Context, cfg config.Config, store clusterservice.HeartbeatStore, componentRole domaincluster.NodeRole) (*clusterservice.HeartbeatHandle, error) {
	role := domaincluster.NodeRole(cfg.Runtime.DeploymentRole)
	nodeID := strings.TrimSpace(cfg.Runtime.ClusterNodeID)
	if cfg.Runtime.DeploymentRole == config.DeploymentRoleSingle {
		if componentRole != domaincluster.NodeRoleAPI && componentRole != domaincluster.NodeRoleWorker {
			return nil, fmt.Errorf("single deployment component role %q cannot publish a runtime heartbeat", componentRole)
		}
		role = componentRole
		nodeID = clusterservice.LogicalSingleComponentNodeID(cfg.Runtime.InstallationID, componentRole)
	} else if nodeID == "" {
		return nil, fmt.Errorf("cluster node ID is required for role %q", cfg.Runtime.DeploymentRole)
	}
	runner, err := clusterservice.NewHeartbeatRunner(clusterservice.HeartbeatOptions{
		Store: store, InstallationID: cfg.Runtime.InstallationID, NodeID: nodeID,
		Role: role, ApplicationVersion: cfg.Runtime.ApplicationVersion,
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
