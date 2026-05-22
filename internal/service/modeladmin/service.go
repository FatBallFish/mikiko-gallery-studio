package modeladmin

import (
	"context"
	"errors"
	"strings"

	domainmodeladmin "github.com/fatballfish/pic-gallery/internal/domain/modeladmin"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type Service struct {
	store Store
}

func NewServiceWithStore(store Store) *Service {
	if store == nil {
		store = NewMemoryStore()
	}
	return &Service{store: store}
}

func (s *Service) ListProviders(ctx context.Context, req domainmodeladmin.ProviderListRequest) (domainmodeladmin.ProviderListPage, error) {
	req.Page, req.PageSize = normalizePage(req.Page, req.PageSize)
	req.ProviderType = strings.ToLower(strings.TrimSpace(req.ProviderType))
	return s.store.ListProviders(ctx, req)
}

func (s *Service) GetProvider(ctx context.Context, providerCode string) (domainmodeladmin.Provider, error) {
	providerCode = normalizeCode(providerCode)
	if providerCode == "" {
		return domainmodeladmin.Provider{}, errs.BadRequest("provider_code is required")
	}
	item, err := s.store.GetProvider(ctx, providerCode)
	return item, normalizeStoreError(err, "model provider not found")
}

func (s *Service) CreateProvider(ctx context.Context, req domainmodeladmin.ProviderWriteRequest) (domainmodeladmin.Provider, error) {
	normalized, err := normalizeProviderWrite(req, true)
	if err != nil {
		return domainmodeladmin.Provider{}, err
	}
	item, err := s.store.CreateProvider(ctx, normalized)
	return item, normalizeStoreError(err, "model provider already exists")
}

func (s *Service) UpdateProvider(ctx context.Context, providerCode string, req domainmodeladmin.ProviderWriteRequest) (domainmodeladmin.Provider, error) {
	providerCode = normalizeCode(providerCode)
	if providerCode == "" {
		return domainmodeladmin.Provider{}, errs.BadRequest("provider_code is required")
	}
	if strings.TrimSpace(req.ProviderCode) == "" {
		req.ProviderCode = providerCode
	}
	normalized, err := normalizeProviderWrite(req, false)
	if err != nil {
		return domainmodeladmin.Provider{}, err
	}
	item, err := s.store.UpdateProvider(ctx, providerCode, normalized)
	return item, normalizeStoreError(err, "model provider not found")
}

func (s *Service) DeleteProvider(ctx context.Context, providerCode string) error {
	providerCode = normalizeCode(providerCode)
	if providerCode == "" {
		return errs.BadRequest("provider_code is required")
	}
	return normalizeStoreError(s.store.DeleteProvider(ctx, providerCode), "model provider not found")
}

func (s *Service) ListRoutes(ctx context.Context, req domainmodeladmin.RouteListRequest) (domainmodeladmin.RouteListPage, error) {
	req.Page, req.PageSize = normalizePage(req.Page, req.PageSize)
	req.GroupCode = normalizeCode(req.GroupCode)
	req.TaskType = normalizeCode(req.TaskType)
	req.ProviderCode = normalizeCode(req.ProviderCode)
	return s.store.ListRoutes(ctx, req)
}

func (s *Service) GetRoute(ctx context.Context, routeID int64) (domainmodeladmin.Route, error) {
	if routeID <= 0 {
		return domainmodeladmin.Route{}, errs.BadRequest("invalid route_id")
	}
	item, err := s.store.GetRoute(ctx, routeID)
	return item, normalizeStoreError(err, "model route not found")
}

func (s *Service) CreateRoute(ctx context.Context, req domainmodeladmin.RouteWriteRequest) (domainmodeladmin.Route, error) {
	normalized, err := normalizeRouteWrite(req)
	if err != nil {
		return domainmodeladmin.Route{}, err
	}
	item, err := s.store.CreateRoute(ctx, normalized)
	return item, normalizeStoreError(err, "model provider not found")
}

func (s *Service) UpdateRoute(ctx context.Context, routeID int64, req domainmodeladmin.RouteWriteRequest) (domainmodeladmin.Route, error) {
	if routeID <= 0 {
		return domainmodeladmin.Route{}, errs.BadRequest("invalid route_id")
	}
	normalized, err := normalizeRouteWrite(req)
	if err != nil {
		return domainmodeladmin.Route{}, err
	}
	item, err := s.store.UpdateRoute(ctx, routeID, normalized)
	return item, normalizeStoreError(err, "model route not found")
}

func (s *Service) DeleteRoute(ctx context.Context, routeID int64) error {
	if routeID <= 0 {
		return errs.BadRequest("invalid route_id")
	}
	return normalizeStoreError(s.store.DeleteRoute(ctx, routeID), "model route not found")
}

func (s *Service) ModelRoutingConfig(ctx context.Context) (modelhub.ModelRoutingSnapshot, error) {
	return s.store.ModelRoutingConfig(ctx)
}

func normalizeProviderWrite(req domainmodeladmin.ProviderWriteRequest, create bool) (domainmodeladmin.ProviderWriteRequest, error) {
	req.ProviderCode = normalizeCode(req.ProviderCode)
	req.ProviderType = normalizeCode(req.ProviderType)
	req.HealthStatus = normalizeCode(req.HealthStatus)
	req.AuthConfigEncrypted = strings.TrimSpace(req.AuthConfigEncrypted)
	if req.ProviderCode == "" {
		return domainmodeladmin.ProviderWriteRequest{}, errs.BadRequest("provider_code is required")
	}
	if req.ProviderType == "" {
		return domainmodeladmin.ProviderWriteRequest{}, errs.BadRequest("provider_type is required")
	}
	if req.HealthStatus == "" {
		req.HealthStatus = "unknown"
	}
	if create && !req.Enabled {
		req.Enabled = false
	}
	return req, nil
}

func normalizeRouteWrite(req domainmodeladmin.RouteWriteRequest) (domainmodeladmin.RouteWriteRequest, error) {
	req.GroupCode = normalizeCode(req.GroupCode)
	req.TaskType = normalizeCode(req.TaskType)
	req.ProviderCode = normalizeCode(req.ProviderCode)
	if req.GroupCode == "" || req.TaskType == "" {
		return domainmodeladmin.RouteWriteRequest{}, errs.BadRequest("group_code and task_type are required")
	}
	if req.ProviderCode == "" && req.ProviderModelID <= 0 {
		return domainmodeladmin.RouteWriteRequest{}, errs.BadRequest("provider_code or provider_model_id is required")
	}
	if req.WeightPercent < 0 || req.WeightPercent > 100 {
		return domainmodeladmin.RouteWriteRequest{}, errs.BadRequest("weight_percent must be between 0 and 100")
	}
	if req.WeightPercent == 0 {
		req.WeightPercent = 100
	}
	return req, nil
}

func normalizePage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func normalizeCode(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeStoreError(err error, notFoundMessage string) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, repoerr.ErrNotFound):
		return errs.New(404, errs.CodeNotFound, notFoundMessage)
	case errors.Is(err, repoerr.ErrConflict):
		return errs.New(409, errs.CodeConflict, notFoundMessage)
	default:
		return err
	}
}
