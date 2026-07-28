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
