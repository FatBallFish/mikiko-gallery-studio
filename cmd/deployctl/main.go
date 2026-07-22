package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/fatballfish/pic-gallery/internal/deployctl"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(deployctl.Run(ctx, os.Args[1:], deployctl.CLIDependencies{
		Terminal: deployctl.NewStdioTerminal(os.Stdin, os.Stderr), Stdout: os.Stdout, Stderr: os.Stderr,
	}))
}
