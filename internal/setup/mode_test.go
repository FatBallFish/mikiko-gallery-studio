package setup

import (
	"errors"
	"strconv"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
)

func TestResolveStartupModePendingControlInstallationEntersSetup(t *testing.T) {
	for _, role := range []config.DeploymentRole{config.DeploymentRoleSingle, config.DeploymentRoleControl} {
		t.Run(string(role), func(t *testing.T) {
			bootstrap := pendingBootstrap(role)
			state := pendingStateForRole(role)
			mode, err := ResolveStartupMode(bootstrap, state, true)
			if err != nil || mode != StartupModeSetup {
				t.Fatalf("ResolveStartupMode() = (%q, %v), want (%q, nil)", mode, err, StartupModeSetup)
			}
		})
	}
}

func TestResolveStartupModeRejectsJoinedRolesFromSetup(t *testing.T) {
	for _, role := range []config.DeploymentRole{config.DeploymentRoleAPI, config.DeploymentRoleWorker, config.DeploymentRoleWeb} {
		t.Run(string(role), func(t *testing.T) {
			mode, err := ResolveStartupMode(pendingBootstrap(role), pendingStateForRole(role), true)
			if mode != StartupModeBroken || !errors.Is(err, ErrStartupStateInconsistent) {
				t.Fatalf("ResolveStartupMode() = (%q, %v), want broken inconsistent", mode, err)
			}
		})
	}
}

func TestResolveStartupModeRequiresBothCompletedStateAndCompleteEnv(t *testing.T) {
	completedState := completedStateForRole(config.DeploymentRoleSingle)
	completeBootstrap := completedBootstrap(config.DeploymentRoleSingle)

	mode, err := ResolveStartupMode(completeBootstrap, completedState, true)
	if err != nil || mode != StartupModeNormal {
		t.Fatalf("ResolveStartupMode(completed) = (%q, %v), want normal", mode, err)
	}

	tests := []struct {
		name        string
		bootstrap   config.BootstrapConfig
		state       InstallState
		stateExists bool
	}{
		{name: "state missing", bootstrap: completeBootstrap, stateExists: false},
		{name: "env missing after completion", bootstrap: config.BootstrapConfig{}, state: completedState, stateExists: true},
		{name: "env incomplete after completion", bootstrap: pendingBootstrap(config.DeploymentRoleSingle), state: completedState, stateExists: true},
		{name: "installation mismatch", bootstrap: withInstallationID(completeBootstrap, "019d0000-0000-7000-8000-000000000099"), state: completedState, stateExists: true},
		{name: "role mismatch", bootstrap: completeBootstrap, state: completedStateForRole(config.DeploymentRoleControl), stateExists: true},
		{name: "schema mismatch", bootstrap: withSchemaVersion(completeBootstrap, config.CurrentRuntimeSchemaVersion+1), state: completedState, stateExists: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, err := ResolveStartupMode(tt.bootstrap, tt.state, tt.stateExists)
			if mode != StartupModeBroken || !errors.Is(err, ErrStartupStateInconsistent) {
				t.Fatalf("ResolveStartupMode() = (%q, %v), want broken inconsistent", mode, err)
			}
		})
	}
}

func TestResolveStartupModeAllowsCompletedJoinedRoles(t *testing.T) {
	for _, role := range []config.DeploymentRole{
		config.DeploymentRoleControl,
		config.DeploymentRoleAPI,
		config.DeploymentRoleWorker,
		config.DeploymentRoleWeb,
	} {
		t.Run(string(role), func(t *testing.T) {
			mode, err := ResolveStartupMode(completedBootstrap(role), completedStateForRole(role), true)
			if err != nil || mode != StartupModeNormal {
				t.Fatalf("ResolveStartupMode() = (%q, %v), want normal", mode, err)
			}
		})
	}
}

func TestResolveStartupModeDoesNotTrustSetupCompletedFlagAlone(t *testing.T) {
	mode, err := ResolveStartupMode(completedBootstrap(config.DeploymentRoleSingle), pendingState(), true)
	if mode != StartupModeBroken || !errors.Is(err, ErrStartupStateInconsistent) {
		t.Fatalf("ResolveStartupMode() = (%q, %v), want broken inconsistent", mode, err)
	}
}

func TestResolveStartupModeMissingStateAllowsOnlyIdentifiedSetupAuthority(t *testing.T) {
	mode, err := ResolveStartupMode(pendingBootstrap(config.DeploymentRoleSingle), InstallState{}, false)
	if err != nil || mode != StartupModeSetup {
		t.Fatalf("ResolveStartupMode(single missing state) = (%q, %v), want setup", mode, err)
	}

	mode, err = ResolveStartupMode(config.BootstrapConfig{}, InstallState{}, false)
	if mode != StartupModeBroken || !errors.Is(err, ErrStartupStateInconsistent) {
		t.Fatalf("ResolveStartupMode(unidentified missing state) = (%q, %v), want broken", mode, err)
	}
}

func TestResolveStartupDecisionDistinguishesCommitCrashPoints(t *testing.T) {
	state := committingStateForRole(config.DeploymentRoleSingle)

	before, err := ResolveStartupDecision(pendingBootstrap(config.DeploymentRoleSingle), state, true)
	if err != nil {
		t.Fatalf("ResolveStartupDecision(before rename) error = %v", err)
	}
	if before.Mode != StartupModeSetup || before.Reconciliation != ReconciliationResumeSetup {
		t.Fatalf("before-rename decision = %+v, want setup/resume", before)
	}

	after, err := ResolveStartupDecision(completedBootstrap(config.DeploymentRoleSingle), state, true)
	if err != nil {
		t.Fatalf("ResolveStartupDecision(after rename) error = %v", err)
	}
	if after.Mode != StartupModeBroken || after.Reconciliation != ReconciliationRequireDatabase {
		t.Fatalf("after-rename decision = %+v, want broken/requires-database", after)
	}

	mode, err := ResolveStartupMode(completedBootstrap(config.DeploymentRoleSingle), state, true)
	if err != nil || mode != StartupModeBroken {
		t.Fatalf("ResolveStartupMode(after rename) = (%q, %v), want fail-closed broken without error", mode, err)
	}
}

func TestResolveStartupDecisionRejectsInconsistentCommitJournalAndEnv(t *testing.T) {
	tests := []struct {
		name      string
		bootstrap config.BootstrapConfig
		state     InstallState
	}{
		{name: "revision mismatch", bootstrap: withConfigRevision(completedBootstrap(config.DeploymentRoleSingle), 8), state: committingStateForRole(config.DeploymentRoleSingle)},
		{name: "operation installation mismatch", bootstrap: completedBootstrap(config.DeploymentRoleSingle), state: func() InstallState {
			state := committingStateForRole(config.DeploymentRoleSingle)
			state.Commit.InstallationID = "019d0000-0000-7000-8000-000000000099"
			return state
		}()},
		{name: "runtime schema journal mismatch", bootstrap: completedBootstrap(config.DeploymentRoleSingle), state: func() InstallState {
			state := committingStateForRole(config.DeploymentRoleSingle)
			state.Commit.RuntimeSchemaVersion++
			return state
		}()},
		{name: "completed env incomplete", bootstrap: func() config.BootstrapConfig {
			bootstrap := completedBootstrap(config.DeploymentRoleSingle)
			delete(bootstrap.Values, "DATABASE_URL")
			return bootstrap
		}(), state: committingStateForRole(config.DeploymentRoleSingle)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := ResolveStartupDecision(tt.bootstrap, tt.state, true)
			if decision.Mode != StartupModeBroken || !errors.Is(err, ErrStartupStateInconsistent) {
				t.Fatalf("ResolveStartupDecision() = (%+v, %v), want broken inconsistent", decision, err)
			}
		})
	}
}

func TestResolveStartupDecisionRejectsUnsupportedJournalBeforeEnvRename(t *testing.T) {
	for name, version := range map[string]int{
		"future": config.CurrentRuntimeSchemaVersion + 1,
		"old":    0,
	} {
		t.Run(name, func(t *testing.T) {
			state := committingStateForRole(config.DeploymentRoleSingle)
			state.Commit.RuntimeSchemaVersion = version
			decision, err := ResolveStartupDecision(pendingBootstrap(config.DeploymentRoleSingle), state, true)
			if decision.Mode != StartupModeBroken || decision.Reconciliation != ReconciliationNone || !errors.Is(err, ErrStartupStateInconsistent) {
				t.Fatalf("ResolveStartupDecision() = (%+v, %v), want broken inconsistent", decision, err)
			}
		})
	}
}

func pendingStateForRole(role config.DeploymentRole) InstallState {
	state := pendingState()
	state.DeploymentRole = role
	return state
}

func committingStateForRole(role config.DeploymentRole) InstallState {
	state := pendingStateForRole(role)
	state.Phase = InstallPhaseCommitting
	state.Commit = validCommitJournal()
	return state
}

func completedStateForRole(role config.DeploymentRole) InstallState {
	state := committingStateForRole(role)
	state.Phase = InstallPhaseCompleted
	state.EverCompleted = true
	return state
}

func pendingBootstrap(role config.DeploymentRole) config.BootstrapConfig {
	context := deploymentContextForRole(role, false)
	values := map[string]string{
		"RUNTIME_SCHEMA_VERSION": strconv.Itoa(config.CurrentRuntimeSchemaVersion),
		"DEPLOYMENT_MODE":        string(context.Mode),
		"DEPLOYMENT_PROFILE":     string(context.Profile),
		"DEPLOYMENT_TOPOLOGY":    string(context.Topology),
		"DEPLOYMENT_ROLE":        string(context.Role),
		"DEPLOYMENT_MODULES":     "api,worker",
		"SETUP_COMPLETED":        "false",
		"INSTALLATION_ID":        testInstallationID,
		"APPLICATION_VERSION":    "v1.0.0",
	}
	return config.BootstrapConfig{
		SchemaVersion:  config.CurrentRuntimeSchemaVersion,
		Deployment:     context,
		SetupCompleted: false,
		InstallationID: testInstallationID,
		Values:         values,
	}
}

func completedBootstrap(role config.DeploymentRole) config.BootstrapConfig {
	bootstrap := pendingBootstrap(role)
	bootstrap.SetupCompleted = true
	bootstrap.Deployment.SetupCompleted = true
	bootstrap.ConfigRevision = 7
	bootstrap.Values["SETUP_COMPLETED"] = "true"
	bootstrap.Values["CONFIG_REVISION"] = "7"
	bootstrap.Values["POSTGRES_MANAGED"] = "false"
	bootstrap.Values["REDIS_MANAGED"] = "false"
	bootstrap.Values["OBJECT_STORAGE_MANAGED"] = "false"
	bootstrap.Values["DATABASE_URL"] = "postgres://app:password@127.0.0.1:5432/app?sslmode=disable"
	bootstrap.Values["REDIS_URL"] = "redis://127.0.0.1:6379/0"
	bootstrap.Values["REDIS_KEY_PREFIX"] = "app"
	if role == config.DeploymentRoleSingle {
		bootstrap.Values["STORAGE_DRIVER"] = "local"
		bootstrap.Values["STORAGE_LOCAL_ROOT"] = "./data/storage"
		bootstrap.Values["STORAGE_SHARED_VOLUME"] = "true"
		bootstrap.Deployment.StorageDriver = "local"
	} else {
		bootstrap.Values["STORAGE_DRIVER"] = "s3"
		bootstrap.Values["STORAGE_S3_ENDPOINT"] = "http://127.0.0.1:9000"
		bootstrap.Values["STORAGE_S3_REGION"] = "us-east-1"
		bootstrap.Values["STORAGE_S3_BUCKET"] = "app-assets"
		bootstrap.Values["STORAGE_S3_ACCESS_KEY_ID"] = "access-key"
		bootstrap.Values["STORAGE_S3_SECRET_ACCESS_KEY"] = "secret-key"
		bootstrap.Values["CLUSTER_NODE_ID"] = "019d0000-0000-7000-8000-000000000002"
		bootstrap.ClusterNodeID = bootstrap.Values["CLUSTER_NODE_ID"]
		bootstrap.Deployment.StorageDriver = "s3"
	}
	bootstrap.Values["AUTH_ACCESS_TOKEN_SECRET"] = "access-token-secret"
	bootstrap.Values["API_KEY_SIGNING_SECRET_ENCRYPTION_KEY"] = "api-key-secret"
	bootstrap.Values["CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY"] = "cashier-key"
	bootstrap.Values["PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY"] = "secure-config-key"
	bootstrap.Values["PROMPT_OPTIMIZATION_QUOTE_SIGNING_KEY"] = "quote-signing-key"
	bootstrap.Values["API_PORT"] = "8080"
	bootstrap.Values["IMAGE_TAG"] = "v1.0.0"
	bootstrap.Values["PUBLIC_API_URL"] = "http://127.0.0.1:8080"
	return bootstrap
}

func deploymentContextForRole(role config.DeploymentRole, completed bool) config.DeploymentContext {
	context := config.DeploymentContext{
		Mode:           config.DeploymentModeDocker,
		Profile:        config.DeploymentProfileCore,
		Role:           role,
		StorageDriver:  "local",
		SetupCompleted: completed,
	}
	if role == config.DeploymentRoleSingle {
		context.Topology = config.DeploymentTopologySingle
	} else {
		context.Topology = config.DeploymentTopologyCluster
		context.StorageDriver = "s3"
	}
	return context
}

func withInstallationID(bootstrap config.BootstrapConfig, id string) config.BootstrapConfig {
	bootstrap.InstallationID = id
	bootstrap.Values = cloneValues(bootstrap.Values)
	bootstrap.Values["INSTALLATION_ID"] = id
	return bootstrap
}

func withSchemaVersion(bootstrap config.BootstrapConfig, version int) config.BootstrapConfig {
	bootstrap.SchemaVersion = version
	bootstrap.Values = cloneValues(bootstrap.Values)
	bootstrap.Values["RUNTIME_SCHEMA_VERSION"] = strconv.Itoa(version)
	return bootstrap
}

func withConfigRevision(bootstrap config.BootstrapConfig, revision int) config.BootstrapConfig {
	bootstrap.ConfigRevision = revision
	bootstrap.Values = cloneValues(bootstrap.Values)
	bootstrap.Values["CONFIG_REVISION"] = strconv.Itoa(revision)
	return bootstrap
}

func cloneValues(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
