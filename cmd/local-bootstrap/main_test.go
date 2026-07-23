package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/localbootstrap"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

func TestFormatLocalBootstrapResultEscapesAdministratorControls(t *testing.T) {
	output := formatLocalBootstrapResult(localbootstrap.Result{Binding: setup.SetupBinding{
		InstallationID: "pic-gallery-local",
		AdminEmail:     "admin@example.com\nINJECTED=secret\r",
	}})
	if strings.ContainsAny(output, "\r\n") {
		t.Fatalf("bootstrap output contains raw line controls: %q", output)
	}
	if !strings.Contains(output, `administrator="admin@example.com\nINJECTED=secret\r"`) {
		t.Fatalf("bootstrap output did not safely quote administrator: %q", output)
	}
}

func TestProtectLocalRuntimeFilesUsesPrivateModes(t *testing.T) {
	directory := t.TempDir()
	runtimePath := filepath.Join(directory, "runtime.env")
	statePath := filepath.Join(directory, "install-state.json")
	lockPath := statePath + ".lock"
	for _, path := range []string{runtimePath, statePath, lockPath} {
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := protectLocalRuntimeFiles(runtimePath); err != nil {
		t.Fatalf("protectLocalRuntimeFiles returned error: %v", err)
	}
	for _, path := range []string{runtimePath, statePath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("%s mode = %o, want 600", path, got)
		}
	}
}
