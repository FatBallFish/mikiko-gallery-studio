package modelhub

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type ResolveRequest struct {
	AbstractModel             string
	RouteModelCode            string
	TaskType                  string
	RequestedQuality          string
	RequestedSize             string
	RequestedOutputImageCount int
	ReferenceImageCount       int
	MaskPresent               bool
	RouteKey                  string
	UserGroupCodes            []string
}

func (r *Resolver) ListVisibleRouteModels(ctx context.Context, userGroupCodes []string, taskMultiplierByType map[string]string) ([]VisibleRouteModel, error) {
	routing, err := r.runtimeRouting(ctx)
	if err != nil {
		return nil, err
	}
	if len(routing.RouteModels) == 0 {
		return []VisibleRouteModel{}, nil
	}
	groupsByCode := map[string]UserGroupConfig{}
	activeGroupIDs := map[int64]UserGroupConfig{}
	for _, group := range routing.Groups {
		if !strings.EqualFold(group.Status, "enabled") && !strings.EqualFold(group.Status, "active") {
			continue
		}
		groupsByCode[strings.ToLower(group.Code)] = group
	}
	for _, code := range userGroupCodes {
		if group, ok := groupsByCode[strings.ToLower(strings.TrimSpace(code))]; ok {
			activeGroupIDs[group.ID] = group
		}
	}
	visibility := map[int64][]UserGroupConfig{}
	for _, item := range routing.Visibility {
		if group, ok := activeGroupIDs[item.GroupID]; ok {
			visibility[item.RouteModelID] = append(visibility[item.RouteModelID], group)
		}
	}
	pricesByRoute := map[int64][]RoutePriceConfig{}
	for _, price := range routing.Prices {
		if price.Enabled {
			pricesByRoute[price.RouteModelID] = append(pricesByRoute[price.RouteModelID], price)
		}
	}
	models := make([]RouteModelConfig, 0, len(routing.RouteModels))
	for _, routeModel := range routing.RouteModels {
		if !routeModel.Enabled || routeModel.Visibility == "hidden" {
			continue
		}
		matched := visibility[routeModel.ID]
		if routeModel.Visibility == "groups" && len(matched) == 0 {
			continue
		}
		models = append(models, routeModel)
	}
	sort.SliceStable(models, func(i, j int) bool {
		if models[i].SortOrder == models[j].SortOrder {
			return models[i].Code < models[j].Code
		}
		return models[i].SortOrder < models[j].SortOrder
	})
	visible := make([]VisibleRouteModel, 0, len(models))
	for _, routeModel := range models {
		multiplier, ok := effectiveMultiplier(routeModel, visibility[routeModel.ID])
		if !ok {
			continue
		}
		prices := make([]VisibleRouteModelPrice, 0, len(pricesByRoute[routeModel.ID]))
		taskTypes := map[string]struct{}{}
		qualities := map[string]struct{}{}
		for _, price := range pricesByRoute[routeModel.ID] {
			base, err := decimal.NewFromString(price.BasePoints)
			if err != nil {
				return nil, errs.Internal("invalid route model price")
			}
			taskMul := decimal.NewFromInt(1)
			if raw := strings.TrimSpace(taskMultiplierByType[price.TaskType]); raw != "" {
				parsed, err := decimal.NewFromString(raw)
				if err != nil {
					return nil, errs.Internal("invalid task multiplier")
				}
				taskMul = parsed
			}
			charged := base.Mul(multiplier).Mul(taskMul).Round(5)
			prices = append(prices, VisibleRouteModelPrice{
				TaskType:      price.TaskType,
				Quality:       price.Quality,
				BasePoints:    base.StringFixed(5),
				ChargedPoints: charged.StringFixed(5),
				DisplayPoints: charged.Round(2).StringFixed(2),
			})
			taskTypes[price.TaskType] = struct{}{}
			qualities[price.Quality] = struct{}{}
		}
		aspectRatios, maxOutputCount, maxReferenceCount := r.visibleRouteModelLimits(routeModel, routing)
		visible = append(visible, VisibleRouteModel{
			ID:                     routeModel.ID,
			Code:                   routeModel.Code,
			Name:                   routeModel.Name,
			Description:            routeModel.Description,
			TaskTypes:              sortedSet(taskTypes),
			Qualities:              append([]string{"auto"}, sortedSet(qualities)...),
			AspectRatios:           aspectRatios,
			MaxOutputImageCount:    maxOutputCount,
			MaxReferenceImageCount: maxReferenceCount,
			EffectiveMultiplier:    multiplier.StringFixed(5),
			Prices:                 prices,
		})
	}
	return visible, nil
}

func (r *Resolver) visibleRouteModelLimits(routeModel RouteModelConfig, routing ModelRoutingSnapshot) ([]string, int, int) {
	candidateByID := map[int64]ProviderCandidate{}
	for _, candidate := range routing.ProviderModels {
		candidateByID[candidate.AccountModelID] = candidate
	}
	ratios := map[string]struct{}{}
	maxOutputCount := 0
	maxReferenceCount := 0
	for _, route := range routing.Candidates {
		if !route.Enabled || route.RouteModelID != routeModel.ID {
			continue
		}
		candidate, ok := candidateByID[route.AccountModelID]
		if !ok {
			continue
		}
		for _, ratio := range candidate.SupportedAspectRatios {
			if trimmed := strings.TrimSpace(ratio); trimmed != "" {
				ratios[trimmed] = struct{}{}
			}
		}
		if candidate.MaxImageCount > maxOutputCount {
			maxOutputCount = candidate.MaxImageCount
		}
		if candidate.MaxReferenceImageCount > maxReferenceCount {
			maxReferenceCount = candidate.MaxReferenceImageCount
		}
	}
	if maxOutputCount <= 0 {
		maxOutputCount = r.cfg.GenerationLimits.MaxImageCount
	}
	if maxReferenceCount <= 0 {
		maxReferenceCount = r.cfg.GenerationLimits.ReferenceImageMaxCount
	}
	return sortedSet(ratios), maxOutputCount, maxReferenceCount
}

func effectiveMultiplier(routeModel RouteModelConfig, groups []UserGroupConfig) (decimal.Decimal, bool) {
	values := []decimal.Decimal{}
	if routeModel.Visibility == "public" {
		values = append(values, decimal.NewFromInt(1))
	}
	for _, group := range groups {
		parsed, err := decimal.NewFromString(strings.TrimSpace(group.Multiplier))
		if err != nil {
			continue
		}
		values = append(values, parsed)
	}
	if len(values) == 0 {
		return decimal.Zero, false
	}
	best := values[0]
	for _, value := range values[1:] {
		if value.LessThan(best) {
			best = value
		}
	}
	return best, true
}

func matchedActiveGroups(routing ModelRoutingSnapshot, userGroupCodes []string, routeModelID int64) []UserGroupConfig {
	groupsByCode := map[string]UserGroupConfig{}
	activeGroupIDs := map[int64]UserGroupConfig{}
	for _, group := range routing.Groups {
		if !strings.EqualFold(group.Status, "enabled") && !strings.EqualFold(group.Status, "active") {
			continue
		}
		groupsByCode[strings.ToLower(group.Code)] = group
	}
	for _, code := range userGroupCodes {
		if group, ok := groupsByCode[strings.ToLower(strings.TrimSpace(code))]; ok {
			activeGroupIDs[group.ID] = group
		}
	}
	matched := make([]UserGroupConfig, 0)
	seen := map[int64]struct{}{}
	for _, item := range routing.Visibility {
		if item.RouteModelID != routeModelID {
			continue
		}
		group, ok := activeGroupIDs[item.GroupID]
		if !ok {
			continue
		}
		if _, exists := seen[group.ID]; exists {
			continue
		}
		seen[group.ID] = struct{}{}
		matched = append(matched, group)
	}
	return matched
}

func sortedSet(values map[string]struct{}) []string {
	items := make([]string, 0, len(values))
	for item := range values {
		if item != "" {
			items = append(items, item)
		}
	}
	sort.Strings(items)
	return items
}

type ProviderCandidate struct {
	AccountModelID         int64
	ModelAccountID         int64
	RouteModelID           int64
	RouteModelCode         string
	AdapterType            string
	AuthType               string
	BaseURL                string
	Credentials            map[string]string
	ProviderModelID        int64
	Provider               string
	ModelCode              string
	CompatMode             string
	SupportedTaskTypes     []string
	SupportedQualities     []string
	SupportedAspectRatios  []string
	MaxImageCount          int
	MaxReferenceImageCount int
	SupportsImageInput     bool
	SupportsMask           bool
	Priority               int
	WeightPercent          int
	FallbackOrder          int
	HealthStatus           string
	InputCost              string
	OutputCost             string
	Currency               string
	RouteSnapshotVersion   string
}

type ModelProviderConfig struct {
	ID           int64
	ProviderCode string
	ProviderType string
	Enabled      bool
}

type ModelRouteConfig struct {
	ID              int64
	GroupCode       string
	TaskType        string
	ProviderModelID int64
	ProviderCode    string
	Priority        int
	WeightPercent   int
	FallbackOrder   int
	Enabled         bool
}

type ModelRoutingSnapshot struct {
	Version        string
	Providers      []ModelProviderConfig
	ProviderModels []ProviderCandidate
	Routes         []ModelRouteConfig
	RouteModels    []RouteModelConfig
	Candidates     []RouteCandidateConfig
	Prices         []RoutePriceConfig
	Groups         []UserGroupConfig
	Visibility     []RouteVisibilityConfig
}

type RouteModelConfig struct {
	ID          int64
	Code        string
	Name        string
	Description string
	Visibility  string
	Enabled     bool
	SortOrder   int
}

type RouteCandidateConfig struct {
	ID             int64
	RouteModelID   int64
	AccountModelID int64
	Priority       int
	Weight         int
	FallbackOrder  int
	Enabled        bool
}

type RoutePriceConfig struct {
	ID                  int64
	RouteModelID        int64
	TaskType            string
	Quality             string
	BasePoints          string
	ReferenceMultiplier string
	Enabled             bool
}

type UserGroupConfig struct {
	ID         int64
	Code       string
	Name       string
	Multiplier string
	Status     string
}

type RouteVisibilityConfig struct {
	RouteModelID int64
	GroupID      int64
}

type ModelRoutingSource interface {
	ModelRoutingConfig(ctx context.Context) (ModelRoutingSnapshot, error)
}

type ResolvedRequest struct {
	ResolvedQualityBucket  string
	Providers              []ProviderCandidate
	MaxOutputImageCount    int
	MaxReferenceImageCount int
	RuntimeRoutingApplied  bool
}

type CapabilityItem struct {
	AbstractModel          string
	TaskTypes              []string
	Qualities              []string
	AspectRatios           []string
	MaxOutputImageCount    int
	MaxReferenceImageCount int
}

type VisibleRouteModel struct {
	ID                     int64
	Code                   string
	Name                   string
	Description            string
	TaskTypes              []string
	Qualities              []string
	AspectRatios           []string
	MaxOutputImageCount    int
	MaxReferenceImageCount int
	EffectiveMultiplier    string
	Prices                 []VisibleRouteModelPrice
}

type VisibleRouteModelPrice struct {
	TaskType      string
	Quality       string
	BasePoints    string
	ChargedPoints string
	DisplayPoints string
}

type Resolver struct {
	cfg    config.Config
	source ModelRoutingSource
}

func NewResolver(cfg config.Config) *Resolver {
	return &Resolver{cfg: cfg}
}

func (r *Resolver) SetModelRoutingSource(source ModelRoutingSource) {
	r.source = source
}

func (r *Resolver) ResolveQuality(requestedQuality, requestedSize, abstractModel string) (string, error) {
	if quality, ok, err := resolveExplicitOrSizedQuality(requestedQuality, requestedSize); ok || err != nil {
		return quality, err
	}
	value := r.cfg.Billing.AutoQualityDefaultByGroup[strings.ToLower(abstractModel)]
	if value == "" {
		return "", errs.New(400, errs.CodeImageCapabilityMismatch, fmt.Sprintf("unsupported abstract model %s", abstractModel))
	}
	return strings.ToLower(strings.TrimSpace(value)), nil
}

func ResolveRouteQuality(routeModel RouteModelConfig, taskType, requestedQuality, requestedSize string, autoDefaults map[string]string, prices []RoutePriceConfig) (string, error) {
	if quality, ok, err := resolveExplicitOrSizedQuality(requestedQuality, requestedSize); ok || err != nil {
		if err != nil {
			return "", err
		}
		if !hasRoutePrice(routeModel.ID, taskType, quality, prices) {
			return "", errs.New(409, errs.CodeRouteModelPriceMissing, "model pricing not found")
		}
		return quality, nil
	}

	quality := strings.ToLower(strings.TrimSpace(autoDefaults[strings.ToLower(routeModel.Code)]))
	source := "route_model_default"
	if quality == "" || !hasRoutePrice(routeModel.ID, taskType, quality, prices) {
		quality = firstRouteQuality(routeModel.ID, taskType, prices)
		source = "first_configured_price"
	}
	if quality == "" {
		return "", errs.New(409, errs.CodeRouteModelPriceMissing, "model pricing not found")
	}
	slog.Warn("route model auto quality fell back to default bucket",
		"route_model_id", routeModel.ID,
		"route_model_code", routeModel.Code,
		"task_type", taskType,
		"requested_quality", requestedQuality,
		"requested_size", requestedSize,
		"resolved_quality", quality,
		"fallback_source", source,
	)
	return quality, nil
}

func resolveExplicitOrSizedQuality(requestedQuality, requestedSize string) (string, bool, error) {
	quality := strings.ToLower(strings.TrimSpace(requestedQuality))
	if quality == "1k" || quality == "2k" || quality == "4k" {
		return quality, true, nil
	}
	if quality != "" && quality != "auto" {
		return "", true, errs.New(400, errs.CodeImageCapabilityMismatch, "unsupported quality")
	}
	if strings.TrimSpace(requestedSize) == "" || strings.EqualFold(strings.TrimSpace(requestedSize), "auto") {
		return "", false, nil
	}
	parts := strings.Split(strings.ToLower(strings.TrimSpace(requestedSize)), "x")
	if len(parts) != 2 {
		return "", true, errs.New(400, errs.CodeImageAutoUnsupported, "unsupported size")
	}
	width, errW := strconv.Atoi(strings.TrimSpace(parts[0]))
	height, errH := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errW != nil || errH != nil {
		return "", true, errs.New(400, errs.CodeImageAutoUnsupported, "unsupported size")
	}
	longest := width
	if height > longest {
		longest = height
	}
	switch {
	case longest <= 1024:
		return "1k", true, nil
	case longest <= 2048:
		return "2k", true, nil
	case longest <= 4096:
		return "4k", true, nil
	default:
		return "", true, errs.New(400, errs.CodeImageAutoUnsupported, "unsupported size")
	}
}

func (r *Resolver) Resolve(req ResolveRequest) (ResolvedRequest, error) {
	return r.ResolveContext(context.Background(), req)
}

func (r *Resolver) ResolveContext(ctx context.Context, req ResolveRequest) (ResolvedRequest, error) {
	if strings.TrimSpace(req.RouteModelCode) != "" {
		return r.resolveRouteContext(ctx, req)
	}
	model := strings.ToLower(req.AbstractModel)
	quality, err := r.ResolveQuality(req.RequestedQuality, req.RequestedSize, model)
	if err != nil {
		return ResolvedRequest{}, err
	}
	if req.RequestedOutputImageCount <= 0 {
		req.RequestedOutputImageCount = 1
	}
	if req.RequestedOutputImageCount > r.cfg.GenerationLimits.MaxImageCount {
		return ResolvedRequest{}, errs.New(400, errs.CodeImageCapabilityMismatch, "requested output image count exceeds platform limit")
	}
	if req.ReferenceImageCount > r.cfg.GenerationLimits.ReferenceImageMaxCount {
		return ResolvedRequest{}, errs.New(400, errs.CodeImageReferenceExceeded, "reference image count exceeds platform limit")
	}
	if _, ok := r.cfg.Billing.QualityPointsByModel[model]; !ok {
		return ResolvedRequest{}, errs.New(400, errs.CodeImageCapabilityMismatch, "unsupported abstract model")
	}
	routing, err := r.runtimeRouting(ctx)
	if err != nil {
		return ResolvedRequest{}, err
	}

	candidates := make([]ProviderCandidate, 0, len(r.cfg.Routing.ProviderCapabilities))
	if len(routing.ProviderModels) > 0 {
		for _, candidate := range routing.ProviderModels {
			if !r.providerEnabledWithRouting(candidate.Provider, routing) {
				continue
			}
			if !candidateHealthUsable(candidate.HealthStatus) {
				continue
			}
			if len(candidate.SupportedTaskTypes) > 0 && !containsString(candidate.SupportedTaskTypes, req.TaskType) {
				continue
			}
			if len(candidate.SupportedQualities) > 0 && !containsString(candidate.SupportedQualities, quality) {
				continue
			}
			if candidate.MaxImageCount > 0 && req.RequestedOutputImageCount > candidate.MaxImageCount {
				continue
			}
			if candidate.MaxReferenceImageCount > 0 && req.ReferenceImageCount > candidate.MaxReferenceImageCount {
				continue
			}
			if req.ReferenceImageCount > 0 && !candidate.SupportsImageInput {
				continue
			}
			if req.MaskPresent && !candidate.SupportsMask {
				continue
			}
			candidates = append(candidates, candidate)
		}
	} else {
		for providerName, capability := range r.cfg.Routing.ProviderCapabilities {
			if !r.providerEnabledWithRouting(providerName, routing) {
				continue
			}
			if !containsString(capability.SupportedModels, model) {
				continue
			}
			if !containsString(capability.SupportedTaskTypes, req.TaskType) {
				continue
			}
			if !containsString(capability.SupportedQualities, quality) {
				continue
			}
			if req.RequestedOutputImageCount > capability.MaxImageCount {
				continue
			}
			if req.ReferenceImageCount > capability.MaxReferenceImageCount {
				continue
			}
			if req.ReferenceImageCount > 0 && !capability.SupportsImageInput {
				continue
			}
			if req.MaskPresent && !capability.SupportsMask {
				continue
			}
			candidates = append(candidates, ProviderCandidate{
				Provider:               providerName,
				ModelCode:              strings.TrimSpace(r.cfg.Routing.ProviderModelMap[strings.ToLower(model)][strings.ToLower(providerName)]),
				SupportedTaskTypes:     cloneStrings(capability.SupportedTaskTypes),
				SupportedQualities:     cloneStrings(capability.SupportedQualities),
				SupportedAspectRatios:  cloneStrings(capability.SupportedAspectRatios),
				MaxImageCount:          capability.MaxImageCount,
				MaxReferenceImageCount: capability.MaxReferenceImageCount,
				SupportsImageInput:     capability.SupportsImageInput,
				SupportsMask:           capability.SupportsMask,
				Priority:               capability.Priority,
			})
		}
	}
	candidates = applyRuntimeRouteOrder(candidates, model, req.TaskType, req.RouteKey, routing)
	if len(candidates) == 0 {
		return ResolvedRequest{}, errs.New(400, errs.CodeImageCapabilityMismatch, "no eligible provider for request")
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Priority == candidates[j].Priority {
			return candidates[i].Provider < candidates[j].Provider
		}
		return candidates[i].Priority < candidates[j].Priority
	})
	return ResolvedRequest{
		ResolvedQualityBucket:  quality,
		Providers:              candidates,
		MaxOutputImageCount:    r.cfg.GenerationLimits.MaxImageCount,
		MaxReferenceImageCount: r.cfg.GenerationLimits.ReferenceImageMaxCount,
		RuntimeRoutingApplied:  len(routing.Providers) > 0 || len(routing.Routes) > 0,
	}, nil
}

func candidateHealthUsable(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "healthy", "unknown", "enabled":
		return true
	default:
		return false
	}
}

func (r *Resolver) resolveRouteContext(ctx context.Context, req ResolveRequest) (ResolvedRequest, error) {
	routeCode := strings.ToLower(strings.TrimSpace(req.RouteModelCode))
	routing, err := r.runtimeRouting(ctx)
	if err != nil {
		return ResolvedRequest{}, err
	}
	var routeModel RouteModelConfig
	for _, item := range routing.RouteModels {
		if item.Enabled && strings.EqualFold(item.Code, routeCode) {
			routeModel = item
			break
		}
	}
	if routeModel.ID == 0 {
		return ResolvedRequest{}, errs.New(404, errs.CodeModelRouteNotFound, "route model not found")
	}
	if routeModel.Visibility == "hidden" {
		return ResolvedRequest{}, errs.New(403, errs.CodeModelRouteNotVisible, "route model is not visible")
	}
	matchedGroups := matchedActiveGroups(routing, req.UserGroupCodes, routeModel.ID)
	if _, ok := effectiveMultiplier(routeModel, matchedGroups); !ok {
		return ResolvedRequest{}, errs.New(403, errs.CodeModelRouteNotVisible, "route model is not visible")
	}
	quality := strings.ToLower(strings.TrimSpace(req.RequestedQuality))
	quality, err = ResolveRouteQuality(routeModel, req.TaskType, req.RequestedQuality, req.RequestedSize, r.cfg.Billing.AutoQualityDefaultByGroup, routing.Prices)
	if err != nil {
		return ResolvedRequest{}, err
	}
	if req.RequestedOutputImageCount <= 0 {
		req.RequestedOutputImageCount = 1
	}
	if req.RequestedOutputImageCount > r.cfg.GenerationLimits.MaxImageCount {
		return ResolvedRequest{}, errs.New(400, errs.CodeImageCapabilityMismatch, "requested output image count exceeds platform limit")
	}
	if req.ReferenceImageCount > r.cfg.GenerationLimits.ReferenceImageMaxCount {
		return ResolvedRequest{}, errs.New(400, errs.CodeImageReferenceExceeded, "reference image count exceeds platform limit")
	}
	accountModels := make(map[int64]ProviderCandidate, len(routing.ProviderModels))
	for _, candidate := range routing.ProviderModels {
		accountModels[candidate.AccountModelID] = candidate
	}
	candidates := make([]ProviderCandidate, 0, len(routing.Candidates))
	for _, item := range routing.Candidates {
		if !item.Enabled || item.RouteModelID != routeModel.ID {
			continue
		}
		candidate, ok := accountModels[item.AccountModelID]
		if !ok {
			continue
		}
		if len(candidate.SupportedTaskTypes) > 0 && !containsString(candidate.SupportedTaskTypes, req.TaskType) {
			continue
		}
		if len(candidate.SupportedQualities) > 0 && !containsString(candidate.SupportedQualities, quality) {
			continue
		}
		candidate.RouteModelID = routeModel.ID
		candidate.RouteModelCode = routeModel.Code
		candidate.Priority = item.Priority
		candidate.WeightPercent = item.Weight
		candidate.FallbackOrder = item.FallbackOrder
		candidate.RouteSnapshotVersion = routing.Version
		candidates = append(candidates, candidate)
	}
	if len(candidates) == 0 {
		return ResolvedRequest{}, errs.New(409, errs.CodeModelRouteNoCandidate, "route model has no available candidate")
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority < candidates[j].Priority
		}
		if candidates[i].FallbackOrder != candidates[j].FallbackOrder {
			return candidates[i].FallbackOrder < candidates[j].FallbackOrder
		}
		return candidates[i].AccountModelID < candidates[j].AccountModelID
	})
	return ResolvedRequest{
		ResolvedQualityBucket:  quality,
		Providers:              candidates,
		MaxOutputImageCount:    r.cfg.GenerationLimits.MaxImageCount,
		MaxReferenceImageCount: r.cfg.GenerationLimits.ReferenceImageMaxCount,
		RuntimeRoutingApplied:  true,
	}, nil
}

func firstRouteQuality(routeModelID int64, taskType string, prices []RoutePriceConfig) string {
	qualities := []string{}
	for _, price := range prices {
		if price.Enabled && price.RouteModelID == routeModelID && strings.EqualFold(price.TaskType, taskType) {
			qualities = append(qualities, strings.ToLower(price.Quality))
		}
	}
	sort.Strings(qualities)
	if len(qualities) == 0 {
		return ""
	}
	return qualities[0]
}

func hasRoutePrice(routeModelID int64, taskType, quality string, prices []RoutePriceConfig) bool {
	for _, price := range prices {
		if price.Enabled && price.RouteModelID == routeModelID && strings.EqualFold(price.TaskType, taskType) && strings.EqualFold(price.Quality, quality) {
			return true
		}
	}
	return false
}

func (r *Resolver) ListCapabilities() []CapabilityItem {
	models := make([]string, 0, len(r.cfg.Billing.QualityPointsByModel))
	for model := range r.cfg.Billing.QualityPointsByModel {
		models = append(models, model)
	}
	sort.Strings(models)

	items := make([]CapabilityItem, 0, len(models))
	for _, model := range models {
		items = append(items, CapabilityItem{
			AbstractModel:          model,
			TaskTypes:              unionStrings(r.taskTypesForModel(model)),
			Qualities:              append([]string{"auto"}, sortedKeys(r.cfg.Billing.QualityPointsByModel[model])...),
			AspectRatios:           unionStrings(r.aspectRatiosForModel(model)),
			MaxOutputImageCount:    r.maxOutputCountForModel(model),
			MaxReferenceImageCount: r.maxReferenceCountForModel(model),
		})
	}
	return items
}

func (r *Resolver) providerEnabled(name string) bool {
	return r.providerEnabledWithRouting(name, ModelRoutingSnapshot{})
}

func (r *Resolver) providerEnabledWithRouting(name string, routing ModelRoutingSnapshot) bool {
	if len(routing.Providers) > 0 {
		normalized := strings.ToLower(name)
		for _, provider := range routing.Providers {
			if strings.EqualFold(provider.ProviderCode, normalized) {
				return provider.Enabled
			}
		}
		return false
	}
	switch strings.ToLower(name) {
	case "openai":
		return r.cfg.Providers.OpenAI.Enabled
	case "openrouter":
		return r.cfg.Providers.OpenRouter.Enabled
	default:
		return false
	}
}

func (r *Resolver) runtimeRouting(ctx context.Context) (ModelRoutingSnapshot, error) {
	if r.source == nil {
		return ModelRoutingSnapshot{}, nil
	}
	snapshot, err := r.source.ModelRoutingConfig(ctx)
	if err != nil {
		return ModelRoutingSnapshot{}, errs.Internal("failed to load model routing config")
	}
	return snapshot, nil
}

func applyRuntimeRouteOrder(candidates []ProviderCandidate, model, taskType, routeKey string, routing ModelRoutingSnapshot) []ProviderCandidate {
	if len(routing.Routes) == 0 {
		return candidates
	}
	byProviderModel := make(map[int64]ProviderCandidate, len(candidates))
	for _, candidate := range candidates {
		if candidate.ProviderModelID > 0 {
			byProviderModel[candidate.ProviderModelID] = candidate
		}
	}
	orderedRoutes := make([]ModelRouteConfig, 0, len(routing.Routes))
	for _, route := range routing.Routes {
		if !route.Enabled || !strings.EqualFold(route.GroupCode, model) || !strings.EqualFold(route.TaskType, taskType) {
			continue
		}
		if route.ProviderModelID <= 0 {
			continue
		}
		orderedRoutes = append(orderedRoutes, route)
	}
	if len(orderedRoutes) == 0 {
		return candidates
	}
	sort.SliceStable(orderedRoutes, func(i, j int) bool {
		if orderedRoutes[i].Priority != orderedRoutes[j].Priority {
			return orderedRoutes[i].Priority < orderedRoutes[j].Priority
		}
		if orderedRoutes[i].FallbackOrder != orderedRoutes[j].FallbackOrder {
			return orderedRoutes[i].FallbackOrder < orderedRoutes[j].FallbackOrder
		}
		return orderedRoutes[i].ID < orderedRoutes[j].ID
	})
	ordered := make([]ProviderCandidate, 0, len(candidates))
	seen := map[int64]struct{}{}
	buckets := map[int][]ModelRouteConfig{}
	priorities := []int{}
	for _, route := range orderedRoutes {
		if _, ok := buckets[route.Priority]; !ok {
			priorities = append(priorities, route.Priority)
		}
		buckets[route.Priority] = append(buckets[route.Priority], route)
	}
	sort.Ints(priorities)
	for _, priority := range priorities {
		bucket := buckets[priority]
		primary, fallbacks := weightedRouteSelection(bucket, routeKey)
		selection := append([]ModelRouteConfig{primary}, fallbacks...)
		sort.SliceStable(selection[1:], func(i, j int) bool {
			left := selection[i+1]
			right := selection[j+1]
			if left.FallbackOrder != right.FallbackOrder {
				return left.FallbackOrder < right.FallbackOrder
			}
			return left.ID < right.ID
		})
		for _, route := range selection {
			candidate, ok := byProviderModel[route.ProviderModelID]
			if !ok {
				continue
			}
			candidate.Priority = route.Priority
			candidate.WeightPercent = route.WeightPercent
			candidate.FallbackOrder = route.FallbackOrder
			candidate.RouteSnapshotVersion = routing.Version
			ordered = append(ordered, candidate)
			seen[route.ProviderModelID] = struct{}{}
		}
	}
	for _, candidate := range candidates {
		if _, ok := seen[candidate.ProviderModelID]; ok {
			continue
		}
		ordered = append(ordered, candidate)
	}
	return ordered
}

func weightedRouteSelection(routes []ModelRouteConfig, routeKey string) (ModelRouteConfig, []ModelRouteConfig) {
	if len(routes) == 1 {
		return routes[0], nil
	}
	hashValue := hashKey(defaultRouteKey(routeKey))
	totalWeight := 0
	for _, route := range routes {
		weight := route.WeightPercent
		if weight <= 0 {
			weight = 1
		}
		totalWeight += weight
	}
	pick := int(hashValue % uint32(totalWeight))
	acc := 0
	primaryIndex := 0
	for idx, route := range routes {
		weight := route.WeightPercent
		if weight <= 0 {
			weight = 1
		}
		acc += weight
		if pick < acc {
			primaryIndex = idx
			break
		}
	}
	primary := routes[primaryIndex]
	fallbacks := make([]ModelRouteConfig, 0, len(routes)-1)
	for idx, route := range routes {
		if idx == primaryIndex {
			continue
		}
		fallbacks = append(fallbacks, route)
	}
	return primary, fallbacks
}

func defaultRouteKey(value string) string {
	if strings.TrimSpace(value) == "" {
		return "default"
	}
	return strings.TrimSpace(value)
}

func hashKey(value string) uint32 {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(value))
	return hasher.Sum32()
}

func (r *Resolver) taskTypesForModel(model string) []string {
	values := []string{}
	for providerName, capability := range r.cfg.Routing.ProviderCapabilities {
		if !r.providerEnabled(providerName) || !containsString(capability.SupportedModels, model) {
			continue
		}
		values = append(values, capability.SupportedTaskTypes...)
	}
	return values
}

func (r *Resolver) aspectRatiosForModel(model string) []string {
	values := []string{}
	for providerName, capability := range r.cfg.Routing.ProviderCapabilities {
		if !r.providerEnabled(providerName) || !containsString(capability.SupportedModels, model) {
			continue
		}
		values = append(values, capability.SupportedAspectRatios...)
	}
	return values
}

func (r *Resolver) maxOutputCountForModel(model string) int {
	maxCount := 0
	for providerName, capability := range r.cfg.Routing.ProviderCapabilities {
		if !r.providerEnabled(providerName) || !containsString(capability.SupportedModels, model) {
			continue
		}
		if capability.MaxImageCount > maxCount {
			maxCount = capability.MaxImageCount
		}
	}
	if maxCount == 0 {
		return r.cfg.GenerationLimits.MaxImageCount
	}
	return maxCount
}

func (r *Resolver) maxReferenceCountForModel(model string) int {
	maxCount := 0
	for providerName, capability := range r.cfg.Routing.ProviderCapabilities {
		if !r.providerEnabled(providerName) || !containsString(capability.SupportedModels, model) {
			continue
		}
		if capability.MaxReferenceImageCount > maxCount {
			maxCount = capability.MaxReferenceImageCount
		}
	}
	if maxCount == 0 {
		return r.cfg.GenerationLimits.ReferenceImageMaxCount
	}
	return maxCount
}

func containsString(values []string, expected string) bool {
	expected = strings.ToLower(expected)
	for _, value := range values {
		if strings.ToLower(value) == expected {
			return true
		}
	}
	return false
}

func cloneStrings(values []string) []string {
	return append([]string(nil), values...)
}

func unionStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
