package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type executeFunc func(context.Context, openplatform.Principal, string, json.RawMessage) (any, error)

func (f executeFunc) Execute(ctx context.Context, p openplatform.Principal, op string, input json.RawMessage) (any, error) {
	return f(ctx, p, op, input)
}

type bearerTransport struct {
	token string
	next  http.RoundTripper
}

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.next.RoundTrip(r)
}

func TestOfficialSDKAuthenticatedTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var mu sync.Mutex
	validations := 0
	calls := 0
	revoked := false
	handler, err := NewHandler(Config{ResourceURL: "https://studio.example/mcp", Validator: ValidateAccessFunc(func(_ context.Context, token string) (openplatform.Principal, error) {
		mu.Lock()
		defer mu.Unlock()
		validations++
		if revoked {
			return openplatform.Principal{}, errors.New("revoked")
		}
		id := int64(41)
		if token == "second" {
			id = 42
		}
		return openplatform.Principal{UserID: id, ClientID: "client", GrantID: token, Scopes: []string{"account:read", "assets:read"}}, nil
	}), Service: executeFunc(func(_ context.Context, p openplatform.Principal, op string, input json.RawMessage) (any, error) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if op == "get_asset" {
			var args struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(input, &args)
			if args.ID != "owned" {
				return nil, errors.New("asset not found")
			}
		}
		return map[string]any{"id": p.UserID, "email": "fixture@example.com", "nickname": "fixture", "status": "active", "group_code": "basic"}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()
	connect := func(token string) *mcp.ClientSession {
		t.Helper()
		client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
		cs, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: srv.URL, HTTPClient: &http.Client{Transport: bearerTransport{token: token, next: http.DefaultTransport}}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cs.Close() })
		return cs
	}
	first := connect("first")
	if got := first.InitializeResult().ProtocolVersion; got != "2026-07-28" {
		t.Fatalf("negotiated %q", got)
	}
	tools, err := first.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != len(Catalogue()) {
		t.Fatalf("tool count %d", len(tools.Tools))
	}
	for _, tool := range tools.Tools {
		if tool.InputSchema == nil || tool.OutputSchema == nil || tool.Annotations == nil {
			t.Fatalf("missing contract for %s", tool.Name)
		}
	}
	call := func(cs *mcp.ClientSession, name string, args any) *mcp.CallToolResult {
		t.Helper()
		result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	result := call(first, "get_current_user", map[string]any{})
	if result.IsError || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, `"id":41`) {
		t.Fatalf("unexpected result %#v", result)
	}
	second := connect("second")
	result = call(second, "get_current_user", map[string]any{})
	if !strings.Contains(result.Content[0].(*mcp.TextContent).Text, `"id":42`) {
		t.Fatalf("principal leaked: %#v", result)
	}
	mu.Lock()
	before := calls
	mu.Unlock()
	result = call(first, "create_image_task", map[string]any{"project_id": "p", "route_model_code": "m", "idempotency_key": "k"})
	if !result.IsError || result.Meta["mcp/www_authenticate"] == nil {
		t.Fatalf("missing scope challenge %#v", result)
	}
	result = call(first, "get_asset", map[string]any{"id": "owned", "user_id": 42})
	if !result.IsError {
		t.Fatal("accepted principal injection")
	}
	mu.Lock()
	after := calls
	mu.Unlock()
	if before != after {
		t.Fatal("invalid calls reached application")
	}
	result = call(first, "get_asset", map[string]any{"id": "another-user"})
	if !result.IsError {
		t.Fatal("application ownership failure not propagated")
	}
	mu.Lock()
	if validations < 5 {
		t.Errorf("not revalidated every request: %d", validations)
	}
	revoked = true
	mu.Unlock()
	if _, err := first.ListTools(ctx, nil); err == nil {
		t.Fatal("revoked grant still authorized")
	}
}

func TestHTTPAuthenticationOriginAndVersion(t *testing.T) {
	h, err := NewHandler(Config{ResourceURL: "https://studio.example/mcp", Service: executeFunc(func(context.Context, openplatform.Principal, string, json.RawMessage) (any, error) { return nil, nil }), Validator: ValidateAccessFunc(func(context.Context, string) (openplatform.Principal, error) {
		return openplatform.Principal{UserID: 1, ClientID: "client", GrantID: "grant", Scopes: []string{"account:read"}}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, method, token, origin, version string
		status                               int
	}{
		{name: "missing", method: "POST", status: 401},
		{name: "get requires auth", method: "GET", status: 401},
		{name: "delete requires auth", method: "DELETE", status: 401},
		{name: "evil origin", method: "POST", token: "valid", origin: "https://evil.example", status: 403},
		{name: "null origin", method: "POST", token: "valid", origin: "null", status: 403},
		{name: "invalid version", method: "POST", token: "valid", version: "2025-01-01", status: 400},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(test.method, "https://studio.example/mcp", strings.NewReader(`{}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Accept", "application/json, text/event-stream")
			if test.token != "" {
				r.Header.Set("Authorization", "Bearer "+test.token)
			}
			if test.origin != "" {
				r.Header.Set("Origin", test.origin)
			}
			if test.version != "" {
				r.Header.Set("MCP-Protocol-Version", test.version)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != test.status {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if test.status == 401 && !strings.Contains(w.Header().Get("WWW-Authenticate"), `resource_metadata="https://studio.example/.well-known/oauth-protected-resource/mcp"`) {
				t.Fatal("missing OAuth resource challenge")
			}
		})
	}
}

func TestSchemasRejectUntrustedInput(t *testing.T) {
	for _, spec := range Catalogue() {
		if _, err := resolveSchema(spec.InputSchema); err != nil {
			t.Fatalf("%s input: %v", spec.Name, err)
		}
		if _, err := resolveSchema(spec.OutputSchema); err != nil {
			t.Fatalf("%s output: %v", spec.Name, err)
		}
		for _, input := range []string{`{"user_id":9}`, `{"url":"https://attacker.example"}`, `{"base64":"blob"}`, `null`, `[]`} {
			if err := validateInput(spec, json.RawMessage(input)); err == nil {
				t.Fatalf("%s accepted %s", spec.Name, input)
			}
		}
	}
}

// Keep errors from unauthorized SDK transports bounded and bodies closed.
func TestInvalidTokenChallenge(t *testing.T) {
	h, err := NewHandler(Config{ResourceURL: "http://127.0.0.1:1234/mcp", Service: executeFunc(func(context.Context, openplatform.Principal, string, json.RawMessage) (any, error) { return nil, nil }), Validator: ValidateAccessFunc(func(context.Context, string) (openplatform.Principal, error) {
		return openplatform.Principal{}, errors.New("private token failure")
	})})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	r, _ := http.NewRequest("GET", srv.URL, nil)
	r.Header.Set("Authorization", "Bearer bad")
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 401 || !strings.Contains(resp.Header.Get("WWW-Authenticate"), `error="invalid_token"`) || strings.Contains(string(body), "private token") {
		t.Fatalf("invalid challenge %d %s", resp.StatusCode, body)
	}
}
