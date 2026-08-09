package galleryexport

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/storage"
)

func TestNormalizeIDsDeduplicatesAndCapsExplicitSelection(t *testing.T) {
	ids, err := normalizeIDs([]string{" image-1 ", "image-2", "image-1"}, 3)
	if err != nil {
		t.Fatalf("normalize IDs: %v", err)
	}
	if got := joinIDs(ids); got != "image-1,image-2" {
		t.Fatalf("normalized IDs = %q", got)
	}
	if _, err := normalizeIDs([]string{"one", "two", "three"}, 2); !errors.Is(err, ErrBatchTooLarge) {
		t.Fatalf("over-cap error = %v, want ErrBatchTooLarge", err)
	}
}

func TestCreateDownloadPromotesLargeSelectionsToDurableJob(t *testing.T) {
	store := &exportStoreStub{assets: []Asset{
		{ID: "one", FileSizeBytes: 6},
		{ID: "two", FileSizeBytes: 6},
	}}
	service := NewService(store, storage.NewStaticRouter(&exportBackend{}), Options{
		MaxBatchSize:            10,
		DirectMaxCount:          10,
		DirectMaxEstimatedBytes: 10,
	})

	result, err := service.CreateDownload(context.Background(), CreateDownloadRequest{
		UserID: 7, ProjectID: "project-1", ImageIDs: []string{"one", "two"},
	})
	if err != nil {
		t.Fatalf("create download: %v", err)
	}
	if result.Archive != nil || result.Job == nil || result.Job.State != StateQueued {
		t.Fatalf("large download result = %#v", result)
	}
	if store.created.UserID != 7 || joinIDs(store.created.ImageIDs) != "one,two" || store.created.EstimatedBytes != 12 {
		t.Fatalf("persisted job = %#v", store.created)
	}
}

func TestBuildArchiveSanitizesAndDeduplicatesNamesAndReportsReadFailures(t *testing.T) {
	backend := &exportBackend{
		objects: map[string][]byte{"ok-1": []byte("first"), "ok-2": []byte("second")},
		errors:  map[string]error{"missing": storage.ErrNotFound},
	}
	service := NewService(&exportStoreStub{}, storage.NewStaticRouter(backend), Options{MaxArchiveBytes: 1024})
	archive, err := service.buildArchive(context.Background(), []Asset{
		{ID: "one", ObjectKey: "ok-1", MIMEType: "image/png", DisplayName: "../same"},
		{ID: "two", ObjectKey: "ok-2", MIMEType: "image/png", DisplayName: "..\\same"},
		{ID: "three", ObjectKey: "missing", MIMEType: "image/png", DisplayName: "manifest.json"},
	})
	if err != nil {
		t.Fatalf("build archive: %v", err)
	}
	t.Cleanup(func() { _ = archive.Close() })
	manifest := archive.Manifest
	if len(manifest.Files) != 3 || manifest.Files[0].Filename != "same.png" || manifest.Files[1].Filename != "same-2.png" {
		t.Fatalf("manifest filenames = %#v", manifest.Files)
	}
	if manifest.Files[2].Status != FileStatusFailed || manifest.Files[2].ErrorCode != "object_not_found" {
		t.Fatalf("failed manifest entry = %#v", manifest.Files[2])
	}

	reader, err := zip.OpenReader(archive.Path)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer reader.Close()
	if len(reader.File) != 3 {
		t.Fatalf("zip entries = %d, want two images plus manifest", len(reader.File))
	}
	for _, file := range reader.File {
		if file.Name == "../same.png" || file.Name == "..\\same.png" || file.Name == "manifest.json.png" {
			t.Fatalf("unsafe or reserved archive filename %q", file.Name)
		}
		if file.Name != "manifest.json" {
			continue
		}
		stream, openErr := file.Open()
		if openErr != nil {
			t.Fatalf("open manifest: %v", openErr)
		}
		payload, readErr := io.ReadAll(stream)
		_ = stream.Close()
		if readErr != nil {
			t.Fatalf("read manifest: %v", readErr)
		}
		var decoded Manifest
		if err := json.Unmarshal(payload, &decoded); err != nil || len(decoded.Files) != 3 {
			t.Fatalf("manifest payload = %s err=%v", payload, err)
		}
	}
}

func TestBuildArchiveEnforcesIndependentFileSourceAndFinalArchiveBudgets(t *testing.T) {
	backend := &exportBackend{objects: map[string][]byte{"one": bytes.Repeat([]byte("a"), 64), "two": bytes.Repeat([]byte("b"), 64)}}
	assets := []Asset{{ID: "one", ObjectKey: "one"}, {ID: "two", ObjectKey: "two"}}
	if _, err := NewService(&exportStoreStub{}, storage.NewStaticRouter(backend), Options{MaxFileCount: 1}).buildArchive(t.Context(), assets); !errors.Is(err, ErrBatchTooLarge) {
		t.Fatalf("file-count error=%v", err)
	}
	if _, err := NewService(&exportStoreStub{}, storage.NewStaticRouter(backend), Options{MaxFileCount: 2, MaxSourceBytes: 100}).buildArchive(t.Context(), assets); !errors.Is(err, ErrSourceLimitExceeded) {
		t.Fatalf("source budget error=%v", err)
	}
	if archive, err := NewService(&exportStoreStub{}, storage.NewStaticRouter(backend), Options{MaxFileCount: 2, MaxSourceBytes: 1024, MaxArchiveBytes: 100}).buildArchive(t.Context(), assets); !errors.Is(err, ErrArchiveLimitExceeded) {
		if archive.Path != "" {
			_ = archive.Close()
		}
		t.Fatalf("archive budget error=%v", err)
	}
}

func TestBuildArchiveUsesTempFileAndCleansItUp(t *testing.T) {
	backend := &exportBackend{objects: map[string][]byte{"one": []byte("content")}}
	service := NewService(&exportStoreStub{}, storage.NewStaticRouter(backend), Options{TempDir: t.TempDir(), MaxArchiveBytes: 4096})
	archive, err := service.buildArchive(t.Context(), []Asset{{ID: "one", ObjectKey: "one"}})
	if err != nil {
		t.Fatal(err)
	}
	if archive.Path == "" || archive.Size <= 0 {
		t.Fatalf("archive=%#v", archive)
	}
	if _, err := os.Stat(archive.Path); err != nil {
		t.Fatal(err)
	}
	path := archive.Path
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temp archive remains: %v", err)
	}
}

func TestCreateDownloadAppliesDirectDeadline(t *testing.T) {
	backend := &blockingExportBackend{started: make(chan struct{})}
	service := NewService(&exportStoreStub{assets: []Asset{{ID: "one", ObjectKey: "one"}}}, storage.NewStaticRouter(backend), Options{DirectTimeout: 20 * time.Millisecond})
	_, err := service.CreateDownload(t.Context(), CreateDownloadRequest{UserID: 7, ProjectID: "project", ImageIDs: []string{"one"}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("direct deadline error=%v", err)
	}
}

type exportStoreStub struct {
	assets  []Asset
	created CreateJobRequest
}

func (s *exportStoreStub) AuthorizeAssets(context.Context, int64, string, []string) ([]Asset, error) {
	return append([]Asset(nil), s.assets...), nil
}

func (s *exportStoreStub) CreateJob(_ context.Context, req CreateJobRequest) (Job, error) {
	s.created = req
	return Job{ID: "job-1", UserID: req.UserID, ProjectID: req.ProjectID, ImageIDs: req.ImageIDs, State: StateQueued}, nil
}

type exportBackend struct {
	objects map[string][]byte
	errors  map[string]error
}

func (*exportBackend) Driver() string { return "local" }
func (b *exportBackend) Put(_ context.Context, key, _ string, content []byte) error {
	if b.objects == nil {
		b.objects = map[string][]byte{}
	}
	b.objects[key] = append([]byte(nil), content...)
	return nil
}
func (b *exportBackend) PutReader(ctx context.Context, key, contentType string, reader io.Reader, size int64) error {
	content, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	if int64(len(content)) != size {
		return storage.ErrSizeMismatch
	}
	return b.Put(ctx, key, contentType, content)
}
func (b *exportBackend) Get(_ context.Context, key string) ([]byte, error) {
	if err := b.errors[key]; err != nil {
		return nil, err
	}
	return append([]byte(nil), b.objects[key]...), nil
}
func (b *exportBackend) OpenReader(ctx context.Context, key string, maxBytes int64) (io.ReadCloser, int64, error) {
	content, err := b.Get(ctx, key)
	if err != nil {
		return nil, 0, err
	}
	if int64(len(content)) > maxBytes {
		return nil, 0, storage.ErrObjectTooLarge
	}
	return io.NopCloser(bytes.NewReader(content)), int64(len(content)), nil
}
func (b *exportBackend) Delete(_ context.Context, key string) error {
	delete(b.objects, key)
	return nil
}

func joinIDs(ids []string) string {
	var result bytes.Buffer
	for index, id := range ids {
		if index > 0 {
			result.WriteByte(',')
		}
		result.WriteString(id)
	}
	return result.String()
}
