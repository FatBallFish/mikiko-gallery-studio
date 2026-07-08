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
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

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
	backend      storage.Backend
	router       storage.Router
	maxBytes     int64
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
	return &Service{store: store, router: router, maxBytes: int64(limits.ReferenceImageMaxMB) * 1024 * 1024, assetsByID: map[string]storedAsset{}, assetsByHash: map[string]string{}}
}

func (s *Service) Upload(userID int64, filename string, contentType string, content []byte) (domainassets.ReferenceAsset, error) {
	return s.UploadWithMetadata(userID, filename, contentType, content, domainassets.UploadMetadata{UploadSource: "web"})
}

func (s *Service) UploadWithMetadata(userID int64, filename string, contentType string, content []byte, metadata domainassets.UploadMetadata) (domainassets.ReferenceAsset, error) {
	if len(content) == 0 {
		return domainassets.ReferenceAsset{}, errs.New(400, errs.CodeImageReferenceRequired, "reference asset file is required")
	}
	if s.maxBytes > 0 && int64(len(content)) > s.maxBytes {
		maxMB := int(s.maxBytes / (1024 * 1024))
		if maxMB <= 0 {
			maxMB = 1
		}
		return domainassets.ReferenceAsset{}, errs.WithDetails(
			errs.New(400, errs.CodeImageReferenceTooLarge, fmt.Sprintf("参考图文件超过 %d MB，请压缩后重新上传。", maxMB)),
			map[string]any{
				"max_size_bytes":    s.maxBytes,
				"max_size_mb":       maxMB,
				"actual_size_bytes": int64(len(content)),
			},
		)
	}
	hash := sha256.Sum256(content)
	sha := hex.EncodeToString(hash[:])
	key := fmt.Sprintf("%d:%s", userID, sha)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store != nil {
		existing, err := s.store.GetByUserAndHash(context.Background(), userID, sha)
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

	config, _, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		return domainassets.ReferenceAsset{}, errs.New(400, errs.CodeUpstreamBadRequest, "unsupported image format")
	}
	assetID := uuid.NewString()
	ext := filepath.Ext(filename)
	if ext == "" {
		if exts, _ := mime.ExtensionsByType(contentType); len(exts) > 0 {
			ext = exts[0]
		}
		if ext == "" {
			ext = ".bin"
		}
	}
	objectKey := filepath.Join("reference-assets", assetID+strings.ToLower(ext))
	writer, err := s.router.DefaultWriter(context.Background())
	if err != nil {
		return domainassets.ReferenceAsset{}, errs.New(500, "STORAGE_CONFIG_UNAVAILABLE", "default storage config is unavailable")
	}
	if err := writer.Backend.Put(context.Background(), objectKey, contentType, content); err != nil {
		return domainassets.ReferenceAsset{}, errs.New(500, errs.CodeImageStorageFailed, "failed to store reference asset")
	}
	asset := domainassets.ReferenceAsset{ID: assetID, APIKeyID: metadata.APIKeyID, UploadSource: defaultString(metadata.UploadSource, "web"), Status: "ready", StorageConfigID: writer.ConfigID, StorageDriver: writer.Driver, MimeType: contentType, FileSizeBytes: int64(len(content)), Width: config.Width, Height: config.Height, SHA256: sha, ObjectKey: objectKey, CreatedAt: time.Now()}
	if s.store != nil {
		if metadataStore, ok := s.store.(MetadataStore); ok {
			err = metadataStore.SaveWithMetadata(context.Background(), userID, asset, metadata)
		} else {
			err = s.store.Save(context.Background(), userID, asset)
		}
		if err != nil {
			_ = writer.Backend.Delete(context.Background(), objectKey)
			return domainassets.ReferenceAsset{}, err
		}
	}
	s.assetsByID[assetID] = storedAsset{UserID: userID, Asset: asset}
	s.assetsByHash[key] = assetID
	return asset, nil
}

func (s *Service) Get(userID int64, assetID string) (domainassets.ReferenceAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store != nil {
		asset, err := s.store.GetByUserAndID(context.Background(), userID, assetID)
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
	asset, err := s.Get(userID, assetID)
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
	asset, err := s.Get(userID, assetID)
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
