package modeladmin

import (
	"context"
	"sync"
	"time"

	domainmodeladmin "github.com/fatballfish/pic-gallery/internal/domain/modeladmin"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
)

type Store interface {
	ListProviders(ctx context.Context, req domainmodeladmin.ProviderListRequest) (domainmodeladmin.ProviderListPage, error)
	GetProvider(ctx context.Context, providerCode string) (domainmodeladmin.Provider, error)
	CreateProvider(ctx context.Context, req domainmodeladmin.ProviderWriteRequest) (domainmodeladmin.Provider, error)
	UpdateProvider(ctx context.Context, providerCode string, req domainmodeladmin.ProviderWriteRequest) (domainmodeladmin.Provider, error)
	DeleteProvider(ctx context.Context, providerCode string) error
	ListProviderModels(ctx context.Context, req domainmodeladmin.ProviderModelListRequest) (domainmodeladmin.ProviderModelListPage, error)
	GetProviderModel(ctx context.Context, providerModelID int64) (domainmodeladmin.ProviderModel, error)
	CreateProviderModel(ctx context.Context, req domainmodeladmin.ProviderModelWriteRequest) (domainmodeladmin.ProviderModel, error)
	UpdateProviderModel(ctx context.Context, providerModelID int64, req domainmodeladmin.ProviderModelWriteRequest) (domainmodeladmin.ProviderModel, error)
	DeleteProviderModel(ctx context.Context, providerModelID int64) error
	ListRoutes(ctx context.Context, req domainmodeladmin.RouteListRequest) (domainmodeladmin.RouteListPage, error)
	GetRoute(ctx context.Context, routeID int64) (domainmodeladmin.Route, error)
	CreateRoute(ctx context.Context, req domainmodeladmin.RouteWriteRequest) (domainmodeladmin.Route, error)
	UpdateRoute(ctx context.Context, routeID int64, req domainmodeladmin.RouteWriteRequest) (domainmodeladmin.Route, error)
	DeleteRoute(ctx context.Context, routeID int64) error
	ModelRoutingConfig(ctx context.Context) (modelhub.ModelRoutingSnapshot, error)
}

type MemoryStore struct {
	mu        sync.RWMutex
	nextID    int64
	providers map[string]domainmodeladmin.Provider
	models    map[int64]domainmodeladmin.ProviderModel
	routes    map[int64]domainmodeladmin.Route
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		nextID:    1,
		providers: map[string]domainmodeladmin.Provider{},
		models:    map[int64]domainmodeladmin.ProviderModel{},
		routes:    map[int64]domainmodeladmin.Route{},
	}
}

func (s *MemoryStore) ListProviders(_ context.Context, req domainmodeladmin.ProviderListRequest) (domainmodeladmin.ProviderListPage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	page, pageSize := normalizePage(req.Page, req.PageSize)
	items := make([]domainmodeladmin.Provider, 0, len(s.providers))
	for _, item := range s.providers {
		if req.ProviderType != "" && item.ProviderType != req.ProviderType {
			continue
		}
		if req.Enabled != nil && item.Enabled != *req.Enabled {
			continue
		}
		items = append(items, item)
	}
	total := len(items)
	items = slicePage(items, page, pageSize)
	return domainmodeladmin.ProviderListPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *MemoryStore) GetProvider(_ context.Context, providerCode string) (domainmodeladmin.Provider, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.providers[providerCode]
	if !ok {
		return domainmodeladmin.Provider{}, repoerr.ErrNotFound
	}
	return item, nil
}

func (s *MemoryStore) CreateProvider(_ context.Context, req domainmodeladmin.ProviderWriteRequest) (domainmodeladmin.Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.providers[req.ProviderCode]; ok {
		return domainmodeladmin.Provider{}, repoerr.ErrConflict
	}
	now := time.Now().UTC()
	item := domainmodeladmin.Provider{
		ID:                  s.nextID,
		ProviderCode:        req.ProviderCode,
		ProviderType:        req.ProviderType,
		AuthConfigEncrypted: req.AuthConfigEncrypted,
		HealthStatus:        req.HealthStatus,
		Enabled:             req.Enabled,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	s.nextID++
	s.providers[item.ProviderCode] = item
	s.createDefaultProviderModelLocked(item)
	return item, nil
}

func (s *MemoryStore) UpdateProvider(_ context.Context, providerCode string, req domainmodeladmin.ProviderWriteRequest) (domainmodeladmin.Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.providers[providerCode]
	if !ok {
		return domainmodeladmin.Provider{}, repoerr.ErrNotFound
	}
	item.ProviderCode = req.ProviderCode
	item.ProviderType = req.ProviderType
	item.AuthConfigEncrypted = req.AuthConfigEncrypted
	item.HealthStatus = req.HealthStatus
	item.Enabled = req.Enabled
	item.UpdatedAt = time.Now().UTC()
	delete(s.providers, providerCode)
	s.providers[item.ProviderCode] = item
	for id, model := range s.models {
		if model.ProviderID == item.ID {
			model.ProviderCode = item.ProviderCode
			model.UpdatedAt = item.UpdatedAt
			s.models[id] = model
		}
	}
	for id, route := range s.routes {
		model, ok := s.models[route.ProviderModelID]
		if !ok {
			continue
		}
		route.ProviderCode = model.ProviderCode
		route.UpdatedAt = item.UpdatedAt
		s.routes[id] = route
	}
	return item, nil
}

func (s *MemoryStore) DeleteProvider(_ context.Context, providerCode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.providers[providerCode]; !ok {
		return repoerr.ErrNotFound
	}
	for modelID, model := range s.models {
		if model.ProviderID != s.providers[providerCode].ID {
			continue
		}
		for _, route := range s.routes {
			if route.ProviderModelID == modelID {
				return repoerr.ErrConflict
			}
		}
		delete(s.models, modelID)
	}
	delete(s.providers, providerCode)
	return nil
}

func (s *MemoryStore) ListProviderModels(_ context.Context, req domainmodeladmin.ProviderModelListRequest) (domainmodeladmin.ProviderModelListPage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	page, pageSize := normalizePage(req.Page, req.PageSize)
	items := make([]domainmodeladmin.ProviderModel, 0, len(s.models))
	for _, item := range s.models {
		if req.ProviderCode != "" && item.ProviderCode != req.ProviderCode {
			continue
		}
		if req.ModelCode != "" && item.ModelCode != req.ModelCode {
			continue
		}
		if req.Enabled != nil && item.Enabled != *req.Enabled {
			continue
		}
		items = append(items, item)
	}
	total := len(items)
	items = slicePage(items, page, pageSize)
	return domainmodeladmin.ProviderModelListPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *MemoryStore) GetProviderModel(_ context.Context, providerModelID int64) (domainmodeladmin.ProviderModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.models[providerModelID]
	if !ok {
		return domainmodeladmin.ProviderModel{}, repoerr.ErrNotFound
	}
	return item, nil
}

func (s *MemoryStore) CreateProviderModel(_ context.Context, req domainmodeladmin.ProviderModelWriteRequest) (domainmodeladmin.ProviderModel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	provider, ok := s.providers[req.ProviderCode]
	if !ok {
		return domainmodeladmin.ProviderModel{}, repoerr.ErrNotFound
	}
	for _, item := range s.models {
		if item.ProviderID == provider.ID && item.ModelCode == req.ModelCode {
			return domainmodeladmin.ProviderModel{}, repoerr.ErrConflict
		}
	}
	now := time.Now().UTC()
	item := domainmodeladmin.ProviderModel{
		ID:                     s.nextID,
		ProviderID:             provider.ID,
		ProviderCode:           provider.ProviderCode,
		ModelCode:              req.ModelCode,
		CompatMode:             req.CompatMode,
		SupportsImageInput:     req.SupportsImageInput,
		SupportsMask:           req.SupportsMask,
		SupportedQualities:     append([]string(nil), req.SupportedQualities...),
		SupportedRatios:        append([]string(nil), req.SupportedRatios...),
		MaxImageCount:          req.MaxImageCount,
		MaxReferenceImageCount: req.MaxReferenceImageCount,
		TimeoutMS:              req.TimeoutMS,
		InputCost:              req.InputCost,
		OutputCost:             req.OutputCost,
		Currency:               req.Currency,
		HealthStatus:           req.HealthStatus,
		LastHealthCheckedAt:    req.LastHealthCheckedAt,
		Enabled:                req.Enabled,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	s.nextID++
	s.models[item.ID] = item
	return item, nil
}

func (s *MemoryStore) UpdateProviderModel(_ context.Context, providerModelID int64, req domainmodeladmin.ProviderModelWriteRequest) (domainmodeladmin.ProviderModel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.models[providerModelID]
	if !ok {
		return domainmodeladmin.ProviderModel{}, repoerr.ErrNotFound
	}
	provider, ok := s.providers[req.ProviderCode]
	if !ok {
		return domainmodeladmin.ProviderModel{}, repoerr.ErrNotFound
	}
	item.ProviderID = provider.ID
	item.ProviderCode = provider.ProviderCode
	item.ModelCode = req.ModelCode
	item.CompatMode = req.CompatMode
	item.SupportsImageInput = req.SupportsImageInput
	item.SupportsMask = req.SupportsMask
	item.SupportedQualities = append([]string(nil), req.SupportedQualities...)
	item.SupportedRatios = append([]string(nil), req.SupportedRatios...)
	item.MaxImageCount = req.MaxImageCount
	item.MaxReferenceImageCount = req.MaxReferenceImageCount
	item.TimeoutMS = req.TimeoutMS
	item.InputCost = req.InputCost
	item.OutputCost = req.OutputCost
	item.Currency = req.Currency
	item.HealthStatus = req.HealthStatus
	item.LastHealthCheckedAt = req.LastHealthCheckedAt
	item.Enabled = req.Enabled
	item.UpdatedAt = time.Now().UTC()
	s.models[item.ID] = item
	for id, route := range s.routes {
		if route.ProviderModelID == item.ID {
			route.ProviderCode = item.ProviderCode
			route.UpdatedAt = item.UpdatedAt
			s.routes[id] = route
		}
	}
	return item, nil
}

func (s *MemoryStore) DeleteProviderModel(_ context.Context, providerModelID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.models[providerModelID]; !ok {
		return repoerr.ErrNotFound
	}
	for _, route := range s.routes {
		if route.ProviderModelID == providerModelID {
			return repoerr.ErrConflict
		}
	}
	delete(s.models, providerModelID)
	return nil
}

func (s *MemoryStore) ListRoutes(_ context.Context, req domainmodeladmin.RouteListRequest) (domainmodeladmin.RouteListPage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	page, pageSize := normalizePage(req.Page, req.PageSize)
	items := make([]domainmodeladmin.Route, 0, len(s.routes))
	for _, item := range s.routes {
		if req.GroupCode != "" && item.GroupCode != req.GroupCode {
			continue
		}
		if req.TaskType != "" && item.TaskType != req.TaskType {
			continue
		}
		if req.ProviderCode != "" && item.ProviderCode != req.ProviderCode {
			continue
		}
		if req.Enabled != nil && item.Enabled != *req.Enabled {
			continue
		}
		items = append(items, item)
	}
	total := len(items)
	items = slicePage(items, page, pageSize)
	return domainmodeladmin.RouteListPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *MemoryStore) GetRoute(_ context.Context, routeID int64) (domainmodeladmin.Route, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.routes[routeID]
	if !ok {
		return domainmodeladmin.Route{}, repoerr.ErrNotFound
	}
	return item, nil
}

func (s *MemoryStore) CreateRoute(_ context.Context, req domainmodeladmin.RouteWriteRequest) (domainmodeladmin.Route, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	providerModel, ok := s.providerModelByRouteRequest(req)
	if !ok {
		return domainmodeladmin.Route{}, repoerr.ErrNotFound
	}
	now := time.Now().UTC()
	item := domainmodeladmin.Route{
		ID:              s.nextID,
		GroupCode:       req.GroupCode,
		TaskType:        req.TaskType,
		ProviderModelID: providerModel.ID,
		ProviderCode:    providerModel.ProviderCode,
		Priority:        req.Priority,
		WeightPercent:   req.WeightPercent,
		FallbackOrder:   req.FallbackOrder,
		Enabled:         req.Enabled,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	s.nextID++
	s.routes[item.ID] = item
	return item, nil
}

func (s *MemoryStore) UpdateRoute(_ context.Context, routeID int64, req domainmodeladmin.RouteWriteRequest) (domainmodeladmin.Route, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.routes[routeID]
	if !ok {
		return domainmodeladmin.Route{}, repoerr.ErrNotFound
	}
	providerModel, ok := s.providerModelByRouteRequest(req)
	if !ok {
		return domainmodeladmin.Route{}, repoerr.ErrNotFound
	}
	item.GroupCode = req.GroupCode
	item.TaskType = req.TaskType
	item.ProviderModelID = providerModel.ID
	item.ProviderCode = providerModel.ProviderCode
	item.Priority = req.Priority
	item.WeightPercent = req.WeightPercent
	item.FallbackOrder = req.FallbackOrder
	item.Enabled = req.Enabled
	item.UpdatedAt = time.Now().UTC()
	s.routes[item.ID] = item
	return item, nil
}

func (s *MemoryStore) DeleteRoute(_ context.Context, routeID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.routes[routeID]; !ok {
		return repoerr.ErrNotFound
	}
	delete(s.routes, routeID)
	return nil
}

func (s *MemoryStore) ModelRoutingConfig(_ context.Context) (modelhub.ModelRoutingSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot := modelhub.ModelRoutingSnapshot{
		Providers: make([]modelhub.ModelProviderConfig, 0, len(s.providers)),
		Routes:    make([]modelhub.ModelRouteConfig, 0, len(s.routes)),
	}
	for _, item := range s.providers {
		snapshot.Providers = append(snapshot.Providers, modelhub.ModelProviderConfig{
			ID:           item.ID,
			ProviderCode: item.ProviderCode,
			ProviderType: item.ProviderType,
			Enabled:      item.Enabled,
		})
	}
	for _, item := range s.routes {
		providerCode := item.ProviderCode
		if providerModel, ok := s.models[item.ProviderModelID]; ok {
			providerCode = providerModel.ProviderCode
		}
		snapshot.Routes = append(snapshot.Routes, modelhub.ModelRouteConfig{
			ID:              item.ID,
			GroupCode:       item.GroupCode,
			TaskType:        item.TaskType,
			ProviderModelID: item.ProviderModelID,
			ProviderCode:    providerCode,
			Priority:        item.Priority,
			WeightPercent:   item.WeightPercent,
			FallbackOrder:   item.FallbackOrder,
			Enabled:         item.Enabled,
		})
	}
	return snapshot, nil
}

func (s *MemoryStore) providerModelByRouteRequest(req domainmodeladmin.RouteWriteRequest) (domainmodeladmin.ProviderModel, bool) {
	if req.ProviderModelID > 0 {
		item, ok := s.models[req.ProviderModelID]
		return item, ok
	}
	if req.ProviderCode != "" {
		for _, item := range s.models {
			if item.ProviderCode == req.ProviderCode {
				return item, true
			}
		}
	}
	return domainmodeladmin.ProviderModel{}, false
}

func (s *MemoryStore) createDefaultProviderModelLocked(provider domainmodeladmin.Provider) {
	now := time.Now().UTC()
	item := domainmodeladmin.ProviderModel{
		ID:                     s.nextID,
		ProviderID:             provider.ID,
		ProviderCode:           provider.ProviderCode,
		ModelCode:              provider.ProviderCode + "-default",
		MaxImageCount:          1,
		MaxReferenceImageCount: 0,
		TimeoutMS:              60000,
		InputCost:              "0",
		OutputCost:             "0",
		Currency:               "CNY",
		HealthStatus:           provider.HealthStatus,
		Enabled:                provider.Enabled,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	s.nextID++
	s.models[item.ID] = item
}

func slicePage[T any](items []T, page, pageSize int) []T {
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []T{}
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}
