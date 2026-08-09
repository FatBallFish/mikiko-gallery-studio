package galleryexport

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
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
		Owner: "worker-1", ArchiveTTL: 2 * time.Hour, Now: func() time.Time { return now }, AttemptToken: func() string { return "11111111-1111-4111-8111-111111111111" },
	})
	processed, err := processor.ProcessOnce(context.Background())
	if err != nil || !processed {
		t.Fatalf("process export: processed=%v err=%v", processed, err)
	}
	if store.completed.JobID != "job-1" || store.completed.ObjectKey != "gallery-exports/7/job-1/attempt-1-11111111-1111-4111-8111-111111111111.zip" || store.completed.ArchiveSizeBytes <= 0 {
		t.Fatalf("complete request = %#v", store.completed)
	}
	if !store.completed.ExpiresAt.Equal(now.Add(2 * time.Hour)) {
		t.Fatalf("archive expiry = %s", store.completed.ExpiresAt)
	}
	if !store.completed.CompletedAt.Equal(now) {
		t.Fatalf("completion time = %s", store.completed.CompletedAt)
	}
	if _, ok := backend.objects[store.completed.ObjectKey]; !ok {
		t.Fatalf("temporary archive was not persisted: %#v", backend.objects)
	}
}

func TestProcessorRetainsArchiveWhenCompletionCommittedBeforeConnectionError(t *testing.T) {
	backend := &exportBackend{objects: map[string][]byte{"source.png": []byte("png")}}
	store := &ambiguousCompletionStore{attemptStore: attemptStore{
		activeAttempt: 1,
		assets:        []Asset{{ID: "one", ObjectKey: "source.png", MIMEType: "image/png"}},
	}}
	job := Job{ID: "job-ambiguous", UserID: 7, ProjectID: "project", ImageIDs: []string{"one"}, State: StateRunning, LeaseOwner: "worker", AttemptCount: 1}
	processor := NewProcessor(store, storage.NewStaticRouter(backend), ProcessorOptions{
		Owner: "worker", AttemptToken: func() string { return "33333333-3333-4333-8333-333333333333" },
	})

	processErr := processor.processJob(t.Context(), job, time.Now().UTC())
	if processErr == nil || processErr.code != "archive_completion_failed" {
		t.Fatalf("ambiguous completion error=%#v", processErr)
	}
	completed := store.completedRequest()
	if _, exists := backend.objects[completed.ObjectKey]; completed.ObjectKey == "" || !exists {
		t.Fatalf("committed archive was deleted after ambiguous error: request=%#v objects=%#v", completed, backend.objects)
	}
	archive, err := NewService(store, storage.NewStaticRouter(backend), Options{}).DownloadJob(t.Context(), 7, job.ID)
	if err != nil {
		t.Fatalf("download committed archive: %v", err)
	}
	payload, err := io.ReadAll(archive.Reader)
	_ = archive.Close()
	if err != nil || len(payload) == 0 {
		t.Fatalf("committed archive payload bytes=%d err=%v", len(payload), err)
	}
}

type ambiguousCompletionStore struct{ attemptStore }

func (s *ambiguousCompletionStore) CompleteJob(ctx context.Context, req CompleteJobRequest) (Job, error) {
	job, err := s.attemptStore.CompleteJob(ctx, req)
	if err != nil {
		return job, err
	}
	return job, errors.New("injected connection reset after commit")
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
func (*blockingExportBackend) PutReader(context.Context, string, string, io.Reader, int64) error {
	return nil
}
func (*blockingExportBackend) OpenReader(context.Context, string, int64) (io.ReadCloser, int64, error) {
	return nil, 0, storage.ErrNotFound
}
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
	deadline := time.Now().UTC().Add(20 * time.Millisecond)
	backend := &blockingExportBackend{started: make(chan struct{})}
	store := &processorStoreStub{
		exportStoreStub: exportStoreStub{assets: []Asset{{ID: "one", ObjectKey: "source.png", MIMEType: "image/png"}}},
		claimed:         Job{ID: "job-timeout", UserID: 7, ProjectID: "project-1", ImageIDs: []string{"one"}, State: StateRunning, LeaseOwner: "worker-1", AttemptCount: 1, DeadlineAt: &deadline},
		renewOK:         true,
	}
	processor := NewProcessor(store, storage.NewStaticRouter(backend), ProcessorOptions{Owner: "worker-1", LeaseTTL: time.Second, AsyncTimeout: time.Hour})
	processed, err := processor.ProcessOnce(t.Context())
	if !processed || err != nil {
		t.Fatalf("timeout result processed=%v err=%v", processed, err)
	}
	if !errors.Is(backend.lastError(), context.DeadlineExceeded) {
		t.Fatalf("backend context error=%v, want deadline exceeded", backend.lastError())
	}
}

func TestProcessorRejectsByteOnlyArchiveStorageBeforeCompletion(t *testing.T) {
	store := &processorStoreStub{exportStoreStub: exportStoreStub{assets: []Asset{{ID: "one", ObjectKey: "source.png"}}}, claimed: Job{ID: "job-byte-only", UserID: 7, ProjectID: "project", ImageIDs: []string{"one"}, State: StateRunning, AttemptCount: 1}}
	backend := &byteOnlyBackend{objects: map[string][]byte{"source.png": []byte("source")}}
	processor := NewProcessor(store, storage.NewStaticRouter(backend), ProcessorOptions{Owner: "worker"})
	processErr := processor.processJob(t.Context(), store.claimed, time.Now().UTC())
	if processErr == nil || processErr.code != "streaming_storage_unsupported" {
		t.Fatalf("byte-only storage error=%#v", processErr)
	}
	if store.completed.JobID != "" {
		t.Fatalf("unsupported storage completed job: %#v", store.completed)
	}
}

type byteOnlyBackend struct{ objects map[string][]byte }

func (*byteOnlyBackend) Driver() string { return "legacy" }
func (b *byteOnlyBackend) Put(_ context.Context, key, _ string, content []byte) error {
	b.objects[key] = append([]byte(nil), content...)
	return nil
}
func (b *byteOnlyBackend) Get(_ context.Context, key string) ([]byte, error) {
	content, ok := b.objects[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return append([]byte(nil), content...), nil
}
func (b *byteOnlyBackend) Delete(_ context.Context, key string) error {
	delete(b.objects, key)
	return nil
}

func (b *blockingExportBackend) lastError() error {
	select {
	case <-b.started:
		return context.DeadlineExceeded
	default:
		return nil
	}
}

func TestProcessorAttemptObjectsAreIsolatedAcrossLeaseTakeoverOrderings(t *testing.T) {
	for _, blockBeforeStore := range []bool{false, true} {
		name := "old_uploads_before_winner"
		if blockBeforeStore {
			name = "winner_completes_before_old_upload"
		}
		t.Run(name, func(t *testing.T) {
			backend := newAttemptBackend(blockBeforeStore)
			store := &attemptStore{activeAttempt: 1, assets: []Asset{{ID: "one", ObjectKey: "source.png", MIMEType: "image/png"}}}
			backend.objects["source.png"] = []byte("source")
			oldJob := Job{ID: "job-takeover", UserID: 7, ProjectID: "project", ImageIDs: []string{"one"}, State: StateRunning, LeaseOwner: "old", AttemptCount: 1}
			newJob := oldJob
			newJob.LeaseOwner, newJob.AttemptCount = "new", 2
			oldProcessor := NewProcessor(store, storage.NewStaticRouter(backend), ProcessorOptions{Owner: "old", AttemptToken: func() string { return "11111111-1111-4111-8111-111111111111" }})
			newProcessor := NewProcessor(store, storage.NewStaticRouter(backend), ProcessorOptions{Owner: "new", AttemptToken: func() string { return "22222222-2222-4222-8222-222222222222" }})
			oldDone := make(chan *jobProcessError, 1)
			go func() { oldDone <- oldProcessor.processJob(t.Context(), oldJob, time.Now().UTC()) }()
			<-backend.oldBlocked
			store.setActiveAttempt(2)
			if err := newProcessor.processJob(t.Context(), newJob, time.Now().UTC()); err != nil {
				t.Fatalf("new attempt: %#v", err)
			}
			close(backend.releaseOld)
			if err := <-oldDone; err != nil {
				t.Fatalf("stale attempt should be benign: %#v", err)
			}

			winner := store.completedRequest()
			if winner.AttemptCount != 2 || !strings.Contains(winner.ObjectKey, "/attempt-2-") {
				t.Fatalf("winner=%#v", winner)
			}
			oldKey := "gallery-exports/7/job-takeover/attempt-1-11111111-1111-4111-8111-111111111111.zip"
			if !backend.exists(oldKey) {
				t.Fatalf("stale attempt must remain for reference-aware orphan reconciliation: %s", oldKey)
			}
			if !backend.exists(winner.ObjectKey) {
				t.Fatalf("winner object was deleted: %s", winner.ObjectKey)
			}
			archive, err := NewService(store, storage.NewStaticRouter(backend), Options{}).DownloadJob(t.Context(), 7, oldJob.ID)
			if err != nil {
				t.Fatal(err)
			}
			content, err := io.ReadAll(archive.Reader)
			_ = archive.Close()
			if err != nil || len(content) == 0 {
				t.Fatalf("winner download bytes=%d err=%v", len(content), err)
			}
		})
	}
}

type attemptStore struct {
	mu            sync.Mutex
	activeAttempt int
	assets        []Asset
	completed     CompleteJobRequest
}

func (s *attemptStore) AuthorizeAssets(context.Context, int64, string, []string) ([]Asset, error) {
	return append([]Asset(nil), s.assets...), nil
}
func (*attemptStore) CreateJob(context.Context, CreateJobRequest) (Job, error) { return Job{}, nil }
func (*attemptStore) AcquireNextJob(context.Context, string, time.Time, time.Duration) (Job, bool, error) {
	return Job{}, false, nil
}
func (*attemptStore) RenewJobLease(context.Context, string, string, int, time.Time, time.Duration) (bool, error) {
	return true, nil
}
func (*attemptStore) FailJob(context.Context, Job, time.Time, string, string) error { return nil }
func (*attemptStore) ExpireReady(context.Context, time.Time, int) (int, error)      { return 0, nil }
func (s *attemptStore) CompleteJob(_ context.Context, req CompleteJobRequest) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if req.AttemptCount != s.activeAttempt || s.completed.JobID != "" {
		return Job{}, repoerr.ErrConflict
	}
	s.completed = req
	now := time.Now().UTC().Add(time.Hour)
	return Job{ID: req.JobID, UserID: 7, State: StateSucceeded, ObjectKey: req.ObjectKey, ArchiveSizeBytes: req.ArchiveSizeBytes, ExpiresAt: &now}, nil
}
func (s *attemptStore) GetJob(_ context.Context, _ int64, jobID string, _ time.Time) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.completed.JobID != jobID {
		return Job{}, repoerr.ErrNotFound
	}
	expires := time.Now().UTC().Add(time.Hour)
	return Job{ID: jobID, UserID: 7, State: StateSucceeded, ObjectKey: s.completed.ObjectKey, ArchiveSizeBytes: s.completed.ArchiveSizeBytes, ExpiresAt: &expires}, nil
}
func (s *attemptStore) setActiveAttempt(attempt int) {
	s.mu.Lock()
	s.activeAttempt = attempt
	s.mu.Unlock()
}
func (s *attemptStore) completedRequest() CompleteJobRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.completed
}

type attemptBackend struct {
	mu               sync.Mutex
	objects          map[string][]byte
	blockBeforeStore bool
	oldBlocked       chan struct{}
	releaseOld       chan struct{}
}

func newAttemptBackend(blockBeforeStore bool) *attemptBackend {
	return &attemptBackend{objects: map[string][]byte{}, blockBeforeStore: blockBeforeStore, oldBlocked: make(chan struct{}), releaseOld: make(chan struct{})}
}
func (*attemptBackend) Driver() string { return "local" }
func (b *attemptBackend) Put(ctx context.Context, key, contentType string, content []byte) error {
	return b.PutReader(ctx, key, contentType, bytes.NewReader(content), int64(len(content)))
}
func (b *attemptBackend) PutReader(_ context.Context, key, _ string, reader io.Reader, _ int64) error {
	content, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	old := strings.Contains(key, "/attempt-1-")
	if old && b.blockBeforeStore {
		close(b.oldBlocked)
		<-b.releaseOld
	}
	b.mu.Lock()
	b.objects[key] = content
	b.mu.Unlock()
	if old && !b.blockBeforeStore {
		close(b.oldBlocked)
		<-b.releaseOld
	}
	return nil
}
func (b *attemptBackend) Get(_ context.Context, key string) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	content, ok := b.objects[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return append([]byte(nil), content...), nil
}
func (b *attemptBackend) OpenReader(ctx context.Context, key string, max int64) (io.ReadCloser, int64, error) {
	content, err := b.Get(ctx, key)
	if err != nil {
		return nil, 0, err
	}
	if int64(len(content)) > max {
		return nil, 0, storage.ErrObjectTooLarge
	}
	return io.NopCloser(bytes.NewReader(content)), int64(len(content)), nil
}
func (b *attemptBackend) Delete(_ context.Context, key string) error {
	b.mu.Lock()
	delete(b.objects, key)
	b.mu.Unlock()
	return nil
}
func (b *attemptBackend) exists(key string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.objects[key]
	return ok
}
