package router

import (
	"database/sql"
	"encoding/json"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
	"github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/service/videopublication"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

func TestVideoPublicationAnonymousHTTPAccess(t *testing.T) {
	testVideoPublicationHTTPAccess(t, "")
}

func TestVideoPublicationAdminHTTPPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("PIC_GALLERY_TEST_POSTGRES_URL"))
	if databaseURL == "" {
		t.Skip("set PIC_GALLERY_TEST_POSTGRES_URL to run PostgreSQL video moderation integration")
	}
	admin, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	schema := "video_publication_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.ExecContext(t.Context(), "CREATE SCHEMA "+pq.QuoteIdentifier(schema)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP SCHEMA IF EXISTS " + pq.QuoteIdentifier(schema) + " CASCADE") })
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	testVideoPublicationHTTPAccess(t, parsed.String())
}

func testVideoPublicationHTTPAccess(t *testing.T, databaseURL string) {
	t.Helper()
	ctx := t.Context()
	auth, owner := loginTestUser(t, "public-video-owner@example.com")
	claims, e := auth.ParseAccessToken(owner.AccessToken)
	if e != nil {
		t.Fatal(e)
	}
	driver := dialect.Postgres
	if databaseURL == "" {
		driver = dialect.SQLite
		databaseURL = "file:publication-http-" + uuid.NewString() + "?mode=memory&cache=shared&_fk=1"
	}
	client, e := repoent.Open(driver, databaseURL)
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
	if e = backend.Put(ctx, "original", "video/mp4", []byte("original01")); e != nil {
		t.Fatal(e)
	}
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
	adminStore := entstore.NewAdminAuthStore(client)
	if _, e = adminStore.CreateAdmin(ctx, domainadminauth.AdminUser{Email: "admin-users@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: "super_admin", Status: "active"}); e != nil {
		t.Fatal(e)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	api := handlers.NewAPIWithCompletionServices(cfg, auth, nil, nil, adminConfig, nil, nil, adminAuth, nil)
	api.SetVideoPublications(service)
	handler := NewWithAPI(api)
	adminToken := loginAdminForUserTest(t, handler)
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
	reviewPath := "/api/ops/admin/v1/video-reviews/" + asset.ID.String()
	moderate := func(action, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		w := authenticatedMediaRequest(t, handler, adminToken, "POST", reviewPath+":"+action, body, nil)
		if w.Code != want {
			t.Fatalf("admin %s: want %d, got %d %s", action, want, w.Code, w.Body.String())
		}
		return w
	}
	for _, action := range []string{"approve", "reject"} {
		if w := request("POST", reviewPath+":"+action, ""); w.Code != 401 {
			t.Fatalf("anonymous %s=%d", action, w.Code)
		}
	}
	if w := authenticatedMediaRequest(t, handler, owner.AccessToken, "POST", reviewPath+":approve", `{}`, nil); w.Code != 401 && w.Code != 403 {
		t.Fatalf("owner moderate=%d", w.Code)
	}
	moderate("reject", `{"reason":"  "}`, 400)
	rejected := moderate("reject", `{"reason":"  copyright  "}`, 200)
	var decision struct {
		Data videopublication.Record `json:"data"`
	}
	if e = json.Unmarshal(rejected.Body.Bytes(), &decision); e != nil || decision.Data.Status != "rejected" || decision.Data.ReviewReason != "copyright" || decision.Data.PublishedAt != nil {
		t.Fatalf("rejected response=%s err=%v", rejected.Body.String(), e)
	}
	moderate("reject", `{"reason":"copyright"}`, 409)
	moderate("approve", `{}`, 409)
	for _, suffix := range []string{"", "/video", "/poster"} {
		if w := request("GET", base+suffix, ""); w.Code != 404 {
			t.Fatalf("rejected %s=%d", suffix, w.Code)
		}
	}
	if w := request("GET", "/api/public/v1/videos", ""); w.Code != 200 || strings.Contains(w.Body.String(), asset.ID.String()) {
		t.Fatalf("rejected listed=%d %s", w.Code, w.Body.String())
	}
	if w := authenticatedMediaRequest(t, handler, owner.AccessToken, "POST", "/api/agent/media/v1/video-publications/"+asset.ID.String(), "", nil); w.Code != 200 {
		t.Fatalf("resubmit=%d %s", w.Code, w.Body.String())
	}
	approved := moderate("approve", `{}`, 200)
	decision.Data = videopublication.Record{}
	if e = json.Unmarshal(approved.Body.Bytes(), &decision); e != nil || decision.Data.Status != "approved" || decision.Data.PublishedAt == nil || decision.Data.ReviewReason != "" {
		t.Fatalf("approved response=%s err=%v", approved.Body.String(), e)
	}

	// Reactions are authenticated, explicitly set and idempotent.
	for _, kind := range []string{"like", "favorite"} {
		if w := request("POST", base+"/"+kind, ""); w.Code != 401 {
			t.Fatalf("anonymous %s=%d", kind, w.Code)
		}
		for i := 0; i < 2; i++ {
			w := authenticatedMediaRequest(t, handler, owner.AccessToken, "POST", base+"/"+kind, `{"active":true}`, nil)
			if w.Code != 200 {
				t.Fatalf("reaction %s=%d %s", kind, w.Code, w.Body.String())
			}
		}
	}
	if w := authenticatedMediaRequest(t, handler, owner.AccessToken, "POST", base+"/like", `{}`, nil); w.Code != 400 {
		t.Fatalf("missing active=%d", w.Code)
	}
	viewer := authenticatedMediaRequest(t, handler, owner.AccessToken, "GET", base, "", nil)
	if !strings.Contains(viewer.Body.String(), `"liked_by_viewer":true`) || !strings.Contains(viewer.Body.String(), `"like_count":1`) {
		t.Fatalf("viewer state %s", viewer.Body.String())
	}
	for _, method := range []string{"GET", "HEAD"} {
		if w := request(method, base+"/download", "bytes=2-5"); w.Code != 401 {
			t.Fatalf("anonymous download %s=%d", method, w.Code)
		}
		w := authenticatedMediaRequest(t, handler, owner.AccessToken, method, base+"/download", "", map[string]string{"Range": "bytes=2-5"})
		if w.Code != 206 || w.Header().Get("Content-Range") != "bytes 2-5/10" || w.Header().Get("Content-Length") != "4" {
			t.Fatalf("download %s=%d %s", method, w.Code, w.Body.String())
		}
		disposition, params, err := mime.ParseMediaType(w.Header().Get("Content-Disposition"))
		if err != nil || disposition != "attachment" || params["filename"] != "Public clip.mp4" {
			t.Fatalf("download disposition %s %v", w.Header().Get("Content-Disposition"), err)
		}
		if method == "GET" && w.Body.String() != "igin" {
			t.Fatalf("download original body=%s", w.Body.String())
		}
		if method == "HEAD" && w.Body.Len() != 0 {
			t.Fatal("HEAD body")
		}
	}
	invalidDownload := authenticatedMediaRequest(t, handler, owner.AccessToken, "GET", base+"/download", "", map[string]string{"Range": "bytes=99-"})
	if invalidDownload.Code != 416 || invalidDownload.Header().Get("Content-Range") != "bytes */10" {
		t.Fatalf("invalid download range %d %s", invalidDownload.Code, invalidDownload.Body.String())
	}
	// Withdrawal revokes every download range and reaction before storage access.
	if _, err := service.Withdraw(ctx, claims.UserID, asset.ID); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"GET", "HEAD"} {
		w := authenticatedMediaRequest(t, handler, owner.AccessToken, method, base+"/download", "", map[string]string{"Range": "bytes=2-5"})
		if w.Code != 404 {
			t.Fatalf("withdrawn download %s=%d", method, w.Code)
		}
	}
	if w := authenticatedMediaRequest(t, handler, owner.AccessToken, "POST", base+"/like", `{"active":true}`, nil); w.Code != 404 {
		t.Fatalf("withdrawn reaction=%d", w.Code)
	}
	if w := authenticatedMediaRequest(t, handler, owner.AccessToken, "POST", "/api/agent/media/v1/video-publications/"+asset.ID.String(), "", nil); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	moderate("approve", `{}`, 200)
	moderate("approve", `{}`, 409)
	moderate("reject", `{"reason":"copyright"}`, 409)
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
