package openplatform

import (
	"context"
	"encoding/json"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/google/uuid"
)

// resultAccess grants access only to an output of the owned task. It never
// expands an images/videos grant into general asset-library access.
func (s *Service) resultAccess(ctx context.Context, p Principal, op string, raw json.RawMessage) (any, error) {
	var in struct {
		TaskID  string `json:"task_id"`
		ImageID string `json:"image_id"`
		AssetID string `json:"asset_id"`
		Purpose string `json:"purpose"`
	}
	if err := decode(raw, &in); err != nil {
		return nil, err
	}
	mediaID := ""
	if op == "get_image_result_access" {
		if s.deps.Images == nil {
			return nil, unavailable()
		}
		task, err := s.deps.Images.GetByID(ctx, p.UserID, in.TaskID)
		if err != nil {
			return nil, err
		}
		for _, result := range task.Results {
			if result.ID == in.ImageID && in.ImageID != "" {
				mediaID = result.MediaAssetID
				break
			}
		}
	} else {
		if s.deps.Videos == nil {
			return nil, unavailable()
		}
		id, err := uuid.Parse(in.TaskID)
		if err != nil {
			return nil, errs.BadRequest("invalid task id")
		}
		task, err := s.deps.Videos.Get(ctx, p.UserID, id)
		if err != nil {
			return nil, err
		}
		for _, item := range task.Items {
			if item.ResultAssetID != nil && item.ResultAssetID.String() == in.AssetID {
				mediaID = in.AssetID
				break
			}
		}
	}
	if mediaID == "" {
		return nil, errs.New(404, "NOT_FOUND", "task output not found")
	}
	id, err := uuid.Parse(mediaID)
	if err != nil {
		return nil, errs.Internal("invalid output asset")
	}
	if s.deps.AssetAccess != nil {
		return s.deps.AssetAccess(ctx, p, id, in.Purpose)
	}
	if s.deps.Assets == nil {
		return nil, unavailable()
	}
	return s.deps.Assets.Access(ctx, p.UserID, id, in.Purpose)
}
