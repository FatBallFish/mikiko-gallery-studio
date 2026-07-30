package mgsctl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	runtimeconfig "github.com/fatballfish/pic-gallery/internal/config"
)

const (
	UserConfigSchemaVersion = 1
	LanguageChinese         = "zh-CN"
	LanguageEnglish         = "en-US"
)

type UserConfig struct {
	SchemaVersion int    `json:"schema_version"`
	Language      string `json:"language"`
	RuntimeDir    string `json:"runtime_dir,omitempty"`
}

type UserConfigDependencies struct {
	UserConfigDir func() (string, error)
}

var userConfigWriteMu sync.Mutex

func DefaultUserConfig() UserConfig {
	return UserConfig{SchemaVersion: UserConfigSchemaVersion, Language: LanguageChinese}
}

func UserConfigPath(dependencies UserConfigDependencies) (string, error) {
	resolveDirectory := dependencies.UserConfigDir
	if resolveDirectory == nil {
		resolveDirectory = os.UserConfigDir
	}
	root, err := resolveDirectory()
	if err != nil {
		return "", fmt.Errorf("resolve user configuration directory: %w", err)
	}
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("user configuration directory is empty")
	}
	return filepath.Join(root, "mgsctl", "config.json"), nil
}

func LoadUserConfig(dependencies UserConfigDependencies) (UserConfig, error) {
	fallback := DefaultUserConfig()
	path, err := UserConfigPath(dependencies)
	if err != nil {
		return fallback, err
	}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return fallback, nil
	}
	if err != nil {
		return fallback, fmt.Errorf("read user configuration %q: %w", path, err)
	}
	var loaded UserConfig
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&loaded); err != nil {
		return fallback, fmt.Errorf("parse user configuration %q: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fallback, fmt.Errorf("parse user configuration %q: trailing data", path)
	}
	if err := validateUserConfig(loaded); err != nil {
		return fallback, fmt.Errorf("validate user configuration %q: %w", path, err)
	}
	return loaded, nil
}

func LoadPreferredLanguage(dependencies UserConfigDependencies) string {
	loaded, err := LoadUserConfig(dependencies)
	if err != nil {
		return LanguageChinese
	}
	return loaded.Language
}

func SaveUserLanguage(dependencies UserConfigDependencies, language string) error {
	if !supportedLanguage(language) {
		return fmt.Errorf("unsupported language %q", language)
	}
	return updateUserConfig(dependencies, func(current *UserConfig) {
		current.Language = language
	})
}

func SaveRecentRuntime(dependencies UserConfigDependencies, runtimeDir string) error {
	absolute, err := filepath.Abs(strings.TrimSpace(runtimeDir))
	if err != nil {
		return fmt.Errorf("resolve runtime directory: %w", err)
	}
	return updateUserConfig(dependencies, func(current *UserConfig) {
		current.RuntimeDir = filepath.Clean(absolute)
	})
}

func updateUserConfig(dependencies UserConfigDependencies, update func(*UserConfig)) error {
	userConfigWriteMu.Lock()
	defer userConfigWriteMu.Unlock()

	current, err := LoadUserConfig(dependencies)
	if err != nil {
		current = DefaultUserConfig()
	}
	update(&current)
	current.SchemaVersion = UserConfigSchemaVersion
	if err := validateUserConfig(current); err != nil {
		return err
	}
	content, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return fmt.Errorf("encode user configuration: %w", err)
	}
	content = append(content, '\n')
	path, err := UserConfigPath(dependencies)
	if err != nil {
		return err
	}
	if err := runtimeconfig.WriteRuntimeEnvAtomic(path, content); err != nil {
		return fmt.Errorf("write user configuration: %w", err)
	}
	return nil
}

func validateUserConfig(value UserConfig) error {
	if value.SchemaVersion != UserConfigSchemaVersion {
		return fmt.Errorf("unsupported schema version %d", value.SchemaVersion)
	}
	if !supportedLanguage(value.Language) {
		return fmt.Errorf("unsupported language %q", value.Language)
	}
	if value.RuntimeDir != "" && !filepath.IsAbs(value.RuntimeDir) {
		return fmt.Errorf("runtime directory must be absolute")
	}
	return nil
}

func supportedLanguage(language string) bool {
	return language == LanguageChinese || language == LanguageEnglish
}
