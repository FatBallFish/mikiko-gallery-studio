package entstore

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"

	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaprocessingjob"
)

func TestMediaWorkerStoreReconcileCreatesOnlyMissingProbeJob(t *testing.T) {
	ctx, client, projectID := mediaReconcileClient(t, "missing-job")
	assetID := uuid.New()
	if _, err := client.MediaAsset.Create().SetID(assetID).SetUserID(801).SetProjectID(projectID).SetName("upload.mp4").SetNameKey("upload.mp4").
		SetMediaType("video").SetSourceType("local_upload").SetStatus("processing").SetStorageDriver("local").
		SetObjectKey("media/original/801/upload.mp4").SetMimeType("video/mp4").SetFileSizeBytes(10).Save(ctx); err != nil {
		t.Fatal(err)
	}

	store := NewMediaWorkerStore(client)
	processed, err := store.ReconcileMediaOnce(ctx)
	if err != nil || !processed {
		t.Fatalf("ReconcileMediaOnce() = %v, %v", processed, err)
	}
	jobs, err := client.MediaProcessingJob.Query().Where(mediaprocessingjob.AssetIDEQ(assetID)).All(ctx)
	if err != nil || len(jobs) != 1 || jobs[0].JobType != "probe" || jobs[0].TransformVersion != 1 || jobs[0].Status != "pending" {
		t.Fatalf("jobs = %#v, %v", jobs, err)
	}
	processed, err = store.ReconcileMediaOnce(ctx)
	if err != nil || processed {
		t.Fatalf("idempotent ReconcileMediaOnce() = %v, %v", processed, err)
	}
}

func TestMediaWorkerStoreReconcileReusesSucceededJobWhenDerivativesAreMissing(t *testing.T) {
	ctx, client, projectID := mediaReconcileClient(t, "missing-derivatives")
	assetID := uuid.New()
	if _, err := client.MediaAsset.Create().SetID(assetID).SetUserID(801).SetProjectID(projectID).SetName("result.mp4").SetNameKey("result.mp4").
		SetMediaType("video").SetSourceType("generated").SetStatus("ready_original").SetStorageDriver("local").
		SetObjectKey("media/original/801/result.mp4").SetMimeType("video/mp4").SetFileSizeBytes(10).Save(ctx); err != nil {
		t.Fatal(err)
	}
	job, err := client.MediaProcessingJob.Create().SetAssetID(assetID).SetJobType("probe").SetTransformVersion(1).SetStatus("succeeded").SetAttemptCount(2).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	plan := domainmedia.BuildDerivativePlan(domainmedia.MediaTypeVideo)
	for _, spec := range plan[:len(plan)-1] {
		if _, err := client.MediaDerivative.Create().SetAssetID(assetID).SetKind(string(spec.Kind)).SetTransformVersion(spec.TransformVersion).
			SetStatus("ready").SetStorageDriver("local").SetObjectKey("media/derivatives/801/" + assetID.String() + "/" + string(spec.Kind)).Save(ctx); err != nil {
			t.Fatal(err)
		}
	}

	processed, err := NewMediaWorkerStore(client).ReconcileMediaOnce(ctx)
	if err != nil || !processed {
		t.Fatalf("ReconcileMediaOnce() = %v, %v", processed, err)
	}
	updated, err := client.MediaProcessingJob.Get(ctx, job.ID)
	if err != nil || updated.Status != "pending" || updated.AttemptCount != 0 || updated.LeaseOwner != nil || updated.LeaseExpiresAt != nil {
		t.Fatalf("reused job = %#v, %v", updated, err)
	}
	count, err := client.MediaProcessingJob.Query().Where(mediaprocessingjob.AssetIDEQ(assetID)).Count(ctx)
	if err != nil || count != 1 {
		t.Fatalf("job count = %d, %v", count, err)
	}
}

func TestMediaWorkerStoreReconcileAdvancesAssetOnlyWhenRequiredDerivativesAreReady(t *testing.T) {
	ctx, client, projectID := mediaReconcileClient(t, "ready-derivatives")
	assetID := uuid.New()
	if _, err := client.MediaAsset.Create().SetID(assetID).SetUserID(801).SetProjectID(projectID).SetName("audio.wav").SetNameKey("audio.wav").
		SetMediaType("audio").SetSourceType("local_upload").SetStatus("processing").SetStorageDriver("local").
		SetObjectKey("media/original/801/audio.wav").SetMimeType("audio/wav").SetFileSizeBytes(10).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.MediaProcessingJob.Create().SetAssetID(assetID).SetJobType("probe").SetTransformVersion(1).SetStatus("succeeded").Save(ctx); err != nil {
		t.Fatal(err)
	}
	for _, spec := range domainmedia.BuildDerivativePlan(domainmedia.MediaTypeAudio) {
		if _, err := client.MediaDerivative.Create().SetAssetID(assetID).SetKind(string(spec.Kind)).SetTransformVersion(spec.TransformVersion).
			SetStatus("ready").SetStorageDriver("local").SetObjectKey("media/derivatives/801/" + assetID.String() + "/" + string(spec.Kind)).Save(ctx); err != nil {
			t.Fatal(err)
		}
	}

	store := NewMediaWorkerStore(client)
	processed, err := store.ReconcileMediaOnce(ctx)
	if err != nil || !processed {
		t.Fatalf("ReconcileMediaOnce() = %v, %v", processed, err)
	}
	asset, err := client.MediaAsset.Get(ctx, assetID)
	if err != nil || asset.Status != "ready" {
		t.Fatalf("asset = %#v, %v", asset, err)
	}
	processed, err = store.ReconcileMediaOnce(ctx)
	if err != nil || processed {
		t.Fatalf("idempotent ReconcileMediaOnce() = %v, %v", processed, err)
	}
}

func mediaReconcileClient(t *testing.T, name string) (context.Context, *repoent.Client, uuid.UUID) {
	t.Helper()
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:media-reconcile-"+name+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := client.Project.Create().SetUserID(801).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, client, project.ID
}
