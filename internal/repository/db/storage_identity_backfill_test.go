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

func TestLegacyStorageIdentityBackfillFencesJobsBeforeReferences(t *testing.T) {
	client := openStorageIdentityBackfillSQLite(t, "jobs-first")
	ctx := t.Context()
	configID := uuid.New()
	task := seedStorageIdentityTask(t, ctx, client, "jobs-first")
	if _, err := client.ImageResult.Create().
		SetTaskID(task.ID).SetUserID(task.UserID).SetStorageDriver("local").
		SetObjectKey("generated-images/jobs-first.png").SetMimeType("image/png").SetSha256("jobs-first").
		Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ObjectDeletionJob.Create().
		SetStorageDriver("local").SetObjectKey("generated-images/jobs-first.png").
		SetState(domaincleanup.StateRunning).SetAttemptCount(1).Save(ctx); err != nil {
		t.Fatal(err)
	}

	progress, err := RunLegacyStorageIdentityBackfill(ctx, client, "local", configID, LegacyStorageIdentityBackfillOptions{BatchSize: 1, MaxBatches: 1})
	if err != nil {
		t.Fatal(err)
	}
	if progress.Completed {
		t.Fatalf("single batch unexpectedly completed: %#v", progress)
	}
	job, err := client.ObjectDeletionJob.Query().Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if job.StorageConfigID == nil || *job.StorageConfigID != configID || job.State != domaincleanup.StatePending {
		t.Fatalf("cleanup job was not fenced first: %#v", job)
	}
	result, err := client.ImageResult.Query().Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.StorageConfigID != nil {
		t.Fatalf("reference migrated before cleanup job: %#v", result)
	}
}

func TestLegacyStorageIdentityBackfillReopensCompletedCheckpointForLateRows(t *testing.T) {
	client := openStorageIdentityBackfillSQLite(t, "late-row")
	ctx := t.Context()
	configID := uuid.New()
	if progress, err := RunLegacyStorageIdentityBackfill(ctx, client, "local", configID, LegacyStorageIdentityBackfillOptions{}); err != nil || !progress.Completed {
		t.Fatalf("initial backfill = %#v, %v", progress, err)
	}

	task := seedStorageIdentityTask(t, ctx, client, "late-row")
	late, err := client.ImageResult.Create().
		SetTaskID(task.ID).SetUserID(task.UserID).SetStorageDriver("local").
		SetObjectKey("generated-images/late.png").SetMimeType("image/png").SetSha256("late").
		Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	progress, err := RunLegacyStorageIdentityBackfill(ctx, client, "local", configID, LegacyStorageIdentityBackfillOptions{})
	if err != nil || !progress.Completed {
		t.Fatalf("reopened backfill = %#v, %v", progress, err)
	}
	late, err = client.ImageResult.Get(ctx, late.ID)
	if err != nil || late.StorageConfigID == nil || *late.StorageConfigID != configID {
		t.Fatalf("late row storage identity = %#v, %v", late, err)
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

func TestLegacyStorageIdentityBatchRetriesConstraintRace(t *testing.T) {
	client := openStorageIdentityBackfillSQLite(t, "constraint-retry")
	ctx := t.Context()
	if _, err := client.MigrationCheckpoint.Create().SetName("duplicate").SetPhase("test").Save(ctx); err != nil {
		t.Fatal(err)
	}
	_, constraintErr := client.MigrationCheckpoint.Create().SetName("duplicate").SetPhase("test").Save(ctx)
	if !repoent.IsConstraintError(constraintErr) {
		t.Fatalf("seed constraint error = %v", constraintErr)
	}

	attempts := 0
	updated, err := runLegacyStorageIdentityBatchWithRetry(func() (int, error) {
		attempts++
		if attempts == 1 {
			return 0, constraintErr
		}
		return 3, nil
	})
	if err != nil || updated != 3 || attempts != 2 {
		t.Fatalf("retry result updated=%d attempts=%d err=%v", updated, attempts, err)
	}
}

func TestLegacyStorageIdentityBackfillRetriesConcurrentConfiguredEnqueueOnPostgres(t *testing.T) {
	database, databaseURL := openLegacyMigrationPostgres(t)
	ctx := context.Background()
	clientA, err := repoent.Open(dialect.Postgres, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientA.Close() })
	clientB, err := repoent.Open(dialect.Postgres, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientB.Close() })
	if err := clientA.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	configID := uuid.New()
	objectKey := "generated-images/concurrent-enqueue.png"
	if _, err := clientA.ObjectDeletionJob.Create().SetStorageDriver("local").SetObjectKey(objectKey).
		SetState(domaincleanup.StateRunning).SetAttemptCount(1).Save(ctx); err != nil {
		t.Fatal(err)
	}

	const advisoryKey int64 = 73490217
	if _, err := database.ExecContext(ctx, `
		CREATE FUNCTION block_legacy_cleanup_identity_update() RETURNS trigger AS $$
		BEGIN
			IF OLD.storage_config_id IS NULL AND NEW.storage_config_id IS NOT NULL THEN
				PERFORM pg_advisory_lock(73490217);
				PERFORM pg_advisory_unlock(73490217);
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER block_legacy_cleanup_identity_update
		BEFORE UPDATE ON object_deletion_jobs
		FOR EACH ROW EXECUTE FUNCTION block_legacy_cleanup_identity_update();
	`); err != nil {
		t.Fatal(err)
	}
	lockConn, err := database.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lockConn.Close() })
	var holderPID int
	if err := lockConn.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&holderPID); err != nil {
		t.Fatal(err)
	}
	if _, err := lockConn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, advisoryKey); err != nil {
		t.Fatal(err)
	}
	locked := true
	t.Cleanup(func() {
		if locked {
			_, _ = lockConn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, advisoryKey)
		}
	})

	done := make(chan error, 1)
	go func() {
		_, err := RunLegacyStorageIdentityBackfill(ctx, clientA, "local", configID, LegacyStorageIdentityBackfillOptions{BatchSize: 1, MaxBatches: 20})
		done <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting bool
		if err := database.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_stat_activity
				WHERE datname = current_database() AND pid <> $1
				  AND wait_event_type = 'Lock' AND wait_event = 'advisory'
			)
		`, holderPID).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("backfill did not reach the guarded legacy identity update")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := clientB.ObjectDeletionJob.Create().SetStorageConfigID(configID).SetStorageDriver("local").
		SetObjectKey(objectKey).SetState(domaincleanup.StatePending).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := lockConn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, advisoryKey); err != nil {
		t.Fatal(err)
	}
	locked = false
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("backfill after concurrent enqueue: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("backfill did not finish after releasing identity update")
	}

	live, err := clientA.ObjectDeletionJob.Query().Where(
		objectdeletionjob.StorageConfigIDEQ(configID),
		objectdeletionjob.ObjectKeyEQ(objectKey),
		objectdeletionjob.StateIn(domaincleanup.StatePending, domaincleanup.StateRunning, domaincleanup.StateRetry, domaincleanup.StateBlocked),
	).All(ctx)
	if err != nil || len(live) != 1 || live[0].State != domaincleanup.StatePending {
		t.Fatalf("configured live jobs=%#v err=%v", live, err)
	}
	if count, err := clientA.ObjectDeletionJob.Query().Where(
		objectdeletionjob.StorageConfigIDIsNil(),
		objectdeletionjob.ObjectKeyEQ(objectKey),
		objectdeletionjob.StateIn(domaincleanup.StatePending, domaincleanup.StateRunning, domaincleanup.StateRetry, domaincleanup.StateBlocked),
	).Count(ctx); err != nil || count != 0 {
		t.Fatalf("legacy live jobs=%d err=%v", count, err)
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
