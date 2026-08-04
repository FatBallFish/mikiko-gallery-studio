package setup

import (
	"context"
	"crypto/hmac"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
)

type OperationPhase string

const (
	OperationPhasePending              OperationPhase = "pending"
	OperationPhaseValidating           OperationPhase = "validating"
	OperationPhaseInitializingDatabase OperationPhase = "initializing_database"
	OperationPhaseCreatingAdmin        OperationPhase = "creating_admin"
	OperationPhaseCommittingConfig     OperationPhase = "committing_config"
	OperationPhaseRestartPending       OperationPhase = "restart_pending"
	OperationPhaseComplete             OperationPhase = "complete"
	maxTerminalOperations                             = 128
)

var (
	ErrSetupValidation     = errors.New("setup request validation failed")
	ErrSetupProbe          = errors.New("setup middleware verification failed")
	ErrSetupMigration      = errors.New("setup database initialization failed")
	ErrSetupCommit         = errors.New("setup configuration commit failed")
	ErrSetupReconciliation = errors.New("setup state reconciliation failed")
	ErrSetupOperationGone  = errors.New("setup operation was not found")
)

type ApplyRequest struct {
	OperationID   string            `json:"operation_id"`
	Runtime       map[string]string `json:"runtime"`
	AdminEmail    string            `json:"admin_email"`
	AdminPassword string            `json:"admin_password"`
}

type OperationView struct {
	OperationID string         `json:"operation_id"`
	Phase       OperationPhase `json:"phase"`
	ErrorCode   string         `json:"error_code,omitempty"`
	StartedAt   time.Time      `json:"started_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

func (service *Service) RecoveryOperationID() (string, error) {
	if service == nil || service.dependencies.state == nil {
		return "", ErrInstallStateInvalid
	}
	state, exists, err := service.dependencies.state.Load()
	if err != nil || !exists || state.Validate() != nil {
		return "", ErrInstallStateInvalid
	}
	switch state.Phase {
	case InstallPhasePending:
		if state.Attempt != nil {
			return state.Attempt.OperationID, nil
		}
	case InstallPhaseCommitting:
		if state.Commit != nil {
			return state.Commit.OperationID, nil
		}
	case InstallPhaseCompleted:
		return "", nil
	}
	return "", nil
}

type ServiceOptions struct {
	RuntimeEnvPath string
	StateStore     *StateStore
	ProbeService   *ProbeService
	AuthService    *AuthService
	StoreOpener    SetupStoreOpener
}

type applyStateStore interface {
	Load() (InstallState, bool, error)
	BeginAttempt(SetupAttempt, time.Time) (InstallState, error)
	ClearAttempt(SetupAttempt, time.Time) (InstallState, error)
	BeginCommit(CommitProof, time.Time) (InstallState, error)
	FinalizeCommit(CommitProof, time.Time) (InstallState, error)
}

type applyProber interface {
	ProbePostgres(context.Context, PostgresProbeRequest) ProbeResult
	ProbeRedis(context.Context, RedisProbeRequest) ProbeResult
	ProbeStorage(context.Context, StorageProbeRequest) ProbeResult
}

type completionAuth interface {
	PrepareCompletion() (PreparedCompletion, error)
	CommitCompletion(PreparedCompletion) error
	AbortCompletion(PreparedCompletion) error
	FailClosedCompletion()
}

type runtimeBootstrapLoader func(string) (config.BootstrapConfig, error)
type databaseMigrator func(context.Context, string, db.MigrationRequest) (db.MigrationResult, error)
type runtimeRenderer func(config.RuntimeSchema, map[string]string, []config.EnvEntry) ([]byte, error)
type runtimeWriter func(string, []byte) error
type passwordHasher func(string) (string, error)
type applyUnlock func() error
type applyLocker func(context.Context) (applyUnlock, error)

type serviceDependencies struct {
	runtimeEnvPath string
	state          applyStateStore
	prober         applyProber
	auth           completionAuth
	loadBootstrap  runtimeBootstrapLoader
	migrate        databaseMigrator
	openStore      SetupStoreOpener
	hashPassword   passwordHasher
	renderRuntime  runtimeRenderer
	writeRuntime   runtimeWriter
	now            func() time.Time
	checkpoint     func(string) error
	events         func(string)
	lockApply      applyLocker
}

type Service struct {
	dependencies serviceDependencies

	mu                 sync.Mutex
	activeID           string
	operations         map[string]*setupOperation
	terminalOperations []*setupOperation
	fingerprintKey     [sha256.Size]byte
}

type setupOperation struct {
	view                OperationView
	fingerprint         [sha256.Size]byte
	passwordFingerprint [sha256.Size]byte
	done                chan struct{}
	err                 error
}

type immutableApplyRequest struct {
	operationID         string
	runtime             map[string]string
	adminEmail          string
	adminPassword       string
	fingerprint         [sha256.Size]byte
	passwordFingerprint [sha256.Size]byte
}

type preparedApply struct {
	bootstrap  config.BootstrapConfig
	state      InstallState
	values     map[string]string
	proof      CommitProof
	attempt    SetupAttempt
	digest     string
	adminEmail string
	completed  bool
}

func NewService(options ServiceOptions) (*Service, error) {
	return newServiceWithEntropy(options, cryptorand.Reader)
}

func newServiceWithEntropy(options ServiceOptions, entropy io.Reader) (*Service, error) {
	if strings.TrimSpace(options.RuntimeEnvPath) == "" || options.StateStore == nil || options.ProbeService == nil || options.AuthService == nil || options.StoreOpener == nil {
		return nil, fmt.Errorf("setup service dependencies are incomplete")
	}
	if entropy == nil {
		return nil, fmt.Errorf("initialize setup request fingerprint key: entropy source is required")
	}
	var fingerprintKey [sha256.Size]byte
	if _, err := io.ReadFull(entropy, fingerprintKey[:]); err != nil {
		return nil, fmt.Errorf("initialize setup request fingerprint key: %w", err)
	}
	return newServiceWithFingerprintKey(serviceDependencies{
		runtimeEnvPath: options.RuntimeEnvPath,
		state:          options.StateStore,
		prober:         options.ProbeService,
		auth:           options.AuthService,
		loadBootstrap:  config.LoadBootstrap,
		migrate:        db.Migrate,
		openStore:      options.StoreOpener,
		hashPassword:   adminauthservice.HashPasswordChecked,
		renderRuntime:  config.RenderRuntimeEnv,
		writeRuntime:   config.WriteRuntimeEnvAtomic,
		now:            func() time.Time { return time.Now().UTC() },
	}, fingerprintKey), nil
}

func newService(dependencies serviceDependencies) *Service {
	testKey := sha256.Sum256([]byte("pic-gallery/setup/test-request-fingerprint-key"))
	return newServiceWithFingerprintKey(dependencies, testKey)
}

func newServiceWithFingerprintKey(dependencies serviceDependencies, fingerprintKey [sha256.Size]byte) *Service {
	if dependencies.checkpoint == nil {
		dependencies.checkpoint = func(string) error { return nil }
	}
	if dependencies.events == nil {
		dependencies.events = func(string) {}
	}
	if dependencies.lockApply == nil {
		dependencies.lockApply = newRuntimeApplyLocker(dependencies.runtimeEnvPath)
	}
	return &Service{dependencies: dependencies, operations: make(map[string]*setupOperation), fingerprintKey: fingerprintKey}
}

func (service *Service) Apply(ctx context.Context, request ApplyRequest) (view OperationView, returnErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	immutable, err := service.snapshotApplyRequest(request)
	if err != nil {
		return OperationView{}, ErrSetupValidation
	}
	operation, leader, err := service.acquireOperation(immutable)
	if err != nil {
		return OperationView{}, err
	}
	if !leader {
		select {
		case <-operation.done:
			return service.operationResult(operation)
		case <-ctx.Done():
			return service.operationSnapshot(operation), ctx.Err()
		}
	}

	unlock, lockErr := service.dependencies.lockApply(ctx)
	if lockErr != nil {
		view, returnErr = service.failOperation(operation, stableLockError(lockErr))
		service.finishOperation(operation, view, returnErr)
		return view, returnErr
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			view, returnErr = service.failOperation(operation, ErrSetupCommit)
		}
		if releaseErr := unlock(); releaseErr != nil && returnErr == nil {
			service.dependencies.auth.FailClosedCompletion()
			view, returnErr = service.failOperation(operation, ErrSetupCommit)
		}
		service.finishOperation(operation, view, returnErr)
	}()
	if err := ctx.Err(); err != nil {
		return service.failOperation(operation, err)
	}
	return service.executeApply(ctx, operation, immutable)
}

func stableLockError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return ErrSetupCommit
}

func (service *Service) Progress(ctx context.Context, id string) (OperationView, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !canonicalUUID(id) {
		return OperationView{}, ErrSetupValidation
	}
	service.mu.Lock()
	operation := service.operations[id]
	service.mu.Unlock()
	if operation != nil {
		return service.operationResult(operation)
	}
	return service.progressAfterRestart(ctx, id)
}

// ReconcileCommit verifies the durable database binding against the exact
// startup snapshot before finalizing a runtime-env rename crash window.
func (service *Service) ReconcileCommit(ctx context.Context, bootstrap config.BootstrapConfig, state InstallState) (OperationView, error) {
	if err := service.validateDependencies(); err != nil || state.Commit == nil {
		return OperationView{}, ErrSetupReconciliation
	}
	decision, err := ResolveStartupDecision(bootstrap, state, true)
	if err != nil || decision.Reconciliation != ReconciliationRequireDatabase {
		return OperationView{}, ErrSetupReconciliation
	}
	prepared, err := service.preparedFromCompletedBootstrap(ctx, bootstrap, state)
	if err != nil || service.verifyBinding(ctx, prepared) != nil {
		return OperationView{}, ErrSetupReconciliation
	}
	now := service.now()
	if _, err := service.dependencies.state.FinalizeCommit(prepared.proof, now); err != nil {
		service.dependencies.auth.FailClosedCompletion()
		return OperationView{}, ErrSetupReconciliation
	}
	service.dependencies.auth.FailClosedCompletion()
	return OperationView{
		OperationID: prepared.proof.OperationID, Phase: OperationPhaseComplete,
		StartedAt: state.UpdatedAt, UpdatedAt: now,
	}, nil
}

// VerifyCompletedBinding prevents a completed runtime/install-state pair from
// entering normal mode without its transactionally created administrator and
// installation binding.
func (service *Service) VerifyCompletedBinding(ctx context.Context, bootstrap config.BootstrapConfig, state InstallState) error {
	if err := service.validateDependencies(); err != nil {
		return ErrSetupReconciliation
	}
	decision, err := ResolveStartupDecision(bootstrap, state, true)
	if err != nil || decision.Mode != StartupModeNormal || decision.Reconciliation != ReconciliationNone {
		return ErrSetupReconciliation
	}
	prepared, err := service.preparedFromCompletedBootstrap(ctx, bootstrap, state)
	if err != nil || service.verifyBinding(ctx, prepared) != nil {
		return ErrSetupReconciliation
	}
	return nil
}

func (service *Service) acquireOperation(request immutableApplyRequest) (*setupOperation, bool, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.activeID != "" {
		operation := service.operations[service.activeID]
		if service.activeID != request.operationID {
			return nil, false, ErrSetupOperationConflict
		}
		if operation.fingerprint != request.fingerprint || differentNonEmptyPassword(operation.passwordFingerprint, request.passwordFingerprint) {
			return nil, false, ErrSetupBindingMismatch
		}
		return operation, false, nil
	}
	if existing := service.operations[request.operationID]; existing != nil {
		select {
		case <-existing.done:
			if existing.err == nil {
				if existing.fingerprint != request.fingerprint || differentNonEmptyPassword(existing.passwordFingerprint, request.passwordFingerprint) {
					return nil, false, ErrSetupBindingMismatch
				}
				return existing, false, nil
			}
			delete(service.operations, request.operationID)
		default:
			return existing, false, nil
		}
	}
	now := service.now()
	operation := &setupOperation{
		view:        OperationView{OperationID: request.operationID, Phase: OperationPhasePending, StartedAt: now, UpdatedAt: now},
		fingerprint: request.fingerprint, passwordFingerprint: request.passwordFingerprint, done: make(chan struct{}),
	}
	service.operations[request.operationID] = operation
	service.activeID = request.operationID
	service.dependencies.events("phase:" + string(OperationPhasePending))
	return operation, true, nil
}

func (service *Service) finishOperation(operation *setupOperation, view OperationView, err error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	operation.view = view
	operation.err = err
	clear(operation.passwordFingerprint[:])
	if service.activeID == view.OperationID {
		service.activeID = ""
	}
	service.terminalOperations = append(service.terminalOperations, operation)
	for len(service.terminalOperations) > maxTerminalOperations {
		oldest := service.terminalOperations[0]
		service.terminalOperations = service.terminalOperations[1:]
		if service.operations[oldest.view.OperationID] == oldest && service.activeID != oldest.view.OperationID {
			delete(service.operations, oldest.view.OperationID)
		}
	}
	close(operation.done)
}

func (service *Service) operationResult(operation *setupOperation) (OperationView, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	return operation.view, operation.err
}

func (service *Service) operationSnapshot(operation *setupOperation) OperationView {
	service.mu.Lock()
	defer service.mu.Unlock()
	return operation.view
}

func (service *Service) executeApply(ctx context.Context, operation *setupOperation, request immutableApplyRequest) (OperationView, error) {
	service.setPhase(operation, OperationPhaseValidating)
	prepared, err := service.prepareApply(request)
	if err != nil {
		return service.failOperation(operation, stableSetupError(err, ErrSetupValidation))
	}
	if err := service.dependencies.checkpoint("after_validation"); err != nil {
		return service.failOperation(operation, ErrSetupValidation)
	}
	if prepared.completed {
		service.dependencies.auth.FailClosedCompletion()
		if err := service.verifyBinding(ctx, prepared); err != nil {
			return service.failOperation(operation, stableSetupError(err, ErrSetupReconciliation))
		}
		service.setPhase(operation, OperationPhaseComplete)
		return service.operationSnapshot(operation), nil
	}

	if prepared.state.Phase == InstallPhaseCommitting {
		return service.resumeCommit(ctx, operation, prepared)
	}
	hadAttempt := prepared.state.Attempt != nil
	if !hadAttempt && request.adminPassword == "" {
		return service.failOperation(operation, ErrSetupValidation)
	}
	reserved, err := service.dependencies.state.BeginAttempt(prepared.attempt, service.now())
	if err != nil {
		return service.failOperation(operation, stableSetupError(err, ErrSetupCommit))
	}
	prepared.state = reserved
	databaseBound := false
	migrationCompleted := false
	if hadAttempt {
		binding, found, lookupErr := service.lookupBinding(ctx, prepared)
		if lookupErr != nil {
			return service.failOperation(operation, stableSetupError(lookupErr, ErrSetupReconciliation))
		}
		if found {
			if err := validateBinding(binding, prepared); err != nil {
				return service.failOperation(operation, stableSetupError(err, ErrSetupReconciliation))
			}
			databaseBound = true
		} else {
			if request.adminPassword == "" {
				return service.failOperation(operation, ErrSetupValidation)
			}
			migrationCompleted, lookupErr = service.lookupMigration(ctx, prepared)
			if lookupErr != nil {
				return service.failOperation(operation, stableSetupError(lookupErr, ErrSetupReconciliation))
			}
		}
	}
	if err := service.runFinalProbes(ctx, prepared.values); err != nil {
		if !databaseBound {
			if _, clearErr := service.dependencies.state.ClearAttempt(prepared.attempt, service.now()); clearErr != nil {
				return service.failOperation(operation, stableSetupError(clearErr, ErrSetupCommit))
			}
		}
		return service.failOperation(operation, stableSetupError(err, ErrSetupProbe))
	}
	if databaseBound {
		return service.commitPreparedRuntime(operation, prepared)
	}

	service.setPhase(operation, OperationPhaseInitializingDatabase)
	if !migrationCompleted {
		if err := service.dependencies.checkpoint("before_migration"); err != nil {
			if _, clearErr := service.dependencies.state.ClearAttempt(prepared.attempt, service.now()); clearErr != nil {
				return service.failOperation(operation, stableSetupError(clearErr, ErrSetupCommit))
			}
			return service.failOperation(operation, ErrSetupMigration)
		}
		if _, err := service.dependencies.migrate(ctx, prepared.values["DATABASE_URL"], db.MigrationRequest{
			InstallationID: prepared.bootstrap.InstallationID,
			AppVersion:     prepared.values["APPLICATION_VERSION"],
			ConfigVersion:  config.CurrentRuntimeSchemaVersion,
		}); err != nil {
			return service.failOperation(operation, stableSetupError(err, ErrSetupMigration))
		}
		if err := service.dependencies.checkpoint("after_migration"); err != nil {
			return service.failOperation(operation, ErrSetupMigration)
		}
	}

	service.setPhase(operation, OperationPhaseCreatingAdmin)
	passwordHash := ""
	if request.adminPassword != "" {
		passwordHash, err = service.dependencies.hashPassword(request.adminPassword)
		if err != nil {
			return service.failOperation(operation, ErrSetupCommit)
		}
	}
	store, err := service.dependencies.openStore(ctx, prepared.values["DATABASE_URL"])
	if err != nil {
		return service.failOperation(operation, ErrSetupCommit)
	}
	_, initializeErr := store.Initialize(ctx, SetupInitializationRequest{
		OperationID: request.operationID, InstallationID: prepared.bootstrap.InstallationID,
		ConfigRevision: prepared.proof.ConfigRevision, RequestDigest: prepared.digest,
		AdminEmail: request.adminEmail, AdminPasswordHash: passwordHash,
	})
	closeErr := store.Close()
	passwordHash = ""
	if initializeErr != nil {
		return service.failOperation(operation, stableSetupError(initializeErr, ErrSetupCommit))
	}
	if closeErr != nil {
		return service.failOperation(operation, ErrSetupCommit)
	}
	if err := service.dependencies.checkpoint("after_database_binding"); err != nil {
		return service.failOperation(operation, ErrSetupCommit)
	}

	return service.commitPreparedRuntime(operation, prepared)
}

func (service *Service) resumeCommit(ctx context.Context, operation *setupOperation, prepared preparedApply) (OperationView, error) {
	if prepared.state.Commit == nil || !commitProofsEqual(*prepared.state.Commit, prepared.proof) {
		return service.failOperation(operation, ErrSetupReconciliation)
	}
	if err := service.verifyBinding(ctx, prepared); err != nil {
		return service.failOperation(operation, ErrSetupReconciliation)
	}
	if prepared.bootstrap.SetupCompleted {
		if _, err := service.dependencies.state.FinalizeCommit(prepared.proof, service.now()); err != nil {
			service.dependencies.auth.FailClosedCompletion()
			return service.failOperation(operation, ErrSetupReconciliation)
		}
		service.dependencies.auth.FailClosedCompletion()
		service.setPhase(operation, OperationPhaseComplete)
		return service.operationSnapshot(operation), nil
	}
	return service.commitPreparedRuntime(operation, prepared)
}

func (service *Service) commitPreparedRuntime(operation *setupOperation, prepared preparedApply) (view OperationView, returnErr error) {
	service.setPhase(operation, OperationPhaseCommittingConfig)
	completion, err := service.dependencies.auth.PrepareCompletion()
	if err != nil {
		return service.failOperation(operation, ErrSetupCommit)
	}
	writerStarted := false
	completionClosed := false
	defer func() {
		if completionClosed {
			return
		}
		if writerStarted {
			service.dependencies.auth.FailClosedCompletion()
			return
		}
		_ = service.dependencies.auth.AbortCompletion(completion)
	}()
	if err := service.dependencies.checkpoint("before_state_begin"); err != nil {
		return service.failOperation(operation, ErrSetupCommit)
	}
	if _, err := service.dependencies.state.BeginCommit(prepared.proof, service.now()); err != nil {
		return service.failOperation(operation, stableSetupError(err, ErrSetupCommit))
	}
	data, err := service.dependencies.renderRuntime(config.DefaultRuntimeSchema(), prepared.values, nil)
	if err != nil {
		return service.failOperation(operation, ErrSetupCommit)
	}
	defer clear(data)
	if err := service.dependencies.checkpoint("before_runtime_write"); err != nil {
		return service.failOperation(operation, ErrSetupCommit)
	}
	writerStarted = true
	if err := service.dependencies.writeRuntime(service.dependencies.runtimeEnvPath, data); err != nil {
		return service.failOperation(operation, ErrSetupCommit)
	}
	if err := service.dependencies.checkpoint("after_runtime_write"); err != nil {
		return service.failOperation(operation, ErrSetupCommit)
	}
	if _, err := service.dependencies.state.FinalizeCommit(prepared.proof, service.now()); err != nil {
		return service.failOperation(operation, ErrSetupCommit)
	}
	if err := service.dependencies.checkpoint("after_state_finalize"); err != nil {
		return service.failOperation(operation, ErrSetupCommit)
	}
	if err := service.dependencies.auth.CommitCompletion(completion); err != nil {
		return service.failOperation(operation, ErrSetupCommit)
	}
	completionClosed = true
	if err := service.dependencies.checkpoint("after_auth_commit"); err != nil {
		return service.failOperation(operation, ErrSetupCommit)
	}
	service.setPhase(operation, OperationPhaseRestartPending)
	return service.operationSnapshot(operation), nil
}

func (service *Service) prepareApply(request immutableApplyRequest) (preparedApply, error) {
	if err := service.validateDependencies(); err != nil {
		return preparedApply{}, err
	}
	bootstrap, err := service.dependencies.loadBootstrap(service.dependencies.runtimeEnvPath)
	if err != nil {
		return preparedApply{}, err
	}
	state, exists, err := service.dependencies.state.Load()
	if err != nil || !exists || state.Validate() != nil {
		return preparedApply{}, ErrInstallStateInvalid
	}
	if bootstrap.InstallationID != state.InstallationID || bootstrap.Deployment.Role != state.DeploymentRole || !setupAuthority(state.DeploymentRole) {
		return preparedApply{}, ErrStartupStateInconsistent
	}
	values, revision, err := mergeFinalRuntime(bootstrap, request.runtime)
	if err != nil {
		return preparedApply{}, err
	}
	digest, err := setupRequestDigest(values, request.adminEmail)
	if err != nil {
		return preparedApply{}, err
	}
	proof := CommitProof{
		OperationID: request.operationID, InstallationID: bootstrap.InstallationID,
		RuntimeSchemaVersion: config.CurrentRuntimeSchemaVersion, ConfigRevision: revision, RequestDigest: digest,
	}
	if err := proof.Validate(); err != nil {
		return preparedApply{}, err
	}
	prepared := preparedApply{
		bootstrap: bootstrap, state: state, values: values, proof: proof,
		digest: digest, adminEmail: request.adminEmail,
	}
	switch state.Phase {
	case InstallPhasePending:
		if bootstrap.SetupCompleted {
			return preparedApply{}, ErrStartupStateInconsistent
		}
		if state.Attempt == nil {
			if request.adminPassword == "" {
				return preparedApply{}, ErrSetupValidation
			}
			verifier, err := setupAdminCredentialVerifier(values, request.adminPassword)
			if err != nil {
				return preparedApply{}, err
			}
			prepared.attempt = SetupAttempt{
				OperationID: request.operationID, ConfigRevision: revision,
				RequestDigest: digest, AdminCredentialVerifier: verifier,
			}
			if err := prepared.attempt.Validate(); err != nil {
				return preparedApply{}, err
			}
		} else {
			if state.Attempt.OperationID != request.operationID {
				return preparedApply{}, ErrSetupOperationConflict
			}
			if state.Attempt.ConfigRevision != revision || !constantTimeDigestEqual(state.Attempt.RequestDigest, digest) {
				return preparedApply{}, ErrSetupBindingMismatch
			}
			if request.adminPassword != "" {
				verifier, err := setupAdminCredentialVerifier(values, request.adminPassword)
				if err != nil || !constantTimeDigestEqual(state.Attempt.AdminCredentialVerifier, verifier) {
					return preparedApply{}, ErrSetupBindingMismatch
				}
			}
			prepared.attempt = *state.Attempt
		}
	case InstallPhaseCommitting:
		if state.Commit == nil || state.Commit.OperationID != request.operationID {
			return preparedApply{}, ErrSetupOperationConflict
		}
	case InstallPhaseCompleted:
		if state.Commit == nil || state.Commit.OperationID != request.operationID {
			return preparedApply{}, ErrSetupOperationConflict
		}
		if !commitProofsEqual(*state.Commit, proof) {
			service.dependencies.auth.FailClosedCompletion()
			return preparedApply{}, ErrSetupBindingMismatch
		}
		decision, decisionErr := ResolveStartupDecision(bootstrap, state, true)
		if decisionErr != nil || decision.Mode != StartupModeNormal {
			return preparedApply{}, ErrSetupReconciliation
		}
		prepared.completed = true
	default:
		return preparedApply{}, ErrStartupStateInconsistent
	}
	return prepared, nil
}

func (service *Service) runFinalProbes(ctx context.Context, values map[string]string) error {
	results := []ProbeResult{
		service.dependencies.prober.ProbePostgres(ctx, PostgresProbeRequest{DatabaseURL: values["DATABASE_URL"]}),
		service.dependencies.prober.ProbeRedis(ctx, RedisProbeRequest{RedisURL: values["REDIS_URL"], KeyPrefix: values["REDIS_KEY_PREFIX"]}),
		service.dependencies.prober.ProbeStorage(ctx, StorageProbeRequest{Config: storageConfigFromRuntime(values)}),
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, result := range results {
		if !result.Success || result.Code != ProbeCodeOK {
			return ErrSetupProbe
		}
	}
	return nil
}

func (service *Service) progressAfterRestart(ctx context.Context, operationID string) (OperationView, error) {
	if err := service.validateDependencies(); err != nil {
		return OperationView{}, ErrSetupReconciliation
	}
	bootstrap, err := service.dependencies.loadBootstrap(service.dependencies.runtimeEnvPath)
	if err != nil {
		return OperationView{}, ErrSetupReconciliation
	}
	state, exists, err := service.dependencies.state.Load()
	if err != nil || !exists || state.Validate() != nil || state.Commit == nil || state.Commit.OperationID != operationID {
		return OperationView{}, ErrSetupOperationGone
	}
	decision, err := ResolveStartupDecision(bootstrap, state, true)
	if err != nil {
		return OperationView{}, ErrSetupReconciliation
	}
	now := service.now()
	view := OperationView{OperationID: operationID, StartedAt: state.UpdatedAt, UpdatedAt: now}
	switch decision.Reconciliation {
	case ReconciliationResumeSetup:
		values, _, mergeErr := mergeFinalRuntime(bootstrap, nil)
		if mergeErr != nil {
			return OperationView{}, ErrSetupReconciliation
		}
		prepared := preparedApply{bootstrap: bootstrap, state: state, values: values, proof: *state.Commit}
		binding, found, bindingErr := service.lookupBinding(ctx, prepared)
		if bindingErr != nil || !found {
			return OperationView{}, ErrSetupReconciliation
		}
		prepared.adminEmail = binding.AdminEmail
		prepared.digest, err = setupRequestDigest(values, binding.AdminEmail)
		if err != nil || validateBinding(binding, prepared) != nil {
			return OperationView{}, ErrSetupReconciliation
		}
		view.Phase = OperationPhaseCommittingConfig
		return view, nil
	case ReconciliationRequireDatabase:
		prepared, err := service.preparedFromCompletedBootstrap(ctx, bootstrap, state)
		if err != nil || service.verifyBinding(ctx, prepared) != nil {
			return OperationView{}, ErrSetupReconciliation
		}
		if _, err := service.dependencies.state.FinalizeCommit(prepared.proof, now); err != nil {
			service.dependencies.auth.FailClosedCompletion()
			return OperationView{}, ErrSetupReconciliation
		}
		service.dependencies.auth.FailClosedCompletion()
		view.Phase = OperationPhaseComplete
		return view, nil
	case ReconciliationNone:
		if decision.Mode != StartupModeNormal {
			return OperationView{}, ErrSetupOperationGone
		}
		prepared, err := service.preparedFromCompletedBootstrap(ctx, bootstrap, state)
		if err != nil || service.verifyBinding(ctx, prepared) != nil {
			return OperationView{}, ErrSetupReconciliation
		}
		service.dependencies.auth.FailClosedCompletion()
		view.Phase = OperationPhaseComplete
		return view, nil
	default:
		return OperationView{}, ErrSetupReconciliation
	}
}

func (service *Service) preparedFromCompletedBootstrap(ctx context.Context, bootstrap config.BootstrapConfig, state InstallState) (preparedApply, error) {
	if state.Commit == nil || !bootstrap.SetupCompleted {
		return preparedApply{}, ErrSetupReconciliation
	}
	prepared := preparedApply{bootstrap: bootstrap, state: state, values: cloneRuntimeValues(bootstrap.Values), proof: *state.Commit}
	binding, found, err := service.lookupBinding(ctx, prepared)
	if err != nil || !found {
		return preparedApply{}, ErrSetupReconciliation
	}
	prepared.adminEmail = binding.AdminEmail
	prepared.digest, err = setupRequestDigest(bootstrap.Values, binding.AdminEmail)
	if err != nil || validateBinding(binding, prepared) != nil {
		return preparedApply{}, ErrSetupReconciliation
	}
	return prepared, nil
}

func (service *Service) verifyBinding(ctx context.Context, prepared preparedApply) error {
	binding, found, err := service.lookupBinding(ctx, prepared)
	if err != nil {
		return err
	}
	if !found {
		return ErrSetupBindingNotFound
	}
	return validateBinding(binding, prepared)
}

func (service *Service) lookupBinding(ctx context.Context, prepared preparedApply) (SetupBinding, bool, error) {
	store, err := service.dependencies.openStore(ctx, prepared.values["DATABASE_URL"])
	if err != nil {
		return SetupBinding{}, false, ErrSetupReconciliation
	}
	binding, lookupErr := store.GetBinding(ctx, prepared.bootstrap.InstallationID)
	closeErr := store.Close()
	if errors.Is(lookupErr, ErrSetupBindingNotFound) {
		if closeErr != nil {
			return SetupBinding{}, false, ErrSetupReconciliation
		}
		return SetupBinding{}, false, nil
	}
	if lookupErr != nil || closeErr != nil {
		return SetupBinding{}, false, stableSetupError(lookupErr, ErrSetupReconciliation)
	}
	return binding, true, nil
}

func (service *Service) lookupMigration(ctx context.Context, prepared preparedApply) (bool, error) {
	store, err := service.dependencies.openStore(ctx, prepared.values["DATABASE_URL"])
	if err != nil {
		return false, ErrSetupReconciliation
	}
	completed, lookupErr := store.MigrationCompleted(ctx, db.SchemaVersion{
		InstallationID: prepared.bootstrap.InstallationID,
		AppVersion:     prepared.values["APPLICATION_VERSION"], ConfigVersion: config.CurrentRuntimeSchemaVersion,
		DatabaseSchemaVersion: db.CurrentDatabaseSchemaVersion,
	})
	closeErr := store.Close()
	if lookupErr != nil || closeErr != nil {
		return false, stableSetupError(lookupErr, ErrSetupReconciliation)
	}
	return completed, nil
}

func validateBinding(binding SetupBinding, prepared preparedApply) error {
	if binding.OperationID != prepared.proof.OperationID {
		return ErrSetupOperationConflict
	}
	if !constantTimeDigestEqual(prepared.proof.RequestDigest, prepared.digest) || binding.InstallationID != prepared.proof.InstallationID ||
		binding.ConfigRevision != prepared.proof.ConfigRevision || !constantTimeDigestEqual(binding.RequestDigest, prepared.digest) {
		return ErrSetupBindingMismatch
	}
	if prepared.adminEmail != "" && binding.AdminEmail != prepared.adminEmail {
		return ErrFirstAdminConflict
	}
	return nil
}

func stableSetupError(err, fallback error) error {
	for _, stable := range []error{
		ErrSetupValidation, ErrSetupProbe, ErrSetupMigration, ErrSetupCommit, ErrSetupReconciliation,
		ErrSetupOperationConflict, ErrSetupBindingMismatch, ErrSetupBindingCorrupt, ErrFirstAdminConflict,
		context.Canceled, context.DeadlineExceeded,
	} {
		if errors.Is(err, stable) {
			return stable
		}
	}
	return fallback
}

func (service *Service) validateDependencies() error {
	dependencies := service.dependencies
	if strings.TrimSpace(dependencies.runtimeEnvPath) == "" || dependencies.state == nil || dependencies.prober == nil || dependencies.auth == nil ||
		dependencies.loadBootstrap == nil || dependencies.migrate == nil || dependencies.openStore == nil || dependencies.hashPassword == nil ||
		dependencies.renderRuntime == nil || dependencies.writeRuntime == nil || dependencies.now == nil || dependencies.lockApply == nil {
		return fmt.Errorf("setup service dependencies are incomplete")
	}
	return nil
}

func (service *Service) setPhase(operation *setupOperation, phase OperationPhase) {
	service.mu.Lock()
	operation.view.Phase = phase
	operation.view.UpdatedAt = service.now()
	service.mu.Unlock()
	service.dependencies.events("phase:" + string(phase))
}

func (service *Service) failOperation(operation *setupOperation, stable error) (OperationView, error) {
	service.mu.Lock()
	operation.view.ErrorCode = setupErrorCode(stable)
	operation.view.UpdatedAt = service.now()
	view := operation.view
	service.mu.Unlock()
	return view, stable
}

func (service *Service) now() time.Time {
	return service.dependencies.now().UTC()
}

func (service *Service) snapshotApplyRequest(request ApplyRequest) (immutableApplyRequest, error) {
	if !canonicalUUID(request.OperationID) {
		return immutableApplyRequest{}, ErrSetupValidation
	}
	email := strings.ToLower(strings.TrimSpace(request.AdminEmail))
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || len(email) > 255 {
		return immutableApplyRequest{}, ErrSetupValidation
	}
	if request.AdminPassword != "" && (len(strings.TrimSpace(request.AdminPassword)) < 6 || len([]byte(request.AdminPassword)) > 72) {
		return immutableApplyRequest{}, ErrSetupValidation
	}
	runtime := cloneRuntimeValues(request.Runtime)
	canonical := make([]config.EnvEntry, 0, len(runtime)+1)
	for key, value := range runtime {
		canonical = append(canonical, config.EnvEntry{Key: key, Value: value})
	}
	canonical = append(canonical, config.EnvEntry{Key: "ADMIN_EMAIL", Value: email})
	sort.Slice(canonical, func(i, j int) bool { return canonical[i].Key < canonical[j].Key })
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return immutableApplyRequest{}, ErrSetupValidation
	}
	fingerprint := service.requestFingerprint("setup-request-v1", encoded)
	clear(encoded)
	passwordFingerprint := [sha256.Size]byte{}
	if request.AdminPassword != "" {
		passwordBytes := []byte(request.AdminPassword)
		passwordFingerprint = service.requestFingerprint("setup-password-v1", passwordBytes)
		clear(passwordBytes)
	}
	return immutableApplyRequest{
		operationID: request.OperationID, runtime: runtime, adminEmail: email,
		adminPassword: request.AdminPassword, fingerprint: fingerprint, passwordFingerprint: passwordFingerprint,
	}, nil
}

func (service *Service) requestFingerprint(domain string, value []byte) [sha256.Size]byte {
	digest := hmac.New(sha256.New, service.fingerprintKey[:])
	_, _ = digest.Write([]byte(domain))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(value)
	var result [sha256.Size]byte
	copy(result[:], digest.Sum(nil))
	return result
}

func mergeFinalRuntime(bootstrap config.BootstrapConfig, submitted map[string]string) (map[string]string, int, error) {
	schema := config.DefaultRuntimeSchema()
	if err := schema.Validate(); err != nil || bootstrap.Values == nil {
		return nil, 0, ErrSetupValidation
	}
	fields := make(map[string]config.RuntimeField, len(schema.Fields))
	for _, field := range schema.Fields {
		fields[field.Key] = field
	}
	values := cloneRuntimeValues(bootstrap.Values)
	for _, field := range schema.Fields {
		if _, exists := values[field.Key]; !exists {
			values[field.Key] = field.DefaultValue
		}
	}
	for key, value := range submitted {
		field, exists := fields[key]
		if !exists {
			return nil, 0, ErrSetupValidation
		}
		switch field.Owner {
		case config.FieldOwnerSetup:
			if managedSetupFieldReadOnly(bootstrap, key) && value != bootstrap.Values[key] {
				return nil, 0, ErrSetupValidation
			}
			values[key] = value
		case config.FieldOwnerMGSCTL, config.FieldOwnerApplication:
			if current, exists := bootstrap.Values[key]; !exists || current != value {
				return nil, 0, ErrSetupValidation
			}
		default:
			return nil, 0, ErrSetupValidation
		}
	}
	revision := bootstrap.ConfigRevision
	if revision <= 0 {
		revision = 1
	}
	values["RUNTIME_SCHEMA_VERSION"] = strconv.Itoa(schema.Version)
	values["SETUP_COMPLETED"] = "true"
	values["SETUP_TOKEN"] = ""
	values["SETUP_TOKEN_VERSION"] = strconv.FormatUint(bootstrap.SetupTokenVersion, 10)
	values["CONFIG_REVISION"] = strconv.Itoa(revision)

	deployment := bootstrap.Deployment
	deployment.SetupCompleted = true
	deployment.StorageDriver = values["STORAGE_DRIVER"]
	if err := validateDeploymentManagement(bootstrap, deployment); err != nil {
		return nil, 0, ErrSetupValidation
	}
	required, err := config.RequiredRuntimeFields(schema, deployment)
	if err != nil {
		return nil, 0, ErrSetupValidation
	}
	for _, field := range required {
		if strings.TrimSpace(values[field.Key]) == "" {
			return nil, 0, ErrSetupValidation
		}
	}
	for _, field := range schema.Fields {
		value, exists := values[field.Key]
		if !exists || value == "" {
			continue
		}
		if err := field.Validate(value); err != nil {
			return nil, 0, ErrSetupValidation
		}
	}
	return values, revision, nil
}

func managedSetupFieldReadOnly(bootstrap config.BootstrapConfig, key string) bool {
	if bootstrap.PostgresManaged && key == "DATABASE_URL" {
		return true
	}
	if bootstrap.RedisManaged && key == "REDIS_URL" {
		return true
	}
	if !bootstrap.ObjectStorageManaged {
		return false
	}
	switch key {
	case "STORAGE_DRIVER", "STORAGE_S3_ENDPOINT", "STORAGE_S3_REGION", "STORAGE_S3_BUCKET",
		"STORAGE_S3_ACCESS_KEY_ID", "STORAGE_S3_SECRET_ACCESS_KEY", "STORAGE_S3_FORCE_PATH_STYLE", "STORAGE_S3_PREFIX":
		return true
	default:
		return false
	}
}

func validateDeploymentManagement(bootstrap config.BootstrapConfig, deployment config.DeploymentContext) error {
	managed := bootstrap.PostgresManaged || bootstrap.RedisManaged || bootstrap.ObjectStorageManaged
	if deployment.Mode == config.DeploymentModeNative {
		if deployment.Profile == config.DeploymentProfileFull || managed {
			return ErrSetupValidation
		}
	}
	switch deployment.Profile {
	case config.DeploymentProfileFull:
		if deployment.Mode != config.DeploymentModeDocker || deployment.Topology != config.DeploymentTopologySingle ||
			deployment.Role != config.DeploymentRoleSingle || !bootstrap.PostgresManaged || !bootstrap.RedisManaged ||
			!bootstrap.ObjectStorageManaged || deployment.StorageDriver != "s3" {
			return ErrSetupValidation
		}
	case config.DeploymentProfileCore:
		if managed {
			return ErrSetupValidation
		}
	}
	return nil
}

func setupRequestDigest(values map[string]string, adminEmail string) (string, error) {
	return setupRequestDigestWithReleaseFields(values, adminEmail, false)
}

func legacySetupRequestDigest(values map[string]string, adminEmail string) (string, error) {
	return setupRequestDigestWithReleaseFields(values, adminEmail, true)
}

func setupRequestDigestWithReleaseFields(values map[string]string, adminEmail string, includeReleaseFields bool) (string, error) {
	entries := make([]config.EnvEntry, 0, len(values))
	for name, value := range values {
		if !includeReleaseFields && setupReleaseField(name) {
			continue
		}
		entries = append(entries, config.EnvEntry{Key: name, Value: value})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	canonical, err := json.Marshal(struct {
		Runtime    []config.EnvEntry `json:"runtime"`
		AdminEmail string            `json:"admin_email"`
	}{Runtime: entries, AdminEmail: strings.ToLower(strings.TrimSpace(adminEmail))})
	if err != nil {
		return "", ErrSetupValidation
	}
	defer clear(canonical)
	return setupSecureHMAC(values, "setup-request-v1", canonical)
}

func setupReleaseField(name string) bool {
	switch name {
	case "APPLICATION_VERSION", "IMAGE_REGISTRY", "IMAGE_TAG", "RELEASE_VERSION":
		return true
	default:
		return false
	}
}

// CanonicalRequestDigest returns the setup commit digest for a completed
// runtime and normalized administrator identity.
func CanonicalRequestDigest(values map[string]string, adminEmail string) (string, error) {
	return setupRequestDigest(values, adminEmail)
}

func setupAdminCredentialVerifier(values map[string]string, password string) (string, error) {
	if password == "" {
		return "", ErrSetupValidation
	}
	passwordBytes := []byte(password)
	defer clear(passwordBytes)
	return setupSecureHMAC(values, "setup-admin-credential-v1", passwordBytes)
}

func setupSecureHMAC(values map[string]string, domain string, payload []byte) (string, error) {
	key := []byte(values["PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY"])
	if len(key) < 32 {
		clear(key)
		return "", ErrSetupValidation
	}
	defer clear(key)
	digest := hmac.New(sha256.New, key)
	_, _ = digest.Write([]byte(domain))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(payload)
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func differentNonEmptyPassword(left, right [sha256.Size]byte) bool {
	empty := [sha256.Size]byte{}
	return left != empty && right != empty && !hmac.Equal(left[:], right[:])
}

func storageConfigFromRuntime(values map[string]string) config.StorageConfig {
	shared, _ := strconv.ParseBool(values["STORAGE_SHARED_VOLUME"])
	forcePathStyle, _ := strconv.ParseBool(values["STORAGE_S3_FORCE_PATH_STYLE"])
	return config.StorageConfig{
		Driver: values["STORAGE_DRIVER"], LocalRoot: values["STORAGE_LOCAL_ROOT"],
		PublicBaseURL: values["STORAGE_PUBLIC_BASE_URL"], SharedVolume: shared,
		S3: config.StorageS3Config{
			Endpoint: values["STORAGE_S3_ENDPOINT"], Region: values["STORAGE_S3_REGION"], Bucket: values["STORAGE_S3_BUCKET"],
			AccessKeyID: values["STORAGE_S3_ACCESS_KEY_ID"], SecretAccessKey: values["STORAGE_S3_SECRET_ACCESS_KEY"],
			ForcePathStyle: forcePathStyle, Prefix: values["STORAGE_S3_PREFIX"],
		},
	}
}

func cloneRuntimeValues(source map[string]string) map[string]string {
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func canonicalUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == value
}

func setupErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrSetupValidation):
		return "VALIDATION_FAILED"
	case errors.Is(err, ErrSetupProbe):
		return "PROBE_FAILED"
	case errors.Is(err, ErrSetupMigration):
		return "DATABASE_INITIALIZATION_FAILED"
	case errors.Is(err, ErrSetupReconciliation):
		return "RECONCILIATION_FAILED"
	case errors.Is(err, ErrSetupOperationConflict):
		return "OPERATION_CONFLICT"
	case errors.Is(err, ErrSetupBindingMismatch):
		return "BINDING_MISMATCH"
	case errors.Is(err, ErrFirstAdminConflict):
		return "FIRST_ADMIN_CONFLICT"
	case errors.Is(err, context.Canceled):
		return "CANCELLED"
	case errors.Is(err, context.DeadlineExceeded):
		return "TIMEOUT"
	default:
		return "COMMIT_FAILED"
	}
}
