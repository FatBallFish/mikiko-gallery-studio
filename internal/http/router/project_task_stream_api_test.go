package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	domainproject "github.com/fatballfish/pic-gallery/internal/domain/project"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	"github.com/fatballfish/pic-gallery/internal/provider"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	projectservice "github.com/fatballfish/pic-gallery/internal/service/project"
)

func TestTaskStreamScopesByOwnedProjectBeforeRecentLimit(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: 2 * time.Hour,
		Issuer: "test", AccessTokenSecret: "secret", RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("project-stream@example.com", "login"); err != nil {
		t.Fatal(err)
	}
	user, session, err := loginAuthUserWithPasswordSetup(t, authSvc, "project-stream@example.com", "123456")
	if err != nil {
		t.Fatal(err)
	}
	projectSvc := projectservice.NewService(projectservice.NewMemoryStore())
	projectA, err := projectSvc.EnsureDefault(t.Context(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	projectB, err := projectSvc.Create(t.Context(), user.ID, domainproject.CreateRequest{Name: "Project B"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := projectSvc.Create(t.Context(), user.ID+1, domainproject.CreateRequest{Name: "Foreign"})
	if err != nil {
		t.Fatal(err)
	}
	store := imagetaskservice.NewMemoryStore()
	taskSvc := imagetaskservice.NewServiceWithStore(cfg, store)
	taskSvc.SetProjectResolver(projectSvc)
	oldTaskID := "00000000-0000-4000-8000-000000000001"
	if err := store.Save(t.Context(), domainimagetask.Task{
		ID: oldTaskID, UserID: user.ID, ProjectID: projectA.ID, Status: domainimagetask.StatusSucceeded,
		TaskType: string(provider.TaskTypeTextToImage), AbstractModel: "plus", Prompt: "old project A task",
		CreatedAt: time.Now().UTC().Add(-24 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	for index := range 20 {
		if err := store.Save(t.Context(), domainimagetask.Task{
			ID: fmt.Sprintf("10000000-0000-4000-8000-%012d", index), UserID: user.ID, ProjectID: projectB.ID,
			Status: domainimagetask.StatusSucceeded, TaskType: string(provider.TaskTypeTextToImage), AbstractModel: "plus",
			Prompt: fmt.Sprintf("new project B task %d", index), CreatedAt: time.Now().UTC().Add(time.Duration(index) * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	api := handlers.NewAPIWithTaskService(cfg, authSvc, nil, taskSvc)
	api.SetProjectService(projectSvc)
	handler := NewWithAPI(api)

	streamReq := httptest.NewRequest(http.MethodGet, "/api/agent/image/v1/tasks/events?once=true&project_id="+projectA.ID, nil)
	streamReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	streamRec := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
	handler.ServeHTTP(streamRec, streamReq)
	if streamRec.Code != http.StatusOK {
		t.Fatalf("project A stream status = %d body=%s", streamRec.Code, streamRec.Body.String())
	}
	if body := streamRec.Body.String(); !strings.Contains(body, oldTaskID) || strings.Contains(body, "new project B task") {
		t.Fatalf("project A stream history = %q", body)
	}

	foreignReq := httptest.NewRequest(http.MethodGet, "/api/agent/image/v1/tasks/events?once=true&project_id="+foreign.ID, nil)
	foreignReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	foreignRec := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
	handler.ServeHTTP(foreignRec, foreignReq)
	if foreignRec.Code != http.StatusNotFound {
		t.Fatalf("foreign project stream status = %d body=%s", foreignRec.Code, foreignRec.Body.String())
	}
}
