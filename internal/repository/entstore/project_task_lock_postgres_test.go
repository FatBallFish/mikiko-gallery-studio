package entstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	domainproject "github.com/fatballfish/pic-gallery/internal/domain/project"
	"github.com/fatballfish/pic-gallery/internal/provider"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imageresult"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imagetask"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	projectservice "github.com/fatballfish/pic-gallery/internal/service/project"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

func TestProjectTaskWritesSerializeWithPostgresDelete(t *testing.T) {
	ctx, database, clientA, clientB := openProjectTaskPostgres(t)
	projects := projectservice.NewService(NewProjectStore(clientA))
	target, err := projects.EnsureDefault(ctx, 7001)
	if err != nil {
		t.Fatalf("ensure target project: %v", err)
	}

	t.Run("create share lock commits before delete recount", func(t *testing.T) {
		source, err := projects.Create(ctx, 7001, domainproject.CreateRequest{Name: "Create first"})
		if err != nil {
			t.Fatalf("create source: %v", err)
		}
		taskID := uuid.New()
		task := projectLockTask(taskID, 7001, source.ID)
		createTx, err := clientB.Tx(ctx)
		if err != nil {
			t.Fatalf("start task create transaction: %v", err)
		}
		if err := createImageTask(ctx, createTx, taskID, task, map[string]any{}, map[string]any{}); err != nil {
			_ = createTx.Rollback()
			t.Fatalf("create task under FOR SHARE: %v", err)
		}
		deleteDone := make(chan error, 1)
		go func() {
			_, deleteErr := projects.Delete(context.Background(), 7001, source.ID, domainproject.DeleteRequest{TargetProjectID: target.ID, ExpectedVersion: source.Version})
			deleteDone <- deleteErr
		}()
		assertPostgresOperationBlocked(t, deleteDone, "delete while task create holds FOR SHARE")
		if err := createTx.Commit(); err != nil {
			t.Fatalf("commit task create: %v", err)
		}
		if err := awaitPostgresOperation(t, deleteDone, "delete after task create commit"); err != nil {
			t.Fatalf("delete after task create commit: %v", err)
		}
		entity, err := clientA.ImageTask.Query().Where(imagetask.IDEQ(taskID)).Only(ctx)
		if err != nil || entity.ProjectID == nil || entity.ProjectID.String() != target.ID {
			t.Fatalf("new task was not included in delete recount/transfer: %#v, %v", entity, err)
		}
	})

	t.Run("delete update lock commits before create predicate recheck", func(t *testing.T) {
		source, err := projects.Create(ctx, 7001, domainproject.CreateRequest{Name: "Delete first"})
		if err != nil {
			t.Fatalf("create source: %v", err)
		}
		deleteTx, err := database.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("start delete transaction: %v", err)
		}
		if _, err := deleteTx.ExecContext(ctx, `SELECT id FROM projects WHERE id = $1 FOR UPDATE`, source.ID); err != nil {
			_ = deleteTx.Rollback()
			t.Fatalf("lock project for delete: %v", err)
		}
		task := projectLockTask(uuid.New(), 7001, source.ID)
		createDone := make(chan error, 1)
		go func() { createDone <- NewImageTaskStore(clientB).Save(context.Background(), task) }()
		assertPostgresOperationBlocked(t, createDone, "task create while delete holds FOR UPDATE")
		if _, err := deleteTx.ExecContext(ctx, `UPDATE projects SET status = $2, deleted_at = CURRENT_TIMESTAMP, version = version + 1 WHERE id = $1`, source.ID, domainproject.StatusDeleted); err != nil {
			_ = deleteTx.Rollback()
			t.Fatalf("soft-delete locked project: %v", err)
		}
		if err := deleteTx.Commit(); err != nil {
			t.Fatalf("commit project delete: %v", err)
		}
		if err := awaitPostgresOperation(t, createDone, "task create after delete commit"); !errors.Is(err, repoerr.ErrNotFound) {
			t.Fatalf("task create after delete commit = %v, want not found", err)
		}
		if count, err := clientA.ImageTask.Query().Where(imagetask.IDEQ(uuid.MustParse(task.ID))).Count(ctx); err != nil || count != 0 {
			t.Fatalf("task inserted into deleted project: count=%d err=%v", count, err)
		}
	})

	t.Run("stale worker completion follows transferred task", func(t *testing.T) {
		source, err := projects.Create(ctx, 7001, domainproject.CreateRequest{Name: "Worker completion"})
		if err != nil {
			t.Fatalf("create source: %v", err)
		}
		task := projectLockTask(uuid.New(), 7001, source.ID)
		store := NewImageTaskStore(clientB)
		if err := store.Save(ctx, task); err != nil {
			t.Fatalf("seed running task: %v", err)
		}
		if _, err := projects.Delete(ctx, 7001, source.ID, domainproject.DeleteRequest{TargetProjectID: target.ID, ExpectedVersion: source.Version}); err != nil {
			t.Fatalf("transfer-delete source: %v", err)
		}
		task.Status = domainimagetask.StatusSucceeded
		task.Results = []provider.ImageResult{{ID: uuid.NewString(), ObjectKey: "postgres/worker-result.png", MimeType: "image/png"}}
		if err := store.SaveTerminalState(ctx, task, task.LeaseOwner, time.Now().UTC()); err != nil {
			t.Fatalf("save stale worker completion: %v", err)
		}
		result, err := clientA.ImageResult.Query().Where(imageresult.TaskIDEQ(uuid.MustParse(task.ID))).Only(ctx)
		if err != nil || result.ProjectID == nil || result.ProjectID.String() != target.ID {
			t.Fatalf("stale worker result did not follow transferred task: %#v, %v", result, err)
		}
		if count, err := clientA.ImageResult.Query().Where(imageresult.ProjectIDEQ(uuid.MustParse(source.ID))).Count(ctx); err != nil || count != 0 {
			t.Fatalf("deleted source gained worker result: count=%d err=%v", count, err)
		}
	})

	workerSaves := []struct {
		name string
		save func(context.Context, *ImageTaskStore, domainimagetask.Task, time.Time) error
	}{
		{name: "ordinary save", save: func(ctx context.Context, store *ImageTaskStore, task domainimagetask.Task, _ time.Time) error {
			task.Status = domainimagetask.StatusSucceeded
			return store.Save(ctx, task)
		}},
		{name: "lease-owned save", save: func(ctx context.Context, store *ImageTaskStore, task domainimagetask.Task, now time.Time) error {
			return store.SaveIfOwned(ctx, task, task.LeaseOwner, now)
		}},
		{name: "terminal save", save: func(ctx context.Context, store *ImageTaskStore, task domainimagetask.Task, now time.Time) error {
			task.Status = domainimagetask.StatusSucceeded
			return store.SaveTerminalState(ctx, task, task.LeaseOwner, now)
		}},
	}
	for _, workerSave := range workerSaves {
		t.Run("delete lock precedes "+workerSave.name, func(t *testing.T) {
			source, err := projects.Create(ctx, 7001, domainproject.CreateRequest{Name: "Delete barrier " + workerSave.name})
			if err != nil {
				t.Fatalf("create source: %v", err)
			}
			task := projectLockTask(uuid.New(), 7001, source.ID)
			store := NewImageTaskStore(clientB)
			if err := store.Save(ctx, task); err != nil {
				t.Fatalf("seed running task: %v", err)
			}

			deleteTx, err := database.BeginTx(ctx, nil)
			if err != nil {
				t.Fatalf("start delete barrier transaction: %v", err)
			}
			if _, err := deleteTx.ExecContext(ctx, `SELECT id FROM projects WHERE id = $1 FOR UPDATE`, source.ID); err != nil {
				_ = deleteTx.Rollback()
				t.Fatalf("lock source project: %v", err)
			}
			if _, err := deleteTx.ExecContext(ctx, `SELECT id FROM image_tasks WHERE id = $1 FOR UPDATE`, task.ID); err != nil {
				_ = deleteTx.Rollback()
				t.Fatalf("lock source task: %v", err)
			}

			task.Results = []provider.ImageResult{{ID: uuid.NewString(), ObjectKey: "postgres/delete-barrier-result.png", MimeType: "image/png"}}
			workerDone := make(chan error, 1)
			go func() { workerDone <- workerSave.save(context.Background(), store, task, time.Now().UTC()) }()
			assertPostgresOperationBlocked(t, workerDone, workerSave.name+" while delete holds task row")

			if _, err := deleteTx.ExecContext(ctx, `UPDATE image_tasks SET project_id = $2 WHERE id = $1`, task.ID, target.ID); err != nil {
				_ = deleteTx.Rollback()
				t.Fatalf("transfer locked task: %v", err)
			}
			if _, err := deleteTx.ExecContext(ctx, `UPDATE projects SET status = $2, deleted_at = CURRENT_TIMESTAMP, version = version + 1 WHERE id = $1`, source.ID, domainproject.StatusDeleted); err != nil {
				_ = deleteTx.Rollback()
				t.Fatalf("soft-delete source project: %v", err)
			}
			if err := deleteTx.Commit(); err != nil {
				t.Fatalf("commit delete barrier: %v", err)
			}
			if err := awaitPostgresOperation(t, workerDone, workerSave.name+" after delete commit"); err != nil {
				t.Fatalf("complete stale worker save: %v", err)
			}

			result, err := clientA.ImageResult.Query().Where(imageresult.TaskIDEQ(uuid.MustParse(task.ID))).Only(ctx)
			if err != nil || result.ProjectID == nil || result.ProjectID.String() != target.ID {
				t.Fatalf("barrier worker result did not follow transferred task: %#v, %v", result, err)
			}
			if count, err := clientA.ImageResult.Query().Where(imageresult.ProjectIDEQ(uuid.MustParse(source.ID))).Count(ctx); err != nil || count != 0 {
				t.Fatalf("deleted source gained barrier worker result: count=%d err=%v", count, err)
			}
		})
	}
}

func openProjectTaskPostgres(t *testing.T) (context.Context, *sql.DB, *repoent.Client, *repoent.Client) {
	t.Helper()
	adminURL := strings.TrimSpace(os.Getenv("PIC_GALLERY_TEST_POSTGRES_URL"))
	if adminURL == "" {
		t.Skip("set PIC_GALLERY_TEST_POSTGRES_URL to run PostgreSQL project/task lock integration")
	}
	ctx := context.Background()
	admin, err := sql.Open("postgres", adminURL)
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	schemaName := fmt.Sprintf("project_task_lock_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA `+pq.QuoteIdentifier(schemaName)); err != nil {
		t.Fatalf("create integration schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + pq.QuoteIdentifier(schemaName) + ` CASCADE`) })
	databaseURL := projectPostgresURLWithSearchPath(t, adminURL, schemaName)
	database, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatalf("open scoped integration database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	clientA, err := repoent.Open(dialect.Postgres, databaseURL)
	if err != nil {
		t.Fatalf("open first ent client: %v", err)
	}
	t.Cleanup(func() { _ = clientA.Close() })
	clientB, err := repoent.Open(dialect.Postgres, databaseURL)
	if err != nil {
		t.Fatalf("open second ent client: %v", err)
	}
	t.Cleanup(func() { _ = clientB.Close() })
	if err := clientA.Schema.Create(ctx); err != nil {
		t.Fatalf("create integration tables: %v", err)
	}
	return ctx, database, clientA, clientB
}

func projectLockTask(id uuid.UUID, userID int64, projectID string) domainimagetask.Task {
	expiresAt := time.Now().UTC().Add(time.Minute)
	return domainimagetask.Task{
		ID: id.String(), UserID: userID, ProjectID: projectID, Status: domainimagetask.StatusRunning,
		LeaseOwner: "project-lock-worker", LeaseExpiresAt: &expiresAt,
		TaskType: string(provider.TaskTypeTextToImage), AbstractModel: "plus", Prompt: "project lock integration",
	}
}

func assertPostgresOperationBlocked(t *testing.T, done <-chan error, operation string) {
	t.Helper()
	select {
	case err := <-done:
		t.Fatalf("%s completed before lock release: %v", operation, err)
	case <-time.After(150 * time.Millisecond):
	}
}

func awaitPostgresOperation(t *testing.T, done <-chan error, operation string) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not finish after lock release", operation)
		return nil
	}
}

func projectPostgresURLWithSearchPath(t *testing.T, rawURL, searchPath string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse integration database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", searchPath)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
