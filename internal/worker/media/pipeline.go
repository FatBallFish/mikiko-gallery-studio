package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
	"github.com/fatballfish/pic-gallery/internal/storage"
)

type ProbeInspector interface {
	Inspect(context.Context, string, int64, domainmedia.MediaType) (domainmedia.ProbeResult, error)
}

type DerivativeOutput struct {
	Kind             domainmedia.DerivativeKind
	TransformVersion int
	Path             string
}

type DerivativeGenerator interface {
	Generate(context.Context, domainmedia.MediaType, string, string) ([]DerivativeOutput, error)
}

type PipelineOptions struct {
	TempDir string
	Policy  domainmedia.Policy
}

type Pipeline struct {
	router      storage.Router
	probe       ProbeInspector
	derivatives DerivativeGenerator
	options     PipelineOptions
}

func NewPipeline(router storage.Router, probe ProbeInspector, derivatives DerivativeGenerator, options PipelineOptions) *Pipeline {
	if options.TempDir == "" {
		options.TempDir = filepath.Join(os.TempDir(), "pic-gallery-media")
	}
	if options.Policy.SingleFileMaxBytes <= 0 {
		options.Policy = domainmedia.DefaultPolicy()
	}
	return &Pipeline{router: router, probe: probe, derivatives: derivatives, options: options}
}

func (pipeline *Pipeline) Process(ctx context.Context, item WorkItem) (ProcessResult, error) {
	if pipeline == nil || pipeline.router == nil || pipeline.probe == nil || pipeline.derivatives == nil {
		return ProcessResult{}, errors.New("media pipeline dependencies are unavailable")
	}
	if item.SizeBytes <= 0 || item.SizeBytes > domainmedia.SingleFileHardMaxBytes {
		return ProcessResult{}, errors.New("media object exceeds the processing size limit")
	}
	mediaType := domainmedia.MediaType(strings.ToLower(strings.TrimSpace(item.MediaType)))
	workDir, err := os.MkdirTemp(pipeline.options.TempDir, "job-*")
	if err != nil {
		return ProcessResult{}, fmt.Errorf("create media work directory: %w", err)
	}
	defer os.RemoveAll(workDir)

	inputPath := filepath.Join(workDir, "original"+extensionForMedia(item.MIMEType, item.ObjectKey))
	ref, err := pipeline.router.BackendFor(ctx, item.StorageConfigID, item.StorageDriver)
	if err != nil {
		return ProcessResult{}, fmt.Errorf("resolve media storage: %w", err)
	}
	streaming, ok := ref.Backend.(storage.StreamingBackend)
	if !ok {
		return ProcessResult{}, errors.New("media storage does not support bounded streaming reads")
	}
	reader, size, err := streaming.OpenReader(ctx, item.ObjectKey, domainmedia.SingleFileHardMaxBytes)
	if err != nil {
		return ProcessResult{}, fmt.Errorf("open media object: %w", err)
	}
	if err := writeTemporaryFile(ctx, inputPath, reader, size); err != nil {
		return ProcessResult{}, err
	}
	if size != item.SizeBytes {
		return ProcessResult{}, errors.New("media object size no longer matches its asset record")
	}

	probe, err := pipeline.probe.Inspect(ctx, inputPath, size, mediaType)
	if err != nil {
		return ProcessResult{}, err
	}
	declaration := domainmedia.UploadDeclaration{Filename: filepath.Base(item.ObjectKey), MediaType: mediaType, MIMEType: item.MIMEType, SizeBytes: item.SizeBytes}
	if validation := pipeline.options.Policy.ValidateProbe(declaration, probe); validation != nil {
		return ProcessResult{}, validation
	}
	outputDir := filepath.Join(workDir, "derivatives")
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return ProcessResult{}, fmt.Errorf("create media derivative directory: %w", err)
	}
	outputs, err := pipeline.derivatives.Generate(ctx, mediaType, inputPath, outputDir)
	if err != nil {
		return ProcessResult{}, err
	}
	writer, err := pipeline.router.DefaultWriter(ctx)
	if err != nil {
		return ProcessResult{}, fmt.Errorf("resolve derivative storage: %w", err)
	}
	streamingWriter, ok := writer.Backend.(storage.StreamingBackend)
	if !ok {
		return ProcessResult{}, errors.New("derivative storage does not support streaming writes")
	}
	result := ProcessResult{Probe: probeMetadata(probe)}
	for _, output := range outputs {
		derivative, err := uploadDerivative(ctx, streamingWriter, writer, item, output)
		if err != nil {
			return ProcessResult{}, err
		}
		result.Derivatives = append(result.Derivatives, derivative)
	}
	return result, nil
}

func writeTemporaryFile(ctx context.Context, path string, reader io.ReadCloser, size int64) error {
	defer reader.Close()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	written, err := io.Copy(file, &contextReader{ctx: ctx, reader: reader})
	if err != nil {
		return fmt.Errorf("download media object: %w", err)
	}
	if written != size {
		return errors.New("media object size changed during download")
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}

func uploadDerivative(ctx context.Context, backend storage.StreamingBackend, ref storage.BackendRef, item WorkItem, output DerivativeOutput) (Derivative, error) {
	file, err := os.Open(output.Path)
	if err != nil {
		return Derivative{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() <= 0 {
		return Derivative{}, errors.New("media derivative is empty")
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return Derivative{}, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return Derivative{}, err
	}
	extension := filepath.Ext(output.Path)
	objectKey := filepath.ToSlash(filepath.Join("media", "derivatives", fmt.Sprint(item.UserID), item.AssetID, fmt.Sprintf("%s-v%d%s", output.Kind, output.TransformVersion, extension)))
	mimeType := mime.TypeByExtension(extension)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	if err := backend.PutReader(ctx, objectKey, mimeType, file, info.Size()); err != nil {
		_ = ref.Backend.Delete(context.WithoutCancel(ctx), objectKey)
		return Derivative{}, fmt.Errorf("store media derivative: %w", err)
	}
	return Derivative{Kind: string(output.Kind), TransformVersion: output.TransformVersion, StorageConfigID: ref.ConfigID, StorageDriver: ref.Driver, Bucket: ref.Bucket, ObjectKey: objectKey, MIMEType: mimeType, SizeBytes: info.Size(), SHA256: hex.EncodeToString(hasher.Sum(nil))}, nil
}

func probeMetadata(probe domainmedia.ProbeResult) ProbeMetadata {
	return ProbeMetadata{Format: probe.Format, Container: probe.Container, VideoCodec: probe.VideoCodec, AudioCodec: probe.AudioCodec, Width: probe.Width, Height: probe.Height, DurationMS: probe.DurationMS, FrameRateMilli: probe.FrameRateMilli, Channels: probe.Channels, SampleRate: probe.SampleRate}
}

func extensionForMedia(mimeType, objectKey string) string {
	if extension := filepath.Ext(objectKey); extension != "" && len(extension) <= 10 {
		return extension
	}
	extensions, _ := mime.ExtensionsByType(mimeType)
	if len(extensions) > 0 {
		return extensions[0]
	}
	return ".bin"
}
