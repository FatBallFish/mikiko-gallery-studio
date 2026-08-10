package entstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	"github.com/fatballfish/pic-gallery/internal/provider"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imageresult"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imagetask"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/predicate"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/referenceasset"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
)

type AssetsStore struct {
	client *repoent.Client
}

func NewAssetsStore(client *repoent.Client) *AssetsStore {
	return &AssetsStore{client: client}
}

func (s *AssetsStore) GetByUserAndHash(ctx context.Context, userID int64, sha string) (domainassets.ReferenceAsset, error) {
	entity, err := s.client.ReferenceAsset.Query().
		Where(referenceasset.UserIDEQ(userID), referenceasset.Sha256EQ(sha), referenceasset.OwnsObjectEQ(true), referenceasset.StatusNEQ("deleted")).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainassets.ReferenceAsset{}, repoerr.ErrNotFound
		}
		return domainassets.ReferenceAsset{}, err
	}
	return s.ensureReferenceAssetName(ctx, userID, entity)
}

func (s *AssetsStore) Save(ctx context.Context, userID int64, asset domainassets.ReferenceAsset) error {
	return s.SaveWithMetadata(ctx, userID, asset, domainassets.UploadMetadata{APIKeyID: asset.APIKeyID, UploadSource: asset.UploadSource})
}

func (s *AssetsStore) SaveWithMetadata(ctx context.Context, userID int64, asset domainassets.ReferenceAsset, metadata domainassets.UploadMetadata) error {
	_, err := s.saveWithMetadata(ctx, userID, asset, metadata)
	return err
}

func (s *AssetsStore) saveWithMetadata(ctx context.Context, userID int64, asset domainassets.ReferenceAsset, metadata domainassets.UploadMetadata) (domainassets.ReferenceAsset, error) {
	id, err := uuid.Parse(asset.ID)
	if err != nil {
		return domainassets.ReferenceAsset{}, err
	}
	ownsObject := asset.OwnsObject || strings.TrimSpace(asset.SourceImageResultID) == ""
	create := s.client.ReferenceAsset.Create().
		SetID(id).
		SetUserID(userID).
		SetUploadSource(defaultString(metadata.UploadSource, "web")).
		SetStatus(asset.Status).
		SetStorageDriver(defaultString(asset.StorageDriver, "local")).
		SetObjectKey(asset.ObjectKey).
		SetMimeType(asset.MimeType).
		SetFileSizeBytes(asset.FileSizeBytes).
		SetWidth(asset.Width).
		SetHeight(asset.Height).
		SetSha256(asset.SHA256).
		SetOwnsObject(ownsObject).
		SetExpiresAt(asset.CreatedAt.Add(24 * time.Hour))
	if strings.TrimSpace(asset.Name) != "" {
		name, normalizeErr := domainassets.NormalizeReferenceName(asset.Name)
		if normalizeErr != nil {
			return domainassets.ReferenceAsset{}, normalizeErr
		}
		create.SetName(name).SetNameNormalized(name)
	}
	if strings.TrimSpace(asset.SourceImageResultID) != "" {
		sourceID, parseErr := uuid.Parse(asset.SourceImageResultID)
		if parseErr != nil {
			return domainassets.ReferenceAsset{}, parseErr
		}
		create.SetSourceImageResultID(sourceID)
	}
	if asset.StorageConfigID != "" {
		storageConfigID, parseErr := uuid.Parse(asset.StorageConfigID)
		if parseErr != nil {
			return domainassets.ReferenceAsset{}, parseErr
		}
		create.SetStorageConfigID(storageConfigID)
	}
	if metadata.APIKeyID != nil {
		create.SetAPIKeyID(*metadata.APIKeyID)
	}
	if strings.TrimSpace(asset.StorageConfigID) != "" {
		storageConfigID, err := uuid.Parse(asset.StorageConfigID)
		if err != nil {
			return domainassets.ReferenceAsset{}, err
		}
		create.SetStorageConfigID(storageConfigID)
	}
	entity, err := create.Save(ctx)
	if err != nil {
		return domainassets.ReferenceAsset{}, err
	}
	return mapReferenceAssetEntity(entity), nil
}

func (s *AssetsStore) SaveWithGeneratedName(ctx context.Context, userID int64, asset domainassets.ReferenceAsset, metadata domainassets.UploadMetadata, preferredName string) (domainassets.ReferenceAsset, error) {
	if normalized, err := domainassets.NormalizeReferenceName(preferredName); err == nil {
		preferredName = normalized
	} else {
		preferredName = ""
	}
	for sequence := 1; sequence <= 10000; sequence++ {
		asset.Name = domainassets.ReferenceNameCandidate(preferredName, sequence)
		saved, err := s.saveWithMetadata(ctx, userID, asset, metadata)
		if err == nil {
			return saved, nil
		}
		if !repoent.IsConstraintError(err) {
			return domainassets.ReferenceAsset{}, err
		}
	}
	return domainassets.ReferenceAsset{}, repoerr.ErrConflict
}

func (s *AssetsStore) GetByUserAndSourceImageResultID(ctx context.Context, userID int64, sourceImageResultID string) (domainassets.ReferenceAsset, error) {
	sourceID, err := uuid.Parse(sourceImageResultID)
	if err != nil {
		return domainassets.ReferenceAsset{}, repoerr.ErrNotFound
	}
	entity, err := s.client.ReferenceAsset.Query().Where(
		referenceasset.UserIDEQ(userID),
		referenceasset.SourceImageResultIDEQ(sourceID),
		referenceasset.DeletedAtIsNil(),
		referenceasset.StatusNEQ("deleted"),
	).First(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainassets.ReferenceAsset{}, repoerr.ErrNotFound
		}
		return domainassets.ReferenceAsset{}, err
	}
	return s.ensureReferenceAssetName(ctx, userID, entity)
}

func (s *AssetsStore) ImportGalleryAlias(ctx context.Context, userID int64, result provider.ImageResult) (domainassets.ReferenceAsset, error) {
	sourceID, err := uuid.Parse(strings.TrimSpace(result.ID))
	if err != nil {
		return domainassets.ReferenceAsset{}, repoerr.ErrNotFound
	}
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return domainassets.ReferenceAsset{}, err
	}
	defer func() { _ = tx.Rollback() }()
	source, err := tx.ImageResult.Query().Where(
		imageresult.IDEQ(sourceID),
		imageresult.UserIDEQ(userID),
		imageresult.DeletedAtIsNil(),
		lockImageResultForAlias(),
	).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainassets.ReferenceAsset{}, repoerr.ErrNotFound
		}
		return domainassets.ReferenceAsset{}, err
	}
	task, err := tx.ImageTask.Query().Where(
		imagetask.IDEQ(source.TaskID),
		imagetask.UserIDEQ(userID),
		imagetask.DeletedAtIsNil(),
	).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainassets.ReferenceAsset{}, repoerr.ErrNotFound
		}
		return domainassets.ReferenceAsset{}, err
	}
	snapshot, err := referenceGenerationSnapshot(task)
	if err != nil {
		return domainassets.ReferenceAsset{}, err
	}
	if existing, err := tx.ReferenceAsset.Query().Where(
		referenceasset.UserIDEQ(userID),
		referenceasset.SourceImageResultIDEQ(source.ID),
		referenceasset.DeletedAtIsNil(),
		referenceasset.StatusNEQ("deleted"),
	).First(ctx); err == nil {
		if existing.Name == nil {
			name, nameErr := nextReferenceAssetName(ctx, tx.Client(), userID, "")
			if nameErr != nil {
				return domainassets.ReferenceAsset{}, nameErr
			}
			existing, nameErr = tx.ReferenceAsset.UpdateOneID(existing.ID).SetName(name).SetNameNormalized(name).Save(ctx)
			if nameErr != nil {
				if repoent.IsConstraintError(nameErr) {
					return domainassets.ReferenceAsset{}, repoerr.ErrConflict
				}
				return domainassets.ReferenceAsset{}, nameErr
			}
		}
		asset := mapReferenceAssetEntity(existing)
		asset.GenerationSnapshot = snapshot
		if err := tx.Commit(); err != nil {
			return domainassets.ReferenceAsset{}, err
		}
		return asset, nil
	} else if !repoent.IsNotFound(err) {
		return domainassets.ReferenceAsset{}, err
	}
	assetID := uuid.New()
	name, err := nextReferenceAssetName(ctx, tx.Client(), userID, "")
	if err != nil {
		return domainassets.ReferenceAsset{}, err
	}
	create := tx.ReferenceAsset.Create().
		SetID(assetID).
		SetUserID(userID).
		SetUploadSource("gallery_import").
		SetStatus("ready").
		SetStorageDriver(source.StorageDriver).
		SetObjectKey(source.ObjectKey).
		SetMimeType(source.MimeType).
		SetFileSizeBytes(source.FileSizeBytes).
		SetWidth(source.Width).
		SetHeight(source.Height).
		SetSha256(source.Sha256).
		SetSourceImageResultID(source.ID).
		SetOwnsObject(false).
		SetName(name).
		SetNameNormalized(name).
		SetExpiresAt(time.Now().Add(24 * time.Hour))
	if source.StorageConfigID != nil {
		create.SetStorageConfigID(*source.StorageConfigID)
	}
	entity, err := create.Save(ctx)
	if err != nil {
		if repoent.IsConstraintError(err) {
			return domainassets.ReferenceAsset{}, repoerr.ErrConflict
		}
		return domainassets.ReferenceAsset{}, err
	}
	asset := mapReferenceAssetEntity(entity)
	asset.GenerationSnapshot = snapshot
	if err := tx.Commit(); err != nil {
		return domainassets.ReferenceAsset{}, err
	}
	return asset, nil
}

func referenceGenerationSnapshot(task *repoent.ImageTask) (*domainassets.GenerationSnapshot, error) {
	mappedTask, err := mapImageTaskEntity(task, nil)
	if err != nil {
		return nil, err
	}
	return &domainassets.GenerationSnapshot{
		TaskType:          mappedTask.TaskType,
		AbstractModel:     mappedTask.AbstractModel,
		RouteModelCode:    mappedTask.RouteModelCode,
		CapabilityVersion: mappedTask.GenerationSnapshot.CapabilityVersion,
		SizeMode:          mappedTask.SizeMode,
		RequestedSize:     mappedTask.RequestedSize,
		BaseResolution:    mappedTask.BaseResolution,
		AspectRatio:       mappedTask.AspectRatio,
		Quality:           mappedTask.Quality,
		Background:        mappedTask.Background,
		OutputFormat:      mappedTask.OutputFormat,
		OutputCompression: mappedTask.OutputCompression,
		Moderation:        mappedTask.Moderation,
		ImageCount:        mappedTask.OutputImageCount,
	}, nil
}

func lockImageResultForAlias() predicate.ImageResult {
	return predicate.ImageResult(func(selector *entsql.Selector) {
		if selector.Dialect() == dialect.Postgres {
			selector.ForUpdate()
		}
	})
}

func (s *AssetsStore) GetByUserAndID(ctx context.Context, userID int64, assetID string) (domainassets.ReferenceAsset, error) {
	id, err := uuid.Parse(assetID)
	if err != nil {
		return domainassets.ReferenceAsset{}, err
	}
	entity, err := s.client.ReferenceAsset.Query().
		Where(referenceasset.IDEQ(id), referenceasset.UserIDEQ(userID)).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainassets.ReferenceAsset{}, repoerr.ErrNotFound
		}
		return domainassets.ReferenceAsset{}, err
	}
	return s.ensureReferenceAssetName(ctx, userID, entity)
}

func (s *AssetsStore) GetManyByUserAndIDs(ctx context.Context, userID int64, assetIDs []string) ([]domainassets.ReferenceAsset, error) {
	ids := make([]uuid.UUID, 0, len(assetIDs))
	for _, raw := range assetIDs {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			return nil, repoerr.ErrNotFound
		}
		ids = append(ids, id)
	}
	entities, err := s.client.ReferenceAsset.Query().Where(
		referenceasset.IDIn(ids...), referenceasset.UserIDEQ(userID), referenceasset.DeletedAtIsNil(), referenceasset.StatusNEQ("deleted"),
	).All(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*repoent.ReferenceAsset, len(entities))
	for _, entity := range entities {
		byID[entity.ID.String()] = entity
	}
	result := make([]domainassets.ReferenceAsset, 0, len(assetIDs))
	for _, id := range assetIDs {
		entity, exists := byID[strings.TrimSpace(id)]
		if !exists {
			return nil, repoerr.ErrNotFound
		}
		asset, nameErr := s.ensureReferenceAssetName(ctx, userID, entity)
		if nameErr != nil {
			return nil, nameErr
		}
		result = append(result, asset)
	}
	return result, nil
}

func (s *AssetsStore) RenameByUserAndID(ctx context.Context, userID int64, assetID, name, normalizedName string) (domainassets.ReferenceAsset, error) {
	id, err := uuid.Parse(assetID)
	if err != nil {
		return domainassets.ReferenceAsset{}, repoerr.ErrNotFound
	}
	entity, err := s.client.ReferenceAsset.UpdateOneID(id).
		Where(referenceasset.UserIDEQ(userID), referenceasset.DeletedAtIsNil(), referenceasset.StatusNEQ("deleted")).
		SetName(name).SetNameNormalized(normalizedName).Save(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainassets.ReferenceAsset{}, repoerr.ErrNotFound
		}
		if repoent.IsConstraintError(err) {
			return domainassets.ReferenceAsset{}, repoerr.ErrConflict
		}
		return domainassets.ReferenceAsset{}, err
	}
	return mapReferenceAssetEntity(entity), nil
}

func nextReferenceAssetName(ctx context.Context, client *repoent.Client, userID int64, preferred string) (string, error) {
	entities, err := client.ReferenceAsset.Query().Where(
		referenceasset.UserIDEQ(userID), referenceasset.DeletedAtIsNil(), referenceasset.StatusNEQ("deleted"), referenceasset.NameNormalizedNotNil(),
	).Select(referenceasset.FieldNameNormalized).All(ctx)
	if err != nil {
		return "", err
	}
	used := make(map[string]struct{}, len(entities))
	for _, entity := range entities {
		if entity.NameNormalized != nil {
			used[*entity.NameNormalized] = struct{}{}
		}
	}
	for sequence := 1; sequence <= 10000; sequence++ {
		candidate := domainassets.ReferenceNameCandidate(preferred, sequence)
		if _, exists := used[candidate]; !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("reference asset name space exhausted")
}

func (s *AssetsStore) ensureReferenceAssetName(ctx context.Context, userID int64, entity *repoent.ReferenceAsset) (domainassets.ReferenceAsset, error) {
	if entity.Name != nil && strings.TrimSpace(*entity.Name) != "" {
		return mapReferenceAssetEntity(entity), nil
	}
	for attempt := 0; attempt < 20; attempt++ {
		name, err := nextReferenceAssetName(ctx, s.client, userID, "")
		if err != nil {
			return domainassets.ReferenceAsset{}, err
		}
		updated, err := s.client.ReferenceAsset.UpdateOneID(entity.ID).
			Where(referenceasset.UserIDEQ(userID), referenceasset.DeletedAtIsNil(), referenceasset.StatusNEQ("deleted"), referenceasset.NameIsNil()).
			SetName(name).SetNameNormalized(name).Save(ctx)
		if err == nil {
			return mapReferenceAssetEntity(updated), nil
		}
		if repoent.IsNotFound(err) {
			reloaded, reloadErr := s.client.ReferenceAsset.Query().Where(referenceasset.IDEQ(entity.ID), referenceasset.UserIDEQ(userID), referenceasset.DeletedAtIsNil(), referenceasset.StatusNEQ("deleted")).Only(ctx)
			if reloadErr != nil {
				return domainassets.ReferenceAsset{}, repoerr.ErrNotFound
			}
			return mapReferenceAssetEntity(reloaded), nil
		}
		if !repoent.IsConstraintError(err) {
			return domainassets.ReferenceAsset{}, err
		}
	}
	return domainassets.ReferenceAsset{}, repoerr.ErrConflict
}

func (s *AssetsStore) DeleteByUserAndID(ctx context.Context, userID int64, assetID string) error {
	id, err := uuid.Parse(assetID)
	if err != nil {
		return err
	}
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	entity, err := tx.ReferenceAsset.Query().Where(
		referenceasset.IDEQ(id),
		referenceasset.UserIDEQ(userID),
		referenceasset.DeletedAtIsNil(),
		referenceasset.StatusNEQ("deleted"),
	).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return repoerr.ErrNotFound
		}
		return err
	}
	now := time.Now().UTC()
	if err := tx.ReferenceAsset.UpdateOneID(entity.ID).SetStatus("deleted").SetDeletedAt(now).Exec(ctx); err != nil {
		return err
	}
	configID := ""
	if entity.StorageConfigID != nil {
		configID = entity.StorageConfigID.String()
	}
	if _, err := enqueueObjectDeletionJob(ctx, tx.Client(), cleanupIdentity(configID, entity.StorageDriver, entity.ObjectKey)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func mapReferenceAssetEntity(entity *repoent.ReferenceAsset) domainassets.ReferenceAsset {
	asset := domainassets.ReferenceAsset{
		ID:              entity.ID.String(),
		Name:            nullableString(entity.Name),
		APIKeyID:        entity.APIKeyID,
		UploadSource:    entity.UploadSource,
		Status:          entity.Status,
		StorageConfigID: "",
		StorageDriver:   entity.StorageDriver,
		MimeType:        entity.MimeType,
		FileSizeBytes:   entity.FileSizeBytes,
		SHA256:          entity.Sha256,
		ObjectKey:       entity.ObjectKey,
		OwnsObject:      entity.OwnsObject,
		CreatedAt:       entity.CreatedAt,
	}
	if entity.SourceImageResultID != nil {
		asset.SourceImageResultID = entity.SourceImageResultID.String()
	}
	if entity.StorageConfigID != nil {
		asset.StorageConfigID = entity.StorageConfigID.String()
	}
	if entity.Width != nil {
		asset.Width = *entity.Width
	}
	if entity.Height != nil {
		asset.Height = *entity.Height
	}
	if entity.StorageConfigID != nil {
		asset.StorageConfigID = entity.StorageConfigID.String()
	}
	return asset
}
