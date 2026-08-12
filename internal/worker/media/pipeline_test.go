package media

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
	"github.com/fatballfish/pic-gallery/internal/storage"
)

type fakeProbe struct{ result domainmedia.ProbeResult }

func (probe fakeProbe) Inspect(context.Context, string, int64, domainmedia.MediaType) (domainmedia.ProbeResult, error) {
	return probe.result, nil
}

type fakeDerivatives struct{ sawInput string }

func (processor *fakeDerivatives) Generate(_ context.Context, _ domainmedia.MediaType, input, outputDir string) ([]DerivativeOutput, error) {
	processor.sawInput = input
	path := filepath.Join(outputDir, "proxy.mp4")
	if err := os.WriteFile(path, []byte("proxy"), 0o600); err != nil {
		return nil, err
	}
	return []DerivativeOutput{{Kind: domainmedia.DerivativeProxy, TransformVersion: 1, Path: path}}, nil
}

func TestPipelineStreamsOriginalGeneratesStableDerivativesAndCleansTemp(t *testing.T) {
	root, temp := t.TempDir(), t.TempDir()
	backend := storage.NewLocalBackend(root)
	if err := backend.Put(t.Context(), "media/original/7/asset.mp4", "video/mp4", []byte("original")); err != nil {
		t.Fatal(err)
	}
	derivatives := &fakeDerivatives{}
	pipeline := NewPipeline(storage.NewStaticRouter(backend), fakeProbe{result: domainmedia.ProbeResult{
		MediaType: domainmedia.MediaTypeVideo, Format: "mp4", Container: "mp4", VideoCodec: "h264", SizeBytes: 8,
	}}, derivatives, PipelineOptions{TempDir: temp, Policy: domainmedia.DefaultPolicy()})

	result, err := pipeline.Process(t.Context(), WorkItem{AssetID: "asset-id", UserID: 7, MediaType: "video", MIMEType: "video/mp4", SizeBytes: 8, StorageDriver: "local", ObjectKey: "media/original/7/asset.mp4"})
	if err != nil || len(result.Derivatives) != 1 || result.Derivatives[0].ObjectKey != "media/derivatives/7/asset-id/proxy-v1.mp4" {
		t.Fatalf("Process result=%#v err=%v", result, err)
	}
	if _, err := backend.Get(t.Context(), result.Derivatives[0].ObjectKey); err != nil {
		t.Fatalf("stored derivative: %v", err)
	}
	if _, err := os.Stat(derivatives.sawInput); !os.IsNotExist(err) {
		t.Fatalf("temporary original still exists: %v", err)
	}
	entries, err := os.ReadDir(temp)
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary root entries=%v err=%v", entries, err)
	}
}

func TestPipelineRejectsObjectLargerThanHardLimit(t *testing.T) {
	pipeline := NewPipeline(storage.NewStaticRouter(storage.NewLocalBackend(t.TempDir())), fakeProbe{}, &fakeDerivatives{}, PipelineOptions{TempDir: t.TempDir()})
	_, err := pipeline.Process(t.Context(), WorkItem{AssetID: "large", UserID: 7, MediaType: "video", SizeBytes: domainmedia.SingleFileHardMaxBytes + 1, ObjectKey: "media/original/7/large.mp4"})
	if err == nil {
		t.Fatal("expected hard size limit rejection")
	}
}
