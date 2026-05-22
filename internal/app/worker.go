package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	assetservice "github.com/fatballfish/pic-gallery/internal/service/assets"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/internal/worker"
)

func RunWorker() error {
	cfg, err := config.Load("")
	if err != nil {
		return err
	}
	if err := validateStorageTopology(cfg); err != nil {
		return err
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	client, err := db.Open(cfg.Database.URL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer client.Close()
	if err := client.Schema.Create(context.Background()); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	storageBackend, err := storage.NewBackend(cfg.Storage)
	if err != nil {
		return fmt.Errorf("init storage backend: %w", err)
	}

	redisClient, _, err := newRedisClient(context.Background(), cfg)
	if err != nil {
		return err
	}
	if redisClient != nil {
		defer redisClient.Close()
	}

	assetSvc := entstore.NewAssetsStore(client)
	billingSvc := billingservice.NewServiceWithStore(cfg.Billing, entstore.NewBillingStore(client, cfg.Billing.PointsScale))
	taskSvc := imagetaskservice.NewServiceWithProvidersStoreAssetsBillingAndBackend(
		cfg,
		nil,
		entstore.NewImageTaskStore(client),
		assetservice.NewServiceWithStoreAndBackend(cfg.Storage, cfg.GenerationLimits, assetSvc, storageBackend),
		billingSvc,
		storageBackend,
	)
	taskSvc.SetModelRoutingSource(entstore.NewModelAdminStore(client))
	slog.Info("database-backed task store enabled for worker")

	runner := worker.NewRunner(taskSvc, worker.Config{
		Owner:             workerOwner(),
		LeaseTTL:          30 * time.Second,
		HeartbeatInterval: 10 * time.Second,
		PollInterval:      500 * time.Millisecond,
	})

	slog.Info("starting pic-gallery worker")
	err = runner.Run(context.Background())
	if err == context.Canceled {
		return nil
	}
	return err
}

func workerOwner() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "worker"
	}
	return hostname + "-" + uuid.NewString()
}
