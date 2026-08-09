package db

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	domaincleanup "github.com/fatballfish/pic-gallery/internal/domain/objectcleanup"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imageresult"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imagetask"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/objectdeletionjob"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/referenceasset"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

var errForcedStorageIdentityBackfillStop = errors.New("forced storage identity backfill stop")

func TestLegacyStorageIdentityBackfillProtectsLiveObjectsAndUnifiesCleanupJobs(t *testing.T) {
	client := openStorageIdentityBackfillSQLite(t, "protect-live")
	ctx := t.Context()
	configID := uuid.New()
	deletedAt := time.Now().UTC().Add(-time.Hour)

	liveTask := seedStorageIdentityTask(t, ctx, client, "live")
	if _, err := client.ImageResult.Create().
		SetTaskID(liveTask.ID).SetUserID(liveTask.UserID).
		SetStorageDriver("local").SetObjectKey("generated-images/live.png").
		SetMimeType("image/png").SetSha256("live").Save(ctx); err != nil {
		t.Fatal(err)
	}
	deletedTask := seedStorageIdentityTask(t, ctx, client, "deleted")
	if _, err := client.ImageResult.Create().
		SetTaskID(deletedTask.ID).SetUserID(deletedTask.UserID).
		SetStorageDriver("local").SetObjectKey("generated-images/deleted.png").
		SetMimeType("image/png").SetSha256("deleted").SetDeletedAt(deletedAt).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ObjectDeletionJob.Create().
		SetStorageDriver("local").SetObjectKey("generated-images/deleted.png").
		SetState(domaincleanup.StatePending).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ObjectDeletionJob.Create().
		SetStorageConfigID(configID).SetStorageDriver("renamed-driver").
		SetObjectKey("generated-images/deleted.png").SetState(domaincleanup.StateBlocked).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ObjectDeletionJob.Create().
		SetStorageConfigID(configID).SetStorageDriver("renamed-2").
		SetObjectKey("generated-images/deleted.png").SetState(domaincleanup.StateBlocked).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ObjectDeletionJob.Create().
		SetStorageDriver("local").SetObjectKey("generated-images/historical.png").
		SetState(domaincleanup.StateDone).SetCompletedAt(deletedAt).Save(ctx); err != nil {
		t.Fatal(err)
	}

	progress, err := RunLegacyStorageIdentityBackfill(ctx, client, "local", configID, LegacyStorageIdentityBackfillOptions{BatchSize: 1, MaxBatches: 100})
	if err != nil || !progress.Completed {
		t.Fatalf("backfill = %#v, %v", progress, err)
	}

	liveResult, err := client.ImageResult.Query().Where(imageresult.ObjectKeyEQ("generated-images/live.png")).Only(ctx)
	if err != nil || liveResult.StorageConfigID == nil || *liveResult.StorageConfigID != configID {
		t.Fatalf("configured live result = %#v, %v", liveResult, err)
	}
	ordinaryTask, err := client.ImageTask.Get(ctx, liveTask.ID)
	if err != nil || ordinaryTask.ArtifactStorageConfigID != nil {
		t.Fatalf("ordinary task must not gain artifact storage identity = %#v, %v", ordinaryTask, err)
	}

	liveJobs, err := client.ObjectDeletionJob.Query().Where(
		objectdeletionjob.StorageConfigIDEQ(configID),
		objectdeletionjob.ObjectKeyEQ("generated-images/deleted.png"),
		objectdeletionjob.StateIn(domaincleanup.StatePending, domaincleanup.StateRunning, domaincleanup.StateRetry, domaincleanup.StateBlocked),
	).All(ctx)
	if err != nil || len(liveJobs) != 1 || liveJobs[0].State != domaincleanup.StatePending {
		t.Fatalf("merged configured jobs = %#v, %v", liveJobs, err)
	}
	if count, err := client.ObjectDeletionJob.Query().Where(
		objectdeletionjob.StorageConfigIDIsNil(),
		objectdeletionjob.StateIn(domaincleanup.StatePending, domaincleanup.StateRunning, domaincleanup.StateRetry, domaincleanup.StateBlocked),
	).Count(ctx); err != nil || count != 0 {
		t.Fatalf("legacy live jobs = %d, %v", count, err)
	}
	if count, err := client.ObjectDeletionJob.Query().Where(
		objectdeletionjob.StorageConfigIDIsNil(), objectdeletionjob.StateEQ(domaincleanup.StateDone),
	).Count(ctx); err != nil || count != 1 {
		t.Fatalf("legacy historical jobs = %d, %v", count, err)
	}
}

func TestLegacyStorageIdentityBackfillAssignsDeletedRowsConfiguredIdentity(t *testing.T) {
	client := openStorageIdentityBackfillSQLite(t, "configured-sweep")
	ctx := t.Context()
	configID := uuid.New()
	deletedAt := time.Now().UTC().Add(-time.Hour)
	task := seedStorageIdentityTask(t, ctx, client, "deleted-sweep")
	if _, err := client.ImageResult.Create().
		SetTaskID(task.ID).SetUserID(task.UserID).SetStorageDriver("local").
		SetObjectKey("generated-images/sweep.png").SetMimeType("image/png").SetSha256("sweep").
		SetDeletedAt(deletedAt).Save(ctx); err != nil {
		t.Fatal(err)
	}

	if progress, err := RunLegacyStorageIdentityBackfill(ctx, client, "local", configID, LegacyStorageIdentityBackfillOptions{}); err != nil || !progress.Completed {
		t.Fatalf("backfill = %#v, %v", progress, err)
	}
	result, err := client.ImageResult.Query().Where(imageresult.ObjectKeyEQ("generated-images/sweep.png")).Only(ctx)
	if err != nil || result.StorageConfigID == nil || *result.StorageConfigID != configID {
		t.Fatalf("configured deleted result = %#v, %v", result, err)
	}
}

func TestLegacyStorageIdentityBackfillResumesAfterInterruptionAcrossAllTables(t *testing.T) {
	client := openStorageIdentityBackfillSQLite(t, "resume")
	ctx := t.Context()
	configID := uuid.New()
	task := seedStorageIdentityTask(t, ctx, client, "recovery")
	if _, err := task.Update().SetArtifactRecoveryStatus("pending").SetArtifactStorageDriver("local").SetArtifactObjectKeys([]string{"generated-images/recovery.png"}).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ImageResult.Create().SetTaskID(task.ID).SetUserID(task.UserID).SetStorageDriver("local").SetObjectKey("generated-images/result.png").SetMimeType("image/png").SetSha256("result").Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReferenceAsset.Create().SetUserID(task.UserID).SetStatus("ready").SetStorageDriver("local").SetObjectKey("reference-assets/input.png").SetMimeType("image/png").SetSha256("asset").Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ObjectDeletionJob.Create().SetStorageDriver("local").SetObjectKey("generated-images/job.png").SetState(domaincleanup.StateRetry).Save(ctx); err != nil {
		t.Fatal(err)
	}

	first, err := RunLegacyStorageIdentityBackfill(ctx, client, "local", configID, LegacyStorageIdentityBackfillOptions{
		BatchSize: 1, MaxBatches: 1,
		afterBatch: func(LegacyStorageIdentityBackfillProgress) error { return errForcedStorageIdentityBackfillStop },
	})
	if !errors.Is(err, errForcedStorageIdentityBackfillStop) || first.Completed {
		t.Fatalf("forced first backfill = %#v, %v", first, err)
	}

	var resumed LegacyStorageIdentityBackfillProgress
	for invocation := 0; invocation < 20 && !resumed.Completed; invocation++ {
		resumed, err = RunLegacyStorageIdentityBackfill(ctx, client, "local", configID, LegacyStorageIdentityBackfillOptions{BatchSize: 1, MaxBatches: 1})
		if err != nil {
			t.Fatalf("resume %d: %v", invocation, err)
		}
	}
	if !resumed.Completed {
		t.Fatalf("backfill did not complete: %#v", resumed)
	}
	if count, _ := client.ImageResult.Query().Where(imageresult.StorageConfigIDIsNil(), imageresult.StorageDriverEQ("local")).Count(ctx); count != 0 {
		t.Fatalf("remaining result rows = %d", count)
	}
	if count, _ := client.ReferenceAsset.Query().Where(referenceasset.StorageConfigIDIsNil(), referenceasset.StorageDriverEQ("local")).Count(ctx); count != 0 {
		t.Fatalf("remaining asset rows = %d", count)
	}
	if count, _ := client.ImageTask.Query().Where(imagetask.ArtifactStorageConfigIDIsNil(), imagetask.ArtifactStorageDriverEQ("local")).Count(ctx); count != 0 {
		t.Fatalf("remaining recovery rows = %d", count)
	}
	if count, _ := client.ObjectDeletionJob.Query().Where(
		objectdeletionjob.StorageConfigIDIsNil(), objectdeletionjob.StorageDriverEQ("local"),
		objectdeletionjob.StateIn(domaincleanup.StatePending, domaincleanup.StateRunning, domaincleanup.StateRetry, domaincleanup.StateBlocked),
	).Count(ctx); count != 0 {
		t.Fatalf("remaining live cleanup jobs = %d", count)
	}
}

func TestLegacyStorageIdentityBackfillKeepsSameKeyOnOtherConfigIsolated(t *testing.T) {
	client := openStorageIdentityBackfillSQLite(t, "isolation")
	ctx := t.Context()
	bootstrapConfigID := uuid.New()
	oldConfigID := uuid.New()
	objectKey := "generated-images/shared.png"
	task := seedStorageIdentityTask(t, ctx, client, "old-config")
	if _, err := client.ImageResult.Create().
		SetTaskID(task.ID).SetUserID(task.UserID).SetStorageConfigID(oldConfigID).SetStorageDriver("local").
		SetObjectKey(objectKey).SetMimeType("image/png").SetSha256("old").Save(ctx); err != nil {
		t.Fatal(err)
	}
	deletedAt := time.Now().UTC().Add(-time.Hour)
	if _, err := client.ReferenceAsset.Create().
		SetUserID(task.UserID).SetStatus("deleted").SetStorageDriver("local").SetObjectKey(objectKey).
		SetMimeType("image/png").SetSha256("legacy").SetDeletedAt(deletedAt).Save(ctx); err != nil {
		t.Fatal(err)
	}

	if progress, err := RunLegacyStorageIdentityBackfill(ctx, client, "local", bootstrapConfigID, LegacyStorageIdentityBackfillOptions{}); err != nil || !progress.Completed {
		t.Fatalf("backfill = %#v, %v", progress, err)
	}
	oldResult, err := client.ImageResult.Query().Where(imageresult.ObjectKeyEQ(objectKey)).Only(ctx)
	if err != nil || oldResult.StorageConfigID == nil || *oldResult.StorageConfigID != oldConfigID {
		t.Fatalf("old configured result = %#v, %v", oldResult, err)
	}
	legacyAsset, err := client.ReferenceAsset.Query().Where(referenceasset.ObjectKeyEQ(objectKey)).Only(ctx)
	if err != nil || legacyAsset.StorageConfigID == nil || *legacyAsset.StorageConfigID != bootstrapConfigID {
		t.Fatalf("normalized bootstrap asset = %#v, %v", legacyAsset, err)
	}
}

func openStorageIdentityBackfillSQLite(t *testing.T, name string) *repoent.Client {
	t.Helper()
	client, err := repoent.Open(dialect.SQLite, fmt.Sprintf("file:storage-identity-%s-%s?mode=memory&cache=shared&_fk=1", name, uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	return client
}

func seedStorageIdentityTask(t *testing.T, ctx context.Context, client *repoent.Client, name string) *repoent.ImageTask {
	t.Helper()
	task, err := client.ImageTask.Create().
		SetUserID(901).SetTaskType("text_to_image").SetPrompt(name).SetAbstractModel("plus").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return task
}
