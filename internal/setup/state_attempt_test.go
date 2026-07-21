package setup

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStateStoreAttemptCASBindsPendingOperationAndPromotesToCommit(t *testing.T) {
	store := NewStateStoreAt(filepath.Join(t.TempDir(), "install-state.json"))
	initial := pendingState()
	if err := store.Initialize(initial); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	attempt := SetupAttempt{
		OperationID: validCommitProof().OperationID, ConfigRevision: validCommitProof().ConfigRevision,
		RequestDigest: strings.Repeat("a", 64), AdminCredentialVerifier: strings.Repeat("b", 64),
	}
	reserved, err := store.BeginAttempt(attempt, testStateTime.Add(time.Minute))
	if err != nil || reserved.Attempt == nil || *reserved.Attempt != attempt || reserved.Phase != InstallPhasePending {
		t.Fatalf("BeginAttempt=(%+v, %v)", reserved, err)
	}
	if repeated, err := store.BeginAttempt(attempt, testStateTime.Add(2*time.Minute)); err != nil || repeated.Attempt == nil || *repeated.Attempt != attempt {
		t.Fatalf("idempotent BeginAttempt=(%+v, %v)", repeated, err)
	}
	competing := attempt
	competing.OperationID = "22222222-2222-4222-8222-222222222222"
	if _, err := store.BeginAttempt(competing, testStateTime.Add(2*time.Minute)); !errors.Is(err, ErrSetupOperationConflict) {
		t.Fatalf("competing BeginAttempt error=%v", err)
	}
	mismatchedProof := validCommitProof()
	mismatchedProof.RequestDigest = strings.Repeat("f", 64)
	if _, err := store.BeginCommit(mismatchedProof, testStateTime.Add(2*time.Minute)); !errors.Is(err, ErrSetupBindingMismatch) {
		t.Fatalf("mismatched digest BeginCommit error=%v", err)
	}

	committing, err := store.BeginCommit(validCommitProof(), testStateTime.Add(3*time.Minute))
	if err != nil || committing.Phase != InstallPhaseCommitting || committing.Attempt != nil {
		t.Fatalf("BeginCommit promotion=(%+v, %v)", committing, err)
	}
}

func TestStateStoreClearAttemptRequiresExactPendingBinding(t *testing.T) {
	store := NewStateStoreAt(filepath.Join(t.TempDir(), "install-state.json"))
	initial := pendingState()
	if err := store.Initialize(initial); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	attempt := SetupAttempt{
		OperationID: validCommitProof().OperationID, ConfigRevision: validCommitProof().ConfigRevision,
		RequestDigest: strings.Repeat("c", 64), AdminCredentialVerifier: strings.Repeat("d", 64),
	}
	if _, err := store.BeginAttempt(attempt, testStateTime.Add(time.Minute)); err != nil {
		t.Fatalf("BeginAttempt: %v", err)
	}
	mismatch := attempt
	mismatch.RequestDigest = strings.Repeat("e", 64)
	if _, err := store.ClearAttempt(mismatch, testStateTime.Add(2*time.Minute)); !errors.Is(err, ErrSetupBindingMismatch) {
		t.Fatalf("mismatched ClearAttempt error=%v", err)
	}
	mismatch = attempt
	mismatch.AdminCredentialVerifier = strings.Repeat("f", 64)
	if _, err := store.ClearAttempt(mismatch, testStateTime.Add(2*time.Minute)); !errors.Is(err, ErrSetupBindingMismatch) {
		t.Fatalf("mismatched credential verifier ClearAttempt error=%v", err)
	}
	cleared, err := store.ClearAttempt(attempt, testStateTime.Add(3*time.Minute))
	if err != nil || cleared.Attempt != nil || cleared.Phase != InstallPhasePending {
		t.Fatalf("ClearAttempt=(%+v, %v)", cleared, err)
	}
}
