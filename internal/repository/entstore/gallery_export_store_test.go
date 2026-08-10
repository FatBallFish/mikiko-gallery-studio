package entstore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"

	domaincleanup "github.com/fatballfish/pic-gallery/internal/domain/objectcleanup"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/galleryexportjob"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/objectdeletionjob"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	galleryexportservice "github.com/fatballfish/pic-gallery/internal/service/galleryexport"
)

func TestGalleryExportStoreAuthorizesEveryAssetWithinUserAndSourceProject(t *testing.T) {
	client := openGalleryExportTestClient(t)
	projectID := uuid.New()
	foreignProjectID := uuid.New()
	seedGalleryExportProject(t, client, projectID, 7, "owned")
	seedGalleryExportProject(t, client, foreignProjectID, 7, "other")
	one := seedGalleryExportImage(t, client, projectID, 7, "same", 12)
	two := seedGalleryExportImage(t, client, projectID, 7, "same", 15)
	foreignProject := seedGalleryExportImage(t, client, foreignProjectID, 7, "foreign", 20)
	foreignUser := seedGalleryExportImage(t, client, projectID, 8, "foreign-user", 20)
	store := NewGalleryExportStore(client)

	assets, err := store.AuthorizeAssets(t.Context(), 7, projectID.String(), []string{two.String(), one.String()})
	if err != nil {
		t.Fatalf("authorize assets: %v", err)
	}
	if len(assets) != 2 || assets[0].ID != two.String() || assets[1].ID != one.String() {
		t.Fatalf("authorized assets must preserve explicit order: %#v", assets)
	}
	for _, ids := range [][]string{{one.String(), foreignProject.String()}, {one.String(), foreignUser.String()}, {one.String(), uuid.NewString()}} {
		if _, err := store.AuthorizeAssets(t.Context(), 7, projectID.String(), ids); !errors.Is(err, repoerr.ErrNotFound) {
			t.Fatalf("authorization error for %#v = %v, want opaque not found", ids, err)
		}
	}
}

func TestGalleryExportStoreCreatesAndClaimsDurableJobWithLease(t *testing.T) {
	client := openGalleryExportTestClient(t)
	projectID := uuid.New()
	seedGalleryExportProject(t, client, projectID, 7, "owned")
	store := NewGalleryExportStore(client)
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	deadline := now.Add(20 * time.Minute)
	created, err := store.CreateJob(t.Context(), galleryexportservice.CreateJobRequest{
		UserID: 7, ProjectID: projectID.String(), ImageIDs: []string{"one", "two"}, EstimatedBytes: 27, LifecycleDeadlineAt: deadline,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if created.State != galleryexportservice.StateQueued || created.EstimatedBytes != 27 {
		t.Fatalf("created job = %#v", created)
	}

	persistedCreated, err := client.GalleryExportJob.Get(t.Context(), uuid.MustParse(created.ID))
	if err != nil || persistedCreated.LifecycleDeadlineAt == nil || !persistedCreated.LifecycleDeadlineAt.Equal(deadline) {
		t.Fatalf("persisted lifecycle deadline=%v err=%v", persistedCreated.LifecycleDeadlineAt, err)
	}
	payload, err := json.Marshal(created)
	if err != nil || !strings.Contains(string(payload), `"deadline_at":"2026-08-09T10:20:00Z"`) {
		t.Fatalf("job JSON=%s err=%v", payload, err)
	}

	claimed, ok, err := store.AcquireNextJob(t.Context(), "worker-1", now, time.Minute)
	if err != nil || !ok {
		t.Fatalf("acquire job: ok=%v err=%v", ok, err)
	}
	if claimed.ID != created.ID || claimed.State != galleryexportservice.StateRunning || claimed.AttemptCount != 1 || claimed.LeaseOwner != "worker-1" {
		t.Fatalf("claimed job = %#v", claimed)
	}
	if _, ok, err := store.AcquireNextJob(t.Context(), "worker-2", now, time.Minute); err != nil || ok {
		t.Fatalf("leased job was claimed twice: ok=%v err=%v", ok, err)
	}
	if renewed, err := store.RenewJobLease(t.Context(), claimed.ID, "worker-2", claimed.AttemptCount, now.Add(10*time.Second), 2*time.Minute); err != nil || renewed {
		t.Fatalf("foreign lease renewal renewed=%v err=%v", renewed, err)
	}
	if renewed, err := store.RenewJobLease(t.Context(), claimed.ID, "worker-1", claimed.AttemptCount, now.Add(10*time.Second), 2*time.Minute); err != nil || !renewed {
		t.Fatalf("owned lease renewal renewed=%v err=%v", renewed, err)
	}
	persisted, err := client.GalleryExportJob.Query().Where(galleryexportjob.IDEQ(uuid.MustParse(claimed.ID))).Only(t.Context())
	if err != nil || persisted.LeaseExpiresAt == nil || !persisted.LeaseExpiresAt.Equal(now.Add(130*time.Second)) {
		t.Fatalf("renewed lease=%v err=%v", persisted.LeaseExpiresAt, err)
	}
	if persisted.LifecycleDeadlineAt == nil || !persisted.LifecycleDeadlineAt.Equal(deadline) || persisted.ExpiresAt != nil {
		t.Fatalf("heartbeat changed lifecycle=%v archive expiry=%v", persisted.LifecycleDeadlineAt, persisted.ExpiresAt)
	}
}

func TestGalleryExportStoreRetriesWithinLifecycleAndStopsBeyondDeadline(t *testing.T) {
	client := openGalleryExportTestClient(t)
	projectID := uuid.New()
	seedGalleryExportProject(t, client, projectID, 7, "owned")
	store := NewGalleryExportStore(client)
	start := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	deadline := start.Add(20 * time.Minute)
	created, err := store.CreateJob(t.Context(), galleryexportservice.CreateJobRequest{UserID: 7, ProjectID: projectID.String(), ImageIDs: []string{"one"}, LifecycleDeadlineAt: deadline})
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		claimed, ok, err := store.AcquireNextJob(t.Context(), "worker-1", start, time.Minute)
		if err != nil || !ok || claimed.AttemptCount != attempt {
			t.Fatalf("claim attempt %d: job=%#v ok=%v err=%v", attempt, claimed, ok, err)
		}
		if err := store.FailJob(t.Context(), galleryexportservice.FailJobRequest{
			Job: claimed, FailedAt: start, Disposition: galleryexportservice.FailureRetryable,
			Code: "authorization_failed", Message: "selected assets could not be authorized",
		}); err != nil {
			t.Fatalf("fail attempt %d: %v", attempt, err)
		}
		if attempt == 1 {
			persisted, err := client.GalleryExportJob.Get(t.Context(), uuid.MustParse(claimed.ID))
			if err != nil || persisted.State != galleryexportservice.StateQueued || persisted.NextAttemptAt == nil || !persisted.NextAttemptAt.Equal(start.Add(time.Minute)) {
				t.Fatalf("retryable attempt persisted=%#v err=%v", persisted, err)
			}
		}
		start = start.Add(time.Duration(attempt) * time.Minute)
	}
	third, ok, err := store.AcquireNextJob(t.Context(), "worker-1", start, time.Minute)
	if err != nil || !ok || third.AttemptCount != 3 || third.DeadlineAt == nil || !third.DeadlineAt.Equal(deadline) {
		t.Fatalf("third claim before deadline: job=%#v ok=%v err=%v", third, ok, err)
	}
	if err := store.FailJob(t.Context(), galleryexportservice.FailJobRequest{
		Job: third, FailedAt: start, Disposition: galleryexportservice.FailureRetryable,
		Code: "authorization_failed", Message: "selected assets could not be authorized",
	}); err != nil {
		t.Fatal(err)
	}
	maxed, err := store.GetJob(t.Context(), 7, created.ID, start)
	if err != nil || maxed.State != galleryexportservice.StateFailed || maxed.ErrorCode != "authorization_failed" || maxed.ErrorMessage != "selected assets could not be authorized" {
		t.Fatalf("max-attempt authorization job=%#v err=%v", maxed, err)
	}
	maxedEntity, err := client.GalleryExportJob.Get(t.Context(), uuid.MustParse(created.ID))
	if err != nil || maxedEntity.NextAttemptAt != nil {
		t.Fatalf("max-attempt next_attempt_at=%v err=%v", maxedEntity.NextAttemptAt, err)
	}

	shortDeadline := start.Add(30 * time.Second)
	short, err := store.CreateJob(t.Context(), galleryexportservice.CreateJobRequest{UserID: 7, ProjectID: projectID.String(), ImageIDs: []string{"two"}, LifecycleDeadlineAt: shortDeadline})
	if err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := store.AcquireNextJob(t.Context(), "worker-2", start, time.Minute)
	if err != nil || !ok || claimed.ID != short.ID {
		t.Fatalf("short claim=%#v ok=%v err=%v", claimed, ok, err)
	}
	if err := store.FailJob(t.Context(), galleryexportservice.FailJobRequest{
		Job: claimed, FailedAt: start, Disposition: galleryexportservice.FailureRetryable,
		Code: "authorization_failed", Message: "selected assets could not be authorized",
	}); err != nil {
		t.Fatal(err)
	}
	terminal, err := store.GetJob(t.Context(), 7, short.ID, start)
	if err != nil || terminal.State != galleryexportservice.StateFailed || terminal.ErrorCode != galleryexportservice.ErrorLifecycleDeadlineExceeded {
		t.Fatalf("terminal retry job=%#v err=%v", terminal, err)
	}
	if _, ok, err := store.AcquireNextJob(t.Context(), "worker-3", shortDeadline.Add(time.Second), time.Minute); err != nil || ok {
		t.Fatalf("claim beyond lifecycle ok=%v err=%v", ok, err)
	}
	_ = created
}

func TestGalleryExportStoreTerminalFailurePreservesStableErrorWithoutRetry(t *testing.T) {
	client := openGalleryExportTestClient(t)
	projectID := uuid.New()
	seedGalleryExportProject(t, client, projectID, 7, "owned")
	store := NewGalleryExportStore(client)
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	created, err := store.CreateJob(t.Context(), galleryexportservice.CreateJobRequest{
		UserID: 7, ProjectID: projectID.String(), ImageIDs: []string{"one"}, LifecycleDeadlineAt: now.Add(30 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := store.AcquireNextJob(t.Context(), "worker-1", now, time.Minute)
	if err != nil || !ok || claimed.AttemptCount != 1 {
		t.Fatalf("claim terminal job=%#v ok=%v err=%v", claimed, ok, err)
	}
	const message = "gallery export exceeds the configured size limit"
	if err := store.FailJob(t.Context(), galleryexportservice.FailJobRequest{
		Job: claimed, FailedAt: now, Disposition: galleryexportservice.FailureTerminal,
		Code: galleryexportservice.ErrorExportTooLarge, Message: message,
	}); err != nil {
		t.Fatal(err)
	}
	persisted, err := client.GalleryExportJob.Get(t.Context(), uuid.MustParse(created.ID))
	if err != nil {
		t.Fatal(err)
	}
	if persisted.State != galleryexportservice.StateFailed || persisted.NextAttemptAt != nil {
		t.Fatalf("terminal state=%q next_attempt_at=%v", persisted.State, persisted.NextAttemptAt)
	}
	if persisted.LastErrorCode == nil || *persisted.LastErrorCode != galleryexportservice.ErrorExportTooLarge || persisted.LastErrorMessage == nil || *persisted.LastErrorMessage != message {
		t.Fatalf("terminal error code=%v message=%v", persisted.LastErrorCode, persisted.LastErrorMessage)
	}
	if _, ok, err := store.AcquireNextJob(t.Context(), "worker-2", now.Add(time.Second), time.Minute); err != nil || ok {
		t.Fatalf("terminal job was claimable again: ok=%v err=%v", ok, err)
	}
}

func TestGalleryExportStoreTerminalizesQueuedJobAtImmutableLifecycleDeadline(t *testing.T) {
	client := openGalleryExportTestClient(t)
	projectID := uuid.New()
	seedGalleryExportProject(t, client, projectID, 7, "owned")
	store := NewGalleryExportStore(client)
	deadline := time.Date(2026, 8, 9, 10, 20, 0, 0, time.UTC)
	created, err := store.CreateJob(t.Context(), galleryexportservice.CreateJobRequest{
		UserID: 7, ProjectID: projectID.String(), ImageIDs: []string{"one"}, LifecycleDeadlineAt: deadline,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.AcquireNextJob(t.Context(), "worker-1", deadline, time.Minute); err != nil || ok {
		t.Fatalf("claim at lifecycle deadline ok=%v err=%v", ok, err)
	}
	terminal, err := store.GetJob(t.Context(), 7, created.ID, deadline)
	if err != nil || terminal.State != galleryexportservice.StateFailed || terminal.ErrorCode != galleryexportservice.ErrorLifecycleDeadlineExceeded {
		t.Fatalf("terminal queued job=%#v err=%v", terminal, err)
	}
	if terminal.DeadlineAt == nil || !terminal.DeadlineAt.Equal(deadline) {
		t.Fatalf("terminal job changed lifecycle deadline: %v", terminal.DeadlineAt)
	}
}

func TestGalleryExportCompletionAndExpiryDriveTransactionalArchiveCleanup(t *testing.T) {
	client := openGalleryExportTestClient(t)
	projectID := uuid.New()
	seedGalleryExportProject(t, client, projectID, 7, "owned")
	store := NewGalleryExportStore(client)
	now := time.Now().UTC()
	created, err := store.CreateJob(t.Context(), galleryexportservice.CreateJobRequest{UserID: 7, ProjectID: projectID.String(), ImageIDs: []string{"one"}, LifecycleDeadlineAt: now.Add(20 * time.Minute)})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	claimed, ok, err := store.AcquireNextJob(t.Context(), "worker-1", now, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim job: ok=%v err=%v", ok, err)
	}
	expiresAt := now.Add(time.Hour)
	objectKey := "gallery-exports/7/" + created.ID + ".zip"
	completed, err := store.CompleteJob(t.Context(), galleryexportservice.CompleteJobRequest{
		JobID: claimed.ID, Owner: "worker-1", AttemptCount: claimed.AttemptCount,
		StorageDriver: "local", ObjectKey: objectKey, ArchiveSizeBytes: 123, ExpiresAt: expiresAt, CompletedAt: now,
	})
	if err != nil || completed.State != galleryexportservice.StateSucceeded {
		t.Fatalf("complete job: %#v err=%v", completed, err)
	}
	jobs, err := client.ObjectDeletionJob.Query().Where(objectdeletionjob.ObjectKeyEQ(objectKey)).All(t.Context())
	if err != nil || len(jobs) != 1 || jobs[0].State != domaincleanup.StatePending {
		t.Fatalf("cleanup outbox after completion = %#v err=%v", jobs, err)
	}
	cleanupStore := NewObjectCleanupStore(client)
	identity := domaincleanup.Identity{StorageDriver: "local", ObjectKey: objectKey}
	if live, err := cleanupStore.HasLiveReferences(t.Context(), identity); err != nil || !live {
		t.Fatalf("completed unexpired archive live=%v err=%v", live, err)
	}

	expired, err := store.ExpireReady(t.Context(), expiresAt.Add(time.Second), 10)
	if err != nil || expired != 1 {
		t.Fatalf("expire jobs=%d err=%v", expired, err)
	}
	if live, err := cleanupStore.HasLiveReferences(t.Context(), identity); err != nil || live {
		t.Fatalf("expired archive live=%v err=%v", live, err)
	}
	job, err := client.ObjectDeletionJob.Query().Where(objectdeletionjob.ObjectKeyEQ(objectKey)).Only(t.Context())
	if err != nil || job.State != domaincleanup.StatePending {
		t.Fatalf("cleanup outbox after expiry = %#v err=%v", job, err)
	}
}

func openGalleryExportTestClient(t *testing.T) *repoent.Client {
	t.Helper()
	client, err := repoent.Open(dialect.SQLite, "file:gallery-export-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	if err := client.Schema.Create(context.Background()); err != nil {
		client.Close()
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func seedGalleryExportProject(t *testing.T, client *repoent.Client, id uuid.UUID, userID int64, name string) {
	t.Helper()
	if _, err := client.Project.Create().SetID(id).SetUserID(userID).SetName(name).SetNameKey(name).Save(t.Context()); err != nil {
		t.Fatalf("seed project: %v", err)
	}
}

func seedGalleryExportImage(t *testing.T, client *repoent.Client, projectID uuid.UUID, userID int64, group string, size int64) uuid.UUID {
	t.Helper()
	taskID := uuid.New()
	if _, err := client.ImageTask.Create().SetID(taskID).SetUserID(userID).SetProjectID(projectID).SetTaskType("text_to_image").SetPrompt("prompt").SetAbstractModel("plus").Save(t.Context()); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	imageID := uuid.New()
	if _, err := client.ImageResult.Create().SetID(imageID).SetTaskID(taskID).SetUserID(userID).SetProjectID(projectID).
		SetObjectKey("generated-images/" + imageID.String() + "/same.png").SetMimeType("image/png").SetFileSizeBytes(size).
		SetSha256("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa").SetImageGroup(group).Save(t.Context()); err != nil {
		t.Fatalf("seed image: %v", err)
	}
	return imageID
}
