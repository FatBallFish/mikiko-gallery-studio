package capabilities

import (
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
	Items []Item `json:"items"`
}

type Service struct {
	resolver *modelhub.Resolver
}

func NewService(cfg config.Config) *Service {
	return &Service{resolver: modelhub.NewResolver(cfg)}
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
