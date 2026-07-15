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
	ListModelAccounts(ctx context.Context, req domainmodeladmin.ModelAccountListRequest) (domainmodeladmin.ModelAccountListPage, error)
	GetModelAccount(ctx context.Context, accountID int64) (domainmodeladmin.ModelAccount, error)
	CreateModelAccount(ctx context.Context, req domainmodeladmin.ModelAccountWriteRequest) (domainmodeladmin.ModelAccount, error)
	UpdateModelAccount(ctx context.Context, accountID int64, req domainmodeladmin.ModelAccountWriteRequest) (domainmodeladmin.ModelAccount, error)
	DeleteModelAccount(ctx context.Context, accountID int64) error
	ListModelAccountModels(ctx context.Context, req domainmodeladmin.ModelAccountModelListRequest) (domainmodeladmin.ModelAccountModelListPage, error)
	GetModelAccountModel(ctx context.Context, accountModelID int64) (domainmodeladmin.ModelAccountModel, error)
	CreateModelAccountModel(ctx context.Context, req domainmodeladmin.ModelAccountModelWriteRequest) (domainmodeladmin.ModelAccountModel, error)
	UpdateModelAccountModel(ctx context.Context, accountModelID int64, req domainmodeladmin.ModelAccountModelWriteRequest) (domainmodeladmin.ModelAccountModel, error)
	DeleteModelAccountModel(ctx context.Context, accountModelID int64) error
	ListRouteModels(ctx context.Context, req domainmodeladmin.RouteModelListRequest) (domainmodeladmin.RouteModelListPage, error)
	GetRouteModel(ctx context.Context, routeModelID int64) (domainmodeladmin.RouteModel, error)
	CreateRouteModel(ctx context.Context, req domainmodeladmin.RouteModelWriteRequest) (domainmodeladmin.RouteModel, error)
	UpdateRouteModel(ctx context.Context, routeModelID int64, req domainmodeladmin.RouteModelWriteRequest) (domainmodeladmin.RouteModel, error)
	DeleteRouteModel(ctx context.Context, routeModelID int64) error
	ListRouteModelCandidates(ctx context.Context, routeModelID int64) ([]domainmodeladmin.RouteModelCandidate, error)
	CreateRouteModelCandidate(ctx context.Context, req domainmodeladmin.RouteModelCandidateWriteRequest) (domainmodeladmin.RouteModelCandidate, error)
	UpdateRouteModelCandidate(ctx context.Context, candidateID int64, req domainmodeladmin.RouteModelCandidateWriteRequest) (domainmodeladmin.RouteModelCandidate, error)
	DeleteRouteModelCandidate(ctx context.Context, candidateID int64) error
	ListRouteModelPrices(ctx context.Context, req domainmodeladmin.RouteModelPriceListRequest) (domainmodeladmin.RouteModelPriceListPage, error)
	CreateRouteModelPrice(ctx context.Context, req domainmodeladmin.RouteModelPriceWriteRequest) (domainmodeladmin.RouteModelPrice, error)
	UpdateRouteModelPrice(ctx context.Context, priceID int64, req domainmodeladmin.RouteModelPriceWriteRequest) (domainmodeladmin.RouteModelPrice, error)
	DeleteRouteModelPrice(ctx context.Context, priceID int64) error
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
	mu            sync.RWMutex
	nextID        int64
	providers     map[string]domainmodeladmin.Provider
	models        map[int64]domainmodeladmin.ProviderModel
	routes        map[int64]domainmodeladmin.Route
	accounts      map[int64]domainmodeladmin.ModelAccount
	accountModels map[int64]domainmodeladmin.ModelAccountModel
	routeModels   map[int64]domainmodeladmin.RouteModel
	candidates    map[int64]domainmodeladmin.RouteModelCandidate
	prices        map[int64]domainmodeladmin.RouteModelPrice
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		nextID:        1,
		providers:     map[string]domainmodeladmin.Provider{},
		models:        map[int64]domainmodeladmin.ProviderModel{},
		routes:        map[int64]domainmodeladmin.Route{},
		accounts:      map[int64]domainmodeladmin.ModelAccount{},
		accountModels: map[int64]domainmodeladmin.ModelAccountModel{},
		routeModels:   map[int64]domainmodeladmin.RouteModel{},
		candidates:    map[int64]domainmodeladmin.RouteModelCandidate{},
		prices:        map[int64]domainmodeladmin.RouteModelPrice{},
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

func (s *MemoryStore) ListModelAccounts(_ context.Context, req domainmodeladmin.ModelAccountListRequest) (domainmodeladmin.ModelAccountListPage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	page, pageSize := normalizePage(req.Page, req.PageSize)
	items := make([]domainmodeladmin.ModelAccount, 0, len(s.accounts))
	for _, item := range s.accounts {
		if req.AdapterType != "" && item.AdapterType != req.AdapterType {
			continue
		}
		if req.AuthType != "" && item.AuthType != req.AuthType {
			continue
		}
		if req.Status != "" && item.Status != req.Status {
			continue
		}
		items = append(items, item)
	}
	total := len(items)
	return domainmodeladmin.ModelAccountListPage{Items: slicePage(items, page, pageSize), Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *MemoryStore) GetModelAccount(_ context.Context, accountID int64) (domainmodeladmin.ModelAccount, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.accounts[accountID]
	if !ok {
		return domainmodeladmin.ModelAccount{}, repoerr.ErrNotFound
	}
	return item, nil
}

func (s *MemoryStore) CreateModelAccount(_ context.Context, req domainmodeladmin.ModelAccountWriteRequest) (domainmodeladmin.ModelAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	item := domainmodeladmin.ModelAccount{
		ID: s.nextID, Name: req.Name, AdapterType: req.AdapterType, AuthType: req.AuthType, BaseURL: req.BaseURL,
		CredentialsEncrypted: req.Credentials, CredentialsStatus: domainmodeladmin.CredentialsStatus{HasAPIKey: req.Credentials["api_key"] != ""},
		Status: req.Status, Priority: req.Priority, Weight: req.Weight, ConcurrencyLimit: req.ConcurrencyLimit, TimeoutMS: req.TimeoutMS, Extra: req.Extra,
		CreatedAt: now, UpdatedAt: now,
	}
	s.nextID++
	s.accounts[item.ID] = item
	return item, nil
}

func (s *MemoryStore) UpdateModelAccount(_ context.Context, accountID int64, req domainmodeladmin.ModelAccountWriteRequest) (domainmodeladmin.ModelAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.accounts[accountID]
	if !ok {
		return domainmodeladmin.ModelAccount{}, repoerr.ErrNotFound
	}
	item.Name, item.AdapterType, item.AuthType, item.BaseURL = req.Name, req.AdapterType, req.AuthType, req.BaseURL
	if req.Credentials != nil {
		item.CredentialsEncrypted = req.Credentials
		item.CredentialsStatus = domainmodeladmin.CredentialsStatus{HasAPIKey: req.Credentials["api_key"] != ""}
	}
	item.Status, item.Priority, item.Weight = req.Status, req.Priority, req.Weight
	item.ConcurrencyLimit, item.TimeoutMS, item.Extra = req.ConcurrencyLimit, req.TimeoutMS, req.Extra
	item.UpdatedAt = time.Now().UTC()
	s.accounts[accountID] = item
	return item, nil
}

func (s *MemoryStore) DeleteModelAccount(_ context.Context, accountID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.accounts[accountID]; !ok {
		return repoerr.ErrNotFound
	}
	for _, model := range s.accountModels {
		if model.AccountID == accountID {
			return repoerr.ErrConflict
		}
	}
	delete(s.accounts, accountID)
	return nil
}

func (s *MemoryStore) ListModelAccountModels(_ context.Context, req domainmodeladmin.ModelAccountModelListRequest) (domainmodeladmin.ModelAccountModelListPage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	page, pageSize := normalizePage(req.Page, req.PageSize)
	items := make([]domainmodeladmin.ModelAccountModel, 0, len(s.accountModels))
	for _, item := range s.accountModels {
		if req.AccountID > 0 && item.AccountID != req.AccountID {
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
	return domainmodeladmin.ModelAccountModelListPage{Items: slicePage(items, page, pageSize), Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *MemoryStore) GetModelAccountModel(_ context.Context, accountModelID int64) (domainmodeladmin.ModelAccountModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.accountModels[accountModelID]
	if !ok {
		return domainmodeladmin.ModelAccountModel{}, repoerr.ErrNotFound
	}
	return item, nil
}

func (s *MemoryStore) CreateModelAccountModel(_ context.Context, req domainmodeladmin.ModelAccountModelWriteRequest) (domainmodeladmin.ModelAccountModel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	account, ok := s.accounts[req.AccountID]
	if !ok {
		return domainmodeladmin.ModelAccountModel{}, repoerr.ErrNotFound
	}
	now := time.Now().UTC()
	item := domainmodeladmin.ModelAccountModel{ID: s.nextID, AccountID: req.AccountID, AccountName: account.Name, ModelCode: req.ModelCode, DisplayName: req.DisplayName, TaskTypes: append([]string(nil), req.TaskTypes...), Qualities: append([]string(nil), req.Qualities...), CostPerImage: req.CostPerImage, Currency: req.Currency, Enabled: req.Enabled, Extra: req.Extra, CreatedAt: now, UpdatedAt: now}
	s.nextID++
	s.accountModels[item.ID] = item
	return item, nil
}

func (s *MemoryStore) UpdateModelAccountModel(_ context.Context, accountModelID int64, req domainmodeladmin.ModelAccountModelWriteRequest) (domainmodeladmin.ModelAccountModel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.accountModels[accountModelID]
	if !ok {
		return domainmodeladmin.ModelAccountModel{}, repoerr.ErrNotFound
	}
	account, ok := s.accounts[req.AccountID]
	if !ok {
		return domainmodeladmin.ModelAccountModel{}, repoerr.ErrNotFound
	}
	item.AccountID, item.AccountName, item.ModelCode, item.DisplayName = req.AccountID, account.Name, req.ModelCode, req.DisplayName
	item.TaskTypes, item.Qualities = append([]string(nil), req.TaskTypes...), append([]string(nil), req.Qualities...)
	item.CostPerImage, item.Currency, item.Enabled, item.Extra = req.CostPerImage, req.Currency, req.Enabled, req.Extra
	item.UpdatedAt = time.Now().UTC()
	s.accountModels[accountModelID] = item
	return item, nil
}

func (s *MemoryStore) DeleteModelAccountModel(_ context.Context, accountModelID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.accountModels[accountModelID]; !ok {
		return repoerr.ErrNotFound
	}
	for _, candidate := range s.candidates {
		if candidate.AccountModelID == accountModelID {
			return repoerr.ErrConflict
		}
	}
	delete(s.accountModels, accountModelID)
	return nil
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

func (s *MemoryStore) ListRouteModels(_ context.Context, req domainmodeladmin.RouteModelListRequest) (domainmodeladmin.RouteModelListPage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	page, pageSize := normalizePage(req.Page, req.PageSize)
	items := make([]domainmodeladmin.RouteModel, 0, len(s.routeModels))
	for _, item := range s.routeModels {
		if req.Visibility != "" && item.Visibility != req.Visibility {
			continue
		}
		if req.Enabled != nil && item.Enabled != *req.Enabled {
			continue
		}
		items = append(items, item)
	}
	total := len(items)
	return domainmodeladmin.RouteModelListPage{Items: slicePage(items, page, pageSize), Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *MemoryStore) GetRouteModel(_ context.Context, routeModelID int64) (domainmodeladmin.RouteModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.routeModels[routeModelID]
	if !ok {
		return domainmodeladmin.RouteModel{}, repoerr.ErrNotFound
	}
	return item, nil
}

func (s *MemoryStore) CreateRouteModel(_ context.Context, req domainmodeladmin.RouteModelWriteRequest) (domainmodeladmin.RouteModel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.routeModels {
		if item.Code == req.Code {
			return domainmodeladmin.RouteModel{}, repoerr.ErrConflict
		}
	}
	now := time.Now().UTC()
	item := domainmodeladmin.RouteModel{ID: s.nextID, Code: req.Code, Name: req.Name, Description: req.Description, Visibility: req.Visibility, Enabled: req.Enabled, SortOrder: req.SortOrder, GroupIDs: append([]int64(nil), req.GroupIDs...), CreatedAt: now, UpdatedAt: now}
	s.nextID++
	s.routeModels[item.ID] = item
	return item, nil
}

func (s *MemoryStore) UpdateRouteModel(_ context.Context, routeModelID int64, req domainmodeladmin.RouteModelWriteRequest) (domainmodeladmin.RouteModel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.routeModels[routeModelID]
	if !ok {
		return domainmodeladmin.RouteModel{}, repoerr.ErrNotFound
	}
	item.Code, item.Name, item.Description, item.Visibility = req.Code, req.Name, req.Description, req.Visibility
	item.Enabled, item.SortOrder, item.GroupIDs = req.Enabled, req.SortOrder, append([]int64(nil), req.GroupIDs...)
	item.UpdatedAt = time.Now().UTC()
	s.routeModels[routeModelID] = item
	return item, nil
}

func (s *MemoryStore) DeleteRouteModel(_ context.Context, routeModelID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.routeModels[routeModelID]; !ok {
		return repoerr.ErrNotFound
	}
	delete(s.routeModels, routeModelID)
	return nil
}

func (s *MemoryStore) ListRouteModelCandidates(_ context.Context, routeModelID int64) ([]domainmodeladmin.RouteModelCandidate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := []domainmodeladmin.RouteModelCandidate{}
	for _, item := range s.candidates {
		if routeModelID > 0 && item.RouteModelID != routeModelID {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *MemoryStore) CreateRouteModelCandidate(_ context.Context, req domainmodeladmin.RouteModelCandidateWriteRequest) (domainmodeladmin.RouteModelCandidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	model, ok := s.accountModels[req.AccountModelID]
	if !ok {
		return domainmodeladmin.RouteModelCandidate{}, repoerr.ErrNotFound
	}
	item := domainmodeladmin.RouteModelCandidate{ID: s.nextID, RouteModelID: req.RouteModelID, AccountModelID: req.AccountModelID, AccountID: model.AccountID, AccountName: model.AccountName, ModelCode: model.ModelCode, Priority: req.Priority, Weight: req.Weight, FallbackOrder: req.FallbackOrder, Enabled: req.Enabled}
	s.nextID++
	s.candidates[item.ID] = item
	return item, nil
}

func (s *MemoryStore) UpdateRouteModelCandidate(_ context.Context, candidateID int64, req domainmodeladmin.RouteModelCandidateWriteRequest) (domainmodeladmin.RouteModelCandidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.candidates[candidateID]
	if !ok {
		return domainmodeladmin.RouteModelCandidate{}, repoerr.ErrNotFound
	}
	model, ok := s.accountModels[req.AccountModelID]
	if !ok {
		return domainmodeladmin.RouteModelCandidate{}, repoerr.ErrNotFound
	}
	item.RouteModelID, item.AccountModelID, item.AccountID = req.RouteModelID, req.AccountModelID, model.AccountID
	item.AccountName, item.ModelCode = model.AccountName, model.ModelCode
	item.Priority, item.Weight, item.FallbackOrder, item.Enabled = req.Priority, req.Weight, req.FallbackOrder, req.Enabled
	s.candidates[candidateID] = item
	return item, nil
}

func (s *MemoryStore) DeleteRouteModelCandidate(_ context.Context, candidateID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.candidates[candidateID]; !ok {
		return repoerr.ErrNotFound
	}
	delete(s.candidates, candidateID)
	return nil
}

func (s *MemoryStore) ListRouteModelPrices(_ context.Context, req domainmodeladmin.RouteModelPriceListRequest) (domainmodeladmin.RouteModelPriceListPage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	page, pageSize := normalizePage(req.Page, req.PageSize)
	items := make([]domainmodeladmin.RouteModelPrice, 0, len(s.prices))
	for _, item := range s.prices {
		if req.RouteModelID > 0 && item.RouteModelID != req.RouteModelID {
			continue
		}
		if req.TaskType != "" && item.TaskType != req.TaskType {
			continue
		}
		if req.Quality != "" && item.Quality != req.Quality {
			continue
		}
		if req.Enabled != nil && item.Enabled != *req.Enabled {
			continue
		}
		items = append(items, item)
	}
	total := len(items)
	return domainmodeladmin.RouteModelPriceListPage{Items: slicePage(items, page, pageSize), Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *MemoryStore) CreateRouteModelPrice(_ context.Context, req domainmodeladmin.RouteModelPriceWriteRequest) (domainmodeladmin.RouteModelPrice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := domainmodeladmin.RouteModelPrice{ID: s.nextID, RouteModelID: req.RouteModelID, TaskType: req.TaskType, Quality: req.Quality, BasePoints: req.BasePoints, ReferenceMultiplier: req.ReferenceMultiplier, Enabled: req.Enabled}
	if rm, ok := s.routeModels[req.RouteModelID]; ok {
		item.RouteModelCode = rm.Code
	}
	s.nextID++
	s.prices[item.ID] = item
	return item, nil
}

func (s *MemoryStore) UpdateRouteModelPrice(_ context.Context, priceID int64, req domainmodeladmin.RouteModelPriceWriteRequest) (domainmodeladmin.RouteModelPrice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.prices[priceID]
	if !ok {
		return domainmodeladmin.RouteModelPrice{}, repoerr.ErrNotFound
	}
	item.RouteModelID, item.TaskType, item.Quality = req.RouteModelID, req.TaskType, req.Quality
	item.BasePoints, item.ReferenceMultiplier, item.Enabled = req.BasePoints, req.ReferenceMultiplier, req.Enabled
	if rm, ok := s.routeModels[req.RouteModelID]; ok {
		item.RouteModelCode = rm.Code
	}
	s.prices[priceID] = item
	return item, nil
}

func (s *MemoryStore) DeleteRouteModelPrice(_ context.Context, priceID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.prices[priceID]; !ok {
		return repoerr.ErrNotFound
	}
	delete(s.prices, priceID)
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
		Providers:      make([]modelhub.ModelProviderConfig, 0, len(s.providers)),
		Routes:         make([]modelhub.ModelRouteConfig, 0, len(s.routes)),
		RouteModels:    make([]modelhub.RouteModelConfig, 0, len(s.routeModels)),
		Candidates:     make([]modelhub.RouteCandidateConfig, 0, len(s.candidates)),
		Prices:         make([]modelhub.RoutePriceConfig, 0, len(s.prices)),
		ProviderModels: make([]modelhub.ProviderCandidate, 0, len(s.accountModels)),
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
	for _, item := range s.routeModels {
		snapshot.RouteModels = append(snapshot.RouteModels, modelhub.RouteModelConfig{ID: item.ID, Code: item.Code, Name: item.Name, Description: item.Description, Visibility: item.Visibility, Enabled: item.Enabled, SortOrder: item.SortOrder})
		for _, groupID := range item.GroupIDs {
			snapshot.Visibility = append(snapshot.Visibility, modelhub.RouteVisibilityConfig{RouteModelID: item.ID, GroupID: groupID})
		}
	}
	for _, item := range s.candidates {
		snapshot.Candidates = append(snapshot.Candidates, modelhub.RouteCandidateConfig{ID: item.ID, RouteModelID: item.RouteModelID, AccountModelID: item.AccountModelID, Priority: item.Priority, Weight: item.Weight, FallbackOrder: item.FallbackOrder, Enabled: item.Enabled})
	}
	for _, item := range s.prices {
		snapshot.Prices = append(snapshot.Prices, modelhub.RoutePriceConfig{ID: item.ID, RouteModelID: item.RouteModelID, TaskType: item.TaskType, Quality: item.Quality, BasePoints: item.BasePoints, ReferenceMultiplier: item.ReferenceMultiplier, Enabled: item.Enabled})
	}
	for _, item := range s.accountModels {
		account := s.accounts[item.AccountID]
		if !item.Enabled || account.Status != domainmodeladmin.ModelAccountStatusEnabled {
			continue
		}
		snapshot.ProviderModels = append(snapshot.ProviderModels, modelhub.ProviderCandidate{AccountModelID: item.ID, ModelAccountID: item.AccountID, Provider: account.AdapterType, AdapterType: account.AdapterType, AuthType: account.AuthType, BaseURL: account.BaseURL, Credentials: account.CredentialsEncrypted, ModelCode: item.ModelCode, SupportedTaskTypes: append([]string(nil), item.TaskTypes...), SupportedQualities: append([]string(nil), item.Qualities...), HealthStatus: account.Status, OutputCost: item.CostPerImage, Currency: item.Currency, AccountExtra: cloneModelAdminExtra(account.Extra), ModelExtra: cloneModelAdminExtra(item.Extra)})
	}
	return snapshot, nil
}

func cloneModelAdminExtra(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
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
