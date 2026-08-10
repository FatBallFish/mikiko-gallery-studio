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
		if normalizeBaseResolutionValue(item) == "" || strings.EqualFold(strings.TrimSpace(item), SizeModeAuto) {
			return capability, errs.BadRequest("base_resolution contains an unsupported value")
		}
	}
	capability.BaseResolution = normalizeBaseResolution(defaultStrings(capability.BaseResolution, []string{"1k"}))
	if len(capability.BaseResolution) == 0 {
		return capability, errs.BadRequest("base_resolution is required")
	}
	qualityValues := defaultStrings(capability.Quality, DefaultQuality)
	if !allEnumStringsAllowed(qualityValues, map[string]struct{}{"auto": {}, "low": {}, "medium": {}, "high": {}}) {
		return capability, errs.BadRequest("quality contains an unsupported value")
	}
	capability.Quality = normalizeEnumStrings(qualityValues, map[string]struct{}{"auto": {}, "low": {}, "medium": {}, "high": {}})
	if len(capability.Quality) == 0 {
		return capability, errs.BadRequest("quality is required")
	}
	if len(raw.SizeModes) > 0 && !allEnumStringsAllowed(raw.SizeModes, map[string]struct{}{SizeModeAuto: {}, SizeModeRatio: {}, SizeModePixel: {}}) {
		return capability, errs.BadRequest("size_modes contains an unsupported value")
	}
	capability.SizeModes = normalizeSizeModes(capability.SizeModes)
	if len(capability.SizeModes) == 0 {
		if len(raw.SizeModes) > 0 {
			return capability, errs.BadRequest("unsupported size_modes")
		}
		capability.SizeModes = cloneStrings(DefaultSizeModes)
	}
	if containsString(capability.SizeModes, SizeModeRatio) {
		ratioValues := defaultStrings(capability.SupportedRatios, DefaultSupportedRatios)
		for _, item := range ratioValues {
			ratio := NormalizeRatio(item)
			width, height, ok := parseRatio(ratio)
			if !ok || maxFloat(float64(width)/float64(height), float64(height)/float64(width)) > imageMaxAspectRatio {
				return capability, errs.BadRequest("supported_ratios contains an invalid ratio")
			}
		}
		capability.SupportedRatios = normalizeRatios(ratioValues)
		if len(capability.SupportedRatios) == 0 {
			return capability, errs.BadRequest("ratio mode requires supported_ratios")
		}
	} else {
		capability.SupportedRatios = nil
	}
	if containsString(capability.SizeModes, SizeModePixel) {
		pixelValues := defaultStrings(capability.SupportedPixelSizes, DefaultSupportedPixelSizes)
		for _, item := range pixelValues {
			width, height, ok := ParseImageSize(NormalizePixelSize(item))
			if !ok || !IsLegalCustomImageSize(width, height) {
				return capability, errs.BadRequest("supported_pixel_sizes contains an invalid size")
			}
		}
		capability.SupportedPixelSizes = normalizePixelSizes(pixelValues)
		if len(capability.SupportedPixelSizes) == 0 {
			return capability, errs.BadRequest("pixel mode requires supported_pixel_sizes")
		}
	} else {
		capability.SupportedPixelSizes = nil
	}
	if err := validateConfiguredPixelBounds(capability); err != nil {
		return capability, err
	}
	if containsString(capability.SizeModes, SizeModeRatio) {
		for _, baseResolution := range capability.BaseResolution {
			for _, ratio := range capability.SupportedRatios {
				if _, err := CalculateImageSizeWithinCapability(baseResolution, ratio, capability); err != nil {
					return capability, errs.BadRequest("base_resolution and supported_ratios contain an unsatisfiable combination")
				}
			}
		}
	}
	for _, size := range capability.SupportedPixelSizes {
		width, height, ok := ParseImageSize(size)
		if !ok || !legalExplicitDimensions(width, height, capability) {
			return capability, errs.BadRequest("supported_pixel_sizes contains an invalid size")
		}
	}
	if !allEnumStringsAllowed(raw.SupportedBackgrounds, map[string]struct{}{"auto": {}, "opaque": {}, "transparent": {}}) {
		return capability, errs.BadRequest("supported_backgrounds contains an unsupported value")
	}
	capability.SupportedBackgrounds = normalizeEnumStrings(capability.SupportedBackgrounds, map[string]struct{}{"auto": {}, "opaque": {}, "transparent": {}})
	outputFormats := defaultStrings(capability.OutputFormat, DefaultOutputFormat)
	if !allEnumStringsAllowed(outputFormats, map[string]struct{}{"png": {}, "jpeg": {}, "webp": {}}) {
		return capability, errs.BadRequest("output_format contains an unsupported value")
	}
	capability.OutputFormat = normalizeEnumStrings(outputFormats, map[string]struct{}{"png": {}, "jpeg": {}, "webp": {}})
	if len(capability.OutputFormat) == 0 {
		return capability, errs.BadRequest("output_format is required")
	}
	if capability.OutputCompression < 0 || capability.OutputCompression > 100 {
		return capability, errs.BadRequest("output_compression must be between 0 and 100")
	}
	if capability.OutputCompression == 0 {
		capability.OutputCompression = 100
	}
	moderationValues := defaultStrings(capability.Moderation, DefaultModeration)
	if !allEnumStringsAllowed(moderationValues, map[string]struct{}{"auto": {}, "low": {}}) {
		return capability, errs.BadRequest("moderation contains an unsupported value")
	}
	capability.Moderation = normalizeEnumStrings(moderationValues, map[string]struct{}{"auto": {}, "low": {}})
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
	req.Background = strings.ToLower(strings.TrimSpace(req.Background))
	if req.Background == "transparent" && req.OutputFormat != "png" && req.OutputFormat != "webp" {
		return req, errs.New(400, CodeTransparentFormatConflict, "transparent background requires png or webp")
	}
	req.Moderation = NormalizeModeration(req.Moderation)
	if req.Moderation == "" {
		return req, errs.New(400, errs.CodeImageCapabilityMismatch, "unsupported moderation")
	}
	mode := strings.ToLower(strings.TrimSpace(req.SizeMode))
	explicitMode := mode != ""
	if mode == "" {
		mode = sizeModeLegacyRatio
	}
	switch mode {
	case SizeModeAuto:
		if strings.TrimSpace(req.BaseResolution) != "" || strings.TrimSpace(req.AspectRatio) != "" || strings.TrimSpace(req.RequestedSize) != "" {
			return req, errs.New(400, CodeInvalidSizeMode, "auto size_mode does not accept size fields")
		}
		req.SizeMode = SizeModeAuto
		return req, nil
	case sizeModeLegacyRatio:
		ratio := NormalizeRatio(req.AspectRatio)
		if ratio == "" && strings.TrimSpace(req.AspectRatio) != "" {
			return req, errs.New(400, errs.CodeImageCapabilityMismatch, "unsupported aspect ratio")
		}
		size := ""
		if rawSize := strings.TrimSpace(req.RequestedSize); rawSize != "" && !strings.EqualFold(rawSize, "auto") {
			size = NormalizePixelSize(rawSize)
			if size == "" {
				return req, errs.New(400, errs.CodeImageAutoUnsupported, "unsupported size")
			}
		}
		if ratio == "" && size != "" {
			ratio = RatioFromPixelSize(size)
		}
		if ratio == "" {
			ratio = "1:1"
		}
		baseResolution := normalizeBaseResolutionValue(req.BaseResolution)
		if strings.TrimSpace(req.BaseResolution) != "" && baseResolution == "" {
			return req, errs.New(400, errs.CodeImageCapabilityMismatch, "unsupported base_resolution")
		}
		if baseResolution == "" {
			baseResolution = "auto"
		}
		if size == "" {
			size = "auto"
		}
		req.SizeMode = sizeModeLegacyRatio
		req.RequestedSize = size
		req.AspectRatio = ratio
		req.BaseResolution = baseResolution
		return req, nil
	case SizeModeRatio:
		if explicitMode && strings.TrimSpace(req.RequestedSize) != "" {
			return req, errs.New(400, CodeInvalidSizeMode, "ratio size_mode does not accept requested_size")
		}
		if explicitMode && strings.EqualFold(strings.TrimSpace(req.BaseResolution), SizeModeAuto) {
			return req, errs.New(400, CodeInvalidSizeMode, "ratio size_mode requires an explicit base_resolution")
		}
		ratio := NormalizeRatio(req.AspectRatio)
		if ratio == "" {
			rule := "required"
			if strings.TrimSpace(req.AspectRatio) != "" {
				rule = "format"
			}
			return req, generationFieldError("aspect_ratio", rule, "比例格式不合法", map[string]any{"example": "16:9"})
		}
		widthRatio, heightRatio, ok := parseRatio(ratio)
		if !ok {
			return req, generationFieldError("aspect_ratio", "format", "比例格式不合法", map[string]any{"example": "16:9"})
		}
		if maxFloat(float64(widthRatio)/float64(heightRatio), float64(heightRatio)/float64(widthRatio)) > imageMaxAspectRatio {
			return req, generationFieldError("aspect_ratio", "max_ratio", "比例超出平台限制", map[string]any{"max": int(imageMaxAspectRatio)})
		}
		req.SizeMode = SizeModeRatio
		req.AspectRatio = ratio
		req.BaseResolution = normalizeBaseResolutionValue(req.BaseResolution)
		if explicitMode && req.BaseResolution == "" {
			return req, errs.New(400, CodeInvalidSizeMode, "ratio size_mode requires an explicit base_resolution")
		}
		if !explicitMode && strings.TrimSpace(req.BaseResolution) == "" {
			req.BaseResolution = "auto"
		}
		if !explicitMode && strings.TrimSpace(req.RequestedSize) == "" {
			req.RequestedSize = "auto"
		}
		return req, nil
	case SizeModePixel:
		if explicitMode && (strings.TrimSpace(req.BaseResolution) != "" || strings.TrimSpace(req.AspectRatio) != "") {
			return req, errs.New(400, CodeInvalidSizeMode, "pixel size_mode does not accept ratio fields")
		}
		width, height, ok := ParseImageSize(req.RequestedSize)
		if !ok {
			return req, generationFieldError("pixel_size", "format", "像素尺寸格式不合法", map[string]any{"example": "1024x1024"})
		}
		if width%imageSizeMultiple != 0 || height%imageSizeMultiple != 0 {
			return req, generationFieldError("pixel_size", "multiple_of_16", "宽高必须为 16 的倍数", map[string]any{"multiple": imageSizeMultiple})
		}
		if width > imageMaxEdge {
			return req, generationFieldError("width", "range", "宽度超出平台限制", map[string]any{"min": imageSizeMultiple, "max": imageMaxEdge})
		}
		if height > imageMaxEdge {
			return req, generationFieldError("height", "range", "高度超出平台限制", map[string]any{"min": imageSizeMultiple, "max": imageMaxEdge})
		}
		pixels := width * height
		if pixels < imageMinPixels || pixels > imageMaxPixels {
			return req, generationFieldError("pixel_size", "pixel_count", "总像素数超出平台限制", map[string]any{"min": imageMinPixels, "max": imageMaxPixels})
		}
		if maxFloat(float64(width)/float64(height), float64(height)/float64(width)) > imageMaxAspectRatio {
			return req, generationFieldError("pixel_size", "max_ratio", "宽高比例超出平台限制", map[string]any{"max": int(imageMaxAspectRatio)})
		}
		size := fmt.Sprintf("%dx%d", width, height)
		req.SizeMode = SizeModePixel
		req.RequestedSize = size
		if !explicitMode {
			req.BaseResolution = normalizeBaseResolutionValue(req.BaseResolution)
		}
		if !explicitMode && req.BaseResolution == "" {
			req.BaseResolution = "auto"
		}
		if !explicitMode {
			req.AspectRatio = RatioFromPixelSize(size)
		} else {
			req.BaseResolution, req.AspectRatio = "", ""
		}
		return req, nil
	default:
		return req, errs.New(400, CodeInvalidSizeMode, "size_mode is unsupported")
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

func allEnumStringsAllowed(values []string, allowed map[string]struct{}) bool {
	for _, value := range values {
		if _, ok := allowed[strings.ToLower(strings.TrimSpace(value))]; !ok {
			return false
		}
	}
	return true
}

func defaultStrings(values, fallback []string) []string {
	if len(values) > 0 {
		return values
	}
	return fallback
}
