package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/fatballfish/pic-gallery/internal/app"
)

func main() {
	if err := run(); err != nil {
		log.Printf("database migration failed: %v", err)
		os.Exit(1)
	}
}

func run() error {
	runtimeEnvPath := flag.String("env-file", "", "runtime env path (defaults to APP_ENV_FILE or ./config/runtime.env)")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result, err := app.RunDatabaseMigration(ctx, *runtimeEnvPath)
	if err != nil {
		return err
	}
	fmt.Printf(
		"database migration complete: installation=%s database_schema=%d config_schema=%d app=%s changed=%t backfilled=%d\n",
		result.Current.InstallationID,
		result.Current.DatabaseSchemaVersion,
		result.Current.ConfigVersion,
		result.Current.AppVersion,
		result.Changed,
		result.BackfilledRows,
	)
	return nil
}
