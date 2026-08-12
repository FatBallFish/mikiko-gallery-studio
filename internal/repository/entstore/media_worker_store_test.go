package entstore

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaderivative"
	mediaworker "github.com/fatballfish/pic-gallery/internal/worker/media"
)

func TestMediaWorkerStoreClaimsAndCompletesJobAtomically(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:media-worker-store?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := client.Project.Create().SetUserID(501).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assetID := uuid.New()
	if _, err := client.MediaAsset.Create().SetID(assetID).SetUserID(501).SetProjectID(project.ID).SetName("clip.mp4").SetNameKey("clip.mp4").
		SetMediaType("video").SetSourceType("local_upload").SetStatus("processing").SetStorageDriver("local").
		SetObjectKey("media/original/501/clip.mp4").SetMimeType("video/mp4").SetFileSizeBytes(1024).Save(ctx); err != nil {
		t.Fatal(err)
	}
	job, err := client.MediaProcessingJob.Create().SetAssetID(assetID).SetJobType("probe").SetTransformVersion(1).SetStatus("pending").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}

	store := NewMediaWorkerStore(client)
	now := time.Date(2026, 8, 12, 17, 0, 0, 0, time.UTC)
	claimed, ok, err := store.ClaimDue(ctx, mediaworker.ClaimRequest{Owner: "media-a", Now: now, LeaseTTL: time.Minute})
	if err != nil || !ok || claimed.JobID != job.ID.String() || claimed.AssetID != assetID.String() || claimed.AttemptCount != 1 {
		t.Fatalf("ClaimDue = %#v ok=%v err=%v", claimed, ok, err)
	}
	if _, ok, err := store.ClaimDue(ctx, mediaworker.ClaimRequest{Owner: "media-b", Now: now, LeaseTTL: time.Minute}); err != nil || ok {
		t.Fatalf("duplicate claim ok=%v err=%v", ok, err)
	}
	completed, err := store.Complete(ctx, mediaworker.CompleteRequest{JobID: job.ID.String(), Owner: "media-a", Now: now.Add(time.Second), Result: mediaworker.ProcessResult{
		Probe:       mediaworker.ProbeMetadata{Format: "mp4", Container: "mp4", VideoCodec: "h264", Width: 1280, Height: 720, DurationMS: 5000},
		Derivatives: []mediaworker.Derivative{{Kind: "proxy", TransformVersion: 1, StorageDriver: "local", ObjectKey: "media/derivatives/501/" + assetID.String() + "/proxy.mp4", MIMEType: "video/mp4", SizeBytes: 512, SHA256: "abc"}},
	}})
	if err != nil || !completed {
		t.Fatalf("Complete = %v, %v", completed, err)
	}
	asset, err := client.MediaAsset.Get(ctx, assetID)
	if err != nil || asset.Status != "ready" || asset.Container != "mp4" || asset.Codec != "h264" || asset.Width == nil || *asset.Width != 1280 {
		t.Fatalf("completed asset = %#v err=%v", asset, err)
	}
	derivatives, err := client.MediaDerivative.Query().All(ctx)
	if err != nil || len(derivatives) != 1 || derivatives[0].ObjectKey == "" {
		t.Fatalf("derivatives=%#v err=%v", derivatives, err)
	}
	completedAgain, err := store.Complete(ctx, mediaworker.CompleteRequest{JobID: job.ID.String(), Owner: "media-a", Now: now.Add(2 * time.Second)})
	if err != nil || completedAgain {
		t.Fatalf("idempotent Complete = %v, %v", completedAgain, err)
	}
}

func TestMediaWorkerStoreProbeBackfillsMissingMeteredVideoUsage(t *testing.T) {
	ctx, client := openVideoTaskStoreTestClient(t, "media-worker-metered-backfill")
	project, err := client.Project.Create().SetUserID(504).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	taskID := uuid.New()
	if _, err := client.VideoTask.Create().SetID(taskID).SetUserID(504).SetProjectID(project.ID).
		SetTaskType("text_to_video").SetPromptTemplate("move").SetPromptBindingSnapshot(map[string]any{}).SetExecutionPrompt("move").
		SetRouteModelID(1).SetRouteModelCode("cinema").SetDurationSeconds(5).SetResolution("720p").SetAspectRatio("16:9").
		SetEstimatedPoints("11.00000").SetReservedPoints("12.65000").SetPricingSnapshot(map[string]any{
		"unit_points": "11.00000", "reference_image_count": 0,
		"sales_rule": map[string]any{"pricing_mode": "metered", "fixed_task_points": "1.00000", "output_second_points": "2.00000", "reserve_markup": "1.15000"},
	}).SetRoutingSnapshot(map[string]any{}).SetIdempotencyKey("metered-probe").SetRequestFingerprint("metered-probe").Save(ctx); err != nil {
		t.Fatal(err)
	}
	assetID, itemID := uuid.New(), uuid.New()
	if _, err := client.VideoTaskItem.Create().SetID(itemID).SetTaskID(taskID).SetOrdinal(0).SetStatus(string(domainvideo.ItemStateSucceeded)).SetStage("succeeded").SetResultAssetID(assetID).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.MediaAsset.Create().SetID(assetID).SetUserID(504).SetProjectID(project.ID).SetName("clip.mp4").SetNameKey("clip.mp4").
		SetMediaType("video").SetSourceType("generated").SetStatus("ready_original").SetStorageDriver("local").
		SetObjectKey("media/original/504/clip.mp4").SetMimeType("video/mp4").SetFileSizeBytes(1024).SetSourceTaskKind("video").SetSourceTaskID(taskID).Save(ctx); err != nil {
		t.Fatal(err)
	}
	job, err := client.MediaProcessingJob.Create().SetAssetID(assetID).SetJobType("probe").SetStatus("running").SetAttemptCount(1).
		SetLeaseOwner("media-metered").SetLeaseExpiresAt(time.Now().UTC().Add(time.Minute)).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := NewMediaWorkerStore(client).Complete(ctx, mediaworker.CompleteRequest{JobID: job.ID.String(), Owner: "media-metered", Now: time.Now().UTC(), Result: mediaworker.ProcessResult{
		Probe: mediaworker.ProbeMetadata{Container: "mp4", VideoCodec: "h264", DurationMS: 3_500},
	}})
	if err != nil || !completed {
		t.Fatalf("Complete()=%v err=%v", completed, err)
	}
	item, err := client.VideoTaskItem.Get(ctx, itemID)
	if err != nil || item.ActualOutputSeconds != "3.500" || item.ActualPoints != "8.00000" {
		t.Fatalf("metered item=%#v err=%v", item, err)
	}
}

func TestMediaWorkerStoreClaimsPendingJobPostgres(t *testing.T) {
	ctx, _, client, _ := openProjectTaskPostgres(t)
	project, err := client.Project.Create().SetUserID(503).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assetID := uuid.New()
	if _, err := client.MediaAsset.Create().SetID(assetID).SetUserID(503).SetProjectID(project.ID).SetName("clip.mp4").SetNameKey("clip.mp4").
		SetMediaType("video").SetSourceType("local_upload").SetStatus("processing").SetStorageDriver("local").
		SetObjectKey("media/original/503/clip.mp4").SetMimeType("video/mp4").SetFileSizeBytes(1024).Save(ctx); err != nil {
		t.Fatal(err)
	}
	job, err := client.MediaProcessingJob.Create().SetAssetID(assetID).SetJobType("probe").SetTransformVersion(1).SetStatus("pending").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 8, 12, 17, 15, 0, 0, time.UTC)
	claimed, ok, err := NewMediaWorkerStore(client).ClaimDue(ctx, mediaworker.ClaimRequest{Owner: "media-postgres", Now: now, LeaseTTL: time.Minute})
	if err != nil || !ok || claimed.JobID != job.ID.String() || claimed.AssetID != assetID.String() || claimed.AttemptCount != 1 {
		t.Fatalf("ClaimDue = %#v ok=%v err=%v", claimed, ok, err)
	}
	stored, err := client.MediaProcessingJob.Get(ctx, job.ID)
	if err != nil || stored.Status != "running" || stored.AttemptCount != 1 || stored.LeaseOwner == nil || *stored.LeaseOwner != "media-postgres" {
		t.Fatalf("stored media processing job = %#v err=%v", stored, err)
	}
}

func TestMediaWorkerStoreRetriesExpiredLeaseWithoutRegeneratingJob(t *testing.T) {
	ctx, client, assetID := seedMediaWorkerStore(t, "media-worker-retry")
	store := NewMediaWorkerStore(client)
	now := time.Date(2026, 8, 12, 17, 30, 0, 0, time.UTC)
	claimed, ok, err := store.ClaimDue(ctx, mediaworker.ClaimRequest{Owner: "media-a", Now: now, LeaseTTL: time.Minute})
	if err != nil || !ok {
		t.Fatalf("ClaimDue = %#v %v %v", claimed, ok, err)
	}
	if err := store.Fail(ctx, mediaworker.FailRequest{JobID: claimed.JobID, Owner: "media-a", Now: now, RetryAt: now.Add(time.Minute), ErrorCode: "media_processing_failed", ErrorMessage: "media processing failed"}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.ClaimDue(ctx, mediaworker.ClaimRequest{Owner: "media-b", Now: now.Add(30 * time.Second), LeaseTTL: time.Minute}); err != nil || ok {
		t.Fatalf("early retry=%v err=%v", ok, err)
	}
	retry, ok, err := store.ClaimDue(ctx, mediaworker.ClaimRequest{Owner: "media-b", Now: now.Add(2 * time.Minute), LeaseTTL: time.Minute})
	if err != nil || !ok || retry.AssetID != assetID.String() || retry.AttemptCount != 2 {
		t.Fatalf("retry=%#v ok=%v err=%v", retry, ok, err)
	}
}

func TestMediaWorkerStoreCompleteRollsBackAssetAndDerivativesWhenFinalLeaseUpdateLoses(t *testing.T) {
	ctx, client, assetID := seedMediaWorkerStore(t, "media-worker-complete-conflict")
	store := NewMediaWorkerStore(client)
	now := time.Date(2026, 8, 12, 18, 0, 0, 0, time.UTC)
	claimed, ok, err := store.ClaimDue(ctx, mediaworker.ClaimRequest{Owner: "media-a", Now: now, LeaseTTL: time.Minute})
	if err != nil || !ok {
		t.Fatalf("ClaimDue = %#v %v %v", claimed, ok, err)
	}
	store.beforeFinalUpdate = func(tx *repoent.Tx) error {
		_, updateErr := tx.MediaProcessingJob.UpdateOneID(uuid.MustParse(claimed.JobID)).SetLeaseOwner("media-b").Save(ctx)
		return updateErr
	}
	completed, err := store.Complete(ctx, mediaworker.CompleteRequest{JobID: claimed.JobID, Owner: "media-a", Now: now.Add(time.Second), Result: mediaworker.ProcessResult{
		Probe:       mediaworker.ProbeMetadata{Container: "mp4", VideoCodec: "h264", Width: 1280},
		Derivatives: []mediaworker.Derivative{{Kind: "proxy", TransformVersion: 1, StorageDriver: "local", ObjectKey: "media/derivatives/502/" + assetID.String() + "/proxy.mp4", MIMEType: "video/mp4", SizeBytes: 512}},
	}})
	if err != nil || completed {
		t.Fatalf("Complete = %v, %v", completed, err)
	}
	asset, err := client.MediaAsset.Get(ctx, assetID)
	if err != nil || asset.Status != "processing" || asset.Width != nil {
		t.Fatalf("asset changed after lease conflict: %#v err=%v", asset, err)
	}
	count, err := client.MediaDerivative.Query().Where(mediaderivative.AssetIDEQ(assetID)).Count(ctx)
	if err != nil || count != 0 {
		t.Fatalf("derivatives committed after lease conflict: count=%d err=%v", count, err)
	}
}

func seedMediaWorkerStore(t *testing.T, name string) (context.Context, *repoent.Client, uuid.UUID) {
	t.Helper()
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:"+name+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := client.Project.Create().SetUserID(502).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assetID := uuid.New()
	if _, err := client.MediaAsset.Create().SetID(assetID).SetUserID(502).SetProjectID(project.ID).SetName("image.png").SetNameKey("image.png").SetMediaType("image").SetSourceType("upload").SetStatus("processing").SetStorageDriver("local").SetObjectKey("media/original/502/image.png").SetMimeType("image/png").SetFileSizeBytes(100).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.MediaProcessingJob.Create().SetAssetID(assetID).SetJobType("probe").SetStatus("pending").Save(ctx); err != nil {
		t.Fatal(err)
	}
	return ctx, client, assetID
}
