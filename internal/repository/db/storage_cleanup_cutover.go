package db

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	domaincleanup "github.com/fatballfish/pic-gallery/internal/domain/objectcleanup"
	domainstorageconfig "github.com/fatballfish/pic-gallery/internal/domain/storageconfig"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/migrationcheckpoint"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/objectdeletionjob"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

const (
	legacyCleanupCutoverMarkerPrefix = "legacy_cleanup_nil_config_cutover_v1"
	legacyCleanupCutoverGlobalMarker = legacyCleanupCutoverMarkerPrefix + ":global"
	legacyCleanupCutoverPreparing    = "preparing"
	legacyCleanupCutoverArming       = "arming"
	legacyCleanupCutoverArmed        = "armed"
	legacyCleanupPrepareAdvisoryKey  = int64(0x4d4753434c45414e)
)

func PrepareLegacyStorageCleanupCutovers(ctx context.Context, client *repoent.Client, drivers []string) error {
	if client == nil {
		return fmt.Errorf("legacy cleanup cutover client is required")
	}
	drivers = managedLegacyStorageDrivers(drivers)
	if len(drivers) == 0 {
		return nil
	}
	return runLegacyStoragePrepareWithRetry(ctx, func() error {
		return prepareLegacyStorageCleanupCutoversOnce(ctx, client, drivers)
	})
}

func prepareLegacyStorageCleanupCutoversOnce(ctx context.Context, client *repoent.Client, drivers []string) error {
	tx, err := client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("start legacy cleanup barrier transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if tx.Client().DialectName() == dialect.Postgres {
		if err := tx.ExecRaw(ctx, "SELECT pg_advisory_xact_lock($1)", legacyCleanupPrepareAdvisoryKey); err != nil {
			return fmt.Errorf("lock legacy cleanup prepare transaction: %w", err)
		}
	}
	global, err := tx.MigrationCheckpoint.Query().Where(
		migrationcheckpoint.NameEQ(legacyCleanupCutoverGlobalMarker),
		lockLegacyStorageCheckpoint(),
	).Only(ctx)
	if err != nil && !repoent.IsNotFound(err) {
		return fmt.Errorf("lock legacy cleanup global marker: %w", err)
	}
	if err := installLegacyCleanupWriteBarrier(ctx, tx); err != nil {
		return err
	}
	if repoent.IsNotFound(err) {
		global, err = tx.MigrationCheckpoint.Create().
			SetName(legacyCleanupCutoverGlobalMarker).
			SetPhase(legacyCleanupCutoverPreparing).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("create legacy cleanup global marker: %w", err)
		}
	}
	_ = global
	for _, driver := range drivers {
		name := legacyCleanupCutoverMarkerName(driver)
		exists, err := tx.MigrationCheckpoint.Query().Where(migrationcheckpoint.NameEQ(name)).Exist(ctx)
		if err != nil {
			return fmt.Errorf("inspect legacy cleanup %s marker: %w", driver, err)
		}
		if exists {
			continue
		}
		if _, err := tx.MigrationCheckpoint.Create().SetName(name).SetPhase(legacyCleanupCutoverPreparing).Save(ctx); err != nil {
			return fmt.Errorf("create legacy cleanup %s marker: %w", driver, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit legacy cleanup barrier transaction: %w", err)
	}
	return nil
}

func runLegacyStoragePrepareWithRetry(ctx context.Context, run func() error) error {
	var err error
	for attempt := 0; attempt < legacyStorageBatchRetryLimit; attempt++ {
		err = run()
		if err == nil || !isLegacyStoragePrepareRetryable(err) {
			return err
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
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}

func isLegacyStoragePrepareRetryable(err error) bool {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "database is locked") || strings.Contains(message, "database table is locked") ||
		strings.Contains(message, "unique constraint failed: migration_checkpoints.name") {
		return true
	}
	var postgresErr *pq.Error
	if errors.As(err, &postgresErr) {
		switch postgresErr.Code {
		case "23505", "40001", "40P01", "42710":
			return true
		}
	}
	return false
}

func managedLegacyStorageDrivers(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if driver, ok := domainstorageconfig.NormalizeManagedDriver(value); ok {
			set[driver] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for driver := range set {
		result = append(result, driver)
	}
	sort.Strings(result)
	return result
}

func legacyCleanupCutoverMarkerName(driver string) string {
	return legacyCleanupCutoverMarkerPrefix + ":" + normalizeLegacyStorageDriver(driver)
}

func installLegacyCleanupWriteBarrier(ctx context.Context, tx *repoent.Tx) error {
	switch tx.Client().DialectName() {
	case dialect.Postgres:
		if err := tx.ExecRaw(ctx, postgresLegacyCleanupBarrierSQL); err != nil {
			return fmt.Errorf("install PostgreSQL legacy cleanup barrier: %w", err)
		}
	case dialect.SQLite:
		for _, statement := range sqliteLegacyCleanupBarrierSQL {
			if err := tx.ExecRaw(ctx, statement); err != nil {
				return fmt.Errorf("install SQLite legacy cleanup barrier: %w", err)
			}
		}
	default:
		return fmt.Errorf("unsupported legacy cleanup barrier dialect %q", tx.Client().DialectName())
	}
	return nil
}

const postgresLegacyCleanupBarrierSQL = `
CREATE OR REPLACE FUNCTION enforce_legacy_cleanup_identity_cutover() RETURNS trigger AS $$
DECLARE
    normalized_driver text;
    marker_completed boolean;
BEGIN
    normalized_driver := lower(btrim(COALESCE(NULLIF(NEW.storage_driver, ''), 'local')));
    IF NEW.storage_config_id IS NULL
       AND normalized_driver IN ('local', 's3')
       AND (TG_OP = 'INSERT' OR NEW.state IN ('pending', 'running', 'retry', 'blocked')) THEN
        PERFORM 1 FROM migration_checkpoints
         WHERE name = 'legacy_cleanup_nil_config_cutover_v1:global'
         FOR SHARE;
        SELECT completed INTO marker_completed
          FROM migration_checkpoints
         WHERE name = 'legacy_cleanup_nil_config_cutover_v1:' || normalized_driver
         FOR SHARE;
        IF COALESCE(marker_completed, false) THEN
            RAISE EXCEPTION 'legacy managed cleanup identity cutover is armed' USING ERRCODE = '23514';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger
         WHERE tgname = 'enforce_legacy_cleanup_identity_cutover'
           AND tgrelid = 'object_deletion_jobs'::regclass
    ) THEN
        CREATE TRIGGER enforce_legacy_cleanup_identity_cutover
        BEFORE INSERT OR UPDATE ON object_deletion_jobs
        FOR EACH ROW EXECUTE FUNCTION enforce_legacy_cleanup_identity_cutover();
    END IF;
END;
$$;`

var sqliteLegacyCleanupBarrierSQL = []string{
	`CREATE TRIGGER IF NOT EXISTS enforce_legacy_cleanup_identity_cutover_insert
	 BEFORE INSERT ON object_deletion_jobs
	 WHEN NEW.storage_config_id IS NULL
	  AND lower(trim(COALESCE(NULLIF(NEW.storage_driver, ''), 'local'))) IN ('local', 's3')
	 BEGIN
	   SELECT CASE WHEN EXISTS (
	     SELECT 1 FROM migration_checkpoints
	      WHERE name = 'legacy_cleanup_nil_config_cutover_v1:' || lower(trim(COALESCE(NULLIF(NEW.storage_driver, ''), 'local')))
	        AND completed = 1
	   ) THEN RAISE(ABORT, 'legacy managed cleanup identity cutover is armed') END;
	 END`,
	`CREATE TRIGGER IF NOT EXISTS enforce_legacy_cleanup_identity_cutover_update
	 BEFORE UPDATE ON object_deletion_jobs
	 WHEN NEW.storage_config_id IS NULL
	  AND lower(trim(COALESCE(NULLIF(NEW.storage_driver, ''), 'local'))) IN ('local', 's3')
	  AND NEW.state IN ('pending', 'running', 'retry', 'blocked')
	 BEGIN
	   SELECT CASE WHEN EXISTS (
	     SELECT 1 FROM migration_checkpoints
	      WHERE name = 'legacy_cleanup_nil_config_cutover_v1:' || lower(trim(COALESCE(NULLIF(NEW.storage_driver, ''), 'local')))
	        AND completed = 1
	   ) THEN RAISE(ABORT, 'legacy managed cleanup identity cutover is armed') END;
	 END`,
}

func finalizeLegacyStorageJobCutover(ctx context.Context, tx *repoent.Tx, checkpointID int, driver string, configID uuid.UUID) (int, error) {
	marker, err := tx.MigrationCheckpoint.Query().Where(
		migrationcheckpoint.NameEQ(legacyCleanupCutoverMarkerName(driver)),
		lockLegacyStorageCheckpoint(),
	).Only(ctx)
	if err != nil {
		return 0, fmt.Errorf("lock legacy cleanup cutover marker: %w", err)
	}
	if err := tx.MigrationCheckpoint.UpdateOneID(marker.ID).SetPhase(legacyCleanupCutoverArming).SetCompleted(false).Exec(ctx); err != nil {
		return 0, fmt.Errorf("begin legacy cleanup cutover: %w", err)
	}
	rows, err := tx.ObjectDeletionJob.Query().Where(
		objectdeletionjob.StorageConfigIDIsNil(),
		legacyJobDriver(driver),
		objectdeletionjob.StateIn(domaincleanup.StatePending, domaincleanup.StateRunning, domaincleanup.StateRetry, domaincleanup.StateBlocked),
		lockLegacyStorageJobs(),
	).Order(repoent.Asc(objectdeletionjob.FieldID)).All(ctx)
	if err != nil {
		return 0, fmt.Errorf("rescan legacy cleanup jobs at cutover: %w", err)
	}
	updated := 0
	for _, row := range rows {
		if err := replaceLegacyCleanupJob(ctx, tx, row, configID); err != nil {
			return 0, err
		}
		updated++
	}
	remaining, err := tx.ObjectDeletionJob.Query().Where(
		objectdeletionjob.StorageConfigIDIsNil(),
		legacyJobDriver(driver),
		objectdeletionjob.StateIn(domaincleanup.StatePending, domaincleanup.StateRunning, domaincleanup.StateRetry, domaincleanup.StateBlocked),
	).Exist(ctx)
	if err != nil {
		return 0, fmt.Errorf("validate legacy cleanup cutover: %w", err)
	}
	if remaining {
		return 0, fmt.Errorf("legacy cleanup cutover validation found remaining %s jobs", driver)
	}
	if err := tx.MigrationCheckpoint.UpdateOneID(marker.ID).SetPhase(legacyCleanupCutoverArmed).SetCompleted(true).Exec(ctx); err != nil {
		return 0, fmt.Errorf("arm legacy cleanup cutover: %w", err)
	}
	if err := tx.MigrationCheckpoint.UpdateOneID(checkpointID).
		SetPhase(legacyStoragePhaseResults).
		ClearAfterResultID().
		AddProcessedRows(updated).
		Exec(ctx); err != nil {
		return 0, fmt.Errorf("advance legacy storage checkpoint after cleanup cutover: %w", err)
	}
	return updated, nil
}
