package setup

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
)

func TestSetupApplyRunsEveryPhaseAndCommitsInSafetyOrder(t *testing.T) {
	fixture := newServiceFixture(t)

	view, err := fixture.service.Apply(t.Context(), fixture.request())
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if view.OperationID != fixture.operationID || view.Phase != OperationPhaseRestartPending || view.ErrorCode != "" {
		t.Fatalf("Apply view=%+v", view)
	}
	want := []string{
		"phase:pending", "phase:validating",
		"state:attempt",
		"probe:database", "probe:redis", "probe:storage",
		"phase:initializing_database", "migrate",
		"phase:creating_admin", "hash_password", "store:initialize",
		"phase:committing_config", "auth:prepare", "state:begin",
		"runtime:render", "runtime:write", "state:finalize", "auth:commit",
		"phase:restart_pending",
	}
	if got := fixture.events.snapshot(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("events=%v\nwant=%v", got, want)
	}
	if fixture.writtenValues["SETUP_COMPLETED"] != "true" || fixture.writtenValues["SETUP_TOKEN"] != "" || fixture.writtenValues["SETUP_TOKEN_VERSION"] != "7" {
		t.Fatalf("completion fields=%#v", fixture.writtenValues)
	}
	if revision, err := strconv.Atoi(fixture.writtenValues["CONFIG_REVISION"]); err != nil || revision <= 0 {
		t.Fatalf("CONFIG_REVISION=%q err=%v", fixture.writtenValues["CONFIG_REVISION"], err)
	}
	if fixture.store.binding.RequestDigest == "" || strings.Contains(fixture.store.binding.RequestDigest, fixture.databaseSecret) {
		t.Fatalf("unsafe setup digest %q", fixture.store.binding.RequestDigest)
	}
	if fixture.store.request.AdminPasswordHash == fixture.adminPassword || fixture.store.request.AdminPasswordHash == "" {
		t.Fatalf("administrator password was not safely hashed: %q", fixture.store.request.AdminPasswordHash)
	}
	if got, err := fixture.service.Progress(t.Context(), fixture.operationID); err != nil || got.Phase != OperationPhaseRestartPending {
		t.Fatalf("same-process Progress=(%+v, %v)", got, err)
	}
}

func TestSetupApplyRejectsInvalidOrTamperedDraftBeforeSideEffects(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*serviceFixture, *ApplyRequest)
	}{
		{name: "non-canonical operation id", mutate: func(_ *serviceFixture, request *ApplyRequest) {
			request.OperationID = strings.ToUpper(request.OperationID)
		}},
		{name: "unknown field", mutate: func(_ *serviceFixture, request *ApplyRequest) { request.Runtime["ADMIN_PASSWORD"] = "smuggled" }},
		{name: "mgsctl-owned field", mutate: func(_ *serviceFixture, request *ApplyRequest) { request.Runtime["INSTALLATION_ID"] = uuid.NewString() }},
		{name: "application-owned field", mutate: func(_ *serviceFixture, request *ApplyRequest) { request.Runtime["SETUP_COMPLETED"] = "true" }},
		{name: "weak digest key", mutate: func(fixture *serviceFixture, _ *ApplyRequest) {
			fixture.bootstrap.Values["PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY"] = "short"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newServiceFixture(t)
			request := fixture.request()
			test.mutate(fixture, &request)
			_, err := fixture.service.Apply(t.Context(), request)
			if !errors.Is(err, ErrSetupValidation) {
				t.Fatalf("Apply error=%v, want ErrSetupValidation", err)
			}
			if got := fixture.events.snapshot(); len(got) > 2 {
				t.Fatalf("validation failure reached side effects: %v", got)
			}
			assertDoesNotContainSetupSecrets(t, err.Error(), fixture)
		})
	}
}

func TestSetupApplyRejectsInvalidAdministratorBeforeAnySideEffect(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ApplyRequest)
	}{
		{name: "empty password", mutate: func(request *ApplyRequest) { request.AdminPassword = "" }},
		{name: "bcrypt password too long", mutate: func(request *ApplyRequest) { request.AdminPassword = strings.Repeat("x", 73) }},
		{name: "email too long", mutate: func(request *ApplyRequest) { request.AdminEmail = strings.Repeat("a", 245) + "@example.com" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newServiceFixture(t)
			request := fixture.request()
			test.mutate(&request)
			_, err := fixture.service.Apply(t.Context(), request)
			if !errors.Is(err, ErrSetupValidation) {
				t.Fatalf("Apply error=%v", err)
			}
			if fixture.prober.calls.Load() != 0 || fixture.migrateCalls.Load() != 0 || fixture.store.initializeCalls != 0 || fixture.writeCalls.Load() != 0 {
				t.Fatalf("invalid admin reached side effects: probes=%d migrate=%d store=%d write=%d", fixture.prober.calls.Load(), fixture.migrateCalls.Load(), fixture.store.initializeCalls, fixture.writeCalls.Load())
			}
		})
	}
}

func TestSetupApplyEnforcesManagedFullMiddlewareAsReadOnly(t *testing.T) {
	tests := []struct {
		name  string
		field string
		value string
	}{
		{name: "postgres", field: "DATABASE_URL", value: "postgres://other:password@db.invalid/other"},
		{name: "redis", field: "REDIS_URL", value: "redis://:other@redis.invalid:6379/0"},
		{name: "storage driver", field: "STORAGE_DRIVER", value: "local"},
		{name: "storage endpoint", field: "STORAGE_S3_ENDPOINT", value: "http://other.invalid:9000"},
		{name: "storage region", field: "STORAGE_S3_REGION", value: "other-region"},
		{name: "storage bucket", field: "STORAGE_S3_BUCKET", value: "other-assets"},
		{name: "storage access key", field: "STORAGE_S3_ACCESS_KEY_ID", value: "other-access-key"},
		{name: "storage secret key", field: "STORAGE_S3_SECRET_ACCESS_KEY", value: "other-secret-key"},
		{name: "storage path style", field: "STORAGE_S3_FORCE_PATH_STYLE", value: "false"},
		{name: "storage prefix", field: "STORAGE_S3_PREFIX", value: "other/prefix"},
	}
	for _, flag := range []string{"POSTGRES_MANAGED", "REDIS_MANAGED", "OBJECT_STORAGE_MANAGED"} {
		t.Run("submitted "+flag, func(t *testing.T) {
			fixture := newServiceFixture(t)
			fixture.useManagedFullRuntime()
			request := fixture.request()
			request.Runtime[flag] = "false"
			if _, err := fixture.service.Apply(t.Context(), request); !errors.Is(err, ErrSetupValidation) {
				t.Fatalf("Apply error=%v", err)
			}
			if fixture.prober.calls.Load() != 0 || fixture.migrateCalls.Load() != 0 {
				t.Fatalf("managed flag tamper reached side effects: probes=%d migrate=%d", fixture.prober.calls.Load(), fixture.migrateCalls.Load())
			}
		})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newServiceFixture(t)
			fixture.useManagedFullRuntime()
			request := fixture.request()
			request.Runtime[test.field] = test.value
			if _, err := fixture.service.Apply(t.Context(), request); !errors.Is(err, ErrSetupValidation) {
				t.Fatalf("Apply error=%v", err)
			}
			if fixture.prober.calls.Load() != 0 || fixture.migrateCalls.Load() != 0 {
				t.Fatalf("managed tamper reached side effects: probes=%d migrate=%d", fixture.prober.calls.Load(), fixture.migrateCalls.Load())
			}
		})
	}

	fixture := newServiceFixture(t)
	request := fixture.request()
	request.Runtime["DATABASE_URL"] = "postgres://app:new-password@127.0.0.1:5432/app?sslmode=disable"
	if _, err := fixture.service.Apply(t.Context(), request); err != nil {
		t.Fatalf("core unmanaged Apply: %v", err)
	}

	fixture = newServiceFixture(t)
	fixture.useManagedFullRuntime()
	if _, err := fixture.service.Apply(t.Context(), fixture.request()); err != nil {
		t.Fatalf("unchanged managed full Apply: %v", err)
	}
}

func TestSetupApplyRejectsInvalidDeploymentManagementMatrix(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]string)
	}{
		{name: "docker full without managed postgres", mutate: func(values map[string]string) {
			values["POSTGRES_MANAGED"] = "false"
		}},
		{name: "docker full without managed redis", mutate: func(values map[string]string) {
			values["REDIS_MANAGED"] = "false"
		}},
		{name: "docker full without managed object storage", mutate: func(values map[string]string) {
			values["OBJECT_STORAGE_MANAGED"] = "false"
		}},
		{name: "docker full with local storage", mutate: func(values map[string]string) {
			values["STORAGE_DRIVER"] = "local"
		}},
		{name: "docker core with managed middleware", mutate: func(values map[string]string) {
			values["DEPLOYMENT_PROFILE"] = "core"
		}},
		{name: "native core with managed middleware", mutate: func(values map[string]string) {
			values["DEPLOYMENT_MODE"] = "native"
			values["DEPLOYMENT_PROFILE"] = "core"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newServiceFixture(t)
			fixture.useManagedFullRuntime()
			values := fixture.bootstrap.Values
			test.mutate(values)
			fixture.bootstrap = bootstrapFromValues(values)

			if _, err := fixture.service.Apply(t.Context(), fixture.request()); !errors.Is(err, ErrSetupValidation) {
				t.Fatalf("Apply error=%v, want ErrSetupValidation", err)
			}
			if fixture.prober.calls.Load() != 0 || fixture.migrateCalls.Load() != 0 || fixture.writeCalls.Load() != 0 {
				t.Fatalf("invalid management matrix reached side effects: probes=%d migrate=%d write=%d", fixture.prober.calls.Load(), fixture.migrateCalls.Load(), fixture.writeCalls.Load())
			}
		})
	}
}

func TestSetupApplyRequiresEveryFinalProbeToPass(t *testing.T) {
	for _, failedKind := range []string{"database", "redis", "storage"} {
		t.Run(failedKind, func(t *testing.T) {
			fixture := newServiceFixture(t)
			fixture.prober.failKind = failedKind
			_, err := fixture.service.Apply(t.Context(), fixture.request())
			if !errors.Is(err, ErrSetupProbe) {
				t.Fatalf("Apply error=%v, want ErrSetupProbe", err)
			}
			if fixture.migrateCalls.Load() != 0 || fixture.store.initializeCalls != 0 || fixture.writeCalls.Load() != 0 {
				t.Fatalf("probe failure crossed commit boundary: migrate=%d store=%d write=%d", fixture.migrateCalls.Load(), fixture.store.initializeCalls, fixture.writeCalls.Load())
			}
		})
	}
}

func TestSetupApplySurfacesAttemptCleanupFailureAndLeaderCancellation(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.prober.failKind = "database"
	fixture.state.clearErr = errors.New("injected clear failure")
	if _, err := fixture.service.Apply(t.Context(), fixture.request()); !errors.Is(err, ErrSetupCommit) {
		t.Fatalf("cleanup failure error=%v", err)
	}
	if fixture.state.state.Attempt == nil {
		t.Fatal("failed cleanup unexpectedly removed pending attempt")
	}

	fixture = newServiceFixture(t)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	view, err := fixture.service.Apply(cancelled, fixture.request())
	if !errors.Is(err, context.Canceled) || view.ErrorCode != "CANCELLED" {
		t.Fatalf("cancelled leader=(%+v, %v)", view, err)
	}
	if fixture.prober.calls.Load() != 0 || fixture.migrateCalls.Load() != 0 || fixture.store.initializeCalls != 0 || fixture.state.state.Attempt != nil {
		t.Fatalf("cancelled leader reached side effects: probes=%d migrate=%d store=%d attempt=%+v", fixture.prober.calls.Load(), fixture.migrateCalls.Load(), fixture.store.initializeCalls, fixture.state.state.Attempt)
	}
}

func TestSetupApplyRetriesDatabaseSuccessWithoutPassword(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.failCheckpoint = "before_state_begin"
	if _, err := fixture.service.Apply(t.Context(), fixture.request()); !errors.Is(err, ErrSetupCommit) {
		t.Fatalf("first Apply error=%v", err)
	}
	if fixture.store.initializeCalls != 1 || fixture.writeCalls.Load() != 0 {
		t.Fatalf("first attempt store=%d write=%d", fixture.store.initializeCalls, fixture.writeCalls.Load())
	}
	stateJSON, err := json.Marshal(fixture.state.state)
	if err != nil {
		t.Fatalf("marshal pending attempt: %v", err)
	}
	assertDoesNotContainSetupSecrets(t, string(stateJSON), fixture)
	if strings.Contains(string(stateJSON), "root@example.com") {
		t.Fatalf("pending attempt persisted administrator email: %s", stateJSON)
	}
	barePasswordDigest := fmt.Sprintf("%x", sha256.Sum256([]byte(fixture.adminPassword)))
	if fixture.state.state.Attempt == nil || fixture.state.state.Attempt.AdminCredentialVerifier == "" ||
		strings.Contains(string(stateJSON), fixture.adminPassword) || strings.Contains(string(stateJSON), barePasswordDigest) {
		t.Fatalf("pending attempt did not persist an opaque keyed credential verifier: %s", stateJSON)
	}

	fixture.failCheckpoint = ""
	retry := fixture.request()
	retry.AdminPassword = ""
	view, err := fixture.service.Apply(t.Context(), retry)
	if err != nil || view.Phase != OperationPhaseRestartPending {
		t.Fatalf("passwordless retry=(%+v, %v)", view, err)
	}
	if fixture.migrateCalls.Load() != 1 || fixture.store.initializeCalls != 1 || fixture.store.adminCreations != 1 {
		t.Fatalf("retry migrate=%d store calls=%d admin creations=%d", fixture.migrateCalls.Load(), fixture.store.initializeCalls, fixture.store.adminCreations)
	}
}

func TestSetupApplyPendingAttemptRejectsChangedPasswordAfterRestart(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.failCheckpoint = "after_migration"
	if _, err := fixture.service.Apply(t.Context(), fixture.request()); !errors.Is(err, ErrSetupMigration) {
		t.Fatalf("first Apply error=%v", err)
	}
	probes := fixture.prober.calls.Load()
	migrations := fixture.migrateCalls.Load()
	fixture.failCheckpoint = ""
	changed := fixture.request()
	changed.AdminPassword = "different administrator password"
	if _, err := fixture.newService().Apply(t.Context(), changed); !errors.Is(err, ErrSetupBindingMismatch) {
		t.Fatalf("changed password recovery error=%v", err)
	}
	if fixture.prober.calls.Load() != probes || fixture.migrateCalls.Load() != migrations || fixture.store.initializeCalls != 0 {
		t.Fatalf("changed password crossed recovery boundary: probes=%d/%d migrate=%d/%d store=%d", fixture.prober.calls.Load(), probes, fixture.migrateCalls.Load(), migrations, fixture.store.initializeCalls)
	}
}

func TestSetupRecoveryOperationIDLoadsPersistedAttemptAfterRestart(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.failCheckpoint = "after_migration"
	request := fixture.request()
	if _, err := fixture.service.Apply(t.Context(), request); !errors.Is(err, ErrSetupMigration) {
		t.Fatalf("first Apply error=%v", err)
	}

	operationID, err := fixture.newService().RecoveryOperationID()
	if err != nil || operationID != request.OperationID {
		t.Fatalf("RecoveryOperationID=(%q, %v), want persisted operation %q", operationID, err, request.OperationID)
	}
}

func TestSetupApplyAfterMigrationCrashDoesNotMigrateAgain(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.bootstrap.Values["CONFIG_REVISION"] = "7"
	fixture.bootstrap = bootstrapFromValues(fixture.bootstrap.Values)
	fixture.failCheckpoint = "after_migration"
	request := fixture.request()
	if _, err := fixture.service.Apply(t.Context(), request); !errors.Is(err, ErrSetupMigration) {
		t.Fatalf("first Apply error=%v", err)
	}
	if fixture.migrateCalls.Load() != 1 || fixture.store.binding != (SetupBinding{}) {
		t.Fatalf("first Apply migrate=%d binding=%+v", fixture.migrateCalls.Load(), fixture.store.binding)
	}

	fixture.failCheckpoint = ""
	view, err := fixture.newService().Apply(t.Context(), request)
	if err != nil || view.Phase != OperationPhaseRestartPending {
		t.Fatalf("recovered Apply=(%+v, %v)", view, err)
	}
	if fixture.migrateCalls.Load() != 1 || fixture.store.adminCreations != 1 {
		t.Fatalf("recovered Apply migrate=%d admins=%d", fixture.migrateCalls.Load(), fixture.store.adminCreations)
	}
}

func TestSetupApplyAfterMigrationCrashStillRequiresBoundPassword(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.failCheckpoint = "after_migration"
	if _, err := fixture.service.Apply(t.Context(), fixture.request()); !errors.Is(err, ErrSetupMigration) {
		t.Fatalf("first Apply error=%v", err)
	}
	probes := fixture.prober.calls.Load()
	migrations := fixture.migrateCalls.Load()
	fixture.failCheckpoint = ""
	retry := fixture.request()
	retry.AdminPassword = ""
	if _, err := fixture.newService().Apply(t.Context(), retry); !errors.Is(err, ErrSetupValidation) {
		t.Fatalf("passwordless unbound recovery error=%v", err)
	}
	if fixture.prober.calls.Load() != probes || fixture.migrateCalls.Load() != migrations || fixture.store.initializeCalls != 0 {
		t.Fatalf("passwordless unbound recovery reached side effects: probes=%d/%d migrate=%d/%d store=%d", fixture.prober.calls.Load(), probes, fixture.migrateCalls.Load(), migrations, fixture.store.initializeCalls)
	}
}

func TestSetupApplyPendingAttemptRejectsDifferentOperationAndRuntimeBeforeDatabaseSideEffects(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.failCheckpoint = "before_state_begin"
	if _, err := fixture.service.Apply(t.Context(), fixture.request()); !errors.Is(err, ErrSetupCommit) {
		t.Fatalf("first Apply error=%v", err)
	}
	migrations := fixture.migrateCalls.Load()
	admins := fixture.store.adminCreations

	fixture.failCheckpoint = ""
	competing := fixture.request()
	competing.OperationID = uuid.NewString()
	competing.Runtime["DATABASE_URL"] = "postgres://other:password@different.invalid/other"
	if _, err := fixture.newService().Apply(t.Context(), competing); !errors.Is(err, ErrSetupOperationConflict) {
		t.Fatalf("competing Apply error=%v", err)
	}
	if fixture.migrateCalls.Load() != migrations || fixture.store.adminCreations != admins {
		t.Fatalf("competing request reached DB: migrate=%d/%d admins=%d/%d", fixture.migrateCalls.Load(), migrations, fixture.store.adminCreations, admins)
	}
}

func TestSetupApplyResumesJournalBeforeRuntimeRename(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.failCheckpoint = "before_runtime_write"
	if _, err := fixture.service.Apply(t.Context(), fixture.request()); !errors.Is(err, ErrSetupCommit) {
		t.Fatalf("first Apply error=%v", err)
	}
	if fixture.state.state.Phase != InstallPhaseCommitting || fixture.writeCalls.Load() != 0 {
		t.Fatalf("first attempt state=%+v write=%d", fixture.state.state, fixture.writeCalls.Load())
	}

	fixture.failCheckpoint = ""
	retry := fixture.request()
	retry.AdminPassword = ""
	view, err := fixture.service.Apply(t.Context(), retry)
	if err != nil || view.Phase != OperationPhaseRestartPending {
		t.Fatalf("journal retry=(%+v, %v)", view, err)
	}
	if fixture.migrateCalls.Load() != 1 || fixture.store.initializeCalls != 1 {
		t.Fatalf("journal retry repeated DB work: migrate=%d store=%d", fixture.migrateCalls.Load(), fixture.store.initializeCalls)
	}
}

func TestSetupApplySealsAuthenticationWhenRuntimeRenameCrossesBoundary(t *testing.T) {
	for _, checkpoint := range []string{"after_runtime_write", "after_state_finalize", "after_auth_commit"} {
		t.Run(checkpoint, func(t *testing.T) {
			fixture := newServiceFixture(t)
			fixture.failCheckpoint = checkpoint
			_, err := fixture.service.Apply(t.Context(), fixture.request())
			if !errors.Is(err, ErrSetupCommit) {
				t.Fatalf("Apply error=%v", err)
			}
			if !fixture.auth.sealed {
				t.Fatal("authentication remained open after runtime commit boundary")
			}
			if fixture.auth.abortCalls != 0 {
				t.Fatalf("authentication completion was aborted after runtime commit: %d", fixture.auth.abortCalls)
			}
		})
	}
}

func TestSetupApplyConcurrentCallersJoinAndWaiterCancellationIsIsolated(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.blockMigration = make(chan struct{})
	leaderResult := make(chan error, 1)
	go func() {
		_, err := fixture.service.Apply(context.Background(), fixture.request())
		leaderResult <- err
	}()
	fixture.waitForEvent(t, "migrate")

	waiterCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fixture.service.Apply(waiterCtx, fixture.request()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled waiter error=%v", err)
	}
	other := fixture.request()
	other.OperationID = uuid.NewString()
	if _, err := fixture.service.Apply(t.Context(), other); !errors.Is(err, ErrSetupOperationConflict) {
		t.Fatalf("different operation error=%v", err)
	}

	close(fixture.blockMigration)
	if err := <-leaderResult; err != nil {
		t.Fatalf("leader Apply: %v", err)
	}
	if fixture.migrateCalls.Load() != 1 || fixture.store.initializeCalls != 1 {
		t.Fatalf("joined operation duplicated work: migrate=%d store=%d", fixture.migrateCalls.Load(), fixture.store.initializeCalls)
	}
}

func TestSetupApplyRejectsDifferentNonEmptyPasswordForActiveOperation(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.blockMigration = make(chan struct{})
	leaderResult := make(chan error, 1)
	go func() {
		_, err := fixture.service.Apply(context.Background(), fixture.request())
		leaderResult <- err
	}()
	fixture.waitForEvent(t, "migrate")

	changed := fixture.request()
	changed.AdminPassword = "a different administrator password"
	if _, err := fixture.service.Apply(t.Context(), changed); !errors.Is(err, ErrSetupBindingMismatch) {
		t.Fatalf("changed password error=%v", err)
	}
	close(fixture.blockMigration)
	if err := <-leaderResult; err != nil {
		t.Fatalf("leader Apply: %v", err)
	}
}

func TestSetupApplySerializesDifferentServiceInstancesWithRuntimeFileLock(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.blockMigration = make(chan struct{})
	runtimePath := t.TempDir() + "/runtime.env"
	fixture.service.dependencies.lockApply = newRuntimeApplyLocker(runtimePath)
	second := fixture.newService()
	second.dependencies.lockApply = newRuntimeApplyLocker(runtimePath)

	leaderResult := make(chan error, 1)
	go func() {
		_, err := fixture.service.Apply(context.Background(), fixture.request())
		leaderResult <- err
	}()
	fixture.waitForEvent(t, "migrate")
	secondResult := make(chan error, 1)
	go func() {
		request := fixture.request()
		request.OperationID = uuid.NewString()
		_, err := second.Apply(context.Background(), request)
		secondResult <- err
	}()
	time.Sleep(50 * time.Millisecond)
	if fixture.migrateCalls.Load() != 1 {
		t.Fatalf("second Service crossed runtime lock; migrations=%d", fixture.migrateCalls.Load())
	}
	close(fixture.blockMigration)
	if err := <-leaderResult; err != nil {
		t.Fatalf("leader Apply: %v", err)
	}
	if err := <-secondResult; !errors.Is(err, ErrSetupOperationConflict) {
		t.Fatalf("second operation error=%v", err)
	}
}

func TestSetupApplyReleasesRuntimeLockAfterDependencyPanic(t *testing.T) {
	fixture := newServiceFixture(t)
	runtimePath := t.TempDir() + "/runtime.env"
	fixture.service.dependencies.lockApply = newRuntimeApplyLocker(runtimePath)
	fixture.panicMigration = true
	if _, err := fixture.service.Apply(t.Context(), fixture.request()); !errors.Is(err, ErrSetupCommit) {
		t.Fatalf("panic Apply error=%v", err)
	} else if strings.Contains(strings.ToLower(err.Error()), "panic") {
		t.Fatalf("panic detail leaked: %v", err)
	}
	fixture.panicMigration = false
	fixture.prober.failKind = "database"
	request := fixture.request()
	started := time.Now()
	if _, err := fixture.service.Apply(t.Context(), request); !errors.Is(err, ErrSetupProbe) {
		t.Fatalf("second Apply error=%v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("panic poisoned runtime lock for %s", elapsed)
	}
}

func TestRuntimeApplyLockerCancelsWhileOSFileLockIsHeld(t *testing.T) {
	runtimePath := t.TempDir() + "/runtime.env"
	lockPath, err := normalizeStatePath(runtimePath + ".setup.lock")
	if err != nil {
		t.Fatalf("normalize lock path: %v", err)
	}
	held, err := acquireStateFileLock(lockPath, time.Second, platformStateAtomicOps())
	if err != nil {
		t.Fatalf("hold OS file lock: %v", err)
	}
	defer func() { _ = held.release() }()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := newRuntimeApplyLocker(runtimePath)(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancelled file-lock waiter error=%v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("cancelled file-lock waiter took %s", elapsed)
	}
}

func TestRuntimeApplyLockerCancelsAcrossProcessAndRemainsUsable(t *testing.T) {
	directory := t.TempDir()
	runtimePath := filepath.Join(directory, "runtime.env")
	readyPath := filepath.Join(directory, "ready")
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create helper pipe: %v", err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestRuntimeApplyLockSubprocessHelper$")
	command.Env = append(os.Environ(),
		"PIC_GALLERY_APPLY_LOCK_HELPER=1",
		"PIC_GALLERY_APPLY_LOCK_RUNTIME="+runtimePath,
		"PIC_GALLERY_APPLY_LOCK_READY="+readyPath,
	)
	command.Stdin = reader
	if err := command.Start(); err != nil {
		t.Fatalf("start lock helper: %v", err)
	}
	_ = reader.Close()
	waited := false
	t.Cleanup(func() {
		_ = writer.Close()
		if !waited && command.Process != nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(readyPath); err == nil {
			break
		}
		if !time.Now().Before(deadline) {
			t.Fatal("subprocess did not acquire apply file lock")
		}
		time.Sleep(10 * time.Millisecond)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	if _, err := newRuntimeApplyLocker(runtimePath)(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cross-process cancelled waiter error=%v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("release helper: %v", err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("lock helper: %v", err)
	}
	waited = true

	unlock, err := newRuntimeApplyLocker(runtimePath)(t.Context())
	if err != nil {
		t.Fatalf("acquire after helper release: %v", err)
	}
	if err := unlock(); err != nil {
		t.Fatalf("release after helper: %v", err)
	}
}

func TestRuntimeApplyLockSubprocessHelper(t *testing.T) {
	if os.Getenv("PIC_GALLERY_APPLY_LOCK_HELPER") != "1" {
		return
	}
	runtimePath := os.Getenv("PIC_GALLERY_APPLY_LOCK_RUNTIME")
	lockPath, err := normalizeStatePath(runtimePath + ".setup.lock")
	if err != nil {
		t.Fatalf("normalize helper lock path: %v", err)
	}
	lock, err := acquireStateFileLock(lockPath, 5*time.Second, platformStateAtomicOps())
	if err != nil {
		t.Fatalf("acquire helper lock: %v", err)
	}
	defer func() { _ = lock.release() }()
	if err := os.WriteFile(os.Getenv("PIC_GALLERY_APPLY_LOCK_READY"), []byte("ready"), 0o600); err != nil {
		t.Fatalf("signal helper ready: %v", err)
	}
	buffer := make([]byte, 1)
	_, _ = os.Stdin.Read(buffer)
}

func TestSetupApplyTreatsWriterErrorAfterRenameAsFailClosedCommitBoundary(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.writeErrorAfterCommit = true
	fixture.failLoadAfterWrite = true
	_, err := fixture.service.Apply(t.Context(), fixture.request())
	if !errors.Is(err, ErrSetupCommit) {
		t.Fatalf("Apply error=%v", err)
	}
	if !fixture.bootstrap.SetupCompleted || fixture.state.state.Phase != InstallPhaseCommitting || !fixture.auth.sealed {
		t.Fatalf("ambiguous writer boundary completed=%t phase=%q sealed=%t", fixture.bootstrap.SetupCompleted, fixture.state.state.Phase, fixture.auth.sealed)
	}
}

func TestSetupApplyTreatsWriterPanicAsFailClosedCommitBoundary(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.panicWriter = true
	view, err := fixture.service.Apply(t.Context(), fixture.request())
	if !errors.Is(err, ErrSetupCommit) || view.ErrorCode != "COMMIT_FAILED" {
		t.Fatalf("writer panic=(%+v, %v)", view, err)
	}
	if !fixture.auth.sealed || fixture.auth.abortCalls != 0 || fixture.state.state.Phase != InstallPhaseCommitting {
		t.Fatalf("writer panic sealed=%t abort=%d state=%+v", fixture.auth.sealed, fixture.auth.abortCalls, fixture.state.state)
	}
	for index, value := range fixture.retainedRuntimeData {
		if value != 0 {
			t.Fatalf("writer panic retained rendered byte %d=%d", index, value)
		}
	}
}

func TestSetupApplyPreservesTypedStoreConflicts(t *testing.T) {
	tests := []struct {
		err  error
		code string
	}{
		{err: ErrSetupOperationConflict, code: "OPERATION_CONFLICT"},
		{err: ErrSetupBindingMismatch, code: "BINDING_MISMATCH"},
		{err: ErrFirstAdminConflict, code: "FIRST_ADMIN_CONFLICT"},
	}
	for _, test := range tests {
		t.Run(test.code, func(t *testing.T) {
			fixture := newServiceFixture(t)
			fixture.store.initializeErr = test.err
			view, err := fixture.service.Apply(t.Context(), fixture.request())
			if !errors.Is(err, test.err) || view.ErrorCode != test.code {
				t.Fatalf("Apply=(%+v, %v), want %v/%s", view, err, test.err, test.code)
			}
			assertDoesNotContainSetupSecrets(t, fmt.Sprintf("%+v %v", view, err), fixture)
		})
	}
}

func TestSetupApplyPreservesMGSCTLRuntimeExtensions(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.bootstrap.Values["MGSCTL_EXTENSION"] = "retained"
	if _, err := fixture.service.Apply(t.Context(), fixture.request()); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if fixture.writtenValues["MGSCTL_EXTENSION"] != "retained" {
		t.Fatalf("extension was lost: %#v", fixture.writtenValues)
	}
}

func TestSetupRequestDigestBindsCanonicalRuntimeAndNormalizedAdminEmail(t *testing.T) {
	values := pendingRuntimeValues()
	values["SETUP_COMPLETED"] = "true"
	values["SETUP_TOKEN"] = ""
	first, err := setupRequestDigest(values, "root@example.com")
	if err != nil {
		t.Fatalf("setupRequestDigest: %v", err)
	}
	caseNormalized, err := setupRequestDigest(values, " ROOT@EXAMPLE.COM ")
	if err != nil || caseNormalized != first {
		t.Fatalf("normalized email digest=%q err=%v, want %q", caseNormalized, err, first)
	}
	otherEmail, _ := setupRequestDigest(values, "other@example.com")
	if otherEmail == first {
		t.Fatal("request digest did not bind administrator email")
	}
	changed := serviceCloneValues(values)
	changed["REDIS_KEY_PREFIX"] = "other"
	otherRuntime, _ := setupRequestDigest(changed, "root@example.com")
	if otherRuntime == first {
		t.Fatal("request digest did not bind runtime snapshot")
	}
}

func TestCanonicalRequestDigestMatchesSetupDigest(t *testing.T) {
	values := pendingRuntimeValues()
	values["SETUP_COMPLETED"] = "true"
	values["SETUP_TOKEN"] = ""
	want, err := setupRequestDigest(values, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	got, err := CanonicalRequestDigest(values, " ADMIN@example.com ")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("CanonicalRequestDigest = %q, want %q", got, want)
	}
}

func TestSetupApplyFailureCheckpointsNeverPublishCompletionEarly(t *testing.T) {
	tests := []struct {
		checkpoint       string
		wantError        error
		completedRuntime bool
	}{
		{checkpoint: "after_validation", wantError: ErrSetupValidation},
		{checkpoint: "before_migration", wantError: ErrSetupMigration},
		{checkpoint: "after_migration", wantError: ErrSetupMigration},
		{checkpoint: "after_database_binding", wantError: ErrSetupCommit},
		{checkpoint: "before_state_begin", wantError: ErrSetupCommit},
		{checkpoint: "before_runtime_write", wantError: ErrSetupCommit},
		{checkpoint: "after_runtime_write", wantError: ErrSetupCommit, completedRuntime: true},
		{checkpoint: "after_state_finalize", wantError: ErrSetupCommit, completedRuntime: true},
		{checkpoint: "after_auth_commit", wantError: ErrSetupCommit, completedRuntime: true},
	}
	for _, test := range tests {
		t.Run(test.checkpoint, func(t *testing.T) {
			fixture := newServiceFixture(t)
			fixture.failCheckpoint = test.checkpoint
			_, err := fixture.service.Apply(t.Context(), fixture.request())
			if !errors.Is(err, test.wantError) {
				t.Fatalf("Apply error=%v, want %v", err, test.wantError)
			}
			if fixture.bootstrap.SetupCompleted != test.completedRuntime {
				t.Fatalf("SETUP_COMPLETED=%t, want %t", fixture.bootstrap.SetupCompleted, test.completedRuntime)
			}
			if test.completedRuntime && !fixture.auth.sealed {
				t.Fatal("authentication was not fail-closed after completed runtime publication")
			}
		})
	}
}

func TestSetupApplyRejectsNonAuthorityRoleWithoutMigration(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.bootstrap.Deployment.Role = config.DeploymentRoleAPI
	fixture.bootstrap.Values["DEPLOYMENT_ROLE"] = string(config.DeploymentRoleAPI)
	fixture.state.state.DeploymentRole = config.DeploymentRoleAPI
	if _, err := fixture.service.Apply(t.Context(), fixture.request()); !errors.Is(err, ErrSetupValidation) {
		t.Fatalf("Apply error=%v", err)
	}
	if fixture.migrateCalls.Load() != 0 {
		t.Fatalf("non-authority role ran migrations=%d", fixture.migrateCalls.Load())
	}
}

func TestSetupApplySameOperationWaiterObservesLeaderResult(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.blockMigration = make(chan struct{})
	leaderResult := make(chan error, 1)
	waiterResult := make(chan error, 1)
	go func() {
		_, err := fixture.service.Apply(context.Background(), fixture.request())
		leaderResult <- err
	}()
	fixture.waitForEvent(t, "migrate")
	waiterRequest := fixture.request()
	go func() {
		_, err := fixture.service.Apply(context.Background(), waiterRequest)
		waiterResult <- err
	}()
	close(fixture.blockMigration)
	if err := <-leaderResult; err != nil {
		t.Fatalf("leader Apply: %v", err)
	}
	if err := <-waiterResult; err != nil {
		t.Fatalf("waiter Apply: %v", err)
	}
	if fixture.migrateCalls.Load() != 1 {
		t.Fatalf("same operation ran migrations=%d", fixture.migrateCalls.Load())
	}
}

func TestSetupServiceBoundsTerminalOperationsAndClearsSensitiveBuffers(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.prober.failKind = "database"
	for range maxTerminalOperations + 25 {
		request := fixture.request()
		request.OperationID = uuid.NewString()
		_, _ = fixture.service.Apply(t.Context(), request)
	}
	if got := fixture.service.operationCountForTest(); got > maxTerminalOperations {
		t.Fatalf("terminal operation cache=%d, max=%d", got, maxTerminalOperations)
	}
	for _, operation := range fixture.service.operations {
		if operation.passwordFingerprint != ([32]byte{}) {
			t.Fatal("terminal operation retained password fingerprint")
		}
	}

	fixture = newServiceFixture(t)
	if _, err := fixture.service.Apply(t.Context(), fixture.request()); err != nil {
		t.Fatalf("successful Apply: %v", err)
	}
	if len(fixture.retainedRuntimeData) == 0 {
		t.Fatal("writer did not retain rendered buffer for zeroization assertion")
	}
	for index, value := range fixture.retainedRuntimeData {
		if value != 0 {
			t.Fatalf("rendered secret buffer byte %d=%d, want zero", index, value)
		}
	}
}

func TestSetupApplyCompletedReplayAfterRestartReturnsCompleteWithoutSideEffects(t *testing.T) {
	fixture := newServiceFixture(t)
	request := fixture.request()
	if _, err := fixture.service.Apply(t.Context(), request); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	migrations := fixture.migrateCalls.Load()
	admins := fixture.store.adminCreations
	writes := fixture.writeCalls.Load()

	view, err := fixture.newService().Apply(t.Context(), request)
	if err != nil || view.Phase != OperationPhaseComplete {
		t.Fatalf("completed replay=(%+v, %v)", view, err)
	}
	if fixture.migrateCalls.Load() != migrations || fixture.store.adminCreations != admins || fixture.writeCalls.Load() != writes {
		t.Fatalf("completed replay ran side effects: migrate=%d/%d admins=%d/%d writes=%d/%d", fixture.migrateCalls.Load(), migrations, fixture.store.adminCreations, admins, fixture.writeCalls.Load(), writes)
	}
}

func TestNewServiceReturnsEntropyFailureWithoutPanic(t *testing.T) {
	_, err := newServiceWithEntropy(ServiceOptions{
		RuntimeEnvPath: "runtime.env",
		StateStore:     &StateStore{},
		ProbeService:   &ProbeService{},
		AuthService:    &AuthService{},
		StoreOpener: func(context.Context, string) (SetupStoreSession, error) {
			return nil, errors.New("unused")
		},
	}, &errorReader{err: errors.New("entropy unavailable")})
	if err == nil || !strings.Contains(err.Error(), "fingerprint key") {
		t.Fatalf("NewService entropy error=%v", err)
	}
}

func TestSetupApplyCompletedReplayRejectsTamperedCommitJournalDigest(t *testing.T) {
	fixture := newServiceFixture(t)
	request := fixture.request()
	if _, err := fixture.service.Apply(t.Context(), request); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	fixture.state.mu.Lock()
	fixture.state.state.Commit.RequestDigest = strings.Repeat("f", 64)
	fixture.state.mu.Unlock()
	migrations := fixture.migrateCalls.Load()
	admins := fixture.store.adminCreations
	writes := fixture.writeCalls.Load()
	fixture.auth = &fakeCompletionAuth{events: fixture.events}

	view, err := fixture.newService().Apply(t.Context(), request)
	if !errors.Is(err, ErrSetupBindingMismatch) || view.ErrorCode != "BINDING_MISMATCH" {
		t.Fatalf("completed replay error=%v, want ErrSetupBindingMismatch", err)
	}
	if !fixture.auth.sealed {
		t.Fatal("tampered completed replay did not fail closed")
	}
	if fixture.migrateCalls.Load() != migrations || fixture.store.adminCreations != admins || fixture.writeCalls.Load() != writes {
		t.Fatalf("rejected replay ran side effects: migrate=%d/%d admins=%d/%d writes=%d/%d", fixture.migrateCalls.Load(), migrations, fixture.store.adminCreations, admins, fixture.writeCalls.Load(), writes)
	}
}

func TestSetupProgressAfterRestartRequiresMatchingRuntimeStateAndDatabase(t *testing.T) {
	fixture := newServiceFixture(t)
	if _, err := fixture.service.Apply(t.Context(), fixture.request()); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	restarted := fixture.newService()
	view, err := restarted.Progress(t.Context(), fixture.operationID)
	if err != nil || view.Phase != OperationPhaseComplete {
		t.Fatalf("restarted Progress=(%+v, %v)", view, err)
	}

	fixture.store.binding.RequestDigest = strings.Repeat("f", 64)
	broken := fixture.newService()
	if _, err := broken.Progress(t.Context(), fixture.operationID); !errors.Is(err, ErrSetupReconciliation) {
		t.Fatalf("mismatched database binding error=%v", err)
	}
}

func TestSetupProgressReconcilesRuntimeRenameBeforeStateFinalize(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.failCheckpoint = "after_runtime_write"
	if _, err := fixture.service.Apply(t.Context(), fixture.request()); !errors.Is(err, ErrSetupCommit) {
		t.Fatalf("Apply error=%v", err)
	}
	if fixture.state.state.Phase != InstallPhaseCommitting || !fixture.bootstrap.SetupCompleted {
		t.Fatalf("crash window state=%+v bootstrap completed=%t", fixture.state.state, fixture.bootstrap.SetupCompleted)
	}

	restarted := fixture.newService()
	view, err := restarted.Progress(t.Context(), fixture.operationID)
	if err != nil || view.Phase != OperationPhaseComplete {
		t.Fatalf("reconciled Progress=(%+v, %v)", view, err)
	}
	if fixture.state.state.Phase != InstallPhaseCompleted || !fixture.auth.sealed {
		t.Fatalf("reconciliation did not finalize state/auth: state=%+v sealed=%t", fixture.state.state, fixture.auth.sealed)
	}
}

func TestSetupReconcileCommitUsesProvidedSnapshotAndFinalizesMatchingBinding(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.failCheckpoint = "after_runtime_write"
	if _, err := fixture.service.Apply(t.Context(), fixture.request()); !errors.Is(err, ErrSetupCommit) {
		t.Fatalf("Apply error=%v", err)
	}
	snapshot := cloneBootstrap(fixture.bootstrap)
	state := fixture.state.state
	fixture.failLoadAfterWrite = true

	view, err := fixture.newService().ReconcileCommit(t.Context(), snapshot, state)
	if err != nil || view.Phase != OperationPhaseComplete {
		t.Fatalf("ReconcileCommit=(%+v, %v)", view, err)
	}
	if fixture.state.state.Phase != InstallPhaseCompleted {
		t.Fatalf("install state phase=%q, want completed", fixture.state.state.Phase)
	}
}

func TestSetupReconcileCommitFailsClosedForBindingAndDatabaseFailures(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		mutate func(*serviceFixture, *Service)
	}{
		{name: "missing binding", mutate: func(fixture *serviceFixture, _ *Service) { fixture.store.binding = SetupBinding{} }},
		{name: "mismatched binding", mutate: func(fixture *serviceFixture, _ *Service) {
			fixture.store.binding.RequestDigest = strings.Repeat("f", 64)
		}},
		{name: "database unavailable", mutate: func(_ *serviceFixture, service *Service) {
			service.dependencies.openStore = func(context.Context, string) (SetupStoreSession, error) {
				return nil, errors.New("database unavailable")
			}
		}},
		{name: "context cancelled", mutate: func(_ *serviceFixture, service *Service) {
			service.dependencies.openStore = func(ctx context.Context, _ string) (SetupStoreSession, error) { return nil, ctx.Err() }
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newServiceFixture(t)
			fixture.failCheckpoint = "after_runtime_write"
			if _, err := fixture.service.Apply(t.Context(), fixture.request()); !errors.Is(err, ErrSetupCommit) {
				t.Fatalf("Apply error=%v", err)
			}
			service := fixture.newService()
			testCase.mutate(fixture, service)
			ctx := t.Context()
			if testCase.name == "context cancelled" {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			if _, err := service.ReconcileCommit(ctx, cloneBootstrap(fixture.bootstrap), fixture.state.state); !errors.Is(err, ErrSetupReconciliation) {
				t.Fatalf("ReconcileCommit error=%v, want ErrSetupReconciliation", err)
			}
			if fixture.state.state.Phase != InstallPhaseCommitting {
				t.Fatalf("failed reconciliation changed phase to %q", fixture.state.state.Phase)
			}
		})
	}
}

func TestSetupVerifyCompletedBindingRejectsMissingOrMismatchedAdministratorBinding(t *testing.T) {
	fixture := newServiceFixture(t)
	if _, err := fixture.service.Apply(t.Context(), fixture.request()); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	service := fixture.newService()
	if err := service.VerifyCompletedBinding(t.Context(), cloneBootstrap(fixture.bootstrap), fixture.state.state); err != nil {
		t.Fatalf("VerifyCompletedBinding: %v", err)
	}
	fixture.store.binding = SetupBinding{}
	if err := service.VerifyCompletedBinding(t.Context(), cloneBootstrap(fixture.bootstrap), fixture.state.state); !errors.Is(err, ErrSetupReconciliation) {
		t.Fatalf("missing binding error=%v", err)
	}
	fixture.store.binding = SetupBinding{
		OperationID: fixture.operationID, InstallationID: fixture.bootstrap.InstallationID,
		ConfigRevision: fixture.bootstrap.ConfigRevision, RequestDigest: strings.Repeat("f", 64),
		AdminID: 99, AdminEmail: "other@example.test",
	}
	if err := service.VerifyCompletedBinding(t.Context(), cloneBootstrap(fixture.bootstrap), fixture.state.state); !errors.Is(err, ErrSetupReconciliation) {
		t.Fatalf("mismatched admin binding error=%v", err)
	}
}

func TestSetupProgressDoesNotReportResumeUntilDatabaseBindingCanBeVerified(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.failCheckpoint = "before_runtime_write"
	if _, err := fixture.service.Apply(t.Context(), fixture.request()); !errors.Is(err, ErrSetupCommit) {
		t.Fatalf("Apply error=%v", err)
	}
	fixture.bootstrap.Values["DATABASE_URL"] = ""
	restarted := fixture.newService()
	if _, err := restarted.Progress(t.Context(), fixture.operationID); !errors.Is(err, ErrSetupReconciliation) {
		t.Fatalf("unverified resume Progress error=%v", err)
	}
}

func TestSetupOperationViewsAndErrorsNeverExposeSubmittedSecrets(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.prober.failKind = "database"
	view, err := fixture.service.Apply(t.Context(), fixture.request())
	if err == nil {
		t.Fatal("Apply unexpectedly succeeded")
	}
	assertDoesNotContainSetupSecrets(t, fmt.Sprintf("%+v %v", view, err), fixture)
	progress, progressErr := fixture.service.Progress(t.Context(), fixture.operationID)
	assertDoesNotContainSetupSecrets(t, fmt.Sprintf("%+v %v", progress, progressErr), fixture)
}

type serviceFixture struct {
	t                     *testing.T
	operationID           string
	adminPassword         string
	databaseSecret        string
	bootstrap             config.BootstrapConfig
	state                 *fakeApplyStateStore
	store                 *fakeSetupStore
	prober                *fakeApplyProber
	auth                  *fakeCompletionAuth
	events                *eventLog
	service               *Service
	writtenValues         map[string]string
	failCheckpoint        string
	blockMigration        chan struct{}
	migrateCalls          atomic.Int32
	writeCalls            atomic.Int32
	writeErrorAfterCommit bool
	panicWriter           bool
	failLoadAfterWrite    bool
	panicMigration        bool
	retainedRuntimeData   []byte
}

func newServiceFixture(t *testing.T) *serviceFixture {
	t.Helper()
	values := pendingRuntimeValues()
	bootstrap := bootstrapFromValues(values)
	fixture := &serviceFixture{
		t: t, operationID: uuid.NewString(), adminPassword: "correct horse battery staple",
		databaseSecret: "database-password", bootstrap: bootstrap,
		state: &fakeApplyStateStore{state: InstallState{
			SchemaVersion: CurrentInstallStateSchemaVersion, InstallationID: bootstrap.InstallationID,
			DeploymentRole: bootstrap.Deployment.Role, Phase: InstallPhasePending, UpdatedAt: time.Unix(1, 0).UTC(),
		}, exists: true},
		store: &fakeSetupStore{}, prober: &fakeApplyProber{}, auth: &fakeCompletionAuth{}, events: &eventLog{},
	}
	fixture.state.events = fixture.events
	fixture.prober.events = fixture.events
	fixture.auth.events = fixture.events
	fixture.store.events = fixture.events
	fixture.service = fixture.newService()
	return fixture
}

func (fixture *serviceFixture) newService() *Service {
	return newService(serviceDependencies{
		runtimeEnvPath: "runtime.env", state: fixture.state, prober: fixture.prober, auth: fixture.auth,
		loadBootstrap: func(string) (config.BootstrapConfig, error) {
			if fixture.failLoadAfterWrite && fixture.writeCalls.Load() > 0 {
				return config.BootstrapConfig{}, errors.New("injected runtime readback failure")
			}
			return cloneBootstrap(fixture.bootstrap), nil
		},
		migrate: func(ctx context.Context, _ string, request db.MigrationRequest) (db.MigrationResult, error) {
			fixture.events.add("migrate")
			fixture.migrateCalls.Add(1)
			if fixture.panicMigration {
				panic("injected migration panic")
			}
			if fixture.blockMigration != nil {
				select {
				case <-fixture.blockMigration:
				case <-ctx.Done():
					return db.MigrationResult{}, ctx.Err()
				}
			}
			current := db.SchemaVersion{
				InstallationID: request.InstallationID, AppVersion: request.AppVersion,
				ConfigVersion: request.ConfigVersion, DatabaseSchemaVersion: db.CurrentDatabaseSchemaVersion,
			}
			fixture.store.recordMigration(current)
			return db.MigrationResult{Current: current}, nil
		},
		openStore: func(context.Context, string) (SetupStoreSession, error) { return fixture.store, nil },
		hashPassword: func(password string) (string, error) {
			fixture.events.add("hash_password")
			return "bcrypt:$2a$12$redacted", nil
		},
		renderRuntime: func(_ config.RuntimeSchema, values map[string]string, _ []config.EnvEntry) ([]byte, error) {
			fixture.events.add("runtime:render")
			fixture.writtenValues = serviceCloneValues(values)
			return []byte("redacted-runtime"), nil
		},
		writeRuntime: func(_ string, data []byte) error {
			fixture.events.add("runtime:write")
			fixture.writeCalls.Add(1)
			fixture.retainedRuntimeData = data
			fixture.bootstrap = bootstrapFromValues(serviceCloneValues(fixture.writtenValues))
			if fixture.panicWriter {
				panic("injected writer panic")
			}
			if fixture.writeErrorAfterCommit {
				return errors.New("directory sync failed after rename")
			}
			return nil
		},
		lockApply: noOpApplyLocker,
		now:       func() time.Time { return time.Unix(100, 0).UTC() },
		checkpoint: func(name string) error {
			if fixture.failCheckpoint == name {
				return errors.New("injected secret-bearing failure: " + fixture.databaseSecret)
			}
			return nil
		},
		events: fixture.events.add,
	})
}

func (fixture *serviceFixture) request() ApplyRequest {
	runtime := map[string]string{
		"DATABASE_URL":          fixture.bootstrap.Values["DATABASE_URL"],
		"REDIS_URL":             fixture.bootstrap.Values["REDIS_URL"],
		"REDIS_KEY_PREFIX":      fixture.bootstrap.Values["REDIS_KEY_PREFIX"],
		"STORAGE_DRIVER":        fixture.bootstrap.Values["STORAGE_DRIVER"],
		"STORAGE_LOCAL_ROOT":    fixture.bootstrap.Values["STORAGE_LOCAL_ROOT"],
		"STORAGE_SHARED_VOLUME": fixture.bootstrap.Values["STORAGE_SHARED_VOLUME"],
	}
	if fixture.bootstrap.Values["STORAGE_DRIVER"] == "s3" {
		for _, key := range []string{
			"STORAGE_S3_ENDPOINT", "STORAGE_S3_REGION", "STORAGE_S3_BUCKET", "STORAGE_S3_ACCESS_KEY_ID",
			"STORAGE_S3_SECRET_ACCESS_KEY", "STORAGE_S3_FORCE_PATH_STYLE", "STORAGE_S3_PREFIX",
		} {
			runtime[key] = fixture.bootstrap.Values[key]
		}
	}
	return ApplyRequest{
		OperationID: fixture.operationID,
		Runtime:     runtime,
		AdminEmail:  "Root@Example.com", AdminPassword: fixture.adminPassword,
	}
}

func (fixture *serviceFixture) useManagedFullRuntime() {
	values := fixture.bootstrap.Values
	values["DEPLOYMENT_MODE"] = "docker"
	values["DEPLOYMENT_PROFILE"] = "full"
	values["POSTGRES_MANAGED"] = "true"
	values["REDIS_MANAGED"] = "true"
	values["OBJECT_STORAGE_MANAGED"] = "true"
	values["POSTGRES_DATABASE"] = "app"
	values["POSTGRES_USER"] = "app"
	values["POSTGRES_PASSWORD"] = "managed-postgres-password"
	values["REDIS_PASSWORD"] = "managed-redis-password"
	values["MINIO_ROOT_USER"] = "minio-admin"
	values["MINIO_ROOT_PASSWORD"] = "managed-minio-password"
	values["STORAGE_DRIVER"] = "s3"
	values["STORAGE_S3_ENDPOINT"] = "http://minio:9000"
	values["STORAGE_S3_REGION"] = "us-east-1"
	values["STORAGE_S3_BUCKET"] = "app-assets"
	values["STORAGE_S3_ACCESS_KEY_ID"] = "app-access-key"
	values["STORAGE_S3_SECRET_ACCESS_KEY"] = "app-secret-key"
	values["STORAGE_S3_FORCE_PATH_STYLE"] = "true"
	values["STORAGE_S3_PREFIX"] = "production/assets"
	values["IMAGE_TAG"] = "v1.0.0"
	fixture.bootstrap = bootstrapFromValues(values)
}

func (fixture *serviceFixture) waitForEvent(t *testing.T, event string) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		if fixture.events.contains(event) {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %q; events=%v", event, fixture.events.snapshot())
		case <-time.After(time.Millisecond):
		}
	}
}

type eventLog struct {
	mu     sync.Mutex
	events []string
}

func (log *eventLog) add(event string) {
	log.mu.Lock()
	defer log.mu.Unlock()
	log.events = append(log.events, event)
}

func (log *eventLog) snapshot() []string {
	log.mu.Lock()
	defer log.mu.Unlock()
	return append([]string(nil), log.events...)
}

func (log *eventLog) contains(event string) bool {
	for _, current := range log.snapshot() {
		if current == event {
			return true
		}
	}
	return false
}

type fakeApplyStateStore struct {
	mu       sync.Mutex
	state    InstallState
	exists   bool
	events   *eventLog
	clearErr error
}

func (store *fakeApplyStateStore) BeginAttempt(attempt SetupAttempt, at time.Time) (InstallState, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.events != nil {
		store.events.add("state:attempt")
	}
	if store.state.Attempt != nil && !setupAttemptsEqual(*store.state.Attempt, attempt) {
		return InstallState{}, ErrSetupOperationConflict
	}
	store.state.Attempt = &attempt
	store.state.UpdatedAt = at
	return store.state, nil
}

func (store *fakeApplyStateStore) ClearAttempt(attempt SetupAttempt, at time.Time) (InstallState, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.clearErr != nil {
		return InstallState{}, store.clearErr
	}
	if store.state.Attempt == nil || !setupAttemptsEqual(*store.state.Attempt, attempt) {
		return InstallState{}, ErrSetupBindingMismatch
	}
	store.state.Attempt = nil
	store.state.UpdatedAt = at
	return store.state, nil
}

func (store *fakeApplyStateStore) Load() (InstallState, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.state, store.exists, nil
}

func (store *fakeApplyStateStore) BeginCommit(proof CommitProof, at time.Time) (InstallState, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.events != nil {
		store.events.add("state:begin")
	}
	if store.state.Phase == InstallPhasePending {
		store.state.Phase, store.state.Commit, store.state.Attempt, store.state.UpdatedAt = InstallPhaseCommitting, &proof, nil, at
	} else if store.state.Commit == nil || !commitProofsEqual(*store.state.Commit, proof) {
		return InstallState{}, ErrInstallStateInvalid
	}
	return store.state, nil
}

func (store *fakeApplyStateStore) FinalizeCommit(proof CommitProof, at time.Time) (InstallState, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.events != nil {
		store.events.add("state:finalize")
	}
	if store.state.Commit == nil || !commitProofsEqual(*store.state.Commit, proof) {
		return InstallState{}, ErrInstallStateInvalid
	}
	store.state.Phase, store.state.EverCompleted, store.state.UpdatedAt = InstallPhaseCompleted, true, at
	return store.state, nil
}

type fakeApplyProber struct {
	failKind string
	events   *eventLog
	calls    atomic.Int32
}

func (prober *fakeApplyProber) result(kind string) ProbeResult {
	prober.calls.Add(1)
	if prober.events != nil {
		prober.events.add("probe:" + kind)
	}
	if kind == prober.failKind {
		return ProbeResult{Kind: kind, Code: ProbeCodeConnectionFailed}
	}
	return ProbeResult{Kind: kind, Success: true, Code: ProbeCodeOK}
}

func (prober *fakeApplyProber) ProbePostgres(context.Context, PostgresProbeRequest) ProbeResult {
	return prober.result("database")
}
func (prober *fakeApplyProber) ProbeRedis(context.Context, RedisProbeRequest) ProbeResult {
	return prober.result("redis")
}
func (prober *fakeApplyProber) ProbeStorage(context.Context, StorageProbeRequest) ProbeResult {
	return prober.result("storage")
}

type fakeSetupStore struct {
	mu              sync.Mutex
	events          *eventLog
	request         SetupInitializationRequest
	binding         SetupBinding
	initializeCalls int
	adminCreations  int
	closed          int
	initializeErr   error
	migration       *db.SchemaVersion
}

func (store *fakeSetupStore) MigrationCompleted(_ context.Context, expected db.SchemaVersion) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.migration == nil {
		return false, nil
	}
	if *store.migration != expected {
		return false, ErrSetupBindingMismatch
	}
	return true, nil
}

func (store *fakeSetupStore) recordMigration(current db.SchemaVersion) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.migration = &current
}

func (store *fakeSetupStore) Initialize(_ context.Context, request SetupInitializationRequest) (SetupBinding, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.events != nil {
		store.events.add("store:initialize")
	}
	store.initializeCalls++
	if store.initializeErr != nil {
		return SetupBinding{}, store.initializeErr
	}
	if store.binding != (SetupBinding{}) {
		if store.binding.OperationID != request.OperationID {
			return SetupBinding{}, ErrSetupOperationConflict
		}
		if store.binding.RequestDigest != request.RequestDigest {
			return SetupBinding{}, ErrSetupBindingMismatch
		}
		return store.binding, nil
	}
	if request.AdminPasswordHash == "" {
		return SetupBinding{}, ErrFirstAdminConflict
	}
	store.request = request
	store.adminCreations++
	store.binding = SetupBinding{
		OperationID: request.OperationID, InstallationID: request.InstallationID,
		ConfigRevision: request.ConfigRevision, RequestDigest: request.RequestDigest,
		AdminID: 1, AdminEmail: strings.ToLower(request.AdminEmail),
	}
	return store.binding, nil
}

func (store *fakeSetupStore) GetBinding(_ context.Context, installationID string) (SetupBinding, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.binding == (SetupBinding{}) {
		return SetupBinding{}, ErrSetupBindingNotFound
	}
	if store.binding.InstallationID != installationID {
		return SetupBinding{}, ErrSetupBindingMismatch
	}
	return store.binding, nil
}

func (store *fakeSetupStore) Close() error { store.closed++; return nil }

type fakeCompletionAuth struct {
	events     *eventLog
	prepared   bool
	sealed     bool
	abortCalls int
}

func (auth *fakeCompletionAuth) PrepareCompletion() (PreparedCompletion, error) {
	if auth.events != nil {
		auth.events.add("auth:prepare")
	}
	auth.prepared = true
	return PreparedCompletion{}, nil
}

func (auth *fakeCompletionAuth) CommitCompletion(PreparedCompletion) error {
	if auth.events != nil {
		auth.events.add("auth:commit")
	}
	auth.sealed = true
	return nil
}

func (auth *fakeCompletionAuth) AbortCompletion(PreparedCompletion) error {
	if auth.events != nil {
		auth.events.add("auth:abort")
	}
	auth.abortCalls++
	auth.prepared = false
	return nil
}

func (auth *fakeCompletionAuth) FailClosedCompletion() { auth.sealed = true }

func pendingRuntimeValues() map[string]string {
	return map[string]string{
		"RUNTIME_SCHEMA_VERSION": "1", "DEPLOYMENT_MODE": "native", "DEPLOYMENT_PROFILE": "core",
		"DEPLOYMENT_TOPOLOGY": "single", "DEPLOYMENT_ROLE": "single", "DEPLOYMENT_MODULES": "api,worker,user-web,admin-web",
		"POSTGRES_MANAGED": "false", "REDIS_MANAGED": "false", "OBJECT_STORAGE_MANAGED": "false",
		"SETUP_COMPLETED": "false", "SETUP_TOKEN": "setup-token-secret", "SETUP_TOKEN_VERSION": "7",
		"DATABASE_URL": "postgres://app:database-password@127.0.0.1:5432/app?sslmode=disable",
		"REDIS_URL":    "redis://:redis-password@127.0.0.1:6379/0", "REDIS_KEY_PREFIX": "gallery",
		"STORAGE_DRIVER": "local", "STORAGE_LOCAL_ROOT": "./data/storage", "STORAGE_SHARED_VOLUME": "true",
		"AUTH_ACCESS_TOKEN_SECRET":                 strings.Repeat("a", 64),
		"API_KEY_SIGNING_SECRET_ENCRYPTION_KEY":    strings.Repeat("b", 64),
		"CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY":   strings.Repeat("c", 64),
		"PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY": strings.Repeat("d", 64),
		"PROMPT_OPTIMIZATION_QUOTE_SIGNING_KEY":    strings.Repeat("e", 64),
		"CLUSTER_ENROLLMENT_SEAL_KEY":              "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"API_PORT":                                 "8080", "RELEASE_VERSION": "v1.0.0", "INSTALLATION_ID": uuid.NewString(),
		"CONFIG_REVISION": "1", "APPLICATION_VERSION": "v1.0.0",
	}
}

func bootstrapFromValues(values map[string]string) config.BootstrapConfig {
	completed := values["SETUP_COMPLETED"] == "true"
	postgresManaged := values["POSTGRES_MANAGED"] == "true"
	redisManaged := values["REDIS_MANAGED"] == "true"
	storageManaged := values["OBJECT_STORAGE_MANAGED"] == "true"
	revision, _ := strconv.Atoi(values["CONFIG_REVISION"])
	version, _ := strconv.ParseUint(values["SETUP_TOKEN_VERSION"], 10, 64)
	return config.BootstrapConfig{
		Path: "runtime.env", SchemaVersion: config.CurrentRuntimeSchemaVersion,
		Deployment: config.DeploymentContext{
			Mode: config.DeploymentMode(values["DEPLOYMENT_MODE"]), Profile: config.DeploymentProfile(values["DEPLOYMENT_PROFILE"]),
			Topology: config.DeploymentTopology(values["DEPLOYMENT_TOPOLOGY"]), Role: config.DeploymentRole(values["DEPLOYMENT_ROLE"]),
			StorageDriver: values["STORAGE_DRIVER"], SetupCompleted: completed,
		},
		SetupCompleted: completed, SetupToken: values["SETUP_TOKEN"], SetupTokenVersion: version,
		PostgresManaged: postgresManaged, RedisManaged: redisManaged, ObjectStorageManaged: storageManaged,
		InstallationID: values["INSTALLATION_ID"], ConfigRevision: revision,
		ApplicationVersion: values["APPLICATION_VERSION"], Values: serviceCloneValues(values),
	}
}

func cloneBootstrap(source config.BootstrapConfig) config.BootstrapConfig {
	clone := source
	clone.Values = serviceCloneValues(source.Values)
	clone.DeploymentModules = append([]string(nil), source.DeploymentModules...)
	return clone
}

func serviceCloneValues(source map[string]string) map[string]string {
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func assertDoesNotContainSetupSecrets(t *testing.T, value string, fixture *serviceFixture) {
	t.Helper()
	for _, secret := range []string{fixture.adminPassword, fixture.databaseSecret, "redis-password", strings.Repeat("d", 64), "setup-token-secret"} {
		if strings.Contains(value, secret) {
			t.Fatalf("value exposes secret %q: %s", secret, value)
		}
	}
}

func (service *Service) operationCountForTest() int {
	service.mu.Lock()
	defer service.mu.Unlock()
	return len(service.operations)
}
