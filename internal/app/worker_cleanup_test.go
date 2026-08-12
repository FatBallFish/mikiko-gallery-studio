package app

import (
	"context"
	"testing"
)

type cleanupProcessProbe struct {
	name      string
	processed bool
	calls     *[]string
}

func (probe *cleanupProcessProbe) ProcessOnce(context.Context) (bool, error) {
	*probe.calls = append(*probe.calls, probe.name)
	return probe.processed, nil
}

type cleanupReconcileProbe struct {
	cleanupProcessProbe
	reconciles int
}

func (probe *cleanupReconcileProbe) Reconcile(context.Context, int) (int, error) {
	probe.reconciles++
	return 0, nil
}

func TestCleanupRoleProcessorRotatesAcrossAllProcessors(t *testing.T) {
	calls := []string{}
	cleanup := &cleanupReconcileProbe{cleanupProcessProbe: cleanupProcessProbe{name: "objects", processed: true, calls: &calls}}
	processor := &cleanupRoleProcessor{
		cleanup: cleanup,
		processors: []processOnce{
			cleanup,
			&cleanupProcessProbe{name: "exports", processed: true, calls: &calls},
			&cleanupProcessProbe{name: "multipart", processed: true, calls: &calls},
			&cleanupProcessProbe{name: "media", processed: true, calls: &calls},
			&cleanupProcessProbe{name: "canvas", processed: true, calls: &calls},
		},
	}

	for range 5 {
		processed, err := processor.ProcessOnce(t.Context())
		if err != nil || !processed {
			t.Fatalf("ProcessOnce() = %v, %v", processed, err)
		}
	}
	want := []string{"objects", "exports", "multipart", "media", "canvas"}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for index := range want {
		if calls[index] != want[index] {
			t.Fatalf("calls = %v, want %v", calls, want)
		}
	}
	if cleanup.reconciles != 1 {
		t.Fatalf("object reconciliation calls = %d, want 1", cleanup.reconciles)
	}
}

func TestCleanupRoleProcessorFallsThroughIdleProcessorsWithoutStarvation(t *testing.T) {
	calls := []string{}
	cleanup := &cleanupReconcileProbe{cleanupProcessProbe: cleanupProcessProbe{name: "objects", calls: &calls}}
	processor := &cleanupRoleProcessor{
		cleanup: cleanup,
		processors: []processOnce{
			cleanup,
			&cleanupProcessProbe{name: "exports", calls: &calls},
			&cleanupProcessProbe{name: "multipart", processed: true, calls: &calls},
			&cleanupProcessProbe{name: "media", processed: true, calls: &calls},
		},
	}

	processed, err := processor.ProcessOnce(t.Context())
	if err != nil || !processed {
		t.Fatalf("first ProcessOnce() = %v, %v", processed, err)
	}
	processed, err = processor.ProcessOnce(t.Context())
	if err != nil || !processed {
		t.Fatalf("second ProcessOnce() = %v, %v", processed, err)
	}
	want := []string{"objects", "exports", "multipart", "media"}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for index := range want {
		if calls[index] != want[index] {
			t.Fatalf("calls = %v, want %v", calls, want)
		}
	}
}
