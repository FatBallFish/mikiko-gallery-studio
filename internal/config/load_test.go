package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadIgnoresBusinessEnvOverrides(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("APP_ADDR", ":9090")
	t.Setenv("REDIS_URL", "redis://localhost:6379/1")
	t.Setenv("REDIS_KEY_PREFIX", "test-prefix")
	t.Setenv("STORAGE_DRIVER", "s3")
	t.Setenv("STORAGE_LOCAL_ROOT", "/var/lib/override")
	t.Setenv("OPENAI_API_KEY", "openai-test-key")
	t.Setenv("API_KEY_SIGNING_SECRET_ENCRYPTION_KEY", "api-key-secret-test-key")
	t.Setenv("CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY", "cashier-provider-config-test-key")
	t.Setenv("PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY", "secure-config-test-key")
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_USERNAME", "mailer")
	t.Setenv("SMTP_PASSWORD", "secret")
	t.Setenv("SMTP_FROM", "Pic Gallery <noreply@example.com>")
	t.Setenv("SMTP_STARTTLS", "true")
	t.Setenv("SMTP_INSECURE_SKIP_VERIFY", "true")
	t.Setenv("WORKER_MAX_CONCURRENT_TASKS", "12")

	cfg, err := Load("../../configs/config.dev.yaml")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.App.Env != "local" {
		t.Fatalf("expected app env from YAML, got %q", cfg.App.Env)
	}
	if cfg.App.Addr != ":8080" {
		t.Fatalf("expected app addr from YAML, got %q", cfg.App.Addr)
	}
	if cfg.Redis.URL != "redis://localhost:6379/0" || cfg.Redis.KeyPrefix != "pic-gallery" {
		t.Fatalf("expected Redis values from YAML/defaults, got %#v", cfg.Redis)
	}
	if cfg.Storage.Driver != "local" || cfg.Storage.LocalRoot != "./tmp/storage" {
		t.Fatalf("expected storage values from YAML, got %#v", cfg.Storage)
	}
	if cfg.Providers.OpenAI.APIKey != "" {
		t.Fatalf("expected OpenAI API key from YAML, got %q", cfg.Providers.OpenAI.APIKey)
	}
	if cfg.Billing.PointsScale != 5 {
		t.Fatalf("expected billing scale 5, got %d", cfg.Billing.PointsScale)
	}
	if cfg.Cashier.MaxPendingOrdersPerUser != 3 || cfg.Cashier.OrderTimeoutSeconds != 1800 {
		t.Fatalf("expected cashier defaults from config, got %#v", cfg.Cashier)
	}
	if cfg.APIKey.SigningSecretEncryptionKey != "local-dev-api-key-signing-secret-encryption-key" {
		t.Fatalf("expected API key signing secret default, got %q", cfg.APIKey.SigningSecretEncryptionKey)
	}
	if cfg.Cashier.ProviderConfigEncryptionKey != "local-dev-cashier-provider-config-encryption-key" {
		t.Fatalf("expected cashier provider config default, got %q", cfg.Cashier.ProviderConfigEncryptionKey)
	}
	if cfg.Security.SecureConfigEncryptionKey != "local-dev-secure-config-encryption-key" {
		t.Fatalf("expected secure config default, got %q", cfg.Security.SecureConfigEncryptionKey)
	}
	if cfg.Auth.SMTP.Host != "" || cfg.Auth.SMTP.Port != 0 || cfg.Auth.SMTP.Username != "" || cfg.Auth.SMTP.Password != "" || cfg.Auth.SMTP.From != "" {
		t.Fatalf("expected SMTP values from YAML, got %#v", cfg.Auth.SMTP)
	}
	if !cfg.Auth.SMTP.StartTLS || cfg.Auth.SMTP.InsecureSkipVerify {
		t.Fatalf("expected SMTP bool values from YAML, got %#v", cfg.Auth.SMTP)
	}
	if cfg.Worker.MaxConcurrentTasks != 4 {
		t.Fatalf("expected worker max concurrency default, got %d", cfg.Worker.MaxConcurrentTasks)
	}
}

func TestLoadUsesPicGalleryConfigEnv(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("app:\n  name: from-pic-gallery-config\n"), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	t.Setenv("APP_CONFIG_PATH", "")
	t.Setenv("PIC_GALLERY_CONFIG", configPath)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.App.Name != "from-pic-gallery-config" {
		t.Fatalf("expected PIC_GALLERY_CONFIG path to load config, got app name %q", cfg.App.Name)
	}
}

func TestLoadIgnoresBFSSStorageEnvAliases(t *testing.T) {
	t.Setenv("BFSS_ENDPOINT", "https://bfss.example.com")
	t.Setenv("BFSS_REGION", "cn-test-1")
	t.Setenv("BFSS_BUCKET", "pic-gallery-assets")
	t.Setenv("BFSS_ACCESS_KEY_ID", "bfss-ak")
	t.Setenv("BFSS_SECRET_ACCESS_KEY", "bfss-sk")
	t.Setenv("BFSS_FORCE_PATH_STYLE", "false")
	t.Setenv("BFSS_PREFIX", "prod/pic-gallery")

	cfg, err := Load("../../configs/config.dev.yaml")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
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
