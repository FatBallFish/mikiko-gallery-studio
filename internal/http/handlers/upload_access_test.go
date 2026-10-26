package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/fatballfish/pic-gallery/internal/service/uploadaccess"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/google/uuid"
)

type uploadAccessBackend struct {
	session mediaasset.UploadSession
	bytes   int
}

func (b *uploadAccessBackend) SignPart(context.Context, int64, uuid.UUID, int, string) (storage.MultipartPartTarget, error) {
	return storage.MultipartPartTarget{}, storage.ErrDirectUploadRequired
}
func (b *uploadAccessBackend) GetUploadSession(context.Context, int64, uuid.UUID) (mediaasset.UploadSession, error) {
	return b.session, nil
}
func (b *uploadAccessBackend) UploadPart(_ context.Context, _ int64, _ uuid.UUID, part int, r io.Reader, size int64, checksum string) (storage.CompletedPart, error) {
	raw, err := io.ReadAll(r)
	b.bytes += len(raw)
	if err != nil || int64(len(raw)) != size {
		return storage.CompletedPart{}, storage.ErrSizeMismatch
	}
	return storage.CompletedPart{PartNumber: part, ETag: checksum, SizeBytes: size}, nil
}
func TestUploadAccessHandler(t *testing.T) {
	b := &uploadAccessBackend{session: mediaasset.UploadSession{ID: uuid.New(), UserID: 1, PartSize: 4, PartCount: 1, DeclaredSizeBytes: 4, ExpiresAt: time.Now().Add(time.Hour), Status: "initialized"}}
	s, err := uploadaccess.New(b, uploadaccess.Options{PublicAPIURL: "https://studio.example", SigningKey: []byte(strings.Repeat("k", 32)), ValidatePrincipal: func(context.Context, openplatform.Principal) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	checksum := sha256.Sum256([]byte("data"))
	target, err := s.Target(t.Context(), openplatform.Principal{UserID: 1, ClientID: "client", GrantID: "grant", Scopes: []string{"uploads:write"}}, b.session.ID, 1, hex.EncodeToString(checksum[:]))
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(target.URL)
	// Exercise the real outer handler: capability routing must precede its API Key actor.
	native := NewOpenPlatform(nil, func(*http.Request) (openplatform.Principal, error) {
		t.Fatal("capability required API Key actor")
		return openplatform.Principal{}, nil
	})
	native.SetUploadAccess(NewUploadAccessHandler(s))
	for _, tt := range []struct {
		name, method, path, body string
		length                   int64
		token                    bool
		status                   int
	}{
		{"success", "PUT", u.Path, "data", 4, true, 200},
		{"method", "POST", u.Path, "data", 4, true, 405},
		{"missing capability", "PUT", u.Path, "data", 4, false, 403},
		{"query token", "PUT", u.Path + "?token=secret", "data", 4, true, 400},
		{"unknown length", "PUT", u.Path, "data", -1, true, 400},
		{"large declaration", "PUT", u.Path, "data", (32 << 20) + 1, true, 400},
		{"wrong size", "PUT", u.Path, "data", 3, true, 400},
		{"short body", "PUT", u.Path, "dat", 4, true, 400},
		{"extra bytes", "PUT", u.Path, "dataEXTRA", 4, true, 400},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			r.ContentLength = tt.length
			for k, v := range target.Headers {
				r.Header.Set(k, v)
			}
			if !tt.token {
				r.Header.Del(uploadaccess.Header)
			}
			w := httptest.NewRecorder()
			native.ServeHTTP(w, r)
			if w.Code != tt.status {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tt.status, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("missing no-store")
			}
		})
	}
	if b.bytes != 11 {
		t.Fatalf("body bound not enforced, bytes=%d want=11", b.bytes)
	}
}
