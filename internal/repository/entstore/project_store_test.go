package entstore

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	domainproject "github.com/fatballfish/pic-gallery/internal/domain/project"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imageresult"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imagetask"
	projectservice "github.com/fatballfish/pic-gallery/internal/service/project"
	"github.com/google/uuid"
)

func TestProjectStoreEnsuresDefaultAndEnforcesOwnership(t *testing.T) {
	ctx, client := openProjectStoreTestClient(t, "ensure")
	store := NewProjectStore(client)
	svc := projectservice.NewService(store)

	first, err := svc.EnsureDefault(ctx, 101)
	if err != nil {
		t.Fatalf("EnsureDefault first: %v", err)
	}
	second, err := svc.EnsureDefault(ctx, 101)
	if err != nil || second.ID != first.ID {
		t.Fatalf("EnsureDefault second = %#v, %v", second, err)
	}
	count, err := client.Project.Query().Count(ctx)
	if err != nil || count != 1 {
		t.Fatalf("project count = %d, %v", count, err)
	}
	if _, err := svc.ResolveOwned(ctx, 202, first.ID); !errors.Is(err, projectservice.ErrNotFound) {
		t.Fatalf("foreign resolve err = %v, want not found", err)
	}
	created, err := svc.Create(ctx, 101, domainproject.CreateRequest{Name: "Launch", IdempotencyKey: "launch-once"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	replayed, err := svc.Create(ctx, 101, domainproject.CreateRequest{Name: "Ignored on replay", IdempotencyKey: "launch-once"})
	if err != nil || replayed.ID != created.ID {
		t.Fatalf("durable create replay = %#v, %v", replayed, err)
	}
}

func TestProjectStoreTransferIsAtomicAndAudited(t *testing.T) {
	ctx, client := openProjectStoreTestClient(t, "transfer")
	svc := projectservice.NewService(NewProjectStore(client))
	defaultProject, _ := svc.EnsureDefault(ctx, 303)
	source, err := svc.Create(ctx, 303, domainproject.CreateRequest{Name: "Source"})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	sourceID := uuid.MustParse(source.ID)
	for index := range 2 {
		taskID := uuid.New()
		if _, err := client.ImageTask.Create().
			SetID(taskID).
			SetUserID(303).
			SetProjectID(sourceID).
			SetTaskType("text_to_image").
			SetPrompt("project transfer").
			SetAbstractModel("plus").
			Save(ctx); err != nil {
			t.Fatalf("seed task: %v", err)
		}
		if _, err := client.ImageResult.Create().
			SetTaskID(taskID).
			SetUserID(303).
			SetProjectID(sourceID).
			SetObjectKey(fmt.Sprintf("projects/source/%d.png", index)).
			SetMimeType("image/png").
			SetSha256(fmt.Sprintf("sha-%d", index)).
			Save(ctx); err != nil {
			t.Fatalf("seed result: %v", err)
		}
	}
	deletedAt := time.Now().UTC()
	deletedTaskID := uuid.New()
	if _, err := client.ImageTask.Create().
		SetID(deletedTaskID).
		SetUserID(303).
		SetProjectID(sourceID).
		SetTaskType("text_to_image").
		SetPrompt("deleted project history").
		SetAbstractModel("plus").
		SetDeletedAt(deletedAt).
		Save(ctx); err != nil {
		t.Fatalf("seed deleted task: %v", err)
	}
	if _, err := client.ImageResult.Create().
		SetTaskID(deletedTaskID).
		SetUserID(303).
		SetProjectID(sourceID).
		SetObjectKey("projects/source/deleted.png").
		SetMimeType("image/png").
		SetSha256("sha-deleted").
		SetDeletedAt(deletedAt).
		Save(ctx); err != nil {
		t.Fatalf("seed deleted result: %v", err)
	}

	result, err := svc.Delete(ctx, 303, source.ID, domainproject.DeleteRequest{
		TargetProjectID: defaultProject.ID,
		ExpectedVersion: source.Version,
		IdempotencyKey:  "project-transfer-delete",
		RequestID:       "request-project-transfer-delete",
	})
	if err != nil {
		t.Fatalf("Delete with transfer: %v", err)
	}
	if result.Transferred.Tasks != 2 || result.Transferred.Assets != 2 {
		t.Fatalf("transfer result = %#v", result)
	}
	replayed, err := svc.Delete(ctx, 303, source.ID, domainproject.DeleteRequest{
		TargetProjectID: defaultProject.ID,
		ExpectedVersion: source.Version,
		IdempotencyKey:  "project-transfer-delete",
		RequestID:       "request-project-transfer-retry",
	})
	if err != nil || replayed.Project.ID != result.Project.ID || replayed.Transferred != result.Transferred {
		t.Fatalf("persisted delete replay = %#v, %v; want %#v", replayed, err, result)
	}
	targetID := uuid.MustParse(defaultProject.ID)
	if count, _ := client.ImageTask.Query().Where(imagetask.ProjectIDEQ(targetID)).Count(ctx); count != 3 {
		t.Fatalf("target task count = %d", count)
	}
	if count, _ := client.ImageResult.Query().Where(imageresult.ProjectIDEQ(targetID)).Count(ctx); count != 3 {
		t.Fatalf("target result count = %d", count)
	}
	audits, err := client.AuditLog.Query().All(ctx)
	if err != nil || len(audits) != 1 || audits[0].Action != "project.transfer_delete" {
		t.Fatalf("transfer audit = %#v, %v", audits, err)
	}
	metadata := audits[0].Metadata
	for _, key := range []string{"source_before", "source_after", "target_after", "request_id", "idempotency_key"} {
		if _, ok := metadata[key]; !ok {
			t.Fatalf("transfer audit metadata missing %q: %#v", key, metadata)
		}
	}
	if metadata["request_id"] != "request-project-transfer-delete" || metadata["idempotency_key"] != "project-transfer-delete" {
		t.Fatalf("transfer audit correlation metadata = %#v", metadata)
	}
}

func openProjectStoreTestClient(t *testing.T, name string) (context.Context, *repoent.Client) {
	t.Helper()
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, fmt.Sprintf("file:project-%s-%s?mode=memory&cache=shared&_fk=1", name, uuid.NewString()))
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return ctx, client
}
