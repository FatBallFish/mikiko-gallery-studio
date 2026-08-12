package app

import (
	"context"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
)

func TestNewVideoArtifactHTTPClientTrustsConfiguredTestCA(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "trusted")
	}))
	defer server.Close()
	certificate := server.Certificate()
	caFile := filepath.Join(t.TempDir(), "artifact-ca.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}

	client, err := newVideoArtifactHTTPClient(caFile)
	if err != nil {
		t.Fatalf("newVideoArtifactHTTPClient() error = %v", err)
	}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("configured client rejected test CA: %v", err)
	}
	_ = response.Body.Close()
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.ResponseHeaderTimeout <= 0 || transport.TLSHandshakeTimeout <= 0 || transport.IdleConnTimeout <= 0 {
		t.Fatalf("artifact transport timeouts are not configured: %#v", client.Transport)
	}
}

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

func TestWaitForWorkerMetricsLoopStopsWhenSupervisorCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error)
	result := make(chan error, 1)
	go func() { result <- waitForWorkerMetricsLoop(ctx, done) }()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("waitForWorkerMetricsLoop error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("metrics loop ignored supervisor cancellation")
	}
}

func TestWorkerOwnerReservesLeaseSuffixWithinSchemaLimit(t *testing.T) {
	owner := workerOwnerForHost("a-very-long-container-hostname-that-exceeds-the-media-processing-lease-owner-limit", "12345678-1234-1234-1234-123456789abc")
	for _, roleOwner := range []string{owner, owner + "-video", owner + "-media"} {
		if len(roleOwner) > 64 {
			t.Fatalf("lease owner %q has %d bytes, want at most 64", roleOwner, len(roleOwner))
		}
	}
	if !strings.HasSuffix(owner, "-12345678-1234-1234-1234-123456789abc") {
		t.Fatalf("worker owner lost its full UUID: %q", owner)
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

func TestRunWorkerRoleSlotsContinuesAfterTransientProcessorFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	processed := make(chan struct{}, 1)
	var calls atomic.Int32
	runOnce := func(context.Context) (bool, error) {
		if calls.Add(1) == 1 {
			return false, errors.New("temporary database failure")
		}
		select {
		case processed <- struct{}{}:
		default:
		}
		return false, nil
	}
	done := make(chan error, 1)
	go func() { done <- runWorkerRoleSlots(ctx, "video", 1, time.Millisecond, runOnce) }()

	select {
	case <-processed:
	case err := <-done:
		t.Fatalf("role loop stopped after a transient processor failure: %v", err)
	case <-time.After(time.Second):
		t.Fatal("role loop did not retry after a transient processor failure")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("runWorkerRoleSlots error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("role loop did not stop after cancellation")
	}
}
