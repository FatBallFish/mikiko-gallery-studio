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

type GeneratedNameStore interface {
	SaveWithGeneratedName(ctx context.Context, userID int64, asset domainassets.ReferenceAsset, metadata domainassets.UploadMetadata, preferredName string) (domainassets.ReferenceAsset, error)
}

type RenameStore interface {
	RenameByUserAndID(ctx context.Context, userID int64, assetID, name, normalizedName string) (domainassets.ReferenceAsset, error)
}

type BatchStore interface {
	GetManyByUserAndIDs(ctx context.Context, userID int64, assetIDs []string) ([]domainassets.ReferenceAsset, error)
}

type AliasStore interface {
	GetByUserAndSourceImageResultID(ctx context.Context, userID int64, sourceImageResultID string) (domainassets.ReferenceAsset, error)
	ImportGalleryAlias(ctx context.Context, userID int64, result provider.ImageResult) (domainassets.ReferenceAsset, error)
}
