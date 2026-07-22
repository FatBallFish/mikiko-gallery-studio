package deployctl

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/setup"
)

const installStagePrefix = ".deployctl-stage-"

func writePrivateFileAtomicNoReplace(path string, content []byte) (returnErr error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	if err := secureInstallDirectory(directory); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, installStagePrefix+filepath.Base(path)+"-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	temporaryOpen := true
	linked := false
	published := false
	defer func() {
		if temporaryOpen {
			returnErr = errors.Join(returnErr, temporary.Close())
		}
		if err := os.Remove(temporaryPath); err != nil && !errors.Is(err, os.ErrNotExist) && !published {
			returnErr = errors.Join(returnErr, err)
		}
		if returnErr != nil && linked {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				returnErr = errors.Join(returnErr, err)
			}
		}
	}()
	if err := secureInstallFile(temporaryPath, temporary); err != nil {
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	temporaryOpen = false
	if err := os.Link(temporaryPath, path); err != nil {
		return err
	}
	linked = true
	if runtime.GOOS != "windows" {
		directoryHandle, err := os.Open(directory)
		if err != nil {
			return err
		}
		syncErr := directoryHandle.Sync()
		closeErr := directoryHandle.Close()
		if err := errors.Join(syncErr, closeErr); err != nil {
			return err
		}
	}
	linked = false
	published = true
	return nil
}

func recoverIncompleteInstall(runtimeEnvPath, statePath, manifestPath string) error {
	runtimeExists, err := regularFileExists(runtimeEnvPath)
	if err != nil {
		return err
	}
	if runtimeExists {
		return cleanupInstallStages(runtimeEnvPath, statePath, manifestPath)
	}
	stateExists, err := regularFileExists(statePath)
	if err != nil {
		return err
	}
	manifestExists, err := regularFileExists(manifestPath)
	if err != nil {
		return err
	}
	if !stateExists && !manifestExists {
		return cleanupInstallStages(runtimeEnvPath, statePath, manifestPath)
	}

	var state setup.InstallState
	if stateExists {
		content, err := os.ReadFile(statePath)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(content, &state); err != nil {
			return fmt.Errorf("refuse to remove unrecognized partial install state: %w", err)
		}
		if err := state.Validate(); err != nil || state.Phase != setup.InstallPhasePending || state.EverCompleted {
			return fmt.Errorf("refuse to remove non-pending install state")
		}
	}

	var manifest deploymentManifest
	if manifestExists {
		content, err := os.ReadFile(manifestPath)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(content, &manifest); err != nil {
			return fmt.Errorf("refuse to remove unrecognized partial deployment manifest: %w", err)
		}
		if manifest.SchemaVersion != 1 || uuid.Validate(manifest.InstallationID) != nil || ValidateInstallPlan(manifest.Plan) != nil {
			return fmt.Errorf("refuse to remove invalid partial deployment manifest")
		}
	}
	if stateExists && manifestExists && state.InstallationID != manifest.InstallationID {
		return fmt.Errorf("partial install state and manifest have different installation identities")
	}
	for _, path := range []string{manifestPath, statePath} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return cleanupInstallStages(runtimeEnvPath, statePath, manifestPath)
}

func regularFileExists(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("install target %s is not a regular file", path)
	}
	return true, nil
}

func cleanupInstallStages(paths ...string) error {
	directories := make(map[string]struct{})
	for _, path := range paths {
		directories[filepath.Dir(path)] = struct{}{}
	}
	for directory := range directories {
		entries, err := os.ReadDir(directory)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), installStagePrefix) {
				if err := os.Remove(filepath.Join(directory, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
		}
	}
	return nil
}
