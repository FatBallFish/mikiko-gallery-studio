package imagetask

import (
	"context"
	"errors"
	"strings"

	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

const maxGalleryBatchSize = 100

func (s *Service) BatchSetImageGroup(ctx context.Context, userID int64, projectID string, imageIDs []string, imageGroup string) (domainimagetask.GalleryBatchResult, error) {
	imageGroup = strings.TrimSpace(imageGroup)
	if len([]rune(imageGroup)) > 64 {
		return domainimagetask.GalleryBatchResult{}, errs.BadRequest("image group is too long")
	}
	store, ok := s.store.(interface {
		SetImageGroupInProject(context.Context, int64, string, string, string) (domainimagetask.GalleryImage, error)
	})
	if !ok {
		return domainimagetask.GalleryBatchResult{}, errs.Internal("project-scoped gallery group is unavailable")
	}
	return s.runGalleryBatch(ctx, userID, projectID, imageIDs, func(ctx context.Context, imageID string) (domainimagetask.GalleryImage, error) {
		image, err := store.SetImageGroupInProject(ctx, userID, imageID, projectID, imageGroup)
		if err != nil {
			return domainimagetask.GalleryImage{}, err
		}
		return s.projectGalleryImageMedia(ctx, image, "/api/agent/image/v1/images/"+image.ID)
	})
}

func (s *Service) BatchDeleteImages(ctx context.Context, userID int64, projectID string, imageIDs []string) (domainimagetask.GalleryBatchResult, error) {
	store, ok := s.store.(interface {
		DeleteImageResultInProject(context.Context, int64, string, string) (provider.ImageResult, error)
	})
	if !ok {
		return domainimagetask.GalleryBatchResult{}, errs.Internal("project-scoped gallery deletion is unavailable")
	}
	return s.runGalleryBatch(ctx, userID, projectID, imageIDs, func(ctx context.Context, imageID string) (domainimagetask.GalleryImage, error) {
		if _, err := store.DeleteImageResultInProject(ctx, userID, imageID, projectID); err != nil {
			return domainimagetask.GalleryImage{}, err
		}
		return domainimagetask.GalleryImage{ID: imageID, ProjectID: strings.TrimSpace(projectID)}, nil
	})
}

func (s *Service) BatchPublishImages(ctx context.Context, userID int64, projectID string, imageIDs []string, publish bool) (domainimagetask.GalleryBatchResult, error) {
	store, ok := s.store.(interface {
		RequestPublishInProject(context.Context, int64, string, string) (domainimagetask.GalleryImage, error)
		CancelPublishInProject(context.Context, int64, string, string) (domainimagetask.GalleryImage, error)
	})
	if !ok {
		return domainimagetask.GalleryBatchResult{}, errs.Internal("project-scoped gallery publication is unavailable")
	}
	return s.runGalleryBatch(ctx, userID, projectID, imageIDs, func(ctx context.Context, imageID string) (domainimagetask.GalleryImage, error) {
		if publish {
			return store.RequestPublishInProject(ctx, userID, imageID, projectID)
		}
		return store.CancelPublishInProject(ctx, userID, imageID, projectID)
	})
}

func (s *Service) BatchPublishImagesWithAction(ctx context.Context, userID int64, projectID string, imageIDs []string, action func(context.Context, string, string) (domainimagetask.GalleryImage, error)) (domainimagetask.GalleryBatchResult, error) {
	return s.runGalleryBatch(ctx, userID, projectID, imageIDs, func(ctx context.Context, imageID string) (domainimagetask.GalleryImage, error) {
		return action(ctx, imageID, projectID)
	})
}

func (s *Service) BatchTransferImages(ctx context.Context, userID int64, sourceProjectID, targetProjectID string, imageIDs []string) (domainimagetask.GalleryBatchResult, error) {
	sourceProjectID, targetProjectID = strings.TrimSpace(sourceProjectID), strings.TrimSpace(targetProjectID)
	if sourceProjectID == "" || targetProjectID == "" || sourceProjectID == targetProjectID {
		return domainimagetask.GalleryBatchResult{}, errs.BadRequest("distinct source and target projects are required")
	}
	if s.projects == nil {
		return domainimagetask.GalleryBatchResult{}, errs.Internal("project resolver is unavailable")
	}
	transferStore, ok := s.store.(interface {
		TransferImageProject(context.Context, int64, string, string, string) (domainimagetask.GalleryImage, error)
	})
	if !ok {
		return domainimagetask.GalleryBatchResult{}, errs.Internal("gallery project transfer is unavailable")
	}
	target, err := s.projects.ResolveForWrite(ctx, userID, targetProjectID)
	if err != nil || target.ID != targetProjectID {
		return domainimagetask.GalleryBatchResult{}, errs.New(404, errs.CodeNotFound, "target project not found")
	}
	return s.runGalleryBatch(ctx, userID, sourceProjectID, imageIDs, func(ctx context.Context, imageID string) (domainimagetask.GalleryImage, error) {
		image, err := transferStore.TransferImageProject(ctx, userID, imageID, sourceProjectID, targetProjectID)
		if err != nil {
			if errors.Is(err, repoerr.ErrNotFound) {
				return domainimagetask.GalleryImage{}, errs.New(404, errs.CodeNotFound, "image not found")
			}
			return domainimagetask.GalleryImage{}, errs.Internal("failed to transfer image project")
		}
		return s.projectGalleryImageMedia(ctx, image, "/api/agent/image/v1/images/"+image.ID)
	})
}

func (s *Service) runGalleryBatch(
	ctx context.Context,
	userID int64,
	projectID string,
	imageIDs []string,
	action func(context.Context, string) (domainimagetask.GalleryImage, error),
) (domainimagetask.GalleryBatchResult, error) {
	ids, err := normalizeGalleryBatchIDs(imageIDs)
	if err != nil {
		return domainimagetask.GalleryBatchResult{}, err
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return domainimagetask.GalleryBatchResult{}, errs.BadRequest("project_id is required")
	}
	result := domainimagetask.GalleryBatchResult{
		Succeeded: make([]domainimagetask.GalleryBatchSuccess, 0, len(ids)),
		Failed:    make([]domainimagetask.GalleryBatchFailure, 0),
	}
	for _, imageID := range ids {
		owned, getErr := s.store.GetImageResultByID(ctx, userID, imageID)
		if getErr != nil || strings.TrimSpace(owned.ProjectID) != projectID {
			result.Failed = append(result.Failed, galleryBatchFailure(imageID, getErr))
			continue
		}
		entity, actionErr := action(ctx, imageID)
		if actionErr != nil {
			result.Failed = append(result.Failed, galleryBatchFailure(imageID, actionErr))
			continue
		}
		result.Succeeded = append(result.Succeeded, domainimagetask.GalleryBatchSuccess{ID: imageID, Entity: entity})
	}
	return result, nil
}

func normalizeGalleryBatchIDs(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	ids := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ids = append(ids, value)
		if len(ids) > maxGalleryBatchSize {
			return nil, errs.BadRequest("gallery batch size exceeds 100")
		}
	}
	if len(ids) == 0 {
		return nil, errs.BadRequest("image_ids is required")
	}
	return ids, nil
}

func galleryBatchFailure(imageID string, err error) domainimagetask.GalleryBatchFailure {
	failure := domainimagetask.GalleryBatchFailure{ID: imageID, Code: "conflict", Message: "image could not be updated"}
	var appErr *errs.Error
	switch {
	case err == nil, errors.Is(err, repoerr.ErrNotFound), errors.As(err, &appErr) && appErr.StatusCode == 404:
		failure.Code, failure.Message = "not_found", "image was not found in the selected project"
	case errors.As(err, &appErr):
		failure.Code, failure.Message = strings.ToLower(appErr.Code), appErr.Message
	}
	return failure
}
