package videorouting

import (
	"context"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
)

type Candidate struct {
	RouteCandidateID  int64
	AccountModelID    int64
	ModelAccountID    int64
	ModelCode         string
	AdapterType       string
	CapabilityVersion string
	Capability        domainvideo.Capability
}

type PricingBinding struct {
	TaskType          domainvideo.TaskType
	Resolution        domainvideo.Resolution
	AspectRatio       domainvideo.AspectRatio
	AudioMode         domainvideo.AudioMode
	DurationSeconds   int
	PricingStrategyID int64
}

type Group struct {
	RouteModelID      int64
	Code              string
	Name              string
	Description       string
	ConfigVersion     string
	PricingStrategyID int64
	PricingBindings   []PricingBinding
	MaxOutputCount    int
	TaskTypes         []domainvideo.TaskType
	Candidates        []Candidate
}

func (group Group) PricingStrategyFor(request domainvideo.Request) int64 {
	for _, binding := range group.PricingBindings {
		if binding.PricingStrategyID <= 0 || binding.TaskType != request.TaskType || binding.Resolution != request.Resolution || binding.AudioMode != request.AudioMode {
			continue
		}
		if binding.AspectRatio != "" && binding.AspectRatio != request.AspectRatio {
			continue
		}
		if binding.DurationSeconds > 0 && binding.DurationSeconds != request.DurationSeconds {
			continue
		}
		return binding.PricingStrategyID
	}
	return group.PricingStrategyID
}

type Store interface {
	GetVideoGroup(context.Context, string) (Group, error)
	ListVideoGroups(context.Context) ([]Group, error)
}
