package modelhub

import (
	"github.com/fatballfish/pic-gallery/internal/domain/imagepricing"
	"math"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
)

// nativeRouteLayerPricingConfirmed only advertises layers when every usable
// fallback has a complete, explicitly confirmed cost and sale price.
func nativeRouteLayerPricingConfirmed(route RouteModelConfig, routing ModelRoutingSnapshot) bool {
	return nativeRouteLayerUnavailableReason(route, routing) == ""
}
func nativeRouteLayerUnavailableReason(route RouteModelConfig, routing ModelRoutingSnapshot) string {
	var reasons []string
	add := func(reason string) {
		for _, existing := range reasons {
			if existing == reason {
				return
			}
		}
		reasons = append(reasons, reason)
	}
	hasCandidate := false
	for _, rc := range routing.Candidates {
		if !rc.Enabled || rc.RouteModelID != route.ID {
			continue
		}
		for _, c := range routing.ProviderModels {
			if c.AccountModelID != rc.AccountModelID || !candidateHealthUsable(c.HealthStatus) {
				continue
			}
			hasCandidate = true
			if !IsNativeImageCandidate(c) {
				add("候选模型缺少原生拆层能力")
				continue
			}
			cfg, err := NativeConfig(c)
			profile := cfg.Profile
			if profile != nil {
				if !containsNativeOperation(profile.Operations, nativeimage.OperationLayerDecomposition) {
					add("候选模型未声明拆层操作")
				}
				if !profile.LayerDecomposition {
					add("候选模型未开启拆层开关")
				}
				if profile.MaxLayers < 1 {
					add("候选模型未配置有效的最大图层数")
				}
				if len(profile.LayerSizeModes) == 0 {
					add("候选模型未配置拆层尺寸模式")
				}
			}
			if err != nil {
				add("候选模型原生能力配置无效")
				continue
			}
			if !nativeLayerPricingValid(cfg.Pricing, profile) {
				cost := cfg.Pricing
				if cost == nil {
					add("候选模型未配置拆层成本")
				} else {
					if cost.Model != profile.Model {
						add("拆层成本型号与候选模型不匹配")
					}
					if cost.Currency != "CNY" {
						add("拆层成本币种须为人民币")
					}
					if cost.Version == "" {
						add("拆层成本未配置价格版本")
					}
					if !cost.LayerPricingConfirmed {
						add("拆层成本尚未确认")
					}
					if cost.LayerBaseCNY == nil {
						add("拆层成本缺少底图价格")
					} else if !nativeRateValid(*cost.LayerBaseCNY) {
						add("拆层成本底图价格无效")
					}
					if !nativeRateValid(cost.InputCNY) || cost.FreeInputImages < 0 {
						add("拆层成本输入图片价格或免费张数无效")
					}
					if !nativePriceTiersValid(cost.OutputTiers, profile.MaxPixels) {
						add("拆层成本缺少完整有效的输出价格档位")
					}
					if !nativePriceTiersValid(cost.LayerTiers, profile.MaxPixels) {
						add("拆层成本缺少完整有效的图层价格档位")
					}
				}
			}
		}
	}
	if !hasCandidate {
		add("没有可用的原生拆层候选模型")
	}
	profile := nativeRouteProfile(route, routing)
	if profile == nil || !nativeProfileSupportsLayers(profile) {
		add("候选模型没有共同可用的拆层能力")
	}
	// Inspect the operation before validation so all missing fields can be reported together.
	var rule *imagepricing.Rule
	if route.ImagePricing != nil {
		for i := range route.ImagePricing.Rules {
			if route.ImagePricing.Rules[i].Operation == "layer_decomposition" {
				rule = &route.ImagePricing.Rules[i]
				break
			}
		}
	}
	if rule == nil {
		add("路由未配置拆层销售价格")
	} else {
		if rule.Mode != "per_image" {
			add("路由拆层销售价格须按张计费")
		}
		if rule.Layers == nil {
			add("路由未配置拆层销售明细")
		} else {
			if !rule.Layers.Confirmed {
				add("路由拆层销售价格尚未确认")
			}
			if rule.Layers.BasePrice == nil {
				add("路由拆层销售价格缺少底图价格")
			}
			if len(rule.Layers.Tiers) == 0 {
				add("路由拆层销售价格缺少图层价格档位")
			}
		}
		if _, err := imagepricing.RuleForOperation(route.ImagePricing, "layer_decomposition"); err != nil {
			add("路由图片销售价格配置无效")
		} else if rule.Mode == "per_image" && rule.Layers != nil && rule.Layers.Confirmed && rule.Layers.BasePrice != nil && len(rule.Layers.Tiers) > 0 && profile != nil && nativeProfileSupportsLayers(profile) {
			for _, mode := range profile.LayerSizeModes {
				req := imagepricing.Request{Operation: "layer_decomposition", BaseResolution: mode, Outputs: []imagepricing.Dimensions{{Width: int64(profile.MaxPixels), Height: 1}}, Layers: []imagepricing.Dimensions{{Width: int64(profile.MaxPixels), Height: 1}}}
				if _, err := imagepricing.Calculate(route.ImagePricing, req); err != nil {
					add("路由拆层销售价格档位未覆盖全部拆层尺寸")
				}
			}
		}
	}
	return strings.Join(reasons, "；")
}

func nativeProfileSupportsLayers(p *nativeimage.CapabilityProfile) bool {
	if p == nil || !p.LayerDecomposition || p.MaxLayers < 1 || len(p.LayerSizeModes) == 0 {
		return false
	}
	for _, op := range p.Operations {
		if op == nativeimage.OperationLayerDecomposition {
			return true
		}
	}
	return false
}

func nativeLayerPricingValid(p *nativeimage.Pricing, profile *nativeimage.CapabilityProfile) bool {
	if p == nil || p.Model != profile.Model || p.Currency != "CNY" || p.Version == "" || !p.LayerPricingConfirmed || p.LayerBaseCNY == nil || !nativeRateValid(*p.LayerBaseCNY) || !nativeRateValid(p.InputCNY) || p.FreeInputImages < 0 {
		return false
	}
	return nativePriceTiersValid(p.OutputTiers, profile.MaxPixels) && nativePriceTiersValid(p.LayerTiers, profile.MaxPixels)
}

func nativePriceTiersValid(tiers []nativeimage.PriceTier, maxPixels int) bool {
	previous := 0
	for _, tier := range tiers {
		if tier.Name == "" || tier.MaxPixels <= previous || !nativeRateValid(tier.CNY) {
			return false
		}
		previous = tier.MaxPixels
	}
	return previous >= maxPixels && maxPixels > 0
}

func nativeRateValid(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func nativePricedResolutions(route RouteModelConfig, p *nativeimage.CapabilityProfile, task string) []string {
	op := "generate"
	if task == "image_edit" {
		op = "edit"
	}
	result := []string{}
	for _, mode := range []string{"1K", "1.5K", "2K", "4K"} {
		var pixels int64
		switch mode {
		case "1K":
			pixels = 1024 * 1024
		case "1.5K":
			pixels = 1536 * 1024
		case "2K":
			pixels = 2048 * 2048
		case "4K":
			pixels = 4096 * 4096
		default:
			continue
		}
		if pixels > int64(p.MaxPixels) {
			continue
		}
		if _, err := imagepricing.Calculate(route.ImagePricing, imagepricing.Request{Operation: op, BaseResolution: mode, Outputs: []imagepricing.Dimensions{{Width: pixels, Height: 1}}}); err == nil {
			result = append(result, strings.ToLower(mode))
		}
	}
	return result
}

// These zero values are capability gates only; billing always calculates the frozen rule.
func unifiedRouteCapabilityPrices(route RouteModelConfig, task string, capability VisibleRouteModelTaskCapability) []RoutePriceConfig {
	op := "generate"
	if task == "image_edit" || task == "reference_to_image" {
		op = "edit"
	}
	var prices []RoutePriceConfig
	for _, resolution := range capability.BaseResolution {
		edge := int64(1024)
		switch strings.ToLower(resolution) {
		case "2k":
			edge = 2048
		case "4k":
			edge = 4096
		}
		if _, err := imagepricing.Calculate(route.ImagePricing, imagepricing.Request{Operation: op, BaseResolution: resolution, Outputs: []imagepricing.Dimensions{{Width: edge, Height: edge}}}); err == nil {
			prices = append(prices, RoutePriceConfig{RouteModelID: route.ID, TaskType: task, BaseResolution: resolution, BasePoints: "0", Enabled: true})
		}
	}
	return prices
}
