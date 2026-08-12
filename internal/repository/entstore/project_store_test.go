package entstore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	domainproject "github.com/fatballfish/pic-gallery/internal/domain/project"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imageresult"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imagetask"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediauploadsession"
	projectent "github.com/fatballfish/pic-gallery/internal/repository/ent/project"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videotask"
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

type countingProjectDriver struct {
	dialect.Driver
	queries atomic.Int64
}

func (d *countingProjectDriver) Query(ctx context.Context, query string, args, result any) error {
	d.queries.Add(1)
	return d.Driver.Query(ctx, query, args, result)
}

func TestProjectStoreListUsesBoundedAggregatesAndPreservesSnapshots(t *testing.T) {
	ctx := context.Background()
	driver, err := entsql.Open(dialect.SQLite, fmt.Sprintf("file:project-list-aggregate-%s?mode=memory&cache=shared&_fk=1", uuid.NewString()))
	if err != nil {
		t.Fatalf("open sqlite driver: %v", err)
	}
	counting := &countingProjectDriver{Driver: driver}
	client := repoent.NewClient(repoent.Driver(counting))
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	store := NewProjectStore(client)
	svc := projectservice.NewService(store)
	defaultProject, err := svc.EnsureDefault(ctx, 606)
	if err != nil {
		t.Fatalf("EnsureDefault: %v", err)
	}
	alpha, err := svc.Create(ctx, 606, domainproject.CreateRequest{Name: "Alpha"})
	if err != nil {
		t.Fatalf("Create Alpha: %v", err)
	}
	beta, err := svc.Create(ctx, 606, domainproject.CreateRequest{Name: "Beta"})
	if err != nil {
		t.Fatalf("Create Beta: %v", err)
	}
	for index, project := range []domainproject.Project{defaultProject, alpha, beta} {
		projectID := uuid.MustParse(project.ID)
		taskID := uuid.New()
		if _, err := client.ImageTask.Create().SetID(taskID).SetUserID(606).SetProjectID(projectID).
			SetTaskType("text_to_image").SetPrompt("active").SetAbstractModel("plus").Save(ctx); err != nil {
			t.Fatalf("seed active task %d: %v", index, err)
		}
		if _, err := client.ImageResult.Create().SetTaskID(taskID).SetUserID(606).SetProjectID(projectID).
			SetObjectKey(fmt.Sprintf("project/%d/active.png", index)).SetMimeType("image/png").SetSha256(fmt.Sprintf("active-%d", index)).Save(ctx); err != nil {
			t.Fatalf("seed active result %d: %v", index, err)
		}
	}
	deletedAt := time.Now().UTC()
	deletedTaskID := uuid.New()
	alphaID := uuid.MustParse(alpha.ID)
	if _, err := client.ImageTask.Create().SetID(deletedTaskID).SetUserID(606).SetProjectID(alphaID).
		SetTaskType("text_to_image").SetPrompt("deleted").SetAbstractModel("plus").SetDeletedAt(deletedAt).Save(ctx); err != nil {
		t.Fatalf("seed deleted task: %v", err)
	}
	if _, err := client.ImageResult.Create().SetTaskID(deletedTaskID).SetUserID(606).SetProjectID(alphaID).
		SetObjectKey("project/alpha/deleted.png").SetMimeType("image/png").SetSha256("deleted").SetDeletedAt(deletedAt).Save(ctx); err != nil {
		t.Fatalf("seed deleted result: %v", err)
	}

	counting.queries.Store(0)
	items, err := store.List(ctx, 606)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := counting.queries.Load(); got != 6 {
		t.Fatalf("project list queries = %d, want 6 bounded aggregate queries", got)
	}
	if len(items) != 3 || items[0].ID != defaultProject.ID || items[1].ID != alpha.ID || items[2].ID != beta.ID {
		t.Fatalf("project list order = %#v", items)
	}
	for _, item := range items {
		if item.TaskCount != 1 || item.AssetCount != 1 {
			t.Fatalf("project %s active counts = tasks %d assets %d, want 1/1", item.ID, item.TaskCount, item.AssetCount)
		}
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

func TestProjectStoreTransferIncludesUnifiedMediaVideoTasksAndActiveUploads(t *testing.T) {
	ctx, client := openProjectStoreTestClient(t, "multimedia-transfer")
	svc := projectservice.NewService(NewProjectStore(client))
	target, err := svc.EnsureDefault(ctx, 808)
	if err != nil {
		t.Fatal(err)
	}
	source, err := svc.Create(ctx, 808, domainproject.CreateRequest{Name: "Multimedia"})
	if err != nil {
		t.Fatal(err)
	}
	sourceID := uuid.MustParse(source.ID)
	targetID := uuid.MustParse(target.ID)

	imageTaskID, imageResultID := uuid.New(), uuid.New()
	if _, err := client.ImageTask.Create().SetID(imageTaskID).SetUserID(808).SetProjectID(sourceID).
		SetTaskType("text_to_image").SetPrompt("still").SetAbstractModel("plus").Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ImageResult.Create().SetID(imageResultID).SetTaskID(imageTaskID).SetUserID(808).SetProjectID(sourceID).
		SetObjectKey("projects/multimedia/still.png").SetMimeType("image/png").SetSha256("still").Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.MediaAsset.Create().SetID(imageResultID).SetUserID(808).SetProjectID(sourceID).
		SetLegacyImageResultID(imageResultID).SetName("still.png").SetNameKey("still.png").SetMediaType("image").
		SetSourceType("generated").SetStatus("ready").SetObjectKey("media/original/808/still.png").SetMimeType("image/png").SetFileSizeBytes(100).Save(ctx); err != nil {
		t.Fatal(err)
	}
	standaloneAssetID := uuid.New()
	if _, err := client.MediaAsset.Create().SetID(standaloneAssetID).SetUserID(808).SetProjectID(sourceID).
		SetName("voice.wav").SetNameKey("voice.wav").SetMediaType("audio").SetSourceType("local_upload").
		SetStatus("ready").SetObjectKey("media/original/808/voice.wav").SetMimeType("audio/wav").SetFileSizeBytes(200).Save(ctx); err != nil {
		t.Fatal(err)
	}
	videoTaskID := uuid.New()
	if _, err := client.VideoTask.Create().SetID(videoTaskID).SetUserID(808).SetProjectID(sourceID).
		SetTaskType("text_to_video").SetPromptTemplate("move").SetPromptBindingSnapshot(map[string]any{}).SetExecutionPrompt("move").
		SetRouteModelID(1).SetRouteModelCode("cinema").SetDurationSeconds(5).SetResolution("720p").SetAspectRatio("16:9").
		SetEstimatedPoints("1.00000").SetReservedPoints("1.00000").SetPricingSnapshot(map[string]any{}).SetRoutingSnapshot(map[string]any{}).
		SetIdempotencyKey("multimedia-video").SetRequestFingerprint("multimedia-video").Save(ctx); err != nil {
		t.Fatal(err)
	}
	uploadID := uuid.New()
	if _, err := client.MediaUploadSession.Create().SetID(uploadID).SetUserID(808).SetProjectID(sourceID).
		SetOriginalFilename("clip.mp4").SetDeclaredMediaType("video").SetDeclaredMimeType("video/mp4").SetDeclaredSizeBytes(1024).
		SetObjectKey("media/original/808/upload/clip.mp4").SetPartSize(1024).SetPartCount(1).SetStatus("uploading").
		SetReservedBytes(1024).SetIdempotencyKey("multimedia-upload").SetRequestFingerprint("multimedia-upload").
		SetExpiresAt(time.Now().UTC().Add(time.Hour)).Save(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Delete(ctx, 808, source.ID, domainproject.DeleteRequest{ExpectedVersion: source.Version}); err == nil {
		t.Fatal("multimedia project without transfer target must not be deleted")
	}
	result, err := svc.Delete(ctx, 808, source.ID, domainproject.DeleteRequest{
		TargetProjectID: target.ID, ExpectedVersion: source.Version, IdempotencyKey: "multimedia-transfer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Transferred.Tasks != 2 || result.Transferred.Assets != 3 {
		t.Fatalf("multimedia transfer counts = %#v, want 2 tasks and 3 assets", result.Transferred)
	}
	checks := []struct {
		name  string
		count func() (int, error)
	}{
		{"image tasks", func() (int, error) { return client.ImageTask.Query().Where(imagetask.ProjectIDEQ(targetID)).Count(ctx) }},
		{"image results", func() (int, error) {
			return client.ImageResult.Query().Where(imageresult.ProjectIDEQ(targetID)).Count(ctx)
		}},
		{"media assets", func() (int, error) {
			return client.MediaAsset.Query().Where(mediaasset.ProjectIDEQ(targetID)).Count(ctx)
		}},
		{"video tasks", func() (int, error) { return client.VideoTask.Query().Where(videotask.ProjectIDEQ(targetID)).Count(ctx) }},
		{"upload sessions", func() (int, error) {
			return client.MediaUploadSession.Query().Where(mediauploadsession.ProjectIDEQ(targetID)).Count(ctx)
		}},
	}
	wants := []int{1, 1, 2, 1, 1}
	for index, check := range checks {
		count, err := check.count()
		if err != nil || count != wants[index] {
			t.Fatalf("transferred %s = %d, %v; want %d", check.name, count, err, wants[index])
		}
	}
}

func TestEntProjectServiceRejectsOverlongTrimmedIdempotencyKeys(t *testing.T) {
	ctx, client := openProjectStoreTestClient(t, "idempotency-length")
	svc := projectservice.NewService(NewProjectStore(client))
	overlong := "\t" + strings.Repeat("e", 129) + "\n"
	if _, err := svc.Create(ctx, 404, domainproject.CreateRequest{Name: "Invalid key", IdempotencyKey: overlong}); !errors.Is(err, projectservice.ErrInvalid) {
		t.Fatalf("Create overlong idempotency error = %v, want ErrInvalid", err)
	}
	if count, err := client.Project.Query().Where(projectent.UserIDEQ(404), projectent.NameEQ("Invalid key")).Count(ctx); err != nil || count != 0 {
		t.Fatalf("overlong create persisted projects = %d, %v", count, err)
	}
	source, err := svc.Create(ctx, 404, domainproject.CreateRequest{Name: "Delete key source"})
	if err != nil {
		t.Fatalf("Create source: %v", err)
	}
	if _, err := svc.Delete(ctx, 404, source.ID, domainproject.DeleteRequest{ExpectedVersion: source.Version, IdempotencyKey: overlong}); !errors.Is(err, projectservice.ErrInvalid) {
		t.Fatalf("Delete overlong idempotency error = %v, want ErrInvalid", err)
	}
	if _, err := svc.ResolveOwned(ctx, 404, source.ID); err != nil {
		t.Fatalf("overlong delete key mutated source: %v", err)
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
