package nativeimage

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Defaults contains only official provider fields configurable by administrators.
// Pointer fields distinguish inheritance from an explicit false or zero.
type Defaults struct {
	Seed      *int64           `json:"seed,omitempty"`
	Watermark *bool            `json:"watermark,omitempty"`
	Qwen      *QwenOptions     `json:"qwen,omitempty"`
	Seedream  *SeedreamOptions `json:"seedream,omitempty"`
}

func (d *Defaults) UnmarshalJSON(data []byte) error {
	type plain Defaults
	// Decode over existing values to retain account -> model field inheritance.
	value := plain(*d)
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("invalid native image defaults: %w", err)
	}
	*d = Defaults(value)
	return nil
}

func (d *Defaults) Validate(p CapabilityProfile) error {
	if d == nil {
		return nil
	}
	_, err := EffectiveOptions(p, d, nil)
	return err
}

// EffectiveOptions freezes all platform defaults before validation and submission.
// Explicit API/MCP fields override configured defaults, including false and zero.
func EffectiveOptions(p CapabilityProfile, defaults *Defaults, explicit *Options) (*Options, error) {
	disabled := false
	out := &Options{Watermark: &disabled}
	if p.Adapter == AdapterQwen {
		extend, thinking := false, false
		out.Qwen = &QwenOptions{PromptExtend: &extend, EnableThinking: &thinking}
	} else if p.Adapter == AdapterSeedream {
		out.Seedream = &SeedreamOptions{OptimizePromptMode: "standard"}
	}
	if defaults != nil {
		if defaults.Seed != nil && (!p.Seed || *defaults.Seed < 0 || *defaults.Seed > 2147483647) {
			return nil, fmt.Errorf("unsupported or invalid default seed")
		}
		if defaults.Qwen != nil && p.Adapter != AdapterQwen || defaults.Seedream != nil && p.Adapter != AdapterSeedream {
			return nil, fmt.Errorf("default options do not match model adapter")
		}
		mergeOptions(out, &Options{Seed: defaults.Seed, Watermark: defaults.Watermark, Qwen: defaults.Qwen, Seedream: defaults.Seedream})
	}
	mergeOptions(out, explicit)
	if q := out.Qwen; q != nil {
		if q.PromptExtendMode != "" && q.PromptExtendMode != "direct" && q.PromptExtendMode != "agent" {
			return nil, fmt.Errorf("invalid prompt extend mode")
		}
		if q.EnableThinking != nil && *q.EnableThinking && (q.PromptExtend == nil || !*q.PromptExtend) {
			return nil, fmt.Errorf("thinking requires prompt extension")
		}
		if q.PromptExtend != nil && *q.PromptExtend && q.PromptExtendMode == "" {
			q.PromptExtendMode = "direct"
		}
	}
	if s := out.Seedream; s != nil && (p.Adapter != AdapterSeedream || !contains(p.OptimizePromptModes, s.OptimizePromptMode)) {
		return nil, fmt.Errorf("unsupported prompt optimization mode")
	}
	return out, nil
}

func mergeOptions(out, in *Options) {
	if in == nil {
		return
	}
	if in.Seed != nil {
		v := *in.Seed
		out.Seed = &v
	}
	if in.Watermark != nil {
		v := *in.Watermark
		out.Watermark = &v
	}
	if in.NegativePrompt != "" {
		out.NegativePrompt = in.NegativePrompt
	}
	if in.Annotations != nil {
		out.Annotations = append([]Annotation(nil), in.Annotations...)
	}
	if in.Qwen != nil {
		if out.Qwen == nil {
			out.Qwen = &QwenOptions{}
		}
		if in.Qwen.PromptExtend != nil {
			v := *in.Qwen.PromptExtend
			out.Qwen.PromptExtend = &v
		}
		if in.Qwen.EnableThinking != nil {
			v := *in.Qwen.EnableThinking
			out.Qwen.EnableThinking = &v
		}
		if in.Qwen.PromptExtendMode != "" {
			out.Qwen.PromptExtendMode = in.Qwen.PromptExtendMode
		}
	}
	if in.Seedream != nil {
		if out.Seedream == nil {
			out.Seedream = &SeedreamOptions{}
		}
		if in.Seedream.OptimizePromptMode != "" {
			out.Seedream.OptimizePromptMode = in.Seedream.OptimizePromptMode
		}
	}
}
