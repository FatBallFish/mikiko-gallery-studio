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
	if err := ValidateReferenceAvailability(asset); err != nil {
		return domainassets.ReferenceAsset{}, err
	}
	if strings.TrimSpace(asset.ObjectKey) == "" {
		return domainassets.ReferenceAsset{}, referenceAvailabilityError(asset, "MEDIA_ASSET_NOT_READY", "引用素材正在处理中，请稍后重试")
	}
	return asset, nil
}

// ImportCanvasMediaAlias retains the legacy API name while returning the canonical ID.
func (s *Service) ImportCanvasMediaAlias(ctx context.Context, userID int64, asset mediaassetservice.Asset) (domainassets.ReferenceAsset, error) {
	if asset.UserID != userID {
		return domainassets.ReferenceAsset{}, errs.New(404, errs.CodeNotFound, "media asset not found")
	}
	return s.GetMetadata(ctx, userID, asset.ID.String())
}
