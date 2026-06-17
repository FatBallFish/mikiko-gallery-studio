package entstore

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminstorage "github.com/fatballfish/pic-gallery/internal/domain/adminstorage"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imageresult"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/referenceasset"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/storageconfig"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/internal/service/secretcodec"
	"github.com/fatballfish/pic-gallery/internal/storage"
)

type StorageStore struct {
	client    *repoent.Client
	codec     *secretcodec.Codec
	legacyCfg config.StorageConfig
}

func NewStorageStore(client *repoent.Client) *StorageStore {
	return NewStorageStoreWithEncryptionKey(client, "")
}

func NewStorageStoreWithEncryptionKey(client *repoent.Client, key string) *StorageStore {
	return NewStorageStoreWithLegacyConfig(client, key, config.StorageConfig{})
}

func NewStorageStoreWithLegacyConfig(client *repoent.Client, key string, legacyCfg config.StorageConfig) *StorageStore {
	return &StorageStore{client: client, codec: secretcodec.New(key), legacyCfg: legacyCfg}
}

func (s *StorageStore) ListConfigs(ctx context.Context, req domainadminstorage.ConfigListRequest) ([]domainadminstorage.Config, error) {
	query := s.client.StorageConfig.Query().Order(repoent.Desc(storageconfig.FieldIsDefaultWrite), repoent.Asc(storageconfig.FieldID))
	if req.Driver != "" {
		query.Where(storageconfig.DriverEQ(req.Driver))
	}
	if req.Status != "" {
		query.Where(storageconfig.StatusEQ(req.Status))
	}
	entities, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]domainadminstorage.Config, 0, len(entities))
	for _, entity := range entities {
		items = append(items, s.mapStorageConfigEntity(entity).Config)
	}
	return items, nil
}

func (s *StorageStore) GetConfig(ctx context.Context, id int64) (domainadminstorage.ConfigWithSecret, error) {
	entity, err := s.client.StorageConfig.Get(ctx, int(id))
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainadminstorage.ConfigWithSecret{}, repoerr.ErrNotFound
		}
		return domainadminstorage.ConfigWithSecret{}, err
	}
	return s.mapStorageConfigEntity(entity), nil
}

func (s *StorageStore) GetDefaultWriteConfig(ctx context.Context) (domainadminstorage.ConfigWithSecret, error) {
	entity, err := s.client.StorageConfig.Query().
		Where(storageconfig.IsDefaultWriteEQ(true), storageconfig.StatusEQ(domainadminstorage.StatusActive)).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainadminstorage.ConfigWithSecret{}, repoerr.ErrNotFound
		}
		return domainadminstorage.ConfigWithSecret{}, err
	}
	return s.mapStorageConfigEntity(entity), nil
}

func (s *StorageStore) CreateConfig(ctx context.Context, req domainadminstorage.ConfigWriteRequest) (domainadminstorage.Config, error) {
	accessKey, secretKey, err := s.encryptSecrets(req.AccessKeyID, req.SecretAccessKey)
	if err != nil {
		return domainadminstorage.Config{}, err
	}
	entity, err := s.client.StorageConfig.Create().
		SetCode(req.Code).
		SetName(req.Name).
		SetDriver(req.Driver).
		SetEndpoint(req.Endpoint).
		SetRegion(req.Region).
		SetBucket(req.Bucket).
		SetPrefix(req.Prefix).
		SetForcePathStyle(req.ForcePathStyle).
		SetAccessKeyIDEncrypted(accessKey).
		SetSecretAccessKeyEncrypted(secretKey).
		SetStatus(req.Status).
		SetLastTestStatus(domainadminstorage.TestStatusUnknown).
		Save(ctx)
	if err != nil {
		return domainadminstorage.Config{}, err
	}
	return s.mapStorageConfigEntity(entity).Config, nil
}

func (s *StorageStore) UpdateConfig(ctx context.Context, id int64, req domainadminstorage.ConfigWriteRequest) (domainadminstorage.Config, error) {
	update := s.client.StorageConfig.UpdateOneID(int(id)).
		SetCode(req.Code).
		SetName(req.Name).
		SetDriver(req.Driver).
		SetEndpoint(req.Endpoint).
		SetRegion(req.Region).
		SetBucket(req.Bucket).
		SetPrefix(req.Prefix).
		SetForcePathStyle(req.ForcePathStyle).
		SetStatus(req.Status)
	if req.AccessKeyID != "" {
		accessKey, _, err := s.encryptSecrets(req.AccessKeyID, "")
		if err != nil {
			return domainadminstorage.Config{}, err
		}
		update.SetAccessKeyIDEncrypted(accessKey)
	}
	if req.SecretAccessKey != "" {
		_, secretKey, err := s.encryptSecrets("", req.SecretAccessKey)
		if err != nil {
			return domainadminstorage.Config{}, err
		}
		update.SetSecretAccessKeyEncrypted(secretKey)
	}
	entity, err := update.Save(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainadminstorage.Config{}, repoerr.ErrNotFound
		}
		return domainadminstorage.Config{}, err
	}
	return s.mapStorageConfigEntity(entity).Config, nil
}

func (s *StorageStore) SetDefaultWrite(ctx context.Context, id int64) (domainadminstorage.Config, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return domainadminstorage.Config{}, err
	}
	defer func() { _ = tx.Rollback() }()
	entity, err := tx.StorageConfig.Get(ctx, int(id))
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainadminstorage.Config{}, repoerr.ErrNotFound
		}
		return domainadminstorage.Config{}, err
	}
	if _, err := tx.StorageConfig.Update().SetIsDefaultWrite(false).Save(ctx); err != nil {
		return domainadminstorage.Config{}, err
	}
	entity, err = tx.StorageConfig.UpdateOneID(entity.ID).SetIsDefaultWrite(true).Save(ctx)
	if err != nil {
		return domainadminstorage.Config{}, err
	}
	if err := tx.Commit(); err != nil {
		return domainadminstorage.Config{}, err
	}
	return s.mapStorageConfigEntity(entity).Config, nil
}

func (s *StorageStore) UpdateTestResult(ctx context.Context, id int64, result domainadminstorage.TestResult) (domainadminstorage.Config, error) {
	entity, err := s.client.StorageConfig.UpdateOneID(int(id)).
		SetLastTestStatus(result.Status).
		SetLastTestError(result.Error).
		SetLastTestedAt(result.CheckedAt).
		Save(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainadminstorage.Config{}, repoerr.ErrNotFound
		}
		return domainadminstorage.Config{}, err
	}
	return s.mapStorageConfigEntity(entity).Config, nil
}

func (s *StorageStore) Stats(ctx context.Context) ([]domainadminstorage.StatsItem, error) {
	imageRows, err := s.client.ImageResult.Query().Where(imageresult.DeletedAtIsNil()).All(ctx)
	if err != nil {
		return nil, err
	}
	assetRows, err := s.client.ReferenceAsset.Query().Where(referenceasset.DeletedAtIsNil()).All(ctx)
	if err != nil {
		return nil, err
	}
	configRows, err := s.client.StorageConfig.Query().All(ctx)
	if err != nil {
		return nil, err
	}
	configByID := map[int64]domainadminstorage.Config{}
	for _, row := range configRows {
		cfg := s.mapStorageConfigEntity(row).Config
		configByID[cfg.ID] = cfg
	}
	statsByKey := map[string]*domainadminstorage.StatsItem{}
	for _, row := range imageRows {
		stats := storageStatsBucket(statsByKey, configByID, row.StorageConfigID, row.StorageDriver)
		stats.ImageCount++
		stats.GeneratedImageCount++
		stats.TotalBytes += row.FileSizeBytes
		updateLastWrittenAt(stats, row.CreatedAt)
	}
	for _, row := range assetRows {
		stats := storageStatsBucket(statsByKey, configByID, row.StorageConfigID, row.StorageDriver)
		stats.ImageCount++
		stats.ReferenceAssetCount++
		stats.TotalBytes += row.FileSizeBytes
		updateLastWrittenAt(stats, row.CreatedAt)
	}
	items := make([]domainadminstorage.StatsItem, 0, len(statsByKey))
	for _, item := range statsByKey {
		items = append(items, *item)
	}
	return items, nil
}

func (s *StorageStore) CreateMigration(ctx context.Context, req domainadminstorage.MigrationCreateRequest, createdBy int64) (domainadminstorage.MigrationResult, error) {
	targetCfg, err := s.GetConfig(ctx, req.TargetStorageConfigID)
	if err != nil {
		return domainadminstorage.MigrationResult{}, err
	}
	targetBackend, err := storage.NewBackend(storageConfigFromStoredConfig(targetCfg))
	if err != nil {
		return domainadminstorage.MigrationResult{}, err
	}
	var sourceBackend storage.Backend
	if req.SourceStorageConfigID != nil {
		sourceCfg, err := s.GetConfig(ctx, *req.SourceStorageConfigID)
		if err != nil {
			return domainadminstorage.MigrationResult{}, err
		}
		sourceBackend, err = storage.NewBackend(storageConfigFromStoredConfig(sourceCfg))
		if err != nil {
			return domainadminstorage.MigrationResult{}, err
		}
	} else {
		sourceBackend, err = storage.NewBackend(s.legacyCfg)
		if err != nil {
			return domainadminstorage.MigrationResult{}, err
		}
	}

	items, err := s.scanMigrationItems(ctx, req)
	if err != nil {
		return domainadminstorage.MigrationResult{}, err
	}
	now := time.Now().UTC()
	jobID := uuid.New()
	scopeValue := map[string]any{}
	if raw, err := json.Marshal(req.Scope); err == nil {
		_ = json.Unmarshal(raw, &scopeValue)
	}
	totalBytes := int64(0)
	for _, item := range items {
		totalBytes += item.SizeBytes
	}
	jobEntity, err := s.client.StorageMigrationJob.Create().
		SetID(jobID).
		SetNillableSourceStorageConfigID(req.SourceStorageConfigID).
		SetTargetStorageConfigID(req.TargetStorageConfigID).
		SetScope(scopeValue).
		SetDryRun(req.DryRun).
		SetUpdateRecords(req.UpdateRecords).
		SetStatus("running").
		SetTotalItems(int64(len(items))).
		SetTotalBytes(totalBytes).
		SetCreatedBy(createdBy).
		SetStartedAt(now).
		Save(ctx)
	if err != nil {
		return domainadminstorage.MigrationResult{}, err
	}
	resultItems := make([]domainadminstorage.MigrationItem, 0, len(items))
	processed := int64(0)
	failed := int64(0)
	for _, item := range items {
		item.JobID = jobID.String()
		item.TargetStorageConfigID = req.TargetStorageConfigID
		targetKey := migrationTargetKey(targetCfg.Prefix, item.SourceObjectKey)
		item.TargetObjectKey = targetKey
		createdItem, createErr := s.client.StorageMigrationItem.Create().
			SetJobID(jobID).
			SetObjectKind(item.ObjectKind).
			SetObjectID(uuid.MustParse(item.ObjectID)).
			SetSourceObjectKey(item.SourceObjectKey).
			SetTargetObjectKey(targetKey).
			SetSizeBytes(item.SizeBytes).
			SetStatus("copying").
			Save(ctx)
		if createErr == nil {
			item.ID = strconv.Itoa(createdItem.ID)
		} else {
			item.ID = uuid.NewString()
		}
		copyErr := s.copyMigrationItem(ctx, req, sourceBackend, targetBackend, item)
		status := "succeeded"
		if copyErr != nil {
			status = "failed"
			item.Error = copyErr.Error()
			failed++
		} else {
			processed++
		}
		item.Status = status
		if createErr == nil {
			update := s.client.StorageMigrationItem.UpdateOneID(createdItem.ID).SetStatus(status).SetTargetObjectKey(targetKey)
			if copyErr != nil {
				update.SetError(copyErr.Error())
			} else {
				t := time.Now().UTC()
				update.SetCopiedAt(t)
				if !req.DryRun && req.UpdateRecords {
					update.SetRecordUpdatedAt(t)
				}
			}
			_, _ = update.Save(ctx)
		}
		resultItems = append(resultItems, item)
	}
	finishedAt := time.Now().UTC()
	status := "succeeded"
	if failed > 0 {
		status = "failed"
	}
	jobEntity, err = s.client.StorageMigrationJob.UpdateOneID(jobEntity.ID).
		SetStatus(status).
		SetProcessedItems(processed).
		SetFailedItems(failed).
		SetFinishedAt(finishedAt).
		Save(ctx)
	if err != nil {
		return domainadminstorage.MigrationResult{}, err
	}
	return domainadminstorage.MigrationResult{Job: mapMigrationJob(jobEntity), Items: resultItems}, nil
}

func (s *StorageStore) mapStorageConfigEntity(entity *repoent.StorageConfig) domainadminstorage.ConfigWithSecret {
	return mapStorageConfigEntityWithCodec(entity, s.codec)
}

func mapStorageConfigEntityWithCodec(entity *repoent.StorageConfig, codec *secretcodec.Codec) domainadminstorage.ConfigWithSecret {
	if entity == nil {
		return domainadminstorage.ConfigWithSecret{}
	}
	accessKey := decryptStorageSecret(codec, entity.AccessKeyIDEncrypted, "access_key_id")
	secretKey := decryptStorageSecret(codec, entity.SecretAccessKeyEncrypted, "secret_access_key")
	return domainadminstorage.ConfigWithSecret{
		Config: domainadminstorage.Config{
			ID:                 int64(entity.ID),
			Code:               entity.Code,
			Name:               entity.Name,
			Driver:             entity.Driver,
			Endpoint:           entity.Endpoint,
			Region:             entity.Region,
			Bucket:             entity.Bucket,
			Prefix:             entity.Prefix,
			ForcePathStyle:     entity.ForcePathStyle,
			AccessKeyIDSet:     accessKey != "",
			SecretAccessKeySet: secretKey != "",
			Status:             entity.Status,
			IsDefaultWrite:     entity.IsDefaultWrite,
			LastTestStatus:     entity.LastTestStatus,
			LastTestError:      entity.LastTestError,
			LastTestedAt:       entity.LastTestedAt,
			CreatedAt:          entity.CreatedAt,
			UpdatedAt:          entity.UpdatedAt,
		},
		Secret: domainadminstorage.ConfigSecret{
			AccessKeyID:     accessKey,
			SecretAccessKey: secretKey,
		},
	}
}

func (s *StorageStore) encryptSecrets(accessKeyID, secretAccessKey string) (string, string, error) {
	accessEnvelope, err := s.codec.EncryptJSON(map[string]any{"access_key_id": accessKeyID})
	if err != nil {
		return "", "", err
	}
	secretEnvelope, err := s.codec.EncryptJSON(map[string]any{"secret_access_key": secretAccessKey})
	if err != nil {
		return "", "", err
	}
	return fmt.Sprint(accessEnvelope["ciphertext"]), fmt.Sprint(secretEnvelope["ciphertext"]), nil
}

func decryptStorageSecret(codec *secretcodec.Codec, value string, key string) string {
	if value == "" {
		return ""
	}
	decoded, err := codec.DecryptJSON(map[string]any{"ciphertext": value})
	if err == nil {
		if raw, ok := decoded[key]; ok {
			return strings.TrimSpace(fmt.Sprint(raw))
		}
	}
	return value
}

func storageStatsBucket(items map[string]*domainadminstorage.StatsItem, configs map[int64]domainadminstorage.Config, storageConfigID *int64, legacyDriver string) *domainadminstorage.StatsItem {
	if storageConfigID != nil {
		id := *storageConfigID
		key := "config:" + strconv.FormatInt(id, 10)
		if item, ok := items[key]; ok {
			return item
		}
		cfg := configs[id]
		item := &domainadminstorage.StatsItem{
			StorageConfigID: &id,
			StorageCode:     cfg.Code,
			Driver:          cfg.Driver,
			Bucket:          cfg.Bucket,
		}
		items[key] = item
		return item
	}
	key := "legacy:" + legacyDriver
	if item, ok := items[key]; ok {
		return item
	}
	item := &domainadminstorage.StatsItem{StorageCode: "legacy-default", Driver: legacyDriver, Bucket: legacyDriver, LegacyStorageDriver: legacyDriver}
	items[key] = item
	return item
}

func updateLastWrittenAt(item *domainadminstorage.StatsItem, value time.Time) {
	if item.LastWrittenAt == nil || value.After(*item.LastWrittenAt) {
		v := value
		item.LastWrittenAt = &v
	}
}

func (s *StorageStore) scanMigrationItems(ctx context.Context, req domainadminstorage.MigrationCreateRequest) ([]domainadminstorage.MigrationItem, error) {
	items := []domainadminstorage.MigrationItem{}
	includeGenerated := migrationScopeIncludes(req.Scope, "generated_image")
	includeReference := migrationScopeIncludes(req.Scope, "reference_asset")
	if includeGenerated {
		query := s.client.ImageResult.Query().Where(imageresult.DeletedAtIsNil())
		if req.SourceStorageConfigID == nil {
			query.Where(imageresult.StorageConfigIDIsNil())
		} else {
			query.Where(imageresult.StorageConfigIDEQ(*req.SourceStorageConfigID))
		}
		if req.Scope.CreatedBefore != nil {
			query.Where(imageresult.CreatedAtLT(*req.Scope.CreatedBefore))
		}
		rows, err := query.All(ctx)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			items = append(items, domainadminstorage.MigrationItem{
				ObjectKind:            "task_image",
				ObjectID:              row.ID.String(),
				SourceStorageConfigID: row.StorageConfigID,
				SourceObjectKey:       row.ObjectKey,
				SizeBytes:             row.FileSizeBytes,
				Status:                "pending",
			})
		}
	}
	if includeReference {
		query := s.client.ReferenceAsset.Query().Where(referenceasset.DeletedAtIsNil())
		if req.SourceStorageConfigID == nil {
			query.Where(referenceasset.StorageConfigIDIsNil())
		} else {
			query.Where(referenceasset.StorageConfigIDEQ(*req.SourceStorageConfigID))
		}
		if req.Scope.CreatedBefore != nil {
			query.Where(referenceasset.CreatedAtLT(*req.Scope.CreatedBefore))
		}
		rows, err := query.All(ctx)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			items = append(items, domainadminstorage.MigrationItem{
				ObjectKind:            "reference_asset",
				ObjectID:              row.ID.String(),
				SourceStorageConfigID: row.StorageConfigID,
				SourceObjectKey:       row.ObjectKey,
				SizeBytes:             row.FileSizeBytes,
				Status:                "pending",
			})
		}
	}
	return items, nil
}

func (s *StorageStore) copyMigrationItem(ctx context.Context, req domainadminstorage.MigrationCreateRequest, source storage.Backend, target storage.Backend, item domainadminstorage.MigrationItem) error {
	content, err := source.Get(ctx, item.SourceObjectKey)
	if err != nil {
		return err
	}
	if err := target.Put(ctx, item.TargetObjectKey, "", content); err != nil {
		return err
	}
	if req.DryRun {
		_ = target.Delete(ctx, item.TargetObjectKey)
		return nil
	}
	if !req.UpdateRecords {
		return nil
	}
	targetID := req.TargetStorageConfigID
	switch item.ObjectKind {
	case "task_image":
		objectID, err := uuid.Parse(item.ObjectID)
		if err != nil {
			return err
		}
		return s.client.ImageResult.UpdateOneID(objectID).
			SetStorageConfigID(targetID).
			SetStorageDriver(target.Driver()).
			SetObjectKey(item.TargetObjectKey).
			Exec(ctx)
	case "reference_asset":
		objectID, err := uuid.Parse(item.ObjectID)
		if err != nil {
			return err
		}
		return s.client.ReferenceAsset.UpdateOneID(objectID).
			SetStorageConfigID(targetID).
			SetStorageDriver(target.Driver()).
			SetObjectKey(item.TargetObjectKey).
			Exec(ctx)
	default:
		return fmt.Errorf("unsupported migration object kind %q", item.ObjectKind)
	}
}

func migrationScopeIncludes(scope domainadminstorage.MigrationScope, role string) bool {
	if len(scope.ObjectRoles) == 0 {
		return true
	}
	for _, item := range scope.ObjectRoles {
		switch strings.TrimSpace(item) {
		case role:
			return true
		case "generated_image":
			if role == "task_image" {
				return true
			}
		case "task_image":
			if role == "generated_image" {
				return true
			}
		}
	}
	return false
}

func migrationTargetKey(prefix string, sourceKey string) string {
	sourceKey = strings.TrimPrefix(path.Clean(strings.TrimSpace(sourceKey)), "/")
	if strings.TrimSpace(prefix) == "" {
		return sourceKey
	}
	return path.Join(strings.Trim(strings.TrimSpace(prefix), "/"), sourceKey)
}

func storageConfigFromStoredConfig(cfg domainadminstorage.ConfigWithSecret) config.StorageConfig {
	driver := cfg.Driver
	if strings.EqualFold(driver, domainadminstorage.DriverBFSS) {
		driver = domainadminstorage.DriverS3
	}
	return config.StorageConfig{
		Driver:        driver,
		LocalRoot:     cfg.Bucket,
		SharedVolume:  true,
		PublicBaseURL: "",
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

func mapMigrationJob(row *repoent.StorageMigrationJob) domainadminstorage.MigrationJob {
	if row == nil {
		return domainadminstorage.MigrationJob{}
	}
	scope := domainadminstorage.MigrationScope{}
	if raw, err := json.Marshal(row.Scope); err == nil {
		_ = json.Unmarshal(raw, &scope)
	}
	return domainadminstorage.MigrationJob{
		JobID:                 row.ID.String(),
		SourceStorageConfigID: row.SourceStorageConfigID,
		TargetStorageConfigID: valueInt64(row.TargetStorageConfigID),
		Scope:                 scope,
		DryRun:                row.DryRun,
		UpdateRecords:         row.UpdateRecords,
		Status:                row.Status,
		TotalItems:            row.TotalItems,
		ProcessedItems:        row.ProcessedItems,
		FailedItems:           row.FailedItems,
		CreatedAt:             row.CreatedAt,
		StartedAt:             row.StartedAt,
		FinishedAt:            row.FinishedAt,
	}
}

func valueInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
