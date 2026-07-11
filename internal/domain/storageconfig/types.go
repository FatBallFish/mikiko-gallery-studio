package storageconfig

import "time"

const (
	DriverLocal = "local"
	DriverS3    = "s3"

	ProviderLocal    = "local"
	ProviderAWSS3    = "aws_s3"
	ProviderMinIO    = "minio"
	ProviderR2       = "r2"
	ProviderCustomS3 = "custom_s3"

	StatusEnabled  = "enabled"
	StatusDisabled = "disabled"
	StatusDeleted  = "deleted"

	ProbeStatusNever   = "never"
	ProbeStatusSuccess = "success"
	ProbeStatusFailed  = "failed"
)

type SecretStatus struct {
	HasSecret    bool       `json:"has_secret"`
	Fingerprint  string     `json:"fingerprint,omitempty"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
	SecretFields []string   `json:"secret_fields,omitempty"`
}

type ProbeView struct {
	Status    string     `json:"status"`
	CheckedAt *time.Time `json:"checked_at,omitempty"`
	LatencyMS int64      `json:"latency_ms,omitempty"`
	Message   string     `json:"message,omitempty"`
}

type ConfigView struct {
	ID             string       `json:"id"`
	Code           string       `json:"code"`
	Name           string       `json:"name"`
	Driver         string       `json:"driver"`
	Provider       string       `json:"provider"`
	Status         string       `json:"status"`
	ReadEnabled    bool         `json:"read_enabled"`
	WriteEnabled   bool         `json:"write_enabled"`
	IsDefault      bool         `json:"is_default"`
	Endpoint       string       `json:"endpoint,omitempty"`
	Region         string       `json:"region,omitempty"`
	Bucket         string       `json:"bucket,omitempty"`
	Prefix         string       `json:"prefix,omitempty"`
	ForcePathStyle bool         `json:"force_path_style"`
	PublicBaseURL  string       `json:"public_base_url,omitempty"`
	LocalRoot      string       `json:"local_root,omitempty"`
	SecretStatus   SecretStatus `json:"secret_status"`
	LastProbe      ProbeView    `json:"last_probe"`
	Version        int64        `json:"version"`
	UpdatedBy      int64        `json:"updated_by,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

type ConfigRecord struct {
	ID                string
	Code              string
	Name              string
	Driver            string
	Provider          string
	Status            string
	ReadEnabled       bool
	WriteEnabled      bool
	IsDefault         bool
	Endpoint          string
	Region            string
	Bucket            string
	Prefix            string
	ForcePathStyle    bool
	PublicBaseURL     string
	LocalRoot         string
	PublicValue       map[string]any
	SecretEncrypted   map[string]any
	SecretFingerprint string
	SecretFields      []string
	LastProbeStatus   string
	LastProbeMessage  string
	LastProbeAt       *time.Time
	Version           int64
	UpdatedBy         int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type ResolvedConfig struct {
	ConfigRecord
	Secrets map[string]any
}

type WriteRequest struct {
	ID             string
	Version        int64
	Code           string
	Name           string
	Driver         string
	Provider       string
	Status         string
	ReadEnabled    bool
	WriteEnabled   bool
	Endpoint       string
	Region         string
	Bucket         string
	Prefix         string
	ForcePathStyle bool
	PublicBaseURL  string
	LocalRoot      string
	Secrets        map[string]string
	ClearSecrets   []string
	UpdatedBy      int64
}

type StatusRequest struct {
	ID           string
	Version      int64
	Status       string
	ReadEnabled  bool
	WriteEnabled bool
	UpdatedBy    int64
}

type SetDefaultRequest struct {
	ID        string
	Version   int64
	UpdatedBy int64
}

type ProbeResult struct {
	Status    string    `json:"status"`
	CheckedAt time.Time `json:"checked_at,omitempty"`
	LatencyMS int64     `json:"latency_ms,omitempty"`
	Message   string    `json:"message,omitempty"`
}
