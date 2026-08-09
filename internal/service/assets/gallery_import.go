package assets

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

func (s *Service) ImportGalleryImage(ctx context.Context, userID int64, result provider.ImageResult) (domainassets.ReferenceAsset, error) {
	if strings.EqualFold(strings.TrimSpace(result.StorageDriver), "remote") || strings.TrimSpace(result.ObjectKey) == "" {
		return domainassets.ReferenceAsset{}, errs.New(404, errs.CodeNotFound, "image not found")
	}
	policy, err := s.AttachmentPolicy(ctx)
	if err != nil {
		return domainassets.ReferenceAsset{}, fmt.Errorf("resolve attachment policy: %w", err)
	}
	_, metadataReady := galleryImportMetadata(result, policy.Image)
	if !metadataReady {
		return domainassets.ReferenceAsset{}, errs.New(422, "IMAGE_REFERENCE_METADATA_UNAVAILABLE", "图片元数据不完整，请重新生成或下载后上传。")
	}
	key := fmt.Sprintf("%d:%s", userID, strings.TrimSpace(result.ID))
	s.mu.Lock()
	defer s.mu.Unlock()

	if aliasStore, ok := s.store.(AliasStore); ok {
		return aliasStore.ImportGalleryAlias(ctx, userID, result)
	}
	if assetID, ok := s.assetsBySource[key]; ok {
		if stored, exists := s.assetsByID[assetID]; exists && stored.Asset.Status != "deleted" {
			return stored.Asset, nil
		}
		delete(s.assetsBySource, key)
	}

	assetID := uuid.NewString()
	asset := domainassets.ReferenceAsset{
		ID: assetID, UploadSource: "gallery_import", Status: "ready",
		StorageConfigID: result.StorageConfigID, StorageDriver: result.StorageDriver,
		MimeType: result.MimeType, FileSizeBytes: result.FileSizeBytes,
		Width: result.Width, Height: result.Height, SHA256: result.SHA256,
		ObjectKey: result.ObjectKey, SourceImageResultID: result.ID, OwnsObject: false, CreatedAt: time.Now(),
	}
	if s.store != nil {
		var err error
		metadata := domainassets.UploadMetadata{UploadSource: "gallery_import"}
		if metadataStore, ok := s.store.(MetadataStore); ok {
			err = metadataStore.SaveWithMetadata(ctx, userID, asset, metadata)
		} else {
			err = s.store.Save(ctx, userID, asset)
		}
		if err != nil {
			return domainassets.ReferenceAsset{}, err
		}
	}
	s.assetsByID[assetID] = storedAsset{UserID: userID, Asset: asset}
	s.assetsBySource[key] = assetID
	return asset, nil
}

func galleryImportMetadata(result provider.ImageResult, policy FilePolicy) (string, bool) {
	format := galleryImportFormat(result.MimeType)
	if format == "" || !containsString(policy.AllowedFormats, format) ||
		result.FileSizeBytes <= 0 || result.FileSizeBytes > policy.MaxBytes ||
		result.Width <= 0 || result.Height <= 0 || len(result.SHA256) != 64 {
		return format, false
	}
	_, err := hex.DecodeString(result.SHA256)
	return format, err == nil
}

func galleryImportFormat(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0])) {
	case "image/png":
		return "png"
	case "image/jpeg", "image/jpg":
		return "jpeg"
	case "image/webp":
		return "webp"
	case "image/gif":
		return "gif"
	default:
		return ""
	}
}

func galleryImportFilename(result provider.ImageResult, format string) string {
	extension := format
	if extension == "" {
		extension = galleryImportFormat(result.MimeType)
	}
	if extension == "jpeg" {
		extension = "jpg"
	}
	if extension == "" {
		extension = "png"
	}
	return strings.TrimSpace(result.ID) + "." + extension
}

func galleryImportTooLargeError(policy FilePolicy, actual int64) error {
	details := map[string]any{"max_size_bytes": policy.MaxBytes, "max_size_mb": policy.MaxMB}
	if actual > 0 {
		details["actual_size_bytes"] = actual
	}
	return errs.WithDetails(
		errs.New(400, errs.CodeImageReferenceTooLarge, fmt.Sprintf("参考图文件超过 %d MB，请压缩后重新上传。", policy.MaxMB)),
		details,
	)
}
