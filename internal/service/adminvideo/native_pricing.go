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
	RouteCandidateID   int64
	AccountModelID     int64
	ProviderCode       string
	ModelCode          string
	CapabilityVersion  string
	Capability         map[string]any
	ResolutionMappings map[string]string
	RateCard           RateCardSummary
}

type RouteQuoteContext struct {
	Route             RouteConfigSummary
	Candidates        []RouteQuoteCandidate
	CNYPerPoint       string
	ConversionVersion string
}

type QuoteSimulationRequest struct {
	RouteModelID        int64  `json:"route_model_id"`
	TaskType            string `json:"task_type"`
	Resolution          string `json:"resolution"`
	AspectRatio         string `json:"aspect_ratio"`
	AudioMode           string `json:"audio_mode"`
	DurationSeconds     int    `json:"duration_seconds"`
	OutputCount         int    `json:"output_count"`
	ReferenceImageCount int    `json:"reference_image_count"`
	InputVideoSeconds   string `json:"input_video_seconds"`
	HasInputAudio       bool   `json:"has_input_audio"`
}

type QuoteSimulationCandidate struct {
	RouteCandidateID int64          `json:"route_candidate_id"`
	AccountModelID   int64          `json:"account_model_id"`
	ProviderCode     string         `json:"provider_code"`
	ModelCode        string         `json:"model_code"`
	Eligible         bool           `json:"eligible"`
	MappedResolution string         `json:"mapped_resolution"`
	EstimatedCNY     string         `json:"estimated_cny"`
	ExclusionCode    string         `json:"exclusion_code"`
	Calculation      map[string]any `json:"calculation"`
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
	if model.AccountModelID != input.AccountModelID || !strings.EqualFold(strings.TrimSpace(model.ProviderCode), strings.TrimSpace(input.ProviderCode)) {
		return RateCardSummary{}, errs.BadRequest("video rate card provider does not match the real model account")
	}
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
	input.ProviderCode = strings.ToLower(strings.TrimSpace(model.ProviderCode))
	input.Currency = "CNY"
	if input.EffectiveAt.IsZero() {
		input.EffectiveAt = time.Now().UTC()
	}
	return s.store.SaveVideoModelRateCard(ctx, input)
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
	request := domainvideo.Request{
		TaskType: domainvideo.TaskType(input.TaskType), DurationSeconds: input.DurationSeconds,
		Resolution: domainvideo.Resolution(input.Resolution), AspectRatio: domainvideo.AspectRatio(input.AspectRatio),
		AudioMode: domainvideo.AudioMode(input.AudioMode), OutputCount: input.OutputCount,
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
		row := QuoteSimulationCandidate{
			RouteCandidateID: candidate.RouteCandidateID, AccountModelID: candidate.AccountModelID,
			ProviderCode: candidate.ProviderCode, ModelCode: candidate.ModelCode, EstimatedCNY: "0.00000",
			Calculation: map[string]any{},
		}
		mappedRequest := request
		mappedResolution := input.Resolution
		if mapped := candidate.ResolutionMappings[input.Resolution]; mapped != "" {
			mappedResolution = mapped
			mappedRequest.Resolution = domainvideo.Resolution(mapped)
		}
		row.MappedResolution = mappedResolution
		capability, decodeErr := decodePricingCapability(candidate.Capability)
		if decodeErr != nil {
			row.ExclusionCode = "VIDEO_CAPABILITY_INVALID"
			result.Candidates = append(result.Candidates, row)
			continue
		}
		if match := capability.Match(mappedRequest); !match.Matches {
			row.ExclusionCode = errs.CodeVideoCapabilityMismatch
			row.Calculation["field_errors"] = match.FieldErrors
			result.Candidates = append(result.Candidates, row)
			continue
		}
		if candidate.RateCard.RateVersion <= 0 {
			row.ExclusionCode = errs.CodeVideoRateCardMissing
			result.Candidates = append(result.Candidates, row)
			continue
		}
		card, decodeErr := decodeNativeRateCard(candidate.ProviderCode, candidate.ModelCode, candidate.RateCard.PricingSchema, candidate.RateCard.RateConfig)
		if decodeErr != nil {
			row.ExclusionCode = errs.CodeVideoPricingSchemaUnsupported
			row.Calculation["error"] = decodeErr.Error()
			result.Candidates = append(result.Candidates, row)
			continue
		}
		quote, quoteErr := domainvideo.QuoteNativePricing(domainvideo.NativePricingRequest{
			Video: mappedRequest, InputVideoSeconds: input.InputVideoSeconds,
			ReferenceImageCount: input.ReferenceImageCount, HasInputAudio: input.HasInputAudio,
		}, card)
		if quoteErr != nil {
			row.ExclusionCode = errs.CodeVideoPricingSchemaUnsupported
			row.Calculation["error"] = quoteErr.Error()
			result.Candidates = append(result.Candidates, row)
			continue
		}
		value, parseErr := decimal.NewFromString(quote.CNY)
		if parseErr != nil {
			return QuoteSimulationResult{}, errs.Internal("invalid native video quote")
		}
		row.Eligible = true
		row.EstimatedCNY = value.StringFixed(5)
		row.Calculation = quote.Calculation
		eligibleCount++
		if value.GreaterThan(highest) {
			highest = value
			result.HighestAccountModelID = candidate.AccountModelID
		}
		result.Candidates = append(result.Candidates, row)
	}
	if eligibleCount == 0 {
		return QuoteSimulationResult{}, errs.New(409, errs.CodeVideoRateCardMissing, "no eligible video candidate has a valid rate card")
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
	payload, err := json.Marshal(value)
	if err != nil {
		return domainvideo.Capability{}, errs.BadRequest("invalid video capability")
	}
	var capability domainvideo.Capability
	if err := json.Unmarshal(payload, &capability); err != nil {
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
