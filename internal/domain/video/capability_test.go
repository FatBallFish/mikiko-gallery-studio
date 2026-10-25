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

	for _, format := range []string{"image/png", "image/png; charset=binary", "png"} {
		request := valid
		request.Inputs = append([]Input(nil), valid.Inputs...)
		request.Inputs[0].Format = format
		if result := capability.Match(request); !result.Matches {
			t.Fatalf("format %q should match short capability format: %#v", format, result.FieldErrors)
		}
	}
	for _, format := range []string{"image/jpeg", "jpeg"} {
		capability.TaskTypes[TaskTypeImageToVideo].Inputs[InputRoleFirstFrame] = InputCapability{Required: true, MaxCount: 1, MediaTypes: []string{"image"}, Formats: []string{"jpg"}}
		request := valid
		request.Inputs = append([]Input(nil), valid.Inputs...)
		request.Inputs[0].Format = format
		if result := capability.Match(request); !result.Matches {
			t.Fatalf("format %q should match jpg capability format: %#v", format, result.FieldErrors)
		}
	}
	capability.TaskTypes[TaskTypeImageToVideo].Inputs[InputRoleFirstFrame] = InputCapability{Required: true, MaxCount: 1, MaxBytes: 30 << 20, MediaTypes: []string{"image"}, Formats: []string{"png", "webp"}}

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

func TestCapabilityInputFormatAliasesPreserveMediaBoundaries(t *testing.T) {
	cases := []struct {
		name, mediaType, configured, supplied string
		matches                               bool
	}{
		{"png MIME", "image", "png", "image/png", true},
		{"png short", "image", "image/png", "png", true},
		{"jpeg MIME", "image", "jpg", "image/jpeg", true},
		{"jpeg short", "image", "image/jpeg", "jpeg", true},
		{"jpg short", "image", "image/jpeg", "jpg", true},
		{"webp MIME", "image", "webp", "image/webp", true},
		{"webp short", "image", "image/webp", "webp", true},
		{"case whitespace parameters", "image", " PNG ", " IMAGE/PNG; charset=binary ", true},
		{"unsupported MIME", "image", "png", "image/heic", false},
		{"unsupported short", "image", "image/png", "heic", false},
		{"wrong MIME family", "image", "png", "video/png", false},
		{"wrong configured family", "image", "video/png", "image/png", false},
		{"same wrong MIME family", "image", "video/png", "video/png", false},
		{"wrong media for known short", "video", "png", "png", false},
		{"empty format", "image", "", "", false},
		{"mp4 MIME", "video", "mp4", "video/mp4", true},
		{"mp4 short", "video", "video/mp4", "mp4", true},
		{"mov MIME", "video", "mov", "video/quicktime", true},
		{"mov short", "video", "video/quicktime", "mov", true},
		{"mp3 MIME", "audio", "mp3", "audio/mpeg", true},
		{"mp3 short", "audio", "audio/mpeg", "mp3", true},
		{"m4a MIME", "audio", "m4a", "audio/mp4", true},
		{"m4a alias", "audio", "audio/x-m4a", "m4a", true},
		{"wav MIME", "audio", "wav", "audio/x-wav", true},
		{"wav short", "audio", "audio/wav", "wav", true},
		{"audio mp4 is not video mp4", "audio", "mp4", "audio/mp4", false},
		{"video mp4 is not audio mp4", "video", "audio/mp4", "video/mp4", false},
		{"unknown MIME exact", "video", "video/custom", "video/custom", true},
		{"unknown subtype is not alias", "video", "custom", "video/custom", false},
	}
	for _, role := range []InputRole{InputRoleFirstFrame, InputRoleReferenceImage} {
		for _, tc := range cases {
			t.Run(string(role)+"/"+tc.name, func(t *testing.T) {
				taskType := TaskTypeImageToVideo
				if role == InputRoleReferenceImage {
					taskType = TaskTypeReferenceToVideo
				}
				capability := Capability{SchemaVersion: 1, ProviderNativeMaxN: 1, TaskTypes: map[TaskType]TaskCapability{
					taskType: {
						Durations: DiscreteIntValues(5), Resolutions: []Resolution{Resolution720P},
						AspectRatios: []AspectRatio{AspectRatioAdaptive}, AudioModes: []AudioMode{AudioModeSilent},
						Inputs: map[InputRole]InputCapability{role: {Required: true, MaxCount: 1, MediaTypes: []string{tc.mediaType}, Formats: []string{tc.configured}}},
					},
				}}
				request := Request{TaskType: taskType, DurationSeconds: 5, Resolution: Resolution720P,
					AspectRatio: AspectRatioAdaptive, AudioMode: AudioModeSilent, OutputCount: 1,
					Inputs: []Input{{Role: role, MediaType: tc.mediaType, Format: tc.supplied}}}
				result := capability.Match(request)
				if result.Matches != tc.matches {
					t.Fatalf("Match(%q, %q) = %#v, want matches=%v", tc.configured, tc.supplied, result, tc.matches)
				}
				if !tc.matches && (len(result.FieldErrors) != 1 || result.FieldErrors[0].Field != "inputs."+string(role)+".format" || result.FieldErrors[0].Code != "unsupported") {
					t.Fatalf("format mismatch must retain field error: %#v", result)
				}
				request.Inputs[0].MediaType = "application"
				if result := capability.Match(request); result.Matches || result.FieldErrors[0].Field != "inputs."+string(role)+".media_type" {
					t.Fatalf("media type mismatch = %#v", result)
				}
			})
		}
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
