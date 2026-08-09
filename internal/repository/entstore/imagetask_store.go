package entstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imageresult"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imagetask"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/predicate"
	projectent "github.com/fatballfish/pic-gallery/internal/repository/ent/project"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/publicimageinteraction"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/publicimagestat"
	entuser "github.com/fatballfish/pic-gallery/internal/repository/ent/user"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
)

type ImageTaskStore struct {
	client *repoent.Client
}

func NewImageTaskStore(client *repoent.Client) *ImageTaskStore {
	return &ImageTaskStore{client: client}
}

func (s *ImageTaskStore) UserConcurrencyLimit(ctx context.Context, userID int64) (int, error) {
	entity, err := s.client.User.Query().Where(entuser.IDEQ(int(userID)), entuser.DeletedAtIsNil()).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return 0, repoerr.ErrNotFound
		}
		return 0, err
	}
	return entity.ConcurrencyLimit, nil
}

func (s *ImageTaskStore) Save(ctx context.Context, task domainimagetask.Task) error {
	taskUUID, err := uuid.Parse(task.ID)
	if err != nil {
		return err
	}

	trace, err := buildProviderTrace(task)
	if err != nil {
		return err
	}
	routingSnapshot, err := buildRoutingSnapshot(task)
	if err != nil {
		return err
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	resultProjectID := task.ProjectID
	entity, err := tx.ImageTask.Query().Where(imagetask.IDEQ(taskUUID), lockImageTaskForWorkerUpdate()).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			if err := createImageTask(ctx, tx, taskUUID, task, trace, routingSnapshot); err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		if err := updateImageTask(ctx, tx, entity, task, trace, routingSnapshot); err != nil {
			return err
		}
		resultProjectID = persistedImageTaskProjectID(entity)
	}

	if _, err := tx.ImageResult.Delete().Where(imageresult.TaskIDEQ(taskUUID)).Exec(ctx); err != nil {
		return err
	}
	for idx, result := range task.Results {
		if err := createImageResult(ctx, tx, taskUUID, task.UserID, resultProjectID, idx, result); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *ImageTaskStore) SaveIfOwned(ctx context.Context, task domainimagetask.Task, owner string, now time.Time) error {
	taskUUID, err := uuid.Parse(task.ID)
	if err != nil {
		return err
	}

	trace, err := buildProviderTrace(task)
	if err != nil {
		return err
	}
	routingSnapshot, err := buildRoutingSnapshot(task)
	if err != nil {
		return err
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	entity, err := tx.ImageTask.Query().Where(imagetask.IDEQ(taskUUID), imagetask.DeletedAtIsNil(), lockImageTaskForWorkerUpdate()).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return repoerr.ErrNotFound
		}
		return err
	}
	if entity.Status != domainimagetask.StatusRunning || entity.LeaseOwner == nil || *entity.LeaseOwner != owner {
		return repoerr.ErrConflict
	}
	if entity.LeaseExpiresAt != nil && entity.LeaseExpiresAt.Before(now) {
		return repoerr.ErrConflict
	}

	affected, err := updateLeaseOwnedImageTask(ctx, tx, entity, task, owner, now, trace, routingSnapshot)
	if err != nil {
		return err
	}
	if affected == 0 {
		return repoerr.ErrConflict
	}

	if _, err := tx.ImageResult.Delete().Where(imageresult.TaskIDEQ(taskUUID)).Exec(ctx); err != nil {
		return err
	}
	for idx, result := range task.Results {
		if err := createImageResult(ctx, tx, taskUUID, entity.UserID, persistedImageTaskProjectID(entity), idx, result); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *ImageTaskStore) SaveTerminalState(ctx context.Context, task domainimagetask.Task, owner string, now time.Time) error {
	taskUUID, err := uuid.Parse(task.ID)
	if err != nil {
		return err
	}

	trace, err := buildProviderTrace(task)
	if err != nil {
		return err
	}
	routingSnapshot, err := buildRoutingSnapshot(task)
	if err != nil {
		return err
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	entity, err := tx.ImageTask.Query().Where(imagetask.IDEQ(taskUUID), imagetask.DeletedAtIsNil(), lockImageTaskForWorkerUpdate()).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return repoerr.ErrNotFound
		}
		return err
	}
	if entity.Status != domainimagetask.StatusRunning || entity.LeaseOwner == nil || *entity.LeaseOwner != owner {
		return repoerr.ErrConflict
	}

	affected, err := updateRecoverableImageTask(ctx, tx, entity, task, owner, now, trace, routingSnapshot)
	if err != nil {
		return err
	}
	if affected == 0 {
		return repoerr.ErrConflict
	}

	if _, err := tx.ImageResult.Delete().Where(imageresult.TaskIDEQ(taskUUID)).Exec(ctx); err != nil {
		return err
	}
	for idx, result := range task.Results {
		if err := createImageResult(ctx, tx, taskUUID, entity.UserID, persistedImageTaskProjectID(entity), idx, result); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *ImageTaskStore) UpdateProgressIfOwned(ctx context.Context, taskID, owner, stage, message string, now time.Time) error {
	taskUUID, err := uuid.Parse(taskID)
	if err != nil {
		return err
	}
	affected, err := s.client.ImageTask.Update().
		Where(
			imagetask.IDEQ(taskUUID),
			imagetask.DeletedAtIsNil(),
			imagetask.StatusEQ(domainimagetask.StatusRunning),
			imagetask.LeaseOwnerEQ(owner),
			imagetask.Or(imagetask.LeaseExpiresAtIsNil(), imagetask.LeaseExpiresAtGTE(now)),
		).
		SetProgressStage(stage).
		SetProgressMessage(message).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return repoerr.ErrConflict
	}
	return nil
}

func (s *ImageTaskStore) GetByID(ctx context.Context, userID int64, taskID string) (domainimagetask.Task, error) {
	taskUUID, err := uuid.Parse(taskID)
	if err != nil {
		return domainimagetask.Task{}, err
	}

	entity, err := s.client.ImageTask.Query().
		Where(imagetask.IDEQ(taskUUID), imagetask.UserIDEQ(userID), imagetask.DeletedAtIsNil()).
		WithProject().
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainimagetask.Task{}, repoerr.ErrNotFound
		}
		return domainimagetask.Task{}, err
	}
	results, err := s.client.ImageResult.Query().
		Where(imageresult.TaskIDEQ(taskUUID), imageresult.UserIDEQ(userID), imageresult.DeletedAtIsNil()).
		Order(repoent.Asc(imageresult.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return domainimagetask.Task{}, err
	}
	return mapImageTaskEntity(entity, results)
}

func (s *ImageTaskStore) GetImageResultByID(ctx context.Context, userID int64, imageID string) (provider.ImageResult, error) {
	imageUUID, err := uuid.Parse(imageID)
	if err != nil {
		return provider.ImageResult{}, repoerr.ErrNotFound
	}
	result, err := s.client.ImageResult.Query().
		Where(imageresult.IDEQ(imageUUID), imageresult.UserIDEQ(userID), imageresult.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return provider.ImageResult{}, repoerr.ErrNotFound
		}
		return provider.ImageResult{}, err
	}
	_, err = s.client.ImageTask.Query().
		Where(imagetask.IDEQ(result.TaskID), imagetask.UserIDEQ(userID), imagetask.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return provider.ImageResult{}, repoerr.ErrNotFound
		}
		return provider.ImageResult{}, err
	}
	return mapImageResultEntity(result), nil
}

func (s *ImageTaskStore) GetImageResultForAdmin(ctx context.Context, imageID string) (provider.ImageResult, error) {
	imageUUID, err := uuid.Parse(imageID)
	if err != nil {
		return provider.ImageResult{}, repoerr.ErrNotFound
	}
	result, err := s.client.ImageResult.Query().
		Where(imageresult.IDEQ(imageUUID), imageresult.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return provider.ImageResult{}, repoerr.ErrNotFound
		}
		return provider.ImageResult{}, err
	}
	_, err = s.client.ImageTask.Query().
		Where(imagetask.IDEQ(result.TaskID), imagetask.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return provider.ImageResult{}, repoerr.ErrNotFound
		}
		return provider.ImageResult{}, err
	}
	return mapImageResultEntity(result), nil
}

func (s *ImageTaskStore) ListByUser(ctx context.Context, userID int64) ([]domainimagetask.Task, error) {
	return s.listByUserProject(ctx, userID, "", 0)
}

func (s *ImageTaskStore) ListByUserProject(ctx context.Context, userID int64, projectID string) ([]domainimagetask.Task, error) {
	return s.listByUserProject(ctx, userID, projectID, 0)
}

func (s *ImageTaskStore) ListRecentByUserProject(ctx context.Context, userID int64, projectID string, limit int) ([]domainimagetask.Task, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.listByUserProject(ctx, userID, projectID, limit)
}

func (s *ImageTaskStore) listByUserProject(ctx context.Context, userID int64, projectID string, limit int) ([]domainimagetask.Task, error) {
	query := s.client.ImageTask.Query().Where(imagetask.UserIDEQ(userID), imagetask.DeletedAtIsNil())
	if projectID = strings.TrimSpace(projectID); projectID != "" {
		parsedProjectID, err := uuid.Parse(projectID)
		if err != nil {
			return nil, repoerr.ErrNotFound
		}
		query.Where(imagetask.ProjectIDEQ(parsedProjectID))
	}
	query.WithProject().Order(repoent.Desc(imagetask.FieldCreatedAt))
	if limit > 0 {
		query.Limit(limit)
	}
	entities, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	if len(entities) == 0 {
		return []domainimagetask.Task{}, nil
	}

	taskIDs := make([]uuid.UUID, 0, len(entities))
	for _, entity := range entities {
		taskIDs = append(taskIDs, entity.ID)
	}
	resultEntities, err := s.client.ImageResult.Query().
		Where(imageresult.UserIDEQ(userID), imageresult.TaskIDIn(taskIDs...), imageresult.DeletedAtIsNil()).
		Order(repoent.Asc(imageresult.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	resultsByTask := map[uuid.UUID][]*repoent.ImageResult{}
	for _, entity := range resultEntities {
		resultsByTask[entity.TaskID] = append(resultsByTask[entity.TaskID], entity)
	}

	list := make([]domainimagetask.Task, 0, len(entities))
	for _, entity := range entities {
		task, err := mapImageTaskEntity(entity, resultsByTask[entity.ID])
		if err != nil {
			return nil, err
		}
		list = append(list, task)
	}
	return list, nil
}

func (s *ImageTaskStore) RequestPublish(ctx context.Context, userID int64, imageID string) (domainimagetask.GalleryImage, error) {
	return s.RequestPublishInProject(ctx, userID, imageID, "")
}

func (s *ImageTaskStore) RequestPublishInProject(ctx context.Context, userID int64, imageID, projectID string) (domainimagetask.GalleryImage, error) {
	imageUUID, err := uuid.Parse(imageID)
	if err != nil {
		return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
	}
	entity, taskEntity, err := s.loadGalleryImageWithTask(ctx, imageUUID)
	if err != nil {
		return domainimagetask.GalleryImage{}, err
	}
	if taskEntity.UserID != userID || (projectID != "" && (entity.ProjectID == nil || entity.ProjectID.String() != projectID)) {
		return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
	}
	currentStatus := defaultString(entity.VisibilityStatus, domainimagetask.VisibilityPrivate)
	update := s.client.ImageResult.Update().Where(imageresult.IDEQ(entity.ID), imageresult.UserIDEQ(userID), imageresult.DeletedAtIsNil(), imageresult.VisibilityStatusEQ(entity.VisibilityStatus))
	if projectID != "" {
		parsed, parseErr := uuid.Parse(projectID)
		if parseErr != nil {
			return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
		}
		update.Where(imageresult.ProjectIDEQ(parsed))
	}
	if currentStatus != domainimagetask.VisibilityPendingReview && currentStatus != domainimagetask.VisibilityApproved {
		update.SetVisibilityStatus(domainimagetask.VisibilityPendingReview).ClearReviewReason().ClearPublishedAt()
	}
	count, err := update.Save(ctx)
	if err != nil {
		return domainimagetask.GalleryImage{}, err
	}
	if count != 1 {
		return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
	}
	updated, taskEntity, err := s.loadGalleryImageWithTask(ctx, imageUUID)
	if err != nil {
		return domainimagetask.GalleryImage{}, err
	}
	return mapGalleryImageEntity(updated, taskEntity), nil
}

func (s *ImageTaskStore) CancelPublish(ctx context.Context, userID int64, imageID string) (domainimagetask.GalleryImage, error) {
	return s.CancelPublishInProject(ctx, userID, imageID, "")
}

func (s *ImageTaskStore) CancelPublishInProject(ctx context.Context, userID int64, imageID, projectID string) (domainimagetask.GalleryImage, error) {
	imageUUID, err := uuid.Parse(imageID)
	if err != nil {
		return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
	}
	predicates := []predicate.ImageResult{
		imageresult.IDEQ(imageUUID),
		imageresult.UserIDEQ(userID),
		imageresult.DeletedAtIsNil(),
		imageresult.VisibilityStatusIn(
			domainimagetask.VisibilityPrivate,
			domainimagetask.VisibilityPendingReview,
			domainimagetask.VisibilityApproved,
		),
	}
	if projectID != "" {
		parsed, parseErr := uuid.Parse(projectID)
		if parseErr != nil {
			return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
		}
		predicates = append(predicates, imageresult.ProjectIDEQ(parsed))
	}
	updated, err := s.client.ImageResult.Update().Where(predicates...).
		SetVisibilityStatus(domainimagetask.VisibilityPrivate).
		ClearReviewReason().
		ClearPublishedAt().
		Save(ctx)
	if err != nil {
		return domainimagetask.GalleryImage{}, err
	}
	if updated == 0 {
		return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
	}
	entity, taskEntity, err := s.loadGalleryImageWithTask(ctx, imageUUID)
	if err != nil {
		return domainimagetask.GalleryImage{}, err
	}
	if taskEntity.UserID != userID {
		return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
	}
	return mapGalleryImageEntity(entity, taskEntity), nil
}

func (s *ImageTaskStore) SetImageGroup(ctx context.Context, userID int64, imageID, imageGroup string) (domainimagetask.GalleryImage, error) {
	return s.SetImageGroupInProject(ctx, userID, imageID, "", imageGroup)
}

func (s *ImageTaskStore) SetImageGroupInProject(ctx context.Context, userID int64, imageID, projectID, imageGroup string) (domainimagetask.GalleryImage, error) {
	imageUUID, err := uuid.Parse(imageID)
	if err != nil {
		return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
	}
	imageGroup = strings.TrimSpace(imageGroup)
	predicates := []predicate.ImageResult{imageresult.IDEQ(imageUUID), imageresult.UserIDEQ(userID), imageresult.DeletedAtIsNil()}
	if projectID != "" {
		parsed, parseErr := uuid.Parse(projectID)
		if parseErr != nil {
			return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
		}
		predicates = append(predicates, imageresult.ProjectIDEQ(parsed))
	}
	count, err := s.client.ImageResult.Update().Where(predicates...).SetImageGroup(imageGroup).Save(ctx)
	if err != nil {
		return domainimagetask.GalleryImage{}, err
	}
	if count != 1 {
		return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
	}
	updated, taskEntity, err := s.loadGalleryImageWithTask(ctx, imageUUID)
	if err != nil {
		return domainimagetask.GalleryImage{}, err
	}
	return mapGalleryImageEntity(updated, taskEntity), nil
}

func (s *ImageTaskStore) TransferImageProject(ctx context.Context, userID int64, imageID, sourceProjectID, targetProjectID string) (domainimagetask.GalleryImage, error) {
	imageUUID, err := uuid.Parse(imageID)
	if err != nil {
		return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
	}
	sourceID, err := uuid.Parse(sourceProjectID)
	if err != nil {
		return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
	}
	targetID, err := uuid.Parse(targetProjectID)
	if err != nil {
		return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
	}
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return domainimagetask.GalleryImage{}, fmt.Errorf("start gallery project transfer: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Project.Query().Where(
		activeOwnedProject(userID), projectent.IDEQ(targetID), lockProjectForGalleryTransfer(),
	).Only(ctx); err != nil {
		if repoent.IsNotFound(err) {
			return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
		}
		return domainimagetask.GalleryImage{}, fmt.Errorf("lock gallery transfer target: %w", err)
	}
	updated, err := tx.ImageResult.Update().Where(
		imageresult.IDEQ(imageUUID), imageresult.UserIDEQ(userID), imageresult.ProjectIDEQ(sourceID), imageresult.DeletedAtIsNil(),
	).SetProjectID(targetID).Save(ctx)
	if err != nil {
		return domainimagetask.GalleryImage{}, err
	}
	if updated != 1 {
		return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
	}
	entity, err := tx.ImageResult.Query().Where(imageresult.IDEQ(imageUUID), imageresult.DeletedAtIsNil()).Only(ctx)
	if err != nil {
		return domainimagetask.GalleryImage{}, err
	}
	taskEntity, err := tx.ImageTask.Get(ctx, entity.TaskID)
	if err != nil {
		return domainimagetask.GalleryImage{}, err
	}
	result := mapGalleryImageEntity(entity, taskEntity)
	if err := tx.Commit(); err != nil {
		return domainimagetask.GalleryImage{}, fmt.Errorf("commit gallery project transfer: %w", err)
	}
	return result, nil
}

func lockProjectForGalleryTransfer() predicate.Project {
	return func(selector *entsql.Selector) {
		if selector.Dialect() != dialect.SQLite {
			selector.ForShare()
		}
	}
}

func (s *ImageTaskStore) ReviewImage(ctx context.Context, imageID, nextStatus, reviewReason string, publishedAt *time.Time) (domainimagetask.GalleryImage, error) {
	imageUUID, err := uuid.Parse(imageID)
	if err != nil {
		return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
	}
	entity, taskEntity, err := s.loadGalleryImageWithTask(ctx, imageUUID)
	if err != nil {
		return domainimagetask.GalleryImage{}, err
	}
	currentStatus := defaultString(entity.VisibilityStatus, domainimagetask.VisibilityPrivate)
	if currentStatus == nextStatus {
		return mapGalleryImageEntity(entity, taskEntity), nil
	}
	if !canTransitionVisibility(currentStatus, nextStatus) {
		return domainimagetask.GalleryImage{}, repoerr.ErrConflict
	}

	update := s.client.ImageResult.UpdateOneID(entity.ID).SetVisibilityStatus(nextStatus)
	switch nextStatus {
	case domainimagetask.VisibilityApproved:
		update.ClearReviewReason()
		if publishedAt != nil {
			update.SetPublishedAt(*publishedAt)
		}
	case domainimagetask.VisibilityRejected, domainimagetask.VisibilityUnpublished:
		update.SetReviewReason(strings.TrimSpace(reviewReason))
		update.ClearPublishedAt()
	default:
		update.ClearPublishedAt()
	}
	updated, err := update.Save(ctx)
	if err != nil {
		return domainimagetask.GalleryImage{}, err
	}
	return mapGalleryImageEntity(updated, taskEntity), nil
}

func (s *ImageTaskStore) ReviewImageInProject(ctx context.Context, userID int64, imageID, projectID, nextStatus, reviewReason string, publishedAt *time.Time) (domainimagetask.GalleryImage, error) {
	imageUUID, err := uuid.Parse(imageID)
	if err != nil {
		return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
	}
	projectUUID, err := uuid.Parse(projectID)
	if err != nil {
		return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
	}
	update := s.client.ImageResult.Update().Where(imageresult.IDEQ(imageUUID), imageresult.UserIDEQ(userID), imageresult.ProjectIDEQ(projectUUID), imageresult.DeletedAtIsNil()).SetVisibilityStatus(nextStatus)
	if nextStatus == domainimagetask.VisibilityRejected || nextStatus == domainimagetask.VisibilityUnpublished {
		update.SetReviewReason(strings.TrimSpace(reviewReason)).ClearPublishedAt()
	} else if nextStatus == domainimagetask.VisibilityApproved {
		update.ClearReviewReason()
		if publishedAt != nil {
			update.SetPublishedAt(*publishedAt)
		}
	}
	count, err := update.Save(ctx)
	if err != nil {
		return domainimagetask.GalleryImage{}, err
	}
	if count != 1 {
		return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
	}
	entity, task, err := s.loadGalleryImageWithTask(ctx, imageUUID)
	if err != nil {
		return domainimagetask.GalleryImage{}, err
	}
	return mapGalleryImageEntity(entity, task), nil
}

func (s *ImageTaskStore) DeleteImageResult(ctx context.Context, userID int64, imageID string) (provider.ImageResult, error) {
	return s.DeleteImageResultInProject(ctx, userID, imageID, "")
}

func (s *ImageTaskStore) DeleteImageResultInProject(ctx context.Context, userID int64, imageID, projectID string) (provider.ImageResult, error) {
	imageUUID, err := uuid.Parse(imageID)
	if err != nil {
		return provider.ImageResult{}, repoerr.ErrNotFound
	}
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return provider.ImageResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	writePredicates := []predicate.ImageResult{imageresult.IDEQ(imageUUID), imageresult.UserIDEQ(userID), imageresult.DeletedAtIsNil()}
	if projectID != "" {
		parsed, parseErr := uuid.Parse(projectID)
		if parseErr != nil {
			return provider.ImageResult{}, repoerr.ErrNotFound
		}
		writePredicates = append(writePredicates, imageresult.ProjectIDEQ(parsed))
	}
	queryPredicates := append(append([]predicate.ImageResult(nil), writePredicates...), lockImageResultForCleanup())
	entity, err := tx.ImageResult.Query().
		Where(queryPredicates...).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return provider.ImageResult{}, repoerr.ErrNotFound
		}
		return provider.ImageResult{}, err
	}
	if _, err := tx.ImageTask.Query().
		Where(imagetask.IDEQ(entity.TaskID), imagetask.UserIDEQ(userID), imagetask.DeletedAtIsNil()).
		Only(ctx); err != nil {
		if repoent.IsNotFound(err) {
			return provider.ImageResult{}, repoerr.ErrNotFound
		}
		return provider.ImageResult{}, err
	}
	result := mapImageResultEntity(entity)
	updated, err := tx.ImageResult.Update().Where(writePredicates...).SetDeletedAt(time.Now().UTC()).Save(ctx)
	if err != nil {
		return provider.ImageResult{}, err
	}
	if updated != 1 {
		return provider.ImageResult{}, repoerr.ErrNotFound
	}
	configID := ""
	if entity.StorageConfigID != nil {
		configID = entity.StorageConfigID.String()
	}
	if _, err := enqueueObjectDeletionJob(ctx, tx.Client(), cleanupIdentity(configID, entity.StorageDriver, entity.ObjectKey)); err != nil {
		return provider.ImageResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return provider.ImageResult{}, err
	}
	return result, nil
}

func lockImageResultForCleanup() predicate.ImageResult {
	return predicate.ImageResult(func(selector *entsql.Selector) {
		if selector.Dialect() == dialect.Postgres {
			selector.ForUpdate()
		}
	})
}

func (s *ImageTaskStore) ListGallery(ctx context.Context, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error) {
	page, pageSize := normalizePageBounds(req.Page, req.PageSize)
	query := s.client.ImageResult.Query().
		Where(imageresult.DeletedAtIsNil()).
		Order(repoent.Desc(imageresult.FieldCreatedAt), repoent.Desc(imageresult.FieldID))
	if req.ReviewOnly {
		query.Where(imageresult.VisibilityStatusNEQ(domainimagetask.VisibilityPrivate))
	}
	if status := strings.TrimSpace(req.Status); status != "" {
		query.Where(imageresult.VisibilityStatusEQ(status))
	}
	empty, err := s.applyAdminGalleryFilters(ctx, query, req)
	if err != nil {
		return domainimagetask.GalleryPage{}, err
	}
	if empty {
		return domainimagetask.GalleryPage{Items: []domainimagetask.GalleryImage{}, Page: page, PageSize: pageSize, Total: 0}, nil
	}
	return s.galleryPageFromQuery(ctx, query, page, pageSize)
}

func (s *ImageTaskStore) applyAdminGalleryFilters(ctx context.Context, query *repoent.ImageResultQuery, req domainimagetask.GalleryListRequest) (bool, error) {
	if userQuery := strings.TrimSpace(req.UserQuery); userQuery != "" {
		userPredicates := []predicate.User{
			entuser.EmailContainsFold(userQuery),
			entuser.NicknameContainsFold(userQuery),
		}
		if userID, err := strconv.ParseInt(userQuery, 10, 64); err == nil && userID > 0 && int64(int(userID)) == userID {
			userPredicates = append(userPredicates, entuser.IDEQ(int(userID)))
		}
		userIDs, err := s.client.User.Query().
			Where(entuser.DeletedAtIsNil(), entuser.Or(userPredicates...)).
			IDs(ctx)
		if err != nil {
			return false, err
		}
		if len(userIDs) == 0 {
			return true, nil
		}
		resultUserIDs := make([]int64, 0, len(userIDs))
		for _, userID := range userIDs {
			resultUserIDs = append(resultUserIDs, int64(userID))
		}
		query.Where(imageresult.UserIDIn(resultUserIDs...))
	}

	taskPredicates := make([]predicate.ImageTask, 0, 8)
	if prompt := strings.TrimSpace(req.PromptQuery); prompt != "" {
		taskPredicates = append(taskPredicates, imagetask.PromptContainsFold(prompt))
	}
	if model := strings.TrimSpace(req.ModelQuery); model != "" {
		taskPredicates = append(taskPredicates, imagetask.Or(imagetask.AbstractModelContainsFold(model), imagetask.RouteModelCodeContainsFold(model)))
	}
	if taskType := strings.TrimSpace(req.TaskType); taskType != "" {
		taskPredicates = append(taskPredicates, imagetask.TaskTypeEQ(taskType))
	}
	if baseResolution := strings.TrimSpace(req.BaseResolution); baseResolution != "" {
		taskPredicates = append(taskPredicates, imagetask.BaseResolutionEQ(baseResolution))
	}
	if requestedSize := strings.TrimSpace(req.RequestedSize); requestedSize != "" {
		taskPredicates = append(taskPredicates, imagetask.RequestedSizeEQ(requestedSize))
	}
	if aspectRatio := strings.TrimSpace(req.AspectRatio); aspectRatio != "" {
		taskPredicates = append(taskPredicates, imagetask.AspectRatioEQ(aspectRatio))
	}
	if len(taskPredicates) > 0 {
		taskIDs, err := s.client.ImageTask.Query().
			Where(imagetask.DeletedAtIsNil()).
			Where(taskPredicates...).
			IDs(ctx)
		if err != nil {
			return false, err
		}
		if len(taskIDs) == 0 {
			return true, nil
		}
		query.Where(imageresult.TaskIDIn(taskIDs...))
	}

	if req.Width > 0 {
		query.Where(imageresult.WidthEQ(req.Width))
	}
	if req.Height > 0 {
		query.Where(imageresult.HeightEQ(req.Height))
	}
	if !req.CreatedFrom.IsZero() {
		query.Where(imageresult.CreatedAtGTE(req.CreatedFrom))
	}
	if !req.CreatedTo.IsZero() {
		query.Where(imageresult.CreatedAtLTE(req.CreatedTo))
	}
	if !req.PublishedFrom.IsZero() {
		query.Where(imageresult.PublishedAtGTE(req.PublishedFrom))
	}
	if !req.PublishedTo.IsZero() {
		query.Where(imageresult.PublishedAtLTE(req.PublishedTo))
	}
	return false, nil
}

func (s *ImageTaskStore) ListGalleryByUser(ctx context.Context, userID int64, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error) {
	page, pageSize := normalizePageBounds(req.Page, req.PageSize)
	query := s.client.ImageResult.Query().
		Where(imageresult.UserIDEQ(userID), imageresult.DeletedAtIsNil()).
		Order(repoent.Desc(imageresult.FieldCreatedAt), repoent.Desc(imageresult.FieldID))
	if projectID := strings.TrimSpace(req.ProjectID); projectID != "" {
		parsedProjectID, err := uuid.Parse(projectID)
		if err != nil {
			return domainimagetask.GalleryPage{}, repoerr.ErrNotFound
		}
		query.Where(imageresult.ProjectIDEQ(parsedProjectID))
	}
	if status := strings.TrimSpace(req.Status); status != "" {
		query.Where(imageresult.VisibilityStatusEQ(status))
	}
	return s.galleryPageFromQuery(ctx, query, page, pageSize)
}

func (s *ImageTaskStore) ListPublicGallery(ctx context.Context, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error) {
	page, pageSize := normalizePageBounds(req.Page, req.PageSize)
	query := s.client.ImageResult.Query().
		Where(
			imageresult.DeletedAtIsNil(),
			imageresult.VisibilityStatusEQ(domainimagetask.VisibilityApproved),
		).
		Order(repoent.Desc(imageresult.FieldPublishedAt), repoent.Desc(imageresult.FieldCreatedAt), repoent.Desc(imageresult.FieldID))
	if req.ViewerUserID > 0 && (req.LikedOnly || req.FavoritedOnly) {
		interactionQuery := s.client.PublicImageInteraction.Query().Where(publicimageinteraction.UserIDEQ(req.ViewerUserID))
		if req.LikedOnly {
			interactionQuery.Where(publicimageinteraction.LikedEQ(true))
		}
		if req.FavoritedOnly {
			interactionQuery.Where(publicimageinteraction.FavoritedEQ(true))
		}
		interactions, err := interactionQuery.All(ctx)
		if err != nil {
			return domainimagetask.GalleryPage{}, err
		}
		if len(interactions) == 0 {
			return domainimagetask.GalleryPage{Items: []domainimagetask.GalleryImage{}, Page: page, PageSize: pageSize, Total: 0}, nil
		}
		imageIDs := make([]uuid.UUID, 0, len(interactions))
		for _, interaction := range interactions {
			imageIDs = append(imageIDs, interaction.ImageID)
		}
		query.Where(imageresult.IDIn(imageIDs...))
	}
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return domainimagetask.GalleryPage{}, err
	}
	entities, err := query.All(ctx)
	if err != nil {
		return domainimagetask.GalleryPage{}, err
	}
	items, err := s.galleryImagesFromEntities(ctx, entities, req.ViewerUserID)
	if err != nil {
		return domainimagetask.GalleryPage{}, err
	}
	items = filterPublicGalleryItems(items, req)
	total = len(items)
	if req.Sort == "hot" {
		sort.SliceStable(items, func(i, j int) bool {
			left := publicGalleryHotScore(items[i])
			right := publicGalleryHotScore(items[j])
			if left != right {
				return left > right
			}
			return items[i].CreatedAt.After(items[j].CreatedAt)
		})
	}
	start := (page - 1) * pageSize
	if start >= len(items) {
		items = []domainimagetask.GalleryImage{}
	} else {
		end := start + pageSize
		if end > len(items) {
			end = len(items)
		}
		items = items[start:end]
	}
	return domainimagetask.GalleryPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func filterPublicGalleryItems(items []domainimagetask.GalleryImage, req domainimagetask.GalleryListRequest) []domainimagetask.GalleryImage {
	query := strings.TrimSpace(req.Query)
	routeModelCode := strings.TrimSpace(req.RouteModelCode)
	taskType := strings.TrimSpace(req.TaskType)
	if query == "" && routeModelCode == "" && taskType == "" {
		return items
	}
	filtered := make([]domainimagetask.GalleryImage, 0, len(items))
	for _, item := range items {
		if query != "" && !publicGalleryQueryMatches(item, query) {
			continue
		}
		if routeModelCode != "" && !galleryRouteModelMatches(item, routeModelCode) {
			continue
		}
		if taskType != "" && !strings.EqualFold(item.TaskType, taskType) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func galleryRouteModelMatches(item domainimagetask.GalleryImage, routeModelCode string) bool {
	return strings.EqualFold(item.RouteModelCode, routeModelCode) || (item.RouteModelCode == "" && strings.EqualFold(item.AbstractModel, routeModelCode))
}

func publicGalleryQueryMatches(item domainimagetask.GalleryImage, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	fields := []string{
		item.ID,
		item.PromptExcerpt,
		publicGallerySearchExcerpt(item.Prompt, 24),
		item.RouteModelCode,
		item.AbstractModel,
		item.TaskType,
		item.AuthorName,
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(strings.TrimSpace(field)), query) {
			return true
		}
	}
	return false
}

func publicGallerySearchExcerpt(prompt string, limit int) string {
	prompt = strings.Join(strings.Fields(prompt), " ")
	if limit <= 0 || prompt == "" {
		return ""
	}
	runes := []rune(prompt)
	if len(runes) <= limit {
		visible := len(runes) / 2
		if visible < 1 {
			return "…"
		}
		if visible > limit-1 {
			visible = limit - 1
		}
		return string(runes[:visible]) + "…"
	}
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}

func (s *ImageTaskStore) GetPublicImage(ctx context.Context, imageID string, viewerUserID int64) (domainimagetask.GalleryImage, error) {
	imageUUID, err := uuid.Parse(imageID)
	if err != nil {
		return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
	}
	entity, taskEntity, err := s.loadGalleryImageWithTask(ctx, imageUUID)
	if err != nil {
		return domainimagetask.GalleryImage{}, err
	}
	if entity.VisibilityStatus != domainimagetask.VisibilityApproved {
		return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
	}
	image := mapGalleryImageEntity(entity, taskEntity)
	items, err := s.decoratePublicImages(ctx, []domainimagetask.GalleryImage{image}, viewerUserID)
	if err != nil {
		return domainimagetask.GalleryImage{}, err
	}
	return items[0], nil
}

func (s *ImageTaskStore) SetPublicImageInteraction(ctx context.Context, userID int64, imageID, kind string, active bool) (domainimagetask.GalleryImage, error) {
	imageUUID, err := uuid.Parse(imageID)
	if err != nil {
		return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
	}
	entity, taskEntity, err := s.loadGalleryImageWithTask(ctx, imageUUID)
	if err != nil {
		return domainimagetask.GalleryImage{}, err
	}
	if entity.VisibilityStatus != domainimagetask.VisibilityApproved {
		return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
	}
	interaction, err := s.client.PublicImageInteraction.Query().
		Where(publicimageinteraction.ImageIDEQ(imageUUID), publicimageinteraction.UserIDEQ(userID)).
		Only(ctx)
	if err != nil && !repoent.IsNotFound(err) {
		return domainimagetask.GalleryImage{}, err
	}
	liked := false
	favorited := false
	if interaction != nil {
		liked = interaction.Liked
		favorited = interaction.Favorited
	}
	nextLiked, nextFavorited := liked, favorited
	switch kind {
	case "like":
		nextLiked = active
	case "favorite":
		nextFavorited = active
	default:
		return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
	}
	if interaction == nil {
		_, err = s.client.PublicImageInteraction.Create().
			SetImageID(imageUUID).
			SetUserID(userID).
			SetLiked(nextLiked).
			SetFavorited(nextFavorited).
			Save(ctx)
	} else {
		_, err = s.client.PublicImageInteraction.UpdateOneID(interaction.ID).
			SetLiked(nextLiked).
			SetFavorited(nextFavorited).
			Save(ctx)
	}
	if err != nil {
		return domainimagetask.GalleryImage{}, err
	}
	if err := s.recalculatePublicImageStats(ctx, imageUUID); err != nil {
		return domainimagetask.GalleryImage{}, err
	}
	image := mapGalleryImageEntity(entity, taskEntity)
	items, err := s.decoratePublicImages(ctx, []domainimagetask.GalleryImage{image}, userID)
	if err != nil {
		return domainimagetask.GalleryImage{}, err
	}
	return items[0], nil
}

func (s *ImageTaskStore) DeleteByID(ctx context.Context, userID int64, taskID string) error {
	taskUUID, err := uuid.Parse(taskID)
	if err != nil {
		return err
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	entity, err := tx.ImageTask.Query().
		Where(imagetask.IDEQ(taskUUID), imagetask.UserIDEQ(userID), imagetask.DeletedAtIsNil(), lockImageTaskForWorkerUpdate()).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return repoerr.ErrNotFound
		}
		return err
	}

	results, err := tx.ImageResult.Query().Where(imageresult.TaskIDEQ(taskUUID), imageresult.UserIDEQ(userID), imageresult.DeletedAtIsNil()).All(ctx)
	if err != nil {
		return err
	}
	deletedAt := time.Now().UTC()
	if err := tx.ImageTask.UpdateOneID(entity.ID).SetDeletedAt(deletedAt).Exec(ctx); err != nil {
		return err
	}
	if _, err := tx.ImageResult.Update().
		Where(imageresult.TaskIDEQ(taskUUID), imageresult.UserIDEQ(userID), imageresult.DeletedAtIsNil()).
		SetDeletedAt(deletedAt).
		Save(ctx); err != nil {
		return err
	}
	for _, result := range results {
		configID := ""
		if result.StorageConfigID != nil {
			configID = result.StorageConfigID.String()
		}
		if _, err := enqueueObjectDeletionJob(ctx, tx.Client(), cleanupIdentity(configID, result.StorageDriver, result.ObjectKey)); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *ImageTaskStore) AcquireNextQueuedTask(ctx context.Context, owner string, now time.Time, leaseTTL time.Duration) (domainimagetask.Task, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return domainimagetask.Task{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	entity, err := tx.ImageTask.Query().
		Where(imagetask.DeletedAtIsNil(), acquireEligiblePredicate(now)).
		Order(repoent.Asc(imagetask.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainimagetask.Task{}, repoerr.ErrNotFound
		}
		return domainimagetask.Task{}, err
	}

	expiresAt := now.Add(leaseTTL)
	update := tx.ImageTask.Update().
		Where(imagetask.IDEQ(entity.ID), imagetask.DeletedAtIsNil(), acquireEligiblePredicate(now)).
		SetStatus(domainimagetask.StatusRunning).
		SetProgressStage(domainimagetask.ProgressStageProvider).
		SetProgressMessage("正在调用模型生成图片").
		SetLeaseOwner(owner).
		SetLeaseExpiresAt(expiresAt)
	if entity.StartedAt == nil {
		update.SetStartedAt(now)
	}
	affected, err := update.Save(ctx)
	if err != nil {
		return domainimagetask.Task{}, err
	}
	if affected == 0 {
		return domainimagetask.Task{}, repoerr.ErrNotFound
	}

	updated, err := tx.ImageTask.Query().Where(imagetask.IDEQ(entity.ID)).Only(ctx)
	if err != nil {
		return domainimagetask.Task{}, err
	}
	results, err := tx.ImageResult.Query().
		Where(imageresult.TaskIDEQ(entity.ID), imageresult.UserIDEQ(updated.UserID), imageresult.DeletedAtIsNil()).
		Order(repoent.Asc(imageresult.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return domainimagetask.Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return domainimagetask.Task{}, err
	}
	return mapImageTaskEntity(updated, results)
}

func (s *ImageTaskStore) RenewTaskLease(ctx context.Context, taskID, owner string, now time.Time, leaseTTL time.Duration) (domainimagetask.Task, error) {
	taskUUID, err := uuid.Parse(taskID)
	if err != nil {
		return domainimagetask.Task{}, err
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return domainimagetask.Task{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	entity, err := tx.ImageTask.Query().Where(imagetask.IDEQ(taskUUID), imagetask.DeletedAtIsNil()).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainimagetask.Task{}, repoerr.ErrNotFound
		}
		return domainimagetask.Task{}, err
	}
	if entity.Status != domainimagetask.StatusRunning || entity.LeaseOwner == nil || *entity.LeaseOwner != owner {
		return domainimagetask.Task{}, repoerr.ErrConflict
	}
	if entity.LeaseExpiresAt != nil && entity.LeaseExpiresAt.Before(now) {
		return domainimagetask.Task{}, repoerr.ErrConflict
	}

	expiresAt := now.Add(leaseTTL)
	affected, err := tx.ImageTask.Update().
		Where(
			imagetask.IDEQ(taskUUID),
			imagetask.DeletedAtIsNil(),
			imagetask.StatusEQ(domainimagetask.StatusRunning),
			imagetask.LeaseOwnerEQ(owner),
			imagetask.Or(imagetask.LeaseExpiresAtIsNil(), imagetask.LeaseExpiresAtGTE(now)),
		).
		SetLeaseExpiresAt(expiresAt).
		Save(ctx)
	if err != nil {
		return domainimagetask.Task{}, err
	}
	if affected == 0 {
		return domainimagetask.Task{}, repoerr.ErrConflict
	}

	updated, err := tx.ImageTask.Query().Where(imagetask.IDEQ(taskUUID)).Only(ctx)
	if err != nil {
		return domainimagetask.Task{}, err
	}
	results, err := tx.ImageResult.Query().
		Where(imageresult.TaskIDEQ(taskUUID), imageresult.UserIDEQ(updated.UserID), imageresult.DeletedAtIsNil()).
		Order(repoent.Asc(imageresult.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return domainimagetask.Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return domainimagetask.Task{}, err
	}
	return mapImageTaskEntity(updated, results)
}

func createImageTask(ctx context.Context, tx *repoent.Tx, taskUUID uuid.UUID, task domainimagetask.Task, trace map[string]any, routingSnapshot map[string]any) error {
	pricingSnapshot, err := buildPricingSnapshot(task)
	if err != nil {
		return err
	}
	builder := tx.ImageTask.Create().
		SetID(taskUUID).
		SetUserID(task.UserID).
		SetSourceChannel(defaultString(task.SourceChannel, "web")).
		SetTaskType(defaultTaskType(task.TaskType)).
		SetStatus(defaultTaskStatus(task.Status)).
		SetProgressStage(task.ProgressStage).
		SetProgressMessage(task.ProgressMessage).
		SetPrompt(task.Prompt).
		SetAbstractModel(task.AbstractModel).
		SetSizeMode(defaultString(task.SizeMode, "ratio")).
		SetAspectRatio(defaultString(task.AspectRatio, "1:1")).
		SetBaseResolution(defaultString(task.BaseResolution, "auto")).
		SetQuality(defaultString(task.Quality, "auto")).
		SetOutputFormat(defaultString(task.OutputFormat, "png")).
		SetOutputCompression(defaultPositive(task.OutputCompression, 100)).
		SetModeration(defaultString(task.Moderation, "auto")).
		SetRequestedSize(task.RequestedSize).
		SetResolvedWidth(task.ResolvedWidth).
		SetResolvedHeight(task.ResolvedHeight).
		SetBackground(task.Background).
		SetRequestedOutputImageCount(defaultPositive(task.OutputImageCount, 1)).
		SetSuccessOutputImageCount(len(task.Results)).
		SetReferenceImageCount(task.ReferenceImageCount).
		SetMaskPresent(false).
		SetResponseMode(defaultString(task.ResponseMode, "async")).
		SetSavePolicy(defaultString(task.SavePolicy, "private")).
		SetEstimatedPoints(defaultString(task.EstimatedPoints, "0.00000")).
		SetActualPoints(defaultString(task.ActualPoints, "0.00000")).
		SetRouteModelCode(defaultString(task.RouteModelCode, "")).
		SetEffectiveMultiplier(defaultString(task.EffectiveMultiplier, "1.00000")).
		SetChargedPoints(defaultString(task.ChargedPoints, task.EstimatedPoints)).
		SetProviderCost(defaultString(task.ProviderCost, "0.00000")).
		SetGrossMargin(defaultString(task.GrossMargin, "0.00000")).
		SetFallbackCount(task.FallbackCount).
		SetRouteSnapshotVersion(defaultString(task.RouteSnapshotVersion, "")).
		SetPricingSnapshot(pricingSnapshot).
		SetRoutingSnapshot(routingSnapshot).
		SetProviderTrace(trace).
		SetArtifactRecoveryStatus(task.ArtifactRecovery.Status).
		SetArtifactAttemptCount(task.ArtifactRecovery.AttemptCount).
		SetArtifactLastDiagnostic(artifactDiagnosticsMap(task.ArtifactRecovery)).
		SetArtifactStorageVersion(task.ArtifactRecovery.StorageVersion)
	if projectID := strings.TrimSpace(task.ProjectID); projectID != "" {
		parsedProjectID, parseErr := uuid.Parse(projectID)
		if parseErr != nil {
			return repoerr.ErrNotFound
		}
		if _, ownedErr := tx.Project.Query().Where(
			projectent.IDEQ(parsedProjectID), projectent.UserIDEQ(task.UserID),
			projectent.StatusEQ("active"), projectent.DeletedAtIsNil(), lockProjectForTaskWrite(),
		).Only(ctx); ownedErr != nil {
			if repoent.IsNotFound(ownedErr) {
				return repoerr.ErrNotFound
			}
			return ownedErr
		}
		builder.SetProjectID(parsedProjectID)
	}
	setImageTaskCreateArtifactFields(builder, task)
	if task.APIKeyID > 0 {
		builder.SetAPIKeyID(task.APIKeyID)
	}
	if task.ProviderModelID > 0 {
		builder.SetProviderModelID(task.ProviderModelID)
	}
	if task.RouteModelID > 0 {
		builder.SetRouteModelID(task.RouteModelID)
	}
	if task.AccountModelID > 0 {
		builder.SetAccountModelID(task.AccountModelID)
	}
	if task.ModelAccountID > 0 {
		builder.SetModelAccountID(task.ModelAccountID)
	}
	if strings.TrimSpace(task.UpstreamModelCode) != "" {
		builder.SetUpstreamModelCode(task.UpstreamModelCode)
	}
	if strings.TrimSpace(task.NegativePrompt) != "" {
		builder.SetNegativePrompt(task.NegativePrompt)
	}
	if task.ReferenceStrength > 0 {
		builder.SetReferenceStrength(task.ReferenceStrength)
	}
	if task.Seed != nil {
		builder.SetSeed(*task.Seed)
	}

	now := time.Now().UTC()
	if task.LeaseOwner != "" {
		builder.SetLeaseOwner(task.LeaseOwner)
	}
	if task.LeaseExpiresAt != nil {
		builder.SetLeaseExpiresAt(*task.LeaseExpiresAt)
	}
	if task.Status == domainimagetask.StatusRunning {
		builder.SetStartedAt(now)
	}
	if isTerminalStatus(task.Status) {
		builder.SetStartedAt(now)
		builder.SetFinishedAt(now)
	}
	if strings.TrimSpace(task.ErrorCode) != "" {
		builder.SetErrorCode(task.ErrorCode)
	}
	if strings.TrimSpace(task.ErrorMessage) != "" {
		builder.SetErrorMessage(task.ErrorMessage)
	}
	_, err = builder.Save(ctx)
	return err
}

func lockProjectForTaskWrite() predicate.Project {
	return func(selector *entsql.Selector) {
		if selector.Dialect() != dialect.SQLite {
			selector.ForShare()
		}
	}
}

func lockImageTaskForWorkerUpdate() predicate.ImageTask {
	return func(selector *entsql.Selector) {
		if selector.Dialect() != dialect.SQLite {
			selector.ForUpdate()
		}
	}
}

func updateImageTask(ctx context.Context, tx *repoent.Tx, entity *repoent.ImageTask, task domainimagetask.Task, trace map[string]any, routingSnapshot map[string]any) error {
	pricingSnapshot, err := buildPricingSnapshot(task)
	if err != nil {
		return err
	}
	builder := tx.ImageTask.UpdateOneID(entity.ID).
		SetUserID(task.UserID).
		SetSourceChannel(defaultString(task.SourceChannel, entity.SourceChannel)).
		SetTaskType(defaultTaskType(task.TaskType)).
		SetStatus(defaultTaskStatus(task.Status)).
		SetProgressStage(task.ProgressStage).
		SetProgressMessage(task.ProgressMessage).
		SetPrompt(task.Prompt).
		SetAbstractModel(task.AbstractModel).
		SetSizeMode(defaultString(task.SizeMode, entity.SizeMode)).
		SetAspectRatio(defaultString(task.AspectRatio, entity.AspectRatio)).
		SetBaseResolution(defaultString(task.BaseResolution, "auto")).
		SetQuality(defaultString(task.Quality, entity.Quality)).
		SetOutputFormat(defaultString(task.OutputFormat, entity.OutputFormat)).
		SetOutputCompression(defaultPositive(task.OutputCompression, entity.OutputCompression)).
		SetModeration(defaultString(task.Moderation, entity.Moderation)).
		SetRequestedSize(task.RequestedSize).
		SetResolvedWidth(task.ResolvedWidth).
		SetResolvedHeight(task.ResolvedHeight).
		SetBackground(task.Background).
		SetRequestedOutputImageCount(defaultPositive(task.OutputImageCount, 1)).
		SetSuccessOutputImageCount(len(task.Results)).
		SetReferenceImageCount(task.ReferenceImageCount).
		SetResponseMode(defaultString(task.ResponseMode, entity.ResponseMode)).
		SetSavePolicy(defaultString(task.SavePolicy, entity.SavePolicy)).
		SetEstimatedPoints(defaultString(task.EstimatedPoints, entity.EstimatedPoints)).
		SetActualPoints(defaultString(task.ActualPoints, entity.ActualPoints)).
		SetRouteModelCode(defaultString(task.RouteModelCode, "")).
		SetEffectiveMultiplier(defaultString(task.EffectiveMultiplier, "1.00000")).
		SetChargedPoints(defaultString(task.ChargedPoints, task.EstimatedPoints)).
		SetProviderCost(defaultString(task.ProviderCost, entity.ProviderCost)).
		SetGrossMargin(defaultString(task.GrossMargin, entity.GrossMargin)).
		SetFallbackCount(task.FallbackCount).
		SetRouteSnapshotVersion(defaultString(task.RouteSnapshotVersion, entity.RouteSnapshotVersion)).
		SetPricingSnapshot(pricingSnapshot).
		SetRoutingSnapshot(routingSnapshot).
		SetProviderTrace(trace).
		SetArtifactRecoveryStatus(task.ArtifactRecovery.Status).
		SetArtifactAttemptCount(task.ArtifactRecovery.AttemptCount).
		SetArtifactLastDiagnostic(artifactDiagnosticsMap(task.ArtifactRecovery)).
		SetArtifactStorageVersion(task.ArtifactRecovery.StorageVersion)
	setImageTaskUpdateOneArtifactFields(builder, task)
	if task.APIKeyID > 0 {
		builder.SetAPIKeyID(task.APIKeyID)
	} else {
		builder.ClearAPIKeyID()
	}
	if task.ProviderModelID > 0 {
		builder.SetProviderModelID(task.ProviderModelID)
	} else {
		builder.ClearProviderModelID()
	}
	if task.RouteModelID > 0 {
		builder.SetRouteModelID(task.RouteModelID)
	} else {
		builder.ClearRouteModelID()
	}
	if task.AccountModelID > 0 {
		builder.SetAccountModelID(task.AccountModelID)
	} else {
		builder.ClearAccountModelID()
	}
	if task.ModelAccountID > 0 {
		builder.SetModelAccountID(task.ModelAccountID)
	} else {
		builder.ClearModelAccountID()
	}
	builder.SetUpstreamModelCode(defaultString(task.UpstreamModelCode, ""))
	if task.RouteModelID > 0 {
		builder.SetRouteModelID(task.RouteModelID)
	} else {
		builder.ClearRouteModelID()
	}
	if task.AccountModelID > 0 {
		builder.SetAccountModelID(task.AccountModelID)
	} else {
		builder.ClearAccountModelID()
	}
	if task.ModelAccountID > 0 {
		builder.SetModelAccountID(task.ModelAccountID)
	} else {
		builder.ClearModelAccountID()
	}
	builder.SetUpstreamModelCode(defaultString(task.UpstreamModelCode, ""))
	if strings.TrimSpace(task.NegativePrompt) != "" {
		builder.SetNegativePrompt(task.NegativePrompt)
	} else {
		builder.ClearNegativePrompt()
	}
	if task.ReferenceStrength > 0 {
		builder.SetReferenceStrength(task.ReferenceStrength)
	} else {
		builder.ClearReferenceStrength()
	}
	if task.Seed != nil {
		builder.SetSeed(*task.Seed)
	} else {
		builder.ClearSeed()
	}

	if task.LeaseOwner != "" {
		builder.SetLeaseOwner(task.LeaseOwner)
	} else {
		builder.ClearLeaseOwner()
	}
	if task.LeaseExpiresAt != nil {
		builder.SetLeaseExpiresAt(*task.LeaseExpiresAt)
	} else {
		builder.ClearLeaseExpiresAt()
	}
	if strings.TrimSpace(task.ErrorCode) != "" {
		builder.SetErrorCode(task.ErrorCode)
	} else {
		builder.ClearErrorCode()
	}
	if strings.TrimSpace(task.ErrorMessage) != "" {
		builder.SetErrorMessage(task.ErrorMessage)
	} else {
		builder.ClearErrorMessage()
	}

	startedAt := entity.StartedAt
	if task.Status == domainimagetask.StatusRunning && startedAt == nil {
		now := time.Now().UTC()
		startedAt = &now
	}
	if startedAt != nil {
		builder.SetStartedAt(*startedAt)
	}
	if isTerminalStatus(task.Status) {
		builder.SetFinishedAt(time.Now().UTC())
	} else {
		builder.ClearFinishedAt()
	}

	return builder.Exec(ctx)
}

func updateLeaseOwnedImageTask(ctx context.Context, tx *repoent.Tx, entity *repoent.ImageTask, task domainimagetask.Task, owner string, now time.Time, trace map[string]any, routingSnapshot map[string]any) (int, error) {
	pricingSnapshot, err := buildPricingSnapshot(task)
	if err != nil {
		return 0, err
	}
	builder := tx.ImageTask.Update().
		Where(
			imagetask.IDEQ(entity.ID),
			imagetask.DeletedAtIsNil(),
			imagetask.StatusEQ(domainimagetask.StatusRunning),
			imagetask.LeaseOwnerEQ(owner),
			imagetask.Or(imagetask.LeaseExpiresAtIsNil(), imagetask.LeaseExpiresAtGTE(now)),
		).
		SetUserID(task.UserID).
		SetSourceChannel(defaultString(task.SourceChannel, entity.SourceChannel)).
		SetTaskType(defaultTaskType(task.TaskType)).
		SetStatus(defaultTaskStatus(task.Status)).
		SetProgressStage(task.ProgressStage).
		SetProgressMessage(task.ProgressMessage).
		SetPrompt(task.Prompt).
		SetAbstractModel(task.AbstractModel).
		SetSizeMode(defaultString(task.SizeMode, entity.SizeMode)).
		SetAspectRatio(defaultString(task.AspectRatio, entity.AspectRatio)).
		SetBaseResolution(defaultString(task.BaseResolution, "auto")).
		SetQuality(defaultString(task.Quality, entity.Quality)).
		SetOutputFormat(defaultString(task.OutputFormat, entity.OutputFormat)).
		SetOutputCompression(defaultPositive(task.OutputCompression, entity.OutputCompression)).
		SetModeration(defaultString(task.Moderation, entity.Moderation)).
		SetRequestedSize(task.RequestedSize).
		SetResolvedWidth(task.ResolvedWidth).
		SetResolvedHeight(task.ResolvedHeight).
		SetBackground(task.Background).
		SetRequestedOutputImageCount(defaultPositive(task.OutputImageCount, 1)).
		SetSuccessOutputImageCount(len(task.Results)).
		SetReferenceImageCount(task.ReferenceImageCount).
		SetResponseMode(defaultString(task.ResponseMode, entity.ResponseMode)).
		SetSavePolicy(defaultString(task.SavePolicy, entity.SavePolicy)).
		SetEstimatedPoints(defaultString(task.EstimatedPoints, entity.EstimatedPoints)).
		SetActualPoints(defaultString(task.ActualPoints, entity.ActualPoints)).
		SetRouteModelCode(defaultString(task.RouteModelCode, "")).
		SetEffectiveMultiplier(defaultString(task.EffectiveMultiplier, "1.00000")).
		SetChargedPoints(defaultString(task.ChargedPoints, task.EstimatedPoints)).
		SetProviderCost(defaultString(task.ProviderCost, entity.ProviderCost)).
		SetGrossMargin(defaultString(task.GrossMargin, entity.GrossMargin)).
		SetFallbackCount(task.FallbackCount).
		SetRouteSnapshotVersion(defaultString(task.RouteSnapshotVersion, entity.RouteSnapshotVersion)).
		SetPricingSnapshot(pricingSnapshot).
		SetRoutingSnapshot(routingSnapshot).
		SetProviderTrace(trace).
		SetArtifactRecoveryStatus(task.ArtifactRecovery.Status).
		SetArtifactAttemptCount(task.ArtifactRecovery.AttemptCount).
		SetArtifactLastDiagnostic(artifactDiagnosticsMap(task.ArtifactRecovery)).
		SetArtifactStorageVersion(task.ArtifactRecovery.StorageVersion)
	setImageTaskUpdateArtifactFields(builder, task)
	if task.APIKeyID > 0 {
		builder.SetAPIKeyID(task.APIKeyID)
	} else {
		builder.ClearAPIKeyID()
	}
	if task.ProviderModelID > 0 {
		builder.SetProviderModelID(task.ProviderModelID)
	} else {
		builder.ClearProviderModelID()
	}
	if task.RouteModelID > 0 {
		builder.SetRouteModelID(task.RouteModelID)
	} else {
		builder.ClearRouteModelID()
	}
	if task.AccountModelID > 0 {
		builder.SetAccountModelID(task.AccountModelID)
	} else {
		builder.ClearAccountModelID()
	}
	if task.ModelAccountID > 0 {
		builder.SetModelAccountID(task.ModelAccountID)
	} else {
		builder.ClearModelAccountID()
	}
	builder.SetUpstreamModelCode(defaultString(task.UpstreamModelCode, ""))
	if strings.TrimSpace(task.NegativePrompt) != "" {
		builder.SetNegativePrompt(task.NegativePrompt)
	} else {
		builder.ClearNegativePrompt()
	}
	if task.ReferenceStrength > 0 {
		builder.SetReferenceStrength(task.ReferenceStrength)
	} else {
		builder.ClearReferenceStrength()
	}
	if task.Seed != nil {
		builder.SetSeed(*task.Seed)
	} else {
		builder.ClearSeed()
	}

	// Running-state progress updates must not rewrite lease columns because
	// heartbeat renewals happen concurrently in a separate transaction.
	if task.Status != domainimagetask.StatusRunning {
		if task.LeaseOwner != "" {
			builder.SetLeaseOwner(task.LeaseOwner)
		} else {
			builder.ClearLeaseOwner()
		}
		if task.LeaseExpiresAt != nil {
			builder.SetLeaseExpiresAt(*task.LeaseExpiresAt)
		} else {
			builder.ClearLeaseExpiresAt()
		}
	}
	if strings.TrimSpace(task.ErrorCode) != "" {
		builder.SetErrorCode(task.ErrorCode)
	} else {
		builder.ClearErrorCode()
	}
	if strings.TrimSpace(task.ErrorMessage) != "" {
		builder.SetErrorMessage(task.ErrorMessage)
	} else {
		builder.ClearErrorMessage()
	}

	startedAt := entity.StartedAt
	if task.Status == domainimagetask.StatusRunning && startedAt == nil {
		startedAt = &now
	}
	if startedAt != nil {
		builder.SetStartedAt(*startedAt)
	}
	if isTerminalStatus(task.Status) {
		builder.SetFinishedAt(now)
	} else {
		builder.ClearFinishedAt()
	}
	return builder.Save(ctx)
}

func updateRecoverableImageTask(ctx context.Context, tx *repoent.Tx, entity *repoent.ImageTask, task domainimagetask.Task, owner string, now time.Time, trace map[string]any, routingSnapshot map[string]any) (int, error) {
	pricingSnapshot, err := buildPricingSnapshot(task)
	if err != nil {
		return 0, err
	}
	builder := tx.ImageTask.Update().
		Where(
			imagetask.IDEQ(entity.ID),
			imagetask.DeletedAtIsNil(),
			imagetask.StatusEQ(domainimagetask.StatusRunning),
			imagetask.LeaseOwnerEQ(owner),
		).
		SetUserID(task.UserID).
		SetSourceChannel(defaultString(task.SourceChannel, entity.SourceChannel)).
		SetTaskType(defaultTaskType(task.TaskType)).
		SetStatus(defaultTaskStatus(task.Status)).
		SetProgressStage(task.ProgressStage).
		SetProgressMessage(task.ProgressMessage).
		SetPrompt(task.Prompt).
		SetAbstractModel(task.AbstractModel).
		SetSizeMode(defaultString(task.SizeMode, entity.SizeMode)).
		SetAspectRatio(defaultString(task.AspectRatio, entity.AspectRatio)).
		SetBaseResolution(defaultString(task.BaseResolution, "auto")).
		SetQuality(defaultString(task.Quality, entity.Quality)).
		SetOutputFormat(defaultString(task.OutputFormat, entity.OutputFormat)).
		SetOutputCompression(defaultPositive(task.OutputCompression, entity.OutputCompression)).
		SetModeration(defaultString(task.Moderation, entity.Moderation)).
		SetRequestedSize(task.RequestedSize).
		SetResolvedWidth(task.ResolvedWidth).
		SetResolvedHeight(task.ResolvedHeight).
		SetBackground(task.Background).
		SetRequestedOutputImageCount(defaultPositive(task.OutputImageCount, 1)).
		SetSuccessOutputImageCount(len(task.Results)).
		SetReferenceImageCount(task.ReferenceImageCount).
		SetResponseMode(defaultString(task.ResponseMode, entity.ResponseMode)).
		SetSavePolicy(defaultString(task.SavePolicy, entity.SavePolicy)).
		SetEstimatedPoints(defaultString(task.EstimatedPoints, entity.EstimatedPoints)).
		SetActualPoints(defaultString(task.ActualPoints, entity.ActualPoints)).
		SetRouteModelCode(defaultString(task.RouteModelCode, "")).
		SetEffectiveMultiplier(defaultString(task.EffectiveMultiplier, "1.00000")).
		SetChargedPoints(defaultString(task.ChargedPoints, task.EstimatedPoints)).
		SetProviderCost(defaultString(task.ProviderCost, entity.ProviderCost)).
		SetGrossMargin(defaultString(task.GrossMargin, entity.GrossMargin)).
		SetFallbackCount(task.FallbackCount).
		SetRouteSnapshotVersion(defaultString(task.RouteSnapshotVersion, entity.RouteSnapshotVersion)).
		SetPricingSnapshot(pricingSnapshot).
		SetRoutingSnapshot(routingSnapshot).
		SetProviderTrace(trace).
		SetArtifactRecoveryStatus(task.ArtifactRecovery.Status).
		SetArtifactAttemptCount(task.ArtifactRecovery.AttemptCount).
		SetArtifactLastDiagnostic(artifactDiagnosticsMap(task.ArtifactRecovery)).
		SetArtifactStorageVersion(task.ArtifactRecovery.StorageVersion)
	setImageTaskUpdateArtifactFields(builder, task)
	if task.APIKeyID > 0 {
		builder.SetAPIKeyID(task.APIKeyID)
	} else {
		builder.ClearAPIKeyID()
	}
	if task.ProviderModelID > 0 {
		builder.SetProviderModelID(task.ProviderModelID)
	} else {
		builder.ClearProviderModelID()
	}
	if strings.TrimSpace(task.NegativePrompt) != "" {
		builder.SetNegativePrompt(task.NegativePrompt)
	} else {
		builder.ClearNegativePrompt()
	}
	if task.ReferenceStrength > 0 {
		builder.SetReferenceStrength(task.ReferenceStrength)
	} else {
		builder.ClearReferenceStrength()
	}
	if task.Seed != nil {
		builder.SetSeed(*task.Seed)
	} else {
		builder.ClearSeed()
	}

	if task.Status != domainimagetask.StatusRunning {
		if task.LeaseOwner != "" {
			builder.SetLeaseOwner(task.LeaseOwner)
		} else {
			builder.ClearLeaseOwner()
		}
		if task.LeaseExpiresAt != nil {
			builder.SetLeaseExpiresAt(*task.LeaseExpiresAt)
		} else {
			builder.ClearLeaseExpiresAt()
		}
	}
	if strings.TrimSpace(task.ErrorCode) != "" {
		builder.SetErrorCode(task.ErrorCode)
	} else {
		builder.ClearErrorCode()
	}
	if strings.TrimSpace(task.ErrorMessage) != "" {
		builder.SetErrorMessage(task.ErrorMessage)
	} else {
		builder.ClearErrorMessage()
	}

	startedAt := entity.StartedAt
	if task.Status == domainimagetask.StatusRunning && startedAt == nil {
		startedAt = &now
	}
	if startedAt != nil {
		builder.SetStartedAt(*startedAt)
	}
	if isTerminalStatus(task.Status) {
		builder.SetFinishedAt(now)
	} else {
		builder.ClearFinishedAt()
	}
	return builder.Save(ctx)
}

func setImageTaskCreateArtifactFields(builder *repoent.ImageTaskCreate, task domainimagetask.Task) {
	if task.ProviderRequestID != "" {
		builder.SetProviderRequestID(task.ProviderRequestID)
	}
	if task.UpstreamSucceededAt != nil {
		builder.SetUpstreamSucceededAt(*task.UpstreamSucceededAt)
	}
	if task.ArtifactRecovery.EncryptedPayload != "" {
		builder.SetArtifactRecoveryPayload(task.ArtifactRecovery.EncryptedPayload)
	}
	if task.ArtifactRecovery.NextRetryAt != nil {
		builder.SetArtifactNextRetryAt(*task.ArtifactRecovery.NextRetryAt)
	}
	if id, err := uuid.Parse(task.ArtifactRecovery.StorageConfigID); err == nil {
		builder.SetArtifactStorageConfigID(id)
	}
	builder.SetArtifactStorageDriver(task.ArtifactRecovery.StorageDriver)
	builder.SetArtifactStorageBucket(task.ArtifactRecovery.StorageBucket)
	builder.SetArtifactObjectKeys(append([]string(nil), task.ArtifactRecovery.ObjectKeys...))
}

func setImageTaskUpdateOneArtifactFields(builder *repoent.ImageTaskUpdateOne, task domainimagetask.Task) {
	if task.ProviderRequestID != "" {
		builder.SetProviderRequestID(task.ProviderRequestID)
	} else {
		builder.ClearProviderRequestID()
	}
	if task.UpstreamSucceededAt != nil {
		builder.SetUpstreamSucceededAt(*task.UpstreamSucceededAt)
	} else {
		builder.ClearUpstreamSucceededAt()
	}
	if task.ArtifactRecovery.EncryptedPayload != "" {
		builder.SetArtifactRecoveryPayload(task.ArtifactRecovery.EncryptedPayload)
	} else {
		builder.ClearArtifactRecoveryPayload()
	}
	if task.ArtifactRecovery.NextRetryAt != nil {
		builder.SetArtifactNextRetryAt(*task.ArtifactRecovery.NextRetryAt)
	} else {
		builder.ClearArtifactNextRetryAt()
	}
	if id, err := uuid.Parse(task.ArtifactRecovery.StorageConfigID); err == nil {
		builder.SetArtifactStorageConfigID(id)
	} else {
		builder.ClearArtifactStorageConfigID()
	}
	builder.SetArtifactStorageDriver(task.ArtifactRecovery.StorageDriver)
	builder.SetArtifactStorageBucket(task.ArtifactRecovery.StorageBucket)
	builder.SetArtifactObjectKeys(append([]string(nil), task.ArtifactRecovery.ObjectKeys...))
}

func setImageTaskUpdateArtifactFields(builder *repoent.ImageTaskUpdate, task domainimagetask.Task) {
	if task.ProviderRequestID != "" {
		builder.SetProviderRequestID(task.ProviderRequestID)
	} else {
		builder.ClearProviderRequestID()
	}
	if task.UpstreamSucceededAt != nil {
		builder.SetUpstreamSucceededAt(*task.UpstreamSucceededAt)
	} else {
		builder.ClearUpstreamSucceededAt()
	}
	if task.ArtifactRecovery.EncryptedPayload != "" {
		builder.SetArtifactRecoveryPayload(task.ArtifactRecovery.EncryptedPayload)
	} else {
		builder.ClearArtifactRecoveryPayload()
	}
	if task.ArtifactRecovery.NextRetryAt != nil {
		builder.SetArtifactNextRetryAt(*task.ArtifactRecovery.NextRetryAt)
	} else {
		builder.ClearArtifactNextRetryAt()
	}
	if id, err := uuid.Parse(task.ArtifactRecovery.StorageConfigID); err == nil {
		builder.SetArtifactStorageConfigID(id)
	} else {
		builder.ClearArtifactStorageConfigID()
	}
	builder.SetArtifactStorageDriver(task.ArtifactRecovery.StorageDriver)
	builder.SetArtifactStorageBucket(task.ArtifactRecovery.StorageBucket)
	builder.SetArtifactObjectKeys(append([]string(nil), task.ArtifactRecovery.ObjectKeys...))
}

func artifactDiagnosticsMap(recovery domainimagetask.ArtifactRecovery) map[string]any {
	encoded, err := json.Marshal(map[string]any{"last": recovery.LastDiagnostic, "attempts": recovery.Diagnostics})
	if err != nil {
		return map[string]any{}
	}
	decoded := map[string]any{}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return map[string]any{}
	}
	return decoded
}

func decodeArtifactDiagnostics(value map[string]any) (domainimagetask.ArtifactDiagnostic, []domainimagetask.ArtifactDiagnostic) {
	if _, hasEnvelope := value["last"]; !hasEnvelope {
		encoded, err := json.Marshal(value)
		if err != nil {
			return domainimagetask.ArtifactDiagnostic{}, nil
		}
		var legacy domainimagetask.ArtifactDiagnostic
		if err := json.Unmarshal(encoded, &legacy); err != nil {
			return domainimagetask.ArtifactDiagnostic{}, nil
		}
		return legacy, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return domainimagetask.ArtifactDiagnostic{}, nil
	}
	var decoded struct {
		Last     domainimagetask.ArtifactDiagnostic   `json:"last"`
		Attempts []domainimagetask.ArtifactDiagnostic `json:"attempts"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return domainimagetask.ArtifactDiagnostic{}, nil
	}
	return decoded.Last, decoded.Attempts
}

func createImageResult(ctx context.Context, tx *repoent.Tx, taskUUID uuid.UUID, userID int64, projectID string, index int, result provider.ImageResult) error {
	resultID, err := imageResultUUID(result.ID)
	if err != nil {
		return err
	}
	storageDriver := defaultString(result.StorageDriver, "local")
	objectKey := result.ObjectKey
	if strings.TrimSpace(result.URL) != "" && storageDriver == "local" && strings.TrimSpace(result.ObjectKey) == "" {
		storageDriver = "remote"
		objectKey = result.URL
	}
	if strings.TrimSpace(objectKey) == "" {
		objectKey = fmt.Sprintf("task:%s:%d", taskUUID.String(), index)
	}
	mimeType := result.MimeType
	if strings.TrimSpace(mimeType) == "" {
		mimeType = result.Format
		if strings.TrimSpace(mimeType) != "" && !strings.Contains(mimeType, "/") {
			mimeType = "image/" + strings.TrimSpace(mimeType)
		}
	}
	sha := sha256.Sum256([]byte(result.URL + "|" + result.B64JSON + "|" + fmt.Sprintf("%d", index)))
	shaValue := defaultString(result.SHA256, hex.EncodeToString(sha[:]))
	builder := tx.ImageResult.Create().
		SetID(resultID).
		SetTaskID(taskUUID).
		SetUserID(userID).
		SetImageRole("output").
		SetStorageDriver(storageDriver).
		SetObjectKey(objectKey).
		SetMimeType(defaultString(mimeType, "application/octet-stream")).
		SetFileSizeBytes(result.FileSizeBytes).
		SetWidth(result.Width).
		SetHeight(result.Height).
		SetSha256(shaValue).
		SetImageGroup(strings.TrimSpace(result.ImageGroup)).
		SetVisibilityStatus(defaultString(result.VisibilityStatus, "private"))
	if projectID = strings.TrimSpace(projectID); projectID != "" {
		parsedProjectID, parseErr := uuid.Parse(projectID)
		if parseErr != nil {
			return repoerr.ErrNotFound
		}
		builder.SetProjectID(parsedProjectID)
	}
	if strings.TrimSpace(result.StorageConfigID) != "" {
		storageConfigID, parseErr := uuid.Parse(strings.TrimSpace(result.StorageConfigID))
		if parseErr != nil {
			return parseErr
		}
		builder.SetStorageConfigID(storageConfigID)
	}
	if strings.TrimSpace(result.ReviewReason) != "" {
		builder.SetReviewReason(strings.TrimSpace(result.ReviewReason))
	}
	if result.PublishedAt != nil {
		builder.SetPublishedAt(*result.PublishedAt)
	}
	if strings.TrimSpace(result.StorageConfigID) != "" {
		storageConfigID, err := uuid.Parse(result.StorageConfigID)
		if err != nil {
			return err
		}
		builder.SetStorageConfigID(storageConfigID)
	}
	return builder.Exec(ctx)
}

func persistedImageTaskProjectID(entity *repoent.ImageTask) string {
	if entity == nil || entity.ProjectID == nil {
		return ""
	}
	return entity.ProjectID.String()
}

func imageResultUUID(value string) (uuid.UUID, error) {
	if strings.TrimSpace(value) == "" {
		return uuid.New(), nil
	}
	return uuid.Parse(value)
}

func mapImageTaskEntity(entity *repoent.ImageTask, resultEntities []*repoent.ImageResult) (domainimagetask.Task, error) {
	task := domainimagetask.Task{
		UserID:               entity.UserID,
		SourceChannel:        entity.SourceChannel,
		ID:                   entity.ID.String(),
		Status:               entity.Status,
		ProgressStage:        entity.ProgressStage,
		ProgressMessage:      entity.ProgressMessage,
		AbstractModel:        entity.AbstractModel,
		RouteModelCode:       entity.RouteModelCode,
		RouteModelID:         nullableInt64(entity.RouteModelID),
		AccountModelID:       nullableInt64(entity.AccountModelID),
		ModelAccountID:       nullableInt64(entity.ModelAccountID),
		UpstreamModelCode:    entity.UpstreamModelCode,
		EffectiveMultiplier:  entity.EffectiveMultiplier,
		ChargedPoints:        entity.ChargedPoints,
		TaskType:             entity.TaskType,
		Prompt:               entity.Prompt,
		NegativePrompt:       nullableString(entity.NegativePrompt),
		AspectRatio:          entity.AspectRatio,
		RequestedSize:        nullableString(entity.RequestedSize),
		ResolvedWidth:        nullableInt(entity.ResolvedWidth),
		ResolvedHeight:       nullableInt(entity.ResolvedHeight),
		SizeMode:             entity.SizeMode,
		BaseResolution:       entity.BaseResolution,
		Quality:              entity.Quality,
		OutputFormat:         entity.OutputFormat,
		Background:           nullableString(entity.Background),
		OutputCompression:    entity.OutputCompression,
		Moderation:           entity.Moderation,
		ResponseMode:         entity.ResponseMode,
		SavePolicy:           entity.SavePolicy,
		OutputImageCount:     entity.RequestedOutputImageCount,
		ReferenceImageCount:  entity.ReferenceImageCount,
		ReferenceAssetIDs:    decodeReferenceAssetIDs(entity.RoutingSnapshot),
		ReferenceStrength:    nullableInt(entity.ReferenceStrength),
		Seed:                 entity.Seed,
		EstimatedPoints:      entity.EstimatedPoints,
		ActualPoints:         entity.ActualPoints,
		ProviderModelID:      nullableInt64(entity.ProviderModelID),
		ProviderCost:         entity.ProviderCost,
		GrossMargin:          entity.GrossMargin,
		FallbackCount:        entity.FallbackCount,
		RouteSnapshotVersion: entity.RouteSnapshotVersion,
		LeaseOwner:           nullableString(entity.LeaseOwner),
		LeaseExpiresAt:       entity.LeaseExpiresAt,
		ErrorCode:            nullableString(entity.ErrorCode),
		ErrorMessage:         nullableString(entity.ErrorMessage),
		ProviderRequestID:    nullableString(entity.ProviderRequestID),
		UpstreamSucceededAt:  entity.UpstreamSucceededAt,
		ArtifactRecovery: domainimagetask.ArtifactRecovery{
			Status:           entity.ArtifactRecoveryStatus,
			EncryptedPayload: nullableString(entity.ArtifactRecoveryPayload),
			AttemptCount:     entity.ArtifactAttemptCount,
			NextRetryAt:      entity.ArtifactNextRetryAt,
			StorageDriver:    entity.ArtifactStorageDriver,
			StorageBucket:    entity.ArtifactStorageBucket,
			ObjectKeys:       append([]string(nil), entity.ArtifactObjectKeys...),
			StorageVersion:   entity.ArtifactStorageVersion,
		},
		CreatedAt: entity.CreatedAt,
		UpdatedAt: entity.UpdatedAt,
	}
	if entity.ProjectID != nil {
		task.ProjectID = entity.ProjectID.String()
		task.Project = &domainimagetask.ProjectSnapshot{ID: task.ProjectID}
		if entity.Edges.Project != nil {
			task.Project.Name = entity.Edges.Project.Name
			task.Project.IsDefault = entity.Edges.Project.IsDefault
		}
	}
	task.ArtifactRecovery.LastDiagnostic, task.ArtifactRecovery.Diagnostics = decodeArtifactDiagnostics(entity.ArtifactLastDiagnostic)
	if entity.ArtifactStorageConfigID != nil {
		task.ArtifactRecovery.StorageConfigID = entity.ArtifactStorageConfigID.String()
	}
	if entity.APIKeyID != nil {
		task.APIKeyID = *entity.APIKeyID
	}
	if entity.PricingSnapshot != nil {
		if snapshot, err := decodePricingSnapshot(entity.PricingSnapshot); err == nil {
			task.PricingSnapshot = snapshot
		} else {
			return domainimagetask.Task{}, err
		}
	}
	if entity.RoutingSnapshot != nil {
		if snapshot, err := decodeGenerationSnapshot(entity.RoutingSnapshot["generation_snapshot"]); err == nil {
			task.GenerationSnapshot = snapshot
		} else {
			return domainimagetask.Task{}, err
		}
	}

	if entity.ProviderTrace != nil {
		trace := entity.ProviderTrace
		if providerName, ok := trace["provider"].(string); ok {
			task.Provider = providerName
		}
		if attempts, err := decodeAttempts(trace["attempts"]); err == nil {
			task.Attempts = attempts
		}
	}
	task.Results = mapFallbackResults(resultEntities)
	for index := range task.Results {
		task.Results[index].ProjectID = task.ProjectID
		if task.Project != nil {
			snapshot := *task.Project
			task.Results[index].Project = &snapshot
		}
	}
	return task, nil
}

func buildProviderTrace(task domainimagetask.Task) (map[string]any, error) {
	trace := map[string]any{
		"provider":               task.Provider,
		"provider_model_id":      task.ProviderModelID,
		"provider_cost":          task.ProviderCost,
		"gross_margin":           task.GrossMargin,
		"fallback_count":         task.FallbackCount,
		"route_snapshot_version": task.RouteSnapshotVersion,
	}

	attempts, err := jsonRoundTrip(task.Attempts)
	if err != nil {
		return nil, err
	}
	results, err := jsonRoundTrip(task.Results)
	if err != nil {
		return nil, err
	}
	trace["attempts"] = attempts
	trace["results"] = results
	return trace, nil
}

func buildRoutingSnapshot(task domainimagetask.Task) (map[string]any, error) {
	snapshot := map[string]any{
		"reference_asset_ids":    task.ReferenceAssetIDs,
		"provider_model_id":      task.ProviderModelID,
		"route_snapshot_version": task.RouteSnapshotVersion,
		"fallback_count":         task.FallbackCount,
	}
	if strings.TrimSpace(task.GenerationSnapshot.CapabilityVersion) != "" {
		generationSnapshot, err := jsonRoundTrip(task.GenerationSnapshot)
		if err != nil {
			return nil, err
		}
		snapshot["generation_snapshot"] = generationSnapshot
	}
	return snapshot, nil
}

func decodeGenerationSnapshot(value any) (domainimagetask.GenerationSnapshot, error) {
	var snapshot domainimagetask.GenerationSnapshot
	if value == nil {
		return snapshot, nil
	}
	if err := decodeJSONValue(value, &snapshot); err != nil {
		return domainimagetask.GenerationSnapshot{}, err
	}
	return snapshot, nil
}

func buildPricingSnapshot(task domainimagetask.Task) (map[string]any, error) {
	if task.PricingSnapshot == (domainbilling.PricingSnapshot{}) {
		return map[string]any{}, nil
	}
	value, err := jsonRoundTrip(task.PricingSnapshot)
	if err != nil {
		return nil, err
	}
	decoded, _ := value.(map[string]any)
	return decoded, nil
}

func decodePricingSnapshot(value map[string]any) (domainbilling.PricingSnapshot, error) {
	var snapshot domainbilling.PricingSnapshot
	if len(value) == 0 {
		return snapshot, nil
	}
	if err := decodeJSONValue(value, &snapshot); err != nil {
		return domainbilling.PricingSnapshot{}, err
	}
	return snapshot, nil
}

func decodeAttempts(value any) ([]domainimagetask.Attempt, error) {
	var attempts []domainimagetask.Attempt
	if value == nil {
		return attempts, nil
	}
	if err := decodeJSONValue(value, &attempts); err != nil {
		return nil, err
	}
	return attempts, nil
}

func decodeResults(value any) ([]provider.ImageResult, error) {
	var results []provider.ImageResult
	if value == nil {
		return results, nil
	}
	if err := decodeJSONValue(value, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func decodeJSONValue(value any, target any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, target)
}

func jsonRoundTrip(value any) (any, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func mapFallbackResults(resultEntities []*repoent.ImageResult) []provider.ImageResult {
	if len(resultEntities) == 0 {
		return nil
	}
	sorted := append([]*repoent.ImageResult(nil), resultEntities...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt.Before(sorted[j].CreatedAt)
	})

	results := make([]provider.ImageResult, 0, len(sorted))
	for _, entity := range sorted {
		results = append(results, mapImageResultEntity(entity))
	}
	return results
}

func mapImageResultEntity(entity *repoent.ImageResult) provider.ImageResult {
	item := provider.ImageResult{
		ID:               entity.ID.String(),
		MimeType:         entity.MimeType,
		FileSizeBytes:    entity.FileSizeBytes,
		Width:            entity.Width,
		Height:           entity.Height,
		SHA256:           entity.Sha256,
		StorageConfigID:  "",
		ObjectKey:        entity.ObjectKey,
		StorageDriver:    entity.StorageDriver,
		ImageGroup:       entity.ImageGroup,
		VisibilityStatus: entity.VisibilityStatus,
		ReviewReason:     nullableString(entity.ReviewReason),
		PublishedAt:      entity.PublishedAt,
	}
	if entity.ProjectID != nil {
		item.ProjectID = entity.ProjectID.String()
		item.Project = &domainimagetask.ProjectSnapshot{ID: item.ProjectID}
	}
	if entity.StorageConfigID != nil {
		item.StorageConfigID = entity.StorageConfigID.String()
	}
	if entity.StorageDriver == "remote" {
		item.URL = entity.ObjectKey
		return item
	}
	if entity.StorageConfigID != nil {
		item.StorageConfigID = entity.StorageConfigID.String()
	}
	if strings.TrimSpace(entity.StorageDriver) != "" {
		item.DownloadURL = "/api/agent/image/v1/images/" + entity.ID.String()
		item.URL = item.DownloadURL
	}
	return item
}

func (s *ImageTaskStore) loadGalleryImageWithTask(ctx context.Context, imageID uuid.UUID) (*repoent.ImageResult, *repoent.ImageTask, error) {
	entity, err := s.client.ImageResult.Query().
		Where(imageresult.IDEQ(imageID), imageresult.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return nil, nil, repoerr.ErrNotFound
		}
		return nil, nil, err
	}
	taskEntity, err := s.client.ImageTask.Query().
		Where(imagetask.IDEQ(entity.TaskID), imagetask.DeletedAtIsNil()).
		WithProject().
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return nil, nil, repoerr.ErrNotFound
		}
		return nil, nil, err
	}
	return entity, taskEntity, nil
}

func (s *ImageTaskStore) galleryPageFromQuery(ctx context.Context, query *repoent.ImageResultQuery, page, pageSize int) (domainimagetask.GalleryPage, error) {
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return domainimagetask.GalleryPage{}, err
	}
	entities, err := query.Offset((page - 1) * pageSize).Limit(pageSize).All(ctx)
	if err != nil {
		return domainimagetask.GalleryPage{}, err
	}
	items, err := s.galleryImagesFromEntities(ctx, entities, 0)
	if err != nil {
		return domainimagetask.GalleryPage{}, err
	}
	return domainimagetask.GalleryPage{
		Items:    items,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}, nil
}

func (s *ImageTaskStore) galleryImagesFromEntities(ctx context.Context, entities []*repoent.ImageResult, viewerUserID int64) ([]domainimagetask.GalleryImage, error) {
	taskIDs := make([]uuid.UUID, 0, len(entities))
	for _, entity := range entities {
		taskIDs = append(taskIDs, entity.TaskID)
	}
	taskMap := map[uuid.UUID]*repoent.ImageTask{}
	if len(taskIDs) > 0 {
		tasks, err := s.client.ImageTask.Query().Where(imagetask.IDIn(taskIDs...), imagetask.DeletedAtIsNil()).WithProject().All(ctx)
		if err != nil {
			return nil, err
		}
		for _, task := range tasks {
			taskMap[task.ID] = task
		}
	}
	items := make([]domainimagetask.GalleryImage, 0, len(entities))
	for _, entity := range entities {
		taskEntity, ok := taskMap[entity.TaskID]
		if !ok {
			continue
		}
		items = append(items, mapGalleryImageEntity(entity, taskEntity))
	}
	return s.decoratePublicImages(ctx, items, viewerUserID)
}

func (s *ImageTaskStore) decoratePublicImages(ctx context.Context, items []domainimagetask.GalleryImage, viewerUserID int64) ([]domainimagetask.GalleryImage, error) {
	if len(items) == 0 {
		return items, nil
	}
	imageIDs := make([]uuid.UUID, 0, len(items))
	imageIDByString := map[string]uuid.UUID{}
	for _, item := range items {
		imageUUID, err := uuid.Parse(item.ID)
		if err != nil {
			continue
		}
		imageIDs = append(imageIDs, imageUUID)
		imageIDByString[item.ID] = imageUUID
	}
	stats, err := s.client.PublicImageStat.Query().Where(publicimagestat.ImageIDIn(imageIDs...)).All(ctx)
	if err != nil {
		return nil, err
	}
	statsByImage := map[uuid.UUID]*repoent.PublicImageStat{}
	for _, stat := range stats {
		statsByImage[stat.ImageID] = stat
	}
	interactionsByImage := map[uuid.UUID]*repoent.PublicImageInteraction{}
	if viewerUserID > 0 {
		interactions, err := s.client.PublicImageInteraction.Query().
			Where(publicimageinteraction.ImageIDIn(imageIDs...), publicimageinteraction.UserIDEQ(viewerUserID)).
			All(ctx)
		if err != nil {
			return nil, err
		}
		for _, interaction := range interactions {
			interactionsByImage[interaction.ImageID] = interaction
		}
	}
	userIDs := make([]int, 0, len(items))
	seenUsers := map[int64]struct{}{}
	for _, item := range items {
		if item.UserID <= 0 {
			continue
		}
		if _, ok := seenUsers[item.UserID]; ok {
			continue
		}
		seenUsers[item.UserID] = struct{}{}
		userIDs = append(userIDs, int(item.UserID))
	}
	authorsByID := map[int64]string{}
	if len(userIDs) > 0 {
		users, err := s.client.User.Query().Where(entuser.IDIn(userIDs...), entuser.DeletedAtIsNil()).All(ctx)
		if err != nil {
			return nil, err
		}
		for _, userEntity := range users {
			displayName := strings.TrimSpace(userEntity.Nickname)
			if displayName == "" {
				displayName = strings.TrimSpace(strings.Split(userEntity.Email, "@")[0])
			}
			if displayName != "" {
				authorsByID[int64(userEntity.ID)] = displayName
			}
		}
	}
	for idx := range items {
		items[idx].AuthorName = authorsByID[items[idx].UserID]
		if items[idx].AuthorName == "" {
			items[idx].AuthorName = fmt.Sprintf("user-%d", items[idx].UserID)
		}
		imageUUID, ok := imageIDByString[items[idx].ID]
		if !ok {
			continue
		}
		if stat := statsByImage[imageUUID]; stat != nil {
			items[idx].LikeCount = stat.LikeCount
			items[idx].FavoriteCount = stat.FavoriteCount
		}
		if interaction := interactionsByImage[imageUUID]; interaction != nil {
			items[idx].LikedByViewer = interaction.Liked
			items[idx].FavoritedByViewer = interaction.Favorited
		}
	}
	return items, nil
}

func publicGalleryHotScore(image domainimagetask.GalleryImage) int {
	return image.LikeCount*2 + image.FavoriteCount*3
}

func (s *ImageTaskStore) recalculatePublicImageStats(ctx context.Context, imageID uuid.UUID) error {
	likeCount, err := s.client.PublicImageInteraction.Query().Where(publicimageinteraction.ImageIDEQ(imageID), publicimageinteraction.LikedEQ(true)).Count(ctx)
	if err != nil {
		return err
	}
	favoriteCount, err := s.client.PublicImageInteraction.Query().Where(publicimageinteraction.ImageIDEQ(imageID), publicimageinteraction.FavoritedEQ(true)).Count(ctx)
	if err != nil {
		return err
	}
	stat, err := s.client.PublicImageStat.Query().Where(publicimagestat.ImageIDEQ(imageID)).Only(ctx)
	if err != nil {
		if !repoent.IsNotFound(err) {
			return err
		}
		_, err = s.client.PublicImageStat.Create().SetImageID(imageID).SetLikeCount(likeCount).SetFavoriteCount(favoriteCount).SetCommentCount(0).Save(ctx)
		return err
	}
	_, err = s.client.PublicImageStat.UpdateOneID(stat.ID).SetLikeCount(likeCount).SetFavoriteCount(favoriteCount).Save(ctx)
	return err
}

func mapGalleryImageEntity(entity *repoent.ImageResult, taskEntity *repoent.ImageTask) domainimagetask.GalleryImage {
	item := mapImageResultEntity(entity)
	return domainimagetask.GalleryImage{
		ID:                entity.ID.String(),
		TaskID:            entity.TaskID.String(),
		UserID:            taskEntity.UserID,
		ProjectID:         item.ProjectID,
		Project:           projectSnapshotFromTaskEntity(taskEntity),
		Prompt:            taskEntity.Prompt,
		AbstractModel:     taskEntity.AbstractModel,
		RouteModelCode:    taskEntity.RouteModelCode,
		TaskType:          taskEntity.TaskType,
		TaskStatus:        taskEntity.Status,
		SizeMode:          taskEntity.SizeMode,
		RequestedSize:     nullableString(taskEntity.RequestedSize),
		BaseResolution:    taskEntity.BaseResolution,
		Quality:           taskEntity.Quality,
		AspectRatio:       taskEntity.AspectRatio,
		OutputFormat:      taskEntity.OutputFormat,
		OutputCompression: taskEntity.OutputCompression,
		Moderation:        taskEntity.Moderation,
		OutputImageCount:  taskEntity.RequestedOutputImageCount,
		ActualPoints:      taskEntity.ActualPoints,
		ReferenceAssetIDs: decodeReferenceAssetIDs(taskEntity.RoutingSnapshot),
		ReferenceAssets:   galleryReferenceAssets(decodeReferenceAssetIDs(taskEntity.RoutingSnapshot)),
		URL:               item.URL,
		DownloadURL:       item.DownloadURL,
		MimeType:          item.MimeType,
		FileSizeBytes:     item.FileSizeBytes,
		Width:             item.Width,
		Height:            item.Height,
		SHA256:            item.SHA256,
		StorageConfigID:   item.StorageConfigID,
		ObjectKey:         item.ObjectKey,
		StorageDriver:     item.StorageDriver,
		ImageGroup:        item.ImageGroup,
		VisibilityStatus:  defaultString(entity.VisibilityStatus, domainimagetask.VisibilityPrivate),
		ReviewReason:      nullableString(entity.ReviewReason),
		PublishedAt:       entity.PublishedAt,
		CreatedAt:         entity.CreatedAt,
	}
}

func projectSnapshotFromTaskEntity(entity *repoent.ImageTask) *domainimagetask.ProjectSnapshot {
	if entity == nil || entity.ProjectID == nil {
		return nil
	}
	snapshot := &domainimagetask.ProjectSnapshot{ID: entity.ProjectID.String()}
	if entity.Edges.Project != nil {
		snapshot.Name = entity.Edges.Project.Name
		snapshot.IsDefault = entity.Edges.Project.IsDefault
	}
	return snapshot
}

func galleryReferenceAssets(assetIDs []string) []domainimagetask.GalleryReferenceAsset {
	assets := make([]domainimagetask.GalleryReferenceAsset, 0, len(assetIDs))
	for _, id := range assetIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		assets = append(assets, domainimagetask.GalleryReferenceAsset{
			ID:         id,
			Name:       "reference",
			PreviewURL: "/api/agent/image/v1/reference-assets/" + id + "/download",
		})
	}
	return assets
}

func canTransitionVisibility(currentStatus, nextStatus string) bool {
	switch nextStatus {
	case domainimagetask.VisibilityApproved:
		return currentStatus == domainimagetask.VisibilityPendingReview
	case domainimagetask.VisibilityRejected:
		return currentStatus == domainimagetask.VisibilityPendingReview || currentStatus == domainimagetask.VisibilityPrivate
	case domainimagetask.VisibilityUnpublished:
		return currentStatus == domainimagetask.VisibilityApproved
	default:
		return false
	}
}

func normalizePageBounds(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func decodeReferenceAssetIDs(value any) []string {
	if value == nil {
		return nil
	}
	data, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := data["reference_asset_ids"]
	if !ok || raw == nil {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if value, ok := item.(string); ok && strings.TrimSpace(value) != "" {
			ids = append(ids, value)
		}
	}
	return ids
}

func acquireEligiblePredicate(now time.Time) predicate.ImageTask {
	return imagetask.Or(
		imagetask.And(
			imagetask.StatusEQ(domainimagetask.StatusQueued),
			imagetask.Or(
				imagetask.ArtifactRecoveryStatusNEQ("pending"),
				imagetask.ArtifactNextRetryAtIsNil(),
				imagetask.ArtifactNextRetryAtLTE(now),
			),
		),
		imagetask.And(
			imagetask.StatusEQ(domainimagetask.StatusRunning),
			imagetask.Or(imagetask.LeaseExpiresAtIsNil(), imagetask.LeaseExpiresAtLT(now)),
		),
	)
}

func defaultTaskType(value string) string {
	if strings.TrimSpace(value) == "" {
		return string(provider.TaskTypeTextToImage)
	}
	return value
}

func defaultTaskStatus(value string) string {
	if strings.TrimSpace(value) == "" {
		return domainimagetask.StatusQueued
	}
	return value
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func defaultPositive(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func isTerminalStatus(status string) bool {
	return status == domainimagetask.StatusSucceeded || status == domainimagetask.StatusFailed
}

func nullableString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func nullableInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
