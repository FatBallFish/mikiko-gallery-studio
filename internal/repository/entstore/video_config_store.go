package entstore

import (
	"context"
	"encoding/json"
	"time"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/modelaccount"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/modelaccountmodel"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/routemodel"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/routemodelcandidate"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videomodelcapability"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videopricerule"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videopricingstrategy"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videorouteconfig"
	videopricingservice "github.com/fatballfish/pic-gallery/internal/service/videopricing"
	videoroutingservice "github.com/fatballfish/pic-gallery/internal/service/videorouting"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type VideoConfigStore struct{ client *repoent.Client }

func NewVideoConfigStore(client *repoent.Client) *VideoConfigStore {
	return &VideoConfigStore{client: client}
}

func (s *VideoConfigStore) GetVideoGroup(ctx context.Context, code string) (videoroutingservice.Group, error) {
	entity, err := s.client.RouteModel.Query().Where(
		routemodel.CodeEQ(code), routemodel.MediaTypeEQ("video"), routemodel.EnabledEQ(true), routemodel.VisibilityEQ("public"), routemodel.DeletedAtIsNil(),
	).Only(ctx)
	if repoent.IsNotFound(err) {
		return videoroutingservice.Group{}, errs.New(404, errs.CodeNotFound, "video route model not found")
	}
	if err != nil {
		return videoroutingservice.Group{}, err
	}
	config, err := s.client.VideoRouteConfig.Query().Where(
		videorouteconfig.RouteModelIDEQ(int64(entity.ID)), videorouteconfig.EnabledEQ(true), videorouteconfig.DeletedAtIsNil(),
	).Only(ctx)
	if repoent.IsNotFound(err) {
		return videoroutingservice.Group{}, errs.New(409, errs.CodeConflict, "video route configuration is unavailable")
	}
	if err != nil {
		return videoroutingservice.Group{}, err
	}
	group := videoroutingservice.Group{
		RouteModelID: int64(entity.ID), Code: entity.Code, Name: entity.Name, Description: entity.Description,
		ConfigVersion: config.ConfigVersion, PricingStrategyID: legacyPricingStrategyID(config.VisibleOptions), MaxOutputCount: config.MaxOutputCount,
	}
	group.PricingBindings = decodeVideoPricingBindings(config.VisibleOptions)
	for _, value := range config.TaskTypes {
		group.TaskTypes = append(group.TaskTypes, domainvideo.TaskType(value))
	}
	candidates, err := s.client.RouteModelCandidate.Query().Where(
		routemodelcandidate.RouteModelIDEQ(int64(entity.ID)), routemodelcandidate.EnabledEQ(true), routemodelcandidate.DeletedAtIsNil(),
	).Order(repoent.Asc(routemodelcandidate.FieldPriority), repoent.Asc(routemodelcandidate.FieldFallbackOrder)).All(ctx)
	if err != nil {
		return videoroutingservice.Group{}, err
	}
	for _, candidate := range candidates {
		accountModel, err := s.client.ModelAccountModel.Query().Where(
			modelaccountmodel.IDEQ(int(candidate.AccountModelID)), modelaccountmodel.EnabledEQ(true), modelaccountmodel.DeletedAtIsNil(),
		).Only(ctx)
		if repoent.IsNotFound(err) {
			continue
		}
		if err != nil {
			return videoroutingservice.Group{}, err
		}
		account, err := s.client.ModelAccount.Query().Where(
			modelaccount.IDEQ(int(accountModel.AccountID)), modelaccount.StatusEQ("enabled"), modelaccount.DeletedAtIsNil(),
		).Only(ctx)
		if repoent.IsNotFound(err) {
			continue
		}
		if err != nil {
			return videoroutingservice.Group{}, err
		}
		capabilityEntity, err := s.client.VideoModelCapability.Query().Where(
			videomodelcapability.AccountModelIDEQ(int64(accountModel.ID)), videomodelcapability.EnabledEQ(true),
			videomodelcapability.ValidationStatusEQ("verified"), videomodelcapability.DeletedAtIsNil(),
		).Only(ctx)
		if repoent.IsNotFound(err) {
			continue
		}
		if err != nil {
			return videoroutingservice.Group{}, err
		}
		capability, err := decodeVideoCapability(capabilityEntity.CapabilityJSON)
		if err != nil {
			return videoroutingservice.Group{}, err
		}
		group.Candidates = append(group.Candidates, videoroutingservice.Candidate{
			RouteCandidateID: int64(candidate.ID), AccountModelID: int64(accountModel.ID), ModelAccountID: int64(account.ID),
			ModelCode: accountModel.ModelCode, AdapterType: account.AdapterType,
			CapabilityVersion: capabilityEntity.CapabilityVersion, Capability: capability,
		})
	}
	return group, nil
}

func legacyPricingStrategyID(options map[string]any) int64 {
	switch value := options["legacy_pricing_strategy_id"].(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	default:
		return 0
	}
}

func decodeVideoPricingBindings(options map[string]any) []videoroutingservice.PricingBinding {
	raw, err := json.Marshal(options["pricing_bindings"])
	if err != nil {
		return nil
	}
	var values []struct {
		TaskType          string `json:"task_type"`
		Resolution        string `json:"resolution"`
		AspectRatio       string `json:"aspect_ratio"`
		AudioMode         string `json:"audio_mode"`
		DurationSeconds   int    `json:"duration_seconds"`
		PricingStrategyID int64  `json:"pricing_strategy_id"`
	}
	if json.Unmarshal(raw, &values) != nil {
		return nil
	}
	result := make([]videoroutingservice.PricingBinding, 0, len(values))
	for _, value := range values {
		if value.PricingStrategyID <= 0 || value.TaskType == "" || value.Resolution == "" || value.AudioMode == "" {
			continue
		}
		result = append(result, videoroutingservice.PricingBinding{TaskType: domainvideo.TaskType(value.TaskType), Resolution: domainvideo.Resolution(value.Resolution), AspectRatio: domainvideo.AspectRatio(value.AspectRatio), AudioMode: domainvideo.AudioMode(value.AudioMode), DurationSeconds: value.DurationSeconds, PricingStrategyID: value.PricingStrategyID})
	}
	return result
}

func (s *VideoConfigStore) ListVideoGroups(ctx context.Context) ([]videoroutingservice.Group, error) {
	entities, err := s.client.RouteModel.Query().Where(
		routemodel.MediaTypeEQ("video"), routemodel.EnabledEQ(true), routemodel.VisibilityEQ("public"), routemodel.DeletedAtIsNil(),
	).Order(repoent.Asc(routemodel.FieldSortOrder), repoent.Asc(routemodel.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	groups := make([]videoroutingservice.Group, 0, len(entities))
	for _, entity := range entities {
		group, err := s.GetVideoGroup(ctx, entity.Code)
		if err != nil {
			continue
		}
		groups = append(groups, group)
	}
	return groups, nil
}

func (s *VideoConfigStore) GetVideoPriceRule(ctx context.Context, strategyID int64, taskType domainvideo.TaskType, resolution domainvideo.Resolution, audioMode domainvideo.AudioMode, now time.Time) (videopricingservice.Rule, error) {
	strategy, err := s.client.VideoPricingStrategy.Query().Where(
		videopricingstrategy.IDEQ(int(strategyID)), videopricingstrategy.EnabledEQ(true), videopricingstrategy.DeletedAtIsNil(),
	).Only(ctx)
	if repoent.IsNotFound(err) {
		return videopricingservice.Rule{}, errs.New(409, errs.CodeConflict, "video pricing strategy is unavailable")
	}
	if err != nil {
		return videopricingservice.Rule{}, err
	}
	rule, err := s.client.VideoPriceRule.Query().Where(
		videopricerule.PricingStrategyIDEQ(strategyID), videopricerule.TaskTypeEQ(string(taskType)),
		videopricerule.ResolutionEQ(string(resolution)), videopricerule.AudioModeEQ(string(audioMode)),
		videopricerule.EnabledEQ(true), videopricerule.EffectiveAtLTE(now), videopricerule.DeletedAtIsNil(),
		videopricerule.Or(videopricerule.ExpiresAtIsNil(), videopricerule.ExpiresAtGT(now)),
	).Order(repoent.Desc(videopricerule.FieldRuleVersion)).First(ctx)
	if repoent.IsNotFound(err) {
		return videopricingservice.Rule{}, errs.New(409, errs.CodeConflict, "video price rule is unavailable for this combination")
	}
	if err != nil {
		return videopricingservice.Rule{}, err
	}
	return videopricingservice.Rule{
		StrategyID: strategyID, StrategyVersion: strategy.StrategyVersion, RuleVersion: rule.RuleVersion, SafetyPoints: rule.SafetyPoints,
		SalesRule: domainvideo.SalesRule{
			PricingMode:     rule.PricingMode,
			FixedTaskPoints: rule.FixedTaskPoints, OutputSecondPoints: rule.OutputSecondPoints, ReferenceImagePoints: rule.ReferenceImagePoints,
			InputVideoSecondPoints: rule.InputVideoSecondPoints, ReferenceAudioSecondPoints: rule.ReferenceAudioSecondPoints,
			GeneratedAudioFixedPoints: rule.GeneratedAudioFixedPoints, GeneratedAudioSecondPoints: rule.GeneratedAudioSecondPoints,
			MinimumBillableSeconds: rule.MinimumBillableSeconds, MinimumTaskPoints: rule.MinimumTaskPoints, ReserveMarkup: rule.ReserveMarkup,
		},
	}, nil
}

func decodeVideoCapability(value map[string]any) (domainvideo.Capability, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return domainvideo.Capability{}, err
	}
	var capability domainvideo.Capability
	if err := json.Unmarshal(payload, &capability); err != nil {
		return domainvideo.Capability{}, errs.BadRequest("invalid video capability json")
	}
	return capability, nil
}
