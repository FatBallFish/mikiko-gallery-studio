package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/fatballfish/pic-gallery/internal/app"
)

type mediaBackfillRunner func(context.Context, string, app.MediaAssetBackfillCommandOptions) (app.MediaAssetBackfillCommandReport, error)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runMediaBackfillCommand(ctx, os.Args[1:], os.Stdout, app.RunMediaAssetBackfill); err != nil {
		log.Printf("media asset backfill failed: %v", err)
		os.Exit(1)
	}
}

func runMediaBackfillCommand(ctx context.Context, args []string, output io.Writer, runner mediaBackfillRunner) error {
	flags := flag.NewFlagSet("mikiko-gallery-studio-media-backfill", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	runtimeEnvPath := flags.String("env-file", "", "runtime env path (defaults to APP_ENV_FILE or ./config/runtime.env)")
	dryRun := flags.Bool("dry-run", false, "plan the backfill without writing assets or a checkpoint")
	verify := flags.Bool("verify", false, "verify aggregate counts, bytes, and deterministic samples")
	batchSize := flags.Int("batch-size", 100, "rows per transaction (1-1000)")
	maxBatches := flags.Int("max-batches", 0, "maximum batches in this invocation; 0 means until complete")
	sampleSize := flags.Int("sample-size", 20, "deterministic verification sample size (1-1000)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}
	if *batchSize < 1 || *batchSize > 1000 {
		return fmt.Errorf("batch-size must be between 1 and 1000")
	}
	if *maxBatches < 0 {
		return fmt.Errorf("max-batches must be non-negative")
	}
	if *sampleSize < 1 || *sampleSize > 1000 {
		return fmt.Errorf("sample-size must be between 1 and 1000")
	}
	if runner == nil {
		return fmt.Errorf("media asset backfill runner is required")
	}
	report, err := runner(ctx, *runtimeEnvPath, app.MediaAssetBackfillCommandOptions{DryRun: *dryRun, Verify: *verify, BatchSize: *batchSize, MaxBatches: *maxBatches, SampleSize: *sampleSize})
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("write media asset backfill report: %w", err)
	}
	return nil
}
