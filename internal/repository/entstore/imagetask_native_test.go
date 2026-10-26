package entstore

import (
	image "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"github.com/fatballfish/pic-gallery/internal/provider"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/google/uuid"
	"reflect"
	"testing"
	"time"
)

func TestNativeTaskSnapshotMigrationAndLayerMetadata(t *testing.T) {
	no := false
	task := image.Task{Operation: nativeimage.OperationLayerDecomposition, NativeOptions: &nativeimage.Options{Watermark: &no}, BillingDiagnostic: "reservation exceeded", Results: []provider.ImageResult{{ID: uuid.NewString(), Layer: &nativeimage.LayerMetadata{ZIndex: 1, Name: "transparent"}}}}
	started := time.Now().UTC()
	task.UpstreamStartedAt = &started
	snapshot, err := buildRoutingSnapshot(task)
	if err != nil {
		t.Fatal(err)
	}
	trace, err := buildProviderTrace(task)
	if err != nil {
		t.Fatal(err)
	}
	resultID := uuid.MustParse(task.Results[0].ID)
	entity := &repoent.ImageTask{ID: uuid.New(), RoutingSnapshot: snapshot, ProviderTrace: trace}
	loaded, err := mapImageTaskEntity(entity, []*repoent.ImageResult{{ID: resultID, Width: 1024, Height: 1024}})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.UpstreamStartedAt == nil || !loaded.UpstreamStartedAt.Equal(started) {
		t.Fatal("upstream submission marker lost on database roundtrip")
	}
	if loaded.Operation != task.Operation || !reflect.DeepEqual(loaded.NativeOptions, task.NativeOptions) || loaded.BillingDiagnostic != task.BillingDiagnostic {
		t.Fatalf("native snapshot not restored: %#v", loaded)
	}
	if len(loaded.Results) != 1 || !reflect.DeepEqual(loaded.Results[0].Layer, task.Results[0].Layer) {
		t.Fatalf("layer metadata lost: %#v", loaded.Results)
	}
	loaded, err = mapImageTaskEntity(entity, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Results) != 0 {
		t.Fatal("trace resurrected deleted image")
	}
	legacy, err := mapImageTaskEntity(&repoent.ImageTask{ID: uuid.New()}, nil)
	if err != nil || legacy.NativeOptions != nil || legacy.Operation != "" {
		t.Fatalf("legacy read migration failed: %#v %v", legacy, err)
	}
}
