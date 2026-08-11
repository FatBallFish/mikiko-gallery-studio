package video

import "testing"

func TestCalculateQuoteUsesDecimalAddonsAndReserveMarkup(t *testing.T) {
	rule := SalesRule{
		FixedTaskPoints:            "1.00000",
		OutputSecondPoints:         "3.10000",
		ReferenceImagePoints:       "0.20000",
		GeneratedAudioFixedPoints:  "2.00000",
		GeneratedAudioSecondPoints: "0.50000",
		MinimumTaskPoints:          "0.00000",
		ReserveMarkup:              "1.00000",
	}
	quote, err := CalculateQuote(rule, QuoteRequest{
		DurationSeconds:     5,
		ReferenceImageCount: 2,
		GenerateAudio:       true,
		OutputCount:         2,
	})
	if err != nil {
		t.Fatalf("CalculateQuote() error = %v", err)
	}
	if quote.UnitPoints != "21.40000" || quote.EstimatedPoints != "42.80000" || quote.MaxReservedPoints != "42.80000" {
		t.Fatalf("quote = %#v", quote)
	}

	rule.ReserveMarkup = "1.15000"
	metered, err := CalculateQuote(rule, QuoteRequest{DurationSeconds: 5, OutputCount: 3})
	if err != nil {
		t.Fatalf("CalculateQuote(metered) error = %v", err)
	}
	if metered.EstimatedPoints != "49.50000" || metered.MaxReservedPoints != "56.92500" {
		t.Fatalf("metered quote = %#v", metered)
	}
}

func TestSafeMinimumPointsUsesWorstCandidateCostAndRoundsUp(t *testing.T) {
	points, err := SafeMinimumPoints(SafetyInput{
		CandidateCostsCNY:           []string{"0.80000", "1.00000", "0.95000"},
		ProviderCostBufferRate:      "0.10000",
		PlatformFixedCostCNY:        "0.15000",
		PlatformOutputSecondCostCNY: "0.02000",
		PlatformReferenceCostCNY:    "0.03000",
		DurationSeconds:             5,
		ReferenceImageCount:         2,
		MinimumNetPointIncomeCNY:    "0.25260",
		TargetGrossMarginRate:       "0.25000",
	})
	if err != nil {
		t.Fatalf("SafeMinimumPoints() error = %v", err)
	}
	if points != "7.50000" {
		t.Fatalf("SafeMinimumPoints() = %s, want 7.50000", points)
	}
}

func TestSettleQuoteChargesOnlySuccessfulItems(t *testing.T) {
	settlement, err := SettleQuote("156.97500", []ItemCharge{
		{Succeeded: true, Points: "45.50000"},
		{Succeeded: false, Points: "45.50000"},
		{Succeeded: true, Points: "45.50000"},
	})
	if err != nil {
		t.Fatalf("SettleQuote() error = %v", err)
	}
	if settlement.ActualPoints != "91.00000" || settlement.ReleasedPoints != "65.97500" {
		t.Fatalf("settlement = %#v", settlement)
	}
}
