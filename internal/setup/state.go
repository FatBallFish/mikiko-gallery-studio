package setup

import (
	"fmt"
	"regexp"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
)

const CurrentInstallStateSchemaVersion = 1

type InstallPhase string

const (
	InstallPhasePending    InstallPhase = "pending"
	InstallPhaseCommitting InstallPhase = "committing"
	InstallPhaseCompleted  InstallPhase = "completed"
)

type CommitJournal struct {
	OperationID          string `json:"operation_id"`
	InstallationID       string `json:"installation_id"`
	RuntimeSchemaVersion int    `json:"runtime_schema_version"`
	ConfigRevision       int    `json:"config_revision"`
}

type InstallState struct {
	SchemaVersion  int                   `json:"schema_version"`
	InstallationID string                `json:"installation_id"`
	DeploymentRole config.DeploymentRole `json:"deployment_role"`
	Phase          InstallPhase          `json:"phase"`
	EverCompleted  bool                  `json:"ever_completed"`
	UpdatedAt      time.Time             `json:"updated_at"`
	Commit         *CommitJournal        `json:"commit,omitempty"`
}

var installIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func (state InstallState) Validate() error {
	if state.SchemaVersion != CurrentInstallStateSchemaVersion {
		return fmt.Errorf("install state schema version must be %d, got %d", CurrentInstallStateSchemaVersion, state.SchemaVersion)
	}
	if err := validateInstallIdentifier("installation ID", state.InstallationID); err != nil {
		return err
	}
	if !validDeploymentRole(state.DeploymentRole) {
		return fmt.Errorf("deployment role %q is invalid", state.DeploymentRole)
	}
	if state.UpdatedAt.IsZero() {
		return fmt.Errorf("updated_at must not be zero")
	}
	if state.UpdatedAt.Year() < 1 || state.UpdatedAt.Year() > 9999 {
		return fmt.Errorf("updated_at is outside the JSON timestamp range")
	}
	if state.UpdatedAt.Location() != time.UTC {
		return fmt.Errorf("updated_at must use UTC")
	}

	switch state.Phase {
	case InstallPhasePending:
		if state.EverCompleted {
			return fmt.Errorf("pending state cannot have ever_completed=true")
		}
		if state.Commit != nil {
			return fmt.Errorf("pending state cannot contain a commit journal")
		}
	case InstallPhaseCommitting:
		if state.EverCompleted {
			return fmt.Errorf("committing state cannot have ever_completed=true")
		}
		if state.Commit == nil {
			return fmt.Errorf("committing state requires a commit journal")
		}
	case InstallPhaseCompleted:
		if !state.EverCompleted {
			return fmt.Errorf("completed state requires ever_completed=true")
		}
		if state.Commit == nil {
			return fmt.Errorf("completed state requires the finalized commit journal")
		}
	default:
		return fmt.Errorf("install phase %q is invalid", state.Phase)
	}

	if state.Commit != nil {
		if err := state.Commit.Validate(); err != nil {
			return fmt.Errorf("validate commit journal: %w", err)
		}
		if state.Commit.InstallationID != state.InstallationID {
			return fmt.Errorf("commit journal installation ID does not match install state")
		}
	}
	return nil
}

func (journal CommitJournal) Validate() error {
	if err := validateInstallIdentifier("operation ID", journal.OperationID); err != nil {
		return err
	}
	if err := validateInstallIdentifier("installation ID", journal.InstallationID); err != nil {
		return err
	}
	if journal.RuntimeSchemaVersion <= 0 {
		return fmt.Errorf("runtime schema version must be positive")
	}
	if journal.ConfigRevision <= 0 {
		return fmt.Errorf("config revision must be positive")
	}
	return nil
}

func validateInstallIdentifier(name, value string) error {
	if !installIdentifierPattern.MatchString(value) {
		return fmt.Errorf("%s %q is invalid", name, value)
	}
	return nil
}

func validDeploymentRole(role config.DeploymentRole) bool {
	switch role {
	case config.DeploymentRoleSingle, config.DeploymentRoleControl, config.DeploymentRoleAPI, config.DeploymentRoleWorker, config.DeploymentRoleWeb:
		return true
	default:
		return false
	}
}

func setupAuthority(role config.DeploymentRole) bool {
	return role == config.DeploymentRoleSingle || role == config.DeploymentRoleControl
}

func validateTransitionTime(at time.Time) (time.Time, error) {
	if at.IsZero() {
		return time.Time{}, fmt.Errorf("transition time must not be zero")
	}
	return at.UTC(), nil
}
