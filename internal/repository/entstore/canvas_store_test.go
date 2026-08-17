package entstore

import (
	"context"
	"errors"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"

	domaincanvas "github.com/fatballfish/pic-gallery/internal/domain/canvas"
	domainproject "github.com/fatballfish/pic-gallery/internal/domain/project"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaassetreference"
	canvasservice "github.com/fatballfish/pic-gallery/internal/service/canvas"
	projectservice "github.com/fatballfish/pic-gallery/internal/service/project"
)

func TestCanvasStorePersistsRevisionReferencesAndCAS(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, "file:canvas-store?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	projectID := uuid.New()
	if _, err := client.Project.Create().SetID(projectID).SetUserID(12).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	assetID := uuid.New()
	if _, err := client.MediaAsset.Create().SetID(assetID).SetUserID(12).SetProjectID(projectID).SetName("a.png").SetNameKey("a.png").SetMediaType("image").SetSourceType("upload").SetStatus("ready").SetStorageDriver("s3").SetObjectKey("a.png").SetMimeType("image/png").SetFileSizeBytes(1).Save(t.Context()); err != nil {
		t.Fatal(err)
	}

	store := NewCanvasStore(client)
	service := canvasservice.NewService(store, nil, nil)
	created, err := service.Create(t.Context(), canvasservice.CreateRequest{UserID: 12, ProjectID: projectID, Name: "Canvas", Template: canvasservice.TemplateBlank})
	if err != nil {
		t.Fatal(err)
	}
	document := domaincanvas.DocumentV1{SchemaVersion: 1, Nodes: []domaincanvas.Node{{ID: "image", Type: domaincanvas.NodeTypeImage, AssetID: assetID.String(), Size: domaincanvas.Size{Width: 220, Height: 160}}}}
	saved, err := service.SaveDocument(t.Context(), canvasservice.SaveDocumentRequest{UserID: 12, CanvasID: created.ID, ExpectedRevision: 1, Document: document})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision != 2 {
		t.Fatalf("revision = %d", saved.Revision)
	}
	refs, err := client.MediaAssetReference.Query().Where(mediaassetreference.RefTypeEQ("canvas_node"), mediaassetreference.RefIDEQ(created.ID), mediaassetreference.DeletedAtIsNil()).All(t.Context())
	if err != nil || len(refs) != 1 || refs[0].AssetID != assetID {
		t.Fatalf("references = (%#v, %v)", refs, err)
	}
	if count, err := client.CreativeCanvasRevision.Query().Count(t.Context()); err != nil || count != 1 {
		t.Fatalf("revision snapshots = (%d, %v)", count, err)
	}

	_, err = service.SaveDocument(t.Context(), canvasservice.SaveDocumentRequest{UserID: 12, CanvasID: created.ID, ExpectedRevision: 1, Document: document})
	var conflict *canvasservice.RevisionConflictError
	if !errors.As(err, &conflict) || conflict.RemoteRevision != 2 {
		t.Fatalf("stale save error = %#v", err)
	}
}

func TestCanvasStoreAttachAndTransferAreAtomic(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, "file:canvas-store-attach?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	projectOne, projectTwo := uuid.New(), uuid.New()
	for index, id := range []uuid.UUID{projectOne, projectTwo} {
		if _, err := client.Project.Create().SetID(id).SetUserID(22).SetName(string(rune('A' + index))).SetNameKey(string(rune('a' + index))).Save(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	resultAssetID := uuid.New()
	if _, err := client.MediaAsset.Create().SetID(resultAssetID).SetUserID(22).SetProjectID(projectOne).SetName("result.png").SetNameKey("result.png").SetMediaType("image").SetSourceType("generation").SetStatus("ready").SetStorageDriver("s3").SetObjectKey("result.png").SetMimeType("image/png").SetFileSizeBytes(1).Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	store := NewCanvasStore(client)
	service := canvasservice.NewService(store, &fakeCanvasStoreGenerator{result: resultAssetID}, nil)
	created, err := service.Create(t.Context(), canvasservice.CreateRequest{UserID: 22, ProjectID: projectOne, Name: "Run", Template: canvasservice.TemplateImageExploration})
	if err != nil {
		t.Fatal(err)
	}
	created.Document.Nodes = append(created.Document.Nodes, domaincanvas.Node{ID: "result-slot", Type: domaincanvas.NodeTypeImage, Position: domaincanvas.Point{X: 800, Y: 80}, Size: domaincanvas.Size{Width: 220, Height: 160}})
	created.Document.Edges = append(created.Document.Edges, domaincanvas.Edge{ID: "result-slot-edge", Source: "image-generation", Target: "result-slot", InputRole: domaincanvas.InputRoleResult})
	created, err = service.SaveDocument(t.Context(), canvasservice.SaveDocumentRequest{UserID: 22, CanvasID: created.ID, ExpectedRevision: created.Revision, Document: created.Document})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.Generate(t.Context(), canvasservice.GenerateRequest{UserID: 22, CanvasID: created.ID, NodeID: "image-generation", IdempotencyKey: "one"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.TransferProject(t.Context(), canvasservice.TransferProjectRequest{UserID: 22, CanvasID: created.ID, TargetProjectID: projectTwo, ExpectedMetadataVersion: 1}); !errors.Is(err, canvasservice.ErrCanvasBusy) {
		t.Fatalf("busy transfer = %v", err)
	}
	attached, err := service.AttachResults(t.Context(), canvasservice.AttachResultsRequest{UserID: 22, CanvasID: created.ID, RunID: run.ID})
	if err != nil || attached.Status != canvasservice.RunStatusAttached {
		t.Fatalf("attach = (%#v,%v)", attached, err)
	}
	afterAttach, err := service.Get(t.Context(), 22, created.ID)
	if err != nil || len(afterAttach.Document.Nodes) != 3 || afterAttach.Document.Nodes[2].ID != "result-slot" || afterAttach.Document.Nodes[2].AssetID != resultAssetID.String() {
		t.Fatalf("persisted result slot = (%#v, %v)", afterAttach.Document, err)
	}
	refs, err := client.MediaAssetReference.Query().Where(mediaassetreference.RefTypeEQ("canvas_node"), mediaassetreference.RefIDEQ(created.ID), mediaassetreference.DeletedAtIsNil()).All(t.Context())
	if err != nil || len(refs) != 1 || refs[0].AssetID != resultAssetID {
		t.Fatalf("result slot references = (%#v, %v)", refs, err)
	}
	moved, err := service.TransferProject(t.Context(), canvasservice.TransferProjectRequest{UserID: 22, CanvasID: created.ID, TargetProjectID: projectTwo, ExpectedMetadataVersion: 1})
	if err != nil || moved.ProjectID != projectTwo {
		t.Fatalf("transfer = (%#v,%v)", moved, err)
	}
}

func TestCanvasStoreRecoversUnplacedResultsAtRequestedPosition(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, "file:canvas-store-unplaced?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	projectID := uuid.New()
	if _, err := client.Project.Create().SetID(projectID).SetUserID(23).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	resultAssetID := uuid.New()
	if _, err := client.MediaAsset.Create().SetID(resultAssetID).SetUserID(23).SetProjectID(projectID).SetName("result.png").SetNameKey("result.png").SetMediaType("image").SetSourceType("generation").SetStatus("ready").SetStorageDriver("s3").SetObjectKey("unplaced-result.png").SetMimeType("image/png").SetFileSizeBytes(1).Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	service := canvasservice.NewService(NewCanvasStore(client), &fakeCanvasStoreGenerator{result: resultAssetID}, nil)
	created, err := service.Create(t.Context(), canvasservice.CreateRequest{UserID: 23, ProjectID: projectID, Name: "Unplaced", Template: canvasservice.TemplateImageExploration})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.Generate(t.Context(), canvasservice.GenerateRequest{UserID: 23, CanvasID: created.ID, NodeID: "image-generation", IdempotencyKey: "recover"})
	if err != nil {
		t.Fatal(err)
	}
	created.Document.Nodes = created.Document.Nodes[:1]
	created.Document.Edges = nil
	if _, err := service.SaveDocument(t.Context(), canvasservice.SaveDocumentRequest{UserID: 23, CanvasID: created.ID, ExpectedRevision: created.Revision, Document: created.Document}); err != nil {
		t.Fatal(err)
	}
	if unplaced, err := service.RefreshRun(t.Context(), 23, created.ID, run.ID); err != nil || unplaced.Status != canvasservice.RunStatusUnplaced {
		t.Fatalf("mark unplaced = (%#v, %v)", unplaced, err)
	}
	recovered, err := service.AttachResults(t.Context(), canvasservice.AttachResultsRequest{UserID: 23, CanvasID: created.ID, RunID: run.ID, RecoveryPosition: &domaincanvas.Point{X: 500, Y: 400}})
	if err != nil || recovered.Status != canvasservice.RunStatusAttached {
		t.Fatalf("recover unplaced = (%#v, %v)", recovered, err)
	}
	after, err := service.Get(t.Context(), 23, created.ID)
	if err != nil || len(after.Document.Nodes) != 2 || len(after.Document.Edges) != 0 || after.Document.Nodes[1].Position.X != 340 || after.Document.Nodes[1].Position.Y != 280 {
		t.Fatalf("recovered canvas = (%#v, %v)", after.Document, err)
	}
}

func TestProjectDeleteRejectsActiveCanvasRunAndTransfersIdleCanvas(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, "file:project-canvas-delete?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	source, err := client.Project.Create().SetUserID(71).SetName("Source").SetNameKey("source").Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	target, err := client.Project.Create().SetUserID(71).SetName("Target").SetNameKey("target").Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	canvasStore := NewCanvasStore(client)
	canvasService := canvasservice.NewService(canvasStore, &fakeCanvasStoreGenerator{}, nil)
	created, err := canvasService.Create(t.Context(), canvasservice.CreateRequest{UserID: 71, ProjectID: source.ID, Name: "Canvas", Template: canvasservice.TemplateImageExploration})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := canvasService.Generate(t.Context(), canvasservice.GenerateRequest{UserID: 71, CanvasID: created.ID, NodeID: "image-generation", IdempotencyKey: "active"}); err != nil {
		t.Fatal(err)
	}
	projectStore := NewProjectStore(client)
	_, err = projectStore.Delete(t.Context(), 71, source.ID.String(), domainproject.DeleteRequest{TargetProjectID: target.ID.String(), ExpectedVersion: 1})
	if !errors.Is(err, projectservice.ErrCanvasBusy) {
		t.Fatalf("active canvas project delete error = %v", err)
	}
	runs, err := canvasService.ListRuns(t.Context(), 71, created.ID, false)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs=(%#v,%v)", runs, err)
	}
	if _, err := canvasStore.UpdateRun(t.Context(), canvasservice.RunUpdate{UserID: 71, CanvasID: created.ID, RunID: runs[0].ID, Status: canvasservice.RunStatusFailed}); err != nil {
		t.Fatal(err)
	}
	result, err := projectStore.Delete(t.Context(), 71, source.ID.String(), domainproject.DeleteRequest{TargetProjectID: target.ID.String(), ExpectedVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Transferred.Canvases != 1 {
		t.Fatalf("transferred = %#v", result.Transferred)
	}
	moved, err := canvasService.Get(t.Context(), 71, created.ID)
	if err != nil || moved.ProjectID != target.ID {
		t.Fatalf("moved canvas=(%#v,%v)", moved, err)
	}
}

type fakeCanvasStoreGenerator struct{ result uuid.UUID }

func (f *fakeCanvasStoreGenerator) Estimate(context.Context, canvasservice.GenerationSubmission) (canvasservice.Estimate, error) {
	return canvasservice.Estimate{}, nil
}
func (f *fakeCanvasStoreGenerator) Generate(context.Context, canvasservice.GenerationSubmission) (canvasservice.GenerationTask, error) {
	return canvasservice.GenerationTask{TaskID: uuid.New(), Kind: canvasservice.TaskKindImage, Status: canvasservice.RunStatusRunning}, nil
}
func (f *fakeCanvasStoreGenerator) Status(context.Context, int64, canvasservice.TaskKind, uuid.UUID) (canvasservice.TaskStatus, error) {
	return canvasservice.TaskStatus{Status: canvasservice.RunStatusSucceeded, ResultAssetIDs: []uuid.UUID{f.result}}, nil
}
func (f *fakeCanvasStoreGenerator) Cancel(context.Context, int64, canvasservice.TaskKind, uuid.UUID) error {
	return nil
}
