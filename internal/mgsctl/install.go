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
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

type InstallDependencies struct {
	Entropy             io.Reader
	Now                 func() time.Time
	ReadFile            func(string) ([]byte, error)
	PathExists          func(string) (bool, error)
	MakeDirectory       func(string, os.FileMode) error
	AcquireInstallLock  func(context.Context, string) (func() error, error)
	RecoverIncomplete   func(string, string, string, []string) error
	WriteRuntimeEnv     func(string, []byte) error
	InitializeState     func(string, setup.InstallState) error
	WriteManifest       func(string, []byte) error
	WriteDeploymentFile func(string, []byte) error
	RemovePath          func(string) error
	ApplyDeployment     func(context.Context, InstallPlan) error
	OverwriteExisting   bool
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
	var overwritePaths []string
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
		existing, existingPlan, existingState := loadExistingInstall(runtimeEnvPath, statePath, manifestPath, dependencies.ReadFile)
		if existingState == existingInstallPending && reflect.DeepEqual(existingPlan, plan) && dependencies.ApplyDeployment != nil {
			if err := dependencies.ApplyDeployment(ctx, plan); err != nil {
				return InstallResult{}, fmt.Errorf("resume deployment: %w", err)
			}
			return existing, nil
		}
		if dependencies.OverwriteExisting {
			switch existingState {
			case existingInstallPending:
				existingFiles, err := buildDeploymentFiles(existingPlan)
				if err != nil {
					return InstallResult{}, fmt.Errorf("resolve existing deployment files: %w", err)
				}
				overwritePaths = []string{runtimeEnvPath, statePath, manifestPath}
				runtimeRoot := filepath.Dir(manifestPath)
				for _, file := range existingFiles {
					overwritePaths = append(overwritePaths, filepath.Join(runtimeRoot, file.RelativePath))
				}
			case existingInstallCompleted:
				return InstallResult{}, &InstallTargetExistsError{Path: firstExisting, State: "completed"}
			default:
				return InstallResult{}, &InstallTargetExistsError{Path: firstExisting, State: "unrecognized"}
			}
		} else {
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
	artifacts, err := BuildRuntimeArtifacts(plan, dependencies.Entropy, dependencies.Now())
	if err != nil {
		return InstallResult{}, err
	}
	if len(overwritePaths) > 0 {
		if err := rollbackInstallArtifacts(dependencies, overwritePaths...); err != nil {
			return InstallResult{}, fmt.Errorf("remove incomplete installation configuration: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return InstallResult{}, err
	}
	for _, directory := range []string{filepath.Join(plan.RuntimeDir, "data"), filepath.Join(plan.RuntimeDir, "logs")} {
		if err := dependencies.MakeDirectory(directory, 0o700); err != nil {
			return InstallResult{}, fmt.Errorf("create runtime directory: %w", err)
		}
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

func loadExistingInstall(runtimeEnvPath, statePath, manifestPath string, readFile func(string) ([]byte, error)) (InstallResult, InstallPlan, existingInstallState) {
	manifestContent, err := readFile(manifestPath)
	if err != nil {
		return InstallResult{}, InstallPlan{}, existingInstallUnrecognized
	}
	var manifest deploymentManifest
	if err := json.Unmarshal(manifestContent, &manifest); err != nil || manifest.SchemaVersion != 1 || ValidateInstallPlan(manifest.Plan) != nil {
		return InstallResult{}, InstallPlan{}, existingInstallUnrecognized
	}
	expectedFiles, err := buildDeploymentFiles(manifest.Plan)
	if err != nil || len(manifest.Files) != len(expectedFiles) {
		return InstallResult{}, InstallPlan{}, existingInstallUnrecognized
	}
	for _, file := range expectedFiles {
		relativePath := filepath.ToSlash(file.RelativePath)
		wantHash, exists := manifest.Files[relativePath]
		if !exists {
			return InstallResult{}, InstallPlan{}, existingInstallUnrecognized
		}
		content, err := readFile(filepath.Join(filepath.Dir(manifestPath), file.RelativePath))
		if err != nil {
			return InstallResult{}, InstallPlan{}, existingInstallUnrecognized
		}
		digest := sha256.Sum256(content)
		if fmt.Sprintf("%x", digest) != wantHash {
			return InstallResult{}, InstallPlan{}, existingInstallUnrecognized
		}
	}
	stateContent, err := readFile(statePath)
	if err != nil {
		return InstallResult{}, InstallPlan{}, existingInstallUnrecognized
	}
	var state setup.InstallState
	if err := json.Unmarshal(stateContent, &state); err != nil {
		return InstallResult{}, InstallPlan{}, existingInstallUnrecognized
	}
	if state.EverCompleted || state.Phase == setup.InstallPhaseCompleted {
		return InstallResult{}, manifest.Plan, existingInstallCompleted
	}
	if state.Validate() != nil || state.Phase != setup.InstallPhasePending || state.InstallationID != manifest.InstallationID {
		return InstallResult{}, InstallPlan{}, existingInstallUnrecognized
	}
	runtimeContent, err := readFile(runtimeEnvPath)
	if err != nil {
		return InstallResult{}, InstallPlan{}, existingInstallUnrecognized
	}
	document, err := config.ParseRuntimeEnv(runtimeContent)
	if err != nil || document.Values["INSTALLATION_ID"] != manifest.InstallationID || document.Values["SETUP_TOKEN"] == "" {
		return InstallResult{}, InstallPlan{}, existingInstallUnrecognized
	}
	setupCompleted, err := strconv.ParseBool(document.Values["SETUP_COMPLETED"])
	if err != nil {
		return InstallResult{}, InstallPlan{}, existingInstallUnrecognized
	}
	if setupCompleted {
		return InstallResult{}, manifest.Plan, existingInstallCompleted
	}
	return InstallResult{RuntimeEnvPath: runtimeEnvPath, ManifestPath: manifestPath, SetupToken: document.Values["SETUP_TOKEN"]}, manifest.Plan, existingInstallPending
}
