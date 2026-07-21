package app

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
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

func TestDefaultAdminSeedRoleDefaultsToAdmin(t *testing.T) {
	if got := defaultAdminSeedRole(""); got != domainadminauth.RoleAdmin {
		t.Fatalf("defaultAdminSeedRole() = %q, want %q", got, domainadminauth.RoleAdmin)
	}
}

func TestDefaultAdminSeedRoleAllowsExplicitSuperAdmin(t *testing.T) {
	if got := defaultAdminSeedRole(" super_admin "); got != domainadminauth.RoleSuperAdmin {
		t.Fatalf("defaultAdminSeedRole() = %q, want %q", got, domainadminauth.RoleSuperAdmin)
	}
}

func TestDefaultAdminSeedRoleRejectsUnknownRole(t *testing.T) {
	if got := defaultAdminSeedRole("ops_admin"); got != domainadminauth.RoleAdmin {
		t.Fatalf("defaultAdminSeedRole() = %q, want %q", got, domainadminauth.RoleAdmin)
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
