package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRouterExposesMetricsEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	New().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected /metrics status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "pic_gallery_http_requests_total") {
		t.Fatalf("expected custom request metric in response body, got %q", rec.Body.String())
	}
}
