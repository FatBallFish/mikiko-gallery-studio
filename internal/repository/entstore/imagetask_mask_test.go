package entstore

import (
	"encoding/json"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/google/uuid"
)

func TestImageTaskMaskSnapshotRoundTrip(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:mask-snapshot-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	store := NewImageTaskStore(client)
	task := domainimagetask.Task{ID: uuid.NewString(), UserID: 72, Status: domainimagetask.StatusQueued, TaskType: "image_edit", AbstractModel: "plus", Prompt: "edit", MaskAssetID: "mask-asset", RequestFingerprint: "fingerprint"}
	if err := store.Save(ctx, task); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{domainimagetask.StatusQueued, domainimagetask.StatusFailed} {
		task.Status = status
		if err := store.Save(ctx, task); err != nil {
			t.Fatal(err)
		}
		loaded, err := store.GetByID(ctx, 72, task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.MaskAssetID != "mask-asset" || loaded.RequestFingerprint != "fingerprint" {
			t.Fatalf("lost snapshot: %#v", loaded)
		}
		entity, err := client.ImageTask.Get(ctx, uuid.MustParse(task.ID))
		if err != nil {
			t.Fatal(err)
		}
		if !entity.MaskPresent || entity.RoutingSnapshot["mask_asset_id"] != "mask-asset" || entity.RoutingSnapshot["open_request_fingerprint"] != "fingerprint" {
			t.Fatalf("bad durable snapshot: %#v", entity)
		}
		payload, _ := json.Marshal(entity.ProviderTrace)
		if strings.Contains(string(payload), "mask_asset") || strings.Contains(string(payload), "fingerprint") {
			t.Fatalf("inputs leaked into provider trace: %s", payload)
		}
	}
}
