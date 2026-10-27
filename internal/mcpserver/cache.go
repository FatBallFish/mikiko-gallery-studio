package mcpserver

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	snapshotTTL          = 2 * time.Minute
	snapshotMax          = 8 << 20
	snapshotGlobalMax    = 64 << 20
	snapshotPrincipalMax = 16 << 20
)

type snapshotKey struct {
	UserID                                                    int64
	APIKeyID, ClientID, GrantID, GroupCode, CredentialVersion string
	Scopes                                                    string
}
type snapshot struct {
	data      []byte
	expires   time.Time
	principal snapshotKey
	scope     string
}
type snapshotCache struct {
	mu              sync.Mutex
	items           map[string]snapshot
	byPrincipal     map[snapshotKey]int
	total, maxItems int
}

func newSnapshotCache() *snapshotCache {
	return &snapshotCache{items: map[string]snapshot{}, byPrincipal: map[snapshotKey]int{}, maxItems: 4096}
}
func principalSnapshotKey(p openplatform.Principal) snapshotKey {
	scopes := append([]string(nil), p.Scopes...)
	slices.Sort(scopes)
	return snapshotKey{p.UserID, p.APIKeyID, p.ClientID, p.GrantID, p.GroupCode, p.CredentialVersion, strings.Join(scopes, " ")}
}
func (c *snapshotCache) token() (string, error) {
	var b [24]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", errors.New("snapshot token unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}
func (c *snapshotCache) put(p openplatform.Principal, scope string, v any) (string, error) {
	d, e := json.Marshal(v)
	if e != nil {
		return "", e
	}
	if len(d) > snapshotMax {
		return "", errors.New("snapshot exceeds maximum size")
	}
	k := principalSnapshotKey(p)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cleanupLocked(time.Now())
	if len(c.items) >= c.maxItems || c.byPrincipal[k]+len(d) > snapshotPrincipalMax || c.total+len(d) > snapshotGlobalMax {
		return "", errors.New("snapshot capacity unavailable")
	}
	t, e := c.token()
	if e != nil {
		return "", e
	}
	c.items[t] = snapshot{d, time.Now().Add(snapshotTTL), k, scope}
	c.byPrincipal[k] += len(d)
	c.total += len(d)
	return t, nil
}
func (c *snapshotCache) get(t string, p openplatform.Principal, scope string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cleanupLocked(time.Now())
	s, ok := c.items[t]
	if !ok {
		return nil, errors.New("snapshot expired or not found")
	}
	if s.principal != principalSnapshotKey(p) || !slices.Contains(p.Scopes, s.scope) {
		return nil, errors.New("snapshot access denied")
	}
	return append([]byte(nil), s.data...), nil
}
func (c *snapshotCache) cleanupLocked(now time.Time) {
	for t, s := range c.items {
		if now.Before(s.expires) {
			continue
		}
		delete(c.items, t)
		c.total -= len(s.data)
		c.byPrincipal[s.principal] -= len(s.data)
		if c.byPrincipal[s.principal] <= 0 {
			delete(c.byPrincipal, s.principal)
		}
	}
}
func jsonPointer(root any, raw string) (any, error) {
	if raw == "" {
		return root, nil
	}
	if !strings.HasPrefix(raw, "/") {
		return nil, errors.New("path must be RFC6901 JSON pointer")
	}
	cur := root
	for _, part := range strings.Split(raw[1:], "/") {
		for i := 0; i < len(part); i++ {
			if part[i] == '~' {
				if i+1 >= len(part) || (part[i+1] != '0' && part[i+1] != '1') {
					return nil, errors.New("invalid RFC6901 escape")
				}
				i++
			}
		}
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch v := cur.(type) {
		case map[string]any:
			n, ok := v[part]
			if !ok {
				return nil, errors.New("path not found")
			}
			cur = n
		case []any:
			if part == "" || (len(part) > 1 && part[0] == '0') {
				return nil, errors.New("invalid array index")
			}
			for j := 0; j < len(part); j++ {
				if part[j] < '0' || part[j] > '9' {
					return nil, errors.New("invalid array index")
				}
			}
			i, e := strconv.Atoi(part)
			if e != nil || i < 0 || i >= len(v) {
				return nil, errors.New("path not found")
			}
			cur = v[i]
		default:
			return nil, errors.New("path not found")
		}
	}
	return cur, nil
}
