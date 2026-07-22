package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/fatballfish/pic-gallery/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.RunWorkerContext(ctx); err != nil {
		code := app.ExitCode(err)
		if code != app.SupervisorRestartExitCode {
			log.Printf("pic-gallery worker exited with error: %v", err)
		}
		os.Exit(code)
	}
}
