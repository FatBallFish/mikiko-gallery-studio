package canvas

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	domaincanvas "github.com/fatballfish/pic-gallery/internal/domain/canvas"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/google/uuid"
)

type layerTaskPort struct {
	task   domainimagetask.Task
	owner  int64
	taskID uuid.UUID
}

func (p *layerTaskPort) CreateTask(context.Context, domainimagetask.CreateRequest) (domainimagetask.Task, error) {
	return p.task, nil
}
func (p *layerTaskPort) GetByID(_ context.Context, userID int64, taskID string) (domainimagetask.Task, error) {
	if userID != p.owner || taskID != p.taskID.String() {
		return domainimagetask.Task{}, ErrNotFound
	}
	return p.task, nil
}

func TestLayerRunAttachmentAndRecovery(t *testing.T) {
	for _, recover := range []bool{false, true} {
		for _, mediaID := range []bool{false, true} {
			t.Run(map[bool]string{false: "attach", true: "recover"}[recover]+map[bool]string{false: "/image-id", true: "/media-id"}[mediaID], func(t *testing.T) {
				store := NewMemoryStore()
				taskID := uuid.New()
				baseID, lowerID, upperID := uuid.New(), uuid.New(), uuid.New()
				results := []provider.ImageResult{
					{ID: upperID.String(), Width: 200, Height: 100, Format: "png", Layer: &nativeimage.LayerMetadata{ZIndex: 7, Name: "upper", BoundingBox: &nativeimage.BoundingBox{Absolute: [4]int{100, 200, 300, 300}, Normalized: [4]int{100, 200, 300, 300}}}},
					{ID: baseID.String(), Width: 1000, Height: 500, Format: "png", Layer: &nativeimage.LayerMetadata{ZIndex: 0, IsBase: true, Name: "base", BoundingBox: &nativeimage.BoundingBox{Absolute: [4]int{40, 50, 60, 70}}}},
					{ID: lowerID.String(), Width: 400, Height: 200, Format: "png", Layer: &nativeimage.LayerMetadata{ZIndex: 2, Name: "lower", Description: "transparent", BoundingBox: &nativeimage.BoundingBox{Absolute: [4]int{50, 60, 450, 260}}}},
				}
				if mediaID {
					for i := range results {
						results[i].MediaAssetID = results[i].ID
						results[i].ID = uuid.NewString()
					}
				}
				port := &layerTaskPort{owner: 9, taskID: taskID, task: domainimagetask.Task{Results: results}}
				service := NewService(store, NewTaskGenerator(port, nil, nil), nil)
				source := domaincanvas.Node{ID: "source", Type: domaincanvas.NodeTypeImageV2, Position: domaincanvas.Point{X: 10, Y: 20}, Size: domaincanvas.Size{Width: 500, Height: 400}}
				doc := domaincanvas.DocumentV1{SchemaVersion: 1, Nodes: []domaincanvas.Node{source}}
				canvas, err := service.Create(t.Context(), CreateRequest{UserID: 9, ProjectID: uuid.New(), Document: &doc})
				if err != nil {
					t.Fatal(err)
				}
				run := Run{ID: uuid.New(), CanvasID: canvas.ID, UserID: 9, NodeID: source.ID, TaskKind: TaskKindImage, TaskID: taskID, Status: RunStatusSucceeded, SubmittedRevision: canvas.Revision, NodeSnapshot: mustJSON(t, map[string]any{"node": source}), ResultAssetIDs: []uuid.UUID{upperID, baseID, lowerID}}
				if _, _, err := store.CreateRun(t.Context(), run); err != nil {
					t.Fatal(err)
				}
				if recover {
					doc.Nodes = nil
					if _, err := service.SaveDocument(t.Context(), SaveDocumentRequest{UserID: 9, CanvasID: canvas.ID, ExpectedRevision: canvas.Revision, Document: doc}); err != nil {
						t.Fatal(err)
					}
				}
				req := AttachResultsRequest{UserID: 9, CanvasID: canvas.ID, RunID: run.ID}
				if _, err := service.AttachResults(t.Context(), AttachResultsRequest{UserID: 10, CanvasID: canvas.ID, RunID: run.ID}); !errors.Is(err, ErrNotFound) {
					t.Fatalf("foreign attach: %v", err)
				}
				if recover {
					unplaced, err := service.AttachResults(t.Context(), req)
					if err != nil || unplaced.Status != RunStatusUnplaced {
						t.Fatalf("unplaced: %+v %v", unplaced, err)
					}
					req.RecoveryPosition = &domaincanvas.Point{X: 1000, Y: 1000}
				}
				attached, err := service.AttachResults(t.Context(), req)
				if err != nil || attached.Status != RunStatusAttached {
					t.Fatalf("attach: %+v %v", attached, err)
				}
				saved, err := service.Get(t.Context(), 9, canvas.ID)
				if err != nil {
					t.Fatal(err)
				}
				nodes := saved.Document.Nodes
				width, origin := 500.0, domaincanvas.Point{X: 590, Y: 20}
				if recover {
					width, origin = 320, domaincanvas.Point{X: 840, Y: 920}
				} else {
					if nodes[0].AssetID != "" {
						t.Fatal("layer attachment filled source slot")
					}
					nodes = nodes[1:]
				}
				if len(nodes) != 3 || len(saved.Document.Edges) != 0 {
					t.Fatalf("document: %+v", saved.Document)
				}
				ids := []uuid.UUID{baseID, lowerID, upperID}
				for i, node := range nodes {
					if node.AssetID != ids[i].String() || node.Type != domaincanvas.NodeTypeImageV2 {
						t.Fatalf("layer order/type: %+v", nodes)
					}
					var payload struct {
						Layer  nativeimage.LayerMetadata `json:"layer"`
						Width  int                       `json:"width"`
						Height int                       `json:"height"`
						RunID  string                    `json:"run_id"`
						Format string                    `json:"format"`
					}
					if err := json.Unmarshal(node.Payload, &payload); err != nil {
						t.Fatal(err)
					}
					if payload.RunID != run.ID.String() || payload.Format != "png" || payload.Width <= 0 || payload.Height <= 0 {
						t.Fatalf("metadata lost: %s", node.Payload)
					}
					if i == 0 && (node.Position != origin || node.Size != (domaincanvas.Size{Width: width, Height: width / 2})) {
						t.Fatalf("base must use full image dimensions: %+v", node)
					}
					if i == 1 && (node.Position.X != origin.X+50*width/1000 || node.Position.Y != origin.Y+60*width/1000 || node.Size.Width != 400*width/1000 || payload.Layer.ZIndex != 2) {
						t.Fatalf("lower bbox: %+v", node)
					}
					if i == 2 && (node.Position.X != origin.X+100*width/1000 || node.Position.Y != origin.Y+200*width/1000 || node.Size.Height != 100*width/1000 || payload.Layer.ZIndex != 7) {
						t.Fatalf("upper bbox: %+v", node)
					}
				}
				if _, err := service.SaveDocument(t.Context(), SaveDocumentRequest{UserID: 9, CanvasID: canvas.ID, ExpectedRevision: saved.Revision, Document: saved.Document}); err != nil {
					t.Fatalf("roundtrip: %v", err)
				}
				if _, err := service.AttachResults(t.Context(), req); err != nil {
					t.Fatal(err)
				}
				replay, _ := service.Get(t.Context(), 9, canvas.ID)
				if len(replay.Document.Nodes) != len(saved.Document.Nodes) || replay.Revision != saved.Revision+1 {
					t.Fatal("repeated attachment mutated document")
				}
			})
		}
	}
}
