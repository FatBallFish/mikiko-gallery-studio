package modelhub

import (
	"errors"
	"testing"

	"github.com/fatballfish/pic-gallery/pkg/errs"
)

func generationTestCapability() ImageModelCapability {
	return ImageModelCapability{
		SizeModes: []string{SizeModeAuto, SizeModeRatio, SizeModePixel}, BaseResolution: []string{"1k", "2k"},
		SupportedRatios: []string{"1:1", "16:9"}, SupportsCustomRatio: true,
		SupportedPixelSizes: []string{"1024x1024"}, SupportsCustomSize: true,
		MinWidth: 512, MaxWidth: 2560, MinHeight: 512, MaxHeight: 2560,
		OutputFormat: []string{"png", "jpeg", "webp"}, SupportedBackgrounds: []string{"auto", "opaque", "transparent"},
		Quality: []string{"auto"}, Moderation: []string{"auto"}, MaxImageCount: 4,
	}
}

func TestNormalizeGenerationRequestModes(t *testing.T) {
	tests := []struct {
		name string
		in   GenerationRequest
		size string
		w, h int
	}{
		{name: "auto omits size", in: GenerationRequest{SizeMode: "auto", OutputFormat: "png"}},
		{name: "ratio preset", in: GenerationRequest{SizeMode: "ratio", BaseResolution: "1K", AspectRatio: "16:9", OutputFormat: "webp"}, size: "1280x720", w: 1280, h: 720},
		{name: "custom ratio", in: GenerationRequest{SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "7:5", OutputFormat: "png"}, size: "1488x1056", w: 1488, h: 1056},
		{name: "pixel preset", in: GenerationRequest{SizeMode: "pixel", RequestedSize: "1024x1024", OutputFormat: "png"}, size: "1024x1024", w: 1024, h: 1024},
		{name: "custom pixel exact", in: GenerationRequest{SizeMode: "pixel", RequestedSize: "1280x720", OutputFormat: "png"}, size: "1280x720", w: 1280, h: 720},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeGenerationRequest(generationTestCapability(), tt.in)
			if err != nil {
				t.Fatalf("NormalizeGenerationRequest() error = %v", err)
			}
			if got.OutboundSize != tt.size || got.Width != tt.w || got.Height != tt.h {
				t.Fatalf("got size=%q %dx%d", got.OutboundSize, got.Width, got.Height)
			}
		})
	}
}

func TestNormalizeGenerationRequestRejectsInvalidExplicitInput(t *testing.T) {
	capability := generationTestCapability()
	capability.SupportsCustomRatio = false
	capability.SupportedBackgrounds = []string{"auto", "transparent"}
	tests := []struct {
		name string
		in   GenerationRequest
		code string
	}{
		{name: "auto stale base", in: GenerationRequest{SizeMode: "auto", BaseResolution: "1k", OutputFormat: "png"}, code: CodeInvalidSizeMode},
		{name: "auto stale size", in: GenerationRequest{SizeMode: "auto", RequestedSize: "1024x1024", OutputFormat: "png"}, code: CodeInvalidSizeMode},
		{name: "ratio custom disabled", in: GenerationRequest{SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "7:5", OutputFormat: "png"}, code: CodeInvalidAspectRatio},
		{name: "pixel is never rounded", in: GenerationRequest{SizeMode: "pixel", RequestedSize: "1001x777", OutputFormat: "png"}, code: CodeInvalidExplicitDimensions},
		{name: "pixel outside bounds", in: GenerationRequest{SizeMode: "pixel", RequestedSize: "3840x2160", OutputFormat: "png"}, code: CodeInvalidExplicitDimensions},
		{name: "transparent jpeg", in: GenerationRequest{SizeMode: "auto", Background: "transparent", OutputFormat: "jpeg"}, code: CodeTransparentFormatConflict},
		{name: "unsupported background", in: GenerationRequest{SizeMode: "auto", Background: "opaque", OutputFormat: "png"}, code: errs.CodeImageCapabilityMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NormalizeGenerationRequest(capability, tt.in)
			var appErr *errs.Error
			if !errors.As(err, &appErr) || appErr.Code != tt.code {
				t.Fatalf("error = %#v, want code %s", err, tt.code)
			}
		})
	}
}

func TestNormalizeCapabilityRejectsInvalidNewConfiguration(t *testing.T) {
	base := generationTestCapability()
	tests := []struct {
		name string
		edit func(*ImageModelCapability)
	}{
		{name: "base auto", edit: func(c *ImageModelCapability) { c.BaseResolution = []string{"auto", "1k"} }},
		{name: "bad pixel preset", edit: func(c *ImageModelCapability) { c.SupportedPixelSizes = []string{"1001x777"} }},
		{name: "invalid bounds", edit: func(c *ImageModelCapability) { c.MinWidth, c.MaxWidth = 2048, 1024 }},
		{name: "bad background", edit: func(c *ImageModelCapability) { c.SupportedBackgrounds = []string{"blue"} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := base
			tt.edit(&candidate)
			if _, err := NormalizeCapability(candidate); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestFilterEffectiveCapabilityDropsLegacyInvalidOptions(t *testing.T) {
	got := FilterEffectiveCapability(ImageModelCapability{MaxImageCount: 1, SizeModes: []string{"auto", "ratio", "pixel"}, BaseResolution: []string{"auto", "1k", "weird"}, SupportedRatios: []string{"1:1", "4:1"}, SupportedPixelSizes: []string{"1024x1024", "1001x777", "4096x4096"}, MinWidth: 512, MaxWidth: 2048, MinHeight: 512, MaxHeight: 2048})
	if len(got.BaseResolution) != 1 || got.BaseResolution[0] != "1k" {
		t.Fatalf("base resolutions = %#v", got.BaseResolution)
	}
	if len(got.SupportedRatios) != 1 || got.SupportedRatios[0] != "1:1" {
		t.Fatalf("ratios = %#v", got.SupportedRatios)
	}
	if len(got.SupportedPixelSizes) != 1 || got.SupportedPixelSizes[0] != "1024x1024" {
		t.Fatalf("pixel sizes = %#v", got.SupportedPixelSizes)
	}
}
