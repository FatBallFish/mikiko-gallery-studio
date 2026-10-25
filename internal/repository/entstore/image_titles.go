package entstore

import (
	"context"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/referenceasset"
	"github.com/google/uuid"
)

func (s *ImageTaskStore) imageNames(ctx context.Context, ids []uuid.UUID) map[string]string {
	names := map[string]string{}
	if len(ids) == 0 {
		return names
	}
	assets, err := s.client.MediaAsset.Query().Where(mediaasset.IDIn(ids...), mediaasset.DeletedAtIsNil()).All(ctx)
	if err != nil {
		return names
	}
	for _, asset := range assets {
		names[asset.ID.String()] = asset.Name
	}
	return names
}
func (s *ImageTaskStore) mapNamedImageResult(ctx context.Context, entity *repoent.ImageResult) provider.ImageResult {
	item := mapImageResultEntity(entity)
	if name, ok := s.imageNames(ctx, []uuid.UUID{entity.ID})[item.ID]; ok {
		item.Name = name
		item.MediaAssetID = item.ID
	}
	return item
}
func (s *ImageTaskStore) mapNamedImageTask(ctx context.Context, entity *repoent.ImageTask, results []*repoent.ImageResult) (domainimagetask.Task, error) {
	task, err := mapImageTaskEntity(entity, results)
	if err != nil {
		return task, err
	}
	ids := make([]uuid.UUID, 0, len(task.Results))
	for _, result := range task.Results {
		if id, err := uuid.Parse(result.ID); err == nil {
			ids = append(ids, id)
		}
	}
	names := s.imageNames(ctx, ids)
	for i := range task.Results {
		if name, ok := names[task.Results[i].ID]; ok {
			task.Results[i].Name = name
			task.Results[i].MediaAssetID = task.Results[i].ID
		}
	}
	return task, nil
}
func (s *ImageTaskStore) mapNamedGalleryImage(ctx context.Context, entity *repoent.ImageResult, task *repoent.ImageTask) domainimagetask.GalleryImage {
	item := mapGalleryImageEntity(entity, task)
	return s.decorateNamedGalleryImages(ctx, []domainimagetask.GalleryImage{item})[0]
}

func (s *ImageTaskStore) decorateNamedGalleryImages(ctx context.Context, items []domainimagetask.GalleryImage) []domainimagetask.GalleryImage {
	ids := []uuid.UUID{}
	refIDs := []uuid.UUID{}
	for _, item := range items {
		if id, err := uuid.Parse(item.ID); err == nil {
			ids = append(ids, id)
		}
		for _, ref := range item.ReferenceAssets {
			if id, err := uuid.Parse(ref.ID); err == nil {
				refIDs = append(refIDs, id)
				ids = append(ids, id)
			}
		}
	}
	aliases := map[string]uuid.UUID{}
	refNames := map[string]string{}
	if len(refIDs) > 0 {
		refs, err := s.client.ReferenceAsset.Query().Where(referenceasset.IDIn(refIDs...), referenceasset.DeletedAtIsNil()).All(ctx)
		if err == nil {
			for _, ref := range refs {
				if ref.Name != nil {
					refNames[ref.ID.String()] = *ref.Name
				}
				canonical := ref.MediaAssetID
				if canonical == nil {
					canonical = ref.SourceImageResultID
				}
				if canonical != nil {
					aliases[ref.ID.String()] = *canonical
					ids = append(ids, *canonical)
				}
			}
		}
	}
	names := s.imageNames(ctx, ids)
	for i := range items {
		if name, ok := names[items[i].ID]; ok {
			items[i].Name = name
			items[i].MediaAssetID = items[i].ID
		}
		for j := range items[i].ReferenceAssets {
			ref := &items[i].ReferenceAssets[j]
			if name, ok := refNames[ref.ID]; ok {
				ref.Name = name
			}
			id := ref.ID
			if alias, ok := aliases[id]; ok {
				id = alias.String()
			}
			if name, ok := names[id]; ok {
				ref.Name = name
			}
		}
	}
	return items
}
