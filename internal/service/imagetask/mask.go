package imagetask

import (
	"strings"

	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

// validateMaskAsset checks ownership and readiness through the same loader used
// by workers. Only the asset ID is retained in the durable task snapshot.
func (s *Service) validateMaskAsset(req domainimagetask.CreateRequest) error {
	if strings.TrimSpace(req.MaskAssetID) == "" {
		return nil
	}
	_, err := s.loadMaskInput(req.UserID, req.MaskAssetID)
	return err
}

func (s *Service) loadMaskInput(userID int64, assetID string) (*provider.ImageInput, error) {
	if strings.TrimSpace(assetID) == "" {
		return nil, nil
	}
	if s.assets == nil {
		return nil, errs.New(500, errs.CodeImageReferenceRequired, "mask asset loader unavailable")
	}
	input, err := s.assets.LoadInput(userID, assetID)
	if err != nil {
		return nil, err
	}
	return &input, nil
}

func (s *Service) resolveReferenceInputs(task domainimagetask.Task) ([]provider.ImageInput, *provider.ImageInput, error) {
	references, err := s.loadReferenceImages(task)
	if err != nil {
		return nil, nil, err
	}
	mask, err := s.loadMaskInput(task.UserID, task.MaskAssetID)
	if err != nil {
		return nil, nil, err
	}
	return references, mask, nil
}
