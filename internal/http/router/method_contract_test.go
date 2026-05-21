package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/http/handlers"
)

func TestMethodNotAllowedResponsesUseJSONEnvelope(t *testing.T) {
	handler := NewWithAPI(handlers.NewAPI(taskAPIConfig("http://127.0.0.1:1"), nil, nil))

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{name: "send email code", method: http.MethodGet, path: "/api/agent/auth/v1/email/send-code"},
		{name: "login email code", method: http.MethodGet, path: "/api/agent/auth/v1/login/email-code"},
		{name: "refresh", method: http.MethodGet, path: "/api/agent/auth/v1/session/refresh"},
		{name: "agent estimate", method: http.MethodPost, path: "/api/agent/billing/v1/estimate"},
		{name: "agent capabilities", method: http.MethodPost, path: "/api/agent/image/v1/capabilities"},
		{name: "reference upload", method: http.MethodGet, path: "/api/agent/image/v1/reference-assets"},
		{name: "reference download", method: http.MethodPost, path: "/api/agent/image/v1/reference-assets/asset-id/download"},
		{name: "agent tasks", method: http.MethodPatch, path: "/api/agent/image/v1/tasks"},
		{name: "agent task detail", method: http.MethodPost, path: "/api/agent/image/v1/tasks/task-id"},
		{name: "history tasks", method: http.MethodPost, path: "/api/agent/image/v1/history/tasks"},
		{name: "history task detail", method: http.MethodPatch, path: "/api/agent/image/v1/history/tasks/task-id"},
		{name: "compat generations", method: http.MethodGet, path: "/v1/images/generations"},
		{name: "compat edits", method: http.MethodGet, path: "/v1/images/edits"},
		{name: "compat models", method: http.MethodPost, path: "/v1/models"},
	}

	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"bad":`))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s expected 405, got %d body=%s", tc.name, rec.Code, rec.Body.String())
		}
		if contentType := rec.Header().Get("Content-Type"); !strings.Contains(contentType, "application/json") {
			t.Fatalf("%s expected JSON content type, got %q body=%s", tc.name, contentType, rec.Body.String())
		}
		var payload struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
			t.Fatalf("%s expected JSON error body: %v", tc.name, err)
		}
		if payload.Error.Code != "METHOD_NOT_ALLOWED" || payload.Error.Message == "" {
			t.Fatalf("%s unexpected error payload %#v", tc.name, payload)
		}
	}
}
