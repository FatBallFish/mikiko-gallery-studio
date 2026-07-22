package deployctl

import (
	"context"
	"fmt"
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

func UpgradeDeploymentDependencies(executors RuntimeExecutors, migrate func(context.Context, string) error) UpgradeDependencies {
	return UpgradeDependencies{
		Migrate:       migrate,
		WriteManifest: writeDeploymentManifestPlan,
		ApplyDeployment: func(ctx context.Context, plan InstallPlan) error {
			return executeDeploymentAction(ctx, plan, DockerActionUpdate, NativeActionUpdate, executors)
		},
	}
}

func SetupTokenResetRuntimeDependencies(executors RuntimeExecutors) SetupTokenResetDependencies {
	return SetupTokenResetDependencies{
		RestartDeployment: func(ctx context.Context, plan InstallPlan) error {
			return executeDeploymentAction(ctx, plan, DockerActionRestart, NativeActionRestart, executors)
		},
	}
}

func UninstallRuntimeDependencies(executors RuntimeExecutors) UninstallDependencies {
	return UninstallDependencies{
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
