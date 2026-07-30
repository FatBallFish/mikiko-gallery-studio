package mgsctl

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/config"
)

type DockerAction string

const (
	DockerActionInstall          DockerAction = "install"
	DockerActionUpdate           DockerAction = "update"
	DockerActionRestart          DockerAction = "restart"
	DockerActionReloadSetupToken DockerAction = "reload-setup-token"
	DockerActionStatus           DockerAction = "status"
	DockerActionUninstall        DockerAction = "uninstall"
	DockerActionDestroy          DockerAction = "destroy"
)

type DockerExecutor struct {
	Runner          ProcessRunner
	ReadFile        func(string) ([]byte, error)
	Environment     func() []string
	RuntimeUser     func() string
	SourceDirectory func() (string, error)
	Stderr          io.Writer
}

func (executor DockerExecutor) PrepareUpgrade(ctx context.Context, target *UpgradeTarget) error {
	if target == nil {
		return fmt.Errorf("Docker upgrade target is required")
	}
	if executor.Runner == nil {
		return fmt.Errorf("Docker process runner is required")
	}
	seen := make(map[string]struct{})
	for _, component := range componentOrder {
		image, exists := target.Release.Images[component]
		if !exists {
			continue
		}
		ref := upgradeImageReference(target.Plan, component, image)
		if _, exists := seen[ref]; exists {
			continue
		}
		seen[ref] = struct{}{}
		if err := executor.Runner.Run(ctx, ProcessSpec{Executable: "docker", Arguments: []string{"pull", ref}, Directory: target.Plan.RuntimeDir}); err != nil {
			return fmt.Errorf("pull target %s image: %w", component, err)
		}
	}
	migrationRef := upgradeImageReference(target.Plan, ComponentAPI, target.Release.MigrationImage)
	if _, exists := seen[migrationRef]; !exists {
		if err := executor.Runner.Run(ctx, ProcessSpec{Executable: "docker", Arguments: []string{"pull", migrationRef}, Directory: target.Plan.RuntimeDir}); err != nil {
			return fmt.Errorf("pull target migration image: %w", err)
		}
	}
	return nil
}

func (executor DockerExecutor) MigrateUpgrade(ctx context.Context, target UpgradeTarget, runtimeEnvPath string) error {
	if executor.Runner == nil {
		return fmt.Errorf("Docker process runner is required")
	}
	if executor.ReadFile == nil {
		executor.ReadFile = os.ReadFile
	}
	if executor.Environment == nil {
		executor.Environment = os.Environ
	}
	if executor.RuntimeUser == nil {
		executor.RuntimeUser = dockerRuntimeUser
	}
	content, err := executor.ReadFile(runtimeEnvPath)
	if err != nil {
		return fmt.Errorf("read Docker runtime environment: %w", err)
	}
	document, err := config.ParseRuntimeEnv(content)
	if err != nil {
		return fmt.Errorf("parse Docker runtime environment: %w", err)
	}
	spec, err := BuildDockerMigrationProcessSpec(
		target, document.Values["INSTALLATION_ID"], document.Values["CLUSTER_NODE_ID"], executor.RuntimeUser(), executor.Environment(),
	)
	if err != nil {
		return err
	}
	if err := executor.Runner.Run(ctx, spec); err != nil {
		return fmt.Errorf("run target Docker database migration: %w", err)
	}
	return nil
}

func (executor DockerExecutor) Preflight(ctx context.Context, plan InstallPlan) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ValidateInstallPlan(plan); err != nil {
		return fmt.Errorf("validate Docker plan: %w", err)
	}
	if plan.Mode != config.DeploymentModeDocker {
		return fmt.Errorf("Docker executor cannot preflight deployment mode %q", plan.Mode)
	}
	if executor.Runner == nil {
		return fmt.Errorf("Docker process runner is required")
	}
	for _, spec := range []ProcessSpec{
		{Executable: "docker", Arguments: []string{"version", "--format", "{{.Server.Version}}"}, Directory: plan.RuntimeDir},
		{Executable: "docker", Arguments: []string{"compose", "version"}, Directory: plan.RuntimeDir},
	} {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := executor.Runner.Run(ctx, spec); err != nil {
			return fmt.Errorf("Docker preflight %s: %w", strings.Join(spec.Arguments, " "), err)
		}
	}
	return nil
}

func (executor DockerExecutor) Run(ctx context.Context, action DockerAction, plan InstallPlan) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if executor.Runner == nil {
		return fmt.Errorf("Docker process runner is required")
	}
	if executor.ReadFile == nil {
		executor.ReadFile = os.ReadFile
	}
	if executor.Environment == nil {
		executor.Environment = os.Environ
	}
	if executor.RuntimeUser == nil {
		executor.RuntimeUser = dockerRuntimeUser
	}
	if executor.SourceDirectory == nil {
		executor.SourceDirectory = resolveDockerBuildSourceDirectory
	}
	if executor.Stderr == nil {
		executor.Stderr = io.Discard
	}
	runtimeEnvPath := filepath.Join(plan.RuntimeDir, "config", "runtime.env")
	content, err := executor.ReadFile(runtimeEnvPath)
	if err != nil {
		return fmt.Errorf("read Docker runtime environment: %w", err)
	}
	document, err := config.ParseRuntimeEnv(content)
	if err != nil {
		return fmt.Errorf("parse Docker runtime environment: %w", err)
	}
	installationID := document.Values["INSTALLATION_ID"]
	specs, err := BuildDockerProcessSpecsForNode(action, plan, installationID, document.Values["CLUSTER_NODE_ID"], executor.RuntimeUser(), executor.Environment())
	if err != nil {
		return err
	}
	for index, spec := range specs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := executor.Runner.Run(ctx, spec); err != nil {
			processErr := fmt.Errorf("docker compose %s: %w", dockerSpecOperation(spec), err)
			if index == 0 && dockerSpecOperation(spec) == "pull" && (action == DockerActionInstall || action == DockerActionUpdate) {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				fmt.Fprintln(executor.Stderr, "Docker image pull failed; building application images locally from the complete source checkout.")
				if buildErr := executor.buildApplicationImages(ctx, plan, spec.Environment); buildErr != nil {
					return errors.Join(processErr, fmt.Errorf("local Docker image fallback: %w", buildErr))
				}
				continue
			}
			return processErr
		}
	}
	return nil
}

func (executor DockerExecutor) buildApplicationImages(ctx context.Context, plan InstallPlan, environment []string) error {
	sourceDirectory, err := executor.SourceDirectory()
	if err != nil {
		return err
	}
	sourceDirectory, err = filepath.Abs(sourceDirectory)
	if err != nil {
		return fmt.Errorf("resolve source checkout: %w", err)
	}
	registry := strings.TrimSuffix(defaultString(strings.TrimSpace(plan.ImageRegistry), "docker.io/fatballfish"), "/")
	definitions := map[Component]struct {
		image      string
		dockerfile string
	}{
		ComponentAPI:      {image: "mikiko-gallery-studio-api", dockerfile: "Dockerfile.api"},
		ComponentWorker:   {image: "mikiko-gallery-studio-worker", dockerfile: "Dockerfile.worker"},
		ComponentUserWeb:  {image: "mikiko-gallery-studio-user-web", dockerfile: "Dockerfile.user-web"},
		ComponentAdminWeb: {image: "mikiko-gallery-studio-admin-web", dockerfile: "Dockerfile.admin-web"},
		ComponentDocsWeb:  {image: "mikiko-gallery-studio-docs-web", dockerfile: "Dockerfile.docs-web"},
	}
	built := 0
	for _, component := range plan.Components {
		definition, ok := definitions[component]
		if !ok {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		spec := ProcessSpec{
			Executable: "docker",
			Arguments: []string{
				"build", "--tag", registry + "/" + definition.image + ":" + plan.ImageTag,
				"--file", filepath.Join(sourceDirectory, definition.dockerfile), sourceDirectory,
			},
			Directory:   sourceDirectory,
			Environment: slices.Clone(environment),
		}
		if err := executor.Runner.Run(ctx, spec); err != nil {
			return fmt.Errorf("build %s: %w", definition.image, err)
		}
		built++
	}
	if built == 0 {
		return fmt.Errorf("deployment contains no locally buildable application images")
	}
	return nil
}

func resolveDockerBuildSourceDirectory() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("MGSCTL_SOURCE_DIR")); configured != "" {
		if err := validateDockerBuildSourceDirectory(configured); err != nil {
			return "", fmt.Errorf("validate MGSCTL_SOURCE_DIR: %w", err)
		}
		return configured, nil
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	for candidate := filepath.Clean(workingDirectory); ; candidate = filepath.Dir(candidate) {
		if validateDockerBuildSourceDirectory(candidate) == nil {
			return candidate, nil
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			break
		}
	}
	return "", fmt.Errorf("complete source checkout not found; rerun scripts/install.sh from the project checkout or publish the requested images")
}

func validateDockerBuildSourceDirectory(directory string) error {
	required := []string{
		"go.mod", "Makefile", "Dockerfile.api", "Dockerfile.worker",
		"Dockerfile.user-web", "Dockerfile.admin-web", "Dockerfile.docs-web",
	}
	for _, relativePath := range required {
		info, err := os.Stat(filepath.Join(directory, relativePath))
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("complete source checkout is missing %s", relativePath)
		}
	}
	return nil
}

func BuildDockerProcessSpecs(action DockerAction, plan InstallPlan, installationID, runtimeUser string, baseEnvironment []string) ([]ProcessSpec, error) {
	return BuildDockerProcessSpecsForNode(action, plan, installationID, "", runtimeUser, baseEnvironment)
}

func BuildDockerProcessSpecsForNode(action DockerAction, plan InstallPlan, installationID, nodeID, runtimeUser string, baseEnvironment []string) ([]ProcessSpec, error) {
	if err := ValidateInstallPlan(plan); err != nil {
		return nil, fmt.Errorf("validate Docker plan: %w", err)
	}
	if plan.Mode != config.DeploymentModeDocker {
		return nil, fmt.Errorf("Docker executor cannot run deployment mode %q", plan.Mode)
	}
	parsedInstallationID, err := uuid.Parse(installationID)
	if err != nil {
		return nil, fmt.Errorf("validate Docker installation identity: %w", err)
	}
	absoluteRuntime, err := filepath.Abs(plan.RuntimeDir)
	if err != nil {
		return nil, fmt.Errorf("resolve Docker runtime directory: %w", err)
	}
	profiles, err := DockerProfiles(plan)
	if err != nil {
		return nil, err
	}
	projectName, err := dockerProjectName(parsedInstallationID.String(), nodeID)
	if err != nil {
		return nil, err
	}
	baseArguments := []string{
		"compose",
		"--project-directory", absoluteRuntime,
		"--env-file", filepath.Join(absoluteRuntime, "config", "runtime.env"),
		"--file", filepath.Join(absoluteRuntime, "compose.yml"),
		"--project-name", projectName,
	}
	for _, profile := range profiles {
		baseArguments = append(baseArguments, "--profile", profile)
	}
	environment := sanitizeDockerEnvironment(baseEnvironment, absoluteRuntime, runtimeUser)
	newSpec := func(arguments ...string) ProcessSpec {
		return ProcessSpec{
			Executable: "docker", Arguments: append(slices.Clone(baseArguments), arguments...),
			Directory: absoluteRuntime, Environment: slices.Clone(environment),
		}
	}
	switch action {
	case DockerActionInstall, DockerActionUpdate:
		specs := []ProcessSpec{newSpec("pull")}
		managedServices := dockerManagedServices(plan)
		if len(managedServices) > 0 {
			specs = append(specs, newSpec(append([]string{"up", "--detach", "--wait"}, managedServices...)...))
		}
		if slices.Contains(plan.Components, ComponentMinIO) {
			specs = append(specs, newSpec("run", "--rm", "--no-deps", "minio-init"))
		}
		applicationServices := dockerApplicationServices(plan)
		specs = append(specs, newSpec(append([]string{"up", "--detach", "--wait", "--remove-orphans"}, applicationServices...)...))
		return specs, nil
	case DockerActionRestart:
		return []ProcessSpec{newSpec("restart")}, nil
	case DockerActionReloadSetupToken:
		services := []string{"api"}
		if slices.Contains(plan.Components, ComponentGateway) {
			services = append(services, "gateway")
		}
		return []ProcessSpec{newSpec(append([]string{"restart"}, services...)...)}, nil
	case DockerActionStatus:
		return []ProcessSpec{newSpec("ps", "--all")}, nil
	case DockerActionUninstall:
		return []ProcessSpec{newSpec("down", "--remove-orphans")}, nil
	case DockerActionDestroy:
		return []ProcessSpec{newSpec("down", "--volumes", "--remove-orphans")}, nil
	default:
		return nil, fmt.Errorf("unsupported Docker action %q", action)
	}
}

func BuildDockerMigrationProcessSpec(target UpgradeTarget, installationID, nodeID, runtimeUser string, baseEnvironment []string) (ProcessSpec, error) {
	if target.Plan.Mode != config.DeploymentModeDocker {
		return ProcessSpec{}, fmt.Errorf("Docker migration cannot use deployment mode %q", target.Plan.Mode)
	}
	parsedInstallationID, err := uuid.Parse(installationID)
	if err != nil {
		return ProcessSpec{}, fmt.Errorf("validate Docker installation identity: %w", err)
	}
	projectName, err := dockerProjectName(parsedInstallationID.String(), nodeID)
	if err != nil {
		return ProcessSpec{}, err
	}
	if !validSHA256Digest(target.Release.MigrationImage.Digest) || strings.TrimSpace(target.Release.MigrationImage.Repository) == "" {
		return ProcessSpec{}, fmt.Errorf("target API migration image must use an immutable digest")
	}
	absoluteRuntime, err := filepath.Abs(target.Plan.RuntimeDir)
	if err != nil {
		return ProcessSpec{}, fmt.Errorf("resolve Docker runtime directory: %w", err)
	}
	imageRef := upgradeImageReference(target.Plan, ComponentAPI, target.Release.MigrationImage)
	arguments := []string{
		"run", "--rm", "--network", projectName + "_default", "--user", runtimeUser,
		"--volume", filepath.Join(absoluteRuntime, "config") + ":/app/config:ro",
		"--entrypoint", "mikiko-gallery-studio-db-migrate", imageRef,
		"--env-file", "/app/config/runtime.env",
	}
	return ProcessSpec{
		Executable: "docker", Arguments: arguments, Directory: absoluteRuntime,
		Environment: sanitizeDockerEnvironment(baseEnvironment, absoluteRuntime, runtimeUser),
	}, nil
}

func dockerProjectName(installationID, nodeID string) (string, error) {
	parsedInstallationID, err := uuid.Parse(installationID)
	if err != nil {
		return "", fmt.Errorf("validate Docker installation identity: %w", err)
	}
	projectName := "app-" + strings.ReplaceAll(parsedInstallationID.String(), "-", "")
	if strings.TrimSpace(nodeID) == "" {
		return projectName, nil
	}
	parsedNodeID, err := uuid.Parse(nodeID)
	if err != nil {
		return "", fmt.Errorf("validate Docker cluster node identity: %w", err)
	}
	nodeDigest := sha256.Sum256([]byte(parsedNodeID.String()))
	return projectName + fmt.Sprintf("-%x", nodeDigest[:6]), nil
}

func upgradeImageReference(plan InstallPlan, component Component, image ReleaseImage) string {
	repository := strings.TrimSpace(image.Repository)
	if registry := strings.TrimRight(strings.TrimSpace(plan.ImageRegistry), "/"); registry != "" {
		repository = registry + "/mikiko-gallery-studio-" + string(component)
	}
	return repository + "@" + image.Digest
}

func dockerManagedServices(plan InstallPlan) []string {
	services := make([]string, 0, 3)
	for _, component := range plan.Components {
		switch component {
		case ComponentPostgres, ComponentRedis, ComponentMinIO:
			services = append(services, string(component))
		}
	}
	return services
}

func dockerApplicationServices(plan InstallPlan) []string {
	services := make([]string, 0, len(plan.Components))
	for _, component := range plan.Components {
		switch component {
		case ComponentPostgres, ComponentRedis, ComponentMinIO:
			continue
		default:
			services = append(services, string(component))
		}
	}
	return services
}

func DockerProfiles(plan InstallPlan) ([]string, error) {
	if plan.Mode != config.DeploymentModeDocker {
		return nil, fmt.Errorf("deployment mode %q does not use Docker profiles", plan.Mode)
	}
	if plan.Profile == config.DeploymentProfileFull {
		return []string{"full"}, nil
	}
	if plan.Profile == config.DeploymentProfileCore && (plan.Role == config.DeploymentRoleSingle || plan.Role == config.DeploymentRoleControl) {
		return []string{"core"}, nil
	}
	profiles := make([]string, 0, len(plan.Components))
	for _, component := range plan.Components {
		profiles = append(profiles, string(component))
	}
	return profiles, nil
}

func DockerServices(plan InstallPlan) ([]string, error) {
	if plan.Mode != config.DeploymentModeDocker {
		return nil, fmt.Errorf("deployment mode %q does not use Docker services", plan.Mode)
	}
	services := make([]string, 0, len(plan.Components)+1)
	for _, component := range plan.Components {
		services = append(services, string(component))
		if component == ComponentMinIO {
			services = append(services, "minio-init")
		}
	}
	return services, nil
}

func sanitizeDockerEnvironment(base []string, runtimeDirectory, runtimeUser string) []string {
	runtimeKeys := make(map[string]struct{})
	for _, field := range config.DefaultRuntimeSchema().Fields {
		runtimeKeys[field.Key] = struct{}{}
	}
	for _, key := range []string{"PIC_GALLERY_IMAGE_REGISTRY", "PIC_GALLERY_IMAGE_TAG", "NGINX_PORT", "PROMETHEUS_PORT"} {
		runtimeKeys[key] = struct{}{}
	}
	clean := make([]string, 0, len(base)+2)
	for _, entry := range base {
		key, _, found := strings.Cut(entry, "=")
		if !found || strings.HasPrefix(key, "COMPOSE_") || key == "MGSCTL_RUNTIME_DIR" || key == "MGSCTL_RUNTIME_USER" {
			continue
		}
		if _, remove := runtimeKeys[key]; remove {
			continue
		}
		clean = append(clean, entry)
	}
	clean = append(clean, "MGSCTL_RUNTIME_DIR="+runtimeDirectory, "MGSCTL_RUNTIME_USER="+runtimeUser)
	return clean
}

func dockerSpecOperation(spec ProcessSpec) string {
	for _, argument := range spec.Arguments {
		switch argument {
		case "pull", "up", "run", "restart", "ps", "down":
			return argument
		}
	}
	return "command"
}
