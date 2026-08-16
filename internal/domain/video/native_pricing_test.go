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
