package storage

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainstorageconfig "github.com/fatballfish/pic-gallery/internal/domain/storageconfig"
)

func TestRegistryInvalidationSynchronizesDefaultAcrossInstances(t *testing.T) {
	source := &mutableConfigSource{records: map[string]domainstorageconfig.ResolvedConfig{}}
	source.setDefault(localResolved("one", 1, t.TempDir()))
	first := NewRegistry(source, time.Hour)
	second := NewRegistry(source, time.Hour)

	firstBefore, err := first.DefaultWriter(context.Background())
	if err != nil {
		t.Fatalf("first default: %v", err)
	}
	secondBefore, err := second.DefaultWriter(context.Background())
	if err != nil {
		t.Fatalf("second default: %v", err)
	}
	if firstBefore.ConfigID != "one" || secondBefore.ConfigID != "one" {
		t.Fatalf("expected initial config one, got first=%q second=%q", firstBefore.ConfigID, secondBefore.ConfigID)
	}

	source.setDefault(localResolved("two", 1, t.TempDir()))
	first.Invalidate(StorageInvalidation{ConfigID: "two", Version: 1, DefaultChanged: true})
	second.Invalidate(StorageInvalidation{ConfigID: "two", Version: 1, DefaultChanged: true})

	firstAfter, err := first.DefaultWriter(context.Background())
	if err != nil {
		t.Fatalf("first refreshed default: %v", err)
	}
	secondAfter, err := second.DefaultWriter(context.Background())
	if err != nil {
		t.Fatalf("second refreshed default: %v", err)
	}
	if firstAfter.ConfigID != "two" || secondAfter.ConfigID != "two" {
		t.Fatalf("expected invalidated config two, got first=%q second=%q", firstAfter.ConfigID, secondAfter.ConfigID)
	}
}

func TestRegistryEnumeratesEnabledAndDisabledReadableBackends(t *testing.T) {
	source := &mutableConfigSource{defaultID: "current", records: map[string]domainstorageconfig.ResolvedConfig{
		"current": localResolved("current", 1, t.TempDir()),
		"history": localResolved("history", 3, t.TempDir()),
	}}
	history := source.records["history"]
	history.Status, history.WriteEnabled, history.IsDefault = domainstorageconfig.StatusDisabled, false, false
	source.records["history"] = history
	registry := NewRegistry(source, time.Hour)

	refs, err := registry.ReadableBackends(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]BackendRef{}
	for _, ref := range refs {
		seen[ref.ConfigID] = ref
	}
	if len(seen) != 2 || seen["current"].Namespace == "" || seen["history"].Namespace == "" {
		t.Fatalf("readable backends=%#v", refs)
	}
}

func TestRegistryTTLConvergesWithoutInvalidation(t *testing.T) {
	now := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	source := &mutableConfigSource{records: map[string]domainstorageconfig.ResolvedConfig{}}
	source.setDefault(localResolved("one", 1, t.TempDir()))
	registry := NewRegistry(source, time.Minute)
	registry.now = func() time.Time { return now }

	if ref, err := registry.DefaultWriter(context.Background()); err != nil || ref.ConfigID != "one" {
		t.Fatalf("initial default ref=%#v err=%v", ref, err)
	}
	source.setDefault(localResolved("two", 1, t.TempDir()))
	if ref, err := registry.DefaultWriter(context.Background()); err != nil || ref.ConfigID != "one" {
		t.Fatalf("default should remain cached before TTL, ref=%#v err=%v", ref, err)
	}
	now = now.Add(time.Minute + time.Second)
	if ref, err := registry.DefaultWriter(context.Background()); err != nil || ref.ConfigID != "two" {
		t.Fatalf("default should converge after TTL, ref=%#v err=%v", ref, err)
	}
}

func TestRegistryRoutesHistoricalResourceByStorageConfigID(t *testing.T) {
	source := &mutableConfigSource{records: map[string]domainstorageconfig.ResolvedConfig{}}
	old := localResolved("old", 2, t.TempDir())
	current := localResolved("current", 4, t.TempDir())
	source.records[old.ID] = old
	source.setDefault(current)
	registry := NewRegistry(source, time.Minute)

	ref, err := registry.BackendFor(context.Background(), "old", "local")
	if err != nil {
		t.Fatalf("historical backend: %v", err)
	}
	if ref.ConfigID != "old" || ref.Version != 2 {
		t.Fatalf("expected historical old v2, got %#v", ref)
	}
}

func TestRegistryProbeFailureCleanupRemainsBounded(t *testing.T) {
	testCases := []struct {
		name   string
		get    []byte
		getErr error
	}{
		{name: "get failure", getErr: errors.New("get failed")},
		{name: "content mismatch", get: []byte("wrong probe content")},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			backend := &blockingProbeCleanupBackend{
				get: testCase.get, getErr: testCase.getErr,
				deleteStarted: make(chan struct{}), releaseDelete: make(chan struct{}),
			}
			registry := NewRegistry(nil, time.Minute)
			registry.newBackend = func(config.StorageConfig) (Backend, error) { return backend, nil }
			registry.probeCleanupTimeout = 30 * time.Millisecond
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			result := make(chan domainstorageconfig.ProbeResult, 1)
			go func() {
				result <- registry.Probe(ctx, domainstorageconfig.ResolvedConfig{})
			}()
			select {
			case <-backend.deleteStarted:
			case <-time.After(200 * time.Millisecond):
				close(backend.releaseDelete)
				<-result
				t.Fatal("probe did not attempt best-effort cleanup")
			}
			select {
			case probeResult := <-result:
				if probeResult.Status != domainstorageconfig.ProbeStatusFailed {
					t.Fatalf("probe result=%#v, want failed", probeResult)
				}
			case <-time.After(200 * time.Millisecond):
				close(backend.releaseDelete)
				<-result
				t.Fatal("probe cleanup ignored the bounded probe context")
			}
		})
	}
}

func TestRegistryProbeRequiresBoundedReadAndNeverCallsGet(t *testing.T) {
	t.Run("unsupported backend fails closed", func(t *testing.T) {
		backend := &unboundedRegistryProbeBackend{}
		registry := NewRegistry(nil, time.Minute)
		registry.newBackend = func(config.StorageConfig) (Backend, error) { return backend, nil }
		result := registry.Probe(context.Background(), domainstorageconfig.ResolvedConfig{})
		if result.Status != domainstorageconfig.ProbeStatusFailed || backend.getCalls != 0 || backend.deleteCalls != 1 {
			t.Fatalf("unbounded backend probe=%#v backend=%#v", result, backend)
		}
	})
	t.Run("bounded backend uses fixed maximum", func(t *testing.T) {
		backend := &boundedRegistryProbeBackend{}
		registry := NewRegistry(nil, time.Minute)
		registry.newBackend = func(config.StorageConfig) (Backend, error) { return backend, nil }
		result := registry.Probe(context.Background(), domainstorageconfig.ResolvedConfig{})
		if result.Status != domainstorageconfig.ProbeStatusSuccess || backend.getCalls != 0 || backend.boundedGetCalls != 1 || backend.boundedGetMax != int64(len("pic-gallery-storage-probe")) {
			t.Fatalf("bounded backend probe=%#v backend=%#v", result, backend)
		}
	})
	t.Run("oversized bounded response fails and cleans", func(t *testing.T) {
		backend := &boundedRegistryProbeBackend{boundedResult: make([]byte, 1<<20)}
		registry := NewRegistry(nil, time.Minute)
		registry.newBackend = func(config.StorageConfig) (Backend, error) { return backend, nil }
		result := registry.Probe(context.Background(), domainstorageconfig.ResolvedConfig{})
		if result.Status != domainstorageconfig.ProbeStatusFailed || backend.getCalls != 0 || backend.boundedGetMax != int64(len("pic-gallery-storage-probe")) || backend.deleteCalls != 1 {
			t.Fatalf("oversized bounded response probe=%#v backend=%#v", result, backend)
		}
	})
}

func TestRegistryProbeCleansCommittedPutErrorWithIndependentContext(t *testing.T) {
	backend := newLifecycleProbeBackend()
	backend.putErr = errors.New("provider committed object before reporting failure")
	result := probeWithLifecycleBackend(context.WithValue(context.Background(), probeContextMarker{}, true), backend)
	state := backend.snapshot()
	if result.Status != domainstorageconfig.ProbeStatusFailed || state.objectExists || state.deleteCalls != 1 {
		t.Fatalf("committed Put failure probe=%#v backend=%#v", result, state)
	}
	if state.deleteContextErr != nil || state.deleteInheritedMarker {
		t.Fatalf("committed Put cleanup reused probe context: %#v", state)
	}
}

func TestRegistryProbeCleansExpiredReadAndMismatchWithIndependentContext(t *testing.T) {
	testCases := []struct {
		name           string
		waitForContext bool
		loaded         []byte
	}{
		{name: "expired read", waitForContext: true},
		{name: "content mismatch", loaded: []byte("wrong")},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			backend := newLifecycleProbeBackend()
			backend.waitForReadContext = testCase.waitForContext
			backend.loaded = testCase.loaded
			ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), probeContextMarker{}, true), 20*time.Millisecond)
			defer cancel()
			result := probeWithLifecycleBackend(ctx, backend)
			deadline := time.Now().Add(time.Second)
			state := backend.snapshot()
			for state.deleteCalls == 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
				state = backend.snapshot()
			}
			if result.Status != domainstorageconfig.ProbeStatusFailed || state.objectExists || state.deleteCalls != 1 {
				t.Fatalf("%s probe=%#v backend=%#v", testCase.name, result, state)
			}
			if state.deleteContextErr != nil || state.deleteInheritedMarker {
				t.Fatalf("%s cleanup reused probe context: %#v", testCase.name, state)
			}
		})
	}
}

func TestRegistryProbeCleanupFailureAndNotFoundSemantics(t *testing.T) {
	t.Run("cleanup failure keeps probe failed", func(t *testing.T) {
		backend := newLifecycleProbeBackend()
		backend.deleteErr = errors.New("delete failed")
		backend.deleteCommitsBeforeError = true
		result := probeWithLifecycleBackend(context.Background(), backend)
		state := backend.snapshot()
		if result.Status != domainstorageconfig.ProbeStatusFailed || state.objectExists || state.deleteCalls == 0 {
			t.Fatalf("cleanup failure probe=%#v backend=%#v", result, state)
		}
	})
	t.Run("not found is successful cleanup", func(t *testing.T) {
		backend := newLifecycleProbeBackend()
		backend.deleteErr = ErrNotFound
		backend.deleteCommitsBeforeError = true
		result := probeWithLifecycleBackend(context.Background(), backend)
		state := backend.snapshot()
		if result.Status != domainstorageconfig.ProbeStatusSuccess || state.objectExists || state.deleteCalls != 1 {
			t.Fatalf("not-found cleanup probe=%#v backend=%#v", result, state)
		}
	})
}

func TestRegistryProbeHardDeadlineAndLateCleanupForBlockedOperations(t *testing.T) {
	for _, stage := range []string{"put", "get", "delete"} {
		t.Run(stage, func(t *testing.T) {
			backend := newBlockedStageProbeBackend(stage)
			registry := NewRegistry(nil, time.Minute)
			registry.newBackend = func(config.StorageConfig) (Backend, error) { return backend, nil }
			registry.probeTimeout = 25 * time.Millisecond
			registry.probeCleanupTimeout = 25 * time.Millisecond
			registry.probeSlots = make(chan struct{}, 1)
			resultChannel := make(chan domainstorageconfig.ProbeResult, 1)
			go func() { resultChannel <- registry.Probe(context.Background(), domainstorageconfig.ResolvedConfig{}) }()
			select {
			case <-backend.started:
			case <-time.After(time.Second):
				t.Fatal("blocked probe operation did not start")
			}
			var result domainstorageconfig.ProbeResult
			select {
			case result = <-resultChannel:
			case <-time.After(150 * time.Millisecond):
				close(backend.release)
				<-resultChannel
				t.Fatal("blocked storage operation held the Probe caller past its deadline")
			}
			if result.Status != domainstorageconfig.ProbeStatusFailed {
				t.Fatalf("blocked %s probe result=%#v, want failed", stage, result)
			}
			close(backend.release)
			select {
			case <-backend.cleaned:
			case <-time.After(time.Second):
				t.Fatal("late storage operation did not finish cleanup")
			}
			deadline := time.Now().Add(time.Second)
			for len(registry.probeSlots) != 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if len(registry.probeSlots) != 0 {
				t.Fatal("late storage operation did not release its runner slot")
			}
		})
	}
}

func TestRegistryProbeRunnerBoundsConcurrencyAndSkipsCancelledCalls(t *testing.T) {
	release := make(chan struct{})
	var starts atomic.Int32
	registry := NewRegistry(nil, time.Minute)
	registry.probeTimeout = 30 * time.Millisecond
	registry.probeCleanupTimeout = 20 * time.Millisecond
	registry.probeSlots = make(chan struct{}, 2)
	registry.newBackend = func(config.StorageConfig) (Backend, error) {
		return &slotBlockingProbeBackend{starts: &starts, release: release}, nil
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if result := registry.Probe(cancelled, domainstorageconfig.ResolvedConfig{}); result.Status != domainstorageconfig.ProbeStatusFailed || starts.Load() != 0 {
		t.Fatalf("canceled probe result=%#v starts=%d", result, starts.Load())
	}
	const callers = 24
	results := make(chan domainstorageconfig.ProbeResult, callers)
	for range callers {
		go func() { results <- registry.Probe(context.Background(), domainstorageconfig.ResolvedConfig{}) }()
	}
	for range callers {
		if result := <-results; result.Status != domainstorageconfig.ProbeStatusFailed {
			t.Errorf("bounded runner returned %#v", result)
		}
	}
	if starts.Load() != 2 {
		t.Fatalf("storage runner started %d blocking operations, want 2", starts.Load())
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for len(registry.probeSlots) != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(registry.probeSlots) != 0 {
		t.Fatal("released storage runners did not return their slots")
	}
}

func TestRegistryProbeRunnerRecoversBackendPanic(t *testing.T) {
	registry := NewRegistry(nil, time.Minute)
	registry.probeTimeout = time.Second
	registry.probeSlots = make(chan struct{}, 1)
	registry.newBackend = func(config.StorageConfig) (Backend, error) { return panicProbeBackend{}, nil }
	type outcome struct {
		result    domainstorageconfig.ProbeResult
		recovered any
	}
	outcomes := make(chan outcome, 1)
	go func() {
		completed := outcome{}
		defer func() {
			completed.recovered = recover()
			outcomes <- completed
		}()
		completed.result = registry.Probe(context.Background(), domainstorageconfig.ResolvedConfig{})
	}()
	completed := <-outcomes
	if completed.recovered != nil || completed.result.Status != domainstorageconfig.ProbeStatusFailed {
		t.Fatalf("probe panic escaped or succeeded: recovered=%v result=%#v", completed.recovered, completed.result)
	}
}

type probeContextMarker struct{}

func probeWithLifecycleBackend(ctx context.Context, backend Backend) domainstorageconfig.ProbeResult {
	registry := NewRegistry(nil, time.Minute)
	registry.newBackend = func(config.StorageConfig) (Backend, error) { return backend, nil }
	return registry.Probe(ctx, domainstorageconfig.ResolvedConfig{})
}

type blockingProbeCleanupBackend struct {
	get           []byte
	getErr        error
	deleteStarted chan struct{}
	releaseDelete chan struct{}
	deleteOnce    sync.Once
}

type unboundedRegistryProbeBackend struct {
	getCalls    int
	deleteCalls int
}

func (*unboundedRegistryProbeBackend) Driver() string { return "test-unbounded" }
func (*unboundedRegistryProbeBackend) Put(context.Context, string, string, []byte) error {
	return nil
}
func (b *unboundedRegistryProbeBackend) Get(context.Context, string) ([]byte, error) {
	b.getCalls++
	return []byte("pic-gallery-storage-probe"), nil
}
func (b *unboundedRegistryProbeBackend) Delete(context.Context, string) error {
	b.deleteCalls++
	return nil
}

type boundedRegistryProbeBackend struct {
	unboundedRegistryProbeBackend
	boundedGetCalls int
	boundedGetMax   int64
	boundedResult   []byte
}

func (b *boundedRegistryProbeBackend) GetBounded(_ context.Context, _ string, maxBytes int64) ([]byte, error) {
	b.boundedGetCalls++
	b.boundedGetMax = maxBytes
	if b.boundedResult != nil {
		return b.boundedResult, nil
	}
	return []byte("pic-gallery-storage-probe"), nil
}

type lifecycleProbeBackend struct {
	mu                       sync.Mutex
	objectExists             bool
	putErr                   error
	waitForReadContext       bool
	loaded                   []byte
	deleteErr                error
	deleteCommitsBeforeError bool
	deleteCalls              int
	deleteContextErr         error
	deleteInheritedMarker    bool
}

type lifecycleProbeSnapshot struct {
	objectExists             bool
	putErr                   error
	waitForReadContext       bool
	loaded                   []byte
	deleteErr                error
	deleteCommitsBeforeError bool
	deleteCalls              int
	deleteContextErr         error
	deleteInheritedMarker    bool
}

func newLifecycleProbeBackend() *lifecycleProbeBackend {
	return &lifecycleProbeBackend{loaded: []byte("pic-gallery-storage-probe")}
}

func (*lifecycleProbeBackend) Driver() string { return "test-lifecycle" }

func (b *lifecycleProbeBackend) Put(context.Context, string, string, []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.objectExists = true
	return b.putErr
}

func (*lifecycleProbeBackend) Get(context.Context, string) ([]byte, error) {
	return nil, errors.New("unbounded Get must not be called")
}

func (b *lifecycleProbeBackend) GetBounded(ctx context.Context, _ string, _ int64) ([]byte, error) {
	if b.waitForReadContext {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.loaded...), nil
}

func (b *lifecycleProbeBackend) Delete(ctx context.Context, _ string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.deleteCalls++
	b.deleteContextErr = ctx.Err()
	_, b.deleteInheritedMarker = ctx.Value(probeContextMarker{}).(bool)
	if b.deleteErr == nil || b.deleteCommitsBeforeError {
		b.objectExists = false
	}
	return b.deleteErr
}

func (b *lifecycleProbeBackend) snapshot() lifecycleProbeSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	return lifecycleProbeSnapshot{
		objectExists: b.objectExists, putErr: b.putErr, waitForReadContext: b.waitForReadContext,
		loaded: append([]byte(nil), b.loaded...), deleteErr: b.deleteErr,
		deleteCommitsBeforeError: b.deleteCommitsBeforeError, deleteCalls: b.deleteCalls,
		deleteContextErr: b.deleteContextErr, deleteInheritedMarker: b.deleteInheritedMarker,
	}
}

type blockedStageProbeBackend struct {
	stage       string
	started     chan struct{}
	release     chan struct{}
	cleaned     chan struct{}
	startOnce   sync.Once
	cleanupOnce sync.Once
}

func newBlockedStageProbeBackend(stage string) *blockedStageProbeBackend {
	return &blockedStageProbeBackend{stage: stage, started: make(chan struct{}), release: make(chan struct{}), cleaned: make(chan struct{})}
}

func (*blockedStageProbeBackend) Driver() string { return "test-blocked" }

func (b *blockedStageProbeBackend) block(stage string) {
	if b.stage != stage {
		return
	}
	b.startOnce.Do(func() { close(b.started) })
	<-b.release
}

func (b *blockedStageProbeBackend) Put(context.Context, string, string, []byte) error {
	b.block("put")
	return nil
}
func (*blockedStageProbeBackend) Get(context.Context, string) ([]byte, error) {
	return nil, errors.New("unbounded Get must not be called")
}
func (b *blockedStageProbeBackend) GetBounded(context.Context, string, int64) ([]byte, error) {
	b.block("get")
	return []byte("pic-gallery-storage-probe"), nil
}
func (b *blockedStageProbeBackend) Delete(context.Context, string) error {
	b.block("delete")
	b.cleanupOnce.Do(func() { close(b.cleaned) })
	return nil
}

type slotBlockingProbeBackend struct {
	starts  *atomic.Int32
	release <-chan struct{}
}

func (*slotBlockingProbeBackend) Driver() string { return "test-slot" }
func (b *slotBlockingProbeBackend) Put(context.Context, string, string, []byte) error {
	b.starts.Add(1)
	<-b.release
	return nil
}
func (*slotBlockingProbeBackend) Get(context.Context, string) ([]byte, error) {
	return nil, errors.New("unbounded Get must not be called")
}
func (*slotBlockingProbeBackend) GetBounded(context.Context, string, int64) ([]byte, error) {
	return []byte("pic-gallery-storage-probe"), nil
}
func (*slotBlockingProbeBackend) Delete(context.Context, string) error { return nil }

type panicProbeBackend struct{}

func (panicProbeBackend) Driver() string { return "test-panic" }
func (panicProbeBackend) Put(context.Context, string, string, []byte) error {
	panic("storage probe panic marker")
}
func (panicProbeBackend) Get(context.Context, string) ([]byte, error) { return nil, nil }
func (panicProbeBackend) Delete(context.Context, string) error        { return nil }

func (b *blockingProbeCleanupBackend) Driver() string { return "test" }

func (b *blockingProbeCleanupBackend) Put(context.Context, string, string, []byte) error { return nil }

func (b *blockingProbeCleanupBackend) Get(context.Context, string) ([]byte, error) {
	return b.get, b.getErr
}

func (b *blockingProbeCleanupBackend) GetBounded(ctx context.Context, key string, _ int64) ([]byte, error) {
	return b.Get(ctx, key)
}

func (b *blockingProbeCleanupBackend) Delete(ctx context.Context, _ string) error {
	b.deleteOnce.Do(func() { close(b.deleteStarted) })
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-b.releaseDelete:
		return errors.New("cleanup released by test")
	}
}

func localResolved(id string, version int64, root string) domainstorageconfig.ResolvedConfig {
	return domainstorageconfig.ResolvedConfig{ConfigRecord: domainstorageconfig.ConfigRecord{
		ID: id, Code: id, Name: id, Driver: "local", Provider: "local", Status: "enabled",
		ReadEnabled: true, WriteEnabled: true, IsDefault: true, LocalRoot: root, Version: version,
	}}
}

type mutableConfigSource struct {
	mu        sync.Mutex
	defaultID string
	records   map[string]domainstorageconfig.ResolvedConfig
}

func (s *mutableConfigSource) setDefault(config domainstorageconfig.ResolvedConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, item := range s.records {
		item.IsDefault = false
		s.records[id] = item
	}
	config.IsDefault = true
	s.records[config.ID] = config
	s.defaultID = config.ID
}

func (s *mutableConfigSource) ResolveDefaultWritable(context.Context) (domainstorageconfig.ResolvedConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.records[s.defaultID], nil
}

func (s *mutableConfigSource) ResolveByID(_ context.Context, id string) (domainstorageconfig.ResolvedConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.records[id], nil
}

func (s *mutableConfigSource) ResolveLegacyByDriver(context.Context, string) (domainstorageconfig.ResolvedConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.records[s.defaultID], nil
}

func (s *mutableConfigSource) ListReadableConfigIDs(context.Context) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.records))
	for id, record := range s.records {
		if record.Status != domainstorageconfig.StatusDeleted && record.ReadEnabled {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
