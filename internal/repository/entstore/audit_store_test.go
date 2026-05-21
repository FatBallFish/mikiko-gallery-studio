package entstore_test

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	domainaudit "github.com/fatballfish/pic-gallery/internal/domain/audit"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	_ "github.com/mattn/go-sqlite3"
)

func TestAuditStoreCreatesAuditLog(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:auditstore?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := entstore.NewAuditStore(client)
	created, err := store.Create(ctx, domainaudit.Log{
		ActorType:  "admin",
		ActorID:    "7",
		Action:     "admin.create",
		TargetType: "admin_user",
		TargetID:   "8",
		Result:     "success",
		Metadata:   map[string]any{"role": "ops_admin"},
		IPAddr:     "127.0.0.1",
		UserAgent:  "store-test",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 || created.Metadata["role"] != "ops_admin" {
		t.Fatalf("unexpected created log %#v", created)
	}
}
