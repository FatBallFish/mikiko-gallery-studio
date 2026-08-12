package video

import "fmt"

type ItemState string

const (
	ItemStateQueued           ItemState = "queued"
	ItemStateSubmitting       ItemState = "submitting"
	ItemStateReconciling      ItemState = "reconciling"
	ItemStateProviderQueued   ItemState = "provider_queued"
	ItemStateProviderRunning  ItemState = "provider_running"
	ItemStateCancelRequested  ItemState = "cancel_requested"
	ItemStateArtifactPending  ItemState = "artifact_pending"
	ItemStateRecoveryRequired ItemState = "recovery_required"
	ItemStateSucceeded        ItemState = "succeeded"
	ItemStateFailed           ItemState = "failed"
	ItemStateCancelled        ItemState = "cancelled"
)

type TaskStatus string

const (
	TaskStatusQueued    TaskStatus = "queued"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusSaving    TaskStatus = "saving"
	TaskStatusSucceeded TaskStatus = "succeeded"
	TaskStatusPartial   TaskStatus = "partial"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

type ItemStateSnapshot struct {
	State   ItemState
	Version int64
}

type ItemTransition struct {
	ExpectedVersion int64
	Target          ItemState
}

type ItemTransitionResult struct {
	Snapshot ItemStateSnapshot
	Changed  bool
}

var allowedItemTransitions = map[ItemState]map[ItemState]struct{}{
	ItemStateQueued: {
		ItemStateSubmitting:      {},
		ItemStateCancelRequested: {},
		ItemStateFailed:          {},
	},
	ItemStateSubmitting: {
		ItemStateProviderQueued: {},
		ItemStateReconciling:    {},
		ItemStateFailed:         {},
	},
	ItemStateReconciling: {
		ItemStateProviderQueued: {},
		ItemStateFailed:         {},
	},
	ItemStateProviderQueued: {
		ItemStateProviderRunning: {},
		ItemStateCancelRequested: {},
		ItemStateArtifactPending: {},
		ItemStateFailed:          {},
	},
	ItemStateProviderRunning: {
		ItemStateCancelRequested: {},
		ItemStateArtifactPending: {},
		ItemStateFailed:          {},
	},
	ItemStateCancelRequested: {
		ItemStateCancelled:       {},
		ItemStateArtifactPending: {},
		ItemStateFailed:          {},
	},
	ItemStateArtifactPending: {
		ItemStateSucceeded:        {},
		ItemStateRecoveryRequired: {},
	},
	ItemStateRecoveryRequired: {
		ItemStateArtifactPending: {},
		ItemStateFailed:          {},
	},
}

func AdvanceItemState(current ItemStateSnapshot, transition ItemTransition) (ItemTransitionResult, error) {
	if transition.ExpectedVersion < current.Version {
		return ItemTransitionResult{Snapshot: current}, nil
	}
	if transition.ExpectedVersion > current.Version {
		return ItemTransitionResult{}, fmt.Errorf("video item version conflict: expected %d, current %d", transition.ExpectedVersion, current.Version)
	}
	if transition.Target == current.State {
		return ItemTransitionResult{Snapshot: current}, nil
	}
	allowed, ok := allowedItemTransitions[current.State]
	if !ok {
		return ItemTransitionResult{}, fmt.Errorf("video item state %q is terminal", current.State)
	}
	if _, ok := allowed[transition.Target]; !ok {
		return ItemTransitionResult{}, fmt.Errorf("invalid video item transition %q -> %q", current.State, transition.Target)
	}
	return ItemTransitionResult{
		Snapshot: ItemStateSnapshot{State: transition.Target, Version: current.Version + 1},
		Changed:  true,
	}, nil
}

func AggregateTaskStatus(items []ItemState) TaskStatus {
	if len(items) == 0 {
		return TaskStatusQueued
	}
	var succeeded, failed, cancelled, queued, running, saving int
	for _, state := range items {
		switch state {
		case ItemStateSucceeded:
			succeeded++
		case ItemStateFailed:
			failed++
		case ItemStateCancelled:
			cancelled++
		case ItemStateQueued:
			queued++
		case ItemStateArtifactPending, ItemStateRecoveryRequired:
			saving++
		default:
			running++
		}
	}
	if saving > 0 {
		return TaskStatusSaving
	}
	if running > 0 || (queued > 0 && succeeded+failed+cancelled > 0) {
		return TaskStatusRunning
	}
	if queued == len(items) {
		return TaskStatusQueued
	}
	if succeeded == len(items) {
		return TaskStatusSucceeded
	}
	if cancelled == len(items) {
		return TaskStatusCancelled
	}
	if succeeded > 0 {
		return TaskStatusPartial
	}
	return TaskStatusFailed
}
