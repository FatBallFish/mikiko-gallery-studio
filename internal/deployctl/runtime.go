package deployctl

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
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

type RuntimeArtifacts struct {
	RuntimeEnv   []byte
	InstallState setup.InstallState
	Manifest     []byte
	SetupToken   string
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
	SchemaVersion  int         `json:"schema_version"`
	InstallationID string      `json:"installation_id"`
	CreatedAt      time.Time   `json:"created_at"`
	Plan           InstallPlan `json:"plan"`
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
		"PUBLIC_API_URL":                           plan.PublicAPIURL,
		"AUTH_ACCESS_TOKEN_SECRET":                 derivedSecret(root, "auth-access-token"),
		"API_KEY_SIGNING_SECRET_ENCRYPTION_KEY":    derivedSecret(root, "api-key-encryption"),
		"CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY":   derivedSecret(root, "cashier-encryption"),
		"PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY": derivedSecret(root, "secure-config-encryption"),
		"PROMPT_OPTIMIZATION_QUOTE_SIGNING_KEY":    derivedSecret(root, "prompt-quote-signing"),
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
	manifest, err := json.MarshalIndent(deploymentManifest{SchemaVersion: 1, InstallationID: installationID, CreatedAt: now, Plan: plan}, "", "  ")
	if err != nil {
		return RuntimeArtifacts{}, fmt.Errorf("render deployment manifest: %w", err)
	}
	manifest = append(manifest, '\n')
	return RuntimeArtifacts{RuntimeEnv: runtimeEnv, InstallState: state, Manifest: manifest, SetupToken: setupToken}, nil
}

func populateManagedResources(values map[string]string, root []byte, postgres, redis, storage bool) {
	if postgres {
		postgresPassword := derivedSecret(root, "postgres-password")
		values["POSTGRES_DATABASE"] = "app"
		values["POSTGRES_USER"] = "app"
		values["POSTGRES_PASSWORD"] = postgresPassword
		values["DATABASE_URL"] = (&url.URL{Scheme: "postgres", User: url.UserPassword("app", postgresPassword), Host: "postgres:5432", Path: "/app", RawQuery: "sslmode=disable"}).String()
	}
	if redis {
		redisPassword := derivedSecret(root, "redis-password")
		values["REDIS_PASSWORD"] = redisPassword
		values["REDIS_URL"] = (&url.URL{Scheme: "redis", User: url.UserPassword("", redisPassword), Host: "redis:6379", Path: "/0"}).String()
		values["REDIS_KEY_PREFIX"] = "app"
	}
	if storage {
		minioPassword := derivedSecret(root, "minio-root-password")
		storageAccessIDSecret := derivedSecret(root, "minio-app-access-id")
		values["MINIO_ROOT_USER"] = "minio-admin"
		values["MINIO_ROOT_PASSWORD"] = minioPassword
		values["STORAGE_DRIVER"] = "s3"
		values["STORAGE_S3_ENDPOINT"] = "http://minio:9000"
		values["STORAGE_S3_REGION"] = "us-east-1"
		values["STORAGE_S3_BUCKET"] = "app-assets"
		values["STORAGE_S3_ACCESS_KEY_ID"] = "app-" + storageAccessIDSecret[:20]
		values["STORAGE_S3_SECRET_ACCESS_KEY"] = derivedSecret(root, "minio-app-secret")
		values["STORAGE_S3_FORCE_PATH_STYLE"] = "true"
	}
}

func derive(root []byte, purpose string) []byte {
	digest := hmac.New(sha256.New, root)
	_, _ = digest.Write([]byte("deployctl-runtime-v1:" + purpose))
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
