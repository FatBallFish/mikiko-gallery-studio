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
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imageresult"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/objectdeletionjob"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/referenceasset"
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

func TestStartupStorageIdentityBackfillMigratesEveryLegacyDriver(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, fmt.Sprintf("file:startup-storage-drivers-%s?mode=memory&cache=shared&_fk=1", uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	task, err := client.ImageTask.Create().SetUserID(903).SetTaskType("text_to_image").SetPrompt("drivers").SetAbstractModel("plus").
		SetArtifactRecoveryStatus("pending").SetArtifactStorageDriver("s3").SetArtifactObjectKeys([]string{"generated-images/recovery.png"}).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ImageResult.Create().SetTaskID(task.ID).SetUserID(task.UserID).SetStorageDriver("").
		SetObjectKey("generated-images/local.png").SetMimeType("image/png").SetSha256("local").Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReferenceAsset.Create().SetUserID(task.UserID).SetStatus("ready").SetStorageDriver("s3").
		SetObjectKey("reference-assets/s3.png").SetMimeType("image/png").SetSha256("s3").Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ObjectDeletionJob.Create().SetStorageDriver("local").SetObjectKey("generated-images/job.png").SetState(domaincleanup.StatePending).Save(ctx); err != nil {
		t.Fatal(err)
	}
	localID, s3ID := uuid.New(), uuid.New()
	resolver := &legacyStorageResolverMap{resolved: map[string]domainstorageconfig.ResolvedConfig{
		"local": {ConfigRecord: domainstorageconfig.ConfigRecord{ID: localID.String(), Driver: "local"}},
		"s3":    {ConfigRecord: domainstorageconfig.ConfigRecord{ID: s3ID.String(), Driver: "s3"}},
	}}

	progress, err := backfillLegacyStorageIdentitiesAtStartup(ctx, client, resolver, "local", db.LegacyStorageIdentityBackfillOptions{BatchSize: 1, MaxBatches: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(progress) != 2 || strings.Join(resolver.calls, ",") != "local,s3" {
		t.Fatalf("progress=%#v resolver calls=%v", progress, resolver.calls)
	}
	result, err := client.ImageResult.Query().Where(imageresult.ObjectKeyEQ("generated-images/local.png")).Only(ctx)
	if err != nil || result.StorageConfigID == nil || *result.StorageConfigID != localID {
		t.Fatalf("local result=%#v err=%v", result, err)
	}
	asset, err := client.ReferenceAsset.Query().Where(referenceasset.ObjectKeyEQ("reference-assets/s3.png")).Only(ctx)
	if err != nil || asset.StorageConfigID == nil || *asset.StorageConfigID != s3ID {
		t.Fatalf("s3 asset=%#v err=%v", asset, err)
	}
	recovery, err := client.ImageTask.Get(ctx, task.ID)
	if err != nil || recovery.ArtifactStorageConfigID == nil || *recovery.ArtifactStorageConfigID != s3ID {
		t.Fatalf("s3 recovery=%#v err=%v", recovery, err)
	}
}

func TestStartupStorageIdentityBackfillArmsCurrentDriverWithoutLegacyRows(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, fmt.Sprintf("file:startup-storage-empty-cutover-%s?mode=memory&cache=shared&_fk=1", uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	configID := uuid.New()
	resolver := &legacyStorageResolverMap{resolved: map[string]domainstorageconfig.ResolvedConfig{
		"local": {ConfigRecord: domainstorageconfig.ConfigRecord{ID: configID.String(), Driver: "local"}},
	}}

	progress, err := backfillLegacyStorageIdentitiesAtStartup(
		t.Context(), client, resolver, "local", db.LegacyStorageIdentityBackfillOptions{},
	)
	if err != nil || len(progress) != 1 || !progress["local"].Completed {
		t.Fatalf("empty startup cutover progress=%#v err=%v", progress, err)
	}
	if _, err := client.ObjectDeletionJob.Create().
		SetStorageDriver("local").
		SetObjectKey("generated-images/first-old-write.png").
		SetState(domaincleanup.StatePending).
		Save(t.Context()); err == nil {
		t.Fatal("startup cutover allowed first nil-config write for the current storage driver")
	}
}

func TestStartupStorageIdentityBackfillArmsReadableHistoricalBootstrapDrivers(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, fmt.Sprintf("file:startup-storage-history-cutover-%s?mode=memory&cache=shared&_fk=1", uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	resolver := &legacyStorageResolverMap{
		resolved: map[string]domainstorageconfig.ResolvedConfig{
			"local": {ConfigRecord: domainstorageconfig.ConfigRecord{ID: uuid.NewString(), Driver: "local"}},
			"s3":    {ConfigRecord: domainstorageconfig.ConfigRecord{ID: uuid.NewString(), Driver: "s3"}},
		},
		legacyDrivers: []string{"s3"},
	}

	progress, err := backfillLegacyStorageIdentitiesAtStartup(
		t.Context(), client, resolver, "local", db.LegacyStorageIdentityBackfillOptions{},
	)
	if err != nil || len(progress) != 2 || strings.Join(resolver.calls, ",") != "local,s3" {
		t.Fatalf("historical startup cutover progress=%#v calls=%v err=%v", progress, resolver.calls, err)
	}
	for _, driver := range []string{"local", "s3"} {
		if _, err := client.ObjectDeletionJob.Create().
			SetStorageDriver(driver).
			SetObjectKey("generated-images/late-" + driver + ".png").
			SetState(domaincleanup.StatePending).
			Save(t.Context()); err == nil {
			t.Fatalf("startup cutover allowed first nil-config %s write", driver)
		}
	}
}

func TestStartupStorageIdentityBackfillResolvesAllDriversBeforeMutation(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, fmt.Sprintf("file:startup-storage-unresolved-%s?mode=memory&cache=shared&_fk=1", uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	task, err := client.ImageTask.Create().SetUserID(904).SetTaskType("text_to_image").SetPrompt("unresolved").SetAbstractModel("plus").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	localResult, err := client.ImageResult.Create().SetTaskID(task.ID).SetUserID(task.UserID).SetStorageDriver("local").
		SetObjectKey("generated-images/local-before-failure.png").SetMimeType("image/png").SetSha256("local-before-failure").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReferenceAsset.Create().SetUserID(task.UserID).SetStatus("ready").SetStorageDriver("archive").
		SetObjectKey("reference-assets/archive.png").SetMimeType("image/png").SetSha256("archive").Save(ctx); err != nil {
		t.Fatal(err)
	}
	localID := uuid.New()
	resolver := &legacyStorageResolverMap{resolved: map[string]domainstorageconfig.ResolvedConfig{
		"local": {ConfigRecord: domainstorageconfig.ConfigRecord{ID: localID.String(), Driver: "local"}},
	}}

	if _, err := backfillLegacyStorageIdentitiesAtStartup(ctx, client, resolver, "local", db.LegacyStorageIdentityBackfillOptions{}); err == nil {
		t.Fatal("unresolved historical driver must fail startup gate")
	}
	if count, err := client.MigrationCheckpoint.Query().Count(ctx); err != nil || count != 0 {
		t.Fatalf("startup installed cutover state before resolving all drivers: count=%d err=%v", count, err)
	}
	localResult, err = client.ImageResult.Get(ctx, localResult.ID)
	if err != nil || localResult.StorageConfigID != nil {
		t.Fatalf("resolvable row mutated before all drivers resolved: %#v, %v", localResult, err)
	}
}

func TestLegacyClaimCannotDeleteAfterStorageIdentityBackfill(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, fmt.Sprintf("file:startup-storage-stale-claim-%s?mode=memory&cache=shared&_fk=1", uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	objectKey := "generated-images/stale-backfill.png"
	task, err := client.ImageTask.Create().SetUserID(905).SetTaskType("text_to_image").SetPrompt("stale claim").SetAbstractModel("plus").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ImageResult.Create().SetTaskID(task.ID).SetUserID(task.UserID).SetStorageDriver("local").
		SetObjectKey(objectKey).SetMimeType("image/png").SetSha256("stale-backfill").Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ObjectDeletionJob.Create().SetStorageDriver("local").SetObjectKey(objectKey).
		SetState(domaincleanup.StatePending).Save(ctx); err != nil {
		t.Fatal(err)
	}
	cleanupStore := entstore.NewObjectCleanupStore(client)
	claim, ok, err := cleanupStore.Claim(ctx, time.Now().UTC())
	if err != nil || !ok {
		t.Fatalf("Claim()=%#v ok=%v err=%v", claim, ok, err)
	}
	configID := uuid.New()
	if _, err := db.RunLegacyStorageIdentityBackfill(ctx, client, "local", configID, db.LegacyStorageIdentityBackfillOptions{BatchSize: 1, MaxBatches: 20}); err != nil {
		t.Fatal(err)
	}

	deleteCalled := false
	blocked, err := cleanupStore.DeleteIfUnreferenced(ctx, claim, func(domaincleanup.Identity) error {
		deleteCalled = true
		return nil
	})
	if !errors.Is(err, domaincleanup.ErrStaleClaim) || blocked || deleteCalled {
		t.Fatalf("stale completion blocked=%v deleteCalled=%v err=%v", blocked, deleteCalled, err)
	}
	result, err := client.ImageResult.Query().Where(imageresult.ObjectKeyEQ(objectKey)).Only(ctx)
	if err != nil || result.StorageConfigID == nil || *result.StorageConfigID != configID {
		t.Fatalf("configured live result=%#v err=%v", result, err)
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

type legacyStorageResolverMap struct {
	resolved      map[string]domainstorageconfig.ResolvedConfig
	legacyDrivers []string
	calls         []string
}

func (s *legacyStorageResolverMap) ListLegacyDrivers(context.Context) ([]string, error) {
	return append([]string(nil), s.legacyDrivers...), nil
}

func (s *legacyStorageResolverMap) ResolveLegacyByDriver(_ context.Context, driver string) (domainstorageconfig.ResolvedConfig, error) {
	driver = strings.ToLower(strings.TrimSpace(driver))
	s.calls = append(s.calls, driver)
	resolved, ok := s.resolved[driver]
	if !ok {
		return domainstorageconfig.ResolvedConfig{}, fmt.Errorf("legacy driver %s is unavailable", driver)
	}
	return resolved, nil
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
