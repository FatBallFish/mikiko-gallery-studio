package entstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	_ "github.com/mattn/go-sqlite3"
)

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
		UserID:                12,
		APIKeyID:              88,
		SourceChannel:         "openapi",
		ID:                    "11111111-1111-1111-1111-111111111111",
		Status:                domainimagetask.StatusSucceeded,
		Provider:              "openrouter",
		AbstractModel:         "plus",
		TaskType:              string(provider.TaskTypeTextToImage),
		RequestedQuality:      "auto",
		ResolvedQualityBucket: "2k",
		OutputImageCount:      2,
		ReferenceImageCount:   2,
		ReferenceAssetIDs:     []string{"asset-a", "asset-b"},
		EstimatedPoints:       "16.00000",
		ActualPoints:          "8.00000",
		Results:               []provider.ImageResult{{URL: "https://cdn.example.com/task.png"}},
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
		UserID:                88,
		ID:                    "22222222-2222-2222-2222-222222222222",
		Status:                domainimagetask.StatusQueued,
		AbstractModel:         "plus",
		TaskType:              string(provider.TaskTypeTextToImage),
		Prompt:                "queue this",
		RequestedQuality:      "auto",
		ResolvedQualityBucket: "2k",
		RequestedSize:         "auto",
		OutputImageCount:      1,
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
		UserID:                91,
		ID:                    "33333333-3333-3333-3333-333333333333",
		Status:                domainimagetask.StatusQueued,
		AbstractModel:         "plus",
		TaskType:              string(provider.TaskTypeTextToImage),
		Prompt:                "renew me",
		RequestedQuality:      "auto",
		ResolvedQualityBucket: "2k",
		RequestedSize:         "auto",
		OutputImageCount:      1,
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
		UserID:                92,
		ID:                    "44444444-4444-4444-4444-444444444444",
		Status:                domainimagetask.StatusQueued,
		AbstractModel:         "plus",
		TaskType:              string(provider.TaskTypeTextToImage),
		Prompt:                "stale write",
		RequestedQuality:      "auto",
		ResolvedQualityBucket: "2k",
		RequestedSize:         "auto",
		OutputImageCount:      1,
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
		UserID:                93,
		ID:                    "55555555-5555-5555-5555-555555555555",
		Status:                domainimagetask.StatusQueued,
		AbstractModel:         "plus",
		TaskType:              string(provider.TaskTypeTextToImage),
		Prompt:                "renewed lease",
		RequestedQuality:      "auto",
		ResolvedQualityBucket: "2k",
		RequestedSize:         "auto",
		OutputImageCount:      1,
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
