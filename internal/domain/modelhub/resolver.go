package modelhub

import (
	"fmt"
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
}

type ProviderCandidate struct {
	Provider               string
	SupportedTaskTypes     []string
	SupportedQualities     []string
	SupportedAspectRatios  []string
	MaxImageCount          int
	MaxReferenceImageCount int
	SupportsImageInput     bool
	SupportsMask           bool
	Priority               int
}

type ResolvedRequest struct {
	ResolvedQualityBucket  string
	Providers              []ProviderCandidate
	MaxOutputImageCount    int
	MaxReferenceImageCount int
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
	cfg config.Config
}

func NewResolver(cfg config.Config) *Resolver {
	return &Resolver{cfg: cfg}
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

	candidates := make([]ProviderCandidate, 0, len(r.cfg.Routing.ProviderCapabilities))
	for providerName, capability := range r.cfg.Routing.ProviderCapabilities {
		if !r.providerEnabled(providerName) {
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
	switch strings.ToLower(name) {
	case "openai":
		return r.cfg.Providers.OpenAI.Enabled
	case "openrouter":
		return r.cfg.Providers.OpenRouter.Enabled
	default:
		return false
	}
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
