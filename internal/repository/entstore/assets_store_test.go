package entstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
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

func ptrInt64(value int64) *int64 {
	return &value
}
