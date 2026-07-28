package deployctl

import (
	"context"
	"fmt"
	"io"
	"strings"

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
}

func NewTUIModel(catalog []CommandCatalogEntry) TUIModel {
	return TUIModel{catalog: append([]CommandCatalogEntry(nil), catalog...), screen: TUIScreenRoot}
}

func (model TUIModel) Init() tea.Cmd { return nil }

func (model TUIModel) Screen() TUIScreen { return model.screen }

func (model TUIModel) Cursor() int { return model.cursor }

func (model TUIModel) Quitting() bool { return model.quitting }

func (model TUIModel) SelectedArgs() []string {
	return append([]string(nil), model.selectedArgs...)
}

func (model TUIModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyMsg)
	if !ok {
		return model, nil
	}
	if key.Type == tea.KeyCtrlC {
		model.quitting = true
		return model, tea.Quit
	}
	if key.Type == tea.KeyEsc && model.screen != TUIScreenRoot {
		model.screen = TUIScreenRoot
		model.cursor = 0
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
	model.selectedArgs = strings.Fields(entries[model.cursor].Path)
	model.quitting = true
	return model, tea.Quit
}

func (model TUIModel) items() []string {
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
	view.WriteString("\nArrow keys navigate · Enter confirms · Esc returns · Ctrl+C exits\n")
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
