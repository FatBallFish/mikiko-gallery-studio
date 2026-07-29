package deployctl

import (
	"fmt"
	"strconv"
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
	Choices   []string
	Selected  map[string]bool
	Choice    int
}

type TUICommandForm struct {
	Entry  CommandCatalogEntry
	Fields []TUIField
	Focus  int
}

func NewTUICommandForm(entry CommandCatalogEntry) TUICommandForm {
	return TUICommandForm{Entry: entry, Fields: tuiFieldsForCommand(entry.Path)}
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
	if field.Kind == tuiFieldChoice {
		field.Value = field.Choices[field.Choice]
	}
}

func (form *TUICommandForm) ToggleCurrent() {
	if form.Focus < 0 || form.Focus >= len(form.Fields) {
		return
	}
	field := &form.Fields[form.Focus]
	switch field.Kind {
	case tuiFieldBool:
		field.Value = fmt.Sprintf("%t", field.Value != "true")
	case tuiFieldChoice:
		form.CycleCurrent(1)
	case tuiFieldMulti:
		if len(field.Choices) > 0 {
			value := field.Choices[field.Choice]
			field.Selected[value] = !field.Selected[value]
		}
	}
}

func (form *TUICommandForm) AppendRune(value rune) {
	if form.Focus >= 0 && form.Focus < len(form.Fields) && form.Fields[form.Focus].Kind == tuiFieldText {
		form.Fields[form.Focus].Value += string(value)
	}
}

func (form *TUICommandForm) TypeSpace() {
	if form.Focus >= 0 && form.Focus < len(form.Fields) && form.Fields[form.Focus].Kind == tuiFieldText {
		form.Fields[form.Focus].Value += " "
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
	}
}

func (form *TUICommandForm) ClearCurrent() {
	if form.Focus >= 0 && form.Focus < len(form.Fields) && form.Fields[form.Focus].Kind == tuiFieldText {
		form.Fields[form.Focus].Value = ""
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
		for index, choice := range field.Choices {
			if choice == value {
				field.Choice = index
				break
			}
		}
	}
	return nil
}

func (form *TUICommandForm) ToggleMultiValue(name, value string) bool {
	field := form.field(name)
	if field == nil || field.Kind != tuiFieldMulti || !containsString(field.Choices, value) {
		return false
	}
	field.Selected[value] = !field.Selected[value]
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
		return "deployctl " + form.Entry.Path
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
	return "deployctl " + strings.Join(formatted, " ")
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
			parts := make([]string, 0, len(field.Choices))
			for _, choice := range field.Choices {
				checked := " "
				if field.Selected[choice] {
					checked = "x"
				}
				current := ""
				if index == form.Focus && field.Choice < len(field.Choices) && field.Choices[field.Choice] == choice {
					current = ">"
				}
				parts = append(parts, fmt.Sprintf("[%s] %s%s", checked, current, choice))
			}
			value = strings.Join(parts, " ")
		}
		fmt.Fprintf(&view, "%s%s: %s\n", marker, field.Label, value)
	}
	return view.String()
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
	return strconv.Quote(value)
}

func (form *TUICommandForm) field(name string) *TUIField {
	for index := range form.Fields {
		if form.Fields[index].Name == name {
			return &form.Fields[index]
		}
	}
	return nil
}

func tuiFieldsForCommand(path string) []TUIField {
	text := func(name, label, value string) TUIField {
		return TUIField{Name: name, Label: label, Kind: tuiFieldText, Value: value}
	}
	choice := func(name, label, value string, choices ...string) TUIField {
		field := TUIField{Name: name, Label: label, Kind: tuiFieldChoice, Value: value, Choices: choices}
		for index, candidate := range choices {
			if candidate == value {
				field.Choice = index
			}
		}
		return field
	}
	boolean := func(name, label string, value bool) TUIField {
		return TUIField{Name: name, Label: label, Kind: tuiFieldBool, Value: fmt.Sprintf("%t", value)}
	}
	runtimeDir := func() []TUIField { return []TUIField{text("runtime-dir", "Runtime directory", ".")} }
	switch path {
	case "install":
		components := TUIField{Name: "components", Label: "Components", Kind: tuiFieldMulti, Choices: componentNames(), Selected: map[string]bool{"api": true, "worker": true}}
		return []TUIField{
			choice("mode", "Mode", "docker", "docker", "native"), choice("profile", "Profile", "core", "full", "core", "custom"),
			choice("topology", "Topology", "single", "single", "cluster"), choice("role", "Role", "single", "single", "control"), components,
			text("runtime-dir", "Runtime directory", "."), choice("storage-driver", "Object storage", "local", "local", "s3"),
			text("public-api-url", "Public API URL", ""), text("application-version", "Application version", DefaultApplicationVersion),
			text("image-registry", "Image registry", ""), text("image-tag", "Image tag", DefaultApplicationVersion), text("release-version", "Release version", ""),
			text("api-port", "API port", "8080"), text("gateway-port", "Gateway port", ""), text("user-web-port", "User Web port", ""),
			text("admin-web-port", "Admin Web port", ""), text("docs-web-port", "Docs Web port", ""), text("monitoring-port", "Monitoring port", ""),
			boolean("external-gateway", "External gateway configured", false), boolean("migrate", "Run migration", false), boolean("overwrite", "Overwrite incomplete config", false), boolean("yes", "Skip terminal confirmation", false),
		}
	case "import-config":
		components := TUIField{Name: "components", Label: "Components", Kind: tuiFieldMulti, Choices: componentNames(), Selected: map[string]bool{}}
		return []TUIField{text("source", "Legacy environment file", ".env"), text("runtime-dir", "Runtime directory", "."), choice("mode", "Mode", "docker", "docker", "native"), choice("profile", "Profile", "core", "full", "core", "custom"), choice("topology", "Topology", "single", "single", "cluster"), choice("role", "Role", "single", "single", "control"), components, choice("storage-driver", "Object storage", "local", "local", "s3"), text("public-api-url", "Public API URL", ""), text("application-version", "Application version", DefaultApplicationVersion), text("image-registry", "Image registry", ""), text("image-tag", "Image tag", DefaultApplicationVersion), text("release-version", "Release version", "")}
	case "status", "doctor", "restart", "setup status", "setup token show":
		return runtimeDir()
	case "setup token reset":
		return append(runtimeDir(), boolean("yes", "Confirm token reset", true))
	case "version":
		return []TUIField{boolean("json", "JSON output", false)}
	case "self-update":
		return []TUIField{text("version", "Target version", "latest"), text("release-base-url", "Release base URL", DefaultDeployctlReleaseBaseURL), text("download-url", "Artifact URL", ""), text("sha256", "Expected SHA-256", ""), boolean("yes", "Confirm update", true)}
	case "upgrade":
		return []TUIField{text("runtime-dir", "Runtime directory", "."), text("application-version", "Application version", ""), text("image-registry", "Image registry", ""), text("image-tag", "Image tag", ""), text("release-version", "Release version", ""), boolean("migrate", "Run migration", true)}
	case "uninstall":
		return []TUIField{text("runtime-dir", "Runtime directory", "."), boolean("delete-data", "Delete persistent data", false), text("confirm", "Installation-specific phrase", ""), boolean("yes", "Confirm non-destructive stop", true)}
	case "cluster token create":
		return []TUIField{choice("role", "Node role", "worker", "api", "worker", "web"), text("ttl", "Token lifetime", "10m"), text("runtime-dir", "Runtime directory", ".")}
	case "cluster join":
		return []TUIField{text("server", "Control API URL", "http://127.0.0.1:8080"), {Name: "token", Label: "Single-use token", Kind: tuiFieldText, Value: "pgjoin.v1.placeholder", Sensitive: true}, text("runtime-dir", "Runtime directory", "."), choice("mode", "Mode", "docker", "docker", "native"), text("application-version", "Application version", DefaultApplicationVersion), text("image-registry", "Image registry", ""), text("image-tag", "Image tag", DefaultApplicationVersion), text("release-version", "Release version", ""), text("api-port", "API port", "8080"), text("gateway-port", "Gateway port", "80"), text("user-web-port", "User Web port", "5173"), text("admin-web-port", "Admin Web port", "5174"), text("docs-web-port", "Docs Web port", "5175")}
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

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func removeFlag(args []string, flag string) []string {
	for index := 0; index < len(args); index++ {
		if args[index] == flag && index+1 < len(args) {
			return append(args[:index], args[index+2:]...)
		}
	}
	return args
}
