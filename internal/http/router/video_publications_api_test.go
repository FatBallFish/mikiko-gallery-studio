package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
	"github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/service/videopublication"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/google/uuid"
)

func TestVideoPublicationAnonymousHTTPAccess(t *testing.T) {
	ctx := t.Context()
	auth, owner := loginTestUser(t, "public-video-owner@example.com")
	claims, e := auth.ParseAccessToken(owner.AccessToken)
	if e != nil {
		t.Fatal(e)
	}
	client, e := repoent.Open(dialect.SQLite, "file:publication-http-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = client.Close() })
	if e = client.Schema.Create(ctx); e != nil {
		t.Fatal(e)
	}
	project, e := client.Project.Create().SetUserID(claims.UserID).SetName("private project").SetNameKey("private").Save(ctx)
	if e != nil {
		t.Fatal(e)
	}
	asset, e := client.MediaAsset.Create().SetUserID(claims.UserID).SetProjectID(project.ID).SetName("Public clip").SetNameKey("clip").SetMediaType("video").SetSourceType("upload").SetStatus("ready").SetStorageDriver("local").SetObjectKey("original").SetMimeType("video/mp4").SetFileSizeBytes(10).Save(ctx)
	if e != nil {
		t.Fatal(e)
	}
	backend := storage.NewLocalBackend(t.TempDir())
	for _, kind := range []string{"proxy", "poster"} {
		if e = backend.Put(ctx, kind, "video/mp4", []byte("0123456789")); e != nil {
			t.Fatal(e)
		}
		if _, e = client.MediaDerivative.Create().SetAssetID(asset.ID).SetKind(kind).SetStatus("ready").SetStorageDriver("local").SetObjectKey(kind).SetMimeType("video/mp4").Save(ctx); e != nil {
			t.Fatal(e)
		}
	}
	media := mediaasset.NewService(entstore.NewMediaStore(client), storage.NewStaticRouter(backend), mediaasset.Options{})
	service := videopublication.NewService(entstore.NewVideoPublicationStore(client), media)
	cfg := taskAPIConfig("http://provider.invalid")
	adminConfig := adminconfigservice.NewService(cfg)
	api := handlers.NewAPIWithRuntimeServices(cfg, auth, nil, nil, adminConfig, nil)
	api.SetVideoPublications(service)
	handler := NewWithAPI(api)
	request := func(method, path, rangeValue string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		if rangeValue != "" {
			r.Header.Set("Range", rangeValue)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	base := "/api/public/v1/videos/" + asset.ID.String()
	for _, suffix := range []string{"", "/video", "/poster"} {
		if w := request("GET", base+suffix, ""); w.Code != 404 {
			t.Fatalf("private %s =%d %s", suffix, w.Code, w.Body.String())
		}
	}
	if w := request("POST", "/api/agent/media/v1/video-publications/"+asset.ID.String(), ""); w.Code != 401 {
		t.Fatalf("anonymous submit=%d", w.Code)
	}
	submit := authenticatedMediaRequest(t, handler, owner.AccessToken, "POST", "/api/agent/media/v1/video-publications/"+asset.ID.String(), "", nil)
	if submit.Code != 200 {
		t.Fatalf("submit=%d %s", submit.Code, submit.Body.String())
	}
	if w := request("POST", "/api/ops/admin/v1/video-reviews/"+asset.ID.String()+":approve", ""); w.Code != 401 {
		t.Fatalf("anonymous moderate=%d", w.Code)
	}
	if _, e = service.Review(ctx, asset.ID, "approved", ""); e != nil {
		t.Fatal(e)
	}
	for _, test := range []struct {
		method, suffix, rangeValue, body string
		status                           int
	}{
		{"GET", "/video", "bytes=2-5", "2345", 206}, {"GET", "/video", "bytes=-3", "789", 206}, {"HEAD", "/video", "bytes=2-5", "", 206}, {"GET", "/poster", "", "0123456789", 200}, {"GET", "/video", "bytes=99-", "", 416},
	} {
		w := request(test.method, base+test.suffix, test.rangeValue)
		if w.Code != test.status || w.Body.String() != test.body {
			t.Fatalf("%+v got %d %s", test, w.Code, w.Body.String())
		}
	}
	detail := request("GET", base, "")
	if detail.Code != http.StatusOK {
		t.Fatal(detail.Body.String())
	}
	for _, secret := range []string{"project_id", "owner_id", "user_id", "access_token", "source_task", "storage_driver", "object_key", "review_reason"} {
		if strings.Contains(detail.Body.String(), secret) {
			t.Fatalf("public leak %s", detail.Body.String())
		}
	}
	if w := request("GET", "/api/public/v1/videos", ""); w.Code != 200 || !strings.Contains(w.Body.String(), asset.ID.String()) {
		t.Fatalf("list=%d %s", w.Code, w.Body.String())
	}
	if _, err := adminConfig.UpdateTab(ctx, domainadminconfig.UpdateTabRequest{TabKey: "public_gallery", Version: 1, UpdatedBy: 1, Items: []domainadminconfig.Item{{ConfigCategory: "public_gallery", ConfigKey: "gallery_enabled", ConfigValue: map[string]any{"value": false}, Scope: "global"}}}); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"GET", "HEAD"} {
		for _, suffix := range []string{"", "/video", "/poster"} {
			if w := request(method, base+suffix, "bytes=-3"); w.Code != 404 {
				t.Fatalf("disabled %s %s =%d %s", method, suffix, w.Code, w.Body.String())
			}
		}
	}
	if w := request("GET", "/api/public/v1/videos", ""); w.Code != 200 || strings.Contains(w.Body.String(), asset.ID.String()) || !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatalf("disabled list=%d %s", w.Code, w.Body.String())
	}
	if _, e = service.Withdraw(ctx, claims.UserID, asset.ID); e != nil {
		t.Fatal(e)
	}
	if w := request("GET", base+"/video", ""); w.Code != 404 {
		t.Fatalf("withdraw playback=%d", w.Code)
	}
}
