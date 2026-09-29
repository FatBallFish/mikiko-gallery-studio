package video

import (
	"strings"
	"testing"
)

func TestNativePricingValidatesSeedanceRateCardAgainstCapability(t *testing.T) {
	capability := Capability{
		SchemaVersion:      1,
		ProviderNativeMaxN: 1,
		TaskTypes: map[TaskType]TaskCapability{
			TaskTypeTextToVideo: {
				Resolutions: []Resolution{Resolution720P},
			},
		},
	}
	card := RateCard{
		ProviderCode:  "seedance",
		ModelCode:     "doubao-seedance-2-0-260128",
		PricingSchema: PricingSchemaSeedanceTokenV1,
		RuleVersion:   SeedanceRuleVersion202608,
		Seedance: &SeedanceTokenRateCard{Resolutions: map[Resolution]SeedanceResolutionRate{
			Resolution720P: {WithoutInputVideoMillionTokensCNY: "46"},
		}},
	}

	if err := ValidateRateCard(card, capability); err != nil {
		t.Fatalf("valid rate card rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*RateCard, *Capability)
		want   string
	}{
		{
			name: "unknown schema",
			mutate: func(card *RateCard, _ *Capability) {
				card.PricingSchema = PricingSchema("future_schema")
			},
			want: "unsupported pricing schema",
		},
		{
			name: "unsupported resolution row",
			mutate: func(card *RateCard, _ *Capability) {
				card.Seedance.Resolutions[Resolution4K] = SeedanceResolutionRate{WithoutInputVideoMillionTokensCNY: "46"}
			},
			want: "resolution 4k",
		},
		{
			name: "zero active output rate",
			mutate: func(card *RateCard, _ *Capability) {
				card.Seedance.Resolutions[Resolution720P] = SeedanceResolutionRate{WithoutInputVideoMillionTokensCNY: "0"}
			},
			want: "must be positive",
		},
		{
			name: "missing supported resolution rate",
			mutate: func(_ *RateCard, capability *Capability) {
				task := capability.TaskTypes[TaskTypeTextToVideo]
				task.Resolutions = append(task.Resolutions, Resolution1080P)
				capability.TaskTypes[TaskTypeTextToVideo] = task
			},
			want: "rate is missing for resolution 1080p",
		},
		{
			name: "video input rate required",
			mutate: func(_ *RateCard, capability *Capability) {
				task := capability.TaskTypes[TaskTypeTextToVideo]
				task.Inputs = map[InputRole]InputCapability{
					InputRoleFirstFrame: {MaxCount: 1, MediaTypes: []string{"video"}},
				}
				capability.TaskTypes[TaskTypeTextToVideo] = task
			},
			want: "with_input_video_million_tokens_cny",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cloneCard := card
			cloneCard.Seedance = &SeedanceTokenRateCard{Resolutions: map[Resolution]SeedanceResolutionRate{
				Resolution720P: card.Seedance.Resolutions[Resolution720P],
			}}
			cloneCapability := capability
			cloneCapability.TaskTypes = map[TaskType]TaskCapability{
				TaskTypeTextToVideo: capability.TaskTypes[TaskTypeTextToVideo],
			}
			test.mutate(&cloneCard, &cloneCapability)
			err := ValidateRateCard(cloneCard, cloneCapability)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestSeedanceNativePricingUsesOfficialTokenFormula(t *testing.T) {
	card := RateCard{
		ProviderCode:  "seedance",
		ModelCode:     "doubao-seedance-2-0-260128",
		PricingSchema: PricingSchemaSeedanceTokenV1,
		RuleVersion:   SeedanceRuleVersion202608,
		Seedance: &SeedanceTokenRateCard{Resolutions: map[Resolution]SeedanceResolutionRate{
			Resolution720P: {WithoutInputVideoMillionTokensCNY: "46"},
		}},
	}
	quote, err := QuoteNativePricing(NativePricingRequest{Video: Request{
		TaskType:        TaskTypeTextToVideo,
		DurationSeconds: 5,
		Resolution:      Resolution720P,
		AspectRatio:     AspectRatio16x9,
		OutputCount:     1,
	}}, card)
	if err != nil {
		t.Fatal(err)
	}
	if quote.CNY != "4.96800" {
		t.Fatalf("expected 4.96800 CNY, got %s", quote.CNY)
	}
	if quote.Calculation["estimated_tokens"] != "108000" {
		t.Fatalf("expected 108000 tokens, got %#v", quote.Calculation["estimated_tokens"])
	}
}

func TestSeedanceNativePricingAppliesInputVideoMinimumAndRejectsUnknownRule(t *testing.T) {
	card := RateCard{
		ProviderCode:  "seedance",
		ModelCode:     "doubao-seedance-2-0-260128",
		PricingSchema: PricingSchemaSeedanceTokenV1,
		RuleVersion:   SeedanceRuleVersion202608,
		Seedance: &SeedanceTokenRateCard{Resolutions: map[Resolution]SeedanceResolutionRate{
			Resolution720P: {
				WithoutInputVideoMillionTokensCNY: "46",
				WithInputVideoMillionTokensCNY:    "50",
			},
		}},
	}
	request := NativePricingRequest{Video: Request{
		TaskType:        TaskTypeImageToVideo,
		DurationSeconds: 5,
		Resolution:      Resolution720P,
		AspectRatio:     AspectRatio16x9,
		OutputCount:     1,
	}, InputVideoSeconds: "1"}

	quote, err := QuoteNativePricing(request, card)
	if err != nil {
		t.Fatal(err)
	}
	if quote.Calculation["minimum_tokens_applied"] != true {
		t.Fatalf("expected minimum token floor, got %#v", quote.Calculation)
	}
	if quote.Calculation["billable_tokens"] != "151200" {
		t.Fatalf("expected versioned minimum of 151200 tokens, got %#v", quote.Calculation["billable_tokens"])
	}

	card.RuleVersion = "seedance-rules-future"
	if _, err := QuoteNativePricing(request, card); err == nil || !strings.Contains(err.Error(), "unsupported seedance rule version") {
		t.Fatalf("expected unsupported rule version, got %v", err)
	}
}

func TestMiniMaxH3NativePricingUsesSecondsAndMaterialRules(t *testing.T) {
	card := RateCard{
		ProviderCode:  "minimax",
		ModelCode:     "MiniMax-H3",
		PricingSchema: PricingSchemaMiniMaxH3SecondV1,
		RuleVersion:   MiniMaxH3RuleVersion202608,
		MiniMaxH3: &MiniMaxH3SecondRateCard{
			Resolutions: map[Resolution]MiniMaxResolutionRate{
				Resolution768P: {OutputSecondCNY: "0.50", InputVideoSecondCNY: "0.50"},
			},
			FreeImageCount: 5,
			ExtraImageCNY:  "0.20",
			InputAudioFree: true,
		},
	}
	tests := []struct {
		name              string
		images            int
		inputVideoSeconds string
		want              string
	}{
		{name: "five images are free", images: 5, want: "2.50000"},
		{name: "two excess images", images: 7, want: "2.90000"},
		{name: "input video is billed by second", images: 5, inputVideoSeconds: "3.5", want: "4.25000"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			quote, err := QuoteNativePricing(NativePricingRequest{Video: Request{
				TaskType:        TaskTypeTextToVideo,
				DurationSeconds: 5,
				Resolution:      Resolution768P,
				AspectRatio:     AspectRatio16x9,
				OutputCount:     1,
			}, ReferenceImageCount: test.images, InputVideoSeconds: test.inputVideoSeconds, HasInputAudio: true}, card)
			if err != nil {
				t.Fatal(err)
			}
			if quote.CNY != test.want {
				t.Fatalf("expected %s CNY, got %s", test.want, quote.CNY)
			}
		})
	}
}

func TestMiniMaxH3MaxNativePricingSupports480pAnd768p(t *testing.T) {
	capability := Capability{
		SchemaVersion:      1,
		ProviderNativeMaxN: 1,
		TaskTypes: map[TaskType]TaskCapability{
			TaskTypeTextToVideo: {
				Durations:    IntValues{Min: 5, Max: 15},
				Resolutions:  []Resolution{Resolution480P, Resolution768P},
				AspectRatios: []AspectRatio{AspectRatio16x9},
				AudioModes:   []AudioMode{AudioModeGenerated},
			},
		},
	}
	card := RateCard{
		ProviderCode:  "minimax",
		ModelCode:     "MiniMax-H3-Max",
		PricingSchema: PricingSchemaMiniMaxH3SecondV1,
		RuleVersion:   MiniMaxH3RuleVersion202608,
		MiniMaxH3: &MiniMaxH3SecondRateCard{
			Resolutions: map[Resolution]MiniMaxResolutionRate{
				Resolution480P: {OutputSecondCNY: "0.30", InputVideoSecondCNY: "0.30"},
				Resolution768P: {OutputSecondCNY: "0.50", InputVideoSecondCNY: "0.50"},
			},
			FreeImageCount: 5,
			ExtraImageCNY:  "0.20",
			InputAudioFree: true,
		},
	}
	if err := ValidateRateCard(card, capability); err != nil {
		t.Fatalf("H3-Max rate card should validate: %v", err)
	}
	quote, err := QuoteNativePricing(NativePricingRequest{Video: Request{
		TaskType: TaskTypeTextToVideo, DurationSeconds: 6, Resolution: Resolution480P, AspectRatio: AspectRatio16x9, OutputCount: 1,
	}}, card)
	if err != nil {
		t.Fatal(err)
	}
	if quote.CNY != "1.80000" {
		t.Fatalf("expected 1.80000 CNY for 6s@480p, got %s", quote.CNY)
	}
	invalid := card
	invalid.MiniMaxH3 = &MiniMaxH3SecondRateCard{
		Resolutions: map[Resolution]MiniMaxResolutionRate{
			Resolution480P: {OutputSecondCNY: "0.30", InputVideoSecondCNY: "0.30"},
			Resolution768P: {OutputSecondCNY: "0.50", InputVideoSecondCNY: "0.50"},
			Resolution2K:   {OutputSecondCNY: "0.80", InputVideoSecondCNY: "0.80"},
		},
		FreeImageCount: 5, ExtraImageCNY: "0.20", InputAudioFree: true,
	}
	widerCapability := capability
	widerCapability.TaskTypes = map[TaskType]TaskCapability{
		TaskTypeTextToVideo: {
			Durations:    IntValues{Min: 5, Max: 15},
			Resolutions:  []Resolution{Resolution480P, Resolution768P, Resolution2K},
			AspectRatios: []AspectRatio{AspectRatio16x9},
			AudioModes:   []AudioMode{AudioModeGenerated},
		},
	}
	if err := ValidateRateCard(invalid, widerCapability); err == nil || !strings.Contains(err.Error(), "unsupported by MiniMax-H3-Max pricing") {
		t.Fatalf("2K should be rejected for H3-Max, got %v", err)
	}
}

func TestSeedance25Supports1080pPricingPreset(t *testing.T) {
	const model25 = "doubao-seedance-2-5-260628"
	for _, resolution := range []Resolution{Resolution480P, Resolution720P, Resolution1080P} {
		if !seedanceModelSupportsResolution(model25, resolution) {
			t.Fatalf("seedance 2.5 should support %s", resolution)
		}
		width, height, fps, err := seedanceOutputPreset(model25, Resolution1080P, AspectRatio16x9)
		if err != nil {
			t.Fatalf("1080p preset: %v", err)
		}
		if width != 1920 || height != 1080 || fps != 24 {
			t.Fatalf("1080p preset = %dx%d@%d, want 1920x1080@24", width, height, fps)
		}
	}

	capability := Capability{
		SchemaVersion:      1,
		ProviderNativeMaxN: 1,
		TaskTypes: map[TaskType]TaskCapability{
			TaskTypeTextToVideo: {
				Durations:    IntValues{Values: []int{5}},
				Resolutions:  []Resolution{Resolution480P, Resolution720P, Resolution1080P},
				AspectRatios: []AspectRatio{AspectRatio16x9},
				AudioModes:   []AudioMode{AudioModeSilent, AudioModeGenerated},
			},
		},
	}
	card := RateCard{
		ProviderCode: "seedance", ModelCode: model25,
		PricingSchema: PricingSchemaSeedanceTokenV1, RuleVersion: SeedanceRuleVersion202608,
		Seedance: &SeedanceTokenRateCard{Resolutions: map[Resolution]SeedanceResolutionRate{
			Resolution480P:  {WithoutInputVideoMillionTokensCNY: "60"},
			Resolution720P:  {WithoutInputVideoMillionTokensCNY: "90"},
			Resolution1080P: {WithoutInputVideoMillionTokensCNY: "150"},
		}},
	}
	if err := ValidateRateCard(card, capability); err != nil {
		t.Fatalf("seedance 2.5 1080p rate card rejected: %v", err)
	}
	quote, err := QuoteNativePricing(NativePricingRequest{Video: Request{
		TaskType: TaskTypeTextToVideo, DurationSeconds: 5, Resolution: Resolution1080P,
		AspectRatio: AspectRatio16x9, AudioMode: AudioModeSilent, OutputCount: 1,
	}}, card)
	if err != nil {
		t.Fatalf("quote seedance 2.5 1080p: %v", err)
	}
	if !strings.Contains(quote.CNY, ".") && quote.CNY == "" {
		t.Fatalf("empty 1080p quote")
	}
}
