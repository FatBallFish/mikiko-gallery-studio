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
	if _, err := client.ImageTask.Create().SetID(uuid.New()).SetUserID(active).SetTaskType("text_to_image").SetPrompt("late legacy row").SetAbstractModel("plus").Save(ctx); err != nil {
		t.Fatalf("seed late null-project task: %v", err)
	}
	if _, err := RunProjectOwnershipBackfill(ctx, client, ProjectBackfillOptions{BatchSize: 1, MaxBatches: 1}); err == nil {
		t.Fatal("completed checkpoint replay skipped final null ownership validation")
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
	ctx, cancel := context.WithCancel(context.Background())
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
			value, mutateErr := next.Mutate(hookCtx, mutation)
			if _, ok := mutation.(*repoent.ImageResultMutation); ok && mutateErr == nil {
				cancel()
			}
			return value, mutateErr
		})
	})

	progress, err := RunProjectOwnershipBackfill(ctx, client, ProjectBackfillOptions{BatchSize: 10, MaxBatches: 100})
	if err == nil {
		t.Fatal("backfill unexpectedly committed after its transaction context was canceled")
	}
	if progress.UpdatedRows != 1 {
		t.Fatalf("reported updated rows = %d, want the one previously committed task batch", progress.UpdatedRows)
	}
	committedTasks, _ := client.ImageTask.Query().Where(imagetask.ProjectIDNotNil()).Count(context.Background())
	committedResults, _ := client.ImageResult.Query().Where(imageresult.ProjectIDNotNil()).Count(context.Background())
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
