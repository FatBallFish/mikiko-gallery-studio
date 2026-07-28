package deployctl

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
)

type Terminal interface {
	Interactive() bool
	Prompt(context.Context, string, string) (string, error)
	Confirm(context.Context, string) (bool, error)
}

type CLIDependencies struct {
	Terminal             Terminal
	Stdout               io.Writer
	Stderr               io.Writer
	BuildInfo            BuildInfo
	StdoutIsTerminal     func(io.Writer) bool
	Install              InstallDependencies
	ImportConfig         ImportConfigDependencies
	Doctor               DoctorDependencies
	Upgrade              UpgradeDependencies
	Uninstall            UninstallDependencies
	SetupTokenReset      SetupTokenResetDependencies
	ClusterJoin          ClusterJoinDependencies
	ExecuteRuntimeAction func(context.Context, CommandKind, string) error
	ExecuteImportConfig  func(context.Context, ImportConfigOptions, ImportConfigDependencies) (ImportConfigResult, error)
	ExecuteDoctor        func(context.Context, string, DoctorDependencies) DoctorReport
	ExecuteUpgrade       func(context.Context, UpgradeOptions, UpgradeDependencies) (UpgradeResult, error)
	ExecuteUninstall     func(context.Context, UninstallOptions, UninstallDependencies) error
	CreateClusterToken   func(context.Context, string, ClusterTokenCreateOptions) (ClusterTokenCreateResult, error)
	ExecuteClusterJoin   func(context.Context, ClusterJoinOptions, ClusterJoinDependencies) (ClusterJoinResult, error)
}

func Run(ctx context.Context, args []string, dependencies CLIDependencies) int {
	if ctx == nil {
		ctx = context.Background()
	}
	if dependencies.Stdout == nil {
		dependencies.Stdout = io.Discard
	}
	if dependencies.Stderr == nil {
		dependencies.Stderr = io.Discard
	}
	if dependencies.StdoutIsTerminal == nil {
		dependencies.StdoutIsTerminal = writerIsTerminal
	}
	command, err := ParseCommand(args)
	if err != nil {
		fmt.Fprintf(dependencies.Stderr, "deployctl: %v\n", err)
		return 2
	}
	switch command.Kind {
	case CommandVersion:
		info := NormalizeBuildInfo(dependencies.BuildInfo)
		if command.Version.JSON {
			encoded, encodeErr := info.JSON()
			if encodeErr != nil {
				return writeRunError(dependencies.Stderr, fmt.Errorf("encode build information: %w", encodeErr))
			}
			fmt.Fprintln(dependencies.Stdout, string(encoded))
			return 0
		}
		fmt.Fprintln(dependencies.Stdout, info.Text())
		return 0
	case CommandImportConfig:
		execute := dependencies.ExecuteImportConfig
		if execute == nil {
			execute = ImportConfig
		}
		result, executeErr := execute(ctx, *command.ImportConfig, dependencies.ImportConfig)
		if executeErr != nil {
			return writeRunError(dependencies.Stderr, executeErr)
		}
		fmt.Fprintf(dependencies.Stdout, "Imported runtime configuration: %s\nSetup completed: %t\n", result.RuntimeEnvPath, result.Completed)
		return 0
	case CommandStatus, CommandRestart:
		if dependencies.ExecuteRuntimeAction == nil {
			return writeRunError(dependencies.Stderr, fmt.Errorf("runtime action dependency is required"))
		}
		if executeErr := dependencies.ExecuteRuntimeAction(ctx, command.Kind, command.RuntimeDir); executeErr != nil {
			return writeRunError(dependencies.Stderr, executeErr)
		}
		return 0
	case CommandDoctor:
		execute := dependencies.ExecuteDoctor
		if execute == nil {
			execute = Doctor
		}
		report := execute(ctx, command.RuntimeDir, dependencies.Doctor)
		fmt.Fprint(dependencies.Stdout, report.String())
		if !report.Healthy() {
			return 1
		}
		return 0
	case CommandUpgrade:
		execute := dependencies.ExecuteUpgrade
		if execute == nil {
			execute = Upgrade
		}
		result, executeErr := execute(ctx, *command.Upgrade, dependencies.Upgrade)
		if executeErr != nil {
			return writeRunError(dependencies.Stderr, executeErr)
		}
		fmt.Fprintf(dependencies.Stdout, "Upgraded application from %s to %s. Migration executed: %t.\n", result.PreviousVersion, result.CurrentVersion, result.Migrated)
		return 0
	case CommandUninstall:
		execute := dependencies.ExecuteUninstall
		if execute == nil {
			execute = Uninstall
		}
		if executeErr := execute(ctx, *command.Uninstall, dependencies.Uninstall); executeErr != nil {
			return writeRunError(dependencies.Stderr, executeErr)
		}
		if command.Uninstall.DeleteData {
			fmt.Fprintln(dependencies.Stdout, "Services, persistent data, and runtime configuration removed.")
		} else {
			fmt.Fprintln(dependencies.Stdout, "Services stopped. Runtime configuration and persistent data were preserved.")
		}
		return 0
	case CommandSetupStatus:
		status, executeErr := LoadSetupStatus(command.RuntimeDir)
		if executeErr != nil {
			return writeRunError(dependencies.Stderr, executeErr)
		}
		fmt.Fprintf(dependencies.Stdout, "Installation: %s\nRole: %s\nSetup phase: %s\nCompleted: %t\nToken version: %d\n", status.InstallationID, status.Role, status.Phase, status.Completed, status.TokenVersion)
		return 0
	case CommandSetupTokenShow:
		token, executeErr := ShowSetupToken(command.RuntimeDir)
		if executeErr != nil {
			return writeRunError(dependencies.Stderr, executeErr)
		}
		fmt.Fprintf(dependencies.Stdout, "Setup token: %s\n", token)
		return 0
	case CommandSetupTokenReset:
		token, executeErr := ResetSetupToken(ctx, command.RuntimeDir, dependencies.SetupTokenReset)
		if executeErr != nil {
			return writeRunError(dependencies.Stderr, executeErr)
		}
		fmt.Fprintf(dependencies.Stdout, "Setup token reset. New token: %s\n", token)
		return 0
	case CommandClusterTokenCreate:
		if dependencies.CreateClusterToken == nil {
			return writeRunError(dependencies.Stderr, fmt.Errorf("cluster token creation dependency is required"))
		}
		result, executeErr := dependencies.CreateClusterToken(ctx, command.RuntimeDir, *command.ClusterTokenCreate)
		if executeErr != nil {
			return writeRunError(dependencies.Stderr, executeErr)
		}
		fmt.Fprintf(dependencies.Stdout, "Cluster join token (%s, expires %s): %s\n", result.Role, result.ExpiresAt.UTC().Format(time.RFC3339), result.Credential)
		return 0
	}
	if command.Kind == CommandClusterJoin {
		execute := dependencies.ExecuteClusterJoin
		if execute == nil {
			execute = ExecuteClusterJoin
		}
		result, executeErr := execute(ctx, *command.ClusterJoin, dependencies.ClusterJoin)
		if executeErr != nil {
			return writeRunError(dependencies.Stderr, executeErr)
		}
		fmt.Fprintf(dependencies.Stdout, "Joined cluster node %s as %s.\nRuntime configuration: %s\n", result.NodeID, result.Role, result.RuntimeEnvPath)
		return 0
	}
	if command.Kind != CommandInstall {
		return writeRunError(dependencies.Stderr, fmt.Errorf("unsupported command %q", command.Kind))
	}
	input := *command.Install
	if input.Interactive {
		if dependencies.Terminal == nil || !dependencies.Terminal.Interactive() {
			fmt.Fprintln(dependencies.Stderr, "deployctl: interactive install requires a terminal; provide all required flags with --yes")
			return 2
		}
		input, err = resolveInteractiveInstall(ctx, input, dependencies.Terminal)
		if err != nil {
			return writeRunError(dependencies.Stderr, err)
		}
	}
	plan, err := BuildInstallPlan(input)
	if err != nil {
		fmt.Fprintf(dependencies.Stderr, "deployctl: invalid install plan: %v\n", err)
		return 2
	}
	if input.Interactive {
		confirmed, confirmErr := dependencies.Terminal.Confirm(ctx, fmt.Sprintf("Install %s/%s in %s?", plan.Mode, plan.Profile, plan.RuntimeDir))
		if confirmErr != nil {
			return writeRunError(dependencies.Stderr, confirmErr)
		}
		if !confirmed {
			fmt.Fprintln(dependencies.Stdout, "Installation cancelled.")
			return 0
		}
	}
	result, err := ExecuteInstall(ctx, plan, dependencies.Install)
	if err != nil {
		return writeRunError(dependencies.Stderr, err)
	}
	fmt.Fprintf(dependencies.Stdout, "Runtime configuration: %s\n", result.RuntimeEnvPath)
	if input.Interactive && dependencies.StdoutIsTerminal(dependencies.Stdout) {
		fmt.Fprintf(dependencies.Stdout, "Setup token: %s\n", result.SetupToken)
	} else {
		fmt.Fprintf(dependencies.Stdout, "Setup token stored securely. Run deployctl setup token show --runtime-dir %q on the deployment host to display it.\n", plan.RuntimeDir)
	}
	return 0
}

func writerIsTerminal(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func resolveInteractiveInstall(ctx context.Context, input InstallInput, terminal Terminal) (InstallInput, error) {
	var err error
	if input.Mode == "" {
		value, promptErr := terminal.Prompt(ctx, "Deployment mode", "docker")
		if promptErr != nil {
			return InstallInput{}, promptErr
		}
		input.Mode = config.DeploymentMode(value)
	}
	if input.Profile == "" {
		value, promptErr := terminal.Prompt(ctx, "Deployment profile", "core")
		if promptErr != nil {
			return InstallInput{}, promptErr
		}
		input.Profile = config.DeploymentProfile(value)
	}
	if input.Topology == "" {
		value, promptErr := terminal.Prompt(ctx, "Deployment topology", "single")
		if promptErr != nil {
			return InstallInput{}, promptErr
		}
		input.Topology = config.DeploymentTopology(value)
	}
	if input.Role == "" {
		if input.Topology == config.DeploymentTopologyCluster {
			input.Role = config.DeploymentRoleControl
		} else {
			input.Role = config.DeploymentRoleSingle
		}
	}
	if input.StorageDriver == "" && input.Role != config.DeploymentRoleWeb {
		fallback := "local"
		if input.Topology == config.DeploymentTopologyCluster || input.Profile == config.DeploymentProfileFull {
			fallback = "s3"
		}
		input.StorageDriver, err = terminal.Prompt(ctx, "Object storage driver", fallback)
		if err != nil {
			return InstallInput{}, err
		}
	}
	if input.Profile == config.DeploymentProfileCustom && len(input.Components) == 0 {
		value, promptErr := terminal.Prompt(ctx, "Components", "api,worker,user-web,admin-web,docs-web,gateway")
		if promptErr != nil {
			return InstallInput{}, promptErr
		}
		input.Components = parseComponents(value)
	}
	if !input.RuntimeDirExplicit {
		input.RuntimeDir, err = terminal.Prompt(ctx, "Runtime directory", defaultString(input.RuntimeDir, "."))
		if err != nil {
			return InstallInput{}, err
		}
	}
	if !input.ApplicationVersionExplicit {
		input.ApplicationVersion, err = terminal.Prompt(ctx, "Application version", defaultString(input.ApplicationVersion, DefaultApplicationVersion))
		if err != nil {
			return InstallInput{}, err
		}
	}
	components, err := componentsForInput(input)
	if err != nil {
		return InstallInput{}, err
	}
	if slices.Contains(components, ComponentAPI) && !input.APIPortExplicit {
		input.APIPort, err = terminal.Prompt(ctx, "API port", defaultString(input.APIPort, "8080"))
		if err != nil {
			return InstallInput{}, err
		}
	}
	portPrompts := []struct {
		component Component
		explicit  bool
		label     string
		fallback  string
		value     *string
	}{
		{ComponentGateway, input.GatewayPortExplicit, "Gateway port", "80", &input.GatewayPort},
		{ComponentUserWeb, input.UserWebPortExplicit, "User web port", "5173", &input.UserWebPort},
		{ComponentAdminWeb, input.AdminWebPortExplicit, "Admin web port", "5174", &input.AdminWebPort},
		{ComponentDocsWeb, input.DocsWebPortExplicit, "Documentation web port", "5175", &input.DocsWebPort},
		{ComponentMonitoring, input.MonitoringPortExplicit, "Monitoring port", "9090", &input.MonitoringPort},
	}
	for _, prompt := range portPrompts {
		if !slices.Contains(components, prompt.component) || prompt.explicit {
			continue
		}
		*prompt.value, err = terminal.Prompt(ctx, prompt.label, defaultString(*prompt.value, prompt.fallback))
		if err != nil {
			return InstallInput{}, err
		}
	}
	if input.Mode == config.DeploymentModeDocker && !input.ImageTagExplicit {
		input.ImageTag, err = terminal.Prompt(ctx, "Docker image tag", defaultString(input.ImageTag, input.ApplicationVersion))
		if err != nil {
			return InstallInput{}, err
		}
	}
	if input.Mode == config.DeploymentModeNative && !input.ReleaseVersionExplicit {
		input.ReleaseVersion, err = terminal.Prompt(ctx, "Native release version", defaultString(input.ReleaseVersion, input.ApplicationVersion))
		if err != nil {
			return InstallInput{}, err
		}
	}
	input.Interactive = true
	return input, nil
}

func writeRunError(stderr io.Writer, err error) int {
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(stderr, "deployctl: interrupted")
		return 130
	}
	fmt.Fprintf(stderr, "deployctl: %v\n", err)
	return 1
}

type stdioTerminal struct {
	reader      *bufio.Reader
	writer      io.Writer
	interactive bool
}

func NewStdioTerminal(reader io.Reader, writer io.Writer) Terminal {
	interactive := true
	if file, ok := reader.(*os.File); ok {
		info, err := file.Stat()
		interactive = err == nil && info.Mode()&os.ModeCharDevice != 0
	}
	return &stdioTerminal{reader: bufio.NewReader(reader), writer: writer, interactive: interactive}
}

func (terminal *stdioTerminal) Interactive() bool { return terminal.interactive }

func (terminal *stdioTerminal) Prompt(ctx context.Context, label, fallback string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	fmt.Fprintf(terminal.writer, "%s [%s]: ", label, fallback)
	line, err := terminal.reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	value := strings.TrimSpace(line)
	if value == "" {
		value = fallback
	}
	return value, nil
}

func (terminal *stdioTerminal) Confirm(ctx context.Context, message string) (bool, error) {
	value, err := terminal.Prompt(ctx, message+" (y/N)", "n")
	if err != nil {
		return false, err
	}
	return strings.EqualFold(value, "y") || strings.EqualFold(value, "yes"), nil
}
