package db

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/migrationcheckpoint"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/referenceasset"
	"github.com/google/uuid"
)

var errForcedReferenceAssetNameBackfillStop = errors.New("forced reference asset name backfill stop")

func TestReferenceAssetNameFieldsAdvanceDatabaseSchemaVersion(t *testing.T) {
	if CurrentDatabaseSchemaVersion < 4 {
		t.Fatalf("database schema version = %d, want at least 4 for reference asset names", CurrentDatabaseSchemaVersion)
	}
}

func TestReferenceAssetNameBackfillIsBoundedDeterministicAndRepairsLateRows(t *testing.T) {
	client := openReferenceAssetNameBackfillSQLite(t, "resume")
	ctx := context.Background()
	firstUser := seedReferenceAssetNameBackfillUser(t, ctx, client, "first-reference-name@example.com")
	secondUser := seedReferenceAssetNameBackfillUser(t, ctx, client, "second-reference-name@example.com")
	baseTime := time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)

	seedReferenceAssetNameBackfill(t, ctx, client, firstUser, baseTime.Add(-time.Hour), "existing", "图片2", nil)
	first := seedReferenceAssetNameBackfill(t, ctx, client, firstUser, baseTime, "first", "", nil)
	second := seedReferenceAssetNameBackfill(t, ctx, client, firstUser, baseTime.Add(time.Minute), "second", "", nil)
	third := seedReferenceAssetNameBackfill(t, ctx, client, secondUser, baseTime, "third", "", nil)
	deletedAt := baseTime.Add(time.Hour)
	deleted := seedReferenceAssetNameBackfill(t, ctx, client, firstUser, baseTime.Add(-time.Minute), "deleted", "", &deletedAt)

	progress, err := RunReferenceAssetNameBackfill(ctx, client, ReferenceAssetNameBackfillOptions{
		BatchSize:  1,
		MaxBatches: 1,
		afterBatch: func(ReferenceAssetNameBackfillProgress) error {
			return errForcedReferenceAssetNameBackfillStop
		},
	})
	if !errors.Is(err, errForcedReferenceAssetNameBackfillStop) || progress.UpdatedRows != 1 || progress.Completed {
		t.Fatalf("forced first batch = %#v, %v", progress, err)
	}
	checkpoint, err := client.MigrationCheckpoint.Query().Where(migrationcheckpoint.NameEQ(referenceAssetNameMigrationName)).Only(ctx)
	if err != nil || checkpoint.Completed {
		t.Fatalf("persisted checkpoint = %#v, %v", checkpoint, err)
	}

	for invocation := 0; invocation < 20 && !progress.Completed; invocation++ {
		progress, err = RunReferenceAssetNameBackfill(ctx, client, ReferenceAssetNameBackfillOptions{BatchSize: 1, MaxBatches: 1})
		if err != nil {
			t.Fatalf("resume invocation %d: %v", invocation, err)
		}
	}
	if !progress.Completed || progress.ProcessedRows != 4 {
		t.Fatalf("completed progress = %#v, want four incomplete active historical rows", progress)
	}
	assertReferenceAssetBackfillName(t, ctx, client, first, "图片1")
	assertReferenceAssetBackfillName(t, ctx, client, second, "图片3")
	assertReferenceAssetBackfillName(t, ctx, client, third, "图片1")
	if entity, getErr := client.ReferenceAsset.Get(ctx, deleted); getErr != nil || entity.Name != nil || entity.NameNormalized != nil {
		t.Fatalf("deleted asset was mutated: %#v, %v", entity, getErr)
	}

	late := seedReferenceAssetNameBackfill(t, ctx, client, firstUser, baseTime.Add(2*time.Hour), "late", "", nil)
	reset, err := RunReferenceAssetNameBackfill(ctx, client, ReferenceAssetNameBackfillOptions{BatchSize: 1, MaxBatches: 1})
	if err != nil || reset.Completed || reset.Phase != referenceAssetNameBackfillPhaseAssets {
		t.Fatalf("late-row reset = %#v, %v", reset, err)
	}
	checkpoint, err = client.MigrationCheckpoint.Query().Where(migrationcheckpoint.NameEQ(referenceAssetNameMigrationName)).Only(ctx)
	if err != nil || checkpoint.AfterUserID != 0 || checkpoint.Completed {
		t.Fatalf("reset checkpoint = %#v, %v", checkpoint, err)
	}
	repaired, err := RunReferenceAssetNameBackfill(ctx, client, ReferenceAssetNameBackfillOptions{BatchSize: 1, MaxBatches: 10})
	if err != nil || !repaired.Completed {
		t.Fatalf("repair late asset = %#v, %v", repaired, err)
	}
	assertReferenceAssetBackfillName(t, ctx, client, late, "图片4")
}

func TestReferenceAssetNameBackfillNormalizesUsableLegacyNamesAndResolvesDuplicates(t *testing.T) {
	client := openReferenceAssetNameBackfillSQLite(t, "normalize")
	ctx := context.Background()
	userID := seedReferenceAssetNameBackfillUser(t, ctx, client, "normalize-reference-name@example.com")
	baseTime := time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)
	first := seedReferenceAssetNameBackfill(t, ctx, client, userID, baseTime, "first", "  主体  ", nil)
	second := seedReferenceAssetNameBackfill(t, ctx, client, userID, baseTime.Add(time.Minute), "second", "主体", nil)

	progress, err := RunReferenceAssetNameBackfill(ctx, client, ReferenceAssetNameBackfillOptions{BatchSize: 10, MaxBatches: 10})
	if err != nil || !progress.Completed {
		t.Fatalf("normalize legacy names = %#v, %v", progress, err)
	}
	assertReferenceAssetBackfillName(t, ctx, client, first, "主体")
	assertReferenceAssetBackfillName(t, ctx, client, second, "图片1")
}

func openReferenceAssetNameBackfillSQLite(t *testing.T, name string) *repoent.Client {
	t.Helper()
	client, err := repoent.Open(dialect.SQLite, fmt.Sprintf("file:reference-name-backfill-%s-%s?mode=memory&cache=shared&_fk=1", name, uuid.NewString()))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return client
}

func seedReferenceAssetNameBackfillUser(t *testing.T, ctx context.Context, client *repoent.Client, email string) int64 {
	t.Helper()
	entity, err := client.User.Create().SetEmail(email).SetStatus("active").Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return int64(entity.ID)
}

func seedReferenceAssetNameBackfill(t *testing.T, ctx context.Context, client *repoent.Client, userID int64, createdAt time.Time, key, name string, deletedAt *time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	create := client.ReferenceAsset.Create().
		SetID(id).
		SetUserID(userID).
		SetStatus("ready").
		SetObjectKey("reference/" + key + ".png").
		SetMimeType("image/png").
		SetSha256(key).
		SetCreatedAt(createdAt).
		SetUpdatedAt(createdAt)
	if name != "" {
		create.SetName(name)
	}
	if deletedAt != nil {
		create.SetDeletedAt(*deletedAt)
	}
	if _, err := create.Save(ctx); err != nil {
		t.Fatalf("create reference asset %s: %v", key, err)
	}
	return id
}

func assertReferenceAssetBackfillName(t *testing.T, ctx context.Context, client *repoent.Client, id uuid.UUID, want string) {
	t.Helper()
	entity, err := client.ReferenceAsset.Query().Where(referenceasset.IDEQ(id)).Only(ctx)
	if err != nil || entity.Name == nil || entity.NameNormalized == nil || *entity.Name != want || *entity.NameNormalized != want {
		t.Fatalf("asset %s name = %#v/%#v, %v; want %q", id, entity.Name, entity.NameNormalized, err, want)
	}
}
