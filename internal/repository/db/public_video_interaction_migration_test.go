package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/schema"
	"github.com/fatballfish/pic-gallery/internal/config"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	entmigrate "github.com/fatballfish/pic-gallery/internal/repository/ent/migrate"
	"github.com/google/uuid"
)

// Compare every persisted/exported field while excluding Ent's internal driver
// and query configuration, which differ between create and query builders.
func publicVideoMigrationEqual(t *testing.T, before, after any) bool {
	t.Helper()
	left, err := json.Marshal(before)
	if err != nil {
		t.Fatal(err)
	}
	right, err := json.Marshal(after)
	if err != nil {
		t.Fatal(err)
	}
	return string(left) == string(right)
}

// Start with the actual v10 table shape, rather than migrating to v11 and
// deleting its table. CopyTables keeps generated schema globals untouched.
func publicVideoInteractionV10Tables(t *testing.T) []*schema.Table {
	t.Helper()
	tables, err := schema.CopyTables(entmigrate.Tables)
	if err != nil {
		t.Fatal(err)
	}
	var legacy []*schema.Table
	removed := false
	for _, table := range tables {
		if table.Name == "public_video_interactions" {
			removed = true
			continue
		}
		legacy = append(legacy, table)
	}
	if !removed {
		t.Fatal("generated schema is missing public_video_interactions")
	}
	return legacy
}

func TestPublicVideoInteractionSchemaV11OfficialPostgresMigration(t *testing.T) {
	database, databaseURL := openLegacyMigrationPostgres(t)
	var version string
	if err := database.QueryRowContext(t.Context(), "SHOW server_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	t.Logf("isolated PostgreSQL server version: %s", version)
	driver := entsql.OpenDB(dialect.Postgres, database)
	client := repoent.NewClient(repoent.Driver(driver))
	testPublicVideoInteractionV11Migration(t, driver, client, func(ctx context.Context, request MigrationRequest) (MigrationResult, error) {
		return Migrate(ctx, databaseURL, request)
	})
}

// SQLite covers the Ent DDL and compatibility contract on ordinary test runs.
// Only the PostgreSQL test above exercises the public Migrate lifecycle/lock.
func TestPublicVideoInteractionSchemaV11SQLiteMigration(t *testing.T) {
	database, err := sql.Open("sqlite3", "file:public-video-interaction-v10?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	driver := entsql.OpenDB(dialect.SQLite, database)
	client := repoent.NewClient(repoent.Driver(driver))
	testPublicVideoInteractionV11Migration(t, driver, client, func(ctx context.Context, request MigrationRequest) (MigrationResult, error) {
		if err := client.Schema.Create(ctx); err != nil {
			return MigrationResult{}, err
		}
		return recordInstallationMigration(ctx, client, request)
	})
}

func testPublicVideoInteractionV11Migration(t *testing.T, driver *entsql.Driver, client *repoent.Client, migrate func(context.Context, MigrationRequest) (MigrationResult, error)) {
	t.Helper()
	ctx := t.Context()
	if CurrentDatabaseSchemaVersion != 11 {
		t.Fatalf("v10 -> v11 fixture requires schema 11, got %d", CurrentDatabaseSchemaVersion)
	}
	migration, err := schema.NewMigrate(driver)
	if err != nil {
		t.Fatal(err)
	}
	if err := migration.Create(ctx, publicVideoInteractionV10Tables(t)...); err != nil {
		t.Fatalf("create v10 schema: %v", err)
	}
	request := MigrationRequest{InstallationID: "public-video-v11", AppVersion: "v0.0.39", ConfigVersion: config.CurrentRuntimeSchemaVersion}
	initialized := time.Date(2026, 10, 20, 1, 2, 3, 0, time.UTC)
	installation, err := client.Installation.Create().
		SetSingletonKey(installationSingletonKey).SetInstallationID(request.InstallationID).
		SetAppVersion(request.AppVersion).SetConfigSchemaVersion(request.ConfigVersion).
		SetDatabaseSchemaVersion(10).SetInitializedAt(initialized).SetMigratedAt(initialized).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	user, err := client.User.Create().SetEmail("video-migration@example.test").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	project, err := client.Project.Create().SetUserID(int64(user.ID)).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	route, err := client.RouteModel.Create().SetCode("video-migration-legacy").SetName("Existing route").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	videoID := uuid.MustParse("00000000-0000-0000-0000-000000000011")
	imageInteraction, err := client.PublicImageInteraction.Create().SetImageID(videoID).SetUserID(int64(user.ID)).SetLiked(true).SetFavorited(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Reload baselines to compare stored values (drivers normalize timestamps
	// and omit the monotonic clock carried by freshly created entities).
	installation, err = client.Installation.Get(ctx, installation.ID)
	if err != nil {
		t.Fatal(err)
	}
	user, err = client.User.Get(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err = client.Project.Get(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	route, err = client.RouteModel.Get(ctx, route.ID)
	if err != nil {
		t.Fatal(err)
	}
	imageInteraction, err = client.PublicImageInteraction.Get(ctx, imageInteraction.ID)
	if err != nil {
		t.Fatal(err)
	}
	oldNode := SchemaVersion{InstallationID: request.InstallationID, AppVersion: request.AppVersion, ConfigVersion: request.ConfigVersion, DatabaseSchemaVersion: 10}
	newNode := oldNode
	newNode.DatabaseSchemaVersion = 11
	if err := CheckSchemaCompatibility(ctx, client, oldNode); err != nil {
		t.Fatalf("v10 node must accept original fixture: %v", err)
	}
	assertCompatibilityErrorKind(t, CheckSchemaCompatibility(ctx, client, newNode), CompatibilityDatabaseSchemaMismatch)
	if _, err := client.PublicVideoInteraction.Query().Count(ctx); err == nil {
		t.Fatal("v10 fixture unexpectedly has the new interaction table")
	}
	unchanged, err := client.Installation.Get(ctx, installation.ID)
	if err != nil || !publicVideoMigrationEqual(t, installation, unchanged) {
		t.Fatalf("compatibility checks changed installation: %+v, %v", unchanged, err)
	}

	result, err := migrate(ctx, request)
	if err != nil {
		t.Fatalf("migrate v10 -> v11: %v", err)
	}
	if result.Previous == nil || *result.Previous != oldNode || result.Current != newNode || !result.Changed || result.BackfilledRows != 0 {
		t.Fatalf("unexpected migration result: %+v", result)
	}
	if err := CheckSchemaCompatibility(ctx, client, newNode); err != nil {
		t.Fatalf("v11 node rejected migrated schema: %v", err)
	}
	// Keep app/config/installation identical: only the schema gate can reject
	// an old node after the upgrade, including a rolling-deployment restart.
	assertCompatibilityErrorKind(t, CheckSchemaCompatibility(ctx, client, oldNode), CompatibilityDatabaseSchemaMismatch)
	migrated, err := client.Installation.Get(ctx, installation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if migrated.ID != installation.ID || migrated.SingletonKey != installation.SingletonKey || migrated.InstallationID != installation.InstallationID || !migrated.InitializedAt.Equal(installation.InitializedAt) || !migrated.CreatedAt.Equal(installation.CreatedAt) || migrated.AppVersion != installation.AppVersion || migrated.ConfigSchemaVersion != installation.ConfigSchemaVersion || migrated.DatabaseSchemaVersion != 11 || !migrated.MigratedAt.After(initialized) || migrated.MigratedAt.Sub(result.MigratedAt).Abs() > time.Microsecond {
		t.Fatalf("installation identity/version changed unexpectedly: before=%+v after=%+v", installation, migrated)
	}
	if count, err := client.PublicVideoInteraction.Query().Count(ctx); err != nil || count != 0 {
		t.Fatalf("new table must initially be empty: count=%d err=%v", count, err)
	}
	interaction, err := client.PublicVideoInteraction.Create().SetVideoID(videoID).SetUserID(int64(user.ID)).Save(ctx)
	if err != nil || interaction.Liked || interaction.Favorited {
		t.Fatalf("new table/defaults: interaction=%+v err=%v", interaction, err)
	}
	if _, err := client.PublicVideoInteraction.Create().SetVideoID(videoID).SetUserID(int64(user.ID)).Save(ctx); !repoent.IsConstraintError(err) {
		t.Fatalf("duplicate video/user must violate unique constraint: %v", err)
	}
	if _, err := client.PublicVideoInteraction.Create().SetVideoID(videoID).SetUserID(int64(user.ID) + 1).Save(ctx); err != nil {
		t.Fatalf("same video with another user must be allowed: %v", err)
	}
	if _, err := client.PublicVideoInteraction.Create().SetVideoID(uuid.New()).SetUserID(int64(user.ID)).Save(ctx); err != nil {
		t.Fatalf("same user with another video must be allowed: %v", err)
	}
	interaction, err = client.PublicVideoInteraction.UpdateOne(interaction).SetLiked(true).SetFavorited(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	interaction, err = client.PublicVideoInteraction.Get(ctx, interaction.ID)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := migrate(ctx, request)
	if err != nil || repeated.Changed || repeated.Previous == nil || *repeated.Previous != newNode || repeated.Current != newNode || !repeated.MigratedAt.Equal(migrated.MigratedAt) || repeated.BackfilledRows != 0 {
		t.Fatalf("repeat migration must be idempotent: %+v, %v", repeated, err)
	}
	if got, err := client.Installation.Get(ctx, installation.ID); err != nil || !publicVideoMigrationEqual(t, migrated, got) {
		t.Fatalf("repeat migration changed installation: %+v, %v", got, err)
	}
	if got, err := client.User.Get(ctx, user.ID); err != nil || !publicVideoMigrationEqual(t, user, got) {
		t.Fatalf("migration changed legacy user: %+v, %v", got, err)
	}
	if got, err := client.Project.Get(ctx, project.ID); err != nil || !publicVideoMigrationEqual(t, project, got) {
		t.Fatalf("migration changed legacy project: %+v, %v", got, err)
	}
	if got, err := client.RouteModel.Get(ctx, route.ID); err != nil || !publicVideoMigrationEqual(t, route, got) {
		t.Fatalf("migration changed legacy route: %+v, %v", got, err)
	}
	if got, err := client.PublicImageInteraction.Get(ctx, imageInteraction.ID); err != nil || !publicVideoMigrationEqual(t, imageInteraction, got) {
		t.Fatalf("migration/video write changed same-UUID image interaction: %+v, %v", got, err)
	}
	if got, err := client.PublicVideoInteraction.Get(ctx, interaction.ID); err != nil || !publicVideoMigrationEqual(t, interaction, got) {
		t.Fatalf("repeat migration changed video interaction: %+v, %v", got, err)
	}
	if count, err := client.PublicVideoInteraction.Query().Count(ctx); err != nil || count != 3 {
		t.Fatalf("repeat migration changed interaction count: %d, %v", count, err)
	}
}
