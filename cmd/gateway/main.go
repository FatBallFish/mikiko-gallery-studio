package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/gateway"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		log.Printf("Gateway exited with error: %v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	bootstrap, err := config.LoadBootstrap("")
	if err != nil {
		return fmt.Errorf("load Gateway runtime configuration: %w", err)
	}
	gatewayConfig, err := gateway.ConfigFromBootstrap(bootstrap, ".")
	if err != nil {
		return fmt.Errorf("configure Gateway: %w", err)
	}
	return gateway.ListenAndServe(ctx, gatewayConfig)
}
