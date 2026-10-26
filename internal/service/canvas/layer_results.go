package canvas

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	domaincanvas "github.com/fatballfish/pic-gallery/internal/domain/canvas"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/google/uuid"
)

// RunResultMetadata is optional so existing generators keep their ordinary layout.
// Metadata is read from the owned task on attachment, including after a restart.
type RunResultMetadata interface {
	CanvasImageResults(context.Context, int64, uuid.UUID) ([]provider.ImageResult, error)
}

func (g *TaskGenerator) CanvasImageResults(ctx context.Context, userID int64, taskID uuid.UUID) ([]provider.ImageResult, error) {
	if g == nil || g.images == nil {
		return nil, fmt.Errorf("canvas image generation is unavailable")
	}
	task, err := g.images.GetByID(ctx, userID, taskID.String())
	if err != nil {
		return nil, fmt.Errorf("read canvas image result metadata: %w", err)
	}
	return task.Results, nil
}

type layerResult struct {
	assetID uuid.UUID
	result  provider.ImageResult
}

func (s *Service) runLayers(ctx context.Context, run Run) ([]layerResult, error) {
	reader, ok := s.generator.(RunResultMetadata)
	if !ok || run.TaskKind != TaskKindImage {
		return nil, nil
	}
	results, err := reader.CanvasImageResults(ctx, run.UserID, run.TaskID)
	if err != nil {
		return nil, err
	}
	hasLayers := false
	byID := make(map[string]provider.ImageResult, len(results)*2)
	for _, result := range results {
		hasLayers = hasLayers || result.Layer != nil
		byID[result.ID] = result
		if result.MediaAssetID != "" {
			byID[result.MediaAssetID] = result
		}
	}
	if !hasLayers {
		return nil, nil
	}
	layers := make([]layerResult, 0, len(run.ResultAssetIDs))
	seen := make(map[uuid.UUID]bool)
	baseCount := 0
	for _, id := range run.ResultAssetIDs {
		result, exists := byID[id.String()]
		if !exists || result.Layer == nil || result.Width <= 0 || result.Height <= 0 {
			return nil, fmt.Errorf("missing canvas layer metadata for result %s", id)
		}
		// Historical runs can contain generated Image IDs; canvas references use
		// the durable MediaAsset identity whenever the task projection provides it.
		if result.MediaAssetID != "" {
			assetID, err := uuid.Parse(result.MediaAssetID)
			if err != nil {
				return nil, fmt.Errorf("invalid canvas layer media asset id: %w", err)
			}
			id = assetID
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		if result.Layer.IsBase {
			baseCount++
		} else if b := result.Layer.BoundingBox; b != nil && (b.Absolute[2] <= b.Absolute[0] || b.Absolute[3] <= b.Absolute[1]) {
			return nil, fmt.Errorf("invalid canvas layer bounding box for result %s", id)
		}
		layers = append(layers, layerResult{assetID: id, result: result})
	}
	if baseCount != 1 {
		return nil, fmt.Errorf("canvas layers require exactly one base image")
	}
	sort.SliceStable(layers, func(i, j int) bool {
		a, b := layers[i].result.Layer, layers[j].result.Layer
		if a.IsBase != b.IsBase {
			return a.IsBase
		}
		return a.ZIndex < b.ZIndex
	})
	return layers, nil
}

// layerPlacement uses one shared origin and scale for the complete composition.
// Node order is paint order; raw dimensions and metadata survive document saves.
func layerPlacement(run Run, layers []layerResult, origin domaincanvas.Point, width float64) []domaincanvas.Node {
	base := layers[0].result
	scale := width / float64(base.Width)
	nodes := make([]domaincanvas.Node, 0, len(layers))
	for _, item := range layers {
		result := item.result
		bbox := [4]int{0, 0, result.Width, result.Height}
		if !result.Layer.IsBase && result.Layer.BoundingBox != nil {
			bbox = result.Layer.BoundingBox.Absolute
		}
		id := domaincanvas.StableResultNodeID(run.ID.String(), item.assetID.String())
		name := strings.TrimSpace(result.Layer.Name)
		if name == "" {
			name = id
		}
		payload, _ := json.Marshal(struct {
			Name   string                     `json:"name"`
			Layer  *nativeimage.LayerMetadata `json:"layer"`
			RunID  string                     `json:"run_id"`
			Width  int                        `json:"width"`
			Height int                        `json:"height"`
			Format string                     `json:"format,omitempty"`
		}{name, result.Layer, run.ID.String(), result.Width, result.Height, result.Format})
		nodes = append(nodes, domaincanvas.Node{ID: id, Type: domaincanvas.NodeTypeImageV2, AssetID: item.assetID.String(), Position: domaincanvas.Point{X: origin.X + float64(bbox[0])*scale, Y: origin.Y + float64(bbox[1])*scale}, Size: domaincanvas.Size{Width: float64(bbox[2]-bbox[0]) * scale, Height: float64(bbox[3]-bbox[1]) * scale}, Payload: payload})
	}
	return nodes
}
