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

var preDocumentationSetupFields = map[string]struct{}{
	"PIC_GALLERY_DOCS_URL":       {},
	"PIC_GALLERY_DOCS_PROBE_URL": {},
}

var v012RuntimeCompatibilityDefaults = map[string]string{
	"WORKER_ROLES":                     "image,video,media,cleanup",
	"WORKER_MAX_CONCURRENT_TASKS":      "4",
	"WORKER_IMAGE_CONCURRENCY":         "4",
	"WORKER_VIDEO_CONCURRENCY":         "2",
	"WORKER_MEDIA_CONCURRENCY":         "2",
	"WORKER_CLEANUP_CONCURRENCY":       "1",
	"MEDIA_FFMPEG_PATH":                "ffmpeg",
	"MEDIA_FFPROBE_PATH":               "ffprobe",
	"MEDIA_TEMP_DIR":                   "./data/tmp",
	"MEDIA_TEMP_DISK_PAUSE_PERCENT":    "75",
	"MEDIA_TEMP_DISK_CRITICAL_PERCENT": "90",
	"WORKER_METRICS_ADDR":              "127.0.0.1:9091",
	"VIDEO_ARTIFACT_ALLOW_LOOPBACK":    "false",
	"VIDEO_ARTIFACT_TEST_CA_FILE":      "",
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
// derived from an allowlisted historical digest algorithm and runtime schema.
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
	legacyValues := previousRelease.apply(bootstrap.Values)
	legacyDigest, err := legacySetupRequestDigest(legacyValues, binding.AdminEmail)
	if err != nil {
		return false, fmt.Errorf("derive legacy setup binding digest: %w", err)
	}
	canonicalDigest, err := setupRequestDigest(bootstrap.Values, binding.AdminEmail)
	if err != nil {
		return false, fmt.Errorf("derive canonical setup binding digest: %w", err)
	}
	trustedDigests := []string{canonicalDigest, legacyDigest}
	docsURL, docsURLExists := bootstrap.Values["PIC_GALLERY_DOCS_URL"]
	docsProbeURL, docsProbeURLExists := bootstrap.Values["PIC_GALLERY_DOCS_PROBE_URL"]
	if docsURLExists && docsProbeURLExists && docsURL == "/developer-docs/" && docsProbeURL == "" {
		preDocumentationCanonical, digestErr := setupRequestDigestWithOmittedFields(bootstrap.Values, binding.AdminEmail, false, preDocumentationSetupFields)
		if digestErr != nil {
			return false, fmt.Errorf("derive pre-documentation canonical setup binding digest: %w", digestErr)
		}
		preDocumentationLegacy, digestErr := setupRequestDigestWithOmittedFields(legacyValues, binding.AdminEmail, true, preDocumentationSetupFields)
		if digestErr != nil {
			return false, fmt.Errorf("derive pre-documentation legacy setup binding digest: %w", digestErr)
		}
		trustedDigests = append(trustedDigests, preDocumentationCanonical, preDocumentationLegacy)
	}
	if runtimeValuesMatchDefaults(bootstrap.Values, v012RuntimeCompatibilityDefaults) {
		v012Fields := make(map[string]struct{}, len(v012RuntimeCompatibilityDefaults))
		for name := range v012RuntimeCompatibilityDefaults {
			v012Fields[name] = struct{}{}
		}
		v012Canonical, digestErr := setupRequestDigestWithOmittedFields(bootstrap.Values, binding.AdminEmail, false, v012Fields)
		if digestErr != nil {
			return false, fmt.Errorf("derive v0.0.12 canonical setup binding digest: %w", digestErr)
		}
		v012Legacy, digestErr := setupRequestDigestWithOmittedFields(legacyValues, binding.AdminEmail, true, v012Fields)
		if digestErr != nil {
			return false, fmt.Errorf("derive v0.0.12 legacy setup binding digest: %w", digestErr)
		}
		trustedDigests = append(trustedDigests, v012Canonical, v012Legacy)
	}
	stateCanonical := constantTimeDigestEqual(proof.RequestDigest, canonicalDigest)
	bindingCanonical := constantTimeDigestEqual(binding.RequestDigest, canonicalDigest)
	if !matchesSetupDigest(proof.RequestDigest, trustedDigests) || !matchesSetupDigest(binding.RequestDigest, trustedDigests) {
		return false, ErrSetupBindingMismatch
	}
	if !stateCanonical && !bindingCanonical && !constantTimeDigestEqual(proof.RequestDigest, binding.RequestDigest) {
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
	previousBindingDigest := binding.RequestDigest
	if !bindingCanonical {
		if _, err := reconciler.ReconcileRequestDigest(ctx, SetupBindingDigestUpdate{
			OperationID: proof.OperationID, InstallationID: proof.InstallationID, ConfigRevision: proof.ConfigRevision,
			ExpectedRequestDigest: previousBindingDigest, RequestDigest: canonicalDigest,
		}); err != nil {
			return false, fmt.Errorf("reconcile database setup binding digest: %w", err)
		}
		databaseChanged = true
	}
	if !stateCanonical {
		canonicalProof := proof
		canonicalProof.RequestDigest = canonicalDigest
		if _, err := stateStore.ReconcileCompletedCommit(canonicalProof, time.Now().UTC()); err != nil {
			if databaseChanged {
				_, rollbackErr := reconciler.ReconcileRequestDigest(context.WithoutCancel(ctx), SetupBindingDigestUpdate{
					OperationID: proof.OperationID, InstallationID: proof.InstallationID, ConfigRevision: proof.ConfigRevision,
					ExpectedRequestDigest: canonicalDigest, RequestDigest: previousBindingDigest,
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

func runtimeValuesMatchDefaults(values map[string]string, defaults map[string]string) bool {
	for name, expected := range defaults {
		actual, exists := values[name]
		if !exists || actual != expected {
			return false
		}
	}
	return true
}

func matchesSetupDigest(digest string, candidates []string) bool {
	for _, candidate := range candidates {
		if constantTimeDigestEqual(digest, candidate) {
			return true
		}
	}
	return false
}
