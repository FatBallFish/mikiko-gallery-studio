package entstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	domainproject "github.com/fatballfish/pic-gallery/internal/domain/project"
	"github.com/fatballfish/pic-gallery/internal/provider"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imageresult"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imagetask"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	projectservice "github.com/fatballfish/pic-gallery/internal/service/project"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

func TestTaskProjectOwnershipCheckUsesCompatibleRowLock(t *testing.T) {
	table := entsql.Table("projects")
	selector := entsql.Dialect(dialect.Postgres).Select().From(table)
	lockProjectForTaskWrite()(selector)
	query, _ := selector.Query()
	if !strings.Contains(query, "FOR SHARE") {
		t.Fatalf("task project ownership query = %q, want FOR SHARE to serialize with project deletion", query)
	}
}

func TestWorkerTaskUpdateUsesRowLock(t *testing.T) {
	table := entsql.Table("image_tasks")
	selector := entsql.Dialect(dialect.Postgres).Select().From(table)
	lockImageTaskForWorkerUpdate()(selector)
	query, _ := selector.Query()
	if !strings.Contains(query, "FOR UPDATE") {
		t.Fatalf("worker task query = %q, want FOR UPDATE to reload ownership after project transfer", query)
	}

	sqliteSelector := entsql.Dialect(dialect.SQLite).Select().From(table)
	lockImageTaskForWorkerUpdate()(sqliteSelector)
	sqliteQuery, _ := sqliteSelector.Query()
	if strings.Contains(sqliteQuery, "FOR UPDATE") {
		t.Fatalf("SQLite worker task query = %q, must not contain unsupported FOR UPDATE", sqliteQuery)
	}
}

func TestUpdatedTaskResultsFollowPersistedProjectAfterTransferDelete(t *testing.T) {
	tests := []struct {
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
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			client, err := repoent.Open(dialect.SQLite, fmt.Sprintf("file:imagetask-project-transfer-worker-%s?mode=memory&cache=shared&_fk=1", uuid.NewString()))
			if err != nil {
				t.Fatalf("open ent client: %v", err)
			}
			t.Cleanup(func() { _ = client.Close() })
			if err := client.Schema.Create(ctx); err != nil {
				t.Fatalf("create schema: %v", err)
			}
			projects := projectservice.NewService(NewProjectStore(client))
			target, err := projects.EnsureDefault(ctx, 909)
			if err != nil {
				t.Fatalf("ensure target project: %v", err)
			}
			source, err := projects.Create(ctx, 909, domainproject.CreateRequest{Name: "Worker source"})
			if err != nil {
				t.Fatalf("create source project: %v", err)
			}
			now := time.Now().UTC()
			expiresAt := now.Add(time.Minute)
			task := domainimagetask.Task{
				ID: uuid.NewString(), UserID: 909, ProjectID: source.ID,
				Status: domainimagetask.StatusRunning, LeaseOwner: "worker-project-transfer", LeaseExpiresAt: &expiresAt,
				TaskType: string(provider.TaskTypeTextToImage), AbstractModel: "plus", Prompt: "persist in transferred project",
			}
			store := NewImageTaskStore(client)
			if err := store.Save(ctx, task); err != nil {
				t.Fatalf("seed running task: %v", err)
			}
			if _, err := projects.Delete(ctx, 909, source.ID, domainproject.DeleteRequest{TargetProjectID: target.ID, ExpectedVersion: source.Version}); err != nil {
				t.Fatalf("transfer-delete source project: %v", err)
			}
			task.Results = []provider.ImageResult{{ID: uuid.NewString(), ObjectKey: "worker/transferred.png", MimeType: "image/png"}}
			if err := tt.save(ctx, store, task, now); err != nil {
				t.Fatalf("save stale task result: %v", err)
			}
			taskEntity, err := client.ImageTask.Query().Where(imagetask.IDEQ(uuid.MustParse(task.ID))).Only(ctx)
			if err != nil {
				t.Fatalf("query transferred task: %v", err)
			}
			resultEntity, err := client.ImageResult.Query().Where(imageresult.TaskIDEQ(taskEntity.ID)).Only(ctx)
			if err != nil {
				t.Fatalf("query saved result: %v", err)
			}
			targetID := uuid.MustParse(target.ID)
			if taskEntity.ProjectID == nil || *taskEntity.ProjectID != targetID || resultEntity.ProjectID == nil || *resultEntity.ProjectID != targetID {
				t.Fatalf("task/result project after stale save = task %v, result %v, want %s", taskEntity.ProjectID, resultEntity.ProjectID, target.ID)
			}
			if count, countErr := client.ImageResult.Query().Where(imageresult.ProjectIDEQ(uuid.MustParse(source.ID))).Count(ctx); countErr != nil || count != 0 {
				t.Fatalf("deleted source gained results: count=%d err=%v", count, countErr)
			}
		})
	}
}

func TestImageTaskStoreLoadsUserConcurrencyLimit(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:imagetask-user-concurrency?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	user, err := client.User.Create().SetEmail("concurrency@example.com").SetStatus("active").SetConcurrencyLimit(3).Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	limit, err := NewImageTaskStore(client).UserConcurrencyLimit(ctx, int64(user.ID))
	if err != nil {
		t.Fatalf("UserConcurrencyLimit: %v", err)
	}
	if limit != 3 {
		t.Fatalf("expected user concurrency 3, got %d", limit)
	}
}

func TestImageTaskStorePersistsAndQueriesTasks(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:imagetaskstore?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewImageTaskStore(client)
	task := domainimagetask.Task{
		UserID:        12,
		APIKeyID:      88,
		SourceChannel: "openapi",
		ID:            "11111111-1111-1111-1111-111111111111",
		Status:        domainimagetask.StatusSucceeded,
		Provider:      "openrouter",
		AbstractModel: "plus",
		TaskType:      string(provider.TaskTypeTextToImage),

		BaseResolution:      "2k",
		OutputImageCount:    2,
		ReferenceImageCount: 2,
		ReferenceAssetIDs:   []string{"asset-a", "asset-b"},
		GenerationSnapshot: domainimagetask.GenerationSnapshot{
			CapabilityVersion: "capability-v1", SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "1:1",
			ResolvedSize: "896x896", ResolvedWidth: 896, ResolvedHeight: 896,
		},
		EstimatedPoints: "16.00000",
		ActualPoints:    "8.00000",
		Results:         []provider.ImageResult{{URL: "https://cdn.example.com/task.png"}},
	}
	if err := store.Save(ctx, task); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.GetByID(ctx, 12, task.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if loaded.ID != task.ID || loaded.Status != domainimagetask.StatusSucceeded {
		t.Fatalf("unexpected loaded task %#v", loaded)
	}
	if loaded.EstimatedPoints != "16.00000" || loaded.ActualPoints != "8.00000" {
		t.Fatalf("expected billing fields to round-trip, got %#v", loaded)
	}
	if loaded.APIKeyID != 88 || loaded.SourceChannel != "openapi" {
		t.Fatalf("expected api key source fields to round-trip, got %#v", loaded)
	}
	if len(loaded.ReferenceAssetIDs) != 2 || loaded.ReferenceAssetIDs[0] != "asset-a" || loaded.ReferenceAssetIDs[1] != "asset-b" {
		t.Fatalf("expected reference asset ids to round-trip, got %#v", loaded.ReferenceAssetIDs)
	}
	if loaded.GenerationSnapshot != task.GenerationSnapshot {
		t.Fatalf("expected immutable generation snapshot to round-trip, got %#v", loaded.GenerationSnapshot)
	}
	if len(loaded.Results) != 1 {
		t.Fatalf("expected persisted image result, got %#v", loaded.Results)
	}
	result := loaded.Results[0]
	if result.ID == "" || result.URL != "https://cdn.example.com/task.png" || result.VisibilityStatus != "private" {
		t.Fatalf("expected frontend usable remote result metadata, got %#v", result)
	}
	if result.MimeType == "" || result.FileSizeBytes != 0 || result.Width != 0 || result.Height != 0 {
		t.Fatalf("expected remote result to expose metadata fields with safe zero defaults, got %#v", result)
	}

	list, err := store.ListByUser(ctx, 12)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(list) != 1 || list[0].ID != task.ID {
		t.Fatalf("unexpected task list %#v", list)
	}

	if err := store.DeleteByID(ctx, 12, task.ID); err != nil {
		t.Fatalf("DeleteByID: %v", err)
	}

	if _, err := store.GetByID(ctx, 12, task.ID); !errors.Is(err, repoerr.ErrNotFound) {
		t.Fatalf("expected repoerr.ErrNotFound after delete, got %v", err)
	}

	list, err = store.ListByUser(ctx, 12)
	if err != nil {
		t.Fatalf("ListByUser after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list after delete, got %#v", list)
	}
}

func TestImageTaskStoreCancelPublishIsOwnerScopedAndReversible(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:imagetask-cancel-publish?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewImageTaskStore(client)
	task := domainimagetask.Task{
		UserID: 72, ID: "72727272-7272-7272-7272-727272727272", Status: domainimagetask.StatusSucceeded,
		AbstractModel: "basic", TaskType: string(provider.TaskTypeTextToImage), Prompt: "owner cancellation", Results: []provider.ImageResult{{
			ObjectKey: "generated/cancel.png", MimeType: "image/png", StorageDriver: "local",
			VisibilityStatus: domainimagetask.VisibilityPrivate,
		}},
	}
	if err := store.Save(ctx, task); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := store.GetByID(ctx, task.UserID, task.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	imageID := loaded.Results[0].ID
	if _, err := store.RequestPublish(ctx, task.UserID, imageID); err != nil {
		t.Fatalf("RequestPublish: %v", err)
	}
	if _, err := store.CancelPublish(ctx, task.UserID+1, imageID); !errors.Is(err, repoerr.ErrNotFound) {
		t.Fatalf("non-owner cancellation must be hidden as not found, got %v", err)
	}
	canceled, err := store.CancelPublish(ctx, task.UserID, imageID)
	if err != nil {
		t.Fatalf("CancelPublish pending: %v", err)
	}
	if canceled.VisibilityStatus != domainimagetask.VisibilityPrivate || canceled.ReviewReason != "" || canceled.PublishedAt != nil {
		t.Fatalf("unexpected pending cancellation %#v", canceled)
	}
	if _, err := store.RequestPublish(ctx, task.UserID, imageID); err != nil {
		t.Fatalf("RequestPublish again: %v", err)
	}
	publishedAt := time.Now().UTC()
	if _, err := store.ReviewImage(ctx, imageID, domainimagetask.VisibilityApproved, "", &publishedAt); err != nil {
		t.Fatalf("ReviewImage approve: %v", err)
	}
	canceled, err = store.CancelPublish(ctx, task.UserID, imageID)
	if err != nil {
		t.Fatalf("CancelPublish approved: %v", err)
	}
	if canceled.VisibilityStatus != domainimagetask.VisibilityPrivate || canceled.PublishedAt != nil {
		t.Fatalf("unexpected approved cancellation %#v", canceled)
	}
	if _, err := store.CancelPublish(ctx, task.UserID, imageID); err != nil {
		t.Fatalf("CancelPublish private idempotently: %v", err)
	}
	reapplied, err := store.RequestPublish(ctx, task.UserID, imageID)
	if err != nil || reapplied.VisibilityStatus != domainimagetask.VisibilityPendingReview {
		t.Fatalf("reapply after cancellation: image=%#v err=%v", reapplied, err)
	}
}

func TestAdminReviewFilterUsesCombinedDatabasePredicatesAndExactTotal(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:admin-review-filter?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	alice, err := client.User.Create().SetEmail("alice@example.com").SetNickname("Alice Studio").SetStatus("active").Save(ctx)
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, err := client.User.Create().SetEmail("bob@example.com").SetNickname("Bob").SetStatus("active").Save(ctx)
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}

	store := NewImageTaskStore(client)
	var windowStart, windowEnd, publishedWindowStart, publishedWindowEnd time.Time
	tasks := []domainimagetask.Task{
		{
			UserID: int64(alice.ID), ID: "81818181-8181-8181-8181-818181818181", Status: domainimagetask.StatusSucceeded,
			AbstractModel: "studio", RouteModelCode: "studio-v2", TaskType: string(provider.TaskTypeImageEdit),
			Prompt: "alpine portrait with warm light", BaseResolution: "2k", RequestedSize: "1536x1024", AspectRatio: "3:2",
			Results: []provider.ImageResult{{ID: "81818181-0000-0000-0000-818181818181", ObjectKey: "generated/alice.png", MimeType: "image/png", Width: 1536, Height: 1024, SHA256: "alice", StorageDriver: "local"}},
		},
		{
			UserID: int64(bob.ID), ID: "82828282-8282-8282-8282-828282828282", Status: domainimagetask.StatusSucceeded,
			AbstractModel: "basic", RouteModelCode: "basic-v1", TaskType: string(provider.TaskTypeTextToImage),
			Prompt: "city skyline", BaseResolution: "1k", RequestedSize: "1024x1024", AspectRatio: "1:1",
			Results: []provider.ImageResult{{ID: "82828282-0000-0000-0000-828282828282", ObjectKey: "generated/bob.png", MimeType: "image/png", Width: 1024, Height: 1024, SHA256: "bob", StorageDriver: "local"}},
		},
	}
	for _, task := range tasks {
		if err := store.Save(ctx, task); err != nil {
			t.Fatalf("Save %s: %v", task.ID, err)
		}
		if _, err := store.RequestPublish(ctx, task.UserID, task.Results[0].ID); err != nil {
			t.Fatalf("RequestPublish %s: %v", task.ID, err)
		}
		publishedAt := time.Now().UTC()
		approved, err := store.ReviewImage(ctx, task.Results[0].ID, domainimagetask.VisibilityApproved, "", &publishedAt)
		if err != nil {
			t.Fatalf("ReviewImage %s: %v", task.ID, err)
		}
		_ = approved
	}
	baseline, err := store.ListGallery(ctx, domainimagetask.GalleryListRequest{Page: 1, PageSize: 10, ReviewOnly: true})
	if err != nil {
		t.Fatalf("ListGallery baseline: %v", err)
	}
	for _, item := range baseline.Items {
		if item.TaskID != tasks[0].ID {
			continue
		}
		windowStart, windowEnd = item.CreatedAt.Add(-time.Second), item.CreatedAt.Add(time.Second)
		if item.PublishedAt != nil {
			publishedWindowStart, publishedWindowEnd = item.PublishedAt.Add(-time.Second), item.PublishedAt.Add(time.Second)
		}
	}

	page, err := store.ListGallery(ctx, domainimagetask.GalleryListRequest{
		Page: 1, PageSize: 1, ReviewOnly: true, Status: domainimagetask.VisibilityApproved,
		UserQuery: "alice", PromptQuery: "warm light", ModelQuery: "studio-v2",
		TaskType: string(provider.TaskTypeImageEdit), BaseResolution: "2k", RequestedSize: "1536x1024",
		Width: 1536, Height: 1024, AspectRatio: "3:2",
		CreatedFrom: windowStart, CreatedTo: windowEnd, PublishedFrom: publishedWindowStart, PublishedTo: publishedWindowEnd,
	})
	if err != nil {
		t.Fatalf("ListGallery combined filters: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].TaskID != tasks[0].ID {
		t.Fatalf("combined filters must produce an exact filtered total, got %#v", page)
	}

	byID, err := store.ListGallery(ctx, domainimagetask.GalleryListRequest{Page: 1, PageSize: 10, ReviewOnly: true, UserQuery: fmt.Sprintf("%d", alice.ID)})
	if err != nil {
		t.Fatalf("ListGallery user id: %v", err)
	}
	if byID.Total != 1 || len(byID.Items) != 1 || byID.Items[0].UserID != int64(alice.ID) {
		t.Fatalf("numeric user query must match exact user id, got %#v", byID)
	}
}

func TestImageTaskStoreProgressUpdatePreservesStateAndRejectsStaleOwner(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:imagetask-progress-owner?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	now := time.Now().UTC()
	expiresAt := now.Add(time.Minute)
	store := NewImageTaskStore(client)
	task := domainimagetask.Task{
		ID:               "14141414-1414-1414-1414-141414141414",
		UserID:           44,
		Status:           domainimagetask.StatusRunning,
		ProgressStage:    domainimagetask.ProgressStageProvider,
		ProgressMessage:  "provider",
		LeaseOwner:       "worker-b",
		LeaseExpiresAt:   &expiresAt,
		TaskType:         string(provider.TaskTypeTextToImage),
		AbstractModel:    "plus",
		BaseResolution:   "1k",
		ActualPoints:     "2.00000",
		OutputImageCount: 1,
		Results:          []provider.ImageResult{{ID: "15151515-1515-1515-1515-151515151515", URL: "https://example.test/result.png"}},
	}
	if err := store.Save(ctx, task); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.UpdateProgressIfOwned(ctx, task.ID, "worker-b", domainimagetask.ProgressStagePersisting, "persisting", now); err != nil {
		t.Fatalf("UpdateProgressIfOwned: %v", err)
	}
	loaded, err := store.GetByID(ctx, task.UserID, task.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if loaded.ProgressStage != domainimagetask.ProgressStagePersisting || loaded.ActualPoints != task.ActualPoints || len(loaded.Results) != 1 {
		t.Fatalf("progress update must preserve results and billing, got %#v", loaded)
	}
	if loaded.LeaseOwner != "worker-b" || loaded.LeaseExpiresAt == nil || !loaded.LeaseExpiresAt.Equal(expiresAt) {
		t.Fatalf("progress update must preserve lease, got %#v", loaded)
	}

	stale := loaded
	stale.Status = domainimagetask.StatusSucceeded
	stale.Results = []provider.ImageResult{{ID: "16161616-1616-1616-1616-161616161616", URL: "https://example.test/stale.png"}}
	if err := store.SaveTerminalState(ctx, stale, "worker-a", now); !errors.Is(err, repoerr.ErrConflict) {
		t.Fatalf("expected stale terminal save conflict, got %v", err)
	}
	afterConflict, err := store.GetByID(ctx, task.UserID, task.ID)
	if err != nil {
		t.Fatalf("GetByID after conflict: %v", err)
	}
	if afterConflict.Status != domainimagetask.StatusRunning || afterConflict.LeaseOwner != "worker-b" || len(afterConflict.Results) != 1 || afterConflict.Results[0].ID != loaded.Results[0].ID {
		t.Fatalf("stale terminal save must not replace reclaimed task, got %#v", afterConflict)
	}
}

func TestImageTaskStorePersistsLocalImageResultMetadata(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:imagetasklocalresult?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	imageBytes := []byte("fake-png")
	hash := sha256.Sum256(imageBytes)
	upstreamSucceededAt := time.Now().UTC().Add(-time.Second)
	nextRetryAt := time.Now().UTC().Add(time.Minute)
	store := NewImageTaskStore(client)
	task := domainimagetask.Task{
		UserID:              41,
		ID:                  "12121212-1212-1212-1212-121212121212",
		Status:              domainimagetask.StatusSucceeded,
		Provider:            "openai",
		AbstractModel:       "plus",
		TaskType:            string(provider.TaskTypeTextToImage),
		Prompt:              "store local result",
		SizeMode:            "ratio",
		BaseResolution:      "2k",
		Quality:             "auto",
		RequestedSize:       "1024x1024",
		OutputImageCount:    1,
		ProviderRequestID:   "provider-request-1",
		UpstreamSucceededAt: &upstreamSucceededAt,
		ArtifactRecovery: domainimagetask.ArtifactRecovery{
			Status: "pending", EncryptedPayload: `{"ciphertext":"v1:encrypted"}`, AttemptCount: 2, NextRetryAt: &nextRetryAt,
			StorageConfigID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", StorageVersion: 4,
			LastDiagnostic: domainimagetask.ArtifactDiagnostic{Code: "ARTIFACT_STORAGE_WRITE_FAILED", Stage: "store", Attempt: 2, Retryable: true, BytesRead: 68},
			Diagnostics:    []domainimagetask.ArtifactDiagnostic{{Code: "ARTIFACT_FETCH_TIMEOUT", Attempt: 1}, {Code: "ARTIFACT_STORAGE_WRITE_FAILED", Attempt: 2}},
		},
		Results: []provider.ImageResult{{
			StorageConfigID:  "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
			ObjectKey:        "generated-images/41/12121212-1212-1212-1212-121212121212/0.png",
			MimeType:         "image/png",
			FileSizeBytes:    int64(len(imageBytes)),
			Width:            2,
			Height:           1,
			SHA256:           hex.EncodeToString(hash[:]),
			StorageDriver:    "local",
			VisibilityStatus: "private",
			DownloadURL:      "/api/agent/image/v1/images/local-image-id",
		}},
	}
	if err := store.Save(ctx, task); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.GetByID(ctx, 41, task.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if len(loaded.Results) != 1 {
		t.Fatalf("expected one result, got %#v", loaded.Results)
	}
	result := loaded.Results[0]
	if result.ID == "" {
		t.Fatalf("expected persisted result id, got %#v", result)
	}
	if result.StorageDriver != "local" || result.ObjectKey != task.Results[0].ObjectKey {
		t.Fatalf("expected local object key to round-trip, got %#v", result)
	}
	if result.StorageConfigID != task.Results[0].StorageConfigID {
		t.Fatalf("expected storage config id to round-trip, got %#v", result)
	}
	if result.MimeType != "image/png" || result.FileSizeBytes != int64(len(imageBytes)) || result.Width != 2 || result.Height != 1 {
		t.Fatalf("expected local image metadata to round-trip, got %#v", result)
	}
	if result.SHA256 != task.Results[0].SHA256 || result.VisibilityStatus != "private" {
		t.Fatalf("expected hash and visibility to round-trip, got %#v", result)
	}
	if result.DownloadURL == "" {
		t.Fatalf("expected local result to expose download_url, got %#v", result)
	}
	if loaded.ProviderRequestID != task.ProviderRequestID || loaded.UpstreamSucceededAt == nil || !loaded.UpstreamSucceededAt.Equal(upstreamSucceededAt) {
		t.Fatalf("expected provider success checkpoint to round-trip, got %#v", loaded)
	}
	if loaded.ArtifactRecovery.Status != "pending" || loaded.ArtifactRecovery.EncryptedPayload != task.ArtifactRecovery.EncryptedPayload || loaded.ArtifactRecovery.AttemptCount != 2 {
		t.Fatalf("expected artifact recovery envelope to round-trip, got %#v", loaded.ArtifactRecovery)
	}
	if loaded.ArtifactRecovery.NextRetryAt == nil || loaded.ArtifactRecovery.LastDiagnostic.Code != "ARTIFACT_STORAGE_WRITE_FAILED" || len(loaded.ArtifactRecovery.Diagnostics) != 2 || loaded.ArtifactRecovery.StorageVersion != 4 {
		t.Fatalf("expected artifact retry state to round-trip, got %#v", loaded.ArtifactRecovery)
	}
}

func TestImageTaskStoreListsApprovedPublicImagesWithoutPublishedAt(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:imagetaskpublicgallery?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewImageTaskStore(client)
	task := domainimagetask.Task{
		UserID:        61,
		ID:            "61616161-6161-6161-6161-616161616161",
		Status:        domainimagetask.StatusSucceeded,
		Provider:      "openai",
		AbstractModel: "basic",
		TaskType:      string(provider.TaskTypeTextToImage),
		Prompt:        "public gallery image",

		BaseResolution:   "1k",
		OutputImageCount: 1,
		Results: []provider.ImageResult{{
			ObjectKey:        "generated-images/61/61616161-6161-6161-6161-616161616161/0.png",
			MimeType:         "image/png",
			SHA256:           "abc123",
			StorageDriver:    "local",
			VisibilityStatus: domainimagetask.VisibilityPrivate,
		}},
	}
	if err := store.Save(ctx, task); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := store.GetByID(ctx, 61, task.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	imageID := loaded.Results[0].ID
	if _, err := store.RequestPublish(ctx, 61, imageID); err != nil {
		t.Fatalf("RequestPublish: %v", err)
	}
	if _, err := store.ReviewImage(ctx, imageID, domainimagetask.VisibilityApproved, "", nil); err != nil {
		t.Fatalf("ReviewImage: %v", err)
	}

	publicPage, err := store.ListPublicGallery(ctx, domainimagetask.GalleryListRequest{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListPublicGallery: %v", err)
	}
	if publicPage.Total != 1 || len(publicPage.Items) != 1 || publicPage.Items[0].ID != imageID {
		t.Fatalf("expected approved image in public gallery, got %#v", publicPage)
	}
	filteredByModel, err := store.ListPublicGallery(ctx, domainimagetask.GalleryListRequest{
		Page:           1,
		PageSize:       10,
		RouteModelCode: "basic",
		TaskType:       string(provider.TaskTypeTextToImage),
	})
	if err != nil {
		t.Fatalf("ListPublicGallery filtered by model: %v", err)
	}
	if filteredByModel.Total != 1 || len(filteredByModel.Items) != 1 || filteredByModel.Items[0].ID != imageID {
		t.Fatalf("expected route_model_code/task_type filter to include image, got %#v", filteredByModel)
	}
	filteredOut, err := store.ListPublicGallery(ctx, domainimagetask.GalleryListRequest{
		Page:           1,
		PageSize:       10,
		RouteModelCode: "missing",
		TaskType:       string(provider.TaskTypeTextToImage),
	})
	if err != nil {
		t.Fatalf("ListPublicGallery filtered out by model: %v", err)
	}
	if filteredOut.Total != 0 || len(filteredOut.Items) != 0 {
		t.Fatalf("expected non-matching route_model_code filter to exclude image, got %#v", filteredOut)
	}
	filteredOutByTaskType, err := store.ListPublicGallery(ctx, domainimagetask.GalleryListRequest{
		Page:     1,
		PageSize: 10,
		TaskType: string(provider.TaskTypeImageEdit),
	})
	if err != nil {
		t.Fatalf("ListPublicGallery filtered out by task type: %v", err)
	}
	if filteredOutByTaskType.Total != 0 || len(filteredOutByTaskType.Items) != 0 {
		t.Fatalf("expected non-matching task_type filter to exclude image, got %#v", filteredOutByTaskType)
	}

	if _, err := store.SetPublicImageInteraction(ctx, 62, imageID, "like", true); err != nil {
		t.Fatalf("SetPublicImageInteraction like: %v", err)
	}
	if _, err := store.SetPublicImageInteraction(ctx, 62, imageID, "favorite", true); err != nil {
		t.Fatalf("SetPublicImageInteraction favorite: %v", err)
	}
	likedPage, err := store.ListPublicGallery(ctx, domainimagetask.GalleryListRequest{Page: 1, PageSize: 10, ViewerUserID: 62, LikedOnly: true})
	if err != nil {
		t.Fatalf("ListPublicGallery liked: %v", err)
	}
	if len(likedPage.Items) != 1 || !likedPage.Items[0].LikedByViewer || likedPage.Items[0].LikeCount != 1 {
		t.Fatalf("expected liked viewer state, got %#v", likedPage)
	}
	favoritedPage, err := store.ListPublicGallery(ctx, domainimagetask.GalleryListRequest{Page: 1, PageSize: 10, ViewerUserID: 62, FavoritedOnly: true})
	if err != nil {
		t.Fatalf("ListPublicGallery favorited: %v", err)
	}
	if len(favoritedPage.Items) != 1 || !favoritedPage.Items[0].FavoritedByViewer || favoritedPage.Items[0].FavoriteCount != 1 {
		t.Fatalf("expected favorited viewer state, got %#v", favoritedPage)
	}
}

func TestImageTaskStorePublicGalleryHotSortIgnoresLegacyCommentCount(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:imagetaskpublichot?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewImageTaskStore(client)
	likedTask := publicGalleryTask("71717171-7171-7171-7171-717171717171", "71717171-0000-0000-0000-717171717171")
	legacyCommentTask := publicGalleryTask("72727272-7272-7272-7272-727272727272", "72727272-0000-0000-0000-727272727272")
	if err := store.Save(ctx, likedTask); err != nil {
		t.Fatalf("Save liked task: %v", err)
	}
	if err := store.Save(ctx, legacyCommentTask); err != nil {
		t.Fatalf("Save legacy comment task: %v", err)
	}
	if _, err := store.RequestPublish(ctx, likedTask.UserID, likedTask.Results[0].ID); err != nil {
		t.Fatalf("RequestPublish liked: %v", err)
	}
	if _, err := store.RequestPublish(ctx, legacyCommentTask.UserID, legacyCommentTask.Results[0].ID); err != nil {
		t.Fatalf("RequestPublish legacy comment: %v", err)
	}
	if _, err := store.ReviewImage(ctx, likedTask.Results[0].ID, domainimagetask.VisibilityApproved, "", nil); err != nil {
		t.Fatalf("ReviewImage liked: %v", err)
	}
	if _, err := store.ReviewImage(ctx, legacyCommentTask.Results[0].ID, domainimagetask.VisibilityApproved, "", nil); err != nil {
		t.Fatalf("ReviewImage legacy comment: %v", err)
	}
	if _, err := store.SetPublicImageInteraction(ctx, 72, likedTask.Results[0].ID, "favorite", true); err != nil {
		t.Fatalf("SetPublicImageInteraction favorite: %v", err)
	}
	if _, err := client.PublicImageStat.Create().
		SetImageID(mustUUID(t, legacyCommentTask.Results[0].ID)).
		SetLikeCount(0).
		SetFavoriteCount(0).
		SetCommentCount(99).
		Save(ctx); err != nil {
		t.Fatalf("seed legacy comment count: %v", err)
	}

	page, err := store.ListPublicGallery(ctx, domainimagetask.GalleryListRequest{Page: 1, PageSize: 10, Sort: "hot"})
	if err != nil {
		t.Fatalf("ListPublicGallery hot: %v", err)
	}
	if len(page.Items) < 2 {
		t.Fatalf("expected two public images, got %#v", page)
	}
	if page.Items[0].ID != likedTask.Results[0].ID {
		t.Fatalf("hot sort should ignore legacy comment_count and rank favorite first, got %#v", page.Items)
	}
}

func publicGalleryTask(taskID, imageID string) domainimagetask.Task {
	return domainimagetask.Task{
		UserID:        72,
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

func mustUUID(t *testing.T, value string) uuid.UUID {
	t.Helper()
	parsed, err := uuid.Parse(value)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", value, err)
	}
	return parsed
}

func TestImageTaskStoreAcquireNextQueuedTaskAndReclaimExpiredLease(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:imagetasklease?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewImageTaskStore(client)
	task := domainimagetask.Task{
		UserID:        88,
		ID:            "22222222-2222-2222-2222-222222222222",
		Status:        domainimagetask.StatusQueued,
		AbstractModel: "plus",
		TaskType:      string(provider.TaskTypeTextToImage),
		Prompt:        "queue this",

		BaseResolution:   "2k",
		RequestedSize:    "auto",
		OutputImageCount: 1,
	}
	if err := store.Save(ctx, task); err != nil {
		t.Fatalf("Save queued task: %v", err)
	}

	now := time.Now().UTC()
	claimed, err := store.AcquireNextQueuedTask(ctx, "worker-a", now, 30*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextQueuedTask first claim: %v", err)
	}
	if claimed.LeaseOwner != "worker-a" || claimed.LeaseExpiresAt == nil {
		t.Fatalf("expected lease to be assigned, got %#v", claimed)
	}

	if _, err := store.AcquireNextQueuedTask(ctx, "worker-b", now.Add(time.Second), 30*time.Second); !errors.Is(err, repoerr.ErrNotFound) {
		t.Fatalf("expected repoerr.ErrNotFound while lease active, got %v", err)
	}

	expiredAt := now.Add(-time.Minute)
	claimed.LeaseExpiresAt = &expiredAt
	if err := store.Save(ctx, claimed); err != nil {
		t.Fatalf("Save expired claim: %v", err)
	}

	reclaimed, err := store.AcquireNextQueuedTask(ctx, "worker-b", now.Add(2*time.Second), 45*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextQueuedTask reclaim: %v", err)
	}
	if reclaimed.LeaseOwner != "worker-b" || reclaimed.LeaseExpiresAt == nil {
		t.Fatalf("expected worker-b to reclaim task, got %#v", reclaimed)
	}
}

func TestImageTaskStoreRenewTaskLeaseRequiresOwner(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:imagetaskrenew?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewImageTaskStore(client)
	task := domainimagetask.Task{
		UserID:        91,
		ID:            "33333333-3333-3333-3333-333333333333",
		Status:        domainimagetask.StatusQueued,
		AbstractModel: "plus",
		TaskType:      string(provider.TaskTypeTextToImage),
		Prompt:        "renew me",

		BaseResolution:   "2k",
		RequestedSize:    "auto",
		OutputImageCount: 1,
	}
	if err := store.Save(ctx, task); err != nil {
		t.Fatalf("Save queued task: %v", err)
	}

	now := time.Now().UTC()
	claimed, err := store.AcquireNextQueuedTask(ctx, "worker-a", now, 20*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextQueuedTask: %v", err)
	}
	if claimed.LeaseExpiresAt == nil {
		t.Fatalf("expected lease expiry on claimed task, got %#v", claimed)
	}
	firstExpiry := *claimed.LeaseExpiresAt

	renewed, err := store.RenewTaskLease(ctx, claimed.ID, "worker-a", now.Add(5*time.Second), 45*time.Second)
	if err != nil {
		t.Fatalf("RenewTaskLease: %v", err)
	}
	if renewed.LeaseExpiresAt == nil || !renewed.LeaseExpiresAt.After(firstExpiry) {
		t.Fatalf("expected renewed expiry after %v, got %v", firstExpiry, renewed.LeaseExpiresAt)
	}

	if _, err := store.RenewTaskLease(ctx, claimed.ID, "worker-b", now.Add(10*time.Second), 45*time.Second); !errors.Is(err, repoerr.ErrConflict) {
		t.Fatalf("expected repoerr.ErrConflict for non-owner renew, got %v", err)
	}
}

func TestImageTaskStoreSaveIfOwnedRejectsStaleWorkerWriteAfterReclaim(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:imagetaskstale?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewImageTaskStore(client)
	task := domainimagetask.Task{
		UserID:        92,
		ID:            "44444444-4444-4444-4444-444444444444",
		Status:        domainimagetask.StatusQueued,
		AbstractModel: "plus",
		TaskType:      string(provider.TaskTypeTextToImage),
		Prompt:        "stale write",

		BaseResolution:   "2k",
		RequestedSize:    "auto",
		OutputImageCount: 1,
	}
	if err := store.Save(ctx, task); err != nil {
		t.Fatalf("Save queued task: %v", err)
	}

	now := time.Now().UTC()
	claimedByA, err := store.AcquireNextQueuedTask(ctx, "worker-a", now, 30*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextQueuedTask worker-a: %v", err)
	}
	staleSnapshot := claimedByA

	expiredAt := now.Add(-time.Minute)
	claimedByA.LeaseExpiresAt = &expiredAt
	if err := store.Save(ctx, claimedByA); err != nil {
		t.Fatalf("Save expired task: %v", err)
	}
	if _, err := store.AcquireNextQueuedTask(ctx, "worker-b", now.Add(2*time.Second), 30*time.Second); err != nil {
		t.Fatalf("AcquireNextQueuedTask worker-b: %v", err)
	}

	staleSnapshot.Status = domainimagetask.StatusSucceeded
	staleSnapshot.LeaseOwner = ""
	staleSnapshot.LeaseExpiresAt = nil
	staleSnapshot.Results = []provider.ImageResult{{URL: "https://cdn.example.com/stale.png"}}
	if err := store.SaveIfOwned(ctx, staleSnapshot, "worker-a", now.Add(3*time.Second)); !errors.Is(err, repoerr.ErrConflict) {
		t.Fatalf("expected repoerr.ErrConflict for stale worker write, got %v", err)
	}
}

func TestImageTaskStoreSaveIfOwnedPreservesRenewedLeaseForRunningTask(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:imagetaskrenewedlease?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewImageTaskStore(client)
	task := domainimagetask.Task{
		UserID:        93,
		ID:            "55555555-5555-5555-5555-555555555555",
		Status:        domainimagetask.StatusQueued,
		AbstractModel: "plus",
		TaskType:      string(provider.TaskTypeTextToImage),
		Prompt:        "renewed lease",

		BaseResolution:   "2k",
		RequestedSize:    "auto",
		OutputImageCount: 1,
	}
	if err := store.Save(ctx, task); err != nil {
		t.Fatalf("Save queued task: %v", err)
	}

	now := time.Now().UTC()
	claimed, err := store.AcquireNextQueuedTask(ctx, "worker-a", now, 20*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextQueuedTask: %v", err)
	}
	renewed, err := store.RenewTaskLease(ctx, claimed.ID, "worker-a", now.Add(5*time.Second), 45*time.Second)
	if err != nil {
		t.Fatalf("RenewTaskLease: %v", err)
	}

	staleRunningSnapshot := claimed
	staleRunningSnapshot.Attempts = []domainimagetask.Attempt{{Provider: "openrouter", Status: domainimagetask.StatusFailed, Error: "retry me"}}
	if err := store.SaveIfOwned(ctx, staleRunningSnapshot, "worker-a", now.Add(6*time.Second)); err != nil {
		t.Fatalf("SaveIfOwned: %v", err)
	}

	loaded, err := store.GetByID(ctx, 93, claimed.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if loaded.LeaseExpiresAt == nil || renewed.LeaseExpiresAt == nil {
		t.Fatalf("expected persisted lease expiry, got task=%#v renewed=%#v", loaded, renewed)
	}
	if !loaded.LeaseExpiresAt.Equal(*renewed.LeaseExpiresAt) {
		t.Fatalf("expected latest renewed lease %v, got %v", renewed.LeaseExpiresAt, loaded.LeaseExpiresAt)
	}
}

func TestImageTaskStorePersistsProgressAndGuardsUpdatesByLeaseOwner(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:imagetaskprogress?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewImageTaskStore(client)
	task := domainimagetask.Task{
		UserID:           94,
		ID:               "66666666-6666-6666-6666-666666666666",
		Status:           domainimagetask.StatusQueued,
		ProgressStage:    "queued",
		ProgressMessage:  "task queued",
		AbstractModel:    "plus",
		TaskType:         string(provider.TaskTypeTextToImage),
		Prompt:           "track real progress",
		BaseResolution:   "2k",
		RequestedSize:    "auto",
		OutputImageCount: 1,
	}
	if err := store.Save(ctx, task); err != nil {
		t.Fatalf("Save queued task: %v", err)
	}

	loaded, err := store.GetByID(ctx, task.UserID, task.ID)
	if err != nil {
		t.Fatalf("GetByID queued task: %v", err)
	}
	if loaded.ProgressStage != "queued" || loaded.ProgressMessage != "task queued" {
		t.Fatalf("expected queued progress to round-trip, got stage=%q message=%q", loaded.ProgressStage, loaded.ProgressMessage)
	}

	now := time.Now().UTC()
	claimed, err := store.AcquireNextQueuedTask(ctx, "worker-a", now, 30*time.Second)
	if err != nil {
		t.Fatalf("AcquireNextQueuedTask: %v", err)
	}
	claimed.ProgressStage = "provider"
	claimed.ProgressMessage = "provider generating"
	if err := store.SaveIfOwned(ctx, claimed, "worker-b", now.Add(time.Second)); !errors.Is(err, repoerr.ErrConflict) {
		t.Fatalf("expected non-owner progress update to conflict, got %v", err)
	}
	if err := store.SaveIfOwned(ctx, claimed, "worker-a", now.Add(time.Second)); err != nil {
		t.Fatalf("owner progress update: %v", err)
	}

	loaded, err = store.GetByID(ctx, task.UserID, task.ID)
	if err != nil {
		t.Fatalf("GetByID updated task: %v", err)
	}
	if loaded.ProgressStage != "provider" || loaded.ProgressMessage != "provider generating" {
		t.Fatalf("expected provider progress to round-trip, got stage=%q message=%q", loaded.ProgressStage, loaded.ProgressMessage)
	}
}
