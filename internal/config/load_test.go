package config

import "testing"

func TestLoadAppliesEnvOverrides(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("APP_ADDR", ":9090")
	t.Setenv("OPENAI_API_KEY", "openai-test-key")

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
	if cfg.Providers.OpenAI.APIKey != "openai-test-key" {
		t.Fatalf("expected OPENAI_API_KEY override, got %q", cfg.Providers.OpenAI.APIKey)
	}
	if cfg.Billing.PointsScale != 5 {
		t.Fatalf("expected billing scale 5, got %d", cfg.Billing.PointsScale)
	}
}
