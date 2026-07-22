package deployctl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/config"
)

type DockerAction string

const (
	DockerActionInstall   DockerAction = "install"
	DockerActionUpdate    DockerAction = "update"
	DockerActionRestart   DockerAction = "restart"
	DockerActionStatus    DockerAction = "status"
	DockerActionUninstall DockerAction = "uninstall"
)

type DockerExecutor struct {
	Runner      ProcessRunner
	ReadFile    func(string) ([]byte, error)
	Environment func() []string
	RuntimeUser func() string
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
	specs, err := BuildDockerProcessSpecs(action, plan, installationID, executor.RuntimeUser(), executor.Environment())
	if err != nil {
		return err
	}
	for _, spec := range specs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := executor.Runner.Run(ctx, spec); err != nil {
			return fmt.Errorf("docker compose %s: %w", dockerSpecOperation(spec), err)
		}
	}
	return nil
}

func BuildDockerProcessSpecs(action DockerAction, plan InstallPlan, installationID, runtimeUser string, baseEnvironment []string) ([]ProcessSpec, error) {
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
	projectName := "app-" + strings.ReplaceAll(parsedInstallationID.String(), "-", "")
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
	case DockerActionStatus:
		return []ProcessSpec{newSpec("ps", "--all")}, nil
	case DockerActionUninstall:
		return []ProcessSpec{newSpec("down", "--remove-orphans")}, nil
	default:
		return nil, fmt.Errorf("unsupported Docker action %q", action)
	}
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
		if !found || strings.HasPrefix(key, "COMPOSE_") || key == "DEPLOYCTL_RUNTIME_DIR" || key == "DEPLOYCTL_RUNTIME_USER" {
			continue
		}
		if _, remove := runtimeKeys[key]; remove {
			continue
		}
		clean = append(clean, entry)
	}
	clean = append(clean, "DEPLOYCTL_RUNTIME_DIR="+runtimeDirectory, "DEPLOYCTL_RUNTIME_USER="+runtimeUser)
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
