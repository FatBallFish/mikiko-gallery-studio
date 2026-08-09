package entstore

import (
	"context"
	"strings"
	"sync"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	domaincleanup "github.com/fatballfish/pic-gallery/internal/domain/objectcleanup"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imageresult"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imagetask"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/objectdeletionjob"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/referenceasset"
)

type ObjectCleanupStore struct {
	client *repoent.Client

	reconcileMu          sync.Mutex
	resultCursor         cleanupSweepCursor
	referenceAssetCursor cleanupSweepCursor
	legacyRecoveryCursor recoverySweepCursor
	nextSingleSweepAsset bool
}

type cleanupSweepCursor struct {
	deletedAt time.Time
	id        uuid.UUID
	valid     bool
}

type recoverySweepCursor struct {
	updatedAt time.Time
	id        uuid.UUID
	valid     bool
}

func NewObjectCleanupStore(client *repoent.Client) *ObjectCleanupStore {
	return &ObjectCleanupStore{client: client}
}

func cleanupIdentity(configID, driver, objectKey string) domaincleanup.Identity {
	return domaincleanup.CanonicalIdentity(domaincleanup.Identity{StorageConfigID: configID, StorageDriver: driver, ObjectKey: objectKey})
}

func enqueueObjectDeletionJob(ctx context.Context, client *repoent.Client, identity domaincleanup.Identity) (*repoent.ObjectDeletionJob, error) {
	identity = domaincleanup.CanonicalIdentity(identity)
	if strings.TrimSpace(identity.ObjectKey) == "" || strings.EqualFold(identity.StorageDriver, "remote") {
		return nil, nil
	}
	query := client.ObjectDeletionJob.Query().Where(
		objectdeletionjob.ObjectKeyEQ(identity.ObjectKey),
		objectdeletionjob.StateIn(domaincleanup.StatePending, domaincleanup.StateRunning, domaincleanup.StateRetry, domaincleanup.StateBlocked),
	)
	if parsed, err := uuid.Parse(identity.StorageConfigID); err == nil {
		query.Where(objectdeletionjob.StorageConfigIDEQ(parsed))
	} else {
		query.Where(
			objectdeletionjob.StorageConfigIDIsNil(),
			objectdeletionjob.StorageDriverEQ(defaultString(identity.StorageDriver, "local")),
			objectdeletionjob.BucketEQ(identity.Bucket),
		)
	}
	existing, err := query.First(ctx)
	if err == nil {
		return existing.Update().SetState(domaincleanup.StatePending).ClearNextAttemptAt().ClearCompletedAt().ClearLastErrorCode().ClearLastErrorMessage().Save(ctx)
	}
	if !repoent.IsNotFound(err) {
		return nil, err
	}
	create := client.ObjectDeletionJob.Create().
		SetStorageDriver(defaultString(identity.StorageDriver, "local")).
		SetBucket(identity.Bucket).
		SetObjectKey(identity.ObjectKey).
		SetState(domaincleanup.StatePending)
	if parsed, err := uuid.Parse(identity.StorageConfigID); err == nil {
		create.SetStorageConfigID(parsed)
	}
	return create.Save(ctx)
}

func (s *ObjectCleanupStore) Enqueue(ctx context.Context, identity domaincleanup.Identity) (domaincleanup.Job, error) {
	entity, err := enqueueObjectDeletionJob(ctx, s.client, identity)
	if err != nil {
		return domaincleanup.Job{}, err
	}
	if entity == nil {
		return domaincleanup.Job{}, nil
	}
	return mapCleanupJob(entity), nil
}

func (s *ObjectCleanupStore) Claim(ctx context.Context, now time.Time) (domaincleanup.Job, bool, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return domaincleanup.Job{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	stale := now.Add(-time.Minute)
	entity, err := tx.ObjectDeletionJob.Query().Where(
		objectdeletionjob.Or(
			objectdeletionjob.And(
				objectdeletionjob.StateIn(domaincleanup.StatePending, domaincleanup.StateRetry),
				objectdeletionjob.Or(objectdeletionjob.NextAttemptAtIsNil(), objectdeletionjob.NextAttemptAtLTE(now)),
			),
			objectdeletionjob.And(objectdeletionjob.StateEQ(domaincleanup.StateRunning), objectdeletionjob.UpdatedAtLTE(stale)),
		),
		lockCleanupJobForClaim(),
	).Order(repoent.Asc(objectdeletionjob.FieldCreatedAt)).First(ctx)
	if repoent.IsNotFound(err) {
		return domaincleanup.Job{}, false, nil
	}
	if err != nil {
		return domaincleanup.Job{}, false, err
	}
	entity, err = tx.ObjectDeletionJob.UpdateOneID(entity.ID).SetState(domaincleanup.StateRunning).AddAttemptCount(1).ClearNextAttemptAt().Save(ctx)
	if err != nil {
		return domaincleanup.Job{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domaincleanup.Job{}, false, err
	}
	return mapCleanupJob(entity), true, nil
}

func lockCleanupJobForClaim() func(*entsql.Selector) {
	return func(selector *entsql.Selector) {
		if selector.Dialect() == "postgres" {
			selector.ForUpdate(entsql.WithLockAction(entsql.SkipLocked))
		}
	}
}

func (s *ObjectCleanupStore) DeleteIfUnreferenced(ctx context.Context, job domaincleanup.Job, deleteFn func() error) (bool, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	id, err := uuid.Parse(job.ID)
	if err != nil {
		return false, err
	}
	if _, err := tx.ObjectDeletionJob.Query().Where(objectdeletionjob.IDEQ(id), lockCleanupJobForClaim()).Only(ctx); err != nil {
		return false, err
	}
	live, err := hasLiveObjectReferences(ctx, tx.Client(), job.Identity)
	if err != nil || live {
		return live, err
	}
	if err := deleteFn(); err != nil {
		return false, err
	}
	return false, tx.Commit()
}

func (s *ObjectCleanupStore) HasLiveReferences(ctx context.Context, identity domaincleanup.Identity) (bool, error) {
	return hasLiveObjectReferences(ctx, s.client, identity)
}

func hasLiveObjectReferences(ctx context.Context, client *repoent.Client, identity domaincleanup.Identity) (bool, error) {
	identity = domaincleanup.CanonicalIdentity(identity)
	resultQuery := client.ImageResult.Query().Where(imageresult.DeletedAtIsNil(), imageresult.ObjectKeyEQ(identity.ObjectKey))
	assetQuery := client.ReferenceAsset.Query().Where(referenceasset.DeletedAtIsNil(), referenceasset.StatusNEQ("deleted"), referenceasset.ObjectKeyEQ(identity.ObjectKey))
	if parsed, err := uuid.Parse(identity.StorageConfigID); err == nil {
		resultQuery.Where(imageresult.StorageConfigIDEQ(parsed))
		assetQuery.Where(referenceasset.StorageConfigIDEQ(parsed))
	} else {
		resultQuery.Where(imageresult.StorageConfigIDIsNil(), imageresult.StorageDriverEQ(defaultString(identity.StorageDriver, "local")))
		assetQuery.Where(referenceasset.StorageConfigIDIsNil(), referenceasset.StorageDriverEQ(defaultString(identity.StorageDriver, "local")))
	}
	if exists, err := resultQuery.Exist(ctx); err != nil || exists {
		return exists, err
	}
	if exists, err := assetQuery.Exist(ctx); err != nil || exists {
		return exists, err
	}
	recoveryQuery := client.ImageTask.Query().Where(
		imagetask.DeletedAtIsNil(),
		imagetask.ArtifactRecoveryStatusIn("pending", "persisting", "retry", "running"),
	)
	if parsed, err := uuid.Parse(identity.StorageConfigID); err == nil {
		recoveryQuery.Where(imagetask.ArtifactStorageConfigIDEQ(parsed))
	} else {
		recoveryQuery.Where(imagetask.ArtifactStorageConfigIDIsNil())
	}
	recoveries, err := recoveryQuery.All(ctx)
	if err != nil {
		return false, err
	}
	for _, recovery := range recoveries {
		if artifactRecoveryReferencesIdentity(recovery, identity) {
			return true, nil
		}
	}
	return false, nil
}

func artifactRecoveryReferencesIdentity(recovery *repoent.ImageTask, identity domaincleanup.Identity) bool {
	if recovery == nil {
		return false
	}
	if len(recovery.ArtifactObjectKeys) == 0 {
		// Legacy recoveries without persisted keys remain conservative until
		// reconciliation backfills them or marks the envelope unrecoverable.
		return true
	}
	identity = domaincleanup.CanonicalIdentity(identity)
	if identity.StorageConfigID == "" {
		if !strings.EqualFold(defaultString(recovery.ArtifactStorageDriver, "local"), defaultString(identity.StorageDriver, "local")) ||
			strings.TrimSpace(recovery.ArtifactStorageBucket) != identity.Bucket {
			return false
		}
	}
	objectKey := strings.TrimSpace(identity.ObjectKey)
	for _, candidate := range recovery.ArtifactObjectKeys {
		if strings.TrimSpace(candidate) == objectKey {
			return true
		}
	}
	return false
}

func (s *ObjectCleanupStore) MarkDone(ctx context.Context, id string) error {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	return s.client.ObjectDeletionJob.Update().
		Where(objectdeletionjob.IDEQ(parsed), objectdeletionjob.StateEQ(domaincleanup.StateRunning)).
		SetState(domaincleanup.StateDone).
		SetCompletedAt(now).
		ClearNextAttemptAt().
		ClearLastErrorCode().
		ClearLastErrorMessage().
		Exec(ctx)
}
func (s *ObjectCleanupStore) MarkBlocked(ctx context.Context, id, code string) error {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	return s.client.ObjectDeletionJob.Update().
		Where(objectdeletionjob.IDEQ(parsed), objectdeletionjob.StateEQ(domaincleanup.StateRunning)).
		SetState(domaincleanup.StateBlocked).
		SetLastErrorCode(cleanupError(code, 64)).
		ClearLastErrorMessage().
		Exec(ctx)
}
func (s *ObjectCleanupStore) MarkRetry(ctx context.Context, id string, next time.Time, code, message string) error {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	return s.client.ObjectDeletionJob.Update().
		Where(objectdeletionjob.IDEQ(parsed), objectdeletionjob.StateEQ(domaincleanup.StateRunning)).
		SetState(domaincleanup.StateRetry).
		SetNextAttemptAt(next).
		SetLastErrorCode(cleanupError(code, 64)).
		SetLastErrorMessage(cleanupError(message, 160)).
		Exec(ctx)
}
func cleanupError(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

func (s *ObjectCleanupStore) Reconcile(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()
	if err := s.reconcileLegacyArtifactRecoveries(ctx, limit); err != nil {
		return 0, err
	}

	resultLimit, assetLimit := limit/2, limit-limit/2
	if limit == 1 {
		if s.nextSingleSweepAsset {
			resultLimit, assetLimit = 0, 1
		} else {
			resultLimit, assetLimit = 1, 0
		}
		s.nextSingleSweepAsset = !s.nextSingleSweepAsset
	}
	resultCount, err := s.reconcileDeletedResults(ctx, resultLimit)
	if err != nil {
		return 0, err
	}
	assetCount, err := s.reconcileDeletedReferenceAssets(ctx, assetLimit)
	if err != nil {
		return resultCount, err
	}
	return resultCount + assetCount, nil
}

func (s *ObjectCleanupStore) reconcileLegacyArtifactRecoveries(ctx context.Context, limit int) error {
	query := s.client.ImageTask.Query().Where(
		imagetask.DeletedAtIsNil(),
		imagetask.ArtifactRecoveryStatusIn("pending", "persisting", "retry", "running"),
	)
	if s.legacyRecoveryCursor.valid {
		query.Where(imagetask.Or(
			imagetask.UpdatedAtGT(s.legacyRecoveryCursor.updatedAt),
			imagetask.And(imagetask.UpdatedAtEQ(s.legacyRecoveryCursor.updatedAt), imagetask.IDGT(s.legacyRecoveryCursor.id)),
		))
	}
	rows, err := query.Order(repoent.Asc(imagetask.FieldUpdatedAt), repoent.Asc(imagetask.FieldID)).Limit(limit).All(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		s.legacyRecoveryCursor = recoverySweepCursor{updatedAt: row.UpdatedAt, id: row.ID, valid: true}
		if len(row.ArtifactObjectKeys) > 0 {
			continue
		}
		results, err := s.client.ImageResult.Query().Where(imageresult.TaskIDEQ(row.ID)).
			Order(repoent.Asc(imageresult.FieldCreatedAt), repoent.Asc(imageresult.FieldID)).
			All(ctx)
		if err != nil {
			return err
		}
		if len(results) > 0 {
			keys := make([]string, 0, len(results))
			for _, result := range results {
				if key := strings.TrimSpace(result.ObjectKey); key != "" {
					keys = append(keys, key)
				}
			}
			if len(keys) > 0 {
				update := s.client.ImageTask.UpdateOneID(row.ID).
					SetArtifactStorageDriver(defaultString(results[0].StorageDriver, "local")).
					SetArtifactStorageBucket("").
					SetArtifactObjectKeys(keys)
				if row.ArtifactStorageConfigID == nil && results[0].StorageConfigID != nil {
					update.SetArtifactStorageConfigID(*results[0].StorageConfigID)
				}
				if err := update.Exec(ctx); err != nil {
					return err
				}
				continue
			}
		}
		if row.ArtifactRecoveryPayload == nil || strings.TrimSpace(*row.ArtifactRecoveryPayload) == "" {
			if err := s.client.ImageTask.UpdateOneID(row.ID).
				SetArtifactRecoveryStatus("unrecoverable").
				SetArtifactLastDiagnostic(map[string]any{
					"code":      "artifact_recovery_identity_unavailable",
					"stage":     "migration",
					"retryable": false,
				}).
				Exec(ctx); err != nil {
				return err
			}
		}
	}
	if len(rows) < limit {
		s.legacyRecoveryCursor = recoverySweepCursor{}
	}
	return nil
}

func (s *ObjectCleanupStore) reconcileDeletedResults(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	query := s.client.ImageResult.Query().Where(imageresult.DeletedAtNotNil())
	if s.resultCursor.valid {
		query.Where(imageresult.Or(
			imageresult.DeletedAtGT(s.resultCursor.deletedAt),
			imageresult.And(imageresult.DeletedAtEQ(s.resultCursor.deletedAt), imageresult.IDGT(s.resultCursor.id)),
		))
	}
	rows, err := query.Order(repoent.Asc(imageresult.FieldDeletedAt), repoent.Asc(imageresult.FieldID)).Limit(limit).All(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, row := range rows {
		s.resultCursor = cleanupSweepCursor{deletedAt: *row.DeletedAt, id: row.ID, valid: true}
		configID := ""
		if row.StorageConfigID != nil {
			configID = row.StorageConfigID.String()
		}
		identity := cleanupIdentity(configID, row.StorageDriver, row.ObjectKey)
		needed, err := s.cleanupJobNeeded(ctx, identity, *row.DeletedAt)
		if err != nil {
			return count, err
		}
		if !needed {
			continue
		}
		if _, err := s.Enqueue(ctx, identity); err != nil {
			return count, err
		}
		count++
	}
	if len(rows) < limit {
		s.resultCursor = cleanupSweepCursor{}
	}
	return count, nil
}

func (s *ObjectCleanupStore) reconcileDeletedReferenceAssets(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	query := s.client.ReferenceAsset.Query().Where(referenceasset.DeletedAtNotNil())
	if s.referenceAssetCursor.valid {
		query.Where(referenceasset.Or(
			referenceasset.DeletedAtGT(s.referenceAssetCursor.deletedAt),
			referenceasset.And(referenceasset.DeletedAtEQ(s.referenceAssetCursor.deletedAt), referenceasset.IDGT(s.referenceAssetCursor.id)),
		))
	}
	rows, err := query.Order(repoent.Asc(referenceasset.FieldDeletedAt), repoent.Asc(referenceasset.FieldID)).Limit(limit).All(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, row := range rows {
		s.referenceAssetCursor = cleanupSweepCursor{deletedAt: *row.DeletedAt, id: row.ID, valid: true}
		configID := ""
		if row.StorageConfigID != nil {
			configID = row.StorageConfigID.String()
		}
		identity := cleanupIdentity(configID, row.StorageDriver, row.ObjectKey)
		needed, err := s.cleanupJobNeeded(ctx, identity, *row.DeletedAt)
		if err != nil {
			return count, err
		}
		if !needed {
			continue
		}
		if _, err := s.Enqueue(ctx, identity); err != nil {
			return count, err
		}
		count++
	}
	if len(rows) < limit {
		s.referenceAssetCursor = cleanupSweepCursor{}
	}
	return count, nil
}

func (s *ObjectCleanupStore) cleanupJobNeeded(ctx context.Context, identity domaincleanup.Identity, deletedAt time.Time) (bool, error) {
	identity = domaincleanup.CanonicalIdentity(identity)
	query := s.client.ObjectDeletionJob.Query().Where(
		objectdeletionjob.ObjectKeyEQ(identity.ObjectKey),
	)
	if parsed, err := uuid.Parse(identity.StorageConfigID); err == nil {
		query.Where(objectdeletionjob.StorageConfigIDEQ(parsed))
	} else {
		query.Where(
			objectdeletionjob.StorageConfigIDIsNil(),
			objectdeletionjob.StorageDriverEQ(defaultString(identity.StorageDriver, "local")),
			objectdeletionjob.BucketEQ(identity.Bucket),
		)
	}
	if active, err := query.Clone().Where(objectdeletionjob.StateIn(
		domaincleanup.StatePending,
		domaincleanup.StateRunning,
		domaincleanup.StateRetry,
		domaincleanup.StateBlocked,
	)).Exist(ctx); err != nil || active {
		return false, err
	}
	done, err := query.Clone().Where(objectdeletionjob.StateEQ(domaincleanup.StateDone)).
		Order(repoent.Desc(objectdeletionjob.FieldCompletedAt), repoent.Desc(objectdeletionjob.FieldUpdatedAt)).
		First(ctx)
	if repoent.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return done.CompletedAt == nil || done.CompletedAt.Before(deletedAt), nil
}

func mapCleanupJob(entity *repoent.ObjectDeletionJob) domaincleanup.Job {
	configID := ""
	if entity.StorageConfigID != nil {
		configID = entity.StorageConfigID.String()
	}
	identity := domaincleanup.CanonicalIdentity(domaincleanup.Identity{
		StorageConfigID: configID,
		StorageDriver:   entity.StorageDriver,
		Bucket:          entity.Bucket,
		ObjectKey:       entity.ObjectKey,
	})
	return domaincleanup.Job{
		ID:               entity.ID.String(),
		Identity:         identity,
		State:            entity.State,
		AttemptCount:     entity.AttemptCount,
		NextAttemptAt:    entity.NextAttemptAt,
		LastErrorCode:    nullableString(entity.LastErrorCode),
		LastErrorMessage: nullableString(entity.LastErrorMessage),
		CreatedAt:        entity.CreatedAt,
		UpdatedAt:        entity.UpdatedAt,
		CompletedAt:      entity.CompletedAt,
	}
}
