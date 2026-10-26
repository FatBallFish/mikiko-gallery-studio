package uploadaccess

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/google/uuid"
)

type backend struct {
	session mediaasset.UploadSession
	direct  bool
	writes  int
}

func (b *backend) SignPart(context.Context, int64, uuid.UUID, int, string) (storage.MultipartPartTarget, error) {
	if b.direct {
		return storage.MultipartPartTarget{URL: "https://s3.example/signed"}, nil
	}
	return storage.MultipartPartTarget{}, storage.ErrDirectUploadRequired
}
func (b *backend) GetUploadSession(_ context.Context, user int64, id uuid.UUID) (mediaasset.UploadSession, error) {
	if user != b.session.UserID || id != b.session.ID {
		return mediaasset.UploadSession{}, errors.New("not owned")
	}
	return b.session, nil
}
func (b *backend) UploadPart(_ context.Context, _ int64, _ uuid.UUID, number int, r io.Reader, size int64, checksum string) (storage.CompletedPart, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return storage.CompletedPart{}, err
	}
	if int64(len(raw)) != size {
		return storage.CompletedPart{}, storage.ErrSizeMismatch
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != checksum {
		return storage.CompletedPart{}, storage.ErrMultipartChecksum
	}
	b.writes++
	return storage.CompletedPart{PartNumber: number, SizeBytes: size, Checksum: checksum}, nil
}
func TestCapabilityConstraints(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	b := &backend{session: mediaasset.UploadSession{ID: uuid.New(), UserID: 7, PartSize: 4, PartCount: 2, DeclaredSizeBytes: 8, Status: "initialized", ExpiresAt: now.Add(time.Minute)}}
	revoked := false
	s, err := New(b, Options{PublicAPIURL: "https://studio.example", SigningKey: []byte(strings.Repeat("s", 32)), Now: func() time.Time { return now }, ValidatePrincipal: func(context.Context, openplatform.Principal) error {
		if revoked {
			return errors.New("revoked")
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	p := openplatform.Principal{UserID: 7, APIKeyID: "1", CredentialVersion: "version1", Scopes: []string{"uploads:write"}}
	sum := sha256.Sum256([]byte("data"))
	checksum := hex.EncodeToString(sum[:])
	target, err := s.Target(t.Context(), p, b.session.ID, 1, checksum)
	if err != nil {
		t.Fatal(err)
	}
	if !target.ExpiresAt.Equal(b.session.ExpiresAt) {
		t.Fatal("expiry exceeds session")
	}
	u, _ := url.Parse(target.URL)
	token := target.Headers[Header]
	if u.RawQuery != "" || strings.Contains(target.URL, token) {
		t.Fatal("token leaked into URL")
	}
	put := func(token, method, path string, size int64, checksum, body string) error {
		_, err := s.Put(t.Context(), token, method, path, size, checksum, strings.NewReader(body))
		return err
	}
	if err := put(token, "PUT", u.Path, 4, checksum, "data"); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, token, method, path string
		size                      int64
		checksum, body            string
	}{
		{"tampered", token + "x", "PUT", u.Path, 4, checksum, "data"},
		{"method", token, "POST", u.Path, 4, checksum, "data"},
		{"part", token, "PUT", strings.TrimSuffix(u.Path, "1") + "2", 4, checksum, "data"},
		{"session", token, "PUT", PathPrefix + uuid.NewString() + "/parts/1", 4, checksum, "data"},
		{"audience", token, "PUT", "https://evil.example" + u.Path, 4, checksum, "data"},
		{"size", token, "PUT", u.Path, 5, checksum, "data!"},
		{"checksum header", token, "PUT", u.Path, 4, strings.Repeat("0", 64), "data"},
		{"corrupt bytes", token, "PUT", u.Path, 4, checksum, "xxxx"},
		{"short", token, "PUT", u.Path, 4, checksum, "dat"},
		{"oversized", token, "PUT", u.Path, 4, checksum, "data!"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := put(tt.token, tt.method, tt.path, tt.size, tt.checksum, tt.body); err == nil {
				t.Fatal("accepted invalid capability upload")
			}
		})
	}
	for _, state := range []string{"aborted", "completed", "completing", "expired"} {
		b.session.Status = state
		if err := put(token, "PUT", u.Path, 4, checksum, "data"); err == nil {
			t.Fatalf("accepted %s", state)
		}
	}
	b.session.Status = "initialized"
	revoked = true
	if err := put(token, "PUT", u.Path, 4, checksum, "data"); err == nil {
		t.Fatal("accepted revoked")
	}
	revoked = false
	b.session.UserID = 8
	if err := put(token, "PUT", u.Path, 4, checksum, "data"); err == nil {
		t.Fatal("accepted changed owner")
	}
	b.session.UserID = 7
	now = now.Add(time.Minute)
	if err := put(token, "PUT", u.Path, 4, checksum, "data"); err == nil {
		t.Fatal("accepted expired")
	}
	if b.writes != 1 {
		t.Fatalf("invalid requests wrote bytes %d", b.writes)
	}
	b.direct = true
	direct, err := s.Target(t.Context(), p, b.session.ID, 1, checksum)
	if err != nil || direct.URL != "https://s3.example/signed" || len(direct.Headers) != 0 {
		t.Fatalf("direct signing changed: %#v %v", direct, err)
	}
}
