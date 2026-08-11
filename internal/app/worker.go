package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/config"
	domaincluster "github.com/fatballfish/pic-gallery/internal/domain/cluster"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
	assetservice "github.com/fatballfish/pic-gallery/internal/service/assets"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	galleryexportservice "github.com/fatballfish/pic-gallery/internal/service/galleryexport"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	objectcleanupservice "github.com/fatballfish/pic-gallery/internal/service/objectcleanup"
	storageconfigservice "github.com/fatballfish/pic-gallery/internal/service/storageconfig"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/internal/worker"
)

func RunWorker() error {
	return RunWorkerContext(context.Background())
}

func RunWorkerContext(ctx context.Context) error {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	return runWorker(ctx, workerRunDependencies{})
}

func runNormalWorker(ctx context.Context, startup workerBootstrap) error {
	return runNormalWorkerWithOptions(ctx, startup, workerNormalStartupOptions{})
}

type workerNormalStartupOptions struct {
	dependencyTimeout        time.Duration
	openDatabase             func(context.Context, string) (*repoent.Client, error)
	checkSchemaCompatibility func(context.Context, *repoent.Client, config.Config) error
	verifyCompletedBinding   func(context.Context, workerBootstrap) error
}

func runNormalWorkerWithOptions(ctx context.Context, startup workerBootstrap, options workerNormalStartupOptions) error {
	cfg, err := config.RuntimeFromBootstrap(startup.Bootstrap)
	if err != nil {
		return err
	}
	if !runtimeMatchesBootstrapSnapshot(cfg, startup.Bootstrap) {
		return fmt.Errorf("worker runtime changed after bootstrap mode selection")
	}
	if err := validateStorageTopology(cfg); err != nil {
		return err
	}

	if options.dependencyTimeout <= 0 {
		options.dependencyTimeout = 15 * time.Second
	}
	if options.openDatabase == nil {
		options.openDatabase = db.OpenContext
	}
	if options.checkSchemaCompatibility == nil {
		options.checkSchemaCompatibility = checkRuntimeSchemaCompatibility
	}
	if options.verifyCompletedBinding == nil {
		options.verifyCompletedBinding = func(ctx context.Context, startup workerBootstrap) error {
			return verifyCompletedStartupBinding(ctx, apiStartup{Bootstrap: startup.Bootstrap, State: startup.State})
		}
	}

	startupContext, cancelStartup := context.WithTimeout(ctx, options.dependencyTimeout)
	defer cancelStartup()
	client, err := options.openDatabase(startupContext, cfg.Database.URL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer client.Close()
	if err := options.checkSchemaCompatibility(startupContext, client, cfg); err != nil {
		return err
	}
	if shouldVerifyOriginalSetupBinding(cfg.Runtime.DeploymentRole) {
		if err := options.verifyCompletedBinding(startupContext, startup); err != nil {
			return fmt.Errorf("verify completed setup binding: %w", err)
		}
	}
	redisClient, err := newRedisClient(startupContext, cfg)
	if err != nil {
		return err
	}
	if redisClient != nil {
		defer redisClient.Close()
	}
	clusterStore := entstore.NewClusterStore(client)
	heartbeat, err := startRuntimeHeartbeat(ctx, cfg, clusterStore, domaincluster.NodeRoleWorker)
	if err != nil {
		return err
	}
	defer heartbeat.Stop()

	storageConfigSvc := storageconfigservice.NewServiceWithOptions(
		entstore.NewStorageConfigStore(client), cfg.Security.SecureConfigEncryptionKey, cfg.Storage, cfg.App.Env,
		storageconfigservice.ServiceOptions{BootstrapStorageManaged: startup.Bootstrap.ObjectStorageManaged},
	)
	if err := storageConfigSvc.Bootstrap(startupContext, 0); err != nil {
		return fmt.Errorf("bootstrap storage config: %w", err)
	}
	if err := requireLegacyStorageIdentityBackfill(startupContext, client, storageConfigSvc, cfg.Storage.Driver); err != nil {
		return fmt.Errorf("prepare object cleanup storage identities: %w", err)
	}
	cancelStartup()
	storageRegistry := storage.NewRegistry(storageConfigSvc, 30*time.Second)
	if redisClient != nil {
		storageInvalidationBus := storage.NewRedisInvalidationBus(redisClient, cfg.Redis.KeyPrefix)
		subscriber := startStorageInvalidationSubscriber(ctx, storageInvalidationBus, storageRegistry.Invalidate, time.Second, 30*time.Second)
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

	owner := workerOwner()
	runner := worker.NewRunner(taskSvc, worker.Config{
		Owner:                 owner,
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
	runner.SetPaymentExpiryService(billingSvc)
	runner.SetCleanupService(objectcleanupservice.NewProcessor(entstore.NewObjectCleanupStore(client), storageRegistry, objectcleanupservice.ProcessorOptions{}))
	runner.SetGalleryExportService(galleryexportservice.NewProcessor(entstore.NewGalleryExportStore(client), storageRegistry, galleryexportservice.ProcessorOptions{Owner: owner}))

	slog.Info("starting pic-gallery worker")
	err = runner.Run(ctx)
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
