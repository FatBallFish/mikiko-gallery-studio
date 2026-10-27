package setup

import (
	"context"
	"fmt"
	"time"
)

// CompareSwapCompletedCommit changes a completed configuration proof only when
// the caller still owns its exact previous proof. Setup is never reopened.
func (store *StateStore) CompareSwapCompletedCommit(expected, replacement CommitProof, at time.Time) (InstallState, error) {
	if expected.Validate() != nil || replacement.Validate() != nil || expected.InstallationID != replacement.InstallationID || expected.OperationID != replacement.OperationID || expected.RuntimeSchemaVersion != replacement.RuntimeSchemaVersion || (replacement.ConfigRevision != expected.ConfigRevision+1 && replacement.ConfigRevision != expected.ConfigRevision-1) {
		return InstallState{}, fmt.Errorf("invalid completed configuration transition")
	}
	at, err := validateTransitionTime(at)
	if err != nil {
		return InstallState{}, err
	}
	if err := store.validate(); err != nil {
		return InstallState{}, err
	}
	return store.withMutationLock(func() (InstallState, error) {
		state, exists, err := store.loadUnlocked()
		if err != nil {
			return InstallState{}, err
		}
		if !exists || state.Phase != InstallPhaseCompleted || !state.EverCompleted || state.Commit == nil || !commitProofsEqual(*state.Commit, expected) {
			return InstallState{}, ErrSetupBindingMismatch
		}
		state.Commit = &replacement
		state.UpdatedAt = at
		if err := store.saveUnlocked(state); err != nil {
			return InstallState{}, err
		}
		return state, nil
	})
}

// SetupBindingConfigUpdate is an exact compare-and-swap of a completed binding.
// The administrator and original setup operation remain unchanged.
type SetupBindingConfigUpdate struct {
	Expected       SetupBinding
	ConfigRevision int
	RequestDigest  string
}

type SetupBindingConfigUpdater interface {
	CompareSwapConfiguration(ctx context.Context, update SetupBindingConfigUpdate) (SetupBinding, error)
}
