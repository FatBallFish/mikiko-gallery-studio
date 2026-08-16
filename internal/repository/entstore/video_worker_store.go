package entstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	providervideo "github.com/fatballfish/pic-gallery/internal/provider/video"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/modelaccount"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/modelaccountmodel"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/routemodelcandidate"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videomodelcapability"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videoprovidercallbackevent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videoprovidercostrule"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videotask"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videotaskattempt"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videotaskinput"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videotaskitem"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	worker "github.com/fatballfish/pic-gallery/internal/worker/video"
	"github.com/shopspring/decimal"
)

func (s *VideoTaskStore) ClaimDue(ctx context.Context, req worker.ClaimRequest) (worker.WorkItem, bool, error) {
	if s == nil || s.client == nil || strings.TrimSpace(req.Owner) == "" || req.LeaseTTL <= 0 {
		return worker.WorkItem{}, false, errors.New("invalid video worker claim")
	}
	result, err := withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (videoClaimResult, error) {
		if err := projectVideoCallback(ctx, tx, req.Now.UTC()); err != nil {
			return videoClaimResult{}, err
		}
		terminalStates := []string{string(domainvideo.ItemStateSucceeded), string(domainvideo.ItemStateFailed), string(domainvideo.ItemStateCancelled)}
		entities, err := tx.VideoTaskItem.Query().Where(
			videotaskitem.Or(
				videotaskitem.And(
					videotaskitem.StatusNotIn(terminalStates...),
					videotaskitem.Or(videotaskitem.NextActionAtIsNil(), videotaskitem.NextActionAtLTE(req.Now.UTC())),
				),
				videotaskitem.And(
					videotaskitem.StatusIn(terminalStates...),
					videotaskitem.StageNEQ("usage_pending"),
					videotaskitem.HasTaskWith(videotask.SettlementStatusNEQ("finalized")),
				),
			),
			videotaskitem.Or(videotaskitem.LeaseOwnerIsNil(), videotaskitem.LeaseExpiresAtIsNil(), videotaskitem.LeaseExpiresAtLTE(req.Now.UTC())),
		).Order(repoent.Asc(videotaskitem.FieldNextActionAt), repoent.Asc(videotaskitem.FieldCreatedAt)).Limit(8).All(ctx)
		if err != nil {
			return videoClaimResult{}, fmt.Errorf("query due video item: %w", err)
		}
		for _, entity := range entities {
			expiresAt := req.Now.UTC().Add(req.LeaseTTL)
			count, updateErr := tx.VideoTaskItem.Update().Where(
				videotaskitem.IDEQ(entity.ID), videotaskitem.VersionEQ(entity.Version),
				videotaskitem.Or(videotaskitem.LeaseOwnerIsNil(), videotaskitem.LeaseExpiresAtIsNil(), videotaskitem.LeaseExpiresAtLTE(req.Now.UTC())),
			).SetLeaseOwner(strings.TrimSpace(req.Owner)).SetLeaseExpiresAt(expiresAt).Save(ctx)
			if updateErr != nil {
				return videoClaimResult{}, fmt.Errorf("claim video item: %w", updateErr)
			}
			if count != 1 {
				continue
			}
			claimed, mapErr := loadVideoWorkItem(ctx, tx.Client(), entity.ID)
			if mapErr != nil {
				return videoClaimResult{}, mapErr
			}
			return videoClaimResult{item: claimed, claimed: true}, nil
		}
		return videoClaimResult{}, nil
	})
	return result.item, result.claimed, err
}

type videoClaimResult struct {
	item    worker.WorkItem
	claimed bool
}

func (s *VideoTaskStore) GetExecutionAccount(ctx context.Context, ref worker.ProviderRef) (worker.ExecutionAccount, error) {
	if s == nil || s.client == nil || ref.RouteCandidateID <= 0 || ref.AccountModelID <= 0 || ref.ModelAccountID <= 0 {
		return worker.ExecutionAccount{}, errors.New("invalid video execution account reference")
	}
	candidate, err := s.client.RouteModelCandidate.Query().Where(
		routemodelcandidate.IDEQ(int(ref.RouteCandidateID)), routemodelcandidate.AccountModelIDEQ(ref.AccountModelID),
		routemodelcandidate.EnabledEQ(true), routemodelcandidate.DeletedAtIsNil(),
	).Only(ctx)
	if err != nil {
		return worker.ExecutionAccount{}, fmt.Errorf("load video route candidate: %w", err)
	}
	accountModel, err := s.client.ModelAccountModel.Query().Where(
		modelaccountmodel.IDEQ(int(ref.AccountModelID)), modelaccountmodel.AccountIDEQ(ref.ModelAccountID),
		modelaccountmodel.ModelCodeEQ(ref.ModelCode), modelaccountmodel.EnabledEQ(true), modelaccountmodel.DeletedAtIsNil(),
	).Only(ctx)
	if err != nil {
		return worker.ExecutionAccount{}, fmt.Errorf("load video account model: %w", err)
	}
	account, err := s.client.ModelAccount.Query().Where(
		modelaccount.IDEQ(int(ref.ModelAccountID)), modelaccount.AdapterTypeEQ(ref.ProviderCode),
		modelaccount.StatusEQ("enabled"), modelaccount.DeletedAtIsNil(),
	).Only(ctx)
	if err != nil {
		return worker.ExecutionAccount{}, fmt.Errorf("load video model account: %w", err)
	}
	if _, err := s.client.VideoModelCapability.Query().Where(
		videomodelcapability.AccountModelIDEQ(ref.AccountModelID), videomodelcapability.EnabledEQ(true),
		videomodelcapability.ValidationStatusEQ("verified"), videomodelcapability.DeletedAtIsNil(),
	).Only(ctx); err != nil {
		return worker.ExecutionAccount{}, fmt.Errorf("load verified video capability: %w", err)
	}
	if candidate.AccountModelID != int64(accountModel.ID) || accountModel.AccountID != int64(account.ID) {
		return worker.ExecutionAccount{}, errors.New("video execution account relationship changed")
	}
	return worker.ExecutionAccount{
		RouteCandidateID: int64(candidate.ID), AccountModelID: int64(accountModel.ID), ModelAccountID: int64(account.ID),
		ProviderCode: account.AdapterType, BaseURL: account.BaseURL, APIKey: strings.TrimSpace(account.CredentialsEncrypted["api_key"]),
		ModelCode: accountModel.ModelCode, CallbackURL: stringSnapshot(account.Extra, "video_callback_url"),
		CallbackSecret: strings.TrimSpace(account.CredentialsEncrypted["callback_secret"]), Timeout: time.Duration(account.TimeoutMs) * time.Millisecond,
		ArtifactAllowedHosts: stringSliceSnapshot(account.Extra, "video_artifact_hosts"),
	}, nil
}

func stringSliceSnapshot(snapshot map[string]any, key string) []string {
	values, _ := snapshot[key].([]any)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			result = append(result, strings.TrimSpace(text))
		}
	}
	if typed, ok := snapshot[key].([]string); ok {
		result = append(result, typed...)
	}
	return result
}

func loadVideoCostSnapshot(ctx context.Context, client *repoent.Client, accountModelID int64, request providervideo.Request, now time.Time) (map[string]any, error) {
	rule, err := client.VideoProviderCostRule.Query().Where(
		videoprovidercostrule.AccountModelIDEQ(accountModelID), videoprovidercostrule.EnabledEQ(true),
		videoprovidercostrule.ValidationStatusEQ("verified"), videoprovidercostrule.EffectiveAtLTE(now),
		videoprovidercostrule.DeletedAtIsNil(), videoprovidercostrule.Or(videoprovidercostrule.ExpiresAtIsNil(), videoprovidercostrule.ExpiresAtGT(now)),
	).Order(repoent.Desc(videoprovidercostrule.FieldRuleVersion)).First(ctx)
	if repoent.IsNotFound(err) {
		return nil, errors.New("verified video provider cost rule is unavailable")
	}
	if err != nil {
		return nil, fmt.Errorf("load video provider cost rule: %w", err)
	}
	rates := cloneMap(rule.RatesJSON)
	currency := strings.ToUpper(strings.TrimSpace(rule.Currency))
	if currency == "" {
		currency = "CNY"
	}
	exchangeRate := "1.00000"
	if currency != "CNY" {
		exchangeRate = stringSnapshot(rates, "cny_exchange_rate")
		if value, parseErr := decimal.NewFromString(exchangeRate); parseErr != nil || !value.IsPositive() {
			return nil, errors.New("non-CNY video cost rule requires a positive cny_exchange_rate")
		}
	}
	return map[string]any{
		"rule_id": int64(rule.ID), "rule_version": rule.RuleVersion, "billing_mode": rule.BillingMode,
		"currency": currency, "cny_exchange_rate": exchangeRate, "rates": rates,
		"cost_reserve_markup": rule.CostReserveMarkup, "effective_at": rule.EffectiveAt.UTC().Format(time.RFC3339Nano),
		"task_type": request.TaskType, "resolution": request.Resolution,
		"audio_mode": videoAudioMode(request), "duration_seconds": request.DurationSeconds,
	}, nil
}

func videoAudioMode(request providervideo.Request) string {
	if request.GenerateAudio {
		return "audio_on"
	}
	return "silent"
}

func hasProviderUsage(req worker.ApplyStepRequest) bool {
	return req.UsageRaw != nil || req.UsageNormalized.OutputSeconds != "" || req.UsageNormalized.InputVideoSeconds != "" || req.UsageNormalized.ProviderTokens != "" || req.UsageNormalized.ReferenceImageCount != 0
}

func calculateProviderCost(snapshot map[string]any, usage providervideo.Usage) (string, error) {
	if int64Snapshot(snapshot, "schema_version") == 2 {
		return "", nil
	}
	rates, _ := snapshot["rates"].(map[string]any)
	if rates == nil {
		return "0.00000", errors.New("video provider cost snapshot has no rates")
	}
	base := decimal.Zero
	if combinations, ok := rates["combinations"].([]any); ok {
		for _, raw := range combinations {
			entry, _ := raw.(map[string]any)
			if stringSnapshot(entry, "task_type") == stringSnapshot(snapshot, "task_type") &&
				stringSnapshot(entry, "resolution") == stringSnapshot(snapshot, "resolution") &&
				stringSnapshot(entry, "audio_mode") == stringSnapshot(snapshot, "audio_mode") &&
				int64Snapshot(entry, "duration_seconds") == int64Snapshot(snapshot, "duration_seconds") {
				value, err := decimal.NewFromString(stringSnapshot(entry, "cost_cny"))
				if err != nil || value.IsNegative() {
					return "0.00000", errors.New("video provider combination cost is invalid")
				}
				base = value
				break
			}
		}
	}
	parseUsage := func(raw string) (decimal.Decimal, error) {
		if strings.TrimSpace(raw) == "" {
			return decimal.Zero, nil
		}
		value, err := decimal.NewFromString(raw)
		if err != nil || value.IsNegative() {
			return decimal.Zero, errors.New("normalized video usage is invalid")
		}
		return value, nil
	}
	outputSeconds, err := parseUsage(usage.OutputSeconds)
	if err != nil {
		return "0.00000", err
	}
	inputSeconds, err := parseUsage(usage.InputVideoSeconds)
	if err != nil {
		return "0.00000", err
	}
	tokens, err := parseUsage(usage.ProviderTokens)
	if err != nil {
		return "0.00000", err
	}
	for key, units := range map[string]decimal.Decimal{
		"output_second_cny": outputSeconds, "input_video_second_cny": inputSeconds,
		"reference_image_cny":         decimal.NewFromInt(int64(usage.ReferenceImageCount)),
		"provider_million_tokens_cny": tokens.Div(decimal.NewFromInt(1_000_000)),
	} {
		raw := stringSnapshot(rates, key)
		if raw == "" {
			continue
		}
		rate, parseErr := decimal.NewFromString(raw)
		if parseErr != nil || rate.IsNegative() {
			return "0.00000", fmt.Errorf("video provider cost rate %s is invalid", key)
		}
		base = base.Add(rate.Mul(units))
	}
	exchange, err := decimal.NewFromString(stringSnapshot(snapshot, "cny_exchange_rate"))
	if err != nil || !exchange.IsPositive() {
		return "0.00000", errors.New("video provider cost exchange rate is invalid")
	}
	return base.Mul(exchange).Round(5).StringFixed(5), nil
}

func (s *VideoTaskStore) PrepareAttempt(ctx context.Context, req worker.PrepareAttemptRequest) (worker.WorkItem, error) {
	itemID, attemptID, err := parseWorkerIDs(req.ItemID, req.AttemptID)
	if err != nil {
		return worker.WorkItem{}, err
	}
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (worker.WorkItem, error) {
		item, err := tx.VideoTaskItem.Query().Where(videotaskitem.IDEQ(itemID)).WithTask().Only(ctx)
		if err != nil {
			return worker.WorkItem{}, err
		}
		if item.Version != req.ExpectedVersion || optionalStringValue(item.LeaseOwner) != req.Owner || domainvideo.ItemState(item.Status) != domainvideo.ItemStateQueued {
			return worker.WorkItem{}, worker.ErrStepConflict
		}
		routeCandidateID := int64Snapshot(item.Edges.Task.RoutingSnapshot, "route_candidate_id")
		accountModelID := int64Snapshot(item.Edges.Task.RoutingSnapshot, "account_model_id")
		modelAccountID := int64Snapshot(item.Edges.Task.RoutingSnapshot, "model_account_id")
		providerCode := stringSnapshot(item.Edges.Task.RoutingSnapshot, "provider_code")
		modelCode := stringSnapshot(item.Edges.Task.RoutingSnapshot, "model_code")
		if routeCandidateID <= 0 || accountModelID <= 0 || modelAccountID <= 0 || providerCode == "" || modelCode == "" {
			return worker.WorkItem{}, errors.New("video task routing snapshot is incomplete")
		}
		attemptNo, err := tx.VideoTaskAttempt.Query().Where(videotaskattempt.ItemIDEQ(itemID)).Count(ctx)
		if err != nil {
			return worker.WorkItem{}, err
		}
		request := providerRequestFromTask(item.Edges.Task, item.ID)
		request.AttemptID, request.IdempotencyKey = attemptID.String(), req.ProviderIdempotencyKey
		requestSnapshot, err := structMap(request)
		if err != nil {
			return worker.WorkItem{}, err
		}
		costSnapshot := map[string]any{"schema_version": 2, "provider_cost_status": "unknown"}
		if int64Snapshot(item.Edges.Task.PricingSnapshot, "schema_version") != 2 {
			costSnapshot, err = loadVideoCostSnapshot(ctx, tx.Client(), accountModelID, request, time.Now().UTC())
			if err != nil {
				return worker.WorkItem{}, err
			}
		}
		if _, err := tx.VideoTaskAttempt.Create().SetID(attemptID).SetItemID(itemID).SetAttemptNo(attemptNo + 1).
			SetRouteCandidateID(routeCandidateID).SetAccountModelID(accountModelID).SetModelAccountID(modelAccountID).
			SetProviderCode(providerCode).SetModelCode(modelCode).SetProviderIdempotencyKey(req.ProviderIdempotencyKey).
			SetStatus("submitting").SetRequestSnapshot(requestSnapshot).SetCostSnapshot(costSnapshot).SetStartedAt(time.Now().UTC()).Save(ctx); err != nil {
			return worker.WorkItem{}, fmt.Errorf("create video attempt: %w", err)
		}
		count, err := tx.VideoTaskItem.Update().Where(videotaskitem.IDEQ(itemID), videotaskitem.VersionEQ(req.ExpectedVersion), videotaskitem.LeaseOwnerEQ(req.Owner)).
			SetStatus(string(domainvideo.ItemStateSubmitting)).SetStage("submitting").SetVersion(req.ExpectedVersion + 1).Save(ctx)
		if err != nil {
			return worker.WorkItem{}, err
		}
		if count != 1 {
			return worker.WorkItem{}, worker.ErrStepConflict
		}
		return loadVideoWorkItem(ctx, tx.Client(), itemID)
	})
}

func (s *VideoTaskStore) ApplyStep(ctx context.Context, req worker.ApplyStepRequest) (bool, error) {
	itemID, err := uuid.Parse(req.ItemID)
	if err != nil {
		return false, fmt.Errorf("invalid video item id: %w", err)
	}
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (bool, error) {
		item, err := tx.VideoTaskItem.Get(ctx, itemID)
		if err != nil {
			return false, err
		}
		if item.Version != req.ExpectedVersion || optionalStringValue(item.LeaseOwner) != req.Owner {
			return false, nil
		}
		current := domainvideo.ItemStateSnapshot{State: domainvideo.ItemState(item.Status), Version: item.Version}
		if req.ArtifactExhausted && current.State != domainvideo.ItemStateArtifactPending && current.State != domainvideo.ItemStateRecoveryRequired {
			return false, errors.New("artifact exhaustion requires an artifact recovery state")
		}
		if req.ArtifactExhausted && current.State == domainvideo.ItemStateArtifactPending {
			recovery, transitionErr := domainvideo.AdvanceItemState(current, domainvideo.ItemTransition{ExpectedVersion: current.Version, Target: domainvideo.ItemStateRecoveryRequired})
			if transitionErr != nil {
				return false, transitionErr
			}
			current = recovery.Snapshot
		}
		transition, err := domainvideo.AdvanceItemState(current, domainvideo.ItemTransition{ExpectedVersion: current.Version, Target: req.Target})
		if err != nil {
			return false, err
		}
		update := tx.VideoTaskItem.Update().Where(videotaskitem.IDEQ(itemID), videotaskitem.VersionEQ(req.ExpectedVersion), videotaskitem.LeaseOwnerEQ(req.Owner)).
			SetStatus(string(transition.Snapshot.State)).SetStage(stepStage(req)).SetVersion(transition.Snapshot.Version).
			SetNillableNextActionAt(req.NextActionAt).ClearLeaseOwner().ClearLeaseExpiresAt()
		if strings.TrimSpace(req.UsageNormalized.OutputSeconds) != "" {
			update.SetActualOutputSeconds(req.UsageNormalized.OutputSeconds)
		}
		if req.ErrorCode == "" {
			update.ClearErrorCode()
		} else {
			update.SetErrorCode(req.ErrorCode)
		}
		if req.ErrorMessage == "" {
			update.ClearErrorMessage()
		} else {
			update.SetErrorMessage(req.ErrorMessage)
		}
		if req.Artifact.URL != "" {
			artifact, mapErr := structMap(req.Artifact)
			if mapErr != nil {
				return false, mapErr
			}
			update.SetArtifactSnapshot(artifact)
		}
		if req.IncrementArtifactAttempts {
			update.AddArtifactAttempts(1)
		}
		count, err := update.Save(ctx)
		if err != nil || count != 1 {
			return false, err
		}
		attempt, err := latestVideoAttempt(ctx, tx.Client(), itemID)
		if err != nil && !repoent.IsNotFound(err) {
			return false, err
		}
		if attempt != nil {
			attemptUpdate := tx.VideoTaskAttempt.UpdateOne(attempt)
			if hasProviderUsage(req) {
				providerCost, costErr := calculateProviderCost(attempt.CostSnapshot, req.UsageNormalized)
				if costErr != nil {
					return false, costErr
				}
				if providerCost != "" {
					attemptUpdate.SetProviderCost(providerCost)
					if _, costUpdateErr := tx.VideoTaskItem.UpdateOneID(itemID).SetProviderCost(providerCost).Save(ctx); costUpdateErr != nil {
						return false, costUpdateErr
					}
				}
			}
			if req.ProviderStatusSnapshot != nil {
				attemptUpdate.SetProviderStatusSnapshot(cloneMap(req.ProviderStatusSnapshot))
			}
			if req.UsageRaw != nil {
				attemptUpdate.SetUsageRaw(cloneMap(req.UsageRaw))
			}
			if req.UsageNormalized.OutputSeconds != "" || req.UsageNormalized.InputVideoSeconds != "" || req.UsageNormalized.ReferenceImageCount != 0 || req.UsageNormalized.ProviderTokens != "" || req.UsageNormalized.Raw != nil {
				usage, mapErr := structMap(req.UsageNormalized)
				if mapErr != nil {
					return false, mapErr
				}
				attemptUpdate.SetUsageNormalized(usage)
			}
			if req.UsageNormalizationError != "" {
				attemptUpdate.SetErrorCategory("usage_normalization").SetErrorCode("usage_normalization_failed").SetErrorMessage(req.UsageNormalizationError)
			}
			if req.ProviderJobID != "" {
				attemptUpdate.SetProviderJobID(req.ProviderJobID)
			}
			if req.AttemptStatus != "" {
				attemptUpdate.SetStatus(req.AttemptStatus)
			}
			if req.PlatformAbsorbed {
				attemptUpdate.SetPlatformAbsorbed(true)
			}
			if req.ErrorCode != "" {
				attemptUpdate.SetErrorCode(req.ErrorCode)
			}
			if req.ErrorMessage != "" {
				attemptUpdate.SetErrorMessage(req.ErrorMessage)
			}
			if isTerminalVideoItem(transition.Snapshot.State) {
				attemptUpdate.SetFinishedAt(time.Now().UTC())
			}
			if _, err := attemptUpdate.Save(ctx); err != nil {
				return false, err
			}
		}
		return true, nil
	})
}

func (s *VideoTaskStore) CommitArtifact(ctx context.Context, req worker.ArtifactCommitRequest) (bool, error) {
	itemID, assetID, err := parseWorkerIDs(req.ItemID, req.AssetID)
	if err != nil {
		return false, err
	}
	projectID, err := uuid.Parse(req.ProjectID)
	if err != nil {
		return false, fmt.Errorf("invalid video project id: %w", err)
	}
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (bool, error) {
		item, err := tx.VideoTaskItem.Query().Where(videotaskitem.IDEQ(itemID)).WithTask().Only(ctx)
		if err != nil {
			return false, err
		}
		if item.Version != req.ExpectedVersion || optionalStringValue(item.LeaseOwner) != req.Owner || domainvideo.ItemState(item.Status) != domainvideo.ItemStateArtifactPending {
			return false, nil
		}
		if item.Edges.Task.UserID != req.UserID || item.Edges.Task.ProjectID != projectID {
			return false, errors.New("video artifact ownership does not match task")
		}
		asset := tx.MediaAsset.Create().SetID(assetID).SetUserID(req.UserID).SetProjectID(projectID).
			SetName(filepath.Base(req.ObjectKey)).SetNameKey(strings.ToLower(filepath.Base(req.ObjectKey))).SetMediaType("video").SetSourceType("generated").
			SetStatus(req.Status).SetStorageDriver(req.StorageDriver).SetBucket(req.Bucket).SetObjectKey(req.ObjectKey).SetMimeType(req.MIMEType).
			SetFileSizeBytes(req.SizeBytes).SetSha256(req.SHA256).SetSourceTaskKind("video").SetSourceTaskID(item.TaskID).SetProcessedAt(time.Now().UTC())
		if req.StorageConfigID != "" {
			storageID, parseErr := uuid.Parse(req.StorageConfigID)
			if parseErr != nil {
				return false, fmt.Errorf("invalid storage config id: %w", parseErr)
			}
			asset.SetStorageConfigID(storageID)
		}
		if _, err := asset.Save(ctx); err != nil {
			return false, fmt.Errorf("create generated video asset: %w", err)
		}
		if _, err := tx.MediaAssetReference.Create().SetAssetID(assetID).SetRefType("video_task_result").SetRefID(item.TaskID).
			SetRefKey(item.ID.String()).SetUserID(req.UserID).Save(ctx); err != nil {
			return false, fmt.Errorf("create video result asset reference: %w", err)
		}
		if _, err := tx.MediaProcessingJob.Create().SetAssetID(assetID).SetJobType("probe").SetTransformVersion(1).
			SetStatus("pending").SetRequestedByType("video_task").SetRequestedByID(item.TaskID.String()).Save(ctx); err != nil {
			return false, fmt.Errorf("create generated video processing job: %w", err)
		}
		price, usagePending, priceErr := actualVideoItemPoints(item, item.Edges.Task)
		if priceErr != nil {
			return false, priceErr
		}
		if usagePending {
			price = "0.00000"
		}
		stage := "succeeded"
		if usagePending {
			stage = "usage_pending"
		}
		count, err := tx.VideoTaskItem.Update().Where(videotaskitem.IDEQ(itemID), videotaskitem.VersionEQ(req.ExpectedVersion), videotaskitem.LeaseOwnerEQ(req.Owner)).
			SetStatus(string(domainvideo.ItemStateSucceeded)).SetStage(stage).SetVersion(req.ExpectedVersion + 1).
			SetResultAssetID(assetID).SetActualPoints(price).ClearNextActionAt().ClearLeaseOwner().ClearLeaseExpiresAt().ClearErrorCode().ClearErrorMessage().Save(ctx)
		if err != nil || count != 1 {
			return false, err
		}
		return true, nil
	})
}

func (s *VideoTaskStore) LoadSettlement(ctx context.Context, rawTaskID string) (worker.SettlementSnapshot, error) {
	taskID, err := uuid.Parse(rawTaskID)
	if err != nil {
		return worker.SettlementSnapshot{}, fmt.Errorf("invalid video task id: %w", err)
	}
	task, err := s.client.VideoTask.Query().Where(videotask.IDEQ(taskID)).WithItems(func(query *repoent.VideoTaskItemQuery) {
		query.Order(videotaskitem.ByOrdinal())
	}).Only(ctx)
	if err != nil {
		return worker.SettlementSnapshot{}, err
	}
	result := worker.SettlementSnapshot{TaskID: task.ID.String(), ReservedPoints: task.ReservedPoints}
	for _, item := range task.Edges.Items {
		_, usagePending, usageErr := actualVideoItemPoints(item, task)
		if usageErr != nil {
			return worker.SettlementSnapshot{}, usageErr
		}
		result.Items = append(result.Items, worker.SettlementItem{State: domainvideo.ItemState(item.Status), PricePoints: itemPricePoints(item, task), UsagePending: usagePending && domainvideo.ItemState(item.Status) == domainvideo.ItemStateSucceeded})
	}
	return result, nil
}

func (s *VideoTaskStore) FinalizeTask(ctx context.Context, req worker.FinalizeRequest) (bool, error) {
	taskID, err := uuid.Parse(req.TaskID)
	if err != nil {
		return false, fmt.Errorf("invalid video task id: %w", err)
	}
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (bool, error) {
		task, err := tx.VideoTask.Get(ctx, taskID)
		if err != nil {
			return false, err
		}
		if task.SettlementStatus == "finalized" {
			return false, nil
		}
		states, err := tx.VideoTaskItem.Query().Where(videotaskitem.TaskIDEQ(taskID)).Select(videotaskitem.FieldStatus).Strings(ctx)
		if err != nil {
			return false, err
		}
		for _, state := range states {
			if !isTerminalVideoItem(domainvideo.ItemState(state)) {
				return false, nil
			}
		}
		apiKeyID := int64(0)
		if task.APIKeyID != nil {
			apiKeyID = *task.APIKeyID
		}
		if _, err := s.billing.FinalizeTaskTx(ctx, tx, billingservice.FinalizeStoreRequest{
			UserID: task.UserID, APIKeyID: apiKeyID, TaskID: task.ID.String(), EstimatedPoints: task.ReservedPoints,
			ActualPoints: req.ActualPoints, Reason: "video generation finalize",
		}, "video", map[string]any{"successful_video_count": req.SuccessOutputCount}); err != nil {
			return false, fmt.Errorf("finalize video task points: %w", err)
		}
		if _, err := tx.VideoTask.UpdateOne(task).SetStatus(string(req.Status)).SetProgressStage("completed").SetSuccessOutputCount(req.SuccessOutputCount).
			SetActualPoints(req.ActualPoints).SetSettlementStatus("finalized").SetFinishedAt(time.Now().UTC()).Save(ctx); err != nil {
			return false, err
		}
		return true, nil
	})
}

func (s *VideoTaskStore) ReleaseLease(ctx context.Context, ref worker.LeaseRef) error {
	itemID, err := uuid.Parse(ref.ItemID)
	if err != nil {
		return fmt.Errorf("invalid video item id: %w", err)
	}
	_, err = s.client.VideoTaskItem.Update().Where(videotaskitem.IDEQ(itemID), videotaskitem.LeaseOwnerEQ(ref.Owner)).ClearLeaseOwner().ClearLeaseExpiresAt().Save(ctx)
	return err
}

func projectVideoCallback(ctx context.Context, tx *repoent.Tx, now time.Time) error {
	events, err := tx.VideoProviderCallbackEvent.Query().Where(videoprovidercallbackevent.StatusEQ("received")).
		Order(repoent.Asc(videoprovidercallbackevent.FieldReceivedAt)).Limit(16).All(ctx)
	if err != nil {
		return err
	}
	for _, event := range events {
		attempt, err := tx.VideoTaskAttempt.Query().Where(videotaskattempt.ModelAccountIDEQ(event.ModelAccountID), videotaskattempt.ProviderJobIDEQ(event.ProviderJobID)).Only(ctx)
		if repoent.IsNotFound(err) {
			if _, updateErr := tx.VideoProviderCallbackEvent.UpdateOne(event).SetStatus("unmatched").SetErrorCode("attempt_not_found").SetProcessedAt(now).Save(ctx); updateErr != nil {
				return updateErr
			}
			continue
		}
		if err != nil {
			return err
		}
		item, err := tx.VideoTaskItem.Get(ctx, attempt.ItemID)
		if err != nil {
			return err
		}
		target, artifact, code, message, ok := callbackProjection(event.PayloadSnapshot)
		if !ok {
			if _, err := tx.VideoProviderCallbackEvent.UpdateOne(event).SetStatus("failed").SetErrorCode("invalid_callback_state").SetProcessedAt(now).Save(ctx); err != nil {
				return err
			}
			continue
		}
		transition, transitionErr := domainvideo.AdvanceItemState(domainvideo.ItemStateSnapshot{State: domainvideo.ItemState(item.Status), Version: item.Version}, domainvideo.ItemTransition{ExpectedVersion: item.Version, Target: target})
		if transitionErr == nil {
			update := tx.VideoTaskItem.Update().Where(videotaskitem.IDEQ(item.ID), videotaskitem.VersionEQ(item.Version)).SetStatus(string(transition.Snapshot.State)).SetStage(string(transition.Snapshot.State)).SetVersion(transition.Snapshot.Version).SetNextActionAt(now).ClearLeaseOwner().ClearLeaseExpiresAt()
			usage := callbackNormalizedUsage(event.PayloadSnapshot)
			if usage.OutputSeconds != "" {
				update.SetActualOutputSeconds(usage.OutputSeconds)
			}
			if event.PayloadSnapshot["usage"] != nil || usage.OutputSeconds != "" || usage.ProviderTokens != "" {
				providerCost, costErr := calculateProviderCost(attempt.CostSnapshot, usage)
				if costErr != nil {
					return costErr
				}
				attemptUpdate := tx.VideoTaskAttempt.UpdateOne(attempt).SetStatus(string(transition.Snapshot.State))
				if providerCost != "" {
					update.SetProviderCost(providerCost)
					attemptUpdate.SetProviderCost(providerCost)
				}
				if raw, ok := event.PayloadSnapshot["usage"].(map[string]any); ok {
					attemptUpdate.SetUsageRaw(cloneMap(raw))
				}
				if normalized, ok := event.PayloadSnapshot["usage_normalized"].(map[string]any); ok {
					attemptUpdate.SetUsageNormalized(cloneMap(normalized))
				}
				if providerStatus, ok := event.PayloadSnapshot["provider_status"].(map[string]any); ok {
					attemptUpdate.SetProviderStatusSnapshot(cloneMap(providerStatus))
				}
				if isTerminalVideoItem(transition.Snapshot.State) {
					attemptUpdate.SetFinishedAt(now)
				}
				if _, err := attemptUpdate.Save(ctx); err != nil {
					return err
				}
			}
			if artifact.URL != "" {
				snapshot, mapErr := structMap(artifact)
				if mapErr != nil {
					return mapErr
				}
				update.SetArtifactSnapshot(snapshot)
			}
			if code != "" {
				update.SetErrorCode(code)
			}
			if message != "" {
				update.SetErrorMessage(message)
			}
			if _, err := update.Save(ctx); err != nil {
				return err
			}
		}
		if _, err := tx.VideoProviderCallbackEvent.UpdateOne(event).SetStatus("processed").SetProcessedAt(now).Save(ctx); err != nil {
			return err
		}
	}
	return nil
}

func callbackNormalizedUsage(snapshot map[string]any) providervideo.Usage {
	value, _ := snapshot["usage_normalized"].(map[string]any)
	return providervideo.Usage{
		OutputSeconds: stringSnapshot(value, "output_seconds"), InputVideoSeconds: stringSnapshot(value, "input_video_seconds"),
		ReferenceImageCount: int(int64Snapshot(value, "reference_image_count")), ProviderTokens: stringSnapshot(value, "provider_tokens"),
	}
}

func loadVideoWorkItem(ctx context.Context, client *repoent.Client, itemID uuid.UUID) (worker.WorkItem, error) {
	item, err := client.VideoTaskItem.Query().Where(videotaskitem.IDEQ(itemID)).WithTask().Only(ctx)
	if err != nil {
		return worker.WorkItem{}, err
	}
	attempt, err := latestVideoAttempt(ctx, client, itemID)
	if err != nil && !repoent.IsNotFound(err) {
		return worker.WorkItem{}, err
	}
	result := worker.WorkItem{
		ID: item.ID.String(), TaskID: item.TaskID.String(), UserID: item.Edges.Task.UserID, ProjectID: item.Edges.Task.ProjectID.String(),
		ProviderCode: stringSnapshot(item.Edges.Task.RoutingSnapshot, "provider_code"), State: domainvideo.ItemState(item.Status), Version: item.Version,
		PricePoints: itemPricePoints(item, item.Edges.Task), ActualPoints: item.ActualPoints, Request: providerRequestFromTask(item.Edges.Task, item.ID),
		ArtifactAttempts: item.ArtifactAttempts, MaxArtifactAttempts: item.MaxArtifactAttempts, ErrorCode: optionalStringValue(item.ErrorCode), ErrorMessage: optionalStringValue(item.ErrorMessage),
		NeedsSettlement: isTerminalVideoItem(domainvideo.ItemState(item.Status)) && item.Edges.Task.SettlementStatus != "finalized", NextActionAt: item.NextActionAt,
	}
	inputs, err := client.VideoTaskInput.Query().Where(videotaskinput.TaskIDEQ(item.TaskID)).WithAsset().Order(repoent.Asc(videotaskinput.FieldOrdinal)).All(ctx)
	if err != nil {
		return worker.WorkItem{}, fmt.Errorf("load video worker inputs: %w", err)
	}
	for _, input := range inputs {
		asset := input.Edges.Asset
		if asset == nil {
			return worker.WorkItem{}, errors.New("video worker input asset is unavailable")
		}
		providerInput := providervideo.Input{
			AssetID: asset.ID.String(), Role: input.Role, Ordinal: input.Ordinal, StorageDriver: asset.StorageDriver,
			ObjectKey: asset.ObjectKey, MIMEType: asset.MimeType,
		}
		if asset.StorageConfigID != nil {
			providerInput.StorageConfigID = asset.StorageConfigID.String()
		}
		result.Request.Inputs = append(result.Request.Inputs, providerInput)
	}
	if item.ResultAssetID != nil {
		result.ResultAssetID = item.ResultAssetID.String()
	}
	if item.LeaseOwner != nil {
		result.LeaseOwner = *item.LeaseOwner
	}
	if item.LeaseExpiresAt != nil {
		result.LeaseExpiresAt = *item.LeaseExpiresAt
	}
	_ = mapStruct(item.ArtifactSnapshot, &result.Artifact)
	if attempt != nil {
		result.Attempt = worker.Attempt{
			ID: attempt.ID.String(), No: attempt.AttemptNo, RouteCandidateID: attempt.RouteCandidateID, AccountModelID: attempt.AccountModelID,
			ModelAccountID: attempt.ModelAccountID, ProviderCode: attempt.ProviderCode, ModelCode: attempt.ModelCode,
			IdempotencyKey: attempt.ProviderIdempotencyKey, Status: attempt.Status, PlatformAbsorbed: attempt.PlatformAbsorbed,
		}
		if attempt.ProviderJobID != nil {
			result.Attempt.JobID = *attempt.ProviderJobID
		}
	}
	return result, nil
}

func latestVideoAttempt(ctx context.Context, client *repoent.Client, itemID uuid.UUID) (*repoent.VideoTaskAttempt, error) {
	return client.VideoTaskAttempt.Query().Where(videotaskattempt.ItemIDEQ(itemID)).Order(repoent.Desc(videotaskattempt.FieldAttemptNo)).First(ctx)
}

func providerRequestFromTask(task *repoent.VideoTask, itemID uuid.UUID) providervideo.Request {
	resolution := task.Resolution
	if mapped := stringSnapshot(task.RoutingSnapshot, "mapped_resolution"); mapped != "" {
		resolution = mapped
	}
	return providervideo.Request{TaskID: task.ID.String(), ItemID: itemID.String(), TaskType: task.TaskType, Prompt: task.ExecutionPrompt, DurationSeconds: task.DurationSeconds, Resolution: resolution, AspectRatio: task.AspectRatio, GenerateAudio: task.GenerateAudio, OutputFormat: "mp4"}
}

func callbackProjection(snapshot map[string]any) (domainvideo.ItemState, providervideo.Artifact, string, string, bool) {
	state := providervideo.State(stringSnapshot(snapshot, "state"))
	var artifacts []providervideo.Artifact
	_ = mapStruct(snapshot["artifacts"], &artifacts)
	artifact := providervideo.Artifact{}
	if len(artifacts) > 0 {
		artifact = artifacts[0]
	}
	switch state {
	case providervideo.StateQueued:
		return domainvideo.ItemStateProviderQueued, artifact, "", "", true
	case providervideo.StateRunning:
		return domainvideo.ItemStateProviderRunning, artifact, "", "", true
	case providervideo.StateSucceeded:
		if artifact.URL == "" {
			return domainvideo.ItemStateFailed, artifact, "provider_artifact_missing", "provider succeeded without an artifact", true
		}
		return domainvideo.ItemStateArtifactPending, artifact, "", "", true
	case providervideo.StateFailed:
		return domainvideo.ItemStateFailed, artifact, stringSnapshot(snapshot, "error_code"), stringSnapshot(snapshot, "error_message"), true
	case providervideo.StateCancelled:
		return domainvideo.ItemStateCancelled, artifact, "", "", true
	default:
		return "", artifact, "", "", false
	}
}

func parseWorkerIDs(first, second string) (uuid.UUID, uuid.UUID, error) {
	firstID, err := uuid.Parse(first)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("invalid first UUID: %w", err)
	}
	secondID, err := uuid.Parse(second)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("invalid second UUID: %w", err)
	}
	return firstID, secondID, nil
}

func structMap(value any) (map[string]any, error) {
	var result map[string]any
	payload, err := json.Marshal(value)
	if err == nil {
		err = json.Unmarshal(payload, &result)
	}
	return result, err
}

func mapStruct(value any, target any) error {
	payload, err := json.Marshal(value)
	if err == nil {
		err = json.Unmarshal(payload, target)
	}
	return err
}

func stringSnapshot(snapshot map[string]any, key string) string {
	value, _ := snapshot[key].(string)
	return strings.TrimSpace(value)
}

func int64Snapshot(snapshot map[string]any, key string) int64 {
	switch value := snapshot[key].(type) {
	case float64:
		return int64(value)
	case int64:
		return value
	case int:
		return int64(value)
	default:
		return 0
	}
}

func itemPricePoints(item *repoent.VideoTaskItem, task *repoent.VideoTask) string {
	if item.ActualPoints != "" && item.ActualPoints != "0.00000" {
		return item.ActualPoints
	}
	if value := stringSnapshot(task.PricingSnapshot, "unit_points"); value != "" {
		return value
	}
	return "0.00000"
}

func actualVideoItemPoints(item *repoent.VideoTaskItem, task *repoent.VideoTask) (string, bool, error) {
	quoted := itemPricePoints(item, task)
	if int64Snapshot(task.PricingSnapshot, "schema_version") == 2 {
		return quoted, false, nil
	}
	var rule domainvideo.SalesRule
	if err := mapStruct(task.PricingSnapshot["sales_rule"], &rule); err != nil || rule.PricingMode == "" || rule.PricingMode == "exact" {
		return quoted, false, nil
	}
	usageSeconds, usageErr := decimal.NewFromString(strings.TrimSpace(item.ActualOutputSeconds))
	if strings.TrimSpace(item.ActualOutputSeconds) == "" || usageErr != nil || !usageSeconds.IsPositive() {
		return "0.00000", true, nil
	}
	actual, err := domainvideo.CalculateActualUnitPoints(rule, domainvideo.QuoteRequest{
		DurationSeconds: task.DurationSeconds, ReferenceImageCount: int(int64Snapshot(task.PricingSnapshot, "reference_image_count")), GenerateAudio: task.GenerateAudio, OutputCount: 1,
	}, domainvideo.ActualUsage{OutputSeconds: item.ActualOutputSeconds})
	if err != nil {
		return "", false, fmt.Errorf("calculate metered video item points: %w", err)
	}
	reserved, err := decimal.NewFromString(task.ReservedPoints)
	if err != nil || reserved.IsNegative() {
		return "", false, errors.New("video reserved points snapshot is invalid")
	}
	count := task.RequestedOutputCount
	if count < 1 {
		count = 1
	}
	itemCap := reserved.Div(decimal.NewFromInt(int64(count))).Round(5)
	value, _ := decimal.NewFromString(actual)
	if value.GreaterThan(itemCap) {
		slog.Error("metered video price exceeded reserved item cap", "task_id", task.ID, "item_id", item.ID, "actual_points", actual, "reserved_item_cap", itemCap.StringFixed(5))
		return itemCap.StringFixed(5), false, nil
	}
	return actual, false, nil
}

func stepStage(req worker.ApplyStepRequest) string {
	if req.AttemptStatus != "" {
		return req.AttemptStatus
	}
	return string(req.Target)
}

var _ worker.Store = (*VideoTaskStore)(nil)
