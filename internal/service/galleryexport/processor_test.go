package galleryexport

import (
	"context"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/storage"
)

func TestProcessorBuildsQueuedArchiveAndPersistsExpiringCleanupIdentity(t *testing.T) {
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	backend := &exportBackend{objects: map[string][]byte{"source.png": []byte("png")}}
	store := &processorStoreStub{
		exportStoreStub: exportStoreStub{assets: []Asset{{ID: "one", ObjectKey: "source.png", MIMEType: "image/png"}}},
		claimed:         Job{ID: "job-1", UserID: 7, ProjectID: "project-1", ImageIDs: []string{"one"}, State: StateRunning, LeaseOwner: "worker-1", AttemptCount: 1},
	}
	processor := NewProcessor(store, storage.NewStaticRouter(backend), ProcessorOptions{
		Owner: "worker-1", ArchiveTTL: 2 * time.Hour, Now: func() time.Time { return now },
	})
	processed, err := processor.ProcessOnce(context.Background())
	if err != nil || !processed {
		t.Fatalf("process export: processed=%v err=%v", processed, err)
	}
	if store.completed.JobID != "job-1" || store.completed.ObjectKey != "gallery-exports/7/job-1.zip" || store.completed.ArchiveSizeBytes <= 0 {
		t.Fatalf("complete request = %#v", store.completed)
	}
	if !store.completed.ExpiresAt.Equal(now.Add(2 * time.Hour)) {
		t.Fatalf("archive expiry = %s", store.completed.ExpiresAt)
	}
	if _, ok := backend.objects[store.completed.ObjectKey]; !ok {
		t.Fatalf("temporary archive was not persisted: %#v", backend.objects)
	}
}

type processorStoreStub struct {
	exportStoreStub
	claimed   Job
	completed CompleteJobRequest
}

func (s *processorStoreStub) AcquireNextJob(context.Context, string, time.Time, time.Duration) (Job, bool, error) {
	return s.claimed, s.claimed.ID != "", nil
}
func (s *processorStoreStub) CompleteJob(_ context.Context, req CompleteJobRequest) (Job, error) {
	s.completed = req
	job := s.claimed
	job.State = StateSucceeded
	return job, nil
}
func (*processorStoreStub) FailJob(context.Context, Job, time.Time, string, string) error { return nil }
func (*processorStoreStub) ExpireReady(context.Context, time.Time, int) (int, error)      { return 0, nil }
