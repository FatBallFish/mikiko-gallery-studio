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
	dockerExecutor := deployctl.DockerExecutor{Runner: deployctl.OSProcessRunner{Stdout: os.Stdout, Stderr: os.Stderr}}
	os.Exit(deployctl.Run(ctx, os.Args[1:], deployctl.CLIDependencies{
		Terminal: deployctl.NewStdioTerminal(os.Stdin, os.Stderr), Stdout: os.Stdout, Stderr: os.Stderr,
		Install: deployctl.InstallDependencies{ApplyDeployment: func(ctx context.Context, plan deployctl.InstallPlan) error {
			if plan.Mode != "docker" {
				return fmt.Errorf("native service installation is not available in this build")
			}
			return dockerExecutor.Run(ctx, deployctl.DockerActionInstall, plan)
		}},
	}))
}
