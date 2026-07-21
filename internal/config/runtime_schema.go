package config

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const CurrentRuntimeSchemaVersion = 1

type DeploymentMode string

const (
	DeploymentModeDocker DeploymentMode = "docker"
	DeploymentModeNative DeploymentMode = "native"
)

type DeploymentProfile string

const (
	DeploymentProfileFull   DeploymentProfile = "full"
	DeploymentProfileCore   DeploymentProfile = "core"
	DeploymentProfileCustom DeploymentProfile = "custom"
)

type DeploymentTopology string

const (
	DeploymentTopologySingle  DeploymentTopology = "single"
	DeploymentTopologyCluster DeploymentTopology = "cluster"
)

type DeploymentRole string

const (
	DeploymentRoleSingle  DeploymentRole = "single"
	DeploymentRoleControl DeploymentRole = "control"
	DeploymentRoleAPI     DeploymentRole = "api"
	DeploymentRoleWorker  DeploymentRole = "worker"
	DeploymentRoleWeb     DeploymentRole = "web"
)

type FieldOwner string

const (
	FieldOwnerDeployctl   FieldOwner = "deployctl"
	FieldOwnerSetup       FieldOwner = "setup"
	FieldOwnerApplication FieldOwner = "application"
)

type DeploymentContext struct {
	Mode           DeploymentMode
	Profile        DeploymentProfile
	Topology       DeploymentTopology
	Role           DeploymentRole
	StorageDriver  string
	SetupCompleted bool
}

type RuntimeField struct {
	Key             string
	Group           string
	DescriptionZH   string
	DescriptionEN   string
	Example         string
	DefaultValue    string
	Secret          bool
	Owner           FieldOwner
	RestartRequired bool
	RequiredWhen    func(DeploymentContext) bool
	Validate        func(string) error
}

type RuntimeSchema struct {
	Version int
	Fields  []RuntimeField
}

var (
	runtimeFieldKeyPattern    = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	applicationVersionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+:-]{0,127}$`)
)

// ValidateApplicationVersion ensures build identifiers are safe to persist and
// include in operational output without accepting whitespace or control bytes.
func ValidateApplicationVersion(value string) error {
	if !applicationVersionPattern.MatchString(value) {
		return fmt.Errorf("application version must be 1 to 128 safe identifier characters")
	}
	return nil
}

func DefaultRuntimeSchema() RuntimeSchema {
	return RuntimeSchema{
		Version: CurrentRuntimeSchemaVersion,
		Fields: []RuntimeField{
			field("RUNTIME_SCHEMA_VERSION", "schema", "运行时配置 Schema 版本。升级工具使用它判断需要补充的字段。", "Runtime configuration schema version. Upgrade tooling uses it to add newly introduced fields.", "1", "1", FieldOwnerDeployctl, requiredAlways, validatePositiveInteger),
			field("DEPLOYMENT_MODE", "deployment", "应用运行方式，可选 docker 或 native。", "Application runtime mode: docker or native.", "docker", "", FieldOwnerDeployctl, requiredAlways, validateDeploymentMode),
			field("DEPLOYMENT_PROFILE", "deployment", "服务规格，可选 full、core 或 custom。原生部署不支持 full。", "Service profile: full, core, or custom. Native deployments do not support full.", "core", "", FieldOwnerDeployctl, requiredAlways, validateDeploymentProfile),
			field("DEPLOYMENT_TOPOLOGY", "deployment", "部署拓扑，可选 single 或 cluster；集群拓扑中的后端节点必须使用 S3 对象存储。", "Deployment topology: single or cluster. Backend nodes in a cluster topology must use S3 object storage.", "single", "", FieldOwnerDeployctl, requiredAlways, validateDeploymentTopology),
			field("DEPLOYMENT_ROLE", "deployment", "当前节点角色，可选 single、control、api、worker 或 web。", "Current node role: single, control, api, worker, or web.", "single", "", FieldOwnerDeployctl, requiredAlways, validateDeploymentRole),
			field("DEPLOYMENT_MODULES", "deployment", "逗号分隔的本机模块清单，由部署工具维护。", "Comma-separated module list for this host, managed by the deployment tool.", "api,worker,user-web,admin-web,docs-web,gateway", "", FieldOwnerDeployctl, requiredAlways, validateNonEmpty),

			field("POSTGRES_MANAGED", "ownership", "PostgreSQL 是否由当前 Docker 完整部署管理。", "Whether PostgreSQL is managed by this Docker full deployment.", "false", "false", FieldOwnerDeployctl, requiredAlways, validateBool),
			field("REDIS_MANAGED", "ownership", "Redis 是否由当前 Docker 完整部署管理。", "Whether Redis is managed by this Docker full deployment.", "false", "false", FieldOwnerDeployctl, requiredAlways, validateBool),
			field("OBJECT_STORAGE_MANAGED", "ownership", "对象存储是否由当前 Docker 完整部署管理。", "Whether object storage is managed by this Docker full deployment.", "false", "false", FieldOwnerDeployctl, requiredAlways, validateBool),

			field("SETUP_COMPLETED", "setup", "首次初始化是否已经完整提交。不得通过手工修改此值绕过初始化。", "Whether first-run setup has been fully committed. Do not edit this value to bypass setup.", "false", "false", FieldOwnerApplication, requiredAlways, validateBool),
			secretField("SETUP_TOKEN", "setup", "首次初始化访问凭证；初始化完成前可重复使用，完成后会被移除并永久失效。", "First-run setup access credential; reusable until setup completes, then removed and permanently invalidated.", FieldOwnerDeployctl, requiredSetupAuthority),
			field("SETUP_TOKEN_VERSION", "setup", "初始化访问凭证的单调递增版本。重置凭证时版本加一，用于立即作废旧凭证和会话；初始化完成后保留最后版本。", "Monotonically increasing setup credential version. Reset increments it to invalidate old credentials and sessions immediately; the final version is retained after setup completes.", "1", "1", FieldOwnerDeployctl, requiredSetupAuthorityRole, validateRequiredPositiveUint64),

			field("POSTGRES_DATABASE", "managed middleware", "Docker 完整模式创建的 PostgreSQL 数据库名，仅由部署工具维护。", "PostgreSQL database name created in Docker full mode and managed only by the deployment tool.", "app", "", FieldOwnerDeployctl, requiredDockerFull, validateNonEmpty),
			field("POSTGRES_USER", "managed middleware", "Docker 完整模式创建的 PostgreSQL 应用用户。", "PostgreSQL application user created in Docker full mode.", "app", "", FieldOwnerDeployctl, requiredDockerFull, validateNonEmpty),
			secretField("POSTGRES_PASSWORD", "managed middleware", "Docker 完整模式创建的 PostgreSQL 用户密码。", "PostgreSQL user password created in Docker full mode.", FieldOwnerDeployctl, requiredDockerFull),
			secretField("REDIS_PASSWORD", "managed middleware", "Docker 完整模式创建的 Redis 访问密码。", "Redis password created in Docker full mode.", FieldOwnerDeployctl, requiredDockerFull),
			field("MINIO_ROOT_USER", "managed middleware", "Docker 完整模式创建的 MinIO 管理用户，仅用于资源准备。", "MinIO administrative user created in Docker full mode for resource provisioning.", "minio-admin", "", FieldOwnerDeployctl, requiredDockerFull, validateNonEmpty),
			secretField("MINIO_ROOT_PASSWORD", "managed middleware", "Docker 完整模式创建的 MinIO 管理密码，仅用于资源准备。", "MinIO administrative password created in Docker full mode for resource provisioning.", FieldOwnerDeployctl, requiredDockerFull),

			secretValidatedField("DATABASE_URL", "database", "PostgreSQL 连接地址。账号需要具备当前数据库的建表、索引和读写权限，URL 内密码必须进行 percent encoding。", "PostgreSQL connection URL. The account must be able to create schema objects and read/write data, and URL passwords must be percent-encoded.", FieldOwnerSetup, requiredBackend, validateConnectionURL("postgres", "postgresql")),
			field("DATABASE_MAX_OPEN_CONNS", "database", "数据库连接池最大打开连接数；留空使用应用默认值。", "Maximum open database connections; leave empty to use the application default.", "25", "", FieldOwnerSetup, requiredNever, validateOptionalPositiveInteger),
			field("DATABASE_MAX_IDLE_CONNS", "database", "数据库连接池最大空闲连接数；留空使用应用默认值。", "Maximum idle database connections; leave empty to use the application default.", "10", "", FieldOwnerSetup, requiredNever, validateOptionalNonNegativeInteger),
			field("DATABASE_CONN_MAX_LIFETIME", "database", "数据库连接最大复用时间，使用 Go duration 格式。", "Maximum database connection lifetime in Go duration format.", "30m", "", FieldOwnerSetup, requiredNever, validateOptionalDurationLike),

			secretValidatedField("REDIS_URL", "redis", "Redis 连接地址，可能包含密码；所有 API 和 Worker 节点必须连接同一实例或集群。", "Redis connection URL, which may contain a password. All API and Worker nodes must use the same instance or cluster.", FieldOwnerSetup, requiredBackend, validateConnectionURL("redis", "rediss")),
			field("REDIS_KEY_PREFIX", "redis", "Redis 键名前缀；同一 Redis 上部署多个实例时必须保持唯一。", "Redis key prefix; it must be unique when several installations share Redis.", "app", "app", FieldOwnerSetup, requiredBackend, validateNonEmpty),

			field("STORAGE_DRIVER", "storage", "对象存储驱动，可选 local 或 s3；多节点部署必须使用 s3。", "Object storage driver: local or s3. Multi-node deployments require s3.", "s3", "", FieldOwnerSetup, requiredBackend, validateStorageDriver),
			field("STORAGE_LOCAL_ROOT", "storage", "单节点 local 存储根目录，API 与 Worker 必须可访问同一路径。", "Root directory for single-node local storage; API and Worker must access the same path.", "./data/storage", "", FieldOwnerSetup, requiredLocalStorage, validateNonEmpty),
			field("STORAGE_PUBLIC_BASE_URL", "storage", "资源公开访问基础地址；留空时由 API 按部署方式生成访问地址。", "Public asset base URL; leave empty for the API to derive it from the deployment.", "http://127.0.0.1:8080/assets", "", FieldOwnerSetup, requiredNever, validateOptionalHTTPURL),
			field("STORAGE_SHARED_VOLUME", "storage", "local 存储目录是否在本节点 API 与 Worker 间共享。", "Whether the local storage directory is shared by API and Worker on this host.", "true", "false", FieldOwnerSetup, requiredLocalStorage, validateBool),
			field("STORAGE_S3_ENDPOINT", "storage", "S3 兼容服务地址。当前版本的所有 S3 部署均必须填写完整的 HTTP 或 HTTPS 地址。", "S3-compatible endpoint. Every S3 deployment in the current release must provide a complete HTTP or HTTPS URL.", "http://127.0.0.1:9000", "", FieldOwnerSetup, requiredS3Storage, validateOptionalHTTPURL),
			field("STORAGE_S3_REGION", "storage", "S3 区域。服务不校验区域时也必须填写其约定值，例如 us-east-1。", "S3 region. Providers that do not enforce regions still require their conventional value, such as us-east-1.", "us-east-1", "", FieldOwnerSetup, requiredS3Storage, validateOptionalNonEmpty),
			field("STORAGE_S3_BUCKET", "storage", "存放任务输入和生成结果的 S3 Bucket 名称。", "S3 bucket containing task inputs and generated outputs.", "app-assets", "", FieldOwnerSetup, requiredS3Storage, validateNonEmpty),
			secretField("STORAGE_S3_ACCESS_KEY_ID", "storage", "S3 访问密钥 ID。", "S3 access key ID.", FieldOwnerSetup, requiredS3Storage),
			secretField("STORAGE_S3_SECRET_ACCESS_KEY", "storage", "S3 访问密钥内容。", "S3 secret access key.", FieldOwnerSetup, requiredS3Storage),
			field("STORAGE_S3_FORCE_PATH_STYLE", "storage", "是否强制使用 path-style S3 地址；MinIO 通常需要开启。", "Whether to force path-style S3 addressing; MinIO commonly requires it.", "true", "false", FieldOwnerSetup, requiredNever, validateBool),
			field("STORAGE_S3_PREFIX", "storage", "当前安装在 Bucket 内使用的对象键前缀。", "Object-key prefix used by this installation inside the bucket.", "production/assets", "", FieldOwnerSetup, requiredNever, validateOptionalNonEmpty),

			secretField("AUTH_ACCESS_TOKEN_SECRET", "application secrets", "签发用户和管理员访问令牌的密钥；初始化后不得替换。", "Secret used to sign user and administrator access tokens; do not replace it after initialization.", FieldOwnerDeployctl, requiredAPINode),
			secretField("API_KEY_SIGNING_SECRET_ENCRYPTION_KEY", "application secrets", "加密数据库中 API Key 签名材料的密钥；丢失后已有密钥无法恢复。", "Key encrypting API-key signing material in the database; losing it makes existing keys unrecoverable.", FieldOwnerDeployctl, requiredAPINode),
			secretField("CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY", "application secrets", "加密支付渠道敏感配置的密钥；初始化后不得替换。", "Key encrypting sensitive payment-provider configuration; do not replace it after initialization.", FieldOwnerDeployctl, requiredAPINode),
			secretField("PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY", "application secrets", "加密数据库中上游账号、对象存储和 SMTP 等敏感配置的共享密钥。", "Shared key encrypting provider, object-storage, SMTP, and other sensitive database configuration.", FieldOwnerDeployctl, requiredBackend),
			secretField("PROMPT_OPTIMIZATION_QUOTE_SIGNING_KEY", "application secrets", "签名提示词优化报价的密钥；所有 API 节点必须一致。", "Key signing prompt-optimization quotes; it must be identical on every API node.", FieldOwnerDeployctl, requiredAPINode),

			field("PUBLIC_API_URL", "public endpoints", "浏览器和 Web 节点可访问的 API 公共基础地址，可使用 HTTP、域名或 IP 加端口。", "Public API base URL reachable by browsers and Web nodes; HTTP, domains, and IP-with-port are supported.", "http://127.0.0.1:8080", "", FieldOwnerSetup, requiredWebNode, validateHTTPURL),
			field("CORS_ALLOWED_ORIGINS", "public endpoints", "允许跨域调用 API 的前端来源，多个来源使用逗号分隔；同源部署可留空。", "Frontend origins allowed to call the API, comma-separated; leave empty for same-origin deployments.", "http://127.0.0.1:5173,http://127.0.0.1:5174", "", FieldOwnerSetup, requiredNever, validateOptionalCSV),

			field("API_PORT", "ports", "API 在宿主机或 Gateway 后监听的端口。", "Port on which the API listens on the host or behind the Gateway.", "8080", "8080", FieldOwnerDeployctl, requiredAPINode, validatePort),
			field("GATEWAY_PORT", "ports", "可移植 Gateway 对外监听端口；未部署 Gateway 时可留空。", "Public listening port for the portable Gateway; leave empty when Gateway is not deployed.", "80", "", FieldOwnerDeployctl, requiredNever, validateOptionalPort),
			field("USER_WEB_PORT", "ports", "用户前端模块的内部监听端口。", "Internal listening port for the user frontend module.", "5173", "", FieldOwnerDeployctl, requiredNever, validateOptionalPort),
			field("ADMIN_WEB_PORT", "ports", "管理后台模块的内部监听端口。", "Internal listening port for the admin frontend module.", "5174", "", FieldOwnerDeployctl, requiredNever, validateOptionalPort),
			field("DOCS_WEB_PORT", "ports", "文档前端模块的内部监听端口。", "Internal listening port for the documentation frontend module.", "5175", "", FieldOwnerDeployctl, requiredNever, validateOptionalPort),

			field("IMAGE_REGISTRY", "release", "Docker 镜像仓库地址；原生部署可留空。", "Docker image registry; leave empty for native deployments.", "registry.example.com/project", "", FieldOwnerDeployctl, requiredNever, validateOptionalNonEmpty),
			field("IMAGE_TAG", "release", "Docker 各模块使用的不可变镜像标签。", "Immutable Docker image tag used by application modules.", "v1.0.0", "", FieldOwnerDeployctl, requiredDocker, validateNonEmpty),
			field("RELEASE_VERSION", "release", "原生发布包版本或本次部署选定的发行版本。", "Native release bundle version or the release selected for this deployment.", "v1.0.0", "", FieldOwnerDeployctl, requiredNative, validateNonEmpty),

			field("INSTALLATION_ID", "identity", "当前安装的稳定唯一标识，所有集群节点必须一致。", "Stable unique installation identifier shared by every cluster node.", "019d0000-0000-7000-8000-000000000000", "", FieldOwnerDeployctl, requiredAlways, validateIdentifier),
			field("CLUSTER_NODE_ID", "identity", "当前集群节点的唯一标识；single 节点可留空。", "Unique identifier of the current cluster node; single-node deployments may leave it empty.", "019d0000-0000-7000-8000-000000000001", "", FieldOwnerDeployctl, requiredClusterNode, validateIdentifier),
			field("CONFIG_REVISION", "identity", "节点当前配置修订号，用于检测集群配置漂移。", "Current node configuration revision used to detect cluster configuration drift.", "1", "1", FieldOwnerApplication, requiredClusterNode, validatePositiveInteger),
			field("APPLICATION_VERSION", "identity", "当前 API、Worker 或 Web 构建版本，用于加入和健康检查兼容性判断。", "Current API, Worker, or Web build version used for enrollment and health compatibility checks.", "v1.0.0", "", FieldOwnerDeployctl, requiredAlways, ValidateApplicationVersion),
		},
	}
}

func (schema RuntimeSchema) Validate() error {
	if schema.Version <= 0 {
		return fmt.Errorf("runtime schema version must be positive")
	}
	seen := make(map[string]struct{}, len(schema.Fields))
	for _, runtimeField := range schema.Fields {
		if !runtimeFieldKeyPattern.MatchString(runtimeField.Key) {
			return fmt.Errorf("runtime field key %q is invalid", runtimeField.Key)
		}
		if _, exists := seen[runtimeField.Key]; exists {
			return fmt.Errorf("runtime field key %q is duplicated", runtimeField.Key)
		}
		seen[runtimeField.Key] = struct{}{}
		if strings.TrimSpace(runtimeField.Group) == "" || strings.TrimSpace(runtimeField.DescriptionZH) == "" || strings.TrimSpace(runtimeField.DescriptionEN) == "" {
			return fmt.Errorf("runtime field %q has incomplete metadata", runtimeField.Key)
		}
		if runtimeField.RequiredWhen == nil || runtimeField.Validate == nil {
			return fmt.Errorf("runtime field %q has incomplete rules", runtimeField.Key)
		}
		if runtimeField.Secret && strings.TrimSpace(runtimeField.Example) != "" {
			return fmt.Errorf("secret runtime field %q must not contain an example", runtimeField.Key)
		}
		switch runtimeField.Owner {
		case FieldOwnerDeployctl, FieldOwnerSetup, FieldOwnerApplication:
		default:
			return fmt.Errorf("runtime field %q has invalid owner %q", runtimeField.Key, runtimeField.Owner)
		}
	}
	return nil
}

func RequiredRuntimeFields(schema RuntimeSchema, context DeploymentContext) ([]RuntimeField, error) {
	if err := ValidateDeploymentContext(context); err != nil {
		return nil, fmt.Errorf("validate deployment context: %w", err)
	}
	fields := make([]RuntimeField, 0, len(schema.Fields))
	for _, runtimeField := range schema.Fields {
		if runtimeField.RequiredWhen != nil && runtimeField.RequiredWhen(context) {
			fields = append(fields, runtimeField)
		}
	}
	return fields, nil
}

func ValidateDeploymentContext(context DeploymentContext) error {
	if err := validateDeploymentMode(string(context.Mode)); err != nil {
		return err
	}
	if err := validateDeploymentProfile(string(context.Profile)); err != nil {
		return err
	}
	if err := validateDeploymentTopology(string(context.Topology)); err != nil {
		return err
	}
	if err := validateDeploymentRole(string(context.Role)); err != nil {
		return err
	}
	if context.Mode == DeploymentModeNative && context.Profile == DeploymentProfileFull {
		return fmt.Errorf("native deployments do not support the full profile")
	}
	if context.Profile == DeploymentProfileFull {
		if context.Mode != DeploymentModeDocker || context.Topology != DeploymentTopologySingle || context.Role != DeploymentRoleSingle {
			return fmt.Errorf("the full profile supports only single-node Docker deployments")
		}
	}
	if context.Role == DeploymentRoleSingle && context.Topology != DeploymentTopologySingle {
		return fmt.Errorf("role %q requires the single topology", context.Role)
	}
	if context.Role != DeploymentRoleSingle && context.Topology != DeploymentTopologyCluster {
		return fmt.Errorf("role %q requires the cluster topology", context.Role)
	}
	if context.Topology == DeploymentTopologyCluster && context.StorageDriver != "s3" && context.Role != DeploymentRoleWeb {
		return fmt.Errorf("cluster deployments require s3 object storage")
	}
	if context.Role != DeploymentRoleWeb {
		if err := validateStorageDriver(context.StorageDriver); err != nil {
			return err
		}
	}
	if context.Role == DeploymentRoleAPI || context.Role == DeploymentRoleWorker || context.Role == DeploymentRoleWeb {
		if context.Profile == DeploymentProfileFull {
			return fmt.Errorf("joined nodes cannot use the full profile")
		}
		if !context.SetupCompleted {
			return fmt.Errorf("role %q can join only a completed installation", context.Role)
		}
	}
	return nil
}

func field(key, group, zh, en, example, defaultValue string, owner FieldOwner, required func(DeploymentContext) bool, validate func(string) error) RuntimeField {
	return RuntimeField{
		Key: key, Group: group, DescriptionZH: zh, DescriptionEN: en,
		Example: example, DefaultValue: defaultValue, Owner: owner,
		RestartRequired: true, RequiredWhen: required, Validate: validate,
	}
}

func secretField(key, group, zh, en string, owner FieldOwner, required func(DeploymentContext) bool) RuntimeField {
	runtimeField := field(key, group, zh, en, "", "", owner, required, validateOptionalNonEmpty)
	runtimeField.Secret = true
	return runtimeField
}

func secretValidatedField(key, group, zh, en string, owner FieldOwner, required func(DeploymentContext) bool, validate func(string) error) RuntimeField {
	runtimeField := secretField(key, group, zh, en, owner, required)
	runtimeField.Validate = validate
	return runtimeField
}

func requiredAlways(DeploymentContext) bool { return true }
func requiredNever(DeploymentContext) bool  { return false }
func requiredBackend(context DeploymentContext) bool {
	return context.Role != DeploymentRoleWeb
}
func requiredAPINode(context DeploymentContext) bool {
	return context.Role == DeploymentRoleSingle || context.Role == DeploymentRoleControl || context.Role == DeploymentRoleAPI
}
func requiredWebNode(context DeploymentContext) bool { return context.Role == DeploymentRoleWeb }
func requiredClusterNode(context DeploymentContext) bool {
	return context.Topology == DeploymentTopologyCluster
}
func requiredSetupAuthority(context DeploymentContext) bool {
	return !context.SetupCompleted && (context.Role == DeploymentRoleSingle || context.Role == DeploymentRoleControl)
}
func requiredSetupAuthorityRole(context DeploymentContext) bool {
	return context.Role == DeploymentRoleSingle || context.Role == DeploymentRoleControl
}
func requiredDockerFull(context DeploymentContext) bool {
	return context.Mode == DeploymentModeDocker && context.Profile == DeploymentProfileFull
}
func requiredDocker(context DeploymentContext) bool { return context.Mode == DeploymentModeDocker }
func requiredNative(context DeploymentContext) bool { return context.Mode == DeploymentModeNative }
func requiredS3Storage(context DeploymentContext) bool {
	return requiredBackend(context) && context.StorageDriver == "s3"
}
func requiredLocalStorage(context DeploymentContext) bool {
	return requiredBackend(context) && context.Topology == DeploymentTopologySingle && context.StorageDriver == "local"
}

func validateDeploymentMode(value string) error {
	switch DeploymentMode(value) {
	case DeploymentModeDocker, DeploymentModeNative:
		return nil
	default:
		return fmt.Errorf("deployment mode %q is invalid", value)
	}
}

func validateDeploymentProfile(value string) error {
	switch DeploymentProfile(value) {
	case DeploymentProfileFull, DeploymentProfileCore, DeploymentProfileCustom:
		return nil
	default:
		return fmt.Errorf("deployment profile %q is invalid", value)
	}
}

func validateDeploymentTopology(value string) error {
	switch DeploymentTopology(value) {
	case DeploymentTopologySingle, DeploymentTopologyCluster:
		return nil
	default:
		return fmt.Errorf("deployment topology %q is invalid", value)
	}
}

func validateDeploymentRole(value string) error {
	switch DeploymentRole(value) {
	case DeploymentRoleSingle, DeploymentRoleControl, DeploymentRoleAPI, DeploymentRoleWorker, DeploymentRoleWeb:
		return nil
	default:
		return fmt.Errorf("deployment role %q is invalid", value)
	}
}

func validateStorageDriver(value string) error {
	if value == "local" || value == "s3" {
		return nil
	}
	return fmt.Errorf("storage driver %q is invalid", value)
}

func validateBool(value string) error {
	if value == "" {
		return nil
	}
	if _, err := strconv.ParseBool(value); err != nil {
		return fmt.Errorf("parse boolean %q: %w", value, err)
	}
	return nil
}

func validatePositiveInteger(value string) error {
	if value == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fmt.Errorf("value %q must be a positive integer", value)
	}
	return nil
}

func validateRequiredPositiveUint64(value string) error {
	if value == "" {
		return fmt.Errorf("value must be a positive integer")
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed == 0 {
		return fmt.Errorf("value must be a positive integer")
	}
	return nil
}

func validateOptionalPositiveInteger(value string) error { return validatePositiveInteger(value) }

func validateOptionalNonNegativeInteger(value string) error {
	if value == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fmt.Errorf("value %q must be a non-negative integer", value)
	}
	return nil
}

func validatePort(value string) error {
	if value == "" {
		return nil
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("port %q must be between 1 and 65535", value)
	}
	return nil
}

func validateOptionalPort(value string) error { return validatePort(value) }

func validateNonEmpty(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("value must not be empty")
	}
	return nil
}

func validateOptionalNonEmpty(value string) error {
	if value != "" && strings.TrimSpace(value) == "" {
		return fmt.Errorf("provided value must not contain only whitespace")
	}
	return nil
}

func validateIdentifier(value string) error {
	if value == "" {
		return nil
	}
	if strings.ContainsAny(value, " \t\r\n") {
		return fmt.Errorf("identifier %q contains whitespace", value)
	}
	return nil
}

func validateConnectionURL(schemes ...string) func(string) error {
	return func(value string) error {
		if value == "" {
			return nil
		}
		parsed, err := url.Parse(value)
		if err != nil {
			return errors.New("connection URL is invalid")
		}
		for _, scheme := range schemes {
			if parsed.Scheme == scheme && parsed.Host != "" {
				return nil
			}
		}
		return fmt.Errorf("connection URL must use one of %v and include a host", schemes)
	}
}

func validateHTTPURL(value string) error {
	if value == "" {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("value %q must be an HTTP URL", value)
	}
	return nil
}

func validateOptionalHTTPURL(value string) error { return validateHTTPURL(value) }

func validateOptionalCSV(value string) error {
	for _, item := range strings.Split(value, ",") {
		if value != "" && strings.TrimSpace(item) == "" {
			return fmt.Errorf("CSV value contains an empty item")
		}
	}
	return nil
}

func validateOptionalDurationLike(value string) error {
	if value == "" {
		return nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return fmt.Errorf("duration %q is invalid", value)
	}
	return nil
}
