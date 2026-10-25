package compat

import (
	"context"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/internal/provider"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type ModelItem struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

type ModelsResponse struct {
	Object string      `json:"object"`
	Data   []ModelItem `json:"data"`
}

type Service struct {
	operations Operations
	cfg        config.Config
	taskSvc    *imagetaskservice.Service
	routing    modelhub.ModelRoutingSource
}

type ModelSelection struct {
	AbstractModel  string
	RouteModelCode string
}

func NewService(cfg config.Config) *Service {
	return &Service{cfg: cfg, taskSvc: imagetaskservice.NewService(cfg)}
}

func NewServiceWithProviders(cfg config.Config, providers map[string]provider.ImageProvider) *Service {
	return &Service{cfg: cfg, taskSvc: imagetaskservice.NewServiceWithProviders(cfg, providers)}
}

func NewServiceWithTaskService(cfg config.Config, taskSvc *imagetaskservice.Service) *Service {
	if taskSvc == nil {
		taskSvc = imagetaskservice.NewService(cfg)
	}
	return &Service{cfg: cfg, taskSvc: taskSvc}
}

func (s *Service) SetModelRoutingSource(source modelhub.ModelRoutingSource) {
	s.routing = source
}

func (s *Service) ResolveModel(ctx context.Context, model string) (ModelSelection, error) {
	resolved := s.resolveModelCode(model)
	if resolved == "" || s.routing == nil {
		return ModelSelection{AbstractModel: resolved}, nil
	}
	snapshot, err := s.routing.ModelRoutingConfig(ctx)
	if err != nil {
		return ModelSelection{}, errs.Internal("failed to load model routing config")
	}
	if len(snapshot.RouteModels) == 0 {
		return ModelSelection{AbstractModel: resolved}, nil
	}
	return ModelSelection{AbstractModel: resolved, RouteModelCode: resolved}, nil
}

func MapError(err error) *errs.Error {
	if err == nil {
		return nil
	}
	if appErr, ok := err.(*errs.Error); ok {
		return appErr
	}
	if upstream, ok := provider.AsUpstreamError(err); ok {
		switch upstream.Family {
		case provider.UpstreamErrorFamilyBadRequest:
			return errs.New(400, errs.CodeUpstreamBadRequest, firstNonEmpty(upstream.Message, "upstream request rejected"))
		case provider.UpstreamErrorFamilyBlocked:
			return errs.New(400, errs.CodeContentBlocked, firstNonEmpty(upstream.Message, "request blocked by upstream provider"))
		case provider.UpstreamErrorFamilyRateLimited:
			return errs.New(429, errs.CodeRateLimited, firstNonEmpty(upstream.Message, "upstream rate limited"))
		default:
			return errs.New(503, errs.CodeUpstreamUnavailable, firstNonEmpty(upstream.Message, "upstream provider unavailable"))
		}
	}
	return errs.Internal(err.Error())
}

func (s *Service) resolveModelCode(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	if value := strings.ToLower(strings.TrimSpace(s.cfg.Routing.OpenAICompatModelMap[model])); value != "" {
		return value
	}
	if _, ok := s.cfg.Billing.BaseResolutionPointsByModel[model]; ok {
		return model
	}
	return ""
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
