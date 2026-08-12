package db

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imageresult"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/migrationcheckpoint"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/predicate"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/project"
)

const (
	defaultMediaAssetBackfillBatchSize = 100
	mediaAssetBackfillMigrationName    = "media_asset_backfill_v1"
	mediaAssetBackfillPhaseAssets      = "assets"
	mediaAssetBackfillPhaseDone        = "done"
)

type MediaAssetBackfillCheckpoint struct {
	CreatedAt time.Time `json:"created_at"`
	ID        uuid.UUID `json:"id"`
}

type MediaAssetBackfillOptions struct {
	BatchSize  int
	Checkpoint MediaAssetBackfillCheckpoint
	DryRun     bool
}

type MediaAssetBackfillResult struct {
	Processed   int                          `json:"processed"`
	Created     int                          `json:"created"`
	WouldCreate int                          `json:"would_create"`
	Skipped     int                          `json:"skipped"`
	Done        bool                         `json:"done"`
	Checkpoint  MediaAssetBackfillCheckpoint `json:"checkpoint"`
}

type MediaAssetBackfillVerifyOptions struct {
	SampleSize int
}

type MediaAssetBackfillAggregate struct {
	StorageIdentity string `json:"storage_identity"`
	SourceCount     int    `json:"source_count"`
	AssetCount      int    `json:"asset_count"`
	SourceBytes     int64  `json:"source_bytes"`
	AssetBytes      int64  `json:"asset_bytes"`
}

type MediaAssetBackfillSample struct {
	ImageResultID uuid.UUID `json:"image_result_id"`
	UserID        int64     `json:"user_id"`
	ProjectID     uuid.UUID `json:"project_id"`
	ObjectKey     string    `json:"object_key"`
	Valid         bool      `json:"valid"`
}

type MediaAssetBackfillVerification struct {
	Valid             bool                          `json:"valid"`
	SourceCount       int                           `json:"source_count"`
	AssetCount        int                           `json:"asset_count"`
	SourceBytes       int64                         `json:"source_bytes"`
	AssetBytes        int64                         `json:"asset_bytes"`
	MismatchedSamples int                           `json:"mismatched_samples"`
	Aggregates        []MediaAssetBackfillAggregate `json:"aggregates"`
	Samples           []MediaAssetBackfillSample    `json:"samples"`
}

type MediaAssetBackfillProcessorOptions struct {
	BatchSize int
}

type MediaAssetBackfillProcessor struct {
	client *repoent.Client
	opts   MediaAssetBackfillProcessorOptions
}

type MediaAssetBackfillProgress struct {
	Processed int  `json:"processed"`
	Completed bool `json:"completed"`
}

func NewMediaAssetBackfillProcessor(client *repoent.Client, opts MediaAssetBackfillProcessorOptions) *MediaAssetBackfillProcessor {
	if opts.BatchSize <= 0 || opts.BatchSize > 1000 {
		opts.BatchSize = defaultMediaAssetBackfillBatchSize
	}
	return &MediaAssetBackfillProcessor{client: client, opts: opts}
}

func (processor *MediaAssetBackfillProcessor) ProcessOnce(ctx context.Context) (bool, error) {
	result, err := processor.ProcessBatch(ctx)
	return result.Processed > 0, err
}

func (processor *MediaAssetBackfillProcessor) ProcessBatch(ctx context.Context) (MediaAssetBackfillResult, error) {
	if processor == nil || processor.client == nil {
		return MediaAssetBackfillResult{}, fmt.Errorf("media asset backfill processor is unavailable")
	}
	checkpoint, err := loadMediaAssetBackfillCheckpoint(ctx, processor.client)
	if err != nil {
		return MediaAssetBackfillResult{}, err
	}
	tx, err := processor.client.Tx(ctx)
	if err != nil {
		return MediaAssetBackfillResult{}, fmt.Errorf("start media asset backfill batch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	checkpoint, err = tx.MigrationCheckpoint.Query().Where(
		migrationcheckpoint.IDEQ(checkpoint.ID), lockMediaAssetBackfillCheckpoint(),
	).Only(ctx)
	if err != nil {
		return MediaAssetBackfillResult{}, fmt.Errorf("lock media asset backfill checkpoint: %w", err)
	}
	if checkpoint.Completed || checkpoint.Phase == mediaAssetBackfillPhaseDone {
		pending, err := mediaAssetBackfillHasPendingSources(ctx, tx.Client())
		if err != nil {
			return MediaAssetBackfillResult{}, err
		}
		if !pending {
			return MediaAssetBackfillResult{Done: true}, nil
		}
		checkpoint, err = tx.MigrationCheckpoint.UpdateOneID(checkpoint.ID).
			SetPhase(mediaAssetBackfillPhaseAssets).SetCompleted(false).ClearAfterResultID().ClearAfterCreatedAt().Save(ctx)
		if err != nil {
			return MediaAssetBackfillResult{}, fmt.Errorf("reopen media asset backfill checkpoint: %w", err)
		}
	}
	cursor, err := mediaAssetCheckpointCursor(ctx, tx.Client(), checkpoint)
	if err != nil {
		return MediaAssetBackfillResult{}, err
	}
	result, err := backfillMediaAssetsTx(ctx, tx, MediaAssetBackfillOptions{BatchSize: processor.opts.BatchSize, Checkpoint: cursor})
	if err != nil {
		return MediaAssetBackfillResult{}, err
	}
	update := tx.MigrationCheckpoint.UpdateOneID(checkpoint.ID).AddProcessedRows(result.Processed)
	if result.Processed > 0 {
		update.SetAfterResultID(result.Checkpoint.ID).SetAfterCreatedAt(result.Checkpoint.CreatedAt)
	}
	if result.Done {
		update.SetPhase(mediaAssetBackfillPhaseDone).SetCompleted(true)
	}
	if _, err := update.Save(ctx); err != nil {
		return MediaAssetBackfillResult{}, fmt.Errorf("checkpoint media asset backfill: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return MediaAssetBackfillResult{}, fmt.Errorf("commit media asset backfill batch: %w", err)
	}
	return result, nil
}

func ReadMediaAssetBackfillProgress(ctx context.Context, client *repoent.Client) (MediaAssetBackfillProgress, error) {
	if client == nil {
		return MediaAssetBackfillProgress{}, fmt.Errorf("media asset backfill client is unavailable")
	}
	checkpoint, err := client.MigrationCheckpoint.Query().Where(migrationcheckpoint.NameEQ(mediaAssetBackfillMigrationName)).Only(ctx)
	if repoent.IsNotFound(err) {
		return MediaAssetBackfillProgress{}, nil
	}
	if err != nil {
		return MediaAssetBackfillProgress{}, fmt.Errorf("query media asset backfill progress: %w", err)
	}
	return MediaAssetBackfillProgress{Processed: checkpoint.ProcessedRows, Completed: checkpoint.Completed || checkpoint.Phase == mediaAssetBackfillPhaseDone}, nil
}

func BackfillMediaAssets(ctx context.Context, client *repoent.Client, options MediaAssetBackfillOptions) (MediaAssetBackfillResult, error) {
	if client == nil {
		return MediaAssetBackfillResult{}, fmt.Errorf("media asset backfill client is unavailable")
	}
	tx, err := client.Tx(ctx)
	if err != nil {
		return MediaAssetBackfillResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := backfillMediaAssetsTx(ctx, tx, options)
	if err != nil {
		return MediaAssetBackfillResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaAssetBackfillResult{}, err
	}
	return result, nil
}

func backfillMediaAssetsTx(ctx context.Context, tx *repoent.Tx, options MediaAssetBackfillOptions) (MediaAssetBackfillResult, error) {
	batchSize := options.BatchSize
	if batchSize <= 0 {
		batchSize = defaultMediaAssetBackfillBatchSize
	}
	if batchSize > 1000 {
		batchSize = 1000
	}
	query := tx.ImageResult.Query().Where(imageresult.DeletedAtIsNil())
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
	for _, row := range rows {
		result.Processed++
		result.Checkpoint = MediaAssetBackfillCheckpoint{CreatedAt: row.CreatedAt, ID: row.ID}
		projectID, err := backfillMediaProjectID(ctx, tx, row)
		if err != nil {
			return MediaAssetBackfillResult{}, err
		}
		if options.DryRun {
			exists, err := tx.MediaAsset.Query().Where(mediaasset.IDEQ(row.ID)).Exist(ctx)
			if err != nil {
				return MediaAssetBackfillResult{}, err
			}
			if exists {
				result.Skipped++
			} else {
				result.WouldCreate++
			}
			continue
		}
		created, err := insertMediaAssetBackfillRow(ctx, tx, row, projectID)
		if err != nil {
			return MediaAssetBackfillResult{}, err
		}
		if created {
			result.Created++
		} else {
			result.Skipped++
		}
	}
	return result, nil
}

func loadMediaAssetBackfillCheckpoint(ctx context.Context, client *repoent.Client) (*repoent.MigrationCheckpoint, error) {
	checkpoint, err := client.MigrationCheckpoint.Query().Where(migrationcheckpoint.NameEQ(mediaAssetBackfillMigrationName)).Only(ctx)
	if err == nil {
		return checkpoint, nil
	}
	if !repoent.IsNotFound(err) {
		return nil, fmt.Errorf("query media asset backfill checkpoint: %w", err)
	}
	checkpoint, err = client.MigrationCheckpoint.Create().SetName(mediaAssetBackfillMigrationName).SetPhase(mediaAssetBackfillPhaseAssets).Save(ctx)
	if repoent.IsConstraintError(err) {
		checkpoint, err = client.MigrationCheckpoint.Query().Where(migrationcheckpoint.NameEQ(mediaAssetBackfillMigrationName)).Only(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("create media asset backfill checkpoint: %w", err)
	}
	return checkpoint, nil
}

func mediaAssetCheckpointCursor(ctx context.Context, client *repoent.Client, checkpoint *repoent.MigrationCheckpoint) (MediaAssetBackfillCheckpoint, error) {
	if checkpoint.AfterResultID == nil {
		return MediaAssetBackfillCheckpoint{}, nil
	}
	if checkpoint.AfterCreatedAt != nil {
		return MediaAssetBackfillCheckpoint{CreatedAt: *checkpoint.AfterCreatedAt, ID: *checkpoint.AfterResultID}, nil
	}
	row, err := client.ImageResult.Query().Where(imageresult.IDEQ(*checkpoint.AfterResultID)).Only(ctx)
	if repoent.IsNotFound(err) {
		// Legacy checkpoints did not persist created_at. Restarting from zero is
		// safe because inserts are identity-checked and idempotent.
		return MediaAssetBackfillCheckpoint{}, nil
	}
	if err != nil {
		return MediaAssetBackfillCheckpoint{}, fmt.Errorf("load media asset backfill checkpoint source: %w", err)
	}
	return MediaAssetBackfillCheckpoint{CreatedAt: row.CreatedAt, ID: row.ID}, nil
}

func mediaAssetBackfillHasPendingSources(ctx context.Context, client *repoent.Client) (bool, error) {
	pending, err := client.ImageResult.Query().Where(
		imageresult.DeletedAtIsNil(),
		missingMediaAssetBackfillTarget(),
	).Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("query pending media asset backfill sources: %w", err)
	}
	return pending, nil
}

func missingMediaAssetBackfillTarget() predicate.ImageResult {
	return func(source *entsql.Selector) {
		assets := entsql.Table(mediaasset.Table)
		mapped := entsql.Select(assets.C(mediaasset.FieldID)).From(assets).Where(entsql.And(
			entsql.IsNull(assets.C(mediaasset.FieldDeletedAt)),
			entsql.ColumnsEQ(assets.C(mediaasset.FieldLegacyImageResultID), source.C(imageresult.FieldID)),
		))
		source.Where(entsql.NotExists(mapped))
	}
}

func lockMediaAssetBackfillCheckpoint() func(*entsql.Selector) {
	return func(selector *entsql.Selector) {
		if selector.Dialect() == dialect.Postgres {
			selector.ForUpdate()
		}
	}
}

func insertMediaAssetBackfillRow(ctx context.Context, tx *repoent.Tx, row *repoent.ImageResult, projectID uuid.UUID) (bool, error) {
	name := mediaAssetName(row.ObjectKey, row.ID)
	var storageConfigID any
	if row.StorageConfigID != nil {
		storageConfigID = *row.StorageConfigID
	}
	var width, height any
	if row.Width > 0 {
		width = row.Width
	}
	if row.Height > 0 {
		height = row.Height
	}
	args := []any{
		row.ID, row.CreatedAt, row.UpdatedAt, row.UserID, row.ID, name, strings.ToLower(name), row.ImageGroup,
		"image", "generated", "ready", row.VisibilityStatus, storageConfigID, row.StorageDriver, "", row.ObjectKey,
		row.MimeType, row.FileSizeBytes, row.Sha256, width, height, "image", row.TaskID, int64(1), projectID,
	}
	placeholders := make([]string, len(args))
	for index := range args {
		placeholders[index] = "?"
		if tx.Client().DialectName() == dialect.Postgres {
			placeholders[index] = fmt.Sprintf("$%d", index+1)
		}
	}
	query := `INSERT INTO media_assets (` +
		`id, created_at, updated_at, user_id, legacy_image_result_id, name, name_key, group_name, media_type, source_type, status, visibility_status, ` +
		`storage_config_id, storage_driver, bucket, object_key, mime_type, file_size_bytes, sha256, width, height, source_task_kind, source_task_id, version, project_id` +
		`) VALUES (` + strings.Join(placeholders, ",") + `) ON CONFLICT (id) DO NOTHING`
	affected, err := tx.ExecRawAffected(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("insert media asset backfill row: %w", err)
	}
	if affected == 1 {
		return true, nil
	}
	existing, err := tx.MediaAsset.Query().Where(mediaasset.IDEQ(row.ID)).Only(ctx)
	if err != nil {
		return false, fmt.Errorf("load conflicting media asset %s: %w", row.ID, err)
	}
	if existing.LegacyImageResultID == nil || *existing.LegacyImageResultID != row.ID || existing.UserID != row.UserID || existing.ObjectKey != row.ObjectKey || existing.MediaType != "image" || existing.SourceType != "generated" {
		return false, fmt.Errorf("media asset %s conflicts with image result %s", existing.ID, row.ID)
	}
	return false, nil
}

func VerifyMediaAssetBackfill(ctx context.Context, client *repoent.Client, options MediaAssetBackfillVerifyOptions) (MediaAssetBackfillVerification, error) {
	if client == nil {
		return MediaAssetBackfillVerification{}, fmt.Errorf("media asset backfill client is unavailable")
	}
	type aggregateRow struct {
		StorageConfigID *uuid.UUID `json:"storage_config_id"`
		StorageDriver   string     `json:"storage_driver"`
		Count           int        `json:"count"`
		Bytes           int64      `json:"bytes"`
	}
	var sourceRows []aggregateRow
	if err := client.ImageResult.Query().Where(imageresult.DeletedAtIsNil()).
		GroupBy(imageresult.FieldStorageConfigID, imageresult.FieldStorageDriver).
		Aggregate(repoent.As(repoent.Count(), "count"), repoent.As(repoent.Sum(imageresult.FieldFileSizeBytes), "bytes")).
		Scan(ctx, &sourceRows); err != nil {
		return MediaAssetBackfillVerification{}, fmt.Errorf("aggregate media asset backfill sources: %w", err)
	}
	var assetRows []aggregateRow
	if err := client.MediaAsset.Query().Where(mediaasset.LegacyImageResultIDNotNil(), mediaasset.DeletedAtIsNil()).
		GroupBy(mediaasset.FieldStorageConfigID, mediaasset.FieldStorageDriver).
		Aggregate(repoent.As(repoent.Count(), "count"), repoent.As(repoent.Sum(mediaasset.FieldFileSizeBytes), "bytes")).
		Scan(ctx, &assetRows); err != nil {
		return MediaAssetBackfillVerification{}, fmt.Errorf("aggregate media asset backfill targets: %w", err)
	}
	type totals struct {
		count int
		bytes int64
	}
	sourceTotals, assetTotals := map[string]totals{}, map[string]totals{}
	report := MediaAssetBackfillVerification{}
	for _, row := range sourceRows {
		identity := mediaBackfillStorageIdentity(row.StorageConfigID, row.StorageDriver)
		sourceTotals[identity] = totals{count: row.Count, bytes: row.Bytes}
		report.SourceCount += row.Count
		report.SourceBytes += row.Bytes
	}
	for _, row := range assetRows {
		identity := mediaBackfillStorageIdentity(row.StorageConfigID, row.StorageDriver)
		assetTotals[identity] = totals{count: row.Count, bytes: row.Bytes}
		report.AssetCount += row.Count
		report.AssetBytes += row.Bytes
	}
	identities := make(map[string]struct{}, len(sourceTotals)+len(assetTotals))
	for identity := range sourceTotals {
		identities[identity] = struct{}{}
	}
	for identity := range assetTotals {
		identities[identity] = struct{}{}
	}
	keys := make([]string, 0, len(identities))
	for identity := range identities {
		keys = append(keys, identity)
	}
	sort.Strings(keys)
	for _, identity := range keys {
		source, target := sourceTotals[identity], assetTotals[identity]
		report.Aggregates = append(report.Aggregates, MediaAssetBackfillAggregate{StorageIdentity: identity, SourceCount: source.count, AssetCount: target.count, SourceBytes: source.bytes, AssetBytes: target.bytes})
	}
	sampleSize := options.SampleSize
	if sampleSize <= 0 {
		sampleSize = 20
	}
	if sampleSize > 1000 {
		sampleSize = 1000
	}
	images, err := client.ImageResult.Query().Where(imageresult.DeletedAtIsNil()).Order(repoent.Asc(imageresult.FieldCreatedAt), repoent.Asc(imageresult.FieldID)).Limit(sampleSize).All(ctx)
	if err != nil {
		return MediaAssetBackfillVerification{}, fmt.Errorf("sample media asset backfill sources: %w", err)
	}
	imageIDs := make([]uuid.UUID, 0, len(images))
	for _, image := range images {
		imageIDs = append(imageIDs, image.ID)
	}
	assetsByLegacy := make(map[uuid.UUID]*repoent.MediaAsset, len(images))
	if len(imageIDs) > 0 {
		assets, err := client.MediaAsset.Query().Where(mediaasset.LegacyImageResultIDIn(imageIDs...), mediaasset.DeletedAtIsNil()).All(ctx)
		if err != nil {
			return MediaAssetBackfillVerification{}, fmt.Errorf("sample media asset backfill targets: %w", err)
		}
		for _, asset := range assets {
			if asset.LegacyImageResultID != nil {
				assetsByLegacy[*asset.LegacyImageResultID] = asset
			}
		}
	}
	for _, image := range images {
		asset := assetsByLegacy[image.ID]
		projectID, err := expectedMediaProjectID(ctx, client, image)
		if err != nil {
			return MediaAssetBackfillVerification{}, fmt.Errorf("resolve sampled media project: %w", err)
		}
		valid := asset != nil && asset.ID == image.ID && asset.UserID == image.UserID && asset.ProjectID == projectID && asset.ObjectKey == image.ObjectKey && asset.LegacyImageResultID != nil && *asset.LegacyImageResultID == image.ID
		if !valid {
			report.MismatchedSamples++
		}
		report.Samples = append(report.Samples, MediaAssetBackfillSample{ImageResultID: image.ID, UserID: image.UserID, ProjectID: projectID, ObjectKey: image.ObjectKey, Valid: valid})
	}
	report.Valid = report.SourceCount == report.AssetCount && report.SourceBytes == report.AssetBytes && report.MismatchedSamples == 0
	for _, aggregate := range report.Aggregates {
		if aggregate.SourceCount != aggregate.AssetCount || aggregate.SourceBytes != aggregate.AssetBytes {
			report.Valid = false
		}
	}
	return report, nil
}

func expectedMediaProjectID(ctx context.Context, client *repoent.Client, image *repoent.ImageResult) (uuid.UUID, error) {
	if image.ProjectID != nil {
		exists, err := client.Project.Query().Where(project.IDEQ(*image.ProjectID), project.UserIDEQ(image.UserID), project.StatusEQ("active"), project.DeletedAtIsNil()).Exist(ctx)
		if err != nil {
			return uuid.Nil, err
		}
		if exists {
			return *image.ProjectID, nil
		}
	}
	entity, err := client.Project.Query().Where(project.UserIDEQ(image.UserID), project.IsDefaultEQ(true), project.StatusEQ("active"), project.DeletedAtIsNil()).Only(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	return entity.ID, nil
}

func mediaBackfillStorageIdentity(configID *uuid.UUID, driver string) string {
	if configID != nil {
		return "config:" + configID.String()
	}
	return "legacy:" + strings.ToLower(strings.TrimSpace(driver))
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
