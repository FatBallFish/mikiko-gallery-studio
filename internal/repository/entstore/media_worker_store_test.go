package entstore

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"

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
