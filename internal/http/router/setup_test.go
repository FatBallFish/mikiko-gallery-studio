package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

type setupAuthStub struct{}

func (setupAuthStub) Exchange(string, string) (string, error) { return "session", nil }
func (setupAuthStub) VerifySession(string) error              { return nil }

type setupProbeStub struct{}

func (setupProbeStub) ProbePostgres(context.Context, setup.PostgresProbeRequest) setup.ProbeResult {
	return setup.ProbeResult{Success: true, Code: setup.ProbeCodeOK}
}
func (setupProbeStub) ProbeRedis(context.Context, setup.RedisProbeRequest) setup.ProbeResult {
	return setup.ProbeResult{Success: true, Code: setup.ProbeCodeOK}
}
func (setupProbeStub) ProbeStorage(context.Context, setup.StorageProbeRequest) setup.ProbeResult {
	return setup.ProbeResult{Success: true, Code: setup.ProbeCodeOK}
}

type setupApplicationStub struct{}

func (setupApplicationStub) Apply(context.Context, setup.ApplyRequest) (setup.OperationView, error) {
	return setup.OperationView{}, nil
}
func (setupApplicationStub) Progress(context.Context, string) (setup.OperationView, error) {
	return setup.OperationView{}, nil
}

func TestSetupRouterExposesOnlyBootstrapAndSetupSurface(t *testing.T) {
	setupAPI := newSetupAPIForRouterTest(t, handlers.BootstrapStatus{
		Phase:             handlers.BootstrapPhaseSetupRequired,
		SetupURL:          "/setup",
		RetryAfterSeconds: 2,
	})
	handler := NewSetup(setupAPI, nil)

	assertRouteStatus(t, handler, http.MethodGet, "/healthz", http.StatusOK)
	assertRouteStatus(t, handler, http.MethodGet, "/readyz", http.StatusServiceUnavailable)
	assertRouteStatus(t, handler, http.MethodGet, "/api/system/v1/bootstrap-status", http.StatusOK)
	assertRouteStatus(t, handler, http.MethodGet, "/setup", http.StatusOK)
	assertRouteStatus(t, handler, http.MethodPost, "/api/setup/v1/session", http.StatusNoContent)
	for _, target := range []string{
		"/api/setup/v1/probes/database",
		"/api/setup/v1/probes/redis",
		"/api/setup/v1/probes/storage",
		"/api/setup/v1/apply",
	} {
		assertRouteStatus(t, handler, http.MethodPost, target, http.StatusUnauthorized)
	}
	assertRouteStatus(t, handler, http.MethodGet, "/api/setup/v1/progress/019d0000-0000-7000-8000-000000000001", http.StatusUnauthorized)

	for _, path := range []string{
		"/metrics",
		"/debug/pprof/",
		"/v1/models",
		"/api/agent/auth/v1/login/password",
		"/api/open/image/v1/gallery/images",
		"/api/ops/admin/v1/auth/login",
		"/docs/openapi.yaml",
	} {
		assertRouteStatus(t, handler, http.MethodGet, path, http.StatusNotFound)
	}
}

func TestNormalRouterClosesSetupSurfaceAndKeepsOperationalRoutes(t *testing.T) {
	handler := NewNormal(nil, config.Config{})

	assertRouteStatus(t, handler, http.MethodGet, "/healthz", http.StatusOK)
	assertRouteStatus(t, handler, http.MethodGet, "/readyz", http.StatusOK)
	assertRouteStatus(t, handler, http.MethodGet, "/api/system/v1/bootstrap-status", http.StatusOK)
	assertRouteStatus(t, handler, http.MethodGet, "/metrics", http.StatusOK)
	assertRouteStatus(t, handler, http.MethodGet, "/setup", http.StatusNotFound)
	assertRouteStatus(t, handler, http.MethodGet, "/setup/assets/app.js", http.StatusNotFound)
	assertRouteStatus(t, handler, http.MethodPost, "/api/setup/v1/session", http.StatusNotFound)
	assertRouteStatus(t, handler, http.MethodPost, "/api/setup/v1/apply", http.StatusNotFound)
	assertRouteStatus(t, handler, http.MethodGet, "/legacy-non-api-root", http.StatusOK)
}

func TestBrokenRouterExposesDiagnosticsOnly(t *testing.T) {
	handler := NewBroken(handlers.NewSystemAPI(handlers.BootstrapStatus{
		Phase:             handlers.BootstrapPhaseBroken,
		RetryAfterSeconds: 5,
	}))

	assertRouteStatus(t, handler, http.MethodGet, "/healthz", http.StatusOK)
	assertRouteStatus(t, handler, http.MethodGet, "/readyz", http.StatusServiceUnavailable)
	assertRouteStatus(t, handler, http.MethodGet, "/api/system/v1/bootstrap-status", http.StatusOK)
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/setup"},
		{http.MethodPost, "/api/setup/v1/session"},
		{http.MethodPost, "/api/setup/v1/probes/database"},
		{http.MethodPost, "/api/setup/v1/apply"},
		{http.MethodPost, "/api/agent/auth/v1/login/password"},
		{http.MethodGet, "/metrics"},
		{http.MethodGet, "/debug/pprof/"},
	} {
		assertRouteStatus(t, handler, route.method, route.path, http.StatusNotFound)
	}
}

func TestStartupModePreflightMethodMatrix(t *testing.T) {
	setupHandler := NewSetup(newSetupAPIForRouterTest(t, handlers.BootstrapStatus{Phase: handlers.BootstrapPhaseSetupRequired}), nil)
	normalHandler := NewNormal(handlers.NewAPI(config.Config{}, nil, nil), config.Config{})
	normalWithoutBusinessHandler := NewNormal(nil, config.Config{})
	brokenHandler := NewBroken(handlers.NewSystemAPI(handlers.BootstrapStatus{Phase: handlers.BootstrapPhaseBroken}))
	testCases := []struct {
		name      string
		handler   http.Handler
		path      string
		requested string
		want      int
	}{
		{name: "setup session post", handler: setupHandler, path: "/api/setup/v1/session", requested: http.MethodPost, want: http.StatusNoContent},
		{name: "setup session get absent", handler: setupHandler, path: "/api/setup/v1/session", requested: http.MethodGet, want: http.StatusNotFound},
		{name: "setup business absent", handler: setupHandler, path: "/api/agent/auth/v1/login/password", requested: http.MethodPost, want: http.StatusNotFound},
		{name: "setup asset absent", handler: setupHandler, path: "/setup/assets/app.js", requested: http.MethodGet, want: http.StatusNotFound},
		{name: "setup metrics absent", handler: setupHandler, path: "/metrics", requested: http.MethodGet, want: http.StatusNotFound},
		{name: "setup pprof absent", handler: setupHandler, path: "/debug/pprof/", requested: http.MethodGet, want: http.StatusNotFound},
		{name: "normal business post", handler: normalHandler, path: "/api/agent/auth/v1/login/password", requested: http.MethodPost, want: http.StatusNoContent},
		{name: "normal business wrong get", handler: normalHandler, path: "/api/agent/auth/v1/login/password", requested: http.MethodGet, want: http.StatusNotFound},
		{name: "normal business wrong delete", handler: normalHandler, path: "/api/agent/auth/v1/login/password", requested: http.MethodDelete, want: http.StatusNotFound},
		{name: "normal exact trailing slash absent", handler: normalHandler, path: "/api/agent/auth/v1/login/password/", requested: http.MethodPost, want: http.StatusNotFound},
		{name: "normal gallery get", handler: normalHandler, path: "/api/open/image/v1/gallery/images", requested: http.MethodGet, want: http.StatusNoContent},
		{name: "normal gallery wrong post", handler: normalHandler, path: "/api/open/image/v1/gallery/images", requested: http.MethodPost, want: http.StatusNotFound},
		{name: "normal dynamic admin delete", handler: normalHandler, path: "/api/ops/admin/v1/users/42", requested: http.MethodDelete, want: http.StatusNoContent},
		{name: "normal dynamic admin wrong patch", handler: normalHandler, path: "/api/ops/admin/v1/users/42", requested: http.MethodPatch, want: http.StatusNotFound},
		{name: "normal dynamic admin trailing slash absent", handler: normalHandler, path: "/api/ops/admin/v1/users/42/", requested: http.MethodDelete, want: http.StatusNotFound},
		{name: "normal dynamic admin double slash absent", handler: normalHandler, path: "/api/ops/admin/v1/users//42", requested: http.MethodDelete, want: http.StatusNotFound},
		{name: "normal dynamic admin encoded slash absent", handler: normalHandler, path: "/api/ops/admin/v1/users/%2F", requested: http.MethodDelete, want: http.StatusNotFound},
		{name: "normal profile put", handler: normalHandler, path: "/api/agent/user/v1/profile", requested: http.MethodPut, want: http.StatusNoContent},
		{name: "normal profile wrong delete", handler: normalHandler, path: "/api/agent/user/v1/profile", requested: http.MethodDelete, want: http.StatusNotFound},
		{name: "normal supplemental preferences put", handler: normalHandler, path: "/api/agent/user/v1/preferences", requested: http.MethodPut, want: http.StatusNoContent},
		{name: "normal supplemental preferences wrong get", handler: normalHandler, path: "/api/agent/user/v1/preferences", requested: http.MethodGet, want: http.StatusNotFound},
		{name: "normal supplemental developer update", handler: normalHandler, path: "/api/agent/developer/v1/api-keys/42", requested: http.MethodPatch, want: http.StatusNoContent},
		{name: "normal supplemental developer wrong post", handler: normalHandler, path: "/api/agent/developer/v1/api-keys/42", requested: http.MethodPost, want: http.StatusNotFound},
		{name: "normal supplemental developer reset", handler: normalHandler, path: "/api/agent/developer/v1/api-keys/42/reset-secret", requested: http.MethodPost, want: http.StatusNoContent},
		{name: "normal supplemental gallery group", handler: normalHandler, path: "/api/agent/gallery/v1/images/image-1/group", requested: http.MethodPut, want: http.StatusNoContent},
		{name: "normal supplemental gallery group wrong delete", handler: normalHandler, path: "/api/agent/gallery/v1/images/image-1/group", requested: http.MethodDelete, want: http.StatusNotFound},
		{name: "normal without API has no business preflight", handler: normalWithoutBusinessHandler, path: "/api/agent/auth/v1/login/password", requested: http.MethodPost, want: http.StatusNotFound},
		{name: "normal setup write absent", handler: normalHandler, path: "/api/setup/v1/apply", requested: http.MethodPost, want: http.StatusNotFound},
		{name: "normal setup asset absent", handler: normalHandler, path: "/setup/assets/app.js", requested: http.MethodGet, want: http.StatusNotFound},
		{name: "normal metrics get", handler: normalHandler, path: "/metrics", requested: http.MethodGet, want: http.StatusNoContent},
		{name: "normal metrics post absent", handler: normalHandler, path: "/metrics", requested: http.MethodPost, want: http.StatusNotFound},
		{name: "normal pprof get", handler: normalHandler, path: "/debug/pprof/", requested: http.MethodGet, want: http.StatusNoContent},
		{name: "normal pprof delete absent", handler: normalHandler, path: "/debug/pprof/", requested: http.MethodDelete, want: http.StatusNotFound},
		{name: "broken health get", handler: brokenHandler, path: "/healthz", requested: http.MethodGet, want: http.StatusNoContent},
		{name: "broken setup write absent", handler: brokenHandler, path: "/api/setup/v1/apply", requested: http.MethodPost, want: http.StatusNotFound},
		{name: "broken business absent", handler: brokenHandler, path: "/api/agent/auth/v1/login/password", requested: http.MethodPost, want: http.StatusNotFound},
		{name: "broken metrics absent", handler: brokenHandler, path: "/metrics", requested: http.MethodGet, want: http.StatusNotFound},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodOptions, testCase.path, nil)
			request.Header.Set("Origin", "http://localhost:5173")
			request.Header.Set("Access-Control-Request-Method", testCase.requested)
			testCase.handler.ServeHTTP(recorder, request)
			if recorder.Code != testCase.want {
				t.Fatalf("OPTIONS %s for %s = %d, want %d; body=%s", testCase.path, testCase.requested, recorder.Code, testCase.want, recorder.Body.String())
			}
		})
	}
}

func newSetupAPIForRouterTest(t *testing.T, status handlers.BootstrapStatus) *handlers.SetupAPI {
	t.Helper()
	api, err := handlers.NewSetupAPI(handlers.SetupAPIOptions{
		System:      handlers.NewSystemAPI(status),
		Auth:        setupAuthStub{},
		Prober:      setupProbeStub{},
		Application: setupApplicationStub{},
	})
	if err != nil {
		t.Fatalf("NewSetupAPI: %v", err)
	}
	return api
}

func assertRouteStatus(t *testing.T, handler http.Handler, method, target string, want int) {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(`{}`))
	handler.ServeHTTP(recorder, request)
	if recorder.Code != want {
		t.Fatalf("%s %s status = %d, want %d; body=%s", method, target, recorder.Code, want, recorder.Body.String())
	}
}
