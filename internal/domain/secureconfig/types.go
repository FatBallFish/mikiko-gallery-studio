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
