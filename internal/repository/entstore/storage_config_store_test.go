package entstore_test

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	domainstorageconfig "github.com/fatballfish/pic-gallery/internal/domain/storageconfig"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

func TestStorageConfigStoreTreatsMalformedIDAsNotFound(t *testing.T) {
	store := entstore.NewStorageConfigStore(nil)
	_, ok, err := store.GetByID(context.Background(), "{storage_config_id}")
	if err != nil || ok {
		t.Fatalf("malformed storage config id must be a not-found result: ok=%v err=%v", ok, err)
	}
}

func TestStorageConfigStorePersistsAndSwitchesDefault(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:storage-config-store?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := entstore.NewStorageConfigStore(client)
	first, err := store.Save(ctx, domainstorageconfig.ConfigRecord{
		Code: "first", Name: "First", Driver: "local", Provider: "local", Status: "enabled",
		ReadEnabled: true, WriteEnabled: true, IsDefault: true, LocalRoot: "/first", Version: 1,
	})
	if err != nil {
		t.Fatalf("save first: %v", err)
	}
	second, err := store.Save(ctx, domainstorageconfig.ConfigRecord{
		Code: "second", Name: "Second", Driver: "s3", Provider: "r2", Status: "enabled",
		ReadEnabled: true, WriteEnabled: true, Endpoint: "https://r2.example.com", Region: "auto", Bucket: "images",
		SecretEncrypted: map[string]any{"ciphertext": "v1:encrypted"}, SecretFingerprint: "sha256:test", Version: 1,
	})
	if err != nil {
		t.Fatalf("save second: %v", err)
	}
	staleSecond := second
	second, err = store.SetDefault(ctx, second.ID, second.Version, 7)
	if err != nil {
		t.Fatalf("set second default: %v", err)
	}

	resolved, ok, err := store.GetDefaultWritable(ctx)
	if err != nil || !ok {
		t.Fatalf("get default: ok=%v err=%v", ok, err)
	}
	if resolved.ID != second.ID || resolved.SecretFingerprint != "sha256:test" || resolved.SecretEncrypted["ciphertext"] != "v1:encrypted" {
		t.Fatalf("unexpected default record %#v", resolved)
	}
	if _, err := store.SetDefault(ctx, first.ID, first.Version+99, 8); err == nil {
		t.Fatal("expected stale default switch to fail")
	}
	stillDefault, ok, err := store.GetDefaultWritable(ctx)
	if err != nil || !ok || stillDefault.ID != second.ID {
		t.Fatalf("failed switch must preserve current default: record=%#v ok=%v err=%v", stillDefault, ok, err)
	}
	staleSecond.Name = "stale update"
	staleSecond.Version++
	if _, err := store.Save(ctx, staleSecond); err == nil {
		t.Fatal("stale update must not overwrite a concurrent default switch")
	}
	stillDefault, ok, err = store.GetDefaultWritable(ctx)
	if err != nil || !ok || stillDefault.ID != second.ID {
		t.Fatalf("stale update must preserve current default: record=%#v ok=%v err=%v", stillDefault, ok, err)
	}
	if _, err := client.ObjectStorageConfig.UpdateOneID(uuid.MustParse(first.ID)).SetIsDefault(true).Save(ctx); err == nil {
		t.Fatal("database unique index must reject a second default")
	}
	historical, ok, err := store.GetByID(ctx, first.ID)
	if err != nil || !ok || historical.LocalRoot != "/first" {
		t.Fatalf("historical lookup failed: record=%#v ok=%v err=%v", historical, ok, err)
	}
}
