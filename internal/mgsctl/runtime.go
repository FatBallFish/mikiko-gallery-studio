package mgsctl

import (
	"bytes"
	"crypto/hmac"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	deploymentassets "github.com/fatballfish/pic-gallery/deployments"
	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

type RuntimeArtifacts struct {
	RuntimeEnv      []byte
	InstallState    setup.InstallState
	Manifest        []byte
	SetupToken      string
	DeploymentFiles []DeploymentFile
}

type DeploymentFile struct {
	RelativePath string
	Content      []byte
}

func (RuntimeArtifacts) String() string {
	return "RuntimeArtifacts{RuntimeEnv:<redacted>, InstallState:<metadata>, Manifest:<metadata>, SetupToken:<redacted>}"
}
func (artifacts RuntimeArtifacts) GoString() string { return artifacts.String() }
func (artifacts RuntimeArtifacts) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		InstallState setup.InstallState `json:"install_state"`
		RuntimeEnv   string             `json:"runtime_env"`
		Manifest     string             `json:"manifest"`
		SetupToken   string             `json:"setup_token"`
	}{InstallState: artifacts.InstallState, RuntimeEnv: "REDACTED", Manifest: "REDACTED", SetupToken: "REDACTED"})
}

type deploymentManifest struct {
	SchemaVersion  int               `json:"schema_version"`
	InstallationID string            `json:"installation_id"`
	CreatedAt      time.Time         `json:"created_at"`
	Plan           InstallPlan       `json:"plan"`
	Files          map[string]string `json:"files,omitempty"`
}

func BuildRuntimeArtifacts(plan InstallPlan, random io.Reader, now time.Time) (RuntimeArtifacts, error) {
	if err := ValidateInstallPlan(plan); err != nil {
		return RuntimeArtifacts{}, fmt.Errorf("validate install plan: %w", err)
	}
	if plan.RequiresEnrollment {
		return RuntimeArtifacts{}, fmt.Errorf("joined role %q requires cluster enrollment artifacts", plan.Role)
	}
	if plan.Role != config.DeploymentRoleSingle && plan.Role != config.DeploymentRoleControl {
		return RuntimeArtifacts{}, fmt.Errorf("role %q cannot create pending setup artifacts", plan.Role)
	}
	if random == nil {
		random = cryptorand.Reader
	}
	root := make([]byte, 32)
	if _, err := io.ReadFull(random, root); err != nil {
		return RuntimeArtifacts{}, fmt.Errorf("generate installation entropy: %w", err)
	}
	defer clear(root)
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	installationID := derivedUUID(root, "installation-id")
	setupRaw := derive(root, "setup-token")
	setupToken, err := setup.GenerateSetupToken(bytes.NewReader(setupRaw))
	clear(setupRaw)
	if err != nil {
		return RuntimeArtifacts{}, err
	}

	postgresManaged := slices.Contains(plan.Components, ComponentPostgres)
	redisManaged := slices.Contains(plan.Components, ComponentRedis)
	storageManaged := slices.Contains(plan.Components, ComponentMinIO)
	values := map[string]string{
		"DEPLOYMENT_MODE": string(plan.Mode), "DEPLOYMENT_PROFILE": string(plan.Profile),
		"DEPLOYMENT_TOPOLOGY": string(plan.Topology), "DEPLOYMENT_ROLE": string(plan.Role),
		"DEPLOYMENT_MODULES":     componentsCSV(plan.Components),
		"POSTGRES_MANAGED":       strconv.FormatBool(postgresManaged),
		"REDIS_MANAGED":          strconv.FormatBool(redisManaged),
		"OBJECT_STORAGE_MANAGED": strconv.FormatBool(storageManaged),
		"SETUP_COMPLETED":        "false", "SETUP_TOKEN": setupToken, "SETUP_TOKEN_VERSION": "1",
		"STORAGE_DRIVER": plan.StorageDriver, "INSTALLATION_ID": installationID,
		"APPLICATION_VERSION": plan.ApplicationVersion, "API_PORT": plan.APIPort,
		"GATEWAY_PORT": plan.GatewayPort, "USER_WEB_PORT": plan.UserWebPort,
		"ADMIN_WEB_PORT": plan.AdminWebPort, "DOCS_WEB_PORT": plan.DocsWebPort,
		"MONITORING_PORT":                          plan.MonitoringPort,
		"PUBLIC_API_URL":                           plan.PublicAPIURL,
		"AUTH_ACCESS_TOKEN_SECRET":                 derivedSecret(root, "auth-access-token"),
		"API_KEY_SIGNING_SECRET_ENCRYPTION_KEY":    derivedSecret(root, "api-key-encryption"),
		"CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY":   derivedSecret(root, "cashier-encryption"),
		"PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY": derivedSecret(root, "secure-config-encryption"),
		"PROMPT_OPTIMIZATION_QUOTE_SIGNING_KEY":    derivedSecret(root, "prompt-quote-signing"),
		"CLUSTER_ENROLLMENT_SEAL_KEY":              derivedSecret(root, "cluster-enrollment-seal"),
	}
	if plan.Topology == config.DeploymentTopologyCluster {
		values["CLUSTER_NODE_ID"] = derivedUUID(root, "cluster-node-id")
		values["CONFIG_REVISION"] = "1"
	}
	if plan.Mode == config.DeploymentModeDocker {
		values["IMAGE_REGISTRY"] = plan.ImageRegistry
		values["IMAGE_TAG"] = plan.ImageTag
	} else {
		values["RELEASE_VERSION"] = plan.ReleaseVersion
	}
	if plan.StorageDriver == "local" {
		values["STORAGE_LOCAL_ROOT"] = "./data/storage"
		values["STORAGE_SHARED_VOLUME"] = "true"
	}
	if postgresManaged || redisManaged || storageManaged {
		populateManagedResources(values, root, postgresManaged, redisManaged, storageManaged)
	}
	runtimeEnv, err := config.RenderRuntimeEnv(config.DefaultRuntimeSchema(), values, nil)
	if err != nil {
		return RuntimeArtifacts{}, fmt.Errorf("render runtime env: %w", err)
	}
	state := setup.InstallState{
		SchemaVersion: setup.CurrentInstallStateSchemaVersion, InstallationID: installationID,
		DeploymentRole: plan.Role, Phase: setup.InstallPhasePending, UpdatedAt: now,
	}
	deploymentFiles, err := buildDeploymentFiles(plan)
	if err != nil {
		return RuntimeArtifacts{}, err
	}
	fileHashes := make(map[string]string, len(deploymentFiles))
	for _, file := range deploymentFiles {
		digest := sha256.Sum256(file.Content)
		fileHashes[filepath.ToSlash(file.RelativePath)] = fmt.Sprintf("%x", digest)
	}
	manifest, err := json.MarshalIndent(deploymentManifest{SchemaVersion: 1, InstallationID: installationID, CreatedAt: now, Plan: plan, Files: fileHashes}, "", "  ")
	if err != nil {
		return RuntimeArtifacts{}, fmt.Errorf("render deployment manifest: %w", err)
	}
	manifest = append(manifest, '\n')
	return RuntimeArtifacts{RuntimeEnv: runtimeEnv, InstallState: state, Manifest: manifest, SetupToken: setupToken, DeploymentFiles: deploymentFiles}, nil
}

func buildDeploymentFiles(plan InstallPlan) ([]DeploymentFile, error) {
	if plan.Mode != config.DeploymentModeDocker {
		return nil, nil
	}
	nginxConfig, err := materializeNginxConfig(plan, deploymentassets.NginxDefault())
	if err != nil {
		return nil, err
	}
	prometheusConfig := deploymentassets.Prometheus()
	oldPrometheusTarget := []byte("host.docker.internal:8080")
	newPrometheusTarget := []byte("api:" + plan.APIPort)
	if bytes.Count(prometheusConfig, oldPrometheusTarget) != 1 {
		return nil, fmt.Errorf("embedded Prometheus API target is not canonical")
	}
	prometheusConfig = bytes.Replace(prometheusConfig, oldPrometheusTarget, newPrometheusTarget, 1)
	return []DeploymentFile{
		{RelativePath: "compose.yml", Content: deploymentassets.DockerCompose()},
		{RelativePath: filepath.Join("assets", "nginx-default.conf"), Content: nginxConfig},
		{RelativePath: filepath.Join("assets", "minio-init.sh"), Content: deploymentassets.MinIOInit()},
		{RelativePath: filepath.Join("assets", "postgres-init.sh"), Content: deploymentassets.PostgresInit()},
		{RelativePath: filepath.Join("assets", "prometheus.yml"), Content: prometheusConfig},
	}, nil
}

func materializeNginxConfig(plan InstallPlan, template []byte) ([]byte, error) {
	const canonicalUpstream = "server api:8080;"
	if bytes.Count(template, []byte(canonicalUpstream)) != 1 {
		return nil, fmt.Errorf("embedded Nginx API upstream is not canonical")
	}
	if plan.Role != config.DeploymentRoleWeb {
		return bytes.Replace(template, []byte(canonicalUpstream), []byte("server api:"+plan.APIPort+";"), 1), nil
	}
	publicAPI, err := url.Parse(plan.PublicAPIURL)
	if err != nil {
		return nil, fmt.Errorf("parse public API URL: %w", err)
	}
	upstreamAddress := publicAPI.Host
	if publicAPI.Port() == "" {
		if publicAPI.Scheme == "https" {
			upstreamAddress += ":443"
		} else {
			upstreamAddress += ":80"
		}
	}
	configContent := bytes.Replace(template, []byte(canonicalUpstream), []byte("server "+upstreamAddress+";"), 1)
	apiStart := bytes.Index(configContent, []byte("    location = /healthz {"))
	apiEnd := bytes.Index(configContent, []byte("    location = /developer-docs {"))
	if apiStart < 0 || apiEnd <= apiStart {
		return nil, fmt.Errorf("embedded Nginx API proxy section is not canonical")
	}
	apiSection := configContent[apiStart:apiEnd]
	const canonicalProxy = "proxy_pass http://pic_gallery_api;"
	if bytes.Count(apiSection, []byte(canonicalProxy)) != 8 {
		return nil, fmt.Errorf("embedded Nginx API proxy count is not canonical")
	}
	basePath := strings.TrimSuffix(publicAPI.EscapedPath(), "/")
	proxyTarget := "proxy_pass " + publicAPI.Scheme + "://pic_gallery_api" + basePath + "$request_uri;"
	apiSection = bytes.ReplaceAll(apiSection, []byte(canonicalProxy), []byte(proxyTarget))
	apiSection = bytes.ReplaceAll(apiSection, []byte("proxy_set_header Host $host;"), []byte("proxy_set_header Host "+publicAPI.Host+";"))
	if publicAPI.Scheme == "https" {
		apiSection = bytes.ReplaceAll(apiSection, []byte(proxyTarget), []byte(proxyTarget+"\n        proxy_ssl_server_name on;\n        proxy_ssl_name "+publicAPI.Hostname()+";"))
	}
	materialized := make([]byte, 0, len(configContent)+len(apiSection))
	materialized = append(materialized, configContent[:apiStart]...)
	materialized = append(materialized, apiSection...)
	materialized = append(materialized, configContent[apiEnd:]...)
	return materialized, nil
}

func deploymentFilePaths(plan InstallPlan) ([]string, error) {
	files, err := buildDeploymentFiles(plan)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, filepath.Join(plan.RuntimeDir, file.RelativePath))
	}
	return paths, nil
}

func populateManagedResources(values map[string]string, root []byte, postgres, redis, storage bool) {
	if postgres {
		postgresPassword := "pg_" + derivedSecret(root, "postgres-password")
		values["POSTGRES_DATABASE"] = "app"
		values["POSTGRES_USER"] = "app"
		values["POSTGRES_PASSWORD"] = postgresPassword
		values["DATABASE_URL"] = (&url.URL{Scheme: "postgres", User: url.UserPassword("app", postgresPassword), Host: "postgres:5432", Path: "/app", RawQuery: "sslmode=disable"}).String()
	}
	if redis {
		redisPassword := "redis_" + derivedSecret(root, "redis-password")
		values["REDIS_PASSWORD"] = redisPassword
		values["REDIS_URL"] = (&url.URL{Scheme: "redis", User: url.UserPassword("", redisPassword), Host: "redis:6379", Path: "/0"}).String()
		values["REDIS_KEY_PREFIX"] = "app"
	}
	if storage {
		minioPassword := "minio_" + derivedSecret(root, "minio-root-password")
		storageAccessIDSecret := derivedSecret(root, "minio-app-access-id")
		values["MINIO_ROOT_USER"] = "minio-admin"
		values["MINIO_ROOT_PASSWORD"] = minioPassword
		values["STORAGE_DRIVER"] = "s3"
		values["STORAGE_S3_ENDPOINT"] = "http://minio:9000"
		values["STORAGE_S3_REGION"] = "us-east-1"
		values["STORAGE_S3_BUCKET"] = "app-assets"
		values["STORAGE_S3_ACCESS_KEY_ID"] = "app-" + storageAccessIDSecret[:20]
		values["STORAGE_S3_SECRET_ACCESS_KEY"] = "s3_" + derivedSecret(root, "minio-app-secret")
		values["STORAGE_S3_FORCE_PATH_STYLE"] = "true"
	}
}

func derive(root []byte, purpose string) []byte {
	digest := hmac.New(sha256.New, root)
	_, _ = digest.Write([]byte("mgsctl-runtime-v1:" + purpose))
	return digest.Sum(nil)
}

func derivedSecret(root []byte, purpose string) string {
	derived := derive(root, purpose)
	defer clear(derived)
	return base64.RawURLEncoding.EncodeToString(derived)
}

func derivedUUID(root []byte, purpose string) string {
	derived := derive(root, purpose)
	defer clear(derived)
	bytes16 := derived[:16]
	bytes16[6] = (bytes16[6] & 0x0f) | 0x40
	bytes16[8] = (bytes16[8] & 0x3f) | 0x80
	id, _ := uuid.FromBytes(bytes16)
	return id.String()
}

func componentsCSV(components []Component) string {
	values := make([]string, 0, len(components))
	for _, component := range components {
		values = append(values, string(component))
	}
	return strings.Join(values, ",")
}
