package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
)

type runtimeLoader func(string) (config.Config, error)
type databaseMigrator func(context.Context, string, db.MigrationRequest) (db.MigrationResult, error)

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

func runDatabaseMigration(ctx context.Context, runtimeEnvPath string, load runtimeLoader, migrate databaseMigrator) (db.MigrationResult, error) {
	if load == nil || migrate == nil {
		return db.MigrationResult{}, fmt.Errorf("database migration dependencies are required")
	}
	cfg, err := load(runtimeEnvPath)
	if err != nil {
		return db.MigrationResult{}, fmt.Errorf("load runtime configuration for database migration: %w", err)
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
