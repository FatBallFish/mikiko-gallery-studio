package assets

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "golang.org/x/image/webp"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type storedAsset struct {
	UserID int64
	Asset  domainassets.ReferenceAsset
}

type Service struct {
	mu           sync.Mutex
	store        Store
	router       storage.Router
	fallback     AttachmentPolicy
	policyMu     sync.RWMutex
	policy       *AttachmentPolicyResolver
	assetsByID   map[string]storedAsset
	assetsByHash map[string]string
}

func NewService(cfg config.StorageConfig, limits config.GenerationLimitsConfig) *Service {
	return NewServiceWithStore(cfg, limits, nil)
}

func NewServiceWithStore(cfg config.StorageConfig, limits config.GenerationLimitsConfig, store Store) *Service {
	backend, err := storage.NewBackend(cfg)
	if err != nil {
		backend = storage.NewLocalBackend(cfg.LocalRoot)
	}
	return NewServiceWithStoreAndBackend(cfg, limits, store, backend)
}

func NewServiceWithStoreAndBackend(_ config.StorageConfig, limits config.GenerationLimitsConfig, store Store, backend storage.Backend) *Service {
	if backend == nil {
		backend = storage.NewLocalBackend("")
	}
	return NewServiceWithStoreAndRouter(limits, store, storage.NewStaticRouter(backend))
}

func NewServiceWithStoreAndRouter(limits config.GenerationLimitsConfig, store Store, router storage.Router) *Service {
	if router == nil {
		router = storage.NewStaticRouter(storage.NewLocalBackend(""))
	}
	defaults := config.ApplyAttachmentPolicyDefaults(config.AttachmentPolicyConfig{}, limits.ReferenceImageMaxMB)
	fallback, _ := NewAttachmentPolicyResolver(defaults, nil).Resolve(context.Background())
	return &Service{store: store, router: router, fallback: fallback, assetsByID: map[string]storedAsset{}, assetsByHash: map[string]string{}}
}

func (s *Service) SetAttachmentPolicyResolver(resolver *AttachmentPolicyResolver) {
	s.policyMu.Lock()
	s.policy = resolver
	s.policyMu.Unlock()
}

func (s *Service) AttachmentPolicy(ctx context.Context) (AttachmentPolicy, error) {
	s.policyMu.RLock()
	resolver := s.policy
	s.policyMu.RUnlock()
	if resolver != nil {
		return resolver.Resolve(ctx)
	}
	return cloneAttachmentPolicy(s.fallback), nil
}

func (s *Service) Upload(userID int64, filename string, contentType string, content []byte) (domainassets.ReferenceAsset, error) {
	return s.UploadWithMetadataContext(context.Background(), userID, filename, contentType, content, domainassets.UploadMetadata{UploadSource: "web"})
}

func (s *Service) UploadWithMetadata(userID int64, filename string, contentType string, content []byte, metadata domainassets.UploadMetadata) (domainassets.ReferenceAsset, error) {
	return s.UploadWithMetadataContext(context.Background(), userID, filename, contentType, content, metadata)
}

func (s *Service) UploadWithMetadataContext(ctx context.Context, userID int64, filename string, contentType string, content []byte, metadata domainassets.UploadMetadata) (domainassets.ReferenceAsset, error) {
	if len(content) == 0 {
		return domainassets.ReferenceAsset{}, errs.New(400, errs.CodeImageReferenceRequired, "reference asset file is required")
	}
	policy, policyErr := s.AttachmentPolicy(ctx)
	if policyErr != nil {
		return domainassets.ReferenceAsset{}, fmt.Errorf("resolve attachment policy: %w", policyErr)
	}
	if policy.Image.MaxBytes > 0 && int64(len(content)) > policy.Image.MaxBytes {
		maxMB := policy.Image.MaxMB
		if maxMB <= 0 {
			maxMB = 1
		}
		return domainassets.ReferenceAsset{}, errs.WithDetails(
			errs.New(400, errs.CodeImageReferenceTooLarge, fmt.Sprintf("参考图文件超过 %d MB，请压缩后重新上传。", maxMB)),
			map[string]any{
				"max_size_bytes":    policy.Image.MaxBytes,
				"max_size_mb":       maxMB,
				"actual_size_bytes": int64(len(content)),
			},
		)
	}
	imageConfig, detectedFormat, detectedMIME, validationErr := validateImageContent(filename, contentType, content, policy.Image)
	if validationErr != nil {
		return domainassets.ReferenceAsset{}, validationErr
	}
	hash := sha256.Sum256(content)
	sha := hex.EncodeToString(hash[:])
	key := fmt.Sprintf("%d:%s", userID, sha)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store != nil {
		existing, err := s.store.GetByUserAndHash(ctx, userID, sha)
		if err == nil {
			return existing, nil
		}
		if err != nil && err != repoerr.ErrNotFound {
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
	ext := "." + detectedFormat
	if detectedFormat == "jpeg" {
		ext = ".jpg"
	}
	objectKey := filepath.Join("reference-assets", assetID+strings.ToLower(ext))
	writer, err := s.router.DefaultWriter(ctx)
	if err != nil {
		return domainassets.ReferenceAsset{}, errs.New(500, "STORAGE_CONFIG_UNAVAILABLE", "default storage config is unavailable")
	}
	if err := writer.Backend.Put(ctx, objectKey, detectedMIME, content); err != nil {
		return domainassets.ReferenceAsset{}, errs.New(500, errs.CodeImageStorageFailed, "failed to store reference asset")
	}
	asset := domainassets.ReferenceAsset{ID: assetID, APIKeyID: metadata.APIKeyID, UploadSource: defaultString(metadata.UploadSource, "web"), Status: "ready", StorageConfigID: writer.ConfigID, StorageDriver: writer.Driver, MimeType: detectedMIME, FileSizeBytes: int64(len(content)), Width: imageConfig.Width, Height: imageConfig.Height, SHA256: sha, ObjectKey: objectKey, CreatedAt: time.Now()}
	if s.store != nil {
		if metadataStore, ok := s.store.(MetadataStore); ok {
			err = metadataStore.SaveWithMetadata(ctx, userID, asset, metadata)
		} else {
			err = s.store.Save(ctx, userID, asset)
		}
		if err != nil {
			_ = writer.Backend.Delete(ctx, objectKey)
			return domainassets.ReferenceAsset{}, err
		}
	}
	s.assetsByID[assetID] = storedAsset{UserID: userID, Asset: asset}
	s.assetsByHash[key] = assetID
	return asset, nil
}

func validateImageContent(filename, declaredContentType string, content []byte, policy FilePolicy) (image.Config, string, string, error) {
	imageConfig, format, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		return image.Config{}, "", "", imageFormatError(filename, policy, "unsupported image content")
	}
	format = strings.ToLower(strings.TrimSpace(format))
	actualMIME := imageFormatMIME(format)
	if actualMIME == "" || !containsString(policy.AllowedFormats, format) {
		return image.Config{}, "", "", imageFormatError(filename, policy, "image format is not allowed")
	}
	detectedMIME, _, _ := mime.ParseMediaType(http.DetectContentType(content))
	if detectedMIME != "" && detectedMIME != "application/octet-stream" && !strings.EqualFold(detectedMIME, actualMIME) {
		return image.Config{}, "", "", imageFormatError(filename, policy, "detected image content is inconsistent")
	}
	declaredMIME := strings.TrimSpace(declaredContentType)
	if declaredMIME != "" {
		parsed, _, parseErr := mime.ParseMediaType(declaredMIME)
		if parseErr != nil {
			return image.Config{}, "", "", imageFormatError(filename, policy, "declared content type is invalid")
		}
		if parsed != "application/octet-stream" && !strings.EqualFold(parsed, actualMIME) {
			return image.Config{}, "", "", imageFormatError(filename, policy, "declared content type does not match image content")
		}
	}
	return imageConfig, format, actualMIME, nil
}

func imageFormatMIME(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "png":
		return "image/png"
	case "jpeg", "jpg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	case "gif":
		return "image/gif"
	default:
		return ""
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
}

func imageFormatError(filename string, policy FilePolicy, message string) error {
	return errs.WithDetails(errs.New(400, errs.CodeValidationFailed, message), map[string]any{
		"filename":        filepath.Base(filename),
		"allowed_formats": append([]string(nil), policy.AllowedFormats...),
	})
}

func (s *Service) Get(userID int64, assetID string) (domainassets.ReferenceAsset, error) {
	return s.GetWithContext(context.Background(), userID, assetID)
}

func (s *Service) GetWithContext(ctx context.Context, userID int64, assetID string) (domainassets.ReferenceAsset, error) {
	asset, err := s.getStored(ctx, userID, assetID)
	if err != nil {
		return domainassets.ReferenceAsset{}, err
	}
	return s.ProjectURLs(ctx, asset)
}

func (s *Service) getStored(ctx context.Context, userID int64, assetID string) (domainassets.ReferenceAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store != nil {
		asset, err := s.store.GetByUserAndID(ctx, userID, assetID)
		if err != nil {
			if err == repoerr.ErrNotFound {
				return domainassets.ReferenceAsset{}, errs.New(404, errs.CodeNotFound, "reference asset not found")
			}
			return domainassets.ReferenceAsset{}, err
		}
		if asset.Status == "deleted" {
			return domainassets.ReferenceAsset{}, errs.New(404, errs.CodeNotFound, "reference asset not found")
		}
		return asset, nil
	}
	stored, ok := s.assetsByID[assetID]
	if !ok || stored.UserID != userID || stored.Asset.Status == "deleted" {
		return domainassets.ReferenceAsset{}, errs.New(404, errs.CodeNotFound, "reference asset not found")
	}
	return stored.Asset, nil
}

func (s *Service) ProjectURLs(ctx context.Context, asset domainassets.ReferenceAsset) (domainassets.ReferenceAsset, error) {
	if strings.TrimSpace(asset.ObjectKey) == "" || strings.EqualFold(strings.TrimSpace(asset.StorageDriver), "remote") {
		return asset, nil
	}
	backend, err := s.router.BackendFor(ctx, asset.StorageConfigID, asset.StorageDriver)
	if err != nil {
		return domainassets.ReferenceAsset{}, errs.New(500, "STORAGE_CONFIG_UNAVAILABLE", "storage config is unavailable")
	}
	urls, supported, err := storage.ProjectTemporaryMediaURLs(ctx, backend.Backend, asset.ObjectKey, asset.MimeType, filepath.Base(asset.ObjectKey))
	if err != nil {
		return domainassets.ReferenceAsset{}, errs.New(500, "STORAGE_CONFIG_UNAVAILABLE", "storage config is unavailable")
	}
	if supported {
		asset.PreviewURL, asset.DownloadURL = urls.PreviewURL, urls.DownloadURL
		asset.PreviewExpiresAt = mediaExpiryPointer(urls.PreviewExpiresAt)
		asset.DownloadExpiresAt = mediaExpiryPointer(urls.DownloadExpiresAt)
	}
	return asset, nil
}

func mediaExpiryPointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	value = value.UTC()
	return &value
}

func (s *Service) Delete(userID int64, assetID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store != nil {
		asset, err := s.store.GetByUserAndID(context.Background(), userID, assetID)
		if err != nil {
			if err == repoerr.ErrNotFound {
				return errs.New(404, errs.CodeNotFound, "reference asset not found")
			}
			return err
		}
		if err := s.store.DeleteByUserAndID(context.Background(), userID, assetID); err != nil {
			if err == repoerr.ErrNotFound {
				return errs.New(404, errs.CodeNotFound, "reference asset not found")
			}
			return err
		}
		delete(s.assetsByID, assetID)
		delete(s.assetsByHash, fmt.Sprintf("%d:%s", userID, asset.SHA256))
		return nil
	}
	stored, ok := s.assetsByID[assetID]
	if !ok || stored.UserID != userID {
		return errs.New(404, errs.CodeNotFound, "reference asset not found")
	}
	stored.Asset.Status = "deleted"
	s.assetsByID[assetID] = stored
	delete(s.assetsByHash, fmt.Sprintf("%d:%s", userID, stored.Asset.SHA256))
	return nil
}

func (s *Service) Download(userID int64, assetID string) (domainassets.ReferenceAsset, []byte, error) {
	asset, err := s.getStored(context.Background(), userID, assetID)
	if err != nil {
		return domainassets.ReferenceAsset{}, nil, err
	}
	backend, err := s.router.BackendFor(context.Background(), asset.StorageConfigID, asset.StorageDriver)
	if err != nil {
		return domainassets.ReferenceAsset{}, nil, errs.New(500, "STORAGE_CONFIG_UNAVAILABLE", "storage config is unavailable")
	}
	content, readErr := backend.Backend.Get(context.Background(), asset.ObjectKey)
	if readErr != nil {
		return domainassets.ReferenceAsset{}, nil, errs.New(500, errs.CodeImageStorageFailed, "failed to read reference asset")
	}
	return asset, content, nil
}

func (s *Service) LoadInput(userID int64, assetID string) (provider.ImageInput, error) {
	asset, err := s.getStored(context.Background(), userID, assetID)
	if err != nil {
		return provider.ImageInput{}, err
	}
	backend, err := s.router.BackendFor(context.Background(), asset.StorageConfigID, asset.StorageDriver)
	if err != nil {
		return provider.ImageInput{}, errs.New(500, "STORAGE_CONFIG_UNAVAILABLE", "storage config is unavailable")
	}
	content, readErr := backend.Backend.Get(context.Background(), asset.ObjectKey)
	if readErr != nil {
		return provider.ImageInput{}, errs.New(500, errs.CodeImageStorageFailed, "failed to read reference asset")
	}
	return provider.ImageInput{
		Filename: filepath.Base(asset.ObjectKey),
		MIMEType: asset.MimeType,
		Data:     content,
	}, nil
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
