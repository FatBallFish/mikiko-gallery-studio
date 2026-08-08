package modelhub

import (
	"fmt"
	"strings"

	"github.com/fatballfish/pic-gallery/pkg/errs"
)

const (
	SizeModeAuto            = "auto"
	SizeModeRatio           = "ratio"
	SizeModePixel           = "pixel"
	sizeModeLegacyRatio     = "legacy_ratio_size"
	MaxTaskOutputImageCount = 1000
)

var (
	DefaultSizeModes           = []string{SizeModeRatio}
	DefaultSupportedRatios     = []string{"1:1", "16:9", "9:16", "4:3", "3:4"}
	DefaultSupportedPixelSizes = []string{"1024x1024"}
	DefaultQuality             = []string{"auto"}
	DefaultOutputFormat        = []string{"png"}
	DefaultModeration          = []string{"auto"}
)

type ImageModelCapability struct {
	MaxReferenceImageCount    int
	MaxImageCount             int
	BaseResolution            []string
	Quality                   []string
	SizeModes                 []string
	SupportedRatios           []string
	SupportedPixelSizes       []string
	SupportsCustomRatio       bool
	SupportedBackgrounds      []string
	OutputFormat              []string
	OutputCompression         int
	SupportsOutputCompression bool
	SupportsCustomSize        bool
	MinWidth                  int
	MaxWidth                  int
	MinHeight                 int
	MaxHeight                 int
	Moderation                []string
}

func NormalizeCapability(raw ImageModelCapability) (ImageModelCapability, error) {
	capability := raw
	if capability.MaxReferenceImageCount < 0 {
		return capability, errs.BadRequest("max_reference_image_count must be greater than or equal to 0")
	}
	if capability.MaxImageCount < 1 || capability.MaxImageCount > 10 {
		return capability, errs.BadRequest("max_image_count must be between 1 and 10")
	}
	for _, item := range raw.BaseResolution {
		if strings.EqualFold(strings.TrimSpace(item), SizeModeAuto) {
			return capability, errs.BadRequest("base_resolution must not contain auto")
		}
	}
	capability.BaseResolution = normalizeBaseResolution(defaultStrings(capability.BaseResolution, []string{"1k"}))
	if len(capability.BaseResolution) == 0 {
		return capability, errs.BadRequest("base_resolution is required")
	}
	capability.Quality = normalizeEnumStrings(defaultStrings(capability.Quality, DefaultQuality), map[string]struct{}{"auto": {}, "low": {}, "medium": {}, "high": {}})
	if len(capability.Quality) == 0 {
		return capability, errs.BadRequest("quality is required")
	}
	capability.SizeModes = normalizeSizeModes(capability.SizeModes)
	if len(capability.SizeModes) == 0 {
		if len(raw.SizeModes) > 0 {
			return capability, errs.BadRequest("unsupported size_modes")
		}
		capability.SizeModes = cloneStrings(DefaultSizeModes)
	}
	if containsString(capability.SizeModes, SizeModeRatio) {
		capability.SupportedRatios = normalizeRatios(defaultStrings(capability.SupportedRatios, DefaultSupportedRatios))
		if len(capability.SupportedRatios) == 0 {
			return capability, errs.BadRequest("ratio mode requires supported_ratios")
		}
	} else {
		capability.SupportedRatios = nil
	}
	if containsString(capability.SizeModes, SizeModePixel) {
		capability.SupportedPixelSizes = normalizePixelSizes(defaultStrings(capability.SupportedPixelSizes, DefaultSupportedPixelSizes))
		if len(capability.SupportedPixelSizes) == 0 {
			return capability, errs.BadRequest("pixel mode requires supported_pixel_sizes")
		}
	} else {
		capability.SupportedPixelSizes = nil
	}
	if err := validateConfiguredPixelBounds(capability); err != nil {
		return capability, err
	}
	for _, size := range capability.SupportedPixelSizes {
		width, height, ok := ParseImageSize(size)
		if !ok || !legalExplicitDimensions(width, height, capability) {
			return capability, errs.BadRequest("supported_pixel_sizes contains an invalid size")
		}
	}
	capability.SupportedBackgrounds = normalizeEnumStrings(capability.SupportedBackgrounds, map[string]struct{}{"auto": {}, "opaque": {}, "transparent": {}})
	if len(raw.SupportedBackgrounds) != len(capability.SupportedBackgrounds) {
		return capability, errs.BadRequest("supported_backgrounds contains an unsupported value")
	}
	capability.OutputFormat = normalizeEnumStrings(defaultStrings(capability.OutputFormat, DefaultOutputFormat), map[string]struct{}{"png": {}, "jpeg": {}, "webp": {}})
	if len(capability.OutputFormat) == 0 {
		return capability, errs.BadRequest("output_format is required")
	}
	if capability.OutputCompression < 0 || capability.OutputCompression > 100 {
		return capability, errs.BadRequest("output_compression must be between 0 and 100")
	}
	if capability.OutputCompression == 0 {
		capability.OutputCompression = 100
	}
	capability.Moderation = normalizeEnumStrings(defaultStrings(capability.Moderation, DefaultModeration), map[string]struct{}{"auto": {}, "low": {}})
	if len(capability.Moderation) == 0 {
		return capability, errs.BadRequest("moderation is required")
	}
	return capability, nil
}

func NormalizeResolveRequest(req ResolveRequest) (ResolveRequest, error) {
	if req.RequestedOutputImageCount <= 0 {
		req.RequestedOutputImageCount = 1
	}
	if req.RequestedOutputImageCount > MaxTaskOutputImageCount {
		return req, errs.New(400, errs.CodeImageCapabilityMismatch, fmt.Sprintf("requested output image count exceeds task safety limit %d", MaxTaskOutputImageCount))
	}
	req.Quality = NormalizeQuality(req.Quality)
	if req.Quality == "" {
		return req, errs.New(400, errs.CodeImageCapabilityMismatch, "unsupported quality")
	}
	req.OutputFormat = NormalizeOutputFormat(req.OutputFormat)
	if req.OutputFormat == "" {
		return req, errs.New(400, errs.CodeImageCapabilityMismatch, "unsupported output_format")
	}
	if req.OutputCompression < 0 || req.OutputCompression > 100 {
		return req, errs.BadRequest("output_compression must be between 0 and 100")
	}
	if req.OutputCompression == 0 {
		req.OutputCompression = 100
	}
	req.Moderation = NormalizeModeration(req.Moderation)
	if req.Moderation == "" {
		return req, errs.New(400, errs.CodeImageCapabilityMismatch, "unsupported moderation")
	}
	mode := strings.ToLower(strings.TrimSpace(req.SizeMode))
	if mode == "" {
		if strings.TrimSpace(req.AspectRatio) == "" {
			if size := NormalizePixelSize(req.RequestedSize); size != "" {
				req.SizeMode = sizeModeLegacyRatio
				req.RequestedSize = size
				req.AspectRatio = RatioFromPixelSize(size)
				return req, nil
			}
		}
		mode = SizeModeRatio
	}
	switch mode {
	case SizeModeAuto:
		if strings.TrimSpace(req.BaseResolution) != "" || strings.TrimSpace(req.AspectRatio) != "" || strings.TrimSpace(req.RequestedSize) != "" {
			return req, errs.New(400, CodeInvalidSizeMode, "auto size_mode does not accept size fields")
		}
		req.SizeMode = SizeModeAuto
		return req, nil
	case sizeModeLegacyRatio:
		size := NormalizePixelSize(req.RequestedSize)
		if size == "" {
			return req, errs.New(400, errs.CodeImageAutoUnsupported, "unsupported size")
		}
		req.SizeMode = sizeModeLegacyRatio
		req.RequestedSize = size
		req.AspectRatio = RatioFromPixelSize(size)
		return req, nil
	case SizeModeRatio:
		ratio := NormalizeRatio(req.AspectRatio)
		if ratio == "" {
			if strings.TrimSpace(req.AspectRatio) != "" {
				return req, errs.New(400, errs.CodeImageCapabilityMismatch, "unsupported aspect ratio")
			}
			ratio = "1:1"
		}
		req.SizeMode = SizeModeRatio
		req.AspectRatio = ratio
		req.BaseResolution = normalizeBaseResolutionValue(req.BaseResolution)
		if strings.TrimSpace(req.BaseResolution) == "" {
			req.BaseResolution = "auto"
		}
		if strings.TrimSpace(req.RequestedSize) == "" {
			req.RequestedSize = "auto"
		}
		return req, nil
	case SizeModePixel:
		width, height, ok := ParseImageSize(req.RequestedSize)
		if !ok {
			return req, errs.New(400, errs.CodeImageAutoUnsupported, "unsupported size")
		}
		if !IsLegalCustomImageSize(width, height) {
			return req, errs.New(400, errs.CodeImageAutoUnsupported, "unsupported size")
		}
		size := fmt.Sprintf("%dx%d", width, height)
		req.SizeMode = SizeModePixel
		req.RequestedSize = size
		req.BaseResolution = normalizeBaseResolutionValue(req.BaseResolution)
		if req.BaseResolution == "" {
			req.BaseResolution = "auto"
		}
		req.AspectRatio = RatioFromPixelSize(size)
		return req, nil
	default:
		return req, errs.BadRequest("unsupported size_mode")
	}
}

func NormalizeQuality(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "auto":
		return "auto"
	case "low", "medium", "high":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func NormalizeOutputFormat(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "png":
		return "png"
	case "jpeg", "webp":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func NormalizeModeration(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "auto":
		return "auto"
	case "low":
		return "low"
	default:
		return ""
	}
}

func PublicSizeMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), sizeModeLegacyRatio) {
		return SizeModeRatio
	}
	normalized := strings.ToLower(strings.TrimSpace(mode))
	if normalized == SizeModeAuto {
		return SizeModeAuto
	}
	if normalized == SizeModePixel {
		return SizeModePixel
	}
	return SizeModeRatio
}

func BaseResolutionByPixelSize(size string) (string, error) {
	width, height, ok := ParseImageSize(size)
	if !ok {
		return "", errs.New(400, errs.CodeImageAutoUnsupported, "unsupported size")
	}
	longest := width
	if height > longest {
		longest = height
	}
	switch {
	case longest <= 1024:
		return "1k", nil
	case longest <= 2048:
		return "2k", nil
	case longest <= 4096:
		return "4k", nil
	default:
		return "", errs.New(400, errs.CodeImageAutoUnsupported, "unsupported size")
	}
}

func NormalizePixelSize(size string) string {
	width, height, ok := ParseImageSize(size)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%dx%d", width, height)
}

func RatioFromPixelSize(size string) string {
	width, height, ok := ParseImageSize(size)
	if !ok {
		return ""
	}
	divisor := gcd(width, height)
	return fmt.Sprintf("%d:%d", width/divisor, height/divisor)
}

func NormalizeRatio(value string) string {
	width, height, ok := parseRatio(value)
	if !ok {
		return ""
	}
	divisor := gcd(width, height)
	return fmt.Sprintf("%d:%d", width/divisor, height/divisor)
}

func normalizeSizeModes(values []string) []string {
	seen := map[string]struct{}{}
	result := []string{}
	for _, value := range values {
		mode := strings.ToLower(strings.TrimSpace(value))
		if mode != SizeModeAuto && mode != SizeModeRatio && mode != SizeModePixel {
			continue
		}
		if _, ok := seen[mode]; ok {
			continue
		}
		seen[mode] = struct{}{}
		result = append(result, mode)
	}
	return result
}

func normalizeRatios(values []string) []string {
	seen := map[string]struct{}{}
	result := []string{}
	for _, value := range values {
		ratio := NormalizeRatio(value)
		if ratio == "" {
			continue
		}
		if _, ok := seen[ratio]; ok {
			continue
		}
		seen[ratio] = struct{}{}
		result = append(result, ratio)
	}
	return result
}

func normalizePixelSizes(values []string) []string {
	seen := map[string]struct{}{}
	result := []string{}
	for _, value := range values {
		size := NormalizePixelSize(value)
		if size == "" {
			continue
		}
		if _, ok := seen[size]; ok {
			continue
		}
		seen[size] = struct{}{}
		result = append(result, size)
	}
	return result
}

func normalizeBaseResolution(values []string) []string {
	seen := map[string]struct{}{}
	result := []string{}
	for _, value := range values {
		item := normalizeBaseResolutionValue(value)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func normalizeBaseResolutionValue(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto":
		return "auto"
	case "1k", "2k", "4k":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizeEnumStrings(values []string, allowed map[string]struct{}) []string {
	seen := map[string]struct{}{}
	result := []string{}
	for _, value := range values {
		item := strings.ToLower(strings.TrimSpace(value))
		if _, ok := allowed[item]; !ok {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func defaultStrings(values, fallback []string) []string {
	if len(values) > 0 {
		return values
	}
	return fallback
}
