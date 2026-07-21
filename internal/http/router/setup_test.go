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
