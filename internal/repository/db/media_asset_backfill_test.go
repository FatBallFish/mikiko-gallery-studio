package db

import (
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaasset"
)

func TestBackfillMediaAssetsUsesStableCheckpointAndPreservesObjectIdentity(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:media-asset-backfill-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	const userID int64 = 401
	project, err := client.Project.Create().SetUserID(userID).SetName("Default").SetNameKey("default").SetIsDefault(true).SetStatus("active").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	task, err := client.ImageTask.Create().SetUserID(userID).SetProjectID(project.ID).SetTaskType("text_to_image").SetPrompt("backfill").SetAbstractModel("basic").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ids := []uuid.UUID{uuid.New(), uuid.New()}
	keys := []string{"generated-images/first.png", "generated-images/second.png"}
	for index := range ids {
		if _, err := client.ImageResult.Create().SetID(ids[index]).SetTaskID(task.ID).SetUserID(userID).SetProjectID(project.ID).
			SetStorageDriver("local").SetObjectKey(keys[index]).SetMimeType("image/png").SetFileSizeBytes(int64(100 + index)).
			SetWidth(640).SetHeight(480).SetSha256(strings.Repeat(string(rune('a'+index)), 64)).Save(ctx); err != nil {
			t.Fatal(err)
		}
	}

	first, err := BackfillMediaAssets(ctx, client, MediaAssetBackfillOptions{BatchSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if first.Processed != 1 || first.Created != 1 || first.Done || first.Checkpoint.ID == uuid.Nil {
		t.Fatalf("first batch=%+v", first)
	}
	second, err := BackfillMediaAssets(ctx, client, MediaAssetBackfillOptions{BatchSize: 1, Checkpoint: first.Checkpoint})
	if err != nil {
		t.Fatal(err)
	}
	if second.Processed != 1 || second.Created != 1 {
		t.Fatalf("second batch=%+v", second)
	}
	final, err := BackfillMediaAssets(ctx, client, MediaAssetBackfillOptions{BatchSize: 1, Checkpoint: second.Checkpoint})
	if err != nil {
		t.Fatal(err)
	}
	if !final.Done || final.Processed != 0 {
		t.Fatalf("final batch=%+v", final)
	}

	for index, id := range ids {
		asset, err := client.MediaAsset.Query().Where(mediaasset.IDEQ(id)).Only(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if asset.LegacyImageResultID == nil || *asset.LegacyImageResultID != id || asset.ObjectKey != keys[index] || asset.ProjectID != project.ID {
			t.Fatalf("asset %s changed identity: %+v", id, asset)
		}
	}

	replayed, err := BackfillMediaAssets(ctx, client, MediaAssetBackfillOptions{BatchSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Processed != 2 || replayed.Created != 0 || replayed.Skipped != 2 {
		t.Fatalf("replayed batch=%+v", replayed)
	}
}

func TestBackfillMediaAssetsUsesUsersDefaultProjectForLegacyRows(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:media-asset-backfill-default-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	const userID int64 = 402
	project, err := client.Project.Create().SetUserID(userID).SetName("Default").SetNameKey("default").SetIsDefault(true).SetStatus("active").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	task, err := client.ImageTask.Create().SetUserID(userID).SetTaskType("text_to_image").SetPrompt("legacy").SetAbstractModel("basic").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	image, err := client.ImageResult.Create().SetTaskID(task.ID).SetUserID(userID).SetObjectKey("generated-images/legacy.png").SetMimeType("image/png").SetSha256(strings.Repeat("c", 64)).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BackfillMediaAssets(ctx, client, MediaAssetBackfillOptions{BatchSize: 10}); err != nil {
		t.Fatal(err)
	}
	asset, err := client.MediaAsset.Get(ctx, image.ID)
	if err != nil {
		t.Fatal(err)
	}
	if asset.ProjectID != project.ID {
		t.Fatalf("project=%s want %s", asset.ProjectID, project.ID)
	}
}
