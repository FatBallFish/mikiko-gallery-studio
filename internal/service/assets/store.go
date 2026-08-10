package assets

import (
	"context"

	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	"github.com/fatballfish/pic-gallery/internal/provider"
)

type Store interface {
	GetByUserAndHash(ctx context.Context, userID int64, sha string) (domainassets.ReferenceAsset, error)
	Save(ctx context.Context, userID int64, asset domainassets.ReferenceAsset) error
	GetByUserAndID(ctx context.Context, userID int64, assetID string) (domainassets.ReferenceAsset, error)
	DeleteByUserAndID(ctx context.Context, userID int64, assetID string) error
}

type MetadataStore interface {
	SaveWithMetadata(ctx context.Context, userID int64, asset domainassets.ReferenceAsset, metadata domainassets.UploadMetadata) error
}

type AliasStore interface {
	GetByUserAndSourceImageResultID(ctx context.Context, userID int64, sourceImageResultID string) (domainassets.ReferenceAsset, error)
	ImportGalleryAlias(ctx context.Context, userID int64, result provider.ImageResult) (domainassets.ReferenceAsset, error)
}

type AliasCreationGate interface {
	AliasCreationEnabled(ctx context.Context) (bool, error)
}

type AliasRolloutStore interface {
	AliasCreationGate
	GetAliasCreationRollout(ctx context.Context) (domainassets.AliasCreationRollout, error)
	UpdateAliasCreationRollout(ctx context.Context, req domainassets.UpdateAliasCreationRolloutRequest) (domainassets.AliasCreationRollout, error)
}
