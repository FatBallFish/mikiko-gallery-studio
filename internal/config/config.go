package config

import "time"

type Config struct {
	App              AppConfig              `yaml:"app"`
	Database         DatabaseConfig         `yaml:"database"`
	Redis            RedisConfig            `yaml:"redis"`
	Storage          StorageConfig          `yaml:"storage"`
	Auth             AuthConfig             `yaml:"auth"`
	Billing          BillingConfig          `yaml:"billing"`
	GenerationLimits GenerationLimitsConfig `yaml:"generation_limits"`
	Providers        ProvidersConfig        `yaml:"providers"`
	Routing          RoutingConfig          `yaml:"routing"`
	Docs             DocsConfig             `yaml:"docs"`
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
	URL string `yaml:"url"`
}

type StorageConfig struct {
	Driver        string `yaml:"driver"`
	LocalRoot     string `yaml:"local_root"`
	PublicBaseURL string `yaml:"public_base_url"`
	SharedVolume  bool   `yaml:"shared_volume"`
}

type AuthConfig struct {
	AccessTokenTTL    time.Duration `yaml:"access_token_ttl"`
	RefreshTokenTTL   time.Duration `yaml:"refresh_token_ttl"`
	Issuer            string        `yaml:"issuer"`
	AccessTokenSecret string        `yaml:"access_token_secret"`
	RefreshCookieName string        `yaml:"refresh_cookie_name"`
}

type BillingConfig struct {
	CNYPerPoint               string                       `yaml:"cny_per_point"`
	PointsScale               int                          `yaml:"points_scale"`
	AutoQualityDefaultByGroup map[string]string            `yaml:"auto_quality_default_by_group"`
	QualityPointsByModel      map[string]map[string]string `yaml:"quality_points_by_model"`
	UserGroupMultipliers      map[string]string            `yaml:"user_group_multipliers"`
	TaskMultipliers           map[string]string            `yaml:"task_multipliers"`
	ReferenceImageExtra       ReferenceExtra               `yaml:"reference_image_extra"`
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
	SupportedModels        []string `yaml:"supported_models"`
	SupportedTaskTypes     []string `yaml:"supported_task_types"`
	SupportedQualities     []string `yaml:"supported_qualities"`
	SupportedAspectRatios  []string `yaml:"supported_aspect_ratios"`
	MaxImageCount          int      `yaml:"max_image_count"`
	MaxReferenceImageCount int      `yaml:"max_reference_image_count"`
	SupportsImageInput     bool     `yaml:"supports_image_input"`
	SupportsMask           bool     `yaml:"supports_mask"`
	Priority               int      `yaml:"priority"`
}

type DocsConfig struct {
	Title    string `yaml:"title"`
	BasePath string `yaml:"base_path"`
}
