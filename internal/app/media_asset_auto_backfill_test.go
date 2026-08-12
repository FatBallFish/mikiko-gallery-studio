package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/repository/db"
)

type automaticBackfillProbe struct {
	results []db.MediaAssetBackfillResult
	errors  []error
	calls   int
}

func (probe *automaticBackfillProbe) ProcessBatch(context.Context) (db.MediaAssetBackfillResult, error) {
	index := probe.calls
	probe.calls++
	var result db.MediaAssetBackfillResult
	if index < len(probe.results) {
		result = probe.results[index]
	}
	var err error
	if index < len(probe.errors) {
		err = probe.errors[index]
	}
	return result, err
}

func TestAutomaticMediaAssetBackfillContinuesWhileBatchesAreProcessed(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	probe := &automaticBackfillProbe{results: []db.MediaAssetBackfillResult{{Processed: 1}, {Processed: 1}}}
	processor := newAutomaticMediaAssetBackfill(probe, automaticMediaAssetBackfillOptions{
		now: func() time.Time { return now },
	})

	for index := range 2 {
		processed, err := processor.ProcessOnce(t.Context())
		if err != nil || !processed {
			t.Fatalf("ProcessOnce(%d) = %t, %v", index, processed, err)
		}
	}
	if probe.calls != 2 {
		t.Fatalf("underlying calls = %d, want 2", probe.calls)
	}
}

func TestAutomaticMediaAssetBackfillRechecksCompletedWorkAtLowFrequency(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	probe := &automaticBackfillProbe{results: []db.MediaAssetBackfillResult{{Done: true}, {Processed: 1}}}
	processor := newAutomaticMediaAssetBackfill(probe, automaticMediaAssetBackfillOptions{
		now:             func() time.Time { return now },
		recheckInterval: 6 * time.Hour,
	})

	processed, err := processor.ProcessOnce(t.Context())
	if err != nil || processed {
		t.Fatalf("initial ProcessOnce() = %t, %v", processed, err)
	}
	now = now.Add(6*time.Hour - time.Second)
	processed, err = processor.ProcessOnce(t.Context())
	if err != nil || processed || probe.calls != 1 {
		t.Fatalf("early recheck = %t, %v, calls %d", processed, err, probe.calls)
	}
	now = now.Add(time.Second)
	processed, err = processor.ProcessOnce(t.Context())
	if err != nil || !processed || probe.calls != 2 {
		t.Fatalf("due recheck = %t, %v, calls %d", processed, err, probe.calls)
	}
}

func TestAutomaticMediaAssetBackfillBacksOffFailuresAndResetsAfterSuccess(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	firstErr := errors.New("first failure")
	secondErr := errors.New("second failure")
	probe := &automaticBackfillProbe{
		results: []db.MediaAssetBackfillResult{{}, {}, {Processed: 1}, {}},
		errors:  []error{firstErr, secondErr, nil, firstErr},
	}
	processor := newAutomaticMediaAssetBackfill(probe, automaticMediaAssetBackfillOptions{
		now:          func() time.Time { return now },
		initialRetry: time.Minute,
		maxRetry:     4 * time.Minute,
	})

	if _, err := processor.ProcessOnce(t.Context()); !errors.Is(err, firstErr) {
		t.Fatalf("first error = %v", err)
	}
	now = now.Add(time.Minute - time.Second)
	if processed, err := processor.ProcessOnce(t.Context()); err != nil || processed || probe.calls != 1 {
		t.Fatalf("first backoff = %t, %v, calls %d", processed, err, probe.calls)
	}
	now = now.Add(time.Second)
	if _, err := processor.ProcessOnce(t.Context()); !errors.Is(err, secondErr) {
		t.Fatalf("second error = %v", err)
	}
	now = now.Add(2 * time.Minute)
	if processed, err := processor.ProcessOnce(t.Context()); err != nil || !processed {
		t.Fatalf("recovery = %t, %v", processed, err)
	}
	if _, err := processor.ProcessOnce(t.Context()); !errors.Is(err, firstErr) {
		t.Fatalf("error after reset = %v", err)
	}
	now = now.Add(time.Minute)
	if _, err := processor.ProcessOnce(t.Context()); err != nil {
		t.Fatalf("retry after reset = %v", err)
	}
	if probe.calls != 5 {
		t.Fatalf("underlying calls = %d, want 5", probe.calls)
	}
}

func TestAutomaticMediaAssetBackfillSchedulesRecheckFromTerminalBatch(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	probe := &automaticBackfillProbe{results: []db.MediaAssetBackfillResult{{Processed: 1, Created: 1, Done: true}, {Processed: 1}}}
	processor := newAutomaticMediaAssetBackfill(probe, automaticMediaAssetBackfillOptions{
		now:             func() time.Time { return now },
		recheckInterval: 6 * time.Hour,
	})

	processed, err := processor.ProcessOnce(t.Context())
	if err != nil || !processed {
		t.Fatalf("terminal batch = %t, %v", processed, err)
	}
	now = now.Add(time.Hour)
	processed, err = processor.ProcessOnce(t.Context())
	if err != nil || processed || probe.calls != 1 {
		t.Fatalf("early terminal recheck = %t, %v, calls %d", processed, err, probe.calls)
	}
}

func TestAutomaticMediaAssetBackfillRejectsMissingProcessor(t *testing.T) {
	processor := newAutomaticMediaAssetBackfill(nil, automaticMediaAssetBackfillOptions{})
	if _, err := processor.ProcessOnce(t.Context()); err == nil {
		t.Fatal("ProcessOnce() error = nil, want unavailable processor")
	}
}

func TestAutomaticMediaAssetBackfillCapsInitialRetryAtMaximum(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	probe := &automaticBackfillProbe{errors: []error{errors.New("failure")}}
	processor := newAutomaticMediaAssetBackfill(probe, automaticMediaAssetBackfillOptions{
		now:          func() time.Time { return now },
		initialRetry: 10 * time.Minute,
		maxRetry:     time.Minute,
	})

	if _, err := processor.ProcessOnce(t.Context()); err == nil {
		t.Fatal("first ProcessOnce() error = nil")
	}
	now = now.Add(time.Minute)
	if _, err := processor.ProcessOnce(t.Context()); err != nil {
		t.Fatalf("retry at maximum = %v", err)
	}
	if probe.calls != 2 {
		t.Fatalf("underlying calls = %d, want 2", probe.calls)
	}
}
