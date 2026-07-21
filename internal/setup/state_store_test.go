package setup

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

	if err := store.Save(state); err != nil {
		t.Fatalf("Save() error = %v", err)
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
			if err := store.Save(original); err != nil {
				t.Fatalf("seed state: %v", err)
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
			changed := original
			changed.UpdatedAt = changed.UpdatedAt.Add(time.Minute)
			if err := store.Save(changed); err == nil {
				t.Fatal("Save() error = nil, want injected failure")
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
				assertInstallStateEqual(t, got, original)
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
	if err := store.Save(pendingState()); err != nil {
		t.Fatalf("seed pending state: %v", err)
	}

	proof := validCommitProof()
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
	if err := store.Save(original); err != nil {
		t.Fatalf("seed pending state: %v", err)
	}
	proof := validCommitProof()
	proof.InstallationID = "019d0000-0000-7000-8000-000000000099"
	if _, err := store.BeginCommit(proof, testStateTime.Add(time.Minute)); err == nil {
		t.Fatal("BeginCommit() silently accepted mismatched installation proof")
	}
	got, exists, err := store.Load()
	if err != nil || !exists {
		t.Fatalf("Load() after rejected proof = (%+v, %t, %v)", got, exists, err)
	}
	assertInstallStateEqual(t, got, original)
}

func TestStateStoreBeginCommitRejectsUnsupportedRuntimeSchemaBeforeWritingJournal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install-state.json")
	store := NewStateStoreAt(path)
	original := pendingState()
	if err := store.Save(original); err != nil {
		t.Fatalf("seed pending state: %v", err)
	}
	proof := validCommitProof()
	proof.RuntimeSchemaVersion++
	if _, err := store.BeginCommit(proof, testStateTime.Add(time.Minute)); err == nil {
		t.Fatal("BeginCommit() with unsupported runtime schema succeeded")
	}
	got, exists, err := store.Load()
	if err != nil || !exists {
		t.Fatalf("Load() after rejected BeginCommit = (%+v, %t, %v)", got, exists, err)
	}
	assertInstallStateEqual(t, got, original)
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
	if err := store.Save(pendingState()); err != nil {
		t.Fatalf("Save() error = %v", err)
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
	}
}

func validCommitProof() CommitProof {
	return CommitProof(*validCommitJournal())
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
	return `{"schema_version":1,"installation_id":"` + testInstallationID + `","deployment_role":"single","phase":"committing","ever_completed":false,"updated_at":"2026-07-21T06:00:00Z","commit":{"operation_id":"` + testOperationID + `","installation_id":"` + testInstallationID + `","runtime_schema_version":1,"config_revision":7}}`
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
		return
	case got.Commit == nil || want.Commit == nil || *got.Commit != *want.Commit:
		t.Fatalf("InstallState.Commit = %+v, want %+v", got.Commit, want.Commit)
	}
}
