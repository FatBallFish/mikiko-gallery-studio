package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/fatballfish/pic-gallery/internal/app"
	"github.com/fatballfish/pic-gallery/internal/mgsctl"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
	dirty     = "false"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runner := mgsctl.OSProcessRunner{Stdout: os.Stdout, Stderr: os.Stderr}
	dockerExecutor := mgsctl.DockerExecutor{Runner: runner, Stderr: os.Stderr}
	nativeExecutor := mgsctl.NativeExecutor{Runner: runner}
	executors := mgsctl.RuntimeExecutors{Docker: dockerExecutor, Native: nativeExecutor}
	userConfig := mgsctl.UserConfigDependencies{UserConfigDir: os.UserConfigDir}
	os.Exit(mgsctl.Run(ctx, os.Args[1:], mgsctl.CLIDependencies{
		Terminal: mgsctl.NewStdioTerminal(os.Stdin, os.Stderr), Stdout: os.Stdout, Stderr: os.Stderr,
		ExecuteTUI: func(ctx context.Context) ([]string, error) {
			return mgsctl.ExecuteTUI(ctx, os.Stdin, os.Stdout)
		},
		BuildInfo: mgsctl.BuildInfo{Version: version, Commit: commit, BuildTime: buildTime, Dirty: strings.EqualFold(dirty, "true")},
		ResolveRuntime: func(options mgsctl.RuntimeResolutionOptions) (string, error) {
			return mgsctl.ResolveRuntimeDirectory(options, mgsctl.RuntimeResolverDependencies{UserConfig: userConfig})
		},
		RememberRuntime: func(runtimeDir string) error { return mgsctl.SaveRecentRuntime(userConfig, runtimeDir) },
		UserConfig:      userConfig,
		ImportConfig:    mgsctl.ImportConfigDependencies{ProbeCompletion: mgsctl.ProbeLegacyCompletion},
		Doctor:          mgsctl.ProductionDoctorDependencies(),
		SelfUpdate:      mgsctl.ProductionSelfUpdateDependencies(),
		Upgrade: mgsctl.UpgradeDeploymentDependencies(executors, func(ctx context.Context, runtimeEnvPath string) error {
			_, err := app.RunDatabaseMigration(ctx, runtimeEnvPath)
			return err
		}),
		Uninstall:       mgsctl.UninstallRuntimeDependencies(executors),
		SetupTokenReset: mgsctl.SetupTokenResetRuntimeDependencies(executors),
		ExecuteRuntimeAction: func(ctx context.Context, kind mgsctl.CommandKind, runtimeDir string) error {
			return mgsctl.ExecuteRuntimeAction(ctx, kind, runtimeDir, executors)
		},
		CreateClusterToken: mgsctl.CreateClusterToken,
		Install: mgsctl.InstallDependencies{ApplyDeployment: func(ctx context.Context, plan mgsctl.InstallPlan) error {
			switch plan.Mode {
			case "docker":
				return dockerExecutor.Run(ctx, mgsctl.DockerActionInstall, plan)
			case "native":
				return nativeExecutor.Run(ctx, mgsctl.NativeActionInstall, plan)
			default:
				return fmt.Errorf("unsupported deployment mode %q", plan.Mode)
			}
		}},
		ClusterJoin: mgsctl.ClusterJoinDependencies{
			PreflightDeployment: func(ctx context.Context, plan mgsctl.InstallPlan) error {
				switch plan.Mode {
				case "docker":
					return dockerExecutor.Preflight(ctx, plan)
				case "native":
					return nativeExecutor.Preflight(ctx, plan)
				default:
					return fmt.Errorf("unsupported deployment mode %q", plan.Mode)
				}
			},
			ApplyDeployment: func(ctx context.Context, plan mgsctl.InstallPlan) error {
				switch plan.Mode {
				case "docker":
					return dockerExecutor.Run(ctx, mgsctl.DockerActionInstall, plan)
				case "native":
					return nativeExecutor.Run(ctx, mgsctl.NativeActionInstall, plan)
				default:
					return fmt.Errorf("unsupported deployment mode %q", plan.Mode)
				}
			},
		},
	}))
}
