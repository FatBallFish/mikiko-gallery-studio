package entstore

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	domaincleanup "github.com/fatballfish/pic-gallery/internal/domain/objectcleanup"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
)

func TestObjectCleanupStoreConcurrentEnqueuePostgres(t *testing.T) {
	ctx, _, clientA, clientB := openProjectTaskPostgres(t)
	var creates atomic.Int32
	release := make(chan struct{})
	barrier := func(next repoent.Mutator) repoent.Mutator {
		return repoent.MutateFunc(func(ctx context.Context, mutation repoent.Mutation) (repoent.Value, error) {
			if _, ok := mutation.(*repoent.ObjectDeletionJobMutation); ok && creates.Add(1) <= 2 {
				if creates.Load() == 2 {
					close(release)
				}
				<-release
			}
			return next.Mutate(ctx, mutation)
		})
	}
	clientA.Use(barrier)
	clientB.Use(barrier)

	identity := domaincleanup.Identity{StorageDriver: "s3", Bucket: "generated", ObjectKey: "shared/postgres.png"}
	results := make(chan domaincleanup.Job, 2)
	errorsCh := make(chan error, 2)
	var wg sync.WaitGroup
	for _, store := range []*ObjectCleanupStore{NewObjectCleanupStore(clientA), NewObjectCleanupStore(clientB)} {
		wg.Add(1)
		go func(store *ObjectCleanupStore) {
			defer wg.Done()
			job, err := store.Enqueue(ctx, identity)
			results <- job
			errorsCh <- err
		}(store)
	}
	wg.Wait()
	close(results)
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("concurrent PostgreSQL Enqueue: %v", err)
		}
	}
	var jobID string
	for job := range results {
		if jobID == "" {
			jobID = job.ID
		} else if job.ID != jobID {
			t.Fatalf("PostgreSQL enqueue returned different jobs: %q != %q", job.ID, jobID)
		}
	}
	if count, err := clientA.ObjectDeletionJob.Query().Count(ctx); err != nil || count != 1 {
		t.Fatalf("PostgreSQL live cleanup jobs=%d err=%v", count, err)
	}
}
