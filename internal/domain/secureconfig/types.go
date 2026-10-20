package secureconfig

import (
	"time"
)

type SecretStatus struct {
	HasSecret    bool       `json:"has_secret"`
	Fingerprint  string     `json:"fingerprint,omitempty"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
	SecretFields []string   `json:"secret_fields,omitempty"`
}

type SMTPConfigView struct {
	Enabled            bool         `json:"enabled"`
	Host               string       `json:"host"`
	Port               int          `json:"port"`
	Username           string       `json:"username"`
	From               string       `json:"from"`
	StartTLS           bool         `json:"starttls"`
	InsecureSkipVerify bool         `json:"insecure_skip_verify"`
	SecretStatus       SecretStatus `json:"secret_status"`
	Version            int64        `json:"version"`
	UpdatedAt          time.Time    `json:"updated_at"`
}

type UpdateSMTPConfigRequest struct {
	Version            int64             `json:"version"`
	Enabled            bool              `json:"enabled"`
	Host               string            `json:"host"`
	Port               int               `json:"port"`
	Username           string            `json:"username"`
	From               string            `json:"from"`
	StartTLS           bool              `json:"starttls"`
	InsecureSkipVerify bool              `json:"insecure_skip_verify"`
	Secrets            map[string]string `json:"secrets,omitempty"`
	ClearSecrets       []string          `json:"clear_secrets,omitempty"`
	UpdatedBy          int64             `json:"-"`
}

type SMTPTestRequest struct {
	Email string `json:"email"`
	Scene string `json:"scene"`
}

type OIDCConfigView struct {
	Enabled           bool         `json:"enabled"`
	IssuerURL         string       `json:"issuer_url"`
	ClientID          string       `json:"client_id"`
	DisplayName       string       `json:"display_name"`
	AllowLocalLogin   bool         `json:"allow_local_login"`
	AllowRegistration bool         `json:"allow_registration"`
	SecretStatus      SecretStatus `json:"secret_status"`
	Version           int64        `json:"version"`
	UpdatedAt         time.Time    `json:"updated_at"`
}

type UpdateOIDCConfigRequest struct {
	Version           int64             `json:"version"`
	Enabled           bool              `json:"enabled"`
	IssuerURL         string            `json:"issuer_url"`
	ClientID          string            `json:"client_id"`
	DisplayName       string            `json:"display_name"`
	AllowLocalLogin   bool              `json:"allow_local_login"`
	AllowRegistration bool              `json:"allow_registration"`
	Secrets           map[string]string `json:"secrets,omitempty"`
	ClearSecrets      []string          `json:"clear_secrets,omitempty"`
	UpdatedBy         int64             `json:"-"`
}

// OIDCPublicSettings exposes the login-mode switches to unauthenticated
// surfaces (feature flags endpoint, local-login gating).
type OIDCPublicSettings struct {
	Enabled           bool
	DisplayName       string
	AllowLocalLogin   bool
	AllowRegistration bool
}

// OIDCRuntimeConfig is the resolved provider configuration (secret included)
// used by the OIDC authorize/callback flows. Never serialized to clients.
type OIDCRuntimeConfig struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	DisplayName  string
}

type OIDCTestResult struct {
	Issuer                 string `json:"issuer"`
	AuthorizationEndpoint  string `json:"authorization_endpoint"`
	TokenEndpoint          string `json:"token_endpoint"`
	UserInfoEndpoint       string `json:"userinfo_endpoint"`
}
