package entstore

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	providervideo "github.com/fatballfish/pic-gallery/internal/provider/video"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaassetreference"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaprocessingjob"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videoprovidercallbackevent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videoprovidercostrule"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videotaskattempt"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videotaskitem"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	worker "github.com/fatballfish/pic-gallery/internal/worker/video"
)

func TestVideoWorkerStoreClaimsDueItemWithLease(t *testing.T) {
	ctx, client := openVideoTaskStoreTestClient(t, "video-worker-claim")
	store := NewVideoTaskStore(client, NewBillingStore(client, 5))
	now := time.Date(2026, 8, 12, 14, 0, 0, 0, time.UTC)
	_, due := seedVideoWorkerTask(t, ctx, client, 9101, "claim", now.Add(-time.Second))
	_, future := seedVideoWorkerTask(t, ctx, client, 9102, "future", now.Add(time.Minute))

	claimed, ok, err := store.ClaimDue(ctx, worker.ClaimRequest{Owner: "worker-a", Now: now, LeaseTTL: 30 * time.Second})
	if err != nil || !ok || claimed.ID != due.String() || claimed.LeaseOwner != "worker-a" || !claimed.LeaseExpiresAt.Equal(now.Add(30*time.Second)) {
		t.Fatalf("ClaimDue() = %#v ok=%v err=%v", claimed, ok, err)
	}
	if _, ok, err := store.ClaimDue(ctx, worker.ClaimRequest{Owner: "worker-b", Now: now, LeaseTTL: 30 * time.Second}); err != nil || ok {
		t.Fatalf("second ClaimDue() ok=%v err=%v", ok, err)
	}
	if entity, err := client.VideoTaskItem.Get(ctx, future); err != nil || entity.LeaseOwner != nil {
		t.Fatalf("future item changed: %#v err=%v", entity, err)
	}
	if err := store.ReleaseLease(ctx, worker.LeaseRef{ItemID: due.String(), Owner: "worker-a"}); err != nil {
		t.Fatal(err)
	}
	if entity, err := client.VideoTaskItem.Get(ctx, due); err != nil || entity.LeaseOwner != nil || entity.LeaseExpiresAt != nil {
		t.Fatalf("lease not released: %#v err=%v", entity, err)
	}
}

func TestVideoWorkerStoreLoadsInputStorageForTemporaryProviderAccess(t *testing.T) {
	ctx, client := openVideoTaskStoreTestClient(t, "video-worker-input-storage")
	store := NewVideoTaskStore(client, NewBillingStore(client, 5))
	now := time.Date(2026, 8, 12, 14, 15, 0, 0, time.UTC)
	taskID, _ := seedVideoWorkerTask(t, ctx, client, 9104, "input-storage", now)
	task, err := client.VideoTask.Get(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	assetID := uuid.New()
	storageID := uuid.New()
	if _, err := client.MediaAsset.Create().SetID(assetID).SetUserID(task.UserID).SetProjectID(task.ProjectID).
		SetName("first.png").SetNameKey("first.png").SetMediaType("image").SetSourceType("uploaded").SetStatus("ready").
		SetStorageConfigID(storageID).SetStorageDriver("s3").SetObjectKey("media/original/first.png").SetMimeType("image/png").SetFileSizeBytes(123).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.VideoTaskInput.Create().SetTaskID(taskID).SetAssetID(assetID).SetRole("first_frame").SetOrdinal(0).SetAssetSnapshot(map[string]any{}).Save(ctx); err != nil {
		t.Fatal(err)
	}

	claimed, ok, err := store.ClaimDue(ctx, worker.ClaimRequest{Owner: "worker-input", Now: now, LeaseTTL: time.Minute})
	if err != nil || !ok || len(claimed.Request.Inputs) != 1 {
		t.Fatalf("ClaimDue() = %#v ok=%v err=%v", claimed, ok, err)
	}
	input := claimed.Request.Inputs[0]
	if input.StorageConfigID != storageID.String() || input.StorageDriver != "s3" || input.ObjectKey != "media/original/first.png" || input.MIMEType != "image/png" || input.URL != "" {
		t.Fatalf("worker input = %#v", input)
	}
}

func TestVideoWorkerStoreResolvesOnlyVerifiedExactExecutionAccount(t *testing.T) {
	ctx, client := openVideoTaskStoreTestClient(t, "video-worker-execution-account")
	store := NewVideoTaskStore(client, NewBillingStore(client, 5))
	account, err := client.ModelAccount.Create().SetName("seedance-a").SetAdapterType("seedance").SetAuthType("api_key").SetBaseURL("https://ark.example.test").
		SetCredentialsEncrypted(map[string]string{"api_key": "secret", "callback_secret": "callback-secret"}).SetStatus("enabled").SetTimeoutMs(45000).
		SetExtra(map[string]any{"video_callback_url": "https://app.example.test/callback", "video_artifact_hosts": []any{"cdn.seedance.example"}}).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	accountModel, err := client.ModelAccountModel.Create().SetAccountID(int64(account.ID)).SetModelCode("seedance-2-5").SetDisplayName("Seedance 2.5").SetTaskTypes([]string{"text_to_video"}).SetEnabled(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	routeModel, err := client.RouteModel.Create().SetCode("video-exec").SetName("Video Exec").SetMediaType("video").SetEnabled(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := client.RouteModelCandidate.Create().SetRouteModelID(int64(routeModel.ID)).SetAccountModelID(int64(accountModel.ID)).SetEnabled(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.VideoModelCapability.Create().SetAccountModelID(int64(accountModel.ID)).SetCapabilityVersion("cap-v1").SetCapabilityJSON(map[string]any{"schema_version": 1}).SetValidationStatus("verified").SetEnabled(true).Save(ctx); err != nil {
		t.Fatal(err)
	}
	ref := worker.ProviderRef{RouteCandidateID: int64(candidate.ID), AccountModelID: int64(accountModel.ID), ModelAccountID: int64(account.ID), ProviderCode: "seedance", ModelCode: "seedance-2-5"}

	resolved, err := store.GetExecutionAccount(ctx, ref)
	if err != nil || resolved.APIKey != "secret" || resolved.CallbackSecret != "callback-secret" || resolved.Timeout != 45*time.Second || len(resolved.ArtifactAllowedHosts) != 1 || resolved.ArtifactAllowedHosts[0] != "cdn.seedance.example" {
		t.Fatalf("GetExecutionAccount() = %#v err=%v", resolved, err)
	}
	if _, err := store.GetExecutionAccount(ctx, worker.ProviderRef{RouteCandidateID: ref.RouteCandidateID, AccountModelID: ref.AccountModelID, ModelAccountID: ref.ModelAccountID + 1, ProviderCode: ref.ProviderCode, ModelCode: ref.ModelCode}); err == nil {
		t.Fatal("expected mismatched model account rejection")
	}
}

func TestVideoWorkerStoreClaimsTerminalItemThatNeedsSettlement(t *testing.T) {
	ctx, client := openVideoTaskStoreTestClient(t, "video-worker-claim-settlement")
	store := NewVideoTaskStore(client, NewBillingStore(client, 5))
	now := time.Date(2026, 8, 12, 14, 30, 0, 0, time.UTC)
	taskID, itemID := seedVideoWorkerTask(t, ctx, client, 9103, "claim-settlement", now)
	if _, err := client.VideoTaskItem.UpdateOneID(itemID).SetStatus(string(domainvideo.ItemStateFailed)).SetStage("failed").Save(ctx); err != nil {
		t.Fatal(err)
	}

	claimed, ok, err := store.ClaimDue(ctx, worker.ClaimRequest{Owner: "worker-settle", Now: now, LeaseTTL: time.Minute})
	if err != nil || !ok || claimed.TaskID != taskID.String() || !claimed.NeedsSettlement {
		t.Fatalf("settlement ClaimDue() = %#v ok=%v err=%v", claimed, ok, err)
	}
	if _, err := client.VideoTask.UpdateOneID(taskID).SetSettlementStatus("finalized").Save(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.ReleaseLease(ctx, worker.LeaseRef{ItemID: itemID.String(), Owner: "worker-settle"}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.ClaimDue(ctx, worker.ClaimRequest{Owner: "worker-settle", Now: now, LeaseTTL: time.Minute}); err != nil || ok {
		t.Fatalf("finalized terminal item claimed=%v err=%v", ok, err)
	}
}

func TestVideoWorkerStoreProjectsReceivedCallbackBeforeClaim(t *testing.T) {
	ctx, client := openVideoTaskStoreTestClient(t, "video-worker-callback")
	store := NewVideoTaskStore(client, NewBillingStore(client, 5))
	now := time.Date(2026, 8, 12, 15, 0, 0, 0, time.UTC)
	taskID, itemID := seedVideoWorkerTask(t, ctx, client, 9110, "callback", now.Add(time.Hour))
	if _, err := client.VideoTaskItem.UpdateOneID(itemID).SetStatus(string(domainvideo.ItemStateProviderRunning)).SetStage("provider_running").Save(ctx); err != nil {
		t.Fatal(err)
	}
	attemptID := uuid.New()
	if _, err := createVideoWorkerAttempt(client, itemID, attemptID, 1, "job-callback").SetCostSnapshot(map[string]any{
		"currency": "CNY", "cny_exchange_rate": "1.00000", "task_type": "text_to_video", "resolution": "720p", "audio_mode": "silent", "duration_seconds": 5,
		"rates": map[string]any{"combinations": []any{map[string]any{"task_type": "text_to_video", "resolution": "720p", "audio_mode": "silent", "duration_seconds": 5, "cost_cny": "1.25000"}}},
	}).Save(ctx); err != nil {
		t.Fatal(err)
	}
	event, err := client.VideoProviderCallbackEvent.Create().
		SetProviderCode("fake").SetModelAccountID(33).SetProviderEventID("event-callback").SetProviderJobID("job-callback").
		SetStatus("received").SetReceivedAt(now.Add(-time.Second)).SetPayloadSnapshot(map[string]any{
		"job_id": "job-callback", "state": "succeeded",
		"usage":            map[string]any{"output_seconds": float64(5), "total_tokens": float64(1200)},
		"usage_normalized": map[string]any{"output_seconds": "5.000", "provider_tokens": "1200"},
		"provider_status":  map[string]any{"request_id": "callback-request"},
		"artifacts":        []any{map[string]any{"url": "https://media.example.test/final.mp4", "mime_type": "video/mp4", "size_bytes": float64(42), "sha256": "abc"}},
	}).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}

	claimed, ok, err := store.ClaimDue(ctx, worker.ClaimRequest{Owner: "worker-callback", Now: now, LeaseTTL: time.Minute})
	if err != nil || !ok || claimed.ID != itemID.String() || claimed.TaskID != taskID.String() || claimed.State != domainvideo.ItemStateArtifactPending || claimed.Artifact.URL == "" {
		t.Fatalf("callback ClaimDue() = %#v ok=%v err=%v", claimed, ok, err)
	}
	projected, err := client.VideoProviderCallbackEvent.Get(ctx, event.ID)
	if err != nil || projected.Status != "processed" || projected.ProcessedAt == nil {
		t.Fatalf("callback event not processed: %#v err=%v", projected, err)
	}
	attempt, err := client.VideoTaskAttempt.Get(ctx, attemptID)
	item, itemErr := client.VideoTaskItem.Get(ctx, itemID)
	if err != nil || itemErr != nil || attempt.ProviderCost != "1.25000" || item.ProviderCost != "1.25000" || attempt.UsageNormalized["provider_tokens"] != "1200" || item.ActualOutputSeconds != "5.000" {
		t.Fatalf("callback accounting attempt=%#v item=%#v errors=%v/%v", attempt, item, err, itemErr)
	}
}

func TestVideoWorkerStoreDoesNotLetUnmatchedCallbacksStarveValidEvents(t *testing.T) {
	ctx, client := openVideoTaskStoreTestClient(t, "video-worker-callback-starvation")
	store := NewVideoTaskStore(client, NewBillingStore(client, 5))
	now := time.Date(2026, 8, 12, 15, 30, 0, 0, time.UTC)
	for index := 0; index < 16; index++ {
		if _, err := client.VideoProviderCallbackEvent.Create().SetProviderCode("fake").SetModelAccountID(33).
			SetProviderEventID(fmt.Sprintf("orphan-%02d", index)).SetProviderJobID(fmt.Sprintf("missing-%02d", index)).SetStatus("received").
			SetReceivedAt(now.Add(-time.Minute)).SetPayloadSnapshot(map[string]any{"state": "running"}).Save(ctx); err != nil {
			t.Fatal(err)
		}
	}
	_, itemID := seedVideoWorkerTask(t, ctx, client, 9111, "callback-after-orphans", now.Add(time.Hour))
	if _, err := client.VideoTaskItem.UpdateOneID(itemID).SetStatus(string(domainvideo.ItemStateProviderRunning)).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := createVideoWorkerAttempt(client, itemID, uuid.New(), 1, "job-valid-after-orphans").Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.VideoProviderCallbackEvent.Create().SetProviderCode("fake").SetModelAccountID(33).SetProviderEventID("valid-after-orphans").
		SetProviderJobID("job-valid-after-orphans").SetStatus("received").SetReceivedAt(now).SetPayloadSnapshot(map[string]any{"state": "running"}).Save(ctx); err != nil {
		t.Fatal(err)
	}

	if _, _, err := store.ClaimDue(ctx, worker.ClaimRequest{Owner: "worker-callback", Now: now, LeaseTTL: time.Minute}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ClaimDue(ctx, worker.ClaimRequest{Owner: "worker-callback", Now: now, LeaseTTL: time.Minute}); err != nil {
		t.Fatal(err)
	}
	unmatched, err := client.VideoProviderCallbackEvent.Query().Where(videoprovidercallbackevent.StatusEQ("unmatched")).Count(ctx)
	if err != nil || unmatched != 16 {
		t.Fatalf("unmatched callback count=%d err=%v", unmatched, err)
	}
	valid, err := client.VideoProviderCallbackEvent.Query().Where(videoprovidercallbackevent.ProviderEventIDEQ("valid-after-orphans")).Only(ctx)
	if err != nil || valid.Status != "processed" {
		t.Fatalf("valid callback = %#v err=%v", valid, err)
	}
}

func TestVideoWorkerStorePreparesAndAppliesMonotonicSteps(t *testing.T) {
	ctx, client := openVideoTaskStoreTestClient(t, "video-worker-steps")
	store := NewVideoTaskStore(client, NewBillingStore(client, 5))
	now := time.Date(2026, 8, 12, 16, 0, 0, 0, time.UTC)
	_, itemID := seedVideoWorkerTask(t, ctx, client, 9120, "steps", now)
	claimed, ok, err := store.ClaimDue(ctx, worker.ClaimRequest{Owner: "worker-steps", Now: now, LeaseTTL: time.Minute})
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	attemptID := uuid.New()
	prepared, err := store.PrepareAttempt(ctx, worker.PrepareAttemptRequest{
		ItemID: itemID.String(), Owner: "worker-steps", ExpectedVersion: claimed.Version,
		AttemptID: attemptID.String(), ProviderIdempotencyKey: "task:item:attempt",
	})
	if err != nil || prepared.State != domainvideo.ItemStateSubmitting || prepared.Attempt.ID != attemptID.String() || prepared.Attempt.No != 1 || prepared.Attempt.ProviderCode != "fake" {
		t.Fatalf("PrepareAttempt() = %#v err=%v", prepared, err)
	}
	next := now.Add(5 * time.Second)
	changed, err := store.ApplyStep(ctx, worker.ApplyStepRequest{
		ItemID: itemID.String(), Owner: "worker-steps", ExpectedVersion: prepared.Version,
		Target: domainvideo.ItemStateProviderQueued, ProviderJobID: "job-steps", AttemptStatus: "provider_queued", NextActionAt: &next,
		ProviderStatusSnapshot: map[string]any{"request_id": "provider-request"},
		UsageRaw:               map[string]any{"output_seconds": 5, "total_tokens": 1200},
		UsageNormalized:        providervideo.Usage{OutputSeconds: "5.000", ProviderTokens: "1200"},
	})
	if err != nil || !changed {
		t.Fatalf("ApplyStep() changed=%v err=%v", changed, err)
	}
	if changed, err := store.ApplyStep(ctx, worker.ApplyStepRequest{ItemID: itemID.String(), Owner: "worker-steps", ExpectedVersion: prepared.Version, Target: domainvideo.ItemStateFailed}); err != nil || changed {
		t.Fatalf("stale ApplyStep() changed=%v err=%v", changed, err)
	}
	entity, err := client.VideoTaskItem.Get(ctx, itemID)
	if err != nil || entity.Status != string(domainvideo.ItemStateProviderQueued) || entity.LeaseOwner != nil || entity.NextActionAt == nil || entity.ActualOutputSeconds != "5.000" {
		t.Fatalf("applied item=%#v err=%v", entity, err)
	}
	attempt, err := client.VideoTaskAttempt.Query().Where(videotaskattempt.IDEQ(attemptID)).Only(ctx)
	if err != nil || attempt.ProviderJobID == nil || *attempt.ProviderJobID != "job-steps" || attempt.Status != "provider_queued" ||
		attempt.ProviderStatusSnapshot["request_id"] != "provider-request" || attempt.UsageRaw["total_tokens"] != float64(1200) || attempt.UsageNormalized["provider_tokens"] != "1200" {
		t.Fatalf("applied attempt=%#v err=%v", attempt, err)
	}

	if _, err := client.VideoTaskItem.UpdateOneID(itemID).SetLeaseOwner("worker-steps").SetLeaseExpiresAt(now.Add(time.Minute)).Save(ctx); err != nil {
		t.Fatal(err)
	}
	current, _ := client.VideoTaskItem.Get(ctx, itemID)
	artifact := providervideo.Artifact{URL: "https://media.example.test/retry.mp4", MIMEType: "video/mp4", SizeBytes: 42, SHA256: "abc"}
	changed, err = store.ApplyStep(ctx, worker.ApplyStepRequest{
		ItemID: itemID.String(), Owner: "worker-steps", ExpectedVersion: current.Version,
		Target: domainvideo.ItemStateFailed, AttemptStatus: "failed", Artifact: artifact,
		IncrementArtifactAttempts: true, ArtifactExhausted: true, PlatformAbsorbed: true,
	})
	if err == nil || changed {
		t.Fatalf("invalid exhausted transition changed=%v err=%v", changed, err)
	}
}

func TestVideoWorkerStoreCommitArtifactCreatesResultReferenceAndProbeJob(t *testing.T) {
	ctx, client := openVideoTaskStoreTestClient(t, "video-worker-artifact-lifecycle")
	store := NewVideoTaskStore(client, NewBillingStore(client, 5))
	now := time.Date(2026, 8, 12, 16, 20, 0, 0, time.UTC)
	taskID, itemID := seedVideoWorkerTask(t, ctx, client, 9122, "artifact-lifecycle", now)
	if _, err := client.VideoTask.UpdateOneID(taskID).SetPricingSnapshot(map[string]any{
		"unit_points": "11.00000", "max_reserved_points": "12.65000",
		"sales_rule": map[string]any{"pricing_mode": "metered", "fixed_task_points": "1.00000", "output_second_points": "2.00000", "reserve_markup": "1.15000"},
	}).SetReservedPoints("12.65000").Save(ctx); err != nil {
		t.Fatal(err)
	}
	item, err := client.VideoTaskItem.UpdateOneID(itemID).SetStatus(string(domainvideo.ItemStateArtifactPending)).SetActualOutputSeconds("3.000").SetVersion(4).SetLeaseOwner("worker-artifact").SetLeaseExpiresAt(now.Add(time.Minute)).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assetID := uuid.New()
	committed, err := store.CommitArtifact(ctx, worker.ArtifactCommitRequest{
		ItemID: itemID.String(), Owner: "worker-artifact", ExpectedVersion: item.Version, AssetID: assetID.String(), UserID: 9122,
		ProjectID: mustVideoWorkerTask(t, ctx, client, taskID).ProjectID.String(), Status: "ready_original", StorageDriver: "local",
		ObjectKey: "media/original/9122/result.mp4", MIMEType: "video/mp4", SizeBytes: 1024, SHA256: strings.Repeat("a", 64),
	})
	if err != nil || !committed {
		t.Fatalf("CommitArtifact() committed=%v err=%v", committed, err)
	}
	if count, err := client.MediaAssetReference.Query().Where(mediaassetreference.AssetIDEQ(assetID), mediaassetreference.RefTypeEQ("video_task_result"), mediaassetreference.RefIDEQ(taskID), mediaassetreference.DeletedAtIsNil()).Count(ctx); err != nil || count != 1 {
		t.Fatalf("result references=%d err=%v", count, err)
	}
	if count, err := client.MediaProcessingJob.Query().Where(mediaprocessingjob.AssetIDEQ(assetID), mediaprocessingjob.JobTypeEQ("probe"), mediaprocessingjob.StatusEQ("pending")).Count(ctx); err != nil || count != 1 {
		t.Fatalf("probe jobs=%d err=%v", count, err)
	}
	storedItem, err := client.VideoTaskItem.Get(ctx, itemID)
	if err != nil || storedItem.ActualPoints != "7.00000" {
		t.Fatalf("metered actual points=%q err=%v", storedItem.ActualPoints, err)
	}
}

func TestVideoWorkerStoreMeteredArtifactWaitsForProbeWithoutUsage(t *testing.T) {
	ctx, client := openVideoTaskStoreTestClient(t, "video-worker-metered-missing-usage")
	store := NewVideoTaskStore(client, NewBillingStore(client, 5))
	now := time.Date(2026, 8, 12, 16, 25, 0, 0, time.UTC)
	taskID, itemID := seedVideoWorkerTask(t, ctx, client, 9123, "metered-missing-usage", now)
	if _, err := client.VideoTask.UpdateOneID(taskID).SetPricingSnapshot(map[string]any{
		"unit_points": "11.00000", "max_reserved_points": "12.65000",
		"sales_rule": map[string]any{"pricing_mode": "metered", "fixed_task_points": "1.00000", "output_second_points": "2.00000", "reserve_markup": "1.15000"},
	}).SetReservedPoints("12.65000").Save(ctx); err != nil {
		t.Fatal(err)
	}
	item, err := client.VideoTaskItem.UpdateOneID(itemID).SetStatus(string(domainvideo.ItemStateArtifactPending)).SetVersion(4).SetLeaseOwner("worker-artifact").SetLeaseExpiresAt(now.Add(time.Minute)).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := store.CommitArtifact(ctx, worker.ArtifactCommitRequest{
		ItemID: itemID.String(), Owner: "worker-artifact", ExpectedVersion: item.Version, AssetID: uuid.NewString(), UserID: 9123,
		ProjectID: mustVideoWorkerTask(t, ctx, client, taskID).ProjectID.String(), Status: "ready_original", StorageDriver: "local",
		ObjectKey: "media/original/9123/result.mp4", MIMEType: "video/mp4", SizeBytes: 1024, SHA256: strings.Repeat("b", 64),
	})
	if err != nil || !committed {
		t.Fatalf("CommitArtifact() committed=%v err=%v", committed, err)
	}
	storedItem, err := client.VideoTaskItem.Get(ctx, itemID)
	if err != nil || storedItem.ActualPoints != "0.00000" {
		t.Fatalf("pending actual points=%q err=%v", storedItem.ActualPoints, err)
	}
	settlement, err := store.LoadSettlement(ctx, taskID.String())
	if err != nil || len(settlement.Items) != 1 || !settlement.Items[0].UsagePending {
		t.Fatalf("pending settlement=%#v err=%v", settlement, err)
	}
	if _, claimed, err := store.ClaimDue(ctx, worker.ClaimRequest{Owner: "worker-settlement", Now: now.Add(time.Minute), LeaseTTL: time.Minute}); err != nil || claimed {
		t.Fatalf("usage-pending item must wait for probe: claimed=%v err=%v", claimed, err)
	}
}

func TestVideoWorkerStoreFreezesAndAppliesProviderCostRule(t *testing.T) {
	ctx, client := openVideoTaskStoreTestClient(t, "video-worker-cost")
	store := NewVideoTaskStore(client, NewBillingStore(client, 5))
	now := time.Date(2026, 8, 12, 16, 30, 0, 0, time.UTC)
	_, itemID := seedVideoWorkerTask(t, ctx, client, 9121, "cost", now)
	if _, err := client.VideoProviderCostRule.Create().SetAccountModelID(22).SetBillingMode("output_second").SetRuleVersion(3).
		SetCurrency("CNY").SetRatesJSON(map[string]any{"combinations": []any{map[string]any{
		"task_type": "text_to_video", "resolution": "720p", "audio_mode": "silent", "duration_seconds": 5, "cost_cny": "1.25000",
	}}}).SetCostReserveMarkup("1.00000").SetValidationStatus("verified").SetEffectiveAt(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)).SetEnabled(true).Save(ctx); err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := store.ClaimDue(ctx, worker.ClaimRequest{Owner: "worker-cost", Now: now, LeaseTTL: time.Minute})
	if err != nil || !ok {
		t.Fatalf("ClaimDue() ok=%v err=%v", ok, err)
	}
	attemptID := uuid.New()
	prepared, err := store.PrepareAttempt(ctx, worker.PrepareAttemptRequest{ItemID: itemID.String(), Owner: "worker-cost", ExpectedVersion: claimed.Version, AttemptID: attemptID.String(), ProviderIdempotencyKey: "cost-attempt"})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := client.VideoTaskAttempt.Get(ctx, attemptID)
	if err != nil || attempt.CostSnapshot["rule_version"] != float64(3) || attempt.CostSnapshot["currency"] != "CNY" {
		t.Fatalf("frozen cost snapshot=%#v err=%v", attempt.CostSnapshot, err)
	}
	changed, err := store.ApplyStep(ctx, worker.ApplyStepRequest{
		ItemID: itemID.String(), Owner: "worker-cost", ExpectedVersion: prepared.Version, Target: domainvideo.ItemStateProviderQueued,
		ProviderJobID: "job-cost", AttemptStatus: "provider_queued", UsageRaw: map[string]any{"output_seconds": 5},
		UsageNormalized: providervideo.Usage{OutputSeconds: "5.000"},
	})
	if err != nil || !changed {
		t.Fatalf("ApplyStep() changed=%v err=%v", changed, err)
	}
	attempt, err = client.VideoTaskAttempt.Get(ctx, attemptID)
	item, itemErr := client.VideoTaskItem.Get(ctx, itemID)
	if err != nil || itemErr != nil || attempt.ProviderCost != "1.25000" || item.ProviderCost != "1.25000" {
		t.Fatalf("provider cost attempt=%#v item=%#v errors=%v/%v", attempt, item, err, itemErr)
	}
}

func TestVideoWorkerStorePersistsArtifactRetryAndCommitsMediaAsset(t *testing.T) {
	ctx, client := openVideoTaskStoreTestClient(t, "video-worker-artifact")
	store := NewVideoTaskStore(client, NewBillingStore(client, 5))
	now := time.Date(2026, 8, 12, 17, 0, 0, 0, time.UTC)
	taskID, itemID := seedVideoWorkerTask(t, ctx, client, 9130, "artifact", now)
	if _, err := client.VideoTaskItem.UpdateOneID(itemID).SetStatus(string(domainvideo.ItemStateArtifactPending)).SetStage("artifact_pending").Save(ctx); err != nil {
		t.Fatal(err)
	}
	attemptID := uuid.New()
	if _, err := createVideoWorkerAttempt(client, itemID, attemptID, 1, "job-artifact").Save(ctx); err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := store.ClaimDue(ctx, worker.ClaimRequest{Owner: "worker-artifact", Now: now, LeaseTTL: time.Minute})
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	next := now.Add(time.Second)
	changed, err := store.ApplyStep(ctx, worker.ApplyStepRequest{
		ItemID: itemID.String(), Owner: "worker-artifact", ExpectedVersion: claimed.Version,
		Target: domainvideo.ItemStateRecoveryRequired, AttemptStatus: "recovery_required",
		Artifact:                  providervideo.Artifact{URL: "https://media.example.test/result.mp4", MIMEType: "video/mp4", SizeBytes: 42, SHA256: "abc"},
		IncrementArtifactAttempts: true, NextActionAt: &next,
	})
	if err != nil || !changed {
		t.Fatalf("artifact retry changed=%v err=%v", changed, err)
	}
	item, _ := client.VideoTaskItem.Get(ctx, itemID)
	if item.ArtifactAttempts != 1 || item.ArtifactSnapshot["url"] == nil {
		t.Fatalf("artifact retry snapshot=%#v attempts=%d", item.ArtifactSnapshot, item.ArtifactAttempts)
	}
	if _, err := client.VideoTaskItem.UpdateOneID(itemID).SetStatus(string(domainvideo.ItemStateArtifactPending)).SetLeaseOwner("worker-artifact").SetLeaseExpiresAt(now.Add(time.Minute)).Save(ctx); err != nil {
		t.Fatal(err)
	}
	item, _ = client.VideoTaskItem.Get(ctx, itemID)
	assetID := uuid.New()
	commitRequest := worker.ArtifactCommitRequest{
		ItemID: itemID.String(), Owner: "worker-artifact", ExpectedVersion: item.Version,
		AssetID: assetID.String(), UserID: 9130, ProjectID: mustVideoWorkerTask(t, ctx, client, taskID).ProjectID.String(),
		Status: "ready_original", StorageDriver: "local", ObjectKey: "media/original/result.mp4",
		MIMEType: "video/mp4", SizeBytes: 42, SHA256: "abc",
	}
	committed, err := store.CommitArtifact(ctx, commitRequest)
	if err != nil || !committed {
		t.Fatalf("CommitArtifact() committed=%v err=%v", committed, err)
	}
	asset, err := client.MediaAsset.Query().Where(mediaasset.IDEQ(assetID)).Only(ctx)
	if err != nil || asset.Status != "ready_original" || asset.SourceTaskID == nil || *asset.SourceTaskID != taskID {
		t.Fatalf("committed asset=%#v err=%v", asset, err)
	}
	item, _ = client.VideoTaskItem.Get(ctx, itemID)
	if item.Status != string(domainvideo.ItemStateSucceeded) || item.ResultAssetID == nil || *item.ResultAssetID != assetID || item.ActualPoints != "7.50000" {
		t.Fatalf("committed item=%#v", item)
	}
	if committed, err := store.CommitArtifact(ctx, commitRequest); err != nil || committed {
		t.Fatalf("duplicate CommitArtifact() committed=%v err=%v", committed, err)
	}
}

func TestVideoWorkerStoreLoadsAndFinalizesPartialAndZeroSuccessIdempotently(t *testing.T) {
	for _, tc := range []struct {
		name       string
		states     []domainvideo.ItemState
		status     domainvideo.TaskStatus
		successes  int
		actual     string
		wantPoints string
	}{
		{name: "partial", states: []domainvideo.ItemState{domainvideo.ItemStateSucceeded, domainvideo.ItemStateFailed}, status: domainvideo.TaskStatusPartial, successes: 1, actual: "7.50000", wantPoints: "92.50000"},
		{name: "zero", states: []domainvideo.ItemState{domainvideo.ItemStateFailed, domainvideo.ItemStateCancelled}, status: domainvideo.TaskStatusFailed, successes: 0, actual: "0.00000", wantPoints: "100.00000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, client := openVideoTaskStoreTestClient(t, "video-worker-finalize-"+tc.name)
			billing := NewBillingStore(client, 5)
			store := NewVideoTaskStore(client, billing)
			const userID int64 = 9140
			if _, err := billing.Adjust(ctx, billingservice.AdjustStoreRequest{UserID: userID, ChangePoints: "100.00000", Reason: "seed"}); err != nil {
				t.Fatal(err)
			}
			taskID, itemIDs := createReservedVideoWorkerTask(t, ctx, client, store, userID, tc.name, 2)
			for index, state := range tc.states {
				update := client.VideoTaskItem.UpdateOneID(itemIDs[index]).SetStatus(string(state)).SetStage(string(state))
				if state == domainvideo.ItemStateSucceeded {
					update.SetActualPoints("7.50000")
				}
				if _, err := update.Save(ctx); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := store.LoadSettlement(ctx, taskID.String())
			if err != nil || len(snapshot.Items) != 2 || snapshot.Items[0].PricePoints != "7.50000" {
				t.Fatalf("LoadSettlement() = %#v err=%v", snapshot, err)
			}
			request := worker.FinalizeRequest{TaskID: taskID.String(), Status: tc.status, SuccessOutputCount: tc.successes, ActualPoints: tc.actual, ReservedPoints: "15.00000"}
			finalized, err := store.FinalizeTask(ctx, request)
			if err != nil || !finalized {
				t.Fatalf("FinalizeTask() finalized=%v err=%v", finalized, err)
			}
			finalized, err = store.FinalizeTask(ctx, request)
			if err != nil || finalized {
				t.Fatalf("repeat FinalizeTask() finalized=%v err=%v", finalized, err)
			}
			balance, err := billing.GetBalance(ctx, userID)
			if err != nil || balance.AvailablePoints != tc.wantPoints || balance.FrozenPoints != "0.00000" {
				t.Fatalf("balance=%#v err=%v", balance, err)
			}
		})
	}
}

func seedVideoWorkerTask(t *testing.T, ctx context.Context, client *repoent.Client, userID int64, suffix string, next time.Time) (uuid.UUID, uuid.UUID) {
	t.Helper()
	exists, err := client.VideoProviderCostRule.Query().Where(videoprovidercostrule.AccountModelIDEQ(22), videoprovidercostrule.RuleVersionEQ(1)).Exist(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		if _, err := client.VideoProviderCostRule.Create().SetAccountModelID(22).SetBillingMode("output_second").SetRuleVersion(1).
			SetCurrency("CNY").SetRatesJSON(map[string]any{"combinations": []any{map[string]any{
			"task_type": "text_to_video", "resolution": "720p", "audio_mode": "silent", "duration_seconds": 5, "cost_cny": "0.50000",
		}}}).SetValidationStatus("verified").SetEffectiveAt(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)).SetEnabled(true).Save(ctx); err != nil {
			t.Fatal(err)
		}
	}
	project := mustCreateVideoTaskProject(t, ctx, client, userID)
	taskID, itemID := uuid.New(), uuid.New()
	_, err = client.VideoTask.Create().SetID(taskID).SetUserID(userID).SetProjectID(project.ID).
		SetTaskType("text_to_video").SetPromptTemplate("move").SetPromptBindingSnapshot(map[string]any{}).SetExecutionPrompt("move").SetRouteModelID(1).SetRouteModelCode("cinema").
		SetDurationSeconds(5).SetResolution("720p").SetAspectRatio("16:9").SetRequestedOutputCount(1).
		SetEstimatedPoints("7.50000").SetReservedPoints("7.50000").SetPricingSnapshot(map[string]any{"unit_points": "7.50000"}).
		SetRoutingSnapshot(map[string]any{"route_candidate_id": float64(11), "account_model_id": float64(22), "model_account_id": float64(33), "provider_code": "fake", "model_code": "video-fake"}).
		SetIdempotencyKey("worker-" + suffix).SetRequestFingerprint("fingerprint-" + suffix).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.VideoTaskItem.Create().SetID(itemID).SetTaskID(taskID).SetOrdinal(0).SetNextActionAt(next).Save(ctx); err != nil {
		t.Fatal(err)
	}
	return taskID, itemID
}

func createVideoWorkerAttempt(client *repoent.Client, itemID, attemptID uuid.UUID, attemptNo int, jobID string) *repoent.VideoTaskAttemptCreate {
	return client.VideoTaskAttempt.Create().SetID(attemptID).SetItemID(itemID).SetAttemptNo(attemptNo).
		SetRouteCandidateID(11).SetAccountModelID(22).SetModelAccountID(33).SetProviderCode("fake").SetModelCode("video-fake").
		SetProviderJobID(jobID).SetProviderIdempotencyKey("idempotency-" + attemptID.String()).SetStatus("provider_running").SetRequestSnapshot(map[string]any{})
}

func mustVideoWorkerTask(t *testing.T, ctx context.Context, client *repoent.Client, taskID uuid.UUID) *repoent.VideoTask {
	t.Helper()
	task, err := client.VideoTask.Get(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func createReservedVideoWorkerTask(t *testing.T, ctx context.Context, client *repoent.Client, store *VideoTaskStore, userID int64, suffix string, outputs int) (uuid.UUID, []uuid.UUID) {
	t.Helper()
	project := mustCreateVideoTaskProject(t, ctx, client, userID)
	taskID := uuid.New()
	items := make([]VideoTaskItemCreate, outputs)
	itemIDs := make([]uuid.UUID, outputs)
	for index := range items {
		itemIDs[index] = uuid.New()
		items[index] = VideoTaskItemCreate{ID: itemIDs[index], Ordinal: index}
	}
	_, err := store.CreateWithReservation(ctx, CreateVideoTaskWithReservationRequest{
		Task:  VideoTaskCreate{ID: taskID, UserID: userID, ProjectID: project.ID, TaskType: "text_to_video", PromptTemplate: "move", ExecutionPrompt: "move", RouteModelID: 1, RouteModelCode: "cinema", DurationSeconds: 5, Resolution: "720p", AspectRatio: "16:9", RequestedOutputCount: outputs, EstimatedPoints: "15.00000", ReservedPoints: "15.00000", PricingSnapshot: map[string]any{"unit_points": "7.50000"}, IdempotencyKey: "finalize-" + suffix, RequestFingerprint: "fingerprint-" + suffix},
		Items: items, Reserve: billingservice.ReserveStoreRequest{UserID: userID, TaskID: taskID.String(), EstimatedPoints: "15.00000", Reason: "reserve"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return taskID, itemIDs
}

var _ worker.Store = (*VideoTaskStore)(nil)
var _ = videoprovidercallbackevent.FieldStatus
var _ = videotaskitem.FieldStatus
