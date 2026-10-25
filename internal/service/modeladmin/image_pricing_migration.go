package modeladmin

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"

	"github.com/fatballfish/pic-gallery/internal/domain/imagepricing"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
)

type ImagePricingMigration struct {
	RouteModelID   int64                `json:"route_model_id"`
	ImagePricing   *imagepricing.Config `json:"image_pricing"`
	AlreadyApplied bool                 `json:"already_applied"`
}

// PlanImagePricingMigration changes no state. It refuses incomplete/mixed sales, and
// never reads costs as sale prices or clears legacy route prices/account extra.
func PlanImagePricingMigration(snapshot modelhub.ModelRoutingSnapshot) ([]ImagePricingMigration, error) {
	var result []ImagePricingMigration
	for _, route := range snapshot.RouteModels {
		if !route.Enabled || route.MediaType == "video" {
			continue
		}
		nativeRoute := false
		for _, rc := range snapshot.Candidates {
			if rc.RouteModelID == route.ID && rc.Enabled {
				for _, c := range snapshot.ProviderModels {
					if c.AccountModelID == rc.AccountModelID && modelhub.IsNativeImageCandidate(c) {
						nativeRoute = true
					}
				}
			}
		}
		if !nativeRoute {
			continue
		}
		for _, price := range snapshot.Prices {
			if price.RouteModelID == route.ID && price.Enabled {
				return nil, fmt.Errorf("route %d has legacy sale prices requiring manual conflict review", route.ID)
			}
		}
		var config *imagepricing.Config
		found := false
		for _, rc := range snapshot.Candidates {
			if rc.RouteModelID != route.ID || !rc.Enabled {
				continue
			}
			var candidate *modelhub.ProviderCandidate
			for i := range snapshot.ProviderModels {
				if snapshot.ProviderModels[i].AccountModelID == rc.AccountModelID {
					candidate = &snapshot.ProviderModels[i]
					break
				}
			}
			// Disabled account/models are not effective candidates.
			if candidate == nil {
				continue
			}
			if !modelhub.IsNativeImageCandidate(*candidate) {
				return nil, fmt.Errorf("route %d mixes native and non-native candidates", route.ID)
			}
			if err := validateRawSalePricing(*candidate); err != nil {
				return nil, fmt.Errorf("route %d candidate %d: %w", route.ID, rc.AccountModelID, err)
			}
			cfg, err := modelhub.NativeConfig(*candidate)
			if err != nil {
				return nil, fmt.Errorf("route %d: %w", route.ID, err)
			}
			if cfg.SalePricing == nil || cfg.SalePricing.Model != cfg.Model {
				return nil, fmt.Errorf("route %d candidate %d has no matching sale pricing", route.ID, rc.AccountModelID)
			}
			sale := cfg.SalePricing
			convert := func(v float64) (string, error) {
				if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
					return "", fmt.Errorf("invalid legacy sale rate")
				}
				return strconv.FormatFloat(v, 'f', -1, 64), nil
			}
			input, err := convert(sale.InputCNY)
			if err != nil {
				return nil, err
			}
			tiers := func() ([]imagepricing.Tier, error) {
				var out []imagepricing.Tier
				for _, t := range sale.OutputTiers {
					v, e := convert(t.CNY)
					if e != nil {
						return nil, e
					}
					out = append(out, imagepricing.Tier{Name: t.Name, MaxPixels: int64(t.MaxPixels), Price: v})
				}
				return out, nil
			}
			out, err := tiers()
			if err != nil {
				return nil, err
			}
			current := &imagepricing.Config{SchemaVersion: 1}
			for _, operation := range []string{"generate", "edit"} {
				current.Rules = append(current.Rules, imagepricing.Rule{Operation: operation, Mode: "per_image", Currency: sale.Currency, Version: sale.Version, Input: imagepricing.Input{FreeImages: sale.FreeInputImages, PerImage: &input}, OutputTiers: out})
			}
			if sale.LayerBaseCNY != nil || len(sale.LayerTiers) > 0 || sale.LayerPricingConfirmed {
				layer := &imagepricing.Layers{Confirmed: sale.LayerPricingConfirmed}
				if sale.LayerBaseCNY != nil {
					v, e := convert(*sale.LayerBaseCNY)
					if e != nil {
						return nil, e
					}
					layer.BasePrice = &v
				}
				for _, t := range sale.LayerTiers {
					v, e := convert(t.CNY)
					if e != nil {
						return nil, e
					}
					layer.Tiers = append(layer.Tiers, imagepricing.Tier{Name: t.Name, MaxPixels: int64(t.MaxPixels), Price: v})
				}
				current.Rules = append(current.Rules, imagepricing.Rule{Operation: "layer_decomposition", Mode: "per_image", Currency: sale.Currency, Version: sale.Version, Input: imagepricing.Input{FreeImages: sale.FreeInputImages, PerImage: &input}, Layers: layer})
			}
			if err := ValidateRouteImagePricing(current, []modelhub.ProviderCandidate{*candidate}); err != nil {
				return nil, fmt.Errorf("route %d: %w", route.ID, err)
			}
			if found && !reflect.DeepEqual(config, current) {
				return nil, fmt.Errorf("route %d has conflicting candidate sales", route.ID)
			}
			config = current
			found = true
		}
		if !found {
			continue
		}
		if route.ImagePricing != nil && !reflect.DeepEqual(route.ImagePricing, config) {
			return nil, fmt.Errorf("route %d already has conflicting image_pricing", route.ID)
		}
		result = append(result, ImagePricingMigration{RouteModelID: route.ID, ImagePricing: config, AlreadyApplied: route.ImagePricing != nil})
	}
	return result, nil
}

// EqualImagePricingMigration compares a reviewed plan with a fresh database plan.
func EqualImagePricingMigration(a, b []ImagePricingMigration) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// Validate raw prices before float64 decoding can turn absent or null values into zero.
func validateRawSalePricing(candidate modelhub.ProviderCandidate) error {
	var sale json.RawMessage
	for _, extra := range []map[string]any{candidate.AccountExtra, candidate.ModelExtra} {
		raw, ok := extra["native_image"]
		if !ok {
			continue
		}
		data, err := json.Marshal(raw)
		if err != nil {
			return err
		}
		var node map[string]json.RawMessage
		if err := json.Unmarshal(data, &node); err != nil {
			return err
		}
		if value, ok := node["sale_pricing"]; ok {
			sale = value
		}
	}
	var node map[string]json.RawMessage
	if len(sale) == 0 || string(sale) == "null" {
		return fmt.Errorf("sale_pricing required")
	}
	if err := json.Unmarshal(sale, &node); err != nil {
		return err
	}
	number := func(raw json.RawMessage, label string) error {
		if len(raw) == 0 || string(raw) == "null" || raw[0] == '"' {
			return fmt.Errorf("%s requires an explicit numeric rate", label)
		}
		var v float64
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("%s requires a numeric rate: %w", label, err)
		}
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return fmt.Errorf("invalid %s", label)
		}
		return nil
	}
	if err := number(node["input_cny"], "input_cny"); err != nil {
		return err
	}
	for _, key := range []string{"output_tiers", "layer_tiers"} {
		raw, exists := node[key]
		if !exists && key == "layer_tiers" {
			continue
		}
		if len(raw) == 0 || string(raw) == "null" {
			return fmt.Errorf("%s requires an explicit array", key)
		}
		var tiers []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &tiers); err != nil {
			return err
		}
		for i, tier := range tiers {
			if err := number(tier["cny"], fmt.Sprintf("%s[%d].cny", key, i)); err != nil {
				return err
			}
		}
	}
	if raw, exists := node["layer_base_cny"]; exists && string(raw) != "null" {
		if err := number(raw, "layer_base_cny"); err != nil {
			return err
		}
	}
	// A layer rule cannot silently acquire a free or missing base rate.
	var layers []json.RawMessage
	_ = json.Unmarshal(node["layer_tiers"], &layers)
	var confirmed bool
	_ = json.Unmarshal(node["layer_pricing_confirmed"], &confirmed)
	if len(layers) > 0 || confirmed {
		if err := number(node["layer_base_cny"], "layer_base_cny"); err != nil {
			return err
		}
	}
	return nil
}
