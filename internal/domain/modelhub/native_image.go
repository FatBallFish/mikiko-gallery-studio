package modelhub

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

// NativeImageConfig is stored in account/model extra.native_image; model values take precedence.
type NativeImageConfig struct {
	Defaults    *nativeimage.Defaults          `json:"defaults,omitempty"`
	Model       string                         `json:"model"`
	Profile     *nativeimage.CapabilityProfile `json:"profile,omitempty"`
	Pricing     *nativeimage.Pricing           `json:"pricing,omitempty"`
	SalePricing *nativeimage.Pricing           `json:"sale_pricing,omitempty"`
}

func IsNativeImageCandidate(c ProviderCandidate) bool {
	return c.AdapterType == nativeimage.AdapterQwen || c.AdapterType == nativeimage.AdapterSeedream
}
func NativeConfig(c ProviderCandidate) (NativeImageConfig, error) {
	var cfg NativeImageConfig
	for _, extra := range []map[string]any{c.AccountExtra, c.ModelExtra} {
		if raw, ok := extra["native_image"]; ok {
			b, e := json.Marshal(raw)
			if e != nil {
				return cfg, e
			}
			if e = json.Unmarshal(b, &cfg); e != nil {
				return cfg, e
			}
		}
	}
	if cfg.Model == "" {
		cfg.Model = c.ModelCode
	}
	p, ok := nativeimage.ProfileForModel(cfg.Model)
	if cfg.Profile != nil {
		if !ok || cfg.Profile.Model != cfg.Model || cfg.Profile.Version == "" {
			return cfg, fmt.Errorf("invalid native profile model/version")
		}
		p = *cfg.Profile
	}
	if err := nativeimage.ValidateProfile(p); err != nil {
		return cfg, err
	}
	if !ok || math.IsNaN(p.MaxAspectRatio) || math.IsInf(p.MaxAspectRatio, 0) || p.Adapter != c.AdapterType || p.MinPixels <= 0 || p.MaxPixels < p.MinPixels || p.MaxAspectRatio < 1 || p.MaxReferenceImages < 0 || p.MaxOutputImages < 1 {
		return cfg, fmt.Errorf("invalid or unknown native image model profile")
	}
	options, err := nativeimage.EffectiveOptions(p, cfg.Defaults, nil)
	if err != nil {
		return cfg, err
	}
	cfg.Defaults = &nativeimage.Defaults{Seed: options.Seed, Watermark: options.Watermark, Qwen: options.Qwen, Seedream: options.Seedream}
	cfg.Profile = &p
	return cfg, nil
}
func NativeOperation(op nativeimage.Operation, references int) nativeimage.Operation {
	if op != "" {
		return op
	}
	if references > 0 {
		return nativeimage.OperationEdit
	}
	return nativeimage.OperationGenerate
}
func nativeSize(req ResolveRequest) string {
	if req.TrustedResolvedSize != "" {
		return req.TrustedResolvedSize
	}
	if req.RequestedSize != "" {
		return req.RequestedSize
	}
	if req.BaseResolution != "" && req.BaseResolution != "auto" {
		return strings.ToUpper(req.BaseResolution)
	}
	return ""
}
func nativeCandidateRequest(c ProviderCandidate, req ResolveRequest) (NormalizedGenerationRequest, error) {
	cfg, err := NativeConfig(c)
	if err != nil {
		return NormalizedGenerationRequest{}, err
	}
	if req.MaskPresent || req.Quality != "" && req.Quality != "auto" || req.Moderation != "" && req.Moderation != "auto" || req.OutputCompression > 0 && req.OutputCompression < 100 {
		return NormalizedGenerationRequest{}, fmt.Errorf("native model does not support generic mask, quality, moderation or compression")
	}
	size := nativeSize(req)
	if req.AspectRatio != "" {
		return NormalizedGenerationRequest{}, fmt.Errorf("native image requires explicit dimensions or native size tier")
	}
	options := req.NativeOptions
	if !req.PreserveNativeOptions {
		options, err = nativeimage.EffectiveOptions(*cfg.Profile, cfg.Defaults, req.NativeOptions)
		if err != nil {
			return NormalizedGenerationRequest{}, err
		}
	}
	err = nativeimage.ValidateRequest(*cfg.Profile, nativeimage.ValidationRequest{Operation: NativeOperation(req.Operation, req.ReferenceImageCount), Prompt: "validation", Size: size, OutputFormat: req.OutputFormat, Background: req.Background, Count: req.RequestedOutputImageCount, ReferenceCount: req.ReferenceImageCount, Options: options})
	return NormalizedGenerationRequest{OutboundSize: size}, err
}
func nativeCapabilityCNYPerPoint(route RouteModelConfig, routing ModelRoutingSnapshot, value string) string {
	if route.ImagePricing != nil || hasNativeRoute(route, routing) {
		return canonicalDecimal(value)
	}
	return ""
}
func hasNativeRoute(route RouteModelConfig, routing ModelRoutingSnapshot) bool {
	for _, rc := range routing.Candidates {
		if rc.Enabled && rc.RouteModelID == route.ID {
			for _, c := range routing.ProviderModels {
				if c.AccountModelID == rc.AccountModelID && IsNativeImageCandidate(c) {
					return true
				}
			}
		}
	}
	return false
}
func nativeRouteProfile(route RouteModelConfig, routing ModelRoutingSnapshot) *nativeimage.CapabilityProfile {
	var profile *nativeimage.CapabilityProfile
	for _, rc := range routing.Candidates {
		if !rc.Enabled || rc.RouteModelID != route.ID {
			continue
		}
		for _, c := range routing.ProviderModels {
			if c.AccountModelID != rc.AccountModelID || !candidateHealthUsable(c.HealthStatus) {
				continue
			}
			if !IsNativeImageCandidate(c) {
				return nil
			}
			cfg, err := NativeConfig(c)
			if err != nil {
				return nil
			}
			if profile == nil {
				p := *cfg.Profile
				profile = &p
			} else { // Every advertised option must survive fallback to every candidate.
				other := cfg.Profile
				if profile.Model != other.Model || profile.Adapter != other.Adapter {
					return nil
				}
				profile.Operations = nativeProfileIntersection(profile.Operations, other.Operations)
				profile.InputFormats = nativeProfileIntersection(profile.InputFormats, other.InputFormats)
				profile.OutputFormats = nativeProfileIntersection(profile.OutputFormats, other.OutputFormats)
				profile.SizeModes = nativeProfileIntersection(profile.SizeModes, other.SizeModes)
				profile.LayerSizeModes = nativeProfileIntersection(profile.LayerSizeModes, other.LayerSizeModes)
				profile.OptimizePromptModes = nativeProfileIntersection(profile.OptimizePromptModes, other.OptimizePromptModes)
				profile.MinPixels = max(profile.MinPixels, other.MinPixels)
				profile.MaxPixels = min(profile.MaxPixels, other.MaxPixels)
				profile.MaxAspectRatio = min(profile.MaxAspectRatio, other.MaxAspectRatio)
				profile.MaxReferenceImages = min(profile.MaxReferenceImages, other.MaxReferenceImages)
				profile.MaxOutputImages = min(profile.MaxOutputImages, other.MaxOutputImages)
				profile.MaxLayers = min(profile.MaxLayers, other.MaxLayers)
				profile.NegativePrompt = profile.NegativePrompt && other.NegativePrompt
				profile.Seed = profile.Seed && other.Seed
				profile.LayerDecomposition = profile.LayerDecomposition && other.LayerDecomposition
			}
		}
	}
	if profile != nil && (nativeimage.ValidateProfile(*profile) != nil || len(profile.SizeModes) == 0) {
		return nil
	}
	return profile
}

func nativeProfileIntersection[T comparable](left, right []T) []T {
	if left == nil || right == nil {
		return nil
	}
	result := make([]T, 0, len(left))
	for _, value := range left {
		for _, other := range right {
			if value == other {
				result = append(result, value)
				break
			}
		}
	}
	return result
}
func (r *Resolver) resolveNativeRoute(req ResolveRequest, route RouteModelConfig, routing ModelRoutingSnapshot, partial ResolvedRequest) (ResolvedRequest, error) {
	if req.ReferenceImageCount < 0 || req.RequestedOutputImageCount < 0 {
		return partial, errs.BadRequest("negative image counts")
	}
	for _, rc := range routing.Candidates {
		if !rc.Enabled || rc.RouteModelID != route.ID {
			continue
		}
		for _, c := range routing.ProviderModels {
			if c.AccountModelID != rc.AccountModelID || !IsNativeImageCandidate(c) || !candidateHealthUsable(c.HealthStatus) {
				continue
			}
			n, err := nativeCandidateRequest(c, req)
			if err != nil {
				continue
			}
			cfg, _ := NativeConfig(c)
			options := req.NativeOptions
			if !req.PreserveNativeOptions {
				options, err = nativeimage.EffectiveOptions(*cfg.Profile, cfg.Defaults, req.NativeOptions)
				if err != nil {
					continue
				}
			}
			// A route must have one effective request across all fallback candidates.
			if partial.NativeOptions != nil && !reflect.DeepEqual(partial.NativeOptions, options) {
				return partial, errs.BadRequest("native route candidates have conflicting effective defaults")
			}
			partial.NativeOptions = options
			c.RouteModelID = route.ID
			c.RouteModelCode = route.Code
			c.Priority = rc.Priority
			c.WeightPercent = rc.Weight
			c.FallbackOrder = rc.FallbackOrder
			c.RouteSnapshotVersion = routing.Version
			partial.Providers = append(partial.Providers, c)
			partial.ResolvedSize = n.OutboundSize
		}
	}
	if len(partial.Providers) == 0 {
		return partial, errs.New(400, errs.CodeImageCapabilityMismatch, "no native model supports all requested image parameters")
	}
	sort.SliceStable(partial.Providers, func(i, j int) bool {
		a, b := partial.Providers[i], partial.Providers[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		if a.FallbackOrder != b.FallbackOrder {
			return a.FallbackOrder < b.FallbackOrder
		}
		return a.AccountModelID < b.AccountModelID
	})
	partial.BaseResolution = strings.ToLower(req.BaseResolution)
	if partial.BaseResolution == "" || partial.BaseResolution == "auto" {
		partial.BaseResolution = strings.ToLower(partial.ResolvedSize)
		if _, _, e := nativeimage.ParseDimensions(partial.ResolvedSize); e == nil {
			partial.BaseResolution = "1k"
		}
		if w, h, e := nativeimage.ParseDimensions(partial.ResolvedSize); e == nil && w*h > 2610000 {
			partial.BaseResolution = "2k"
		}
	}
	partial.MaxOutputImageCount = r.cfg.GenerationLimits.MaxImageCount
	partial.MaxReferenceImageCount = r.cfg.GenerationLimits.ReferenceImageMaxCount
	if req.ReferenceImageCount > partial.MaxReferenceImageCount || req.RequestedOutputImageCount > partial.MaxOutputImageCount {
		return partial, errs.BadRequest("image count exceeds platform limits")
	}
	return partial, nil
}
