package adminstorage

import "time"

const (
	DriverLocal = "local"
	DriverS3    = "s3"
	DriverBFSS  = "bfss"

	StatusActive   = "active"
	StatusDisabled = "disabled"

	TestStatusUnknown = "unknown"
	TestStatusPassed  = "passed"
	TestStatusFailed  = "failed"

	DeliveryModeProxy     = "proxy"
	DeliveryModePresigned = "presigned"
)

type Config struct {
	ID                 int64      `json:"id"`
	Code               string     `json:"code"`
	Name               string     `json:"name"`
	Driver             string     `json:"driver"`
	Endpoint           string     `json:"endpoint,omitempty"`
	Region             string     `json:"region,omitempty"`
	Bucket             string     `json:"bucket"`
	Prefix             string     `json:"prefix,omitempty"`
	ForcePathStyle     bool       `json:"force_path_style"`
	AccessKeyIDSet     bool       `json:"access_key_id_set"`
	SecretAccessKeySet bool       `json:"secret_access_key_set"`
	Status             string     `json:"status"`
	IsDefaultWrite     bool       `json:"is_default_write"`
	LastTestStatus     string     `json:"last_test_status"`
	LastTestError      string     `json:"last_test_error,omitempty"`
	LastTestedAt       *time.Time `json:"last_tested_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type ConfigSecret struct {
	AccessKeyID     string
	SecretAccessKey string
}

type ConfigWithSecret struct {
	Config
	Secret ConfigSecret
}

type ConfigWriteRequest struct {
	Code            string `json:"code"`
	Name            string `json:"name"`
	Driver          string `json:"driver"`
	Endpoint        string `json:"endpoint"`
	Region          string `json:"region"`
	Bucket          string `json:"bucket"`
	Prefix          string `json:"prefix"`
	ForcePathStyle  bool   `json:"force_path_style"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	Status          string `json:"status"`
}

type ConfigListRequest struct {
	Driver string
	Status string
}

type ConfigPage struct {
	Items []Config `json:"items"`
}

type StatsItem struct {
	StorageConfigID      *int64     `json:"storage_config_id,omitempty"`
	StorageCode          string     `json:"storage_code"`
	Driver               string     `json:"driver"`
	Bucket               string     `json:"bucket"`
	ImageCount           int64      `json:"image_count"`
	GeneratedImageCount  int64      `json:"generated_image_count"`
	ReferenceAssetCount  int64      `json:"reference_asset_count"`
	AvatarCount          int64      `json:"avatar_count"`
	TotalBytes           int64      `json:"total_bytes"`
	LastWrittenAt        *time.Time `json:"last_written_at,omitempty"`
	LegacyStorageDriver  string     `json:"legacy_storage_driver,omitempty"`
	LegacyStorageRootKey string     `json:"legacy_storage_root_key,omitempty"`
}

type StatsPage struct {
	Items []StatsItem `json:"items"`
}

type TestResult struct {
	Status    string    `json:"status"`
	LatencyMS int64     `json:"latency_ms"`
	Error     string    `json:"error,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

type AccessURL struct {
	ImageID      string     `json:"image_id"`
	AssetURL     string     `json:"asset_url"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	DeliveryMode string     `json:"delivery_mode"`
}

type MigrationScope struct {
	ObjectRoles   []string   `json:"object_roles,omitempty"`
	CreatedBefore *time.Time `json:"created_before,omitempty"`
}

type MigrationCreateRequest struct {
	SourceStorageConfigID *int64         `json:"source_storage_config_id,omitempty"`
	TargetStorageConfigID int64          `json:"target_storage_config_id"`
	Scope                 MigrationScope `json:"scope"`
	DryRun                bool           `json:"dry_run"`
	UpdateRecords         bool           `json:"update_records"`
}

type MigrationJob struct {
	JobID                 string         `json:"job_id"`
	SourceStorageConfigID *int64         `json:"source_storage_config_id,omitempty"`
	TargetStorageConfigID int64          `json:"target_storage_config_id"`
	Scope                 MigrationScope `json:"scope"`
	DryRun                bool           `json:"dry_run"`
	UpdateRecords         bool           `json:"update_records"`
	Status                string         `json:"status"`
	TotalItems            int64          `json:"total_items"`
	ProcessedItems        int64          `json:"processed_items"`
	FailedItems           int64          `json:"failed_items"`
	TotalBytes            int64          `json:"total_bytes"`
	CreatedAt             time.Time      `json:"created_at"`
	StartedAt             *time.Time     `json:"started_at,omitempty"`
	FinishedAt            *time.Time     `json:"finished_at,omitempty"`
}

type MigrationItem struct {
	ID                    string `json:"id"`
	JobID                 string `json:"job_id"`
	ObjectKind            string `json:"object_kind"`
	ObjectID              string `json:"object_id"`
	SourceStorageConfigID *int64 `json:"source_storage_config_id,omitempty"`
	SourceObjectKey       string `json:"source_object_key"`
	TargetStorageConfigID int64  `json:"target_storage_config_id"`
	TargetObjectKey       string `json:"target_object_key,omitempty"`
	SizeBytes             int64  `json:"size_bytes"`
	Status                string `json:"status"`
	Error                 string `json:"error,omitempty"`
}

type MigrationResult struct {
	Job   MigrationJob    `json:"job"`
	Items []MigrationItem `json:"items,omitempty"`
}

type ObjectLocation struct {
	StorageConfigID *int64
	LegacyDriver    string
	ObjectKey       string
}
