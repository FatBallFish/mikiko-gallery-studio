package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	"github.com/fatballfish/pic-gallery/internal/http/router"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	"github.com/fatballfish/pic-gallery/internal/service/apikey"
	"github.com/fatballfish/pic-gallery/internal/service/assets"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	"github.com/fatballfish/pic-gallery/internal/service/billing"
	"github.com/fatballfish/pic-gallery/internal/service/imagetask"
	"github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/service/project"
	"github.com/fatballfish/pic-gallery/internal/service/uploadaccess"
	"github.com/fatballfish/pic-gallery/internal/service/videorouting"
	"github.com/fatballfish/pic-gallery/internal/service/videotask"
	"github.com/fatballfish/pic-gallery/internal/storage"
	_ "github.com/mattn/go-sqlite3"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type developerBearerTransport struct {
	token string
	next  http.RoundTripper
}

func (b developerBearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.next.RoundTrip(r)
}

// Exercise the runtime composition, including its persisted grant validator, rather
// than constructing an independent OAuth or MCP handler with a fake executor.
func TestConfigureDeveloperServicesHTTPIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := repoent.Open(dialect.SQLite, "file:developer-runtime-integration?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(nil)
	defer server.Close()
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{}
	cfg.Runtime.PublicAPIURL = "http://" + server.Listener.Addr().String()
	cfg.Auth = config.AuthConfig{Issuer: "developer-integration", AccessTokenSecret: base64.RawURLEncoding.EncodeToString(secret), AccessTokenTTL: time.Hour, RefreshTokenTTL: time.Hour}
	cfg.Billing = config.BillingConfig{PointsScale: 5, CNYPerPoint: "1.00000", UserGroupMultipliers: map[string]string{"basic": "1.00000"}}
	auth := authservice.NewServiceWithStore(cfg.Auth, cfg.Billing.UserGroupMultipliers, entstore.NewAuthStore(db))
	first, err := auth.LoginWithOIDC(authservice.OIDCLoginInput{Subject: "developer-first", Email: "first@example.com", EmailVerified: true})
	if err != nil {
		t.Fatal(err)
	}
	second, err := auth.LoginWithOIDC(authservice.OIDCLoginInput{Subject: "developer-second", Email: "second@example.com", EmailVerified: true})
	if err != nil {
		t.Fatal(err)
	}
	keys := apikey.NewService(entstore.NewAPIKeyStore(db))
	bills := billing.NewServiceWithStore(cfg.Billing, entstore.NewBillingStore(db, 5))
	refs := assets.NewServiceWithStoreAndRouter(cfg.GenerationLimits, entstore.NewAssetsStore(db), nil)
	images := imagetask.NewServiceWithProvidersStoreAssetsBillingAndRouter(cfg, nil, entstore.NewImageTaskStore(db), refs, bills, nil)
	media := mediaasset.NewService(entstore.NewMediaStore(db), storage.NewStaticRouter(storage.NewLocalBackend(t.TempDir())), mediaasset.Options{})
	projects := project.NewService(entstore.NewProjectStore(db))
	models := videorouting.NewService(entstore.NewVideoConfigStore(db))
	videos := videotask.NewService(entstore.NewVideoTaskStore(db, entstore.NewBillingStore(db, 5)), nil, projects, media, nil)
	api := handlers.NewAPIWithRuntimeServices(cfg, auth, refs, images, nil, bills, keys)
	if err := configureDeveloperServices(cfg, db, api, auth, keys, bills, images, refs, media, projects, models, videos); err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = router.NewWithAPIAndConfig(api, cfg)
	server.Start()
	httpClient := server.Client()
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	request := func(method, path, token, contentType, body string, status int) (http.Header, map[string]any) {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, method, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != status {
			t.Fatalf("%s %s: status %d, want %d: %s", method, path, resp.StatusCode, status, raw)
		}
		var result map[string]any
		if len(raw) > 0 && json.Valid(raw) {
			if err := json.Unmarshal(raw, &result); err != nil {
				t.Fatal(err)
			}
		}
		return resp.Header, result
	}
	jsonBody := func(v any) string {
		t.Helper()
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	data := func(v map[string]any) map[string]any {
		t.Helper()
		d, ok := v["data"].(map[string]any)
		if !ok {
			t.Fatalf("missing response data: %#v", v)
		}
		return d
	}
	_, metadata := request("GET", "/.well-known/oauth-authorization-server", "", "", "", 200)
	if metadata["issuer"] != server.URL || metadata["token_endpoint"] != server.URL+"/oauth/token" {
		t.Fatalf("noncanonical metadata: %#v", metadata)
	}
	for _, path := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp"} {
		_, resource := request("GET", path, "", "", "", 200)
		if resource["resource"] != server.URL+"/mcp" {
			t.Fatalf("resource metadata: %#v", resource)
		}
	}
	headers, _ := request("POST", "/mcp", "", "application/json", `{}`, 401)
	if !strings.Contains(headers.Get("WWW-Authenticate"), server.URL+"/.well-known/oauth-protected-resource/mcp") {
		t.Fatal("missing MCP discovery challenge")
	}
	_, registered := request("POST", "/oauth/register", "", "application/json", `{"client_name":"Runtime integration","redirect_uris":["http://127.0.0.1:32123/callback"],"token_endpoint_auth_method":"none"}`, 201)
	clientID, ok := registered["client_id"].(string)
	if !ok || clientID == "" {
		t.Fatalf("registration: %#v", registered)
	}
	scopes := []string{"account:read", "models:read", "projects:read", "projects:write", "projects:delete", "groups:read", "groups:write", "groups:delete", "assets:read", "uploads:read", "uploads:write", "uploads:delete"}
	authorize := func(studioToken string) string {
		t.Helper()
		verifier := strings.Repeat("v", 43)
		hash := sha256.Sum256([]byte(verifier))
		q := url.Values{"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {"http://127.0.0.1:32123/callback"}, "resource": {server.URL + "/mcp"}, "scope": {strings.Join(scopes, " ")}, "state": {"runtime-state"}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(hash[:])}}
		headers, _ := request("GET", "/oauth/authorize?"+q.Encode(), "", "", "", 303)
		location, err := url.Parse(headers.Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		id := location.Query().Get("oauth_request")
		if id == "" {
			t.Fatalf("missing authorization request: %s", location)
		}
		path := "/api/v1/oauth/consent?request_id=" + url.QueryEscape(id)
		request("GET", path, "", "", "", 401)
		_, consent := request("GET", path, studioToken, "", "", 200)
		csrf, ok := data(consent)["csrf_token"].(string)
		if !ok || csrf == "" {
			t.Fatal("missing consent CSRF")
		}
		request("POST", "/api/v1/oauth/consent", studioToken, "application/json", jsonBody(map[string]any{"request_id": id, "csrf_token": "invalid", "allow": true}), 400)
		_, decision := request("POST", "/api/v1/oauth/consent", studioToken, "application/json", jsonBody(map[string]any{"request_id": id, "csrf_token": csrf, "allow": true}), 200)
		redirect, err := url.Parse(data(decision)["redirect_uri"].(string))
		if err != nil {
			t.Fatal(err)
		}
		if redirect.Query().Get("state") != "runtime-state" || redirect.Query().Get("code") == "" {
			t.Fatalf("authorization redirect: %s", redirect)
		}
		form := url.Values{"grant_type": {"authorization_code"}, "code": {redirect.Query().Get("code")}, "client_id": {clientID}, "redirect_uri": {"http://127.0.0.1:32123/callback"}, "resource": {server.URL + "/mcp"}, "code_verifier": {verifier}}
		_, tokens := request("POST", "/oauth/token", "", "application/x-www-form-urlencoded", form.Encode(), 200)
		request("POST", "/oauth/token", "", "application/x-www-form-urlencoded", form.Encode(), 400)
		token, ok := tokens["access_token"].(string)
		if !ok || token == "" {
			t.Fatalf("token: %#v", tokens)
		}
		return token
	}
	firstToken := authorize(first.Session.AccessToken)
	secondToken := authorize(second.Session.AccessToken)
	connect := func(token string) *mcp.ClientSession {
		t.Helper()
		client := mcp.NewClient(&mcp.Implementation{Name: "runtime-integration", Version: "1"}, nil)
		session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.URL + "/mcp", HTTPClient: &http.Client{Transport: developerBearerTransport{token: token, next: httpClient.Transport}}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = session.Close() })
		return session
	}
	firstSDK, secondSDK := connect(firstToken), connect(secondToken)
	call := func(session *mcp.ClientSession, name string, args map[string]any, wantError bool) map[string]any {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if result.IsError != wantError {
			t.Fatalf("%s error=%v want %v: %#v", name, result.IsError, wantError, result)
		}
		if wantError {
			return nil
		}
		raw, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("%s structured content %s: %v", name, raw, err)
		}
		return data(out)
	}
	empty := map[string]any{}
	user1 := call(firstSDK, "get_current_user", empty, false)
	user2 := call(secondSDK, "get_current_user", empty, false)
	if user1["id"] == nil || user1["id"] == user2["id"] {
		t.Fatalf("user isolation: %#v %#v", user1, user2)
	}
	for _, name := range []string{"get_balance", "list_image_models", "list_video_models"} {
		call(firstSDK, name, empty, false)
	}
	p := call(firstSDK, "create_project", map[string]any{"name": "Integration project", "idempotency_key": "runtime-project"}, false)
	projectID, ok := p["id"].(string)
	if !ok || projectID == "" {
		t.Fatalf("project: %#v", p)
	}
	group := call(firstSDK, "create_group", map[string]any{"project_id": projectID, "name": "Integration group"}, false)
	groupID, ok := group["id"].(string)
	if !ok || groupID == "" {
		t.Fatalf("group: %#v", group)
	}
	call(secondSDK, "get_project", map[string]any{"id": projectID}, true)
	call(secondSDK, "get_group", map[string]any{"project_id": projectID, "id": groupID}, true)
	call(firstSDK, "get_group", map[string]any{"project_id": projectID, "id": groupID}, false)
	updated := call(firstSDK, "update_group", map[string]any{"project_id": projectID, "id": groupID, "name": "Renamed group", "expected_version": group["version"]}, false)
	if updated["name"] != "Renamed group" {
		t.Fatalf("rename: %#v", updated)
	}
	key, err := keys.CreateKey(ctx, apikey.CreateRequest{UserID: int64(user1["id"].(float64)), Name: "runtime", Scopes: scopes})
	if err != nil {
		t.Fatal(err)
	}
	_, me := request("GET", "/api/open/v1/me", key.Secret, "", "", 200)
	if data(me)["id"] != user1["id"] {
		t.Fatalf("REST/MCP identity mismatch: %#v %#v", me, user1)
	}
	_, groups := request("GET", "/api/open/v1/groups?project_id="+projectID, key.Secret, "", "", 200)
	if !strings.Contains(jsonBody(data(groups)), "Renamed group") {
		t.Fatalf("REST cannot see MCP group: %#v", groups)
	}
	request("GET", "/api/open/v1/assets?project_id="+projectID, key.Secret, "", "", 200)
	limited, err := keys.CreateKey(ctx, apikey.CreateRequest{UserID: int64(user1["id"].(float64)), Name: "limited", Scopes: []string{"account:read"}})
	if err != nil {
		t.Fatal(err)
	}
	request("GET", "/api/open/v1/assets", limited.Secret, "", "", 403)
	request("GET", "/api/open/image/v1/tasks", key.Secret, "", "", 404)
	call(firstSDK, "delete_group", map[string]any{"project_id": projectID, "id": groupID, "expected_version": updated["version"]}, false)
	call(firstSDK, "get_group", map[string]any{"project_id": projectID, "id": groupID}, true)
	// Both transports must return a fetchable local target without the original credential.
	payload := "local multipart regression bytes"
	digest := sha256.Sum256([]byte(payload))
	checksum := base64.StdEncoding.EncodeToString(digest[:])
	putTarget := func(target map[string]any, want int) map[string]any {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, "PUT", target["url"].(string), strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		for name, value := range target["headers"].(map[string]any) {
			req.Header.Set(name, value.(string))
		}
		if req.Header.Get("Authorization") != "" || req.Header.Get(uploadaccess.Header) == "" {
			t.Fatal("target leaked credentials or lacks capability")
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != want {
			t.Fatalf("capability PUT status=%d want=%d body=%s", resp.StatusCode, want, raw)
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		if want != 200 {
			return nil
		}
		return data(out)
	}
	var revokedTarget map[string]any
	for _, transport := range []string{"REST", "MCP"} {
		args := map[string]any{"project_id": projectID, "filename": "sample.png", "media_type": "image", "mime_type": "image/png", "size_bytes": len(payload), "checksum": checksum, "idempotency_key": "upload-" + transport}
		var upload map[string]any
		if transport == "MCP" {
			upload = call(firstSDK, "create_upload", args, false)
		} else {
			_, out := request("POST", "/api/open/v1/uploads", key.Secret, "application/json", jsonBody(args), 201)
			upload = data(out)
		}
		id := upload["id"].(string)
		call(secondSDK, "get_upload_part_url", map[string]any{"id": id, "part_number": 1, "checksum": checksum}, true)
		var target map[string]any
		if transport == "MCP" {
			target = call(firstSDK, "get_upload_part_url", map[string]any{"id": id, "part_number": 1, "checksum": checksum}, false)
		} else {
			_, out := request("POST", "/api/open/v1/uploads/"+id+"/parts/1/url", key.Secret, "application/json", jsonBody(map[string]any{"checksum": checksum}), 200)
			target = data(out)
		}
		part := putTarget(target, 200)
		if transport == "REST" {
			// Failed completion stays locked for completion retries; GET cannot unlock it.
			badPart := map[string]any{}
			for k, v := range part {
				badPart[k] = v
			}
			badPart["etag"] = "wrong-etag"
			request("POST", "/api/open/v1/uploads/"+id+"/complete", key.Secret, "application/json", jsonBody(map[string]any{"parts": []any{badPart}}), 500)
			_, state := request("GET", "/api/open/v1/uploads/"+id, key.Secret, "", "", 200)
			if data(state)["status"] != "completing" {
				t.Fatalf("GET restored writable state: %#v", state)
			}
			putTarget(target, 403)
			request("POST", "/api/open/v1/uploads/"+id+"/parts/1/url", key.Secret, "application/json", jsonBody(map[string]any{"checksum": checksum}), 409)
			req, err := http.NewRequestWithContext(ctx, "PUT", server.URL+"/api/open/v1/uploads/"+id+"/parts/1", strings.NewReader(payload))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+key.Secret)
			req.Header.Set("X-Checksum-SHA256", checksum)
			resp, err := httpClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != 409 {
				t.Fatalf("ordinary PUT restored completing upload: %d", resp.StatusCode)
			}
		}
		complete := map[string]any{"id": id, "parts": []any{part}}
		if transport == "MCP" {
			call(firstSDK, "complete_upload", complete, false)
		} else {
			request("POST", "/api/open/v1/uploads/"+id+"/complete", key.Secret, "application/json", jsonBody(map[string]any{"parts": []any{part}}), 200)
		}
		putTarget(target, 403)
	}
	// Minted capabilities must follow current key generation and scopes too.
	_, pendingREST := request("POST", "/api/open/v1/uploads", key.Secret, "application/json", jsonBody(map[string]any{"project_id": projectID, "filename": "rotate.png", "media_type": "image", "mime_type": "image/png", "size_bytes": len(payload), "idempotency_key": "upload-key-state"}), 201)
	uploadPath := "/api/open/v1/uploads/" + data(pendingREST)["id"].(string) + "/parts/1/url"
	_, beforeRotate := request("POST", uploadPath, key.Secret, "application/json", jsonBody(map[string]any{"checksum": checksum}), 200)
	rotated, err := keys.ResetSecret(ctx, key.Key.UserID, key.Key.ID)
	if err != nil {
		t.Fatal(err)
	}
	putTarget(data(beforeRotate), 401)
	key.Secret = rotated.Secret
	_, beforeScope := request("POST", uploadPath, key.Secret, "application/json", jsonBody(map[string]any{"checksum": checksum}), 200)
	readScopes := []string{"account:read"}
	if _, err := keys.Update(ctx, key.Key.UserID, key.Key.ID, apikey.UpdateRequest{Scopes: &readScopes}); err != nil {
		t.Fatal(err)
	}
	putTarget(data(beforeScope), 403)
	if _, err := keys.Update(ctx, key.Key.UserID, key.Key.ID, apikey.UpdateRequest{Scopes: &scopes}); err != nil {
		t.Fatal(err)
	}
	_, beforeDisable := request("POST", uploadPath, key.Secret, "application/json", jsonBody(map[string]any{"checksum": checksum}), 200)
	if err := keys.UpdateStatus(ctx, key.Key.UserID, key.Key.ID, "disabled"); err != nil {
		t.Fatal(err)
	}
	putTarget(data(beforeDisable), 401)
	if err := keys.UpdateStatus(ctx, key.Key.UserID, key.Key.ID, "active"); err != nil {
		t.Fatal(err)
	}
	pending := call(firstSDK, "create_upload", map[string]any{"project_id": projectID, "filename": "pending.png", "media_type": "image", "mime_type": "image/png", "size_bytes": len(payload), "idempotency_key": "upload-revoke"}, false)
	revokedTarget = call(firstSDK, "get_upload_part_url", map[string]any{"id": pending["id"], "part_number": 1, "checksum": checksum}, false)
	request("POST", "/oauth/revoke", "", "application/x-www-form-urlencoded", url.Values{"token": {firstToken}, "client_id": {clientID}}.Encode(), 200)
	putTarget(revokedTarget, 403)
	if _, err := firstSDK.ListTools(ctx, nil); err == nil {
		t.Fatal("revoked OAuth grant still authorizes SDK")
	}
	call(secondSDK, "get_current_user", empty, false)
	request("GET", "/api/open/v1/me", key.Secret, "", "", 200)
}
