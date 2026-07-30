package mgsctl

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBootstrapWrappersLocateOrDownloadMGSCTLAndForwardArguments(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate wrapper contract test")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
	tests := []struct {
		path      string
		required  []string
		forbidden []string
	}{
		{
			path:      "scripts/install.sh",
			required:  []string{`exec "$MGSCTL_BIN" "$@"`, "command -v mgsctl", "git -C", "version --json", "PATH mgsctl is stale", "MGSCTL_VERSION", "MGSCTL_INSTALL_DIR", "make", "MGSCTL_OUTPUT", "curl", "sha256", "checksum verification failed"},
			forbidden: []string{`exec mgsctl "$@"`, "eval ", "docker compose", "systemctl"},
		},
		{
			path:      "scripts/install.ps1",
			required:  []string{"& $binary @args", "git", "version --json", "ConvertFrom-Json", "PATH mgsctl is stale", "MGSCTL_VERSION", "MGSCTL_INSTALL_DIR", "MGSCTL_OUTPUT", "Invoke-WebRequest", "Get-FileHash", "GetLeftPart", "checksum verification failed"},
			forbidden: []string{"Invoke-Expression", "docker compose", "systemctl"},
		},
	}
	for _, testCase := range tests {
		t.Run(filepath.Base(testCase.path), func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(repositoryRoot, testCase.path))
			if err != nil {
				t.Fatal(err)
			}
			text := string(content)
			for _, fragment := range testCase.required {
				if !strings.Contains(text, fragment) {
					t.Errorf("wrapper missing %q", fragment)
				}
			}
			for _, fragment := range testCase.forbidden {
				if strings.Contains(text, fragment) {
					t.Errorf("wrapper duplicates policy or unsafe execution with %q", fragment)
				}
			}
		})
	}
}
