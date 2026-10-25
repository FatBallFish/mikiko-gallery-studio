package entstore

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/pointledger"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/walletreservationallocation"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	videocallbackservice "github.com/fatballfish/pic-gallery/internal/service/videocallback"
	videotaskservice "github.com/fatballfish/pic-gallery/internal/service/videotask"
	worker "github.com/fatballfish/pic-gallery/internal/worker/video"
)

func TestVideoTaskStoreCreateWithReservationRollsBackTaskAndWallet(t *testing.T) {
	ctx, client := openVideoTaskStoreTestClient(t, "video-task-reservation-rollback")
	billing := NewBillingStore(client, 5)
	store := NewVideoTaskStore(client, billing)
	const userID int64 = 8801
	if _, err := billing.Adjust(ctx, billingservice.AdjustStoreRequest{
		UserID: userID, ChangePoints: "100.00000", Reason: "seed video task test",
	}); err != nil {
		t.Fatal(err)
	}
	project := mustCreateVideoTaskProject(t, ctx, client, userID)
	firstAsset := mustCreateVideoTaskAsset(t, ctx, client, userID, project.ID, "first")
	lastAsset := mustCreateVideoTaskAsset(t, ctx, client, userID, project.ID, "last")
	taskID := uuid.New()

	_, err := store.CreateWithReservation(ctx, CreateVideoTaskWithReservationRequest{
		Task: VideoTaskCreate{
			ID: taskID, UserID: userID, ProjectID: project.ID, TaskType: "image_to_video",
			PromptTemplate: "camera move", ExecutionPrompt: "camera move", RouteModelID: 8,
			RouteModelCode: "video-pro", DurationSeconds: 5, Resolution: "1080p", AspectRatio: "16:9",
			RequestedOutputCount: 2, EstimatedPoints: "30.00000", ReservedPoints: "30.00000",
			IdempotencyKey: "rollback-key", RequestFingerprint: "rollback-fingerprint",
		},
		Items: []VideoTaskItemCreate{{Ordinal: 0}, {Ordinal: 1}},
		Inputs: []VideoTaskInputCreate{
			{AssetID: firstAsset.ID, Role: "first_frame", Ordinal: 0},
			{AssetID: lastAsset.ID, Role: "first_frame", Ordinal: 0}, // unique-key failure after reserve
		},
		Reserve: billingservice.ReserveStoreRequest{
			UserID: userID, TaskID: taskID.String(), EstimatedPoints: "30.00000", Reason: "video generation reserve",
		},
	})
	if err == nil {
		t.Fatal("expected duplicate input role/ordinal to fail")
	}

	assertVideoTaskTransactionEmpty(t, ctx, client, taskID)
	balance, err := billing.GetBalance(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if balance.AvailablePoints != "100.00000" || balance.FrozenPoints != "0.00000" {
		t.Fatalf("wallet changed after rolled-back task transaction: %#v", balance)
	}
}

func TestVideoTaskStoreServicePortSupportsReplayListGetAndCancel(t *testing.T) {
	ctx, client := openVideoTaskStoreTestClient(t, "video-task-service-port")
	billing := NewBillingStore(client, 5)
	store := NewVideoTaskStore(client, billing)
	const userID int64 = 8810
	if _, err := billing.Adjust(ctx, billingservice.AdjustStoreRequest{UserID: userID, ChangePoints: "100.00000", Reason: "seed"}); err != nil {
		t.Fatal(err)
	}
	project := mustCreateVideoTaskProject(t, ctx, client, userID)
	asset := mustCreateVideoTaskAsset(t, ctx, client, userID, project.ID, "input")
	taskID, itemID := uuid.New(), uuid.New()
	record := videotaskservice.CreateRecord{
		Task: videotaskservice.Task{
			ID: taskID, UserID: userID, ProjectID: project.ID, SourceChannel: "web", TaskType: "image_to_video",
			Status: "queued", ProgressStage: "queued", PromptTemplate: "move", ExecutionPrompt: "move",
			RouteModelID: 7, RouteModelCode: "cinema", DurationSeconds: 5, Resolution: "720p", AspectRatio: "adaptive",
			RequestedOutputCount: 1, EstimatedPoints: "10.00000", ReservedPoints: "10.00000", ActualPoints: "0.00000",
			SettlementStatus: "reserved", IdempotencyKey: "task-port", RequestFingerprint: "fingerprint",
			Items: []videotaskservice.Item{{ID: itemID, Ordinal: 0, Status: "queued", Stage: "queued", Version: 1}},
		},
		Inputs:        []videotaskservice.CreateInputRecord{{ID: uuid.New(), AssetID: asset.ID, Role: "first_frame", Ordinal: 0, AssetSnapshot: map[string]any{"name": "input.png"}}},
		ReservePoints: "10.00000", ReserveReason: "video generation reserve",
	}
	created, replayed, err := store.Create(ctx, record)
	if err != nil || replayed || created.ID != taskID || len(created.Items) != 1 || len(created.Inputs) != 1 {
		t.Fatalf("Create() = %#v replayed=%v, %v", created, replayed, err)
	}
	if count, err := client.MediaAssetReference.Query().Count(ctx); err != nil || count != 1 {
		t.Fatalf("media references count=%d err=%v", count, err)
	}
	replay, found, err := store.FindByIdempotency(ctx, userID, "task-port")
	if err != nil || !found || replay.ID != taskID {
		t.Fatalf("FindByIdempotency() = %#v found=%v err=%v", replay, found, err)
	}
	page, err := store.List(ctx, videotaskservice.ListRequest{UserID: userID, ProjectID: &project.ID, Limit: 20})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("List() = %#v, %v", page, err)
	}
	if _, err := store.Get(ctx, userID+1, taskID); err == nil {
		t.Fatal("expected owner isolation")
	}
	cancelled, err := store.RequestCancel(ctx, userID, taskID, "cancel-one")
	if err != nil || cancelled.Items[0].Status != "cancelled" || cancelled.SettlementStatus != "finalized" {
		t.Fatalf("RequestCancel() = %#v, %v", cancelled, err)
	}
	repeated, err := store.RequestCancel(ctx, userID, taskID, "cancel-one")
	if err != nil || repeated.Items[0].Version != cancelled.Items[0].Version {
		t.Fatalf("repeat cancel = %#v, %v", repeated, err)
	}
	if _, err := store.RequestCancel(ctx, userID+1, taskID, "cancel-one"); !errors.Is(err, repoerr.ErrNotFound) {
		t.Fatalf("foreign cancel error = %v", err)
	}
}

func TestVideoTaskStoreListCursorDoesNotRepeatTheFirstPage(t *testing.T) {
	ctx, client := openVideoTaskStoreTestClient(t, "video-task-list-cursor")
	billing := NewBillingStore(client, 5)
	store := NewVideoTaskStore(client, billing)
	const userID int64 = 8811
	if _, err := billing.Adjust(ctx, billingservice.AdjustStoreRequest{UserID: userID, ChangePoints: "100.00000", Reason: "seed"}); err != nil {
		t.Fatal(err)
	}
	project := mustCreateVideoTaskProject(t, ctx, client, userID)
	for index := 0; index < 3; index++ {
		id := uuid.New()
		_, _, err := store.Create(ctx, videotaskservice.CreateRecord{
			Task: videotaskservice.Task{
				ID: id, UserID: userID, ProjectID: project.ID, SourceChannel: "web", TaskType: "text_to_video", Status: "queued", ProgressStage: "queued",
				PromptTemplate: "move", ExecutionPrompt: "move", RouteModelID: 7, RouteModelCode: "cinema", DurationSeconds: 5,
				Resolution: "720p", AspectRatio: "16:9", RequestedOutputCount: 1, EstimatedPoints: "1.00000", ReservedPoints: "1.00000",
				SettlementStatus: "reserved", IdempotencyKey: fmt.Sprintf("cursor-%d", index), RequestFingerprint: fmt.Sprintf("fingerprint-%d", index),
				Items: []videotaskservice.Item{{ID: uuid.New(), Ordinal: 0, Status: "queued", Stage: "queued", Version: 1}},
			},
			ReservePoints: "1.00000", ReserveReason: "reserve",
		})
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}
	first, err := store.List(ctx, videotaskservice.ListRequest{UserID: userID, Limit: 1})
	if err != nil || len(first.Items) != 1 || first.NextCursor == "" {
		t.Fatalf("first page=%#v err=%v", first, err)
	}
	second, err := store.List(ctx, videotaskservice.ListRequest{UserID: userID, Limit: 1, Cursor: first.NextCursor})
	if err != nil || len(second.Items) != 1 || second.Items[0].ID == first.Items[0].ID {
		t.Fatalf("second page=%#v err=%v", second, err)
	}
}

func TestVideoCallbackStoreDeduplicatesEventsPerAccount(t *testing.T) {
	ctx, client := openVideoTaskStoreTestClient(t, "video-callback-inbox")
	store := NewVideoCallbackStore(client)
	receivedAt := time.Date(2026, 8, 12, 13, 0, 0, 0, time.UTC)
	record := videocallbackservice.EventRecord{
		ProviderCode: "minimax", ModelAccountID: 9, ProviderEventID: "event-1", ProviderJobID: "job-1",
		PayloadSnapshot: map[string]any{"state": "running"}, ReceivedAt: receivedAt,
	}
	duplicate, err := store.RecordEvent(ctx, record)
	if err != nil || duplicate {
		t.Fatalf("first RecordEvent duplicate=%v err=%v", duplicate, err)
	}
	duplicate, err = store.RecordEvent(ctx, record)
	if err != nil || !duplicate {
		t.Fatalf("second RecordEvent duplicate=%v err=%v", duplicate, err)
	}
	if count, err := client.VideoProviderCallbackEvent.Query().Count(ctx); err != nil || count != 1 {
		t.Fatalf("callback inbox count=%d err=%v", count, err)
	}
}

func TestVideoTaskStoreCreateWithReservationRollsBackOnInsufficientBalance(t *testing.T) {
	ctx, client := openVideoTaskStoreTestClient(t, "video-task-insufficient-balance")
	billing := NewBillingStore(client, 5)
	store := NewVideoTaskStore(client, billing)
	const userID int64 = 8802
	project := mustCreateVideoTaskProject(t, ctx, client, userID)
	taskID := uuid.New()

	_, err := store.CreateWithReservation(ctx, CreateVideoTaskWithReservationRequest{
		Task: VideoTaskCreate{
			ID: taskID, UserID: userID, ProjectID: project.ID, TaskType: "text_to_video",
			PromptTemplate: "ocean", ExecutionPrompt: "ocean", RouteModelID: 9, RouteModelCode: "video-basic",
			DurationSeconds: 5, Resolution: "720p", AspectRatio: "16:9", RequestedOutputCount: 1,
			EstimatedPoints: "10.00000", ReservedPoints: "10.00000", IdempotencyKey: "no-balance",
			RequestFingerprint: "no-balance-fingerprint",
		},
		Items: []VideoTaskItemCreate{{Ordinal: 0}},
		Reserve: billingservice.ReserveStoreRequest{
			UserID: userID, TaskID: taskID.String(), EstimatedPoints: "10.00000", Reason: "video generation reserve",
		},
	})
	if err == nil {
		t.Fatal("expected insufficient balance")
	}
	assertVideoTaskTransactionEmpty(t, ctx, client, taskID)
}

func TestVideoTaskStoreCreateAndFinalizeAreAtomicAndIdempotent(t *testing.T) {
	ctx, client := openVideoTaskStoreTestClient(t, "video-task-finalize")
	billing := NewBillingStore(client, 5)
	store := NewVideoTaskStore(client, billing)
	const userID int64 = 8803
	if _, err := billing.Adjust(ctx, billingservice.AdjustStoreRequest{
		UserID: userID, ChangePoints: "100.00000", Reason: "seed video finalize test",
	}); err != nil {
		t.Fatal(err)
	}
	project := mustCreateVideoTaskProject(t, ctx, client, userID)
	taskID := uuid.New()

	created, err := store.CreateWithReservation(ctx, CreateVideoTaskWithReservationRequest{
		Task: VideoTaskCreate{
			ID: taskID, UserID: userID, ProjectID: project.ID, TaskType: "text_to_video",
			PromptTemplate: "city", ExecutionPrompt: "city", RouteModelID: 10, RouteModelCode: "video-pro",
			DurationSeconds: 10, Resolution: "1080p", AspectRatio: "16:9", RequestedOutputCount: 2,
			EstimatedPoints: "40.00000", ReservedPoints: "46.00000", IdempotencyKey: "success-key",
			RequestFingerprint: "success-fingerprint",
		},
		Items: []VideoTaskItemCreate{{Ordinal: 0}, {Ordinal: 1}},
		Reserve: billingservice.ReserveStoreRequest{
			UserID: userID, TaskID: taskID.String(), EstimatedPoints: "46.00000", Reason: "video generation reserve",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != taskID {
		t.Fatalf("created task id = %s", created.ID)
	}

	finalize := FinalizeVideoTaskRequest{
		TaskID: taskID, UserID: userID, Status: "partial_failed", SuccessOutputCount: 1,
		ActualPoints: "20.00000", UsageSummary: map[string]any{"successful_video_count": 1, "total_output_seconds": "10.000"},
	}
	if _, err := store.FinalizeWithBilling(ctx, finalize); err != nil {
		t.Fatal(err)
	}
	if _, err := store.FinalizeWithBilling(ctx, finalize); err != nil {
		t.Fatalf("idempotent finalize: %v", err)
	}

	task, err := client.VideoTask.Get(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.SettlementStatus != "finalized" || task.ActualPoints != "20.00000" || task.SuccessOutputCount != 1 {
		t.Fatalf("unexpected finalized task: %#v", task)
	}
	consumeCount, err := client.PointLedger.Query().Where(
		pointledger.TaskIDEQ(taskID), pointledger.LedgerTypeEQ("consume"), pointledger.TaskMediaTypeEQ("video"),
	).Count(ctx)
	if err != nil || consumeCount != 1 {
		t.Fatalf("video consume ledger count=%d err=%v", consumeCount, err)
	}
	balance, err := billing.GetBalance(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if balance.AvailablePoints != "80.00000" || balance.FrozenPoints != "0.00000" {
		t.Fatalf("unexpected finalized wallet balance: %#v", balance)
	}
}

func TestVideoAPIKeyQuotaReserveSettleReplayAndRollback(t *testing.T) {
	for _, actual := range []string{"0.00000", "4.00000"} {
		t.Run(actual, func(t *testing.T) {
			ctx, client := openVideoTaskStoreTestClient(t, "video-key-settle-"+actual)
			billing := NewBillingStore(client, 5)
			store := NewVideoTaskStore(client, billing)
			record, key := videoKeyQuotaFixture(t, ctx, client, billing, "10.00000", "100.00000")
			created, replay, err := store.Create(ctx, record)
			if err != nil || replay || created.APIKeyID != int64(key.ID) {
				t.Fatalf("create attribution: task=%#v replay=%v err=%v", created, replay, err)
			}
			if _, replay, err := store.Create(ctx, record); err != nil || !replay {
				t.Fatalf("replay=%v err=%v", replay, err)
			}
			assertVideoKeyUsage(t, ctx, client, key.ID, "10.00000")
			for _, item := range created.Items {
				if err := client.VideoTaskItem.UpdateOneID(item.ID).SetStatus("failed").Exec(ctx); err != nil {
					t.Fatal(err)
				}
			}
			settle := worker.FinalizeRequest{TaskID: created.ID.String(), Status: "failed", ActualPoints: actual, ReservedPoints: "10.00000"}
			if done, err := store.FinalizeTask(ctx, settle); err != nil || !done {
				t.Fatalf("settle done=%v err=%v", done, err)
			}
			if done, err := store.FinalizeTask(ctx, settle); err != nil || done {
				t.Fatalf("repeat settle done=%v err=%v", done, err)
			}
			assertVideoKeyUsage(t, ctx, client, key.ID, actual)
			usage, err := billing.APIKeyUsage(ctx, int64(key.ID), nil)
			if err != nil || usage != actual {
				t.Fatalf("ledger usage=%s err=%v", usage, err)
			}
			count, err := client.PointLedger.Query().Where(pointledger.TaskIDEQ(created.ID), pointledger.APIKeyIDEQ(int64(key.ID))).Count(ctx)
			if err != nil || count < 2 {
				t.Fatalf("attributed ledger count=%d err=%v", count, err)
			}
		})
	}
	for _, tc := range []struct{ name, limit, balance string }{{"limit", "9.00000", "100.00000"}, {"wallet", "100.00000", "1.00000"}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, client := openVideoTaskStoreTestClient(t, "video-key-rollback-"+tc.name)
			billing := NewBillingStore(client, 5)
			record, key := videoKeyQuotaFixture(t, ctx, client, billing, tc.limit, tc.balance)
			if _, _, err := NewVideoTaskStore(client, billing).Create(ctx, record); err == nil {
				t.Fatal("expected quota/wallet rejection")
			}
			assertVideoTaskTransactionEmpty(t, ctx, client, record.Task.ID)
			assertVideoKeyUsage(t, ctx, client, key.ID, "0.00000")
			if count, err := client.APIKeyQuotaReservation.Query().Count(ctx); err != nil || count != 0 {
				t.Fatalf("reservation leak count=%d err=%v", count, err)
			}
		})
	}
}

func TestVideoAPIKeyQuotaIncludesImageLedgerUsage(t *testing.T) {
	ctx, client := openVideoTaskStoreTestClient(t, "video-key-image-usage")
	billing := NewBillingStore(client, 5)
	record, key := videoKeyQuotaFixture(t, ctx, client, billing, "15.00000", "100.00000")
	if _, err := billing.ReserveTask(ctx, billingservice.ReserveStoreRequest{UserID: record.Task.UserID, APIKeyID: int64(key.ID), TaskID: uuid.NewString(), EstimatedPoints: "6.00000", Reason: "existing image reserve"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewVideoTaskStore(client, billing).Create(ctx, record); err == nil {
		t.Fatal("video must include image spending when checking key limit")
	}
	if count, err := client.VideoTask.Query().Count(ctx); err != nil || count != 0 {
		t.Fatalf("video count=%d err=%v", count, err)
	}
	usage, err := billing.APIKeyUsage(ctx, int64(key.ID), nil)
	if err != nil || usage != "6.00000" {
		t.Fatalf("usage=%s err=%v", usage, err)
	}
}

func TestVideoAPIKeyQuotaConcurrentImageAndVideoReserve(t *testing.T) {
	ctx, client := openVideoTaskStoreTestClient(t, "video-key-cross-media-concurrent")
	billing := NewBillingStore(client, 5)
	record, key := videoKeyQuotaFixture(t, ctx, client, billing, "10.00000", "100.00000")
	results := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _, err := NewVideoTaskStore(client, billing).Create(ctx, record)
		results <- err
	}()
	go func() {
		defer wg.Done()
		_, err := billing.ReserveTask(ctx, billingservice.ReserveStoreRequest{UserID: record.Task.UserID, APIKeyID: int64(key.ID), TaskID: uuid.NewString(), EstimatedPoints: "10.00000", Reason: "image reserve"})
		results <- err
	}()
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("cross-media reservations=%d want 1", successes)
	}
	usage, err := billing.APIKeyUsage(ctx, int64(key.ID), nil)
	if err != nil || usage != "10.00000" {
		t.Fatalf("usage=%s err=%v", usage, err)
	}
}

func TestVideoAPIKeyQuotaConcurrentReserve(t *testing.T) {
	ctx, client := openVideoTaskStoreTestClient(t, "video-key-concurrent")
	billing := NewBillingStore(client, 5)
	record, key := videoKeyQuotaFixture(t, ctx, client, billing, "10.00000", "100.00000")
	store := NewVideoTaskStore(client, billing)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		r := record
		r.Task.ID = uuid.New()
		r.Task.IdempotencyKey = fmt.Sprintf("concurrent-%d", i)
		r.Task.Items = []videotaskservice.Item{{ID: uuid.New(), Ordinal: 0}}
		wg.Add(1)
		go func() { defer wg.Done(); _, _, err := store.Create(ctx, r); results <- err }()
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful reservations=%d want 1", successes)
	}
	assertVideoKeyUsage(t, ctx, client, key.ID, "10.00000")
	balance, err := billing.GetBalance(ctx, record.Task.UserID)
	if err != nil || balance.FrozenPoints != "10.00000" {
		t.Fatalf("balance=%#v err=%v", balance, err)
	}
}

func videoKeyQuotaFixture(t *testing.T, ctx context.Context, client *repoent.Client, billing *BillingStore, limit, balance string) (videotaskservice.CreateRecord, *repoent.APIKey) {
	t.Helper()
	const userID int64 = 8830
	if _, err := billing.Adjust(ctx, billingservice.AdjustStoreRequest{UserID: userID, ChangePoints: balance, Reason: "seed"}); err != nil {
		t.Fatal(err)
	}
	project := mustCreateVideoTaskProject(t, ctx, client, userID)
	key, err := client.APIKey.Create().SetUserID(userID).SetAccessKey("video-test-key").SetSecretHash("hash").SetName("video test").SetTotalQuotaPoints(limit).SetDailyQuotaPoints(limit).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return videotaskservice.CreateRecord{Task: videotaskservice.Task{
		ID: uuid.New(), UserID: userID, APIKeyID: int64(key.ID), ProjectID: project.ID, SourceChannel: "api", TaskType: "text_to_video",
		PromptTemplate: "ocean", ExecutionPrompt: "ocean", RouteModelID: 9, RouteModelCode: "video-basic", DurationSeconds: 5, Resolution: "720p", AspectRatio: "16:9",
		RequestedOutputCount: 1, EstimatedPoints: "10.00000", ReservedPoints: "10.00000", IdempotencyKey: "video-key", RequestFingerprint: "video-key-fingerprint",
		Items: []videotaskservice.Item{{ID: uuid.New(), Ordinal: 0}},
	}, ReservePoints: "10.00000", ReserveReason: "video reserve"}, key
}

func assertVideoKeyUsage(t *testing.T, ctx context.Context, client *repoent.Client, id int, expected string) {
	t.Helper()
	key, err := client.APIKey.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if key.TotalQuotaUsedPoints != expected || key.DailyQuotaUsedPoints != expected {
		t.Fatalf("quota total=%s daily=%s want %s", key.TotalQuotaUsedPoints, key.DailyQuotaUsedPoints, expected)
	}
}

func openVideoTaskStoreTestClient(t *testing.T, name string) (context.Context, *repoent.Client) {
	t.Helper()
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:"+name+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	return ctx, client
}

func mustCreateVideoTaskProject(t *testing.T, ctx context.Context, client *repoent.Client, userID int64) *repoent.Project {
	t.Helper()
	project, err := client.Project.Create().SetUserID(userID).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return project
}

func mustCreateVideoTaskAsset(t *testing.T, ctx context.Context, client *repoent.Client, userID int64, projectID uuid.UUID, suffix string) *repoent.MediaAsset {
	t.Helper()
	asset, err := client.MediaAsset.Create().
		SetUserID(userID).SetProjectID(projectID).SetName(suffix + ".png").SetNameKey(suffix + ".png").
		SetMediaType("image").SetSourceType("upload").SetStatus("ready").SetObjectKey("users/test/" + suffix + ".png").
		SetMimeType("image/png").SetFileSizeBytes(10).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return asset
}

func assertVideoTaskTransactionEmpty(t *testing.T, ctx context.Context, client *repoent.Client, taskID uuid.UUID) {
	t.Helper()
	queries := []struct {
		name  string
		count func() (int, error)
	}{
		{name: "tasks", count: func() (int, error) { return client.VideoTask.Query().Count(ctx) }},
		{name: "items", count: func() (int, error) { return client.VideoTaskItem.Query().Count(ctx) }},
		{name: "inputs", count: func() (int, error) { return client.VideoTaskInput.Query().Count(ctx) }},
		{name: "allocations", count: func() (int, error) {
			return client.WalletReservationAllocation.Query().Where(walletreservationallocation.TaskIDEQ(taskID)).Count(ctx)
		}},
		{name: "task ledger", count: func() (int, error) { return client.PointLedger.Query().Where(pointledger.TaskIDEQ(taskID)).Count(ctx) }},
	}
	for _, query := range queries {
		count, err := query.count()
		if err != nil || count != 0 {
			t.Fatalf("%s after rollback: count=%d err=%v", query.name, count, err)
		}
	}
}

func TestVideoCancelLocalSettlementConcurrentAndClaimGuards(t *testing.T) {
	for _, mode := range []string{"queued", "cancel_requested", "expired", "live", "attempt", "submitted"} {
		t.Run(mode, func(t *testing.T) {
			ctx, client := openVideoTaskStoreTestClient(t, "video-cancel-"+mode)
			billing := NewBillingStore(client, 5)
			store := NewVideoTaskStore(client, billing)
			record, key := videoKeyQuotaFixture(t, ctx, client, billing, "10.00000", "100.00000")
			task, _, err := store.Create(ctx, record)
			if err != nil {
				t.Fatal(err)
			}
			id := task.Items[0].ID
			update := client.VideoTaskItem.UpdateOneID(id)
			if mode == "cancel_requested" || mode == "attempt" {
				update.SetStatus("cancel_requested")
			}
			if mode == "submitted" {
				update.SetStatus("provider_queued")
			}
			if mode == "expired" {
				update.SetLeaseOwner("old").SetLeaseExpiresAt(time.Now().UTC().Add(-time.Minute))
			}
			if mode == "live" {
				update.SetLeaseOwner("active").SetLeaseExpiresAt(time.Now().UTC().Add(time.Hour))
			}
			if err := update.Exec(ctx); err != nil {
				t.Fatal(err)
			}
			if mode == "attempt" || mode == "submitted" {
				a := client.VideoTaskAttempt.Create().SetItemID(id).SetAttemptNo(1).SetRouteCandidateID(1).SetAccountModelID(1).SetModelAccountID(1).SetProviderCode("seedance").SetModelCode("model").SetProviderIdempotencyKey("upstream").SetRequestSnapshot(map[string]any{})
				if mode == "submitted" {
					a.SetProviderJobID("job")
				}
				if _, err := a.Save(ctx); err != nil {
					t.Fatal(err)
				}
			}
			results := make(chan error, 4)
			var wg sync.WaitGroup
			for range 4 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, err := store.RequestCancel(ctx, task.UserID, task.ID, "cancel")
					results <- err
				}()
			}
			wg.Wait()
			close(results)
			for err := range results {
				if err != nil {
					t.Fatal(err)
				}
			}
			got, err := store.Get(ctx, task.UserID, task.ID)
			if err != nil {
				t.Fatal(err)
			}
			local := mode != "live" && mode != "attempt" && mode != "submitted"
			wantState, wantUsage, wantAvailable, wantFrozen := "cancel_requested", "10.00000", "90.00000", "10.00000"
			if local {
				wantState, wantUsage, wantAvailable, wantFrozen = "cancelled", "0.00000", "100.00000", "0.00000"
			}
			if string(got.Items[0].Status) != wantState || (got.SettlementStatus == "finalized") != local {
				t.Fatalf("cancel result=%#v", got)
			}
			assertVideoKeyUsage(t, ctx, client, key.ID, wantUsage)
			balance, err := billing.GetBalance(ctx, task.UserID)
			if err != nil || balance.AvailablePoints != wantAvailable || balance.FrozenPoints != wantFrozen {
				t.Fatalf("balance=%#v err=%v", balance, err)
			}
			if local {
				n, err := client.PointLedger.Query().Where(pointledger.TaskIDEQ(task.ID), pointledger.LedgerTypeEQ("refund")).Count(ctx)
				if err != nil || n != 1 {
					t.Fatalf("refund count=%d err=%v", n, err)
				}
				if _, ok, err := store.ClaimDue(ctx, worker.ClaimRequest{Owner: "late", Now: time.Now(), LeaseTTL: time.Minute}); err != nil || ok {
					t.Fatalf("terminal claim=%v err=%v", ok, err)
				}
			}
		})
	}
}

func TestVideoCancelRacesWorkerClaim(t *testing.T) {
	for i := range 12 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			ctx, client := openVideoTaskStoreTestClient(t, fmt.Sprintf("video-cancel-claim-%d", i))
			billing := NewBillingStore(client, 5)
			store := NewVideoTaskStore(client, billing)
			record, key := videoKeyQuotaFixture(t, ctx, client, billing, "10.00000", "100.00000")
			task, _, err := store.Create(ctx, record)
			if err != nil {
				t.Fatal(err)
			}
			start := make(chan struct{})
			results := make(chan error, 2)
			go func() { <-start; _, err := store.RequestCancel(ctx, task.UserID, task.ID, "cancel"); results <- err }()
			go func() {
				<-start
				_, _, err := store.ClaimDue(ctx, worker.ClaimRequest{Owner: "worker", Now: time.Now(), LeaseTTL: time.Minute})
				results <- err
			}()
			close(start)
			for range 2 {
				if err := <-results; err != nil {
					t.Fatal(err)
				}
			}
			got, err := store.Get(ctx, task.UserID, task.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Items[0].Status == "cancelled" {
				assertVideoKeyUsage(t, ctx, client, key.ID, "0.00000")
			} else if got.Items[0].Status == "cancel_requested" {
				assertVideoKeyUsage(t, ctx, client, key.ID, "10.00000")
			} else {
				t.Fatalf("unexpected state %s", got.Items[0].Status)
			}
			// Once the claim is released, another cancel repairs the pending settlement.
			if err := store.ReleaseLease(ctx, worker.LeaseRef{ItemID: task.Items[0].ID.String(), Owner: "worker"}); err != nil {
				t.Fatal(err)
			}
			got, err = store.RequestCancel(ctx, task.UserID, task.ID, "cancel")
			if err != nil || got.SettlementStatus != "finalized" {
				t.Fatalf("repair=%#v err=%v", got, err)
			}
			assertVideoKeyUsage(t, ctx, client, key.ID, "0.00000")
		})
	}
}

type failingVideoFinalizeBilling struct{ videoTaskBillingTx }

func (b failingVideoFinalizeBilling) FinalizeTaskTx(ctx context.Context, tx *repoent.Tx, req billingservice.FinalizeStoreRequest, media string, usage map[string]any) (billingservice.BalanceState, error) {
	if _, err := b.videoTaskBillingTx.FinalizeTaskTx(ctx, tx, req, media, usage); err != nil {
		return billingservice.BalanceState{}, err
	}
	return billingservice.BalanceState{}, errors.New("injected failure after wallet settlement")
}

func TestVideoCancelSettlementFailureRollsBackAndRetries(t *testing.T) {
	ctx, client := openVideoTaskStoreTestClient(t, "video-cancel-rollback")
	billing := NewBillingStore(client, 5)
	store := NewVideoTaskStore(client, billing)
	record, key := videoKeyQuotaFixture(t, ctx, client, billing, "10.00000", "100.00000")
	task, _, err := store.Create(ctx, record)
	if err != nil {
		t.Fatal(err)
	}
	failing := NewVideoTaskStore(client, failingVideoFinalizeBilling{billing})
	if _, err := failing.RequestCancel(ctx, task.UserID, task.ID, "cancel"); err == nil {
		t.Fatal("expected settlement failure")
	}
	got, err := store.Get(ctx, task.UserID, task.ID)
	if err != nil || got.Items[0].Status != "queued" || got.SettlementStatus == "finalized" {
		t.Fatalf("rolled back task=%#v err=%v", got, err)
	}
	balance, err := billing.GetBalance(ctx, task.UserID)
	if err != nil || balance.AvailablePoints != "90.00000" || balance.FrozenPoints != "10.00000" {
		t.Fatalf("rolled back balance=%#v err=%v", balance, err)
	}
	assertVideoKeyUsage(t, ctx, client, key.ID, "10.00000")
	if _, err := store.RequestCancel(ctx, task.UserID, task.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	assertVideoKeyUsage(t, ctx, client, key.ID, "0.00000")
}

func TestVideoCancelMixedSuccessChargesOnlySuccessfulOutput(t *testing.T) {
	ctx, client := openVideoTaskStoreTestClient(t, "video-cancel-mixed")
	billing := NewBillingStore(client, 5)
	store := NewVideoTaskStore(client, billing)
	record, key := videoKeyQuotaFixture(t, ctx, client, billing, "20.00000", "100.00000")
	record.Task.RequestedOutputCount = 2
	record.Task.EstimatedPoints = "20.00000"
	record.Task.ReservedPoints = "20.00000"
	record.ReservePoints = "20.00000"
	record.Task.PricingSnapshot = map[string]any{"schema_version": 2, "unit_points": "10.00000"}
	record.Task.Items = append(record.Task.Items, videotaskservice.Item{ID: uuid.New(), Ordinal: 1})
	task, _, err := store.Create(ctx, record)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.VideoTaskItem.UpdateOneID(record.Task.Items[0].ID).SetStatus("succeeded").SetStage("succeeded").SetActualPoints("10.00000").Exec(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := store.RequestCancel(ctx, task.UserID, task.ID, "cancel")
	if err != nil || got.Status != "partial" || got.ActualPoints != "10.00000" || got.SuccessOutputCount != 1 || got.SettlementStatus != "finalized" {
		t.Fatalf("mixed cancel=%#v err=%v", got, err)
	}
	assertVideoKeyUsage(t, ctx, client, key.ID, "10.00000")
	balance, err := billing.GetBalance(ctx, task.UserID)
	if err != nil || balance.AvailablePoints != "90.00000" || balance.FrozenPoints != "0.00000" {
		t.Fatalf("mixed balance=%#v err=%v", balance, err)
	}
	if _, err := store.RequestCancel(ctx, task.UserID, task.ID, "cancel-again"); err != nil {
		t.Fatal(err)
	}
	assertVideoKeyUsage(t, ctx, client, key.ID, "10.00000")
}
