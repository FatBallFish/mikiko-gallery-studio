package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/configitem"
	"github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

func TestPrepareLegacyDataSkipsNonPostgresDatabases(t *testing.T) {
	for _, url := range []string{"file:test.db", "sqlite://test.db", ":memory:"} {
		if err := PrepareLegacyData(context.Background(), url); err != nil {
			t.Fatalf("PrepareLegacyData(%q): %v", url, err)
		}
	}
}

func TestIsPostgresURL(t *testing.T) {
	for _, url := range []string{"postgres://db/app", " postgresql://db/app "} {
		if !isPostgresURL(url) {
			t.Fatalf("expected postgres URL: %q", url)
		}
	}
	if isPostgresURL("file:app.db") {
		t.Fatal("sqlite URL must not be treated as postgres")
	}
}

func TestPrepareLegacyDataRemovesObsoleteRoutePriceQuality(t *testing.T) {
	database, databaseURL := openLegacyMigrationPostgres(t)
	ctx := context.Background()
	if _, err := database.ExecContext(ctx, `
		CREATE TABLE route_model_prices (
			id bigint PRIMARY KEY,
			route_model_id bigint NOT NULL,
			task_type varchar(64) NOT NULL,
			base_resolution varchar(32) NOT NULL,
			base_points numeric(18,5) NOT NULL DEFAULT 0.00000,
			reference_multiplier numeric(18,5) NOT NULL DEFAULT 1.00000,
			enabled boolean NOT NULL DEFAULT true,
			quality varchar(32) NOT NULL
		);
		CREATE UNIQUE INDEX routemodelprice_route_model_id_task_type_base_resolution
			ON route_model_prices(route_model_id, task_type, base_resolution);
		CREATE UNIQUE INDEX routemodelprice_route_model_id_task_type_quality
			ON route_model_prices(route_model_id, task_type, quality);
		INSERT INTO route_model_prices
			(id, route_model_id, task_type, base_resolution, base_points, reference_multiplier, enabled, quality)
		VALUES (1, 1, 'text_to_image', '1k', 1.00000, 1.00000, true, '1k');
	`); err != nil {
		t.Fatalf("create route model price compatibility fixture: %v", err)
	}

	if err := PrepareLegacyData(ctx, databaseURL); err != nil {
		t.Fatalf("PrepareLegacyData: %v", err)
	}
	if err := PrepareLegacyData(ctx, databaseURL); err != nil {
		t.Fatalf("second PrepareLegacyData: %v", err)
	}

	assertColumnExists(t, database, "base_resolution", true)
	assertColumnExists(t, database, "quality", false)
	assertIndexExists(t, database, "routemodelprice_route_model_id_task_type_base_resolution", true)
	assertIndexExists(t, database, "routemodelprice_route_model_id_task_type_quality", false)
	var originalBaseResolution string
	if err := database.QueryRowContext(ctx, `SELECT base_resolution FROM route_model_prices WHERE id = 1`).Scan(&originalBaseResolution); err != nil {
		t.Fatalf("query original route price: %v", err)
	}
	if originalBaseResolution != "1k" {
		t.Fatalf("expected original base_resolution 1k, got %q", originalBaseResolution)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO route_model_prices
			(id, route_model_id, task_type, base_resolution, base_points, reference_multiplier, enabled)
		VALUES (2, 1, 'text_to_image', '2k', 2.00000, 1.00000, true)
	`); err != nil {
		t.Fatalf("insert route price without obsolete quality: %v", err)
	}
}

func TestPrepareLegacyDataRenamesLegacyRoutePriceQuality(t *testing.T) {
	database, databaseURL := openLegacyMigrationPostgres(t)
	ctx := context.Background()
	if _, err := database.ExecContext(ctx, `
		CREATE TABLE route_model_prices (
			id bigint PRIMARY KEY,
			route_model_id bigint NOT NULL,
			task_type varchar(64) NOT NULL,
			quality varchar(32) NOT NULL,
			base_points numeric(18,5) NOT NULL DEFAULT 0.00000,
			reference_multiplier numeric(18,5) NOT NULL DEFAULT 1.00000,
			enabled boolean NOT NULL DEFAULT true
		);
		CREATE UNIQUE INDEX routemodelprice_route_model_id_task_type_quality
			ON route_model_prices(route_model_id, task_type, quality);
		INSERT INTO route_model_prices
			(id, route_model_id, task_type, quality, base_points, reference_multiplier, enabled)
		VALUES (1, 1, 'text_to_image', '1k', 1.00000, 1.00000, true);
	`); err != nil {
		t.Fatalf("create legacy route model price fixture: %v", err)
	}

	if err := PrepareLegacyData(ctx, databaseURL); err != nil {
		t.Fatalf("PrepareLegacyData: %v", err)
	}

	assertColumnExists(t, database, "base_resolution", true)
	assertColumnExists(t, database, "quality", false)
	var baseResolution string
	if err := database.QueryRowContext(ctx, `SELECT base_resolution FROM route_model_prices WHERE id = 1`).Scan(&baseResolution); err != nil {
		t.Fatalf("query migrated route price: %v", err)
	}
	if baseResolution != "1k" {
		t.Fatalf("expected migrated base_resolution 1k, got %q", baseResolution)
	}
	assertIndexExists(t, database, "routemodelprice_route_model_id_task_type_base_resolution", true)
	assertIndexExists(t, database, "routemodelprice_route_model_id_task_type_quality", false)
}

func TestPrepareLegacyDataRejectsConflictingRoutePriceDimensions(t *testing.T) {
	database, databaseURL := openLegacyMigrationPostgres(t)
	ctx := context.Background()
	if _, err := database.ExecContext(ctx, `
		CREATE TABLE route_model_prices (
			id bigint PRIMARY KEY,
			route_model_id bigint NOT NULL,
			task_type varchar(64) NOT NULL,
			base_resolution varchar(32) NOT NULL,
			base_points numeric(18,5) NOT NULL DEFAULT 0.00000,
			reference_multiplier numeric(18,5) NOT NULL DEFAULT 1.00000,
			enabled boolean NOT NULL DEFAULT true,
			quality varchar(32) NOT NULL
		);
		INSERT INTO route_model_prices
			(id, route_model_id, task_type, base_resolution, base_points, reference_multiplier, enabled, quality)
		VALUES (1, 1, 'text_to_image', '2k', 1.00000, 1.00000, true, '1k');
	`); err != nil {
		t.Fatalf("create conflicting route model price fixture: %v", err)
	}

	err := PrepareLegacyData(ctx, databaseURL)
	if err == nil || !strings.Contains(err.Error(), "conflicting route model price dimensions") {
		t.Fatalf("expected conflicting dimensions error, got %v", err)
	}
	assertColumnExists(t, database, "quality", true)
}

func TestPrepareLegacyDataSQLLocksLegacyDDL(t *testing.T) {
	if !strings.Contains(prepareLegacyDataSQL, "pg_advisory_xact_lock") {
		t.Fatal("legacy DDL migration must hold a transaction advisory lock across concurrent service startup")
	}
}

func TestPrepareLegacyDataSQLPurgesRemovedReferenceGeneration(t *testing.T) {
	for _, fragment := range []string{
		"DELETE FROM task_images",
		"DELETE FROM point_ledgers",
		"DELETE FROM api_key_quota_reservations",
		"DELETE FROM image_tasks",
		"DELETE FROM model_routes",
		"DELETE FROM route_model_prices",
		"reference_generate",
		"reference_to_image",
	} {
		if !strings.Contains(prepareLegacyDataSQL, fragment) {
			t.Fatalf("reference-generation cleanup is missing %q", fragment)
		}
	}
}

func openLegacyMigrationPostgres(t *testing.T) (*sql.DB, string) {
	t.Helper()
	rawURL := os.Getenv("PIC_GALLERY_TEST_POSTGRES_URL")
	if rawURL == "" {
		t.Skip("PIC_GALLERY_TEST_POSTGRES_URL is not set")
	}
	admin, err := sql.Open("postgres", rawURL)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	schemaName := fmt.Sprintf("legacy_route_price_%d", time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE SCHEMA ` + pq.QuoteIdentifier(schemaName)); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + pq.QuoteIdentifier(schemaName) + ` CASCADE`)
	})

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse postgres URL: %v", err)
	}
	query := parsedURL.Query()
	query.Set("search_path", schemaName)
	parsedURL.RawQuery = query.Encode()
	databaseURL := parsedURL.String()
	database, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatalf("open schema postgres: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database, databaseURL
}

func assertColumnExists(t *testing.T, database *sql.DB, columnName string, expected bool) {
	t.Helper()
	var exists bool
	if err := database.QueryRow(`
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = 'route_model_prices'
			  AND column_name = $1
		)
	`, columnName).Scan(&exists); err != nil {
		t.Fatalf("query column %q: %v", columnName, err)
	}
	if exists != expected {
		t.Fatalf("column %q exists=%t, expected %t", columnName, exists, expected)
	}
}

func assertIndexExists(t *testing.T, database *sql.DB, indexName string, expected bool) {
	t.Helper()
	var exists bool
	if err := database.QueryRow(`
		SELECT EXISTS (
			SELECT 1
			FROM pg_indexes
			WHERE schemaname = current_schema()
			  AND tablename = 'route_model_prices'
			  AND indexname = $1
		)
	`, indexName).Scan(&exists); err != nil {
		t.Fatalf("query index %q: %v", indexName, err)
	}
	if exists != expected {
		t.Fatalf("index %q exists=%t, expected %t", indexName, exists, expected)
	}
}

func TestBackfillLegacyModelAccountCapabilitiesRunsOnce(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:legacy-model-capabilities?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	legacyRatios := []string{"1:1", "16:9", "9:16", "4:3", "3:4"}
	legacy, err := client.ModelAccountModel.Create().
		SetAccountID(1).
		SetModelCode("legacy-default").
		SetSupportedRatios(legacyRatios).
		SetMaxImageCount(1).
		SetMaxReferenceImageCount(4).
		Save(ctx)
	if err != nil {
		t.Fatalf("create legacy model: %v", err)
	}
	legacyEmptyRatios, err := client.ModelAccountModel.Create().
		SetAccountID(1).
		SetModelCode("legacy-empty-ratios").
		SetSupportedRatios([]string{}).
		SetMaxImageCount(1).
		SetMaxReferenceImageCount(4).
		Save(ctx)
	if err != nil {
		t.Fatalf("create legacy model with empty ratios: %v", err)
	}
	explicit, err := client.ModelAccountModel.Create().
		SetAccountID(1).
		SetModelCode("explicit-capability").
		SetSupportedRatios([]string{"1:1", "16:9"}).
		SetMaxImageCount(1).
		SetMaxReferenceImageCount(4).
		Save(ctx)
	if err != nil {
		t.Fatalf("create explicit model: %v", err)
	}

	updated, err := BackfillLegacyModelAccountCapabilities(ctx, client)
	if err != nil {
		t.Fatalf("BackfillLegacyModelAccountCapabilities: %v", err)
	}
	if updated != 2 {
		t.Fatalf("expected both legacy row shapes to be backfilled, got %d", updated)
	}
	legacy, err = client.ModelAccountModel.Get(ctx, legacy.ID)
	if err != nil {
		t.Fatalf("reload legacy model: %v", err)
	}
	if !reflect.DeepEqual(legacy.SupportedRatios, []string{"1:1"}) || legacy.MaxReferenceImageCount != 0 {
		t.Fatalf("legacy optimistic defaults were not replaced: %#v", legacy)
	}
	legacyEmptyRatios, err = client.ModelAccountModel.Get(ctx, legacyEmptyRatios.ID)
	if err != nil {
		t.Fatalf("reload legacy model with empty ratios: %v", err)
	}
	if !reflect.DeepEqual(legacyEmptyRatios.SupportedRatios, []string{"1:1"}) || legacyEmptyRatios.MaxReferenceImageCount != 0 {
		t.Fatalf("legacy empty ratios and optimistic reference limit were not replaced: %#v", legacyEmptyRatios)
	}
	explicit, err = client.ModelAccountModel.Get(ctx, explicit.ID)
	if err != nil {
		t.Fatalf("reload explicit model: %v", err)
	}
	if !reflect.DeepEqual(explicit.SupportedRatios, []string{"1:1", "16:9"}) || explicit.MaxReferenceImageCount != 4 {
		t.Fatalf("explicit capabilities must not be changed: %#v", explicit)
	}
	markerCount, err := client.ConfigItem.Query().Where(
		configitem.ConfigCategoryEQ("system_migration"),
		configitem.ConfigKeyEQ("model_account_capability_defaults_v1"),
		configitem.ScopeEQ("global"),
	).Count(ctx)
	if err != nil || markerCount != 1 {
		t.Fatalf("expected one migration marker, count=%d err=%v", markerCount, err)
	}

	if _, err := legacy.Update().SetSupportedRatios(legacyRatios).SetMaxReferenceImageCount(4).Save(ctx); err != nil {
		t.Fatalf("restore explicit post-migration capabilities: %v", err)
	}
	updated, err = BackfillLegacyModelAccountCapabilities(ctx, client)
	if err != nil {
		t.Fatalf("second BackfillLegacyModelAccountCapabilities: %v", err)
	}
	if updated != 0 {
		t.Fatalf("migration must run once, second update count=%d", updated)
	}
	legacy, err = client.ModelAccountModel.Get(ctx, legacy.ID)
	if err != nil {
		t.Fatalf("reload post-migration model: %v", err)
	}
	if !reflect.DeepEqual(legacy.SupportedRatios, legacyRatios) || legacy.MaxReferenceImageCount != 4 {
		t.Fatalf("post-migration explicit capabilities must survive restart: %#v", legacy)
	}
}
