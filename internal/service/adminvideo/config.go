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

type PointProduct struct {
	ID          int64  `json:"id"`
	Code        string `json:"code"`
	PriceCNY    string `json:"price_cny"`
	Points      string `json:"points"`
	BonusPoints string `json:"bonus_points"`
	Enabled     bool   `json:"enabled"`
}

type ConfigKind string

const (
	ConfigCapability ConfigKind = "capability"
	ConfigCostRule   ConfigKind = "cost_rule"
	ConfigStrategy   ConfigKind = "strategy"
	ConfigPriceRule  ConfigKind = "price_rule"
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

type CostRuleWrite struct {
	ID                  int64          `json:"id"`
	AccountModelID      int64          `json:"account_model_id"`
	ExpectedRuleVersion int            `json:"expected_rule_version"`
	BillingMode         string         `json:"billing_mode"`
	Currency            string         `json:"currency"`
	Rates               map[string]any `json:"rates"`
	CostReserveMarkup   string         `json:"cost_reserve_markup"`
	ValidationStatus    string         `json:"validation_status"`
	SourceType          string         `json:"source_type"`
	SourceReference     string         `json:"source_reference"`
	EffectiveAt         time.Time      `json:"effective_at"`
	ExpiresAt           *time.Time     `json:"expires_at,omitempty"`
	Enabled             bool           `json:"enabled"`
}

type RateCardWrite struct {
	AccountModelID      int64          `json:"account_model_id"`
	ProviderCode        string         `json:"provider_code"`
	PricingSchema       string         `json:"pricing_schema"`
	ExpectedRateVersion int            `json:"expected_rate_version"`
	Currency            string         `json:"currency"`
	RateConfig          map[string]any `json:"rate_config"`
	SourceReference     string         `json:"source_reference"`
	EffectiveAt         time.Time      `json:"effective_at"`
	Enabled             bool           `json:"enabled"`
}

type StrategyWrite struct {
	ID                          int64  `json:"id"`
	ExpectedVersion             int    `json:"expected_version"`
	Code                        string `json:"code"`
	Name                        string `json:"name"`
	GrossPointValueCNY          string `json:"gross_point_value_cny"`
	MinimumNetPointIncomeCNY    string `json:"minimum_net_point_income_cny"`
	MaxBonusRatio               string `json:"max_bonus_ratio"`
	PaymentFeeRate              string `json:"payment_fee_rate"`
	TargetMarginRate            string `json:"target_margin_rate"`
	ProviderCostBufferRate      string `json:"provider_cost_buffer_rate"`
	PlatformFixedCostCNY        string `json:"platform_fixed_cost_cny"`
	PlatformOutputSecondCostCNY string `json:"platform_output_second_cost_cny"`
	PlatformReferenceCostCNY    string `json:"platform_reference_cost_cny"`
	PlatformAudioFixedCostCNY   string `json:"platform_audio_fixed_cost_cny"`
	PlatformAudioSecondCostCNY  string `json:"platform_audio_second_cost_cny"`
	ExactReserveMarkup          string `json:"exact_reserve_markup"`
	MeteredReserveMarkup        string `json:"metered_reserve_markup"`
	Enabled                     bool   `json:"enabled"`
}

type PriceRuleWrite struct {
	ID                         int64          `json:"id"`
	RouteModelID               int64          `json:"route_model_id"`
	StrategyID                 int64          `json:"pricing_strategy_id"`
	ExpectedVersion            int            `json:"expected_version"`
	TaskType                   string         `json:"task_type"`
	Resolution                 string         `json:"resolution"`
	AudioMode                  string         `json:"audio_mode"`
	PricingMode                string         `json:"pricing_mode"`
	DurationSeconds            int            `json:"duration_seconds"`
	EffectiveAt                time.Time      `json:"effective_at"`
	ExpiresAt                  *time.Time     `json:"expires_at,omitempty"`
	OutputSecondPoints         string         `json:"output_second_points"`
	FixedTaskPoints            string         `json:"fixed_task_points"`
	ReferenceImagePoints       string         `json:"reference_image_points"`
	InputVideoSecondPoints     string         `json:"input_video_second_points"`
	ReferenceAudioSecondPoints string         `json:"reference_audio_second_points"`
	GeneratedAudioFixedPoints  string         `json:"generated_audio_fixed_points"`
	GeneratedAudioSecondPoints string         `json:"generated_audio_second_points"`
	MinimumBillableSeconds     int            `json:"minimum_billable_seconds"`
	MinimumTaskPoints          string         `json:"minimum_task_points"`
	ReserveMarkup              string         `json:"reserve_markup"`
	SafetyPoints               string         `json:"safety_points"`
	CandidateCostUpperCNY      string         `json:"candidate_cost_upper_cny"`
	SafetySnapshot             map[string]any `json:"safety_snapshot"`
	Enabled                    bool           `json:"enabled"`
	InternalNote               string         `json:"internal_note"`
}

type VisibleCombination struct {
	TaskType        string `json:"task_type"`
	Resolution      string `json:"resolution"`
	AspectRatio     string `json:"aspect_ratio"`
	AudioMode       string `json:"audio_mode"`
	DurationSeconds int    `json:"duration_seconds"`
}
type RouteConfigWrite struct {
	RouteModelID               int64                `json:"route_model_id"`
	ExpectedVersion            string               `json:"expected_version"`
	ConfigVersion              string               `json:"config_version"`
	PricingStrategyID          int64                `json:"pricing_strategy_id"`
	CandidateParameterMappings map[string]any       `json:"candidate_parameter_mappings"`
	MinimumTaskPoints          string               `json:"minimum_task_points"`
	RoundingStepPoints         int                  `json:"rounding_step_points"`
	TaskTypes                  []string             `json:"task_types"`
	VisibleOptions             map[string]any       `json:"visible_options"`
	Defaults                   map[string]any       `json:"defaults"`
	VisibleCombinations        []VisibleCombination `json:"visible_combinations"`
	MaxOutputCount             int                  `json:"max_output_count"`
	Enabled                    bool                 `json:"enabled"`
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

func (s *Service) SaveCostRule(ctx context.Context, input CostRuleWrite) (CostRuleSummary, error) {
	if input.AccountModelID <= 0 || input.ExpectedRuleVersion < 0 || input.EffectiveAt.IsZero() || len(input.Rates) == 0 {
		return CostRuleSummary{}, errs.BadRequest("invalid video cost rule")
	}
	if input.Enabled && input.ValidationStatus != "verified" {
		return CostRuleSummary{}, errs.New(409, errs.CodeConflict, "only verified provider cost rules can be enabled")
	}
	return s.store.SaveCostRule(ctx, input)
}

func (s *Service) SaveStrategy(ctx context.Context, input StrategyWrite) (PricingStrategySummary, error) {
	if err := validateStrategy(input); err != nil {
		return PricingStrategySummary{}, err
	}
	snapshot, err := s.Snapshot(ctx)
	if err != nil {
		return PricingStrategySummary{}, err
	}
	if input.Enabled {
		actual, products, err := minimumPointProductIncome(snapshot.Plans, input.PaymentFeeRate)
		if err != nil {
			return PricingStrategySummary{}, err
		}
		required, parseErr := decimal.NewFromString(input.MinimumNetPointIncomeCNY)
		if parseErr != nil || actual.LessThan(required) {
			return PricingStrategySummary{}, errs.New(409, errs.CodeConflict, fmt.Sprintf("point product net income is below protection floor: %s (%s)", actual.StringFixed(5), strings.Join(products, ",")))
		}
	}
	return s.store.SaveStrategy(ctx, input)
}

func validateStrategy(input StrategyWrite) error {
	for name, raw := range map[string]string{
		"gross_point_value_cny": input.GrossPointValueCNY, "minimum_net_point_income_cny": input.MinimumNetPointIncomeCNY,
		"platform_fixed_cost_cny": input.PlatformFixedCostCNY, "platform_output_second_cost_cny": input.PlatformOutputSecondCostCNY,
		"platform_reference_cost_cny": input.PlatformReferenceCostCNY, "platform_audio_fixed_cost_cny": input.PlatformAudioFixedCostCNY,
		"platform_audio_second_cost_cny": input.PlatformAudioSecondCostCNY,
	} {
		if raw == "" {
			raw = "0"
		}
		value, err := decimal.NewFromString(raw)
		if err != nil || value.IsNegative() {
			return errs.BadRequest(name + " must be a non-negative decimal")
		}
	}
	for name, raw := range map[string]string{
		"max_bonus_ratio": input.MaxBonusRatio, "payment_fee_rate": input.PaymentFeeRate,
		"target_margin_rate": input.TargetMarginRate, "provider_cost_buffer_rate": input.ProviderCostBufferRate,
	} {
		if raw == "" {
			raw = "0"
		}
		value, err := decimal.NewFromString(raw)
		if err != nil || value.IsNegative() || !value.LessThan(decimal.NewFromInt(1)) {
			return errs.BadRequest(name + " must be between 0 and 1")
		}
	}
	for name, raw := range map[string]string{"exact_reserve_markup": input.ExactReserveMarkup, "metered_reserve_markup": input.MeteredReserveMarkup} {
		if raw == "" {
			raw = "1"
		}
		value, err := decimal.NewFromString(raw)
		if err != nil || value.LessThan(decimal.NewFromInt(1)) || value.GreaterThan(decimal.NewFromInt(2)) {
			return errs.BadRequest(name + " must be between 1 and 2")
		}
	}
	return nil
}

func minimumPointProductIncome(products []PointProduct, feeRaw string) (decimal.Decimal, []string, error) {
	fee, err := decimal.NewFromString(feeRaw)
	if err != nil || fee.IsNegative() || !fee.LessThan(decimal.NewFromInt(1)) {
		return decimal.Zero, nil, errs.BadRequest("invalid payment_fee_rate")
	}
	minimum := decimal.Zero
	affected := []string{}
	for _, product := range products {
		if !product.Enabled {
			continue
		}
		price, e1 := decimal.NewFromString(product.PriceCNY)
		points, e2 := decimal.NewFromString(product.Points)
		bonusRaw := product.BonusPoints
		if bonusRaw == "" {
			bonusRaw = "0"
		}
		bonus, e3 := decimal.NewFromString(bonusRaw)
		if e1 != nil || e2 != nil || e3 != nil || !price.IsPositive() || !points.Add(bonus).IsPositive() {
			return decimal.Zero, nil, errs.BadRequest("invalid point product pricing")
		}
		income := price.Mul(decimal.NewFromInt(1).Sub(fee)).Div(points.Add(bonus))
		if minimum.IsZero() || income.LessThan(minimum) {
			minimum = income
			affected = []string{product.Code}
		} else if income.Equal(minimum) {
			affected = append(affected, product.Code)
		}
	}
	if minimum.IsZero() {
		return decimal.Zero, nil, errs.New(409, errs.CodeConflict, "no enabled point product can prove net point income")
	}
	return minimum, affected, nil
}

func (s *Service) SavePriceRule(ctx context.Context, input PriceRuleWrite) (PriceRuleSummary, error) {
	if input.EffectiveAt.IsZero() {
		return PriceRuleSummary{}, errs.BadRequest("effective_at is required")
	}
	input.PricingMode = strings.ToLower(strings.TrimSpace(input.PricingMode))
	if input.PricingMode == "" {
		input.PricingMode = "exact"
	}
	if input.PricingMode != "exact" && input.PricingMode != "metered" {
		return PriceRuleSummary{}, errs.BadRequest("pricing_mode must be exact or metered")
	}
	if _, err := domainvideo.CalculateQuote(domainvideo.SalesRule{
		PricingMode:     input.PricingMode,
		FixedTaskPoints: input.FixedTaskPoints, OutputSecondPoints: input.OutputSecondPoints,
		ReferenceImagePoints: input.ReferenceImagePoints, InputVideoSecondPoints: input.InputVideoSecondPoints,
		ReferenceAudioSecondPoints: input.ReferenceAudioSecondPoints, GeneratedAudioFixedPoints: input.GeneratedAudioFixedPoints,
		GeneratedAudioSecondPoints: input.GeneratedAudioSecondPoints, MinimumBillableSeconds: input.MinimumBillableSeconds,
		MinimumTaskPoints: input.MinimumTaskPoints, ReserveMarkup: input.ReserveMarkup,
	}, domainvideo.QuoteRequest{DurationSeconds: maxInt(input.DurationSeconds, 1), OutputCount: 1}); err != nil {
		return PriceRuleSummary{}, errs.BadRequest(err.Error())
	}
	if input.Enabled {
		if input.RouteModelID <= 0 || input.DurationSeconds <= 0 {
			return PriceRuleSummary{}, errs.BadRequest("route_model_id and duration_seconds are required to enable a video price")
		}
		simulation, err := s.Simulate(ctx, SimulationRequest{RouteModelID: input.RouteModelID, StrategyID: input.StrategyID, TaskType: input.TaskType, Resolution: input.Resolution, AudioMode: input.AudioMode, DurationSeconds: input.DurationSeconds})
		if err != nil {
			return PriceRuleSummary{}, err
		}
		quote, err := domainvideo.CalculateQuote(domainvideo.SalesRule{
			PricingMode:     input.PricingMode,
			FixedTaskPoints: input.FixedTaskPoints, OutputSecondPoints: input.OutputSecondPoints,
			ReferenceImagePoints: input.ReferenceImagePoints, InputVideoSecondPoints: input.InputVideoSecondPoints,
			ReferenceAudioSecondPoints: input.ReferenceAudioSecondPoints, GeneratedAudioFixedPoints: input.GeneratedAudioFixedPoints,
			GeneratedAudioSecondPoints: input.GeneratedAudioSecondPoints, MinimumBillableSeconds: input.MinimumBillableSeconds,
			MinimumTaskPoints: input.MinimumTaskPoints, ReserveMarkup: input.ReserveMarkup,
		}, domainvideo.QuoteRequest{DurationSeconds: input.DurationSeconds, OutputCount: 1})
		if err != nil {
			return PriceRuleSummary{}, errs.BadRequest(err.Error())
		}
		sales, _ := decimal.NewFromString(quote.UnitPoints)
		safety, _ := decimal.NewFromString(simulation.SafetyPoints)
		if sales.LessThan(safety) {
			return PriceRuleSummary{}, errs.New(409, errs.CodeConflict, "video price is below server-calculated safety floor")
		}
		input.SafetyPoints = simulation.SafetyPoints
		input.CandidateCostUpperCNY = simulation.WorstCandidateCostCNY
		input.SafetySnapshot = map[string]any{
			"candidate_account_model_id": simulation.CandidateAccountModelID,
			"net_point_income_cny":       simulation.NetPointIncomeCNY,
			"duration_seconds":           input.DurationSeconds,
		}
	}
	return s.store.SavePriceRule(ctx, input)
}

func (s *Service) SaveRouteConfig(ctx context.Context, input RouteConfigWrite) (RouteConfigSummary, error) {
	if input.MaxOutputCount < 1 || input.MaxOutputCount > 4 {
		return RouteConfigSummary{}, errs.BadRequest("max_output_count must be between 1 and 4")
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
		if len(input.VisibleCombinations) == 0 {
			return RouteConfigSummary{}, errs.New(409, errs.CodeConflict, "enabled video route must expose at least one complete combination")
		}
		strategies := make(map[int64]PricingStrategySummary)
		for _, item := range snapshot.Strategies {
			if item.Enabled {
				strategies[item.ID] = item
			}
		}
		strategy := strategies[input.PricingStrategyID]
		if strategy.ID == 0 {
			return RouteConfigSummary{}, errs.New(409, errs.CodeConflict, "video route pricing strategy is not enabled")
		}
		actualIncome, products, incomeErr := minimumPointProductIncome(snapshot.Plans, strategy.PaymentFeeRate)
		floor, floorErr := decimal.NewFromString(strategy.MinimumNetPointIncomeCNY)
		if incomeErr != nil {
			return RouteConfigSummary{}, incomeErr
		}
		if floorErr != nil || actualIncome.LessThan(floor) {
			return RouteConfigSummary{}, errs.New(409, errs.CodeConflict, fmt.Sprintf("point product net income is below protection floor: %s (%s)", actualIncome.StringFixed(5), strings.Join(products, ",")))
		}
		bindings, bindingErr := decodePricingBindings(input.VisibleOptions)
		if bindingErr != nil {
			return RouteConfigSummary{}, bindingErr
		}
		for _, binding := range bindings {
			if _, ok := strategies[binding.PricingStrategyID]; !ok {
				return RouteConfigSummary{}, errs.New(409, errs.CodeConflict, "video route pricing binding strategy is not enabled")
			}
		}
		rules := map[string]PriceRuleSummary{}
		for _, rule := range snapshot.PriceRules {
			if rule.Enabled {
				rules[fmt.Sprintf("%d/%s/%s/%s", rule.StrategyID, rule.TaskType, rule.Resolution, rule.AudioMode)] = rule
			}
		}
		for _, combo := range input.VisibleCombinations {
			if combo.TaskType == "" || combo.Resolution == "" || combo.AudioMode == "" || combo.DurationSeconds <= 0 {
				return RouteConfigSummary{}, errs.BadRequest("visible video combination is incomplete")
			}
			if !snapshotSupportsCombination(snapshot, route.CandidateAccountModelIDs, combo) {
				return RouteConfigSummary{}, errs.New(409, errs.CodeConflict, "visible video combination is unsupported by every enabled candidate")
			}
			strategyID := pricingStrategyForCombination(input.PricingStrategyID, bindings, combo)
			rule, ok := rules[fmt.Sprintf("%d/%s/%s/%s", strategyID, combo.TaskType, combo.Resolution, combo.AudioMode)]
			if !ok {
				return RouteConfigSummary{}, errs.New(409, errs.CodeConflict, "visible video combination has no price")
			}
			simulation, simulationErr := s.Simulate(ctx, SimulationRequest{RouteModelID: input.RouteModelID, StrategyID: strategyID, TaskType: combo.TaskType, Resolution: combo.Resolution, AudioMode: combo.AudioMode, DurationSeconds: combo.DurationSeconds})
			if simulationErr != nil {
				return RouteConfigSummary{}, simulationErr
			}
			sales, salesErr := decimal.NewFromString(rule.SalesPoints)
			safety, safetyErr := decimal.NewFromString(simulation.SafetyPoints)
			if salesErr != nil || safetyErr != nil || sales.LessThan(safety) {
				return RouteConfigSummary{}, errs.New(409, errs.CodeConflict, "visible video combination price is below safety floor")
			}
		}
	}
	return s.store.SaveRouteConfig(ctx, input)
}

type pricingBinding struct {
	TaskType          string `json:"task_type"`
	Resolution        string `json:"resolution"`
	AspectRatio       string `json:"aspect_ratio"`
	AudioMode         string `json:"audio_mode"`
	DurationSeconds   int    `json:"duration_seconds"`
	PricingStrategyID int64  `json:"pricing_strategy_id"`
}

func decodePricingBindings(options map[string]any) ([]pricingBinding, error) {
	if options == nil || options["pricing_bindings"] == nil {
		return nil, nil
	}
	raw, err := json.Marshal(options["pricing_bindings"])
	if err != nil {
		return nil, errs.BadRequest("invalid video pricing bindings")
	}
	var bindings []pricingBinding
	if json.Unmarshal(raw, &bindings) != nil {
		return nil, errs.BadRequest("invalid video pricing bindings")
	}
	for _, binding := range bindings {
		if binding.TaskType == "" || binding.Resolution == "" || binding.AudioMode == "" || binding.PricingStrategyID <= 0 {
			return nil, errs.BadRequest("video pricing binding is incomplete")
		}
	}
	return bindings, nil
}

func pricingStrategyForCombination(fallback int64, bindings []pricingBinding, combo VisibleCombination) int64 {
	for _, binding := range bindings {
		if binding.TaskType != combo.TaskType || binding.Resolution != combo.Resolution || binding.AudioMode != combo.AudioMode {
			continue
		}
		if binding.AspectRatio != "" && binding.AspectRatio != combo.AspectRatio {
			continue
		}
		if binding.DurationSeconds > 0 && binding.DurationSeconds != combo.DurationSeconds {
			continue
		}
		return binding.PricingStrategyID
	}
	return fallback
}

func snapshotSupportsCombination(snapshot Snapshot, candidateIDs []int64, combo VisibleCombination) bool {
	for _, capability := range snapshot.Capabilities {
		if !capability.Enabled || capability.ValidationState != "verified" || !containsInt64(candidateIDs, capability.AccountModelID) {
			continue
		}
		payload, err := json.Marshal(capability.Capability)
		if err != nil {
			continue
		}
		var parsed domainvideo.Capability
		if json.Unmarshal(payload, &parsed) != nil {
			continue
		}
		task, ok := parsed.TaskTypes[domainvideo.TaskType(combo.TaskType)]
		if !ok || !task.Durations.Contains(combo.DurationSeconds) {
			continue
		}
		if !containsResolutionValue(task.Resolutions, domainvideo.Resolution(combo.Resolution)) || !containsAudioValue(task.AudioModes, domainvideo.AudioMode(combo.AudioMode)) {
			continue
		}
		if combo.AspectRatio != "" && !containsAspectValue(task.AspectRatios, domainvideo.AspectRatio(combo.AspectRatio)) {
			continue
		}
		return true
	}
	return false
}

func containsResolutionValue(values []domainvideo.Resolution, target domainvideo.Resolution) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
func containsAudioValue(values []domainvideo.AudioMode, target domainvideo.AudioMode) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
func containsAspectValue(values []domainvideo.AspectRatio, target domainvideo.AspectRatio) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

type SimulationRequest struct {
	RouteModelID        int64  `json:"route_model_id"`
	StrategyID          int64  `json:"pricing_strategy_id"`
	TaskType            string `json:"task_type"`
	Resolution          string `json:"resolution"`
	AudioMode           string `json:"audio_mode"`
	DurationSeconds     int    `json:"duration_seconds"`
	ReferenceImageCount int    `json:"reference_image_count"`
}
type SimulationResult struct {
	WorstCandidateCostCNY   string `json:"worst_candidate_cost_cny"`
	SafetyPoints            string `json:"safety_points"`
	NetPointIncomeCNY       string `json:"net_point_income_cny"`
	CandidateAccountModelID int64  `json:"candidate_account_model_id"`
}

func (s *Service) Simulate(ctx context.Context, req SimulationRequest) (SimulationResult, error) {
	snapshot, err := s.Snapshot(ctx)
	if err != nil {
		return SimulationResult{}, err
	}
	var strategy PricingStrategySummary
	var route RouteConfigSummary
	for _, v := range snapshot.Strategies {
		if v.ID == req.StrategyID {
			strategy = v
		}
	}
	for _, v := range snapshot.Routes {
		if v.RouteModelID == req.RouteModelID {
			route = v
		}
	}
	worst := decimal.Zero
	candidateID := int64(0)
	for _, cost := range snapshot.CostRules {
		if !cost.Enabled || !containsInt64(route.CandidateAccountModelIDs, cost.AccountModelID) {
			continue
		}
		for _, entry := range costCombinationEntries(cost.Rates) {
			if entry.TaskType == req.TaskType && entry.Resolution == req.Resolution && entry.AudioMode == req.AudioMode && entry.DurationSeconds == req.DurationSeconds {
				value, _ := decimal.NewFromString(entry.CostCNY)
				if value.GreaterThan(worst) {
					worst = value
					candidateID = cost.AccountModelID
				}
			}
		}
	}
	if candidateID == 0 {
		return SimulationResult{}, errs.New(409, errs.CodeConflict, "no provider cost for video combination")
	}
	safety, err := domainvideo.SafeMinimumPoints(domainvideo.SafetyInput{CandidateCostsCNY: []string{worst.StringFixed(5)}, ProviderCostBufferRate: strategy.ProviderCostBufferRate, PlatformFixedCostCNY: strategy.PlatformFixedCostCNY, PlatformOutputSecondCostCNY: strategy.PlatformOutputSecondCostCNY, PlatformReferenceCostCNY: strategy.PlatformReferenceCostCNY, DurationSeconds: req.DurationSeconds, ReferenceImageCount: req.ReferenceImageCount, MinimumNetPointIncomeCNY: strategy.MinimumNetPointIncomeCNY, TargetGrossMarginRate: strategy.TargetMarginRate})
	if err != nil {
		return SimulationResult{}, errs.BadRequest(err.Error())
	}
	return SimulationResult{WorstCandidateCostCNY: worst.StringFixed(5), SafetyPoints: safety, NetPointIncomeCNY: strategy.MinimumNetPointIncomeCNY, CandidateAccountModelID: candidateID}, nil
}

type costCombination struct {
	TaskType        string `json:"task_type"`
	Resolution      string `json:"resolution"`
	AudioMode       string `json:"audio_mode"`
	DurationSeconds int    `json:"duration_seconds"`
	CostCNY         string `json:"cost_cny"`
}

func costCombinationEntries(rates map[string]any) []costCombination {
	raw, _ := json.Marshal(rates["combinations"])
	var values []costCombination
	_ = json.Unmarshal(raw, &values)
	return values
}
func containsInt64(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

type RecalculateRequest struct {
	RouteModelID int64               `json:"route_model_id"`
	StrategyID   int64               `json:"pricing_strategy_id"`
	Combinations []SimulationRequest `json:"combinations"`
	EffectiveAt  time.Time           `json:"effective_at"`
}

func (s *Service) Recalculate(ctx context.Context, req RecalculateRequest) ([]PriceRuleSummary, error) {
	snapshot, err := s.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	result := []PriceRuleSummary{}
	for _, combo := range req.Combinations {
		combo.RouteModelID = req.RouteModelID
		combo.StrategyID = req.StrategyID
		simulation, err := s.Simulate(ctx, combo)
		if err != nil {
			return nil, err
		}
		version := 0
		id := int64(0)
		pricingMode := "exact"
		for _, rule := range snapshot.PriceRules {
			if rule.StrategyID == req.StrategyID && rule.TaskType == combo.TaskType && rule.Resolution == combo.Resolution && rule.AudioMode == combo.AudioMode && rule.RuleVersion > version {
				version = rule.RuleVersion
				id = rule.ID
				if rule.PricingMode != "" {
					pricingMode = rule.PricingMode
				}
			}
		}
		saved, err := s.SavePriceRule(ctx, PriceRuleWrite{ID: id, RouteModelID: req.RouteModelID, StrategyID: req.StrategyID, ExpectedVersion: version, TaskType: combo.TaskType, Resolution: combo.Resolution, AudioMode: combo.AudioMode, PricingMode: pricingMode, DurationSeconds: combo.DurationSeconds, EffectiveAt: req.EffectiveAt, MinimumTaskPoints: simulation.SafetyPoints, SafetyPoints: simulation.SafetyPoints, CandidateCostUpperCNY: simulation.WorstCandidateCostCNY, SafetySnapshot: map[string]any{"candidate_account_model_id": simulation.CandidateAccountModelID, "net_point_income_cny": simulation.NetPointIncomeCNY}, Enabled: true})
		if err != nil {
			return nil, err
		}
		result = append(result, saved)
	}
	return result, nil
}
