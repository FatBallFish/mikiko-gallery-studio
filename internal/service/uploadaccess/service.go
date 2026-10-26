// Package uploadaccess issues revocable capabilities for local multipart byte uploads.
package uploadaccess

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/google/uuid"
)

const PathPrefix = "/api/open/v1/upload-access/"
const Header = "X-Upload-Capability"
const maximumTTL = 5 * time.Minute
const MaximumPartSize = int64(32 << 20)

type Backend interface {
	SignPart(context.Context, int64, uuid.UUID, int, string) (storage.MultipartPartTarget, error)
	GetUploadSession(context.Context, int64, uuid.UUID) (mediaasset.UploadSession, error)
	UploadPart(context.Context, int64, uuid.UUID, int, io.Reader, int64, string) (storage.CompletedPart, error)
}
type Options struct {
	PublicAPIURL      string
	SigningKey        []byte
	Now               func() time.Time
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
	Session   uuid.UUID              `json:"session"`
	Part      int                    `json:"part"`
	Method    string                 `json:"method"`
	Audience  string                 `json:"aud"`
	Size      int64                  `json:"size"`
	Checksum  string                 `json:"checksum"`
	IssuedAt  int64                  `json:"iat"`
	ExpiresAt int64                  `json:"exp"`
}

func New(media Backend, opts Options) (*Service, error) {
	u, err := url.Parse(opts.PublicAPIURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("upload access requires canonical public API origin")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || ip != nil && ip.IsLoopback())) {
		return nil, fmt.Errorf("upload access requires HTTPS or loopback HTTP")
	}
	if media == nil || len(opts.SigningKey) < 32 || opts.ValidatePrincipal == nil {
		return nil, fmt.Errorf("upload access requires backend, signing key and principal validator")
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	opts.SigningKey = append([]byte(nil), opts.SigningKey...)
	return &Service{media: media, opts: opts, base: strings.TrimRight(u.String(), "/")}, nil
}
func invalid() error {
	return errs.New(403, errs.CodeForbidden, "invalid or expired upload capability")
}
func checksumValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	b, err := hex.DecodeString(value)
	if err != nil || len(b) != sha256.Size {
		b, err = base64.StdEncoding.Strict().DecodeString(value)
	}
	if err != nil || len(b) != sha256.Size {
		return "", errs.BadRequest("SHA256 checksum must be 64 hex characters or base64 encoded 32 bytes")
	}
	return hex.EncodeToString(b), nil
}
func (s *Service) validate(ctx context.Context, p openplatform.Principal) error {
	if p.UserID <= 0 || (p.APIKeyID == "" && (p.ClientID == "" || p.GrantID == "")) || (p.APIKeyID != "" && p.CredentialVersion == "") {
		return invalid()
	}
	allowed := false
	for _, scope := range p.Scopes {
		if scope == "uploads:write" {
			allowed = true
		}
	}
	if !allowed {
		return invalid()
	}
	if err := s.opts.ValidatePrincipal(ctx, p); err != nil {
		var app *errs.Error
		if errors.As(err, &app) {
			return err
		}
		return invalid()
	}
	return nil
}
func (s *Service) session(ctx context.Context, p openplatform.Principal, id uuid.UUID, part int) (mediaasset.UploadSession, int64, error) {
	session, err := s.media.GetUploadSession(ctx, p.UserID, id)
	if err != nil {
		return session, 0, err
	}
	if session.ID != id || session.UserID != p.UserID || !session.ExpiresAt.After(s.opts.Now()) || (session.Status != "initialized" && session.Status != "uploading") {
		return session, 0, invalid()
	}
	if part < 1 || part > session.PartCount || session.PartSize <= 0 || session.PartSize > MaximumPartSize || session.DeclaredSizeBytes <= 0 {
		return session, 0, errs.BadRequest("invalid upload part")
	}
	size := session.PartSize
	if part == session.PartCount {
		size = session.DeclaredSizeBytes - int64(part-1)*session.PartSize
	}
	if size <= 0 || size > MaximumPartSize {
		return session, 0, errs.BadRequest("invalid upload part size")
	}
	return session, size, nil
}
func (s *Service) sign(payload string) []byte {
	mac := hmac.New(sha256.New, s.opts.SigningKey)
	mac.Write([]byte("upload-access-v1\x00" + payload))
	return mac.Sum(nil)
}

// Target preserves direct S3 signing and only falls back for local proxy uploads.
func (s *Service) Target(ctx context.Context, p openplatform.Principal, id uuid.UUID, part int, checksum string) (storage.MultipartPartTarget, error) {
	if err := s.validate(ctx, p); err != nil {
		return storage.MultipartPartTarget{}, err
	}
	target, err := s.media.SignPart(ctx, p.UserID, id, part, checksum)
	if !errors.Is(err, storage.ErrDirectUploadRequired) {
		return target, err
	}
	checksum, err = checksumValue(checksum)
	if err != nil {
		return target, err
	}
	session, size, err := s.session(ctx, p, id, part)
	if err != nil {
		return target, err
	}
	now := s.opts.Now().UTC()
	expiry := now.Add(maximumTTL)
	if session.ExpiresAt.Before(expiry) {
		expiry = session.ExpiresAt
	}
	path := PathPrefix + id.String() + "/parts/" + strconv.Itoa(part)
	c := claims{Version: 1, Principal: p, Session: id, Part: part, Method: http.MethodPut, Audience: s.base + path, Size: size, Checksum: checksum, IssuedAt: now.Unix(), ExpiresAt: expiry.Unix()}
	if c.ExpiresAt <= c.IssuedAt {
		return target, invalid()
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return target, err
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	token := payload + "." + base64.RawURLEncoding.EncodeToString(s.sign(payload))
	return storage.MultipartPartTarget{URL: c.Audience, Headers: map[string]string{Header: token, "X-Checksum-SHA256": checksum}, ExpiresAt: time.Unix(c.ExpiresAt, 0).UTC()}, nil
}

// Put validates all authority before consuming any bytes. The HTTP adapter must also bound its body.
func (s *Service) Put(ctx context.Context, token, method, path string, size int64, checksum string, body io.Reader) (storage.CompletedPart, error) {
	var zero storage.CompletedPart
	if len(token) > 16384 {
		return zero, invalid()
	}
	pieces := strings.Split(token, ".")
	if len(pieces) != 2 {
		return zero, invalid()
	}
	signature, err := base64.RawURLEncoding.DecodeString(pieces[1])
	if err != nil || !hmac.Equal(signature, s.sign(pieces[0])) {
		return zero, invalid()
	}
	raw, err := base64.RawURLEncoding.DecodeString(pieces[0])
	if err != nil {
		return zero, invalid()
	}
	var c claims
	now := s.opts.Now().Unix()
	if json.Unmarshal(raw, &c) != nil || c.Version != 1 || c.Session == uuid.Nil || c.Part < 1 || c.Method != http.MethodPut || method != c.Method || c.Audience != s.base+path || path != PathPrefix+c.Session.String()+"/parts/"+strconv.Itoa(c.Part) || c.IssuedAt > now || c.ExpiresAt <= now || c.ExpiresAt <= c.IssuedAt || c.ExpiresAt-c.IssuedAt > int64(maximumTTL/time.Second) || c.Size <= 0 || c.Size > MaximumPartSize {
		return zero, invalid()
	}
	checksum, err = checksumValue(checksum)
	if err != nil || checksum != c.Checksum || size != c.Size {
		return zero, errs.BadRequest("upload length or checksum differs from capability")
	}
	if err := s.validate(ctx, c.Principal); err != nil {
		return zero, err
	}
	session, expected, err := s.session(ctx, c.Principal, c.Session, c.Part)
	if err != nil {
		return zero, err
	}
	if expected != c.Size || c.ExpiresAt > session.ExpiresAt.Unix() {
		return zero, invalid()
	}
	part, err := s.media.UploadPart(ctx, c.Principal.UserID, c.Session, c.Part, io.LimitReader(body, c.Size+1), c.Size, c.Checksum)
	if errors.Is(err, storage.ErrSizeMismatch) || errors.Is(err, storage.ErrMultipartChecksum) {
		return zero, errs.BadRequest("upload part length or checksum does not match")
	}
	return part, err
}
