package compat

import (
	"context"
	"sort"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type GenerateRequest struct {
	TaskID              string
	UserID              int64
	APIKeyID            int64
	SourceChannel       string
	UserGroupCode       string
	UserGroupMultiplier string
	Model               string
	Prompt              string
	Size                string
	N                   int
	Quality             string
	ResponseFormat      string
	User                string
}

type EditRequest struct {
	TaskID              string
	UserID              int64
	APIKeyID            int64
	SourceChannel       string
	UserGroupCode       string
	UserGroupMultiplier string
	Model               string
	Prompt              string
	Size                string
	N                   int
	Quality             string
	ResponseFormat      string
	Images              []provider.ImageInput
	Mask                *provider.ImageInput
	User                string
}

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
	cfg     config.Config
	taskSvc *imagetaskservice.Service
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

func (s *Service) Generate(ctx context.Context, req GenerateRequest) (provider.ImageResponse, error) {
	if strings.TrimSpace(req.Model) == "" || strings.TrimSpace(req.Prompt) == "" {
		return provider.ImageResponse{}, errs.BadRequest("model and prompt are required")
	}
	result, err := s.taskSvc.Execute(ctx, domainimagetask.ExecuteRequest{
		TaskID:              req.TaskID,
		UserID:              req.UserID,
		APIKeyID:            req.APIKeyID,
		SourceChannel:       req.SourceChannel,
		UserGroupCode:       req.UserGroupCode,
		UserGroupMultiplier: req.UserGroupMultiplier,
		AbstractModel:       s.resolveAbstractModel(req.Model),
		TaskType:            string(provider.TaskTypeTextToImage),
		Prompt:              req.Prompt,
		RequestedSize:       defaultString(req.Size, "auto"),
		RequestedQuality:    normalizeCompatQuality(req.Quality),
		OutputImageCount:    req.N,
		ResponseFormat:      string(normalizeResponseFormat(req.ResponseFormat)),
		User:                req.User,
		PreferredProviders:  compatProviderPreference(s.cfg),
	})
	if err != nil {
		return provider.ImageResponse{}, err
	}
	return result.Response, nil
}

func (s *Service) Edit(ctx context.Context, req EditRequest) (provider.ImageResponse, error) {
	if strings.TrimSpace(req.Model) == "" || strings.TrimSpace(req.Prompt) == "" {
		return provider.ImageResponse{}, errs.BadRequest("model and prompt are required")
	}
	if len(req.Images) == 0 {
		return provider.ImageResponse{}, errs.New(400, errs.CodeImageReferenceRequired, "image is required")
	}
	result, err := s.taskSvc.Execute(ctx, domainimagetask.ExecuteRequest{
		TaskID:              req.TaskID,
		UserID:              req.UserID,
		APIKeyID:            req.APIKeyID,
		SourceChannel:       req.SourceChannel,
		UserGroupCode:       req.UserGroupCode,
		UserGroupMultiplier: req.UserGroupMultiplier,
		AbstractModel:       s.resolveAbstractModel(req.Model),
		TaskType:            string(provider.TaskTypeImageEdit),
		Prompt:              req.Prompt,
		RequestedSize:       defaultString(req.Size, "auto"),
		RequestedQuality:    normalizeCompatQuality(req.Quality),
		OutputImageCount:    req.N,
		ResponseFormat:      string(normalizeResponseFormat(req.ResponseFormat)),
		ReferenceImages:     append([]provider.ImageInput(nil), req.Images...),
		Mask:                req.Mask,
		User:                req.User,
		PreferredProviders:  compatProviderPreference(s.cfg),
	})
	if err != nil {
		return provider.ImageResponse{}, err
	}
	return result.Response, nil
}

func (s *Service) Models() ModelsResponse {
	ids := make([]string, 0, len(s.cfg.Routing.OpenAICompatModelMap))
	for id := range s.cfg.Routing.OpenAICompatModelMap {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	items := make([]ModelItem, 0, len(ids))
	for _, id := range ids {
		items = append(items, ModelItem{
			ID:      id,
			Object:  "model",
			OwnedBy: defaultString(s.cfg.App.Name, "pic-gallery"),
		})
	}
	return ModelsResponse{Object: "list", Data: items}
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

func (s *Service) resolveAbstractModel(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	if value := strings.ToLower(strings.TrimSpace(s.cfg.Routing.OpenAICompatModelMap[model])); value != "" {
		return value
	}
	if _, ok := s.cfg.Billing.QualityPointsByModel[model]; ok {
		return model
	}
	return ""
}

func normalizeCompatQuality(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low":
		return "1k"
	case "medium":
		return "2k"
	case "high":
		return "4k"
	case "1k", "2k", "4k":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "auto"
	}
}

func normalizeResponseFormat(value string) provider.ResponseFormat {
	if strings.EqualFold(strings.TrimSpace(value), string(provider.ResponseFormatB64JSON)) {
		return provider.ResponseFormatB64JSON
	}
	return provider.ResponseFormatURL
}

func compatProviderPreference(cfg config.Config) []string {
	preferred := []string{}
	if cfg.Routing.DefaultProvider != "" {
		preferred = append(preferred, cfg.Routing.DefaultProvider)
	}
	preferred = append(preferred, cfg.Routing.FallbackProviders...)
	return preferred
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
