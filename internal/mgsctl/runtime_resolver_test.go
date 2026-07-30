package mgsctl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRuntimeDirectoryUsesDocumentedPrecedence(t *testing.T) {
	root := t.TempDir()
	working := filepath.Join(root, "working")
	explicit := filepath.Join(root, "explicit")
	nested := filepath.Join(working, "runtime")
	saved := filepath.Join(root, "saved")
	for _, directory := range []string{working, explicit, nested, saved} {
		writeRuntimeManifestForResolverTest(t, directory)
	}
	dependencies := RuntimeResolverDependencies{
		WorkingDirectory: func() (string, error) { return working, nil },
		LoadUserConfig: func() (UserConfig, error) {
			return UserConfig{SchemaVersion: UserConfigSchemaVersion, Language: LanguageChinese, RuntimeDir: saved}, nil
		},
	}

	resolved, err := ResolveRuntimeDirectory(RuntimeResolutionOptions{Explicit: true, RuntimeDir: explicit}, dependencies)
	if err != nil || resolved != explicit {
		t.Fatalf("explicit resolution = %q, %v", resolved, err)
	}
	resolved, err = ResolveRuntimeDirectory(RuntimeResolutionOptions{}, dependencies)
	if err != nil || resolved != working {
		t.Fatalf("cwd resolution = %q, %v", resolved, err)
	}
	if err := os.Remove(filepath.Join(working, "deployment.json")); err != nil {
		t.Fatal(err)
	}
	resolved, err = ResolveRuntimeDirectory(RuntimeResolutionOptions{}, dependencies)
	if err != nil || resolved != nested {
		t.Fatalf("cwd/runtime resolution = %q, %v", resolved, err)
	}
	if err := os.Remove(filepath.Join(nested, "deployment.json")); err != nil {
		t.Fatal(err)
	}
	resolved, err = ResolveRuntimeDirectory(RuntimeResolutionOptions{}, dependencies)
	if err != nil || resolved != saved {
		t.Fatalf("saved resolution = %q, %v", resolved, err)
	}
}

func TestResolveRuntimeDirectoryDoesNotFallBackFromInvalidExplicitPath(t *testing.T) {
	root := t.TempDir()
	working := filepath.Join(root, "working")
	writeRuntimeManifestForResolverTest(t, working)
	missing := filepath.Join(root, "missing")
	_, err := ResolveRuntimeDirectory(RuntimeResolutionOptions{Explicit: true, RuntimeDir: missing}, RuntimeResolverDependencies{
		WorkingDirectory: func() (string, error) { return working, nil },
	})
	if err == nil || !strings.Contains(err.Error(), missing) || !strings.Contains(err.Error(), "--runtime-dir") {
		t.Fatalf("invalid explicit runtime error = %v", err)
	}
}

func TestResolveRuntimeDirectoryReportsCheckedLocationsAndCorruptSavedConfig(t *testing.T) {
	working := t.TempDir()
	_, err := ResolveRuntimeDirectory(RuntimeResolutionOptions{}, RuntimeResolverDependencies{
		WorkingDirectory: func() (string, error) { return working, nil },
		LoadUserConfig:   func() (UserConfig, error) { return DefaultUserConfig(), os.ErrInvalid },
	})
	if err == nil || !strings.Contains(err.Error(), filepath.Join(working, "deployment.json")) ||
		!strings.Contains(err.Error(), filepath.Join(working, "runtime", "deployment.json")) ||
		!strings.Contains(err.Error(), "saved runtime configuration") || !strings.Contains(err.Error(), "--runtime-dir") {
		t.Fatalf("missing runtime error = %v", err)
	}
}

func writeRuntimeManifestForResolverTest(t *testing.T, directory string) {
	t.Helper()
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "deployment.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
