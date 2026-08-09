package galleryexport

import (
	"context"
	"errors"
	"sync"
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
	renewed   chan struct{}
	renewOK   bool
	mu        sync.Mutex
}

func (s *processorStoreStub) AcquireNextJob(context.Context, string, time.Time, time.Duration) (Job, bool, error) {
	return s.claimed, s.claimed.ID != "", nil
}
func (s *processorStoreStub) CompleteJob(_ context.Context, req CompleteJobRequest) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completed = req
	job := s.claimed
	job.State = StateSucceeded
	return job, nil
}
func (s *processorStoreStub) RenewJobLease(context.Context, string, string, int, time.Time, time.Duration) (bool, error) {
	if s.renewed != nil {
		select {
		case s.renewed <- struct{}{}:
		default:
		}
	}
	return s.renewOK, nil
}
func (*processorStoreStub) FailJob(context.Context, Job, time.Time, string, string) error { return nil }
func (*processorStoreStub) ExpireReady(context.Context, time.Time, int) (int, error)      { return 0, nil }

func TestProcessorRenewsLeaseAndTreatsLostOwnershipAsBenign(t *testing.T) {
	renewed := make(chan struct{}, 1)
	backend := &blockingExportBackend{started: make(chan struct{})}
	store := &processorStoreStub{
		exportStoreStub: exportStoreStub{assets: []Asset{{ID: "one", ObjectKey: "source.png", MIMEType: "image/png"}}},
		claimed:         Job{ID: "job-lease", UserID: 7, ProjectID: "project-1", ImageIDs: []string{"one"}, State: StateRunning, LeaseOwner: "worker-1", AttemptCount: 2},
		renewed:         renewed,
		renewOK:         false,
	}
	processor := NewProcessor(store, storage.NewStaticRouter(backend), ProcessorOptions{Owner: "worker-1", LeaseTTL: 30 * time.Millisecond})
	processed, err := processor.ProcessOnce(t.Context())
	if err != nil || !processed {
		t.Fatalf("lost lease result processed=%v err=%v", processed, err)
	}
	select {
	case <-renewed:
	default:
		t.Fatal("processor never attempted to renew its lease")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.completed.JobID != "" {
		t.Fatalf("lost owner completed job: %#v", store.completed)
	}
}

type blockingExportBackend struct{ started chan struct{} }

func (*blockingExportBackend) Driver() string                                    { return "local" }
func (*blockingExportBackend) Put(context.Context, string, string, []byte) error { return nil }
func (b *blockingExportBackend) Get(ctx context.Context, _ string) ([]byte, error) {
	select {
	case <-b.started:
	default:
		close(b.started)
	}
	<-ctx.Done()
	return nil, ctx.Err()
}
func (*blockingExportBackend) Delete(context.Context, string) error { return nil }

func TestProcessorAppliesAsyncDeadline(t *testing.T) {
	backend := &blockingExportBackend{started: make(chan struct{})}
	store := &processorStoreStub{
		exportStoreStub: exportStoreStub{assets: []Asset{{ID: "one", ObjectKey: "source.png", MIMEType: "image/png"}}},
		claimed:         Job{ID: "job-timeout", UserID: 7, ProjectID: "project-1", ImageIDs: []string{"one"}, State: StateRunning, LeaseOwner: "worker-1", AttemptCount: 1},
		renewOK:         true,
	}
	processor := NewProcessor(store, storage.NewStaticRouter(backend), ProcessorOptions{Owner: "worker-1", LeaseTTL: time.Second, AsyncTimeout: 20 * time.Millisecond})
	processed, err := processor.ProcessOnce(t.Context())
	if !processed || err != nil {
		t.Fatalf("timeout result processed=%v err=%v", processed, err)
	}
	if !errors.Is(backend.lastError(), context.DeadlineExceeded) {
		t.Fatalf("backend context error=%v, want deadline exceeded", backend.lastError())
	}
}

func (b *blockingExportBackend) lastError() error {
	select {
	case <-b.started:
		return context.DeadlineExceeded
	default:
		return nil
	}
}
