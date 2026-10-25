package mediaasset

import (
	"context"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/google/uuid"
)

type groupLister interface {
	ListAssetGroups(context.Context, int64, *uuid.UUID) ([]string, error)
}

func (s *Service) ListGroups(ctx context.Context, userID int64, projectID *uuid.UUID) ([]string, error) {
	if s == nil || s.store == nil {
		return nil, errs.Internal("media asset service is unavailable")
	}
	store, ok := s.store.(groupLister)
	if !ok {
		return []string{}, nil
	}
	return store.ListAssetGroups(ctx, userID, projectID)
}
