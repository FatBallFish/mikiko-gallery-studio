package app

import (
	"context"
	"fmt"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
)

func checkRuntimeSchemaCompatibility(ctx context.Context, client *repoent.Client, cfg config.Config) error {
	expected := db.SchemaVersion{
		InstallationID:        cfg.Runtime.InstallationID,
		AppVersion:            cfg.Runtime.ApplicationVersion,
		ConfigVersion:         cfg.Runtime.ConfigSchemaVersion,
		DatabaseSchemaVersion: db.CurrentDatabaseSchemaVersion,
	}
	if err := db.CheckSchemaCompatibility(ctx, client, expected); err != nil {
		return fmt.Errorf("check database schema compatibility: %w", err)
	}
	return nil
}
