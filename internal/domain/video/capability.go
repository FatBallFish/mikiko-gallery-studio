package video

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

type IntValues struct {
	Values []int `json:"values,omitempty"`
	Min    int   `json:"min,omitempty"`
	Max    int   `json:"max,omitempty"`
	Step   int   `json:"step,omitempty"`
}

func DiscreteIntValues(values ...int) IntValues {
	return IntValues{Values: append([]int(nil), values...)}
}

func (values IntValues) Contains(value int) bool {
	for _, candidate := range values.Values {
		if candidate == value {
			return true
		}
	}
	if values.Min == 0 && values.Max == 0 {
		return false
	}
	if value < values.Min || value > values.Max {
		return false
	}
	if values.Step <= 1 {
		return true
	}
	return (value-values.Min)%values.Step == 0
}

type InputCapability struct {
	Required   bool     `json:"required"`
	MaxCount   int      `json:"max_count"`
	MaxBytes   int64    `json:"max_bytes"`
	MediaTypes []string `json:"media_types"`
	Formats    []string `json:"formats"`
}

type TaskCapability struct {
	Durations    IntValues                     `json:"durations"`
	Resolutions  []Resolution                  `json:"resolutions"`
	AspectRatios []AspectRatio                 `json:"aspect_ratios"`
	AudioModes   []AudioMode                   `json:"audio_modes"`
	Inputs       map[InputRole]InputCapability `json:"inputs,omitempty"`
}

type Capability struct {
	SchemaVersion      int                         `json:"schema_version"`
	ProviderNativeMaxN int                         `json:"provider_native_max_n"`
	PromptMaxRunes     int                         `json:"prompt_max_runes"`
	TaskTypes          map[TaskType]TaskCapability `json:"task_types"`
}

type Candidate struct {
	ID         int64
	Enabled    bool
	Capability Capability
}

type Combination struct {
	CandidateID     int64
	TaskType        TaskType
	DurationSeconds int
	Resolution      Resolution
	AspectRatio     AspectRatio
	AudioMode       AudioMode
}

type VisibleCapability struct {
	TaskType     TaskType
	Combinations []Combination
}

func (capability Capability) Match(request Request) MatchResult {
	if capability.SchemaVersion != 1 {
		return mismatch("capability_version", "unsupported", "unsupported video capability version")
	}
	if capability.ProviderNativeMaxN < 1 || capability.ProviderNativeMaxN > 10 {
		return mismatch("provider_native_max_n", "invalid", "provider native output count must be between 1 and 10")
	}
	if request.OutputCount < 1 || request.OutputCount > 4 {
		return mismatch("output_count", "out_of_range", "output count must be between 1 and 4")
	}
	task, ok := capability.TaskTypes[request.TaskType]
	if !ok {
		return mismatch("task_type", "unsupported", "video task type is not supported")
	}
	if capability.PromptMaxRunes > 0 && utf8.RuneCountInString(request.Prompt) > capability.PromptMaxRunes {
		return mismatch("prompt", "too_long", fmt.Sprintf("prompt must not exceed %d characters", capability.PromptMaxRunes))
	}
	if !task.Durations.Contains(request.DurationSeconds) {
		return mismatch("duration_seconds", "unsupported", "video duration is not supported")
	}
	if !containsResolution(task.Resolutions, request.Resolution) {
		return mismatch("resolution", "unsupported", "video resolution is not supported")
	}
	if !containsAspectRatio(task.AspectRatios, request.AspectRatio) {
		return mismatch("aspect_ratio", "unsupported", "video aspect ratio is not supported")
	}
	if !containsAudioMode(task.AudioModes, request.AudioMode) {
		return mismatch("generate_audio", "unsupported", "generated audio is not supported for this combination")
	}
	for role, inputCapability := range task.Inputs {
		inputs := inputsForRole(request.Inputs, role)
		if inputCapability.Required && len(inputs) == 0 {
			return mismatch("inputs."+string(role), "required", "required video input is missing")
		}
		if inputCapability.MaxCount >= 0 && len(inputs) > inputCapability.MaxCount {
			return mismatch("inputs."+string(role), "too_many", "too many video inputs")
		}
		for _, input := range inputs {
			if !containsFold(inputCapability.MediaTypes, input.MediaType) {
				return mismatch("inputs."+string(role)+".media_type", "unsupported", "input media type is not supported")
			}
			if !containsFold(inputCapability.Formats, input.Format) {
				return mismatch("inputs."+string(role)+".format", "unsupported", "input format is not supported")
			}
			if inputCapability.MaxBytes > 0 && input.SizeBytes > inputCapability.MaxBytes {
				return mismatch("inputs."+string(role)+".size_bytes", "too_large", "input exceeds the provider size limit")
			}
		}
	}
	for _, input := range request.Inputs {
		if _, ok := task.Inputs[input.Role]; !ok {
			return mismatch("inputs."+string(input.Role), "unsupported", "input role is not supported")
		}
	}
	return MatchResult{Matches: true}
}

func ProviderBatches(outputCount, providerNativeMaxN int) []int {
	if outputCount <= 0 || providerNativeMaxN <= 0 {
		return nil
	}
	if providerNativeMaxN > 10 {
		providerNativeMaxN = 10
	}
	batches := make([]int, 0, (outputCount+providerNativeMaxN-1)/providerNativeMaxN)
	for remaining := outputCount; remaining > 0; {
		count := providerNativeMaxN
		if remaining < count {
			count = remaining
		}
		batches = append(batches, count)
		remaining -= count
	}
	return batches
}

func MatchingCandidates(candidates []Candidate, request Request) []Candidate {
	matched := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Enabled && candidate.Capability.Match(request).Matches {
			matched = append(matched, candidate)
		}
	}
	return matched
}

func BuildVisibleCapability(candidates []Candidate, taskType TaskType) VisibleCapability {
	visible := VisibleCapability{TaskType: taskType}
	seen := make(map[string]struct{})
	for _, candidate := range candidates {
		if !candidate.Enabled {
			continue
		}
		task, ok := candidate.Capability.TaskTypes[taskType]
		if !ok {
			continue
		}
		for _, duration := range enumerateDurations(task.Durations) {
			for _, resolution := range task.Resolutions {
				for _, ratio := range task.AspectRatios {
					for _, audio := range task.AudioModes {
						combination := Combination{CandidateID: candidate.ID, TaskType: taskType, DurationSeconds: duration, Resolution: resolution, AspectRatio: ratio, AudioMode: audio}
						key := fmt.Sprintf("%d/%s/%d/%s/%s/%s", candidate.ID, taskType, duration, resolution, ratio, audio)
						if _, ok := seen[key]; ok {
							continue
						}
						seen[key] = struct{}{}
						visible.Combinations = append(visible.Combinations, combination)
					}
				}
			}
		}
	}
	sort.Slice(visible.Combinations, func(i, j int) bool {
		left, right := visible.Combinations[i], visible.Combinations[j]
		if left.DurationSeconds != right.DurationSeconds {
			return left.DurationSeconds < right.DurationSeconds
		}
		if left.Resolution != right.Resolution {
			return left.Resolution < right.Resolution
		}
		if left.AspectRatio != right.AspectRatio {
			return left.AspectRatio < right.AspectRatio
		}
		return left.AudioMode < right.AudioMode
	})
	return visible
}

func mismatch(field, code, message string) MatchResult {
	return MatchResult{FieldErrors: []FieldError{{Field: field, Code: code, Message: message}}}
}

func inputsForRole(inputs []Input, role InputRole) []Input {
	matched := make([]Input, 0, 1)
	for _, input := range inputs {
		if input.Role == role {
			matched = append(matched, input)
		}
	}
	return matched
}

func enumerateDurations(values IntValues) []int {
	if len(values.Values) > 0 {
		return append([]int(nil), values.Values...)
	}
	if values.Min <= 0 || values.Max < values.Min {
		return nil
	}
	step := values.Step
	if step <= 0 {
		step = 1
	}
	result := make([]int, 0, (values.Max-values.Min)/step+1)
	for value := values.Min; value <= values.Max; value += step {
		result = append(result, value)
	}
	return result
}

func containsResolution(values []Resolution, target Resolution) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsAspectRatio(values []AspectRatio, target AspectRatio) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsAudioMode(values []AudioMode, target AudioMode) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}
