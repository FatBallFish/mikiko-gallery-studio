package setup

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
)

func TestReconcileLegacyCompletedBindingMigratesOnlyVerifiedLegacyDigest(t *testing.T) {
	values := legacyBindingRuntimeValues()
	previousRelease := LegacySetupReleaseIdentity{
		ApplicationVersion: "v1.0.0", ImageRegistry: "docker.io/fatballfish", ImageTag: "v1.0.0", ReleaseVersion: "v1.0.0",
	}
	legacyValues := previousRelease.apply(values)
	legacyDigest, err := legacySetupRequestDigest(legacyValues, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	canonicalDigest, err := setupRequestDigest(values, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if legacyDigest == canonicalDigest {
		t.Fatal("legacy and canonical fixtures unexpectedly produced the same digest")
	}

	proof := CommitProof{
		OperationID: "019d0000-0000-7000-8000-000000000456", InstallationID: values["INSTALLATION_ID"],
		RuntimeSchemaVersion: config.CurrentRuntimeSchemaVersion, ConfigRevision: 7, RequestDigest: legacyDigest,
	}
	stateStore := &legacyBindingStateStore{state: completedLegacyBindingState(proof)}
	bindingStore := &legacyBindingSetupStore{binding: SetupBinding{
		OperationID: proof.OperationID, InstallationID: proof.InstallationID,
		ConfigRevision: proof.ConfigRevision, RequestDigest: legacyDigest,
		AdminID: 1, AdminEmail: "admin@example.com",
	}}

	changed, err := ReconcileLegacyCompletedBinding(t.Context(), completedLegacyBindingBootstrap(values), previousRelease, stateStore, func(context.Context, string) (SetupStoreSession, error) {
		return bindingStore, nil
	})
	if err != nil {
		t.Fatalf("ReconcileLegacyCompletedBinding: %v", err)
	}
	if !changed || bindingStore.binding.RequestDigest != canonicalDigest || stateStore.state.Commit.RequestDigest != canonicalDigest {
		t.Fatalf("legacy binding was not canonicalized: changed=%t binding=%q state=%q want=%q", changed, bindingStore.binding.RequestDigest, stateStore.state.Commit.RequestDigest, canonicalDigest)
	}
	if len(bindingStore.updates) != 1 || bindingStore.updates[0].ExpectedRequestDigest != legacyDigest || bindingStore.updates[0].RequestDigest != canonicalDigest {
		t.Fatalf("database digest updates = %#v", bindingStore.updates)
	}
}

func TestReconcileLegacyCompletedBindingRejectsUnverifiableDigestWithoutWriting(t *testing.T) {
	values := legacyBindingRuntimeValues()
	previousRelease := LegacySetupReleaseIdentity{ApplicationVersion: "v1.0.0", ImageTag: "v1.0.0"}
	legacyDigest, err := legacySetupRequestDigest(previousRelease.apply(values), "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	proof := CommitProof{
		OperationID: "019d0000-0000-7000-8000-000000000456", InstallationID: values["INSTALLATION_ID"],
		RuntimeSchemaVersion: config.CurrentRuntimeSchemaVersion, ConfigRevision: 7, RequestDigest: strings.Repeat("f", 64),
	}
	stateStore := &legacyBindingStateStore{state: completedLegacyBindingState(proof)}
	bindingStore := &legacyBindingSetupStore{binding: SetupBinding{
		OperationID: proof.OperationID, InstallationID: proof.InstallationID,
		ConfigRevision: proof.ConfigRevision, RequestDigest: legacyDigest,
		AdminID: 1, AdminEmail: "admin@example.com",
	}}

	changed, err := ReconcileLegacyCompletedBinding(t.Context(), completedLegacyBindingBootstrap(values), previousRelease, stateStore, func(context.Context, string) (SetupStoreSession, error) {
		return bindingStore, nil
	})
	if !errors.Is(err, ErrSetupBindingMismatch) || changed {
		t.Fatalf("unverifiable binding = changed %t, err %v", changed, err)
	}
	if len(bindingStore.updates) != 0 || stateStore.reconcileCalls != 0 {
		t.Fatalf("unverifiable binding was mutated: database=%#v state_calls=%d", bindingStore.updates, stateStore.reconcileCalls)
	}
}

func TestReconcileLegacyCompletedBindingRollsBackDatabaseWhenStateWriteFails(t *testing.T) {
	values := legacyBindingRuntimeValues()
	previousRelease := LegacySetupReleaseIdentity{ApplicationVersion: "v1.0.0", ImageTag: "v1.0.0"}
	legacyDigest, err := legacySetupRequestDigest(previousRelease.apply(values), "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	proof := CommitProof{
		OperationID: "019d0000-0000-7000-8000-000000000456", InstallationID: values["INSTALLATION_ID"],
		RuntimeSchemaVersion: config.CurrentRuntimeSchemaVersion, ConfigRevision: 7, RequestDigest: legacyDigest,
	}
	stateStore := &legacyBindingStateStore{state: completedLegacyBindingState(proof), reconcileErr: errors.New("disk full")}
	bindingStore := &legacyBindingSetupStore{binding: SetupBinding{
		OperationID: proof.OperationID, InstallationID: proof.InstallationID,
		ConfigRevision: proof.ConfigRevision, RequestDigest: legacyDigest,
		AdminID: 1, AdminEmail: "admin@example.com",
	}}

	changed, err := ReconcileLegacyCompletedBinding(t.Context(), completedLegacyBindingBootstrap(values), previousRelease, stateStore, func(context.Context, string) (SetupStoreSession, error) {
		return bindingStore, nil
	})
	if err == nil || changed {
		t.Fatalf("state failure = changed %t, err %v", changed, err)
	}
	if bindingStore.binding.RequestDigest != legacyDigest || len(bindingStore.updates) != 2 {
		t.Fatalf("database binding was not rolled back: digest=%q updates=%#v", bindingStore.binding.RequestDigest, bindingStore.updates)
	}
}

func legacyBindingRuntimeValues() map[string]string {
	return map[string]string{
		"APPLICATION_VERSION": "v2.0.0", "IMAGE_REGISTRY": "docker.io/fatballfish", "IMAGE_TAG": "v2.0.0", "RELEASE_VERSION": "v2.0.0",
		"DATABASE_URL": "postgres://app:secret@postgres/app", "INSTALLATION_ID": "019d0000-0000-7000-8000-000000000123",
		"PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY": strings.Repeat("a", 64), "SETUP_COMPLETED": "true",
	}
}

func completedLegacyBindingBootstrap(values map[string]string) config.BootstrapConfig {
	return config.BootstrapConfig{
		Path: "config/runtime.env", SchemaVersion: config.CurrentRuntimeSchemaVersion,
		Deployment: config.DeploymentContext{Role: config.DeploymentRoleSingle}, SetupCompleted: true,
		InstallationID: values["INSTALLATION_ID"], ConfigRevision: 7, Values: values,
	}
}

func completedLegacyBindingState(proof CommitProof) InstallState {
	commit := CommitJournal(proof)
	return InstallState{
		SchemaVersion: CurrentInstallStateSchemaVersion, InstallationID: proof.InstallationID,
		DeploymentRole: config.DeploymentRoleSingle, Phase: InstallPhaseCompleted, EverCompleted: true,
		UpdatedAt: time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC), Commit: &commit,
	}
}

type legacyBindingStateStore struct {
	state          InstallState
	reconcileErr   error
	reconcileCalls int
}

func (store *legacyBindingStateStore) Load() (InstallState, bool, error) {
	return store.state, true, nil
}

func (store *legacyBindingStateStore) ReconcileCompletedCommit(proof CommitProof, at time.Time) (InstallState, error) {
	store.reconcileCalls++
	if store.reconcileErr != nil {
		return InstallState{}, store.reconcileErr
	}
	commit := CommitJournal(proof)
	store.state.Commit = &commit
	store.state.UpdatedAt = at
	return store.state, nil
}

type legacyBindingSetupStore struct {
	binding SetupBinding
	updates []SetupBindingDigestUpdate
}

func (store *legacyBindingSetupStore) Initialize(context.Context, SetupInitializationRequest) (SetupBinding, error) {
	return SetupBinding{}, errors.New("not used")
}
func (store *legacyBindingSetupStore) GetBinding(context.Context, string) (SetupBinding, error) {
	return store.binding, nil
}
func (store *legacyBindingSetupStore) MigrationCompleted(context.Context, db.SchemaVersion) (bool, error) {
	return true, nil
}
func (store *legacyBindingSetupStore) Close() error { return nil }
func (store *legacyBindingSetupStore) ReconcileRequestDigest(_ context.Context, update SetupBindingDigestUpdate) (SetupBinding, error) {
	store.updates = append(store.updates, update)
	if store.binding.InstallationID != update.InstallationID || store.binding.OperationID != update.OperationID ||
		store.binding.ConfigRevision != update.ConfigRevision || store.binding.RequestDigest != update.ExpectedRequestDigest {
		return SetupBinding{}, ErrSetupBindingMismatch
	}
	store.binding.RequestDigest = update.RequestDigest
	return store.binding, nil
}
