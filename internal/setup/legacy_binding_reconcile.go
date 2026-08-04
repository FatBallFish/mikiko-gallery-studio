package setup

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
)

type LegacySetupReleaseIdentity struct {
	ApplicationVersion string
	ImageRegistry      string
	ImageTag           string
	ReleaseVersion     string
}

func (identity LegacySetupReleaseIdentity) apply(values map[string]string) map[string]string {
	legacyValues := make(map[string]string, len(values))
	for key, value := range values {
		legacyValues[key] = value
	}
	legacyValues["APPLICATION_VERSION"] = identity.ApplicationVersion
	legacyValues["IMAGE_REGISTRY"] = identity.ImageRegistry
	legacyValues["IMAGE_TAG"] = identity.ImageTag
	legacyValues["RELEASE_VERSION"] = identity.ReleaseVersion
	return legacyValues
}

// ReconcileLegacyCompletedBinding migrates only bindings that are provably
// derived from the pre-release-field-exclusion digest algorithm.
func ReconcileLegacyCompletedBinding(
	ctx context.Context,
	bootstrap config.BootstrapConfig,
	previousRelease LegacySetupReleaseIdentity,
	stateStore CompletedBindingStateStore,
	openStore SetupStoreOpener,
) (changed bool, returnErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if stateStore == nil || openStore == nil {
		return false, fmt.Errorf("legacy setup binding reconciliation dependencies are required")
	}
	if !bootstrap.SetupCompleted || bootstrap.InstallationID == "" || bootstrap.Values == nil {
		return false, ErrSetupBindingMismatch
	}
	state, exists, err := stateStore.Load()
	if err != nil {
		return false, fmt.Errorf("load completed install state: %w", err)
	}
	if !exists || state.Validate() != nil || state.Phase != InstallPhaseCompleted || !state.EverCompleted || state.Commit == nil {
		return false, ErrSetupBindingMismatch
	}
	proof := CommitProof(*state.Commit)
	if state.InstallationID != bootstrap.InstallationID || proof.InstallationID != bootstrap.InstallationID ||
		proof.ConfigRevision != bootstrap.ConfigRevision || proof.RuntimeSchemaVersion != bootstrap.SchemaVersion {
		return false, ErrSetupBindingMismatch
	}

	store, err := openStore(ctx, bootstrap.Values["DATABASE_URL"])
	if err != nil {
		return false, fmt.Errorf("open setup binding store: %w", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("close setup binding store: %w", closeErr))
		}
	}()
	binding, err := store.GetBinding(ctx, bootstrap.InstallationID)
	if err != nil {
		return false, fmt.Errorf("load setup binding: %w", err)
	}
	if binding.OperationID != proof.OperationID || binding.InstallationID != proof.InstallationID || binding.ConfigRevision != proof.ConfigRevision {
		return false, ErrSetupBindingMismatch
	}
	legacyDigest, err := legacySetupRequestDigest(previousRelease.apply(bootstrap.Values), binding.AdminEmail)
	if err != nil {
		return false, fmt.Errorf("derive legacy setup binding digest: %w", err)
	}
	canonicalDigest, err := setupRequestDigest(bootstrap.Values, binding.AdminEmail)
	if err != nil {
		return false, fmt.Errorf("derive canonical setup binding digest: %w", err)
	}
	stateLegacy := constantTimeDigestEqual(proof.RequestDigest, legacyDigest)
	stateCanonical := constantTimeDigestEqual(proof.RequestDigest, canonicalDigest)
	bindingLegacy := constantTimeDigestEqual(binding.RequestDigest, legacyDigest)
	bindingCanonical := constantTimeDigestEqual(binding.RequestDigest, canonicalDigest)
	if (!stateLegacy && !stateCanonical) || (!bindingLegacy && !bindingCanonical) {
		return false, ErrSetupBindingMismatch
	}
	if stateCanonical && bindingCanonical {
		return false, nil
	}
	reconciler, ok := store.(SetupBindingDigestReconciler)
	if !ok {
		return false, fmt.Errorf("setup binding store does not support digest reconciliation")
	}

	databaseChanged := false
	if bindingLegacy && !bindingCanonical {
		if _, err := reconciler.ReconcileRequestDigest(ctx, SetupBindingDigestUpdate{
			OperationID: proof.OperationID, InstallationID: proof.InstallationID, ConfigRevision: proof.ConfigRevision,
			ExpectedRequestDigest: legacyDigest, RequestDigest: canonicalDigest,
		}); err != nil {
			return false, fmt.Errorf("reconcile database setup binding digest: %w", err)
		}
		databaseChanged = true
	}
	if stateLegacy && !stateCanonical {
		canonicalProof := proof
		canonicalProof.RequestDigest = canonicalDigest
		if _, err := stateStore.ReconcileCompletedCommit(canonicalProof, time.Now().UTC()); err != nil {
			if databaseChanged {
				_, rollbackErr := reconciler.ReconcileRequestDigest(context.WithoutCancel(ctx), SetupBindingDigestUpdate{
					OperationID: proof.OperationID, InstallationID: proof.InstallationID, ConfigRevision: proof.ConfigRevision,
					ExpectedRequestDigest: canonicalDigest, RequestDigest: legacyDigest,
				})
				if rollbackErr != nil {
					return false, errors.Join(fmt.Errorf("reconcile install-state setup binding digest: %w", err), fmt.Errorf("restore legacy database setup binding digest: %w", rollbackErr))
				}
			}
			return false, fmt.Errorf("reconcile install-state setup binding digest: %w", err)
		}
	}
	return true, nil
}
