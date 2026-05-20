package entstore

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	_ "github.com/mattn/go-sqlite3"

	domainapikey "github.com/fatballfish/pic-gallery/internal/domain/apikey"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
)

func TestAPIKeyStoreRoundTripAndLastUsedAt(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, "file:apikey-store?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewAPIKeyStore(client)
	key, err := store.Create(context.Background(), domainapikey.APIKey{
		UserID:     101,
		AccessKey:  "ak-store",
		SecretHash: domainapikey.HashSecret("sk-store"),
		Name:       "store-test",
		Status:     "active",
		GroupCode:  "plus",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if key.ID == 0 || key.SecretHash == "sk-store" {
		t.Fatalf("expected persisted key with hashed secret, got %#v", key)
	}

	byAccess, err := store.GetByAccessKey(context.Background(), "ak-store")
	if err != nil {
		t.Fatalf("GetByAccessKey: %v", err)
	}
	bySecret, err := store.GetBySecretHash(context.Background(), domainapikey.HashSecret("sk-store"))
	if err != nil {
		t.Fatalf("GetBySecretHash: %v", err)
	}
	if byAccess.ID != key.ID || bySecret.ID != key.ID {
		t.Fatalf("expected lookups to return created key, access=%#v secret=%#v", byAccess, bySecret)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if err := store.UpdateLastUsedAt(context.Background(), key.ID, now); err != nil {
		t.Fatalf("UpdateLastUsedAt: %v", err)
	}
	reloaded, err := store.GetByAccessKey(context.Background(), "ak-store")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.LastUsedAt == nil || !reloaded.LastUsedAt.Equal(now) {
		t.Fatalf("expected last_used_at to round-trip, got %#v", reloaded.LastUsedAt)
	}
}
