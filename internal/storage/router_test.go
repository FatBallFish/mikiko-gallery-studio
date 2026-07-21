package storage

import (
	"context"
	"errors"
	"sync"
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

type blockingProbeCleanupBackend struct {
	get           []byte
	getErr        error
	deleteStarted chan struct{}
	releaseDelete chan struct{}
	deleteOnce    sync.Once
}

func (b *blockingProbeCleanupBackend) Driver() string { return "test" }

func (b *blockingProbeCleanupBackend) Put(context.Context, string, string, []byte) error { return nil }

func (b *blockingProbeCleanupBackend) Get(context.Context, string) ([]byte, error) {
	return b.get, b.getErr
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
