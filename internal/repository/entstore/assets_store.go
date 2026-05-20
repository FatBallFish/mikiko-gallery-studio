package entstore

import (
	"context"
	"time"

	"github.com/google/uuid"

	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
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
		Where(referenceasset.UserIDEQ(userID), referenceasset.Sha256EQ(sha)).
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
	create := s.client.ReferenceAsset.Create().
		SetID(id).
		SetUserID(userID).
		SetUploadSource(defaultString(metadata.UploadSource, "web")).
		SetStatus(asset.Status).
		SetStorageDriver("local").
		SetObjectKey(asset.ObjectKey).
		SetMimeType(asset.MimeType).
		SetFileSizeBytes(asset.FileSizeBytes).
		SetWidth(asset.Width).
		SetHeight(asset.Height).
		SetSha256(asset.SHA256).
		SetExpiresAt(asset.CreatedAt.Add(24 * time.Hour))
	if metadata.APIKeyID != nil {
		create.SetAPIKeyID(*metadata.APIKeyID)
	}
	return create.Exec(ctx)
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

func mapReferenceAssetEntity(entity *repoent.ReferenceAsset) domainassets.ReferenceAsset {
	asset := domainassets.ReferenceAsset{
		ID:            entity.ID.String(),
		APIKeyID:      entity.APIKeyID,
		UploadSource:  entity.UploadSource,
		Status:        entity.Status,
		MimeType:      entity.MimeType,
		FileSizeBytes: entity.FileSizeBytes,
		SHA256:        entity.Sha256,
		ObjectKey:     entity.ObjectKey,
		CreatedAt:     entity.CreatedAt,
	}
	if entity.Width != nil {
		asset.Width = *entity.Width
	}
	if entity.Height != nil {
		asset.Height = *entity.Height
	}
	return asset
}
