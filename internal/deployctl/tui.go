package deployctl

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type TUIScreen string

const (
	TUIScreenRoot                    TUIScreen = "root"
	TUIScreenInstallAndDeployment    TUIScreen = "install-and-deployment"
	TUIScreenRuntimeOperations       TUIScreen = "runtime-operations"
	TUIScreenSetupInitialization     TUIScreen = "setup-initialization"
	TUIScreenUpgradeAndConfiguration TUIScreen = "upgrade-and-configuration-migration"
	TUIScreenClusterManagement       TUIScreen = "cluster-management"
	TUIScreenDeployctlTool           TUIScreen = "deployctl-tool"
	TUIScreenForm                    TUIScreen = "form"
	TUIScreenReview                  TUIScreen = "review"
)

type tuiRootItem struct {
	label  string
	screen TUIScreen
}

var tuiRootItems = [...]tuiRootItem{
	{label: "Install and deployment", screen: TUIScreenInstallAndDeployment},
	{label: "Runtime operations", screen: TUIScreenRuntimeOperations},
	{label: "Setup initialization", screen: TUIScreenSetupInitialization},
	{label: "Upgrade and configuration migration", screen: TUIScreenUpgradeAndConfiguration},
	{label: "Cluster management", screen: TUIScreenClusterManagement},
	{label: "Deployctl tool", screen: TUIScreenDeployctlTool},
	{label: "Exit"},
}

var tuiGroupScreens = map[string]TUIScreen{
	"Install and deployment":              TUIScreenInstallAndDeployment,
	"Runtime operations":                  TUIScreenRuntimeOperations,
	"Setup initialization":                TUIScreenSetupInitialization,
	"Upgrade and configuration migration": TUIScreenUpgradeAndConfiguration,
	"Cluster management":                  TUIScreenClusterManagement,
	"Deployctl tool":                      TUIScreenDeployctlTool,
}

type TUIModel struct {
	catalog      []CommandCatalogEntry
	screen       TUIScreen
	cursor       int
	quitting     bool
	selectedArgs []string
	parentScreen TUIScreen
	form         *TUICommandForm
	validation   string
	viewport     viewport.Model
}

func NewTUIModel(catalog []CommandCatalogEntry) TUIModel {
	return TUIModel{catalog: append([]CommandCatalogEntry(nil), catalog...), screen: TUIScreenRoot, viewport: viewport.New(80, 16)}
}

func (model TUIModel) Init() tea.Cmd { return nil }

func (model TUIModel) Screen() TUIScreen { return model.screen }

func (model TUIModel) Cursor() int { return model.cursor }

func (model TUIModel) Quitting() bool { return model.quitting }

func (model TUIModel) SelectedArgs() []string {
	return append([]string(nil), model.selectedArgs...)
}

func (model TUIModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := message.(tea.WindowSizeMsg); ok {
		model.viewport.Width = max(20, size.Width)
		model.viewport.Height = max(3, size.Height-8)
		model.syncFormViewport()
		return model, nil
	}
	key, ok := message.(tea.KeyMsg)
	if !ok {
		return model, nil
	}
	if key.Type == tea.KeyCtrlC {
		model.quitting = true
		return model, tea.Quit
	}
	if key.Type == tea.KeyEsc && model.screen != TUIScreenRoot {
		switch model.screen {
		case TUIScreenReview:
			model.screen = TUIScreenForm
		case TUIScreenForm:
			model.screen = model.parentScreen
		default:
			model.screen = TUIScreenRoot
		}
		model.cursor = 0
		return model, nil
	}
	if model.screen == TUIScreenForm {
		return model.updateForm(key)
	}
	if model.screen == TUIScreenReview {
		if key.Type == tea.KeyEnter {
			model.quitting = true
			return model, tea.Quit
		}
		return model, nil
	}

	items := model.items()
	switch key.Type {
	case tea.KeyUp:
		if model.cursor > 0 {
			model.cursor--
		}
	case tea.KeyDown:
		if model.cursor+1 < len(items) {
			model.cursor++
		}
	case tea.KeyRunes:
		if len(key.Runes) == 1 {
			model.selectNumber(key.Runes[0], len(items))
		}
	case tea.KeyEnter:
		return model.confirmSelection()
	}
	return model, nil
}

func (model *TUIModel) selectNumber(key rune, itemCount int) {
	if model.screen == TUIScreenRoot && key == '0' {
		model.cursor = len(tuiRootItems) - 1
		return
	}
	if key >= '1' && key <= '9' {
		index := int(key - '1')
		if index < itemCount {
			model.cursor = index
		}
	}
}

func (model TUIModel) confirmSelection() (tea.Model, tea.Cmd) {
	if model.screen == TUIScreenRoot {
		item := tuiRootItems[model.cursor]
		if item.screen == "" {
			model.quitting = true
			return model, tea.Quit
		}
		model.screen = item.screen
		model.cursor = 0
		return model, nil
	}
	entries := model.screenEntries()
	if len(entries) == 0 {
		return model, nil
	}
	form := NewTUICommandForm(entries[model.cursor])
	model.parentScreen = model.screen
	model.screen = TUIScreenForm
	model.cursor = 0
	model.form = &form
	model.validation = ""
	model.syncFormViewport()
	return model, nil
}

func (model TUIModel) updateForm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if model.form == nil {
		return model, nil
	}
	switch key.Type {
	case tea.KeyTab, tea.KeyDown:
		model.form.MoveFocus(1)
	case tea.KeyShiftTab, tea.KeyUp:
		model.form.MoveFocus(-1)
	case tea.KeyLeft:
		model.form.CycleCurrent(-1)
	case tea.KeyRight:
		model.form.CycleCurrent(1)
	case tea.KeySpace:
		model.form.TypeSpace()
	case tea.KeyBackspace, tea.KeyDelete:
		model.form.Backspace()
	case tea.KeyCtrlU:
		model.form.ClearCurrent()
	case tea.KeyRunes:
		for _, value := range key.Runes {
			model.form.AppendRune(value)
		}
	case tea.KeyEnter:
		args, err := model.form.Arguments()
		if err != nil {
			model.validation = err.Error()
			model.syncFormViewport()
			return model, nil
		}
		model.selectedArgs = args
		model.validation = ""
		model.screen = TUIScreenReview
	}
	model.syncFormViewport()
	return model, nil
}

func (model *TUIModel) syncFormViewport() {
	if model.form == nil {
		return
	}
	model.viewport.SetContent(model.form.View())
	if model.form.Focus < model.viewport.YOffset {
		model.viewport.SetYOffset(model.form.Focus)
	}
	if model.form.Focus >= model.viewport.YOffset+model.viewport.Height {
		model.viewport.SetYOffset(model.form.Focus - model.viewport.Height + 1)
	}
}

func (model TUIModel) items() []string {
	if model.screen == TUIScreenForm || model.screen == TUIScreenReview {
		return nil
	}
	if model.screen == TUIScreenRoot {
		items := make([]string, len(tuiRootItems))
		for index, item := range tuiRootItems {
			items[index] = item.label
		}
		return items
	}
	entries := model.screenEntries()
	items := make([]string, len(entries))
	for index, entry := range entries {
		items[index] = entry.Path + " - " + entry.Summary
	}
	return items
}

func (model TUIModel) screenEntries() []CommandCatalogEntry {
	entries := make([]CommandCatalogEntry, 0)
	for _, entry := range model.catalog {
		if tuiGroupScreens[entry.Group] == model.screen {
			entries = append(entries, entry)
		}
	}
	return entries
}

func (model TUIModel) View() string {
	if model.quitting {
		return ""
	}
	var view strings.Builder
	view.WriteString("deployctl\n\n")
	if model.screen == TUIScreenForm && model.form != nil {
		fmt.Fprintf(&view, "%s\n\n", model.form.Entry.Summary)
		view.WriteString(model.viewport.View())
		if model.validation != "" {
			fmt.Fprintf(&view, "\nValidation error: %s\n", model.validation)
		}
		view.WriteString("\nArrow keys navigate · Tab changes field · Space toggles · Enter reviews · Esc returns · Ctrl+C exits\n")
		return view.String()
	}
	if model.screen == TUIScreenReview && model.form != nil {
		view.WriteString("Review command\n\n")
		view.WriteString(model.form.SafeCommand())
		view.WriteString("\n\nEnter confirms · Esc edits · Ctrl+C exits\n")
		return view.String()
	}
	for index, item := range model.items() {
		marker := "  "
		if index == model.cursor {
			marker = "> "
		}
		number := index + 1
		if model.screen == TUIScreenRoot && index == len(tuiRootItems)-1 {
			number = 0
		}
		fmt.Fprintf(&view, "%s%d. %s\n", marker, number, item)
	}
	view.WriteString("\nArrow keys navigate · Enter confirms · Space toggles · Esc returns · Ctrl+C exits\n")
	return view.String()
}

func ExecuteTUI(ctx context.Context, input io.Reader, output io.Writer) ([]string, error) {
	program := tea.NewProgram(NewTUIModel(CommandCatalog()), tea.WithContext(ctx), tea.WithInput(input), tea.WithOutput(output), tea.WithAltScreen())
	result, err := program.Run()
	if err != nil {
		return nil, fmt.Errorf("run deployctl TUI: %w", err)
	}
	model, ok := result.(TUIModel)
	if !ok {
		return nil, fmt.Errorf("run deployctl TUI: unexpected model %T", result)
	}
	return model.SelectedArgs(), nil
}
