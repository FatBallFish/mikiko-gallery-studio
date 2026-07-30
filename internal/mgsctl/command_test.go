package mgsctl

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
)

func TestParseCommandSupportsTheApprovedCommandTree(t *testing.T) {
	tests := []struct {
		args []string
		kind CommandKind
	}{
		{args: []string{"install"}, kind: CommandInstall},
		{args: []string{"import-config", "--source", ".env"}, kind: CommandImportConfig},
		{args: []string{"status"}, kind: CommandStatus},
		{args: []string{"doctor"}, kind: CommandDoctor},
		{args: []string{"restart"}, kind: CommandRestart},
		{args: []string{"version"}, kind: CommandVersion},
		{args: []string{"version", "--json"}, kind: CommandVersion},
		{args: []string{"self-update", "--version", "v1.2.3", "--yes"}, kind: CommandSelfUpdate},
		{args: []string{"upgrade"}, kind: CommandUpgrade},
		{args: []string{"uninstall"}, kind: CommandUninstall},
		{args: []string{"setup", "status"}, kind: CommandSetupStatus},
		{args: []string{"setup", "token", "show"}, kind: CommandSetupTokenShow},
		{args: []string{"setup", "token", "reset"}, kind: CommandSetupTokenReset},
		{args: []string{"cluster", "token", "create", "--role", "worker", "--ttl", "10m"}, kind: CommandClusterTokenCreate},
		{args: []string{"cluster", "join", "--server", "http://10.0.0.10:8080", "--token", "join-secret"}, kind: CommandClusterJoin},
	}
	for _, testCase := range tests {
		t.Run(strings.Join(testCase.args, "_"), func(t *testing.T) {
			command, err := ParseCommand(testCase.args)
			if err != nil || command.Kind != testCase.kind {
				t.Fatalf("ParseCommand(%v) = %#v, %v; want kind %q", testCase.args, command, err, testCase.kind)
			}
		})
	}
}

func TestParseSelfUpdateCommandKeepsToolAndApplicationUpgradeSeparate(t *testing.T) {
	command, err := ParseCommand([]string{
		"self-update", "--version", "v1.2.3", "--release-base-url", "https://downloads.example.test/releases",
		"--download-url", "https://cdn.example.test/mgsctl", "--sha256", strings.Repeat("a", 64), "--yes",
	})
	if err != nil {
		t.Fatal(err)
	}
	if command.Kind != CommandSelfUpdate || command.SelfUpdate == nil || command.SelfUpdate.Version != "v1.2.3" ||
		command.SelfUpdate.ReleaseBaseURL != "https://downloads.example.test/releases" || command.SelfUpdate.DownloadURL != "https://cdn.example.test/mgsctl" ||
		command.SelfUpdate.ExpectedSHA256 != strings.Repeat("a", 64) || !command.SelfUpdate.Yes {
		t.Fatalf("self-update command = %#v", command)
	}

	defaultCommand, err := ParseCommand([]string{"self-update"})
	if err != nil || defaultCommand.SelfUpdate.Version != "latest" || defaultCommand.SelfUpdate.ReleaseBaseURL != DefaultMGSCTLReleaseBaseURL {
		t.Fatalf("default self-update = %#v, %v", defaultCommand, err)
	}

	application, err := ParseCommand([]string{"upgrade", "--image-tag", "v2.0.0"})
	if err != nil || application.Kind != CommandUpgrade || application.Upgrade == nil || application.Upgrade.ImageTag != "v2.0.0" || application.Upgrade.ApplicationVersion != "" || application.SelfUpdate != nil {
		t.Fatalf("application upgrade changed meaning: %#v, %v", application, err)
	}

	for _, args := range [][]string{
		{"self-update", "unexpected"},
		{"self-update", "--version", "v1", "--version", "v2"},
		{"self-update", "--release-base-url", "ftp://downloads.example.test"},
		{"self-update", "--download-url", "https://user:secret@downloads.example.test/mgsctl"},
		{"self-update", "--sha256", "not-a-digest"},
	} {
		if _, err := ParseCommand(args); err == nil {
			t.Errorf("ParseCommand(%v) unexpectedly succeeded", args)
		}
	}
}

func TestParseVersionCommandAcceptsOnlyTheJSONFlag(t *testing.T) {
	text, err := ParseCommand([]string{"version"})
	if err != nil || text.Version == nil || text.Version.JSON {
		t.Fatalf("ParseCommand(version) = %#v, %v", text, err)
	}

	jsonCommand, err := ParseCommand([]string{"version", "--json"})
	if err != nil || jsonCommand.Version == nil || !jsonCommand.Version.JSON {
		t.Fatalf("ParseCommand(version --json) = %#v, %v", jsonCommand, err)
	}

	for _, args := range [][]string{
		{"version", "unexpected"},
		{"version", "--json", "--json"},
	} {
		if _, err := ParseCommand(args); err == nil {
			t.Errorf("ParseCommand(%v) unexpectedly succeeded", args)
		}
	}
}

func TestParseCommandCapturesOperationalOptionsAndRequiresExactDestructiveIntent(t *testing.T) {
	importCommand, err := ParseCommand([]string{
		"import-config", "--source", ".env.prod", "--runtime-dir", "runtime",
		"--mode", "docker", "--profile", "core", "--topology", "single", "--role", "single",
		"--application-version", "v2.1.0",
	})
	if err != nil {
		t.Fatalf("ParseCommand(import-config): %v", err)
	}
	if importCommand.ImportConfig == nil || importCommand.ImportConfig.Source != ".env.prod" || importCommand.RuntimeDir != "runtime" {
		t.Fatalf("import command parsed incorrectly: %#v", importCommand)
	}

	upgrade, err := ParseCommand([]string{
		"upgrade", "--runtime-dir", "runtime",
		"--image-tag", "sha-123", "--release-version", "v2.2.0", "--migrate=false",
	})
	if err != nil {
		t.Fatalf("ParseCommand(upgrade): %v", err)
	}
	if upgrade.Upgrade == nil || upgrade.Upgrade.ApplicationVersion != "" || upgrade.Upgrade.ImageTag != "sha-123" || upgrade.Upgrade.ReleaseVersion != "v2.2.0" || upgrade.Upgrade.Migrate {
		t.Fatalf("upgrade command parsed incorrectly: %#v", upgrade)
	}

	const installationID = "019d0000-0000-7000-8000-000000000123"
	phrase := DestructiveUninstallConfirmation(installationID)
	destructive, err := ParseCommand([]string{
		"uninstall", "--runtime-dir", "runtime", "--delete-data", "--confirm", phrase,
	})
	if err != nil {
		t.Fatalf("ParseCommand(destructive uninstall): %v", err)
	}
	if destructive.Uninstall == nil || !destructive.Uninstall.DeleteData || destructive.Uninstall.Confirmation != phrase {
		t.Fatalf("destructive uninstall parsed incorrectly: %#v", destructive)
	}

	for _, args := range [][]string{
		{"import-config"},
		{"import-config", "--source", ".env", "--source", ".env.prod"},
		{"upgrade", "--image-tag", "one", "--image-tag", "two"},
		{"uninstall", "--delete-data"},
		{"uninstall", "--confirm", phrase},
		{"uninstall", "--delete-data", "--yes"},
	} {
		if _, err := ParseCommand(args); err == nil {
			t.Errorf("ParseCommand(%v) unexpectedly succeeded", args)
		}
	}
}

func TestParseCommandSeparatesInteractiveAndNonInteractiveInstall(t *testing.T) {
	interactive, err := ParseCommand([]string{"install"})
	if err != nil || !interactive.Install.Interactive {
		t.Fatalf("interactive install = %#v, %v", interactive, err)
	}
	if interactive.Install.Mode != config.DeploymentModeDocker || interactive.Install.Profile != config.DeploymentProfileFull || interactive.Install.Topology != config.DeploymentTopologySingle || interactive.Install.Role != config.DeploymentRoleSingle || interactive.Install.ImageTag != "latest" || interactive.Install.ApplicationVersion != "" {
		t.Fatalf("interactive install defaults = %#v", interactive.Install)
	}

	automated, err := ParseCommand([]string{"install", "--runtime-dir", ".", "--overwrite", "--yes"})
	if err != nil {
		t.Fatalf("ParseCommand(non-interactive install): %v", err)
	}
	if automated.Install.Interactive || !automated.Install.OverwriteExisting || automated.Install.Mode != config.DeploymentModeDocker || automated.Install.Profile != config.DeploymentProfileFull || automated.Install.Role != config.DeploymentRoleSingle || automated.Install.ApplicationVersion != "" || automated.Install.ImageTag != "latest" || automated.RuntimeDir != "." {
		t.Fatalf("non-interactive install parsed incorrectly: %#v", automated)
	}

	withPorts, err := ParseCommand([]string{
		"install", "--mode", "docker", "--profile", "core", "--topology", "single", "--yes",
		"--api-port", "18080", "--gateway-port", "18000", "--user-web-port", "15173", "--admin-web-port", "15174", "--docs-web-port", "15175", "--monitoring-port", "19090",
	})
	if err != nil {
		t.Fatal(err)
	}
	if withPorts.Install.APIPort != "18080" || withPorts.Install.GatewayPort != "18000" || withPorts.Install.DocsWebPort != "15175" || withPorts.Install.MonitoringPort != "19090" {
		t.Fatalf("install ports parsed incorrectly: %#v", withPorts.Install)
	}

	if _, err := ParseCommand([]string{"install", "--application-version", "v1.2.3"}); err == nil {
		t.Fatal("install accepted an operator-supplied application version")
	}
	if _, err := ParseCommand([]string{"upgrade", "--application-version", "v1.2.3"}); err == nil {
		t.Fatal("upgrade accepted an operator-supplied application version")
	}
}

func TestParseCommandPreservesWhetherRuntimeDirectoryWasExplicit(t *testing.T) {
	implicit, err := ParseCommand([]string{"upgrade"})
	if err != nil {
		t.Fatal(err)
	}
	explicit, err := ParseCommand([]string{"upgrade", "--runtime-dir", "."})
	if err != nil {
		t.Fatal(err)
	}
	if implicit.RuntimeDirExplicit || !explicit.RuntimeDirExplicit {
		t.Fatalf("runtime explicit flags implicit=%t explicit=%t", implicit.RuntimeDirExplicit, explicit.RuntimeDirExplicit)
	}
}

func TestParseCommandValidatesNestedArgumentsAndRedactsTokens(t *testing.T) {
	command, err := ParseCommand([]string{"cluster", "join", "--server", "https://api.example.test", "--token", "super-secret-token"})
	if err != nil {
		t.Fatalf("ParseCommand(cluster join): %v", err)
	}
	if strings.Contains(command.String(), "super-secret-token") || !strings.Contains(command.String(), "<redacted>") {
		t.Fatalf("command string leaked token: %s", command.String())
	}
	serialized, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	for _, rendered := range []string{fmt.Sprintf("%#v", command), string(serialized)} {
		if strings.Contains(rendered, "super-secret-token") {
			t.Fatalf("command diagnostic representation leaked token: %s", rendered)
		}
	}

	token, err := ParseCommand([]string{"cluster", "token", "create", "--role", "api", "--ttl", "15m"})
	if err != nil || token.ClusterTokenCreate.Role != config.DeploymentRoleAPI || token.ClusterTokenCreate.TTL != 15*time.Minute {
		t.Fatalf("cluster token command = %#v, %v", token, err)
	}

	for _, args := range [][]string{
		{}, {"unknown"}, {"setup"}, {"setup", "token"}, {"cluster", "join", "--server", "ftp://invalid", "--token", "x"},
		{"cluster", "join", "--server", "https://safe.example\\@evil.example", "--token", "x"},
		{"cluster", "join", "--server", "http://api.example.test:99999", "--token", "x"},
		{"cluster", "join", "--server", "http://api.example.test/?", "--token", "x"},
		{"cluster", "join", "--server", "https://api.example.test", "--token", "first", "--token", "second"},
		{"install", "--mode", "docker", "--mode", "native"},
		{"cluster", "token", "create", "--role", "single"}, {"status", "unexpected"},
	} {
		if _, err := ParseCommand(args); err == nil {
			t.Errorf("ParseCommand(%v) unexpectedly succeeded", args)
		}
	}
}

func TestInstallResultAndJoinOptionsRedactSecretsFromDiagnostics(t *testing.T) {
	const secret = "do-not-render-this-token"
	values := []any{
		InstallResult{RuntimeEnvPath: "config/runtime.env", SetupToken: secret},
		ClusterJoinOptions{Server: "https://api.example.test", Token: secret},
	}
	for _, value := range values {
		serialized, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, rendered := range []string{fmt.Sprintf("%v", value), fmt.Sprintf("%#v", value), string(serialized)} {
			if strings.Contains(rendered, secret) {
				t.Fatalf("diagnostic representation leaked token: %s", rendered)
			}
		}
	}
}
