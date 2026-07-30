package mgsctl

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseDefaultsUseCurrentRepository(t *testing.T) {
	const want = "https://github.com/fatballfish/mikiko-gallery-studio/releases"
	if DefaultMGSCTLReleaseBaseURL != want {
		t.Fatalf("DefaultMGSCTLReleaseBaseURL = %q, want %q", DefaultMGSCTLReleaseBaseURL, want)
	}
	if defaultNativeReleaseBaseURL != want {
		t.Fatalf("defaultNativeReleaseBaseURL = %q, want %q", defaultNativeReleaseBaseURL, want)
	}
}

func TestMGSCTLArtifactURLMatchesPublishedPlatformNames(t *testing.T) {
	tests := []struct {
		goos, goarch, version, name, path string
	}{
		{"linux", "amd64", "latest", "mgsctl-linux-amd64", "/latest/download/mgsctl-linux-amd64"},
		{"darwin", "arm64", "v1.2.3", "mgsctl-darwin-arm64", "/download/v1.2.3/mgsctl-darwin-arm64"},
		{"windows", "amd64", "v2.0.0", "mgsctl-windows-amd64.exe", "/download/v2.0.0/mgsctl-windows-amd64.exe"},
	}
	for _, testCase := range tests {
		name, downloadURL, err := ResolveMGSCTLArtifact(SelfUpdateOptions{
			Version: testCase.version, ReleaseBaseURL: "https://example.test/releases",
		}, testCase.goos, testCase.goarch)
		if err != nil || name != testCase.name || downloadURL != "https://example.test/releases"+testCase.path {
			t.Errorf("ResolveMGSCTLArtifact(%s/%s %s) = %q, %q, %v", testCase.goos, testCase.goarch, testCase.version, name, downloadURL, err)
		}
	}

	for _, options := range []SelfUpdateOptions{
		{Version: "../secret", ReleaseBaseURL: "https://example.test/releases"},
		{Version: "v1", ReleaseBaseURL: "ftp://example.test/releases"},
		{Version: "v1", ReleaseBaseURL: "https://user:secret@example.test/releases"},
	} {
		if _, _, err := ResolveMGSCTLArtifact(options, "linux", "amd64"); err == nil {
			t.Errorf("ResolveMGSCTLArtifact(%#v) unexpectedly succeeded", options)
		}
	}
}

func TestSelfUpdateDownloadsVerifiesAndStagesReplacement(t *testing.T) {
	newBinary := []byte("new mgsctl binary")
	digest := fmt.Sprintf("%x", sha256.Sum256(newBinary))
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/download/v1.2.3/mgsctl-linux-amd64":
			_, _ = writer.Write(newBinary)
		case "/download/v1.2.3/mgsctl-linux-amd64.sha256":
			fmt.Fprintf(writer, "%s  mgsctl-linux-amd64\n", digest)
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	directory := t.TempDir()
	executable := filepath.Join(directory, "mgsctl")
	if err := os.WriteFile(executable, []byte("old mgsctl binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	replaced := false
	result, err := SelfUpdate(context.Background(), SelfUpdateOptions{
		Version: "v1.2.3", ReleaseBaseURL: server.URL,
	}, SelfUpdateDependencies{
		HTTPClient:     server.Client(),
		ExecutablePath: func() (string, error) { return executable, nil },
		GOOS:           "linux",
		GOARCH:         "amd64",
		Replace: func(current, staged string) (bool, error) {
			replaced = current == executable && filepath.Dir(staged) == directory
			content, readErr := os.ReadFile(staged)
			if readErr != nil {
				return false, readErr
			}
			if string(content) != string(newBinary) {
				return false, fmt.Errorf("staged content = %q", content)
			}
			return false, os.Rename(staged, current)
		},
	})
	if err != nil || !replaced || result.PreviousVersion == "" || result.CurrentVersion != "v1.2.3" || result.Executable != executable || result.Deferred {
		t.Fatalf("SelfUpdate result=%#v replaced=%t err=%v", result, replaced, err)
	}
	content, err := os.ReadFile(executable)
	if err != nil || string(content) != string(newBinary) {
		t.Fatalf("updated executable = %q, %v", content, err)
	}
}

func TestSelfUpdateRejectsChecksumMismatchWithoutReplacingCurrentBinary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasSuffix(request.URL.Path, ".sha256") {
			fmt.Fprintln(writer, strings.Repeat("0", 64))
			return
		}
		fmt.Fprint(writer, "tampered")
	}))
	t.Cleanup(server.Close)

	executable := filepath.Join(t.TempDir(), "mgsctl")
	if err := os.WriteFile(executable, []byte("known-good"), 0o755); err != nil {
		t.Fatal(err)
	}
	replaced := false
	_, err := SelfUpdate(context.Background(), SelfUpdateOptions{Version: "v1", ReleaseBaseURL: server.URL}, SelfUpdateDependencies{
		HTTPClient: server.Client(), ExecutablePath: func() (string, error) { return executable, nil }, GOOS: "linux", GOARCH: "amd64",
		Replace: func(string, string) (bool, error) { replaced = true; return false, nil },
	})
	content, readErr := os.ReadFile(executable)
	if err == nil || !strings.Contains(err.Error(), "checksum verification failed") || replaced || readErr != nil || string(content) != "known-good" {
		t.Fatalf("checksum mismatch err=%v replaced=%t content=%q readErr=%v", err, replaced, content, readErr)
	}
}

func TestSelfUpdateReportsUnavailableReleaseWithoutFallback(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)

	executable := filepath.Join(t.TempDir(), "mgsctl")
	if err := os.WriteFile(executable, []byte("known-good"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := SelfUpdate(context.Background(), SelfUpdateOptions{Version: "v404", ReleaseBaseURL: server.URL}, SelfUpdateDependencies{
		HTTPClient: server.Client(), ExecutablePath: func() (string, error) { return executable, nil }, GOOS: "linux", GOARCH: "amd64",
	})
	if err == nil || !strings.Contains(err.Error(), "release artifact is unavailable") || !strings.Contains(err.Error(), "install.sh") {
		t.Fatalf("unavailable release error = %v", err)
	}
}

func TestSelfUpdatePreservesCancellationWithoutLeakingDownloadQuery(t *testing.T) {
	const secret = "signed-query-secret"
	executable := filepath.Join(t.TempDir(), "mgsctl")
	if err := os.WriteFile(executable, []byte("known-good"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := SelfUpdate(ctx, SelfUpdateOptions{
		Version: "v1", ReleaseBaseURL: "https://example.test/releases",
		DownloadURL:    "https://example.test/mgsctl?token=" + secret,
		ExpectedSHA256: strings.Repeat("0", 64),
	}, SelfUpdateDependencies{
		ExecutablePath: func() (string, error) { return executable, nil }, GOOS: "linux", GOARCH: "amd64",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled self-update error = %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("cancelled self-update leaked signed URL query: %v", err)
	}
}

func TestWindowsSelfUpdateScriptQuotesPathsAndPreservesRecoveryLog(t *testing.T) {
	script := windowsSelfUpdateScript(`C:\Program Files\mgsctl's\mgsctl.exe`, `C:\Program Files\mgsctl's\mgsctl.exe.new`, 1234)
	for _, required := range []string{"Wait-Process", "1234", "Move-Item", "mgsctl''s", ".update-error.log"} {
		if !strings.Contains(script, required) {
			t.Errorf("Windows replacement script missing %q:\n%s", required, script)
		}
	}
}
