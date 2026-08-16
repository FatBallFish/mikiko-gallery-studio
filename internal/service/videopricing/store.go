package videopricing

import (
	"context"

	adminvideo "github.com/fatballfish/pic-gallery/internal/service/adminvideo"
)

type RouteQuoteSimulator interface {
	SimulateRouteQuote(context.Context, adminvideo.QuoteSimulationRequest) (adminvideo.QuoteSimulationResult, error)
}
