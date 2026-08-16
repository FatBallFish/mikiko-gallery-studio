package video

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

type PricingSchema string

const (
	PricingSchemaSeedanceTokenV1   PricingSchema = "seedance_token_v1"
	PricingSchemaMiniMaxH3SecondV1 PricingSchema = "minimax_h3_second_v1"

	SeedanceRuleVersion202608  = "seedance-rules-2026-08"
	MiniMaxH3RuleVersion202608 = "minimax-h3-rules-2026-08"
)

type SeedanceTokenRateCard struct {
	Resolutions map[Resolution]SeedanceResolutionRate `json:"resolutions"`
}

type SeedanceResolutionRate struct {
	WithoutInputVideoMillionTokensCNY string `json:"without_input_video_million_tokens_cny"`
	WithInputVideoMillionTokensCNY    string `json:"with_input_video_million_tokens_cny,omitempty"`
}

type MiniMaxH3SecondRateCard struct {
	Resolutions    map[Resolution]MiniMaxResolutionRate `json:"resolutions"`
	FreeImageCount int                                  `json:"free_image_count"`
	ExtraImageCNY  string                               `json:"extra_image_cny"`
	InputAudioFree bool                                 `json:"input_audio_free"`
}

type MiniMaxResolutionRate struct {
	OutputSecondCNY     string `json:"output_second_cny"`
	InputVideoSecondCNY string `json:"input_video_second_cny"`
}

type RateCard struct {
	ProviderCode  string                   `json:"provider_code"`
	ModelCode     string                   `json:"model_code"`
	PricingSchema PricingSchema            `json:"pricing_schema"`
	RuleVersion   string                   `json:"rule_version"`
	Seedance      *SeedanceTokenRateCard   `json:"seedance,omitempty"`
	MiniMaxH3     *MiniMaxH3SecondRateCard `json:"minimax_h3,omitempty"`
}

type NativePricingRequest struct {
	Video               Request `json:"video"`
	InputVideoSeconds   string  `json:"input_video_seconds,omitempty"`
	ReferenceImageCount int     `json:"reference_image_count,omitempty"`
	HasInputAudio       bool    `json:"has_input_audio,omitempty"`
}

type CandidateQuote struct {
	CNY         string         `json:"cny"`
	Calculation map[string]any `json:"calculation"`
}

func ValidateRateCard(card RateCard, capability Capability) error {
	switch card.PricingSchema {
	case PricingSchemaSeedanceTokenV1:
		return validateSeedanceRateCard(card, capability)
	case PricingSchemaMiniMaxH3SecondV1:
		return validateMiniMaxRateCard(card, capability)
	default:
		return fmt.Errorf("unsupported pricing schema %q", card.PricingSchema)
	}
}

func QuoteNativePricing(request NativePricingRequest, card RateCard) (CandidateQuote, error) {
	if request.Video.DurationSeconds <= 0 {
		return CandidateQuote{}, fmt.Errorf("duration_seconds must be positive")
	}
	if request.ReferenceImageCount < 0 {
		return CandidateQuote{}, fmt.Errorf("reference_image_count must not be negative")
	}
	inputVideoSeconds, err := parseOptionalPositiveDecimal(request.InputVideoSeconds, "input_video_seconds")
	if err != nil {
		return CandidateQuote{}, err
	}
	switch card.PricingSchema {
	case PricingSchemaSeedanceTokenV1:
		return quoteSeedance(request, inputVideoSeconds, card)
	case PricingSchemaMiniMaxH3SecondV1:
		return quoteMiniMaxH3(request, inputVideoSeconds, card)
	default:
		return CandidateQuote{}, fmt.Errorf("unsupported pricing schema %q", card.PricingSchema)
	}
}

func validateSeedanceRateCard(card RateCard, capability Capability) error {
	if !strings.EqualFold(strings.TrimSpace(card.ProviderCode), "seedance") {
		return fmt.Errorf("seedance pricing schema requires seedance provider")
	}
	if card.RuleVersion != SeedanceRuleVersion202608 {
		return fmt.Errorf("unsupported seedance rule version %q", card.RuleVersion)
	}
	if card.Seedance == nil || len(card.Seedance.Resolutions) == 0 {
		return fmt.Errorf("seedance resolutions are required")
	}
	supported := capabilityResolutions(capability)
	for resolution := range supported {
		if _, ok := card.Seedance.Resolutions[resolution]; !ok {
			return fmt.Errorf("seedance rate is missing for resolution %s", resolution)
		}
	}
	requiresInputVideoRate := capabilitySupportsMediaType(capability, "video")
	for resolution, rate := range card.Seedance.Resolutions {
		if _, ok := supported[resolution]; !ok {
			return fmt.Errorf("resolution %s is not supported by the model capability", resolution)
		}
		if !seedanceModelSupportsResolution(card.ModelCode, resolution) {
			return fmt.Errorf("resolution %s has no audited seedance pricing preset for model %s", resolution, card.ModelCode)
		}
		if _, err := parsePositiveDecimal(rate.WithoutInputVideoMillionTokensCNY, "without_input_video_million_tokens_cny"); err != nil {
			return err
		}
		if requiresInputVideoRate {
			if _, err := parsePositiveDecimal(rate.WithInputVideoMillionTokensCNY, "with_input_video_million_tokens_cny"); err != nil {
				return err
			}
		} else if strings.TrimSpace(rate.WithInputVideoMillionTokensCNY) != "" {
			if _, err := parsePositiveDecimal(rate.WithInputVideoMillionTokensCNY, "with_input_video_million_tokens_cny"); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateMiniMaxRateCard(card RateCard, capability Capability) error {
	if !strings.EqualFold(strings.TrimSpace(card.ProviderCode), "minimax") {
		return fmt.Errorf("minimax pricing schema requires minimax provider")
	}
	if card.RuleVersion != MiniMaxH3RuleVersion202608 {
		return fmt.Errorf("unsupported minimax h3 rule version %q", card.RuleVersion)
	}
	if !strings.EqualFold(strings.TrimSpace(card.ModelCode), "MiniMax-H3") {
		return fmt.Errorf("minimax h3 pricing schema does not support model %s", card.ModelCode)
	}
	if card.MiniMaxH3 == nil || len(card.MiniMaxH3.Resolutions) == 0 {
		return fmt.Errorf("minimax h3 resolutions are required")
	}
	if card.MiniMaxH3.FreeImageCount < 0 {
		return fmt.Errorf("free_image_count must not be negative")
	}
	if card.MiniMaxH3.InputAudioFree != true {
		return fmt.Errorf("input_audio_free must be true for minimax h3")
	}
	if _, err := parseNonNegativeDecimal(card.MiniMaxH3.ExtraImageCNY, "extra_image_cny"); err != nil {
		return err
	}
	supported := capabilityResolutions(capability)
	for resolution := range supported {
		if _, ok := card.MiniMaxH3.Resolutions[resolution]; !ok {
			return fmt.Errorf("minimax h3 rate is missing for resolution %s", resolution)
		}
	}
	requiresInputVideoRate := capabilitySupportsMediaType(capability, "video")
	for resolution, rate := range card.MiniMaxH3.Resolutions {
		if _, ok := supported[resolution]; !ok {
			return fmt.Errorf("resolution %s is not supported by the model capability", resolution)
		}
		if resolution != Resolution768P && resolution != Resolution2K {
			return fmt.Errorf("resolution %s is unsupported by minimax h3 pricing", resolution)
		}
		if _, err := parsePositiveDecimal(rate.OutputSecondCNY, "output_second_cny"); err != nil {
			return err
		}
		if requiresInputVideoRate || strings.TrimSpace(rate.InputVideoSecondCNY) != "" {
			if _, err := parsePositiveDecimal(rate.InputVideoSecondCNY, "input_video_second_cny"); err != nil {
				return err
			}
		}
	}
	return nil
}

func quoteSeedance(request NativePricingRequest, inputVideoSeconds decimal.Decimal, card RateCard) (CandidateQuote, error) {
	if card.RuleVersion != SeedanceRuleVersion202608 {
		return CandidateQuote{}, fmt.Errorf("unsupported seedance rule version %q", card.RuleVersion)
	}
	if card.Seedance == nil {
		return CandidateQuote{}, fmt.Errorf("seedance rate config is required")
	}
	width, height, fps, err := seedanceOutputPreset(card.ModelCode, request.Video.Resolution, request.Video.AspectRatio)
	if err != nil {
		return CandidateQuote{}, err
	}
	rate, ok := card.Seedance.Resolutions[request.Video.Resolution]
	if !ok {
		return CandidateQuote{}, fmt.Errorf("seedance rate is missing for resolution %s", request.Video.Resolution)
	}
	rateRaw := rate.WithoutInputVideoMillionTokensCNY
	withInputVideo := inputVideoSeconds.GreaterThan(decimal.Zero)
	if withInputVideo {
		rateRaw = rate.WithInputVideoMillionTokensCNY
	}
	rateCNY, err := parsePositiveDecimal(rateRaw, "seedance million token rate")
	if err != nil {
		return CandidateQuote{}, err
	}
	outputSeconds := decimal.NewFromInt(int64(request.Video.DurationSeconds))
	totalSeconds := outputSeconds.Add(inputVideoSeconds)
	pixelsPerSecond := decimal.NewFromInt(int64(width)).Mul(decimal.NewFromInt(int64(height))).Mul(decimal.NewFromInt(int64(fps)))
	estimatedTokens := totalSeconds.Mul(pixelsPerSecond).Div(decimal.NewFromInt(1024)).Ceil()
	billableTokens := estimatedTokens
	minimumApplied := false
	minimumTokens := decimal.Zero
	if withInputVideo {
		minimumTokens, err = seedanceMinimumTokens(card.ModelCode, request.Video.Resolution, request.Video.AspectRatio, request.Video.DurationSeconds)
		if err != nil {
			return CandidateQuote{}, err
		}
		if billableTokens.LessThan(minimumTokens) {
			billableTokens = minimumTokens
			minimumApplied = true
		}
	}
	cny := billableTokens.Div(decimal.NewFromInt(1_000_000)).Mul(rateCNY)
	calculation := map[string]any{
		"rule_version":           card.RuleVersion,
		"width":                  width,
		"height":                 height,
		"fps":                    fps,
		"estimated_tokens":       estimatedTokens.StringFixed(0),
		"billable_tokens":        billableTokens.StringFixed(0),
		"minimum_tokens_applied": minimumApplied,
		"million_token_rate_cny": rateCNY.String(),
	}
	if withInputVideo {
		calculation["minimum_tokens"] = minimumTokens.StringFixed(0)
	}
	return CandidateQuote{CNY: cny.Round(5).StringFixed(5), Calculation: calculation}, nil
}

func quoteMiniMaxH3(request NativePricingRequest, inputVideoSeconds decimal.Decimal, card RateCard) (CandidateQuote, error) {
	if card.RuleVersion != MiniMaxH3RuleVersion202608 {
		return CandidateQuote{}, fmt.Errorf("unsupported minimax h3 rule version %q", card.RuleVersion)
	}
	if card.MiniMaxH3 == nil {
		return CandidateQuote{}, fmt.Errorf("minimax h3 rate config is required")
	}
	rate, ok := card.MiniMaxH3.Resolutions[request.Video.Resolution]
	if !ok {
		return CandidateQuote{}, fmt.Errorf("minimax h3 rate is missing for resolution %s", request.Video.Resolution)
	}
	outputRate, err := parsePositiveDecimal(rate.OutputSecondCNY, "output_second_cny")
	if err != nil {
		return CandidateQuote{}, err
	}
	inputVideoRate := decimal.Zero
	if inputVideoSeconds.GreaterThan(decimal.Zero) {
		inputVideoRate, err = parsePositiveDecimal(rate.InputVideoSecondCNY, "input_video_second_cny")
		if err != nil {
			return CandidateQuote{}, err
		}
	}
	extraImageRate, err := parseNonNegativeDecimal(card.MiniMaxH3.ExtraImageCNY, "extra_image_cny")
	if err != nil {
		return CandidateQuote{}, err
	}
	extraImageCount := request.ReferenceImageCount - card.MiniMaxH3.FreeImageCount
	if extraImageCount < 0 {
		extraImageCount = 0
	}
	outputCost := decimal.NewFromInt(int64(request.Video.DurationSeconds)).Mul(outputRate)
	inputVideoCost := inputVideoSeconds.Mul(inputVideoRate)
	extraImageCost := decimal.NewFromInt(int64(extraImageCount)).Mul(extraImageRate)
	total := outputCost.Add(inputVideoCost).Add(extraImageCost)
	return CandidateQuote{
		CNY: total.Round(5).StringFixed(5),
		Calculation: map[string]any{
			"rule_version":               card.RuleVersion,
			"output_seconds":             request.Video.DurationSeconds,
			"output_second_rate_cny":     outputRate.String(),
			"input_video_seconds":        inputVideoSeconds.String(),
			"input_video_second_cny":     inputVideoRate.String(),
			"reference_image_count":      request.ReferenceImageCount,
			"free_image_count":           card.MiniMaxH3.FreeImageCount,
			"billable_extra_image_count": extraImageCount,
			"extra_image_cny":            extraImageRate.String(),
			"input_audio_free":           request.HasInputAudio && card.MiniMaxH3.InputAudioFree,
		},
	}, nil
}

func capabilityResolutions(capability Capability) map[Resolution]struct{} {
	result := make(map[Resolution]struct{})
	for _, task := range capability.TaskTypes {
		for _, resolution := range task.Resolutions {
			result[resolution] = struct{}{}
		}
	}
	return result
}

func capabilitySupportsMediaType(capability Capability, mediaType string) bool {
	for _, task := range capability.TaskTypes {
		for _, input := range task.Inputs {
			for _, supported := range input.MediaTypes {
				if strings.EqualFold(strings.TrimSpace(supported), mediaType) {
					return true
				}
			}
		}
	}
	return false
}

func seedanceModelSupportsResolution(modelCode string, resolution Resolution) bool {
	model := strings.ToLower(strings.TrimSpace(modelCode))
	switch {
	case strings.Contains(model, "seedance-2-5"):
		return resolution == Resolution480P || resolution == Resolution720P
	case strings.Contains(model, "seedance-2-0"):
		return resolution == Resolution480P || resolution == Resolution720P || resolution == Resolution1080P || resolution == Resolution4K
	default:
		return false
	}
}

func seedanceOutputPreset(modelCode string, resolution Resolution, ratio AspectRatio) (int, int, int, error) {
	if !seedanceModelSupportsResolution(modelCode, resolution) {
		return 0, 0, 0, fmt.Errorf("unsupported seedance model/resolution preset %s/%s", modelCode, resolution)
	}
	longSide, shortSide := 0, 0
	switch resolution {
	case Resolution480P:
		longSide, shortSide = 854, 480
	case Resolution720P:
		longSide, shortSide = 1280, 720
	case Resolution1080P:
		longSide, shortSide = 1920, 1080
	case Resolution4K:
		longSide, shortSide = 3840, 2160
	default:
		return 0, 0, 0, fmt.Errorf("unsupported seedance resolution %s", resolution)
	}
	switch ratio {
	case AspectRatio16x9:
		return longSide, shortSide, 24, nil
	case AspectRatio9x16:
		return shortSide, longSide, 24, nil
	case AspectRatio1x1:
		return shortSide, shortSide, 24, nil
	case AspectRatio4x3:
		return shortSide * 4 / 3, shortSide, 24, nil
	case AspectRatio3x4:
		return shortSide, shortSide * 4 / 3, 24, nil
	case AspectRatio21x9:
		return shortSide * 21 / 9, shortSide, 24, nil
	default:
		return 0, 0, 0, fmt.Errorf("unsupported seedance aspect ratio %s", ratio)
	}
}

func seedanceMinimumTokens(modelCode string, resolution Resolution, ratio AspectRatio, outputSeconds int) (decimal.Decimal, error) {
	width, height, fps, err := seedanceOutputPreset(modelCode, resolution, ratio)
	if err != nil {
		return decimal.Zero, err
	}
	if outputSeconds <= 0 {
		return decimal.Zero, fmt.Errorf("duration_seconds must be positive")
	}
	// The 2026-08 official input-video table floors billing at two input seconds.
	minimumTotalSeconds := decimal.NewFromInt(int64(outputSeconds + 2))
	return minimumTotalSeconds.
		Mul(decimal.NewFromInt(int64(width))).
		Mul(decimal.NewFromInt(int64(height))).
		Mul(decimal.NewFromInt(int64(fps))).
		Div(decimal.NewFromInt(1024)).Ceil(), nil
}

func parsePositiveDecimal(raw, field string) (decimal.Decimal, error) {
	value, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil || !value.GreaterThan(decimal.Zero) {
		return decimal.Zero, fmt.Errorf("%s must be positive decimal", field)
	}
	return value, nil
}

func parseNonNegativeDecimal(raw, field string) (decimal.Decimal, error) {
	value, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil || value.IsNegative() {
		return decimal.Zero, fmt.Errorf("%s must be a non-negative decimal", field)
	}
	return value, nil
}

func parseOptionalPositiveDecimal(raw, field string) (decimal.Decimal, error) {
	if strings.TrimSpace(raw) == "" {
		return decimal.Zero, nil
	}
	return parsePositiveDecimal(raw, field)
}
