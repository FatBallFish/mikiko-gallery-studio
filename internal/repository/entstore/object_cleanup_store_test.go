package entstore

import (
	"fmt"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	domaincleanup "github.com/fatballfish/pic-gallery/internal/domain/objectcleanup"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/objectdeletionjob"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

func TestObjectCleanupStoreStaleWorkerTransitionDoesNotOverrideReenqueue(t *testing.T) {
	tests := []struct {
		name       string
		transition func(*ObjectCleanupStore, string) error
	}{
		{
			name: "done",
			transition: func(store *ObjectCleanupStore, id string) error {
				return store.MarkDone(t.Context(), id)
			},
		},
		{
			name: "blocked",
			transition: func(store *ObjectCleanupStore, id string) error {
				return store.MarkBlocked(t.Context(), id, "live_reference")
			},
		},
		{
			name: "retry",
			transition: func(store *ObjectCleanupStore, id string) error {
				return store.MarkRetry(t.Context(), id, time.Now().Add(time.Minute), "delete_failed", "delete failed")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := repoent.Open(dialect.SQLite, "file:cleanup-transition-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			if err := client.Schema.Create(t.Context()); err != nil {
				t.Fatal(err)
			}

			store := NewObjectCleanupStore(client)
			identity := domaincleanup.Identity{StorageDriver: "local", ObjectKey: "generated/shared.png"}
			first, err := store.Enqueue(t.Context(), identity)
			if err != nil {
				t.Fatal(err)
			}
			if _, claimed, err := store.Claim(t.Context(), time.Now()); err != nil || !claimed {
				t.Fatalf("Claim() claimed=%v err=%v", claimed, err)
			}

			second, err := store.Enqueue(t.Context(), identity)
			if err != nil {
				t.Fatal(err)
			}
			if first.ID != second.ID {
				t.Fatalf("Enqueue() created duplicate job: first=%q second=%q", first.ID, second.ID)
			}
			if err := test.transition(store, first.ID); err != nil {
				t.Fatal(err)
			}

			job, err := client.ObjectDeletionJob.Query().Only(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if job.State != domaincleanup.StatePending {
				t.Fatalf("job.State=%q, want %q", job.State, domaincleanup.StatePending)
			}
			if job.CompletedAt != nil || job.NextAttemptAt != nil || job.LastErrorCode != nil || job.LastErrorMessage != nil {
				t.Fatalf("stale transition metadata persisted: %#v", job)
			}
		})
	}
}

func TestArtifactRecoveryReferenceMatchesCanonicalObjectTuple(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, "file:cleanup-recovery-tuple-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}

	configID := uuid.New()
	if _, err := client.ImageTask.Create().
		SetUserID(91).
		SetTaskType("text_to_image").
		SetPrompt("recovery").
		SetAbstractModel("plus").
		SetArtifactRecoveryStatus("pending").
		SetArtifactStorageConfigID(configID).
		SetArtifactStorageDriver("s3").
		SetArtifactStorageBucket("owned-bucket").
		SetArtifactObjectKeys([]string{"generated-images/91/r.png"}).
		Save(t.Context()); err != nil {
		t.Fatal(err)
	}

	other := domaincleanup.Identity{
		StorageConfigID: configID.String(),
		StorageDriver:   "s3",
		Bucket:          "owned-bucket",
		ObjectKey:       "generated-images/91/x.png",
	}
	if live, err := hasLiveObjectReferences(t.Context(), client, other); err != nil || live {
		t.Fatalf("unrelated object live=%v err=%v", live, err)
	}

	recovery := other
	recovery.StorageDriver = "renamed-driver"
	recovery.Bucket = "renamed-bucket"
	recovery.ObjectKey = "generated-images/91/r.png"
	if live, err := hasLiveObjectReferences(t.Context(), client, recovery); err != nil || !live {
		t.Fatalf("recovery object live=%v err=%v", live, err)
	}

	differentConfig := recovery
	differentConfig.StorageConfigID = uuid.NewString()
	if live, err := hasLiveObjectReferences(t.Context(), client, differentConfig); err != nil || live {
		t.Fatalf("different config object live=%v err=%v", live, err)
	}
}

func TestObjectCleanupStoreConfiguredIdentityUsesConfigAndObjectKey(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, "file:cleanup-configured-identity-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}

	store := NewObjectCleanupStore(client)
	configID := uuid.NewString()
	first, err := store.Enqueue(t.Context(), domaincleanup.Identity{
		StorageConfigID: configID,
		StorageDriver:   "s3",
		Bucket:          "old-bucket",
		ObjectKey:       "generated-images/91/shared.png",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Enqueue(t.Context(), domaincleanup.Identity{
		StorageConfigID: configID,
		StorageDriver:   "renamed-driver",
		Bucket:          "new-bucket",
		ObjectKey:       "generated-images/91/shared.png",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("configured identity created duplicate jobs: first=%#v second=%#v", first, second)
	}
	if count, err := client.ObjectDeletionJob.Query().Count(t.Context()); err != nil || count != 1 {
		t.Fatalf("job count=%d err=%v", count, err)
	}
}

func TestObjectCleanupStoreEnqueueAfterDoneCreatesNewPendingJob(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, "file:cleanup-done-aba-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}

	store := NewObjectCleanupStore(client)
	identity := domaincleanup.Identity{StorageDriver: "local", ObjectKey: "generated/reused.png"}
	first, err := store.Enqueue(t.Context(), identity)
	if err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := store.Claim(t.Context(), time.Now()); err != nil || !claimed {
		t.Fatalf("Claim() claimed=%v err=%v", claimed, err)
	}
	if err := store.MarkDone(t.Context(), first.ID); err != nil {
		t.Fatal(err)
	}

	second, err := store.Enqueue(t.Context(), identity)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID || second.State != domaincleanup.StatePending {
		t.Fatalf("second=%#v first=%#v", second, first)
	}
	if count, err := client.ObjectDeletionJob.Query().Count(t.Context()); err != nil || count != 2 {
		t.Fatalf("job count=%d err=%v", count, err)
	}
	claimed, ok, err := store.Claim(t.Context(), time.Now().Add(time.Second))
	if err != nil || !ok || claimed.ID != second.ID {
		t.Fatalf("second Claim()=%#v ok=%v err=%v", claimed, ok, err)
	}
	if err := store.MarkDone(t.Context(), second.ID); err != nil {
		t.Fatal(err)
	}
	completed, err := client.ObjectDeletionJob.Get(t.Context(), uuid.MustParse(second.ID))
	if err != nil || completed.State != domaincleanup.StateDone {
		t.Fatalf("completed=%#v err=%v", completed, err)
	}
}

func TestObjectCleanupReconcileUsesFairKeysetAndSkipsCompletedDeletion(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, "file:cleanup-reconcile-keyset-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}

	deletedAt := time.Now().UTC().Add(-time.Hour)
	task, err := client.ImageTask.Create().
		SetUserID(92).
		SetTaskType("text_to_image").
		SetPrompt("deleted results").
		SetAbstractModel("plus").
		Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for index := range 5 {
		if _, err := client.ImageResult.Create().
			SetTaskID(task.ID).
			SetUserID(92).
			SetStorageDriver("local").
			SetObjectKey(fmt.Sprintf("generated-images/92/result-%d.png", index)).
			SetMimeType("image/png").
			SetSha256(fmt.Sprintf("hash-%d", index)).
			SetDeletedAt(deletedAt.Add(time.Duration(index) * time.Second)).
			Save(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := client.ReferenceAsset.Create().
		SetUserID(92).
		SetStatus("deleted").
		SetStorageDriver("local").
		SetObjectKey("reference-assets/deleted.png").
		SetMimeType("image/png").
		SetSha256("reference-hash").
		SetDeletedAt(deletedAt).
		Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ObjectDeletionJob.Create().
		SetStorageDriver("local").
		SetObjectKey("generated-images/92/result-0.png").
		SetState(domaincleanup.StateDone).
		SetCompletedAt(deletedAt.Add(time.Minute)).
		Save(t.Context()); err != nil {
		t.Fatal(err)
	}

	store := NewObjectCleanupStore(client)
	for range 6 {
		if _, err := store.Reconcile(t.Context(), 2); err != nil {
			t.Fatal(err)
		}
	}
	for _, objectKey := range []string{
		"reference-assets/deleted.png",
		"generated-images/92/result-1.png",
		"generated-images/92/result-2.png",
		"generated-images/92/result-3.png",
		"generated-images/92/result-4.png",
	} {
		if exists, err := client.ObjectDeletionJob.Query().Where(objectdeletionjob.ObjectKeyEQ(objectKey)).Exist(t.Context()); err != nil || !exists {
			t.Fatalf("cleanup job for %q exists=%v err=%v", objectKey, exists, err)
		}
	}
	if count, err := client.ObjectDeletionJob.Query().Where(objectdeletionjob.ObjectKeyEQ("generated-images/92/result-0.png")).Count(t.Context()); err != nil || count != 1 {
		t.Fatalf("completed cleanup history count=%d err=%v", count, err)
	}
}

func TestObjectCleanupReconcileRestartSkipsCompletedFrontPage(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, "file:cleanup-reconcile-restart-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}

	deletedAt := time.Now().UTC().Add(-time.Hour)
	task, err := client.ImageTask.Create().SetUserID(95).SetTaskType("text_to_image").SetPrompt("restart sweep").SetAbstractModel("plus").Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for index := range 5 {
		key := fmt.Sprintf("generated-images/95/result-%d.png", index)
		if _, err := client.ImageResult.Create().SetTaskID(task.ID).SetUserID(95).SetStorageDriver("local").SetObjectKey(key).
			SetMimeType("image/png").SetSha256(fmt.Sprintf("restart-%d", index)).SetDeletedAt(deletedAt.Add(time.Duration(index) * time.Second)).Save(t.Context()); err != nil {
			t.Fatal(err)
		}
		if index < 4 {
			if _, err := client.ObjectDeletionJob.Create().SetStorageDriver("local").SetObjectKey(key).SetState(domaincleanup.StateDone).
				SetCompletedAt(deletedAt.Add(time.Minute)).Save(t.Context()); err != nil {
				t.Fatal(err)
			}
		}
	}

	for range 2 {
		store := NewObjectCleanupStore(client)
		if _, err := store.Reconcile(t.Context(), 1); err != nil {
			t.Fatal(err)
		}
	}
	if exists, err := client.ObjectDeletionJob.Query().Where(objectdeletionjob.ObjectKeyEQ("generated-images/95/result-4.png")).Exist(t.Context()); err != nil || !exists {
		t.Fatalf("orphan after completed front page exists=%v err=%v", exists, err)
	}
}

func TestObjectCleanupCheckpointCompareAndSwapRejectsStaleCursor(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, "file:cleanup-checkpoint-cas-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	store := NewObjectCleanupStore(client)
	createdAt := time.Date(2026, time.August, 9, 3, 4, 5, 0, time.UTC)
	created := domaincleanup.ReconcileCheckpoint{
		StorageIdentity: "config-a", Namespace: "sha256:namespace", Prefix: "generated-images/", Cursor: "cursor-a", UpdatedAt: createdAt,
	}
	if saved, err := store.SaveReconcileCheckpoint(t.Context(), created, time.Time{}); err != nil || !saved {
		t.Fatalf("create checkpoint saved=%v err=%v", saved, err)
	}
	if saved, err := store.SaveReconcileCheckpoint(t.Context(), created, time.Time{}); err != nil || saved {
		t.Fatalf("duplicate create must lose CAS saved=%v err=%v", saved, err)
	}
	advanced := created
	advanced.Cursor = "cursor-b"
	advanced.UpdatedAt = createdAt.Add(time.Microsecond)
	if saved, err := store.SaveReconcileCheckpoint(t.Context(), advanced, createdAt); err != nil || !saved {
		t.Fatalf("advance checkpoint saved=%v err=%v", saved, err)
	}
	stale := created
	stale.Cursor = "stale-cursor"
	stale.UpdatedAt = createdAt.Add(2 * time.Microsecond)
	if saved, err := store.SaveReconcileCheckpoint(t.Context(), stale, createdAt); err != nil || saved {
		t.Fatalf("stale checkpoint must lose CAS saved=%v err=%v", saved, err)
	}
	checkpoint, ok, err := store.GetReconcileCheckpoint(t.Context(), created.StorageIdentity, created.Prefix)
	if err != nil || !ok || checkpoint.Cursor != advanced.Cursor || !checkpoint.UpdatedAt.Equal(advanced.UpdatedAt) {
		t.Fatalf("checkpoint=%#v ok=%v err=%v", checkpoint, ok, err)
	}
}

func TestObjectCleanupReconcileMigratesLegacyArtifactRecoveriesExplicitly(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, "file:cleanup-recovery-migration-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}

	configID := uuid.New()
	backfillable, err := client.ImageTask.Create().
		SetUserID(93).
		SetTaskType("text_to_image").
		SetPrompt("backfillable").
		SetAbstractModel("plus").
		SetArtifactRecoveryStatus("pending").
		SetArtifactStorageConfigID(configID).
		Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ImageResult.Create().
		SetTaskID(backfillable.ID).
		SetUserID(93).
		SetStorageConfigID(configID).
		SetStorageDriver("s3").
		SetObjectKey("generated-images/93/recovered.png").
		SetMimeType("image/png").
		SetSha256("backfill-hash").
		Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	unrecoverable, err := client.ImageTask.Create().
		SetUserID(93).
		SetTaskType("text_to_image").
		SetPrompt("unrecoverable").
		SetAbstractModel("plus").
		SetArtifactRecoveryStatus("pending").
		SetArtifactStorageConfigID(configID).
		Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	conservative, err := client.ImageTask.Create().
		SetUserID(93).
		SetTaskType("text_to_image").
		SetPrompt("encrypted legacy").
		SetAbstractModel("plus").
		SetArtifactRecoveryStatus("pending").
		SetArtifactRecoveryPayload("encrypted-envelope").
		SetArtifactStorageConfigID(configID).
		Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	store := NewObjectCleanupStore(client)
	if _, err := store.Reconcile(t.Context(), 10); err != nil {
		t.Fatal(err)
	}
	backfilled, err := client.ImageTask.Get(t.Context(), backfillable.ID)
	if err != nil {
		t.Fatal(err)
	}
	if backfilled.ArtifactStorageDriver != "s3" || len(backfilled.ArtifactObjectKeys) != 1 || backfilled.ArtifactObjectKeys[0] != "generated-images/93/recovered.png" {
		t.Fatalf("backfilled recovery=%#v", backfilled)
	}
	terminal, err := client.ImageTask.Get(t.Context(), unrecoverable.ID)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.ArtifactRecoveryStatus != "unrecoverable" || terminal.ArtifactLastDiagnostic["code"] != "artifact_recovery_identity_unavailable" {
		t.Fatalf("terminal recovery=%#v", terminal)
	}
	stillPending, err := client.ImageTask.Get(t.Context(), conservative.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stillPending.ArtifactRecoveryStatus != "pending" || len(stillPending.ArtifactObjectKeys) != 0 {
		t.Fatalf("conservative recovery=%#v", stillPending)
	}
	identity := domaincleanup.Identity{StorageConfigID: configID.String(), StorageDriver: "s3", ObjectKey: "generated-images/93/unrelated.png"}
	if live, err := hasLiveObjectReferences(t.Context(), client, identity); err != nil || !live {
		t.Fatalf("legacy encrypted recovery must remain conservative: live=%v err=%v", live, err)
	}
}

func TestObjectCleanupReconcileLegacyRecoveryRestartSkipsConservativeRows(t *testing.T) {
	tests := []struct {
		name string
		seed func(*testing.T, *repoent.Client, time.Time) *repoent.ImageTask
		want func(*testing.T, *repoent.ImageTask)
	}{
		{
			name: "backfillable",
			seed: func(t *testing.T, client *repoent.Client, updatedAt time.Time) *repoent.ImageTask {
				task, err := client.ImageTask.Create().SetUserID(94).SetTaskType("text_to_image").SetPrompt("backfillable target").
					SetAbstractModel("plus").SetArtifactRecoveryStatus("pending").SetArtifactRecoveryPayload("encrypted-envelope").
					SetUpdatedAt(updatedAt).Save(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if _, err := client.ImageResult.Create().SetTaskID(task.ID).SetUserID(task.UserID).SetStorageDriver("local").
					SetObjectKey("generated-images/94/backfillable.png").SetMimeType("image/png").SetSha256("backfillable").Save(t.Context()); err != nil {
					t.Fatal(err)
				}
				return task
			},
			want: func(t *testing.T, task *repoent.ImageTask) {
				if len(task.ArtifactObjectKeys) != 1 || task.ArtifactObjectKeys[0] != "generated-images/94/backfillable.png" {
					t.Fatalf("backfillable target=%#v", task)
				}
			},
		},
		{
			name: "unrecoverable",
			seed: func(t *testing.T, client *repoent.Client, updatedAt time.Time) *repoent.ImageTask {
				task, err := client.ImageTask.Create().SetUserID(94).SetTaskType("text_to_image").SetPrompt("unrecoverable target").
					SetAbstractModel("plus").SetArtifactRecoveryStatus("pending").SetArtifactRecoveryPayload("   ").
					SetArtifactObjectKeys([]string{}).SetUpdatedAt(updatedAt).Save(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				return task
			},
			want: func(t *testing.T, task *repoent.ImageTask) {
				if task.ArtifactRecoveryStatus != "unrecoverable" {
					t.Fatalf("unrecoverable target=%#v", task)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := repoent.Open(dialect.SQLite, "file:cleanup-recovery-fairness-"+test.name+"-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			if err := client.Schema.Create(t.Context()); err != nil {
				t.Fatal(err)
			}

			base := time.Now().UTC().Add(-time.Hour)
			for index := range 100 {
				if _, err := client.ImageTask.Create().SetUserID(94).SetTaskType("text_to_image").
					SetPrompt(fmt.Sprintf("conservative encrypted %d", index)).SetAbstractModel("plus").
					SetArtifactRecoveryStatus("pending").SetArtifactRecoveryPayload("encrypted-envelope").
					SetUpdatedAt(base.Add(time.Duration(index) * time.Second)).Save(t.Context()); err != nil {
					t.Fatal(err)
				}
			}
			target := test.seed(t, client, base.Add(101*time.Second))

			if _, err := NewObjectCleanupStore(client).Reconcile(t.Context(), 1); err != nil {
				t.Fatal(err)
			}
			if _, err := NewObjectCleanupStore(client).Reconcile(t.Context(), 1); err != nil {
				t.Fatal(err)
			}
			updated, err := client.ImageTask.Get(t.Context(), target.ID)
			if err != nil {
				t.Fatal(err)
			}
			test.want(t, updated)
		})
	}
}
