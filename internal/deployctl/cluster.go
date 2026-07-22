package deployctl

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/config"
	domaincluster "github.com/fatballfish/pic-gallery/internal/domain/cluster"
	clusterservice "github.com/fatballfish/pic-gallery/internal/service/cluster"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

const maxClusterHTTPBodyBytes = 1 << 20

type ClusterJoinDependencies struct {
	HTTPClient          *http.Client
	Entropy             io.Reader
	Now                 func() time.Time
	PathExists          func(string) (bool, error)
	MakeDirectory       func(string, os.FileMode) error
	AcquireInstallLock  func(context.Context, string) (func() error, error)
	WriteRuntimeEnv     func(string, []byte) error
	WriteInstallState   func(string, []byte) error
	WriteManifest       func(string, []byte) error
	WriteDeploymentFile func(string, []byte) error
	RemovePath          func(string) error
	PreflightDeployment func(context.Context, InstallPlan) error
	ApplyDeployment     func(context.Context, InstallPlan) error
}

type ClusterJoinResult struct {
	RuntimeEnvPath string
	ManifestPath   string
	InstallationID string
	NodeID         string
	Role           config.DeploymentRole
	Plan           InstallPlan
}

func ExecuteClusterJoin(ctx context.Context, options ClusterJoinOptions, dependencies ClusterJoinDependencies) (result ClusterJoinResult, returnErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateClusterJoinOptions(options); err != nil {
		return ClusterJoinResult{}, err
	}
	dependencies = defaultClusterJoinDependencies(dependencies)
	runtimeDir := filepath.Clean(defaultString(options.RuntimeDir, "."))
	runtimeEnvPath := filepath.Join(runtimeDir, "config", "runtime.env")
	statePath := filepath.Join(runtimeDir, "config", "install-state.json")
	manifestPath := filepath.Join(runtimeDir, "deployment.json")
	for _, directory := range []string{runtimeDir, filepath.Join(runtimeDir, "config")} {
		if err := dependencies.MakeDirectory(directory, 0o700); err != nil {
			return ClusterJoinResult{}, fmt.Errorf("create cluster runtime directory: %w", err)
		}
	}
	releaseLock, err := dependencies.AcquireInstallLock(ctx, filepath.Join(runtimeDir, "config", ".deployctl-install.lock"))
	if err != nil {
		return ClusterJoinResult{}, fmt.Errorf("acquire cluster join lock: %w", err)
	}
	defer func() {
		if err := releaseLock(); err != nil && returnErr == nil {
			returnErr = fmt.Errorf("release cluster join lock: %w", err)
		}
	}()
	for _, path := range []string{runtimeEnvPath, statePath, manifestPath} {
		exists, err := dependencies.PathExists(path)
		if err != nil {
			return ClusterJoinResult{}, fmt.Errorf("inspect cluster join target: %w", err)
		}
		if exists {
			return ClusterJoinResult{}, fmt.Errorf("cluster join target already exists: %s", path)
		}
	}

	tokenID, err := clusterJoinTokenID(options.Token)
	if err != nil {
		return ClusterJoinResult{}, err
	}
	proofKey, err := clusterservice.TokenProofKeyFromCredential(options.Token)
	if err != nil {
		return ClusterJoinResult{}, errors.New("cluster join token is invalid")
	}
	defer proofKey.Clear()
	clientKey, err := clusterservice.GenerateEphemeralKey(dependencies.Entropy)
	if err != nil {
		return ClusterJoinResult{}, fmt.Errorf("generate cluster node key: %w", err)
	}
	defer clientKey.Clear()
	nodeID, err := randomClusterNodeID(dependencies.Entropy)
	if err != nil {
		return ClusterJoinResult{}, err
	}
	challengeRequest := domaincluster.CreateChallengeRequest{
		Protocol: clusterservice.EnrollmentProtocolV1, TokenID: tokenID, NodeID: nodeID,
		NodePublicKey: clientKey.PublicKey(), ApplicationVersion: options.ApplicationVersion,
		RuntimeSchemaVersion: config.CurrentRuntimeSchemaVersion,
	}
	var challenge domaincluster.EnrollmentChallenge
	if err := postClusterJSON(ctx, dependencies.HTTPClient, options.Server+"/api/open/cluster/v1/challenges", challengeRequest, &challenge); err != nil {
		return ClusterJoinResult{}, fmt.Errorf("create cluster challenge: %w", err)
	}
	if err := validateRemoteChallenge(challenge, challengeRequest, dependencies.Now()); err != nil {
		return ClusterJoinResult{}, err
	}
	preflightPlan, err := buildJoinedPlan(options, config.DeploymentRole(challenge.Role), challenge.ApplicationVersion, challenge.NodeID, "s3", options.Server)
	if err != nil {
		return ClusterJoinResult{}, err
	}
	preflightPaths, err := deploymentFilePaths(preflightPlan)
	if err != nil {
		return ClusterJoinResult{}, err
	}
	for _, path := range preflightPaths {
		exists, err := dependencies.PathExists(path)
		if err != nil {
			return ClusterJoinResult{}, fmt.Errorf("inspect cluster deployment target: %w", err)
		}
		if exists {
			return ClusterJoinResult{}, fmt.Errorf("cluster deployment target already exists: %s", path)
		}
	}
	if dependencies.PreflightDeployment != nil {
		if err := dependencies.PreflightDeployment(ctx, preflightPlan); err != nil {
			return ClusterJoinResult{}, fmt.Errorf("preflight cluster deployment: %w", err)
		}
	}
	proof, err := clusterservice.ComputeClientPossessionProof(proofKey, challenge, nodeID)
	if err != nil {
		return ClusterJoinResult{}, fmt.Errorf("compute cluster possession proof: %w", err)
	}
	var joined domaincluster.JoinResponse
	if err := postClusterJSON(ctx, dependencies.HTTPClient, options.Server+"/api/open/cluster/v1/join", domaincluster.JoinRequest{
		Protocol: clusterservice.EnrollmentProtocolV1, ChallengeID: challenge.ChallengeID, Proof: proof,
	}, &joined); err != nil {
		return ClusterJoinResult{}, fmt.Errorf("complete cluster join: %w", err)
	}
	if err := validateJoinResponse(joined, challenge, dependencies.Now()); err != nil {
		return ClusterJoinResult{}, err
	}
	payload, err := clusterservice.OpenRuntimeEnvelope(clientKey, proofKey, challenge, nodeID, joined.EncryptedEnvelope)
	if err != nil {
		return ClusterJoinResult{}, fmt.Errorf("decrypt cluster runtime envelope: %w", err)
	}
	if err := validateRemoteRuntimeValues(joined.Role, payload.Values); err != nil {
		return ClusterJoinResult{}, err
	}
	storageDriver := payload.Values["STORAGE_DRIVER"]
	publicAPIURL := payload.Values["PUBLIC_API_URL"]
	plan, err := buildJoinedPlan(options, config.DeploymentRole(joined.Role), joined.ApplicationVersion, nodeID, storageDriver, publicAPIURL)
	if err != nil {
		return ClusterJoinResult{}, err
	}
	runtimeValues := joinedRuntimeValues(options, plan, joined, payload.Values)
	runtimeEnv, err := config.RenderRuntimeEnv(config.DefaultRuntimeSchema(), runtimeValues, nil)
	if err != nil {
		return ClusterJoinResult{}, fmt.Errorf("render joined runtime environment: %w", err)
	}
	bootstrap := config.BootstrapConfig{
		SchemaVersion:     joined.RuntimeSchemaVersion,
		Deployment:        config.DeploymentContext{Mode: plan.Mode, Profile: plan.Profile, Topology: plan.Topology, Role: plan.Role, StorageDriver: plan.StorageDriver, SetupCompleted: true},
		DeploymentModules: componentsStrings(plan.Components), SetupCompleted: true, InstallationID: joined.InstallationID,
		ClusterNodeID: joined.NodeID, ConfigRevision: int(joined.ConfigRevision), ApplicationVersion: joined.ApplicationVersion,
		Values: runtimeValues,
	}
	if _, err := config.RuntimeFromBootstrap(bootstrap); err != nil {
		return ClusterJoinResult{}, fmt.Errorf("validate joined runtime environment: %w", err)
	}
	deploymentFiles, err := buildDeploymentFiles(plan)
	if err != nil {
		return ClusterJoinResult{}, err
	}
	manifest, err := renderJoinedManifest(plan, joined, deploymentFiles, dependencies.Now())
	if err != nil {
		return ClusterJoinResult{}, err
	}
	digest := sha256.Sum256(runtimeEnv)
	state := setup.InstallState{
		SchemaVersion: setup.CurrentInstallStateSchemaVersion, InstallationID: joined.InstallationID,
		DeploymentRole: plan.Role, Phase: setup.InstallPhaseCompleted, EverCompleted: true, UpdatedAt: dependencies.Now().UTC(),
		Commit: &setup.CommitProof{
			OperationID: challenge.ChallengeID, InstallationID: joined.InstallationID,
			RuntimeSchemaVersion: joined.RuntimeSchemaVersion, ConfigRevision: int(joined.ConfigRevision),
			RequestDigest: fmt.Sprintf("%x", digest),
		},
	}
	stateContent, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return ClusterJoinResult{}, fmt.Errorf("render joined install state: %w", err)
	}
	stateContent = append(stateContent, '\n')
	deploymentPaths, err := deploymentFilePaths(plan)
	if err != nil {
		return ClusterJoinResult{}, err
	}
	published := make([]string, 0, len(deploymentPaths)+3)
	rollback := func(operation string, operationErr error) (ClusterJoinResult, error) {
		rollbackPaths := slices.Clone(published)
		slices.Reverse(rollbackPaths)
		return ClusterJoinResult{}, installArtifactError(operation, operationErr, rollbackInstallArtifacts(InstallDependencies{RemovePath: dependencies.RemovePath}, rollbackPaths...))
	}
	if err := dependencies.WriteManifest(manifestPath, manifest); err != nil {
		return ClusterJoinResult{}, fmt.Errorf("write joined manifest: %w", err)
	}
	published = append(published, manifestPath)
	for index, file := range deploymentFiles {
		if err := dependencies.WriteDeploymentFile(deploymentPaths[index], file.Content); err != nil {
			return rollback("write joined deployment asset", err)
		}
		published = append(published, deploymentPaths[index])
	}
	if err := dependencies.WriteRuntimeEnv(runtimeEnvPath, runtimeEnv); err != nil {
		return rollback("write joined runtime environment", err)
	}
	published = append(published, runtimeEnvPath)
	if err := dependencies.WriteInstallState(statePath, stateContent); err != nil {
		return rollback("write joined install state", err)
	}
	published = append(published, statePath)
	if dependencies.ApplyDeployment != nil {
		if err := dependencies.ApplyDeployment(ctx, plan); err != nil {
			return ClusterJoinResult{}, fmt.Errorf("apply joined deployment: %w", err)
		}
	}
	return ClusterJoinResult{
		RuntimeEnvPath: runtimeEnvPath, ManifestPath: manifestPath, InstallationID: joined.InstallationID,
		NodeID: joined.NodeID, Role: plan.Role, Plan: plan,
	}, nil
}

func defaultClusterJoinDependencies(dependencies ClusterJoinDependencies) ClusterJoinDependencies {
	if dependencies.HTTPClient == nil {
		dependencies.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	if dependencies.Entropy == nil {
		dependencies.Entropy = cryptorand.Reader
	}
	if dependencies.Now == nil {
		dependencies.Now = func() time.Time { return time.Now().UTC() }
	}
	if dependencies.PathExists == nil {
		dependencies.PathExists = func(path string) (bool, error) {
			_, err := os.Stat(path)
			if err == nil {
				return true, nil
			}
			if errors.Is(err, os.ErrNotExist) {
				return false, nil
			}
			return false, err
		}
	}
	if dependencies.MakeDirectory == nil {
		dependencies.MakeDirectory = func(path string, mode os.FileMode) error {
			if err := os.MkdirAll(path, mode); err != nil {
				return err
			}
			return secureInstallDirectory(path)
		}
	}
	if dependencies.AcquireInstallLock == nil {
		dependencies.AcquireInstallLock = acquireInstallLock
	}
	if dependencies.WriteRuntimeEnv == nil {
		dependencies.WriteRuntimeEnv = writePrivateFileAtomicNoReplace
	}
	if dependencies.WriteInstallState == nil {
		dependencies.WriteInstallState = writePrivateFileAtomicNoReplace
	}
	if dependencies.WriteManifest == nil {
		dependencies.WriteManifest = writePrivateFileAtomicNoReplace
	}
	if dependencies.WriteDeploymentFile == nil {
		dependencies.WriteDeploymentFile = writeDeploymentFileAtomicNoReplace
	}
	if dependencies.RemovePath == nil {
		dependencies.RemovePath = os.Remove
	}
	return dependencies
}

func validateClusterJoinOptions(options ClusterJoinOptions) error {
	if err := validateServerURL(options.Server); err != nil {
		return err
	}
	if strings.TrimSpace(options.Token) == "" {
		return errors.New("cluster join token is required")
	}
	if options.Mode != config.DeploymentModeDocker && options.Mode != config.DeploymentModeNative {
		return errors.New("cluster join mode must be docker or native")
	}
	if err := config.ValidateApplicationVersion(options.ApplicationVersion); err != nil {
		return fmt.Errorf("validate cluster application version: %w", err)
	}
	return nil
}

func postClusterJSON(ctx context.Context, client *http.Client, endpoint string, requestValue, responseValue any) error {
	requestBody, err := json.Marshal(requestValue)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if client == nil {
		client = http.DefaultClient
	}
	strictClient := *client
	strictClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("cluster enrollment redirects are not allowed")
	}
	response, err := strictClient.Do(request)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, maxClusterHTTPBodyBytes+1)
	content, err := io.ReadAll(limited)
	if err != nil {
		return errors.New("read cluster response failed")
	}
	if len(content) > maxClusterHTTPBodyBytes {
		return errors.New("cluster response is too large")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("cluster API returned HTTP %d", response.StatusCode)
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil || len(envelope.Data) == 0 {
		return errors.New("cluster API returned an invalid response")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("cluster API response must contain one JSON object")
	}
	decoder = json.NewDecoder(bytes.NewReader(envelope.Data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(responseValue); err != nil {
		return errors.New("cluster API returned invalid data")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("cluster API data must contain one JSON object")
	}
	return nil
}

func validateRemoteChallenge(challenge domaincluster.EnrollmentChallenge, request domaincluster.CreateChallengeRequest, now time.Time) error {
	if challenge.Protocol != clusterservice.EnrollmentProtocolV1 || challenge.TokenID != request.TokenID || challenge.NodeID != request.NodeID || challenge.ClientPublicKey != request.NodePublicKey || challenge.ApplicationVersion != request.ApplicationVersion || challenge.RuntimeSchemaVersion != request.RuntimeSchemaVersion || challenge.ConfigRevision <= 0 || challenge.InstallationID == "" || challenge.Role == "" || challenge.ServerPublicKey == "" || challenge.ServerNonce == "" || !challenge.ExpiresAt.After(now.UTC()) {
		return errors.New("cluster challenge metadata is invalid or incompatible")
	}
	if _, err := uuid.Parse(challenge.ChallengeID); err != nil {
		return errors.New("cluster challenge ID is invalid")
	}
	return nil
}

func validateJoinResponse(joined domaincluster.JoinResponse, challenge domaincluster.EnrollmentChallenge, now time.Time) error {
	if joined.Protocol != clusterservice.EnrollmentProtocolV1 || joined.InstallationID != challenge.InstallationID || joined.NodeID != challenge.NodeID || joined.Role != domaincluster.NodeRole(challenge.Role) || joined.ApplicationVersion != challenge.ApplicationVersion || joined.RuntimeSchemaVersion != challenge.RuntimeSchemaVersion || joined.ConfigRevision != challenge.ConfigRevision || !joined.ExpiresAt.Equal(challenge.ExpiresAt) || !joined.ExpiresAt.After(now.UTC()) {
		return errors.New("cluster join response binding is invalid")
	}
	return nil
}

func validateRemoteRuntimeValues(role domaincluster.NodeRole, values map[string]string) error {
	allowed := clusterservice.RuntimeKeysForRole(role)
	if len(allowed) == 0 {
		return fmt.Errorf("unsupported joined role %q", role)
	}
	for key := range values {
		if !slices.Contains(allowed, key) {
			return fmt.Errorf("cluster runtime contains forbidden key %s", key)
		}
	}
	return nil
}

func buildJoinedPlan(options ClusterJoinOptions, role config.DeploymentRole, applicationVersion, nodeID, storageDriver, publicAPIURL string) (InstallPlan, error) {
	if role != config.DeploymentRoleAPI && role != config.DeploymentRoleWorker && role != config.DeploymentRoleWeb {
		return InstallPlan{}, fmt.Errorf("unsupported joined role %q", role)
	}
	if role != config.DeploymentRoleWeb && storageDriver == "" {
		storageDriver = "s3"
	}
	return BuildInstallPlan(InstallInput{
		Mode: options.Mode, Profile: config.DeploymentProfileCore, Topology: config.DeploymentTopologyCluster,
		Role: role, RuntimeDir: filepath.Clean(defaultString(options.RuntimeDir, ".")), StorageDriver: storageDriver,
		PublicAPIURL: publicAPIURL, InstallationInitialized: true, ApplicationVersion: applicationVersion,
		ImageRegistry: options.ImageRegistry, ImageTag: options.ImageTag, ReleaseVersion: options.ReleaseVersion,
		APIPort: options.APIPort, GatewayPort: options.GatewayPort, UserWebPort: options.UserWebPort,
		AdminWebPort: options.AdminWebPort, DocsWebPort: options.DocsWebPort,
	})
}

func joinedRuntimeValues(options ClusterJoinOptions, plan InstallPlan, joined domaincluster.JoinResponse, remote map[string]string) map[string]string {
	values := make(map[string]string, len(remote)+24)
	for key, value := range remote {
		values[key] = value
	}
	values["RUNTIME_SCHEMA_VERSION"] = strconv.Itoa(joined.RuntimeSchemaVersion)
	values["DEPLOYMENT_MODE"] = string(plan.Mode)
	values["DEPLOYMENT_PROFILE"] = string(plan.Profile)
	values["DEPLOYMENT_TOPOLOGY"] = string(plan.Topology)
	values["DEPLOYMENT_ROLE"] = string(plan.Role)
	values["DEPLOYMENT_MODULES"] = componentsCSV(plan.Components)
	values["POSTGRES_MANAGED"] = "false"
	values["REDIS_MANAGED"] = "false"
	values["OBJECT_STORAGE_MANAGED"] = "false"
	values["SETUP_COMPLETED"] = "true"
	values["INSTALLATION_ID"] = joined.InstallationID
	values["CLUSTER_NODE_ID"] = joined.NodeID
	values["CONFIG_REVISION"] = strconv.FormatInt(joined.ConfigRevision, 10)
	values["APPLICATION_VERSION"] = joined.ApplicationVersion
	values["API_PORT"] = plan.APIPort
	values["GATEWAY_PORT"] = plan.GatewayPort
	values["USER_WEB_PORT"] = plan.UserWebPort
	values["ADMIN_WEB_PORT"] = plan.AdminWebPort
	values["DOCS_WEB_PORT"] = plan.DocsWebPort
	if plan.Mode == config.DeploymentModeDocker {
		values["IMAGE_REGISTRY"] = options.ImageRegistry
		values["IMAGE_TAG"] = defaultString(options.ImageTag, joined.ApplicationVersion)
	} else {
		values["RELEASE_VERSION"] = defaultString(options.ReleaseVersion, joined.ApplicationVersion)
	}
	return values
}

func renderJoinedManifest(plan InstallPlan, joined domaincluster.JoinResponse, files []DeploymentFile, now time.Time) ([]byte, error) {
	fileHashes := make(map[string]string, len(files))
	for _, file := range files {
		digest := sha256.Sum256(file.Content)
		fileHashes[filepath.ToSlash(file.RelativePath)] = fmt.Sprintf("%x", digest)
	}
	content, err := json.MarshalIndent(deploymentManifest{
		SchemaVersion: 1, InstallationID: joined.InstallationID, CreatedAt: now.UTC(), Plan: plan, Files: fileHashes,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("render joined deployment manifest: %w", err)
	}
	return append(content, '\n'), nil
}

func randomClusterNodeID(entropy io.Reader) (string, error) {
	value := make([]byte, 16)
	if _, err := io.ReadFull(entropy, value); err != nil {
		return "", fmt.Errorf("generate cluster node ID: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	id, err := uuid.FromBytes(value)
	if err != nil {
		return "", fmt.Errorf("generate cluster node ID: %w", err)
	}
	return id.String(), nil
}

func clusterJoinTokenID(credential string) (string, error) {
	parts := strings.Split(strings.TrimSpace(credential), ".")
	if len(parts) != 4 || parts[0] != "pgjoin" || parts[1] != "v1" {
		return "", errors.New("cluster join token is invalid")
	}
	id, err := uuid.Parse(parts[2])
	if err != nil || id.String() != parts[2] {
		return "", errors.New("cluster join token is invalid")
	}
	return id.String(), nil
}

func componentsStrings(components []Component) []string {
	result := make([]string, 0, len(components))
	for _, component := range components {
		result = append(result, string(component))
	}
	return result
}
