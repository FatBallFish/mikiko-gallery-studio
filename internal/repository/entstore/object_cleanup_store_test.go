package entstore

import (
	"testing"
	"time"

	"entgo.io/ent/dialect"
	domaincleanup "github.com/fatballfish/pic-gallery/internal/domain/objectcleanup"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

func TestObjectCleanupStoreStaleWorkerTransitionDoesNotOverrideReenqueue(t *testing.T) {
	tests := []struct {
		name       string
		transition func(*ObjectCleanupStore, string) error
	}{
		{
			name: "done",
			transition: func(store *ObjectCleanupStore, id string) error {
				return store.MarkDone(t.Context(), id)
			},
		},
		{
			name: "blocked",
			transition: func(store *ObjectCleanupStore, id string) error {
				return store.MarkBlocked(t.Context(), id, "live_reference")
			},
		},
		{
			name: "retry",
			transition: func(store *ObjectCleanupStore, id string) error {
				return store.MarkRetry(t.Context(), id, time.Now().Add(time.Minute), "delete_failed", "delete failed")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := repoent.Open(dialect.SQLite, "file:cleanup-transition-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			if err := client.Schema.Create(t.Context()); err != nil {
				t.Fatal(err)
			}

			store := NewObjectCleanupStore(client)
			identity := domaincleanup.Identity{StorageDriver: "local", ObjectKey: "generated/shared.png"}
			first, err := store.Enqueue(t.Context(), identity)
			if err != nil {
				t.Fatal(err)
			}
			if _, claimed, err := store.Claim(t.Context(), time.Now()); err != nil || !claimed {
				t.Fatalf("Claim() claimed=%v err=%v", claimed, err)
			}

			second, err := store.Enqueue(t.Context(), identity)
			if err != nil {
				t.Fatal(err)
			}
			if first.ID != second.ID {
				t.Fatalf("Enqueue() created duplicate job: first=%q second=%q", first.ID, second.ID)
			}
			if err := test.transition(store, first.ID); err != nil {
				t.Fatal(err)
			}

			job, err := client.ObjectDeletionJob.Query().Only(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if job.State != domaincleanup.StatePending {
				t.Fatalf("job.State=%q, want %q", job.State, domaincleanup.StatePending)
			}
			if job.CompletedAt != nil || job.NextAttemptAt != nil || job.LastErrorCode != nil || job.LastErrorMessage != nil {
				t.Fatalf("stale transition metadata persisted: %#v", job)
			}
		})
	}
}
