package canvas

import (
	"testing"

	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/google/uuid"
)

func TestRunLayersMetadataResolution(t *testing.T) {
	imageID, assetID, taskID := uuid.New(), uuid.New(), uuid.New()
	for _, tc := range []struct {
		name      string
		results   []provider.ImageResult
		ids       []uuid.UUID
		wantError bool
		wantAsset uuid.UUID
	}{
		{name: "ordinary results retain legacy layout", results: []provider.ImageResult{{ID: imageID.String()}}, ids: []uuid.UUID{imageID}},
		{name: "image ID resolves durable media asset", results: []provider.ImageResult{{ID: imageID.String(), MediaAssetID: assetID.String(), Width: 1000, Height: 500, Layer: &nativeimage.LayerMetadata{IsBase: true}}}, ids: []uuid.UUID{imageID}, wantAsset: assetID},
		{name: "missing metadata rejects layered batch", results: []provider.ImageResult{{ID: imageID.String(), Width: 1000, Height: 500, Layer: &nativeimage.LayerMetadata{IsBase: true}}}, ids: []uuid.UUID{imageID, uuid.New()}, wantError: true},
		{name: "missing base rejects layered batch", results: []provider.ImageResult{{ID: imageID.String(), Width: 200, Height: 100, Layer: &nativeimage.LayerMetadata{ZIndex: 1}}}, ids: []uuid.UUID{imageID}, wantError: true},
		{name: "duplicate result identities collapse", results: []provider.ImageResult{{ID: imageID.String(), MediaAssetID: assetID.String(), Width: 1000, Height: 500, Layer: &nativeimage.LayerMetadata{IsBase: true}}}, ids: []uuid.UUID{imageID, assetID}, wantAsset: assetID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			port := &layerTaskPort{owner: 9, taskID: taskID, task: domainimagetask.Task{Results: tc.results}}
			service := NewService(NewMemoryStore(), NewTaskGenerator(port, nil, nil), nil)
			layers, err := service.runLayers(t.Context(), Run{UserID: 9, TaskKind: TaskKindImage, TaskID: taskID, ResultAssetIDs: tc.ids})
			if (err != nil) != tc.wantError {
				t.Fatalf("runLayers error=%v", err)
			}
			if tc.wantAsset != uuid.Nil && (len(layers) != 1 || layers[0].assetID != tc.wantAsset) {
				t.Fatalf("layers=%+v", layers)
			}
			if tc.wantAsset == uuid.Nil && !tc.wantError && len(layers) != 0 {
				t.Fatalf("ordinary results should keep legacy layout: %+v", layers)
			}
		})
	}
}
