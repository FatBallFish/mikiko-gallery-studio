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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type storedAsset struct {
	UserID int64
	Asset  domainassets.ReferenceAsset
}

type Service struct {
	mu           sync.Mutex
	store        Store
	storageRoot  string
	maxBytes     int64
	assetsByID   map[string]storedAsset
	assetsByHash map[string]string
}

func NewService(cfg config.StorageConfig, limits config.GenerationLimitsConfig) *Service {
	return NewServiceWithStore(cfg, limits, nil)
}

func NewServiceWithStore(cfg config.StorageConfig, limits config.GenerationLimitsConfig, store Store) *Service {
	root := filepath.Join(cfg.LocalRoot, "reference-assets")
	_ = os.MkdirAll(root, 0o755)
	return &Service{store: store, storageRoot: root, maxBytes: int64(limits.ReferenceImageMaxMB) * 1024 * 1024, assetsByID: map[string]storedAsset{}, assetsByHash: map[string]string{}}
}

func (s *Service) Upload(userID int64, filename string, contentType string, content []byte) (domainassets.ReferenceAsset, error) {
	return s.UploadWithMetadata(userID, filename, contentType, content, domainassets.UploadMetadata{UploadSource: "web"})
}

func (s *Service) UploadWithMetadata(userID int64, filename string, contentType string, content []byte, metadata domainassets.UploadMetadata) (domainassets.ReferenceAsset, error) {
	if len(content) == 0 {
		return domainassets.ReferenceAsset{}, errs.New(400, errs.CodeImageReferenceRequired, "reference asset file is required")
	}
	if s.maxBytes > 0 && int64(len(content)) > s.maxBytes {
		return domainassets.ReferenceAsset{}, errs.New(400, errs.CodeImageReferenceExceeded, "reference asset too large")
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
		return s.assetsByID[assetID].Asset, nil
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
	fullPath := filepath.Join(s.storageRoot, assetID+strings.ToLower(ext))
	if err := os.WriteFile(fullPath, content, 0o644); err != nil {
		return domainassets.ReferenceAsset{}, errs.New(500, errs.CodeImageStorageFailed, "failed to store reference asset")
	}
	asset := domainassets.ReferenceAsset{ID: assetID, APIKeyID: metadata.APIKeyID, UploadSource: defaultString(metadata.UploadSource, "web"), Status: "ready", MimeType: contentType, FileSizeBytes: int64(len(content)), Width: config.Width, Height: config.Height, SHA256: sha, ObjectKey: objectKey, CreatedAt: time.Now()}
	if s.store != nil {
		if metadataStore, ok := s.store.(MetadataStore); ok {
			err = metadataStore.SaveWithMetadata(context.Background(), userID, asset, metadata)
		} else {
			err = s.store.Save(context.Background(), userID, asset)
		}
		if err != nil {
			_ = os.Remove(fullPath)
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
		return asset, nil
	}
	stored, ok := s.assetsByID[assetID]
	if !ok || stored.UserID != userID {
		return domainassets.ReferenceAsset{}, errs.New(404, errs.CodeNotFound, "reference asset not found")
	}
	return stored.Asset, nil
}

func (s *Service) LoadInput(userID int64, assetID string) (provider.ImageInput, error) {
	asset, err := s.Get(userID, assetID)
	if err != nil {
		return provider.ImageInput{}, err
	}
	fullPath := filepath.Join(s.storageRoot, filepath.Base(asset.ObjectKey))
	content, readErr := os.ReadFile(fullPath)
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
