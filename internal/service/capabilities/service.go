package capabilities

import (
	"context"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
)

type Item struct {
	AbstractModel          string   `json:"abstract_model"`
	TaskTypes              []string `json:"task_types"`
	Qualities              []string `json:"qualities"`
	AspectRatios           []string `json:"aspect_ratios"`
	MaxOutputImageCount    int      `json:"max_output_image_count"`
	MaxReferenceImageCount int      `json:"max_reference_image_count"`
}

type Response struct {
	Items       []Item                       `json:"items,omitempty"`
	ModelGroups []modelhub.VisibleRouteModel `json:"model_groups,omitempty"`
}

type Service struct {
	resolver *modelhub.Resolver
}

func NewService(cfg config.Config) *Service {
	return &Service{resolver: modelhub.NewResolver(cfg)}
}

func (s *Service) SetModelRoutingSource(source modelhub.ModelRoutingSource) {
	s.resolver.SetModelRoutingSource(source)
}

func (s *Service) List() Response {
	resolved := s.resolver.ListCapabilities()
	items := make([]Item, 0, len(resolved))
	for _, item := range resolved {
		items = append(items, Item{
			AbstractModel:          item.AbstractModel,
			TaskTypes:              item.TaskTypes,
			Qualities:              item.Qualities,
			AspectRatios:           item.AspectRatios,
			MaxOutputImageCount:    item.MaxOutputImageCount,
			MaxReferenceImageCount: item.MaxReferenceImageCount,
		})
	}
	return Response{Items: items}
}

func (s *Service) ListForGroups(ctx context.Context, groupCodes []string, taskMultipliers map[string]string) (Response, error) {
	items, err := s.resolver.ListVisibleRouteModels(ctx, groupCodes, taskMultipliers)
	if err != nil {
		return Response{}, err
	}
	return Response{ModelGroups: items}, nil
}
