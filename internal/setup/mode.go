package setup

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/config"
)

type StartupMode string

const (
	StartupModeSetup  StartupMode = "setup"
	StartupModeNormal StartupMode = "normal"
	StartupModeBroken StartupMode = "broken"
)

type ReconciliationDecision string

const (
	ReconciliationNone            ReconciliationDecision = "none"
	ReconciliationResumeSetup     ReconciliationDecision = "resume_setup"
	ReconciliationRequireDatabase ReconciliationDecision = "require_database"
)

type StartupDecision struct {
	Mode           StartupMode
	Reconciliation ReconciliationDecision
}

var ErrStartupStateInconsistent = errors.New("startup state is inconsistent")

func ResolveStartupMode(bootstrap config.BootstrapConfig, state InstallState, stateExists bool) (StartupMode, error) {
	decision, err := ResolveStartupDecision(bootstrap, state, stateExists)
	return decision.Mode, err
}

func ResolveStartupDecision(bootstrap config.BootstrapConfig, state InstallState, stateExists bool) (StartupDecision, error) {
	broken := StartupDecision{Mode: StartupModeBroken, Reconciliation: ReconciliationNone}
	if !stateExists {
		if err := validateBootstrapMetadata(bootstrap); err != nil {
			return broken, inconsistent("missing install state and invalid bootstrap metadata", err)
		}
		if bootstrap.SetupCompleted {
			return broken, inconsistent("runtime env claims completion but install state is missing", nil)
		}
		if !setupAuthority(bootstrap.Deployment.Role) {
			return broken, inconsistent("joined node cannot enter setup without install state", nil)
		}
		return StartupDecision{Mode: StartupModeSetup, Reconciliation: ReconciliationNone}, nil
	}

	if err := state.Validate(); err != nil {
		return broken, inconsistent("install state validation failed", err)
	}
	if err := validateBootstrapStateIdentity(bootstrap, state); err != nil {
		return broken, err
	}

	switch state.Phase {
	case InstallPhasePending:
		if bootstrap.SetupCompleted {
			return broken, inconsistent("runtime env completion flag has no completed install-state marker", nil)
		}
		if !setupAuthority(state.DeploymentRole) {
			return broken, inconsistent("joined node cannot enter setup", nil)
		}
		return StartupDecision{Mode: StartupModeSetup, Reconciliation: ReconciliationNone}, nil
	case InstallPhaseCommitting:
		reconciliation, err := ResolveCommitReconciliation(bootstrap, state)
		if err != nil {
			return broken, err
		}
		switch reconciliation {
		case ReconciliationResumeSetup:
			return StartupDecision{Mode: StartupModeSetup, Reconciliation: reconciliation}, nil
		case ReconciliationRequireDatabase:
			// The env rename has happened, but normal routes remain closed until the
			// database installation record is checked and FinalizeCommit succeeds.
			return StartupDecision{Mode: StartupModeBroken, Reconciliation: reconciliation}, nil
		default:
			return broken, inconsistent("committing state produced no reconciliation action", nil)
		}
	case InstallPhaseCompleted:
		if err := validateCompleteBootstrap(bootstrap); err != nil {
			return broken, inconsistent("completed installation has incomplete runtime env", err)
		}
		if state.Commit.RuntimeSchemaVersion != bootstrap.SchemaVersion {
			return broken, inconsistent("completed install state runtime schema does not match runtime env", nil)
		}
		if state.Commit.ConfigRevision != bootstrap.ConfigRevision {
			return broken, inconsistent("completed install state config revision does not match runtime env", nil)
		}
		return StartupDecision{Mode: StartupModeNormal, Reconciliation: ReconciliationNone}, nil
	default:
		return broken, inconsistent("unsupported install phase", nil)
	}
}

func ResolveCommitReconciliation(bootstrap config.BootstrapConfig, state InstallState) (ReconciliationDecision, error) {
	if err := state.Validate(); err != nil {
		return ReconciliationNone, inconsistent("install state validation failed", err)
	}
	if state.Phase != InstallPhaseCommitting || state.Commit == nil {
		return ReconciliationNone, inconsistent("install state is not committing", nil)
	}
	if err := validateBootstrapStateIdentity(bootstrap, state); err != nil {
		return ReconciliationNone, err
	}
	if !setupAuthority(state.DeploymentRole) {
		return ReconciliationNone, inconsistent("joined node cannot reconcile setup commit", nil)
	}
	if !bootstrap.SetupCompleted {
		return ReconciliationResumeSetup, nil
	}
	if err := validateCompleteBootstrap(bootstrap); err != nil {
		return ReconciliationNone, inconsistent("renamed runtime env is incomplete", err)
	}
	if bootstrap.ConfigRevision != state.Commit.ConfigRevision {
		return ReconciliationNone, inconsistent("commit journal config revision does not match runtime env", nil)
	}
	if bootstrap.SchemaVersion != state.Commit.RuntimeSchemaVersion {
		return ReconciliationNone, inconsistent("commit journal runtime schema does not match runtime env", nil)
	}
	return ReconciliationRequireDatabase, nil
}

func validateBootstrapStateIdentity(bootstrap config.BootstrapConfig, state InstallState) error {
	if err := validateBootstrapMetadata(bootstrap); err != nil {
		return inconsistent("bootstrap metadata is invalid", err)
	}
	if bootstrap.InstallationID != state.InstallationID {
		return inconsistent("runtime and install-state installation IDs do not match", nil)
	}
	if bootstrap.Deployment.Role != state.DeploymentRole {
		return inconsistent("runtime and install-state deployment roles do not match", nil)
	}
	return nil
}

func validateBootstrapMetadata(bootstrap config.BootstrapConfig) error {
	if bootstrap.SchemaVersion != config.CurrentRuntimeSchemaVersion {
		return fmt.Errorf("runtime schema version must be %d, got %d", config.CurrentRuntimeSchemaVersion, bootstrap.SchemaVersion)
	}
	if err := validateInstallIdentifier("installation ID", bootstrap.InstallationID); err != nil {
		return err
	}
	if !validDeploymentRole(bootstrap.Deployment.Role) {
		return fmt.Errorf("deployment role %q is invalid", bootstrap.Deployment.Role)
	}
	if bootstrap.Values == nil {
		return fmt.Errorf("runtime env values are missing")
	}

	checks := map[string]string{
		"RUNTIME_SCHEMA_VERSION": strconv.Itoa(bootstrap.SchemaVersion),
		"DEPLOYMENT_MODE":        string(bootstrap.Deployment.Mode),
		"DEPLOYMENT_PROFILE":     string(bootstrap.Deployment.Profile),
		"DEPLOYMENT_TOPOLOGY":    string(bootstrap.Deployment.Topology),
		"DEPLOYMENT_ROLE":        string(bootstrap.Deployment.Role),
		"INSTALLATION_ID":        bootstrap.InstallationID,
		"SETUP_COMPLETED":        strconv.FormatBool(bootstrap.SetupCompleted),
	}
	for key, want := range checks {
		if got := bootstrap.Values[key]; got != want {
			return fmt.Errorf("bootstrap field %s = %q, want %q", key, got, want)
		}
	}
	if bootstrap.Deployment.SetupCompleted != bootstrap.SetupCompleted {
		return fmt.Errorf("deployment setup-completed metadata is inconsistent")
	}
	if err := config.ValidateDeploymentContext(bootstrap.Deployment); err != nil {
		return fmt.Errorf("validate deployment context: %w", err)
	}
	return nil
}

func validateCompleteBootstrap(bootstrap config.BootstrapConfig) error {
	if err := validateBootstrapMetadata(bootstrap); err != nil {
		return err
	}
	if !bootstrap.SetupCompleted {
		return fmt.Errorf("SETUP_COMPLETED must be true")
	}
	if rawRevision, exists := bootstrap.Values["CONFIG_REVISION"]; exists && strings.TrimSpace(rawRevision) != "" {
		revision, err := strconv.Atoi(rawRevision)
		if err != nil || revision <= 0 {
			return fmt.Errorf("CONFIG_REVISION must be a positive integer")
		}
		if revision != bootstrap.ConfigRevision {
			return fmt.Errorf("CONFIG_REVISION does not match bootstrap metadata")
		}
	}

	schema := config.DefaultRuntimeSchema()
	required, err := config.RequiredRuntimeFields(schema, bootstrap.Deployment)
	if err != nil {
		return fmt.Errorf("resolve required runtime fields: %w", err)
	}
	for _, field := range required {
		if strings.TrimSpace(bootstrap.Values[field.Key]) == "" {
			return fmt.Errorf("required runtime field %s is empty", field.Key)
		}
	}
	for _, field := range schema.Fields {
		value, exists := bootstrap.Values[field.Key]
		if !exists || value == "" {
			continue
		}
		if err := field.Validate(value); err != nil {
			return fmt.Errorf("validate runtime field %s: %w", field.Key, err)
		}
	}
	return nil
}

func inconsistent(message string, cause error) error {
	if cause == nil {
		return fmt.Errorf("%w: %s", ErrStartupStateInconsistent, message)
	}
	return fmt.Errorf("%w: %s: %v", ErrStartupStateInconsistent, message, cause)
}
