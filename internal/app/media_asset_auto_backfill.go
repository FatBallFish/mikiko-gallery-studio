package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/fatballfish/pic-gallery/internal/repository/db"
)

const (
	defaultMediaAssetBackfillRecheckInterval = 6 * time.Hour
	defaultMediaAssetBackfillInitialRetry    = 5 * time.Second
	defaultMediaAssetBackfillMaxRetry        = 5 * time.Minute
)

type automaticMediaAssetBackfillOptions struct {
	now             func() time.Time
	recheckInterval time.Duration
	initialRetry    time.Duration
	maxRetry        time.Duration
	observer        mediaAssetBackfillObserver
}

type mediaAssetBackfillObserver interface {
	RecordMediaAssetBackfill(result string)
}

type mediaAssetBackfillBatchProcessor interface {
	ProcessBatch(context.Context) (db.MediaAssetBackfillResult, error)
}

type automaticMediaAssetBackfill struct {
	processor mediaAssetBackfillBatchProcessor
	options   automaticMediaAssetBackfillOptions

	mu                  sync.Mutex
	nextAttempt         time.Time
	consecutiveFailures int
}

func newAutomaticMediaAssetBackfill(processor mediaAssetBackfillBatchProcessor, options automaticMediaAssetBackfillOptions) *automaticMediaAssetBackfill {
	if options.now == nil {
		options.now = time.Now
	}
	if options.recheckInterval <= 0 {
		options.recheckInterval = defaultMediaAssetBackfillRecheckInterval
	}
	if options.initialRetry <= 0 {
		options.initialRetry = defaultMediaAssetBackfillInitialRetry
	}
	if options.maxRetry <= 0 {
		options.maxRetry = defaultMediaAssetBackfillMaxRetry
	}
	if options.initialRetry > options.maxRetry {
		options.initialRetry = options.maxRetry
	}
	return &automaticMediaAssetBackfill{processor: processor, options: options}
}

func (processor *automaticMediaAssetBackfill) ProcessOnce(ctx context.Context) (bool, error) {
	if processor == nil || processor.processor == nil {
		return false, fmt.Errorf("automatic media asset backfill processor is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}

	processor.mu.Lock()
	defer processor.mu.Unlock()
	now := processor.options.now()
	if !processor.nextAttempt.IsZero() && now.Before(processor.nextAttempt) {
		return false, nil
	}

	result, err := processor.processor.ProcessBatch(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false, err
		}
		processor.consecutiveFailures++
		retryDelay := exponentialBackoff(processor.options.initialRetry, processor.options.maxRetry, processor.consecutiveFailures)
		processor.nextAttempt = now.Add(retryDelay)
		if processor.options.observer != nil {
			processor.options.observer.RecordMediaAssetBackfill("failed")
		}
		slog.WarnContext(ctx, "automatic media asset backfill batch failed",
			"event", "media_asset_backfill_failed",
			"consecutive_failures", processor.consecutiveFailures,
			"retry_in", retryDelay,
			"error", err,
		)
		return false, err
	}

	processor.consecutiveFailures = 0
	processed := result.Processed > 0
	if processed {
		if processor.options.observer != nil {
			processor.options.observer.RecordMediaAssetBackfill("processed")
		}
		slog.InfoContext(ctx, "automatic media asset backfill batch completed",
			"event", "media_asset_backfill_batch_completed",
			"processed", result.Processed,
			"created", result.Created,
			"skipped", result.Skipped,
			"done", result.Done,
		)
	}
	if result.Done {
		processor.nextAttempt = now.Add(processor.options.recheckInterval)
		if processor.options.observer != nil {
			processor.options.observer.RecordMediaAssetBackfill("completed")
		}
		slog.InfoContext(ctx, "automatic media asset backfill is complete",
			"event", "media_asset_backfill_completed",
			"recheck_in", processor.options.recheckInterval,
		)
	} else {
		processor.nextAttempt = time.Time{}
	}
	return processed, nil
}

func exponentialBackoff(initial, maximum time.Duration, attempt int) time.Duration {
	if attempt <= 1 {
		return initial
	}
	delay := initial
	for range attempt - 1 {
		if delay >= maximum/2 {
			return maximum
		}
		delay *= 2
	}
	if delay > maximum {
		return maximum
	}
	return delay
}
