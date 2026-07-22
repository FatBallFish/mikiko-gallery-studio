package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/fatballfish/pic-gallery/internal/deployctl"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runner := deployctl.OSProcessRunner{Stdout: os.Stdout, Stderr: os.Stderr}
	dockerExecutor := deployctl.DockerExecutor{Runner: runner}
	nativeExecutor := deployctl.NativeExecutor{Runner: runner}
	os.Exit(deployctl.Run(ctx, os.Args[1:], deployctl.CLIDependencies{
		Terminal: deployctl.NewStdioTerminal(os.Stdin, os.Stderr), Stdout: os.Stdout, Stderr: os.Stderr,
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
	}))
}
