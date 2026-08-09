package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	domaincleanup "github.com/fatballfish/pic-gallery/internal/domain/objectcleanup"
	domainstorageconfig "github.com/fatballfish/pic-gallery/internal/domain/storageconfig"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/objectdeletionjob"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	objectcleanupservice "github.com/fatballfish/pic-gallery/internal/service/objectcleanup"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

func TestStartupStorageIdentityBackfillGatesCleanupUntilComplete(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, fmt.Sprintf("file:startup-storage-identity-%s?mode=memory&cache=shared&_fk=1", uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	deletedAt := time.Now().UTC().Add(-2 * time.Hour)
	for index := range 3 {
		task, err := client.ImageTask.Create().SetUserID(902).SetTaskType("text_to_image").SetPrompt("legacy").SetAbstractModel("plus").Save(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		create := client.ImageResult.Create().SetTaskID(task.ID).SetUserID(task.UserID).SetStorageDriver("local").
			SetObjectKey(fmt.Sprintf("generated-images/%d.png", index)).SetMimeType("image/png").SetSha256(fmt.Sprint(index))
		if index == 2 {
			create.SetDeletedAt(deletedAt)
		}
		if _, err := create.Save(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	configID := uuid.New()
	resolver := legacyStorageResolverStub{resolved: domainstorageconfig.ResolvedConfig{ConfigRecord: domainstorageconfig.ConfigRecord{ID: configID.String(), Driver: "local"}}}

	progress, err := backfillLegacyStorageIdentityAtStartup(t.Context(), client, resolver, "local", db.LegacyStorageIdentityBackfillOptions{BatchSize: 1, MaxBatches: 1})
	if !errors.Is(err, ErrLegacyStorageIdentityBackfillIncomplete) || progress.Completed {
		t.Fatalf("bounded startup backfill = %#v, %v", progress, err)
	}

	for attempt := 0; attempt < 10 && !progress.Completed; attempt++ {
		progress, err = backfillLegacyStorageIdentityAtStartup(t.Context(), client, resolver, "local", db.LegacyStorageIdentityBackfillOptions{BatchSize: 1, MaxBatches: 10})
		if err != nil && !errors.Is(err, ErrLegacyStorageIdentityBackfillIncomplete) {
			t.Fatal(err)
		}
	}
	if err != nil || !progress.Completed {
		t.Fatalf("completed startup backfill = %#v, %v", progress, err)
	}

	backend := &startupStorageListingBackend{object: storage.ObjectInfo{ObjectKey: "generated-images/0.png", ModifiedAt: deletedAt}}
	ref := storage.BackendRef{ConfigID: configID.String(), Driver: "local", Namespace: "bootstrap-local", Backend: backend}
	cleanupStore := entstore.NewObjectCleanupStore(client)
	processor := objectcleanupservice.NewProcessor(cleanupStore, startupStorageRouter{ref: ref}, objectcleanupservice.ProcessorOptions{
		Now: func() time.Time { return time.Now().UTC() }, OrphanGracePeriod: time.Hour, ObjectListPageSize: 10,
	})
	for range 2 {
		if _, err := processor.Reconcile(t.Context(), 10); err != nil {
			t.Fatal(err)
		}
	}
	jobs, err := client.ObjectDeletionJob.Query().Where(
		objectdeletionjob.StateIn(domaincleanup.StatePending, domaincleanup.StateRunning, domaincleanup.StateRetry, domaincleanup.StateBlocked),
	).All(t.Context())
	if err != nil || len(jobs) != 1 || jobs[0].StorageConfigID == nil || *jobs[0].StorageConfigID != configID || jobs[0].ObjectKey != "generated-images/2.png" {
		t.Fatalf("configured cleanup jobs after gate = %#v, %v", jobs, err)
	}
	if processed, err := processor.ProcessOnce(t.Context()); err != nil || !processed {
		t.Fatalf("process configured cleanup = %v, %v", processed, err)
	}
	if len(backend.deleted) != 1 || backend.deleted[0] != "generated-images/2.png" {
		t.Fatalf("deleted objects = %v", backend.deleted)
	}
}

func TestAPIAndWorkerGateCleanupOnLegacyStorageIdentityBackfill(t *testing.T) {
	tests := []struct {
		path       string
		cleanupUse string
	}{
		{path: "run.go", cleanupUse: "startObjectCleanupLoop("},
		{path: "worker.go", cleanupUse: "runner.SetCleanupService("},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			contents, err := os.ReadFile(test.path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(contents)
			bootstrapAt := strings.Index(source, "storageConfigSvc.Bootstrap(")
			gateAt := strings.Index(source, "requireLegacyStorageIdentityBackfill(")
			cleanupAt := strings.Index(source, test.cleanupUse)
			if bootstrapAt < 0 || gateAt < 0 || cleanupAt < 0 || bootstrapAt > gateAt || gateAt > cleanupAt {
				t.Fatalf("%s startup order must be storage bootstrap -> identity backfill gate -> cleanup runtime", test.path)
			}
		})
	}
}

type legacyStorageResolverStub struct {
	resolved domainstorageconfig.ResolvedConfig
	err      error
}

func (s legacyStorageResolverStub) ResolveLegacyByDriver(context.Context, string) (domainstorageconfig.ResolvedConfig, error) {
	return s.resolved, s.err
}

type startupStorageRouter struct{ ref storage.BackendRef }

func (r startupStorageRouter) DefaultWriter(context.Context) (storage.BackendRef, error) {
	return r.ref, nil
}

func (r startupStorageRouter) BackendFor(context.Context, string, string) (storage.BackendRef, error) {
	return r.ref, nil
}

func (r startupStorageRouter) ReadableBackends(context.Context) ([]storage.BackendRef, error) {
	return []storage.BackendRef{r.ref}, nil
}

type startupStorageListingBackend struct {
	object  storage.ObjectInfo
	deleted []string
}

func (*startupStorageListingBackend) Driver() string { return "local" }
func (*startupStorageListingBackend) Put(context.Context, string, string, []byte) error {
	return nil
}
func (*startupStorageListingBackend) Get(context.Context, string) ([]byte, error) {
	return nil, storage.ErrNotFound
}
func (b *startupStorageListingBackend) Delete(_ context.Context, objectKey string) error {
	b.deleted = append(b.deleted, objectKey)
	return nil
}
func (b *startupStorageListingBackend) ListObjects(_ context.Context, prefix, _ string, _ int) (storage.ObjectPage, error) {
	if prefix == "generated-images/" {
		return storage.ObjectPage{Objects: []storage.ObjectInfo{b.object}}, nil
	}
	return storage.ObjectPage{}, nil
}
