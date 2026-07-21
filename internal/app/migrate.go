package app

import (
	"context"
	"fmt"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
)

type runtimeLoader func(string) (config.Config, error)
type databaseMigrator func(context.Context, string, db.MigrationRequest) (db.MigrationResult, error)

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
