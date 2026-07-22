package deployctl

import (
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

type SetupStatus struct {
	InstallationID string
	Role           config.DeploymentRole
	Phase          setup.InstallPhase
	Completed      bool
	TokenVersion   uint64
}

func LoadSetupStatus(runtimeDir string) (SetupStatus, error) {
	runtimeDir = filepath.Clean(defaultString(runtimeDir, "."))
	runtimeEnvPath := filepath.Join(runtimeDir, "config", "runtime.env")
	bootstrap, err := config.LoadBootstrap(runtimeEnvPath)
	if err != nil {
		return SetupStatus{}, fmt.Errorf("load setup runtime configuration: %w", err)
	}
	state, exists, err := setup.NewStateStore(runtimeEnvPath).Load()
	if err != nil {
		return SetupStatus{}, fmt.Errorf("load setup install state: %w", err)
	}
	if !exists {
		return SetupStatus{}, fmt.Errorf("setup install state does not exist")
	}
	if state.InstallationID != bootstrap.InstallationID || state.DeploymentRole != bootstrap.Deployment.Role {
		return SetupStatus{}, fmt.Errorf("setup runtime and install state identities do not match")
	}
	if bootstrap.SetupCompleted != (state.Phase == setup.InstallPhaseCompleted) {
		return SetupStatus{}, fmt.Errorf("setup runtime and install state completion markers do not match")
	}
	return SetupStatus{
		InstallationID: bootstrap.InstallationID, Role: bootstrap.Deployment.Role, Phase: state.Phase,
		Completed: bootstrap.SetupCompleted, TokenVersion: bootstrap.SetupTokenVersion,
	}, nil
}

func ShowSetupToken(runtimeDir string) (string, error) {
	runtimeDir = filepath.Clean(defaultString(runtimeDir, "."))
	runtimeEnvPath := filepath.Join(runtimeDir, "config", "runtime.env")
	bootstrap, err := config.LoadBootstrap(runtimeEnvPath)
	if err != nil {
		return "", fmt.Errorf("load setup runtime configuration: %w", err)
	}
	state, exists, err := setup.NewStateStore(runtimeEnvPath).Load()
	if err != nil {
		return "", fmt.Errorf("load setup install state: %w", err)
	}
	if !exists || state.InstallationID != bootstrap.InstallationID {
		return "", fmt.Errorf("setup install state does not match runtime configuration")
	}
	if bootstrap.SetupCompleted || state.Phase == setup.InstallPhaseCompleted || state.EverCompleted {
		return "", fmt.Errorf("setup is completed; its token is permanently unavailable")
	}
	if state.Phase != setup.InstallPhasePending || (state.DeploymentRole != config.DeploymentRoleSingle && state.DeploymentRole != config.DeploymentRoleControl) {
		return "", fmt.Errorf("setup token is available only for a pending single or control installation")
	}
	if strings.TrimSpace(bootstrap.SetupToken) == "" {
		return "", fmt.Errorf("pending setup token is missing; run setup token reset")
	}
	return bootstrap.SetupToken, nil
}

type SetupTokenResetDependencies struct {
	Entropy           io.Reader
	AcquireLock       func(context.Context, string) (func() error, error)
	WriteRuntimeEnv   func(string, []byte) error
	RestartDeployment func(context.Context, InstallPlan) error
}

func ResetSetupToken(ctx context.Context, runtimeDir string, dependencies SetupTokenResetDependencies) (token string, returnErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if dependencies.Entropy == nil {
		dependencies.Entropy = cryptorand.Reader
	}
	if dependencies.AcquireLock == nil {
		dependencies.AcquireLock = acquireInstallLock
	}
	if dependencies.WriteRuntimeEnv == nil {
		dependencies.WriteRuntimeEnv = config.WriteRuntimeEnvAtomic
	}
	runtimeDir = filepath.Clean(defaultString(runtimeDir, "."))
	runtimeEnvPath := filepath.Join(runtimeDir, "config", "runtime.env")
	release, err := dependencies.AcquireLock(ctx, filepath.Join(runtimeDir, "config", ".deployctl-setup-token.lock"))
	if err != nil {
		return "", fmt.Errorf("acquire setup token lock: %w", err)
	}
	defer func() {
		if err := release(); err != nil && returnErr == nil {
			returnErr = fmt.Errorf("release setup token lock: %w", err)
		}
	}()

	plan, document, err := loadInstallation(runtimeDir)
	if err != nil {
		return "", err
	}
	state, exists, err := setup.NewStateStore(runtimeEnvPath).Load()
	if err != nil {
		return "", fmt.Errorf("load setup install state: %w", err)
	}
	completed := strings.EqualFold(document.Values["SETUP_COMPLETED"], "true")
	if !exists || state.InstallationID != document.Values["INSTALLATION_ID"] {
		return "", fmt.Errorf("setup install state does not match runtime configuration")
	}
	if completed || state.Phase == setup.InstallPhaseCompleted || state.EverCompleted {
		return "", fmt.Errorf("setup is completed; its token cannot be reset")
	}
	if state.Phase != setup.InstallPhasePending || state.Attempt != nil {
		return "", fmt.Errorf("setup token cannot be reset while setup is in progress")
	}
	version, err := strconv.ParseUint(document.Values["SETUP_TOKEN_VERSION"], 10, 64)
	if err != nil || version == 0 || version == math.MaxUint64 {
		return "", fmt.Errorf("setup token version cannot be incremented")
	}
	token, err = setup.GenerateSetupToken(dependencies.Entropy)
	if err != nil {
		return "", fmt.Errorf("generate setup token: %w", err)
	}
	document.Values["SETUP_TOKEN"] = token
	document.Values["SETUP_TOKEN_VERSION"] = strconv.FormatUint(version+1, 10)
	rendered, err := config.RenderRuntimeEnv(config.DefaultRuntimeSchema(), document.Values, document.Extensions)
	if err != nil {
		return "", fmt.Errorf("render reset setup token configuration: %w", redactRuntimeError(err, document.Values))
	}
	if err := dependencies.WriteRuntimeEnv(runtimeEnvPath, rendered); err != nil {
		return "", fmt.Errorf("write reset setup token configuration: %w", err)
	}
	if dependencies.RestartDeployment != nil {
		if err := dependencies.RestartDeployment(ctx, plan); err != nil {
			return "", fmt.Errorf("restart after setup token reset: %w; the token was rotated and can be displayed with setup token show", err)
		}
	}
	return token, nil
}
