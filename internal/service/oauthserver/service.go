package oauthserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrInvalid = errors.New("invalid OAuth request or credential")
var ErrConflict = errors.New("OAuth record conflict")
var ErrNotFound = errors.New("OAuth record not found")
var ErrInvalidScope = fmt.Errorf("invalid scope: %w", ErrInvalid)
var ErrInvalidTarget = fmt.Errorf("invalid resource: %w", ErrInvalid)
var ErrUnsupportedGrant = fmt.Errorf("unsupported grant type: %w", ErrInvalid)
var ErrInvalidRequest = fmt.Errorf("malformed request: %w", ErrInvalid)
var ErrAuthorizationExpired = fmt.Errorf("authorization request expired: %w", ErrInvalid)
var ErrRegistrationDisabled = errors.New("OAuth registration disabled")

type Record struct {
	ID, Kind string
	UserID   int64
	Payload  []byte
	Version  int64
}
type Store interface {
	Get(context.Context, string) (Record, error)
	Create(context.Context, Record) error
	CompareSwap(context.Context, Record) error
	List(context.Context, string, int64) ([]Record, error)
}
type Principal struct {
	UserID                      int64
	APIKeyID, ClientID, GrantID string
	Scopes                      []string
	GroupCode                   string
}
type User struct {
	ID        int64
	GroupCode string
	Active    bool
}
type Config struct {
	PublicAPIURL, WebURL string
	Scopes               []string
	Clients              []Client
	User                 func(context.Context, int64) (User, error)
	EnableRegistration   bool
	// RegistrationNow supplies the registration limiter clock; nil uses time.Now.
	RegistrationNow func() time.Time
}
type Service struct {
	store            Store
	cfg              Config
	issuer, resource string
	metadataMu       sync.Mutex
	metadataNext     time.Time
}
type Client struct {
	ID                      string   `json:"client_id"`
	Name                    string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
	GrantTypes              []string `json:"grant_types,omitempty"`
	ResponseTypes           []string `json:"response_types,omitempty"`
}
type Authorization struct {
	ID               string `json:"request_id"`
	Client           Client `json:"client"`
	RedirectURI      string
	Resource         string   `json:"resource"`
	Scopes           []string `json:"scopes"`
	State, Challenge string
	UserID           int64
	CSRFHash         string
	Expires          int64
	Used             bool
}
type family struct {
	ID, ClientID, ClientName, RedirectURI, Resource, Challenge, CodeHash string
	UserID                                                               int64
	Scopes                                                               []string
	CodeExpires                                                          int64
	CodeUsed, Revoked                                                    bool
	Access                                                               map[string]int64
	Refresh                                                              map[string]int64
	UsedRefresh                                                          map[string]bool
	Created                                                              int64
}
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}
type Grant struct {
	ID         string   `json:"id"`
	ClientID   string   `json:"client_id"`
	ClientName string   `json:"client_name"`
	Scopes     []string `json:"scopes"`
	Created    int64    `json:"created_at"`
}

func New(store Store, cfg Config) (*Service, error) {
	u, e := url.Parse(cfg.PublicAPIURL)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "https" && !(u.Scheme == "http" && loopbackHost(u.Hostname()))) {
		return nil, fmt.Errorf("OAuth public API URL must be canonical HTTPS (HTTP loopback development only)")
	}
	if store == nil || cfg.User == nil {
		return nil, errors.New("OAuth durable store and user resolver are required")
	}
	issuer := strings.TrimRight(u.String(), "/")
	if cfg.WebURL == "" {
		cfg.WebURL = issuer
	}
	w, e := url.Parse(cfg.WebURL)
	if e != nil || w.Host == "" || w.User != nil || w.RawQuery != "" || w.Fragment != "" || (w.Scheme != "https" && !(w.Scheme == "http" && loopbackHost(w.Hostname()))) {
		return nil, errors.New("invalid OAuth web URL")
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"account:read", "models:read", "images:read", "images:write", "videos:read", "videos:write", "videos:delete", "assets:read", "assets:write", "assets:delete", "uploads:read", "uploads:write", "uploads:delete", "projects:read", "projects:write", "projects:delete", "groups:read", "groups:write", "groups:delete"}
	}
	return &Service{store: store, cfg: cfg, issuer: issuer, resource: issuer + "/mcp"}, nil
}
func loopbackHost(h string) bool { return h == "localhost" || h == "127.0.0.1" || h == "::1" }
func secret() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func hash(s string) string { b := sha256.Sum256([]byte(s)); return hex.EncodeToString(b[:]) }
func document(id, kind string, uid int64, v any) Record {
	b, _ := json.Marshal(v)
	return Record{ID: id, Kind: kind, UserID: uid, Payload: b, Version: 1}
}
func (s *Service) save(ctx context.Context, r Record, v any) error {
	r.Payload, _ = json.Marshal(v)
	return s.store.CompareSwap(ctx, r)
}
func (s *Service) active(ctx context.Context, id int64) (User, error) {
	u, e := s.cfg.User(ctx, id)
	if e != nil {
		return User{}, e
	}
	if !u.Active {
		return User{}, ErrInvalid
	}
	return u, nil
}
func (s *Service) Issuer() string   { return s.issuer }
func (s *Service) Resource() string { return s.resource }
func (s *Service) ConsentURL(id string) string {
	return strings.TrimRight(s.cfg.WebURL, "/") + "/?oauth_request=" + url.QueryEscape(id) + "#oauth-consent"
}
func (s *Service) Metadata() map[string]any {
	return map[string]any{"issuer": s.issuer, "authorization_endpoint": s.issuer + "/oauth/authorize", "token_endpoint": s.issuer + "/oauth/token", "revocation_endpoint": s.issuer + "/oauth/revoke", "registration_endpoint": s.issuer + "/oauth/register", "response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"}, "token_endpoint_auth_methods_supported": []string{"none"}, "code_challenge_methods_supported": []string{"S256"}, "scopes_supported": s.cfg.Scopes, "client_id_metadata_document_supported": true}
}
func (s *Service) ProtectedMetadata() map[string]any {
	return map[string]any{"resource": s.resource, "authorization_servers": []string{s.issuer}, "scopes_supported": s.cfg.Scopes, "bearer_methods_supported": []string{"header"}}
}
func scopes(input string, allowed []string) ([]string, error) {
	a := map[string]bool{}
	for _, x := range allowed {
		a[x] = true
	}
	out := []string{}
	seen := map[string]bool{}
	for _, x := range strings.Fields(input) {
		if !a[x] {
			return nil, ErrInvalidScope
		}
		if !seen[x] {
			out = append(out, x)
			seen[x] = true
		}
	}
	if len(out) == 0 {
		return nil, ErrInvalidScope
	}
	sort.Strings(out)
	return out, nil
}
func (s *Service) Begin(ctx context.Context, q url.Values) (string, error) {
	if q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" || q.Get("resource") != s.resource || len(q.Get("state")) > 2048 {
		return "", ErrInvalid
	}
	for _, k := range []string{"client_id", "redirect_uri", "resource", "response_type", "scope", "code_challenge", "code_challenge_method", "state"} {
		if len(q[k]) > 1 {
			return "", ErrInvalid
		}
	}
	ch, e := base64.RawURLEncoding.DecodeString(q.Get("code_challenge"))
	if e != nil || len(ch) != 32 {
		return "", ErrInvalid
	}
	c, e := s.Client(ctx, q.Get("client_id"))
	if e != nil {
		return "", e
	}
	ok := false
	for _, r := range c.RedirectURIs {
		if r == q.Get("redirect_uri") {
			ok = true
		}
	}
	if !ok {
		return "", ErrInvalid
	}
	sc, e := scopes(q.Get("scope"), s.cfg.Scopes)
	if e != nil {
		return "", e
	}
	a := Authorization{ID: secret(), Client: c, RedirectURI: q.Get("redirect_uri"), Resource: s.resource, Scopes: sc, State: q.Get("state"), Challenge: q.Get("code_challenge"), Expires: time.Now().Add(10 * time.Minute).Unix()}
	return a.ID, s.store.Create(ctx, document("request:"+a.ID, "request", 0, a))
}
func (s *Service) Consent(ctx context.Context, id string, uid int64) (Authorization, string, error) {
	if _, e := s.active(ctx, uid); e != nil {
		return Authorization{}, "", e
	}
	for i := 0; i < 8; i++ {
		r, e := s.store.Get(ctx, "request:"+id)
		if e != nil {
			if errors.Is(e, ErrNotFound) {
				return Authorization{}, "", ErrInvalid
			}
			return Authorization{}, "", e
		}
		var a Authorization
		if json.Unmarshal(r.Payload, &a) != nil || a.Used || (a.UserID != 0 && a.UserID != uid) {
			return Authorization{}, "", ErrInvalid
		}
		if a.Expires <= time.Now().Unix() {
			return Authorization{}, "", ErrAuthorizationExpired
		}
		csrf := secret()
		a.UserID = uid
		a.CSRFHash = hash(csrf)
		e = s.save(ctx, r, a)
		if errors.Is(e, ErrConflict) {
			continue
		}
		return a, csrf, e
	}
	return Authorization{}, "", ErrConflict
}
func (s *Service) Decide(ctx context.Context, id string, uid int64, csrf string, allow bool) (string, error) {
	if _, e := s.active(ctx, uid); e != nil {
		return "", e
	}
	r, e := s.store.Get(ctx, "request:"+id)
	if e != nil {
		if errors.Is(e, ErrNotFound) {
			return "", ErrInvalid
		}
		return "", e
	}
	var a Authorization
	if json.Unmarshal(r.Payload, &a) != nil || a.Used || a.UserID != uid || a.CSRFHash != hash(csrf) || csrf == "" {
		return "", ErrInvalid
	}
	if a.Expires <= time.Now().Unix() {
		return "", ErrAuthorizationExpired
	}
	a.Used = true
	if e = s.save(ctx, r, a); e != nil {
		return "", e
	}
	u, _ := url.Parse(a.RedirectURI)
	q := u.Query()
	if a.State != "" {
		q.Set("state", a.State)
	}
	if !allow {
		q.Set("error", "access_denied")
	} else {
		code := secret()
		f := family{ID: secret(), ClientID: a.Client.ID, ClientName: a.Client.Name, UserID: uid, RedirectURI: a.RedirectURI, Resource: a.Resource, Challenge: a.Challenge, CodeHash: hash(code), CodeExpires: time.Now().Add(2 * time.Minute).Unix(), Scopes: a.Scopes, Access: map[string]int64{}, Refresh: map[string]int64{}, UsedRefresh: map[string]bool{}, Created: time.Now().Unix()}
		if e = s.store.Create(ctx, document("family:"+f.ID, "family", uid, f)); e != nil {
			return "", e
		}
		if e = s.store.Create(ctx, document("code:"+hash(code), "index", uid, f.ID)); e != nil {
			return "", e
		}
		q.Set("code", code)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
func (s *Service) Token(ctx context.Context, q url.Values) (TokenResponse, error) {
	for k, v := range q {
		if len(v) != 1 || k == "client_secret" {
			return TokenResponse{}, ErrInvalidRequest
		}
	}
	typ := q.Get("grant_type")
	raw := q.Get("code")
	prefix := "code:"
	if typ == "refresh_token" {
		raw = q.Get("refresh_token")
		prefix = "refresh:"
	} else if typ != "authorization_code" {
		return TokenResponse{}, ErrUnsupportedGrant
	}
	if q.Get("resource") != s.resource {
		return TokenResponse{}, ErrInvalidTarget
	}
	if raw == "" || q.Get("client_id") == "" {
		return TokenResponse{}, ErrInvalidRequest
	}
	idx, e := s.store.Get(ctx, prefix+hash(raw))
	if e != nil {
		if errors.Is(e, ErrNotFound) {
			return TokenResponse{}, ErrInvalid
		}
		return TokenResponse{}, e
	}
	var id string
	if json.Unmarshal(idx.Payload, &id) != nil {
		return TokenResponse{}, ErrInvalid
	}
	for i := 0; i < 12; i++ {
		r, e := s.store.Get(ctx, "family:"+id)
		if e != nil {
			if errors.Is(e, ErrNotFound) {
				return TokenResponse{}, ErrInvalid
			}
			return TokenResponse{}, e
		}
		var f family
		if json.Unmarshal(r.Payload, &f) != nil || f.Revoked || f.ClientID != q.Get("client_id") || f.Resource != q.Get("resource") {
			return TokenResponse{}, ErrInvalid
		}
		if _, e = s.active(ctx, f.UserID); e != nil {
			return TokenResponse{}, e
		}
		now := time.Now().Unix()
		if typ == "authorization_code" {
			v := q.Get("code_verifier")
			valid := len(v) >= 43 && len(v) <= 128
			for _, c := range v {
				if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~", c) {
					valid = false
				}
			}
			sum := sha256.Sum256([]byte(v))
			if !valid || f.CodeUsed || f.CodeExpires <= now || f.CodeHash != hash(raw) || f.RedirectURI != q.Get("redirect_uri") || f.Challenge != base64.RawURLEncoding.EncodeToString(sum[:]) {
				return TokenResponse{}, ErrInvalid
			}
			if q.Get("scope") != "" {
				requested, err := scopes(q.Get("scope"), f.Scopes)
				if err != nil || strings.Join(requested, " ") != strings.Join(f.Scopes, " ") {
					return TokenResponse{}, ErrInvalid
				}
			}
			f.CodeUsed = true
		} else {
			h := hash(raw)
			if f.UsedRefresh[h] {
				f.Revoked = true
				e = s.save(ctx, r, f)
				if errors.Is(e, ErrConflict) {
					continue
				}
				if e != nil {
					return TokenResponse{}, e
				}
				return TokenResponse{}, ErrInvalid
			}
			if f.Refresh[h] <= now {
				return TokenResponse{}, ErrInvalid
			}
			if q.Get("scope") != "" {
				sc, e := scopes(q.Get("scope"), f.Scopes)
				if e != nil {
					return TokenResponse{}, e
				}
				f.Scopes = sc
			}
			delete(f.Refresh, h)
			f.UsedRefresh[h] = true
		}
		for h, expires := range f.Access {
			if expires <= now {
				delete(f.Access, h)
			}
		}
		access, refresh := secret(), secret()
		f.Access[hash(access)] = now + 600
		f.Refresh[hash(refresh)] = now + 30*24*3600
		// Indexes are created before committing the family. Uncommitted tokens cannot validate.
		if e = s.store.Create(ctx, document("access:"+hash(access), "index", f.UserID, f.ID)); e != nil {
			return TokenResponse{}, e
		}
		if e = s.store.Create(ctx, document("refresh:"+hash(refresh), "index", f.UserID, f.ID)); e != nil {
			return TokenResponse{}, e
		}
		e = s.save(ctx, r, f)
		if errors.Is(e, ErrConflict) {
			continue
		}
		if e != nil {
			return TokenResponse{}, e
		}
		return TokenResponse{access, "Bearer", 600, refresh, strings.Join(f.Scopes, " ")}, nil
	}
	return TokenResponse{}, ErrConflict
}
func (s *Service) ValidateAccess(ctx context.Context, token string) (Principal, error) {
	idx, e := s.store.Get(ctx, "access:"+hash(token))
	if e != nil {
		return Principal{}, ErrInvalid
	}
	var id string
	_ = json.Unmarshal(idx.Payload, &id)
	r, e := s.store.Get(ctx, "family:"+id)
	if e != nil {
		return Principal{}, ErrInvalid
	}
	var f family
	if json.Unmarshal(r.Payload, &f) != nil || f.Revoked || f.Resource != s.resource || f.Access[hash(token)] <= time.Now().Unix() {
		return Principal{}, ErrInvalid
	}
	u, e := s.active(ctx, f.UserID)
	if e != nil {
		return Principal{}, e
	}
	return Principal{UserID: f.UserID, ClientID: f.ClientID, GrantID: f.ID, Scopes: f.Scopes, GroupCode: u.GroupCode}, nil
}
func (s *Service) Disconnect(ctx context.Context, uid int64, id string) error {
	for i := 0; i < 12; i++ {
		r, e := s.store.Get(ctx, "family:"+id)
		if e != nil {
			if errors.Is(e, ErrNotFound) {
				return ErrInvalid
			}
			return e
		}
		var f family
		if json.Unmarshal(r.Payload, &f) != nil || f.UserID != uid {
			return ErrInvalid
		}
		f.Revoked = true
		e = s.save(ctx, r, f)
		if errors.Is(e, ErrConflict) {
			continue
		}
		return e
	}
	return ErrConflict
}
func (s *Service) Revoke(ctx context.Context, token, client string) error {
	for _, prefix := range []string{"access:", "refresh:"} {
		r, e := s.store.Get(ctx, prefix+hash(token))
		if e != nil {
			if errors.Is(e, ErrNotFound) {
				continue
			}
			return e
		}
		var id string
		_ = json.Unmarshal(r.Payload, &id)
		r, e = s.store.Get(ctx, "family:"+id)
		if e != nil {
			if errors.Is(e, ErrNotFound) {
				continue
			}
			return e
		}
		var f family
		if json.Unmarshal(r.Payload, &f) == nil && f.ClientID == client {
			return s.Disconnect(ctx, f.UserID, id)
		}
	}
	return nil
}
func (s *Service) Grants(ctx context.Context, uid int64) ([]Grant, error) {
	if _, e := s.active(ctx, uid); e != nil {
		return nil, e
	}
	rs, e := s.store.List(ctx, "family", uid)
	out := []Grant{}
	for _, r := range rs {
		var f family
		if json.Unmarshal(r.Payload, &f) == nil && !f.Revoked && f.CodeUsed {
			out = append(out, Grant{f.ID, f.ClientID, f.ClientName, f.Scopes, f.Created})
		}
	}
	return out, e
}

// ValidateGrant rechecks a delegated capability against the current durable
// grant. Callers must additionally enforce their own signed capability expiry.
func (s *Service) ValidateGrant(ctx context.Context, userID int64, clientID, grantID string, requested []string) error {
	r, err := s.store.Get(ctx, "family:"+grantID)
	if err != nil {
		return ErrInvalid
	}
	var f family
	if json.Unmarshal(r.Payload, &f) != nil || f.Revoked || !f.CodeUsed || f.ID != grantID || f.UserID != userID || f.ClientID != clientID || f.Resource != s.resource {
		return ErrInvalid
	}
	if _, err = s.active(ctx, userID); err != nil {
		return err
	}
	allowed := map[string]bool{}
	for _, scope := range f.Scopes {
		allowed[scope] = true
	}
	for _, scope := range requested {
		if !allowed[scope] {
			return ErrInvalid
		}
	}
	now := time.Now().Unix()
	live := false
	for _, expires := range f.Access {
		if expires > now {
			live = true
		}
	}
	for _, expires := range f.Refresh {
		if expires > now {
			live = true
		}
	}
	if !live {
		return ErrInvalid
	}
	return nil
}
