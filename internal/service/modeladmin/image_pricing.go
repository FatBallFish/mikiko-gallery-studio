package modeladmin

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/domain/imagepricing"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

// ValidateRouteImagePricing checks configured tiers against every effective candidate.
// An empty candidate set permits drafting; adding an enabled candidate revalidates it.
func ValidateRouteImagePricing(config *imagepricing.Config, candidates []modelhub.ProviderCandidate) error {
	if err := imagepricing.Validate(config); err != nil {
		return err
	}
	if config == nil {
		return nil
	}
	for _, c := range candidates {
		if modelhub.IsNativeImageCandidate(c) {
			cfg, err := modelhub.NativeConfig(c)
			if err != nil {
				return err
			}
			p := cfg.Profile
			for _, r := range config.Rules {
				supported := false
				for _, op := range p.Operations {
					if string(op) == r.Operation {
						supported = true
					}
				}
				if !supported {
					return fmt.Errorf("candidate %s does not support %s", c.ModelCode, r.Operation)
				}
				tiers := r.OutputTiers
				modes := p.SizeModes
				if r.Layers != nil {
					tiers = r.Layers.Tiers
					modes = p.LayerSizeModes
				}
				for _, t := range tiers {
					if t.MaxPixels > int64(p.MaxPixels) || (t.BaseResolution != "" && (strings.EqualFold(t.BaseResolution, "auto") || !containsResolution(modes, t.BaseResolution))) {
						return fmt.Errorf("candidate %s does not support price tier %s", c.ModelCode, t.Name)
					}
				}
			}
		} else {
			for _, r := range config.Rules {
				if r.Operation == "layer_decomposition" {
					return fmt.Errorf("candidate %s has no native layer capability", c.ModelCode)
				}
				task := "text_to_image"
				if r.Operation == "edit" {
					task = "image_edit"
				}
				if !containsResolution(c.SupportedTaskTypes, task) || (r.Operation == "edit" && (!c.SupportsImageInput || c.MaxReferenceImageCount < 1)) {
					return fmt.Errorf("candidate %s does not support %s", c.ModelCode, r.Operation)
				}
				maxPixels := genericCandidateMaxPixels(c)
				for _, t := range r.OutputTiers {
					if t.MaxPixels > 0 && (maxPixels <= 0 || t.MaxPixels > maxPixels) {
						return fmt.Errorf("candidate %s does not support price tier %s", c.ModelCode, t.Name)
					}
					if t.BaseResolution != "" && !containsResolution(c.SupportedBaseResolution, t.BaseResolution) {
						return fmt.Errorf("candidate %s does not support resolution %s", c.ModelCode, t.BaseResolution)
					}
				}
			}
		}
	}
	return nil
}
func containsResolution(values []string, value string) bool {
	for _, v := range values {
		if strings.EqualFold(v, value) {
			return true
		}
	}
	return false
}
func (s *Service) validateRoutePricing(ctx context.Context, routeID int64, config *imagepricing.Config, replacement *modelhub.RouteCandidateConfig) error {
	if config == nil {
		return nil
	}
	snapshot, err := s.store.ModelRoutingConfig(ctx)
	if err != nil {
		return err
	}
	if replacement != nil {
		found := false
		for i, c := range snapshot.Candidates {
			if c.ID == replacement.ID && replacement.ID != 0 {
				snapshot.Candidates[i] = *replacement
				found = true
			}
		}
		if !found {
			snapshot.Candidates = append(snapshot.Candidates, *replacement)
		}
	}
	var candidates []modelhub.ProviderCandidate
	for _, rc := range snapshot.Candidates {
		if rc.RouteModelID != routeID || !rc.Enabled {
			continue
		}
		for _, c := range snapshot.ProviderModels {
			if c.AccountModelID == rc.AccountModelID {
				candidates = append(candidates, c)
			}
		}
	}
	if err := ValidateRouteImagePricing(config, candidates); err != nil {
		return errs.BadRequest(err.Error())
	}
	return nil
}

// Bound pixel tiers by declared output presets, resolution budgets and custom bounds.
func genericCandidateMaxPixels(c modelhub.ProviderCandidate) int64 {
	var limit int64
	for _, r := range c.SupportedBaseResolution {
		var n int64
		switch strings.ToLower(r) {
		case "1k":
			n = 1024 * 1024
		case "2k":
			n = 2048 * 2048
		case "4k":
			n = 4096 * 4096
		}
		if n > limit {
			limit = n
		}
	}
	for _, size := range c.SupportedPixelSizes {
		w, h, ok := modelhub.ParseImageSize(size)
		if ok && w > 0 && h > 0 && int64(w) <= math.MaxInt64/int64(h) {
			n := int64(w) * int64(h)
			if n > limit {
				limit = n
			}
		}
	}
	if c.SupportsCustomSize && c.MaxWidth > 0 && c.MaxHeight > 0 && int64(c.MaxWidth) <= math.MaxInt64/int64(c.MaxHeight) {
		n := int64(c.MaxWidth) * int64(c.MaxHeight)
		if n > limit {
			limit = n
		}
	}
	return limit
}
