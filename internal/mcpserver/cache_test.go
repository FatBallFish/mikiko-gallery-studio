package mcpserver

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
)

func cacheFixturePrincipal() openplatform.Principal {
	return openplatform.Principal{UserID: 1, APIKeyID: "api", ClientID: "client", GrantID: "grant", GroupCode: "group", CredentialVersion: "v1", Scopes: []string{"models:read", "account:read"}}
}

func TestSnapshotPrincipalIdentityAndScopeIsolation(t *testing.T) {
	p := cacheFixturePrincipal()
	originalScopes := append([]string(nil), p.Scopes...)
	c := newSnapshotCache()
	token, err := c.put(p, "models:read", map[string]any{"id": "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	reordered := p
	reordered.Scopes = []string{"account:read", "models:read"}
	if principalSnapshotKey(p) != principalSnapshotKey(reordered) {
		t.Error("scope order changes snapshot identity")
	}
	if !reflect.DeepEqual(p.Scopes, originalScopes) {
		t.Fatal("identity calculation mutated caller scopes")
	}
	data, err := c.get(token, reordered, "")
	if err != nil {
		t.Fatalf("same principal with reordered scopes denied: %v", err)
	}
	// Returned bytes must not permit mutation of the cached immutable snapshot.
	data[0] = '!'
	again, err := c.get(token, p, "")
	if err != nil || !json.Valid(again) {
		t.Fatalf("cached snapshot mutated through returned bytes: %v", err)
	}
	for _, tc := range []struct {
		name   string
		change func(*openplatform.Principal)
	}{
		{"user", func(p *openplatform.Principal) { p.UserID++ }},
		{"api key", func(p *openplatform.Principal) { p.APIKeyID = "other" }},
		{"client", func(p *openplatform.Principal) { p.ClientID = "other" }},
		{"grant", func(p *openplatform.Principal) { p.GrantID = "other" }},
		{"group", func(p *openplatform.Principal) { p.GroupCode = "other" }},
		{"credential version", func(p *openplatform.Principal) { p.CredentialVersion = "v2" }},
		{"missing original scope", func(p *openplatform.Principal) { p.Scopes = []string{"account:read"} }},
		{"expanded scope set", func(p *openplatform.Principal) { p.Scopes = []string{"models:read", "account:read", "images:read"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := p
			tc.change(&changed)
			if _, err := c.get(token, changed, ""); err == nil {
				t.Fatal("snapshot accessible across principal identity boundary")
			}
		})
	}
	if _, err := c.get("nonexistent-fixture", p, ""); err == nil {
		t.Fatal("unknown token accepted")
	}
}

func TestSnapshotExpiryAndEntryCapacity(t *testing.T) {
	c := newSnapshotCache()
	c.maxItems = 1
	p := cacheFixturePrincipal()
	before := time.Now()
	token, err := c.put(p, "models:read", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	s := c.items[token]
	if s.expires.Before(before.Add(snapshotTTL)) || s.expires.After(time.Now().Add(snapshotTTL)) {
		t.Error("snapshot expiry does not use configured TTL")
	}
	if _, err := c.put(p, "models:read", "second"); err == nil {
		t.Fatal("entry capacity not enforced")
	}
	total := c.total
	if total != len(s.data) || c.byPrincipal[principalSnapshotKey(p)] != total || len(c.items) != 1 {
		t.Fatal("failed put changed cache accounting")
	}
	s.expires = time.Now().Add(-time.Second)
	c.items[token] = s
	if _, err := c.get(token, p, ""); err == nil {
		t.Fatal("expired snapshot accepted")
	}
	if len(c.items) != 0 || c.total != 0 || len(c.byPrincipal) != 0 {
		t.Fatal("expiry did not release entry and byte capacity")
	}
	if _, err := c.put(p, "models:read", "replacement"); err != nil {
		t.Fatalf("expired entry prevented replacement: %v", err)
	}
}

func TestSnapshotByteCapacity(t *testing.T) {
	p := cacheFixturePrincipal()
	for _, tc := range []struct {
		name  string
		setup func(*snapshotCache)
		value any
	}{
		{"single snapshot", func(*snapshotCache) {}, strings.Repeat("x", snapshotMax)},
		{"principal bytes", func(c *snapshotCache) { c.byPrincipal[principalSnapshotKey(p)] = snapshotPrincipalMax }, "small"},
		{"global bytes", func(c *snapshotCache) { c.total = snapshotGlobalMax }, "small"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newSnapshotCache()
			tc.setup(c)
			total, principalBytes := c.total, c.byPrincipal[principalSnapshotKey(p)]
			if _, err := c.put(p, "models:read", tc.value); err == nil {
				t.Fatal("byte capacity exceeded without error")
			}
			if len(c.items) != 0 || c.total != total || c.byPrincipal[principalSnapshotKey(p)] != principalBytes {
				t.Fatal("failed put consumed cache capacity")
			}
		})
	}
}

func TestJSONPointerStrictPaths(t *testing.T) {
	root := map[string]any{"array": []any{"zero", "one"}, "a/b": map[string]any{"~": "escaped"}, "": "empty-key", "scalar": 1}
	for _, tc := range []struct {
		path string
		want any
	}{
		{"", root}, {"/array/0", "zero"}, {"/array/1", "one"}, {"/a~1b/~0", "escaped"}, {"/", "empty-key"},
	} {
		got, err := jsonPointer(root, tc.path)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("valid pointer %q failed: %v", tc.path, err)
		}
	}
	for _, path := range []string{"array/0", "/missing", "/a~2b", "/a~", "/array/", "/array/+0", "/array/+1", "/array/-0", "/array/-1", "/array/00", "/array/01", "/array/1.0", "/array/ 0", "/array/-", "/array/2", "/array/99999999999999999999999", "/scalar/child"} {
		t.Run(path, func(t *testing.T) {
			if _, err := jsonPointer(root, path); err == nil {
				t.Fatal("invalid pointer accepted")
			}
		})
	}
	// Escaping must round-trip without decoding ~01 twice.
	for _, key := range []string{"a/b", "~", "~1", "", "中文/~"} {
		root := map[string]any{key: "value"}
		got, err := jsonPointer(root, pointerChild("", key))
		if err != nil || got != "value" {
			t.Errorf("pointerChild round trip for %q: %v", key, err)
		}
	}
}

func TestSnapshotLargeIntegerSurvivesRead(t *testing.T) {
	const integer = "9007199254740993"
	c := newSnapshotCache()
	p := cacheFixturePrincipal()
	token, err := c.put(p, "models:read", map[string]any{"id": json.Number(integer)})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := c.get(token, p, "")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeResult(raw)
	if err != nil {
		t.Fatal(err)
	}
	value, err := jsonPointer(decoded, "/id")
	if err != nil {
		t.Fatal(err)
	}
	if n, ok := value.(json.Number); !ok || n.String() != integer {
		t.Fatal("snapshot lost exact integer larger than 2^53")
	}
	page, err := readPage(token, "/id", 0, 1, value)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(envelopeResult(page))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(wire, []byte(integer)) || bytes.Contains(wire, []byte("9007199254740992")) {
		t.Fatal("wire result rounded large integer")
	}
}
