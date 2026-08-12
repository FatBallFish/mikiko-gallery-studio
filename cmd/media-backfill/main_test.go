package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/app"
)

func TestRunMediaBackfillCommandParsesOptionsAndWritesJSON(t *testing.T) {
	var gotPath string
	var gotOptions app.MediaAssetBackfillCommandOptions
	runner := func(_ context.Context, path string, options app.MediaAssetBackfillCommandOptions) (app.MediaAssetBackfillCommandReport, error) {
		gotPath, gotOptions = path, options
		return app.MediaAssetBackfillCommandReport{Mode: "dry_run", Completed: true}, nil
	}
	var output strings.Builder
	err := runMediaBackfillCommand(t.Context(), []string{
		"--env-file", "runtime.env", "--dry-run", "--verify", "--batch-size", "25", "--max-batches", "4", "--sample-size", "7",
	}, &output, runner)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "runtime.env" || !gotOptions.DryRun || !gotOptions.Verify || gotOptions.BatchSize != 25 || gotOptions.MaxBatches != 4 || gotOptions.SampleSize != 7 {
		t.Fatalf("path=%q options=%+v", gotPath, gotOptions)
	}
	var decoded app.MediaAssetBackfillCommandReport
	if err := json.Unmarshal([]byte(output.String()), &decoded); err != nil {
		t.Fatalf("decode output %q: %v", output.String(), err)
	}
	if decoded.Mode != "dry_run" || !decoded.Completed {
		t.Fatalf("decoded report = %+v", decoded)
	}
}

func TestRunMediaBackfillCommandRejectsInvalidBoundsBeforeRunner(t *testing.T) {
	called := false
	runner := func(context.Context, string, app.MediaAssetBackfillCommandOptions) (app.MediaAssetBackfillCommandReport, error) {
		called = true
		return app.MediaAssetBackfillCommandReport{}, errors.New("must not run")
	}
	for _, args := range [][]string{{"--batch-size", "0"}, {"--batch-size", "1001"}, {"--max-batches", "-1"}, {"--sample-size", "0"}} {
		if err := runMediaBackfillCommand(t.Context(), args, &strings.Builder{}, runner); err == nil {
			t.Fatalf("args %v unexpectedly accepted", args)
		}
	}
	if called {
		t.Fatal("runner called for invalid options")
	}
}
