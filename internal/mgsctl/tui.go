package mgsctl

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
	TUIScreenMGSCTLTool              TUIScreen = "mgsctl-tool"
	TUIScreenForm                    TUIScreen = "form"
	TUIScreenReview                  TUIScreen = "review"
)

type tuiRootItem struct {
	key    string
	screen TUIScreen
	action string
}

var tuiRootItems = [...]tuiRootItem{
	{key: "root.install", screen: TUIScreenInstallAndDeployment},
	{key: "root.runtime", screen: TUIScreenRuntimeOperations},
	{key: "root.setup", screen: TUIScreenSetupInitialization},
	{key: "root.upgrade", screen: TUIScreenUpgradeAndConfiguration},
	{key: "root.cluster", screen: TUIScreenClusterManagement},
	{key: "root.tool", screen: TUIScreenMGSCTLTool},
	{action: "language"},
	{key: "root.exit", action: "exit"},
}

var tuiGroupScreens = map[string]TUIScreen{
	"Install and deployment":              TUIScreenInstallAndDeployment,
	"Runtime operations":                  TUIScreenRuntimeOperations,
	"Setup initialization":                TUIScreenSetupInitialization,
	"Upgrade and configuration migration": TUIScreenUpgradeAndConfiguration,
	"Cluster management":                  TUIScreenClusterManagement,
	"MGSCTL tool":                         TUIScreenMGSCTLTool,
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
	warning      string
	language     string
	saveLanguage func(string) error
	viewport     viewport.Model
}

type TUIDependencies struct {
	Language     string
	SaveLanguage func(string) error
}

func NewTUIModel(catalog []CommandCatalogEntry) TUIModel {
	return NewTUIModelWithDependencies(catalog, TUIDependencies{Language: LanguageChinese})
}

func NewTUIModelWithDependencies(catalog []CommandCatalogEntry, dependencies TUIDependencies) TUIModel {
	return TUIModel{
		catalog: append([]CommandCatalogEntry(nil), catalog...), screen: TUIScreenRoot,
		language: normalizedTUILanguage(dependencies.Language), saveLanguage: dependencies.SaveLanguage,
		viewport: viewport.New(80, 16),
	}
}

func (model TUIModel) Init() tea.Cmd { return nil }

func (model TUIModel) Screen() TUIScreen { return model.screen }

func (model TUIModel) Cursor() int { return model.cursor }

func (model TUIModel) Quitting() bool { return model.quitting }

func (model TUIModel) Language() string { return model.language }

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
		if item.action == "language" {
			if model.language == LanguageChinese {
				model.language = LanguageEnglish
			} else {
				model.language = LanguageChinese
			}
			model.warning = ""
			if model.saveLanguage != nil {
				if err := model.saveLanguage(model.language); err != nil {
					model.warning = err.Error()
				}
			}
			if model.form != nil {
				model.form.SetLocale(model.language)
			}
			return model, nil
		}
		if item.action == "exit" {
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
	form := NewTUICommandFormLocalized(entries[model.cursor], model.language)
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
			if item.action == "language" {
				key := "root.language.zh"
				if model.language == LanguageEnglish {
					key = "root.language.en"
				}
				items[index] = tuiMessage(model.language, key)
				continue
			}
			items[index] = tuiMessage(model.language, item.key)
		}
		return items
	}
	entries := model.screenEntries()
	items := make([]string, len(entries))
	for index, entry := range entries {
		items[index] = entry.Path + " - " + tuiCatalogSummary(model.language, entry)
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
	view.WriteString("mgsctl\n\n")
	if model.screen == TUIScreenForm && model.form != nil {
		fmt.Fprintf(&view, "%s\n\n", tuiCatalogSummary(model.language, model.form.Entry))
		view.WriteString(model.viewport.View())
		if model.validation != "" {
			fmt.Fprintf(&view, "\n%s: %s\n", tuiMessage(model.language, "validation.prefix"), model.validation)
		}
		view.WriteString("\n" + tuiMessage(model.language, "nav.form") + "\n")
		return view.String()
	}
	if model.screen == TUIScreenReview && model.form != nil {
		view.WriteString(tuiMessage(model.language, "review.title") + "\n\n")
		view.WriteString(model.form.SafeCommand())
		view.WriteString("\n\n" + tuiMessage(model.language, "nav.review") + "\n")
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
	if model.warning != "" {
		fmt.Fprintf(&view, "\n%s: %s\n", tuiMessage(model.language, "warning.prefix"), model.warning)
	}
	view.WriteString("\n" + tuiMessage(model.language, "nav.menu") + "\n")
	return view.String()
}

func ExecuteTUI(ctx context.Context, input io.Reader, output io.Writer) ([]string, error) {
	return ExecuteTUIWithDependencies(ctx, input, output, TUIDependencies{Language: LanguageChinese})
}

func ExecuteTUIWithDependencies(ctx context.Context, input io.Reader, output io.Writer, dependencies TUIDependencies) ([]string, error) {
	program := tea.NewProgram(NewTUIModelWithDependencies(CommandCatalog(), dependencies), tea.WithContext(ctx), tea.WithInput(input), tea.WithOutput(output), tea.WithAltScreen())
	result, err := program.Run()
	if err != nil {
		return nil, fmt.Errorf("run mgsctl TUI: %w", err)
	}
	model, ok := result.(TUIModel)
	if !ok {
		return nil, fmt.Errorf("run mgsctl TUI: unexpected model %T", result)
	}
	return model.SelectedArgs(), nil
}
