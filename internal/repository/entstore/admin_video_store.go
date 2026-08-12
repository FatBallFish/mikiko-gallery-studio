package entstore

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/configitem"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaprocessingjob"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/routemodel"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/routemodelcandidate"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/subscriptionplan"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videomodelcapability"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videopricerule"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videopricingstrategy"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videoprovidercostrule"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videorouteconfig"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videotask"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videotaskitem"
	adminvideoservice "github.com/fatballfish/pic-gallery/internal/service/adminvideo"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

const (
	mediaPolicyCategory = "media_policy"
	mediaPolicyKey      = "policy"
)

type AdminVideoStore struct{ client *repoent.Client }

func NewAdminVideoStore(client *repoent.Client) *AdminVideoStore {
	return &AdminVideoStore{client: client}
}

func (s *AdminVideoStore) Snapshot(ctx context.Context) (adminvideoservice.Snapshot, error) {
	capabilities, err := s.client.VideoModelCapability.Query().Where(videomodelcapability.DeletedAtIsNil()).All(ctx)
	if err != nil {
		return adminvideoservice.Snapshot{}, err
	}
	costRules, err := s.client.VideoProviderCostRule.Query().Where(videoprovidercostrule.DeletedAtIsNil()).All(ctx)
	if err != nil {
		return adminvideoservice.Snapshot{}, err
	}
	strategies, err := s.client.VideoPricingStrategy.Query().Where(videopricingstrategy.DeletedAtIsNil()).All(ctx)
	if err != nil {
		return adminvideoservice.Snapshot{}, err
	}
	priceRules, err := s.client.VideoPriceRule.Query().Where(videopricerule.DeletedAtIsNil()).All(ctx)
	if err != nil {
		return adminvideoservice.Snapshot{}, err
	}
	routeConfigs, err := s.client.VideoRouteConfig.Query().Where(videorouteconfig.DeletedAtIsNil()).All(ctx)
	if err != nil {
		return adminvideoservice.Snapshot{}, err
	}

	snapshot := adminvideoservice.Snapshot{GeneratedAt: time.Now().UTC()}
	for _, row := range capabilities {
		snapshot.Capabilities = append(snapshot.Capabilities, adminvideoservice.CapabilitySummary{AccountModelID: row.AccountModelID, Version: row.CapabilityVersion, ValidationState: row.ValidationStatus, Capability: row.CapabilityJSON, Enabled: row.Enabled})
	}
	for _, row := range costRules {
		snapshot.CostRules = append(snapshot.CostRules, adminvideoservice.CostRuleSummary{ID: int64(row.ID), AccountModelID: row.AccountModelID, BillingMode: row.BillingMode, RuleVersion: row.RuleVersion, Currency: row.Currency, Rates: row.RatesJSON, Validation: row.ValidationStatus, EffectiveAt: row.EffectiveAt, ExpiresAt: row.ExpiresAt, Enabled: row.Enabled})
	}
	for _, row := range strategies {
		snapshot.Strategies = append(snapshot.Strategies, adminvideoservice.PricingStrategySummary{ID: int64(row.ID), Code: row.Code, Name: row.Name, StrategyVersion: row.StrategyVersion, MinimumNetPointIncomeCNY: row.MinimumNetPointIncomeCny, TargetMarginRate: row.TargetMarginRate, ProviderCostBufferRate: row.ProviderCostBufferRate, PaymentFeeRate: row.PaymentFeeRate, PlatformFixedCostCNY: row.PlatformFixedCostCny, PlatformOutputSecondCostCNY: row.PlatformOutputSecondCostCny, PlatformReferenceCostCNY: row.PlatformReferenceCostCny, Enabled: row.Enabled})
	}
	for _, row := range priceRules {
		salesPoints := row.MinimumTaskPoints
		if salesPoints == "0.00000" {
			salesPoints = row.OutputSecondPoints
		}
		snapshot.PriceRules = append(snapshot.PriceRules, adminvideoservice.PriceRuleSummary{ID: int64(row.ID), StrategyID: row.PricingStrategyID, TaskType: row.TaskType, Resolution: row.Resolution, AudioMode: row.AudioMode, RuleVersion: row.RuleVersion, SafetyPoints: row.SafetyPoints, SalesPoints: salesPoints, CandidateCostUpperCNY: row.CandidateCostUpperCny, Enabled: row.Enabled})
	}
	for _, row := range routeConfigs {
		route, routeErr := s.client.RouteModel.Query().Where(routemodel.IDEQ(int(row.RouteModelID)), routemodel.DeletedAtIsNil()).Only(ctx)
		if repoent.IsNotFound(routeErr) {
			continue
		}
		if routeErr != nil {
			return adminvideoservice.Snapshot{}, routeErr
		}
		candidateRows, countErr := s.client.RouteModelCandidate.Query().Where(routemodelcandidate.RouteModelIDEQ(row.RouteModelID), routemodelcandidate.EnabledEQ(true), routemodelcandidate.DeletedAtIsNil()).All(ctx)
		if countErr != nil {
			return adminvideoservice.Snapshot{}, countErr
		}
		candidateIDs := make([]int64, 0, len(candidateRows))
		for _, candidate := range candidateRows {
			candidateIDs = append(candidateIDs, candidate.AccountModelID)
		}
		snapshot.Routes = append(snapshot.Routes, adminvideoservice.RouteConfigSummary{RouteModelID: row.RouteModelID, RouteCode: route.Code, RouteName: route.Name, ConfigVersion: row.ConfigVersion, PricingStrategyID: row.PricingStrategyID, CandidateCount: len(candidateRows), CandidateAccountModelIDs: candidateIDs, TaskTypes: row.TaskTypes, VisibleOptions: row.VisibleOptions, Defaults: row.Defaults, MaxOutputCount: row.MaxOutputCount, Enabled: row.Enabled && route.Enabled})
	}
	plans, err := s.client.SubscriptionPlan.Query().Where(subscriptionplan.PlanTypeEQ("points_package"), subscriptionplan.PurchaseEnabledEQ(true), subscriptionplan.StatusEQ("active"), subscriptionplan.CurrencyEQ("CNY")).All(ctx)
	if err != nil {
		return adminvideoservice.Snapshot{}, err
	}
	for _, plan := range plans {
		snapshot.Plans = append(snapshot.Plans, adminvideoservice.PointProduct{ID: int64(plan.ID), Code: plan.PlanCode, PriceCNY: plan.PriceCny, Points: plan.Points, BonusPoints: plan.BonusPoints, Enabled: true})
	}
	return snapshot, nil
}

func (s *AdminVideoStore) ListTasks(ctx context.Context, filter adminvideoservice.TaskFilter) (adminvideoservice.TaskPage, error) {
	query := s.client.VideoTask.Query().Where(videotask.DeletedAtIsNil()).WithItems(func(items *repoent.VideoTaskItemQuery) {
		items.Order(repoent.Asc(videotaskitem.FieldOrdinal)).WithAttempts()
	})
	if filter.UserID > 0 {
		query.Where(videotask.UserIDEQ(filter.UserID))
	}
	if filter.TaskID != nil {
		query.Where(videotask.IDEQ(*filter.TaskID))
	}
	if filter.ProjectID != nil {
		query.Where(videotask.ProjectIDEQ(*filter.ProjectID))
	}
	if filter.RouteModelID > 0 {
		query.Where(videotask.RouteModelIDEQ(filter.RouteModelID))
	}
	if strings.TrimSpace(filter.Status) != "" {
		query.Where(videotask.StatusEQ(strings.TrimSpace(filter.Status)))
	}
	if filter.From != nil {
		query.Where(videotask.CreatedAtGTE(*filter.From))
	}
	if filter.To != nil {
		query.Where(videotask.CreatedAtLTE(*filter.To))
	}
	if cursor := strings.TrimSpace(filter.Cursor); cursor != "" {
		cursorID, parseErr := uuid.Parse(cursor)
		if parseErr != nil {
			return adminvideoservice.TaskPage{}, errs.BadRequest("invalid video task cursor")
		}
		anchor, anchorErr := s.client.VideoTask.Query().Where(videotask.IDEQ(cursorID), videotask.DeletedAtIsNil()).Only(ctx)
		if repoent.IsNotFound(anchorErr) {
			return adminvideoservice.TaskPage{}, errs.BadRequest("invalid video task cursor")
		}
		if anchorErr != nil {
			return adminvideoservice.TaskPage{}, anchorErr
		}
		query.Where(videotask.Or(videotask.CreatedAtLT(anchor.CreatedAt), videotask.And(videotask.CreatedAtEQ(anchor.CreatedAt), videotask.IDLT(anchor.ID))))
	}
	rows, err := query.Order(repoent.Desc(videotask.FieldCreatedAt), repoent.Desc(videotask.FieldID)).All(ctx)
	if err != nil {
		return adminvideoservice.TaskPage{}, err
	}
	page := adminvideoservice.TaskPage{}
	for _, row := range rows {
		if !adminVideoTaskMatchesAttempts(row, filter) {
			continue
		}
		page.Items = append(page.Items, mapAdminVideoTaskSummary(row))
		if len(page.Items) == filter.Limit {
			page.NextCursor = row.ID.String()
			break
		}
	}
	return page, nil
}

func adminVideoTaskMatchesAttempts(row *repoent.VideoTask, filter adminvideoservice.TaskFilter) bool {
	if strings.TrimSpace(filter.ProviderTaskID) == "" && filter.AccountModelID == 0 {
		return true
	}
	for _, item := range row.Edges.Items {
		for _, attempt := range item.Edges.Attempts {
			if filter.AccountModelID > 0 && attempt.AccountModelID != filter.AccountModelID {
				continue
			}
			if filter.ProviderTaskID != "" && (attempt.ProviderJobID == nil || *attempt.ProviderJobID != filter.ProviderTaskID) {
				continue
			}
			return true
		}
	}
	return false
}

func (s *AdminVideoStore) GetTask(ctx context.Context, id uuid.UUID) (adminvideoservice.TaskDetail, error) {
	row, err := s.client.VideoTask.Query().Where(videotask.IDEQ(id), videotask.DeletedAtIsNil()).WithItems(func(items *repoent.VideoTaskItemQuery) {
		items.Order(repoent.Asc(videotaskitem.FieldOrdinal)).WithAttempts()
	}).Only(ctx)
	if repoent.IsNotFound(err) {
		return adminvideoservice.TaskDetail{}, errs.New(404, errs.CodeNotFound, "video task not found")
	}
	if err != nil {
		return adminvideoservice.TaskDetail{}, err
	}
	detail := adminvideoservice.TaskDetail{TaskSummary: mapAdminVideoTaskSummary(row), PricingSnapshot: row.PricingSnapshot, RoutingSnapshot: row.RoutingSnapshot, ReservedPoints: row.ReservedPoints}
	for _, item := range row.Edges.Items {
		mapped := adminvideoservice.TaskItem{ID: item.ID, Ordinal: item.Ordinal, Status: item.Status, Stage: item.Stage, ResultAssetID: item.ResultAssetID, ActualPoints: item.ActualPoints, ProviderCost: item.ProviderCost, ArtifactSnapshot: item.ArtifactSnapshot}
		if item.ErrorCode != nil {
			mapped.ErrorCode = *item.ErrorCode
		}
		if item.ErrorMessage != nil {
			mapped.ErrorMessage = *item.ErrorMessage
		}
		for _, attempt := range item.Edges.Attempts {
			a := adminvideoservice.Attempt{ID: attempt.ID, ItemID: attempt.ItemID, AttemptNo: attempt.AttemptNo, ProviderCode: attempt.ProviderCode, ModelCode: attempt.ModelCode, Status: attempt.Status, UsageRaw: attempt.UsageRaw, UsageNormalized: attempt.UsageNormalized, CostSnapshot: attempt.CostSnapshot, ProviderCost: attempt.ProviderCost, StartedAt: attempt.StartedAt, FinishedAt: attempt.FinishedAt}
			if attempt.ProviderJobID != nil {
				a.ProviderJobID = *attempt.ProviderJobID
			}
			if attempt.ErrorCategory != nil {
				a.ErrorCategory = *attempt.ErrorCategory
			}
			if attempt.ErrorCode != nil {
				a.ErrorCode = *attempt.ErrorCode
			}
			if attempt.ErrorMessage != nil {
				a.ErrorMessage = *attempt.ErrorMessage
			}
			mapped.Attempts = append(mapped.Attempts, a)
		}
		detail.Items = append(detail.Items, mapped)
	}
	return detail, nil
}

func mapAdminVideoTaskSummary(row *repoent.VideoTask) adminvideoservice.TaskSummary {
	return adminvideoservice.TaskSummary{ID: row.ID, UserID: row.UserID, ProjectID: row.ProjectID, RouteModelID: row.RouteModelID, RouteModelCode: row.RouteModelCode, Status: row.Status, SettlementStatus: row.SettlementStatus, EstimatedPoints: row.EstimatedPoints, ActualPoints: row.ActualPoints, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func (s *AdminVideoStore) Retry(ctx context.Context, request adminvideoservice.RetryRequest) error {
	switch request.Kind {
	case adminvideoservice.RetryArtifact:
		query := s.client.VideoTaskItem.Update().Where(
			videotaskitem.TaskIDEQ(request.TaskID),
			videotaskitem.Or(videotaskitem.StatusIn("artifact_failed", "artifact_retry"), videotaskitem.StageEQ("artifact_failed")),
		)
		if request.ItemID != uuid.Nil {
			query.Where(videotaskitem.IDEQ(request.ItemID))
		}
		count, err := query.SetStatus("artifact_pending").SetStage("artifact").SetNextActionAt(time.Now().UTC()).ClearLeaseOwner().ClearLeaseExpiresAt().Save(ctx)
		if err != nil {
			return err
		}
		if count == 0 {
			return errs.New(404, errs.CodeNotFound, "video task item not found")
		}
		return nil
	case adminvideoservice.RetryDerivative:
		job, err := s.client.MediaProcessingJob.Query().Where(mediaprocessingjob.IDEQ(request.JobID)).Only(ctx)
		if repoent.IsNotFound(err) {
			return errs.New(404, errs.CodeNotFound, "media processing job not found")
		}
		if err != nil {
			return err
		}
		if job.Status != "failed" && job.Status != "retry" {
			return errs.New(409, errs.CodeConflict, "only failed derivative processing can be retried")
		}
		_, err = job.Update().SetStatus("pending").SetNextRetryAt(time.Now().UTC()).ClearErrorCode().ClearErrorMessage().ClearLeaseOwner().ClearLeaseExpiresAt().Save(ctx)
		return err
	case adminvideoservice.RetrySettlement:
		return s.retrySettlement(ctx, request.TaskID)
	default:
		return errs.BadRequest("unsupported video recovery kind")
	}
}

func (s *AdminVideoStore) retrySettlement(ctx context.Context, taskID uuid.UUID) error {
	_, err := withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (struct{}, error) {
		task, err := tx.VideoTask.Query().Where(videotask.IDEQ(taskID), videotask.DeletedAtIsNil()).WithItems().Only(ctx)
		if repoent.IsNotFound(err) {
			return struct{}{}, errs.New(404, errs.CodeNotFound, "video task not found")
		}
		if err != nil {
			return struct{}{}, err
		}
		if task.SettlementStatus == "finalized" {
			return struct{}{}, errs.New(409, errs.CodeConflict, "video task settlement is already finalized")
		}
		if len(task.Edges.Items) == 0 {
			return struct{}{}, errs.New(409, errs.CodeConflict, "video task has no result items to settle")
		}
		terminal := map[string]bool{"succeeded": true, "failed": true, "cancelled": true}
		for _, item := range task.Edges.Items {
			if !terminal[item.Status] {
				return struct{}{}, errs.New(409, errs.CodeConflict, "video task is still running and cannot be settled manually")
			}
		}
		now := time.Now().UTC()
		_, err = tx.VideoTaskItem.Update().Where(videotaskitem.TaskIDEQ(taskID)).SetNextActionAt(now).ClearLeaseOwner().ClearLeaseExpiresAt().Save(ctx)
		return struct{}{}, err
	})
	return err
}

func (s *AdminVideoStore) GetMediaPolicy(ctx context.Context) (adminvideoservice.MediaPolicy, error) {
	row, err := s.client.ConfigItem.Query().Where(configitem.ConfigCategoryEQ(mediaPolicyCategory), configitem.ConfigKeyEQ(mediaPolicyKey), configitem.ScopeEQ("global")).Only(ctx)
	if repoent.IsNotFound(err) {
		return adminvideoservice.DefaultMediaPolicy(), nil
	}
	if err != nil {
		return adminvideoservice.MediaPolicy{}, err
	}
	payload, err := json.Marshal(row.ConfigValue)
	if err != nil {
		return adminvideoservice.MediaPolicy{}, err
	}
	var policy adminvideoservice.MediaPolicy
	if err := json.Unmarshal(payload, &policy); err != nil {
		return adminvideoservice.MediaPolicy{}, err
	}
	policy.Version = row.Version
	return policy, nil
}

func (s *AdminVideoStore) SaveMediaPolicy(ctx context.Context, policy adminvideoservice.MediaPolicy, updatedBy int64) (adminvideoservice.MediaPolicy, error) {
	payload, err := json.Marshal(policy)
	if err != nil {
		return adminvideoservice.MediaPolicy{}, err
	}
	value := map[string]any{}
	if err := json.Unmarshal(payload, &value); err != nil {
		return adminvideoservice.MediaPolicy{}, err
	}
	delete(value, "version")
	now := time.Now().UTC()
	row, err := s.client.ConfigItem.Query().Where(configitem.ConfigCategoryEQ(mediaPolicyCategory), configitem.ConfigKeyEQ(mediaPolicyKey), configitem.ScopeEQ("global")).Only(ctx)
	if repoent.IsNotFound(err) {
		_, err = s.client.ConfigItem.Create().SetConfigCategory(mediaPolicyCategory).SetConfigKey(mediaPolicyKey).SetScope("global").SetConfigValue(value).SetVersion(policy.Version).SetUpdatedBy(updatedBy).SetUpdatedAt(now).Save(ctx)
		return policy, err
	}
	if err != nil {
		return adminvideoservice.MediaPolicy{}, err
	}
	if row.Version+1 != policy.Version {
		return adminvideoservice.MediaPolicy{}, errs.New(409, errs.CodeConflict, "media policy version conflict")
	}
	_, err = row.Update().SetConfigValue(value).SetVersion(policy.Version).SetUpdatedBy(updatedBy).SetUpdatedAt(now).Save(ctx)
	return policy, err
}

func (s *AdminVideoStore) Readiness(ctx context.Context, now time.Time) (adminvideoservice.ReadinessSnapshot, error) {
	snapshot, err := s.Snapshot(ctx)
	if err != nil {
		return adminvideoservice.ReadinessSnapshot{}, err
	}
	result := adminvideoservice.ReadinessSnapshot{}
	for _, route := range snapshot.Routes {
		if !route.Enabled {
			continue
		}
		result.EnabledVideoRoutes++
		if route.CandidateCount == 0 {
			result.RoutesMissingCandidate++
		}
		hasPrice := false
		for _, rule := range snapshot.PriceRules {
			if rule.Enabled && rule.StrategyID == route.PricingStrategyID {
				hasPrice = true
				break
			}
		}
		if !hasPrice {
			result.VisibleCombosMissingPrice++
		}
	}
	artifact, err := s.client.VideoTaskItem.Query().Where(videotaskitem.StatusIn("artifact_pending", "artifact_failed", "artifact_retry")).Count(ctx)
	if err != nil {
		return result, err
	}
	derivative, err := s.client.MediaProcessingJob.Query().Where(mediaprocessingjob.StatusIn("pending", "retry", "failed")).Count(ctx)
	if err != nil {
		return result, err
	}
	settlement, err := s.client.VideoTask.Query().Where(videotask.SettlementStatusIn("reserved", "pending", "refund_pending"), videotask.UpdatedAtLT(now.Add(-10*time.Minute)), videotask.DeletedAtIsNil()).Count(ctx)
	if err != nil {
		return result, err
	}
	result.ArtifactBacklog, result.DerivativeBacklog, result.SettlementBacklog = artifact, derivative, settlement
	return result, nil
}
