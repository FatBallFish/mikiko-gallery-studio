package router

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	canvasservice "github.com/fatballfish/pic-gallery/internal/service/canvas"
)

func TestCanvasAPICRUDRevisionConflictAndOwnerIsolation(t *testing.T) {
	cfg := taskAPIConfig("http://provider.invalid")
	authSvc, owner := loginTestUser(t, "canvas-owner@example.com")
	other := loginExistingAuthUser(t, authSvc, "canvas-other@example.com")
	projectID := uuid.New()
	service := canvasservice.NewService(canvasservice.NewMemoryStore(), nil, nil)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, enabledFeatureAdmin(t, "creative_canvas"), nil)
	api.SetCanvasService(service)
	handler := NewWithAPI(api)

	created := authenticatedProjectRequest(t, handler, owner.AccessToken, http.MethodPost, "/api/agent/canvas/v1/canvases", `{"project_id":"`+projectID.String()+`","name":"Storyboard","template":"image_exploration"}`, nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("create=%d %s", created.Code, created.Body.String())
	}
	var payload struct {
		Data struct {
			ID       string `json:"id"`
			Revision int64  `json:"revision"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	foreign := authenticatedProjectRequest(t, handler, other.AccessToken, http.MethodGet, "/api/agent/canvas/v1/canvases/"+payload.Data.ID, "", nil)
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("foreign=%d %s", foreign.Code, foreign.Body.String())
	}

	doc := `{"expected_revision":1,"document":{"schema_version":1,"viewport":{"x":0,"y":0,"zoom":1},"nodes":[{"id":"note","type":"note","position":{"x":0,"y":0},"size":{"width":100,"height":100}}],"edges":[]}}`
	saved := authenticatedProjectRequest(t, handler, owner.AccessToken, http.MethodPut, "/api/agent/canvas/v1/canvases/"+payload.Data.ID+"/document", doc, nil)
	if saved.Code != http.StatusOK || !bytes.Contains(saved.Body.Bytes(), []byte(`"revision":2`)) {
		t.Fatalf("save=%d %s", saved.Code, saved.Body.String())
	}
	stale := authenticatedProjectRequest(t, handler, owner.AccessToken, http.MethodPut, "/api/agent/canvas/v1/canvases/"+payload.Data.ID+"/document", doc, nil)
	if stale.Code != http.StatusConflict || !bytes.Contains(stale.Body.Bytes(), []byte(`"remote_revision":2`)) {
		t.Fatalf("stale=%d %s", stale.Code, stale.Body.String())
	}
	list := authenticatedProjectRequest(t, handler, owner.AccessToken, http.MethodGet, "/api/agent/canvas/v1/canvases?project_id="+projectID.String(), "", nil)
	if list.Code != http.StatusOK || !bytes.Contains(list.Body.Bytes(), []byte(payload.Data.ID)) {
		t.Fatalf("list=%d %s", list.Code, list.Body.String())
	}
}

func TestCanvasAPIRunStatusCancelAndAttachRoutes(t *testing.T) {
	cfg := taskAPIConfig("http://provider.invalid")
	authSvc, owner := loginTestUser(t, "canvas-run@example.com")
	claims, err := authSvc.ParseAccessToken(owner.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	projectID := uuid.New()
	generator := &canvasAPIGenerator{result: uuid.New()}
	service := canvasservice.NewService(canvasservice.NewMemoryStore(), generator, nil)
	created, err := service.Create(t.Context(), canvasservice.CreateRequest{UserID: claims.UserID, ProjectID: projectID, Name: "Run", Template: canvasservice.TemplateImageExploration})
	if err != nil {
		t.Fatal(err)
	}
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, enabledFeatureAdmin(t, "creative_canvas"), nil)
	api.SetCanvasService(service)
	handler := NewWithAPI(api)
	generated := authenticatedProjectRequest(t, handler, owner.AccessToken, http.MethodPost, "/api/agent/canvas/v1/canvases/"+created.ID.String()+"/nodes/image-generation:generate", "", map[string]string{"Idempotency-Key": "canvas-api-run"})
	if generated.Code != http.StatusAccepted {
		t.Fatalf("generate=%d %s", generated.Code, generated.Body.String())
	}
	var body struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(generated.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	runs := authenticatedProjectRequest(t, handler, owner.AccessToken, http.MethodGet, "/api/agent/canvas/v1/canvases/"+created.ID.String()+"/runs?refresh=true", "", nil)
	if runs.Code != http.StatusOK || !bytes.Contains(runs.Body.Bytes(), []byte(`"status":"attached"`)) {
		t.Fatalf("runs=%d %s", runs.Code, runs.Body.String())
	}
	attached := authenticatedProjectRequest(t, handler, owner.AccessToken, http.MethodPost, "/api/agent/canvas/v1/canvases/"+created.ID.String()+"/runs/"+body.Data.ID+":attach-results", "", nil)
	if attached.Code != http.StatusOK || !bytes.Contains(attached.Body.Bytes(), []byte(`"status":"attached"`)) {
		t.Fatalf("attach=%d %s", attached.Code, attached.Body.String())
	}
}

func TestCanvasAPIAttachUnplacedResultsAtViewportCenter(t *testing.T) {
	cfg := taskAPIConfig("http://provider.invalid")
	authSvc, owner := loginTestUser(t, "canvas-unplaced@example.com")
	claims, err := authSvc.ParseAccessToken(owner.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	service := canvasservice.NewService(canvasservice.NewMemoryStore(), &canvasAPIGenerator{result: uuid.New()}, nil)
	created, err := service.Create(t.Context(), canvasservice.CreateRequest{UserID: claims.UserID, ProjectID: uuid.New(), Name: "Unplaced", Template: canvasservice.TemplateImageExploration})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.Generate(t.Context(), canvasservice.GenerateRequest{UserID: claims.UserID, CanvasID: created.ID, NodeID: "image-generation", IdempotencyKey: "unplaced-api"})
	if err != nil {
		t.Fatal(err)
	}
	created.Document.Nodes = created.Document.Nodes[:1]
	created.Document.Edges = nil
	if _, err := service.SaveDocument(t.Context(), canvasservice.SaveDocumentRequest{UserID: claims.UserID, CanvasID: created.ID, ExpectedRevision: created.Revision, Document: created.Document}); err != nil {
		t.Fatal(err)
	}
	if unplaced, err := service.RefreshRun(t.Context(), claims.UserID, created.ID, run.ID); err != nil || unplaced.Status != canvasservice.RunStatusUnplaced {
		t.Fatalf("unplaced = (%#v, %v)", unplaced, err)
	}
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, enabledFeatureAdmin(t, "creative_canvas"), nil)
	api.SetCanvasService(service)
	handler := NewWithAPI(api)
	response := authenticatedProjectRequest(t, handler, owner.AccessToken, http.MethodPost, "/api/agent/canvas/v1/canvases/"+created.ID.String()+"/runs/"+run.ID.String()+":attach-results", `{"recovery_position":{"x":700,"y":450}}`, nil)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"status":"attached"`)) {
		t.Fatalf("recover=%d %s", response.Code, response.Body.String())
	}
	after, err := service.Get(t.Context(), claims.UserID, created.ID)
	if err != nil || len(after.Document.Nodes) != 2 || after.Document.Nodes[1].Position.X != 540 || after.Document.Nodes[1].Position.Y != 330 {
		t.Fatalf("recovered canvas=(%#v,%v)", after.Document, err)
	}
}

type canvasAPIGenerator struct{ result uuid.UUID }

func (c *canvasAPIGenerator) Estimate(context.Context, canvasservice.GenerationSubmission) (canvasservice.Estimate, error) {
	return canvasservice.Estimate{Points: "1.00000"}, nil
}
func (c *canvasAPIGenerator) Generate(context.Context, canvasservice.GenerationSubmission) (canvasservice.GenerationTask, error) {
	return canvasservice.GenerationTask{TaskID: uuid.New(), Kind: canvasservice.TaskKindImage, Status: canvasservice.RunStatusRunning}, nil
}
func (c *canvasAPIGenerator) Status(context.Context, int64, canvasservice.TaskKind, uuid.UUID) (canvasservice.TaskStatus, error) {
	return canvasservice.TaskStatus{Status: canvasservice.RunStatusSucceeded, ResultAssetIDs: []uuid.UUID{c.result}}, nil
}
func (c *canvasAPIGenerator) Cancel(context.Context, int64, canvasservice.TaskKind, uuid.UUID) error {
	return nil
}
