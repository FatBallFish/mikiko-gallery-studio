package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

type runtimeLoader func(string) (config.Config, error)
type databaseMigrator func(context.Context, string, db.MigrationRequest) (db.MigrationResult, error)
type upgradeRuntimeLoader func(string) (config.BootstrapConfig, config.Config, error)
type legacyBindingReconciler func(context.Context, config.BootstrapConfig, setup.LegacySetupReleaseIdentity) (bool, error)

var ErrDatabaseMigrationRoleForbidden = errors.New("database migration is allowed only for single or control deployment roles")

type DatabaseMigrationRoleError struct {
	Role config.DeploymentRole
}

func (e *DatabaseMigrationRoleError) Error() string {
	role := string(e.Role)
	if role == "" {
		role = "<empty>"
	}
	return fmt.Sprintf("%v: got role %q", ErrDatabaseMigrationRoleForbidden, role)
}

func (e *DatabaseMigrationRoleError) Unwrap() error {
	return ErrDatabaseMigrationRoleForbidden
}

// RunDatabaseMigration is the explicit schema mutation control operation used
// by setup and deployment tooling. Ordinary API and Worker startup never call it.
func RunDatabaseMigration(ctx context.Context, runtimeEnvPath string) (db.MigrationResult, error) {
	return runDatabaseMigration(ctx, runtimeEnvPath, config.LoadRuntime, db.Migrate)
}

// RunUpgradeDatabaseMigration runs the target schema migration and then
// canonicalizes a verifiable legacy setup binding before services are rolled.
func RunUpgradeDatabaseMigration(ctx context.Context, runtimeEnvPath string, previousRelease setup.LegacySetupReleaseIdentity) (db.MigrationResult, error) {
	return runUpgradeDatabaseMigration(ctx, runtimeEnvPath, previousRelease, loadUpgradeRuntime, db.Migrate, func(ctx context.Context, bootstrap config.BootstrapConfig, identity setup.LegacySetupReleaseIdentity) (bool, error) {
		return setup.ReconcileLegacyCompletedBinding(ctx, bootstrap, identity, setup.NewStateStore(bootstrap.Path), entstore.OpenSetupStore)
	})
}

func loadUpgradeRuntime(path string) (config.BootstrapConfig, config.Config, error) {
	bootstrap, err := config.LoadBootstrap(path)
	if err != nil {
		return config.BootstrapConfig{}, config.Config{}, err
	}
	cfg, err := config.RuntimeFromBootstrap(bootstrap)
	if err != nil {
		return config.BootstrapConfig{}, config.Config{}, err
	}
	return bootstrap, cfg, nil
}

func runUpgradeDatabaseMigration(ctx context.Context, runtimeEnvPath string, previousRelease setup.LegacySetupReleaseIdentity, load upgradeRuntimeLoader, migrate databaseMigrator, reconcile legacyBindingReconciler) (db.MigrationResult, error) {
	if load == nil || migrate == nil || reconcile == nil {
		return db.MigrationResult{}, fmt.Errorf("upgrade database migration dependencies are required")
	}
	bootstrap, cfg, err := load(runtimeEnvPath)
	if err != nil {
		return db.MigrationResult{}, fmt.Errorf("load runtime configuration for upgrade database migration: %w", err)
	}
	result, err := runDatabaseMigrationSnapshot(ctx, cfg, migrate)
	if err != nil {
		return db.MigrationResult{}, err
	}
	if _, err := reconcile(ctx, bootstrap, previousRelease); err != nil {
		return db.MigrationResult{}, fmt.Errorf("reconcile legacy setup binding after database migration: %w", err)
	}
	return result, nil
}

// RunDatabaseMigrationSnapshot performs the explicit migration using an
// already validated immutable runtime snapshot.
func RunDatabaseMigrationSnapshot(ctx context.Context, cfg config.Config) (db.MigrationResult, error) {
	return runDatabaseMigrationSnapshot(ctx, cfg, db.Migrate)
}

func runDatabaseMigration(ctx context.Context, runtimeEnvPath string, load runtimeLoader, migrate databaseMigrator) (db.MigrationResult, error) {
	if load == nil || migrate == nil {
		return db.MigrationResult{}, fmt.Errorf("database migration dependencies are required")
	}
	cfg, err := load(runtimeEnvPath)
	if err != nil {
		return db.MigrationResult{}, fmt.Errorf("load runtime configuration for database migration: %w", err)
	}
	return runDatabaseMigrationSnapshot(ctx, cfg, migrate)
}

func runDatabaseMigrationSnapshot(ctx context.Context, cfg config.Config, migrate databaseMigrator) (db.MigrationResult, error) {
	if migrate == nil {
		return db.MigrationResult{}, fmt.Errorf("database migration dependency is required")
	}
	if err := validateDatabaseMigrationRole(cfg.Runtime.DeploymentRole); err != nil {
		return db.MigrationResult{}, err
	}
	result, err := migrate(ctx, cfg.Database.URL, db.MigrationRequest{
		InstallationID: cfg.Runtime.InstallationID,
		AppVersion:     cfg.Runtime.ApplicationVersion,
		ConfigVersion:  cfg.Runtime.ConfigSchemaVersion,
	})
	if err != nil {
		return db.MigrationResult{}, fmt.Errorf("run explicit database migration: %w", err)
	}
	return result, nil
}

func validateDatabaseMigrationRole(role config.DeploymentRole) error {
	switch role {
	case config.DeploymentRoleSingle, config.DeploymentRoleControl:
		return nil
	default:
		return &DatabaseMigrationRoleError{Role: role}
	}
}
