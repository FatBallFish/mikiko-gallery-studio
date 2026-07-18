package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/storage"
)

func TestStorageInvalidationSubscriberRetriesRecoversPanicAndStops(t *testing.T) {
	source := &scriptedInvalidationSource{}
	var (
		mu     sync.Mutex
		events []storage.StorageInvalidation
	)
	subscriber := startStorageInvalidationSubscriber(context.Background(), source, func(event storage.StorageInvalidation) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	}, time.Millisecond, 2*time.Millisecond)

	deadline := time.Now().Add(time.Second)
	for source.callCount() < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if source.callCount() < 3 {
		t.Fatalf("subscriber did not retry after error and panic: calls=%d", source.callCount())
	}

	done := make(chan struct{})
	go func() {
		subscriber.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("subscriber Stop did not wait for a clean exit")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 1 || events[0].ConfigID != "cfg-1" {
		t.Fatalf("unexpected events: %#v", events)
	}
}

type scriptedInvalidationSource struct {
	mu    sync.Mutex
	calls int
}

func (s *scriptedInvalidationSource) Subscribe(ctx context.Context, handler func(storage.StorageInvalidation)) error {
	s.mu.Lock()
	s.calls++
	call := s.calls
	s.mu.Unlock()
	switch call {
	case 1:
		return errors.New("temporary subscribe failure")
	case 2:
		panic("temporary subscriber panic")
	default:
		handler(storage.StorageInvalidation{ConfigID: "cfg-1", Version: 2})
		<-ctx.Done()
		return ctx.Err()
	}
}

func (s *scriptedInvalidationSource) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}
