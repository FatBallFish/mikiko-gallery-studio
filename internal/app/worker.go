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
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
	assetservice "github.com/fatballfish/pic-gallery/internal/service/assets"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	storageconfigservice "github.com/fatballfish/pic-gallery/internal/service/storageconfig"
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
	if err := checkRuntimeSchemaCompatibility(context.Background(), client, cfg); err != nil {
		return err
	}
	redisClient, err := newRedisClient(context.Background(), cfg)
	if err != nil {
		return err
	}
	if redisClient != nil {
		defer redisClient.Close()
	}

	storageConfigSvc := storageconfigservice.NewService(entstore.NewStorageConfigStore(client), cfg.Security.SecureConfigEncryptionKey, cfg.Storage, cfg.App.Env)
	if err := storageConfigSvc.Bootstrap(context.Background(), 0); err != nil {
		return fmt.Errorf("bootstrap storage config: %w", err)
	}
	storageRegistry := storage.NewRegistry(storageConfigSvc, 30*time.Second)
	if redisClient != nil {
		storageInvalidationBus := storage.NewRedisInvalidationBus(redisClient, cfg.Redis.KeyPrefix)
		subscriber := startStorageInvalidationSubscriber(context.Background(), storageInvalidationBus, storageRegistry.Invalidate, time.Second, 30*time.Second)
		defer subscriber.Stop()
	}

	assetStore := entstore.NewAssetsStore(client)
	adminCfgSvc := adminconfigservice.NewServiceWithStore(cfg, entstore.NewAdminConfigStore(client))
	billingSvc := billingservice.NewServiceWithStore(cfg.Billing, entstore.NewBillingStore(client, cfg.Billing.PointsScale))
	billingSvc.SetAdminConfigResolver(adminCfgSvc)
	taskSvc := imagetaskservice.NewServiceWithProvidersStoreAssetsBillingAndRouter(
		cfg,
		nil,
		entstore.NewImageTaskStore(client),
		assetservice.NewServiceWithStoreAndRouter(cfg.GenerationLimits, assetStore, storageRegistry),
		billingSvc,
		storageRegistry,
	)
	taskSvc.SetModelRoutingSource(entstore.NewModelAdminStore(client))
	if redisClient != nil {
		taskSvc.SetConcurrencyGate(imagetaskservice.NewRedisConcurrencyGate(redisClient, cfg.Redis.KeyPrefix))
	}
	slog.Info("database-backed task store enabled for worker")

	runner := worker.NewRunner(taskSvc, worker.Config{
		Owner:                 workerOwner(),
		LeaseTTL:              30 * time.Second,
		HeartbeatInterval:     10 * time.Second,
		PollInterval:          500 * time.Millisecond,
		MaxConcurrentTasks:    cfg.Worker.MaxConcurrentTasks,
		ConfigRefreshInterval: 5 * time.Second,
		MaxConcurrentTasksResolver: func(ctx context.Context) (int, error) {
			return workerMaxConcurrentTasksFromAdminConfig(ctx, adminCfgSvc, cfg.Worker.MaxConcurrentTasks)
		},
	})
	runner.SetCompensationService(billingSvc)

	slog.Info("starting pic-gallery worker")
	err = runner.Run(context.Background())
	if err == context.Canceled {
		return nil
	}
	return err
}

func workerMaxConcurrentTasksFromAdminConfig(ctx context.Context, adminCfgSvc *adminconfigservice.Service, fallback int) (int, error) {
	if adminCfgSvc == nil {
		return fallback, nil
	}
	tab, err := adminCfgSvc.GetTab(ctx, "runtime")
	if err != nil {
		return fallback, nil
	}
	for _, item := range tab.Items {
		if item.ConfigKey != "worker_max_concurrent_tasks" {
			continue
		}
		switch value := item.ConfigValue["value"].(type) {
		case int:
			return value, nil
		case int64:
			return int(value), nil
		case float64:
			return int(value), nil
		}
	}
	return fallback, nil
}

func workerOwner() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "worker"
	}
	return hostname + "-" + uuid.NewString()
}
