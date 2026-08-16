package videopricing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	adminvideo "github.com/fatballfish/pic-gallery/internal/service/adminvideo"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type Service struct {
	simulator RouteQuoteSimulator
	now       func() time.Time
}

type QuoteResult struct {
	SchemaVersion         int                                   `json:"schema_version"`
	QuoteMode             string                                `json:"quote_mode"`
	PriceVersion          string                                `json:"price_version"`
	UnitPoints            string                                `json:"unit_points"`
	EstimatedPoints       string                                `json:"estimated_points"`
	MaxReservedPoints     string                                `json:"max_reserved_points"`
	HighestAccountModelID int64                                 `json:"highest_account_model_id"`
	HighestCNY            string                                `json:"highest_cny"`
	CNYPerPoint           string                                `json:"cny_per_point"`
	ConversionVersion     string                                `json:"conversion_version"`
	MinimumTaskPoints     string                                `json:"minimum_task_points"`
	RoundingStepPoints    int                                   `json:"rounding_step_points"`
	CandidateQuotes       []adminvideo.QuoteSimulationCandidate `json:"candidate_quotes"`
}

func NewService(simulator RouteQuoteSimulator, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{simulator: simulator, now: now}
}

func (s *Service) Quote(ctx context.Context, routeModelID int64, request domainvideo.Request) (QuoteResult, error) {
	if s == nil || s.simulator == nil || routeModelID <= 0 {
		return QuoteResult{}, errs.Internal("video pricing is unavailable")
	}
	referenceImages := 0
	inputVideoSeconds := ""
	hasInputAudio := false
	for _, input := range request.Inputs {
		switch input.MediaType {
		case "image":
			referenceImages++
		case "audio":
			hasInputAudio = true
		}
	}
	simulation, err := s.simulator.SimulateRouteQuote(ctx, adminvideo.QuoteSimulationRequest{
		RouteModelID: routeModelID, TaskType: string(request.TaskType), Resolution: string(request.Resolution),
		AspectRatio: string(request.AspectRatio), AudioMode: string(request.AudioMode), DurationSeconds: request.DurationSeconds,
		OutputCount: request.OutputCount, ReferenceImageCount: referenceImages, InputVideoSeconds: inputVideoSeconds, HasInputAudio: hasInputAudio,
		Inputs: append([]domainvideo.Input(nil), request.Inputs...),
	})
	if err != nil {
		return QuoteResult{}, err
	}
	versionPayload, err := json.Marshal(struct {
		ConfigVersion     string                                `json:"config_version"`
		ConversionVersion string                                `json:"conversion_version"`
		Candidates        []adminvideo.QuoteSimulationCandidate `json:"candidates"`
	}{simulation.ConfigVersion, simulation.ConversionVersion, simulation.Candidates})
	if err != nil {
		return QuoteResult{}, errs.Internal("build video price version")
	}
	digest := sha256.Sum256(versionPayload)
	return QuoteResult{
		SchemaVersion: 2, QuoteMode: "route_candidate_max_fixed", PriceVersion: "native-" + hex.EncodeToString(digest[:8]),
		UnitPoints: simulation.UnitPoints, EstimatedPoints: simulation.TotalPoints, MaxReservedPoints: simulation.TotalPoints,
		HighestAccountModelID: simulation.HighestAccountModelID, HighestCNY: simulation.HighestCNY,
		CNYPerPoint: simulation.CNYPerPoint, ConversionVersion: simulation.ConversionVersion,
		MinimumTaskPoints: simulation.MinimumTaskPoints, RoundingStepPoints: simulation.RoundingStepPoints,
		CandidateQuotes: append([]adminvideo.QuoteSimulationCandidate(nil), simulation.Candidates...),
	}, nil
}
