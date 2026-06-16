package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadUsesEnvByDefault(t *testing.T) {
	t.Setenv("PIC_GALLERY_ENV", "production")
	t.Setenv("PIC_GALLERY_ADDR", ":9090")
	t.Setenv("DATABASE_URL", "postgres://pic_gallery:secret@db:5432/pic_gallery?sslmode=disable")
	t.Setenv("REDIS_URL", "redis://localhost:6379/1")
	t.Setenv("REDIS_KEY_PREFIX", "test-prefix")
	t.Setenv("STORAGE_DRIVER", "s3")
	t.Setenv("STORAGE_LOCAL_ROOT", "/var/lib/override")
	t.Setenv("API_KEY_SIGNING_SECRET_ENCRYPTION_KEY", "api-key-secret-test-key")
	t.Setenv("CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY", "cashier-provider-config-test-key")
	t.Setenv("PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY", "secure-config-test-key")
	t.Setenv("WORKER_MAX_CONCURRENT_TASKS", "12")
	t.Setenv("PIC_GALLERY_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("PIC_GALLERY_ADMIN_PASSWORD", "admin-password")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.App.Env != "production" {
		t.Fatalf("expected app env from env, got %q", cfg.App.Env)
	}
	if cfg.App.Addr != ":9090" {
		t.Fatalf("expected app addr from env, got %q", cfg.App.Addr)
	}
	if cfg.Database.URL != "postgres://pic_gallery:secret@db:5432/pic_gallery?sslmode=disable" {
		t.Fatalf("expected database URL from env, got %q", cfg.Database.URL)
	}
	if cfg.Redis.URL != "redis://localhost:6379/1" || cfg.Redis.KeyPrefix != "test-prefix" {
		t.Fatalf("expected Redis values from env, got %#v", cfg.Redis)
	}
	if cfg.Storage.Driver != "s3" || cfg.Storage.LocalRoot != "/var/lib/override" {
		t.Fatalf("expected storage values from env, got %#v", cfg.Storage)
	}
	if cfg.Billing.PointsScale != 5 {
		t.Fatalf("expected billing scale 5, got %d", cfg.Billing.PointsScale)
	}
	if cfg.Billing.QualityPointsByModel["basic"]["1k"] != "2.00000" {
		t.Fatalf("expected default basic 1k pricing, got %#v", cfg.Billing.QualityPointsByModel)
	}
	if cfg.Cashier.MaxPendingOrdersPerUser != 3 || cfg.Cashier.OrderTimeoutSeconds != 1800 {
		t.Fatalf("expected cashier defaults from config, got %#v", cfg.Cashier)
	}
	if cfg.APIKey.SigningSecretEncryptionKey != "api-key-secret-test-key" {
		t.Fatalf("expected API key signing secret env value, got %q", cfg.APIKey.SigningSecretEncryptionKey)
	}
	if cfg.Cashier.ProviderConfigEncryptionKey != "cashier-provider-config-test-key" {
		t.Fatalf("expected cashier provider config env value, got %q", cfg.Cashier.ProviderConfigEncryptionKey)
	}
	if cfg.Security.SecureConfigEncryptionKey != "secure-config-test-key" {
		t.Fatalf("expected secure config env value, got %q", cfg.Security.SecureConfigEncryptionKey)
	}
	if cfg.Worker.MaxConcurrentTasks != 12 {
		t.Fatalf("expected worker max concurrency from env, got %d", cfg.Worker.MaxConcurrentTasks)
	}
	if cfg.Admin.SeedEmail != "admin@example.com" || cfg.Admin.SeedPassword != "admin-password" {
		t.Fatalf("expected admin bootstrap from env, got %#v", cfg.Admin)
	}
}

func TestLoadYAMLUsesPicGalleryConfigEnv(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("app:\n  name: from-pic-gallery-config\n"), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	t.Setenv("APP_CONFIG_PATH", "")
	t.Setenv("PIC_GALLERY_CONFIG", configPath)

	cfg, err := LoadYAML("")
	if err != nil {
		t.Fatalf("LoadYAML returned error: %v", err)
	}
	if cfg.App.Name != "from-pic-gallery-config" {
		t.Fatalf("expected PIC_GALLERY_CONFIG path to load config, got app name %q", cfg.App.Name)
	}
}

func TestLoadIgnoresLegacyYAMLSelectorsByDefault(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("app:\n  name: legacy-yaml\n"), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	t.Setenv("PIC_GALLERY_CONFIG", configPath)
	t.Setenv("PIC_GALLERY_ENV", "production")

	_, err := Load("")
	if err == nil {
		t.Fatal("expected default Load to ignore legacy YAML selector and require env DATABASE_URL")
	}
	if got := err.Error(); got != "DATABASE_URL must be configured in production env" {
		t.Fatalf("unexpected Load error %q", got)
	}
}

func TestLoadEnvIgnoresObjectStorageSecretEnv(t *testing.T) {
	t.Setenv("PIC_GALLERY_ENV", "local")
	t.Setenv("STORAGE_DRIVER", "local")
	t.Setenv("STORAGE_S3_ENDPOINT", "https://s3.example.com")
	t.Setenv("STORAGE_S3_REGION", "cn-test-1")
	t.Setenv("STORAGE_S3_BUCKET", "pic-gallery-assets")
	t.Setenv("STORAGE_S3_ACCESS_KEY_ID", "s3-ak")
	t.Setenv("STORAGE_S3_SECRET_ACCESS_KEY", "s3-sk")
	t.Setenv("STORAGE_S3_FORCE_PATH_STYLE", "true")
	t.Setenv("STORAGE_S3_PREFIX", "prod/pic-gallery")
	t.Setenv("BFSS_ENDPOINT", "https://bfss.example.com")
	t.Setenv("BFSS_REGION", "cn-test-1")
	t.Setenv("BFSS_BUCKET", "pic-gallery-assets")
	t.Setenv("BFSS_ACCESS_KEY_ID", "bfss-ak")
	t.Setenv("BFSS_SECRET_ACCESS_KEY", "bfss-sk")
	t.Setenv("BFSS_FORCE_PATH_STYLE", "false")
	t.Setenv("BFSS_PREFIX", "prod/pic-gallery")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Storage.S3.Endpoint != "" ||
		cfg.Storage.S3.Region != "" ||
		cfg.Storage.S3.Bucket != "" ||
		cfg.Storage.S3.AccessKeyID != "" ||
		cfg.Storage.S3.SecretAccessKey != "" ||
		cfg.Storage.S3.ForcePathStyle ||
		cfg.Storage.S3.Prefix != "" {
		t.Fatalf("expected object storage secret env vars to be ignored, got %#v", cfg.Storage.S3)
	}
}

func TestLoadYAMLIgnoresBFSSStorageEnvAliases(t *testing.T) {
	t.Setenv("BFSS_ENDPOINT", "https://bfss.example.com")
	t.Setenv("BFSS_REGION", "cn-test-1")
	t.Setenv("BFSS_BUCKET", "pic-gallery-assets")
	t.Setenv("BFSS_ACCESS_KEY_ID", "bfss-ak")
	t.Setenv("BFSS_SECRET_ACCESS_KEY", "bfss-sk")
	t.Setenv("BFSS_FORCE_PATH_STYLE", "false")
	t.Setenv("BFSS_PREFIX", "prod/pic-gallery")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`storage:
  s3:
    region: us-east-1
    force_path_style: true
    prefix: pic-gallery
`), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := LoadYAML(configPath)
	if err != nil {
		t.Fatalf("LoadYAML returned error: %v", err)
	}

	if cfg.Storage.S3.Endpoint != "" ||
		cfg.Storage.S3.Region != "us-east-1" ||
		cfg.Storage.S3.Bucket != "" ||
		cfg.Storage.S3.AccessKeyID != "" ||
		cfg.Storage.S3.SecretAccessKey != "" ||
		!cfg.Storage.S3.ForcePathStyle ||
		cfg.Storage.S3.Prefix != "pic-gallery" {
		t.Fatalf("expected BFSS env aliases to be ignored, got %#v", cfg.Storage.S3)
	}
}

func TestLoadEnvFileDoesNotOverrideProcessEnv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := []byte("PIC_GALLERY_ENV=production\nPIC_GALLERY_ADDR=:8081\nDATABASE_URL=postgres://file@db:5432/pic_gallery?sslmode=disable\n")
	if err := os.WriteFile(envPath, content, 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	t.Setenv("PIC_GALLERY_ADDR", ":9090")

	cfg, err := LoadEnv(envPath)
	if err != nil {
		t.Fatalf("LoadEnv returned error: %v", err)
	}
	if cfg.App.Addr != ":9090" {
		t.Fatalf("expected process env to win, got %q", cfg.App.Addr)
	}
	if cfg.Database.URL != "postgres://file@db:5432/pic_gallery?sslmode=disable" {
		t.Fatalf("expected database URL from env file, got %q", cfg.Database.URL)
	}
}

func TestLoadEnvDoesNotLeakFileValuesBetweenLoads(t *testing.T) {
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "first.env")
	secondPath := filepath.Join(dir, "second.env")
	if err := os.WriteFile(firstPath, []byte("PIC_GALLERY_ENV=production\nDATABASE_URL=postgres://first@db:5432/pic_gallery?sslmode=disable\n"), 0o600); err != nil {
		t.Fatalf("write first env file: %v", err)
	}
	if err := os.WriteFile(secondPath, []byte("PIC_GALLERY_ENV=production\nDATABASE_URL=postgres://second@db:5432/pic_gallery?sslmode=disable\n"), 0o600); err != nil {
		t.Fatalf("write second env file: %v", err)
	}

	if _, err := LoadEnv(firstPath); err != nil {
		t.Fatalf("LoadEnv first returned error: %v", err)
	}
	cfg, err := LoadEnv(secondPath)
	if err != nil {
		t.Fatalf("LoadEnv second returned error: %v", err)
	}

	if cfg.Database.URL != "postgres://second@db:5432/pic_gallery?sslmode=disable" {
		t.Fatalf("expected second env file database URL, got %q", cfg.Database.URL)
	}
}

func TestLoadEnvSupportsPicGalleryCashierEncryptionAlias(t *testing.T) {
	t.Setenv("PIC_GALLERY_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://pic_gallery:secret@db:5432/pic_gallery?sslmode=disable")
	t.Setenv("PIC_GALLERY_CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY", "cashier-alias-key")

	cfg, err := LoadEnv("")
	if err != nil {
		t.Fatalf("LoadEnv returned error: %v", err)
	}

	if cfg.Cashier.ProviderConfigEncryptionKey != "cashier-alias-key" {
		t.Fatalf("expected cashier provider config alias value, got %q", cfg.Cashier.ProviderConfigEncryptionKey)
	}
}
