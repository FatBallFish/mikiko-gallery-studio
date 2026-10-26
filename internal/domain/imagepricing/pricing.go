// Package imagepricing defines route sale prices without currency conversion or points rounding.
package imagepricing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"strings"

	"github.com/shopspring/decimal"
)

type Config struct {
	SchemaVersion int    `json:"schema_version"`
	Rules         []Rule `json:"rules"`
}
type Rule struct {
	Operation   string      `json:"operation"`
	Mode        string      `json:"mode"`
	Currency    string      `json:"currency"`
	Version     string      `json:"version"`
	Input       Input       `json:"input"`
	OutputTiers []Tier      `json:"output_tiers,omitempty"`
	PixelToken  *PixelToken `json:"pixel_token,omitempty"`
	Layers      *Layers     `json:"layers,omitempty"`
}
type Input struct {
	FreeImages int     `json:"free_images"`
	PerImage   *string `json:"per_image,omitempty"`
}
type Tier struct {
	Name           string `json:"name"`
	MaxPixels      int64  `json:"max_pixels,omitempty"`
	BaseResolution string `json:"base_resolution,omitempty"`
	Price          string `json:"price"`
}
type PixelToken struct {
	Definition       string `json:"definition"`
	PixelsPerToken   int64  `json:"pixels_per_token"`
	InputPerMillion  string `json:"input_per_million"`
	OutputPerMillion string `json:"output_per_million"`
}
type Layers struct {
	Confirmed bool    `json:"confirmed"`
	BasePrice *string `json:"base_price,omitempty"`
	Tiers     []Tier  `json:"tiers"`
}

// UnmarshalJSON rejects unknown fields only inside this known contract, not account extra.
func (c *Config) UnmarshalJSON(data []byte) error {
	type plain Config
	var v plain
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&v); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("unexpected trailing image pricing data")
	}
	*c = Config(v)
	return nil
}

var decimalPattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`)

func rate(s string) (decimal.Decimal, error) {
	if len(s) > 128 || !decimalPattern.MatchString(s) {
		return decimal.Zero, fmt.Errorf("price must be a finite nonnegative decimal string")
	}
	return decimal.NewFromString(s)
}
func validateTiers(tiers []Tier) error {
	if len(tiers) == 0 {
		return fmt.Errorf("price tiers required")
	}
	var previous int64
	names := map[string]bool{}
	resolutions := map[string]bool{}
	kind := ""
	for _, t := range tiers {
		if strings.TrimSpace(t.Name) == "" || names[t.Name] {
			return fmt.Errorf("tier names must be nonempty and unique")
		}
		names[t.Name] = true
		if _, err := rate(t.Price); err != nil {
			return err
		}
		if t.MaxPixels > 0 && t.BaseResolution == "" {
			if kind == "resolution" || t.MaxPixels <= previous {
				return fmt.Errorf("pixel tiers must have increasing positive max_pixels")
			}
			kind = "pixels"
			previous = t.MaxPixels
		} else if t.MaxPixels == 0 && strings.TrimSpace(t.BaseResolution) != "" {
			r := strings.ToUpper(t.BaseResolution)
			if kind == "pixels" || resolutions[r] {
				return fmt.Errorf("resolution tiers must be unique and cannot mix with pixel tiers")
			}
			kind = "resolution"
			resolutions[r] = true
		} else {
			return fmt.Errorf("tier requires exactly one of positive max_pixels or base_resolution")
		}
	}
	return nil
}

// Validate accepts nil as unconfigured. Calculation always requires a configured rule.
func Validate(c *Config) error {
	if c == nil {
		return nil
	}
	if c.SchemaVersion != 1 || len(c.Rules) == 0 {
		return fmt.Errorf("image_pricing requires schema_version 1 and rules")
	}
	seen := map[string]bool{}
	for _, r := range c.Rules {
		if (r.Operation != "generate" && r.Operation != "edit" && r.Operation != "layer_decomposition") || seen[r.Operation] {
			return fmt.Errorf("invalid or duplicate pricing operation %q", r.Operation)
		}
		seen[r.Operation] = true
		if r.Currency != "CNY" && r.Currency != "points" {
			return fmt.Errorf("invalid pricing currency")
		}
		if strings.TrimSpace(r.Version) == "" {
			return fmt.Errorf("pricing version required")
		}
		if r.Input.FreeImages < 0 {
			return fmt.Errorf("free_images must be nonnegative")
		}
		if r.Mode == "per_image" {
			if r.Input.PerImage == nil {
				return fmt.Errorf("input.per_image required")
			}
			if _, err := rate(*r.Input.PerImage); err != nil {
				return err
			}
			if r.PixelToken != nil {
				return fmt.Errorf("per_image cannot include pixel_token")
			}
			if r.Operation != "layer_decomposition" {
				if err := validateTiers(r.OutputTiers); err != nil {
					return err
				}
				if r.Layers != nil {
					return fmt.Errorf("layers require layer_decomposition")
				}
			}
			if r.Operation == "layer_decomposition" {
				if len(r.OutputTiers) > 0 {
					return fmt.Errorf("layer operation uses layers.tiers")
				}
				if r.Layers == nil {
					return fmt.Errorf("layers required")
				}
				if r.Layers.BasePrice != nil {
					if _, err := rate(*r.Layers.BasePrice); err != nil {
						return err
					}
				}
				if len(r.Layers.Tiers) > 0 {
					if err := validateTiers(r.Layers.Tiers); err != nil {
						return err
					}
				}
				if r.Layers.Confirmed && (r.Layers.BasePrice == nil || len(r.Layers.Tiers) == 0) {
					return fmt.Errorf("confirmed layers require base_price and tiers")
				}
			}
		} else if r.Mode == "pixel_token" {
			p := r.PixelToken
			if r.Operation == "layer_decomposition" || r.Layers != nil || len(r.OutputTiers) > 0 || r.Input.PerImage != nil {
				return fmt.Errorf("pixel_token cannot mix per-image or layer pricing")
			}
			if p == nil || p.Definition != "platform_pixels_v1" || p.PixelsPerToken <= 0 {
				return fmt.Errorf("platform_pixels_v1 requires positive pixels_per_token")
			}
			if _, err := rate(p.InputPerMillion); err != nil {
				return err
			}
			if _, err := rate(p.OutputPerMillion); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("invalid pricing mode")
		}
	}
	return nil
}
func RuleForOperation(c *Config, operation string) (Rule, error) {
	if err := Validate(c); err != nil {
		return Rule{}, err
	}
	if c != nil {
		for _, r := range c.Rules {
			if r.Operation == operation {
				return r, nil
			}
		}
	}
	return Rule{}, fmt.Errorf("route image_pricing missing for %s", operation)
}

type Dimensions struct {
	Width  int64 `json:"width"`
	Height int64 `json:"height"`
}
type Request struct {
	Operation      string
	Inputs         []Dimensions
	Outputs        []Dimensions
	BaseResolution string
	Layers         []Dimensions
}
type Quote struct {
	Currency     string
	Amount       decimal.Decimal
	InputTokens  int64
	OutputTokens int64
}

func pixels(d Dimensions) (int64, error) {
	if d.Width <= 0 || d.Height <= 0 || d.Width > math.MaxInt64/d.Height {
		return 0, fmt.Errorf("valid image dimensions required")
	}
	return d.Width * d.Height, nil
}
func tierPrice(tiers []Tier, d Dimensions, resolution string) (decimal.Decimal, error) {
	for _, t := range tiers {
		if t.BaseResolution != "" {
			if strings.EqualFold(t.BaseResolution, resolution) {
				return rate(t.Price)
			}
		} else {
			p, err := pixels(d)
			if err != nil {
				return decimal.Zero, err
			}
			if p <= t.MaxPixels {
				return rate(t.Price)
			}
		}
	}
	return decimal.Zero, fmt.Errorf("no matching route price tier")
}
func tokenSum(images []Dimensions, unit int64) (int64, error) {
	var sum int64
	for _, d := range images {
		p, err := pixels(d)
		if err != nil {
			return 0, err
		}
		n := p / unit
		if p%unit != 0 {
			n++
		}
		if sum > math.MaxInt64-n {
			return 0, fmt.Errorf("image token overflow")
		}
		sum += n
	}
	return sum, nil
}

// Calculate returns an exact amount in the rule currency. Billing owns FX and points precision.
func Calculate(c *Config, req Request) (Quote, error) {
	r, err := RuleForOperation(c, req.Operation)
	if err != nil {
		return Quote{}, err
	}
	q := Quote{Currency: r.Currency, Amount: decimal.Zero}
	if len(req.Outputs) == 0 {
		return q, fmt.Errorf("output images required")
	}
	free := r.Input.FreeImages
	if free > len(req.Inputs) {
		free = len(req.Inputs)
	}
	if r.Mode == "pixel_token" {
		if len(req.Layers) > 0 {
			return q, fmt.Errorf("pixel_token layers unsupported")
		}
		// Even free references need trusted dimensions for a reproducible token quote.
		if _, err = tokenSum(req.Inputs, r.PixelToken.PixelsPerToken); err != nil {
			return q, err
		}
		q.InputTokens, err = tokenSum(req.Inputs[free:], r.PixelToken.PixelsPerToken)
		if err != nil {
			return q, err
		}
		q.OutputTokens, err = tokenSum(req.Outputs, r.PixelToken.PixelsPerToken)
		if err != nil {
			return q, err
		}
		in, _ := rate(r.PixelToken.InputPerMillion)
		out, _ := rate(r.PixelToken.OutputPerMillion)
		q.Amount = in.Mul(decimal.NewFromInt(q.InputTokens)).Add(out.Mul(decimal.NewFromInt(q.OutputTokens))).Shift(-6)
		return q, nil
	}
	in, _ := rate(*r.Input.PerImage)
	q.Amount = in.Mul(decimal.NewFromInt(int64(len(req.Inputs) - free)))
	if req.Operation == "layer_decomposition" {
		if len(req.Outputs) != 1 || !r.Layers.Confirmed || r.Layers.BasePrice == nil || len(r.Layers.Tiers) == 0 {
			return q, fmt.Errorf("confirmed base and layer pricing required")
		}
		base, _ := rate(*r.Layers.BasePrice)
		q.Amount = q.Amount.Add(base)
		for _, d := range req.Layers {
			v, e := tierPrice(r.Layers.Tiers, d, req.BaseResolution)
			if e != nil {
				return q, e
			}
			q.Amount = q.Amount.Add(v)
		}
		return q, nil
	}
	if len(req.Layers) > 0 {
		return q, fmt.Errorf("layers require layer_decomposition")
	}
	for _, d := range req.Outputs {
		v, e := tierPrice(r.OutputTiers, d, req.BaseResolution)
		if e != nil {
			return q, e
		}
		q.Amount = q.Amount.Add(v)
	}
	return q, nil
}
