package adminstorage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminstorage "github.com/fatballfish/pic-gallery/internal/domain/adminstorage"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type Service struct {
	cfg   config.StorageConfig
	store Store
}

func NewService(cfg config.StorageConfig) *Service {
	return NewServiceWithStore(cfg, NewMemoryStore())
}

func NewServiceWithStore(cfg config.StorageConfig, store Store) *Service {
	if store == nil {
		store = NewMemoryStore()
	}
	return &Service{cfg: cfg, store: store}
}

func (s *Service) ListConfigs(ctx context.Context, req domainadminstorage.ConfigListRequest) (domainadminstorage.ConfigPage, error) {
	items, err := s.store.ListConfigs(ctx, req)
	if err != nil {
		return domainadminstorage.ConfigPage{}, errs.Internal("failed to list storage configs")
	}
	items = append(legacyBootstrapConfig(s.cfg), items...)
	return domainadminstorage.ConfigPage{Items: items}, nil
}

func (s *Service) CreateConfig(ctx context.Context, req domainadminstorage.ConfigWriteRequest) (domainadminstorage.Config, error) {
	normalized, appErr := normalizeWriteRequest(req)
	if appErr != nil {
		return domainadminstorage.Config{}, appErr
	}
	created, err := s.store.CreateConfig(ctx, normalized)
	if err != nil {
		return domainadminstorage.Config{}, errs.Internal("failed to create storage config")
	}
	return created, nil
}

func (s *Service) UpdateConfig(ctx context.Context, id int64, req domainadminstorage.ConfigWriteRequest) (domainadminstorage.Config, error) {
	normalized, appErr := normalizeWriteRequest(req)
	if appErr != nil {
		return domainadminstorage.Config{}, appErr
	}
	updated, err := s.store.UpdateConfig(ctx, id, normalized)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return domainadminstorage.Config{}, errs.New(404, errs.CodeNotFound, "storage config not found")
		}
		return domainadminstorage.Config{}, errs.Internal("failed to update storage config")
	}
	return updated, nil
}

func (s *Service) SetDefaultWrite(ctx context.Context, id int64) (domainadminstorage.Config, error) {
	cfg, err := s.store.GetConfig(ctx, id)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return domainadminstorage.Config{}, errs.New(404, errs.CodeNotFound, "storage config not found")
		}
		return domainadminstorage.Config{}, errs.Internal("failed to load storage config")
	}
	if cfg.Status != domainadminstorage.StatusActive {
		return domainadminstorage.Config{}, errs.New(409, errs.CodeConflict, "storage config must be active before becoming default")
	}
	if cfg.Driver == domainadminstorage.DriverS3 && cfg.LastTestStatus != domainadminstorage.TestStatusPassed {
		return domainadminstorage.Config{}, errs.New(409, errs.CodeConflict, "storage config must pass connection test before becoming default")
	}
	updated, err := s.store.SetDefaultWrite(ctx, id)
	if err != nil {
		return domainadminstorage.Config{}, errs.Internal("failed to set default storage config")
	}
	return updated, nil
}

func (s *Service) TestConfig(ctx context.Context, id int64) (domainadminstorage.TestResult, error) {
	cfg, err := s.store.GetConfig(ctx, id)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return domainadminstorage.TestResult{}, errs.New(404, errs.CodeNotFound, "storage config not found")
		}
		return domainadminstorage.TestResult{}, errs.Internal("failed to load storage config")
	}
	start := time.Now()
	result := domainadminstorage.TestResult{Status: domainadminstorage.TestStatusPassed, CheckedAt: time.Now().UTC()}
	backend, backendErr := storage.NewBackend(storageConfigFromAdminConfig(cfg))
	if backendErr != nil {
		result.Status = domainadminstorage.TestStatusFailed
		result.Error = backendErr.Error()
	} else {
		key := fmt.Sprintf("_storage-tests/%d-%d.txt", cfg.ID, result.CheckedAt.UnixNano())
		if err := backend.Put(ctx, key, "text/plain", []byte("ok")); err != nil {
			result.Status = domainadminstorage.TestStatusFailed
			result.Error = err.Error()
		} else if _, err := backend.Get(ctx, key); err != nil {
			result.Status = domainadminstorage.TestStatusFailed
			result.Error = err.Error()
		} else if err := backend.Delete(ctx, key); err != nil && !errors.Is(err, storage.ErrNotFound) {
			result.Status = domainadminstorage.TestStatusFailed
			result.Error = err.Error()
		}
	}
	result.LatencyMS = time.Since(start).Milliseconds()
	if _, err := s.store.UpdateTestResult(ctx, id, result); err != nil {
		return domainadminstorage.TestResult{}, errs.Internal("failed to save storage test result")
	}
	if result.Status == domainadminstorage.TestStatusFailed {
		return result, errs.New(409, errs.CodeConflict, "storage config test failed")
	}
	return result, nil
}

func (s *Service) Stats(ctx context.Context) (domainadminstorage.StatsPage, error) {
	items, err := s.store.Stats(ctx)
	if err != nil {
		return domainadminstorage.StatsPage{}, errs.Internal("failed to load storage stats")
	}
	if len(items) == 0 {
		items = []domainadminstorage.StatsItem{legacyStatsPlaceholder(s.cfg)}
	}
	return domainadminstorage.StatsPage{Items: items}, nil
}

func (s *Service) CreateMigration(ctx context.Context, req domainadminstorage.MigrationCreateRequest, createdBy int64) (domainadminstorage.MigrationResult, error) {
	if req.TargetStorageConfigID <= 0 {
		return domainadminstorage.MigrationResult{}, errs.BadRequest("target_storage_config_id is required")
	}
	if req.SourceStorageConfigID != nil && *req.SourceStorageConfigID == req.TargetStorageConfigID {
		return domainadminstorage.MigrationResult{}, errs.BadRequest("source and target storage must be different")
	}
	target, err := s.store.GetConfig(ctx, req.TargetStorageConfigID)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return domainadminstorage.MigrationResult{}, errs.New(404, errs.CodeNotFound, "target storage config not found")
		}
		return domainadminstorage.MigrationResult{}, errs.Internal("failed to load target storage config")
	}
	if target.Status != domainadminstorage.StatusActive {
		return domainadminstorage.MigrationResult{}, errs.New(409, errs.CodeConflict, "target storage config must be active")
	}
	if req.SourceStorageConfigID != nil {
		if _, err := s.store.GetConfig(ctx, *req.SourceStorageConfigID); err != nil {
			if errors.Is(err, repoerr.ErrNotFound) {
				return domainadminstorage.MigrationResult{}, errs.New(404, errs.CodeNotFound, "source storage config not found")
			}
			return domainadminstorage.MigrationResult{}, errs.Internal("failed to load source storage config")
		}
	}
	result, err := s.store.CreateMigration(ctx, req, createdBy)
	if err != nil {
		return domainadminstorage.MigrationResult{}, errs.Internal("failed to create storage migration")
	}
	return result, nil
}

func (s *Service) ResolveWriteBackend(ctx context.Context) (storage.ResolvedBackend, error) {
	cfg, err := s.store.GetDefaultWriteConfig(ctx)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			backend, backendErr := storage.NewBackend(s.cfg)
			if backendErr != nil {
				return storage.ResolvedBackend{}, backendErr
			}
			return storage.ResolvedBackend{Driver: backend.Driver(), Backend: backend}, nil
		}
		return storage.ResolvedBackend{}, err
	}
	backend, err := storage.NewBackend(storageConfigFromAdminConfig(cfg))
	if err != nil {
		return storage.ResolvedBackend{}, err
	}
	id := cfg.ID
	return storage.ResolvedBackend{StorageConfigID: &id, Driver: backend.Driver(), Backend: backend}, nil
}

func (s *Service) ResolveReadBackend(ctx context.Context, loc domainadminstorage.ObjectLocation) (storage.ResolvedBackend, error) {
	if loc.StorageConfigID == nil {
		backend, err := storage.NewBackend(s.cfg)
		if err != nil {
			return storage.ResolvedBackend{}, err
		}
		return storage.ResolvedBackend{Driver: backend.Driver(), Backend: backend}, nil
	}
	cfg, err := s.store.GetConfig(ctx, *loc.StorageConfigID)
	if err != nil {
		return storage.ResolvedBackend{}, err
	}
	backend, err := storage.NewBackend(storageConfigFromAdminConfig(cfg))
	if err != nil {
		return storage.ResolvedBackend{}, err
	}
	id := cfg.ID
	return storage.ResolvedBackend{StorageConfigID: &id, Driver: backend.Driver(), Backend: backend}, nil
}

func (s *Service) BuildAccessURL(ctx context.Context, imageID string, loc domainadminstorage.ObjectLocation, ttl time.Duration) domainadminstorage.AccessURL {
	access := s.BuildLegacyProxyAccessURL(imageID)
	if strings.TrimSpace(loc.ObjectKey) == "" {
		return access
	}
	resolved, err := s.ResolveReadBackend(ctx, loc)
	if err != nil {
		return access
	}
	presigner, ok := resolved.Backend.(storage.PresignBackend)
	if !ok {
		return access
	}
	url, expiresAt, err := presigner.PresignGet(ctx, loc.ObjectKey, ttl)
	if err != nil {
		return access
	}
	access.AssetURL = url
	access.ExpiresAt = &expiresAt
	access.DeliveryMode = domainadminstorage.DeliveryModePresigned
	return access
}

func (s *Service) BuildLegacyProxyAccessURL(imageID string) domainadminstorage.AccessURL {
	return domainadminstorage.AccessURL{
		ImageID:      imageID,
		AssetURL:     "/api/agent/image/v1/images/" + strings.TrimSpace(imageID),
		DeliveryMode: domainadminstorage.DeliveryModeProxy,
	}
}

func normalizeWriteRequest(req domainadminstorage.ConfigWriteRequest) (domainadminstorage.ConfigWriteRequest, *errs.Error) {
	req.Code = normalizeCode(req.Code)
	req.Name = strings.TrimSpace(req.Name)
	req.Driver = strings.ToLower(strings.TrimSpace(req.Driver))
	req.Endpoint = strings.TrimSpace(req.Endpoint)
	req.Region = strings.TrimSpace(req.Region)
	req.Bucket = strings.TrimSpace(req.Bucket)
	req.Prefix = strings.Trim(strings.TrimSpace(req.Prefix), "/")
	req.AccessKeyID = strings.TrimSpace(req.AccessKeyID)
	req.SecretAccessKey = strings.TrimSpace(req.SecretAccessKey)
	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	if req.Status == "" {
		req.Status = domainadminstorage.StatusActive
	}
	if req.Code == "" {
		return req, errs.BadRequest("storage config code is required")
	}
	if req.Name == "" {
		req.Name = req.Code
	}
	switch req.Driver {
	case domainadminstorage.DriverLocal:
		if req.Bucket == "" {
			req.Bucket = "local"
		}
	case domainadminstorage.DriverS3, domainadminstorage.DriverBFSS:
		if req.Endpoint == "" || req.Region == "" || req.Bucket == "" {
			return req, errs.BadRequest("object storage endpoint, region and bucket are required")
		}
		if req.AccessKeyID == "" || req.SecretAccessKey == "" {
			return req, errs.BadRequest("object storage credentials are required")
		}
	default:
		return req, errs.BadRequest("unsupported storage driver")
	}
	switch req.Status {
	case domainadminstorage.StatusActive, domainadminstorage.StatusDisabled:
	default:
		return req, errs.BadRequest("unsupported storage status")
	}
	return req, nil
}

func storageConfigFromAdminConfig(cfg domainadminstorage.ConfigWithSecret) config.StorageConfig {
	return config.StorageConfig{
		Driver:        storageDriverForBackend(cfg.Driver),
		LocalRoot:     cfg.Bucket,
		PublicBaseURL: "",
		SharedVolume:  true,
		S3: config.StorageS3Config{
			Endpoint:        cfg.Endpoint,
			Region:          cfg.Region,
			Bucket:          cfg.Bucket,
			AccessKeyID:     cfg.Secret.AccessKeyID,
			SecretAccessKey: cfg.Secret.SecretAccessKey,
			ForcePathStyle:  cfg.ForcePathStyle,
			Prefix:          cfg.Prefix,
		},
	}
}

func storageDriverForBackend(driver string) string {
	if strings.EqualFold(strings.TrimSpace(driver), domainadminstorage.DriverBFSS) {
		return domainadminstorage.DriverS3
	}
	return strings.TrimSpace(driver)
}

func legacyBootstrapConfig(cfg config.StorageConfig) []domainadminstorage.Config {
	driver := strings.ToLower(strings.TrimSpace(cfg.Driver))
	if driver == "" {
		driver = domainadminstorage.DriverLocal
	}
	return []domainadminstorage.Config{{
		ID:             0,
		Code:           "legacy-default",
		Name:           "Legacy default storage",
		Driver:         driver,
		Bucket:         legacyBucket(cfg),
		Prefix:         strings.Trim(strings.TrimSpace(cfg.S3.Prefix), "/"),
		ForcePathStyle: cfg.S3.ForcePathStyle,
		Status:         domainadminstorage.StatusActive,
		IsDefaultWrite: true,
		LastTestStatus: domainadminstorage.TestStatusUnknown,
		CreatedAt:      time.Time{},
		UpdatedAt:      time.Time{},
	}}
}

func legacyStatsPlaceholder(cfg config.StorageConfig) domainadminstorage.StatsItem {
	driver := strings.ToLower(strings.TrimSpace(cfg.Driver))
	if driver == "" {
		driver = domainadminstorage.DriverLocal
	}
	return domainadminstorage.StatsItem{
		StorageCode:          "legacy-default",
		Driver:               driver,
		Bucket:               legacyBucket(cfg),
		LegacyStorageDriver:  driver,
		LegacyStorageRootKey: legacyBucket(cfg),
	}
}

func legacyBucket(cfg config.StorageConfig) string {
	if strings.EqualFold(strings.TrimSpace(cfg.Driver), domainadminstorage.DriverS3) {
		return strings.TrimSpace(cfg.S3.Bucket)
	}
	return strings.TrimSpace(cfg.LocalRoot)
}

func normalizeCode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "-")
	return value
}
