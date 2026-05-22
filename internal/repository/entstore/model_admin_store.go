package entstore

import (
	"context"
	"strings"

	domainmodeladmin "github.com/fatballfish/pic-gallery/internal/domain/modeladmin"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/modelprovider"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/modelroute"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
)

type ModelAdminStore struct {
	client *repoent.Client
}

func NewModelAdminStore(client *repoent.Client) *ModelAdminStore {
	return &ModelAdminStore{client: client}
}

func (s *ModelAdminStore) ListProviders(ctx context.Context, req domainmodeladmin.ProviderListRequest) (domainmodeladmin.ProviderListPage, error) {
	page, pageSize := normalizeModelAdminPage(req.Page, req.PageSize)
	query := s.client.ModelProvider.Query()
	if req.ProviderType != "" {
		query = query.Where(modelprovider.ProviderTypeEQ(req.ProviderType))
	}
	if req.Enabled != nil {
		query = query.Where(modelprovider.EnabledEQ(*req.Enabled))
	}
	total, err := query.Count(ctx)
	if err != nil {
		return domainmodeladmin.ProviderListPage{}, err
	}
	entities, err := query.Order(repoent.Asc(modelprovider.FieldProviderCode)).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		return domainmodeladmin.ProviderListPage{}, err
	}
	items := make([]domainmodeladmin.Provider, 0, len(entities))
	for _, entity := range entities {
		items = append(items, mapModelProvider(entity))
	}
	return domainmodeladmin.ProviderListPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *ModelAdminStore) GetProvider(ctx context.Context, providerCode string) (domainmodeladmin.Provider, error) {
	entity, err := s.client.ModelProvider.Query().Where(modelprovider.ProviderCodeEQ(providerCode)).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainmodeladmin.Provider{}, repoerr.ErrNotFound
		}
		return domainmodeladmin.Provider{}, err
	}
	return mapModelProvider(entity), nil
}

func (s *ModelAdminStore) CreateProvider(ctx context.Context, req domainmodeladmin.ProviderWriteRequest) (domainmodeladmin.Provider, error) {
	entity, err := s.client.ModelProvider.Create().
		SetProviderCode(req.ProviderCode).
		SetProviderType(req.ProviderType).
		SetAuthConfigEncrypted(req.AuthConfigEncrypted).
		SetHealthStatus(req.HealthStatus).
		SetEnabled(req.Enabled).
		Save(ctx)
	if err != nil {
		if repoent.IsConstraintError(err) {
			return domainmodeladmin.Provider{}, repoerr.ErrConflict
		}
		return domainmodeladmin.Provider{}, err
	}
	return mapModelProvider(entity), nil
}

func (s *ModelAdminStore) UpdateProvider(ctx context.Context, providerCode string, req domainmodeladmin.ProviderWriteRequest) (domainmodeladmin.Provider, error) {
	entity, err := s.client.ModelProvider.Query().Where(modelprovider.ProviderCodeEQ(providerCode)).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainmodeladmin.Provider{}, repoerr.ErrNotFound
		}
		return domainmodeladmin.Provider{}, err
	}
	updated, err := s.client.ModelProvider.UpdateOneID(entity.ID).
		SetProviderCode(req.ProviderCode).
		SetProviderType(req.ProviderType).
		SetAuthConfigEncrypted(req.AuthConfigEncrypted).
		SetHealthStatus(req.HealthStatus).
		SetEnabled(req.Enabled).
		Save(ctx)
	if err != nil {
		if repoent.IsConstraintError(err) {
			return domainmodeladmin.Provider{}, repoerr.ErrConflict
		}
		return domainmodeladmin.Provider{}, err
	}
	if updated.ProviderCode != providerCode {
		if _, err := s.client.ModelRoute.Update().
			Where(modelroute.ProviderModelIDEQ(int64(entity.ID))).
			Save(ctx); err != nil {
			return domainmodeladmin.Provider{}, err
		}
	}
	return mapModelProvider(updated), nil
}

func (s *ModelAdminStore) DeleteProvider(ctx context.Context, providerCode string) error {
	entity, err := s.client.ModelProvider.Query().Where(modelprovider.ProviderCodeEQ(providerCode)).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return repoerr.ErrNotFound
		}
		return err
	}
	if count, err := s.client.ModelRoute.Query().Where(modelroute.ProviderModelIDEQ(int64(entity.ID))).Count(ctx); err != nil {
		return err
	} else if count > 0 {
		return repoerr.ErrConflict
	}
	deleted, err := s.client.ModelProvider.Delete().Where(modelprovider.ProviderCodeEQ(providerCode)).Exec(ctx)
	if err != nil {
		return err
	}
	if deleted == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

func (s *ModelAdminStore) ListRoutes(ctx context.Context, req domainmodeladmin.RouteListRequest) (domainmodeladmin.RouteListPage, error) {
	page, pageSize := normalizeModelAdminPage(req.Page, req.PageSize)
	query := s.client.ModelRoute.Query()
	if req.GroupCode != "" {
		query = query.Where(modelroute.GroupCodeEQ(req.GroupCode))
	}
	if req.TaskType != "" {
		query = query.Where(modelroute.TaskTypeEQ(req.TaskType))
	}
	if req.Enabled != nil {
		query = query.Where(modelroute.EnabledEQ(*req.Enabled))
	}
	if req.ProviderCode != "" {
		providerEntity, err := s.client.ModelProvider.Query().Where(modelprovider.ProviderCodeEQ(req.ProviderCode)).Only(ctx)
		if err != nil {
			if repoent.IsNotFound(err) {
				return domainmodeladmin.RouteListPage{Items: []domainmodeladmin.Route{}, Page: page, PageSize: pageSize, Total: 0}, nil
			}
			return domainmodeladmin.RouteListPage{}, err
		}
		query = query.Where(modelroute.ProviderModelIDEQ(int64(providerEntity.ID)))
	}
	total, err := query.Count(ctx)
	if err != nil {
		return domainmodeladmin.RouteListPage{}, err
	}
	entities, err := query.Order(repoent.Asc(modelroute.FieldPriority), repoent.Asc(modelroute.FieldFallbackOrder), repoent.Asc(modelroute.FieldID)).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		return domainmodeladmin.RouteListPage{}, err
	}
	items, err := s.mapModelRoutes(ctx, entities)
	if err != nil {
		return domainmodeladmin.RouteListPage{}, err
	}
	return domainmodeladmin.RouteListPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *ModelAdminStore) GetRoute(ctx context.Context, routeID int64) (domainmodeladmin.Route, error) {
	entity, err := s.client.ModelRoute.Get(ctx, int(routeID))
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainmodeladmin.Route{}, repoerr.ErrNotFound
		}
		return domainmodeladmin.Route{}, err
	}
	return s.mapModelRoute(ctx, entity)
}

func (s *ModelAdminStore) CreateRoute(ctx context.Context, req domainmodeladmin.RouteWriteRequest) (domainmodeladmin.Route, error) {
	providerEntity, err := s.resolveRouteProvider(ctx, req)
	if err != nil {
		return domainmodeladmin.Route{}, err
	}
	entity, err := s.client.ModelRoute.Create().
		SetGroupCode(req.GroupCode).
		SetTaskType(req.TaskType).
		SetProviderModelID(int64(providerEntity.ID)).
		SetPriority(req.Priority).
		SetWeightPercent(req.WeightPercent).
		SetFallbackOrder(req.FallbackOrder).
		SetEnabled(req.Enabled).
		Save(ctx)
	if err != nil {
		return domainmodeladmin.Route{}, err
	}
	return mapModelRoute(entity, providerEntity.ProviderCode), nil
}

func (s *ModelAdminStore) UpdateRoute(ctx context.Context, routeID int64, req domainmodeladmin.RouteWriteRequest) (domainmodeladmin.Route, error) {
	if _, err := s.client.ModelRoute.Get(ctx, int(routeID)); err != nil {
		if repoent.IsNotFound(err) {
			return domainmodeladmin.Route{}, repoerr.ErrNotFound
		}
		return domainmodeladmin.Route{}, err
	}
	providerEntity, err := s.resolveRouteProvider(ctx, req)
	if err != nil {
		return domainmodeladmin.Route{}, err
	}
	entity, err := s.client.ModelRoute.UpdateOneID(int(routeID)).
		SetGroupCode(req.GroupCode).
		SetTaskType(req.TaskType).
		SetProviderModelID(int64(providerEntity.ID)).
		SetPriority(req.Priority).
		SetWeightPercent(req.WeightPercent).
		SetFallbackOrder(req.FallbackOrder).
		SetEnabled(req.Enabled).
		Save(ctx)
	if err != nil {
		return domainmodeladmin.Route{}, err
	}
	return mapModelRoute(entity, providerEntity.ProviderCode), nil
}

func (s *ModelAdminStore) DeleteRoute(ctx context.Context, routeID int64) error {
	err := s.client.ModelRoute.DeleteOneID(int(routeID)).Exec(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return repoerr.ErrNotFound
		}
		return err
	}
	return nil
}

func (s *ModelAdminStore) ModelRoutingConfig(ctx context.Context) (modelhub.ModelRoutingSnapshot, error) {
	providers, err := s.client.ModelProvider.Query().Order(repoent.Asc(modelprovider.FieldProviderCode)).All(ctx)
	if err != nil {
		return modelhub.ModelRoutingSnapshot{}, err
	}
	routes, err := s.client.ModelRoute.Query().
		Where(modelroute.EnabledEQ(true)).
		Order(repoent.Asc(modelroute.FieldPriority), repoent.Asc(modelroute.FieldFallbackOrder), repoent.Asc(modelroute.FieldID)).
		All(ctx)
	if err != nil {
		return modelhub.ModelRoutingSnapshot{}, err
	}
	providerByID := make(map[int64]*repoent.ModelProvider, len(providers))
	snapshot := modelhub.ModelRoutingSnapshot{
		Providers: make([]modelhub.ModelProviderConfig, 0, len(providers)),
		Routes:    make([]modelhub.ModelRouteConfig, 0, len(routes)),
	}
	for _, providerEntity := range providers {
		providerByID[int64(providerEntity.ID)] = providerEntity
		snapshot.Providers = append(snapshot.Providers, modelhub.ModelProviderConfig{
			ID:           int64(providerEntity.ID),
			ProviderCode: providerEntity.ProviderCode,
			ProviderType: providerEntity.ProviderType,
			Enabled:      providerEntity.Enabled,
		})
	}
	for _, route := range routes {
		providerCode := ""
		if providerEntity := providerByID[route.ProviderModelID]; providerEntity != nil {
			providerCode = providerEntity.ProviderCode
		}
		snapshot.Routes = append(snapshot.Routes, modelhub.ModelRouteConfig{
			ID:              int64(route.ID),
			GroupCode:       route.GroupCode,
			TaskType:        route.TaskType,
			ProviderModelID: route.ProviderModelID,
			ProviderCode:    providerCode,
			Priority:        route.Priority,
			WeightPercent:   route.WeightPercent,
			FallbackOrder:   route.FallbackOrder,
			Enabled:         route.Enabled,
		})
	}
	return snapshot, nil
}

func (s *ModelAdminStore) resolveRouteProvider(ctx context.Context, req domainmodeladmin.RouteWriteRequest) (*repoent.ModelProvider, error) {
	var (
		providerEntity *repoent.ModelProvider
		err            error
	)
	if strings.TrimSpace(req.ProviderCode) != "" {
		providerEntity, err = s.client.ModelProvider.Query().Where(modelprovider.ProviderCodeEQ(req.ProviderCode)).Only(ctx)
	} else {
		providerEntity, err = s.client.ModelProvider.Get(ctx, int(req.ProviderModelID))
	}
	if err != nil {
		if repoent.IsNotFound(err) {
			return nil, repoerr.ErrNotFound
		}
		return nil, err
	}
	return providerEntity, nil
}

func (s *ModelAdminStore) mapModelRoutes(ctx context.Context, entities []*repoent.ModelRoute) ([]domainmodeladmin.Route, error) {
	providers, err := s.client.ModelProvider.Query().All(ctx)
	if err != nil {
		return nil, err
	}
	codeByID := make(map[int64]string, len(providers))
	for _, providerEntity := range providers {
		codeByID[int64(providerEntity.ID)] = providerEntity.ProviderCode
	}
	items := make([]domainmodeladmin.Route, 0, len(entities))
	for _, entity := range entities {
		items = append(items, mapModelRoute(entity, codeByID[entity.ProviderModelID]))
	}
	return items, nil
}

func (s *ModelAdminStore) mapModelRoute(ctx context.Context, entity *repoent.ModelRoute) (domainmodeladmin.Route, error) {
	providerCode := ""
	if entity.ProviderModelID > 0 {
		providerEntity, err := s.client.ModelProvider.Get(ctx, int(entity.ProviderModelID))
		if err != nil && !repoent.IsNotFound(err) {
			return domainmodeladmin.Route{}, err
		}
		if err == nil {
			providerCode = providerEntity.ProviderCode
		}
	}
	return mapModelRoute(entity, providerCode), nil
}

func mapModelProvider(entity *repoent.ModelProvider) domainmodeladmin.Provider {
	return domainmodeladmin.Provider{
		ID:                  int64(entity.ID),
		ProviderCode:        entity.ProviderCode,
		ProviderType:        entity.ProviderType,
		AuthConfigEncrypted: entity.AuthConfigEncrypted,
		HealthStatus:        entity.HealthStatus,
		Enabled:             entity.Enabled,
		CreatedAt:           entity.CreatedAt,
		UpdatedAt:           entity.UpdatedAt,
	}
}

func mapModelRoute(entity *repoent.ModelRoute, providerCode string) domainmodeladmin.Route {
	return domainmodeladmin.Route{
		ID:              int64(entity.ID),
		GroupCode:       entity.GroupCode,
		TaskType:        entity.TaskType,
		ProviderModelID: entity.ProviderModelID,
		ProviderCode:    providerCode,
		Priority:        entity.Priority,
		WeightPercent:   entity.WeightPercent,
		FallbackOrder:   entity.FallbackOrder,
		Enabled:         entity.Enabled,
		CreatedAt:       entity.CreatedAt,
		UpdatedAt:       entity.UpdatedAt,
	}
}

func normalizeModelAdminPage(page, pageSize int) (int, int) {
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
