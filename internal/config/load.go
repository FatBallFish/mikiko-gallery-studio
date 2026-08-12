package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const defaultConfigPath = "config.yaml"

type BootstrapConfig struct {
	Path                 string
	SchemaVersion        int
	Deployment           DeploymentContext
	DeploymentModules    []string
	PostgresManaged      bool
	RedisManaged         bool
	ObjectStorageManaged bool
	SetupCompleted       bool
	SetupToken           string
	SetupTokenVersion    uint64
	InstallationID       string
	ClusterNodeID        string
	ConfigRevision       int
	ApplicationVersion   string
	Values               map[string]string
}

func DefaultRuntimeEnvPath() string {
	return filepath.FromSlash("./config/runtime.env")
}

func Load(path string) (Config, error) {
	return LoadRuntime(path)
}

func LoadYAML(path string) (Config, error) {
	if path == "" {
		path = defaultConfigPath
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
	return LoadRuntime(path)
}

func LoadBootstrap(path string) (BootstrapConfig, error) {
	resolvedPath := resolveRuntimeEnvPath(path)
	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		return BootstrapConfig{}, fmt.Errorf("read runtime env %q: %w", resolvedPath, err)
	}
	document, err := ParseRuntimeEnv(content)
	if err != nil {
		return BootstrapConfig{}, fmt.Errorf("parse runtime env %q: %w", resolvedPath, err)
	}

	bootstrap := BootstrapConfig{
		Path:               resolvedPath,
		DeploymentModules:  envCSV(document.Values, "DEPLOYMENT_MODULES"),
		SetupToken:         document.Values["SETUP_TOKEN"],
		InstallationID:     document.Values["INSTALLATION_ID"],
		ClusterNodeID:      document.Values["CLUSTER_NODE_ID"],
		ApplicationVersion: document.Values["APPLICATION_VERSION"],
		Values:             cloneStringMap(document.Values),
	}
	bootstrap.Deployment.Mode = DeploymentMode(document.Values["DEPLOYMENT_MODE"])
	bootstrap.Deployment.Profile = DeploymentProfile(document.Values["DEPLOYMENT_PROFILE"])
	bootstrap.Deployment.Topology = DeploymentTopology(document.Values["DEPLOYMENT_TOPOLOGY"])
	bootstrap.Deployment.Role = DeploymentRole(document.Values["DEPLOYMENT_ROLE"])
	bootstrap.Deployment.StorageDriver = document.Values["STORAGE_DRIVER"]

	if bootstrap.SchemaVersion, err = optionalEnvInt(document.Values, "RUNTIME_SCHEMA_VERSION"); err != nil {
		return BootstrapConfig{}, err
	}
	if bootstrap.ConfigRevision, err = optionalEnvInt(document.Values, "CONFIG_REVISION"); err != nil {
		return BootstrapConfig{}, err
	}
	if bootstrap.SetupTokenVersion, err = optionalEnvPositiveUint64(document.Values, "SETUP_TOKEN_VERSION"); err != nil {
		return BootstrapConfig{}, err
	}
	if bootstrap.SetupCompleted, err = optionalEnvBool(document.Values, "SETUP_COMPLETED"); err != nil {
		return BootstrapConfig{}, err
	}
	if bootstrap.PostgresManaged, err = optionalEnvBool(document.Values, "POSTGRES_MANAGED"); err != nil {
		return BootstrapConfig{}, err
	}
	if bootstrap.RedisManaged, err = optionalEnvBool(document.Values, "REDIS_MANAGED"); err != nil {
		return BootstrapConfig{}, err
	}
	if bootstrap.ObjectStorageManaged, err = optionalEnvBool(document.Values, "OBJECT_STORAGE_MANAGED"); err != nil {
		return BootstrapConfig{}, err
	}
	bootstrap.Deployment.SetupCompleted = bootstrap.SetupCompleted
	return bootstrap, nil
}

func LoadRuntime(path string) (Config, error) {
	bootstrap, err := LoadBootstrap(path)
	if err != nil {
		return Config{}, err
	}
	return RuntimeFromBootstrap(bootstrap)
}

// RuntimeFromBootstrap validates and maps the exact document snapshot already
// loaded for startup-mode selection. It performs no filesystem reads.
func RuntimeFromBootstrap(bootstrap BootstrapConfig) (Config, error) {
	if !bootstrap.SetupCompleted {
		return Config{}, fmt.Errorf("SETUP_COMPLETED must be true before loading runtime configuration")
	}
	if err := validateRuntimeValues(bootstrap.Values, bootstrap.Deployment, bootstrap.SchemaVersion); err != nil {
		return Config{}, err
	}

	cfg := configFromRuntimeValues(bootstrap.Values)
	cfg.Runtime = RuntimeConfig{
		DeploymentMode:      bootstrap.Deployment.Mode,
		DeploymentRole:      bootstrap.Deployment.Role,
		DeploymentModules:   append([]string(nil), bootstrap.DeploymentModules...),
		Path:                bootstrap.Path,
		InstallationID:      bootstrap.InstallationID,
		ClusterNodeID:       bootstrap.ClusterNodeID,
		ApplicationVersion:  bootstrap.ApplicationVersion,
		ConfigSchemaVersion: bootstrap.SchemaVersion,
		ConfigRevision:      bootstrap.ConfigRevision,
		PublicAPIURL:        envString(bootstrap.Values, "PUBLIC_API_URL", ""),
		DocsURL:             envString(bootstrap.Values, "PIC_GALLERY_DOCS_URL", "/developer-docs/"),
		DocsProbeURL:        envString(bootstrap.Values, "PIC_GALLERY_DOCS_PROBE_URL", ""),
		GatewayPort:         envString(bootstrap.Values, "GATEWAY_PORT", ""),
	}
	applyDefaults(&cfg)
	if bootstrap.Deployment.Role != DeploymentRoleWeb {
		if err := validateEnvConfig(cfg); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}

func resolveRuntimeEnvPath(path string) string {
	if path != "" {
		return path
	}
	if override := strings.TrimSpace(os.Getenv("APP_ENV_FILE")); override != "" {
		return override
	}
	return DefaultRuntimeEnvPath()
}

func validateRuntimeValues(values map[string]string, context DeploymentContext, schemaVersion int) error {
	schema := DefaultRuntimeSchema()
	if schemaVersion != schema.Version {
		return fmt.Errorf("RUNTIME_SCHEMA_VERSION must be %d, got %d", schema.Version, schemaVersion)
	}
	required, err := RequiredRuntimeFields(schema, context)
	if err != nil {
		return fmt.Errorf("validate runtime deployment metadata: %w", err)
	}
	for _, runtimeField := range required {
		if strings.TrimSpace(values[runtimeField.Key]) == "" {
			return fmt.Errorf("required runtime field %s must be configured", runtimeField.Key)
		}
	}
	for _, runtimeField := range schema.Fields {
		value, exists := values[runtimeField.Key]
		if !exists || value == "" {
			continue
		}
		if err := runtimeField.Validate(value); err != nil {
			return fmt.Errorf("validate runtime field %s: %w", runtimeField.Key, err)
		}
	}
	return nil
}

func configFromRuntimeValues(fileEnv map[string]string) Config {

	cfg := Config{}
	cfg.App.Name = envString(fileEnv, "PIC_GALLERY_NAME", envString(fileEnv, "APP_NAME", ""))
	cfg.App.Env = envString(fileEnv, "PIC_GALLERY_ENV", envString(fileEnv, "APP_ENV", "production"))
	cfg.App.Addr = envString(fileEnv, "PIC_GALLERY_ADDR", envString(fileEnv, "APP_ADDR", apiAddress(fileEnv["API_PORT"])))

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
	cfg.Storage.S3.Endpoint = envString(fileEnv, "STORAGE_S3_ENDPOINT", "")
	cfg.Storage.S3.Region = envString(fileEnv, "STORAGE_S3_REGION", "")
	cfg.Storage.S3.Bucket = envString(fileEnv, "STORAGE_S3_BUCKET", "")
	cfg.Storage.S3.AccessKeyID = envString(fileEnv, "STORAGE_S3_ACCESS_KEY_ID", "")
	cfg.Storage.S3.SecretAccessKey = envString(fileEnv, "STORAGE_S3_SECRET_ACCESS_KEY", "")
	cfg.Storage.S3.ForcePathStyle = envBool(fileEnv, "STORAGE_S3_FORCE_PATH_STYLE", false)
	cfg.Storage.S3.Prefix = envString(fileEnv, "STORAGE_S3_PREFIX", "")

	cfg.Auth.AccessTokenTTL = envDuration(fileEnv, "AUTH_ACCESS_TOKEN_TTL", 0)
	cfg.Auth.RefreshTokenTTL = envDuration(fileEnv, "AUTH_REFRESH_TOKEN_TTL", 0)
	cfg.Auth.Issuer = envString(fileEnv, "AUTH_ISSUER", "")
	cfg.Auth.AccessTokenSecret = envString(fileEnv, "AUTH_ACCESS_TOKEN_SECRET", "")
	cfg.Auth.RefreshCookieName = envString(fileEnv, "AUTH_REFRESH_COOKIE_NAME", "")
	cfg.Auth.AdminRefreshCookieName = envString(fileEnv, "AUTH_ADMIN_REFRESH_COOKIE_NAME", "")
	cfg.Auth.FixedEmailCode = envString(fileEnv, "AUTH_FIXED_EMAIL_CODE", "")
	cfg.Auth.DevEmailCodes = envBool(fileEnv, "AUTH_DEV_EMAIL_CODES", false)

	cfg.APIKey.SigningSecretEncryptionKey = envString(fileEnv, "API_KEY_SIGNING_SECRET_ENCRYPTION_KEY", "")
	cfg.Security.SecureConfigEncryptionKey = envString(fileEnv, "PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY", envString(fileEnv, "SECURE_CONFIG_ENCRYPTION_KEY", ""))
	cfg.Security.PromptOptimizationQuoteSigningKey = envString(fileEnv, "PROMPT_OPTIMIZATION_QUOTE_SIGNING_KEY", "")
	cfg.Cashier.ProviderConfigEncryptionKey = envString(fileEnv, "CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY", envString(fileEnv, "PIC_GALLERY_CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY", ""))
	cfg.Cashier.Enabled = envBool(fileEnv, "CASHIER_ENABLED", false)
	cfg.Cashier.MockEnabled = envBool(fileEnv, "CASHIER_MOCK_ENABLED", false)
	cfg.Cashier.OrderTimeoutSeconds = envInt(fileEnv, "CASHIER_ORDER_TIMEOUT_SECONDS", 0)
	cfg.Cashier.MaxPendingOrdersPerUser = envInt(fileEnv, "CASHIER_MAX_PENDING_ORDERS_PER_USER", 0)
	cfg.Cashier.SiteBaseURL = envString(fileEnv, "CASHIER_SITE_BASE_URL", "")
	cfg.Cashier.StripeAPIBaseURL = envString(fileEnv, "CASHIER_STRIPE_API_BASE_URL", "")

	cfg.Worker.MaxConcurrentTasks = envInt(fileEnv, "WORKER_MAX_CONCURRENT_TASKS", 0)
	for _, role := range envCSV(fileEnv, "WORKER_ROLES") {
		cfg.Worker.Roles = append(cfg.Worker.Roles, WorkerRole(role))
	}
	cfg.Worker.ImageConcurrency = envInt(fileEnv, "WORKER_IMAGE_CONCURRENCY", 0)
	cfg.Worker.VideoConcurrency = envInt(fileEnv, "WORKER_VIDEO_CONCURRENCY", 0)
	cfg.Worker.MediaConcurrency = envInt(fileEnv, "WORKER_MEDIA_CONCURRENCY", 0)
	cfg.Worker.CleanupConcurrency = envInt(fileEnv, "WORKER_CLEANUP_CONCURRENCY", 0)
	cfg.Worker.FFmpegPath = envString(fileEnv, "MEDIA_FFMPEG_PATH", "")
	cfg.Worker.FFprobePath = envString(fileEnv, "MEDIA_FFPROBE_PATH", "")
	cfg.Worker.TempDir = envString(fileEnv, "MEDIA_TEMP_DIR", "")
	cfg.Worker.TempDiskPausePercent = envInt(fileEnv, "MEDIA_TEMP_DISK_PAUSE_PERCENT", 0)
	cfg.Worker.TempDiskCriticalPercent = envInt(fileEnv, "MEDIA_TEMP_DISK_CRITICAL_PERCENT", 0)
	cfg.Worker.MetricsAddr = envString(fileEnv, "WORKER_METRICS_ADDR", "")
	cfg.Worker.AllowLoopbackVideoArtifacts = envBool(fileEnv, "VIDEO_ARTIFACT_ALLOW_LOOPBACK", false)
	cfg.HTTP.CORSAllowedOrigins = envCSV(fileEnv, "CORS_ALLOWED_ORIGINS")

	cfg.Providers.OpenAI.Enabled = envBool(fileEnv, "OPENAI_ENABLED", false)
	cfg.Providers.OpenAI.BaseURL = envString(fileEnv, "OPENAI_BASE_URL", "")
	cfg.Providers.OpenAI.APIKey = envString(fileEnv, "OPENAI_API_KEY", "")
	cfg.Providers.OpenRouter.Enabled = envBool(fileEnv, "OPENROUTER_ENABLED", false)
	cfg.Providers.OpenRouter.BaseURL = envString(fileEnv, "OPENROUTER_BASE_URL", "")
	cfg.Providers.OpenRouter.APIKey = envString(fileEnv, "OPENROUTER_API_KEY", "")

	return cfg
}

func apiAddress(port string) string {
	if strings.TrimSpace(port) == "" {
		return ""
	}
	return ":" + port
}

func optionalEnvInt(values map[string]string, key string) (int, error) {
	value := strings.TrimSpace(values[key])
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse runtime field %s as integer: %w", key, err)
	}
	return parsed, nil
}

func optionalEnvPositiveUint64(values map[string]string, key string) (uint64, error) {
	value := strings.TrimSpace(values[key])
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed == 0 {
		return 0, fmt.Errorf("parse runtime field %s as positive integer", key)
	}
	return parsed, nil
}

func optionalEnvBool(values map[string]string, key string) (bool, error) {
	value := strings.TrimSpace(values[key])
	if value == "" {
		return false, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("parse runtime field %s as boolean: %w", key, err)
	}
	return parsed, nil
}

func cloneStringMap(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
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
	if cfg.Auth.AdminRefreshCookieName == "" {
		cfg.Auth.AdminRefreshCookieName = "pg_admin_refresh_token"
	}
	if len(cfg.HTTP.CORSAllowedOrigins) == 0 {
		cfg.HTTP.CORSAllowedOrigins = []string{"http://localhost:5173", "http://127.0.0.1:5173", "http://localhost:5174", "http://127.0.0.1:5174"}
	}
	if cfg.APIKey.SigningSecretEncryptionKey == "" {
		cfg.APIKey.SigningSecretEncryptionKey = "local-dev-api-key-signing-secret-encryption-key"
	}
	cfg.Billing.PointsScale = 5
	if strings.TrimSpace(cfg.Billing.CNYPerPoint) == "" {
		cfg.Billing.CNYPerPoint = "0.3125"
	}
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
		cfg.Cashier.OrderTimeoutSeconds = 900
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
	if strings.TrimSpace(cfg.Security.PromptOptimizationQuoteSigningKey) == "" {
		cfg.Security.PromptOptimizationQuoteSigningKey = "local-dev-prompt-optimization-quote-signing-key"
	}
	if cfg.Worker.MaxConcurrentTasks <= 0 {
		cfg.Worker.MaxConcurrentTasks = 4
	}
	if cfg.Worker.MaxConcurrentTasks > 64 {
		cfg.Worker.MaxConcurrentTasks = 64
	}
	if len(cfg.Worker.Roles) == 0 {
		cfg.Worker.Roles = []WorkerRole{WorkerRoleImage, WorkerRoleVideo, WorkerRoleMedia, WorkerRoleCleanup}
	}
	if cfg.Worker.ImageConcurrency <= 0 {
		cfg.Worker.ImageConcurrency = cfg.Worker.MaxConcurrentTasks
	}
	if cfg.Worker.VideoConcurrency <= 0 {
		cfg.Worker.VideoConcurrency = 2
	}
	if cfg.Worker.MediaConcurrency <= 0 {
		cfg.Worker.MediaConcurrency = 2
	}
	if cfg.Worker.CleanupConcurrency <= 0 {
		cfg.Worker.CleanupConcurrency = 1
	}
	if cfg.Worker.FFmpegPath == "" {
		cfg.Worker.FFmpegPath = "ffmpeg"
	}
	if cfg.Worker.FFprobePath == "" {
		cfg.Worker.FFprobePath = "ffprobe"
	}
	if cfg.Worker.TempDir == "" {
		cfg.Worker.TempDir = "./data/tmp"
	}
	if cfg.Worker.TempDiskPausePercent == 0 {
		cfg.Worker.TempDiskPausePercent = 75
	}
	if cfg.Worker.TempDiskCriticalPercent == 0 {
		cfg.Worker.TempDiskCriticalPercent = 90
	}
	if cfg.Worker.MetricsAddr == "" {
		cfg.Worker.MetricsAddr = "127.0.0.1:9091"
	}
	if len(cfg.Billing.AutoBaseResolutionDefaultByGroup) == 0 {
		cfg.Billing.AutoBaseResolutionDefaultByGroup = map[string]string{"basic": "1k", "plus": "2k", "pro": "4k"}
	}
	if len(cfg.Billing.BaseResolutionPointsByModel) == 0 {
		cfg.Billing.BaseResolutionPointsByModel = map[string]map[string]string{
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
		cfg.GenerationLimits.ReferenceImageMaxMB = 20
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
	cfg.AttachmentPolicy = ApplyAttachmentPolicyDefaults(cfg.AttachmentPolicy, cfg.GenerationLimits.ReferenceImageMaxMB)
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
		capability.SupportedTaskTypes = currentImageTaskTypes(capability.SupportedTaskTypes)
		if len(capability.SupportedTaskTypes) == 0 {
			capability.SupportedTaskTypes = []string{"text_to_image", "image_edit"}
		}
		if len(capability.SupportedBaseResolution) == 0 {
			capability.SupportedBaseResolution = []string{"1k", "2k", "4k"}
		}
		if len(capability.Quality) == 0 {
			capability.Quality = []string{"auto", "low", "medium", "high"}
		}
		if len(capability.SupportedAspectRatios) == 0 {
			capability.SupportedAspectRatios = []string{"1:1", "4:3", "16:9"}
		}
		if len(capability.OutputFormat) == 0 {
			capability.OutputFormat = []string{"png"}
		}
		if capability.OutputCompression == 0 {
			capability.OutputCompression = 100
		}
		if len(capability.Moderation) == 0 {
			capability.Moderation = []string{"auto"}
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
		SupportedModels:         models,
		SupportedTaskTypes:      []string{"text_to_image", "image_edit"},
		SupportedBaseResolution: []string{"1k", "2k", "4k"},
		Quality:                 []string{"auto", "low", "medium", "high"},
		SupportedAspectRatios:   []string{"1:1", "4:3", "16:9"},
		OutputFormat:            []string{"png"},
		OutputCompression:       100,
		Moderation:              []string{"auto"},
		MaxImageCount:           cfg.GenerationLimits.MaxImageCount,
		MaxReferenceImageCount:  cfg.GenerationLimits.ReferenceImageMaxCount,
		SupportsImageInput:      supportsImageInput,
		SupportsMask:            supportsMask,
		Priority:                priority,
	}
}

func currentImageTaskTypes(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "text_to_image" || value == "image_edit" {
			result = append(result, value)
		}
	}
	return result
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

func envString(fileEnv map[string]string, key string, fallback string) string {
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
	if cfg.Worker.AllowLoopbackVideoArtifacts && !isLocalLikeEnv(cfg.App.Env) {
		return fmt.Errorf("VIDEO_ARTIFACT_ALLOW_LOOPBACK is only allowed in local or test environments")
	}
	if cfg.Worker.TempDiskCriticalPercent <= cfg.Worker.TempDiskPausePercent {
		return fmt.Errorf("media temporary disk critical watermark must be greater than pause watermark")
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
