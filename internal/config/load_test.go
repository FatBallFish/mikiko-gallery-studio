package config

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestDefaultRuntimeEnvPathIsRelativeToWorkingDirectory(t *testing.T) {
	want := filepath.FromSlash("./config/runtime.env")
	if got := DefaultRuntimeEnvPath(); got != want {
		t.Fatalf("DefaultRuntimeEnvPath() = %q, want %q", got, want)
	}

	workingDirectory := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	path := filepath.Join(workingDirectory, "config", "runtime.env")
	writeRuntimeValuesForTest(t, path, map[string]string{"SETUP_COMPLETED": "false", "SETUP_TOKEN": "pending-token"})
	t.Setenv("APP_ENV_FILE", "")

	bootstrap, err := LoadBootstrap("")
	if err != nil {
		t.Fatalf("LoadBootstrap default path returned error: %v", err)
	}
	if bootstrap.Path != want {
		t.Fatalf("bootstrap path = %q, want %q", bootstrap.Path, want)
	}
	if bootstrap.SetupCompleted {
		t.Fatal("pending default runtime file loaded as completed")
	}
}

func TestRuntimeHeartbeatRolesFollowDeploymentModules(t *testing.T) {
	testCases := []struct {
		name    string
		runtime RuntimeConfig
		want    []DeploymentRole
	}{
		{name: "full single modules", runtime: RuntimeConfig{DeploymentRole: DeploymentRoleSingle, DeploymentModules: []string{"api", "worker", "user-web", "gateway"}}, want: []DeploymentRole{DeploymentRoleAPI, DeploymentRoleWorker}},
		{name: "custom api only", runtime: RuntimeConfig{DeploymentRole: DeploymentRoleSingle, DeploymentModules: []string{" API ", "docs-web"}}, want: []DeploymentRole{DeploymentRoleAPI}},
		{name: "single safe default", runtime: RuntimeConfig{DeploymentRole: DeploymentRoleSingle}, want: []DeploymentRole{DeploymentRoleAPI, DeploymentRoleWorker}},
		{name: "distributed worker", runtime: RuntimeConfig{DeploymentRole: DeploymentRoleWorker, DeploymentModules: []string{"worker"}}, want: []DeploymentRole{DeploymentRoleWorker}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := RuntimeHeartbeatRoles(testCase.runtime); !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("RuntimeHeartbeatRoles() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestLoadBootstrapUsesAPPENVFILEOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom.env")
	writeRuntimeValuesForTest(t, path, map[string]string{
		"DEPLOYMENT_MODE": "native",
		"SETUP_COMPLETED": "false",
		"SETUP_TOKEN":     "custom-token",
	})
	t.Setenv("APP_ENV_FILE", path)

	bootstrap, err := LoadBootstrap("")
	if err != nil {
		t.Fatalf("LoadBootstrap APP_ENV_FILE returned error: %v", err)
	}
	if bootstrap.Path != path || bootstrap.Deployment.Mode != DeploymentModeNative || bootstrap.SetupToken != "custom-token" {
		t.Fatalf("unexpected bootstrap config: %#v", bootstrap)
	}
}

func TestLoadBootstrapIgnoresRemovedPicGalleryEnvFile(t *testing.T) {
	workingDirectory := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	defaultPath := filepath.Join(workingDirectory, "config", "runtime.env")
	removedSelectorPath := filepath.Join(workingDirectory, "removed-selector.env")
	writeRuntimeValuesForTest(t, defaultPath, map[string]string{"SETUP_COMPLETED": "false", "SETUP_TOKEN": "default-token"})
	writeRuntimeValuesForTest(t, removedSelectorPath, map[string]string{"SETUP_COMPLETED": "false", "SETUP_TOKEN": "removed-token"})
	t.Setenv("APP_ENV_FILE", "")
	removedSelector := strings.Join([]string{"PIC", "GALLERY", "ENV", "FILE"}, "_")
	t.Setenv(removedSelector, removedSelectorPath)

	bootstrap, err := LoadBootstrap("")
	if err != nil {
		t.Fatalf("LoadBootstrap returned error: %v", err)
	}
	if bootstrap.SetupToken != "default-token" {
		t.Fatalf("removed branded selector affected loading: token = %q", bootstrap.SetupToken)
	}
}

func TestLoadBootstrapAllowsIncompletePendingRuntime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending.env")
	writeRuntimeValuesForTest(t, path, map[string]string{
		"RUNTIME_SCHEMA_VERSION": "1",
		"DEPLOYMENT_MODE":        "docker",
		"DEPLOYMENT_PROFILE":     "core",
		"SETUP_COMPLETED":        "false",
		"DATABASE_URL":           "",
		"REDIS_URL":              "",
		"SETUP_TOKEN":            "pending-token",
	})

	bootstrap, err := LoadBootstrap(path)
	if err != nil {
		t.Fatalf("LoadBootstrap rejected incomplete pending config: %v", err)
	}
	if bootstrap.SetupCompleted || bootstrap.SetupToken != "pending-token" {
		t.Fatalf("unexpected pending bootstrap: %#v", bootstrap)
	}
	if bootstrap.Values["DATABASE_URL"] != "" || bootstrap.Values["REDIS_URL"] != "" {
		t.Fatalf("bootstrap did not preserve incomplete values: %#v", bootstrap.Values)
	}
}

func TestLoadBootstrapReadsSetupTokenVersionFromSameSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending.env")
	writeRuntimeValuesForTest(t, path, map[string]string{
		"SETUP_COMPLETED":     "false",
		"SETUP_TOKEN":         "pending-token",
		"SETUP_TOKEN_VERSION": "7",
	})

	bootstrap, err := LoadBootstrap(path)
	if err != nil {
		t.Fatalf("LoadBootstrap returned error: %v", err)
	}
	if bootstrap.SetupTokenVersion != 7 {
		t.Fatalf("setup token version = %d, want 7", bootstrap.SetupTokenVersion)
	}
	if bootstrap.Values["SETUP_TOKEN_VERSION"] != "7" {
		t.Fatal("typed setup token version did not come from bootstrap Values snapshot")
	}

	for _, invalid := range []string{"0", "-1", "not-a-number", "18446744073709551616"} {
		content := []byte("SETUP_COMPLETED=false\nSETUP_TOKEN=pending-token\nSETUP_TOKEN_VERSION=" + invalid + "\n")
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatalf("write invalid runtime env: %v", err)
		}
		if _, err := LoadBootstrap(path); err == nil || strings.Contains(err.Error(), invalid) {
			t.Errorf("LoadBootstrap(%q) error = %v, want sanitized positive-version error", invalid, err)
		}
	}
}

func TestLoadRuntimeRequiresCompletedSetupAndRequiredFields(t *testing.T) {
	pendingPath := filepath.Join(t.TempDir(), "pending.env")
	writeRuntimeValuesForTest(t, pendingPath, map[string]string{"SETUP_COMPLETED": "false"})
	if _, err := LoadRuntime(pendingPath); err == nil || !strings.Contains(err.Error(), "SETUP_COMPLETED") {
		t.Fatalf("LoadRuntime pending error = %v, want SETUP_COMPLETED diagnostic", err)
	}

	missingRedisPath := filepath.Join(t.TempDir(), "missing-redis.env")
	values := completeRuntimeValuesForTest()
	delete(values, "REDIS_URL")
	writeRuntimeValuesForTest(t, missingRedisPath, values)
	if _, err := LoadRuntime(missingRedisPath); err == nil || !strings.Contains(err.Error(), "REDIS_URL") {
		t.Fatalf("LoadRuntime missing required field error = %v, want REDIS_URL diagnostic", err)
	}
}

func TestRuntimeWorkerConfigurationDefaultsAndOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.env")
	values := completeRuntimeValuesForTest()
	writeRuntimeValuesForTest(t, path, values)

	cfg, err := LoadRuntime(path)
	if err != nil {
		t.Fatalf("LoadRuntime defaults: %v", err)
	}
	wantRoles := []WorkerRole{WorkerRoleImage, WorkerRoleVideo, WorkerRoleMedia, WorkerRoleCleanup}
	if !reflect.DeepEqual(cfg.Worker.Roles, wantRoles) {
		t.Fatalf("default worker roles = %v, want %v", cfg.Worker.Roles, wantRoles)
	}
	if cfg.Worker.ImageConcurrency != 4 || cfg.Worker.VideoConcurrency != 2 || cfg.Worker.MediaConcurrency != 2 || cfg.Worker.CleanupConcurrency != 1 {
		t.Fatalf("default worker concurrency = %#v", cfg.Worker)
	}
	if cfg.Worker.FFmpegPath != "ffmpeg" || cfg.Worker.FFprobePath != "ffprobe" || cfg.Worker.TempDir != "./data/tmp" {
		t.Fatalf("default media tools/temp = %#v", cfg.Worker)
	}
	if cfg.Worker.MetricsAddr != "127.0.0.1:9091" {
		t.Fatalf("default Worker metrics address = %q", cfg.Worker.MetricsAddr)
	}
	if cfg.Worker.TempDiskPausePercent != 75 || cfg.Worker.TempDiskCriticalPercent != 90 {
		t.Fatalf("default disk watermarks = %#v", cfg.Worker)
	}

	values["WORKER_ROLES"] = "video,media"
	values["WORKER_IMAGE_CONCURRENCY"] = "7"
	values["WORKER_VIDEO_CONCURRENCY"] = "3"
	values["WORKER_MEDIA_CONCURRENCY"] = "4"
	values["WORKER_CLEANUP_CONCURRENCY"] = "2"
	values["MEDIA_FFMPEG_PATH"] = "/opt/media/ffmpeg"
	values["MEDIA_FFPROBE_PATH"] = "/opt/media/ffprobe"
	values["MEDIA_TEMP_DIR"] = "/var/lib/pic-gallery/tmp"
	values["MEDIA_TEMP_DISK_PAUSE_PERCENT"] = "70"
	values["MEDIA_TEMP_DISK_CRITICAL_PERCENT"] = "85"
	values["WORKER_METRICS_ADDR"] = ":19091"
	values["PIC_GALLERY_ENV"] = "local"
	values["VIDEO_ARTIFACT_ALLOW_LOOPBACK"] = "true"
	writeRuntimeValuesForTest(t, path, values)

	cfg, err = LoadRuntime(path)
	if err != nil {
		t.Fatalf("LoadRuntime overrides: %v", err)
	}
	if !reflect.DeepEqual(cfg.Worker.Roles, []WorkerRole{WorkerRoleVideo, WorkerRoleMedia}) {
		t.Fatalf("configured worker roles = %v", cfg.Worker.Roles)
	}
	if cfg.Worker.ImageConcurrency != 7 || cfg.Worker.VideoConcurrency != 3 || cfg.Worker.MediaConcurrency != 4 || cfg.Worker.CleanupConcurrency != 2 {
		t.Fatalf("configured worker concurrency = %#v", cfg.Worker)
	}
	if cfg.Worker.FFmpegPath != "/opt/media/ffmpeg" || cfg.Worker.FFprobePath != "/opt/media/ffprobe" || cfg.Worker.TempDir != "/var/lib/pic-gallery/tmp" {
		t.Fatalf("configured media tools/temp = %#v", cfg.Worker)
	}
	if cfg.Worker.MetricsAddr != ":19091" {
		t.Fatalf("configured Worker metrics address = %q", cfg.Worker.MetricsAddr)
	}
	if !cfg.Worker.AllowLoopbackVideoArtifacts {
		t.Fatal("local runtime did not enable the explicit loopback video artifact test option")
	}
}

func TestRuntimeWorkerConfigurationRejectsUnsafeValues(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
		want   string
	}{
		{name: "unknown role", values: map[string]string{"WORKER_ROLES": "image,unknown"}, want: "WORKER_ROLES"},
		{name: "duplicate role", values: map[string]string{"WORKER_ROLES": "video,video"}, want: "WORKER_ROLES"},
		{name: "zero concurrency", values: map[string]string{"WORKER_VIDEO_CONCURRENCY": "0"}, want: "WORKER_VIDEO_CONCURRENCY"},
		{name: "excessive concurrency", values: map[string]string{"WORKER_MEDIA_CONCURRENCY": "65"}, want: "WORKER_MEDIA_CONCURRENCY"},
		{name: "invalid pause watermark", values: map[string]string{"MEDIA_TEMP_DISK_PAUSE_PERCENT": "101"}, want: "MEDIA_TEMP_DISK_PAUSE_PERCENT"},
		{name: "critical below pause", values: map[string]string{"MEDIA_TEMP_DISK_PAUSE_PERCENT": "90", "MEDIA_TEMP_DISK_CRITICAL_PERCENT": "80"}, want: "critical"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "runtime.env")
			values := completeRuntimeValuesForTest()
			writeRuntimeValuesForTest(t, path, values)
			for key, value := range tt.values {
				replaceRuntimeFieldForTest(t, path, key, value)
			}
			_, err := LoadRuntime(path)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.want)) {
				t.Fatalf("LoadRuntime error = %v, want diagnostic containing %q", err, tt.want)
			}
		})
	}
}

func TestRuntimeRejectsLoopbackVideoArtifactsOutsideLocalEnvironments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.env")
	values := completeRuntimeValuesForTest()
	values["PIC_GALLERY_ENV"] = "production"
	values["VIDEO_ARTIFACT_ALLOW_LOOPBACK"] = "true"
	writeRuntimeValuesForTest(t, path, values)
	if _, err := LoadRuntime(path); err == nil || !strings.Contains(err.Error(), "VIDEO_ARTIFACT_ALLOW_LOOPBACK") {
		t.Fatalf("LoadRuntime error = %v, want production loopback artifact rejection", err)
	}
}

func replaceRuntimeFieldForTest(t *testing.T, path, key, value string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read runtime env: %v", err)
	}
	lines := strings.Split(string(content), "\n")
	for index, line := range lines {
		if strings.HasPrefix(line, key+"=") {
			lines[index] = key + "=" + value
			if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
				t.Fatalf("write runtime field %s: %v", key, err)
			}
			return
		}
	}
	t.Fatalf("runtime env field %s not found", key)
}

func TestLoadRuntimeRequiresRetainedSetupTokenVersionOnlyOnAuthorities(t *testing.T) {
	authorityPath := filepath.Join(t.TempDir(), "authority.env")
	authorityValues := completeRuntimeValuesForTest()
	writeRuntimeValuesForTest(t, authorityPath, authorityValues)
	removeRuntimeFieldForTest(t, authorityPath, "SETUP_TOKEN_VERSION")
	if _, err := LoadRuntime(authorityPath); err == nil || !strings.Contains(err.Error(), "SETUP_TOKEN_VERSION") {
		t.Fatalf("completed authority missing token version error = %v", err)
	}

	joinedPath := filepath.Join(t.TempDir(), "joined.env")
	writeRuntimeValuesForTest(t, joinedPath, map[string]string{
		"RUNTIME_SCHEMA_VERSION": "1",
		"DEPLOYMENT_MODE":        "native",
		"DEPLOYMENT_PROFILE":     "core",
		"DEPLOYMENT_TOPOLOGY":    "cluster",
		"DEPLOYMENT_ROLE":        "web",
		"DEPLOYMENT_MODULES":     "user-web,gateway",
		"POSTGRES_MANAGED":       "false",
		"REDIS_MANAGED":          "false",
		"OBJECT_STORAGE_MANAGED": "false",
		"SETUP_COMPLETED":        "true",
		"PUBLIC_API_URL":         "http://api.internal:8080",
		"RELEASE_VERSION":        "test",
		"INSTALLATION_ID":        "installation-test",
		"CLUSTER_NODE_ID":        "web-node-test",
		"CONFIG_REVISION":        "1",
		"APPLICATION_VERSION":    "test",
	})
	removeRuntimeFieldForTest(t, joinedPath, "SETUP_TOKEN_VERSION")
	joined, err := LoadRuntime(joinedPath)
	if err != nil {
		t.Fatalf("joined node without setup token version was rejected: %v", err)
	}
	if joined.Runtime.ClusterNodeID != "web-node-test" {
		t.Fatalf("joined runtime cluster node ID = %q", joined.Runtime.ClusterNodeID)
	}
}

func TestLoadRuntimeRejectsSQLiteDatabaseURL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.env")
	rendered, err := RenderRuntimeEnv(DefaultRuntimeSchema(), completeRuntimeValuesForTest(), nil)
	if err != nil {
		t.Fatalf("render valid runtime env: %v", err)
	}
	lines := strings.Split(string(rendered), "\n")
	replaced := false
	for index, line := range lines {
		if strings.HasPrefix(line, "DATABASE_URL=") {
			lines[index] = "DATABASE_URL=file:smoke.db?cache=shared"
			replaced = true
			break
		}
	}
	if !replaced {
		t.Fatal("rendered runtime env did not contain DATABASE_URL")
	}
	rendered = []byte(strings.Join(lines, "\n"))
	if err := os.WriteFile(path, rendered, 0o600); err != nil {
		t.Fatalf("write runtime env with SQLite URL: %v", err)
	}

	_, err = LoadRuntime(path)
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") || !strings.Contains(err.Error(), "postgres") {
		t.Fatalf("LoadRuntime SQLite URL error = %v, want PostgreSQL-only DATABASE_URL diagnostic", err)
	}
}

func TestLoadRuntimeUsesFileValuesInsteadOfProcessEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.env")
	values := completeRuntimeValuesForTest()
	values["DATABASE_URL"] = "postgres://file-user:file-pass@file-db:5432/app?sslmode=disable"
	values["REDIS_URL"] = "redis://file-cache:6379/2"
	writeRuntimeValuesForTest(t, path, values)
	t.Setenv("DATABASE_URL", "postgres://process-user:process-pass@process-db:5432/app?sslmode=disable")
	t.Setenv("REDIS_URL", "redis://process-cache:6379/9")
	t.Setenv("AUTH_ACCESS_TOKEN_SECRET", "process-secret")

	cfg, err := LoadRuntime(path)
	if err != nil {
		t.Fatalf("LoadRuntime returned error: %v", err)
	}
	if cfg.Database.URL != values["DATABASE_URL"] || cfg.Redis.URL != values["REDIS_URL"] {
		t.Fatalf("process environment overrode runtime file: database=%q redis=%q", cfg.Database.URL, cfg.Redis.URL)
	}
	if cfg.Auth.AccessTokenSecret != values["AUTH_ACCESS_TOKEN_SECRET"] {
		t.Fatalf("process secret overrode runtime file: %q", cfg.Auth.AccessTokenSecret)
	}
}

func TestRuntimeFromBootstrapUsesOneImmutableDocumentSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.env")
	values := completeRuntimeValuesForTest()
	values["CONFIG_REVISION"] = "7"
	values["PUBLIC_API_URL"] = "http://192.0.2.10:8080"
	values["CORS_ALLOWED_ORIGINS"] = "https://user.example.test"
	values["AUTH_ACCESS_TOKEN_SECRET"] = "snapshot-auth-secret"
	values["REDIS_URL"] = "redis://snapshot-cache:6379/0"
	writeRuntimeValuesForTest(t, path, values)
	bootstrap, err := LoadBootstrap(path)
	if err != nil {
		t.Fatalf("LoadBootstrap: %v", err)
	}

	replaced := completeRuntimeValuesForTest()
	replaced["CONFIG_REVISION"] = "7"
	replaced["DATABASE_URL"] = "postgres://replacement:password@replacement-db:5432/app?sslmode=disable"
	replaced["REDIS_URL"] = "redis://replacement-cache:6379/0"
	replaced["STORAGE_LOCAL_ROOT"] = "/replacement/storage"
	replaced["DEPLOYMENT_PROFILE"] = "full"
	replaced["DEPLOYMENT_TOPOLOGY"] = "cluster"
	replaced["POSTGRES_MANAGED"] = "true"
	replaced["PIC_GALLERY_ADDR"] = "127.0.0.1:9999"
	replaced["PUBLIC_API_URL"] = "http://replacement.example.test"
	replaced["CORS_ALLOWED_ORIGINS"] = "https://replacement.example.test"
	replaced["AUTH_ACCESS_TOKEN_SECRET"] = "replacement-auth-secret"
	writeRuntimeValuesForTest(t, path, replaced)

	cfg, err := RuntimeFromBootstrap(bootstrap)
	if err != nil {
		t.Fatalf("RuntimeFromBootstrap: %v", err)
	}
	if cfg.Database.URL != values["DATABASE_URL"] || cfg.Redis.URL != values["REDIS_URL"] || cfg.Storage.LocalRoot != values["STORAGE_LOCAL_ROOT"] {
		t.Fatalf("middleware configuration mixed snapshots: db=%q redis=%q storage=%q", cfg.Database.URL, cfg.Redis.URL, cfg.Storage.LocalRoot)
	}
	if cfg.Auth.AccessTokenSecret != values["AUTH_ACCESS_TOKEN_SECRET"] || bootstrap.Values["PUBLIC_API_URL"] != values["PUBLIC_API_URL"] || bootstrap.Values["CORS_ALLOWED_ORIGINS"] != values["CORS_ALLOWED_ORIGINS"] {
		t.Fatal("listener/public/auth configuration mixed runtime documents")
	}
	if bootstrap.Deployment.Profile != DeploymentProfileCore || bootstrap.Deployment.Topology != DeploymentTopologySingle || bootstrap.PostgresManaged {
		t.Fatalf("deployment ownership mixed runtime documents: %#v", bootstrap)
	}
}

func TestRuntimeIgnoresLegacyPlaintextAdministratorExtensions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.env")
	values := completeRuntimeValuesForTest()
	values["CONFIG_REVISION"] = "1"
	writeRuntimeValuesForTest(t, path, values)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open runtime env: %v", err)
	}
	if _, err := file.WriteString("PIC_GALLERY_ADMIN_EMAIL=legacy@example.test\nPIC_GALLERY_ADMIN_PASSWORD=plaintext-secret\nPIC_GALLERY_ADMIN_ROLE=super_admin\n"); err != nil {
		_ = file.Close()
		t.Fatalf("append legacy admin extensions: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close runtime env: %v", err)
	}
	cfg, err := LoadRuntime(path)
	if err != nil {
		t.Fatalf("LoadRuntime: %v", err)
	}
	if _, exists := reflect.TypeOf(cfg).FieldByName("Admin"); exists {
		t.Fatal("runtime config still exposes the retired plaintext administrator seed")
	}
}

func TestConfigDoesNotExposeObsoleteDocumentationSettings(t *testing.T) {
	if _, exists := reflect.TypeOf(Config{}).FieldByName("Docs"); exists {
		t.Fatal("runtime config must not expose obsolete documentation title/base-path settings")
	}
}

func TestLoadRuntimeCarriesDeploymentDocumentationTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.env")
	values := completeRuntimeValuesForTest()
	values["PUBLIC_API_URL"] = "https://studio.example.test/api"
	values["DEPLOYMENT_MODULES"] = "api,worker,docs-web,gateway"
	values["PIC_GALLERY_DOCS_URL"] = "/developer-docs/"
	values["PIC_GALLERY_DOCS_PROBE_URL"] = "http://gateway/developer-docs/"
	values["GATEWAY_PORT"] = "18000"
	writeRuntimeValuesForTest(t, path, values)

	cfg, err := LoadRuntime(path)
	if err != nil {
		t.Fatalf("LoadRuntime returned error: %v", err)
	}
	if cfg.Runtime.PublicAPIURL != values["PUBLIC_API_URL"] || cfg.Runtime.DocsURL != values["PIC_GALLERY_DOCS_URL"] || cfg.Runtime.DocsProbeURL != values["PIC_GALLERY_DOCS_PROBE_URL"] {
		t.Fatalf("deployment endpoints were not loaded from runtime snapshot: %#v", cfg.Runtime)
	}
	if cfg.Runtime.DeploymentMode != DeploymentModeDocker || !slices.Contains(cfg.Runtime.DeploymentModules, "gateway") || cfg.Runtime.GatewayPort != "18000" {
		t.Fatalf("documentation probe topology was not loaded from runtime snapshot: %#v", cfg.Runtime)
	}
}

func TestLoadRuntimeLeavesProbeUnsetForLegacyRuntime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.env")
	values := completeRuntimeValuesForTest()
	delete(values, "PIC_GALLERY_DOCS_PROBE_URL")
	writeRuntimeValuesForTest(t, path, values)
	cfg, err := LoadRuntime(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Runtime.DocsProbeURL != "" {
		t.Fatalf("legacy runtime received an invented explicit probe URL: %q", cfg.Runtime.DocsProbeURL)
	}
}

func TestLoadRuntimeCarriesIdentityFromTheSameRuntimeSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.env")
	t.Setenv("DEPLOYMENT_ROLE", string(DeploymentRoleWorker))
	values := completeRuntimeValuesForTest()
	values["DEPLOYMENT_ROLE"] = string(DeploymentRoleSingle)
	values["INSTALLATION_ID"] = "snapshot-installation"
	values["APPLICATION_VERSION"] = "snapshot-v1"
	values["CONFIG_REVISION"] = "7"
	writeRuntimeValuesForTest(t, path, values)

	cfg, err := LoadRuntime(path)
	if err != nil {
		t.Fatalf("LoadRuntime returned error: %v", err)
	}
	if cfg.Runtime.Path != path {
		t.Fatalf("runtime path = %q, want %q", cfg.Runtime.Path, path)
	}
	if cfg.Runtime.DeploymentRole != DeploymentRoleSingle {
		t.Fatalf("runtime deployment role = %q, want file role %q", cfg.Runtime.DeploymentRole, DeploymentRoleSingle)
	}
	if cfg.Runtime.InstallationID != "snapshot-installation" {
		t.Fatalf("runtime installation ID = %q", cfg.Runtime.InstallationID)
	}
	if cfg.Runtime.ApplicationVersion != "snapshot-v1" {
		t.Fatalf("runtime app version = %q", cfg.Runtime.ApplicationVersion)
	}
	if cfg.Runtime.ConfigSchemaVersion != CurrentRuntimeSchemaVersion {
		t.Fatalf("runtime config schema version = %d, want %d", cfg.Runtime.ConfigSchemaVersion, CurrentRuntimeSchemaVersion)
	}
	if cfg.Runtime.ConfigRevision != 7 {
		t.Fatalf("runtime config revision = %d, want 7", cfg.Runtime.ConfigRevision)
	}

	values["INSTALLATION_ID"] = "changed-after-load"
	writeRuntimeValuesForTest(t, path, values)
	if cfg.Runtime.InstallationID != "snapshot-installation" {
		t.Fatalf("loaded config changed after runtime file rewrite: %q", cfg.Runtime.InstallationID)
	}
}

func TestLoadRuntimeUsesRoleSpecificRequiredMatrix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web-runtime.env")
	writeRuntimeValuesForTest(t, path, map[string]string{
		"RUNTIME_SCHEMA_VERSION": "1",
		"DEPLOYMENT_MODE":        "native",
		"DEPLOYMENT_PROFILE":     "core",
		"DEPLOYMENT_TOPOLOGY":    "cluster",
		"DEPLOYMENT_ROLE":        "web",
		"DEPLOYMENT_MODULES":     "user-web,admin-web,gateway",
		"POSTGRES_MANAGED":       "false",
		"REDIS_MANAGED":          "false",
		"OBJECT_STORAGE_MANAGED": "false",
		"SETUP_COMPLETED":        "true",
		"PUBLIC_API_URL":         "http://api.internal:8080",
		"RELEASE_VERSION":        "test",
		"INSTALLATION_ID":        "installation-test",
		"CLUSTER_NODE_ID":        "web-node-test",
		"CONFIG_REVISION":        "1",
		"APPLICATION_VERSION":    "test",
	})

	if _, err := LoadRuntime(path); err != nil {
		t.Fatalf("LoadRuntime rejected a valid web-node matrix without database or Redis: %v", err)
	}
}

func TestLoadRuntimeMapsSpecialDotenvValuesAndDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.env")
	values := completeRuntimeValuesForTest()
	values["STORAGE_DRIVER"] = "s3"
	delete(values, "STORAGE_LOCAL_ROOT")
	delete(values, "STORAGE_SHARED_VOLUME")
	values["STORAGE_S3_ENDPOINT"] = "http://minio.internal:9000/path?label=primary#assets"
	values["STORAGE_S3_REGION"] = "us-east-1"
	values["STORAGE_S3_BUCKET"] = "generated-assets"
	values["STORAGE_S3_ACCESS_KEY_ID"] = "access key #1"
	values["STORAGE_S3_SECRET_ACCESS_KEY"] = `secret = value with "quotes"`
	values["STORAGE_S3_FORCE_PATH_STYLE"] = "true"
	values["STORAGE_S3_PREFIX"] = "projects/July 2026 # primary"
	values["DATABASE_CONN_MAX_LIFETIME"] = "45m"
	values["DATABASE_MAX_OPEN_CONNS"] = "30"
	values["DATABASE_MAX_IDLE_CONNS"] = "12"
	writeRuntimeValuesForTest(t, path, values)

	cfg, err := LoadRuntime(path)
	if err != nil {
		t.Fatalf("LoadRuntime returned error: %v", err)
	}
	if cfg.Storage.S3.Endpoint != values["STORAGE_S3_ENDPOINT"] || cfg.Storage.S3.AccessKeyID != values["STORAGE_S3_ACCESS_KEY_ID"] || cfg.Storage.S3.SecretAccessKey != values["STORAGE_S3_SECRET_ACCESS_KEY"] || cfg.Storage.S3.Prefix != values["STORAGE_S3_PREFIX"] {
		t.Fatalf("special dotenv values were not mapped exactly: %#v", cfg.Storage.S3)
	}
	if cfg.Database.MaxOpenConns != 30 || cfg.Database.MaxIdleConns != 12 || cfg.Database.ConnMaxLifetime != 45*time.Minute {
		t.Fatalf("database tuning values were not mapped: %#v", cfg.Database)
	}
	if cfg.App.Addr != ":8080" || cfg.Auth.AccessTokenTTL != 10*time.Minute || cfg.Worker.MaxConcurrentTasks != 4 {
		t.Fatalf("application defaults were not applied: app=%#v auth=%#v worker=%#v", cfg.App, cfg.Auth, cfg.Worker)
	}
}

func TestLoadEnvExplicitPathUsesRuntimeContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "explicit.env")
	values := completeRuntimeValuesForTest()
	values["REDIS_KEY_PREFIX"] = "explicit-prefix"
	writeRuntimeValuesForTest(t, path, values)

	cfg, err := LoadEnv(path)
	if err != nil {
		t.Fatalf("LoadEnv explicit path returned error: %v", err)
	}
	if cfg.Redis.KeyPrefix != "explicit-prefix" {
		t.Fatalf("LoadEnv ignored explicit path: %#v", cfg.Redis)
	}
}

func TestLoadYAMLRemainsExplicit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("app:\n  name: explicit-yaml\n"), 0o600); err != nil {
		t.Fatalf("write YAML: %v", err)
	}
	t.Setenv("APP_ENV_FILE", filepath.Join(t.TempDir(), "unused.env"))

	cfg, err := LoadYAML(path)
	if err != nil {
		t.Fatalf("LoadYAML returned error: %v", err)
	}
	if cfg.App.Name != "explicit-yaml" {
		t.Fatalf("LoadYAML app name = %q", cfg.App.Name)
	}
}

func TestLoadYAMLDefaultsAttachmentPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("app:\n  name: attachment-policy-test\n"), 0o600); err != nil {
		t.Fatalf("write YAML: %v", err)
	}

	cfg, err := LoadYAML(path)
	if err != nil {
		t.Fatalf("LoadYAML: %v", err)
	}
	if cfg.GenerationLimits.ReferenceImageMaxMB != 20 || cfg.AttachmentPolicy.ImageMaxMB != 20 {
		t.Fatalf("expected 20 MB default image policy: generation=%d attachment=%d", cfg.GenerationLimits.ReferenceImageMaxMB, cfg.AttachmentPolicy.ImageMaxMB)
	}
	if !slices.Equal(cfg.AttachmentPolicy.ImageAllowedFormats, []string{"png", "jpeg", "webp", "gif"}) {
		t.Fatalf("unexpected default image formats: %#v", cfg.AttachmentPolicy.ImageAllowedFormats)
	}
}

func TestLocalRuntimeExampleLoadsAsCompletedSharedDevelopmentRuntime(t *testing.T) {
	path := filepath.Join("..", "..", "config", "runtime.local.env.example")
	cfg, err := LoadRuntime(path)
	if err != nil {
		t.Fatalf("LoadRuntime local development example returned error: %v", err)
	}
	if cfg.Runtime.InstallationID != "pic-gallery-local" || cfg.Runtime.DeploymentRole != DeploymentRoleSingle {
		t.Fatalf("local development identity = %q/%q", cfg.Runtime.InstallationID, cfg.Runtime.DeploymentRole)
	}
	if cfg.Database.URL != "postgres://postgres@postgres:5432/pic_gallery?sslmode=disable" {
		t.Fatalf("local development database URL = %q", cfg.Database.URL)
	}
	if !cfg.Auth.DevEmailCodes || cfg.Auth.FixedEmailCode != "123456" {
		t.Fatalf("local development email code settings = enabled:%t code:%q", cfg.Auth.DevEmailCodes, cfg.Auth.FixedEmailCode)
	}
	if !cfg.Cashier.Enabled || !cfg.Cashier.MockEnabled {
		t.Fatalf("local development cashier settings = enabled:%t mock:%t", cfg.Cashier.Enabled, cfg.Cashier.MockEnabled)
	}
	if cfg.Worker.MaxConcurrentTasks != 4 {
		t.Fatalf("local development worker concurrency = %d", cfg.Worker.MaxConcurrentTasks)
	}
}

func TestLoadRuntimeReadsStripeLoopbackAPIBaseURL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.env")
	values := completeRuntimeValuesForTest()
	values["CASHIER_STRIPE_API_BASE_URL"] = "http://127.0.0.1:19090"
	writeRuntimeValuesForTest(t, path, values)
	cfg, err := LoadRuntime(path)
	if err != nil {
		t.Fatalf("LoadRuntime: %v", err)
	}
	if cfg.Cashier.StripeAPIBaseURL != "http://127.0.0.1:19090" {
		t.Fatalf("StripeAPIBaseURL = %q", cfg.Cashier.StripeAPIBaseURL)
	}
}

func TestLoadRuntimeAppliesDefaultCNYPerPoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.env")
	writeRuntimeValuesForTest(t, path, completeRuntimeValuesForTest())
	cfg, err := LoadRuntime(path)
	if err != nil {
		t.Fatalf("LoadRuntime: %v", err)
	}
	if cfg.Billing.CNYPerPoint != "0.3125" {
		t.Fatalf("CNYPerPoint = %q, want default 0.3125", cfg.Billing.CNYPerPoint)
	}
}

func completeRuntimeValuesForTest() map[string]string {
	return map[string]string{
		"RUNTIME_SCHEMA_VERSION":                   "1",
		"DEPLOYMENT_MODE":                          "docker",
		"DEPLOYMENT_PROFILE":                       "core",
		"DEPLOYMENT_TOPOLOGY":                      "single",
		"DEPLOYMENT_ROLE":                          "single",
		"DEPLOYMENT_MODULES":                       "api,worker",
		"POSTGRES_MANAGED":                         "false",
		"REDIS_MANAGED":                            "false",
		"OBJECT_STORAGE_MANAGED":                   "false",
		"SETUP_COMPLETED":                          "true",
		"DATABASE_URL":                             "postgres://app:password@db:5432/app?sslmode=disable",
		"REDIS_URL":                                "redis://cache:6379/0",
		"REDIS_KEY_PREFIX":                         "app-test",
		"STORAGE_DRIVER":                           "local",
		"STORAGE_LOCAL_ROOT":                       "./data/storage",
		"STORAGE_SHARED_VOLUME":                    "true",
		"AUTH_ACCESS_TOKEN_SECRET":                 "access-token-secret",
		"API_KEY_SIGNING_SECRET_ENCRYPTION_KEY":    "api-key-encryption-secret",
		"CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY":   "cashier-encryption-secret",
		"PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY": "secure-config-encryption-secret",
		"PROMPT_OPTIMIZATION_QUOTE_SIGNING_KEY":    "quote-signing-secret",
		"CLUSTER_ENROLLMENT_SEAL_KEY":              "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"API_PORT":                                 "8080",
		"IMAGE_TAG":                                "test",
		"INSTALLATION_ID":                          "installation-test",
		"APPLICATION_VERSION":                      "test",
	}
}

func writeRuntimeValuesForTest(t *testing.T, path string, values map[string]string) {
	t.Helper()
	rendered, err := RenderRuntimeEnv(DefaultRuntimeSchema(), values, nil)
	if err != nil {
		t.Fatalf("render runtime env: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create runtime config directory: %v", err)
	}
	if err := os.WriteFile(path, rendered, 0o600); err != nil {
		t.Fatalf("write runtime env: %v", err)
	}
}

func removeRuntimeFieldForTest(t *testing.T, path, key string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read runtime env: %v", err)
	}
	lines := strings.Split(string(content), "\n")
	filtered := lines[:0]
	for _, line := range lines {
		if !strings.HasPrefix(line, key+"=") {
			filtered = append(filtered, line)
		}
	}
	if err := os.WriteFile(path, []byte(strings.Join(filtered, "\n")), 0o600); err != nil {
		t.Fatalf("write runtime env without %s: %v", key, err)
	}
}
