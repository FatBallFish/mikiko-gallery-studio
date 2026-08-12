package app

import (
	"context"
	"fmt"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
)

type MediaAssetBackfillCommandOptions struct {
	DryRun     bool
	Verify     bool
	BatchSize  int
	MaxBatches int
	SampleSize int
}

type MediaAssetBackfillCommandReport struct {
	Mode         string                             `json:"mode"`
	Batches      int                                `json:"batches"`
	RunProcessed int                                `json:"run_processed"`
	Processed    int                                `json:"processed"`
	Created      int                                `json:"created"`
	WouldCreate  int                                `json:"would_create"`
	Skipped      int                                `json:"skipped"`
	Completed    bool                               `json:"completed"`
	Checkpoint   db.MediaAssetBackfillCheckpoint    `json:"checkpoint"`
	Verification *db.MediaAssetBackfillVerification `json:"verification,omitempty"`
}

type mediaAssetBackfillDependencies struct {
	loadRuntime              func(string) (config.Config, error)
	openDatabase             func(context.Context, string) (*repoent.Client, error)
	closeDatabase            func(*repoent.Client) error
	checkSchemaCompatibility func(context.Context, *repoent.Client, config.Config) error
}

func RunMediaAssetBackfill(ctx context.Context, runtimeEnvPath string, options MediaAssetBackfillCommandOptions) (MediaAssetBackfillCommandReport, error) {
	return runMediaAssetBackfill(ctx, runtimeEnvPath, options, mediaAssetBackfillDependencies{})
}

func runMediaAssetBackfill(ctx context.Context, runtimeEnvPath string, options MediaAssetBackfillCommandOptions, dependencies mediaAssetBackfillDependencies) (MediaAssetBackfillCommandReport, error) {
	if dependencies.loadRuntime == nil {
		dependencies.loadRuntime = config.LoadRuntime
	}
	if dependencies.openDatabase == nil {
		dependencies.openDatabase = db.OpenContext
	}
	if dependencies.closeDatabase == nil {
		dependencies.closeDatabase = func(client *repoent.Client) error { return client.Close() }
	}
	if dependencies.checkSchemaCompatibility == nil {
		dependencies.checkSchemaCompatibility = checkRuntimeSchemaCompatibility
	}
	cfg, err := dependencies.loadRuntime(runtimeEnvPath)
	if err != nil {
		return MediaAssetBackfillCommandReport{}, fmt.Errorf("load runtime configuration for media asset backfill: %w", err)
	}
	if err := validateDatabaseMigrationRole(cfg.Runtime.DeploymentRole); err != nil {
		return MediaAssetBackfillCommandReport{}, err
	}
	client, err := dependencies.openDatabase(ctx, cfg.Database.URL)
	if err != nil {
		return MediaAssetBackfillCommandReport{}, fmt.Errorf("open media asset backfill database: %w", err)
	}
	defer func() { _ = dependencies.closeDatabase(client) }()
	if err := dependencies.checkSchemaCompatibility(ctx, client, cfg); err != nil {
		return MediaAssetBackfillCommandReport{}, err
	}

	report := MediaAssetBackfillCommandReport{Mode: "apply"}
	if options.DryRun {
		report.Mode = "dry_run"
		cursor := db.MediaAssetBackfillCheckpoint{}
		for options.MaxBatches == 0 || report.Batches < options.MaxBatches {
			result, err := db.BackfillMediaAssets(ctx, client, db.MediaAssetBackfillOptions{BatchSize: options.BatchSize, Checkpoint: cursor, DryRun: true})
			if err != nil {
				return MediaAssetBackfillCommandReport{}, fmt.Errorf("dry-run media asset backfill: %w", err)
			}
			report.Batches++
			report.RunProcessed += result.Processed
			report.Processed += result.Processed
			report.WouldCreate += result.WouldCreate
			report.Skipped += result.Skipped
			report.Checkpoint = result.Checkpoint
			cursor = result.Checkpoint
			if result.Done {
				report.Completed = true
				break
			}
		}
	} else {
		processor := db.NewMediaAssetBackfillProcessor(client, db.MediaAssetBackfillProcessorOptions{BatchSize: options.BatchSize})
		for options.MaxBatches == 0 || report.Batches < options.MaxBatches {
			result, err := processor.ProcessBatch(ctx)
			if err != nil {
				return MediaAssetBackfillCommandReport{}, fmt.Errorf("apply media asset backfill: %w", err)
			}
			if result.Processed > 0 {
				report.Batches++
				report.RunProcessed += result.Processed
				report.Created += result.Created
				report.Skipped += result.Skipped
				report.Checkpoint = result.Checkpoint
			}
			if result.Done {
				report.Completed = true
				break
			}
		}
		progress, err := db.ReadMediaAssetBackfillProgress(ctx, client)
		if err != nil {
			return MediaAssetBackfillCommandReport{}, err
		}
		report.Processed = progress.Processed
		report.Completed = progress.Completed
	}
	if options.Verify {
		verification, err := db.VerifyMediaAssetBackfill(ctx, client, db.MediaAssetBackfillVerifyOptions{SampleSize: options.SampleSize})
		if err != nil {
			return MediaAssetBackfillCommandReport{}, fmt.Errorf("verify media asset backfill: %w", err)
		}
		report.Verification = &verification
	}
	return report, nil
}
