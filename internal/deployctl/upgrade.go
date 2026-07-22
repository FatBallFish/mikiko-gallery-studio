package deployctl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/config"
)

type UpgradeDependencies struct {
	LoadInstallation func(string) (InstallPlan, config.RuntimeEnvDocument, error)
	WriteRuntimeEnv  func(string, []byte) error
	WriteManifest    func(string, InstallPlan) error
	Migrate          func(context.Context, string) error
	ApplyDeployment  func(context.Context, InstallPlan) error
}

type UpgradeResult struct {
	RuntimeEnvPath  string
	PreviousVersion string
	CurrentVersion  string
	Migrated        bool
}

func Upgrade(ctx context.Context, options UpgradeOptions, dependencies UpgradeDependencies) (UpgradeResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return UpgradeResult{}, err
	}
	if dependencies.LoadInstallation == nil {
		dependencies.LoadInstallation = loadInstallation
	}
	if dependencies.WriteRuntimeEnv == nil {
		dependencies.WriteRuntimeEnv = config.WriteRuntimeEnvAtomic
	}
	if dependencies.ApplyDeployment == nil {
		dependencies.ApplyDeployment = func(context.Context, InstallPlan) error {
			return fmt.Errorf("upgrade deployment dependency is required")
		}
	}
	runtimeDir := filepath.Clean(defaultString(options.RuntimeDir, "."))
	plan, document, err := dependencies.LoadInstallation(runtimeDir)
	if err != nil {
		return UpgradeResult{}, fmt.Errorf("load installation for upgrade: %w", err)
	}
	if options.Migrate && plan.Role != config.DeploymentRoleSingle && plan.Role != config.DeploymentRoleControl {
		return UpgradeResult{}, fmt.Errorf("deployment role %q cannot execute migrations", plan.Role)
	}
	if dependencies.Migrate == nil && options.Migrate {
		return UpgradeResult{}, fmt.Errorf("upgrade migration dependency is required")
	}
	if !strings.EqualFold(document.Values["SETUP_COMPLETED"], "true") {
		return UpgradeResult{}, fmt.Errorf("upgrade requires a completed installation")
	}

	previousValues := cloneRuntimeValues(document.Values)
	previousPlan := plan
	updatedValues := cloneRuntimeValues(document.Values)
	previousVersion := updatedValues["APPLICATION_VERSION"]
	targetVersion := strings.TrimSpace(options.ApplicationVersion)
	if targetVersion == "" {
		targetVersion = previousVersion
	}
	if err := config.ValidateApplicationVersion(targetVersion); err != nil {
		return UpgradeResult{}, fmt.Errorf("validate target application version: %w", err)
	}
	updatedValues["APPLICATION_VERSION"] = targetVersion
	plan.ApplicationVersion = targetVersion
	plan.RuntimeDir = runtimeDir
	if plan.Mode == config.DeploymentModeDocker {
		if strings.TrimSpace(options.ImageRegistry) != "" {
			updatedValues["IMAGE_REGISTRY"] = options.ImageRegistry
			plan.ImageRegistry = options.ImageRegistry
		}
		imageTag := strings.TrimSpace(options.ImageTag)
		if imageTag == "" && targetVersion != previousVersion {
			imageTag = targetVersion
		}
		if imageTag != "" {
			updatedValues["IMAGE_TAG"] = imageTag
			plan.ImageTag = imageTag
		}
	} else {
		releaseVersion := strings.TrimSpace(options.ReleaseVersion)
		if releaseVersion == "" && targetVersion != previousVersion {
			releaseVersion = targetVersion
		}
		if releaseVersion != "" {
			updatedValues["RELEASE_VERSION"] = releaseVersion
			plan.ReleaseVersion = releaseVersion
		}
	}

	updated, err := config.RenderRuntimeEnv(config.DefaultRuntimeSchema(), updatedValues, document.Extensions)
	if err != nil {
		return UpgradeResult{}, fmt.Errorf("render upgraded runtime configuration: %w", redactRuntimeError(err, updatedValues))
	}
	previous, err := config.RenderRuntimeEnv(config.DefaultRuntimeSchema(), previousValues, document.Extensions)
	if err != nil {
		return UpgradeResult{}, fmt.Errorf("render current runtime configuration for rollback: %w", redactRuntimeError(err, previousValues))
	}
	runtimeEnvPath := filepath.Join(runtimeDir, "config", "runtime.env")
	manifestPath := filepath.Join(runtimeDir, "deployment.json")
	manifestUpdated := false
	if dependencies.WriteManifest != nil {
		if err := dependencies.WriteManifest(manifestPath, plan); err != nil {
			return UpgradeResult{}, fmt.Errorf("write upgraded deployment manifest: %w", err)
		}
		manifestUpdated = true
	}
	if err := dependencies.WriteRuntimeEnv(runtimeEnvPath, updated); err != nil {
		if manifestUpdated {
			_ = dependencies.WriteManifest(manifestPath, previousPlan)
		}
		return UpgradeResult{}, fmt.Errorf("write upgraded runtime configuration: %w", err)
	}
	rollback := func(cause error, migrationWasApplied bool) error {
		restoreErr := dependencies.WriteRuntimeEnv(runtimeEnvPath, previous)
		if manifestUpdated {
			restoreErr = errors.Join(restoreErr, dependencies.WriteManifest(manifestPath, previousPlan))
		}
		if migrationWasApplied && restoreErr == nil && dependencies.Migrate != nil {
			restoreErr = errors.Join(restoreErr, dependencies.Migrate(ctx, runtimeEnvPath))
		}
		if restoreErr != nil {
			return errors.Join(cause, fmt.Errorf("restore previous runtime configuration: %w", restoreErr))
		}
		return cause
	}

	migrated := false
	if options.Migrate {
		if err := dependencies.Migrate(ctx, runtimeEnvPath); err != nil {
			return UpgradeResult{}, rollback(fmt.Errorf("migrate upgraded database: %w", redactRuntimeError(err, updatedValues)), false)
		}
		migrated = true
	}
	if err := dependencies.ApplyDeployment(ctx, plan); err != nil {
		return UpgradeResult{}, rollback(fmt.Errorf("roll upgraded services: %w", redactRuntimeError(err, updatedValues)), migrated)
	}
	return UpgradeResult{RuntimeEnvPath: runtimeEnvPath, PreviousVersion: previousVersion, CurrentVersion: targetVersion, Migrated: migrated}, nil
}

func writeDeploymentManifestPlan(path string, plan InstallPlan) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var manifest deploymentManifest
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil || manifest.SchemaVersion != 1 || manifest.InstallationID == "" {
		return fmt.Errorf("deployment manifest is invalid")
	}
	manifest.Plan = plan
	rendered, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return config.WriteRuntimeEnvAtomic(path, append(rendered, '\n'))
}

func loadInstallation(runtimeDir string) (InstallPlan, config.RuntimeEnvDocument, error) {
	runtimeDir = filepath.Clean(defaultString(runtimeDir, "."))
	manifestContent, err := os.ReadFile(filepath.Join(runtimeDir, "deployment.json"))
	if err != nil {
		return InstallPlan{}, config.RuntimeEnvDocument{}, fmt.Errorf("read deployment manifest: %w", err)
	}
	var manifest deploymentManifest
	decoder := json.NewDecoder(strings.NewReader(string(manifestContent)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil || manifest.SchemaVersion != 1 || strings.TrimSpace(manifest.InstallationID) == "" {
		return InstallPlan{}, config.RuntimeEnvDocument{}, fmt.Errorf("deployment manifest is invalid")
	}
	manifest.Plan.RuntimeDir = runtimeDir
	if err := ValidateInstallPlan(manifest.Plan); err != nil {
		return InstallPlan{}, config.RuntimeEnvDocument{}, fmt.Errorf("validate deployment manifest plan: %w", err)
	}
	runtimeContent, err := os.ReadFile(filepath.Join(runtimeDir, "config", "runtime.env"))
	if err != nil {
		return InstallPlan{}, config.RuntimeEnvDocument{}, fmt.Errorf("read runtime configuration: %w", err)
	}
	document, err := config.ParseRuntimeEnv(runtimeContent)
	if err != nil {
		return InstallPlan{}, config.RuntimeEnvDocument{}, fmt.Errorf("parse runtime configuration: %w", err)
	}
	if document.Values["INSTALLATION_ID"] != manifest.InstallationID {
		return InstallPlan{}, config.RuntimeEnvDocument{}, fmt.Errorf("runtime and deployment manifest installation identities do not match")
	}
	return manifest.Plan, document, nil
}

type UninstallDependencies struct {
	LoadInstallation           func(string) (InstallPlan, string, error)
	StopDeployment             func(context.Context, InstallPlan) error
	DestroyPersistentResources func(context.Context, InstallPlan) error
	RemoveRuntimeDirectory     func(string) error
}

func DestructiveUninstallConfirmation(installationID string) string {
	return "DELETE " + installationID + " PERSISTENT DATA"
}

func Uninstall(ctx context.Context, options UninstallOptions, dependencies UninstallDependencies) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if dependencies.LoadInstallation == nil {
		dependencies.LoadInstallation = func(runtimeDir string) (InstallPlan, string, error) {
			plan, document, err := loadInstallation(runtimeDir)
			return plan, document.Values["INSTALLATION_ID"], err
		}
	}
	if dependencies.StopDeployment == nil {
		return fmt.Errorf("uninstall stop dependency is required")
	}
	runtimeDir := filepath.Clean(defaultString(options.RuntimeDir, "."))
	plan, installationID, err := dependencies.LoadInstallation(runtimeDir)
	if err != nil {
		return fmt.Errorf("load installation for uninstall: %w", err)
	}
	if options.DeleteData && options.Confirmation != DestructiveUninstallConfirmation(installationID) {
		return fmt.Errorf("destructive confirmation does not match installation %s", installationID)
	}
	if options.DeleteData && (dependencies.DestroyPersistentResources == nil || dependencies.RemoveRuntimeDirectory == nil) {
		return fmt.Errorf("destructive uninstall dependencies are required")
	}
	if err := dependencies.StopDeployment(ctx, plan); err != nil {
		return fmt.Errorf("stop deployment: %w", err)
	}
	if !options.DeleteData {
		return nil
	}
	if err := dependencies.DestroyPersistentResources(ctx, plan); err != nil {
		return fmt.Errorf("destroy persistent resources: %w", err)
	}
	if err := dependencies.RemoveRuntimeDirectory(runtimeDir); err != nil {
		return fmt.Errorf("remove runtime directory: %w", err)
	}
	return nil
}
