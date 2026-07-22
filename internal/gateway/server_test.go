package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
)

func TestNativeGatewayServesFrontendRoutesAndSPAFallbacks(t *testing.T) {
	assets := gatewayAssetsForTest(t)
	apiURL, _ := url.Parse("http://127.0.0.1:1")
	handler, err := NewHandler(Config{
		APIURL: apiURL, UserDir: assets.user, AdminDir: assets.admin, DocsDir: assets.docs,
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		path        string
		status      int
		body        string
		location    string
		cacheHeader string
	}{
		{path: "/", status: http.StatusOK, body: "user-index", cacheHeader: "no-store"},
		{path: "/workspace/tasks/123", status: http.StatusOK, body: "user-index", cacheHeader: "no-store"},
		{path: "/assets/user-deadbeef.js", status: http.StatusOK, body: "user-asset", cacheHeader: "immutable"},
		{path: "/admin?tab=security", status: http.StatusPermanentRedirect, location: "/admin/?tab=security"},
		{path: "/admin/", status: http.StatusOK, body: "admin-index", cacheHeader: "no-store"},
		{path: "/admin/providers/12", status: http.StatusOK, body: "admin-index", cacheHeader: "no-store"},
		{path: "/developer-docs?section=api", status: http.StatusPermanentRedirect, location: "/developer-docs/?section=api"},
		{path: "/developer-docs/reference/images", status: http.StatusOK, body: "docs-index", cacheHeader: "no-store"},
	}
	for _, testCase := range tests {
		t.Run(testCase.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, testCase.path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != testCase.status || (testCase.body != "" && !strings.Contains(response.Body.String(), testCase.body)) {
				t.Fatalf("response = %d %q", response.Code, response.Body.String())
			}
			if testCase.location != "" && response.Header().Get("Location") != testCase.location {
				t.Fatalf("Location = %q, want %q", response.Header().Get("Location"), testCase.location)
			}
			if testCase.cacheHeader != "" && !strings.Contains(response.Header().Get("Cache-Control"), testCase.cacheHeader) {
				t.Fatalf("Cache-Control = %q, want %q", response.Header().Get("Cache-Control"), testCase.cacheHeader)
			}
		})
	}
}

func TestNativeGatewayServesPublicRuntimeConfigWithoutBackendSecrets(t *testing.T) {
	assets := gatewayAssetsForTest(t)
	apiURL, _ := url.Parse("https://api.internal.example/base")
	handler, err := NewHandler(Config{
		APIURL: apiURL, FrontendAPIBaseURL: "", DocsURL: "/developer-docs/",
		UserDir: assets.user, AdminDir: assets.admin, DocsDir: assets.docs,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/env.js", "/admin/env.js"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		body := response.Body.String()
		if response.Code != http.StatusOK || !strings.Contains(body, `"apiBaseUrl":""`) || !strings.Contains(body, `"VITE_DOCS_URL":"/developer-docs/"`) {
			t.Fatalf("%s runtime config = %d %q", path, response.Code, body)
		}
		for _, secret := range []string{"DATABASE_URL", "SETUP_TOKEN", "AUTH_ACCESS_TOKEN_SECRET", "api.internal.example"} {
			if strings.Contains(body, secret) {
				t.Fatalf("public runtime config leaked %q: %s", secret, body)
			}
		}
		if response.Header().Get("Content-Type") != "application/javascript; charset=utf-8" || !strings.Contains(response.Header().Get("Cache-Control"), "no-store") {
			t.Fatalf("runtime config headers = %#v", response.Header())
		}
	}
}

func TestNativeGatewayProxiesAPISetupAndHealthRoutes(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Upstream-Path", request.URL.RequestURI())
		writer.Header().Set("X-Upstream-Host", request.Host)
		writer.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(writer, "proxied")
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL + "/base")
	assets := gatewayAssetsForTest(t)
	handler, err := NewHandler(Config{APIURL: target, UserDir: assets.user, AdminDir: assets.admin, DocsDir: assets.docs})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/healthz", "/readyz", "/setup", "/api/system/v1/bootstrap-status?source=gateway",
		"/api", "/metrics", "/docs", "/docs/openapi.yaml", "/v1", "/v1/images/generations", "/files", "/files/result.png",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusCreated || response.Body.String() != "proxied" {
			t.Fatalf("%s response = %d %q", path, response.Code, response.Body.String())
		}
		if got := response.Header().Get("X-Upstream-Path"); got != "/base"+path {
			t.Fatalf("%s upstream path = %q, want %q", path, got, "/base"+path)
		}
		if response.Header().Get("X-Upstream-Host") != strings.TrimPrefix(upstream.URL, "http://") {
			t.Fatalf("%s did not use upstream Host", path)
		}
		if (path == "/setup" || strings.HasPrefix(path, "/api/system/v1/bootstrap-status")) && !strings.Contains(response.Header().Get("Cache-Control"), "no-store") {
			t.Fatalf("%s Cache-Control = %q, want no-store", path, response.Header().Get("Cache-Control"))
		}
	}
}

func TestNativeGatewayRebuildsForwardedHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Seen-Forwarded", request.Header.Get("Forwarded"))
		writer.Header().Set("X-Seen-For", request.Header.Get("X-Forwarded-For"))
		writer.Header().Set("X-Seen-Host", request.Header.Get("X-Forwarded-Host"))
		writer.Header().Set("X-Seen-Proto", request.Header.Get("X-Forwarded-Proto"))
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	assets := gatewayAssetsForTest(t)
	handler, err := NewHandler(Config{APIURL: target, UserDir: assets.user, AdminDir: assets.admin, DocsDir: assets.docs})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.RemoteAddr = "192.0.2.10:4321"
	request.Host = "gateway.example.test"
	request.Header.Set("Forwarded", "for=attacker;host=evil.example")
	request.Header.Set("X-Forwarded-For", "198.51.100.99")
	request.Header.Set("X-Forwarded-Host", "evil.example")
	request.Header.Set("X-Forwarded-Proto", "https")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("proxy status = %d", response.Code)
	}
	if response.Header().Get("X-Seen-Forwarded") != "" || response.Header().Get("X-Seen-For") != "192.0.2.10" || response.Header().Get("X-Seen-Host") != "gateway.example.test" || response.Header().Get("X-Seen-Proto") != "http" {
		t.Fatalf("forwarded headers = %#v", response.Header())
	}
}

func TestNativeGatewayRuntimeConfigEscapesScriptContent(t *testing.T) {
	content, err := renderRuntimeConfig("", "</script>\nwindow.injected = 'yes'\\")
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(content)
	if strings.Contains(rendered, "</script>") || !strings.Contains(rendered, `\u003c/script\u003e\nwindow.injected = 'yes'\\`) {
		t.Fatalf("runtime JavaScript was not safely JSON encoded: %s", rendered)
	}
}

func TestNativeGatewayRejectsTraversalAndUnsupportedMethods(t *testing.T) {
	assets := gatewayAssetsForTest(t)
	outside := filepath.Join(filepath.Dir(assets.user), "secret.txt")
	if err := os.WriteFile(outside, []byte("outside-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(assets.user, "escape.txt")); err != nil {
		t.Fatal(err)
	}
	apiURL, _ := url.Parse("http://127.0.0.1:1")
	handler, err := NewHandler(Config{APIURL: apiURL, UserDir: assets.user, AdminDir: assets.admin, DocsDir: assets.docs})
	if err != nil {
		t.Fatal(err)
	}
	for _, rawURL := range []string{"/../secret.txt", "/%2e%2e/secret.txt", "/..%2fsecret.txt", "/%252e%252e/secret.txt", "/escape.txt", "/admin/%2e%2e/secret.txt", "/.env", "/config/runtime.env", "/data/private.txt", "/logs/api.log"} {
		request := httptest.NewRequest(http.MethodGet, rawURL, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code < 400 || strings.Contains(response.Body.String(), "outside-secret") {
			t.Fatalf("traversal %q returned %d %q", rawURL, response.Code, response.Body.String())
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/assets/user-deadbeef.js", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("static POST status = %d", response.Code)
	}
}

func TestNativeGatewayDoesNotFallbackMissingAssetsOrListDirectories(t *testing.T) {
	assets := gatewayAssetsForTest(t)
	apiURL, _ := url.Parse("http://127.0.0.1:1")
	handler, err := NewHandler(Config{APIURL: apiURL, UserDir: assets.user, AdminDir: assets.admin, DocsDir: assets.docs})
	if err != nil {
		t.Fatal(err)
	}
	for _, requestPath := range []string{"/assets/missing-deadbeef.js", "/assets/", "/admin/assets/missing.css", "/developer-docs/openapi/missing.yaml"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, requestPath, nil))
		if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "index") {
			t.Fatalf("missing asset %q returned %d %q", requestPath, response.Code, response.Body.String())
		}
	}
}

func TestNativeGatewayValidatesConfiguration(t *testing.T) {
	assets := gatewayAssetsForTest(t)
	validURL, _ := url.Parse("http://127.0.0.1:8080")
	tests := []Config{
		{UserDir: assets.user, AdminDir: assets.admin, DocsDir: assets.docs},
		{APIURL: validURL, UserDir: "", AdminDir: assets.admin, DocsDir: assets.docs},
		{APIURL: &url.URL{Scheme: "file", Path: "/tmp/api"}, UserDir: assets.user, AdminDir: assets.admin, DocsDir: assets.docs},
		{APIURL: &url.URL{Scheme: "https", Host: "api.example.test", RawQuery: "target=other"}, UserDir: assets.user, AdminDir: assets.admin, DocsDir: assets.docs},
		{APIURL: &url.URL{Scheme: "https", Host: "api.example.test", Fragment: "secret"}, UserDir: assets.user, AdminDir: assets.admin, DocsDir: assets.docs},
	}
	for _, testCase := range tests {
		if _, err := NewHandler(testCase); err == nil {
			t.Fatalf("NewHandler accepted invalid config: %#v", testCase)
		}
	}
}

func TestNativeGatewayConfigFromBootstrapUsesLocalOrPublicAPI(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name      string
		bootstrap config.BootstrapConfig
		apiURL    string
	}{
		{
			name: "local API",
			bootstrap: config.BootstrapConfig{
				Deployment:        config.DeploymentContext{Role: config.DeploymentRoleSingle},
				DeploymentModules: []string{"api", "gateway"}, Values: map[string]string{"API_PORT": "18080", "GATEWAY_PORT": "18000"},
			},
			apiURL: "http://127.0.0.1:18080",
		},
		{
			name: "external API for web role",
			bootstrap: config.BootstrapConfig{
				Deployment:        config.DeploymentContext{Role: config.DeploymentRoleWeb},
				DeploymentModules: []string{"user-web", "admin-web", "docs-web", "gateway"},
				Values:            map[string]string{"PUBLIC_API_URL": "https://api.example.test:8443/base", "GATEWAY_PORT": "18000"},
			},
			apiURL: "https://api.example.test:8443/base",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			gatewayConfig, err := ConfigFromBootstrap(testCase.bootstrap, root)
			if err != nil {
				t.Fatal(err)
			}
			if gatewayConfig.Address != ":18000" || gatewayConfig.APIURL.String() != testCase.apiURL || gatewayConfig.FrontendAPIBaseURL != "" {
				t.Fatalf("Gateway config = %#v", gatewayConfig)
			}
			if gatewayConfig.UserDir != filepath.Join(root, "web", "user") || gatewayConfig.AdminDir != filepath.Join(root, "web", "admin") || gatewayConfig.DocsDir != filepath.Join(root, "web", "docs") {
				t.Fatalf("asset paths = %q %q %q", gatewayConfig.UserDir, gatewayConfig.AdminDir, gatewayConfig.DocsDir)
			}
		})
	}
}

func TestNativeGatewayConfigFromBootstrapRejectsMissingModuleAndPorts(t *testing.T) {
	base := config.BootstrapConfig{
		Deployment:        config.DeploymentContext{Role: config.DeploymentRoleSingle},
		DeploymentModules: []string{"api", "gateway"}, Values: map[string]string{"API_PORT": "8080", "GATEWAY_PORT": "8000"},
	}
	tests := []config.BootstrapConfig{base, base, base}
	tests[0].DeploymentModules = []string{"api"}
	tests[1].Values = map[string]string{"API_PORT": "8080", "GATEWAY_PORT": "70000"}
	tests[2].Deployment.Role = config.DeploymentRoleWeb
	tests[2].Values = map[string]string{"GATEWAY_PORT": "8000", "PUBLIC_API_URL": "file:///tmp/api"}
	for _, bootstrap := range tests {
		if _, err := ConfigFromBootstrap(bootstrap, t.TempDir()); err == nil {
			t.Fatalf("ConfigFromBootstrap accepted invalid bootstrap: %#v", bootstrap)
		}
	}
}

func TestNativeGatewayConfigFromBootstrapFindsSourceTreeDistAssets(t *testing.T) {
	root := t.TempDir()
	for _, frontend := range []string{"user", "admin", "docs"} {
		directory := filepath.Join(root, "web", frontend, "dist")
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte(frontend), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bootstrap := config.BootstrapConfig{
		Deployment: config.DeploymentContext{Role: config.DeploymentRoleSingle}, DeploymentModules: []string{"api", "gateway"},
		Values: map[string]string{"API_PORT": "8080", "GATEWAY_PORT": "8000"},
	}
	gatewayConfig, err := ConfigFromBootstrap(bootstrap, root)
	if err != nil {
		t.Fatal(err)
	}
	if gatewayConfig.UserDir != filepath.Join(root, "web", "user", "dist") || gatewayConfig.AdminDir != filepath.Join(root, "web", "admin", "dist") || gatewayConfig.DocsDir != filepath.Join(root, "web", "docs", "dist") {
		t.Fatalf("source-tree asset paths = %q %q %q", gatewayConfig.UserDir, gatewayConfig.AdminDir, gatewayConfig.DocsDir)
	}
}

type gatewayTestAssets struct {
	user  string
	admin string
	docs  string
}

func gatewayAssetsForTest(t *testing.T) gatewayTestAssets {
	t.Helper()
	root := t.TempDir()
	assets := gatewayTestAssets{
		user: filepath.Join(root, "user"), admin: filepath.Join(root, "admin"), docs: filepath.Join(root, "docs"),
	}
	for directory, index := range map[string]string{assets.user: "user-index", assets.admin: "admin-index", assets.docs: "docs-index"} {
		if err := os.MkdirAll(filepath.Join(directory, "assets"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte(index), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(assets.user, "assets", "user-deadbeef.js"), []byte("user-asset"), 0o644); err != nil {
		t.Fatal(err)
	}
	return assets
}
