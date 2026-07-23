package localbootstrap

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/fatballfish/pic-gallery/internal/app"
	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

const (
	InstallationID = "pic-gallery-local"
	OperationID    = "local-bootstrap"
	AdminEmail     = "admin@example.com"
	AdminPassword  = "admin123456"
)

var requiredModules = []string{"admin-web", "api", "docs-web", "mailpit", "minio", "nginx", "postgres", "redis", "user-web", "worker"}

type Result struct {
	RuntimePath string
	Migration   db.MigrationResult
	Binding     setup.SetupBinding
}

type dependencies struct {
	load      func(string) (config.BootstrapConfig, config.Config, setup.InstallState, error)
	migrate   func(context.Context, config.Config) (db.MigrationResult, error)
	bind      func(context.Context, string, entstore.LocalBindingRequest) (setup.SetupBinding, error)
	hash      func(string) (string, error)
	reconcile func(string, setup.CommitProof, time.Time) error
	now       func() time.Time
}

func Run(ctx context.Context, runtimeEnvPath string) (Result, error) {
	return run(ctx, runtimeEnvPath, dependencies{
		load: loadInputs, migrate: app.RunDatabaseMigrationSnapshot,
		bind: entstore.OpenAndBindLocalInstallation, hash: adminauthservice.HashPasswordChecked,
		reconcile: reconcileState, now: func() time.Time { return time.Now().UTC() },
	})
}

func run(ctx context.Context, runtimeEnvPath string, deps dependencies) (Result, error) {
	if deps.load == nil || deps.migrate == nil {
		return Result{}, fmt.Errorf("local bootstrap dependencies are incomplete")
	}
	bootstrap, cfg, state, err := deps.load(runtimeEnvPath)
	if err != nil {
		return Result{}, fmt.Errorf("load local bootstrap configuration: %w", err)
	}
	if err := validateLocalConfiguration(bootstrap, cfg, state); err != nil {
		return Result{}, err
	}
	migration, err := deps.migrate(ctx, cfg)
	if err != nil {
		return Result{}, fmt.Errorf("run local bootstrap migration: %w", err)
	}
	if deps.bind == nil || deps.hash == nil || deps.reconcile == nil || deps.now == nil {
		return Result{}, fmt.Errorf("local bootstrap dependencies are incomplete")
	}
	passwordHash, err := deps.hash(AdminPassword)
	if err != nil {
		return Result{}, fmt.Errorf("hash local administrator password: %w", err)
	}
	binding, err := deps.bind(ctx, cfg.Database.URL, entstore.LocalBindingRequest{
		OperationID: OperationID, InstallationID: InstallationID,
		ConfigRevision: bootstrap.ConfigRevision, RuntimeValues: bootstrap.Values,
		PreferredAdminEmail: AdminEmail, FreshAdminPasswordHash: passwordHash,
	})
	if err != nil {
		return Result{}, fmt.Errorf("bind local bootstrap installation: %w", err)
	}
	proof := setup.CommitProof{
		OperationID: OperationID, InstallationID: InstallationID,
		RuntimeSchemaVersion: bootstrap.SchemaVersion, ConfigRevision: bootstrap.ConfigRevision,
		RequestDigest: binding.RequestDigest,
	}
	if err := deps.reconcile(bootstrap.Path, proof, deps.now()); err != nil {
		return Result{}, fmt.Errorf("reconcile local bootstrap install state: %w", err)
	}
	return Result{RuntimePath: bootstrap.Path, Migration: migration, Binding: binding}, nil
}

func loadInputs(runtimeEnvPath string) (config.BootstrapConfig, config.Config, setup.InstallState, error) {
	bootstrap, err := config.LoadBootstrap(runtimeEnvPath)
	if err != nil {
		return config.BootstrapConfig{}, config.Config{}, setup.InstallState{}, err
	}
	cfg, err := config.RuntimeFromBootstrap(bootstrap)
	if err != nil {
		return config.BootstrapConfig{}, config.Config{}, setup.InstallState{}, err
	}
	state, exists, err := setup.NewStateStore(bootstrap.Path).Load()
	if err != nil {
		return config.BootstrapConfig{}, config.Config{}, setup.InstallState{}, err
	}
	if !exists {
		return config.BootstrapConfig{}, config.Config{}, setup.InstallState{}, fmt.Errorf("local install state is missing")
	}
	return bootstrap, cfg, state, nil
}

func validateLocalConfiguration(bootstrap config.BootstrapConfig, cfg config.Config, state setup.InstallState) error {
	fail := func(reason string) error { return fmt.Errorf("local bootstrap configuration rejected: %s", reason) }
	if cfg.App.Env != "local" {
		return fail("application environment must be local")
	}
	if bootstrap.Deployment.Mode != config.DeploymentModeDocker || bootstrap.Deployment.Profile != config.DeploymentProfileCustom ||
		bootstrap.Deployment.Topology != config.DeploymentTopologySingle || bootstrap.Deployment.Role != config.DeploymentRoleSingle {
		return fail("deployment identity must be docker/custom/single/single")
	}
	if !bootstrap.SetupCompleted || bootstrap.InstallationID != InstallationID || cfg.Runtime.InstallationID != InstallationID || bootstrap.ConfigRevision != 1 {
		return fail("runtime installation identity is not the shared local installation")
	}
	modules := append([]string(nil), bootstrap.DeploymentModules...)
	slices.Sort(modules)
	if !slices.Equal(modules, requiredModules) {
		return fail("deployment modules do not match the shared local stack")
	}
	if err := requireLocalDatabaseURL(cfg.Database.URL); err != nil {
		return fail(err.Error())
	}
	if err := requireLocalRedisURL(cfg.Redis.URL); err != nil {
		return fail(err.Error())
	}
	if cfg.Storage.Driver != "local" || cfg.Storage.LocalRoot != "/var/lib/pic-gallery/storage" || !cfg.Storage.SharedVolume {
		return fail("storage must use the shared local volume")
	}
	if state.InstallationID != InstallationID || state.DeploymentRole != config.DeploymentRoleSingle || state.Phase != setup.InstallPhaseCompleted || !state.EverCompleted || state.Commit == nil {
		return fail("install state is not the completed shared local installation")
	}
	if state.Commit.OperationID != OperationID || state.Commit.InstallationID != InstallationID ||
		state.Commit.RuntimeSchemaVersion != bootstrap.SchemaVersion || state.Commit.ConfigRevision != bootstrap.ConfigRevision {
		return fail("install-state commit identity does not match the local runtime")
	}
	return nil
}

func requireLocalDatabaseURL(raw string) error {
	if raw != "postgres://postgres@postgres:5432/pic_gallery?sslmode=disable" {
		return fmt.Errorf("PostgreSQL URL must exactly match the shared local database URL")
	}
	return nil
}

func requireLocalRedisURL(raw string) error {
	if raw != "redis://redis:6379/0" {
		return fmt.Errorf("Redis URL must exactly match the shared local cache URL")
	}
	return nil
}

func reconcileState(runtimePath string, proof setup.CommitProof, at time.Time) error {
	_, err := setup.NewStateStore(runtimePath).ReconcileCompletedCommit(proof, at)
	return err
}
