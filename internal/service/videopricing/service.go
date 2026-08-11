package videopricing

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type Service struct {
	store Store
	now   func() time.Time
}

type QuoteResult struct {
	domainvideo.Quote
	PriceVersion string `json:"price_version"`
}

func NewService(store Store, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: store, now: now}
}

func (s *Service) Quote(ctx context.Context, strategyID int64, request domainvideo.Request) (QuoteResult, error) {
	if s == nil || s.store == nil {
		return QuoteResult{}, errs.Internal("video pricing is unavailable")
	}
	rule, err := s.store.GetVideoPriceRule(ctx, strategyID, request.TaskType, request.Resolution, request.AudioMode, s.now().UTC())
	if err != nil {
		return QuoteResult{}, err
	}
	quote, err := domainvideo.CalculateQuote(rule.SalesRule, domainvideo.QuoteRequest{
		DurationSeconds: request.DurationSeconds, ReferenceImageCount: len(request.Inputs), GenerateAudio: request.AudioMode == domainvideo.AudioModeGenerated, OutputCount: request.OutputCount,
	})
	if err != nil {
		return QuoteResult{}, errs.BadRequest(err.Error())
	}
	unit, unitErr := decimal.NewFromString(quote.UnitPoints)
	safety, safetyErr := decimal.NewFromString(rule.SafetyPoints)
	if unitErr != nil || safetyErr != nil || unit.LessThan(safety) {
		return QuoteResult{}, errs.New(409, errs.CodeConflict, "video price is below the configured safety floor")
	}
	return QuoteResult{Quote: quote, PriceVersion: fmt.Sprintf("%d:%d", rule.StrategyVersion, rule.RuleVersion)}, nil
}
