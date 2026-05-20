package assets

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
)

func TestUploadDeduplicatesByHash(t *testing.T) {
	data, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+y1X8AAAAASUVORK5CYII=")
	storageRoot := t.TempDir()
	svc := NewService(config.StorageConfig{LocalRoot: storageRoot}, config.GenerationLimitsConfig{ReferenceImageMaxMB: 10})
	first, err := svc.Upload(1, "tiny.png", "image/png", data)
	if err != nil {
		t.Fatalf("Upload first: %v", err)
	}
	second, err := svc.Upload(1, "copy.png", "image/png", data)
	if err != nil {
		t.Fatalf("Upload second: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected duplicate upload to reuse asset id")
	}
	if first.Width != 1 || first.Height != 1 {
		t.Fatalf("expected image size 1x1, got %dx%d", first.Width, first.Height)
	}
	if _, err := svc.Get(1, first.ID); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, err := os.Stat(filepath.Join(storageRoot, "reference-assets", filepath.Base(first.ObjectKey))); err != nil {
		t.Fatalf("expected stored file to exist: %v", err)
	}
}

type failingStore struct{}

func (f failingStore) GetByUserAndHash(_ context.Context, _ int64, _ string) (domainassets.ReferenceAsset, error) {
	return domainassets.ReferenceAsset{}, repoerr.ErrNotFound
}

func (f failingStore) Save(_ context.Context, _ int64, _ domainassets.ReferenceAsset) error {
	return errors.New("db unavailable")
}

func (f failingStore) GetByUserAndID(_ context.Context, _ int64, _ string) (domainassets.ReferenceAsset, error) {
	return domainassets.ReferenceAsset{}, repoerr.ErrNotFound
}

func TestUploadDoesNotCacheFailedPersistence(t *testing.T) {
	data, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+y1X8AAAAASUVORK5CYII=")
	storageRoot := t.TempDir()
	svc := NewServiceWithStore(config.StorageConfig{LocalRoot: storageRoot}, config.GenerationLimitsConfig{ReferenceImageMaxMB: 10}, failingStore{})

	if _, err := svc.Upload(1, "tiny.png", "image/png", data); err == nil {
		t.Fatal("expected upload to fail when metadata persistence fails")
	}
	files, err := os.ReadDir(filepath.Join(storageRoot, "reference-assets"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected failed upload to clean up stored file, found %d files", len(files))
	}
	if len(svc.assetsByID) != 0 || len(svc.assetsByHash) != 0 {
		t.Fatalf("expected failed upload to avoid cache pollution, got ids=%d hashes=%d", len(svc.assetsByID), len(svc.assetsByHash))
	}
}
