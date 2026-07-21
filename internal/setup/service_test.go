package setup

import (
	"context"
	"errors"
	"fmt"
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
		{name: "deployctl-owned field", mutate: func(_ *serviceFixture, request *ApplyRequest) { request.Runtime["INSTALLATION_ID"] = uuid.NewString() }},
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

func TestSetupApplyRetriesDatabaseSuccessWithoutPassword(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.failCheckpoint = "before_state_begin"
	if _, err := fixture.service.Apply(t.Context(), fixture.request()); !errors.Is(err, ErrSetupCommit) {
		t.Fatalf("first Apply error=%v", err)
	}
	if fixture.store.initializeCalls != 1 || fixture.writeCalls.Load() != 0 {
		t.Fatalf("first attempt store=%d write=%d", fixture.store.initializeCalls, fixture.writeCalls.Load())
	}

	fixture.failCheckpoint = ""
	retry := fixture.request()
	retry.AdminPassword = ""
	view, err := fixture.service.Apply(t.Context(), retry)
	if err != nil || view.Phase != OperationPhaseRestartPending {
		t.Fatalf("passwordless retry=(%+v, %v)", view, err)
	}
	if fixture.store.initializeCalls != 2 || fixture.store.adminCreations != 1 {
		t.Fatalf("retry store calls=%d admin creations=%d", fixture.store.initializeCalls, fixture.store.adminCreations)
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

func TestSetupApplyTreatsWriterErrorAfterRenameAsFailClosedCommitBoundary(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.writeErrorAfterCommit = true
	_, err := fixture.service.Apply(t.Context(), fixture.request())
	if !errors.Is(err, ErrSetupCommit) {
		t.Fatalf("Apply error=%v", err)
	}
	if !fixture.bootstrap.SetupCompleted || fixture.state.state.Phase != InstallPhaseCommitting || !fixture.auth.sealed {
		t.Fatalf("ambiguous writer boundary completed=%t phase=%q sealed=%t", fixture.bootstrap.SetupCompleted, fixture.state.state.Phase, fixture.auth.sealed)
	}
}

func TestSetupApplyPreservesDeployctlRuntimeExtensions(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.bootstrap.Values["DEPLOYCTL_EXTENSION"] = "retained"
	if _, err := fixture.service.Apply(t.Context(), fixture.request()); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if fixture.writtenValues["DEPLOYCTL_EXTENSION"] != "retained" {
		t.Fatalf("extension was lost: %#v", fixture.writtenValues)
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
		loadBootstrap: func(string) (config.BootstrapConfig, error) { return cloneBootstrap(fixture.bootstrap), nil },
		migrate: func(ctx context.Context, _ string, request db.MigrationRequest) (db.MigrationResult, error) {
			fixture.events.add("migrate")
			fixture.migrateCalls.Add(1)
			if fixture.blockMigration != nil {
				select {
				case <-fixture.blockMigration:
				case <-ctx.Done():
					return db.MigrationResult{}, ctx.Err()
				}
			}
			return db.MigrationResult{Current: db.SchemaVersion{InstallationID: request.InstallationID}}, nil
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
		writeRuntime: func(_ string, _ []byte) error {
			fixture.events.add("runtime:write")
			fixture.writeCalls.Add(1)
			fixture.bootstrap = bootstrapFromValues(serviceCloneValues(fixture.writtenValues))
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
	return ApplyRequest{
		OperationID: fixture.operationID,
		Runtime: map[string]string{
			"DATABASE_URL":          fixture.bootstrap.Values["DATABASE_URL"],
			"REDIS_URL":             fixture.bootstrap.Values["REDIS_URL"],
			"REDIS_KEY_PREFIX":      fixture.bootstrap.Values["REDIS_KEY_PREFIX"],
			"STORAGE_DRIVER":        fixture.bootstrap.Values["STORAGE_DRIVER"],
			"STORAGE_LOCAL_ROOT":    fixture.bootstrap.Values["STORAGE_LOCAL_ROOT"],
			"STORAGE_SHARED_VOLUME": fixture.bootstrap.Values["STORAGE_SHARED_VOLUME"],
		},
		AdminEmail: "Root@Example.com", AdminPassword: fixture.adminPassword,
	}
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
	mu     sync.Mutex
	state  InstallState
	exists bool
	events *eventLog
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
		store.state.Phase, store.state.Commit, store.state.UpdatedAt = InstallPhaseCommitting, &proof, at
	} else if store.state.Commit == nil || *store.state.Commit != proof {
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
	if store.state.Commit == nil || *store.state.Commit != proof {
		return InstallState{}, ErrInstallStateInvalid
	}
	store.state.Phase, store.state.EverCompleted, store.state.UpdatedAt = InstallPhaseCompleted, true, at
	return store.state, nil
}

type fakeApplyProber struct {
	failKind string
	events   *eventLog
}

func (prober *fakeApplyProber) result(kind string) ProbeResult {
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
}

func (store *fakeSetupStore) Initialize(_ context.Context, request SetupInitializationRequest) (SetupBinding, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.events != nil {
		store.events.add("store:initialize")
	}
	store.initializeCalls++
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
		"API_PORT":                                 "8080", "RELEASE_VERSION": "v1.0.0", "INSTALLATION_ID": uuid.NewString(),
		"CONFIG_REVISION": "1", "APPLICATION_VERSION": "v1.0.0",
	}
}

func bootstrapFromValues(values map[string]string) config.BootstrapConfig {
	completed := values["SETUP_COMPLETED"] == "true"
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
