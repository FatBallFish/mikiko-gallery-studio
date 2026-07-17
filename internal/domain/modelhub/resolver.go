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
	SizeMode                  string
	AspectRatio               string
	BaseResolution            string
	Quality                   string
	OutputFormat              string
	OutputCompression         int
	Moderation                string
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
		baseResolution := map[string]struct{}{}
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
				TaskType:       price.TaskType,
				BaseResolution: price.BaseResolution,
				BasePoints:     base.StringFixed(5),
				ChargedPoints:  charged.StringFixed(5),
				DisplayPoints:  charged.Round(2).StringFixed(2),
			})
			taskTypes[price.TaskType] = struct{}{}
			baseResolution[price.BaseResolution] = struct{}{}
		}
		sizeModes, aspectRatios, pixelSizes, quality, outputFormat, supportsOutputCompression, moderation, maxOutputCount, maxReferenceCount := r.visibleRouteModelLimits(routeModel, routing)
		visible = append(visible, VisibleRouteModel{
			ID:                        routeModel.ID,
			Code:                      routeModel.Code,
			Name:                      routeModel.Name,
			Description:               routeModel.Description,
			TaskTypes:                 sortedSet(taskTypes),
			BaseResolution:            append([]string{"auto"}, sortedSet(baseResolution)...),
			Quality:                   quality,
			SizeModes:                 sizeModes,
			AspectRatios:              aspectRatios,
			PixelSizes:                pixelSizes,
			OutputFormat:              outputFormat,
			SupportsOutputCompression: supportsOutputCompression,
			Moderation:                moderation,
			MaxOutputImageCount:       maxOutputCount,
			MaxReferenceImageCount:    maxReferenceCount,
			EffectiveMultiplier:       multiplier.StringFixed(5),
			Prices:                    prices,
		})
	}
	return visible, nil
}

func (r *Resolver) visibleRouteModelLimits(routeModel RouteModelConfig, routing ModelRoutingSnapshot) ([]string, []string, []string, []string, []string, bool, []string, int, int) {
	candidateByID := map[int64]ProviderCandidate{}
	for _, candidate := range routing.ProviderModels {
		candidateByID[candidate.AccountModelID] = candidate
	}
	sizeModes := map[string]struct{}{}
	ratios := map[string]struct{}{}
	pixelSizes := map[string]struct{}{}
	quality := map[string]struct{}{}
	outputFormat := map[string]struct{}{}
	moderation := map[string]struct{}{}
	maxOutputCount := 0
	maxReferenceCount := 0
	hasConfiguredCandidate := false
	for _, route := range routing.Candidates {
		if !route.Enabled || route.RouteModelID != routeModel.ID {
			continue
		}
		candidate, ok := candidateByID[route.AccountModelID]
		if !ok {
			continue
		}
		hasConfiguredCandidate = true
		for _, ratio := range candidate.SupportedAspectRatios {
			if trimmed := strings.TrimSpace(ratio); trimmed != "" {
				ratios[trimmed] = struct{}{}
			}
		}
		for _, size := range candidate.SupportedPixelSizes {
			if trimmed := strings.TrimSpace(size); trimmed != "" {
				pixelSizes[trimmed] = struct{}{}
			}
		}
		for _, item := range candidate.Quality {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				quality[trimmed] = struct{}{}
			}
		}
		for _, item := range candidate.OutputFormat {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				outputFormat[trimmed] = struct{}{}
			}
		}
		for _, item := range candidate.Moderation {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				moderation[trimmed] = struct{}{}
			}
		}
		if candidate.SupportsOutputCompression {
			supportsOutputCompression = true
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
	if !hasConfiguredCandidate {
		maxReferenceCount = r.cfg.GenerationLimits.ReferenceImageMaxCount
	}
	if len(sizeModes) == 0 {
		sizeModes[SizeModeRatio] = struct{}{}
	}
	if len(quality) == 0 {
		quality["auto"] = struct{}{}
	}
	if len(outputFormat) == 0 {
		outputFormat["png"] = struct{}{}
	}
	if len(moderation) == 0 {
		moderation["auto"] = struct{}{}
	}
	return sortedSet(sizeModes), sortedSet(ratios), sortedSet(pixelSizes), sortedSet(quality), sortedSet(outputFormat), supportsOutputCompression, sortedSet(moderation), maxOutputCount, maxReferenceCount
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
	AccountModelID            int64
	ModelAccountID            int64
	RouteModelID              int64
	RouteModelCode            string
	AdapterType               string
	AuthType                  string
	BaseURL                   string
	Credentials               map[string]string
	ProviderModelID           int64
	Provider                  string
	ModelCode                 string
	CompatMode                string
	SupportedTaskTypes        []string
	SupportedBaseResolution   []string
	Quality                   []string
	SizeModes                 []string
	SupportedAspectRatios     []string
	SupportedPixelSizes       []string
	OutputFormat              []string
	OutputCompression         int
	SupportsOutputCompression bool
	Moderation                []string
	MaxImageCount             int
	MaxReferenceImageCount    int
	SupportsImageInput        bool
	SupportsMask              bool
	Priority                  int
	WeightPercent             int
	FallbackOrder             int
	HealthStatus              string
	TimeoutMS                 int
	InputCost                 string
	OutputCost                string
	Currency                  string
	AccountExtra              map[string]any
	ModelExtra                map[string]any
	RouteSnapshotVersion      string
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
	BaseResolution      string
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
	BaseResolution         string
	RouteModelID           int64
	RouteModelCode         string
	RouteSnapshotVersion   string
	Providers              []ProviderCandidate
	MaxOutputImageCount    int
	MaxReferenceImageCount int
	RuntimeRoutingApplied  bool
}

type CapabilityItem struct {
	AbstractModel             string
	TaskTypes                 []string
	BaseResolution            []string
	Quality                   []string
	SizeModes                 []string
	AspectRatios              []string
	PixelSizes                []string
	OutputFormat              []string
	SupportsOutputCompression bool
	Moderation                []string
	MaxOutputImageCount       int
	MaxReferenceImageCount    int
}

type VisibleRouteModel struct {
	ID                        int64                    `json:"id"`
	Code                      string                   `json:"code"`
	Name                      string                   `json:"name"`
	Description               string                   `json:"description,omitempty"`
	TaskTypes                 []string                 `json:"task_types"`
	BaseResolution            []string                 `json:"base_resolution"`
	Quality                   []string                 `json:"quality"`
	SizeModes                 []string                 `json:"size_modes"`
	AspectRatios              []string                 `json:"aspect_ratios"`
	PixelSizes                []string                 `json:"pixel_sizes"`
	OutputFormat              []string                 `json:"output_format"`
	SupportsOutputCompression bool                     `json:"supports_output_compression"`
	Moderation                []string                 `json:"moderation"`
	MaxOutputImageCount       int                      `json:"max_output_image_count"`
	MaxReferenceImageCount    int                      `json:"max_reference_image_count"`
	EffectiveMultiplier       string                   `json:"effective_multiplier"`
	Prices                    []VisibleRouteModelPrice `json:"prices"`
}

type VisibleRouteModelPrice struct {
	TaskType       string `json:"task_type"`
	BaseResolution string `json:"base_resolution"`
	BasePoints     string `json:"base_points"`
	ChargedPoints  string `json:"charged_points"`
	DisplayPoints  string `json:"display_points"`
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

func (r *Resolver) ResolveBaseResolution(requestedBaseResolution, requestedSize, abstractModel string) (string, error) {
	if baseResolution, ok, err := resolveExplicitOrSizedBaseResolution(requestedBaseResolution, requestedSize); ok || err != nil {
		return baseResolution, err
	}
	value := r.cfg.Billing.AutoBaseResolutionDefaultByGroup[strings.ToLower(abstractModel)]
	if value == "" {
		return "", errs.New(400, errs.CodeImageCapabilityMismatch, fmt.Sprintf("unsupported abstract model %s", abstractModel))
	}
	return strings.ToLower(strings.TrimSpace(value)), nil
}

func ResolveRouteBaseResolution(routeModel RouteModelConfig, taskType, requestedBaseResolution, requestedSize string, autoDefaults map[string]string, prices []RoutePriceConfig) (string, error) {
	if baseResolution, ok, err := resolveExplicitOrSizedBaseResolution(requestedBaseResolution, requestedSize); ok || err != nil {
		if err != nil {
			return "", err
		}
		if !hasRoutePrice(routeModel.ID, taskType, baseResolution, prices) {
			return "", errs.New(409, errs.CodeRouteModelPriceMissing, "model pricing not found")
		}
		return baseResolution, nil
	}

	baseResolution := strings.ToLower(strings.TrimSpace(autoDefaults[strings.ToLower(routeModel.Code)]))
	source := "route_model_default"
	if baseResolution == "" || !hasRoutePrice(routeModel.ID, taskType, baseResolution, prices) {
		baseResolution = firstRouteBaseResolution(routeModel.ID, taskType, prices)
		source = "first_configured_price"
	}
	if baseResolution == "" {
		return "", errs.New(409, errs.CodeRouteModelPriceMissing, "model pricing not found")
	}
	slog.Warn("route model auto base resolution fell back to default bucket",
		"route_model_id", routeModel.ID,
		"route_model_code", routeModel.Code,
		"task_type", taskType,
		"base_resolution", requestedBaseResolution,
		"requested_size", requestedSize,
		"base_resolution", baseResolution,
		"fallback_source", source,
	)
	return baseResolution, nil
}

func ResolveRouteBaseResolutionBySizeMode(routeModel RouteModelConfig, req ResolveRequest, autoDefaults map[string]string, prices []RoutePriceConfig) (string, error) {
	switch PublicSizeMode(req.SizeMode) {
	case SizeModePixel:
		baseResolution, err := BaseResolutionByPixelSize(req.RequestedSize)
		if err != nil {
			return "", err
		}
		if !hasRoutePrice(routeModel.ID, req.TaskType, baseResolution, prices) {
			return "", errs.New(409, errs.CodeRouteModelPriceMissing, "model pricing not found")
		}
		return baseResolution, nil
	default:
		requestedSize := ""
		if strings.EqualFold(strings.TrimSpace(req.SizeMode), sizeModeLegacyRatio) {
			requestedSize = req.RequestedSize
		}
		return ResolveRouteBaseResolution(routeModel, req.TaskType, req.BaseResolution, requestedSize, autoDefaults, prices)
	}
}

func resolveExplicitOrSizedBaseResolution(requestedBaseResolution, requestedSize string) (string, bool, error) {
	baseResolution := strings.ToLower(strings.TrimSpace(requestedBaseResolution))
	if baseResolution == "1k" || baseResolution == "2k" || baseResolution == "4k" {
		return baseResolution, true, nil
	}
	if baseResolution != "" && baseResolution != "auto" {
		return "", true, errs.New(400, errs.CodeImageCapabilityMismatch, "unsupported base_resolution")
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
	normalizedReq, err := NormalizeResolveRequest(req)
	if err != nil {
		return ResolvedRequest{}, err
	}
	req = normalizedReq
	if strings.TrimSpace(req.RouteModelCode) != "" {
		return r.resolveRouteContext(ctx, req)
	}
	model := strings.ToLower(req.AbstractModel)
	baseResolution, err := r.ResolveBaseResolution(req.BaseResolution, req.RequestedSize, model)
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
	if _, ok := r.cfg.Billing.BaseResolutionPointsByModel[model]; !ok {
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
			if !candidateSupportsGenerationRequest(candidate, req) {
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
			if !containsString(capability.SupportedBaseResolution, baseResolution) {
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
				Provider:                providerName,
				ModelCode:               strings.TrimSpace(r.cfg.Routing.ProviderModelMap[strings.ToLower(model)][strings.ToLower(providerName)]),
				SupportedTaskTypes:      cloneStrings(capability.SupportedTaskTypes),
				SupportedBaseResolution: cloneStrings(capability.SupportedBaseResolution),
				Quality:                 cloneStrings(defaultStrings(capability.Quality, DefaultQuality)),
				SizeModes:               cloneStrings(DefaultSizeModes),
				SupportedAspectRatios:   cloneStrings(capability.SupportedAspectRatios),
				SupportedPixelSizes:     nil,
				OutputFormat:            cloneStrings(defaultStrings(capability.OutputFormat, DefaultOutputFormat)),
				OutputCompression:       capability.OutputCompression,
				Moderation:              cloneStrings(defaultStrings(capability.Moderation, DefaultModeration)),
				MaxImageCount:           capability.MaxImageCount,
				MaxReferenceImageCount:  capability.MaxReferenceImageCount,
				SupportsImageInput:      capability.SupportsImageInput,
				SupportsMask:            capability.SupportsMask,
				Priority:                capability.Priority,
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
		BaseResolution:         baseResolution,
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

func CandidateSupportsRequest(candidate ProviderCandidate, req ResolveRequest, resolvedBaseResolution string) bool {
	candidate = normalizeProviderCandidate(candidate)
	if !candidateHealthUsable(candidate.HealthStatus) {
		return false
	}
	if len(candidate.SupportedTaskTypes) > 0 && !containsString(candidate.SupportedTaskTypes, req.TaskType) {
		return false
	}
	if req.ReferenceImageCount > candidate.MaxReferenceImageCount {
		return false
	}
	if req.ReferenceImageCount > 0 && !candidate.SupportsImageInput {
		return false
	}
	if req.MaskPresent && !candidate.SupportsMask {
		return false
	}
	quality := NormalizeQuality(req.Quality)
	if quality == "" || !containsString(candidate.Quality, quality) {
		return false
	}
	outputFormat := NormalizeOutputFormat(req.OutputFormat)
	if outputFormat == "" || !containsString(candidate.OutputFormat, outputFormat) {
		return false
	}
	moderation := NormalizeModeration(req.Moderation)
	if moderation == "" || !containsString(candidate.Moderation, moderation) {
		return false
	}
	if req.OutputCompression < 0 || req.OutputCompression > 100 {
		return false
	}
	if req.OutputCompression > 0 && req.OutputCompression < 100 {
		if !candidate.SupportsOutputCompression || (outputFormat != "jpeg" && outputFormat != "webp") {
			return false
		}
	}
	mode := PublicSizeMode(req.SizeMode)
	if !containsString(candidate.SizeModes, mode) {
		return false
	}
	switch mode {
	case SizeModePixel:
		size := NormalizePixelSize(req.RequestedSize)
		if size == "" {
			return false
		}
		if len(candidate.SupportedPixelSizes) > 0 && !containsString(candidate.SupportedPixelSizes, size) {
			return false
		}
		return true
	default:
		ratio := NormalizeRatio(req.AspectRatio)
		if ratio == "" {
			return false
		}
		if !strings.EqualFold(strings.TrimSpace(req.SizeMode), sizeModeLegacyRatio) && len(candidate.SupportedAspectRatios) > 0 && !containsString(candidate.SupportedAspectRatios, ratio) {
			return false
		}
		if len(candidate.SupportedBaseResolution) > 0 && !containsString(candidate.SupportedBaseResolution, resolvedBaseResolution) {
			return false
		}
		return true
	}
}

func normalizeProviderCandidate(candidate ProviderCandidate) ProviderCandidate {
	capability, err := NormalizeCapability(ImageModelCapability{
		MaxReferenceImageCount:    candidate.MaxReferenceImageCount,
		MaxImageCount:             candidate.MaxImageCount,
		BaseResolution:            candidate.SupportedBaseResolution,
		Quality:                   candidate.Quality,
		SizeModes:                 candidate.SizeModes,
		SupportedRatios:           candidate.SupportedAspectRatios,
		SupportedPixelSizes:       candidate.SupportedPixelSizes,
		OutputFormat:              candidate.OutputFormat,
		OutputCompression:         candidate.OutputCompression,
		SupportsOutputCompression: candidate.SupportsOutputCompression,
		Moderation:                candidate.Moderation,
	})
	if err != nil {
		return candidate
	}
	candidate.MaxReferenceImageCount = capability.MaxReferenceImageCount
	candidate.MaxImageCount = capability.MaxImageCount
	candidate.SupportedBaseResolution = capability.BaseResolution
	candidate.Quality = capability.Quality
	candidate.SizeModes = capability.SizeModes
	candidate.SupportedAspectRatios = capability.SupportedRatios
	candidate.SupportedPixelSizes = capability.SupportedPixelSizes
	candidate.OutputFormat = capability.OutputFormat
	candidate.OutputCompression = capability.OutputCompression
	candidate.SupportsOutputCompression = capability.SupportsOutputCompression
	candidate.Moderation = capability.Moderation
	if candidate.MaxReferenceImageCount > 0 {
		candidate.SupportsImageInput = true
	}
	return candidate
}

func (r *Resolver) resolveRouteContext(ctx context.Context, req ResolveRequest) (ResolvedRequest, error) {
	normalizedReq, err := NormalizeResolveRequest(req)
	if err != nil {
		return ResolvedRequest{}, err
	}
	req = normalizedReq
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
	partial := ResolvedRequest{
		RouteModelID:          routeModel.ID,
		RouteModelCode:        routeModel.Code,
		RouteSnapshotVersion:  routing.Version,
		RuntimeRoutingApplied: true,
	}
	baseResolution := strings.ToLower(strings.TrimSpace(req.BaseResolution))
	baseResolution, err = ResolveRouteBaseResolutionBySizeMode(routeModel, req, r.cfg.Billing.AutoBaseResolutionDefaultByGroup, routing.Prices)
	if err != nil {
		return partial, err
	}
	partial.BaseResolution = baseResolution
	if req.RequestedOutputImageCount <= 0 {
		req.RequestedOutputImageCount = 1
	}
	if req.RequestedOutputImageCount > r.cfg.GenerationLimits.MaxImageCount {
		return partial, errs.New(400, errs.CodeImageCapabilityMismatch, "requested output image count exceeds platform limit")
	}
	if req.ReferenceImageCount > r.cfg.GenerationLimits.ReferenceImageMaxCount {
		return partial, errs.New(400, errs.CodeImageReferenceExceeded, "reference image count exceeds platform limit")
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
		if !CandidateSupportsRequest(candidate, req, baseResolution) {
			continue
		}
		if !candidateSupportsGenerationRequest(candidate, req) {
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
		return partial, errs.New(409, errs.CodeImageCapabilityMismatch, "当前配置暂不支持生成，请更换类似配置。")
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
		BaseResolution:         baseResolution,
		RouteModelID:           routeModel.ID,
		RouteModelCode:         routeModel.Code,
		RouteSnapshotVersion:   routing.Version,
		Providers:              candidates,
		MaxOutputImageCount:    r.cfg.GenerationLimits.MaxImageCount,
		MaxReferenceImageCount: r.cfg.GenerationLimits.ReferenceImageMaxCount,
		RuntimeRoutingApplied:  true,
	}, nil
}

func candidateSupportsGenerationRequest(candidate ProviderCandidate, req ResolveRequest) bool {
	if candidate.MaxImageCount > 0 && req.RequestedOutputImageCount > candidate.MaxImageCount {
		return false
	}
	if req.ReferenceImageCount > 0 {
		if !candidate.SupportsImageInput || candidate.MaxReferenceImageCount <= 0 || req.ReferenceImageCount > candidate.MaxReferenceImageCount {
			return false
		}
	}
	if req.MaskPresent && !candidate.SupportsMask {
		return false
	}
	return candidateSupportsRequestedSize(candidate, req.RequestedSize)
}

func candidateSupportsRequestedSize(candidate ProviderCandidate, requestedSize string) bool {
	requestedSize = strings.TrimSpace(requestedSize)
	if requestedSize == "" || strings.EqualFold(requestedSize, "auto") || len(candidate.SupportedAspectRatios) == 0 {
		return true
	}
	requestedWidth, requestedHeight, ok := ParseImageSize(requestedSize)
	if !ok {
		requestedWidth, requestedHeight, ok = parseRatio(requestedSize)
		if !ok {
			return false
		}
	}
	requestedRatio := simplifiedRatioKey(requestedWidth, requestedHeight)
	for _, ratio := range candidate.SupportedAspectRatios {
		supportedWidth, supportedHeight, valid := parseRatio(ratio)
		if valid && simplifiedRatioKey(supportedWidth, supportedHeight) == requestedRatio {
			return true
		}
	}
	return false
}

func firstRouteQuality(routeModelID int64, taskType string, prices []RoutePriceConfig) string {
	qualities := []string{}
	for _, price := range prices {
		if price.Enabled && price.RouteModelID == routeModelID && strings.EqualFold(price.TaskType, taskType) {
			baseResolution = append(baseResolution, strings.ToLower(price.BaseResolution))
		}
	}
	sort.Strings(baseResolution)
	if len(baseResolution) == 0 {
		return ""
	}
	return baseResolution[0]
}

func hasRoutePrice(routeModelID int64, taskType, baseResolution string, prices []RoutePriceConfig) bool {
	for _, price := range prices {
		if price.Enabled && price.RouteModelID == routeModelID && strings.EqualFold(price.TaskType, taskType) && strings.EqualFold(price.BaseResolution, baseResolution) {
			return true
		}
	}
	return false
}

func (r *Resolver) ListCapabilities() []CapabilityItem {
	models := make([]string, 0, len(r.cfg.Billing.BaseResolutionPointsByModel))
	for model := range r.cfg.Billing.BaseResolutionPointsByModel {
		models = append(models, model)
	}
	sort.Strings(models)

	items := make([]CapabilityItem, 0, len(models))
	for _, model := range models {
		items = append(items, CapabilityItem{
			AbstractModel:          model,
			TaskTypes:              unionStrings(r.taskTypesForModel(model)),
			BaseResolution:         append([]string{"auto"}, sortedKeys(r.cfg.Billing.BaseResolutionPointsByModel[model])...),
			Quality:                cloneStrings(DefaultQuality),
			SizeModes:              cloneStrings(DefaultSizeModes),
			AspectRatios:           unionStrings(r.aspectRatiosForModel(model)),
			PixelSizes:             []string{},
			OutputFormat:           cloneStrings(DefaultOutputFormat),
			Moderation:             cloneStrings(DefaultModeration),
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
