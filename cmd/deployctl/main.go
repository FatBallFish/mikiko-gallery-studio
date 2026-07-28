package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/fatballfish/pic-gallery/internal/app"
	"github.com/fatballfish/pic-gallery/internal/deployctl"
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
	runner := deployctl.OSProcessRunner{Stdout: os.Stdout, Stderr: os.Stderr}
	dockerExecutor := deployctl.DockerExecutor{Runner: runner}
	nativeExecutor := deployctl.NativeExecutor{Runner: runner}
	executors := deployctl.RuntimeExecutors{Docker: dockerExecutor, Native: nativeExecutor}
	os.Exit(deployctl.Run(ctx, os.Args[1:], deployctl.CLIDependencies{
		Terminal: deployctl.NewStdioTerminal(os.Stdin, os.Stderr), Stdout: os.Stdout, Stderr: os.Stderr,
		BuildInfo:    deployctl.BuildInfo{Version: version, Commit: commit, BuildTime: buildTime, Dirty: strings.EqualFold(dirty, "true")},
		ImportConfig: deployctl.ImportConfigDependencies{ProbeCompletion: deployctl.ProbeLegacyCompletion},
		Doctor:       deployctl.ProductionDoctorDependencies(),
		SelfUpdate:   deployctl.ProductionSelfUpdateDependencies(),
		Upgrade: deployctl.UpgradeDeploymentDependencies(executors, func(ctx context.Context, runtimeEnvPath string) error {
			_, err := app.RunDatabaseMigration(ctx, runtimeEnvPath)
			return err
		}),
		Uninstall:       deployctl.UninstallRuntimeDependencies(executors),
		SetupTokenReset: deployctl.SetupTokenResetRuntimeDependencies(executors),
		ExecuteRuntimeAction: func(ctx context.Context, kind deployctl.CommandKind, runtimeDir string) error {
			return deployctl.ExecuteRuntimeAction(ctx, kind, runtimeDir, executors)
		},
		CreateClusterToken: deployctl.CreateClusterToken,
		Install: deployctl.InstallDependencies{ApplyDeployment: func(ctx context.Context, plan deployctl.InstallPlan) error {
			switch plan.Mode {
			case "docker":
				return dockerExecutor.Run(ctx, deployctl.DockerActionInstall, plan)
			case "native":
				return nativeExecutor.Run(ctx, deployctl.NativeActionInstall, plan)
			default:
				return fmt.Errorf("unsupported deployment mode %q", plan.Mode)
			}
		}},
		ClusterJoin: deployctl.ClusterJoinDependencies{
			PreflightDeployment: func(ctx context.Context, plan deployctl.InstallPlan) error {
				switch plan.Mode {
				case "docker":
					return dockerExecutor.Preflight(ctx, plan)
				case "native":
					return nativeExecutor.Preflight(ctx, plan)
				default:
					return fmt.Errorf("unsupported deployment mode %q", plan.Mode)
				}
			},
			ApplyDeployment: func(ctx context.Context, plan deployctl.InstallPlan) error {
				switch plan.Mode {
				case "docker":
					return dockerExecutor.Run(ctx, deployctl.DockerActionInstall, plan)
				case "native":
					return nativeExecutor.Run(ctx, deployctl.NativeActionInstall, plan)
				default:
					return fmt.Errorf("unsupported deployment mode %q", plan.Mode)
				}
			},
		},
	}))
}
