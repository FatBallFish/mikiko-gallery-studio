package video

import "testing"

func TestAdvanceItemStateIsMonotonicAndIdempotent(t *testing.T) {
	current := ItemStateSnapshot{State: ItemStateProviderRunning, Version: 5}

	stale, err := AdvanceItemState(current, ItemTransition{ExpectedVersion: 4, Target: ItemStateProviderQueued})
	if err != nil {
		t.Fatalf("stale transition returned error: %v", err)
	}
	if stale.Changed || stale.Snapshot != current {
		t.Fatalf("stale transition changed state: %#v", stale)
	}

	if _, err := AdvanceItemState(current, ItemTransition{ExpectedVersion: 5, Target: ItemStateProviderQueued}); err == nil {
		t.Fatal("state regression must be rejected")
	}

	duplicate, err := AdvanceItemState(current, ItemTransition{ExpectedVersion: 5, Target: ItemStateProviderRunning})
	if err != nil {
		t.Fatalf("duplicate transition returned error: %v", err)
	}
	if duplicate.Changed || duplicate.Snapshot != current {
		t.Fatalf("duplicate transition changed state: %#v", duplicate)
	}
}

func TestAdvanceItemStateSupportsRecoveryAndLateCancellation(t *testing.T) {
	tests := []struct {
		name string
		from ItemState
		to   ItemState
	}{
		{name: "unknown submit reconciles", from: ItemStateSubmitting, to: ItemStateReconciling},
		{name: "reconciled submit finds job", from: ItemStateReconciling, to: ItemStateProviderQueued},
		{name: "artifact transfer fails", from: ItemStateArtifactPending, to: ItemStateRecoveryRequired},
		{name: "artifact retry resumes", from: ItemStateRecoveryRequired, to: ItemStateArtifactPending},
		{name: "cancel arrives after provider success", from: ItemStateCancelRequested, to: ItemStateArtifactPending},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := AdvanceItemState(ItemStateSnapshot{State: tt.from, Version: 9}, ItemTransition{ExpectedVersion: 9, Target: tt.to})
			if err != nil {
				t.Fatalf("AdvanceItemState() error = %v", err)
			}
			if !result.Changed || result.Snapshot.State != tt.to || result.Snapshot.Version != 10 {
				t.Fatalf("AdvanceItemState() = %#v", result)
			}
		})
	}
}

func TestAggregateTaskStatusSeparatesPartialSuccess(t *testing.T) {
	tests := []struct {
		name  string
		items []ItemState
		want  TaskStatus
	}{
		{name: "queued", items: []ItemState{ItemStateQueued, ItemStateQueued}, want: TaskStatusQueued},
		{name: "running", items: []ItemState{ItemStateProviderRunning, ItemStateQueued}, want: TaskStatusRunning},
		{name: "saving", items: []ItemState{ItemStateArtifactPending, ItemStateSucceeded}, want: TaskStatusSaving},
		{name: "succeeded", items: []ItemState{ItemStateSucceeded, ItemStateSucceeded}, want: TaskStatusSucceeded},
		{name: "partial", items: []ItemState{ItemStateSucceeded, ItemStateFailed}, want: TaskStatusPartial},
		{name: "failed", items: []ItemState{ItemStateFailed, ItemStateCancelled}, want: TaskStatusFailed},
		{name: "cancelled", items: []ItemState{ItemStateCancelled, ItemStateCancelled}, want: TaskStatusCancelled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AggregateTaskStatus(tt.items); got != tt.want {
				t.Fatalf("AggregateTaskStatus(%v) = %q, want %q", tt.items, got, tt.want)
			}
		})
	}
}
