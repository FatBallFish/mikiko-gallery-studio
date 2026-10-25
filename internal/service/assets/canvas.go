package assets

import (
	"context"
	"strings"

	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	mediaassetservice "github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

// GetMetadata validates ownership and readiness without projecting URLs or reading objects.
func (s *Service) GetMetadata(ctx context.Context, userID int64, id string) (domainassets.ReferenceAsset, error) {
	asset, err := s.getStored(ctx, userID, id)
	if err != nil {
		return domainassets.ReferenceAsset{}, err
	}
	if asset.Status != "ready" || strings.TrimSpace(asset.ObjectKey) == "" || !strings.HasPrefix(asset.MimeType, "image/") {
		return domainassets.ReferenceAsset{}, errs.New(409, "IMAGE_REFERENCE_UNAVAILABLE", "reference image is not ready")
	}
	return asset, nil
}

// ImportCanvasMediaAlias reuses the persisted media alias without signing URLs or loading bytes.
func (s *Service) ImportCanvasMediaAlias(ctx context.Context, userID int64, asset mediaassetservice.Asset) (domainassets.ReferenceAsset, error) {
	if asset.UserID != userID || asset.DeletedAt != nil || asset.Status == "deleted" {
		return domainassets.ReferenceAsset{}, errs.New(404, errs.CodeNotFound, "media asset not found")
	}
	if asset.MediaType != "image" {
		return domainassets.ReferenceAsset{}, errs.BadRequest("only image media assets can be used as image references")
	}
	if asset.Status != "ready" || strings.TrimSpace(asset.ObjectKey) == "" || !strings.HasPrefix(asset.MIMEType, "image/") {
		return domainassets.ReferenceAsset{}, errs.New(409, "IMAGE_REFERENCE_UNAVAILABLE", "reference image is not ready")
	}
	store, ok := s.store.(MediaAssetAliasStore)
	if !ok {
		return domainassets.ReferenceAsset{}, errs.Internal("media asset reference store is unavailable")
	}
	alias, err := store.ImportMediaAssetAlias(ctx, userID, asset)
	if err != nil {
		return domainassets.ReferenceAsset{}, err
	}
	return s.GetMetadata(ctx, userID, alias.ID)
}
