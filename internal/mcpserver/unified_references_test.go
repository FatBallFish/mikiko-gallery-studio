package mcpserver

import (
	"context"
	"time"

	"github.com/fatballfish/pic-gallery/internal/service/videotask"
)

// Exercise real video preparation, media ownership and quotes while keeping
// video persistence/provider execution independent of this image fixture.
type unifiedVideoStore struct {
	videotask.Store
	record videotask.CreateRecord
}

func (s *unifiedVideoStore) FindByIdempotency(context.Context, int64, string) (videotask.Task, bool, error) {
	return videotask.Task{}, false, nil
}
func (s *unifiedVideoStore) Create(_ context.Context, record videotask.CreateRecord) (videotask.Task, bool, error) {
	s.record = record
	return record.Task, false, nil
}

type unifiedVideoQuotes struct{}

func (unifiedVideoQuotes) Estimate(context.Context, int64, videotask.EstimateRequest) (videotask.Estimate, error) {
	return videotask.Estimate{RouteModelCode: "fixture-video", CapabilityVersion: "cap-v1", ConfigVersion: "config-v1", PriceVersion: "price-v1", UnitPoints: "0.00000", EstimatedPoints: "0.00000", MaxReservedPoints: "0.00000", QuoteToken: "fixture-quote", ExpiresAt: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}, nil
}
func (q unifiedVideoQuotes) Verify(ctx context.Context, userID int64, req videotask.EstimateRequest, _ string) (videotask.Estimate, error) {
	return q.Estimate(ctx, userID, req)
}
