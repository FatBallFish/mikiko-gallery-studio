package mgsctl

import (
	"reflect"
	"strings"
	"testing"
)

func TestCommandCatalogCoversApprovedParserTree(t *testing.T) {
	want := []CommandCatalogEntry{
		{Path: "install", Kind: CommandInstall, Group: "Install and deployment", Summary: "Install a new deployment", Usage: "install [options]"},
		{Path: "import-config", Kind: CommandImportConfig, Group: "Upgrade and configuration migration", Summary: "Import a legacy runtime configuration", Usage: "import-config --source <path> [options]"},
		{Path: "status", Kind: CommandStatus, Group: "Runtime operations", Summary: "Show deployment status", Usage: "status [--runtime-dir <dir>]"},
		{Path: "doctor", Kind: CommandDoctor, Group: "Runtime operations", Summary: "Diagnose deployment health", Usage: "doctor [--runtime-dir <dir>]"},
		{Path: "restart", Kind: CommandRestart, Group: "Runtime operations", Summary: "Restart deployment services", Usage: "restart [--runtime-dir <dir>]"},
		{Path: "version", Kind: CommandVersion, Group: "MGSCTL tool", Summary: "Show mgsctl build information", Usage: "version [--json]"},
		{Path: "self-update", Kind: CommandSelfUpdate, Group: "MGSCTL tool", Summary: "Update the mgsctl executable", Usage: "self-update [--version <version>] [--yes]"},
		{Path: "upgrade", Kind: CommandUpgrade, Group: "Upgrade and configuration migration", Summary: "Upgrade the deployed application", Usage: "upgrade [options]"},
		{Path: "uninstall", Kind: CommandUninstall, Group: "Runtime operations", Summary: "Stop services or remove a deployment", Usage: "uninstall [options]"},
		{Path: "setup status", Kind: CommandSetupStatus, Group: "Setup initialization", Summary: "Show Setup initialization status", Usage: "setup status [--runtime-dir <dir>]"},
		{Path: "setup token show", Kind: CommandSetupTokenShow, Group: "Setup initialization", Summary: "Show the current Setup token", Usage: "setup token show [--runtime-dir <dir>]"},
		{Path: "setup token reset", Kind: CommandSetupTokenReset, Group: "Setup initialization", Summary: "Reset the Setup token", Usage: "setup token reset [--runtime-dir <dir>]"},
		{Path: "cluster token create", Kind: CommandClusterTokenCreate, Group: "Cluster management", Summary: "Create a single-use cluster join token", Usage: "cluster token create --role <api|worker|web> [options]"},
		{Path: "cluster join", Kind: CommandClusterJoin, Group: "Cluster management", Summary: "Join a node to a cluster", Usage: "cluster join --server <url> --token <token> [options]"},
	}

	gotCatalog := CommandCatalog()
	if !reflect.DeepEqual(gotCatalog, want) {
		t.Fatalf("CommandCatalog() = %#v, want %#v", gotCatalog, want)
	}

	help := HelpText()
	if !strings.HasPrefix(help, "Usage:\n") {
		t.Fatalf("HelpText() does not start with usage: %q", help)
	}
	for _, entry := range want {
		for _, expected := range []string{entry.Group + ":", "mgsctl " + entry.Usage, entry.Summary} {
			if !strings.Contains(help, expected) {
				t.Errorf("HelpText() does not contain %q for %q: %q", expected, entry.Path, help)
			}
		}
	}
}

func TestCommandCatalogReturnsIndependentSnapshots(t *testing.T) {
	first := CommandCatalog()
	first[0].Path = "changed"
	if second := CommandCatalog(); second[0].Path == "changed" {
		t.Fatal("CommandCatalog() exposed mutable shared catalog storage")
	}
}
