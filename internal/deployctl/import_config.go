package deployctl

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

type ImportConfigOptions struct {
	Source             string
	RuntimeDir         string
	Mode               config.DeploymentMode
	Profile            config.DeploymentProfile
	Topology           config.DeploymentTopology
	Role               config.DeploymentRole
	Components         []Component
	StorageDriver      string
	PublicAPIURL       string
	ApplicationVersion string
	ImageRegistry      string
	ImageTag           string
	ReleaseVersion     string
}

type LegacyCompletionProbe struct {
	MiddlewareReachable bool
	InstallationMatches bool
	AdministratorExists bool
	Commit              *setup.CommitJournal
}

func (probe LegacyCompletionProbe) Complete() bool {
	return probe.MiddlewareReachable && probe.InstallationMatches && probe.AdministratorExists && probe.Commit != nil
}

type ImportConfigDependencies struct {
	Entropy             io.Reader
	Now                 func() time.Time
	ReadFile            func(string) ([]byte, error)
	PathExists          func(string) (bool, error)
	MakeDirectory       func(string, os.FileMode) error
	AcquireLock         func(context.Context, string) (func() error, error)
	ProbeCompletion     func(context.Context, map[string]string) (LegacyCompletionProbe, error)
	WriteRuntimeEnv     func(string, []byte) error
	WriteInstallState   func(string, setup.InstallState) error
	WriteManifest       func(string, []byte) error
	WriteDeploymentFile func(string, []byte) error
	RemovePath          func(string) error
}

type ImportConfigResult struct {
	RuntimeEnvPath string
	ManifestPath   string
	Completed      bool
}

var legacyRuntimeAliases = map[string]string{
	"POSTGRES_DB":                "POSTGRES_DATABASE",
	"PIC_GALLERY_IMAGE_REGISTRY": "IMAGE_REGISTRY",
	"PIC_GALLERY_IMAGE_TAG":      "IMAGE_TAG",
	"NGINX_PORT":                 "GATEWAY_PORT",
	"PROMETHEUS_PORT":            "MONITORING_PORT",
	"BFSS_ENDPOINT":              "STORAGE_S3_ENDPOINT",
	"BFSS_REGION":                "STORAGE_S3_REGION",
	"BFSS_BUCKET":                "STORAGE_S3_BUCKET",
	"BFSS_ACCESS_KEY_ID":         "STORAGE_S3_ACCESS_KEY_ID",
	"BFSS_SECRET_ACCESS_KEY":     "STORAGE_S3_SECRET_ACCESS_KEY",
	"BFSS_FORCE_PATH_STYLE":      "STORAGE_S3_FORCE_PATH_STYLE",
	"BFSS_PREFIX":                "STORAGE_S3_PREFIX",
}

var importProtectedKeys = map[string]struct{}{
	"RUNTIME_SCHEMA_VERSION": {}, "DEPLOYMENT_MODE": {}, "DEPLOYMENT_PROFILE": {},
	"DEPLOYMENT_TOPOLOGY": {}, "DEPLOYMENT_ROLE": {}, "DEPLOYMENT_MODULES": {},
	"POSTGRES_MANAGED": {}, "REDIS_MANAGED": {}, "OBJECT_STORAGE_MANAGED": {},
	"SETUP_COMPLETED": {}, "SETUP_TOKEN": {}, "SETUP_TOKEN_VERSION": {},
	"STORAGE_DRIVER":  {},
	"INSTALLATION_ID": {}, "CLUSTER_NODE_ID": {}, "CONFIG_REVISION": {}, "APPLICATION_VERSION": {},
}

func ImportConfig(ctx context.Context, options ImportConfigOptions, dependencies ImportConfigDependencies) (result ImportConfigResult, returnErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ImportConfigResult{}, err
	}
	if err := validateLegacySource(options.Source); err != nil {
		return ImportConfigResult{}, err
	}
	dependencies = defaultImportConfigDependencies(dependencies)
	source, err := dependencies.ReadFile(options.Source)
	if err != nil {
		return ImportConfigResult{}, fmt.Errorf("read legacy configuration: %w", err)
	}
	legacy, err := config.ParseRuntimeEnv(source)
	if err != nil {
		return ImportConfigResult{}, fmt.Errorf("parse legacy configuration: %w", err)
	}
	if driver := strings.TrimSpace(legacy.Values["STORAGE_DRIVER"]); options.StorageDriver == "" && driver != "" {
		options.StorageDriver = driver
	}
	plan, err := BuildInstallPlan(InstallInput{
		Mode: options.Mode, Profile: options.Profile, Topology: options.Topology, Role: options.Role,
		Components: options.Components, RuntimeDir: filepath.Clean(defaultString(options.RuntimeDir, ".")),
		StorageDriver: options.StorageDriver, PublicAPIURL: options.PublicAPIURL,
		ApplicationVersion: defaultString(options.ApplicationVersion, DefaultApplicationVersion),
		ImageRegistry:      options.ImageRegistry, ImageTag: options.ImageTag, ReleaseVersion: options.ReleaseVersion,
	})
	if err != nil {
		return ImportConfigResult{}, fmt.Errorf("build imported deployment plan: %w", err)
	}
	artifacts, err := BuildRuntimeArtifacts(plan, dependencies.Entropy, dependencies.Now())
	if err != nil {
		return ImportConfigResult{}, fmt.Errorf("build imported runtime artifacts: %w", err)
	}
	generated, err := config.ParseRuntimeEnv(artifacts.RuntimeEnv)
	if err != nil {
		return ImportConfigResult{}, fmt.Errorf("parse generated runtime configuration: %w", err)
	}
	mapLegacyRuntimeValues(generated.Values, legacy.Values)

	probe := LegacyCompletionProbe{}
	if dependencies.ProbeCompletion != nil {
		probe, err = dependencies.ProbeCompletion(ctx, runtimeProbeValues(generated.Values, plan.RuntimeDir))
		if err != nil {
			return ImportConfigResult{}, fmt.Errorf("probe legacy installation: %w", redactRuntimeError(err, generated.Values))
		}
	}
	completed := probe.Complete()
	if completed {
		generated.Values["SETUP_COMPLETED"] = "true"
		delete(generated.Values, "SETUP_TOKEN")
		generated.Values["CONFIG_REVISION"] = strconv.Itoa(probe.Commit.ConfigRevision)
		artifacts.InstallState.Phase = setup.InstallPhaseCompleted
		artifacts.InstallState.EverCompleted = true
		commit := *probe.Commit
		artifacts.InstallState.Commit = &commit
	}
	runtimeEnv, err := config.RenderRuntimeEnv(config.DefaultRuntimeSchema(), generated.Values, nil)
	if err != nil {
		return ImportConfigResult{}, fmt.Errorf("render imported runtime configuration: %w", redactRuntimeError(err, generated.Values))
	}

	runtimeEnvPath := filepath.Join(plan.RuntimeDir, "config", "runtime.env")
	statePath := filepath.Join(plan.RuntimeDir, "config", "install-state.json")
	manifestPath := filepath.Join(plan.RuntimeDir, "deployment.json")
	for _, directory := range []string{plan.RuntimeDir, filepath.Join(plan.RuntimeDir, "config")} {
		if err := dependencies.MakeDirectory(directory, 0o700); err != nil {
			return ImportConfigResult{}, fmt.Errorf("create import runtime directory: %w", err)
		}
	}
	release, err := dependencies.AcquireLock(ctx, filepath.Join(plan.RuntimeDir, "config", ".deployctl-import.lock"))
	if err != nil {
		return ImportConfigResult{}, fmt.Errorf("acquire config import lock: %w", err)
	}
	defer func() {
		if err := release(); err != nil && returnErr == nil {
			returnErr = fmt.Errorf("release config import lock: %w", err)
		}
	}()
	targets := []string{statePath, manifestPath}
	for _, file := range artifacts.DeploymentFiles {
		targets = append(targets, filepath.Join(plan.RuntimeDir, file.RelativePath))
	}
	targets = append(targets, runtimeEnvPath)
	for _, target := range targets {
		exists, inspectErr := dependencies.PathExists(target)
		if inspectErr != nil {
			return ImportConfigResult{}, fmt.Errorf("inspect config import target: %w", inspectErr)
		}
		if exists {
			return ImportConfigResult{}, fmt.Errorf("refuse to overwrite existing config import target %q", target)
		}
	}
	published := make([]string, 0, len(targets))
	fail := func(operation string, cause error) (ImportConfigResult, error) {
		rollbackErr := rollbackImportedTargets(dependencies.RemovePath, published)
		return ImportConfigResult{}, errors.Join(fmt.Errorf("%s: %w", operation, cause), rollbackErr)
	}
	if err := dependencies.WriteInstallState(statePath, artifacts.InstallState); err != nil {
		return fail("write imported install state", err)
	}
	published = append(published, statePath)
	if err := dependencies.WriteManifest(manifestPath, artifacts.Manifest); err != nil {
		return fail("write imported deployment manifest", err)
	}
	published = append(published, manifestPath)
	for _, file := range artifacts.DeploymentFiles {
		path := filepath.Join(plan.RuntimeDir, file.RelativePath)
		if err := dependencies.WriteDeploymentFile(path, file.Content); err != nil {
			return fail("write imported deployment file", err)
		}
		published = append(published, path)
	}
	if err := dependencies.WriteRuntimeEnv(runtimeEnvPath, runtimeEnv); err != nil {
		return fail("write imported runtime configuration", err)
	}
	published = append(published, runtimeEnvPath)
	return ImportConfigResult{RuntimeEnvPath: runtimeEnvPath, ManifestPath: manifestPath, Completed: completed}, nil
}

func ProbeLegacyCompletion(ctx context.Context, values map[string]string) (LegacyCompletionProbe, error) {
	probe := LegacyCompletionProbe{}
	if err := ProductionDoctorDependencies().ProbeMiddleware(ctx, cloneRuntimeValues(values)); err != nil {
		return probe, nil
	}
	probe.MiddlewareReachable = true
	client, err := db.OpenContext(ctx, values["DATABASE_URL"])
	if err != nil {
		return LegacyCompletionProbe{}, redactRuntimeError(err, values)
	}
	defer client.Close()
	installations, err := client.Installation.Query().Limit(2).All(ctx)
	if err != nil || len(installations) != 1 {
		return probe, nil
	}
	installation := installations[0]
	if installation.InstallationID != values["INSTALLATION_ID"] || installation.ConfigSchemaVersion != config.CurrentRuntimeSchemaVersion || installation.SetupOperationID == nil || installation.SetupConfigRevision == nil || installation.SetupRequestDigest == nil || installation.SetupAdminID == nil {
		return probe, nil
	}
	probe.InstallationMatches = true
	adminCount, err := client.AdminUser.Query().Count(ctx)
	if err != nil {
		return probe, nil
	}
	probe.AdministratorExists = adminCount > 0
	probe.Commit = &setup.CommitJournal{
		OperationID: *installation.SetupOperationID, InstallationID: installation.InstallationID,
		RuntimeSchemaVersion: installation.ConfigSchemaVersion, ConfigRevision: *installation.SetupConfigRevision,
		RequestDigest: *installation.SetupRequestDigest,
	}
	if probe.Commit.Validate() != nil {
		probe.InstallationMatches = false
		probe.Commit = nil
	}
	return probe, nil
}

func validateLegacySource(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("legacy configuration source is required")
	}
	switch filepath.Base(filepath.Clean(path)) {
	case ".env", ".env.prod", "backend.env":
		return nil
	default:
		return fmt.Errorf("legacy configuration source must be named .env, .env.prod, or backend.env")
	}
}

func defaultImportConfigDependencies(dependencies ImportConfigDependencies) ImportConfigDependencies {
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
			_, err := os.Lstat(path)
			if err == nil {
				return true, nil
			}
			if errors.Is(err, os.ErrNotExist) {
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
	if dependencies.WriteRuntimeEnv == nil {
		dependencies.WriteRuntimeEnv = writePrivateFileAtomicNoReplace
	}
	if dependencies.AcquireLock == nil {
		dependencies.AcquireLock = acquireInstallLock
	}
	if dependencies.WriteInstallState == nil {
		dependencies.WriteInstallState = func(path string, state setup.InstallState) error {
			content, err := json.MarshalIndent(state, "", "  ")
			if err != nil {
				return err
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

func mapLegacyRuntimeValues(target, legacy map[string]string) {
	known := make(map[string]struct{})
	for _, field := range config.DefaultRuntimeSchema().Fields {
		known[field.Key] = struct{}{}
	}
	for sourceKey, value := range legacy {
		targetKey := sourceKey
		if alias, exists := legacyRuntimeAliases[sourceKey]; exists {
			targetKey = alias
		}
		if _, protected := importProtectedKeys[targetKey]; protected {
			continue
		}
		if _, supported := known[targetKey]; !supported || strings.TrimSpace(value) == "" {
			continue
		}
		target[targetKey] = value
	}
	// Older managed Compose files expressed middleware pieces separately.
	_, legacyDatabaseURL := legacy["DATABASE_URL"]
	_, legacyPostgresPassword := legacy["POSTGRES_PASSWORD"]
	if !legacyDatabaseURL && legacyPostgresPassword && target["POSTGRES_DATABASE"] != "" && target["POSTGRES_USER"] != "" && target["POSTGRES_PASSWORD"] != "" {
		target["DATABASE_URL"] = (&url.URL{Scheme: "postgres", User: url.UserPassword(target["POSTGRES_USER"], target["POSTGRES_PASSWORD"]), Host: "postgres:5432", Path: "/" + target["POSTGRES_DATABASE"], RawQuery: "sslmode=disable"}).String()
	}
	_, legacyRedisURL := legacy["REDIS_URL"]
	_, legacyRedisPassword := legacy["REDIS_PASSWORD"]
	if !legacyRedisURL && legacyRedisPassword && target["REDIS_PASSWORD"] != "" {
		target["REDIS_URL"] = (&url.URL{Scheme: "redis", User: url.UserPassword("", target["REDIS_PASSWORD"]), Host: "redis:6379", Path: "/0"}).String()
	}
	if target["REDIS_KEY_PREFIX"] == "" {
		target["REDIS_KEY_PREFIX"] = "app"
	}
	if target["SETUP_TOKEN_VERSION"] == "" {
		target["SETUP_TOKEN_VERSION"] = strconv.Itoa(1)
	}
}

func rollbackImportedTargets(remove func(string) error, published []string) error {
	var rollbackErr error
	for index := len(published) - 1; index >= 0; index-- {
		if err := remove(published[index]); err != nil && !errors.Is(err, os.ErrNotExist) {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("remove imported target %q: %w", published[index], err))
		}
	}
	return rollbackErr
}

func cloneRuntimeValues(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
