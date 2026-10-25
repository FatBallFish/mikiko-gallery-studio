package nativeimage

import "fmt"

// ValidateProfile accepts only known model profiles whose limits do not expand
// the published model contract. Endpoint IDs retain the real model in Model.
func ValidateProfile(p CapabilityProfile) error {
	base, ok := ProfileForModel(p.Model)
	if !ok {
		return fmt.Errorf("unknown native image profile model")
	}
	if p.Adapter != base.Adapter || p.MinPixels < base.MinPixels || p.MaxPixels > base.MaxPixels || p.MinPixels > p.MaxPixels || p.MaxAspectRatio <= 0 || p.MaxAspectRatio > base.MaxAspectRatio || p.MaxReferenceImages < 0 || p.MaxReferenceImages > base.MaxReferenceImages || p.MaxOutputImages < 1 || p.MaxOutputImages > base.MaxOutputImages || p.MaxLayers < 0 || p.MaxLayers > base.MaxLayers {
		return fmt.Errorf("profile expands official model limits")
	}
	if (p.NegativePrompt && !base.NegativePrompt) || (p.Seed && !base.Seed) || (p.LayerDecomposition && !base.LayerDecomposition) {
		return fmt.Errorf("profile expands official options")
	}
	for _, pair := range [][2][]string{{p.InputFormats, base.InputFormats}, {p.OutputFormats, base.OutputFormats}, {p.SizeModes, base.SizeModes}, {p.LayerSizeModes, base.LayerSizeModes}, {p.OptimizePromptModes, base.OptimizePromptModes}} {
		for _, v := range pair[0] {
			if !contains(pair[1], v) {
				return fmt.Errorf("profile expands official formats or modes")
			}
		}
	}
	if len(p.Operations) == 0 || len(p.OutputFormats) == 0 || len(p.InputFormats) == 0 {
		return fmt.Errorf("profile requires operations and formats")
	}
	for _, op := range p.Operations {
		found := false
		for _, allowed := range base.Operations {
			if op == allowed {
				found = true
			}
		}
		if !found || (op == OperationLayerDecomposition && !p.LayerDecomposition) {
			return fmt.Errorf("profile expands official operations")
		}
	}
	return nil
}
