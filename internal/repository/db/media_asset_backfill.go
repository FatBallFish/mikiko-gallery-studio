package db

import (
	"context"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imageresult"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/project"
)

const defaultMediaAssetBackfillBatchSize = 100

type MediaAssetBackfillCheckpoint struct {
	CreatedAt time.Time `json:"created_at"`
	ID        uuid.UUID `json:"id"`
}

type MediaAssetBackfillOptions struct {
	BatchSize  int
	Checkpoint MediaAssetBackfillCheckpoint
}

type MediaAssetBackfillResult struct {
	Processed  int                          `json:"processed"`
	Created    int                          `json:"created"`
	Skipped    int                          `json:"skipped"`
	Done       bool                         `json:"done"`
	Checkpoint MediaAssetBackfillCheckpoint `json:"checkpoint"`
}

func BackfillMediaAssets(ctx context.Context, client *repoent.Client, options MediaAssetBackfillOptions) (MediaAssetBackfillResult, error) {
	batchSize := options.BatchSize
	if batchSize <= 0 {
		batchSize = defaultMediaAssetBackfillBatchSize
	}
	if batchSize > 1000 {
		batchSize = 1000
	}
	query := client.ImageResult.Query().Where(imageresult.DeletedAtIsNil())
	if !options.Checkpoint.CreatedAt.IsZero() && options.Checkpoint.ID != uuid.Nil {
		query.Where(imageresult.Or(
			imageresult.CreatedAtGT(options.Checkpoint.CreatedAt),
			imageresult.And(imageresult.CreatedAtEQ(options.Checkpoint.CreatedAt), imageresult.IDGT(options.Checkpoint.ID)),
		))
	}
	rows, err := query.Order(repoent.Asc(imageresult.FieldCreatedAt), repoent.Asc(imageresult.FieldID)).Limit(batchSize).All(ctx)
	if err != nil {
		return MediaAssetBackfillResult{}, err
	}
	result := MediaAssetBackfillResult{Done: len(rows) < batchSize, Checkpoint: options.Checkpoint}
	if len(rows) == 0 {
		return result, nil
	}
	tx, err := client.Tx(ctx)
	if err != nil {
		return MediaAssetBackfillResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	for _, row := range rows {
		result.Processed++
		result.Checkpoint = MediaAssetBackfillCheckpoint{CreatedAt: row.CreatedAt, ID: row.ID}
		exists, err := tx.MediaAsset.Query().Where(mediaasset.IDEQ(row.ID)).Exist(ctx)
		if err != nil {
			return MediaAssetBackfillResult{}, err
		}
		if exists {
			result.Skipped++
			continue
		}
		projectID, err := backfillMediaProjectID(ctx, tx, row)
		if err != nil {
			return MediaAssetBackfillResult{}, err
		}
		name := mediaAssetName(row.ObjectKey, row.ID)
		builder := tx.MediaAsset.Create().
			SetID(row.ID).
			SetUserID(row.UserID).
			SetProjectID(projectID).
			SetLegacyImageResultID(row.ID).
			SetName(name).
			SetNameKey(strings.ToLower(name)).
			SetGroupName(row.ImageGroup).
			SetMediaType("image").
			SetSourceType("generated").
			SetStatus("ready").
			SetVisibilityStatus(row.VisibilityStatus).
			SetStorageDriver(row.StorageDriver).
			SetObjectKey(row.ObjectKey).
			SetMimeType(row.MimeType).
			SetFileSizeBytes(row.FileSizeBytes).
			SetSha256(row.Sha256).
			SetSourceTaskKind("image").
			SetSourceTaskID(row.TaskID).
			SetCreatedAt(row.CreatedAt).
			SetUpdatedAt(row.UpdatedAt)
		if row.StorageConfigID != nil {
			builder.SetStorageConfigID(*row.StorageConfigID)
		}
		if row.Width > 0 {
			builder.SetWidth(row.Width)
		}
		if row.Height > 0 {
			builder.SetHeight(row.Height)
		}
		if err := builder.Exec(ctx); err != nil {
			return MediaAssetBackfillResult{}, err
		}
		result.Created++
	}
	if err := tx.Commit(); err != nil {
		return MediaAssetBackfillResult{}, err
	}
	return result, nil
}

func backfillMediaProjectID(ctx context.Context, tx *repoent.Tx, row *repoent.ImageResult) (uuid.UUID, error) {
	if row.ProjectID != nil {
		exists, err := tx.Project.Query().Where(
			project.IDEQ(*row.ProjectID), project.UserIDEQ(row.UserID), project.StatusEQ("active"), project.DeletedAtIsNil(),
		).Exist(ctx)
		if err != nil {
			return uuid.Nil, err
		}
		if exists {
			return *row.ProjectID, nil
		}
	}
	entity, err := tx.Project.Query().Where(
		project.UserIDEQ(row.UserID), project.IsDefaultEQ(true), project.StatusEQ("active"), project.DeletedAtIsNil(),
	).Only(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	return entity.ID, nil
}

func mediaAssetName(objectKey string, id uuid.UUID) string {
	name := strings.TrimSpace(path.Base(strings.TrimSpace(objectKey)))
	if name == "" || name == "." || name == "/" {
		name = "image-" + id.String()
	}
	if len(name) > 255 {
		name = name[:255]
	}
	return name
}
