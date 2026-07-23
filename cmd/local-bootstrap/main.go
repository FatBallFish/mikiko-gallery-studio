package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/fatballfish/pic-gallery/internal/localbootstrap"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

func main() {
	if err := run(); err != nil {
		log.Printf("local runtime bootstrap failed: %v", err)
		os.Exit(1)
	}
}

func run() error {
	runtimeEnvPath := flag.String("env-file", "", "runtime env path (defaults to APP_ENV_FILE or ./config/runtime.env)")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result, err := localbootstrap.Run(ctx, *runtimeEnvPath)
	if err != nil {
		return err
	}
	if err := protectLocalRuntimeFiles(result.RuntimePath); err != nil {
		return err
	}
	fmt.Println(formatLocalBootstrapResult(result))
	return nil
}

func formatLocalBootstrapResult(result localbootstrap.Result) string {
	return fmt.Sprintf("local runtime bootstrap complete: installation=%s administrator=%q changed=%t", result.Binding.InstallationID, result.Binding.AdminEmail, result.Migration.Changed)
}

func protectLocalRuntimeFiles(runtimePath string) error {
	statePath := setup.StatePathForRuntimeEnv(runtimePath)
	for _, path := range []string{runtimePath, statePath} {
		if err := os.Chmod(path, 0o600); err != nil {
			return fmt.Errorf("protect local runtime file: %w", err)
		}
	}
	for _, path := range []string{statePath, statePath + ".lock"} {
		if err := restoreLocalFileOwnership(runtimePath, path); err != nil {
			if os.IsNotExist(err) && path != statePath {
				continue
			}
			return fmt.Errorf("restore local runtime file ownership: %w", err)
		}
	}
	return nil
}
