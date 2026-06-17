package adminstorage

import (
	"context"
	"sort"
	"sync"
	"time"

	domainadminstorage "github.com/fatballfish/pic-gallery/internal/domain/adminstorage"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
)

type Store interface {
	ListConfigs(ctx context.Context, req domainadminstorage.ConfigListRequest) ([]domainadminstorage.Config, error)
	GetConfig(ctx context.Context, id int64) (domainadminstorage.ConfigWithSecret, error)
	GetDefaultWriteConfig(ctx context.Context) (domainadminstorage.ConfigWithSecret, error)
	CreateConfig(ctx context.Context, req domainadminstorage.ConfigWriteRequest) (domainadminstorage.Config, error)
	UpdateConfig(ctx context.Context, id int64, req domainadminstorage.ConfigWriteRequest) (domainadminstorage.Config, error)
	SetDefaultWrite(ctx context.Context, id int64) (domainadminstorage.Config, error)
	UpdateTestResult(ctx context.Context, id int64, result domainadminstorage.TestResult) (domainadminstorage.Config, error)
	Stats(ctx context.Context) ([]domainadminstorage.StatsItem, error)
	CreateMigration(ctx context.Context, req domainadminstorage.MigrationCreateRequest, createdBy int64) (domainadminstorage.MigrationResult, error)
}

type MemoryStore struct {
	mu      sync.Mutex
	nextID  int64
	configs map[int64]domainadminstorage.ConfigWithSecret
	stats   []domainadminstorage.StatsItem
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{nextID: 1, configs: map[int64]domainadminstorage.ConfigWithSecret{}}
}

func (s *MemoryStore) ListConfigs(_ context.Context, req domainadminstorage.ConfigListRequest) ([]domainadminstorage.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := make([]domainadminstorage.Config, 0, len(s.configs))
	for _, item := range s.configs {
		cfg := item.Config
		if req.Driver != "" && cfg.Driver != req.Driver {
			continue
		}
		if req.Status != "" && cfg.Status != req.Status {
			continue
		}
		items = append(items, cfg)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].IsDefaultWrite != items[j].IsDefaultWrite {
			return items[i].IsDefaultWrite
		}
		return items[i].ID < items[j].ID
	})
	return items, nil
}

func (s *MemoryStore) GetConfig(_ context.Context, id int64) (domainadminstorage.ConfigWithSecret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.configs[id]
	if !ok {
		return domainadminstorage.ConfigWithSecret{}, repoerr.ErrNotFound
	}
	return item, nil
}

func (s *MemoryStore) GetDefaultWriteConfig(ctx context.Context) (domainadminstorage.ConfigWithSecret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.configs {
		if item.IsDefaultWrite {
			return item, nil
		}
	}
	return domainadminstorage.ConfigWithSecret{}, repoerr.ErrNotFound
}

func (s *MemoryStore) CreateConfig(_ context.Context, req domainadminstorage.ConfigWriteRequest) (domainadminstorage.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	id := s.nextID
	s.nextID++
	item := domainadminstorage.ConfigWithSecret{
		Config: domainadminstorage.Config{
			ID:                 id,
			Code:               req.Code,
			Name:               req.Name,
			Driver:             req.Driver,
			Endpoint:           req.Endpoint,
			Region:             req.Region,
			Bucket:             req.Bucket,
			Prefix:             req.Prefix,
			ForcePathStyle:     req.ForcePathStyle,
			AccessKeyIDSet:     req.AccessKeyID != "",
			SecretAccessKeySet: req.SecretAccessKey != "",
			Status:             req.Status,
			LastTestStatus:     domainadminstorage.TestStatusUnknown,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		Secret: domainadminstorage.ConfigSecret{AccessKeyID: req.AccessKeyID, SecretAccessKey: req.SecretAccessKey},
	}
	if len(s.configs) == 0 && item.Status == domainadminstorage.StatusActive {
		item.IsDefaultWrite = true
	}
	s.configs[id] = item
	return item.Config, nil
}

func (s *MemoryStore) UpdateConfig(_ context.Context, id int64, req domainadminstorage.ConfigWriteRequest) (domainadminstorage.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.configs[id]
	if !ok {
		return domainadminstorage.Config{}, repoerr.ErrNotFound
	}
	item.Code = req.Code
	item.Name = req.Name
	item.Driver = req.Driver
	item.Endpoint = req.Endpoint
	item.Region = req.Region
	item.Bucket = req.Bucket
	item.Prefix = req.Prefix
	item.ForcePathStyle = req.ForcePathStyle
	item.Status = req.Status
	if req.AccessKeyID != "" {
		item.Secret.AccessKeyID = req.AccessKeyID
	}
	if req.SecretAccessKey != "" {
		item.Secret.SecretAccessKey = req.SecretAccessKey
	}
	item.AccessKeyIDSet = item.Secret.AccessKeyID != ""
	item.SecretAccessKeySet = item.Secret.SecretAccessKey != ""
	item.UpdatedAt = time.Now().UTC()
	s.configs[id] = item
	return item.Config, nil
}

func (s *MemoryStore) SetDefaultWrite(_ context.Context, id int64) (domainadminstorage.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.configs[id]
	if !ok {
		return domainadminstorage.Config{}, repoerr.ErrNotFound
	}
	for key, cfg := range s.configs {
		cfg.IsDefaultWrite = key == id
		cfg.UpdatedAt = time.Now().UTC()
		s.configs[key] = cfg
	}
	item = s.configs[id]
	return item.Config, nil
}

func (s *MemoryStore) UpdateTestResult(_ context.Context, id int64, result domainadminstorage.TestResult) (domainadminstorage.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.configs[id]
	if !ok {
		return domainadminstorage.Config{}, repoerr.ErrNotFound
	}
	item.LastTestStatus = result.Status
	item.LastTestError = result.Error
	item.LastTestedAt = &result.CheckedAt
	item.UpdatedAt = time.Now().UTC()
	s.configs[id] = item
	return item.Config, nil
}

func (s *MemoryStore) Stats(_ context.Context) ([]domainadminstorage.StatsItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domainadminstorage.StatsItem(nil), s.stats...), nil
}

func (s *MemoryStore) CreateMigration(_ context.Context, req domainadminstorage.MigrationCreateRequest, _ int64) (domainadminstorage.MigrationResult, error) {
	now := time.Now().UTC()
	job := domainadminstorage.MigrationJob{
		JobID:                 "memory-migration",
		SourceStorageConfigID: req.SourceStorageConfigID,
		TargetStorageConfigID: req.TargetStorageConfigID,
		Scope:                 req.Scope,
		DryRun:                req.DryRun,
		UpdateRecords:         req.UpdateRecords,
		Status:                "succeeded",
		CreatedAt:             now,
		StartedAt:             &now,
		FinishedAt:            &now,
	}
	return domainadminstorage.MigrationResult{Job: job}, nil
}

func (s *MemoryStore) SetStatsForTest(items []domainadminstorage.StatsItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats = append([]domainadminstorage.StatsItem(nil), items...)
}
