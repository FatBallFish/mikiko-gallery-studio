package entstore

import (
	videotaskservice "github.com/fatballfish/pic-gallery/internal/service/videotask"
	"github.com/google/uuid"
	"testing"
)

func TestHistoryHidingRetainsSourcesAndBulkScopesEntireDataset(t *testing.T) {
	ctx, client := openVideoTaskStoreTestClient(t, "history-hide")
	image := NewImageTaskStore(client)
	video := NewVideoTaskStore(client, nil)
	project := mustCreateVideoTaskProject(t, ctx, client, 21)
	other := mustCreateVideoTaskProject(t, ctx, client, 22)
	for i := 0; i < 125; i++ {
		if _, err := client.ImageTask.Create().SetUserID(21).SetProjectID(project.ID).SetStatus("failed").SetAbstractModel("test").SetTaskType("text_to_image").SetPrompt("source").Save(ctx); err != nil {
			t.Fatal(err)
		}
	}
	succeeded, err := client.ImageTask.Create().SetUserID(21).SetProjectID(project.ID).SetStatus("succeeded").SetAbstractModel("test").SetTaskType("text_to_image").SetPrompt("source").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"partial_failed", "rejected", "running"} {
		if _, err := client.ImageTask.Create().SetUserID(21).SetProjectID(project.ID).SetStatus(status).SetAbstractModel("test").SetTaskType("text_to_image").SetPrompt("source").Save(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := client.ImageTask.Create().SetUserID(22).SetProjectID(other.ID).SetStatus("failed").SetAbstractModel("test").SetTaskType("text_to_image").SetPrompt("source").Save(ctx); err != nil {
		t.Fatal(err)
	}
	result, err := client.ImageResult.Create().SetTaskID(succeeded.ID).SetUserID(21).SetProjectID(project.ID).SetStorageDriver("remote").SetObjectKey("https://example.test/image.png").SetMimeType("image/png").SetSha256("source-hash").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	withOutput, err := client.ImageTask.Create().SetUserID(21).SetProjectID(project.ID).SetStatus("failed").SetAbstractModel("test").SetTaskType("text_to_image").SetPrompt("source").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ImageResult.Create().SetTaskID(withOutput.ID).SetUserID(21).SetProjectID(project.ID).SetStorageDriver("remote").SetObjectKey("https://example.test/partial.png").SetMimeType("image/png").SetSha256("partial-hash").Save(ctx); err != nil {
		t.Fatal(err)
	}
	n, err := image.ClearFailedHistory(ctx, 21, project.ID.String())
	if err != nil || n != 125 {
		t.Fatalf("bulk count=%d err=%v", n, err)
	}
	if err := image.HideHistory(ctx, 21, succeeded.ID.String()); err != nil {
		t.Fatal(err)
	}
	task, err := image.GetByID(ctx, 21, succeeded.ID.String())
	if err != nil || task.Prompt != "source" || len(task.Results) != 1 || task.Results[0].ID != result.ID.String() {
		t.Fatalf("source lost: %+v %v", task, err)
	}
	list, err := image.ListByUser(ctx, 21)
	if err != nil || len(list) != 4 {
		t.Fatalf("list len=%d err=%v", len(list), err)
	}
	if err := image.HideHistory(ctx, 22, succeeded.ID.String()); err == nil {
		t.Fatal("cross-user hide allowed")
	}
	for _, status := range []string{"failed", "partial", "cancelled", "running"} {
		id := uuid.New()
		_, err := client.VideoTask.Create().SetID(id).SetUserID(21).SetProjectID(project.ID).SetStatus(status).SetTaskType("text_to_video").SetPromptBindingSnapshot(map[string]any{}).SetPricingSnapshot(map[string]any{}).SetRoutingSnapshot(map[string]any{}).SetPromptTemplate("source").SetExecutionPrompt("source").SetRouteModelID(1).SetRouteModelCode("test").SetDurationSeconds(5).SetResolution("720p").SetAspectRatio("16:9").SetIdempotencyKey(id.String()).SetRequestFingerprint(id.String()).Save(ctx)
		if err != nil {
			t.Fatal(err)
		}
	}
	n, err = video.ClearFailedHistory(ctx, 21, project.ID.String())
	if err != nil || n != 1 {
		t.Fatalf("video bulk count=%d err=%v", n, err)
	}
	page, err := video.List(ctx, videotaskservice.ListRequest{UserID: 21})
	if err != nil || len(page.Items) != 3 {
		t.Fatalf("video list=%+v err=%v", page, err)
	}
	n, err = video.ClearFailedHistory(ctx, 21, "")
	if err != nil || n != 0 {
		t.Fatalf("repeat=%d %v", n, err)
	}
}
