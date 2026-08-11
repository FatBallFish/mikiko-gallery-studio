package entstore

import (
	"context"
	"encoding/base64"
	"errors"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
	domaincleanup "github.com/fatballfish/pic-gallery/internal/domain/objectcleanup"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imageresult"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaassetreference"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaderivative"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaprocessingjob"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/project"
	mediaassetservice "github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

func (s *MediaStore) ListAssets(ctx context.Context, req mediaassetservice.AssetListRequest) (mediaassetservice.AssetPage, error) {
	if req.UserID <= 0 {
		return mediaassetservice.AssetPage{}, errs.BadRequest("user id is required")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 40
	}
	if limit > 100 {
		limit = 100
	}
	offset, err := decodeMediaCursor(req.Cursor)
	if err != nil {
		return mediaassetservice.AssetPage{}, errs.BadRequest("invalid media asset cursor")
	}
	entities, err := s.client.MediaAsset.Query().Where(mediaasset.UserIDEQ(req.UserID), mediaasset.DeletedAtIsNil(), mediaasset.StatusNEQ("deleted")).All(ctx)
	if err != nil {
		return mediaassetservice.AssetPage{}, err
	}
	items := make([]mediaassetservice.Asset, 0, len(entities))
	projectedLegacy := make(map[uuid.UUID]struct{}, len(entities))
	for _, entity := range entities {
		items = append(items, mapMediaAsset(entity))
		projectedLegacy[entity.ID] = struct{}{}
		if entity.LegacyImageResultID != nil {
			projectedLegacy[*entity.LegacyImageResultID] = struct{}{}
		}
	}
	legacyQuery := s.client.ImageResult.Query().Where(imageresult.UserIDEQ(req.UserID), imageresult.DeletedAtIsNil())
	legacy, err := legacyQuery.All(ctx)
	if err != nil {
		return mediaassetservice.AssetPage{}, err
	}
	defaultProjectID, err := s.defaultProjectID(ctx, req.UserID)
	if err != nil && !repoent.IsNotFound(err) {
		return mediaassetservice.AssetPage{}, err
	}
	for _, image := range legacy {
		if _, exists := projectedLegacy[image.ID]; exists {
			continue
		}
		item, ok := mapLegacyImageAsset(image, defaultProjectID)
		if ok {
			items = append(items, item)
		}
	}
	filtered := items[:0]
	for _, item := range items {
		if mediaAssetMatches(item, req) {
			filtered = append(filtered, item)
		}
	}
	sortMediaAssets(filtered, req.SortBy, req.SortOrder)
	if offset > len(filtered) {
		return mediaassetservice.AssetPage{}, errs.BadRequest("media asset cursor is out of range")
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	page := mediaassetservice.AssetPage{Items: append([]mediaassetservice.Asset(nil), filtered[offset:end]...)}
	if end < len(filtered) {
		page.NextCursor = encodeMediaCursor(end)
	}
	return page, nil
}

func (s *MediaStore) GetAsset(ctx context.Context, userID int64, id uuid.UUID) (mediaassetservice.Asset, error) {
	entity, err := s.client.MediaAsset.Query().Where(
		mediaasset.IDEQ(id),
		mediaasset.UserIDEQ(userID),
		mediaasset.Or(
			mediaasset.And(mediaasset.DeletedAtIsNil(), mediaasset.StatusNEQ("deleted")),
			mediaasset.HasReferencesWith(mediaassetreference.DeletedAtIsNil()),
		),
	).Only(ctx)
	if err == nil {
		return mapMediaAsset(entity), nil
	}
	if !repoent.IsNotFound(err) {
		return mediaassetservice.Asset{}, err
	}
	legacy, err := s.client.ImageResult.Query().Where(imageresult.IDEQ(id), imageresult.UserIDEQ(userID), imageresult.DeletedAtIsNil()).Only(ctx)
	if repoent.IsNotFound(err) {
		return mediaassetservice.Asset{}, errs.New(404, errs.CodeNotFound, "media asset not found")
	}
	if err != nil {
		return mediaassetservice.Asset{}, err
	}
	defaultProjectID, defaultErr := s.defaultProjectID(ctx, userID)
	if defaultErr != nil && legacy.ProjectID == nil {
		return mediaassetservice.Asset{}, defaultErr
	}
	asset, ok := mapLegacyImageAsset(legacy, defaultProjectID)
	if !ok {
		return mediaassetservice.Asset{}, errs.New(404, errs.CodeNotFound, "media asset not found")
	}
	return asset, nil
}

func (s *MediaStore) UpdateAsset(ctx context.Context, req mediaassetservice.UpdateAssetRequest) (mediaassetservice.Asset, error) {
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (mediaassetservice.Asset, error) {
		entity, err := ensureMediaAsset(ctx, tx, req.UserID, req.AssetID)
		if err != nil {
			return mediaassetservice.Asset{}, err
		}
		if req.ExpectedVersion <= 0 || entity.Version != req.ExpectedVersion {
			return mediaassetservice.Asset{}, errs.New(409, errs.CodeConflict, "media asset changed")
		}
		builder := tx.MediaAsset.UpdateOne(entity).AddVersion(1)
		if req.Name != nil {
			name := strings.TrimSpace(*req.Name)
			if name == "" || len(name) > 255 {
				return mediaassetservice.Asset{}, errs.BadRequest("media asset name is invalid")
			}
			builder.SetName(name).SetNameKey(strings.ToLower(name))
		}
		if req.GroupName != nil {
			group := strings.TrimSpace(*req.GroupName)
			if len(group) > 64 {
				return mediaassetservice.Asset{}, errs.BadRequest("media asset group is too long")
			}
			builder.SetGroupName(group)
		}
		if req.ProjectID != nil {
			exists, err := tx.Project.Query().Where(project.IDEQ(*req.ProjectID), project.UserIDEQ(req.UserID), project.StatusEQ("active"), project.DeletedAtIsNil()).Exist(ctx)
			if err != nil {
				return mediaassetservice.Asset{}, err
			}
			if !exists {
				return mediaassetservice.Asset{}, errs.New(404, errs.CodeNotFound, "target project not found")
			}
			builder.SetProjectID(*req.ProjectID)
		}
		updated, err := builder.Save(ctx)
		if err != nil {
			return mediaassetservice.Asset{}, err
		}
		if entity.LegacyImageResultID != nil {
			legacyUpdate := tx.ImageResult.UpdateOneID(*entity.LegacyImageResultID)
			if req.GroupName != nil {
				legacyUpdate.SetImageGroup(strings.TrimSpace(*req.GroupName))
			}
			if req.ProjectID != nil {
				legacyUpdate.SetProjectID(*req.ProjectID)
			}
			if err := legacyUpdate.Exec(ctx); err != nil {
				return mediaassetservice.Asset{}, err
			}
		}
		return mapMediaAsset(updated), nil
	})
}

func (s *MediaStore) DeleteAsset(ctx context.Context, req mediaassetservice.DeleteAssetRequest) (mediaassetservice.Asset, error) {
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (mediaassetservice.Asset, error) {
		entity, err := ensureMediaAsset(ctx, tx, req.UserID, req.AssetID)
		if err != nil {
			return mediaassetservice.Asset{}, err
		}
		if req.ExpectedVersion <= 0 || entity.Version != req.ExpectedVersion {
			return mediaassetservice.Asset{}, errs.New(409, errs.CodeConflict, "media asset changed")
		}
		derivatives, err := tx.MediaDerivative.Query().Where(mediaderivative.AssetIDEQ(entity.ID), mediaderivative.DeletedAtIsNil()).All(ctx)
		if err != nil {
			return mediaassetservice.Asset{}, err
		}
		now := time.Now().UTC()
		updated, err := tx.MediaAsset.UpdateOne(entity).SetStatus("deleted").SetDeletedAt(now).AddVersion(1).Save(ctx)
		if err != nil {
			return mediaassetservice.Asset{}, err
		}
		if entity.LegacyImageResultID != nil {
			if err := tx.ImageResult.UpdateOneID(*entity.LegacyImageResultID).SetDeletedAt(now).Exec(ctx); err != nil {
				return mediaassetservice.Asset{}, err
			}
		}
		if _, err := tx.MediaDerivative.Update().Where(mediaderivative.AssetIDEQ(entity.ID), mediaderivative.DeletedAtIsNil()).SetDeletedAt(now).Save(ctx); err != nil {
			return mediaassetservice.Asset{}, err
		}
		if err := enqueueMediaObjectCleanup(ctx, tx.Client(), entity.StorageConfigID, entity.StorageDriver, entity.Bucket, entity.ObjectKey); err != nil {
			return mediaassetservice.Asset{}, err
		}
		for _, derivative := range derivatives {
			if err := enqueueMediaObjectCleanup(ctx, tx.Client(), derivative.StorageConfigID, derivative.StorageDriver, derivative.Bucket, derivative.ObjectKey); err != nil {
				return mediaassetservice.Asset{}, err
			}
		}
		return mapMediaAsset(updated), nil
	})
}

func enqueueMediaObjectCleanup(ctx context.Context, client *repoent.Client, storageConfigID *uuid.UUID, driver, bucket, objectKey string) error {
	configID := ""
	if storageConfigID != nil {
		configID = storageConfigID.String()
	}
	_, err := enqueueObjectDeletionJob(ctx, client, domaincleanup.Identity{
		StorageConfigID: configID,
		StorageDriver:   driver,
		Bucket:          bucket,
		ObjectKey:       objectKey,
	})
	return err
}

func (s *MediaStore) ListReadyDerivatives(ctx context.Context, userID int64, id uuid.UUID) ([]mediaassetservice.AssetDerivative, error) {
	if _, err := s.GetAsset(ctx, userID, id); err != nil {
		return nil, err
	}
	entities, err := s.client.MediaDerivative.Query().Where(
		mediaderivative.AssetIDEQ(id), mediaderivative.StatusEQ("ready"), mediaderivative.DeletedAtIsNil(),
	).All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]mediaassetservice.AssetDerivative, 0, len(entities))
	for _, entity := range entities {
		configID := ""
		if entity.StorageConfigID != nil {
			configID = entity.StorageConfigID.String()
		}
		result = append(result, mediaassetservice.AssetDerivative{
			Kind: domainmedia.DerivativeKind(entity.Kind), Status: entity.Status, StorageConfigID: configID,
			StorageDriver: entity.StorageDriver, Bucket: entity.Bucket, ObjectKey: entity.ObjectKey, MIMEType: entity.MimeType,
		})
	}
	return result, nil
}

func (s *MediaStore) RetryAssetProcessing(ctx context.Context, userID int64, id uuid.UUID) (mediaassetservice.Asset, error) {
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (mediaassetservice.Asset, error) {
		entity, err := tx.MediaAsset.Query().Where(mediaasset.IDEQ(id), mediaasset.UserIDEQ(userID), mediaasset.DeletedAtIsNil()).Only(ctx)
		if repoent.IsNotFound(err) {
			return mediaassetservice.Asset{}, errs.New(404, errs.CodeNotFound, "media asset not found")
		}
		if err != nil {
			return mediaassetservice.Asset{}, err
		}
		job, err := tx.MediaProcessingJob.Query().Where(mediaprocessingjob.AssetIDEQ(id), mediaprocessingjob.JobTypeEQ("probe"), mediaprocessingjob.TransformVersionEQ(1)).Only(ctx)
		if repoent.IsNotFound(err) {
			_, err = tx.MediaProcessingJob.Create().SetAssetID(id).SetJobType("probe").SetTransformVersion(1).SetStatus("pending").Save(ctx)
		} else if err == nil {
			_, err = tx.MediaProcessingJob.UpdateOne(job).SetStatus("pending").SetAttemptCount(0).ClearNextRetryAt().ClearLeaseOwner().ClearLeaseExpiresAt().ClearErrorCode().ClearErrorMessage().Save(ctx)
		}
		if err != nil {
			return mediaassetservice.Asset{}, err
		}
		updated, err := tx.MediaAsset.UpdateOne(entity).SetStatus("processing").ClearProcessingErrorCode().ClearProcessingErrorMessage().AddVersion(1).Save(ctx)
		if err != nil {
			return mediaassetservice.Asset{}, err
		}
		return mapMediaAsset(updated), nil
	})
}

func ensureMediaAsset(ctx context.Context, tx *repoent.Tx, userID int64, id uuid.UUID) (*repoent.MediaAsset, error) {
	entity, err := tx.MediaAsset.Query().Where(mediaasset.IDEQ(id), mediaasset.UserIDEQ(userID), mediaasset.DeletedAtIsNil()).Only(ctx)
	if err == nil {
		return entity, nil
	}
	if !repoent.IsNotFound(err) {
		return nil, err
	}
	legacy, err := tx.ImageResult.Query().Where(imageresult.IDEQ(id), imageresult.UserIDEQ(userID), imageresult.DeletedAtIsNil()).Only(ctx)
	if repoent.IsNotFound(err) {
		return nil, errs.New(404, errs.CodeNotFound, "media asset not found")
	}
	if err != nil {
		return nil, err
	}
	projectID := legacy.ProjectID
	if projectID == nil {
		defaultProject, err := tx.Project.Query().Where(project.UserIDEQ(userID), project.IsDefaultEQ(true), project.StatusEQ("active"), project.DeletedAtIsNil()).Only(ctx)
		if err != nil {
			return nil, err
		}
		projectID = &defaultProject.ID
	}
	name := legacyAssetName(legacy.ObjectKey, legacy.ID, legacy.MimeType)
	builder := tx.MediaAsset.Create().
		SetID(legacy.ID).SetUserID(legacy.UserID).SetProjectID(*projectID).SetLegacyImageResultID(legacy.ID).
		SetName(name).SetNameKey(strings.ToLower(name)).SetGroupName(legacy.ImageGroup).
		SetMediaType("image").SetSourceType("generated").SetStatus("ready").SetVisibilityStatus(legacy.VisibilityStatus).
		SetStorageDriver(legacy.StorageDriver).SetObjectKey(legacy.ObjectKey).SetMimeType(legacy.MimeType).
		SetFileSizeBytes(legacy.FileSizeBytes).SetSha256(legacy.Sha256)
	if legacy.StorageConfigID != nil {
		builder.SetStorageConfigID(*legacy.StorageConfigID)
	}
	if legacy.Width > 0 {
		builder.SetWidth(legacy.Width)
	}
	if legacy.Height > 0 {
		builder.SetHeight(legacy.Height)
	}
	created, err := builder.Save(ctx)
	if repoent.IsConstraintError(err) {
		return tx.MediaAsset.Query().Where(mediaasset.IDEQ(id), mediaasset.UserIDEQ(userID)).Only(ctx)
	}
	return created, err
}

func (s *MediaStore) defaultProjectID(ctx context.Context, userID int64) (uuid.UUID, error) {
	entity, err := s.client.Project.Query().Where(project.UserIDEQ(userID), project.IsDefaultEQ(true), project.StatusEQ("active"), project.DeletedAtIsNil()).Only(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	return entity.ID, nil
}

func mapLegacyImageAsset(entity *repoent.ImageResult, defaultProjectID uuid.UUID) (mediaassetservice.Asset, bool) {
	projectID := defaultProjectID
	if entity.ProjectID != nil {
		projectID = *entity.ProjectID
	}
	if projectID == uuid.Nil {
		return mediaassetservice.Asset{}, false
	}
	configID := ""
	if entity.StorageConfigID != nil {
		configID = entity.StorageConfigID.String()
	}
	width, height := entity.Width, entity.Height
	return mediaassetservice.Asset{
		ID: entity.ID, UserID: entity.UserID, ProjectID: projectID, LegacyImageID: &entity.ID,
		Name: legacyAssetName(entity.ObjectKey, entity.ID, entity.MimeType), GroupName: entity.ImageGroup,
		MediaType: domainmedia.MediaTypeImage, SourceType: "generated", Status: "ready", VisibilityStatus: entity.VisibilityStatus,
		StorageConfigID: configID, StorageDriver: entity.StorageDriver, ObjectKey: entity.ObjectKey, MIMEType: entity.MimeType,
		FileSizeBytes: entity.FileSizeBytes, SHA256: entity.Sha256, Width: positiveIntPointer(width), Height: positiveIntPointer(height),
		Version: 1, CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt,
	}, true
}

func legacyAssetName(objectKey string, id uuid.UUID, mimeType string) string {
	name := path.Base(strings.TrimSpace(objectKey))
	if name == "" || name == "." || name == "/" || strings.Contains(name, "://") {
		extension := ".png"
		if strings.Contains(strings.ToLower(mimeType), "jpeg") {
			extension = ".jpg"
		} else if strings.Contains(strings.ToLower(mimeType), "webp") {
			extension = ".webp"
		}
		name = "image-" + id.String() + extension
	}
	if len(name) > 255 {
		name = "image-" + id.String()
	}
	return name
}

func positiveIntPointer(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}

func mediaAssetMatches(item mediaassetservice.Asset, req mediaassetservice.AssetListRequest) bool {
	if req.ProjectID != nil && item.ProjectID != *req.ProjectID {
		return false
	}
	if req.MediaType != "" && item.MediaType != req.MediaType {
		return false
	}
	if req.SourceType != "" && !strings.EqualFold(item.SourceType, strings.TrimSpace(req.SourceType)) {
		return false
	}
	if req.GroupName != "" && !strings.EqualFold(item.GroupName, strings.TrimSpace(req.GroupName)) {
		return false
	}
	if req.Status != "" && !strings.EqualFold(item.Status, strings.TrimSpace(req.Status)) {
		return false
	}
	keyword := strings.ToLower(strings.TrimSpace(req.Keyword))
	return keyword == "" || strings.Contains(strings.ToLower(item.Name), keyword)
}

func sortMediaAssets(items []mediaassetservice.Asset, sortBy, sortOrder string) {
	descending := !strings.EqualFold(strings.TrimSpace(sortOrder), "asc")
	sort.SliceStable(items, func(i, j int) bool {
		left, right := items[i], items[j]
		comparison := 0
		switch strings.ToLower(strings.TrimSpace(sortBy)) {
		case "name":
			comparison = strings.Compare(strings.ToLower(left.Name), strings.ToLower(right.Name))
		case "size":
			comparison = compareInt64(left.FileSizeBytes, right.FileSizeBytes)
		case "duration":
			comparison = compareInt64(pointerInt64Value(left.DurationMS), pointerInt64Value(right.DurationMS))
		default:
			if left.CreatedAt.Before(right.CreatedAt) {
				comparison = -1
			} else if left.CreatedAt.After(right.CreatedAt) {
				comparison = 1
			}
		}
		if comparison == 0 {
			comparison = strings.Compare(left.ID.String(), right.ID.String())
		}
		if descending {
			return comparison > 0
		}
		return comparison < 0
	})
}

func compareInt64(left, right int64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func pointerInt64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func encodeMediaCursor(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
}

func decodeMediaCursor(cursor string) (int, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return 0, nil
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(cursor)
	if err != nil {
		return 0, err
	}
	offset, err := strconv.Atoi(string(payload))
	if err != nil || offset < 0 {
		return 0, errors.New("invalid cursor")
	}
	return offset, nil
}
