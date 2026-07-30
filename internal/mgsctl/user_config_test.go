package mgsctl

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUserConfigDefaultsToChineseAndRoundTripsPreferences(t *testing.T) {
	configRoot := t.TempDir()
	dependencies := UserConfigDependencies{UserConfigDir: func() (string, error) { return configRoot, nil }}

	loaded, err := LoadUserConfig(dependencies)
	if err != nil {
		t.Fatalf("LoadUserConfig(missing): %v", err)
	}
	if loaded.SchemaVersion != UserConfigSchemaVersion || loaded.Language != LanguageChinese || loaded.RuntimeDir != "" {
		t.Fatalf("default user config = %#v", loaded)
	}

	runtimeDir := filepath.Join(t.TempDir(), "runtime")
	if err := SaveUserLanguage(dependencies, LanguageEnglish); err != nil {
		t.Fatalf("SaveUserLanguage: %v", err)
	}
	if err := SaveRecentRuntime(dependencies, runtimeDir); err != nil {
		t.Fatalf("SaveRecentRuntime: %v", err)
	}
	loaded, err = LoadUserConfig(dependencies)
	if err != nil {
		t.Fatalf("LoadUserConfig(saved): %v", err)
	}
	absoluteRuntime, _ := filepath.Abs(runtimeDir)
	if loaded.Language != LanguageEnglish || loaded.RuntimeDir != absoluteRuntime {
		t.Fatalf("saved user config = %#v", loaded)
	}
	path, err := UserConfigPath(dependencies)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("user config permissions = %o", info.Mode().Perm())
	}
}

func TestUserConfigCorruptionFallsBackToChineseWithoutHidingRuntimeError(t *testing.T) {
	configRoot := t.TempDir()
	dependencies := UserConfigDependencies{UserConfigDir: func() (string, error) { return configRoot, nil }}
	path, err := UserConfigPath(dependencies)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"language":`), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadUserConfig(dependencies)
	if err == nil || loaded.Language != LanguageChinese || loaded.RuntimeDir != "" {
		t.Fatalf("corrupt config = %#v, %v", loaded, err)
	}
	if language := LoadPreferredLanguage(dependencies); language != LanguageChinese {
		t.Fatalf("preferred language = %q", language)
	}
}

func TestSaveUserLanguageRejectsUnsupportedLocale(t *testing.T) {
	dependencies := UserConfigDependencies{UserConfigDir: func() (string, error) { return t.TempDir(), nil }}
	if err := SaveUserLanguage(dependencies, "fr-FR"); err == nil {
		t.Fatal("unsupported language was accepted")
	}
}
