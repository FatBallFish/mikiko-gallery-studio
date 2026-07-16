package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const defaultConfigPath = "config.yaml"

func Load(path string) (Config, error) {
	if path == "" {
		return LoadEnv("")
	}
	return LoadYAML(path)
}

func LoadYAML(path string) (Config, error) {
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

	applyDefaults(&cfg)
	return cfg, nil
}

func LoadEnv(path string) (Config, error) {
	fileEnv := map[string]string{}
	if path == "" {
		path = os.Getenv("PIC_GALLERY_ENV_FILE")
	}
	if path != "" {
		loaded, err := loadDotEnv(path)
		if err != nil {
			return Config{}, err
		}
		fileEnv = loaded
	} else if _, err := os.Stat(".env"); err == nil {
		loaded, err := loadDotEnv(".env")
		if err != nil {
			return Config{}, err
		}
		fileEnv = loaded
	} else if err != nil && !os.IsNotExist(err) {
		return Config{}, fmt.Errorf("stat .env: %w", err)
	}

	cfg := Config{}
	cfg.App.Name = envString(fileEnv, "PIC_GALLERY_NAME", envString(fileEnv, "APP_NAME", ""))
	cfg.App.Env = envString(fileEnv, "PIC_GALLERY_ENV", envString(fileEnv, "APP_ENV", ""))
	cfg.App.Addr = envString(fileEnv, "PIC_GALLERY_ADDR", envString(fileEnv, "APP_ADDR", ""))

	cfg.Database.URL = envString(fileEnv, "DATABASE_URL", "")
	cfg.Database.MaxOpenConns = envInt(fileEnv, "DATABASE_MAX_OPEN_CONNS", 0)
	cfg.Database.MaxIdleConns = envInt(fileEnv, "DATABASE_MAX_IDLE_CONNS", 0)
	cfg.Database.ConnMaxLifetime = envDuration(fileEnv, "DATABASE_CONN_MAX_LIFETIME", 0)

	cfg.Redis.URL = envString(fileEnv, "REDIS_URL", "")
	cfg.Redis.KeyPrefix = envString(fileEnv, "REDIS_KEY_PREFIX", "")

	cfg.Storage.Driver = envString(fileEnv, "STORAGE_DRIVER", "")
	cfg.Storage.LocalRoot = envString(fileEnv, "STORAGE_LOCAL_ROOT", "")
	cfg.Storage.PublicBaseURL = envString(fileEnv, "STORAGE_PUBLIC_BASE_URL", "")
	cfg.Storage.SharedVolume = envBool(fileEnv, "STORAGE_SHARED_VOLUME", false)

	cfg.Auth.AccessTokenTTL = envDuration(fileEnv, "AUTH_ACCESS_TOKEN_TTL", 0)
	cfg.Auth.RefreshTokenTTL = envDuration(fileEnv, "AUTH_REFRESH_TOKEN_TTL", 0)
	cfg.Auth.Issuer = envString(fileEnv, "AUTH_ISSUER", "")
	cfg.Auth.AccessTokenSecret = envString(fileEnv, "AUTH_ACCESS_TOKEN_SECRET", "")
	cfg.Auth.RefreshCookieName = envString(fileEnv, "AUTH_REFRESH_COOKIE_NAME", "")
	cfg.Auth.FixedEmailCode = envString(fileEnv, "AUTH_FIXED_EMAIL_CODE", "")
	cfg.Auth.DevEmailCodes = envBool(fileEnv, "AUTH_DEV_EMAIL_CODES", false)

	cfg.Admin.SeedEmail = envString(fileEnv, "PIC_GALLERY_ADMIN_EMAIL", "")
	cfg.Admin.SeedPassword = envString(fileEnv, "PIC_GALLERY_ADMIN_PASSWORD", "")
	cfg.Admin.SeedRole = envString(fileEnv, "PIC_GALLERY_ADMIN_ROLE", "")

	cfg.APIKey.SigningSecretEncryptionKey = envString(fileEnv, "API_KEY_SIGNING_SECRET_ENCRYPTION_KEY", "")
	cfg.Security.SecureConfigEncryptionKey = envString(fileEnv, "PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY", envString(fileEnv, "SECURE_CONFIG_ENCRYPTION_KEY", ""))
	cfg.Cashier.ProviderConfigEncryptionKey = envString(fileEnv, "CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY", envString(fileEnv, "PIC_GALLERY_CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY", ""))
	cfg.Cashier.Enabled = envBool(fileEnv, "CASHIER_ENABLED", false)
	cfg.Cashier.MockEnabled = envBool(fileEnv, "CASHIER_MOCK_ENABLED", false)
	cfg.Cashier.OrderTimeoutSeconds = envInt(fileEnv, "CASHIER_ORDER_TIMEOUT_SECONDS", 0)
	cfg.Cashier.MaxPendingOrdersPerUser = envInt(fileEnv, "CASHIER_MAX_PENDING_ORDERS_PER_USER", 0)
	cfg.Cashier.SiteBaseURL = envString(fileEnv, "CASHIER_SITE_BASE_URL", "")

	cfg.Worker.MaxConcurrentTasks = envInt(fileEnv, "WORKER_MAX_CONCURRENT_TASKS", 0)
	cfg.HTTP.CORSAllowedOrigins = envCSV(fileEnv, "CORS_ALLOWED_ORIGINS")

	cfg.Providers.OpenAI.Enabled = envBool(fileEnv, "OPENAI_ENABLED", false)
	cfg.Providers.OpenAI.BaseURL = envString(fileEnv, "OPENAI_BASE_URL", "")
	cfg.Providers.OpenAI.APIKey = envString(fileEnv, "OPENAI_API_KEY", "")
	cfg.Providers.OpenRouter.Enabled = envBool(fileEnv, "OPENROUTER_ENABLED", false)
	cfg.Providers.OpenRouter.BaseURL = envString(fileEnv, "OPENROUTER_BASE_URL", "")
	cfg.Providers.OpenRouter.APIKey = envString(fileEnv, "OPENROUTER_API_KEY", "")

	cfg.Docs.Title = envString(fileEnv, "DOCS_TITLE", "")
	cfg.Docs.BasePath = envString(fileEnv, "DOCS_BASE_PATH", "")

	applyDefaults(&cfg)
	if err := validateEnvConfig(cfg); err != nil {
		return Config{}, err
	}
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
	if cfg.Redis.KeyPrefix == "" {
		cfg.Redis.KeyPrefix = "pic-gallery"
	}
	if cfg.Auth.AccessTokenSecret == "" {
		cfg.Auth.AccessTokenSecret = "local-dev-secret"
	}
	if cfg.Auth.AccessTokenTTL == 0 {
		cfg.Auth.AccessTokenTTL = 10 * time.Minute
	}
	if cfg.Auth.RefreshTokenTTL == 0 {
		cfg.Auth.RefreshTokenTTL = 2 * time.Hour
	}
	if cfg.Auth.RefreshCookieName == "" {
		cfg.Auth.RefreshCookieName = "pg_refresh_token"
	}
	if len(cfg.HTTP.CORSAllowedOrigins) == 0 {
		cfg.HTTP.CORSAllowedOrigins = []string{"http://localhost:5173", "http://127.0.0.1:5173", "http://localhost:5174", "http://127.0.0.1:5174"}
	}
	if cfg.APIKey.SigningSecretEncryptionKey == "" {
		cfg.APIKey.SigningSecretEncryptionKey = "local-dev-api-key-signing-secret-encryption-key"
	}
	cfg.Billing.PointsScale = 5
	if strings.TrimSpace(cfg.Billing.SignupTrial.Points) == "" {
		cfg.Billing.SignupTrial.Points = "20.00000"
	}
	if cfg.Billing.SignupTrial.ValidDays == 0 {
		cfg.Billing.SignupTrial.ValidDays = 7
	}
	if cfg.Billing.SignupTrial.ExpiryReminderDays == 0 {
		cfg.Billing.SignupTrial.ExpiryReminderDays = 2
	}
	cfg.Billing.SignupTrial.GrantOncePerUser = true
	if cfg.Cashier.OrderTimeoutSeconds == 0 {
		cfg.Cashier.OrderTimeoutSeconds = 1800
	}
	if cfg.Cashier.MaxPendingOrdersPerUser == 0 {
		cfg.Cashier.MaxPendingOrdersPerUser = 3
	}
	if strings.TrimSpace(cfg.Cashier.ProviderConfigEncryptionKey) == "" {
		cfg.Cashier.ProviderConfigEncryptionKey = "local-dev-cashier-provider-config-encryption-key"
	}
	if strings.TrimSpace(cfg.Security.SecureConfigEncryptionKey) == "" {
		cfg.Security.SecureConfigEncryptionKey = "local-dev-secure-config-encryption-key"
	}
	if cfg.Worker.MaxConcurrentTasks <= 0 {
		cfg.Worker.MaxConcurrentTasks = 4
	}
	if cfg.Worker.MaxConcurrentTasks > 64 {
		cfg.Worker.MaxConcurrentTasks = 64
	}
	if len(cfg.Billing.AutoQualityDefaultByGroup) == 0 {
		cfg.Billing.AutoQualityDefaultByGroup = map[string]string{"basic": "1k", "plus": "2k", "pro": "4k"}
	}
	if len(cfg.Billing.QualityPointsByModel) == 0 {
		cfg.Billing.QualityPointsByModel = map[string]map[string]string{
			"basic": {"1k": "2.00000", "2k": "4.00000", "4k": "8.00000"},
			"plus":  {"1k": "5.00000", "2k": "8.00000", "4k": "16.00000"},
			"pro":   {"1k": "8.00000", "2k": "12.00000", "4k": "20.00000"},
		}
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

func loadDotEnv(path string) (map[string]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read env file %q: %w", path, err)
	}
	values := map[string]string{}
	for lineNo, rawLine := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("parse env file %q line %d: missing '='", path, lineNo+1)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("parse env file %q line %d: empty key", path, lineNo+1)
		}
		value = stripInlineComment(strings.TrimSpace(value))
		value = strings.Trim(value, `"'`)
		values[key] = value
	}
	return values, nil
}

func stripInlineComment(value string) string {
	if strings.HasPrefix(value, `"`) || strings.HasPrefix(value, `'`) {
		return value
	}
	if idx := strings.Index(value, " #"); idx >= 0 {
		return strings.TrimSpace(value[:idx])
	}
	return value
}

func envString(fileEnv map[string]string, key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	if value := fileEnv[key]; value != "" {
		return value
	}
	return fallback
}

func envInt(fileEnv map[string]string, key string, fallback int) int {
	value := strings.TrimSpace(envString(fileEnv, key, ""))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(fileEnv map[string]string, key string, fallback bool) bool {
	value := strings.TrimSpace(envString(fileEnv, key, ""))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(fileEnv map[string]string, key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(envString(fileEnv, key, ""))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err == nil {
		return parsed
	}
	seconds, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

func envCSV(fileEnv map[string]string, key string) []string {
	raw := strings.TrimSpace(envString(fileEnv, key, ""))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func validateEnvConfig(cfg Config) error {
	if strings.TrimSpace(cfg.Database.URL) == "" && !isLocalLikeEnv(cfg.App.Env) {
		return fmt.Errorf("DATABASE_URL must be configured in %s env", cfg.App.Env)
	}
	return nil
}

func isLocalLikeEnv(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "", "local", "dev", "development", "test":
		return true
	default:
		return false
	}
}
