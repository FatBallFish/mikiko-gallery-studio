package videopricing

import (
	"context"
	"time"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
)

type Rule struct {
	StrategyID      int64
	StrategyVersion int
	RuleVersion     int
	SafetyPoints    string
	SalesRule       domainvideo.SalesRule
}

type Store interface {
	GetVideoPriceRule(context.Context, int64, domainvideo.TaskType, domainvideo.Resolution, domainvideo.AudioMode, time.Time) (Rule, error)
}
