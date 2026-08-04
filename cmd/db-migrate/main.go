package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/fatballfish/pic-gallery/internal/app"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

func main() {
	if err := run(); err != nil {
		log.Printf("database migration failed: %v", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runMigrationCommand(ctx, os.Args[1:], os.Stdout, app.RunDatabaseMigration, app.RunUpgradeDatabaseMigration)
}

type migrationRunner func(context.Context, string) (db.MigrationResult, error)
type upgradeMigrationRunner func(context.Context, string, setup.LegacySetupReleaseIdentity) (db.MigrationResult, error)

func runMigrationCommand(ctx context.Context, args []string, output io.Writer, regular migrationRunner, upgrade upgradeMigrationRunner) error {
	flags := flag.NewFlagSet("mikiko-gallery-studio-db-migrate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	runtimeEnvPath := flags.String("env-file", "", "runtime env path (defaults to APP_ENV_FILE or ./config/runtime.env)")
	reconcileLegacyBinding := flags.Bool("reconcile-legacy-setup-binding", false, "reconcile a verified legacy setup binding during application upgrade")
	legacyApplicationVersion := flags.String("legacy-application-version", "", "application version used by the completed legacy setup binding")
	legacyImageRegistry := flags.String("legacy-image-registry", "", "image registry used by the completed legacy setup binding")
	legacyImageTag := flags.String("legacy-image-tag", "", "image tag used by the completed legacy setup binding")
	legacyReleaseVersion := flags.String("legacy-release-version", "", "native release version used by the completed legacy setup binding")
	if err := flags.Parse(args); err != nil {
		return err
	}
	var result db.MigrationResult
	var err error
	if *reconcileLegacyBinding {
		if upgrade == nil {
			return fmt.Errorf("upgrade database migration runner is required")
		}
		result, err = upgrade(ctx, *runtimeEnvPath, setup.LegacySetupReleaseIdentity{
			ApplicationVersion: *legacyApplicationVersion, ImageRegistry: *legacyImageRegistry,
			ImageTag: *legacyImageTag, ReleaseVersion: *legacyReleaseVersion,
		})
	} else {
		if regular == nil {
			return fmt.Errorf("database migration runner is required")
		}
		result, err = regular(ctx, *runtimeEnvPath)
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(output, formatMigrationResult(result))
	if err != nil {
		return fmt.Errorf("write migration result: %w", err)
	}
	return nil
}

func formatMigrationResult(result db.MigrationResult) string {
	return fmt.Sprintf(
		"database migration complete: installation=%s database_schema=%d config_schema=%d app=%q changed=%t backfilled=%d",
		result.Current.InstallationID,
		result.Current.DatabaseSchemaVersion,
		result.Current.ConfigVersion,
		result.Current.AppVersion,
		result.Changed,
		result.BackfilledRows,
	)
}
