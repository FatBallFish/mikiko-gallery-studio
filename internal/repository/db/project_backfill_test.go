package db

import (
	"context"
	"fmt"
	"testing"

	"entgo.io/ent/dialect"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/google/uuid"
)

func TestBackfillLegacyProjectOwnershipCountsOnlyCommittedRows(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, fmt.Sprintf("file:project-backfill-%s?mode=memory&cache=shared&_fk=1", uuid.NewString()))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}
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

	updated, err := BackfillLegacyProjectOwnership(ctx, client, 10)
	if err == nil {
		t.Fatal("backfill unexpectedly committed after its transaction context was canceled")
	}
	if updated != 0 {
		t.Fatalf("reported updated rows = %d, want 0 because the transaction did not commit", updated)
	}
}
