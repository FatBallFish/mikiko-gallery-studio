package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type appCleanupProbe struct{ calls atomic.Int32 }

func (p *appCleanupProbe) ProcessOnce(context.Context) (bool, error) {
	p.calls.Add(1)
	return false, nil
}
func (*appCleanupProbe) Reconcile(context.Context, int) (int, error) { return 0, nil }

func TestObjectCleanupLoopStopsGracefully(t *testing.T) {
	probe := &appCleanupProbe{}
	handle := startObjectCleanupLoop(t.Context(), probe, time.Millisecond, time.Hour)
	deadline := time.Now().Add(time.Second)
	for probe.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	handle.Stop()
	stoppedAt := probe.calls.Load()
	time.Sleep(5 * time.Millisecond)
	if stoppedAt == 0 || probe.calls.Load() != stoppedAt {
		t.Fatalf("calls before/after stop=%d/%d", stoppedAt, probe.calls.Load())
	}
	// Stop is intentionally idempotent for stacked runtime defers.
	handle.Stop()
}
