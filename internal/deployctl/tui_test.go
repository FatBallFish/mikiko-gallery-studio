package deployctl

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTUIRootNavigationAndExitKeys(t *testing.T) {
	model := NewTUIModel(CommandCatalog())
	if model.Screen() != TUIScreenRoot || model.Cursor() != 0 {
		t.Fatalf("initial model screen=%q cursor=%d", model.Screen(), model.Cursor())
	}

	model = updateTUIWithKeys(t, model, "down")
	if model.Cursor() != 1 {
		t.Fatalf("down cursor=%d", model.Cursor())
	}
	model = updateTUIWithKeys(t, model, "up", "2")
	if model.Cursor() != 1 || model.Screen() != TUIScreenRoot {
		t.Fatalf("number selection screen=%q cursor=%d", model.Screen(), model.Cursor())
	}
	model = updateTUIWithKeys(t, model, "enter")
	if model.Screen() != TUIScreenRuntimeOperations {
		t.Fatalf("enter screen=%q", model.Screen())
	}
	model = updateTUIWithKeys(t, model, "esc")
	if model.Screen() != TUIScreenRoot {
		t.Fatalf("escape screen=%q", model.Screen())
	}
	model = updateTUIWithKeys(t, model, "0")
	if model.Quitting() || model.Cursor() != 6 {
		t.Fatalf("exit selection quitting=%t cursor=%d", model.Quitting(), model.Cursor())
	}
	model = updateTUIWithKeys(t, model, "enter")
	if !model.Quitting() {
		t.Fatal("confirmed root Exit did not quit")
	}
}

func TestTUIControlCQuitsFromEveryScreen(t *testing.T) {
	for _, keys := range [][]string{{"ctrl+c"}, {"3", "enter", "ctrl+c"}} {
		model := updateTUIWithKeys(t, NewTUIModel(CommandCatalog()), keys...)
		if !model.Quitting() {
			t.Fatalf("keys %v did not quit from %q", keys, model.Screen())
		}
	}
}

func TestTUIViewNamesKeyboardControls(t *testing.T) {
	view := NewTUIModel(CommandCatalog()).View()
	for _, expected := range []string{"deployctl", "Arrow keys", "Enter", "Esc", "Ctrl+C"} {
		if !strings.Contains(view, expected) {
			t.Errorf("View() missing %q: %q", expected, view)
		}
	}
}

func TestTUICommandFormRequiresReviewBeforeReturningArguments(t *testing.T) {
	model := updateTUIWithKeys(t, NewTUIModel(CommandCatalog()), "2", "enter", "1", "enter")
	if model.Screen() != TUIScreenForm || len(model.SelectedArgs()) != 0 {
		t.Fatalf("command selection screen=%q args=%q", model.Screen(), model.SelectedArgs())
	}
	model = updateTUIWithKeys(t, model, "enter")
	if model.Screen() != TUIScreenReview || len(model.SelectedArgs()) == 0 || model.Quitting() {
		t.Fatalf("review screen=%q args=%q quitting=%t", model.Screen(), model.SelectedArgs(), model.Quitting())
	}
	model = updateTUIWithKeys(t, model, "esc")
	if model.Screen() != TUIScreenForm {
		t.Fatalf("review escape screen=%q", model.Screen())
	}
	model = updateTUIWithKeys(t, model, "enter", "enter")
	if !model.Quitting() {
		t.Fatal("review confirmation did not quit")
	}
}

func TestTUILongFormScrollsToKeepFocusedFieldVisible(t *testing.T) {
	model := updateTUIWithKeys(t, NewTUIModel(CommandCatalog()), "1", "enter", "1", "enter")
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	model = updated.(TUIModel)
	model = updateTUIWithKeys(t, model, "down", "down", "down", "down", "down", "down", "down", "down", "down", "down", "down", "down")
	view := model.View()
	if !strings.Contains(view, model.form.Fields[model.form.Focus].Label) {
		t.Fatalf("focused field is outside viewport: focus=%d\n%s", model.form.Focus, view)
	}
	if strings.Contains(view, "Mode: docker") {
		t.Fatalf("long form did not scroll away from first field:\n%s", view)
	}
}

func updateTUIWithKeys(t *testing.T, model TUIModel, keys ...string) TUIModel {
	t.Helper()
	for _, key := range keys {
		updated, _ := model.Update(tuiKeyMessage(t, key))
		var ok bool
		model, ok = updated.(TUIModel)
		if !ok {
			t.Fatalf("Update(%q) returned %T", key, updated)
		}
	}
	return model
}

func tuiKeyMessage(t *testing.T, key string) tea.KeyMsg {
	t.Helper()
	switch key {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	default:
		if len([]rune(key)) == 1 {
			return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
		}
		t.Fatalf("unsupported test key %q", key)
		return tea.KeyMsg{}
	}
}
