package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type fakeProvider struct {
	generateFunc func(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error)
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
			return provider.ImageResponse{Created: 1770000004, Data: []provider.ImageResult{{URL: "https://cdn.example.com/worker.png"}}}, nil
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
		RequestedQuality: "auto",
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

func (s *countingStore) SaveTerminalState(ctx context.Context, task domainimagetask.Task, owner string, now time.Time) error {
	return s.base.SaveTerminalState(ctx, task, owner, now)
}

func (s *countingStore) GetByID(ctx context.Context, userID int64, taskID string) (domainimagetask.Task, error) {
	return s.base.GetByID(ctx, userID, taskID)
}

func (s *countingStore) ListByUser(ctx context.Context, userID int64) ([]domainimagetask.Task, error) {
	return s.base.ListByUser(ctx, userID)
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
				return provider.ImageResponse{Created: 1770000005, Data: []provider.ImageResult{{URL: "https://cdn.example.com/heartbeat.png"}}}, nil
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
		RequestedQuality: "auto",
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
				return provider.ImageResponse{Created: 1770000006, Data: []provider.ImageResult{{URL: "https://cdn.example.com/never.png"}}}, nil
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
		RequestedQuality: "auto",
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
	cfg.Billing.AutoQualityDefaultByGroup = map[string]string{"plus": "2k"}
	cfg.Billing.QualityPointsByModel = map[string]map[string]string{
		"plus": {"1k": "5.00000", "2k": "8.00000", "4k": "16.00000"},
	}
	cfg.GenerationLimits.MaxImageCount = 5
	cfg.GenerationLimits.ReferenceImageMaxCount = 4
	cfg.Providers.OpenRouter.Enabled = true
	cfg.Routing.ProviderCapabilities = map[string]config.ProviderCapabilityConfig{
		"openrouter": {
			SupportedModels:        []string{"plus"},
			SupportedTaskTypes:     []string{"text_to_image", "image_edit", "reference_generate"},
			SupportedQualities:     []string{"1k", "2k", "4k"},
			SupportedAspectRatios:  []string{"1:1", "4:3", "16:9"},
			MaxImageCount:          5,
			MaxReferenceImageCount: 4,
			SupportsImageInput:     true,
			SupportsMask:           false,
			Priority:               1,
		},
	}
	cfg.Routing.ProviderModelMap = map[string]map[string]string{
		"plus": {"openrouter": "openrouter/vision"},
	}
	return cfg
}
