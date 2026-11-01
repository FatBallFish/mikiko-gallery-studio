package app

import (
	"context"
	"fmt"

	"github.com/fatballfish/pic-gallery/internal/repository/db"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
)

// Complete ownership transfer before serving requests or starting cleanup.
func requireReferenceMediaBackfill(ctx context.Context, client *repoent.Client) error {
	if _, err := db.BackfillReferenceMediaAssets(ctx, client); err != nil {
		return fmt.Errorf("prepare unified reference media assets: %w", err)
	}
	return nil
}
