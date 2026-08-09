package entstore

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	domainproject "github.com/fatballfish/pic-gallery/internal/domain/project"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imageresult"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imagetask"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/predicate"
	projectent "github.com/fatballfish/pic-gallery/internal/repository/ent/project"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	projectservice "github.com/fatballfish/pic-gallery/internal/service/project"
	"github.com/google/uuid"
)

type ProjectStore struct{ client *repoent.Client }

func NewProjectStore(client *repoent.Client) *ProjectStore { return &ProjectStore{client: client} }

func createDefaultProjectInTx(ctx context.Context, tx *repoent.Tx, userID int64) (*repoent.Project, error) {
	return tx.Project.Create().
		SetUserID(userID).
		SetName(domainproject.DefaultName).
		SetNameKey(domainproject.DefaultName).
		SetIsDefault(true).
		SetStatus(domainproject.StatusActive).
		Save(ctx)
}

func (s *ProjectStore) EnsureDefault(ctx context.Context, userID int64) (domainproject.Project, error) {
	if s == nil || s.client == nil {
		return domainproject.Project{}, fmt.Errorf("ensure default project: nil client")
	}
	entity, err := s.client.Project.Query().Where(activeOwnedProject(userID), projectent.IsDefaultEQ(true)).Only(ctx)
	if err == nil {
		return s.mapWithCounts(ctx, entity)
	}
	if !repoent.IsNotFound(err) {
		return domainproject.Project{}, fmt.Errorf("query default project: %w", err)
	}
	entity, err = s.client.Project.Create().
		SetUserID(userID).
		SetName(domainproject.DefaultName).
		SetNameKey(domainproject.DefaultName).
		SetIsDefault(true).
		SetStatus(domainproject.StatusActive).
		Save(ctx)
	if err != nil {
		if !repoent.IsConstraintError(err) {
			return domainproject.Project{}, fmt.Errorf("create default project: %w", err)
		}
		entity, err = s.client.Project.Query().Where(activeOwnedProject(userID), projectent.IsDefaultEQ(true)).Only(ctx)
		if err != nil {
			return domainproject.Project{}, fmt.Errorf("reload concurrent default project: %w", err)
		}
	}
	return s.mapWithCounts(ctx, entity)
}

func (s *ProjectStore) List(ctx context.Context, userID int64) ([]domainproject.Project, error) {
	entities, err := s.client.Project.Query().
		Where(activeOwnedProject(userID)).
		Order(repoent.Desc(projectent.FieldIsDefault), repoent.Asc(projectent.FieldCreatedAt), repoent.Asc(projectent.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	items := make([]domainproject.Project, 0, len(entities))
	for _, entity := range entities {
		item, mapErr := s.mapWithCounts(ctx, entity)
		if mapErr != nil {
			return nil, mapErr
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *ProjectStore) Get(ctx context.Context, userID int64, projectID string) (domainproject.Project, error) {
	id, err := uuid.Parse(projectID)
	if err != nil {
		return domainproject.Project{}, repoerr.ErrNotFound
	}
	entity, err := s.client.Project.Query().Where(activeOwnedProject(userID), projectent.IDEQ(id)).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainproject.Project{}, repoerr.ErrNotFound
		}
		return domainproject.Project{}, fmt.Errorf("get project: %w", err)
	}
	return s.mapWithCounts(ctx, entity)
}

func (s *ProjectStore) Create(ctx context.Context, userID int64, name, nameKey, idempotencyKey string) (domainproject.Project, error) {
	if idempotencyKey != "" {
		existing, err := s.client.Project.Query().Where(projectent.UserIDEQ(userID), projectent.CreateKeyEQ(idempotencyKey)).Only(ctx)
		if err == nil {
			return s.mapWithCounts(ctx, existing)
		}
		if !repoent.IsNotFound(err) {
			return domainproject.Project{}, fmt.Errorf("query project idempotency key: %w", err)
		}
	}
	builder := s.client.Project.Create().
		SetUserID(userID).
		SetName(name).
		SetNameKey(nameKey).
		SetStatus(domainproject.StatusActive)
	if idempotencyKey != "" {
		builder.SetCreateKey(idempotencyKey)
	}
	entity, err := builder.Save(ctx)
	if err != nil {
		if repoent.IsConstraintError(err) {
			if idempotencyKey != "" {
				replayed, replayErr := s.client.Project.Query().Where(projectent.UserIDEQ(userID), projectent.CreateKeyEQ(idempotencyKey)).Only(ctx)
				if replayErr == nil {
					return s.mapWithCounts(ctx, replayed)
				}
			}
			return domainproject.Project{}, projectservice.ErrNameConflict
		}
		return domainproject.Project{}, fmt.Errorf("create project: %w", err)
	}
	return mapProjectEntity(entity), nil
}

func (s *ProjectStore) Rename(ctx context.Context, userID int64, projectID, name, nameKey string, expectedVersion int64) (domainproject.Project, error) {
	id, err := uuid.Parse(projectID)
	if err != nil {
		return domainproject.Project{}, repoerr.ErrNotFound
	}
	entity, err := s.client.Project.Update().
		Where(activeOwnedProject(userID), projectent.IDEQ(id), projectent.IsDefaultEQ(false), projectent.VersionEQ(expectedVersion)).
		SetName(name).
		SetNameKey(nameKey).
		AddVersion(1).
		Save(ctx)
	if err != nil {
		if repoent.IsConstraintError(err) {
			return domainproject.Project{}, projectservice.ErrNameConflict
		}
		return domainproject.Project{}, fmt.Errorf("rename project: %w", err)
	}
	if entity == 0 {
		current, getErr := s.client.Project.Query().Where(activeOwnedProject(userID), projectent.IDEQ(id)).Only(ctx)
		if repoent.IsNotFound(getErr) {
			return domainproject.Project{}, repoerr.ErrNotFound
		}
		if getErr != nil {
			return domainproject.Project{}, fmt.Errorf("check project version: %w", getErr)
		}
		if current.IsDefault {
			return domainproject.Project{}, projectservice.ErrDefaultImmutable
		}
		return domainproject.Project{}, projectservice.ErrProjectChanged
	}
	updated, err := s.client.Project.Get(ctx, id)
	if err != nil {
		return domainproject.Project{}, fmt.Errorf("reload renamed project: %w", err)
	}
	return s.mapWithCounts(ctx, updated)
}

func (s *ProjectStore) Delete(ctx context.Context, userID int64, projectID string, req domainproject.DeleteRequest) (domainproject.DeleteResult, error) {
	sourceID, err := uuid.Parse(projectID)
	if err != nil {
		return domainproject.DeleteResult{}, repoerr.ErrNotFound
	}
	var targetID uuid.UUID
	if req.TargetProjectID != "" {
		targetID, err = uuid.Parse(req.TargetProjectID)
		if err != nil {
			return domainproject.DeleteResult{}, repoerr.ErrNotFound
		}
	}
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return domainproject.DeleteResult{}, fmt.Errorf("start project deletion: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if req.IdempotencyKey != "" {
		replayed, replayErr := tx.Project.Query().Where(projectent.UserIDEQ(userID), projectent.DeleteKeyEQ(req.IdempotencyKey)).Only(ctx)
		if replayErr == nil {
			if replayed.ID != sourceID {
				return domainproject.DeleteResult{}, projectservice.ErrIdempotencyConflict
			}
			return deleteResultFromEntity(replayed), nil
		}
		if !repoent.IsNotFound(replayErr) {
			return domainproject.DeleteResult{}, fmt.Errorf("query project delete idempotency key: %w", replayErr)
		}
	}

	ids := []uuid.UUID{sourceID}
	if req.TargetProjectID != "" {
		ids = append(ids, targetID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	locked, err := tx.Project.Query().Where(projectent.IDIn(ids...), lockProjectRows()).Order(repoent.Asc(projectent.FieldID)).All(ctx)
	if err != nil {
		return domainproject.DeleteResult{}, fmt.Errorf("lock project deletion rows: %w", err)
	}
	byID := make(map[uuid.UUID]*repoent.Project, len(locked))
	for _, entity := range locked {
		byID[entity.ID] = entity
	}
	source := byID[sourceID]
	if source == nil || source.UserID != userID {
		return domainproject.DeleteResult{}, repoerr.ErrNotFound
	}
	if !isActiveProject(source) {
		if req.IdempotencyKey != "" && source.DeleteKey != nil && *source.DeleteKey == req.IdempotencyKey {
			return deleteResultFromEntity(source), nil
		}
		return domainproject.DeleteResult{}, repoerr.ErrNotFound
	}
	if source.IsDefault {
		return domainproject.DeleteResult{}, projectservice.ErrDefaultImmutable
	}
	if source.Version != req.ExpectedVersion {
		return domainproject.DeleteResult{}, projectservice.ErrProjectChanged
	}
	if req.TargetProjectID != "" {
		target := byID[targetID]
		if target == nil || target.UserID != userID || !isActiveProject(target) || target.ID == source.ID {
			return domainproject.DeleteResult{}, repoerr.ErrNotFound
		}
	}
	counts, err := countProjectOwnership(ctx, tx, userID, sourceID)
	if err != nil {
		return domainproject.DeleteResult{}, err
	}
	if counts.Tasks+counts.Assets > 0 && req.TargetProjectID == "" {
		return domainproject.DeleteResult{}, &projectservice.NonEmptyError{Counts: counts}
	}
	sourceBefore := mapProjectEntity(source)
	sourceBefore.TaskCount, sourceBefore.AssetCount = counts.Tasks, counts.Assets
	if req.TargetProjectID != "" {
		if _, err := tx.ImageTask.Update().Where(imagetask.UserIDEQ(userID), imagetask.ProjectIDEQ(sourceID)).SetProjectID(targetID).Save(ctx); err != nil {
			return domainproject.DeleteResult{}, fmt.Errorf("transfer project tasks: %w", err)
		}
		if _, err := tx.ImageResult.Update().Where(imageresult.UserIDEQ(userID), imageresult.ProjectIDEQ(sourceID)).SetProjectID(targetID).Save(ctx); err != nil {
			return domainproject.DeleteResult{}, fmt.Errorf("transfer project assets: %w", err)
		}
	}
	now := time.Now().UTC()
	deleteBuilder := tx.Project.UpdateOneID(sourceID).
		SetStatus(domainproject.StatusDeleted).
		SetDeletedAt(now).
		SetDeletedTaskCount(counts.Tasks).
		SetDeletedAssetCount(counts.Assets).
		AddVersion(1)
	if req.IdempotencyKey != "" {
		deleteBuilder.SetDeleteKey(req.IdempotencyKey)
	}
	deleted, err := deleteBuilder.Save(ctx)
	if err != nil {
		if repoent.IsConstraintError(err) && req.IdempotencyKey != "" {
			return domainproject.DeleteResult{}, projectservice.ErrIdempotencyConflict
		}
		return domainproject.DeleteResult{}, fmt.Errorf("soft-delete project: %w", err)
	}
	sourceAfter := mapProjectEntity(deleted)
	metadata := map[string]any{
		"tasks": counts.Tasks, "assets": counts.Assets,
		"source_before": sourceBefore, "source_after": sourceAfter,
		"request_id": req.RequestID, "idempotency_key": req.IdempotencyKey,
	}
	if req.TargetProjectID != "" {
		target := byID[targetID]
		targetCounts, countErr := countProjectOwnership(ctx, tx, userID, targetID)
		if countErr != nil {
			return domainproject.DeleteResult{}, countErr
		}
		targetAfter := mapProjectEntity(target)
		targetAfter.TaskCount, targetAfter.AssetCount = targetCounts.Tasks, targetCounts.Assets
		metadata["target_project_id"] = req.TargetProjectID
		metadata["target_after"] = targetAfter
	}
	if _, err := tx.AuditLog.Create().
		SetActorType("user").SetActorID(strconv.FormatInt(userID, 10)).
		SetAction("project.transfer_delete").SetTargetType("project").SetTargetID(projectID).
		SetMetadata(metadata).Save(ctx); err != nil {
		return domainproject.DeleteResult{}, fmt.Errorf("audit project deletion: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domainproject.DeleteResult{}, fmt.Errorf("commit project deletion: %w", err)
	}
	return domainproject.DeleteResult{Project: mapProjectEntity(deleted), Transferred: counts}, nil
}

func deleteResultFromEntity(entity *repoent.Project) domainproject.DeleteResult {
	return domainproject.DeleteResult{
		Project:     mapProjectEntity(entity),
		Transferred: domainproject.OwnershipCounts{Tasks: entity.DeletedTaskCount, Assets: entity.DeletedAssetCount},
	}
}

func (s *ProjectStore) mapWithCounts(ctx context.Context, entity *repoent.Project) (domainproject.Project, error) {
	counts, err := countProjectOwnershipClient(ctx, s.client, entity.UserID, entity.ID)
	if err != nil {
		return domainproject.Project{}, err
	}
	item := mapProjectEntity(entity)
	item.TaskCount, item.AssetCount = counts.Tasks, counts.Assets
	return item, nil
}

func mapProjectEntity(entity *repoent.Project) domainproject.Project {
	if entity == nil {
		return domainproject.Project{}
	}
	return domainproject.Project{ID: entity.ID.String(), UserID: entity.UserID, Name: entity.Name, NameKey: entity.NameKey, IsDefault: entity.IsDefault, Status: entity.Status, Version: entity.Version, CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt}
}

func activeOwnedProject(userID int64) predicate.Project {
	return projectent.And(projectent.UserIDEQ(userID), projectent.StatusEQ(domainproject.StatusActive), projectent.DeletedAtIsNil())
}

func isActiveProject(entity *repoent.Project) bool {
	return entity.Status == domainproject.StatusActive && entity.DeletedAt == nil
}

func lockProjectRows() predicate.Project {
	return func(selector *entsql.Selector) {
		if selector.Dialect() != dialect.SQLite {
			selector.ForUpdate()
		}
	}
}

func countProjectOwnership(ctx context.Context, tx *repoent.Tx, userID int64, projectID uuid.UUID) (domainproject.OwnershipCounts, error) {
	tasks, err := tx.ImageTask.Query().Where(imagetask.UserIDEQ(userID), imagetask.ProjectIDEQ(projectID), imagetask.DeletedAtIsNil()).Count(ctx)
	if err != nil {
		return domainproject.OwnershipCounts{}, fmt.Errorf("count project tasks: %w", err)
	}
	assets, err := tx.ImageResult.Query().Where(imageresult.UserIDEQ(userID), imageresult.ProjectIDEQ(projectID), imageresult.DeletedAtIsNil()).Count(ctx)
	if err != nil {
		return domainproject.OwnershipCounts{}, fmt.Errorf("count project assets: %w", err)
	}
	return domainproject.OwnershipCounts{Tasks: tasks, Assets: assets}, nil
}

func countProjectOwnershipClient(ctx context.Context, client *repoent.Client, userID int64, projectID uuid.UUID) (domainproject.OwnershipCounts, error) {
	tasks, err := client.ImageTask.Query().Where(imagetask.UserIDEQ(userID), imagetask.ProjectIDEQ(projectID), imagetask.DeletedAtIsNil()).Count(ctx)
	if err != nil {
		return domainproject.OwnershipCounts{}, fmt.Errorf("count project tasks: %w", err)
	}
	assets, err := client.ImageResult.Query().Where(imageresult.UserIDEQ(userID), imageresult.ProjectIDEQ(projectID), imageresult.DeletedAtIsNil()).Count(ctx)
	if err != nil {
		return domainproject.OwnershipCounts{}, fmt.Errorf("count project assets: %w", err)
	}
	return domainproject.OwnershipCounts{Tasks: tasks, Assets: assets}, nil
}

var _ projectservice.Store = (*ProjectStore)(nil)
