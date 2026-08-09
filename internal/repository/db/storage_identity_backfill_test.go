package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	domaincleanup "github.com/fatballfish/pic-gallery/internal/domain/objectcleanup"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imageresult"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imagetask"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/migrationcheckpoint"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/objectdeletionjob"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/referenceasset"
	"github.com/google/uuid"
	"github.com/lib/pq"
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

func TestLegacyStorageIdentityBackfillReplacesClaimedJobIDForOldWorkerSafety(t *testing.T) {
	client := openStorageIdentityBackfillSQLite(t, "old-worker-id")
	ctx := t.Context()
	configID := uuid.New()
	objectKey := "generated-images/old-worker.png"
	task := seedStorageIdentityTask(t, ctx, client, "old-worker")
	if _, err := client.ImageResult.Create().
		SetTaskID(task.ID).SetUserID(task.UserID).SetStorageDriver("local").
		SetObjectKey(objectKey).SetMimeType("image/png").SetSha256("old-worker").Save(ctx); err != nil {
		t.Fatal(err)
	}
	legacy, err := client.ObjectDeletionJob.Create().
		SetStorageDriver("local").SetObjectKey(objectKey).
		SetState(domaincleanup.StateRunning).SetAttemptCount(1).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if progress, err := RunLegacyStorageIdentityBackfill(ctx, client, "local", configID, LegacyStorageIdentityBackfillOptions{BatchSize: 1, MaxBatches: 20}); err != nil || !progress.Completed {
		t.Fatalf("backfill=%#v err=%v", progress, err)
	}
	configured, err := client.ObjectDeletionJob.Query().Where(
		objectdeletionjob.StorageConfigIDEQ(configID),
		objectdeletionjob.ObjectKeyEQ(objectKey),
		objectdeletionjob.StateIn(domaincleanup.StatePending, domaincleanup.StateRunning, domaincleanup.StateRetry, domaincleanup.StateBlocked),
	).Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if configured.ID == legacy.ID {
		t.Fatalf("legacy job ID was mutated in place: %s", legacy.ID)
	}

	deleteCalled := false
	err = c3DeleteIfUnreferencedCompatibility(ctx, client, legacy.ID, func() error {
		deleteCalled = true
		return nil
	})
	if !repoent.IsNotFound(err) || deleteCalled {
		t.Fatalf("c3 compatibility deleteCalled=%v err=%v", deleteCalled, err)
	}
}

func c3DeleteIfUnreferencedCompatibility(ctx context.Context, client *repoent.Client, jobID uuid.UUID, deleteFn func() error) error {
	tx, err := client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ObjectDeletionJob.Get(ctx, jobID); err != nil {
		return err
	}
	if err := deleteFn(); err != nil {
		return err
	}
	return tx.Commit()
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
	updated, err := runLegacyStorageIdentityBatchWithRetry(t.Context(), func() (int, error) {
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

func TestLegacyStorageIdentityBatchRetriesTransientDatabaseErrors(t *testing.T) {
	for _, transientErr := range []error{
		fmt.Errorf("wrapped SQLite busy: %w", errors.New("database is locked")),
		fmt.Errorf("wrapped SQLite locked: %w", errors.New("database table is locked")),
		fmt.Errorf("wrapped PostgreSQL deadlock: %w", &pq.Error{Code: "40P01"}),
		fmt.Errorf("wrapped PostgreSQL serialization: %w", &pq.Error{Code: "40001"}),
	} {
		attempts := 0
		updated, err := runLegacyStorageIdentityBatchWithRetry(t.Context(), func() (int, error) {
			attempts++
			if attempts == 1 {
				return 0, transientErr
			}
			return 1, nil
		})
		if err != nil || updated != 1 || attempts != 2 {
			t.Fatalf("retry %T: updated=%d attempts=%d err=%v", transientErr, updated, attempts, err)
		}
	}
}

func TestLegacyStorageDriversExcludeRemoteRowsFromEveryStorageTable(t *testing.T) {
	client := openStorageIdentityBackfillSQLite(t, "remote-rows")
	ctx := t.Context()
	task := seedStorageIdentityTask(t, ctx, client, "remote-rows")
	remote, err := client.ImageResult.Create().
		SetTaskID(task.ID).SetUserID(task.UserID).SetStorageDriver("remote").
		SetObjectKey("https://cdn.example.com/remote.png").SetMimeType("image/png").SetSha256("remote").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	asset, err := client.ReferenceAsset.Create().
		SetUserID(task.UserID).SetStatus("ready").SetStorageDriver("remote").
		SetObjectKey("https://cdn.example.com/reference.png").SetMimeType("image/png").SetSha256("remote-reference").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	task, err = task.Update().
		SetArtifactRecoveryStatus("pending").
		SetArtifactStorageDriver("remote").
		SetArtifactObjectKeys([]string{"https://cdn.example.com/recovery.png"}).
		Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	job, err := client.ObjectDeletionJob.Create().
		SetStorageDriver("remote").SetObjectKey("https://cdn.example.com/delete.png").
		SetState(domaincleanup.StatePending).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	drivers, err := ListLegacyStorageDrivers(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	if len(drivers) != 0 {
		t.Fatalf("remote result discovered as managed storage: %v", drivers)
	}
	remote, err = client.ImageResult.Get(ctx, remote.ID)
	if err != nil || remote.StorageConfigID != nil {
		t.Fatalf("remote result mutated: %#v err=%v", remote, err)
	}
	asset, err = client.ReferenceAsset.Get(ctx, asset.ID)
	if err != nil || asset.StorageConfigID != nil {
		t.Fatalf("remote asset mutated: %#v err=%v", asset, err)
	}
	task, err = client.ImageTask.Get(ctx, task.ID)
	if err != nil || task.ArtifactStorageConfigID != nil {
		t.Fatalf("remote recovery mutated: %#v err=%v", task, err)
	}
	job, err = client.ObjectDeletionJob.Get(ctx, job.ID)
	if err != nil || job.StorageConfigID != nil {
		t.Fatalf("remote cleanup job mutated: %#v err=%v", job, err)
	}
}

func TestLegacyStorageIdentityBackfillRemoteDriverIsNoOp(t *testing.T) {
	client := openStorageIdentityBackfillSQLite(t, "remote-noop")
	ctx := t.Context()
	task := seedStorageIdentityTask(t, ctx, client, "remote-noop")
	remote, err := client.ReferenceAsset.Create().
		SetUserID(task.UserID).SetStatus("ready").SetStorageDriver("remote").
		SetObjectKey("https://cdn.example.com/reference.png").SetMimeType("image/png").SetSha256("remote").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}

	progress, err := RunLegacyStorageIdentityBackfill(ctx, client, "remote", uuid.New(), LegacyStorageIdentityBackfillOptions{})
	if err != nil || !progress.Completed || progress.ProcessedRows != 0 {
		t.Fatalf("remote backfill progress=%#v err=%v", progress, err)
	}
	if count, err := client.MigrationCheckpoint.Query().Count(ctx); err != nil || count != 0 {
		t.Fatalf("remote backfill created checkpoints: count=%d err=%v", count, err)
	}
	remote, err = client.ReferenceAsset.Get(ctx, remote.ID)
	if err != nil || remote.StorageConfigID != nil {
		t.Fatalf("remote backfill mutated row: %#v err=%v", remote, err)
	}
}

func TestPrepareLegacyStorageCleanupCutoversRetriesSimultaneousSQLiteFirstStartup(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "legacy-prepare-race.db")
	databaseURL := fmt.Sprintf("file:%s?_fk=1&_busy_timeout=5000&_journal_mode=WAL", databasePath)
	clients := make([]*repoent.Client, 3)
	for index := range clients {
		client, err := repoent.Open(dialect.SQLite, databaseURL)
		if err != nil {
			t.Fatal(err)
		}
		clients[index] = client
		t.Cleanup(func() { _ = client.Close() })
	}
	ctx := t.Context()
	if err := clients[0].Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	blocker, err := clients[0].Tx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := blocker.MigrationCheckpoint.Create().
		SetName("prepare-race-blocker").SetPhase("test").Save(ctx); err != nil {
		_ = blocker.Rollback()
		t.Fatal(err)
	}

	start := make(chan struct{})
	done := make(chan error, 2)
	for _, client := range clients[1:] {
		go func(client *repoent.Client) {
			<-start
			done <- PrepareLegacyStorageCleanupCutovers(ctx, client, []string{"local", "s3"})
		}(client)
	}
	close(start)
	time.Sleep(50 * time.Millisecond)
	if err := blocker.Commit(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := <-done; err != nil {
			t.Fatalf("simultaneous SQLite prepare: %v", err)
		}
	}
	markers, err := clients[0].MigrationCheckpoint.Query().Where(
		migrationcheckpoint.NameIn(
			legacyCleanupCutoverGlobalMarker,
			legacyCleanupCutoverMarkerName("local"),
			legacyCleanupCutoverMarkerName("s3"),
		),
	).All(ctx)
	if err != nil || len(markers) != 3 {
		t.Fatalf("prepared markers=%#v err=%v", markers, err)
	}
	for _, marker := range markers {
		if marker.Completed || marker.Phase != legacyCleanupCutoverPreparing {
			t.Fatalf("invalid prepared marker=%#v", marker)
		}
	}
	database, err := sql.Open("sqlite3", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	var triggerCount int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'trigger' AND name IN (
		  'enforce_legacy_cleanup_identity_cutover_insert',
		  'enforce_legacy_cleanup_identity_cutover_update'
		)
	`).Scan(&triggerCount); err != nil || triggerCount != 2 {
		t.Fatalf("cleanup barrier trigger count=%d err=%v", triggerCount, err)
	}
}

func TestPrepareLegacyStorageCleanupCutoversSurfacesPermanentSQLError(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, "file:legacy-prepare-permanent-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := PrepareLegacyStorageCleanupCutovers(t.Context(), client, []string{"local"}); err == nil {
		t.Fatal("prepare without schema must surface its permanent SQL error")
	}
}

func TestPrepareLegacyStorageCleanupCutoversRetriesSimultaneousPostgresFirstStartup(t *testing.T) {
	database, databaseURL := openLegacyMigrationPostgres(t)
	ctx := context.Background()
	clientA, err := repoent.Open(dialect.Postgres, postgresURLWithApplicationName(t, databaseURL, "legacy-prepare-a"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientA.Close() })
	clientB, err := repoent.Open(dialect.Postgres, postgresURLWithApplicationName(t, databaseURL, "legacy-prepare-b"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientB.Close() })
	if err := clientA.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	blocker, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := blocker.ExecContext(ctx, `LOCK TABLE migration_checkpoints IN ACCESS EXCLUSIVE MODE`); err != nil {
		_ = blocker.Rollback()
		t.Fatal(err)
	}
	start := make(chan struct{})
	done := make(chan error, 2)
	for _, client := range []*repoent.Client{clientA, clientB} {
		go func(client *repoent.Client) {
			<-start
			done <- PrepareLegacyStorageCleanupCutovers(ctx, client, []string{"local", "s3"})
		}(client)
	}
	close(start)
	waitForPostgresApplicationLock(t, database, "legacy-prepare-a", "")
	waitForPostgresApplicationLock(t, database, "legacy-prepare-b", "")
	if err := blocker.Commit(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := <-done; err != nil {
			t.Fatalf("simultaneous PostgreSQL prepare: %v", err)
		}
	}
	var markerCount int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM migration_checkpoints
		WHERE name IN (
		  'legacy_cleanup_nil_config_cutover_v1:global',
		  'legacy_cleanup_nil_config_cutover_v1:local',
		  'legacy_cleanup_nil_config_cutover_v1:s3'
		)
	`).Scan(&markerCount); err != nil || markerCount != 3 {
		t.Fatalf("PostgreSQL marker count=%d err=%v", markerCount, err)
	}
	var triggerCount int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM pg_trigger
		WHERE tgname = 'enforce_legacy_cleanup_identity_cutover'
		  AND tgrelid = 'object_deletion_jobs'::regclass
		  AND NOT tgisinternal
	`).Scan(&triggerCount); err != nil || triggerCount != 1 {
		t.Fatalf("PostgreSQL trigger count=%d err=%v", triggerCount, err)
	}
}

func TestListLegacyStorageDriversIgnoresPostgresJSONNullArtifactKeys(t *testing.T) {
	database, databaseURL := openLegacyMigrationPostgres(t)
	ctx := t.Context()
	client, err := repoent.Open(dialect.Postgres, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	task := seedStorageIdentityTask(t, ctx, client, "postgres-json-null")
	if _, err := database.ExecContext(ctx, `UPDATE image_tasks SET artifact_object_keys = 'null'::jsonb WHERE id = $1`, task.ID); err != nil {
		t.Fatal(err)
	}
	drivers, err := ListLegacyStorageDrivers(ctx, client)
	if err != nil {
		t.Fatalf("list drivers with JSON null artifact keys: %v", err)
	}
	if len(drivers) != 0 {
		t.Fatalf("JSON null artifact keys produced legacy drivers: %v", drivers)
	}
}

func TestLegacyCleanupWriteBarrierAllowsPreparingAndRejectsManagedWritesAfterCutover(t *testing.T) {
	client := openStorageIdentityBackfillSQLite(t, "write-barrier")
	ctx := t.Context()
	if err := PrepareLegacyStorageCleanupCutovers(ctx, client, []string{"local", "s3"}); err != nil {
		t.Fatal(err)
	}
	before, err := client.ObjectDeletionJob.Create().SetStorageDriver("local").SetObjectKey("generated-images/before-cutover.png").
		SetState(domaincleanup.StatePending).Save(ctx)
	if err != nil {
		t.Fatalf("preparing barrier rejected old writer: %v", err)
	}
	localID := uuid.New()
	if progress, err := RunLegacyStorageIdentityBackfill(ctx, client, "local", localID, LegacyStorageIdentityBackfillOptions{}); err != nil || !progress.Completed {
		t.Fatalf("local cutover=%#v err=%v", progress, err)
	}
	if _, err := client.ObjectDeletionJob.Get(ctx, before.ID); !repoent.IsNotFound(err) {
		t.Fatalf("pre-cutover legacy job still exists: %v", err)
	}
	if _, err := client.ObjectDeletionJob.Create().SetStorageDriver("local").SetObjectKey("generated-images/after-cutover.png").
		SetState(domaincleanup.StatePending).Save(ctx); err == nil {
		t.Fatal("armed local barrier allowed old nil-config writer")
	}
	if _, err := client.ObjectDeletionJob.Create().SetStorageDriver("s3").SetObjectKey("generated-images/s3-preparing.png").
		SetState(domaincleanup.StatePending).Save(ctx); err != nil {
		t.Fatalf("preparing s3 barrier rejected old writer: %v", err)
	}
	if _, err := client.ObjectDeletionJob.Create().SetStorageDriver("remote").SetObjectKey("https://cdn.example.com/remote.png").
		SetState(domaincleanup.StatePending).Save(ctx); err != nil {
		t.Fatalf("remote cleanup semantics were blocked: %v", err)
	}
}

func TestLegacyCleanupWriteBarrierRejectsFirstLateWriteAfterEmptyCutover(t *testing.T) {
	client := openStorageIdentityBackfillSQLite(t, "write-barrier-empty")
	ctx := t.Context()
	if err := PrepareLegacyStorageCleanupCutovers(ctx, client, []string{"local"}); err != nil {
		t.Fatal(err)
	}
	if progress, err := RunLegacyStorageIdentityBackfill(ctx, client, "local", uuid.New(), LegacyStorageIdentityBackfillOptions{}); err != nil || !progress.Completed {
		t.Fatalf("empty cutover=%#v err=%v", progress, err)
	}
	if _, err := client.ObjectDeletionJob.Create().SetStorageDriver("").SetObjectKey("generated-images/first-late.png").
		SetState(domaincleanup.StatePending).Save(ctx); err == nil {
		t.Fatal("empty cutover allowed first late local write")
	}
}

func TestLegacyStorageCleanupFinalCutoverSerializesOldWriterOnSQLite(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "legacy-cutover.db")
	databaseURL := fmt.Sprintf("file:%s?_fk=1&_busy_timeout=5000&_journal_mode=WAL", databasePath)
	finalizer, err := repoent.Open(dialect.SQLite, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = finalizer.Close() })
	oldWriter, err := repoent.Open(dialect.SQLite, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = oldWriter.Close() })
	ctx := t.Context()
	if err := finalizer.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	if err := PrepareLegacyStorageCleanupCutovers(ctx, finalizer, []string{"local"}); err != nil {
		t.Fatal(err)
	}
	configID := uuid.New()
	type legacyWrite struct {
		tx  *repoent.Tx
		job *repoent.ObjectDeletionJob
	}
	inserted := make(chan legacyWrite, 1)
	done := make(chan error, 1)
	go func() {
		injected := false
		_, err := RunLegacyStorageIdentityBackfill(ctx, finalizer, "local", configID, LegacyStorageIdentityBackfillOptions{
			BatchSize: 100, MaxBatches: 20,
			afterBatch: func(progress LegacyStorageIdentityBackfillProgress) error {
				if injected || progress.Phase != legacyStoragePhaseJobsCutover {
					return nil
				}
				injected = true
				tx, err := oldWriter.Tx(ctx)
				if err != nil {
					return err
				}
				job, err := tx.ObjectDeletionJob.Create().
					SetStorageDriver("local").
					SetObjectKey("generated-images/sqlite-old-write.png").
					SetState(domaincleanup.StatePending).
					Save(ctx)
				if err != nil {
					_ = tx.Rollback()
					return err
				}
				inserted <- legacyWrite{tx: tx, job: job}
				return nil
			},
		})
		done <- err
	}()
	old := <-inserted
	time.Sleep(50 * time.Millisecond)
	if err := old.tx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("finalize SQLite cleanup cutover: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SQLite cleanup cutover did not finish")
	}
	if _, err := finalizer.ObjectDeletionJob.Get(ctx, old.job.ID); !repoent.IsNotFound(err) {
		t.Fatalf("old SQLite writer job survived cutover: %v", err)
	}
	if _, err := finalizer.ObjectDeletionJob.Query().Where(
		objectdeletionjob.StorageConfigIDEQ(configID),
		objectdeletionjob.ObjectKeyEQ(old.job.ObjectKey),
		objectdeletionjob.StateEQ(domaincleanup.StatePending),
	).Only(ctx); err != nil {
		t.Fatalf("configured SQLite replacement: %v", err)
	}
	if _, err := oldWriter.ObjectDeletionJob.Create().
		SetStorageDriver("local").
		SetObjectKey("generated-images/sqlite-late-write.png").
		SetState(domaincleanup.StatePending).
		Save(ctx); err == nil {
		t.Fatal("armed SQLite cutover allowed a late nil-config writer")
	}
}

func TestLegacyStorageIdentityBackfillRetriesConcurrentConfiguredEnqueueOnPostgres(t *testing.T) {
	database, databaseURL := openLegacyMigrationPostgres(t)
	ctx := context.Background()
	clientA, err := repoent.Open(dialect.Postgres, postgresURLWithApplicationName(t, databaseURL, "legacy-backfill-test"))
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
		CREATE FUNCTION block_configured_cleanup_insert() RETURNS trigger AS $$
		BEGIN
			IF NEW.storage_config_id IS NOT NULL
			   AND current_setting('application_name') = 'legacy-backfill-test' THEN
				PERFORM pg_advisory_lock(73490217);
				PERFORM pg_advisory_unlock(73490217);
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER block_configured_cleanup_insert
		BEFORE INSERT ON object_deletion_jobs
		FOR EACH ROW EXECUTE FUNCTION block_configured_cleanup_insert();
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
			t.Fatal("backfill did not reach the guarded configured-job insert")
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

func TestLegacyStorageCleanupFinalCutoverSerializesOldWritersOnPostgres(t *testing.T) {
	database, databaseURL := openLegacyMigrationPostgres(t)
	ctx := context.Background()
	finalizer, err := repoent.Open(dialect.Postgres, postgresURLWithApplicationName(t, databaseURL, "legacy-cutover-finalizer"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = finalizer.Close() })
	oldWriter, err := repoent.Open(dialect.Postgres, postgresURLWithApplicationName(t, databaseURL, "legacy-cutover-old-writer"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = oldWriter.Close() })
	lateWriter, err := repoent.Open(dialect.Postgres, postgresURLWithApplicationName(t, databaseURL, "legacy-cutover-late-writer"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lateWriter.Close() })
	if err := finalizer.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	if err := PrepareLegacyStorageCleanupCutovers(ctx, finalizer, []string{"local"}); err != nil {
		t.Fatal(err)
	}
	checkpoint, err := finalizer.MigrationCheckpoint.Create().
		SetName("test-final-cutover-" + uuid.NewString()).
		SetPhase(legacyStoragePhaseJobsCutover).
		Save(ctx)
	if err != nil {
		t.Fatal(err)
	}

	const advisoryKey int64 = 73490218
	if _, err := database.ExecContext(ctx, `
		CREATE FUNCTION block_legacy_cleanup_cutover_arm() RETURNS trigger AS $$
		BEGIN
			IF NEW.name = 'legacy_cleanup_nil_config_cutover_v1:local'
			   AND NEW.phase = 'arming'
			   AND current_setting('application_name') = 'legacy-cutover-finalizer' THEN
				PERFORM pg_advisory_lock(73490218);
				PERFORM pg_advisory_unlock(73490218);
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER block_legacy_cleanup_cutover_arm
		BEFORE UPDATE ON migration_checkpoints
		FOR EACH ROW EXECUTE FUNCTION block_legacy_cleanup_cutover_arm();
	`); err != nil {
		t.Fatal(err)
	}
	lockConn, err := database.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lockConn.Close() })
	if _, err := lockConn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, advisoryKey); err != nil {
		t.Fatal(err)
	}
	locked := true
	t.Cleanup(func() {
		if locked {
			_, _ = lockConn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, advisoryKey)
		}
	})

	oldTx, err := oldWriter.Tx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	legacyJob, err := oldTx.ObjectDeletionJob.Create().
		SetStorageDriver("local").
		SetObjectKey("generated-images/final-cutover-old-write.png").
		SetState(domaincleanup.StatePending).
		Save(ctx)
	if err != nil {
		_ = oldTx.Rollback()
		t.Fatal(err)
	}

	configID := uuid.New()
	finalDone := make(chan error, 1)
	go func() {
		_, err := runLegacyStorageIdentityBatch(ctx, finalizer, checkpoint.ID, "local", configID, 100)
		finalDone <- err
	}()
	waitForPostgresApplicationLock(t, database, "legacy-cutover-finalizer", "")
	if err := oldTx.Commit(); err != nil {
		t.Fatal(err)
	}
	waitForPostgresApplicationLock(t, database, "legacy-cutover-finalizer", "advisory")

	lateDone := make(chan error, 1)
	go func() {
		_, err := lateWriter.ObjectDeletionJob.Create().
			SetStorageDriver("local").
			SetObjectKey("generated-images/final-cutover-late-write.png").
			SetState(domaincleanup.StatePending).
			Save(ctx)
		lateDone <- err
	}()
	waitForPostgresApplicationLock(t, database, "legacy-cutover-late-writer", "")

	if _, err := lockConn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, advisoryKey); err != nil {
		t.Fatal(err)
	}
	locked = false
	select {
	case err := <-finalDone:
		if err != nil {
			t.Fatalf("finalize cleanup cutover: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cleanup cutover did not finish")
	}
	select {
	case err := <-lateDone:
		if err == nil {
			t.Fatal("late nil-config writer was not rejected after cutover")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("late nil-config writer did not finish")
	}

	if _, err := finalizer.ObjectDeletionJob.Get(ctx, legacyJob.ID); !repoent.IsNotFound(err) {
		t.Fatalf("old writer job survived cutover: %v", err)
	}
	configured, err := finalizer.ObjectDeletionJob.Query().Where(
		objectdeletionjob.StorageConfigIDEQ(configID),
		objectdeletionjob.ObjectKeyEQ(legacyJob.ObjectKey),
		objectdeletionjob.StateEQ(domaincleanup.StatePending),
	).Only(ctx)
	if err != nil || configured.ID == legacyJob.ID {
		t.Fatalf("configured replacement=%#v err=%v", configured, err)
	}
	marker, err := finalizer.MigrationCheckpoint.Query().Where(
		migrationcheckpoint.NameEQ(legacyCleanupCutoverMarkerName("local")),
	).Only(ctx)
	if err != nil || !marker.Completed || marker.Phase != legacyCleanupCutoverArmed {
		t.Fatalf("cutover marker=%#v err=%v", marker, err)
	}
}

func waitForPostgresApplicationLock(t *testing.T, database interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, applicationName, waitEvent string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting bool
		if err := database.QueryRowContext(t.Context(), `
			SELECT EXISTS (
				SELECT 1 FROM pg_stat_activity
				WHERE datname = current_database()
				  AND application_name = $1
				  AND wait_event_type = 'Lock'
				  AND ($2 = '' OR wait_event = $2)
			)
		`, applicationName, waitEvent).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("application %q did not wait for %q lock", applicationName, waitEvent)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func postgresURLWithApplicationName(t *testing.T, rawURL, applicationName string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse postgres URL: %v", err)
	}
	query := parsed.Query()
	query.Set("application_name", applicationName)
	parsed.RawQuery = query.Encode()
	return parsed.String()
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
