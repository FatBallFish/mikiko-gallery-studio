package assets

import (
	"context"

	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
)

type Store interface {
	GetByUserAndHash(ctx context.Context, userID int64, sha string) (domainassets.ReferenceAsset, error)
	Save(ctx context.Context, userID int64, asset domainassets.ReferenceAsset) error
	GetByUserAndID(ctx context.Context, userID int64, assetID string) (domainassets.ReferenceAsset, error)
}

type MetadataStore interface {
	SaveWithMetadata(ctx context.Context, userID int64, asset domainassets.ReferenceAsset, metadata domainassets.UploadMetadata) error
}
