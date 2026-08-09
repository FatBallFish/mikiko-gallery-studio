package db

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	domaincleanup "github.com/fatballfish/pic-gallery/internal/domain/objectcleanup"
	domainstorageconfig "github.com/fatballfish/pic-gallery/internal/domain/storageconfig"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imageresult"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imagetask"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/migrationcheckpoint"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/objectdeletionjob"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/predicate"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/referenceasset"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

const (
	legacyStorageIdentityMigrationPrefix = "legacy_storage_identity_v3"
	legacyStoragePhaseResults            = "results"
	legacyStoragePhaseAssets             = "assets"
	legacyStoragePhaseTasks              = "tasks"
	legacyStoragePhaseJobs               = "jobs"
	legacyStoragePhaseJobsCutover        = "jobs_cutover"
	legacyStoragePhaseValidate           = "validate"
	legacyStoragePhaseDone               = "done"
	legacyStorageBatchRetryLimit         = 5
	legacyStorageBatchRetryDelay         = 25 * time.Millisecond
)

type LegacyStorageIdentityBackfillOptions struct {
	BatchSize  int
	MaxBatches int
	afterBatch func(LegacyStorageIdentityBackfillProgress) error
}

type LegacyStorageIdentityBackfillProgress struct {
	Phase         string
	Batches       int
	UpdatedRows   int
	ProcessedRows int
	Completed     bool
}

func ListLegacyStorageDrivers(ctx context.Context, client *repoent.Client) ([]string, error) {
	if client == nil {
		return nil, fmt.Errorf("legacy storage identity client is required")
	}
	drivers := make(map[string]struct{})
	collect := func(values []string) {
		for _, value := range values {
			if driver, ok := domainstorageconfig.NormalizeManagedDriver(value); ok {
				drivers[driver] = struct{}{}
			}
		}
	}
	resultDrivers, err := client.ImageResult.Query().Where(imageresult.StorageConfigIDIsNil()).
		Select(imageresult.FieldStorageDriver).Strings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list legacy image result storage drivers: %w", err)
	}
	collect(resultDrivers)
	assetDrivers, err := client.ReferenceAsset.Query().Where(referenceasset.StorageConfigIDIsNil()).
		Select(referenceasset.FieldStorageDriver).Strings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list legacy reference asset storage drivers: %w", err)
	}
	collect(assetDrivers)
	taskDrivers, err := client.ImageTask.Query().Where(imagetask.ArtifactStorageConfigIDIsNil(), legacyArtifactStorageTuple()).
		Select(imagetask.FieldArtifactStorageDriver).Strings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list legacy artifact recovery storage drivers: %w", err)
	}
	collect(taskDrivers)
	jobDrivers, err := client.ObjectDeletionJob.Query().Where(
		objectdeletionjob.StorageConfigIDIsNil(),
		objectdeletionjob.StateIn(domaincleanup.StatePending, domaincleanup.StateRunning, domaincleanup.StateRetry, domaincleanup.StateBlocked),
	).Select(objectdeletionjob.FieldStorageDriver).Strings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list legacy cleanup job storage drivers: %w", err)
	}
	collect(jobDrivers)
	result := make([]string, 0, len(drivers))
	for driver := range drivers {
		result = append(result, driver)
	}
	sort.Strings(result)
	return result, nil
}

// RunLegacyStorageIdentityBackfill assigns the immutable bootstrap storage
// config to managed rows that previously relied on driver-based runtime
// fallback. Non-managed drivers are a completed no-op.
func RunLegacyStorageIdentityBackfill(
	ctx context.Context,
	client *repoent.Client,
	driver string,
	configID uuid.UUID,
	opts LegacyStorageIdentityBackfillOptions,
) (LegacyStorageIdentityBackfillProgress, error) {
	if client == nil {
		return LegacyStorageIdentityBackfillProgress{}, fmt.Errorf("legacy storage identity backfill client is required")
	}
	managedDriver, managed := domainstorageconfig.NormalizeManagedDriver(driver)
	if !managed {
		return LegacyStorageIdentityBackfillProgress{Phase: legacyStoragePhaseDone, Completed: true}, nil
	}
	driver = managedDriver
	if configID == uuid.Nil {
		return LegacyStorageIdentityBackfillProgress{}, fmt.Errorf("legacy storage identity config ID is required")
	}
	if opts.BatchSize <= 0 || opts.BatchSize > 1000 {
		opts.BatchSize = 100
	}
	if opts.MaxBatches <= 0 {
		opts.MaxBatches = 100
	}
	if err := PrepareLegacyStorageCleanupCutovers(ctx, client, []string{driver}); err != nil {
		return LegacyStorageIdentityBackfillProgress{}, err
	}
	checkpoint, err := loadLegacyStorageCheckpoint(ctx, client, driver, configID)
	if err != nil {
		return LegacyStorageIdentityBackfillProgress{}, err
	}
	checkpoint, err = revalidateLegacyStorageCheckpoint(ctx, client, checkpoint.ID, driver)
	if err != nil {
		return LegacyStorageIdentityBackfillProgress{}, err
	}
	progress := legacyStorageProgress(checkpoint)
	for progress.Batches < opts.MaxBatches && !progress.Completed {
		updated, batchErr := runLegacyStorageIdentityBatchWithRetry(ctx, func() (int, error) {
			return runLegacyStorageIdentityBatch(ctx, client, checkpoint.ID, driver, configID, opts.BatchSize)
		})
		if batchErr != nil {
			return progress, batchErr
		}
		progress.Batches++
		progress.UpdatedRows += updated
		checkpoint, err = client.MigrationCheckpoint.Get(ctx, checkpoint.ID)
		if err != nil {
			return progress, fmt.Errorf("reload legacy storage identity checkpoint: %w", err)
		}
		progress.Phase = checkpoint.Phase
		progress.ProcessedRows = checkpoint.ProcessedRows
		progress.Completed = checkpoint.Completed
		if opts.afterBatch != nil {
			if err := opts.afterBatch(progress); err != nil {
				return progress, err
			}
		}
	}
	return progress, nil
}

func runLegacyStorageIdentityBatchWithRetry(ctx context.Context, run func() (int, error)) (int, error) {
	var err error
	for attempt := 0; attempt < legacyStorageBatchRetryLimit; attempt++ {
		var updated int
		updated, err = run()
		if err == nil || !isLegacyStorageBatchRetryable(err) {
			return updated, err
		}
		if attempt == legacyStorageBatchRetryLimit-1 {
			break
		}
		timer := time.NewTimer(time.Duration(attempt+1) * legacyStorageBatchRetryDelay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return 0, ctx.Err()
		case <-timer.C:
		}
	}
	return 0, err
}

func isLegacyStorageBatchRetryable(err error) bool {
	if repoent.IsConstraintError(err) {
		return true
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "database is locked") || strings.Contains(message, "database table is locked") {
		return true
	}
	var postgresErr *pq.Error
	if errors.As(err, &postgresErr) {
		return postgresErr.Code == "40P01" || postgresErr.Code == "40001"
	}
	return false
}

func loadLegacyStorageCheckpoint(ctx context.Context, client *repoent.Client, driver string, configID uuid.UUID) (*repoent.MigrationCheckpoint, error) {
	name := legacyStorageCheckpointName(driver, configID)
	checkpoint, err := client.MigrationCheckpoint.Query().Where(migrationcheckpoint.NameEQ(name)).Only(ctx)
	if err == nil {
		return checkpoint, nil
	}
	if !repoent.IsNotFound(err) {
		return nil, fmt.Errorf("load legacy storage identity checkpoint: %w", err)
	}
	checkpoint, err = client.MigrationCheckpoint.Create().SetName(name).SetPhase(legacyStoragePhaseJobs).Save(ctx)
	if repoent.IsConstraintError(err) {
		checkpoint, err = client.MigrationCheckpoint.Query().Where(migrationcheckpoint.NameEQ(name)).Only(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("create legacy storage identity checkpoint: %w", err)
	}
	return checkpoint, nil
}

func revalidateLegacyStorageCheckpoint(ctx context.Context, client *repoent.Client, checkpointID int, driver string) (*repoent.MigrationCheckpoint, error) {
	tx, err := client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("start legacy storage checkpoint validation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	checkpoint, err := tx.MigrationCheckpoint.Query().Where(migrationcheckpoint.IDEQ(checkpointID), lockLegacyStorageCheckpoint()).Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("lock legacy storage checkpoint for validation: %w", err)
	}
	if checkpoint.Completed || checkpoint.Phase == legacyStoragePhaseDone {
		remaining, err := legacyStorageIdentityRowsRemain(ctx, tx, driver)
		if err != nil {
			return nil, err
		}
		if remaining {
			checkpoint, err = tx.MigrationCheckpoint.UpdateOneID(checkpoint.ID).
				SetPhase(legacyStoragePhaseJobs).
				SetCompleted(false).
				ClearAfterTaskID().
				ClearAfterResultID().
				Save(ctx)
			if err != nil {
				return nil, fmt.Errorf("reopen legacy storage checkpoint: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit legacy storage checkpoint validation: %w", err)
	}
	return checkpoint, nil
}

func legacyStorageCheckpointName(driver string, configID uuid.UUID) string {
	return legacyStorageIdentityMigrationPrefix + ":" + normalizeLegacyStorageDriver(driver) + ":" + configID.String()
}

func runLegacyStorageIdentityBatch(ctx context.Context, client *repoent.Client, checkpointID int, driver string, configID uuid.UUID, batchSize int) (int, error) {
	tx, err := client.Tx(ctx)
	if err != nil {
		return 0, fmt.Errorf("start legacy storage identity batch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	checkpoint, err := tx.MigrationCheckpoint.Query().Where(migrationcheckpoint.IDEQ(checkpointID), lockLegacyStorageCheckpoint()).Only(ctx)
	if err != nil {
		return 0, fmt.Errorf("lock legacy storage identity checkpoint: %w", err)
	}
	var updated int
	switch checkpoint.Phase {
	case legacyStoragePhaseJobs:
		updated, err = backfillLegacyStorageJobs(ctx, tx, checkpoint, driver, configID, batchSize)
	case legacyStoragePhaseJobsCutover:
		updated, err = finalizeLegacyStorageJobCutover(ctx, tx, checkpoint.ID, driver, configID)
	case legacyStoragePhaseResults:
		updated, err = backfillLegacyStorageResults(ctx, tx, checkpoint, driver, configID, batchSize)
	case legacyStoragePhaseAssets:
		updated, err = backfillLegacyStorageAssets(ctx, tx, checkpoint, driver, configID, batchSize)
	case legacyStoragePhaseTasks:
		updated, err = backfillLegacyStorageTasks(ctx, tx, checkpoint, driver, configID, batchSize)
	case legacyStoragePhaseValidate:
		err = validateLegacyStorageIdentity(ctx, tx, checkpoint, driver)
	case legacyStoragePhaseDone:
		return 0, nil
	default:
		return 0, fmt.Errorf("unsupported legacy storage identity phase %q", checkpoint.Phase)
	}
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit legacy storage identity batch: %w", err)
	}
	return updated, nil
}

func lockLegacyStorageCheckpoint() func(*entsql.Selector) {
	return func(selector *entsql.Selector) {
		if selector.Dialect() == "postgres" {
			selector.ForUpdate()
		}
	}
}

func backfillLegacyStorageResults(ctx context.Context, tx *repoent.Tx, checkpoint *repoent.MigrationCheckpoint, driver string, configID uuid.UUID, limit int) (int, error) {
	query := tx.ImageResult.Query().Where(imageresult.StorageConfigIDIsNil(), legacyResultDriver(driver))
	if checkpoint.AfterResultID != nil {
		query.Where(imageresult.IDGT(*checkpoint.AfterResultID))
	}
	rows, err := query.Order(repoent.Asc(imageresult.FieldID)).Limit(limit + 1).All(ctx)
	if err != nil {
		return 0, fmt.Errorf("list legacy storage image results: %w", err)
	}
	rows, hasMore := boundedImageResults(rows, limit)
	updated := 0
	for _, row := range rows {
		count, err := tx.ImageResult.Update().Where(imageresult.IDEQ(row.ID), imageresult.StorageConfigIDIsNil()).SetStorageConfigID(configID).Save(ctx)
		if err != nil {
			return 0, fmt.Errorf("backfill image result %s storage identity: %w", row.ID, err)
		}
		updated += count
	}
	update := tx.MigrationCheckpoint.UpdateOneID(checkpoint.ID).AddProcessedRows(updated)
	if len(rows) > 0 {
		update.SetAfterResultID(rows[len(rows)-1].ID)
	}
	if !hasMore {
		update.SetPhase(legacyStoragePhaseAssets).ClearAfterResultID()
	}
	if err := update.Exec(ctx); err != nil {
		return 0, fmt.Errorf("checkpoint legacy image results: %w", err)
	}
	return updated, nil
}

func backfillLegacyStorageAssets(ctx context.Context, tx *repoent.Tx, checkpoint *repoent.MigrationCheckpoint, driver string, configID uuid.UUID, limit int) (int, error) {
	query := tx.ReferenceAsset.Query().Where(referenceasset.StorageConfigIDIsNil(), legacyAssetDriver(driver))
	if checkpoint.AfterResultID != nil {
		query.Where(referenceasset.IDGT(*checkpoint.AfterResultID))
	}
	rows, err := query.Order(repoent.Asc(referenceasset.FieldID)).Limit(limit + 1).All(ctx)
	if err != nil {
		return 0, fmt.Errorf("list legacy storage reference assets: %w", err)
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	updated := 0
	for _, row := range rows {
		count, err := tx.ReferenceAsset.Update().Where(referenceasset.IDEQ(row.ID), referenceasset.StorageConfigIDIsNil()).SetStorageConfigID(configID).Save(ctx)
		if err != nil {
			return 0, fmt.Errorf("backfill reference asset %s storage identity: %w", row.ID, err)
		}
		updated += count
	}
	update := tx.MigrationCheckpoint.UpdateOneID(checkpoint.ID).AddProcessedRows(updated)
	if len(rows) > 0 {
		update.SetAfterResultID(rows[len(rows)-1].ID)
	}
	if !hasMore {
		update.SetPhase(legacyStoragePhaseTasks).ClearAfterResultID().ClearAfterTaskID()
	}
	if err := update.Exec(ctx); err != nil {
		return 0, fmt.Errorf("checkpoint legacy reference assets: %w", err)
	}
	return updated, nil
}

func backfillLegacyStorageTasks(ctx context.Context, tx *repoent.Tx, checkpoint *repoent.MigrationCheckpoint, driver string, configID uuid.UUID, limit int) (int, error) {
	query := tx.ImageTask.Query().Where(imagetask.ArtifactStorageConfigIDIsNil(), legacyTaskDriver(driver), legacyArtifactStorageTuple())
	if checkpoint.AfterTaskID != nil {
		query.Where(imagetask.IDGT(*checkpoint.AfterTaskID))
	}
	rows, err := query.Order(repoent.Asc(imagetask.FieldID)).Limit(limit + 1).All(ctx)
	if err != nil {
		return 0, fmt.Errorf("list legacy artifact recoveries: %w", err)
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	updated := 0
	for _, row := range rows {
		count, err := tx.ImageTask.Update().Where(imagetask.IDEQ(row.ID), imagetask.ArtifactStorageConfigIDIsNil()).SetArtifactStorageConfigID(configID).Save(ctx)
		if err != nil {
			return 0, fmt.Errorf("backfill image task %s artifact storage identity: %w", row.ID, err)
		}
		updated += count
	}
	update := tx.MigrationCheckpoint.UpdateOneID(checkpoint.ID).AddProcessedRows(updated)
	if len(rows) > 0 {
		update.SetAfterTaskID(rows[len(rows)-1].ID)
	}
	if !hasMore {
		update.SetPhase(legacyStoragePhaseValidate).ClearAfterTaskID().ClearAfterResultID()
	}
	if err := update.Exec(ctx); err != nil {
		return 0, fmt.Errorf("checkpoint legacy artifact recoveries: %w", err)
	}
	return updated, nil
}

func backfillLegacyStorageJobs(ctx context.Context, tx *repoent.Tx, checkpoint *repoent.MigrationCheckpoint, driver string, configID uuid.UUID, limit int) (int, error) {
	query := tx.ObjectDeletionJob.Query().Where(
		objectdeletionjob.StorageConfigIDIsNil(),
		legacyJobDriver(driver),
		objectdeletionjob.StateIn(domaincleanup.StatePending, domaincleanup.StateRunning, domaincleanup.StateRetry, domaincleanup.StateBlocked),
		lockLegacyStorageJobs(),
	)
	if checkpoint.AfterResultID != nil {
		query.Where(objectdeletionjob.IDGT(*checkpoint.AfterResultID))
	}
	rows, err := query.Order(repoent.Asc(objectdeletionjob.FieldID)).Limit(limit + 1).All(ctx)
	if err != nil {
		return 0, fmt.Errorf("list legacy object deletion jobs: %w", err)
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	updated := 0
	for _, row := range rows {
		if err := replaceLegacyCleanupJob(ctx, tx, row, configID); err != nil {
			return 0, err
		}
		updated++
	}
	update := tx.MigrationCheckpoint.UpdateOneID(checkpoint.ID).AddProcessedRows(updated)
	if len(rows) > 0 {
		update.SetAfterResultID(rows[len(rows)-1].ID)
	}
	if !hasMore {
		update.SetPhase(legacyStoragePhaseJobsCutover).ClearAfterResultID()
	}
	if err := update.Exec(ctx); err != nil {
		return 0, fmt.Errorf("checkpoint legacy object deletion jobs: %w", err)
	}
	return updated, nil
}

func replaceLegacyCleanupJob(ctx context.Context, tx *repoent.Tx, legacy *repoent.ObjectDeletionJob, configID uuid.UUID) error {
	existing, err := tx.ObjectDeletionJob.Query().Where(
		objectdeletionjob.StorageConfigIDEQ(configID),
		objectdeletionjob.ObjectKeyEQ(legacy.ObjectKey),
		objectdeletionjob.StateIn(domaincleanup.StatePending, domaincleanup.StateRunning, domaincleanup.StateRetry, domaincleanup.StateBlocked),
	).Order(repoent.Asc(objectdeletionjob.FieldCreatedAt), repoent.Asc(objectdeletionjob.FieldID)).All(ctx)
	if err != nil {
		return fmt.Errorf("find configured object deletion jobs for %s: %w", legacy.ID, err)
	}
	if len(existing) == 0 {
		created, err := tx.ObjectDeletionJob.Create().
			SetStorageConfigID(configID).
			SetStorageDriver(normalizeLegacyStorageDriver(legacy.StorageDriver)).
			SetBucket(legacy.Bucket).
			SetObjectKey(legacy.ObjectKey).
			SetState(domaincleanup.StatePending).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("create configured object deletion job for %s: %w", legacy.ID, err)
		}
		existing = []*repoent.ObjectDeletionJob{created}
	}
	primary := configuredCleanupJobPrimary(existing)
	for _, duplicate := range existing {
		if duplicate.ID == primary.ID {
			continue
		}
		if err := tx.ObjectDeletionJob.DeleteOneID(duplicate.ID).Exec(ctx); err != nil {
			return fmt.Errorf("merge configured object deletion job %s: %w", duplicate.ID, err)
		}
	}
	if err := resetConfiguredCleanupJob(ctx, primary); err != nil {
		return err
	}
	if err := tx.ObjectDeletionJob.DeleteOneID(legacy.ID).Exec(ctx); err != nil {
		return fmt.Errorf("remove legacy object deletion job %s: %w", legacy.ID, err)
	}
	return nil
}

func configuredCleanupJobPrimary(jobs []*repoent.ObjectDeletionJob) *repoent.ObjectDeletionJob {
	for _, job := range jobs {
		if job.State != domaincleanup.StateBlocked {
			return job
		}
	}
	return jobs[0]
}

func resetConfiguredCleanupJob(ctx context.Context, job *repoent.ObjectDeletionJob) error {
	if err := job.Update().SetState(domaincleanup.StatePending).
		ClearNextAttemptAt().ClearCompletedAt().ClearLastErrorCode().ClearLastErrorMessage().Exec(ctx); err != nil {
		return fmt.Errorf("reactivate configured object deletion job %s: %w", job.ID, err)
	}
	return nil
}

func lockLegacyStorageJobs() func(*entsql.Selector) {
	return func(selector *entsql.Selector) {
		if selector.Dialect() == "postgres" {
			selector.ForUpdate()
		}
	}
}

func validateLegacyStorageIdentity(ctx context.Context, tx *repoent.Tx, checkpoint *repoent.MigrationCheckpoint, driver string) error {
	remaining, err := legacyStorageIdentityRowsRemain(ctx, tx, driver)
	if err != nil {
		return err
	}
	update := tx.MigrationCheckpoint.UpdateOneID(checkpoint.ID).ClearAfterTaskID().ClearAfterResultID()
	if remaining {
		update.SetPhase(legacyStoragePhaseJobs).SetCompleted(false)
	} else {
		update.SetPhase(legacyStoragePhaseDone).SetCompleted(true)
	}
	if err := update.Exec(ctx); err != nil {
		return fmt.Errorf("validate legacy storage identity backfill: %w", err)
	}
	return nil
}

func legacyStorageIdentityRowsRemain(ctx context.Context, tx *repoent.Tx, driver string) (bool, error) {
	checks := []func(context.Context) (bool, error){
		func(ctx context.Context) (bool, error) {
			return tx.ImageResult.Query().Where(imageresult.StorageConfigIDIsNil(), legacyResultDriver(driver)).Exist(ctx)
		},
		func(ctx context.Context) (bool, error) {
			return tx.ReferenceAsset.Query().Where(referenceasset.StorageConfigIDIsNil(), legacyAssetDriver(driver)).Exist(ctx)
		},
		func(ctx context.Context) (bool, error) {
			return tx.ImageTask.Query().Where(imagetask.ArtifactStorageConfigIDIsNil(), legacyTaskDriver(driver), legacyArtifactStorageTuple()).Exist(ctx)
		},
		func(ctx context.Context) (bool, error) {
			return tx.ObjectDeletionJob.Query().Where(
				objectdeletionjob.StorageConfigIDIsNil(), legacyJobDriver(driver),
				objectdeletionjob.StateIn(domaincleanup.StatePending, domaincleanup.StateRunning, domaincleanup.StateRetry, domaincleanup.StateBlocked),
			).Exist(ctx)
		},
	}
	for _, check := range checks {
		remaining, err := check(ctx)
		if err != nil || remaining {
			return remaining, err
		}
	}
	return false, nil
}

func normalizeLegacyStorageDriver(driver string) string {
	driver = strings.ToLower(strings.TrimSpace(driver))
	if driver == "" {
		return "local"
	}
	return driver
}

func legacyResultDriver(driver string) func(*entsql.Selector) {
	return legacyDriverSelector(imageresult.FieldStorageDriver, driver)
}

func legacyAssetDriver(driver string) func(*entsql.Selector) {
	return legacyDriverSelector(referenceasset.FieldStorageDriver, driver)
}

func legacyTaskDriver(driver string) func(*entsql.Selector) {
	return legacyDriverSelector(imagetask.FieldArtifactStorageDriver, driver)
}

func legacyJobDriver(driver string) func(*entsql.Selector) {
	return legacyDriverSelector(objectdeletionjob.FieldStorageDriver, driver)
}

func legacyArtifactStorageTuple() predicate.ImageTask {
	return func(selector *entsql.Selector) {
		jsonArrayLength := "json_array_length"
		if selector.Dialect() == "postgres" {
			jsonArrayLength = "jsonb_array_length"
		}
		selector.Where(entsql.Or(
			entsql.ExprP("TRIM("+selector.C(imagetask.FieldArtifactRecoveryStatus)+") <> ''"),
			entsql.And(
				entsql.NotNull(selector.C(imagetask.FieldArtifactRecoveryPayload)),
				entsql.ExprP("TRIM("+selector.C(imagetask.FieldArtifactRecoveryPayload)+") <> ''"),
			),
			entsql.ExprP("TRIM("+selector.C(imagetask.FieldArtifactStorageBucket)+") <> ''"),
			entsql.ExprP("COALESCE("+jsonArrayLength+"("+selector.C(imagetask.FieldArtifactObjectKeys)+"), 0) > 0"),
			entsql.GT(selector.C(imagetask.FieldArtifactStorageVersion), 0),
		))
	}
}

func legacyDriverSelector(fieldName, driver string) func(*entsql.Selector) {
	driver, managed := domainstorageconfig.NormalizeManagedDriver(driver)
	return func(selector *entsql.Selector) {
		if !managed {
			selector.Where(entsql.ExprP("1 = 0"))
			return
		}
		if driver == "local" {
			selector.Where(entsql.Or(entsql.EQ(selector.C(fieldName), driver), entsql.EQ(selector.C(fieldName), "")))
			return
		}
		selector.Where(entsql.EQ(selector.C(fieldName), driver))
	}
}

func boundedImageResults(rows []*repoent.ImageResult, limit int) ([]*repoent.ImageResult, bool) {
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	return rows, hasMore
}

func legacyStorageProgress(checkpoint *repoent.MigrationCheckpoint) LegacyStorageIdentityBackfillProgress {
	if checkpoint == nil {
		return LegacyStorageIdentityBackfillProgress{}
	}
	return LegacyStorageIdentityBackfillProgress{
		Phase: checkpoint.Phase, ProcessedRows: checkpoint.ProcessedRows, Completed: checkpoint.Completed,
	}
}
