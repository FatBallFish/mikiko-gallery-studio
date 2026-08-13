package mgsctl

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestDoctorReportsConfigurationPermissionsIdentityMiddlewareAndSchemaWithoutSecrets(t *testing.T) {
	runtimeDir := t.TempDir()
	configDir := filepath.Join(runtimeDir, "config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	const secret = "doctor-must-not-print-this-password"
	env := "RUNTIME_SCHEMA_VERSION=999\nDEPLOYMENT_MODE=docker\nDEPLOYMENT_PROFILE=core\nDEPLOYMENT_TOPOLOGY=single\nDEPLOYMENT_ROLE=single\nDEPLOYMENT_MODULES=api,worker\nPOSTGRES_MANAGED=false\nREDIS_MANAGED=false\nOBJECT_STORAGE_MANAGED=false\nSETUP_COMPLETED=true\nSETUP_TOKEN_VERSION=1\nDATABASE_URL=postgres://app:" + secret + "@db:5432/app\nREDIS_URL=redis://:" + secret + "@redis:6379/0\nREDIS_KEY_PREFIX=app\nSTORAGE_DRIVER=local\nSTORAGE_LOCAL_ROOT=./data\nSTORAGE_SHARED_VOLUME=true\nINSTALLATION_ID=runtime-id\nAPPLICATION_VERSION=v1\nAPI_PORT=8080\nIMAGE_TAG=v1\n"
	envPath := filepath.Join(configDir, "runtime.env")
	if err := os.WriteFile(envPath, []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{"schema_version":1,"installation_id":"manifest-id","created_at":"2026-07-23T00:00:00Z","plan":{}}`)
	if err := os.WriteFile(filepath.Join(runtimeDir, "deployment.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}

	report := Doctor(context.Background(), runtimeDir, DoctorDependencies{
		ProbeMiddleware: func(context.Context, map[string]string) error { return errors.New("database rejected " + secret) },
		CheckSchema:     func(context.Context, map[string]string) error { return errors.New("schema drift near " + secret) },
	})
	rendered := report.String()
	for _, code := range []string{"CONFIG_REQUIRED_FIELD", "CONFIG_PERMISSIONS", "INSTALLATION_MISMATCH", "MIDDLEWARE", "SCHEMA_DRIFT"} {
		if !strings.Contains(rendered, code) {
			t.Errorf("doctor report missing %s: %s", code, rendered)
		}
	}
	if strings.Contains(rendered, secret) || strings.Contains(rendered, "postgres://") || strings.Contains(rendered, "redis://") {
		t.Fatalf("doctor leaked a secret: %s", rendered)
	}
}

func TestDoctorUsesDockerAPIReadinessInsideTheContainerNetwork(t *testing.T) {
	runtimeDir := writeDoctorRuntime(t, "docker", "api,worker")
	readinessCalls := 0
	report := Doctor(t.Context(), runtimeDir, DoctorDependencies{
		CheckRuntimeReadiness: func(context.Context, map[string]string) error {
			readinessCalls++
			return nil
		},
		ProbeMiddleware: func(context.Context, map[string]string) error {
			t.Fatal("Docker doctor attempted host-side middleware probes")
			return nil
		},
		CheckSchema: func(context.Context, map[string]string) error {
			t.Fatal("Docker doctor attempted a host-side schema query")
			return nil
		},
	})
	if readinessCalls != 1 || !doctorCheckOK(report, "MIDDLEWARE") || !doctorCheckOK(report, "SCHEMA_DRIFT") {
		t.Fatalf("Docker readiness report = %#v, calls=%d", report.Checks, readinessCalls)
	}
}

func TestProbeDockerAPIReadinessRequiresReadyResponseOnLoopback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := probeDockerAPIReadiness(t.Context(), map[string]string{"API_PORT": parsed.Port()}); err != nil {
		t.Fatalf("probe Docker API readiness: %v", err)
	}
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	if err := probeDockerAPIReadiness(t.Context(), map[string]string{"API_PORT": parsed.Port()}); err == nil {
		t.Fatal("Docker API readiness accepted HTTP 503")
	}
}

func TestDoctorChecksMediaWorkerToolsAndTemporaryDirectory(t *testing.T) {
	runtimeDir := writeDoctorRuntime(t, "native", "worker")
	envPath := filepath.Join(runtimeDir, "config", "runtime.env")
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	content = append(content, []byte("WORKER_ROLES=image,video,media,cleanup\nMEDIA_FFMPEG_PATH=ffmpeg-custom\nMEDIA_FFPROBE_PATH=ffprobe-custom\nMEDIA_TEMP_DIR=./data/tmp\n")...)
	if err := os.WriteFile(envPath, content, 0o600); err != nil {
		t.Fatal(err)
	}

	report := Doctor(t.Context(), runtimeDir, DoctorDependencies{
		ProbeMiddleware: func(context.Context, map[string]string) error { return nil },
		CheckSchema:     func(context.Context, map[string]string) error { return nil },
		LookPath: func(name string) (string, error) {
			if name == "ffprobe-custom" {
				return "", errors.New("missing")
			}
			return "/usr/bin/" + name, nil
		},
	})
	if !doctorCheckOK(report, "WORKER_TEMP_DIR") || doctorCheckOK(report, "WORKER_MEDIA_TOOLS") {
		t.Fatalf("worker dependency checks = %#v", report.Checks)
	}
	if rendered := report.String(); !strings.Contains(rendered, "ffprobe") || strings.Contains(rendered, runtimeDir) {
		t.Fatalf("worker dependency diagnostic = %s", rendered)
	}
}

func TestDoctorChecksDockerMediaWorkerToolsInsideWorkerContainer(t *testing.T) {
	runtimeDir := writeDoctorRuntime(t, "docker", "api,worker")
	envPath := filepath.Join(runtimeDir, "config", "runtime.env")
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	content = append(content, []byte("WORKER_ROLES=image,video,media,cleanup\nMEDIA_FFMPEG_PATH=ffmpeg-custom\nMEDIA_FFPROBE_PATH=ffprobe-custom\nMEDIA_TEMP_DIR=./data/tmp\n")...)
	if err := os.WriteFile(envPath, content, 0o600); err != nil {
		t.Fatal(err)
	}

	containerChecks := 0
	report := Doctor(t.Context(), runtimeDir, DoctorDependencies{
		CheckRuntimeReadiness: func(context.Context, map[string]string) error { return nil },
		LookPath: func(string) (string, error) {
			t.Fatal("Docker doctor attempted a host-side media tool lookup")
			return "", errors.New("unreachable")
		},
		CheckDockerWorkerMediaTools: func(_ context.Context, gotRuntimeDir string, values map[string]string, ffmpeg, ffprobe string) error {
			containerChecks++
			if gotRuntimeDir != runtimeDir || values["INSTALLATION_ID"] != "runtime-id" {
				t.Fatalf("Docker media check context = %q, %#v", gotRuntimeDir, values)
			}
			if ffmpeg != "ffmpeg-custom" || ffprobe != "ffprobe-custom" {
				t.Fatalf("Docker media tools = %q, %q", ffmpeg, ffprobe)
			}
			return nil
		},
	})
	if containerChecks != 1 || !doctorCheckOK(report, "WORKER_MEDIA_TOOLS") {
		t.Fatalf("Docker worker dependency checks = %#v calls=%d", report.Checks, containerChecks)
	}
}

func TestBuildDockerWorkerMediaToolsCheckSpecUsesRuntimeIdentityAndSafeArguments(t *testing.T) {
	runtimeDir := t.TempDir()
	const installationID = "019d0000-0000-7000-8000-000000000123"
	const nodeID = "019d0000-0000-7000-8000-000000000456"
	const ffmpeg = "ffmpeg; echo unsafe"
	const ffprobe = "/opt/media/ffprobe custom"

	spec, err := BuildDockerWorkerMediaToolsCheckSpec(runtimeDir, installationID, nodeID, ffmpeg, ffprobe, []string{"PATH=/usr/bin", "DATABASE_URL=host-secret"})
	if err != nil {
		t.Fatal(err)
	}
	projectName, err := dockerProjectName(installationID, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	wantPrefix := []string{
		"compose", "--project-directory", runtimeDir,
		"--env-file", filepath.Join(runtimeDir, "config", "runtime.env"),
		"--file", filepath.Join(runtimeDir, "compose.yml"),
		"--project-name", projectName,
		"exec", "--no-TTY", "worker", "sh", "-c",
	}
	if len(spec.Arguments) != len(wantPrefix)+4 || !slices.Equal(spec.Arguments[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("Docker media check arguments = %q", spec.Arguments)
	}
	if spec.Arguments[len(spec.Arguments)-3] != "mgsctl-media-tools" {
		t.Fatalf("Docker media check shell name = %q", spec.Arguments)
	}
	if spec.Arguments[len(spec.Arguments)-2] != ffmpeg || spec.Arguments[len(spec.Arguments)-1] != ffprobe {
		t.Fatalf("media tool paths were not passed as separate arguments: %q", spec.Arguments)
	}
	if strings.Contains(strings.Join(spec.Arguments[:len(spec.Arguments)-2], " "), ffmpeg) || strings.Contains(strings.Join(spec.Arguments[:len(spec.Arguments)-2], " "), ffprobe) {
		t.Fatalf("media tool paths were interpolated into the command: %q", spec.Arguments)
	}
	if strings.Contains(strings.Join(spec.Environment, "\n"), "host-secret") {
		t.Fatalf("Docker media check inherited a runtime secret: %q", spec.Environment)
	}
}

func TestDoctorReportsGenericDockerMediaToolFailureWithoutLeakingDetails(t *testing.T) {
	runtimeDir := writeDoctorRuntime(t, "docker", "api,worker")
	envPath := filepath.Join(runtimeDir, "config", "runtime.env")
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	content = append(content, []byte("WORKER_ROLES=media\nMEDIA_FFMPEG_PATH=/secret/custom-ffmpeg\n")...)
	if err := os.WriteFile(envPath, content, 0o600); err != nil {
		t.Fatal(err)
	}

	report := Doctor(t.Context(), runtimeDir, DoctorDependencies{
		CheckRuntimeReadiness: func(context.Context, map[string]string) error { return nil },
		CheckDockerWorkerMediaTools: func(context.Context, string, map[string]string, string, string) error {
			return errors.New("docker exec failed with DATABASE_URL=postgres://user:password@example/database")
		},
	})
	rendered := report.String()
	if doctorCheckOK(report, "WORKER_MEDIA_TOOLS") {
		t.Fatalf("Docker worker media tools unexpectedly passed: %#v", report.Checks)
	}
	if !strings.Contains(rendered, "missing required media tools: FFmpeg, ffprobe") {
		t.Fatalf("Docker media failure was not generic: %s", rendered)
	}
	for _, secret := range []string{"password", "DATABASE_URL", "/secret/custom-ffmpeg"} {
		if strings.Contains(rendered, secret) {
			t.Fatalf("Docker media failure leaked %q: %s", secret, rendered)
		}
	}
}

func TestDoctorSkipsMediaToolsForVideoOnlyWorker(t *testing.T) {
	runtimeDir := writeDoctorRuntime(t, "native", "worker")
	envPath := filepath.Join(runtimeDir, "config", "runtime.env")
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	content = append(content, []byte("WORKER_ROLES=video\n")...)
	if err := os.WriteFile(envPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	lookups := 0
	report := Doctor(t.Context(), runtimeDir, DoctorDependencies{
		ProbeMiddleware: func(context.Context, map[string]string) error { return nil },
		CheckSchema:     func(context.Context, map[string]string) error { return nil },
		LookPath:        func(string) (string, error) { lookups++; return "", errors.New("missing") },
	})
	if lookups != 0 || !doctorCheckOK(report, "WORKER_MEDIA_TOOLS") {
		t.Fatalf("video-only worker checks = %#v lookups=%d", report.Checks, lookups)
	}
}

func writeDoctorRuntime(t *testing.T, mode, modules string) string {
	t.Helper()
	runtimeDir := t.TempDir()
	configDir := filepath.Join(runtimeDir, "config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	env := "RUNTIME_SCHEMA_VERSION=1\nDEPLOYMENT_MODE=" + mode + "\nDEPLOYMENT_PROFILE=core\nDEPLOYMENT_TOPOLOGY=single\nDEPLOYMENT_ROLE=single\nDEPLOYMENT_MODULES=" + modules + "\nPOSTGRES_MANAGED=false\nREDIS_MANAGED=false\nOBJECT_STORAGE_MANAGED=false\nSETUP_COMPLETED=true\nSETUP_TOKEN_VERSION=1\nDATABASE_URL=postgres://app:secret@db:5432/app\nREDIS_URL=redis://:secret@redis:6379/0\nREDIS_KEY_PREFIX=app\nSTORAGE_DRIVER=local\nSTORAGE_LOCAL_ROOT=./data\nSTORAGE_SHARED_VOLUME=true\nINSTALLATION_ID=runtime-id\nAPPLICATION_VERSION=v1\nAPI_PORT=18080\nIMAGE_TAG=v1\n"
	if err := os.WriteFile(filepath.Join(configDir, "runtime.env"), []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "deployment.json"), []byte(`{"schema_version":1,"installation_id":"runtime-id"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "install-state.json"), []byte(`{"schema_version":1,"installation_id":"runtime-id","deployment_role":"single","phase":"complete","ever_completed":true,"updated_at":"2026-07-23T00:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return runtimeDir
}

func doctorCheckOK(report DoctorReport, code string) bool {
	for _, check := range report.Checks {
		if check.Code == code {
			return check.OK
		}
	}
	return false
}
