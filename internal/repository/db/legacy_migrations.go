package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

const backfillRouteModelPriceQualitySQL = `
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.tables
        WHERE table_schema = current_schema()
          AND table_name = 'route_model_prices'
    ) AND NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'route_model_prices'
          AND column_name = 'quality'
    ) THEN
        ALTER TABLE route_model_prices ADD COLUMN quality varchar(32);
    END IF;

    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'route_model_prices'
          AND column_name = 'quality'
    ) THEN
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
              AND indexname = 'routemodelprice_route_model_id_task_type_quality'
        ) THEN
            UPDATE route_model_prices
            SET quality = COALESCE(NULLIF(btrim(base_resolution), ''), 'auto');
        ELSE
            UPDATE route_model_prices
            SET quality = 'auto'
            WHERE quality IS NULL OR btrim(quality) = '';
        END IF;
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
	if _, err := database.ExecContext(ctx, backfillRouteModelPriceQualitySQL); err != nil {
		return fmt.Errorf("backfill route model price quality: %w", err)
	}
	return nil
}

func isPostgresURL(url string) bool {
	url = strings.ToLower(strings.TrimSpace(url))
	return strings.HasPrefix(url, "postgres://") || strings.HasPrefix(url, "postgresql://")
}
