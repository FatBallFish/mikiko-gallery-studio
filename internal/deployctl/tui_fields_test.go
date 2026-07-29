package deployctl

import (
	"strings"
	"testing"
)

func TestTUICommandBuildersRoundTripThroughParser(t *testing.T) {
	for _, entry := range CommandCatalog() {
		t.Run(entry.Path, func(t *testing.T) {
			form := NewTUICommandForm(entry)
			args, err := form.Arguments()
			if err != nil {
				t.Fatal(err)
			}
			command, err := ParseCommand(args)
			if err != nil {
				t.Fatalf("ParseCommand(%q): %v", args, err)
			}
			if command.Kind != entry.Kind {
				t.Fatalf("kind=%q want %q", command.Kind, entry.Kind)
			}
		})
	}
}

func TestTUISensitiveFieldsAreMaskedAndReviewIsRedacted(t *testing.T) {
	entry := catalogEntryForTest(t, "cluster join")
	form := NewTUICommandForm(entry)
	if err := form.SetValue("token", "pgjoin.secret-value"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(form.View(), "secret-value") || !strings.Contains(form.View(), "••••") {
		t.Fatalf("field view leaked or did not mask token: %q", form.View())
	}
	if review := form.SafeCommand(); strings.Contains(review, "secret-value") || !strings.Contains(review, "<redacted>") {
		t.Fatalf("review leaked or did not redact token: %q", review)
	}
}

func TestTUIMultiSelectUsesSpaceAndProducesComponents(t *testing.T) {
	form := NewTUICommandForm(catalogEntryForTest(t, "install"))
	if err := form.SetValue("profile", "custom"); err != nil {
		t.Fatal(err)
	}
	if !form.ToggleMultiValue("components", "monitoring") {
		t.Fatal("component toggle failed")
	}
	args, err := form.Arguments()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(args, " "), "monitoring") {
		t.Fatalf("components missing from %q", args)
	}
}

func TestTUIDestructiveUninstallCannotUseGenericConfirmation(t *testing.T) {
	form := NewTUICommandForm(catalogEntryForTest(t, "uninstall"))
	if err := form.SetValue("delete-data", "true"); err != nil {
		t.Fatal(err)
	}
	if _, err := form.Arguments(); err == nil || !strings.Contains(err.Error(), "installation-specific") {
		t.Fatalf("destructive arguments error=%v", err)
	}
	if err := form.SetValue("confirm", "DELETE INSTALLATION test-id"); err != nil {
		t.Fatal(err)
	}
	args, err := form.Arguments()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCommand(args); err != nil {
		t.Fatalf("confirmed destructive command rejected: %v", err)
	}
}

func TestTUITextFieldAcceptsSpacesForDestructiveConfirmation(t *testing.T) {
	form := NewTUICommandForm(catalogEntryForTest(t, "uninstall"))
	form.Focus = 2
	form.ClearCurrent()
	for _, value := range "DELETE installation-id PERSISTENT DATA" {
		if value == ' ' {
			form.TypeSpace()
		} else {
			form.AppendRune(value)
		}
	}
	if form.Fields[2].Value != "DELETE installation-id PERSISTENT DATA" {
		t.Fatalf("confirmation=%q", form.Fields[2].Value)
	}
}

func TestTUIInstallRemainsInteractiveAndImportCustomSupportsComponents(t *testing.T) {
	install := NewTUICommandForm(catalogEntryForTest(t, "install"))
	installArgs, err := install.Arguments()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(" "+strings.Join(installArgs, " ")+" ", " --yes ") {
		t.Fatalf("TUI install unexpectedly became non-interactive: %q", installArgs)
	}

	importForm := NewTUICommandForm(catalogEntryForTest(t, "import-config"))
	if err := importForm.SetValue("profile", "custom"); err != nil {
		t.Fatal(err)
	}
	if !importForm.ToggleMultiValue("components", "api") || !importForm.ToggleMultiValue("components", "worker") {
		t.Fatal("import component selection unavailable")
	}
	args, err := importForm.Arguments()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(args, " "), "--components api,worker") {
		t.Fatalf("custom import components missing: %q", args)
	}
}

func TestTUISafeCommandMatchesActualArgumentsAndQuotesValues(t *testing.T) {
	form := NewTUICommandForm(catalogEntryForTest(t, "install"))
	if err := form.SetValue("runtime-dir", "runtime with space"); err != nil {
		t.Fatal(err)
	}
	review := form.SafeCommand()
	if strings.Contains(review, "--components") || !strings.Contains(review, `--runtime-dir "runtime with space"`) {
		t.Fatalf("safe command does not match actual arguments: %q", review)
	}

	uninstall := NewTUICommandForm(catalogEntryForTest(t, "uninstall"))
	_ = uninstall.SetValue("delete-data", "true")
	_ = uninstall.SetValue("confirm", "DELETE installation-id PERSISTENT DATA")
	if review := uninstall.SafeCommand(); strings.Contains(review, "--yes") || !strings.Contains(review, `--confirm "DELETE installation-id PERSISTENT DATA"`) {
		t.Fatalf("destructive review is not executable: %q", review)
	}
}

func TestTUIInstallCombinationValidationStaysInForm(t *testing.T) {
	form := NewTUICommandForm(catalogEntryForTest(t, "install"))
	_ = form.SetValue("mode", "native")
	_ = form.SetValue("profile", "full")
	if _, err := form.Arguments(); err == nil || !strings.Contains(err.Error(), "full") {
		t.Fatalf("native/full validation error=%v", err)
	}
}

func TestTUIMultiSelectShowsCurrentChoice(t *testing.T) {
	form := NewTUICommandForm(catalogEntryForTest(t, "install"))
	form.Focus = 4
	before := form.View()
	form.CycleCurrent(1)
	after := form.View()
	if before == after || !strings.Contains(after, ">worker") {
		t.Fatalf("multi-select current choice is not visible:\n%s", after)
	}
}

func catalogEntryForTest(t *testing.T, path string) CommandCatalogEntry {
	t.Helper()
	for _, entry := range CommandCatalog() {
		if entry.Path == path {
			return entry
		}
	}
	t.Fatalf("catalog entry %q not found", path)
	return CommandCatalogEntry{}
}
