package entstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"

	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
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
	status, err := nodeB.UpdateAliasCreationRollout(t.Context(), aliasRolloutUpdateRequest(true, 0, 91))
	if err != nil || !status.Enabled || status.Version != 1 || status.UpdatedBy != 91 {
		t.Fatalf("activate status=%#v err=%v", status, err)
	}
	if enabled, err := nodeA.AliasCreationEnabled(t.Context()); err != nil || !enabled {
		t.Fatalf("node A did not observe persisted activation immediately: enabled=%v err=%v", enabled, err)
	}
	status, err = nodeA.UpdateAliasCreationRollout(t.Context(), aliasRolloutUpdateRequest(false, status.Version, 92))
	if err != nil || status.Enabled || status.Version != 2 || status.UpdatedBy != 92 {
		t.Fatalf("rollback status=%#v err=%v", status, err)
	}
	if enabled, err := nodeB.AliasCreationEnabled(t.Context()); err != nil || enabled {
		t.Fatalf("node B did not observe persisted rollback immediately: enabled=%v err=%v", enabled, err)
	}
}

func TestAliasCreationRolloutUpdateAndAuditAreAtomic(t *testing.T) {
	databaseURL := fmt.Sprintf("file:%s?_fk=1&_busy_timeout=5000&_journal_mode=WAL", filepath.Join(t.TempDir(), "alias-rollout-atomic-"+uuid.NewString()+".db"))
	client, err := repoent.Open(dialect.SQLite, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}

	injectedAuditErr := errors.New("injected audit insert failure")
	var failAudit atomic.Bool
	failAudit.Store(true)
	client.Use(func(next repoent.Mutator) repoent.Mutator {
		return repoent.MutateFunc(func(ctx context.Context, mutation repoent.Mutation) (repoent.Value, error) {
			if _, ok := mutation.(*repoent.AuditLogMutation); ok && failAudit.Load() {
				return nil, injectedAuditErr
			}
			return next.Mutate(ctx, mutation)
		})
	})

	store := NewAliasRolloutStore(client)
	request := aliasRolloutUpdateRequest(true, 0, 91)
	request.AllAPINodesCleanupAware = true
	request.RequestID = "req-rollout-atomic"
	request.IPAddr = "198.51.100.7:443"
	request.UserAgent = "rollout-test/1.0"
	if _, err := store.UpdateAliasCreationRollout(t.Context(), request); !errors.Is(err, injectedAuditErr) {
		t.Fatalf("audit failure error = %v, want %v", err, injectedAuditErr)
	}
	status, err := store.GetAliasCreationRollout(t.Context())
	if err != nil || status.Enabled || status.Version != 0 {
		t.Fatalf("audit failure committed rollout: status=%#v err=%v", status, err)
	}
	if count, err := client.AuditLog.Query().Count(t.Context()); err != nil || count != 0 {
		t.Fatalf("audit failure left partial event: count=%d err=%v", count, err)
	}

	failAudit.Store(false)
	status, err = store.UpdateAliasCreationRollout(t.Context(), request)
	if err != nil || !status.Enabled || status.Version != 1 || status.UpdatedBy != 91 {
		t.Fatalf("atomic rollout success: status=%#v err=%v", status, err)
	}
	logs, err := client.AuditLog.Query().All(t.Context())
	if err != nil || len(logs) != 1 {
		t.Fatalf("atomic rollout audit count=%d err=%v", len(logs), err)
	}
	log := logs[0]
	if log.ActorType != "admin" || log.ActorID != "91" || log.Action != "runtime_rollout.alias_creation.enable" || log.TargetType != "runtime_rollout" || log.TargetID != "no_copy_reference_aliases" || log.IPAddr != request.IPAddr || log.UserAgent != request.UserAgent {
		t.Fatalf("atomic rollout audit identity=%#v", log)
	}
	if log.Metadata["request_id"] != request.RequestID || log.Metadata["expected_version"] != float64(0) || log.Metadata["version"] != float64(1) {
		t.Fatalf("atomic rollout audit metadata=%#v", log.Metadata)
	}
	before, beforeOK := log.Metadata["before"].(map[string]any)
	after, afterOK := log.Metadata["after"].(map[string]any)
	if !beforeOK || before["enabled"] != false || before["version"] != float64(0) || !afterOK || after["enabled"] != true || after["version"] != float64(1) {
		t.Fatalf("atomic rollout before/after metadata=%#v", log.Metadata)
	}

	if _, err := store.UpdateAliasCreationRollout(t.Context(), aliasRolloutUpdateRequest(false, 0, 92)); !errors.Is(err, domainassets.ErrAliasRolloutChanged) {
		t.Fatalf("stale expected_version error=%v", err)
	}
	status, err = store.GetAliasCreationRollout(t.Context())
	if err != nil || !status.Enabled || status.Version != 1 {
		t.Fatalf("stale expected_version changed rollout: status=%#v err=%v", status, err)
	}
	if count, err := client.AuditLog.Query().Count(t.Context()); err != nil || count != 1 {
		t.Fatalf("stale expected_version wrote audit: count=%d err=%v", count, err)
	}
}

func TestAliasCreationRolloutConcurrentExpectedVersionCommitsOnce(t *testing.T) {
	databaseURL := fmt.Sprintf("file:%s?_fk=1&_busy_timeout=5000&_journal_mode=WAL", filepath.Join(t.TempDir(), "alias-rollout-concurrent-"+uuid.NewString()+".db"))
	db, err := sql.Open("sqlite3", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	client := repoent.NewClient(repoent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	store := NewAliasRolloutStore(client)
	if _, err := store.UpdateAliasCreationRollout(t.Context(), aliasRolloutUpdateRequest(true, 0, 90)); err != nil {
		t.Fatal(err)
	}

	type updateResult struct {
		request domainassets.UpdateAliasCreationRolloutRequest
		status  domainassets.AliasCreationRollout
		err     error
	}
	start := make(chan struct{})
	results := make(chan updateResult, 2)
	requests := []domainassets.UpdateAliasCreationRolloutRequest{
		aliasRolloutUpdateRequest(false, 1, 91),
		aliasRolloutUpdateRequest(true, 1, 92),
	}
	var workers sync.WaitGroup
	for _, request := range requests {
		request := request
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			status, err := store.UpdateAliasCreationRollout(t.Context(), request)
			results <- updateResult{request: request, status: status, err: err}
		}()
	}
	close(start)
	workers.Wait()
	close(results)

	var winner updateResult
	successes, conflicts := 0, 0
	for result := range results {
		switch {
		case result.err == nil:
			successes++
			winner = result
		case errors.Is(result.err, domainassets.ErrAliasRolloutChanged):
			conflicts++
		default:
			t.Fatalf("concurrent update returned unexpected error: %v", result.err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent update outcomes: successes=%d conflicts=%d", successes, conflicts)
	}
	status, err := store.GetAliasCreationRollout(t.Context())
	if err != nil || status.Version != 2 || status.Enabled != winner.request.Enabled || status.UpdatedBy != winner.request.UpdatedBy {
		t.Fatalf("concurrent rollout state=%#v winner=%#v err=%v", status, winner, err)
	}
	if count, err := client.AuditLog.Query().Count(t.Context()); err != nil || count != 2 {
		t.Fatalf("concurrent rollout audit count=%d err=%v", count, err)
	}
}

func aliasRolloutUpdateRequest(enabled bool, expectedVersion, updatedBy int64) domainassets.UpdateAliasCreationRolloutRequest {
	return domainassets.UpdateAliasCreationRolloutRequest{
		Enabled: enabled, ExpectedVersion: expectedVersion, UpdatedBy: updatedBy,
		ActorType: "admin", ActorID: fmt.Sprintf("%d", updatedBy),
	}
}
