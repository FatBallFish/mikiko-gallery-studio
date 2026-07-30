package mgsctl

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/config"
)

type RuntimeExecutors struct {
	Docker DockerExecutor
	Native NativeExecutor
}

func ExecuteRuntimeAction(ctx context.Context, kind CommandKind, runtimeDir string, executors RuntimeExecutors) error {
	plan, _, err := loadInstallation(filepath.Clean(defaultString(runtimeDir, ".")))
	if err != nil {
		return err
	}
	switch kind {
	case CommandStatus:
		return executeDeploymentAction(ctx, plan, DockerActionStatus, NativeActionStatus, executors)
	case CommandRestart:
		return executeDeploymentAction(ctx, plan, DockerActionRestart, NativeActionRestart, executors)
	default:
		return fmt.Errorf("unsupported runtime action %q", kind)
	}
}

func UpgradeDeploymentDependencies(executors RuntimeExecutors) UpgradeDependencies {
	return UpgradeDependencies{
		WriteManifest: writeDeploymentManifestPlan,
		PrepareTarget: func(ctx context.Context, target *UpgradeTarget) error {
			switch target.Plan.Mode {
			case config.DeploymentModeDocker:
				return executors.Docker.PrepareUpgrade(ctx, target)
			case config.DeploymentModeNative:
				return executors.Native.PrepareUpgrade(ctx, target)
			default:
				return fmt.Errorf("unsupported deployment mode %q", target.Plan.Mode)
			}
		},
		MigrateTarget: func(ctx context.Context, target UpgradeTarget, runtimeEnvPath string) error {
			switch target.Plan.Mode {
			case config.DeploymentModeDocker:
				return executors.Docker.MigrateUpgrade(ctx, target, runtimeEnvPath)
			case config.DeploymentModeNative:
				return executors.Native.MigrateUpgrade(ctx, target, runtimeEnvPath)
			default:
				return fmt.Errorf("unsupported deployment mode %q", target.Plan.Mode)
			}
		},
		ApplyDeployment: func(ctx context.Context, plan InstallPlan) error {
			return executeDeploymentAction(ctx, plan, DockerActionUpdate, NativeActionUpdate, executors)
		},
	}
}

func SetupTokenResetRuntimeDependencies(executors RuntimeExecutors) SetupTokenResetDependencies {
	return SetupTokenResetDependencies{
		RestartDeployment: func(ctx context.Context, plan InstallPlan) error {
			return executeDeploymentAction(ctx, plan, DockerActionReloadSetupToken, NativeActionReloadSetupToken, executors)
		},
	}
}

func UninstallRuntimeDependencies(executors RuntimeExecutors) UninstallDependencies {
	return UninstallDependencies{
		ValidateRuntimeDirectory: validateManagedRuntimeDirectory,
		StopDeployment: func(ctx context.Context, plan InstallPlan) error {
			return executeDeploymentAction(ctx, plan, DockerActionUninstall, NativeActionUninstall, executors)
		},
		DestroyPersistentResources: func(ctx context.Context, plan InstallPlan) error {
			if plan.Mode == config.DeploymentModeDocker {
				return executors.Docker.Run(ctx, DockerActionDestroy, plan)
			}
			return nil
		},
		RemoveRuntimeDirectory: func(runtimeDir string) error {
			absolute, err := filepath.Abs(runtimeDir)
			if err != nil {
				return err
			}
			workingDirectory, err := os.Getwd()
			if err != nil {
				return err
			}
			workingDirectory, err = filepath.Abs(workingDirectory)
			if err != nil {
				return err
			}
			if absolute == filepath.VolumeName(absolute)+string(filepath.Separator) || absolute == workingDirectory {
				return fmt.Errorf("refuse to remove filesystem root or current working directory")
			}
			relativeWorkingDirectory, err := filepath.Rel(absolute, workingDirectory)
			if err != nil {
				return err
			}
			if relativeWorkingDirectory != ".." && !strings.HasPrefix(relativeWorkingDirectory, ".."+string(filepath.Separator)) {
				return fmt.Errorf("refuse to remove a directory containing the current working directory")
			}
			return os.RemoveAll(absolute)
		},
	}
}

func validateManagedRuntimeDirectory(plan InstallPlan) error {
	return validateManagedRuntimeDirectoryForPlatform(plan, currentNativePlatform())
}

func validateManagedRuntimeDirectoryForPlatform(plan InstallPlan, nativePlatform NativePlatform) error {
	runtimeDir, err := filepath.Abs(plan.RuntimeDir)
	if err != nil {
		return fmt.Errorf("resolve runtime directory: %w", err)
	}
	deploymentFiles, err := buildDeploymentFiles(plan)
	if err != nil {
		return fmt.Errorf("resolve managed deployment files: %w", err)
	}
	allowedFiles := map[string]struct{}{
		"deployment.json":                                   {},
		filepath.Join("config", "runtime.env"):              {},
		filepath.Join("config", "install-state.json"):       {},
		filepath.Join("config", ".mgsctl-install.lock"):     {},
		filepath.Join("config", ".mgsctl-import.lock"):      {},
		filepath.Join("config", ".mgsctl-setup-token.lock"): {},
		filepath.Join("config", "runtime.env.setup.lock"):   {},
		filepath.Join("config", "install-state.json.lock"):  {},
	}
	allowedDirectories := map[string]struct{}{
		"config": {}, "data": {}, "logs": {},
	}
	for _, file := range deploymentFiles {
		relative := filepath.Clean(file.RelativePath)
		allowedFiles[relative] = struct{}{}
		for parent := filepath.Dir(relative); parent != "."; parent = filepath.Dir(parent) {
			allowedDirectories[parent] = struct{}{}
		}
	}
	if plan.Mode == config.DeploymentModeNative {
		manifest, manifestExists, err := readNativeReleaseJournal(filepath.Join(runtimeDir, ".native-release.manifest.json"))
		if err != nil {
			return fmt.Errorf("read installed native release manifest: %w", err)
		}
		markerDigest, markerExists, err := readNativeReleaseMarker(filepath.Join(runtimeDir, ".native-release.sha256"))
		if err != nil {
			return fmt.Errorf("read installed native release marker: %w", err)
		}
		if !manifestExists || !markerExists || manifest.ArchiveSHA256 != markerDigest || manifest.PreviousArchiveSHA256 != "" || len(manifest.PreviousFiles) != 0 {
			return fmt.Errorf("installed native release manifest does not match its checksum marker")
		}
		if err := validateNativeReleaseTree(runtimeDir, manifest.Files); err != nil {
			return fmt.Errorf("validate installed native release tree: %w", err)
		}
		for relative := range manifest.Files {
			addManagedRuntimeFile(allowedFiles, allowedDirectories, filepath.FromSlash(relative))
		}
		runtimeContent, err := os.ReadFile(filepath.Join(runtimeDir, "config", "runtime.env"))
		if err != nil {
			return fmt.Errorf("read native runtime configuration: %w", err)
		}
		document, err := config.ParseRuntimeEnv(runtimeContent)
		if err != nil {
			return fmt.Errorf("parse native runtime configuration: %w", err)
		}
		serviceFiles, err := BuildNativeServiceFiles(plan, document.Values["INSTALLATION_ID"], nativePlatform)
		if err != nil {
			return fmt.Errorf("resolve managed native service files: %w", err)
		}
		for _, file := range serviceFiles {
			addManagedRuntimeFile(allowedFiles, allowedDirectories, file.RelativePath)
		}
		for _, name := range []string{".native-release.sha256", ".native-release.manifest.json", ".native-release.pending.json"} {
			allowedFiles[name] = struct{}{}
		}
	}

	return filepath.WalkDir(runtimeDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(runtimeDir, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		if pathWithinManagedTree(relative, "data") || pathWithinManagedTree(relative, "logs") {
			return nil
		}
		if entry.IsDir() {
			if _, ok := allowedDirectories[relative]; ok {
				return nil
			}
			return fmt.Errorf("runtime contains unmanaged directory %s", relative)
		}
		if _, ok := allowedFiles[relative]; ok || managedTemporaryConfigPath(relative) {
			return nil
		}
		return fmt.Errorf("runtime contains unmanaged file %s", relative)
	})
}

func addManagedRuntimeFile(files, directories map[string]struct{}, path string) {
	relative := filepath.Clean(path)
	files[relative] = struct{}{}
	for parent := filepath.Dir(relative); parent != "."; parent = filepath.Dir(parent) {
		directories[parent] = struct{}{}
	}
}

func pathWithinManagedTree(path, tree string) bool {
	return path == tree || strings.HasPrefix(path, tree+string(filepath.Separator))
}

func managedTemporaryConfigPath(path string) bool {
	directory, name := filepath.Split(path)
	if filepath.Clean(directory) != "config" {
		return false
	}
	return strings.HasPrefix(name, installStagePrefix) ||
		strings.HasPrefix(name, ".runtime.env.tmp-") || strings.HasPrefix(name, ".install-state.json.tmp-")
}

func executeDeploymentAction(ctx context.Context, plan InstallPlan, dockerAction DockerAction, nativeAction NativeAction, executors RuntimeExecutors) error {
	switch plan.Mode {
	case config.DeploymentModeDocker:
		return executors.Docker.Run(ctx, dockerAction, plan)
	case config.DeploymentModeNative:
		return executors.Native.Run(ctx, nativeAction, plan)
	default:
		return fmt.Errorf("unsupported deployment mode %q", plan.Mode)
	}
}
