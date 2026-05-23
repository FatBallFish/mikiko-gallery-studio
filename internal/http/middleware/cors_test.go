package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSAllowsLocalFrontendPreflight(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/api/agent/auth/v1/login/email-code", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "content-type, authorization")
	rec := httptest.NewRecorder()

	CORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("preflight should not call next handler")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected preflight status 204, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("expected allow origin for local frontend, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected credentials to be allowed, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "content-type, authorization" {
		t.Fatalf("expected requested headers to be echoed, got %q", got)
	}
}

func TestCORSAddsHeadersToActualLocalFrontendRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	req.Header.Set("Origin", "http://localhost:5174")
	rec := httptest.NewRecorder()

	CORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5174" {
		t.Fatalf("expected allow origin for admin frontend, got %q", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("expected vary origin, got %q", got)
	}
}
