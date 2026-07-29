package deployctl

import (
	"fmt"
	"strings"
)

type CommandCatalogEntry struct {
	Path    string
	Kind    CommandKind
	Group   string
	Summary string
	Usage   string
}

var commandCatalog = [...]CommandCatalogEntry{
	{Path: "install", Kind: CommandInstall, Group: "Install and deployment", Summary: "Install a new deployment", Usage: "install [options]"},
	{Path: "import-config", Kind: CommandImportConfig, Group: "Upgrade and configuration migration", Summary: "Import a legacy runtime configuration", Usage: "import-config --source <path> [options]"},
	{Path: "status", Kind: CommandStatus, Group: "Runtime operations", Summary: "Show deployment status", Usage: "status [--runtime-dir <dir>]"},
	{Path: "doctor", Kind: CommandDoctor, Group: "Runtime operations", Summary: "Diagnose deployment health", Usage: "doctor [--runtime-dir <dir>]"},
	{Path: "restart", Kind: CommandRestart, Group: "Runtime operations", Summary: "Restart deployment services", Usage: "restart [--runtime-dir <dir>]"},
	{Path: "version", Kind: CommandVersion, Group: "Deployctl tool", Summary: "Show deployctl build information", Usage: "version [--json]"},
	{Path: "self-update", Kind: CommandSelfUpdate, Group: "Deployctl tool", Summary: "Update the deployctl executable", Usage: "self-update [--version <version>] [--yes]"},
	{Path: "upgrade", Kind: CommandUpgrade, Group: "Upgrade and configuration migration", Summary: "Upgrade the deployed application", Usage: "upgrade [options]"},
	{Path: "uninstall", Kind: CommandUninstall, Group: "Runtime operations", Summary: "Stop services or remove a deployment", Usage: "uninstall [options]"},
	{Path: "setup status", Kind: CommandSetupStatus, Group: "Setup initialization", Summary: "Show Setup initialization status", Usage: "setup status [--runtime-dir <dir>]"},
	{Path: "setup token show", Kind: CommandSetupTokenShow, Group: "Setup initialization", Summary: "Show the current Setup token", Usage: "setup token show [--runtime-dir <dir>]"},
	{Path: "setup token reset", Kind: CommandSetupTokenReset, Group: "Setup initialization", Summary: "Reset the Setup token", Usage: "setup token reset [--runtime-dir <dir>]"},
	{Path: "cluster token create", Kind: CommandClusterTokenCreate, Group: "Cluster management", Summary: "Create a single-use cluster join token", Usage: "cluster token create --role <api|worker|web> [options]"},
	{Path: "cluster join", Kind: CommandClusterJoin, Group: "Cluster management", Summary: "Join a node to a cluster", Usage: "cluster join --server <url> --token <token> [options]"},
}

func CommandCatalog() []CommandCatalogEntry {
	entries := make([]CommandCatalogEntry, len(commandCatalog))
	copy(entries, commandCatalog[:])
	return entries
}

func HelpText() string {
	var text strings.Builder
	text.WriteString("Usage:\n")
	text.WriteString("  deployctl <command> [options]\n")
	text.WriteString("  deployctl -h | --help\n\n")
	text.WriteString("Commands:\n")

	entries := CommandCatalog()
	groups := make([]string, 0, len(entries))
	for _, entry := range entries {
		found := false
		for _, group := range groups {
			if group == entry.Group {
				found = true
				break
			}
		}
		if !found {
			groups = append(groups, entry.Group)
		}
	}
	for _, group := range groups {
		fmt.Fprintf(&text, "\n%s:\n", group)
		for _, entry := range entries {
			if entry.Group == group {
				fmt.Fprintf(&text, "  %-68s %s\n", "deployctl "+entry.Usage, entry.Summary)
			}
		}
	}

	return text.String()
}
