package deployctl

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/setup"
)

type fakeTerminal struct {
	interactive bool
	answers     []string
	confirmed   bool
	prompts     int
}

func (terminal *fakeTerminal) Interactive() bool { return terminal.interactive }
func (terminal *fakeTerminal) Prompt(_ context.Context, _ string, fallback string) (string, error) {
	terminal.prompts++
	if len(terminal.answers) == 0 {
		return fallback, nil
	}
	answer := terminal.answers[0]
	terminal.answers = terminal.answers[1:]
	return answer, nil
}
func (terminal *fakeTerminal) Confirm(context.Context, string) (bool, error) {
	return terminal.confirmed, nil
}

func TestRunNonInteractiveInstallUsesNoTerminalAndInjectedFilesystemOnly(t *testing.T) {
	terminal := &fakeTerminal{interactive: true}
	stdout := new(bytes.Buffer)
	writes := make([]string, 0, 3)
	code := Run(context.Background(), []string{"install", "--mode", "docker", "--profile", "full", "--topology", "single", "--yes"}, CLIDependencies{
		Terminal: terminal, Stdout: stdout, Stderr: new(bytes.Buffer),
		Install: testInstallDependencies(&writes),
	})
	if code != 0 || terminal.prompts != 0 {
		t.Fatalf("Run(non-interactive) = %d, prompts %d, stdout %q", code, terminal.prompts, stdout.String())
	}
	if strings.Join(writes, ",") != "state,deployment.json,runtime.env" || strings.Contains(stdout.String(), "Setup token:") || !strings.Contains(stdout.String(), "deployctl setup token show") {
		t.Fatalf("install side effects/output = %v, %q", writes, stdout.String())
	}
}

func TestRunInteractiveInstallPromptsForMissingChoicesAndHonorsCancellation(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, answers: []string{"docker", "core", "single", "local"}, confirmed: false}
	writes := make([]string, 0)
	code := Run(context.Background(), []string{"install"}, CLIDependencies{
		Terminal: terminal, Stdout: new(bytes.Buffer), Stderr: new(bytes.Buffer), Install: testInstallDependencies(&writes),
	})
	if code != 0 || terminal.prompts != 12 || len(writes) != 0 {
		t.Fatalf("Run(cancelled interactive) = %d, prompts %d, writes %v", code, terminal.prompts, writes)
	}
}

func TestRunInteractiveInstallShowsTheOneTimeSetupToken(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, confirmed: true}
	stdout := new(bytes.Buffer)
	writes := make([]string, 0)
	code := Run(context.Background(), []string{"install"}, CLIDependencies{
		Terminal: terminal, Stdout: stdout, Stderr: new(bytes.Buffer), Install: testInstallDependencies(&writes),
		StdoutIsTerminal: func(io.Writer) bool { return true },
	})
	if code != 0 || !strings.Contains(stdout.String(), "Setup token:") {
		t.Fatalf("interactive install = %d, stdout %q", code, stdout.String())
	}
}

func TestRunInteractiveInstallDoesNotWriteTheTokenToRedirectedStdout(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, confirmed: true}
	stdout := new(bytes.Buffer)
	writes := make([]string, 0)
	code := Run(context.Background(), []string{"install"}, CLIDependencies{
		Terminal: terminal, Stdout: stdout, Stderr: new(bytes.Buffer), Install: testInstallDependencies(&writes),
	})
	if code != 0 || strings.Contains(stdout.String(), "Setup token:") || !strings.Contains(stdout.String(), "setup token show") {
		t.Fatalf("redirected interactive install = %d, stdout %q", code, stdout.String())
	}
}

func TestRunReturnsUsageCodeWithoutLeakingSensitiveArguments(t *testing.T) {
	stderr := new(bytes.Buffer)
	code := Run(context.Background(), []string{"cluster", "join", "--server", "ftp://invalid", "--token", "do-not-leak"}, CLIDependencies{Stderr: stderr})
	if code != 2 || strings.Contains(stderr.String(), "do-not-leak") {
		t.Fatalf("Run(invalid secret command) = %d, stderr %q", code, stderr.String())
	}
}

func testInstallDependencies(writes *[]string) InstallDependencies {
	return InstallDependencies{
		Entropy:       bytes.NewReader(bytes.Repeat([]byte{0x55}, 64)),
		Now:           func() time.Time { return time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC) },
		PathExists:    func(string) (bool, error) { return false, nil },
		MakeDirectory: func(string, os.FileMode) error { return nil },
		AcquireInstallLock: func(context.Context, string) (func() error, error) {
			return func() error { return nil }, nil
		},
		RecoverIncomplete: func(string, string, string) error { return nil },
		WriteRuntimeEnv:   func(string, []byte) error { *writes = append(*writes, "runtime.env"); return nil },
		InitializeState:   func(string, setup.InstallState) error { *writes = append(*writes, "state"); return nil },
		WriteManifest:     func(string, []byte) error { *writes = append(*writes, "deployment.json"); return nil },
	}
}

func TestExecuteInstallStopsBeforeEntropyOrWritesOnCollisionAndBeforeWritesOnEntropyFailure(t *testing.T) {
	plan, err := BuildInstallPlan(InstallInput{Mode: "docker", Profile: "core", Topology: "single", Role: "single", RuntimeDir: ".", StorageDriver: "local", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	directories := 0
	collisionEntropy := &countingReader{content: bytes.Repeat([]byte{0x44}, 32)}
	_, err = ExecuteInstall(context.Background(), plan, InstallDependencies{
		Entropy:       collisionEntropy,
		PathExists:    func(string) (bool, error) { return true, nil },
		MakeDirectory: func(string, os.FileMode) error { directories++; return nil },
		AcquireInstallLock: func(context.Context, string) (func() error, error) {
			return func() error { return nil }, nil
		},
		RecoverIncomplete: func(string, string, string) error { return nil },
	})
	if err == nil || directories != 2 || collisionEntropy.reads != 0 {
		t.Fatalf("collision = err %v, directories %d, entropy reads %d", err, directories, collisionEntropy.reads)
	}

	directories = 0
	_, err = ExecuteInstall(context.Background(), plan, InstallDependencies{
		Entropy:       errorReader("entropy unavailable"),
		PathExists:    func(string) (bool, error) { return false, nil },
		MakeDirectory: func(string, os.FileMode) error { directories++; return nil },
		AcquireInstallLock: func(context.Context, string) (func() error, error) {
			return func() error { return nil }, nil
		},
		RecoverIncomplete: func(string, string, string) error { return nil },
	})
	if err == nil || directories != 2 {
		t.Fatalf("entropy failure = err %v, directories %d", err, directories)
	}
}

func TestExecuteInstallHoldsAnExclusiveInstallLockAndRollsBackPartialArtifacts(t *testing.T) {
	plan, err := BuildInstallPlan(InstallInput{Mode: "docker", Profile: "core", Topology: "single", Role: "single", RuntimeDir: "runtime", StorageDriver: "local", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	events := make([]string, 0, 16)
	removed := make([]string, 0, 3)
	_, err = ExecuteInstall(context.Background(), plan, InstallDependencies{
		Entropy: bytes.NewReader(bytes.Repeat([]byte{0x44}, 32)),
		Now:     func() time.Time { return time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC) },
		MakeDirectory: func(path string, _ os.FileMode) error {
			events = append(events, "mkdir:"+path)
			return nil
		},
		AcquireInstallLock: func(_ context.Context, path string) (func() error, error) {
			events = append(events, "lock:"+path)
			return func() error { events = append(events, "unlock"); return nil }, nil
		},
		RecoverIncomplete: func(string, string, string) error { return nil },
		PathExists: func(path string) (bool, error) {
			events = append(events, "inspect:"+path)
			return false, nil
		},
		WriteRuntimeEnv: func(path string, _ []byte) error {
			events = append(events, "write:"+path)
			return nil
		},
		InitializeState: func(_ string, _ setup.InstallState) error {
			events = append(events, "write:state")
			return nil
		},
		WriteManifest: func(path string, _ []byte) error {
			events = append(events, "write:"+path)
			return errors.New("disk full")
		},
		RemovePath: func(path string) error {
			removed = append(removed, path)
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "write deployment manifest") {
		t.Fatalf("ExecuteInstall error = %v", err)
	}
	lockIndex := -1
	for index, event := range events {
		if strings.HasPrefix(event, "lock:") {
			lockIndex = index
			break
		}
	}
	if lockIndex < 0 || lockIndex+1 >= len(events) || !strings.HasPrefix(events[lockIndex+1], "inspect:") {
		t.Fatalf("install lock was not acquired before the final collision checks: %v", events)
	}
	if got := strings.Join(removed, ","); got != "runtime/config/install-state.json" {
		t.Fatalf("rollback paths = %q", got)
	}
	if events[len(events)-1] != "unlock" {
		t.Fatalf("install lock was not released: %v", events)
	}
}

func TestExecuteInstallHonorsCancellationAfterAcquiringTheInstallLock(t *testing.T) {
	plan, err := BuildInstallPlan(InstallInput{Mode: "docker", Profile: "core", Topology: "single", Role: "single", RuntimeDir: "runtime", StorageDriver: "local", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	writes := 0
	inspects := 0
	released := false
	_, err = ExecuteInstall(ctx, plan, InstallDependencies{
		Entropy:       bytes.NewReader(bytes.Repeat([]byte{0x44}, 32)),
		MakeDirectory: func(string, os.FileMode) error { return nil },
		AcquireInstallLock: func(context.Context, string) (func() error, error) {
			cancel()
			return func() error { released = true; return nil }, nil
		},
		PathExists:      func(string) (bool, error) { inspects++; return false, nil },
		WriteRuntimeEnv: func(string, []byte) error { writes++; return nil },
	})
	if !errors.Is(err, context.Canceled) || writes != 0 || inspects != 0 || !released {
		t.Fatalf("cancelled install = err %v, inspects %d, writes %d, released %t", err, inspects, writes, released)
	}
}

func TestExecuteInstallSecuresExistingRuntimeDirectories(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows directory ACLs are installed with native service support")
	}
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	for _, directory := range []string{runtimeDirectory, filepath.Join(runtimeDirectory, "config"), filepath.Join(runtimeDirectory, "data"), filepath.Join(runtimeDirectory, "logs")} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := BuildInstallPlan(InstallInput{Mode: "docker", Profile: "core", Topology: "single", Role: "single", RuntimeDir: runtimeDirectory, StorageDriver: "local", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteInstall(context.Background(), plan, InstallDependencies{Entropy: bytes.NewReader(bytes.Repeat([]byte{0x66}, 32))}); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{runtimeDirectory, filepath.Join(runtimeDirectory, "config"), filepath.Join(runtimeDirectory, "data"), filepath.Join(runtimeDirectory, "logs")} {
		info, err := os.Stat(directory)
		if err != nil {
			t.Fatal(err)
		}
		if permissions := info.Mode().Perm(); permissions != 0o700 {
			t.Errorf("%s permissions = %#o, want 0700", directory, permissions)
		}
	}
}

func TestExecuteInstallDoesNotDeleteAFileThatWinsANoClobberRace(t *testing.T) {
	plan, err := BuildInstallPlan(InstallInput{Mode: "docker", Profile: "core", Topology: "single", Role: "single", RuntimeDir: "runtime", StorageDriver: "local", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	removed := make([]string, 0)
	writes := 0
	_, err = ExecuteInstall(context.Background(), plan, InstallDependencies{
		Entropy:            bytes.NewReader(bytes.Repeat([]byte{0x77}, 32)),
		PathExists:         func(string) (bool, error) { return false, nil },
		MakeDirectory:      func(string, os.FileMode) error { return nil },
		RecoverIncomplete:  func(string, string, string) error { return nil },
		AcquireInstallLock: func(context.Context, string) (func() error, error) { return func() error { return nil }, nil },
		InitializeState:    func(string, setup.InstallState) error { writes++; return os.ErrExist },
		RemovePath:         func(path string) error { removed = append(removed, path); return nil },
	})
	if err == nil || writes != 1 || len(removed) != 0 {
		t.Fatalf("no-clobber race = err %v, writes %d, removed %v", err, writes, removed)
	}
}

type errorReader string

func (reader errorReader) Read([]byte) (int, error) { return 0, errors.New(string(reader)) }
