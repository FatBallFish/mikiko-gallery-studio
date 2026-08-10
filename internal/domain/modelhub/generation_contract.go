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
	CodeCapabilityChanged         = "capability_changed"
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
			return NormalizedGenerationRequest{}, generationFieldError("base_resolution", "unsupported", "基础分辨率不受当前模型支持", nil)
		}
		if strings.TrimSpace(req.RequestedSize) != "" {
			return NormalizedGenerationRequest{}, errs.New(400, CodeInvalidSizeMode, "ratio size_mode does not accept requested_size")
		}
		ratio := NormalizeRatio(req.AspectRatio)
		widthRatio, heightRatio, ok := parseRatio(ratio)
		if !ok {
			return NormalizedGenerationRequest{}, generationFieldError("aspect_ratio", "format", "比例格式不合法", map[string]any{"example": "16:9"})
		}
		if maxFloat(float64(widthRatio)/float64(heightRatio), float64(heightRatio)/float64(widthRatio)) > imageMaxAspectRatio {
			return NormalizedGenerationRequest{}, generationFieldError("aspect_ratio", "max_ratio", "比例超出当前模型限制", map[string]any{"max": int(imageMaxAspectRatio)})
		}
		if !containsString(capability.SupportedRatios, ratio) && !capability.SupportsCustomRatio {
			return NormalizedGenerationRequest{}, generationFieldError("aspect_ratio", "unsupported", "当前模型不支持自定义比例", nil)
		}
		size, err := CalculateImageSizeWithinCapability(base, ratio, capability)
		if err != nil {
			return NormalizedGenerationRequest{}, generationFieldError("aspect_ratio", "unresolvable", "当前比例无法生成符合模型限制的尺寸", nil)
		}
		width, height, _ := ParseImageSize(size)
		if !legalExplicitDimensions(width, height, capability) {
			return NormalizedGenerationRequest{}, generationFieldError("aspect_ratio", "resolved_size_range", "比例计算结果超出当前模型尺寸限制", nil)
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
		if !ok {
			return NormalizedGenerationRequest{}, generationFieldError("pixel_size", "format", "像素尺寸格式不合法", map[string]any{"example": "1024x1024"})
		}
		if width%imageSizeMultiple != 0 || height%imageSizeMultiple != 0 {
			return NormalizedGenerationRequest{}, generationFieldError("pixel_size", "multiple_of_16", "宽高必须为 16 的倍数", map[string]any{"multiple": imageSizeMultiple})
		}
		minWidth, maxWidth := effectiveExplicitDimensionBounds(capability.MinWidth, capability.MaxWidth)
		minHeight, maxHeight := effectiveExplicitDimensionBounds(capability.MinHeight, capability.MaxHeight)
		if width < minWidth || width > maxWidth {
			return NormalizedGenerationRequest{}, generationFieldError("width", "range", "宽度超出当前模型限制", map[string]any{"min": minWidth, "max": maxWidth})
		}
		if height < minHeight || height > maxHeight {
			return NormalizedGenerationRequest{}, generationFieldError("height", "range", "高度超出当前模型限制", map[string]any{"min": minHeight, "max": maxHeight})
		}
		pixels := width * height
		if pixels < imageMinPixels || pixels > imageMaxPixels {
			return NormalizedGenerationRequest{}, generationFieldError("pixel_size", "pixel_count", "总像素数超出平台限制", map[string]any{"min": imageMinPixels, "max": imageMaxPixels})
		}
		if maxFloat(float64(width)/float64(height), float64(height)/float64(width)) > imageMaxAspectRatio {
			return NormalizedGenerationRequest{}, generationFieldError("pixel_size", "max_ratio", "宽高比例超出平台限制", map[string]any{"max": int(imageMaxAspectRatio)})
		}
		if !containsString(capability.SupportedPixelSizes, size) && !capability.SupportsCustomSize {
			return NormalizedGenerationRequest{}, generationFieldError("pixel_size", "unsupported", "当前模型不支持该像素尺寸", nil)
		}
		result.RequestedSize, result.OutboundSize, result.Width, result.Height = size, size, width, height
		return result, nil
	default:
		return NormalizedGenerationRequest{}, errs.New(400, CodeInvalidSizeMode, "size_mode is unsupported")
	}
}

func generationFieldError(field, rule, message string, extra map[string]any) *errs.Error {
	details := map[string]any{"field": field, "rule": rule}
	for key, value := range extra {
		details[key] = value
	}
	return errs.WithDetails(errs.New(400, errs.CodeImageCapabilityMismatch, message), details)
}

func effectiveExplicitDimensionBounds(minimum, maximum int) (int, int) {
	if minimum <= 0 {
		minimum = imageSizeMultiple
	}
	if maximum <= 0 || maximum > imageMaxEdge {
		maximum = imageMaxEdge
	}
	return minimum, maximum
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
	filterResolvableRatioCapability(&result)
	return result
}

func filterResolvableRatioCapability(capability *ImageModelCapability) {
	if capability == nil || !containsString(capability.SizeModes, SizeModeRatio) {
		return
	}
	originalBases := cloneStrings(capability.BaseResolution)
	usableBases := make([]string, 0, len(originalBases))
	for _, base := range originalBases {
		usable := capability.SupportsCustomRatio && baseSupportsAnyRatioSize(base, *capability)
		for _, ratio := range capability.SupportedRatios {
			if _, err := CalculateImageSizeWithinCapability(base, ratio, *capability); err == nil {
				usable = true
				break
			}
		}
		if usable {
			usableBases = append(usableBases, base)
		}
	}
	usableRatios := make([]string, 0, len(capability.SupportedRatios))
	for _, ratio := range capability.SupportedRatios {
		usable := len(usableBases) > 0
		for _, base := range usableBases {
			if _, err := CalculateImageSizeWithinCapability(base, ratio, *capability); err != nil {
				usable = false
				break
			}
		}
		if usable {
			usableRatios = append(usableRatios, ratio)
		}
	}
	capability.SupportedRatios = usableRatios
	if len(usableBases) == 0 || len(usableRatios) == 0 && !capability.SupportsCustomRatio {
		capability.SizeModes = removeString(capability.SizeModes, SizeModeRatio)
		capability.BaseResolution = nil
		capability.SupportedRatios = nil
		return
	}
	capability.BaseResolution = usableBases
}

func baseSupportsAnyRatioSize(baseResolution string, capability ImageModelCapability) bool {
	resolution := normalizeSizeBaseResolution(baseResolution)
	pixelBudget := minInt(imageTierPixelBudget[resolution], imageMaxPixels)
	minWidth, maxWidth := effectiveRatioDimensionBounds(capability.MinWidth, capability.MaxWidth)
	minHeight, maxHeight := effectiveRatioDimensionBounds(capability.MinHeight, capability.MaxHeight)
	for width := minWidth; width <= maxWidth; width += imageSizeMultiple {
		lower := roundUpToImageGrid(maxInt(minHeight, maxInt(ceilDiv(imageMinPixels, width), ceilDiv(width, imageMaxAspectRatioInt))))
		upper := roundDownToImageGrid(minInt(maxHeight, minInt(pixelBudget/width, width*imageMaxAspectRatioInt)))
		if lower <= upper {
			return true
		}
	}
	return false
}

func removeString(values []string, target string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != target {
			result = append(result, value)
		}
	}
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
