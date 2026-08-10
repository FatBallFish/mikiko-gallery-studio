package cluster

import "time"

type JoinRole string

const (
	JoinRoleAPI    JoinRole = "api"
	JoinRoleWorker JoinRole = "worker"
	JoinRoleWeb    JoinRole = "web"
)

type NodeRole string

const (
	NodeRoleSingle  NodeRole = "single"
	NodeRoleControl NodeRole = "control"
	NodeRoleAPI     NodeRole = "api"
	NodeRoleWorker  NodeRole = "worker"
	NodeRoleWeb     NodeRole = "web"
)

type NodeHealth string

const (
	NodeHealthJoining  NodeHealth = "joining"
	NodeHealthHealthy  NodeHealth = "healthy"
	NodeHealthDegraded NodeHealth = "degraded"
	NodeHealthUnready  NodeHealth = "unready"
	NodeHealthOffline  NodeHealth = "offline"
)

type NodeSource string

const (
	NodeSourceLogicalSingle NodeSource = "logical-single"
	NodeSourceHeartbeat     NodeSource = "heartbeat"
)

type Installation struct {
	InstallationID       string
	Initialized          bool
	ApplicationVersion   string
	RuntimeSchemaVersion int
	ConfigRevision       int64
}

type Token struct {
	TokenID          string     `json:"token_id"`
	InstallationID   string     `json:"installation_id"`
	Role             JoinRole   `json:"role"`
	ExpiresAt        time.Time  `json:"expires_at"`
	ConsumedAt       *time.Time `json:"consumed_at,omitempty"`
	ConsumedByNodeID string     `json:"consumed_by_node_id,omitempty"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	CreatedBy        string     `json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type TokenRecord struct {
	Token
	TokenHash           string `json:"-"`
	TokenProofPublicKey string `json:"-"`
}

type IssuedToken struct {
	Token      Token  `json:"token"`
	Credential string `json:"credential"`
}

type CreateTokenRequest struct {
	Role    JoinRole
	TTL     time.Duration
	ActorID string
}

type ListTokensRequest struct {
	Page     int
	PageSize int
	Role     JoinRole
	Status   string
	At       time.Time
}

type TokenPage struct {
	Items    []Token `json:"items"`
	Page     int     `json:"page"`
	PageSize int     `json:"page_size"`
	Total    int     `json:"total"`
}

type Node struct {
	NodeID               string     `json:"node_id"`
	InstallationID       string     `json:"installation_id"`
	Role                 NodeRole   `json:"role"`
	ApplicationVersion   string     `json:"application_version"`
	RuntimeSchemaVersion int        `json:"runtime_schema_version"`
	ConfigRevision       int64      `json:"config_revision"`
	Health               NodeHealth `json:"health"`
	LastError            string     `json:"last_error,omitempty"`
	LastHeartbeatAt      *time.Time `json:"last_heartbeat_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type ListNodesRequest struct {
	Page     int
	PageSize int
	Role     NodeRole
}

type NodeStatus struct {
	Node
	Source                  NodeSource `json:"source"`
	EffectiveHealth         NodeHealth `json:"effective_health"`
	ApplicationVersionDrift bool       `json:"application_version_drift"`
	RuntimeSchemaDrift      bool       `json:"runtime_schema_drift"`
	ConfigRevisionDrift     bool       `json:"config_revision_drift"`
}

type NodePage struct {
	Items    []NodeStatus `json:"items"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
	Total    int          `json:"total"`
}

type RegisterNodeRequest struct {
	NodeID               string
	Role                 NodeRole
	ApplicationVersion   string
	RuntimeSchemaVersion int
	ConfigRevision       int64
}

type Enrollment struct {
	Token Token `json:"token"`
	Node  Node  `json:"node"`
}

type EnrollmentChallenge struct {
	Protocol             string    `json:"protocol"`
	ChallengeID          string    `json:"challenge_id"`
	InstallationID       string    `json:"installation_id"`
	TokenID              string    `json:"token_id"`
	Role                 JoinRole  `json:"role"`
	NodeID               string    `json:"node_id"`
	ClientPublicKey      string    `json:"client_public_key"`
	ServerPublicKey      string    `json:"server_public_key"`
	ServerNonce          string    `json:"server_nonce"`
	ApplicationVersion   string    `json:"application_version"`
	RuntimeSchemaVersion int       `json:"runtime_schema_version"`
	ConfigRevision       int64     `json:"config_revision"`
	ExpiresAt            time.Time `json:"expires_at"`
}

type RuntimeEnvelopePayload struct {
	Protocol             string            `json:"protocol"`
	InstallationID       string            `json:"installation_id"`
	NodeID               string            `json:"node_id"`
	Role                 NodeRole          `json:"role"`
	ApplicationVersion   string            `json:"application_version"`
	RuntimeSchemaVersion int               `json:"runtime_schema_version"`
	ConfigRevision       int64             `json:"config_revision"`
	Values               map[string]string `json:"values"`
}

type EncryptedRuntimeEnvelope struct {
	Algorithm  string `json:"algorithm"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type ChallengeRecord struct {
	EnrollmentChallenge
	SealedServerPrivateKey string     `json:"-"`
	ConsumedAt             *time.Time `json:"consumed_at,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

type CreateChallengeRequest struct {
	Protocol             string `json:"protocol"`
	TokenID              string `json:"token_id"`
	NodeID               string `json:"node_id"`
	NodePublicKey        string `json:"node_public_key"`
	ApplicationVersion   string `json:"application_version"`
	RuntimeSchemaVersion int    `json:"runtime_schema_version"`
}

type JoinRequest struct {
	Protocol    string `json:"protocol"`
	ChallengeID string `json:"challenge_id"`
	Proof       string `json:"proof"`
}

type JoinResponse struct {
	Protocol             string                   `json:"protocol"`
	InstallationID       string                   `json:"installation_id"`
	NodeID               string                   `json:"node_id"`
	Role                 NodeRole                 `json:"role"`
	ApplicationVersion   string                   `json:"application_version"`
	RuntimeSchemaVersion int                      `json:"runtime_schema_version"`
	ConfigRevision       int64                    `json:"config_revision"`
	ExpiresAt            time.Time                `json:"expires_at"`
	EncryptedEnvelope    EncryptedRuntimeEnvelope `json:"encrypted_envelope"`
}

type HeartbeatRequest struct {
	NodeID               string
	Role                 NodeRole
	Health               NodeHealth
	LastError            string
	ApplicationVersion   string
	RuntimeSchemaVersion int
	ConfigRevision       int64
}
