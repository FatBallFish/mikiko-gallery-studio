package objectcleanup

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/storage"
)

func TestProcessOnceBlocksWhileLiveAliasReferencesObject(t *testing.T) {
	ctx := t.Context()
	store := NewMemoryStore()
	identity := Identity{StorageConfigID: "storage-a", StorageDriver: "s3", ObjectKey: "generated/shared.png"}
	store.AddLiveReference(identity, "alias:one")
	if _, err := store.Enqueue(ctx, identity); err != nil {
		t.Fatal(err)
	}
	backend := &cleanupBackend{}
	processor := NewProcessor(store, storage.NewStaticRouter(backend), ProcessorOptions{})

	processed, err := processor.ProcessOnce(ctx)
	if err != nil || !processed {
		t.Fatalf("ProcessOnce = %v, %v", processed, err)
	}
	job := store.Jobs()[0]
	if job.State != StateBlocked || backend.deleteCalls != 0 {
		t.Fatalf("job=%#v deleteCalls=%d", job, backend.deleteCalls)
	}
}

func TestProcessOnceTreatsStorageNotFoundAsDone(t *testing.T) {
	store := NewMemoryStore()
	identity := Identity{StorageDriver: "local", ObjectKey: "missing.png"}
	_, _ = store.Enqueue(t.Context(), identity)
	backend := &cleanupBackend{deleteErr: storage.ErrNotFound}
	processed, err := NewProcessor(store, storage.NewStaticRouter(backend), ProcessorOptions{}).ProcessOnce(t.Context())
	if err != nil || !processed || store.Jobs()[0].State != StateDone {
		t.Fatalf("processed=%v err=%v job=%#v", processed, err, store.Jobs()[0])
	}
}

func TestStorageFailureRetriesAcrossProcessorRestart(t *testing.T) {
	now := time.Date(2026, 8, 9, 2, 0, 0, 0, time.UTC)
	store := NewMemoryStore()
	store.SetNow(func() time.Time { return now })
	identity := Identity{StorageDriver: "local", ObjectKey: "retry.png"}
	_, _ = store.Enqueue(t.Context(), identity)
	backend := &cleanupBackend{deleteErr: errors.New("secret signed-url credential failure")}
	first := NewProcessor(store, storage.NewStaticRouter(backend), ProcessorOptions{Now: func() time.Time { return now }, Jitter: func(time.Duration) time.Duration { return 0 }})
	if processed, err := first.ProcessOnce(t.Context()); err != nil || !processed {
		t.Fatalf("first=%v,%v", processed, err)
	}
	job := store.Jobs()[0]
	if job.State != StateRetry || job.AttemptCount != 1 || job.NextAttemptAt == nil || job.LastErrorMessage == "secret signed-url credential failure" {
		t.Fatalf("retry job=%#v", job)
	}
	now = job.NextAttemptAt.Add(time.Millisecond)
	backend.deleteErr = nil
	second := NewProcessor(store, storage.NewStaticRouter(backend), ProcessorOptions{Now: func() time.Time { return now }})
	if processed, err := second.ProcessOnce(t.Context()); err != nil || !processed || store.Jobs()[0].State != StateDone {
		t.Fatalf("restart=%v,%v job=%#v", processed, err, store.Jobs()[0])
	}
}

func TestConcurrentProcessorsDeleteCanonicalObjectOnce(t *testing.T) {
	store := NewMemoryStore()
	identity := Identity{StorageDriver: "local", ObjectKey: "once.png"}
	_, _ = store.Enqueue(t.Context(), identity)
	backend := &cleanupBackend{}
	processor := NewProcessor(store, storage.NewStaticRouter(backend), ProcessorOptions{})
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = processor.ProcessOnce(context.Background()) }()
	}
	wg.Wait()
	if backend.deleteCalls != 1 || store.Jobs()[0].State != StateDone {
		t.Fatalf("deleteCalls=%d job=%#v", backend.deleteCalls, store.Jobs()[0])
	}
}

func TestDeletingLastReferenceReactivatesOneJobAndReconciliationFindsCandidate(t *testing.T) {
	store := NewMemoryStore()
	identity := Identity{StorageDriver: "local", ObjectKey: "shared.png"}
	store.AddLiveReference(identity, "result:one")
	first, _ := store.Enqueue(t.Context(), identity)
	claim, claimed, err := store.Claim(t.Context(), time.Now())
	if err != nil || !claimed {
		t.Fatalf("Claim() claimed=%v err=%v", claimed, err)
	}
	_ = store.MarkBlocked(t.Context(), claim, "live_reference")
	store.RemoveLiveReference(identity, "result:one")
	second, _ := store.Enqueue(t.Context(), identity)
	third, _ := store.Enqueue(t.Context(), identity)
	if first.ID != second.ID || second.ID != third.ID || len(store.Jobs()) != 1 || store.Jobs()[0].State != StatePending {
		t.Fatalf("jobs=%#v", store.Jobs())
	}

	orphan := Identity{StorageDriver: "s3", ObjectKey: "orphan.png"}
	store.AddDeletedCandidate(orphan)
	if count, err := store.Reconcile(t.Context(), 10); err != nil || count != 1 {
		t.Fatalf("reconcile=%d,%v", count, err)
	}
	if len(store.Jobs()) != 2 {
		t.Fatalf("jobs=%#v", store.Jobs())
	}
}

func TestEnqueueReactivatesRunningJobAndRejectsStaleWorkerTransition(t *testing.T) {
	store := NewMemoryStore()
	identity := Identity{StorageDriver: "local", ObjectKey: "shared-running.png"}
	first, err := store.Enqueue(t.Context(), identity)
	if err != nil {
		t.Fatal(err)
	}
	claim, claimed, err := store.Claim(t.Context(), time.Now())
	if err != nil || !claimed {
		t.Fatalf("Claim() claimed=%v err=%v", claimed, err)
	}

	second, err := store.Enqueue(t.Context(), identity)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("Enqueue() created duplicate job: first=%q second=%q", first.ID, second.ID)
	}
	if err := store.MarkBlocked(t.Context(), claim, "live_reference"); err != nil {
		t.Fatal(err)
	}

	job := store.Jobs()[0]
	if job.State != StatePending {
		t.Fatalf("job.State=%q, want %q", job.State, StatePending)
	}
}

func TestEnqueueAfterDoneCreatesNewPendingJob(t *testing.T) {
	store := NewMemoryStore()
	identity := Identity{StorageDriver: "local", ObjectKey: "generated/reused.png"}
	first, err := store.Enqueue(t.Context(), identity)
	if err != nil {
		t.Fatal(err)
	}
	claim, claimed, err := store.Claim(t.Context(), time.Now())
	if err != nil || !claimed {
		t.Fatalf("Claim() claimed=%v err=%v", claimed, err)
	}
	if err := store.MarkDone(t.Context(), claim); err != nil {
		t.Fatal(err)
	}

	second, err := store.Enqueue(t.Context(), identity)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID || second.State != StatePending {
		t.Fatalf("second=%#v first=%#v", second, first)
	}
	if len(store.Jobs()) != 2 {
		t.Fatalf("jobs=%#v", store.Jobs())
	}
}

func TestConfiguredStorageIdentityUsesConfigAndObjectKeyNamespace(t *testing.T) {
	store := NewMemoryStore()
	first, err := store.Enqueue(t.Context(), Identity{
		StorageConfigID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		StorageDriver:   "s3",
		Bucket:          "old-bucket",
		ObjectKey:       "generated-images/7/shared.png",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Enqueue(t.Context(), Identity{
		StorageConfigID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		StorageDriver:   "renamed-driver",
		Bucket:          "new-bucket",
		ObjectKey:       "generated-images/7/shared.png",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || len(store.Jobs()) != 1 {
		t.Fatalf("first=%#v second=%#v jobs=%#v", first, second, store.Jobs())
	}
}

func TestReconcileScansOnlyOwnedPrefixesWithPaginationGraceAndRestart(t *testing.T) {
	now := time.Date(2026, 8, 9, 4, 0, 0, 0, time.UTC)
	store := NewMemoryStore()
	store.SetNow(func() time.Time { return now })
	live := Identity{StorageDriver: "local", ObjectKey: "generated-images/7/live.png"}
	store.AddLiveReference(live, "result:live")
	backend := &listingCleanupBackend{pages: map[string]map[string]storage.ObjectPage{
		"generated-images/": {
			"": {
				Objects:    []storage.ObjectInfo{{ObjectKey: "generated-images/7/orphan-a.png", ModifiedAt: now.Add(-2 * time.Hour)}},
				NextCursor: "generated-live",
			},
			"generated-live": {
				Objects:    []storage.ObjectInfo{{ObjectKey: live.ObjectKey, ModifiedAt: now.Add(-2 * time.Hour)}},
				NextCursor: "generated-fresh",
			},
			"generated-fresh": {
				Objects:    []storage.ObjectInfo{{ObjectKey: "generated-images/7/fresh.png", ModifiedAt: now.Add(-time.Minute)}},
				NextCursor: "generated-orphan-b",
			},
			"generated-orphan-b": {
				Objects: []storage.ObjectInfo{{ObjectKey: "generated-images/7/orphan-b.png", ModifiedAt: now.Add(-3 * time.Hour)}},
			},
		},
		"reference-assets/": {
			"": {
				Objects: []storage.ObjectInfo{{ObjectKey: "reference-assets/orphan.png", ModifiedAt: now.Add(-4 * time.Hour)}},
			},
		},
		"gallery-exports/": {"": {}},
	}}
	options := ProcessorOptions{
		Now:                func() time.Time { return now },
		OrphanGracePeriod:  time.Hour,
		ObjectListPageSize: 1,
	}
	processor := NewProcessor(store, storage.NewStaticRouter(backend), options)
	for range 7 {
		if _, err := processor.Reconcile(t.Context(), 1); err != nil {
			t.Fatal(err)
		}
	}
	if len(store.Jobs()) != 3 {
		t.Fatalf("jobs=%#v", store.Jobs())
	}
	for _, job := range store.Jobs() {
		if job.Identity.ObjectKey == live.ObjectKey || job.Identity.ObjectKey == "generated-images/7/fresh.png" {
			t.Fatalf("protected object was enqueued: %#v", job)
		}
	}
	for _, prefix := range backend.prefixes {
		if prefix != "generated-images/" && prefix != "reference-assets/" && prefix != "gallery-exports/" {
			t.Fatalf("listed non-owned prefix %q in %v", prefix, backend.prefixes)
		}
	}
	if !strings.Contains(fmt.Sprint(backend.prefixes), "reference-assets/") {
		t.Fatalf("reference prefix starved: %v", backend.prefixes)
	}

	restarted := NewProcessor(store, storage.NewStaticRouter(backend), options)
	if _, err := restarted.Reconcile(t.Context(), 10); err != nil {
		t.Fatal(err)
	}
	if len(store.Jobs()) != 3 {
		t.Fatalf("restart created duplicates: %#v", store.Jobs())
	}
}

func TestReconcileScansHistoricalReadableStorageAfterDefaultSwitch(t *testing.T) {
	now := time.Date(2026, 8, 9, 5, 0, 0, 0, time.UTC)
	objectKey := "generated-images/7/shared.png"
	oldBackend := &listingCleanupBackend{pages: map[string]map[string]storage.ObjectPage{
		"generated-images/": {"": {Objects: []storage.ObjectInfo{{ObjectKey: objectKey, ModifiedAt: now.Add(-2 * time.Hour)}}}},
		"reference-assets/": {"": {}},
		"gallery-exports/":  {"": {}},
	}}
	newBackend := &listingCleanupBackend{pages: map[string]map[string]storage.ObjectPage{
		"generated-images/": {"": {Objects: []storage.ObjectInfo{{ObjectKey: objectKey, ModifiedAt: now.Add(-2 * time.Hour)}}}},
		"reference-assets/": {"": {}},
		"gallery-exports/":  {"": {}},
	}}
	oldRef := storage.BackendRef{ConfigID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Driver: "local", Namespace: "old", Backend: oldBackend}
	newRef := storage.BackendRef{ConfigID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", Driver: "local", Namespace: "new", Backend: newBackend}
	router := &multiCleanupRouter{defaultRef: newRef, refs: []storage.BackendRef{oldRef, newRef}}
	store := NewMemoryStore()
	store.SetNow(func() time.Time { return now })
	store.AddLiveReference(Identity{StorageConfigID: newRef.ConfigID, StorageDriver: "local", ObjectKey: objectKey}, "result:new")
	processor := NewProcessor(store, router, ProcessorOptions{Now: func() time.Time { return now }, OrphanGracePeriod: time.Hour, ObjectListPageSize: 10})

	if _, err := processor.Reconcile(t.Context(), 4); err != nil {
		t.Fatal(err)
	}
	jobs := store.Jobs()
	if len(jobs) != 1 || jobs[0].Identity.StorageConfigID != oldRef.ConfigID {
		t.Fatalf("cleanup jobs=%#v", jobs)
	}
	if processed, err := processor.ProcessOnce(t.Context()); err != nil || !processed {
		t.Fatalf("ProcessOnce()=%v err=%v", processed, err)
	}
	if oldBackend.deleteCalls != 1 || newBackend.deleteCalls != 0 {
		t.Fatalf("delete calls old=%d new=%d", oldBackend.deleteCalls, newBackend.deleteCalls)
	}
}

func TestReconcilePersistsObjectCursorAcrossProcessorRestart(t *testing.T) {
	now := time.Date(2026, 8, 9, 6, 0, 0, 0, time.UTC)
	liveKey := "generated-images/7/live-first.png"
	orphanKey := "generated-images/7/orphan-second.png"
	backend := &listingCleanupBackend{pages: map[string]map[string]storage.ObjectPage{
		"generated-images/": {
			"":          {Objects: []storage.ObjectInfo{{ObjectKey: liveKey, ModifiedAt: now.Add(-2 * time.Hour)}}, NextCursor: "next-page"},
			"next-page": {Objects: []storage.ObjectInfo{{ObjectKey: orphanKey, ModifiedAt: now.Add(-2 * time.Hour)}}, NextCursor: ""},
		},
		"reference-assets/": {"": {}},
		"gallery-exports/":  {"": {}},
	}}
	ref := storage.BackendRef{ConfigID: "cccccccc-cccc-cccc-cccc-cccccccccccc", Driver: "local", Namespace: "stable-c", Backend: backend}
	store := NewMemoryStore()
	store.SetNow(func() time.Time { return now })
	store.AddLiveReference(Identity{StorageConfigID: ref.ConfigID, StorageDriver: ref.Driver, ObjectKey: liveKey}, "result:live")
	options := ProcessorOptions{Now: func() time.Time { return now }, OrphanGracePeriod: time.Hour, ObjectListPageSize: 1}

	if _, err := NewProcessor(store, &multiCleanupRouter{defaultRef: ref, refs: []storage.BackendRef{ref}}, options).Reconcile(t.Context(), 2); err != nil {
		t.Fatal(err)
	}
	restarted := NewProcessor(store, &multiCleanupRouter{defaultRef: ref, refs: []storage.BackendRef{ref}}, options)
	if _, err := restarted.Reconcile(t.Context(), 2); err != nil {
		t.Fatal(err)
	}
	jobs := store.Jobs()
	if len(jobs) != 1 || jobs[0].Identity.ObjectKey != orphanKey {
		t.Fatalf("restart jobs=%#v cursor calls=%v", jobs, backend.cursors)
	}
	if !slices.Contains(backend.cursors, "next-page") {
		t.Fatalf("restart did not continue persisted cursor: %v", backend.cursors)
	}
}

type cleanupBackend struct {
	mu          sync.Mutex
	deleteCalls int
	deleteErr   error
}

type listingCleanupBackend struct {
	cleanupBackend
	pages    map[string]map[string]storage.ObjectPage
	prefixes []string
	cursors  []string
}

type multiCleanupRouter struct {
	defaultRef storage.BackendRef
	refs       []storage.BackendRef
}

func (r *multiCleanupRouter) DefaultWriter(context.Context) (storage.BackendRef, error) {
	return r.defaultRef, nil
}

func (r *multiCleanupRouter) BackendFor(_ context.Context, configID, _ string) (storage.BackendRef, error) {
	for _, ref := range r.refs {
		if ref.ConfigID == configID {
			return ref, nil
		}
	}
	return storage.BackendRef{}, storage.ErrStorageUnreadable
}

func (r *multiCleanupRouter) ReadableBackends(context.Context) ([]storage.BackendRef, error) {
	return append([]storage.BackendRef(nil), r.refs...), nil
}

func (b *listingCleanupBackend) ListObjects(_ context.Context, prefix, cursor string, _ int) (storage.ObjectPage, error) {
	b.prefixes = append(b.prefixes, prefix)
	b.cursors = append(b.cursors, cursor)
	return b.pages[prefix][cursor], nil
}

func (*cleanupBackend) Driver() string                                    { return "local" }
func (*cleanupBackend) Put(context.Context, string, string, []byte) error { return nil }
func (*cleanupBackend) Get(context.Context, string) ([]byte, error)       { return nil, storage.ErrNotFound }
func (b *cleanupBackend) Delete(context.Context, string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.deleteCalls++
	return b.deleteErr
}
