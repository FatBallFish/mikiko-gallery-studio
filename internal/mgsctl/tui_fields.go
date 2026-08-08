package mgsctl

import (
	"fmt"
	"slices"
	"strings"
)

type tuiFieldKind string

const (
	tuiFieldText   tuiFieldKind = "text"
	tuiFieldChoice tuiFieldKind = "choice"
	tuiFieldBool   tuiFieldKind = "bool"
	tuiFieldMulti  tuiFieldKind = "multi"
)

type TUIField struct {
	Name      string
	Label     string
	Kind      tuiFieldKind
	Value     string
	Sensitive bool
	ReadOnly  bool
	Choices   []string
	Selected  map[string]bool
	Choice    int
}

type TUICommandForm struct {
	Entry  CommandCatalogEntry
	Fields []TUIField
	Focus  int
	Locale string

	installValues         map[string]string
	installSelected       map[string]bool
	installCustomSelected map[string]bool
}

func NewTUICommandForm(entry CommandCatalogEntry) TUICommandForm {
	return NewTUICommandFormLocalized(entry, LanguageChinese)
}

func NewTUICommandFormLocalized(entry CommandCatalogEntry, language string) TUICommandForm {
	form := TUICommandForm{Entry: entry, Locale: normalizedTUILanguage(language)}
	if entry.Path != "install" {
		form.Fields = tuiFieldsForCommand(entry.Path, form.Locale)
		return form
	}
	form.installValues = map[string]string{
		"mode": "docker", "profile": "full", "topology": "single", "role": "single",
		"runtime-dir": ".", "storage-driver": "s3", "image-tag": "latest",
		"api-port": "8080", "gateway-port": "80", "user-web-port": "5173",
		"admin-web-port": "5174", "docs-web-port": "5175", "monitoring-port": "9090",
		"docs-probe-url":   "",
		"external-gateway": "false", "migrate": "false", "overwrite": "false", "yes": "false",
	}
	form.installSelected = selectedComponents(fullPreset)
	form.rebuildInstallFields("")
	return form
}

func (form *TUICommandForm) SetLocale(language string) {
	form.Locale = normalizedTUILanguage(language)
	if form.Entry.Path == "install" {
		focus := form.focusedFieldName()
		form.rebuildInstallFields(focus)
		return
	}
	for index := range form.Fields {
		form.Fields[index].Label = tuiMessage(form.Locale, "field."+form.Fields[index].Name)
	}
}

func (form *TUICommandForm) MoveFocus(delta int) {
	if len(form.Fields) > 0 {
		form.Focus = (form.Focus + delta + len(form.Fields)) % len(form.Fields)
	}
}

func (form *TUICommandForm) CycleCurrent(delta int) {
	if form.Focus < 0 || form.Focus >= len(form.Fields) {
		return
	}
	field := &form.Fields[form.Focus]
	if len(field.Choices) == 0 {
		return
	}
	field.Choice = (field.Choice + delta + len(field.Choices)) % len(field.Choices)
	if field.Kind != tuiFieldChoice {
		return
	}
	name, value := field.Name, field.Choices[field.Choice]
	_ = form.SetValue(name, value)
}

func (form *TUICommandForm) ToggleCurrent() {
	if form.Focus < 0 || form.Focus >= len(form.Fields) {
		return
	}
	field := &form.Fields[form.Focus]
	switch field.Kind {
	case tuiFieldBool:
		_ = form.SetValue(field.Name, fmt.Sprintf("%t", field.Value != "true"))
	case tuiFieldChoice:
		form.CycleCurrent(1)
	case tuiFieldMulti:
		if len(field.Choices) > 0 && !field.ReadOnly {
			form.ToggleMultiValue(field.Name, field.Choices[field.Choice])
		}
	}
}

func (form *TUICommandForm) AppendRune(value rune) {
	if form.Focus >= 0 && form.Focus < len(form.Fields) && form.Fields[form.Focus].Kind == tuiFieldText {
		form.Fields[form.Focus].Value += string(value)
		form.recordFieldValue(form.Fields[form.Focus])
	}
}

func (form *TUICommandForm) TypeSpace() {
	if form.Focus >= 0 && form.Focus < len(form.Fields) && form.Fields[form.Focus].Kind == tuiFieldText {
		form.Fields[form.Focus].Value += " "
		form.recordFieldValue(form.Fields[form.Focus])
		return
	}
	form.ToggleCurrent()
}

func (form *TUICommandForm) Backspace() {
	if form.Focus < 0 || form.Focus >= len(form.Fields) || form.Fields[form.Focus].Kind != tuiFieldText {
		return
	}
	runes := []rune(form.Fields[form.Focus].Value)
	if len(runes) > 0 {
		form.Fields[form.Focus].Value = string(runes[:len(runes)-1])
		form.recordFieldValue(form.Fields[form.Focus])
	}
}

func (form *TUICommandForm) ClearCurrent() {
	if form.Focus >= 0 && form.Focus < len(form.Fields) && form.Fields[form.Focus].Kind == tuiFieldText {
		form.Fields[form.Focus].Value = ""
		form.recordFieldValue(form.Fields[form.Focus])
	}
}

func (form *TUICommandForm) SetValue(name, value string) error {
	field := form.field(name)
	if field == nil {
		return fmt.Errorf("unknown TUI field %q", name)
	}
	if field.Kind == tuiFieldChoice && !containsString(field.Choices, value) {
		return fmt.Errorf("invalid %s value %q", name, value)
	}
	if field.Kind == tuiFieldBool && value != "true" && value != "false" {
		return fmt.Errorf("invalid boolean %s value %q", name, value)
	}
	field.Value = value
	if field.Kind == tuiFieldChoice {
		field.Choice = slices.Index(field.Choices, value)
	}
	if form.Entry.Path != "install" {
		return nil
	}
	focus := name
	previousProfile := form.installValues["profile"]
	form.installValues[name] = value
	if name == "profile" {
		if previousProfile == "custom" {
			form.installCustomSelected = cloneSelectedComponents(form.installSelected)
		}
		switch value {
		case "full":
			form.installSelected = selectedComponents(fullPreset)
			form.installValues["storage-driver"] = "s3"
		case "core":
			form.installSelected = selectedComponents(applicationPreset)
			if form.installValues["topology"] == "cluster" {
				form.installValues["storage-driver"] = "s3"
			} else {
				form.installValues["storage-driver"] = "local"
			}
		case "custom":
			if len(form.installCustomSelected) == 0 {
				form.installCustomSelected = cloneSelectedComponents(form.installSelected)
			}
			form.installSelected = cloneSelectedComponents(form.installCustomSelected)
		}
	}
	if name == "topology" && value == "cluster" {
		form.installValues["role"] = "control"
		if form.installValues["storage-driver"] == "local" {
			form.installValues["storage-driver"] = "s3"
		}
	}
	if name == "topology" && value == "single" && form.installValues["role"] == "control" {
		form.installValues["role"] = "single"
	}
	form.rebuildInstallFields(focus)
	return nil
}

func (form *TUICommandForm) ToggleMultiValue(name, value string) bool {
	field := form.field(name)
	if field == nil || field.Kind != tuiFieldMulti || field.ReadOnly || !containsString(field.Choices, value) {
		return false
	}
	field.Selected[value] = !field.Selected[value]
	if form.Entry.Path == "install" {
		form.installSelected = cloneSelectedComponents(field.Selected)
		form.installCustomSelected = cloneSelectedComponents(field.Selected)
		form.rebuildInstallFields(name)
	}
	return true
}

func (form TUICommandForm) Arguments() ([]string, error) {
	args := strings.Fields(form.Entry.Path)
	values := make(map[string]string, len(form.Fields))
	for _, field := range form.Fields {
		values[field.Name] = field.Value
		value := field.Value
		if field.Kind == tuiFieldMulti {
			selected := make([]string, 0, len(field.Selected))
			for _, choice := range field.Choices {
				if field.Selected[choice] {
					selected = append(selected, choice)
				}
			}
			value = strings.Join(selected, ",")
		}
		if field.Kind == tuiFieldBool {
			if form.Entry.Path == "uninstall" && field.Name == "yes" && values["delete-data"] == "true" {
				continue
			}
			if value == "true" {
				args = append(args, "--"+field.Name)
			} else if field.Name == "migrate" {
				args = append(args, "--migrate=false")
			}
			continue
		}
		if strings.TrimSpace(value) != "" {
			args = append(args, "--"+field.Name, value)
		}
	}
	if (form.Entry.Path == "install" || form.Entry.Path == "import-config") && values["profile"] != "custom" {
		args = removeFlag(args, "--components")
	}
	if form.Entry.Path == "uninstall" && values["delete-data"] == "true" && strings.TrimSpace(values["confirm"]) == "" {
		return nil, fmt.Errorf("persistent data deletion requires the installation-specific confirmation phrase")
	}
	command, err := ParseCommand(args)
	if err != nil {
		return nil, err
	}
	if err := validateTUICommand(command); err != nil {
		return nil, err
	}
	return args, nil
}

func (form TUICommandForm) SafeCommand() string {
	args, err := form.Arguments()
	if err != nil {
		return "mgsctl " + form.Entry.Path
	}
	sensitiveFlags := make(map[string]bool)
	for _, field := range form.Fields {
		if field.Sensitive {
			sensitiveFlags["--"+field.Name] = true
		}
	}
	formatted := make([]string, len(args))
	for index, argument := range args {
		if index > 0 && sensitiveFlags[args[index-1]] {
			argument = "<redacted>"
		}
		formatted[index] = safeTUIArgument(argument)
	}
	return "mgsctl " + strings.Join(formatted, " ")
}

func (form TUICommandForm) View() string {
	var view strings.Builder
	for index, field := range form.Fields {
		marker := "  "
		if index == form.Focus {
			marker = "> "
		}
		value := field.Value
		if field.Sensitive && value != "" {
			value = "••••••••"
		}
		if field.Kind == tuiFieldMulti {
			selected := 0
			for _, enabled := range field.Selected {
				if enabled {
					selected++
				}
			}
			choice := field.Choices[field.Choice]
			checked := " "
			if field.Selected[choice] {
				checked = "x"
			}
			status := fmt.Sprintf(tuiMessage(form.Locale, "multi.status"), selected)
			if field.ReadOnly {
				status += "; " + tuiMessage(form.Locale, "multi.readonly")
			}
			value = fmt.Sprintf("[%s] >%s (%s)", checked, choice, status)
		}
		fmt.Fprintf(&view, "%s%s: %s\n", marker, field.Label, value)
	}
	return view.String()
}

func (form *TUICommandForm) rebuildInstallFields(focusName string) {
	text := form.textField
	choice := form.choiceField
	boolean := form.boolField
	components := TUIField{
		Name: "components", Label: tuiMessage(form.Locale, "field.components"), Kind: tuiFieldMulti,
		Choices: componentNames(), Selected: cloneSelectedComponents(form.installSelected), ReadOnly: form.installValues["profile"] != "custom",
	}
	fields := []TUIField{
		choice("mode", form.installValues["mode"], "docker", "native"),
		choice("profile", form.installValues["profile"], "full", "core", "custom"),
		choice("topology", form.installValues["topology"], "single", "cluster"),
		choice("role", form.installValues["role"], "single", "control"), components,
		text("runtime-dir", form.installValues["runtime-dir"]),
		choice("storage-driver", form.installValues["storage-driver"], "local", "s3"),
		text("docs-probe-url", form.installValues["docs-probe-url"]),
	}
	if form.installValues["mode"] == "docker" {
		fields = append(fields, text("image-tag", form.installValues["image-tag"]))
	}
	ports := []struct {
		component Component
		name      string
	}{
		{ComponentAPI, "api-port"}, {ComponentGateway, "gateway-port"}, {ComponentUserWeb, "user-web-port"},
		{ComponentAdminWeb, "admin-web-port"}, {ComponentDocsWeb, "docs-web-port"}, {ComponentMonitoring, "monitoring-port"},
	}
	for _, port := range ports {
		if form.installSelected[string(port.component)] {
			fields = append(fields, text(port.name, form.installValues[port.name]))
		}
	}
	fields = append(fields,
		boolean("external-gateway", form.installValues["external-gateway"] == "true"),
		boolean("migrate", form.installValues["migrate"] == "true"),
		boolean("overwrite", form.installValues["overwrite"] == "true"),
		boolean("yes", form.installValues["yes"] == "true"),
	)
	form.Fields = fields
	form.Focus = 0
	if focusName != "" {
		for index := range form.Fields {
			if form.Fields[index].Name == focusName {
				form.Focus = index
				break
			}
		}
	}
}

func (form TUICommandForm) textField(name, value string) TUIField {
	return TUIField{Name: name, Label: tuiMessage(form.Locale, "field."+name), Kind: tuiFieldText, Value: value}
}

func (form TUICommandForm) choiceField(name, value string, choices ...string) TUIField {
	field := TUIField{Name: name, Label: tuiMessage(form.Locale, "field."+name), Kind: tuiFieldChoice, Value: value, Choices: choices}
	field.Choice = max(0, slices.Index(choices, value))
	return field
}

func (form TUICommandForm) boolField(name string, value bool) TUIField {
	return TUIField{Name: name, Label: tuiMessage(form.Locale, "field."+name), Kind: tuiFieldBool, Value: fmt.Sprintf("%t", value)}
}

func (form *TUICommandForm) recordFieldValue(field TUIField) {
	if form.Entry.Path == "install" {
		form.installValues[field.Name] = field.Value
	}
}

func (form TUICommandForm) focusedFieldName() string {
	if form.Focus >= 0 && form.Focus < len(form.Fields) {
		return form.Fields[form.Focus].Name
	}
	return ""
}

func validateTUICommand(command Command) error {
	switch command.Kind {
	case CommandInstall:
		_, err := BuildInstallPlan(*command.Install)
		return err
	case CommandImportConfig:
		options := command.ImportConfig
		_, err := BuildInstallPlan(InstallInput{
			Mode: options.Mode, Profile: options.Profile, Topology: options.Topology, Role: options.Role,
			Components: options.Components, RuntimeDir: options.RuntimeDir, StorageDriver: options.StorageDriver,
			PublicAPIURL: options.PublicAPIURL, ApplicationVersion: options.ApplicationVersion,
			ImageRegistry: options.ImageRegistry, ImageTag: options.ImageTag, ReleaseVersion: options.ReleaseVersion,
		})
		return err
	default:
		return nil
	}
}

func safeTUIArgument(value string) string {
	if value != "" && strings.IndexFunc(value, func(character rune) bool {
		return !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_./:@,+-=%", character)
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func (form *TUICommandForm) field(name string) *TUIField {
	for index := range form.Fields {
		if form.Fields[index].Name == name {
			return &form.Fields[index]
		}
	}
	return nil
}

func tuiFieldsForCommand(path, language string) []TUIField {
	form := TUICommandForm{Locale: normalizedTUILanguage(language)}
	text := form.textField
	choice := form.choiceField
	boolean := form.boolField
	runtimeDir := func() []TUIField { return []TUIField{text("runtime-dir", "")} }
	switch path {
	case "import-config":
		components := TUIField{Name: "components", Label: tuiMessage(form.Locale, "field.components"), Kind: tuiFieldMulti, Choices: componentNames(), Selected: map[string]bool{}}
		return []TUIField{text("source", ".env"), text("runtime-dir", "."), choice("mode", "docker", "docker", "native"), choice("profile", "core", "full", "core", "custom"), choice("topology", "single", "single", "cluster"), choice("role", "single", "single", "control"), components, choice("storage-driver", "local", "local", "s3"), text("public-api-url", ""), text("application-version", DefaultApplicationVersion), text("image-registry", ""), text("image-tag", DefaultApplicationVersion), text("release-version", "")}
	case "status", "doctor", "restart", "setup status", "setup token show":
		return runtimeDir()
	case "setup token reset":
		return append(runtimeDir(), boolean("yes", true))
	case "version":
		return []TUIField{boolean("json", false)}
	case "self-update":
		return []TUIField{text("version", "latest"), text("release-base-url", DefaultMGSCTLReleaseBaseURL), text("download-url", ""), text("sha256", ""), boolean("yes", true)}
	case "upgrade":
		return []TUIField{text("runtime-dir", ""), text("image-registry", ""), text("image-tag", "latest"), text("release-version", "latest"), boolean("migrate", true)}
	case "uninstall":
		return []TUIField{text("runtime-dir", ""), boolean("delete-data", false), text("confirm", ""), boolean("yes", true)}
	case "cluster token create":
		return []TUIField{choice("role", "worker", "api", "worker", "web"), text("ttl", "10m"), text("runtime-dir", "")}
	case "cluster join":
		return []TUIField{text("server", "http://127.0.0.1:8080"), {Name: "token", Label: tuiMessage(form.Locale, "field.token"), Kind: tuiFieldText, Value: "pgjoin.v1.placeholder", Sensitive: true}, text("runtime-dir", "."), choice("mode", "docker", "docker", "native"), text("application-version", DefaultApplicationVersion), text("image-registry", ""), text("image-tag", DefaultApplicationVersion), text("release-version", ""), text("api-port", "8080"), text("gateway-port", "80"), text("user-web-port", "5173"), text("admin-web-port", "5174"), text("docs-web-port", "5175"), text("docs-probe-url", "")}
	default:
		return nil
	}
}

func componentNames() []string {
	names := make([]string, len(componentOrder))
	for index, component := range componentOrder {
		names[index] = string(component)
	}
	return names
}

func selectedComponents(components []Component) map[string]bool {
	selected := make(map[string]bool, len(components))
	for _, component := range components {
		selected[string(component)] = true
	}
	return selected
}

func cloneSelectedComponents(selected map[string]bool) map[string]bool {
	cloned := make(map[string]bool, len(selected))
	for component, enabled := range selected {
		if enabled {
			cloned[component] = true
		}
	}
	return cloned
}

func containsString(values []string, value string) bool {
	return slices.Contains(values, value)
}

func removeFlag(args []string, flag string) []string {
	for index := 0; index < len(args); index++ {
		if args[index] == flag && index+1 < len(args) {
			return append(args[:index], args[index+2:]...)
		}
	}
	return args
}
