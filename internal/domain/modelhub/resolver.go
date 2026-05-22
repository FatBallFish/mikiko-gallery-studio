package modelhub

import (
	"context"
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type ResolveRequest struct {
	AbstractModel             string
	TaskType                  string
	RequestedQuality          string
	RequestedSize             string
	RequestedOutputImageCount int
	ReferenceImageCount       int
	MaskPresent               bool
	RouteKey                  string
}

type ProviderCandidate struct {
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
	quality := strings.ToLower(requestedQuality)
	if quality == "1k" || quality == "2k" || quality == "4k" {
		return quality, nil
	}
	if requestedSize != "" && !strings.EqualFold(requestedSize, "auto") {
		parts := strings.Split(strings.ToLower(requestedSize), "x")
		if len(parts) != 2 {
			return "", errs.New(400, errs.CodeImageAutoUnsupported, "unsupported size")
		}
		width, errW := strconv.Atoi(parts[0])
		height, errH := strconv.Atoi(parts[1])
		if errW != nil || errH != nil {
			return "", errs.New(400, errs.CodeImageAutoUnsupported, "unsupported size")
		}
		longest := width
		if height > longest {
			longest = height
		}
		switch {
		case longest <= 1024:
			return "1k", nil
		case longest <= 2048:
			return "2k", nil
		case longest <= 4096:
			return "4k", nil
		default:
			return "", errs.New(400, errs.CodeImageAutoUnsupported, "unsupported size")
		}
	}
	value := r.cfg.Billing.AutoQualityDefaultByGroup[strings.ToLower(abstractModel)]
	if value == "" {
		return "", errs.New(400, errs.CodeImageCapabilityMismatch, fmt.Sprintf("unsupported abstract model %s", abstractModel))
	}
	return value, nil
}

func (r *Resolver) Resolve(req ResolveRequest) (ResolvedRequest, error) {
	return r.ResolveContext(context.Background(), req)
}

func (r *Resolver) ResolveContext(ctx context.Context, req ResolveRequest) (ResolvedRequest, error) {
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
			if candidate.HealthStatus != "" && !strings.EqualFold(candidate.HealthStatus, "healthy") && !strings.EqualFold(candidate.HealthStatus, "unknown") {
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
