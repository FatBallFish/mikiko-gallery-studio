package schema_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/clustertoken"
	_ "github.com/mattn/go-sqlite3"
)

func TestClusterCredentialHashIsQueryableButNotSerializableOrPrintable(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, "file:cluster-token-sensitive?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	hash := strings.Repeat("a", 64)
	created, err := client.ClusterToken.Create().
		SetTokenID("token-test").
		SetTokenHash(hash).
		SetInstallationID("installation-test").
		SetRole(clustertoken.RoleAPI).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetAuditActor("admin-test").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster token: %v", err)
	}
	queried, err := client.ClusterToken.Query().Where(clustertoken.TokenHashEQ(hash)).Only(ctx)
	if err != nil {
		t.Fatalf("query cluster token by hash: %v", err)
	}
	if queried.ID != created.ID {
		t.Fatalf("queried cluster token id = %d, want %d", queried.ID, created.ID)
	}
	encoded, err := json.Marshal(queried)
	if err != nil {
		t.Fatalf("marshal cluster token: %v", err)
	}
	if strings.Contains(string(encoded), hash) || strings.Contains(string(encoded), "token_hash") {
		t.Fatalf("cluster token JSON exposed hash: %s", encoded)
	}
	if printable := queried.String(); strings.Contains(printable, hash) {
		t.Fatalf("cluster token String exposed hash: %s", printable)
	}
}
