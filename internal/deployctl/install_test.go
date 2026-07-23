package deployctl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

func TestBuildRuntimeArtifactsGeneratesPortableSecretSafeSetupFiles(t *testing.T) {
	plan, err := BuildInstallPlan(InstallInput{
		Mode: config.DeploymentModeDocker, Profile: config.DeploymentProfileCore, Topology: config.DeploymentTopologySingle,
		Role: config.DeploymentRoleSingle, RuntimeDir: ".", StorageDriver: "local", ApplicationVersion: "v1.2.3",
	})
	if err != nil {
		t.Fatalf("BuildInstallPlan: %v", err)
	}
	artifacts, err := BuildRuntimeArtifacts(plan, bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)), time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("BuildRuntimeArtifacts: %v", err)
	}
	document, err := config.ParseRuntimeEnv(artifacts.RuntimeEnv)
	if err != nil {
		t.Fatalf("parse generated runtime env: %v", err)
	}
	for _, fragment := range []string{"# [中文]", "# [English]", "DEPLOYMENT_MODE=docker", "DEPLOYMENT_MODULES=api,worker,user-web,admin-web,docs-web,gateway"} {
		if !strings.Contains(string(artifacts.RuntimeEnv), fragment) {
			t.Errorf("runtime env missing %q", fragment)
		}
	}
	for _, key := range []string{"SETUP_TOKEN", "AUTH_ACCESS_TOKEN_SECRET", "API_KEY_SIGNING_SECRET_ENCRYPTION_KEY", "CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY", "PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY", "PROMPT_OPTIMIZATION_QUOTE_SIGNING_KEY"} {
		if strings.TrimSpace(document.Values[key]) == "" {
			t.Errorf("generated runtime env missing %s", key)
		}
	}
	if artifacts.SetupToken == "" || artifacts.SetupToken != document.Values["SETUP_TOKEN"] {
		t.Fatal("setup token must be returned once and match the protected runtime env")
	}
	if artifacts.InstallState.Phase != setup.InstallPhasePending || artifacts.InstallState.InstallationID != document.Values["INSTALLATION_ID"] {
		t.Fatalf("pending install state does not bind runtime env: %#v", artifacts.InstallState)
	}
	serializedState, _ := json.Marshal(artifacts.InstallState)
	for _, publicArtifact := range [][]byte{serializedState, artifacts.Manifest} {
		if bytes.Contains(publicArtifact, []byte(artifacts.SetupToken)) || bytes.Contains(bytes.ToLower(publicArtifact), []byte("password")) {
			t.Fatalf("non-secret artifact leaked credentials: %s", publicArtifact)
		}
	}
}

func TestBuildRuntimeArtifactsPrepopulatesManagedDockerFullMiddleware(t *testing.T) {
	plan, err := BuildInstallPlan(InstallInput{
		Mode: config.DeploymentModeDocker, Profile: config.DeploymentProfileFull, Topology: config.DeploymentTopologySingle,
		Role: config.DeploymentRoleSingle, RuntimeDir: "runtime", StorageDriver: "s3", ApplicationVersion: "v1",
	})
	if err != nil {
		t.Fatalf("BuildInstallPlan: %v", err)
	}
	artifacts, err := BuildRuntimeArtifacts(plan, bytes.NewReader(bytes.Repeat([]byte{0x24}, 1024)), time.Now().UTC())
	if err != nil {
		t.Fatalf("BuildRuntimeArtifacts: %v", err)
	}
	document, err := config.ParseRuntimeEnv(artifacts.RuntimeEnv)
	if err != nil {
		t.Fatalf("parse runtime env: %v", err)
	}
	for _, key := range []string{"POSTGRES_MANAGED", "REDIS_MANAGED", "OBJECT_STORAGE_MANAGED"} {
		if document.Values[key] != "true" {
			t.Errorf("%s = %q, want true", key, document.Values[key])
		}
	}
	for _, key := range []string{"POSTGRES_PASSWORD", "REDIS_PASSWORD", "MINIO_ROOT_PASSWORD", "DATABASE_URL", "REDIS_URL", "STORAGE_S3_ENDPOINT", "STORAGE_S3_ACCESS_KEY_ID", "STORAGE_S3_SECRET_ACCESS_KEY"} {
		if document.Values[key] == "" {
			t.Errorf("managed full runtime missing %s", key)
		}
	}
	if document.Values["STORAGE_S3_ACCESS_KEY_ID"] == document.Values["MINIO_ROOT_USER"] || document.Values["STORAGE_S3_SECRET_ACCESS_KEY"] == document.Values["MINIO_ROOT_PASSWORD"] {
		t.Fatal("application object storage credentials must not reuse MinIO root credentials")
	}
	for key, prefix := range map[string]string{
		"POSTGRES_PASSWORD": "pg_", "REDIS_PASSWORD": "redis_", "MINIO_ROOT_PASSWORD": "minio_", "STORAGE_S3_SECRET_ACCESS_KEY": "s3_",
	} {
		if !strings.HasPrefix(document.Values[key], prefix) {
			t.Errorf("managed credential %s must use a CLI-safe prefix %q", key, prefix)
		}
	}
}

func TestBuildRuntimeArtifactsPrepopulatesOnlySelectedDockerCustomMiddleware(t *testing.T) {
	plan, err := BuildInstallPlan(InstallInput{
		Mode: "docker", Profile: "custom", Topology: "single", Role: "single", RuntimeDir: ".",
		StorageDriver: "local", ApplicationVersion: "v1", Components: []Component{ComponentAPI, ComponentPostgres},
	})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := BuildRuntimeArtifacts(plan, bytes.NewReader(bytes.Repeat([]byte{0x31}, 64)), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	document, err := config.ParseRuntimeEnv(artifacts.RuntimeEnv)
	if err != nil {
		t.Fatal(err)
	}
	if document.Values["POSTGRES_MANAGED"] != "true" || document.Values["POSTGRES_PASSWORD"] == "" || document.Values["DATABASE_URL"] == "" {
		t.Fatalf("selected PostgreSQL was not configured: %#v", document.Values)
	}
	if document.Values["REDIS_MANAGED"] != "false" || document.Values["REDIS_PASSWORD"] != "" || document.Values["OBJECT_STORAGE_MANAGED"] != "false" {
		t.Fatalf("unselected middleware was configured: %#v", document.Values)
	}
}

func TestBuildRuntimeArtifactsIncludesSelfContainedDockerAssets(t *testing.T) {
	plan, err := BuildInstallPlan(InstallInput{Mode: "docker", Profile: "core", Topology: "single", Role: "single", RuntimeDir: ".", StorageDriver: "local", ApplicationVersion: "v1", APIPort: "18080"})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := BuildRuntimeArtifacts(plan, bytes.NewReader(bytes.Repeat([]byte{0x39}, 32)), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	wantPaths := []string{"compose.yml", filepath.Join("assets", "nginx-default.conf"), filepath.Join("assets", "minio-init.sh"), filepath.Join("assets", "postgres-init.sh"), filepath.Join("assets", "prometheus.yml")}
	if len(artifacts.DeploymentFiles) != len(wantPaths) {
		t.Fatalf("deployment files = %#v", artifacts.DeploymentFiles)
	}
	for index, file := range artifacts.DeploymentFiles {
		if file.RelativePath != wantPaths[index] || len(file.Content) == 0 {
			t.Errorf("deployment file %d = %#v", index, file)
		}
	}
	if !bytes.Contains(artifacts.DeploymentFiles[1].Content, []byte("server api:18080;")) {
		t.Fatal("materialized Nginx config did not use the configured API port")
	}
	if !bytes.Contains(artifacts.DeploymentFiles[4].Content, []byte("api:18080")) {
		t.Fatal("materialized Prometheus config did not use the configured API port")
	}
}

func TestBuildDeploymentFilesRoutesWebNodesToThePublicAPIBaseURL(t *testing.T) {
	plan, err := BuildInstallPlan(InstallInput{
		Mode: "docker", Profile: "core", Topology: "cluster", Role: "web", RuntimeDir: ".",
		PublicAPIURL: "https://api.example.test:8443/base", ApplicationVersion: "v1", InstallationInitialized: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	files, err := buildDeploymentFiles(plan)
	if err != nil {
		t.Fatal(err)
	}
	nginx := string(files[1].Content)
	for _, required := range []string{
		"server api.example.test:8443;",
		"proxy_pass https://pic_gallery_api/base$request_uri;",
		"proxy_set_header Host api.example.test:8443;",
		"proxy_ssl_server_name on;",
		"proxy_ssl_name api.example.test;",
	} {
		if !strings.Contains(nginx, required) {
			t.Errorf("web-node Nginx config missing %q:\n%s", required, nginx)
		}
	}
	if strings.Contains(nginx, "server api:8080;") {
		t.Fatal("web-node Nginx config still depends on a local API container")
	}
	if got := strings.Count(nginx, "proxy_set_header Host api.example.test:8443;"); got != 8 {
		t.Fatalf("external API Host header count = %d, want 8", got)
	}
	for _, preserved := range []string{
		"proxy_pass http://pic_gallery_docs_web/;\n        proxy_set_header Host $host;",
		"proxy_pass http://pic_gallery_admin_web/;\n        proxy_set_header Host $host;",
		"proxy_pass http://pic_gallery_user_web/;\n        proxy_set_header Host $host;",
	} {
		if !strings.Contains(nginx, preserved) {
			t.Errorf("frontend proxy headers were changed: missing %q", preserved)
		}
	}
}

func TestBuildRuntimeArtifactsPersistsMonitoringPort(t *testing.T) {
	plan, err := BuildInstallPlan(InstallInput{
		Mode: "docker", Profile: "custom", Topology: "single", Role: "single", RuntimeDir: ".",
		StorageDriver: "local", ApplicationVersion: "v1", Components: []Component{ComponentAPI, ComponentMonitoring}, MonitoringPort: "19090",
	})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := BuildRuntimeArtifacts(plan, bytes.NewReader(bytes.Repeat([]byte{0x29}, 64)), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	document, err := config.ParseRuntimeEnv(artifacts.RuntimeEnv)
	if err != nil {
		t.Fatal(err)
	}
	if plan.MonitoringPort != "19090" || document.Values["MONITORING_PORT"] != "19090" {
		t.Fatalf("monitoring port was not preserved: plan=%q env=%q", plan.MonitoringPort, document.Values["MONITORING_PORT"])
	}
}

func TestBuildRuntimeArtifactsRejectsJoinedPlansAndDerivesDistinctSecrets(t *testing.T) {
	joined, err := BuildInstallPlan(InstallInput{
		Mode: config.DeploymentModeNative, Profile: config.DeploymentProfileCore, Topology: config.DeploymentTopologyCluster,
		Role: config.DeploymentRoleWorker, RuntimeDir: ".", StorageDriver: "s3", ApplicationVersion: "v1", InstallationInitialized: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildRuntimeArtifacts(joined, bytes.NewReader(bytes.Repeat([]byte{1}, 32)), time.Now()); err == nil {
		t.Fatal("joined plan generated ordinary pending artifacts")
	}

	authority, err := BuildInstallPlan(InstallInput{Mode: "docker", Profile: "core", Topology: "single", Role: "single", RuntimeDir: ".", StorageDriver: "local", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := BuildRuntimeArtifacts(authority, bytes.NewReader(bytes.Repeat([]byte{1}, 32)), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	document, err := config.ParseRuntimeEnv(artifacts.RuntimeEnv)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, key := range []string{"SETUP_TOKEN", "AUTH_ACCESS_TOKEN_SECRET", "API_KEY_SIGNING_SECRET_ENCRYPTION_KEY", "CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY", "PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY", "PROMPT_OPTIMIZATION_QUOTE_SIGNING_KEY"} {
		value := document.Values[key]
		if seen[value] {
			t.Fatalf("derived secrets are not purpose-separated; duplicate at %s", key)
		}
		seen[value] = true
	}
	result := InstallResult{RuntimeEnvPath: "config/runtime.env", SetupToken: artifacts.SetupToken}
	for _, rendered := range []string{fmt.Sprintf("%#v", artifacts), fmt.Sprintf("%#v", result)} {
		if strings.Contains(rendered, artifacts.SetupToken) {
			t.Fatalf("artifact diagnostic representation leaked setup token: %s", rendered)
		}
	}
}

func TestBuildRuntimeArtifactsRejectsAnUnvalidatedDirectPlanBeforeReadingEntropy(t *testing.T) {
	plan := InstallPlan{
		Mode: "docker", Profile: "core", Topology: "single", Role: "single", RuntimeDir: ".",
		StorageDriver: "local", ApplicationVersion: "v1", APIPort: "8080",
		Components: []Component{ComponentWorker},
	}
	reader := &countingReader{content: bytes.Repeat([]byte{0x22}, 32)}
	if _, err := BuildRuntimeArtifacts(plan, reader, time.Now()); err == nil {
		t.Fatal("invalid direct plan generated runtime artifacts")
	}
	if reader.reads != 0 {
		t.Fatalf("invalid plan consumed entropy %d times", reader.reads)
	}
}

type countingReader struct {
	content []byte
	reads   int
}

func (reader *countingReader) Read(target []byte) (int, error) {
	reader.reads++
	if len(reader.content) == 0 {
		return 0, io.EOF
	}
	count := copy(target, reader.content)
	reader.content = reader.content[count:]
	return count, nil
}

func TestAtomicNoReplaceWriterPreservesAnExistingTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.env")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateFileAtomicNoReplace(path, []byte("replacement")); err == nil {
		t.Fatal("no-replace writer overwrote an existing target")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "original" {
		t.Fatalf("existing target changed to %q", content)
	}
}

func TestExecuteInstallRecoversAValidatedCrashBeforeRuntimeCommit(t *testing.T) {
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	plan, err := BuildInstallPlan(InstallInput{Mode: "docker", Profile: "core", Topology: "single", Role: "single", RuntimeDir: runtimeDirectory, StorageDriver: "local", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	oldArtifacts, err := BuildRuntimeArtifacts(plan, bytes.NewReader(bytes.Repeat([]byte{0x41}, 32)), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	stateContent, err := json.MarshalIndent(oldArtifacts.InstallState, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	configDirectory := filepath.Join(runtimeDirectory, "config")
	if err := os.MkdirAll(configDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDirectory, "install-state.json"), stateContent, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDirectory, "deployment.json"), oldArtifacts.Manifest, 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := ExecuteInstall(context.Background(), plan, InstallDependencies{Entropy: bytes.NewReader(bytes.Repeat([]byte{0x42}, 32))})
	if err != nil {
		t.Fatalf("recover interrupted install: %v", err)
	}
	document, err := config.ParseRuntimeEnv(mustReadFile(t, result.RuntimeEnvPath))
	if err != nil {
		t.Fatal(err)
	}
	if document.Values["INSTALLATION_ID"] == oldArtifacts.InstallState.InstallationID {
		t.Fatal("recovered install reused artifacts from the interrupted transaction")
	}
	if _, err := os.Stat(filepath.Join(configDirectory, ".deployctl-install.lock")); err != nil {
		t.Fatalf("advisory lock file should remain reusable after process exit: %v", err)
	}
}

func TestExistingInstallCleansDeferredStageFilesBeforeReturningCollision(t *testing.T) {
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	plan, err := BuildInstallPlan(InstallInput{Mode: "docker", Profile: "core", Topology: "single", Role: "single", RuntimeDir: runtimeDirectory, StorageDriver: "local", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteInstall(context.Background(), plan, InstallDependencies{Entropy: bytes.NewReader(bytes.Repeat([]byte{0x51}, 32))}); err != nil {
		t.Fatal(err)
	}
	stagePath := filepath.Join(runtimeDirectory, "config", installStagePrefix+"runtime.env-orphan")
	if err := os.WriteFile(stagePath, []byte("deferred secret stage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteInstall(context.Background(), plan, InstallDependencies{Entropy: bytes.NewReader(bytes.Repeat([]byte{0x52}, 32))}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second install error = %v", err)
	}
	if _, err := os.Stat(stagePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deferred stage was not cleaned: %v", err)
	}
}

func TestExecuteInstallResumesTheSamePendingDockerPlanAfterApplyFailure(t *testing.T) {
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	plan, err := BuildInstallPlan(InstallInput{Mode: "docker", Profile: "core", Topology: "single", Role: "single", RuntimeDir: runtimeDirectory, StorageDriver: "local", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	applyCalls := 0
	firstDependencies := InstallDependencies{
		Entropy: bytes.NewReader(bytes.Repeat([]byte{0x61}, 32)),
		ApplyDeployment: func(context.Context, InstallPlan) error {
			applyCalls++
			return errors.New("registry unavailable")
		},
	}
	if _, err := ExecuteInstall(context.Background(), plan, firstDependencies); err == nil || !strings.Contains(err.Error(), "apply deployment") {
		t.Fatalf("first install error = %v", err)
	}
	result, err := ExecuteInstall(context.Background(), plan, InstallDependencies{
		Entropy: errorReader("resume must not regenerate entropy"),
		ApplyDeployment: func(context.Context, InstallPlan) error {
			applyCalls++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("resume pending install: %v", err)
	}
	if applyCalls != 2 || result.SetupToken == "" || result.RuntimeEnvPath != filepath.Join(runtimeDirectory, "config", "runtime.env") {
		t.Fatalf("resume result = %#v, apply calls %d", result, applyCalls)
	}
}

func TestWindowsInstallPermissionsUseAProtectedHandleDACL(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate Windows permissions implementation")
	}
	content := string(mustReadFile(t, filepath.Join(filepath.Dir(sourceFile), "install_permissions_windows.go")))
	for _, required := range []string{"windows.SetSecurityInfo", "windows.PROTECTED_DACL_SECURITY_INFORMATION", "windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT", "windows.WinBuiltinAdministratorsSid", "windows.WinLocalSystemSid"} {
		if !strings.Contains(content, required) {
			t.Errorf("Windows permission implementation missing %q", required)
		}
	}
	nativeRelease := string(mustReadFile(t, filepath.Join(filepath.Dir(sourceFile), "native_release.go")))
	if !strings.Contains(nativeRelease, "secureInstallDirectory(plan.RuntimeDir)") {
		t.Error("native release installer does not secure its runtime root before creating release files")
	}
}

func TestInstallAdvisoryLockIsReleasedWithoutDeletingItsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", ".deployctl-install.lock")
	release, err := acquireInstallLock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	blockedContext, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := acquireInstallLock(blockedContext, path); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("concurrent lock error = %v", err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	releaseAgain, err := acquireInstallLock(context.Background(), path)
	if err != nil {
		t.Fatalf("reacquire stale lock file: %v", err)
	}
	if err := releaseAgain(); err != nil {
		t.Fatal(err)
	}
}

func TestInstallAdvisoryLockRefusesSymlinksWithoutChangingTheTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reparse-point behavior is covered by its platform implementation")
	}
	directory := t.TempDir()
	victim := filepath.Join(directory, "victim")
	if err := os.WriteFile(victim, []byte("do not touch"), 0o644); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(directory, ".deployctl-install.lock")
	if err := os.Symlink(victim, lockPath); err != nil {
		t.Fatal(err)
	}
	if _, err := acquireInstallLock(context.Background(), lockPath); err == nil {
		t.Fatal("advisory lock followed a symlink")
	}
	info, err := os.Stat(victim)
	if err != nil {
		t.Fatal(err)
	}
	if permissions := info.Mode().Perm(); permissions != 0o644 {
		t.Fatalf("victim permissions changed to %#o", permissions)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}
