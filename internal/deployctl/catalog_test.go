package deployctl

import (
	"reflect"
	"strings"
	"testing"
)

func TestCommandCatalogCoversApprovedParserTree(t *testing.T) {
	want := []CommandCatalogEntry{
		{Path: "install", Kind: CommandInstall},
		{Path: "import-config", Kind: CommandImportConfig},
		{Path: "status", Kind: CommandStatus},
		{Path: "doctor", Kind: CommandDoctor},
		{Path: "restart", Kind: CommandRestart},
		{Path: "version", Kind: CommandVersion},
		{Path: "self-update", Kind: CommandSelfUpdate},
		{Path: "upgrade", Kind: CommandUpgrade},
		{Path: "uninstall", Kind: CommandUninstall},
		{Path: "setup status", Kind: CommandSetupStatus},
		{Path: "setup token show", Kind: CommandSetupTokenShow},
		{Path: "setup token reset", Kind: CommandSetupTokenReset},
		{Path: "cluster token create", Kind: CommandClusterTokenCreate},
		{Path: "cluster join", Kind: CommandClusterJoin},
	}

	gotCatalog := CommandCatalog()
	got := make([]CommandCatalogEntry, len(gotCatalog))
	for index, entry := range gotCatalog {
		got[index] = CommandCatalogEntry{Path: entry.Path, Kind: entry.Kind}
		if entry.Group == "" || entry.Summary == "" || entry.Usage == "" {
			t.Errorf("catalog entry %q has incomplete presentation metadata: %#v", entry.Path, entry)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CommandCatalog() paths = %#v, want %#v", got, want)
	}

	help := HelpText()
	if !strings.HasPrefix(help, "Usage:\n") {
		t.Fatalf("HelpText() does not start with usage: %q", help)
	}
	for _, entry := range gotCatalog {
		if !strings.Contains(help, "deployctl "+entry.Usage) {
			t.Errorf("HelpText() does not contain usage for %q: %q", entry.Path, help)
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
