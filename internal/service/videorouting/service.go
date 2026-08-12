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
	RouteModelCode    string                    `json:"route_model_code"`
	Name              string                    `json:"name"`
	Description       string                    `json:"description,omitempty"`
	ConfigVersion     string                    `json:"config_version"`
	CapabilityVersion string                    `json:"capability_version"`
	MaxOutputCount    int                       `json:"max_output_count"`
	TaskTypes         []domainvideo.TaskType    `json:"task_types"`
	Combinations      []domainvideo.Combination `json:"combinations"`
}

type Resolution struct {
	Group             Group
	Request           domainvideo.Request
	Candidates        []Candidate
	CapabilityVersion string
}

type CapabilityListResponse struct {
	Groups []CapabilityResponse `json:"groups"`
}

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) Capabilities(ctx context.Context, code string) (CapabilityResponse, error) {
	group, err := s.group(ctx, code)
	if err != nil {
		return CapabilityResponse{}, err
	}
	response := CapabilityResponse{
		RouteModelCode: group.Code, Name: group.Name, Description: group.Description, ConfigVersion: group.ConfigVersion,
		CapabilityVersion: capabilityVersion(group.Candidates), MaxOutputCount: group.MaxOutputCount, TaskTypes: append([]domainvideo.TaskType(nil), group.TaskTypes...),
	}
	seen := make(map[string]struct{})
	for _, taskType := range group.TaskTypes {
		candidates := make([]domainvideo.Candidate, 0, len(group.Candidates))
		for _, candidate := range group.Candidates {
			candidates = append(candidates, domainvideo.Candidate{ID: candidate.AccountModelID, Enabled: true, Capability: candidate.Capability})
		}
		visible := domainvideo.BuildVisibleCapability(candidates, taskType)
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

func (s *Service) ListCapabilities(ctx context.Context) (CapabilityListResponse, error) {
	if s == nil || s.store == nil {
		return CapabilityListResponse{}, errs.Internal("video routing service is unavailable")
	}
	groups, err := s.store.ListVideoGroups(ctx)
	if err != nil {
		return CapabilityListResponse{}, err
	}
	response := CapabilityListResponse{Groups: make([]CapabilityResponse, 0, len(groups))}
	for _, group := range groups {
		capability, err := s.Capabilities(ctx, group.Code)
		if err != nil {
			continue
		}
		response.Groups = append(response.Groups, capability)
	}
	return response, nil
}

func (s *Service) Resolve(ctx context.Context, code string, request domainvideo.Request) (Resolution, error) {
	group, err := s.group(ctx, code)
	if err != nil {
		return Resolution{}, err
	}
	if request.OutputCount < 1 || request.OutputCount > group.MaxOutputCount {
		return Resolution{}, errs.BadRequest("video output_count exceeds the route model limit")
	}
	matched := make([]Candidate, 0, len(group.Candidates))
	var fieldErrors []domainvideo.FieldError
	for _, candidate := range group.Candidates {
		match := candidate.Capability.Match(request)
		if match.Matches {
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

func (s *Service) group(ctx context.Context, code string) (Group, error) {
	if s == nil || s.store == nil || strings.TrimSpace(code) == "" {
		return Group{}, errs.BadRequest("route_model_code is required")
	}
	group, err := s.store.GetVideoGroup(ctx, strings.TrimSpace(code))
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
