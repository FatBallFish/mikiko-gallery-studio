package localbootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	"github.com/fatballfish/pic-gallery/internal/setup"
	"github.com/lib/pq"
)

func TestRunCompletesGuardedStagesInOrder(t *testing.T) {
	bootstrap, cfg, state := validLocalInputs()
	bootstrap.Path = "resolved-runtime.env"
	stages := make([]string, 0, 4)
	result, err := run(context.Background(), "runtime.env", dependencies{
		load: func(string) (config.BootstrapConfig, config.Config, setup.InstallState, error) {
			stages = append(stages, "load")
			return bootstrap, cfg, state, nil
		},
		migrate: func(_ context.Context, snapshot config.Config) (db.MigrationResult, error) {
			stages = append(stages, "migrate:"+snapshot.Database.URL)
			return db.MigrationResult{Changed: true}, nil
		},
		hash: func(password string) (string, error) {
			stages = append(stages, "hash")
			if password != AdminPassword {
				t.Fatalf("password = %q", password)
			}
			return "bcrypt$test", nil
		},
		bind: func(_ context.Context, databaseURL string, request entstore.LocalBindingRequest) (setup.SetupBinding, error) {
			stages = append(stages, "bind")
			if databaseURL != cfg.Database.URL || request.InstallationID != InstallationID || request.FreshAdminPasswordHash != "bcrypt$test" {
				t.Fatalf("binding request = %+v database=%q", request, databaseURL)
			}
			return setup.SetupBinding{OperationID: OperationID, InstallationID: InstallationID, ConfigRevision: 1, RequestDigest: strings.Repeat("b", 64), AdminEmail: AdminEmail}, nil
		},
		reconcile: func(path string, proof setup.CommitProof, _ time.Time) error {
			stages = append(stages, "reconcile")
			if path != bootstrap.Path || proof.RequestDigest != strings.Repeat("b", 64) {
				t.Fatalf("reconciliation = path:%q proof:%+v", path, proof)
			}
			return nil
		},
		now: func() time.Time { return time.Date(2026, 7, 23, 1, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if got := strings.Join(stages, ","); got != "load,migrate:"+cfg.Database.URL+",hash,bind,reconcile" {
		t.Fatalf("stage order = %q", got)
	}
	if result.RuntimePath != bootstrap.Path || !result.Migration.Changed || result.Binding.AdminEmail != AdminEmail {
		t.Fatalf("result = %+v", result)
	}
}

func TestRunStopsAtFailingStage(t *testing.T) {
	stageError := errors.New("stage failed")
	for _, stage := range []string{"load", "migrate", "hash", "bind", "reconcile"} {
		t.Run(stage, func(t *testing.T) {
			bootstrap, cfg, state := validLocalInputs()
			deps := dependencies{
				load: func(string) (config.BootstrapConfig, config.Config, setup.InstallState, error) {
					return bootstrap, cfg, state, nil
				},
				migrate: func(context.Context, config.Config) (db.MigrationResult, error) { return db.MigrationResult{}, nil },
				hash:    func(string) (string, error) { return "bcrypt$test", nil },
				bind: func(context.Context, string, entstore.LocalBindingRequest) (setup.SetupBinding, error) {
					return setup.SetupBinding{OperationID: OperationID, InstallationID: InstallationID, ConfigRevision: 1, RequestDigest: strings.Repeat("b", 64)}, nil
				},
				reconcile: func(string, setup.CommitProof, time.Time) error { return nil },
				now:       func() time.Time { return time.Now().UTC() },
			}
			switch stage {
			case "load":
				deps.load = func(string) (config.BootstrapConfig, config.Config, setup.InstallState, error) {
					return config.BootstrapConfig{}, config.Config{}, setup.InstallState{}, stageError
				}
			case "migrate":
				deps.migrate = func(context.Context, config.Config) (db.MigrationResult, error) {
					return db.MigrationResult{}, stageError
				}
			case "hash":
				deps.hash = func(string) (string, error) { return "", stageError }
			case "bind":
				deps.bind = func(context.Context, string, entstore.LocalBindingRequest) (setup.SetupBinding, error) {
					return setup.SetupBinding{}, stageError
				}
			case "reconcile":
				deps.reconcile = func(string, setup.CommitProof, time.Time) error { return stageError }
			}
			if _, err := run(context.Background(), "runtime.env", deps); !errors.Is(err, stageError) {
				t.Fatalf("run error = %v, want wrapped stage error", err)
			}
		})
	}
}

func TestRunRetriesPostgresStartupMigrationErrors(t *testing.T) {
	bootstrap, cfg, state := validLocalInputs()
	attempts := 0
	waits := 0
	result, err := run(context.Background(), "runtime.env", successfulDependencies(bootstrap, cfg, state, func(context.Context, config.Config) (db.MigrationResult, error) {
		attempts++
		if attempts < 3 {
			return db.MigrationResult{}, fmt.Errorf("reserve migration connection: %w", &pq.Error{Code: "57P03", Message: "the database system is starting up"})
		}
		return db.MigrationResult{Changed: true}, nil
	}, func(context.Context, time.Duration) error {
		waits++
		return nil
	}))
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if attempts != 3 || waits != 2 || !result.Migration.Changed {
		t.Fatalf("migration retry result = attempts:%d waits:%d result:%+v", attempts, waits, result.Migration)
	}
}

func TestRunDoesNotRetryNonStartupMigrationErrors(t *testing.T) {
	bootstrap, cfg, state := validLocalInputs()
	wantErr := errors.New("migration statement failed")
	attempts := 0
	waits := 0
	_, err := run(context.Background(), "runtime.env", successfulDependencies(bootstrap, cfg, state, func(context.Context, config.Config) (db.MigrationResult, error) {
		attempts++
		return db.MigrationResult{}, wantErr
	}, func(context.Context, time.Duration) error {
		waits++
		return nil
	}))
	if !errors.Is(err, wantErr) {
		t.Fatalf("run error = %v, want wrapped migration error", err)
	}
	if attempts != 1 || waits != 0 {
		t.Fatalf("non-startup migration retried: attempts=%d waits=%d", attempts, waits)
	}
}

func TestRunStopsMigrationRetryWhenContextIsCanceled(t *testing.T) {
	bootstrap, cfg, state := validLocalInputs()
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	waits := 0
	_, err := run(ctx, "runtime.env", successfulDependencies(bootstrap, cfg, state, func(context.Context, config.Config) (db.MigrationResult, error) {
		attempts++
		return db.MigrationResult{}, &pq.Error{Code: "57P03", Message: "the database system is starting up"}
	}, func(context.Context, time.Duration) error {
		waits++
		cancel()
		return ctx.Err()
	}))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context cancellation", err)
	}
	if attempts != 1 || waits != 1 {
		t.Fatalf("canceled migration retry continued: attempts=%d waits=%d", attempts, waits)
	}
}

func TestRunBoundsPostgresStartupMigrationRetries(t *testing.T) {
	bootstrap, cfg, state := validLocalInputs()
	attempts := 0
	waits := 0
	_, err := run(context.Background(), "runtime.env", successfulDependencies(bootstrap, cfg, state, func(context.Context, config.Config) (db.MigrationResult, error) {
		attempts++
		return db.MigrationResult{}, &pq.Error{Code: "57P03", Message: "the database system is starting up"}
	}, func(context.Context, time.Duration) error {
		waits++
		return nil
	}))
	if err == nil || !strings.Contains(err.Error(), "database system is starting up") {
		t.Fatalf("run error = %v, want final PostgreSQL startup error", err)
	}
	if attempts != localMigrationMaxAttempts || waits != localMigrationMaxAttempts-1 {
		t.Fatalf("migration retry bounds = attempts:%d waits:%d", attempts, waits)
	}
}

func successfulDependencies(
	bootstrap config.BootstrapConfig,
	cfg config.Config,
	state setup.InstallState,
	migrate func(context.Context, config.Config) (db.MigrationResult, error),
	wait func(context.Context, time.Duration) error,
) dependencies {
	return dependencies{
		load: func(string) (config.BootstrapConfig, config.Config, setup.InstallState, error) {
			return bootstrap, cfg, state, nil
		},
		migrate: migrate,
		wait:    wait,
		hash:    func(string) (string, error) { return "bcrypt$test", nil },
		bind: func(context.Context, string, entstore.LocalBindingRequest) (setup.SetupBinding, error) {
			return setup.SetupBinding{OperationID: OperationID, InstallationID: InstallationID, ConfigRevision: 1, RequestDigest: strings.Repeat("b", 64)}, nil
		},
		reconcile: func(string, setup.CommitProof, time.Time) error { return nil },
		now:       func() time.Time { return time.Now().UTC() },
	}
}

func TestRunRejectsNonLocalConfigurationBeforeMigration(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*config.BootstrapConfig, *config.Config, *setup.InstallState)
	}{
		{name: "application environment", mutate: func(_ *config.BootstrapConfig, cfg *config.Config, _ *setup.InstallState) { cfg.App.Env = "production" }},
		{name: "deployment mode", mutate: func(bootstrap *config.BootstrapConfig, _ *config.Config, _ *setup.InstallState) {
			bootstrap.Deployment.Mode = config.DeploymentModeNative
		}},
		{name: "deployment profile", mutate: func(bootstrap *config.BootstrapConfig, _ *config.Config, _ *setup.InstallState) {
			bootstrap.Deployment.Profile = config.DeploymentProfileCore
		}},
		{name: "deployment topology", mutate: func(bootstrap *config.BootstrapConfig, _ *config.Config, _ *setup.InstallState) {
			bootstrap.Deployment.Topology = config.DeploymentTopologyCluster
		}},
		{name: "deployment role", mutate: func(bootstrap *config.BootstrapConfig, _ *config.Config, _ *setup.InstallState) {
			bootstrap.Deployment.Role = config.DeploymentRoleWorker
		}},
		{name: "installation identity", mutate: func(bootstrap *config.BootstrapConfig, _ *config.Config, _ *setup.InstallState) {
			bootstrap.InstallationID = "production-installation"
		}},
		{name: "runtime identity", mutate: func(_ *config.BootstrapConfig, cfg *config.Config, _ *setup.InstallState) {
			cfg.Runtime.InstallationID = "production-installation"
		}},
		{name: "setup incomplete", mutate: func(bootstrap *config.BootstrapConfig, _ *config.Config, _ *setup.InstallState) {
			bootstrap.SetupCompleted = false
		}},
		{name: "config revision", mutate: func(bootstrap *config.BootstrapConfig, _ *config.Config, _ *setup.InstallState) {
			bootstrap.ConfigRevision = 2
		}},
		{name: "deployment modules", mutate: func(bootstrap *config.BootstrapConfig, _ *config.Config, _ *setup.InstallState) {
			bootstrap.DeploymentModules = bootstrap.DeploymentModules[:len(bootstrap.DeploymentModules)-1]
		}},
		{name: "database host", mutate: func(_ *config.BootstrapConfig, cfg *config.Config, _ *setup.InstallState) {
			cfg.Database.URL = "postgres://postgres@external-db:5432/pic_gallery?sslmode=disable"
		}},
		{name: "database user", mutate: func(_ *config.BootstrapConfig, cfg *config.Config, _ *setup.InstallState) {
			cfg.Database.URL = "postgres://other@postgres:5432/pic_gallery?sslmode=disable"
		}},
		{name: "database name", mutate: func(_ *config.BootstrapConfig, cfg *config.Config, _ *setup.InstallState) {
			cfg.Database.URL = "postgres://postgres@postgres:5432/other?sslmode=disable"
		}},
		{name: "database hostaddr override", mutate: func(_ *config.BootstrapConfig, cfg *config.Config, _ *setup.InstallState) {
			cfg.Database.URL = "postgres://postgres@postgres:5432/pic_gallery?sslmode=disable&hostaddr=203.0.113.10"
		}},
		{name: "database password", mutate: func(_ *config.BootstrapConfig, cfg *config.Config, _ *setup.InstallState) {
			cfg.Database.URL = "postgres://postgres:secret@postgres:5432/pic_gallery?sslmode=disable"
		}},
		{name: "redis host", mutate: func(_ *config.BootstrapConfig, cfg *config.Config, _ *setup.InstallState) {
			cfg.Redis.URL = "redis://external-cache:6379/0"
		}},
		{name: "redis database", mutate: func(_ *config.BootstrapConfig, cfg *config.Config, _ *setup.InstallState) {
			cfg.Redis.URL = "redis://redis:6379/1"
		}},
		{name: "redis query override", mutate: func(_ *config.BootstrapConfig, cfg *config.Config, _ *setup.InstallState) {
			cfg.Redis.URL = "redis://redis:6379/0?protocol=2"
		}},
		{name: "storage", mutate: func(_ *config.BootstrapConfig, cfg *config.Config, _ *setup.InstallState) {
			cfg.Storage.Driver = "s3"
		}},
		{name: "state identity", mutate: func(_ *config.BootstrapConfig, _ *config.Config, state *setup.InstallState) {
			state.InstallationID = "production-installation"
		}},
		{name: "state phase", mutate: func(_ *config.BootstrapConfig, _ *config.Config, state *setup.InstallState) {
			state.Phase = setup.InstallPhasePending
			state.EverCompleted = false
			state.Commit = nil
		}},
		{name: "state operation", mutate: func(_ *config.BootstrapConfig, _ *config.Config, state *setup.InstallState) {
			state.Commit.OperationID = "production-setup"
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bootstrap, cfg, state := validLocalInputs()
			tt.mutate(&bootstrap, &cfg, &state)
			migrations := 0
			_, err := run(context.Background(), "runtime.env", dependencies{
				load: func(string) (config.BootstrapConfig, config.Config, setup.InstallState, error) {
					return bootstrap, cfg, state, nil
				},
				migrate: func(context.Context, config.Config) (db.MigrationResult, error) {
					migrations++
					return db.MigrationResult{}, nil
				},
			})
			if err == nil || !strings.Contains(err.Error(), "local bootstrap") {
				t.Fatalf("run() error = %v, want local bootstrap validation error", err)
			}
			if migrations != 0 {
				t.Fatalf("migration called %d times for invalid local configuration", migrations)
			}
		})
	}
}

func TestLoadInputsAndReconcileStateUseTheSameRuntimeDirectory(t *testing.T) {
	configDir := t.TempDir()
	runtimePath := filepath.Join(configDir, "runtime.env")
	statePath := filepath.Join(configDir, "install-state.json")
	for source, target := range map[string]string{
		filepath.Join("..", "..", "config", "runtime.local.env.example"):        runtimePath,
		filepath.Join("..", "..", "config", "install-state.local.json.example"): statePath,
	} {
		content, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bootstrap, _, state, err := loadInputs(runtimePath)
	if err != nil {
		t.Fatalf("loadInputs returned error: %v", err)
	}
	proof := *state.Commit
	proof.RequestDigest = strings.Repeat("c", 64)
	if err := reconcileState(bootstrap.Path, proof, time.Now().UTC()); err != nil {
		t.Fatalf("reconcileState returned error: %v", err)
	}
	loaded, exists, err := setup.NewStateStore(runtimePath).Load()
	if err != nil || !exists || loaded.Commit == nil || loaded.Commit.RequestDigest != proof.RequestDigest {
		t.Fatalf("reconciled state = (%+v, %t, %v)", loaded, exists, err)
	}
}

func TestProductionRunRejectsMissingRuntime(t *testing.T) {
	if _, err := Run(context.Background(), filepath.Join(t.TempDir(), "missing.env")); err == nil {
		t.Fatal("Run accepted a missing runtime file")
	}
}

func TestLoadInputsRejectsMissingInstallState(t *testing.T) {
	configDir := t.TempDir()
	runtimePath := filepath.Join(configDir, "runtime.env")
	content, err := os.ReadFile(filepath.Join("..", "..", "config", "runtime.local.env.example"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runtimePath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := loadInputs(runtimePath); err == nil || !strings.Contains(err.Error(), "state is missing") {
		t.Fatalf("loadInputs error = %v, want missing state diagnostic", err)
	}
}

func TestRunRejectsIncompleteDependencies(t *testing.T) {
	if _, err := run(context.Background(), "runtime.env", dependencies{}); err == nil {
		t.Fatal("run accepted incomplete load/migrate dependencies")
	}
	bootstrap, cfg, state := validLocalInputs()
	if _, err := run(context.Background(), "runtime.env", dependencies{
		load: func(string) (config.BootstrapConfig, config.Config, setup.InstallState, error) {
			return bootstrap, cfg, state, nil
		},
		migrate: func(context.Context, config.Config) (db.MigrationResult, error) { return db.MigrationResult{}, nil },
	}); err == nil {
		t.Fatal("run accepted incomplete post-migration dependencies")
	}
}

func validLocalInputs() (config.BootstrapConfig, config.Config, setup.InstallState) {
	bootstrap := config.BootstrapConfig{
		SchemaVersion: config.CurrentRuntimeSchemaVersion,
		Deployment: config.DeploymentContext{
			Mode: config.DeploymentModeDocker, Profile: config.DeploymentProfileCustom,
			Topology: config.DeploymentTopologySingle, Role: config.DeploymentRoleSingle,
			StorageDriver: "local", SetupCompleted: true,
		},
		DeploymentModules: []string{"postgres", "redis", "minio", "mailpit", "api", "worker", "user-web", "admin-web", "docs-web", "nginx"},
		SetupCompleted:    true, InstallationID: InstallationID, ConfigRevision: 1, ApplicationVersion: "dev",
		Values: map[string]string{"PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY": "local-secret"},
	}
	cfg := config.Config{
		Runtime:  config.RuntimeConfig{DeploymentRole: config.DeploymentRoleSingle, InstallationID: InstallationID, ApplicationVersion: "dev", ConfigSchemaVersion: config.CurrentRuntimeSchemaVersion, ConfigRevision: 1},
		App:      config.AppConfig{Env: "local"},
		Database: config.DatabaseConfig{URL: "postgres://postgres@postgres:5432/pic_gallery?sslmode=disable"},
		Redis:    config.RedisConfig{URL: "redis://redis:6379/0"},
		Storage:  config.StorageConfig{Driver: "local", LocalRoot: "/var/lib/pic-gallery/storage", SharedVolume: true},
	}
	state := setup.InstallState{
		SchemaVersion: setup.CurrentInstallStateSchemaVersion, InstallationID: InstallationID,
		DeploymentRole: config.DeploymentRoleSingle, Phase: setup.InstallPhaseCompleted, EverCompleted: true,
		UpdatedAt: time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC),
		Commit: &setup.CommitProof{
			OperationID: OperationID, InstallationID: InstallationID,
			RuntimeSchemaVersion: config.CurrentRuntimeSchemaVersion, ConfigRevision: 1,
			RequestDigest: strings.Repeat("a", 64),
		},
	}
	return bootstrap, cfg, state
}
