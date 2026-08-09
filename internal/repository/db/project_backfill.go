package db

import (
	"context"
	"fmt"
	"time"

	domainproject "github.com/fatballfish/pic-gallery/internal/domain/project"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imageresult"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imagetask"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/migrationcheckpoint"
	projectent "github.com/fatballfish/pic-gallery/internal/repository/ent/project"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/user"
	"github.com/google/uuid"
)

const (
	projectOwnershipMigrationName = "project_ownership_v3"
	projectBackfillPhaseUsers     = "users"
	projectBackfillPhaseTasks     = "tasks"
	projectBackfillPhaseResults   = "results"
	projectBackfillPhaseValidate  = "validate"
	projectBackfillPhaseDone      = "done"
)

type ProjectBackfillOptions struct {
	BatchSize  int
	MaxBatches int
	BatchPause time.Duration
	afterBatch func(ProjectBackfillProgress) error
}

type ProjectBackfillProgress struct {
	Phase         string
	Batches       int
	UpdatedRows   int
	ProcessedRows int
	Completed     bool
}

// RunProjectOwnershipBackfill performs a bounded amount of work. Each batch
// advances a durable checkpoint, so a later process resumes without rescanning.
func RunProjectOwnershipBackfill(ctx context.Context, client *repoent.Client, opts ProjectBackfillOptions) (ProjectBackfillProgress, error) {
	if client == nil {
		return ProjectBackfillProgress{}, fmt.Errorf("project backfill client is required")
	}
	if opts.BatchSize <= 0 || opts.BatchSize > 1000 {
		opts.BatchSize = 100
	}
	if opts.MaxBatches <= 0 {
		opts.MaxBatches = 100
	}
	checkpoint, err := loadProjectBackfillCheckpoint(ctx, client)
	if err != nil {
		return ProjectBackfillProgress{}, err
	}
	progress := projectBackfillProgress(checkpoint)
	if progress.Completed {
		reset, err := resetProjectBackfillForMissingOwnership(ctx, client, checkpoint)
		if err != nil {
			return progress, err
		}
		if reset {
			checkpoint, err = client.MigrationCheckpoint.Get(ctx, checkpoint.ID)
			if err != nil {
				return progress, fmt.Errorf("reload reset project backfill checkpoint: %w", err)
			}
			return projectBackfillProgress(checkpoint), nil
		}
		return progress, nil
	}
	for progress.Batches < opts.MaxBatches && !progress.Completed {
		updated, batchErr := runProjectBackfillBatch(ctx, client, checkpoint, opts.BatchSize)
		if batchErr != nil {
			return progress, batchErr
		}
		progress.Batches++
		progress.UpdatedRows += updated
		checkpoint, err = client.MigrationCheckpoint.Get(ctx, checkpoint.ID)
		if err != nil {
			return progress, fmt.Errorf("reload project backfill checkpoint: %w", err)
		}
		progress.Phase = checkpoint.Phase
		progress.ProcessedRows = checkpoint.ProcessedRows
		progress.Completed = checkpoint.Completed
		if opts.afterBatch != nil {
			if err := opts.afterBatch(progress); err != nil {
				return progress, err
			}
		}
		if !progress.Completed && opts.BatchPause > 0 && progress.Batches < opts.MaxBatches {
			timer := time.NewTimer(opts.BatchPause)
			select {
			case <-ctx.Done():
				timer.Stop()
				return progress, fmt.Errorf("pause project backfill: %w", ctx.Err())
			case <-timer.C:
			}
		}
	}
	return progress, nil
}

func runProjectBackfillBatch(ctx context.Context, client *repoent.Client, checkpoint *repoent.MigrationCheckpoint, batchSize int) (int, error) {
	switch checkpoint.Phase {
	case projectBackfillPhaseUsers:
		return backfillProjectUsers(ctx, client, checkpoint, batchSize)
	case projectBackfillPhaseTasks:
		return backfillProjectTasks(ctx, client, checkpoint, batchSize)
	case projectBackfillPhaseResults:
		return backfillProjectResults(ctx, client, checkpoint, batchSize)
	case projectBackfillPhaseValidate:
		reset, err := resetProjectBackfillForMissingOwnership(ctx, client, checkpoint)
		if err != nil {
			return 0, err
		}
		if reset {
			return 0, nil
		}
		if _, err := client.MigrationCheckpoint.UpdateOneID(checkpoint.ID).
			SetPhase(projectBackfillPhaseDone).SetCompleted(true).Save(ctx); err != nil {
			return 0, fmt.Errorf("complete project backfill checkpoint: %w", err)
		}
		return 0, nil
	case projectBackfillPhaseDone:
		return 0, nil
	default:
		return 0, fmt.Errorf("unsupported project backfill phase %q", checkpoint.Phase)
	}
}

func backfillProjectUsers(ctx context.Context, client *repoent.Client, checkpoint *repoent.MigrationCheckpoint, batchSize int) (int, error) {
	users, err := client.User.Query().Where(user.IDGT(checkpoint.AfterUserID)).
		Order(repoent.Asc(user.FieldID)).Limit(batchSize + 1).All(ctx)
	if err != nil {
		return 0, fmt.Errorf("list project backfill users: %w", err)
	}
	hasMore := len(users) > batchSize
	if hasMore {
		users = users[:batchSize]
	}
	afterID := checkpoint.AfterUserID
	for _, entity := range users {
		if _, err := ensureBackfillDefaultProject(ctx, client, int64(entity.ID)); err != nil {
			return 0, err
		}
		afterID = entity.ID
	}
	update := client.MigrationCheckpoint.UpdateOneID(checkpoint.ID).SetAfterUserID(afterID)
	if !hasMore {
		update.SetPhase(projectBackfillPhaseTasks)
	}
	if _, err := update.Save(ctx); err != nil {
		return 0, fmt.Errorf("checkpoint project user backfill: %w", err)
	}
	return 0, nil
}

func backfillProjectTasks(ctx context.Context, client *repoent.Client, checkpoint *repoent.MigrationCheckpoint, batchSize int) (int, error) {
	query := client.ImageTask.Query().Where(imagetask.ProjectIDIsNil())
	if checkpoint.AfterTaskID != nil {
		query.Where(imagetask.IDGT(*checkpoint.AfterTaskID))
	}
	tasks, err := query.Order(repoent.Asc(imagetask.FieldID)).Limit(batchSize + 1).All(ctx)
	if err != nil {
		return 0, fmt.Errorf("list null-project tasks: %w", err)
	}
	hasMore := len(tasks) > batchSize
	if hasMore {
		tasks = tasks[:batchSize]
	}
	projects, err := resolveBackfillProjectsForTasks(ctx, client, tasks)
	if err != nil {
		return 0, err
	}
	tx, err := client.Tx(ctx)
	if err != nil {
		return 0, fmt.Errorf("start project task backfill batch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	updated := 0
	var afterID *uuid.UUID
	for _, task := range tasks {
		count, updateErr := tx.ImageTask.Update().Where(imagetask.IDEQ(task.ID), imagetask.ProjectIDIsNil()).SetProjectID(projects[task.UserID]).Save(ctx)
		if updateErr != nil {
			return 0, fmt.Errorf("backfill task %s project: %w", task.ID, updateErr)
		}
		updated += count
		id := task.ID
		afterID = &id
	}
	checkpointUpdate := tx.MigrationCheckpoint.UpdateOneID(checkpoint.ID).AddProcessedRows(updated)
	if afterID != nil {
		checkpointUpdate.SetAfterTaskID(*afterID)
	}
	if !hasMore {
		checkpointUpdate.SetPhase(projectBackfillPhaseResults)
	}
	if _, err := checkpointUpdate.Save(ctx); err != nil {
		return 0, fmt.Errorf("checkpoint project task backfill: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit project task backfill: %w", err)
	}
	return updated, nil
}

func backfillProjectResults(ctx context.Context, client *repoent.Client, checkpoint *repoent.MigrationCheckpoint, batchSize int) (int, error) {
	query := client.ImageResult.Query().Where(imageresult.ProjectIDIsNil())
	if checkpoint.AfterResultID != nil {
		query.Where(imageresult.IDGT(*checkpoint.AfterResultID))
	}
	results, err := query.Order(repoent.Asc(imageresult.FieldID)).Limit(batchSize + 1).All(ctx)
	if err != nil {
		return 0, fmt.Errorf("list null-project results: %w", err)
	}
	hasMore := len(results) > batchSize
	if hasMore {
		results = results[:batchSize]
	}
	projects, err := resolveBackfillProjectsForResults(ctx, client, results)
	if err != nil {
		return 0, err
	}
	tx, err := client.Tx(ctx)
	if err != nil {
		return 0, fmt.Errorf("start project result backfill batch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	updated := 0
	var afterID *uuid.UUID
	for _, result := range results {
		count, updateErr := tx.ImageResult.Update().Where(imageresult.IDEQ(result.ID), imageresult.ProjectIDIsNil()).SetProjectID(projects[result.UserID]).Save(ctx)
		if updateErr != nil {
			return 0, fmt.Errorf("backfill result %s project: %w", result.ID, updateErr)
		}
		updated += count
		id := result.ID
		afterID = &id
	}
	checkpointUpdate := tx.MigrationCheckpoint.UpdateOneID(checkpoint.ID).AddProcessedRows(updated)
	if afterID != nil {
		checkpointUpdate.SetAfterResultID(*afterID)
	}
	if !hasMore {
		checkpointUpdate.SetPhase(projectBackfillPhaseValidate)
	}
	if _, err := checkpointUpdate.Save(ctx); err != nil {
		return 0, fmt.Errorf("checkpoint project result backfill: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit project result backfill: %w", err)
	}
	return updated, nil
}

func loadProjectBackfillCheckpoint(ctx context.Context, client *repoent.Client) (*repoent.MigrationCheckpoint, error) {
	checkpoint, err := client.MigrationCheckpoint.Query().Where(migrationcheckpoint.NameEQ(projectOwnershipMigrationName)).Only(ctx)
	if err == nil {
		return checkpoint, nil
	}
	if !repoent.IsNotFound(err) {
		return nil, fmt.Errorf("query project backfill checkpoint: %w", err)
	}
	checkpoint, err = client.MigrationCheckpoint.Create().SetName(projectOwnershipMigrationName).SetPhase(projectBackfillPhaseUsers).Save(ctx)
	if err != nil {
		if repoent.IsConstraintError(err) {
			checkpoint, err = client.MigrationCheckpoint.Query().Where(migrationcheckpoint.NameEQ(projectOwnershipMigrationName)).Only(ctx)
		}
		if err != nil {
			return nil, fmt.Errorf("create project backfill checkpoint: %w", err)
		}
	}
	return checkpoint, nil
}

func ensureBackfillDefaultProject(ctx context.Context, client *repoent.Client, userID int64) (uuid.UUID, error) {
	entity, err := client.Project.Query().Where(
		projectent.UserIDEQ(userID), projectent.IsDefaultEQ(true),
		projectent.StatusEQ(domainproject.StatusActive), projectent.DeletedAtIsNil(),
	).Only(ctx)
	if err == nil {
		return entity.ID, nil
	}
	if !repoent.IsNotFound(err) {
		return uuid.Nil, fmt.Errorf("query default project for user %d: %w", userID, err)
	}
	entity, err = client.Project.Create().SetUserID(userID).
		SetName(domainproject.DefaultName).SetNameKey(domainproject.DefaultName).
		SetIsDefault(true).SetStatus(domainproject.StatusActive).Save(ctx)
	if err != nil {
		if repoent.IsConstraintError(err) {
			entity, err = client.Project.Query().Where(
				projectent.UserIDEQ(userID), projectent.IsDefaultEQ(true),
				projectent.StatusEQ(domainproject.StatusActive), projectent.DeletedAtIsNil(),
			).Only(ctx)
		}
		if err != nil {
			return uuid.Nil, fmt.Errorf("ensure default project for user %d: %w", userID, err)
		}
	}
	return entity.ID, nil
}

func resolveBackfillProjectsForTasks(ctx context.Context, client *repoent.Client, tasks []*repoent.ImageTask) (map[int64]uuid.UUID, error) {
	projects := make(map[int64]uuid.UUID)
	for _, task := range tasks {
		if _, ok := projects[task.UserID]; ok {
			continue
		}
		projectID, err := ensureBackfillDefaultProject(ctx, client, task.UserID)
		if err != nil {
			return nil, err
		}
		projects[task.UserID] = projectID
	}
	return projects, nil
}

func resolveBackfillProjectsForResults(ctx context.Context, client *repoent.Client, results []*repoent.ImageResult) (map[int64]uuid.UUID, error) {
	projects := make(map[int64]uuid.UUID)
	for _, result := range results {
		if _, ok := projects[result.UserID]; ok {
			continue
		}
		projectID, err := ensureBackfillDefaultProject(ctx, client, result.UserID)
		if err != nil {
			return nil, err
		}
		projects[result.UserID] = projectID
	}
	return projects, nil
}

func validateProjectOwnership(ctx context.Context, client *repoent.Client) error {
	tasks, results, err := countMissingProjectOwnership(ctx, client)
	if err != nil {
		return err
	}
	if tasks != 0 || results != 0 {
		return fmt.Errorf("project ownership backfill incomplete: %d tasks and %d results remain", tasks, results)
	}
	return nil
}

func resetProjectBackfillForMissingOwnership(ctx context.Context, client *repoent.Client, checkpoint *repoent.MigrationCheckpoint) (bool, error) {
	tasks, results, err := countMissingProjectOwnership(ctx, client)
	if err != nil {
		return false, err
	}
	if tasks == 0 && results == 0 {
		return false, nil
	}
	update := client.MigrationCheckpoint.UpdateOneID(checkpoint.ID).SetCompleted(false)
	if tasks > 0 {
		update.SetPhase(projectBackfillPhaseTasks).ClearAfterTaskID().ClearAfterResultID()
	} else {
		update.SetPhase(projectBackfillPhaseResults).ClearAfterResultID()
	}
	if _, err := update.Save(ctx); err != nil {
		return false, fmt.Errorf("reset project backfill checkpoint: %w", err)
	}
	return true, nil
}

func countMissingProjectOwnership(ctx context.Context, client *repoent.Client) (int, int, error) {
	tasks, err := client.ImageTask.Query().Where(imagetask.ProjectIDIsNil()).Count(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("count tasks without project: %w", err)
	}
	results, err := client.ImageResult.Query().Where(imageresult.ProjectIDIsNil()).Count(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("count results without project: %w", err)
	}
	return tasks, results, nil
}

func projectBackfillProgress(checkpoint *repoent.MigrationCheckpoint) ProjectBackfillProgress {
	return ProjectBackfillProgress{
		Phase: checkpoint.Phase, ProcessedRows: checkpoint.ProcessedRows, Completed: checkpoint.Completed,
	}
}
