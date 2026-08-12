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

type Group struct {
	RouteModelID      int64
	Code              string
	Name              string
	Description       string
	ConfigVersion     string
	PricingStrategyID int64
	MaxOutputCount    int
	TaskTypes         []domainvideo.TaskType
	Candidates        []Candidate
}

type Store interface {
	GetVideoGroup(context.Context, string) (Group, error)
	ListVideoGroups(context.Context) ([]Group, error)
}
