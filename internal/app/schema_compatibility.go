package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

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
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		var networkErr net.Error
		if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) && errors.As(err, &networkErr) && networkErr.Timeout() {
			return context.DeadlineExceeded
		}
		return fmt.Errorf("check database schema compatibility: %w", err)
	}
	return nil
}
