package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/http/handlers"
)

func TestFeatureFlagsProjectionRouteDefaultsClosedAndSupportsCORS(t *testing.T) {
	handler := NewWithAPI(handlers.NewAPI(taskAPIConfig("http://provider.invalid"), nil, nil))

	request := httptest.NewRequest(http.MethodGet, "/api/agent/features/v1", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"video_creation":false`) || !strings.Contains(recorder.Body.String(), `"creative_canvas":false`) || !strings.Contains(recorder.Body.String(), `"media_upload":false`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	preflight := httptest.NewRequest(http.MethodOptions, "/api/agent/features/v1", nil)
	preflight.Header.Set("Origin", "http://localhost:5173")
	preflight.Header.Set("Access-Control-Request-Method", http.MethodGet)
	preflightRecorder := httptest.NewRecorder()
	handler.ServeHTTP(preflightRecorder, preflight)
	if preflightRecorder.Code != http.StatusNoContent {
		t.Fatalf("preflight=%d body=%s", preflightRecorder.Code, preflightRecorder.Body.String())
	}
}
