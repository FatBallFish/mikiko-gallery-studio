package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

const defaultWorkerBootstrapPollInterval = 500 * time.Millisecond

var ErrWorkerBootstrapInvalid = errors.New("worker bootstrap state is invalid")

type workerBootstrap struct {
	Bootstrap config.BootstrapConfig
	State     setup.InstallState
}

type workerBootstrapDependencies struct {
	loadBootstrap    func(string) (config.BootstrapConfig, error)
	loadInstallState func(string) (setup.InstallState, bool, error)
	wait             func(context.Context, time.Duration) error
	pollInterval     time.Duration
}

type workerRunDependencies struct {
	runtimeEnvPath func() string
	bootstrap      workerBootstrapDependencies
	runNormal      func(context.Context, workerBootstrap) error
}

type workerBootstrapSnapshot struct {
	diagnosticCode        string
	bootstrapSchema       int
	bootstrapCompleted    bool
	bootstrapInstallation string
	bootstrapRole         config.DeploymentRole
	bootstrapRevision     int
	bootstrapAppVersion   string
	stateSchema           int
	stateInstallation     string
	stateRole             config.DeploymentRole
	statePhase            setup.InstallPhase
	stateCompleted        bool
	stateUpdatedAt        int64
	commitPresent         bool
	commitOperation       string
	commitInstallation    string
	commitRuntimeSchema   int
	commitRevision        int
}

func runWorker(ctx context.Context, dependencies workerRunDependencies) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if dependencies.runtimeEnvPath == nil {
		dependencies.runtimeEnvPath = configuredRuntimeEnvPath
	}
	if dependencies.runNormal == nil {
		dependencies.runNormal = runNormalWorker
	}

	startup, err := waitForWorkerBootstrap(ctx, dependencies.runtimeEnvPath(), dependencies.bootstrap)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
	if err := dependencies.runNormal(ctx, startup); err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
	return nil
}

func waitForWorkerBootstrap(ctx context.Context, runtimeEnvPath string, dependencies workerBootstrapDependencies) (workerBootstrap, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if dependencies.loadBootstrap == nil {
		dependencies.loadBootstrap = config.LoadBootstrap
	}
	if dependencies.loadInstallState == nil {
		dependencies.loadInstallState = func(path string) (setup.InstallState, bool, error) {
			return setup.NewStateStore(path).Load()
		}
	}
	if dependencies.wait == nil {
		dependencies.wait = waitForWorkerBootstrapPoll
	}
	if dependencies.pollInterval <= 0 {
		dependencies.pollInterval = defaultWorkerBootstrapPollInterval
	}

	var previousInconsistent *workerBootstrapSnapshot
	for {
		if err := ctx.Err(); err != nil {
			return workerBootstrap{}, err
		}
		startup := loadAPIStartup(runtimeEnvPath, apiStartupDependencies{
			loadBootstrap: dependencies.loadBootstrap, loadInstallState: dependencies.loadInstallState,
		})
		switch {
		case startup.Mode == setup.StartupModeNormal:
			if !workerRuntimeRole(startup.Bootstrap.Deployment.Role) {
				return workerBootstrap{}, fmt.Errorf("%w: deployment role cannot run worker", ErrWorkerBootstrapInvalid)
			}
			return workerBootstrap{Bootstrap: startup.Bootstrap, State: startup.State}, nil
		case startup.Mode == setup.StartupModeSetup:
			// A co-located Worker waits while the control API completes browser setup.
			previousInconsistent = nil
		case startup.Decision.Reconciliation == setup.ReconciliationRequireDatabase:
			// Only the API/control path may reconcile the setup commit. The Worker
			// keeps all runtime dependencies closed until install-state is complete.
			previousInconsistent = nil
		case startup.DiagnosticCode == "STARTUP_STATE_INCONSISTENT":
			// runtime.env and install-state.json are individually atomic but cannot
			// be replaced in one cross-file transaction. Confirm the same non-secret
			// metadata snapshot twice before treating an inconsistent pair as stable.
			snapshot := workerStartupSnapshot(startup)
			if previousInconsistent != nil && *previousInconsistent == snapshot {
				return workerBootstrap{}, fmt.Errorf("%w: %s", ErrWorkerBootstrapInvalid, startup.DiagnosticCode)
			}
			previousInconsistent = &snapshot
		default:
			return workerBootstrap{}, fmt.Errorf("%w: %s", ErrWorkerBootstrapInvalid, startup.DiagnosticCode)
		}
		if err := dependencies.wait(ctx, dependencies.pollInterval); err != nil {
			return workerBootstrap{}, err
		}
	}
}

func workerStartupSnapshot(startup apiStartup) workerBootstrapSnapshot {
	snapshot := workerBootstrapSnapshot{
		diagnosticCode:  startup.DiagnosticCode,
		bootstrapSchema: startup.Bootstrap.SchemaVersion, bootstrapCompleted: startup.Bootstrap.SetupCompleted,
		bootstrapInstallation: startup.Bootstrap.InstallationID, bootstrapRole: startup.Bootstrap.Deployment.Role,
		bootstrapRevision: startup.Bootstrap.ConfigRevision, bootstrapAppVersion: startup.Bootstrap.ApplicationVersion,
		stateSchema: startup.State.SchemaVersion, stateInstallation: startup.State.InstallationID,
		stateRole: startup.State.DeploymentRole, statePhase: startup.State.Phase,
		stateCompleted: startup.State.EverCompleted, stateUpdatedAt: startup.State.UpdatedAt.UnixNano(),
		commitPresent: startup.State.Commit != nil,
	}
	if startup.State.Commit != nil {
		snapshot.commitOperation = startup.State.Commit.OperationID
		snapshot.commitInstallation = startup.State.Commit.InstallationID
		snapshot.commitRuntimeSchema = startup.State.Commit.RuntimeSchemaVersion
		snapshot.commitRevision = startup.State.Commit.ConfigRevision
	}
	return snapshot
}

func waitForWorkerBootstrapPoll(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func workerRuntimeRole(role config.DeploymentRole) bool {
	return role == config.DeploymentRoleSingle || role == config.DeploymentRoleControl || role == config.DeploymentRoleWorker
}
