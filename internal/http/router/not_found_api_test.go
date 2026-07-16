package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUnknownAPIRouteDoesNotFallThroughToRootSuccess(t *testing.T) {
	handler := New()
	req := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/not-a-real-route", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown API route status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}
