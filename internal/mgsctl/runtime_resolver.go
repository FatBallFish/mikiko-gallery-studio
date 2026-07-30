package mgsctl

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type RuntimeResolutionOptions struct {
	RuntimeDir string
	Explicit   bool
}

type RuntimeResolverDependencies struct {
	WorkingDirectory func() (string, error)
	Stat             func(string) (os.FileInfo, error)
	LoadUserConfig   func() (UserConfig, error)
	UserConfig       UserConfigDependencies
}

func ResolveRuntimeDirectory(options RuntimeResolutionOptions, dependencies RuntimeResolverDependencies) (string, error) {
	workingDirectory := dependencies.WorkingDirectory
	if workingDirectory == nil {
		workingDirectory = os.Getwd
	}
	stat := dependencies.Stat
	if stat == nil {
		stat = os.Stat
	}
	working, err := workingDirectory()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	working, err = filepath.Abs(working)
	if err != nil {
		return "", fmt.Errorf("resolve absolute working directory: %w", err)
	}

	checked := make([]string, 0, 3)
	resolveCandidate := func(directory string) (string, bool, error) {
		if !filepath.IsAbs(directory) {
			directory = filepath.Join(working, directory)
		}
		directory = filepath.Clean(directory)
		manifestPath := filepath.Join(directory, "deployment.json")
		checked = append(checked, manifestPath)
		info, statErr := stat(manifestPath)
		if errors.Is(statErr, os.ErrNotExist) {
			return directory, false, nil
		}
		if statErr != nil {
			return "", false, fmt.Errorf("inspect deployment manifest %q: %w", manifestPath, statErr)
		}
		if !info.Mode().IsRegular() {
			return "", false, fmt.Errorf("deployment manifest is not a regular file: %s", manifestPath)
		}
		return directory, true, nil
	}

	if options.Explicit {
		resolved, exists, resolveErr := resolveCandidate(defaultString(strings.TrimSpace(options.RuntimeDir), "."))
		if resolveErr != nil {
			return "", resolveErr
		}
		if !exists {
			return "", runtimeResolutionError(checked, "explicit runtime directory has no deployment manifest")
		}
		return resolved, nil
	}

	for _, candidate := range []string{working, filepath.Join(working, "runtime")} {
		resolved, exists, resolveErr := resolveCandidate(candidate)
		if resolveErr != nil {
			return "", resolveErr
		}
		if exists {
			return resolved, nil
		}
	}

	loadConfig := dependencies.LoadUserConfig
	if loadConfig == nil {
		loadConfig = func() (UserConfig, error) { return LoadUserConfig(dependencies.UserConfig) }
	}
	userConfig, configErr := loadConfig()
	if configErr == nil && strings.TrimSpace(userConfig.RuntimeDir) != "" {
		resolved, exists, resolveErr := resolveCandidate(userConfig.RuntimeDir)
		if resolveErr != nil {
			return "", resolveErr
		}
		if exists {
			return resolved, nil
		}
		return "", runtimeResolutionError(checked, "saved runtime directory is no longer valid")
	}
	if configErr != nil {
		return "", runtimeResolutionError(checked, "saved runtime configuration could not be read: "+configErr.Error())
	}
	return "", runtimeResolutionError(checked, "no installed runtime was found")
}

func runtimeResolutionError(checked []string, reason string) error {
	return fmt.Errorf("%s; checked %s; specify the installation with --runtime-dir <dir>", reason, strings.Join(checked, ", "))
}
