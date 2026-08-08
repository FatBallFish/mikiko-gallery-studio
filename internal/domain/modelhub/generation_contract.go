package modelhub

import (
	"fmt"
	"strings"

	"github.com/fatballfish/pic-gallery/pkg/errs"
)

const (
	CodeInvalidSizeMode           = "invalid_size_mode"
	CodeInvalidExplicitDimensions = "invalid_explicit_dimensions"
	CodeInvalidAspectRatio        = "invalid_aspect_ratio"
	CodeTransparentFormatConflict = "transparent_format_conflict"
)

type GenerationRequest struct {
	SizeMode       string
	BaseResolution string
	AspectRatio    string
	RequestedSize  string
	Background     string
	OutputFormat   string
}

type NormalizedGenerationRequest struct {
	SizeMode       string
	BaseResolution string
	AspectRatio    string
	RequestedSize  string
	OutboundSize   string
	Width          int
	Height         int
	Background     string
	OutputFormat   string
}

func NormalizeGenerationRequest(capability ImageModelCapability, req GenerationRequest) (NormalizedGenerationRequest, error) {
	mode := strings.ToLower(strings.TrimSpace(req.SizeMode))
	if mode == "" || !containsString(capability.SizeModes, mode) {
		return NormalizedGenerationRequest{}, errs.New(400, CodeInvalidSizeMode, "size_mode is unsupported")
	}
	format := NormalizeOutputFormat(req.OutputFormat)
	if format == "" || (len(capability.OutputFormat) > 0 && !containsString(capability.OutputFormat, format)) {
		return NormalizedGenerationRequest{}, errs.New(400, errs.CodeImageCapabilityMismatch, "output_format is unsupported")
	}
	background := strings.ToLower(strings.TrimSpace(req.Background))
	if background != "" && !containsString(capability.SupportedBackgrounds, background) {
		return NormalizedGenerationRequest{}, errs.New(400, errs.CodeImageCapabilityMismatch, "background is unsupported")
	}
	if background == "transparent" && format != "png" && format != "webp" {
		return NormalizedGenerationRequest{}, errs.New(400, CodeTransparentFormatConflict, "transparent background requires png or webp")
	}
	result := NormalizedGenerationRequest{SizeMode: mode, Background: background, OutputFormat: format}
	switch mode {
	case SizeModeAuto:
		if strings.TrimSpace(req.BaseResolution) != "" || strings.TrimSpace(req.AspectRatio) != "" || strings.TrimSpace(req.RequestedSize) != "" {
			return NormalizedGenerationRequest{}, errs.New(400, CodeInvalidSizeMode, "auto size_mode does not accept size fields")
		}
		return result, nil
	case SizeModeRatio:
		base := strings.ToLower(strings.TrimSpace(req.BaseResolution))
		if base == "" || base == SizeModeAuto || !containsString(capability.BaseResolution, base) {
			return NormalizedGenerationRequest{}, errs.New(400, errs.CodeImageCapabilityMismatch, "base_resolution is unsupported")
		}
		if strings.TrimSpace(req.RequestedSize) != "" {
			return NormalizedGenerationRequest{}, errs.New(400, CodeInvalidSizeMode, "ratio size_mode does not accept requested_size")
		}
		ratio := NormalizeRatio(req.AspectRatio)
		widthRatio, heightRatio, ok := parseRatio(ratio)
		if !ok || maxFloat(float64(widthRatio)/float64(heightRatio), float64(heightRatio)/float64(widthRatio)) > imageMaxAspectRatio {
			return NormalizedGenerationRequest{}, errs.New(400, CodeInvalidAspectRatio, "aspect_ratio is invalid")
		}
		if !containsString(capability.SupportedRatios, ratio) && !capability.SupportsCustomRatio {
			return NormalizedGenerationRequest{}, errs.New(400, CodeInvalidAspectRatio, "custom aspect_ratio is unsupported")
		}
		size, err := CalculateImageSize(base, ratio)
		if err != nil {
			return NormalizedGenerationRequest{}, errs.New(400, CodeInvalidAspectRatio, "aspect_ratio cannot be resolved")
		}
		width, height, _ := ParseImageSize(size)
		if !legalExplicitDimensions(width, height, capability) {
			return NormalizedGenerationRequest{}, errs.New(400, CodeInvalidAspectRatio, "resolved size violates model limits")
		}
		result.BaseResolution, result.AspectRatio = base, ratio
		result.RequestedSize, result.OutboundSize, result.Width, result.Height = size, size, width, height
		return result, nil
	case SizeModePixel:
		if strings.TrimSpace(req.BaseResolution) != "" || strings.TrimSpace(req.AspectRatio) != "" {
			return NormalizedGenerationRequest{}, errs.New(400, CodeInvalidSizeMode, "pixel size_mode does not accept ratio fields")
		}
		size := NormalizePixelSize(req.RequestedSize)
		width, height, ok := ParseImageSize(size)
		if !ok || !legalExplicitDimensions(width, height, capability) {
			return NormalizedGenerationRequest{}, errs.New(400, CodeInvalidExplicitDimensions, "explicit dimensions violate model limits")
		}
		if !containsString(capability.SupportedPixelSizes, size) && !capability.SupportsCustomSize {
			return NormalizedGenerationRequest{}, errs.New(400, CodeInvalidExplicitDimensions, "custom dimensions are unsupported")
		}
		result.RequestedSize, result.OutboundSize, result.Width, result.Height = size, size, width, height
		return result, nil
	default:
		return NormalizedGenerationRequest{}, errs.New(400, CodeInvalidSizeMode, "size_mode is unsupported")
	}
}

func FilterEffectiveCapability(raw ImageModelCapability) ImageModelCapability {
	result := raw
	result.SizeModes = normalizeSizeModes(raw.SizeModes)
	result.BaseResolution = nil
	for _, item := range raw.BaseResolution {
		value := strings.ToLower(strings.TrimSpace(item))
		if value == "1k" || value == "2k" || value == "4k" {
			result.BaseResolution = appendUnique(result.BaseResolution, value)
		}
	}
	result.SupportedRatios = nil
	for _, item := range raw.SupportedRatios {
		ratio := NormalizeRatio(item)
		rw, rh, ok := parseRatio(ratio)
		if ok && maxFloat(float64(rw)/float64(rh), float64(rh)/float64(rw)) <= imageMaxAspectRatio {
			result.SupportedRatios = appendUnique(result.SupportedRatios, ratio)
		}
	}
	result.SupportedPixelSizes = nil
	for _, item := range raw.SupportedPixelSizes {
		size := NormalizePixelSize(item)
		width, height, ok := ParseImageSize(size)
		if ok && legalExplicitDimensions(width, height, raw) {
			result.SupportedPixelSizes = appendUnique(result.SupportedPixelSizes, size)
		}
	}
	result.SupportedBackgrounds = normalizeEnumStrings(raw.SupportedBackgrounds, map[string]struct{}{"auto": {}, "opaque": {}, "transparent": {}})
	return result
}

func validateConfiguredPixelBounds(capability ImageModelCapability) error {
	if containsString(capability.SizeModes, SizeModePixel) {
		for _, value := range []int{capability.MinWidth, capability.MaxWidth, capability.MinHeight, capability.MaxHeight} {
			if value < imageSizeMultiple {
				return errs.BadRequest("pixel mode requires complete positive min/max bounds")
			}
		}
	}
	for _, value := range []int{capability.MinWidth, capability.MaxWidth, capability.MinHeight, capability.MaxHeight} {
		if value < 0 || value > imageMaxEdge {
			return errs.BadRequest("pixel bounds exceed hard limits")
		}
	}
	if capability.MinWidth > 0 && capability.MaxWidth > 0 && capability.MinWidth > capability.MaxWidth {
		return errs.BadRequest("min_width must not exceed max_width")
	}
	if capability.MinHeight > 0 && capability.MaxHeight > 0 && capability.MinHeight > capability.MaxHeight {
		return errs.BadRequest("min_height must not exceed max_height")
	}
	return nil
}

func legalExplicitDimensions(width, height int, capability ImageModelCapability) bool {
	if !IsLegalCustomImageSize(width, height) {
		return false
	}
	minWidth, maxWidth := capability.MinWidth, capability.MaxWidth
	minHeight, maxHeight := capability.MinHeight, capability.MaxHeight
	if minWidth == 0 {
		minWidth = imageSizeMultiple
	}
	if maxWidth == 0 {
		maxWidth = imageMaxEdge
	}
	if minHeight == 0 {
		minHeight = imageSizeMultiple
	}
	if maxHeight == 0 {
		maxHeight = imageMaxEdge
	}
	return width >= minWidth && width <= maxWidth && height >= minHeight && height <= maxHeight
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func ResolvedSize(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	return fmt.Sprintf("%dx%d", width, height)
}
