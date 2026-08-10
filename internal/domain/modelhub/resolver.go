package modelhub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	Background                string
	OutputCompression         int
	Moderation                string
	RequestedSize             string
	RequestedOutputImageCount int
	ReferenceImageCount       int
	MaskPresent               bool
	RouteKey                  string
	UserGroupCodes            []string
	ExpectedCapabilityVersion string
	TrustedResolvedSize       string
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
		taskTypeList := sortedSet(taskTypes)
		autoBaseResolutionByTaskType := make(map[string]string, len(taskTypeList))
		capabilitiesByTaskType := make(map[string]VisibleRouteModelTaskCapability, len(taskTypeList))
		for _, taskType := range taskTypeList {
			autoBaseResolution, _ := resolveAutoRouteBaseResolution(routeModel, taskType, r.cfg.Billing.AutoBaseResolutionDefaultByGroup, routing.Prices)
			if autoBaseResolution != "" {
				autoBaseResolutionByTaskType[taskType] = autoBaseResolution
			}
			taskCapability := r.visibleRouteModelTaskCapability(routeModel, routing, taskType)
			taskCapability.AutoBaseResolution = autoBaseResolution
			capabilitiesByTaskType[taskType] = taskCapability
		}
		aggregateCapability := r.visibleRouteModelLimits(routeModel, routing, "")
		aggregateCapability.PixelSizes = commonPixelTaskPresets(capabilitiesByTaskType)
		aggregateCapability.SupportsCustomSize = aggregateCapability.SupportsCustomSize && everyPixelTaskSupportsCustomSize(capabilitiesByTaskType)
		visible = append(visible, VisibleRouteModel{
			ID:                           routeModel.ID,
			Code:                         routeModel.Code,
			Name:                         routeModel.Name,
			Description:                  routeModel.Description,
			TaskTypes:                    taskTypeList,
			BaseResolution:               sortedSet(baseResolution),
			AutoBaseResolutionByTaskType: autoBaseResolutionByTaskType,
			Quality:                      aggregateCapability.Quality,
			SizeModes:                    aggregateCapability.SizeModes,
			AspectRatios:                 aggregateCapability.AspectRatios,
			PixelSizes:                   aggregateCapability.PixelSizes,
			OutputFormat:                 aggregateCapability.OutputFormat,
			SupportsOutputCompression:    aggregateCapability.SupportsOutputCompression,
			SupportsCustomSize:           aggregateCapability.SupportsCustomSize,
			SupportsCustomRatio:          aggregateCapability.SupportsCustomRatio,
			SupportedBackgrounds:         aggregateCapability.SupportedBackgrounds,
			MinWidth:                     aggregateCapability.MinWidth,
			MaxWidth:                     aggregateCapability.MaxWidth,
			MinHeight:                    aggregateCapability.MinHeight,
			MaxHeight:                    aggregateCapability.MaxHeight,
			Moderation:                   aggregateCapability.Moderation,
			MaxOutputImageCount:          aggregateCapability.MaxOutputImageCount,
			MaxReferenceImageCount:       aggregateCapability.MaxReferenceImageCount,
			CapabilitiesByTaskType:       capabilitiesByTaskType,
			EffectiveMultiplier:          multiplier.StringFixed(5),
			Prices:                       prices,
		})
	}
	return visible, nil
}

func (r *Resolver) visibleRouteModelTaskCapability(routeModel RouteModelConfig, routing ModelRoutingSnapshot, taskType string) VisibleRouteModelTaskCapability {
	capability := r.visibleRouteModelLimits(routeModel, routing, taskType)
	pricedBaseResolutions := map[string]struct{}{}
	for _, price := range routing.Prices {
		if price.Enabled && price.RouteModelID == routeModel.ID && strings.EqualFold(price.TaskType, taskType) {
			pricedBaseResolutions[strings.ToLower(strings.TrimSpace(price.BaseResolution))] = struct{}{}
		}
	}
	pricedPixelSizes := filterPricedPixelSizes(routeModel.ID, taskType, routing.Prices, capability.PixelSizes)
	intersectCapabilitySet(pricedBaseResolutions, capability.BaseResolution, false)
	capability.BaseResolution = sortedSet(pricedBaseResolutions)
	capability.PixelSizes = pricedPixelSizes
	capability.SupportsCustomSize = capability.SupportsCustomSize && everyReachablePixelBucketPriced(capability, pricedBaseResolutions)
	return capability
}

func everyReachablePixelBucketPriced(capability VisibleRouteModelTaskCapability, priced map[string]struct{}) bool {
	minWidth, maxWidth := effectivePixelBounds(capability.MinWidth, capability.MaxWidth)
	minHeight, maxHeight := effectivePixelBounds(capability.MinHeight, capability.MaxHeight)
	if minWidth > maxWidth || minHeight > maxHeight {
		return false
	}
	hasReachableBucket := false
	for _, bucket := range []struct {
		code     string
		min, max int
	}{
		{code: "1k", min: imageSizeMultiple, max: 1024},
		{code: "2k", min: 1024 + imageSizeMultiple, max: 2048},
		{code: "4k", min: 2048 + imageSizeMultiple, max: imageMaxEdge},
	} {
		if pixelBucketReachable(minWidth, maxWidth, minHeight, maxHeight, bucket.min, bucket.max) {
			hasReachableBucket = true
			if _, ok := priced[bucket.code]; ok {
				continue
			}
			return false
		}
	}
	return hasReachableBucket
}

func everyPixelTaskSupportsCustomSize(capabilities map[string]VisibleRouteModelTaskCapability) bool {
	hasPixelTask := false
	for _, capability := range capabilities {
		if !containsString(capability.SizeModes, SizeModePixel) {
			continue
		}
		hasPixelTask = true
		if !capability.SupportsCustomSize {
			return false
		}
	}
	return hasPixelTask
}

func commonPixelTaskPresets(capabilities map[string]VisibleRouteModelTaskCapability) []string {
	var common map[string]struct{}
	for _, capability := range capabilities {
		if !containsString(capability.SizeModes, SizeModePixel) {
			continue
		}
		current := make(map[string]struct{}, len(capability.PixelSizes))
		for _, size := range capability.PixelSizes {
			current[size] = struct{}{}
		}
		if common == nil {
			common = current
			continue
		}
		for size := range common {
			if _, ok := current[size]; !ok {
				delete(common, size)
			}
		}
	}
	return sortedSet(common)
}

func effectivePixelBounds(minimum, maximum int) (int, int) {
	if minimum <= 0 {
		minimum = imageSizeMultiple
	}
	if maximum <= 0 {
		maximum = imageMaxEdge
	}
	return roundUpToImageGrid(maxInt(minimum, imageSizeMultiple)), roundDownToImageGrid(minInt(maximum, imageMaxEdge))
}

func pixelBucketReachable(minWidth, maxWidth, minHeight, maxHeight, bucketMin, bucketMax int) bool {
	return longestEdgeRangeReachable(minWidth, maxWidth, minHeight, maxHeight, bucketMin, bucketMax) ||
		longestEdgeRangeReachable(minHeight, maxHeight, minWidth, maxWidth, bucketMin, bucketMax)
}

func longestEdgeRangeReachable(minLong, maxLong, minShort, maxShort, bucketMin, bucketMax int) bool {
	lower := roundUpToImageGrid(maxInt(maxInt(bucketMin, minLong), minShort))
	upper := roundDownToImageGrid(minInt(minInt(bucketMax, maxLong), imageMaxAspectRatioInt*maxShort))
	for longest := lower; longest <= upper; longest += imageSizeMultiple {
		shortestLower := maxInt(minShort, maxInt(ceilDiv(longest, imageMaxAspectRatioInt), ceilDiv(imageMinPixels, longest)))
		shortestUpper := minInt(maxShort, minInt(longest, imageMaxPixels/longest))
		if roundUpToImageGrid(shortestLower) <= roundDownToImageGrid(shortestUpper) {
			return true
		}
	}
	return false
}

const imageMaxAspectRatioInt = 3

func roundUpToImageGrid(value int) int {
	return (value + imageSizeMultiple - 1) / imageSizeMultiple * imageSizeMultiple
}

func roundDownToImageGrid(value int) int {
	return value / imageSizeMultiple * imageSizeMultiple
}

func ceilDiv(value, divisor int) int {
	return (value + divisor - 1) / divisor
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func filterPricedPixelSizes(routeModelID int64, taskType string, prices []RoutePriceConfig, pixelSizes []string) []string {
	pricedBaseResolutions := pricedBaseResolutionBuckets(routeModelID, taskType, prices)
	result := make([]string, 0, len(pixelSizes))
	for _, size := range pixelSizes {
		baseResolution, err := BaseResolutionByPixelSize(size)
		if _, priced := pricedBaseResolutions[baseResolution]; err == nil && priced {
			result = append(result, size)
		}
	}
	return result
}

func pricedBaseResolutionBuckets(routeModelID int64, taskType string, prices []RoutePriceConfig) map[string]struct{} {
	result := map[string]struct{}{}
	for _, price := range prices {
		if !price.Enabled || price.RouteModelID != routeModelID || taskType != "" && !strings.EqualFold(price.TaskType, taskType) {
			continue
		}
		result[strings.ToLower(strings.TrimSpace(price.BaseResolution))] = struct{}{}
	}
	return result
}

func (r *Resolver) visibleRouteModelLimits(routeModel RouteModelConfig, routing ModelRoutingSnapshot, taskType string) VisibleRouteModelTaskCapability {
	candidateByID := map[int64]ProviderCandidate{}
	for _, candidate := range routing.ProviderModels {
		candidateByID[candidate.AccountModelID] = candidate
	}
	sizeModes := map[string]struct{}{}
	baseResolution := map[string]struct{}{}
	ratios := map[string]struct{}{}
	pixelSizes := map[string]struct{}{}
	quality := map[string]struct{}{}
	outputFormat := map[string]struct{}{}
	moderation := map[string]struct{}{}
	backgrounds := map[string]struct{}{}
	maxOutputCount := 0
	maxReferenceCount := 0
	supportsOutputCompression := false
	supportsCustomSize := false
	supportsCustomRatio := false
	minWidth, maxWidth, minHeight, maxHeight := 0, 0, 0, 0
	hasCandidate := false
	for _, route := range routing.Candidates {
		if !route.Enabled || route.RouteModelID != routeModel.ID {
			continue
		}
		candidate, ok := candidateByID[route.AccountModelID]
		if !ok {
			continue
		}
		if !candidateHealthUsable(candidate.HealthStatus) {
			continue
		}
		candidate = normalizeProviderCandidate(candidate)
		if taskType != "" && len(candidate.SupportedTaskTypes) > 0 && !containsString(candidate.SupportedTaskTypes, taskType) {
			continue
		}
		firstCandidate := !hasCandidate
		hasCandidate = true
		intersectCapabilitySet(sizeModes, candidate.SizeModes, firstCandidate)
		intersectCapabilitySet(baseResolution, candidate.SupportedBaseResolution, firstCandidate)
		intersectCapabilitySet(ratios, candidate.SupportedAspectRatios, firstCandidate)
		intersectCapabilitySet(pixelSizes, candidate.SupportedPixelSizes, firstCandidate)
		intersectCapabilitySet(quality, candidate.Quality, firstCandidate)
		intersectCapabilitySet(outputFormat, candidate.OutputFormat, firstCandidate)
		intersectCapabilitySet(moderation, candidate.Moderation, firstCandidate)
		intersectCapabilitySet(backgrounds, candidate.SupportedBackgrounds, firstCandidate)
		if firstCandidate {
			supportsOutputCompression = candidate.SupportsOutputCompression
			supportsCustomSize = candidate.SupportsCustomSize && containsString(candidate.SizeModes, SizeModePixel)
			supportsCustomRatio = candidate.SupportsCustomRatio && containsString(candidate.SizeModes, SizeModeRatio)
			minWidth, maxWidth, minHeight, maxHeight = candidate.MinWidth, candidate.MaxWidth, candidate.MinHeight, candidate.MaxHeight
		} else {
			supportsOutputCompression = supportsOutputCompression && candidate.SupportsOutputCompression
			supportsCustomSize = supportsCustomSize && candidate.SupportsCustomSize && containsString(candidate.SizeModes, SizeModePixel)
			supportsCustomRatio = supportsCustomRatio && candidate.SupportsCustomRatio && containsString(candidate.SizeModes, SizeModeRatio)
			if candidate.MinWidth > minWidth {
				minWidth = candidate.MinWidth
			}
			if maxWidth == 0 || candidate.MaxWidth > 0 && candidate.MaxWidth < maxWidth {
				maxWidth = candidate.MaxWidth
			}
			if candidate.MinHeight > minHeight {
				minHeight = candidate.MinHeight
			}
			if maxHeight == 0 || candidate.MaxHeight > 0 && candidate.MaxHeight < maxHeight {
				maxHeight = candidate.MaxHeight
			}
		}
		if candidate.MaxImageCount > maxOutputCount {
			maxOutputCount = candidate.MaxImageCount
		}
		if candidate.MaxReferenceImageCount > maxReferenceCount {
			maxReferenceCount = candidate.MaxReferenceImageCount
		}
	}
	result := VisibleRouteModelTaskCapability{
		BaseResolution: sortedSet(baseResolution), Quality: sortedSet(quality), SizeModes: sortedSet(sizeModes), AspectRatios: sortedSet(ratios), PixelSizes: sortedSet(pixelSizes),
		OutputFormat: sortedSet(outputFormat), SupportsOutputCompression: supportsOutputCompression, SupportsCustomSize: supportsCustomSize, SupportsCustomRatio: supportsCustomRatio,
		MinWidth: minWidth, MaxWidth: maxWidth, MinHeight: minHeight, MaxHeight: maxHeight,
		SupportedBackgrounds: sortedSet(backgrounds), Moderation: sortedSet(moderation), MaxOutputImageCount: maxOutputCount, MaxReferenceImageCount: maxReferenceCount,
	}
	effective := FilterEffectiveCapability(ImageModelCapability{
		BaseResolution: result.BaseResolution, SizeModes: result.SizeModes, SupportedRatios: result.AspectRatios, SupportedPixelSizes: result.PixelSizes,
		SupportsCustomRatio: result.SupportsCustomRatio, SupportsCustomSize: result.SupportsCustomSize,
		MinWidth: result.MinWidth, MaxWidth: result.MaxWidth, MinHeight: result.MinHeight, MaxHeight: result.MaxHeight,
		SupportedBackgrounds: result.SupportedBackgrounds,
	})
	result.BaseResolution = cloneStringsOrEmpty(effective.BaseResolution)
	result.SizeModes = cloneStringsOrEmpty(effective.SizeModes)
	result.AspectRatios = cloneStringsOrEmpty(effective.SupportedRatios)
	result.PixelSizes = cloneStringsOrEmpty(effective.SupportedPixelSizes)
	result.SupportedBackgrounds = cloneStringsOrEmpty(effective.SupportedBackgrounds)
	result.SupportsCustomRatio = result.SupportsCustomRatio && containsString(result.SizeModes, SizeModeRatio)
	result.SupportsCustomSize = result.SupportsCustomSize && containsString(result.SizeModes, SizeModePixel)
	return result
}

func intersectCapabilitySet(target map[string]struct{}, values []string, first bool) {
	current := map[string]struct{}{}
	for _, value := range values {
		if item := strings.ToLower(strings.TrimSpace(value)); item != "" {
			current[item] = struct{}{}
		}
	}
	if first {
		for item := range current {
			target[item] = struct{}{}
		}
		return
	}
	for item := range target {
		if _, ok := current[item]; !ok {
			delete(target, item)
		}
	}
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
	SupportsCustomRatio       bool
	SupportedBackgrounds      []string
	OutputFormat              []string
	OutputCompression         int
	SupportsOutputCompression bool
	SupportsCustomSize        bool
	MinWidth                  int
	MaxWidth                  int
	MinHeight                 int
	MaxHeight                 int
	Moderation                []string
	MaxImageCount             int
	ConcurrencyLimit          int
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
	ResolvedSize           string
	RouteModelID           int64
	RouteModelCode         string
	RouteSnapshotVersion   string
	Providers              []ProviderCandidate
	MaxOutputImageCount    int
	MaxReferenceImageCount int
	RuntimeRoutingApplied  bool
	CapabilityVersion      string
	EffectiveMultiplier    string
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
	SupportsCustomSize        bool
	Moderation                []string
	MaxOutputImageCount       int
	MaxReferenceImageCount    int
}

type VisibleRouteModel struct {
	ID                           int64                                      `json:"id"`
	Code                         string                                     `json:"code"`
	Name                         string                                     `json:"name"`
	Description                  string                                     `json:"description,omitempty"`
	TaskTypes                    []string                                   `json:"task_types"`
	BaseResolution               []string                                   `json:"base_resolution"`
	AutoBaseResolutionByTaskType map[string]string                          `json:"auto_base_resolution_by_task_type"`
	Quality                      []string                                   `json:"quality"`
	SizeModes                    []string                                   `json:"size_modes"`
	AspectRatios                 []string                                   `json:"aspect_ratios"`
	PixelSizes                   []string                                   `json:"pixel_sizes"`
	OutputFormat                 []string                                   `json:"output_format"`
	SupportsOutputCompression    bool                                       `json:"supports_output_compression"`
	SupportsCustomSize           bool                                       `json:"supports_custom_size"`
	SupportsCustomRatio          bool                                       `json:"supports_custom_ratio"`
	SupportedBackgrounds         []string                                   `json:"supported_backgrounds"`
	MinWidth                     int                                        `json:"min_width,omitempty"`
	MaxWidth                     int                                        `json:"max_width,omitempty"`
	MinHeight                    int                                        `json:"min_height,omitempty"`
	MaxHeight                    int                                        `json:"max_height,omitempty"`
	Moderation                   []string                                   `json:"moderation"`
	MaxOutputImageCount          int                                        `json:"max_output_image_count"`
	MaxReferenceImageCount       int                                        `json:"max_reference_image_count"`
	CapabilitiesByTaskType       map[string]VisibleRouteModelTaskCapability `json:"capabilities_by_task_type"`
	EffectiveMultiplier          string                                     `json:"effective_multiplier"`
	Prices                       []VisibleRouteModelPrice                   `json:"prices"`
}

type VisibleRouteModelTaskCapability struct {
	BaseResolution            []string `json:"base_resolution"`
	AutoBaseResolution        string   `json:"auto_base_resolution,omitempty"`
	Quality                   []string `json:"quality"`
	SizeModes                 []string `json:"size_modes"`
	AspectRatios              []string `json:"aspect_ratios"`
	PixelSizes                []string `json:"pixel_sizes"`
	OutputFormat              []string `json:"output_format"`
	SupportsOutputCompression bool     `json:"supports_output_compression"`
	SupportsCustomSize        bool     `json:"supports_custom_size"`
	SupportsCustomRatio       bool     `json:"supports_custom_ratio"`
	SupportedBackgrounds      []string `json:"supported_backgrounds"`
	MinWidth                  int      `json:"min_width,omitempty"`
	MaxWidth                  int      `json:"max_width,omitempty"`
	MinHeight                 int      `json:"min_height,omitempty"`
	MaxHeight                 int      `json:"max_height,omitempty"`
	Moderation                []string `json:"moderation"`
	MaxOutputImageCount       int      `json:"max_output_image_count"`
	MaxReferenceImageCount    int      `json:"max_reference_image_count"`
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

	baseResolution, source := resolveAutoRouteBaseResolution(routeModel, taskType, autoDefaults, prices)
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

func resolveAutoRouteBaseResolution(routeModel RouteModelConfig, taskType string, autoDefaults map[string]string, prices []RoutePriceConfig) (string, string) {
	baseResolution := strings.ToLower(strings.TrimSpace(autoDefaults[strings.ToLower(routeModel.Code)]))
	if baseResolution != "" && hasRoutePrice(routeModel.ID, taskType, baseResolution, prices) {
		return baseResolution, "route_model_default"
	}
	return firstRouteBaseResolution(routeModel.ID, taskType, prices), "first_configured_price"
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
	if strings.TrimSpace(req.RouteModelCode) != "" {
		return r.resolveRouteContext(ctx, req)
	}
	normalizedReq, err := NormalizeResolveRequest(req)
	if err != nil {
		return ResolvedRequest{}, err
	}
	req = normalizedReq
	model := strings.ToLower(req.AbstractModel)
	baseResolution, err := r.ResolveBaseResolution(req.BaseResolution, req.RequestedSize, model)
	if err != nil {
		return ResolvedRequest{}, err
	}
	if req.RequestedOutputImageCount <= 0 {
		req.RequestedOutputImageCount = 1
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
			if !CandidateSupportsRequest(candidate, req, baseResolution) {
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
	if !strings.EqualFold(strings.TrimSpace(req.SizeMode), sizeModeLegacyRatio) {
		if _, err := normalizeCandidateGenerationRequest(candidate, req); err != nil {
			return false
		}
		return true
	}
	switch mode {
	case SizeModeAuto:
		return true
	case SizeModePixel:
		size := NormalizePixelSize(req.RequestedSize)
		if size == "" {
			return false
		}
		width, height, ok := ParseImageSize(size)
		if !ok || !IsLegalCustomImageSize(width, height) {
			return false
		}
		if !candidate.SupportsCustomSize && !containsString(candidate.SupportedPixelSizes, size) {
			return false
		}
		return true
	default:
		ratio := NormalizeRatio(req.AspectRatio)
		if ratio == "" {
			return false
		}
		if !strings.EqualFold(strings.TrimSpace(req.SizeMode), sizeModeLegacyRatio) && len(candidate.SupportedAspectRatios) > 0 && !candidate.SupportsCustomRatio && !containsString(candidate.SupportedAspectRatios, ratio) {
			return false
		}
		if len(candidate.SupportedBaseResolution) > 0 && !containsString(candidate.SupportedBaseResolution, resolvedBaseResolution) {
			return false
		}
		return true
	}
}

func NormalizeCandidateGenerationRequest(candidate ProviderCandidate, req ResolveRequest) (NormalizedGenerationRequest, error) {
	return normalizeCandidateGenerationRequest(normalizeProviderCandidate(candidate), req)
}

func normalizeCandidateGenerationRequest(candidate ProviderCandidate, req ResolveRequest) (NormalizedGenerationRequest, error) {
	return NormalizeGenerationRequest(ImageModelCapability{
		SizeModes: candidate.SizeModes, BaseResolution: candidate.SupportedBaseResolution,
		SupportedRatios: candidate.SupportedAspectRatios, SupportedPixelSizes: candidate.SupportedPixelSizes,
		SupportsCustomRatio: candidate.SupportsCustomRatio, SupportsCustomSize: candidate.SupportsCustomSize,
		MinWidth: candidate.MinWidth, MaxWidth: candidate.MaxWidth, MinHeight: candidate.MinHeight, MaxHeight: candidate.MaxHeight,
		OutputFormat: candidate.OutputFormat, SupportedBackgrounds: candidate.SupportedBackgrounds,
	}, GenerationRequest{
		SizeMode: PublicSizeMode(req.SizeMode), BaseResolution: req.BaseResolution, AspectRatio: req.AspectRatio,
		RequestedSize: req.RequestedSize, Background: req.Background, OutputFormat: req.OutputFormat,
	})
}

func normalizeProviderCandidate(candidate ProviderCandidate) ProviderCandidate {
	hasConfiguredSizeModes := len(candidate.SizeModes) > 0
	hasConfiguredRatios := len(candidate.SupportedAspectRatios) > 0
	hasConfiguredBaseResolutions := len(candidate.SupportedBaseResolution) > 0
	hasConfiguredPixelSizes := len(candidate.SupportedPixelSizes) > 0
	sizeModes := candidate.SizeModes
	if !hasConfiguredSizeModes {
		sizeModes = cloneStrings(DefaultSizeModes)
	}
	baseResolutions := candidate.SupportedBaseResolution
	if !hasConfiguredBaseResolutions {
		baseResolutions = []string{"1k", "2k", "4k"}
	}
	aspectRatios := candidate.SupportedAspectRatios
	if !hasConfiguredRatios && containsString(sizeModes, SizeModeRatio) {
		aspectRatios = cloneStrings(DefaultSupportedRatios)
	}
	pixelSizes := candidate.SupportedPixelSizes
	if !hasConfiguredPixelSizes && containsString(sizeModes, SizeModePixel) {
		pixelSizes = cloneStrings(DefaultSupportedPixelSizes)
	}
	maxImageCount := candidate.MaxImageCount
	if maxImageCount == 0 {
		maxImageCount = 1
	}
	capability := FilterEffectiveCapability(ImageModelCapability{
		MaxReferenceImageCount:    candidate.MaxReferenceImageCount,
		MaxImageCount:             maxImageCount,
		BaseResolution:            baseResolutions,
		Quality:                   candidate.Quality,
		SizeModes:                 sizeModes,
		SupportedRatios:           aspectRatios,
		SupportedPixelSizes:       pixelSizes,
		SupportsCustomRatio:       candidate.SupportsCustomRatio,
		SupportedBackgrounds:      candidate.SupportedBackgrounds,
		OutputFormat:              candidate.OutputFormat,
		OutputCompression:         candidate.OutputCompression,
		SupportsOutputCompression: candidate.SupportsOutputCompression,
		SupportsCustomSize:        candidate.SupportsCustomSize,
		MinWidth:                  candidate.MinWidth,
		MaxWidth:                  candidate.MaxWidth,
		MinHeight:                 candidate.MinHeight,
		MaxHeight:                 candidate.MaxHeight,
		Moderation:                candidate.Moderation,
	})
	capability.Quality = normalizeEnumStrings(defaultStrings(candidate.Quality, DefaultQuality), map[string]struct{}{"auto": {}, "low": {}, "medium": {}, "high": {}})
	capability.OutputFormat = normalizeEnumStrings(defaultStrings(candidate.OutputFormat, DefaultOutputFormat), map[string]struct{}{"png": {}, "jpeg": {}, "webp": {}})
	capability.Moderation = normalizeEnumStrings(defaultStrings(candidate.Moderation, DefaultModeration), map[string]struct{}{"auto": {}, "low": {}})
	candidate.MaxReferenceImageCount = capability.MaxReferenceImageCount
	candidate.MaxImageCount = capability.MaxImageCount
	candidate.SupportedBaseResolution = capability.BaseResolution
	candidate.Quality = capability.Quality
	candidate.SizeModes = capability.SizeModes
	candidate.SupportedAspectRatios = capability.SupportedRatios
	candidate.SupportedPixelSizes = capability.SupportedPixelSizes
	candidate.SupportsCustomRatio = capability.SupportsCustomRatio
	candidate.SupportedBackgrounds = capability.SupportedBackgrounds
	candidate.OutputFormat = capability.OutputFormat
	candidate.OutputCompression = capability.OutputCompression
	candidate.SupportsOutputCompression = capability.SupportsOutputCompression
	candidate.SupportsCustomSize = capability.SupportsCustomSize
	candidate.MinWidth, candidate.MaxWidth = capability.MinWidth, capability.MaxWidth
	candidate.MinHeight, candidate.MaxHeight = capability.MinHeight, capability.MaxHeight
	candidate.Moderation = capability.Moderation
	if candidate.MaxReferenceImageCount > 0 {
		candidate.SupportsImageInput = true
	}
	return candidate
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
	multiplier, ok := effectiveMultiplier(routeModel, matchedGroups)
	if !ok {
		return ResolvedRequest{}, errs.New(403, errs.CodeModelRouteNotVisible, "route model is not visible")
	}
	partial := ResolvedRequest{
		RouteModelID:          routeModel.ID,
		RouteModelCode:        routeModel.Code,
		RouteSnapshotVersion:  routing.Version,
		RuntimeRoutingApplied: true,
		EffectiveMultiplier:   multiplier.StringFixed(5),
	}
	visibleCapability := r.visibleRouteModelTaskCapability(routeModel, routing, req.TaskType)
	visibleCapability.AutoBaseResolution, _ = resolveAutoRouteBaseResolution(routeModel, req.TaskType, r.cfg.Billing.AutoBaseResolutionDefaultByGroup, routing.Prices)
	capabilityVersion, err := r.routeCapabilityVersion(routeModel, routing, req.TaskType, matchedGroups, visibleCapability)
	if err != nil {
		return partial, err
	}
	partial.CapabilityVersion = capabilityVersion
	if expected := strings.TrimSpace(req.ExpectedCapabilityVersion); expected != "" && expected != capabilityVersion {
		return partial, errs.New(409, CodeCapabilityChanged, "model capability changed; refresh the estimate and try again")
	}
	normalizedReq, err := NormalizeResolveRequest(req)
	if err != nil {
		return partial, err
	}
	req = normalizedReq
	var normalizedGeneration NormalizedGenerationRequest
	trustedResolvedSize := NormalizePixelSize(req.TrustedResolvedSize)
	if trustedResolvedSize != "" {
		partial.ResolvedSize = trustedResolvedSize
	} else if !strings.EqualFold(strings.TrimSpace(req.SizeMode), sizeModeLegacyRatio) {
		normalizedGeneration, err = normalizeVisibleRouteRequest(visibleCapability, req)
		if err != nil {
			return partial, err
		}
		partial.ResolvedSize = normalizedGeneration.OutboundSize
	}
	if PublicSizeMode(req.SizeMode) == SizeModePixel {
		pixelBaseResolution, pixelErr := BaseResolutionByPixelSize(req.RequestedSize)
		if pixelErr != nil || !hasRoutePrice(routeModel.ID, req.TaskType, pixelBaseResolution, routing.Prices) {
			return partial, errs.New(400, CodeInvalidExplicitDimensions, "explicit dimensions do not have an enabled price bucket")
		}
	}
	baseResolution := strings.ToLower(strings.TrimSpace(req.BaseResolution))
	baseResolution, err = ResolveRouteBaseResolutionBySizeMode(routeModel, req, r.cfg.Billing.AutoBaseResolutionDefaultByGroup, routing.Prices)
	if err != nil {
		return partial, err
	}
	partial.BaseResolution = baseResolution
	if trustedResolvedSize == "" && strings.EqualFold(strings.TrimSpace(req.SizeMode), sizeModeLegacyRatio) {
		legacyValidation := req
		legacyValidation.SizeMode = SizeModeRatio
		legacyValidation.BaseResolution = baseResolution
		legacyValidation.RequestedSize = ""
		normalizedGeneration, err = normalizeVisibleRouteRequest(visibleCapability, legacyValidation)
		if err != nil {
			return partial, err
		}
		partial.ResolvedSize = normalizedGeneration.OutboundSize
	}
	if req.RequestedOutputImageCount <= 0 {
		req.RequestedOutputImageCount = 1
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
		if trustedResolvedSize != "" {
			candidate = candidateWithTrustedSizeCapability(candidate, req, baseResolution, trustedResolvedSize)
		}
		if !CandidateSupportsRequest(candidate, req, baseResolution) {
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
		return partial, errs.New(400, errs.CodeImageCapabilityMismatch, "当前配置暂不支持生成，请更换类似配置。")
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
		ResolvedSize:           partial.ResolvedSize,
		RouteModelID:           routeModel.ID,
		RouteModelCode:         routeModel.Code,
		RouteSnapshotVersion:   routing.Version,
		Providers:              candidates,
		MaxOutputImageCount:    r.cfg.GenerationLimits.MaxImageCount,
		MaxReferenceImageCount: r.cfg.GenerationLimits.ReferenceImageMaxCount,
		RuntimeRoutingApplied:  true,
		CapabilityVersion:      capabilityVersion,
		EffectiveMultiplier:    multiplier.StringFixed(5),
	}, nil
}

func candidateWithTrustedSizeCapability(candidate ProviderCandidate, req ResolveRequest, baseResolution, resolvedSize string) ProviderCandidate {
	mode := PublicSizeMode(req.SizeMode)
	candidate.SizeModes = appendUnique(candidate.SizeModes, mode)
	switch mode {
	case SizeModeRatio:
		candidate.SupportedBaseResolution = appendUnique(candidate.SupportedBaseResolution, strings.ToLower(strings.TrimSpace(baseResolution)))
		candidate.SupportedAspectRatios = appendUnique(candidate.SupportedAspectRatios, NormalizeRatio(req.AspectRatio))
		candidate.SupportsCustomRatio = true
		candidate.MinWidth, candidate.MaxWidth = 0, 0
		candidate.MinHeight, candidate.MaxHeight = 0, 0
	case SizeModePixel:
		candidate.SupportedPixelSizes = appendUnique(candidate.SupportedPixelSizes, resolvedSize)
		candidate.SupportsCustomSize = true
		candidate.MinWidth, candidate.MaxWidth = 0, 0
		candidate.MinHeight, candidate.MaxHeight = 0, 0
	}
	return candidate
}

type capabilityVersionCandidate struct {
	AccountModelID         int64    `json:"account_model_id"`
	Provider               string   `json:"provider"`
	ModelCode              string   `json:"model_code"`
	Priority               int      `json:"priority"`
	Weight                 int      `json:"weight"`
	FallbackOrder          int      `json:"fallback_order"`
	BaseResolution         []string `json:"base_resolution"`
	SizeModes              []string `json:"size_modes"`
	AspectRatios           []string `json:"aspect_ratios"`
	PixelSizes             []string `json:"pixel_sizes"`
	Quality                []string `json:"quality"`
	OutputFormat           []string `json:"output_format"`
	Backgrounds            []string `json:"backgrounds"`
	Moderation             []string `json:"moderation"`
	SupportsCustomRatio    bool     `json:"supports_custom_ratio"`
	SupportsCustomSize     bool     `json:"supports_custom_size"`
	SupportsCompression    bool     `json:"supports_compression"`
	SupportsImageInput     bool     `json:"supports_image_input"`
	SupportsMask           bool     `json:"supports_mask"`
	MinWidth               int      `json:"min_width"`
	MaxWidth               int      `json:"max_width"`
	MinHeight              int      `json:"min_height"`
	MaxHeight              int      `json:"max_height"`
	MaxImageCount          int      `json:"max_image_count"`
	MaxReferenceImageCount int      `json:"max_reference_image_count"`
}

type capabilityVersionPrice struct {
	BaseResolution string `json:"base_resolution"`
	BasePoints     string `json:"base_points"`
}

func (r *Resolver) routeCapabilityVersion(routeModel RouteModelConfig, routing ModelRoutingSnapshot, taskType string, matchedGroups []UserGroupConfig, capability VisibleRouteModelTaskCapability) (string, error) {
	multiplier, ok := effectiveMultiplier(routeModel, matchedGroups)
	if !ok {
		return "", errs.New(403, errs.CodeModelRouteNotVisible, "route model is not visible")
	}
	accountModels := make(map[int64]ProviderCandidate, len(routing.ProviderModels))
	for _, candidate := range routing.ProviderModels {
		accountModels[candidate.AccountModelID] = candidate
	}
	candidates := make([]capabilityVersionCandidate, 0, len(routing.Candidates))
	for _, route := range routing.Candidates {
		candidate, exists := accountModels[route.AccountModelID]
		if !route.Enabled || route.RouteModelID != routeModel.ID || !exists || !candidateHealthUsable(candidate.HealthStatus) {
			continue
		}
		candidate = normalizeProviderCandidate(candidate)
		if len(candidate.SupportedTaskTypes) > 0 && !containsString(candidate.SupportedTaskTypes, taskType) {
			continue
		}
		candidates = append(candidates, capabilityVersionCandidate{
			AccountModelID: candidate.AccountModelID,
			Provider:       strings.ToLower(strings.TrimSpace(candidate.Provider)), ModelCode: strings.TrimSpace(candidate.ModelCode),
			Priority: route.Priority, Weight: route.Weight, FallbackOrder: route.FallbackOrder,
			BaseResolution: sortedStrings(candidate.SupportedBaseResolution), SizeModes: sortedStrings(candidate.SizeModes),
			AspectRatios: sortedStrings(candidate.SupportedAspectRatios), PixelSizes: sortedStrings(candidate.SupportedPixelSizes),
			Quality: sortedStrings(candidate.Quality), OutputFormat: sortedStrings(candidate.OutputFormat),
			Backgrounds: sortedStrings(candidate.SupportedBackgrounds), Moderation: sortedStrings(candidate.Moderation),
			SupportsCustomRatio: candidate.SupportsCustomRatio, SupportsCustomSize: candidate.SupportsCustomSize,
			SupportsCompression: candidate.SupportsOutputCompression,
			SupportsImageInput:  candidate.SupportsImageInput,
			SupportsMask:        candidate.SupportsMask,
			MinWidth:            candidate.MinWidth, MaxWidth: candidate.MaxWidth, MinHeight: candidate.MinHeight, MaxHeight: candidate.MaxHeight,
			MaxImageCount: candidate.MaxImageCount, MaxReferenceImageCount: candidate.MaxReferenceImageCount,
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority < candidates[j].Priority
		}
		if candidates[i].FallbackOrder != candidates[j].FallbackOrder {
			return candidates[i].FallbackOrder < candidates[j].FallbackOrder
		}
		return candidates[i].AccountModelID < candidates[j].AccountModelID
	})
	prices := make([]capabilityVersionPrice, 0)
	for _, price := range routing.Prices {
		if !price.Enabled || price.RouteModelID != routeModel.ID || !strings.EqualFold(price.TaskType, taskType) {
			continue
		}
		prices = append(prices, capabilityVersionPrice{
			BaseResolution: strings.ToLower(strings.TrimSpace(price.BaseResolution)),
			BasePoints:     canonicalDecimal(price.BasePoints),
		})
	}
	sort.Slice(prices, func(i, j int) bool {
		if prices[i].BaseResolution != prices[j].BaseResolution {
			return prices[i].BaseResolution < prices[j].BaseResolution
		}
		if prices[i].BasePoints != prices[j].BasePoints {
			return prices[i].BasePoints < prices[j].BasePoints
		}
		return false
	})
	projection := struct {
		RouteCode      string                          `json:"route_code"`
		TaskType       string                          `json:"task_type"`
		Capability     VisibleRouteModelTaskCapability `json:"capability"`
		Candidates     []capabilityVersionCandidate    `json:"candidates"`
		Prices         []capabilityVersionPrice        `json:"prices"`
		Multiplier     string                          `json:"multiplier"`
		TaskMultiplier string                          `json:"task_multiplier"`
	}{
		RouteCode: strings.ToLower(strings.TrimSpace(routeModel.Code)), TaskType: strings.ToLower(strings.TrimSpace(taskType)),
		Capability: capability, Candidates: candidates, Prices: prices, Multiplier: multiplier.String(),
		TaskMultiplier: canonicalDecimal(defaultTaskMultiplier(r.cfg.Billing.TaskMultipliers[taskType])),
	}
	payload, err := json.Marshal(projection)
	if err != nil {
		return "", errs.Internal("failed to encode effective model capability")
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func sortedStrings(values []string) []string {
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		if normalized := strings.ToLower(strings.TrimSpace(value)); normalized != "" {
			unique[normalized] = struct{}{}
		}
	}
	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func canonicalDecimal(value string) string {
	parsed, err := decimal.NewFromString(strings.TrimSpace(value))
	if err != nil {
		return strings.TrimSpace(value)
	}
	return parsed.String()
}

func defaultTaskMultiplier(value string) string {
	if strings.TrimSpace(value) == "" {
		return "1.00000"
	}
	return value
}

func normalizeVisibleRouteRequest(capability VisibleRouteModelTaskCapability, req ResolveRequest) (NormalizedGenerationRequest, error) {
	if !containsString(capability.Quality, req.Quality) {
		return NormalizedGenerationRequest{}, errs.New(400, errs.CodeImageCapabilityMismatch, "quality is unsupported")
	}
	if !containsString(capability.Moderation, req.Moderation) {
		return NormalizedGenerationRequest{}, errs.New(400, errs.CodeImageCapabilityMismatch, "moderation is unsupported")
	}
	if !containsString(capability.OutputFormat, req.OutputFormat) {
		return NormalizedGenerationRequest{}, errs.New(400, errs.CodeImageCapabilityMismatch, "output_format is unsupported")
	}
	if req.OutputCompression < 100 && (!capability.SupportsOutputCompression || (req.OutputFormat != "jpeg" && req.OutputFormat != "webp")) {
		return NormalizedGenerationRequest{}, errs.New(400, errs.CodeImageCapabilityMismatch, "output_compression is unsupported")
	}
	return NormalizeGenerationRequest(ImageModelCapability{
		SizeModes:                 capability.SizeModes,
		BaseResolution:            capability.BaseResolution,
		SupportedRatios:           capability.AspectRatios,
		SupportedPixelSizes:       capability.PixelSizes,
		SupportsCustomRatio:       capability.SupportsCustomRatio,
		SupportedBackgrounds:      capability.SupportedBackgrounds,
		OutputFormat:              capability.OutputFormat,
		SupportsCustomSize:        capability.SupportsCustomSize,
		MinWidth:                  capability.MinWidth,
		MaxWidth:                  capability.MaxWidth,
		MinHeight:                 capability.MinHeight,
		MaxHeight:                 capability.MaxHeight,
		SupportsOutputCompression: capability.SupportsOutputCompression,
	}, GenerationRequest{
		SizeMode:       PublicSizeMode(req.SizeMode),
		BaseResolution: req.BaseResolution,
		AspectRatio:    req.AspectRatio,
		RequestedSize:  req.RequestedSize,
		Background:     req.Background,
		OutputFormat:   req.OutputFormat,
	})
}

func firstRouteBaseResolution(routeModelID int64, taskType string, prices []RoutePriceConfig) string {
	baseResolution := []string{}
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
			BaseResolution:         sortedKeys(r.cfg.Billing.BaseResolutionPointsByModel[model]),
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

func cloneStringsOrEmpty(values []string) []string {
	return append(make([]string, 0, len(values)), values...)
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
