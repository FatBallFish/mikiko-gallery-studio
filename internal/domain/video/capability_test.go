package video

import (
	"testing"
)

func TestCapabilityMatchesCompleteRequestCombination(t *testing.T) {
	capability := Capability{
		SchemaVersion:      1,
		ProviderNativeMaxN: 1,
		PromptMaxRunes:     2000,
		TaskTypes: map[TaskType]TaskCapability{
			TaskTypeImageToVideo: {
				Durations:    DiscreteIntValues(5, 10),
				Resolutions:  []Resolution{Resolution720P},
				AspectRatios: []AspectRatio{AspectRatioAdaptive},
				AudioModes:   []AudioMode{AudioModeSilent},
				Inputs: map[InputRole]InputCapability{
					InputRoleFirstFrame: {
						Required:   true,
						MaxCount:   1,
						MaxBytes:   30 << 20,
						MediaTypes: []string{"image"},
						Formats:    []string{"png", "webp"},
					},
				},
			},
		},
	}

	valid := Request{
		TaskType:        TaskTypeImageToVideo,
		Prompt:          "camera pushes forward",
		DurationSeconds: 5,
		Resolution:      Resolution720P,
		AspectRatio:     AspectRatioAdaptive,
		AudioMode:       AudioModeSilent,
		OutputCount:     4,
		Inputs: []Input{{
			Role:      InputRoleFirstFrame,
			MediaType: "image",
			Format:    "png",
			SizeBytes: 4 << 20,
		}},
	}
	if result := capability.Match(valid); !result.Matches {
		t.Fatalf("valid request did not match: %#v", result.FieldErrors)
	}

	invalidAudio := valid
	invalidAudio.AudioMode = AudioModeGenerated
	if result := capability.Match(invalidAudio); result.Matches || result.FieldErrors[0].Field != "generate_audio" {
		t.Fatalf("audio mismatch result = %#v", result)
	}

	invalidFormat := valid
	invalidFormat.Inputs = append([]Input(nil), valid.Inputs...)
	invalidFormat.Inputs[0].Format = "heic"
	if result := capability.Match(invalidFormat); result.Matches || result.FieldErrors[0].Field != "inputs.first_frame.format" {
		t.Fatalf("format mismatch result = %#v", result)
	}
}

func TestCapabilityProviderNativeNDoesNotLimitPlatformOutputItems(t *testing.T) {
	capability := Capability{
		SchemaVersion:      1,
		ProviderNativeMaxN: 1,
		TaskTypes: map[TaskType]TaskCapability{
			TaskTypeTextToVideo: {
				Durations:    DiscreteIntValues(5),
				Resolutions:  []Resolution{Resolution720P},
				AspectRatios: []AspectRatio{AspectRatio16x9},
				AudioModes:   []AudioMode{AudioModeSilent},
			},
		},
	}

	request := Request{
		TaskType:        TaskTypeTextToVideo,
		Prompt:          "city at sunrise",
		DurationSeconds: 5,
		Resolution:      Resolution720P,
		AspectRatio:     AspectRatio16x9,
		AudioMode:       AudioModeSilent,
		OutputCount:     4,
	}
	if result := capability.Match(request); !result.Matches {
		t.Fatalf("platform output count should be split into items, got %#v", result.FieldErrors)
	}
	batches := ProviderBatches(request.OutputCount, capability.ProviderNativeMaxN)
	if len(batches) != 4 {
		t.Fatalf("ProviderBatches() = %#v, want four single-result requests", batches)
	}
	for _, count := range batches {
		if count != 1 {
			t.Fatalf("batch count = %d, want 1", count)
		}
	}
}

func TestVisibleCapabilityUnionKeepsOnlyRoutableCombinations(t *testing.T) {
	candidates := []Candidate{
		{
			ID:      1,
			Enabled: true,
			Capability: Capability{SchemaVersion: 1, ProviderNativeMaxN: 1, TaskTypes: map[TaskType]TaskCapability{
				TaskTypeTextToVideo: {
					Durations:    DiscreteIntValues(5),
					Resolutions:  []Resolution{Resolution720P},
					AspectRatios: []AspectRatio{AspectRatio16x9},
					AudioModes:   []AudioMode{AudioModeSilent},
				},
			}},
		},
		{
			ID:      2,
			Enabled: true,
			Capability: Capability{SchemaVersion: 1, ProviderNativeMaxN: 1, TaskTypes: map[TaskType]TaskCapability{
				TaskTypeTextToVideo: {
					Durations:    DiscreteIntValues(10),
					Resolutions:  []Resolution{Resolution1080P},
					AspectRatios: []AspectRatio{AspectRatio9x16},
					AudioModes:   []AudioMode{AudioModeGenerated},
				},
			}},
		},
	}

	visible := BuildVisibleCapability(candidates, TaskTypeTextToVideo)
	if len(visible.Combinations) != 2 {
		t.Fatalf("visible combinations = %#v, want exactly two complete candidate combinations", visible.Combinations)
	}
	request := Request{
		TaskType:        TaskTypeTextToVideo,
		Prompt:          "portrait clip",
		DurationSeconds: 5,
		Resolution:      Resolution1080P,
		AspectRatio:     AspectRatio16x9,
		AudioMode:       AudioModeSilent,
		OutputCount:     1,
	}
	if matched := MatchingCandidates(candidates, request); len(matched) != 0 {
		t.Fatalf("independently visible values must not create a synthetic routable combination: %#v", matched)
	}
}
