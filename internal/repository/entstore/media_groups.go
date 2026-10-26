package entstore

import (
	"context"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/assetgroup"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imageresult"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaasset"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/google/uuid"
	"sort"
	"strings"
)

func (s *MediaStore) ListAssetGroups(ctx context.Context, userID int64, projectID *uuid.UUID) ([]string, error) {
	if userID <= 0 {
		return nil, errs.BadRequest("user id is required")
	}
	query := s.client.MediaAsset.Query().Where(mediaasset.UserIDEQ(userID), mediaasset.DeletedAtIsNil(), mediaasset.StatusNotIn("deleted", "deleting"), mediaasset.GroupNameNEQ(""))
	legacy := s.client.ImageResult.Query().Where(imageresult.UserIDEQ(userID), imageresult.DeletedAtIsNil(), imageresult.ImageGroupNEQ(""))
	if projectID != nil {
		query.Where(mediaasset.ProjectIDEQ(*projectID))
		legacy.Where(imageresult.ProjectIDEQ(*projectID))
	}
	names, err := query.Select(mediaasset.FieldGroupName).Strings(ctx)
	if err != nil {
		return nil, err
	}
	oldNames, err := legacy.Select(imageresult.FieldImageGroup).Strings(ctx)
	if err != nil {
		return nil, err
	}
	real := s.client.AssetGroup.Query().Where(assetgroup.UserIDEQ(userID))
	if projectID != nil {
		real.Where(assetgroup.ProjectIDEQ(*projectID))
	}
	realNames, err := real.Select(assetgroup.FieldName).Strings(ctx)
	if err != nil {
		return nil, err
	}
	names = append(names, realNames...)
	unique := make(map[string]struct{}, len(names)+len(oldNames))
	for _, name := range append(names, oldNames...) {
		if name = strings.TrimSpace(name); name != "" {
			unique[name] = struct{}{}
		}
	}
	result := make([]string, 0, len(unique))
	for name := range unique {
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}
