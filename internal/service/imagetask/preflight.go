package imagetask

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

// prepareCreate validates persisted references as well as the prompt before resolving
// capabilities. Both preflight and persistent task creation use this path.
func (s *Service) prepareCreate(ctx context.Context, req domainimagetask.CreateRequest) (domainimagetask.CreateRequest, error) {
	return s.prepareRequest(ctx, req, false)
}

type assetMetadataLoader interface {
	GetMetadata(context.Context, int64, string) (domainassets.ReferenceAsset, error)
}

func (s *Service) prepareRequest(ctx context.Context, req domainimagetask.CreateRequest, metadataOnly bool) (domainimagetask.CreateRequest, error) {
	if s.projects != nil {
		project, err := s.projects.ResolveForWrite(ctx, req.UserID, req.ProjectID)
		if err != nil {
			return req, mapProjectResolutionError("prepare task", err)
		}
		req.ProjectID = project.ID
	}
	normalized, err := normalizeCreateRequest(req)
	if err != nil {
		return req, err
	}
	req = normalized
	if metadataOnly && req.MaskAssetID != "" {
		if err := s.validateAssetMetadata(ctx, req.UserID, req.MaskAssetID, "mask_asset_id"); err != nil {
			return req, err
		}
	} else if err := s.validateMaskAsset(req); err != nil {
		return req, err
	}
	req.ReferenceImageCount = len(req.ReferenceAssetIDs)
	seen := map[string]bool{}
	for _, id := range req.ReferenceAssetIDs {
		if seen[id] {
			return req, errs.BadRequest("duplicate reference asset")
		}
		seen[id] = true
		if metadataOnly {
			if err := s.validateAssetMetadata(ctx, req.UserID, id, "reference_asset_ids"); err != nil {
				return req, err
			}
			continue
		}
		if s.assets == nil {
			return req, errs.BadRequest("reference assets are unavailable")
		}
		if loader, ok := s.assets.(assetMediaURLLoader); ok {
			if _, err := loader.GetWithContext(ctx, req.UserID, id); err != nil {
				return req, err
			}
		} else if _, err := s.assets.LoadInput(req.UserID, id); err != nil {
			return req, err
		}
	}
	if err := s.resolvePromptTemplate(ctx, &req); err != nil {
		return req, err
	}
	return req, nil
}

// Preflight validates the complete create request without saving a task or
// reserving points. Creation revalidates and reserves through the billing store.
func (s *Service) Preflight(ctx context.Context, req domainimagetask.CreateRequest) (domainbilling.EstimateResult, error) {
	req, err := s.prepareCreate(ctx, req)
	if err != nil {
		return domainbilling.EstimateResult{}, err
	}
	return s.estimatePrepared(ctx, req)
}

// PreflightMetadata shares create normalization, prompt and capability validation,
// but requires metadata support instead of falling back to binary downloads.
func (s *Service) PreflightMetadata(ctx context.Context, req domainimagetask.CreateRequest) (domainbilling.EstimateResult, error) {
	start := time.Now()
	prepared, err := s.prepareRequest(ctx, req, true)
	slog.InfoContext(ctx, "image preflight phase", "phase", "load_metadata", "duration_ms", time.Since(start).Milliseconds(), "reference_count", len(req.ReferenceAssetIDs))
	if err != nil {
		return domainbilling.EstimateResult{}, err
	}
	return s.estimatePrepared(ctx, prepared)
}

func (s *Service) validateAssetMetadata(ctx context.Context, userID int64, id, field string) error {
	loader, ok := s.assets.(assetMetadataLoader)
	if !ok {
		return errs.WithDetails(errs.New(503, "IMAGE_REFERENCE_UNAVAILABLE", "reference metadata is unavailable"), map[string]any{"field": field})
	}
	_, err := loader.GetMetadata(ctx, userID, id)
	if err != nil {
		var typed *errs.Error
		if errors.As(err, &typed) {
			copy := *typed
			return errs.WithDetails(&copy, map[string]any{"field": field})
		}
		return fmt.Errorf("validate %s metadata: %w", field, err)
	}
	return nil
}

func (s *Service) estimatePrepared(ctx context.Context, req domainimagetask.CreateRequest) (domainbilling.EstimateResult, error) {
	var err error
	var estimate domainbilling.EstimateResult
	if manager, ok := s.billing.(routeTaskPreflightManager); ok && req.RouteModelCode != "" {
		_, estimate, err = manager.ResolveAndEstimateRouteTask(ctx, taskEstimateRequest(req, normalizedCount(req.OutputImageCount)), s.cfg.GenerationLimits)
	} else {
		_, err = s.resolveTask(ctx, req.TaskID, req.AbstractModel, req.RouteModelCode, req.UserGroupCodes, req.TaskType, req.SizeMode, req.AspectRatio, req.BaseResolution, req.Quality, req.OutputFormat, req.Background, req.OutputCompression, req.Moderation, req.RequestedSize, req.OutputImageCount, req.ReferenceImageCount, req.MaskPresent, req.CapabilityVersion)
		if err == nil && s.billing != nil {
			estimate, err = s.billing.Estimate(taskEstimateRequest(req, normalizedCount(req.OutputImageCount)))
		}
	}
	if err != nil {
		return estimate, err
	}
	if s.apiKeys != nil && req.APIKeyID > 0 {
		if _, err = s.apiKeys.CheckTaskAllowed(ctx, req.APIKeyID, req.UserID, estimate.EstimatedPoints, s.nowUTC()); err != nil {
			return estimate, err
		}
	}
	return estimate, nil
}
