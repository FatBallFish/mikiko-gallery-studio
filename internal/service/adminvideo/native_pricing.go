package adminvideo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type ModelPricingContext struct {
	AccountModelID int64
	ProviderCode   string
	ModelCode      string
	Capability     map[string]any
}

type RouteQuoteCandidate struct {
	RouteCandidateID       int64
	AccountModelID         int64
	ProviderCode           string
	ModelCode              string
	PreflightExclusionCode string
	CapabilityVersion      string
	Capability             map[string]any
	ResolutionMappings     map[string]string
	RateCard               RateCardSummary
}

type RouteQuoteContext struct {
	Route             RouteConfigSummary
	Candidates        []RouteQuoteCandidate
	CNYPerPoint       string
	ConversionVersion string
}

type QuoteSimulationRequest struct {
	RouteModelID        int64               `json:"route_model_id"`
	TaskType            string              `json:"task_type"`
	Resolution          string              `json:"resolution"`
	AspectRatio         string              `json:"aspect_ratio"`
	AudioMode           string              `json:"audio_mode"`
	DurationSeconds     int                 `json:"duration_seconds"`
	OutputCount         int                 `json:"output_count"`
	ReferenceImageCount int                 `json:"reference_image_count"`
	InputVideoSeconds   string              `json:"input_video_seconds"`
	HasInputAudio       bool                `json:"has_input_audio"`
	Inputs              []domainvideo.Input `json:"inputs,omitempty"`
}

type QuoteSimulationCandidate struct {
	RouteCandidateID  int64          `json:"route_candidate_id"`
	AccountModelID    int64          `json:"account_model_id"`
	ProviderCode      string         `json:"provider_code"`
	ModelCode         string         `json:"model_code"`
	CapabilityVersion string         `json:"capability_version"`
	PricingSchema     string         `json:"pricing_schema"`
	RateVersion       int            `json:"rate_version"`
	Eligible          bool           `json:"eligible"`
	MappedResolution  string         `json:"mapped_resolution"`
	EstimatedCNY      string         `json:"estimated_cny"`
	ExclusionCode     string         `json:"exclusion_code"`
	Calculation       map[string]any `json:"calculation"`
}

type QuoteSimulationResult struct {
	RouteModelID          int64                      `json:"route_model_id"`
	ConfigVersion         string                     `json:"config_version"`
	Candidates            []QuoteSimulationCandidate `json:"candidates"`
	HighestAccountModelID int64                      `json:"highest_account_model_id"`
	HighestCNY            string                     `json:"highest_cny"`
	CNYPerPoint           string                     `json:"cny_per_point"`
	ConversionVersion     string                     `json:"conversion_version"`
	MinimumTaskPoints     string                     `json:"minimum_task_points"`
	RoundingStepPoints    int                        `json:"rounding_step_points"`
	UnitPoints            string                     `json:"unit_points"`
	TotalPoints           string                     `json:"total_points"`
}

func (s *Service) SaveVideoModelRateCard(ctx context.Context, input RateCardWrite) (RateCardSummary, error) {
	if s == nil || s.store == nil || input.AccountModelID <= 0 || input.ExpectedRateVersion < 0 {
		return RateCardSummary{}, errs.BadRequest("invalid video rate card")
	}
	model, err := s.store.GetVideoModelPricingContext(ctx, input.AccountModelID)
	if err != nil {
		return RateCardSummary{}, err
	}
	if model.AccountModelID != input.AccountModelID {
		return RateCardSummary{}, errs.BadRequest("video rate card does not match the real model")
	}
	input.ProviderCode = strings.ToLower(strings.TrimSpace(model.ProviderCode))
	capability, err := decodePricingCapability(model.Capability)
	if err != nil {
		return RateCardSummary{}, err
	}
	card, err := decodeNativeRateCard(input.ProviderCode, model.ModelCode, input.PricingSchema, input.RateConfig)
	if err != nil {
		return RateCardSummary{}, err
	}
	if err := domainvideo.ValidateRateCard(card, capability); err != nil {
		return RateCardSummary{}, errs.WithDetails(errs.New(422, errs.CodeValidationFailed, "video rate card is invalid"), map[string]any{"reason": err.Error()})
	}
	input.Currency = "CNY"
	input.SourceReference = nativePricingSourceReference(input.PricingSchema)
	input.EffectiveAt = time.Now().UTC()
	return s.store.SaveVideoModelRateCard(ctx, input)
}

func nativePricingSourceReference(schema string) string {
	switch domainvideo.PricingSchema(schema) {
	case domainvideo.PricingSchemaSeedanceTokenV1:
		return "https://docs.volcengine.com/docs/82379/1544106?lang=zh"
	case domainvideo.PricingSchemaMiniMaxH3SecondV1:
		return "https://platform.minimaxi.com/docs/guides/pricing-paygo"
	default:
		return ""
	}
}

func (s *Service) ListVideoModelRateCards(ctx context.Context, accountModelID int64) ([]RateCardSummary, error) {
	if s == nil || s.store == nil || accountModelID <= 0 {
		return nil, errs.BadRequest("account_model_id is required")
	}
	return s.store.ListVideoModelRateCards(ctx, accountModelID)
}

func (s *Service) DeleteVideoModelRateCard(ctx context.Context, id int64, expectedVersion int) error {
	if s == nil || s.store == nil || id <= 0 || expectedVersion <= 0 {
		return errs.BadRequest("invalid video rate card delete request")
	}
	return s.store.DeleteVideoModelRateCard(ctx, id, expectedVersion)
}

func (s *Service) SimulateRouteQuote(ctx context.Context, input QuoteSimulationRequest) (QuoteSimulationResult, error) {
	if s == nil || s.store == nil || input.RouteModelID <= 0 {
		return QuoteSimulationResult{}, errs.BadRequest("route_model_id is required")
	}
	if input.OutputCount == 0 {
		input.OutputCount = 1
	}
	contextValue, err := s.store.GetVideoRouteQuoteContext(ctx, input.RouteModelID, time.Now().UTC())
	if err != nil {
		return QuoteSimulationResult{}, err
	}
	result := QuoteSimulationResult{
		RouteModelID: input.RouteModelID, ConfigVersion: contextValue.Route.ConfigVersion,
		Candidates:  make([]QuoteSimulationCandidate, 0, len(contextValue.Candidates)),
		CNYPerPoint: contextValue.CNYPerPoint, ConversionVersion: contextValue.ConversionVersion,
		MinimumTaskPoints:  normalizePointString(contextValue.Route.MinimumTaskPoints),
		RoundingStepPoints: contextValue.Route.RoundingStepPoints,
	}
	if result.RoundingStepPoints == 0 {
		result.RoundingStepPoints = 1
	}
	highest := decimal.Zero
	eligibleCount := 0
	for _, candidate := range contextValue.Candidates {
		row, value, candidateErr := evaluateRouteQuoteCandidate(input, candidate)
		if candidateErr != nil {
			return QuoteSimulationResult{}, candidateErr
		}
		if row.Eligible {
			eligibleCount++
		}
		if row.Eligible && value.GreaterThan(highest) {
			highest = value
			result.HighestAccountModelID = candidate.AccountModelID
		}
		result.Candidates = append(result.Candidates, row)
	}
	if eligibleCount == 0 {
		return QuoteSimulationResult{}, errs.WithDetails(
			errs.New(409, errs.CodeVideoRoutePriceUnavailable, "no eligible video candidate has a valid price"),
			map[string]any{"candidates": result.Candidates},
		)
	}
	cnyPerPoint, err := parsePositivePointDecimal(contextValue.CNYPerPoint, "cny_per_point")
	if err != nil {
		return QuoteSimulationResult{}, err
	}
	minimum, err := parseNonNegativePointDecimal(result.MinimumTaskPoints, "minimum_task_points")
	if err != nil {
		return QuoteSimulationResult{}, err
	}
	step := decimal.NewFromInt(int64(result.RoundingStepPoints))
	unit := highest.Div(cnyPerPoint).Div(step).Ceil().Mul(step)
	if unit.LessThan(minimum) {
		unit = minimum
	}
	result.HighestCNY = highest.StringFixed(5)
	result.UnitPoints = unit.StringFixed(5)
	result.TotalPoints = unit.Mul(decimal.NewFromInt(int64(input.OutputCount))).StringFixed(5)
	return result, nil
}

func evaluateRouteQuoteCandidate(input QuoteSimulationRequest, candidate RouteQuoteCandidate) (QuoteSimulationCandidate, decimal.Decimal, error) {
	row := QuoteSimulationCandidate{
		RouteCandidateID: candidate.RouteCandidateID, AccountModelID: candidate.AccountModelID,
		ProviderCode: candidate.ProviderCode, ModelCode: candidate.ModelCode,
		CapabilityVersion: candidate.CapabilityVersion, PricingSchema: candidate.RateCard.PricingSchema,
		RateVersion: candidate.RateCard.RateVersion, EstimatedCNY: "0.00000",
		Calculation: map[string]any{},
	}
	mappedRequest := domainvideo.Request{
		TaskType: domainvideo.TaskType(input.TaskType), DurationSeconds: input.DurationSeconds,
		Resolution: domainvideo.Resolution(input.Resolution), AspectRatio: domainvideo.AspectRatio(input.AspectRatio),
		AudioMode: domainvideo.AudioMode(input.AudioMode), OutputCount: input.OutputCount,
		Inputs: append([]domainvideo.Input(nil), input.Inputs...),
	}
	row.MappedResolution = input.Resolution
	if mapped := candidate.ResolutionMappings[input.Resolution]; mapped != "" {
		row.MappedResolution = mapped
		mappedRequest.Resolution = domainvideo.Resolution(mapped)
	}
	if candidate.PreflightExclusionCode != "" {
		row.ExclusionCode = candidate.PreflightExclusionCode
		return row, decimal.Zero, nil
	}
	capability, decodeErr := decodePricingCapability(candidate.Capability)
	if decodeErr != nil {
		row.ExclusionCode = "VIDEO_CAPABILITY_INVALID"
		return row, decimal.Zero, nil
	}
	if match := capability.Match(mappedRequest); !match.Matches {
		row.ExclusionCode = errs.CodeVideoCapabilityMismatch
		row.Calculation["field_errors"] = match.FieldErrors
		return row, decimal.Zero, nil
	}
	if candidate.RateCard.RateVersion <= 0 {
		row.ExclusionCode = errs.CodeVideoRateCardMissing
		return row, decimal.Zero, nil
	}
	card, decodeErr := decodeNativeRateCard(candidate.ProviderCode, candidate.ModelCode, candidate.RateCard.PricingSchema, candidate.RateCard.RateConfig)
	if decodeErr != nil {
		row.ExclusionCode = nativeRateCardExclusionCode(decodeErr)
		row.Calculation["error"] = decodeErr.Error()
		return row, decimal.Zero, nil
	}
	if validationErr := domainvideo.ValidateRateCard(card, capability); validationErr != nil {
		row.ExclusionCode = errs.CodeVideoRateCardInvalid
		row.Calculation["error"] = validationErr.Error()
		return row, decimal.Zero, nil
	}
	quote, quoteErr := domainvideo.QuoteNativePricing(domainvideo.NativePricingRequest{
		Video: mappedRequest, InputVideoSeconds: input.InputVideoSeconds,
		ReferenceImageCount: input.ReferenceImageCount, HasInputAudio: input.HasInputAudio,
	}, card)
	if quoteErr != nil {
		row.ExclusionCode = errs.CodeVideoRateCardInvalid
		row.Calculation["error"] = quoteErr.Error()
		return row, decimal.Zero, nil
	}
	value, parseErr := decimal.NewFromString(quote.CNY)
	if parseErr != nil {
		return QuoteSimulationCandidate{}, decimal.Zero, errs.Internal("invalid native video quote")
	}
	row.Eligible = true
	row.EstimatedCNY = value.StringFixed(5)
	row.Calculation = quote.Calculation
	return row, value, nil
}

func routeQuoteContextHasPriceableCombination(contextValue RouteQuoteContext, combo VisibleCombination) bool {
	for _, candidate := range contextValue.Candidates {
		input := QuoteSimulationRequest{
			RouteModelID: contextValue.Route.RouteModelID, TaskType: combo.TaskType,
			Resolution: combo.Resolution, AspectRatio: combo.AspectRatio, AudioMode: combo.AudioMode,
			DurationSeconds: combo.DurationSeconds, OutputCount: 1,
		}
		capability, err := decodePricingCapability(candidate.Capability)
		if err == nil {
			if task, ok := capability.TaskTypes[domainvideo.TaskType(combo.TaskType)]; ok {
				input.Inputs = capabilityValidationInputs(task)
				for _, mediaInput := range input.Inputs {
					switch {
					case strings.EqualFold(mediaInput.MediaType, "image"):
						input.ReferenceImageCount++
					case strings.EqualFold(mediaInput.MediaType, "video"):
						input.InputVideoSeconds = "1"
					case strings.EqualFold(mediaInput.MediaType, "audio"):
						input.HasInputAudio = true
					}
				}
			}
		}
		row, _, err := evaluateRouteQuoteCandidate(input, candidate)
		if err == nil && row.Eligible {
			return true
		}
	}
	return false
}

func nativeRateCardExclusionCode(err error) string {
	if typed, ok := err.(*errs.Error); ok && typed.Code == errs.CodeVideoPricingSchemaUnsupported {
		return errs.CodeVideoPricingSchemaUnsupported
	}
	return errs.CodeVideoRateCardInvalid
}

func decodeNativeRateCard(providerCode, modelCode, schema string, config map[string]any) (domainvideo.RateCard, error) {
	payload, err := json.Marshal(config)
	if err != nil {
		return domainvideo.RateCard{}, errs.BadRequest("invalid video rate config")
	}
	card := domainvideo.RateCard{ProviderCode: providerCode, ModelCode: modelCode, PricingSchema: domainvideo.PricingSchema(schema)}
	switch card.PricingSchema {
	case domainvideo.PricingSchemaSeedanceTokenV1:
		card.RuleVersion = domainvideo.SeedanceRuleVersion202608
		card.Seedance = &domainvideo.SeedanceTokenRateCard{}
		if err := json.Unmarshal(payload, card.Seedance); err != nil {
			return domainvideo.RateCard{}, errs.BadRequest("invalid seedance rate config")
		}
	case domainvideo.PricingSchemaMiniMaxH3SecondV1:
		card.RuleVersion = domainvideo.MiniMaxH3RuleVersion202608
		card.MiniMaxH3 = &domainvideo.MiniMaxH3SecondRateCard{}
		if err := json.Unmarshal(payload, card.MiniMaxH3); err != nil {
			return domainvideo.RateCard{}, errs.BadRequest("invalid minimax h3 rate config")
		}
	default:
		return domainvideo.RateCard{}, errs.New(422, errs.CodeVideoPricingSchemaUnsupported, fmt.Sprintf("unsupported video pricing schema %q", schema))
	}
	return card, nil
}

func decodePricingCapability(value map[string]any) (domainvideo.Capability, error) {
	if len(value) == 0 {
		return domainvideo.Capability{}, errs.BadRequest("invalid video capability")
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return domainvideo.Capability{}, errs.BadRequest("invalid video capability")
	}
	var capability domainvideo.Capability
	if err := json.Unmarshal(payload, &capability); err != nil {
		return domainvideo.Capability{}, errs.BadRequest("invalid video capability")
	}
	if capability.SchemaVersion != 1 || capability.ProviderNativeMaxN < 1 || capability.ProviderNativeMaxN > 10 || len(capability.TaskTypes) == 0 {
		return domainvideo.Capability{}, errs.BadRequest("invalid video capability")
	}
	return capability, nil
}

func normalizePointString(value string) string {
	if strings.TrimSpace(value) == "" {
		return "0.00000"
	}
	parsed, err := decimal.NewFromString(strings.TrimSpace(value))
	if err != nil {
		return value
	}
	return parsed.StringFixed(5)
}

func parsePositivePointDecimal(value, field string) (decimal.Decimal, error) {
	parsed, err := decimal.NewFromString(strings.TrimSpace(value))
	if err != nil || !parsed.GreaterThan(decimal.Zero) {
		return decimal.Zero, errs.BadRequest(field + " must be positive")
	}
	return parsed, nil
}

func parseNonNegativePointDecimal(value, field string) (decimal.Decimal, error) {
	parsed, err := decimal.NewFromString(strings.TrimSpace(value))
	if err != nil || parsed.IsNegative() {
		return decimal.Zero, errs.BadRequest(field + " must be non-negative")
	}
	return parsed, nil
}
