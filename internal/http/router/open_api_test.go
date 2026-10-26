package router

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/fatballfish/pic-gallery/internal/config"
	domainapikey "github.com/fatballfish/pic-gallery/internal/domain/apikey"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	apikeyservice "github.com/fatballfish/pic-gallery/internal/service/apikey"
	assetservice "github.com/fatballfish/pic-gallery/internal/service/assets"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	"github.com/fatballfish/pic-gallery/internal/service/capabilities"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	"github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/fatballfish/pic-gallery/internal/service/project"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/google/uuid"
)

type openAPICredentials struct {
	UserID                       int64
	AccessKey, Secret, ProjectID string
}

func openRequest(t *testing.T, h http.Handler, c openAPICredentials, method, path, body string, status int) map[string]any {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+c.Secret)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != status {
		t.Fatalf("%s %s: want %d got %d: %s", method, path, status, rec.Code, rec.Body.String())
	}
	var out struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.Data
}

func openImageInput(c openAPICredentials) map[string]any {
	return map[string]any{"project_id": c.ProjectID, "task_type": "text_to_image", "prompt": "cat", "abstract_model": "plus", "base_resolution": "auto", "requested_size": "1536x1024", "output_image_count": 1}
}
func openJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestOpenImagePreflightCreateGetModelsAndBalanceUseBearer(t *testing.T) {
	h, c, bills := newOpenAPIHandler(t)
	for _, path := range []string{"/api/open/v1/me", "/api/open/v1/me/balance", "/api/open/v1/image-models"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+c.Secret)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
	}
	input := openImageInput(c)
	quote := openRequest(t, h, c, "POST", "/api/open/v1/image-tasks/preflight", openJSON(t, input), 200)
	if quote["estimated_points"] != "8.00000" || quote["quote_token"] == "" || quote["expires_at"] == nil {
		t.Fatalf("invalid quote %#v", quote)
	}
	changed := openImageInput(c)
	changed["prompt"] = "another prompt"
	changed["quote_token"] = quote["quote_token"]
	changed["idempotency_key"] = "changed-quote"
	openRequest(t, h, c, "POST", "/api/open/v1/image-tasks", openJSON(t, changed), 409)
	before, _ := bills.GetBalance(t.Context(), c.UserID, "1.00000")
	if before.AvailablePoints != "100.00000" || before.FrozenPoints != "0.00000" {
		t.Fatalf("preflight charged %#v", before)
	}
	input["quote_token"] = quote["quote_token"]
	input["idempotency_key"] = "open-idem"
	first := openRequest(t, h, c, "POST", "/api/open/v1/image-tasks", openJSON(t, input), 201)
	second := openRequest(t, h, c, "POST", "/api/open/v1/image-tasks", openJSON(t, input), 201)
	id, ok := first["id"].(string)
	if !ok || id == "" || second["id"] != id {
		t.Fatalf("replay mismatch %#v %#v", first, second)
	}
	detail := openRequest(t, h, c, "GET", "/api/open/v1/image-tasks/"+id, "", 200)
	if detail["id"] != id || detail["status"] != "queued" {
		t.Fatalf("invalid detail %#v", detail)
	}
	openRequest(t, h, c, "GET", "/api/open/v1/image-tasks?project_id="+c.ProjectID, "", 200)
	after, _ := bills.GetBalance(t.Context(), c.UserID, "1.00000")
	if after.AvailablePoints != "92.00000" || after.FrozenPoints != "8.00000" {
		t.Fatalf("expected single reserve %#v", after)
	}
	input["prompt"] = "changed"
	openRequest(t, h, c, "POST", "/api/open/v1/image-tasks", openJSON(t, input), 409)
}

func TestOpenImageAPIRejectsLegacyRoutesAndSignatureOnlyAuthentication(t *testing.T) {
	h, c, _ := newOpenAPIHandler(t)
	for _, path := range []string{"/api/open/image/v1/estimate", "/api/open/image/v1/tasks", "/api/open/image/v1/reference-assets", "/api/open/image/v1/reference-assets/uploads"} {
		for _, method := range []string{"GET", "POST"} {
			openRequest(t, h, c, method, path, "{}", 404)
		}
	}
	body := openJSON(t, openImageInput(c))
	req := httptest.NewRequest("POST", "/api/open/v1/image-tasks/preflight", strings.NewReader(body))
	timestamp := time.Now().UTC()
	hash := apikeyservice.BodySHA256([]byte(body))
	req.Header.Set("X-Access-Key", c.AccessKey)
	req.Header.Set("X-Timestamp", timestamp.Format(time.RFC3339))
	req.Header.Set("X-Body-SHA256", hash)
	req.Header.Set("X-Signature", apikeyservice.SignCanonicalHMAC(c.Secret, req.Method, req.URL.RequestURI(), timestamp, hash))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("signature-only auth accepted: %d %s", rec.Code, rec.Body.String())
	}
}

type failOnReadBody struct{ reads int }

func (b *failOnReadBody) Read([]byte) (int, error) {
	b.reads++
	return 0, errors.New("body read before authentication")
}
func TestOpenImageAPIRejectsMissingOrUnknownBearerBeforeReadingBody(t *testing.T) {
	h, _, _ := newOpenAPIHandler(t)
	for _, token := range []string{"", "Bearer sk-does-not-exist"} {
		body := &failOnReadBody{}
		req := httptest.NewRequest("POST", "/api/open/v1/uploads", body)
		req.Header.Set("Authorization", token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 401 || body.reads != 0 {
			t.Fatalf("auth=%q status=%d reads=%d body=%s", token, rec.Code, body.reads, rec.Body.String())
		}
	}
}
func TestOpenImagePreflightAndCreateRejectLegacyFieldsAndMissingQuotes(t *testing.T) {
	h, c, _ := newOpenAPIHandler(t)
	for _, field := range []string{"requested_output_image_count", "response_mode", "reference_image_count"} {
		input := openImageInput(c)
		input[field] = 1
		for _, path := range []string{"/api/open/v1/image-tasks/preflight", "/api/open/v1/image-tasks"} {
			openRequest(t, h, c, "POST", path, openJSON(t, input), 400)
		}
	}
	input := openImageInput(c)
	input["task_type"] = "reference_generate"
	openRequest(t, h, c, "POST", "/api/open/v1/image-tasks/preflight", openJSON(t, input), 400)
	input = openImageInput(c)
	input["idempotency_key"] = "missing-quote"
	openRequest(t, h, c, "POST", "/api/open/v1/image-tasks", openJSON(t, input), 409)
}
func TestOpenImageTaskRejectsAPIKeyQuotaExceeded(t *testing.T) {
	daily, total := "1.00000", "100.00000"
	h, c, _ := newOpenAPIHandlerWithCreateRequest(t, apikeyservice.CreateRequest{Name: "limited", Secret: "sk-limited", GroupCode: "plus", TotalQuotaPoints: &total, DailyQuotaPoints: &daily})
	input := openImageInput(c)
	openRequest(t, h, c, "POST", "/api/open/v1/image-tasks/preflight", openJSON(t, input), 403)
	input["idempotency_key"] = "quota"
	openRequest(t, h, c, "POST", "/api/open/v1/image-tasks", openJSON(t, input), 403)
}
func TestOpenAndCompatAPIRejectDisabledUserAPIKey(t *testing.T) {
	h, c, auth, _ := newOpenAPIHandlerWithAuth(t, apikeyservice.CreateRequest{Name: "disabled", GroupCode: "plus", Secret: "sk-disabled"})
	auth.DisableUserForTest(c.UserID)
	for _, path := range []string{"/api/open/v1/me", "/v1/models"} {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Authorization", "Bearer "+c.Secret)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 403 {
			t.Fatalf("disabled account %s: %d %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestOpenUploadsUseBearerMultipartLifecycle(t *testing.T) {
	h, c, _ := newOpenAPIHandler(t)
	raw := tinyPNG(t)
	sum := sha256.Sum256(raw)
	checksum := hex.EncodeToString(sum[:])
	input := map[string]any{"project_id": c.ProjectID, "filename": "tiny.png", "mime_type": "image/png", "media_type": "image", "size_bytes": len(raw), "checksum": checksum, "idempotency_key": "upload"}
	upload := openRequest(t, h, c, "POST", "/api/open/v1/uploads", openJSON(t, input), 201)
	id, ok := upload["id"].(string)
	if !ok {
		t.Fatalf("upload missing id %#v", upload)
	}
	openRequest(t, h, c, "GET", "/api/open/v1/uploads/"+id, "", 200)
	missingChecksum := httptest.NewRequest("PUT", "/api/open/v1/uploads/"+id+"/parts/1", bytes.NewReader(raw))
	missingChecksum.Header.Set("Authorization", "Bearer "+c.Secret)
	invalid := httptest.NewRecorder()
	h.ServeHTTP(invalid, missingChecksum)
	if invalid.Code != 400 {
		t.Fatalf("missing checksum: %d %s", invalid.Code, invalid.Body.String())
	}
	wrongChecksum := httptest.NewRequest("PUT", "/api/open/v1/uploads/"+id+"/parts/1", bytes.NewReader(raw))
	wrongChecksum.Header.Set("Authorization", "Bearer "+c.Secret)
	wrongChecksum.Header.Set("X-Checksum-SHA256", strings.Repeat("0", 64))
	invalid = httptest.NewRecorder()
	h.ServeHTTP(invalid, wrongChecksum)
	if invalid.Code != 400 {
		t.Fatalf("incorrect checksum: %d %s", invalid.Code, invalid.Body.String())
	}
	req := httptest.NewRequest("PUT", "/api/open/v1/uploads/"+id+"/parts/1", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+c.Secret)
	req.Header.Set("X-Checksum-SHA256", checksum)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("part: %d %s", rec.Code, rec.Body.String())
	}
	var part struct {
		Data storage.CompletedPart `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &part); err != nil {
		t.Fatal(err)
	}
	completed := openRequest(t, h, c, "POST", "/api/open/v1/uploads/"+id+"/complete", openJSON(t, map[string]any{"parts": []storage.CompletedPart{part.Data}}), 200)
	if completed == nil {
		t.Fatal("empty upload completion")
	}
	input["idempotency_key"] = "abort"
	abandoned := openRequest(t, h, c, "POST", "/api/open/v1/uploads", openJSON(t, input), 201)
	openRequest(t, h, c, "POST", "/api/open/v1/uploads/"+abandoned["id"].(string)+"/abort", "{}", 200)
}

func TestOpenProjectDeleteAfterCompletedUploadsAndPendingTransfer(t *testing.T) {
	h, c, _ := newOpenAPIHandler(t) // Real SQLite with foreign keys enabled.
	created := openRequest(t, h, c, "POST", "/api/open/v1/projects", `{"name":"Upload cleanup"}`, 201)
	projectID := created["id"].(string)
	projectPath := "/api/open/v1/projects/" + projectID
	raw := tinyPNG(t)
	sum := sha256.Sum256(raw)
	checksum := hex.EncodeToString(sum[:])
	input := map[string]any{"project_id": projectID, "filename": "cleanup.png", "mime_type": "image/png", "media_type": "image", "size_bytes": len(raw), "checksum": checksum}
	for _, key := range []string{"completed-rest", "completed-mcp"} {
		input["idempotency_key"] = key
		upload := openRequest(t, h, c, "POST", "/api/open/v1/uploads", openJSON(t, input), 201)
		uploadPath := "/api/open/v1/uploads/" + upload["id"].(string)
		req := httptest.NewRequest("PUT", uploadPath+"/parts/1", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+c.Secret)
		req.Header.Set("X-Checksum-SHA256", checksum)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("upload part: %d %s", rec.Code, rec.Body.String())
		}
		var part struct {
			Data storage.CompletedPart `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &part); err != nil {
			t.Fatal(err)
		}
		asset := openRequest(t, h, c, "POST", uploadPath+"/complete", openJSON(t, map[string]any{"parts": []storage.CompletedPart{part.Data}}), 200)
		openRequest(t, h, c, "DELETE", "/api/open/v1/assets/"+asset["id"].(string), openJSON(t, map[string]any{"expected_version": asset["version"]}), 200)
	}
	current := openRequest(t, h, c, "GET", projectPath, "", 200)
	if current["asset_count"] != float64(0) {
		t.Fatalf("softdeleted assets counted: %#v", current)
	}
	// Completed uploads and their deleted assets must not block deletion or cause FK errors.
	openRequest(t, h, c, "DELETE", projectPath, `{"expected_version":1}`, 200)
	created = openRequest(t, h, c, "POST", "/api/open/v1/projects", `{"name":"Pending cleanup"}`, 201)
	projectID = created["id"].(string)
	projectPath = "/api/open/v1/projects/" + projectID
	input["project_id"] = projectID
	// A pending session is deliberately counted although it has no listed asset.
	input["idempotency_key"] = "pending"
	pending := openRequest(t, h, c, "POST", "/api/open/v1/uploads", openJSON(t, input), 201)
	current = openRequest(t, h, c, "GET", projectPath, "", 200)
	if current["asset_count"] != float64(1) {
		t.Fatalf("pending count: %#v", current)
	}
	openRequest(t, h, c, "DELETE", projectPath, `{"expected_version":1}`, 409)
	openRequest(t, h, c, "DELETE", projectPath, `{}`, 400)
	openRequest(t, h, c, "DELETE", "/api/open/v1/projects/"+c.ProjectID, `{"expected_version":1}`, 403)
	openRequest(t, h, c, "DELETE", projectPath, openJSON(t, map[string]any{"expected_version": 1, "target_project_id": c.ProjectID, "idempotency_key": "cleanup-transfer"}), 200)
	transferred := openRequest(t, h, c, "GET", "/api/open/v1/uploads/"+pending["id"].(string), "", 200)
	if transferred["project_id"] != c.ProjectID {
		t.Fatalf("pending upload not transferred: %#v", transferred)
	}
	openRequest(t, h, c, "POST", "/api/open/v1/uploads/"+pending["id"].(string)+"/abort", `{}`, 200)
	// Terminal sessions and softdeleted assets retain their references safely.
	empty := openRequest(t, h, c, "POST", "/api/open/v1/projects", `{"name":"Terminal cleanup"}`, 201)
	input["project_id"], input["idempotency_key"] = empty["id"], "terminal"
	terminal := openRequest(t, h, c, "POST", "/api/open/v1/uploads", openJSON(t, input), 201)
	openRequest(t, h, c, "POST", "/api/open/v1/uploads/"+terminal["id"].(string)+"/abort", `{}`, 200)
	openRequest(t, h, c, "DELETE", "/api/open/v1/projects/"+empty["id"].(string), `{"expected_version":1}`, 200)
}

func newOpenAPIHandler(t *testing.T) (http.Handler, openAPICredentials, *billingservice.Service) {
	t.Helper()
	return newOpenAPIHandlerWithCreateRequest(t, apikeyservice.CreateRequest{Name: "openapi", GroupCode: "plus", Secret: "sk-openapi-secret", Scopes: domainapikey.SupportedScopes()})
}
func newOpenAPIHandlerWithCreateRequest(t *testing.T, req apikeyservice.CreateRequest) (http.Handler, openAPICredentials, *billingservice.Service) {
	h, c, _, b := newOpenAPIHandlerWithAuth(t, req)
	return h, c, b
}
func newOpenAPIHandlerWithAuth(t *testing.T, req apikeyservice.CreateRequest) (http.Handler, openAPICredentials, *authservice.Service, *billingservice.Service) {
	return newOpenAPIHandlerWithAuthAndConfig(t, req, nil)
}
func newOpenAPIHandlerWithAuthAndConfig(t *testing.T, createReq apikeyservice.CreateRequest, configure func(*config.Config)) (http.Handler, openAPICredentials, *authservice.Service, *billingservice.Service) {
	t.Helper()
	cfg := taskAPIConfig("http://127.0.0.1:1")
	cfg.Storage.LocalRoot = t.TempDir()
	if configure != nil {
		configure(&cfg)
	}
	auth := authservice.NewService(config.AuthConfig{AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: 2 * time.Hour, Issuer: "test", AccessTokenSecret: "secret", RefreshCookieName: "pg_refresh"}, map[string]string{"basic": "1.00000", "plus": "1.00000"})
	if err := auth.SendEmailCode("openapi@example.com", "login"); err != nil {
		t.Fatal(err)
	}
	user, _, err := loginAuthUserWithPasswordSetup(t, auth, "openapi@example.com", "123456")
	if err != nil {
		t.Fatal(err)
	}
	bills := billingservice.NewService(cfg.Billing)
	if _, err = bills.AdminAdjust(context.Background(), domainbilling.AdjustRequest{UserID: user.ID, ChangePoints: "100.00000", Reason: "seed balance"}); err != nil {
		t.Fatal(err)
	}
	keys := apikeyservice.NewService(nil)
	keys.SetUsageReader(bills)
	createReq.UserID = user.ID
	created, err := keys.CreateKey(t.Context(), createReq)
	if err != nil {
		t.Fatal(err)
	}
	client, err := repoent.Open(dialect.SQLite, "file:"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err = client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	projects := project.NewService(entstore.NewProjectStore(client))
	selected, err := projects.EnsureDefault(t.Context(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	refs := assetservice.NewService(cfg.Storage, cfg.GenerationLimits)
	tasks := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), refs, bills)
	tasks.SetAPIKeyUsageManager(keys)
	tasks.SetProjectResolver(projects)
	media := mediaasset.NewService(entstore.NewMediaStore(client), storage.NewStaticRouter(storage.NewLocalBackend(t.TempDir())), mediaasset.Options{})
	api := handlers.NewAPIWithRuntimeServices(cfg, auth, refs, tasks, nil, bills, keys)
	api.SetProjectService(projects)
	shared := openplatform.New(openplatform.Dependencies{Auth: auth, Billing: bills, Images: tasks, ImageModels: capabilities.NewService(cfg), References: refs, Projects: projects, Assets: media, APIKeys: keys, QuoteKey: []byte("01234567890123456789012345678901")})
	actor := func(r *http.Request) (openplatform.Principal, error) {
		p, e := api.GetOpenPrincipal(r)
		if e != nil {
			return p, e
		}
		return p, nil
	}
	api.SetDeveloperTransports(handlers.NewOpenPlatform(shared, actor), nil, nil, nil)
	api.SetCompatOperations(shared)
	return NewWithAPI(api), openAPICredentials{UserID: user.ID, AccessKey: created.Key.AccessKey, Secret: created.Secret, ProjectID: selected.ID}, auth, bills
}
