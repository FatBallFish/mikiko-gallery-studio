package mgsctl

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

func TestSetupTokenShowAndResetRefuseCompletedInstallation(t *testing.T) {
	runtimeDir := setupOperationsFixture(t, true)
	if _, err := ShowSetupToken(runtimeDir); err == nil || !strings.Contains(err.Error(), "completed") {
		t.Fatalf("ShowSetupToken completed error = %v", err)
	}
	if _, err := ResetSetupToken(context.Background(), runtimeDir, SetupTokenResetDependencies{}); err == nil || !strings.Contains(err.Error(), "completed") {
		t.Fatalf("ResetSetupToken completed error = %v", err)
	}
}

func TestSetupTokenResetRotatesVersionAtomicallyAndRestartsDeployment(t *testing.T) {
	runtimeDir := setupOperationsFixture(t, false)
	oldToken, err := ShowSetupToken(runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	restarts := 0
	newToken, err := ResetSetupToken(context.Background(), runtimeDir, SetupTokenResetDependencies{
		Entropy:           bytes.NewReader(bytes.Repeat([]byte{0x77}, 32)),
		RestartDeployment: func(context.Context, InstallPlan) error { restarts++; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if newToken == "" || newToken == oldToken || restarts != 1 {
		t.Fatalf("reset result token changed=%t restarts=%d", newToken != oldToken, restarts)
	}
	document, err := config.ParseRuntimeEnv(mustReadFile(t, filepath.Join(runtimeDir, "config", "runtime.env")))
	if err != nil {
		t.Fatal(err)
	}
	if document.Values["SETUP_TOKEN"] != newToken || document.Values["SETUP_TOKEN_VERSION"] != "2" {
		t.Fatalf("rotated runtime config = %#v", document.Values)
	}
}

func TestSetupTokenResetRuntimeDependenciesRestartOnlyAPIAndGatewayServices(t *testing.T) {
	const installationID = "019d0000-0000-7000-8000-000000000123"
	t.Run("docker", func(t *testing.T) {
		runtimeDir := setupOperationsFixture(t, false)
		plan, _, err := loadInstallation(runtimeDir)
		if err != nil {
			t.Fatal(err)
		}
		runner := &recordingProcessRunner{}
		dependencies := SetupTokenResetRuntimeDependencies(RuntimeExecutors{Docker: DockerExecutor{
			Runner: runner, RuntimeUser: func() string { return "1000:1000" }, Environment: func() []string { return nil },
		}})
		if err := dependencies.RestartDeployment(t.Context(), plan); err != nil {
			t.Fatal(err)
		}
		if len(runner.specs) != 1 || !strings.HasSuffix(strings.Join(runner.specs[0].Arguments, " "), "restart api gateway") {
			t.Fatalf("Docker setup token restart specs = %#v", runner.specs)
		}
	})

	t.Run("native linux", func(t *testing.T) {
		plan, err := BuildInstallPlan(InstallInput{
			Mode: config.DeploymentModeNative, Profile: config.DeploymentProfileCore, Topology: config.DeploymentTopologySingle,
			Role: config.DeploymentRoleSingle, RuntimeDir: t.TempDir(), StorageDriver: "local", ApplicationVersion: "v1",
		})
		if err != nil {
			t.Fatal(err)
		}
		runner := &recordingProcessRunner{}
		dependencies := SetupTokenResetRuntimeDependencies(RuntimeExecutors{Native: NativeExecutor{
			Runner: runner, Platform: func() NativePlatform { return NativePlatformLinux }, CheckPrivileges: func(NativePlatform) error { return nil },
			ReadFile: func(string) ([]byte, error) { return []byte("INSTALLATION_ID=" + installationID + "\n"), nil },
		}})
		if err := dependencies.RestartDeployment(t.Context(), plan); err != nil {
			t.Fatal(err)
		}
		if len(runner.specs) != 4 {
			t.Fatalf("native setup token restart specs = %#v", runner.specs)
		}
		for _, spec := range runner.specs {
			arguments := strings.Join(spec.Arguments, " ")
			if (!strings.Contains(arguments, "-api.service") && !strings.Contains(arguments, "-gateway.service")) || strings.Contains(arguments, "-worker.service") {
				t.Fatalf("native setup token restart touched a non-API service: %#v", spec)
			}
		}
	})
}

func setupOperationsFixture(t *testing.T, completed bool) string {
	t.Helper()
	runtimeDir := t.TempDir()
	plan, err := BuildInstallPlan(InstallInput{Mode: config.DeploymentModeDocker, Profile: config.DeploymentProfileCore, Topology: config.DeploymentTopologySingle, Role: config.DeploymentRoleSingle, RuntimeDir: runtimeDir, StorageDriver: "local", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := BuildRuntimeArtifacts(plan, bytes.NewReader(bytes.Repeat([]byte{0x44}, 64)), time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if completed {
		document, parseErr := config.ParseRuntimeEnv(artifacts.RuntimeEnv)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		document.Values["SETUP_COMPLETED"] = "true"
		delete(document.Values, "SETUP_TOKEN")
		artifacts.RuntimeEnv, err = config.RenderRuntimeEnv(config.DefaultRuntimeSchema(), document.Values, document.Extensions)
		if err != nil {
			t.Fatal(err)
		}
		digest := strings.Repeat("a", 64)
		artifacts.InstallState.Phase = setup.InstallPhaseCompleted
		artifacts.InstallState.EverCompleted = true
		artifacts.InstallState.Commit = &setup.CommitJournal{OperationID: "setup-op", InstallationID: artifacts.InstallState.InstallationID, RuntimeSchemaVersion: config.CurrentRuntimeSchemaVersion, ConfigRevision: 1, RequestDigest: digest}
	}
	if err := os.MkdirAll(filepath.Join(runtimeDir, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "config", "runtime.env"), artifacts.RuntimeEnv, 0o600); err != nil {
		t.Fatal(err)
	}
	stateContent, _ := json.Marshal(artifacts.InstallState)
	if err := os.WriteFile(filepath.Join(runtimeDir, "config", "install-state.json"), stateContent, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "deployment.json"), artifacts.Manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	return runtimeDir
}
