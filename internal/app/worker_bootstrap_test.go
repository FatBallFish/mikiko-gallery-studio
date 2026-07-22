package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/fatballfish/pic-gallery/internal/config"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

func TestWorkerBootstrapWaitsWithoutStartingRuntimeDependenciesWhileSetupIsPending(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	normalCalls := 0
	waitCalls := 0
	err := runWorker(ctx, workerRunDependencies{
		runtimeEnvPath: func() string { return "runtime.env" },
		bootstrap: workerBootstrapDependencies{
			loadBootstrap: func(string) (config.BootstrapConfig, error) { return pendingAPIBootstrapForTest(), nil },
			loadInstallState: func(string) (setup.InstallState, bool, error) {
				return pendingAPIInstallStateForTest(), true, nil
			},
			wait: func(waitCtx context.Context, _ time.Duration) error {
				waitCalls++
				cancel()
				return waitCtx.Err()
			},
		},
		runNormal: func(context.Context, workerBootstrap) error {
			normalCalls++
			return nil
		},
	})
	if err != nil || normalCalls != 0 || waitCalls != 1 {
		t.Fatalf("runWorker(pending) = err %v, normal calls %d, waits %d; want graceful wait cancellation without runtime start", err, normalCalls, waitCalls)
	}
}

func TestWorkerBootstrapStartsOnlyAfterCompletedRuntimeAndInstallStateMatch(t *testing.T) {
	pendingBootstrap := pendingAPIBootstrapForTest()
	completedBootstrap := completedAPIBootstrapForTest()
	pendingState := pendingAPIInstallStateForTest()
	completedState := completedWorkerInstallStateForTest(completedBootstrap)
	iteration := 0
	normalCalls := 0
	err := runWorker(context.Background(), workerRunDependencies{
		runtimeEnvPath: func() string { return "runtime.env" },
		bootstrap: workerBootstrapDependencies{
			loadBootstrap: func(string) (config.BootstrapConfig, error) {
				iteration++
				if iteration == 1 {
					return pendingBootstrap, nil
				}
				return completedBootstrap, nil
			},
			loadInstallState: func(string) (setup.InstallState, bool, error) {
				if iteration == 1 {
					return pendingState, true, nil
				}
				return completedState, true, nil
			},
			wait: func(context.Context, time.Duration) error { return nil },
		},
		runNormal: func(ctx context.Context, startup workerBootstrap) error {
			normalCalls++
			if ctx.Err() != nil || startup.Bootstrap.InstallationID != completedBootstrap.InstallationID || startup.State.Phase != setup.InstallPhaseCompleted {
				t.Fatalf("normal worker received invalid startup: ctx=%v startup=%#v", ctx.Err(), startup)
			}
			return nil
		},
	})
	if err != nil || normalCalls != 1 || iteration != 2 {
		t.Fatalf("runWorker(completes) = err %v, normal calls %d, loads %d; want one start after second snapshot", err, normalCalls, iteration)
	}
}

func TestWorkerBootstrapWaitsThroughControlNodeCommitReconciliation(t *testing.T) {
	committingBootstrap := completedAPIBootstrapForTest()
	committingState := completedWorkerInstallStateForTest(committingBootstrap)
	committingState.Phase = setup.InstallPhaseCommitting
	committingState.EverCompleted = false
	completedState := completedWorkerInstallStateForTest(committingBootstrap)
	iteration := 0
	waits := 0

	startup, err := waitForWorkerBootstrap(context.Background(), "runtime.env", workerBootstrapDependencies{
		loadBootstrap: func(string) (config.BootstrapConfig, error) {
			iteration++
			return committingBootstrap, nil
		},
		loadInstallState: func(string) (setup.InstallState, bool, error) {
			if iteration == 1 {
				return committingState, true, nil
			}
			return completedState, true, nil
		},
		wait: func(context.Context, time.Duration) error {
			waits++
			return nil
		},
	})
	if err != nil || startup.State.Phase != setup.InstallPhaseCompleted || waits != 1 {
		t.Fatalf("waitForWorkerBootstrap(reconciliation) = %#v, %v, waits %d", startup, err, waits)
	}
}

func TestWorkerBootstrapRetriesAnInconsistentCrossFileSnapshotBeforeFailingClosed(t *testing.T) {
	pendingBootstrap := pendingAPIBootstrapForTest()
	completedBootstrap := completedAPIBootstrapForTest()
	completedState := completedWorkerInstallStateForTest(completedBootstrap)
	iteration := 0
	waits := 0

	startup, err := waitForWorkerBootstrap(context.Background(), "runtime.env", workerBootstrapDependencies{
		loadBootstrap: func(string) (config.BootstrapConfig, error) {
			iteration++
			if iteration == 1 {
				return pendingBootstrap, nil
			}
			return completedBootstrap, nil
		},
		loadInstallState: func(string) (setup.InstallState, bool, error) {
			return completedState, true, nil
		},
		wait: func(context.Context, time.Duration) error {
			waits++
			return nil
		},
	})
	if err != nil || startup.State.Phase != setup.InstallPhaseCompleted || iteration != 2 || waits != 1 {
		t.Fatalf("cross-file commit snapshot = startup %#v, err %v, loads %d, waits %d; want retry into completed state", startup, err, iteration, waits)
	}
}

func TestWorkerBootstrapRejectsUninitializedJoinedWorkerAndIdentityMismatch(t *testing.T) {
	tests := []struct {
		name      string
		bootstrap config.BootstrapConfig
		state     setup.InstallState
		exists    bool
	}{
		{
			name: "joined worker cannot enter setup",
			bootstrap: func() config.BootstrapConfig {
				bootstrap := pendingAPIBootstrapForTest()
				bootstrap.Deployment.Topology = config.DeploymentTopologyCluster
				bootstrap.Deployment.Role = config.DeploymentRoleWorker
				bootstrap.Values["DEPLOYMENT_TOPOLOGY"] = "cluster"
				bootstrap.Values["DEPLOYMENT_ROLE"] = "worker"
				return bootstrap
			}(),
			exists: false,
		},
		{
			name:      "runtime and state identities differ",
			bootstrap: completedAPIBootstrapForTest(),
			state: func() setup.InstallState {
				state := completedWorkerInstallStateForTest(completedAPIBootstrapForTest())
				state.InstallationID = "019d0000-0000-7000-8000-000000000099"
				state.Commit.InstallationID = state.InstallationID
				return state
			}(),
			exists: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			waits := 0
			_, err := waitForWorkerBootstrap(context.Background(), "runtime.env", workerBootstrapDependencies{
				loadBootstrap: func(string) (config.BootstrapConfig, error) { return testCase.bootstrap, nil },
				loadInstallState: func(string) (setup.InstallState, bool, error) {
					return testCase.state, testCase.exists, nil
				},
				wait: func(context.Context, time.Duration) error { waits++; return nil },
			})
			if !errors.Is(err, ErrWorkerBootstrapInvalid) || waits != 1 {
				t.Fatalf("waitForWorkerBootstrap = %v, waits %d; want one stability read then fail-closed invalid bootstrap", err, waits)
			}
		})
	}
}

func TestWorkerBootstrapErrorsDoNotExposeLoaderSecrets(t *testing.T) {
	secret := "postgres://worker:do-not-log-me@example.invalid/app"
	_, err := waitForWorkerBootstrap(context.Background(), "runtime.env", workerBootstrapDependencies{
		loadBootstrap: func(string) (config.BootstrapConfig, error) {
			return config.BootstrapConfig{}, errors.New("failed value " + secret)
		},
		loadInstallState: func(string) (setup.InstallState, bool, error) {
			return setup.InstallState{}, false, nil
		},
	})
	if !errors.Is(err, ErrWorkerBootstrapInvalid) || strings.Contains(err.Error(), secret) {
		t.Fatalf("worker bootstrap error must be typed and secret-free: %v", err)
	}
}

func TestWorkerNormalStartupVerifiesCompletedBindingBeforeRuntimeServices(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, "file:worker-binding-order?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open test client: %v", err)
	}
	bindingErr := errors.New("completed binding rejected")
	checks := make([]string, 0, 2)
	startup := workerBootstrap{
		Bootstrap: completedAPIBootstrapForTest(),
		State:     completedWorkerInstallStateForTest(completedAPIBootstrapForTest()),
	}
	err = runNormalWorkerWithOptions(context.Background(), startup, workerNormalStartupOptions{
		openDatabase: func(context.Context, string) (*repoent.Client, error) {
			return client, nil
		},
		checkSchemaCompatibility: func(context.Context, *repoent.Client, config.Config) error {
			checks = append(checks, "schema")
			return nil
		},
		verifyCompletedBinding: func(context.Context, workerBootstrap) error {
			checks = append(checks, "binding")
			return bindingErr
		},
	})
	if !errors.Is(err, bindingErr) || strings.Join(checks, ",") != "schema,binding" {
		t.Fatalf("runNormalWorkerWithOptions = err %v, checks %v; want schema then binding rejection before runtime services", err, checks)
	}
}

func TestWorkerStartupKeepsCompatibilityCheckAndContainsNoMigration(t *testing.T) {
	workerSource, err := os.ReadFile("worker.go")
	if err != nil {
		t.Fatalf("read worker.go: %v", err)
	}
	source := string(workerSource)
	for _, required := range []string{"options.openDatabase = db.OpenContext", "options.checkSchemaCompatibility = checkRuntimeSchemaCompatibility", "verifyCompletedStartupBinding(ctx", "runner.Run(ctx)"} {
		if !strings.Contains(source, required) {
			t.Errorf("worker startup missing %q", required)
		}
	}
	for _, name := range []string{"worker.go", "worker_bootstrap.go"} {
		contents, readErr := os.ReadFile(name)
		if readErr != nil {
			t.Fatalf("read %s: %v", name, readErr)
		}
		for _, forbidden := range []string{"db.Migrate(", "RunDatabaseMigration(", ".Schema.Create(", "PrepareLegacyData("} {
			if strings.Contains(string(contents), forbidden) {
				t.Errorf("ordinary worker startup %s contains migration call %q", name, forbidden)
			}
		}
	}
}

func TestWorkerCommandUsesSignalContextAndSharedExitCode(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("..", "..", "cmd", "worker", "main.go"))
	if err != nil {
		t.Fatalf("read worker main: %v", err)
	}
	source := string(contents)
	for _, required := range []string{"signal.NotifyContext", "app.RunWorkerContext(ctx)", "app.ExitCode(err)"} {
		if !strings.Contains(source, required) {
			t.Errorf("worker command missing %q", required)
		}
	}
}

func completedWorkerInstallStateForTest(bootstrap config.BootstrapConfig) setup.InstallState {
	return setup.InstallState{
		SchemaVersion: setup.CurrentInstallStateSchemaVersion, InstallationID: bootstrap.InstallationID,
		DeploymentRole: bootstrap.Deployment.Role, Phase: setup.InstallPhaseCompleted, EverCompleted: true,
		UpdatedAt: time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC),
		Commit: &setup.CommitProof{
			OperationID: "019d0000-0000-7000-8000-000000000010", InstallationID: bootstrap.InstallationID,
			RuntimeSchemaVersion: bootstrap.SchemaVersion, ConfigRevision: bootstrap.ConfigRevision,
			RequestDigest: strings.Repeat("a", 64),
		},
	}
}
