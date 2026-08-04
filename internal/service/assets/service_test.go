package assets

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

func TestStorageRouterPinsReferenceAssetToOriginalConfig(t *testing.T) {
	data, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+y1X8AAAAASUVORK5CYII=")
	original := storage.BackendRef{ConfigID: "original", Version: 2, Driver: "local", Backend: storage.NewLocalBackend(t.TempDir())}
	replacement := storage.BackendRef{ConfigID: "replacement", Version: 1, Driver: "local", Backend: storage.NewLocalBackend(t.TempDir())}
	router := &switchingAssetRouter{defaultRef: original, refs: map[string]storage.BackendRef{"original": original, "replacement": replacement}}
	store := newMemoryAssetStore()
	svc := NewServiceWithStoreAndRouter(config.GenerationLimitsConfig{ReferenceImageMaxMB: 10}, store, router)

	asset, err := svc.Upload(9, "tiny.png", "image/png", data)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if asset.StorageConfigID != "original" {
		t.Fatalf("expected original storage config, got %#v", asset)
	}
	router.defaultRef = replacement
	_, downloaded, err := svc.Download(9, asset.ID)
	if err != nil {
		t.Fatalf("Download after default switch: %v", err)
	}
	if string(downloaded) != string(data) {
		t.Fatal("downloaded historical asset content mismatch")
	}
}

func TestTemporaryMediaURLProjectionForReferenceAsset(t *testing.T) {
	data, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+y1X8AAAAASUVORK5CYII=")
	backend := &temporaryAssetBackend{}
	svc := NewServiceWithStoreAndBackend(config.StorageConfig{}, config.GenerationLimitsConfig{ReferenceImageMaxMB: 10}, nil, backend)
	asset, err := svc.Upload(9, "tiny.png", "image/png", data)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	projected, err := svc.Get(9, asset.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if projected.PreviewURL != "https://assets.example.test/preview?sig=secret" || projected.DownloadURL != "https://assets.example.test/download?sig=secret" {
		t.Fatalf("unexpected projected asset %#v", projected)
	}
	if backend.signCalls != 2 || backend.lastOptions.Expiry != 5*time.Minute || backend.lastOptions.ResponseFilename != filepath.Base(asset.ObjectKey) {
		t.Fatalf("unexpected signer calls=%d options=%#v", backend.signCalls, backend.lastOptions)
	}
	if _, err := svc.Get(10, asset.ID); err == nil {
		t.Fatal("non-owner must be rejected")
	}
	if backend.signCalls != 2 {
		t.Fatalf("non-owner request reached signer; sign calls=%d", backend.signCalls)
	}
	backend.signErr = errors.New("signing unavailable")
	if _, err := svc.Get(9, asset.ID); err == nil {
		t.Fatal("reference asset signing failure must not silently fall back")
	}
}

type temporaryAssetBackend struct {
	objects     map[string][]byte
	signCalls   int
	lastOptions storage.TemporaryGetURLOptions
	signErr     error
}

func (b *temporaryAssetBackend) Driver() string { return "s3" }
func (b *temporaryAssetBackend) Put(_ context.Context, key, _ string, content []byte) error {
	if b.objects == nil {
		b.objects = map[string][]byte{}
	}
	b.objects[key] = append([]byte(nil), content...)
	return nil
}
func (b *temporaryAssetBackend) Get(_ context.Context, key string) ([]byte, error) {
	return append([]byte(nil), b.objects[key]...), nil
}
func (b *temporaryAssetBackend) Delete(_ context.Context, key string) error {
	delete(b.objects, key)
	return nil
}
func (b *temporaryAssetBackend) TemporaryGetURL(_ context.Context, _ string, options storage.TemporaryGetURLOptions) (string, error) {
	b.signCalls++
	b.lastOptions = options
	if b.signErr != nil {
		return "", b.signErr
	}
	if options.ResponseFilename == "" {
		return "https://assets.example.test/preview?sig=secret", nil
	}
	return "https://assets.example.test/download?sig=secret", nil
}

type switchingAssetRouter struct {
	defaultRef storage.BackendRef
	refs       map[string]storage.BackendRef
}

func (r *switchingAssetRouter) DefaultWriter(context.Context) (storage.BackendRef, error) {
	return r.defaultRef, nil
}

func (r *switchingAssetRouter) BackendFor(_ context.Context, configID string, _ string) (storage.BackendRef, error) {
	return r.refs[configID], nil
}

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

	if err := svc.Delete(1, first.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	third, err := svc.Upload(1, "reupload.png", "image/png", data)
	if err != nil {
		t.Fatalf("Upload after delete: %v", err)
	}
	if third.ID == first.ID {
		t.Fatalf("expected reupload after delete to create a new asset")
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

func (f failingStore) DeleteByUserAndID(_ context.Context, _ int64, _ string) error {
	return repoerr.ErrNotFound
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

func TestUploadReturnsTooLargeErrorWithSizeDetails(t *testing.T) {
	svc := NewService(config.StorageConfig{LocalRoot: t.TempDir()}, config.GenerationLimitsConfig{ReferenceImageMaxMB: 1})

	_, err := svc.Upload(1, "large.png", "image/png", make([]byte, 1024*1024+1))
	if err == nil {
		t.Fatal("expected upload to reject oversized reference asset")
	}
	appErr, ok := err.(*errs.Error)
	if !ok {
		t.Fatalf("expected app error, got %T %v", err, err)
	}
	if appErr.Code != errs.CodeImageReferenceTooLarge {
		t.Fatalf("expected %s, got %s", errs.CodeImageReferenceTooLarge, appErr.Code)
	}
	if appErr.Details["max_size_bytes"] != int64(1024*1024) {
		t.Fatalf("expected max_size_bytes detail, got %#v", appErr.Details)
	}
	if appErr.Details["max_size_mb"] != 1 {
		t.Fatalf("expected max_size_mb detail, got %#v", appErr.Details)
	}
	if appErr.Details["actual_size_bytes"] != int64(1024*1024+1) {
		t.Fatalf("expected actual_size_bytes detail, got %#v", appErr.Details)
	}
}

func TestDeleteWithStoreClearsDedupeCache(t *testing.T) {
	data, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+y1X8AAAAASUVORK5CYII=")
	store := newMemoryAssetStore()
	svc := NewServiceWithStore(config.StorageConfig{LocalRoot: t.TempDir()}, config.GenerationLimitsConfig{ReferenceImageMaxMB: 10}, store)
	first, err := svc.Upload(1, "tiny.png", "image/png", data)
	if err != nil {
		t.Fatalf("Upload first: %v", err)
	}
	if err := svc.Delete(1, first.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	second, err := svc.Upload(1, "tiny-again.png", "image/png", data)
	if err != nil {
		t.Fatalf("Upload second: %v", err)
	}
	if second.ID == first.ID || second.Status == "deleted" {
		t.Fatalf("expected delete to clear dedupe cache, first=%#v second=%#v", first, second)
	}
}

type memoryAssetStore struct {
	assets map[string]domainassets.ReferenceAsset
	users  map[string]int64
}

func newMemoryAssetStore() *memoryAssetStore {
	return &memoryAssetStore{assets: map[string]domainassets.ReferenceAsset{}, users: map[string]int64{}}
}

func (s *memoryAssetStore) GetByUserAndHash(_ context.Context, userID int64, sha string) (domainassets.ReferenceAsset, error) {
	for id, asset := range s.assets {
		if s.users[id] == userID && asset.SHA256 == sha && asset.Status != "deleted" {
			return asset, nil
		}
	}
	return domainassets.ReferenceAsset{}, repoerr.ErrNotFound
}

func (s *memoryAssetStore) Save(_ context.Context, userID int64, asset domainassets.ReferenceAsset) error {
	if asset.CreatedAt.IsZero() {
		asset.CreatedAt = time.Now()
	}
	s.assets[asset.ID] = asset
	s.users[asset.ID] = userID
	return nil
}

func (s *memoryAssetStore) GetByUserAndID(_ context.Context, userID int64, assetID string) (domainassets.ReferenceAsset, error) {
	asset, ok := s.assets[assetID]
	if !ok || s.users[assetID] != userID {
		return domainassets.ReferenceAsset{}, repoerr.ErrNotFound
	}
	return asset, nil
}

func (s *memoryAssetStore) DeleteByUserAndID(_ context.Context, userID int64, assetID string) error {
	asset, ok := s.assets[assetID]
	if !ok || s.users[assetID] != userID {
		return repoerr.ErrNotFound
	}
	asset.Status = "deleted"
	s.assets[assetID] = asset
	return nil
}
