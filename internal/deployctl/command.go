package deployctl

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
)

const DefaultApplicationVersion = "dev"

type CommandKind string

const (
	CommandInstall            CommandKind = "install"
	CommandImportConfig       CommandKind = "import-config"
	CommandStatus             CommandKind = "status"
	CommandDoctor             CommandKind = "doctor"
	CommandRestart            CommandKind = "restart"
	CommandVersion            CommandKind = "version"
	CommandUpgrade            CommandKind = "upgrade"
	CommandUninstall          CommandKind = "uninstall"
	CommandSetupStatus        CommandKind = "setup status"
	CommandSetupTokenShow     CommandKind = "setup token show"
	CommandSetupTokenReset    CommandKind = "setup token reset"
	CommandClusterTokenCreate CommandKind = "cluster token create"
	CommandClusterJoin        CommandKind = "cluster join"
)

type ClusterTokenCreateOptions struct {
	Role config.DeploymentRole
	TTL  time.Duration
}

type UpgradeOptions struct {
	RuntimeDir         string
	ApplicationVersion string
	ImageRegistry      string
	ImageTag           string
	ReleaseVersion     string
	Migrate            bool
}

type VersionOptions struct {
	JSON bool
}

type UninstallOptions struct {
	RuntimeDir   string
	DeleteData   bool
	Confirmation string
}

type ClusterJoinOptions struct {
	Server             string
	Token              string
	RuntimeDir         string
	Mode               config.DeploymentMode
	ApplicationVersion string
	ImageRegistry      string
	ImageTag           string
	ReleaseVersion     string
	APIPort            string
	GatewayPort        string
	UserWebPort        string
	AdminWebPort       string
	DocsWebPort        string
}

func (options ClusterJoinOptions) String() string {
	return fmt.Sprintf("ClusterJoinOptions{Server:%q, Token:<redacted>}", options.Server)
}

func (options ClusterJoinOptions) GoString() string { return options.String() }

func (options ClusterJoinOptions) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Server string                `json:"server"`
		Token  string                `json:"token"`
		Mode   config.DeploymentMode `json:"mode"`
	}{Server: options.Server, Token: "REDACTED", Mode: options.Mode})
}

type Command struct {
	Kind               CommandKind
	RuntimeDir         string
	Yes                bool
	Install            *InstallInput
	ImportConfig       *ImportConfigOptions
	Version            *VersionOptions
	Upgrade            *UpgradeOptions
	Uninstall          *UninstallOptions
	ClusterTokenCreate *ClusterTokenCreateOptions
	ClusterJoin        *ClusterJoinOptions
}

func (command Command) String() string {
	switch command.Kind {
	case CommandClusterJoin:
		if command.ClusterJoin == nil {
			return "deployctl cluster join"
		}
		return fmt.Sprintf("deployctl cluster join --server %s --token <redacted>", command.ClusterJoin.Server)
	case CommandClusterTokenCreate:
		if command.ClusterTokenCreate == nil {
			return "deployctl cluster token create"
		}
		return fmt.Sprintf("deployctl cluster token create --role %s --ttl %s", command.ClusterTokenCreate.Role, command.ClusterTokenCreate.TTL)
	default:
		return "deployctl " + string(command.Kind)
	}
}

func (command Command) GoString() string { return command.String() }

func (command Command) MarshalJSON() ([]byte, error) {
	type safeCommand struct {
		Kind       CommandKind `json:"kind"`
		RuntimeDir string      `json:"runtime_dir,omitempty"`
		Yes        bool        `json:"yes,omitempty"`
		Server     string      `json:"server,omitempty"`
		Token      string      `json:"token,omitempty"`
	}
	safe := safeCommand{Kind: command.Kind, RuntimeDir: command.RuntimeDir, Yes: command.Yes}
	if command.ClusterJoin != nil {
		safe.Server = command.ClusterJoin.Server
		safe.Token = "REDACTED"
	}
	return json.Marshal(safe)
}

func ParseCommand(args []string) (Command, error) {
	if len(args) == 0 {
		return Command{}, fmt.Errorf("a deployctl command is required")
	}
	if err := rejectDuplicateFlags(args); err != nil {
		return Command{}, err
	}
	switch args[0] {
	case "install":
		return parseInstallCommand(args[1:])
	case "import-config":
		return parseImportConfigCommand(args[1:])
	case "status":
		return parseRuntimeCommand(CommandStatus, args[1:])
	case "doctor":
		return parseRuntimeCommand(CommandDoctor, args[1:])
	case "restart":
		return parseRuntimeCommand(CommandRestart, args[1:])
	case "version":
		return parseVersionCommand(args[1:])
	case "upgrade":
		return parseUpgradeCommand(args[1:])
	case "uninstall":
		return parseUninstallCommand(args[1:])
	case "setup":
		return parseSetupCommand(args[1:])
	case "cluster":
		return parseClusterCommand(args[1:])
	default:
		return Command{}, fmt.Errorf("unknown deployctl command %q", args[0])
	}
}

func parseVersionCommand(args []string) (Command, error) {
	set := newFlagSet("version")
	jsonOutput := set.Bool("json", false, "write machine-readable JSON")
	if err := set.Parse(args); err != nil {
		return Command{}, err
	}
	if set.NArg() != 0 {
		return Command{}, fmt.Errorf("version does not accept positional arguments")
	}
	return Command{Kind: CommandVersion, Version: &VersionOptions{JSON: *jsonOutput}}, nil
}

func parseImportConfigCommand(args []string) (Command, error) {
	set := newFlagSet("import-config")
	source := set.String("source", "", "legacy .env, .env.prod, or backend.env path")
	runtimeDir := set.String("runtime-dir", ".", "portable runtime directory")
	mode := set.String("mode", string(config.DeploymentModeDocker), "docker or native")
	profile := set.String("profile", string(config.DeploymentProfileCore), "full, core, or custom")
	topology := set.String("topology", string(config.DeploymentTopologySingle), "single or cluster")
	role := set.String("role", string(config.DeploymentRoleSingle), "single or control")
	components := set.String("components", "", "comma-separated components")
	storageDriver := set.String("storage-driver", "", "local or s3")
	publicAPIURL := set.String("public-api-url", "", "public API URL")
	applicationVersion := set.String("application-version", DefaultApplicationVersion, "application version")
	imageRegistry := set.String("image-registry", "", "Docker image registry")
	imageTag := set.String("image-tag", "", "Docker image tag")
	releaseVersion := set.String("release-version", "", "native release version")
	if err := set.Parse(args); err != nil {
		return Command{}, err
	}
	if set.NArg() != 0 {
		return Command{}, fmt.Errorf("import-config does not accept positional arguments")
	}
	if strings.TrimSpace(*source) == "" {
		return Command{}, fmt.Errorf("import-config requires --source")
	}
	options := &ImportConfigOptions{
		Source: *source, RuntimeDir: filepath.Clean(*runtimeDir), Mode: config.DeploymentMode(*mode),
		Profile: config.DeploymentProfile(*profile), Topology: config.DeploymentTopology(*topology), Role: config.DeploymentRole(*role),
		Components: parseComponents(*components), StorageDriver: *storageDriver, PublicAPIURL: *publicAPIURL,
		ApplicationVersion: *applicationVersion, ImageRegistry: *imageRegistry, ImageTag: defaultString(*imageTag, *applicationVersion),
		ReleaseVersion: defaultString(*releaseVersion, *applicationVersion),
	}
	return Command{Kind: CommandImportConfig, RuntimeDir: *runtimeDir, ImportConfig: options}, nil
}

func parseUpgradeCommand(args []string) (Command, error) {
	set := newFlagSet("upgrade")
	runtimeDir := set.String("runtime-dir", ".", "portable runtime directory")
	applicationVersion := set.String("application-version", "", "target application version")
	imageRegistry := set.String("image-registry", "", "target Docker image registry")
	imageTag := set.String("image-tag", "", "target Docker image tag")
	releaseVersion := set.String("release-version", "", "target native release version")
	migrate := set.Bool("migrate", true, "run the control-node database migration")
	if err := set.Parse(args); err != nil {
		return Command{}, err
	}
	if set.NArg() != 0 {
		return Command{}, fmt.Errorf("upgrade does not accept positional arguments")
	}
	options := &UpgradeOptions{
		RuntimeDir: filepath.Clean(*runtimeDir), ApplicationVersion: *applicationVersion,
		ImageRegistry: *imageRegistry, ImageTag: *imageTag, ReleaseVersion: *releaseVersion, Migrate: *migrate,
	}
	return Command{Kind: CommandUpgrade, RuntimeDir: *runtimeDir, Upgrade: options}, nil
}

func parseUninstallCommand(args []string) (Command, error) {
	set := newFlagSet("uninstall")
	runtimeDir := set.String("runtime-dir", ".", "portable runtime directory")
	deleteData := set.Bool("delete-data", false, "permanently delete persistent data and configuration")
	confirmation := set.String("confirm", "", "exact destructive confirmation phrase")
	yes := set.Bool("yes", false, "non-interactive confirmation for the non-destructive stop")
	if err := set.Parse(args); err != nil {
		return Command{}, err
	}
	if set.NArg() != 0 {
		return Command{}, fmt.Errorf("uninstall does not accept positional arguments")
	}
	if *deleteData && strings.TrimSpace(*confirmation) == "" {
		return Command{}, fmt.Errorf("destructive uninstall requires --confirm with the installation-specific phrase")
	}
	if !*deleteData && *confirmation != "" {
		return Command{}, fmt.Errorf("--confirm is accepted only with --delete-data")
	}
	if *deleteData && *yes {
		return Command{}, fmt.Errorf("--yes cannot authorize persistent data deletion; use the exact --confirm phrase")
	}
	options := &UninstallOptions{RuntimeDir: filepath.Clean(*runtimeDir), DeleteData: *deleteData, Confirmation: *confirmation}
	return Command{Kind: CommandUninstall, RuntimeDir: *runtimeDir, Yes: *yes, Uninstall: options}, nil
}

func parseInstallCommand(args []string) (Command, error) {
	set := newFlagSet("install")
	mode := set.String("mode", "", "docker or native")
	profile := set.String("profile", "", "full, core, or custom")
	topology := set.String("topology", "", "single or cluster")
	role := set.String("role", "", "single or control")
	components := set.String("components", "", "comma-separated components")
	runtimeDir := set.String("runtime-dir", ".", "portable runtime directory")
	storageDriver := set.String("storage-driver", "", "local or s3")
	publicAPIURL := set.String("public-api-url", "", "public API URL")
	applicationVersion := set.String("application-version", DefaultApplicationVersion, "application version")
	imageRegistry := set.String("image-registry", "", "Docker image registry")
	imageTag := set.String("image-tag", "", "Docker image tag")
	releaseVersion := set.String("release-version", "", "native release version")
	apiPort := set.String("api-port", "", "API port")
	gatewayPort := set.String("gateway-port", "", "Gateway port")
	userWebPort := set.String("user-web-port", "", "user web port")
	adminWebPort := set.String("admin-web-port", "", "admin web port")
	docsWebPort := set.String("docs-web-port", "", "documentation web port")
	monitoringPort := set.String("monitoring-port", "", "monitoring port")
	externalGateway := set.Bool("external-gateway", false, "confirm external web hosting/proxy")
	migrate := set.Bool("migrate", false, "request a control migration")
	yes := set.Bool("yes", false, "non-interactive confirmation")
	if err := set.Parse(args); err != nil {
		return Command{}, err
	}
	if set.NArg() != 0 {
		return Command{}, fmt.Errorf("install does not accept positional arguments")
	}
	if *yes && (*mode == "" || *profile == "" || *topology == "") {
		return Command{}, fmt.Errorf("non-interactive install requires --mode, --profile, and --topology")
	}
	resolvedRole := config.DeploymentRole(*role)
	if resolvedRole == "" {
		if config.DeploymentTopology(*topology) == config.DeploymentTopologyCluster {
			resolvedRole = config.DeploymentRoleControl
		} else if *topology != "" {
			resolvedRole = config.DeploymentRoleSingle
		}
	}
	if resolvedRole == config.DeploymentRoleAPI || resolvedRole == config.DeploymentRoleWorker || resolvedRole == config.DeploymentRoleWeb {
		return Command{}, fmt.Errorf("joined roles must use deployctl cluster join")
	}
	input := &InstallInput{
		Interactive: !*yes, Mode: config.DeploymentMode(*mode), Profile: config.DeploymentProfile(*profile),
		Topology: config.DeploymentTopology(*topology), Role: resolvedRole, Components: parseComponents(*components),
		RuntimeDir: *runtimeDir, StorageDriver: *storageDriver, PublicAPIURL: *publicAPIURL,
		ExternalGatewayConfirmed: *externalGateway, MigrationRequested: *migrate,
		ApplicationVersion: *applicationVersion, ImageRegistry: *imageRegistry, ImageTag: *imageTag, ReleaseVersion: *releaseVersion,
		APIPort: *apiPort, GatewayPort: *gatewayPort, UserWebPort: *userWebPort, AdminWebPort: *adminWebPort, DocsWebPort: *docsWebPort, MonitoringPort: *monitoringPort,
		RuntimeDirExplicit: flagWasProvided(args, "runtime-dir"), ApplicationVersionExplicit: flagWasProvided(args, "application-version"),
		ImageTagExplicit: flagWasProvided(args, "image-tag"), ReleaseVersionExplicit: flagWasProvided(args, "release-version"),
		APIPortExplicit: flagWasProvided(args, "api-port"), GatewayPortExplicit: flagWasProvided(args, "gateway-port"),
		UserWebPortExplicit: flagWasProvided(args, "user-web-port"), AdminWebPortExplicit: flagWasProvided(args, "admin-web-port"),
		DocsWebPortExplicit:    flagWasProvided(args, "docs-web-port"),
		MonitoringPortExplicit: flagWasProvided(args, "monitoring-port"),
	}
	return Command{Kind: CommandInstall, RuntimeDir: *runtimeDir, Yes: *yes, Install: input}, nil
}

func parseSetupCommand(args []string) (Command, error) {
	if len(args) == 0 {
		return Command{}, fmt.Errorf("setup subcommand is required")
	}
	if args[0] == "status" {
		return parseRuntimeCommand(CommandSetupStatus, args[1:])
	}
	if len(args) >= 2 && args[0] == "token" {
		switch args[1] {
		case "show":
			return parseRuntimeCommand(CommandSetupTokenShow, args[2:])
		case "reset":
			return parseRuntimeCommand(CommandSetupTokenReset, args[2:])
		}
	}
	return Command{}, fmt.Errorf("unknown setup subcommand")
}

func parseClusterCommand(args []string) (Command, error) {
	if len(args) == 0 {
		return Command{}, fmt.Errorf("cluster subcommand is required")
	}
	if len(args) >= 2 && args[0] == "token" && args[1] == "create" {
		set := newFlagSet("cluster token create")
		role := set.String("role", "", "api, worker, or web")
		ttl := set.Duration("ttl", 10*time.Minute, "token lifetime")
		runtimeDir := set.String("runtime-dir", ".", "portable runtime directory")
		if err := set.Parse(args[2:]); err != nil || set.NArg() != 0 {
			if err != nil {
				return Command{}, err
			}
			return Command{}, fmt.Errorf("cluster token create does not accept positional arguments")
		}
		parsedRole := config.DeploymentRole(*role)
		if parsedRole != config.DeploymentRoleAPI && parsedRole != config.DeploymentRoleWorker && parsedRole != config.DeploymentRoleWeb {
			return Command{}, fmt.Errorf("cluster token role must be api, worker, or web")
		}
		if *ttl <= 0 || *ttl > 24*time.Hour {
			return Command{}, fmt.Errorf("cluster token TTL must be between zero and 24h")
		}
		return Command{Kind: CommandClusterTokenCreate, RuntimeDir: *runtimeDir, ClusterTokenCreate: &ClusterTokenCreateOptions{Role: parsedRole, TTL: *ttl}}, nil
	}
	if args[0] == "join" {
		set := newFlagSet("cluster join")
		server := set.String("server", "", "control API URL")
		token := set.String("token", "", "single-use join token")
		runtimeDir := set.String("runtime-dir", ".", "portable runtime directory")
		mode := set.String("mode", "docker", "docker or native")
		applicationVersion := set.String("application-version", DefaultApplicationVersion, "application version")
		imageRegistry := set.String("image-registry", "", "Docker image registry")
		imageTag := set.String("image-tag", "", "Docker image tag")
		releaseVersion := set.String("release-version", "", "native release version")
		apiPort := set.String("api-port", "8080", "API port")
		gatewayPort := set.String("gateway-port", "80", "Gateway port")
		userWebPort := set.String("user-web-port", "5173", "user web port")
		adminWebPort := set.String("admin-web-port", "5174", "admin web port")
		docsWebPort := set.String("docs-web-port", "5175", "documentation web port")
		if err := set.Parse(args[1:]); err != nil || set.NArg() != 0 {
			if err != nil {
				return Command{}, err
			}
			return Command{}, fmt.Errorf("cluster join does not accept positional arguments")
		}
		if err := validateServerURL(*server); err != nil {
			return Command{}, err
		}
		if strings.TrimSpace(*token) == "" {
			return Command{}, fmt.Errorf("cluster join requires --token")
		}
		if config.DeploymentMode(*mode) != config.DeploymentModeDocker && config.DeploymentMode(*mode) != config.DeploymentModeNative {
			return Command{}, fmt.Errorf("cluster join mode must be docker or native")
		}
		options := &ClusterJoinOptions{
			Server: strings.TrimRight(*server, "/"), Token: *token, RuntimeDir: filepath.Clean(*runtimeDir), Mode: config.DeploymentMode(*mode),
			ApplicationVersion: *applicationVersion, ImageRegistry: *imageRegistry, ImageTag: defaultString(*imageTag, *applicationVersion),
			ReleaseVersion: defaultString(*releaseVersion, *applicationVersion), APIPort: *apiPort,
			GatewayPort: *gatewayPort, UserWebPort: *userWebPort, AdminWebPort: *adminWebPort, DocsWebPort: *docsWebPort,
		}
		return Command{Kind: CommandClusterJoin, RuntimeDir: *runtimeDir, ClusterJoin: options}, nil
	}
	return Command{}, fmt.Errorf("unknown cluster subcommand")
}

func parseRuntimeCommand(kind CommandKind, args []string) (Command, error) {
	set := newFlagSet(string(kind))
	runtimeDir := set.String("runtime-dir", ".", "portable runtime directory")
	yes := set.Bool("yes", false, "non-interactive confirmation")
	if err := set.Parse(args); err != nil {
		return Command{}, err
	}
	if set.NArg() != 0 {
		return Command{}, fmt.Errorf("%s does not accept positional arguments", kind)
	}
	return Command{Kind: kind, RuntimeDir: *runtimeDir, Yes: *yes}, nil
}

func newFlagSet(name string) *flag.FlagSet {
	set := flag.NewFlagSet(name, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	return set
}

func parseComponents(value string) []Component {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	components := make([]Component, 0, len(parts))
	for _, part := range parts {
		components = append(components, Component(strings.TrimSpace(part)))
	}
	return components
}

func rejectDuplicateFlags(args []string) error {
	seen := make(map[string]struct{})
	for _, argument := range args {
		if !strings.HasPrefix(argument, "--") || argument == "--" {
			continue
		}
		name := strings.TrimPrefix(strings.SplitN(argument, "=", 2)[0], "--")
		if _, exists := seen[name]; exists {
			return fmt.Errorf("flag --%s may be provided only once", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func flagWasProvided(args []string, name string) bool {
	flagName := "--" + name
	for _, argument := range args {
		if argument == flagName || strings.HasPrefix(argument, flagName+"=") {
			return true
		}
	}
	return false
}

func validateServerURL(value string) error {
	return validateHTTPBaseURL(value, "cluster server")
}
