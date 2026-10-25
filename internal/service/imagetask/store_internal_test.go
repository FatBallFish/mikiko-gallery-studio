package imagetask

import (
	"context"
	"testing"
	"time"

	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
)

func TestMemoryStoreClaimRecoveryProgressAndHeartbeatPreservation(t *testing.T) {
	for _, status := range []string{domainimagetask.StatusQueued, domainimagetask.StatusRunning} {
		t.Run(status, func(t *testing.T) {
			ctx := context.Background()
			store := NewMemoryStore()
			now := time.Now().UTC()
			expired := now.Add(-time.Minute)
			task := domainimagetask.Task{
				ID: "recovery-progress", UserID: 91, Status: status,
				LeaseOwner: "old-worker", LeaseExpiresAt: &expired,
				ArtifactRecovery: domainimagetask.ArtifactRecovery{Status: "pending", EncryptedPayload: "encrypted"},
			}
			if err := store.Save(ctx, task); err != nil {
				t.Fatal(err)
			}
			claimed, err := store.AcquireNextQueuedTask(ctx, "worker", now, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			if claimed.ProgressStage != domainimagetask.ProgressStagePersisting || claimed.ProgressMessage != "正在恢复并保存生成结果" {
				t.Fatalf("claimed recovery progress = %q %q", claimed.ProgressStage, claimed.ProgressMessage)
			}
			for _, stage := range []string{domainimagetask.ProgressStagePersisting, domainimagetask.ProgressStageSettling} {
				if err := store.UpdateProgressIfOwned(ctx, task.ID, "worker", stage, "detailed progress", now); err != nil {
					t.Fatal(err)
				}
				renewed, err := store.RenewTaskLease(ctx, task.ID, "worker", now.Add(time.Second), 2*time.Minute)
				if err != nil {
					t.Fatal(err)
				}
				loaded, err := store.GetByID(ctx, task.UserID, task.ID)
				if err != nil {
					t.Fatal(err)
				}
				for _, snapshot := range []domainimagetask.Task{renewed, loaded} {
					if snapshot.ProgressStage != stage || snapshot.ProgressMessage != "detailed progress" || snapshot.ArtifactRecovery.EncryptedPayload != "encrypted" {
						t.Fatalf("heartbeat changed progress or recovery: %#v", snapshot)
					}
					if snapshot.LeaseOwner != "worker" || snapshot.LeaseExpiresAt == nil || !snapshot.LeaseExpiresAt.Equal(now.Add(time.Second+2*time.Minute)) {
						t.Fatalf("heartbeat lease = %q %v", snapshot.LeaseOwner, snapshot.LeaseExpiresAt)
					}
				}
			}
		})
	}
}

func TestMemoryStoreUpdateProgressIfOwnedPreservesTaskState(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	now := time.Now().UTC()
	expiresAt := now.Add(time.Minute)
	task := domainimagetask.Task{
		ID:              "progress-task",
		UserID:          91,
		Status:          domainimagetask.StatusRunning,
		ProgressStage:   domainimagetask.ProgressStageProvider,
		ProgressMessage: "provider",
		LeaseOwner:      "worker-a",
		LeaseExpiresAt:  &expiresAt,
		ActualPoints:    "2.00000",
		Results:         []provider.ImageResult{{ID: "image-a", URL: "https://example.test/image-a.png"}},
	}
	if err := store.Save(ctx, task); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.UpdateProgressIfOwned(ctx, task.ID, "worker-a", domainimagetask.ProgressStagePersisting, "persisting", now); err != nil {
		t.Fatalf("UpdateProgressIfOwned: %v", err)
	}

	loaded, err := store.GetByID(ctx, task.UserID, task.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if loaded.ProgressStage != domainimagetask.ProgressStagePersisting || loaded.ProgressMessage != "persisting" {
		t.Fatalf("expected progress metadata update, got %#v", loaded)
	}
	if loaded.ActualPoints != task.ActualPoints || len(loaded.Results) != 1 || loaded.Results[0].ID != "image-a" {
		t.Fatalf("progress update must preserve billing and results, got %#v", loaded)
	}
	if loaded.LeaseOwner != "worker-a" || loaded.LeaseExpiresAt == nil || !loaded.LeaseExpiresAt.Equal(expiresAt) {
		t.Fatalf("progress update must preserve lease, got %#v", loaded)
	}
	if err := store.UpdateProgressIfOwned(ctx, task.ID, "worker-b", domainimagetask.ProgressStageSettling, "settling", now); err == nil {
		t.Fatal("expected stale owner progress update to conflict")
	}
}

func TestMemoryStoreSaveTerminalStateRejectsReclaimedOwner(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	now := time.Now().UTC()
	expiresAt := now.Add(time.Minute)
	current := domainimagetask.Task{
		ID:             "terminal-owner-task",
		UserID:         92,
		Status:         domainimagetask.StatusRunning,
		LeaseOwner:     "worker-b",
		LeaseExpiresAt: &expiresAt,
	}
	if err := store.Save(ctx, current); err != nil {
		t.Fatalf("Save: %v", err)
	}
	stale := current
	stale.Status = domainimagetask.StatusSucceeded
	stale.LeaseOwner = ""
	stale.LeaseExpiresAt = nil
	stale.Results = []provider.ImageResult{{ID: "stale-result"}}
	if err := store.SaveTerminalState(ctx, stale, "worker-a", now); err == nil {
		t.Fatal("expected stale terminal save to conflict after reclaim")
	}
}

func TestMemoryStoreTerminalStateClearsExpandedPromptValues(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().UTC()
	expires := now.Add(time.Minute)
	task := domainimagetask.Task{
		ID: "template-task", UserID: 42, Status: domainimagetask.StatusRunning, LeaseOwner: "worker-a", LeaseExpiresAt: &expires,
		Prompt: "{{$地点}}", PromptTemplate: "{{$地点}}", PromptTemplateVersion: 1, ExecutionPrompt: "隐私地点值",
	}
	if err := store.Save(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	task.Status = domainimagetask.StatusSucceeded
	if err := store.SaveTerminalState(context.Background(), task, "worker-a", now); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.GetByID(context.Background(), 42, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Prompt != "{{$地点}}" || loaded.ExecutionPrompt != "{{$地点}}" {
		t.Fatalf("terminal prompt fields = public %q execution %q", loaded.Prompt, loaded.ExecutionPrompt)
	}
}

func TestGalleryImageFromMemoryTaskPreservesReusableCreationConfiguration(t *testing.T) {
	task := domainimagetask.Task{
		ID: "reuse-task", UserID: 42, Prompt: "reuse prompt", TaskType: string(provider.TaskTypeImageEdit),
		RouteModelCode: "plus", SizeMode: "pixel", RequestedSize: "1536x1024", BaseResolution: "2k",
		Quality: "high", AspectRatio: "3:2", OutputFormat: "webp", OutputCompression: 72,
		Moderation: "low", OutputImageCount: 4, ReferenceAssetIDs: []string{"ref-a", "ref-b"},
	}
	image := galleryImageFromMemoryTask(task, provider.ImageResult{ID: "image-a", URL: "https://example.test/image-a.webp"})
	if image.SizeMode != task.SizeMode || image.RequestedSize != task.RequestedSize || image.OutputFormat != task.OutputFormat || image.OutputCompression != task.OutputCompression || image.Moderation != task.Moderation || image.OutputImageCount != task.OutputImageCount {
		t.Fatalf("gallery image lost reusable creation configuration: %#v", image)
	}
}

func TestMemoryGalleryFiltersByOwnedProjectAndProjectsSnapshots(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	task := domainimagetask.Task{
		ID: "project-task-a", UserID: 42, ProjectID: "project-a",
		Project: &domainimagetask.ProjectSnapshot{ID: "project-a", Name: "A"}, Status: domainimagetask.StatusSucceeded,
		Results: []provider.ImageResult{{ID: "project-image-b", ProjectID: "project-b", Project: &domainimagetask.ProjectSnapshot{ID: "project-b", Name: "B"}}},
	}
	if err := store.Save(ctx, task); err != nil {
		t.Fatal(err)
	}
	page, err := store.ListGalleryByUser(ctx, 42, domainimagetask.GalleryListRequest{Page: 1, PageSize: 20, ProjectID: "project-b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "project-image-b" || page.Items[0].ProjectID != "project-b" || page.Items[0].Project == nil || page.Items[0].Project.Name != "B" {
		t.Fatalf("project filtered gallery = %#v", page.Items)
	}
}

func TestMemoryStorePublicGalleryHotSortIgnoresLegacyCommentCount(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	likedImageID := "liked-image"
	legacyCommentImageID := "legacy-comment-image"
	if err := store.Save(ctx, publicGalleryTask("liked-task", likedImageID)); err != nil {
		t.Fatalf("Save liked task: %v", err)
	}
	if err := store.Save(ctx, publicGalleryTask("legacy-comment-task", legacyCommentImageID)); err != nil {
		t.Fatalf("Save legacy comment task: %v", err)
	}
	if _, err := store.ReviewImage(ctx, likedImageID, domainimagetask.VisibilityApproved, "", nil); err != nil {
		t.Fatalf("ReviewImage liked: %v", err)
	}
	if _, err := store.ReviewImage(ctx, legacyCommentImageID, domainimagetask.VisibilityApproved, "", nil); err != nil {
		t.Fatalf("ReviewImage legacy comment: %v", err)
	}
	if _, err := store.SetPublicImageInteraction(ctx, 91, likedImageID, "favorite", true); err != nil {
		t.Fatalf("SetPublicImageInteraction favorite: %v", err)
	}

	store.publicStats[legacyCommentImageID] = &memoryPublicStats{comments: 99}

	page, err := store.ListPublicGallery(ctx, domainimagetask.GalleryListRequest{Page: 1, PageSize: 10, Sort: "hot"})
	if err != nil {
		t.Fatalf("ListPublicGallery hot: %v", err)
	}
	if len(page.Items) < 2 {
		t.Fatalf("expected two public images, got %#v", page)
	}
	if page.Items[0].ID != likedImageID {
		t.Fatalf("hot sort should ignore legacy comment_count and rank favorite first, got %#v", page.Items)
	}
}

func publicGalleryTask(taskID, imageID string) domainimagetask.Task {
	return domainimagetask.Task{
		UserID:        91,
		ID:            taskID,
		Status:        domainimagetask.StatusSucceeded,
		Provider:      "openai",
		AbstractModel: "basic",
		TaskType:      string(provider.TaskTypeTextToImage),
		Prompt:        "public gallery hot sort",

		BaseResolution:   "1k",
		OutputImageCount: 1,
		Results: []provider.ImageResult{{
			ID:               imageID,
			URL:              "https://cdn.example.com/" + imageID + ".png",
			VisibilityStatus: domainimagetask.VisibilityPrivate,
		}},
	}
}
