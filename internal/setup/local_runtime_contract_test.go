package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
)

func TestLocalRuntimeCompletedStateUsesCanonicalSetupDigest(t *testing.T) {
	root := filepath.Join("..", "..", "config")
	bootstrap, err := config.LoadBootstrap(filepath.Join(root, "runtime.local.env.example"))
	if err != nil {
		t.Fatalf("load local runtime bootstrap: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(root, "install-state.local.json.example"))
	if err != nil {
		t.Fatalf("read local install-state: %v", err)
	}
	var state InstallState
	if err := json.Unmarshal(content, &state); err != nil {
		t.Fatalf("decode local install-state: %v", err)
	}
	if err := state.Validate(); err != nil {
		t.Fatalf("validate local install-state: %v", err)
	}
	if state.InstallationID != bootstrap.InstallationID || state.Commit == nil {
		t.Fatalf("local runtime/install-state identity is inconsistent")
	}
	want, err := setupRequestDigest(bootstrap.Values, "admin@example.com")
	if err != nil {
		t.Fatalf("calculate local setup digest: %v", err)
	}
	if state.Commit.RequestDigest != want {
		t.Fatalf("local setup digest = %q, want %q", state.Commit.RequestDigest, want)
	}
}
