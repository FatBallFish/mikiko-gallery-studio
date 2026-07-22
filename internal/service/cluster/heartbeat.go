package cluster

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	domaincluster "github.com/fatballfish/pic-gallery/internal/domain/cluster"
)

var ErrRuntimeSchemaMismatch = errors.New("cluster runtime schema version mismatch")

type HeartbeatStore interface {
	GetInstallation(context.Context, string) (domaincluster.Installation, error)
	CreateNode(context.Context, domaincluster.Node) (domaincluster.Node, error)
	HeartbeatNode(context.Context, string, domaincluster.HeartbeatRequest, time.Time) (domaincluster.Node, error)
}

type HeartbeatOptions struct {
	Store                HeartbeatStore
	InstallationID       string
	NodeID               string
	Role                 domaincluster.NodeRole
	ApplicationVersion   string
	RuntimeSchemaVersion int
	ConfigRevision       int64
	Interval             time.Duration
	Now                  func() time.Time
}

type HeartbeatRunner struct {
	store                HeartbeatStore
	installationID       string
	nodeID               string
	role                 domaincluster.NodeRole
	applicationVersion   string
	runtimeSchemaVersion int
	configRevision       int64
	interval             time.Duration
	now                  func() time.Time
}

type HeartbeatHandle struct {
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

func NewHeartbeatRunner(options HeartbeatOptions) (*HeartbeatRunner, error) {
	if options.Store == nil {
		return nil, errors.New("cluster heartbeat store is required")
	}
	options.InstallationID = strings.TrimSpace(options.InstallationID)
	options.NodeID = strings.TrimSpace(options.NodeID)
	options.ApplicationVersion = strings.TrimSpace(options.ApplicationVersion)
	if options.InstallationID == "" || options.NodeID == "" || !validHeartbeatRole(options.Role) || options.ApplicationVersion == "" || options.RuntimeSchemaVersion <= 0 || options.ConfigRevision < 0 {
		return nil, errors.New("cluster heartbeat identity is invalid")
	}
	if options.Interval <= 0 {
		options.Interval = 10 * time.Second
	}
	if options.Now == nil {
		options.Now = func() time.Time { return time.Now().UTC() }
	}
	return &HeartbeatRunner{
		store: options.Store, installationID: options.InstallationID, nodeID: options.NodeID, role: options.Role,
		applicationVersion: options.ApplicationVersion, runtimeSchemaVersion: options.RuntimeSchemaVersion,
		configRevision: options.ConfigRevision, interval: options.Interval, now: options.Now,
	}, nil
}

func (r *HeartbeatRunner) Pulse(ctx context.Context) (domaincluster.Node, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	installation, err := r.store.GetInstallation(ctx, r.installationID)
	if err != nil {
		return domaincluster.Node{}, err
	}
	if !installation.Initialized || installation.InstallationID != r.installationID {
		return domaincluster.Node{}, ErrInstallationNotFound
	}
	health, lastError, compatibilityErr := r.healthFor(installation)
	now := r.now().UTC()
	request := domaincluster.HeartbeatRequest{
		NodeID: r.nodeID, Role: r.role, Health: health, LastError: lastError,
		ApplicationVersion: r.applicationVersion, RuntimeSchemaVersion: r.runtimeSchemaVersion, ConfigRevision: r.configRevision,
	}
	node, err := r.store.HeartbeatNode(ctx, r.installationID, request, now)
	if errors.Is(err, ErrNodeNotFound) {
		_, err = r.store.CreateNode(ctx, domaincluster.Node{
			NodeID: r.nodeID, InstallationID: r.installationID, Role: r.role,
			ApplicationVersion: r.applicationVersion, RuntimeSchemaVersion: r.runtimeSchemaVersion,
			ConfigRevision: r.configRevision, Health: health, LastError: lastError, CreatedAt: now, UpdatedAt: now,
		})
		if err == nil {
			node, err = r.store.HeartbeatNode(ctx, r.installationID, request, now)
		}
	}
	if err != nil {
		return domaincluster.Node{}, fmt.Errorf("persist cluster heartbeat: %w", err)
	}
	return node, compatibilityErr
}

func (r *HeartbeatRunner) Start(parent context.Context) (*HeartbeatHandle, error) {
	if parent == nil {
		parent = context.Background()
	}
	if _, err := r.Pulse(parent); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	handle := &HeartbeatHandle{cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(handle.done)
		defer func() {
			if recover() != nil {
				slog.Error("cluster heartbeat loop stopped after panic", "node_id", r.nodeID)
			}
		}()
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := r.Pulse(ctx); err != nil && !errors.Is(err, context.Canceled) {
					slog.Warn("cluster heartbeat update failed", "node_id", r.nodeID)
				}
			}
		}
	}()
	return handle, nil
}

func (h *HeartbeatHandle) Stop() {
	if h == nil {
		return
	}
	h.once.Do(func() {
		h.cancel()
		<-h.done
	})
}

func (r *HeartbeatRunner) healthFor(installation domaincluster.Installation) (domaincluster.NodeHealth, string, error) {
	if r.runtimeSchemaVersion != installation.RuntimeSchemaVersion {
		return domaincluster.NodeHealthUnready, "runtime schema version mismatch", ErrRuntimeSchemaMismatch
	}
	drifts := make([]string, 0, 2)
	if r.applicationVersion != installation.ApplicationVersion {
		drifts = append(drifts, "application version")
	}
	if r.configRevision != installation.ConfigRevision {
		drifts = append(drifts, "config revision")
	}
	if len(drifts) > 0 {
		return domaincluster.NodeHealthDegraded, strings.Join(drifts, " and ") + " drift", nil
	}
	return domaincluster.NodeHealthHealthy, "", nil
}

func validHeartbeatRole(role domaincluster.NodeRole) bool {
	switch role {
	case domaincluster.NodeRoleSingle, domaincluster.NodeRoleControl, domaincluster.NodeRoleAPI, domaincluster.NodeRoleWorker:
		return true
	default:
		return false
	}
}
