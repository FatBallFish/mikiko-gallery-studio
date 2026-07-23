package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
)

func TestRunDatabaseMigrationLoadsOneRuntimeSnapshot(t *testing.T) {
	wantConfig := config.Config{
		Runtime: config.RuntimeConfig{
			DeploymentRole:      config.DeploymentRoleSingle,
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

func TestRunDatabaseMigrationSnapshotUsesProvidedConfiguration(t *testing.T) {
	cfg := config.Config{
		Runtime: config.RuntimeConfig{
			DeploymentRole: config.DeploymentRoleSingle, InstallationID: "validated-installation",
			ApplicationVersion: "validated-version", ConfigSchemaVersion: 7,
		},
		Database: config.DatabaseConfig{URL: "postgres://validated:secret@validated-db/app"},
	}
	called := 0
	result, err := runDatabaseMigrationSnapshot(t.Context(), cfg, func(_ context.Context, databaseURL string, request db.MigrationRequest) (db.MigrationResult, error) {
		called++
		if databaseURL != cfg.Database.URL || request.InstallationID != cfg.Runtime.InstallationID || request.AppVersion != cfg.Runtime.ApplicationVersion || request.ConfigVersion != cfg.Runtime.ConfigSchemaVersion {
			t.Fatalf("migration received a different configuration snapshot: url=%q request=%+v", databaseURL, request)
		}
		return db.MigrationResult{Changed: true}, nil
	})
	if err != nil || !result.Changed || called != 1 {
		t.Fatalf("runDatabaseMigrationSnapshot = (%+v, %v), calls=%d", result, err, called)
	}
}

func TestRunDatabaseMigrationEnforcesControlRoleBeforeMigrator(t *testing.T) {
	tests := []struct {
		name    string
		role    config.DeploymentRole
		allowed bool
	}{
		{name: "single", role: config.DeploymentRoleSingle, allowed: true},
		{name: "control", role: config.DeploymentRoleControl, allowed: true},
		{name: "api", role: config.DeploymentRoleAPI},
		{name: "worker", role: config.DeploymentRoleWorker},
		{name: "web", role: config.DeploymentRoleWeb},
		{name: "empty"},
		{name: "unknown", role: config.DeploymentRole("unknown")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			migrateCalls := 0
			cfg := config.Config{
				Runtime: config.RuntimeConfig{
					DeploymentRole:      tt.role,
					InstallationID:      "installation-test",
					ApplicationVersion:  "v1",
					ConfigSchemaVersion: 1,
				},
				Database: config.DatabaseConfig{URL: "postgres://app:do-not-leak@db/app"},
			}
			loader := func(string) (config.Config, error) { return cfg, nil }
			migrator := func(context.Context, string, db.MigrationRequest) (db.MigrationResult, error) {
				migrateCalls++
				return db.MigrationResult{}, nil
			}

			_, err := runDatabaseMigration(context.Background(), "runtime.env", loader, migrator)
			if tt.allowed {
				if err != nil {
					t.Fatalf("allowed role %q rejected: %v", tt.role, err)
				}
				if migrateCalls != 1 {
					t.Fatalf("allowed role migrator calls = %d, want 1", migrateCalls)
				}
				return
			}
			if !errors.Is(err, ErrDatabaseMigrationRoleForbidden) {
				t.Fatalf("role %q error = %T %v, want forbidden sentinel", tt.role, err, err)
			}
			var roleErr *DatabaseMigrationRoleError
			if !errors.As(err, &roleErr) || roleErr.Role != tt.role {
				t.Fatalf("role %q error = %T %v, want typed role error", tt.role, err, err)
			}
			if migrateCalls != 0 {
				t.Fatalf("forbidden role migrator calls = %d, want 0", migrateCalls)
			}
			if strings.Contains(err.Error(), "do-not-leak") {
				t.Fatalf("role rejection leaked database credentials: %v", err)
			}
		})
	}
}
