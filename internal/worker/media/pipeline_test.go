package media

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
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

func TestPipelineUsesRuntimePolicyForProbeAndDerivativePlan(t *testing.T) {
	root := t.TempDir()
	backend := storage.NewLocalBackend(root)
	if err := backend.Put(t.Context(), "media/original/7/asset.mp4", "video/mp4", []byte("original")); err != nil {
		t.Fatal(err)
	}
	derivatives := &policyAwareDerivatives{}
	policy := domainmedia.DefaultPolicy()
	policy.VideoMaxDurationMS = 4_000
	policy.VideoPosterEnabled = false
	pipeline := NewPipeline(storage.NewStaticRouter(backend), fakeProbe{result: domainmedia.ProbeResult{
		MediaType: domainmedia.MediaTypeVideo, Format: "mp4", Container: "mp4", VideoCodec: "h264", SizeBytes: 8, DurationMS: 5_000,
	}}, derivatives, PipelineOptions{TempDir: t.TempDir(), Policy: domainmedia.DefaultPolicy(), PolicyResolver: func(context.Context) (domainmedia.Policy, error) { return policy, nil }})
	item := WorkItem{AssetID: "asset-id", UserID: 7, MediaType: "video", MIMEType: "video/mp4", SizeBytes: 8, StorageDriver: "local", ObjectKey: "media/original/7/asset.mp4"}
	if _, err := pipeline.Process(t.Context(), item); err == nil || !strings.Contains(err.Error(), "duration") {
		t.Fatalf("runtime duration limit error = %v", err)
	}
	policy.VideoMaxDurationMS = 6_000
	if _, err := pipeline.Process(t.Context(), item); err != nil {
		t.Fatal(err)
	}
	if derivatives.policy.VideoPosterEnabled {
		t.Fatal("runtime derivative policy was not forwarded")
	}
}

type policyAwareDerivatives struct{ policy domainmedia.Policy }

func (processor *policyAwareDerivatives) Generate(context.Context, domainmedia.MediaType, string, string) ([]DerivativeOutput, error) {
	return nil, errors.New("legacy derivative generator should not be used when a runtime policy is available")
}

func (processor *policyAwareDerivatives) GenerateWithPolicy(_ context.Context, _ domainmedia.MediaType, _, _ string, policy domainmedia.Policy) ([]DerivativeOutput, error) {
	processor.policy = policy
	return nil, nil
}

func TestPipelineRemovesEarlierDerivativeWhenLaterUploadFails(t *testing.T) {
	root := t.TempDir()
	backend := &failingDerivativeBackend{LocalBackend: storage.NewLocalBackend(root), failAt: 2}
	if err := backend.Put(t.Context(), "media/original/7/asset.mp4", "video/mp4", []byte("original")); err != nil {
		t.Fatal(err)
	}
	pipeline := NewPipeline(storage.NewStaticRouter(backend), fakeProbe{result: domainmedia.ProbeResult{
		MediaType: domainmedia.MediaTypeVideo, Format: "mp4", Container: "mp4", VideoCodec: "h264", SizeBytes: 8,
	}}, twoFakeDerivatives{}, PipelineOptions{TempDir: t.TempDir(), Policy: domainmedia.DefaultPolicy()})

	if _, err := pipeline.Process(t.Context(), WorkItem{AssetID: "asset-id", UserID: 7, MediaType: "video", MIMEType: "video/mp4", SizeBytes: 8, StorageDriver: "local", ObjectKey: "media/original/7/asset.mp4"}); err == nil {
		t.Fatal("expected second derivative upload failure")
	}
	firstKey := "media/derivatives/7/asset-id/poster-v1.jpg"
	if _, err := backend.Get(t.Context(), firstKey); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("earlier derivative was not rolled back: %v", err)
	}
}

type twoFakeDerivatives struct{}

func (twoFakeDerivatives) Generate(_ context.Context, _ domainmedia.MediaType, _ string, outputDir string) ([]DerivativeOutput, error) {
	outputs := []DerivativeOutput{
		{Kind: domainmedia.DerivativePoster, TransformVersion: 1, Path: filepath.Join(outputDir, "poster.jpg")},
		{Kind: domainmedia.DerivativeProxy, TransformVersion: 1, Path: filepath.Join(outputDir, "proxy.mp4")},
	}
	for _, output := range outputs {
		if err := os.WriteFile(output.Path, []byte("fixture"), 0o600); err != nil {
			return nil, err
		}
	}
	return outputs, nil
}

type failingDerivativeBackend struct {
	*storage.LocalBackend
	puts   int
	failAt int
}

func (backend *failingDerivativeBackend) PutReader(ctx context.Context, objectKey, contentType string, reader io.Reader, size int64) error {
	backend.puts++
	if backend.puts == backend.failAt {
		return errors.New("injected derivative upload failure")
	}
	return backend.LocalBackend.PutReader(ctx, objectKey, contentType, reader, size)
}
