package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

const tinyPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAFgwJ/lqR5DQAAAABJRU5ErkJggg=="

type fakeProvider struct {
	generateFunc func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error)
}

type fakeCleanupService struct{ calls atomic.Int32 }

type fakeGalleryExportService struct{ calls atomic.Int32 }

func (s *fakeGalleryExportService) ProcessOnce(context.Context) (bool, error) {
	s.calls.Add(1)
	return true, nil
}

func (s *fakeCleanupService) ProcessOnce(context.Context) (bool, error) {
	s.calls.Add(1)
	return true, nil
}
func (*fakeCleanupService) Reconcile(context.Context, int) (int, error) { return 0, nil }

func TestRunnerProcessesCleanupBeforeLookingForImageTask(t *testing.T) {
	cleanup := &fakeCleanupService{}
	runner := NewRunner(fakeTaskService{}, Config{Owner: "cleanup-worker"})
	runner.SetCleanupService(cleanup)
	processed, err := runner.ProcessOnce(t.Context())
	if err != nil || !processed || cleanup.calls.Load() != 1 {
		t.Fatalf("processed=%v err=%v calls=%d", processed, err, cleanup.calls.Load())
	}
}

func TestRunnerProcessesGalleryExportBeforeLookingForImageTask(t *testing.T) {
	exports := &fakeGalleryExportService{}
	runner := NewRunner(fakeTaskService{}, Config{Owner: "export-worker"})
	runner.SetGalleryExportService(exports)
	processed, err := runner.ProcessOnce(t.Context())
	if err != nil || !processed || exports.calls.Load() != 1 {
		t.Fatalf("processed=%v err=%v calls=%d", processed, err, exports.calls.Load())
	}
}

func TestRunnerRoundRobinsCleanupExportAndGenerationBacklogs(t *testing.T) {
	cleanup := &fakeCleanupService{}
	exports := &fakeGalleryExportService{}
	var generated atomic.Int32
	tasks := fakeTaskService{
		acquireFunc: func(context.Context, string, time.Duration) (domainimagetask.Task, bool, error) {
			return domainimagetask.Task{ID: fmt.Sprintf("task-%d", generated.Load()+1)}, true, nil
		},
		heartbeatFunc: func(context.Context, string, string, time.Duration) (domainimagetask.Task, error) {
			return domainimagetask.Task{}, nil
		},
		executeFunc: func(context.Context, domainimagetask.Task, string, []string) (domainimagetask.ExecuteResult, error) {
			generated.Add(1)
			return domainimagetask.ExecuteResult{}, nil
		},
	}
	runner := NewRunner(tasks, Config{Owner: "round-robin-worker"})
	runner.SetCleanupService(cleanup)
	runner.SetGalleryExportService(exports)
	for range 12 {
		processed, err := runner.ProcessOnce(t.Context())
		if err != nil || !processed {
			t.Fatalf("ProcessOnce processed=%v err=%v", processed, err)
		}
	}
	if cleanup.calls.Load() < 2 || exports.calls.Load() < 2 || generated.Load() < 4 {
		t.Fatalf("backlog starvation: cleanup=%d exports=%d generated=%d", cleanup.calls.Load(), exports.calls.Load(), generated.Load())
	}
}

func TestRunnerBoundsCleanupStreakWhileCleanupBacklogAndImageTasksRemain(t *testing.T) {
	cleanup := &fakeCleanupService{}
	var acquired atomic.Int32
	tasks := fakeTaskService{
		acquireFunc: func(context.Context, string, time.Duration) (domainimagetask.Task, bool, error) {
			index := acquired.Add(1)
			return domainimagetask.Task{ID: fmt.Sprintf("task-%d", index)}, true, nil
		},
		heartbeatFunc: func(context.Context, string, string, time.Duration) (domainimagetask.Task, error) {
			return domainimagetask.Task{}, nil
		},
		executeFunc: func(context.Context, domainimagetask.Task, string, []string) (domainimagetask.ExecuteResult, error) {
			return domainimagetask.ExecuteResult{}, nil
		},
	}
	runner := NewRunner(tasks, Config{Owner: "fair-worker"})
	runner.SetCleanupService(cleanup)
	for range 6 {
		processed, err := runner.ProcessOnce(t.Context())
		if err != nil || !processed {
			t.Fatalf("ProcessOnce processed=%v err=%v", processed, err)
		}
	}
	if got := acquired.Load(); got < 3 {
		t.Fatalf("sustained cleanup backlog starved image tasks: acquired=%d cleanup=%d", got, cleanup.calls.Load())
	}
	if got := cleanup.calls.Load(); got < 3 {
		t.Fatalf("fair scheduling starved cleanup: acquired=%d cleanup=%d", acquired.Load(), got)
	}
}

func (f fakeProvider) Generate(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
	return f.generateFunc(ctx, req)
}

func (f fakeProvider) Edit(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
	return provider.ImageResponse{}, nil
}

func TestRunnerProcessOnceExecutesQueuedTask(t *testing.T) {
	cfg := workerTestConfig()
	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			return provider.ImageResponse{Created: 1770000004, Data: []provider.ImageResult{{B64JSON: tinyPNGBase64}}}, nil
		}},
	}
	store := imagetaskservice.NewMemoryStore()
	svc := imagetaskservice.NewServiceWithProvidersAndStore(cfg, providers, store)

	created, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:           81,
		AbstractModel:    "plus",
		TaskType:         string(provider.TaskTypeTextToImage),
		Prompt:           "worker task",
		RequestedSize:    "auto",
		BaseResolution:   "auto",
		OutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	runner := NewRunner(svc, Config{Owner: "worker-1", LeaseTTL: 30 * time.Second, PreferredProviders: []string{"openrouter"}})
	processed, err := runner.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce: %v", err)
	}
	if !processed {
		t.Fatal("expected worker to process queued task")
	}

	loaded, err := svc.GetByID(context.Background(), 81, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if loaded.Status != domainimagetask.StatusSucceeded {
		t.Fatalf("expected succeeded task, got %s", loaded.Status)
	}
	if len(loaded.Results) != 1 || loaded.Results[0].URL == "" {
		t.Fatalf("expected worker result, got %#v", loaded.Results)
	}
}

type countingStore struct {
	base            imagetaskservice.Store
	mu              sync.Mutex
	heartbeatCalls  int
	heartbeatSignal chan struct{}
}

func newCountingStore(base imagetaskservice.Store) *countingStore {
	return &countingStore{
		base:            base,
		heartbeatSignal: make(chan struct{}, 8),
	}
}

func (s *countingStore) Save(ctx context.Context, task domainimagetask.Task) error {
	return s.base.Save(ctx, task)
}

func (s *countingStore) SaveIfOwned(ctx context.Context, task domainimagetask.Task, owner string, now time.Time) error {
	return s.base.SaveIfOwned(ctx, task, owner, now)
}

func (s *countingStore) UpdateProgressIfOwned(ctx context.Context, taskID, owner, stage, message string, now time.Time) error {
	return s.base.UpdateProgressIfOwned(ctx, taskID, owner, stage, message, now)
}

func (s *countingStore) SaveTerminalState(ctx context.Context, task domainimagetask.Task, owner string, now time.Time) error {
	return s.base.SaveTerminalState(ctx, task, owner, now)
}

func (s *countingStore) GetByID(ctx context.Context, userID int64, taskID string) (domainimagetask.Task, error) {
	return s.base.GetByID(ctx, userID, taskID)
}

func (s *countingStore) GetImageResultByID(ctx context.Context, userID int64, imageID string) (provider.ImageResult, error) {
	return s.base.GetImageResultByID(ctx, userID, imageID)
}

func (s *countingStore) GetImageResultForAdmin(ctx context.Context, imageID string) (provider.ImageResult, error) {
	return s.base.GetImageResultForAdmin(ctx, imageID)
}

func (s *countingStore) ListByUser(ctx context.Context, userID int64) ([]domainimagetask.Task, error) {
	return s.base.ListByUser(ctx, userID)
}

func (s *countingStore) RequestPublish(ctx context.Context, userID int64, imageID string) (domainimagetask.GalleryImage, error) {
	return s.base.RequestPublish(ctx, userID, imageID)
}

func (s *countingStore) CancelPublish(ctx context.Context, userID int64, imageID string) (domainimagetask.GalleryImage, error) {
	return s.base.CancelPublish(ctx, userID, imageID)
}

func (s *countingStore) SetImageGroup(ctx context.Context, userID int64, imageID, imageGroup string) (domainimagetask.GalleryImage, error) {
	return s.base.SetImageGroup(ctx, userID, imageID, imageGroup)
}

func (s *countingStore) ReviewImage(ctx context.Context, imageID, nextStatus, reviewReason string, publishedAt *time.Time) (domainimagetask.GalleryImage, error) {
	return s.base.ReviewImage(ctx, imageID, nextStatus, reviewReason, publishedAt)
}

func (s *countingStore) DeleteImageResult(ctx context.Context, userID int64, imageID string) (provider.ImageResult, error) {
	return s.base.DeleteImageResult(ctx, userID, imageID)
}

func (s *countingStore) ListGallery(ctx context.Context, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error) {
	return s.base.ListGallery(ctx, req)
}

func (s *countingStore) ListGalleryByUser(ctx context.Context, userID int64, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error) {
	return s.base.ListGalleryByUser(ctx, userID, req)
}

func (s *countingStore) ListPublicGallery(ctx context.Context, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error) {
	return s.base.ListPublicGallery(ctx, req)
}

func (s *countingStore) GetPublicImage(ctx context.Context, imageID string, viewerUserID int64) (domainimagetask.GalleryImage, error) {
	return s.base.GetPublicImage(ctx, imageID, viewerUserID)
}

func (s *countingStore) SetPublicImageInteraction(ctx context.Context, userID int64, imageID, kind string, active bool) (domainimagetask.GalleryImage, error) {
	return s.base.SetPublicImageInteraction(ctx, userID, imageID, kind, active)
}

func (s *countingStore) DeleteByID(ctx context.Context, userID int64, taskID string) error {
	return s.base.DeleteByID(ctx, userID, taskID)
}

func (s *countingStore) AcquireNextQueuedTask(ctx context.Context, owner string, now time.Time, leaseTTL time.Duration) (domainimagetask.Task, error) {
	return s.base.AcquireNextQueuedTask(ctx, owner, now, leaseTTL)
}

func (s *countingStore) RenewTaskLease(ctx context.Context, taskID, owner string, now time.Time, leaseTTL time.Duration) (domainimagetask.Task, error) {
	task, err := s.base.RenewTaskLease(ctx, taskID, owner, now, leaseTTL)
	if err != nil {
		return task, err
	}
	s.mu.Lock()
	s.heartbeatCalls++
	s.mu.Unlock()
	select {
	case s.heartbeatSignal <- struct{}{}:
	default:
	}
	return task, nil
}

func (s *countingStore) heartbeatCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.heartbeatCalls
}

func TestRunnerProcessOnceHeartbeatsLongRunningTask(t *testing.T) {
	cfg := workerTestConfig()
	baseStore := imagetaskservice.NewMemoryStore()
	store := newCountingStore(baseStore)
	gate := make(chan struct{})

	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			select {
			case <-ctx.Done():
				return provider.ImageResponse{}, ctx.Err()
			case <-gate:
				return provider.ImageResponse{Created: 1770000005, Data: []provider.ImageResult{{B64JSON: tinyPNGBase64}}}, nil
			}
		}},
	}
	svc := imagetaskservice.NewServiceWithProvidersAndStore(cfg, providers, store)

	created, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:           82,
		AbstractModel:    "plus",
		TaskType:         string(provider.TaskTypeTextToImage),
		Prompt:           "heartbeat task",
		RequestedSize:    "auto",
		BaseResolution:   "auto",
		OutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	runner := NewRunner(svc, Config{
		Owner:              "worker-heartbeat",
		LeaseTTL:           90 * time.Millisecond,
		HeartbeatInterval:  20 * time.Millisecond,
		PreferredProviders: []string{"openrouter"},
	})

	done := make(chan error, 1)
	go func() {
		_, runErr := runner.ProcessOnce(context.Background())
		done <- runErr
	}()

	select {
	case <-store.heartbeatSignal:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected worker to renew task lease while provider call is still running")
	}
	close(gate)

	if err := <-done; err != nil {
		t.Fatalf("ProcessOnce: %v", err)
	}
	if store.heartbeatCount() == 0 {
		t.Fatal("expected at least one heartbeat call")
	}

	loaded, err := svc.GetByID(context.Background(), 82, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if loaded.Status != domainimagetask.StatusSucceeded {
		t.Fatalf("expected succeeded task, got %s", loaded.Status)
	}
}

func TestRunnerStopsWhenHeartbeatFails(t *testing.T) {
	cfg := workerTestConfig()
	store := imagetaskservice.NewMemoryStore()
	gate := make(chan struct{})

	providers := map[string]provider.ImageProvider{
		"openrouter": fakeProvider{generateFunc: func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
			select {
			case <-ctx.Done():
				return provider.ImageResponse{}, ctx.Err()
			case <-gate:
				return provider.ImageResponse{Created: 1770000006, Data: []provider.ImageResult{{B64JSON: tinyPNGBase64}}}, nil
			}
		}},
	}
	svc := imagetaskservice.NewServiceWithProvidersAndStore(cfg, providers, store)

	created, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:           83,
		AbstractModel:    "plus",
		TaskType:         string(provider.TaskTypeTextToImage),
		Prompt:           "heartbeat conflict",
		RequestedSize:    "auto",
		BaseResolution:   "auto",
		OutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	runner := NewRunner(svc, Config{
		Owner:              "worker-a",
		LeaseTTL:           60 * time.Millisecond,
		HeartbeatInterval:  15 * time.Millisecond,
		PreferredProviders: []string{"openrouter"},
	})

	done := make(chan error, 1)
	go func() {
		_, runErr := runner.ProcessOnce(context.Background())
		done <- runErr
	}()

	time.Sleep(20 * time.Millisecond)
	claimed, err := store.GetByID(context.Background(), 83, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	claimed.Status = domainimagetask.StatusRunning
	claimed.LeaseOwner = "worker-b"
	expiresAt := time.Now().Add(time.Second)
	claimed.LeaseExpiresAt = &expiresAt
	if err := store.Save(context.Background(), claimed); err != nil {
		t.Fatalf("Save: %v", err)
	}

	err = <-done
	if err == nil {
		t.Fatal("expected heartbeat failure to stop processing")
	}
	appErr, ok := err.(*errs.Error)
	if !ok {
		t.Fatalf("expected application error, got %T (%v)", err, err)
	}
	if appErr.Code != errs.CodeConflict {
		t.Fatalf("expected heartbeat conflict error, got %v", err)
	}
	close(gate)
}

type fakeTaskService struct {
	acquireFunc   func(ctx context.Context, owner string, leaseTTL time.Duration) (domainimagetask.Task, bool, error)
	heartbeatFunc func(ctx context.Context, taskID, owner string, leaseTTL time.Duration) (domainimagetask.Task, error)
	executeFunc   func(ctx context.Context, task domainimagetask.Task, owner string, preferredProviders []string) (domainimagetask.ExecuteResult, error)
}

func (f fakeTaskService) AcquireNextTask(ctx context.Context, owner string, leaseTTL time.Duration) (domainimagetask.Task, bool, error) {
	return f.acquireFunc(ctx, owner, leaseTTL)
}

func (f fakeTaskService) HeartbeatTask(ctx context.Context, taskID, owner string, leaseTTL time.Duration) (domainimagetask.Task, error) {
	return f.heartbeatFunc(ctx, taskID, owner, leaseTTL)
}

func (f fakeTaskService) ExecuteLeasedTask(ctx context.Context, task domainimagetask.Task, owner string, preferredProviders []string) (domainimagetask.ExecuteResult, error) {
	return f.executeFunc(ctx, task, owner, preferredProviders)
}

type fakeCompensationService struct {
	calls int
	fn    func(ctx context.Context, limit int) (int, error)
}

func (f *fakeCompensationService) ProcessRefundFinalizeFailures(ctx context.Context, limit int) (int, error) {
	f.calls++
	return f.fn(ctx, limit)
}

func TestProcessOnceTreatsTransientStoreLockAsNoWork(t *testing.T) {
	var acquireCalls atomic.Int64
	task := domainimagetask.Task{ID: "sqlite-lock-retry-task", Status: domainimagetask.StatusRunning}
	runner := NewRunner(fakeTaskService{
		acquireFunc: func(ctx context.Context, owner string, leaseTTL time.Duration) (domainimagetask.Task, bool, error) {
			if acquireCalls.Add(1) == 1 {
				return domainimagetask.Task{}, false, errors.New("database table is locked: image_tasks")
			}
			return task, true, nil
		},
		heartbeatFunc: func(ctx context.Context, taskID, owner string, leaseTTL time.Duration) (domainimagetask.Task, error) {
			return domainimagetask.Task{}, nil
		},
		executeFunc: func(ctx context.Context, claimed domainimagetask.Task, owner string, preferredProviders []string) (domainimagetask.ExecuteResult, error) {
			return domainimagetask.ExecuteResult{
				Task: domainimagetask.Task{ID: claimed.ID, Status: domainimagetask.StatusSucceeded},
			}, nil
		},
	}, Config{Owner: "worker-sqlite-lock", LeaseTTL: 30 * time.Second, HeartbeatInterval: time.Hour})

	processed, err := runner.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("expected transient store lock to be swallowed, got %v", err)
	}
	if processed {
		t.Fatal("expected transient store lock to count as no work for this poll")
	}

	processed, err = runner.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce after transient lock: %v", err)
	}
	if !processed {
		t.Fatal("expected runner to keep processing after transient lock")
	}
}

func TestProcessOnceTreatsTransientCompensationLockAsNoWork(t *testing.T) {
	var acquireCalls atomic.Int64
	task := domainimagetask.Task{ID: "sqlite-compensation-lock-retry-task", Status: domainimagetask.StatusRunning}
	runner := NewRunner(fakeTaskService{
		acquireFunc: func(ctx context.Context, owner string, leaseTTL time.Duration) (domainimagetask.Task, bool, error) {
			acquireCalls.Add(1)
			return task, true, nil
		},
		heartbeatFunc: func(ctx context.Context, taskID, owner string, leaseTTL time.Duration) (domainimagetask.Task, error) {
			return domainimagetask.Task{}, nil
		},
		executeFunc: func(ctx context.Context, claimed domainimagetask.Task, owner string, preferredProviders []string) (domainimagetask.ExecuteResult, error) {
			return domainimagetask.ExecuteResult{
				Task: domainimagetask.Task{ID: claimed.ID, Status: domainimagetask.StatusSucceeded},
			}, nil
		},
	}, Config{Owner: "worker-sqlite-compensation-lock", LeaseTTL: 30 * time.Second, HeartbeatInterval: time.Hour})
	var compensationCalls atomic.Int64
	runner.SetCompensationService(&fakeCompensationService{
		fn: func(ctx context.Context, limit int) (int, error) {
			if compensationCalls.Add(1) == 1 {
				return 0, errors.New("database table is locked: payment_webhook_events")
			}
			return 0, nil
		},
	})

	processed, err := runner.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("expected transient compensation lock to be swallowed, got %v", err)
	}
	if processed {
		t.Fatal("expected transient compensation lock to count as no work for this poll")
	}
	if acquireCalls.Load() != 0 {
		t.Fatalf("expected task acquisition to wait for next poll after compensation lock, got %d calls", acquireCalls.Load())
	}

	processed, err = runner.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce after transient compensation lock: %v", err)
	}
	if !processed {
		t.Fatal("expected runner to keep processing after transient compensation lock")
	}
}

func TestRunnerProcessOnceProcessesRefundCompensationBeforeTasks(t *testing.T) {
	var acquireCalls int
	tasks := fakeTaskService{
		acquireFunc: func(ctx context.Context, owner string, leaseTTL time.Duration) (domainimagetask.Task, bool, error) {
			acquireCalls++
			return domainimagetask.Task{}, false, nil
		},
		heartbeatFunc: func(ctx context.Context, taskID, owner string, leaseTTL time.Duration) (domainimagetask.Task, error) {
			return domainimagetask.Task{}, nil
		},
		executeFunc: func(ctx context.Context, task domainimagetask.Task, owner string, preferredProviders []string) (domainimagetask.ExecuteResult, error) {
			return domainimagetask.ExecuteResult{}, nil
		},
	}
	compensation := &fakeCompensationService{
		fn: func(ctx context.Context, limit int) (int, error) {
			if limit != 3 {
				t.Fatalf("expected compensation batch limit 3, got %d", limit)
			}
			return 1, nil
		},
	}
	runner := NewRunner(tasks, Config{Owner: "worker-compensation", RefundCompensationBatchSize: 3})
	runner.SetCompensationService(compensation)

	processed, err := runner.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce: %v", err)
	}
	if !processed {
		t.Fatal("expected compensation work to count as processed")
	}
	if compensation.calls != 1 {
		t.Fatalf("expected one compensation call, got %d", compensation.calls)
	}
	if acquireCalls != 0 {
		t.Fatalf("expected task acquisition to be skipped when compensation was processed, got %d", acquireCalls)
	}
}

func TestRunnerRunProcessesTasksConcurrently(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tasks := []domainimagetask.Task{
		{ID: "concurrent-1", Status: domainimagetask.StatusRunning},
		{ID: "concurrent-2", Status: domainimagetask.StatusRunning},
		{ID: "concurrent-3", Status: domainimagetask.StatusRunning},
	}
	started := make(chan string, len(tasks))
	release := make(chan struct{})
	var mu sync.Mutex
	maxActive := 0
	active := 0

	runner := NewRunner(fakeTaskService{
		acquireFunc: func(ctx context.Context, owner string, leaseTTL time.Duration) (domainimagetask.Task, bool, error) {
			mu.Lock()
			defer mu.Unlock()
			if len(tasks) == 0 {
				return domainimagetask.Task{}, false, nil
			}
			task := tasks[0]
			tasks = tasks[1:]
			return task, true, nil
		},
		executeFunc: func(ctx context.Context, task domainimagetask.Task, owner string, preferredProviders []string) (domainimagetask.ExecuteResult, error) {
			mu.Lock()
			active++
			if active > maxActive {
				maxActive = active
			}
			mu.Unlock()
			started <- task.ID
			select {
			case <-ctx.Done():
				return domainimagetask.ExecuteResult{}, ctx.Err()
			case <-release:
			}
			mu.Lock()
			active--
			mu.Unlock()
			return domainimagetask.ExecuteResult{Task: domainimagetask.Task{ID: task.ID, Status: domainimagetask.StatusSucceeded}}, nil
		},
		heartbeatFunc: func(ctx context.Context, taskID, owner string, leaseTTL time.Duration) (domainimagetask.Task, error) {
			return domainimagetask.Task{}, nil
		},
	}, Config{Owner: "worker-concurrent", MaxConcurrentTasks: 3, LeaseTTL: time.Second, HeartbeatInterval: time.Hour, PollInterval: time.Millisecond})

	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	for i := 0; i < 3; i++ {
		select {
		case <-started:
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("expected task %d to start concurrently", i+1)
		}
	}

	mu.Lock()
	gotMaxActive := maxActive
	mu.Unlock()
	if gotMaxActive != 3 {
		t.Fatalf("expected three active tasks, got %d", gotMaxActive)
	}
	close(release)
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run: %v", err)
	}
}

func TestProcessClaimedTaskTreatsTerminalHeartbeatRaceAsSuccess(t *testing.T) {
	heartbeatStarted := make(chan struct{})
	executeFinished := make(chan struct{})
	allowOutcomePublish := make(chan struct{})
	executeReturned := make(chan struct{})
	task := domainimagetask.Task{ID: "terminal-race-task", Status: domainimagetask.StatusRunning}

	runner := NewRunner(fakeTaskService{
		acquireFunc: func(ctx context.Context, owner string, leaseTTL time.Duration) (domainimagetask.Task, bool, error) {
			return task, true, nil
		},
		executeFunc: func(ctx context.Context, claimed domainimagetask.Task, owner string, preferredProviders []string) (domainimagetask.ExecuteResult, error) {
			<-heartbeatStarted
			close(executeFinished)
			<-allowOutcomePublish
			close(executeReturned)
			return domainimagetask.ExecuteResult{
				Task: domainimagetask.Task{ID: claimed.ID, Status: domainimagetask.StatusSucceeded},
			}, nil
		},
		heartbeatFunc: func(ctx context.Context, taskID, owner string, leaseTTL time.Duration) (domainimagetask.Task, error) {
			close(heartbeatStarted)
			<-executeFinished
			close(allowOutcomePublish)
			<-executeReturned
			return domainimagetask.Task{}, errs.New(409, errs.CodeConflict, "task already completed")
		},
	}, Config{Owner: "worker-race", LeaseTTL: 30 * time.Second})

	heartbeatC := make(chan time.Time, 1)
	heartbeatC <- time.Now()
	if err := runner.processClaimedTask(context.Background(), task, heartbeatC); err != nil {
		t.Fatalf("expected terminal heartbeat race to be treated as success, got %v", err)
	}
}

func TestRunnerProcessOnceSwallowsPersistedBusinessFailureButReturnsExecutorFailure(t *testing.T) {
	task := domainimagetask.Task{ID: "failure-task", Status: domainimagetask.StatusRunning}
	businessFailureRunner := NewRunner(fakeTaskService{
		acquireFunc: func(ctx context.Context, owner string, leaseTTL time.Duration) (domainimagetask.Task, bool, error) {
			return task, true, nil
		},
		executeFunc: func(ctx context.Context, claimed domainimagetask.Task, owner string, preferredProviders []string) (domainimagetask.ExecuteResult, error) {
			return domainimagetask.ExecuteResult{
				Task: domainimagetask.Task{ID: claimed.ID, Status: domainimagetask.StatusFailed},
			}, errors.New("upstream rejected prompt")
		},
		heartbeatFunc: func(ctx context.Context, taskID, owner string, leaseTTL time.Duration) (domainimagetask.Task, error) {
			return domainimagetask.Task{}, nil
		},
	}, Config{Owner: "worker-business", LeaseTTL: 30 * time.Second, HeartbeatInterval: time.Hour})

	processed, err := businessFailureRunner.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("expected persisted business failure to be swallowed, got %v", err)
	}
	if !processed {
		t.Fatal("expected claimed task to count as processed")
	}

	executorFailureRunner := NewRunner(fakeTaskService{
		acquireFunc: func(ctx context.Context, owner string, leaseTTL time.Duration) (domainimagetask.Task, bool, error) {
			return task, true, nil
		},
		executeFunc: func(ctx context.Context, claimed domainimagetask.Task, owner string, preferredProviders []string) (domainimagetask.ExecuteResult, error) {
			return domainimagetask.ExecuteResult{}, errs.Internal("persist failed")
		},
		heartbeatFunc: func(ctx context.Context, taskID, owner string, leaseTTL time.Duration) (domainimagetask.Task, error) {
			return domainimagetask.Task{}, nil
		},
	}, Config{Owner: "worker-executor", LeaseTTL: 30 * time.Second, HeartbeatInterval: time.Hour})

	processed, err = executorFailureRunner.ProcessOnce(context.Background())
	if !processed {
		t.Fatal("expected claimed task to count as processed")
	}
	if err == nil {
		t.Fatal("expected executor failure to be returned")
	}
}

func TestProcessClaimedTaskPreservesHeartbeatInfrastructureFailure(t *testing.T) {
	heartbeatStarted := make(chan struct{})
	task := domainimagetask.Task{ID: "heartbeat-failure-task", Status: domainimagetask.StatusRunning}
	heartbeatErr := errs.Internal("heartbeat storage unavailable")

	runner := NewRunner(fakeTaskService{
		acquireFunc: func(ctx context.Context, owner string, leaseTTL time.Duration) (domainimagetask.Task, bool, error) {
			return task, true, nil
		},
		executeFunc: func(ctx context.Context, claimed domainimagetask.Task, owner string, preferredProviders []string) (domainimagetask.ExecuteResult, error) {
			<-heartbeatStarted
			<-ctx.Done()
			return domainimagetask.ExecuteResult{
				Task: domainimagetask.Task{ID: claimed.ID, Status: domainimagetask.StatusFailed},
			}, ctx.Err()
		},
		heartbeatFunc: func(ctx context.Context, taskID, owner string, leaseTTL time.Duration) (domainimagetask.Task, error) {
			close(heartbeatStarted)
			return domainimagetask.Task{}, heartbeatErr
		},
	}, Config{Owner: "worker-heartbeat-failure", LeaseTTL: 30 * time.Second})

	heartbeatC := make(chan time.Time, 1)
	heartbeatC <- time.Now()
	err := runner.processClaimedTask(context.Background(), task, heartbeatC)
	if err == nil {
		t.Fatal("expected heartbeat infrastructure failure to be returned")
	}
	if !errors.Is(err, heartbeatErr) {
		t.Fatalf("expected heartbeat error %v, got %v", heartbeatErr, err)
	}
}

func workerTestConfig() config.Config {
	cfg := config.Config{}
	cfg.Billing.AutoBaseResolutionDefaultByGroup = map[string]string{"plus": "2k"}
	cfg.Billing.BaseResolutionPointsByModel = map[string]map[string]string{
		"plus": {"1k": "5.00000", "2k": "8.00000", "4k": "16.00000"},
	}
	cfg.GenerationLimits.MaxImageCount = 5
	cfg.GenerationLimits.ReferenceImageMaxCount = 4
	cfg.Providers.OpenRouter.Enabled = true
	cfg.Routing.ProviderCapabilities = map[string]config.ProviderCapabilityConfig{
		"openrouter": {
			SupportedModels:         []string{"plus"},
			SupportedTaskTypes:      []string{"text_to_image", "image_edit"},
			SupportedBaseResolution: []string{"1k", "2k", "4k"},
			SupportedAspectRatios:   []string{"1:1", "4:3", "16:9"},
			MaxImageCount:           5,
			MaxReferenceImageCount:  4,
			SupportsImageInput:      true,
			SupportsMask:            false,
			Priority:                1,
		},
	}
	cfg.Routing.ProviderModelMap = map[string]map[string]string{
		"plus": {"openrouter": "openrouter/vision"},
	}
	return cfg
}
