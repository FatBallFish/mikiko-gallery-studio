// Package nativeimage defines portable, serializable native image capabilities.
package nativeimage

import (
	"fmt"
	"strings"
)

type Operation string

const (
	OperationGenerate           Operation = "generate"
	OperationEdit               Operation = "edit"
	OperationLayerDecomposition Operation = "layer_decomposition"
)
const AdapterQwen = "qwen_image"
const AdapterSeedream = "seedream_image"

type QwenOptions struct {
	PromptExtend     *bool  `json:"prompt_extend,omitempty"`
	PromptExtendMode string `json:"prompt_extend_mode,omitempty"`
	EnableThinking   *bool  `json:"enable_thinking,omitempty"`
}
type SeedreamOptions struct {
	OptimizePromptMode string `json:"optimize_prompt_mode,omitempty"`
}
type Options struct {
	Seed           *int64           `json:"seed,omitempty"`
	NegativePrompt string           `json:"negative_prompt,omitempty"`
	Watermark      *bool            `json:"watermark,omitempty"`
	Qwen           *QwenOptions     `json:"qwen,omitempty"`
	Seedream       *SeedreamOptions `json:"seedream,omitempty"`
	Annotations    []Annotation     `json:"annotations,omitempty"`
}

// Annotation coordinates use the editing prompt's inclusive 0..999 scale.
type Annotation struct {
	ReferenceIndex int     `json:"reference_index"`
	Label          string  `json:"label,omitempty"`
	Point          *[2]int `json:"point,omitempty"`
	BBox           *[4]int `json:"bbox,omitempty"`
}
type BoundingBox struct {
	Absolute   [4]int `json:"absolute"`
	Normalized [4]int `json:"normalized"`
}
type LayerMetadata struct {
	ZIndex      int          `json:"z_index"`
	Name        string       `json:"name,omitempty"`
	Description string       `json:"description,omitempty"`
	BoundingBox *BoundingBox `json:"bounding_box,omitempty"`
	IsBase      bool         `json:"is_base"`
}
type CapabilityProfile struct {
	Version             string      `json:"version"`
	Model               string      `json:"model"`
	Adapter             string      `json:"adapter"`
	Operations          []Operation `json:"operations"`
	MinPixels           int         `json:"min_pixels"`
	MaxPixels           int         `json:"max_pixels"`
	MaxAspectRatio      float64     `json:"max_aspect_ratio"`
	MaxReferenceImages  int         `json:"max_reference_images"`
	MaxOutputImages     int         `json:"max_output_images"`
	OutputFormats       []string    `json:"output_formats"`
	InputFormats        []string    `json:"input_formats"`
	LayerSizeModes      []string    `json:"layer_size_modes,omitempty"`
	MaxLayers           int         `json:"max_layers,omitempty"`
	SizeModes           []string    `json:"size_modes"`
	NegativePrompt      bool        `json:"negative_prompt"`
	Seed                bool        `json:"seed"`
	LayerDecomposition  bool        `json:"layer_decomposition"`
	OptimizePromptModes []string    `json:"optimize_prompt_modes,omitempty"`
}

func ProfileForModel(model string) (CapabilityProfile, bool) {
	p := CapabilityProfile{Version: "2026-10-25", Model: model, Operations: []Operation{OperationGenerate, OperationEdit}}
	switch model {
	case "qwen-image-3.0", "qwen-image-3.0-pro", "qwen-image-2.1-pro", "qwen-image-2.1-turbo":
		p.Adapter = AdapterQwen
		p.MinPixels = 262144
		p.MaxPixels = 4194304
		p.MaxAspectRatio = 8
		p.MaxReferenceImages = 10
		p.MaxOutputImages = 6
		p.InputFormats = []string{"jpeg", "png", "bmp", "tiff", "webp", "gif"}
		p.OutputFormats = []string{"png"}
		p.SizeModes = []string{"auto", "dimensions"}
		p.Seed = true
		if strings.HasPrefix(model, "qwen-image-3.0") {
			p.MaxReferenceImages = 3
			p.NegativePrompt = true
		}
	case "doubao-seedream-5-0-pro-260628", "doubao-seedream-5-0-flash-260915":
		p.Adapter = AdapterSeedream
		p.MinPixels = 921600
		p.MaxPixels = 4624220
		p.MaxAspectRatio = 16
		p.MaxReferenceImages = 10
		p.MaxOutputImages = 1
		p.InputFormats = []string{"jpeg", "png", "webp", "bmp", "tiff", "gif", "heic", "heif"}
		p.LayerSizeModes = []string{"auto", "1K", "1.5K", "2K"}
		p.MaxLayers = 16
		p.OutputFormats = []string{"jpeg", "png"}
		p.SizeModes = []string{"1K", "1.5K", "2K", "dimensions"}
		p.LayerDecomposition = true
		p.Operations = append(p.Operations, OperationLayerDecomposition)
		p.OptimizePromptModes = []string{"standard"}
		if strings.Contains(model, "-pro-") {
			p.OptimizePromptModes = append(p.OptimizePromptModes, "fast")
		}
	default:
		return CapabilityProfile{}, false
	}
	return p, true
}

type ValidationRequest struct {
	Operation      Operation
	Prompt         string
	Size           string
	OutputFormat   string
	Background     string
	Count          int
	ReferenceCount int
	Options        *Options
}

func ValidateRequest(p CapabilityProfile, r ValidationRequest) error {
	if r.Operation == "" {
		r.Operation = OperationGenerate
	}
	supported := false
	for _, op := range p.Operations {
		if op == r.Operation {
			supported = true
		}
	}
	if !supported {
		return fmt.Errorf("unsupported operation %q", r.Operation)
	}
	if r.Operation != OperationLayerDecomposition && strings.TrimSpace(r.Prompt) == "" {
		return fmt.Errorf("prompt is required")
	}
	if r.ReferenceCount > p.MaxReferenceImages {
		return fmt.Errorf("too many reference images: maximum %d", p.MaxReferenceImages)
	}
	if r.Operation == OperationGenerate && r.ReferenceCount != 0 {
		return fmt.Errorf("generate does not accept reference images; use edit")
	}
	if r.Operation == OperationEdit && r.ReferenceCount == 0 {
		return fmt.Errorf("edit requires reference images")
	}
	if r.Operation == OperationLayerDecomposition && r.ReferenceCount != 1 {
		return fmt.Errorf("layer decomposition requires exactly one reference")
	}
	if r.Count < 0 || r.Count > p.MaxOutputImages {
		return fmt.Errorf("output count must be 1..%d", p.MaxOutputImages)
	}
	if r.OutputFormat != "" && !contains(p.OutputFormats, r.OutputFormat) {
		return fmt.Errorf("unsupported output format %q", r.OutputFormat)
	}
	if r.Background != "" && r.Background != "opaque" && r.Background != "transparent" {
		return fmt.Errorf("unsupported background")
	}
	if r.Background != "" && p.Adapter == AdapterQwen {
		return fmt.Errorf("Qwen background must be specified through prompt")
	}
	if r.Background == "transparent" && (r.ReferenceCount != 1 || r.OutputFormat == "jpeg") {
		return fmt.Errorf("transparent output requires one transparent reference and PNG")
	}
	if err := ValidateSize(p, r.Size, r.Operation); err != nil {
		return err
	}
	if o := r.Options; o != nil {
		if o.Seed != nil && (!p.Seed || *o.Seed < 0 || *o.Seed > 2147483647) {
			return fmt.Errorf("unsupported or invalid seed")
		}
		if o.NegativePrompt != "" && !p.NegativePrompt {
			return fmt.Errorf("negative prompt is unsupported")
		}
		if o.Qwen != nil {
			if p.Adapter != AdapterQwen {
				return fmt.Errorf("Qwen options are unsupported")
			}
			q := o.Qwen
			if q.PromptExtendMode != "" && q.PromptExtendMode != "direct" && q.PromptExtendMode != "agent" {
				return fmt.Errorf("invalid prompt extend mode")
			}
			if q.PromptExtendMode == "agent" && r.Operation != OperationGenerate {
				return fmt.Errorf("agent mode supports generation only")
			}
			if q.EnableThinking != nil && *q.EnableThinking && q.PromptExtend != nil && !*q.PromptExtend {
				return fmt.Errorf("thinking requires prompt extension")
			}
		}
		if o.Seedream != nil && (p.Adapter != AdapterSeedream || (o.Seedream.OptimizePromptMode != "" && !contains(p.OptimizePromptModes, o.Seedream.OptimizePromptMode))) {
			return fmt.Errorf("unsupported prompt optimization mode")
		}
		if len(o.Annotations) > 0 && (p.Adapter != AdapterSeedream || r.Operation == OperationGenerate) {
			return fmt.Errorf("coordinate annotations require Seedream editing")
		}
		for _, a := range o.Annotations {
			if a.ReferenceIndex < 1 || a.ReferenceIndex > r.ReferenceCount {
				return fmt.Errorf("annotation reference index outside input images")
			}
			if err := a.Validate(); err != nil {
				return err
			}
		}
	}
	return nil
}
func contains(v []string, s string) bool {
	for _, x := range v {
		if x == s {
			return true
		}
	}
	return false
}
func ParseDimensions(size string) (int, int, error) {
	var w, h int
	normalized := strings.ReplaceAll(size, "*", "x")
	if _, err := fmt.Sscanf(normalized, "%dx%d", &w, &h); err != nil || w <= 0 || h <= 0 || fmt.Sprintf("%dx%d", w, h) != normalized {
		return 0, 0, fmt.Errorf("invalid dimensions %q", size)
	}
	return w, h, nil
}
func ValidateSize(p CapabilityProfile, size string, op Operation) error {
	if size == "" {
		return nil
	}
	if size == "auto" && (p.Adapter == AdapterQwen || op == OperationLayerDecomposition) {
		return nil
	}
	if contains(p.SizeModes, size) && size != "dimensions" {
		return nil
	}
	if op == OperationLayerDecomposition {
		return fmt.Errorf("decomposition accepts auto or size tiers only")
	}
	w, h, err := ParseDimensions(size)
	if err != nil {
		return err
	}
	if w > p.MaxPixels || h > p.MaxPixels || int64(w)*int64(h) < int64(p.MinPixels) || int64(w)*int64(h) > int64(p.MaxPixels) || float64(w)/float64(h) > p.MaxAspectRatio || float64(h)/float64(w) > p.MaxAspectRatio {
		return fmt.Errorf("size outside model pixel/aspect constraints")
	}
	return nil
}
func (a Annotation) Validate() error {
	if (a.Point == nil) == (a.BBox == nil) {
		return fmt.Errorf("annotation requires exactly one point or bbox")
	}
	if a.Point != nil {
		for _, n := range a.Point {
			if n < 0 || n > 999 {
				return fmt.Errorf("point outside 0..999")
			}
		}
	}
	if a.BBox != nil {
		b := *a.BBox
		for _, n := range b {
			if n < 0 || n > 999 {
				return fmt.Errorf("bbox outside 0..999")
			}
		}
		if b[0] >= b[2] || b[1] >= b[3] {
			return fmt.Errorf("invalid bbox ordering")
		}
	}
	return nil
}
func (a Annotation) PromptTag() string {
	if a.Point != nil {
		return fmt.Sprintf("<point>%d %d</point>", a.Point[0], a.Point[1])
	}
	if a.BBox != nil {
		return fmt.Sprintf("<bbox>%d %d %d %d</bbox>", a.BBox[0], a.BBox[1], a.BBox[2], a.BBox[3])
	}
	return ""
}

// NormalizePoint maps canvas pixels to prompt coordinates, not response bbox coordinates.
func NormalizePoint(x, y, width, height int) ([2]int, error) {
	if width <= 0 || height <= 0 || x < 0 || y < 0 || x > width || y > height {
		return [2]int{}, fmt.Errorf("point outside canvas")
	}
	return [2]int{int(float64(x)*999/float64(width) + 0.5), int(float64(y)*999/float64(height) + 0.5)}, nil
}
