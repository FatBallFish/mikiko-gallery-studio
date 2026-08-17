package adminvideo

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type ConfigKind string

const (
	ConfigCapability ConfigKind = "capability"
	ConfigRoute      ConfigKind = "route"
)

func (s *Service) DeleteConfig(ctx context.Context, kind ConfigKind, id, expected int64) error {
	if id <= 0 || expected < 0 {
		return errs.BadRequest("invalid video config delete request")
	}
	return s.store.DeleteVideoConfig(ctx, kind, id, expected)
}

type CapabilityWrite struct {
	AccountModelID    int64          `json:"account_model_id"`
	ExpectedVersion   string         `json:"expected_version"`
	CapabilityVersion string         `json:"capability_version"`
	Capability        map[string]any `json:"capability"`
	ValidationStatus  string         `json:"validation_status"`
	Enabled           bool           `json:"enabled"`
}

type RateCardWrite struct {
	AccountModelID      int64          `json:"account_model_id"`
	ProviderCode        string         `json:"-"`
	PricingSchema       string         `json:"pricing_schema"`
	ExpectedRateVersion int            `json:"expected_rate_version"`
	Currency            string         `json:"-"`
	RateConfig          map[string]any `json:"rate_config"`
	SourceReference     string         `json:"-"`
	EffectiveAt         time.Time      `json:"-"`
	Enabled             bool           `json:"enabled"`
}

type RouteConfigWrite struct {
	RouteModelID               int64          `json:"route_model_id"`
	ExpectedVersion            string         `json:"expected_version"`
	ConfigVersion              string         `json:"config_version"`
	CandidateParameterMappings map[string]any `json:"candidate_parameter_mappings"`
	MinimumTaskPoints          string         `json:"minimum_task_points"`
	RoundingStepPoints         int            `json:"rounding_step_points"`
	TaskTypes                  []string       `json:"task_types"`
	VisibleOptions             map[string]any `json:"visible_options"`
	Defaults                   map[string]any `json:"defaults"`
	MaxOutputCount             int            `json:"max_output_count"`
	Enabled                    bool           `json:"enabled"`
}

func (s *Service) SaveCapability(ctx context.Context, input CapabilityWrite) (CapabilitySummary, error) {
	if input.Enabled && input.ValidationStatus != "verified" {
		return CapabilitySummary{}, errs.New(409, errs.CodeConflict, "only verified video capabilities can be enabled")
	}
	snapshot, err := s.Snapshot(ctx)
	if err != nil {
		return CapabilitySummary{}, err
	}
	current := ""
	for _, item := range snapshot.Capabilities {
		if item.AccountModelID == input.AccountModelID {
			current = item.Version
		}
	}
	if current != input.ExpectedVersion {
		return CapabilitySummary{}, errs.New(409, errs.CodeConflict, "video capability version conflict")
	}
	payload, err := json.Marshal(input.Capability)
	if err != nil {
		return CapabilitySummary{}, errs.BadRequest("invalid video capability")
	}
	var capability domainvideo.Capability
	if err := json.Unmarshal(payload, &capability); err != nil {
		return CapabilitySummary{}, errs.BadRequest("invalid video capability")
	}
	if capability.SchemaVersion != 1 || capability.ProviderNativeMaxN < 1 || capability.ProviderNativeMaxN > 10 || len(capability.TaskTypes) == 0 {
		return CapabilitySummary{}, errs.BadRequest("video capability is invalid")
	}
	for taskType, task := range capability.TaskTypes {
		durations := task.Durations.Values
		if len(durations) == 0 && task.Durations.Min > 0 && task.Durations.Max >= task.Durations.Min {
			step := task.Durations.Step
			if step <= 0 {
				step = 1
			}
			for value := task.Durations.Min; value <= task.Durations.Max; value += step {
				durations = append(durations, value)
			}
		}
		if len(durations) == 0 || len(task.Resolutions) == 0 || len(task.AspectRatios) == 0 || len(task.AudioModes) == 0 {
			return CapabilitySummary{}, errs.BadRequest("video capability task parameter sets must not be empty")
		}
		for _, duration := range durations {
			if duration <= 0 {
				return CapabilitySummary{}, errs.BadRequest("video capability duration must be positive")
			}
			for _, resolution := range task.Resolutions {
				for _, ratio := range task.AspectRatios {
					for _, audio := range task.AudioModes {
						request := domainvideo.Request{TaskType: taskType, OutputCount: 1, DurationSeconds: duration, Resolution: resolution, AspectRatio: ratio, AudioMode: audio, Inputs: capabilityValidationInputs(task)}
						if !capability.Match(request).Matches {
							return CapabilitySummary{}, errs.BadRequest("video capability contains an invalid parameter combination")
						}
					}
				}
			}
		}
	}
	return s.store.SaveCapability(ctx, input)
}

func capabilityValidationInputs(task domainvideo.TaskCapability) []domainvideo.Input {
	inputs := make([]domainvideo.Input, 0, len(task.Inputs))
	for role, config := range task.Inputs {
		if !config.Required {
			continue
		}
		input := domainvideo.Input{Role: role, SizeBytes: 1}
		if len(config.MediaTypes) > 0 {
			input.MediaType = config.MediaTypes[0]
		}
		if len(config.Formats) > 0 {
			input.Format = config.Formats[0]
		}
		inputs = append(inputs, input)
	}
	return inputs
}

func (s *Service) SaveRouteConfig(ctx context.Context, input RouteConfigWrite) (RouteConfigSummary, error) {
	if input.MaxOutputCount < 1 || input.MaxOutputCount > 4 {
		return RouteConfigSummary{}, errs.BadRequest("max_output_count must be between 1 and 4")
	}
	if _, err := parseNonNegativePointDecimal(normalizePointString(input.MinimumTaskPoints), "minimum_task_points"); err != nil {
		return RouteConfigSummary{}, err
	}
	if input.RoundingStepPoints == 0 {
		input.RoundingStepPoints = 1
	}
	if input.RoundingStepPoints != 1 && input.RoundingStepPoints != 5 && input.RoundingStepPoints != 10 {
		return RouteConfigSummary{}, errs.BadRequest("rounding_step_points must be 1, 5, or 10")
	}
	if input.Enabled {
		snapshot, err := s.Snapshot(ctx)
		if err != nil {
			return RouteConfigSummary{}, err
		}
		var route RouteConfigSummary
		for _, item := range snapshot.Routes {
			if item.RouteModelID == input.RouteModelID {
				route = item
			}
		}
		if route.CandidateCount == 0 {
			return RouteConfigSummary{}, errs.New(409, errs.CodeConflict, "video route has no enabled candidate")
		}
	}
	return s.store.SaveRouteConfig(ctx, input)
}

func mappedCandidateResolution(mappings map[string]any, accountModelID int64, fallback string) string {
	value, ok := mappings[strconv.FormatInt(accountModelID, 10)]
	if !ok {
		return fallback
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return fallback
	}
	var mapping struct {
		Resolutions map[string]string `json:"resolutions"`
	}
	if json.Unmarshal(payload, &mapping) != nil || mapping.Resolutions[fallback] == "" {
		return fallback
	}
	return mapping.Resolutions[fallback]
}
