package app

import (
	"os"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/google/uuid"
)

func TestReferenceMediaStartupMigratesBeforeCleanupAndPreservesRestart(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:reference-startup-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	ref, err := client.ReferenceAsset.Create().SetUserID(504).SetObjectKey("reference-assets/startup.png").SetMimeType("image/png").SetSha256(strings.Repeat("a", 64)).SetStatus("ready").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := requireReferenceMediaBackfill(ctx, client); err != nil {
			t.Fatal(err)
		}
	}
	media, err := client.MediaAsset.Get(ctx, ref.ID)
	if err != nil {
		t.Fatal(err)
	}
	if media.UserID != 504 || media.ObjectKey != ref.ObjectKey {
		t.Fatalf("wrong media: %+v", media)
	}
	mapped, err := client.ReferenceAsset.Get(ctx, ref.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mapped.OwnsObject || mapped.MediaAssetID == nil || *mapped.MediaAssetID != ref.ID {
		t.Fatal("ownership transfer incomplete")
	}
	for _, file := range []string{"run.go", "worker.go"} {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		source := string(data)
		identities := strings.Index(source, "if err := requireLegacyStorageIdentityBackfill(")
		backfill := strings.Index(source, "if err := requireReferenceMediaBackfill(startupContext, client)")
		registry := strings.Index(source, "storageRegistry := storage.NewRegistry(")
		if identities < 0 || backfill <= identities || registry <= backfill {
			t.Fatalf("%s must migrate references after identities and before storage workers", file)
		}
	}
}

func TestReferenceMediaStartupFailsClosed(t *testing.T) {
	if err := requireReferenceMediaBackfill(t.Context(), nil); err == nil || !strings.Contains(err.Error(), "prepare unified reference media assets") {
		t.Fatalf("err=%v", err)
	}
}
