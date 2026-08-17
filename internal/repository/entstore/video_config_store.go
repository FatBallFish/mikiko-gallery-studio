package entstore

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/modelaccount"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/modelaccountmodel"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/routemodel"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/routemodelcandidate"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/routemodelvisibilitygroup"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/usergroup"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videomodelcapability"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videorouteconfig"
	videoroutingservice "github.com/fatballfish/pic-gallery/internal/service/videorouting"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type VideoConfigStore struct{ client *repoent.Client }

func NewVideoConfigStore(client *repoent.Client) *VideoConfigStore {
	return &VideoConfigStore{client: client}
}

func (s *VideoConfigStore) GetVideoGroup(ctx context.Context, code string, userGroupCodes []string) (videoroutingservice.Group, error) {
	entity, err := s.client.RouteModel.Query().Where(
		routemodel.CodeEQ(code), routemodel.MediaTypeEQ("video"), routemodel.EnabledEQ(true), routemodel.DeletedAtIsNil(),
	).Only(ctx)
	if repoent.IsNotFound(err) {
		return videoroutingservice.Group{}, errs.New(404, errs.CodeNotFound, "video route model not found")
	}
	if err != nil {
		return videoroutingservice.Group{}, err
	}
	visible, err := s.videoRouteVisible(ctx, int64(entity.ID), entity.Visibility, userGroupCodes)
	if err != nil {
		return videoroutingservice.Group{}, err
	}
	if !visible {
		return videoroutingservice.Group{}, errs.New(403, errs.CodeModelRouteNotVisible, "video route model is not visible")
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
		ConfigVersion: config.ConfigVersion, MinimumTaskPoints: config.MinimumTaskPoints, RoundingStepPoints: config.RoundingStepPoints, MaxOutputCount: config.MaxOutputCount,
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
			ResolutionMappings: videoResolutionMappings(config.CandidateParameterMappings, int64(accountModel.ID)),
		})
	}
	group.TaskTypes = deriveVideoGroupTaskTypes(group.Candidates)
	return group, nil
}

func deriveVideoGroupTaskTypes(candidates []videoroutingservice.Candidate) []domainvideo.TaskType {
	seen := make(map[domainvideo.TaskType]struct{})
	for _, candidate := range candidates {
		for taskType := range candidate.Capability.TaskTypes {
			seen[taskType] = struct{}{}
		}
	}
	taskTypes := make([]domainvideo.TaskType, 0, len(seen))
	for taskType := range seen {
		taskTypes = append(taskTypes, taskType)
	}
	sort.Slice(taskTypes, func(i, j int) bool { return taskTypes[i] < taskTypes[j] })
	return taskTypes
}

func videoResolutionMappings(value map[string]any, accountModelID int64) map[domainvideo.Resolution]domainvideo.Resolution {
	decoded := decodeCandidateResolutionMappings(value, accountModelID)
	result := make(map[domainvideo.Resolution]domainvideo.Resolution, len(decoded))
	for source, target := range decoded {
		result[domainvideo.Resolution(source)] = domainvideo.Resolution(target)
	}
	return result
}

func (s *VideoConfigStore) ListVideoGroups(ctx context.Context, userGroupCodes []string) ([]videoroutingservice.Group, error) {
	entities, err := s.client.RouteModel.Query().Where(
		routemodel.MediaTypeEQ("video"), routemodel.EnabledEQ(true), routemodel.DeletedAtIsNil(),
	).Order(repoent.Asc(routemodel.FieldSortOrder), repoent.Asc(routemodel.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	groups := make([]videoroutingservice.Group, 0, len(entities))
	for _, entity := range entities {
		group, err := s.GetVideoGroup(ctx, entity.Code, userGroupCodes)
		if err != nil {
			continue
		}
		groups = append(groups, group)
	}
	return groups, nil
}

func (s *VideoConfigStore) videoRouteVisible(ctx context.Context, routeModelID int64, visibility string, userGroupCodes []string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(visibility)) {
	case "public":
		return true, nil
	case "groups":
		codes := make([]string, 0, len(userGroupCodes))
		seen := make(map[string]struct{}, len(userGroupCodes))
		for _, value := range userGroupCodes {
			code := strings.ToLower(strings.TrimSpace(value))
			if code == "" {
				continue
			}
			if _, exists := seen[code]; exists {
				continue
			}
			seen[code] = struct{}{}
			codes = append(codes, code)
		}
		if len(codes) == 0 {
			return false, nil
		}
		groups, err := s.client.UserGroup.Query().Where(usergroup.GroupCodeIn(codes...), usergroup.StatusEQ("enabled")).All(ctx)
		if err != nil {
			return false, err
		}
		groupIDs := make([]int64, 0, len(groups))
		for _, group := range groups {
			groupIDs = append(groupIDs, int64(group.ID))
		}
		if len(groupIDs) == 0 {
			return false, nil
		}
		count, err := s.client.RouteModelVisibilityGroup.Query().Where(
			routemodelvisibilitygroup.RouteModelIDEQ(routeModelID),
			routemodelvisibilitygroup.GroupIDIn(groupIDs...),
		).Count(ctx)
		return count > 0, err
	default:
		return false, nil
	}
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
