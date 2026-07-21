package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
)

var (
	ErrInstallStateCorrupt = errors.New("install state is corrupt")
	ErrInstallStateInvalid = errors.New("install state is invalid")
)

type stateAtomicOps struct {
	secureDirectory func(string) error
	secureFile      func(string, *os.File) error
	replaceFile     func(string, string) error
	syncDirectory   func(string) error
}

type StateStore struct {
	path       string
	operations stateAtomicOps
	mu         *sync.Mutex
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
	return &StateStore{
		path:       path,
		operations: platformStateAtomicOps(),
		mu:         &sync.Mutex{},
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
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.loadUnlocked()
}

func (store *StateStore) Save(state InstallState) error {
	if err := store.validate(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.saveUnlocked(state)
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

	store.mu.Lock()
	defer store.mu.Unlock()
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
		state.Phase = InstallPhaseCommitting
		state.Commit = &proof
		state.UpdatedAt = at
		if err := store.saveUnlocked(state); err != nil {
			return InstallState{}, err
		}
		return state, nil
	case InstallPhaseCommitting:
		if state.Commit != nil && *state.Commit == proof {
			return state, nil
		}
		return InstallState{}, fmt.Errorf("%w: active commit journal does not match requested operation", ErrInstallStateInvalid)
	case InstallPhaseCompleted:
		if state.Commit != nil && *state.Commit == proof {
			return state, nil
		}
		return InstallState{}, fmt.Errorf("%w: completed installation cannot begin a different setup commit", ErrInstallStateInvalid)
	default:
		return InstallState{}, fmt.Errorf("%w: unsupported phase %q", ErrInstallStateInvalid, state.Phase)
	}
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

	store.mu.Lock()
	defer store.mu.Unlock()
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
	if state.Commit == nil || *state.Commit != proof {
		return InstallState{}, fmt.Errorf("%w: commit journal does not match requested operation", ErrInstallStateInvalid)
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
	if store.mu == nil {
		return fmt.Errorf("state store mutex must not be nil")
	}
	return store.operations.validate()
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
		"commit":          {},
	}
	for key := range topLevel {
		if _, allowed := allowedTopLevel[key]; !allowed {
			return fmt.Errorf("unknown install-state field %q", key)
		}
	}

	commitJSON, hasCommit := topLevel["commit"]
	if !hasCommit {
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(commitJSON), []byte("null")) {
		return fmt.Errorf("commit must be omitted instead of null")
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
	}
	for key := range commit {
		if _, allowed := allowedCommit[key]; !allowed {
			return fmt.Errorf("unknown commit-journal field %q", key)
		}
	}
	return nil
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
