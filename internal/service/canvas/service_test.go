package canvas

import (
	"context"
	"errors"
	"strings"
	"testing"

	domaincanvas "github.com/fatballfish/pic-gallery/internal/domain/canvas"
	"github.com/google/uuid"
)

func TestCanvasCRUDRevisionAndOwnerIsolation(t *testing.T) {
	store := NewMemoryStore()
	service := NewService(store, nil, nil)
	observer := &canvasObserverSpy{}
	service.SetObserver(observer)
	projectID := uuid.New()

	created, err := service.Create(t.Context(), CreateRequest{UserID: 7, ProjectID: projectID, Name: "探索", Template: TemplateImageExploration})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Revision != 1 || created.MetadataVersion != 1 || len(created.Document.Nodes) != 2 {
		t.Fatalf("Create() = %#v", created)
	}
	if _, err := service.Get(t.Context(), 8, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign Get() error = %v, want not found", err)
	}
	if _, err := service.ListRuns(t.Context(), 8, created.ID, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign ListRuns() error = %v, want not found", err)
	}

	saved, err := service.SaveDocument(t.Context(), SaveDocumentRequest{UserID: 7, CanvasID: created.ID, ExpectedRevision: 1, Document: domaincanvas.DocumentV1{SchemaVersion: 1, Nodes: []domaincanvas.Node{{ID: "asset", Type: domaincanvas.NodeTypeImage, AssetID: "  " + uuid.NewString() + "  "}}}})
	if err != nil {
		t.Fatalf("SaveDocument() error = %v", err)
	}
	if saved.Revision != 2 || len(saved.AssetReferences) != 1 {
		t.Fatalf("saved = %#v", saved)
	}
	_, err = service.SaveDocument(t.Context(), SaveDocumentRequest{UserID: 7, CanvasID: created.ID, ExpectedRevision: 1, Document: saved.Document})
	var conflict *RevisionConflictError
	if !errors.As(err, &conflict) || conflict.RemoteRevision != 2 || conflict.Summary.NodeCount != 1 {
		t.Fatalf("stale SaveDocument() error = %#v", err)
	}
	if got := strings.Join(observer.results, ","); got != "success,failed" {
		t.Fatalf("canvas save observations = %q", got)
	}

	renamed, err := service.Rename(t.Context(), RenameRequest{UserID: 7, CanvasID: created.ID, Name: "新名字", ExpectedMetadataVersion: 1})
	if err != nil || renamed.Name != "新名字" || renamed.MetadataVersion != 2 {
		t.Fatalf("Rename() = (%#v, %v)", renamed, err)
	}
	copy, err := service.Duplicate(t.Context(), DuplicateRequest{UserID: 7, CanvasID: created.ID, Name: "副本"})
	if err != nil || copy.ID == created.ID || copy.Revision != 1 || copy.RunningTaskCount != 0 || len(copy.AssetReferences) != 1 {
		t.Fatalf("Duplicate() = (%#v, %v)", copy, err)
	}
	items, err := service.List(t.Context(), ListRequest{UserID: 7, ProjectID: &projectID})
	if err != nil || len(items) != 2 {
		t.Fatalf("List() = (%#v, %v)", items, err)
	}
}

type canvasObserverSpy struct{ results []string }

func (spy *canvasObserverSpy) RecordCanvasSave(result string) {
	spy.results = append(spy.results, result)
}

func TestCanvasDeleteAndTransferRejectActiveRuns(t *testing.T) {
	store := NewMemoryStore()
	service := NewService(store, &fakeGenerator{}, nil)
	created, err := service.Create(t.Context(), CreateRequest{UserID: 3, ProjectID: uuid.New(), Name: "运行中", Template: TemplateImageExploration})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.Generate(t.Context(), GenerateRequest{UserID: 3, CanvasID: created.ID, NodeID: "image-generation", IdempotencyKey: "run-1"})
	if err != nil || run.Status != RunStatusRunning {
		t.Fatalf("Generate() = (%#v, %v)", run, err)
	}
	if _, err := service.TransferProject(t.Context(), TransferProjectRequest{UserID: 3, CanvasID: created.ID, TargetProjectID: uuid.New(), ExpectedMetadataVersion: 1}); !errors.Is(err, ErrCanvasBusy) {
		t.Fatalf("TransferProject() error = %v, want busy", err)
	}
	if err := service.Delete(t.Context(), DeleteRequest{UserID: 3, CanvasID: created.ID, ExpectedMetadataVersion: 1}); !errors.Is(err, ErrCanvasBusy) {
		t.Fatalf("Delete() error = %v, want busy", err)
	}
}

func TestImageToVideoTemplateDoesNotCreateDanglingAssetReference(t *testing.T) {
	service := NewService(NewMemoryStore(), nil, nil)
	created, err := service.Create(t.Context(), CreateRequest{UserID: 3, ProjectID: uuid.New(), Name: "视频", Template: TemplateImageToVideo})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if len(created.AssetReferences) != 0 || len(created.Document.Nodes) != 2 || created.Document.Nodes[1].Type != domaincanvas.NodeTypeVideoGeneration {
		t.Fatalf("image-to-video template = %#v", created.Document)
	}
}

func TestCanvasDocumentAllowsURLInPromptButRejectsPersistedURLField(t *testing.T) {
	service := NewService(NewMemoryStore(), nil, nil)
	projectID := uuid.New()
	allowed := domaincanvas.DocumentV1{SchemaVersion: 1, Nodes: []domaincanvas.Node{{ID: "prompt", Type: domaincanvas.NodeTypePrompt, Payload: []byte(`{"text":"describe https://example.com"}`)}}}
	if _, err := service.Create(t.Context(), CreateRequest{UserID: 3, ProjectID: projectID, Name: "Allowed", Document: &allowed}); err != nil {
		t.Fatalf("prompt URL rejected: %v", err)
	}
	forbidden := domaincanvas.DocumentV1{SchemaVersion: 1, Nodes: []domaincanvas.Node{{ID: "note", Type: domaincanvas.NodeTypeNote, Payload: []byte(`{"preview_url":"https://example.com/media"}`)}}}
	if _, err := service.Create(t.Context(), CreateRequest{UserID: 3, ProjectID: projectID, Name: "Forbidden", Document: &forbidden}); err == nil {
		t.Fatal("persisted preview_url was accepted")
	}
}

func TestCanvasAttachResultsIsStableIdempotentAndCanBecomeUnplaced(t *testing.T) {
	store := NewMemoryStore()
	assets := []uuid.UUID{uuid.New(), uuid.New()}
	generator := &fakeGenerator{status: TaskStatus{Status: RunStatusSucceeded, ResultAssetIDs: assets}}
	service := NewService(store, generator, nil)
	created, _ := service.Create(t.Context(), CreateRequest{UserID: 9, ProjectID: uuid.New(), Name: "Attach", Template: TemplateImageExploration})
	run, err := service.Generate(t.Context(), GenerateRequest{UserID: 9, CanvasID: created.ID, NodeID: "image-generation", IdempotencyKey: "stable"})
	if err != nil {
		t.Fatal(err)
	}
	attached, err := service.AttachResults(t.Context(), AttachResultsRequest{UserID: 9, CanvasID: created.ID, RunID: run.ID})
	if err != nil || attached.Status != RunStatusAttached {
		t.Fatalf("AttachResults() = (%#v, %v)", attached, err)
	}
	canvasAfter, _ := service.Get(t.Context(), 9, created.ID)
	if len(canvasAfter.Document.Nodes) != 4 || len(canvasAfter.Document.Edges) != 3 {
		t.Fatalf("attached document = %#v", canvasAfter.Document)
	}
	firstIDs := []string{canvasAfter.Document.Nodes[2].ID, canvasAfter.Document.Nodes[3].ID}
	if _, err := service.AttachResults(t.Context(), AttachResultsRequest{UserID: 9, CanvasID: created.ID, RunID: run.ID}); err != nil {
		t.Fatalf("repeated AttachResults() error = %v", err)
	}
	reloaded, _ := service.Get(t.Context(), 9, created.ID)
	if len(reloaded.Document.Nodes) != 4 || reloaded.Document.Nodes[2].ID != firstIDs[0] || reloaded.Document.Nodes[3].ID != firstIDs[1] {
		t.Fatalf("repeated attach changed document = %#v", reloaded.Document)
	}

	second, _ := service.Create(t.Context(), CreateRequest{UserID: 9, ProjectID: uuid.New(), Name: "Unplaced", Template: TemplateImageExploration})
	unplacedRun, _ := service.Generate(t.Context(), GenerateRequest{UserID: 9, CanvasID: second.ID, NodeID: "image-generation", IdempotencyKey: "unplaced"})
	current, _ := service.Get(t.Context(), 9, second.ID)
	current.Document.Nodes = current.Document.Nodes[:1]
	current.Document.Edges = nil
	_, _ = service.SaveDocument(t.Context(), SaveDocumentRequest{UserID: 9, CanvasID: second.ID, ExpectedRevision: current.Revision, Document: current.Document})
	unplaced, err := service.AttachResults(t.Context(), AttachResultsRequest{UserID: 9, CanvasID: second.ID, RunID: unplacedRun.ID})
	if err != nil || unplaced.Status != RunStatusUnplaced {
		t.Fatalf("unplaced AttachResults() = (%#v, %v)", unplaced, err)
	}
	if _, err := service.AttachResults(t.Context(), AttachResultsRequest{UserID: 9, CanvasID: second.ID, RunID: unplacedRun.ID, RecoveryPosition: &domaincanvas.Point{X: 1_000_001, Y: 0}}); err == nil {
		t.Fatal("out-of-range unplaced recovery position was accepted")
	}
	recovered, err := service.AttachResults(t.Context(), AttachResultsRequest{UserID: 9, CanvasID: second.ID, RunID: unplacedRun.ID, RecoveryPosition: &domaincanvas.Point{X: 800, Y: 600}})
	if err != nil || recovered.Status != RunStatusAttached || recovered.AttachedRevision == nil {
		t.Fatalf("recover unplaced AttachResults() = (%#v, %v)", recovered, err)
	}
	recoveredCanvas, err := service.Get(t.Context(), 9, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(recoveredCanvas.Document.Nodes) != 3 || len(recoveredCanvas.Document.Edges) != 0 {
		t.Fatalf("recovered unplaced document = %#v", recoveredCanvas.Document)
	}
	if recoveredCanvas.Document.Nodes[1].Position.X != 640 || recoveredCanvas.Document.Nodes[1].Position.Y != 480 {
		t.Fatalf("first recovered node position = %#v", recoveredCanvas.Document.Nodes[1].Position)
	}
	if again, err := service.AttachResults(t.Context(), AttachResultsRequest{UserID: 9, CanvasID: second.ID, RunID: unplacedRun.ID, RecoveryPosition: &domaincanvas.Point{X: 1, Y: 1}}); err != nil || again.Status != RunStatusAttached || *again.AttachedRevision != *recovered.AttachedRevision {
		t.Fatalf("idempotent unplaced recovery = (%#v, %v)", again, err)
	}
}

func TestCanvasRefreshRunAutomaticallyAttachesSucceededResults(t *testing.T) {
	store := NewMemoryStore()
	assetID := uuid.New()
	service := NewService(store, &fakeGenerator{status: TaskStatus{Status: RunStatusSucceeded, ResultAssetIDs: []uuid.UUID{assetID}}}, nil)
	created, err := service.Create(t.Context(), CreateRequest{UserID: 9, ProjectID: uuid.New(), Name: "Auto attach", Template: TemplateImageExploration})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.Generate(t.Context(), GenerateRequest{UserID: 9, CanvasID: created.ID, NodeID: "image-generation", IdempotencyKey: "auto-attach"})
	if err != nil {
		t.Fatal(err)
	}
	refreshed, err := service.RefreshRun(t.Context(), 9, created.ID, run.ID)
	if err != nil || refreshed.Status != RunStatusAttached {
		t.Fatalf("RefreshRun() = (%#v, %v), want attached", refreshed, err)
	}
	after, err := service.Get(t.Context(), 9, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Document.Nodes) != 3 || after.Document.Nodes[2].AssetID != assetID.String() || len(after.Document.Edges) != 2 {
		t.Fatalf("automatic result attachment = %#v", after.Document)
	}
}

func TestCanvasRefreshRunRecoversPersistedSucceededRun(t *testing.T) {
	store := NewMemoryStore()
	assetID := uuid.New()
	service := NewService(store, &fakeGenerator{}, nil)
	created, err := service.Create(t.Context(), CreateRequest{UserID: 9, ProjectID: uuid.New(), Name: "Recover attach", Template: TemplateImageExploration})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.Generate(t.Context(), GenerateRequest{UserID: 9, CanvasID: created.ID, NodeID: "image-generation", IdempotencyKey: "recover-attach"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateRun(t.Context(), RunUpdate{UserID: 9, CanvasID: created.ID, RunID: run.ID, Status: RunStatusSucceeded, ResultAssetIDs: []uuid.UUID{assetID}}); err != nil {
		t.Fatal(err)
	}
	refreshed, err := service.RefreshRun(t.Context(), 9, created.ID, run.ID)
	if err != nil || refreshed.Status != RunStatusAttached {
		t.Fatalf("RefreshRun() = (%#v, %v), want recovered attachment", refreshed, err)
	}
}

func TestCanvasListRunsRefreshRecoversPersistedSucceededRun(t *testing.T) {
	store := NewMemoryStore()
	assetID := uuid.New()
	service := NewService(store, &fakeGenerator{}, nil)
	created, err := service.Create(t.Context(), CreateRequest{UserID: 9, ProjectID: uuid.New(), Name: "List recover", Template: TemplateImageExploration})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.Generate(t.Context(), GenerateRequest{UserID: 9, CanvasID: created.ID, NodeID: "image-generation", IdempotencyKey: "list-recover"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateRun(t.Context(), RunUpdate{UserID: 9, CanvasID: created.ID, RunID: run.ID, Status: RunStatusSucceeded, ResultAssetIDs: []uuid.UUID{assetID}}); err != nil {
		t.Fatal(err)
	}
	runs, err := service.ListRuns(t.Context(), 9, created.ID, true)
	if err != nil || len(runs) != 1 || runs[0].Status != RunStatusAttached {
		t.Fatalf("ListRuns(refresh) = (%#v, %v), want recovered attachment", runs, err)
	}
}

func TestCanvasGenerateClaimsIdempotencyBeforeSubmitting(t *testing.T) {
	store := NewMemoryStore()
	generator := &fakeGenerator{}
	service := NewService(store, generator, nil)
	created, _ := service.Create(t.Context(), CreateRequest{UserID: 4, ProjectID: uuid.New(), Name: "Idempotent", Template: TemplateImageExploration})
	request := GenerateRequest{UserID: 4, CanvasID: created.ID, NodeID: "image-generation", IdempotencyKey: "same"}
	first, err := service.Generate(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Generate(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || generator.generateCalls != 1 {
		t.Fatalf("runs = %s, %s; generate calls = %d", first.ID, second.ID, generator.generateCalls)
	}
}

type fakeGenerator struct {
	status        TaskStatus
	generateCalls int
}

func (f *fakeGenerator) Estimate(context.Context, GenerationSubmission) (Estimate, error) {
	return Estimate{Points: "1.00000"}, nil
}

func (f *fakeGenerator) Generate(_ context.Context, req GenerationSubmission) (GenerationTask, error) {
	f.generateCalls++
	return GenerationTask{TaskID: uuid.New(), Kind: req.Kind, Status: RunStatusRunning}, nil
}

func (f *fakeGenerator) Status(context.Context, int64, TaskKind, uuid.UUID) (TaskStatus, error) {
	return f.status, nil
}

func (f *fakeGenerator) Cancel(context.Context, int64, TaskKind, uuid.UUID) error { return nil }
