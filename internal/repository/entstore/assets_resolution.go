package entstore

import (
	"context"
	"strings"

	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/project"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/referenceasset"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/google/uuid"
)

// resolveReference keeps the requested ID stable for historical prompt bindings,
// but takes metadata and availability exclusively from the canonical media row.
func (s *AssetsStore) resolveReference(ctx context.Context, userID int64, id uuid.UUID) (domainassets.ReferenceAsset, error) {
	media, err := s.client.MediaAsset.Query().Where(mediaasset.IDEQ(id), mediaasset.UserIDEQ(userID)).Only(ctx)
	if err == nil {
		return s.projectReferenceMedia(ctx, id, media, nil)
	}
	if !repoent.IsNotFound(err) {
		return domainassets.ReferenceAsset{}, err
	}
	alias, err := s.client.ReferenceAsset.Query().Where(referenceasset.IDEQ(id), referenceasset.UserIDEQ(userID)).Only(ctx)
	if repoent.IsNotFound(err) {
		return domainassets.ReferenceAsset{}, repoerr.ErrNotFound
	}
	if err != nil {
		return domainassets.ReferenceAsset{}, err
	}
	if alias.DeletedAt != nil || alias.Status == "deleted" || alias.Status == "deleting" {
		return domainassets.ReferenceAsset{}, errs.WithDetails(errs.New(409, "MEDIA_ASSET_DELETED", "引用素材已删除"), map[string]any{"asset_id": id.String(), "status": alias.Status, "media_type": "image", "expected_media_type": "image"})
	}
	query := s.client.MediaAsset.Query().Where(mediaasset.UserIDEQ(userID))
	switch {
	case alias.MediaAssetID != nil:
		query.Where(mediaasset.IDEQ(*alias.MediaAssetID))
	case alias.SourceImageResultID != nil:
		query.Where(mediaasset.Or(mediaasset.LegacyImageResultIDEQ(*alias.SourceImageResultID), mediaasset.IDEQ(*alias.SourceImageResultID)))
	default:
		query.Where(mediaasset.StorageDriverEQ(alias.StorageDriver), mediaasset.ObjectKeyEQ(alias.ObjectKey))
		if alias.StorageConfigID == nil {
			query.Where(mediaasset.StorageConfigIDIsNil())
		} else {
			query.Where(mediaasset.StorageConfigIDEQ(*alias.StorageConfigID))
		}
	}
	media, err = query.Only(ctx)
	if repoent.IsNotFound(err) && alias.MediaAssetID == nil {
		return s.canonicalizeLateReference(ctx, userID, alias)
	}
	if repoent.IsNotFound(err) {
		return domainassets.ReferenceAsset{}, repoerr.ErrNotFound
	}
	if err != nil {
		return domainassets.ReferenceAsset{}, err
	}
	return s.projectReferenceMedia(ctx, id, media, alias)
}

func (s *AssetsStore) projectReferenceMedia(ctx context.Context, requestedID uuid.UUID, media *repoent.MediaAsset, alias *repoent.ReferenceAsset) (domainassets.ReferenceAsset, error) {
	if media.DeletedAt != nil || media.Status == "deleted" || media.Status == "deleting" {
		return domainassets.ReferenceAsset{}, errs.WithDetails(errs.New(409, "MEDIA_ASSET_DELETED", "引用素材已删除"), map[string]any{"asset_id": requestedID.String(), "status": media.Status, "media_type": media.MediaType, "expected_media_type": "image"})
	}
	if media.MediaType != "image" {
		return domainassets.ReferenceAsset{}, errs.WithDetails(errs.New(400, "MEDIA_ASSET_TYPE_MISMATCH", "图片引用仅支持图片素材"), map[string]any{"asset_id": requestedID.String(), "status": media.Status, "media_type": media.MediaType, "expected_media_type": "image"})
	}
	asset := domainassets.ReferenceAsset{ID: requestedID.String(), MediaAssetID: media.ID.String(), Name: media.Name, DisplayName: media.Name, Status: media.Status, UploadSource: media.SourceType, StorageDriver: media.StorageDriver, StorageConfigID: nullableUUID(media.StorageConfigID), ObjectKey: media.ObjectKey, MimeType: media.MimeType, FileSizeBytes: media.FileSizeBytes, SHA256: media.Sha256, CreatedAt: media.CreatedAt}
	if normalized, err := domainassets.NormalizeReferenceName(asset.Name); err == nil {
		asset.Name = normalized
	} else {
		asset.Name = "图片-" + media.ID.String()
	}
	if media.Width != nil {
		asset.Width = *media.Width
	}
	if media.Height != nil {
		asset.Height = *media.Height
	}
	if alias == nil {
		var err error
		alias, err = s.client.ReferenceAsset.Query().Where(referenceasset.IDEQ(requestedID), referenceasset.UserIDEQ(media.UserID)).Only(ctx)
		if err != nil && !repoent.IsNotFound(err) {
			return domainassets.ReferenceAsset{}, err
		}
	}
	if alias != nil {
		if alias.Name != nil {
			asset.Name = *alias.Name
		}
		asset.APIKeyID = alias.APIKeyID
		asset.UploadSource = alias.UploadSource
	}
	sourceID := media.LegacyImageResultID
	if sourceID == nil && alias != nil {
		sourceID = alias.SourceImageResultID
	}
	if sourceID != nil {
		asset.SourceImageResultID = sourceID.String()
		source, err := s.client.ImageResult.Get(ctx, *sourceID)
		if err != nil && !repoent.IsNotFound(err) {
			return domainassets.ReferenceAsset{}, err
		}
		if source != nil && source.UserID == media.UserID {
			task, err := s.client.ImageTask.Get(ctx, source.TaskID)
			if err != nil && !repoent.IsNotFound(err) {
				return domainassets.ReferenceAsset{}, err
			}
			if task != nil && task.UserID == media.UserID {
				asset.GenerationSnapshot, err = referenceGenerationSnapshot(task)
				if err != nil {
					return domainassets.ReferenceAsset{}, err
				}
			}
		}
	}
	return asset, nil
}

func nullableUUID(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func persistUploadedReferenceMedia(ctx context.Context, tx *repoent.Tx, userID int64, asset domainassets.ReferenceAsset) error {
	projectRow, err := tx.Project.Query().Where(project.UserIDEQ(userID), project.IsDefaultEQ(true), project.StatusEQ("active"), project.DeletedAtIsNil()).Only(ctx)
	if repoent.IsNotFound(err) {
		projectRow, err = createDefaultProjectInTx(ctx, tx, userID)
	}
	if err != nil {
		return err
	}
	name := asset.Name
	if strings.TrimSpace(name) == "" {
		name = "图片"
	}
	create := tx.MediaAsset.Create().SetID(uuid.MustParse(asset.ID)).SetUserID(userID).SetProjectID(projectRow.ID).SetName(name).SetNameKey(strings.ToLower(name)).SetMediaType("image").SetSourceType("local_upload").SetStatus(asset.Status).SetStorageDriver(defaultString(asset.StorageDriver, "local")).SetObjectKey(asset.ObjectKey).SetMimeType(asset.MimeType).SetFileSizeBytes(asset.FileSizeBytes).SetSha256(asset.SHA256).SetWidth(asset.Width).SetHeight(asset.Height)
	if asset.StorageConfigID != "" {
		id, err := uuid.Parse(asset.StorageConfigID)
		if err != nil {
			return err
		}
		create.SetStorageConfigID(id)
	}
	if !asset.CreatedAt.IsZero() {
		create.SetCreatedAt(asset.CreatedAt)
	}
	return create.Exec(ctx)
}

// Old application instances can write legacy rows after the startup backfill.
// Adopt those rows on first use rather than trusting stale legacy readiness.
func (s *AssetsStore) canonicalizeLateReference(ctx context.Context, userID int64, alias *repoent.ReferenceAsset) (domainassets.ReferenceAsset, error) {
	legacy, err := s.ensureReferenceAssetName(ctx, userID, alias)
	if err != nil {
		return domainassets.ReferenceAsset{}, err
	}
	canonicalID, err := withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (uuid.UUID, error) {
		row, err := tx.ReferenceAsset.Query().Where(referenceasset.IDEQ(alias.ID), referenceasset.UserIDEQ(userID), referenceasset.DeletedAtIsNil(), referenceasset.StatusNEQ("deleted")).Only(ctx)
		if repoent.IsNotFound(err) {
			return uuid.Nil, repoerr.ErrNotFound
		}
		if err != nil {
			return uuid.Nil, err
		}
		if row.MediaAssetID != nil {
			return *row.MediaAssetID, nil
		}
		var mediaID uuid.UUID
		if row.SourceImageResultID != nil {
			media, err := tx.MediaAsset.Query().Where(mediaasset.UserIDEQ(userID), mediaasset.Or(mediaasset.IDEQ(*row.SourceImageResultID), mediaasset.LegacyImageResultIDEQ(*row.SourceImageResultID))).Only(ctx)
			if repoent.IsNotFound(err) {
				media, err = ensureMediaAsset(ctx, tx, userID, *row.SourceImageResultID)
			}
			if err != nil {
				return uuid.Nil, err
			}
			mediaID = media.ID
		} else {
			// Repeat the identity lookup inside the transaction before creating a row.
			query := tx.MediaAsset.Query().Where(mediaasset.UserIDEQ(userID), mediaasset.StorageDriverEQ(row.StorageDriver), mediaasset.ObjectKeyEQ(row.ObjectKey))
			if row.StorageConfigID == nil {
				query.Where(mediaasset.StorageConfigIDIsNil())
			} else {
				query.Where(mediaasset.StorageConfigIDEQ(*row.StorageConfigID))
			}
			media, err := query.Only(ctx)
			if repoent.IsNotFound(err) {
				if err := persistUploadedReferenceMedia(ctx, tx, userID, legacy); err != nil {
					return uuid.Nil, err
				}
				mediaID = row.ID
			} else if err != nil {
				return uuid.Nil, err
			} else {
				mediaID = media.ID
			}
		}
		if err := tx.ReferenceAsset.UpdateOneID(row.ID).SetMediaAssetID(mediaID).SetOwnsObject(false).Exec(ctx); err != nil {
			return uuid.Nil, err
		}
		return mediaID, nil
	})
	if err != nil {
		return domainassets.ReferenceAsset{}, err
	}
	media, err := s.client.MediaAsset.Query().Where(mediaasset.IDEQ(canonicalID), mediaasset.UserIDEQ(userID)).Only(ctx)
	if err != nil {
		return domainassets.ReferenceAsset{}, err
	}
	return s.projectReferenceMedia(ctx, alias.ID, media, alias)
}
