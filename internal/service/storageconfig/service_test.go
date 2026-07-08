package storageconfig

import (
	"context"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainstorageconfig "github.com/fatballfish/pic-gallery/internal/domain/storageconfig"
)

func TestServiceCreatesR2WithAutoRegionAndRequiresProbeBeforeDefault(t *testing.T) {
	store := newMemoryStore()
	svc := NewService(store, "test-key", config.StorageConfig{Driver: "local"}, "local")

	created, err := svc.Create(context.Background(), domainstorageconfig.WriteRequest{
		Code:         "r2-prod",
		Name:         "R2 Prod",
		Driver:       "s3",
		Provider:     "r2",
		Status:       "enabled",
		ReadEnabled:  true,
		WriteEnabled: true,
		Endpoint:     "https://example.r2.cloudflarestorage.com",
		Bucket:       "pic-gallery",
		Secrets:      map[string]string{"access_key_id": "ak", "secret_access_key": "sk"},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if created.Region != "auto" || created.SecretStatus.HasSecret != true {
		t.Fatalf("expected R2 auto region and secret status, got %#v", created)
	}

	if _, err := svc.SetDefault(context.Background(), domainstorageconfig.SetDefaultRequest{ID: created.ID, Version: created.Version}); err == nil {
		t.Fatal("expected set-default to require successful probe")
	}

	probed, err := svc.UpdateProbe(context.Background(), created.ID, domainstorageconfig.ProbeResult{Status: "success", Message: "ok"}, 7)
	if err != nil {
		t.Fatalf("UpdateProbe returned error: %v", err)
	}
	updated, err := svc.SetDefault(context.Background(), domainstorageconfig.SetDefaultRequest{ID: created.ID, Version: probed.Version, UpdatedBy: 7})
	if err != nil {
		t.Fatalf("SetDefault returned error: %v", err)
	}
	if !updated.IsDefault {
		t.Fatalf("expected config to become default")
	}
}

func TestServiceRejectsMaskedSecretPlaceholder(t *testing.T) {
	svc := NewService(newMemoryStore(), "test-key", config.StorageConfig{Driver: "local"}, "local")
	_, err := svc.Create(context.Background(), domainstorageconfig.WriteRequest{
		Code:         "s3-prod",
		Name:         "S3 Prod",
		Driver:       "s3",
		Provider:     "custom_s3",
		Status:       "enabled",
		ReadEnabled:  true,
		WriteEnabled: true,
		Endpoint:     "https://s3.example.com",
		Region:       "us-east-1",
		Bucket:       "pic-gallery",
		Secrets:      map[string]string{"access_key_id": "****", "secret_access_key": "sk"},
	})
	if err == nil {
		t.Fatal("expected masked placeholder to be rejected")
	}
}

func TestServiceBootstrapsLocalDefault(t *testing.T) {
	store := newMemoryStore()
	svc := NewService(store, "test-key", config.StorageConfig{Driver: "local", LocalRoot: "/tmp/pic-gallery"}, "local")
	if err := svc.Bootstrap(context.Background(), 0); err != nil {
		t.Fatalf("Bootstrap returned error: %v", err)
	}
	resolved, err := svc.ResolveDefaultWritable(context.Background())
	if err != nil {
		t.Fatalf("ResolveDefaultWritable returned error: %v", err)
	}
	if resolved.Code != "bootstrap-local" || resolved.LocalRoot != "/tmp/pic-gallery" {
		t.Fatalf("unexpected bootstrap config: %#v", resolved)
	}
}

type memoryStore struct {
	records map[string]domainstorageconfig.ConfigRecord
	nextID  int
}

func newMemoryStore() *memoryStore {
	return &memoryStore{records: map[string]domainstorageconfig.ConfigRecord{}}
}

func (s *memoryStore) List(context.Context) ([]domainstorageconfig.ConfigRecord, error) {
	items := make([]domainstorageconfig.ConfigRecord, 0, len(s.records))
	for _, item := range s.records {
		if item.Status != domainstorageconfig.StatusDeleted {
			items = append(items, item)
		}
	}
	return items, nil
}

func (s *memoryStore) GetByID(_ context.Context, id string) (domainstorageconfig.ConfigRecord, bool, error) {
	item, ok := s.records[id]
	return item, ok, nil
}

func (s *memoryStore) GetByCode(_ context.Context, code string) (domainstorageconfig.ConfigRecord, bool, error) {
	for _, item := range s.records {
		if item.Code == code {
			return item, true, nil
		}
	}
	return domainstorageconfig.ConfigRecord{}, false, nil
}

func (s *memoryStore) GetDefaultWritable(context.Context) (domainstorageconfig.ConfigRecord, bool, error) {
	for _, item := range s.records {
		if item.IsDefault && item.Status == domainstorageconfig.StatusEnabled && item.ReadEnabled && item.WriteEnabled {
			return item, true, nil
		}
	}
	return domainstorageconfig.ConfigRecord{}, false, nil
}

func (s *memoryStore) GetLegacyByDriver(_ context.Context, driver string) (domainstorageconfig.ConfigRecord, bool, error) {
	return s.GetByCode(context.Background(), "bootstrap-"+driver)
}

func (s *memoryStore) Save(_ context.Context, record domainstorageconfig.ConfigRecord) (domainstorageconfig.ConfigRecord, error) {
	if record.ID == "" {
		s.nextID++
		record.ID = string(rune('a' + s.nextID))
	}
	s.records[record.ID] = record
	return record, nil
}

func (s *memoryStore) ClearDefault(context.Context) error {
	for id, item := range s.records {
		item.IsDefault = false
		s.records[id] = item
	}
	return nil
}
