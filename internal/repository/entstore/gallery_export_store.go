package entstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/galleryexportjob"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imageresult"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	galleryexportservice "github.com/fatballfish/pic-gallery/internal/service/galleryexport"
)

type GalleryExportStore struct {
	client *repoent.Client
}

func NewGalleryExportStore(client *repoent.Client) *GalleryExportStore {
	return &GalleryExportStore{client: client}
}

func (s *GalleryExportStore) AuthorizeAssets(ctx context.Context, userID int64, projectID string, imageIDs []string) ([]galleryexportservice.Asset, error) {
	projectUUID, err := uuid.Parse(strings.TrimSpace(projectID))
	if err != nil || len(imageIDs) == 0 {
		return nil, repoerr.ErrNotFound
	}
	parsed := make([]uuid.UUID, 0, len(imageIDs))
	for _, imageID := range imageIDs {
		value, err := uuid.Parse(strings.TrimSpace(imageID))
		if err != nil {
			return nil, repoerr.ErrNotFound
		}
		parsed = append(parsed, value)
	}
	entities, err := s.client.ImageResult.Query().Where(
		imageresult.IDIn(parsed...),
		imageresult.UserIDEQ(userID),
		imageresult.ProjectIDEQ(projectUUID),
		imageresult.DeletedAtIsNil(),
	).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query gallery export assets: %w", err)
	}
	if len(entities) != len(parsed) {
		return nil, repoerr.ErrNotFound
	}
	byID := make(map[uuid.UUID]*repoent.ImageResult, len(entities))
	for _, entity := range entities {
		byID[entity.ID] = entity
	}
	assets := make([]galleryexportservice.Asset, 0, len(parsed))
	for _, imageID := range parsed {
		entity := byID[imageID]
		if entity == nil {
			return nil, repoerr.ErrNotFound
		}
		configID := ""
		if entity.StorageConfigID != nil {
			configID = entity.StorageConfigID.String()
		}
		assets = append(assets, galleryexportservice.Asset{
			ID: imageID.String(), ProjectID: projectID, StorageConfigID: configID,
			StorageDriver: entity.StorageDriver, ObjectKey: entity.ObjectKey, MIMEType: entity.MimeType,
			FileSizeBytes: entity.FileSizeBytes, DisplayName: entity.ImageGroup,
		})
	}
	return assets, nil
}

func (s *GalleryExportStore) CreateJob(ctx context.Context, req galleryexportservice.CreateJobRequest) (galleryexportservice.Job, error) {
	projectID, err := uuid.Parse(strings.TrimSpace(req.ProjectID))
	if err != nil {
		return galleryexportservice.Job{}, repoerr.ErrNotFound
	}
	entity, err := s.client.GalleryExportJob.Create().
		SetUserID(req.UserID).
		SetProjectID(projectID).
		SetImageIds(append([]string(nil), req.ImageIDs...)).
		SetState(galleryexportservice.StateQueued).
		SetEstimatedBytes(req.EstimatedBytes).
		SetLifecycleDeadlineAt(req.LifecycleDeadlineAt.UTC()).
		Save(ctx)
	if err != nil {
		return galleryexportservice.Job{}, fmt.Errorf("create gallery export job: %w", err)
	}
	return mapGalleryExportJob(entity), nil
}

func (s *GalleryExportStore) AcquireNextJob(ctx context.Context, owner string, now time.Time, leaseTTL time.Duration) (galleryexportservice.Job, bool, error) {
	if leaseTTL <= 0 {
		leaseTTL = time.Minute
	}
	now = now.UTC()
	if err := s.expireLifecycleJobs(ctx, now); err != nil {
		return galleryexportservice.Job{}, false, err
	}
	eligible := galleryexportjob.Or(
		galleryexportjob.And(galleryexportjob.StateEQ(galleryexportservice.StateQueued), galleryexportjob.Or(galleryexportjob.NextAttemptAtIsNil(), galleryexportjob.NextAttemptAtLTE(now))),
		galleryexportjob.And(galleryexportjob.StateEQ(galleryexportservice.StateRunning), galleryexportjob.LeaseExpiresAtNotNil(), galleryexportjob.LeaseExpiresAtLTE(now)),
	)
	eligible = galleryexportjob.And(eligible, galleryexportjob.LifecycleDeadlineAtNotNil(), galleryexportjob.LifecycleDeadlineAtGT(now))
	candidate, err := s.client.GalleryExportJob.Query().Where(eligible).Order(repoent.Asc(galleryexportjob.FieldCreatedAt)).First(ctx)
	if repoent.IsNotFound(err) {
		return galleryexportservice.Job{}, false, nil
	}
	if err != nil {
		return galleryexportservice.Job{}, false, fmt.Errorf("query gallery export job: %w", err)
	}
	updated, err := s.client.GalleryExportJob.Update().Where(galleryexportjob.IDEQ(candidate.ID), eligible).
		SetState(galleryexportservice.StateRunning).
		SetLeaseOwner(strings.TrimSpace(owner)).
		SetLeaseExpiresAt(now.Add(leaseTTL)).
		AddAttemptCount(1).
		ClearNextAttemptAt().
		Save(ctx)
	if err != nil {
		return galleryexportservice.Job{}, false, fmt.Errorf("claim gallery export job: %w", err)
	}
	if updated != 1 {
		return galleryexportservice.Job{}, false, nil
	}
	claimed, err := s.client.GalleryExportJob.Get(ctx, candidate.ID)
	if err != nil {
		return galleryexportservice.Job{}, false, fmt.Errorf("reload gallery export job: %w", err)
	}
	return mapGalleryExportJob(claimed), true, nil
}

func (s *GalleryExportStore) RenewJobLease(ctx context.Context, jobID, owner string, attempt int, now time.Time, leaseTTL time.Duration) (bool, error) {
	id, err := uuid.Parse(strings.TrimSpace(jobID))
	if err != nil {
		return false, repoerr.ErrNotFound
	}
	if leaseTTL <= 0 {
		leaseTTL = time.Minute
	}
	updated, err := s.client.GalleryExportJob.Update().Where(
		galleryexportjob.IDEQ(id),
		galleryexportjob.StateEQ(galleryexportservice.StateRunning),
		galleryexportjob.LeaseOwnerEQ(strings.TrimSpace(owner)),
		galleryexportjob.AttemptCountEQ(attempt),
		galleryexportjob.LifecycleDeadlineAtNotNil(),
		galleryexportjob.LifecycleDeadlineAtGT(now.UTC()),
	).SetLeaseExpiresAt(now.UTC().Add(leaseTTL)).Save(ctx)
	if err != nil {
		return false, fmt.Errorf("renew gallery export lease: %w", err)
	}
	return updated == 1, nil
}

func (s *GalleryExportStore) GetJob(ctx context.Context, userID int64, jobID string, now time.Time) (galleryexportservice.Job, error) {
	id, err := uuid.Parse(strings.TrimSpace(jobID))
	if err != nil {
		return galleryexportservice.Job{}, repoerr.ErrNotFound
	}
	_, err = s.client.GalleryExportJob.Update().Where(
		galleryexportjob.IDEQ(id), galleryexportjob.UserIDEQ(userID),
		galleryexportjob.StateIn(galleryexportservice.StateQueued, galleryexportservice.StateRunning),
		galleryexportjob.LifecycleDeadlineAtNotNil(), galleryexportjob.LifecycleDeadlineAtLTE(now.UTC()),
	).SetState(galleryexportservice.StateFailed).
		ClearLeaseOwner().ClearLeaseExpiresAt().ClearNextAttemptAt().
		SetLastErrorCode(galleryexportservice.ErrorLifecycleDeadlineExceeded).
		SetLastErrorMessage("gallery export lifecycle deadline exceeded").Save(ctx)
	if err != nil {
		return galleryexportservice.Job{}, fmt.Errorf("expire gallery export job status: %w", err)
	}
	entity, err := s.client.GalleryExportJob.Query().Where(galleryexportjob.IDEQ(id), galleryexportjob.UserIDEQ(userID)).Only(ctx)
	if repoent.IsNotFound(err) {
		return galleryexportservice.Job{}, repoerr.ErrNotFound
	}
	if err != nil {
		return galleryexportservice.Job{}, fmt.Errorf("get gallery export job: %w", err)
	}
	return mapGalleryExportJob(entity), nil
}

func (s *GalleryExportStore) CompleteJob(ctx context.Context, req galleryexportservice.CompleteJobRequest) (galleryexportservice.Job, error) {
	id, err := uuid.Parse(req.JobID)
	if err != nil {
		return galleryexportservice.Job{}, repoerr.ErrNotFound
	}
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return galleryexportservice.Job{}, fmt.Errorf("start gallery export completion: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	update := tx.GalleryExportJob.Update().Where(
		galleryexportjob.IDEQ(id), galleryexportjob.StateEQ(galleryexportservice.StateRunning),
		galleryexportjob.LeaseOwnerEQ(strings.TrimSpace(req.Owner)), galleryexportjob.AttemptCountEQ(req.AttemptCount),
		galleryexportjob.LifecycleDeadlineAtNotNil(), galleryexportjob.LifecycleDeadlineAtGT(req.CompletedAt.UTC()),
	).SetState(galleryexportservice.StateSucceeded).
		SetStorageDriver(strings.TrimSpace(req.StorageDriver)).SetBucket(strings.TrimSpace(req.Bucket)).SetObjectKey(strings.TrimSpace(req.ObjectKey)).
		SetArchiveSizeBytes(req.ArchiveSizeBytes).SetExpiresAt(req.ExpiresAt.UTC()).ClearLeaseOwner().ClearLeaseExpiresAt().ClearLastErrorCode().ClearLastErrorMessage()
	if configID, parseErr := uuid.Parse(strings.TrimSpace(req.StorageConfigID)); parseErr == nil {
		update.SetStorageConfigID(configID)
	} else {
		update.ClearStorageConfigID()
	}
	updated, err := update.Save(ctx)
	if err != nil {
		return galleryexportservice.Job{}, fmt.Errorf("complete gallery export job: %w", err)
	}
	if updated != 1 {
		return galleryexportservice.Job{}, repoerr.ErrConflict
	}
	configID := ""
	if parsed, parseErr := uuid.Parse(strings.TrimSpace(req.StorageConfigID)); parseErr == nil {
		configID = parsed.String()
	}
	if _, err := enqueueObjectDeletionJob(ctx, tx.Client(), cleanupIdentity(configID, req.StorageDriver, req.ObjectKey)); err != nil {
		return galleryexportservice.Job{}, fmt.Errorf("enqueue gallery export cleanup: %w", err)
	}
	entity, err := tx.GalleryExportJob.Get(ctx, id)
	if err != nil {
		return galleryexportservice.Job{}, fmt.Errorf("reload completed gallery export: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return galleryexportservice.Job{}, fmt.Errorf("commit gallery export completion: %w", err)
	}
	return mapGalleryExportJob(entity), nil
}

func (s *GalleryExportStore) FailJob(ctx context.Context, req galleryexportservice.FailJobRequest) error {
	id, err := uuid.Parse(req.Job.ID)
	if err != nil {
		return repoerr.ErrNotFound
	}
	update := s.client.GalleryExportJob.Update().Where(
		galleryexportjob.IDEQ(id), galleryexportjob.StateEQ(galleryexportservice.StateRunning),
		galleryexportjob.LeaseOwnerEQ(req.Job.LeaseOwner), galleryexportjob.AttemptCountEQ(req.Job.AttemptCount),
	).ClearLeaseOwner().ClearLeaseExpiresAt().SetLastErrorCode(limitGalleryExportError(req.Code, 64)).SetLastErrorMessage(limitGalleryExportError(req.Message, 512))
	switch req.Disposition {
	case galleryexportservice.FailureTerminal:
		update.SetState(galleryexportservice.StateFailed).ClearNextAttemptAt()
	case galleryexportservice.FailureRetryable:
		nextAttemptAt := req.FailedAt.UTC().Add(time.Duration(req.Job.AttemptCount) * time.Minute)
		if req.Job.DeadlineAt != nil && !nextAttemptAt.Before(req.Job.DeadlineAt.UTC()) {
			update.SetState(galleryexportservice.StateFailed).ClearNextAttemptAt().
				SetLastErrorCode(galleryexportservice.ErrorLifecycleDeadlineExceeded).
				SetLastErrorMessage("gallery export lifecycle deadline exceeded")
		} else if req.Job.AttemptCount >= 3 {
			update.SetState(galleryexportservice.StateFailed).ClearNextAttemptAt()
		} else {
			update.SetState(galleryexportservice.StateQueued).SetNextAttemptAt(nextAttemptAt)
		}
	default:
		return fmt.Errorf("fail gallery export job: invalid failure disposition %q", req.Disposition)
	}
	updated, err := update.Save(ctx)
	if err != nil {
		return fmt.Errorf("fail gallery export job: %w", err)
	}
	if updated != 1 {
		return repoerr.ErrConflict
	}
	return nil
}

func (s *GalleryExportStore) expireLifecycleJobs(ctx context.Context, now time.Time) error {
	_, err := s.client.GalleryExportJob.Update().Where(
		galleryexportjob.StateIn(galleryexportservice.StateQueued, galleryexportservice.StateRunning),
		galleryexportjob.LifecycleDeadlineAtNotNil(), galleryexportjob.LifecycleDeadlineAtLTE(now.UTC()),
	).SetState(galleryexportservice.StateFailed).
		ClearLeaseOwner().ClearLeaseExpiresAt().ClearNextAttemptAt().
		SetLastErrorCode(galleryexportservice.ErrorLifecycleDeadlineExceeded).
		SetLastErrorMessage("gallery export lifecycle deadline exceeded").Save(ctx)
	if err != nil {
		return fmt.Errorf("expire gallery export lifecycle jobs: %w", err)
	}
	return nil
}

func (s *GalleryExportStore) ExpireReady(ctx context.Context, now time.Time, limit int) (int, error) {
	if limit <= 0 {
		limit = 25
	}
	entities, err := s.client.GalleryExportJob.Query().Where(
		galleryexportjob.StateEQ(galleryexportservice.StateSucceeded), galleryexportjob.ExpiresAtNotNil(), galleryexportjob.ExpiresAtLTE(now.UTC()),
	).Order(repoent.Asc(galleryexportjob.FieldExpiresAt)).Limit(limit).All(ctx)
	if err != nil {
		return 0, fmt.Errorf("query expired gallery exports: %w", err)
	}
	expired := 0
	for _, entity := range entities {
		tx, err := s.client.Tx(ctx)
		if err != nil {
			return expired, fmt.Errorf("start gallery export expiry: %w", err)
		}
		updated, err := tx.GalleryExportJob.Update().Where(
			galleryexportjob.IDEQ(entity.ID), galleryexportjob.StateEQ(galleryexportservice.StateSucceeded), galleryexportjob.ExpiresAtLTE(now.UTC()),
		).SetState(galleryexportservice.StateExpired).Save(ctx)
		if err != nil || updated != 1 {
			_ = tx.Rollback()
			if err != nil {
				return expired, fmt.Errorf("expire gallery export: %w", err)
			}
			continue
		}
		configID := ""
		if entity.StorageConfigID != nil {
			configID = entity.StorageConfigID.String()
		}
		if _, err := enqueueObjectDeletionJob(ctx, tx.Client(), cleanupIdentity(configID, entity.StorageDriver, entity.ObjectKey)); err != nil {
			_ = tx.Rollback()
			return expired, fmt.Errorf("reactivate gallery export cleanup: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return expired, fmt.Errorf("commit gallery export expiry: %w", err)
		}
		expired++
	}
	return expired, nil
}

func limitGalleryExportError(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

func mapGalleryExportJob(entity *repoent.GalleryExportJob) galleryexportservice.Job {
	if entity == nil {
		return galleryexportservice.Job{}
	}
	return galleryexportservice.Job{
		ID: entity.ID.String(), UserID: entity.UserID, ProjectID: entity.ProjectID.String(), ImageIDs: append([]string(nil), entity.ImageIds...),
		State: entity.State, EstimatedBytes: entity.EstimatedBytes, ArchiveSizeBytes: entity.ArchiveSizeBytes,
		StorageConfigID: optionalUUIDString(entity.StorageConfigID), StorageDriver: entity.StorageDriver, Bucket: entity.Bucket, ObjectKey: entity.ObjectKey,
		AttemptCount: entity.AttemptCount, LeaseOwner: valueOrEmpty(entity.LeaseOwner), LeaseExpiresAt: entity.LeaseExpiresAt,
		DeadlineAt: entity.LifecycleDeadlineAt, ExpiresAt: entity.ExpiresAt, ErrorCode: valueOrEmpty(entity.LastErrorCode), ErrorMessage: valueOrEmpty(entity.LastErrorMessage), CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt,
	}
}

func optionalUUIDString(value *uuid.UUID) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

var _ galleryexportservice.Store = (*GalleryExportStore)(nil)
