package entstore

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/pointledger"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/walletreservationallocation"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
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
