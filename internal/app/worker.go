package app

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/app/observability"
	"github.com/fatballfish/pic-gallery/internal/config"
	domaincluster "github.com/fatballfish/pic-gallery/internal/domain/cluster"
	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
	assetservice "github.com/fatballfish/pic-gallery/internal/service/assets"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	galleryexportservice "github.com/fatballfish/pic-gallery/internal/service/galleryexport"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	mediaassetservice "github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	mediaprocessservice "github.com/fatballfish/pic-gallery/internal/service/mediaprocess"
	objectcleanupservice "github.com/fatballfish/pic-gallery/internal/service/objectcleanup"
	storageconfigservice "github.com/fatballfish/pic-gallery/internal/service/storageconfig"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/internal/worker"
	mediaworker "github.com/fatballfish/pic-gallery/internal/worker/media"
	videoworker "github.com/fatballfish/pic-gallery/internal/worker/video"
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
	metricsListen            func(string, string) (net.Listener, error)
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
	if err := validateWorkerRuntime(cfg.Worker, workerRuntimeChecks{}); err != nil {
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
	metricsContext, stopMetrics := context.WithCancel(ctx)
	defer stopMetrics()
	metrics := observability.DefaultMetrics()
	metrics.Runtime().Start(metricsContext)
	metricsDone, metricsAddress, err := startConfiguredWorkerMetricsServer(metricsContext, cfg.Worker.MetricsAddr, metrics.Handler(), options.metricsListen)
	if err != nil {
		return fmt.Errorf("listen for Worker metrics: %w", err)
	}
	slog.Info("worker metrics listener started", "address", metricsAddress)

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
	imageRunner := worker.NewRunner(taskSvc, worker.Config{
		Owner:                 owner,
		LeaseTTL:              30 * time.Second,
		HeartbeatInterval:     10 * time.Second,
		PollInterval:          500 * time.Millisecond,
		MaxConcurrentTasks:    cfg.Worker.ImageConcurrency,
		ConfigRefreshInterval: 5 * time.Second,
		MaxConcurrentTasksResolver: func(ctx context.Context) (int, error) {
			return workerMaxConcurrentTasksFromAdminConfig(ctx, adminCfgSvc, cfg.Worker.ImageConcurrency)
		},
	})
	imageRunner.SetCompensationService(billingSvc)
	imageRunner.SetPaymentExpiryService(billingSvc)

	cleanupProcessor := objectcleanupservice.NewProcessor(entstore.NewObjectCleanupStore(client), storageRegistry, objectcleanupservice.ProcessorOptions{})
	exportProcessor := galleryexportservice.NewProcessor(entstore.NewGalleryExportStore(client), storageRegistry, galleryexportservice.ProcessorOptions{Owner: owner + "-cleanup"})
	uploadExpiryProcessor := mediaassetservice.NewUploadExpiryProcessor(entstore.NewMediaStore(client), storageRegistry, mediaassetservice.UploadExpiryProcessorOptions{})
	mediaWorkerStore := entstore.NewMediaWorkerStore(client)
	adminVideoStore := entstore.NewAdminVideoStore(client)
	mediaReconcileProcessor := mediaassetservice.NewMediaReconcileProcessor(mediaWorkerStore)
	cleanupRole := &cleanupRoleProcessor{cleanup: cleanupProcessor, processors: []processOnce{cleanupProcessor, exportProcessor, uploadExpiryProcessor, mediaReconcileProcessor}}

	videoStore := entstore.NewVideoTaskStore(client, entstore.NewBillingStore(client, cfg.Billing.PointsScale))
	videoArtifactHTTPClient, err := newVideoArtifactHTTPClient(cfg.Worker.VideoArtifactTestCAFile)
	if err != nil {
		return fmt.Errorf("configure video artifact HTTP client: %w", err)
	}
	videoRunner := videoworker.NewRunner(videoStore, videoworker.NewExecutionAccountResolver(videoStore), storageRegistry, videoworker.Options{
		Owner: owner + "-video", LeaseTTL: 30 * time.Second,
		AllowLoopbackArtifactHosts: cfg.Worker.AllowLoopbackVideoArtifacts,
		HTTPClient:                 videoArtifactHTTPClient,
		ClaimAllowed:               newRedisVideoClaimGate(redisClient).Allowed,
		Observer:                   workerMetricsObserver{metrics: metrics},
	})
	mediaCommandRunner := mappedMediaCommandRunner{ffmpeg: cfg.Worker.FFmpegPath, ffprobe: cfg.Worker.FFprobePath}
	mediaPipeline := mediaworker.NewPipeline(
		storageRegistry,
		mediaprocessservice.NewProbe(mediaCommandRunner, 30*time.Second),
		mediaDerivativeAdapter{processor: mediaprocessservice.NewDerivativeProcessor(mediaCommandRunner, 2*time.Minute)},
		mediaworker.PipelineOptions{TempDir: cfg.Worker.TempDir, PolicyResolver: func(ctx context.Context) (domainmedia.Policy, error) {
			policy, err := adminVideoStore.GetMediaPolicy(ctx)
			if err != nil {
				return domainmedia.Policy{}, err
			}
			return policy.RuntimePolicy().Policy, nil
		}},
	)
	mediaRunner := mediaworker.NewRunner(mediaWorkerStore, mediaPipeline, mediaworker.Options{
		Owner: owner + "-media", LeaseTTL: 2 * time.Minute,
		ClaimAllowed: newDiskClaimGate(cfg.Worker.TempDir, cfg.Worker.TempDiskPausePercent, metrics).Allowed,
		Observer:     workerMetricsObserver{metrics: metrics},
		Reporter:     workerMediaFailureReporter{},
	})

	loops := make([]func(context.Context) error, 0, 5)
	loops = append(loops, func(loopCtx context.Context) error { return waitForWorkerMetricsLoop(loopCtx, metricsDone) })
	roles := make([]string, 0, len(cfg.Worker.Roles))
	if cfg.Worker.HasRole(config.WorkerRoleImage) {
		roles = append(roles, string(config.WorkerRoleImage))
		loops = append(loops, imageRunner.Run)
	}
	if cfg.Worker.HasRole(config.WorkerRoleVideo) {
		roles = append(roles, string(config.WorkerRoleVideo))
		loops = append(loops, func(loopCtx context.Context) error {
			return runWorkerRoleSlots(loopCtx, "video", cfg.Worker.VideoConcurrency, 500*time.Millisecond, videoRunner.RunOnce)
		})
	}
	if cfg.Worker.HasRole(config.WorkerRoleMedia) {
		roles = append(roles, string(config.WorkerRoleMedia))
		loops = append(loops, func(loopCtx context.Context) error {
			return runWorkerRoleSlots(loopCtx, "media", cfg.Worker.MediaConcurrency, 500*time.Millisecond, mediaRunner.RunOnce)
		})
	}
	if cfg.Worker.HasRole(config.WorkerRoleCleanup) {
		roles = append(roles, string(config.WorkerRoleCleanup))
		loops = append(loops, func(loopCtx context.Context) error {
			return runWorkerRoleSlots(loopCtx, "cleanup", cfg.Worker.CleanupConcurrency, time.Second, cleanupRole.ProcessOnce)
		})
	}
	slog.Info("starting pic-gallery worker")
	slog.Info("worker roles enabled", "roles", strings.Join(roles, ","), "image_concurrency", cfg.Worker.ImageConcurrency, "video_concurrency", cfg.Worker.VideoConcurrency, "media_concurrency", cfg.Worker.MediaConcurrency, "cleanup_concurrency", cfg.Worker.CleanupConcurrency)
	err = runIndependentWorkerLoops(ctx, loops...)
	if err == context.Canceled {
		return nil
	}
	return err
}

type workerMetricsObserver struct{ metrics *observability.Metrics }

type workerMediaFailureReporter struct{}

func (workerMediaFailureReporter) ReportMediaProcessingFailure(ctx context.Context, item mediaworker.WorkItem, terminal bool, err error) {
	slog.WarnContext(ctx, "media processing attempt failed",
		"error_code", "media_processing_failed",
		"job_id", item.JobID,
		"asset_id", item.AssetID,
		"attempt", item.AttemptCount,
		"max_attempts", item.MaxAttempts,
		"terminal", terminal,
		"error", err,
	)
}

func (observer workerMetricsObserver) RecordVideoStage(stage, result string) {
	observer.metrics.RecordVideoStage(stage, result)
}

func (observer workerMetricsObserver) RecordArtifactTransfer(mediaType, result string, bytes int64) {
	observer.metrics.RecordArtifactTransfer(mediaType, result, bytes)
	if result == "success" {
		observer.metrics.AddObjectBytes("written", bytes)
	}
}

func (observer workerMetricsObserver) RecordSettlement(kind, result string) {
	observer.metrics.RecordSettlement(kind, result)
}

func (observer workerMetricsObserver) RecordDerivative(kind, result string, bytes int64) {
	observer.metrics.RecordDerivative(kind, result, bytes)
	if result == "success" {
		observer.metrics.AddObjectBytes("written", bytes)
	}
}

type processOnce interface {
	ProcessOnce(context.Context) (bool, error)
}

type reconcileOnce interface {
	Reconcile(context.Context, int) (int, error)
}

type cleanupRoleProcessor struct {
	cleanup interface {
		processOnce
		reconcileOnce
	}
	processors    []processOnce
	mu            sync.Mutex
	nextProcessor int
	lastReconcile time.Time
}

func (processor *cleanupRoleProcessor) ProcessOnce(ctx context.Context) (bool, error) {
	processor.mu.Lock()
	start := processor.nextProcessor
	due := processor.lastReconcile.IsZero() || time.Since(processor.lastReconcile) >= 6*time.Hour
	if due {
		processor.lastReconcile = time.Now()
	}
	processor.mu.Unlock()
	if due {
		if _, err := processor.cleanup.Reconcile(ctx, 100); err != nil {
			return false, err
		}
	}
	if len(processor.processors) == 0 {
		return false, nil
	}
	for offset := range len(processor.processors) {
		index := (start + offset) % len(processor.processors)
		processed, err := processor.processors[index].ProcessOnce(ctx)
		if err != nil || processed {
			processor.mu.Lock()
			processor.nextProcessor = (index + 1) % len(processor.processors)
			processor.mu.Unlock()
			return processed, err
		}
	}
	processor.mu.Lock()
	processor.nextProcessor = (start + 1) % len(processor.processors)
	processor.mu.Unlock()
	return false, nil
}

type mappedMediaCommandRunner struct{ ffmpeg, ffprobe string }

func (runner mappedMediaCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	switch name {
	case "ffmpeg":
		name = runner.ffmpeg
	case "ffprobe":
		name = runner.ffprobe
	}
	return (mediaprocessservice.ExecRunner{}).Run(ctx, name, args...)
}

type mediaDerivativeAdapter struct {
	processor *mediaprocessservice.DerivativeProcessor
}

func (adapter mediaDerivativeAdapter) Generate(ctx context.Context, mediaType domainmedia.MediaType, input, outputDir string) ([]mediaworker.DerivativeOutput, error) {
	outputs, err := adapter.processor.Generate(ctx, mediaType, input, outputDir)
	return mapMediaDerivativeOutputs(outputs, err)
}

func (adapter mediaDerivativeAdapter) GenerateWithPolicy(ctx context.Context, mediaType domainmedia.MediaType, input, outputDir string, policy domainmedia.Policy) ([]mediaworker.DerivativeOutput, error) {
	outputs, err := adapter.processor.GenerateWithPolicy(ctx, mediaType, input, outputDir, policy)
	return mapMediaDerivativeOutputs(outputs, err)
}

func mapMediaDerivativeOutputs(outputs []mediaprocessservice.DerivativeOutput, err error) ([]mediaworker.DerivativeOutput, error) {
	if err != nil {
		return nil, err
	}
	result := make([]mediaworker.DerivativeOutput, 0, len(outputs))
	for _, output := range outputs {
		result = append(result, mediaworker.DerivativeOutput{Kind: output.Kind, TransformVersion: output.TransformVersion, Path: output.Path})
	}
	return result, nil
}

func runIndependentWorkerLoops(ctx context.Context, loops ...func(context.Context) error) error {
	loopCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, len(loops))
	var wg sync.WaitGroup
	for _, loop := range loops {
		if loop == nil {
			continue
		}
		wg.Add(1)
		go func(run func(context.Context) error) {
			defer wg.Done()
			errCh <- run(loopCtx)
		}(loop)
	}
	first := <-errCh
	cancel()
	wg.Wait()
	if errors.Is(first, context.Canceled) && ctx.Err() != nil {
		return ctx.Err()
	}
	return first
}

func waitForWorkerMetricsLoop(ctx context.Context, done <-chan error) error {
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

type workerRuntimeChecks struct {
	lookPath func(string) (string, error)
	mkdirAll func(string, os.FileMode) error
}

func newVideoArtifactHTTPClient(testCAFile string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 30 * time.Second
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.IdleConnTimeout = 90 * time.Second
	transport.ExpectContinueTimeout = time.Second
	transport.MaxResponseHeaderBytes = 1 << 20
	if strings.TrimSpace(testCAFile) != "" {
		roots, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("load system certificate pool: %w", err)
		}
		payload, err := os.ReadFile(testCAFile)
		if err != nil {
			return nil, fmt.Errorf("read test CA file: %w", err)
		}
		if !roots.AppendCertsFromPEM(payload) {
			return nil, errors.New("test CA file does not contain a valid PEM certificate")
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	}
	return &http.Client{Transport: transport}, nil
}

func validateWorkerRuntime(cfg config.WorkerConfig, checks workerRuntimeChecks) error {
	if !cfg.HasRole(config.WorkerRoleMedia) {
		return nil
	}
	if checks.lookPath == nil {
		checks.lookPath = exec.LookPath
	}
	if checks.mkdirAll == nil {
		checks.mkdirAll = os.MkdirAll
	}
	if _, err := checks.lookPath(cfg.FFmpegPath); err != nil {
		return fmt.Errorf("resolve FFmpeg executable: %w", err)
	}
	if _, err := checks.lookPath(cfg.FFprobePath); err != nil {
		return fmt.Errorf("resolve ffprobe executable: %w", err)
	}
	if err := checks.mkdirAll(cfg.TempDir, 0o700); err != nil {
		return fmt.Errorf("prepare media temporary directory: %w", err)
	}
	return nil
}

func runWorkerRoleSlots(ctx context.Context, role string, concurrency int, pollInterval time.Duration, runOnce func(context.Context) (bool, error)) error {
	if concurrency <= 0 {
		return fmt.Errorf("%s worker concurrency must be positive", role)
	}
	if runOnce == nil {
		return fmt.Errorf("%s worker processor is unavailable", role)
	}
	if pollInterval <= 0 {
		pollInterval = 500 * time.Millisecond
	}
	loops := make([]func(context.Context) error, 0, concurrency)
	for range concurrency {
		loops = append(loops, func(slotCtx context.Context) error {
			ticker := time.NewTicker(pollInterval)
			defer ticker.Stop()
			for {
				if err := slotCtx.Err(); err != nil {
					return err
				}
				processed, err := runOnce(slotCtx)
				if err != nil {
					if slotCtx.Err() != nil {
						return slotCtx.Err()
					}
					slog.ErrorContext(slotCtx, "worker role iteration failed", "role", role, "error", err)
					select {
					case <-slotCtx.Done():
						return slotCtx.Err()
					case <-ticker.C:
					}
					continue
				}
				if processed {
					continue
				}
				select {
				case <-slotCtx.Done():
					return slotCtx.Err()
				case <-ticker.C:
				}
			}
		})
	}
	return runIndependentWorkerLoops(ctx, loops...)
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
	return workerOwnerForHost(hostname, uuid.NewString())
}

func workerOwnerForHost(hostname, id string) string {
	const maxBaseOwnerBytes = 64 - len("-video")
	suffix := "-" + id
	maxHostnameBytes := maxBaseOwnerBytes - len(suffix)
	if maxHostnameBytes < 0 {
		return id[:maxBaseOwnerBytes]
	}
	if len(hostname) > maxHostnameBytes {
		hostname = hostname[:maxHostnameBytes]
	}
	if hostname == "" {
		return id
	}
	return hostname + suffix
}
