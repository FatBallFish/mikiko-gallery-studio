package entstore

import (
	"context"
	"errors"
	"fmt"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	"github.com/fatballfish/pic-gallery/internal/provider"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imageresult"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/predicate"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/referenceasset"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	mediaassetservice "github.com/fatballfish/pic-gallery/internal/service/mediaasset"
)

type AssetsStore struct {
	client *repoent.Client
}

func NewAssetsStore(client *repoent.Client) *AssetsStore {
	return &AssetsStore{client: client}
}

func (s *AssetsStore) GetByUserAndHash(ctx context.Context, userID int64, sha string) (domainassets.ReferenceAsset, error) {
	rows, err := s.client.MediaAsset.Query().Where(mediaasset.UserIDEQ(userID), mediaasset.Sha256EQ(sha), mediaasset.MediaTypeEQ("image"), mediaasset.DeletedAtIsNil(), mediaasset.StatusIn("ready", "ready_original")).Order(repoent.Asc(mediaasset.FieldCreatedAt)).All(ctx)
	if err != nil {
		return domainassets.ReferenceAsset{}, err
	}
	for _, row := range rows {
		return s.GetByUserAndID(ctx, userID, row.ID.String())
	}
	legacy, err := s.client.ReferenceAsset.Query().Where(referenceasset.UserIDEQ(userID), referenceasset.Sha256EQ(sha), referenceasset.DeletedAtIsNil(), referenceasset.StatusIn("ready", "ready_original"), referenceasset.MediaAssetIDIsNil()).All(ctx)
	if err != nil {
		return domainassets.ReferenceAsset{}, err
	}
	for _, row := range legacy {
		asset, err := s.GetByUserAndID(ctx, userID, row.ID.String())
		if err != nil {
			if errors.Is(err, repoerr.ErrNotFound) {
				continue
			}
			var apiErr *errs.Error
			if errors.As(err, &apiErr) && strings.HasPrefix(apiErr.Code, "MEDIA_ASSET_") {
				continue
			}
			return domainassets.ReferenceAsset{}, err
		}
		if asset.Status == "ready" || asset.Status == "ready_original" {
			return asset, nil
		}
	}
	return domainassets.ReferenceAsset{}, repoerr.ErrNotFound
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
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return domainassets.ReferenceAsset{}, err
	}
	defer func() { _ = tx.Rollback() }()
	// Media-asset aliases and generated-result aliases borrow an object owned by
	// another aggregate; only direct uploads own their storage object.
	ownsObject := asset.OwnsObject || (strings.TrimSpace(asset.SourceImageResultID) == "" && strings.TrimSpace(asset.MediaAssetID) == "")
	create := tx.ReferenceAsset.Create().
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
	if strings.TrimSpace(asset.MediaAssetID) != "" {
		mediaID, parseErr := uuid.Parse(asset.MediaAssetID)
		if parseErr != nil {
			return domainassets.ReferenceAsset{}, parseErr
		}
		create.SetMediaAssetID(mediaID)
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
	if ownsObject {
		if err := persistUploadedReferenceMedia(ctx, tx, userID, asset); err != nil {
			return domainassets.ReferenceAsset{}, err
		}
		create.SetMediaAssetID(id).SetOwnsObject(false)
	}
	entity, err := create.Save(ctx)
	if err != nil {
		return domainassets.ReferenceAsset{}, err
	}
	if err := tx.Commit(); err != nil {
		return domainassets.ReferenceAsset{}, err
	}
	return mapReferenceAssetEntity(entity), nil
}

func (s *AssetsStore) ImportMediaAssetAlias(ctx context.Context, userID int64, asset mediaassetservice.Asset) (domainassets.ReferenceAsset, error) {
	return s.GetByUserAndID(ctx, userID, asset.ID.String())
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
			return s.projectReferenceDisplayName(ctx, saved), nil
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
	return s.GetByUserAndID(ctx, userID, entity.ID.String())
}

func (s *AssetsStore) ImportGalleryAlias(ctx context.Context, userID int64, result provider.ImageResult) (domainassets.ReferenceAsset, error) {
	id, err := uuid.Parse(strings.TrimSpace(result.ID))
	if err != nil {
		return domainassets.ReferenceAsset{}, repoerr.ErrNotFound
	}
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return domainassets.ReferenceAsset{}, err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ImageResult.Query().Where(imageresult.IDEQ(id), imageresult.UserIDEQ(userID), imageresult.DeletedAtIsNil(), lockImageResultForAlias()).Only(ctx)
	if repoent.IsNotFound(err) {
		return domainassets.ReferenceAsset{}, repoerr.ErrNotFound
	}

	if err != nil {
		return domainassets.ReferenceAsset{}, err
	}
	media, err := tx.MediaAsset.Query().Where(mediaasset.UserIDEQ(userID), mediaasset.Or(mediaasset.IDEQ(id), mediaasset.LegacyImageResultIDEQ(id))).Only(ctx)
	if repoent.IsNotFound(err) {
		media, err = ensureMediaAsset(ctx, tx, userID, id)
	}
	if err != nil {
		return domainassets.ReferenceAsset{}, err
	}
	canonicalID := media.ID
	if err := tx.Commit(); err != nil {
		return domainassets.ReferenceAsset{}, err
	}
	return s.GetByUserAndID(ctx, userID, canonicalID.String())
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
	id, err := uuid.Parse(strings.TrimSpace(assetID))
	if err != nil {
		return domainassets.ReferenceAsset{}, repoerr.ErrNotFound
	}
	return s.resolveReference(ctx, userID, id)
}

func (s *AssetsStore) GetManyByUserAndIDs(ctx context.Context, userID int64, assetIDs []string) ([]domainassets.ReferenceAsset, error) {
	result := make([]domainassets.ReferenceAsset, 0, len(assetIDs))
	for _, id := range assetIDs {
		asset, err := s.GetByUserAndID(ctx, userID, id)
		if err != nil {
			return nil, err
		}
		result = append(result, asset)
	}
	return result, nil
}

func (s *AssetsStore) RenameByUserAndID(ctx context.Context, userID int64, assetID, name, normalizedName string) (domainassets.ReferenceAsset, error) {
	resolved, resolveErr := s.GetByUserAndID(ctx, userID, assetID)
	if resolveErr != nil {
		return domainassets.ReferenceAsset{}, resolveErr
	}
	if resolved.MediaAssetID != "" {
		tx, err := s.client.Tx(ctx)
		if err != nil {
			return domainassets.ReferenceAsset{}, err
		}
		defer func() { _ = tx.Rollback() }()
		canonical := uuid.MustParse(resolved.MediaAssetID)
		// Historical consumers still use the unique reference-name namespace.
		if _, err := tx.ReferenceAsset.Update().Where(referenceasset.UserIDEQ(userID), referenceasset.IDEQ(uuid.MustParse(resolved.ID)), referenceasset.DeletedAtIsNil(), referenceasset.StatusNEQ("deleted")).SetName(name).SetNameNormalized(normalizedName).Save(ctx); err != nil {
			if repoent.IsConstraintError(err) {
				return domainassets.ReferenceAsset{}, repoerr.ErrConflict
			}
			return domainassets.ReferenceAsset{}, err
		}
		if err := tx.MediaAsset.UpdateOneID(canonical).Where(mediaasset.UserIDEQ(userID), mediaasset.DeletedAtIsNil(), mediaasset.StatusNEQ("deleted")).SetName(name).SetNameKey(strings.ToLower(name)).AddVersion(1).Exec(ctx); err != nil {
			return domainassets.ReferenceAsset{}, err
		}
		if err := tx.Commit(); err != nil {
			return domainassets.ReferenceAsset{}, err
		}
		return s.GetByUserAndID(ctx, userID, assetID)
	}
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
		return s.referenceWithCanonicalName(ctx, entity), nil
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
			return s.referenceWithCanonicalName(ctx, updated), nil
		}
		if repoent.IsNotFound(err) {
			reloaded, reloadErr := s.client.ReferenceAsset.Query().Where(referenceasset.IDEQ(entity.ID), referenceasset.UserIDEQ(userID), referenceasset.DeletedAtIsNil(), referenceasset.StatusNEQ("deleted")).Only(ctx)
			if reloadErr != nil {
				return domainassets.ReferenceAsset{}, repoerr.ErrNotFound
			}
			return s.referenceWithCanonicalName(ctx, reloaded), nil
		}
		if !repoent.IsConstraintError(err) {
			return domainassets.ReferenceAsset{}, err
		}
	}
	return domainassets.ReferenceAsset{}, repoerr.ErrConflict
}

func (s *AssetsStore) DeleteByUserAndID(ctx context.Context, userID int64, assetID string) error {
	id, parseErr := uuid.Parse(strings.TrimSpace(assetID))
	if parseErr != nil {
		return repoerr.ErrNotFound
	}
	// A historical borrowed handle is independently removable, even if its
	// canonical source no longer exists. Never delete that source's object.
	alias, queryErr := s.client.ReferenceAsset.Query().Where(referenceasset.IDEQ(id), referenceasset.UserIDEQ(userID), referenceasset.DeletedAtIsNil(), referenceasset.StatusNEQ("deleted")).Only(ctx)
	if queryErr != nil && !repoent.IsNotFound(queryErr) {
		return queryErr
	}
	if alias != nil && ((alias.MediaAssetID != nil && *alias.MediaAssetID != id) || (alias.MediaAssetID == nil && alias.SourceImageResultID != nil)) {
		return s.client.ReferenceAsset.UpdateOneID(id).SetStatus("deleted").SetDeletedAt(time.Now().UTC()).Exec(ctx)
	}
	resolved, resolveErr := s.GetByUserAndID(ctx, userID, assetID)
	if resolveErr != nil {
		var typed *errs.Error
		if errors.As(resolveErr, &typed) && typed.Code == "MEDIA_ASSET_DELETED" {
			return repoerr.ErrNotFound
		}
		return resolveErr
	}
	if resolved.MediaAssetID != "" {
		if resolved.ID != resolved.MediaAssetID {
			// Deleting an old borrowed reference only unlinks that compatibility handle.
			return s.client.ReferenceAsset.UpdateOneID(uuid.MustParse(resolved.ID)).Where(referenceasset.UserIDEQ(userID), referenceasset.DeletedAtIsNil()).SetStatus("deleted").SetDeletedAt(time.Now().UTC()).Exec(ctx)
		}
		id := uuid.MustParse(resolved.MediaAssetID)
		entity, err := s.client.MediaAsset.Get(ctx, id)
		if err != nil {
			return err
		}
		if entity.SourceType == "generated" || entity.LegacyImageResultID != nil {
			// The compatibility reference endpoint only releases a borrowed handle.
			// Canonical generated media is removed exclusively by media DELETE.
			if alias != nil {
				return s.client.ReferenceAsset.UpdateOneID(alias.ID).SetStatus("deleted").SetDeletedAt(time.Now().UTC()).Exec(ctx)
			}
			return nil
		}
		_, err = NewMediaStore(s.client).DeleteAsset(ctx, mediaassetservice.DeleteAssetRequest{UserID: userID, AssetID: id, ExpectedVersion: entity.Version})
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
	if entity.OwnsObject && entity.MediaAssetID == nil && entity.SourceImageResultID == nil {
		if _, err := enqueueObjectDeletionJob(ctx, tx.Client(), cleanupIdentity(configID, entity.StorageDriver, entity.ObjectKey)); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *AssetsStore) referenceWithCanonicalName(ctx context.Context, entity *repoent.ReferenceAsset) domainassets.ReferenceAsset {
	return s.projectReferenceDisplayName(ctx, mapReferenceAssetEntity(entity))
}

func (s *AssetsStore) projectReferenceDisplayName(ctx context.Context, asset domainassets.ReferenceAsset) domainassets.ReferenceAsset {
	id := asset.MediaAssetID
	if id == "" {
		id = asset.SourceImageResultID
	}
	if parsed, err := uuid.Parse(id); err == nil {
		canonical, err := s.client.MediaAsset.Get(ctx, parsed)
		if err == nil && canonical.DeletedAt == nil {
			asset.DisplayName = canonical.Name
		}
	}
	return asset
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
	if entity.MediaAssetID != nil {
		asset.MediaAssetID = entity.MediaAssetID.String()
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
