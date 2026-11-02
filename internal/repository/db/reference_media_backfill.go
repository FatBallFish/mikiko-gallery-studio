package db

import (
	"context"
	"fmt"
	"strings"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/migrationcheckpoint"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/referenceasset"
	"github.com/google/uuid"
)

const referenceMediaMigration = "reference_media_assets_v1"

// BackfillReferenceMediaAssets preserves legacy IDs as compatibility mappings.
// Re-scan on every startup: a completed checkpoint must not hide writes made by
// an older instance during a rolling upgrade. Each bounded transaction locks
// the shared checkpoint before resolving identities or transferring ownership.
func BackfillReferenceMediaAssets(ctx context.Context, client *repoent.Client) (int, error) {
	if client == nil {
		return 0, fmt.Errorf("reference media backfill client is required")
	}
	if err := RetireReferenceMediaAliasUniqueness(ctx, client); err != nil {
		return 0, err
	}
	_, err := runLegacyStorageIdentityBatchWithRetry(ctx, func() (int, error) {
		_, err := client.MigrationCheckpoint.Query().Where(migrationcheckpoint.NameEQ(referenceMediaMigration)).Only(ctx)
		if repoent.IsNotFound(err) {
			_, err = client.MigrationCheckpoint.Create().SetName(referenceMediaMigration).SetPhase("assets").Save(ctx)
		}
		return 0, err
	})
	if err != nil {
		return 0, fmt.Errorf("prepare reference media migration: %w", err)
	}
	var cursor uuid.UUID
	total := 0
	for {
		var next uuid.UUID
		count, err := runLegacyStorageIdentityBatchWithRetry(ctx, func() (int, error) {
			next = cursor
			tx, err := client.Tx(ctx)
			if err != nil {
				return 0, err
			}
			defer func() { _ = tx.Rollback() }()
			checkpoint, err := tx.MigrationCheckpoint.Query().Where(migrationcheckpoint.NameEQ(referenceMediaMigration), lockLegacyStorageCheckpoint()).Only(ctx)
			if err != nil {
				return 0, err
			}
			query := tx.ReferenceAsset.Query()
			if cursor != uuid.Nil {
				query.Where(referenceasset.IDGT(cursor))
			}
			rows, err := query.Order(repoent.Asc(referenceasset.FieldID)).Limit(100).All(ctx)
			if err != nil {
				return 0, err
			}
			for _, row := range rows {
				if err := backfillReferenceMediaRow(ctx, tx, row); err != nil {
					return 0, fmt.Errorf("reference asset %s: %w", row.ID, err)
				}
				next = row.ID
			}
			if err := tx.MigrationCheckpoint.UpdateOneID(checkpoint.ID).SetPhase("assets").SetCompleted(len(rows) == 0).Exec(ctx); err != nil {
				return 0, err
			}
			if err := tx.Commit(); err != nil {
				return 0, err
			}
			return len(rows), nil
		})
		if err != nil {
			return total, fmt.Errorf("backfill reference media assets: %w", err)
		}
		total += count
		if count == 0 {
			return total, nil
		}
		cursor = next
	}
}

func backfillReferenceMediaRow(ctx context.Context, tx *repoent.Tx, row *repoent.ReferenceAsset) error {
	var target *repoent.MediaAsset
	var err error
	switch {
	case row.MediaAssetID != nil:
		target, err = tx.MediaAsset.Get(ctx, *row.MediaAssetID)
	case row.SourceImageResultID != nil:
		target, err = tx.MediaAsset.Query().Where(mediaasset.LegacyImageResultIDEQ(*row.SourceImageResultID)).Only(ctx)
		if repoent.IsNotFound(err) {
			source, sourceErr := tx.ImageResult.Get(ctx, *row.SourceImageResultID)
			if sourceErr != nil {
				return fmt.Errorf("resolve source image: %w", sourceErr)
			}
			if source.UserID != row.UserID {
				return fmt.Errorf("source image owner mismatch")
			}
			projectID, projectErr := backfillMediaProjectID(ctx, tx, source)
			if projectErr != nil {
				return projectErr
			}
			created, insertErr := insertMediaAssetBackfillRow(ctx, tx, source, projectID)
			if insertErr != nil {
				return insertErr
			}
			if created && source.DeletedAt != nil {
				if err = tx.MediaAsset.UpdateOneID(source.ID).SetStatus("deleted").SetDeletedAt(*source.DeletedAt).Exec(ctx); err != nil {
					return err
				}
			}
			target, err = tx.MediaAsset.Get(ctx, source.ID)
		}
	default:
		target, err = tx.MediaAsset.Get(ctx, row.ID)
		if repoent.IsNotFound(err) {
			query := tx.MediaAsset.Query().Where(mediaasset.ObjectKeyEQ(row.ObjectKey))
			if row.StorageConfigID != nil {
				query.Where(mediaasset.StorageConfigIDEQ(*row.StorageConfigID))
			} else {
				query.Where(mediaasset.StorageConfigIDIsNil(), mediaasset.StorageDriverEQ(row.StorageDriver))
			}
			target, err = query.Only(ctx)
		}
		if repoent.IsNotFound(err) {
			canonical, selectErr := standaloneReferenceCanonical(ctx, tx, row)
			if selectErr != nil {
				return selectErr
			}
			projectID, projectErr := ensureBackfillDefaultProject(ctx, tx.Client(), canonical.UserID)
			if projectErr != nil {
				return projectErr
			}
			name := mediaAssetName(canonical.ObjectKey, canonical.ID)
			if canonical.Name != nil && strings.TrimSpace(*canonical.Name) != "" {
				name = *canonical.Name
			}
			status := canonical.Status
			if canonical.DeletedAt != nil {
				status = "deleted"
			}
			create := tx.MediaAsset.Create().SetID(canonical.ID).SetUserID(canonical.UserID).SetProjectID(projectID).
				SetName(name).SetNameKey(strings.ToLower(strings.TrimSpace(name))).SetMediaType("image").SetSourceType("local_upload").
				SetStatus(status).SetStorageDriver(canonical.StorageDriver).SetNillableStorageConfigID(canonical.StorageConfigID).
				SetObjectKey(canonical.ObjectKey).SetMimeType(canonical.MimeType).SetFileSizeBytes(canonical.FileSizeBytes).SetSha256(canonical.Sha256).
				SetNillableWidth(canonical.Width).SetNillableHeight(canonical.Height).SetCreatedAt(canonical.CreatedAt).SetUpdatedAt(canonical.UpdatedAt).SetNillableDeletedAt(canonical.DeletedAt)
			target, err = create.Save(ctx)
		}
	}
	if err != nil {
		return fmt.Errorf("resolve canonical media: %w", err)
	}
	if target.UserID != row.UserID || target.MediaType != "image" {
		return fmt.Errorf("canonical media owner or type mismatch")
	}
	// Explicit mappings are authoritative: a media storage move may leave the
	// compatibility tuple stale. Only storage-discovered identities need equality.
	if row.MediaAssetID == nil && row.SourceImageResultID == nil && (target.ObjectKey != row.ObjectKey || !sameReferenceStorage(target, row)) {
		return fmt.Errorf("canonical media storage identity mismatch")
	}
	deleted := target.DeletedAt != nil || target.Status == "deleted"
	if !row.OwnsObject && row.MediaAssetID != nil && (!deleted || (row.Status == "deleted" && row.DeletedAt != nil)) {
		return nil
	}
	update := tx.ReferenceAsset.UpdateOneID(row.ID).SetOwnsObject(false).
		SetMediaAssetID(target.ID).SetUpdatedAt(row.UpdatedAt)
	if deleted {
		update.SetStatus("deleted")
		if row.DeletedAt == nil {
			tombstone := target.UpdatedAt
			if target.DeletedAt != nil {
				tombstone = *target.DeletedAt
			}
			update.SetDeletedAt(tombstone)
		}
	}
	return update.Exec(ctx)
}

func sameReferenceStorage(asset *repoent.MediaAsset, row *repoent.ReferenceAsset) bool {
	if asset.StorageConfigID != nil || row.StorageConfigID != nil {
		return asset.StorageConfigID != nil && row.StorageConfigID != nil && *asset.StorageConfigID == *row.StorageConfigID
	}
	return normalizeLegacyStorageDriver(asset.StorageDriver) == normalizeLegacyStorageDriver(row.StorageDriver)
}

// Choose from the entire storage identity before creating the first media row,
// so UUID ordering and batch boundaries cannot let a deleted alias hide a live
// upload. Existing media is resolved before this helper and is never revived.
func standaloneReferenceCanonical(ctx context.Context, tx *repoent.Tx, row *repoent.ReferenceAsset) (*repoent.ReferenceAsset, error) {
	query := tx.ReferenceAsset.Query().Where(referenceasset.UserIDEQ(row.UserID), referenceasset.ObjectKeyEQ(row.ObjectKey), referenceasset.MediaAssetIDIsNil(), referenceasset.SourceImageResultIDIsNil(), referenceasset.DeletedAtIsNil(), referenceasset.StatusNEQ("deleted"))
	if row.StorageConfigID != nil {
		query.Where(referenceasset.StorageConfigIDEQ(*row.StorageConfigID))
	} else {
		query.Where(referenceasset.StorageConfigIDIsNil())
		if normalizeLegacyStorageDriver(row.StorageDriver) == "local" {
			query.Where(referenceasset.StorageDriverIn("", "local"))
		} else {
			query.Where(referenceasset.StorageDriverEQ(row.StorageDriver))
		}
	}
	canonical, err := query.Order(repoent.Asc(referenceasset.FieldID)).First(ctx)
	if repoent.IsNotFound(err) {
		return row, nil
	}
	if err != nil {
		return nil, fmt.Errorf("choose live standalone reference: %w", err)
	}
	return canonical, nil
}
