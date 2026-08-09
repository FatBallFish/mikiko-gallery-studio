package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	domainstorageconfig "github.com/fatballfish/pic-gallery/internal/domain/storageconfig"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/google/uuid"
)

const (
	startupStorageIdentityBatchSize  = 100
	startupStorageIdentityMaxBatches = 1000
)

var ErrLegacyStorageIdentityBackfillIncomplete = errors.New("legacy storage identity backfill is incomplete")

type legacyStorageIdentityResolver interface {
	ResolveLegacyByDriver(context.Context, string) (domainstorageconfig.ResolvedConfig, error)
}

type legacyStorageIdentityStartupResolver interface {
	legacyStorageIdentityResolver
	ListLegacyDrivers(context.Context) ([]string, error)
}

func backfillLegacyStorageIdentityAtStartup(
	ctx context.Context,
	client *repoent.Client,
	resolver legacyStorageIdentityResolver,
	driver string,
	options db.LegacyStorageIdentityBackfillOptions,
) (db.LegacyStorageIdentityBackfillProgress, error) {
	if resolver == nil {
		return db.LegacyStorageIdentityBackfillProgress{}, fmt.Errorf("resolve legacy storage identity: resolver is required")
	}
	resolved, err := resolver.ResolveLegacyByDriver(ctx, driver)
	if err != nil {
		return db.LegacyStorageIdentityBackfillProgress{}, fmt.Errorf("resolve legacy %s storage config: %w", strings.TrimSpace(driver), err)
	}
	configID, err := uuid.Parse(strings.TrimSpace(resolved.ID))
	if err != nil {
		return db.LegacyStorageIdentityBackfillProgress{}, fmt.Errorf("resolve legacy storage config ID: %w", err)
	}
	return backfillResolvedLegacyStorageIdentityAtStartup(ctx, client, driver, configID, options)
}

func backfillLegacyStorageIdentitiesAtStartup(
	ctx context.Context,
	client *repoent.Client,
	resolver legacyStorageIdentityStartupResolver,
	currentDriver string,
	options db.LegacyStorageIdentityBackfillOptions,
) (map[string]db.LegacyStorageIdentityBackfillProgress, error) {
	if resolver == nil {
		return nil, fmt.Errorf("resolve legacy storage identity: resolver is required")
	}
	drivers, err := db.ListLegacyStorageDrivers(ctx, client)
	if err != nil {
		return nil, err
	}
	persistedDrivers, err := resolver.ListLegacyDrivers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list legacy storage config drivers: %w", err)
	}
	drivers = normalizedStorageDrivers(append(append(drivers, persistedDrivers...), currentDriver))
	configIDs := make(map[string]uuid.UUID, len(drivers))
	for _, driver := range drivers {
		resolved, err := resolver.ResolveLegacyByDriver(ctx, driver)
		if err != nil {
			return nil, fmt.Errorf("resolve legacy %s storage config: %w", driver, err)
		}
		configID, err := uuid.Parse(strings.TrimSpace(resolved.ID))
		if err != nil {
			return nil, fmt.Errorf("resolve legacy %s storage config ID: %w", driver, err)
		}
		configIDs[driver] = configID
	}
	if err := db.PrepareLegacyStorageCleanupCutovers(ctx, client, drivers); err != nil {
		return nil, err
	}
	progressByDriver := make(map[string]db.LegacyStorageIdentityBackfillProgress, len(drivers))
	for _, driver := range drivers {
		progress, err := backfillResolvedLegacyStorageIdentityAtStartup(ctx, client, driver, configIDs[driver], options)
		progressByDriver[driver] = progress
		if err != nil {
			return progressByDriver, err
		}
	}
	return progressByDriver, nil
}

func normalizedStorageDrivers(values []string) []string {
	driverSet := make(map[string]struct{}, len(values))
	for _, value := range values {
		driver := strings.ToLower(strings.TrimSpace(value))
		if driver == "" {
			driver = domainstorageconfig.DriverLocal
		}
		driverSet[driver] = struct{}{}
	}
	drivers := make([]string, 0, len(driverSet))
	for driver := range driverSet {
		drivers = append(drivers, driver)
	}
	sort.Strings(drivers)
	return drivers
}

func backfillResolvedLegacyStorageIdentityAtStartup(
	ctx context.Context,
	client *repoent.Client,
	driver string,
	configID uuid.UUID,
	options db.LegacyStorageIdentityBackfillOptions,
) (db.LegacyStorageIdentityBackfillProgress, error) {
	progress, err := db.RunLegacyStorageIdentityBackfill(ctx, client, driver, configID, options)
	if err != nil {
		return progress, fmt.Errorf("backfill legacy storage identity: %w", err)
	}
	if !progress.Completed {
		return progress, fmt.Errorf("%w: phase=%s processed_rows=%d", ErrLegacyStorageIdentityBackfillIncomplete, progress.Phase, progress.ProcessedRows)
	}
	slog.InfoContext(ctx, "legacy storage identity backfill ready",
		"storage_config_id", configID.String(),
		"storage_driver", strings.ToLower(strings.TrimSpace(driver)),
		"processed_rows", progress.ProcessedRows,
	)
	return progress, nil
}

func requireLegacyStorageIdentityBackfill(ctx context.Context, client *repoent.Client, resolver legacyStorageIdentityStartupResolver, currentDriver string) error {
	_, err := backfillLegacyStorageIdentitiesAtStartup(ctx, client, resolver, currentDriver, db.LegacyStorageIdentityBackfillOptions{
		BatchSize: startupStorageIdentityBatchSize, MaxBatches: startupStorageIdentityMaxBatches,
	})
	return err
}
