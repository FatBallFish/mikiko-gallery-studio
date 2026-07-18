package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/fatballfish/pic-gallery/internal/storage"
)

type storageInvalidationSource interface {
	Subscribe(context.Context, func(storage.StorageInvalidation)) error
}

type storageInvalidationSubscriber struct {
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

func startStorageInvalidationSubscriber(parent context.Context, source storageInvalidationSource, handler func(storage.StorageInvalidation), minBackoff, maxBackoff time.Duration) *storageInvalidationSubscriber {
	ctx, cancel := context.WithCancel(parent)
	subscriber := &storageInvalidationSubscriber{cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(subscriber.done)
		runStorageInvalidationSubscriber(ctx, source, handler, minBackoff, maxBackoff)
	}()
	return subscriber
}

func (s *storageInvalidationSubscriber) Stop() {
	if s == nil {
		return
	}
	s.once.Do(s.cancel)
	<-s.done
}

func runStorageInvalidationSubscriber(ctx context.Context, source storageInvalidationSource, handler func(storage.StorageInvalidation), minBackoff, maxBackoff time.Duration) {
	if source == nil {
		return
	}
	if minBackoff <= 0 {
		minBackoff = time.Second
	}
	if maxBackoff < minBackoff {
		maxBackoff = 30 * time.Second
	}
	backoff := minBackoff
	for ctx.Err() == nil {
		err := subscribeStorageInvalidations(ctx, source, handler)
		if ctx.Err() != nil {
			return
		}
		slog.WarnContext(ctx, "storage invalidation subscriber stopped; retrying", "backoff", backoff, "error", err)
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func subscribeStorageInvalidations(ctx context.Context, source storageInvalidationSource, handler func(storage.StorageInvalidation)) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("storage invalidation subscriber panic: %v", recovered)
		}
	}()
	return source.Subscribe(ctx, handler)
}
