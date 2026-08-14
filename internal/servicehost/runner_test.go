package servicehost

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestServiceHostRestartsDocumentedExitCodeUntilStopped(t *testing.T) {
	markerDirectory := t.TempDir()
	workingDirectory := t.TempDir()
	logDirectory := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- RunChild(ctx, ChildOptions{
			ServiceName: "app-test-api", WorkingDirectory: workingDirectory, Executable: os.Args[0],
			Arguments:    []string{"-test.run=^TestServiceHostHelperProcess$"},
			LogDirectory: logDirectory, RestartExitCode: 75, RestartDelay: 5 * time.Millisecond,
			Environment: append(os.Environ(), "SERVICEHOST_HELPER=restart", "SERVICEHOST_MARKER_DIR="+markerDirectory),
		})
	}()

	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	restartCount := 0
	for restartCount < 2 {
		entries, err := os.ReadDir(markerDirectory)
		if err != nil {
			t.Fatalf("read restart markers: %v", err)
		}
		restartCount = 0
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "restart-") {
				restartCount++
			}
		}
		if restartCount >= 2 {
			break
		}
		select {
		case err := <-result:
			t.Fatalf("RunChild returned after %d restarts: %v", restartCount, err)
		case <-deadline.C:
			t.Fatalf("timed out waiting for two restarts; observed %d", restartCount)
		case <-ticker.C:
		}
	}

	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("RunChild returned during controlled stop: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for RunChild to stop")
	}
}

func TestServiceHostReturnsOrdinaryChildFailureForSCMRecovery(t *testing.T) {
	err := RunChild(context.Background(), ChildOptions{
		ServiceName: "app-test-worker", WorkingDirectory: t.TempDir(), Executable: os.Args[0],
		Arguments:    []string{"-test.run=^TestServiceHostHelperProcess$"},
		LogDirectory: t.TempDir(), RestartExitCode: 75,
		Environment: append(os.Environ(), "SERVICEHOST_HELPER=fail"),
	})
	if err == nil || !strings.Contains(err.Error(), "exit code 7") {
		t.Fatalf("ordinary child failure = %v", err)
	}
}

func TestServiceHostValidatesPathsAndServiceName(t *testing.T) {
	valid := ChildOptions{
		ServiceName: "app-test-api", WorkingDirectory: t.TempDir(), Executable: os.Args[0], LogDirectory: t.TempDir(), RestartExitCode: 75,
	}
	tests := []ChildOptions{valid, valid, valid}
	tests[0].ServiceName = "../escape"
	tests[1].WorkingDirectory = ""
	tests[2].Executable = ""
	for _, options := range tests {
		if err := Validate(options); err == nil {
			t.Fatalf("Validate accepted %#v", options)
		}
	}
}

func TestServiceHostHelperProcess(t *testing.T) {
	switch os.Getenv("SERVICEHOST_HELPER") {
	case "restart":
		marker, err := os.CreateTemp(os.Getenv("SERVICEHOST_MARKER_DIR"), "restart-")
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "create restart marker: %v\n", err)
			os.Exit(8)
		}
		if err := marker.Close(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "close restart marker: %v\n", err)
			os.Exit(8)
		}
		os.Exit(75)
	case "fail":
		os.Exit(7)
	}
}
