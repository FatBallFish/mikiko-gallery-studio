package mgsctl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

// ReconfigureDeploymentDependencies keeps managed middleware running while all
// application processes are stopped. Native and distributed changes fail closed.
func ReconfigureDeploymentDependencies(executors RuntimeExecutors) ReconfigureDependencies {
	specsFor := func(plan InstallPlan) ([]ProcessSpec, error) {
		if runtime.GOOS != "linux" || plan.Mode != config.DeploymentModeDocker {
			return nil, fmt.Errorf("reconfigure runtime currently supports Linux Docker only")
		}
		content, err := os.ReadFile(filepath.Join(plan.RuntimeDir, "config", "runtime.env"))
		if err != nil {
			return nil, fmt.Errorf("read deployment identity failed")
		}
		doc, err := config.ParseRuntimeEnv(content)
		if err != nil {
			return nil, fmt.Errorf("parse deployment identity failed")
		}
		env := os.Environ()
		if executors.Docker.Environment != nil {
			env = executors.Docker.Environment()
		}
		user := dockerRuntimeUser()
		if executors.Docker.RuntimeUser != nil {
			user = executors.Docker.RuntimeUser()
		}
		return BuildDockerProcessSpecsForNode(DockerActionUpdate, plan, doc.Values["INSTALLATION_ID"], doc.Values["CLUSTER_NODE_ID"], user, env)
	}
	run := func(ctx context.Context, plan InstallPlan, action string) error {
		specs, err := specsFor(plan)
		if err != nil {
			return err
		}
		spec := specs[len(specs)-1]
		i := -1
		for n, arg := range spec.Arguments {
			if arg == "up" {
				i = n
				break
			}
		}
		if i < 0 {
			return fmt.Errorf("deployment process is invalid")
		}
		base := append([]string(nil), spec.Arguments[:i]...)
		switch action {
		case "stop":
			spec.Arguments = append(base, append([]string{"stop", "--timeout", "60"}, dockerApplicationServices(plan)...)...)
		case "apply": // Do not pull images or touch managed middleware.
			spec.Arguments = append(base, append([]string{"up", "--detach", "--wait", "--no-deps", "--force-recreate"}, dockerApplicationServices(plan)...)...)
		case "validate":
			return validateDockerReconfigureNginx(ctx, spec, base, executors.Docker.Runner)
		}
		if executors.Docker.Runner == nil {
			return fmt.Errorf("Docker process runner required")
		}
		if err := executors.Docker.Runner.Run(ctx, spec); err != nil {
			return fmt.Errorf("application deployment action failed")
		}
		return nil
	}
	return ReconfigureDependencies{
		OpenInstallationStore: func(ctx context.Context, plan InstallPlan, dsn string) (setup.SetupStoreSession, error) {
			if _, err := specsFor(plan); err != nil {
				return nil, err
			}
			resolved, err := resolveDockerReconfigureDSN(ctx, plan, dsn)
			if err != nil {
				return nil, err
			}
			return entstore.OpenSetupStore(ctx, resolved)
		},
		StopDeployment:  func(ctx context.Context, plan InstallPlan) error { return run(ctx, plan, "stop") },
		ApplyDeployment: func(ctx context.Context, plan InstallPlan) error { return run(ctx, plan, "apply") },
		RefreshAssets: func(ctx context.Context, plan InstallPlan) error {
			return refreshManagedNginxLocked(ctx, plan.RuntimeDir, ManagedNginxDependencies{Validate: func(ctx context.Context, p InstallPlan) error { return run(ctx, p, "validate") }})
		},
	}
}

// validateDockerReconfigureNginx uses Compose's service-level extra_hosts,
// rather than docker-run-only flags. The override contains no environment data.
func validateDockerReconfigureNginx(ctx context.Context, spec ProcessSpec, base []string, runner ProcessRunner) (returnErr error) {
	if runner == nil {
		return fmt.Errorf("Docker process runner required")
	}
	temporary, err := os.CreateTemp("", "mgsctl-nginx-validation-*.json")
	if err != nil {
		return fmt.Errorf("create nginx validation override failed")
	}
	path := temporary.Name()
	defer func() {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			returnErr = fmt.Errorf("nginx validation override cleanup failed")
		}
	}()
	const override = `{"services":{"gateway":{"extra_hosts":{"api":"127.0.0.1","user-web":"127.0.0.1","admin-web":"127.0.0.1","docs-web":"127.0.0.1"}}}}`
	if err := temporary.Chmod(0600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect nginx validation override failed")
	}
	if _, err := temporary.WriteString(override); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write nginx validation override failed")
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close nginx validation override failed")
	}
	spec.Arguments = append(append(append([]string(nil), base...), "--file", path), "run", "--rm", "--no-deps", "gateway", "nginx", "-t")
	if err := runner.Run(ctx, spec); err != nil {
		return fmt.Errorf("validate nginx configuration failed")
	}
	return nil
}

// resolveDockerReconfigureDSN uses only an in-memory address substitution. The
// installation's canonical DSN remains unchanged and bound by its original HMAC.
func resolveDockerReconfigureDSN(ctx context.Context, plan InstallPlan, dsn string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return "", fmt.Errorf("unsupported database connection URL")
	}
	if u.Hostname() != "postgres" {
		return dsn, nil
	}
	if u.Port() != "" && u.Port() != "5432" {
		return "", fmt.Errorf("managed database port mismatch")
	}
	if runtime.GOOS != "linux" || plan.Mode != config.DeploymentModeDocker {
		return "", fmt.Errorf("managed database connection requires Linux Docker")
	}
	content, err := os.ReadFile(filepath.Join(plan.RuntimeDir, "config", "runtime.env"))
	if err != nil {
		return "", fmt.Errorf("read database deployment identity failed")
	}
	doc, err := config.ParseRuntimeEnv(content)
	if err != nil || doc.Values["POSTGRES_MANAGED"] != "true" {
		return "", fmt.Errorf("database is not managed by this installation")
	}
	project, err := dockerProjectName(doc.Values["INSTALLATION_ID"], doc.Values["CLUSTER_NODE_ID"])
	if err != nil {
		return "", fmt.Errorf("invalid database project identity")
	}
	absolute, err := filepath.Abs(plan.RuntimeDir)
	if err != nil {
		return "", err
	}
	// Capture output privately; never forward inspect output or the connection URL.
	command := exec.CommandContext(ctx, "docker", "compose", "--project-directory", absolute, "--env-file", filepath.Join(absolute, "config", "runtime.env"), "--file", filepath.Join(absolute, "compose.yml"), "--project-name", project, "ps", "--quiet", "postgres")
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &bytes.Buffer{}
	if command.Run() != nil {
		return "", fmt.Errorf("resolve managed database container failed")
	}
	id := strings.TrimSpace(output.String())
	if len(id) != 64 || strings.IndexFunc(id, func(r rune) bool { return !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') }) >= 0 {
		return "", fmt.Errorf("managed database container identity invalid")
	}
	command = exec.CommandContext(ctx, "docker", "inspect", id)
	output.Reset()
	command.Stdout = &output
	command.Stderr = &bytes.Buffer{}
	if command.Run() != nil {
		return "", fmt.Errorf("inspect managed database network failed")
	}
	var containers []struct {
		Config          struct{ Labels map[string]string }
		State           struct{ Running bool }
		NetworkSettings struct {
			Networks map[string]struct{ IPAddress string }
		}
	}
	if json.Unmarshal(output.Bytes(), &containers) != nil || len(containers) != 1 {
		return "", fmt.Errorf("managed database network response invalid")
	}
	c := containers[0]
	if !c.State.Running || c.Config.Labels["com.docker.compose.project"] != project || c.Config.Labels["com.docker.compose.service"] != "postgres" {
		return "", fmt.Errorf("managed database container ownership mismatch")
	}
	network, ok := c.NetworkSettings.Networks[project+"_default"]
	if !ok || net.ParseIP(network.IPAddress) == nil {
		return "", fmt.Errorf("managed database bridge address unavailable")
	}
	u.Host = net.JoinHostPort(network.IPAddress, "5432")
	return u.String(), nil
}
