package entstore

import (
	"context"
	"strings"
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

type ObjectCleanupStore struct{ client *repoent.Client }

func NewObjectCleanupStore(client *repoent.Client) *ObjectCleanupStore {
	return &ObjectCleanupStore{client: client}
}

func cleanupIdentity(configID, driver, objectKey string) domaincleanup.Identity {
	return domaincleanup.Identity{StorageConfigID: strings.TrimSpace(configID), StorageDriver: strings.ToLower(strings.TrimSpace(driver)), ObjectKey: strings.TrimSpace(objectKey)}
}

func enqueueObjectDeletionJob(ctx context.Context, client *repoent.Client, identity domaincleanup.Identity) (*repoent.ObjectDeletionJob, error) {
	if strings.TrimSpace(identity.ObjectKey) == "" || strings.EqualFold(identity.StorageDriver, "remote") {
		return nil, nil
	}
	query := client.ObjectDeletionJob.Query().Where(
		objectdeletionjob.StorageDriverEQ(defaultString(identity.StorageDriver, "local")),
		objectdeletionjob.BucketEQ(identity.Bucket),
		objectdeletionjob.ObjectKeyEQ(identity.ObjectKey),
		objectdeletionjob.StateIn(domaincleanup.StatePending, domaincleanup.StateRunning, domaincleanup.StateRetry, domaincleanup.StateBlocked),
	)
	if parsed, err := uuid.Parse(identity.StorageConfigID); err == nil {
		query.Where(objectdeletionjob.StorageConfigIDEQ(parsed))
	} else {
		query.Where(objectdeletionjob.StorageConfigIDIsNil())
	}
	existing, err := query.First(ctx)
	if err == nil {
		return existing.Update().SetState(domaincleanup.StatePending).ClearNextAttemptAt().ClearCompletedAt().ClearLastErrorCode().ClearLastErrorMessage().Save(ctx)
	}
	if !repoent.IsNotFound(err) {
		return nil, err
	}
	doneQuery := client.ObjectDeletionJob.Query().Where(
		objectdeletionjob.StorageDriverEQ(defaultString(identity.StorageDriver, "local")),
		objectdeletionjob.BucketEQ(identity.Bucket),
		objectdeletionjob.ObjectKeyEQ(identity.ObjectKey),
		objectdeletionjob.StateEQ(domaincleanup.StateDone),
	)
	if parsed, err := uuid.Parse(identity.StorageConfigID); err == nil {
		doneQuery.Where(objectdeletionjob.StorageConfigIDEQ(parsed))
	} else {
		doneQuery.Where(objectdeletionjob.StorageConfigIDIsNil())
	}
	done, doneErr := doneQuery.Order(repoent.Desc(objectdeletionjob.FieldUpdatedAt)).First(ctx)
	if doneErr == nil {
		return done, nil
	}
	if !repoent.IsNotFound(doneErr) {
		return nil, doneErr
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

func hasLiveObjectReferences(ctx context.Context, client *repoent.Client, identity domaincleanup.Identity) (bool, error) {
	resultQuery := client.ImageResult.Query().Where(imageresult.DeletedAtIsNil(), imageresult.StorageDriverEQ(identity.StorageDriver), imageresult.ObjectKeyEQ(identity.ObjectKey))
	assetQuery := client.ReferenceAsset.Query().Where(referenceasset.DeletedAtIsNil(), referenceasset.StatusNEQ("deleted"), referenceasset.StorageDriverEQ(identity.StorageDriver), referenceasset.ObjectKeyEQ(identity.ObjectKey))
	if parsed, err := uuid.Parse(identity.StorageConfigID); err == nil {
		resultQuery.Where(imageresult.StorageConfigIDEQ(parsed))
		assetQuery.Where(referenceasset.StorageConfigIDEQ(parsed))
	} else {
		resultQuery.Where(imageresult.StorageConfigIDIsNil())
		assetQuery.Where(referenceasset.StorageConfigIDIsNil())
	}
	if exists, err := resultQuery.Exist(ctx); err != nil || exists {
		return exists, err
	}
	if exists, err := assetQuery.Exist(ctx); err != nil || exists {
		return exists, err
	}
	if parsed, err := uuid.Parse(identity.StorageConfigID); err == nil {
		return client.ImageTask.Query().Where(imagetask.DeletedAtIsNil(), imagetask.ArtifactStorageConfigIDEQ(parsed), imagetask.ArtifactRecoveryStatusIn("pending", "retry", "running")).Exist(ctx)
	}
	return client.ImageTask.Query().Where(imagetask.DeletedAtIsNil(), imagetask.ArtifactStorageConfigIDIsNil(), imagetask.ArtifactRecoveryStatusIn("pending", "retry", "running")).Exist(ctx)
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
	results, err := s.client.ImageResult.Query().Where(imageresult.DeletedAtNotNil()).Limit(limit).All(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, row := range results {
		configID := ""
		if row.StorageConfigID != nil {
			configID = row.StorageConfigID.String()
		}
		if _, err := s.Enqueue(ctx, cleanupIdentity(configID, row.StorageDriver, row.ObjectKey)); err != nil {
			return count, err
		}
		count++
	}
	if count >= limit {
		return count, nil
	}
	assets, err := s.client.ReferenceAsset.Query().Where(referenceasset.DeletedAtNotNil()).Limit(limit - count).All(ctx)
	if err != nil {
		return count, err
	}
	for _, row := range assets {
		configID := ""
		if row.StorageConfigID != nil {
			configID = row.StorageConfigID.String()
		}
		if _, err := s.Enqueue(ctx, cleanupIdentity(configID, row.StorageDriver, row.ObjectKey)); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func mapCleanupJob(entity *repoent.ObjectDeletionJob) domaincleanup.Job {
	configID := ""
	if entity.StorageConfigID != nil {
		configID = entity.StorageConfigID.String()
	}
	return domaincleanup.Job{
		ID: entity.ID.String(),
		Identity: domaincleanup.Identity{
			StorageConfigID: configID,
			StorageDriver:   entity.StorageDriver,
			Bucket:          entity.Bucket,
			ObjectKey:       entity.ObjectKey,
		},
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
