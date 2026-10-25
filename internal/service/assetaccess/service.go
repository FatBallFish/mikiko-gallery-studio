// Package assetaccess issues resource-only capabilities for external asset consumers.
package assetaccess

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/google/uuid"
)

const PathPrefix = "/api/open/v1/asset-access/"
const maximumTTL = 5 * time.Minute

// Backend is implemented by mediaasset.Service. OpenContentRange must use native
// storage ranges, including metadata-only HEAD, never discard a full prefix.
type Backend interface {
	Access(context.Context, int64, uuid.UUID, string) (mediaasset.AccessResult, error)
	GetAsset(context.Context, int64, uuid.UUID) (mediaasset.Asset, error)
	OpenContentRange(context.Context, int64, uuid.UUID, string, string, bool) (mediaasset.RangedContentStream, error)
}
type Options struct {
	PublicAPIURL string
	// SigningKey is a stable server secret, never an API key or user token.
	SigningKey []byte
	TTL        time.Duration
	Now        func() time.Time
	// ValidatePrincipal rechecks current user/key/OAuth grant state. The caller
	// authorizes scopes before minting; this helper does not add assets scopes.
	ValidatePrincipal func(context.Context, openplatform.Principal) error
}
type Service struct {
	media Backend
	opts  Options
	base  string
}
type claims struct {
	Version   int                    `json:"v"`
	Principal openplatform.Principal `json:"principal"`
	AssetID   uuid.UUID              `json:"asset"`
	Purpose   string                 `json:"purpose"`
	IssuedAt  int64                  `json:"iat"`
	ExpiresAt int64                  `json:"exp"`
	Audience  string                 `json:"aud"`
}
type Content struct {
	mediaasset.RangedContentStream
	Filename string
	Purpose  string
}

func New(media Backend, opts Options) (*Service, error) {
	u, err := url.Parse(opts.PublicAPIURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "https" && !(u.Scheme == "http" && loopback(u.Hostname()))) {
		return nil, fmt.Errorf("asset access public API URL must be a canonical HTTPS origin (HTTP loopback development only)")
	}
	if media == nil || len(opts.SigningKey) < 32 {
		return nil, fmt.Errorf("asset access backend and stable signing key of at least 32 bytes are required")
	}
	if opts.TTL == 0 {
		opts.TTL = maximumTTL
	}
	if opts.TTL < time.Second || opts.TTL > maximumTTL {
		return nil, fmt.Errorf("asset access TTL must be between one second and five minutes")
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	opts.SigningKey = append([]byte(nil), opts.SigningKey...)
	return &Service{media: media, opts: opts, base: strings.TrimRight(u.String(), "/")}, nil
}
func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Access has the exact shared openplatform.Dependencies.AssetAccess signature.
// Every backend uses the proxy so current principal revocation is checked on reads.
func (s *Service) Access(ctx context.Context, p openplatform.Principal, id uuid.UUID, purpose string) (any, error) {
	purpose = strings.ToLower(strings.TrimSpace(purpose))
	if !mediaasset.ValidAccessPurpose(purpose) || id == uuid.Nil {
		return nil, errs.BadRequest("invalid asset access request")
	}
	if err := s.validatePrincipal(ctx, p); err != nil {
		return nil, err
	}
	if _, err := s.liveAsset(ctx, p.UserID, id); err != nil {
		return nil, err
	}
	// Resolve the requested purpose now, but never publish storage presigned URLs:
	// those bypass subsequent principal disablement and credential rotation.
	_, err := s.media.Access(ctx, p.UserID, id, purpose)
	if err != nil {
		return nil, err
	}
	now := s.opts.Now().UTC()
	c := claims{Version: 1, Principal: p, AssetID: id, Purpose: purpose, IssuedAt: now.Unix(), ExpiresAt: now.Add(s.opts.TTL).Unix(), Audience: s.base + PathPrefix}
	body, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	payload := base64.RawURLEncoding.EncodeToString(body)
	token := payload + "." + base64.RawURLEncoding.EncodeToString(s.sign(payload))
	return mediaasset.AccessResult{URL: s.base + PathPrefix + token + "/content", ExpiresAt: time.Unix(c.ExpiresAt, 0).UTC(), Range: true}, nil
}
func (s *Service) sign(payload string) []byte {
	mac := hmac.New(sha256.New, s.opts.SigningKey)
	mac.Write([]byte("asset-access-v1\x00" + payload))
	return mac.Sum(nil)
}
func invalidGrant() error {
	return errs.New(403, errs.CodeForbidden, "invalid or expired asset access grant")
}
func (s *Service) validatePrincipal(ctx context.Context, p openplatform.Principal) error {
	if p.UserID <= 0 || (p.APIKeyID == "" && (p.ClientID == "" || p.GrantID == "")) || (p.APIKeyID != "" && strings.TrimSpace(p.CredentialVersion) == "") {
		return invalidGrant()
	}
	allowed := false
	for _, scope := range p.Scopes {
		switch scope {
		case "assets:read", "images:read", "videos:read":
			allowed = true
		}
	}
	if !allowed {
		return invalidGrant()
	}
	if s.opts.ValidatePrincipal != nil {
		return s.opts.ValidatePrincipal(ctx, p)
	}
	return nil
}
func (s *Service) liveAsset(ctx context.Context, user int64, id uuid.UUID) (mediaasset.Asset, error) {
	asset, err := s.media.GetAsset(ctx, user, id)
	if err != nil {
		return mediaasset.Asset{}, err
	}
	if asset.UserID != user || asset.ID != id || asset.DeletedAt != nil || asset.Status == "deleted" {
		return mediaasset.Asset{}, invalidGrant()
	}
	return asset, nil
}
func (s *Service) OpenContent(ctx context.Context, token, value string, metadataOnly bool) (Content, error) {
	if len(token) > 16384 {
		return Content{}, invalidGrant()
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return Content{}, invalidGrant()
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, s.sign(parts[0])) {
		return Content{}, invalidGrant()
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Content{}, invalidGrant()
	}
	var c claims
	if json.Unmarshal(body, &c) != nil || c.Version != 1 || c.AssetID == uuid.Nil || !mediaasset.ValidAccessPurpose(c.Purpose) || c.Audience != s.base+PathPrefix || c.ExpiresAt <= s.opts.Now().Unix() || c.IssuedAt > s.opts.Now().Unix() || c.ExpiresAt <= c.IssuedAt || c.ExpiresAt-c.IssuedAt > int64(maximumTTL/time.Second) {
		return Content{}, invalidGrant()
	}
	if err := s.validatePrincipal(ctx, c.Principal); err != nil {
		return Content{}, err
	}
	asset, err := s.liveAsset(ctx, c.Principal.UserID, c.AssetID)
	if err != nil {
		return Content{}, err
	}
	stream, err := s.media.OpenContentRange(ctx, c.Principal.UserID, c.AssetID, c.Purpose, value, metadataOnly)
	return Content{RangedContentStream: stream, Filename: asset.Name, Purpose: c.Purpose}, err
}
