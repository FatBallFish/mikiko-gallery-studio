package db

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imageresult"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imagetask"
	projectent "github.com/fatballfish/pic-gallery/internal/repository/ent/project"
	"github.com/google/uuid"
)

var errForcedProjectBackfillStop = errors.New("forced project backfill stop")
var errForcedProjectResultBatchFailure = errors.New("forced project result batch failure")

func TestProjectBackfillPersistsBoundedProgressAndResumesAllHistoricalRows(t *testing.T) {
	client := openProjectBackfillSQLite(t, "resume")
	ctx := context.Background()
	active := seedProjectBackfillUser(t, ctx, client, "active-backfill@example.com", nil, 5)
	deletedAt := time.Now().UTC()
	deleted := seedProjectBackfillUser(t, ctx, client, "deleted-backfill@example.com", &deletedAt, 1)

	first, err := RunProjectOwnershipBackfill(ctx, client, ProjectBackfillOptions{
		BatchSize: 2, MaxBatches: 1,
		afterBatch: func(ProjectBackfillProgress) error { return errForcedProjectBackfillStop },
	})
	if !errors.Is(err, errForcedProjectBackfillStop) || first.Completed {
		t.Fatalf("forced first invocation = %#v, %v", first, err)
	}
	checkpoint, err := client.MigrationCheckpoint.Query().Only(ctx)
	if err != nil || checkpoint.Phase == "" {
		t.Fatalf("persisted checkpoint = %#v, %v", checkpoint, err)
	}

	var current ProjectBackfillProgress
	for invocation := 0; invocation < 20; invocation++ {
		current, err = RunProjectOwnershipBackfill(ctx, client, ProjectBackfillOptions{BatchSize: 2, MaxBatches: 1})
		if err != nil {
			t.Fatalf("resume invocation %d: %v", invocation, err)
		}
		if current.Completed {
			break
		}
	}
	if !current.Completed || current.ProcessedRows != 12 {
		t.Fatalf("completed progress = %#v, want 12 task/result rows", current)
	}
	for _, userID := range []int64{active, deleted} {
		if count, countErr := client.Project.Query().Where(projectent.UserIDEQ(userID), projectent.IsDefaultEQ(true)).Count(ctx); countErr != nil || count != 1 {
			t.Fatalf("user %d default projects = %d, %v", userID, count, countErr)
		}
	}
	if count, _ := client.ImageTask.Query().Where(imagetask.ProjectIDIsNil()).Count(ctx); count != 0 {
		t.Fatalf("remaining null task projects = %d", count)
	}
	if count, _ := client.ImageResult.Query().Where(imageresult.ProjectIDIsNil()).Count(ctx); count != 0 {
		t.Fatalf("remaining null result projects = %d", count)
	}

	replayed, err := RunProjectOwnershipBackfill(ctx, client, ProjectBackfillOptions{BatchSize: 1, MaxBatches: 1})
	if err != nil || !replayed.Completed || replayed.ProcessedRows != current.ProcessedRows {
		t.Fatalf("completed replay = %#v, %v; want %#v", replayed, err, current)
	}
	lateTask, err := client.ImageTask.Create().SetID(uuid.New()).SetUserID(active).SetTaskType("text_to_image").SetPrompt("late legacy row").SetAbstractModel("plus").Save(ctx)
	if err != nil {
		t.Fatalf("seed late null-project task: %v", err)
	}
	if _, err := client.ImageResult.Create().SetTaskID(lateTask.ID).SetUserID(active).SetObjectKey("legacy/late.png").SetMimeType("image/png").SetSha256("late-result").Save(ctx); err != nil {
		t.Fatalf("seed late null-project result: %v", err)
	}

	reset, err := RunProjectOwnershipBackfill(ctx, client, ProjectBackfillOptions{BatchSize: 1, MaxBatches: 1})
	if err != nil || reset.Completed || reset.Phase != projectBackfillPhaseTasks {
		t.Fatalf("completed checkpoint repair reset = %#v, %v; want tasks phase", reset, err)
	}
	checkpoint, err = client.MigrationCheckpoint.Query().Only(ctx)
	if err != nil || checkpoint.AfterTaskID != nil || checkpoint.AfterResultID != nil || checkpoint.Completed {
		t.Fatalf("persisted repair checkpoint = %#v, %v; want cleared cursors", checkpoint, err)
	}

	repaired, err := RunProjectOwnershipBackfill(ctx, client, ProjectBackfillOptions{BatchSize: 1, MaxBatches: 10})
	if err != nil || !repaired.Completed || repaired.Phase != projectBackfillPhaseDone {
		t.Fatalf("late ownership repair = %#v, %v; want completed", repaired, err)
	}
	if lateTask, err = client.ImageTask.Get(ctx, lateTask.ID); err != nil || lateTask.ProjectID == nil {
		t.Fatalf("late task project after repair = %#v, %v", lateTask, err)
	}
	lateResult, err := client.ImageResult.Query().Where(imageresult.TaskIDEQ(lateTask.ID)).Only(ctx)
	if err != nil || lateResult.ProjectID == nil || *lateResult.ProjectID != *lateTask.ProjectID {
		t.Fatalf("late result project after repair = %#v, %v", lateResult, err)
	}
}

func TestProjectBackfillValidationResetsCursorForLowerUUIDOmission(t *testing.T) {
	client := openProjectBackfillSQLite(t, "lower-cursor")
	ctx := context.Background()
	userID := seedProjectBackfillUser(t, ctx, client, "lower-cursor@example.com", nil, 0)
	lowerID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	higherID := uuid.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff")
	if _, err := client.ImageTask.Create().SetID(lowerID).SetUserID(userID).SetTaskType("text_to_image").SetPrompt("missed below cursor").SetAbstractModel("plus").Save(ctx); err != nil {
		t.Fatalf("seed lower UUID task: %v", err)
	}
	if _, err := client.MigrationCheckpoint.Create().
		SetName(projectOwnershipMigrationName).
		SetPhase(projectBackfillPhaseTasks).
		SetAfterTaskID(higherID).
		Save(ctx); err != nil {
		t.Fatalf("seed advanced checkpoint: %v", err)
	}

	reset, err := RunProjectOwnershipBackfill(ctx, client, ProjectBackfillOptions{BatchSize: 1, MaxBatches: 3})
	if err != nil || reset.Completed || reset.Phase != projectBackfillPhaseTasks {
		t.Fatalf("lower UUID repair reset = %#v, %v; want tasks phase", reset, err)
	}
	checkpoint, err := client.MigrationCheckpoint.Query().Only(ctx)
	if err != nil || checkpoint.AfterTaskID != nil || checkpoint.AfterResultID != nil {
		t.Fatalf("lower UUID persisted reset = %#v, %v", checkpoint, err)
	}

	repaired, err := RunProjectOwnershipBackfill(ctx, client, ProjectBackfillOptions{BatchSize: 1, MaxBatches: 10})
	if err != nil || !repaired.Completed {
		t.Fatalf("lower UUID repair = %#v, %v", repaired, err)
	}
	task, err := client.ImageTask.Get(ctx, lowerID)
	if err != nil || task.ProjectID == nil {
		t.Fatalf("lower UUID task after repair = %#v, %v", task, err)
	}
}

func TestProjectBackfillValidationRejectsRemainingNullOwnership(t *testing.T) {
	client := openProjectBackfillSQLite(t, "validation")
	ctx := context.Background()
	seedProjectBackfillUser(t, ctx, client, "validation-backfill@example.com", nil, 1)
	if err := validateProjectOwnership(ctx, client); err == nil {
		t.Fatal("validation accepted task/result rows without project ownership")
	}
}

func TestProjectBackfillCountsOnlyCommittedRows(t *testing.T) {
	client := openProjectBackfillSQLite(t, "committed")
	ctx := context.Background()
	user, err := client.User.Create().SetEmail("project-backfill@example.com").SetStatus("active").Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	taskID := uuid.New()
	if _, err := client.ImageTask.Create().SetID(taskID).SetUserID(int64(user.ID)).SetTaskType("text_to_image").SetPrompt("legacy").SetAbstractModel("plus").Save(ctx); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := client.ImageResult.Create().SetTaskID(taskID).SetUserID(int64(user.ID)).SetObjectKey("legacy/result.png").SetMimeType("image/png").SetSha256("legacy-result").Save(ctx); err != nil {
		t.Fatalf("create result: %v", err)
	}

	client.Use(func(next repoent.Mutator) repoent.Mutator {
		return repoent.MutateFunc(func(hookCtx context.Context, mutation repoent.Mutation) (repoent.Value, error) {
			if _, ok := mutation.(*repoent.ImageResultMutation); ok {
				return nil, errForcedProjectResultBatchFailure
			}
			return next.Mutate(hookCtx, mutation)
		})
	})

	progress, err := RunProjectOwnershipBackfill(ctx, client, ProjectBackfillOptions{BatchSize: 10, MaxBatches: 100})
	if !errors.Is(err, errForcedProjectResultBatchFailure) {
		t.Fatalf("backfill result batch error = %v, want injected failure", err)
	}
	if progress.UpdatedRows != 1 {
		t.Fatalf("reported updated rows = %d, want the one previously committed task batch", progress.UpdatedRows)
	}
	committedTasks, err := client.ImageTask.Query().Where(imagetask.ProjectIDNotNil()).Count(ctx)
	if err != nil {
		t.Fatalf("count committed tasks: %v", err)
	}
	committedResults, err := client.ImageResult.Query().Where(imageresult.ProjectIDNotNil()).Count(ctx)
	if err != nil {
		t.Fatalf("count committed results: %v", err)
	}
	if committedTasks != 1 || committedResults != 0 {
		t.Fatalf("committed ownership after canceled result batch = tasks %d, results %d", committedTasks, committedResults)
	}
}

func openProjectBackfillSQLite(t *testing.T, name string) *repoent.Client {
	t.Helper()
	client, err := repoent.Open(dialect.SQLite, fmt.Sprintf("file:project-backfill-%s-%s?mode=memory&cache=shared&_fk=1", name, uuid.NewString()))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return client
}

func seedProjectBackfillUser(t *testing.T, ctx context.Context, client *repoent.Client, email string, deletedAt *time.Time, rows int) int64 {
	t.Helper()
	builder := client.User.Create().SetEmail(email).SetStatus("active")
	if deletedAt != nil {
		builder.SetDeletedAt(*deletedAt)
	}
	user, err := builder.Save(ctx)
	if err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	for index := range rows {
		taskID := uuid.New()
		if _, err := client.ImageTask.Create().SetID(taskID).SetUserID(int64(user.ID)).SetTaskType("text_to_image").SetPrompt("legacy").SetAbstractModel("plus").Save(ctx); err != nil {
			t.Fatalf("seed task %d: %v", index, err)
		}
		if _, err := client.ImageResult.Create().SetTaskID(taskID).SetUserID(int64(user.ID)).SetObjectKey(fmt.Sprintf("legacy/%d/%s.png", user.ID, taskID)).SetMimeType("image/png").SetSha256(taskID.String()).Save(ctx); err != nil {
			t.Fatalf("seed result %d: %v", index, err)
		}
	}
	return int64(user.ID)
}
