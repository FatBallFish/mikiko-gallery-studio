package videorouting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type Service struct{ store Store }

type CapabilityResponse struct {
	RouteModelCode              string                                                                         `json:"route_model_code"`
	Name                        string                                                                         `json:"name"`
	Description                 string                                                                         `json:"description,omitempty"`
	MinimumPoints               string                                                                         `json:"minimum_points"`
	IconKey                     string                                                                         `json:"icon_key,omitempty"`
	IconSVG                     string                                                                         `json:"icon_svg,omitempty"`
	GroupCodes                  []string                                                                       `json:"group_codes,omitempty"`
	ConfigVersion               string                                                                         `json:"config_version"`
	CapabilityVersion           string                                                                         `json:"capability_version"`
	MaxOutputCount              int                                                                            `json:"max_output_count"`
	TaskTypes                   []domainvideo.TaskType                                                         `json:"task_types"`
	Combinations                []domainvideo.Combination                                                      `json:"combinations"`
	InputCapabilitiesByTaskType map[domainvideo.TaskType]map[domainvideo.InputRole]domainvideo.InputCapability `json:"input_capabilities_by_task_type"`
}

// RouteModelGroupInfo mirrors the image capability payload's grouping
// metadata so both consoles render the same two-level picker.
type RouteModelGroupInfo struct {
	Code            string   `json:"code"`
	Name            string   `json:"name"`
	IconKey         string   `json:"icon_key,omitempty"`
	IconSVG         string   `json:"icon_svg,omitempty"`
	RouteModelCodes []string `json:"route_model_codes"`
}

type Resolution struct {
	Group             Group
	Request           domainvideo.Request
	Candidates        []Candidate
	CapabilityVersion string
}

type CapabilityListResponse struct {
	Groups           []CapabilityResponse  `json:"groups"`
	RouteModelGroups []RouteModelGroupInfo `json:"route_model_groups,omitempty"`
}

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) Capabilities(ctx context.Context, code string, userGroupCodes []string) (CapabilityResponse, error) {
	group, err := s.group(ctx, code, userGroupCodes)
	if err != nil {
		return CapabilityResponse{}, err
	}
	response := CapabilityResponse{
		RouteModelCode: group.Code, Name: group.Name, Description: group.Description, ConfigVersion: group.ConfigVersion,
		MinimumPoints: group.MinimumTaskPoints, CapabilityVersion: capabilityVersion(group.Candidates), MaxOutputCount: group.MaxOutputCount, TaskTypes: append([]domainvideo.TaskType(nil), group.TaskTypes...),
	}
	response.InputCapabilitiesByTaskType = make(map[domainvideo.TaskType]map[domainvideo.InputRole]domainvideo.InputCapability)
	seen := make(map[string]struct{})
	for _, taskType := range group.TaskTypes {
		candidates := make([]domainvideo.Candidate, 0, len(group.Candidates))
		for _, candidate := range group.Candidates {
			// Enumerate public resolutions through the same forward mapping
			// used by Resolve. Reversing a mapping loses identity resolutions
			// and advertises sources whose mapped target is unsupported.
			capability := candidate.Capability
			if capability.SchemaVersion != 1 || capability.ProviderNativeMaxN < 1 || capability.ProviderNativeMaxN > 10 {
				continue
			}
			task, ok := capability.TaskTypes[taskType]
			if !ok {
				continue
			}
			resolutions := make([]domainvideo.Resolution, 0)
			for _, public := range []domainvideo.Resolution{domainvideo.Resolution480P, domainvideo.Resolution720P, domainvideo.Resolution768P, domainvideo.Resolution1080P, domainvideo.Resolution2K, domainvideo.Resolution4K} {
				mapped := mapCandidateRequest(candidate, domainvideo.Request{Resolution: public}).Resolution
				for _, supported := range task.Resolutions {
					if mapped == supported {
						resolutions = append(resolutions, public)
						break
					}
				}
			}
			task.Resolutions = resolutions
			capability.TaskTypes = map[domainvideo.TaskType]domainvideo.TaskCapability{taskType: task}
			candidates = append(candidates, domainvideo.Candidate{ID: candidate.AccountModelID, Enabled: true, Capability: capability})
		}
		visible := domainvideo.BuildVisibleCapability(candidates, taskType)
		var publicInputs map[domainvideo.InputRole]domainvideo.InputCapability
		for _, candidate := range candidates {
			if len(domainvideo.BuildVisibleCapability([]domainvideo.Candidate{candidate}, taskType).Combinations) == 0 {
				continue
			}
			inputs := candidate.Capability.TaskTypes[taskType].Inputs
			if publicInputs == nil {
				publicInputs = make(map[domainvideo.InputRole]domainvideo.InputCapability)
				for role, input := range inputs {
					publicInputs[role] = input
				}
				continue
			}
			for role, input := range publicInputs {
				other, ok := inputs[role]
				if !ok {
					delete(publicInputs, role)
					continue
				}
				input.Required = input.Required || other.Required
				if input.MaxCount < 0 || (other.MaxCount >= 0 && other.MaxCount < input.MaxCount) {
					input.MaxCount = other.MaxCount
				}
				if input.MaxBytes <= 0 || (other.MaxBytes > 0 && other.MaxBytes < input.MaxBytes) {
					input.MaxBytes = other.MaxBytes
				}
				input.MediaTypes = intersectInputValues(input.MediaTypes, other.MediaTypes)
				input.Formats = intersectInputValues(input.Formats, other.Formats)
				publicInputs[role] = input
			}
		}
		if publicInputs == nil {
			publicInputs = make(map[domainvideo.InputRole]domainvideo.InputCapability)
		}
		response.InputCapabilitiesByTaskType[taskType] = publicInputs
		for _, combination := range visible.Combinations {
			combination.CandidateID = 0
			key := string(combination.TaskType) + "/" + intString(combination.DurationSeconds) + "/" + string(combination.Resolution) + "/" + string(combination.AspectRatio) + "/" + string(combination.AudioMode)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			response.Combinations = append(response.Combinations, combination)
		}
	}
	return response, nil
}

func intersectInputValues(left, right []string) []string {
	result := make([]string, 0)
	for _, value := range left {
		for _, other := range right {
			if strings.EqualFold(value, other) {
				result = append(result, value)
				break
			}
		}
	}
	return result
}

func (s *Service) ListCapabilities(ctx context.Context, userGroupCodes []string) (CapabilityListResponse, error) {
	if s == nil || s.store == nil {
		return CapabilityListResponse{}, errs.Internal("video routing service is unavailable")
	}
	groups, err := s.store.ListVideoGroups(ctx, userGroupCodes)
	if err != nil {
		return CapabilityListResponse{}, err
	}
	response := CapabilityListResponse{Groups: make([]CapabilityResponse, 0, len(groups))}
	for _, group := range groups {
		capability, err := s.Capabilities(ctx, group.Code, userGroupCodes)
		if err != nil {
			continue
		}
		response.Groups = append(response.Groups, capability)
	}
	return response, nil
}

func (s *Service) Resolve(ctx context.Context, code string, userGroupCodes []string, request domainvideo.Request) (Resolution, error) {
	group, err := s.group(ctx, code, userGroupCodes)
	if err != nil {
		return Resolution{}, err
	}
	if request.OutputCount < 1 || request.OutputCount > group.MaxOutputCount {
		return Resolution{}, errs.BadRequest("video output_count exceeds the route model limit")
	}
	matched := make([]Candidate, 0, len(group.Candidates))
	var fieldErrors []domainvideo.FieldError
	for _, candidate := range group.Candidates {
		mapped := mapCandidateRequest(candidate, request)
		match := candidate.Capability.Match(mapped)
		if match.Matches {
			candidate.MappedRequest = mapped
			matched = append(matched, candidate)
		} else if len(fieldErrors) == 0 {
			fieldErrors = match.FieldErrors
		}
	}
	if len(matched) == 0 {
		return Resolution{}, errs.WithDetails(errs.New(422, errs.CodeVideoCapabilityMismatch, "no video candidate supports the complete parameter combination"), map[string]any{"field_errors": fieldErrors})
	}
	return Resolution{Group: group, Request: request, Candidates: matched, CapabilityVersion: capabilityVersion(matched)}, nil
}

func mapCandidateRequest(candidate Candidate, request domainvideo.Request) domainvideo.Request {
	if resolution := candidate.ResolutionMappings[request.Resolution]; resolution != "" {
		request.Resolution = resolution
	}
	return request
}

func (s *Service) group(ctx context.Context, code string, userGroupCodes []string) (Group, error) {
	if s == nil || s.store == nil || strings.TrimSpace(code) == "" {
		return Group{}, errs.BadRequest("route_model_code is required")
	}
	group, err := s.store.GetVideoGroup(ctx, strings.TrimSpace(code), userGroupCodes)
	if err != nil {
		return Group{}, err
	}
	if len(group.Candidates) == 0 {
		return Group{}, errs.New(409, errs.CodeConflict, "video route model has no verified candidates")
	}
	return group, nil
}

func capabilityVersion(candidates []Candidate) string {
	versions := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		versions = append(versions, candidate.CapabilityVersion)
	}
	sort.Strings(versions)
	if len(versions) == 1 {
		return versions[0]
	}
	digest := sha256.Sum256([]byte(strings.Join(versions, "\x00")))
	return "cap-" + hex.EncodeToString(digest[:8])
}

func intString(value int) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 12)
	for value > 0 {
		digits = append(digits, byte('0'+value%10))
		value /= 10
	}
	for left, right := 0, len(digits)-1; left < right; left, right = left+1, right-1 {
		digits[left], digits[right] = digits[right], digits[left]
	}
	return string(digits)
}
