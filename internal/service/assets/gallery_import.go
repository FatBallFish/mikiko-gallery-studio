package assets

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"

	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/internal/storage"
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
	source, err := s.router.BackendFor(ctx, result.StorageConfigID, result.StorageDriver)
	if err != nil {
		return domainassets.ReferenceAsset{}, errs.New(500, "STORAGE_CONFIG_UNAVAILABLE", "source storage config is unavailable")
	}
	destination, err := s.router.DefaultWriter(ctx)
	if err != nil {
		return domainassets.ReferenceAsset{}, errs.New(500, "STORAGE_CONFIG_UNAVAILABLE", "destination storage config is unavailable")
	}

	format, metadataReady := galleryImportMetadata(result, policy.Image)
	if matchingBackendConfiguration(source, destination) {
		if !metadataReady {
			return domainassets.ReferenceAsset{}, errs.New(422, "IMAGE_REFERENCE_METADATA_UNAVAILABLE", "图片元数据不完整，请重新生成或下载后上传。")
		}
		copier, ok := source.Backend.(storage.ObjectCopier)
		if !ok {
			return domainassets.ReferenceAsset{}, errs.New(500, errs.CodeImageStorageFailed, "storage backend does not support server-side copy")
		}
		return s.importGalleryImageByCopy(ctx, userID, result, format, destination, copier)
	}

	getter, ok := source.Backend.(storage.BoundedGetter)
	if !ok {
		return domainassets.ReferenceAsset{}, errs.New(500, errs.CodeImageStorageFailed, "storage backend does not support bounded reads")
	}
	content, err := getter.GetBounded(ctx, result.ObjectKey, policy.Image.MaxBytes+1)
	if err != nil {
		if errors.Is(err, storage.ErrObjectTooLarge) {
			return domainassets.ReferenceAsset{}, galleryImportTooLargeError(policy.Image, 0)
		}
		if errors.Is(err, storage.ErrNotFound) {
			return domainassets.ReferenceAsset{}, errs.New(404, errs.CodeNotFound, "image not found")
		}
		return domainassets.ReferenceAsset{}, errs.New(500, errs.CodeImageStorageFailed, "failed to read gallery image")
	}
	return s.UploadWithMetadataContext(
		ctx,
		userID,
		galleryImportFilename(result, format),
		result.MimeType,
		content,
		domainassets.UploadMetadata{UploadSource: "gallery_import"},
	)
}

func (s *Service) importGalleryImageByCopy(
	ctx context.Context,
	userID int64,
	result provider.ImageResult,
	format string,
	destination storage.BackendRef,
	copier storage.ObjectCopier,
) (domainassets.ReferenceAsset, error) {
	key := fmt.Sprintf("%d:%s", userID, result.SHA256)
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.store != nil {
		existing, err := s.store.GetByUserAndHash(ctx, userID, result.SHA256)
		if err == nil {
			return existing, nil
		}
		if err != nil && !errors.Is(err, repoerr.ErrNotFound) {
			return domainassets.ReferenceAsset{}, err
		}
	}
	if assetID, ok := s.assetsByHash[key]; ok {
		if stored, exists := s.assetsByID[assetID]; exists && stored.Asset.Status != "deleted" {
			return stored.Asset, nil
		}
		delete(s.assetsByHash, key)
	}

	assetID := uuid.NewString()
	extension := "." + format
	if format == "jpeg" {
		extension = ".jpg"
	}
	objectKey := filepath.Join("reference-assets", assetID+extension)
	if err := copier.Copy(ctx, result.ObjectKey, objectKey); err != nil {
		return domainassets.ReferenceAsset{}, errs.New(500, errs.CodeImageStorageFailed, "failed to copy gallery image")
	}
	asset := domainassets.ReferenceAsset{
		ID: assetID, UploadSource: "gallery_import", Status: "ready",
		StorageConfigID: destination.ConfigID, StorageDriver: destination.Driver,
		MimeType: result.MimeType, FileSizeBytes: result.FileSizeBytes,
		Width: result.Width, Height: result.Height, SHA256: result.SHA256,
		ObjectKey: objectKey, CreatedAt: time.Now(),
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
			_ = destination.Backend.Delete(ctx, objectKey)
			return domainassets.ReferenceAsset{}, err
		}
	}
	s.assetsByID[assetID] = storedAsset{UserID: userID, Asset: asset}
	s.assetsByHash[key] = assetID
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

func matchingBackendConfiguration(source, destination storage.BackendRef) bool {
	if !strings.EqualFold(source.Driver, destination.Driver) || source.Version != destination.Version {
		return false
	}
	if source.ConfigID != "" || destination.ConfigID != "" {
		return source.ConfigID != "" && source.ConfigID == destination.ConfigID
	}
	left, right := reflect.ValueOf(source.Backend), reflect.ValueOf(destination.Backend)
	return left.IsValid() && right.IsValid() && left.Type() == right.Type() && left.Kind() == reflect.Pointer && left.Pointer() == right.Pointer()
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
