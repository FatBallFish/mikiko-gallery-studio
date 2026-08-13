package entstore

import (
	"context"
	"errors"
	"fmt"
	"strings"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaderivative"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaprocessingjob"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/predicate"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videotaskitem"
	mediaworker "github.com/fatballfish/pic-gallery/internal/worker/media"
)

type MediaWorkerStore struct {
	client            *repoent.Client
	policyResolver    func(context.Context) (domainmedia.Policy, error)
	beforeFinalUpdate func(*repoent.Tx) error
}

var errMediaCompletionConflict = errors.New("media processing completion lease changed")

func NewMediaWorkerStore(client *repoent.Client) *MediaWorkerStore {
	return &MediaWorkerStore{
		client: client,
		policyResolver: func(ctx context.Context) (domainmedia.Policy, error) {
			policy, err := NewAdminVideoStore(client).GetMediaPolicy(ctx)
			if err != nil {
				return domainmedia.Policy{}, err
			}
			return policy.RuntimePolicy().Policy, nil
		},
	}
}

func (store *MediaWorkerStore) ReconcileMediaOnce(ctx context.Context) (bool, error) {
	if store == nil || store.client == nil {
		return false, errors.New("media reconciliation store is unavailable")
	}
	policy := domainmedia.DefaultPolicy()
	if store.policyResolver != nil {
		resolved, err := store.policyResolver(ctx)
		if err != nil {
			return false, fmt.Errorf("resolve media reconciliation policy: %w", err)
		}
		policy = resolved
	}
	return withSerializableTx(ctx, store.client, func(tx *repoent.Tx) (bool, error) {
		asset, err := tx.MediaAsset.Query().Where(
			mediaasset.StatusIn("processing", "ready_original"),
			mediaasset.DeletedAtIsNil(),
			lockMediaAssetForReconcile(),
		).Order(repoent.Asc(mediaasset.FieldUpdatedAt), repoent.Asc(mediaasset.FieldCreatedAt)).First(ctx)
		if repoent.IsNotFound(err) {
			plan := domainmedia.BuildDerivativePlanWithPolicy(domainmedia.MediaTypeImage, policy)
			if len(plan) == 0 {
				return false, nil
			}
			asset, err = generatedImageMissingDerivativesQuery(tx, plan).Where(lockMediaAssetForReconcile()).
				Order(repoent.Asc(mediaasset.FieldUpdatedAt), repoent.Asc(mediaasset.FieldCreatedAt)).First(ctx)
			if repoent.IsNotFound(err) {
				return false, nil
			}
		}
		if err != nil {
			return false, fmt.Errorf("query media asset for reconciliation: %w", err)
		}

		job, err := tx.MediaProcessingJob.Query().Where(
			mediaprocessingjob.AssetIDEQ(asset.ID),
			mediaprocessingjob.JobTypeEQ("probe"),
			mediaprocessingjob.TransformVersionEQ(1),
		).Only(ctx)
		if repoent.IsNotFound(err) {
			if _, err := tx.MediaProcessingJob.Create().SetAssetID(asset.ID).SetJobType("probe").SetTransformVersion(1).SetStatus("pending").Save(ctx); err != nil {
				return false, fmt.Errorf("reconcile missing media processing job: %w", err)
			}
			return true, nil
		}
		if err != nil {
			return false, fmt.Errorf("query media processing job for reconciliation: %w", err)
		}
		if job.Status == "pending" || job.Status == "retry" || job.Status == "running" {
			return false, nil
		}

		plan := domainmedia.BuildDerivativePlanWithPolicy(domainmedia.MediaType(asset.MediaType), policy)
		ready, err := requiredMediaDerivativesReady(ctx, tx, asset.ID, plan)
		if err != nil {
			return false, err
		}
		if ready && job.Status == "succeeded" {
			count, err := tx.MediaAsset.Update().Where(
				mediaasset.IDEQ(asset.ID),
				mediaasset.StatusIn("processing", "ready_original"),
			).SetStatus("ready").ClearProcessingErrorCode().ClearProcessingErrorMessage().AddVersion(1).Save(ctx)
			if err != nil {
				return false, fmt.Errorf("reconcile ready media asset: %w", err)
			}
			return count == 1, nil
		}

		count, err := tx.MediaProcessingJob.Update().Where(
			mediaprocessingjob.IDEQ(job.ID),
			mediaprocessingjob.StatusEQ(job.Status),
		).SetStatus("pending").SetAttemptCount(0).ClearNextRetryAt().ClearLeaseOwner().ClearLeaseExpiresAt().ClearErrorCode().ClearErrorMessage().Save(ctx)
		if err != nil {
			return false, fmt.Errorf("reconcile media processing job: %w", err)
		}
		return count == 1, nil
	})
}

func generatedImageMissingDerivativesQuery(tx *repoent.Tx, plan []domainmedia.DerivativeSpec) *repoent.MediaAssetQuery {
	missing := make([]predicate.MediaAsset, 0, len(plan))
	for _, spec := range plan {
		missing = append(missing, mediaasset.Not(mediaasset.HasDerivativesWith(
			mediaderivative.KindEQ(string(spec.Kind)),
			mediaderivative.TransformVersionEQ(spec.TransformVersion),
			mediaderivative.StatusEQ("ready"),
			mediaderivative.DeletedAtIsNil(),
		)))
	}
	return tx.MediaAsset.Query().Where(
		mediaasset.MediaTypeEQ(string(domainmedia.MediaTypeImage)),
		mediaasset.SourceTypeEQ("generated"),
		mediaasset.StatusEQ("ready"),
		mediaasset.StorageDriverNEQ("remote"),
		mediaasset.Not(mediaasset.ObjectKeyHasPrefix("task:")),
		mediaasset.Not(mediaasset.HasProcessingJobsWith(
			mediaprocessingjob.JobTypeEQ("probe"),
			mediaprocessingjob.TransformVersionEQ(1),
			mediaprocessingjob.StatusIn("pending", "retry", "running"),
		)),
		mediaasset.DeletedAtIsNil(),
		mediaasset.Or(missing...),
	)
}

func lockMediaAssetForReconcile() func(*entsql.Selector) {
	return func(selector *entsql.Selector) {
		if selector.Dialect() == "postgres" {
			selector.ForUpdate(entsql.WithLockAction(entsql.SkipLocked))
		}
	}
}

func requiredMediaDerivativesReady(ctx context.Context, tx *repoent.Tx, assetID uuid.UUID, plan []domainmedia.DerivativeSpec) (bool, error) {
	if len(plan) == 0 {
		return true, nil
	}
	for _, spec := range plan {
		exists, err := tx.MediaDerivative.Query().Where(
			mediaderivative.AssetIDEQ(assetID),
			mediaderivative.KindEQ(string(spec.Kind)),
			mediaderivative.TransformVersionEQ(spec.TransformVersion),
			mediaderivative.StatusEQ("ready"),
			mediaderivative.DeletedAtIsNil(),
		).Exist(ctx)
		if err != nil {
			return false, fmt.Errorf("query required media derivative: %w", err)
		}
		if !exists {
			return false, nil
		}
	}
	return true, nil
}

type mediaClaimResult struct {
	item    mediaworker.WorkItem
	claimed bool
}

func (store *MediaWorkerStore) ClaimDue(ctx context.Context, request mediaworker.ClaimRequest) (mediaworker.WorkItem, bool, error) {
	if store == nil || store.client == nil || strings.TrimSpace(request.Owner) == "" || request.LeaseTTL <= 0 {
		return mediaworker.WorkItem{}, false, errors.New("invalid media worker claim")
	}
	result, err := withSerializableTx(ctx, store.client, func(tx *repoent.Tx) (mediaClaimResult, error) {
		now := request.Now.UTC()
		jobs, err := tx.MediaProcessingJob.Query().Where(
			mediaprocessingjob.Or(
				mediaprocessingjob.And(
					mediaprocessingjob.StatusIn("pending", "retry"),
					mediaprocessingjob.Or(mediaprocessingjob.NextRetryAtIsNil(), mediaprocessingjob.NextRetryAtLTE(now)),
				),
				mediaprocessingjob.And(mediaprocessingjob.StatusEQ("running"), mediaprocessingjob.LeaseExpiresAtLTE(now)),
			),
			mediaprocessingjob.Or(mediaprocessingjob.LeaseOwnerIsNil(), mediaprocessingjob.LeaseExpiresAtIsNil(), mediaprocessingjob.LeaseExpiresAtLTE(now)),
		).Order(repoent.Asc(mediaprocessingjob.FieldNextRetryAt), repoent.Asc(mediaprocessingjob.FieldCreatedAt)).Limit(8).All(ctx)
		if err != nil {
			return mediaClaimResult{}, fmt.Errorf("query due media processing job: %w", err)
		}
		for _, job := range jobs {
			count, err := tx.MediaProcessingJob.Update().Where(
				mediaprocessingjob.IDEQ(job.ID),
				mediaprocessingjob.StatusIn(job.Status),
				mediaprocessingjob.Or(mediaprocessingjob.LeaseOwnerIsNil(), mediaprocessingjob.LeaseExpiresAtIsNil(), mediaprocessingjob.LeaseExpiresAtLTE(now)),
			).SetStatus("running").AddAttemptCount(1).SetLeaseOwner(strings.TrimSpace(request.Owner)).
				SetLeaseExpiresAt(now.Add(request.LeaseTTL)).ClearNextRetryAt().ClearErrorCode().ClearErrorMessage().Save(ctx)
			if err != nil {
				return mediaClaimResult{}, fmt.Errorf("claim media processing job: %w", err)
			}
			if count != 1 {
				continue
			}
			claimed, err := tx.MediaProcessingJob.Query().Where(mediaprocessingjob.IDEQ(job.ID)).WithAsset().Only(ctx)
			if err != nil {
				return mediaClaimResult{}, err
			}
			asset := claimed.Edges.Asset
			if asset == nil {
				return mediaClaimResult{}, errors.New("media processing asset is unavailable")
			}
			item := mediaworker.WorkItem{
				JobID: claimed.ID.String(), AssetID: asset.ID.String(), UserID: asset.UserID, ProjectID: asset.ProjectID.String(),
				MediaType: asset.MediaType, MIMEType: asset.MimeType, SizeBytes: asset.FileSizeBytes,
				StorageDriver: asset.StorageDriver, Bucket: asset.Bucket, ObjectKey: asset.ObjectKey,
				AttemptCount: claimed.AttemptCount, MaxAttempts: claimed.MaxAttempts,
			}
			if asset.StorageConfigID != nil {
				item.StorageConfigID = asset.StorageConfigID.String()
			}
			return mediaClaimResult{item: item, claimed: true}, nil
		}
		return mediaClaimResult{}, nil
	})
	return result.item, result.claimed, err
}

func (store *MediaWorkerStore) Complete(ctx context.Context, request mediaworker.CompleteRequest) (bool, error) {
	jobID, err := uuid.Parse(request.JobID)
	if err != nil {
		return false, fmt.Errorf("invalid media processing job id: %w", err)
	}
	completed, err := withSerializableTx(ctx, store.client, func(tx *repoent.Tx) (bool, error) {
		job, err := tx.MediaProcessingJob.Query().Where(mediaprocessingjob.IDEQ(jobID)).WithAsset().Only(ctx)
		if err != nil {
			return false, err
		}
		if job.Status == "succeeded" {
			return false, nil
		}
		if job.Status != "running" || job.LeaseOwner == nil || *job.LeaseOwner != request.Owner || job.Edges.Asset == nil {
			return false, nil
		}
		asset := job.Edges.Asset
		probe := request.Result.Probe
		assetUpdate := tx.MediaAsset.UpdateOne(asset).SetStatus("ready").SetContainer(probe.Container).SetCodec(probe.VideoCodec).
			SetAudioCodec(probe.AudioCodec).SetProcessedAt(request.Now.UTC()).ClearProcessingErrorCode().ClearProcessingErrorMessage().AddVersion(1)
		if probe.Width > 0 {
			assetUpdate.SetWidth(probe.Width)
		}
		if probe.Height > 0 {
			assetUpdate.SetHeight(probe.Height)
		}
		if probe.DurationMS > 0 {
			assetUpdate.SetDurationMs(probe.DurationMS)
		}
		if probe.FrameRateMilli > 0 {
			assetUpdate.SetFrameRateMilli(probe.FrameRateMilli)
		}
		if probe.Channels > 0 {
			assetUpdate.SetChannels(probe.Channels)
		}
		if probe.SampleRate > 0 {
			assetUpdate.SetSampleRate(probe.SampleRate)
		}
		if _, err := assetUpdate.Save(ctx); err != nil {
			return false, fmt.Errorf("update processed media asset: %w", err)
		}
		if probe.DurationMS > 0 && asset.SourceTaskKind != nil && *asset.SourceTaskKind == "video" {
			item, err := tx.VideoTaskItem.Query().Where(videotaskitem.ResultAssetIDEQ(asset.ID)).WithTask().Only(ctx)
			if err != nil && !repoent.IsNotFound(err) {
				return false, fmt.Errorf("load metered video item for probe usage: %w", err)
			}
			if err == nil {
				seconds := decimal.NewFromInt(probe.DurationMS).Div(decimal.NewFromInt(1000)).StringFixed(3)
				item.ActualOutputSeconds = seconds
				points, pending, priceErr := actualVideoItemPoints(item, item.Edges.Task)
				if priceErr != nil {
					return false, priceErr
				}
				if !pending && item.ActualPoints == "0.00000" {
					if _, err := tx.VideoTaskItem.UpdateOne(item).SetActualOutputSeconds(seconds).SetActualPoints(points).SetStage("succeeded").SetNextActionAt(request.Now.UTC()).Save(ctx); err != nil {
						return false, fmt.Errorf("backfill metered video usage from probe: %w", err)
					}
				}
			}
		}
		for _, derivative := range request.Result.Derivatives {
			exists, err := tx.MediaDerivative.Query().Where(
				mediaderivative.AssetIDEQ(asset.ID), mediaderivative.KindEQ(derivative.Kind), mediaderivative.TransformVersionEQ(derivative.TransformVersion),
			).Exist(ctx)
			if err != nil {
				return false, err
			}
			if exists {
				continue
			}
			builder := tx.MediaDerivative.Create().SetAssetID(asset.ID).SetKind(derivative.Kind).SetTransformVersion(derivative.TransformVersion).
				SetStatus("ready").SetStorageDriver(derivative.StorageDriver).SetBucket(derivative.Bucket).SetObjectKey(derivative.ObjectKey).
				SetMimeType(derivative.MIMEType).SetFileSizeBytes(derivative.SizeBytes).SetSha256(derivative.SHA256)
			if derivative.StorageConfigID != "" {
				storageID, err := uuid.Parse(derivative.StorageConfigID)
				if err != nil {
					return false, fmt.Errorf("invalid derivative storage config id: %w", err)
				}
				builder.SetStorageConfigID(storageID)
			}
			if _, err := builder.Save(ctx); err != nil {
				return false, fmt.Errorf("create media derivative: %w", err)
			}
		}
		if store.beforeFinalUpdate != nil {
			if err := store.beforeFinalUpdate(tx); err != nil {
				return false, err
			}
		}
		count, err := tx.MediaProcessingJob.Update().Where(
			mediaprocessingjob.IDEQ(job.ID), mediaprocessingjob.StatusEQ("running"), mediaprocessingjob.LeaseOwnerEQ(request.Owner),
		).SetStatus("succeeded").ClearLeaseOwner().ClearLeaseExpiresAt().ClearNextRetryAt().ClearErrorCode().ClearErrorMessage().Save(ctx)
		if err != nil {
			return false, err
		}
		if count != 1 {
			return false, errMediaCompletionConflict
		}
		return true, nil
	})
	if errors.Is(err, errMediaCompletionConflict) {
		return false, nil
	}
	return completed, err
}

func (store *MediaWorkerStore) Fail(ctx context.Context, request mediaworker.FailRequest) error {
	jobID, err := uuid.Parse(request.JobID)
	if err != nil {
		return fmt.Errorf("invalid media processing job id: %w", err)
	}
	return withSerializableTxError(ctx, store.client, func(tx *repoent.Tx) error {
		job, err := tx.MediaProcessingJob.Query().Where(mediaprocessingjob.IDEQ(jobID)).WithAsset().Only(ctx)
		if err != nil {
			return err
		}
		if job.Status != "running" || job.LeaseOwner == nil || *job.LeaseOwner != request.Owner {
			return nil
		}
		status := "retry"
		if request.Terminal {
			status = "failed"
		}
		update := tx.MediaProcessingJob.Update().Where(mediaprocessingjob.IDEQ(job.ID), mediaprocessingjob.StatusEQ("running"), mediaprocessingjob.LeaseOwnerEQ(request.Owner)).
			SetStatus(status).SetErrorCode(request.ErrorCode).SetErrorMessage(request.ErrorMessage).ClearLeaseOwner().ClearLeaseExpiresAt()
		if request.Terminal {
			update.ClearNextRetryAt()
		} else {
			update.SetNextRetryAt(request.RetryAt.UTC())
		}
		if _, err := update.Save(ctx); err != nil {
			return err
		}
		if request.Terminal && job.Edges.Asset != nil {
			_, err = tx.MediaAsset.UpdateOne(job.Edges.Asset).SetStatus("processing_failed").SetProcessingErrorCode(request.ErrorCode).
				SetProcessingErrorMessage(request.ErrorMessage).AddVersion(1).Save(ctx)
		}
		return err
	})
}

func (store *MediaWorkerStore) ReleaseLease(ctx context.Context, ref mediaworker.LeaseRef) error {
	jobID, err := uuid.Parse(ref.JobID)
	if err != nil {
		return fmt.Errorf("invalid media processing job id: %w", err)
	}
	_, err = store.client.MediaProcessingJob.Update().Where(mediaprocessingjob.IDEQ(jobID), mediaprocessingjob.LeaseOwnerEQ(ref.Owner)).
		ClearLeaseOwner().ClearLeaseExpiresAt().Save(ctx)
	return err
}

func withSerializableTxError(ctx context.Context, client *repoent.Client, run func(*repoent.Tx) error) error {
	_, err := withSerializableTx(ctx, client, func(tx *repoent.Tx) (struct{}, error) {
		return struct{}{}, run(tx)
	})
	return err
}

var _ mediaworker.Store = (*MediaWorkerStore)(nil)
