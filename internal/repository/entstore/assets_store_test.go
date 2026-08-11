package entstore

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	"github.com/fatballfish/pic-gallery/internal/provider"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

func TestAssetsStorePersistsAndQueriesByHash(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:assetstore?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewAssetsStore(client)
	asset := domainassets.ReferenceAsset{
		ID:              "11111111-1111-1111-1111-111111111111",
		APIKeyID:        ptrInt64(33),
		UploadSource:    "openapi",
		Status:          "ready",
		StorageConfigID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		StorageDriver:   "local",
		MimeType:        "image/png",
		FileSizeBytes:   68,
		Width:           1,
		Height:          1,
		SHA256:          "hash-asset",
		ObjectKey:       "reference-assets/11111111-1111-1111-1111-111111111111.png",
		CreatedAt:       time.Now(),
	}
	if err := store.Save(ctx, 7, asset); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.GetByUserAndHash(ctx, 7, "hash-asset")
	if err != nil {
		t.Fatalf("GetByUserAndHash: %v", err)
	}
	if loaded.ID != asset.ID {
		t.Fatalf("expected %s, got %s", asset.ID, loaded.ID)
	}
	if loaded.APIKeyID == nil || *loaded.APIKeyID != 33 || loaded.UploadSource != "openapi" {
		t.Fatalf("expected api key upload metadata to round-trip, got %#v", loaded)
	}
	if loaded.StorageConfigID != asset.StorageConfigID {
		t.Fatalf("expected storage config id to round-trip, got %#v", loaded)
	}

	loadedByID, err := store.GetByUserAndID(ctx, 7, asset.ID)
	if err != nil {
		t.Fatalf("GetByUserAndID: %v", err)
	}
	if loadedByID.ObjectKey != asset.ObjectKey {
		t.Fatalf("unexpected object key: %s", loadedByID.ObjectKey)
	}

	if err := store.DeleteByUserAndID(ctx, 7, asset.ID); err != nil {
		t.Fatalf("DeleteByUserAndID: %v", err)
	}
	if _, err := store.GetByUserAndHash(ctx, 7, "hash-asset"); !errors.Is(err, repoerr.ErrNotFound) {
		t.Fatalf("expected deleted asset to be excluded from hash lookup, got %v", err)
	}
	if err := store.DeleteByUserAndID(ctx, 7, "22222222-2222-2222-2222-222222222222"); !errors.Is(err, repoerr.ErrNotFound) {
		t.Fatalf("expected missing asset delete to return not found, got %v", err)
	}

	otherAsset := asset
	otherAsset.ID = "33333333-3333-3333-3333-333333333333"
	otherAsset.SHA256 = "hash-other"
	otherAsset.ObjectKey = "reference-assets/33333333-3333-3333-3333-333333333333.png"
	if err := store.Save(ctx, 7, otherAsset); err != nil {
		t.Fatalf("Save other asset: %v", err)
	}
	if err := store.DeleteByUserAndID(ctx, 8, otherAsset.ID); !errors.Is(err, repoerr.ErrNotFound) {
		t.Fatalf("expected wrong-user asset delete to return not found, got %v", err)
	}
}

func TestAssetsStoreAssignsUniqueNamesRenamesAndBatchLoads(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:assetstore-names?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	store := NewAssetsStore(client)
	first := domainassets.ReferenceAsset{ID: uuid.NewString(), Status: "ready", StorageDriver: "local", ObjectKey: "a.png", MimeType: "image/png", SHA256: "hash-a", OwnsObject: true, CreatedAt: time.Now()}
	second := domainassets.ReferenceAsset{ID: uuid.NewString(), Status: "ready", StorageDriver: "local", ObjectKey: "b.png", MimeType: "image/png", SHA256: "hash-b", OwnsObject: true, CreatedAt: time.Now()}
	first, err = store.SaveWithGeneratedName(ctx, 90, first, domainassets.UploadMetadata{UploadSource: "web"}, "主体")
	if err != nil {
		t.Fatal(err)
	}
	second, err = store.SaveWithGeneratedName(ctx, 90, second, domainassets.UploadMetadata{UploadSource: "web"}, "主体")
	if err != nil {
		t.Fatal(err)
	}
	if first.Name != "主体" || second.Name != "主体 2" {
		t.Fatalf("names = %q, %q", first.Name, second.Name)
	}
	if _, err := store.RenameByUserAndID(ctx, 90, second.ID, "主体", "主体"); !errors.Is(err, repoerr.ErrConflict) {
		t.Fatalf("rename conflict = %v", err)
	}
	renamed, err := store.RenameByUserAndID(ctx, 90, second.ID, "风格", "风格")
	if err != nil || renamed.Name != "风格" {
		t.Fatalf("renamed = %#v err=%v", renamed, err)
	}
	items, err := store.GetManyByUserAndIDs(ctx, 90, []string{second.ID, first.ID})
	if err != nil || len(items) != 2 || items[0].ID != second.ID || items[1].ID != first.ID {
		t.Fatalf("batch = %#v err=%v", items, err)
	}
	if _, err := store.GetManyByUserAndIDs(ctx, 91, []string{first.ID}); !errors.Is(err, repoerr.ErrNotFound) {
		t.Fatalf("cross-user batch = %v", err)
	}
}

func TestAssetsStoreLazilyBackfillsHistoricalUnnamedAssets(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:assetstore-name-backfill?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := client.ReferenceAsset.Create().SetUserID(93).SetStatus("ready").SetStorageDriver("local").SetObjectKey("legacy-a.png").SetMimeType("image/png").SetSha256("legacy-a").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.ReferenceAsset.Create().SetUserID(93).SetStatus("ready").SetStorageDriver("local").SetObjectKey("legacy-b.png").SetMimeType("image/png").SetSha256("legacy-b").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	store := NewAssetsStore(client)
	items, err := store.GetManyByUserAndIDs(ctx, 93, []string{first.ID.String(), second.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if items[0].Name != "图片1" || items[1].Name != "图片2" {
		t.Fatalf("backfilled names = %q, %q", items[0].Name, items[1].Name)
	}
	reloaded, err := store.GetByUserAndID(ctx, 93, first.ID.String())
	if err != nil || reloaded.Name != "图片1" {
		t.Fatalf("reloaded = %#v err=%v", reloaded, err)
	}
}

func TestAssetsStoreDeleteSoftDeletesAndEnqueuesCanonicalObjectOnce(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:assetstore-cleanup?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	store := NewAssetsStore(client)
	asset := domainassets.ReferenceAsset{ID: "11111111-1111-4111-8111-111111111111", Status: "ready", StorageDriver: "s3", MimeType: "image/png", SHA256: "source-hash", ObjectKey: "generated/shared.png", SourceImageResultID: "22222222-2222-4222-8222-222222222222", OwnsObject: false, CreatedAt: time.Now()}
	if err := store.Save(ctx, 9, asset); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteByUserAndID(ctx, 9, asset.ID); err != nil {
		t.Fatal(err)
	}
	if count, err := client.ObjectDeletionJob.Query().Count(ctx); err != nil || count != 1 {
		t.Fatalf("cleanup jobs=%d err=%v", count, err)
	}
	row, err := client.ReferenceAsset.Query().Only(ctx)
	if err != nil || row.DeletedAt == nil || row.Status != "deleted" {
		t.Fatalf("reference=%#v err=%v", row, err)
	}
	if err := store.DeleteByUserAndID(ctx, 9, asset.ID); !errors.Is(err, repoerr.ErrNotFound) {
		t.Fatalf("repeat delete=%v", err)
	}
	if count, _ := client.ObjectDeletionJob.Query().Count(ctx); count != 1 {
		t.Fatalf("repeat delete created %d jobs", count)
	}
}

func TestAssetsStoreImportsLockedSourceAliasWithGenerationSnapshot(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:assetstore-alias-source?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	projectID := uuid.New()
	if _, err := client.Project.Create().SetID(projectID).SetUserID(17).SetName("P").SetNameKey("p").SetStatus("active").Save(ctx); err != nil {
		t.Fatal(err)
	}
	task, err := client.ImageTask.Create().SetUserID(17).SetProjectID(projectID).SetTaskType("image_edit").SetPrompt("source").SetAbstractModel("plus").SetRouteModelCode("route-plus").SetSizeMode("pixel").SetRequestedSize("1536x1024").SetBaseResolution("2K").SetAspectRatio("3:2").SetQuality("high").SetBackground("transparent").SetOutputFormat("webp").SetOutputCompression(72).SetModeration("low").SetRequestedOutputImageCount(6).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.ImageResult.Create().SetTaskID(task.ID).SetUserID(17).SetProjectID(projectID).SetStorageDriver("s3").SetObjectKey("generated/source.webp").SetMimeType("image/webp").SetFileSizeBytes(99).SetWidth(1536).SetHeight(1024).SetSha256(strings.Repeat("a", 64)).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	store := NewAssetsStore(client)
	asset, err := store.ImportGalleryAlias(ctx, 17, provider.ImageResult{ID: result.ID.String(), ProjectID: projectID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if asset.SourceImageResultID != result.ID.String() || asset.OwnsObject || asset.ObjectKey != result.ObjectKey {
		t.Fatalf("asset=%#v", asset)
	}
	if asset.GenerationSnapshot == nil || asset.GenerationSnapshot.RouteModelCode != "route-plus" || asset.GenerationSnapshot.RequestedSize != "1536x1024" || asset.GenerationSnapshot.ImageCount != 6 {
		t.Fatalf("snapshot=%#v", asset.GenerationSnapshot)
	}
	second, err := store.ImportGalleryAlias(ctx, 17, provider.ImageResult{ID: result.ID.String(), ProjectID: projectID.String()})
	if err != nil || second.ID != asset.ID {
		t.Fatalf("idempotent=%#v err=%v", second, err)
	}
	if err := client.ImageResult.UpdateOneID(result.ID).SetDeletedAt(time.Now()).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.GetByUserAndID(ctx, 17, asset.ID)
	if err != nil || loaded.ObjectKey != result.ObjectKey {
		t.Fatalf("alias after source deletion=%#v err=%v", loaded, err)
	}
	if _, err := store.ImportGalleryAlias(ctx, 17, provider.ImageResult{ID: result.ID.String(), ProjectID: projectID.String()}); !errors.Is(err, repoerr.ErrNotFound) {
		t.Fatalf("late import=%v", err)
	}
}

func TestAssetsStoreImportsAliasWhenSourceTaskIsSoftDeleted(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:assetstore-alias-soft-deleted-task?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	projectID := uuid.New()
	if _, err := client.Project.Create().SetID(projectID).SetUserID(18).SetName("P").SetNameKey("p").SetStatus("active").Save(ctx); err != nil {
		t.Fatal(err)
	}
	task, err := client.ImageTask.Create().SetUserID(18).SetProjectID(projectID).SetTaskType("image_generation").SetPrompt("source").SetAbstractModel("plus").SetRouteModelCode("route-plus").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.ImageResult.Create().SetTaskID(task.ID).SetUserID(18).SetProjectID(projectID).SetStorageDriver("s3").SetObjectKey("generated/soft-deleted-task.png").SetMimeType("image/png").SetFileSizeBytes(99).SetWidth(1024).SetHeight(1024).SetSha256(strings.Repeat("b", 64)).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.ImageTask.UpdateOneID(task.ID).SetDeletedAt(time.Now().UTC()).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	asset, err := NewAssetsStore(client).ImportGalleryAlias(ctx, 18, provider.ImageResult{ID: result.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if asset.SourceImageResultID != result.ID.String() || asset.OwnsObject || asset.ObjectKey != result.ObjectKey {
		t.Fatalf("asset=%#v", asset)
	}
}

func ptrInt64(value int64) *int64 {
	return &value
}
