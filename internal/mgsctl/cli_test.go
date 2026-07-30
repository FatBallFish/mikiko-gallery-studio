package mgsctl

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

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

type fakeTerminal struct {
	interactive    bool
	answers        []string
	confirmed      bool
	confirmAnswers []bool
	prompts        int
	confirmations  int
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
	terminal.confirmations++
	if len(terminal.confirmAnswers) > 0 {
		answer := terminal.confirmAnswers[0]
		terminal.confirmAnswers = terminal.confirmAnswers[1:]
		return answer, nil
	}
	return terminal.confirmed, nil
}

func TestRunHelpAndNonTTYNoArgsExitSuccessfully(t *testing.T) {
	invocations := [][]string{nil, {"-h"}, {"--help"}}
	var expected string
	for _, args := range invocations {
		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)
		code := Run(context.Background(), args, CLIDependencies{
			Terminal: &fakeTerminal{interactive: false},
			Stdout:   stdout,
			Stderr:   stderr,
		})
		if code != 0 || stderr.Len() != 0 {
			t.Fatalf("Run(%v) = %d, stderr %q", args, code, stderr.String())
		}
		if expected == "" {
			expected = stdout.String()
		}
		if stdout.String() != expected {
			t.Fatalf("Run(%v) help differs from no-argument help:\n%s", args, stdout.String())
		}
		if !strings.Contains(stdout.String(), "mgsctl install") || !strings.Contains(stdout.String(), "mgsctl cluster join") {
			t.Fatalf("Run(%v) output is not catalog help: %q", args, stdout.String())
		}
	}
}

func TestRunHelpPreservesUsageErrorsForUnrelatedArguments(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		terminal Terminal
	}{
		{name: "unknown command", args: []string{"unknown"}, terminal: &fakeTerminal{interactive: false}},
		{name: "extra help argument", args: []string{"--help", "unexpected"}, terminal: &fakeTerminal{interactive: false}},
		{name: "nested help", args: []string{"status", "--help"}, terminal: &fakeTerminal{interactive: false}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			code := Run(context.Background(), testCase.args, CLIDependencies{
				Terminal: testCase.terminal,
				Stdout:   stdout,
				Stderr:   stderr,
			})
			if code != 2 || stdout.Len() != 0 || stderr.Len() == 0 {
				t.Fatalf("Run(%v) = %d, stdout %q, stderr %q", testCase.args, code, stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunNoArgsUsesTUIOnlyWhenInputAndOutputAreTerminals(t *testing.T) {
	t.Run("interactive input and output", func(t *testing.T) {
		called := 0
		stdout := new(bytes.Buffer)
		code := Run(context.Background(), nil, CLIDependencies{
			Terminal: &fakeTerminal{interactive: true}, Stdout: stdout, Stderr: new(bytes.Buffer),
			StdoutIsTerminal: func(io.Writer) bool { return true },
			ExecuteTUI: func(context.Context) ([]string, error) {
				called++
				return []string{"version"}, nil
			},
			BuildInfo: BuildInfo{Version: "v1"},
		})
		if code != 0 || called != 1 || !strings.Contains(stdout.String(), "mgsctl v1") {
			t.Fatalf("Run(no args)=%d called=%d stdout=%q", code, called, stdout.String())
		}
	})

	t.Run("non-terminal output", func(t *testing.T) {
		called := 0
		stdout := new(bytes.Buffer)
		code := Run(context.Background(), nil, CLIDependencies{
			Terminal: &fakeTerminal{interactive: true}, Stdout: stdout, Stderr: new(bytes.Buffer),
			StdoutIsTerminal: func(io.Writer) bool { return false },
			ExecuteTUI:       func(context.Context) ([]string, error) { called++; return nil, nil },
		})
		if code != 0 || called != 0 || stdout.String() != HelpText() {
			t.Fatalf("Run(no args)=%d called=%d stdout=%q", code, called, stdout.String())
		}
	})

	t.Run("exit selection", func(t *testing.T) {
		code := Run(context.Background(), nil, CLIDependencies{
			Terminal: &fakeTerminal{interactive: true}, Stdout: new(bytes.Buffer), Stderr: new(bytes.Buffer),
			StdoutIsTerminal: func(io.Writer) bool { return true },
			ExecuteTUI:       func(context.Context) ([]string, error) { return nil, nil },
		})
		if code != 0 {
			t.Fatalf("Run(TUI exit)=%d", code)
		}
	})
}

func TestRunNonInteractiveInstallUsesNoTerminalAndInjectedFilesystemOnly(t *testing.T) {
	terminal := &fakeTerminal{interactive: true}
	stdout := new(bytes.Buffer)
	writes := make([]string, 0, 3)
	code := Run(context.Background(), []string{"install", "--mode", "docker", "--profile", "full", "--topology", "single", "--yes"}, CLIDependencies{
		Terminal: terminal, Stdout: stdout, Stderr: new(bytes.Buffer),
		Install: testInstallDependencies(&writes), ResolveRelease: resolvedReleaseForInstallSelector,
	})
	if code != 0 || terminal.prompts != 0 {
		t.Fatalf("Run(non-interactive) = %d, prompts %d, stdout %q", code, terminal.prompts, stdout.String())
	}
	if strings.Join(writes, ",") != "state,deployment.json,asset:compose.yml,asset:nginx-default.conf,asset:minio-init.sh,asset:postgres-init.sh,asset:prometheus.yml,runtime.env" || strings.Contains(stdout.String(), "Setup token:") || !strings.Contains(stdout.String(), "mgsctl setup token show") {
		t.Fatalf("install side effects/output = %v, %q", writes, stdout.String())
	}
}

func TestRunInteractiveInstallPromptsForMissingChoicesAndHonorsCancellation(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, answers: []string{"docker", "core", "single", "local"}, confirmed: false}
	writes := make([]string, 0)
	code := Run(context.Background(), []string{"install"}, CLIDependencies{
		Terminal: terminal, Stdout: new(bytes.Buffer), Stderr: new(bytes.Buffer), Install: testInstallDependencies(&writes), ResolveRelease: resolvedReleaseForInstallSelector,
	})
	if code != 0 || terminal.prompts != 11 || len(writes) != 0 {
		t.Fatalf("Run(cancelled interactive) = %d, prompts %d, writes %v", code, terminal.prompts, writes)
	}
}

func TestRunInteractiveInstallShowsTheOneTimeSetupToken(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, confirmed: true}
	stdout := new(bytes.Buffer)
	writes := make([]string, 0)
	code := Run(context.Background(), []string{"install"}, CLIDependencies{
		Terminal: terminal, Stdout: stdout, Stderr: new(bytes.Buffer), Install: testInstallDependencies(&writes), ResolveRelease: resolvedReleaseForInstallSelector,
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
		Terminal: terminal, Stdout: stdout, Stderr: new(bytes.Buffer), Install: testInstallDependencies(&writes), ResolveRelease: resolvedReleaseForInstallSelector,
	})
	if code != 0 || strings.Contains(stdout.String(), "Setup token:") || !strings.Contains(stdout.String(), "setup token show") {
		t.Fatalf("redirected interactive install = %d, stdout %q", code, stdout.String())
	}
}

func TestRunInteractiveInstallConfirmsBeforeOverwritingADifferentPendingPlan(t *testing.T) {
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	oldPlan, err := BuildInstallPlan(InstallInput{Mode: "docker", Profile: "core", Topology: "single", Role: "single", RuntimeDir: runtimeDirectory, StorageDriver: "local", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	oldResult, err := ExecuteInstall(context.Background(), oldPlan, InstallDependencies{Entropy: bytes.NewReader(bytes.Repeat([]byte{0x74}, 64))})
	if err != nil {
		t.Fatal(err)
	}
	oldRuntime := mustReadFile(t, oldResult.RuntimeEnvPath)

	t.Run("cancel preserves existing runtime", func(t *testing.T) {
		terminal := &fakeTerminal{interactive: true, confirmAnswers: []bool{true, false}}
		stdout := new(bytes.Buffer)
		code := Run(context.Background(), []string{"install", "--mode", "docker", "--profile", "core", "--topology", "single", "--runtime-dir", runtimeDirectory, "--image-tag", "v2"}, CLIDependencies{
			Terminal: terminal, Stdout: stdout, Stderr: new(bytes.Buffer), ResolveRelease: resolvedReleaseForInstallSelector,
		})
		if code != 0 || terminal.confirmations != 2 || !strings.Contains(stdout.String(), "preserved") {
			t.Fatalf("cancel overwrite code=%d confirmations=%d stdout=%q", code, terminal.confirmations, stdout.String())
		}
		if current := mustReadFile(t, oldResult.RuntimeEnvPath); !bytes.Equal(current, oldRuntime) {
			t.Fatal("cancelled overwrite changed runtime.env")
		}
	})

	terminal := &fakeTerminal{interactive: true, confirmAnswers: []bool{true, true}}
	code := Run(context.Background(), []string{"install", "--mode", "docker", "--profile", "core", "--topology", "single", "--runtime-dir", runtimeDirectory, "--image-tag", "v2"}, CLIDependencies{
		Terminal: terminal, Stdout: new(bytes.Buffer), Stderr: new(bytes.Buffer), ResolveRelease: resolvedReleaseForInstallSelector,
	})
	if code != 0 || terminal.confirmations != 2 {
		t.Fatalf("confirmed overwrite code=%d confirmations=%d", code, terminal.confirmations)
	}
	document, err := config.ParseRuntimeEnv(mustReadFile(t, oldResult.RuntimeEnvPath))
	if err != nil || document.Values["APPLICATION_VERSION"] != "v2" {
		t.Fatalf("confirmed overwrite runtime = %#v, %v", document.Values, err)
	}
}

func TestRunReturnsUsageCodeWithoutLeakingSensitiveArguments(t *testing.T) {
	stderr := new(bytes.Buffer)
	code := Run(context.Background(), []string{"cluster", "join", "--server", "ftp://invalid", "--token", "do-not-leak"}, CLIDependencies{Stderr: stderr})
	if code != 2 || strings.Contains(stderr.String(), "do-not-leak") {
		t.Fatalf("Run(invalid secret command) = %d, stderr %q", code, stderr.String())
	}
}

func TestRunVersionWritesTextOrJSONWithoutDeploymentDependencies(t *testing.T) {
	info := BuildInfo{Version: "v9.8.7", Commit: "abcdef0", BuildTime: "2026-07-28T01:02:03Z", Dirty: true}

	textOutput := new(bytes.Buffer)
	if code := Run(context.Background(), []string{"version"}, CLIDependencies{Stdout: textOutput, Stderr: new(bytes.Buffer), BuildInfo: info}); code != 0 {
		t.Fatalf("Run(version) = %d", code)
	}
	if !strings.Contains(textOutput.String(), "mgsctl v9.8.7") || !strings.Contains(textOutput.String(), "dirty: true") {
		t.Fatalf("text version output = %q", textOutput.String())
	}

	jsonOutput := new(bytes.Buffer)
	if code := Run(context.Background(), []string{"version", "--json"}, CLIDependencies{Stdout: jsonOutput, Stderr: new(bytes.Buffer), BuildInfo: info}); code != 0 {
		t.Fatalf("Run(version --json) = %d", code)
	}
	if !strings.Contains(jsonOutput.String(), `"version":"v9.8.7"`) || !strings.Contains(jsonOutput.String(), `"dirty":true`) {
		t.Fatalf("JSON version output = %q", jsonOutput.String())
	}
}

func TestRunSelfUpdateRequiresExplicitConfirmationAndReportsResult(t *testing.T) {
	t.Run("cancelled", func(t *testing.T) {
		terminal := &fakeTerminal{interactive: true, confirmed: false}
		called := false
		stdout := new(bytes.Buffer)
		code := Run(context.Background(), []string{"self-update", "--version", "v2"}, CLIDependencies{
			Terminal: terminal, Stdout: stdout, Stderr: new(bytes.Buffer), BuildInfo: BuildInfo{Version: "v1"},
			ExecuteSelfUpdate: func(context.Context, SelfUpdateOptions, SelfUpdateDependencies) (SelfUpdateResult, error) {
				called = true
				return SelfUpdateResult{}, nil
			},
		})
		if code != 0 || called || terminal.confirmations != 1 || !strings.Contains(stdout.String(), "cancelled") {
			t.Fatalf("cancelled self-update code=%d called=%t confirmations=%d output=%q", code, called, terminal.confirmations, stdout.String())
		}
	})

	t.Run("non-interactive success", func(t *testing.T) {
		terminal := &fakeTerminal{interactive: false}
		stdout := new(bytes.Buffer)
		code := Run(context.Background(), []string{"self-update", "--version", "v2", "--yes"}, CLIDependencies{
			Terminal: terminal, Stdout: stdout, Stderr: new(bytes.Buffer), BuildInfo: BuildInfo{Version: "v1"},
			ExecuteSelfUpdate: func(_ context.Context, options SelfUpdateOptions, _ SelfUpdateDependencies) (SelfUpdateResult, error) {
				if options.CurrentVersion != "v1" {
					t.Fatalf("current version = %q", options.CurrentVersion)
				}
				return SelfUpdateResult{PreviousVersion: "v1", CurrentVersion: "v2", Executable: "/tools/mgsctl"}, nil
			},
		})
		if code != 0 || terminal.confirmations != 0 || !strings.Contains(stdout.String(), "Updated mgsctl from v1 to v2") {
			t.Fatalf("successful self-update code=%d confirmations=%d output=%q", code, terminal.confirmations, stdout.String())
		}
	})
}

func TestRunDispatchesClusterJoinWithoutRenderingTheCredential(t *testing.T) {
	const credential = "pgjoin.v1.019d0000-0000-7000-8000-000000000999.secret"
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	var received ClusterJoinOptions
	code := Run(context.Background(), []string{
		"cluster", "join", "--server", "http://127.0.0.1:8080", "--token", credential,
		"--runtime-dir", "joined", "--mode", "docker", "--application-version", "v1",
	}, CLIDependencies{
		Stdout: stdout,
		Stderr: stderr,
		ExecuteClusterJoin: func(_ context.Context, options ClusterJoinOptions, _ ClusterJoinDependencies) (ClusterJoinResult, error) {
			received = options
			return ClusterJoinResult{
				RuntimeEnvPath: "joined/config/runtime.env",
				InstallationID: "019d0000-0000-7000-8000-000000000951",
				NodeID:         "019d0000-0000-7000-8000-000000000952",
				Role:           "worker",
			}, nil
		},
	})
	if code != 0 || received.Token != credential || received.RuntimeDir != "joined" {
		t.Fatalf("cluster join dispatch code=%d options=%#v stderr=%q", code, received, stderr.String())
	}
	for _, output := range []string{stdout.String(), stderr.String()} {
		if strings.Contains(output, credential) {
			t.Fatalf("cluster join output leaked credential: %q", output)
		}
	}
	if !strings.Contains(stdout.String(), "joined/config/runtime.env") || !strings.Contains(stdout.String(), "worker") {
		t.Fatalf("cluster join output = %q", stdout.String())
	}
}

func TestRunDispatchesOperationalCommands(t *testing.T) {
	t.Run("import", func(t *testing.T) {
		called := false
		stdout := new(bytes.Buffer)
		code := Run(context.Background(), []string{"import-config", "--source", ".env", "--runtime-dir", "runtime"}, CLIDependencies{
			Stdout: stdout, Stderr: new(bytes.Buffer),
			ExecuteImportConfig: func(_ context.Context, options ImportConfigOptions, _ ImportConfigDependencies) (ImportConfigResult, error) {
				called = options.Source == ".env" && options.RuntimeDir == "runtime"
				return ImportConfigResult{RuntimeEnvPath: "runtime/config/runtime.env"}, nil
			},
		})
		if code != 0 || !called || !strings.Contains(stdout.String(), "runtime/config/runtime.env") {
			t.Fatalf("import dispatch code=%d called=%t stdout=%q", code, called, stdout.String())
		}
	})

	t.Run("runtime action", func(t *testing.T) {
		var gotKind CommandKind
		code := Run(context.Background(), []string{"status", "--runtime-dir", "runtime"}, CLIDependencies{
			Stdout: io.Discard, Stderr: new(bytes.Buffer),
			ExecuteRuntimeAction: func(_ context.Context, kind CommandKind, runtimeDir string) error {
				gotKind = kind
				if runtimeDir != "runtime" {
					t.Fatalf("runtime dir = %q", runtimeDir)
				}
				return nil
			},
		})
		if code != 0 || gotKind != CommandStatus {
			t.Fatalf("status dispatch code=%d kind=%q", code, gotKind)
		}
	})

	t.Run("doctor failure", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		code := Run(context.Background(), []string{"doctor"}, CLIDependencies{
			Stdout: stdout, Stderr: new(bytes.Buffer),
			ExecuteDoctor: func(context.Context, string, DoctorDependencies) DoctorReport {
				return DoctorReport{Checks: []DoctorCheck{{Code: "SCHEMA_DRIFT", Message: "schema mismatch"}}}
			},
		})
		if code != 1 || !strings.Contains(stdout.String(), "SCHEMA_DRIFT") {
			t.Fatalf("doctor dispatch code=%d stdout=%q", code, stdout.String())
		}
	})

	t.Run("upgrade and uninstall", func(t *testing.T) {
		upgradeCalled, uninstallCalled := false, false
		upgradeCode := Run(context.Background(), []string{"upgrade", "--image-tag", "v2"}, CLIDependencies{
			Stdout: io.Discard, Stderr: new(bytes.Buffer),
			ExecuteUpgrade: func(_ context.Context, options UpgradeOptions, _ UpgradeDependencies) (UpgradeResult, error) {
				upgradeCalled = options.ApplicationVersion == "" && options.ImageTag == "v2" && options.Migrate
				return UpgradeResult{PreviousVersion: "v1", CurrentVersion: "v2", Migrated: true}, nil
			},
		})
		uninstallCode := Run(context.Background(), []string{"uninstall", "--yes"}, CLIDependencies{
			Stdout: io.Discard, Stderr: new(bytes.Buffer),
			ExecuteUninstall: func(_ context.Context, options UninstallOptions, _ UninstallDependencies) error {
				uninstallCalled = !options.DeleteData
				return nil
			},
		})
		if upgradeCode != 0 || uninstallCode != 0 || !upgradeCalled || !uninstallCalled {
			t.Fatalf("operational dispatch upgrade=(%d,%t) uninstall=(%d,%t)", upgradeCode, upgradeCalled, uninstallCode, uninstallCalled)
		}
	})
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
		RecoverIncomplete: func(string, string, string, []string) error { return nil },
		WriteRuntimeEnv:   func(string, []byte) error { *writes = append(*writes, "runtime.env"); return nil },
		InitializeState:   func(string, setup.InstallState) error { *writes = append(*writes, "state"); return nil },
		WriteManifest:     func(string, []byte) error { *writes = append(*writes, "deployment.json"); return nil },
		WriteDeploymentFile: func(path string, _ []byte) error {
			*writes = append(*writes, "asset:"+filepath.Base(path))
			return nil
		},
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
		RecoverIncomplete: func(string, string, string, []string) error { return nil },
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
		RecoverIncomplete: func(string, string, string, []string) error { return nil },
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
		RecoverIncomplete: func(string, string, string, []string) error { return nil },
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
	for _, path := range []string{filepath.Join(runtimeDirectory, "compose.yml"), filepath.Join(runtimeDirectory, "assets", "minio-init.sh"), filepath.Join(runtimeDirectory, "assets", "postgres-init.sh"), filepath.Join(runtimeDirectory, "assets", "nginx-default.conf"), filepath.Join(runtimeDirectory, "assets", "prometheus.yml")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if permissions := info.Mode().Perm(); permissions != 0o644 {
			t.Errorf("%s permissions = %#o, want 0644", path, permissions)
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
		RecoverIncomplete:  func(string, string, string, []string) error { return nil },
		AcquireInstallLock: func(context.Context, string) (func() error, error) { return func() error { return nil }, nil },
		InitializeState:    func(string, setup.InstallState) error { writes++; return os.ErrExist },
		RemovePath:         func(path string) error { removed = append(removed, path); return nil },
	})
	if err == nil || writes != 1 || len(removed) != 0 {
		t.Fatalf("no-clobber race = err %v, writes %d, removed %v", err, writes, removed)
	}
}

func TestExecuteInstallRollsBackOnlyPublishedDeploymentAssets(t *testing.T) {
	plan, err := BuildInstallPlan(InstallInput{Mode: "docker", Profile: "core", Topology: "single", Role: "single", RuntimeDir: "runtime", StorageDriver: "local", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	assetWrites := 0
	removed := make([]string, 0)
	_, err = ExecuteInstall(context.Background(), plan, InstallDependencies{
		Entropy:       bytes.NewReader(bytes.Repeat([]byte{0x79}, 32)),
		PathExists:    func(string) (bool, error) { return false, nil },
		MakeDirectory: func(string, os.FileMode) error { return nil },
		AcquireInstallLock: func(context.Context, string) (func() error, error) {
			return func() error { return nil }, nil
		},
		RecoverIncomplete: func(string, string, string, []string) error { return nil },
		InitializeState:   func(string, setup.InstallState) error { return nil },
		WriteManifest:     func(string, []byte) error { return nil },
		WriteDeploymentFile: func(string, []byte) error {
			assetWrites++
			if assetWrites == 2 {
				return errors.New("asset disk full")
			}
			return nil
		},
		RemovePath: func(path string) error { removed = append(removed, path); return nil },
	})
	if err == nil || assetWrites != 2 {
		t.Fatalf("asset failure = %v, writes %d", err, assetWrites)
	}
	want := "runtime/compose.yml,runtime/deployment.json,runtime/config/install-state.json"
	if got := strings.Join(removed, ","); got != want {
		t.Fatalf("asset rollback = %q, want %q", got, want)
	}
}

type errorReader string

func (reader errorReader) Read([]byte) (int, error) { return 0, errors.New(string(reader)) }
