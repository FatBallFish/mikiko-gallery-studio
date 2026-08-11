package db

import (
	"context"
	"fmt"
	"time"

	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/migrationcheckpoint"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/predicate"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/referenceasset"
	"github.com/google/uuid"
)

const (
	referenceAssetNameMigrationName         = "reference_asset_names_v4"
	referenceAssetNameBackfillPhaseAssets   = "assets"
	referenceAssetNameBackfillPhaseValidate = "validate"
	referenceAssetNameBackfillPhaseDone     = "done"
)

type ReferenceAssetNameBackfillOptions struct {
	BatchSize  int
	MaxBatches int
	BatchPause time.Duration
	afterBatch func(ReferenceAssetNameBackfillProgress) error
}

type ReferenceAssetNameBackfillProgress struct {
	Phase         string
	Batches       int
	UpdatedRows   int
	ProcessedRows int
	Completed     bool
}

type ReferenceAssetNameBackfillIncompleteError struct {
	Progress ReferenceAssetNameBackfillProgress
}

func (e *ReferenceAssetNameBackfillIncompleteError) Error() string {
	return fmt.Sprintf("reference asset name backfill paused in phase %q after %d batches and %d processed rows; rerun migration to resume", e.Progress.Phase, e.Progress.Batches, e.Progress.ProcessedRows)
}

func requireCompletedReferenceAssetNameBackfill(progress ReferenceAssetNameBackfillProgress) error {
	if progress.Completed {
		return nil
	}
	return &ReferenceAssetNameBackfillIncompleteError{Progress: progress}
}

func RunReferenceAssetNameBackfill(ctx context.Context, client *repoent.Client, opts ReferenceAssetNameBackfillOptions) (ReferenceAssetNameBackfillProgress, error) {
	if client == nil {
		return ReferenceAssetNameBackfillProgress{}, fmt.Errorf("reference asset name backfill client is required")
	}
	if opts.BatchSize <= 0 || opts.BatchSize > 1000 {
		opts.BatchSize = 100
	}
	if opts.MaxBatches <= 0 {
		opts.MaxBatches = 100
	}
	checkpoint, err := loadReferenceAssetNameBackfillCheckpoint(ctx, client)
	if err != nil {
		return ReferenceAssetNameBackfillProgress{}, err
	}
	progress := referenceAssetNameBackfillProgress(checkpoint)
	if progress.Completed {
		reset, resetErr := resetReferenceAssetNameBackfillIfIncomplete(ctx, client, checkpoint)
		if resetErr != nil {
			return progress, resetErr
		}
		if !reset {
			return progress, nil
		}
		checkpoint, err = client.MigrationCheckpoint.Get(ctx, checkpoint.ID)
		if err != nil {
			return progress, fmt.Errorf("reload reset reference asset name checkpoint: %w", err)
		}
		return referenceAssetNameBackfillProgress(checkpoint), nil
	}

	for progress.Batches < opts.MaxBatches && !progress.Completed {
		updated, batchErr := runReferenceAssetNameBackfillBatch(ctx, client, checkpoint, opts.BatchSize)
		if batchErr != nil {
			return progress, batchErr
		}
		progress.Batches++
		progress.UpdatedRows += updated
		checkpoint, err = client.MigrationCheckpoint.Get(ctx, checkpoint.ID)
		if err != nil {
			return progress, fmt.Errorf("reload reference asset name checkpoint: %w", err)
		}
		progress.Phase = checkpoint.Phase
		progress.ProcessedRows = checkpoint.ProcessedRows
		progress.Completed = checkpoint.Completed
		if opts.afterBatch != nil {
			if err := opts.afterBatch(progress); err != nil {
				return progress, err
			}
		}
		if !progress.Completed && opts.BatchPause > 0 && progress.Batches < opts.MaxBatches {
			timer := time.NewTimer(opts.BatchPause)
			select {
			case <-ctx.Done():
				timer.Stop()
				return progress, fmt.Errorf("pause reference asset name backfill: %w", ctx.Err())
			case <-timer.C:
			}
		}
	}
	return progress, nil
}

func runReferenceAssetNameBackfillBatch(ctx context.Context, client *repoent.Client, checkpoint *repoent.MigrationCheckpoint, batchSize int) (int, error) {
	switch checkpoint.Phase {
	case referenceAssetNameBackfillPhaseAssets:
		for attempt := 0; attempt < 5; attempt++ {
			updated, err := backfillReferenceAssetNames(ctx, client, checkpoint, batchSize)
			if err == nil || !repoent.IsConstraintError(err) {
				return updated, err
			}
		}
		return 0, fmt.Errorf("backfill reference asset names exhausted conflict retries")
	case referenceAssetNameBackfillPhaseValidate:
		reset, err := resetReferenceAssetNameBackfillIfIncomplete(ctx, client, checkpoint)
		if err != nil || reset {
			return 0, err
		}
		if _, err := client.MigrationCheckpoint.UpdateOneID(checkpoint.ID).
			SetPhase(referenceAssetNameBackfillPhaseDone).
			SetCompleted(true).
			Save(ctx); err != nil {
			return 0, fmt.Errorf("complete reference asset name checkpoint: %w", err)
		}
		return 0, nil
	case referenceAssetNameBackfillPhaseDone:
		return 0, nil
	default:
		return 0, fmt.Errorf("unsupported reference asset name backfill phase %q", checkpoint.Phase)
	}
}

func backfillReferenceAssetNames(ctx context.Context, client *repoent.Client, checkpoint *repoent.MigrationCheckpoint, batchSize int) (int, error) {
	predicates := activeReferenceAssetsMissingNames()
	if checkpoint.AfterUserID > 0 {
		predicates = append(predicates, referenceasset.UserIDGT(int64(checkpoint.AfterUserID)))
	}
	first, err := client.ReferenceAsset.Query().
		Where(predicates...).
		Order(repoent.Asc(referenceasset.FieldUserID), repoent.Asc(referenceasset.FieldCreatedAt), repoent.Asc(referenceasset.FieldID)).
		First(ctx)
	if repoent.IsNotFound(err) {
		if _, updateErr := client.MigrationCheckpoint.UpdateOneID(checkpoint.ID).
			SetPhase(referenceAssetNameBackfillPhaseValidate).
			Save(ctx); updateErr != nil {
			return 0, fmt.Errorf("checkpoint reference asset name validation phase: %w", updateErr)
		}
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("find reference asset name backfill user: %w", err)
	}

	assets, err := client.ReferenceAsset.Query().
		Where(append(activeReferenceAssetsMissingNames(), referenceasset.UserIDEQ(first.UserID))...).
		Order(repoent.Asc(referenceasset.FieldCreatedAt), repoent.Asc(referenceasset.FieldID)).
		Limit(batchSize + 1).
		All(ctx)
	if err != nil {
		return 0, fmt.Errorf("list reference assets without names: %w", err)
	}
	hasMore := len(assets) > batchSize
	if hasMore {
		assets = assets[:batchSize]
	}

	tx, err := client.Tx(ctx)
	if err != nil {
		return 0, fmt.Errorf("start reference asset name backfill batch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	used, err := referenceAssetNamesInUse(ctx, tx, first.UserID, assets)
	if err != nil {
		return 0, err
	}
	updated := 0
	for _, asset := range assets {
		name, nameErr := historicalReferenceAssetName(asset, used)
		if nameErr != nil {
			return 0, nameErr
		}
		count, updateErr := tx.ReferenceAsset.Update().
			Where(append(activeReferenceAssetsMissingNames(), referenceasset.IDEQ(asset.ID), referenceasset.UserIDEQ(first.UserID))...).
			SetName(name).
			SetNameNormalized(name).
			Save(ctx)
		if updateErr != nil {
			return 0, fmt.Errorf("backfill reference asset %s name: %w", asset.ID, updateErr)
		}
		if count > 0 {
			used[name] = struct{}{}
			updated += count
		}
	}
	checkpointUpdate := tx.MigrationCheckpoint.UpdateOneID(checkpoint.ID).AddProcessedRows(updated)
	if !hasMore {
		checkpointUpdate.SetAfterUserID(int(first.UserID))
	}
	if _, err := checkpointUpdate.Save(ctx); err != nil {
		return 0, fmt.Errorf("checkpoint reference asset name backfill: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit reference asset name backfill: %w", err)
	}
	return updated, nil
}

func referenceAssetNamesInUse(ctx context.Context, tx *repoent.Tx, userID int64, batch []*repoent.ReferenceAsset) (map[string]struct{}, error) {
	selected := make(map[uuid.UUID]struct{}, len(batch))
	for _, asset := range batch {
		selected[asset.ID] = struct{}{}
	}
	assets, err := tx.ReferenceAsset.Query().Where(
		referenceasset.UserIDEQ(userID),
		referenceasset.DeletedAtIsNil(),
		referenceasset.StatusNEQ("deleted"),
		referenceasset.NameNormalizedNotNil(),
	).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list used reference asset names: %w", err)
	}
	used := make(map[string]struct{}, len(assets))
	for _, asset := range assets {
		if _, replacing := selected[asset.ID]; replacing || asset.NameNormalized == nil {
			continue
		}
		if normalized, normalizeErr := domainassets.NormalizeReferenceName(*asset.NameNormalized); normalizeErr == nil {
			used[normalized] = struct{}{}
		}
	}
	return used, nil
}

func historicalReferenceAssetName(asset *repoent.ReferenceAsset, used map[string]struct{}) (string, error) {
	for _, value := range []*string{asset.Name, asset.NameNormalized} {
		if value == nil {
			continue
		}
		normalized, err := domainassets.NormalizeReferenceName(*value)
		if err == nil {
			if _, exists := used[normalized]; !exists {
				return normalized, nil
			}
		}
	}
	for sequence := 1; sequence <= 10000; sequence++ {
		candidate := domainassets.ReferenceNameCandidate("", sequence)
		if _, exists := used[candidate]; !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("reference asset name space exhausted for user %d", asset.UserID)
}

func activeReferenceAssetsMissingNames() []predicate.ReferenceAsset {
	return []predicate.ReferenceAsset{
		referenceasset.DeletedAtIsNil(),
		referenceasset.StatusNEQ("deleted"),
		referenceasset.Or(
			referenceasset.NameIsNil(),
			referenceasset.NameEQ(""),
			referenceasset.NameNormalizedIsNil(),
			referenceasset.NameNormalizedEQ(""),
		),
	}
}

func loadReferenceAssetNameBackfillCheckpoint(ctx context.Context, client *repoent.Client) (*repoent.MigrationCheckpoint, error) {
	checkpoint, err := client.MigrationCheckpoint.Query().Where(migrationcheckpoint.NameEQ(referenceAssetNameMigrationName)).Only(ctx)
	if err == nil {
		return checkpoint, nil
	}
	if !repoent.IsNotFound(err) {
		return nil, fmt.Errorf("query reference asset name checkpoint: %w", err)
	}
	checkpoint, err = client.MigrationCheckpoint.Create().
		SetName(referenceAssetNameMigrationName).
		SetPhase(referenceAssetNameBackfillPhaseAssets).
		Save(ctx)
	if err != nil {
		if repoent.IsConstraintError(err) {
			checkpoint, err = client.MigrationCheckpoint.Query().Where(migrationcheckpoint.NameEQ(referenceAssetNameMigrationName)).Only(ctx)
		}
		if err != nil {
			return nil, fmt.Errorf("create reference asset name checkpoint: %w", err)
		}
	}
	return checkpoint, nil
}

func resetReferenceAssetNameBackfillIfIncomplete(ctx context.Context, client *repoent.Client, checkpoint *repoent.MigrationCheckpoint) (bool, error) {
	count, err := client.ReferenceAsset.Query().Where(activeReferenceAssetsMissingNames()...).Count(ctx)
	if err != nil {
		return false, fmt.Errorf("count reference assets without names: %w", err)
	}
	if count == 0 {
		return false, nil
	}
	if _, err := client.MigrationCheckpoint.UpdateOneID(checkpoint.ID).
		SetPhase(referenceAssetNameBackfillPhaseAssets).
		SetAfterUserID(0).
		SetCompleted(false).
		Save(ctx); err != nil {
		return false, fmt.Errorf("reset reference asset name checkpoint: %w", err)
	}
	return true, nil
}

func referenceAssetNameBackfillProgress(checkpoint *repoent.MigrationCheckpoint) ReferenceAssetNameBackfillProgress {
	return ReferenceAssetNameBackfillProgress{
		Phase: checkpoint.Phase, ProcessedRows: checkpoint.ProcessedRows, Completed: checkpoint.Completed,
	}
}
