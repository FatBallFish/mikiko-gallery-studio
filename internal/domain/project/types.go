package project

import "time"

const (
	DefaultName             = "默认"
	DefaultProjectName      = DefaultName
	MaxIdempotencyKeyLength = 128
	StatusActive            = "active"
	StatusTransferring      = "transferring"
	StatusDeleted           = "deleted"
)

type Project struct {
	ID         string    `json:"id"`
	UserID     int64     `json:"-"`
	Name       string    `json:"name"`
	NameKey    string    `json:"-"`
	IsDefault  bool      `json:"is_default"`
	Status     string    `json:"status"`
	Version    int64     `json:"version"`
	TaskCount  int       `json:"task_count"`
	AssetCount int       `json:"asset_count"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Snapshot struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
}

func (p Project) Snapshot() Snapshot {
	return Snapshot{ID: p.ID, Name: p.Name, IsDefault: p.IsDefault}
}

type CreateRequest struct {
	Name           string `json:"name"`
	IdempotencyKey string `json:"-"`
}

type RenameRequest struct {
	Name            string `json:"name"`
	ExpectedVersion int64  `json:"expected_version"`
}

type DeleteRequest struct {
	TargetProjectID string `json:"target_project_id,omitempty"`
	ExpectedVersion int64  `json:"expected_version"`
	IdempotencyKey  string `json:"-"`
	RequestID       string `json:"-"`
}

type OwnershipCounts struct {
	Tasks  int `json:"tasks"`
	Assets int `json:"assets"`
}

type DeleteResult struct {
	Project     Project         `json:"project"`
	Transferred OwnershipCounts `json:"transferred"`
}
