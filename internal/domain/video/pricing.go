package video

import (
	"fmt"

	"github.com/shopspring/decimal"
)

type SalesRule struct {
	FixedTaskPoints            string
	OutputSecondPoints         string
	ReferenceImagePoints       string
	InputVideoSecondPoints     string
	ReferenceAudioSecondPoints string
	GeneratedAudioFixedPoints  string
	GeneratedAudioSecondPoints string
	MinimumBillableSeconds     int
	MinimumTaskPoints          string
	ReserveMarkup              string
}

type QuoteRequest struct {
	DurationSeconds       int
	ReferenceImageCount   int
	InputVideoSeconds     int
	ReferenceAudioSeconds int
	GenerateAudio         bool
	OutputCount           int
}

type Quote struct {
	UnitPoints        string
	EstimatedPoints   string
	MaxReservedPoints string
}

type SafetyInput struct {
	CandidateCostsCNY           []string
	ProviderCostBufferRate      string
	PlatformFixedCostCNY        string
	PlatformOutputSecondCostCNY string
	PlatformReferenceCostCNY    string
	DurationSeconds             int
	ReferenceImageCount         int
	MinimumNetPointIncomeCNY    string
	TargetGrossMarginRate       string
}

type ItemCharge struct {
	Succeeded bool
	Points    string
}

type Settlement struct {
	ActualPoints   string
	ReleasedPoints string
}

func CalculateQuote(rule SalesRule, request QuoteRequest) (Quote, error) {
	if request.DurationSeconds <= 0 {
		return Quote{}, fmt.Errorf("duration_seconds must be positive")
	}
	if request.OutputCount < 1 || request.OutputCount > 4 {
		return Quote{}, fmt.Errorf("output_count must be between 1 and 4")
	}
	if request.ReferenceImageCount < 0 || request.InputVideoSeconds < 0 || request.ReferenceAudioSeconds < 0 {
		return Quote{}, fmt.Errorf("usage values must not be negative")
	}
	values, err := parseSalesRule(rule)
	if err != nil {
		return Quote{}, err
	}
	billableSeconds := request.DurationSeconds
	if rule.MinimumBillableSeconds > billableSeconds {
		billableSeconds = rule.MinimumBillableSeconds
	}
	unit := values.fixedTask.
		Add(values.outputSecond.Mul(decimal.NewFromInt(int64(billableSeconds)))).
		Add(values.referenceImage.Mul(decimal.NewFromInt(int64(request.ReferenceImageCount)))).
		Add(values.inputVideoSecond.Mul(decimal.NewFromInt(int64(request.InputVideoSeconds)))).
		Add(values.referenceAudioSecond.Mul(decimal.NewFromInt(int64(request.ReferenceAudioSeconds))))
	if request.GenerateAudio {
		unit = unit.
			Add(values.generatedAudioFixed).
			Add(values.generatedAudioSecond.Mul(decimal.NewFromInt(int64(request.DurationSeconds))))
	}
	if unit.LessThan(values.minimumTask) {
		unit = values.minimumTask
	}
	unit = unit.Round(5)
	estimated := unit.Mul(decimal.NewFromInt(int64(request.OutputCount))).Round(5)
	reserved := estimated.Mul(values.reserveMarkup).RoundCeil(5)
	return Quote{
		UnitPoints:        unit.StringFixed(5),
		EstimatedPoints:   estimated.StringFixed(5),
		MaxReservedPoints: reserved.StringFixed(5),
	}, nil
}

func SafeMinimumPoints(input SafetyInput) (string, error) {
	if len(input.CandidateCostsCNY) == 0 {
		return "", fmt.Errorf("at least one candidate cost is required")
	}
	if input.DurationSeconds <= 0 || input.ReferenceImageCount < 0 {
		return "", fmt.Errorf("invalid duration or reference image count")
	}
	worstCost := decimal.Zero
	for index, raw := range input.CandidateCostsCNY {
		value, err := parseNonNegative(raw, fmt.Sprintf("candidate_costs_cny[%d]", index))
		if err != nil {
			return "", err
		}
		if value.GreaterThan(worstCost) {
			worstCost = value
		}
	}
	buffer, err := parseRate(input.ProviderCostBufferRate, "provider_cost_buffer_rate", false)
	if err != nil {
		return "", err
	}
	fixed, err := parseNonNegative(input.PlatformFixedCostCNY, "platform_fixed_cost_cny")
	if err != nil {
		return "", err
	}
	second, err := parseNonNegative(input.PlatformOutputSecondCostCNY, "platform_output_second_cost_cny")
	if err != nil {
		return "", err
	}
	reference, err := parseNonNegative(input.PlatformReferenceCostCNY, "platform_reference_cost_cny")
	if err != nil {
		return "", err
	}
	netIncome, err := parseNonNegative(input.MinimumNetPointIncomeCNY, "minimum_net_point_income_cny")
	if err != nil || netIncome.IsZero() {
		return "", fmt.Errorf("minimum_net_point_income_cny must be positive")
	}
	margin, err := parseRate(input.TargetGrossMarginRate, "target_gross_margin_rate", true)
	if err != nil {
		return "", err
	}
	cost := worstCost.Mul(decimal.NewFromInt(1).Add(buffer)).
		Add(fixed).
		Add(second.Mul(decimal.NewFromInt(int64(input.DurationSeconds)))).
		Add(reference.Mul(decimal.NewFromInt(int64(input.ReferenceImageCount))))
	points := cost.Div(netIncome).Div(decimal.NewFromInt(1).Sub(margin))
	points = points.Mul(decimal.NewFromInt(10)).Ceil().Div(decimal.NewFromInt(10))
	return points.StringFixed(5), nil
}

func SettleQuote(reservedPoints string, items []ItemCharge) (Settlement, error) {
	reserved, err := parseNonNegative(reservedPoints, "reserved_points")
	if err != nil {
		return Settlement{}, err
	}
	actual := decimal.Zero
	for index, item := range items {
		points, err := parseNonNegative(item.Points, fmt.Sprintf("items[%d].points", index))
		if err != nil {
			return Settlement{}, err
		}
		if item.Succeeded {
			actual = actual.Add(points)
		}
	}
	actual = actual.Round(5)
	if actual.GreaterThan(reserved) {
		return Settlement{}, fmt.Errorf("actual points exceed reserved points")
	}
	return Settlement{
		ActualPoints:   actual.StringFixed(5),
		ReleasedPoints: reserved.Sub(actual).Round(5).StringFixed(5),
	}, nil
}

type parsedSalesRule struct {
	fixedTask            decimal.Decimal
	outputSecond         decimal.Decimal
	referenceImage       decimal.Decimal
	inputVideoSecond     decimal.Decimal
	referenceAudioSecond decimal.Decimal
	generatedAudioFixed  decimal.Decimal
	generatedAudioSecond decimal.Decimal
	minimumTask          decimal.Decimal
	reserveMarkup        decimal.Decimal
}

func parseSalesRule(rule SalesRule) (parsedSalesRule, error) {
	fields := []struct {
		name  string
		value string
		dest  *decimal.Decimal
	}{
		{name: "fixed_task_points", value: rule.FixedTaskPoints},
		{name: "output_second_points", value: rule.OutputSecondPoints},
		{name: "reference_image_points", value: rule.ReferenceImagePoints},
		{name: "input_video_second_points", value: rule.InputVideoSecondPoints},
		{name: "reference_audio_second_points", value: rule.ReferenceAudioSecondPoints},
		{name: "generated_audio_fixed_points", value: rule.GeneratedAudioFixedPoints},
		{name: "generated_audio_second_points", value: rule.GeneratedAudioSecondPoints},
		{name: "minimum_task_points", value: rule.MinimumTaskPoints},
	}
	parsed := parsedSalesRule{}
	fields[0].dest = &parsed.fixedTask
	fields[1].dest = &parsed.outputSecond
	fields[2].dest = &parsed.referenceImage
	fields[3].dest = &parsed.inputVideoSecond
	fields[4].dest = &parsed.referenceAudioSecond
	fields[5].dest = &parsed.generatedAudioFixed
	fields[6].dest = &parsed.generatedAudioSecond
	fields[7].dest = &parsed.minimumTask
	for _, field := range fields {
		value := field.value
		if value == "" {
			value = "0"
		}
		amount, err := parseNonNegative(value, field.name)
		if err != nil {
			return parsedSalesRule{}, err
		}
		*field.dest = amount
	}
	markup := rule.ReserveMarkup
	if markup == "" {
		markup = "1"
	}
	reserveMarkup, err := parseNonNegative(markup, "reserve_markup")
	if err != nil || reserveMarkup.LessThan(decimal.NewFromInt(1)) || reserveMarkup.GreaterThan(decimal.NewFromInt(2)) {
		return parsedSalesRule{}, fmt.Errorf("reserve_markup must be between 1 and 2")
	}
	parsed.reserveMarkup = reserveMarkup
	return parsed, nil
}

func parseNonNegative(raw, field string) (decimal.Decimal, error) {
	value, err := decimal.NewFromString(raw)
	if err != nil || value.IsNegative() {
		return decimal.Zero, fmt.Errorf("%s must be a non-negative decimal", field)
	}
	return value, nil
}

func parseRate(raw, field string, excludeOne bool) (decimal.Decimal, error) {
	value, err := parseNonNegative(raw, field)
	if err != nil {
		return decimal.Zero, err
	}
	if (excludeOne && !value.LessThan(decimal.NewFromInt(1))) || (!excludeOne && value.GreaterThan(decimal.NewFromInt(1))) {
		return decimal.Zero, fmt.Errorf("%s must be between 0 and 1", field)
	}
	return value, nil
}
