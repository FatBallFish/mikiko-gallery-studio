package setup

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
)

const (
	testInstallationID = "019d0000-0000-7000-8000-000000000000"
	testOperationID    = "019d0000-0000-7000-8000-000000000001"
)

var testStateTime = time.Date(2026, time.July, 21, 6, 0, 0, 0, time.UTC)

func TestStatePathForRuntimeEnvUsesSameDirectory(t *testing.T) {
	runtimeEnvPath := filepath.Join("portable", "config", "runtime.env")
	want := filepath.Join("portable", "config", "install-state.json")
	if got := StatePathForRuntimeEnv(runtimeEnvPath); got != want {
		t.Fatalf("StatePathForRuntimeEnv(%q) = %q, want %q", runtimeEnvPath, got, want)
	}

	want = filepath.Join("config", "install-state.json")
	if got := DefaultInstallStatePath(); got != want {
		t.Fatalf("DefaultInstallStatePath() = %q, want %q", got, want)
	}
}

func TestStateStoreRoundTripsPrivateAtomicState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "install-state.json")
	store := NewStateStoreAt(path)
	state := pendingState()

	if err := store.Initialize(state); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	got, exists, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !exists {
		t.Fatal("Load() exists = false, want true")
	}
	assertInstallStateEqual(t, got, state)

	if runtime.GOOS != "windows" {
		directoryInfo, err := os.Stat(filepath.Dir(path))
		if err != nil {
			t.Fatalf("stat state directory: %v", err)
		}
		if got := directoryInfo.Mode().Perm(); got != 0o700 {
			t.Fatalf("state directory mode = %o, want 700", got)
		}
		fileInfo, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat state file: %v", err)
		}
		if got := fileInfo.Mode().Perm(); got != 0o600 {
			t.Fatalf("state file mode = %o, want 600", got)
		}
	}
}

func TestStateStoreInitializeIsCreateOnlyAndIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install-state.json")
	store := NewStateStoreAt(path)
	original := pendingState()
	if err := store.Initialize(original); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if err := NewStateStoreAt(path).Initialize(original); err != nil {
		t.Fatalf("idempotent Initialize() error = %v", err)
	}

	changes := []struct {
		name   string
		mutate func(*InstallState)
	}{
		{name: "identity", mutate: func(state *InstallState) { state.InstallationID = "019d0000-0000-7000-8000-000000000099" }},
		{name: "role", mutate: func(state *InstallState) { state.DeploymentRole = config.DeploymentRoleControl }},
	}
	for _, tt := range changes {
		t.Run(tt.name, func(t *testing.T) {
			changed := original
			tt.mutate(&changed)
			if err := NewStateStoreAt(path).Initialize(changed); !errors.Is(err, ErrInstallStateInvalid) {
				t.Fatalf("Initialize(changed %s) error = %v, want invalid", tt.name, err)
			}
			got, exists, err := store.Load()
			if err != nil || !exists {
				t.Fatalf("Load() = (%+v, %t, %v)", got, exists, err)
			}
			assertInstallStateEqual(t, got, original)
		})
	}
}

func TestStateStoreInitializeCannotDowngradeCompletedInstallation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install-state.json")
	store := NewStateStoreAt(path)
	pending := pendingState()
	if err := store.Initialize(pending); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	proof := validCommitProof()
	reserveCommitAttempt(t, store, proof, testStateTime.Add(30*time.Second))
	if _, err := store.BeginCommit(proof, testStateTime.Add(time.Minute)); err != nil {
		t.Fatalf("BeginCommit() error = %v", err)
	}
	completed, err := store.FinalizeCommit(proof, testStateTime.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("FinalizeCommit() error = %v", err)
	}

	if err := NewStateStoreAt(path).Initialize(pending); !errors.Is(err, ErrInstallStateInvalid) {
		t.Fatalf("Initialize(pending over completed) error = %v, want invalid", err)
	}
	got, exists, err := store.Load()
	if err != nil || !exists {
		t.Fatalf("Load() = (%+v, %t, %v)", got, exists, err)
	}
	assertInstallStateEqual(t, got, completed)
}

func TestStateStoreInitializeRejectsJoinedOrNonPendingState(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*InstallState)
	}{
		{name: "joined role", mutate: func(state *InstallState) { state.DeploymentRole = config.DeploymentRoleAPI }},
		{name: "committing", mutate: func(state *InstallState) {
			state.Phase = InstallPhaseCommitting
			state.Commit = validCommitJournal()
		}},
		{name: "completed", mutate: func(state *InstallState) {
			state.Phase = InstallPhaseCompleted
			state.EverCompleted = true
			state.Commit = validCommitJournal()
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			state := pendingState()
			tt.mutate(&state)
			store := NewStateStoreAt(filepath.Join(t.TempDir(), "install-state.json"))
			if err := store.Initialize(state); !errors.Is(err, ErrInstallStateInvalid) {
				t.Fatalf("Initialize() error = %v, want invalid", err)
			}
			if _, exists, err := store.Load(); err != nil || exists {
				t.Fatalf("Load() after rejected Initialize = (exists=%t, err=%v)", exists, err)
			}
		})
	}
}

func TestStateStoreInitializeDoesNotOverwriteCorruptState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install-state.json")
	original := []byte(`{"schema_version":`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("write corrupt state: %v", err)
	}
	if err := NewStateStoreAt(path).Initialize(pendingState()); !errors.Is(err, ErrInstallStateCorrupt) {
		t.Fatalf("Initialize() error = %v, want corrupt", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state after rejected Initialize: %v", err)
	}
	if string(got) != string(original) {
		t.Fatalf("corrupt state was overwritten: %q", got)
	}
}

func TestStateStoresShareCanonicalProcessLock(t *testing.T) {
	directory := t.TempDir()
	canonical := filepath.Join(directory, "install-state.json")
	alias := filepath.Join(directory, "nested", "..", "install-state.json")
	first := NewStateStoreAt(canonical)
	second := NewStateStoreAt(alias)
	if first.Path() != second.Path() {
		t.Fatalf("normalized paths differ: %q != %q", first.Path(), second.Path())
	}
	if first.processLock != second.processLock {
		t.Fatal("stores for the same normalized path do not share a process lock")
	}
}

func TestWindowsProcessLockKeyDoesNotRewriteIOPathCase(t *testing.T) {
	mixedCasePath := filepath.Join(t.TempDir(), "CaseSensitive", "Install-State.JSON")
	normalized, err := normalizeStatePath(mixedCasePath)
	if err != nil {
		t.Fatalf("normalizeStatePath() error = %v", err)
	}
	if filepath.Base(normalized) != "Install-State.JSON" {
		t.Fatalf("I/O path case was rewritten: %q", normalized)
	}
	lockKey := normalizeProcessLockKey(normalized, true)
	if lockKey != strings.ToLower(normalized) {
		t.Fatalf("Windows lock key = %q, want case-folded %q", lockKey, strings.ToLower(normalized))
	}
	if filepath.Base(normalized) != "Install-State.JSON" {
		t.Fatalf("building lock key mutated I/O path: %q", normalized)
	}
}

func TestStateStoresSerializeConcurrentBeginWithDifferentProofs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install-state.json")
	first := NewStateStoreAt(path)
	second := NewStateStoreAt(path)
	first.lockTimeout = 2 * time.Second
	second.lockTimeout = 2 * time.Second
	if err := first.Initialize(pendingState()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	firstProof := validCommitProof()
	secondProof := validCommitProof()
	secondProof.OperationID = "019d0000-0000-7000-8000-000000000099"
	reserveCommitAttempt(t, first, firstProof, testStateTime.Add(30*time.Second))

	reachedReplace := make(chan struct{})
	releaseReplace := make(chan struct{})
	originalReplace := first.operations.replaceFile
	var signalOnce sync.Once
	first.operations.replaceFile = func(source, destination string) error {
		signalOnce.Do(func() { close(reachedReplace) })
		<-releaseReplace
		return originalReplace(source, destination)
	}
	type result struct {
		proof CommitProof
		state InstallState
		err   error
	}
	results := make(chan result, 2)
	go func() {
		state, err := first.BeginCommit(firstProof, testStateTime.Add(time.Minute))
		results <- result{proof: firstProof, state: state, err: err}
	}()
	select {
	case <-reachedReplace:
	case <-time.After(time.Second):
		t.Fatal("first BeginCommit did not reach atomic replacement")
	}
	go func() {
		state, err := second.BeginCommit(secondProof, testStateTime.Add(time.Minute))
		results <- result{proof: secondProof, state: state, err: err}
	}()
	select {
	case early := <-results:
		t.Fatalf("second store bypassed held lock: %+v", early)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseReplace)

	firstResult := <-results
	secondResult := <-results
	succeeded := []result{}
	for _, got := range []result{firstResult, secondResult} {
		if got.err == nil {
			succeeded = append(succeeded, got)
		}
	}
	if len(succeeded) != 1 {
		t.Fatalf("successful BeginCommit calls = %d, want exactly one; results=%+v %+v", len(succeeded), firstResult, secondResult)
	}
	disk, exists, err := NewStateStoreAt(path).Load()
	if err != nil || !exists || disk.Commit == nil {
		t.Fatalf("Load() after concurrent Begin = (%+v, %t, %v)", disk, exists, err)
	}
	if *disk.Commit != succeeded[0].proof {
		t.Fatalf("disk journal = %+v, successful proof = %+v", *disk.Commit, succeeded[0].proof)
	}
}

func TestStateStoresSerializeConcurrentFinalizeAndMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install-state.json")
	first := NewStateStoreAt(path)
	second := NewStateStoreAt(path)
	proof := validCommitProof()
	if err := first.Initialize(pendingState()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	reserveCommitAttempt(t, first, proof, testStateTime.Add(30*time.Second))
	if _, err := first.BeginCommit(proof, testStateTime.Add(time.Minute)); err != nil {
		t.Fatalf("BeginCommit() error = %v", err)
	}
	mismatch := proof
	mismatch.ConfigRevision++

	type result struct {
		proof CommitProof
		state InstallState
		err   error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for store, candidate := range map[*StateStore]CommitProof{first: proof, second: mismatch} {
		go func() {
			<-start
			state, err := store.FinalizeCommit(candidate, testStateTime.Add(2*time.Minute))
			results <- result{proof: candidate, state: state, err: err}
		}()
	}
	close(start)
	one, two := <-results, <-results
	if (one.err == nil) == (two.err == nil) {
		t.Fatalf("Finalize results = (%v, %v), want one success and one failure", one.err, two.err)
	}
	completed, exists, err := first.Load()
	if err != nil || !exists || completed.Phase != InstallPhaseCompleted || completed.Commit == nil || *completed.Commit != proof {
		t.Fatalf("completed disk state = (%+v, %t, %v)", completed, exists, err)
	}
}

func TestStateFileLockIsPrivateAndTimesOutInsteadOfWaitingForever(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install-state.json.lock")
	lock, err := acquireStateFileLock(path, time.Second, platformStateAtomicOps())
	if err != nil {
		t.Fatalf("acquire first file lock: %v", err)
	}
	defer lock.release()
	started := time.Now()
	if _, err := acquireStateFileLock(path, 50*time.Millisecond, platformStateAtomicOps()); !errors.Is(err, ErrInstallStateLockTimeout) {
		t.Fatalf("second lock error = %v, want timeout", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("lock timeout took %s", elapsed)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat lock file: %v", err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("lock file mode = %o, want 600", got)
		}
	}
}

func TestStateStoreLoadTimesOutInsteadOfWaitingForeverForProcessLock(t *testing.T) {
	store := NewStateStoreAt(filepath.Join(t.TempDir(), "install-state.json"))
	store.lockTimeout = 40 * time.Millisecond
	store.processLock.Lock()
	cancelRelease := make(chan struct{})
	releaseDone := make(chan struct{})
	go func() {
		defer close(releaseDone)
		select {
		case <-time.After(150 * time.Millisecond):
			store.processLock.Unlock()
		case <-cancelRelease:
		}
	}()
	started := time.Now()
	_, _, err := store.Load()
	timedOut := errors.Is(err, ErrInstallStateLockTimeout)
	close(cancelRelease)
	<-releaseDone
	if timedOut {
		store.processLock.Unlock()
	}
	if !timedOut {
		t.Fatalf("Load() error = %v, want lock timeout", err)
	}
	if elapsed := time.Since(started); elapsed > 120*time.Millisecond {
		t.Fatalf("Load() timeout took %s", elapsed)
	}
}

func TestStateStoreLoadDistinguishesMissingCorruptAndInvalid(t *testing.T) {
	tests := []struct {
		name       string
		contents   string
		wantExists bool
		wantError  error
	}{
		{name: "missing", wantExists: false},
		{name: "corrupt json", contents: `{`, wantExists: true, wantError: ErrInstallStateCorrupt},
		{name: "trailing json", contents: `{}` + "\n{}", wantExists: true, wantError: ErrInstallStateCorrupt},
		{name: "unknown field", contents: validStateJSON(`,"setup_token":"must-not-be-accepted"`), wantExists: true, wantError: ErrInstallStateInvalid},
		{name: "duplicate field", contents: strings.Replace(validStateJSON(""), `"phase":"pending"`, `"phase":"pending","phase":"pending"`, 1), wantExists: true, wantError: ErrInstallStateInvalid},
		{name: "case folded field alias", contents: strings.Replace(validStateJSON(""), `"schema_version"`, `"Schema_Version"`, 1), wantExists: true, wantError: ErrInstallStateInvalid},
		{name: "null commit field", contents: validStateJSON(`,"commit":null`), wantExists: true, wantError: ErrInstallStateInvalid},
		{name: "missing required field", contents: strings.Replace(validStateJSON(""), `,"ever_completed":false`, ``, 1), wantExists: true, wantError: ErrInstallStateInvalid},
		{name: "null required field", contents: strings.Replace(validStateJSON(""), `"ever_completed":false`, `"ever_completed":null`, 1), wantExists: true, wantError: ErrInstallStateInvalid},
		{name: "commit missing required field", contents: strings.Replace(committingStateJSON(), `,"config_revision":7`, ``, 1), wantExists: true, wantError: ErrInstallStateInvalid},
		{name: "commit null required field", contents: strings.Replace(committingStateJSON(), `"config_revision":7`, `"config_revision":null`, 1), wantExists: true, wantError: ErrInstallStateInvalid},
		{name: "invalid phase", contents: validStateJSON(`,"phase_override":"bad"`), wantExists: true, wantError: ErrInstallStateInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "install-state.json")
			if tt.name == "invalid phase" {
				tt.contents = strings.Replace(validStateJSON(""), `"phase":"pending"`, `"phase":"unknown"`, 1)
			}
			if tt.contents != "" {
				if err := os.WriteFile(path, []byte(tt.contents), 0o600); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
			}

			_, exists, err := NewStateStoreAt(path).Load()
			if exists != tt.wantExists {
				t.Fatalf("Load() exists = %t, want %t", exists, tt.wantExists)
			}
			if tt.wantError == nil {
				if err != nil {
					t.Fatalf("Load() error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("Load() error = %v, want errors.Is(..., %v)", err, tt.wantError)
			}
		})
	}
}

func TestInstallStateValidationRejectsInvalidSchemaIdentityRolePhaseTimeAndJournal(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*InstallState)
	}{
		{name: "schema", mutate: func(state *InstallState) { state.SchemaVersion++ }},
		{name: "installation id", mutate: func(state *InstallState) { state.InstallationID = "contains space" }},
		{name: "role", mutate: func(state *InstallState) { state.DeploymentRole = "database" }},
		{name: "phase", mutate: func(state *InstallState) { state.Phase = "unknown" }},
		{name: "time", mutate: func(state *InstallState) { state.UpdatedAt = time.Time{} }},
		{name: "out of range time", mutate: func(state *InstallState) { state.UpdatedAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) }},
		{name: "non utc time", mutate: func(state *InstallState) { state.UpdatedAt = state.UpdatedAt.In(time.FixedZone("offset", 3600)) }},
		{name: "pending completed", mutate: func(state *InstallState) { state.EverCompleted = true }},
		{name: "pending journal", mutate: func(state *InstallState) { state.Commit = validCommitJournal() }},
		{name: "committing without journal", mutate: func(state *InstallState) { state.Phase = InstallPhaseCommitting }},
		{name: "completed without marker", mutate: func(state *InstallState) { state.Phase = InstallPhaseCompleted }},
		{name: "completed without journal", mutate: func(state *InstallState) {
			state.Phase = InstallPhaseCompleted
			state.EverCompleted = true
			state.Commit = nil
		}},
		{name: "journal runtime schema", mutate: func(state *InstallState) {
			state.Phase = InstallPhaseCommitting
			state.Commit = validCommitJournal()
			state.Commit.RuntimeSchemaVersion = 0
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := pendingState()
			tt.mutate(&state)
			if err := state.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want validation failure")
			}
		})
	}
}

func TestStateStoreAtomicFailurePreservesTargetAndCleansTemporaryFile(t *testing.T) {
	for _, failure := range []string{"secure file", "replace file", "sync directory"} {
		t.Run(failure, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "install-state.json")
			store := NewStateStoreAt(path)
			original := pendingState()
			if err := store.Initialize(original); err != nil {
				t.Fatalf("seed state: %v", err)
			}
			reserveCommitAttempt(t, store, validCommitProof(), testStateTime.Add(30*time.Second))
			reserved, _, err := store.Load()
			if err != nil {
				t.Fatalf("load reserved attempt: %v", err)
			}

			operations := platformStateAtomicOps()
			switch failure {
			case "secure file":
				operations.secureFile = func(string, *os.File) error { return errors.New("injected secure failure") }
			case "replace file":
				operations.replaceFile = func(string, string) error { return errors.New("injected replace failure") }
			case "sync directory":
				operations.syncDirectory = func(string) error { return errors.New("injected sync failure") }
			}
			store.operations = operations
			if _, err := store.BeginCommit(validCommitProof(), testStateTime.Add(time.Minute)); err == nil {
				t.Fatal("BeginCommit() error = nil, want injected failure")
			}

			got, exists, err := NewStateStoreAt(path).Load()
			if failure == "sync directory" {
				// Replacement already happened; fsync failure must be reported even though
				// the new file may be visible.
				if err != nil || !exists {
					t.Fatalf("Load() after sync failure = (%v, %t, %v)", got, exists, err)
				}
			} else {
				if err != nil || !exists {
					t.Fatalf("Load() after failed write = (%v, %t, %v)", got, exists, err)
				}
				assertInstallStateEqual(t, got, reserved)
			}
			matches, err := filepath.Glob(filepath.Join(directory, ".install-state.json.tmp-*"))
			if err != nil {
				t.Fatalf("glob temporary state files: %v", err)
			}
			if len(matches) != 0 {
				t.Fatalf("temporary state files remain after failure: %v", matches)
			}
		})
	}
}

func TestStateStoreBeginAndFinalizeCommitAreIdempotentAndFailClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install-state.json")
	store := NewStateStoreAt(path)
	if err := store.Initialize(pendingState()); err != nil {
		t.Fatalf("seed pending state: %v", err)
	}

	proof := validCommitProof()
	reserveCommitAttempt(t, store, proof, testStateTime.Add(30*time.Second))
	committing, err := store.BeginCommit(proof, testStateTime.Add(time.Minute))
	if err != nil {
		t.Fatalf("BeginCommit() error = %v", err)
	}
	if committing.Phase != InstallPhaseCommitting || committing.Commit == nil {
		t.Fatalf("BeginCommit() state = %+v, want committing journal", committing)
	}
	if committing.Commit.OperationID != testOperationID || committing.Commit.InstallationID != testInstallationID || committing.Commit.RuntimeSchemaVersion != config.CurrentRuntimeSchemaVersion || committing.Commit.ConfigRevision != 7 {
		t.Fatalf("BeginCommit() journal = %+v", committing.Commit)
	}
	second, err := store.BeginCommit(proof, testStateTime.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("idempotent BeginCommit() error = %v", err)
	}
	assertInstallStateEqual(t, second, committing)
	for _, tt := range commitProofMismatchTests() {
		t.Run("begin mismatch "+tt.name, func(t *testing.T) {
			if _, err := store.BeginCommit(tt.proof, testStateTime.Add(2*time.Minute)); err == nil {
				t.Fatalf("BeginCommit() with mismatched %s succeeded", tt.name)
			}
		})
		t.Run("finalize mismatch "+tt.name, func(t *testing.T) {
			if _, err := store.FinalizeCommit(tt.proof, testStateTime.Add(3*time.Minute)); err == nil {
				t.Fatalf("FinalizeCommit() with mismatched %s succeeded", tt.name)
			}
		})
	}

	completed, err := store.FinalizeCommit(proof, testStateTime.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("FinalizeCommit() error = %v", err)
	}
	if completed.Phase != InstallPhaseCompleted || !completed.EverCompleted {
		t.Fatalf("FinalizeCommit() state = %+v, want completed", completed)
	}
	idempotent, err := store.FinalizeCommit(proof, testStateTime.Add(4*time.Minute))
	if err != nil {
		t.Fatalf("idempotent FinalizeCommit() error = %v", err)
	}
	assertInstallStateEqual(t, idempotent, completed)
	wrongOperation := proof
	wrongOperation.OperationID = "019d0000-0000-7000-8000-000000000099"
	if _, err := store.FinalizeCommit(wrongOperation, testStateTime.Add(4*time.Minute)); err == nil {
		t.Fatal("completed FinalizeCommit() with mismatched operation ID succeeded")
	}
	beginAfterFinalize, err := store.BeginCommit(proof, testStateTime.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("idempotent BeginCommit() after finalize error = %v", err)
	}
	assertInstallStateEqual(t, beginAfterFinalize, completed)
}

func TestStateStoreBeginCommitRequiresCallerInstallationProof(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install-state.json")
	store := NewStateStoreAt(path)
	original := pendingState()
	if err := store.Initialize(original); err != nil {
		t.Fatalf("seed pending state: %v", err)
	}
	proof := validCommitProof()
	reserveCommitAttempt(t, store, proof, testStateTime.Add(30*time.Second))
	reserved, _, err := store.Load()
	if err != nil {
		t.Fatalf("load reserved attempt: %v", err)
	}
	proof.InstallationID = "019d0000-0000-7000-8000-000000000099"
	if _, err := store.BeginCommit(proof, testStateTime.Add(time.Minute)); err == nil {
		t.Fatal("BeginCommit() silently accepted mismatched installation proof")
	}
	got, exists, err := store.Load()
	if err != nil || !exists {
		t.Fatalf("Load() after rejected proof = (%+v, %t, %v)", got, exists, err)
	}
	assertInstallStateEqual(t, got, reserved)
}

func TestStateStoreBeginCommitRejectsUnsupportedRuntimeSchemaBeforeWritingJournal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install-state.json")
	store := NewStateStoreAt(path)
	original := pendingState()
	if err := store.Initialize(original); err != nil {
		t.Fatalf("seed pending state: %v", err)
	}
	proof := validCommitProof()
	reserveCommitAttempt(t, store, proof, testStateTime.Add(30*time.Second))
	reserved, _, err := store.Load()
	if err != nil {
		t.Fatalf("load reserved attempt: %v", err)
	}
	proof.RuntimeSchemaVersion++
	if _, err := store.BeginCommit(proof, testStateTime.Add(time.Minute)); err == nil {
		t.Fatal("BeginCommit() with unsupported runtime schema succeeded")
	}
	got, exists, err := store.Load()
	if err != nil || !exists {
		t.Fatalf("Load() after rejected BeginCommit = (%+v, %t, %v)", got, exists, err)
	}
	assertInstallStateEqual(t, got, reserved)
}

func TestStateStoreLoadRejectsUnsupportedRuntimeSchemaInJournal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install-state.json")
	contents := strings.Replace(
		committingStateJSON(),
		`"runtime_schema_version":1`,
		`"runtime_schema_version":2`,
		1,
	)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write future journal fixture: %v", err)
	}
	_, exists, err := NewStateStoreAt(path).Load()
	if !exists || !errors.Is(err, ErrInstallStateInvalid) {
		t.Fatalf("Load() = (exists=%t, err=%v), want invalid existing state", exists, err)
	}
}

func TestInstallStateJSONContainsNoSecretMaterial(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install-state.json")
	store := NewStateStoreAt(path)
	if err := store.Initialize(pendingState()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	for _, forbidden := range []string{"setup_token", "password", "secret", "api_key"} {
		if strings.Contains(strings.ToLower(string(contents)), forbidden) {
			t.Fatalf("state JSON contains forbidden secret-shaped field %q: %s", forbidden, contents)
		}
	}
}

func pendingState() InstallState {
	return InstallState{
		SchemaVersion:  CurrentInstallStateSchemaVersion,
		InstallationID: testInstallationID,
		DeploymentRole: config.DeploymentRoleSingle,
		Phase:          InstallPhasePending,
		EverCompleted:  false,
		UpdatedAt:      testStateTime,
	}
}

func validCommitJournal() *CommitJournal {
	return &CommitJournal{
		OperationID:          testOperationID,
		InstallationID:       testInstallationID,
		RuntimeSchemaVersion: config.CurrentRuntimeSchemaVersion,
		ConfigRevision:       7,
		RequestDigest:        strings.Repeat("a", 64),
	}
}

func validCommitProof() CommitProof {
	return CommitProof(*validCommitJournal())
}

func reserveCommitAttempt(t *testing.T, store *StateStore, proof CommitProof, at time.Time) {
	t.Helper()
	if _, err := store.BeginAttempt(SetupAttempt{
		OperationID: proof.OperationID, ConfigRevision: proof.ConfigRevision, RequestDigest: strings.Repeat("a", 64),
		AdminCredentialVerifier: strings.Repeat("b", 64),
	}, at); err != nil {
		t.Fatalf("BeginAttempt before commit: %v", err)
	}
}

func commitProofMismatchTests() []struct {
	name  string
	proof CommitProof
} {
	operation := validCommitProof()
	operation.OperationID = "019d0000-0000-7000-8000-000000000099"
	installation := validCommitProof()
	installation.InstallationID = "019d0000-0000-7000-8000-000000000099"
	runtimeSchema := validCommitProof()
	runtimeSchema.RuntimeSchemaVersion++
	revision := validCommitProof()
	revision.ConfigRevision++
	return []struct {
		name  string
		proof CommitProof
	}{
		{name: "operation ID", proof: operation},
		{name: "installation ID", proof: installation},
		{name: "runtime schema", proof: runtimeSchema},
		{name: "config revision", proof: revision},
	}
}

func committingStateJSON() string {
	return `{"schema_version":1,"installation_id":"` + testInstallationID + `","deployment_role":"single","phase":"committing","ever_completed":false,"updated_at":"2026-07-21T06:00:00Z","commit":{"operation_id":"` + testOperationID + `","installation_id":"` + testInstallationID + `","runtime_schema_version":1,"config_revision":7,"request_digest":"` + strings.Repeat("a", 64) + `"}}`
}

func validStateJSON(extra string) string {
	return `{"schema_version":1,"installation_id":"` + testInstallationID + `","deployment_role":"single","phase":"pending","ever_completed":false,"updated_at":"2026-07-21T06:00:00Z"` + extra + `}`
}

func assertInstallStateEqual(t *testing.T, got, want InstallState) {
	t.Helper()
	if got.SchemaVersion != want.SchemaVersion || got.InstallationID != want.InstallationID || got.DeploymentRole != want.DeploymentRole || got.Phase != want.Phase || got.EverCompleted != want.EverCompleted || !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Fatalf("InstallState = %+v, want %+v", got, want)
	}
	switch {
	case got.Commit == nil && want.Commit == nil:
	case got.Commit == nil || want.Commit == nil || *got.Commit != *want.Commit:
		t.Fatalf("InstallState.Commit = %+v, want %+v", got.Commit, want.Commit)
	}
	switch {
	case got.Attempt == nil && want.Attempt == nil:
		return
	case got.Attempt == nil || want.Attempt == nil || !setupAttemptsEqual(*got.Attempt, *want.Attempt):
		t.Fatalf("InstallState.Attempt = %+v, want %+v", got.Attempt, want.Attempt)
	}
}
