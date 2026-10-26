package entstore

import (
	"errors"
	"testing"

	domaincanvas "github.com/fatballfish/pic-gallery/internal/domain/canvas"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaassetreference"
	canvasservice "github.com/fatballfish/pic-gallery/internal/service/canvas"
)

// Task success precedes derivative processing in both production generators.
func TestCanvasStoreRefreshAttachesReadyOriginal(t *testing.T) {
	for _, kind := range []string{"image", "video"} {
		t.Run(kind, func(t *testing.T) {
			ctx, client := openProjectStoreTestClient(t, "canvas-ready-original-"+kind)
			project, err := client.Project.Create().SetUserID(91).SetName("Default").SetNameKey("default").Save(ctx)
			if err != nil {
				t.Fatal(err)
			}
			asset, err := client.MediaAsset.Create().SetUserID(91).SetProjectID(project.ID).SetName("result").SetNameKey("result").SetMediaType(kind).SetSourceType("generated").SetStatus("ready_original").SetObjectKey("result." + kind).SetMimeType(kind + "/test").SetFileSizeBytes(10).Save(ctx)
			if err != nil {
				t.Fatal(err)
			}
			service := canvasservice.NewService(NewCanvasStore(client), &fakeCanvasStoreGenerator{result: asset.ID}, nil)
			template, nodeID := canvasservice.TemplateImageExploration, "image-generation"
			if kind == "video" {
				template, nodeID = canvasservice.TemplateImageToVideo, "video-generation"
			}
			canvas, err := service.Create(ctx, canvasservice.CreateRequest{UserID: 91, ProjectID: project.ID, Name: "Ready original", Template: template})
			if err != nil {
				t.Fatal(err)
			}
			run, err := service.Generate(ctx, canvasservice.GenerateRequest{UserID: 91, CanvasID: canvas.ID, NodeID: nodeID, IdempotencyKey: "original"})
			if err != nil {
				t.Fatal(err)
			}
			runs, err := service.ListRuns(ctx, 91, canvas.ID, true)
			if err != nil || len(runs) != 1 || runs[0].Status != canvasservice.RunStatusAttached {
				t.Fatalf("refresh before derivatives: %+v %v", runs, err)
			}
			attached, err := service.Get(ctx, 91, canvas.ID)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, node := range attached.Document.Nodes {
				if node.ID == nodeID {
					found = node.AssetID == asset.ID.String()
				}
			}
			if !found {
				t.Fatal("original not attached to integrated source")
			}
			replay, err := service.AttachResults(ctx, canvasservice.AttachResultsRequest{UserID: 91, CanvasID: canvas.ID, RunID: run.ID})
			if err != nil || replay.Status != canvasservice.RunStatusAttached {
				t.Fatalf("replay: %+v %v", replay, err)
			}
			after, err := service.Get(ctx, 91, canvas.ID)
			if err != nil || after.Revision != attached.Revision {
				t.Fatalf("replay modified revision: %+v %v", after, err)
			}
			refs, err := client.MediaAssetReference.Query().Where(mediaassetreference.RefIDEQ(canvas.ID), mediaassetreference.RefTypeEQ("canvas_node"), mediaassetreference.DeletedAtIsNil()).All(ctx)
			if err != nil || len(refs) != 1 || refs[0].AssetID != asset.ID {
				t.Fatalf("references: %+v %v", refs, err)
			}
			for _, state := range []string{"processing", "processing_failed", "deleted"} {
				if err := client.MediaAsset.UpdateOne(asset).SetStatus(state).Exec(ctx); err != nil {
					t.Fatal(err)
				}
				if _, err := service.SaveDocument(ctx, canvasservice.SaveDocumentRequest{UserID: 91, CanvasID: canvas.ID, ExpectedRevision: after.Revision, Document: after.Document}); !errors.Is(err, canvasservice.ErrNotFound) {
					t.Fatalf("unusable status %s accepted: %v", state, err)
				}
			}
			if err := client.MediaAsset.UpdateOne(asset).SetStatus("ready_original").Exec(ctx); err != nil {
				t.Fatal(err)
			}
			wrongKind := after.Document
			wrongKind.Nodes = append([]domaincanvas.Node(nil), after.Document.Nodes...)
			for i := range wrongKind.Nodes {
				if wrongKind.Nodes[i].ID == nodeID {
					if kind == "image" {
						wrongKind.Nodes[i].Type = domaincanvas.NodeTypeVideoV2
					} else {
						wrongKind.Nodes[i].Type = domaincanvas.NodeTypeImageV2
					}
				}
			}
			if _, err := service.SaveDocument(ctx, canvasservice.SaveDocumentRequest{UserID: 91, CanvasID: canvas.ID, ExpectedRevision: after.Revision, Document: wrongKind}); !errors.Is(err, canvasservice.ErrNotFound) {
				t.Fatalf("wrong media kind accepted: %v", err)
			}
			foreign, err := client.MediaAsset.Create().SetUserID(92).SetProjectID(project.ID).SetName("foreign").SetNameKey("foreign").SetMediaType(kind).SetSourceType("generated").SetStatus("ready_original").SetObjectKey("foreign." + kind).SetMimeType(kind + "/test").SetFileSizeBytes(10).Save(ctx)
			if err != nil {
				t.Fatal(err)
			}
			invalid := after.Document
			invalid.Nodes = append([]domaincanvas.Node(nil), after.Document.Nodes...)
			for i := range invalid.Nodes {
				if invalid.Nodes[i].ID == nodeID {
					invalid.Nodes[i].AssetID = foreign.ID.String()
				}
			}
			if _, err := service.SaveDocument(ctx, canvasservice.SaveDocumentRequest{UserID: 91, CanvasID: canvas.ID, ExpectedRevision: after.Revision, Document: invalid}); !errors.Is(err, canvasservice.ErrNotFound) {
				t.Fatalf("foreign original must remain not found: %v", err)
			}
		})
	}
}
