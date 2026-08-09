package project

import (
	"context"
	"errors"
	"sync"
	"testing"

	domainproject "github.com/fatballfish/pic-gallery/internal/domain/project"
)

func TestEnsureDefaultIsIdempotentAndUniquePerUser(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store)

	const callers = 16
	ids := make(chan string, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			project, err := svc.EnsureDefault(context.Background(), 42)
			if err != nil {
				t.Errorf("EnsureDefault: %v", err)
				return
			}
			ids <- project.ID
		}()
	}
	wg.Wait()
	close(ids)

	var expected string
	for id := range ids {
		if expected == "" {
			expected = id
		}
		if id != expected {
			t.Fatalf("concurrent ensure returned different defaults: %q != %q", id, expected)
		}
	}
	projects, err := svc.List(context.Background(), 42)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(projects) != 1 || !projects[0].IsDefault || projects[0].Name != domainproject.DefaultName {
		t.Fatalf("expected one immutable default project, got %#v", projects)
	}
}

func TestProjectOwnershipAndLifecycleInvariants(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewMemoryStore())
	defaultOne, err := svc.EnsureDefault(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EnsureDefault(ctx, 2); err != nil {
		t.Fatal(err)
	}
	created, err := svc.Create(ctx, 1, domainproject.CreateRequest{Name: "Campaign", IdempotencyKey: "create-campaign"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	replayed, err := svc.Create(ctx, 1, domainproject.CreateRequest{Name: "Campaign", IdempotencyKey: "create-campaign"})
	if err != nil || replayed.ID != created.ID {
		t.Fatalf("idempotent create replay = %#v, %v", replayed, err)
	}
	if _, err := svc.Create(ctx, 1, domainproject.CreateRequest{Name: " campaign "}); !errors.Is(err, ErrNameConflict) {
		t.Fatalf("case-folded active name duplicate err = %v, want ErrNameConflict", err)
	}
	if _, err := svc.Rename(ctx, 2, created.ID, domainproject.RenameRequest{Name: "Foreign", ExpectedVersion: created.Version}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign rename err = %v, want ErrNotFound", err)
	}
	if _, err := svc.Rename(ctx, 1, defaultOne.ID, domainproject.RenameRequest{Name: "Other", ExpectedVersion: defaultOne.Version}); !errors.Is(err, ErrDefaultImmutable) {
		t.Fatalf("default rename err = %v, want ErrDefaultImmutable", err)
	}
	renamed, err := svc.Rename(ctx, 1, created.ID, domainproject.RenameRequest{Name: "Launch", ExpectedVersion: created.Version})
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if _, err := svc.Rename(ctx, 1, created.ID, domainproject.RenameRequest{Name: "Stale", ExpectedVersion: created.Version}); !errors.Is(err, ErrProjectChanged) {
		t.Fatalf("stale rename err = %v, want ErrProjectChanged", err)
	}
	if renamed.Version != created.Version+1 {
		t.Fatalf("rename version = %d, want %d", renamed.Version, created.Version+1)
	}
	if _, err := svc.ResolveOwned(ctx, 2, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign resolve err = %v, want ErrNotFound", err)
	}
}

func TestDeleteEmptyAndAtomicallyTransferPopulatedProject(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	svc := NewService(store)
	defaultProject, _ := svc.EnsureDefault(ctx, 7)
	empty, _ := svc.Create(ctx, 7, domainproject.CreateRequest{Name: "Empty"})
	if _, err := svc.Delete(ctx, 7, empty.ID, domainproject.DeleteRequest{ExpectedVersion: empty.Version}); err != nil {
		t.Fatalf("delete empty: %v", err)
	}
	if _, err := svc.ResolveOwned(ctx, 7, empty.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted project remains resolvable: %v", err)
	}

	source, _ := svc.Create(ctx, 7, domainproject.CreateRequest{Name: "Source"})
	store.SeedOwnedRecords(7, source.ID, 2, 3)
	if _, err := svc.Delete(ctx, 7, source.ID, domainproject.DeleteRequest{ExpectedVersion: source.Version}); err == nil {
		t.Fatal("populated project deleted without transfer target")
	} else {
		var nonEmpty *NonEmptyError
		if !errors.As(err, &nonEmpty) || nonEmpty.Counts.Tasks != 2 || nonEmpty.Counts.Assets != 3 {
			t.Fatalf("non-empty error = %#v, %v", nonEmpty, err)
		}
	}
	deleted, err := svc.Delete(ctx, 7, source.ID, domainproject.DeleteRequest{
		TargetProjectID: defaultProject.ID,
		ExpectedVersion: source.Version,
	})
	if err != nil {
		t.Fatalf("transfer delete: %v", err)
	}
	if deleted.Transferred.Tasks != 2 || deleted.Transferred.Assets != 3 {
		t.Fatalf("transfer counts = %#v", deleted.Transferred)
	}
	if got := store.CountOwnedRecords(7, defaultProject.ID); got.Tasks != 2 || got.Assets != 3 {
		t.Fatalf("target records = %#v", got)
	}
	if got := store.CountOwnedRecords(7, source.ID); got.Tasks != 0 || got.Assets != 0 {
		t.Fatalf("source retained records after atomic transfer: %#v", got)
	}
	if _, err := svc.Delete(ctx, 7, defaultProject.ID, domainproject.DeleteRequest{ExpectedVersion: defaultProject.Version}); !errors.Is(err, ErrDefaultImmutable) {
		t.Fatalf("default delete err = %v, want ErrDefaultImmutable", err)
	}
}

func TestDeleteReplaysPersistedResultByUserScopedIdempotencyKey(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	svc := NewService(store)
	target, _ := svc.EnsureDefault(ctx, 71)
	source, _ := svc.Create(ctx, 71, domainproject.CreateRequest{Name: "Replay source"})
	store.SeedOwnedRecords(71, source.ID, 4, 5)
	req := domainproject.DeleteRequest{
		TargetProjectID: target.ID, ExpectedVersion: source.Version,
		IdempotencyKey: "delete-replay-key", RequestID: "request-delete-first",
	}
	first, err := svc.Delete(ctx, 71, source.ID, req)
	if err != nil {
		t.Fatalf("first Delete: %v", err)
	}
	replay, err := svc.Delete(ctx, 71, source.ID, req)
	if err != nil {
		t.Fatalf("replayed Delete: %v", err)
	}
	if replay.Project.ID != first.Project.ID || replay.Project.Version != first.Project.Version || replay.Transferred != first.Transferred {
		t.Fatalf("replayed delete = %#v, want persisted %#v", replay, first)
	}
	other, _ := svc.Create(ctx, 71, domainproject.CreateRequest{Name: "Other source"})
	if _, err := svc.Delete(ctx, 71, other.ID, domainproject.DeleteRequest{ExpectedVersion: other.Version, IdempotencyKey: req.IdempotencyKey}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("same user/key on another project err = %v, want ErrIdempotencyConflict", err)
	}
}

func TestResolveForWriteFallsBackOnlyToOwnedDefault(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewMemoryStore())
	defaultProject, _ := svc.EnsureDefault(ctx, 9)
	foreign, _ := svc.Create(ctx, 10, domainproject.CreateRequest{Name: "Foreign"})

	resolved, err := svc.ResolveForWrite(ctx, 9, "")
	if err != nil || resolved.ID != defaultProject.ID {
		t.Fatalf("omitted project fallback = %#v, %v", resolved, err)
	}
	if _, err := svc.ResolveForWrite(ctx, 9, foreign.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign explicit project err = %v, want ErrNotFound", err)
	}
}

func TestCreateEnsuresDefaultBeforeCreatingNamedProject(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	svc := NewService(store)

	created, err := svc.Create(ctx, 88, domainproject.CreateRequest{Name: "First named project"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	projects, err := store.List(ctx, 88)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(projects) != 2 || !projects[0].IsDefault || projects[0].Name != domainproject.DefaultName || created.IsDefault {
		t.Fatalf("direct create must preserve exactly one default before the named project, got %#v", projects)
	}
}
