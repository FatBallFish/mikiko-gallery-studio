package storage

import (
	"context"
	"path/filepath"
	"testing"

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
