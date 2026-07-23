package setup

import (
	"bytes"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
)

var (
	ErrInstallStateCorrupt     = errors.New("install state is corrupt")
	ErrInstallStateInvalid     = errors.New("install state is invalid")
	ErrInstallStateLockTimeout = errors.New("timed out acquiring install state lock")
)

const defaultStateLockTimeout = 10 * time.Second

var stateProcessLocks sync.Map

type stateAtomicOps struct {
	secureDirectory func(string) error
	secureFile      func(string, *os.File) error
	replaceFile     func(string, string) error
	syncDirectory   func(string) error
}

type StateStore struct {
	path        string
	pathErr     error
	operations  stateAtomicOps
	processLock *sync.Mutex
	lockTimeout time.Duration
}

func DefaultInstallStatePath() string {
	return StatePathForRuntimeEnv(config.DefaultRuntimeEnvPath())
}

func StatePathForRuntimeEnv(runtimeEnvPath string) string {
	if strings.TrimSpace(runtimeEnvPath) == "" {
		runtimeEnvPath = config.DefaultRuntimeEnvPath()
	}
	return filepath.Join(filepath.Dir(runtimeEnvPath), "install-state.json")
}

func NewStateStore(runtimeEnvPath string) *StateStore {
	return NewStateStoreAt(StatePathForRuntimeEnv(runtimeEnvPath))
}

func NewStateStoreAt(path string) *StateStore {
	normalizedPath, pathErr := normalizeStatePath(path)
	lockKey := normalizeProcessLockKey(normalizedPath, runtime.GOOS == "windows")
	if pathErr != nil {
		lockKey = normalizeProcessLockKey(filepath.Clean(path), runtime.GOOS == "windows")
	}
	return &StateStore{
		path:        normalizedPath,
		pathErr:     pathErr,
		operations:  platformStateAtomicOps(),
		processLock: processLockForPath(lockKey),
		lockTimeout: defaultStateLockTimeout,
	}
}

func (store *StateStore) Path() string {
	if store == nil {
		return ""
	}
	return store.path
}

func (store *StateStore) Load() (InstallState, bool, error) {
	if err := store.validate(); err != nil {
		return InstallState{}, false, err
	}
	if err := acquireStateProcessLock(store.processLock, store.lockTimeout); err != nil {
		return InstallState{}, false, err
	}
	defer store.processLock.Unlock()
	return store.loadUnlocked()
}

func (store *StateStore) Initialize(state InstallState) error {
	if err := store.validate(); err != nil {
		return err
	}
	if err := state.Validate(); err != nil {
		return fmt.Errorf("%w: validate initial state: %v", ErrInstallStateInvalid, err)
	}
	if state.Phase != InstallPhasePending || state.EverCompleted || state.Commit != nil || !setupAuthority(state.DeploymentRole) {
		return fmt.Errorf("%w: Initialize accepts only a pending single/control installation", ErrInstallStateInvalid)
	}
	_, err := store.withMutationLock(func() (InstallState, error) {
		existing, exists, err := store.loadUnlocked()
		if err != nil {
			return InstallState{}, err
		}
		if exists {
			if installStatesEqual(existing, state) {
				return existing, nil
			}
			return InstallState{}, fmt.Errorf("%w: install state already exists with different identity or phase", ErrInstallStateInvalid)
		}
		if err := store.saveUnlocked(state); err != nil {
			return InstallState{}, err
		}
		return state, nil
	})
	return err
}

func (store *StateStore) BeginAttempt(attempt SetupAttempt, at time.Time) (InstallState, error) {
	if err := store.validate(); err != nil {
		return InstallState{}, err
	}
	at, err := validateTransitionTime(at)
	if err != nil {
		return InstallState{}, err
	}
	if err := attempt.Validate(); err != nil {
		return InstallState{}, fmt.Errorf("validate setup attempt: %w", err)
	}
	return store.withMutationLock(func() (InstallState, error) {
		state, exists, err := store.loadUnlocked()
		if err != nil {
			return InstallState{}, err
		}
		if !exists || state.Phase != InstallPhasePending {
			return InstallState{}, fmt.Errorf("%w: setup attempt requires pending install state", ErrInstallStateInvalid)
		}
		if state.Attempt != nil {
			if setupAttemptsEqual(*state.Attempt, attempt) {
				return state, nil
			}
			if state.Attempt.OperationID != attempt.OperationID {
				return InstallState{}, ErrSetupOperationConflict
			}
			return InstallState{}, ErrSetupBindingMismatch
		}
		state.Attempt = &attempt
		state.UpdatedAt = at
		if err := store.saveUnlocked(state); err != nil {
			return InstallState{}, err
		}
		return state, nil
	})
}

func (store *StateStore) ClearAttempt(attempt SetupAttempt, at time.Time) (InstallState, error) {
	if err := store.validate(); err != nil {
		return InstallState{}, err
	}
	at, err := validateTransitionTime(at)
	if err != nil {
		return InstallState{}, err
	}
	if err := attempt.Validate(); err != nil {
		return InstallState{}, fmt.Errorf("validate setup attempt: %w", err)
	}
	return store.withMutationLock(func() (InstallState, error) {
		state, exists, err := store.loadUnlocked()
		if err != nil {
			return InstallState{}, err
		}
		if !exists || state.Phase != InstallPhasePending || state.Attempt == nil {
			return InstallState{}, fmt.Errorf("%w: no pending setup attempt", ErrInstallStateInvalid)
		}
		if !setupAttemptsEqual(*state.Attempt, attempt) {
			if state.Attempt.OperationID != attempt.OperationID {
				return InstallState{}, ErrSetupOperationConflict
			}
			return InstallState{}, ErrSetupBindingMismatch
		}
		state.Attempt = nil
		state.UpdatedAt = at
		if err := store.saveUnlocked(state); err != nil {
			return InstallState{}, err
		}
		return state, nil
	})
}

func (store *StateStore) BeginCommit(proof CommitProof, at time.Time) (InstallState, error) {
	if err := store.validate(); err != nil {
		return InstallState{}, err
	}
	at, err := validateTransitionTime(at)
	if err != nil {
		return InstallState{}, err
	}
	if err := proof.Validate(); err != nil {
		return InstallState{}, fmt.Errorf("validate commit proof: %w", err)
	}

	return store.withMutationLock(func() (InstallState, error) {
		state, exists, err := store.loadUnlocked()
		if err != nil {
			return InstallState{}, err
		}
		if !exists {
			return InstallState{}, fmt.Errorf("%w: cannot begin commit without install state", ErrInstallStateInvalid)
		}
		if proof.InstallationID != state.InstallationID {
			return InstallState{}, fmt.Errorf("%w: commit proof installation ID does not match install state", ErrInstallStateInvalid)
		}

		switch state.Phase {
		case InstallPhasePending:
			if state.Attempt == nil {
				return InstallState{}, fmt.Errorf("%w: cannot begin commit without a pending setup attempt", ErrInstallStateInvalid)
			}
			if state.Attempt.OperationID != proof.OperationID {
				return InstallState{}, ErrSetupOperationConflict
			}
			if state.Attempt.ConfigRevision != proof.ConfigRevision || !constantTimeDigestEqual(state.Attempt.RequestDigest, proof.RequestDigest) {
				return InstallState{}, ErrSetupBindingMismatch
			}
			state.Phase = InstallPhaseCommitting
			state.Attempt = nil
			state.Commit = &proof
			state.UpdatedAt = at
			if err := store.saveUnlocked(state); err != nil {
				return InstallState{}, err
			}
			return state, nil
		case InstallPhaseCommitting:
			if state.Commit != nil && commitProofsEqual(*state.Commit, proof) {
				return state, nil
			}
			return InstallState{}, fmt.Errorf("%w: active commit journal does not match requested proof", ErrInstallStateInvalid)
		case InstallPhaseCompleted:
			if state.Commit != nil && commitProofsEqual(*state.Commit, proof) {
				return state, nil
			}
			return InstallState{}, fmt.Errorf("%w: completed installation cannot begin a different setup commit", ErrInstallStateInvalid)
		default:
			return InstallState{}, fmt.Errorf("%w: unsupported phase %q", ErrInstallStateInvalid, state.Phase)
		}
	})
}

func (store *StateStore) FinalizeCommit(proof CommitProof, at time.Time) (InstallState, error) {
	if err := store.validate(); err != nil {
		return InstallState{}, err
	}
	at, err := validateTransitionTime(at)
	if err != nil {
		return InstallState{}, err
	}
	if err := proof.Validate(); err != nil {
		return InstallState{}, fmt.Errorf("validate commit proof: %w", err)
	}

	return store.withMutationLock(func() (InstallState, error) {
		state, exists, err := store.loadUnlocked()
		if err != nil {
			return InstallState{}, err
		}
		if !exists {
			return InstallState{}, fmt.Errorf("%w: cannot finalize commit without install state", ErrInstallStateInvalid)
		}
		if state.InstallationID != proof.InstallationID {
			return InstallState{}, fmt.Errorf("%w: installation ID does not match install state", ErrInstallStateInvalid)
		}
		if state.Commit == nil || !commitProofsEqual(*state.Commit, proof) {
			return InstallState{}, fmt.Errorf("%w: commit journal does not match requested proof", ErrInstallStateInvalid)
		}

		switch state.Phase {
		case InstallPhaseCommitting:
			state.Phase = InstallPhaseCompleted
			state.EverCompleted = true
			state.UpdatedAt = at
			if err := store.saveUnlocked(state); err != nil {
				return InstallState{}, err
			}
			return state, nil
		case InstallPhaseCompleted:
			return state, nil
		default:
			return InstallState{}, fmt.Errorf("%w: phase %q cannot be finalized", ErrInstallStateInvalid, state.Phase)
		}
	})
}

// ReconcileCompletedCommit updates only the digest of an already completed
// commit while preserving its operation, installation, schema, and revision.
// It is used when an imported database is rebound to an existing administrator.
func (store *StateStore) ReconcileCompletedCommit(proof CommitProof, at time.Time) (InstallState, error) {
	if err := store.validate(); err != nil {
		return InstallState{}, err
	}
	at, err := validateTransitionTime(at)
	if err != nil {
		return InstallState{}, err
	}
	if err := proof.Validate(); err != nil {
		return InstallState{}, fmt.Errorf("validate reconciled commit proof: %w", err)
	}
	return store.withMutationLock(func() (InstallState, error) {
		state, exists, err := store.loadUnlocked()
		if err != nil {
			return InstallState{}, err
		}
		if !exists || state.Phase != InstallPhaseCompleted || !state.EverCompleted || state.Commit == nil {
			return InstallState{}, fmt.Errorf("%w: reconciliation requires completed install state", ErrInstallStateInvalid)
		}
		existing := *state.Commit
		if existing.OperationID != proof.OperationID || existing.InstallationID != proof.InstallationID ||
			existing.RuntimeSchemaVersion != proof.RuntimeSchemaVersion || existing.ConfigRevision != proof.ConfigRevision ||
			state.InstallationID != proof.InstallationID {
			return InstallState{}, fmt.Errorf("%w: reconciled commit identity does not match completed state", ErrInstallStateInvalid)
		}
		if constantTimeDigestEqual(existing.RequestDigest, proof.RequestDigest) {
			return state, nil
		}
		state.Commit = &proof
		state.UpdatedAt = at
		if err := store.saveUnlocked(state); err != nil {
			return InstallState{}, err
		}
		return state, nil
	})
}

func (store *StateStore) loadUnlocked() (InstallState, bool, error) {
	content, err := os.ReadFile(store.path)
	if errors.Is(err, os.ErrNotExist) {
		return InstallState{}, false, nil
	}
	if err != nil {
		return InstallState{}, true, fmt.Errorf("read install state %q: %w", store.path, err)
	}
	if !json.Valid(content) {
		return InstallState{}, true, fmt.Errorf("%w: malformed JSON in %q", ErrInstallStateCorrupt, store.path)
	}
	if err := validateStrictStateJSON(content); err != nil {
		return InstallState{}, true, fmt.Errorf("%w: validate JSON shape in %q: %v", ErrInstallStateInvalid, store.path, err)
	}

	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var state InstallState
	if err := decoder.Decode(&state); err != nil {
		return InstallState{}, true, fmt.Errorf("%w: decode %q: %v", ErrInstallStateInvalid, store.path, err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return InstallState{}, true, fmt.Errorf("%w: decode %q: %v", ErrInstallStateCorrupt, store.path, err)
	}
	if err := state.Validate(); err != nil {
		return InstallState{}, true, fmt.Errorf("%w: validate %q: %v", ErrInstallStateInvalid, store.path, err)
	}
	return state, true, nil
}

func (store *StateStore) saveUnlocked(state InstallState) error {
	if err := state.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInstallStateInvalid, err)
	}
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal install state: %w", err)
	}
	content = append(content, '\n')
	if err := writeStateAtomic(store.path, content, store.operations); err != nil {
		return err
	}
	return nil
}

func (store *StateStore) validate() error {
	if store == nil {
		return fmt.Errorf("state store must not be nil")
	}
	if strings.TrimSpace(store.path) == "" {
		return fmt.Errorf("install state path must not be empty")
	}
	if store.pathErr != nil {
		return fmt.Errorf("normalize install state path: %w", store.pathErr)
	}
	if store.processLock == nil {
		return fmt.Errorf("state store process lock must not be nil")
	}
	if store.lockTimeout <= 0 {
		return fmt.Errorf("state store lock timeout must be positive")
	}
	return store.operations.validate()
}

func (store *StateStore) withMutationLock(operation func() (InstallState, error)) (state InstallState, returnErr error) {
	if err := acquireStateProcessLock(store.processLock, store.lockTimeout); err != nil {
		return InstallState{}, err
	}
	defer store.processLock.Unlock()

	fileLock, err := acquireStateFileLock(store.path+".lock", store.lockTimeout, store.operations)
	if err != nil {
		return InstallState{}, err
	}
	defer func() {
		if releaseErr := fileLock.release(); releaseErr != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("release install state file lock: %w", releaseErr))
		}
	}()
	return operation()
}

func normalizeStatePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("install state path must not be empty")
	}
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	if resolvedDirectory, err := filepath.EvalSymlinks(filepath.Dir(absolute)); err == nil {
		absolute = filepath.Join(resolvedDirectory, filepath.Base(absolute))
	}
	absolute = filepath.Clean(absolute)
	return absolute, nil
}

func normalizeProcessLockKey(path string, caseInsensitive bool) string {
	if caseInsensitive {
		return strings.ToLower(path)
	}
	return path
}

func processLockForPath(path string) *sync.Mutex {
	lock, _ := stateProcessLocks.LoadOrStore(path, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func acquireStateProcessLock(lock *sync.Mutex, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if lock.TryLock() {
			return nil
		}
		if !time.Now().Before(deadline) {
			return ErrInstallStateLockTimeout
		}
		time.Sleep(min(10*time.Millisecond, time.Until(deadline)))
	}
}

func installStatesEqual(left, right InstallState) bool {
	if left.SchemaVersion != right.SchemaVersion || left.InstallationID != right.InstallationID || left.DeploymentRole != right.DeploymentRole || left.Phase != right.Phase || left.EverCompleted != right.EverCompleted || !left.UpdatedAt.Equal(right.UpdatedAt) {
		return false
	}
	if left.Commit == nil || right.Commit == nil {
		if left.Commit != nil || right.Commit != nil {
			return false
		}
	} else if !commitProofsEqual(*left.Commit, *right.Commit) {
		return false
	}
	if left.Attempt == nil || right.Attempt == nil {
		return left.Attempt == nil && right.Attempt == nil
	}
	return setupAttemptsEqual(*left.Attempt, *right.Attempt)
}

func setupAttemptsEqual(left, right SetupAttempt) bool {
	return left.OperationID == right.OperationID && left.ConfigRevision == right.ConfigRevision &&
		constantTimeDigestEqual(left.RequestDigest, right.RequestDigest) &&
		constantTimeDigestEqual(left.AdminCredentialVerifier, right.AdminCredentialVerifier)
}

func commitProofsEqual(left, right CommitProof) bool {
	return left.OperationID == right.OperationID && left.InstallationID == right.InstallationID &&
		left.RuntimeSchemaVersion == right.RuntimeSchemaVersion && left.ConfigRevision == right.ConfigRevision &&
		constantTimeDigestEqual(left.RequestDigest, right.RequestDigest)
}

func constantTimeDigestEqual(left, right string) bool {
	return hmac.Equal([]byte(left), []byte(right))
}

func (operations stateAtomicOps) validate() error {
	if operations.secureDirectory == nil || operations.secureFile == nil || operations.replaceFile == nil || operations.syncDirectory == nil {
		return fmt.Errorf("install state atomic operations are incomplete")
	}
	return nil
}

func writeStateAtomic(path string, content []byte, operations stateAtomicOps) (returnErr error) {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("install state path must not be empty")
	}
	if err := operations.validate(); err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create install state directory %q: %w", directory, err)
	}
	if err := operations.secureDirectory(directory); err != nil {
		return fmt.Errorf("secure install state directory %q: %w", directory, err)
	}

	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return fmt.Errorf("create temporary install state in %q: %w", directory, err)
	}
	temporaryPath := temporary.Name()
	temporaryOpen := true
	defer func() {
		if temporaryOpen {
			if closeErr := temporary.Close(); returnErr == nil && closeErr != nil {
				returnErr = fmt.Errorf("close temporary install state: %w", closeErr)
			}
		}
		if removeErr := os.Remove(temporaryPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) && returnErr == nil {
			returnErr = fmt.Errorf("remove temporary install state: %w", removeErr)
		}
	}()

	if err := operations.secureFile(temporaryPath, temporary); err != nil {
		return fmt.Errorf("secure temporary install state: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		return fmt.Errorf("write temporary install state: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary install state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary install state before replace: %w", err)
	}
	temporaryOpen = false
	if err := operations.replaceFile(temporaryPath, path); err != nil {
		return fmt.Errorf("replace install state %q: %w", path, err)
	}
	if err := operations.syncDirectory(directory); err != nil {
		return fmt.Errorf("sync install state directory %q: %w", directory, err)
	}
	return nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return fmt.Errorf("multiple JSON values are not allowed")
	}
	return err
}

func validateStrictStateJSON(content []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(content))
	if err := walkJSONValueRejectingDuplicateKeys(decoder); err != nil {
		return err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}

	var topLevel map[string]json.RawMessage
	if err := json.Unmarshal(content, &topLevel); err != nil || topLevel == nil {
		return fmt.Errorf("install state must be a JSON object")
	}
	allowedTopLevel := map[string]struct{}{
		"schema_version":  {},
		"installation_id": {},
		"deployment_role": {},
		"phase":           {},
		"ever_completed":  {},
		"updated_at":      {},
		"attempt":         {},
		"commit":          {},
	}
	for key := range topLevel {
		if _, allowed := allowedTopLevel[key]; !allowed {
			return fmt.Errorf("unknown install-state field %q", key)
		}
	}
	requiredTopLevel := []string{"schema_version", "installation_id", "deployment_role", "phase", "ever_completed", "updated_at"}
	for _, key := range requiredTopLevel {
		value, exists := topLevel[key]
		if !exists {
			return fmt.Errorf("required install-state field %q is missing", key)
		}
		if isJSONNull(value) {
			return fmt.Errorf("install-state field %q must not be null", key)
		}
	}
	for key, value := range topLevel {
		if isJSONNull(value) {
			return fmt.Errorf("install-state field %q must not be null", key)
		}
	}

	attemptJSON, hasAttempt := topLevel["attempt"]
	if hasAttempt {
		var attempt map[string]json.RawMessage
		if err := json.Unmarshal(attemptJSON, &attempt); err != nil || attempt == nil {
			return fmt.Errorf("attempt must be a JSON object")
		}
		allowedAttempt := map[string]struct{}{
			"operation_id": {}, "config_revision": {}, "request_digest": {}, "admin_credential_verifier": {},
		}
		for key, value := range attempt {
			if _, allowed := allowedAttempt[key]; !allowed {
				return fmt.Errorf("unknown setup attempt field %q", key)
			}
			if isJSONNull(value) {
				return fmt.Errorf("setup attempt field %q must not be null", key)
			}
		}
		for key := range allowedAttempt {
			if _, exists := attempt[key]; !exists {
				return fmt.Errorf("required setup attempt field %q is missing", key)
			}
		}
	}

	commitJSON, hasCommit := topLevel["commit"]
	if !hasCommit {
		return nil
	}
	var commit map[string]json.RawMessage
	if err := json.Unmarshal(commitJSON, &commit); err != nil || commit == nil {
		return fmt.Errorf("commit must be a JSON object")
	}
	allowedCommit := map[string]struct{}{
		"operation_id":           {},
		"installation_id":        {},
		"runtime_schema_version": {},
		"config_revision":        {},
		"request_digest":         {},
	}
	for key := range commit {
		if _, allowed := allowedCommit[key]; !allowed {
			return fmt.Errorf("unknown commit-journal field %q", key)
		}
	}
	for _, key := range []string{"operation_id", "installation_id", "runtime_schema_version", "config_revision", "request_digest"} {
		value, exists := commit[key]
		if !exists {
			return fmt.Errorf("required commit-journal field %q is missing", key)
		}
		if isJSONNull(value) {
			return fmt.Errorf("commit-journal field %q must not be null", key)
		}
	}
	return nil
}

func isJSONNull(value json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

func walkJSONValueRejectingDuplicateKeys(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("JSON object key is not a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate JSON field %q", key)
			}
			seen[key] = struct{}{}
			if err := walkJSONValueRejectingDuplicateKeys(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return fmt.Errorf("JSON object is not closed")
		}
	case '[':
		for decoder.More() {
			if err := walkJSONValueRejectingDuplicateKeys(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return fmt.Errorf("JSON array is not closed")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	return nil
}

func platformStateAtomicOps() stateAtomicOps {
	return stateAtomicOps{
		secureDirectory: secureStateDirectory,
		secureFile:      secureStateFile,
		replaceFile:     replaceStateFile,
		syncDirectory:   syncStateDirectory,
	}
}
