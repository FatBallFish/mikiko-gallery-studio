package entstore

import (
	"context"
	"strings"
	"time"

	domainmodeladmin "github.com/fatballfish/pic-gallery/internal/domain/modeladmin"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/modelaccount"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/modelaccountmodel"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/modelprovider"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/modelroute"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/providermodel"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/routemodel"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/routemodelcandidate"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/routemodelprice"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/routemodelvisibilitygroup"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
)

type ModelAdminStore struct {
	client *repoent.Client
}

func NewModelAdminStore(client *repoent.Client) *ModelAdminStore {
	return &ModelAdminStore{client: client}
}

func (s *ModelAdminStore) ListModelAccounts(ctx context.Context, req domainmodeladmin.ModelAccountListRequest) (domainmodeladmin.ModelAccountListPage, error) {
	page, pageSize := normalizeModelAdminPage(req.Page, req.PageSize)
	query := s.client.ModelAccount.Query().Where(modelaccount.DeletedAtIsNil())
	if req.AdapterType != "" {
		query = query.Where(modelaccount.AdapterTypeEQ(req.AdapterType))
	}
	if req.AuthType != "" {
		query = query.Where(modelaccount.AuthTypeEQ(req.AuthType))
	}
	if req.Status != "" {
		query = query.Where(modelaccount.StatusEQ(req.Status))
	}
	total, err := query.Count(ctx)
	if err != nil {
		return domainmodeladmin.ModelAccountListPage{}, err
	}
	entities, err := query.Order(repoent.Asc(modelaccount.FieldID)).Offset((page - 1) * pageSize).Limit(pageSize).All(ctx)
	if err != nil {
		return domainmodeladmin.ModelAccountListPage{}, err
	}
	items := make([]domainmodeladmin.ModelAccount, 0, len(entities))
	for _, entity := range entities {
		items = append(items, mapModelAccount(entity))
	}
	return domainmodeladmin.ModelAccountListPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *ModelAdminStore) GetModelAccount(ctx context.Context, accountID int64) (domainmodeladmin.ModelAccount, error) {
	entity, err := s.client.ModelAccount.Query().Where(modelaccount.IDEQ(int(accountID)), modelaccount.DeletedAtIsNil()).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainmodeladmin.ModelAccount{}, repoerr.ErrNotFound
		}
		return domainmodeladmin.ModelAccount{}, err
	}
	return mapModelAccount(entity), nil
}

func (s *ModelAdminStore) CreateModelAccount(ctx context.Context, req domainmodeladmin.ModelAccountWriteRequest) (domainmodeladmin.ModelAccount, error) {
	entity, err := s.client.ModelAccount.Create().
		SetName(req.Name).
		SetAdapterType(req.AdapterType).
		SetAuthType(req.AuthType).
		SetBaseURL(req.BaseURL).
		SetCredentialsEncrypted(req.Credentials).
		SetStatus(req.Status).
		SetPriority(req.Priority).
		SetWeight(req.Weight).
		SetConcurrencyLimit(req.ConcurrencyLimit).
		SetTimeoutMs(req.TimeoutMS).
		SetExtra(req.Extra).
		Save(ctx)
	if err != nil {
		if repoent.IsConstraintError(err) {
			return domainmodeladmin.ModelAccount{}, repoerr.ErrConflict
		}
		return domainmodeladmin.ModelAccount{}, err
	}
	return mapModelAccount(entity), nil
}

func (s *ModelAdminStore) UpdateModelAccount(ctx context.Context, accountID int64, req domainmodeladmin.ModelAccountWriteRequest) (domainmodeladmin.ModelAccount, error) {
	update := s.client.ModelAccount.UpdateOneID(int(accountID)).
		SetName(req.Name).
		SetAdapterType(req.AdapterType).
		SetAuthType(req.AuthType).
		SetBaseURL(req.BaseURL).
		SetStatus(req.Status).
		SetPriority(req.Priority).
		SetWeight(req.Weight).
		SetConcurrencyLimit(req.ConcurrencyLimit).
		SetTimeoutMs(req.TimeoutMS).
		SetExtra(req.Extra)
	if req.Credentials != nil {
		update.SetCredentialsEncrypted(req.Credentials)
	}
	entity, err := update.Save(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainmodeladmin.ModelAccount{}, repoerr.ErrNotFound
		}
		return domainmodeladmin.ModelAccount{}, err
	}
	return mapModelAccount(entity), nil
}

func (s *ModelAdminStore) DeleteModelAccount(ctx context.Context, accountID int64) error {
	count, err := s.client.ModelAccountModel.Query().Where(modelaccountmodel.AccountIDEQ(accountID), modelaccountmodel.DeletedAtIsNil()).Count(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return repoerr.ErrConflict
	}
	affected, err := s.client.ModelAccount.Update().Where(modelaccount.IDEQ(int(accountID)), modelaccount.DeletedAtIsNil()).SetDeletedAt(time.Now().UTC()).Save(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

func (s *ModelAdminStore) ListModelAccountModels(ctx context.Context, req domainmodeladmin.ModelAccountModelListRequest) (domainmodeladmin.ModelAccountModelListPage, error) {
	page, pageSize := normalizeModelAdminPage(req.Page, req.PageSize)
	query := s.client.ModelAccountModel.Query().Where(modelaccountmodel.DeletedAtIsNil())
	if req.AccountID > 0 {
		query = query.Where(modelaccountmodel.AccountIDEQ(req.AccountID))
	}
	if req.ModelCode != "" {
		query = query.Where(modelaccountmodel.ModelCodeEQ(req.ModelCode))
	}
	if req.Enabled != nil {
		query = query.Where(modelaccountmodel.EnabledEQ(*req.Enabled))
	}
	total, err := query.Count(ctx)
	if err != nil {
		return domainmodeladmin.ModelAccountModelListPage{}, err
	}
	entities, err := query.Order(repoent.Asc(modelaccountmodel.FieldAccountID), repoent.Asc(modelaccountmodel.FieldModelCode)).Offset((page - 1) * pageSize).Limit(pageSize).All(ctx)
	if err != nil {
		return domainmodeladmin.ModelAccountModelListPage{}, err
	}
	items := make([]domainmodeladmin.ModelAccountModel, 0, len(entities))
	for _, entity := range entities {
		items = append(items, s.mapModelAccountModel(ctx, entity))
	}
	return domainmodeladmin.ModelAccountModelListPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *ModelAdminStore) GetModelAccountModel(ctx context.Context, accountModelID int64) (domainmodeladmin.ModelAccountModel, error) {
	entity, err := s.client.ModelAccountModel.Query().Where(modelaccountmodel.IDEQ(int(accountModelID)), modelaccountmodel.DeletedAtIsNil()).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainmodeladmin.ModelAccountModel{}, repoerr.ErrNotFound
		}
		return domainmodeladmin.ModelAccountModel{}, err
	}
	return s.mapModelAccountModel(ctx, entity), nil
}

func (s *ModelAdminStore) CreateModelAccountModel(ctx context.Context, req domainmodeladmin.ModelAccountModelWriteRequest) (domainmodeladmin.ModelAccountModel, error) {
	entity, err := s.client.ModelAccountModel.Create().
		SetAccountID(req.AccountID).
		SetModelCode(req.ModelCode).
		SetDisplayName(req.DisplayName).
		SetTaskTypes(req.TaskTypes).
		SetBaseResolution(req.BaseResolution).
		SetQuality(req.Quality).
		SetMaxReferenceImageCount(req.MaxReferenceImageCount).
		SetMaxImageCount(req.MaxImageCount).
		SetSizeModes(req.SizeModes).
		SetSupportedRatios(req.SupportedRatios).
		SetSupportedPixelSizes(req.SupportedPixelSizes).
		SetSupportsCustomRatio(req.SupportsCustomRatio).
		SetSupportedBackgrounds(req.SupportedBackgrounds).
		SetOutputFormat(req.OutputFormat).
		SetOutputCompression(req.OutputCompression).
		SetSupportsOutputCompression(req.SupportsOutputCompression).
		SetSupportsCustomSize(req.SupportsCustomSize).
		SetMinWidth(req.MinWidth).
		SetMaxWidth(req.MaxWidth).
		SetMinHeight(req.MinHeight).
		SetMaxHeight(req.MaxHeight).
		SetModeration(req.Moderation).
		SetCostPerImage(req.CostPerImage).
		SetCurrency(req.Currency).
		SetEnabled(req.Enabled).
		SetExtra(req.Extra).
		Save(ctx)
	if err != nil {
		if repoent.IsConstraintError(err) {
			return domainmodeladmin.ModelAccountModel{}, repoerr.ErrConflict
		}
		return domainmodeladmin.ModelAccountModel{}, err
	}
	return s.mapModelAccountModel(ctx, entity), nil
}

func (s *ModelAdminStore) UpdateModelAccountModel(ctx context.Context, accountModelID int64, req domainmodeladmin.ModelAccountModelWriteRequest) (domainmodeladmin.ModelAccountModel, error) {
	entity, err := s.client.ModelAccountModel.UpdateOneID(int(accountModelID)).
		SetAccountID(req.AccountID).
		SetModelCode(req.ModelCode).
		SetDisplayName(req.DisplayName).
		SetTaskTypes(req.TaskTypes).
		SetBaseResolution(req.BaseResolution).
		SetQuality(req.Quality).
		SetMaxReferenceImageCount(req.MaxReferenceImageCount).
		SetMaxImageCount(req.MaxImageCount).
		SetSizeModes(req.SizeModes).
		SetSupportedRatios(req.SupportedRatios).
		SetSupportedPixelSizes(req.SupportedPixelSizes).
		SetSupportsCustomRatio(req.SupportsCustomRatio).
		SetSupportedBackgrounds(req.SupportedBackgrounds).
		SetOutputFormat(req.OutputFormat).
		SetOutputCompression(req.OutputCompression).
		SetSupportsOutputCompression(req.SupportsOutputCompression).
		SetSupportsCustomSize(req.SupportsCustomSize).
		SetMinWidth(req.MinWidth).
		SetMaxWidth(req.MaxWidth).
		SetMinHeight(req.MinHeight).
		SetMaxHeight(req.MaxHeight).
		SetModeration(req.Moderation).
		SetCostPerImage(req.CostPerImage).
		SetCurrency(req.Currency).
		SetEnabled(req.Enabled).
		SetExtra(req.Extra).
		Save(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainmodeladmin.ModelAccountModel{}, repoerr.ErrNotFound
		}
		return domainmodeladmin.ModelAccountModel{}, err
	}
	return s.mapModelAccountModel(ctx, entity), nil
}

func (s *ModelAdminStore) DeleteModelAccountModel(ctx context.Context, accountModelID int64) error {
	count, err := s.client.RouteModelCandidate.Query().Where(routemodelcandidate.AccountModelIDEQ(accountModelID)).Count(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return repoerr.ErrConflict
	}
	affected, err := s.client.ModelAccountModel.Update().Where(modelaccountmodel.IDEQ(int(accountModelID)), modelaccountmodel.DeletedAtIsNil()).SetDeletedAt(time.Now().UTC()).Save(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

func (s *ModelAdminStore) ListRouteModels(ctx context.Context, req domainmodeladmin.RouteModelListRequest) (domainmodeladmin.RouteModelListPage, error) {
	page, pageSize := normalizeModelAdminPage(req.Page, req.PageSize)
	query := s.client.RouteModel.Query().Where(routemodel.DeletedAtIsNil())
	if req.Visibility != "" {
		query = query.Where(routemodel.VisibilityEQ(req.Visibility))
	}
	if req.Enabled != nil {
		query = query.Where(routemodel.EnabledEQ(*req.Enabled))
	}
	total, err := query.Count(ctx)
	if err != nil {
		return domainmodeladmin.RouteModelListPage{}, err
	}
	entities, err := query.Order(repoent.Asc(routemodel.FieldSortOrder), repoent.Asc(routemodel.FieldCode)).Offset((page - 1) * pageSize).Limit(pageSize).All(ctx)
	if err != nil {
		return domainmodeladmin.RouteModelListPage{}, err
	}
	items := make([]domainmodeladmin.RouteModel, 0, len(entities))
	for _, entity := range entities {
		items = append(items, mapRouteModel(entity, nil))
	}
	return domainmodeladmin.RouteModelListPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *ModelAdminStore) GetRouteModel(ctx context.Context, routeModelID int64) (domainmodeladmin.RouteModel, error) {
	entity, err := s.client.RouteModel.Query().Where(routemodel.IDEQ(int(routeModelID)), routemodel.DeletedAtIsNil()).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainmodeladmin.RouteModel{}, repoerr.ErrNotFound
		}
		return domainmodeladmin.RouteModel{}, err
	}
	return mapRouteModel(entity, nil), nil
}

func (s *ModelAdminStore) CreateRouteModel(ctx context.Context, req domainmodeladmin.RouteModelWriteRequest) (domainmodeladmin.RouteModel, error) {
	entity, err := s.client.RouteModel.Create().
		SetCode(req.Code).
		SetName(req.Name).
		SetDescription(req.Description).
		SetVisibility(req.Visibility).
		SetEnabled(req.Enabled).
		SetSortOrder(req.SortOrder).
		Save(ctx)
	if err != nil {
		if repoent.IsConstraintError(err) {
			return domainmodeladmin.RouteModel{}, repoerr.ErrConflict
		}
		return domainmodeladmin.RouteModel{}, err
	}
	if err := s.replaceRouteVisibilityGroups(ctx, int64(entity.ID), req.GroupIDs); err != nil {
		return domainmodeladmin.RouteModel{}, err
	}
	return mapRouteModel(entity, req.GroupIDs), nil
}

func (s *ModelAdminStore) UpdateRouteModel(ctx context.Context, routeModelID int64, req domainmodeladmin.RouteModelWriteRequest) (domainmodeladmin.RouteModel, error) {
	entity, err := s.client.RouteModel.UpdateOneID(int(routeModelID)).
		SetCode(req.Code).
		SetName(req.Name).
		SetDescription(req.Description).
		SetVisibility(req.Visibility).
		SetEnabled(req.Enabled).
		SetSortOrder(req.SortOrder).
		Save(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainmodeladmin.RouteModel{}, repoerr.ErrNotFound
		}
		return domainmodeladmin.RouteModel{}, err
	}
	if err := s.replaceRouteVisibilityGroups(ctx, routeModelID, req.GroupIDs); err != nil {
		return domainmodeladmin.RouteModel{}, err
	}
	return mapRouteModel(entity, req.GroupIDs), nil
}

func (s *ModelAdminStore) replaceRouteVisibilityGroups(ctx context.Context, routeModelID int64, groupIDs []int64) error {
	if _, err := s.client.RouteModelVisibilityGroup.Delete().Where(routemodelvisibilitygroup.RouteModelIDEQ(routeModelID)).Exec(ctx); err != nil {
		return err
	}
	for _, groupID := range groupIDs {
		if groupID <= 0 {
			continue
		}
		if _, err := s.client.RouteModelVisibilityGroup.Create().SetRouteModelID(routeModelID).SetGroupID(groupID).Save(ctx); err != nil {
			if repoent.IsConstraintError(err) {
				continue
			}
			return err
		}
	}
	return nil
}

func (s *ModelAdminStore) DeleteRouteModel(ctx context.Context, routeModelID int64) error {
	affected, err := s.client.RouteModel.Update().Where(routemodel.IDEQ(int(routeModelID)), routemodel.DeletedAtIsNil()).SetDeletedAt(time.Now().UTC()).Save(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

func (s *ModelAdminStore) ListRouteModelCandidates(ctx context.Context, routeModelID int64) ([]domainmodeladmin.RouteModelCandidate, error) {
	query := s.client.RouteModelCandidate.Query()
	if routeModelID > 0 {
		query = query.Where(routemodelcandidate.RouteModelIDEQ(routeModelID))
	}
	entities, err := query.Order(repoent.Asc(routemodelcandidate.FieldPriority), repoent.Asc(routemodelcandidate.FieldFallbackOrder)).All(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]domainmodeladmin.RouteModelCandidate, 0, len(entities))
	for _, entity := range entities {
		items = append(items, s.mapRouteModelCandidate(ctx, entity))
	}
	return items, nil
}

func (s *ModelAdminStore) CreateRouteModelCandidate(ctx context.Context, req domainmodeladmin.RouteModelCandidateWriteRequest) (domainmodeladmin.RouteModelCandidate, error) {
	entity, err := s.client.RouteModelCandidate.Create().SetRouteModelID(req.RouteModelID).SetAccountModelID(req.AccountModelID).SetPriority(req.Priority).SetWeight(req.Weight).SetFallbackOrder(req.FallbackOrder).SetEnabled(req.Enabled).Save(ctx)
	if err != nil {
		if repoent.IsConstraintError(err) {
			return domainmodeladmin.RouteModelCandidate{}, repoerr.ErrConflict
		}
		return domainmodeladmin.RouteModelCandidate{}, err
	}
	return s.mapRouteModelCandidate(ctx, entity), nil
}

func (s *ModelAdminStore) UpdateRouteModelCandidate(ctx context.Context, candidateID int64, req domainmodeladmin.RouteModelCandidateWriteRequest) (domainmodeladmin.RouteModelCandidate, error) {
	entity, err := s.client.RouteModelCandidate.UpdateOneID(int(candidateID)).SetRouteModelID(req.RouteModelID).SetAccountModelID(req.AccountModelID).SetPriority(req.Priority).SetWeight(req.Weight).SetFallbackOrder(req.FallbackOrder).SetEnabled(req.Enabled).Save(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainmodeladmin.RouteModelCandidate{}, repoerr.ErrNotFound
		}
		return domainmodeladmin.RouteModelCandidate{}, err
	}
	return s.mapRouteModelCandidate(ctx, entity), nil
}

func (s *ModelAdminStore) DeleteRouteModelCandidate(ctx context.Context, candidateID int64) error {
	if err := s.client.RouteModelCandidate.DeleteOneID(int(candidateID)).Exec(ctx); err != nil {
		if repoent.IsNotFound(err) {
			return repoerr.ErrNotFound
		}
		return err
	}
	return nil
}

func (s *ModelAdminStore) ListRouteModelPrices(ctx context.Context, req domainmodeladmin.RouteModelPriceListRequest) (domainmodeladmin.RouteModelPriceListPage, error) {
	page, pageSize := normalizeModelAdminPage(req.Page, req.PageSize)
	query := s.client.RouteModelPrice.Query()
	if req.RouteModelID > 0 {
		query = query.Where(routemodelprice.RouteModelIDEQ(req.RouteModelID))
	}
	if req.TaskType != "" {
		query = query.Where(routemodelprice.TaskTypeEQ(req.TaskType))
	}
	if req.BaseResolution != "" {
		query = query.Where(routemodelprice.BaseResolutionEQ(req.BaseResolution))
	}
	if req.Enabled != nil {
		query = query.Where(routemodelprice.EnabledEQ(*req.Enabled))
	}
	total, err := query.Count(ctx)
	if err != nil {
		return domainmodeladmin.RouteModelPriceListPage{}, err
	}
	entities, err := query.Order(repoent.Asc(routemodelprice.FieldRouteModelID), repoent.Asc(routemodelprice.FieldTaskType), repoent.Asc(routemodelprice.FieldBaseResolution)).Offset((page - 1) * pageSize).Limit(pageSize).All(ctx)
	if err != nil {
		return domainmodeladmin.RouteModelPriceListPage{}, err
	}
	items := make([]domainmodeladmin.RouteModelPrice, 0, len(entities))
	for _, entity := range entities {
		items = append(items, s.mapRouteModelPrice(ctx, entity))
	}
	return domainmodeladmin.RouteModelPriceListPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *ModelAdminStore) CreateRouteModelPrice(ctx context.Context, req domainmodeladmin.RouteModelPriceWriteRequest) (domainmodeladmin.RouteModelPrice, error) {
	entity, err := s.client.RouteModelPrice.Create().SetRouteModelID(req.RouteModelID).SetTaskType(req.TaskType).SetBaseResolution(req.BaseResolution).SetBasePoints(req.BasePoints).SetReferenceMultiplier(req.ReferenceMultiplier).SetEnabled(req.Enabled).Save(ctx)
	if err != nil {
		if repoent.IsConstraintError(err) {
			return domainmodeladmin.RouteModelPrice{}, repoerr.ErrConflict
		}
		return domainmodeladmin.RouteModelPrice{}, err
	}
	return s.mapRouteModelPrice(ctx, entity), nil
}

func (s *ModelAdminStore) UpdateRouteModelPrice(ctx context.Context, priceID int64, req domainmodeladmin.RouteModelPriceWriteRequest) (domainmodeladmin.RouteModelPrice, error) {
	entity, err := s.client.RouteModelPrice.UpdateOneID(int(priceID)).SetRouteModelID(req.RouteModelID).SetTaskType(req.TaskType).SetBaseResolution(req.BaseResolution).SetBasePoints(req.BasePoints).SetReferenceMultiplier(req.ReferenceMultiplier).SetEnabled(req.Enabled).Save(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainmodeladmin.RouteModelPrice{}, repoerr.ErrNotFound
		}
		return domainmodeladmin.RouteModelPrice{}, err
	}
	return s.mapRouteModelPrice(ctx, entity), nil
}

func (s *ModelAdminStore) DeleteRouteModelPrice(ctx context.Context, priceID int64) error {
	if err := s.client.RouteModelPrice.DeleteOneID(int(priceID)).Exec(ctx); err != nil {
		if repoent.IsNotFound(err) {
			return repoerr.ErrNotFound
		}
		return err
	}
	return nil
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
		SetSupportedBaseResolution(req.SupportedBaseResolution).
		SetQuality(req.Quality).
		SetSupportedRatios(req.SupportedRatios).
		SetOutputFormat(req.OutputFormat).
		SetOutputCompression(req.OutputCompression).
		SetModeration(req.Moderation).
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
		SetSupportedBaseResolution(req.SupportedBaseResolution).
		SetQuality(req.Quality).
		SetSupportedRatios(req.SupportedRatios).
		SetOutputFormat(req.OutputFormat).
		SetOutputCompression(req.OutputCompression).
		SetModeration(req.Moderation).
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
	newSnapshot, err := s.newModelRoutingConfig(ctx)
	if err != nil {
		return modelhub.ModelRoutingSnapshot{}, err
	}
	if len(newSnapshot.RouteModels) > 0 || len(newSnapshot.ProviderModels) > 0 {
		return newSnapshot, nil
	}
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
			ProviderModelID:         int64(providerModel.ID),
			Provider:                providerCode,
			ModelCode:               providerModel.ModelCode,
			CompatMode:              providerModel.CompatMode,
			SupportedTaskTypes:      nil,
			SupportedBaseResolution: append([]string(nil), providerModel.SupportedBaseResolution...),
			SupportedAspectRatios:   append([]string(nil), providerModel.SupportedRatios...),
			MaxImageCount:           providerModel.MaxImageCount,
			MaxReferenceImageCount:  providerModel.MaxReferenceImageCount,
			SupportsImageInput:      providerModel.SupportsImageInput,
			SupportsMask:            providerModel.SupportsMask,
			InputCost:               providerModel.InputCost,
			OutputCost:              providerModel.OutputCost,
			Currency:                providerModel.Currency,
			HealthStatus:            providerModel.HealthStatus,
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

func (s *ModelAdminStore) newModelRoutingConfig(ctx context.Context) (modelhub.ModelRoutingSnapshot, error) {
	accounts, err := s.client.ModelAccount.Query().Where(modelaccount.DeletedAtIsNil()).All(ctx)
	if err != nil {
		return modelhub.ModelRoutingSnapshot{}, err
	}
	accountByID := make(map[int64]*repoent.ModelAccount, len(accounts))
	for _, account := range accounts {
		accountByID[int64(account.ID)] = account
	}
	accountModels, err := s.client.ModelAccountModel.Query().Where(modelaccountmodel.DeletedAtIsNil()).All(ctx)
	if err != nil {
		return modelhub.ModelRoutingSnapshot{}, err
	}
	routeModels, err := s.client.RouteModel.Query().Where(routemodel.DeletedAtIsNil()).All(ctx)
	if err != nil {
		return modelhub.ModelRoutingSnapshot{}, err
	}
	candidates, err := s.client.RouteModelCandidate.Query().All(ctx)
	if err != nil {
		return modelhub.ModelRoutingSnapshot{}, err
	}
	prices, err := s.client.RouteModelPrice.Query().All(ctx)
	if err != nil {
		return modelhub.ModelRoutingSnapshot{}, err
	}
	groups, err := s.client.UserGroup.Query().All(ctx)
	if err != nil {
		return modelhub.ModelRoutingSnapshot{}, err
	}
	visibility, err := s.client.RouteModelVisibilityGroup.Query().All(ctx)
	if err != nil {
		return modelhub.ModelRoutingSnapshot{}, err
	}
	snapshot := modelhub.ModelRoutingSnapshot{
		ProviderModels: make([]modelhub.ProviderCandidate, 0, len(accountModels)),
		RouteModels:    make([]modelhub.RouteModelConfig, 0, len(routeModels)),
		Candidates:     make([]modelhub.RouteCandidateConfig, 0, len(candidates)),
		Prices:         make([]modelhub.RoutePriceConfig, 0, len(prices)),
		Groups:         make([]modelhub.UserGroupConfig, 0, len(groups)),
		Visibility:     make([]modelhub.RouteVisibilityConfig, 0, len(visibility)),
	}
	latestVersionAt := time.Time{}
	for _, model := range accountModels {
		account := accountByID[model.AccountID]
		if account == nil || !model.Enabled || account.Status != domainmodeladmin.ModelAccountStatusEnabled {
			continue
		}
		if model.UpdatedAt.After(latestVersionAt) {
			latestVersionAt = model.UpdatedAt
		}
		maxImageCount := model.MaxImageCount
		if maxImageCount <= 0 {
			maxImageCount = 1
		}
		snapshot.ProviderModels = append(snapshot.ProviderModels, modelhub.ProviderCandidate{
			AccountModelID:            int64(model.ID),
			ModelAccountID:            model.AccountID,
			Provider:                  account.AdapterType,
			AdapterType:               account.AdapterType,
			AuthType:                  account.AuthType,
			BaseURL:                   account.BaseURL,
			Credentials:               account.CredentialsEncrypted,
			ModelCode:                 model.ModelCode,
			SupportedTaskTypes:        append([]string(nil), model.TaskTypes...),
			SupportedBaseResolution:   append([]string(nil), model.BaseResolution...),
			Quality:                   append([]string(nil), model.Quality...),
			SizeModes:                 append([]string(nil), model.SizeModes...),
			SupportedAspectRatios:     append([]string(nil), model.SupportedRatios...),
			SupportedPixelSizes:       append([]string(nil), model.SupportedPixelSizes...),
			SupportsCustomRatio:       model.SupportsCustomRatio,
			SupportedBackgrounds:      append([]string(nil), model.SupportedBackgrounds...),
			MaxImageCount:             maxImageCount,
			ConcurrencyLimit:          account.ConcurrencyLimit,
			MaxReferenceImageCount:    model.MaxReferenceImageCount,
			SupportsImageInput:        model.MaxReferenceImageCount > 0,
			OutputFormat:              append([]string(nil), model.OutputFormat...),
			OutputCompression:         model.OutputCompression,
			SupportsOutputCompression: model.SupportsOutputCompression,
			SupportsCustomSize:        model.SupportsCustomSize,
			MinWidth:                  model.MinWidth,
			MaxWidth:                  normalizeLegacyModelAccountMaxBound(model.MaxWidth),
			MinHeight:                 model.MinHeight,
			MaxHeight:                 normalizeLegacyModelAccountMaxBound(model.MaxHeight),
			Moderation:                append([]string(nil), model.Moderation...),
			HealthStatus:              account.Status,
			TimeoutMS:                 account.TimeoutMs,
			OutputCost:                model.CostPerImage,
			Currency:                  model.Currency,
			AccountExtra:              cloneModelAdminExtra(account.Extra),
			ModelExtra:                cloneModelAdminExtra(model.Extra),
		})
	}
	for _, model := range routeModels {
		if model.UpdatedAt.After(latestVersionAt) {
			latestVersionAt = model.UpdatedAt
		}
		snapshot.RouteModels = append(snapshot.RouteModels, modelhub.RouteModelConfig{ID: int64(model.ID), Code: model.Code, Name: model.Name, Description: model.Description, Visibility: model.Visibility, Enabled: model.Enabled, SortOrder: model.SortOrder})
	}
	for _, candidate := range candidates {
		snapshot.Candidates = append(snapshot.Candidates, modelhub.RouteCandidateConfig{ID: int64(candidate.ID), RouteModelID: candidate.RouteModelID, AccountModelID: candidate.AccountModelID, Priority: candidate.Priority, Weight: candidate.Weight, FallbackOrder: candidate.FallbackOrder, Enabled: candidate.Enabled})
	}
	for _, price := range prices {
		snapshot.Prices = append(snapshot.Prices, modelhub.RoutePriceConfig{ID: int64(price.ID), RouteModelID: price.RouteModelID, TaskType: price.TaskType, BaseResolution: price.BaseResolution, BasePoints: price.BasePoints, ReferenceMultiplier: price.ReferenceMultiplier, Enabled: price.Enabled})
	}
	for _, group := range groups {
		snapshot.Groups = append(snapshot.Groups, modelhub.UserGroupConfig{ID: int64(group.ID), Code: group.GroupCode, Name: group.GroupName, Multiplier: group.Multiplier, Status: group.Status})
	}
	for _, item := range visibility {
		snapshot.Visibility = append(snapshot.Visibility, modelhub.RouteVisibilityConfig{RouteModelID: item.RouteModelID, GroupID: item.GroupID})
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

func mapModelAccount(entity *repoent.ModelAccount) domainmodeladmin.ModelAccount {
	credentials := entity.CredentialsEncrypted
	return domainmodeladmin.ModelAccount{
		ID:                     int64(entity.ID),
		Name:                   entity.Name,
		AdapterType:            entity.AdapterType,
		AuthType:               entity.AuthType,
		BaseURL:                entity.BaseURL,
		CredentialsStatus:      domainmodeladmin.CredentialsStatus{HasAPIKey: strings.TrimSpace(credentials["api_key"]) != ""},
		CredentialsEncrypted:   credentials,
		CredentialsFingerprint: entity.CredentialsFingerprint,
		Status:                 entity.Status,
		Priority:               entity.Priority,
		Weight:                 entity.Weight,
		ConcurrencyLimit:       entity.ConcurrencyLimit,
		TimeoutMS:              entity.TimeoutMs,
		ErrorMessage:           stringValue(entity.ErrorMessage),
		LastUsedAt:             entity.LastUsedAt,
		Extra:                  entity.Extra,
		CreatedAt:              entity.CreatedAt,
		UpdatedAt:              entity.UpdatedAt,
	}
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

func (s *ModelAdminStore) mapModelAccountModel(ctx context.Context, entity *repoent.ModelAccountModel) domainmodeladmin.ModelAccountModel {
	accountName := ""
	if account, err := s.client.ModelAccount.Get(ctx, int(entity.AccountID)); err == nil {
		accountName = account.Name
	}
	return domainmodeladmin.ModelAccountModel{
		ID:                        int64(entity.ID),
		AccountID:                 entity.AccountID,
		AccountName:               accountName,
		ModelCode:                 entity.ModelCode,
		DisplayName:               entity.DisplayName,
		TaskTypes:                 append([]string(nil), entity.TaskTypes...),
		BaseResolution:            append([]string(nil), entity.BaseResolution...),
		Quality:                   append([]string(nil), entity.Quality...),
		MaxReferenceImageCount:    entity.MaxReferenceImageCount,
		MaxImageCount:             entity.MaxImageCount,
		SizeModes:                 append([]string(nil), entity.SizeModes...),
		SupportedRatios:           append([]string(nil), entity.SupportedRatios...),
		SupportedPixelSizes:       append([]string(nil), entity.SupportedPixelSizes...),
		SupportsCustomRatio:       entity.SupportsCustomRatio,
		SupportedBackgrounds:      append([]string(nil), entity.SupportedBackgrounds...),
		OutputFormat:              append([]string(nil), entity.OutputFormat...),
		OutputCompression:         entity.OutputCompression,
		SupportsOutputCompression: entity.SupportsOutputCompression,
		SupportsCustomSize:        entity.SupportsCustomSize,
		MinWidth:                  entity.MinWidth,
		MaxWidth:                  normalizeLegacyModelAccountMaxBound(entity.MaxWidth),
		MinHeight:                 entity.MinHeight,
		MaxHeight:                 normalizeLegacyModelAccountMaxBound(entity.MaxHeight),
		Moderation:                append([]string(nil), entity.Moderation...),
		CostPerImage:              entity.CostPerImage,
		Currency:                  entity.Currency,
		Enabled:                   entity.Enabled,
		Extra:                     entity.Extra,
		CreatedAt:                 entity.CreatedAt,
		UpdatedAt:                 entity.UpdatedAt,
	}
}

func normalizeLegacyModelAccountMaxBound(value int) int {
	if value == 4096 {
		return 3840
	}
	return value
}

func mapRouteModel(entity *repoent.RouteModel, groupIDs []int64) domainmodeladmin.RouteModel {
	return domainmodeladmin.RouteModel{
		ID:          int64(entity.ID),
		Code:        entity.Code,
		Name:        entity.Name,
		Description: entity.Description,
		Visibility:  entity.Visibility,
		Enabled:     entity.Enabled,
		SortOrder:   entity.SortOrder,
		GroupIDs:    append([]int64(nil), groupIDs...),
		CreatedAt:   entity.CreatedAt,
		UpdatedAt:   entity.UpdatedAt,
	}
}

func (s *ModelAdminStore) mapRouteModelCandidate(ctx context.Context, entity *repoent.RouteModelCandidate) domainmodeladmin.RouteModelCandidate {
	item := domainmodeladmin.RouteModelCandidate{ID: int64(entity.ID), RouteModelID: entity.RouteModelID, AccountModelID: entity.AccountModelID, Priority: entity.Priority, Weight: entity.Weight, FallbackOrder: entity.FallbackOrder, Enabled: entity.Enabled}
	if model, err := s.client.ModelAccountModel.Get(ctx, int(entity.AccountModelID)); err == nil {
		item.AccountID = model.AccountID
		item.ModelCode = model.ModelCode
		if account, err := s.client.ModelAccount.Get(ctx, int(model.AccountID)); err == nil {
			item.AccountName = account.Name
		}
	}
	return item
}

func (s *ModelAdminStore) mapRouteModelPrice(ctx context.Context, entity *repoent.RouteModelPrice) domainmodeladmin.RouteModelPrice {
	item := domainmodeladmin.RouteModelPrice{ID: int64(entity.ID), RouteModelID: entity.RouteModelID, TaskType: entity.TaskType, BaseResolution: entity.BaseResolution, BasePoints: entity.BasePoints, ReferenceMultiplier: entity.ReferenceMultiplier, Enabled: entity.Enabled}
	if routeModel, err := s.client.RouteModel.Get(ctx, int(entity.RouteModelID)); err == nil {
		item.RouteModelCode = routeModel.Code
	}
	return item
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
		ID:                      int64(entity.ID),
		ProviderID:              entity.ProviderID,
		ProviderCode:            providerCode,
		ModelCode:               entity.ModelCode,
		CompatMode:              entity.CompatMode,
		SupportsImageInput:      entity.SupportsImageInput,
		SupportsMask:            entity.SupportsMask,
		SupportedBaseResolution: append([]string(nil), entity.SupportedBaseResolution...),
		Quality:                 append([]string(nil), entity.Quality...),
		SupportedRatios:         append([]string(nil), entity.SupportedRatios...),
		OutputFormat:            append([]string(nil), entity.OutputFormat...),
		OutputCompression:       entity.OutputCompression,
		Moderation:              append([]string(nil), entity.Moderation...),
		MaxImageCount:           entity.MaxImageCount,
		MaxReferenceImageCount:  entity.MaxReferenceImageCount,
		TimeoutMS:               entity.TimeoutMs,
		InputCost:               entity.InputCost,
		OutputCost:              entity.OutputCost,
		Currency:                entity.Currency,
		HealthStatus:            entity.HealthStatus,
		LastHealthCheckedAt:     entity.LastHealthCheckedAt,
		Enabled:                 entity.Enabled,
		CreatedAt:               entity.CreatedAt,
		UpdatedAt:               entity.UpdatedAt,
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
