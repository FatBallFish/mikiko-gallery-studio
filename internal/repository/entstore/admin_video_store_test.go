package entstore

import (
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	adminvideoservice "github.com/fatballfish/pic-gallery/internal/service/adminvideo"
)

func openAdminVideoTestStore(t *testing.T, name string) (*repoent.Client, *AdminVideoStore) {
	t.Helper()
	client, err := repoent.Open(dialect.SQLite, "file:"+name+"-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	return client, NewAdminVideoStore(client)
}

func TestAdminVideoConfigStoreKeepsImmutableVersionsAndRollsBackFailedInsert(t *testing.T) {
	client, store := openAdminVideoTestStore(t, "admin-video-config-versions")
	ctx := t.Context()
	account, err := client.ModelAccount.Create().SetName("Provider").SetAdapterType("minimax").SetAuthType("api_key").SetBaseURL("https://provider.invalid").SetStatus("enabled").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	model, err := client.ModelAccountModel.Create().SetAccountID(int64(account.ID)).SetModelCode("video-model").SetEnabled(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	first, err := store.SaveCostRule(ctx, adminvideoservice.CostRuleWrite{AccountModelID: int64(model.ID), EffectiveAt: now, BillingMode: "output_second", Currency: "CNY", Rates: map[string]any{"combinations": []any{map[string]any{"cost_cny": "1"}}}, ExpectedRuleVersion: 0})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.SaveCostRule(ctx, adminvideoservice.CostRuleWrite{ID: first.ID, AccountModelID: int64(model.ID), EffectiveAt: now, BillingMode: "output_second", Currency: "CNY", Rates: map[string]any{"combinations": []any{map[string]any{"cost_cny": "2"}}}, ExpectedRuleVersion: 1, Enabled: true})
	if err != nil || second.RuleVersion != 2 {
		t.Fatalf("second=%#v err=%v", second, err)
	}
	rows, err := client.VideoProviderCostRule.Query().All(ctx)
	if err != nil || len(rows) != 2 || rows[0].Enabled || !rows[1].Enabled {
		t.Fatalf("rows=%#v err=%v", rows, err)
	}
	if _, err := store.SaveCostRule(ctx, adminvideoservice.CostRuleWrite{ID: first.ID, AccountModelID: int64(model.ID), EffectiveAt: now, BillingMode: "output_second", Currency: "CNY", Rates: map[string]any{"x": "3"}, ExpectedRuleVersion: 1}); err == nil {
		t.Fatal("stale cost rule version must fail")
	}

	strategyV1, err := client.VideoPricingStrategy.Create().SetCode("video").SetName("Video v1").SetStrategyVersion(1).SetEnabled(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.VideoPricingStrategy.Create().SetCode("video").SetName("collision").SetStrategyVersion(2).SetEnabled(false).Save(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = store.SaveStrategy(ctx, adminvideoservice.StrategyWrite{ID: int64(strategyV1.ID), ExpectedVersion: 1, Code: "video", Name: strings.Repeat("x", 300)})
	if err == nil {
		t.Fatal("duplicate next version must fail")
	}
	refreshed, getErr := client.VideoPricingStrategy.Get(ctx, strategyV1.ID)
	if getErr != nil || !refreshed.Enabled {
		t.Fatalf("failed insert must roll back old strategy disable: enabled=%v err=%v", refreshed.Enabled, getErr)
	}
}

func TestAdminVideoStoreVersionsModelRateCardsAndClonesConfig(t *testing.T) {
	client, store := openAdminVideoTestStore(t, "admin-video-rate-cards")
	ctx := t.Context()
	account, err := client.ModelAccount.Create().SetName("Seedance").SetAdapterType("seedance").SetAuthType("api_key").SetBaseURL("https://provider.invalid").SetStatus("enabled").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	model, err := client.ModelAccountModel.Create().SetAccountID(int64(account.ID)).SetModelCode("doubao-seedance-2-0-260128").SetEnabled(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	config := map[string]any{"resolutions": map[string]any{"720p": map[string]any{"without_input_video_million_tokens_cny": "46"}}}
	first, err := store.SaveVideoModelRateCard(ctx, adminvideoservice.RateCardWrite{
		AccountModelID: int64(model.ID), ProviderCode: "seedance", PricingSchema: "seedance_token_v1",
		ExpectedRateVersion: 0, Currency: "CNY", RateConfig: config, EffectiveAt: now, Enabled: true,
	})
	if err != nil || first.RateVersion != 1 {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	config["resolutions"].(map[string]any)["720p"].(map[string]any)["without_input_video_million_tokens_cny"] = "999"
	loaded, err := store.GetEffectiveVideoModelRateCard(ctx, int64(model.ID), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	gotRate := loaded.RateConfig["resolutions"].(map[string]any)["720p"].(map[string]any)["without_input_video_million_tokens_cny"]
	if gotRate != "46" {
		t.Fatalf("stored config mutated through caller map: %#v", loaded.RateConfig)
	}

	second, err := store.SaveVideoModelRateCard(ctx, adminvideoservice.RateCardWrite{
		AccountModelID: int64(model.ID), ProviderCode: "seedance", PricingSchema: "seedance_token_v1",
		ExpectedRateVersion: 1, Currency: "CNY", RateConfig: map[string]any{"resolutions": map[string]any{"720p": map[string]any{"without_input_video_million_tokens_cny": "50"}}}, EffectiveAt: now.Add(time.Minute), Enabled: true,
	})
	if err != nil || second.RateVersion != 2 {
		t.Fatalf("second=%#v err=%v", second, err)
	}
	if _, err := store.SaveVideoModelRateCard(ctx, adminvideoservice.RateCardWrite{AccountModelID: int64(model.ID), ExpectedRateVersion: 1}); err == nil {
		t.Fatal("stale rate card version must fail")
	}
	rows, err := store.ListVideoModelRateCards(ctx, int64(model.ID))
	if err != nil || len(rows) != 2 || rows[0].Enabled || !rows[1].Enabled {
		t.Fatalf("rows=%#v err=%v", rows, err)
	}
	current, err := store.GetEffectiveVideoModelRateCard(ctx, int64(model.ID), now.Add(2*time.Minute))
	if err != nil || current.RateVersion != 2 {
		t.Fatalf("current=%#v err=%v", current, err)
	}
}

func TestAdminVideoConfigStoreCapabilityAndRouteUseCASAndDoNotMutateInput(t *testing.T) {
	client, store := openAdminVideoTestStore(t, "admin-video-config-cas")
	ctx := t.Context()
	account, _ := client.ModelAccount.Create().SetName("Provider").SetAdapterType("minimax").SetAuthType("api_key").SetBaseURL("https://provider.invalid").SetStatus("enabled").Save(ctx)
	model, _ := client.ModelAccountModel.Create().SetAccountID(int64(account.ID)).SetModelCode("video-model").SetEnabled(true).Save(ctx)
	capability, err := store.SaveCapability(ctx, adminvideoservice.CapabilityWrite{AccountModelID: int64(model.ID), CapabilityVersion: "v1", Capability: map[string]any{"schema_version": 1}})
	if err != nil || capability.Version != "v1" {
		t.Fatalf("capability=%#v err=%v", capability, err)
	}
	if _, err := store.SaveCapability(ctx, adminvideoservice.CapabilityWrite{AccountModelID: int64(model.ID), ExpectedVersion: "stale", CapabilityVersion: "v2", Capability: map[string]any{"schema_version": 1}}); err == nil {
		t.Fatal("stale capability version must fail")
	}
	strategy, _ := client.VideoPricingStrategy.Create().SetCode("video").SetName("Video").SetStrategyVersion(1).Save(ctx)
	route, _ := client.RouteModel.Create().SetCode("video").SetName("Video").SetMediaType("video").Save(ctx)
	visible := map[string]any{"resolutions": []any{"720p"}}
	saved, err := store.SaveRouteConfig(ctx, adminvideoservice.RouteConfigWrite{RouteModelID: int64(route.ID), ConfigVersion: "v1", PricingStrategyID: int64(strategy.ID), TaskTypes: []string{"text_to_video"}, VisibleOptions: visible, VisibleCombinations: []adminvideoservice.VisibleCombination{{TaskType: "text_to_video", Resolution: "720p", DurationSeconds: 5}}, Defaults: map[string]any{}, MaxOutputCount: 1})
	if err != nil || saved.ConfigVersion != "v1" {
		t.Fatalf("route=%#v err=%v", saved, err)
	}
	if _, exists := visible["combinations"]; exists {
		t.Fatal("SaveRouteConfig must not mutate caller-owned visible options")
	}
	if _, err := store.SaveRouteConfig(ctx, adminvideoservice.RouteConfigWrite{RouteModelID: int64(route.ID), ExpectedVersion: "stale", ConfigVersion: "v2", PricingStrategyID: int64(strategy.ID), MaxOutputCount: 1}); err == nil {
		t.Fatal("stale route config version must fail")
	}
}

func TestAdminVideoSnapshotIncludesUnconfiguredVideoRouteCandidates(t *testing.T) {
	client, store := openAdminVideoTestStore(t, "admin-video-unconfigured-route")
	ctx := t.Context()
	account, err := client.ModelAccount.Create().SetName("MiniMax").SetAdapterType("minimax").SetAuthType("api_key").SetBaseURL("https://provider.invalid").SetStatus("enabled").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	model, err := client.ModelAccountModel.Create().SetAccountID(int64(account.ID)).SetModelCode("MiniMax-H3").SetEnabled(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	route, err := client.RouteModel.Create().SetCode("unconfigured-video").SetName("Unconfigured Video").SetMediaType("video").SetEnabled(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.RouteModelCandidate.Create().SetRouteModelID(int64(route.ID)).SetAccountModelID(int64(model.ID)).SetEnabled(true).Save(ctx); err != nil {
		t.Fatal(err)
	}

	snapshot, err := store.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Routes) != 1 {
		t.Fatalf("snapshot routes = %#v", snapshot.Routes)
	}
	got := snapshot.Routes[0]
	if got.RouteModelID != int64(route.ID) || got.CandidateCount != 1 || len(got.CandidateAccountModelIDs) != 1 || got.CandidateAccountModelIDs[0] != int64(model.ID) {
		t.Fatalf("unconfigured route projection = %#v", got)
	}
	if got.ConfigVersion != "" || got.PricingStrategyID != 0 || got.Enabled {
		t.Fatalf("unconfigured route must remain disabled until configured: %#v", got)
	}
}

func TestAdminVideoStoreProjectsConfigurationTaskDiagnosticsAndRecovery(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:admin-video-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 12, 18, 0, 0, 0, time.UTC)

	account, err := client.ModelAccount.Create().SetName("MiniMax").SetAdapterType("minimax").SetAuthType("api_key").SetBaseURL("https://provider.invalid").SetStatus("enabled").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	model, err := client.ModelAccountModel.Create().SetAccountID(int64(account.ID)).SetModelCode("MiniMax-H3").SetEnabled(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.VideoModelCapability.Create().SetAccountModelID(int64(model.ID)).SetCapabilityVersion("cap-v3").SetCapabilityJSON(map[string]any{"schema_version": 1}).SetValidationStatus("verified").SetEnabled(true).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.VideoProviderCostRule.Create().SetAccountModelID(int64(model.ID)).SetBillingMode("output_second").SetRuleVersion(2).SetRatesJSON(map[string]any{"720p": "0.1"}).SetEffectiveAt(now.Add(-time.Hour)).SetEnabled(true).Save(ctx); err != nil {
		t.Fatal(err)
	}
	strategy, err := client.VideoPricingStrategy.Create().SetCode("video").SetName("Video").SetStrategyVersion(4).SetEnabled(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.VideoPriceRule.Create().SetPricingStrategyID(int64(strategy.ID)).SetTaskType("text_to_video").SetResolution("720p").SetAudioMode("silent").SetRuleVersion(5).SetEffectiveAt(now.Add(-time.Hour)).SetMinimumTaskPoints("9.00000").SetSafetyPoints("8.00000").SetSafetySnapshot(map[string]any{}).SetEnabled(true).Save(ctx); err != nil {
		t.Fatal(err)
	}
	route, err := client.RouteModel.Create().SetCode("cinema").SetName("Cinema").SetMediaType("video").SetEnabled(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.RouteModelCandidate.Create().SetRouteModelID(int64(route.ID)).SetAccountModelID(int64(model.ID)).SetEnabled(true).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.VideoRouteConfig.Create().SetRouteModelID(int64(route.ID)).SetTaskTypes([]string{"text_to_video"}).SetVisibleOptions(map[string]any{}).SetDefaults(map[string]any{}).SetMinimumTaskPoints("9.00000").SetRoundingStepPoints(1).SetConfigVersion("route-v6").SetEnabled(true).Save(ctx); err != nil {
		t.Fatal(err)
	}

	project, err := client.Project.Create().SetUserID(42).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	taskID, itemID, attemptID := uuid.New(), uuid.New(), uuid.New()
	if _, err := client.VideoTask.Create().SetID(taskID).SetUserID(42).SetProjectID(project.ID).SetTaskType("text_to_video").SetPromptTemplate("move").SetPromptBindingSnapshot(map[string]any{}).SetExecutionPrompt("move").SetRouteModelID(int64(route.ID)).SetRouteModelCode("cinema").SetDurationSeconds(5).SetResolution("720p").SetAspectRatio("16:9").SetEstimatedPoints("9.00000").SetReservedPoints("9.00000").SetActualPoints("9.00000").SetPricingSnapshot(map[string]any{"version": 5}).SetRoutingSnapshot(map[string]any{"version": "route-v6"}).SetSettlementStatus("settled").SetIdempotencyKey("admin-task").SetRequestFingerprint("fingerprint").Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.VideoTaskItem.Create().SetID(itemID).SetTaskID(taskID).SetOrdinal(0).SetStatus("artifact_failed").SetStage("artifact").SetProviderCost("0.50000").SetArtifactSnapshot(map[string]any{"object": "pending"}).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.VideoTaskAttempt.Create().SetID(attemptID).SetItemID(itemID).SetAttemptNo(1).SetRouteCandidateID(1).SetAccountModelID(int64(model.ID)).SetModelAccountID(int64(account.ID)).SetProviderCode("minimax").SetModelCode("MiniMax-H3").SetProviderJobID("provider-job-7").SetProviderIdempotencyKey("attempt-key").SetStatus("succeeded").SetRequestSnapshot(map[string]any{}).SetUsageRaw(map[string]any{"seconds": 5}).SetUsageNormalized(map[string]any{"output_seconds": "5"}).SetCostSnapshot(map[string]any{"rule_version": 2}).SetProviderCost("0.50000").Save(ctx); err != nil {
		t.Fatal(err)
	}
	asset, err := client.MediaAsset.Create().SetUserID(42).SetProjectID(project.ID).SetName("video").SetNameKey("video").SetMediaType("video").SetSourceType("generated").SetStatus("ready").SetObjectKey("video.mp4").SetMimeType("video/mp4").SetFileSizeBytes(10).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	jobID := uuid.New()
	if _, err := client.MediaProcessingJob.Create().SetID(jobID).SetAssetID(asset.ID).SetJobType("video_proxy").SetStatus("failed").SetErrorCode("ffmpeg").Save(ctx); err != nil {
		t.Fatal(err)
	}

	store := NewAdminVideoStore(client)
	snapshot, err := store.Snapshot(ctx)
	if err != nil || snapshot.Capabilities[0].Version != "cap-v3" || snapshot.CostRules[0].RuleVersion != 2 || snapshot.Strategies[0].StrategyVersion != 4 || snapshot.Routes[0].ConfigVersion != "route-v6" {
		t.Fatalf("snapshot=%#v err=%v", snapshot, err)
	}
	detail, err := store.GetTask(ctx, taskID)
	if err != nil || len(detail.Items) != 1 || len(detail.Items[0].Attempts) != 1 || detail.Items[0].Attempts[0].ProviderJobID != "provider-job-7" || detail.Items[0].Attempts[0].UsageNormalized["output_seconds"] != "5" {
		t.Fatalf("detail=%#v err=%v", detail, err)
	}
	page, err := store.ListTasks(ctx, adminvideoservice.TaskFilter{ProviderTaskID: "provider-job-7", AccountModelID: int64(model.ID), Limit: 20})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("page=%#v err=%v", page, err)
	}
	if err := store.Retry(ctx, adminvideoservice.RetryRequest{Kind: adminvideoservice.RetryArtifact, TaskID: taskID, ItemID: itemID}); err != nil {
		t.Fatal(err)
	}
	if refreshed, _ := client.VideoTaskItem.Get(ctx, itemID); refreshed.Status != "artifact_pending" {
		t.Fatalf("artifact status=%s", refreshed.Status)
	}
	if err := store.Retry(ctx, adminvideoservice.RetryRequest{Kind: adminvideoservice.RetryDerivative, JobID: jobID}); err != nil {
		t.Fatal(err)
	}
	if refreshed, _ := client.MediaProcessingJob.Get(ctx, jobID); refreshed.Status != "pending" || refreshed.ErrorCode != nil {
		t.Fatalf("job=%#v", refreshed)
	}
	if _, err := client.VideoTaskItem.UpdateOneID(itemID).SetStatus("succeeded").SetStage("completed").Save(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Retry(ctx, adminvideoservice.RetryRequest{Kind: adminvideoservice.RetryArtifact, TaskID: taskID, ItemID: itemID}); err == nil {
		t.Fatal("successful artifact must not be retried")
	}
	if _, err := client.MediaProcessingJob.UpdateOneID(jobID).SetStatus("succeeded").Save(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Retry(ctx, adminvideoservice.RetryRequest{Kind: adminvideoservice.RetryDerivative, JobID: jobID}); err == nil {
		t.Fatal("successful derivative must not be retried")
	}
	if _, err := client.VideoTask.UpdateOneID(taskID).SetSettlementStatus("refund_pending").Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.VideoTaskItem.UpdateOneID(itemID).SetLeaseOwner("stale-worker").SetLeaseExpiresAt(time.Now().Add(time.Hour)).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Retry(ctx, adminvideoservice.RetryRequest{Kind: adminvideoservice.RetrySettlement, TaskID: taskID}); err != nil {
		t.Fatal(err)
	}
	if refreshed, _ := client.VideoTaskItem.Get(ctx, itemID); refreshed.LeaseOwner != nil || refreshed.NextActionAt == nil {
		t.Fatalf("settlement recovery must release stale lease and make terminal item due: %#v", refreshed)
	}
	if _, err := client.VideoTask.UpdateOneID(taskID).SetSettlementStatus("finalized").Save(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Retry(ctx, adminvideoservice.RetryRequest{Kind: adminvideoservice.RetrySettlement, TaskID: taskID}); err == nil {
		t.Fatal("finalized settlement must not be retried")
	}
	if _, err := client.VideoTask.UpdateOneID(taskID).SetSettlementStatus("reserved").Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.VideoTaskItem.UpdateOneID(itemID).SetStatus("provider_running").Save(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Retry(ctx, adminvideoservice.RetryRequest{Kind: adminvideoservice.RetrySettlement, TaskID: taskID}); err == nil {
		t.Fatal("running task must not enter manual settlement recovery")
	}
}

func TestAdminVideoStorePersistsMediaPolicyWithVersionConflict(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:admin-media-policy-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	store := NewAdminVideoStore(client)
	policy, err := store.GetMediaPolicy(ctx)
	if err != nil || policy.Version != 1 {
		t.Fatalf("policy=%#v err=%v", policy, err)
	}
	policy.Version = 2
	policy.SingleFileMaxBytes = 128 << 20
	if _, err := store.SaveMediaPolicy(ctx, policy, 7); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveMediaPolicy(ctx, policy, 7); err == nil {
		t.Fatal("stale version must fail")
	}
	loaded, err := store.GetMediaPolicy(ctx)
	if err != nil || loaded.Version != 2 || loaded.SingleFileMaxBytes != 128<<20 {
		t.Fatalf("loaded=%#v err=%v", loaded, err)
	}
}
