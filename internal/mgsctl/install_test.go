package mgsctl

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
	if document.Values["PIC_GALLERY_DOCS_URL"] != "/developer-docs/" || document.Values["PIC_GALLERY_DOCS_PROBE_URL"] != "http://gateway/developer-docs/" {
		t.Fatalf("full Docker documentation targets = user %q probe %q", document.Values["PIC_GALLERY_DOCS_URL"], document.Values["PIC_GALLERY_DOCS_PROBE_URL"])
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

func TestBuildRuntimeArtifactsPersistsNativeGatewayProbe(t *testing.T) {
	plan, err := BuildInstallPlan(InstallInput{
		Mode: config.DeploymentModeNative, Profile: config.DeploymentProfileCore, Topology: config.DeploymentTopologySingle,
		Role: config.DeploymentRoleSingle, RuntimeDir: "runtime", StorageDriver: "local", ApplicationVersion: "v1", GatewayPort: "18000",
	})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := BuildRuntimeArtifacts(plan, bytes.NewReader(bytes.Repeat([]byte{0x25}, 128)), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	document, err := config.ParseRuntimeEnv(artifacts.RuntimeEnv)
	if err != nil {
		t.Fatal(err)
	}
	if document.Values["PIC_GALLERY_DOCS_PROBE_URL"] != "http://127.0.0.1:18000/developer-docs/" {
		t.Fatalf("native probe URL = %q", document.Values["PIC_GALLERY_DOCS_PROBE_URL"])
	}
}

func TestBuildRuntimeArtifactsPersistsAbsoluteDocsWithoutInternalProbe(t *testing.T) {
	plan, err := BuildInstallPlan(InstallInput{
		Mode: config.DeploymentModeDocker, Profile: config.DeploymentProfileCore, Topology: config.DeploymentTopologySingle,
		Role: config.DeploymentRoleSingle, RuntimeDir: "runtime", StorageDriver: "local", ApplicationVersion: "v1",
		DocsURL: "https://docs.example.test/reference/",
	})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := BuildRuntimeArtifacts(plan, bytes.NewReader(bytes.Repeat([]byte{0x26}, 128)), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	document, err := config.ParseRuntimeEnv(artifacts.RuntimeEnv)
	if err != nil {
		t.Fatal(err)
	}
	if document.Values["PIC_GALLERY_DOCS_URL"] != plan.DocsURL || document.Values["PIC_GALLERY_DOCS_PROBE_URL"] != "" {
		t.Fatalf("runtime docs = user %q probe %q", document.Values["PIC_GALLERY_DOCS_URL"], document.Values["PIC_GALLERY_DOCS_PROBE_URL"])
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
	if _, err := os.Stat(filepath.Join(configDirectory, ".mgsctl-install.lock")); err != nil {
		t.Fatalf("advisory lock file should remain reusable after process exit: %v", err)
	}
}

func TestExistingPendingInstallCleansDeferredStageFilesAndResumesIdempotently(t *testing.T) {
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
	if _, err := ExecuteInstall(context.Background(), plan, InstallDependencies{Entropy: errorReader("resume must not consume entropy")}); err != nil {
		t.Fatalf("second install error = %v", err)
	}
	if _, err := os.Stat(stagePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deferred stage was not cleaned: %v", err)
	}
}

func TestExecuteInstallRestoresPendingArtifactsWhenOverwritePublicationFails(t *testing.T) {
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	oldPlan, err := BuildInstallPlan(InstallInput{Mode: "docker", Profile: "core", Topology: "single", Role: "single", RuntimeDir: runtimeDirectory, StorageDriver: "local", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteInstall(context.Background(), oldPlan, InstallDependencies{Entropy: bytes.NewReader(bytes.Repeat([]byte{0x58}, 64))}); err != nil {
		t.Fatal(err)
	}
	paths := []string{
		filepath.Join(runtimeDirectory, "config", "runtime.env"),
		filepath.Join(runtimeDirectory, "config", "install-state.json"),
		filepath.Join(runtimeDirectory, "deployment.json"),
		filepath.Join(runtimeDirectory, "compose.yml"),
	}
	before := make(map[string][]byte, len(paths))
	for _, path := range paths {
		before[path] = mustReadFile(t, path)
	}
	newPlan := oldPlan
	newPlan.ApplicationVersion = "v2"
	newPlan.ImageTag = "v2"
	newPlan.APIPort = "19090"
	failure := errors.New("injected deployment publication failure")
	_, err = ExecuteInstall(context.Background(), newPlan, InstallDependencies{
		Entropy:           errorReader("stable pending overwrite must not consume entropy"),
		OverwriteExisting: true,
		ReplaceDeploymentFile: func(string, []byte) error {
			return failure
		},
	})
	if !errors.Is(err, failure) {
		t.Fatalf("overwrite error = %v, want injected failure", err)
	}
	for _, path := range paths {
		if current := mustReadFile(t, path); !bytes.Equal(current, before[path]) {
			t.Errorf("artifact %s was not restored", path)
		}
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

func TestExecuteInstallResumesPendingPlanWhenGeneratedAssetIsMissing(t *testing.T) {
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	plan, err := BuildInstallPlan(InstallInput{Mode: "docker", Profile: "core", Topology: "single", Role: "single", RuntimeDir: runtimeDirectory, StorageDriver: "local", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := ExecuteInstall(context.Background(), plan, InstallDependencies{Entropy: bytes.NewReader(bytes.Repeat([]byte{0x69}, 64))})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(runtimeDirectory, "compose.yml")); err != nil {
		t.Fatal(err)
	}

	applyCalls := 0
	resumed, err := ExecuteInstall(context.Background(), plan, InstallDependencies{
		Entropy: errorReader("resume must not regenerate entropy"),
		ApplyDeployment: func(context.Context, InstallPlan) error {
			applyCalls++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("resume with missing generated asset: %v", err)
	}
	if applyCalls != 1 || resumed.SetupToken != result.SetupToken {
		t.Fatalf("resume result = %#v, apply calls = %d", resumed, applyCalls)
	}
	if _, err := os.Stat(filepath.Join(runtimeDirectory, "compose.yml")); err != nil {
		t.Fatalf("missing generated asset was not rebuilt: %v", err)
	}
}

func TestExecuteInstallRejectsSymlinkedPendingDeploymentAsset(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reparse-point behavior is covered by platform-specific filesystem tests")
	}
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	plan, err := BuildInstallPlan(InstallInput{Mode: "docker", Profile: "core", Topology: "single", Role: "single", RuntimeDir: runtimeDirectory, StorageDriver: "local", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteInstall(context.Background(), plan, InstallDependencies{Entropy: bytes.NewReader(bytes.Repeat([]byte{0x6a}, 64))}); err != nil {
		t.Fatal(err)
	}
	outsidePath := filepath.Join(t.TempDir(), "outside-compose.yml")
	outsideContent := []byte("outside deployment content\n")
	if err := os.WriteFile(outsidePath, outsideContent, 0o600); err != nil {
		t.Fatal(err)
	}
	composePath := filepath.Join(runtimeDirectory, "compose.yml")
	if err := os.Remove(composePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsidePath, composePath); err != nil {
		t.Fatal(err)
	}

	newPlan := plan
	newPlan.APIPort = "19090"
	_, err = ExecuteInstall(context.Background(), newPlan, InstallDependencies{
		Entropy:           errorReader("symlinked installation must not consume entropy"),
		OverwriteExisting: true,
	})
	var collision *InstallTargetExistsError
	if !errors.As(err, &collision) || collision.State != "unrecognized" || collision.Overwritable {
		t.Fatalf("symlinked pending install error = %v", err)
	}
	if content := mustReadFile(t, outsidePath); !bytes.Equal(content, outsideContent) {
		t.Fatalf("symlink target changed: %q", content)
	}
}

func TestExecuteInstallOverwritesOnlyARecognizedPendingInstallAndPreservesData(t *testing.T) {
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	oldPlan, err := BuildInstallPlan(InstallInput{Mode: "docker", Profile: "core", Topology: "single", Role: "single", RuntimeDir: runtimeDirectory, StorageDriver: "local", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	oldResult, err := ExecuteInstall(context.Background(), oldPlan, InstallDependencies{Entropy: bytes.NewReader(bytes.Repeat([]byte{0x71}, 64))})
	if err != nil {
		t.Fatal(err)
	}
	oldRuntime := mustReadFile(t, oldResult.RuntimeEnvPath)
	oldDocument, err := config.ParseRuntimeEnv(oldRuntime)
	if err != nil {
		t.Fatal(err)
	}
	dataMarker := filepath.Join(runtimeDirectory, "data", "keep-me")
	if err := os.WriteFile(dataMarker, []byte("persistent"), 0o600); err != nil {
		t.Fatal(err)
	}

	newPlan := oldPlan
	newPlan.ApplicationVersion = "v2"
	newPlan.ImageTag = "v2"
	newPlan.APIPort = "19090"
	newPlan.GatewayPort = "19080"
	_, err = ExecuteInstall(context.Background(), newPlan, InstallDependencies{Entropy: errorReader("collision must not consume entropy")})
	var collision *InstallTargetExistsError
	if !errors.As(err, &collision) || !collision.Overwritable {
		t.Fatalf("pending collision = %v, want overwritable typed error", err)
	}
	newResult, err := ExecuteInstall(context.Background(), newPlan, InstallDependencies{
		Entropy:           errorReader("existing identity and credentials must not consume entropy"),
		OverwriteExisting: true,
	})
	if err != nil {
		t.Fatalf("overwrite pending install: %v", err)
	}
	if newResult.SetupToken != oldResult.SetupToken {
		t.Fatal("overwrite changed the previous setup token")
	}
	if content, err := os.ReadFile(dataMarker); err != nil || string(content) != "persistent" {
		t.Fatalf("persistent data changed: %q, %v", content, err)
	}
	document, err := config.ParseRuntimeEnv(mustReadFile(t, newResult.RuntimeEnvPath))
	if err != nil || document.Values["APPLICATION_VERSION"] != "v2" || document.Values["IMAGE_TAG"] != "v2" || document.Values["API_PORT"] != "19090" || document.Values["GATEWAY_PORT"] != "19080" {
		t.Fatalf("overwritten runtime = %#v, %v", document.Values, err)
	}
	for _, key := range []string{
		"INSTALLATION_ID", "SETUP_TOKEN", "AUTH_ACCESS_TOKEN_SECRET",
		"API_KEY_SIGNING_SECRET_ENCRYPTION_KEY", "CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY",
		"PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY", "PROMPT_OPTIMIZATION_QUOTE_SIGNING_KEY",
		"CLUSTER_ENROLLMENT_SEAL_KEY",
	} {
		if document.Values[key] != oldDocument.Values[key] {
			t.Errorf("overwrite changed stable runtime value %s", key)
		}
	}
}

func TestExecuteInstallOverwriteNeverUsesManifestRuntimeDirectoryAsADeletionRoot(t *testing.T) {
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	oldPlan, err := BuildInstallPlan(InstallInput{Mode: "docker", Profile: "core", Topology: "single", Role: "single", RuntimeDir: runtimeDirectory, StorageDriver: "local", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteInstall(context.Background(), oldPlan, InstallDependencies{Entropy: bytes.NewReader(bytes.Repeat([]byte{0x75}, 64))}); err != nil {
		t.Fatal(err)
	}
	outsideDirectory := filepath.Join(t.TempDir(), "outside")
	files, err := buildDeploymentFiles(oldPlan)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		destination := filepath.Join(outsideDirectory, file.RelativePath)
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, file.Content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	manifestPath := filepath.Join(runtimeDirectory, "deployment.json")
	var manifest deploymentManifest
	if err := json.Unmarshal(mustReadFile(t, manifestPath), &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Plan.RuntimeDir = outsideDirectory
	manifestContent, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, append(manifestContent, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	newPlan := oldPlan
	newPlan.ApplicationVersion = "v2"
	newPlan.ImageTag = "v2"
	if _, err := ExecuteInstall(context.Background(), newPlan, InstallDependencies{
		Entropy: bytes.NewReader(bytes.Repeat([]byte{0x76}, 64)), OverwriteExisting: true,
	}); err != nil {
		t.Fatalf("safe overwrite: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outsideDirectory, "compose.yml")); err != nil {
		t.Fatalf("overwrite removed a file outside the selected runtime: %v", err)
	}
}

func TestExecuteInstallRefusesToOverwriteACompletedInstall(t *testing.T) {
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	plan, err := BuildInstallPlan(InstallInput{Mode: "docker", Profile: "core", Topology: "single", Role: "single", RuntimeDir: runtimeDirectory, StorageDriver: "local", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := ExecuteInstall(context.Background(), plan, InstallDependencies{Entropy: bytes.NewReader(bytes.Repeat([]byte{0x73}, 64))})
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(runtimeDirectory, "config", "install-state.json")
	var state setup.InstallState
	if err := json.Unmarshal(mustReadFile(t, statePath), &state); err != nil {
		t.Fatal(err)
	}
	state.Phase = setup.InstallPhaseCompleted
	state.EverCompleted = true
	stateContent, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, append(stateContent, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	runtimeContent := strings.Replace(string(mustReadFile(t, result.RuntimeEnvPath)), "SETUP_COMPLETED=false", "SETUP_COMPLETED=true", 1)
	if err := os.WriteFile(result.RuntimeEnvPath, []byte(runtimeContent), 0o600); err != nil {
		t.Fatal(err)
	}

	newPlan := plan
	newPlan.ApplicationVersion = "v2"
	newPlan.ImageTag = "v2"
	_, err = ExecuteInstall(context.Background(), newPlan, InstallDependencies{Entropy: errorReader("completed collision must not consume entropy"), OverwriteExisting: true})
	var collision *InstallTargetExistsError
	if !errors.As(err, &collision) || collision.Overwritable || !strings.Contains(err.Error(), "completed") {
		t.Fatalf("completed collision = %v", err)
	}
	if _, statErr := os.Stat(result.RuntimeEnvPath); statErr != nil {
		t.Fatalf("completed runtime was removed: %v", statErr)
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
	path := filepath.Join(t.TempDir(), "config", ".mgsctl-install.lock")
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
	lockPath := filepath.Join(directory, ".mgsctl-install.lock")
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
