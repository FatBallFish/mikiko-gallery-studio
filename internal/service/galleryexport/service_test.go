package galleryexport

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

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
	archive, manifest, err := service.buildArchive(context.Background(), []Asset{
		{ID: "one", ObjectKey: "ok-1", MIMEType: "image/png", DisplayName: "../same"},
		{ID: "two", ObjectKey: "ok-2", MIMEType: "image/png", DisplayName: "..\\same"},
		{ID: "three", ObjectKey: "missing", MIMEType: "image/png", DisplayName: "manifest.json"},
	})
	if err != nil {
		t.Fatalf("build archive: %v", err)
	}
	if len(manifest.Files) != 3 || manifest.Files[0].Filename != "same.png" || manifest.Files[1].Filename != "same-2.png" {
		t.Fatalf("manifest filenames = %#v", manifest.Files)
	}
	if manifest.Files[2].Status != FileStatusFailed || manifest.Files[2].ErrorCode != "object_not_found" {
		t.Fatalf("failed manifest entry = %#v", manifest.Files[2])
	}

	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
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
func (b *exportBackend) Get(_ context.Context, key string) ([]byte, error) {
	if err := b.errors[key]; err != nil {
		return nil, err
	}
	return append([]byte(nil), b.objects[key]...), nil
}
func (*exportBackend) Delete(context.Context, string) error { return nil }

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
