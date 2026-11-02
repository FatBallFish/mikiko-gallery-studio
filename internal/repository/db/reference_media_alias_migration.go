package db

import (
	"context"
	"fmt"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
)

// RetireReferenceMediaAliasUniqueness removes only the obsolete partial unique
// index. Ent Schema.Create does not drop removed indexes by default; enabling
// WithDropIndex globally would also remove unrelated compatibility indexes.
// PostgreSQL and SQLite both implement transactional DROP INDEX IF EXISTS.
// The ordinary media_asset_id lookup index remains in the Ent schema.
func RetireReferenceMediaAliasUniqueness(ctx context.Context, client *repoent.Client) error {
	if client == nil {
		return fmt.Errorf("reference alias index migration client is required")
	}
	switch client.DialectName() {
	case "postgres", "sqlite3":
	default:
		return fmt.Errorf("unsupported reference alias index migration dialect %q", client.DialectName())
	}
	_, err := runLegacyStorageIdentityBatchWithRetry(ctx, func() (int, error) {
		return 0, client.ExecRaw(ctx, `DROP INDEX IF EXISTS reference_asset_active_media_asset`)
	})
	if err != nil {
		return fmt.Errorf("retire reference media alias unique index: %w", err)
	}
	return nil
}
