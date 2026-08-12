package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
)

func TestRunIndependentWorkerLoopsRunsVideoWithoutImageSlots(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	var imageStarted, videoStarted atomic.Bool
	done := make(chan error, 1)
	go func() {
		done <- runIndependentWorkerLoops(ctx,
			func(ctx context.Context) error { imageStarted.Store(true); <-ctx.Done(); return ctx.Err() },
			func(ctx context.Context) error { videoStarted.Store(true); cancel(); return nil },
		)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runIndependentWorkerLoops() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("worker loops did not stop")
	}
	if !imageStarted.Load() || !videoStarted.Load() {
		t.Fatalf("started image=%v video=%v", imageStarted.Load(), videoStarted.Load())
	}
}

func TestRunIndependentWorkerLoopsPropagatesFailureAndCancelsSibling(t *testing.T) {
	want := errors.New("video failed")
	siblingStopped := make(chan struct{})
	err := runIndependentWorkerLoops(t.Context(),
		func(ctx context.Context) error { <-ctx.Done(); close(siblingStopped); return ctx.Err() },
		func(context.Context) error { return want },
	)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	select {
	case <-siblingStopped:
	case <-time.After(time.Second):
		t.Fatal("sibling loop was not cancelled")
	}
}

func TestValidateWorkerRuntimeRequiresMediaToolsOnlyForMediaRole(t *testing.T) {
	tempDir := filepath.Join(t.TempDir(), "media")
	cfg := config.WorkerConfig{
		Roles: []config.WorkerRole{config.WorkerRoleMedia}, FFmpegPath: "custom-ffmpeg", FFprobePath: "custom-ffprobe", TempDir: tempDir,
		TempDiskPausePercent: 75, TempDiskCriticalPercent: 90,
	}
	checks := workerRuntimeChecks{
		lookPath: func(name string) (string, error) {
			if name == "custom-ffmpeg" {
				return "", errors.New("missing")
			}
			return "/usr/bin/" + name, nil
		},
		mkdirAll: os.MkdirAll,
	}
	if err := validateWorkerRuntime(cfg, checks); err == nil || !strings.Contains(err.Error(), "FFmpeg") {
		t.Fatalf("validateWorkerRuntime missing FFmpeg error = %v", err)
	}

	cfg.Roles = []config.WorkerRole{config.WorkerRoleVideo}
	if err := validateWorkerRuntime(cfg, checks); err != nil {
		t.Fatalf("video-only Worker required media tools: %v", err)
	}
}

func TestRunWorkerRoleSlotsKeepsConcurrencyIndependent(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	var current, peak atomic.Int32
	runOnce := func(context.Context) (bool, error) {
		now := current.Add(1)
		for {
			prior := peak.Load()
			if now <= prior || peak.CompareAndSwap(prior, now) {
				break
			}
		}
		started <- struct{}{}
		<-release
		current.Add(-1)
		return true, nil
	}
	done := make(chan error, 1)
	go func() { done <- runWorkerRoleSlots(ctx, "video", 3, time.Millisecond, runOnce) }()
	for range 3 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("role slots did not reach configured concurrency")
		}
	}
	if got := peak.Load(); got != 3 {
		t.Fatalf("peak concurrency = %d, want 3", got)
	}
	close(release)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("runWorkerRoleSlots error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("role slots did not stop after cancellation")
	}
}
