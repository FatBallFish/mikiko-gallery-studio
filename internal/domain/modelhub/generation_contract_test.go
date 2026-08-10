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
		{name: "ratio custom disabled", in: GenerationRequest{SizeMode: "ratio", BaseResolution: "1k", AspectRatio: "7:5", OutputFormat: "png"}, code: errs.CodeImageCapabilityMismatch},
		{name: "pixel is never rounded", in: GenerationRequest{SizeMode: "pixel", RequestedSize: "1001x777", OutputFormat: "png"}, code: errs.CodeImageCapabilityMismatch},
		{name: "pixel inside bounds is still never rounded", in: GenerationRequest{SizeMode: "pixel", RequestedSize: "899x899", OutputFormat: "png"}, code: errs.CodeImageCapabilityMismatch},
		{name: "pixel outside bounds", in: GenerationRequest{SizeMode: "pixel", RequestedSize: "3840x2160", OutputFormat: "png"}, code: errs.CodeImageCapabilityMismatch},
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

func TestNormalizeGenerationRequestReportsSafeFieldDetails(t *testing.T) {
	tests := []struct {
		name    string
		request GenerationRequest
		field   string
		rule    string
		wantMin int
		wantMax int
	}{
		{name: "pixel width range", request: GenerationRequest{SizeMode: SizeModePixel, RequestedSize: "4096x1024", OutputFormat: "png"}, field: "width", rule: "range", wantMin: 512, wantMax: 2560},
		{name: "pixel grid", request: GenerationRequest{SizeMode: SizeModePixel, RequestedSize: "1001x777", OutputFormat: "png"}, field: "pixel_size", rule: "multiple_of_16"},
		{name: "ratio format", request: GenerationRequest{SizeMode: SizeModeRatio, BaseResolution: "1k", AspectRatio: "wide", OutputFormat: "png"}, field: "aspect_ratio", rule: "format"},
		{name: "ratio limit", request: GenerationRequest{SizeMode: SizeModeRatio, BaseResolution: "1k", AspectRatio: "4:1", OutputFormat: "png"}, field: "aspect_ratio", rule: "max_ratio", wantMax: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NormalizeGenerationRequest(generationTestCapability(), tt.request)
			var appErr *errs.Error
			if !errors.As(err, &appErr) {
				t.Fatalf("error = %#v, want *errs.Error", err)
			}
			if appErr.Code != errs.CodeImageCapabilityMismatch {
				t.Fatalf("code = %q, want %q", appErr.Code, errs.CodeImageCapabilityMismatch)
			}
			if appErr.Details["field"] != tt.field || appErr.Details["rule"] != tt.rule {
				t.Fatalf("details = %#v, want field=%q rule=%q", appErr.Details, tt.field, tt.rule)
			}
			if tt.wantMin > 0 && appErr.Details["min"] != tt.wantMin {
				t.Fatalf("details min = %#v, want %d", appErr.Details["min"], tt.wantMin)
			}
			if tt.wantMax > 0 && appErr.Details["max"] != tt.wantMax {
				t.Fatalf("details max = %#v, want %d", appErr.Details["max"], tt.wantMax)
			}
		})
	}
}

func TestNormalizeGenerationRequestResolvesRatioInsideCapabilityBounds(t *testing.T) {
	capability := generationTestCapability()
	capability.MinWidth, capability.MaxWidth = 512, 900
	capability.MinHeight, capability.MaxHeight = 512, 900
	got, err := NormalizeGenerationRequest(capability, GenerationRequest{
		SizeMode: SizeModeRatio, BaseResolution: "1k", AspectRatio: "1:1", OutputFormat: "png",
	})
	if err != nil || got.OutboundSize != "896x896" || got.RequestedSize != "896x896" || got.Width != 896 || got.Height != 896 {
		t.Fatalf("bounded ratio normalization = %#v, %v; want 896x896", got, err)
	}
}

func TestNormalizeGenerationRequestReturnsTypedErrorWhenRatioHasNoBoundedSolution(t *testing.T) {
	capability := generationTestCapability()
	capability.MinWidth, capability.MaxWidth = 512, 700
	capability.MinHeight, capability.MaxHeight = 512, 700
	_, err := NormalizeGenerationRequest(capability, GenerationRequest{
		SizeMode: SizeModeRatio, BaseResolution: "1k", AspectRatio: "1:1", OutputFormat: "png",
	})
	var appErr *errs.Error
	if !errors.As(err, &appErr) || appErr.StatusCode != 400 || appErr.Code != errs.CodeImageCapabilityMismatch {
		t.Fatalf("bounded ratio error = %#v, want 400/%s", err, errs.CodeImageCapabilityMismatch)
	}
}

func TestNormalizeResolveRequestRejectsMixedDiscriminatedSizeFields(t *testing.T) {
	tests := []struct {
		name string
		req  ResolveRequest
		code string
	}{
		{name: "auto base", req: ResolveRequest{SizeMode: SizeModeAuto, BaseResolution: "1k"}, code: CodeInvalidSizeMode},
		{name: "auto aspect", req: ResolveRequest{SizeMode: SizeModeAuto, AspectRatio: "1:1"}, code: CodeInvalidSizeMode},
		{name: "auto pixels", req: ResolveRequest{SizeMode: SizeModeAuto, RequestedSize: "1024x1024"}, code: CodeInvalidSizeMode},
		{name: "ratio auto base", req: ResolveRequest{SizeMode: SizeModeRatio, BaseResolution: "auto", AspectRatio: "1:1"}, code: CodeInvalidSizeMode},
		{name: "ratio requested size", req: ResolveRequest{SizeMode: SizeModeRatio, BaseResolution: "1k", AspectRatio: "1:1", RequestedSize: "1024x1024"}, code: CodeInvalidSizeMode},
		{name: "ratio missing aspect", req: ResolveRequest{SizeMode: SizeModeRatio, BaseResolution: "1k"}, code: errs.CodeImageCapabilityMismatch},
		{name: "pixel base", req: ResolveRequest{SizeMode: SizeModePixel, BaseResolution: "1k", RequestedSize: "1024x1024"}, code: CodeInvalidSizeMode},
		{name: "pixel aspect", req: ResolveRequest{SizeMode: SizeModePixel, AspectRatio: "1:1", RequestedSize: "1024x1024"}, code: CodeInvalidSizeMode},
		{name: "ratio hard bound", req: ResolveRequest{SizeMode: SizeModeRatio, BaseResolution: "1k", AspectRatio: "4:1"}, code: errs.CodeImageCapabilityMismatch},
		{name: "illegal pixels", req: ResolveRequest{SizeMode: SizeModePixel, RequestedSize: "1001x777"}, code: errs.CodeImageCapabilityMismatch},
		{name: "transparent jpeg", req: ResolveRequest{SizeMode: SizeModeAuto, Background: "transparent", OutputFormat: "jpeg"}, code: CodeTransparentFormatConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NormalizeResolveRequest(tt.req)
			var appErr *errs.Error
			if !errors.As(err, &appErr) || appErr.StatusCode != 400 || appErr.Code != tt.code {
				t.Fatalf("NormalizeResolveRequest() error = %#v, want 400/%s", err, tt.code)
			}
		})
	}
}

func TestNormalizeResolveRequestReportsExplicitFieldDetails(t *testing.T) {
	tests := []struct {
		name    string
		request ResolveRequest
		field   string
		rule    string
	}{
		{name: "ratio format", request: ResolveRequest{SizeMode: SizeModeRatio, BaseResolution: "1k", AspectRatio: "wide"}, field: "aspect_ratio", rule: "format"},
		{name: "ratio maximum", request: ResolveRequest{SizeMode: SizeModeRatio, BaseResolution: "1k", AspectRatio: "4:1"}, field: "aspect_ratio", rule: "max_ratio"},
		{name: "pixel grid", request: ResolveRequest{SizeMode: SizeModePixel, RequestedSize: "1001x777"}, field: "pixel_size", rule: "multiple_of_16"},
		{name: "pixel width range", request: ResolveRequest{SizeMode: SizeModePixel, RequestedSize: "4096x1024"}, field: "width", rule: "range"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NormalizeResolveRequest(tt.request)
			var appErr *errs.Error
			if !errors.As(err, &appErr) || appErr.Code != errs.CodeImageCapabilityMismatch {
				t.Fatalf("error = %#v, want %s", err, errs.CodeImageCapabilityMismatch)
			}
			if appErr.Details["field"] != tt.field || appErr.Details["rule"] != tt.rule {
				t.Fatalf("details = %#v, want field=%q rule=%q", appErr.Details, tt.field, tt.rule)
			}
		})
	}
}

func TestNormalizeResolveRequestLegacyRatioDefaultsMissingAspect(t *testing.T) {
	got, err := NormalizeResolveRequest(ResolveRequest{BaseResolution: "1k"})
	if err != nil {
		t.Fatalf("NormalizeResolveRequest() error = %v", err)
	}
	if got.SizeMode != sizeModeLegacyRatio || got.AspectRatio != "1:1" {
		t.Fatalf("legacy normalized request = %#v, want legacy ratio with 1:1 aspect", got)
	}
}

func TestCandidateSupportsLegalCustomRatio(t *testing.T) {
	candidate := ProviderCandidate{
		SupportedTaskTypes: []string{"text_to_image"}, SupportedBaseResolution: []string{"1k"},
		SizeModes: []string{SizeModeRatio}, SupportedAspectRatios: []string{"1:1"}, SupportsCustomRatio: true,
		Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"}, MaxImageCount: 1,
		MinWidth: 16, MaxWidth: 3840, MinHeight: 16, MaxHeight: 3840,
	}
	req := ResolveRequest{
		TaskType: "text_to_image", SizeMode: SizeModeRatio, BaseResolution: "1k", AspectRatio: "7:5",
		Quality: "auto", OutputFormat: "png", Moderation: "auto", RequestedOutputImageCount: 1,
	}
	if !CandidateSupportsRequest(candidate, req, "1k") {
		t.Fatal("candidate with supports_custom_ratio must accept a legal non-preset ratio")
	}
}

func TestNormalizeCapabilityRejectsInvalidNewConfiguration(t *testing.T) {
	base := generationTestCapability()
	tests := []struct {
		name string
		edit func(*ImageModelCapability)
	}{
		{name: "base auto", edit: func(c *ImageModelCapability) { c.BaseResolution = []string{"auto", "1k"} }},
		{name: "mixed invalid quality", edit: func(c *ImageModelCapability) { c.Quality = []string{"auto", "ultra"} }},
		{name: "mixed invalid size mode", edit: func(c *ImageModelCapability) { c.SizeModes = []string{"ratio", "automatic"} }},
		{name: "mixed invalid ratio", edit: func(c *ImageModelCapability) { c.SupportedRatios = []string{"1:1", "bad"} }},
		{name: "ratio outside hard bound", edit: func(c *ImageModelCapability) { c.SupportedRatios = []string{"1:1", "4:1"} }},
		{name: "bad pixel preset", edit: func(c *ImageModelCapability) { c.SupportedPixelSizes = []string{"1001x777"} }},
		{name: "mixed invalid pixel preset", edit: func(c *ImageModelCapability) { c.SupportedPixelSizes = []string{"1024x1024", "bad"} }},
		{name: "incomplete pixel bounds", edit: func(c *ImageModelCapability) { c.MinHeight = 0 }},
		{name: "invalid bounds", edit: func(c *ImageModelCapability) { c.MinWidth, c.MaxWidth = 2048, 1024 }},
		{name: "bad background", edit: func(c *ImageModelCapability) { c.SupportedBackgrounds = []string{"blue"} }},
		{name: "mixed invalid output format", edit: func(c *ImageModelCapability) { c.OutputFormat = []string{"png", "jpg"} }},
		{name: "mixed invalid moderation", edit: func(c *ImageModelCapability) { c.Moderation = []string{"auto", "strict"} }},
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

func TestFilterEffectiveCapabilityDropsRatiosWithoutBoundedSolutions(t *testing.T) {
	got := FilterEffectiveCapability(ImageModelCapability{
		MaxImageCount: 1, SizeModes: []string{SizeModeRatio}, BaseResolution: []string{"1k"}, SupportedRatios: []string{"1:1", "16:9"},
		MinWidth: 512, MaxWidth: 900, MinHeight: 512, MaxHeight: 900,
	})
	if !containsString(got.SizeModes, SizeModeRatio) || len(got.SupportedRatios) != 1 || got.SupportedRatios[0] != "1:1" {
		t.Fatalf("effective bounded ratio capability = %#v, want ratio mode with only 1:1", got)
	}

	noSolution := FilterEffectiveCapability(ImageModelCapability{
		MaxImageCount: 1, SizeModes: []string{SizeModeRatio}, BaseResolution: []string{"1k"}, SupportedRatios: []string{"1:1"},
		MinWidth: 512, MaxWidth: 700, MinHeight: 512, MaxHeight: 700,
	})
	if containsString(noSolution.SizeModes, SizeModeRatio) || len(noSolution.SupportedRatios) != 0 {
		t.Fatalf("unsatisfiable ratio mode must be removed from effective capability: %#v", noSolution)
	}
}

func TestNormalizeCapabilityRejectsRatioConfigurationWithoutBoundedSolution(t *testing.T) {
	capability := generationTestCapability()
	capability.SizeModes = []string{SizeModeRatio}
	capability.BaseResolution = []string{"1k"}
	capability.SupportedRatios = []string{"1:1"}
	capability.MinWidth, capability.MaxWidth = 512, 700
	capability.MinHeight, capability.MaxHeight = 512, 700
	if _, err := NormalizeCapability(capability); err == nil {
		t.Fatal("configuration with no legal bounded ratio size must be rejected")
	}
}
