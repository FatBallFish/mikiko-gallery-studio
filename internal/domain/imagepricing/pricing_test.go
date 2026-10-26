package imagepricing

import (
	"encoding/json"
	"math"
	"testing"
)

func str(s string) *string { return &s }
func perImage() *Config {
	return &Config{SchemaVersion: 1, Rules: []Rule{{Operation: "generate", Mode: "per_image", Currency: "CNY", Version: "v1", Input: Input{FreeImages: 1, PerImage: str("0.02")}, OutputTiers: []Tier{{Name: "1K", MaxPixels: 100, Price: "0.25"}, {Name: "2K", MaxPixels: 200, Price: "0.5"}}}}}
}
func TestCalculatePerImageAndPixels(t *testing.T) {
	c := perImage()
	q, err := Calculate(c, Request{Operation: "generate", Inputs: make([]Dimensions, 3), Outputs: []Dimensions{{10, 10}, {10, 20}}})
	if err != nil || q.Amount.String() != "0.79" {
		t.Fatalf("%+v %v", q, err)
	}
	if _, err := Calculate(c, Request{Operation: "generate", Outputs: []Dimensions{{11, 20}}}); err == nil {
		t.Fatal("unpriced output accepted")
	}
	if _, err := Calculate(nil, Request{Operation: "generate"}); err == nil {
		t.Fatal("missing config accepted")
	}
	c.Rules[0].OutputTiers = []Tier{{Name: "legacy", BaseResolution: "1K", Price: "2.000001"}}
	q, err = Calculate(c, Request{Operation: "generate", BaseResolution: "1k", Outputs: []Dimensions{{}}})
	if err != nil || q.Amount.String() != "2.000001" {
		t.Fatalf("legacy resolution or precision: %+v %v", q, err)
	}
}
func TestPlatformTokensRoundEachImageAndRequireReferenceDimensions(t *testing.T) {
	c := &Config{SchemaVersion: 1, Rules: []Rule{{Operation: "edit", Mode: "pixel_token", Currency: "points", Version: "v", Input: Input{FreeImages: 1}, PixelToken: &PixelToken{Definition: "platform_pixels_v1", PixelsPerToken: 10, InputPerMillion: "2", OutputPerMillion: "3"}}}}
	req := Request{Operation: "edit", Inputs: []Dimensions{{1, 1}, {1, 11}, {1, 1}}, Outputs: []Dimensions{{1, 11}, {1, 1}}}
	q, err := Calculate(c, req)
	if err != nil || q.InputTokens != 3 || q.OutputTokens != 3 || q.Amount.String() != "0.000015" {
		t.Fatalf("%+v %v", q, err)
	}
	req.Inputs[0] = Dimensions{}
	if _, err := Calculate(c, req); err == nil {
		t.Fatal("free input missing dimensions accepted")
	}
	req.Inputs = nil
	req.Outputs = []Dimensions{{math.MaxInt64, 2}}
	if _, err := Calculate(c, req); err == nil {
		t.Fatal("pixel overflow accepted")
	}
}
func TestStrictValidation(t *testing.T) {
	cases := []func(*Config){
		func(c *Config) { c.Rules[0].Input.PerImage = nil }, func(c *Config) { c.Rules[0].Input.PerImage = str("NaN") }, func(c *Config) { c.Rules[0].Input.PerImage = str("1e10") }, func(c *Config) { c.Rules[0].Input.PerImage = str("-1") }, func(c *Config) { c.Rules[0].Input.FreeImages = -1 }, func(c *Config) { c.Rules = append(c.Rules, c.Rules[0]) }, func(c *Config) { c.Rules[0].OutputTiers[1].MaxPixels = 100 }, func(c *Config) { c.Rules[0].OutputTiers[0].BaseResolution = "1K" },
	}
	for i, mutate := range cases {
		c := perImage()
		mutate(c)
		if Validate(c) == nil {
			t.Errorf("case %d accepted", i)
		}
	}
	c := perImage()
	c.Rules[0].Input.PerImage = str("0")
	if err := Validate(c); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{`{"schema_version":1,"rules":[],"unknown":true}`, `{"schema_version":1,"rules":[{"operation":"edit","input":{"surprise":1}}]}`, `{"schema_version":1,"rules":[{"task_overrides":{}}]}`} {
		var c Config
		if json.Unmarshal([]byte(data), &c) == nil {
			t.Fatal("unknown contract field accepted")
		}
	}
}
func TestLayerPriceCompleteness(t *testing.T) {
	c := perImage()
	r := &c.Rules[0]
	r.Operation = "layer_decomposition"
	r.OutputTiers = nil
	r.Layers = &Layers{Confirmed: false}
	req := Request{Operation: r.Operation, Outputs: []Dimensions{{10, 10}}, Layers: []Dimensions{{5, 10}, {10, 10}}}
	if err := Validate(c); err != nil {
		t.Fatal("unconfirmed draft rejected", err)
	}
	if _, err := Calculate(c, req); err == nil {
		t.Fatal("unconfirmed quote accepted")
	}
	r.Layers = &Layers{Confirmed: true, BasePrice: str("0.3"), Tiers: []Tier{{Name: "layer", MaxPixels: 100, Price: "0.15"}}}
	q, err := Calculate(c, req)
	if err != nil || q.Amount.String() != "0.6" {
		t.Fatalf("%+v %v", q, err)
	}
	r.Mode = "pixel_token"
	if Validate(c) == nil {
		t.Fatal("token layers accepted")
	}
}
