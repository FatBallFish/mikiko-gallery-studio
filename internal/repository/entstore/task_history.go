package entstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/google/uuid"
)

func visibleHistory(kind string, userID int64) func(*sql.Selector) {
	return func(s *sql.Selector) {
		h := sql.Table("task_history_hidden")
		s.Where(sql.NotExists(sql.Select(h.C("task_id")).From(h).Where(sql.And(sql.EQ(h.C("task_kind"), kind), sql.EQ(h.C("user_id"), userID), sql.ColumnsEQ(h.C("task_id"), s.C("id"))))))
	}
}

func hideHistory(ctx context.Context, client *repoent.Client, kind string, userID int64, id, project string, bulk bool) (int, error) {
	table, terminal, zero := "image_tasks", "'succeeded','partial_failed','failed','rejected'", "NOT EXISTS (SELECT 1 FROM task_images r WHERE r.task_id = t.id)"
	if kind == "video" {
		table, terminal = "video_tasks", "'succeeded','partial','failed','cancelled'"
		zero = "t.success_output_count = 0 AND NOT EXISTS (SELECT 1 FROM video_task_items i WHERE i.task_id = t.id AND (i.status = 'succeeded' OR i.result_asset_id IS NOT NULL))"
	}
	driver := client.HistoryDriver()
	// SQL identifiers and terminal sets above are constants; all caller values are bound.
	t := sql.Table(table).As("t")
	selector := sql.Dialect(driver.Dialect()).Select().From(t)
	selector.SelectExpr(sql.ExprFunc(func(b *sql.Builder) {
		b.Arg(kind).Comma().Ident(t.C("user_id")).Comma().Ident(t.C("id")).Comma().Arg(time.Now().UTC())
	}))
	selector.Where(sql.EQ(t.C("user_id"), userID)).Where(sql.IsNull(t.C("deleted_at")))
	if project != "" {
		projectID, err := uuid.Parse(project)
		if err != nil {
			return 0, errs.BadRequest("invalid project_id")
		}
		selector.Where(sql.EQ(t.C("project_id"), projectID))
	}
	if bulk {
		selector.Where(sql.EQ(t.C("status"), "failed")).Where(sql.ExprP(zero))
	} else {
		taskID, err := uuid.Parse(id)
		if err != nil {
			return 0, repoerr.ErrNotFound
		}
		selector.Where(sql.EQ(t.C("id"), taskID)).Where(sql.ExprP("t.status IN (" + terminal + ")"))
	}
	h := sql.Table("task_history_hidden")
	selector.Where(sql.NotExists(sql.Select(h.C("task_id")).From(h).Where(sql.And(sql.EQ(h.C("task_kind"), kind), sql.EQ(h.C("user_id"), userID), sql.ColumnsEQ(h.C("task_id"), t.C("id"))))))
	// Insert from a selector is portable across PostgreSQL and SQLite.
	query, args := selector.Query()
	query = "INSERT INTO task_history_hidden (task_kind,user_id,task_id,hidden_at) " + query + " ON CONFLICT DO NOTHING"
	var result sql.Result
	if err := driver.Exec(ctx, query, args, &result); err != nil {
		return 0, fmt.Errorf("hide %s history: %w", kind, err)
	}
	count, err := result.RowsAffected()
	return int(count), err
}

func (s *ImageTaskStore) HideHistory(ctx context.Context, userID int64, id string) error {
	task, err := s.GetByID(ctx, userID, id)
	if err != nil {
		return err
	}
	switch task.Status {
	case "succeeded", "partial_failed", "failed", "rejected":
	default:
		return errs.New(409, errs.CodeConflict, "only terminal image tasks can be hidden")
	}
	_, err = hideHistory(ctx, s.client, "image", userID, id, "", false)
	return err
}
func (s *ImageTaskStore) ClearFailedHistory(ctx context.Context, userID int64, project string) (int, error) {
	return hideHistory(ctx, s.client, "image", userID, "", strings.TrimSpace(project), true)
}
func (s *VideoTaskStore) HideHistory(ctx context.Context, userID int64, id uuid.UUID) error {
	task, err := s.Get(ctx, userID, id)
	if err != nil {
		return err
	}
	switch task.Status {
	case "succeeded", "partial", "failed", "cancelled":
	default:
		return errs.New(409, errs.CodeConflict, "only terminal video tasks can be hidden")
	}
	_, err = hideHistory(ctx, s.client, "video", userID, id.String(), "", false)
	return err
}
func (s *VideoTaskStore) ClearFailedHistory(ctx context.Context, userID int64, project string) (int, error) {
	return hideHistory(ctx, s.client, "video", userID, "", strings.TrimSpace(project), true)
}
