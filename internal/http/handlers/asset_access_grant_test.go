package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/service/assetaccess"
	"github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/google/uuid"
)

type grantAssetStore struct {
	mediaasset.Store
	asset mediaasset.Asset
}

func (s *grantAssetStore) GetAsset(context.Context, int64, uuid.UUID) (mediaasset.Asset, error) {
	return s.asset, nil
}
func TestAssetAccessGrantHEADRange(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("0123456789"), 0600); err != nil {
		t.Fatal(err)
	}
	store := &grantAssetStore{asset: mediaasset.Asset{ID: uuid.New(), UserID: 7, Name: "sample.txt", ObjectKey: "file", MIMEType: "text/plain", Status: "ready"}}
	media := mediaasset.NewService(store, storage.NewStaticRouter(storage.NewLocalBackend(root)), mediaasset.Options{})
	s, err := assetaccess.New(media, assetaccess.Options{PublicAPIURL: "https://studio.example", SigningKey: []byte(strings.Repeat("k", 32))})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := s.Access(context.Background(), openplatform.Principal{UserID: 7, APIKeyID: "key", CredentialVersion: "v1", Scopes: []string{"assets:read"}}, store.asset.ID, "download")
	if err != nil {
		t.Fatal(err)
	}
	access := raw.(mediaasset.AccessResult)
	handler := NewAssetAccessGrantHandler(s)
	for _, tc := range []struct {
		method, value    string
		status           int
		body, length, cr string
	}{
		{"GET", "bytes=3-5", 206, "345", "3", "bytes 3-5/10"},
		{"HEAD", "bytes=-2", 206, "", "2", "bytes 8-9/10"},
		{"HEAD", "", 200, "", "10", ""},
		{"GET", "", 200, "0123456789", "10", ""},
		{"GET", "bytes=99-", 416, "", "", "bytes */10"},
	} {
		r := httptest.NewRequest(tc.method, access.URL, nil)
		r.Header.Set("Range", tc.value)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s %s: %d %s", tc.method, tc.value, w.Code, w.Body.String())
		}
		if tc.status != 416 && (w.Body.String() != tc.body || w.Header().Get("Content-Length") != tc.length) {
			t.Fatalf("%s: body=%q headers=%v", tc.method, w.Body.String(), w.Header())
		}
		if w.Header().Get("Content-Range") != tc.cr {
			t.Fatal(w.Header())
		}
		if tc.status != 416 && !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment") {
			t.Fatal(w.Header())
		}
	}
	for _, suffix := range []string{"?purpose=preview", "?asset_id=" + uuid.NewString(), "/another"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, access.URL+suffix, nil))
		if w.Code != 403 {
			t.Fatal(w.Code)
		}
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, strings.TrimSuffix(access.URL, "/content"), nil))
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
}
