package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultConfigPath = "configs/config.dev.yaml"

func Load(path string) (Config, error) {
	if path == "" {
		path = configPathFromEnv()
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config file %q: %w", path, err)
	}

	applyEnvOverrides(&cfg)
	applyDefaults(&cfg)
	return cfg, nil
}

func configPathFromEnv() string {
	if value := os.Getenv("APP_CONFIG_PATH"); value != "" {
		return value
	}
	if value := os.Getenv("PIC_GALLERY_CONFIG"); value != "" {
		return value
	}
	return defaultConfigPath
}

func applyEnvOverrides(cfg *Config) {
	if value := os.Getenv("APP_NAME"); value != "" {
		cfg.App.Name = value
	}
	if value := os.Getenv("APP_ENV"); value != "" {
		cfg.App.Env = value
	}
	if value := os.Getenv("APP_ADDR"); value != "" {
		cfg.App.Addr = value
	}
	if value := os.Getenv("DATABASE_URL"); value != "" {
		cfg.Database.URL = value
	}
	if value := os.Getenv("REDIS_URL"); value != "" {
		cfg.Redis.URL = value
	}
	if value := os.Getenv("STORAGE_DRIVER"); value != "" {
		cfg.Storage.Driver = value
	}
	if value := os.Getenv("STORAGE_LOCAL_ROOT"); value != "" {
		cfg.Storage.LocalRoot = value
	}
	if value := os.Getenv("STORAGE_SHARED_VOLUME"); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			cfg.Storage.SharedVolume = parsed
		}
	}
	if value := os.Getenv("OPENAI_API_KEY"); value != "" {
		cfg.Providers.OpenAI.APIKey = value
	}
	if value := os.Getenv("OPENROUTER_API_KEY"); value != "" {
		cfg.Providers.OpenRouter.APIKey = value
	}
	if value := os.Getenv("AUTH_ACCESS_TOKEN_SECRET"); value != "" {
		cfg.Auth.AccessTokenSecret = value
	}
	if value := os.Getenv("API_KEY_SIGNING_SECRET_ENCRYPTION_KEY"); value != "" {
		cfg.APIKey.SigningSecretEncryptionKey = value
	}
	if value := os.Getenv("PIC_GALLERY_API_KEY_SIGNING_SECRET_ENCRYPTION_KEY"); value != "" {
		cfg.APIKey.SigningSecretEncryptionKey = value
	}
	if value := os.Getenv("SMTP_HOST"); value != "" {
		cfg.Auth.SMTP.Host = value
	}
	if value := os.Getenv("SMTP_PORT"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			cfg.Auth.SMTP.Port = parsed
		}
	}
	if value := os.Getenv("SMTP_USERNAME"); value != "" {
		cfg.Auth.SMTP.Username = value
	}
	if value := os.Getenv("SMTP_PASSWORD"); value != "" {
		cfg.Auth.SMTP.Password = value
	}
	if value := os.Getenv("SMTP_FROM"); value != "" {
		cfg.Auth.SMTP.From = value
	}
	if value := os.Getenv("SMTP_STARTTLS"); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			cfg.Auth.SMTP.StartTLS = parsed
		}
	}
	if value := os.Getenv("SMTP_INSECURE_SKIP_VERIFY"); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			cfg.Auth.SMTP.InsecureSkipVerify = parsed
		}
	}
}

func applyDefaults(cfg *Config) {
	if cfg.App.Name == "" {
		cfg.App.Name = "pic-gallery"
	}
	if cfg.App.Env == "" {
		cfg.App.Env = "local"
	}
	if cfg.Storage.Driver == "" {
		cfg.Storage.Driver = "local"
	}
	if cfg.App.Addr == "" {
		cfg.App.Addr = ":8080"
	}
	if cfg.Auth.AccessTokenSecret == "" {
		cfg.Auth.AccessTokenSecret = "local-dev-secret"
	}
	if cfg.Auth.RefreshCookieName == "" {
		cfg.Auth.RefreshCookieName = "pg_refresh_token"
	}
	if cfg.APIKey.SigningSecretEncryptionKey == "" {
		cfg.APIKey.SigningSecretEncryptionKey = "local-dev-api-key-signing-secret-encryption-key"
	}
	cfg.Billing.PointsScale = 5
	if len(cfg.Billing.AutoQualityDefaultByGroup) == 0 {
		cfg.Billing.AutoQualityDefaultByGroup = map[string]string{"basic": "1k", "plus": "2k", "pro": "4k"}
	}
	if len(cfg.Billing.UserGroupMultipliers) == 0 {
		cfg.Billing.UserGroupMultipliers = map[string]string{"basic": "1.00000", "plus": "1.00000", "pro": "1.00000"}
	}
	if cfg.GenerationLimits.MaxImageCount == 0 {
		cfg.GenerationLimits.MaxImageCount = 5
	}
	if cfg.GenerationLimits.ReferenceImageMaxMB == 0 {
		cfg.GenerationLimits.ReferenceImageMaxMB = 10
	}
	if cfg.GenerationLimits.ReferenceImageMaxCount == 0 {
		cfg.GenerationLimits.ReferenceImageMaxCount = 4
	}
	if cfg.GenerationLimits.PromptMaxChars == 0 {
		cfg.GenerationLimits.PromptMaxChars = 4000
	}
	if cfg.GenerationLimits.NegativePromptMaxChars == 0 {
		cfg.GenerationLimits.NegativePromptMaxChars = 1000
	}
	if cfg.Routing.DefaultProvider == "" {
		cfg.Routing.DefaultProvider = "openai"
	}
	if len(cfg.Routing.ProviderCapabilities) == 0 {
		cfg.Routing.ProviderCapabilities = map[string]ProviderCapabilityConfig{
			"openai":     defaultProviderCapability(cfg, []string{"basic", "plus", "pro"}, true, true, 1),
			"openrouter": defaultProviderCapability(cfg, []string{"basic", "plus", "pro"}, true, false, 2),
		}
	}
	for name, capability := range cfg.Routing.ProviderCapabilities {
		if len(capability.SupportedTaskTypes) == 0 {
			capability.SupportedTaskTypes = []string{"text_to_image", "image_edit", "reference_generate"}
		}
		if len(capability.SupportedQualities) == 0 {
			capability.SupportedQualities = []string{"1k", "2k", "4k"}
		}
		if len(capability.SupportedAspectRatios) == 0 {
			capability.SupportedAspectRatios = []string{"1:1", "4:3", "16:9"}
		}
		if capability.MaxImageCount == 0 {
			capability.MaxImageCount = cfg.GenerationLimits.MaxImageCount
		}
		if capability.MaxReferenceImageCount == 0 {
			capability.MaxReferenceImageCount = cfg.GenerationLimits.ReferenceImageMaxCount
		}
		if capability.Priority == 0 {
			capability.Priority = providerPriority(cfg.Routing, name)
		}
		cfg.Routing.ProviderCapabilities[name] = capability
	}
	if len(cfg.Routing.OpenAICompatModelMap) == 0 {
		cfg.Routing.OpenAICompatModelMap = map[string]string{"gpt-image-2": "plus"}
	}
	if len(cfg.Routing.ProviderModelMap) == 0 {
		cfg.Routing.ProviderModelMap = map[string]map[string]string{
			"basic": {"openai": "gpt-image-1", "openrouter": "openai/gpt-image-1"},
			"plus":  {"openai": "gpt-image-1", "openrouter": "openai/gpt-image-1"},
			"pro":   {"openai": "gpt-image-1", "openrouter": "openai/gpt-image-1"},
		}
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func defaultProviderCapability(cfg *Config, models []string, supportsImageInput bool, supportsMask bool, priority int) ProviderCapabilityConfig {
	return ProviderCapabilityConfig{
		SupportedModels:        models,
		SupportedTaskTypes:     []string{"text_to_image", "image_edit", "reference_generate"},
		SupportedQualities:     []string{"1k", "2k", "4k"},
		SupportedAspectRatios:  []string{"1:1", "4:3", "16:9"},
		MaxImageCount:          cfg.GenerationLimits.MaxImageCount,
		MaxReferenceImageCount: cfg.GenerationLimits.ReferenceImageMaxCount,
		SupportsImageInput:     supportsImageInput,
		SupportsMask:           supportsMask,
		Priority:               priority,
	}
}

func providerPriority(cfg RoutingConfig, provider string) int {
	provider = strings.ToLower(provider)
	if strings.EqualFold(cfg.DefaultProvider, provider) {
		return 1
	}
	for idx, fallback := range cfg.FallbackProviders {
		if strings.EqualFold(fallback, provider) {
			return idx + 2
		}
	}
	return len(cfg.FallbackProviders) + 2
}
