package deployctl

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domaincluster "github.com/fatballfish/pic-gallery/internal/domain/cluster"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	clusterservice "github.com/fatballfish/pic-gallery/internal/service/cluster"
)

type ClusterTokenCreateResult struct {
	Role       config.DeploymentRole
	Credential string
	ExpiresAt  time.Time
}

func CreateClusterToken(ctx context.Context, runtimeDir string, options ClusterTokenCreateOptions) (ClusterTokenCreateResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	runtimeEnvPath := filepath.Join(filepath.Clean(defaultString(runtimeDir, ".")), "config", "runtime.env")
	cfg, err := config.LoadRuntime(runtimeEnvPath)
	if err != nil {
		return ClusterTokenCreateResult{}, fmt.Errorf("load runtime configuration for cluster token: %w", err)
	}
	if cfg.Runtime.DeploymentRole != config.DeploymentRoleControl {
		return ClusterTokenCreateResult{}, fmt.Errorf("cluster join tokens can be created only on the control node")
	}
	client, err := db.OpenContext(ctx, cfg.Database.URL)
	if err != nil {
		return ClusterTokenCreateResult{}, fmt.Errorf("open cluster database: %w", redactRuntimeError(err, map[string]string{"DATABASE_URL": cfg.Database.URL}))
	}
	defer client.Close()
	service := clusterservice.NewService(clusterservice.ServiceOptions{
		Store: entstore.NewClusterStore(client), InstallationID: cfg.Runtime.InstallationID,
		DeploymentRole: domaincluster.NodeRoleControl,
	})
	issued, err := service.CreateToken(ctx, domaincluster.CreateTokenRequest{
		Role: domaincluster.JoinRole(options.Role), TTL: options.TTL, ActorID: "deployctl-local",
	})
	if err != nil {
		return ClusterTokenCreateResult{}, fmt.Errorf("create cluster join token: %w", err)
	}
	return ClusterTokenCreateResult{Role: options.Role, Credential: issued.Credential, ExpiresAt: issued.Token.ExpiresAt}, nil
}
