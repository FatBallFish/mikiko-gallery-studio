package db

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/migrationcheckpoint"
)

func TestBackfillMediaAssetsUsesStableCheckpointAndPreservesObjectIdentity(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:media-asset-backfill-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	const userID int64 = 401
	project, err := client.Project.Create().SetUserID(userID).SetName("Default").SetNameKey("default").SetIsDefault(true).SetStatus("active").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	task, err := client.ImageTask.Create().SetUserID(userID).SetProjectID(project.ID).SetTaskType("text_to_image").SetPrompt("backfill").SetAbstractModel("basic").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ids := []uuid.UUID{uuid.New(), uuid.New()}
	keys := []string{"generated-images/first.png", "generated-images/second.png"}
	for index := range ids {
		if _, err := client.ImageResult.Create().SetID(ids[index]).SetTaskID(task.ID).SetUserID(userID).SetProjectID(project.ID).
			SetStorageDriver("local").SetObjectKey(keys[index]).SetMimeType("image/png").SetFileSizeBytes(int64(100 + index)).
			SetWidth(640).SetHeight(480).SetSha256(strings.Repeat(string(rune('a'+index)), 64)).Save(ctx); err != nil {
			t.Fatal(err)
		}
	}

	first, err := BackfillMediaAssets(ctx, client, MediaAssetBackfillOptions{BatchSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if first.Processed != 1 || first.Created != 1 || first.Done || first.Checkpoint.ID == uuid.Nil {
		t.Fatalf("first batch=%+v", first)
	}
	second, err := BackfillMediaAssets(ctx, client, MediaAssetBackfillOptions{BatchSize: 1, Checkpoint: first.Checkpoint})
	if err != nil {
		t.Fatal(err)
	}
	if second.Processed != 1 || second.Created != 1 {
		t.Fatalf("second batch=%+v", second)
	}
	final, err := BackfillMediaAssets(ctx, client, MediaAssetBackfillOptions{BatchSize: 1, Checkpoint: second.Checkpoint})
	if err != nil {
		t.Fatal(err)
	}
	if !final.Done || final.Processed != 0 {
		t.Fatalf("final batch=%+v", final)
	}

	for index, id := range ids {
		asset, err := client.MediaAsset.Query().Where(mediaasset.IDEQ(id)).Only(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if asset.LegacyImageResultID == nil || *asset.LegacyImageResultID != id || asset.ObjectKey != keys[index] || asset.ProjectID != project.ID {
			t.Fatalf("asset %s changed identity: %+v", id, asset)
		}
	}

	replayed, err := BackfillMediaAssets(ctx, client, MediaAssetBackfillOptions{BatchSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Processed != 2 || replayed.Created != 0 || replayed.Skipped != 2 {
		t.Fatalf("replayed batch=%+v", replayed)
	}
}

func TestMediaAssetBackfillCursorAdvancesDatabaseSchemaVersion(t *testing.T) {
	if CurrentDatabaseSchemaVersion < 5 {
		t.Fatalf("database schema version = %d, want at least 5 for durable media asset cursor", CurrentDatabaseSchemaVersion)
	}
}

func TestBackfillMediaAssetsDryRunDoesNotWriteAndReportsPlan(t *testing.T) {
	ctx, client, project, _, images := seedMediaAssetBackfill(t, "dry-run", 2)
	result, err := BackfillMediaAssets(ctx, client, MediaAssetBackfillOptions{BatchSize: 10, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 2 || result.Created != 0 || result.WouldCreate != 2 || result.Skipped != 0 || !result.Done || result.Checkpoint.ID != images[1].ID {
		t.Fatalf("dry run = %+v", result)
	}
	if count, err := client.MediaAsset.Query().Count(ctx); err != nil || count != 0 {
		t.Fatalf("dry run assets = %d, %v", count, err)
	}
	if project.ID == uuid.Nil {
		t.Fatal("seeded project is unavailable")
	}
}

func TestBackfillMediaAssetsUsesDatabaseConflictIgnore(t *testing.T) {
	ctx, client, project, _, images := seedMediaAssetBackfill(t, "conflict-ignore", 2)
	first := images[0]
	if _, err := client.MediaAsset.Create().SetID(first.ID).SetUserID(first.UserID).SetProjectID(project.ID).SetLegacyImageResultID(first.ID).
		SetName("existing.png").SetNameKey("existing.png").SetMediaType("image").SetSourceType("generated").SetStatus("ready").
		SetStorageDriver(first.StorageDriver).SetObjectKey(first.ObjectKey).SetMimeType(first.MimeType).SetFileSizeBytes(first.FileSizeBytes).Save(ctx); err != nil {
		t.Fatal(err)
	}
	result, err := BackfillMediaAssets(ctx, client, MediaAssetBackfillOptions{BatchSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 2 || result.Created != 1 || result.Skipped != 1 {
		t.Fatalf("conflict result = %+v", result)
	}
	if count, err := client.MediaAsset.Query().Count(ctx); err != nil || count != 2 {
		t.Fatalf("asset count = %d, %v", count, err)
	}
}

func TestVerifyMediaAssetBackfillAggregatesBytesAndDeterministicSamples(t *testing.T) {
	ctx, client, _, _, images := seedMediaAssetBackfill(t, "verify", 3)
	if _, err := BackfillMediaAssets(ctx, client, MediaAssetBackfillOptions{BatchSize: 10}); err != nil {
		t.Fatal(err)
	}
	report, err := VerifyMediaAssetBackfill(ctx, client, MediaAssetBackfillVerifyOptions{SampleSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Valid || report.SourceCount != 3 || report.AssetCount != 3 || report.SourceBytes != 303 || report.AssetBytes != 303 || len(report.Aggregates) != 1 || len(report.Samples) != 2 {
		t.Fatalf("verification report = %+v", report)
	}
	for index, sample := range report.Samples {
		if !sample.Valid || sample.ImageResultID != images[index].ID || sample.UserID != images[index].UserID || sample.ObjectKey != images[index].ObjectKey {
			t.Fatalf("sample %d = %+v", index, sample)
		}
	}
	if report.Aggregates[0].SourceCount != 3 || report.Aggregates[0].AssetCount != 3 || report.Aggregates[0].SourceBytes != 303 || report.Aggregates[0].AssetBytes != 303 {
		t.Fatalf("aggregate = %+v", report.Aggregates[0])
	}

	if _, err := client.MediaAsset.UpdateOneID(images[0].ID).SetObjectKey("generated-images/mismatch.png").Save(ctx); err != nil {
		t.Fatal(err)
	}
	report, err = VerifyMediaAssetBackfill(ctx, client, MediaAssetBackfillVerifyOptions{SampleSize: 3})
	if err != nil {
		t.Fatal(err)
	}
	if report.Valid || report.MismatchedSamples != 1 || report.Samples[0].Valid {
		t.Fatalf("mismatch report = %+v", report)
	}
}

func TestMediaAssetBackfillProcessorPersistsCheckpointAndResumes(t *testing.T) {
	ctx, client, _, _, images := seedMediaAssetBackfill(t, "processor-resume", 3)
	processor := NewMediaAssetBackfillProcessor(client, MediaAssetBackfillProcessorOptions{BatchSize: 1})
	processed, err := processor.ProcessOnce(ctx)
	if err != nil || !processed {
		t.Fatalf("first ProcessOnce() = %v, %v", processed, err)
	}
	checkpoint, err := client.MigrationCheckpoint.Query().Where(migrationcheckpoint.NameEQ(mediaAssetBackfillMigrationName)).Only(ctx)
	if err != nil || checkpoint.AfterResultID == nil || *checkpoint.AfterResultID != images[0].ID || checkpoint.ProcessedRows != 1 || checkpoint.Completed {
		t.Fatalf("first checkpoint = %#v, %v", checkpoint, err)
	}

	resumed := NewMediaAssetBackfillProcessor(client, MediaAssetBackfillProcessorOptions{BatchSize: 1})
	for range 2 {
		processed, err = resumed.ProcessOnce(ctx)
		if err != nil || !processed {
			t.Fatalf("resumed ProcessOnce() = %v, %v", processed, err)
		}
	}
	processed, err = resumed.ProcessOnce(ctx)
	if err != nil || processed {
		t.Fatalf("completion ProcessOnce() = %v, %v", processed, err)
	}
	checkpoint, err = client.MigrationCheckpoint.Query().Where(migrationcheckpoint.NameEQ(mediaAssetBackfillMigrationName)).Only(ctx)
	if err != nil || !checkpoint.Completed || checkpoint.Phase != mediaAssetBackfillPhaseDone || checkpoint.ProcessedRows != 3 {
		t.Fatalf("completed checkpoint = %#v, %v", checkpoint, err)
	}
	if count, err := client.MediaAsset.Query().Count(ctx); err != nil || count != 3 {
		t.Fatalf("asset count = %d, %v", count, err)
	}
}

func TestMediaAssetBackfillProcessorResumesWhenCheckpointSourceWasDeleted(t *testing.T) {
	ctx, client, _, _, images := seedMediaAssetBackfill(t, "deleted-checkpoint-source", 3)
	processor := NewMediaAssetBackfillProcessor(client, MediaAssetBackfillProcessorOptions{BatchSize: 1})
	if result, err := processor.ProcessBatch(ctx); err != nil || result.Processed != 1 {
		t.Fatalf("first batch = %+v, %v", result, err)
	}
	if err := client.ImageResult.DeleteOneID(images[0].ID).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	for attempts := 0; attempts < 5; attempts++ {
		result, err := processor.ProcessBatch(ctx)
		if err != nil {
			t.Fatalf("resume batch %d: %v", attempts, err)
		}
		if result.Done {
			break
		}
	}
	if count, err := client.MediaAsset.Query().Count(ctx); err != nil || count != 3 {
		t.Fatalf("asset count = %d, %v", count, err)
	}
}

func TestMediaAssetBackfillProcessorReopensCompletedCheckpointForLateSource(t *testing.T) {
	ctx, client, project, task, _ := seedMediaAssetBackfill(t, "late-source", 1)
	processor := NewMediaAssetBackfillProcessor(client, MediaAssetBackfillProcessorOptions{BatchSize: 10})
	if result, err := processor.ProcessBatch(ctx); err != nil || !result.Done {
		t.Fatalf("complete first pass = %+v, %v", result, err)
	}
	late, err := client.ImageResult.Create().SetTaskID(task.ID).SetUserID(task.UserID).SetProjectID(project.ID).
		SetObjectKey("generated-images/late.png").SetMimeType("image/png").SetSha256(strings.Repeat("e", 64)).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for attempts := 0; attempts < 3; attempts++ {
		if _, err := processor.ProcessBatch(ctx); err != nil {
			t.Fatalf("late batch %d: %v", attempts, err)
		}
	}
	if exists, err := client.MediaAsset.Query().Where(mediaasset.IDEQ(late.ID), mediaasset.DeletedAtIsNil()).Exist(ctx); err != nil || !exists {
		t.Fatalf("late asset exists = %t, %v", exists, err)
	}
}

func TestBackfillMediaAssetsRejectsConflictingAssetIdentity(t *testing.T) {
	ctx, client, project, _, images := seedMediaAssetBackfill(t, "conflicting-id", 1)
	image := images[0]
	if _, err := client.MediaAsset.Create().SetID(image.ID).SetUserID(image.UserID).SetProjectID(project.ID).
		SetName("wrong.png").SetNameKey("wrong.png").SetMediaType("image").SetSourceType("uploaded").SetStatus("ready").
		SetStorageDriver("local").SetObjectKey("wrong/object.png").SetMimeType("image/png").SetFileSizeBytes(0).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := BackfillMediaAssets(ctx, client, MediaAssetBackfillOptions{BatchSize: 10}); err == nil || !strings.Contains(err.Error(), "conflicts with image result") {
		t.Fatalf("conflict error = %v", err)
	}
}

func TestVerifyMediaAssetBackfillExcludesSoftDeletedTargets(t *testing.T) {
	ctx, client, _, _, images := seedMediaAssetBackfill(t, "soft-deleted-target", 1)
	if _, err := BackfillMediaAssets(ctx, client, MediaAssetBackfillOptions{BatchSize: 10}); err != nil {
		t.Fatal(err)
	}
	deletedAt := time.Now().UTC()
	if _, err := client.MediaAsset.UpdateOneID(images[0].ID).SetDeletedAt(deletedAt).Save(ctx); err != nil {
		t.Fatal(err)
	}
	report, err := VerifyMediaAssetBackfill(ctx, client, MediaAssetBackfillVerifyOptions{SampleSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if report.Valid || report.AssetCount != 0 {
		t.Fatalf("verification report = %+v", report)
	}
}

func seedMediaAssetBackfill(t *testing.T, name string, count int) (context.Context, *repoent.Client, *repoent.Project, *repoent.ImageTask, []*repoent.ImageResult) {
	t.Helper()
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:media-asset-backfill-"+name+"-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	const userID int64 = 499
	project, err := client.Project.Create().SetUserID(userID).SetName("Default").SetNameKey("default").SetIsDefault(true).SetStatus("active").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	task, err := client.ImageTask.Create().SetUserID(userID).SetProjectID(project.ID).SetTaskType("text_to_image").SetPrompt("backfill").SetAbstractModel("basic").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	images := make([]*repoent.ImageResult, 0, count)
	baseCreatedAt := time.Date(2026, 8, 12, 22, 0, 0, 0, time.UTC)
	for index := range count {
		image, err := client.ImageResult.Create().SetTaskID(task.ID).SetUserID(userID).SetProjectID(project.ID).SetStorageDriver("local").
			SetObjectKey(fmt.Sprintf("generated-images/%02d.png", index)).SetMimeType("image/png").SetFileSizeBytes(int64(100 + index)).
			SetWidth(640).SetHeight(480).SetSha256(strings.Repeat("d", 64)).SetCreatedAt(baseCreatedAt.Add(time.Duration(index) * time.Second)).Save(ctx)
		if err != nil {
			t.Fatal(err)
		}
		images = append(images, image)
	}
	return ctx, client, project, task, images
}

func TestBackfillMediaAssetsUsesUsersDefaultProjectForLegacyRows(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:media-asset-backfill-default-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	const userID int64 = 402
	project, err := client.Project.Create().SetUserID(userID).SetName("Default").SetNameKey("default").SetIsDefault(true).SetStatus("active").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	task, err := client.ImageTask.Create().SetUserID(userID).SetTaskType("text_to_image").SetPrompt("legacy").SetAbstractModel("basic").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	image, err := client.ImageResult.Create().SetTaskID(task.ID).SetUserID(userID).SetObjectKey("generated-images/legacy.png").SetMimeType("image/png").SetSha256(strings.Repeat("c", 64)).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BackfillMediaAssets(ctx, client, MediaAssetBackfillOptions{BatchSize: 10}); err != nil {
		t.Fatal(err)
	}
	asset, err := client.MediaAsset.Get(ctx, image.ID)
	if err != nil {
		t.Fatal(err)
	}
	if asset.ProjectID != project.ID {
		t.Fatalf("project=%s want %s", asset.ProjectID, project.ID)
	}
}
