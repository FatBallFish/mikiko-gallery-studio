package imagetask

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

func (s *Service) RefreshImageResultAccess(ctx context.Context, userID int64, imageID, purpose string) (storage.TemporaryMediaAccess, error) {
	result, err := s.store.GetImageResultByID(ctx, userID, imageID)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return storage.TemporaryMediaAccess{}, errs.New(404, errs.CodeNotFound, "image not found")
		}
		return storage.TemporaryMediaAccess{}, errs.Internal("failed to load image result")
	}
	return s.projectImageResultAccess(ctx, result, purpose, "/api/agent/image/v1/images/"+url.PathEscape(strings.TrimSpace(result.ID)))
}

func (s *Service) RefreshPublicImageAccess(ctx context.Context, imageID, purpose string) (storage.TemporaryMediaAccess, error) {
	image, err := s.store.GetPublicImage(ctx, imageID, 0)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return storage.TemporaryMediaAccess{}, errs.New(404, errs.CodeNotFound, "gallery image not found")
		}
		return storage.TemporaryMediaAccess{}, errs.Internal("failed to load public gallery image")
	}
	result := provider.ImageResult{
		ID: image.ID, URL: image.URL, DownloadURL: image.DownloadURL, MimeType: image.MimeType,
		StorageConfigID: image.StorageConfigID, ObjectKey: image.ObjectKey, StorageDriver: image.StorageDriver,
	}
	return s.projectImageResultAccess(ctx, result, purpose, "/api/open/image/v1/gallery/images/"+url.PathEscape(strings.TrimSpace(image.ID))+"/image")
}

func (s *Service) projectImageResultAccess(ctx context.Context, result provider.ImageResult, purpose, fallback string) (storage.TemporaryMediaAccess, error) {
	if strings.EqualFold(strings.TrimSpace(result.StorageDriver), "remote") || strings.TrimSpace(result.ObjectKey) == "" {
		remoteURL := absoluteHTTPMediaURL(defaultString(result.URL, result.ObjectKey))
		if remoteURL == "" {
			return storage.TemporaryMediaAccess{}, errs.New(404, errs.CodeNotFound, "image not found")
		}
		return storage.TemporaryMediaAccess{URL: remoteURL}, nil
	}
	backend, routeErr := s.router.BackendFor(ctx, result.StorageConfigID, result.StorageDriver)
	if routeErr != nil {
		return storage.TemporaryMediaAccess{}, errs.New(500, "STORAGE_CONFIG_UNAVAILABLE", "storage config is unavailable")
	}
	access, supported, projectErr := storage.ProjectTemporaryMediaAccess(ctx, backend.Backend, result.ObjectKey, result.MimeType, imageResultDeliveryFilename(result), purpose)
	if projectErr != nil {
		return storage.TemporaryMediaAccess{}, errs.New(500, "STORAGE_CONFIG_UNAVAILABLE", "storage config is unavailable")
	}
	if supported {
		return access, nil
	}
	return storage.TemporaryMediaAccess{URL: fallback}, nil
}
