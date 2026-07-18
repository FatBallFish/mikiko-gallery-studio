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
)

var legacyModelAccountSupportedRatios = []string{"1:1", "16:9", "9:16", "4:3", "3:4"}

const prepareLegacyDataSQL = `
DO $$
BEGIN
    PERFORM pg_advisory_xact_lock(hashtext('pic-gallery:prepare-legacy-data'));

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
	if _, err := database.ExecContext(ctx, prepareLegacyDataSQL); err != nil {
		return fmt.Errorf("prepare legacy database data: %w", err)
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

func isPostgresURL(url string) bool {
	url = strings.ToLower(strings.TrimSpace(url))
	return strings.HasPrefix(url, "postgres://") || strings.HasPrefix(url, "postgresql://")
}
