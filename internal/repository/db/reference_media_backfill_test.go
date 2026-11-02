package db

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/referenceasset"
	"github.com/google/uuid"
)

func legacyReference(t *testing.T, ctx context.Context, client *repoent.Client, user int64, key string) *repoent.ReferenceAsset {
	t.Helper()
	row, err := client.ReferenceAsset.Create().SetUserID(user).SetStatus("ready").SetObjectKey(key).SetMimeType("image/png").SetSha256(strings.Repeat("a", 64)).SetFileSizeBytes(123).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func TestReferenceMediaBackfillPreservesIdentityAndUserChanges(t *testing.T) {
	ctx, client, project, _, _ := seedMediaAssetBackfill(t, "reference", 0)
	configID := uuid.New()
	ref := legacyReference(t, ctx, client, project.UserID, "reference-assets/a.png")
	ref, err := ref.Update().SetName("Original").SetStorageConfigID(configID).SetWidth(42).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := BackfillReferenceMediaAssets(ctx, client); err != nil {
			t.Fatal(err)
		}
	}
	media, err := client.MediaAsset.Get(ctx, ref.ID)
	if err != nil {
		t.Fatal(err)
	}
	if media.UserID != ref.UserID || media.ProjectID != project.ID || media.Name != "Original" || media.ObjectKey != ref.ObjectKey || media.StorageConfigID == nil || *media.StorageConfigID != configID || media.FileSizeBytes != 123 || media.Width == nil || *media.Width != 42 || !media.CreatedAt.Equal(ref.CreatedAt) {
		t.Fatalf("unexpected media: %+v", media)
	}
	mapped, err := client.ReferenceAsset.Get(ctx, ref.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mapped.OwnsObject || mapped.MediaAssetID == nil || *mapped.MediaAssetID != ref.ID {
		t.Fatalf("ownership not transferred: %+v", mapped)
	}
	moved, err := client.Project.Create().SetUserID(ref.UserID).SetName("Moved").SetNameKey("moved").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	deleted := time.Now().UTC().Truncate(time.Millisecond)
	if err := media.Update().SetName("Renamed").SetNameKey("renamed").SetProjectID(moved.ID).SetStatus("deleted").SetDeletedAt(deleted).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	late := legacyReference(t, ctx, client, ref.UserID, "reference-assets/late.png")
	if _, err := BackfillReferenceMediaAssets(ctx, client); err != nil {
		t.Fatal(err)
	}
	media, err = client.MediaAsset.Get(ctx, ref.ID)
	if err != nil {
		t.Fatal(err)
	}
	if media.Name != "Renamed" || media.ProjectID != moved.ID || media.Status != "deleted" || media.DeletedAt == nil {
		t.Fatalf("migration overwrote user changes: %+v", media)
	}
	if _, err := client.MediaAsset.Get(ctx, late.ID); err != nil {
		t.Fatal(err)
	}
	count, err := client.MediaAsset.Query().Count(ctx)
	if err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestReferenceMediaBackfillAliasesAndDeletedSources(t *testing.T) {
	ctx, client, project, _, images := seedMediaAssetBackfill(t, "reference-alias", 2)
	first := legacyReference(t, ctx, client, project.UserID, images[0].ObjectKey)
	if err := first.Update().SetSourceImageResultID(images[0].ID).SetOwnsObject(false).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	// Existing direct media alias and old source alias resolve to one entity.
	if _, err := BackfillMediaAssets(ctx, client, MediaAssetBackfillOptions{}); err != nil {
		t.Fatal(err)
	}
	direct := legacyReference(t, ctx, client, project.UserID, images[0].ObjectKey)
	if err := direct.Update().SetMediaAssetID(images[0].ID).SetOwnsObject(false).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	deleted := time.Now().UTC().Truncate(time.Millisecond)
	if err := client.ImageResult.UpdateOneID(images[1].ID).SetDeletedAt(deleted).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.MediaAsset.DeleteOneID(images[1].ID).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	second := legacyReference(t, ctx, client, project.UserID, images[1].ObjectKey)
	if err := second.Update().SetSourceImageResultID(images[1].ID).SetOwnsObject(false).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := BackfillReferenceMediaAssets(ctx, client); err != nil {
			t.Fatal(err)
		}
	}
	count, err := client.MediaAsset.Query().Count(ctx)
	if err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	fallback, err := client.ReferenceAsset.Get(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fallback.MediaAssetID == nil || *fallback.MediaAssetID != images[0].ID || fallback.SourceImageResultID == nil || fallback.OwnsObject {
		t.Fatalf("invalid fallback: %+v", fallback)
	}
	media, err := client.MediaAsset.Get(ctx, images[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if media.Status != "deleted" || media.DeletedAt == nil {
		t.Fatalf("deleted source revived: %+v", media)
	}
}

func TestReferenceMediaBackfillDeletedStatusNamesAndOwners(t *testing.T) {
	ctx, client, project, _, _ := seedMediaAssetBackfill(t, "reference-status", 0)
	first := legacyReference(t, ctx, client, project.UserID, "reference-assets/one.png")
	second := legacyReference(t, ctx, client, project.UserID, "reference-assets/two.png")
	if err := first.Update().SetName("Same").Exec(ctx); err != nil {
		t.Fatal(err)
	}
	deleted := time.Now().UTC()
	if err := second.Update().SetName("Same").SetStatus("deleted").SetDeletedAt(deleted).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	other := legacyReference(t, ctx, client, 900, "reference-assets/other.png")
	if _, err := BackfillReferenceMediaAssets(ctx, client); err != nil {
		t.Fatal(err)
	}
	media, err := client.MediaAsset.Get(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if media.Name != "Same" || media.Status != "deleted" || media.DeletedAt == nil {
		t.Fatalf("deleted reference revived: %+v", media)
	}
	media, err = client.MediaAsset.Get(ctx, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	ownerProject, err := client.Project.Get(ctx, media.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if ownerProject.UserID != other.UserID || !ownerProject.IsDefault {
		t.Fatalf("wrong project owner: %+v", ownerProject)
	}
}

func TestReferenceMediaBackfillRejectsConflictsAtomically(t *testing.T) {
	for _, kind := range []string{"owner", "missing"} {
		t.Run(kind, func(t *testing.T) {
			ctx, client, project, _, _ := seedMediaAssetBackfill(t, "reference-conflict-"+kind, 0)
			ref := legacyReference(t, ctx, client, project.UserID, "reference-assets/conflict.png")
			targetID := uuid.New()
			if kind != "missing" {
				user := project.UserID
				if kind == "owner" {
					user++
				}
				key := ref.ObjectKey
				if kind == "storage" {
					key = "reference-assets/different.png"
				}
				_, err := client.MediaAsset.Create().SetID(targetID).SetUserID(user).SetProjectID(project.ID).SetName("existing").SetNameKey("existing").SetMediaType("image").SetSourceType("local_upload").SetStatus("ready").SetObjectKey(key).SetMimeType("image/png").SetFileSizeBytes(123).Save(ctx)
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := ref.Update().SetMediaAssetID(targetID).Exec(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := BackfillReferenceMediaAssets(ctx, client); err == nil {
				t.Fatal("accepted invalid mapping")
			}
			ref, err := client.ReferenceAsset.Get(ctx, ref.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !ref.OwnsObject {
				t.Fatal("ownership changed on failure")
			}
		})
	}
}

func TestReferenceMediaBackfillConcurrentInstances(t *testing.T) {
	ctx, client, project, _, _ := seedMediaAssetBackfill(t, "reference-concurrent", 0)
	for range 12 {
		legacyReference(t, ctx, client, project.UserID, "reference-assets/"+uuid.NewString()+".png")
	}
	var wg sync.WaitGroup
	errors := make(chan error, 4)
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := BackfillReferenceMediaAssets(ctx, client); errors <- err }()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	count, err := client.MediaAsset.Query().Where(mediaasset.UserIDEQ(project.UserID)).Count(ctx)
	if err != nil || count != 12 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestReferenceMediaBackfillDuplicateStorageAndStaleMapping(t *testing.T) {
	ctx, client, project, _, _ := seedMediaAssetBackfill(t, "reference-duplicate", 0)
	first := legacyReference(t, ctx, client, project.UserID, "reference-assets/shared.png")
	second := legacyReference(t, ctx, client, project.UserID, first.ObjectKey)
	if _, err := BackfillReferenceMediaAssets(ctx, client); err != nil {
		t.Fatal(err)
	}
	media, err := client.MediaAsset.Query().Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{first.ID, second.ID} {
		ref, err := client.ReferenceAsset.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if ref.OwnsObject {
			t.Fatal("shared object ownership was not transferred")
		}
	}
	deleted := time.Now().UTC()
	if err := media.Update().SetObjectKey("media/original/moved.png").SetStatus("deleted").SetDeletedAt(deleted).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := BackfillReferenceMediaAssets(ctx, client); err != nil {
		t.Fatal(err)
	}
	media, err = client.MediaAsset.Get(ctx, media.ID)
	if err != nil {
		t.Fatal(err)
	}
	if media.Status != "deleted" || media.ObjectKey != "media/original/moved.png" {
		t.Fatal("canonical media overwritten")
	}
	aliases, err := client.ReferenceAsset.Query().Where(referenceasset.MediaAssetIDEQ(media.ID)).Count(ctx)
	if err != nil || aliases != 2 {
		t.Fatalf("aliases=%d err=%v", aliases, err)
	}
	count, err := client.MediaAsset.Query().Count(ctx)
	if err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestReferenceMediaBackfillPostgresConcurrentInstances(t *testing.T) {
	_, databaseURL := openLegacyMigrationPostgres(t)
	ctx := t.Context()
	client, err := repoent.Open(dialect.Postgres, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.ExecRaw(ctx, `CREATE UNIQUE INDEX reference_asset_active_media_asset ON reference_assets (user_id, media_asset_id) WHERE media_asset_id IS NOT NULL AND deleted_at IS NULL AND status <> 'deleted'`); err != nil {
		t.Fatal(err)
	}
	for range 15 {
		legacyReference(t, ctx, client, 784, "reference-assets/"+uuid.NewString()+".png")
	}
	var wg sync.WaitGroup
	errors := make(chan error, 4)
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := BackfillReferenceMediaAssets(ctx, client); errors <- err }()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	count, err := client.MediaAsset.Query().Count(ctx)
	if err != nil || count != 15 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestReferenceMediaBackfillRetiresSQLiteLegacyUniqueIndex(t *testing.T) {
	ctx, client, project, _, _ := seedMediaAssetBackfill(t, "reference-old-index", 0)
	if err := client.ExecRaw(ctx, `CREATE UNIQUE INDEX reference_asset_active_media_asset ON reference_assets (user_id, media_asset_id) WHERE media_asset_id IS NOT NULL AND deleted_at IS NULL AND status <> 'deleted'`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		legacyReference(t, ctx, client, project.UserID, "reference-assets/old-shared.png")
	}
	for range 2 {
		if _, err := BackfillReferenceMediaAssets(ctx, client); err != nil {
			t.Fatal(err)
		}
	}
	asset, err := client.MediaAsset.Query().Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	aliases, err := client.ReferenceAsset.Query().Where(referenceasset.MediaAssetIDEQ(asset.ID)).Count(ctx)
	if err != nil || aliases != 2 {
		t.Fatalf("aliases=%d err=%v", aliases, err)
	}
}

func TestReferenceMediaBackfillStorageIdentityIsNotObjectKeyAlone(t *testing.T) {
	ctx, client, project, _, _ := seedMediaAssetBackfill(t, "reference-storage", 0)
	first := legacyReference(t, ctx, client, project.UserID, "reference-assets/same-key.png")
	second := legacyReference(t, ctx, client, project.UserID, first.ObjectKey)
	for _, ref := range []*repoent.ReferenceAsset{first, second} {
		if err := ref.Update().SetStorageConfigID(uuid.New()).Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := BackfillReferenceMediaAssets(ctx, client); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []*repoent.ReferenceAsset{first, second} {
		mapped, err := client.ReferenceAsset.Get(ctx, ref.ID)
		if err != nil {
			t.Fatal(err)
		}
		if mapped.MediaAssetID == nil || *mapped.MediaAssetID != ref.ID {
			t.Fatal("different storage configs were merged")
		}
	}
	count, err := client.MediaAsset.Query().Count(ctx)
	if err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestReferenceMediaBackfillLiveCanonicalIndependentOfUUIDOrder(t *testing.T) {
	for _, liveFirst := range []bool{false, true} {
		t.Run(fmt.Sprint(liveFirst), func(t *testing.T) {
			ctx, client, project, _, _ := seedMediaAssetBackfill(t, "live-order", 0)
			low := uuid.MustParse("00000000-0000-0000-0000-000000000001")
			high := uuid.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff")
			liveID, deadID := high, low
			if liveFirst {
				liveID, deadID = low, high
			}
			old := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Millisecond)
			for _, id := range []uuid.UUID{deadID, liveID} {
				create := client.ReferenceAsset.Create().SetID(id).SetUserID(project.UserID).SetObjectKey("reference-assets/shared-live.png").SetMimeType("image/png").SetSha256(strings.Repeat("b", 64)).SetStatus("ready")
				if id == deadID {
					create.SetStatus("deleted").SetDeletedAt(old)
				}
				if _, err := create.Save(ctx); err != nil {
					t.Fatal(err)
				}
			}
			for range 2 {
				if _, err := BackfillReferenceMediaAssets(ctx, client); err != nil {
					t.Fatal(err)
				}
			}
			media, err := client.MediaAsset.Query().Only(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if media.ID != liveID || media.Status != "ready" || media.DeletedAt != nil {
				t.Fatalf("live reference hidden: %+v", media)
			}
			for _, id := range []uuid.UUID{deadID, liveID} {
				ref, err := client.ReferenceAsset.Get(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				if ref.MediaAssetID == nil || *ref.MediaAssetID != liveID || ref.OwnsObject {
					t.Fatalf("wrong mapping: %+v", ref)
				}
				if id == deadID && (ref.DeletedAt == nil || !ref.DeletedAt.Equal(old)) {
					t.Fatal("legacy tombstone changed")
				}
			}
		})
	}
}

func TestReferenceMediaBackfillLateAliasInheritsCanonicalTombstone(t *testing.T) {
	ctx, client, project, _, _ := seedMediaAssetBackfill(t, "late-tombstone", 0)
	ref := legacyReference(t, ctx, client, project.UserID, "reference-assets/tombstone.png")
	if _, err := BackfillReferenceMediaAssets(ctx, client); err != nil {
		t.Fatal(err)
	}
	deleted := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	if err := client.MediaAsset.UpdateOneID(ref.ID).SetStatus("deleted").SetDeletedAt(deleted).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	late := legacyReference(t, ctx, client, project.UserID, ref.ObjectKey)
	for range 2 {
		if _, err := BackfillReferenceMediaAssets(ctx, client); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []uuid.UUID{ref.ID, late.ID} {
		mapped, err := client.ReferenceAsset.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if mapped.Status != "deleted" || mapped.DeletedAt == nil || !mapped.DeletedAt.Equal(deleted) || mapped.OwnsObject || mapped.MediaAssetID == nil || *mapped.MediaAssetID != ref.ID {
			t.Fatalf("live alias prevents cleanup: %+v", mapped)
		}
	}
	media, err := client.MediaAsset.Query().Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if media.ID != ref.ID || media.Status != "deleted" || media.DeletedAt == nil || !media.DeletedAt.Equal(deleted) {
		t.Fatalf("canonical tombstone changed: %+v", media)
	}
}
