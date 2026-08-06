package assets

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

func (s *Service) RefreshAccess(ctx context.Context, userID int64, assetID, purpose string) (storage.TemporaryMediaAccess, error) {
	asset, err := s.getStored(ctx, userID, assetID)
	if err != nil {
		return storage.TemporaryMediaAccess{}, err
	}
	if strings.TrimSpace(asset.ObjectKey) == "" || strings.EqualFold(strings.TrimSpace(asset.StorageDriver), "remote") {
		return storage.TemporaryMediaAccess{}, errs.New(404, errs.CodeNotFound, "reference asset not found")
	}
	backend, routeErr := s.router.BackendFor(ctx, asset.StorageConfigID, asset.StorageDriver)
	if routeErr != nil {
		return storage.TemporaryMediaAccess{}, errs.New(500, "STORAGE_CONFIG_UNAVAILABLE", "storage config is unavailable")
	}
	access, supported, projectErr := storage.ProjectTemporaryMediaAccess(ctx, backend.Backend, asset.ObjectKey, asset.MimeType, filepath.Base(asset.ObjectKey), purpose)
	if projectErr != nil {
		return storage.TemporaryMediaAccess{}, errs.New(500, "STORAGE_CONFIG_UNAVAILABLE", "storage config is unavailable")
	}
	if supported {
		return access, nil
	}
	fallback := "/api/agent/image/v1/reference-assets/" + url.PathEscape(strings.TrimSpace(asset.ID)) + "/download"
	return storage.TemporaryMediaAccess{URL: fallback}, nil
}
