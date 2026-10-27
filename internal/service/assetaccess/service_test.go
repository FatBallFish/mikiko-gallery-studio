package assetaccess

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/google/uuid"
)

type testBackend struct {
	asset   mediaasset.Asset
	direct  string
	purpose string
}

func (b *testBackend) GetAsset(context.Context, int64, uuid.UUID) (mediaasset.Asset, error) {
	return b.asset, nil
}
func (b *testBackend) Access(context.Context, int64, uuid.UUID, string) (mediaasset.AccessResult, error) {
	return mediaasset.AccessResult{URL: b.direct, Range: true, ExpiresAt: time.Now().Add(time.Minute)}, nil
}
func (b *testBackend) OpenContentRange(_ context.Context, _ int64, _ uuid.UUID, p, v string, h bool) (mediaasset.RangedContentStream, error) {
	b.purpose = p
	return mediaasset.RangedContentStream{}, nil
}
func TestCapabilityBindingsAndRevocation(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	b := &testBackend{asset: mediaasset.Asset{ID: uuid.New(), UserID: 7, Status: "ready"}, direct: "/api/agent/media/v1/assets/browser/content"}
	revoked := false
	disabled := false
	currentVersion := "credential-v1"
	opts := Options{PublicAPIURL: "https://studio.example", SigningKey: []byte(strings.Repeat("k", 32)), Now: func() time.Time { return now }, ValidatePrincipal: func(_ context.Context, p openplatform.Principal) error {
		if revoked || disabled || p.APIKeyID != "key-1" || p.CredentialVersion != currentVersion {
			return errors.New("revoked")
		}
		return nil
	}}
	s, err := New(b, opts)
	if err != nil {
		t.Fatal(err)
	}
	p := openplatform.Principal{UserID: 7, APIKeyID: "key-1", CredentialVersion: currentVersion, Scopes: []string{"images:read"}}
	for _, invalid := range []openplatform.Principal{
		{UserID: 7, APIKeyID: "key-1", Scopes: []string{"images:read"}},
		{UserID: 7, APIKeyID: "key-1", CredentialVersion: currentVersion},
		{UserID: 7, ClientID: "client", GrantID: "grant"},
		{UserID: 7, ClientID: "client", GrantID: "grant", Scopes: []string{"models:read"}},
	} {
		if _, err := s.Access(ctx, invalid, b.asset.ID, "download"); err == nil {
			t.Fatal("missing credential version or resource scope accepted")
		}
	}
	raw, err := s.Access(ctx, p, b.asset.ID, "download")
	if err != nil {
		t.Fatal(err)
	}
	access := raw.(mediaasset.AccessResult)
	if !strings.HasPrefix(access.URL, opts.PublicAPIURL+PathPrefix) || strings.Contains(access.URL, "?") {
		t.Fatal(access.URL)
	}
	token := strings.TrimSuffix(strings.TrimPrefix(access.URL, opts.PublicAPIURL+PathPrefix), "/content")
	restarted, _ := New(b, opts)
	if _, err = restarted.OpenContent(ctx, token, "", true); err != nil {
		t.Fatal("stable key:", err)
	}
	if b.purpose != "download" {
		t.Fatal(b.purpose)
	}
	parts := strings.Split(token, ".")
	body, _ := base64.RawURLEncoding.DecodeString(parts[0])
	var c claims
	_ = json.Unmarshal(body, &c)
	for _, mutate := range []func(*claims){func(c *claims) { c.Purpose = "preview" }, func(c *claims) { c.AssetID = uuid.New() }, func(c *claims) { c.Principal.UserID = 8 }, func(c *claims) { c.Principal.APIKeyID = "key-2" }, func(c *claims) { c.Principal.GrantID = "other" }} {
		changed := c
		mutate(&changed)
		payload, _ := json.Marshal(changed)
		if _, err = s.OpenContent(ctx, base64.RawURLEncoding.EncodeToString(payload)+"."+parts[1], "", false); err == nil {
			t.Fatal("tamper accepted")
		}
	}
	revoked = true
	if _, err = s.OpenContent(ctx, token, "", true); err == nil {
		t.Fatal("revoked key accepted")
	}
	revoked = false
	disabled = true
	if _, err = s.OpenContent(ctx, token, "", true); err == nil {
		t.Fatal("disabled principal accepted")
	}
	disabled = false
	currentVersion = "credential-v2"
	if _, err = s.OpenContent(ctx, token, "", true); err == nil {
		t.Fatal("rotated credential accepted")
	}
	currentVersion = p.CredentialVersion
	b.asset.UserID = 8
	if _, err = s.OpenContent(ctx, token, "", true); err == nil {
		t.Fatal("changed owner accepted")
	}
	b.asset.UserID = 7
	b.asset.DeletedAt = &now
	if _, err = s.OpenContent(ctx, token, "", true); err == nil {
		t.Fatal("deleted asset accepted")
	}
	b.asset.DeletedAt = nil
	now = access.ExpiresAt
	if _, err = s.OpenContent(ctx, token, "", true); err == nil {
		t.Fatal("expired accepted")
	}
}
func TestPresignedAccessUsesRevocableProxy(t *testing.T) {
	b := &testBackend{asset: mediaasset.Asset{ID: uuid.New(), UserID: 7, Status: "ready"}, direct: "https://s3.example/private/object?X-Amz-Signature=resource"}
	revoked := false
	s, _ := New(b, Options{PublicAPIURL: "https://studio.example", SigningKey: []byte(strings.Repeat("k", 32)), ValidatePrincipal: func(context.Context, openplatform.Principal) error {
		if revoked {
			return errors.New("revoked")
		}
		return nil
	}})
	raw, err := s.Access(context.Background(), openplatform.Principal{UserID: 7, APIKeyID: "key", CredentialVersion: "v1", Scopes: []string{"assets:read"}}, b.asset.ID, "download")
	if err != nil {
		t.Fatal(err)
	}
	access := raw.(mediaasset.AccessResult)
	if !strings.HasPrefix(access.URL, "https://studio.example"+PathPrefix) || strings.Contains(access.URL, "X-Amz") {
		t.Fatal(access.URL)
	}
	token := strings.TrimSuffix(strings.TrimPrefix(access.URL, "https://studio.example"+PathPrefix), "/content")
	revoked = true
	if _, err := s.OpenContent(context.Background(), token, "", true); err == nil {
		t.Fatal("S3 proxy bypassed revocation")
	}
}
func TestInternalPreviewURLBindings(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	revoked := false
	b := &testBackend{asset: mediaasset.Asset{ID: uuid.New(), UserID: 7, Status: "ready"}}
	s, _ := New(b, Options{PublicAPIURL: "https://studio.example", SigningKey: []byte(strings.Repeat("k", 32)), Now: func() time.Time { return now }, ValidatePrincipal: func(context.Context, openplatform.Principal) error {
		if revoked {
			return errors.New("revoked")
		}
		return nil
	}})
	p := openplatform.Principal{UserID: 7, ClientID: "client", GrantID: "grant", Scopes: []string{"images:read"}}
	raw, err := s.Access(ctx, p, b.asset.ID, "preview")
	if err != nil {
		t.Fatal(err)
	}
	access := raw.(mediaasset.AccessResult)
	if _, err = s.OpenPreviewURL(ctx, p, access.URL); err != nil {
		t.Fatal(err)
	}
	for _, url := range []string{access.URL + "?x=1", access.URL + "#x", strings.Replace(access.URL, "studio.example", "evil.example", 1), strings.Replace(access.URL, "https://", "https://user@", 1), access.URL + "/content"} {
		if _, err = s.OpenPreviewURL(ctx, p, url); err == nil {
			t.Fatal("invalid URL accepted")
		}
	}
	other := p
	other.GrantID = "other"
	if _, err = s.OpenPreviewURL(ctx, other, access.URL); err == nil {
		t.Fatal("other principal accepted")
	}
	download, _ := s.Access(ctx, p, b.asset.ID, "download")
	if _, err = s.OpenPreviewURL(ctx, p, download.(mediaasset.AccessResult).URL); err == nil {
		t.Fatal("download accepted")
	}
	revoked = true
	if _, err = s.OpenPreviewURL(ctx, p, access.URL); err == nil {
		t.Fatal("revoked accepted")
	}
	revoked = false
	now = access.ExpiresAt
	if _, err = s.OpenPreviewURL(ctx, p, access.URL); err == nil {
		t.Fatal("expired accepted")
	}
}

func TestCanonicalPublicURL(t *testing.T) {
	for _, base := range []string{"http://evil.example", "https://u:p@studio.example", "https://studio.example?token=x", "https://studio.example/path", "https://studio.example#x"} {
		if _, err := New(&testBackend{}, Options{PublicAPIURL: base, SigningKey: []byte(strings.Repeat("k", 32))}); err == nil {
			t.Fatal(base)
		}
	}
	for _, base := range []string{"http://localhost:3000", "http://127.0.0.1:3000", "https://studio.example"} {
		if _, err := New(&testBackend{}, Options{PublicAPIURL: base, SigningKey: []byte(strings.Repeat("k", 32))}); err != nil {
			t.Fatal(err)
		}
	}
}
