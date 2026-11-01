package assets

import (
	"context"
	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"strings"
)

func referenceAvailabilityError(asset domainassets.ReferenceAsset, code, message string) error {
	mediaType := strings.SplitN(asset.MimeType, "/", 2)[0]
	statusCode := 409
	if code == "MEDIA_ASSET_TYPE_MISMATCH" {
		statusCode = 400
	}
	return errs.WithDetails(errs.New(statusCode, code, message), map[string]any{"asset_id": asset.ID, "status": asset.Status, "media_type": mediaType, "expected_media_type": "image"})
}

// ValidateReferenceAvailability applies at submit and again before reading bytes.
func ValidateReferenceAvailability(asset domainassets.ReferenceAsset) error {
	if asset.Status == "deleted" || asset.Status == "deleting" {
		return referenceAvailabilityError(asset, "MEDIA_ASSET_DELETED", "引用素材已删除")
	}
	if !strings.HasPrefix(strings.ToLower(asset.MimeType), "image/") {
		return referenceAvailabilityError(asset, "MEDIA_ASSET_TYPE_MISMATCH", "图片引用仅支持图片素材")
	}
	switch asset.Status {
	case "ready", "ready_original":
		return nil
	case "failed", "processing_failed":
		return referenceAvailabilityError(asset, "MEDIA_ASSET_PROCESSING_FAILED", "引用素材处理失败，请重新上传")
	default:
		return referenceAvailabilityError(asset, "MEDIA_ASSET_NOT_READY", "引用素材正在处理中，请稍后重试")
	}
}

func (s *Service) ValidateReferencesWithContext(ctx context.Context, userID int64, ids []string) error {
	assets, err := s.GetManyWithContext(ctx, userID, ids)
	if err != nil {
		return err
	}
	for _, asset := range assets {
		if err := ValidateReferenceAvailability(asset); err != nil {
			return err
		}
	}
	return nil
}
