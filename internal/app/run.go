package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fatballfish/pic-gallery/internal/app/observability"
	"github.com/fatballfish/pic-gallery/internal/config"
	domaincluster "github.com/fatballfish/pic-gallery/internal/domain/cluster"
	domainstorageconfig "github.com/fatballfish/pic-gallery/internal/domain/storageconfig"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	apphttp "github.com/fatballfish/pic-gallery/internal/http/router"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
	admincallrecordservice "github.com/fatballfish/pic-gallery/internal/service/admincallrecord"
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
	adminuserservice "github.com/fatballfish/pic-gallery/internal/service/adminuser"
	apikeyservice "github.com/fatballfish/pic-gallery/internal/service/apikey"
	assetservice "github.com/fatballfish/pic-gallery/internal/service/assets"
	auditservice "github.com/fatballfish/pic-gallery/internal/service/audit"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	cashierservice "github.com/fatballfish/pic-gallery/internal/service/cashier"
	clusterservice "github.com/fatballfish/pic-gallery/internal/service/cluster"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	modeladminservice "github.com/fatballfish/pic-gallery/internal/service/modeladmin"
	objectcleanupservice "github.com/fatballfish/pic-gallery/internal/service/objectcleanup"
	projectservice "github.com/fatballfish/pic-gallery/internal/service/project"
	promptoptimizerservice "github.com/fatballfish/pic-gallery/internal/service/promptoptimizer"
	redeemservice "github.com/fatballfish/pic-gallery/internal/service/redeem"
	secureconfigservice "github.com/fatballfish/pic-gallery/internal/service/secureconfig"
	storageconfigservice "github.com/fatballfish/pic-gallery/internal/service/storageconfig"
	textmodelservice "github.com/fatballfish/pic-gallery/internal/service/textmodel"
	"github.com/fatballfish/pic-gallery/internal/setup"
	"github.com/fatballfish/pic-gallery/internal/storage"
)

// SupervisorRestartExitCode requests a clean service-manager restart.
const SupervisorRestartExitCode = 75

// ErrSupervisorRestart is returned only after the setup response is flushed and the HTTP server shuts down.
var ErrSupervisorRestart = errors.New("api restart requested after setup completion")

// ErrStartupStorageProbe reports a failed live read-write storage check without exposing backend details.
var ErrStartupStorageProbe = errors.New("startup storage read-write probe failed")

// ExitCode maps API termination causes to process exit codes used by cmd/api.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, ErrSupervisorRestart) {
		return SupervisorRestartExitCode
	}
	return 1
}

type apiStartupDependencies struct {
	loadBootstrap    func(string) (config.BootstrapConfig, error)
	loadInstallState func(string) (setup.InstallState, bool, error)
}

type apiStartup struct {
	Mode           setup.StartupMode
	Bootstrap      config.BootstrapConfig
	State          setup.InstallState
	Decision       setup.StartupDecision
	DiagnosticCode string
}

func loadAPIStartup(runtimeEnvPath string, dependencies apiStartupDependencies) apiStartup {
	if dependencies.loadBootstrap == nil {
		dependencies.loadBootstrap = config.LoadBootstrap
	}
	if dependencies.loadInstallState == nil {
		dependencies.loadInstallState = func(path string) (setup.InstallState, bool, error) {
			return setup.NewStateStore(path).Load()
		}
	}
	bootstrap, bootstrapErr := dependencies.loadBootstrap(runtimeEnvPath)
	statePath := runtimeEnvPath
	if bootstrap.Path != "" {
		statePath = bootstrap.Path
	}
	state, stateExists, stateErr := dependencies.loadInstallState(statePath)
	if stateErr != nil {
		return apiStartup{Mode: setup.StartupModeBroken, Bootstrap: bootstrap, State: state, DiagnosticCode: "INSTALL_STATE_INVALID"}
	}
	if bootstrapErr != nil {
		return apiStartup{Mode: setup.StartupModeBroken, DiagnosticCode: "BOOTSTRAP_CONFIG_INVALID"}
	}
	decision, err := setup.ResolveStartupDecision(bootstrap, state, stateExists)
	if err != nil {
		return apiStartup{Mode: setup.StartupModeBroken, Bootstrap: bootstrap, State: state, Decision: decision, DiagnosticCode: "STARTUP_STATE_INCONSISTENT"}
	}
	startup := apiStartup{Mode: decision.Mode, Bootstrap: bootstrap, State: state, Decision: decision}
	if decision.Mode == setup.StartupModeBroken {
		startup.DiagnosticCode = "STARTUP_RECONCILIATION_REQUIRED"
	}
	return startup
}

type apiRunDependencies struct {
	runtimeEnvPath  func() string
	startup         apiStartupDependencies
	newSetupHandler func(config.BootstrapConfig) (http.Handler, error)
	runNormal       func(string, apiStartup) error
	reconcile       func(context.Context, apiStartup) error
	serve           func(string, http.Handler) error
}

func Run() error {
	return runAPI(apiRunDependencies{})
}

func runAPI(dependencies apiRunDependencies) error {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if dependencies.runtimeEnvPath == nil {
		dependencies.runtimeEnvPath = configuredRuntimeEnvPath
	}
	if dependencies.newSetupHandler == nil {
		dependencies.newSetupHandler = newSetupStartupHandler
	}
	if dependencies.runNormal == nil {
		dependencies.runNormal = runNormalStartup
	}
	if dependencies.reconcile == nil {
		dependencies.reconcile = reconcileStartupCommit
	}
	if dependencies.serve == nil {
		dependencies.serve = serveBootstrapAPI
	}
	runtimeEnvPath := dependencies.runtimeEnvPath()
	startup := loadAPIStartup(runtimeEnvPath, dependencies.startup)
	if startup.Decision.Reconciliation == setup.ReconciliationRequireDatabase {
		reconcileContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err := dependencies.reconcile(reconcileContext, startup)
		cancel()
		if err != nil {
			slog.Error("startup commit reconciliation failed", "diagnostic_code", "STARTUP_RECONCILIATION_FAILED")
			return dependencies.serve(bootstrapAddress(startup.Bootstrap), apphttp.NewBroken(handlers.NewSystemAPI(handlers.BootstrapStatus{
				Phase: handlers.BootstrapPhaseBroken, DiagnosticCode: "STARTUP_RECONCILIATION_FAILED", RetryAfterSeconds: 5,
			})))
		}
		return ErrSupervisorRestart
	}
	switch startup.Mode {
	case setup.StartupModeSetup:
		handler, err := dependencies.newSetupHandler(startup.Bootstrap)
		if err != nil {
			slog.Error("setup startup dependencies are invalid", "diagnostic_code", "SETUP_DEPENDENCIES_INVALID")
			return dependencies.serve(bootstrapAddress(startup.Bootstrap), apphttp.NewBroken(handlers.NewSystemAPI(handlers.BootstrapStatus{
				Phase: handlers.BootstrapPhaseBroken, DiagnosticCode: "SETUP_DEPENDENCIES_INVALID", RetryAfterSeconds: 5,
			})))
		}
		slog.Info("starting setup-only api", "addr", bootstrapAddress(startup.Bootstrap))
		return dependencies.serve(bootstrapAddress(startup.Bootstrap), handler)
	case setup.StartupModeBroken:
		slog.Error("api startup is fail-closed", "diagnostic_code", startup.DiagnosticCode)
		return dependencies.serve(bootstrapAddress(startup.Bootstrap), apphttp.NewBroken(handlers.NewSystemAPI(handlers.BootstrapStatus{
			Phase: handlers.BootstrapPhaseBroken, DiagnosticCode: startup.DiagnosticCode, RetryAfterSeconds: 5,
		})))
	case setup.StartupModeNormal:
		return dependencies.runNormal(runtimeEnvPath, startup)
	default:
		return fmt.Errorf("unsupported API startup mode %q", startup.Mode)
	}
}

func reconcileStartupCommit(ctx context.Context, startup apiStartup) error {
	const sessionTTL = 15 * time.Minute
	auth, err := setup.NewAuthService(setup.AuthConfig{
		Version: startup.Bootstrap.SetupTokenVersion, Completed: true,
		SessionTTL: sessionTTL, RateLimit: setup.DefaultSetupRateLimitConfig(),
	})
	if err != nil {
		return setup.ErrSetupReconciliation
	}
	service, err := setup.NewService(setup.ServiceOptions{
		RuntimeEnvPath: startup.Bootstrap.Path,
		StateStore:     setup.NewStateStore(startup.Bootstrap.Path), ProbeService: setup.NewProbeService(),
		AuthService: auth, StoreOpener: entstore.OpenSetupStore,
	})
	if err != nil {
		return setup.ErrSetupReconciliation
	}
	_, err = service.ReconcileCommit(ctx, startup.Bootstrap, startup.State)
	return err
}

func verifyCompletedStartupBinding(ctx context.Context, startup apiStartup) error {
	const sessionTTL = 15 * time.Minute
	auth, err := setup.NewAuthService(setup.AuthConfig{
		Version: startup.Bootstrap.SetupTokenVersion, Completed: true,
		SessionTTL: sessionTTL, RateLimit: setup.DefaultSetupRateLimitConfig(),
	})
	if err != nil {
		return setup.ErrSetupReconciliation
	}
	service, err := setup.NewService(setup.ServiceOptions{
		RuntimeEnvPath: startup.Bootstrap.Path,
		StateStore:     setup.NewStateStore(startup.Bootstrap.Path), ProbeService: setup.NewProbeService(),
		AuthService: auth, StoreOpener: entstore.OpenSetupStore,
	})
	if err != nil {
		return setup.ErrSetupReconciliation
	}
	return service.VerifyCompletedBinding(ctx, startup.Bootstrap, startup.State)
}

func runNormalStartup(_ string, startup apiStartup) error {
	return runNormalStartupWithOptions(startup, normalStartupOptions{})
}

type normalStartupOptions struct {
	dependencyTimeout time.Duration
}

func runNormalStartupWithOptions(startup apiStartup, options normalStartupOptions) error {
	cfg, err := config.RuntimeFromBootstrap(startup.Bootstrap)
	if err != nil {
		slog.Error("normal runtime validation failed", "diagnostic_code", "RUNTIME_CONFIG_INVALID")
		return serveBootstrapAPI(bootstrapAddress(startup.Bootstrap), apphttp.NewBroken(handlers.NewSystemAPI(handlers.BootstrapStatus{
			Phase: handlers.BootstrapPhaseBroken, DiagnosticCode: "RUNTIME_CONFIG_INVALID", RetryAfterSeconds: 5,
		})))
	}
	if !runtimeMatchesBootstrapSnapshot(cfg, startup.Bootstrap) {
		slog.Error("runtime changed after bootstrap mode selection", "diagnostic_code", "RUNTIME_SNAPSHOT_CHANGED")
		return serveBootstrapAPI(bootstrapAddress(startup.Bootstrap), apphttp.NewBroken(handlers.NewSystemAPI(handlers.BootstrapStatus{
			Phase: handlers.BootstrapPhaseBroken, DiagnosticCode: "RUNTIME_SNAPSHOT_CHANGED", RetryAfterSeconds: 5,
		})))
	}
	if err := validateStorageTopology(cfg); err != nil {
		return err
	}
	if err := authservice.ValidateProductionEmailCodeConfig(cfg.App.Env, cfg.Auth); err != nil {
		return err
	}
	if err := validateSecureConfigEncryptionKey(cfg); err != nil {
		return err
	}
	if err := validatePromptOptimizationQuoteSigningKey(cfg); err != nil {
		return err
	}
	if err := cashierservice.ConfigureStripeAPIBackend(cfg.Cashier.StripeAPIBaseURL); err != nil {
		return fmt.Errorf("configure Stripe API backend: %w", err)
	}
	dependencyTimeout := options.dependencyTimeout
	if dependencyTimeout <= 0 {
		dependencyTimeout = 15 * time.Second
	}
	startupContext, cancelStartup := context.WithTimeout(context.Background(), dependencyTimeout)
	defer cancelStartup()
	metricsContext, stopMetrics := context.WithCancel(context.Background())
	defer stopMetrics()
	observability.DefaultMetrics().Runtime().Start(metricsContext)

	client, err := db.OpenContext(startupContext, cfg.Database.URL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer client.Close()
	if err := checkRuntimeSchemaCompatibility(startupContext, client, cfg); err != nil {
		return err
	}
	if shouldVerifyOriginalSetupBinding(cfg.Runtime.DeploymentRole) {
		if err := verifyCompletedStartupBinding(startupContext, startup); err != nil {
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

	var authRedisRuntime authservice.RedisRuntime
	if redisClient != nil {
		authRedisRuntime = authservice.NewRedisRuntime(redisClient, cfg.Redis.KeyPrefix)
	}

	secureConfigSvc := secureconfigservice.NewService(entstore.NewSecureConfigStore(client), cfg.Security.SecureConfigEncryptionKey, cfg.Auth.SMTP, cfg.App.Env)
	storageConfigSvc := storageconfigservice.NewServiceWithOptions(
		entstore.NewStorageConfigStore(client), cfg.Security.SecureConfigEncryptionKey, cfg.Storage, cfg.App.Env,
		storageconfigservice.ServiceOptions{BootstrapStorageManaged: startup.Bootstrap.ObjectStorageManaged},
	)
	if err := storageConfigSvc.Bootstrap(startupContext, 0); err != nil {
		return fmt.Errorf("bootstrap storage config: %w", err)
	}
	if err := requireLegacyStorageIdentityBackfill(startupContext, client, storageConfigSvc); err != nil {
		return fmt.Errorf("prepare object cleanup storage identities: %w", err)
	}
	storageRegistry := storage.NewRegistry(storageConfigSvc, 30*time.Second)
	if err := probeDefaultStorageAtStartup(startupContext, storageConfigSvc, storageRegistry); err != nil {
		return err
	}
	cleanupRuntime := startObjectCleanupLoop(metricsContext, objectcleanupservice.NewProcessor(entstore.NewObjectCleanupStore(client), storageRegistry, objectcleanupservice.ProcessorOptions{}), 2*time.Second, 6*time.Hour)
	defer cleanupRuntime.Stop()
	var storageInvalidationBus *storage.RedisInvalidationBus
	if redisClient != nil {
		storageInvalidationBus = storage.NewRedisInvalidationBus(redisClient, cfg.Redis.KeyPrefix)
		subscriber := startStorageInvalidationSubscriber(context.Background(), storageInvalidationBus, storageRegistry.Invalidate, time.Second, 30*time.Second)
		defer subscriber.Stop()
	}
	authSvc := authservice.NewServiceWithStoreAndRedis(cfg.Auth, cfg.Billing.UserGroupMultipliers, entstore.NewAuthStore(client), authRedisRuntime, false)
	authSvc.SetSMTPConfigResolver(secureConfigSvc)
	adminSvc := adminconfigservice.NewServiceWithStore(cfg, entstore.NewAdminConfigStore(client))
	billingStore := entstore.NewBillingStore(client, cfg.Billing.PointsScale)
	billingSvc := billingservice.NewServiceWithStore(cfg.Billing, billingStore)
	billingSvc.SetAdminConfigResolver(adminSvc)
	assetSvc := assetservice.NewServiceWithStoreAndRouter(cfg.GenerationLimits, entstore.NewAssetsStore(client), storageRegistry)
	projectSvc := projectservice.NewService(entstore.NewProjectStore(client))
	taskSvc := imagetaskservice.NewServiceWithProvidersStoreAssetsBillingAndRouter(cfg, nil, entstore.NewImageTaskStore(client), assetSvc, billingSvc, storageRegistry)
	taskSvc.SetProjectResolver(projectSvc)
	modelAdminStore := entstore.NewModelAdminStore(client)
	taskSvc.SetModelRoutingSource(modelAdminStore)
	if redisClient != nil {
		taskSvc.SetConcurrencyGate(imagetaskservice.NewRedisConcurrencyGate(redisClient, cfg.Redis.KeyPrefix))
	}
	apiKeySvc, err := newRuntimeAPIKeyService(cfg, entstore.NewAPIKeyStore(client))
	if err != nil {
		return err
	}
	adminStore := entstore.NewAdminAuthStore(client)
	adminAuthSvc := adminauthservice.NewService(cfg.Auth, adminStore)
	auditSvc := auditservice.NewService(entstore.NewAuditStore(client))
	clusterStore := entstore.NewClusterStore(client)
	clusterSvc := clusterservice.NewService(clusterservice.ServiceOptions{
		Store:          clusterStore,
		InstallationID: cfg.Runtime.InstallationID, DeploymentRole: domaincluster.NodeRole(cfg.Runtime.DeploymentRole),
		RuntimeValues: startup.Bootstrap.Values, EnrollmentSealKey: startup.Bootstrap.Values["CLUSTER_ENROLLMENT_SEAL_KEY"],
	})
	adminUserSvc := adminuserservice.NewServiceWithStore(entstore.NewAdminUserStore(client, billingStore), billingSvc)
	redeemSvc := redeemservice.NewServiceWithStore(entstore.NewRedeemAdminStore(client))
	callRecordSvc := admincallrecordservice.NewServiceWithStore(entstore.NewAdminCallRecordStore(client))
	modelAdminSvc := modeladminservice.NewServiceWithStore(modelAdminStore)
	textModelStore := entstore.NewTextModelStore(client)
	textModelSvc := textmodelservice.NewService(textModelStore, cfg.Security.SecureConfigEncryptionKey)
	promptOptimizerSvc := promptoptimizerservice.NewService(textModelSvc, textModelStore, cfg.Security.PromptOptimizationQuoteSigningKey, nil)
	slog.Info("database-backed stores enabled")

	api := handlers.NewAPIWithModelAdminService(cfg, authSvc, assetSvc, taskSvc, adminSvc, billingSvc, apiKeySvc, adminAuthSvc, auditSvc, adminUserSvc, redeemSvc, callRecordSvc, modelAdminSvc)
	api.SetCashierProviderInstanceStore(entstore.NewCashierStoreWithConfigEncryptionKey(client, cfg.Cashier.ProviderConfigEncryptionKey))
	api.SetSecureConfigService(secureConfigSvc)
	api.SetTextModelServices(textModelSvc, promptOptimizerSvc)
	api.SetStorageConfigService(storageConfigSvc, storageRegistry, storageInvalidationBus)
	api.SetClusterService(clusterSvc)
	api.SetProjectService(projectSvc)
	heartbeat, err := startRuntimeHeartbeat(metricsContext, cfg, clusterStore)
	if err != nil {
		return err
	}
	defer heartbeat.Stop()

	srv := newApplicationHTTPServer(cfg.App.Addr, apphttp.NewWithAPIAndConfig(api, cfg), bootstrapServeOptions{})

	slog.Info("starting pic-gallery api", "name", cfg.App.Name, "env", cfg.App.Env, "addr", cfg.App.Addr)
	err = srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func shouldVerifyOriginalSetupBinding(role config.DeploymentRole) bool {
	return role == config.DeploymentRoleSingle || role == config.DeploymentRoleControl
}

type startupStorageResolver interface {
	ResolveDefaultWritable(context.Context) (domainstorageconfig.ResolvedConfig, error)
}

type startupStorageProber interface {
	Probe(context.Context, domainstorageconfig.ResolvedConfig) domainstorageconfig.ProbeResult
}

func probeDefaultStorageAtStartup(ctx context.Context, resolver startupStorageResolver, prober startupStorageProber) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if resolver == nil || prober == nil {
		return ErrStartupStorageProbe
	}
	resolved, err := resolver.ResolveDefaultWritable(ctx)
	if err != nil {
		return sanitizedStartupDependencyError(ctx, err, ErrStartupStorageProbe)
	}
	result := prober.Probe(ctx, resolved)
	if err := ctx.Err(); err != nil {
		return err
	}
	if result.Status != domainstorageconfig.ProbeStatusSuccess {
		return ErrStartupStorageProbe
	}
	return nil
}

func sanitizedStartupDependencyError(ctx context.Context, err, fallback error) error {
	if ctx != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return fallback
}

func runtimeMatchesBootstrapSnapshot(cfg config.Config, bootstrap config.BootstrapConfig) bool {
	return cfg.Runtime.InstallationID == bootstrap.InstallationID &&
		cfg.Runtime.ClusterNodeID == bootstrap.ClusterNodeID &&
		cfg.Runtime.ConfigSchemaVersion == bootstrap.SchemaVersion &&
		cfg.Runtime.ConfigRevision == bootstrap.ConfigRevision &&
		cfg.Runtime.ApplicationVersion == bootstrap.ApplicationVersion &&
		cfg.Runtime.DeploymentRole == bootstrap.Deployment.Role
}

func configuredRuntimeEnvPath() string {
	if path := strings.TrimSpace(os.Getenv("APP_ENV_FILE")); path != "" {
		return path
	}
	return config.DefaultRuntimeEnvPath()
}

func newSetupStartupHandler(bootstrap config.BootstrapConfig) (http.Handler, error) {
	const sessionTTL = 15 * time.Minute
	auth, err := setup.NewAuthService(setup.AuthConfig{
		Token: bootstrap.SetupToken, Version: bootstrap.SetupTokenVersion,
		Completed: bootstrap.SetupCompleted, SessionTTL: sessionTTL,
		RateLimit: setup.DefaultSetupRateLimitConfig(),
	})
	if err != nil {
		return nil, fmt.Errorf("initialize setup authentication: %w", err)
	}
	prober := setup.NewProbeService()
	application, err := setup.NewService(setup.ServiceOptions{
		RuntimeEnvPath: bootstrap.Path, StateStore: setup.NewStateStore(bootstrap.Path),
		ProbeService: prober, AuthService: auth, StoreOpener: entstore.OpenSetupStore,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize setup service: %w", err)
	}
	system := handlers.NewSystemAPI(handlers.BootstrapStatus{
		Phase: handlers.BootstrapPhaseSetupRequired, PublicAPIURL: bootstrap.Values["PUBLIC_API_URL"],
		RetryAfterSeconds: 2,
	})
	restart := make(chan struct{})
	var restartOnce sync.Once
	setupAPI, err := handlers.NewSetupAPI(handlers.SetupAPIOptions{
		System: system, Auth: auth, Prober: prober, Application: application, SessionTTL: sessionTTL,
		Bootstrap:        bootstrap,
		OnRestartPending: func() { restartOnce.Do(func() { close(restart) }) },
	})
	if err != nil {
		return nil, fmt.Errorf("initialize setup HTTP API: %w", err)
	}
	return setupRestartHandler{
		Handler: apphttp.NewSetup(setupAPI, splitBootstrapCSV(bootstrap.Values["CORS_ALLOWED_ORIGINS"])),
		restart: restart,
	}, nil
}

func splitBootstrapCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func bootstrapAddress(bootstrap config.BootstrapConfig) string {
	for _, key := range []string{"PIC_GALLERY_ADDR", "APP_ADDR"} {
		if value := strings.TrimSpace(bootstrap.Values[key]); value != "" {
			return value
		}
	}
	if port, err := strconv.Atoi(strings.TrimSpace(bootstrap.Values["API_PORT"])); err == nil && port > 0 && port <= 65535 {
		return ":" + strconv.Itoa(port)
	}
	return ":8080"
}

func serveBootstrapAPI(address string, handler http.Handler) error {
	return serveBootstrapAPIWithOptions(address, handler, bootstrapServeOptions{})
}

type bootstrapServeOptions struct {
	listener        net.Listener
	shutdownTimeout time.Duration
	readTimeout     time.Duration
	idleTimeout     time.Duration
	maxHeaderBytes  int
}

func serveBootstrapAPIWithOptions(address string, handler http.Handler, options bootstrapServeOptions) error {
	server := newApplicationHTTPServer(address, handler, options)
	restartHandler, restartEnabled := handler.(interface{ RestartSignal() <-chan struct{} })
	if !restartEnabled {
		err := server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
	listener := options.listener
	if listener == nil {
		var err error
		listener, err = net.Listen("tcp", address)
		if err != nil {
			return err
		}
	}
	serverErrors := make(chan error, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				serverErrors <- fmt.Errorf("bootstrap HTTP server panicked")
			}
		}()
		serverErrors <- server.Serve(listener)
	}()
	select {
	case serveErr := <-serverErrors:
		if errors.Is(serveErr, http.ErrServerClosed) {
			return nil
		}
		return serveErr
	case <-restartHandler.RestartSignal():
		shutdownTimeout := options.shutdownTimeout
		if shutdownTimeout <= 0 {
			shutdownTimeout = 5 * time.Second
		}
		shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		var restartWarnings []error
		if shutdownErr := server.Shutdown(shutdownContext); shutdownErr != nil {
			restartWarnings = append(restartWarnings, errors.New("bootstrap API graceful shutdown exceeded its deadline"))
			if closeErr := server.Close(); closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) {
				restartWarnings = append(restartWarnings, errors.New("bootstrap API forced close failed"))
			}
		}
		if serveErr := <-serverErrors; serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			restartWarnings = append(restartWarnings, errors.New("bootstrap API server stopped unexpectedly during restart"))
		}
		return errors.Join(append([]error{ErrSupervisorRestart}, restartWarnings...)...)
	}
}

func newApplicationHTTPServer(address string, handler http.Handler, options bootstrapServeOptions) *http.Server {
	readTimeout := options.readTimeout
	if readTimeout <= 0 {
		readTimeout = 15 * time.Second
	}
	idleTimeout := options.idleTimeout
	if idleTimeout <= 0 {
		idleTimeout = 60 * time.Second
	}
	maxHeaderBytes := options.maxHeaderBytes
	if maxHeaderBytes <= 0 {
		maxHeaderBytes = 1 << 20
	}
	return &http.Server{
		Addr: address, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}
}

type setupRestartHandler struct {
	http.Handler
	restart <-chan struct{}
}

func (handler setupRestartHandler) RestartSignal() <-chan struct{} { return handler.restart }

func validateSecureConfigEncryptionKey(cfg config.Config) error {
	key := strings.TrimSpace(cfg.Security.SecureConfigEncryptionKey)
	if isProductionEnv(cfg.App.Env) && isWeakAPIKeySigningSecretEncryptionKey(key) {
		return fmt.Errorf("secure config encryption key must be set to a non-development value in %s env", cfg.App.Env)
	}
	return nil
}

func validatePromptOptimizationQuoteSigningKey(cfg config.Config) error {
	key := strings.TrimSpace(cfg.Security.PromptOptimizationQuoteSigningKey)
	if isProductionEnv(cfg.App.Env) && isWeakAPIKeySigningSecretEncryptionKey(key) {
		return fmt.Errorf("prompt optimization quote signing key must be set to a non-development value in %s env", cfg.App.Env)
	}
	return nil
}

func newRuntimeAPIKeyService(cfg config.Config, store apikeyservice.Store) (*apikeyservice.Service, error) {
	signingSecretEncryptionKey := strings.TrimSpace(cfg.APIKey.SigningSecretEncryptionKey)
	if isProductionEnv(cfg.App.Env) && isWeakAPIKeySigningSecretEncryptionKey(signingSecretEncryptionKey) {
		return nil, fmt.Errorf("api key signing secret encryption key must be set to a non-development value in %s env", cfg.App.Env)
	}
	return apikeyservice.NewServiceWithSigningSecretKey(store, signingSecretEncryptionKey), nil
}

func isProductionEnv(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "prod", "production":
		return true
	default:
		return false
	}
}

func isWeakAPIKeySigningSecretEncryptionKey(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "secret", "password", "admin", "admin-token-secret", "admin-secret":
		return true
	default:
		return strings.HasPrefix(value, "change-me") || strings.HasPrefix(value, "local-dev") || strings.Contains(value, "example") || strings.Contains(value, "sample") || len(value) < 32
	}
}
