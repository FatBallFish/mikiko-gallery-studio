package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
)

// Exercise the same Ent schema creation and installation-version recording used
// by migrateLocked, without requiring a PostgreSQL service for the regression.
func TestRouteImagePricingSchemaV10BlocksUnmigratedV9(t *testing.T) {
	if CurrentDatabaseSchemaVersion != 10 {
		t.Fatalf("image_pricing release requires schema 10, got %d", CurrentDatabaseSchemaVersion)
	}
	ctx := context.Background()
	const dsn = "file:route-image-pricing-v9?mode=memory&cache=shared&_fk=1"
	client := openMigrationSQLite(t, "route-image-pricing-v9")
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create fixture schema: %v", err)
	}
	raw, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open fixture SQL connection: %v", err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	req := MigrationRequest{InstallationID: "image-pricing-installation", AppVersion: "v0.0.38", ConfigVersion: config.CurrentRuntimeSchemaVersion}
	if _, err := client.Installation.Create().
		SetSingletonKey(installationSingletonKey).
		SetInstallationID(req.InstallationID).
		SetAppVersion(req.AppVersion).
		SetConfigSchemaVersion(req.ConfigVersion).
		SetDatabaseSchemaVersion(9).
		SetInitializedAt(time.Now().UTC()).
		SetMigratedAt(time.Now().UTC()).Save(ctx); err != nil {
		t.Fatalf("create schema 9 installation: %v", err)
	}
	if _, err := client.RouteModel.Create().SetCode("legacy-image").SetName("Legacy image").Save(ctx); err != nil {
		t.Fatalf("seed legacy route: %v", err)
	}
	if _, err := raw.ExecContext(ctx, "ALTER TABLE route_models DROP COLUMN image_pricing"); err != nil {
		t.Fatalf("simulate schema 9 without image_pricing: %v", err)
	}
	expected := SchemaVersion{InstallationID: req.InstallationID, AppVersion: req.AppVersion, ConfigVersion: req.ConfigVersion, DatabaseSchemaVersion: CurrentDatabaseSchemaVersion}
	// Keep the app version identical so the database-schema gate itself must
	// stop a new Worker before its first routing query selects the missing column.
	assertCompatibilityErrorKind(t, CheckSchemaCompatibility(ctx, client, expected), CompatibilityDatabaseSchemaMismatch)
	if _, err := client.RouteModel.Query().All(ctx); err == nil {
		t.Fatal("unmigrated routing query unexpectedly succeeded without image_pricing")
	}
	var version int
	if err := raw.QueryRowContext(ctx, "SELECT database_schema_version FROM installations").Scan(&version); err != nil || version != 9 {
		t.Fatalf("startup compatibility check changed schema version: version=%d err=%v", version, err)
	}
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("official migrator schema creation: %v", err)
	}
	result, err := recordInstallationMigration(ctx, client, req)
	if err != nil {
		t.Fatalf("official migrator version recording: %v", err)
	}
	if result.Previous == nil || result.Previous.DatabaseSchemaVersion != 9 || result.Current.DatabaseSchemaVersion != 10 || !result.Changed {
		t.Fatalf("unexpected migration result: %#v", result)
	}
	if err := CheckSchemaCompatibility(ctx, client, expected); err != nil {
		t.Fatalf("migrated schema rejected: %v", err)
	}
	route, err := client.RouteModel.Query().Only(ctx)
	if err != nil || route.Code != "legacy-image" || route.ImagePricing != nil {
		t.Fatalf("migrated route must remain readable with unconfigured pricing: route=%#v err=%v", route, err)
	}
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("repeat schema creation: %v", err)
	}
	repeated, err := recordInstallationMigration(ctx, client, req)
	if err != nil || repeated.Changed {
		t.Fatalf("migration must be idempotent: result=%#v err=%v", repeated, err)
	}
}

// The full public migrator deliberately accepts PostgreSQL only. When an
// isolated integration server is supplied, also cover its complete lifecycle.
func TestRouteImagePricingSchemaV10OfficialPostgresMigration(t *testing.T) {
	adminURL := strings.TrimSpace(os.Getenv("PIC_GALLERY_TEST_POSTGRES_URL"))
	if adminURL == "" {
		t.Skip("set PIC_GALLERY_TEST_POSTGRES_URL to run isolated PostgreSQL migration integration")
	}
	ctx := context.Background()
	admin, err := sql.Open("postgres", adminURL)
	if err != nil {
		t.Fatalf("open integration server: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	schema := fmt.Sprintf("image_pricing_v10_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create isolated schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.ExecContext(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE") })
	databaseURL := databaseURLWithSearchPath(t, adminURL, schema)
	req := MigrationRequest{InstallationID: "image-pricing-installation", AppVersion: "v0.0.38", ConfigVersion: config.CurrentRuntimeSchemaVersion}
	if _, err := Migrate(ctx, databaseURL, req); err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	if _, err := admin.ExecContext(ctx, "ALTER TABLE "+schema+".route_models DROP COLUMN image_pricing"); err != nil {
		t.Fatalf("remove new column: %v", err)
	}
	if _, err := admin.ExecContext(ctx, "UPDATE "+schema+".installations SET database_schema_version = 9"); err != nil {
		t.Fatalf("mark legacy version: %v", err)
	}
	client, err := Open(databaseURL)
	if err != nil {
		t.Fatalf("open isolated fixture: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	expected := SchemaVersion{InstallationID: req.InstallationID, AppVersion: req.AppVersion, ConfigVersion: req.ConfigVersion, DatabaseSchemaVersion: CurrentDatabaseSchemaVersion}
	assertCompatibilityErrorKind(t, CheckSchemaCompatibility(ctx, client, expected), CompatibilityDatabaseSchemaMismatch)
	result, err := Migrate(ctx, databaseURL, req)
	if err != nil {
		t.Fatalf("official migration: %v", err)
	}
	if result.Previous == nil || result.Previous.DatabaseSchemaVersion != 9 || result.Current.DatabaseSchemaVersion != 10 || !result.Changed {
		t.Fatalf("unexpected migration result: %#v", result)
	}
	var dataType, nullable string
	if err := admin.QueryRowContext(ctx, `SELECT data_type, is_nullable FROM information_schema.columns WHERE table_schema = $1 AND table_name = 'route_models' AND column_name = 'image_pricing'`, schema).Scan(&dataType, &nullable); err != nil || dataType != "jsonb" || nullable != "YES" {
		t.Fatalf("migrated column: type=%q nullable=%q err=%v", dataType, nullable, err)
	}
	if err := CheckSchemaCompatibility(ctx, client, expected); err != nil {
		t.Fatalf("migrated compatibility: %v", err)
	}
	if _, err := client.RouteModel.Query().All(ctx); err != nil {
		t.Fatalf("routing query after migration: %v", err)
	}
	repeated, err := Migrate(ctx, databaseURL, req)
	if err != nil || repeated.Changed {
		t.Fatalf("repeat official migration: result=%#v err=%v", repeated, err)
	}
}
