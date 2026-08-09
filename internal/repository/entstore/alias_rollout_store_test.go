package entstore

import (
	"fmt"
	"path/filepath"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
)

func TestAliasCreationRolloutPersistsAcrossNodesWithoutCaching(t *testing.T) {
	databaseURL := fmt.Sprintf("file:%s?_fk=1&_busy_timeout=5000&_journal_mode=WAL", filepath.Join(t.TempDir(), "alias-rollout-"+uuid.NewString()+".db"))
	clientA, err := repoent.Open(dialect.SQLite, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientA.Close() })
	clientB, err := repoent.Open(dialect.SQLite, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientB.Close() })
	if err := clientA.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}

	nodeA := NewAliasRolloutStore(clientA)
	nodeB := NewAliasRolloutStore(clientB)
	if enabled, err := nodeA.AliasCreationEnabled(t.Context()); err != nil || enabled {
		t.Fatalf("missing rollout row must fail closed: enabled=%v err=%v", enabled, err)
	}
	status, err := nodeB.UpdateAliasCreationRollout(t.Context(), true, 0, 91)
	if err != nil || !status.Enabled || status.Version != 1 || status.UpdatedBy != 91 {
		t.Fatalf("activate status=%#v err=%v", status, err)
	}
	if enabled, err := nodeA.AliasCreationEnabled(t.Context()); err != nil || !enabled {
		t.Fatalf("node A did not observe persisted activation immediately: enabled=%v err=%v", enabled, err)
	}
	status, err = nodeA.UpdateAliasCreationRollout(t.Context(), false, status.Version, 92)
	if err != nil || status.Enabled || status.Version != 2 || status.UpdatedBy != 92 {
		t.Fatalf("rollback status=%#v err=%v", status, err)
	}
	if enabled, err := nodeB.AliasCreationEnabled(t.Context()); err != nil || enabled {
		t.Fatalf("node B did not observe persisted rollback immediately: enabled=%v err=%v", enabled, err)
	}
}
