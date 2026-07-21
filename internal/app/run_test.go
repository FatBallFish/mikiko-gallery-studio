package app

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/fatballfish/pic-gallery/internal/config"
	domainapikey "github.com/fatballfish/pic-gallery/internal/domain/apikey"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	apikeyservice "github.com/fatballfish/pic-gallery/internal/service/apikey"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

func TestLoadAPIStartupSelectsSetupFromTolerantStateBeforeRuntimeLoad(t *testing.T) {
	bootstrap := pendingAPIBootstrapForTest()
	state := pendingAPIInstallStateForTest()
	startup := loadAPIStartup("runtime.env", apiStartupDependencies{
		loadBootstrap: func(path string) (config.BootstrapConfig, error) {
			if path != "runtime.env" {
				t.Fatalf("bootstrap path = %q", path)
			}
			return bootstrap, nil
		},
		loadInstallState: func(path string) (setup.InstallState, bool, error) {
			if path != "runtime.env" {
				t.Fatalf("install-state runtime path = %q", path)
			}
			return state, true, nil
		},
	})
	if startup.Mode != setup.StartupModeSetup || startup.Bootstrap.InstallationID != bootstrap.InstallationID || startup.DiagnosticCode != "" {
		t.Fatalf("startup = %#v, want setup mode with loaded bootstrap", startup)
	}
}

func TestLoadAPIStartupFailsClosedToBrokenWithoutLeakingLoaderError(t *testing.T) {
	secret := "postgres://operator:super-secret@database/app"
	startup := loadAPIStartup("runtime.env", apiStartupDependencies{
		loadBootstrap: func(string) (config.BootstrapConfig, error) {
			return config.BootstrapConfig{}, errors.New("cannot read " + secret)
		},
		loadInstallState: func(string) (setup.InstallState, bool, error) {
			return setup.InstallState{}, false, nil
		},
	})
	if startup.Mode != setup.StartupModeBroken || startup.DiagnosticCode != "BOOTSTRAP_CONFIG_INVALID" {
		t.Fatalf("startup = %#v, want broken bootstrap diagnostic", startup)
	}
	if strings.Contains(startup.DiagnosticCode, secret) {
		t.Fatal("startup diagnostic leaked loader error")
	}
}

func TestRunSetupModeNeverConstructsNormalDependencies(t *testing.T) {
	setupConstructed := 0
	normalConstructed := 0
	served := 0
	err := runAPI(apiRunDependencies{
		runtimeEnvPath: func() string { return "runtime.env" },
		startup: apiStartupDependencies{
			loadBootstrap: func(string) (config.BootstrapConfig, error) { return pendingAPIBootstrapForTest(), nil },
			loadInstallState: func(string) (setup.InstallState, bool, error) {
				return pendingAPIInstallStateForTest(), true, nil
			},
		},
		newSetupHandler: func(config.BootstrapConfig) (http.Handler, error) {
			setupConstructed++
			return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), nil
		},
		runNormal: func(string, apiStartup) error {
			normalConstructed++
			return nil
		},
		serve: func(string, http.Handler) error {
			served++
			return nil
		},
	})
	if err != nil || setupConstructed != 1 || normalConstructed != 0 || served != 1 {
		t.Fatalf("runAPI setup = err %v, setup %d, normal %d, served %d", err, setupConstructed, normalConstructed, served)
	}
}

func TestSupervisorRestartExitCodeIsStable(t *testing.T) {
	if !errors.Is(ErrSupervisorRestart, ErrSupervisorRestart) {
		t.Fatal("supervisor restart sentinel must support errors.Is")
	}
	if got := ExitCode(ErrSupervisorRestart); got != SupervisorRestartExitCode || got == 0 || got == 1 {
		t.Fatalf("ExitCode(ErrSupervisorRestart) = %d, want stable dedicated code %d", got, SupervisorRestartExitCode)
	}
	if got := ExitCode(errors.New("ordinary failure")); got != 1 {
		t.Fatalf("ExitCode(ordinary error) = %d, want 1", got)
	}
}

func TestServeBootstrapAPIReturnsRestartSentinelAfterSignal(t *testing.T) {
	restart := make(chan struct{})
	close(restart)
	handler := setupRestartHandler{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
		restart: restart,
	}
	if err := serveBootstrapAPI("127.0.0.1:0", handler); !errors.Is(err, ErrSupervisorRestart) {
		t.Fatalf("serveBootstrapAPI restart error = %v, want ErrSupervisorRestart", err)
	}
}

func TestServeBootstrapAPIPreservesRestartExitAfterForcedDrain(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	restart := make(chan struct{})
	requestStarted := make(chan struct{})
	handler := setupRestartHandler{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(requestStarted)
			<-r.Context().Done()
		}),
		restart: restart,
	}
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- serveBootstrapAPIWithOptions("", handler, bootstrapServeOptions{
			listener: listener, shutdownTimeout: 20 * time.Millisecond,
		})
	}()
	requestResult := make(chan error, 1)
	go func() {
		response, requestErr := http.Get("http://" + listener.Addr().String() + "/blocking")
		if response != nil {
			_ = response.Body.Close()
		}
		requestResult <- requestErr
	}()
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("blocking request did not reach bootstrap server")
	}
	close(restart)
	select {
	case serveErr := <-serveResult:
		if !errors.Is(serveErr, ErrSupervisorRestart) || ExitCode(serveErr) != SupervisorRestartExitCode {
			t.Fatalf("forced restart drain error=%v exit=%d, want wrapped restart sentinel and exit %d", serveErr, ExitCode(serveErr), SupervisorRestartExitCode)
		}
	case <-time.After(time.Second):
		t.Fatal("bootstrap server did not finish forced restart drain")
	}
	select {
	case <-requestResult:
	case <-time.After(time.Second):
		t.Fatal("blocking request goroutine did not terminate after forced close")
	}
	if connection, dialErr := net.DialTimeout("tcp", listener.Addr().String(), 20*time.Millisecond); dialErr == nil {
		_ = connection.Close()
		t.Fatal("bootstrap listener remained open after forced restart drain")
	}
}

func TestServeBootstrapAPIBoundsSlowRequestBody(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	restart := make(chan struct{})
	handler := setupRestartHandler{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusNoContent)
		}),
		restart: restart,
	}
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- serveBootstrapAPIWithOptions("", handler, bootstrapServeOptions{
			listener: listener, shutdownTimeout: time.Second,
			readTimeout: 30 * time.Millisecond, idleTimeout: time.Second, maxHeaderBytes: 8 << 10,
		})
	}()
	connection, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("dial bootstrap server: %v", err)
	}
	if _, err := io.WriteString(connection, "POST /api/setup/v1/apply HTTP/1.1\r\nHost: setup.test\r\nContent-Length: 1048576\r\n\r\nx"); err != nil {
		t.Fatalf("write partial slow request: %v", err)
	}
	if err := connection.SetReadDeadline(time.Now().Add(300 * time.Millisecond)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	buffer := make([]byte, 1)
	if _, err := connection.Read(buffer); err != nil {
		if networkErr, ok := err.(net.Error); ok && networkErr.Timeout() {
			t.Fatal("bootstrap server left slow request body open beyond configured read timeout")
		}
	}
	_ = connection.Close()
	close(restart)
	select {
	case serveErr := <-serveResult:
		if !errors.Is(serveErr, ErrSupervisorRestart) {
			t.Fatalf("slow body cleanup returned %v, want restart sentinel", serveErr)
		}
	case <-time.After(time.Second):
		t.Fatal("bootstrap server did not stop after slow body test")
	}
}

func TestRunMissingRuntimeFailsClosedWithoutSetupOrNormalDependencies(t *testing.T) {
	secret := "missing-runtime-secret"
	setupConstructed := 0
	normalConstructed := 0
	var served http.Handler
	err := runAPI(apiRunDependencies{
		runtimeEnvPath: func() string { return "missing-runtime.env" },
		startup: apiStartupDependencies{
			loadBootstrap: func(string) (config.BootstrapConfig, error) {
				return config.BootstrapConfig{}, errors.New("missing " + secret)
			},
			loadInstallState: func(string) (setup.InstallState, bool, error) {
				return setup.InstallState{}, false, nil
			},
		},
		newSetupHandler: func(config.BootstrapConfig) (http.Handler, error) {
			setupConstructed++
			return nil, nil
		},
		runNormal: func(string, apiStartup) error {
			normalConstructed++
			return nil
		},
		serve: func(_ string, handler http.Handler) error {
			served = handler
			return nil
		},
	})
	if err != nil || setupConstructed != 0 || normalConstructed != 0 || served == nil {
		t.Fatalf("missing runtime = err %v, setup %d, normal %d, served %v", err, setupConstructed, normalConstructed, served != nil)
	}
	recorder := httptest.NewRecorder()
	served.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/system/v1/bootstrap-status", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"phase":"broken"`) || strings.Contains(recorder.Body.String(), secret) {
		t.Fatalf("missing runtime bootstrap response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestRunIncompleteSetupSkeletonServesBrokenDiagnostics(t *testing.T) {
	bootstrap := pendingAPIBootstrapForTest()
	bootstrap.Path = "runtime.env"
	var served http.Handler
	err := runAPI(apiRunDependencies{
		runtimeEnvPath: func() string { return bootstrap.Path },
		startup: apiStartupDependencies{
			loadBootstrap: func(string) (config.BootstrapConfig, error) { return bootstrap, nil },
			loadInstallState: func(string) (setup.InstallState, bool, error) {
				return pendingAPIInstallStateForTest(), true, nil
			},
		},
		newSetupHandler: newSetupStartupHandler,
		runNormal:       func(string, apiStartup) error { t.Fatal("incomplete setup opened normal dependencies"); return nil },
		serve:           func(_ string, handler http.Handler) error { served = handler; return nil },
	})
	if err != nil || served == nil {
		t.Fatalf("incomplete setup run = %v, served=%v", err, served != nil)
	}
	recorder := httptest.NewRecorder()
	served.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/system/v1/bootstrap-status", nil))
	if !strings.Contains(recorder.Body.String(), `"diagnostic_code":"SETUP_DEPENDENCIES_INVALID"`) {
		t.Fatalf("incomplete setup response = %s", recorder.Body.String())
	}
}

func TestRunReconcilesDatabaseCommitBeforeAnyNormalDependencies(t *testing.T) {
	bootstrap := completedAPIBootstrapForTest()
	proof := setup.CommitProof{
		OperationID: "019d0000-0000-7000-8000-000000000002", InstallationID: bootstrap.InstallationID,
		RuntimeSchemaVersion: bootstrap.SchemaVersion, ConfigRevision: 1, RequestDigest: strings.Repeat("a", 64),
	}
	state := pendingAPIInstallStateForTest()
	state.Phase, state.Commit = setup.InstallPhaseCommitting, &proof
	reconciled, normalConstructed, setupConstructed, served := 0, 0, 0, 0
	err := runAPI(apiRunDependencies{
		runtimeEnvPath: func() string { return "runtime.env" },
		startup: apiStartupDependencies{
			loadBootstrap:    func(string) (config.BootstrapConfig, error) { return bootstrap, nil },
			loadInstallState: func(string) (setup.InstallState, bool, error) { return state, true, nil },
		},
		reconcile: func(ctx context.Context, startup apiStartup) error {
			reconciled++
			if ctx.Err() != nil || startup.Decision.Reconciliation != setup.ReconciliationRequireDatabase {
				t.Fatalf("unexpected reconciliation context/startup: %v %#v", ctx.Err(), startup)
			}
			return nil
		},
		newSetupHandler: func(config.BootstrapConfig) (http.Handler, error) { setupConstructed++; return nil, nil },
		runNormal:       func(string, apiStartup) error { normalConstructed++; return nil },
		serve:           func(string, http.Handler) error { served++; return nil },
	})
	if !errors.Is(err, ErrSupervisorRestart) || reconciled != 1 || normalConstructed != 0 || setupConstructed != 0 || served != 0 {
		t.Fatalf("recovery = err %v reconcile %d normal %d setup %d serve %d", err, reconciled, normalConstructed, setupConstructed, served)
	}
}

func TestRunFailedDatabaseReconciliationServesBrokenOnly(t *testing.T) {
	bootstrap := completedAPIBootstrapForTest()
	proof := setup.CommitProof{
		OperationID: "019d0000-0000-7000-8000-000000000002", InstallationID: bootstrap.InstallationID,
		RuntimeSchemaVersion: bootstrap.SchemaVersion, ConfigRevision: 1, RequestDigest: strings.Repeat("a", 64),
	}
	state := pendingAPIInstallStateForTest()
	state.Phase, state.Commit = setup.InstallPhaseCommitting, &proof
	for _, reconciliationErr := range []error{setup.ErrSetupReconciliation, context.Canceled, context.DeadlineExceeded} {
		t.Run(reconciliationErr.Error(), func(t *testing.T) {
			normalConstructed, setupConstructed := 0, 0
			var served http.Handler
			err := runAPI(apiRunDependencies{
				runtimeEnvPath: func() string { return "runtime.env" },
				startup: apiStartupDependencies{
					loadBootstrap:    func(string) (config.BootstrapConfig, error) { return bootstrap, nil },
					loadInstallState: func(string) (setup.InstallState, bool, error) { return state, true, nil },
				},
				reconcile:       func(context.Context, apiStartup) error { return reconciliationErr },
				newSetupHandler: func(config.BootstrapConfig) (http.Handler, error) { setupConstructed++; return nil, nil },
				runNormal:       func(string, apiStartup) error { normalConstructed++; return nil },
				serve:           func(_ string, handler http.Handler) error { served = handler; return nil },
			})
			if err != nil || normalConstructed != 0 || setupConstructed != 0 || served == nil {
				t.Fatalf("failed recovery = err %v normal %d setup %d served %v", err, normalConstructed, setupConstructed, served != nil)
			}
			recorder := httptest.NewRecorder()
			served.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/system/v1/bootstrap-status", nil))
			if !strings.Contains(recorder.Body.String(), `"diagnostic_code":"STARTUP_RECONCILIATION_FAILED"`) {
				t.Fatalf("failed reconciliation response=%s", recorder.Body.String())
			}
		})
	}
}

func TestNewSetupStartupHandlerBuildsWithoutOpeningMiddleware(t *testing.T) {
	bootstrap := pendingAPIBootstrapForTest()
	bootstrap.Path = "runtime.env"
	bootstrap.SetupToken = base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	bootstrap.SetupTokenVersion = 1
	bootstrap.Values["SETUP_TOKEN"] = bootstrap.SetupToken
	bootstrap.Values["SETUP_TOKEN_VERSION"] = "1"
	handler, err := newSetupStartupHandler(bootstrap)
	if err != nil || handler == nil {
		t.Fatalf("newSetupStartupHandler = (%T, %v), want setup handler without middleware connection", handler, err)
	}
}

func TestRuntimeSnapshotMatchRejectsRevisionOrVersionChange(t *testing.T) {
	bootstrap := config.BootstrapConfig{
		InstallationID: "installation", SchemaVersion: 1, ConfigRevision: 7,
		ApplicationVersion: "v1", Deployment: config.DeploymentContext{Role: config.DeploymentRoleSingle},
	}
	cfg := config.Config{Runtime: config.RuntimeConfig{
		InstallationID: "installation", ConfigSchemaVersion: 1, ConfigRevision: 7,
		ApplicationVersion: "v1", DeploymentRole: config.DeploymentRoleSingle,
	}}
	if !runtimeMatchesBootstrapSnapshot(cfg, bootstrap) {
		t.Fatal("identical runtime snapshot did not match")
	}
	cfg.Runtime.ConfigRevision++
	if runtimeMatchesBootstrapSnapshot(cfg, bootstrap) {
		t.Fatal("changed config revision matched bootstrap snapshot")
	}
	cfg.Runtime.ConfigRevision = bootstrap.ConfigRevision
	cfg.Runtime.ApplicationVersion = "v2"
	if runtimeMatchesBootstrapSnapshot(cfg, bootstrap) {
		t.Fatal("changed application version matched bootstrap snapshot")
	}
}

func TestRunNormalReceivesOriginalRuntimeDocumentWhenFileIsReplacedAfterModeLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.env")
	bootstrap := completedAPIBootstrapForTest()
	bootstrap.Path = path
	rendered, err := config.RenderRuntimeEnv(config.DefaultRuntimeSchema(), bootstrap.Values, nil)
	if err != nil {
		t.Fatalf("render runtime: %v", err)
	}
	if err := os.WriteFile(path, rendered, 0o600); err != nil {
		t.Fatalf("write runtime: %v", err)
	}
	proof := setup.CommitProof{
		OperationID: "019d0000-0000-7000-8000-000000000003", InstallationID: bootstrap.InstallationID,
		RuntimeSchemaVersion: bootstrap.SchemaVersion, ConfigRevision: bootstrap.ConfigRevision, RequestDigest: strings.Repeat("a", 64),
	}
	state := setup.InstallState{
		SchemaVersion: setup.CurrentInstallStateSchemaVersion, InstallationID: bootstrap.InstallationID,
		DeploymentRole: bootstrap.Deployment.Role, Phase: setup.InstallPhaseCompleted, EverCompleted: true,
		UpdatedAt: time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC), Commit: &proof,
	}
	normalCalls := 0
	err = runAPI(apiRunDependencies{
		runtimeEnvPath: func() string { return path },
		startup: apiStartupDependencies{
			loadInstallState: func(string) (setup.InstallState, bool, error) {
				replacement := completedAPIBootstrapForTest().Values
				replacement["DATABASE_URL"] = "postgres://replacement:password@db:5432/app?sslmode=disable"
				replacement["REDIS_URL"] = "redis://replacement:6379/0"
				replacement["AUTH_ACCESS_TOKEN_SECRET"] = "replacement-auth-secret"
				data, renderErr := config.RenderRuntimeEnv(config.DefaultRuntimeSchema(), replacement, nil)
				if renderErr != nil {
					t.Fatalf("render replacement: %v", renderErr)
				}
				if writeErr := os.WriteFile(path, data, 0o600); writeErr != nil {
					t.Fatalf("write replacement: %v", writeErr)
				}
				return state, true, nil
			},
		},
		runNormal: func(_ string, startup apiStartup) error {
			normalCalls++
			if startup.Bootstrap.Values["DATABASE_URL"] != bootstrap.Values["DATABASE_URL"] ||
				startup.Bootstrap.Values["REDIS_URL"] != bootstrap.Values["REDIS_URL"] ||
				startup.Bootstrap.Values["AUTH_ACCESS_TOKEN_SECRET"] != bootstrap.Values["AUTH_ACCESS_TOKEN_SECRET"] {
				t.Fatal("normal startup received mixed runtime documents")
			}
			return nil
		},
		serve: func(string, http.Handler) error {
			t.Fatal("normal snapshot unexpectedly served bootstrap mode")
			return nil
		},
	})
	if err != nil || normalCalls != 1 {
		t.Fatalf("runAPI=(%v), normal calls=%d", err, normalCalls)
	}
}

func TestRunNormalStartupBoundsDatabaseCompatibilityBeforeListen(t *testing.T) {
	listener, closeSink := newTCPBlackhole(t)
	defer closeSink()
	secret := "startup-database-secret"
	bootstrap := completedAPIBootstrapForTest()
	bootstrap.Values["PIC_GALLERY_ENV"] = "local"
	bootstrap.Values["DATABASE_URL"] = fmt.Sprintf("postgres://app:%s@%s/app?sslmode=disable", secret, listener.Addr())
	result := make(chan error, 1)
	started := time.Now()
	go func() {
		result <- runNormalStartupWithOptions(apiStartup{Bootstrap: bootstrap}, normalStartupOptions{
			dependencyTimeout: 40 * time.Millisecond,
		})
	}()
	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("bounded normal startup error=%v, want context deadline", err)
		}
		if strings.Contains(err.Error(), secret) || time.Since(started) > 300*time.Millisecond {
			t.Fatalf("bounded normal startup leaked secret or exceeded deadline: elapsed=%s err=%v", time.Since(started), err)
		}
	case <-time.After(300 * time.Millisecond):
		closeSink()
		<-result
		t.Fatal("normal startup database compatibility remained unbounded")
	}
}

func newTCPBlackhole(t *testing.T) (net.Listener, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen TCP blackhole: %v", err)
	}
	var mu sync.Mutex
	connections := make([]net.Conn, 0, 2)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			connection, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			mu.Lock()
			connections = append(connections, connection)
			mu.Unlock()
		}
	}()
	var once sync.Once
	return listener, func() {
		once.Do(func() {
			_ = listener.Close()
			<-done
			mu.Lock()
			for _, connection := range connections {
				_ = connection.Close()
			}
			mu.Unlock()
		})
	}
}

func TestNormalStartupContainsNoLegacyAdministratorSeed(t *testing.T) {
	contents, err := os.ReadFile("run.go")
	if err != nil {
		t.Fatalf("read run.go: %v", err)
	}
	for _, forbidden := range []string{"seedDefaultAdmin", "cfg.Admin", "PIC_GALLERY_ADMIN_PASSWORD"} {
		if strings.Contains(string(contents), forbidden) {
			t.Fatalf("normal startup retains legacy administrator seed path %q", forbidden)
		}
	}
}

func pendingAPIBootstrapForTest() config.BootstrapConfig {
	deployment := config.DeploymentContext{
		Mode: config.DeploymentModeDocker, Profile: config.DeploymentProfileCore,
		Topology: config.DeploymentTopologySingle, Role: config.DeploymentRoleSingle,
		StorageDriver: "local", SetupCompleted: false,
	}
	values := map[string]string{
		"RUNTIME_SCHEMA_VERSION": "1", "DEPLOYMENT_MODE": "docker", "DEPLOYMENT_PROFILE": "core",
		"DEPLOYMENT_TOPOLOGY": "single", "DEPLOYMENT_ROLE": "single", "STORAGE_DRIVER": "local",
		"INSTALLATION_ID": "019d0000-0000-7000-8000-000000000001", "SETUP_COMPLETED": "false",
	}
	return config.BootstrapConfig{
		SchemaVersion: config.CurrentRuntimeSchemaVersion, Deployment: deployment,
		InstallationID: values["INSTALLATION_ID"], Values: values,
	}
}

func completedAPIBootstrapForTest() config.BootstrapConfig {
	bootstrap := pendingAPIBootstrapForTest()
	bootstrap.SetupCompleted = true
	bootstrap.Deployment.SetupCompleted = true
	bootstrap.ConfigRevision = 1
	bootstrap.SetupTokenVersion = 1
	bootstrap.ApplicationVersion = "v1"
	for key, value := range map[string]string{
		"DEPLOYMENT_MODULES": "api,worker", "POSTGRES_MANAGED": "false", "REDIS_MANAGED": "false", "OBJECT_STORAGE_MANAGED": "false",
		"SETUP_COMPLETED": "true", "SETUP_TOKEN_VERSION": "1", "CONFIG_REVISION": "1",
		"DATABASE_URL": "postgres://app:password@127.0.0.1:5432/app?sslmode=disable", "REDIS_URL": "redis://127.0.0.1:6379/0", "REDIS_KEY_PREFIX": "app",
		"STORAGE_LOCAL_ROOT": "./data/storage", "STORAGE_SHARED_VOLUME": "true",
		"AUTH_ACCESS_TOKEN_SECRET": "access-token-secret", "API_KEY_SIGNING_SECRET_ENCRYPTION_KEY": "api-key-secret",
		"CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY": "cashier-key", "PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY": "secure-config-key-which-is-long-enough",
		"PROMPT_OPTIMIZATION_QUOTE_SIGNING_KEY": "quote-signing-key", "API_PORT": "8080", "IMAGE_TAG": "v1", "APPLICATION_VERSION": "v1",
	} {
		bootstrap.Values[key] = value
	}
	return bootstrap
}

func pendingAPIInstallStateForTest() setup.InstallState {
	return setup.InstallState{
		SchemaVersion:  setup.CurrentInstallStateSchemaVersion,
		InstallationID: "019d0000-0000-7000-8000-000000000001",
		DeploymentRole: config.DeploymentRoleSingle, Phase: setup.InstallPhasePending,
		UpdatedAt: time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC),
	}
}

func TestRuntimeSchemaCheckDoesNotCreateOrMigrateTables(t *testing.T) {
	dsn := "file:app-compatibility?mode=memory&cache=shared&_fk=1"
	client, err := repoent.Open(dialect.SQLite, dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer client.Close()
	cfg := config.Config{Runtime: config.RuntimeConfig{
		InstallationID:      "installation-test",
		ApplicationVersion:  "v1",
		ConfigSchemaVersion: config.CurrentRuntimeSchemaVersion,
	}}

	err = checkRuntimeSchemaCompatibility(context.Background(), client, cfg)
	var compatibilityErr *db.CompatibilityError
	if !errors.As(err, &compatibilityErr) || compatibilityErr.Kind != db.CompatibilityMissing {
		t.Fatalf("compatibility check error = %T %v, want typed missing error", err, err)
	}
	database, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open raw sqlite: %v", err)
	}
	defer database.Close()
	var count int
	if err := database.QueryRowContext(context.Background(), `SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`).Scan(&count); err != nil {
		t.Fatalf("inspect sqlite schema: %v", err)
	}
	if count != 0 {
		t.Fatalf("compatibility check created %d application tables", count)
	}
}

func TestOrdinaryAPIAndWorkerStartupContainNoMigrationCalls(t *testing.T) {
	for _, name := range []string{"run.go", "worker.go"} {
		contents, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		source := string(contents)
		for _, forbidden := range []string{"PrepareLegacyData(", ".Schema.Create(", "BackfillLegacyModelAccountCapabilities("} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("ordinary startup %s still contains database mutation %q", name, forbidden)
			}
		}
	}
}

func TestRunWiresAPIKeySigningSecretEncryptionKey(t *testing.T) {
	cfg := config.Config{
		App: config.AppConfig{Env: "prod"},
		APIKey: config.APIKeyConfig{
			SigningSecretEncryptionKey: "prod-runtime-api-key-signing-secret-encryption-key",
		},
	}

	svc, err := newRuntimeAPIKeyService(cfg, apikeyservice.NewMemoryStore())
	if err != nil {
		t.Fatalf("newRuntimeAPIKeyService: %v", err)
	}
	created, err := svc.CreateKey(context.Background(), apikeyservice.CreateRequest{
		UserID: 1,
		Name:   "runtime",
		Secret: "sk-runtime-secret",
	})
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}

	if got, ok := domainapikey.DecryptSigningSecret(created.Key.SigningSecret, cfg.APIKey.SigningSecretEncryptionKey); !ok || got != "sk-runtime-secret" {
		t.Fatalf("expected runtime API key service to encrypt signing secret with cfg key, got %q ok=%v", got, ok)
	}
	if _, ok := domainapikey.DecryptSigningSecret(created.Key.SigningSecret, "local-dev-api-key-signing-secret-encryption-key"); ok {
		t.Fatal("expected runtime API key service not to encrypt signing secret with default local dev key")
	}
}

func TestRunRejectsWeakAPIKeySigningSecretEncryptionKeyInProd(t *testing.T) {
	weakValues := []string{
		"",
		"secret",
		"password",
		"admin",
		"admin-secret",
		"admin-token-secret",
		"short-prod-key",
		"change-me-in-prod",
		"example-api-key-signing-secret-encryption-key",
		"local-dev-api-key-signing-secret-encryption-key",
	}
	for _, value := range weakValues {
		cfg := config.Config{
			App:    config.AppConfig{Env: "prod"},
			APIKey: config.APIKeyConfig{SigningSecretEncryptionKey: value},
		}
		if _, err := newRuntimeAPIKeyService(cfg, apikeyservice.NewMemoryStore()); err == nil {
			t.Fatalf("expected prod runtime API key service to reject weak signing secret encryption key %q", value)
		}
	}
}

func TestRunAllowsDefaultAPIKeySigningSecretEncryptionKeyOutsideProd(t *testing.T) {
	cfg := config.Config{
		App:    config.AppConfig{Env: "local"},
		APIKey: config.APIKeyConfig{SigningSecretEncryptionKey: "local-dev-api-key-signing-secret-encryption-key"},
	}
	if _, err := newRuntimeAPIKeyService(cfg, apikeyservice.NewMemoryStore()); err != nil {
		t.Fatalf("expected local runtime API key service to allow default dev signing secret encryption key: %v", err)
	}
}

func TestRunRejectsWeakSecureConfigEncryptionKeyInProd(t *testing.T) {
	weakValues := []string{
		"",
		"secret",
		"short-prod-key",
		"example-secure-config-encryption-key",
		"local-dev-secure-config-encryption-key",
	}
	for _, value := range weakValues {
		cfg := config.Config{
			App:      config.AppConfig{Env: "prod"},
			Security: config.SecurityConfig{SecureConfigEncryptionKey: value},
		}
		if err := validateSecureConfigEncryptionKey(cfg); err == nil {
			t.Fatalf("expected prod runtime to reject weak secure config encryption key %q", value)
		}
	}
}

func TestRunAllowsDefaultSecureConfigEncryptionKeyOutsideProd(t *testing.T) {
	cfg := config.Config{
		App:      config.AppConfig{Env: "local"},
		Security: config.SecurityConfig{SecureConfigEncryptionKey: "local-dev-secure-config-encryption-key"},
	}
	if err := validateSecureConfigEncryptionKey(cfg); err != nil {
		t.Fatalf("expected local runtime to allow default secure config encryption key: %v", err)
	}
}

func TestRunRejectsWeakPromptOptimizationQuoteSigningKeyInProd(t *testing.T) {
	weakValues := []string{"", "secret", "short-prod-key", "example-prompt-quote-key", "local-dev-prompt-optimization-quote-signing-key"}
	for _, value := range weakValues {
		cfg := config.Config{
			App:      config.AppConfig{Env: "prod"},
			Security: config.SecurityConfig{PromptOptimizationQuoteSigningKey: value},
		}
		if err := validatePromptOptimizationQuoteSigningKey(cfg); err == nil {
			t.Fatalf("expected prod runtime to reject weak prompt optimization quote signing key %q", value)
		}
	}
}

func TestRunAllowsDefaultPromptOptimizationQuoteSigningKeyOutsideProd(t *testing.T) {
	cfg := config.Config{
		App:      config.AppConfig{Env: "local"},
		Security: config.SecurityConfig{PromptOptimizationQuoteSigningKey: "local-dev-prompt-optimization-quote-signing-key"},
	}
	if err := validatePromptOptimizationQuoteSigningKey(cfg); err != nil {
		t.Fatalf("expected local runtime to allow default prompt optimization quote signing key: %v", err)
	}
}
