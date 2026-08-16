package db

import (
	"context"
	"fmt"
	"time"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/migrationcheckpoint"
)

const videoNativePricingMigrationName = "video_native_pricing_v1"

func RetireLegacyVideoPricingConfiguration(ctx context.Context, client *repoent.Client) (err error) {
	if client == nil {
		return fmt.Errorf("video pricing migration client is required")
	}
	completed, err := client.MigrationCheckpoint.Query().Where(
		migrationcheckpoint.NameEQ(videoNativePricingMigrationName), migrationcheckpoint.CompletedEQ(true),
	).Exist(ctx)
	if err != nil || completed {
		return err
	}
	tx, err := client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("start video pricing migration: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	now := time.Now().UTC()
	if _, err = tx.VideoRouteConfig.Update().SetEnabled(false).Save(ctx); err != nil {
		return fmt.Errorf("disable legacy video routes: %w", err)
	}
	if _, err = tx.VideoPricingStrategy.Update().SetEnabled(false).SetDeletedAt(now).Save(ctx); err != nil {
		return fmt.Errorf("retire legacy video pricing strategies: %w", err)
	}
	if _, err = tx.VideoPriceRule.Update().SetEnabled(false).SetDeletedAt(now).Save(ctx); err != nil {
		return fmt.Errorf("retire legacy video price rules: %w", err)
	}
	if _, err = tx.MigrationCheckpoint.Create().SetName(videoNativePricingMigrationName).SetPhase("done").SetCompleted(true).Save(ctx); err != nil {
		return fmt.Errorf("checkpoint video pricing migration: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit video pricing migration: %w", err)
	}
	return nil
}
