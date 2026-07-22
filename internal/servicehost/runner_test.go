package servicehost

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestServiceHostRestartsDocumentedExitCodeUntilStopped(t *testing.T) {
	counterPath := filepath.Join(t.TempDir(), "counter")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	err := RunChild(ctx, ChildOptions{
		ServiceName: "app-test-api", WorkingDirectory: t.TempDir(), Executable: os.Args[0],
		Arguments:    []string{"-test.run=^TestServiceHostHelperProcess$"},
		LogDirectory: t.TempDir(), RestartExitCode: 75, RestartDelay: 5 * time.Millisecond,
		Environment: append(os.Environ(), "SERVICEHOST_HELPER=restart", "SERVICEHOST_COUNTER="+counterPath),
	})
	if err != nil {
		t.Fatalf("RunChild returned during controlled stop: %v", err)
	}
	content, err := os.ReadFile(counterPath)
	if err != nil {
		t.Fatal(err)
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil || count < 2 {
		t.Fatalf("restart count = %q, %v", content, err)
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
		path := os.Getenv("SERVICEHOST_COUNTER")
		count := 0
		if content, err := os.ReadFile(path); err == nil {
			count, _ = strconv.Atoi(strings.TrimSpace(string(content)))
		}
		_ = os.WriteFile(path, []byte(strconv.Itoa(count+1)), 0o600)
		os.Exit(75)
	case "fail":
		os.Exit(7)
	}
}
