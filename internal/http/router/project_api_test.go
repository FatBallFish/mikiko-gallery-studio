package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	projectservice "github.com/fatballfish/pic-gallery/internal/service/project"
)

func TestProjectAPIExposesOwnedCRUDAndTypedConflicts(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: 2 * time.Hour,
		Issuer: "test", AccessTokenSecret: "secret", RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("project-api@example.com", "login"); err != nil {
		t.Fatal(err)
	}
	user, session, err := loginAuthUserWithPasswordSetup(t, authSvc, "project-api@example.com", "123456")
	if err != nil {
		t.Fatal(err)
	}
	projectSvc := projectservice.NewService(projectservice.NewMemoryStore())
	api := handlers.NewAPIWithTaskService(cfg, authSvc, nil, nil)
	api.SetProjectService(projectSvc)
	handler := NewWithAPI(api)

	list := authenticatedProjectRequest(t, handler, session.AccessToken, http.MethodGet, "/api/agent/project/v1/projects", "", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", list.Code, list.Body.String())
	}
	var listPayload struct {
		Data struct {
			Items []struct {
				ID        string `json:"id"`
				Name      string `json:"name"`
				IsDefault bool   `json:"is_default"`
				Version   int64  `json:"version"`
			} `json:"items"`
			DefaultProjectID string `json:"default_project_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(list.Body).Decode(&listPayload); err != nil {
		t.Fatal(err)
	}
	if len(listPayload.Data.Items) != 1 || !listPayload.Data.Items[0].IsDefault || listPayload.Data.DefaultProjectID != listPayload.Data.Items[0].ID {
		t.Fatalf("default project payload = %#v", listPayload.Data)
	}

	createHeaders := map[string]string{"Idempotency-Key": "project-api-create"}
	created := authenticatedProjectRequest(t, handler, session.AccessToken, http.MethodPost, "/api/agent/project/v1/projects", `{"name":"Launch"}`, createHeaders)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", created.Code, created.Body.String())
	}
	var createdPayload struct {
		Data struct {
			ID      string `json:"id"`
			Version int64  `json:"version"`
		} `json:"data"`
	}
	if err := json.NewDecoder(created.Body).Decode(&createdPayload); err != nil {
		t.Fatal(err)
	}

	stale := authenticatedProjectRequest(t, handler, session.AccessToken, http.MethodPatch, "/api/agent/project/v1/projects/"+createdPayload.Data.ID, `{"name":"Stale","expected_version":999}`, nil)
	if stale.Code != http.StatusConflict || responseErrorCode(t, stale) != "project_changed" {
		t.Fatalf("stale response = %d %s", stale.Code, stale.Body.String())
	}
	immutable := authenticatedProjectRequest(t, handler, session.AccessToken, http.MethodDelete, "/api/agent/project/v1/projects/"+listPayload.Data.DefaultProjectID, `{"expected_version":1}`, nil)
	if immutable.Code != http.StatusForbidden || responseErrorCode(t, immutable) != "default_project_immutable" {
		t.Fatalf("immutable response = %d %s", immutable.Code, immutable.Body.String())
	}
	deleteHeaders := map[string]string{"Idempotency-Key": "delete-project-api"}
	deleted := authenticatedProjectRequest(t, handler, session.AccessToken, http.MethodDelete, "/api/agent/project/v1/projects/"+createdPayload.Data.ID, `{"expected_version":1}`, deleteHeaders)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete status = %d body=%s", deleted.Code, deleted.Body.String())
	}
	replayedDelete := authenticatedProjectRequest(t, handler, session.AccessToken, http.MethodDelete, "/api/agent/project/v1/projects/"+createdPayload.Data.ID, `{"expected_version":1}`, deleteHeaders)
	if replayedDelete.Code != http.StatusOK || !bytes.Contains(replayedDelete.Body.Bytes(), []byte(createdPayload.Data.ID)) {
		t.Fatalf("delete replay = %d %s, want persisted project %s", replayedDelete.Code, replayedDelete.Body.String(), createdPayload.Data.ID)
	}

	if projects, err := projectSvc.List(t.Context(), user.ID); err != nil || len(projects) != 1 {
		t.Fatalf("service projects = %#v, %v", projects, err)
	}
}

func authenticatedProjectRequest(t *testing.T, handler http.Handler, token, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func responseErrorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	return payload.Error.Code
}
