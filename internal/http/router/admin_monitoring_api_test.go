package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
)

func TestAdminMonitoringSnapshotSupportsApprovedWindows(t *testing.T) {
	handler, token := monitoringTestHandler(t)

	for _, window := range []string{"5m", "15m", "30m", "60m"} {
		t.Run(window, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/monitoring/snapshot?window="+window, nil)
			request.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("snapshot %s: status=%d body=%s", window, response.Code, response.Body.String())
			}
			var payload struct {
				Data struct {
					GeneratedAt           string           `json:"generated_at"`
					Window                string           `json:"window"`
					SampleIntervalSeconds int              `json:"sample_interval_seconds"`
					State                 string           `json:"state"`
					Current               map[string]any   `json:"current"`
					Series                []map[string]any `json:"series"`
					Statuses              map[string]any   `json:"statuses"`
					Routes                []map[string]any `json:"routes"`
					Providers             []map[string]any `json:"providers"`
				} `json:"data"`
			}
			if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
				t.Fatalf("decode snapshot: %v", err)
			}
			if payload.Data.Window != window || payload.Data.GeneratedAt == "" || payload.Data.SampleIntervalSeconds != 5 {
				t.Fatalf("unexpected metadata: %#v", payload.Data)
			}
			if payload.Data.State == "" || payload.Data.Current == nil || payload.Data.Statuses == nil || payload.Data.Series == nil || payload.Data.Routes == nil || payload.Data.Providers == nil {
				t.Fatalf("snapshot contract missing fields: %#v", payload.Data)
			}
		})
	}
}

func TestAdminMonitoringSnapshotRejectsInvalidWindowAndMissingAuth(t *testing.T) {
	handler, token := monitoringTestHandler(t)

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/monitoring/snapshot?window=5m", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("missing auth status=%d body=%s", unauthorized.Code, unauthorized.Body.String())
	}

	request := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/monitoring/snapshot?window=10m", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid window status=%d body=%s", response.Code, response.Body.String())
	}
	var failure struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&failure); err != nil {
		t.Fatalf("decode validation error: %v", err)
	}
	if failure.Error.Code == "" || failure.Error.Message == "" {
		t.Fatalf("expected structured validation error, got %#v", failure)
	}
}

func monitoringTestHandler(t *testing.T) (http.Handler, string) {
	t.Helper()
	cfg := adminConfigAPIConfig()
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "monitoring-admin@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	api := handlers.NewAPIWithModelAdminService(cfg, nil, nil, nil, nil, nil, nil, adminAuth, nil, nil, nil, nil, nil)
	api.SetAdminPermissionResolver(readOnlyRouteResolver{})
	handler := NewWithAPI(api)

	loginRequest := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"monitoring-admin@example.com","password":"password"}`))
	loginRequest.Header.Set("Content-Type", "application/json")
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, loginRequest)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", loginResponse.Code, loginResponse.Body.String())
	}
	var login struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(loginResponse.Body).Decode(&login); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if login.Data.AccessToken == "" {
		t.Fatal("missing monitoring admin access token")
	}
	return handler, login.Data.AccessToken
}
