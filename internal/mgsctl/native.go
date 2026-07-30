package mgsctl

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/app"
	"github.com/fatballfish/pic-gallery/internal/config"
)

type NativePlatform string

const (
	NativePlatformLinux   NativePlatform = "linux"
	NativePlatformWindows NativePlatform = "windows"
)

type NativeAction string

const (
	NativeActionInstall          NativeAction = "install"
	NativeActionUpdate           NativeAction = "update"
	NativeActionRestart          NativeAction = "restart"
	NativeActionReloadSetupToken NativeAction = "reload-setup-token"
	NativeActionStatus           NativeAction = "status"
	NativeActionUninstall        NativeAction = "uninstall"
)

type NativeService struct {
	Name             string
	Component        Component
	Executable       string
	WorkingDirectory string
	Dependencies     []string
	Environment      []string
	RestartExitCode  int
}

type NativeExecutor struct {
	Runner           ProcessRunner
	ReadFile         func(string) ([]byte, error)
	Platform         func() NativePlatform
	CheckPrivileges  func(NativePlatform) error
	InstallRelease   func(context.Context, InstallPlan, NativePlatform) error
	StageMigration   func(context.Context, InstallPlan, NativePlatform) (string, func() error, error)
	WriteServiceFile func(string, []byte) error
}

func (executor NativeExecutor) PrepareUpgrade(ctx context.Context, target *UpgradeTarget) error {
	if target == nil {
		return fmt.Errorf("native upgrade target is required")
	}
	if executor.Platform == nil {
		executor.Platform = currentNativePlatform
	}
	if executor.CheckPrivileges == nil {
		executor.CheckPrivileges = checkNativePrivileges
	}
	if executor.StageMigration == nil {
		executor.StageMigration = StageNativeReleaseMigration
	}
	platform := executor.Platform()
	if err := executor.CheckPrivileges(platform); err != nil {
		return fmt.Errorf("check native service privileges: %w", err)
	}
	executable, cleanup, err := executor.StageMigration(ctx, target.Plan, platform)
	if err != nil {
		return err
	}
	target.NativeMigrationExecutable = executable
	target.Cleanup = cleanup
	return nil
}

func (executor NativeExecutor) MigrateUpgrade(ctx context.Context, target UpgradeTarget, runtimeEnvPath string) error {
	if executor.Runner == nil {
		return fmt.Errorf("native process runner is required")
	}
	executable := strings.TrimSpace(target.NativeMigrationExecutable)
	if executable == "" {
		return fmt.Errorf("prepared native migration executable is required")
	}
	spec := ProcessSpec{
		Executable: executable, Arguments: []string{"--env-file", runtimeEnvPath}, Directory: target.Plan.RuntimeDir,
	}
	if err := executor.Runner.Run(ctx, spec); err != nil {
		return fmt.Errorf("run target native database migration: %w", err)
	}
	return nil
}

func (executor NativeExecutor) Preflight(ctx context.Context, plan InstallPlan) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidateInstallPlan(plan); err != nil {
		return fmt.Errorf("validate native plan: %w", err)
	}
	if plan.Mode != config.DeploymentModeNative {
		return fmt.Errorf("native executor cannot preflight deployment mode %q", plan.Mode)
	}
	if executor.Platform == nil {
		executor.Platform = currentNativePlatform
	}
	if executor.CheckPrivileges == nil {
		executor.CheckPrivileges = checkNativePrivileges
	}
	platform := executor.Platform()
	if platform != NativePlatformLinux && platform != NativePlatformWindows {
		return fmt.Errorf("native deployment is unsupported on platform %q", platform)
	}
	if err := executor.CheckPrivileges(platform); err != nil {
		return fmt.Errorf("check native service privileges: %w", err)
	}
	return nil
}

func (executor NativeExecutor) Run(ctx context.Context, action NativeAction, plan InstallPlan) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if executor.Runner == nil {
		return fmt.Errorf("native process runner is required")
	}
	if executor.ReadFile == nil {
		executor.ReadFile = os.ReadFile
	}
	if executor.Platform == nil {
		executor.Platform = currentNativePlatform
	}
	if executor.CheckPrivileges == nil {
		executor.CheckPrivileges = checkNativePrivileges
	}
	if executor.InstallRelease == nil {
		executor.InstallRelease = InstallNativeRelease
	}
	if executor.WriteServiceFile == nil {
		executor.WriteServiceFile = writeNativeServiceFile
	}
	platform := executor.Platform()
	if platform != NativePlatformLinux && platform != NativePlatformWindows {
		return fmt.Errorf("native deployment is unsupported on platform %q", platform)
	}
	if action != NativeActionStatus {
		if err := executor.CheckPrivileges(platform); err != nil {
			return fmt.Errorf("check native service privileges: %w", err)
		}
	}
	runtimeContent, err := executor.ReadFile(filepath.Join(plan.RuntimeDir, "config", "runtime.env"))
	if err != nil {
		return fmt.Errorf("read native runtime environment: %w", err)
	}
	document, err := config.ParseRuntimeEnv(runtimeContent)
	if err != nil {
		return fmt.Errorf("parse native runtime environment: %w", err)
	}
	installationID := document.Values["INSTALLATION_ID"]
	if action == NativeActionInstall || action == NativeActionUpdate {
		if err := executor.InstallRelease(ctx, plan, platform); err != nil {
			return fmt.Errorf("install native release: %w", err)
		}
		serviceFiles, err := BuildNativeServiceFiles(plan, installationID, platform)
		if err != nil {
			return err
		}
		for _, file := range serviceFiles {
			if err := executor.WriteServiceFile(filepath.Join(plan.RuntimeDir, file.RelativePath), file.Content); err != nil {
				return fmt.Errorf("write native service file: %w", err)
			}
		}
	}
	specs, err := BuildNativeProcessSpecs(action, plan, installationID, platform)
	if err != nil {
		return err
	}
	for _, spec := range specs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := executor.Runner.Run(ctx, spec); err != nil {
			if nativeProcessErrorIsIdempotent(action, platform, spec, err) {
				continue
			}
			return fmt.Errorf("native service %s: %w", nativeSpecOperation(spec), err)
		}
	}
	return nil
}

func BuildNativeServicePlan(plan InstallPlan, installationID string, platform NativePlatform) ([]NativeService, error) {
	if err := ValidateInstallPlan(plan); err != nil {
		return nil, fmt.Errorf("validate native plan: %w", err)
	}
	if plan.Mode != config.DeploymentModeNative {
		return nil, fmt.Errorf("native service plan cannot use deployment mode %q", plan.Mode)
	}
	if platform != NativePlatformLinux && platform != NativePlatformWindows {
		return nil, fmt.Errorf("unsupported native platform %q", platform)
	}
	parsedInstallationID, err := uuid.Parse(installationID)
	if err != nil {
		return nil, fmt.Errorf("validate native installation identity: %w", err)
	}
	runtimeDirectory, err := filepath.Abs(plan.RuntimeDir)
	if err != nil {
		return nil, fmt.Errorf("resolve native runtime directory: %w", err)
	}
	baseName := "app-" + strings.ReplaceAll(parsedInstallationID.String(), "-", "")
	extension := ""
	if platform == NativePlatformWindows {
		extension = ".exe"
	}
	hasAPI := slices.Contains(plan.Components, ComponentAPI)
	services := make([]NativeService, 0, 3)
	for _, component := range []Component{ComponentAPI, ComponentWorker, ComponentGateway} {
		if !slices.Contains(plan.Components, component) {
			continue
		}
		binaryName := "pic-gallery-" + string(component)
		if component == ComponentAPI {
			binaryName = "pic-gallery-api"
		}
		service := NativeService{
			Name: baseName + "-" + string(component), Component: component,
			Executable: filepath.Join(runtimeDirectory, "bin", binaryName+extension), WorkingDirectory: runtimeDirectory,
			RestartExitCode: app.SupervisorRestartExitCode,
		}
		if component != ComponentAPI && hasAPI {
			service.Dependencies = []string{baseName + "-api"}
		}
		services = append(services, service)
	}
	if len(services) == 0 {
		return services, nil
	}
	return services, nil
}

func BuildNativeServiceFiles(plan InstallPlan, installationID string, platform NativePlatform) ([]DeploymentFile, error) {
	services, err := BuildNativeServicePlan(plan, installationID, platform)
	if err != nil {
		return nil, err
	}
	if len(services) == 0 {
		return []DeploymentFile{}, nil
	}
	if platform == NativePlatformWindows {
		return nil, nil
	}
	files := make([]DeploymentFile, 0, len(services))
	for _, service := range services {
		content, err := renderSystemdUnit(service)
		if err != nil {
			return nil, err
		}
		files = append(files, DeploymentFile{
			RelativePath: filepath.Join("services", service.Name+".service"),
			Content:      content,
		})
	}
	return files, nil
}

func BuildNativeProcessSpecs(action NativeAction, plan InstallPlan, installationID string, platform NativePlatform) ([]ProcessSpec, error) {
	if action != NativeActionInstall && action != NativeActionUpdate && action != NativeActionRestart && action != NativeActionReloadSetupToken && action != NativeActionStatus && action != NativeActionUninstall {
		return nil, fmt.Errorf("unsupported native action %q", action)
	}
	services, err := BuildNativeServicePlan(plan, installationID, platform)
	if err != nil {
		return nil, err
	}
	if action == NativeActionReloadSetupToken {
		services = slices.DeleteFunc(services, func(service NativeService) bool {
			return service.Component != ComponentAPI && service.Component != ComponentGateway
		})
	}
	if len(services) == 0 {
		return []ProcessSpec{}, nil
	}
	if platform == NativePlatformLinux {
		return buildSystemdProcessSpecs(action, services), nil
	}
	return buildWindowsServiceProcessSpecs(action, services)
}

func renderSystemdUnit(service NativeService) ([]byte, error) {
	workingDirectory, err := systemdQuote(service.WorkingDirectory, false)
	if err != nil {
		return nil, fmt.Errorf("render %s working directory: %w", service.Name, err)
	}
	executable, err := systemdQuote(service.Executable, true)
	if err != nil {
		return nil, fmt.Errorf("render %s executable: %w", service.Name, err)
	}
	var unit strings.Builder
	unit.WriteString("[Unit]\nDescription=Application " + string(service.Component) + " service\nAfter=network-online.target\nWants=network-online.target\n")
	for _, dependency := range service.Dependencies {
		unit.WriteString("After=" + dependency + ".service\nWants=" + dependency + ".service\n")
	}
	unit.WriteString("\n[Service]\nType=simple\nWorkingDirectory=" + workingDirectory + "\nExecStart=" + executable + "\n")
	unit.WriteString("Restart=on-failure\nRestartForceExitStatus=" + strconv.Itoa(service.RestartExitCode) + "\nRestartSec=3s\nUMask=0077\n")
	unit.WriteString("\n[Install]\nWantedBy=multi-user.target\n")
	return []byte(unit.String()), nil
}

func systemdQuote(value string, escapeDollar bool) (string, error) {
	if strings.ContainsAny(value, "\r\n\x00") {
		return "", fmt.Errorf("systemd value contains a control character")
	}
	replacer := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "%", "%%")
	escaped := replacer.Replace(value)
	if escapeDollar {
		escaped = strings.ReplaceAll(escaped, "$", "$$")
	}
	return `"` + escaped + `"`, nil
}

func buildSystemdProcessSpecs(action NativeAction, services []NativeService) []ProcessSpec {
	if len(services) == 0 {
		return nil
	}
	runtimeDirectory := services[0].WorkingDirectory
	newSpec := func(arguments ...string) ProcessSpec {
		return ProcessSpec{Executable: "systemctl", Arguments: arguments, Directory: runtimeDirectory}
	}
	serviceNames := make([]string, 0, len(services))
	unitPaths := make([]string, 0, len(services))
	for _, service := range services {
		serviceNames = append(serviceNames, service.Name+".service")
		unitPaths = append(unitPaths, filepath.Join(runtimeDirectory, "services", service.Name+".service"))
	}
	switch action {
	case NativeActionInstall:
		specs := []ProcessSpec{newSpec(append([]string{"link", "--force"}, unitPaths...)...), newSpec("daemon-reload")}
		for _, serviceName := range serviceNames {
			specs = append(specs, newSpec("enable", "--now", serviceName))
		}
		for _, serviceName := range serviceNames {
			specs = append(specs, newSpec("is-active", "--quiet", serviceName))
		}
		return specs
	case NativeActionUpdate:
		specs := []ProcessSpec{newSpec(append([]string{"link", "--force"}, unitPaths...)...), newSpec("daemon-reload")}
		for _, serviceName := range serviceNames {
			specs = append(specs, newSpec("restart", serviceName))
		}
		for _, serviceName := range serviceNames {
			specs = append(specs, newSpec("is-active", "--quiet", serviceName))
		}
		return specs
	case NativeActionRestart, NativeActionReloadSetupToken:
		specs := make([]ProcessSpec, 0, len(serviceNames)*2)
		for _, serviceName := range serviceNames {
			specs = append(specs, newSpec("restart", serviceName))
		}
		for _, serviceName := range serviceNames {
			specs = append(specs, newSpec("is-active", "--quiet", serviceName))
		}
		return specs
	case NativeActionStatus:
		return []ProcessSpec{newSpec(append([]string{"status", "--no-pager"}, serviceNames...)...)}
	case NativeActionUninstall:
		specs := make([]ProcessSpec, 0, len(serviceNames)+1)
		for index := len(serviceNames) - 1; index >= 0; index-- {
			specs = append(specs, newSpec("disable", "--now", serviceNames[index]))
		}
		return append(specs, newSpec("daemon-reload"))
	default:
		return nil
	}
}

func buildWindowsServiceProcessSpecs(action NativeAction, services []NativeService) ([]ProcessSpec, error) {
	if len(services) == 0 {
		return []ProcessSpec{}, nil
	}
	runtimeDirectory := services[0].WorkingDirectory
	newSpec := func(arguments ...string) ProcessSpec {
		return ProcessSpec{Executable: "sc.exe", Arguments: arguments, Directory: runtimeDirectory}
	}
	serviceHost := filepath.Join(runtimeDirectory, "bin", "pic-gallery-service-host.exe")
	switch action {
	case NativeActionInstall:
		specs := make([]ProcessSpec, 0, len(services)*5)
		for _, service := range services {
			binaryPath, err := windowsServiceCommand(serviceHost, service, filepath.Join(runtimeDirectory, "logs"))
			if err != nil {
				return nil, err
			}
			settings := []string{"start=", "auto", "binPath=", binaryPath}
			if len(service.Dependencies) > 0 {
				settings = append(settings, "depend=", strings.Join(service.Dependencies, "/"))
			}
			specs = append(specs,
				newSpec(append([]string{"create", service.Name}, settings...)...),
				newSpec(append([]string{"config", service.Name}, settings...)...),
				newSpec("failure", service.Name, "reset=", "0", "actions=", "restart/5000/restart/5000/restart/5000"),
				newSpec("failureflag", service.Name, "1"),
			)
		}
		for _, service := range services {
			specs = append(specs, newSpec("start", service.Name))
		}
		for _, service := range services {
			specs = append(specs, windowsServiceHealthSpec(runtimeDirectory, service.Name))
		}
		return specs, nil
	case NativeActionUpdate:
		specs := make([]ProcessSpec, 0, len(services)*7)
		for _, service := range services {
			binaryPath, err := windowsServiceCommand(serviceHost, service, filepath.Join(runtimeDirectory, "logs"))
			if err != nil {
				return nil, err
			}
			settings := []string{"start=", "auto", "binPath=", binaryPath}
			if len(service.Dependencies) > 0 {
				settings = append(settings, "depend=", strings.Join(service.Dependencies, "/"))
			}
			specs = append(specs,
				newSpec(append([]string{"config", service.Name}, settings...)...),
				newSpec("failure", service.Name, "reset=", "0", "actions=", "restart/5000/restart/5000/restart/5000"),
				newSpec("failureflag", service.Name, "1"),
			)
		}
		for index := len(services) - 1; index >= 0; index-- {
			specs = append(specs, newSpec("stop", services[index].Name))
		}
		for index := len(services) - 1; index >= 0; index-- {
			specs = append(specs, windowsServiceStoppedSpec(runtimeDirectory, services[index].Name, false))
		}
		for _, service := range services {
			specs = append(specs, newSpec("start", service.Name))
		}
		for _, service := range services {
			specs = append(specs, windowsServiceHealthSpec(runtimeDirectory, service.Name))
		}
		return specs, nil
	case NativeActionRestart, NativeActionReloadSetupToken:
		specs := make([]ProcessSpec, 0, len(services)*4)
		for index := len(services) - 1; index >= 0; index-- {
			specs = append(specs, newSpec("stop", services[index].Name))
		}
		for index := len(services) - 1; index >= 0; index-- {
			specs = append(specs, windowsServiceStoppedSpec(runtimeDirectory, services[index].Name, false))
		}
		for _, service := range services {
			specs = append(specs, newSpec("start", service.Name))
		}
		for _, service := range services {
			specs = append(specs, windowsServiceHealthSpec(runtimeDirectory, service.Name))
		}
		return specs, nil
	case NativeActionStatus:
		specs := make([]ProcessSpec, 0, len(services))
		for _, service := range services {
			specs = append(specs, newSpec("query", service.Name))
		}
		return specs, nil
	case NativeActionUninstall:
		specs := make([]ProcessSpec, 0, len(services)*4)
		for index := len(services) - 1; index >= 0; index-- {
			specs = append(specs, newSpec("stop", services[index].Name))
		}
		for index := len(services) - 1; index >= 0; index-- {
			specs = append(specs, windowsServiceStoppedSpec(runtimeDirectory, services[index].Name, true))
		}
		for index := len(services) - 1; index >= 0; index-- {
			specs = append(specs, newSpec("delete", services[index].Name))
		}
		for index := len(services) - 1; index >= 0; index-- {
			specs = append(specs, windowsServiceAbsentSpec(runtimeDirectory, services[index].Name))
		}
		return specs, nil
	default:
		return nil, fmt.Errorf("unsupported native action %q", action)
	}
}

func windowsServiceStoppedSpec(runtimeDirectory, serviceName string, missingIsSuccess bool) ProcessSpec {
	quotedName := strings.ReplaceAll(serviceName, "'", "''")
	missingExit := "exit 2"
	if missingIsSuccess {
		missingExit = "exit 0"
	}
	command := "$deadline=(Get-Date).AddSeconds(20);do{" +
		"$service=Get-Service -Name '" + quotedName + "' -ErrorAction SilentlyContinue;" +
		"if($null -eq $service){" + missingExit + "};" +
		"if($service.Status -eq 'Stopped'){exit 0};Start-Sleep -Milliseconds 250" +
		"}while((Get-Date) -lt $deadline);exit 1"
	return ProcessSpec{
		Executable: "powershell.exe",
		Arguments:  []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", command},
		Directory:  runtimeDirectory,
	}
}

func windowsServiceAbsentSpec(runtimeDirectory, serviceName string) ProcessSpec {
	quotedName := strings.ReplaceAll(serviceName, "'", "''")
	command := "$deadline=(Get-Date).AddSeconds(20);do{" +
		"$service=Get-Service -Name '" + quotedName + "' -ErrorAction SilentlyContinue;" +
		"if($null -eq $service){exit 0};Start-Sleep -Milliseconds 250" +
		"}while((Get-Date) -lt $deadline);exit 1"
	return ProcessSpec{
		Executable: "powershell.exe",
		Arguments:  []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", command},
		Directory:  runtimeDirectory,
	}
}

func windowsServiceHealthSpec(runtimeDirectory, serviceName string) ProcessSpec {
	quotedName := strings.ReplaceAll(serviceName, "'", "''")
	command := "$deadline=(Get-Date).AddSeconds(20);$stable=0;do{" +
		"$service=Get-Service -Name '" + quotedName + "' -ErrorAction SilentlyContinue;" +
		"if($null -ne $service -and $service.Status -eq 'Running'){$stable++}else{$stable=0};" +
		"if($stable -ge 4){exit 0};Start-Sleep -Milliseconds 500" +
		"}while((Get-Date) -lt $deadline);exit 1"
	return ProcessSpec{
		Executable: "powershell.exe",
		Arguments:  []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", command},
		Directory:  runtimeDirectory,
	}
}

func windowsServiceCommand(serviceHost string, service NativeService, logDirectory string) (string, error) {
	values := []string{serviceHost, "--service-name", service.Name, "--working-directory", service.WorkingDirectory, "--executable", service.Executable, "--log-directory", logDirectory, "--restart-exit-code", strconv.Itoa(service.RestartExitCode)}
	quoted := make([]string, len(values))
	for index, value := range values {
		if strings.ContainsAny(value, "\r\n\x00\"") {
			return "", fmt.Errorf("Windows service command value contains an unsupported character")
		}
		quoted[index] = windowsQuoteCommandLineArgument(value)
	}
	return strings.Join(quoted, " "), nil
}

func windowsQuoteCommandLineArgument(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t") {
		return value
	}
	prefix := strings.TrimRight(value, `\`)
	trailingBackslashes := len(value) - len(prefix)
	return `"` + prefix + strings.Repeat(`\`, trailingBackslashes*2) + `"`
}

func writeNativeServiceFile(path string, content []byte) (returnErr error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	if err := secureInstallDirectory(directory); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, installStagePrefix+"service-")
	if err != nil {
		return err
	}
	temporaryPath := file.Name()
	defer func() {
		if file != nil {
			returnErr = errors.Join(returnErr, file.Close())
		}
		if err := os.Remove(temporaryPath); err != nil && !os.IsNotExist(err) {
			returnErr = errors.Join(returnErr, err)
		}
	}()
	if err := secureInstallFile(temporaryPath, file, 0o644); err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	file = nil
	if err := replaceNativeFile(temporaryPath, path); err != nil {
		return err
	}
	return nil
}

func nativeSpecOperation(spec ProcessSpec) string {
	if len(spec.Arguments) == 0 {
		return "command"
	}
	return spec.Arguments[0]
}

func nativeProcessErrorIsIdempotent(action NativeAction, platform NativePlatform, spec ProcessSpec, err error) bool {
	if platform != NativePlatformWindows || len(spec.Arguments) == 0 {
		return false
	}
	var exitError interface{ ExitCode() int }
	if !errors.As(err, &exitError) {
		return false
	}
	verb := spec.Arguments[0]
	code := exitError.ExitCode()
	return action == NativeActionInstall && verb == "create" && code == 1073 ||
		(action == NativeActionInstall || action == NativeActionUpdate || action == NativeActionRestart || action == NativeActionReloadSetupToken) && verb == "start" && code == 1056 ||
		(action == NativeActionUpdate || action == NativeActionRestart || action == NativeActionReloadSetupToken) && verb == "stop" && code == 1062 ||
		action == NativeActionUninstall && verb == "stop" && (code == 1060 || code == 1062 || code == 1072) ||
		action == NativeActionUninstall && verb == "delete" && (code == 1060 || code == 1072)
}
