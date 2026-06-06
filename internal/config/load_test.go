package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAppliesEnvOverrides(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("APP_ADDR", ":9090")
	t.Setenv("REDIS_URL", "redis://localhost:6379/1")
	t.Setenv("REDIS_KEY_PREFIX", "test-prefix")
	t.Setenv("OPENAI_API_KEY", "openai-test-key")
	t.Setenv("API_KEY_SIGNING_SECRET_ENCRYPTION_KEY", "api-key-secret-test-key")
	t.Setenv("CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY", "cashier-provider-config-test-key")
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_USERNAME", "mailer")
	t.Setenv("SMTP_PASSWORD", "secret")
	t.Setenv("SMTP_FROM", "Pic Gallery <noreply@example.com>")
	t.Setenv("SMTP_STARTTLS", "true")
	t.Setenv("SMTP_INSECURE_SKIP_VERIFY", "true")

	cfg, err := Load("../../configs/config.dev.yaml")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.App.Env != "test" {
		t.Fatalf("expected APP_ENV override, got %q", cfg.App.Env)
	}
	if cfg.App.Addr != ":9090" {
		t.Fatalf("expected APP_ADDR override, got %q", cfg.App.Addr)
	}
	if cfg.Redis.URL != "redis://localhost:6379/1" || cfg.Redis.KeyPrefix != "test-prefix" {
		t.Fatalf("expected Redis env overrides, got %#v", cfg.Redis)
	}
	if cfg.Providers.OpenAI.APIKey != "openai-test-key" {
		t.Fatalf("expected OPENAI_API_KEY override, got %q", cfg.Providers.OpenAI.APIKey)
	}
	if cfg.Billing.PointsScale != 5 {
		t.Fatalf("expected billing scale 5, got %d", cfg.Billing.PointsScale)
	}
	if cfg.Cashier.MaxPendingOrdersPerUser != 3 || cfg.Cashier.OrderTimeoutSeconds != 1800 {
		t.Fatalf("expected cashier defaults from config, got %#v", cfg.Cashier)
	}
	if cfg.APIKey.SigningSecretEncryptionKey != "api-key-secret-test-key" {
		t.Fatalf("expected API key signing secret env override, got %q", cfg.APIKey.SigningSecretEncryptionKey)
	}
	if cfg.Cashier.ProviderConfigEncryptionKey != "cashier-provider-config-test-key" {
		t.Fatalf("expected cashier provider config env override, got %q", cfg.Cashier.ProviderConfigEncryptionKey)
	}
	if cfg.Auth.SMTP.Host != "smtp.example.com" || cfg.Auth.SMTP.Port != 587 || cfg.Auth.SMTP.Username != "mailer" || cfg.Auth.SMTP.Password != "secret" || cfg.Auth.SMTP.From != "Pic Gallery <noreply@example.com>" {
		t.Fatalf("expected SMTP env overrides, got %#v", cfg.Auth.SMTP)
	}
	if !cfg.Auth.SMTP.StartTLS || !cfg.Auth.SMTP.InsecureSkipVerify {
		t.Fatalf("expected SMTP bool env overrides, got %#v", cfg.Auth.SMTP)
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
