package entstore

import (
	"context"
	"time"

	domainmodeladmin "github.com/fatballfish/pic-gallery/internal/domain/modeladmin"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/modelprovider"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/modelroute"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/providermodel"
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
	if _, err := s.client.ProviderModel.Create().
		SetProviderID(int64(entity.ID)).
		SetModelCode(entity.ProviderCode + "-default").
		SetMaxImageCount(1).
		SetMaxReferenceImageCount(0).
		SetTimeoutMs(60000).
		SetInputCost("0").
		SetOutputCost("0").
		SetCurrency("CNY").
		SetHealthStatus(entity.HealthStatus).
		SetEnabled(entity.Enabled).
		Save(ctx); err != nil {
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
	if _, err := s.client.ProviderModel.Update().
		Where(providermodel.ProviderIDEQ(int64(entity.ID))).
		SetHealthStatus(updated.HealthStatus).
		SetEnabled(updated.Enabled).
		Save(ctx); err != nil {
		return domainmodeladmin.Provider{}, err
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
	models, err := s.client.ProviderModel.Query().Where(providermodel.ProviderIDEQ(int64(entity.ID))).All(ctx)
	if err != nil {
		return err
	}
	for _, model := range models {
		if count, err := s.client.ModelRoute.Query().Where(modelroute.ProviderModelIDEQ(int64(model.ID))).Count(ctx); err != nil {
			return err
		} else if count > 0 {
			return repoerr.ErrConflict
		}
	}
	if len(models) > 0 {
		if _, err := s.client.ProviderModel.Delete().Where(providermodel.ProviderIDEQ(int64(entity.ID))).Exec(ctx); err != nil {
			return err
		}
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

func (s *ModelAdminStore) ListProviderModels(ctx context.Context, req domainmodeladmin.ProviderModelListRequest) (domainmodeladmin.ProviderModelListPage, error) {
	page, pageSize := normalizeModelAdminPage(req.Page, req.PageSize)
	query := s.client.ProviderModel.Query()
	if req.ProviderCode != "" {
		providerEntity, err := s.client.ModelProvider.Query().Where(modelprovider.ProviderCodeEQ(req.ProviderCode)).Only(ctx)
		if err != nil {
			if repoent.IsNotFound(err) {
				return domainmodeladmin.ProviderModelListPage{Items: []domainmodeladmin.ProviderModel{}, Page: page, PageSize: pageSize, Total: 0}, nil
			}
			return domainmodeladmin.ProviderModelListPage{}, err
		}
		query = query.Where(providermodel.ProviderIDEQ(int64(providerEntity.ID)))
	}
	if req.ModelCode != "" {
		query = query.Where(providermodel.ModelCodeEQ(req.ModelCode))
	}
	if req.Enabled != nil {
		query = query.Where(providermodel.EnabledEQ(*req.Enabled))
	}
	total, err := query.Count(ctx)
	if err != nil {
		return domainmodeladmin.ProviderModelListPage{}, err
	}
	entities, err := query.Order(repoent.Asc(providermodel.FieldProviderID), repoent.Asc(providermodel.FieldModelCode)).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		return domainmodeladmin.ProviderModelListPage{}, err
	}
	items, err := s.mapProviderModels(ctx, entities)
	if err != nil {
		return domainmodeladmin.ProviderModelListPage{}, err
	}
	return domainmodeladmin.ProviderModelListPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *ModelAdminStore) GetProviderModel(ctx context.Context, providerModelID int64) (domainmodeladmin.ProviderModel, error) {
	entity, err := s.client.ProviderModel.Get(ctx, int(providerModelID))
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainmodeladmin.ProviderModel{}, repoerr.ErrNotFound
		}
		return domainmodeladmin.ProviderModel{}, err
	}
	return s.mapProviderModel(ctx, entity)
}

func (s *ModelAdminStore) CreateProviderModel(ctx context.Context, req domainmodeladmin.ProviderModelWriteRequest) (domainmodeladmin.ProviderModel, error) {
	providerEntity, err := s.client.ModelProvider.Query().Where(modelprovider.ProviderCodeEQ(req.ProviderCode)).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainmodeladmin.ProviderModel{}, repoerr.ErrNotFound
		}
		return domainmodeladmin.ProviderModel{}, err
	}
	entity, err := s.client.ProviderModel.Create().
		SetProviderID(int64(providerEntity.ID)).
		SetModelCode(req.ModelCode).
		SetCompatMode(req.CompatMode).
		SetSupportsImageInput(req.SupportsImageInput).
		SetSupportsMask(req.SupportsMask).
		SetSupportedQualities(req.SupportedQualities).
		SetSupportedRatios(req.SupportedRatios).
		SetMaxImageCount(req.MaxImageCount).
		SetMaxReferenceImageCount(req.MaxReferenceImageCount).
		SetTimeoutMs(req.TimeoutMS).
		SetInputCost(req.InputCost).
		SetOutputCost(req.OutputCost).
		SetCurrency(req.Currency).
		SetHealthStatus(req.HealthStatus).
		SetNillableLastHealthCheckedAt(req.LastHealthCheckedAt).
		SetEnabled(req.Enabled).
		Save(ctx)
	if err != nil {
		if repoent.IsConstraintError(err) {
			return domainmodeladmin.ProviderModel{}, repoerr.ErrConflict
		}
		return domainmodeladmin.ProviderModel{}, err
	}
	return mapProviderModel(entity, providerEntity.ProviderCode), nil
}

func (s *ModelAdminStore) UpdateProviderModel(ctx context.Context, providerModelID int64, req domainmodeladmin.ProviderModelWriteRequest) (domainmodeladmin.ProviderModel, error) {
	if _, err := s.client.ProviderModel.Get(ctx, int(providerModelID)); err != nil {
		if repoent.IsNotFound(err) {
			return domainmodeladmin.ProviderModel{}, repoerr.ErrNotFound
		}
		return domainmodeladmin.ProviderModel{}, err
	}
	providerEntity, err := s.client.ModelProvider.Query().Where(modelprovider.ProviderCodeEQ(req.ProviderCode)).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainmodeladmin.ProviderModel{}, repoerr.ErrNotFound
		}
		return domainmodeladmin.ProviderModel{}, err
	}
	update := s.client.ProviderModel.UpdateOneID(int(providerModelID)).
		SetProviderID(int64(providerEntity.ID)).
		SetModelCode(req.ModelCode).
		SetCompatMode(req.CompatMode).
		SetSupportsImageInput(req.SupportsImageInput).
		SetSupportsMask(req.SupportsMask).
		SetSupportedQualities(req.SupportedQualities).
		SetSupportedRatios(req.SupportedRatios).
		SetMaxImageCount(req.MaxImageCount).
		SetMaxReferenceImageCount(req.MaxReferenceImageCount).
		SetTimeoutMs(req.TimeoutMS).
		SetInputCost(req.InputCost).
		SetOutputCost(req.OutputCost).
		SetCurrency(req.Currency).
		SetHealthStatus(req.HealthStatus).
		SetEnabled(req.Enabled)
	if req.LastHealthCheckedAt != nil {
		update.SetLastHealthCheckedAt(*req.LastHealthCheckedAt)
	} else {
		update.ClearLastHealthCheckedAt()
	}
	entity, err := update.Save(ctx)
	if err != nil {
		if repoent.IsConstraintError(err) {
			return domainmodeladmin.ProviderModel{}, repoerr.ErrConflict
		}
		return domainmodeladmin.ProviderModel{}, err
	}
	return mapProviderModel(entity, providerEntity.ProviderCode), nil
}

func (s *ModelAdminStore) DeleteProviderModel(ctx context.Context, providerModelID int64) error {
	if count, err := s.client.ModelRoute.Query().Where(modelroute.ProviderModelIDEQ(providerModelID)).Count(ctx); err != nil {
		return err
	} else if count > 0 {
		return repoerr.ErrConflict
	}
	if err := s.client.ProviderModel.DeleteOneID(int(providerModelID)).Exec(ctx); err != nil {
		if repoent.IsNotFound(err) {
			return repoerr.ErrNotFound
		}
		return err
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
		models, err := s.client.ProviderModel.Query().Where(providermodel.ProviderIDEQ(int64(providerEntity.ID))).All(ctx)
		if err != nil {
			return domainmodeladmin.RouteListPage{}, err
		}
		if len(models) == 0 {
			return domainmodeladmin.RouteListPage{Items: []domainmodeladmin.Route{}, Page: page, PageSize: pageSize, Total: 0}, nil
		}
		ids := make([]int64, 0, len(models))
		for _, model := range models {
			ids = append(ids, int64(model.ID))
		}
		query = query.Where(modelroute.ProviderModelIDIn(ids...))
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
	providerEntity, err := s.resolveRouteProviderModel(ctx, req)
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
	return s.mapModelRoute(ctx, entity)
}

func (s *ModelAdminStore) UpdateRoute(ctx context.Context, routeID int64, req domainmodeladmin.RouteWriteRequest) (domainmodeladmin.Route, error) {
	if _, err := s.client.ModelRoute.Get(ctx, int(routeID)); err != nil {
		if repoent.IsNotFound(err) {
			return domainmodeladmin.Route{}, repoerr.ErrNotFound
		}
		return domainmodeladmin.Route{}, err
	}
	providerEntity, err := s.resolveRouteProviderModel(ctx, req)
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
	return s.mapModelRoute(ctx, entity)
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
	providerModels, err := s.client.ProviderModel.Query().All(ctx)
	if err != nil {
		return modelhub.ModelRoutingSnapshot{}, err
	}
	providerModelByID := make(map[int64]*repoent.ProviderModel, len(providerModels))
	snapshot := modelhub.ModelRoutingSnapshot{
		Providers:      make([]modelhub.ModelProviderConfig, 0, len(providers)),
		ProviderModels: make([]modelhub.ProviderCandidate, 0, len(providerModels)),
		Routes:         make([]modelhub.ModelRouteConfig, 0, len(routes)),
	}
	latestVersionAt := time.Time{}
	for _, providerEntity := range providers {
		providerByID[int64(providerEntity.ID)] = providerEntity
		if providerEntity.UpdatedAt.After(latestVersionAt) {
			latestVersionAt = providerEntity.UpdatedAt
		}
		snapshot.Providers = append(snapshot.Providers, modelhub.ModelProviderConfig{
			ID:           int64(providerEntity.ID),
			ProviderCode: providerEntity.ProviderCode,
			ProviderType: providerEntity.ProviderType,
			Enabled:      providerEntity.Enabled,
		})
	}
	for _, providerModel := range providerModels {
		providerModelByID[int64(providerModel.ID)] = providerModel
		if providerModel.UpdatedAt.After(latestVersionAt) {
			latestVersionAt = providerModel.UpdatedAt
		}
		providerCode := ""
		if providerEntity := providerByID[providerModel.ProviderID]; providerEntity != nil {
			providerCode = providerEntity.ProviderCode
		}
		snapshot.ProviderModels = append(snapshot.ProviderModels, modelhub.ProviderCandidate{
			ProviderModelID:        int64(providerModel.ID),
			Provider:               providerCode,
			ModelCode:              providerModel.ModelCode,
			CompatMode:             providerModel.CompatMode,
			SupportedTaskTypes:     nil,
			SupportedQualities:     append([]string(nil), providerModel.SupportedQualities...),
			SupportedAspectRatios:  append([]string(nil), providerModel.SupportedRatios...),
			MaxImageCount:          providerModel.MaxImageCount,
			MaxReferenceImageCount: providerModel.MaxReferenceImageCount,
			SupportsImageInput:     providerModel.SupportsImageInput,
			SupportsMask:           providerModel.SupportsMask,
			InputCost:              providerModel.InputCost,
			OutputCost:             providerModel.OutputCost,
			Currency:               providerModel.Currency,
			HealthStatus:           providerModel.HealthStatus,
		})
	}
	for _, route := range routes {
		if route.UpdatedAt.After(latestVersionAt) {
			latestVersionAt = route.UpdatedAt
		}
		providerCode := ""
		if providerModel := providerModelByID[route.ProviderModelID]; providerModel != nil {
			if providerEntity := providerByID[providerModel.ProviderID]; providerEntity != nil {
				providerCode = providerEntity.ProviderCode
			}
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
	if !latestVersionAt.IsZero() {
		snapshot.Version = latestVersionAt.UTC().Format(time.RFC3339Nano)
	}
	return snapshot, nil
}

func (s *ModelAdminStore) resolveRouteProviderModel(ctx context.Context, req domainmodeladmin.RouteWriteRequest) (*repoent.ProviderModel, error) {
	var (
		providerEntity *repoent.ProviderModel
		err            error
	)
	if req.ProviderModelID > 0 {
		providerEntity, err = s.client.ProviderModel.Get(ctx, int(req.ProviderModelID))
	} else {
		rootProvider, providerErr := s.client.ModelProvider.Query().Where(modelprovider.ProviderCodeEQ(req.ProviderCode)).Only(ctx)
		if providerErr != nil {
			if repoent.IsNotFound(providerErr) {
				return nil, repoerr.ErrNotFound
			}
			return nil, providerErr
		}
		providerEntity, err = s.client.ProviderModel.Query().
			Where(providermodel.ProviderIDEQ(int64(rootProvider.ID))).
			Order(repoent.Asc(providermodel.FieldID)).
			First(ctx)
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
	providerModels, err := s.client.ProviderModel.Query().All(ctx)
	if err != nil {
		return nil, err
	}
	providers, err := s.client.ModelProvider.Query().All(ctx)
	if err != nil {
		return nil, err
	}
	providerCodeByProviderID := make(map[int64]string, len(providers))
	for _, providerEntity := range providers {
		providerCodeByProviderID[int64(providerEntity.ID)] = providerEntity.ProviderCode
	}
	codeByModelID := make(map[int64]string, len(providerModels))
	for _, providerModel := range providerModels {
		codeByModelID[int64(providerModel.ID)] = providerCodeByProviderID[providerModel.ProviderID]
	}
	items := make([]domainmodeladmin.Route, 0, len(entities))
	for _, entity := range entities {
		items = append(items, mapModelRoute(entity, codeByModelID[entity.ProviderModelID]))
	}
	return items, nil
}

func (s *ModelAdminStore) mapModelRoute(ctx context.Context, entity *repoent.ModelRoute) (domainmodeladmin.Route, error) {
	providerCode := ""
	if entity.ProviderModelID > 0 {
		providerModelEntity, err := s.client.ProviderModel.Get(ctx, int(entity.ProviderModelID))
		if err != nil && !repoent.IsNotFound(err) {
			return domainmodeladmin.Route{}, err
		}
		if err == nil {
			providerEntity, providerErr := s.client.ModelProvider.Get(ctx, int(providerModelEntity.ProviderID))
			if providerErr != nil && !repoent.IsNotFound(providerErr) {
				return domainmodeladmin.Route{}, providerErr
			}
			if providerErr == nil {
				providerCode = providerEntity.ProviderCode
			}
		}
	}
	return mapModelRoute(entity, providerCode), nil
}

func (s *ModelAdminStore) mapProviderModels(ctx context.Context, entities []*repoent.ProviderModel) ([]domainmodeladmin.ProviderModel, error) {
	providers, err := s.client.ModelProvider.Query().All(ctx)
	if err != nil {
		return nil, err
	}
	codeByID := make(map[int64]string, len(providers))
	for _, providerEntity := range providers {
		codeByID[int64(providerEntity.ID)] = providerEntity.ProviderCode
	}
	items := make([]domainmodeladmin.ProviderModel, 0, len(entities))
	for _, entity := range entities {
		items = append(items, mapProviderModel(entity, codeByID[entity.ProviderID]))
	}
	return items, nil
}

func (s *ModelAdminStore) mapProviderModel(ctx context.Context, entity *repoent.ProviderModel) (domainmodeladmin.ProviderModel, error) {
	providerCode := ""
	if entity.ProviderID > 0 {
		providerEntity, err := s.client.ModelProvider.Get(ctx, int(entity.ProviderID))
		if err != nil && !repoent.IsNotFound(err) {
			return domainmodeladmin.ProviderModel{}, err
		}
		if err == nil {
			providerCode = providerEntity.ProviderCode
		}
	}
	return mapProviderModel(entity, providerCode), nil
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

func mapProviderModel(entity *repoent.ProviderModel, providerCode string) domainmodeladmin.ProviderModel {
	return domainmodeladmin.ProviderModel{
		ID:                     int64(entity.ID),
		ProviderID:             entity.ProviderID,
		ProviderCode:           providerCode,
		ModelCode:              entity.ModelCode,
		CompatMode:             entity.CompatMode,
		SupportsImageInput:     entity.SupportsImageInput,
		SupportsMask:           entity.SupportsMask,
		SupportedQualities:     append([]string(nil), entity.SupportedQualities...),
		SupportedRatios:        append([]string(nil), entity.SupportedRatios...),
		MaxImageCount:          entity.MaxImageCount,
		MaxReferenceImageCount: entity.MaxReferenceImageCount,
		TimeoutMS:              entity.TimeoutMs,
		InputCost:              entity.InputCost,
		OutputCost:             entity.OutputCost,
		Currency:               entity.Currency,
		HealthStatus:           entity.HealthStatus,
		LastHealthCheckedAt:    entity.LastHealthCheckedAt,
		Enabled:                entity.Enabled,
		CreatedAt:              entity.CreatedAt,
		UpdatedAt:              entity.UpdatedAt,
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
