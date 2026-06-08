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

func (s *Service) ListModelAccounts(ctx context.Context, req domainmodeladmin.ModelAccountListRequest) (domainmodeladmin.ModelAccountListPage, error) {
	req.Page, req.PageSize = normalizePage(req.Page, req.PageSize)
	req.AdapterType = normalizeCode(req.AdapterType)
	req.AuthType = normalizeCode(req.AuthType)
	req.Status = normalizeCode(req.Status)
	return s.store.ListModelAccounts(ctx, req)
}

func (s *Service) GetModelAccount(ctx context.Context, accountID int64) (domainmodeladmin.ModelAccount, error) {
	if accountID <= 0 {
		return domainmodeladmin.ModelAccount{}, errs.BadRequest("invalid account_id")
	}
	item, err := s.store.GetModelAccount(ctx, accountID)
	return item, normalizeStoreError(err, "model account not found")
}

func (s *Service) CreateModelAccount(ctx context.Context, req domainmodeladmin.ModelAccountWriteRequest) (domainmodeladmin.ModelAccount, error) {
	normalized, err := normalizeModelAccountWrite(req, true)
	if err != nil {
		return domainmodeladmin.ModelAccount{}, err
	}
	item, err := s.store.CreateModelAccount(ctx, normalized)
	return item, normalizeStoreError(err, "model account already exists")
}

func (s *Service) UpdateModelAccount(ctx context.Context, accountID int64, req domainmodeladmin.ModelAccountWriteRequest) (domainmodeladmin.ModelAccount, error) {
	if accountID <= 0 {
		return domainmodeladmin.ModelAccount{}, errs.BadRequest("invalid account_id")
	}
	normalized, err := normalizeModelAccountWrite(req, false)
	if err != nil {
		return domainmodeladmin.ModelAccount{}, err
	}
	item, err := s.store.UpdateModelAccount(ctx, accountID, normalized)
	return item, normalizeStoreError(err, "model account not found")
}

func (s *Service) DeleteModelAccount(ctx context.Context, accountID int64) error {
	if accountID <= 0 {
		return errs.BadRequest("invalid account_id")
	}
	return normalizeStoreError(s.store.DeleteModelAccount(ctx, accountID), "model account not found")
}

func (s *Service) ListModelAccountModels(ctx context.Context, req domainmodeladmin.ModelAccountModelListRequest) (domainmodeladmin.ModelAccountModelListPage, error) {
	req.Page, req.PageSize = normalizePage(req.Page, req.PageSize)
	req.ModelCode = strings.TrimSpace(req.ModelCode)
	return s.store.ListModelAccountModels(ctx, req)
}

func (s *Service) GetModelAccountModel(ctx context.Context, accountModelID int64) (domainmodeladmin.ModelAccountModel, error) {
	if accountModelID <= 0 {
		return domainmodeladmin.ModelAccountModel{}, errs.BadRequest("invalid account_model_id")
	}
	item, err := s.store.GetModelAccountModel(ctx, accountModelID)
	return item, normalizeStoreError(err, "model account model not found")
}

func (s *Service) CreateModelAccountModel(ctx context.Context, req domainmodeladmin.ModelAccountModelWriteRequest) (domainmodeladmin.ModelAccountModel, error) {
	normalized, err := normalizeModelAccountModelWrite(req)
	if err != nil {
		return domainmodeladmin.ModelAccountModel{}, err
	}
	item, err := s.store.CreateModelAccountModel(ctx, normalized)
	return item, normalizeStoreError(err, "model account model already exists")
}

func (s *Service) UpdateModelAccountModel(ctx context.Context, accountModelID int64, req domainmodeladmin.ModelAccountModelWriteRequest) (domainmodeladmin.ModelAccountModel, error) {
	if accountModelID <= 0 {
		return domainmodeladmin.ModelAccountModel{}, errs.BadRequest("invalid account_model_id")
	}
	normalized, err := normalizeModelAccountModelWrite(req)
	if err != nil {
		return domainmodeladmin.ModelAccountModel{}, err
	}
	item, err := s.store.UpdateModelAccountModel(ctx, accountModelID, normalized)
	return item, normalizeStoreError(err, "model account model not found")
}

func (s *Service) DeleteModelAccountModel(ctx context.Context, accountModelID int64) error {
	if accountModelID <= 0 {
		return errs.BadRequest("invalid account_model_id")
	}
	return normalizeStoreError(s.store.DeleteModelAccountModel(ctx, accountModelID), "model account model not found")
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

func (s *Service) ListProviderModels(ctx context.Context, req domainmodeladmin.ProviderModelListRequest) (domainmodeladmin.ProviderModelListPage, error) {
	req.Page, req.PageSize = normalizePage(req.Page, req.PageSize)
	req.ProviderCode = normalizeCode(req.ProviderCode)
	req.ModelCode = normalizeCode(req.ModelCode)
	return s.store.ListProviderModels(ctx, req)
}

func (s *Service) GetProviderModel(ctx context.Context, providerModelID int64) (domainmodeladmin.ProviderModel, error) {
	if providerModelID <= 0 {
		return domainmodeladmin.ProviderModel{}, errs.BadRequest("invalid provider_model_id")
	}
	item, err := s.store.GetProviderModel(ctx, providerModelID)
	return item, normalizeStoreError(err, "provider model not found")
}

func (s *Service) CreateProviderModel(ctx context.Context, req domainmodeladmin.ProviderModelWriteRequest) (domainmodeladmin.ProviderModel, error) {
	normalized, err := normalizeProviderModelWrite(req)
	if err != nil {
		return domainmodeladmin.ProviderModel{}, err
	}
	item, err := s.store.CreateProviderModel(ctx, normalized)
	return item, normalizeStoreError(err, "provider model already exists")
}

func (s *Service) UpdateProviderModel(ctx context.Context, providerModelID int64, req domainmodeladmin.ProviderModelWriteRequest) (domainmodeladmin.ProviderModel, error) {
	if providerModelID <= 0 {
		return domainmodeladmin.ProviderModel{}, errs.BadRequest("invalid provider_model_id")
	}
	normalized, err := normalizeProviderModelWrite(req)
	if err != nil {
		return domainmodeladmin.ProviderModel{}, err
	}
	item, err := s.store.UpdateProviderModel(ctx, providerModelID, normalized)
	return item, normalizeStoreError(err, "provider model not found")
}

func (s *Service) DeleteProviderModel(ctx context.Context, providerModelID int64) error {
	if providerModelID <= 0 {
		return errs.BadRequest("invalid provider_model_id")
	}
	return normalizeStoreError(s.store.DeleteProviderModel(ctx, providerModelID), "provider model not found")
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

func (s *Service) ListRouteModels(ctx context.Context, req domainmodeladmin.RouteModelListRequest) (domainmodeladmin.RouteModelListPage, error) {
	req.Page, req.PageSize = normalizePage(req.Page, req.PageSize)
	req.Visibility = normalizeCode(req.Visibility)
	return s.store.ListRouteModels(ctx, req)
}

func (s *Service) CreateRouteModel(ctx context.Context, req domainmodeladmin.RouteModelWriteRequest) (domainmodeladmin.RouteModel, error) {
	normalized, err := normalizeRouteModelWrite(req, true)
	if err != nil {
		return domainmodeladmin.RouteModel{}, err
	}
	item, err := s.store.CreateRouteModel(ctx, normalized)
	return item, normalizeStoreError(err, "route model already exists")
}

func (s *Service) UpdateRouteModel(ctx context.Context, routeModelID int64, req domainmodeladmin.RouteModelWriteRequest) (domainmodeladmin.RouteModel, error) {
	if routeModelID <= 0 {
		return domainmodeladmin.RouteModel{}, errs.BadRequest("invalid route_model_id")
	}
	normalized, err := normalizeRouteModelWrite(req, false)
	if err != nil {
		return domainmodeladmin.RouteModel{}, err
	}
	item, err := s.store.UpdateRouteModel(ctx, routeModelID, normalized)
	return item, normalizeStoreError(err, "route model not found")
}

func (s *Service) DeleteRouteModel(ctx context.Context, routeModelID int64) error {
	if routeModelID <= 0 {
		return errs.BadRequest("invalid route_model_id")
	}
	return normalizeStoreError(s.store.DeleteRouteModel(ctx, routeModelID), "route model not found")
}

func (s *Service) ListRouteModelCandidates(ctx context.Context, routeModelID int64) ([]domainmodeladmin.RouteModelCandidate, error) {
	return s.store.ListRouteModelCandidates(ctx, routeModelID)
}

func (s *Service) CreateRouteModelCandidate(ctx context.Context, req domainmodeladmin.RouteModelCandidateWriteRequest) (domainmodeladmin.RouteModelCandidate, error) {
	normalized, err := normalizeRouteModelCandidateWrite(req)
	if err != nil {
		return domainmodeladmin.RouteModelCandidate{}, err
	}
	item, err := s.store.CreateRouteModelCandidate(ctx, normalized)
	return item, normalizeStoreError(err, "route model candidate already exists")
}

func (s *Service) UpdateRouteModelCandidate(ctx context.Context, candidateID int64, req domainmodeladmin.RouteModelCandidateWriteRequest) (domainmodeladmin.RouteModelCandidate, error) {
	if candidateID <= 0 {
		return domainmodeladmin.RouteModelCandidate{}, errs.BadRequest("invalid candidate_id")
	}
	normalized, err := normalizeRouteModelCandidateWrite(req)
	if err != nil {
		return domainmodeladmin.RouteModelCandidate{}, err
	}
	item, err := s.store.UpdateRouteModelCandidate(ctx, candidateID, normalized)
	return item, normalizeStoreError(err, "route model candidate not found")
}

func (s *Service) DeleteRouteModelCandidate(ctx context.Context, candidateID int64) error {
	if candidateID <= 0 {
		return errs.BadRequest("invalid candidate_id")
	}
	return normalizeStoreError(s.store.DeleteRouteModelCandidate(ctx, candidateID), "route model candidate not found")
}

func (s *Service) ListRouteModelPrices(ctx context.Context, req domainmodeladmin.RouteModelPriceListRequest) (domainmodeladmin.RouteModelPriceListPage, error) {
	req.Page, req.PageSize = normalizePage(req.Page, req.PageSize)
	req.TaskType = normalizeCode(req.TaskType)
	req.Quality = normalizeCode(req.Quality)
	return s.store.ListRouteModelPrices(ctx, req)
}

func (s *Service) CreateRouteModelPrice(ctx context.Context, req domainmodeladmin.RouteModelPriceWriteRequest) (domainmodeladmin.RouteModelPrice, error) {
	normalized, err := normalizeRouteModelPriceWrite(req)
	if err != nil {
		return domainmodeladmin.RouteModelPrice{}, err
	}
	item, err := s.store.CreateRouteModelPrice(ctx, normalized)
	return item, normalizeStoreError(err, "route model price already exists")
}

func (s *Service) UpdateRouteModelPrice(ctx context.Context, priceID int64, req domainmodeladmin.RouteModelPriceWriteRequest) (domainmodeladmin.RouteModelPrice, error) {
	if priceID <= 0 {
		return domainmodeladmin.RouteModelPrice{}, errs.BadRequest("invalid price_id")
	}
	normalized, err := normalizeRouteModelPriceWrite(req)
	if err != nil {
		return domainmodeladmin.RouteModelPrice{}, err
	}
	item, err := s.store.UpdateRouteModelPrice(ctx, priceID, normalized)
	return item, normalizeStoreError(err, "route model price not found")
}

func (s *Service) DeleteRouteModelPrice(ctx context.Context, priceID int64) error {
	if priceID <= 0 {
		return errs.BadRequest("invalid price_id")
	}
	return normalizeStoreError(s.store.DeleteRouteModelPrice(ctx, priceID), "route model price not found")
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

func normalizeModelAccountWrite(req domainmodeladmin.ModelAccountWriteRequest, create bool) (domainmodeladmin.ModelAccountWriteRequest, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.AdapterType = normalizeCode(req.AdapterType)
	req.AuthType = normalizeCode(req.AuthType)
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	req.Status = normalizeCode(req.Status)
	if req.Name == "" || req.AdapterType == "" || req.AuthType == "" || req.BaseURL == "" {
		return domainmodeladmin.ModelAccountWriteRequest{}, errs.BadRequest("name, adapter_type, auth_type and base_url are required")
	}
	if req.AdapterType != domainmodeladmin.AdapterTypeOpenAICompatible && req.AdapterType != domainmodeladmin.AdapterTypeOpenRouter {
		return domainmodeladmin.ModelAccountWriteRequest{}, errs.BadRequest("unsupported adapter_type")
	}
	if req.AuthType != domainmodeladmin.AuthTypeAPIKey {
		return domainmodeladmin.ModelAccountWriteRequest{}, errs.BadRequest("unsupported auth_type")
	}
	if req.Status == "" {
		req.Status = domainmodeladmin.ModelAccountStatusDisabled
	}
	if req.Status != domainmodeladmin.ModelAccountStatusEnabled && req.Status != domainmodeladmin.ModelAccountStatusDisabled && req.Status != domainmodeladmin.ModelAccountStatusError {
		return domainmodeladmin.ModelAccountWriteRequest{}, errs.BadRequest("invalid status")
	}
	if req.Status == domainmodeladmin.ModelAccountStatusEnabled && (req.Credentials == nil || strings.TrimSpace(req.Credentials["api_key"]) == "") && create {
		return domainmodeladmin.ModelAccountWriteRequest{}, errs.BadRequest("api_key credentials are required to enable account")
	}
	if req.Weight <= 0 {
		req.Weight = 100
	}
	if req.ConcurrencyLimit <= 0 {
		req.ConcurrencyLimit = 1
	}
	if req.TimeoutMS <= 0 {
		req.TimeoutMS = 120000
	}
	if req.Extra == nil {
		req.Extra = map[string]any{}
	}
	return req, nil
}

func normalizeModelAccountModelWrite(req domainmodeladmin.ModelAccountModelWriteRequest) (domainmodeladmin.ModelAccountModelWriteRequest, error) {
	req.ModelCode = strings.TrimSpace(req.ModelCode)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
	if req.AccountID <= 0 || req.ModelCode == "" {
		return domainmodeladmin.ModelAccountModelWriteRequest{}, errs.BadRequest("account_id and model_code are required")
	}
	if req.DisplayName == "" {
		req.DisplayName = req.ModelCode
	}
	if req.CostPerImage == "" {
		req.CostPerImage = "0.00000"
	}
	if req.Currency == "" {
		req.Currency = "USD"
	}
	req.TaskTypes = cloneNormalizedStrings(req.TaskTypes)
	req.Qualities = cloneNormalizedStrings(req.Qualities)
	if req.Extra == nil {
		req.Extra = map[string]any{}
	}
	return req, nil
}

func normalizeRouteModelWrite(req domainmodeladmin.RouteModelWriteRequest, requireCode bool) (domainmodeladmin.RouteModelWriteRequest, error) {
	req.Code = normalizeCode(req.Code)
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	req.Visibility = normalizeCode(req.Visibility)
	if requireCode && req.Code == "" {
		return domainmodeladmin.RouteModelWriteRequest{}, errs.BadRequest("code is required")
	}
	if req.Name == "" {
		return domainmodeladmin.RouteModelWriteRequest{}, errs.BadRequest("name is required")
	}
	if req.Visibility == "" {
		req.Visibility = domainmodeladmin.RouteModelVisibilityHidden
	}
	if req.Visibility != domainmodeladmin.RouteModelVisibilityPublic && req.Visibility != domainmodeladmin.RouteModelVisibilityGroups && req.Visibility != domainmodeladmin.RouteModelVisibilityHidden {
		return domainmodeladmin.RouteModelWriteRequest{}, errs.BadRequest("invalid visibility")
	}
	return req, nil
}

func normalizeRouteModelCandidateWrite(req domainmodeladmin.RouteModelCandidateWriteRequest) (domainmodeladmin.RouteModelCandidateWriteRequest, error) {
	if req.RouteModelID <= 0 || req.AccountModelID <= 0 {
		return domainmodeladmin.RouteModelCandidateWriteRequest{}, errs.BadRequest("route_model_id and account_model_id are required")
	}
	if req.Weight <= 0 {
		req.Weight = 100
	}
	return req, nil
}

func normalizeRouteModelPriceWrite(req domainmodeladmin.RouteModelPriceWriteRequest) (domainmodeladmin.RouteModelPriceWriteRequest, error) {
	req.TaskType = normalizeCode(req.TaskType)
	req.Quality = normalizeCode(req.Quality)
	req.BasePoints = strings.TrimSpace(req.BasePoints)
	req.ReferenceMultiplier = strings.TrimSpace(req.ReferenceMultiplier)
	if req.RouteModelID <= 0 || req.TaskType == "" || req.Quality == "" {
		return domainmodeladmin.RouteModelPriceWriteRequest{}, errs.BadRequest("route_model_id, task_type and quality are required")
	}
	if req.BasePoints == "" {
		req.BasePoints = "0.00000"
	}
	if req.ReferenceMultiplier == "" {
		req.ReferenceMultiplier = "1.00000"
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

func normalizeProviderModelWrite(req domainmodeladmin.ProviderModelWriteRequest) (domainmodeladmin.ProviderModelWriteRequest, error) {
	req.ProviderCode = normalizeCode(req.ProviderCode)
	req.ModelCode = normalizeCode(req.ModelCode)
	req.CompatMode = normalizeCode(req.CompatMode)
	req.HealthStatus = normalizeCode(req.HealthStatus)
	req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
	if req.ProviderCode == "" {
		return domainmodeladmin.ProviderModelWriteRequest{}, errs.BadRequest("provider_code is required")
	}
	if req.ModelCode == "" {
		return domainmodeladmin.ProviderModelWriteRequest{}, errs.BadRequest("model_code is required")
	}
	if req.HealthStatus == "" {
		req.HealthStatus = "unknown"
	}
	if req.Currency == "" {
		req.Currency = "CNY"
	}
	if req.MaxImageCount <= 0 {
		req.MaxImageCount = 1
	}
	if req.MaxReferenceImageCount < 0 {
		return domainmodeladmin.ProviderModelWriteRequest{}, errs.BadRequest("max_reference_image_count must be non-negative")
	}
	if req.TimeoutMS <= 0 {
		req.TimeoutMS = 60000
	}
	req.SupportedQualities = cloneNormalizedStrings(req.SupportedQualities)
	req.SupportedRatios = cloneNormalizedStrings(req.SupportedRatios)
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

func cloneNormalizedStrings(input []string) []string {
	if len(input) == 0 {
		return []string{}
	}
	items := make([]string, 0, len(input))
	for _, item := range input {
		trimmed := strings.ToLower(strings.TrimSpace(item))
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
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
