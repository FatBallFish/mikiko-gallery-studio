package nativeimage

import "fmt"

// All rates are CNY per image; nil means unknown, never a zero-cost rate.
type PriceTier struct {
	Name      string  `json:"name"`
	MaxPixels int     `json:"max_pixels,omitempty"`
	CNY       float64 `json:"cny"`
}
type Pricing struct {
	Currency              string      `json:"currency"`
	Model                 string      `json:"model"`
	Version               string      `json:"version"`
	InputCNY              float64     `json:"input_cny"`
	FreeInputImages       int         `json:"free_input_images"`
	OutputTiers           []PriceTier `json:"output_tiers"`
	LayerTiers            []PriceTier `json:"layer_tiers,omitempty"`
	LayerBaseCNY          *float64    `json:"layer_base_cny,omitempty"`
	LayerPricingConfirmed bool        `json:"layer_pricing_confirmed"`
}

func PricingForModel(model string) (Pricing, bool) {
	p := Pricing{Currency: "CNY", Model: model, Version: "2026-10-25"}
	switch model {
	case "qwen-image-3.0-pro":
		p.InputCNY = .02
		p.OutputTiers = []PriceTier{{"1K", 2250000, .25}, {"2K", 4194304, .50}}
	case "qwen-image-3.0":
		p.InputCNY = .02
		p.OutputTiers = []PriceTier{{"all", 4194304, .18}}
	case "qwen-image-2.1-pro":
		p.OutputTiers = []PriceTier{{"all", 4194304, .25}}
	case "qwen-image-2.1-turbo":
		p.OutputTiers = []PriceTier{{"all", 4194304, .10}}
	case "doubao-seedream-5-0-pro-260628":
		p.InputCNY = .02
		p.FreeInputImages = 1
		p.OutputTiers = []PriceTier{{"1K", 2610000, .30}, {"2K", 4624220, .60}}
		p.LayerTiers = []PriceTier{{"1K", 2610000, .15}, {"2K", 4624220, .30}}
	case "doubao-seedream-5-0-flash-260915":
		p.OutputTiers = []PriceTier{{"all", 4624220, .12}}
	default:
		return Pricing{}, false
	}
	return p, true
}
func TierCNY(tiers []PriceTier, pixels int) (float64, error) {
	if pixels <= 0 {
		return 0, fmt.Errorf("output pixels required")
	}
	for _, t := range tiers {
		if pixels <= t.MaxPixels {
			return t.CNY, nil
		}
	}
	return 0, fmt.Errorf("no confirmed price tier for %d pixels", pixels)
}
func (p Pricing) EstimateCNY(op Operation, inputImages, outputImages, pixels int) (float64, error) {
	if p.Currency != "CNY" || inputImages < 0 || outputImages < 1 {
		return 0, fmt.Errorf("invalid CNY pricing inputs")
	}
	if op == OperationLayerDecomposition {
		if !p.LayerPricingConfirmed || p.LayerBaseCNY == nil {
			return 0, fmt.Errorf("layer pricing is unconfirmed")
		}
		rate, err := TierCNY(p.LayerTiers, pixels)
		if err != nil {
			return 0, err
		}
		return p.inputCost(inputImages) + *p.LayerBaseCNY + float64(outputImages-1)*rate, nil
	}
	rate, err := TierCNY(p.OutputTiers, pixels)
	if err != nil {
		return 0, err
	}
	return p.inputCost(inputImages) + float64(outputImages)*rate, nil
}
func (p Pricing) inputCost(n int) float64 {
	n -= p.FreeInputImages
	if n < 0 {
		n = 0
	}
	return float64(n) * p.InputCNY
}
