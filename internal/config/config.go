package config

import "time"

const MaxImageAttachmentSizeMB = 100

type Config struct {
	Runtime          RuntimeConfig          `yaml:"-"`
	App              AppConfig              `yaml:"app"`
	Database         DatabaseConfig         `yaml:"database"`
	Redis            RedisConfig            `yaml:"redis"`
	Storage          StorageConfig          `yaml:"storage"`
	Auth             AuthConfig             `yaml:"auth"`
	APIKey           APIKeyConfig           `yaml:"api_key"`
	HTTP             HTTPConfig             `yaml:"http"`
	Billing          BillingConfig          `yaml:"billing"`
	Cashier          CashierConfig          `yaml:"cashier"`
	Security         SecurityConfig         `yaml:"security"`
	Worker           WorkerConfig           `yaml:"worker"`
	GenerationLimits GenerationLimitsConfig `yaml:"generation_limits"`
	AttachmentPolicy AttachmentPolicyConfig `yaml:"attachment_policy"`
	Providers        ProvidersConfig        `yaml:"providers"`
	Routing          RoutingConfig          `yaml:"routing"`
}

// RuntimeConfig is immutable identity metadata loaded from the same runtime.env
// snapshot as Database.URL. It is intentionally not populated from process env.
type RuntimeConfig struct {
	DeploymentRole      DeploymentRole
	Path                string
	InstallationID      string
	ClusterNodeID       string
	ApplicationVersion  string
	ConfigSchemaVersion int
	ConfigRevision      int
}

type AppConfig struct {
	Name string `yaml:"name"`
	Env  string `yaml:"env"`
	Addr string `yaml:"addr"`
}

type DatabaseConfig struct {
	URL             string        `yaml:"url"`
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
}

type RedisConfig struct {
	URL       string `yaml:"url"`
	KeyPrefix string `yaml:"key_prefix"`
}

type StorageConfig struct {
	Driver        string          `yaml:"driver"`
	LocalRoot     string          `yaml:"local_root"`
	PublicBaseURL string          `yaml:"public_base_url"`
	SharedVolume  bool            `yaml:"shared_volume"`
	S3            StorageS3Config `yaml:"s3"`
}

type StorageS3Config struct {
	Endpoint        string `yaml:"endpoint"`
	Region          string `yaml:"region"`
	Bucket          string `yaml:"bucket"`
	AccessKeyID     string `yaml:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key"`
	ForcePathStyle  bool   `yaml:"force_path_style"`
	Prefix          string `yaml:"prefix"`
}

type AuthConfig struct {
	AccessTokenTTL         time.Duration `yaml:"access_token_ttl"`
	RefreshTokenTTL        time.Duration `yaml:"refresh_token_ttl"`
	Issuer                 string        `yaml:"issuer"`
	AccessTokenSecret      string        `yaml:"access_token_secret"`
	RefreshCookieName      string        `yaml:"refresh_cookie_name"`
	AdminRefreshCookieName string        `yaml:"admin_refresh_cookie_name"`
	FixedEmailCode         string        `yaml:"fixed_email_code"`
	DevEmailCodes          bool          `yaml:"dev_email_codes"`
	SMTP                   SMTPConfig    `yaml:"smtp"`
}

type SMTPConfig struct {
	Host               string `yaml:"host"`
	Port               int    `yaml:"port"`
	Username           string `yaml:"username"`
	Password           string `yaml:"password"`
	From               string `yaml:"from"`
	StartTLS           bool   `yaml:"starttls"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
}

type APIKeyConfig struct {
	SigningSecretEncryptionKey string `yaml:"signing_secret_encryption_key"`
}

type HTTPConfig struct {
	CORSAllowedOrigins []string `yaml:"cors_allowed_origins"`
}

type BillingConfig struct {
	CNYPerPoint                      string                       `yaml:"cny_per_point"`
	PointsScale                      int                          `yaml:"points_scale"`
	SignupTrial                      SignupTrialConfig            `yaml:"signup_trial"`
	AutoBaseResolutionDefaultByGroup map[string]string            `yaml:"auto_base_resolution_default_by_group"`
	BaseResolutionPointsByModel      map[string]map[string]string `yaml:"base_resolution_points_by_model"`
	UserGroupMultipliers             map[string]string            `yaml:"user_group_multipliers"`
	TaskMultipliers                  map[string]string            `yaml:"task_multipliers"`
	ReferenceImageExtra              ReferenceExtra               `yaml:"reference_image_extra"`
}

type SignupTrialConfig struct {
	Enabled            bool   `yaml:"enabled"`
	Points             string `yaml:"points"`
	ValidDays          int    `yaml:"valid_days"`
	ExpiryReminderDays int    `yaml:"expiry_reminder_days"`
	GrantOncePerUser   bool   `yaml:"grant_once_per_user"`
}

type CashierConfig struct {
	Enabled                     bool   `yaml:"enabled"`
	MockEnabled                 bool   `yaml:"mock_enabled"`
	OrderTimeoutSeconds         int    `yaml:"order_timeout_seconds"`
	MaxPendingOrdersPerUser     int    `yaml:"max_pending_orders_per_user"`
	SiteBaseURL                 string `yaml:"site_base_url"`
	StripeAPIBaseURL            string `yaml:"stripe_api_base_url"`
	ProviderConfigEncryptionKey string `yaml:"provider_config_encryption_key"`
}

type SecurityConfig struct {
	SecureConfigEncryptionKey         string `yaml:"secure_config_encryption_key"`
	PromptOptimizationQuoteSigningKey string `yaml:"prompt_optimization_quote_signing_key"`
}

type WorkerConfig struct {
	MaxConcurrentTasks int `yaml:"max_concurrent_tasks"`
}

type ReferenceExtra struct {
	First      string `yaml:"first"`
	Additional string `yaml:"additional"`
}

type GenerationLimitsConfig struct {
	MaxImageCount          int `yaml:"max_image_count"`
	ReferenceImageMaxMB    int `yaml:"reference_image_max_mb"`
	ReferenceImageMaxCount int `yaml:"reference_image_max_count"`
	PromptMaxChars         int `yaml:"prompt_max_chars"`
	NegativePromptMaxChars int `yaml:"negative_prompt_max_chars"`
}

type AttachmentPolicyConfig struct {
	ImageMaxMB             int      `yaml:"image_max_mb"`
	VideoMaxMB             int      `yaml:"video_max_mb"`
	AudioMaxMB             int      `yaml:"audio_max_mb"`
	DocumentMaxMB          int      `yaml:"document_max_mb"`
	ImageAllowedFormats    []string `yaml:"image_allowed_formats"`
	VideoAllowedFormats    []string `yaml:"video_allowed_formats"`
	AudioAllowedFormats    []string `yaml:"audio_allowed_formats"`
	DocumentAllowedFormats []string `yaml:"document_allowed_formats"`
}

func ApplyAttachmentPolicyDefaults(policy AttachmentPolicyConfig, referenceImageMaxMB int) AttachmentPolicyConfig {
	if referenceImageMaxMB <= 0 {
		referenceImageMaxMB = 20
	}
	if policy.ImageMaxMB == 0 {
		policy.ImageMaxMB = referenceImageMaxMB
	}
	if policy.VideoMaxMB == 0 {
		policy.VideoMaxMB = 100
	}
	if policy.AudioMaxMB == 0 {
		policy.AudioMaxMB = 50
	}
	if policy.DocumentMaxMB == 0 {
		policy.DocumentMaxMB = 20
	}
	if len(policy.ImageAllowedFormats) == 0 {
		policy.ImageAllowedFormats = []string{"png", "jpeg", "webp", "gif"}
	} else {
		policy.ImageAllowedFormats = append([]string(nil), policy.ImageAllowedFormats...)
	}
	if len(policy.VideoAllowedFormats) == 0 {
		policy.VideoAllowedFormats = []string{"mp4", "webm"}
	} else {
		policy.VideoAllowedFormats = append([]string(nil), policy.VideoAllowedFormats...)
	}
	if len(policy.AudioAllowedFormats) == 0 {
		policy.AudioAllowedFormats = []string{"mp3", "wav"}
	} else {
		policy.AudioAllowedFormats = append([]string(nil), policy.AudioAllowedFormats...)
	}
	if len(policy.DocumentAllowedFormats) == 0 {
		policy.DocumentAllowedFormats = []string{"pdf", "docx"}
	} else {
		policy.DocumentAllowedFormats = append([]string(nil), policy.DocumentAllowedFormats...)
	}
	return policy
}

type ProvidersConfig struct {
	OpenAI     ProviderConfig `yaml:"openai"`
	OpenRouter ProviderConfig `yaml:"openrouter"`
}

type ProviderConfig struct {
	Enabled bool   `yaml:"enabled"`
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key"`
}

type RoutingConfig struct {
	DefaultProvider      string                              `yaml:"default_provider"`
	FallbackProviders    []string                            `yaml:"fallback_providers"`
	ProviderCapabilities map[string]ProviderCapabilityConfig `yaml:"provider_capabilities"`
	OpenAICompatModelMap map[string]string                   `yaml:"openai_compat_model_map"`
	ProviderModelMap     map[string]map[string]string        `yaml:"provider_model_map"`
}

type ProviderCapabilityConfig struct {
	SupportedModels         []string `yaml:"supported_models"`
	SupportedTaskTypes      []string `yaml:"supported_task_types"`
	SupportedBaseResolution []string `yaml:"supported_base_resolution"`
	Quality                 []string `yaml:"quality"`
	SupportedAspectRatios   []string `yaml:"supported_aspect_ratios"`
	OutputFormat            []string `yaml:"output_format"`
	OutputCompression       int      `yaml:"output_compression"`
	Moderation              []string `yaml:"moderation"`
	MaxImageCount           int      `yaml:"max_image_count"`
	MaxReferenceImageCount  int      `yaml:"max_reference_image_count"`
	SupportsImageInput      bool     `yaml:"supports_image_input"`
	SupportsMask            bool     `yaml:"supports_mask"`
	Priority                int      `yaml:"priority"`
}
