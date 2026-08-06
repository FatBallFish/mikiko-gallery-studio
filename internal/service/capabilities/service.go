package capabilities

import (
	"context"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	assetservice "github.com/fatballfish/pic-gallery/internal/service/assets"
)

type Item struct {
	AbstractModel          string   `json:"abstract_model"`
	TaskTypes              []string `json:"task_types"`
	BaseResolution         []string `json:"base_resolution"`
	AspectRatios           []string `json:"aspect_ratios"`
	MaxOutputImageCount    int      `json:"max_output_image_count"`
	MaxReferenceImageCount int      `json:"max_reference_image_count"`
}

type Response struct {
	Items                          []Item                       `json:"items,omitempty"`
	ModelGroups                    []modelhub.VisibleRouteModel `json:"model_groups,omitempty"`
	ReferenceImageMaxMB            int                          `json:"reference_image_max_mb,omitempty"`
	ReferenceImageMaxBytes         int64                        `json:"reference_image_max_bytes,omitempty"`
	ReferenceImageAllowedFormats   []string                     `json:"reference_image_allowed_formats,omitempty"`
	ReferenceImageAllowedMIMETypes []string                     `json:"reference_image_allowed_mime_types,omitempty"`
	UnavailableReason              *UnavailableReason           `json:"unavailable_reason,omitempty"`
}

type UnavailableReason struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Service struct {
	resolver              *modelhub.Resolver
	referenceImageMaxMB   int
	referenceImageMaxByte int64
	attachmentPolicy      *assetservice.AttachmentPolicyResolver
}

func NewService(cfg config.Config) *Service {
	defaults := config.ApplyAttachmentPolicyDefaults(cfg.AttachmentPolicy, cfg.GenerationLimits.ReferenceImageMaxMB)
	return NewServiceWithAttachmentPolicy(cfg, assetservice.NewAttachmentPolicyResolver(defaults, nil))
}

func NewServiceWithAttachmentPolicy(cfg config.Config, policy *assetservice.AttachmentPolicyResolver) *Service {
	maxMB := cfg.GenerationLimits.ReferenceImageMaxMB
	return &Service{
		resolver:              modelhub.NewResolver(cfg),
		referenceImageMaxMB:   maxMB,
		referenceImageMaxByte: int64(maxMB) * 1024 * 1024,
		attachmentPolicy:      policy,
	}
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
			BaseResolution:         item.BaseResolution,
			AspectRatios:           item.AspectRatios,
			MaxOutputImageCount:    item.MaxOutputImageCount,
			MaxReferenceImageCount: item.MaxReferenceImageCount,
		})
	}
	resp := s.withReferenceUploadLimits(context.Background(), Response{Items: items})
	return resp
}

func (s *Service) ListForGroups(ctx context.Context, groupCodes []string, taskMultipliers map[string]string) (Response, error) {
	items, err := s.resolver.ListVisibleRouteModels(ctx, groupCodes, taskMultipliers)
	if err != nil {
		return Response{}, err
	}
	resp := s.withReferenceUploadLimits(ctx, Response{ModelGroups: items})
	if len(items) == 0 {
		resp.UnavailableReason = &UnavailableReason{
			Code:    "NO_ROUTE_MODEL",
			Message: "平台模型配置中，暂不可生成。",
		}
	}
	return resp, nil
}

func (s *Service) withReferenceUploadLimits(ctx context.Context, resp Response) Response {
	if s.attachmentPolicy != nil {
		if policy, err := s.attachmentPolicy.Resolve(ctx); err == nil {
			resp.ReferenceImageMaxMB = policy.Image.MaxMB
			resp.ReferenceImageMaxBytes = policy.Image.MaxBytes
			resp.ReferenceImageAllowedFormats = append([]string(nil), policy.Image.AllowedFormats...)
			resp.ReferenceImageAllowedMIMETypes = append([]string(nil), policy.Image.AllowedMIMETypes...)
			return resp
		}
	}
	resp.ReferenceImageMaxMB = s.referenceImageMaxMB
	resp.ReferenceImageMaxBytes = s.referenceImageMaxByte
	return resp
}
