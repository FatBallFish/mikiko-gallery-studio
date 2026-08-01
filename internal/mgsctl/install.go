package mgsctl

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

type InstallDependencies struct {
	Entropy               io.Reader
	Now                   func() time.Time
	ReadFile              func(string) ([]byte, error)
	PathExists            func(string) (bool, error)
	InspectInstallPath    func(string) error
	MakeDirectory         func(string, os.FileMode) error
	AcquireInstallLock    func(context.Context, string) (func() error, error)
	RecoverIncomplete     func(string, string, string, []string) error
	WriteRuntimeEnv       func(string, []byte) error
	InitializeState       func(string, setup.InstallState) error
	WriteManifest         func(string, []byte) error
	WriteDeploymentFile   func(string, []byte) error
	ReplaceRuntimeEnv     func(string, []byte) error
	ReplaceState          func(string, setup.InstallState) error
	ReplaceManifest       func(string, []byte) error
	ReplaceDeploymentFile func(string, []byte) error
	RemovePath            func(string) error
	ApplyDeployment       func(context.Context, InstallPlan) error
	OverwriteExisting     bool
}

type InstallTargetExistsError struct {
	Path         string
	State        string
	Overwritable bool
}

func (err *InstallTargetExistsError) Error() string {
	switch {
	case err.Overwritable:
		return fmt.Sprintf("install target already exists: %s (recognized incomplete installation; confirm overwrite or rerun with --overwrite)", err.Path)
	case err.State == "completed":
		return fmt.Sprintf("install target already exists: %s (completed installations cannot be overwritten; use mgsctl upgrade or uninstall)", err.Path)
	default:
		return fmt.Sprintf("install target already exists: %s (existing files are not a recognized incomplete installation and were preserved)", err.Path)
	}
}

type ProcessSpec struct {
	Executable  string
	Arguments   []string
	Directory   string
	Environment []string
}

type ProcessRunner interface {
	Run(context.Context, ProcessSpec) error
}

type InstallResult struct {
	RuntimeEnvPath string
	ManifestPath   string
	SetupToken     string
}

func (result InstallResult) String() string {
	return fmt.Sprintf("InstallResult{RuntimeEnvPath:%q, ManifestPath:%q, SetupToken:<redacted>}", result.RuntimeEnvPath, result.ManifestPath)
}
func (result InstallResult) GoString() string { return result.String() }

func (result InstallResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		RuntimeEnvPath string `json:"runtime_env_path"`
		ManifestPath   string `json:"manifest_path"`
		SetupToken     string `json:"setup_token"`
	}{RuntimeEnvPath: result.RuntimeEnvPath, ManifestPath: result.ManifestPath, SetupToken: "REDACTED"})
}

func ExecuteInstall(ctx context.Context, plan InstallPlan, dependencies InstallDependencies) (result InstallResult, returnErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return InstallResult{}, err
	}
	if plan.RequiresEnrollment {
		return InstallResult{}, fmt.Errorf("joined role %q must use cluster join", plan.Role)
	}
	if err := ValidateInstallPlan(plan); err != nil {
		return InstallResult{}, fmt.Errorf("validate install plan: %w", err)
	}
	dependencies = defaultInstallDependencies(dependencies)
	runtimeEnvPath := filepath.Join(plan.RuntimeDir, "config", "runtime.env")
	manifestPath := filepath.Join(plan.RuntimeDir, "deployment.json")
	statePath := filepath.Join(plan.RuntimeDir, "config", "install-state.json")
	deploymentPaths, err := deploymentFilePaths(plan)
	if err != nil {
		return InstallResult{}, err
	}
	installTargets := append([]string{runtimeEnvPath, statePath, manifestPath}, deploymentPaths...)
	for _, directory := range []string{plan.RuntimeDir, filepath.Join(plan.RuntimeDir, "config")} {
		if err := dependencies.MakeDirectory(directory, 0o700); err != nil {
			return InstallResult{}, fmt.Errorf("create runtime directory: %w", err)
		}
	}
	releaseLock, err := dependencies.AcquireInstallLock(ctx, filepath.Join(plan.RuntimeDir, "config", ".mgsctl-install.lock"))
	if err != nil {
		return InstallResult{}, fmt.Errorf("acquire install lock: %w", err)
	}
	defer func() {
		if err := releaseLock(); err != nil && returnErr == nil {
			returnErr = fmt.Errorf("release install lock: %w", err)
		}
	}()
	if err := ctx.Err(); err != nil {
		return InstallResult{}, err
	}
	if err := dependencies.RecoverIncomplete(runtimeEnvPath, statePath, manifestPath, deploymentPaths); err != nil {
		return InstallResult{}, fmt.Errorf("recover incomplete install: %w", err)
	}
	var pendingSnapshot pendingInstallSnapshot
	replaceExisting := false
	firstExisting := ""
	for _, path := range installTargets {
		exists, err := dependencies.PathExists(path)
		if err != nil {
			return InstallResult{}, fmt.Errorf("inspect install target: %w", err)
		}
		if exists && firstExisting == "" {
			firstExisting = path
		}
	}
	if firstExisting != "" {
		snapshot, existingState := loadExistingInstall(runtimeEnvPath, statePath, manifestPath, dependencies.ReadFile, dependencies.InspectInstallPath)
		if existingState == existingInstallPending && installPlansEqual(snapshot.Plan, plan) && !snapshot.GeneratedStale && dependencies.ApplyDeployment != nil {
			if err := dependencies.ApplyDeployment(ctx, plan); err != nil {
				return InstallResult{}, fmt.Errorf("resume deployment: %w", err)
			}
			return snapshot.Result, nil
		}
		if existingState == existingInstallPending && installPlansEqual(snapshot.Plan, plan) && snapshot.GeneratedStale {
			pendingSnapshot = snapshot
			replaceExisting = true
		} else if existingState == existingInstallPending && installPlansEqual(snapshot.Plan, plan) {
			return snapshot.Result, nil
		}
		if !replaceExisting && dependencies.OverwriteExisting {
			switch existingState {
			case existingInstallPending:
				pendingSnapshot = snapshot
				replaceExisting = true
			case existingInstallCompleted:
				return InstallResult{}, &InstallTargetExistsError{Path: firstExisting, State: "completed"}
			default:
				return InstallResult{}, &InstallTargetExistsError{Path: firstExisting, State: "unrecognized"}
			}
		} else if !replaceExisting {
			state := "unrecognized"
			overwritable := false
			if existingState == existingInstallPending {
				state = "pending"
				overwritable = true
			} else if existingState == existingInstallCompleted {
				state = "completed"
			}
			return InstallResult{}, &InstallTargetExistsError{Path: firstExisting, State: state, Overwritable: overwritable}
		}
	}
	var artifacts RuntimeArtifacts
	if replaceExisting {
		artifacts, err = BuildPendingRuntimeArtifacts(plan, pendingSnapshot, dependencies.Entropy, dependencies.Now())
	} else {
		artifacts, err = BuildRuntimeArtifacts(plan, dependencies.Entropy, dependencies.Now())
	}
	if err != nil {
		return InstallResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return InstallResult{}, err
	}
	for _, directory := range []string{filepath.Join(plan.RuntimeDir, "data"), filepath.Join(plan.RuntimeDir, "logs")} {
		if err := dependencies.MakeDirectory(directory, 0o700); err != nil {
			return InstallResult{}, fmt.Errorf("create runtime directory: %w", err)
		}
	}
	if replaceExisting {
		if err := replacePendingInstallArtifacts(dependencies, statePath, manifestPath, runtimeEnvPath, deploymentPaths, artifacts); err != nil {
			return InstallResult{}, err
		}
		if dependencies.ApplyDeployment != nil {
			if err := dependencies.ApplyDeployment(ctx, plan); err != nil {
				return InstallResult{}, fmt.Errorf("apply deployment: %w", err)
			}
		}
		return InstallResult{RuntimeEnvPath: runtimeEnvPath, ManifestPath: manifestPath, SetupToken: artifacts.SetupToken}, nil
	}
	if err := dependencies.InitializeState(statePath, artifacts.InstallState); err != nil {
		return InstallResult{}, installArtifactError("initialize install state", err, nil)
	}
	if err := dependencies.WriteManifest(manifestPath, artifacts.Manifest); err != nil {
		return InstallResult{}, installArtifactError("write deployment manifest", err, rollbackInstallArtifacts(dependencies, statePath))
	}
	published := []string{manifestPath, statePath}
	for index, file := range artifacts.DeploymentFiles {
		path := deploymentPaths[index]
		if err := dependencies.WriteDeploymentFile(path, file.Content); err != nil {
			return InstallResult{}, installArtifactError("write deployment asset", err, rollbackInstallArtifacts(dependencies, published...))
		}
		published = append([]string{path}, published...)
	}
	if err := dependencies.WriteRuntimeEnv(runtimeEnvPath, artifacts.RuntimeEnv); err != nil {
		return InstallResult{}, installArtifactError("write runtime env", err, rollbackInstallArtifacts(dependencies, published...))
	}
	if dependencies.ApplyDeployment != nil {
		if err := dependencies.ApplyDeployment(ctx, plan); err != nil {
			return InstallResult{}, fmt.Errorf("apply deployment: %w", err)
		}
	}
	return InstallResult{RuntimeEnvPath: runtimeEnvPath, ManifestPath: manifestPath, SetupToken: artifacts.SetupToken}, nil
}

type pendingArtifactReplacement struct {
	path    string
	content []byte
	write   func([]byte) error
}

type pendingArtifactBackup struct {
	content []byte
	exists  bool
}

func replacePendingInstallArtifacts(dependencies InstallDependencies, statePath, manifestPath, runtimeEnvPath string, deploymentPaths []string, artifacts RuntimeArtifacts) error {
	stateContent, err := json.MarshalIndent(artifacts.InstallState, "", "  ")
	if err != nil {
		return fmt.Errorf("render install state: %w", err)
	}
	stateContent = append(stateContent, '\n')
	replacements := []pendingArtifactReplacement{
		{
			path: statePath, content: stateContent,
			write: func(content []byte) error {
				var state setup.InstallState
				if err := json.Unmarshal(content, &state); err != nil {
					return fmt.Errorf("decode install state replacement: %w", err)
				}
				return dependencies.ReplaceState(statePath, state)
			},
		},
		{path: manifestPath, content: artifacts.Manifest, write: func(content []byte) error { return dependencies.ReplaceManifest(manifestPath, content) }},
	}
	for index, file := range artifacts.DeploymentFiles {
		path := deploymentPaths[index]
		replacements = append(replacements, pendingArtifactReplacement{
			path: path, content: file.Content,
			write: func(content []byte) error { return dependencies.ReplaceDeploymentFile(path, content) },
		})
	}
	replacements = append(replacements, pendingArtifactReplacement{
		path: runtimeEnvPath, content: artifacts.RuntimeEnv,
		write: func(content []byte) error { return dependencies.ReplaceRuntimeEnv(runtimeEnvPath, content) },
	})

	backups := make(map[string]pendingArtifactBackup, len(replacements))
	for _, replacement := range replacements {
		content, readErr := dependencies.ReadFile(replacement.path)
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return fmt.Errorf("back up pending install artifact %s: %w", replacement.path, readErr)
		}
		backups[replacement.path] = pendingArtifactBackup{content: content, exists: readErr == nil}
	}
	published := make([]pendingArtifactReplacement, 0, len(replacements))
	for _, replacement := range replacements {
		if err := replacement.write(replacement.content); err != nil {
			rollbackErr := restorePendingInstallArtifacts(dependencies, published, backups)
			return installArtifactError("replace pending install artifact", err, rollbackErr)
		}
		published = append(published, replacement)
	}
	return nil
}

func restorePendingInstallArtifacts(dependencies InstallDependencies, published []pendingArtifactReplacement, backups map[string]pendingArtifactBackup) error {
	var rollbackErr error
	for index := len(published) - 1; index >= 0; index-- {
		replacement := published[index]
		backup := backups[replacement.path]
		if !backup.exists {
			if err := dependencies.RemovePath(replacement.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				rollbackErr = errors.Join(rollbackErr, fmt.Errorf("remove newly published %s: %w", replacement.path, err))
			}
			continue
		}
		if err := replacement.write(backup.content); err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("restore %s: %w", replacement.path, err))
		}
	}
	return rollbackErr
}

func installArtifactError(operation string, operationErr, rollbackErr error) error {
	wrapped := fmt.Errorf("%s: %w", operation, operationErr)
	if rollbackErr == nil {
		return wrapped
	}
	return errors.Join(wrapped, fmt.Errorf("rollback incomplete install: %w", rollbackErr))
}

func rollbackInstallArtifacts(dependencies InstallDependencies, paths ...string) error {
	var rollbackErr error
	for _, path := range paths {
		if err := dependencies.RemovePath(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("remove %s: %w", path, err))
		}
	}
	return rollbackErr
}

func defaultInstallDependencies(dependencies InstallDependencies) InstallDependencies {
	if dependencies.Entropy == nil {
		dependencies.Entropy = cryptorand.Reader
	}
	if dependencies.Now == nil {
		dependencies.Now = func() time.Time { return time.Now().UTC() }
	}
	if dependencies.ReadFile == nil {
		dependencies.ReadFile = os.ReadFile
	}
	if dependencies.PathExists == nil {
		dependencies.PathExists = func(path string) (bool, error) {
			_, err := os.Stat(path)
			if err == nil {
				return true, nil
			}
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, err
		}
	}
	if dependencies.InspectInstallPath == nil {
		dependencies.InspectInstallPath = inspectInstallPath
	}
	if dependencies.MakeDirectory == nil {
		dependencies.MakeDirectory = func(path string, mode os.FileMode) error {
			if err := os.MkdirAll(path, mode); err != nil {
				return err
			}
			return secureInstallDirectory(path)
		}
	}
	if dependencies.AcquireInstallLock == nil {
		dependencies.AcquireInstallLock = acquireInstallLock
	}
	if dependencies.RecoverIncomplete == nil {
		dependencies.RecoverIncomplete = recoverIncompleteInstall
	}
	if dependencies.WriteRuntimeEnv == nil {
		dependencies.WriteRuntimeEnv = writePrivateFileAtomicNoReplace
	}
	if dependencies.InitializeState == nil {
		dependencies.InitializeState = func(path string, state setup.InstallState) error {
			content, err := json.MarshalIndent(state, "", "  ")
			if err != nil {
				return fmt.Errorf("render install state: %w", err)
			}
			return writePrivateFileAtomicNoReplace(path, append(content, '\n'))
		}
	}
	if dependencies.WriteManifest == nil {
		dependencies.WriteManifest = writePrivateFileAtomicNoReplace
	}
	if dependencies.WriteDeploymentFile == nil {
		dependencies.WriteDeploymentFile = writeDeploymentFileAtomicNoReplace
	}
	if dependencies.ReplaceRuntimeEnv == nil {
		dependencies.ReplaceRuntimeEnv = writePrivateFileAtomicReplace
	}
	if dependencies.ReplaceState == nil {
		dependencies.ReplaceState = func(path string, state setup.InstallState) error {
			content, err := json.MarshalIndent(state, "", "  ")
			if err != nil {
				return fmt.Errorf("render install state: %w", err)
			}
			return writePrivateFileAtomicReplace(path, append(content, '\n'))
		}
	}
	if dependencies.ReplaceManifest == nil {
		dependencies.ReplaceManifest = writePrivateFileAtomicReplace
	}
	if dependencies.ReplaceDeploymentFile == nil {
		dependencies.ReplaceDeploymentFile = writeDeploymentFileAtomicReplace
	}
	if dependencies.RemovePath == nil {
		dependencies.RemovePath = os.Remove
	}
	return dependencies
}

type existingInstallState int

const (
	existingInstallUnrecognized existingInstallState = iota
	existingInstallPending
	existingInstallCompleted
)

type pendingInstallSnapshot struct {
	Result         InstallResult
	Plan           InstallPlan
	State          setup.InstallState
	Manifest       deploymentManifest
	Runtime        config.RuntimeEnvDocument
	GeneratedStale bool
}

func loadExistingInstall(runtimeEnvPath, statePath, manifestPath string, readFile func(string) ([]byte, error), inspectPath func(string) error) (pendingInstallSnapshot, existingInstallState) {
	for _, path := range []string{runtimeEnvPath, statePath, manifestPath} {
		if inspectPath(path) != nil {
			return pendingInstallSnapshot{}, existingInstallUnrecognized
		}
	}
	manifestContent, err := readFile(manifestPath)
	if err != nil {
		return pendingInstallSnapshot{}, existingInstallUnrecognized
	}
	var manifest deploymentManifest
	if err := json.Unmarshal(manifestContent, &manifest); err != nil || manifest.SchemaVersion != 1 || strings.TrimSpace(manifest.InstallationID) == "" {
		return pendingInstallSnapshot{}, existingInstallUnrecognized
	}
	manifest.Plan.RuntimeDir = filepath.Dir(manifestPath)
	if ValidateInstallPlan(manifest.Plan) != nil {
		return pendingInstallSnapshot{}, existingInstallUnrecognized
	}
	stateContent, err := readFile(statePath)
	if err != nil {
		return pendingInstallSnapshot{}, existingInstallUnrecognized
	}
	var state setup.InstallState
	if err := json.Unmarshal(stateContent, &state); err != nil {
		return pendingInstallSnapshot{}, existingInstallUnrecognized
	}
	if state.EverCompleted || state.Phase == setup.InstallPhaseCompleted {
		return pendingInstallSnapshot{Plan: manifest.Plan, State: state, Manifest: manifest}, existingInstallCompleted
	}
	if state.Validate() != nil || state.Phase != setup.InstallPhasePending || state.InstallationID != manifest.InstallationID {
		return pendingInstallSnapshot{}, existingInstallUnrecognized
	}
	runtimeContent, err := readFile(runtimeEnvPath)
	if err != nil {
		return pendingInstallSnapshot{}, existingInstallUnrecognized
	}
	document, err := config.ParseRuntimeEnv(runtimeContent)
	if err != nil || document.Values["INSTALLATION_ID"] != manifest.InstallationID || document.Values["SETUP_TOKEN"] == "" {
		return pendingInstallSnapshot{}, existingInstallUnrecognized
	}
	setupCompleted, err := strconv.ParseBool(document.Values["SETUP_COMPLETED"])
	if err != nil {
		return pendingInstallSnapshot{}, existingInstallUnrecognized
	}
	if setupCompleted {
		return pendingInstallSnapshot{Plan: manifest.Plan, State: state, Manifest: manifest, Runtime: document}, existingInstallCompleted
	}
	snapshot := pendingInstallSnapshot{
		Result: InstallResult{RuntimeEnvPath: runtimeEnvPath, ManifestPath: manifestPath, SetupToken: document.Values["SETUP_TOKEN"]},
		Plan:   manifest.Plan, State: state, Manifest: manifest, Runtime: document,
	}
	expectedFiles, err := buildDeploymentFiles(manifest.Plan)
	if err != nil || len(manifest.Files) != len(expectedFiles) {
		snapshot.GeneratedStale = true
		return snapshot, existingInstallPending
	}
	for _, file := range expectedFiles {
		relativePath := filepath.ToSlash(file.RelativePath)
		wantHash, exists := manifest.Files[relativePath]
		path := filepath.Join(filepath.Dir(manifestPath), file.RelativePath)
		if inspectPath(path) != nil {
			return pendingInstallSnapshot{}, existingInstallUnrecognized
		}
		content, readErr := readFile(path)
		if !exists || readErr != nil {
			snapshot.GeneratedStale = true
			continue
		}
		digest := sha256.Sum256(content)
		if fmt.Sprintf("%x", digest) != wantHash {
			snapshot.GeneratedStale = true
		}
	}
	return snapshot, existingInstallPending
}

func installPlansEqual(left, right InstallPlan) bool {
	left.RuntimeDir = filepath.Clean(left.RuntimeDir)
	right.RuntimeDir = filepath.Clean(right.RuntimeDir)
	return reflect.DeepEqual(left, right)
}
