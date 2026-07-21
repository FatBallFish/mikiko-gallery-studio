package config

import (
	"os"
	"path/filepath"
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

func TestLoadRuntimeCarriesIdentityFromTheSameRuntimeSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.env")
	values := completeRuntimeValuesForTest()
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
