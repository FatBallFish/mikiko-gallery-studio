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
	TokenHash string `json:"-"`
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

type HeartbeatRequest struct {
	NodeID               string
	Health               NodeHealth
	LastError            string
	ApplicationVersion   string
	RuntimeSchemaVersion int
	ConfigRevision       int64
}
