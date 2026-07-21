package app

import (
	"context"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
)

func TestRunDatabaseMigrationLoadsOneRuntimeSnapshot(t *testing.T) {
	wantConfig := config.Config{
		Runtime: config.RuntimeConfig{
			InstallationID:      "installation-snapshot",
			ApplicationVersion:  "v1.2.3",
			ConfigSchemaVersion: 4,
		},
		Database: config.DatabaseConfig{URL: "postgres://app:secret@db/app"},
	}
	loadCalls := 0
	migrateCalls := 0
	loader := func(path string) (config.Config, error) {
		loadCalls++
		if path != "runtime-test.env" {
			t.Fatalf("runtime path = %q", path)
		}
		return wantConfig, nil
	}
	migrator := func(ctx context.Context, databaseURL string, request db.MigrationRequest) (db.MigrationResult, error) {
		migrateCalls++
		if databaseURL != wantConfig.Database.URL {
			t.Fatalf("database URL = %q", databaseURL)
		}
		wantRequest := db.MigrationRequest{
			InstallationID: wantConfig.Runtime.InstallationID,
			AppVersion:     wantConfig.Runtime.ApplicationVersion,
			ConfigVersion:  wantConfig.Runtime.ConfigSchemaVersion,
		}
		if request != wantRequest {
			t.Fatalf("migration request = %#v, want %#v", request, wantRequest)
		}
		return db.MigrationResult{Current: db.SchemaVersion{
			InstallationID:        request.InstallationID,
			AppVersion:            request.AppVersion,
			ConfigVersion:         request.ConfigVersion,
			DatabaseSchemaVersion: db.CurrentDatabaseSchemaVersion,
		}}, nil
	}

	result, err := runDatabaseMigration(context.Background(), "runtime-test.env", loader, migrator)
	if err != nil {
		t.Fatalf("runDatabaseMigration: %v", err)
	}
	if loadCalls != 1 || migrateCalls != 1 {
		t.Fatalf("load calls = %d, migrate calls = %d, want one each", loadCalls, migrateCalls)
	}
	if result.Current.InstallationID != wantConfig.Runtime.InstallationID {
		t.Fatalf("migration result = %#v", result)
	}
}
