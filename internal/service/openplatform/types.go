package openplatform

import (
	"context"
	"encoding/json"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/google/uuid"
	"time"

	domainauth "github.com/fatballfish/pic-gallery/internal/domain/auth"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/internal/service/assetgroup"
	"github.com/fatballfish/pic-gallery/internal/service/assets"
	"github.com/fatballfish/pic-gallery/internal/service/billing"
	"github.com/fatballfish/pic-gallery/internal/service/capabilities"
	"github.com/fatballfish/pic-gallery/internal/service/imagetask"
	"github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/service/project"
	"github.com/fatballfish/pic-gallery/internal/service/videorouting"
	"github.com/fatballfish/pic-gallery/internal/service/videotask"
)

type Principal struct {
	UserID            int64    `json:"user_id"`
	APIKeyID          string   `json:"api_key_id,omitempty"`
	CredentialVersion string   `json:"credential_version,omitempty"`
	ClientID          string   `json:"client_id,omitempty"`
	GrantID           string   `json:"grant_id,omitempty"`
	Scopes            []string `json:"scopes"`
	GroupCode         string   `json:"group_code,omitempty"`
}

type UserReader interface {
	GetUserByID(int64) (domainauth.User, bool)
}
type GroupOperations interface {
	Execute(context.Context, int64, string, json.RawMessage) (any, error)
}
type APIKeyUsageManager interface {
	CheckTaskAllowed(context.Context, int64, int64, string, time.Time) (domainbilling.APIKeyQuota, error)
}
type Dependencies struct {
	Auth         UserReader
	Billing      *billing.Service
	Images       *imagetask.Service
	ImageModels  *capabilities.Service
	Videos       *videotask.Service
	VideoModels  *videorouting.Service
	Assets       *mediaasset.Service
	UploadTarget func(context.Context, Principal, uuid.UUID, int, string) (storage.MultipartPartTarget, error)
	AssetAccess  func(context.Context, Principal, uuid.UUID, string) (any, error)
	References   *assets.Service
	Projects     *project.Service
	APIKeys      APIKeyUsageManager
	Groups       *assetgroup.Service
	// QuoteKey must be a stable server secret of at least 32 bytes.
	QuoteKey []byte
	Now      func() time.Time
	// ValidatePrincipal rechecks current key or OAuth grant state on each operation.
	ValidatePrincipal func(context.Context, Principal) error
}
type Service struct{ deps Dependencies }

func New(deps Dependencies) *Service {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	deps.QuoteKey = append([]byte(nil), deps.QuoteKey...)
	return &Service{deps: deps}
}

type OperationSpec struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	Scope        string         `json:"scope"`
	InputSchema  map[string]any `json:"input_schema"`
	OutputSchema map[string]any `json:"output_schema"`
	ReadOnly     bool           `json:"read_only"`
	Destructive  bool           `json:"destructive"`
	Idempotent   bool           `json:"idempotent"`
}
