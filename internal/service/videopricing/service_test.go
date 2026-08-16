package videopricing

import (
	"context"
	"testing"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	adminvideo "github.com/fatballfish/pic-gallery/internal/service/adminvideo"
)

type quoteSimulatorStub struct {
	result adminvideo.QuoteSimulationResult
}

func (s quoteSimulatorStub) SimulateRouteQuote(context.Context, adminvideo.QuoteSimulationRequest) (adminvideo.QuoteSimulationResult, error) {
	return s.result, nil
}

func TestQuoteLocksHighestMixedCandidateAndFixedPoints(t *testing.T) {
	service := NewService(quoteSimulatorStub{result: adminvideo.QuoteSimulationResult{
		RouteModelID: 9, ConfigVersion: "route-v3", HighestAccountModelID: 101,
		HighestCNY: "4.96800", CNYPerPoint: "0.01000", ConversionVersion: "billing-v4",
		MinimumTaskPoints: "10.00000", RoundingStepPoints: 1, UnitPoints: "497.00000", TotalPoints: "994.00000",
		Candidates: []adminvideo.QuoteSimulationCandidate{
			{AccountModelID: 101, ProviderCode: "seedance", Eligible: true, EstimatedCNY: "4.96800"},
			{AccountModelID: 202, ProviderCode: "minimax", Eligible: true, MappedResolution: "768p", EstimatedCNY: "4.00000"},
		},
	}}, nil)
	quoted, err := service.Quote(t.Context(), 9, domainvideo.Request{
		TaskType: domainvideo.TaskTypeTextToVideo, DurationSeconds: 5, Resolution: domainvideo.Resolution720P,
		AspectRatio: domainvideo.AspectRatio16x9, AudioMode: domainvideo.AudioModeSilent, OutputCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if quoted.UnitPoints != "497.00000" || quoted.EstimatedPoints != "994.00000" || quoted.MaxReservedPoints != "994.00000" {
		t.Fatalf("fixed quote = %#v", quoted)
	}
	if quoted.SchemaVersion != 2 || quoted.HighestAccountModelID != 101 || len(quoted.CandidateQuotes) != 2 {
		t.Fatalf("native quote metadata = %#v", quoted)
	}
}
