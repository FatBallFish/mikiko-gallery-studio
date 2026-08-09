package app

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type objectCleanupProcessor interface {
	ProcessOnce(context.Context) (bool, error)
	Reconcile(context.Context, int) (int, error)
}

type objectCleanupHandle struct {
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

func startObjectCleanupLoop(parent context.Context, processor objectCleanupProcessor, pollInterval, reconcileInterval time.Duration) *objectCleanupHandle {
	ctx, cancel := context.WithCancel(parent)
	handle := &objectCleanupHandle{cancel: cancel, done: make(chan struct{})}
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	if reconcileInterval <= 0 {
		reconcileInterval = 6 * time.Hour
	}
	go func() {
		defer close(handle.done)
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("object cleanup loop recovered", "error_code", "panic")
			}
		}()
		poll := time.NewTicker(pollInterval)
		defer poll.Stop()
		reconcile := time.NewTicker(reconcileInterval)
		defer reconcile.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-reconcile.C:
				if _, err := processor.Reconcile(ctx, 100); err != nil && ctx.Err() == nil {
					slog.Warn("object cleanup reconciliation failed", "error_code", "reconcile_failed")
				}
			case <-poll.C:
				if _, err := processor.ProcessOnce(ctx); err != nil && ctx.Err() == nil {
					slog.Warn("object cleanup attempt failed", "error_code", "process_failed")
				}
			}
		}
	}()
	return handle
}

func (h *objectCleanupHandle) Stop() {
	if h == nil {
		return
	}
	h.once.Do(func() { h.cancel(); <-h.done })
}
