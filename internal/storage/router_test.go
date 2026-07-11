package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	domainstorageconfig "github.com/fatballfish/pic-gallery/internal/domain/storageconfig"
)

func TestRegistryRoutesDefaultAndLegacyLocalBackends(t *testing.T) {
	oldRoot := t.TempDir()
	newRoot := t.TempDir()
	source := fakeConfigSource{
		defaultConfig: domainstorageconfig.ResolvedConfig{ConfigRecord: domainstorageconfig.ConfigRecord{
			ID: "new", Code: "r2-local", Driver: "local", Status: "enabled", ReadEnabled: true, WriteEnabled: true, IsDefault: true, LocalRoot: newRoot, Version: 1,
		}},
		legacyConfig: domainstorageconfig.ResolvedConfig{ConfigRecord: domainstorageconfig.ConfigRecord{
			ID: "old", Code: "bootstrap-local", Driver: "local", Status: "enabled", ReadEnabled: true, WriteEnabled: false, LocalRoot: oldRoot, Version: 1,
		}},
	}
	registry := NewRegistry(source, 0)

	writer, err := registry.DefaultWriter(context.Background())
	if err != nil {
		t.Fatalf("DefaultWriter returned error: %v", err)
	}
	if writer.ConfigID != "new" {
		t.Fatalf("expected default config new, got %q", writer.ConfigID)
	}
	if err := writer.Backend.Put(context.Background(), "generated-images/new.png", "image/png", []byte("new")); err != nil {
		t.Fatalf("put new object: %v", err)
	}

	legacyBackend := NewLocalBackend(oldRoot)
	if err := legacyBackend.Put(context.Background(), "generated-images/old.png", "image/png", []byte("old")); err != nil {
		t.Fatalf("seed legacy object: %v", err)
	}
	reader, err := registry.BackendFor(context.Background(), "", "local")
	if err != nil {
		t.Fatalf("BackendFor legacy returned error: %v", err)
	}
	got, err := reader.Backend.Get(context.Background(), "generated-images/old.png")
	if err != nil {
		t.Fatalf("get legacy object: %v", err)
	}
	if string(got) != "old" {
		t.Fatalf("unexpected legacy content %q", got)
	}
	if _, err := NewLocalBackend(newRoot).Get(context.Background(), filepath.ToSlash("generated-images/old.png")); err == nil {
		t.Fatal("legacy object should not be read from default backend")
	}
}

func TestRegistryProbeDeletesProbeObjectOnSuccess(t *testing.T) {
	root := t.TempDir()
	registry := NewRegistry(nil, 0)
	result := registry.probeBackend(context.Background(), NewLocalBackend(root), ".pic-gallery-probe/success.txt", time.Now())
	if result.Status != domainstorageconfig.ProbeStatusSuccess {
		t.Fatalf("expected successful probe, got %#v", result)
	}
	if _, err := NewLocalBackend(root).Get(context.Background(), ".pic-gallery-probe/success.txt"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected probe object to be deleted, got %v", err)
	}
}

func TestRegistryProbeCleanupIgnoresCanceledProbeContext(t *testing.T) {
	backend := &cancelSensitiveBackend{content: map[string][]byte{}}
	registry := NewRegistry(nil, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := registry.probeBackend(ctx, backend, ".pic-gallery-probe/canceled.txt", time.Now())
	if result.Status != domainstorageconfig.ProbeStatusFailed {
		t.Fatalf("expected failed probe after canceled get, got %#v", result)
	}
	if !backend.deleted {
		t.Fatal("expected cleanup to delete probe object with an independent context")
	}
	if _, exists := backend.content[".pic-gallery-probe/canceled.txt"]; exists {
		t.Fatal("expected probe object to be removed from backend")
	}
}

type fakeConfigSource struct {
	defaultConfig domainstorageconfig.ResolvedConfig
	legacyConfig  domainstorageconfig.ResolvedConfig
}

func (s fakeConfigSource) ResolveDefaultWritable(context.Context) (domainstorageconfig.ResolvedConfig, error) {
	return s.defaultConfig, nil
}

func (s fakeConfigSource) ResolveByID(_ context.Context, id string) (domainstorageconfig.ResolvedConfig, error) {
	if id == s.defaultConfig.ID {
		return s.defaultConfig, nil
	}
	return s.legacyConfig, nil
}

func (s fakeConfigSource) ResolveLegacyByDriver(context.Context, string) (domainstorageconfig.ResolvedConfig, error) {
	return s.legacyConfig, nil
}

type cancelSensitiveBackend struct {
	content map[string][]byte
	deleted bool
}

func (b *cancelSensitiveBackend) Driver() string { return "test" }

func (b *cancelSensitiveBackend) Put(_ context.Context, key string, _ string, content []byte) error {
	b.content[key] = append([]byte{}, content...)
	return nil
}

func (b *cancelSensitiveBackend) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	content, ok := b.content[key]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte{}, content...), nil
}

func (b *cancelSensitiveBackend) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.deleted = true
	delete(b.content, key)
	return nil
}
