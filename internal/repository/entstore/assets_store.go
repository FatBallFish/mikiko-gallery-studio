package entstore

import (
	"context"
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
	return mapReferenceAssetEntity(entity), nil
}

func (s *AssetsStore) Save(ctx context.Context, userID int64, asset domainassets.ReferenceAsset) error {
	return s.SaveWithMetadata(ctx, userID, asset, domainassets.UploadMetadata{APIKeyID: asset.APIKeyID, UploadSource: asset.UploadSource})
}

func (s *AssetsStore) SaveWithMetadata(ctx context.Context, userID int64, asset domainassets.ReferenceAsset, metadata domainassets.UploadMetadata) error {
	id, err := uuid.Parse(asset.ID)
	if err != nil {
		return err
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
	if strings.TrimSpace(asset.SourceImageResultID) != "" {
		sourceID, parseErr := uuid.Parse(asset.SourceImageResultID)
		if parseErr != nil {
			return parseErr
		}
		create.SetSourceImageResultID(sourceID)
	}
	if asset.StorageConfigID != "" {
		storageConfigID, parseErr := uuid.Parse(asset.StorageConfigID)
		if parseErr != nil {
			return parseErr
		}
		create.SetStorageConfigID(storageConfigID)
	}
	if metadata.APIKeyID != nil {
		create.SetAPIKeyID(*metadata.APIKeyID)
	}
	if strings.TrimSpace(asset.StorageConfigID) != "" {
		storageConfigID, err := uuid.Parse(asset.StorageConfigID)
		if err != nil {
			return err
		}
		create.SetStorageConfigID(storageConfigID)
	}
	return create.Exec(ctx)
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
	return mapReferenceAssetEntity(entity), nil
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
	if strings.TrimSpace(result.ProjectID) == "" || source.ProjectID == nil || source.ProjectID.String() != strings.TrimSpace(result.ProjectID) {
		return domainassets.ReferenceAsset{}, repoerr.ErrNotFound
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
		SetExpiresAt(time.Now().Add(24 * time.Hour))
	if source.StorageConfigID != nil {
		create.SetStorageConfigID(*source.StorageConfigID)
	}
	entity, err := create.Save(ctx)
	if err != nil {
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
	return mapReferenceAssetEntity(entity), nil
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
