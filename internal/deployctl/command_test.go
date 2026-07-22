package deployctl

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
		{args: []string{"status"}, kind: CommandStatus},
		{args: []string{"doctor"}, kind: CommandDoctor},
		{args: []string{"restart"}, kind: CommandRestart},
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

func TestParseCommandSeparatesInteractiveAndNonInteractiveInstall(t *testing.T) {
	interactive, err := ParseCommand([]string{"install"})
	if err != nil || !interactive.Install.Interactive {
		t.Fatalf("interactive install = %#v, %v", interactive, err)
	}

	automated, err := ParseCommand([]string{
		"install", "--mode", "docker", "--profile", "full", "--topology", "single",
		"--runtime-dir", ".", "--yes",
	})
	if err != nil {
		t.Fatalf("ParseCommand(non-interactive install): %v", err)
	}
	if automated.Install.Interactive || automated.Install.Mode != config.DeploymentModeDocker || automated.Install.Profile != config.DeploymentProfileFull || automated.Install.Role != config.DeploymentRoleSingle || automated.Install.ApplicationVersion == "" || automated.RuntimeDir != "." {
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

	if _, err := ParseCommand([]string{"install", "--yes"}); err == nil {
		t.Fatal("non-interactive install accepted missing deployment choices")
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
