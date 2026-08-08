package db

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/configitem"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/modelaccountmodel"
)

const (
	modelAccountCapabilityMigrationCategory = "system_migration"
	modelAccountCapabilityMigrationKey      = "model_account_capability_defaults_v1"
	modelAccountSizeBoundsMigrationKey      = "model_account_size_bounds_v1"
)

var legacyModelAccountSupportedRatios = []string{"1:1", "16:9", "9:16", "4:3", "3:4"}

const prepareLegacyDataSQL = `
DO $$
BEGIN
    PERFORM pg_advisory_xact_lock(hashtext('pic-gallery:prepare-legacy-data'));

    IF to_regclass(current_schema() || '.image_tasks') IS NOT NULL THEN
        IF to_regclass(current_schema() || '.public_image_interactions') IS NOT NULL
           AND to_regclass(current_schema() || '.task_images') IS NOT NULL THEN
            EXECUTE 'DELETE FROM public_image_interactions
                WHERE image_id IN (
                    SELECT images.id FROM task_images AS images
                    JOIN image_tasks AS tasks ON tasks.id = images.task_id
                    WHERE tasks.task_type IN (''reference_generate'', ''reference_to_image'')
                )';
        END IF;

        IF to_regclass(current_schema() || '.public_image_stats') IS NOT NULL
           AND to_regclass(current_schema() || '.task_images') IS NOT NULL THEN
            EXECUTE 'DELETE FROM public_image_stats
                WHERE image_id IN (
                    SELECT images.id FROM task_images AS images
                    JOIN image_tasks AS tasks ON tasks.id = images.task_id
                    WHERE tasks.task_type IN (''reference_generate'', ''reference_to_image'')
                )';
        END IF;

        IF to_regclass(current_schema() || '.task_images') IS NOT NULL THEN
            EXECUTE 'DELETE FROM task_images
                WHERE task_id IN (
                    SELECT id FROM image_tasks
                    WHERE task_type IN (''reference_generate'', ''reference_to_image'')
                )';
        END IF;

        IF to_regclass(current_schema() || '.reference_assets') IS NOT NULL THEN
            EXECUTE 'DELETE FROM reference_assets
                WHERE bound_task_id IN (
                    SELECT id FROM image_tasks
                    WHERE task_type IN (''reference_generate'', ''reference_to_image'')
                )';
        END IF;

        IF to_regclass(current_schema() || '.point_ledgers') IS NOT NULL THEN
            EXECUTE 'DELETE FROM point_ledgers
                WHERE task_id IN (
                    SELECT id FROM image_tasks
                    WHERE task_type IN (''reference_generate'', ''reference_to_image'')
                )';
        END IF;

        IF to_regclass(current_schema() || '.wallet_reservation_allocations') IS NOT NULL THEN
            EXECUTE 'DELETE FROM wallet_reservation_allocations
                WHERE task_id IN (
                    SELECT id FROM image_tasks
                    WHERE task_type IN (''reference_generate'', ''reference_to_image'')
                )';
        END IF;

        IF to_regclass(current_schema() || '.api_key_quota_reservations') IS NOT NULL THEN
            EXECUTE 'DELETE FROM api_key_quota_reservations
                WHERE reservation_id IN (
                    SELECT id::text FROM image_tasks
                    WHERE task_type IN (''reference_generate'', ''reference_to_image'')
                )';
        END IF;

        EXECUTE 'DELETE FROM image_tasks
            WHERE task_type IN (''reference_generate'', ''reference_to_image'')';
    END IF;

    IF to_regclass(current_schema() || '.model_routes') IS NOT NULL THEN
        EXECUTE 'DELETE FROM model_routes
            WHERE task_type IN (''reference_generate'', ''reference_to_image'')';
    END IF;

    IF to_regclass(current_schema() || '.route_model_prices') IS NOT NULL THEN
        EXECUTE 'DELETE FROM route_model_prices
            WHERE task_type IN (''reference_generate'', ''reference_to_image'')';
    END IF;

    IF to_regclass(current_schema() || '.model_account_models') IS NOT NULL THEN
        EXECUTE 'UPDATE model_account_models
            SET task_types = COALESCE((
                SELECT jsonb_agg(value)
                FROM jsonb_array_elements_text(task_types::jsonb) AS value
                WHERE value NOT IN (''reference_generate'', ''reference_to_image'')
            ), ''[]''::jsonb)
            WHERE task_types::jsonb ?| ARRAY[''reference_generate'', ''reference_to_image'']';
    END IF;

    IF to_regclass(current_schema() || '.system_configs') IS NOT NULL THEN
        EXECUTE 'UPDATE system_configs
            SET config_value = jsonb_set(
                config_value::jsonb,
                ''{value}'',
                (config_value::jsonb->''value'') - ''reference_generate'' - ''reference_to_image''
            )
            WHERE jsonb_typeof(config_value::jsonb->''value'') = ''object''
              AND ((config_value::jsonb->''value'') ?| ARRAY[''reference_generate'', ''reference_to_image''])';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'object_storage_configs'
          AND column_name = 'is_default'
    ) THEN
        WITH ranked_defaults AS (
            SELECT id, row_number() OVER (ORDER BY updated_at DESC, id DESC) AS position
            FROM object_storage_configs
            WHERE is_default = true
        )
        UPDATE object_storage_configs AS configs
        SET is_default = false
        FROM ranked_defaults
        WHERE configs.id = ranked_defaults.id
          AND ranked_defaults.position > 1;
    END IF;

    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'route_model_prices'
          AND column_name = 'quality'
    ) THEN
        IF NOT EXISTS (
            SELECT 1
            FROM information_schema.columns
            WHERE table_schema = current_schema()
              AND table_name = 'route_model_prices'
              AND column_name = 'base_resolution'
        ) THEN
            ALTER TABLE route_model_prices RENAME COLUMN quality TO base_resolution;
        ELSE
            IF EXISTS (
                SELECT 1
                FROM route_model_prices
                WHERE NULLIF(btrim(base_resolution), '') IS NOT NULL
                  AND NULLIF(btrim(quality), '') IS NOT NULL
                  AND lower(btrim(base_resolution)) <> lower(btrim(quality))
            ) THEN
                RAISE EXCEPTION 'conflicting route model price dimensions: quality and base_resolution differ';
            END IF;

            UPDATE route_model_prices
            SET base_resolution = quality
            WHERE base_resolution IS NULL OR btrim(base_resolution) = '';

            DROP INDEX IF EXISTS routemodelprice_route_model_id_task_type_quality;
            ALTER TABLE route_model_prices DROP COLUMN quality;
        END IF;

        DROP INDEX IF EXISTS routemodelprice_route_model_id_task_type_quality;
    END IF;

    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'route_model_prices'
          AND column_name = 'base_resolution'
    ) AND NOT EXISTS (
        SELECT 1
        FROM pg_indexes
        WHERE schemaname = current_schema()
          AND tablename = 'route_model_prices'
          AND indexname = 'routemodelprice_route_model_id_task_type_base_resolution'
    ) THEN
        CREATE UNIQUE INDEX routemodelprice_route_model_id_task_type_base_resolution
            ON route_model_prices(route_model_id, task_type, base_resolution);
    END IF;
END $$;
`

func PrepareLegacyData(ctx context.Context, url string) error {
	if !isPostgresURL(url) {
		return nil
	}
	database, err := sql.Open("postgres", url)
	if err != nil {
		return fmt.Errorf("open legacy migration database: %w", err)
	}
	defer database.Close()
	if err := prepareLegacyDataWithExecutor(ctx, database); err != nil {
		return fmt.Errorf("prepare legacy database data: %w", err)
	}
	return nil
}

type legacyMigrationExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func prepareLegacyDataWithExecutor(ctx context.Context, executor legacyMigrationExecer) error {
	if executor == nil {
		return fmt.Errorf("legacy migration executor is required")
	}
	if _, err := executor.ExecContext(ctx, prepareLegacyDataSQL); err != nil {
		return err
	}
	return nil
}

func BackfillLegacyModelAccountCapabilities(ctx context.Context, client *repoent.Client) (int, error) {
	if client == nil {
		return 0, fmt.Errorf("model account capability migration client is required")
	}
	tx, err := client.Tx(ctx)
	if err != nil {
		return 0, fmt.Errorf("start model account capability migration: %w", err)
	}
	rollback := func(cause error) (int, error) {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return 0, fmt.Errorf("%w (rollback: %v)", cause, rollbackErr)
		}
		return 0, cause
	}

	applied, err := tx.ConfigItem.Query().Where(
		configitem.ConfigCategoryEQ(modelAccountCapabilityMigrationCategory),
		configitem.ConfigKeyEQ(modelAccountCapabilityMigrationKey),
		configitem.ScopeEQ("global"),
	).Exist(ctx)
	if err != nil {
		return rollback(fmt.Errorf("check model account capability migration marker: %w", err))
	}
	if applied {
		_ = tx.Rollback()
		return 0, nil
	}
	if _, err := tx.ConfigItem.Create().
		SetConfigCategory(modelAccountCapabilityMigrationCategory).
		SetConfigKey(modelAccountCapabilityMigrationKey).
		SetConfigValue(map[string]any{"applied": true}).
		SetScope("global").
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx); err != nil {
		_, rollbackErr := rollback(fmt.Errorf("create model account capability migration marker: %w", err))
		if repoent.IsConstraintError(err) {
			markerExists, queryErr := client.ConfigItem.Query().Where(
				configitem.ConfigCategoryEQ(modelAccountCapabilityMigrationCategory),
				configitem.ConfigKeyEQ(modelAccountCapabilityMigrationKey),
				configitem.ScopeEQ("global"),
			).Exist(ctx)
			if queryErr == nil && markerExists {
				return 0, nil
			}
		}
		return 0, rollbackErr
	}

	models, err := tx.ModelAccountModel.Query().Where(
		modelaccountmodel.DeletedAtIsNil(),
		modelaccountmodel.MaxImageCountEQ(1),
		modelaccountmodel.MaxReferenceImageCountEQ(4),
	).All(ctx)
	if err != nil {
		return rollback(fmt.Errorf("query legacy model account capabilities: %w", err))
	}
	updated := 0
	for _, model := range models {
		if len(model.SupportedRatios) > 0 && !slices.Equal(model.SupportedRatios, legacyModelAccountSupportedRatios) {
			continue
		}
		if _, err := tx.ModelAccountModel.UpdateOneID(model.ID).
			SetSupportedRatios([]string{"1:1"}).
			SetMaxReferenceImageCount(0).
			Save(ctx); err != nil {
			return rollback(fmt.Errorf("backfill model account %d capabilities: %w", model.ID, err))
		}
		updated++
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit model account capability migration: %w", err)
	}
	return updated, nil
}

func BackfillLegacyModelAccountSizeBounds(ctx context.Context, client *repoent.Client) (int, error) {
	if client == nil {
		return 0, fmt.Errorf("model account size bounds migration client is required")
	}
	tx, err := client.Tx(ctx)
	if err != nil {
		return 0, fmt.Errorf("start model account size bounds migration: %w", err)
	}
	rollback := func(cause error) (int, error) {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return 0, fmt.Errorf("%w (rollback: %v)", cause, rollbackErr)
		}
		return 0, cause
	}

	applied, err := tx.ConfigItem.Query().Where(
		configitem.ConfigCategoryEQ(modelAccountCapabilityMigrationCategory),
		configitem.ConfigKeyEQ(modelAccountSizeBoundsMigrationKey),
		configitem.ScopeEQ("global"),
	).Exist(ctx)
	if err != nil {
		return rollback(fmt.Errorf("check model account size bounds migration marker: %w", err))
	}
	if applied {
		_ = tx.Rollback()
		return 0, nil
	}
	if _, err := tx.ConfigItem.Create().
		SetConfigCategory(modelAccountCapabilityMigrationCategory).
		SetConfigKey(modelAccountSizeBoundsMigrationKey).
		SetConfigValue(map[string]any{"applied": true}).
		SetScope("global").
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx); err != nil {
		_, rollbackErr := rollback(fmt.Errorf("create model account size bounds migration marker: %w", err))
		if repoent.IsConstraintError(err) {
			markerExists, queryErr := client.ConfigItem.Query().Where(
				configitem.ConfigCategoryEQ(modelAccountCapabilityMigrationCategory),
				configitem.ConfigKeyEQ(modelAccountSizeBoundsMigrationKey),
				configitem.ScopeEQ("global"),
			).Exist(ctx)
			if queryErr == nil && markerExists {
				return 0, nil
			}
		}
		return 0, rollbackErr
	}

	models, err := tx.ModelAccountModel.Query().Where(
		modelaccountmodel.DeletedAtIsNil(),
		modelaccountmodel.Or(
			modelaccountmodel.MaxWidthEQ(4096),
			modelaccountmodel.MaxHeightEQ(4096),
		),
	).All(ctx)
	if err != nil {
		return rollback(fmt.Errorf("query legacy model account size bounds: %w", err))
	}
	for _, model := range models {
		update := tx.ModelAccountModel.UpdateOneID(model.ID)
		if model.MaxWidth == 4096 {
			update.SetMaxWidth(3840)
		}
		if model.MaxHeight == 4096 {
			update.SetMaxHeight(3840)
		}
		if _, err := update.Save(ctx); err != nil {
			return rollback(fmt.Errorf("backfill model account %d size bounds: %w", model.ID, err))
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit model account size bounds migration: %w", err)
	}
	return len(models), nil
}

func isPostgresURL(url string) bool {
	url = strings.ToLower(strings.TrimSpace(url))
	return strings.HasPrefix(url, "postgres://") || strings.HasPrefix(url, "postgresql://")
}
